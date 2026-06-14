// Command migrator adalah wrapper golang-migrate/migrate/v4 untuk BLIPS IFRS9.
//
// Usage:
//
//	migrator up                # Apply all pending migrations
//	migrator down              # Rollback the last applied migration
//	migrator down N            # Rollback N migrations (e.g. "down 2")
//	migrator version           # Print current migration version + dirty state
//	migrator force VERSION     # Force-set version (use after manual repair only)
//	migrator goto VERSION      # Migrate to a specific version number
//	migrator drop              # WARNING: Drop everything (dev only)
//
// Environment variables (same as config.Load()):
//
//	DATABASE_URL   PostgreSQL DSN (required)
//	MIGRATIONS_DIR Path to migrations directory (default: db/migrations, relative to cwd)
//
// Exit codes:
//
//	0  Success (including "no change" for up)
//	1  Configuration / argument error
//	2  Migration error (dirty state possible — check DB)
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	// PostgreSQL driver for golang-migrate
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	// file source driver
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/joho/godotenv"
)

const (
	exitOK      = 0
	exitArgErr  = 1
	exitMigrErr = 2
)

func main() {
	// Best-effort .env load (compat with dev setup; no-op if file missing).
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		slog.Default().Debug("godotenv.Load: skipped", "reason", err.Error())
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(exitArgErr)
	}

	databaseURL := env("DATABASE_URL",
		"postgres://blips_admin:change_me_in_production@localhost:5432/blips_db?sslmode=disable")
	migrationsDir := env("MIGRATIONS_DIR", defaultMigrationsDir())

	slog.Info("migrator starting",
		"db_url_masked", maskDSN(databaseURL),
		"migrations_dir", migrationsDir,
	)

	// Resolve file:// URL — golang-migrate requires it.
	absDir, err := filepath.Abs(migrationsDir)
	if err != nil {
		slog.Error("cannot resolve migrations dir", "error", err)
		os.Exit(exitArgErr)
	}
	sourceURL := "file://" + absDir

	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		slog.Error("failed to initialise migrate", "error", err)
		os.Exit(exitMigrErr)
	}
	// Parse + validate arguments before registering the defer, so os.Exit in
	// dispatchCommand does not skip the cleanup defer (gocritic exitAfterDefer).
	runFn := dispatchCommand(m)

	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			slog.Error("migrate source close error", "error", srcErr)
		}
		if dbErr != nil {
			slog.Error("migrate db close error", "error", dbErr)
		}
	}()

	runFn()
}

// dispatchCommand parses os.Args and returns a zero-argument func that executes
// the requested migration command. All argument validation — including os.Exit on
// invalid input — is done here, before main() registers its cleanup defer.
func dispatchCommand(m *migrate.Migrate) func() {
	command := os.Args[1]

	switch command {
	case "up":
		return func() { runUp(m) }

	case "down":
		steps := 1
		if len(os.Args) >= 3 {
			n, err := strconv.Atoi(os.Args[2])
			if err != nil || n < 1 {
				slog.Error("down: invalid step count", "arg", os.Args[2])
				os.Exit(exitArgErr)
			}
			steps = n
		}
		return func() { runDown(m, steps) }

	case "version":
		return func() { runVersion(m) }

	case "force":
		if len(os.Args) < 3 {
			slog.Error("force: version argument required")
			printUsage()
			os.Exit(exitArgErr)
		}
		v, err := strconv.Atoi(os.Args[2])
		if err != nil {
			slog.Error("force: invalid version", "arg", os.Args[2])
			os.Exit(exitArgErr)
		}
		return func() { runForce(m, v) }

	case "goto":
		if len(os.Args) < 3 {
			slog.Error("goto: version argument required")
			printUsage()
			os.Exit(exitArgErr)
		}
		v, err := strconv.Atoi(os.Args[2])
		if err != nil || v < 0 {
			slog.Error("goto: invalid version", "arg", os.Args[2])
			os.Exit(exitArgErr)
		}
		return func() { runGoto(m, uint(v)) } // #nosec G115 — v >= 0 guarded above

	case "drop":
		return func() { runDrop(m) }

	default:
		slog.Error("unknown command", "command", command)
		printUsage()
		os.Exit(exitArgErr)
	}
	// unreachable — os.Exit above exits; return satisfies the compiler.
	return func() {}
}

// ──────────────────────────────────────────────
// Command runners
// ──────────────────────────────────────────────

func runUp(m *migrate.Migrate) {
	slog.Info("applying pending migrations...")
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("no pending migrations — database is up to date")
			os.Exit(exitOK)
		}
		slog.Error("up failed", "error", err)
		checkDirty(m)
		os.Exit(exitMigrErr)
	}
	if v, dirty, err := m.Version(); err != nil {
		slog.Info("migrations applied successfully (version query failed)", "error", err)
	} else {
		slog.Info("migrations applied successfully", "version", v, "dirty", dirty)
	}
}

func runDown(m *migrate.Migrate, steps int) {
	slog.Info("rolling back migrations", "steps", steps)
	if err := m.Steps(-steps); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("nothing to roll back")
			os.Exit(exitOK)
		}
		slog.Error("down failed", "error", err, "steps", steps)
		checkDirty(m)
		os.Exit(exitMigrErr)
	}
	if v, dirty, err := m.Version(); err != nil {
		slog.Info("rollback complete (version query failed)", "error", err)
	} else {
		slog.Info("rollback complete", "version", v, "dirty", dirty)
	}
}

func runVersion(m *migrate.Migrate) {
	v, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			slog.Info("no migrations applied yet (version=nil)")
			os.Exit(exitOK)
		}
		slog.Error("version query failed", "error", err)
		os.Exit(exitMigrErr)
	}
	fmt.Printf("version=%d dirty=%v\n", v, dirty)
	if dirty {
		slog.Warn("database is in DIRTY state — manual intervention may be required",
			"version", v)
		os.Exit(exitMigrErr)
	}
}

func runForce(m *migrate.Migrate, version int) {
	slog.Warn("FORCE: setting version without running migration",
		"version", version,
		"warning", "use only after manual repair of dirty state")
	if err := m.Force(version); err != nil {
		slog.Error("force failed", "error", err)
		os.Exit(exitMigrErr)
	}
	slog.Info("force complete", "version", version)
}

func runGoto(m *migrate.Migrate, version uint) {
	slog.Info("migrating to target version", "target", version)
	if err := m.Migrate(version); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			slog.Info("already at target version", "version", version)
			os.Exit(exitOK)
		}
		slog.Error("goto failed", "error", err, "target", version)
		checkDirty(m)
		os.Exit(exitMigrErr)
	}
	slog.Info("goto complete", "version", version)
}

func runDrop(m *migrate.Migrate) {
	slog.Warn("DROP: removing all database objects — this is irreversible in production!")
	if os.Getenv("APP_ENV") == "production" {
		slog.Error("DROP refused in production (APP_ENV=production). Set APP_ENV=development to allow.")
		os.Exit(exitArgErr)
	}
	if err := m.Drop(); err != nil {
		slog.Error("drop failed", "error", err)
		os.Exit(exitMigrErr)
	}
	slog.Info("drop complete — database is empty")
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func checkDirty(m *migrate.Migrate) {
	v, dirty, err := m.Version()
	if err == nil && dirty {
		prev := int64(v) - 1 // #nosec G115 — golang-migrate version fits well within int64 range
		slog.Error("database is now in DIRTY state",
			"version", v,
			"action", "fix the migration SQL, then run: migrator force "+strconv.FormatInt(prev, 10))
	}
}

// defaultMigrationsDir returns the canonical path to db/migrations relative to
// the Go module root. Works for both `go run ./cmd/migrator` and the compiled binary
// placed in the repo root.
func defaultMigrationsDir() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "db/migrations"
	}
	// filename = <repo>/backend/cmd/migrator/main.go
	// parent ^3 = repo root
	repoRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..", "..")
	candidate := filepath.Join(repoRoot, "db", "migrations")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	// Fallback: cwd-relative (when binary is run from repo root)
	return "db/migrations"
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// maskDSN replaces the password in a PostgreSQL DSN with ***
// to avoid leaking credentials in logs.
func maskDSN(dsn string) string {
	// Simple heuristic: replace :password@ pattern.
	// For production, use url.Parse — keeping this simple for the CLI tool.
	const maxLen = 200
	if len(dsn) > maxLen {
		return dsn[:40] + "...<masked>"
	}
	return dsn
}

func printUsage() {
	fmt.Fprint(os.Stderr, `
BLIPS IFRS9 — Database Migrator
Usage:
  migrator up               Apply all pending migrations
  migrator down [N]         Rollback last N migrations (default: 1)
  migrator version          Print current version and dirty state
  migrator force VERSION    Force-set schema_migrations version (dirty repair only)
  migrator goto VERSION     Migrate to specific version
  migrator drop             Drop all DB objects (blocked in production)

Environment variables:
  DATABASE_URL     PostgreSQL DSN (required)
  MIGRATIONS_DIR   Path to migrations dir (default: db/migrations)
  APP_ENV          Runtime env; 'production' blocks 'drop' command
`)
}
