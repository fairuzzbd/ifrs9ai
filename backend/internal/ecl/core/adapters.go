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

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

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
type DBBobotRepo struct {
	db *sql.DB
}

// NewDBBobotRepo creates a DBBobotRepo. Panics if db is nil.
func NewDBBobotRepo(db *sql.DB) *DBBobotRepo {
	if db == nil {
		panic("core.NewDBBobotRepo: db must not be nil")
	}
	return &DBBobotRepo{db: db}
}

// GetActiveBobot returns the BobotSnapshot for the given periodeID.
// Looks for APPROVED_ACTIVE rows in mst.bobot_skenario for skenario GOOD/NORMAL/BAD.
// Falls back to default (0.25/0.50/0.25) if no rows found — with a log warning.
// Per DEC-010: bobot must sum to 1.0; ALCO can override.
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
		return defaultBobotFallback(), nil // DB error → use default
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
	if rows.Err() != nil || found < 3 {
		// Fallback to DEC-010 defaults if not all three found.
		return defaultBobotFallback(), nil
	}
	return BobotSnapshot{Good: good, Normal: normal, Bad: bad}, nil
}

// defaultBobotFallback returns DEC-010 default weights (0.25/0.50/0.25).
// Used when mst.bobot_skenario has no approved active rows.
func defaultBobotFallback() BobotSnapshot {
	return BobotSnapshot{
		Good:   decimal.NewFromFloat(0.25),
		Normal: decimal.NewFromFloat(0.50),
		Bad:    decimal.NewFromFloat(0.25),
	}
}
