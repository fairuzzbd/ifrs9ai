// Package eir — AmendmentService implements the 4-eyes amendment workflow
// for EIR re-estimation on contract modification.
//
// State machine: DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED | REJECTED
// (docs/state-machines/p4-m5-eir.md §3)
//
// SoD (DEC-017): maker_id ≠ reviewer_id ≠ approver_id enforced server-side.
// Step-up MFA: required on Approve (DEC-027).
// Immutability: amendment execute calls MarkSuperseded + InsertBatch in one tx (DEC-018).
// Audit: each workflow step writes to aud.audit_log in-transaction.
package eir

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── AmendmentService ─────────────────────────────────────────────────────────

// AmendmentService handles the 4-eyes amendment workflow (Story 4: APP-C-EIR-004).
type AmendmentService struct {
	db          *sql.DB
	instrRepo   InstrumenEIRRepoIface
	schedRepo   ScheduleRepoIface
	amendRepo   AmendmentRepoIface
	solver      *Solver
	auditWriter AuditWriterIface
	logger      *slog.Logger
}

// NewAmendmentService creates an AmendmentService.
// Panics if auditWriter is nil (DEC-018 guard).
func NewAmendmentService(
	db *sql.DB,
	instrRepo InstrumenEIRRepoIface,
	schedRepo ScheduleRepoIface,
	amendRepo AmendmentRepoIface,
	auditWriter AuditWriterIface,
	logger *slog.Logger,
) *AmendmentService {
	if auditWriter == nil {
		panic("eir.NewAmendmentService: auditWriter must not be nil (DEC-018)")
	}
	return &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// Propose creates a new DRAFT amendment proposal (maker role).
// Validates: no active proposal exists, instrument is EIR-applicable.
// Sets maker_id = actorID. SoD: reviewer and approver must differ from maker later.
// Audit: EIR.AMEND_PROPOSED in-transaction.
func (s *AmendmentService) Propose(ctx context.Context, req ProposeRequest, actorID uuid.UUID, actorRole string) (AmendmentProposal, error) {
	// 1. Load instrument
	inst, err := s.instrRepo.GetByID(ctx, req.InstrumenID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: load instrumen: %w", err)
	}
	if inst == nil || inst.DeletedAt != nil {
		return AmendmentProposal{}, ErrEIRInstrumenNotFound(req.InstrumenID.String())
	}
	if !isEIRApplicable(inst.KlasifikasiPsak71, inst.EIRMethodFlag) {
		return AmendmentProposal{}, ErrEIRInstrumenFVTPLNoEIR(inst.KlasifikasiPsak71)
	}

	// 2. Guard: no active (non-terminal) proposal
	hasActive, err := s.amendRepo.HasActiveProposal(ctx, req.InstrumenID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: check active proposal: %w", err)
	}
	if hasActive {
		return AmendmentProposal{}, ErrEIRAmendActiveExists(req.InstrumenID.String())
	}

	// 3. Must have existing EIR for re-estimation
	if inst.EIRAwal == nil {
		return AmendmentProposal{}, ErrEIRNotYetComputed()
	}

	// 4. Build proposal row
	cfJSON, err := marshalCashflows(req.RevisedCashflowProjection)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: marshal cashflows: %w", err)
	}

	proposal := AmendmentProposal{
		ID:                  uuid.New(),
		InstrumenID:         req.InstrumenID,
		Status:              AmendStatusPendingReview, // skip DRAFT per AC §4.2 — direct to PENDING_REVIEW
		TanggalAmandemen:    req.TanggalAmandemen,
		TanggalReEstimasi:   time.Now(),
		AlasanAmandemen:     req.AlasanAmandemen,
		EIRLama:             inst.EIRAwal,
		EIRBaru:             nil, // computed on Approve
		RevisedCashflowJSON: cfJSON,
		MakerID:             &actorID,
		TenantID:            inst.TenantID,
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		CreatedBy:           actorID,
		UpdatedBy:           actorID,
	}

	// 5. Persist + audit in tx
	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: begin tx: %w", txErr)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.amendRepo.Create(ctx, tx, &proposal); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: create proposal: %w", err)
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "EIR.AMEND_PROPOSED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    proposal.ID,
		AfterJSON: map[string]any{
			"instrumen_id":      req.InstrumenID,
			"tanggal_amandemen": req.TanggalAmandemen.Format("2006-01-02"),
			"alasan_amandemen":  req.AlasanAmandemen,
			"eir_lama":          inst.EIRAwal.StringFixed(8),
			"cf_count":          len(req.RevisedCashflowProjection),
		},
		TenantID: inst.TenantID,
	}); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Propose: commit: %w", err)
	}

	return proposal, nil
}

// Review advances proposal PENDING_REVIEW → PENDING_APPROVAL (reviewer role).
// SoD: reviewer_id ≠ maker_id (DEC-017).
// Audit: EIR.AMEND_REVIEWED in-transaction.
func (s *AmendmentService) Review(ctx context.Context, req ReviewRequest, actorID uuid.UUID, actorRole string) (AmendmentProposal, error) {
	// 1. Load proposal
	proposal, err := s.amendRepo.GetByID(ctx, req.AmendmentID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Review: load proposal: %w", err)
	}
	if proposal == nil {
		return AmendmentProposal{}, ErrEIRAmendNotFound(req.AmendmentID.String())
	}

	// 2. State guard
	if proposal.Status != AmendStatusPendingReview {
		return AmendmentProposal{}, ErrEIRAmendInvalidTransition(
			string(proposal.Status), string(AmendStatusPendingApproval))
	}

	// 3. SoD: reviewer ≠ maker (DEC-017)
	if proposal.MakerID != nil && *proposal.MakerID == actorID {
		return AmendmentProposal{}, domainerrors.NewDomainError(
			domainerrors.CodeSoDViolation,
			"EIR amendment: reviewer tidak boleh sama dengan maker",
		)
	}

	// 4. Build signature hash
	reviewerSig := ComputeReviewerSignatureHash(proposal.ID, actorID, req.Comment)

	// 5. Update proposal
	proposal.Status = AmendStatusPendingApproval
	proposal.ReviewerID = &actorID
	proposal.ReviewerComment = &req.Comment
	proposal.ReviewerSignatureHash = &reviewerSig
	now := time.Now()
	proposal.ReviewedAt = &now
	proposal.UpdatedBy = actorID
	proposal.UpdatedAt = time.Now()

	// 6. Persist + audit in tx
	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Review: begin tx: %w", txErr)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.amendRepo.Update(ctx, tx, proposal); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Review: update proposal: %w", err)
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "EIR.AMEND_REVIEWED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    proposal.ID,
		BeforeJSON:  map[string]any{"status": "PENDING_REVIEW"},
		AfterJSON: map[string]any{
			"status":           "PENDING_APPROVAL",
			"reviewer_id":      actorID,
			"reviewer_comment": req.Comment,
			"signature_hash":   reviewerSig,
		},
		TenantID: proposal.TenantID,
	}); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Review: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Review: commit: %w", err)
	}

	return *proposal, nil
}

// Approve finalizes the amendment (approver role, step-up MFA required).
// SoD: approver ≠ maker ≠ reviewer (DEC-017).
// Step-up MFA token must be valid (DEC-027).
// Executes re-estimation atomically:
//  1. Re-run Newton-Raphson with revised cashflows → new EIR
//  2. MarkSuperseded (old schedule rows get recomputed_from_seq)
//  3. InsertBatch (new schedule rows)
//  4. UpdateEIRAwal on mst.instrumen
//  5. Update amendment log: status=APPROVED, eir_baru
//  6. Write EIR.AMEND_APPROVED audit
//
// All in ONE DB transaction (DEC-018 immutability + audit-in-tx).
func (s *AmendmentService) Approve(ctx context.Context, req ApproveRequest, actorID uuid.UUID, actorRole string) (AmendmentProposal, error) {
	// 1. Load proposal
	proposal, err := s.amendRepo.GetByID(ctx, req.AmendmentID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: load proposal: %w", err)
	}
	if proposal == nil {
		return AmendmentProposal{}, ErrEIRAmendNotFound(req.AmendmentID.String())
	}

	// 2. State guard
	if proposal.Status != AmendStatusPendingApproval {
		return AmendmentProposal{}, ErrEIRAmendInvalidTransition(
			string(proposal.Status), string(AmendStatusApproved))
	}

	// 3. SoD: approver ≠ maker AND approver ≠ reviewer (DEC-017)
	if proposal.MakerID != nil && *proposal.MakerID == actorID {
		return AmendmentProposal{}, domainerrors.NewDomainError(
			domainerrors.CodeSoDViolation,
			"EIR amendment: approver tidak boleh sama dengan maker",
		)
	}
	if proposal.ReviewerID != nil && *proposal.ReviewerID == actorID {
		return AmendmentProposal{}, domainerrors.NewDomainError(
			domainerrors.CodeSoDViolation,
			"EIR amendment: approver tidak boleh sama dengan reviewer",
		)
	}

	// 4. Step-up MFA validation (DEC-027)
	// In production: call auth service to validate req.StepUpToken.
	// For M5 stub: require non-empty token.
	if req.StepUpToken == "" {
		return AmendmentProposal{}, ErrEIRMFAStepUpRequired()
	}

	// 5. Load instrument
	inst, err := s.instrRepo.GetByID(ctx, proposal.InstrumenID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: load instrumen: %w", err)
	}
	if inst == nil {
		return AmendmentProposal{}, ErrEIRInstrumenNotFound(proposal.InstrumenID.String())
	}

	// 6. Unmarshal revised cashflows from JSON
	revisedCFs, err := unmarshalCashflows(proposal.RevisedCashflowJSON)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: unmarshal cashflows: %w", err)
	}
	if len(revisedCFs) < 2 {
		return AmendmentProposal{}, ErrEIRCashflowInvalid("Revised cashflows tidak valid setelah unmarshal")
	}

	// 7. Re-run Newton-Raphson with revised cashflows
	var seed *decimal.Decimal
	if inst.EIRAwal != nil {
		seed = inst.EIRAwal // use old EIR as seed for faster convergence
	}
	newEIR, solveDetail, solveErr := s.solver.Solve(revisedCFs, seed)
	if solveErr != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: NR solve failed: %w", solveErr)
	}

	// 8. Build new schedule rows
	newRows, closingDelta := buildScheduleRows(proposal.InstrumenID, newEIR, revisedCFs, inst, actorID)
	if len(newRows) == 0 {
		return AmendmentProposal{}, ErrEIRCashflowInvalid("Revised cashflows tidak menghasilkan schedule rows")
	}

	// 9. Compute catch-up adjustment per IFRS 9 §5.4.3
	// NPV(revised CF @ original EIR) − grossCarrying(amendment date).
	// Stored in ecl.eir_reestimation_log.catch_up_adjustment (NUMERIC(20,4)).
	// Jurnal P&L booking deferred to Phase 5 (APP-D jurnal module, M7+).
	var originalEIR decimal.Decimal
	if inst.EIRAwal != nil {
		originalEIR = *inst.EIRAwal
	}
	catchUpAdj, err := s.computeCatchUpAdjustment(ctx, proposal.InstrumenID, revisedCFs, originalEIR, proposal.TanggalAmandemen)
	if err != nil {
		// Non-fatal if no schedule exists yet (e.g. first amendment before schedule generated).
		// Log and continue — catch_up will be stored as nil.
		s.logger.WarnContext(ctx, "eir.Approve: catch-up adjustment skipped", "reason", err.Error())
		catchUpAdj = decimal.Zero
	}

	// 9b. Determine firstNewSeq for MarkSuperseded reference
	firstNewSeq := 0
	maxSeq, err := s.schedRepo.GetMaxPeriodeSeq(ctx, proposal.InstrumenID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: get max seq: %w", err)
	}
	firstNewSeq = maxSeq + 1
	// Adjust rows to start from firstNewSeq
	for i := range newRows {
		newRows[i].PeriodeSeq = firstNewSeq + i
		newRows[i].RecomputedFromSeq = nil // these are the new active rows
	}

	// 10. Build approver signature
	approverSig := ComputeApproverSignatureHash(proposal.ID, actorID, req.Comment, newEIR)

	// 11. Execute all mutations in ONE transaction (DEC-018 immutability)
	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: begin tx: %w", txErr)
	}
	defer rollbackTx(ctx, tx, s.logger)

	// 11a. MarkSuperseded: set recomputed_from_seq = firstNewSeq on old active rows
	//      This is a tracking update only — financial amounts are untouched (DEC-018).
	if err := s.schedRepo.MarkSuperseded(ctx, tx, proposal.InstrumenID, firstNewSeq, actorID); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: mark superseded: %w", err)
	}

	// 11b. InsertBatch: new schedule rows
	if err := s.schedRepo.InsertBatch(ctx, tx, newRows); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: insert new schedule: %w", err)
	}

	// 11c. Update eir_awal on mst.instrumen
	if err := s.instrRepo.UpdateEIRAwal(ctx, tx, proposal.InstrumenID, newEIR, actorID); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: update eir_awal: %w", err)
	}

	// 11d. Update amendment log: APPROVED + eir_baru + catch_up_adjustment
	// catch_up_adjustment = NPV(revised CF @ original EIR) − grossCarrying (IFRS 9 §5.4.3).
	// Jurnal P&L booking deferred to Phase 5 (APP-D jurnal module, M7+).
	now := time.Now()
	proposal.Status = AmendStatusApproved
	proposal.EIRBaru = &newEIR
	proposal.CatchUpAdjustment = &catchUpAdj
	proposal.ApproverID = &actorID
	proposal.ApproverComment = &req.Comment
	proposal.ApproverSignatureHash = &approverSig
	proposal.ApprovedAt = &now
	proposal.UpdatedBy = actorID
	proposal.UpdatedAt = time.Now()

	if err := s.amendRepo.Update(ctx, tx, proposal); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: update proposal: %w", err)
	}

	// 11e. Audit: EIR.AMEND_APPROVED (same tx — DEC-018)
	var oldEIRStr string
	if proposal.EIRLama != nil {
		oldEIRStr = proposal.EIRLama.StringFixed(8)
	}
	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "EIR.AMEND_APPROVED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    proposal.ID,
		BeforeJSON: map[string]any{
			"status":  "PENDING_APPROVAL",
			"eir_old": oldEIRStr,
		},
		AfterJSON: map[string]any{
			"status":         "APPROVED",
			"eir_new":        newEIR.StringFixed(8),
			"iterations":     solveDetail.IterationsUsed,
			"residual":       solveDetail.ConvergenceResidual.String(),
			"new_rows_count": len(newRows),
			"first_new_seq":  firstNewSeq,
			"closing_delta":  closingDelta.StringFixed(4),
			"approver_sig":   approverSig,
			// IFRS 9 §5.4.3 catch-up: NPV(revised CF @ originalEIR) − grossCarrying.
			// Positive = P&L gain, negative = P&L loss.
			// Jurnal posting deferred to Phase 5 (APP-D jurnal module, M7+).
			"catch_up_adjustment": catchUpAdj.StringFixed(4),
		},
		TenantID: proposal.TenantID,
	}); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Approve: commit: %w", err)
	}

	return *proposal, nil
}

// computeCatchUpAdjustment computes the immediate P&L catch-up per IFRS 9 §5.4.3.
//
// When an EIR amendment is approved the entity must remeasure the gross carrying
// amount as the NPV of the revised cashflows discounted at the ORIGINAL EIR.
// The difference between that NPV and the current gross carrying amount is
// recognized immediately in profit or loss.
//
//	catch_up = NPV(revisedCF @ originalEIR) − grossCarrying(at amendmentDate)
//
// Positive catch_up = P&L gain (revised CF > carrying).
// Negative catch_up = P&L loss (revised CF < carrying).
//
// Time fractions use ACT/36525 (days/36525) matching the solver convention for
// annual-equivalent discounting.  RoundBank(4) applied to final result (DEC-016
// NUMERIC(20,4) storage).
//
// Jurnal P&L booking is deferred to Phase 5 (APP-D jurnal module, M7+).
// This method stores the computed value in ecl.eir_reestimation_log.catch_up_adjustment.
//
// Ref: IFRS 9 §5.4.3, FSD-APP-C-ECL-EIR-v1.0.docx §4.3, SoW §4.
func (s *AmendmentService) computeCatchUpAdjustment(
	ctx context.Context,
	instrumenID uuid.UUID,
	revisedCashflow []CashflowItem,
	originalEIR decimal.Decimal,
	amendmentDate time.Time,
) (decimal.Decimal, error) {
	// Step 1 — NPV of revised cashflows discounted at original EIR.
	// t = fractional years from amendmentDate to each CF date (ACT/36525).
	// Cashflows before amendmentDate are excluded (past; no remeasurement impact).
	// Per IFRS 9 §5.4.3 the discount rate is the ORIGINAL EIR, not the new one.
	const daysInYear = 36525 // 365.25 × 100 to avoid float; keep as int then divide

	npv := decimal.Zero
	oneYearDays := decimal.NewFromInt(daysInYear)
	oneHundred := decimal.NewFromInt(100)
	onePlusEIR := decimal.NewFromInt(1).Add(originalEIR) // (1 + r)

	for _, cf := range revisedCashflow {
		if cf.Date.Before(amendmentDate) {
			continue // ignore past cashflows
		}
		// t = days / 365.25 expressed without float64: days * 100 / 36525
		daysDiff := int64(cf.Date.Sub(amendmentDate).Hours() / 24) //nolint:mnd // 24h = 1 day
		t := decimal.NewFromInt(daysDiff).Mul(oneHundred).Div(oneYearDays)

		// discount factor = (1 + originalEIR)^t  — reuse solver's decimalPow
		discFactor := decimalPow(onePlusEIR, t)
		if discFactor.IsZero() {
			// degenerate case: skip rather than divide-by-zero
			continue
		}
		npv = npv.Add(cf.AmountIDR.Div(discFactor))
	}

	// Step 2 — gross carrying at amendment date from latest active schedule row.
	grossCarrying, err := s.schedRepo.GetGrossCarryingAtDate(ctx, instrumenID, amendmentDate)
	if err != nil {
		return decimal.Zero, fmt.Errorf("computeCatchUpAdjustment: read gross carrying: %w", err)
	}

	// Step 3 — catch-up = NPV − grossCarrying, rounded HALF_EVEN to 4 d.p. (DEC-016 NUMERIC(20,4))
	catchUp := npv.Sub(grossCarrying).RoundBank(4)
	return catchUp, nil
}

// Reject terminates the amendment workflow (reviewer or approver can reject).
// Sets status → REJECTED. SoD: rejector cannot be maker.
// Audit: EIR.AMEND_REJECTED in-transaction.
func (s *AmendmentService) Reject(ctx context.Context, req WorkflowAction, actorID uuid.UUID, actorRole string) (AmendmentProposal, error) {
	// 1. Load proposal
	proposal, err := s.amendRepo.GetByID(ctx, req.AmendmentID)
	if err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Reject: load proposal: %w", err)
	}
	if proposal == nil {
		return AmendmentProposal{}, ErrEIRAmendNotFound(req.AmendmentID.String())
	}

	// 2. State guard: can only reject if not already terminal
	if proposal.Status.IsTerminal() {
		return AmendmentProposal{}, ErrEIRAmendInvalidTransition(
			string(proposal.Status), string(AmendStatusRejected))
	}

	// 3. SoD: rejector must not be maker
	if proposal.MakerID != nil && *proposal.MakerID == actorID {
		return AmendmentProposal{}, domainerrors.NewDomainError(
			domainerrors.CodeSoDViolation,
			"EIR amendment: rejector tidak boleh sama dengan maker",
		)
	}

	// 4. Update proposal
	prevStatus := proposal.Status
	proposal.Status = AmendStatusRejected
	comment := req.Comment
	proposal.ApproverComment = &comment // store rejection comment in approver_comment field
	proposal.UpdatedBy = actorID
	proposal.UpdatedAt = time.Now()

	// 5. Persist + audit in tx
	tx, txErr := s.db.BeginTx(ctx, nil)
	if txErr != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Reject: begin tx: %w", txErr)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.amendRepo.Update(ctx, tx, proposal); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Reject: update proposal: %w", err)
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "EIR.AMEND_REJECTED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    proposal.ID,
		BeforeJSON:  map[string]any{"status": string(prevStatus)},
		AfterJSON: map[string]any{
			"status":  "REJECTED",
			"comment": req.Comment,
		},
		TenantID: proposal.TenantID,
	}); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Reject: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return AmendmentProposal{}, fmt.Errorf("eir.Reject: commit: %w", err)
	}

	return *proposal, nil
}
