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

	"blips-ifrs9.tugu-re.com/internal/ecl/staging"
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
	stagingSvc := staging.NewStagingService(
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
