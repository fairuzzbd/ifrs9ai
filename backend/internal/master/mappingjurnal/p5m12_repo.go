package mappingjurnal

// p5m12_repo.go — P5-M12 repository extension.
// Adds methods to DBRepository for:
//   - Version chain (GetActiveByEventCode, GetVersionByID, InsertVersion, FlipActiveVersion)
//   - 6-eyes workflow transitions (Submit, Review, Approve, Approve2, Reject)
//   - COA code existence check
//   - Regulated event detection (GetConfigParam)
//   - RPT-19 Coverage, RPT-20 Validation, RPT-21 History
//   - Bulk import DRAFT version creation
//   - Periode lock check
//   - Duplicate in-flight version check
//
// References: P5-M12-S1..S5, migration 000049, DEC-018.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Repository interface extension ──────────────────────────────────────────

// P5M12Repository extends Repository with P5-M12 specific methods.
// Embedded in P5M12Service; implemented by DBRepository (which also implements Repository).
type P5M12Repository interface {
	Repository // embed base interface

	// Version chain
	GetActiveByEventCode(ctx context.Context, eventCode string, tenantID string) (*Header, error)
	GetVersionByID(ctx context.Context, versionID uuid.UUID, tenantID string) (*Header, error)
	InsertVersion(ctx context.Context, tx *sql.Tx, h *Header, details []AkunDetail, actor uuid.UUID, tenantID string) error
	FlipActiveVersion(ctx context.Context, tx *sql.Tx, eventCode string, newVersionID uuid.UUID, actor uuid.UUID, tenantID string) error
	HasInflightVersion(ctx context.Context, eventCode string, tenantID string) (bool, error)

	// Workflow transitions (direct col updates for 6-eyes SoD)
	SubmitVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, makerID uuid.UUID, now time.Time, tenantID string) error
	ReviewVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, reviewerID uuid.UUID, sigHash []byte, comment string, regulated bool, now time.Time, tenantID string) error
	Approve4Eyes(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, approverID uuid.UUID, sigHash []byte, comment string, now time.Time, tenantID string) error
	Approve6Eyes(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, approver2ID uuid.UUID, sigHash, tokenRef []byte, comment string, now time.Time, tenantID string) error
	RejectVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, reason string, actor uuid.UUID, now time.Time, tenantID string) error

	// COA + config
	CoaCodeExists(ctx context.Context, akunCode string, tenantID string) (bool, error)
	EventCodeExists(ctx context.Context, eventCode string, tenantID string) (bool, error)
	GetConfigParam(ctx context.Context, key string) (string, error)
	GetPeriodeStatus(ctx context.Context, tenantID string) (string, error)

	// Bulk import
	InsertDraftForBulkRow(ctx context.Context, tx *sql.Tx, row MappingBulkRow, batchID uuid.UUID, actor uuid.UUID, tenantID string) error
	InsertUploadBatch(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, actor uuid.UUID, totalRows, validRows, invalidRows int, tenantID string) error

	// RPT-19
	GetCoverageReport(ctx context.Context, tenantID string) (*CoverageResp, error)

	// RPT-20
	GetValidationReport(ctx context.Context, tenantID string) (*ValidationResp, error)

	// RPT-21
	ListMappingHistory(ctx context.Context, q listquery.Query, filterEventCode string, cursor string, limit int, tenantID string) ([]MappingAuditEntry, *string, bool, error)
}

// Compile-time check: DBRepository implements P5M12Repository.
var _ P5M12Repository = (*DBRepository)(nil)

// ─── DBRepository P5-M12 methods ─────────────────────────────────────────────

// GetActiveByEventCode returns the APPROVED_ACTIVE header for an event_code.
func (r *DBRepository) GetActiveByEventCode(ctx context.Context, eventCode string, tenantID string) (*Header, error) {
	const q = `
SELECT id, event_id_kode, event_code, nama_event, kategori_event, trigger_source,
       aktif_flag, catatan, workflow_status, workflow_path,
       maker_id, reviewer_id, approver_id, approver_2_id,
       reviewer_signed_at, reviewer_signature_hash, comment_review,
       approver_signed_at, approver_signature_hash, comment_approve,
       approver_2_signed_at, approver_2_signature_hash, comment_approve_2,
       submit_at, reject_reason,
       parent_id, effective_from, effective_to, regulated_flag, step_up_token_ref,
       created_at, created_by, updated_at, updated_by, deleted_at, row_version, tenant_id
FROM mst.mapping_jurnal_header
WHERE event_code = $1
  AND workflow_status = 'APPROVED_ACTIVE'
  AND deleted_at IS NULL
  AND tenant_id = $2
LIMIT 1`
	return r.scanP5Header(r.db.QueryRowContext(ctx, q, eventCode, tenantID))
}

// GetVersionByID returns a specific version header by ID + tenant.
func (r *DBRepository) GetVersionByID(ctx context.Context, versionID uuid.UUID, tenantID string) (*Header, error) {
	const q = `
SELECT id, event_id_kode, event_code, nama_event, kategori_event, trigger_source,
       aktif_flag, catatan, workflow_status, workflow_path,
       maker_id, reviewer_id, approver_id, approver_2_id,
       reviewer_signed_at, reviewer_signature_hash, comment_review,
       approver_signed_at, approver_signature_hash, comment_approve,
       approver_2_signed_at, approver_2_signature_hash, comment_approve_2,
       submit_at, reject_reason,
       parent_id, effective_from, effective_to, regulated_flag, step_up_token_ref,
       created_at, created_by, updated_at, updated_by, deleted_at, row_version, tenant_id
FROM mst.mapping_jurnal_header
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`
	return r.scanP5Header(r.db.QueryRowContext(ctx, q, versionID, tenantID))
}

// HasInflightVersion returns true if event_code already has a DRAFT/PENDING_* version.
// Used to enforce MAPPING_DUPLICATE_VERSION guard.
func (r *DBRepository) HasInflightVersion(ctx context.Context, eventCode string, tenantID string) (bool, error) {
	const q = `
SELECT EXISTS (
    SELECT 1 FROM mst.mapping_jurnal_header
    WHERE event_code = $1
      AND workflow_status IN ('DRAFT','PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
      AND deleted_at IS NULL
      AND tenant_id = $2
)`
	var exists bool
	err := r.db.QueryRowContext(ctx, q, eventCode, tenantID).Scan(&exists)
	return exists, err
}

// InsertVersion inserts a new DRAFT version of a mapping (for new-version endpoint).
// Writes detail rows atomically. Sets parent_id = prior active version's id (if any).
func (r *DBRepository) InsertVersion(ctx context.Context, tx *sql.Tx, h *Header, details []AkunDetail, actor uuid.UUID, tenantID string) error {
	const hq = `
INSERT INTO mst.mapping_jurnal_header (
    id, event_id_kode, event_code, nama_event, kategori_event, trigger_source,
    aktif_flag, catatan, workflow_status, workflow_path, regulated_flag,
    parent_id, effective_from, effective_to,
    maker_id,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    FALSE, $7, 'DRAFT', $8, $9,
    $10, now(), 'infinity'::TIMESTAMPTZ,
    $11,
    now(), $12, now(), $12, 1, $13
)`
	_, err := tx.ExecContext(ctx, hq,
		h.ID, h.EventIDKode, h.EventCode, h.NamaEvent, h.KategoriEvent, h.TriggerSource,
		h.Catatan,
		h.WorkflowPath,
		h.RegulatedFlag,
		h.ParentID,
		actor,
		actor,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.InsertVersion header: %w", err)
	}

	for i, d := range details {
		const dq = `
INSERT INTO mst.mapping_jurnal_detail (
    id, event_header_id, urutan,
    akun_debit, akun_kredit, dk_indicator, jumlah_calc,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    $1, $2, $3,
    $4, $5, $6, $7,
    now(), $8, now(), $8, 1, $9
)`
		dID := uuid.New()
		_, err := tx.ExecContext(ctx, dq,
			dID, h.ID, i+1,
			d.AkunDebit, d.AkunKredit, d.DebitKredit, d.JumlahCalc,
			actor, tenantID,
		)
		if err != nil {
			return fmt.Errorf("repo.InsertVersion detail row %d: %w", i+1, err)
		}
	}
	return nil
}

// FlipActiveVersion atomically sets effective_to=now() on current APPROVED_ACTIVE row
// for eventCode, then activates newVersionID.
// Called WITHIN the approve/approve-2 transaction (already open).
func (r *DBRepository) FlipActiveVersion(ctx context.Context, tx *sql.Tx, eventCode string, newVersionID uuid.UUID, actor uuid.UUID, tenantID string) error {
	// Step 1: close prior ACTIVE version's effective_to
	const close = `
UPDATE mst.mapping_jurnal_header
SET effective_to = now(),
    updated_at   = now(),
    updated_by   = $1,
    row_version  = row_version + 1
WHERE event_code = $2
  AND workflow_status = 'APPROVED_ACTIVE'
  AND deleted_at IS NULL
  AND tenant_id = $3`
	if _, err := tx.ExecContext(ctx, close, actor, eventCode, tenantID); err != nil {
		return fmt.Errorf("repo.FlipActiveVersion close prior: %w", err)
	}
	// Step 2: set new version aktif_flag = TRUE (workflow_status already set by caller)
	const activate = `
UPDATE mst.mapping_jurnal_header
SET aktif_flag  = TRUE,
    effective_to = 'infinity'::TIMESTAMPTZ,
    updated_at   = now(),
    updated_by   = $1,
    row_version  = row_version + 1
WHERE id = $2 AND tenant_id = $3`
	if _, err := tx.ExecContext(ctx, activate, actor, newVersionID, tenantID); err != nil {
		return fmt.Errorf("repo.FlipActiveVersion activate: %w", err)
	}
	return nil
}

// ─── Workflow transitions ─────────────────────────────────────────────────────

// SubmitVersion transitions a DRAFT version to PENDING_REVIEW.
func (r *DBRepository) SubmitVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, makerID uuid.UUID, now time.Time, tenantID string) error {
	const q = `
UPDATE mst.mapping_jurnal_header
SET workflow_status = 'PENDING_REVIEW',
    maker_id        = $1,
    submit_at       = $2,
    reject_reason   = NULL,
    updated_at      = $2,
    updated_by      = $1,
    row_version     = row_version + 1
WHERE id = $3 AND workflow_status = 'DRAFT' AND tenant_id = $4 AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, q, makerID, now, versionID, tenantID)
	if err != nil {
		return fmt.Errorf("repo.SubmitVersion: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("WORKFLOW_INVALID_TRANSITION: version %s tidak dalam status DRAFT atau tidak ditemukan", versionID)
	}
	return nil
}

// ReviewVersion transitions PENDING_REVIEW → PENDING_APPROVAL (4-eyes) or PENDING_APPROVAL_2 (6-eyes).
func (r *DBRepository) ReviewVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, reviewerID uuid.UUID, sigHash []byte, comment string, regulated bool, now time.Time, tenantID string) error {
	nextStatus := "PENDING_APPROVAL"
	if regulated {
		nextStatus = "PENDING_APPROVAL_2"
	}
	const q = `
UPDATE mst.mapping_jurnal_header
SET workflow_status          = $1,
    reviewer_id              = $2,
    reviewer_signed_at       = $3,
    reviewer_signature_hash  = $4,
    comment_review           = $5,
    updated_at               = $3,
    updated_by               = $2,
    row_version              = row_version + 1
WHERE id = $6 AND workflow_status = 'PENDING_REVIEW' AND tenant_id = $7 AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, q, nextStatus, reviewerID, now, sigHash, comment, versionID, tenantID)
	if err != nil {
		return fmt.Errorf("repo.ReviewVersion: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("WORKFLOW_INVALID_TRANSITION: version %s tidak dalam status PENDING_REVIEW atau tidak ditemukan", versionID)
	}
	return nil
}

// Approve4Eyes transitions PENDING_APPROVAL → APPROVED_ACTIVE (4-eyes path).
func (r *DBRepository) Approve4Eyes(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, approverID uuid.UUID, sigHash []byte, comment string, now time.Time, tenantID string) error {
	const q = `
UPDATE mst.mapping_jurnal_header
SET workflow_status          = 'APPROVED_ACTIVE',
    approver_id              = $1,
    approver_signed_at       = $2,
    approver_signature_hash  = $3,
    comment_approve          = $4,
    updated_at               = $2,
    updated_by               = $1,
    row_version              = row_version + 1
WHERE id = $5 AND workflow_status = 'PENDING_APPROVAL' AND tenant_id = $6 AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, q, approverID, now, sigHash, comment, versionID, tenantID)
	if err != nil {
		return fmt.Errorf("repo.Approve4Eyes: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("WORKFLOW_INVALID_TRANSITION: version %s tidak dalam status PENDING_APPROVAL atau tidak ditemukan", versionID)
	}
	return nil
}

// Approve6Eyes transitions PENDING_APPROVAL_2 → APPROVED_ACTIVE (6-eyes path).
func (r *DBRepository) Approve6Eyes(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, approver2ID uuid.UUID, sigHash, tokenRef []byte, comment string, now time.Time, tenantID string) error {
	const q = `
UPDATE mst.mapping_jurnal_header
SET workflow_status              = 'APPROVED_ACTIVE',
    approver_2_id                = $1,
    approver_2_signed_at         = $2,
    approver_2_signature_hash    = $3,
    comment_approve_2            = $4,
    step_up_token_ref            = $5,
    updated_at                   = $2,
    updated_by                   = $1,
    row_version                  = row_version + 1
WHERE id = $6 AND workflow_status = 'PENDING_APPROVAL_2' AND tenant_id = $7 AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, q, approver2ID, now, sigHash, comment, tokenRef, versionID, tenantID)
	if err != nil {
		return fmt.Errorf("repo.Approve6Eyes: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("WORKFLOW_INVALID_TRANSITION: version %s tidak dalam status PENDING_APPROVAL_2 atau tidak ditemukan", versionID)
	}
	return nil
}

// RejectVersion resets a PENDING_* version back to DRAFT.
func (r *DBRepository) RejectVersion(ctx context.Context, tx *sql.Tx, versionID uuid.UUID, reason string, actor uuid.UUID, now time.Time, tenantID string) error {
	const q = `
UPDATE mst.mapping_jurnal_header
SET workflow_status = 'DRAFT',
    reject_reason   = $1,
    updated_at      = $2,
    updated_by      = $3,
    row_version     = row_version + 1
WHERE id = $4
  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
  AND tenant_id = $5
  AND deleted_at IS NULL`
	res, err := tx.ExecContext(ctx, q, reason, now, actor, versionID, tenantID)
	if err != nil {
		return fmt.Errorf("repo.RejectVersion: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("WORKFLOW_INVALID_TRANSITION: version %s tidak dalam status PENDING_* atau tidak ditemukan", versionID)
	}
	return nil
}

// ─── COA + config ─────────────────────────────────────────────────────────────

// CoaCodeExists returns true if the given account code exists in mst.chart_of_accounts.
func (r *DBRepository) CoaCodeExists(ctx context.Context, akunCode string, tenantID string) (bool, error) {
	const q = `
SELECT EXISTS (
    SELECT 1 FROM mst.chart_of_accounts
    WHERE kode_akun = $1 AND tenant_id = $2 AND deleted_at IS NULL
)`
	var exists bool
	err := r.db.QueryRowContext(ctx, q, akunCode, tenantID).Scan(&exists)
	return exists, err
}

// EventCodeExists returns true if the event_code exists in mst.mapping_jurnal_header.
func (r *DBRepository) EventCodeExists(ctx context.Context, eventCode string, tenantID string) (bool, error) {
	const q = `
SELECT EXISTS (
    SELECT 1 FROM mst.mapping_jurnal_header
    WHERE event_code = $1 AND tenant_id = $2 AND deleted_at IS NULL
)`
	var exists bool
	err := r.db.QueryRowContext(ctx, q, eventCode, tenantID).Scan(&exists)
	return exists, err
}

// GetConfigParam returns the config_value for a given config_key from sys.config.
func (r *DBRepository) GetConfigParam(ctx context.Context, key string) (string, error) {
	const q = `SELECT COALESCE(config_value,'') FROM sys.config WHERE config_key = $1 LIMIT 1`
	var val string
	err := r.db.QueryRowContext(ctx, q, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// GetPeriodeStatus returns the status_periode of the most recent active periode_buku.
func (r *DBRepository) GetPeriodeStatus(ctx context.Context, tenantID string) (string, error) {
	const q = `
SELECT COALESCE(status_periode,'OPEN')
FROM mst.periode_buku
WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY tanggal_mulai DESC
LIMIT 1`
	var status string
	err := r.db.QueryRowContext(ctx, q, tenantID).Scan(&status)
	if err == sql.ErrNoRows {
		return "OPEN", nil
	}
	return status, err
}

// ─── Bulk import ─────────────────────────────────────────────────────────────

// InsertUploadBatch inserts a sys.upload_batch row for MAPPING_BULK.
func (r *DBRepository) InsertUploadBatch(ctx context.Context, tx *sql.Tx, batchID uuid.UUID, actor uuid.UUID, totalRows, validRows, invalidRows int, tenantID string) error {
	const q = `
INSERT INTO sys.upload_batch (
    id, batch_type, status, total_rows, valid_rows, failed_rows,
    uploaded_by, uploaded_at,
    created_at, created_by, updated_at, updated_by, tenant_id
) VALUES (
    $1, 'MAPPING_BULK', 'PARSED', $2, $3, $4,
    $5, now(),
    now(), $5, now(), $5, $6
)`
	_, err := tx.ExecContext(ctx, q, batchID, totalRows, validRows, invalidRows, actor, tenantID)
	if err != nil {
		return fmt.Errorf("repo.InsertUploadBatch: %w", err)
	}
	return nil
}

// InsertDraftForBulkRow creates a DRAFT mapping_jurnal_header from a bulk row.
// Does NOT replace existing ACTIVE mappings (INSERT only; caller handles version chain).
func (r *DBRepository) InsertDraftForBulkRow(ctx context.Context, tx *sql.Tx, row MappingBulkRow, batchID uuid.UUID, actor uuid.UUID, tenantID string) error {
	// Get existing header for event_code to inherit metadata
	existing, err := r.GetActiveByEventCode(ctx, row.EventCode, tenantID)
	if err != nil {
		return fmt.Errorf("repo.InsertDraftForBulkRow GetActive: %w", err)
	}

	headerID := uuid.New()
	var parentID *uuid.UUID
	eventIDKode := row.EventCode // fallback
	namaEvent := row.EventCode
	kategoriEvent := "BULK_IMPORT"
	triggerSource := "USER_INPUT"
	workflowPath := "4-eyes"
	regulatedFlag := isRegulatedFallback(row.EventCode)
	if regulatedFlag {
		workflowPath = "6-eyes"
	}

	if existing != nil {
		parentID = &existing.ID
		eventIDKode = existing.EventIDKode
		namaEvent = existing.NamaEvent
		kategoriEvent = existing.KategoriEvent
		triggerSource = existing.TriggerSource
	}

	const hq = `
INSERT INTO mst.mapping_jurnal_header (
    id, event_id_kode, event_code, nama_event, kategori_event, trigger_source,
    aktif_flag, workflow_status, workflow_path, regulated_flag,
    parent_id, effective_from, effective_to,
    maker_id,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES (
    $1, $2, $3, $4, $5, $6,
    FALSE, 'DRAFT', $7, $8,
    $9, now(), 'infinity'::TIMESTAMPTZ,
    $10,
    now(), $10, now(), $10, 1, $11
)`
	_, err = tx.ExecContext(ctx, hq,
		headerID, eventIDKode, row.EventCode, namaEvent, kategoriEvent, triggerSource,
		workflowPath, regulatedFlag,
		parentID,
		actor,
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.InsertDraftForBulkRow header: %w", err)
	}

	const dq = `
INSERT INTO mst.mapping_jurnal_detail (
    id, event_header_id, urutan,
    akun_debit, akun_kredit, dk_indicator, jumlah_calc,
    created_at, created_by, updated_at, updated_by, row_version, tenant_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, now(), $8, now(), $8, 1, $9)`
	var jumlahCalc *string
	if row.JumlahCalc != "" {
		jumlahCalc = &row.JumlahCalc
	}
	_, err = tx.ExecContext(ctx, dq,
		uuid.New(), headerID, row.Urutan,
		row.AkunDebit, row.AkunKredit, row.DebitKredit, jumlahCalc,
		actor, tenantID,
	)
	if err != nil {
		return fmt.Errorf("repo.InsertDraftForBulkRow detail: %w", err)
	}
	return nil
}

// ─── RPT-19 Coverage ─────────────────────────────────────────────────────────

// GetCoverageReport builds the RPT-19 Mapping Coverage response.
func (r *DBRepository) GetCoverageReport(ctx context.Context, tenantID string) (*CoverageResp, error) {
	const q = `
WITH events AS (
    SELECT DISTINCT event_code, nama_event
    FROM mst.mapping_jurnal_header
    WHERE tenant_id = $1 AND deleted_at IS NULL
),
active AS (
    SELECT h.event_code,
           COUNT(d.id) FILTER (WHERE d.deleted_at IS NULL)               AS active_detail_count,
           COUNT(d.id) FILTER (WHERE d.deleted_at IS NULL AND (d.akun_debit IS NULL OR d.akun_kredit IS NULL)) AS missing_akun_count
    FROM mst.mapping_jurnal_header h
    LEFT JOIN mst.mapping_jurnal_detail d ON d.event_header_id = h.id
    WHERE h.workflow_status = 'APPROVED_ACTIVE'
      AND h.tenant_id = $1
      AND h.deleted_at IS NULL
    GROUP BY h.event_code
),
dlq AS (
    SELECT event_code, MAX(created_at) AS last_dlq_error
    FROM sys.dlq_jurnal_post
    WHERE error_code = 'JURNAL_EVENT_NOT_MAPPED'
      AND tenant_id = $1
    GROUP BY event_code
)
SELECT e.event_code,
       e.nama_event,
       a.active_detail_count,
       a.missing_akun_count,
       dlq.last_dlq_error
FROM events e
LEFT JOIN active a ON a.event_code = e.event_code
LEFT JOIN dlq ON dlq.event_code = e.event_code
ORDER BY e.event_code`

	rows, err := r.db.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("repo.GetCoverageReport: %w", err)
	}
	defer rows.Close()

	var events []CoverageEventP5
	totalEvents, activeEvents, missingEvents := 0, 0, 0

	for rows.Next() {
		var ev CoverageEventP5
		var detailCount, missingCount sql.NullInt64
		var lastDlq sql.NullTime
		if err := rows.Scan(&ev.EventCode, &ev.NamaEvent, &detailCount, &missingCount, &lastDlq); err != nil {
			return nil, fmt.Errorf("repo.GetCoverageReport scan: %w", err)
		}
		totalEvents++
		ev.ActiveDetailCount = int(detailCount.Int64)
		ev.MissingAkunCount = int(missingCount.Int64)
		if lastDlq.Valid {
			ev.LastDlqError = &lastDlq.Time
		}

		if !detailCount.Valid || detailCount.Int64 == 0 {
			ev.GapCoverage = CoverageStatusMissing
			missingEvents++
		} else if missingCount.Valid && missingCount.Int64 > 0 {
			ev.GapCoverage = CoverageStatusIncomplete
		} else {
			ev.GapCoverage = CoverageStatusOK
			activeEvents++
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.GetCoverageReport rows: %w", err)
	}

	return &CoverageResp{
		TotalEvents:   totalEvents,
		ActiveEvents:  activeEvents,
		MissingEvents: missingEvents,
		GapEvents:     events,
	}, nil
}

// ─── RPT-20 Validation ────────────────────────────────────────────────────────

// GetValidationReport validates all APPROVED_ACTIVE mappings (akun non-null + balanced).
func (r *DBRepository) GetValidationReport(ctx context.Context, tenantID string) (*ValidationResp, error) {
	const q = `
SELECT h.id, h.event_code,
       COUNT(d.id) FILTER (WHERE d.deleted_at IS NULL) AS detail_count,
       COUNT(d.id) FILTER (WHERE d.deleted_at IS NULL AND (d.akun_debit IS NULL OR d.akun_kredit IS NULL)) AS null_akun_count,
       COUNT(d.id) FILTER (WHERE d.deleted_at IS NULL AND d.dk_indicator = 'D') AS debit_count,
       COUNT(d.id) FILTER (WHERE d.deleted_at IS NULL AND d.dk_indicator = 'K') AS kredit_count
FROM mst.mapping_jurnal_header h
LEFT JOIN mst.mapping_jurnal_detail d ON d.event_header_id = h.id
WHERE h.workflow_status = 'APPROVED_ACTIVE'
  AND h.tenant_id = $1
  AND h.deleted_at IS NULL
GROUP BY h.id, h.event_code
ORDER BY h.event_code`

	rows, err := r.db.QueryContext(ctx, q, tenantID)
	if err != nil {
		return nil, fmt.Errorf("repo.GetValidationReport: %w", err)
	}
	defer rows.Close()

	var issues []ValidationIssueP5
	total, valid, invalid := 0, 0, 0

	for rows.Next() {
		var headerID, eventCode string
		var detailCount, nullAkunCount, debitCount, kreditCount int
		if err := rows.Scan(&headerID, &eventCode, &detailCount, &nullAkunCount, &debitCount, &kreditCount); err != nil {
			return nil, fmt.Errorf("repo.GetValidationReport scan: %w", err)
		}
		total++
		var errCodes []string
		var detailParts []string

		if nullAkunCount > 0 {
			errCodes = append(errCodes, CodeMappingAkunInvalid)
			detailParts = append(detailParts, fmt.Sprintf("%d detail row(s) dengan akun null", nullAkunCount))
		}
		if debitCount != kreditCount {
			errCodes = append(errCodes, CodeMappingUnbalanced)
			detailParts = append(detailParts, fmt.Sprintf("debit %d ≠ kredit %d lines", debitCount, kreditCount))
		}

		if len(errCodes) > 0 {
			invalid++
			issues = append(issues, ValidationIssueP5{
				EventCode:  eventCode,
				HeaderID:   headerID,
				ErrorCodes: errCodes,
				Details:    joinStr(detailParts, "; "),
			})
		} else {
			valid++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.GetValidationReport rows: %w", err)
	}

	return &ValidationResp{
		TotalActiveMappings: total,
		ValidMappings:       valid,
		InvalidMappings:     invalid,
		Issues:              issues,
	}, nil
}

// ─── RPT-21 History ───────────────────────────────────────────────────────────

// ListMappingHistory returns paginated MAPPING.* audit log entries.
func (r *DBRepository) ListMappingHistory(ctx context.Context, q listquery.Query, filterEventCode string, cursor string, limit int, tenantID string) ([]MappingAuditEntry, *string, bool, error) {
	if limit <= 0 {
		limit = 50
	}

	args := []interface{}{tenantID, limit + 1}
	where := "al.tenant_id = $1 AND al.action LIKE 'MAPPING.%'"
	argIdx := 3

	if filterEventCode != "" {
		where += fmt.Sprintf(" AND al.after_jsonb->>'event_code' = $%d", argIdx)
		args = append(args, filterEventCode)
		argIdx++
	}
	if cursor != "" {
		where += fmt.Sprintf(" AND al.event_time < $%d", argIdx)
		args = append(args, cursor)
	}

	query := fmt.Sprintf(`
SELECT al.event_id, al.event_time, al.actor_user_id, al.actor_role,
       al.action, al.entity_type, al.entity_id,
       al.before_jsonb, al.after_jsonb, al.trace_id
FROM aud.audit_log al
WHERE %s
ORDER BY al.event_time DESC
LIMIT $2`, where)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, false, fmt.Errorf("repo.ListMappingHistory: %w", err)
	}
	defer rows.Close()

	var entries []MappingAuditEntry
	for rows.Next() {
		var e MappingAuditEntry
		var beforeRaw, afterRaw sql.NullString
		if err := rows.Scan(
			&e.EventID, &e.EventTime, &e.ActorUserID, &e.ActorRole,
			&e.Action, &e.EntityType, &e.EntityID,
			&beforeRaw, &afterRaw, &e.TraceID,
		); err != nil {
			return nil, nil, false, fmt.Errorf("repo.ListMappingHistory scan: %w", err)
		}
		if beforeRaw.Valid {
			raw := json.RawMessage(beforeRaw.String)
			e.BeforeJsonb = &raw
		}
		if afterRaw.Valid {
			raw := json.RawMessage(afterRaw.String)
			e.AfterJsonb = &raw
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, err
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}
	var nextCursor *string
	if hasMore && len(entries) > 0 {
		t := entries[len(entries)-1].EventTime.UTC().Format(time.RFC3339Nano)
		nextCursor = &t
	}
	return entries, nextCursor, hasMore, nil
}

// ─── Scan helper ─────────────────────────────────────────────────────────────

// scanP5Header scans a *sql.Row into a *Header with P5-M12 columns.
func (r *DBRepository) scanP5Header(row *sql.Row) (*Header, error) {
	if row == nil {
		return nil, nil
	}
	h := &Header{}
	var (
		workflowPath, workflowStatus string
		makerID, reviewerID, approverID, approver2ID sql.NullString
		parentID                                      sql.NullString
		effectiveFrom, effectiveTo                    sql.NullTime
		reviewerSignedAt, approverSignedAt, approver2SignedAt sql.NullTime
		submitAt                                      sql.NullTime
		reviewerSigHash, approverSigHash, approver2SigHash, stepUpRef []byte
		catatan, commentReview, commentApprove, commentApprove2, rejectReason sql.NullString
		regulatedFlag                                 bool
		updatedAt                                     time.Time
		deletedAt                                     sql.NullTime
	)
	err := row.Scan(
		&h.ID, &h.EventIDKode, &h.EventCode, &h.NamaEvent, &h.KategoriEvent, &h.TriggerSource,
		&h.AktifFlag, &catatan, &workflowStatus, &workflowPath,
		&makerID, &reviewerID, &approverID, &approver2ID,
		&reviewerSignedAt, &reviewerSigHash, &commentReview,
		&approverSignedAt, &approverSigHash, &commentApprove,
		&approver2SignedAt, &approver2SigHash, &commentApprove2,
		&submitAt, &rejectReason,
		&parentID, &effectiveFrom, &effectiveTo, &regulatedFlag, &stepUpRef,
		&h.CreatedAt, &h.CreatedBy, &updatedAt, &h.UpdatedBy, &deletedAt, &h.RowVersion, &h.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scanP5Header: %w", err)
	}
	h.UpdatedAt = updatedAt
	h.WorkflowStatus = WorkflowStatus(workflowStatus)
	h.WorkflowPath = workflowPath
	h.RegulatedFlag = regulatedFlag
	if catatan.Valid {
		h.Catatan = &catatan.String
	}
	// Parse UUID pointer helpers
	parseUUIDPtr := func(ns sql.NullString) *uuid.UUID {
		if !ns.Valid {
			return nil
		}
		u, err := uuid.Parse(ns.String)
		if err != nil {
			return nil
		}
		return &u
	}
	h.MakerID = parseUUIDPtr(makerID)
	h.ReviewerID = parseUUIDPtr(reviewerID)
	h.ApproverID = parseUUIDPtr(approverID)
	h.Approver2ID = parseUUIDPtr(approver2ID)
	if parentID.Valid {
		u, _ := uuid.Parse(parentID.String)
		h.ParentID = &u
	}
	if effectiveFrom.Valid {
		h.EffectiveFrom = &effectiveFrom.Time
	}
	if effectiveTo.Valid {
		h.EffectiveTo = &effectiveTo.Time
	}
	if reviewerSignedAt.Valid {
		h.ReviewerSignedAt = &reviewerSignedAt.Time
	}
	if approverSignedAt.Valid {
		h.ApproverSignedAt = &approverSignedAt.Time
	}
	if approver2SignedAt.Valid {
		h.Approver2SignedAt = &approver2SignedAt.Time
	}
	if submitAt.Valid {
		h.SubmitAt = &submitAt.Time
	}
	if commentReview.Valid {
		h.CommentReview = &commentReview.String
	}
	if commentApprove.Valid {
		h.CommentApprove = &commentApprove.String
	}
	if commentApprove2.Valid {
		h.CommentApprove2 = &commentApprove2.String
	}
	if rejectReason.Valid {
		h.RejectReason = &rejectReason.String
	}
	h.ReviewerSignatureHash = reviewerSigHash
	h.ApproverSignatureHash = approverSigHash
	h.Approver2SignatureHash = approver2SigHash
	h.StepUpTokenRef = stepUpRef
	if deletedAt.Valid {
		h.DeletedAt = &deletedAt.Time
	}
	return h, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// computeSHA256 computes SHA-256 hash of the input string.
func computeSHA256(input string) []byte {
	h := sha256.Sum256([]byte(input))
	return h[:]
}

// joinStr joins non-empty strings with sep.
func joinStr(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// writeAuditP5 writes an audit event in-transaction. Logs on error, never panics.
func writeAuditP5(ctx context.Context, tx *sql.Tx, aw *audit.Writer, evt audit.Event) {
	if aw == nil || tx == nil {
		return
	}
	_ = aw.WithTx(tx).Write(ctx, evt)
}
