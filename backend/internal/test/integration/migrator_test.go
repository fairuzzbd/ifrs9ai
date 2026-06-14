//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestMigrator_UpDown_Reversibility exercises:
//  1. cmd/migrator up 0001..0007 on an empty DB  → all schemas/tables present
//  2. cmd/migrator down 7 steps                  → DB back to empty
//  3. cmd/migrator up again                      → same result (idempotent round-trip)
//
// Uses the existing dev-stack Postgres (must be running). To avoid polluting
// the dev DB, this test creates an ephemeral database and destroys it after.
//
// Covers: §7 quality gate "cmd/migrator up 0001..0007 from empty DB → down → up".
func TestMigrator_UpDown_Reversibility(t *testing.T) {
	SkipIfShort(t)

	// Connect as admin to create a fresh database.
	baseDSN := envOr("INT_PG_DSN", DefaultPGDSN)

	adminDB, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("open admin db: %v", err)
	}
	defer adminDB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Skipf("postgres not reachable: %v — start dev stack first", err)
	}

	// Create ephemeral test database.
	testDB := fmt.Sprintf("blips_migtest_%d", time.Now().UnixMicro()%100000)
	if _, err := adminDB.ExecContext(context.Background(), "CREATE DATABASE "+testDB); err != nil {
		t.Fatalf("create test db %s: %v", testDB, err)
	}
	t.Cleanup(func() {
		// Drop ephemeral database.
		if _, err := adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+testDB+" WITH (FORCE)"); err != nil {
			t.Logf("WARNING: failed to drop test db %s: %v", testDB, err)
		}
	})

	testDSN := buildTestDSN(baseDSN, testDB)
	migsDir := MigrationsDir()
	t.Logf("migrations dir: %s", migsDir)
	t.Logf("test DSN (db=%s): %s", testDB, maskDSNLog(testDSN))

	// ---- Round 1: UP ----
	t.Run("up_from_empty", func(t *testing.T) {
		runMigratorCmd(t, "up", testDSN, migsDir)
		checkSchemas(t, testDSN)
		checkVersion(t, testDSN, 7)
	})

	// ---- Round 2: DOWN 7 ----
	t.Run("down_7_steps", func(t *testing.T) {
		for i := 0; i < 7; i++ {
			runMigratorCmd(t, "down", testDSN, migsDir, "1")
		}
		checkVersionEmpty(t, testDSN)
	})

	// ---- Round 3: UP again (idempotent round-trip) ----
	t.Run("up_second_time", func(t *testing.T) {
		runMigratorCmd(t, "up", testDSN, migsDir)
		checkSchemas(t, testDSN)
		checkVersion(t, testDSN, 7)
	})
}

// runMigratorCmd invokes cmd/migrator via go run.
func runMigratorCmd(t *testing.T, command, pgDSN, migsDir string, extraArgs ...string) {
	t.Helper()
	args := []string{"run", filepath.Join(repoRoot(), "backend", "cmd", "migrator"), command}
	args = append(args, extraArgs...)

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+pgDSN,
		"MIGRATIONS_DIR="+migsDir,
	)
	out, err := cmd.CombinedOutput()
	t.Logf("migrator %s output: %s", command, string(out))
	if err != nil {
		t.Fatalf("migrator %s failed: %v\nOutput:\n%s", command, err, string(out))
	}
}

// checkSchemas verifies that the 9 expected namespaces + rpt exist.
func checkSchemas(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db for schema check: %v", err)
	}
	defer db.Close()

	expected := []string{"mst", "trx", "ecl", "sppi", "doc", "jrnl", "aud", "sec", "sys"}
	for _, schema := range expected {
		var exists bool
		err := db.QueryRowContext(context.Background(), `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.schemata WHERE schema_name = $1
			)`, schema).Scan(&exists)
		if err != nil {
			t.Errorf("checkSchemas: query %s: %v", schema, err)
			continue
		}
		if !exists {
			t.Errorf("schema %q not found after migration up", schema)
		}
	}

	// Check key tables.
	tables := []string{
		"sec.user", "sec.role", "sec.permission",
		"aud.audit_log",
		"sys.config", "sys.idempotency_key", "sys.job",
		"sys.workflow_instance", "sys.workflow_signature",
		"doc.document",
	}
	for _, tbl := range tables {
		parts := splitTableRef(tbl)
		var exists bool
		err := db.QueryRowContext(context.Background(), `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = $1 AND table_name = $2
			)`, parts[0], parts[1]).Scan(&exists)
		if err != nil {
			t.Errorf("checkTable %s: %v", tbl, err)
			continue
		}
		if !exists {
			t.Errorf("table %q not found after migration up", tbl)
		}
	}
	t.Logf("schema check passed: 9 namespaces + key tables present")
}

// checkVersion queries schema_migrations to confirm the expected version.
func checkVersion(t *testing.T, dsn string, expected uint) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db for version check: %v", err)
	}
	defer db.Close()

	var version uint
	var dirty bool
	err = db.QueryRowContext(context.Background(),
		`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&version, &dirty)
	if err != nil {
		t.Fatalf("checkVersion: query schema_migrations: %v", err)
	}
	if version != expected {
		t.Errorf("expected migration version %d, got %d", expected, version)
	}
	if dirty {
		t.Errorf("schema_migrations.dirty=true — migration left in dirty state")
	}
	t.Logf("migration version: %d dirty=%v", version, dirty)
}

// checkVersionEmpty verifies that schema_migrations has no rows (all rolled back).
func checkVersionEmpty(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db for empty version check: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM schema_migrations`).Scan(&count)
	if err != nil {
		// Table may not exist after full rollback — that is OK.
		t.Logf("schema_migrations not present (expected after full rollback): %v", err)
		return
	}
	if count != 0 {
		t.Errorf("expected 0 rows in schema_migrations after full rollback, got %d", count)
	}
	t.Logf("full rollback: schema_migrations empty — OK")
}

// buildTestDSN replaces the database name in a standard PostgreSQL DSN.
func buildTestDSN(baseDSN, dbName string) string {
	// Replace db name in DSN: postgres://user:pass@host:5432/DBNAME?opts
	if idx := lastSlashBefore(baseDSN, "?"); idx >= 0 {
		return baseDSN[:idx+1] + dbName + baseDSN[idx+1+dbNameLen(baseDSN, idx+1):]
	}
	// Last component after final slash.
	for i := len(baseDSN) - 1; i >= 0; i-- {
		if baseDSN[i] == '/' {
			return baseDSN[:i+1] + dbName
		}
	}
	return baseDSN
}

func lastSlashBefore(s, sep string) int {
	qi := len(s)
	for i, c := range s {
		if string(c) == sep {
			qi = i
			break
		}
	}
	for i := qi - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func dbNameLen(dsn string, start int) int {
	for i := start; i < len(dsn); i++ {
		if dsn[i] == '?' || dsn[i] == '&' {
			return i - start
		}
	}
	return len(dsn) - start
}

func splitTableRef(ref string) []string {
	for i, c := range ref {
		if c == '.' {
			return []string{ref[:i], ref[i+1:]}
		}
	}
	return []string{"public", ref}
}

func maskDSNLog(dsn string) string {
	if len(dsn) < 20 {
		return "***"
	}
	return dsn[:20] + "***"
}
