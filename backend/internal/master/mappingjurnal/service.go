package mappingjurnal

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

// debitCreditTolerance is the acceptable rounding tolerance for sum DEBIT = sum KREDIT check.
var debitCreditTolerance = decimal.NewFromFloat(0.0001)

// eventCodeRe validates event_code format: uppercase alphanumeric + underscore.
var eventCodeRe = regexp.MustCompile(`^[A-Z0-9_]+$`)

// rollbackTx attempts to rollback a transaction, logging any error.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "mappingjurnal service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for mapping_jurnal_header + detail.
// Transaction boundary is here; repo methods are called within a tx when mutating.
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

// ─── ListResult ───────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*Header
	Pagination pagination.Result
}

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new mapping_jurnal_header + details in DRAFT state.
// Single atomic transaction: header + all details created together.
// Minimum 2 detail rows (debit+kredit pair) is enforced here.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*HeaderWithDetails, error) {
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

	headerID := uuid.New()
	h := &Header{
		ID:                   headerID,
		EventIDKode:          req.EventIDKode,
		EventCode:            req.EventCode,
		NamaEvent:            req.NamaEvent,
		KategoriEvent:        req.KategoriEvent,
		TriggerSource:        req.TriggerSource,
		TipeInstrumenBerlaku: req.TipeInstrumenBerlaku,
		KlasifikasiBerlaku:   req.KlasifikasiBerlaku,
		AktifFlag:            aktifFlag,
		Catatan:              req.Catatan,
		WorkflowStatus:       WorkflowStatusDraft,
		CreatedAt:            now,
		CreatedBy:            actorID,
		RowVersion:           1,
		TenantID:             tenantID(claims),
	}

	details, err := s.buildDetails(req.Details, headerID, actorID, now, tenantID(claims))
	if err != nil {
		return nil, err
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.CreateHeader(ctx, tx, h); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if isErrEventCodeDuplicate(err) {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Event code '%s' sudah terdaftar di sistem.", req.EventCode),
				domainerrors.Detail{Field: "body.eventCode", Rule: "unique",
					Message: fmt.Sprintf("Event code '%s' sudah ada", req.EventCode)},
			)
		}
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.repo.CreateDetails(ctx, tx, details); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create details: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "MAPPING_JURNAL.CREATE",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   h.ID,
		After:      h,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}

	return &HeaderWithDetails{Header: h, Details: details}, nil
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID fetches header + details by UUID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*HeaderWithDetails, error) {
	h, err := s.repo.GetHeaderByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if h == nil {
		return nil, domainerrors.ErrNotFound("Mapping jurnal " + id.String())
	}

	details, err := s.repo.GetDetailsByHeaderID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID details: %w", err)
	}
	return &HeaderWithDetails{Header: h, Details: details}, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// List returns paginated headers (without details — details are fetched only on GetByID).
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.ListHeaders(ctx, q, cursor, limit, includeDeleted)
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

// Update validates and applies a partial update to header + optionally replaces all details.
// Guards: workflow_status MUST be DRAFT or RETURNED; row_version optimistic lock.
// If req.Details is non-empty, all existing active details are replaced atomically.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*HeaderWithDetails, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetHeaderByID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Mapping jurnal " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"Mapping jurnal sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan perubahan ke Finance Controller untuk diproses melalui workflow.",
		)
	}

	before := *current

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	fields := HeaderUpdateFields{
		EventIDKode:     req.EventIDKode,
		EventCode:       req.EventCode,
		NamaEvent:       req.NamaEvent,
		KategoriEvent:   req.KategoriEvent,
		TriggerSource:   req.TriggerSource,
		AktifFlag:       req.AktifFlag,
		Catatan:         req.Catatan,
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
	}
	if req.TipeInstrumenBerlaku != nil {
		fields.TipeInstrumenBerlaku = req.TipeInstrumenBerlaku
		fields.TipeInstrumenSet = true
	}
	if req.KlasifikasiBerlaku != nil {
		fields.KlasifikasiBerlaku = req.KlasifikasiBerlaku
		fields.KlasifikasiSet = true
	}

	updated, err := s.repo.UpdateHeader(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Mapping jurnal " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		if isErrEventCodeDuplicate(err) {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				"Event code sudah terdaftar di sistem.",
				domainerrors.Detail{Field: "body.eventCode", Rule: "unique", Message: "Event code sudah ada"},
			)
		}
		return nil, fmt.Errorf("service.Update header: %w", err)
	}

	// Replace details if provided
	var details []*Detail
	if len(req.Details) > 0 {
		now := time.Now()
		details, err = s.buildDetails(req.Details, id, actorID, now, tenantID(claims))
		if err != nil {
			rollbackTx(ctx, tx, s.logger)
			return nil, err
		}
		if err := s.repo.BulkReplaceDetails(ctx, tx, id, details, actorID); err != nil {
			rollbackTx(ctx, tx, s.logger)
			return nil, fmt.Errorf("service.Update replace details: %w", err)
		}
	} else {
		// Fetch current details for audit + response
		details, err = s.repo.GetDetailsByHeaderID(ctx, id, false)
		if err != nil {
			rollbackTx(ctx, tx, s.logger)
			return nil, fmt.Errorf("service.Update fetch details: %w", err)
		}
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "MAPPING_JURNAL.UPDATE",
		EntityType: "mst.mapping_jurnal_header",
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
	return &HeaderWithDetails{Header: updated, Details: details}, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete soft-deletes a header. Guards: no active references in trx.transaction.
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetHeaderByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Mapping jurnal " + id.String())
	}

	refCount, err := s.repo.CountHeaderReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Mapping jurnal tidak bisa dihapus karena masih digunakan oleh %d transaksi.", refCount),
			domainerrors.Detail{
				Field:   "id",
				Rule:    "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d transaksi aktif", refCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDeleteHeader(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "MAPPING_JURNAL.DELETE",
		EntityType: "mst.mapping_jurnal_header",
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

// ─── Workflow sync ────────────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine after a state transition.
// On APPROVE transition it also enforces the debit=credit multiplier invariant.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)

	h, err := s.repo.GetHeaderByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if h == nil {
		return domainerrors.ErrNotFound("Mapping jurnal entity")
	}

	// On APPROVE: enforce debit=credit invariant + CoA APPROVED check.
	if wfStatus == WorkflowStatusApproved {
		if err := s.validateApproveInvariants(ctx, entityID); err != nil {
			return err
		}
	}

	auditAction := "MAPPING_JURNAL." + action

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
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(h.WorkflowStatus)},
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

// validateApproveInvariants checks debit=credit balance and all CoA rows are APPROVED.
func (s *Service) validateApproveInvariants(ctx context.Context, headerID uuid.UUID) error {
	details, err := s.repo.GetDetailsByHeaderID(ctx, headerID, false)
	if err != nil {
		return fmt.Errorf("validateApproveInvariants fetch details: %w", err)
	}

	var debitSum, kreditSum decimal.Decimal
	for _, d := range details {
		if d.DKIndicator == "DEBIT" {
			debitSum = debitSum.Add(d.Multiplier)
		} else {
			kreditSum = kreditSum.Add(d.Multiplier)
		}

		// Check CoA approved
		approved, cerr := s.repo.CheckCoAApproved(ctx, d.KodeAkunID)
		if cerr != nil {
			return fmt.Errorf("validateApproveInvariants CoA check: %w", cerr)
		}
		if !approved {
			return domainerrors.New(
				domainerrors.CodeMappingJurnalKodeAkunNotApproved,
				fmt.Sprintf("Akun '%s' belum disetujui (workflow_status != APPROVED). "+
					"Semua akun yang direferensikan harus berstatus APPROVED sebelum mapping dapat disetujui.", d.KodeAkunID),
				domainerrors.Detail{
					Field:   "details.kodeAkunId",
					Rule:    "coa_approved",
					Message: fmt.Sprintf("Akun %s belum APPROVED", d.KodeAkunID),
				},
			)
		}
	}

	diff := debitSum.Sub(kreditSum).Abs()
	if diff.GreaterThan(debitCreditTolerance) {
		return domainerrors.New(
			domainerrors.CodeMappingJurnalDebitCreditMismatch,
			fmt.Sprintf("Total multiplier DEBIT (%.4s) tidak sama dengan total multiplier KREDIT (%.4s). "+
				"Selisih: %.4s (toleransi: 0.0001). Sesuaikan multiplier sebelum approval.",
				debitSum.StringFixed(4), kreditSum.StringFixed(4), diff.StringFixed(4)),
			domainerrors.Detail{
				Field:   "details.multiplier",
				Rule:    "debit_kredit_balance",
				Message: fmt.Sprintf("DEBIT %.4s ≠ KREDIT %.4s", debitSum.StringFixed(4), kreditSum.StringFixed(4)),
			},
		)
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given header UUID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetHeaderByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Mapping jurnal " + id.String())
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all headers matching q as CSV. Writes audit MAPPING_JURNAL.EXPORT.
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

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "MAPPING_JURNAL.EXPORT",
			EntityType:  "mst.mapping_jurnal_header",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "mappingjurnal ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "mappingjurnal ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// buildDetails converts DetailRequest slice to []*Detail, parsing decimal multipliers.
// Returns validation error if any field is invalid.
func (s *Service) buildDetails(reqs []DetailRequest, headerID uuid.UUID, createdBy uuid.UUID, now time.Time, tenant string) ([]*Detail, error) {
	details := make([]*Detail, 0, len(reqs))
	var validationDetails []domainerrors.Detail

	for i := range reqs {
		dr := reqs[i]
		coaID, parseErr := uuid.Parse(dr.KodeAkunID)
		if parseErr != nil {
			validationDetails = append(validationDetails, domainerrors.Detail{
				Field:   fmt.Sprintf("body.details[%d].kodeAkunId", i),
				Rule:    "uuid",
				Message: "kodeAkunId harus berformat UUID v4",
			})
			continue
		}

		multiplier, decErr := decimal.NewFromString(dr.Multiplier)
		if decErr != nil {
			validationDetails = append(validationDetails, domainerrors.Detail{
				Field:   fmt.Sprintf("body.details[%d].multiplier", i),
				Rule:    "decimal",
				Message: "multiplier harus berupa angka desimal (contoh: 1.0000)",
			})
			continue
		}
		if multiplier.LessThanOrEqual(decimal.Zero) {
			validationDetails = append(validationDetails, domainerrors.Detail{
				Field:   fmt.Sprintf("body.details[%d].multiplier", i),
				Rule:    "min",
				Message: "multiplier harus lebih dari 0",
			})
			continue
		}

		aktifFlag := true
		if dr.AktifFlag != nil {
			aktifFlag = *dr.AktifFlag
		}

		mataUangPosting := dr.MataUangPosting
		if mataUangPosting == "" {
			mataUangPosting = "IDR"
		}

		d := &Detail{
			ID:                   uuid.New(),
			EventHeaderID:        headerID,
			Urutan:               dr.Urutan,
			KodeAkunID:           coaID,
			DKIndicator:          dr.DKIndicator,
			SumberAmount:         dr.SumberAmount,
			KlasifikasiFilter:    dr.KlasifikasiFilter,
			TipeInstrumenFilter:  dr.TipeInstrumenFilter,
			UnderlyingTypeFilter: dr.UnderlyingTypeFilter,
			Multiplier:           multiplier,
			MataUangPosting:      mataUangPosting,
			AktifFlag:            aktifFlag,
			Catatan:              dr.Catatan,
			CreatedAt:            now,
			CreatedBy:            createdBy,
			RowVersion:           1,
			TenantID:             tenant,
		}
		if d.TipeInstrumenFilter == nil {
			d.TipeInstrumenFilter = []string{}
		}
		details = append(details, d)
	}

	if len(validationDetails) > 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field detail tidak valid", len(validationDetails)),
			validationDetails...,
		)
	}
	return details, nil
}

// validateCreate validates the CreateRequest.
func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if !eventCodeRe.MatchString(req.EventCode) {
		details = append(details, domainerrors.Detail{
			Field:   "body.eventCode",
			Rule:    "pattern",
			Message: "eventCode harus huruf kapital, angka, atau underscore (^[A-Z0-9_]+$)",
		})
	}
	if len(req.EventIDKode) == 0 || len(req.EventIDKode) > 40 {
		details = append(details, domainerrors.Detail{
			Field:   "body.eventIdKode",
			Rule:    "length",
			Message: "eventIdKode harus 1-40 karakter",
		})
	}
	if len(req.NamaEvent) < 3 || len(req.NamaEvent) > 120 {
		details = append(details, domainerrors.Detail{
			Field:   "body.namaEvent",
			Rule:    "length",
			Message: "namaEvent harus 3-120 karakter",
		})
	}
	if len(req.Details) < 2 {
		details = append(details, domainerrors.Detail{
			Field:   "body.details",
			Rule:    "min",
			Message: "Minimal 2 baris detail (pasangan DEBIT + KREDIT) wajib ada",
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

// validateUpdate validates the UpdateRequest.
func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.EventCode != nil && !eventCodeRe.MatchString(*req.EventCode) {
		details = append(details, domainerrors.Detail{
			Field:   "body.eventCode",
			Rule:    "pattern",
			Message: "eventCode harus huruf kapital, angka, atau underscore (^[A-Z0-9_]+$)",
		})
	}
	if req.NamaEvent != nil && (len(*req.NamaEvent) < 3 || len(*req.NamaEvent) > 120) {
		details = append(details, domainerrors.Detail{
			Field:   "body.namaEvent",
			Rule:    "length",
			Message: "namaEvent harus 3-120 karakter",
		})
	}
	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
		})
	}
	if len(req.Details) > 0 && len(req.Details) < 2 {
		details = append(details, domainerrors.Detail{
			Field:   "body.details",
			Rule:    "min",
			Message: "Jika details disertakan, minimal 2 baris (pasangan DEBIT + KREDIT)",
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

// tenantID extracts tenant_id from claims.
func tenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

// isErrEventCodeDuplicate checks if the error is a duplicate event_code.
func isErrEventCodeDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return containsStr(err.Error(), "event_code duplicate") ||
		containsStr(err.Error(), "duplicate") ||
		containsStr(err.Error(), "unique")
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
