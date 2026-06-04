package ratinghistory

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

func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "ratinghistory service: tx rollback failed", "error", err)
	}
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// CounterpartyRepo is the subset of the counterparty repository needed by ratinghistory.
// This avoids a circular import between counterparty and ratinghistory packages.
// In production, pass a *counterparty.DBRepository wrapped in CounterpartyRepoAdapter.
type CounterpartyRepo interface {
	// UpdateRatingCache updates counterparty.rating_pefindo_current.
	UpdateRatingCache(ctx context.Context, tx *sql.Tx, id uuid.UUID, newRating *string, updatedBy uuid.UUID) error
	// BeginTx is not used directly here; we receive tx from caller.
	// (placeholder for interface definition; unused method needed if adapters must implement it)
}

// Service owns business logic for mst.rating_history_counterparty.
// SICR computation fires on workflow Approve.
type Service struct {
	repo           Repository
	cpRepo         CounterpartyRepo // for UpdateRatingCache on approve
	auditWriter    *audit.Writer
	logger         *slog.Logger
}

// NewService constructs a Service.
func NewService(repo Repository, cpRepo CounterpartyRepo, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, cpRepo: cpRepo, auditWriter: auditWriter, logger: logger}
}

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new RatingHistory in DRAFT state.
// SICR/default flags are NOT set at create time; they are computed at workflow Approve.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*RatingHistory, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	counterpartyID, err := uuid.Parse(req.CounterpartyID)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"counterpartyId bukan UUID valid.",
			domainerrors.Detail{Field: "body.counterpartyId", Rule: "uuid", Message: "Format UUID tidak valid"})
	}

	var dokumenBuktiID *uuid.UUID
	if req.DokumenBuktiID != nil {
		id, err := uuid.Parse(*req.DokumenBuktiID)
		if err != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed, "dokumenBuktiId bukan UUID valid.")
		}
		dokumenBuktiID = &id
	}

	now := time.Now()
	rh := &RatingHistory{
		ID:                     uuid.New(),
		RatingHistoryIDKode:    req.RatingHistoryIDKode,
		CounterpartyID:         counterpartyID,
		TanggalBerlaku:         req.TanggalBerlaku,
		TanggalBerakhir:        nil, // new rating starts as active candidate
		RatingPefindo:          req.RatingPefindo,
		RatingOutlook:          req.RatingOutlook,
		SumberRating:           req.SumberRating,
		TanggalPublikasiRating: req.TanggalPublikasiRating,
		ActionType:             ActionType(req.ActionType),
		NotchChange:            req.NotchChange,
		SicrTriggered:          false, // computed at approve
		DefaultTriggered:       false, // computed at approve
		DokumenBuktiID:         dokumenBuktiID,
		MakerID:                actorID,
		CreatedAt:              now,
		CreatedBy:              &actorID,
		RowVersion:             1,
		TenantID:               tenantID(claims),
		WorkflowStatus:         WorkflowStatusDraft,
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create ratinghistory: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, rh); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrKodeDuplicate || containsStr(err.Error(), "duplicate") {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Kode rating history %s sudah terdaftar.", req.RatingHistoryIDKode),
				domainerrors.Detail{Field: "body.ratingHistoryIdKode", Rule: "unique",
					Message: fmt.Sprintf("Kode %s sudah ada", req.RatingHistoryIDKode)},
			)
		}
		return nil, fmt.Errorf("service.Create ratinghistory: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "RATING_HISTORY.CREATE",
		EntityType: "mst.rating_history_counterparty",
		EntityID:   rh.ID,
		After:      ratingHistoryAuditMap(rh),
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create ratinghistory: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create ratinghistory: commit: %w", err)
	}
	return rh, nil
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID fetches one record.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*RatingHistory, error) {
	rh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID ratinghistory: %w", err)
	}
	if rh == nil {
		return nil, domainerrors.ErrNotFound("Rating history " + id.String())
	}
	return rh, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the paginated list result.
type ListResult struct {
	Items      []*RatingHistory
	Pagination pagination.Result
}

// List returns paginated records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("service.List ratinghistory: %w", err)
	}
	fetchedCount := len(items)
	lastID := ""
	if fetchedCount > limit {
		items = items[:limit]
		lastID = items[limit-1].RatingHistoryIDKode
	}
	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ListByCounterparty returns paginated records for one counterparty.
func (s *Service) ListByCounterparty(ctx context.Context, counterpartyID uuid.UUID, cursor string, limit int) (*ListResult, error) {
	items, err := s.repo.ListByCounterparty(ctx, counterpartyID, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("service.ListByCounterparty: %w", err)
	}
	fetchedCount := len(items)
	lastID := ""
	if fetchedCount > limit {
		items = items[:limit]
		lastID = items[limit-1].RatingHistoryIDKode
	}
	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update applies partial update (only on DRAFT/RETURNED records).
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*RatingHistory, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

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
		return nil, domainerrors.ErrNotFound("Rating history " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"Rating history sudah disetujui dan tidak bisa diedit langsung.",
		)
	}

	before := ratingHistoryAuditMap(current)

	f := UpdateFields{
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
	}
	if req.RatingPefindo != nil {
		f.RatingPefindo = req.RatingPefindo
	}
	if req.RatingOutlook != nil {
		f.RatingOutlook = req.RatingOutlook
	}
	if req.SumberRating != nil {
		f.SumberRating = req.SumberRating
	}
	if req.TanggalPublikasiRating != nil {
		f.TanggalPublikasiRating = req.TanggalPublikasiRating
	}
	if req.ActionType != nil {
		at := ActionType(*req.ActionType)
		f.ActionType = &at
	}
	if req.NotchChange != nil {
		f.NotchChange = req.NotchChange
	}
	if req.DokumenBuktiID != nil {
		id2, err := uuid.Parse(*req.DokumenBuktiID)
		if err == nil {
			f.DokumenBuktiID = &id2
		}
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update ratinghistory: begin tx: %w", err)
	}

	updated, err := s.repo.Update(ctx, tx, id, f)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Rating history " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update ratinghistory: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "RATING_HISTORY.UPDATE",
		EntityType: "mst.rating_history_counterparty",
		EntityID:   updated.ID,
		Before:     before,
		After:      ratingHistoryAuditMap(updated),
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update ratinghistory: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update ratinghistory: commit: %w", err)
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
		return domainerrors.ErrNotFound("Rating history " + id.String())
	}

	if existing.WorkflowStatus == WorkflowStatusApproved {
		return domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"Rating history yang sudah disetujui tidak bisa dihapus. Ajukan koreksi via workflow baru.",
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete ratinghistory: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete ratinghistory: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "RATING_HISTORY.DELETE",
		EntityType: "mst.rating_history_counterparty",
		EntityID:   id,
		Before:     ratingHistoryAuditMap(existing),
		After:      ratingHistoryAuditMap(deleted),
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete ratinghistory: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete ratinghistory: commit: %w", err)
	}
	return nil
}

// ─── SyncWorkflowStatus ───────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the workflow engine EntityHook.
// On APPROVED transition:
//   1. Compute SICR using notch_change and previous active rating.
//   2. Close previous active rating (set tanggal_berakhir = this rating's tanggal_berlaku - 1 day).
//   3. Set sicr_triggered + default_triggered on this record.
//   4. Update counterparty.rating_pefindo_current (cached field) in same tx.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	rh, err := s.repo.GetByID(ctx, entityID)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if rh == nil {
		return domainerrors.ErrNotFound("Rating history entity")
	}

	auditAction := "RATING_HISTORY." + action

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus ratinghistory: begin tx: %w", err)
	}

	before := map[string]interface{}{"workflow_status": string(rh.WorkflowStatus)}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus ratinghistory: %w", err)
	}

	// On APPROVED: compute SICR, close previous rating, update counterparty cache
	if wfStatus == WorkflowStatusApproved {
		if sErr := s.handleApproveTransition(ctx, tx, rh, actorID); sErr != nil {
			rollbackTx(ctx, tx, s.logger)
			return fmt.Errorf("service.SyncWorkflowStatus approve actions: %w", sErr)
		}
	}

	after := map[string]interface{}{"workflow_status": string(wfStatus)}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.rating_history_counterparty",
		EntityID:   entityID,
		Before:     before,
		After:      after,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus ratinghistory: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus ratinghistory: commit: %w", err)
	}
	return nil
}

// handleApproveTransition executes SICR computation + counterparty rating cache update.
// Called inside an open transaction.
func (s *Service) handleApproveTransition(ctx context.Context, tx *sql.Tx, rh *RatingHistory, actorID uuid.UUID) error {
	// Fetch previous active rating (to determine IG→non-IG transition)
	previousActive, err := s.repo.GetActiveByCounterparty(ctx, rh.CounterpartyID)
	if err != nil {
		return fmt.Errorf("handleApproveTransition: get active rating: %w", err)
	}

	previousRating := ""
	if previousActive != nil {
		previousRating = previousActive.RatingPefindo
	}

	// Compute SICR
	sicrTriggered, defaultTriggered := ComputeSICR(rh.NotchChange, previousRating, rh.RatingPefindo)

	s.logger.InfoContext(ctx, "ratinghistory approve: SICR computed",
		"entityID", rh.ID,
		"notchChange", rh.NotchChange,
		"previousRating", previousRating,
		"newRating", rh.RatingPefindo,
		"sicrTriggered", sicrTriggered,
		"defaultTriggered", defaultTriggered,
	)

	// Set SICR flags on this record
	if err := s.repo.SetSICRFlags(ctx, tx, rh.ID, sicrTriggered, defaultTriggered, actorID); err != nil {
		return fmt.Errorf("handleApproveTransition: set SICR flags: %w", err)
	}

	// Close previous active rating (if any) — set tanggal_berakhir = tanggal_berlaku - 1 day
	if previousActive != nil {
		// Parse tanggal_berlaku (YYYY-MM-DD) to compute tanggal_berakhir
		newBerlaku, parseErr := time.Parse("2006-01-02", rh.TanggalBerlaku)
		if parseErr == nil {
			tanggalBerakhir := newBerlaku.AddDate(0, 0, -1).Format("2006-01-02")
			if err := s.repo.CloseActiveRating(ctx, tx, rh.CounterpartyID, tanggalBerakhir, actorID); err != nil {
				return fmt.Errorf("handleApproveTransition: close active rating: %w", err)
			}
		} else {
			s.logger.WarnContext(ctx, "ratinghistory approve: could not parse tanggal_berlaku, skipping close",
				"entityID", rh.ID,
				"tanggalBerlaku", rh.TanggalBerlaku,
				"error", parseErr,
			)
		}
	}

	// Update counterparty.rating_pefindo_current (cached field)
	if s.cpRepo != nil {
		newRating := rh.RatingPefindo
		if err := s.cpRepo.UpdateRatingCache(ctx, tx, rh.CounterpartyID, &newRating, actorID); err != nil {
			// Non-blocking for cache update — log but don't fail the tx
			s.logger.WarnContext(ctx, "ratinghistory approve: UpdateRatingCache failed",
				"counterpartyID", rh.CounterpartyID,
				"error", err,
			)
		}
	}

	return nil
}

// ─── ListHistory ──────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a rating_history entity.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	rh, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if rh == nil {
		return nil, false, domainerrors.ErrNotFound("Rating history " + id.String())
	}
	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── ExportCSV ────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV ratinghistory: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "RATING_HISTORY.EXPORT",
			EntityType:  "mst.rating_history_counterparty",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "ratinghistory ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "ratinghistory ExportCSV: audit commit failed", "error", commitErr)
		}
	}
	return reader, count, nil
}

// ─── Validation ───────────────────────────────────────────────────────────────

func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if len(req.RatingHistoryIDKode) < 3 || len(req.RatingHistoryIDKode) > 20 {
		details = append(details, domainerrors.Detail{
			Field:   "body.ratingHistoryIdKode",
			Rule:    "length",
			Message: "Kode harus 3-20 karakter",
		})
	}
	if req.CounterpartyID == "" {
		details = append(details, domainerrors.Detail{
			Field:   "body.counterpartyId",
			Rule:    "required",
			Message: "counterpartyId wajib diisi",
		})
	}
	if !dateRe.MatchString(req.TanggalBerlaku) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalBerlaku",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if !dateRe.MatchString(req.TanggalPublikasiRating) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalPublikasiRating",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if len(req.RatingPefindo) == 0 || len(req.RatingPefindo) > 8 {
		details = append(details, domainerrors.Detail{
			Field:   "body.ratingPefindo",
			Rule:    "length",
			Message: "Rating Pefindo harus 1-8 karakter",
		})
	}
	if !IsValidActionType(req.ActionType) {
		details = append(details, domainerrors.Detail{
			Field:   "body.actionType",
			Rule:    "oneof",
			Message: "Action type tidak valid. Nilai yang diizinkan: INITIAL, UPGRADE, DOWNGRADE, AFFIRMED, WITHDRAWN, CORRECTION",
		})
	}
	if len(req.SumberRating) == 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.sumberRating",
			Rule:    "required",
			Message: "Sumber rating wajib diisi",
		})
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}
	return nil
}

func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.TanggalPublikasiRating != nil && !dateRe.MatchString(*req.TanggalPublikasiRating) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalPublikasiRating",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.ActionType != nil && !IsValidActionType(*req.ActionType) {
		details = append(details, domainerrors.Detail{
			Field:   "body.actionType",
			Rule:    "oneof",
			Message: "Action type tidak valid",
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

func containsStr(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ratingHistoryAuditMap builds an audit-safe map (no PII).
func ratingHistoryAuditMap(rh *RatingHistory) map[string]interface{} {
	m := map[string]interface{}{
		"id":                       rh.ID.String(),
		"rating_history_id_kode":   rh.RatingHistoryIDKode,
		"counterparty_id":          rh.CounterpartyID.String(),
		"tanggal_berlaku":          rh.TanggalBerlaku,
		"rating_pefindo":           rh.RatingPefindo,
		"action_type":              string(rh.ActionType),
		"notch_change":             rh.NotchChange,
		"sicr_triggered":           rh.SicrTriggered,
		"default_triggered":        rh.DefaultTriggered,
		"workflow_status":          string(rh.WorkflowStatus),
		"row_version":              rh.RowVersion,
	}
	if rh.TanggalBerakhir != nil {
		m["tanggal_berakhir"] = *rh.TanggalBerakhir
	}
	return m
}
