// Package staging — database adapters for InstrumenReader and PeriodeBukuReader.
//
// These adapters query mst.instrumen, mst.rating_history_counterparty, and
// mst.periode_buku directly via *sql.DB to avoid circular imports with the
// master/instrumen and master/periodebuku packages.
//
// In production wiring (cmd/api/main.go), create these adapters with NewDBInstrumenReader
// and NewDBPeriodeBukuReader passing the shared *sql.DB.
package staging

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ─── DBInstrumenReader ────────────────────────────────────────────────────────

// DBInstrumenReader implements InstrumenReader using direct SQL queries.
type DBInstrumenReader struct {
	db *sql.DB
}

// NewDBInstrumenReader creates a DBInstrumenReader.
func NewDBInstrumenReader(db *sql.DB) *DBInstrumenReader {
	return &DBInstrumenReader{db: db}
}

// GetByID fetches a minimal InstrumenSnapshot from mst.instrumen.
func (r *DBInstrumenReader) GetByID(ctx context.Context, id uuid.UUID) (*InstrumenSnapshot, error) {
	if r.db == nil {
		return nil, ErrNotFound
	}
	// M3 fix: include is_poci so staging engine can gate POCI instruments.
	q := `
		SELECT i.id, i.klasifikasi_psak71, i.status, i.tanggal_penempatan, i.tenant_id,
		       COALESCE(i.is_poci, FALSE)
		FROM mst.instrumen i
		WHERE i.id = $1 AND i.deleted_at IS NULL
		LIMIT 1`
	var snap InstrumenSnapshot
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&snap.ID, &snap.KlasifikasiPSAK71, &snap.Status, &snap.TanggalPenempatan, &snap.TenantID,
		&snap.IsPOCI,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("staging DBInstrumenReader.GetByID: %w", err)
	}
	return &snap, nil
}

// GetRatingAtDate fetches the Pefindo rating for a counterparty linked to the instrument.
// Returns ("", nil) if no rating found for the given date.
func (r *DBInstrumenReader) GetRatingAtDate(ctx context.Context, instrumenID uuid.UUID, asOf time.Time) (string, error) {
	if r.db == nil {
		return "", nil
	}
	// Query: get counterparty_id from instrumen, then find the rating effective on asOf.
	q := `
		SELECT rh.grade
		FROM mst.rating_history_counterparty rh
		JOIN mst.instrumen i ON i.counterparty_id = rh.counterparty_id
		WHERE i.id = $1
		  AND rh.tanggal_efektif <= $2
		  AND rh.workflow_status = 'APPROVED'
		  AND rh.deleted_at IS NULL
		ORDER BY rh.tanggal_efektif DESC
		LIMIT 1`
	var grade string
	err := r.db.QueryRowContext(ctx, q, instrumenID, asOf).Scan(&grade)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("staging DBInstrumenReader.GetRatingAtDate: %w", err)
	}
	return grade, nil
}

// GetOriginationDate returns the tanggal_penempatan for an instrument.
func (r *DBInstrumenReader) GetOriginationDate(ctx context.Context, instrumenID uuid.UUID) (time.Time, error) {
	if r.db == nil {
		return time.Time{}, ErrNotFound
	}
	var t time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT tanggal_penempatan FROM mst.instrumen WHERE id = $1 AND deleted_at IS NULL`,
		instrumenID,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("staging DBInstrumenReader.GetOriginationDate: %w", err)
	}
	return t, nil
}

// ─── DBPeriodeBukuReader ──────────────────────────────────────────────────────

// DBPeriodeBukuReader implements PeriodeBukuReader using direct SQL queries.
type DBPeriodeBukuReader struct {
	db *sql.DB
}

// NewDBPeriodeBukuReader creates a DBPeriodeBukuReader.
func NewDBPeriodeBukuReader(db *sql.DB) *DBPeriodeBukuReader {
	return &DBPeriodeBukuReader{db: db}
}

// ListClosedBulananSince returns HARD_CLOSED BULANAN periode_buku with tanggal_mulai >= from,
// ordered ascending by tanggal_mulai.
//
// Per DEC-012 + FSD-APP-C §3.3: cure must count only HARD_CLOSED periods.
// SOFT_CLOSED periods are re-openable and therefore not final — they must NOT
// count toward the 3-consecutive-period cure criterion.
func (r *DBPeriodeBukuReader) ListClosedBulananSince(ctx context.Context, from time.Time, tenantID string) ([]time.Time, error) {
	if r.db == nil {
		return nil, nil
	}
	q := `
		SELECT tanggal_mulai
		FROM mst.periode_buku
		WHERE tipe_periode = 'BULANAN'
		  AND status = 'HARD_CLOSED'
		  AND tanggal_mulai >= $1
		  AND tenant_id = $2
		  AND deleted_at IS NULL
		ORDER BY tanggal_mulai ASC`
	rows, err := r.db.QueryContext(ctx, q, from, tenantID)
	if err != nil {
		return nil, fmt.Errorf("staging DBPeriodeBukuReader.ListClosedBulananSince: %w", err)
	}
	defer rows.Close()

	var periods []time.Time
	for rows.Next() {
		var t time.Time
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("staging DBPeriodeBukuReader.ListClosedBulananSince scan: %w", err)
		}
		periods = append(periods, t)
	}
	return periods, rows.Err()
}

// GetTanggalAkhirByID returns mst.periode_buku.tanggal_akhir for the given periode ID.
// Returns ErrNotFound if no matching (non-deleted) periode_buku row exists.
//
// Used by SubmitOverride to set periodeAkhir from the real periode_buku record
// rather than a hardcoded 1-year offset (F4 fix per migration 000022 §Section 2).
func (r *DBPeriodeBukuReader) GetTanggalAkhirByID(ctx context.Context, periodeID uuid.UUID) (time.Time, error) {
	if r.db == nil {
		return time.Time{}, ErrNotFound
	}
	var t time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT tanggal_akhir FROM mst.periode_buku WHERE id = $1 AND deleted_at IS NULL`,
		periodeID,
	).Scan(&t)
	if err == sql.ErrNoRows {
		return time.Time{}, ErrNotFound
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("staging DBPeriodeBukuReader.GetTanggalAkhirByID: %w", err)
	}
	return t, nil
}
