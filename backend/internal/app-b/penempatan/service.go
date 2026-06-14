// Package penempatan — Service: business logic for Penempatan Deposito lifecycle (P5-M1).
//
// All state transitions write to aud.audit_log IN THE SAME TRANSACTION (DEC-018).
// No float64 for money/rates — shopspring/decimal throughout (DEC-016).
// SoD enforced server-side: maker≠reviewer≠approver + terminate 4-eyes (DEC-017).
// Step-up MFA checked via claims.NeedsStepUp() (DEC-027).
// Constructor panics on nil auditWriter (DEC-018 compliance).
// FVTPL guard applied in-tx at Approve (DEC-P5-M1-001).
// Settlement balance hint populated at Create/Get (DEC-P5-M1-004, informational only).
package penempatan

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
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── AsynqEnqueuer ─────────────────────────────────────────────────────────────

// AsynqEnqueuer is the minimal interface for dispatching Asynq tasks.
type AsynqEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// ─── Service ──────────────────────────────────────────────────────────────────

// Service owns all business logic for the penempatan deposito lifecycle.
type Service struct {
	repo        *Repo
	auditWriter *audit.Writer
	asynqClient AsynqEnqueuer
	logger      *slog.Logger
}

// NewService creates a Service. Panics if repo or auditWriter is nil (DEC-018).
func NewService(
	repo *Repo,
	auditWriter *audit.Writer,
	asynqClient AsynqEnqueuer,
	logger *slog.Logger,
) *Service {
	if repo == nil {
		panic("penempatan.NewService: repo must not be nil")
	}
	if auditWriter == nil {
		panic("penempatan.NewService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		repo:        repo,
		auditWriter: auditWriter,
		asynqClient: asynqClient,
		logger:      logger,
	}
}

// ─── Create ──────────────────────────────────────────────────────────────────

// Create inserts a new DRAFT penempatan deposito.
func (s *Service) Create(ctx context.Context, req CreateRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	instr, err := s.repo.GetInstrumenInfo(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: get instrumen: %w", err)
	}
	if instr == nil {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeInstrumenNotFound),
			fmt.Sprintf("Instrumen %s tidak ditemukan atau sudah dihapus.", req.InstrumenID))
	}
	if instr.WorkflowStatus != "APPROVED" {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeInstrumenInvalidKlasifikasi),
			fmt.Sprintf("Instrumen %s belum memiliki klasifikasi PSAK 71 yang di-approve.", instr.Nama))
	}

	tanggalPenempatan, err := time.Parse("2006-01-02", req.TanggalPenempatan)
	if err != nil {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeTanggalPenempatanInvalid),
			"Format tanggal penempatan tidak valid. Gunakan YYYY-MM-DD.")
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if tanggalPenempatan.UTC().Truncate(24 * time.Hour).After(today) {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeTanggalPenempatanInvalid),
			"Tanggal penempatan tidak boleh lebih dari hari ini.")
	}
	if req.TenorBulan <= 0 {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeTenorInvalid), "Tenor harus lebih dari 0 bulan.")
	}
	if req.KuponPersen.IsNegative() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeKuponInvalid), "Kupon tidak boleh negatif.")
	}

	tanggalJatuhTempo := tanggalPenempatan.AddDate(0, int(req.TenorBulan), 0)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	open, err := s.repo.IsPeriodeOpen(ctx, tx, req.PeriodeID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: check periode: %w", err)
	}
	if !open {
		return nil, domainerrors.New(domainerrors.Code(ErrCodePeriodeHardClosed),
			"Periode buku sudah di-close. Penempatan tidak dapat dibuat.")
	}

	kode, err := s.repo.NextKodeSeq(ctx, tx, tanggalPenempatan)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: next kode seq: %w", err)
	}

	var nominalIDR decimal.Decimal
	var kursPenempatan *decimal.Decimal
	if req.NominalIDR != nil {
		nominalIDR = *req.NominalIDR
	} else if req.NominalFCY != nil {
		kurs, kursErr := s.repo.GetKursJISDOR(ctx, req.MataUangID, tanggalPenempatan)
		if kursErr != nil {
			return nil, fmt.Errorf("penempatan.Service.Create: get kurs: %w", kursErr)
		}
		if kurs == nil {
			return nil, domainerrors.New(domainerrors.CodeEADFXRateMissing,
				fmt.Sprintf("Kurs BI JISDOR untuk tanggal %s belum tersedia.", req.TanggalPenempatan))
		}
		nominalIDR = req.NominalFCY.Mul(*kurs)
		kursPenempatan = kurs
	}

	now := time.Now()
	p := &Penempatan{
		ID:                 uuid.New(),
		KodeTransaksi:      kode,
		InstrumenID:        req.InstrumenID,
		CounterpartyBankID: req.CounterpartyBankID,
		PeriodeID:          req.PeriodeID,
		MataUangID:         req.MataUangID,
		TanggalPenempatan:  tanggalPenempatan,
		TanggalJatuhTempo:  tanggalJatuhTempo,
		NominalIDR:         nominalIDR,
		NominalFCY:         req.NominalFCY,
		KursPenempatan:     kursPenempatan,
		TenorBulan:         req.TenorBulan,
		KuponPersen:        req.KuponPersen,
		BiayaTransaksiIDR:  req.BiayaTransaksiIDR,
		NomorReferensiBank: req.NomorReferensiBank,
		SettlementAccount:  req.SettlementAccount,
		Catatan:            req.Catatan,
		KontrakDocID:       req.KontrakDocID,
		WorkflowStatus:     StatusDraft,
		MakerID:            actorID,
		CreatedAt:          now,
		CreatedBy:          actorID,
		UpdatedAt:          now,
		UpdatedBy:          actorID,
		TenantID:           claims.TenantID,
	}

	if err = s.repo.Create(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: insert: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.CREATED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   p.ID,
		After:      p,
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Create: commit: %w", err)
	}

	// Settlement balance hint — informational, non-blocking (DEC-P5-M1-004).
	if p.SettlementAccount != nil && *p.SettlementAccount != "" {
		hint, hintErr := s.repo.GetSettlementBalanceHint(ctx, *p.SettlementAccount, claims.TenantID)
		if hintErr != nil {
			s.logger.WarnContext(ctx, "settlement balance hint lookup failed (non-blocking)",
				"error", hintErr, "settlement_account", *p.SettlementAccount)
		} else {
			p.SettlementBalanceHint = hint
		}
	}

	p.NamaInstrumen = instr.Nama
	p.KlasifikasiPSAK71 = instr.KlasifikasiPSAK71
	p.TipeInstrumen = instr.TipeInstrumen

	return p, nil
}

// ─── Update (DRAFT) ──────────────────────────────────────────────────────────

// Update applies PATCH fields to a DRAFT penempatan (optimistic lock via row_version).
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Update: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Update: get for update: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if p.MakerID != actorID {
		return nil, domainerrors.ErrForbidden(PermTransaksiUpdate)
	}
	if !p.WorkflowStatus.CanEdit() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeEditLocked),
			fmt.Sprintf("Penempatan %s tidak dapat diedit dari status %s.", p.KodeTransaksi, p.WorkflowStatus))
	}

	var newTanggal time.Time
	if req.TanggalPenempatan != nil {
		newTanggal, err = time.Parse("2006-01-02", *req.TanggalPenempatan)
		if err != nil {
			return nil, domainerrors.New(domainerrors.Code(ErrCodeTanggalPenempatanInvalid), "Format tanggal penempatan tidak valid.")
		}
		today := time.Now().UTC().Truncate(24 * time.Hour)
		if newTanggal.UTC().Truncate(24 * time.Hour).After(today) {
			return nil, domainerrors.New(domainerrors.Code(ErrCodeTanggalPenempatanInvalid), "Tanggal penempatan tidak boleh lebih dari hari ini.")
		}
	} else {
		newTanggal = p.TanggalPenempatan
	}
	if req.TenorBulan != nil && *req.TenorBulan <= 0 {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeTenorInvalid), "Tenor harus lebih dari 0 bulan.")
	}
	if req.KuponPersen != nil && req.KuponPersen.IsNegative() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeKuponInvalid), "Kupon tidak boleh negatif.")
	}

	tenorBulan := p.TenorBulan
	if req.TenorBulan != nil {
		tenorBulan = *req.TenorBulan
	}
	tanggalJatuhTempo := newTanggal.AddDate(0, int(tenorBulan), 0)

	before := *p

	rows, err := s.repo.UpdateDraft(ctx, tx, id, req, tanggalJatuhTempo, actorID, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Update: update draft: %w", err)
	}
	if rows == 0 {
		return nil, domainerrors.ErrConflict()
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.UPDATED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		Before:     before,
		After:      req,
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.Update: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Update: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── Withdraw ────────────────────────────────────────────────────────────────

// Withdraw soft-deletes a DRAFT penempatan (sets workflow_status = CANCELLED).
func (s *Service) Withdraw(ctx context.Context, id uuid.UUID, claims *auth.Claims) error {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("penempatan.Service.Withdraw: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return fmt.Errorf("penempatan.Service.Withdraw: get: %w", err)
	}
	if p == nil {
		return domainerrors.ErrNotFound("Penempatan")
	}
	if p.MakerID != actorID {
		return domainerrors.ErrForbidden(PermTransaksiDelete)
	}
	if !p.WorkflowStatus.CanWithdraw() {
		return domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Penempatan tidak bisa dibatalkan dari status %s.", p.WorkflowStatus))
	}

	now := time.Now()
	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus: StatusCancelled,
		DeletedAt: &now,
		DeletedBy: &actorID,
		UpdatedBy: actorID,
	}, claims.TenantID); err != nil {
		return fmt.Errorf("penempatan.Service.Withdraw: update status: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.WITHDRAWN",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		Before:     p,
	})); auditErr != nil {
		return fmt.Errorf("penempatan.Service.Withdraw: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("penempatan.Service.Withdraw: commit: %w", err)
	}
	return nil
}

// ─── Submit ──────────────────────────────────────────────────────────────────

// Submit transitions a DRAFT penempatan to PENDING_REVIEW (maker action).
func (s *Service) Submit(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Submit: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Submit: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if p.MakerID != actorID {
		return nil, domainerrors.ErrForbidden(PermTransaksiSubmit)
	}
	if !p.WorkflowStatus.CanSubmit() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa submit dari status %s.", p.WorkflowStatus))
	}

	open, err := s.repo.IsPeriodeOpen(ctx, tx, p.PeriodeID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Submit: check periode: %w", err)
	}
	if !open {
		return nil, domainerrors.New(domainerrors.Code(ErrCodePeriodeHardClosed),
			"Periode buku sudah di-close. Penempatan tidak dapat di-submit.")
	}

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus: StatusPendingReview,
		UpdatedBy: actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Submit: update status: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.SUBMITTED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"comment": req.Comment},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.Submit: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Submit: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── Review ──────────────────────────────────────────────────────────────────

// Review transitions PENDING_REVIEW → PENDING_APPROVAL and stores reviewer signature.
func (s *Service) Review(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Review: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Review: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanReview() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa review dari status %s.", p.WorkflowStatus))
	}
	if p.MakerID == actorID {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeSoDViolation),
			"Anda tidak bisa menjadi reviewer untuk penempatan yang Anda buat sendiri (DEC-017).")
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "REVIEW", id, now, req.Comment)

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:             StatusPendingApproval,
		ReviewerID:            &actorID,
		ReviewerSignedAt:      &now,
		ReviewerSignatureHash: sigHash,
		CommentReview:         &req.Comment,
		UpdatedBy:             actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Review: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.REVIEWED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"reviewer_id": actorID, "signature_hash": hex.EncodeToString(sigHash), "comment": req.Comment},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.Review: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Review: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── Approve ──────────────────────────────────────────────────────────────────

// ApproveResult is returned by Approve.
type ApproveResult struct {
	Penempatan      *Penempatan
	StagingAction   string  // STAGE_1_ASSIGNED | SKIPPED_FVTPL
	EIRComputeJobID *string // nil for FVTPL/FVOCI_ELECTION
}

// Approve transitions a penempatan from PENDING_APPROVAL to APPROVED_ACTIVE (approver signs off).
func (s *Service) Approve(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*ApproveResult, error) {
	if claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeStepUpRequired),
			"Persetujuan penempatan memerlukan MFA step-up.")
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanApprove() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa approve dari status %s.", p.WorkflowStatus))
	}
	if p.MakerID == actorID {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeSoDViolation), "Approver tidak boleh sama dengan maker (DEC-017).")
	}
	if p.ReviewerID != nil && *p.ReviewerID == actorID {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeSoDViolation), "Approver tidak boleh sama dengan reviewer (DEC-017).")
	}

	open, err := s.repo.IsPeriodeOpen(ctx, tx, p.PeriodeID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: check periode: %w", err)
	}
	if !open {
		return nil, domainerrors.New(domainerrors.Code(ErrCodePeriodeHardClosed),
			"Periode buku sudah di-close. Penempatan tidak dapat di-approve.")
	}

	instr, err := s.repo.GetInstrumenInfo(ctx, p.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: get instrumen: %w", err)
	}
	if instr == nil {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeInstrumenNotFound), "Instrumen tidak ditemukan saat approve.")
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "APPROVE", id, now, req.Comment)

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:             StatusApprovedActive,
		ApproverID:            &actorID,
		ApproverSignedAt:      &now,
		ApproverSignatureHash: sigHash,
		CommentApprove:        &req.Comment,
		UpdatedBy:             actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)

	klasifikasi := instr.KlasifikasiPSAK71
	isFVTPL := klasifikasi == "FVTPL" || klasifikasi == "FVOCI_ELECTION"
	var stagingAction string

	if isFVTPL {
		stagingAction = "SKIPPED_FVTPL"
		if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
			Action:     "PENEMPATAN.STAGING_SKIPPED_FVTPL",
			EntityType: "trx.penempatan_deposito",
			EntityID:   id,
			After:      map[string]any{"klasifikasi": klasifikasi, "reason": "DEC-P5-M1-001"},
		})); auditErr != nil {
			return nil, fmt.Errorf("penempatan.Service.Approve: audit staging_skipped: %w", auditErr)
		}
	} else {
		stagingAction = "STAGE_1_ASSIGNED"
		if err = s.repo.InsertStageHistory(ctx, tx, p.InstrumenID, id, p.PeriodeID, actorID, claims.TenantID); err != nil {
			return nil, fmt.Errorf("penempatan.Service.Approve: insert stage history: %w", err)
		}
		if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
			Action:     "PENEMPATAN.STAGING_INITIAL",
			EntityType: "trx.penempatan_deposito",
			EntityID:   id,
			After:      map[string]any{"stage": "STAGE_1", "trigger": "INITIAL_PLACEMENT"},
		})); auditErr != nil {
			return nil, fmt.Errorf("penempatan.Service.Approve: audit staging_initial: %w", auditErr)
		}
	}

	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.APPROVED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"approver_id": actorID, "signature_hash": hex.EncodeToString(sigHash), "comment": req.Comment, "staging_action": stagingAction},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: audit approve: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: commit: %w", err)
	}

	var eirJobID *string
	if s.asynqClient != nil {
		if !isFVTPL {
			payload := EIRComputePayload{
				PenempatanID:      id,
				InstrumenID:       p.InstrumenID,
				KlasifikasiPSAK71: klasifikasi,
				NominalIDR:        p.NominalIDR,
				KuponPersen:       p.KuponPersen,
				TenorBulan:        p.TenorBulan,
				BiayaTransaksiIDR: p.BiayaTransaksiIDR,
				TanggalPenempatan: p.TanggalPenempatan,
				TanggalJatuhTempo: p.TanggalJatuhTempo,
				PeriodeID:         p.PeriodeID,
				TenantID:          claims.TenantID,
			}
			payloadBytes, marshalErr := json.Marshal(payload)
			if marshalErr != nil {
				s.logger.WarnContext(ctx, "EIR_COMPUTE payload marshal failed (non-blocking)", "error", marshalErr, "penempatan_id", id)
			} else {
				info, enqErr := s.asynqClient.EnqueueContext(ctx, asynq.NewTask(EIRComputeTaskType, payloadBytes))
				if enqErr != nil {
					s.logger.WarnContext(ctx, "EIR_COMPUTE enqueue failed (non-blocking)", "error", enqErr, "penempatan_id", id)
				} else if info != nil {
					eirJobID = &info.ID
				}
			}
		}

		approvedEvt := ApprovedEvent{
			InstrumenID:       p.InstrumenID,
			PenempatanID:      id,
			KodeTransaksi:     p.KodeTransaksi,
			KlasifikasiPSAK71: klasifikasi,
			TanggalPenempatan: p.TanggalPenempatan,
			TanggalJatuhTempo: p.TanggalJatuhTempo,
			NominalIDR:        p.NominalIDR,
			NominalFCY:        p.NominalFCY,
			KursPenempatan:    p.KursPenempatan,
			KuponPersen:       p.KuponPersen,
			TenorBulan:        p.TenorBulan,
			BiayaTransaksiIDR: p.BiayaTransaksiIDR,
			PeriodeID:         p.PeriodeID,
			StagingAction:     stagingAction,
			EventTime:         now,
			TenantID:          claims.TenantID,
		}
		approvedBytes, marshalErr2 := json.Marshal(approvedEvt)
		if marshalErr2 != nil {
			s.logger.WarnContext(ctx, "ApprovedEvent marshal failed (non-blocking)", "error", marshalErr2)
		} else if _, enqErr2 := s.asynqClient.EnqueueContext(ctx, asynq.NewTask(PenempatanApprovedTaskType, approvedBytes)); enqErr2 != nil {
			s.logger.WarnContext(ctx, "ApprovedEvent enqueue failed (non-blocking)", "error", enqErr2)
		}
	}

	result, err := s.repo.Get(ctx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Approve: re-fetch: %w", err)
	}
	result.StagingAction = stagingAction
	result.EIRComputeJobID = eirJobID

	return &ApproveResult{
		Penempatan:      result,
		StagingAction:   stagingAction,
		EIRComputeJobID: eirJobID,
	}, nil
}

// ─── Reject ──────────────────────────────────────────────────────────────────

// Reject resets a PENDING_REVIEW or PENDING_APPROVAL penempatan back to DRAFT.
func (s *Service) Reject(ctx context.Context, id uuid.UUID, req RejectActionRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Reject: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.Reject: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanReject() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa reject dari status %s.", p.WorkflowStatus))
	}

	fromApproval := p.WorkflowStatus == StatusPendingApproval
	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:    StatusDraft,
		RejectReason: &req.Comment,
		UpdatedBy:    actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Reject: update: %w", err)
	}
	if err = s.repo.ResetReviewer(ctx, tx, id, fromApproval, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Reject: reset reviewer: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.REJECTED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"reject_reason": req.Comment, "rejected_by": actorID},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.Reject: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.Reject: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── TerminateRequest ────────────────────────────────────────────────────────

// TerminateRequest proposes early termination of an APPROVED_ACTIVE penempatan (4-eyes).
func (s *Service) TerminateRequest(ctx context.Context, id uuid.UUID, req TerminateRequestBody, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateRequest: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateRequest: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanRequestTerminate() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeTerminateForbiddenNotActive),
			fmt.Sprintf("Terminasi hanya bisa diajukan dari status APPROVED_ACTIVE. Status: %s.", p.WorkflowStatus))
	}

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:              StatusTerminationPendingReview,
		TerminateMakerID:       &actorID,
		TerminateRequestReason: &req.TerminateReason,
		UpdatedBy:              actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateRequest: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.TERMINATE_PROPOSED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"terminate_reason": req.TerminateReason, "terminate_maker_id": actorID},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateRequest: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateRequest: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── TerminateReview ─────────────────────────────────────────────────────────

// TerminateReview signs the termination proposal (reviewer step, SoD enforced).
func (s *Service) TerminateReview(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReview: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReview: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanTerminateReview() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa review terminate dari status %s.", p.WorkflowStatus))
	}
	if p.MakerID == actorID {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeSoDViolation),
			"Maker tidak bisa menjadi reviewer untuk proposal terminasi yang diajukan sendiri.")
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "TERMINATE_REVIEW", id, now, req.Comment)

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:                      StatusTerminationPendingApproval,
		TerminateReviewerID:            &actorID,
		TerminateReviewerSignedAt:      &now,
		TerminateReviewerSignatureHash: sigHash,
		TerminateReviewComment:         &req.Comment,
		UpdatedBy:                      actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReview: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.TERMINATE_REVIEWED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"terminate_reviewer_id": actorID, "signature_hash": hex.EncodeToString(sigHash), "comment": req.Comment},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReview: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReview: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── TerminateApprove ────────────────────────────────────────────────────────

// TerminateApprove finalizes early termination and emits TerminatedEvent (MFA step-up required).
func (s *Service) TerminateApprove(ctx context.Context, id uuid.UUID, req WorkflowActionRequest, claims *auth.Claims) (*Penempatan, error) {
	if claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeStepUpRequired), "Persetujuan terminasi memerlukan MFA step-up.")
	}

	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateApprove: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateApprove: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanTerminateApprove() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa approve terminate dari status %s.", p.WorkflowStatus))
	}
	if p.MakerID == actorID {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeSoDViolation), "Terminate approver tidak boleh sama dengan maker (DEC-017).")
	}
	if p.TerminateReviewerID != nil && *p.TerminateReviewerID == actorID {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeSoDViolation), "Terminate approver tidak boleh sama dengan terminate reviewer (DEC-017).")
	}

	now := time.Now()
	sigHash := computeSignatureHash(actorID, "TERMINATE_APPROVE", id, now, req.Comment)

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:                      StatusTerminated,
		TerminateApproverID:            &actorID,
		TerminateApproverSignedAt:      &now,
		TerminateApproverSignatureHash: sigHash,
		TerminateApproveComment:        &req.Comment,
		TerminatedAt:                   &now,
		UpdatedBy:                      actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateApprove: update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.TERMINATE_APPROVED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"terminate_approver_id": actorID, "signature_hash": hex.EncodeToString(sigHash), "comment": req.Comment, "terminated_at": now},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateApprove: audit approve: %w", auditErr)
	}

	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.DERECOGNITION_QUEUED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"event_type": "TERMINATE", "downstream": "P5-M9"},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateApprove: audit derecognition: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateApprove: commit: %w", err)
	}

	if s.asynqClient != nil {
		terminatedEvt := TerminatedEvent{
			InstrumenID:   p.InstrumenID,
			PenempatanID:  id,
			KodeTransaksi: p.KodeTransaksi,
			TerminateDate: now,
			NominalIDR:    p.NominalIDR,
			EIRAwal:       p.EIRAwal,
			PeriodeID:     p.PeriodeID,
			EventTime:     now,
			TenantID:      claims.TenantID,
		}
		if p.TerminateRequestReason != nil {
			terminatedEvt.TerminateReason = *p.TerminateRequestReason
		}
		instr, instrErr := s.repo.GetInstrumenInfo(ctx, p.InstrumenID)
		if instrErr != nil {
			s.logger.WarnContext(ctx, "TerminateApprove: get instrumen info failed (non-blocking)", "error", instrErr, "instrumen_id", p.InstrumenID)
		} else if instr != nil {
			terminatedEvt.KlasifikasiPSAK71 = instr.KlasifikasiPSAK71
		}
		evtBytes, marshalErr := json.Marshal(terminatedEvt)
		if marshalErr != nil {
			s.logger.WarnContext(ctx, "TerminatedEvent marshal failed (non-blocking)", "error", marshalErr)
		} else if _, enqErr := s.asynqClient.EnqueueContext(ctx, asynq.NewTask(PenempatanTerminatedTaskType, evtBytes)); enqErr != nil {
			s.logger.WarnContext(ctx, "TerminatedEvent enqueue failed (non-blocking)", "error", enqErr)
		}
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── TerminateReject ──────────────────────────────────────────────────────────

// TerminateReject cancels the termination proposal, returning the penempatan to APPROVED_ACTIVE.
func (s *Service) TerminateReject(ctx context.Context, id uuid.UUID, req RejectActionRequest, claims *auth.Claims) (*Penempatan, error) {
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrForbidden("invalid actor UUID in JWT")
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReject: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	p, err := s.repo.GetForUpdate(ctx, tx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReject: get: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if !p.WorkflowStatus.CanTerminateReject() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("Tidak bisa reject terminate dari status %s.", p.WorkflowStatus))
	}

	if err = s.repo.UpdateStatus(ctx, tx, id, StatusUpdate{
		NewStatus:             StatusApprovedActive,
		TerminateRejectReason: &req.Comment,
		UpdatedBy:             actorID,
	}, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReject: update: %w", err)
	}
	if err = s.repo.ResetTerminateReviewer(ctx, tx, id, claims.TenantID); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReject: reset: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "PENEMPATAN.TERMINATE_REJECTED",
		EntityType: "trx.penempatan_deposito",
		EntityID:   id,
		After:      map[string]any{"reject_comment": req.Comment, "rejected_by": actorID},
	})); auditErr != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReject: audit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("penempatan.Service.TerminateReject: commit: %w", err)
	}

	return s.repo.Get(ctx, id, claims.TenantID)
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetByID loads a single penempatan by ID, populating the settlement balance hint.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, claims *auth.Claims) (*Penempatan, error) {
	p, err := s.repo.Get(ctx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.GetByID: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	if p.SettlementAccount != nil && *p.SettlementAccount != "" {
		hint, hintErr := s.repo.GetSettlementBalanceHint(ctx, *p.SettlementAccount, claims.TenantID)
		if hintErr != nil {
			s.logger.WarnContext(ctx, "settlement balance hint lookup failed (non-blocking)", "error", hintErr)
		} else {
			p.SettlementBalanceHint = hint
		}
	}
	return p, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// List returns a cursor-paginated, filterable list of penempatan for the DataTable.
func (s *Service) List(ctx context.Context, q listquery.Query, includeDeleted bool, claims *auth.Claims) (ListResult, error) {
	return s.repo.List(ctx, q, includeDeleted, claims.TenantID)
}

// ─── EIRPreview ───────────────────────────────────────────────────────────────

// EIRPreview returns a simplified EIR amortization preview (full solver delegated to P4-M5).
func (s *Service) EIRPreview(ctx context.Context, id uuid.UUID, claims *auth.Claims) (*EIRPreviewResult, error) {
	p, err := s.repo.Get(ctx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.EIRPreview: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}

	if p.KlasifikasiPSAK71 == "FVTPL" || p.KlasifikasiPSAK71 == "FVOCI_ELECTION" {
		info := fmt.Sprintf("EIR tidak dihitung untuk instrumen %s (DEC-P5-M1-001). Fair value via MTM engine P5-M6.", p.KlasifikasiPSAK71)
		return &EIRPreviewResult{
			EIRAwal:              nil,
			CarryingAmountAwal:   nil,
			PeriodePreview:       0,
			Info:                 &info,
			AmortizationSchedule: []AmortizationRow{},
		}, nil
	}

	if p.NominalIDR.IsZero() {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeCalc2010), "EIR tidak dapat dihitung: nominal_idr wajib diisi.")
	}
	if p.TenorBulan <= 0 {
		return nil, domainerrors.New(domainerrors.Code(ErrCodeCalc2010), "EIR tidak dapat dihitung: tenor_bulan harus > 0.")
	}

	// Coupon-based monthly rate approximation (full Newton-Raphson by ecl-eir-engineer).
	hundred := decimal.NewFromInt(100)
	twelve := decimal.NewFromInt(12)
	monthlyRate := p.KuponPersen.Div(hundred).Div(twelve)
	carryingAmount := p.NominalIDR.Add(p.BiayaTransaksiIDR)

	previewMonths := 10
	if int(p.TenorBulan) < previewMonths {
		previewMonths = int(p.TenorBulan)
	}

	schedule := make([]AmortizationRow, previewMonths)
	currentDate := p.TanggalPenempatan
	for i := 0; i < previewMonths; i++ {
		currentDate = currentDate.AddDate(0, 1, 0)
		interest := carryingAmount.Mul(monthlyRate)
		isLast := i == int(p.TenorBulan)-1
		var principal decimal.Decimal
		ca := carryingAmount
		if isLast {
			principal = p.NominalIDR
			ca = decimal.Zero
		}
		schedule[i] = AmortizationRow{
			Periode:         i + 1,
			TanggalAngsuran: currentDate,
			AngsuranBunga:   interest,
			AngsuranPokok:   principal,
			CarryingAmount:  ca,
		}
	}

	return &EIRPreviewResult{
		EIRAwal:              &monthlyRate,
		CarryingAmountAwal:   &carryingAmount,
		PeriodePreview:       previewMonths,
		AmortizationSchedule: schedule,
	}, nil
}

// ─── AuditTimeline ────────────────────────────────────────────────────────────

// AuditTimeline returns the audit trail for a penempatan (before/after redacted for non-AUDIT roles).
func (s *Service) AuditTimeline(ctx context.Context, id uuid.UUID, claims *auth.Claims) ([]AuditTimelineEvent, error) {
	p, err := s.repo.Get(ctx, id, claims.TenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Service.AuditTimeline: %w", err)
	}
	if p == nil {
		return nil, domainerrors.ErrNotFound("Penempatan")
	}
	isAuditRole := claims.HasPermission(PermAuditLogRead)
	return s.repo.GetAuditTimeline(ctx, id, isAuditRole, claims.TenantID)
}

// ─── ProcessMaturity ─────────────────────────────────────────────────────────

// ProcessMaturity transitions APPROVED_ACTIVE instruments with jatuh_tempo ≤ asOfDate to MATURED.
func (s *Service) ProcessMaturity(ctx context.Context, asOfDate time.Time, tenantID string) (int, error) {
	systemActorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	maturing, err := s.repo.GetMaturingInstruments(ctx, asOfDate, tenantID)
	if err != nil {
		return 0, fmt.Errorf("penempatan.Service.ProcessMaturity: list: %w", err)
	}

	maturedCount := 0
	for i := range maturing {
		p := &maturing[i]
		if mErr := s.processOneMature(ctx, *p, systemActorID, tenantID); mErr != nil {
			s.logger.ErrorContext(ctx, "maturity processing failed", "penempatan_id", p.ID, "error", mErr)
			continue
		}
		maturedCount++
	}

	return maturedCount, nil
}

func (s *Service) processOneMature(ctx context.Context, p Penempatan, systemActorID uuid.UUID, tenantID string) error {
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("processOneMature begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	current, err := s.repo.GetForUpdate(ctx, tx, p.ID, tenantID)
	if err != nil {
		return fmt.Errorf("processOneMature re-check: %w", err)
	}
	if current == nil || current.WorkflowStatus != StatusApprovedActive {
		// Nothing to do; defer rollbackTx will clean up the idle tx.
		return nil
	}

	now := time.Now()
	if err = s.repo.UpdateStatus(ctx, tx, p.ID, StatusUpdate{
		NewStatus: StatusMatured,
		MaturedAt: &now,
		UpdatedBy: systemActorID,
	}, tenantID); err != nil {
		return fmt.Errorf("processOneMature update: %w", err)
	}

	txWriter := s.auditWriter.WithTx(tx)
	if auditErr := txWriter.Write(ctx, audit.Event{
		Action:      "PENEMPATAN.MATURED",
		EntityType:  "trx.penempatan_deposito",
		EntityID:    p.ID,
		ActorUserID: systemActorID.String(),
		ActorRole:   "SYSTEM",
		After:       map[string]any{"matured_at": now, "kode_transaksi": p.KodeTransaksi},
	}); auditErr != nil {
		return fmt.Errorf("processOneMature audit matured: %w", auditErr)
	}

	if auditErr := txWriter.Write(ctx, audit.Event{
		Action:      "PENEMPATAN.DERECOGNITION_QUEUED",
		EntityType:  "trx.penempatan_deposito",
		EntityID:    p.ID,
		ActorUserID: systemActorID.String(),
		ActorRole:   "SYSTEM",
		After:       map[string]any{"event_type": "MATURE", "downstream": "P5-M9"},
	}); auditErr != nil {
		return fmt.Errorf("processOneMature audit derecognition: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("processOneMature commit: %w", err)
	}

	if s.asynqClient != nil {
		maturedEvt := MaturedEvent{
			InstrumenID:       p.InstrumenID,
			PenempatanID:      p.ID,
			KodeTransaksi:     p.KodeTransaksi,
			KlasifikasiPSAK71: p.KlasifikasiPSAK71,
			TanggalJatuhTempo: p.TanggalJatuhTempo,
			MaturedAt:         now,
			NominalIDR:        p.NominalIDR,
			EIRAwal:           p.EIRAwal,
			PeriodeID:         p.PeriodeID,
			EventTime:         now,
			TenantID:          tenantID,
		}
		evtBytes, marshalErr := json.Marshal(maturedEvt)
		if marshalErr != nil {
			s.logger.WarnContext(ctx, "MaturedEvent marshal failed (non-blocking)", "error", marshalErr, "id", p.ID)
		} else if _, enqErr := s.asynqClient.EnqueueContext(ctx, asynq.NewTask(PenempatanMaturedTaskType, evtBytes)); enqErr != nil {
			s.logger.WarnContext(ctx, "MaturedEvent enqueue failed", "error", enqErr, "id", p.ID)
		}
	}

	return nil
}

// ─── computeSignatureHash ─────────────────────────────────────────────────────

func computeSignatureHash(userID uuid.UUID, step string, entityID uuid.UUID, signedAt time.Time, comment string) []byte {
	h := sha256.New()
	h.Write([]byte(userID.String()))
	h.Write([]byte(step))
	h.Write([]byte(entityID.String()))
	h.Write([]byte(signedAt.Format(time.RFC3339Nano)))
	h.Write([]byte(comment))
	return h.Sum(nil)
}

// rollbackTx rolls back a transaction, logging any error at WARN level.
// Mirrors the pattern in internal/ecl/calcrun/service.go.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		if logger == nil {
			logger = slog.Default()
		}
		logger.WarnContext(ctx, "penempatan: tx rollback error", "error", err)
	}
}
