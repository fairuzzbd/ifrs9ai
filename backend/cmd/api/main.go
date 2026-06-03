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
	"github.com/redis/go-redis/v9"
	_ "github.com/lib/pq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/config"
	"blips-ifrs9.tugu-re.com/internal/document"
	"blips-ifrs9.tugu-re.com/internal/notification"
	"blips-ifrs9.tugu-re.com/internal/workflow"
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

	// JWT Verifier (optional; dipakai di auth middleware).
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
	var asynqClient interface{ EnqueueContext(ctx interface{}, task interface{}, opts ...interface{}) (interface{}, error) }
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

	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown pada SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("blips-api v%s starting on :%s (env=%s)", version, cfg.ServerPort, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown signal received, draining connections...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
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
