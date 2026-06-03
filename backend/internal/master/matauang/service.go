package matauang

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx is a helper that attempts to rollback a transaction.
// Rollback errors are non-actionable and are logged at warning level.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "matauang service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for master mata_uang.
// It manages transaction boundaries; repo methods must be called with a tx when inside one.
//
// REUSE PATTERN for other master modules:
//   - Replace Repository interface calls with module-specific repo
//   - Replace CodeSystemCurrencyProtected with module-specific guard constants
//   - Keep the audit.Write pattern identical
//   - Keep the workflow_status guard pattern identical
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

// kodeMataUangRe validates kode_mata_uang format: exactly 3 uppercase ASCII letters.
var kodeMataUangRe = regexp.MustCompile(`^[A-Z]{3}$`)

// dateRe validates YYYY-MM-DD date string.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new MataUang record in DRAFT state.
// Writes audit MATA_UANG.CREATE in the same transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*MataUang, error) {
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

	m := &MataUang{
		KodeMataUang:      req.KodeMataUang,
		ID:                uuid.New(),
		NamaMataUang:      req.NamaMataUang,
		Simbol:            req.Simbol,
		DecimalPlaces:     req.DecimalPlaces,
		SumberKursDefault: req.SumberKursDefault,
		FrekuensiUpdate:   req.FrekuensiUpdate,
		AktifFlag:         aktifFlag,
		TanggalMulaiAktif: req.TanggalMulaiAktif,
		IsSystemCurrency:  false, // cannot be set via API
		WorkflowStatus:    WorkflowStatusDraft,
		CreatedAt:         now,
		CreatedBy:         &actorID,
		RowVersion:        1,
		TenantID:          tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, m); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if isErrKodeDuplicate(err) {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Mata uang %s sudah terdaftar di sistem.", req.KodeMataUang),
				domainerrors.Detail{Field: "body.kodeMataUang", Rule: "unique",
					Message: fmt.Sprintf("Kode mata uang %s sudah ada", req.KodeMataUang)},
			)
		}
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "MATA_UANG.CREATE",
		EntityType: "mst.mata_uang",
		EntityID:   m.ID,
		After:      m,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}

	return m, nil
}

// ─── GetByKode ────────────────────────────────────────────────────────────────

// GetByKode fetches one record, returning domainerrors.ErrNotFound if absent.
func (s *Service) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*MataUang, error) {
	m, err := s.repo.GetByKode(ctx, kode, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByKode: %w", err)
	}
	if m == nil {
		return nil, domainerrors.ErrNotFound("Mata uang " + kode)
	}
	return m, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*MataUang
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
		lastID = items[limit-1].KodeMataUang
	}

	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies a partial update to an existing record.
// Guard: workflow_status MUST be DRAFT or RETURNED (REJECTED at DB level).
// Guard: row_version optimistic lock.
// Writes audit MATA_UANG.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, kode string, req UpdateRequest) (*MataUang, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Load current state (outside tx — optimistic approach)
	current, err := s.repo.GetByKode(ctx, kode, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Mata uang " + kode)
	}

	// Guard: cannot edit an APPROVED record (must go through workflow amend cycle)
	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Mata uang %s sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan perubahan ke Finance Controller untuk diproses melalui workflow.", kode),
		)
	}

	before := *current

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	fields := UpdateFields{
		NamaMataUang:      req.NamaMataUang,
		Simbol:            req.Simbol,
		DecimalPlaces:     req.DecimalPlaces,
		SumberKursDefault: req.SumberKursDefault,
		FrekuensiUpdate:   req.FrekuensiUpdate,
		AktifFlag:         req.AktifFlag,
		TanggalMulaiAktif: req.TanggalMulaiAktif,
		UpdatedBy:         actorID,
		ExpectedVersion:   req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, kode, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Mata uang " + kode)
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "MATA_UANG.UPDATE",
		EntityType: "mst.mata_uang",
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
// Guards:
//   - is_system_currency = true → SYSTEM_CURRENCY_PROTECTED (403)
//   - Active references exist    → ENTITY_IN_USE (409)
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
		return domainerrors.ErrNotFound("Mata uang " + kode)
	}

	// Guard: system currency cannot be deleted
	if existing.IsSystemCurrency {
		return domainerrors.New(
			domainerrors.CodeSystemCurrencyProtected,
			fmt.Sprintf("Mata uang %s adalah currency fungsional Tugure dan tidak bisa dihapus.", kode),
		)
	}

	// Guard: referential integrity
	refCount, err := s.repo.CountReferences(ctx, kode)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Mata uang %s tidak bisa dihapus karena masih digunakan oleh %d entitas. "+
				"Nonaktifkan mata uang ini dengan mengubah aktif_flag menjadi false.", kode, refCount),
			domainerrors.Detail{
				Field:   "kodeMataUang",
				Rule:    "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d entitas aktif", refCount),
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
		Action:     "MATA_UANG.DELETE",
		EntityType: "mst.mata_uang",
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

// ─── Workflow transitions (called from workflow handler hooks) ─────────────────

// SyncWorkflowStatus is called by the generic workflow engine after a state transition.
// It updates mst.mata_uang.workflow_status to stay in sync with sys.workflow_instance.
// The action string is the workflow action that was performed (e.g. "SUBMIT", "APPROVE").
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	m, err := s.repo.GetByID(ctx, entityID)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if m == nil {
		return domainerrors.ErrNotFound("Mata uang entity")
	}

	auditAction := "MATA_UANG." + action

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
		EntityType: "mst.mata_uang",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(m.WorkflowStatus)},
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
		return nil, false, domainerrors.ErrNotFound("Mata uang " + kode)
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, existing.ID, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit MATA_UANG.EXPORT.
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

	// Write audit outside of a tx (export is read-only + audit is acceptable best-effort)
	// Per convention: audit writes for export are best-effort and don't block the export.
	// A fresh DB tx is opened just for the audit write.
	// Guard: if BeginTx returns nil tx (test stubs, no-DB mode) skip the audit write.
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "MATA_UANG.EXPORT",
			EntityType:  "mst.mata_uang",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			// Best-effort: log but don't block export.
			s.logger.WarnContext(ctx, "matauang ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "matauang ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// validateCreate runs all field-level + cross-field validation for create.
func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if !kodeMataUangRe.MatchString(req.KodeMataUang) {
		details = append(details, domainerrors.Detail{
			Field:   "body.kodeMataUang",
			Rule:    "pattern",
			Message: "Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)",
		})
	}
	if len(req.NamaMataUang) < 3 || len(req.NamaMataUang) > 60 {
		details = append(details, domainerrors.Detail{
			Field:   "body.namaMataUang",
			Rule:    "length",
			Message: "Nama mata uang harus 3-60 karakter",
		})
	}
	if len(req.Simbol) < 1 || len(req.Simbol) > 5 {
		details = append(details, domainerrors.Detail{
			Field:   "body.simbol",
			Rule:    "length",
			Message: "Simbol harus 1-5 karakter",
		})
	}
	if req.DecimalPlaces < 0 || req.DecimalPlaces > 4 {
		details = append(details, domainerrors.Detail{
			Field:   "body.decimalPlaces",
			Rule:    "range",
			Message: "Decimal places harus antara 0 dan 4",
		})
	}
	if req.TanggalMulaiAktif != "" && !dateRe.MatchString(req.TanggalMulaiAktif) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalMulaiAktif",
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

	if req.NamaMataUang != nil && (len(*req.NamaMataUang) < 3 || len(*req.NamaMataUang) > 60) {
		details = append(details, domainerrors.Detail{
			Field:   "body.namaMataUang",
			Rule:    "length",
			Message: "Nama mata uang harus 3-60 karakter",
		})
	}
	if req.Simbol != nil && (len(*req.Simbol) < 1 || len(*req.Simbol) > 5) {
		details = append(details, domainerrors.Detail{
			Field:   "body.simbol",
			Rule:    "length",
			Message: "Simbol harus 1-5 karakter",
		})
	}
	if req.DecimalPlaces != nil && (*req.DecimalPlaces < 0 || *req.DecimalPlaces > 4) {
		details = append(details, domainerrors.Detail{
			Field:   "body.decimalPlaces",
			Rule:    "range",
			Message: "Decimal places harus antara 0 dan 4",
		})
	}
	if req.TanggalMulaiAktif != nil && !dateRe.MatchString(*req.TanggalMulaiAktif) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalMulaiAktif",
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

// isErrKodeDuplicate unwraps to check if the error is a kode duplicate.
func isErrKodeDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == ErrKodeDuplicate.Error() ||
		(len(err.Error()) > 0 && (containsStr(err.Error(), "kode_mata_uang: mata_uang kode duplicate") ||
			containsStr(err.Error(), "duplicate")))
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// mapWorkflowState converts workflow engine state string to mata_uang WorkflowStatus.
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
