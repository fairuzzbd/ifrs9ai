package impactmevpd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx rolls back a transaction, logging (but ignoring) errors.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "impactmevpd service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for mst.impact_mev_pd.
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

// Create validates and persists a new ImpactMevPd in DRAFT state.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ImpactMevPd, error) {
	multiplier, periodeID, skenario, err := s.validateCreate(req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Duplicate check: (periode_id, skenario) must not have an active/pending row.
	dupCount, err := s.repo.CountDuplicate(ctx, periodeID, skenario, uuid.Nil, tenantID(claims))
	if err != nil {
		return nil, fmt.Errorf("service.Create duplicate check: %w", err)
	}
	if dupCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeFLPeriodDuplicate,
			fmt.Sprintf("Sudah terdapat row aktif untuk periode %s skenario %s. "+
				"Hapus atau tunggu workflow selesai sebelum membuat baru.", periodeID, skenario),
			domainerrors.Detail{Field: "body.periodeId", Rule: "fl_periode_duplicate",
				Message: "Duplicate (periode_id, skenario) already exists"},
		)
	}

	now := time.Now()
	e := &ImpactMevPd{
		ID:                uuid.New(),
		PeriodeID:         periodeID,
		Skenario:          skenario,
		ImpactMultiplier:  multiplier,
		MevComponentsJSON: req.MevComponentsJSON,
		Catatan:           req.Catatan,
		WorkflowStatus:    WorkflowStatusDraft,
		CreatedAt:         now,
		CreatedBy:         &actorID,
		MakerID:           actorID,
		RowVersion:        1,
		TenantID:          tenantID(claims),
	}

	if req.DokumenPendukungID != nil {
		docID, parseErr := uuid.Parse(*req.DokumenPendukungID)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"dokumenPendukungId bukan UUID valid",
				domainerrors.Detail{Field: "body.dokumenPendukungId", Rule: "format", Message: "Harus UUID"},
			)
		}
		e.DokumenPendukungID = &docID
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
		Action:     "IMPACT_MEV_PD.CREATE",
		EntityType: "mst.impact_mev_pd",
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

func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*ImpactMevPd, error) {
	e, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if e == nil {
		return nil, domainerrors.ErrNotFound("ImpactMevPd " + id.String())
	}
	return e, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult holds List query result.
type ListResult struct {
	Items      []*ImpactMevPd
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted, tenantID(claims))
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

// Update validates and applies a partial update. Guard: DRAFT or RETURNED only.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*ImpactMevPd, error) {
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
		return nil, domainerrors.ErrNotFound("ImpactMevPd " + id.String())
	}
	if !current.WorkflowStatus.IsEditable() {
		return nil, domainerrors.New(domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("ImpactMevPd %s dalam status %s, tidak bisa diedit.", id, current.WorkflowStatus),
		)
	}

	before := *current

	fields := UpdateFields{
		UpdatedBy:         actorID,
		ExpectedVersion:   req.RowVersion,
		MevComponentsJSON: req.MevComponentsJSON,
		Catatan:           req.Catatan,
	}
	if req.ImpactMultiplier != nil {
		m, parseErr := decimal.NewFromString(*req.ImpactMultiplier)
		if parseErr != nil || !m.IsPositive() {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"impactMultiplier harus angka positif",
				domainerrors.Detail{Field: "body.impactMultiplier", Rule: "positive", Message: "Harus > 0"},
			)
		}
		fields.ImpactMultiplier = &m
	}
	if req.DokumenPendukungID != nil {
		docID, parseErr := uuid.Parse(*req.DokumenPendukungID)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed, "dokumenPendukungId bukan UUID valid",
				domainerrors.Detail{Field: "body.dokumenPendukungId", Rule: "format", Message: "Harus UUID"},
			)
		}
		fields.DokumenPendukungID = &docID
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	updated, err := s.repo.Update(ctx, tx, id, fields, tenantID(claims))
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("ImpactMevPd " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "IMPACT_MEV_PD.UPDATE",
		EntityType: "mst.impact_mev_pd",
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

// SoftDelete marks the record deleted. Guard: DRAFT/REJECTED/RETURNED only.
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
		return domainerrors.ErrNotFound("ImpactMevPd " + id.String())
	}
	if !existing.WorkflowStatus.IsEditable() {
		return domainerrors.New(domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("ImpactMevPd %s dalam status %s, tidak bisa dihapus.", id, existing.WorkflowStatus),
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID, tenantID(claims))
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "IMPACT_MEV_PD.DELETE",
		EntityType: "mst.impact_mev_pd",
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

// ─── GetActive ────────────────────────────────────────────────────────────────

// GetActive returns APPROVED rows for the given periode_id.
// Used by ECL engine Phase 4 (OQ-5: two separate endpoints).
// Returns 404 NOT_FOUND when no APPROVED rows exist for the period.
func (s *Service) GetActive(ctx context.Context, periodeID uuid.UUID) (*ActiveResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	rows, err := s.repo.GetActive(ctx, periodeID, tenantID(claims))
	if err != nil {
		return nil, fmt.Errorf("service.GetActive: %w", err)
	}

	// F2: return 404 when no APPROVED row found.
	if len(rows) == 0 {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Tidak ada impact_mev_pd APPROVED aktif untuk periode %s.", periodeID),
		)
	}

	result := &ActiveResponse{
		PeriodeID:   periodeID.String(),
		Multipliers: make(map[string]string),
		Rows:        make([]Response, 0, len(rows)),
	}
	for _, row := range rows {
		result.Multipliers[string(row.Skenario)] = row.ImpactMultiplier.StringFixed(8)
		result.Rows = append(result.Rows, ToResponse(row))
	}
	return result, nil
}

// ─── Workflow sync ────────────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the workflow engine after a state transition.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	if _, err := requireActor(auth.ClaimsFromContext(ctx)); err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	e, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if e == nil {
		return domainerrors.ErrNotFound("ImpactMevPd entity " + entityID.String())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: begin tx: %w", err)
	}

	claimsSvc := auth.ClaimsFromContext(ctx)
	if err := s.repo.UpdateWorkflowStatusTx(ctx, tx, entityID, wfStatus, tenantID(claimsSvc)); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	auditAction := "IMPACT_MEV_PD." + action
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.impact_mev_pd",
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

func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("ImpactMevPd " + id.String())
	}
	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, 0, err
	}

	reader, count, err := s.repo.ExportAll(ctx, q, tenantID(claims))
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "IMPACT_MEV_PD.EXPORT",
			EntityType:  "mst.impact_mev_pd",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After:       map[string]interface{}{"format": "csv", "row_count": count, "filters": q.AppliedFilter()},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "impactmevpd ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "impactmevpd ExportCSV: audit commit failed", "error", commitErr)
		}
	}
	return reader, count, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

func (s *Service) validateCreate(req CreateRequest) (decimal.Decimal, uuid.UUID, Skenario, error) {
	var details []domainerrors.Detail

	periodeID, err := uuid.Parse(req.PeriodeID)
	if err != nil {
		details = append(details, domainerrors.Detail{Field: "body.periodeId", Rule: "format", Message: "Harus UUID valid"})
	}

	skenario := Skenario(req.Skenario)
	if !skenario.IsValid() {
		details = append(details, domainerrors.Detail{
			Field:   "body.skenario",
			Rule:    "enum",
			Message: "skenario harus 'GOOD' atau 'BAD' (NORMAL tidak memiliki MEV impact row)",
		})
	}

	var multiplier decimal.Decimal
	if req.ImpactMultiplier != "" {
		parsed, parseErr := decimal.NewFromString(req.ImpactMultiplier)
		switch {
		case parseErr != nil:
			details = append(details, domainerrors.Detail{Field: "body.impactMultiplier", Rule: "format", Message: "Harus angka desimal valid"})
		case !parsed.IsPositive():
			details = append(details, domainerrors.Detail{Field: "body.impactMultiplier", Rule: "positive", Message: "Harus > 0"})
		default:
			multiplier = parsed
		}
	}

	if len(details) > 0 {
		return decimal.Zero, uuid.Nil, "", domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}
	return multiplier, periodeID, skenario, nil
}

func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail
	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{Field: "body.rowVersion", Rule: "required", Message: "rowVersion wajib positif"})
	}
	if req.ImpactMultiplier != nil {
		parsed, parseErr := decimal.NewFromString(*req.ImpactMultiplier)
		if parseErr != nil {
			details = append(details, domainerrors.Detail{Field: "body.impactMultiplier", Rule: "format", Message: "Harus angka desimal valid"})
		} else if !parsed.IsPositive() {
			details = append(details, domainerrors.Detail{Field: "body.impactMultiplier", Rule: "positive", Message: "Harus > 0"})
		}
	}
	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed, fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}
	return nil
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

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

func tenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

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
