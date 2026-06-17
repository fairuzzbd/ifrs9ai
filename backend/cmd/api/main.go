// Command api adalah entry point HTTP server BLIPS IFRS9.
//
// Middleware chain (urutan): RequestID → Recovery → Logger → CORS → RateLimiter
// Mutating routes tambahan: Idempotency → Auth → [RequirePermission per route]
//
// Wiring auth: jika JWT_PUBLIC_KEY_PEM tersedia di env, JWT verification RSA-2048 aktif.
// Di development tanpa key, endpoint /healthz dan /readyz tetap bisa akses tanpa auth.
package main

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/config"
	"blips-ifrs9.tugu-re.com/internal/document"
	"blips-ifrs9.tugu-re.com/internal/master/bobotskenario"
	"blips-ifrs9.tugu-re.com/internal/master/coa"
	"blips-ifrs9.tugu-re.com/internal/master/counterparty"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
	"blips-ifrs9.tugu-re.com/internal/master/instrumen"
	"blips-ifrs9.tugu-re.com/internal/master/kurs"
	"blips-ifrs9.tugu-re.com/internal/master/lgdbasel"
	"blips-ifrs9.tugu-re.com/internal/master/lpscoverage"
	"blips-ifrs9.tugu-re.com/internal/master/mappingjurnal"
	"blips-ifrs9.tugu-re.com/internal/master/matauang"
	"blips-ifrs9.tugu-re.com/internal/master/pdpefindo"
	"blips-ifrs9.tugu-re.com/internal/master/periodebuku"
	"blips-ifrs9.tugu-re.com/internal/master/portofolio"
	"blips-ifrs9.tugu-re.com/internal/master/ratinghistory"
	"blips-ifrs9.tugu-re.com/internal/notification"
	"blips-ifrs9.tugu-re.com/internal/workflow"

	"blips-ifrs9.tugu-re.com/internal/ecl/calcrun"
	eclcore "blips-ifrs9.tugu-re.com/internal/ecl/core"
	"blips-ifrs9.tugu-re.com/internal/ecl/eir"
	"blips-ifrs9.tugu-re.com/internal/ecl/helpers"
	"blips-ifrs9.tugu-re.com/internal/ecl/lookthrough"
	"blips-ifrs9.tugu-re.com/internal/ecl/lps"
	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
	"blips-ifrs9.tugu-re.com/internal/ecl/staging"

	"blips-ifrs9.tugu-re.com/internal/app-b/penempatan"
	jurnal "blips-ifrs9.tugu-re.com/internal/app-d/jurnal"
	"blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// version adalah versi service yang dilaporkan probe liveness.
const version = "0.2.0"

func main() {
	cfg := config.Load()

	// Structured logger (slog). JSON di production, text di development.
	var logger *slog.Logger
	if cfg.AppEnv == "production" || cfg.AppEnv == "staging" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	slog.SetDefault(logger)

	// Mode Gin mengikuti lingkungan.
	if cfg.AppEnv == "production" || cfg.AppEnv == "staging" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Database connection (optional di Phase 2; idempotency middleware skip kalau nil).
	var db *sql.DB
	if cfg.DatabaseURL != "" {
		var err error
		db, err = sql.Open("postgres", cfg.DatabaseURL)
		if err != nil {
			logger.Warn("database connection failed", "error", err)
		} else {
			db.SetMaxOpenConns(25)
			db.SetMaxIdleConns(5)
			db.SetConnMaxLifetime(5 * time.Minute)
		}
	}

	// Redis connection (optional; rate limiter skip kalau nil).
	var rdb *redis.Client
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			logger.Warn("redis URL parse failed", "error", err)
		} else {
			rdb = redis.NewClient(opts)
		}
	}

	// JWT Verifier (mandatory in staging/production; dev-only opt-out).
	// Per security-engineer audit SECURITY-counterparty-2026-06-04 F-01: refuse
	// to boot without JWT in non-dev environments — never silently disable auth.
	var jwtVerifier *auth.Verifier
	if cfg.JWTPublicKeyPEM != "" {
		pk, err := parseRSAPublicKey(cfg.JWTPublicKeyPEM)
		if err != nil {
			logger.Error("JWT public key parse failed", "error", err)
			os.Exit(1)
		}
		jwtVerifier = auth.NewVerifier(pk, cfg.JWTIssuer)
		logger.Info("JWT verification enabled (RSA-2048)")
	} else {
		if cfg.AppEnv == "production" || cfg.AppEnv == "staging" {
			logger.Error("JWT_PUBLIC_KEY_PEM is required in staging/production. Refusing to start.",
				"app_env", cfg.AppEnv)
			os.Exit(1)
		}
		logger.Warn("JWT_PUBLIC_KEY_PEM not set — JWT verification DISABLED (dev only)")
	}

	router := gin.New()

	// Global middleware — urutan penting.
	router.Use(middleware.RequestID())      // 1. Trace ID (inject / propagate)
	router.Use(middleware.Recovery(logger)) // 2. Panic recovery → 500 error envelope
	router.Use(middleware.Logger(logger))   // 3. Structured access log

	// CORS.
	corsCfg := cors.Config{
		AllowOrigins:     splitAndTrim(cfg.CORSAllowedOrigins),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Idempotency-Key", "X-Trace-Id", "X-Step-Up-Token"},
		ExposeHeaders:    []string{"X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsCfg))

	// Rate limiter (setelah auth sehingga bisa baca userId/roles).
	// Dipasang di sini sebagai default; endpoint sensitif pakai middleware.SensitiveRateLimit.
	router.Use(middleware.RateLimiter(rdb, middleware.DefaultRateLimit))

	// Probe endpoints — tidak ada auth, tidak ada rate limit individual.
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"service":   "blips-api",
			"version":   version,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// API v1 group.
	v1 := router.Group("/api/v1")

	// Idempotency middleware untuk semua mutating routes di /api/v1.
	v1.Use(middleware.Idempotency(db))

	// Auth middleware — opsional kalau verifier tidak tersedia (dev mode).
	if jwtVerifier != nil {
		v1.Use(auth.Middleware(jwtVerifier))
	}

	// Auth endpoints (tidak butuh permission, tapi butuh idempotency untuk POST).
	authGroup := v1.Group("/auth")
	{
		// Placeholder handlers — implementasi penuh di next PR (auth flow).
		authGroup.POST("/callback", notImplemented)
		authGroup.POST("/refresh", notImplemented)
		authGroup.POST("/step-up", notImplemented)
		authGroup.POST("/logout", notImplemented)
		authGroup.GET("/me", notImplemented)
	}

	// Workflow engine — config-driven, no entity-specific if-else.
	var wfLoader workflow.ConfigLoader
	if db != nil {
		dbLoader := workflow.NewDBConfigLoader(db)
		wfLoader = workflow.NewCachedConfigLoader(dbLoader)
	} else {
		// Dev mode without DB: use seeded in-memory configs.
		wfLoader = workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())
	}

	wfEngine := workflow.NewEngine(wfLoader)
	wfRepo := workflow.NewDBRepository(db) // nil db = skip gracefully in repo methods
	auditWriter := audit.NewWriter(db)
	wfService := workflow.NewService(wfEngine, wfRepo, auditWriter, logger)
	wfHandler := workflow.NewHandler(wfService)

	// Register generic workflow routes under /api/v1.
	// Pattern: POST /api/v1/{resource}/{id}/{submit,review,approve,approve2,reject}
	//          GET  /api/v1/{resource}/{id}/workflow
	workflow.RegisterRoutes(v1, wfHandler)

	// -----------------------------------------------------------------------
	// Notification service — async via Asynq (DEC-007).
	// Dev-safe: bila SMTP_HOST kosong, mailer dalam dry-run mode (hanya log).
	// -----------------------------------------------------------------------
	smtpCfg := notification.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
		UseTLS:   cfg.SMTPUseTLS,
	}
	mailer := notification.NewMailer(smtpCfg, logger)
	inAppSink := notification.NewLogInAppSink(logger)

	// Template store: DB bila tersedia, fallback in-memory (dev/test).
	var notifTemplateStore notification.TemplateStore
	if db != nil {
		notifTemplateStore = notification.NewDBTemplateStore(db)
	} else {
		notifTemplateStore = notification.NewInMemoryTemplateStore()
	}

	// Asynq client untuk enqueue (opsional — nil = sync mode di dev).
	var asynqClient interface {
		EnqueueContext(ctx interface{}, task interface{}, opts ...interface{}) (interface{}, error)
	}
	_ = asynqClient // Phase 2: wire asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisURL})

	notifSvc := notification.NewService(
		nil, // asynq client: nil = sinkron di dev; Phase 2: asynq.NewClient(...)
		notifTemplateStore,
		mailer,
		inAppSink,
		logger,
	)
	_ = notifSvc // digunakan oleh workflow service (Phase 2 hook)

	if smtpCfg.IsDryRun() {
		logger.Warn("SMTP_HOST tidak diset — notification service dalam dry-run mode (dev only)")
	} else {
		logger.Info("notification service: SMTP aktif", "host", cfg.SMTPHost, "port", cfg.SMTPPort)
	}

	// -----------------------------------------------------------------------
	// Document upload service.
	// Dev-safe: bila MINIO_ENDPOINT tidak bisa di-reach, service masih bisa start
	// (MinIO client dibuat lazy, error hanya muncul saat upload pertama).
	// -----------------------------------------------------------------------
	minioCfg := document.MinIOConfig{
		Endpoint:          cfg.MinIOEndpoint,
		AccessKeyID:       cfg.MinIOAccessKey,
		SecretAccessKey:   cfg.MinIOSecretKey,
		UseSSL:            cfg.MinIOUseSSL,
		PresignTTLMinutes: cfg.DocumentPresignTTLMinutes,
	}

	var minioClient *document.MinIOClient
	if mc, err := document.NewMinIOClient(minioCfg, logger); err != nil {
		logger.Warn("MinIO client init failed — document upload tidak tersedia", "error", err)
	} else {
		minioClient = mc
		// Pastikan bucket utama ada (best effort; tidak fatal bila gagal).
		if err := minioClient.EnsureBucket(context.Background(), document.DefaultBucket); err != nil {
			logger.Warn("MinIO: EnsureBucket gagal (bucket mungkin belum dibuat)", "error", err)
		}
	}

	// /readyz — readiness probe dengan real dependency check.
	// Didaftarkan di sini setelah semua klien (db, rdb, minioClient) diinisialisasi.
	// /healthz (liveness) tetap simple tanpa dependency check.
	router.GET("/readyz", makeReadyzHandler(db, rdb, minioClient))

	docRepo := document.NewDBRepository(db) // nil db = skip DB ops gracefully
	docSvc := document.NewService(docRepo, minioClient, auditWriter, logger).
		WithConfig(document.ServiceConfig{BlockPendingDownload: cfg.BlockPendingDownload})
	docHandler := document.NewHandler(docSvc)

	// Register document routes: POST /api/v1/documents, GET /api/v1/documents/{id}
	document.RegisterRoutes(v1, docHandler)

	// -----------------------------------------------------------------------
	// Master Data — Mata Uang (APP-A-MSTR-002)
	// Routes: GET/POST /api/v1/master/mata-uang, GET/PUT/DELETE /api/v1/master/mata-uang/:kode
	//         GET /api/v1/master/mata-uang/export
	//         POST /api/v1/master/mata-uang/:kode/{submit,review,approve,reject}
	//         GET  /api/v1/master/mata-uang/:kode/{history,workflow}
	// -----------------------------------------------------------------------
	mataUangRepo := matauang.NewDBRepository(db)
	mataUangSvc := matauang.NewService(mataUangRepo, auditWriter, logger)
	mataUangHandler := matauang.NewHandler(mataUangSvc, wfHandler)
	matauang.RegisterRoutes(v1, mataUangHandler)

	// Master Data — Bobot Skenario (APP-C ECL Parameter, DEC-010 sum=1.0)
	// -----------------------------------------------------------------------
	bobotSkenarioRepo := bobotskenario.NewDBRepository(db)
	bobotSkenarioSvc := bobotskenario.NewService(bobotSkenarioRepo, auditWriter, logger)
	bobotSkenarioHandler := bobotskenario.NewHandler(bobotSkenarioSvc, wfHandler)
	bobotskenario.RegisterRoutes(v1, bobotSkenarioHandler)

	bobotSkenarioHook := bobotskenario.NewWorkflowHook(bobotSkenarioSvc, bobotSkenarioRepo)
	wfService.RegisterEntityHook("BOBOT_SKENARIO", bobotSkenarioHook)

	// -----------------------------------------------------------------------
	// Master Data — Chart of Accounts (APP-A-MSTR-COA)
	// Routes: GET/POST /api/v1/master/coa
	//         GET /api/v1/master/coa/export
	//         POST /api/v1/master/coa/import-xlsx
	//         GET  /api/v1/master/coa/import-jobs/:jobId
	//         GET/PATCH/DELETE /api/v1/master/coa/:id
	//         GET /api/v1/master/coa/:id/{history,workflow}
	//         POST /api/v1/master/coa/:id/{submit,review,approve,reject}
	// -----------------------------------------------------------------------
	coaRepo := coa.NewDBRepository(db)
	coaJobRepo := coa.NewDBJobRepository(db)
	coaSvc := coa.NewService(coaRepo, auditWriter, logger)
	coaImporter := coa.NewImporter(coaRepo, coaJobRepo, auditWriter, nil /* Asynq: nil = sync goroutine */, logger)
	coaHandler := coa.NewHandler(coaSvc, coaImporter, wfHandler)
	coa.RegisterRoutes(v1, coaHandler)

	// Register EntityHook so workflow transitions sync coa.workflow_status.
	coaHook := coa.NewWorkflowHook(coaSvc)
	wfService.RegisterEntityHook("CHART_OF_ACCOUNTS", coaHook)

	// -----------------------------------------------------------------------
	// Master Data — Counterparty + Rating History (APP-A, modul 7, 4-eyes).
	// BLOCKING security-engineer untuk PII (DEC-028): npwp/nomor_rekening/ktp
	// encrypted via sec.encrypt/decrypt. PII default response masked, full
	// decrypt via GET /:id/pii (requires counterparty.view_pii + audit).
	// -----------------------------------------------------------------------
	counterpartyRepo := counterparty.NewDBRepository(db)
	counterpartySvc := counterparty.NewService(counterpartyRepo, auditWriter, logger)
	counterpartyHook := counterparty.NewWorkflowHook(counterpartySvc, counterpartyRepo)
	wfService.RegisterEntityHook("COUNTERPARTY", counterpartyHook)
	counterpartyHandler := counterparty.NewHandler(counterpartySvc, wfHandler)
	counterparty.RegisterRoutes(v1, counterpartyHandler)

	ratingHistoryRepo := ratinghistory.NewDBRepository(db)
	ratingHistorySvc := ratinghistory.NewService(ratingHistoryRepo, counterpartyRepo, auditWriter, logger)
	ratingHistoryHook := ratinghistory.NewWorkflowHook(ratingHistorySvc, ratingHistoryRepo)
	wfService.RegisterEntityHook("RATING_HISTORY", ratingHistoryHook)
	ratingHistoryHandler := ratinghistory.NewHandler(ratingHistorySvc, wfHandler)
	ratinghistory.RegisterRoutes(v1, ratingHistoryHandler)
	ratinghistory.RegisterCounterpartyNestedRoutes(v1, ratingHistoryHandler)

	// -----------------------------------------------------------------------
	// Master Data — Impact MEV PD (APP-A ECL Param, DEC-010 dual FL multiplier)
	// mst.impact_mev_pd: MEV-to-PD impact per skenario GOOD/BAD per periode.
	// NORMAL implicitly 1.0 (no row stored — OQ-1 resolved 2026-06-09).
	// Independent from impact_pd (OQ-2 resolved 2026-06-09).
	// 6-eyes: Maker(RISK) -> Reviewer(AKUN-CTL) -> Approver1(RISK) -> Approver2(ALCO).
	// Both approve steps require step-up MFA (DEC-027).
	// -----------------------------------------------------------------------
	impactMevPdRepo := impactmevpd.NewDBRepository(db)
	impactMevPdSvc := impactmevpd.NewService(impactMevPdRepo, auditWriter, logger)
	impactMevPdHook := impactmevpd.NewWorkflowHook(impactMevPdSvc, impactMevPdRepo)
	wfService.RegisterEntityHook("IMPACT_MEV_PD", impactMevPdHook)
	impactMevPdHandler := impactmevpd.NewHandler(impactMevPdSvc, wfHandler)
	impactmevpd.RegisterRoutes(v1, impactMevPdHandler)

	// -----------------------------------------------------------------------
	// Master Data — Impact PD (APP-A ECL Param, DEC-010 final FL PD multiplier)
	// mst.impact_pd: Final FL PD multiplier per periode, applied as:
	//   ECL_FL_skenario = ECL_skenario x impact_pd.impact_multiplier
	// Independent from impact_mev_pd (OQ-2). Range [0.5, 2.0] (OQ-3).
	// 6-eyes: same config as IMPACT_MEV_PD. Both approve steps step-up MFA.
	// ECL engine Phase 4 consumes GET /master/impact-pd/active (OQ-5).
	// -----------------------------------------------------------------------
	impactPdRepo := impactpd.NewDBRepository(db)
	impactPdSvc := impactpd.NewService(impactPdRepo, auditWriter, logger)
	impactPdHook := impactpd.NewWorkflowHook(impactPdSvc, impactPdRepo)
	wfService.RegisterEntityHook("IMPACT_PD", impactPdHook)
	impactPdHandler := impactpd.NewHandler(impactPdSvc, wfHandler)
	impactpd.RegisterRoutes(v1, impactPdHandler)

	// -----------------------------------------------------------------------
	// Master Data — Instrumen (APP-A-MSTR-011)
	// Routes: GET/POST /api/v1/master/instrumen
	//         GET /api/v1/master/instrumen/export
	//         GET/PUT/DELETE /api/v1/master/instrumen/:id
	//         POST /api/v1/master/instrumen/:id/{submit,review,approve,reject}
	//         GET  /api/v1/master/instrumen/:id/{history,workflow}
	// -----------------------------------------------------------------------
	instrumenRepo := instrumen.NewDBRepository(db)
	instrumenSvc := instrumen.NewService(instrumenRepo, auditWriter, logger)
	instrumenHook := instrumen.NewWorkflowHook(instrumenSvc, instrumenRepo)
	wfService.RegisterEntityHook("INSTRUMEN", instrumenHook)
	instrumenHandler := instrumen.NewHandler(instrumenSvc, wfHandler)
	instrumen.RegisterRoutes(v1, instrumenHandler)

	// -----------------------------------------------------------------------
	// Master Data — Kurs / FX Rates (APP-A-MSTR-009)
	// Routes: GET/POST /api/v1/master/kurs
	//         POST    /api/v1/master/kurs/jisdor-sync
	//         GET     /api/v1/master/kurs/export
	//         GET/PUT/DELETE /api/v1/master/kurs/:id
	//         POST    /api/v1/master/kurs/:id/{submit,review,approve,reject}
	//         GET     /api/v1/master/kurs/:id/{history,workflow}
	// -----------------------------------------------------------------------
	kursRepo := kurs.NewDBRepository(db)
	kursSvc := kurs.NewService(kursRepo, auditWriter, logger)
	kursHook := kurs.NewWorkflowHook(kursSvc, kursRepo)
	wfService.RegisterEntityHook("KURS", kursHook)
	kursHandler := kurs.NewHandler(kursSvc, wfHandler)
	kurs.RegisterRoutes(v1, kursHandler)

	// -----------------------------------------------------------------------
	// Master Data — LGD Basel (APP-C-MSTR-ECL-001, 6-eyes + step-up MFA)
	// -----------------------------------------------------------------------
	lgdBaselRepo := lgdbasel.NewDBRepository(db)
	lgdBaselSvc := lgdbasel.NewService(lgdBaselRepo, auditWriter, logger)
	lgdBaselHandler := lgdbasel.NewHandler(lgdBaselSvc, wfHandler)
	lgdbasel.RegisterRoutes(v1, lgdBaselHandler)

	// -----------------------------------------------------------------------
	// Master Data — LPS Coverage (APP-C ECL Parameter, DEC-014 IDR 2M cap)
	// -----------------------------------------------------------------------
	lpsCoverageRepo := lpscoverage.NewDBRepository(db)
	lpsCoverageSvc := lpscoverage.NewService(lpsCoverageRepo, auditWriter, logger)
	lpsCoverageHook := lpscoverage.NewWorkflowHook(lpsCoverageSvc, lpsCoverageRepo)
	wfService.RegisterEntityHook("LPS_COVERAGE", lpsCoverageHook)
	lpsCoverageHandler := lpscoverage.NewHandler(lpsCoverageSvc, wfHandler)
	lpscoverage.RegisterRoutes(v1, lpsCoverageHandler)

	// -----------------------------------------------------------------------
	// Master Data — Mapping Jurnal (APP-D)
	// Routes: GET/POST /api/v1/master/mapping-jurnal
	//         GET/PATCH/DELETE /api/v1/master/mapping-jurnal/:id
	//         GET /api/v1/master/mapping-jurnal/export
	//         POST /api/v1/master/mapping-jurnal/:id/{submit,review,approve,reject}
	//         GET  /api/v1/master/mapping-jurnal/:id/{history,workflow}
	//
	// Workflow hook: keeps mst.mapping_jurnal_header.workflow_status in sync with
	// sys.workflow_instance after each transition. On APPROVE also enforces:
	//   - sum(DEBIT multiplier) == sum(KREDIT multiplier) ±0.0001
	//   - All referenced CoA rows must have workflow_status='APPROVED'
	// -----------------------------------------------------------------------
	mappingJurnalRepo := mappingjurnal.NewDBRepository(db)
	mappingJurnalSvc := mappingjurnal.NewService(mappingJurnalRepo, auditWriter, logger)
	mappingJurnalHook := mappingjurnal.NewWorkflowHook(mappingJurnalSvc, mappingJurnalRepo)
	wfService.RegisterEntityHook("MAPPING_JURNAL", mappingJurnalHook)
	mappingJurnalHandler := mappingjurnal.NewHandler(mappingJurnalSvc, wfHandler)
	mappingjurnal.RegisterRoutes(v1, mappingJurnalHandler)

	// -----------------------------------------------------------------------
	// Master Data — PD Pefindo (APP-A ECL Param, MSTR-PDPefindo)
	// 6-eyes workflow: ROLE-RISK -> ROLE-AKUN-CTL -> ROLE-ALCO x 2
	// Both approve + approve2 require step-up MFA (DEC-027).
	// -----------------------------------------------------------------------
	pdPefindoRepo := pdpefindo.NewDBRepository(db)
	pdPefindoSvc := pdpefindo.NewService(pdPefindoRepo, auditWriter, logger)
	pdPefindoUploadSvc := pdpefindo.NewUploadService(pdPefindoRepo, auditWriter, logger)
	pdPefindoHandler := pdpefindo.NewHandler(pdPefindoSvc, pdPefindoUploadSvc, wfHandler, nil /* asynq: nil = sync fallback in dev */)
	pdpefindo.RegisterRoutes(v1, pdPefindoHandler)
	pdPefindoHook := pdpefindo.NewWorkflowHook(pdPefindoSvc, pdPefindoRepo)
	wfService.RegisterEntityHook("PD_PEFINDO", pdPefindoHook)

	// -----------------------------------------------------------------------
	// Master Data — Periode Buku (APP-D-MSTR-001)
	// -----------------------------------------------------------------------
	periodeBukuRepo := periodebuku.NewDBRepository(db)
	periodeBukuSvc := periodebuku.NewService(periodeBukuRepo, auditWriter, logger)
	periodeBukuHandler := periodebuku.NewHandler(periodeBukuSvc, wfHandler)
	periodebuku.RegisterRoutes(v1, periodeBukuHandler)

	// -----------------------------------------------------------------------
	// P5-M4: Periode Buku Close Workflow (APP-D-CLOSE-001..007)
	// Routes (all under /api/v1/periode-buku/:id/):
	//   POST   /soft-close-request          — ROLE-AKUN-CTL submit soft-close (M4-001)
	//   POST   /soft-close-approve          — ROLE-CFO approve soft-close (M4-002)
	//   POST   /hard-close-request          — ROLE-CFO request hard-close (M4-003)
	//   POST   /hard-close-approve          — ROLE-CFO approve (step-up MFA) (M4-004)
	//   POST   /hard-close-reject           — ROLE-CFO reject hard-close (M4-005)
	//   POST   /reopen-request              — ROLE-CFO request reopen (M4-006)
	//   POST   /reopen-approve              — ROLE-CFO approve reopen (M4-006)
	//   GET    /checklist                   — closing checklist status (M4-007)
	//   GET    /reports/status-periode      — list all periods status (M4-007)
	//
	// State machine: OPEN → SOFT_CLOSED → HARD_CLOSE_PENDING → CLOSED
	// Reopen paths: SOFT_CLOSED → OPEN (grace window), CLOSED → SOFT_CLOSED (grace window + step-up).
	// DEC-017 SoD: approver ≠ requester. DEC-027 step-up MFA for hard-close-approve + CLOSED reopen.
	// DEC-018 audit in-tx. DEC-021 Idempotency-Key wajib.
	// PeriodeLockMiddleware: blocks all mutations on SOFT_CLOSED/HARD_CLOSE_PENDING/CLOSED routes.
	// Asynq: ApproveHardClose enqueues "reporting:mv_refresh" task (non-fatal if Redis unavailable).
	// -----------------------------------------------------------------------
	var closeflowEnqueuer closeflow.AsynqEnqueuer
	if cfg.RedisURL != "" {
		closeflowEnqueuer = asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisURL})
	}
	closeflowRepo := closeflow.NewRepo(db)
	closeflowChecklist := closeflow.NewChecklistService(db)
	closeflowSvc := closeflow.NewService(
		closeflowRepo,
		closeflowChecklist,
		auditWriter,
		closeflowEnqueuer, // nil when Redis not set → MV refresh skipped (non-fatal)
		closeflow.DefaultConfig(),
		logger,
	)
	closeflowHandler := closeflow.NewHandler(closeflowSvc)
	closeflowLockMW := closeflow.NewPeriodeLockMiddleware(closeflowRepo, closeflow.DefaultConfig())
	closeflow.RegisterRoutes(router, closeflowHandler, closeflowLockMW)
	closeflow.RegisterLockMiddlewareRoutes(v1, closeflowLockMW)

	// -----------------------------------------------------------------------
	// Master Data — Portofolio (APP-A-MSTR-010)
	// Routes: GET/POST /api/v1/master/portofolio
	//         GET/PUT/DELETE /api/v1/master/portofolio/:kode
	//         GET /api/v1/master/portofolio/export
	//         POST /api/v1/master/portofolio/:kode/{submit,review,approve,reject}
	//         GET  /api/v1/master/portofolio/:kode/{history,workflow}
	// -----------------------------------------------------------------------
	portofolioRepo := portofolio.NewDBRepository(db)
	portofolioSvc := portofolio.NewService(portofolioRepo, auditWriter, logger)
	portofolioHook := portofolio.NewWorkflowHook(portofolioSvc, portofolioRepo)
	wfService.RegisterEntityHook("PORTOFOLIO", portofolioHook)
	portofolioHandler := portofolio.NewHandler(portofolioSvc, wfHandler)
	portofolio.RegisterRoutes(v1, portofolioHandler)

	// -----------------------------------------------------------------------
	// ECL Staging Engine (APP-C-STG-001..005, Phase 4 Module 1)
	// Endpoints:
	//   POST   /ecl/staging/evaluate
	//   GET    /ecl/staging/instrumen/:id
	//   GET    /ecl/staging/instrumen/:id/history
	//   POST   /ecl/staging/override/submit
	//   POST   /ecl/staging/override/:id/{review,approve,approve2,reject}
	//   GET    /ecl/staging/overrides
	//   POST   /ecl/dpd/record
	//   GET    /ecl/dpd/instrumen/:id
	//
	// Note: staging package manages its own workflow state directly in
	// ecl.staging_override_proposal — no sys.workflow_instance used.
	// The WorkflowHook below is an extension point for Phase 5.
	// -----------------------------------------------------------------------
	stagingDPDRepo := staging.NewDBDPDRepository(db)
	stagingHistRepo := staging.NewDBStageHistoryRepository(db)
	stagingOverrideRepo := staging.NewDBOverrideProposalRepository(db)
	// instrumenReader adapter: queries mst.instrumen + mst.rating_history_counterparty
	// directly via *sql.DB, avoiding circular imports with the instrumen package.
	stagingInstrumenReader := staging.NewDBInstrumenReader(db)
	// periodeReader adapter: queries mst.periode_buku directly via *sql.DB.
	stagingPeriodeReader := staging.NewDBPeriodeBukuReader(db)
	stagingSvc := staging.NewService(
		stagingDPDRepo,
		stagingHistRepo,
		stagingOverrideRepo,
		stagingInstrumenReader,
		stagingPeriodeReader,
		auditWriter,
		logger,
	)
	stagingHandler := staging.NewHandler(stagingSvc)
	staging.RegisterRoutes(v1, stagingHandler, jwtVerifier, db)
	stagingHook := staging.NewWorkflowHook(stagingOverrideRepo)
	wfService.RegisterEntityHook("STAGING_OVERRIDE", stagingHook)

	// -----------------------------------------------------------------------
	// ECL Helpers — PD/LGD/EAD/CCF Lookup (APP-C-PAR-001..006, Phase 4 Module 2)
	// Endpoints (all under /api/v1/ecl/helpers/):
	//   GET  pd              — single PD lookup (Stage 1/2/3 × Good/Normal/Bad)
	//   POST pd/bulk         — batch PD (max 1000 items)
	//   GET  lgd             — single LGD lookup (pool-based Basel-style)
	//   POST lgd/bulk        — batch LGD
	//   GET  ead             — single EAD (IDR/FCY, EIR schedule, CCF)
	//   POST ead/bulk        — batch EAD
	//   GET  ccf             — single CCF lookup
	//   GET  preview         — paginated pre-run ECL applicable instrument list
	//   GET  preview/export  — async export (> 10k row → MinIO + notify)
	//   POST bulk-lookup     — combined PD+LGD+EAD+CCF per instrument per periode
	//
	// Permissions: ecl_helpers.read (all except preview) + ecl_helpers.preview
	// ECL formula: SoW §4, FSD-APP-C §3 — uses ALCO-approved params (immutable after seal)
	// Anti-N+1: loadBatchParams loads all data in ≤ 11 DB round-trips
	// -----------------------------------------------------------------------
	helpersSvc := helpers.NewServices(db, auditWriter)
	helpersHandler := helpers.NewHandler(helpersSvc)
	helpers.RegisterRoutes(v1.Group("/ecl"), helpersHandler)

	// -----------------------------------------------------------------------
	// ECL LPS Aggregator (APP-C-LPS-001..005, Phase 4 Module 3)
	// Endpoints (all under /api/v1/ecl/lps/):
	//   POST  aggregate              — single (nasabah,bank) pair LPS coverage
	//   POST  aggregate/bulk         — bulk all pairs for periodeId (202 job)
	//   GET   preview                — DataTable coverage utilization
	//   GET   preview/export         — CSV/XLSX export (inline or async)
	//   POST  override/submit        — ROLE-RISK propose exclusion
	//   POST  override/:id/approve   — ROLE-ALCO approve (MFA required DEC-026)
	//   POST  override/:id/reject    — ROLE-RISK or ROLE-ALCO reject
	//   GET   overrides              — DataTable override list
	//   GET   overrides/:id          — detail
	//
	// Decisions: DEC-010, DEC-014, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022.
	// Formula: SoW §4.3, FSD-APP-C §3.3. Cap IDR 2B per nasabah per bank.
	// -----------------------------------------------------------------------
	lpsDepositoRepo := lps.NewDBDepositoInstrumenRepo(db)
	lpsCovRepoForAgg := lps.NewDBLPSCoverageRepo(db)
	lpsOverrideRepo := lps.NewDBOverrideRepo(db)

	// KursRepoIface adapter: kurs.Repository.GetByKodeAndDate → lps.KursRepoIface.
	lpsKursAdapter := lps.NewKursAdapter(func(ctx context.Context, kode string, tanggal time.Time) (string, bool, error) {
		k, err := kursRepo.GetByKodeAndDate(ctx, kode, tanggal)
		if err != nil || k == nil {
			return "", false, err
		}
		return k.KursTengah.StringFixed(8), true, nil
	})

	// PeriodeBukuRepoIface adapter: periodebuku.Repository.GetByID → lps.PeriodeBukuRepoIface.
	lpsPeriodeAdapter := lps.NewDBPeriodeBukuReader(func(ctx context.Context, id uuid.UUID) (string, string, bool, error) {
		pb, err := periodeBukuRepo.GetByID(ctx, id)
		if err != nil || pb == nil {
			return "", "", false, err
		}
		return pb.TanggalMulai, pb.TanggalAkhir, true, nil
	})

	lpsAggregatorSvc := lps.NewAggregatorService(lpsCovRepoForAgg, lpsDepositoRepo, lpsOverrideRepo, lpsKursAdapter)
	lpsOverrideSvc := lps.NewOverrideService(db, lpsOverrideRepo, lpsPeriodeAdapter, lps.NewAuditWriterAdapter(audit.NewWriter(db)))
	lpsHandler := lps.NewHandler(lpsAggregatorSvc, lpsOverrideSvc)
	lps.RegisterRoutes(v1, lpsHandler, jwtVerifier, db)

	// Register workflow hook for LPS_EXCLUSION_OVERRIDE entity type.
	lpsOverrideHook := lps.NewOverrideWorkflowHook(lpsOverrideRepo)
	wfService.RegisterEntityHook(lpsOverrideHook.EntityType(), lpsOverrideHook)

	// -----------------------------------------------------------------------
	// LPS Expiry Worker — Asynq job: lps:expiry-check (issue #47)
	// Transitions stale APPROVED_ACTIVE overrides → EXPIRED daily at 01:00 WIB.
	// Schedule: "@every 24h" (registered below); production uses cron "@daily 01:00 Asia/Jakarta".
	// Worker panics at startup if auditWriter is nil (DEC-018 compliance guard).
	// -----------------------------------------------------------------------
	lpsExpiryRepo := lps.NewDBExpiryRepo(db)
	lpsExpiryAuditWriter := lps.NewAuditWriterAdapter(audit.NewWriter(db))
	// System actor UUID: fixed well-known value for SYSTEM-initiated transitions.
	// Same pattern as staging.HandleOverrideExpiryCheck (staging/worker_tasks.go:231).
	lpsSystemUserID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	lpsExpiryWorker := lps.NewExpiryWorker(lpsExpiryRepo, lpsExpiryAuditWriter, logger, lpsSystemUserID)
	_ = lpsExpiryWorker // registered on Asynq mux below (Phase 2: wire asynq.ServeMux)

	// -----------------------------------------------------------------------
	// Look-through ECL (APP-C-LKT-001..005, Phase 4 Module 4)
	// Endpoints (all under /api/v1/ecl/lookthrough/):
	//   POST  composition/submit           — ROLE-AKUN submit fund composition
	//   POST  composition/:id/review       — ROLE-RISK review (SoD: reviewer ≠ maker)
	//   POST  composition/:id/approve      — ROLE-ALCO approve (MFA wajib DEC-026)
	//   POST  composition/:id/reject       — reject with reason
	//   POST  composition/:id/amend        — amend (supersedes old APPROVED_ACTIVE)
	//   GET   compositions                 — DataTable list by instrumen
	//   GET   compositions/:id             — detail + lines
	//   POST  compute                      — single instrument ECL
	//   POST  compute/bulk                 — bulk all REKSADANA (202 + jobId)
	//   GET   preview                      — DataTable preview with ECL estimate
	//   GET   preview/export               — CSV/async XLSX export
	//   GET   result/:instrumenId/:runId   — stored result detail
	//
	// Decisions: DEC-010, DEC-015, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022.
	// Formula: SoW §4.4, FSD-APP-C §3.4. 3-skenario × dual FL multiplier.
	// -----------------------------------------------------------------------
	ltAuditWriter := lookthrough.NewAuditWriterAdapter(audit.NewWriter(db))
	ltCompRepo := lookthrough.NewDBFundCompositionRepo(db)
	ltInstRepo := lookthrough.NewDBReksadanaInstrumenRepo(db)
	ltPDLGDRepo := lookthrough.NewDBPDLGDClassRepo(db)
	ltParamRepo := lookthrough.NewDBScenarioParamRepo(db)
	ltResultRepo := lookthrough.NewDBLookthroughResultRepo(db)

	ltCompositionSvc := lookthrough.NewCompositionService(db, ltCompRepo, ltAuditWriter, logger)
	ltLookthroughSvc := lookthrough.NewLookthroughService(
		db, ltInstRepo, ltCompRepo, ltPDLGDRepo, ltParamRepo, ltResultRepo,
		ltAuditWriter, lookthrough.NoopMetrics(), logger,
	)
	ltHandler := lookthrough.NewHandler(ltCompositionSvc, ltLookthroughSvc, ltResultRepo)
	lookthrough.RegisterRoutes(v1, ltHandler, jwtVerifier, db)

	ltCompositionHook := lookthrough.NewCompositionWorkflowHook(ltCompRepo)
	wfService.RegisterEntityHook(ltCompositionHook.EntityType(), ltCompositionHook)

	// -----------------------------------------------------------------------
	// EIR Newton-Raphson + Schedule + Amendment workflow (APP-C-EIR-001..005, P4-M5)
	// Endpoints (all under /api/v1/ecl/eir/):
	//   POST  compute                       — NR solve + optional persist (Story 1)
	//   POST  generate-schedule             — amortization schedule (Story 2)
	//   GET   schedule/:instrumenId         — active schedule DataTable (Story 3)
	//   GET   schedule/:instrumenId/history — full history (superseded) (Story 3)
	//   POST  amendments                    — propose amendment (Story 4)
	//   GET   amendments                    — list amendments (Story 4)
	//   GET   amendments/:id                — detail (Story 4)
	//   POST  amendments/:id/review         — ROLE-RISK review (Story 4)
	//   POST  amendments/:id/approve        — ROLE-ALCO approve + step-up MFA (Story 4)
	//   POST  amendments/:id/reject         — reject (Story 4)
	//   POST  bulk-recompute                — report-only bulk 202+jobId (Story 5)
	//
	// Decisions: DEC-013, DEC-016, DEC-017, DEC-018.
	// -----------------------------------------------------------------------
	eirAuditWriter := eir.NewAuditWriterAdapter(audit.NewWriter(db))
	eirInstrRepo := eir.NewDBInstrumenEIRRepo(db)
	eirSchedRepo := eir.NewDBEIRScheduleRepo(db)
	eirAmendRepo := eir.NewDBAmendmentRepo(db)

	eirComputeSvc := eir.NewService(db, eirInstrRepo, eirAuditWriter, logger)
	eirScheduleSvc := eir.NewScheduleService(db, eirInstrRepo, eirSchedRepo, eirAuditWriter, logger)
	eirAmendSvc := eir.NewAmendmentService(db, eirInstrRepo, eirSchedRepo, eirAmendRepo, eirAuditWriter, logger)
	eirBulkSvc := eir.NewBulkService(db, eirInstrRepo, eirSchedRepo, nil, nil, logger)

	// M6: EIR Amendment Lifecycle — detect from document, daily drift cron,
	// ad-hoc bulk re-estimation, review queue, cancel/withdraw.
	// NewHandlerM6 extends M5 handler with detectionSvc + driftSvc.
	// All routes (M5 + M6) registered via the single RegisterRoutes call below.
	//
	// Daily cron (M6-002): NewDriftCronHandler registered on Asynq mux (Phase 5 worker binary).
	// Decisions: DEC-013, DEC-016, DEC-017, DEC-018.
	eirDriftRepo := eir.NewDBDriftReportRepo(db)
	// B2 fix: wire document repo into DetectionService so document_category is validated
	// before creating an amendment proposal from a document upload (M6-001).
	// docRepo was created earlier (document.NewDBRepository(db)); docRepo satisfies
	// eir.DocumentTypeRepoIface via its GetDocType method.
	eirDetectionSvc := eir.NewDetectionService(db, eirInstrRepo, eirAmendRepo, eirAuditWriter, logger).
		WithDocTypeRepo(docRepo)
	eirDriftSvc := eir.NewDriftService(db, eirInstrRepo, eirSchedRepo, eirAmendRepo, eirDriftRepo, eir.NewSolver(), eirAuditWriter, logger)
	eirHandler := eir.NewHandlerM6(eirComputeSvc, eirScheduleSvc, eirAmendSvc, eirBulkSvc, eirDetectionSvc, eirDriftSvc)
	eir.RegisterRoutes(v1, eirHandler, jwtVerifier, db)

	// -----------------------------------------------------------------------
	// ECL Core Orchestrator (APP-C-ECL-001..007, Phase 4 Module 7)
	// Endpoints (all under /api/v1/ecl/):
	//   POST  compute                                        — single instrument ECL
	//   POST  compute/bulk                                   — bulk 202+jobId (Asynq)
	//   GET   results/:calcRunId                             — paginated result lines
	//   GET   results/:calcRunId/instrumen/:instrumenId      — single result row
	//   GET   results/:calcRunId/portofolio/:id/summary      — portfolio aggregate
	//   GET   results/:calcRunId/roll-forward                — CKPN roll-forward report
	//   POST  recompute/ad-hoc                               — preview recompute (ROLE-RISK)
	//
	// Decisions: DEC-010, DEC-013, DEC-016, DEC-017, DEC-018.
	// Formula: SoW §4, FSD-APP-C §3.
	// -----------------------------------------------------------------------

	// BobotRepo adapter: reads mst.bobot_skenario APPROVED_ACTIVE rows for a periodeID.
	// Implements eclcore.BobotRepo interface inline.
	eclBobotRepo := eclcore.NewDBBobotRepo(db)

	// InstrumenReader adapter: reads mst.instrumen snapshot for M7.
	eclInstrReader := eclcore.NewDBInstrumenReader(db)

	eclOrchestrator := eclcore.NewOrchestrator(
		db,
		auditWriter,
		helpersSvc,
		lpsAggregatorSvc, // M3 LPS aggregator
		ltLookthroughSvc, // M4 look-through
		eclInstrReader,
		eclBobotRepo,
		logger,
	)
	eclBulkWorker := eclcore.NewBulkWorker(eclOrchestrator, nil, logger)
	eclHandler := eclcore.NewHandler(eclOrchestrator)
	eclcore.RegisterRoutes(v1, eclHandler)

	// -----------------------------------------------------------------------
	// ECL Calc Run Lifecycle + Seal (APP-C-CALC-RUN-001..006, Phase 4 Module 8)
	// Endpoints (all under /api/v1/ecl/calc-runs/):
	//   POST   /ecl/calc-runs                              — create DRAFT (M8-001)
	//   GET    /ecl/calc-runs                              — list (M8-003)
	//   GET    /ecl/calc-runs/:id                          — detail (M8-003)
	//   POST   /ecl/calc-runs/:id/start                    — start → IN_PROGRESS (M8-002)
	//   POST   /ecl/calc-runs/:id/cancel                   — cancel with reason (M8-005)
	//   GET    /ecl/calc-runs/:id/parameter-snapshot       — frozen params JSONB (M8-002)
	//   GET    /ecl/calc-runs/:id/result-lines             — list result lines (M8-003)
	//   POST   /ecl/calc-runs/:id/seal/request             — ROLE-RISK request seal (M8-004)
	//   POST   /ecl/calc-runs/:id/seal/approve             — ROLE-ALCO approve + step-up MFA (M8-004)
	//   POST   /ecl/calc-runs/:id/seal/reject              — ROLE-ALCO reject (M8-004)
	//
	// State machine: docs/state-machines/p4-m8-calc-run.md
	// Seal: 4-eyes (RISK request → ALCO approve), LOCKED per §4.
	// After SEALED: block new calc_run for same periode (DB trigger + service guard).
	// Parameter snapshot: all ALCO-approved params frozen at /start (DEC-016, DEC-018).
	// Decisions: DEC-010, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022, DEC-027.
	// -----------------------------------------------------------------------
	calcRunRepo := calcrun.NewRepo(db)
	calcRunSnapshotSvc := calcrun.NewParameterSnapshotService(db)
	calcRunSvc := calcrun.NewService(calcRunRepo, calcRunSnapshotSvc, auditWriter, nil /* asynqClient: wired below */, nil /* jobUpdater: noop */, logger)
	// Wire M7 orchestrator's CalcRunSealChecker: prevents ECL compute on sealed runs.
	eclOrchestrator.WithSealChecker(calcRunSvc)
	calcRunWorker := calcrun.NewWorker(calcRunSvc, eclOrchestrator, nil, logger)
	calcRunHandler := calcrun.NewHandler(calcRunSvc)
	calcrun.RegisterRoutes(v1, calcRunHandler, jwtVerifier, db)
	_ = calcRunWorker // registered on asynqMux below

	// -----------------------------------------------------------------------
	// P4-M11 Roll-Forward CKPN (APP-C-M11-001..006)
	// Computes CKPN movement table: opening → transfers → originations →
	// derecognitions → remeasurements → closing. Reconcile: |delta| < IDR 1.
	//
	// Routes:
	//   POST   /ecl/roll-forward/compute                     — compute full report (M11-001)
	//   GET    /ecl/roll-forward                             — get report (M11-004)
	//   GET    /ecl/roll-forward/:id/export                  — disclosure XLSX (M11-005)
	//   GET    /ecl/roll-forward/portfolios/:pid             — per-portfolio breakdown (M11-004)
	//   GET    /ecl/roll-forward/portfolios/:pid/instruments — instrument list (M11-004)
	//   GET    /ecl/dashboard/ckpn-trend                     — trend dashboard (M11-006)
	//
	// Read-only: no schema migrations (reads M7 result lines).
	// OQ resolutions: reconcile tolerance IDR 1.0000, sign convention, Stage 3→1 override only.
	// Decisions: DEC-010, DEC-016, DEC-018, DEC-021, DEC-022.
	// -----------------------------------------------------------------------
	rfRepo := rollforward.NewRepo(db)
	rfSvc := rollforward.NewService(rfRepo, db, auditWriter, logger)
	rfWorker := rollforward.NewWorker(rfSvc, logger)
	_ = rfWorker // registered on Asynq mux below (when REDIS_URL is set)
	// rfHandler is wired with an Asynq client when Redis is available for >1000 instrument
	// async dispatch (Issue #88, state machine §1). Client is nil in dev mode.
	// The asynqClient is created below in the Redis block; we set a pointer here that
	// the Asynq block will populate before RegisterRoutes is called.
	// To avoid forward-reference, we build handler after the Redis block.
	// Temporarily build without client; re-assigned below with client if Redis available.
	rfHandler := rollforward.NewHandler(rfSvc)
	rollforward.RegisterRoutes(v1, rfHandler, jwtVerifier, db)

	// -----------------------------------------------------------------------
	// APP-B P5-M1 — Penempatan Deposito (4-eyes workflow, DEC-P5-M1-001..005)
	// Endpoints (all under /api/v1/trx/penempatan-deposito/):
	//   POST   /                          — ROLE-MAKER-TR create draft (Story 1)
	//   GET    /                          — list (cursor, sort, filter) (Story 1)
	//   GET    /:id                       — detail (Story 1)
	//   PATCH  /:id                       — update draft (Story 1)
	//   DELETE /:id                       — withdraw draft (Story 1)
	//   POST   /:id/submit                — maker submits (Story 2)
	//   POST   /:id/review               — reviewer signs (Story 2)
	//   POST   /:id/approve              — approver signs + FVTPL guard + EIR dispatch (Story 2)
	//   POST   /:id/reject               — reject to draft (Story 2)
	//   POST   /:id/terminate            — terminate request (Story 3)
	//   POST   /:id/terminate-review     — terminate reviewer signs (Story 3)
	//   POST   /:id/terminate-approve    — terminate approver signs (Story 3)
	//   POST   /:id/terminate-reject     — terminate reject (Story 3)
	//   GET    /:id/eir-preview          — EIR amortization preview (Story 4)
	//   GET    /:id/audit-timeline       — audit event timeline (Story 5)
	//
	// FVTPL guard (DEC-P5-M1-001): On Approve, if klasifikasi IN ('FVTPL','FVOCI_ELECTION')
	//   → audit PENEMPATAN.STAGING_SKIPPED_FVTPL only; else INSERT ecl.stage_history +
	//   audit PENEMPATAN.STAGING_INITIAL + dispatch EIR_COMPUTE task.
	// Settlement balance hint (DEC-P5-M1-004): informational, never blocks.
	// Terminate SoD (DEC-P5-M1-005): terminate_maker ≠ terminate_reviewer ≠ terminate_approver.
	// Maturity cron: daily 02:00 WIB (0 19 * * * UTC) → auto-transition APPROVED_ACTIVE → MATURED.
	// asynqClient nil → EIR_COMPUTE dispatch skipped gracefully in dev (no Redis).
	// -----------------------------------------------------------------------
	penRepo := penempatan.NewRepo(db)
	penSvc := penempatan.NewService(penRepo, auditWriter, nil /* asynqClient: wired below */, logger)
	penHandler := penempatan.NewHandler(penSvc)
	penempatan.RegisterRoutes(v1, penHandler, jwtVerifier, db)

	// -----------------------------------------------------------------------
	// APP-D P5-M2 — Jurnal Event Resolver & Posting Engine
	// Endpoints (all under /api/v1/jurnal/):
	//   POST   /jurnal/mapping-headers                         — create mapping draft
	//   GET    /jurnal/mapping-headers                         — list (cursor, sort, filter)
	//   GET    /jurnal/mapping-headers/export                  — async export
	//   GET    /jurnal/mapping-headers/:id                     — detail
	//   PATCH  /jurnal/mapping-headers/:id                     — edit draft
	//   POST   /jurnal/mapping-headers/:id/{submit,review,...} — workflow transitions
	//   POST   /jurnal/resolve                                  — resolver preview (no write)
	//   POST   /jurnal/post                                    — manual post (PERIODE_ADJUSTMENT)
	//   GET    /jurnal                                         — list jurnal headers
	//   GET    /jurnal/:id                                     — jurnal detail
	//   GET    /jurnal/:id/export                              — single jurnal export
	//   GET    /jurnal/export                                  — bulk export
	//   POST   /jurnal/:id/{submit,approve,reject}             — manual posting workflow
	//   GET    /jurnal/dlq                                     — DLQ list
	//   GET    /jurnal/dlq/:id                                 — DLQ detail
	//   POST   /jurnal/dlq/:id/{replay,discard}               — DLQ actions
	//
	// DEC-P5-M1-002: 27 event codes; DEC-P5-M1-003: 6-eyes regulated.
	// DEC-017 SoD, DEC-018 audit-in-tx, DEC-027 step-up MFA on regulated approve.
	// Balance invariant: Σ DEBIT = Σ KREDIT enforced at service + DB level.
	// Append-only jrnl.* enforced by DB triggers (migration 000035).
	// -----------------------------------------------------------------------
	jurnalMappingRepo := jurnal.NewMappingRepo(db)
	jurnalHeaderRepo := jurnal.NewJurnalRepo(db)
	jurnalDLQRepo := jurnal.NewDLQRepo(db)

	jurnalMappingSvc := jurnal.NewMappingService(jurnalMappingRepo, auditWriter, logger)
	jurnalResolverSvc := jurnal.NewResolverService(jurnalMappingRepo, db, logger)
	jurnalPostingSvc := jurnal.NewPostingService(jurnalHeaderRepo, jurnalDLQRepo, jurnalResolverSvc, auditWriter, logger)
	jurnalDLQSvc := jurnal.NewDLQService(jurnalDLQRepo, jurnalPostingSvc, auditWriter, logger)

	jurnalHandler := jurnal.NewHandler(jurnalMappingSvc, jurnalResolverSvc, jurnalPostingSvc, jurnalDLQSvc)
	jurnal.RegisterRoutes(v1, jurnalHandler, jwtVerifier, db)

	// Asynq worker for P5-M2 (registered on asynqMux below when Redis is set).
	jurnalWorker := jurnal.NewWorker(jurnalPostingSvc, jurnalDLQRepo, logger)
	_ = jurnalWorker // registered in Redis block below

	// -----------------------------------------------------------------------
	// P5-M3: GL Host REST Delivery — DeliveryService, DLQService, ReconciliationService.
	// Uses StubAdapter by default (dev mode); RESTAdapter when GL_HOST_URL is set.
	// All services panic on nil auditWriter (DEC-018).
	// -----------------------------------------------------------------------
	glJurnalRepo := gldelivery.NewJurnalGLRepo(db)
	glDLQRepo := gldelivery.NewDLQRepo(db)
	glReportRepo := gldelivery.NewReconReportRepo(db)
	glMismatchRepo := gldelivery.NewReconMismatchRepo(db)

	glCfg := gldelivery.DefaultConfig()
	var glAdapter gldelivery.GLHostAdapter
	if glHostURL := os.Getenv("GL_HOST_URL"); glHostURL != "" {
		restAdapter, adapterErr := gldelivery.NewRESTAdapter(gldelivery.RESTAdapterConfig{
			BaseURL:        glHostURL,
			AuthType:       os.Getenv("GL_HOST_AUTH_TYPE"),
			APIKey:         os.Getenv("GL_HOST_API_KEY"),
			TimeoutSeconds: 30,
			PIIFields:      glCfg.PIIFields,
		})
		if adapterErr != nil {
			logger.Warn("P5-M3: GL Host RESTAdapter init failed — using StubAdapter", "error", adapterErr)
			glAdapter = gldelivery.NewStubAdapter()
		} else {
			glAdapter = restAdapter
			logger.Info("P5-M3: GL Host RESTAdapter active", "url", glHostURL)
		}
	} else {
		glAdapter = gldelivery.NewStubAdapter()
		logger.Warn("P5-M3: GL_HOST_URL not set — GL delivery using StubAdapter (dev mode)")
	}

	glDeliverySvc := gldelivery.NewDeliveryService(glJurnalRepo, glDLQRepo, glAdapter, auditWriter, nil /* enqueuer wired below */, glCfg, logger)
	glDLQSvc := gldelivery.NewDLQService(glDLQRepo, glJurnalRepo, glDeliverySvc, auditWriter, nil, logger)
	glReconSvc := gldelivery.NewReconciliationService(glJurnalRepo, glReportRepo, glMismatchRepo, glAdapter, auditWriter, nil, glCfg, logger)

	glHandler := gldelivery.NewHandler(glDeliverySvc, glDLQSvc, glReconSvc)
	gldelivery.RegisterRoutes(v1, glHandler, jwtVerifier, db)

	glWorker := gldelivery.NewGLDeliveryWorker(glDeliverySvc, glReconSvc, glCfg, logger)
	_ = glWorker // registered in Redis block below

	// B1 fix: Register DriftCronHandler on Asynq mux + scheduler.
	// Previously the handler was instantiated then discarded (_ = ...), making the
	// drift cron feature completely dead.  Now we:
	//   1. Create an Asynq ServeMux and register both task types.
	//   2. Create an Asynq Scheduler (19:00 UTC = 02:00 WIB per state-machine §7).
	//   3. Start the scheduler in a goroutine.
	//   4. Start the Asynq server in a goroutine (processes tasks from Redis queue).
	// All of this is skipped gracefully when REDIS_URL is not set (dev mode without Redis).
	// References: DEC-007 (Asynq), worker_tasks.go §TaskDriftCron schedule "0 19 * * *".
	driftCronHandler := eir.NewDriftCronHandler(eirDriftSvc, logger)
	if cfg.RedisURL != "" {
		asynqRedisOpt := asynq.RedisClientOpt{Addr: cfg.RedisURL}

		// Asynq ServeMux — registers task type → handler function.
		asynqMux := asynq.NewServeMux()
		asynqMux.HandleFunc(eir.TaskDriftCron, driftCronHandler.HandleDriftCronTask)
		asynqMux.HandleFunc(eir.TaskDriftAdHoc, driftCronHandler.HandleDriftAdHocTask)
		// M8: calcRunWorker overrides the M7 eclBulkWorker for "ecl:bulk_compute" —
		// same task type, but M8 worker also updates ecl.calc_run lifecycle.
		asynqMux.HandleFunc(eclcore.TaskNameECLBulkCompute, calcRunWorker.Handle)
		_ = eclBulkWorker // replaced by calcRunWorker above
		// M11 Issue #88: async roll-forward for >1000 instruments.
		asynqMux.HandleFunc(rollforward.TaskRollForwardCompute, rfWorker.HandleComputeRollForward)
		// lpsExpiryWorker will be registered here in Phase 5 worker binary.
		// asynqMux.HandleFunc(lps.TaskExpiryCheck, lpsExpiryWorker.HandleExpiryCheck)
		// P5-M1: penempatan deposito maturity check daily cron.
		penMaturityWorker := penempatan.NewMaturityCheckHandler(penSvc, logger)
		asynqMux.HandleFunc(penempatan.MaturityCheckTaskType, penMaturityWorker.ProcessTask)

		// P5-M2: jurnal engine subscribers for penempatan events.
		jurnalWorker.RegisterHandlers(asynqMux)

		// P5-M3: GL Host delivery + reconciliation tasks.
		glWorker.RegisterHandlers(asynqMux)

		// Asynq Server — pulls tasks from Redis queue and dispatches to mux.
		asynqServer := asynq.NewServer(asynqRedisOpt, asynq.Config{
			Concurrency: 5,
		})
		go func() {
			if err := asynqServer.Run(asynqMux); err != nil {
				log.Printf("asynq server stopped: %v", err)
			}
		}()

		// Asynq Scheduler — enqueues periodic tasks (cron expressions, UTC).
		// Location: time.UTC so "0 19 * * *" = 19:00 UTC = 02:00 WIB (state-machine §7).
		scheduler := asynq.NewScheduler(asynqRedisOpt, &asynq.SchedulerOpts{
			Location: time.UTC,
		})
		if _, err := scheduler.Register("0 19 * * *", asynq.NewTask(eir.TaskDriftCron, nil)); err != nil {
			log.Fatalf("register drift cron: %v", err)
		}
		// P5-M1: maturity check 02:00 WIB = 19:00 UTC previous day → use 19:00 UTC.
		// Same slot as drift cron to avoid Redis schedule conflicts; asynq serializes by task type.
		if _, err := scheduler.Register("0 19 * * *", penempatan.NewMaturityCheckTask("TUGURE")); err != nil {
			log.Fatalf("register penempatan maturity cron: %v", err)
		}
		// P5-M3: GL reconciliation daily cron — 01:00 UTC (08:00 WIB).
		glReconTask, glReconTaskErr := gldelivery.NewReconcileDailyTask(time.Now().UTC(), "TUGURE")
		if glReconTaskErr != nil {
			log.Fatalf("build gl recon task: %v", glReconTaskErr)
		}
		if _, err := scheduler.Register("0 1 * * *", glReconTask); err != nil {
			log.Fatalf("register gl recon cron: %v", err)
		}
		go func() {
			if err := scheduler.Run(); err != nil {
				log.Fatalf("asynq scheduler: %v", err)
			}
		}()
		logger.Info("asynq drift cron registered", "schedule", "0 19 * * * UTC", "task", eir.TaskDriftCron)
		logger.Info("asynq penempatan maturity cron registered", "schedule", "0 19 * * * UTC", "task", penempatan.MaturityCheckTaskType)
		logger.Info("asynq GL recon cron registered", "schedule", "0 1 * * * UTC", "task", gldelivery.TaskGLReconcileDaily)
	} else {
		logger.Warn("REDIS_URL not set — Asynq drift cron NOT registered (dev mode)")
	}

	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown pada SIGINT/SIGTERM.
	// stop and cancel are called explicitly on every exit path (no defer) to avoid
	// gocritic exitAfterDefer with log.Fatalf.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("blips-api v%s starting on :%s (env=%s)", version, cfg.ServerPort, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-ctx.Done()
	stop()
	log.Println("shutdown signal received, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)

	if err := srv.Shutdown(shutdownCtx); err != nil {
		cancel()
		log.Fatalf("graceful shutdown failed: %v", err)
	}
	cancel()
	log.Println("server stopped cleanly")
}

// notImplemented adalah placeholder handler untuk endpoints yang belum diimplementasi.
func notImplemented(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": gin.H{
			"code":    "NOT_IMPLEMENTED",
			"message": "Endpoint ini belum diimplementasi.",
			"details": []any{},
		},
	})
}

// splitAndTrim memecah daftar comma-separated dan membuang spasi serta entri kosong.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// readyzDependency merepresentasikan satu dependency yang dicek saat /readyz.
type readyzDependency struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok" atau "error"
	Error  string `json:"error,omitempty"`
}

// makeReadyzHandler membuat Gin handler untuk /readyz yang melakukan real connectivity check
// ke Postgres, Redis, dan MinIO. Deadline tiap probe: 3 detik.
//
// Response 200: semua dependency "ok".
// Response 503: setidaknya satu dependency "error", body berisi detail per-dependency.
//
// Format response konsisten dengan error envelope BLIPS (api-conventions.md):
//
//	200 { "status": "ready",    "dependencies": [...] }
//	503 { "status": "degraded", "dependencies": [...], "error": { "code": "DEPENDENCY_DOWN", ... } }
func makeReadyzHandler(
	db *sql.DB,
	rdb *redis.Client,
	minioClient *document.MinIOClient,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		deps := make([]readyzDependency, 0, 3)
		allOK := true

		// -- Postgres --
		dep := readyzDependency{Name: "postgres"}
		if db == nil {
			dep.Status = "error"
			dep.Error = "database client not initialized"
			allOK = false
		} else if err := db.PingContext(ctx); err != nil {
			dep.Status = "error"
			dep.Error = err.Error()
			allOK = false
		} else {
			dep.Status = "ok"
		}
		deps = append(deps, dep)

		// -- Redis --
		dep = readyzDependency{Name: "redis"}
		if rdb == nil {
			dep.Status = "error"
			dep.Error = "redis client not initialized"
			allOK = false
		} else if err := rdb.Ping(ctx).Err(); err != nil {
			dep.Status = "error"
			dep.Error = err.Error()
			allOK = false
		} else {
			dep.Status = "ok"
		}
		deps = append(deps, dep)

		// -- MinIO --
		dep = readyzDependency{Name: "minio"}
		if minioClient == nil {
			dep.Status = "error"
			dep.Error = "minio client not initialized"
			allOK = false
		} else if err := minioClient.Ping(ctx); err != nil {
			dep.Status = "error"
			dep.Error = err.Error()
			allOK = false
		} else {
			dep.Status = "ok"
		}
		deps = append(deps, dep)

		if allOK {
			c.JSON(http.StatusOK, gin.H{
				"status":       "ready",
				"dependencies": deps,
			})
			return
		}

		// Satu atau lebih dependency down → 503 Unavailable.
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":       "degraded",
			"dependencies": deps,
			"error": gin.H{
				"code":    "DEPENDENCY_DOWN",
				"message": "Satu atau lebih dependency tidak tersedia. Lihat field dependencies untuk detail.",
			},
		})
	}
}

// parseRSAPublicKey mem-parse PEM-encoded RSA public key.
func parseRSAPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("gagal decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("bukan RSA public key")
	}

	return rsaPub, nil
}
