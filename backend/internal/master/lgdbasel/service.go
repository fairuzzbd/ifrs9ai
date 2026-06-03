package lgdbasel

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

// rollbackTx is a helper that attempts to rollback a transaction.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "lgdbasel service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for lgd_basel.
// It manages transaction boundaries; repo methods must be called with a tx when inside one.
//
// ECL parameter module — subject to BLOCKING ifrs9-compliance-reviewer gate on every PR.
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

// dateRe validates YYYY-MM-DD date string.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// lgdMin and lgdMax are the inclusive bounds for LGD (DEC-016 + DB CHECK constraint).
var (
	lgdMin = decimal.NewFromInt(0)
	lgdMax = decimal.NewFromInt(1)
)

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new LGDBasel record in DRAFT state.
// Writes audit LGD_BASEL.CREATE in the same transaction.
//
// Legacy note: maker_id in DB is NOT NULL and predates the workflow engine.
// Service sets maker_id = currentUser.ID to satisfy the constraint. The canonical
// source of truth for the approval chain is sys.workflow_instance.maker_id.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*LGDBasel, error) {
	lgdDec, err := s.validateCreate(req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	sumber := "BASEL_III_IRB"
	if req.Sumber != nil && *req.Sumber != "" {
		sumber = *req.Sumber
	}

	var dokumenID *uuid.UUID
	if req.DokumenPendukungID != nil && *req.DokumenPendukungID != "" {
		uid, parseErr := uuid.Parse(*req.DokumenPendukungID)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"dokumenPendukungId bukan UUID valid.",
				domainerrors.Detail{Field: "body.dokumenPendukungId", Rule: "uuid"},
			)
		}
		dokumenID = &uid
	}

	now := time.Now()
	e := &LGDBasel{
		ID:                   uuid.New(),
		TipeEksposur:         TipeEksposur(req.TipeEksposur),
		LGD:                  lgdDec,
		Karakteristik:        req.Karakteristik,
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		Sumber:               sumber,
		DokumenPendukungID:   dokumenID,
		// Legacy columns: set maker_id = currentUser for DB NOT NULL constraint.
		// sys.workflow_instance is the source of truth for the approval chain.
		MakerID:        actorID,
		ApproverID:     nil,
		WorkflowStatus: WorkflowStatusDraft,
		CreatedAt:      now,
		CreatedBy:      &actorID,
		RowVersion:     1,
		TenantID:       tenantID(claims),
	}

	// Overlap check: warn (422) if same tipe_eksposur has overlapping period.
	overlapCount, err := s.repo.CountOverlap(ctx, e.TipeEksposur, e.PeriodeBerlakuDari, e.PeriodeBerlakuSampai, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("service.Create overlap check: %w", err)
	}
	if overlapCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeLGDPeriodOverlap,
			fmt.Sprintf("Tipe eksposur %s sudah memiliki %d entri dengan periode yang overlap. "+
				"Verifikasi dengan ALCO sebelum lanjut.", req.TipeEksposur, overlapCount),
			domainerrors.Detail{
				Field:   "body.periodeBerlakuDari",
				Rule:    "no_overlap",
				Message: fmt.Sprintf("%d entri dengan periode overlapping ditemukan", overlapCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, e); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "LGD_BASEL.CREATE",
		EntityType: "mst.lgd_basel",
		EntityID:   e.ID,
		After:      e,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}
	return e, nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetByID fetches one record, returning ErrNotFound if absent.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*LGDBasel, error) {
	e, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if e == nil {
		return nil, domainerrors.ErrNotFound("LGD Basel " + id.String())
	}
	return e, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*LGDBasel
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
// Guard: workflow_status MUST be DRAFT or RETURNED (REJECTED at DB level).
// Guard: row_version optimistic lock.
// Writes audit LGD_BASEL.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*LGDBasel, error) {
	lgdDec, err := s.validateUpdate(req)
	if err != nil {
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
		return nil, domainerrors.ErrNotFound("LGD Basel " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("LGD Basel %s sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan amandemen melalui workflow.", id.String()),
		)
	}

	before := *current

	// Determine the effective period for overlap check.
	effectiveTipe := current.TipeEksposur
	if req.TipeEksposur != nil {
		effectiveTipe = TipeEksposur(*req.TipeEksposur)
	}
	effectiveDari := current.PeriodeBerlakuDari
	if req.PeriodeBerlakuDari != nil {
		effectiveDari = *req.PeriodeBerlakuDari
	}
	effectiveSampai := current.PeriodeBerlakuSampai
	if req.PeriodeBerlakuSampai != nil {
		effectiveSampai = req.PeriodeBerlakuSampai
	}

	overlapCount, err := s.repo.CountOverlap(ctx, effectiveTipe, effectiveDari, effectiveSampai, id)
	if err != nil {
		return nil, fmt.Errorf("service.Update overlap check: %w", err)
	}
	if overlapCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeLGDPeriodOverlap,
			fmt.Sprintf("Tipe eksposur %s sudah memiliki %d entri dengan periode yang overlap setelah pembaruan ini.", effectiveTipe, overlapCount),
			domainerrors.Detail{
				Field:   "body.periodeBerlakuDari",
				Rule:    "no_overlap",
				Message: fmt.Sprintf("%d entri dengan periode overlapping", overlapCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	var tipePtr *TipeEksposur
	if req.TipeEksposur != nil {
		t := TipeEksposur(*req.TipeEksposur)
		tipePtr = &t
	}

	fields := UpdateFields{
		TipeEksposur:         tipePtr,
		LGD:                  lgdDec,
		Karakteristik:        req.Karakteristik,
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		Sumber:               req.Sumber,
		UpdatedBy:            actorID,
		ExpectedVersion:      req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("LGD Basel " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "LGD_BASEL.UPDATE",
		EntityType: "mst.lgd_basel",
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
// Guard: active ECL calc-result lines reference this record → ENTITY_IN_USE (409).
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
		return domainerrors.ErrNotFound("LGD Basel " + id.String())
	}

	// Guard: referential integrity via ECL calc lines.
	refCount, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("LGD Basel %s tidak bisa dihapus karena masih direferensikan oleh %d calc result line. "+
				"Tutup atau archive calc run terkait terlebih dahulu.", id.String(), refCount),
			domainerrors.Detail{
				Field:   "id",
				Rule:    "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d ECL calc result line", refCount),
			},
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
		Action:     "LGD_BASEL.DELETE",
		EntityType: "mst.lgd_basel",
		EntityID:   deleted.ID,
		Before:     existing,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete: commit: %w", err)
	}
	return nil
}

// ─── Workflow status sync ─────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine callback after a state transition.
// It updates mst.lgd_basel.workflow_status to stay in sync with sys.workflow_instance.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	e, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if e == nil {
		return domainerrors.ErrNotFound("LGD Basel entity")
	}

	auditAction := "LGD_BASEL." + action

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.lgd_basel",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(e.WorkflowStatus)},
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

// ListHistory returns paginated audit log for a given lgd_basel ID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("LGD Basel " + id.String())
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit LGD_BASEL.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV: %w", err)
	}

	// Audit write best-effort (export is read-only).
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "LGD_BASEL.EXPORT",
			EntityType:  "mst.lgd_basel",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "lgdbasel ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "lgdbasel ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

// validateCreate runs all field-level + cross-field validation for create.
// Returns the parsed LGD decimal on success.
func (s *Service) validateCreate(req CreateRequest) (decimal.Decimal, error) {
	var details []domainerrors.Detail

	// tipe_eksposur whitelist.
	if !IsValidTipeEksposur(TipeEksposur(req.TipeEksposur)) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipeEksposur",
			Rule:    "oneof",
			Message: fmt.Sprintf("tipeEksposur harus salah satu dari: SOVEREIGN, BANK, CORPORATE, RETAIL, EQUITY, REINSURANCE. Diterima: %q", req.TipeEksposur),
		})
	}

	// LGD: parse decimal and validate range [0,1].
	lgdDec, lgdErr := decimal.NewFromString(req.LGD)
	if lgdErr != nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.lgd",
			Rule:    "decimal",
			Message: "lgd harus berupa angka desimal (contoh: \"0.4500\")",
		})
	} else if lgdDec.LessThan(lgdMin) || lgdDec.GreaterThan(lgdMax) {
		details = append(details, domainerrors.Detail{
			Field:   "body.lgd",
			Rule:    "range",
			Message: "lgd harus antara 0 dan 1 (inklusif)",
		})
	}

	// Date format.
	if !dateRe.MatchString(req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "periodeBerlakuDari harus dalam format YYYY-MM-DD",
		})
	}

	// sampai must be >= dari (if present).
	if req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai != "" {
		if !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "format",
				Message: "periodeBerlakuSampai harus dalam format YYYY-MM-DD",
			})
		} else if req.PeriodeBerlakuDari != "" && *req.PeriodeBerlakuSampai < req.PeriodeBerlakuDari {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "min",
				Message: "periodeBerlakuSampai harus >= periodeBerlakuDari",
			})
		}
	}

	if len(details) > 0 {
		return decimal.Zero, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return lgdDec, nil
}

// validateUpdate runs validation for update request.
// Returns the parsed LGD decimal pointer (nil if LGD not in request).
func (s *Service) validateUpdate(req UpdateRequest) (*decimal.Decimal, error) {
	var details []domainerrors.Detail

	if req.TipeEksposur != nil && !IsValidTipeEksposur(TipeEksposur(*req.TipeEksposur)) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipeEksposur",
			Rule:    "oneof",
			Message: fmt.Sprintf("tipeEksposur tidak valid: %q", *req.TipeEksposur),
		})
	}

	var lgdPtr *decimal.Decimal
	if req.LGD != nil {
		d, err := decimal.NewFromString(*req.LGD)
		if err != nil {
			details = append(details, domainerrors.Detail{
				Field:   "body.lgd",
				Rule:    "decimal",
				Message: "lgd harus berupa angka desimal",
			})
		} else if d.LessThan(lgdMin) || d.GreaterThan(lgdMax) {
			details = append(details, domainerrors.Detail{
				Field:   "body.lgd",
				Rule:    "range",
				Message: "lgd harus antara 0 dan 1 (inklusif)",
			})
		} else {
			lgdPtr = &d
		}
	}

	if req.PeriodeBerlakuDari != nil && !dateRe.MatchString(*req.PeriodeBerlakuDari) {
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
		}
	}

	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
		})
	}

	if len(details) > 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return lgdPtr, nil
}

// ─── Private helpers ──────────────────────────────────────────────────────────

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

// mapWorkflowState converts workflow engine state string to lgd_basel WorkflowStatus.
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
