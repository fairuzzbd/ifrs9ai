package core

// adapters.go — DB-backed implementations of BobotRepo and InstrumenReaderIface
// for wiring ECLOrchestrator in main.go.
//
// These adapters bridge mst.instrumen and mst.bobot_skenario into the
// minimal interfaces that M7 ECLOrchestrator requires.
//
// References: FSD-APP-C §3, SoW §4, DEC-010, DEC-016.

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// WarnBobotDefaultFallbackUsed is the warning code emitted when bobot falls back to defaults.
// F6 fix: explicit fallback only via AllowBobotDefaultFallback flag — not on DB error.
const WarnBobotDefaultFallbackUsed = "ECL_BOBOT_DEFAULT_FALLBACK_USED"

// ─── DBInstrumenReader ────────────────────────────────────────────────────────

// DBInstrumenReader implements InstrumenReaderIface by querying mst.instrumen.
type DBInstrumenReader struct {
	db *sql.DB
}

// NewDBInstrumenReader creates a DBInstrumenReader. Panics if db is nil.
func NewDBInstrumenReader(db *sql.DB) *DBInstrumenReader {
	if db == nil {
		panic("core.NewDBInstrumenReader: db must not be nil")
	}
	return &DBInstrumenReader{db: db}
}

// GetByID reads a minimal InstrumenSnapshot by primary key.
// Returns CodeECLInstrumenNotFound if not found.
func (r *DBInstrumenReader) GetByID(ctx context.Context, id uuid.UUID) (*InstrumenSnapshot, error) {
	q := `
SELECT id, klasifikasi_psak71, tipe_instrumen, status, workflow_status,
       flag_poci, counterparty_id, nasabah_id, portofolio_id, tenant_id
FROM mst.instrumen
WHERE id = $1 AND deleted_at IS NULL`

	var snap InstrumenSnapshot
	var counterpartyID, nasabahID uuid.UUID
	var portofolioID *uuid.UUID

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&snap.ID,
		&snap.KlasifikasiPsak71,
		&snap.TipeInstrumen,
		&snap.Status,
		&snap.WorkflowStatus,
		&snap.FlagPOCI,
		&counterpartyID,
		&nasabahID,
		&portofolioID,
		&snap.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, errDomain(CodeECLInstrumenNotFound, fmt.Sprintf("instrumen %s not found", id))
	}
	if err != nil {
		return nil, err
	}
	snap.CounterpartyID = counterpartyID
	snap.NasabahID = nasabahID
	snap.PortofolioID = portofolioID
	return &snap, nil
}

// ListActiveByScope returns active (APPROVED, AKTIF, not deleted) instruments
// filtered by optional portofolio_id or instrument_id lists.
// Per SoW §4.2 and DEC-022: cursor not used here (bulk returns all in scope).
func (r *DBInstrumenReader) ListActiveByScope(ctx context.Context, scope *BulkScope) ([]InstrumenSnapshot, error) {
	// Base: all approved active instruments.
	q := `
SELECT id, klasifikasi_psak71, tipe_instrumen, status, workflow_status,
       flag_poci, counterparty_id, nasabah_id, portofolio_id, tenant_id
FROM mst.instrumen
WHERE deleted_at IS NULL
  AND workflow_status = 'APPROVED'
  AND status = 'AKTIF'`

	var args []interface{}
	argIdx := 1

	if scope != nil && len(scope.PortofolioIDs) > 0 {
		placeholders := make([]string, len(scope.PortofolioIDs))
		for i, pid := range scope.PortofolioIDs {
			args = append(args, pid)
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			argIdx++
		}
		q += fmt.Sprintf(" AND portofolio_id IN (%s)", joinStrings(placeholders, ","))
	}

	if scope != nil && len(scope.InstrumenIDs) > 0 {
		placeholders := make([]string, len(scope.InstrumenIDs))
		for i, iid := range scope.InstrumenIDs {
			args = append(args, iid)
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			argIdx++
		}
		q += fmt.Sprintf(" AND id IN (%s)", joinStrings(placeholders, ","))
	}

	q += " ORDER BY id"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InstrumenSnapshot
	for rows.Next() {
		var snap InstrumenSnapshot
		var counterpartyID, nasabahID uuid.UUID
		var portofolioID *uuid.UUID
		if err := rows.Scan(
			&snap.ID,
			&snap.KlasifikasiPsak71,
			&snap.TipeInstrumen,
			&snap.Status,
			&snap.WorkflowStatus,
			&snap.FlagPOCI,
			&counterpartyID,
			&nasabahID,
			&portofolioID,
			&snap.TenantID,
		); err != nil {
			return nil, err
		}
		snap.CounterpartyID = counterpartyID
		snap.NasabahID = nasabahID
		snap.PortofolioID = portofolioID
		result = append(result, snap)
	}
	return result, rows.Err()
}

// joinStrings joins a slice of strings with a separator.
// Used for SQL IN-clause placeholder building.
func joinStrings(ss []string, sep string) string { //nolint:unparam // sep is a documented param; future callers may use different separators
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += sep
		}
		out += s
	}
	return out
}

// ─── DBBobotRepo ─────────────────────────────────────────────────────────────

// DBBobotRepo implements BobotRepo by querying mst.bobot_skenario.
// It reads the three approved active rows (GOOD/NORMAL/BAD) for a given periodeID.
//
// F6 fix: DB errors and empty rows now return an error (not silent fallback).
// AllowDefaultFallback flag enables explicit fallback for seeded/test environments only,
// with a log warning and the WarnBobotDefaultFallbackUsed code to callers.
type DBBobotRepo struct {
	db                   *sql.DB
	AllowDefaultFallback bool // if true, returns default bobot when 0 rows found (with warning)
	logger               *slog.Logger
}

// NewDBBobotRepo creates a DBBobotRepo. Panics if db is nil.
func NewDBBobotRepo(db *sql.DB) *DBBobotRepo {
	if db == nil {
		panic("core.NewDBBobotRepo: db must not be nil")
	}
	return &DBBobotRepo{db: db, logger: slog.Default()}
}

// GetActiveBobot returns the BobotSnapshot for the given periodeID.
// Looks for APPROVED_ACTIVE rows in mst.bobot_skenario for skenario GOOD/NORMAL/BAD.
//
// F6 fix: returns error if DB unreachable or if 0 APPROVED_ACTIVE rows found.
// AllowDefaultFallback=true uses defaults with a warning (explicit seed/test path only).
// Per DEC-010: bobot must be ALCO-approved; silent fallback hides missing ALCO approval.
func (r *DBBobotRepo) GetActiveBobot(ctx context.Context, periodeID string) (BobotSnapshot, error) {
	q := `
SELECT skenario, bobot
FROM mst.bobot_skenario
WHERE periode_berlaku_dari <= $1
  AND (periode_berlaku_sampai IS NULL OR periode_berlaku_sampai >= $1)
  AND workflow_status = 'APPROVED_ACTIVE'
  AND deleted_at IS NULL
ORDER BY skenario`

	rows, err := r.db.QueryContext(ctx, q, periodeID)
	if err != nil {
		// F6 fix: DB error is now propagated — callers should not proceed without bobot.
		return BobotSnapshot{}, fmt.Errorf("core.GetActiveBobot: query bobot_skenario: %w", err)
	}
	defer rows.Close()

	var good, normal, bad decimal.Decimal
	found := 0
	for rows.Next() {
		var skenario string
		var bobot decimal.Decimal
		if err := rows.Scan(&skenario, &bobot); err != nil {
			continue
		}
		switch skenario {
		case "GOOD":
			good = bobot
			found++
		case "NORMAL":
			normal = bobot
			found++
		case "BAD":
			bad = bobot
			found++
		}
	}
	if err := rows.Err(); err != nil {
		return BobotSnapshot{}, fmt.Errorf("core.GetActiveBobot: scan bobot_skenario rows: %w", err)
	}

	if found < 3 {
		if r.AllowDefaultFallback {
			// Explicit fallback path: seed/test environment only.
			// Log warning so operator knows ALCO bobot is missing.
			logger := r.logger
			if logger == nil {
				logger = slog.Default()
			}
			logger.WarnContext(ctx, "core.GetActiveBobot: no APPROVED_ACTIVE bobot found; using DEC-010 defaults",
				"periode_id", periodeID,
				"found_rows", found,
				"warning_code", WarnBobotDefaultFallbackUsed,
			)
			return defaultBobotFallback(), nil
		}
		// F6 fix: error when 0 approved rows and fallback not explicitly allowed.
		return BobotSnapshot{}, fmt.Errorf("%w: no APPROVED_ACTIVE bobot_skenario rows for periodeID %q (found %d/3)",
			errDomain(CodeECLParamNotFound, "bobot_skenario not found"), periodeID, found)
	}
	return BobotSnapshot{Good: good, Normal: normal, Bad: bad}, nil
}

// defaultBobotFallback returns DEC-010 default weights (0.25/0.50/0.25).
// Used only when AllowDefaultFallback=true (seed/test environments).
func defaultBobotFallback() BobotSnapshot {
	return BobotSnapshot{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
}
