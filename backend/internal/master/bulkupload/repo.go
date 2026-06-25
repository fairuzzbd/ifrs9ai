package bulkupload

// repo.go — Repository for sys.upload_batch (INSTRUMEN_BULK) + sys.upload_batch_row.
//
// Conventions (DEC-016/020/022):
//   - tenant_id in all WHERE clauses
//   - cursor-based pagination (no offset); limit+1 trick for hasMore
//   - TX boundary lives in service.go (repo never opens TX directly)
//   - Never float64 — numeric passed as strings to driver / decimal in application
//   - sqlmock-friendly: uses database/sql interface only

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Repository is the persistence interface for bulk upload.
type Repository interface {
	// Batch operations
	InsertBatch(ctx context.Context, tx *sql.Tx, b *Batch) error
	GetBatch(ctx context.Context, batchID uuid.UUID, tenantID string) (*Batch, error)
	UpdateBatchStatus(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, status BatchStatus, updatedBy uuid.UUID) error
	UpdateBatchDryRun(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, result *DryRunResult, expiresAt time.Time, updatedBy uuid.UUID) error
	UpdateBatchCommitted(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, committedRows, failedRows int, graceDays int, updatedBy uuid.UUID) error
	UpdateBatchApproved(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, approverID uuid.UUID, activatedCount int) error
	UpdateBatchRollbackPending(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, reason string, requestedBy uuid.UUID) error
	UpdateBatchRolledBack(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, rolledBackBy uuid.UUID, count int) error

	// Row operations
	InsertBatchRows(ctx context.Context, tx *sql.Tx, rows []BatchRow) error
	ListBatchRows(ctx context.Context, batchID uuid.UUID, q listquery.Query, tenantID string) ([]BatchRow, Pagination, error)
	GetBatchRowsByStatus(ctx context.Context, batchID uuid.UUID, status RowStatus) ([]BatchRow, error)
	UpdateRowStatus(ctx context.Context, tx *sql.Tx, rowID uuid.UUID, status RowStatus, instrumenID *uuid.UUID, rowErr *json.RawMessage) error
	UpdateRowsRolledBack(ctx context.Context, tx *sql.Tx, batchID uuid.UUID) (int, error)

	// Instrumen operations (for commit + approve + rollback)
	InsertInstrumen(ctx context.Context, tx *sql.Tx, row RowValidationResult, batchID uuid.UUID, actor uuid.UUID, tenantID string) (uuid.UUID, error)
	ActivateInstrumenByBatch(ctx context.Context, tx *sql.Tx, batchID uuid.UUID) (int, error)
	SoftDeleteInstrumenByBatch(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, deletedBy uuid.UUID) (int, error)
	CountPendingManualByBatch(ctx context.Context, batchID uuid.UUID) (int, error)

	// Config param
	GetConfigParamInt(ctx context.Context, key string, defaultVal int) (int, error)

	// Periode lock
	GetActivePeriodeStatus(ctx context.Context, tenantID string) (string, error)

	// Cross-ref lookups (for Stage 3 validator)
	CounterpartyExists(id string, tenantID string) (bool, error)
	BankExists(id string, tenantID string) (bool, error)
	MataUangExists(kode string, tenantID string) (bool, error)
	InstrumenKodeExists(kode string, tenantID string) (bool, error)
}

// sqlRepo is the concrete database/sql implementation.
type sqlRepo struct {
	db *sql.DB
}

// NewRepository creates a new sql-backed Repository.
func NewRepository(db *sql.DB) Repository {
	return &sqlRepo{db: db}
}

const defaultLimit = 50

// repoLimit returns page size. listquery.Query does not carry a Limit field.
func repoLimit(_ listquery.Query) int { return defaultLimit }

// ─── Batch operations ─────────────────────────────────────────────────────────

func (r *sqlRepo) InsertBatch(ctx context.Context, tx *sql.Tx, b *Batch) error {
	sheetJSON, _ := json.Marshal(b.SheetBreakdownJson)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sys.upload_batch (
			id, batch_code, batch_type, filename_original, file_sha256, file_storage_url,
			uploaded_by, uploaded_at, total_rows, sheet_breakdown_json, status,
			created_at, tenant_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		b.ID, b.BatchCode, b.BatchType, b.FilenameOriginal, b.FileSHA256, b.FileStorageURL,
		b.UploadedBy, b.UploadedAt, b.TotalRows, sheetJSON, string(b.Status),
		b.CreatedAt, b.TenantID,
	)
	if err != nil {
		return fmt.Errorf("InsertBatch: %w", err)
	}
	return nil
}

func (r *sqlRepo) GetBatch(ctx context.Context, batchID uuid.UUID, tenantID string) (*Batch, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, batch_code, batch_type, filename_original, file_sha256, file_storage_url,
		       uploaded_by, uploaded_at, total_rows, valid_rows, committed_rows,
		       sheet_breakdown_json, status,
		       approver_id, approved_at, committed_at,
		       dry_run_cached_at, dry_run_expires_at, dry_run_result_jsonb,
		       rollback_status, rollback_grace_expires_at, rollback_by, rollback_at, rollback_reason,
		       created_at, tenant_id
		FROM sys.upload_batch
		WHERE id = $1 AND tenant_id = $2`,
		batchID, tenantID)

	b := &Batch{}
	var sheetJSON []byte
	var dryRunResultJSON []byte
	err := row.Scan(
		&b.ID, &b.BatchCode, &b.BatchType, &b.FilenameOriginal, &b.FileSHA256, &b.FileStorageURL,
		&b.UploadedBy, &b.UploadedAt, &b.TotalRows, &b.ValidRows, &b.CommittedRows,
		&sheetJSON, &b.Status,
		&b.ApproverID, &b.ApprovedAt, &b.CommittedAt,
		&b.DryRunCachedAt, &b.DryRunExpiresAt, &dryRunResultJSON,
		&b.RollbackStatus, &b.RollbackGraceExpiresAt, &b.RollbackBy, &b.RollbackAt, &b.RollbackReason,
		&b.CreatedAt, &b.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetBatch: %w", err)
	}
	if len(sheetJSON) > 0 {
		raw := json.RawMessage(sheetJSON)
		b.SheetBreakdownJson = &raw
	}
	if len(dryRunResultJSON) > 0 {
		raw := json.RawMessage(dryRunResultJSON)
		b.DryRunResultJsonb = &raw
	}
	return b, nil
}

func (r *sqlRepo) UpdateBatchStatus(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, status BatchStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch SET status = $1, updated_at = now(), updated_by = $2
		WHERE id = $3`,
		string(status), updatedBy, batchID)
	if err != nil {
		return fmt.Errorf("UpdateBatchStatus: %w", err)
	}
	return nil
}

func (r *sqlRepo) UpdateBatchDryRun(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, result *DryRunResult, expiresAt time.Time, updatedBy uuid.UUID) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("UpdateBatchDryRun: marshal: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE sys.upload_batch
		SET status = $1,
		    dry_run_cached_at = now(),
		    dry_run_expires_at = $2,
		    dry_run_result_jsonb = $3,
		    valid_rows = $4,
		    updated_at = now(), updated_by = $5
		WHERE id = $6`,
		string(result.Status), expiresAt, resultJSON, result.ValidRows, updatedBy, batchID)
	if err != nil {
		return fmt.Errorf("UpdateBatchDryRun: %w", err)
	}
	return nil
}

func (r *sqlRepo) UpdateBatchCommitted(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, committedRows, failedRows int, graceDays int, updatedBy uuid.UUID) error {
	batchStatus := StatusCommitted
	if failedRows > 0 {
		batchStatus = StatusPartialCommit
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch
		SET status = $1,
		    committed_rows = $2,
		    committed_at = now(),
		    rollback_grace_expires_at = now() + ($3 * INTERVAL '1 day'),
		    updated_at = now(), updated_by = $4
		WHERE id = $5`,
		string(batchStatus), committedRows, graceDays, updatedBy, batchID)
	if err != nil {
		return fmt.Errorf("UpdateBatchCommitted: %w", err)
	}
	return nil
}

func (r *sqlRepo) UpdateBatchApproved(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, approverID uuid.UUID, activatedCount int) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch
		SET status = $1, approver_id = $2, approved_at = now(), updated_at = now(), updated_by = $2
		WHERE id = $3`,
		string(StatusApproved), approverID, batchID)
	if err != nil {
		return fmt.Errorf("UpdateBatchApproved: %w", err)
	}
	return nil
}

func (r *sqlRepo) UpdateBatchRollbackPending(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, reason string, requestedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch
		SET status = $1, rollback_status = 'PENDING', rollback_reason = $2,
		    rollback_by = $3, updated_at = now(), updated_by = $3
		WHERE id = $4`,
		string(StatusRollbackPending), reason, requestedBy, batchID)
	if err != nil {
		return fmt.Errorf("UpdateBatchRollbackPending: %w", err)
	}
	return nil
}

func (r *sqlRepo) UpdateBatchRolledBack(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, rolledBackBy uuid.UUID, count int) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch
		SET status = $1, rollback_status = 'APPROVED', rollback_at = now(),
		    updated_at = now(), updated_by = $2
		WHERE id = $3`,
		string(StatusRolledBack), rolledBackBy, batchID)
	if err != nil {
		return fmt.Errorf("UpdateBatchRolledBack: %w", err)
	}
	return nil
}

// ─── Row operations ───────────────────────────────────────────────────────────

func (r *sqlRepo) InsertBatchRows(ctx context.Context, tx *sql.Tx, rows []BatchRow) error {
	for _, row := range rows {
		rowDataJSON, _ := json.Marshal(row.RowDataJson)
		var errJSON []byte
		if row.RowErrorJsonb != nil {
			errJSON, _ = json.Marshal(row.RowErrorJsonb)
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO sys.upload_batch_row (id, batch_id, row_number, sheet_name, row_data_json, row_status, row_error_jsonb, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,now())`,
			row.ID, row.BatchID, row.RowNumber, row.SheetName, rowDataJSON, string(row.RowStatus), errJSON,
		)
		if err != nil {
			return fmt.Errorf("InsertBatchRows row %d: %w", row.RowNumber, err)
		}
	}
	return nil
}

func (r *sqlRepo) ListBatchRows(ctx context.Context, batchID uuid.UUID, q listquery.Query, tenantID string) ([]BatchRow, Pagination, error) {
	limit := repoLimit(q)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, batch_id, row_number, sheet_name, row_data_json, row_status, bulk_instrumen_id, row_error_jsonb, created_at
		FROM sys.upload_batch_row
		WHERE batch_id = $1
		ORDER BY row_number ASC
		LIMIT $2`,
		batchID, limit+1)
	if err != nil {
		return nil, Pagination{}, fmt.Errorf("ListBatchRows: %w", err)
	}
	defer rows.Close()

	var result []BatchRow
	for rows.Next() {
		var br BatchRow
		var rowDataJSON, rowErrJSON []byte
		if err := rows.Scan(&br.ID, &br.BatchID, &br.RowNumber, &br.SheetName,
			&rowDataJSON, &br.RowStatus, &br.BulkInstrumenID, &rowErrJSON, &br.CreatedAt); err != nil {
			return nil, Pagination{}, fmt.Errorf("ListBatchRows scan: %w", err)
		}
		if len(rowDataJSON) > 0 {
			br.RowDataJson = rowDataJSON
		}
		if len(rowErrJSON) > 0 {
			raw := json.RawMessage(rowErrJSON)
			br.RowErrorJsonb = &raw
		}
		result = append(result, br)
	}

	pag := Pagination{Limit: limit}
	if len(result) > limit {
		pag.HasMore = true
		result = result[:limit]
	}
	return result, pag, nil
}

func (r *sqlRepo) GetBatchRowsByStatus(ctx context.Context, batchID uuid.UUID, status RowStatus) ([]BatchRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, batch_id, row_number, sheet_name, row_data_json, row_status, bulk_instrumen_id, row_error_jsonb, created_at
		FROM sys.upload_batch_row
		WHERE batch_id = $1 AND row_status = $2
		ORDER BY row_number ASC`,
		batchID, string(status))
	if err != nil {
		return nil, fmt.Errorf("GetBatchRowsByStatus: %w", err)
	}
	defer rows.Close()

	var result []BatchRow
	for rows.Next() {
		var br BatchRow
		var rowDataJSON, rowErrJSON []byte
		if err := rows.Scan(&br.ID, &br.BatchID, &br.RowNumber, &br.SheetName,
			&rowDataJSON, &br.RowStatus, &br.BulkInstrumenID, &rowErrJSON, &br.CreatedAt); err != nil {
			return nil, fmt.Errorf("GetBatchRowsByStatus scan: %w", err)
		}
		if len(rowDataJSON) > 0 {
			br.RowDataJson = rowDataJSON
		}
		if len(rowErrJSON) > 0 {
			raw := json.RawMessage(rowErrJSON)
			br.RowErrorJsonb = &raw
		}
		result = append(result, br)
	}
	return result, nil
}

func (r *sqlRepo) UpdateRowStatus(ctx context.Context, tx *sql.Tx, rowID uuid.UUID, status RowStatus, instrumenID *uuid.UUID, rowErr *json.RawMessage) error {
	var errJSON []byte
	if rowErr != nil {
		errJSON, _ = json.Marshal(rowErr)
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch_row
		SET row_status = $1, bulk_instrumen_id = $2, row_error_jsonb = $3, updated_at = now()
		WHERE id = $4`,
		string(status), instrumenID, errJSON, rowID)
	if err != nil {
		return fmt.Errorf("UpdateRowStatus: %w", err)
	}
	return nil
}

func (r *sqlRepo) UpdateRowsRolledBack(ctx context.Context, tx *sql.Tx, batchID uuid.UUID) (int, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE sys.upload_batch_row
		SET row_status = 'ROLLED_BACK', updated_at = now()
		WHERE batch_id = $1 AND row_status IN ('COMMITTED','FLAGGED_MANUAL_REVIEW')`,
		batchID)
	if err != nil {
		return 0, fmt.Errorf("UpdateRowsRolledBack: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ─── Instrumen operations ─────────────────────────────────────────────────────

func (r *sqlRepo) InsertInstrumen(ctx context.Context, tx *sql.Tx, row RowValidationResult, batchID uuid.UUID, actor uuid.UUID, tenantID string) (uuid.UUID, error) {
	id := uuid.New()
	kode := getStr(row.RowData, "kode")
	mataUang := getStr(row.RowData, "mata_uang")

	// Determine tipe from sheet
	tipeMap := map[SheetName]string{
		SheetDeposito:  "DEPOSITO",
		SheetObligasi:  "OBLIGASI",
		SheetSaham:     "SAHAM",
		SheetReksadana: "REKSADANA",
		SheetTabungan:  "TABUNGAN",
	}
	tipeInstrumen := tipeMap[row.SheetName]

	instrumenStatus := "PENDING_APPROVAL_BULK"

	klsf := row.KlasifikasiPsak71
	if row.Status == RowStatusFlaggedManualReview {
		instrumenStatus = "PENDING_CLASSIFICATION"
		klsf = nil
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO mst.instrumen (
			id, kode_instrumen, tipe_instrumen, mata_uang,
			klasifikasi_psak71, status, workflow_status,
			bulk_upload_batch_id, portofolio_id,
			created_by, created_at, version, is_deleted, tenant_id
		) VALUES ($1,$2,$3,$4,$5,$6,'DRAFT',$7,
		          (SELECT id FROM mst.portofolio WHERE tenant_id=$8 LIMIT 1),
		          $9, now(), 1, false, $8)`,
		id, kode, tipeInstrumen, mataUang,
		klsf, instrumenStatus,
		batchID, tenantID, actor,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("InsertInstrumen kode=%s: %w", kode, err)
	}
	return id, nil
}

func (r *sqlRepo) ActivateInstrumenByBatch(ctx context.Context, tx *sql.Tx, batchID uuid.UUID) (int, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE mst.instrumen
		SET status = 'AKTIF', updated_at = now()
		WHERE bulk_upload_batch_id = $1 AND status = 'PENDING_APPROVAL_BULK' AND is_deleted = false`,
		batchID)
	if err != nil {
		return 0, fmt.Errorf("ActivateInstrumenByBatch: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *sqlRepo) SoftDeleteInstrumenByBatch(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, deletedBy uuid.UUID) (int, error) {
	res, err := tx.ExecContext(ctx, `
		UPDATE mst.instrumen
		SET is_deleted = true, deleted_at = now(), deleted_by = $1, updated_at = now()
		WHERE bulk_upload_batch_id = $2 AND is_deleted = false`,
		deletedBy, batchID)
	if err != nil {
		return 0, fmt.Errorf("SoftDeleteInstrumenByBatch: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *sqlRepo) CountPendingManualByBatch(ctx context.Context, batchID uuid.UUID) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.instrumen
		WHERE bulk_upload_batch_id = $1 AND status = 'PENDING_CLASSIFICATION' AND is_deleted = false`,
		batchID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("CountPendingManualByBatch: %w", err)
	}
	return count, nil
}

// ─── Config param ─────────────────────────────────────────────────────────────

func (r *sqlRepo) GetConfigParamInt(ctx context.Context, key string, defaultVal int) (int, error) {
	var val string
	err := r.db.QueryRowContext(ctx, `
		SELECT param_value FROM sys.config_param WHERE param_key = $1 LIMIT 1`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return defaultVal, nil
	}
	if err != nil {
		return defaultVal, fmt.Errorf("GetConfigParamInt %s: %w", key, err)
	}
	n := defaultVal
	if _, err := fmt.Sscanf(val, "%d", &n); err != nil {
		return defaultVal, nil
	}
	return n, nil
}

// ─── Periode lock ─────────────────────────────────────────────────────────────

func (r *sqlRepo) GetActivePeriodeStatus(ctx context.Context, tenantID string) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx, `
		SELECT status_periode FROM mst.periode_buku
		WHERE tenant_id = $1 AND is_current = true
		LIMIT 1`, tenantID).Scan(&status)
	if err == sql.ErrNoRows {
		return "OPEN", nil // no active periode — allow upload
	}
	if err != nil {
		return "", fmt.Errorf("GetActivePeriodeStatus: %w", err)
	}
	return status, nil
}

// ─── Cross-ref lookups (Stage 3) ──────────────────────────────────────────────

func (r *sqlRepo) CounterpartyExists(id string, tenantID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.counterparty
		WHERE id::TEXT = $1 AND tenant_id = $2 AND is_deleted = false`, id, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *sqlRepo) BankExists(id string, tenantID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.counterparty
		WHERE id::TEXT = $1 AND tenant_id = $2 AND tipe_counterparty = 'BANK' AND is_deleted = false`, id, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *sqlRepo) MataUangExists(kode string, tenantID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.mata_uang
		WHERE kode = $1 AND tenant_id = $2 AND is_deleted = false`, kode, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *sqlRepo) InstrumenKodeExists(kode string, tenantID string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.instrumen
		WHERE kode_instrumen = $1 AND tenant_id = $2 AND is_deleted = false`, kode, tenantID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
