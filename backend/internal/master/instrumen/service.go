package instrumen

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx rolls back a transaction, logging on failure.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "instrumen service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for mst.instrumen.
// It manages transaction boundaries; repo methods are called within transactions.
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

// Create validates and persists a new Instrumen in DRAFT state.
// FK validations: counterparty APPROVED, portofolio APPROVED, mata_uang APPROVED.
// tipe-specific: kustodian required for SAHAM/REKSADANA; manajer required for REKSADANA.
// Writes audit INSTRUMEN.CREATE in the same transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Instrumen, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Parse FK UUIDs
	counterpartyID, err := uuid.Parse(req.CounterpartyID)
	if err != nil {
		return nil, validationErr("counterpartyId", "uuid", "counterpartyId bukan UUID valid")
	}
	portofolioID, err := uuid.Parse(req.PortofolioID)
	if err != nil {
		return nil, validationErr("portofolioId", "uuid", "portofolioId bukan UUID valid")
	}

	// FK existence + APPROVED checks
	if err := s.checkCounterparty(ctx, counterpartyID); err != nil {
		return nil, err
	}
	approved, bmDefault, err := s.repo.CheckPortofolioApproved(ctx, portofolioID)
	if err != nil {
		return nil, fmt.Errorf("service.Create: check portofolio: %w", err)
	}
	if !approved {
		return nil, domainerrors.New(domainerrors.CodeInstrumenPortofolioNotApproved,
			fmt.Sprintf("Portofolio %s tidak ditemukan atau belum APPROVED. Pastikan portofolio sudah disetujui sebelum membuat instrumen.", req.PortofolioID))
	}
	if err := s.checkMataUang(ctx, strings.ToUpper(req.MataUang)); err != nil {
		return nil, err
	}

	// Optional FK: manajer_investasi + bank_kustodian
	var manajerID *uuid.UUID
	if req.ManajerInvestasiID != nil {
		id, err := uuid.Parse(*req.ManajerInvestasiID)
		if err != nil {
			return nil, validationErr("manajerInvestasiId", "uuid", "manajerInvestasiId bukan UUID valid")
		}
		if err := s.checkCounterpartyExists(ctx, id); err != nil {
			return nil, err
		}
		manajerID = &id
	}
	var kustodianID *uuid.UUID
	if req.BankKustodianID != nil {
		id, err := uuid.Parse(*req.BankKustodianID)
		if err != nil {
			return nil, validationErr("bankKustodianId", "uuid", "bankKustodianId bukan UUID valid")
		}
		if err := s.checkCounterpartyExists(ctx, id); err != nil {
			return nil, err
		}
		kustodianID = &id
	}

	// Parse decimal fields
	nominal, err := decimal.NewFromString(req.Nominal)
	if err != nil || nominal.IsNegative() || nominal.IsZero() {
		return nil, validationErr("nominal", "positive_decimal", "nominal harus angka positif (format: 1000000.00)")
	}

	var jumlahLot *decimal.Decimal
	if req.JumlahLot != nil {
		d, err := decimal.NewFromString(*req.JumlahLot)
		if err != nil {
			return nil, validationErr("jumlahLot", "decimal", "jumlahLot harus angka valid")
		}
		jumlahLot = &d
	}

	var kupon *decimal.Decimal
	if req.Kupon != nil {
		d, err := decimal.NewFromString(*req.Kupon)
		if err != nil || d.IsNegative() {
			return nil, validationErr("kupon", "nonneg_decimal", "kupon harus angka non-negatif")
		}
		kupon = &d
	}

	var eirAwal *decimal.Decimal
	if req.EirAwal != nil {
		d, err := decimal.NewFromString(*req.EirAwal)
		if err != nil || d.IsNegative() || d.GreaterThanOrEqual(decimal.NewFromInt(1)) {
			return nil, validationErr("eirAwal", "range", "eirAwal harus dalam rentang [0, 1)")
		}
		eirAwal = &d
	}

	premium := decimal.Zero
	if req.PremiumDiskonto != nil {
		d, err := decimal.NewFromString(*req.PremiumDiskonto)
		if err != nil {
			return nil, validationErr("premiumDiskonto", "decimal", "premiumDiskonto harus angka valid")
		}
		premium = d
	}

	biaya := decimal.Zero
	if req.BiayaTransaksi != nil {
		d, err := decimal.NewFromString(*req.BiayaTransaksi)
		if err != nil {
			return nil, validationErr("biayaTransaksi", "decimal", "biayaTransaksi harus angka valid")
		}
		biaya = d
	}

	// Resolve bm_category: explicit in request → default from portofolio
	var bmCategory *string
	if req.BmCategory != nil {
		bmCategory = req.BmCategory
	} else if bmDefault != nil {
		bmCategory = bmDefault
	}

	autoRenewal := false
	if req.AutoRenewalFlag != nil {
		autoRenewal = *req.AutoRenewalFlag
	}
	fvociElection := false
	if req.FvociElection != nil {
		fvociElection = *req.FvociElection
	}
	eirMethod := true
	if req.EirMethodFlag != nil {
		eirMethod = *req.EirMethodFlag
	}
	dayCount := "ACT/365"
	if req.DayCountConvention != nil {
		dayCount = *req.DayCountConvention
	}
	status := "AKTIF"
	if req.Status != nil {
		status = *req.Status
	}

	m := &Instrumen{
		ID:                    uuid.New(),
		KodeInstrumen:         req.KodeInstrumen,
		TipeInstrumen:         strings.ToUpper(req.TipeInstrumen),
		SubTipe:               req.SubTipe,
		Nama:                  req.Nama,
		ISIN:                  req.ISIN,
		CounterpartyID:        counterpartyID,
		ManajerInvestasiID:    manajerID,
		BankKustodianID:       kustodianID,
		MataUang:              strings.ToUpper(req.MataUang),
		PortofolioID:          portofolioID,
		Nominal:               nominal,
		JumlahLot:             jumlahLot,
		TanggalPenempatan:     req.TanggalPenempatan,
		TanggalJatuhTempo:     req.TanggalJatuhTempo,
		Kupon:                 kupon,
		FrekuensiBunga:        req.FrekuensiBunga,
		AutoRenewalFlag:       autoRenewal,
		FvociElection:         fvociElection,
		BmCategory:            bmCategory,
		EirAwal:               eirAwal,
		PremiumDiskonto:       premium,
		BiayaTransaksi:        biaya,
		EirMethodFlag:         eirMethod,
		DayCountConvention:    dayCount,
		AmortizationFrequency: req.AmortizationFrequency,
		Status:                status,
		WorkflowStatus:        WorkflowStatusDraft,
		CreatedAt:             time.Now(),
		CreatedBy:             actorID,
		RowVersion:            1,
		TenantID:              tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create instrumen: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, m); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if isKodeDuplicate(err) {
			return nil, domainerrors.New(domainerrors.CodeInstrumenDuplicateKode,
				fmt.Sprintf("Instrumen dengan kode %s sudah terdaftar.", req.KodeInstrumen),
				domainerrors.Detail{Field: "body.kodeInstrumen", Rule: "unique",
					Message: fmt.Sprintf("Kode instrumen %s sudah ada", req.KodeInstrumen)},
			)
		}
		return nil, fmt.Errorf("service.Create instrumen: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "INSTRUMEN.CREATE",
		EntityType: "mst.instrumen",
		EntityID:   m.ID,
		After:      m,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create instrumen: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create instrumen: commit: %w", err)
	}
	return m, nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetByID fetches one record by UUID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Instrumen, error) {
	m, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID instrumen: %w", err)
	}
	if m == nil {
		return nil, domainerrors.ErrNotFound("Instrumen " + id.String())
	}
	return m, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is returned by List.
type ListResult struct {
	Items      []*Instrumen
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List instrumen: %w", err)
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
// Guards: workflow_status MUST be editable; row_version optimistic lock;
// klasifikasi_locked_at IS NULL if klasifikasi fields are being changed.
// Writes audit INSTRUMEN.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*Instrumen, error) {
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
		return nil, fmt.Errorf("service.Update instrumen: load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Instrumen " + id.String())
	}

	// Guard: workflow status
	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Instrumen %s sudah disetujui. Ajukan perubahan melalui workflow.", current.KodeInstrumen))
	}

	// Guard: klasifikasi locked
	if current.KlasifikasiLockedAt != nil {
		if req.FvociElection != nil || req.BmCategory != nil {
			return nil, domainerrors.New(domainerrors.CodeInstrumenKlasifikasiLocked,
				fmt.Sprintf("Instrumen %s telah dikunci klasifikasinya. Perubahan fvoci_election atau bm_category harus melalui workflow SPPI/BM.", current.KodeInstrumen))
		}
	}

	before := *current

	// Optional FK validation for updatable FK fields
	var manajerID *uuid.UUID
	if req.ManajerInvestasiID != nil {
		id2, err := uuid.Parse(*req.ManajerInvestasiID)
		if err != nil {
			return nil, validationErr("manajerInvestasiId", "uuid", "bukan UUID valid")
		}
		if err := s.checkCounterpartyExists(ctx, id2); err != nil {
			return nil, err
		}
		manajerID = &id2
	}
	var kustodianID *uuid.UUID
	if req.BankKustodianID != nil {
		id2, err := uuid.Parse(*req.BankKustodianID)
		if err != nil {
			return nil, validationErr("bankKustodianId", "uuid", "bukan UUID valid")
		}
		if err := s.checkCounterpartyExists(ctx, id2); err != nil {
			return nil, err
		}
		kustodianID = &id2
	}
	if req.MataUang != nil {
		if err := s.checkMataUang(ctx, strings.ToUpper(*req.MataUang)); err != nil {
			return nil, err
		}
	}

	// Parse decimal update fields
	var kupon *decimal.Decimal
	if req.Kupon != nil {
		d, err := decimal.NewFromString(*req.Kupon)
		if err != nil || d.IsNegative() {
			return nil, validationErr("kupon", "nonneg_decimal", "kupon harus angka non-negatif")
		}
		kupon = &d
	}
	var eirAwal *decimal.Decimal
	if req.EirAwal != nil {
		d, err := decimal.NewFromString(*req.EirAwal)
		if err != nil || d.IsNegative() || d.GreaterThanOrEqual(decimal.NewFromInt(1)) {
			return nil, validationErr("eirAwal", "range", "eirAwal harus dalam rentang [0, 1)")
		}
		eirAwal = &d
	}

	fields := UpdateFields{
		SubTipe:               req.SubTipe,
		Nama:                  req.Nama,
		ISIN:                  req.ISIN,
		ManajerInvestasiID:    manajerID,
		BankKustodianID:       kustodianID,
		MataUang:              req.MataUang,
		Kupon:                 kupon,
		FrekuensiBunga:        req.FrekuensiBunga,
		AutoRenewalFlag:       req.AutoRenewalFlag,
		FvociElection:         req.FvociElection,
		BmCategory:            req.BmCategory,
		EirAwal:               eirAwal,
		DayCountConvention:    req.DayCountConvention,
		AmortizationFrequency: req.AmortizationFrequency,
		Status:                req.Status,
		UpdatedBy:             actorID,
		ExpectedVersion:       req.RowVersion,
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update instrumen: begin tx: %w", err)
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Instrumen " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update instrumen: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "INSTRUMEN.UPDATE",
		EntityType: "mst.instrumen",
		EntityID:   id,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update instrumen: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update instrumen: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
// Guard: no active transactions referencing this instrumen.
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete instrumen: load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Instrumen " + id.String())
	}

	refCount, err := s.repo.CountActiveTransactions(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete instrumen: count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(domainerrors.CodeEntityInUse,
			fmt.Sprintf("Instrumen %s tidak bisa dihapus karena masih memiliki %d transaksi aktif.",
				existing.KodeInstrumen, refCount),
			domainerrors.Detail{Field: "id", Rule: "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d transaksi aktif", refCount)},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete instrumen: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete instrumen: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "INSTRUMEN.DELETE",
		EntityType: "mst.instrumen",
		EntityID:   id,
		Before:     existing,
		After:      deleted,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete instrumen: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete instrumen: commit: %w", err)
	}
	return nil
}

// ─── Workflow sync ────────────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine after a state transition.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	m, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus instrumen: load: %w", err)
	}
	if m == nil {
		return domainerrors.ErrNotFound("Instrumen entity")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus instrumen: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus instrumen: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "INSTRUMEN." + action,
		EntityType: "mst.instrumen",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(m.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": string(wfStatus)},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus instrumen: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus instrumen: commit: %w", err)
	}
	return nil
}

// ─── History ──────────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given instrumen UUID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory instrumen: load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Instrumen " + id.String())
	}
	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all matching records as CSV, writes audit INSTRUMEN.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV instrumen: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "INSTRUMEN.EXPORT",
			EntityType:  "mst.instrumen",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "instrumen ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "instrumen ExportCSV: audit commit failed", "error", commitErr)
		}
	}
	return reader, count, nil
}

// ─── Field validation ─────────────────────────────────────────────────────────

func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if !AllowedTipeInstrumen[strings.ToUpper(req.TipeInstrumen)] {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipeInstrumen",
			Rule:    "oneof",
			Message: fmt.Sprintf("tipeInstrumen tidak valid. Gunakan salah satu: %s", joinKeys(AllowedTipeInstrumen)),
		})
	}

	tipe := strings.ToUpper(req.TipeInstrumen)
	if TipeInstrumenRequiresKustodian[tipe] && req.BankKustodianID == nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.bankKustodianId",
			Rule:    "required_for_tipe",
			Message: fmt.Sprintf("bankKustodianId wajib diisi untuk tipe instrumen %s.", tipe),
		})
	}
	if TipeInstrumenRequiresManajerInvestasi[tipe] && req.ManajerInvestasiID == nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.manajerInvestasiId",
			Rule:    "required_for_tipe",
			Message: fmt.Sprintf("manajerInvestasiId wajib diisi untuk tipe instrumen %s.", tipe),
		})
	}

	if req.TanggalPenempatan != "" && !isDateStr(req.TanggalPenempatan) {
		details = append(details, domainerrors.Detail{
			Field: "body.tanggalPenempatan", Rule: "format",
			Message: "tanggalPenempatan harus dalam format YYYY-MM-DD",
		})
	}
	if req.TanggalJatuhTempo != nil && !isDateStr(*req.TanggalJatuhTempo) {
		details = append(details, domainerrors.Detail{
			Field: "body.tanggalJatuhTempo", Rule: "format",
			Message: "tanggalJatuhTempo harus dalam format YYYY-MM-DD",
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

	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field: "body.rowVersion", Rule: "required",
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

// ─── FK check helpers ─────────────────────────────────────────────────────────

// checkCounterparty verifies primary counterparty existence + APPROVED status.
func (s *Service) checkCounterparty(ctx context.Context, id uuid.UUID) error {
	approved, err := s.repo.CheckCounterpartyApproved(ctx, id)
	if err != nil {
		return fmt.Errorf("service.checkCounterparty: %w", err)
	}
	if !approved {
		return domainerrors.New(domainerrors.CodeInstrumenCounterpartyNotApproved,
			fmt.Sprintf("Counterparty %s tidak ditemukan atau belum APPROVED.", id))
	}
	return nil
}

// checkCounterpartyExists verifies a counterparty UUID exists and is APPROVED.
// Used for optional FK fields (manajer_investasi, bank_kustodian).
// Per domain: referenced entities must be APPROVED even when the FK is optional.
func (s *Service) checkCounterpartyExists(ctx context.Context, id uuid.UUID) error {
	return s.checkCounterparty(ctx, id)
}

// checkMataUang verifies mata_uang existence + APPROVED status.
func (s *Service) checkMataUang(ctx context.Context, kode string) error {
	approved, err := s.repo.CheckMataUangApproved(ctx, kode)
	if err != nil {
		return fmt.Errorf("service.checkMataUang: %w", err)
	}
	if !approved {
		return domainerrors.New(domainerrors.CodeInstrumenMataUangNotApproved,
			fmt.Sprintf("Mata uang %s tidak ditemukan atau belum APPROVED.", kode))
	}
	return nil
}

// ─── Misc helpers ─────────────────────────────────────────────────────────────

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

// isKodeDuplicate unwraps to check for kode duplicate error.
func isKodeDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "instrumen kode duplicate") ||
		strings.Contains(err.Error(), "duplicate")
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

// validationErr constructs a single-field VALIDATION_FAILED error.
func validationErr(field, rule, msg string) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.CodeValidationFailed, msg,
		domainerrors.Detail{Field: "body." + field, Rule: rule, Message: msg})
}

// isDateStr checks for YYYY-MM-DD format (basic).
func isDateStr(s string) bool {
	if len(s) != 10 {
		return false
	}
	return s[4] == '-' && s[7] == '-'
}

// joinKeys produces a comma-separated list of map keys.
func joinKeys(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
