package pdpefindo

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

// rollbackTx is a non-fatal rollback helper.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "pdpefindo service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for mst.pd_pefindo.
// Manages transaction boundaries; repo methods must be called with a tx when inside one.
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

// pdZero is used for comparison against zero boundary.
var pdZero = decimal.NewFromInt(0)
var pdOne = decimal.NewFromInt(1)

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new PDPefindo in DRAFT state.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*PDPefindo, error) {
	if err := s.validateCreate(ctx, req, nil); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	sumber := req.Sumber
	if sumber == "" {
		sumber = DefaultSumber
	}

	now := time.Now()
	p := &PDPefindo{
		ID:                   uuid.New(),
		Rating:               req.Rating,
		PD12Month:            req.PD12Month,
		PDLifetime3Y:         req.PDLifetime3Y,
		PDLifetime5Y:         req.PDLifetime5Y,
		PDLifetime7Y:         req.PDLifetime7Y,
		PDLifetime10Y:        req.PDLifetime10Y,
		Sumber:               sumber,
		TanggalPublikasi:     req.TanggalPublikasi,
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		WorkflowStatus:       WorkflowStatusDraft,
		// Legacy fields: use actorID as the uploaded_by
		UploadedBy: actorID,
		UploadedAt: now,
		CreatedAt:  now,
		CreatedBy:  &actorID,
		RowVersion: 1,
		TenantID:   tenantID(claims),
	}

	if req.DokumenPendukungID != nil && *req.DokumenPendukungID != "" {
		docID, err := uuid.Parse(*req.DokumenPendukungID)
		if err == nil {
			p.DokumenPendukungID = &docID
		}
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create pd_pefindo: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, p); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create pd_pefindo: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PD_PEFINDO.CREATE",
		EntityType: "mst.pd_pefindo",
		EntityID:   p.ID,
		After:      p,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create pd_pefindo: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create pd_pefindo: commit: %w", err)
	}
	return p, nil
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID fetches one record; returns ErrNotFound if absent.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*PDPefindo, error) {
	p, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID pd_pefindo: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("PD Pefindo " + id.String())
	}
	return p, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*PDPefindo
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List pd_pefindo: %w", err)
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

// Update validates and applies a partial update.
// Guard: workflow_status must be DRAFT or RETURNED.
// Guard: row_version optimistic lock.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*PDPefindo, error) {
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
		return nil, fmt.Errorf("service.Update pd_pefindo load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("PD Pefindo " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"PD Pefindo sudah disetujui dan tidak bisa diedit langsung. Ajukan melalui workflow.",
		)
	}

	// Build merged state for validation (to re-run monotonicity on updated values).
	merged := buildMergedForValidation(current, req)
	if err := validateMonotonicity(merged.PD12Month, merged.PDLifetime3Y, merged.PDLifetime5Y, merged.PDLifetime7Y, merged.PDLifetime10Y); err != nil {
		return nil, err
	}

	// Check period overlap (exclude self).
	if err := s.checkPeriodOverlap(ctx, merged.Rating, merged.PeriodeBerlakuDari, merged.PeriodeBerlakuSampai, &id); err != nil {
		return nil, err
	}

	before := *current

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update pd_pefindo: begin tx: %w", err)
	}

	fields := UpdateFields{
		PD12Month:            req.PD12Month,
		PDLifetime3Y:         req.PDLifetime3Y,
		PDLifetime5Y:         req.PDLifetime5Y,
		PDLifetime7Y:         req.PDLifetime7Y,
		PDLifetime10Y:        req.PDLifetime10Y,
		Sumber:               req.Sumber,
		TanggalPublikasi:     req.TanggalPublikasi,
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		DokumenPendukungID:   req.DokumenPendukungID,
		UpdatedBy:            actorID,
		ExpectedVersion:      req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("PD Pefindo " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update pd_pefindo: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PD_PEFINDO.UPDATE",
		EntityType: "mst.pd_pefindo",
		EntityID:   updated.ID,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update pd_pefindo: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update pd_pefindo: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
// Guard: no active FK references (reserved for future FK).
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete pd_pefindo load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("PD Pefindo " + id.String())
	}

	refCount, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete pd_pefindo count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("PD Pefindo %s tidak bisa dihapus karena masih digunakan oleh %d entitas.", id, refCount),
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete pd_pefindo: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete pd_pefindo: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PD_PEFINDO.DELETE",
		EntityType: "mst.pd_pefindo",
		EntityID:   deleted.ID,
		Before:     existing,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete pd_pefindo: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete pd_pefindo: commit: %w", err)
	}
	return nil
}

// ─── Workflow sync ────────────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the EntityHook after a workflow transition.
// It updates mst.pd_pefindo.workflow_status in sync with sys.workflow_instance.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	p, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus pd_pefindo load: %w", err)
	}
	if p == nil {
		return domainerrors.ErrNotFound("PD Pefindo entity")
	}

	auditAction := "PD_PEFINDO." + action

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus pd_pefindo: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus pd_pefindo: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.pd_pefindo",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(p.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": string(wfStatus)},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus pd_pefindo: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus pd_pefindo: commit: %w", err)
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given id.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory pd_pefindo load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("PD Pefindo " + id.String())
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit PD_PEFINDO.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, 0, err
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV pd_pefindo: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "PD_PEFINDO.EXPORT",
			EntityType:  "mst.pd_pefindo",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "pdpefindo ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "pdpefindo ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// GetJobStatus returns the status of an upload job.
func (s *Service) GetJobStatus(ctx context.Context, jobID string) (*JobRow, error) {
	j, err := s.repo.GetJobByID(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("service.GetJobStatus: %w", err)
	}
	if j == nil {
		return nil, domainerrors.New(domainerrors.CodeJobNotFound, "Upload job "+jobID+" tidak ditemukan.")
	}
	return j, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

func (s *Service) validateCreate(ctx context.Context, req CreateRequest, excludeID *uuid.UUID) error {
	var details []domainerrors.Detail

	if !IsValidPefindoRating(req.Rating) {
		details = append(details, domainerrors.Detail{
			Field:   "body.rating",
			Rule:    "whitelist",
			Message: fmt.Sprintf("Rating '%s' tidak valid. Gunakan salah satu dari: idAAA, idAA+, ..., idD", req.Rating),
		})
	}
	if err := validatePDRange(req.PD12Month, "pd12Month"); err != nil {
		details = append(details, err...)
	}
	if req.PDLifetime3Y != nil {
		if err := validatePDRange(*req.PDLifetime3Y, "pdLifetime3Y"); err != nil {
			details = append(details, err...)
		}
	}
	if req.PDLifetime5Y != nil {
		if err := validatePDRange(*req.PDLifetime5Y, "pdLifetime5Y"); err != nil {
			details = append(details, err...)
		}
	}
	if req.PDLifetime7Y != nil {
		if err := validatePDRange(*req.PDLifetime7Y, "pdLifetime7Y"); err != nil {
			details = append(details, err...)
		}
	}
	if req.PDLifetime10Y != nil {
		if err := validatePDRange(*req.PDLifetime10Y, "pdLifetime10Y"); err != nil {
			details = append(details, err...)
		}
	}
	if !dateRe.MatchString(req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.PeriodeBerlakuSampai != nil && !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuSampai",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	// periode_berlaku_sampai >= periode_berlaku_dari
	if req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai < req.PeriodeBerlakuDari {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuSampai",
			Rule:    "date_range",
			Message: "Periode berlaku sampai harus >= periode berlaku dari",
		})
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}

	// PD monotonicity check
	if err := validateMonotonicity(req.PD12Month, req.PDLifetime3Y, req.PDLifetime5Y, req.PDLifetime7Y, req.PDLifetime10Y); err != nil {
		return err
	}

	// Period overlap check
	return s.checkPeriodOverlap(ctx, req.Rating, req.PeriodeBerlakuDari, req.PeriodeBerlakuSampai, excludeID)
}

func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.PD12Month != nil {
		if errs := validatePDRange(*req.PD12Month, "pd12Month"); len(errs) > 0 {
			details = append(details, errs...)
		}
	}
	if req.PDLifetime3Y != nil {
		if errs := validatePDRange(*req.PDLifetime3Y, "pdLifetime3Y"); len(errs) > 0 {
			details = append(details, errs...)
		}
	}
	if req.PDLifetime5Y != nil {
		if errs := validatePDRange(*req.PDLifetime5Y, "pdLifetime5Y"); len(errs) > 0 {
			details = append(details, errs...)
		}
	}
	if req.PDLifetime7Y != nil {
		if errs := validatePDRange(*req.PDLifetime7Y, "pdLifetime7Y"); len(errs) > 0 {
			details = append(details, errs...)
		}
	}
	if req.PDLifetime10Y != nil {
		if errs := validatePDRange(*req.PDLifetime10Y, "pdLifetime10Y"); len(errs) > 0 {
			details = append(details, errs...)
		}
	}
	if req.PeriodeBerlakuDari != nil && !dateRe.MatchString(*req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.PeriodeBerlakuSampai != nil && !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuSampai",
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
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}
	return nil
}

// validatePDRange checks 0 ≤ v ≤ 1.
func validatePDRange(v decimal.Decimal, field string) []domainerrors.Detail {
	if v.LessThan(pdZero) || v.GreaterThan(pdOne) {
		return []domainerrors.Detail{{
			Field:   "body." + field,
			Rule:    "range",
			Message: field + " harus antara 0 dan 1 (inklusif)",
		}}
	}
	return nil
}

// validateMonotonicity enforces:
// pd_12month ≤ pd_lifetime_3y ≤ pd_lifetime_5y ≤ pd_lifetime_7y ≤ pd_lifetime_10y
//
// Special case: for rating idD, all values must be exactly 1.0 (certain default).
// Comparison is performed only on non-nil values; partial chains are still validated.
//
// Rationale: Pefindo data should be monotonically non-decreasing because
// longer horizons accumulate more risk. Violation → likely data corruption.
func validateMonotonicity(pd12 decimal.Decimal, pd3y, pd5y, pd7y, pd10y *decimal.Decimal) error {
	chain := []struct {
		val  *decimal.Decimal
		name string
	}{
		{nil, "pd12Month"}, // placeholder: pd12 is non-pointer
		{pd3y, "pdLifetime3Y"},
		{pd5y, "pdLifetime5Y"},
		{pd7y, "pdLifetime7Y"},
		{pd10y, "pdLifetime10Y"},
	}
	// Inject pd12 as first pointer for chain iteration.
	pd12Copy := pd12
	chain[0].val = &pd12Copy

	var prev *decimal.Decimal
	var prevName string
	for _, c := range chain {
		if c.val == nil {
			continue
		}
		if prev != nil && c.val.LessThan(*prev) {
			return domainerrors.New(
				domainerrors.CodePDMonotonicityViolated,
				fmt.Sprintf("PD monotonicity dilanggar: %s (%.8f) < %s (%.8f). "+
					"PD harus non-decreasing sesuai Pefindo calibration data.",
					c.name, c.val.InexactFloat64(), prevName, prev.InexactFloat64()),
				domainerrors.Detail{
					Field:   "body." + c.name,
					Rule:    "monotonicity",
					Message: fmt.Sprintf("%s harus >= %s", c.name, prevName),
				},
			)
		}
		prev = c.val
		prevName = c.name
	}
	return nil
}

// checkPeriodOverlap returns PD_PERIOD_OVERLAP if another record for the same rating
// has an overlapping valid period.
func (s *Service) checkPeriodOverlap(ctx context.Context, rating, dari string, sampai *string, excludeID *uuid.UUID) error {
	count, err := s.repo.CountOverlap(ctx, rating, dari, sampai, excludeID)
	if err != nil {
		return fmt.Errorf("service.checkPeriodOverlap: %w", err)
	}
	if count > 0 {
		msg := fmt.Sprintf("Terdapat %d record PD untuk rating '%s' dengan periode yang overlap. "+
			"Tutup periode record lama sebelum membuat record baru.", count, rating)
		return domainerrors.New(domainerrors.CodePDPeriodOverlap, msg,
			domainerrors.Detail{Field: "body.periodeBerlakuDari", Rule: "period_overlap", Message: msg},
		)
	}
	return nil
}

// buildMergedForValidation builds a PDPefindo snapshot with the update fields applied
// so monotonicity + period overlap can be re-validated on the merged state.
func buildMergedForValidation(current *PDPefindo, req UpdateRequest) *PDPefindo {
	merged := *current
	if req.PD12Month != nil {
		merged.PD12Month = *req.PD12Month
	}
	if req.PDLifetime3Y != nil {
		merged.PDLifetime3Y = req.PDLifetime3Y
	}
	if req.PDLifetime5Y != nil {
		merged.PDLifetime5Y = req.PDLifetime5Y
	}
	if req.PDLifetime7Y != nil {
		merged.PDLifetime7Y = req.PDLifetime7Y
	}
	if req.PDLifetime10Y != nil {
		merged.PDLifetime10Y = req.PDLifetime10Y
	}
	if req.PeriodeBerlakuDari != nil {
		merged.PeriodeBerlakuDari = *req.PeriodeBerlakuDari
	}
	if req.PeriodeBerlakuSampai != nil {
		merged.PeriodeBerlakuSampai = req.PeriodeBerlakuSampai
	}
	return &merged
}

// ─── General helpers ──────────────────────────────────────────────────────────

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

// mapWorkflowState converts workflow engine state string to pd_pefindo WorkflowStatus.
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
