package lpscoverage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// dateRe validates YYYY-MM-DD date strings.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// defaultCoverageAmount is IDR 2_000_000_000 per DEC-014.
var defaultCoverageAmount = decimal.NewFromInt(2_000_000_000)

// rollbackTx rolls back a transaction, logging (but ignoring) errors.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "lpscoverage service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for mst.lps_coverage.
// Transaction boundaries are managed here; repos are called with a tx when inside one.
type Service struct {
	repo        Repository
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewService constructs a Service.
func NewService(repo Repository, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, auditWriter: auditWriter, logger: logger}
}

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new LPSCoverage record in DRAFT state.
// Writes audit LPS_COVERAGE.CREATE in the same transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*LPSCoverage, error) {
	amount, err := s.validateCreate(req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Period overlap check: no APPROVED row may overlap the proposed range.
	overlapCount, err := s.repo.CountOverlap(ctx, req.PeriodeBerlakuDari, req.PeriodeBerlakuSampai, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("service.Create overlap check: %w", err)
	}
	if overlapCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeLPSPeriodOverlap,
			"Periode berlaku bertumpang-tindih dengan record LPS Coverage yang sudah aktif (APPROVED). "+
				"Tutup periode record lama terlebih dahulu dengan mengisi periode_berlaku_sampai sebelum membuat record baru.",
			domainerrors.Detail{
				Field:   "body.periodeBerlakuDari",
				Rule:    "period_overlap",
				Message: "Terdapat record APPROVED dengan periode yang tumpang-tindih",
			},
		)
	}

	now := time.Now()
	lc := &LPSCoverage{
		ID:                   uuid.New(),
		CoverageAmount:       amount,
		MataUang:             "IDR", // always IDR per DEC-014
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		RegulasiReferensi:    req.RegulasiReferensi,
		WorkflowStatus:       WorkflowStatusDraft,
		CreatedAt:            now,
		CreatedBy:            &actorID,
		MakerID:              actorID,
		RowVersion:           1,
		TenantID:             tenantID(claims),
	}

	if req.DokumenPendukungID != nil {
		docID, parseErr := uuid.Parse(*req.DokumenPendukungID)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"dokumenPendukungId bukan UUID valid",
				domainerrors.Detail{Field: "body.dokumenPendukungId", Rule: "format", Message: "Harus berupa UUID"},
			)
		}
		lc.DokumenPendukungID = &docID
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, lc); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "LPS_COVERAGE.CREATE",
		EntityType: "mst.lps_coverage",
		EntityID:   lc.ID,
		After:      lc,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}
	return lc, nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetByID fetches one record, returning domainerrors.ErrNotFound if absent.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*LPSCoverage, error) {
	lc, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if lc == nil {
		return nil, domainerrors.ErrNotFound("LPS Coverage " + id.String())
	}
	return lc, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*LPSCoverage
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List: %w", err)
	}

	fetchedCount := len(items)
	lastID := ""
	if fetchedCount > limit {
		items = items[:limit]
		lastID = items[limit-1].ID.String()
	}

	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies a partial update to an existing record.
// Guard: workflow_status MUST be DRAFT or RETURNED.
// Guard: row_version optimistic lock.
// Overlap guard: the updated period must not overlap any APPROVED row.
// Writes audit LPS_COVERAGE.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*LPSCoverage, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("LPS Coverage " + id.String())
	}

	if !current.WorkflowStatus.IsEditable() {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("LPS Coverage %s sudah dalam status %s dan tidak bisa diedit langsung. "+
				"Ajukan perubahan melalui workflow.", id, current.WorkflowStatus),
		)
	}

	// Compute proposed date range for overlap check.
	proposedDari := current.PeriodeBerlakuDari
	if req.PeriodeBerlakuDari != nil {
		proposedDari = *req.PeriodeBerlakuDari
	}
	proposedSampai := current.PeriodeBerlakuSampai
	clearSampai := false
	if req.PeriodeBerlakuSampai != nil {
		if *req.PeriodeBerlakuSampai == "" {
			proposedSampai = nil
			clearSampai = true
		} else {
			proposedSampai = req.PeriodeBerlakuSampai
		}
	}

	// Validate proposed date ordering.
	if proposedSampai != nil && *proposedSampai < proposedDari {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"periode_berlaku_sampai tidak boleh lebih awal dari periode_berlaku_dari",
			domainerrors.Detail{Field: "body.periodeBerlakuSampai", Rule: "date_order", Message: "Sampai harus >= dari"},
		)
	}

	// Overlap check — exclude self.
	overlapCount, err := s.repo.CountOverlap(ctx, proposedDari, proposedSampai, id)
	if err != nil {
		return nil, fmt.Errorf("service.Update overlap check: %w", err)
	}
	if overlapCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeLPSPeriodOverlap,
			"Periode yang diusulkan bertumpang-tindih dengan record LPS Coverage APPROVED yang lain.",
			domainerrors.Detail{
				Field:   "body.periodeBerlakuDari",
				Rule:    "period_overlap",
				Message: "Terdapat record APPROVED dengan periode yang tumpang-tindih",
			},
		)
	}

	before := *current

	fields := UpdateFields{
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
		ClearSampai:     clearSampai,
	}
	if req.CoverageAmount != nil {
		amt, parseErr := decimal.NewFromString(*req.CoverageAmount)
		if parseErr != nil || !amt.IsPositive() {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"coverageAmount harus angka positif",
				domainerrors.Detail{Field: "body.coverageAmount", Rule: "positive", Message: "Harus > 0"},
			)
		}
		fields.CoverageAmount = &amt
	}
	if req.PeriodeBerlakuDari != nil {
		fields.PeriodeBerlakuDari = req.PeriodeBerlakuDari
	}
	if !clearSampai && req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai != "" {
		fields.PeriodeBerlakuSampai = req.PeriodeBerlakuSampai
	}
	if req.RegulasiReferensi != nil {
		fields.RegulasiReferensi = req.RegulasiReferensi
	}
	if req.DokumenPendukungID != nil {
		docID, parseErr := uuid.Parse(*req.DokumenPendukungID)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"dokumenPendukungId bukan UUID valid",
				domainerrors.Detail{Field: "body.dokumenPendukungId", Rule: "format", Message: "Harus berupa UUID"},
			)
		}
		fields.DokumenPendukungID = &docID
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("LPS Coverage " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "LPS_COVERAGE.UPDATE",
		EntityType: "mst.lps_coverage",
		EntityID:   updated.ID,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
// Guard: referential integrity (no active ECL references).
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("LPS Coverage " + id.String())
	}

	refCount, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("LPS Coverage %s tidak bisa dihapus karena direferensikan oleh %d entitas aktif.", id, refCount),
			domainerrors.Detail{Field: "id", Rule: "referenced_by", Message: fmt.Sprintf("Direferensikan oleh %d entitas", refCount)},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "LPS_COVERAGE.DELETE",
		EntityType: "mst.lps_coverage",
		EntityID:   id,
		Before:     existing,
		After:      deleted,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete: commit: %w", err)
	}
	return nil
}

// ─── Workflow sync ────────────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine after a state transition.
// It updates mst.lps_coverage.workflow_status to stay in sync with sys.workflow_instance.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	if _, err := requireActor(claims); err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	lc, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if lc == nil {
		return domainerrors.ErrNotFound("LPS Coverage entity " + entityID.String())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatusTx(ctx, tx, entityID, wfStatus); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	auditAction := "LPS_COVERAGE." + action
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.lps_coverage",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(lc.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": string(wfStatus)},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: commit: %w", err)
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given entity ID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("LPS Coverage " + id.String())
	}
	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit LPS_COVERAGE.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, 0, err
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV: %w", err)
	}

	// Best-effort audit write.
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "LPS_COVERAGE.EXPORT",
			EntityType:  "mst.lps_coverage",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "lpscoverage ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "lpscoverage ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

// validateCreate runs field-level validation for create.
func (s *Service) validateCreate(req CreateRequest) (decimal.Decimal, error) {
	var details []domainerrors.Detail

	// Parse and validate coverage_amount
	var amount decimal.Decimal
	if req.CoverageAmount == "" {
		// Default to IDR 2 billion per DEC-014.
		amount = defaultCoverageAmount
	} else {
		parsed, parseErr := decimal.NewFromString(req.CoverageAmount)
		if parseErr != nil {
			details = append(details, domainerrors.Detail{
				Field:   "body.coverageAmount",
				Rule:    "format",
				Message: "coverageAmount harus berupa angka desimal valid",
			})
		} else if !parsed.IsPositive() {
			details = append(details, domainerrors.Detail{
				Field:   "body.coverageAmount",
				Rule:    "positive",
				Message: "coverageAmount harus lebih besar dari 0",
			})
		} else {
			amount = parsed
		}
	}

	if !dateRe.MatchString(req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "periodeBerlakuDari harus dalam format YYYY-MM-DD",
		})
	}

	if req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai != "" {
		if !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "format",
				Message: "periodeBerlakuSampai harus dalam format YYYY-MM-DD",
			})
		} else if *req.PeriodeBerlakuSampai < req.PeriodeBerlakuDari {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "date_order",
				Message: "periodeBerlakuSampai tidak boleh lebih awal dari periodeBerlakuDari",
			})
		}
	}

	if req.RegulasiReferensi != nil && len(*req.RegulasiReferensi) > 200 {
		details = append(details, domainerrors.Detail{
			Field:   "body.regulasiReferensi",
			Rule:    "max_length",
			Message: "regulasiReferensi maksimal 200 karakter",
		})
	}

	if len(details) > 0 {
		return decimal.Zero, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return amount, nil
}

// validateUpdate runs field-level validation for update.
func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
		})
	}

	if req.PeriodeBerlakuDari != nil && !dateRe.MatchString(*req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "periodeBerlakuDari harus dalam format YYYY-MM-DD",
		})
	}

	if req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai != "" && !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuSampai",
			Rule:    "format",
			Message: "periodeBerlakuSampai harus dalam format YYYY-MM-DD",
		})
	}

	if req.RegulasiReferensi != nil && len(*req.RegulasiReferensi) > 200 {
		details = append(details, domainerrors.Detail{
			Field:   "body.regulasiReferensi",
			Rule:    "max_length",
			Message: "regulasiReferensi maksimal 200 karakter",
		})
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// requireActor extracts the actor UUID from JWT claims.
func requireActor(claims *auth.Claims) (uuid.UUID, error) {
	if claims == nil || claims.Sub == "" {
		return uuid.Nil, domainerrors.ErrUnauthorized("Claims tidak ditemukan.")
	}
	id, err := uuid.Parse(claims.Sub)
	if err != nil {
		return uuid.Nil, domainerrors.ErrUnauthorized("Sub claim bukan UUID valid.")
	}
	return id, nil
}

// tenantID extracts tenant_id from claims, defaulting to TUGURE.
func tenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

// mapWorkflowState converts workflow engine state string to LPSCoverage WorkflowStatus.
func mapWorkflowState(state string) WorkflowStatus {
	switch state {
	case "DRAFT":
		return WorkflowStatusDraft
	case "PENDING_REVIEW":
		return WorkflowStatusPendingReview
	case "PENDING_APPROVAL":
		return WorkflowStatusPendingApproval
	case "PENDING_APPROVAL_2":
		return WorkflowStatusPendingApproval2
	case "APPROVED":
		return WorkflowStatusApproved
	case "REJECTED":
		return WorkflowStatusRejected
	default:
		return WorkflowStatus(state)
	}
}
