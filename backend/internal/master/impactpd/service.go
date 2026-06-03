package impactpd

import (
	"context"
	"database/sql"
	"fmt"
	"io"
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

// rollbackTx is a helper that silently rolls back a transaction.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "impactpd service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for impact_pd.
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

// ListResult is returned by List.
type ListResult struct {
	Items      []*ImpactPD
	Pagination pagination.Result
}

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and inserts a new record in DRAFT state.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ImpactPD, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	periodeID, err := uuid.Parse(req.PeriodeID)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"periodeId harus berformat UUID v4 yang valid.",
			domainerrors.Detail{Field: "body.periodeId", Rule: "uuid", Message: "Format UUID tidak valid"},
		)
	}

	// Parse and validate multiplier — HARD reject outside [0.5, 2.0] (DB CHECK + service mirror)
	multiplier, err := decimal.NewFromString(req.ImpactMultiplier)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"impactMultiplier harus berupa angka desimal yang valid.",
			domainerrors.Detail{Field: "body.impactMultiplier", Rule: "decimal"},
		)
	}
	if err := validateMultiplier(multiplier); err != nil {
		return nil, err
	}

	// Duplicate guard: UNIQUE periode_id
	count, err := s.repo.CountByPeriode(ctx, periodeID)
	if err != nil {
		return nil, fmt.Errorf("service.Create CountByPeriode: %w", err)
	}
	if count > 0 {
		return nil, domainerrors.New(domainerrors.CodeImpactPDPeriodeExists,
			fmt.Sprintf("Impact PD untuk periode %s sudah ada.", req.PeriodeID),
			domainerrors.Detail{
				Field:   "body.periodeId",
				Rule:    "unique",
				Message: fmt.Sprintf("periode_id=%s sudah terdaftar", req.PeriodeID),
			},
		)
	}

	now := time.Now()
	m := &ImpactPD{
		ID:               uuid.New(),
		PeriodeID:        periodeID,
		ImpactMultiplier: multiplier,
		Catatan:          req.Catatan,
		WorkflowStatus:   WorkflowStatusDraft,
		CreatedAt:        now,
		CreatedBy:        &actorID,
		RowVersion:       1,
		TenantID:         tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, m); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "IMPACT_PD.CREATE",
		EntityType: "mst.impact_pd",
		EntityID:   m.ID,
		After:      m,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create commit: %w", err)
	}
	return m, nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetByID fetches one record.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*ImpactPD, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if m == nil {
		return nil, domainerrors.ErrNotFound("Impact PD " + id.String())
	}
	return m, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

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

// Update validates and applies partial update.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*ImpactPD, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Impact PD " + id.String())
	}

	if !current.WorkflowStatus.IsEditable() {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Impact PD %s sudah dalam status %s dan tidak bisa diedit langsung.", id, current.WorkflowStatus),
		)
	}

	if req.RowVersion <= 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"rowVersion wajib diisi dan harus positif.",
			domainerrors.Detail{Field: "body.rowVersion", Rule: "required"},
		)
	}

	before := *current
	fields := UpdateFields{
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
	}

	if req.ImpactMultiplier != nil {
		d, err := decimal.NewFromString(*req.ImpactMultiplier)
		if err != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"impactMultiplier harus berupa angka desimal yang valid.",
				domainerrors.Detail{Field: "body.impactMultiplier", Rule: "decimal"},
			)
		}
		if err := validateMultiplier(d); err != nil {
			return nil, err
		}
		fields.ImpactMultiplier = &d
	}

	if req.Catatan != nil {
		fields.Catatan = req.Catatan
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update begin tx: %w", err)
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Impact PD " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "IMPACT_PD.UPDATE",
		EntityType: "mst.impact_pd",
		EntityID:   updated.ID,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Impact PD " + id.String())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "IMPACT_PD.DELETE",
		EntityType: "mst.impact_pd",
		EntityID:   deleted.ID,
		Before:     existing,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete commit: %w", err)
	}
	return nil
}

// ─── SyncWorkflowStatus ───────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the workflow EntityHook after a state transition.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := WorkflowStatus(newState)
	m, err := s.repo.GetByID(ctx, entityID)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if m == nil {
		return domainerrors.ErrNotFound("Impact PD entity")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "IMPACT_PD." + action,
		EntityType: "mst.impact_pd",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(m.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": newState},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus commit: %w", err)
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Impact PD " + id.String())
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV and writes audit.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
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
			Action:      "IMPACT_PD.EXPORT",
			EntityType:  "mst.impact_pd",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "impactpd ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "impactpd ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// validateMultiplier checks that the multiplier is in [0.5, 2.0].
// This is the service-side mirror of the DB CHECK constraint.
func validateMultiplier(d decimal.Decimal) error {
	if d.LessThan(MultiplierMin) || d.GreaterThan(MultiplierMax) {
		return domainerrors.New(domainerrors.CodeImpactPDOutOfRange,
			fmt.Sprintf("impactMultiplier harus berada di antara 0.5 dan 2.0 (diterima: %s).", d.String()),
			domainerrors.Detail{
				Field:   "body.impactMultiplier",
				Rule:    "range",
				Message: "Impact multiplier harus antara 0.5 dan 2.0",
			},
		)
	}
	return nil
}

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
