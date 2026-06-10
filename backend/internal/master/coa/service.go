package coa

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

// kodeAkunRe validates kode_akun format: dotted hierarchy of digits e.g. "1", "1.1", "1.1.01.001".
var kodeAkunRe = regexp.MustCompile(`^\d+(\.\d+)*$`)

// dateRe validates YYYY-MM-DD date string.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// rollbackTx attempts to rollback and logs on failure.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "coa service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for chart_of_accounts.
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

// Create validates and persists a new ChartOfAccount in DRAFT state.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*ChartOfAccount, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Resolve optional parent_akun_kode → UUID
	var parentID *uuid.UUID
	if req.ParentAkunKode != nil && *req.ParentAkunKode != "" {
		parent, err := s.repo.GetByKode(ctx, *req.ParentAkunKode, false)
		if err != nil {
			return nil, fmt.Errorf("service.Create: lookup parent: %w", err)
		}
		if parent == nil || parent.WorkflowStatus != WorkflowStatusApproved {
			return nil, domainerrors.New(domainerrors.CodeCoAParentNotFound,
				fmt.Sprintf("Parent akun dengan kode '%s' tidak ditemukan atau belum disetujui (APPROVED).", *req.ParentAkunKode),
				domainerrors.Detail{Field: "body.parentAkunKode", Rule: "exists_approved",
					Message: "Parent harus terdaftar dan berstatus APPROVED"},
			)
		}
		parentID = &parent.ID
	}

	mataUang := "IDR"
	if req.MataUangNative != "" {
		mataUang = strings.ToUpper(req.MataUangNative)
	}

	aktifFlag := true
	if req.AktifFlag != nil {
		aktifFlag = *req.AktifFlag
	}

	now := time.Now()
	c := &ChartOfAccount{
		ID:                uuid.New(),
		KodeAkun:          req.KodeAkun,
		NamaAkun:          req.NamaAkun,
		TipeAkun:          TipeAkun(req.TipeAkun),
		SubTipeAkun:       req.SubTipeAkun,
		KategoriInvestasi: req.KategoriInvestasi,
		MataUangNative:    mataUang,
		PosisiNormal:      PosisiNormal(req.PosisiNormal),
		AktifFlag:         aktifFlag,
		ParentAkunID:      parentID,
		SumberCoa:         req.SumberCoa,
		TanggalMulaiAktif: req.TanggalMulaiAktif,
		WorkflowStatus:    WorkflowStatusDraft,
		CreatedBy:         actorID,
		CreatedAt:         now,
		Version:           1,
		TenantID:          tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, c); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if strings.Contains(err.Error(), ErrKodeDuplicate.Error()) || isErrDuplicate(err) {
			return nil, domainerrors.New(domainerrors.CodeCoADuplicateKode,
				fmt.Sprintf("Kode akun '%s' sudah terdaftar di sistem.", req.KodeAkun),
				domainerrors.Detail{Field: "body.kodeAkun", Rule: "unique",
					Message: fmt.Sprintf("Kode akun %s sudah ada", req.KodeAkun)},
			)
		}
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "CHART_OF_ACCOUNTS.CREATE",
		EntityType: "mst.chart_of_accounts",
		EntityID:   c.ID,
		After:      c,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}
	return c, nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*ChartOfAccount, error) {
	c, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if c == nil {
		return nil, domainerrors.ErrNotFound("Chart of Account")
	}
	return c, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*ChartOfAccount
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
		lastID = items[limit-1].KodeAkun
	}

	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies a partial update.
// Guard: workflow_status MUST be DRAFT or RETURNED; row_version optimistic lock.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*ChartOfAccount, error) {
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
		return nil, domainerrors.ErrNotFound("Chart of Account")
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"Akun ini sudah disetujui dan tidak bisa diedit langsung. Ajukan perubahan melalui workflow.",
		)
	}

	before := *current

	// Resolve optional parent_akun_kode change
	var newParentID *uuid.UUID
	clearParent := false
	if req.ParentAkunKode != nil {
		if *req.ParentAkunKode == "" {
			clearParent = true
		} else {
			parent, err := s.repo.GetByKode(ctx, *req.ParentAkunKode, false)
			if err != nil {
				return nil, fmt.Errorf("service.Update: lookup parent: %w", err)
			}
			if parent == nil || parent.WorkflowStatus != WorkflowStatusApproved {
				return nil, domainerrors.New(domainerrors.CodeCoAParentNotFound,
					fmt.Sprintf("Parent akun dengan kode '%s' tidak ditemukan atau belum disetujui.", *req.ParentAkunKode),
					domainerrors.Detail{Field: "body.parentAkunKode", Rule: "exists_approved"},
				)
			}
			newParentID = &parent.ID
		}
	}

	var newPosisi *PosisiNormal
	if req.PosisiNormal != nil {
		p := PosisiNormal(*req.PosisiNormal)
		newPosisi = &p
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	fields := UpdateFields{
		NamaAkun:          req.NamaAkun,
		SubTipeAkun:       req.SubTipeAkun,
		KategoriInvestasi: req.KategoriInvestasi,
		MataUangNative:    req.MataUangNative,
		PosisiNormal:      newPosisi,
		AktifFlag:         req.AktifFlag,
		ParentAkunID:      newParentID,
		ClearParent:       clearParent,
		SumberCoa:         req.SumberCoa,
		TanggalMulaiAktif: req.TanggalMulaiAktif,
		UpdatedBy:         actorID,
		ExpectedVersion:   req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Chart of Account")
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "CHART_OF_ACCOUNTS.UPDATE",
		EntityType: "mst.chart_of_accounts",
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
// Guard: cannot delete if it has child accounts.
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
		return domainerrors.ErrNotFound("Chart of Account")
	}

	childCount, err := s.repo.CountChildrenOf(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count children: %w", err)
	}
	if childCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Akun ini tidak bisa dihapus karena memiliki %d akun anak. Hapus atau pindahkan akun anak terlebih dahulu.", childCount),
			domainerrors.Detail{
				Field:   "id",
				Rule:    "has_children",
				Message: fmt.Sprintf("Direferensikan oleh %d akun anak", childCount),
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
		Action:     "CHART_OF_ACCOUNTS.DELETE",
		EntityType: "mst.chart_of_accounts",
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

// ─── SyncWorkflowStatus ──────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the EntityHook (workflow engine post-transition).
// It updates mst.chart_of_accounts.workflow_status.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	m, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if m == nil {
		return domainerrors.ErrNotFound("Chart of Account entity")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "CHART_OF_ACCOUNTS." + action,
		EntityType: "mst.chart_of_accounts",
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

// ListHistory returns paginated audit log for the given CoA entity.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Chart of Account")
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit CHART_OF_ACCOUNTS.EXPORT.
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
			Action:      "CHART_OF_ACCOUNTS.EXPORT",
			EntityType:  "mst.chart_of_accounts",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "coa ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "coa ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if !kodeAkunRe.MatchString(req.KodeAkun) {
		details = append(details, domainerrors.Detail{
			Field:   "body.kodeAkun",
			Rule:    "pattern",
			Message: "Kode akun harus berupa angka dengan titik sebagai separator hierarki, contoh: 1.1.01.001",
		})
	}
	if !validTipeAkun[TipeAkun(req.TipeAkun)] {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipeAkun",
			Rule:    "oneof",
			Message: "Tipe akun harus salah satu dari: ASET, LIABILITAS, EKUITAS, PENDAPATAN, BEBAN, KONTINJEN",
		})
	}
	if !validPosisiNormal[PosisiNormal(req.PosisiNormal)] {
		details = append(details, domainerrors.Detail{
			Field:   "body.posisiNormal",
			Rule:    "oneof",
			Message: "Posisi normal harus salah satu dari: DEBIT, KREDIT",
		})
	}
	if req.TanggalMulaiAktif != "" && !dateRe.MatchString(req.TanggalMulaiAktif) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalMulaiAktif",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.MataUangNative != "" && len(req.MataUangNative) != 3 {
		details = append(details, domainerrors.Detail{
			Field:   "body.mataUangNative",
			Rule:    "len",
			Message: "Kode mata uang harus 3 karakter (ISO 4217)",
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

func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.PosisiNormal != nil && !validPosisiNormal[PosisiNormal(*req.PosisiNormal)] {
		details = append(details, domainerrors.Detail{
			Field:   "body.posisiNormal",
			Rule:    "oneof",
			Message: "Posisi normal harus salah satu dari: DEBIT, KREDIT",
		})
	}
	if req.TanggalMulaiAktif != nil && !dateRe.MatchString(*req.TanggalMulaiAktif) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalMulaiAktif",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.MataUangNative != nil && len(*req.MataUangNative) != 3 {
		details = append(details, domainerrors.Detail{
			Field:   "body.mataUangNative",
			Rule:    "len",
			Message: "Kode mata uang harus 3 karakter (ISO 4217)",
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
	case "APPROVED":
		return WorkflowStatusApproved
	case "REJECTED":
		return WorkflowStatusRejected
	default:
		return WorkflowStatus(state)
	}
}

func isErrDuplicate(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate") || strings.Contains(s, "23505") || strings.Contains(s, "unique constraint")
}
