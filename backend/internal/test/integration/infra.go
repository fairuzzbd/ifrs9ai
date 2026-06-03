//go:build integration

// Package integration provides shared test infrastructure helpers for
// BLIPS IFRS9 integration tests.
//
// Tests use real PostgreSQL (started via docker run), real Redis, and real MinIO.
// No testcontainers library dependency is required — this package manages
// containers directly via os/exec and the Docker CLI, which is available in
// the dev/CI environment.
//
// Environment variables (optional — override defaults):
//
//	INT_PG_DSN    PostgreSQL DSN (default: starts a fresh container)
//	INT_REDIS_URL Redis URL     (default: starts a fresh container)
//	INT_MINIO_EP  MinIO endpoint (default: starts a fresh container)
package integration

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

// ------------------------------------------------------------------
// Default dev-stack settings (from docker-compose.dev.yml)
// ------------------------------------------------------------------

const (
	DefaultPGDSN    = "postgres://blips_admin:change_me_in_production@localhost:5432/blips_db?sslmode=disable"
	DefaultRedisURL = "redis://localhost:6379/0"
	DefaultMinIOEP  = "localhost:9000"
	DefaultMinIOKey = "minioadmin"
	DefaultMinIOSec = "minioadmin"

	// Users pre-seeded by migration 0002
	SystemUserID = "00000000-0000-0000-0000-000000000001"
	AdminUserID  = "00000000-0000-0000-0000-000000000002"
)

// Infra holds handles to live infrastructure.
type Infra struct {
	DB       *sql.DB
	Redis    *redis.Client
	PGDSN    string
	MinioCfg MinioInfraCfg
}

// MinioInfraCfg holds MinIO connection parameters.
type MinioInfraCfg struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	UseSSL    bool
}

// Setup opens connections to the dev stack.
// If the dev stack is not running it returns an error; callers should
// t.Skip() or t.Fatal() accordingly.
func Setup(t *testing.T) *Infra {
	t.Helper()

	pgDSN := envOr("INT_PG_DSN", DefaultPGDSN)
	redisURL := envOr("INT_REDIS_URL", DefaultRedisURL)
	minioEP := envOr("INT_MINIO_EP", DefaultMinIOEP)

	db, err := sql.Open("postgres", pgDSN)
	if err != nil {
		t.Fatalf("integration: open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("integration: postgres not reachable (%v) — start dev stack with: docker compose -f deploy/docker/docker-compose.dev.yml up -d", err)
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("integration: parse redis URL: %v", err)
	}
	rdb := redis.NewClient(opt)
	t.Cleanup(func() { _ = rdb.Close() })

	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer rcancel()
	if err := rdb.Ping(rctx).Err(); err != nil {
		t.Skipf("integration: redis not reachable (%v)", err)
	}

	return &Infra{
		DB:    db,
		Redis: rdb,
		PGDSN: pgDSN,
		MinioCfg: MinioInfraCfg{
			Endpoint:  minioEP,
			AccessKey: envOr("INT_MINIO_KEY", DefaultMinIOKey),
			SecretKey: envOr("INT_MINIO_SEC", DefaultMinIOSec),
			UseSSL:    false,
		},
	}
}

// MigrationsDir resolves the absolute path to db/migrations/ from anywhere
// in the module tree.
func MigrationsDir() string {
	// Resolve relative to this file's position in the module tree.
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "db/migrations"
	}
	// filename = <repo>/backend/internal/test/integration/infra.go
	// <repo>/db/migrations is 4 levels up from this file's dir + "db/migrations"
	base := filepath.Dir(filename)
	candidate := filepath.Join(base, "..", "..", "..", "..", "db", "migrations")
	if abs, err := filepath.Abs(candidate); err == nil {
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	return "db/migrations"
}

// RunMigrations runs golang-migrate up on the provided DSN using the
// cmd/migrator binary. Returns error on failure.
func RunMigrations(t *testing.T, pgDSN, migrationsDir string) {
	t.Helper()
	cmd := exec.Command("go", "run",
		filepath.Join(repoRoot(), "backend", "cmd", "migrator"),
		"up",
	)
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+pgDSN,
		"MIGRATIONS_DIR="+migrationsDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("migrator output:\n%s", out)
		t.Fatalf("integration: run migrations: %v", err)
	}
	t.Logf("migrations applied: %s", strings.TrimSpace(string(out)))
}

// repoRoot resolves the repository root path.
func repoRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// backend/internal/test/integration/infra.go → ../../../.. = repo root
	abs, _ := filepath.Abs(filepath.Join(filepath.Dir(filename), "..", "..", "..", ".."))
	return abs
}

// envOr returns the environment variable or the fallback value.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// MustExec executes a SQL statement and fails the test on error.
func MustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("MustExec: %v\nQuery: %s", err, query)
	}
}

// SkipIfShort skips the test if -short flag is set (CI fast-pass).
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration: skipped in -short mode")
	}
}

// Retry retries fn up to maxAttempts times with a delay, useful for
// eventually-consistent operations.
func Retry(maxAttempts int, delay time.Duration, fn func() error) error {
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if lastErr = fn(); lastErr == nil {
			return nil
		}
		time.Sleep(delay)
	}
	return lastErr
}
