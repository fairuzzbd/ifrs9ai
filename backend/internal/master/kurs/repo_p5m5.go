package kurs

// repo_p5m5.go — P5-M5 new repository methods extending DBRepository.
//
// New interface methods added here:
//   - InsertBatch             — batch INSERT for manual upload rows
//   - GetBatchByID            — list all kurs rows for an upload_batch_id
//   - SetBatchApproved        — UPDATE workflow_status → APPROVED for batch
//   - SetBatchRejected        — UPDATE workflow_status → REJECTED + reject_reason for batch
//   - GetPreviousActiveRate   — SELECT latest APPROVED rate for (kode, date < target)
//   - IsHoliday               — SELECT EXISTS from sys.holiday_calendar
//   - GetConfigParam          — SELECT config_value from sys.config
//   - GetInstrumenForTreatment — SELECT klasifikasi + mata_uang for an instrumen UUID
//   - InsertDLQEntry          — INSERT into sys.dlq_fx_jisdor on fetch failure
//   - LockRatesForPeriode     — UPDATE mst.kurs locked_flag=TRUE for periode (P5-M4 hook)
//   - UnlockRatesForPeriode   — UPDATE mst.kurs locked_flag=FALSE for periode (reopen hook)

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Extended Repository interface (P5-M5) ───────────────────────────────────

// RepositoryP5M5 extends Repository with P5-M5-specific operations.
// DBRepository embeds both; callers that need P5-M5 methods type-assert to this.
type RepositoryP5M5 interface {
	Repository

	InsertBatch(ctx context.Context, tx *sql.Tx, rows []*Kurs) error
	GetBatchByID(ctx context.Context, batchID uuid.UUID) ([]*Kurs, error)
	SetBatchApproved(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, approverID uuid.UUID) (int64, error)
	SetBatchRejected(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, reason string, rejectorID uuid.UUID) (int64, error)
	GetPreviousActiveRate(ctx context.Context, kode string, before time.Time) (*decimal.Decimal, error)
	IsHoliday(ctx context.Context, tanggal time.Time) (bool, error)
	GetConfigParam(ctx context.Context, key string) (string, error)
	GetInstrumenForTreatment(ctx context.Context, instrumenID uuid.UUID) (klasifikasi, mataUang string, err error)
	InsertDLQEntry(ctx context.Context, tanggal time.Time, kode, errCode, errMsg string, payloadJSON []byte) error
	LockRatesForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) error
	UnlockRatesForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) error
}

// Ensure DBRepository satisfies the extended interface.
var _ RepositoryP5M5 = (*DBRepository)(nil)

// ─── InsertBatch ──────────────────────────────────────────────────────────────

// InsertBatch inserts multiple kurs rows atomically in the given transaction.
// Each row must have ID, FxRateIDKode, UploadBatchID already set by the caller.
// Duplicate date errors return ErrDuplicateDate on the first violation.
func (r *DBRepository) InsertBatch(ctx context.Context, tx *sql.Tx, rows []*Kurs) error {
	for _, k := range rows {
		if err := r.Create(ctx, tx, k); err != nil {
			return fmt.Errorf("repo.InsertBatch kurs row %s %s: %w",
				k.KodeMataUang, k.TanggalBerlaku.Format("2006-01-02"), err)
		}
	}
	return nil
}

// ─── GetBatchByID ─────────────────────────────────────────────────────────────

// GetBatchByID returns all kurs rows linked to an upload_batch_id.
// Returns empty slice (not error) if batch not found.
func (r *DBRepository) GetBatchByID(ctx context.Context, batchID uuid.UUID) ([]*Kurs, error) {
	// Note: upload_batch_id column added by migration 000039.
	// baseSelectCols from repo.go does not include new P5-M5 cols; we fetch only base cols here.
	query := fmt.Sprintf("SELECT%s FROM mst.kurs k WHERE k.upload_batch_id = $1 AND k.deleted_at IS NULL ORDER BY k.kode_mata_uang ASC, k.tanggal_berlaku ASC", baseSelectCols) //nolint:gosec
	rows, err := r.db.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, fmt.Errorf("repo.GetBatchByID kurs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Kurs
	for rows.Next() {
		k, err := scanKursRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.GetBatchByID kurs scan: %w", err)
		}
		items = append(items, k)
	}
	return items, rows.Err()
}

// ─── SetBatchApproved ─────────────────────────────────────────────────────────

// SetBatchApproved transitions all PENDING_APPROVAL rows of a batch to APPROVED.
// Returns the number of rows updated.
func (r *DBRepository) SetBatchApproved(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, approverID uuid.UUID) (int64, error) {
	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		UPDATE mst.kurs
		SET workflow_status = 'APPROVED',
		    approver_id     = $1,
		    approved_at     = $2,
		    updated_at      = $2,
		    updated_by      = $1,
		    row_version     = row_version + 1
		WHERE upload_batch_id = $3
		  AND workflow_status = 'PENDING_APPROVAL'
		  AND deleted_at IS NULL
		  AND locked_flag = FALSE
	`, approverID, now, batchID)
	if err != nil {
		return 0, fmt.Errorf("repo.SetBatchApproved kurs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ─── SetBatchRejected ─────────────────────────────────────────────────────────

// SetBatchRejected transitions all PENDING_APPROVAL rows of a batch to REJECTED.
// Returns the number of rows updated.
func (r *DBRepository) SetBatchRejected(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, reason string, rejectorID uuid.UUID) (int64, error) {
	now := time.Now()
	res, err := tx.ExecContext(ctx, `
		UPDATE mst.kurs
		SET workflow_status = 'REJECTED',
		    reject_reason   = $1,
		    updated_at      = $2,
		    updated_by      = $3,
		    row_version     = row_version + 1
		WHERE upload_batch_id = $4
		  AND workflow_status = 'PENDING_APPROVAL'
		  AND deleted_at IS NULL
	`, reason, now, rejectorID, batchID)
	if err != nil {
		return 0, fmt.Errorf("repo.SetBatchRejected kurs: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ─── GetPreviousActiveRate ────────────────────────────────────────────────────

// GetPreviousActiveRate fetches kurs_tengah of the most recent APPROVED rate for
// the given kode before the given date (exclusive).
// Returns nil if no prior rate exists (first occurrence).
func (r *DBRepository) GetPreviousActiveRate(ctx context.Context, kode string, before time.Time) (*decimal.Decimal, error) {
	var rawRate string
	err := r.db.QueryRowContext(ctx, `
		SELECT kurs_tengah::TEXT
		FROM mst.kurs
		WHERE kode_mata_uang = $1
		  AND tanggal_berlaku < $2
		  AND workflow_status IN ('APPROVED', 'ACTIVE')
		  AND deleted_at IS NULL
		ORDER BY tanggal_berlaku DESC
		LIMIT 1
	`, kode, before).Scan(&rawRate)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.GetPreviousActiveRate kurs: %w", err)
	}

	d, err := decimal.NewFromString(rawRate)
	if err != nil {
		return nil, fmt.Errorf("repo.GetPreviousActiveRate parse decimal: %w", err)
	}
	return &d, nil
}

// ─── IsHoliday ────────────────────────────────────────────────────────────────

// IsHoliday returns true if the given date is in sys.holiday_calendar.
func (r *DBRepository) IsHoliday(ctx context.Context, tanggal time.Time) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sys.holiday_calendar WHERE tanggal = $1
		)
	`, tanggal.Format("2006-01-02")).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("repo.IsHoliday: %w", err)
	}
	return exists, nil
}

// ─── GetConfigParam ───────────────────────────────────────────────────────────

// GetConfigParam fetches a single config value from sys.config.
// Returns ("", sql.ErrNoRows) if key not found.
func (r *DBRepository) GetConfigParam(ctx context.Context, key string) (string, error) {
	var val string
	err := r.db.QueryRowContext(ctx, `
		SELECT config_value FROM sys.config WHERE config_key = $1 LIMIT 1
	`, key).Scan(&val)
	if err != nil {
		return "", fmt.Errorf("repo.GetConfigParam %q: %w", key, err)
	}
	return val, nil
}

// ─── GetInstrumenForTreatment ─────────────────────────────────────────────────

// GetInstrumenForTreatment returns (klasifikasi, kode_mata_uang) for the given
// instrumen UUID. Used by GetTreatment to determine FX accounting treatment.
// Returns ("", "", nil) if not found.
func (r *DBRepository) GetInstrumenForTreatment(ctx context.Context, instrumenID uuid.UUID) (string, string, error) {
	var klasifikasi, mataUang string
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(ki.kategori_psak71, ''),
		       COALESCE(i.kode_mata_uang, 'IDR')
		FROM mst.instrumen i
		LEFT JOIN mst.klasifikasi_instrumen ki
			ON ki.instrumen_id = i.id
		   AND ki.is_active = TRUE
		   AND ki.deleted_at IS NULL
		WHERE i.id = $1
		  AND i.deleted_at IS NULL
		LIMIT 1
	`, instrumenID).Scan(&klasifikasi, &mataUang)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("repo.GetInstrumenForTreatment: %w", err)
	}
	return klasifikasi, mataUang, nil
}

// ─── InsertDLQEntry ───────────────────────────────────────────────────────────

// InsertDLQEntry records a JISDOR fetch failure in sys.dlq_fx_jisdor.
// Called by worker after exhausting retries.
func (r *DBRepository) InsertDLQEntry(ctx context.Context, tanggal time.Time, kode, errCode, errMsg string, payloadJSON []byte) error {
	var kodeParam interface{}
	if kode != "" {
		kodeParam = kode
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sys.dlq_fx_jisdor
		    (id, job_type, tanggal_target, kode_mata_uang,
		     error_message, error_code, status, payload_jsonb,
		     created_at, created_by, updated_at, updated_by, row_version, tenant_id)
		VALUES
		    ($1, 'jisdor_fetch', $2, $3,
		     $4, $5, 'FAILED', $6,
		     now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE')
	`, uuid.New(), tanggal.Format("2006-01-02"), kodeParam, errMsg, errCode, payloadJSON)
	if err != nil {
		return fmt.Errorf("repo.InsertDLQEntry jisdor: %w", err)
	}
	return nil
}

// ─── LockRatesForPeriode / UnlockRatesForPeriode (P5-M4 hook) ────────────────

// LockRatesForPeriode sets locked_flag=TRUE for all non-deleted kurs rows in the periode.
// Must be called inside the same *sql.Tx as the hard-close commit.
// Idempotent: rows already locked are not double-locked.
func (r *DBRepository) LockRatesForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.kurs
		SET locked_flag = TRUE, updated_at = now()
		WHERE periode_bulanan_id = $1
		  AND deleted_at IS NULL
		  AND locked_flag = FALSE
	`, periodeID)
	if err != nil {
		return fmt.Errorf("repo.LockRatesForPeriode: %w", err)
	}
	return nil
}

// UnlockRatesForPeriode sets locked_flag=FALSE for all kurs rows in the periode.
// Called during CLOSED → SOFT_CLOSED reopen by closeflow.Service.
func (r *DBRepository) UnlockRatesForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.kurs
		SET locked_flag = FALSE, updated_at = now()
		WHERE periode_bulanan_id = $1
		  AND deleted_at IS NULL
	`, periodeID)
	if err != nil {
		return fmt.Errorf("repo.UnlockRatesForPeriode: %w", err)
	}
	return nil
}
