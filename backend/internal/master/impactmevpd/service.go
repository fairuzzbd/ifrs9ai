package impactmevpd

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

// plausibleMin and plausibleMax are the soft-warning range for impact_multiplier.
// Values outside this range are allowed (ALCO override) but the service notes them.
// Per task spec: warning only, not hard reject.
var (
	plausibleMin = decimal.NewFromFloat(0.5)
	plausibleMax = decimal.NewFromFloat(2.5)
)

// rollbackTx is a helper that silently rolls back a transaction.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "impactmevpd service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for impact_mev_pd.
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
	Items      []*ImpactMevPD
	Pagination pagination.Result
}

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and inserts a new record in DRAFT state.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ImpactMevPD, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Parse UUID fields
	periodeID, err := uuid.Parse(req.PeriodeID)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"periodeId harus berformat UUID v4 yang valid.",
			domainerrors.Detail{Field: "body.periodeId", Rule: "uuid", Message: "Format UUID tidak valid"},
		)
	}

	// Validate skenario
	if !ValidSkenario(req.Skenario) {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Skenario harus salah satu dari: GOOD, BAD.",
			domainerrors.Detail{Field: "body.skenario", Rule: "oneof", Message: "Harus GOOD atau BAD"},
		)
	}

	// Parse multiplier (decimal, no float64 per DEC-016)
	multiplier, err := decimal.NewFromString(req.ImpactMultiplier)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"impactMultiplier harus berupa angka desimal yang valid.",
			domainerrors.Detail{Field: "body.impactMultiplier", Rule: "decimal", Message: "Format angka tidak valid"},
		)
	}

	// Plausibility warning — not a hard reject
	if multiplier.LessThan(plausibleMin) || multiplier.GreaterThan(plausibleMax) {
		s.logger.WarnContext(ctx, "impactmevpd: impact_multiplier di luar range plausible 0.5-2.5",
			"skenario", req.Skenario,
			"multiplier", multiplier.String(),
			"actor", actorID.String(),
		)
	}

	// Validate MEV components JSON (optional field)
	if req.MevComponentsJSON != nil && *req.MevComponentsJSON != "" {
		if details := ValidateMevComponentsJSON(*req.MevComponentsJSON); len(details) > 0 {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"mevComponentsJson tidak valid", details...)
		}
	}

	// Duplicate guard: UNIQUE (periode_id, skenario)
	count, err := s.repo.CountByPeriodSkenario(ctx, periodeID, Skenario(req.Skenario))
	if err != nil {
		return nil, fmt.Errorf("service.Create CountByPeriodSkenario: %w", err)
	}
	if count > 0 {
		return nil, domainerrors.New(domainerrors.CodeImpactDuplicatePeriodeSkenario,
			fmt.Sprintf("Impact MEV PD untuk periode %s skenario %s sudah ada.", req.PeriodeID, req.Skenario),
			domainerrors.Detail{
				Field:   "body.periodeId",
				Rule:    "unique",
				Message: fmt.Sprintf("(periode_id=%s, skenario=%s) sudah terdaftar", req.PeriodeID, req.Skenario),
			},
		)
	}

	now := time.Now()
	m := &ImpactMevPD{
		ID:               uuid.New(),
		PeriodeID:        periodeID,
		Skenario:         Skenario(req.Skenario),
		ImpactMultiplier: multiplier,
		MevComponentsJSON: req.MevComponentsJSON,
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
		Action:     "IMPACT_MEV_PD.CREATE",
		EntityType: "mst.impact_mev_pd",
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

// GetByID fetches one record by UUID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*ImpactMevPD, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if m == nil {
		return nil, domainerrors.ErrNotFound("Impact MEV PD " + id.String())
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
// Guard: workflow_status must be DRAFT or REJECTED.
// Guard: row_version optimistic lock.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*ImpactMevPD, error) {
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
		return nil, domainerrors.ErrNotFound("Impact MEV PD " + id.String())
	}

	if !current.WorkflowStatus.IsEditable() {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Impact MEV PD %s sudah dalam status %s dan tidak bisa diedit langsung.", id, current.WorkflowStatus),
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
		if d.LessThan(plausibleMin) || d.GreaterThan(plausibleMax) {
			s.logger.WarnContext(ctx, "impactmevpd: update — multiplier di luar range plausible",
				"multiplier", d.String(), "actor", actorID.String())
		}
		fields.ImpactMultiplier = &d
	}

	if req.MevComponentsJSON != nil && *req.MevComponentsJSON != "" {
		if details := ValidateMevComponentsJSON(*req.MevComponentsJSON); len(details) > 0 {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"mevComponentsJson tidak valid", details...)
		}
		fields.MevComponentsJSON = req.MevComponentsJSON
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
			return nil, domainerrors.ErrNotFound("Impact MEV PD " + id.String())
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
		return domainerrors.ErrNotFound("Impact MEV PD " + id.String())
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
		Action:     "IMPACT_MEV_PD.DELETE",
		EntityType: "mst.impact_mev_pd",
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
		return domainerrors.ErrNotFound("Impact MEV PD entity")
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
		Action:     "IMPACT_MEV_PD." + action,
		EntityType: "mst.impact_mev_pd",
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

// ListHistory returns paginated audit log for an entity.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Impact MEV PD " + id.String())
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

	// Best-effort audit write for export.
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "IMPACT_MEV_PD.EXPORT",
			EntityType:  "mst.impact_mev_pd",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "impactmevpd ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "impactmevpd ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

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
