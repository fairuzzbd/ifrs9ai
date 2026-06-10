package portofolio

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx attempts to rollback a transaction; logs on failure.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "portofolio service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for master portofolio.
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

// kodePortofolioRe validates kode_portofolio: uppercase alphanumeric + underscore, 1-20 chars.
var kodePortofolioRe = regexp.MustCompile(`^[A-Z0-9_]{1,20}$`)

// dateRe validates YYYY-MM-DD date string.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new Portofolio record in DRAFT state.
// Writes audit PORTOFOLIO.CREATE in the same transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Portofolio, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	aktifFlag := true
	if req.AktifFlag != nil {
		aktifFlag = *req.AktifFlag
	}

	p := &Portofolio{
		ID:                     uuid.New(),
		KodePortofolio:         req.KodePortofolio,
		Nama:                   req.Nama,
		TujuanPengelolaan:      req.TujuanPengelolaan,
		BMCategoryDefault:      BMCategory(req.BMCategoryDefault),
		Benchmark:              req.Benchmark,
		KompensasiManagerBasis: req.KompensasiManagerBasis,
		PeriodeReviewTerakhir:  req.PeriodeReviewTerakhir,
		AktifFlag:              aktifFlag,
		WorkflowStatus:         WorkflowStatusDraft,
		CreatedAt:              now,
		CreatedBy:              &actorID,
		RowVersion:             1,
		TenantID:               tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, p); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if isErrKodeDuplicate(err) {
			return nil, domainerrors.New(
				domainerrors.CodePortofolioDuplicateKode,
				fmt.Sprintf("Portofolio dengan kode %s sudah terdaftar di sistem.", req.KodePortofolio),
				domainerrors.Detail{Field: "body.kodePortofolio", Rule: "unique",
					Message: fmt.Sprintf("Kode portofolio %s sudah ada", req.KodePortofolio)},
			)
		}
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PORTOFOLIO.CREATE",
		EntityType: "mst.portofolio",
		EntityID:   p.ID,
		After:      p,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}
	return p, nil
}

// ─── GetByKode ────────────────────────────────────────────────────────────────

// GetByKode fetches one record, returning domainerrors.ErrNotFound if absent.
func (s *Service) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Portofolio, error) {
	p, err := s.repo.GetByKode(ctx, kode, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByKode: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Portofolio " + kode)
	}
	return p, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*Portofolio
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
		lastID = items[limit-1].KodePortofolio
	}

	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies a partial update to an existing record.
// Guard: workflow_status MUST be DRAFT or RETURNED (REJECTED at DB level).
// Guard: row_version optimistic lock.
// Writes audit PORTOFOLIO.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, kode string, req UpdateRequest) (*Portofolio, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetByKode(ctx, kode, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Portofolio " + kode)
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Portofolio %s sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan perubahan melalui workflow.", kode),
		)
	}

	before := *current

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	fields := UpdateFields{
		Nama:                   req.Nama,
		TujuanPengelolaan:      req.TujuanPengelolaan,
		BMCategoryDefault:      req.BMCategoryDefault,
		Benchmark:              req.Benchmark,
		KompensasiManagerBasis: req.KompensasiManagerBasis,
		PeriodeReviewTerakhir:  req.PeriodeReviewTerakhir,
		AktifFlag:              req.AktifFlag,
		UpdatedBy:              actorID,
		ExpectedVersion:        req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, kode, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Portofolio " + kode)
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PORTOFOLIO.UPDATE",
		EntityType: "mst.portofolio",
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
// Guard: active references exist → ENTITY_IN_USE (409).
func (s *Service) SoftDelete(ctx context.Context, kode string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByKode(ctx, kode, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Portofolio " + kode)
	}

	refCount, err := s.repo.CountReferences(ctx, kode)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Portofolio %s tidak bisa dihapus karena masih digunakan oleh %d instrumen. "+
				"Nonaktifkan portofolio ini dengan mengubah aktif_flag menjadi false.", kode, refCount),
			domainerrors.Detail{
				Field:   "kodePortofolio",
				Rule:    "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d instrumen aktif", refCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, kode, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PORTOFOLIO.DELETE",
		EntityType: "mst.portofolio",
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

// ─── Workflow status sync (EntityHook pattern) ────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine after a state transition.
// It updates mst.portofolio.workflow_status to stay in sync with sys.workflow_instance.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	p, err := s.repo.GetByID(ctx, entityID)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if p == nil {
		return domainerrors.ErrNotFound("Portofolio entity")
	}

	auditAction := "PORTOFOLIO." + strings.ToUpper(action)

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
		EntityType: "mst.portofolio",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(p.WorkflowStatus)},
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

// ListHistory returns paginated audit log for a given kode.
func (s *Service) ListHistory(ctx context.Context, kode string, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByKode(ctx, kode, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Portofolio " + kode)
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, existing.ID, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit PORTOFOLIO.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (csvReader interface{ Read([]byte) (int, error) }, rowCount int, err error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "PORTOFOLIO.EXPORT",
			EntityType:  "mst.portofolio",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "portofolio ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "portofolio ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// validateCreate runs all field-level validation for create.
func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if !kodePortofolioRe.MatchString(req.KodePortofolio) {
		details = append(details, domainerrors.Detail{
			Field:   "body.kodePortofolio",
			Rule:    "pattern",
			Message: "Kode portofolio harus 1-20 karakter huruf kapital, angka, atau underscore (contoh: EKUITAS_A1, BOND_HTC)",
		})
	}
	if len(req.Nama) < 3 || len(req.Nama) > 200 {
		details = append(details, domainerrors.Detail{
			Field:   "body.nama",
			Rule:    "length",
			Message: "Nama portofolio harus 3-200 karakter",
		})
	}
	if !BMCategory(req.BMCategoryDefault).IsValid() {
		details = append(details, domainerrors.Detail{
			Field:   "body.bmCategoryDefault",
			Rule:    "oneof",
			Message: "bmCategoryDefault harus salah satu dari: HTC, HTCS, OTHER",
		})
	}
	if req.PeriodeReviewTerakhir != nil && !dateRe.MatchString(*req.PeriodeReviewTerakhir) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeReviewTerakhir",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
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

// validateUpdate runs validation for update request.
func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.Nama != nil && (len(*req.Nama) < 3 || len(*req.Nama) > 200) {
		details = append(details, domainerrors.Detail{
			Field:   "body.nama",
			Rule:    "length",
			Message: "Nama portofolio harus 3-200 karakter",
		})
	}
	if req.BMCategoryDefault != nil && !BMCategory(*req.BMCategoryDefault).IsValid() {
		details = append(details, domainerrors.Detail{
			Field:   "body.bmCategoryDefault",
			Rule:    "oneof",
			Message: "bmCategoryDefault harus salah satu dari: HTC, HTCS, OTHER",
		})
	}
	if req.PeriodeReviewTerakhir != nil && !dateRe.MatchString(*req.PeriodeReviewTerakhir) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeReviewTerakhir",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
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

// isErrKodeDuplicate checks if the error is a kode duplicate.
func isErrKodeDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ErrKodeDuplicate.Error()) ||
		strings.Contains(err.Error(), "duplicate") ||
		strings.Contains(err.Error(), "23505")
}

// mapWorkflowState converts workflow engine state string to WorkflowStatus.
func mapWorkflowState(state string) WorkflowStatus {
	switch state {
	case "DRAFT":
		return WorkflowStatusDraft
	case "PENDING_REVIEW":
		return WorkflowStatusPendingReview
	case "PENDING_APPROVAL":
		return WorkflowStatusPendingApproval
	case "APPROVED":
		return WorkflowStatusApproved
	case "REJECTED":
		return WorkflowStatusRejected
	default:
		return WorkflowStatus(state)
	}
}
