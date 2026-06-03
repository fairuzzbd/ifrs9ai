// Command audit-verify memverifikasi integritas hash chain aud.audit_log.
//
// Usage:
//
//	audit-verify --range "2026-06-01:2026-06-30"
//	audit-verify --range "2026-06-01:2026-06-30" --db "postgres://..."
//	audit-verify --range "2026-06-01:2026-06-30" --json
//
// Exit codes:
//
//	0 = chain valid semua
//	1 = chain rusak atau error
//
// Sesuai PLAN-20260602-phase-2-foundation.md: cmd/audit-verify diperlukan untuk
// verifikasi periodik audit trail (DEC-018, db-conventions.md §"Audit log table").
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/joho/godotenv"

	"blips-ifrs9.tugu-re.com/internal/audit"
)

func main() {
	rangeFlag := flag.String("range", "", `Range tanggal dalam format "YYYY-MM-DD:YYYY-MM-DD"`)
	dbFlag := flag.String("db", "", "PostgreSQL DSN. Default: DATABASE_URL env var")
	jsonFlag := flag.Bool("json", false, "Output dalam format JSON")
	flag.Parse()

	if *rangeFlag == "" {
		fmt.Fprintln(os.Stderr, "Error: --range wajib diisi. Contoh: --range 2026-06-01:2026-06-30")
		flag.Usage()
		os.Exit(1)
	}

	start, end, err := parseRange(*rangeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: format --range tidak valid: %v\n", err)
		os.Exit(1)
	}

	dsn := *dbFlag
	if dsn == "" {
		_ = godotenv.Load()
		dsn = os.Getenv("DATABASE_URL")
		if dsn == "" {
			// No hardcoded credentials (MEDIUM-4/LOW-5 security mandate).
			// production/staging: DATABASE_URL wajib di-set di env (Vault/KMS).
			// development: supply via --db flag atau DATABASE_URL env var.
			appEnv := os.Getenv("APP_ENV")
			if appEnv == "production" || appEnv == "staging" {
				fmt.Fprintln(os.Stderr, "Error: DATABASE_URL wajib di-set di environment production/staging.")
				fmt.Fprintln(os.Stderr, "       Alternatif: gunakan flag --db \"postgres://...\"")
				os.Exit(1)
			}
			// development tanpa DATABASE_URL: exit dengan pesan jelas (tidak ada fallback).
			fmt.Fprintln(os.Stderr, "Error: DATABASE_URL tidak di-set. Gunakan flag --db atau set DATABASE_URL.")
			os.Exit(1)
		}
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: tidak bisa connect ke database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := audit.VerifyHashChain(ctx, db, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: verifikasi gagal: %v\n", err)
		os.Exit(1)
	}

	if *jsonFlag {
		outputJSON(result)
	} else {
		outputText(result)
	}

	if !result.IsValid {
		os.Exit(1)
	}
}

func parseRange(s string) (time.Time, time.Time, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("gunakan format YYYY-MM-DD:YYYY-MM-DD")
	}

	start, err := time.Parse("2006-01-02", strings.TrimSpace(parts[0]))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("tanggal mulai tidak valid: %w", err)
	}

	end, err := time.Parse("2006-01-02", strings.TrimSpace(parts[1]))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("tanggal akhir tidak valid: %w", err)
	}

	// End date adalah hari berikutnya (exclusive).
	end = end.Add(24 * time.Hour)

	if end.Before(start) || end.Equal(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("tanggal akhir harus setelah tanggal mulai")
	}

	return start, end, nil
}

func outputText(result *audit.RangeResult) {
	fmt.Printf("=== Audit Hash Chain Verification ===\n")
	fmt.Printf("Range  : %s - %s\n",
		result.StartDate.Format("2006-01-02"),
		result.EndDate.Add(-24*time.Hour).Format("2006-01-02"))
	fmt.Printf("Events : %d total, %d valid\n", result.TotalEvents, result.ValidEvents)

	if result.IsValid {
		fmt.Printf("Status : VALID — semua hash chain intact\n")
		return
	}

	fmt.Printf("Status : INVALID — %d entity dengan chain rusak\n", len(result.BrokenChains))
	fmt.Printf("\nDetail:\n")
	for i, bc := range result.BrokenChains {
		fmt.Printf("  [%d] entity_type=%s entity_id=%s\n", i+1, bc.EntityType, bc.EntityID)
		if bc.BrokenAt != nil {
			fmt.Printf("       chain rusak pada event_id=%s\n", *bc.BrokenAt)
		}
		fmt.Printf("       %s\n", bc.Message)
	}
}

func outputJSON(result *audit.RangeResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		log.Fatalf("json encode: %v", err)
	}
}
