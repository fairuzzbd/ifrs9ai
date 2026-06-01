// Command api adalah entry point HTTP server BLIPS IFRS9 (Phase 0 bootstrap).
//
// Server hanya menyediakan probe liveness/readiness pada tahap ini; modul bisnis
// (master data, SPPI/BM, ECL/EIR, dst.) ditambahkan pada fase berikutnya di bawah
// prefix /api/v1 sesuai api-conventions.md.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/config"
)

// version adalah versi service yang dilaporkan probe liveness.
const version = "0.1.0"

func main() {
	cfg := config.Load()

	// Mode Gin mengikuti lingkungan: release di luar development untuk menekan log debug.
	if cfg.AppEnv == "production" || cfg.AppEnv == "staging" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// CORS hanya mengizinkan origin yang dikonfigurasi (CORS_ALLOWED_ORIGINS).
	corsCfg := cors.Config{
		AllowOrigins:     splitAndTrim(cfg.CORSAllowedOrigins),
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Idempotency-Key", "X-Trace-Id"},
		ExposeHeaders:    []string{"X-Trace-Id"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
	router.Use(cors.New(corsCfg))

	// GET /healthz — liveness probe.
	// CATATAN: probe ini SENGAJA berada di luar konvensi success-envelope /api/v1
	// (lihat api-conventions.md). Orchestrator (k8s/docker) mengharapkan payload datar
	// yang stabil, bukan { "data": ... }.
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"service":   "blips-api",
			"version":   version,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// GET /readyz — readiness probe (placeholder Phase 0).
	// Nanti akan memeriksa dependensi nyata (PostgreSQL, Redis, MinIO) sebelum melapor ready.
	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

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
