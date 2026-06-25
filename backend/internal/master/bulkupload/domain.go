// Package bulkupload implements APP-A bulk upload of master instrumen (P5-M11).
//
// Flow: upload XLSX → parse 5 sheets → DRY_RUN 4-stage validation → async commit → approve → [rollback]
//
// All amounts: shopspring/decimal (DEC-016).
// Audit in-transaction for all 9 events (DEC-018).
// Idempotency-Key on all 5 mutating endpoints (DEC-021).
// Cursor-based pagination (DEC-022).
// SoD: approver ≠ maker (DEC-017).
// Step-up MFA for rollback-approve (DEC-027).
//
// References: FSD-BLIPS-MASTER-v1.1 §3, FSD-APP-A, P5-M11-S1..S5.
package bulkupload

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	CodeBulkFileTooLarge         = "BULK_FILE_TOO_LARGE"
	CodeBulkMimeInvalid          = "BULK_MIME_INVALID"
	CodeBulkDryRunExpired        = "BULK_DRY_RUN_EXPIRED"
	CodeBulkDryRunFailed         = "BULK_DRY_RUN_FAILED"
	CodeBulkPeriodeLocked        = "BULK_PERIODE_LOCKED"
	CodeBulkRollbackGraceExpired = "BULK_ROLLBACK_GRACE_EXPIRED"
	CodeBulkApproveSoDViolation  = "BULK_APPROVE_SOD_VIOLATION"
)

// ─── Batch status enum ────────────────────────────────────────────────────────

type BatchStatus string

const (
	StatusParsed          BatchStatus = "PARSED"
	StatusDryRunPassed    BatchStatus = "DRY_RUN_PASSED"
	StatusDryRunFailed    BatchStatus = "DRY_RUN_FAILED"
	StatusCommitting      BatchStatus = "COMMITTING"
	StatusCommitted       BatchStatus = "COMMITTED"
	StatusPartialCommit   BatchStatus = "PARTIAL_COMMIT"
	StatusApproved        BatchStatus = "APPROVED"
	StatusRollbackPending BatchStatus = "ROLLBACK_PENDING"
	StatusRolledBack      BatchStatus = "ROLLED_BACK"
)

// ─── Row status enum ──────────────────────────────────────────────────────────

type RowStatus string

const (
	RowStatusPending             RowStatus = "PENDING"
	RowStatusCommitted           RowStatus = "COMMITTED"
	RowStatusFailed              RowStatus = "FAILED"
	RowStatusRolledBack          RowStatus = "ROLLED_BACK"
	RowStatusFlaggedManualReview RowStatus = "FLAGGED_MANUAL_REVIEW"
)

// ─── Sheet enum ───────────────────────────────────────────────────────────────

type SheetName string

const (
	SheetDeposito   SheetName = "Deposito"
	SheetObligasi   SheetName = "Obligasi"
	SheetSaham      SheetName = "Saham"
	SheetReksadana  SheetName = "Reksadana"
	SheetTabungan   SheetName = "Tabungan_Cash"
)

var ValidSheets = []SheetName{
	SheetDeposito, SheetObligasi, SheetSaham, SheetReksadana, SheetTabungan,
}

// MandatoryColumns defines required columns per sheet for Stage 1 validation.
var MandatoryColumns = map[SheetName][]string{
	SheetDeposito:  {"kode", "counterparty_id", "bank_id", "mata_uang", "saldo", "tanggal_penempatan", "jatuh_tempo", "bunga"},
	SheetObligasi:  {"kode", "issuer_id", "mata_uang", "nilai_nominal", "kupon", "tanggal_penerbitan", "jatuh_tempo"},
	SheetSaham:     {"kode", "emiten_id", "mata_uang", "jumlah_lembar", "harga_beli"},
	SheetReksadana: {"kode", "manajer_id", "mata_uang", "nilai_investasi", "tanggal_investasi"},
	SheetTabungan:  {"kode", "bank_id", "mata_uang", "saldo", "tanggal_penempatan"},
}

// ─── Batch domain entity ──────────────────────────────────────────────────────

// Batch is the domain entity for sys.upload_batch (INSTRUMEN_BULK).
type Batch struct {
	ID                     uuid.UUID        `db:"id"`
	BatchCode              string           `db:"batch_code"`
	BatchType              string           `db:"batch_type"` // always 'INSTRUMEN_BULK'
	FilenameOriginal       string           `db:"filename_original"`
	FileSHA256             string           `db:"file_sha256"`
	FileStorageURL         string           `db:"file_storage_url"`
	UploadedBy             uuid.UUID        `db:"uploaded_by"`
	UploadedAt             time.Time        `db:"uploaded_at"`
	TotalRows              int              `db:"total_rows"`
	ValidRows              int              `db:"valid_rows"`
	CommittedRows          int              `db:"committed_rows"`
	FailedRows             int              `db:"failed_rows"`
	SheetBreakdownJson     *json.RawMessage `db:"sheet_breakdown_json"`
	Status                 BatchStatus      `db:"status"`
	ApproverID             *uuid.UUID       `db:"approver_id"`
	ApprovedAt             *time.Time       `db:"approved_at"`
	CommittedAt            *time.Time       `db:"committed_at"`
	DryRunCachedAt         *time.Time       `db:"dry_run_cached_at"`
	DryRunExpiresAt        *time.Time       `db:"dry_run_expires_at"`
	DryRunResultJsonb      *json.RawMessage `db:"dry_run_result_jsonb"`
	RollbackStatus         *string          `db:"rollback_status"`
	RollbackGraceExpiresAt *time.Time       `db:"rollback_grace_expires_at"`
	RollbackBy             *uuid.UUID       `db:"rollback_by"`
	RollbackAt             *time.Time       `db:"rollback_at"`
	RollbackReason         *string          `db:"rollback_reason"`
	// Audit columns
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	TenantID   string     `db:"tenant_id"`
}

// IsApproved returns true if batch is in APPROVED status.
func (b *Batch) IsApproved() bool { return b.Status == StatusApproved }

// IsDryRunPassedAndValid returns true if dry_run_expires_at is still in the future.
func (b *Batch) IsDryRunPassedAndValid(now time.Time) bool {
	if b.Status != StatusDryRunPassed {
		return false
	}
	if b.DryRunExpiresAt == nil {
		return false
	}
	return now.Before(*b.DryRunExpiresAt)
}

// IsInGraceWindow returns true if rollback is still allowed.
func (b *Batch) IsInGraceWindow(now time.Time) bool {
	if b.RollbackGraceExpiresAt == nil {
		return false
	}
	return now.Before(*b.RollbackGraceExpiresAt)
}

// ─── BatchRow domain entity ───────────────────────────────────────────────────

// BatchRow is the domain entity for sys.upload_batch_row (INSTRUMEN_BULK context).
type BatchRow struct {
	ID              uuid.UUID        `db:"id"`
	BatchID         uuid.UUID        `db:"batch_id"`
	RowNumber       int              `db:"row_number"`
	SheetName       string           `db:"sheet_name"`
	RowDataJson     json.RawMessage  `db:"row_data_json"`
	RowStatus       RowStatus        `db:"row_status"`
	BulkInstrumenID *uuid.UUID       `db:"bulk_instrumen_id"` // populated after commit
	RowErrorJsonb   *json.RawMessage `db:"row_error_jsonb"`
	// Audit
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// ─── Parsed Row ───────────────────────────────────────────────────────────────

// ParsedRow holds data parsed from a single XLSX row.
type ParsedRow struct {
	SheetName   SheetName
	RowNumber   int
	Data        map[string]interface{} // raw cell values keyed by column header
	ParseErrors []RowError             // non-nil = row has parse issues (still inserted as FAILED)
}

// ─── ParseResult ─────────────────────────────────────────────────────────────

// ParseResult is the output of the XLSX 5-sheet parser.
type ParseResult struct {
	Rows           []ParsedRow
	SheetBreakdown map[SheetName]int
	ParseErrors    []RowError
	TotalRows      int
}

// ─── Validation types ─────────────────────────────────────────────────────────

// RowError captures one error for a specific row.
type RowError struct {
	Sheet  SheetName `json:"sheet"`
	Row    int       `json:"row"`
	Stage  int       `json:"stage,omitempty"` // 1-4
	Col    string    `json:"col,omitempty"`
	Error  string    `json:"error"`
}

// DryRunResult is the cached result of the 4-stage validation pipeline.
type DryRunResult struct {
	Status      BatchStatus      `json:"status"`
	TotalRows   int              `json:"totalRows"`
	ValidRows   int              `json:"validRows"`
	InvalidRows int              `json:"invalidRows"`
	FlaggedRows int              `json:"flaggedRows"`
	StageSummary StageSummary   `json:"stageSummary"`
	ErrorsPerRow []RowError     `json:"errorsPerRow"`
	ExpiresAt   time.Time       `json:"expiresAt"`
}

// StageSummary holds per-stage results for DRY_RUN.
type StageSummary struct {
	Stage1 StageResult `json:"stage1"`
	Stage2 StageResult `json:"stage2"`
	Stage3 StageResult `json:"stage3"`
	Stage4 Stage4Result `json:"stage4"`
}

// StageResult holds pass/fail for a validation stage.
type StageResult struct {
	Status     string `json:"status"` // "PASS" or "FAIL"
	ErrorCount int    `json:"errorCount"`
}

// Stage4Result holds Stage 4 SPPI+BM evaluation results.
type Stage4Result struct {
	Status                 string `json:"status"` // "PASS", "PARTIAL", "UNAVAILABLE"
	Evaluated              int    `json:"evaluated"`
	Classified             int    `json:"classified"`
	Flagged                int    `json:"flagged"`
	SppiServiceUnavailable bool   `json:"sppiServiceUnavailable"`
}

// ─── Request/Response types ───────────────────────────────────────────────────

// UploadResult is returned from POST /bulk-upload.
type UploadResult struct {
	BatchID     string           `json:"batchId"`
	Status      string           `json:"status"`
	TotalRows   int              `json:"totalRows"`
	ParseErrors []RowError       `json:"parseErrors"`
	Sheets      map[string]int   `json:"sheets"`
	CreatedAt   string           `json:"createdAt"`
}

// CommitJobPayload is the Asynq task payload for bulkupload:commit_instrumen.
type CommitJobPayload struct {
	BatchID  string `json:"batch_id"`
	ActorID  string `json:"actor_id"`
	TenantID string `json:"tenant_id"`
	JobID    string `json:"job_id"`
}

// ApproveRequest is the body for POST /approve.
type ApproveRequest struct {
	Comment         string `json:"comment"         binding:"required,min=10"`
	SignatureMethod  string `json:"signatureMethod"  binding:"required"`
}

// ApproveResult is returned from POST /approve.
type ApproveResult struct {
	BatchID            string    `json:"batchId"`
	Status             string    `json:"status"`
	ActivatedCount     int       `json:"activatedCount"`
	PendingManualCount int       `json:"pendingManualCount"`
	ApproverID         string    `json:"approverId"`
	ApprovedAt         string    `json:"approvedAt"`
}

// RollbackRequestBody is the body for POST /rollback-request.
type RollbackRequestBody struct {
	Reason string `json:"reason" binding:"required,min=50"`
}

// RollbackApproveBody is the body for POST /rollback-approve.
type RollbackApproveBody struct {
	Comment        string `json:"comment"        binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod" binding:"required"`
}

// RollbackResult is returned from POST /rollback-approve.
type RollbackResult struct {
	BatchID         string    `json:"batchId"`
	Status          string    `json:"status"`
	RolledBackCount int       `json:"rolledBackCount"`
	RolledBackAt    string    `json:"rolledBackAt"`
}

// ─── SPPI+BM evaluator interface (Phase 3 stub) ───────────────────────────────

// KlasifikasiResult holds the classification result from Phase 3 SPPI+BM auto-eval.
type KlasifikasiResult struct {
	KlasifikasiPsak71 *string // nil if ambiguous
	SppiResult        string
	BmResult          string
	Ambiguous         bool
	FlagReason        string
}

// SPPIBMEvaluator is the interface for Phase 3 SPPI+BM auto-eval.
// Production wiring in main.go via Phase 3 service.
// P5-M11 stub returns default AC for all rows.
type SPPIBMEvaluator interface {
	Evaluate(sheetName SheetName, rowData map[string]interface{}) (*KlasifikasiResult, error)
}

// ─── Pagination ───────────────────────────────────────────────────────────────

// Pagination is the cursor-based pagination result (DEC-022).
type Pagination struct {
	NextCursor    *string
	HasMore       bool
	TotalEstimate *int64
	Limit         int
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// SheetBreakdownMap converts SheetBreakdown to map[string]int for JSON.
func SheetBreakdownMap(rows []ParsedRow) map[SheetName]int {
	m := make(map[SheetName]int)
	for _, r := range rows {
		m[r.SheetName]++
	}
	return m
}

// IsValidSignatureMethod validates that signatureMethod is "JWT_STEP_UP".
func IsValidSignatureMethod(method string) error {
	if method != "JWT_STEP_UP" {
		return fmt.Errorf("VALIDATION_FAILED: signatureMethod harus 'JWT_STEP_UP', got %q", method)
	}
	return nil
}
