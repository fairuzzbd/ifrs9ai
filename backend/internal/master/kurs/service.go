package kurs

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

// rollbackTx is a helper that attempts to rollback a transaction.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "kurs service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for mst.kurs.
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

// Create validates and persists a new Kurs record in DRAFT state.
// Business rules:
//   - kode_mata_uang != 'IDR'
//   - mata_uang must be APPROVED
//   - kurs_tengah > 0
//   - if beli/jual present: beli ≤ tengah ≤ jual
//   - tanggal_berlaku ≤ today + 1 day
//   - periode_buku must exist for tanggal_berlaku
//
// Writes audit KURS.CREATE in the same transaction.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Kurs, error) {
	parsed, err := s.parseAndValidateCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	k := &Kurs{
		ID:               uuid.New(),
		FxRateIDKode:     buildFxRateIDKode(req.KodeMataUang, parsed.tanggalBerlaku),
		KodeMataUang:     strings.ToUpper(req.KodeMataUang),
		TanggalBerlaku:   parsed.tanggalBerlaku,
		KursBeli:         parsed.kursBeli,
		KursJual:         parsed.kursJual,
		KursTengah:       parsed.kursTengah,
		SumberKurs:       SumberKurs(req.SumberKurs),
		PeriodeBulananID: parsed.periodeID,
		LockedFlag:       false,
		MakerID:          &actorID,
		WorkflowStatus:   WorkflowStatusDraft,
		CreatedAt:        now,
		CreatedBy:        &actorID,
		RowVersion:       1,
		TenantID:         tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create kurs: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, k); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrDuplicateDate {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Kurs %s untuk tanggal %s sudah ada.", k.KodeMataUang,
					k.TanggalBerlaku.Format("2006-01-02")),
				domainerrors.Detail{Field: "body.tanggalBerlaku", Rule: "unique",
					Message: "Kurs untuk mata uang dan tanggal ini sudah terdaftar"},
			)
		}
		return nil, fmt.Errorf("service.Create kurs: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "KURS.CREATE",
		EntityType: "mst.kurs",
		EntityID:   k.ID,
		After:      k,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create kurs: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create kurs: commit: %w", err)
	}

	return k, nil
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID fetches one record by UUID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Kurs, error) {
	k, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID kurs: %w", err)
	}
	if k == nil {
		return nil, domainerrors.ErrNotFound("Kurs " + id.String())
	}
	return k, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*Kurs
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List kurs: %w", err)
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

// Update validates and applies a partial update to an existing record.
// Guards: workflow_status MUST be DRAFT or RETURNED; row_version optimistic lock.
// Writes audit KURS.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*Kurs, error) {
	parsed, err := s.parseAndValidateUpdate(req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update kurs load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Kurs " + id.String())
	}

	if current.LockedFlag {
		return nil, newKursErr(CodeKursLocked,
			"Kurs ini tidak bisa diubah karena periode buku sudah CLOSED.")
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"Kurs ini sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan perubahan melalui workflow.",
		)
	}

	// Cross-validate the merged state
	effectiveTengah := current.KursTengah
	if parsed.kursTengah != nil {
		effectiveTengah = *parsed.kursTengah
	}
	effectiveBeli := current.KursBeli
	if parsed.kursBeli != nil {
		effectiveBeli = parsed.kursBeli
	}
	effectiveJual := current.KursJual
	if parsed.kursJual != nil {
		effectiveJual = parsed.kursJual
	}

	if err := validateRates(effectiveBeli, effectiveJual, effectiveTengah); err != nil {
		return nil, err
	}

	before := *current

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update kurs: begin tx: %w", err)
	}

	fields := UpdateFields{
		KursBeli:        parsed.kursBeli,
		KursJual:        parsed.kursJual,
		KursTengah:      parsed.kursTengah,
		SumberKurs:      parsed.sumberKurs,
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrLocked {
			return nil, newKursErr(CodeKursLocked,
				"Kurs ini tidak bisa diubah karena periode buku sudah CLOSED.")
		}
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Kurs " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update kurs: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "KURS.UPDATE",
		EntityType: "mst.kurs",
		EntityID:   updated.ID,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update kurs: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update kurs: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
// Guards: locked_flag = true → reject.
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete kurs load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Kurs " + id.String())
	}

	if existing.LockedFlag {
		return newKursErr(CodeKursLocked,
			"Kurs ini tidak bisa dihapus karena periode buku sudah CLOSED.")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete kurs: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrLocked {
			return newKursErr(CodeKursLocked,
				"Kurs ini tidak bisa dihapus karena periode buku sudah CLOSED.")
		}
		return fmt.Errorf("service.SoftDelete kurs: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "KURS.DELETE",
		EntityType: "mst.kurs",
		EntityID:   deleted.ID,
		Before:     existing,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete kurs: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete kurs: commit: %w", err)
	}
	return nil
}

// CreateApproved creates a new Kurs record with APPROVED status and auto-sets approver fields.
// Used by the JISDOR integration worker (trusted source — bypasses 4-eyes workflow).
func (s *Service) CreateApproved(ctx context.Context, req CreateRequest, systemActorID uuid.UUID) (*Kurs, error) {
	parsed, err := s.parseAndValidateCreate(ctx, req)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	k := &Kurs{
		ID:               uuid.New(),
		FxRateIDKode:     buildFxRateIDKode(req.KodeMataUang, parsed.tanggalBerlaku),
		KodeMataUang:     strings.ToUpper(req.KodeMataUang),
		TanggalBerlaku:   parsed.tanggalBerlaku,
		KursBeli:         parsed.kursBeli,
		KursJual:         parsed.kursJual,
		KursTengah:       parsed.kursTengah,
		SumberKurs:       SumberKursJISDOR,
		PeriodeBulananID: parsed.periodeID,
		LockedFlag:       false,
		MakerID:          &systemActorID,
		ApproverID:       &systemActorID,
		ApprovedAt:       &now,
		WorkflowStatus:   WorkflowStatusApproved,
		CreatedAt:        now,
		CreatedBy:        &systemActorID,
		RowVersion:       1,
		TenantID:         "TUGURE",
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.CreateApproved kurs: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, k); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrDuplicateDate {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Kurs %s untuk tanggal %s sudah ada.", k.KodeMataUang,
					k.TanggalBerlaku.Format("2006-01-02")))
		}
		return nil, fmt.Errorf("service.CreateApproved kurs: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "KURS.JISDOR_AUTO_APPROVE",
		EntityType:  "mst.kurs",
		EntityID:    k.ID,
		ActorUserID: systemActorID.String(),
		After:       k,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.CreateApproved kurs: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.CreateApproved kurs: commit: %w", err)
	}
	return k, nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given kurs UUID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory kurs load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Kurs " + id.String())
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit KURS.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV kurs: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "KURS.EXPORT",
			EntityType:  "mst.kurs",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "kurs ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "kurs ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Parsed values struct ─────────────────────────────────────────────────────

type parsedCreateValues struct {
	tanggalBerlaku time.Time
	kursBeli       *decimal.Decimal
	kursJual       *decimal.Decimal
	kursTengah     decimal.Decimal
	periodeID      uuid.UUID
}

type parsedUpdateValues struct {
	kursBeli   *decimal.Decimal
	kursJual   *decimal.Decimal
	kursTengah *decimal.Decimal
	sumberKurs *SumberKurs
}

// ─── Validation helpers ───────────────────────────────────────────────────────

// parseAndValidateCreate parses and validates a CreateRequest.
func (s *Service) parseAndValidateCreate(ctx context.Context, req CreateRequest) (*parsedCreateValues, error) {
	var details []domainerrors.Detail

	// kode_mata_uang != 'IDR'
	kodeMataUang := strings.ToUpper(req.KodeMataUang)
	if kodeMataUang == "IDR" {
		return nil, newKursErr(CodeKursInvalidCurrency,
			"Kurs untuk mata uang IDR tidak diperlukan (IDR adalah currency fungsional Tugure).",
			domainerrors.Detail{Field: "body.kodeMataUang", Rule: "not_idr",
				Message: "kodeMataUang tidak boleh 'IDR'"},
		)
	}

	// sumber_kurs whitelist
	if !validSumberKurs[SumberKurs(req.SumberKurs)] {
		details = append(details, domainerrors.Detail{
			Field:   "body.sumberKurs",
			Rule:    "oneof",
			Message: "sumberKurs harus salah satu dari: BI_JISDOR, BI_KURS_TENGAH, INTERNAL, MANUAL",
		})
	}

	// Parse tanggal_berlaku
	tanggalBerlaku, err := time.Parse("2006-01-02", req.TanggalBerlaku)
	if err != nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalBerlaku",
			Rule:    "format",
			Message: "tanggalBerlaku harus format YYYY-MM-DD",
		})
	} else {
		// Sanity: tidak lebih dari today + 1 hari
		tomorrow := time.Now().AddDate(0, 0, 1)
		if tanggalBerlaku.After(tomorrow) {
			details = append(details, domainerrors.Detail{
				Field:   "body.tanggalBerlaku",
				Rule:    "max",
				Message: "tanggalBerlaku tidak boleh lebih dari besok",
			})
		}
	}

	// Parse kurs_tengah
	if len(details) > 0 {
		// Return early to avoid noise
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}

	kursTengah, err := decimal.NewFromString(req.KursTengah)
	if err != nil || kursTengah.LessThanOrEqual(decimal.Zero) {
		return nil, newKursErr(CodeKursInvalidRates,
			"kursTengah harus angka positif.",
			domainerrors.Detail{Field: "body.kursTengah", Rule: "positive",
				Message: "kursTengah harus > 0"},
		)
	}

	// Parse kurs_beli / kurs_jual (optional)
	var kursBeli, kursJual *decimal.Decimal
	if req.KursBeli != nil {
		d, err := decimal.NewFromString(*req.KursBeli)
		if err != nil {
			return nil, newKursErr(CodeKursInvalidRates, "kursBeli bukan angka valid.",
				domainerrors.Detail{Field: "body.kursBeli", Rule: "numeric", Message: "kursBeli harus angka valid"})
		}
		kursBeli = &d
	}
	if req.KursJual != nil {
		d, err := decimal.NewFromString(*req.KursJual)
		if err != nil {
			return nil, newKursErr(CodeKursInvalidRates, "kursJual bukan angka valid.",
				domainerrors.Detail{Field: "body.kursJual", Rule: "numeric", Message: "kursJual harus angka valid"})
		}
		kursJual = &d
	}

	if err := validateRates(kursBeli, kursJual, kursTengah); err != nil {
		return nil, err
	}

	// Check mata_uang is APPROVED
	approved, err := s.repo.FindMataUangApproved(ctx, kodeMataUang)
	if err != nil {
		return nil, fmt.Errorf("service validate mata_uang: %w", err)
	}
	if !approved {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Mata uang %s tidak ditemukan atau belum disetujui (APPROVED). "+
				"Pastikan mata uang sudah melalui proses approval sebelum menambahkan kurs.", kodeMataUang),
			domainerrors.Detail{Field: "body.kodeMataUang", Rule: "approved", Message: "Mata uang belum APPROVED"},
		)
	}

	// Find periode_buku
	periodeID, err := s.repo.FindActivePeriode(ctx, tanggalBerlaku)
	if err != nil {
		return nil, fmt.Errorf("service find periode: %w", err)
	}
	if periodeID == uuid.Nil {
		return nil, newKursErr(CodeKursPeriodeNotFound,
			fmt.Sprintf("Tidak ada periode buku aktif untuk tanggal %s. "+
				"Pastikan periode buku sudah dibuat dan aktif.", req.TanggalBerlaku),
			domainerrors.Detail{Field: "body.tanggalBerlaku", Rule: "periode_required",
				Message: "Tidak ada periode buku untuk tanggal ini"},
		)
	}

	return &parsedCreateValues{
		tanggalBerlaku: tanggalBerlaku,
		kursBeli:       kursBeli,
		kursJual:       kursJual,
		kursTengah:     kursTengah,
		periodeID:      periodeID,
	}, nil
}

// parseAndValidateUpdate parses an UpdateRequest.
func (s *Service) parseAndValidateUpdate(req UpdateRequest) (*parsedUpdateValues, error) {
	if req.RowVersion <= 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"rowVersion wajib diisi dan harus positif",
			domainerrors.Detail{Field: "body.rowVersion", Rule: "required", Message: "rowVersion wajib positif"},
		)
	}

	result := &parsedUpdateValues{}

	if req.KursBeli != nil {
		d, err := decimal.NewFromString(*req.KursBeli)
		if err != nil {
			return nil, newKursErr(CodeKursInvalidRates, "kursBeli bukan angka valid.",
				domainerrors.Detail{Field: "body.kursBeli", Rule: "numeric"})
		}
		result.kursBeli = &d
	}
	if req.KursJual != nil {
		d, err := decimal.NewFromString(*req.KursJual)
		if err != nil {
			return nil, newKursErr(CodeKursInvalidRates, "kursJual bukan angka valid.",
				domainerrors.Detail{Field: "body.kursJual", Rule: "numeric"})
		}
		result.kursJual = &d
	}
	if req.KursTengah != nil {
		d, err := decimal.NewFromString(*req.KursTengah)
		if err != nil || d.LessThanOrEqual(decimal.Zero) {
			return nil, newKursErr(CodeKursInvalidRates, "kursTengah harus angka positif.",
				domainerrors.Detail{Field: "body.kursTengah", Rule: "positive"})
		}
		result.kursTengah = &d
	}
	if req.SumberKurs != nil {
		sk := SumberKurs(*req.SumberKurs)
		if !validSumberKurs[sk] {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"sumberKurs tidak valid.",
				domainerrors.Detail{Field: "body.sumberKurs", Rule: "oneof",
					Message: "sumberKurs harus: BI_JISDOR, BI_KURS_TENGAH, INTERNAL, MANUAL"})
		}
		result.sumberKurs = &sk
	}

	return result, nil
}

// validateRates checks beli ≤ tengah ≤ jual when all three are present.
func validateRates(kursBeli, kursJual *decimal.Decimal, kursTengah decimal.Decimal) error {
	if kursBeli == nil && kursJual == nil {
		return nil
	}
	var details []domainerrors.Detail
	if kursBeli != nil && kursBeli.GreaterThan(kursTengah) {
		details = append(details, domainerrors.Detail{
			Field:   "body.kursBeli",
			Rule:    "lte_tengah",
			Message: "kursBeli harus ≤ kursTengah",
		})
	}
	if kursJual != nil && kursJual.LessThan(kursTengah) {
		details = append(details, domainerrors.Detail{
			Field:   "body.kursJual",
			Rule:    "gte_tengah",
			Message: "kursJual harus ≥ kursTengah",
		})
	}
	if len(details) > 0 {
		return newKursErr(CodeKursInvalidRates,
			"Hubungan kurs beli/tengah/jual tidak valid.", details...)
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// buildFxRateIDKode generates the business identifier {KODE}_{YYYYMMDD}.
func buildFxRateIDKode(kode string, tanggal time.Time) string {
	return strings.ToUpper(kode) + "_" + tanggal.Format("20060102")
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

// mapWorkflowState converts workflow engine state string to kurs WorkflowStatus.
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
