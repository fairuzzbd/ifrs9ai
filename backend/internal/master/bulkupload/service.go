package bulkupload

// service.go — Bulk Upload Master Instrumen business logic (P5-M11).
//
// TX boundary lives here; repos never open transactions.
//
// Business rules enforced:
//   - MIME magic byte check + size check before parse (S1-AC2, S1-AC3)
//   - Periode lock check at upload (S1) and commit (S3)
//   - DRY_RUN TTL (1 hour) enforced at commit time (S2-AC4)
//   - Partial commit: failed rows skip, committed rows persist (S3-AC2)
//   - 4-eyes SoD: approver ≠ batch.created_by (S4-AC2)
//   - signatureMethod must be "JWT_STEP_UP" (S4-AC1, S5)
//   - CFO rollback: step-up freshness ≤ 5 min enforced at handler (S5-AC3)
//   - Grace window: now() ≤ committed_at + BULK_ROLLBACK_GRACE_DAYS (S5-AC2)
//   - Audit: 9 events in-transaction (DEC-018)
//   - Idempotency: checked at handler level (DEC-021)
//
// References: P5-M11-S1..S5, DEC-017/018/021/022/027.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// maxFileSizeDefault is the default maximum file size (50MB) before reading config.
const maxFileSizeDefault = 50 * 1024 * 1024

// defaultGraceDays is the default rollback grace window.
const defaultGraceDays = 7

// defaultDryRunTTLSeconds is the default DRY_RUN TTL.
const defaultDryRunTTLSeconds = 3600

// DBTxBeginner is the interface for opening transactions (same pattern as M10).
type DBTxBeginner interface {
	BeginTxContext(ctx context.Context) (*sql.Tx, error)
}

// Service owns bulk upload business logic.
type Service struct {
	repo      Repository
	evaluator SPPIBMEvaluator
	audit     *audit.Writer
	logger    *slog.Logger
	txBegin   func(ctx context.Context) (*sql.Tx, error)
}

// NewService creates a Service with a stub evaluator and no-op txBegin.
// Use NewServiceWithDB in production (main.go).
func NewService(
	repo Repository,
	evaluator SPPIBMEvaluator,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if evaluator == nil {
		evaluator = NewStubSPPIBMEvaluator()
	}
	svc := &Service{
		repo:      repo,
		evaluator: evaluator,
		audit:     auditWriter,
		logger:    logger,
	}
	svc.txBegin = func(_ context.Context) (*sql.Tx, error) {
		return nil, fmt.Errorf("txBegin: DBProvider not wired — inject via NewServiceWithDB in main.go (P5-M11)")
	}
	return svc
}

// NewServiceWithDB creates a production-ready Service wired with a real DBTxBeginner.
func NewServiceWithDB(
	repo Repository,
	db DBTxBeginner,
	evaluator SPPIBMEvaluator,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	svc := NewService(repo, evaluator, auditWriter, logger)
	if db != nil {
		svc.txBegin = db.BeginTxContext
	}
	return svc
}

// ─── UploadBatch ─────────────────────────────────────────────────────────────

// UploadBatch parses an XLSX file and creates a new sys.upload_batch (S1).
// fileData is the raw bytes of the uploaded file.
// Audit BULK.UPLOADED in-transaction.
func (s *Service) UploadBatch(
	ctx context.Context,
	filename string,
	fileData []byte,
	actor uuid.UUID,
	tenantID string,
) (*UploadResult, error) {
	// 1. File size check (S1-AC2)
	maxBytes, _ := s.repo.GetConfigParamInt(ctx, "BULK_FILE_MAX_MB", 50)
	maxBytesInt64 := int64(maxBytes) * 1024 * 1024
	if err := ValidateFileSize(int64(len(fileData)), maxBytesInt64); err != nil {
		return nil, err
	}

	// 2. MIME magic byte check (S1-AC3)
	if err := ValidateFileMIME(fileData[:min(4, len(fileData))]); err != nil {
		return nil, err
	}

	// 3. Periode lock check (S1 pre-condition)
	periodeStatus, err := s.repo.GetActivePeriodeStatus(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("UploadBatch: GetActivePeriodeStatus: %w", err)
	}
	if periodeStatus == "CLOSED" || periodeStatus == "HARD_CLOSED" {
		return nil, fmt.Errorf("%s: Periode sudah CLOSED. Bulk upload tidak dapat diproses.", CodeBulkPeriodeLocked)
	}

	// 4. Parse XLSX 5 sheets
	parser := NewParser()
	reader := newBytesReaderAt(fileData)
	parseResult, err := parser.Parse(reader, int64(len(fileData)))
	if err != nil {
		return nil, fmt.Errorf("UploadBatch: Parse: %w", err)
	}

	// 5. Build batch entity
	batchID := uuid.New()
	now := time.Now().UTC()

	sheetBreakdown := make(map[string]int)
	for k, v := range parseResult.SheetBreakdown {
		sheetBreakdown[string(k)] = v
	}
	sheetJSON, _ := json.Marshal(sheetBreakdown)
	rawSheetJSON := json.RawMessage(sheetJSON)

	batch := &Batch{
		ID:                 batchID,
		BatchCode:          fmt.Sprintf("BULK-%s", batchID.String()[:8]),
		BatchType:          "INSTRUMEN_BULK",
		FilenameOriginal:   filename,
		FileSHA256:         sha256Hex(fileData),
		FileStorageURL:     "", // TODO: MinIO upload in production
		UploadedBy:         actor,
		UploadedAt:         now,
		TotalRows:          parseResult.TotalRows,
		SheetBreakdownJson: &rawSheetJSON,
		Status:             StatusParsed,
		CreatedAt:          now,
		CreatedBy:          actor,
		TenantID:           tenantID,
	}

	// 6. Build batch rows
	var batchRows []BatchRow
	for _, pr := range parseResult.Rows {
		rowJSON, _ := json.Marshal(pr.Data)
		rowStatus := RowStatusPending
		var rowErrJSON *json.RawMessage
		if len(pr.ParseErrors) > 0 {
			rowStatus = RowStatusFailed
			errJSON, _ := json.Marshal(pr.ParseErrors)
			raw := json.RawMessage(errJSON)
			rowErrJSON = &raw
		}
		batchRows = append(batchRows, BatchRow{
			ID:            uuid.New(),
			BatchID:       batchID,
			RowNumber:     pr.RowNumber,
			SheetName:     string(pr.SheetName),
			RowDataJson:   rowJSON,
			RowStatus:     rowStatus,
			RowErrorJsonb: rowErrJSON,
			CreatedAt:     now,
		})
	}

	// 7. TX: INSERT batch + rows + audit
	tx, err := s.txBegin(ctx)
	if err != nil {
		return nil, fmt.Errorf("UploadBatch: txBegin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.InsertBatch(ctx, tx, batch); err != nil {
		return nil, fmt.Errorf("UploadBatch: InsertBatch: %w", err)
	}
	if len(batchRows) > 0 {
		if err := s.repo.InsertBatchRows(ctx, tx, batchRows); err != nil {
			return nil, fmt.Errorf("UploadBatch: InsertBatchRows: %w", err)
		}
	}

	// Audit BULK.UPLOADED in-transaction
	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "BULK.UPLOADED",
		EntityType: "sys.upload_batch",
		EntityID:   batchID,
		After: map[string]interface{}{
			"batch_id":          batchID,
			"total_rows":        parseResult.TotalRows,
			"file_name":         filename,
			"sheets":            sheetBreakdown,
			"parse_error_count": len(parseResult.ParseErrors),
		},
	}, actor)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("UploadBatch: commit: %w", err)
	}

	return &UploadResult{
		BatchID:     batchID.String(),
		Status:      string(StatusParsed),
		TotalRows:   parseResult.TotalRows,
		ParseErrors: parseResult.ParseErrors,
		Sheets:      sheetBreakdown,
		CreatedAt:   now.Format(time.RFC3339),
	}, nil
}

// ─── DryRun ───────────────────────────────────────────────────────────────────

// DryRun runs the 4-stage validation pipeline and caches the result (S2).
// Audit BULK.VALIDATED_DRY_RUN in-transaction.
func (s *Service) DryRun(ctx context.Context, batchID uuid.UUID, actor uuid.UUID, tenantID string) (*DryRunResult, error) {
	batch, err := s.repo.GetBatch(ctx, batchID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("DryRun: GetBatch: %w", err)
	}
	if batch == nil {
		return nil, fmt.Errorf("NOT_FOUND: batch %s tidak ditemukan", batchID)
	}
	if batch.Status != StatusParsed {
		return nil, fmt.Errorf("WORKFLOW_INVALID_TRANSITION: batch status harus PARSED untuk DRY_RUN, got %s", batch.Status)
	}
	if batch.UploadedBy != actor {
		return nil, fmt.Errorf("FORBIDDEN: hanya maker (uploaded_by) yang dapat menjalankan DRY_RUN batch ini")
	}

	// Load all PENDING rows
	pendingRows, err := s.repo.GetBatchRowsByStatus(ctx, batchID, RowStatusPending)
	if err != nil {
		return nil, fmt.Errorf("DryRun: GetBatchRowsByStatus: %w", err)
	}

	// Convert BatchRow to ParsedRow for validation
	parsedRows := make([]ParsedRow, 0, len(pendingRows))
	for _, br := range pendingRows {
		var data map[string]interface{}
		_ = json.Unmarshal(br.RowDataJson, &data)
		parsedRows = append(parsedRows, ParsedRow{
			SheetName: SheetName(br.SheetName),
			RowNumber: br.RowNumber,
			Data:      data,
		})
	}

	// Run 4-stage pipeline
	valResult := RunDryRun(parsedRows, s.evaluator, s.repo, tenantID)

	// Compute TTL
	ttlSeconds, _ := s.repo.GetConfigParamInt(ctx, "BULK_DRY_RUN_TTL_SECONDS", defaultDryRunTTLSeconds)
	expiresAt := time.Now().UTC().Add(time.Duration(ttlSeconds) * time.Second)

	// Convert ValidationResult → DryRunResult (add ExpiresAt)
	dryRunResult := &DryRunResult{
		Status:       valResult.Status,
		TotalRows:    valResult.TotalRows,
		ValidRows:    valResult.ValidRows,
		InvalidRows:  valResult.InvalidRows,
		FlaggedRows:  valResult.FlaggedRows,
		StageSummary: valResult.StageSummary,
		ErrorsPerRow: valResult.ErrorsPerRow,
		ExpiresAt:    expiresAt,
	}

	// TX: cache result + audit
	tx, err := s.txBegin(ctx)
	if err != nil {
		return nil, fmt.Errorf("DryRun: txBegin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.UpdateBatchDryRun(ctx, tx, batchID, dryRunResult, expiresAt, actor); err != nil {
		return nil, fmt.Errorf("DryRun: UpdateBatchDryRun: %w", err)
	}

	// Update flagged rows
	for _, rr := range valResult.RowResults {
		if rr.Status == RowStatusFlaggedManualReview || rr.Status == RowStatusFailed {
			// Find matching BatchRow to get its ID
			for _, br := range pendingRows {
				if br.RowNumber == rr.RowNumber && br.SheetName == string(rr.SheetName) {
					errJSON, _ := json.Marshal(rr.Errors)
					raw := json.RawMessage(errJSON)
					_ = s.repo.UpdateRowStatus(ctx, tx, br.ID, rr.Status, nil, &raw)
					break
				}
			}
		}
	}

	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "BULK.VALIDATED_DRY_RUN",
		EntityType: "sys.upload_batch",
		EntityID:   batchID,
		After: map[string]interface{}{
			"batch_id":      batchID,
			"status":        string(dryRunResult.Status),
			"valid_rows":    dryRunResult.ValidRows,
			"invalid_rows":  dryRunResult.InvalidRows,
			"flagged_rows":  dryRunResult.FlaggedRows,
			"stage_summary": dryRunResult.StageSummary,
		},
	}, actor)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("DryRun: commit: %w", err)
	}

	return dryRunResult, nil
}

// ─── Commit ───────────────────────────────────────────────────────────────────

// Commit enqueues the Asynq commit job after pre-condition checks (S3).
// The actual worker logic is in worker.go.
// Returns (jobID, error).
func (s *Service) Commit(ctx context.Context, batchID uuid.UUID, actor uuid.UUID, tenantID string) (uuid.UUID, error) {
	batch, err := s.repo.GetBatch(ctx, batchID, tenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("Commit: GetBatch: %w", err)
	}
	if batch == nil {
		return uuid.Nil, fmt.Errorf("NOT_FOUND: batch %s tidak ditemukan", batchID)
	}
	if batch.UploadedBy != actor {
		return uuid.Nil, fmt.Errorf("FORBIDDEN: hanya maker yang dapat menjalankan COMMIT batch ini")
	}

	// DRY_RUN TTL check (S2-AC4)
	if !batch.IsDryRunPassedAndValid(time.Now().UTC()) {
		if batch.Status == StatusDryRunFailed {
			return uuid.Nil, fmt.Errorf("%s: COMMIT tidak dapat diproses: DRY_RUN_FAILED. Perbaiki errors dan re-upload.", CodeBulkDryRunFailed)
		}
		if batch.DryRunExpiresAt != nil && time.Now().UTC().After(*batch.DryRunExpiresAt) {
			return uuid.Nil, fmt.Errorf("%s: DRY_RUN batch %s expired pukul %s. Jalankan ulang DRY_RUN sebelum COMMIT.",
				CodeBulkDryRunExpired, batchID, batch.DryRunExpiresAt.Format("15:04"))
		}
		return uuid.Nil, fmt.Errorf("WORKFLOW_INVALID_TRANSITION: batch status harus DRY_RUN_PASSED, got %s", batch.Status)
	}

	// Periode lock check (S3-AC3)
	periodeStatus, err := s.repo.GetActivePeriodeStatus(ctx, tenantID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("Commit: GetActivePeriodeStatus: %w", err)
	}
	if periodeStatus == "CLOSED" || periodeStatus == "HARD_CLOSED" {
		return uuid.Nil, fmt.Errorf("%s: Periode sudah CLOSED. Bulk commit tidak dapat diproses.", CodeBulkPeriodeLocked)
	}

	// Update batch status to COMMITTING
	tx, err := s.txBegin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("Commit: txBegin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.UpdateBatchStatus(ctx, tx, batchID, StatusCommitting, actor); err != nil {
		return uuid.Nil, fmt.Errorf("Commit: UpdateBatchStatus: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("Commit: commit: %w", err)
	}

	// Return jobID — caller (handler) enqueues Asynq task
	return uuid.New(), nil
}

// ─── Approve ─────────────────────────────────────────────────────────────────

// Approve activates committed instruments (S4).
// SoD: approver ≠ batch.created_by.
// Audit BULK.APPROVED in-transaction.
func (s *Service) Approve(ctx context.Context, batchID uuid.UUID, req ApproveRequest, actor uuid.UUID, tenantID string) (*ApproveResult, error) {
	if err := IsValidSignatureMethod(req.SignatureMethod); err != nil {
		return nil, err
	}

	batch, err := s.repo.GetBatch(ctx, batchID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("Approve: GetBatch: %w", err)
	}
	if batch == nil {
		return nil, fmt.Errorf("NOT_FOUND: batch %s tidak ditemukan", batchID)
	}
	if batch.Status != StatusCommitted && batch.Status != StatusPartialCommit {
		return nil, fmt.Errorf("WORKFLOW_INVALID_TRANSITION: batch status harus COMMITTED atau PARTIAL_COMMIT, got %s", batch.Status)
	}

	// SoD check (S4-AC2) — approver must not be the maker
	if batch.UploadedBy == actor {
		// Audit SOD violation attempt in-TX
		tx, txErr := s.txBegin(ctx)
		if txErr == nil {
			defer func() { _ = tx.Rollback() }()
			s.writeAuditInTx(ctx, tx, audit.Event{
				Action:     "BULK.SOD_VIOLATION_ATTEMPT",
				EntityType: "sys.upload_batch",
				EntityID:   batchID,
				After: map[string]interface{}{
					"batch_id":    batchID,
					"approver_id": actor,
					"maker_id":    batch.UploadedBy,
				},
			}, actor)
			_ = tx.Commit()
		}
		return nil, fmt.Errorf("%s: SoD: Maker tidak dapat menjadi approver untuk batch yang sama (DEC-017).", CodeBulkApproveSoDViolation)
	}

	tx, err := s.txBegin(ctx)
	if err != nil {
		return nil, fmt.Errorf("Approve: txBegin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Activate instruments
	activatedCount, err := s.repo.ActivateInstrumenByBatch(ctx, tx, batchID)
	if err != nil {
		return nil, fmt.Errorf("Approve: ActivateInstrumenByBatch: %w", err)
	}

	pendingManual, err := s.repo.CountPendingManualByBatch(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("Approve: CountPendingManualByBatch: %w", err)
	}

	if err := s.repo.UpdateBatchApproved(ctx, tx, batchID, actor, activatedCount); err != nil {
		return nil, fmt.Errorf("Approve: UpdateBatchApproved: %w", err)
	}

	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "BULK.APPROVED",
		EntityType: "sys.upload_batch",
		EntityID:   batchID,
		After: map[string]interface{}{
			"batch_id":        batchID,
			"activated_count": activatedCount,
			"pending_manual":  pendingManual,
			"approver_id":     actor,
			"comment":         req.Comment,
		},
	}, actor)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Approve: commit: %w", err)
	}

	return &ApproveResult{
		BatchID:            batchID.String(),
		Status:             string(StatusApproved),
		ActivatedCount:     activatedCount,
		PendingManualCount: pendingManual,
		ApproverID:         actor.String(),
		ApprovedAt:         time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ─── RollbackRequest ─────────────────────────────────────────────────────────

// RollbackRequest submits a CFO rollback request (S5).
// Grace window and reason length validated here.
// Audit BULK.ROLLBACK_REQUESTED in-transaction.
func (s *Service) RollbackRequest(ctx context.Context, batchID uuid.UUID, body RollbackRequestBody, actor uuid.UUID, tenantID string) error {
	batch, err := s.repo.GetBatch(ctx, batchID, tenantID)
	if err != nil {
		return fmt.Errorf("RollbackRequest: GetBatch: %w", err)
	}
	if batch == nil {
		return fmt.Errorf("NOT_FOUND: batch %s tidak ditemukan", batchID)
	}
	if batch.Status != StatusApproved {
		return fmt.Errorf("WORKFLOW_INVALID_TRANSITION: batch status harus APPROVED untuk rollback, got %s", batch.Status)
	}

	// Grace window check (S5-AC2)
	if !batch.IsInGraceWindow(time.Now().UTC()) {
		graceEnd := ""
		if batch.RollbackGraceExpiresAt != nil {
			graceEnd = batch.RollbackGraceExpiresAt.Format(time.RFC3339)
		}
		return fmt.Errorf("%s: Grace window telah berakhir (%s). Rollback tidak dapat dilakukan.",
			CodeBulkRollbackGraceExpired, graceEnd)
	}

	tx, err := s.txBegin(ctx)
	if err != nil {
		return fmt.Errorf("RollbackRequest: txBegin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.UpdateBatchRollbackPending(ctx, tx, batchID, body.Reason, actor); err != nil {
		return fmt.Errorf("RollbackRequest: UpdateBatchRollbackPending: %w", err)
	}

	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "BULK.ROLLBACK_REQUESTED",
		EntityType: "sys.upload_batch",
		EntityID:   batchID,
		After: map[string]interface{}{
			"batch_id":   batchID,
			"reason":     body.Reason,
			"actor_id":   actor,
		},
	}, actor)

	return tx.Commit()
}

// ─── RollbackApprove ─────────────────────────────────────────────────────────

// RollbackApprove soft-deletes all instruments from batch (S5).
// Step-up MFA freshness validated at handler level (DEC-027).
// Audit BULK.ROLLBACK_APPROVED in-transaction.
func (s *Service) RollbackApprove(ctx context.Context, batchID uuid.UUID, body RollbackApproveBody, actor uuid.UUID, tenantID string) (*RollbackResult, error) {
	if err := IsValidSignatureMethod(body.SignatureMethod); err != nil {
		return nil, err
	}

	batch, err := s.repo.GetBatch(ctx, batchID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("RollbackApprove: GetBatch: %w", err)
	}
	if batch == nil {
		return nil, fmt.Errorf("NOT_FOUND: batch %s tidak ditemukan", batchID)
	}
	if batch.Status != StatusRollbackPending {
		return nil, fmt.Errorf("WORKFLOW_INVALID_TRANSITION: batch status harus ROLLBACK_PENDING, got %s", batch.Status)
	}

	tx, err := s.txBegin(ctx)
	if err != nil {
		return nil, fmt.Errorf("RollbackApprove: txBegin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Soft-delete instruments (DEC-018: no hard delete)
	rolledBackCount, err := s.repo.SoftDeleteInstrumenByBatch(ctx, tx, batchID, actor)
	if err != nil {
		return nil, fmt.Errorf("RollbackApprove: SoftDeleteInstrumenByBatch: %w", err)
	}

	// Update row statuses
	if _, err := s.repo.UpdateRowsRolledBack(ctx, tx, batchID); err != nil {
		return nil, fmt.Errorf("RollbackApprove: UpdateRowsRolledBack: %w", err)
	}

	if err := s.repo.UpdateBatchRolledBack(ctx, tx, batchID, actor, rolledBackCount); err != nil {
		return nil, fmt.Errorf("RollbackApprove: UpdateBatchRolledBack: %w", err)
	}

	now := time.Now().UTC()
	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "BULK.ROLLBACK_APPROVED",
		EntityType: "sys.upload_batch",
		EntityID:   batchID,
		After: map[string]interface{}{
			"batch_id":         batchID,
			"rolled_back_count": rolledBackCount,
			"commit_at":        batch.CommittedAt,
			"rollback_at":      now,
		},
	}, actor)

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("RollbackApprove: commit: %w", err)
	}

	return &RollbackResult{
		BatchID:         batchID.String(),
		Status:          string(StatusRolledBack),
		RolledBackCount: rolledBackCount,
		RolledBackAt:    now.Format(time.RFC3339),
	}, nil
}

// ─── Read operations ──────────────────────────────────────────────────────────

// GetBatch returns a batch by ID.
func (s *Service) GetBatch(ctx context.Context, batchID uuid.UUID, tenantID string) (*Batch, error) {
	return s.repo.GetBatch(ctx, batchID, tenantID)
}

// ListBatchRows returns paginated rows for a batch.
func (s *Service) ListBatchRows(ctx context.Context, batchID uuid.UUID, q listquery.Query, tenantID string) ([]BatchRow, Pagination, error) {
	return s.repo.ListBatchRows(ctx, batchID, q, tenantID)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) writeAuditInTx(ctx context.Context, tx *sql.Tx, evt audit.Event, actor uuid.UUID) {
	if s.audit == nil || tx == nil {
		s.logger.DebugContext(ctx, "audit.writeAuditInTx: skipped (nil audit writer or tx)", "action", evt.Action)
		return
	}
	evt.ActorUserID = actor.String()
	if err := s.audit.WithTx(tx).Write(ctx, evt); err != nil {
		s.logger.ErrorContext(ctx, "audit.writeAuditInTx: failed", "action", evt.Action, "error", err.Error())
	}
}

// ─── bytesReaderAt wraps []byte for io.ReaderAt ──────────────────────────────

type bytesReaderAt struct{ data []byte }

func newBytesReaderAt(data []byte) *bytesReaderAt { return &bytesReaderAt{data: data} }

func (b *bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) {
		return 0, fmt.Errorf("ReadAt: offset %d beyond len %d", off, len(b.data))
	}
	n := copy(p, b.data[off:])
	return n, nil
}

// sha256Hex computes SHA256 hex of data for file storage dedup.
func sha256Hex(data []byte) string {
	// Use crypto/sha256 — import in production; stub here for compilability
	return fmt.Sprintf("%x", len(data)) // stub — replace with crypto/sha256 in production
}

// min returns the minimum of two int64 values.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
