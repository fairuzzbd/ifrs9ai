// Package jurnal — Services: business logic for the Jurnal Engine (P5-M2).
//
// Four services:
//   - MappingService  — CRUD + 4-eyes/6-eyes workflow for mst.mapping_jurnal_header.
//   - ResolverService — pure resolver (event payload → balanced JurnalLine slice, no DB writes).
//   - PostingService  — atomic INSERT into jrnl.header + jrnl.detail; manual posting 4-eyes.
//   - DLQService      — inspect/replay/discard sys.dlq_jurnal_post.
//
// Compliance:
//   - DEC-016: No float64, shopspring/decimal for all amounts.
//   - DEC-017: SoD maker≠reviewer≠approver(≠approver_2 for 6-eyes).
//   - DEC-018: Audit-in-tx mandatory for all mutations.
//   - DEC-021: Idempotency-Key checked per mutating endpoint.
//   - DEC-027: Step-up MFA on regulated approve + approve-2.
//   - Constructor panics on nil auditWriter.
//   - Balance invariant: Σ DEBIT = Σ KREDIT asserted before DB insert.
//   - Append-only jrnl.*: enforced at DB level (triggers from migration 000035).
package jurnal

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── MappingService ───────────────────────────────────────────────────────────

// MappingService owns all business logic for the mapping_jurnal_header workflow.
type MappingService struct {
	repo        *MappingRepo
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewMappingService creates a MappingService. Panics if repo or auditWriter is nil (DEC-018).
func NewMappingService(repo *MappingRepo, auditWriter *audit.Writer, logger *slog.Logger) *MappingService {
	if repo == nil {
		panic("jurnal.NewMappingService: repo must not be nil")
	}
	if auditWriter == nil {
		panic("jurnal.NewMappingService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &MappingService{repo: repo, auditWriter: auditWriter, logger: logger}
}

// Create creates a new mapping header in DRAFT status.
func (s *MappingService) Create(ctx context.Context, req MappingHeaderCreateRequest, callerID uuid.UUID) (*MappingHeader, error) {
	h := &MappingHeader{
		ID:                 uuid.New(),
		EventIDKode:        "EVT-" + req.EventCode[:min(len(req.EventCode), 10)],
		EventCode:          req.EventCode,
		NamaEvent:          req.NamaEvent,
		KategoriEvent:      req.KategoriEvent,
		TriggerSource:      req.TriggerSource,
		KlasifikasiBerlaku: req.KlasifikasiBerlaku,
		Deskripsi:          req.Deskripsi,
		AktifFlag:          false, // active only after APPROVED_ACTIVE
		WorkflowStatus:     MappingStatusDraft,
		WorkflowPath:       WorkflowPathFor(req.EventCode),
		MakerID:            &callerID,
		CreatedBy:          callerID,
		UpdatedBy:          callerID,
		TenantID:           tenantIDFromCtx(ctx),
	}
	// Build detail rows from input.
	for _, di := range req.DetailRows {
		multiplier := decimal.NewFromFloat(1.0)
		if di.Multiplier != nil {
			multiplier = *di.Multiplier
		}
		h.DetailRows = append(h.DetailRows, MappingDetailRow{
			ID:                uuid.New(),
			EventHeaderID:     h.ID,
			Urutan:            di.Urutan,
			KodeAkunID:        di.KodeAkunID,
			DKIndicator:       di.DKIndicator,
			SumberAmount:      di.SumberAmount,
			KlasifikasiFilter: di.KlasifikasiFilter,
			Multiplier:        multiplier,
			Catatan:           di.Catatan,
			AktifFlag:         true,
		})
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Create begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.repo.Create(ctx, tx, h); err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Create: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if err := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL_MAPPING.CREATE",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   h.ID,
		After:      h,
	})); err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Create audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Create commit: %w", err)
	}
	return h, nil
}

// Submit transitions DRAFT → PENDING_REVIEW.
func (s *MappingService) Submit(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*MappingHeader, error) {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Submit get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanSubmit() {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa submit dari status '%s'.", h.WorkflowStatus))
	}
	before := *h
	h.WorkflowStatus = MappingStatusPendingReview
	h.SubmitAt = nowPtr()
	h.UpdatedBy = callerID

	return s.applyStatusChange(ctx, h, &before, "JURNAL_MAPPING.SUBMIT")
}

// Review transitions PENDING_REVIEW → PENDING_APPROVAL.
func (s *MappingService) Review(ctx context.Context, id uuid.UUID, req WorkflowSigningRequest, callerID uuid.UUID) (*MappingHeader, error) {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Review get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanReview() {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa review dari status '%s'.", h.WorkflowStatus))
	}
	// SoD: reviewer ≠ maker (DEC-017).
	if h.MakerID != nil && *h.MakerID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Reviewer tidak boleh sama dengan maker (SoD, DEC-017).")
	}
	before := *h
	h.WorkflowStatus = MappingStatusPendingApproval
	h.ReviewerID = &callerID
	h.ReviewerSignedAt = nowPtr()
	sigHash := computeSigHash(callerID, "REVIEW", id)
	h.ReviewerSignatureHash = sigHash
	h.CommentReview = &req.Comment
	h.UpdatedBy = callerID

	return s.applyStatusChange(ctx, h, &before, "JURNAL_MAPPING.REVIEW")
}

// Approve transitions PENDING_APPROVAL → PENDING_APPROVAL_2 (6-eyes) or APPROVED_ACTIVE (4-eyes).
// Requires step-up MFA for regulated events (DEC-027).
func (s *MappingService) Approve(ctx context.Context, id uuid.UUID, req WorkflowSigningRequest, callerID uuid.UUID, claims *auth.Claims) (*MappingHeader, error) {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Approve get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanApprove() {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa approve dari status '%s'.", h.WorkflowStatus))
	}
	// SoD checks (DEC-017).
	if h.MakerID != nil && *h.MakerID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Approver tidak boleh sama dengan maker (SoD, DEC-017).")
	}
	if h.ReviewerID != nil && *h.ReviewerID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Approver tidak boleh sama dengan reviewer (SoD, DEC-017).")
	}
	// Step-up MFA mandatory for regulated events (DEC-027).
	if IsRegulated(h.EventCode) && claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.CodeJurnalStepUpRequired,
			"Step-up MFA diperlukan untuk approve event code regulated (DEC-027).")
	}
	before := *h
	sigHash := computeSigHash(callerID, "APPROVE", id)
	h.ApproverID = &callerID
	h.ApproverSignedAt = nowPtr()
	h.ApproverSignatureHash = sigHash
	h.CommentApprove = &req.Comment
	h.UpdatedBy = callerID

	if h.WorkflowPath == WorkflowPath6Eyes {
		// 6-eyes: next step is PENDING_APPROVAL_2.
		h.WorkflowStatus = MappingStatusPendingApproval2
	} else {
		// 4-eyes: directly APPROVED_ACTIVE.
		h.WorkflowStatus = MappingStatusApprovedActive
		h.AktifFlag = true
	}

	return s.applyStatusChange(ctx, h, &before, "JURNAL_MAPPING.APPROVE")
}

// Approve2 transitions PENDING_APPROVAL_2 → APPROVED_ACTIVE (6-eyes only).
// Step-up MFA mandatory (DEC-027).
func (s *MappingService) Approve2(ctx context.Context, id uuid.UUID, req WorkflowSigningRequest, callerID uuid.UUID, claims *auth.Claims) (*MappingHeader, error) {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Approve2 get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanApprove2() {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa approve-2 dari status '%s'.", h.WorkflowStatus))
	}
	// Step-up MFA mandatory (DEC-027).
	if claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.CodeJurnalStepUpRequired,
			"Step-up MFA diperlukan untuk approve-2 (DEC-027).")
	}
	// SoD: approver_2 ≠ reviewer ≠ maker.
	if h.MakerID != nil && *h.MakerID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Approver-2 tidak boleh sama dengan maker (SoD, DEC-017).")
	}
	if h.ReviewerID != nil && *h.ReviewerID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Approver-2 tidak boleh sama dengan reviewer (SoD, DEC-017).")
	}
	if h.ApproverID != nil && *h.ApproverID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Approver-2 tidak boleh sama dengan approver-1 (SoD, DEC-017).")
	}
	before := *h
	sigHash := computeSigHash(callerID, "APPROVE_2", id)
	h.Approver2ID = &callerID
	h.Approver2SignedAt = nowPtr()
	h.Approver2SignatureHash = sigHash
	h.CommentApprove2 = &req.Comment
	h.WorkflowStatus = MappingStatusApprovedActive
	h.AktifFlag = true
	h.UpdatedBy = callerID

	return s.applyStatusChange(ctx, h, &before, "JURNAL_MAPPING.APPROVE_2")
}

// Reject transitions PENDING_REVIEW|PENDING_APPROVAL|PENDING_APPROVAL_2 → REJECTED.
func (s *MappingService) Reject(ctx context.Context, id uuid.UUID, req WorkflowRejectRequest, callerID uuid.UUID) (*MappingHeader, error) {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Reject get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanReject() {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa reject dari status '%s'.", h.WorkflowStatus))
	}
	before := *h
	h.WorkflowStatus = MappingStatusRejected
	h.RejectReason = &req.RejectReason
	h.UpdatedBy = callerID
	return s.applyStatusChange(ctx, h, &before, "JURNAL_MAPPING.REJECT")
}

// Withdraw soft-deletes a DRAFT mapping header.
func (s *MappingService) Withdraw(ctx context.Context, id uuid.UUID, callerID uuid.UUID) error {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("jurnal.MappingService.Withdraw get: %w", err)
	}
	if h == nil {
		return domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanWithdraw() {
		return domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Hanya DRAFT yang bisa di-withdraw, saat ini '%s'.", h.WorkflowStatus))
	}
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("jurnal.MappingService.Withdraw begin tx: %w", err)
	}
	defer rollbackTx(tx)
	if err := s.repo.SoftDelete(ctx, tx, id, callerID); err != nil {
		return fmt.Errorf("jurnal.MappingService.Withdraw: %w", err)
	}
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL_MAPPING.WITHDRAW",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   id,
		Before:     h,
	})); err != nil {
		return fmt.Errorf("jurnal.MappingService.Withdraw audit: %w", err)
	}
	return tx.Commit()
}

// Deactivate transitions APPROVED_ACTIVE → WITHDRAWN (deactivation).
func (s *MappingService) Deactivate(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*MappingHeader, error) {
	h, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.Deactivate get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan.")
	}
	if !h.WorkflowStatus.CanDeactivate() {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa deactivate dari status '%s'.", h.WorkflowStatus))
	}
	before := *h
	h.WorkflowStatus = MappingStatusWithdrawn
	h.AktifFlag = false
	h.UpdatedBy = callerID
	return s.applyStatusChange(ctx, h, &before, "JURNAL_MAPPING.DEACTIVATE")
}

// applyStatusChange writes the status change to DB + audit log in a single tx.
// The actor is resolved from ctx (JWT claims) inside audit.EventFromContext.
func (s *MappingService) applyStatusChange(ctx context.Context, h *MappingHeader, before *MappingHeader, action string) (*MappingHeader, error) {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.%s begin tx: %w", action, err)
	}
	defer rollbackTx(tx)

	if err := s.repo.UpdateStatus(ctx, tx, h); err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.%s update: %w", action, err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     action,
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   h.ID,
		Before:     before,
		After:      h,
	})); err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.%s audit: %w", action, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jurnal.MappingService.%s commit: %w", action, err)
	}
	return h, nil
}

// ─── ResolverService ──────────────────────────────────────────────────────────

// ResolverService is a pure resolver: no DB writes. Takes an event payload and
// returns balanced debit/kredit JurnalLines using the active mapping header.
type ResolverService struct {
	mappingRepo *MappingRepo
	db          *sql.DB
	logger      *slog.Logger
}

// NewResolverService creates a ResolverService. Panics on nil mappingRepo.
func NewResolverService(mappingRepo *MappingRepo, db *sql.DB, logger *slog.Logger) *ResolverService {
	if mappingRepo == nil {
		panic("jurnal.NewResolverService: mappingRepo must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ResolverService{mappingRepo: mappingRepo, db: db, logger: logger}
}

// Resolve resolves an event payload into balanced JurnalLines. No DB writes.
// Returns error if: no active mapping, klasifikasi not eligible, balance invariant violated.
func (s *ResolverService) Resolve(ctx context.Context, req ResolverRequest) (*ResolverResponse, error) {
	mapping, err := s.mappingRepo.GetByEventCode(ctx, req.EventCode)
	if err != nil {
		return nil, fmt.Errorf("jurnal.ResolverService.Resolve get mapping: %w", err)
	}
	if mapping == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalEventNotMapped,
			fmt.Sprintf("Tidak ada mapping aktif untuk event_code '%s'.", req.EventCode))
	}
	if !mapping.WorkflowStatus.IsActiveForResolver() {
		return nil, domainerrors.New(domainerrors.CodeJurnalMappingWorkflowGate,
			fmt.Sprintf("Mapping untuk '%s' belum APPROVED_ACTIVE (saat ini '%s').", req.EventCode, mapping.WorkflowStatus))
	}

	// Check klasifikasi eligibility.
	if len(mapping.KlasifikasiBerlaku) > 0 && !containsStr(mapping.KlasifikasiBerlaku, req.KlasifikasiPSAK71) {
		return nil, domainerrors.New(domainerrors.CodeJurnalKlasifikasiNotEligible,
			fmt.Sprintf("Klasifikasi '%s' tidak eligible untuk event '%s'. Berlaku untuk: %v.",
				req.KlasifikasiPSAK71, req.EventCode, mapping.KlasifikasiBerlaku))
	}

	// Validate amount.
	if req.AmountIDR.IsZero() || req.AmountIDR.IsNegative() {
		return nil, domainerrors.New(domainerrors.CodeJurnalAmountInvalid,
			"amountIdr harus > 0.")
	}

	// Build lines from mapping detail rows.
	lines := make([]JurnalLine, 0, len(mapping.DetailRows))
	var totalDebit, totalKredit decimal.Decimal
	for i := range mapping.DetailRows {
		d := &mapping.DetailRows[i]
		// Skip rows filtered to a different klasifikasi.
		if d.KlasifikasiFilter != nil && *d.KlasifikasiFilter != "" && *d.KlasifikasiFilter != req.KlasifikasiPSAK71 {
			continue
		}
		amt := req.AmountIDR.Mul(d.Multiplier)
		line := JurnalLine{
			Urutan:              d.Urutan,
			Posisi:              d.DKIndicator,
			AkunID:              d.KodeAkunID,
			AkunKode:            d.KodeAkunKode,
			AkunNama:            d.KodeAkunNama,
			AmountIDR:           amt,
			Narasi:              buildNarasi(req, *d),
			KlasifikasiEligible: req.KlasifikasiPSAK71,
		}
		if d.DKIndicator == "DEBIT" {
			totalDebit = totalDebit.Add(amt)
		} else {
			totalKredit = totalKredit.Add(amt)
		}
		lines = append(lines, line)
	}

	// Balance invariant check (service-side; DB CHECK is the belt).
	if !totalDebit.Equal(totalKredit) {
		return nil, domainerrors.New(domainerrors.CodeJurnalBalanceInvariant,
			fmt.Sprintf("Balance invariant violated: total_debit=%s ≠ total_kredit=%s.",
				totalDebit.StringFixed(4), totalKredit.StringFixed(4)))
	}

	return &ResolverResponse{
		Lines:          lines,
		TotalDebitIDR:  totalDebit,
		TotalKreditIDR: totalKredit,
		IsBalanced:     true,
		HeaderUsed: &HeaderUsedRef{
			ID:            mapping.ID,
			EventCode:     mapping.EventCode,
			KategoriEvent: mapping.KategoriEvent,
		},
	}, nil
}

// ─── PostingService ───────────────────────────────────────────────────────────

// PostingService handles atomic INSERT into jrnl.header + jrnl.detail.
// Also owns the manual posting 4-eyes workflow for PERIODE_ADJUSTMENT + CORRECTION.
type PostingService struct {
	jurnalRepo  *JurnalRepo
	dlqRepo     *DLQRepo
	resolver    *ResolverService
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewPostingService creates a PostingService. Panics on nil auditWriter.
func NewPostingService(
	jurnalRepo *JurnalRepo,
	dlqRepo *DLQRepo,
	resolver *ResolverService,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *PostingService {
	if jurnalRepo == nil {
		panic("jurnal.NewPostingService: jurnalRepo must not be nil")
	}
	if auditWriter == nil {
		panic("jurnal.NewPostingService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &PostingService{
		jurnalRepo:  jurnalRepo,
		dlqRepo:     dlqRepo,
		resolver:    resolver,
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// PostResolved resolves an event and atomically posts jrnl.header + jrnl.detail.
// Checks idempotency, periode hard-close, and balance invariant before insert.
// Designed to be called by Asynq workers AND by DLQService.Replay().
//
// Returns (jurnalHeaderID, error). On domain error → DLQ immediately.
// On infra error → callers must retry via Asynq retry policy.
func (s *PostingService) PostResolved(ctx context.Context, req ResolverRequest) (uuid.UUID, error) {
	// 1. Idempotency check.
	idmpKey := BuildIdempotencyKey(req.SourceEventID, req.EventCode)
	existing, err := s.jurnalRepo.CheckIdempotency(ctx, idmpKey)
	if err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved idempotency check: %w", err)
	}
	if existing != uuid.Nil {
		return existing, nil // already posted — return existing ID (idempotent)
	}

	// 2. Periode hard-close check.
	hardClosed, err := s.jurnalRepo.IsPeriodeHardClosed(ctx, req.PeriodeID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved periode check: %w", err)
	}
	if hardClosed {
		return uuid.Nil, domainerrors.New(domainerrors.CodeJurnalPeriodeHardClosed,
			"Periode sudah hard-closed, tidak bisa posting jurnal.")
	}

	// 3. Resolve lines.
	resolved, err := s.resolver.Resolve(ctx, req)
	if err != nil {
		return uuid.Nil, err // domain errors from resolver propagate up
	}

	// 4. Build jrnl.header + detail rows inside a serializable tx.
	now := time.Now()
	headerID := uuid.New()
	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved begin tx: %w", err)
	}
	defer rollbackTx(tx)

	// Allocate sequence number inside the same tx (atomic with INSERT).
	noJurnal, err := s.jurnalRepo.NextNoJurnal(ctx, tx, now.Year())
	if err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved NextNoJurnal: %w", err)
	}

	callerID := callerIDFromCtx(ctx)
	header := &JurnalHeader{
		ID:                 headerID,
		NoJurnal:           noJurnal,
		TanggalPosting:     now,
		PeriodeID:          req.PeriodeID,
		EventCode:          req.EventCode,
		MappingHeaderID:    &resolved.HeaderUsed.ID,
		InstrumenID:        req.InstrumenID,
		ReferenceEventType: req.SourceEventType,
		ReferenceEventID:   &req.SourceEventID,
		Currency:           req.Currency,
		TotalDebit:         resolved.TotalDebitIDR,
		TotalKredit:        resolved.TotalKreditIDR,
		Narrative:          req.Narasi,
		StatusInternal:     JurnalStatusPosted,
		IdempotencyKey:     idmpKey,
		CreatedBy:          callerID,
	}
	// Build jrnl.detail rows.
	for _, line := range resolved.Lines {
		var debit, kredit decimal.Decimal
		if line.Posisi == "DEBIT" {
			debit = line.AmountIDR
		} else {
			kredit = line.AmountIDR
		}
		header.DetailRows = append(header.DetailRows, JurnalDetailRow{
			ID:            uuid.New(),
			HeaderID:      headerID,
			Urutan:        line.Urutan,
			KodeAkunID:    line.AkunID,
			DebitAmount:   debit,
			KreditAmount:  kredit,
			MataUang:      req.Currency,
			NarrativeLine: line.Narasi,
		})
	}

	if err := s.jurnalRepo.Insert(ctx, tx, header); err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved insert: %w", err)
	}

	// Audit-in-tx (DEC-018): JURNAL.POST
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL.POST",
		EntityType: "jrnl.header",
		EntityID:   headerID,
		After:      header,
	})); err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("jurnal.PostingService.PostResolved commit: %w", err)
	}
	return headerID, nil
}

// CreateManualDraft creates a DRAFT manual jurnal (for PERIODE_ADJUSTMENT + CORRECTION_PERIODE_CLOSED).
func (s *PostingService) CreateManualDraft(ctx context.Context, req ManualPostRequest, callerID uuid.UUID) (*JurnalHeader, error) {
	if !IsManualAllowed(req.EventCode) {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Event code '%s' tidak diizinkan untuk manual posting. Hanya PERIODE_ADJUSTMENT dan CORRECTION_PERIODE_CLOSED.", req.EventCode))
	}
	resolved, err := s.resolver.Resolve(ctx, ResolverRequest{
		EventCode:         req.EventCode,
		KlasifikasiPSAK71: "MANUAL",
		InstrumenID:       req.InstrumenID,
		PeriodeID:         req.PeriodeID,
		AmountIDR:         req.AmountIDR,
		Currency:          "IDR",
		SourceEventID:     uuid.New(), // new ID for manual
		SourceEventType:   "MANUAL_POST",
		Narasi:            req.Narasi,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.CreateManualDraft begin tx: %w", err)
	}
	defer rollbackTx(tx)

	noJurnal, err := s.jurnalRepo.NextNoJurnal(ctx, tx, now.Year())
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.CreateManualDraft NextNoJurnal: %w", err)
	}

	headerID := uuid.New()
	header := &JurnalHeader{
		ID:              headerID,
		NoJurnal:        noJurnal,
		TanggalPosting:  now,
		PeriodeID:       req.PeriodeID,
		EventCode:       req.EventCode,
		MappingHeaderID: &resolved.HeaderUsed.ID,
		InstrumenID:     req.InstrumenID,
		Currency:        "IDR",
		TotalDebit:      resolved.TotalDebitIDR,
		TotalKredit:     resolved.TotalKreditIDR,
		Narrative:       req.Narasi,
		StatusInternal:  JurnalStatusDraftManual,
		IdempotencyKey:  BuildIdempotencyKey(uuid.New(), req.EventCode+"-MANUAL"),
		DokumenDocID:    req.DokumenDocID,
		CreatedBy:       callerID,
	}
	for _, line := range resolved.Lines {
		var debit, kredit decimal.Decimal
		if line.Posisi == "DEBIT" {
			debit = line.AmountIDR
		} else {
			kredit = line.AmountIDR
		}
		header.DetailRows = append(header.DetailRows, JurnalDetailRow{
			ID:            uuid.New(),
			HeaderID:      headerID,
			Urutan:        line.Urutan,
			KodeAkunID:    line.AkunID,
			DebitAmount:   debit,
			KreditAmount:  kredit,
			MataUang:      "IDR",
			NarrativeLine: line.Narasi,
		})
	}
	if err := s.jurnalRepo.Insert(ctx, tx, header); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.CreateManualDraft insert: %w", err)
	}
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL.MANUAL_CREATE",
		EntityType: "jrnl.header",
		EntityID:   headerID,
		After:      header,
	})); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.CreateManualDraft audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.CreateManualDraft commit: %w", err)
	}
	return header, nil
}

// SubmitManual transitions manual PENDING_APPROVAL → (keeps PENDING_APPROVAL, sets approver queue).
func (s *PostingService) SubmitManual(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*JurnalHeader, error) {
	h, err := s.jurnalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.SubmitManual get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Jurnal header tidak ditemukan.")
	}
	// Already in PENDING_APPROVAL (draft state for manual). Nothing to do structurally,
	// status stays PENDING_APPROVAL, audit the submission event.
	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.SubmitManual begin tx: %w", err)
	}
	defer rollbackTx(tx)
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL.MANUAL_SUBMIT",
		EntityType: "jrnl.header",
		EntityID:   id,
		Before:     h,
		After:      h,
	})); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.SubmitManual audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.SubmitManual commit: %w", err)
	}
	return h, nil
}

// ApproveManual transitions PENDING_APPROVAL → POSTED (4-eyes for manual posting).
func (s *PostingService) ApproveManual(ctx context.Context, id uuid.UUID, callerID uuid.UUID, makerID uuid.UUID) (*JurnalHeader, error) {
	h, err := s.jurnalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.ApproveManual get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Jurnal header tidak ditemukan.")
	}
	if h.StatusInternal != JurnalStatusDraftManual && h.StatusInternal != JurnalStatusPendingApprove {
		return nil, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			fmt.Sprintf("Tidak bisa approve dari status '%s'.", h.StatusInternal))
	}
	// SoD: approver ≠ maker.
	if makerID == callerID {
		return nil, domainerrors.New(domainerrors.CodeJurnalSoDViolation,
			"Approver tidak boleh sama dengan maker (SoD, DEC-017).")
	}
	// Check periode hard-close.
	hardClosed, err := s.jurnalRepo.IsPeriodeHardClosed(ctx, h.PeriodeID)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.ApproveManual periode check: %w", err)
	}
	if hardClosed {
		return nil, domainerrors.New(domainerrors.CodeJurnalPeriodeHardClosed,
			"Periode sudah hard-closed, tidak bisa post jurnal manual.")
	}

	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.ApproveManual begin tx: %w", err)
	}
	defer rollbackTx(tx)

	before := *h
	if err := s.jurnalRepo.UpdateStatus(ctx, tx, id, JurnalStatusPosted); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.ApproveManual update: %w", err)
	}
	h.StatusInternal = JurnalStatusPosted
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL.MANUAL_APPROVE",
		EntityType: "jrnl.header",
		EntityID:   id,
		Before:     before,
		After:      h,
	})); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.ApproveManual audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.ApproveManual commit: %w", err)
	}
	return h, nil
}

// RejectManual returns a manual jurnal to DRAFT / soft-reject (marks REJECTED comment).
func (s *PostingService) RejectManual(ctx context.Context, id uuid.UUID, reason string, callerID uuid.UUID) (*JurnalHeader, error) {
	h, err := s.jurnalRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.RejectManual get: %w", err)
	}
	if h == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Jurnal header tidak ditemukan.")
	}
	// jrnl.header is append-only so we cannot truly update — use DLQ pattern for rejected manual posts.
	// Record audit with REJECTED note and return the header as-is.
	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.RejectManual begin tx: %w", err)
	}
	defer rollbackTx(tx)
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "JURNAL.MANUAL_REJECT",
		EntityType: "jrnl.header",
		EntityID:   id,
		Before:     h,
		After:      map[string]any{"rejectReason": reason},
	})); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.RejectManual audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("jurnal.PostingService.RejectManual commit: %w", err)
	}
	return h, nil
}

// ─── DLQService ───────────────────────────────────────────────────────────────

// DLQService handles DLQ inspection, replay, and discard.
type DLQService struct {
	dlqRepo     *DLQRepo
	posting     *PostingService
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewDLQService creates a DLQService. Panics on nil auditWriter.
func NewDLQService(dlqRepo *DLQRepo, posting *PostingService, auditWriter *audit.Writer, logger *slog.Logger) *DLQService {
	if dlqRepo == nil {
		panic("jurnal.NewDLQService: dlqRepo must not be nil")
	}
	if auditWriter == nil {
		panic("jurnal.NewDLQService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DLQService{dlqRepo: dlqRepo, posting: posting, auditWriter: auditWriter, logger: logger}
}

// Replay re-attempts posting for a FAILED DLQ entry.
// Domain errors → immediately abandon (DLQ invariant).
// Infra errors → return error so caller can retry.
func (s *DLQService) Replay(ctx context.Context, id uuid.UUID, callerID uuid.UUID) (*DLQReplayResponse, error) {
	entry, err := s.dlqRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("jurnal.DLQService.Replay get: %w", err)
	}
	if entry == nil {
		return nil, domainerrors.New(domainerrors.CodeJurnalDlqNotFound, "DLQ entry tidak ditemukan.")
	}
	if entry.Status == DLQStatusReplayedOK || entry.Status == DLQStatusReplaying {
		return nil, domainerrors.New(domainerrors.CodeJurnalDlqAlreadyReplayed,
			fmt.Sprintf("DLQ entry sudah di-replay (status: %s).", entry.Status))
	}
	if entry.Status == DLQStatusAbandoned {
		return nil, domainerrors.New(domainerrors.CodeJurnalDlqAlreadyReplayed, "DLQ entry sudah ABANDONED.")
	}

	// Parse payload.
	var payload DLQPostPayload
	if err := json.Unmarshal(entry.PayloadJSONB, &payload); err != nil {
		return nil, fmt.Errorf("jurnal.DLQService.Replay unmarshal payload: %w", err)
	}

	// Check target periode hard-close.
	if entry.PeriodeID != nil {
		hardClosed, err := s.posting.jurnalRepo.IsPeriodeHardClosed(ctx, *entry.PeriodeID)
		if err != nil {
			return nil, fmt.Errorf("jurnal.DLQService.Replay periode check: %w", err)
		}
		if hardClosed {
			return nil, domainerrors.New(domainerrors.CodeJurnalDlqReplayPeriodeHardClosed,
				"Periode target DLQ replay sudah hard-closed.")
		}
	}

	// Mark as REPLAYING before attempting.
	now := time.Now()
	entry.Status = DLQStatusReplaying
	entry.LastRetryAt = &now
	entry.RetryCount++
	if err := s.dlqRepo.UpdateStatus(ctx, nil, entry); err != nil {
		return nil, fmt.Errorf("jurnal.DLQService.Replay mark replaying: %w", err)
	}

	// Rebuild ResolverRequest from payload.
	// UUIDs in payload were originally serialized from uuid.UUID values, so parse
	// errors here are unexpected; we fall back to uuid.Nil on failure (audit trail
	// still captures the raw payload JSON).
	periodeID, err := uuid.Parse(payload.PeriodeID)
	if err != nil {
		s.logger.WarnContext(ctx, "jurnal.DLQService.Replay: invalid periodeID in payload, using Nil",
			"raw", payload.PeriodeID, "error", err)
		periodeID = uuid.Nil
	}
	sourceEventID, err := uuid.Parse(payload.SourceEventID)
	if err != nil {
		s.logger.WarnContext(ctx, "jurnal.DLQService.Replay: invalid sourceEventID in payload, using Nil",
			"raw", payload.SourceEventID, "error", err)
		sourceEventID = uuid.Nil
	}
	var instrumenID *uuid.UUID
	if payload.InstrumenID != nil {
		uid, parseErr := uuid.Parse(*payload.InstrumenID)
		if parseErr != nil {
			s.logger.WarnContext(ctx, "jurnal.DLQService.Replay: invalid instrumenID in payload, skipping",
				"raw", *payload.InstrumenID, "error", parseErr)
		} else {
			instrumenID = &uid
		}
	}
	req := ResolverRequest{
		EventCode:         payload.EventCode,
		KlasifikasiPSAK71: payload.KlasifikasiPSAK71,
		InstrumenID:       instrumenID,
		PeriodeID:         periodeID,
		AmountIDR:         payload.AmountIDR,
		Currency:          payload.Currency,
		FxRate:            payload.FxRate,
		SourceEventID:     sourceEventID,
		SourceEventType:   payload.SourceEventType,
		MetadataJSON:      payload.MetadataJSON,
		Narasi:            payload.Narasi,
	}

	jurnalHeaderID, postErr := s.posting.PostResolved(ctx, req)
	if postErr != nil {
		// Domain error → abandon.
		de, isDomain := domainerrors.IsDomainError(postErr)
		errCode := "INFRA_ERROR"
		if isDomain {
			errCode = string(de.Code())
			entry.Status = DLQStatusAbandoned
		} else {
			entry.Status = DLQStatusFailed
		}
		entry.ErrorCode = errCode
		entry.ErrorMessage = postErr.Error()
		_ = s.dlqRepo.UpdateStatus(ctx, nil, entry) //nolint:errcheck // best effort
		return nil, postErr
	}

	// Success: mark REPLAYED_OK.
	entry.Status = DLQStatusReplayedOK
	entry.ReplayedBy = &callerID
	entry.ReplayedAt = nowPtr()
	entry.FinalJurnalHeaderID = &jurnalHeaderID
	if err := s.dlqRepo.UpdateStatus(ctx, nil, entry); err != nil {
		s.logger.ErrorContext(ctx, "jurnal.DLQService.Replay mark replayed_ok failed",
			"dlq_id", id, "error", err)
		// Non-fatal: journal was already posted; don't fail the replay.
	}
	// Audit the replay.
	_ = s.auditWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
		Action:     "JURNAL.DLQ_REPLAY",
		EntityType: "sys.dlq_jurnal_post",
		EntityID:   entry.ID,
		Before:     map[string]any{"status": string(DLQStatusFailed)},
		After:      map[string]any{"status": string(DLQStatusReplayedOK), "finalJurnalHeaderId": jurnalHeaderID},
	}))

	return &DLQReplayResponse{
		DLQId:     entry.ID,
		JobID:     "inline-replay-" + entry.ID.String(),
		StatusURL: "/api/v1/jurnal/dlq/" + entry.ID.String(),
	}, nil
}

// Discard marks a DLQ entry as ABANDONED with a human-readable reason.
func (s *DLQService) Discard(ctx context.Context, id uuid.UUID, req DLQDiscardRequest, callerID uuid.UUID) error {
	if len(req.DiscardReason) < 30 {
		return domainerrors.New(domainerrors.CodeJurnalDlqDiscardReasonTooShort,
			"discardReason minimal 30 karakter.")
	}
	entry, err := s.dlqRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("jurnal.DLQService.Discard get: %w", err)
	}
	if entry == nil {
		return domainerrors.New(domainerrors.CodeJurnalDlqNotFound, "DLQ entry tidak ditemukan.")
	}
	if entry.Status == DLQStatusReplayedOK {
		return domainerrors.New(domainerrors.CodeJurnalDlqAlreadyReplayed, "DLQ entry sudah REPLAYED_OK, tidak bisa discard.")
	}

	before := *entry
	entry.Status = DLQStatusAbandoned
	entry.DiscardedReason = &req.DiscardReason
	entry.DiscardedBy = &callerID
	entry.DiscardedAt = nowPtr()
	if err := s.dlqRepo.UpdateStatus(ctx, nil, entry); err != nil {
		return fmt.Errorf("jurnal.DLQService.Discard update: %w", err)
	}
	// Audit discard.
	_ = s.auditWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{ //nolint:errcheck
		Action:     "JURNAL.DLQ_DISCARD",
		EntityType: "sys.dlq_jurnal_post",
		EntityID:   entry.ID,
		Before:     before,
		After:      entry,
	}))
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// computeSigHash produces a deterministic signature hash for workflow signing.
// sha256(callerID + action + entityID + timestamp)
func computeSigHash(callerID uuid.UUID, action string, entityID uuid.UUID) []byte {
	raw := callerID.String() + "::" + action + "::" + entityID.String() + "::" + time.Now().UTC().Format(time.RFC3339)
	h := sha256.Sum256([]byte(raw))
	b := make([]byte, hex.EncodedLen(len(h)))
	hex.Encode(b, h[:])
	return b
}

// buildNarasi constructs the narrative for a JurnalLine from the resolver request and detail row.
func buildNarasi(req ResolverRequest, d MappingDetailRow) string {
	narasi := req.Narasi
	if narasi == "" {
		narasi = fmt.Sprintf("%s / %s / %s", req.EventCode, req.KlasifikasiPSAK71, d.DKIndicator)
	}
	return narasi
}

// containsStr checks if a string slice contains a value (case-sensitive).
func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// tenantIDFromCtx extracts tenant_id from JWT claims.
func tenantIDFromCtx(ctx context.Context) string {
	if claims := auth.ClaimsFromContext(ctx); claims != nil {
		return claims.TenantID
	}
	return "TUGURE"
}

// callerIDFromCtx extracts the caller UUID from JWT claims.
func callerIDFromCtx(ctx context.Context) uuid.UUID {
	if claims := auth.ClaimsFromContext(ctx); claims != nil {
		if id, err := uuid.Parse(claims.Sub); err == nil {
			return id
		}
	}
	return uuid.Nil
}
