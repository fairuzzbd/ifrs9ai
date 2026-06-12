// detection_service.go — P4-M6 stories M6-001 and M6-005.
//
// DetectionService.DetectFromDocument: auto-detect amendment need from document upload.
// DetectionService.CancelAmendment:    cancel a DRAFT or unsigned PENDING_REVIEW proposal.
// DetectionService.UpdateCashflows:    PATCH cashflows on a DRAFT proposal.
//
// References:
//   - docs/stories/phase-4/M6-eir-amendment-lifecycle.md §M6-001, M6-005
//   - docs/state-machines/p4-m6-amendment-lifecycle.md §4 (cancel transition table)
//   - db/migrations/000027_drift_report_and_amendment_lifecycle.up.sql
//   - DEC-016 (decimal), DEC-018 (no hard delete), DEC-017 (SoD).
package eir

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// DetectionService implements M6-001 (detect) and M6-005 (cancel).
// Separation from AmendmentService keeps M5 untouched.
type DetectionService struct {
	db          *sql.DB
	instrRepo   InstrumenEIRRepoIface
	amendRepo   AmendmentRepoIface
	auditWriter AuditWriterIface
	logger      *slog.Logger
}

// NewDetectionService constructs a DetectionService.
// Panics if auditWriter is nil (audit-in-tx is mandatory per DEC-018).
func NewDetectionService(
	db *sql.DB,
	instrRepo InstrumenEIRRepoIface,
	amendRepo AmendmentRepoIface,
	auditWriter AuditWriterIface,
	logger *slog.Logger,
) *DetectionService {
	if auditWriter == nil {
		panic("eir.DetectionService: auditWriter must not be nil (audit-in-tx mandatory)")
	}
	return &DetectionService{
		db:          db,
		instrRepo:   instrRepo,
		amendRepo:   amendRepo,
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// DetectFromDocument creates an amendment proposal triggered by a document upload (M6-001).
//
// Eligibility checks (→ 422 EIR_AMENDMENT_DETECTION_NO_MATCH if any fail):
//  1. Instrument must exist, be AC or FVOCI, eir_method_flag=TRUE.
//  2. No active (non-terminal) proposal already exists for this instrument.
//
// On success: inserts a DRAFT proposal with trigger_source='DOCUMENT_UPLOAD'.
// Audit event: "EIR.AMEND_DETECTED" in-transaction.
func (s *DetectionService) DetectFromDocument(ctx context.Context, req DetectAmendmentRequest) (*AmendmentProposal, error) {
	// 1. Load instrument.
	inst, err := s.instrRepo.GetByID(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("detection: load instrumen: %w", err)
	}
	if inst == nil {
		return nil, ErrEIRInstrumenNotFound(req.InstrumenID.String())
	}
	if !isEIRApplicableForDetection(inst) {
		return nil, ErrEIRAmendmentDetectionNoMatch(
			fmt.Sprintf("instrumen %s klasifikasi %s tidak eligible untuk EIR amendment",
				inst.KodeInstrumen, inst.KlasifikasiPsak71))
	}

	// 2. Guard against concurrent active proposal.
	hasActive, err := s.amendRepo.HasActiveProposal(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("detection: check active proposal: %w", err)
	}
	if hasActive {
		return nil, ErrEIRAmendmentDetectionNoMatch(
			"sudah ada amendment proposal aktif untuk instrumen " + inst.KodeInstrumen +
				". Selesaikan atau batalkan terlebih dahulu.")
	}

	// 3. Build proposal.
	var cashflowJSON string
	if len(req.OverrideCashflows) > 0 {
		cashflowJSON, err = marshalCashflows(req.OverrideCashflows)
		if err != nil {
			return nil, fmt.Errorf("detection: marshal cashflows: %w", err)
		}
	}

	proposal := &AmendmentProposal{
		ID:                  uuid.New(),
		InstrumenID:         req.InstrumenID,
		TanggalAmandemen:    time.Now().UTC().Truncate(24 * time.Hour),
		TanggalReEstimasi:   time.Now().UTC().Truncate(24 * time.Hour),
		AlasanAmandemen:     req.AlasanDetected,
		RevisedCashflowJSON: cashflowJSON,
		EIRLama:             inst.EIRAwal,
		Status:              AmendStatusDraft,
		TriggerSource:       AmendTriggerDocumentUpload,
		DocumentID:          &req.DocumentID,
		MakerID:             &req.ActorID,
		CreatedBy:           req.ActorID,
		UpdatedBy:           req.ActorID,
		TenantID:            req.TenantID,
	}

	// 4. Persist in tx.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("detection: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := s.amendRepo.Create(ctx, tx, proposal); err != nil {
		return nil, fmt.Errorf("detection: insert proposal: %w", err)
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: req.ActorID,
		Action:      "EIR.AMEND_DETECTED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    proposal.ID,
		AfterJSON: map[string]interface{}{
			"proposal_id":    proposal.ID.String(),
			"instrumen_id":   req.InstrumenID.String(),
			"trigger_source": string(AmendTriggerDocumentUpload),
			"document_id":    req.DocumentID.String(),
			"status":         string(AmendStatusDraft),
		},
		TenantID: req.TenantID,
	}); err != nil {
		return nil, fmt.Errorf("detection: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("detection: commit: %w", err)
	}

	s.logger.Info("eir amendment detected from document",
		slog.String("proposal_id", proposal.ID.String()),
		slog.String("instrumen_id", req.InstrumenID.String()),
		slog.String("document_id", req.DocumentID.String()),
	)
	return proposal, nil
}

// CancelAmendment cancels a DRAFT or unsigned PENDING_REVIEW proposal (M6-005).
//
// Cancel state machine rules (state-machine §4):
//   - DRAFT → CANCELLED: always allowed for maker.
//   - PENDING_REVIEW with reviewer_id IS NULL → CANCELLED: allowed for maker.
//   - PENDING_REVIEW with reviewer_id NOT NULL → 403 (reviewer already engaged).
//   - PENDING_APPROVAL / APPROVED / REJECTED → 422 invalid transition.
//
// SoD: only the maker (proposal.maker_id == actorID) may cancel (DEC-017).
// CancelReason: min 20 chars (DB CHECK constraint from migration 000027).
// Audit event: "EIR.AMEND_CANCELLED" in-transaction.
func (s *DetectionService) CancelAmendment(ctx context.Context, req CancelAmendmentRequest) (*AmendmentProposal, error) {
	// 1. Validate cancel reason length (min 20 chars, mirrors DB CHECK).
	if len(req.CancelReason) < 20 { //nolint:mnd // 20 per DB CHECK constraint
		return nil, ErrEIRAmendmentCancelReasonShort()
	}

	// 2. Load proposal.
	proposal, err := s.amendRepo.GetByID(ctx, req.AmendmentID)
	if err != nil {
		return nil, fmt.Errorf("cancel: load proposal: %w", err)
	}
	if proposal == nil {
		return nil, ErrEIRAmendNotFound(req.AmendmentID.String())
	}

	// 3. SoD check: only maker may cancel.
	if proposal.MakerID == nil || *proposal.MakerID != req.ActorID {
		return nil, ErrEIRAmendmentCancelForbidden(
			"hanya maker yang dapat membatalkan proposal (SoD enforcement)")
	}

	// 4. State machine check.
	switch proposal.Status {
	case AmendStatusDraft:
		// always allowed
	case AmendStatusPendingReview:
		// allowed only if reviewer not yet assigned (no signature)
		if proposal.ReviewerID != nil {
			return nil, ErrEIRAmendmentCancelForbidden(
				"proposal sudah di-pick-up oleh reviewer — tidak bisa dibatalkan (status PENDING_REVIEW, reviewer sudah assign)")
		}
	case AmendStatusPendingApproval:
		return nil, ErrEIRAmendInvalidTransition(string(proposal.Status), string(AmendStatusCancelled))
	case AmendStatusApproved, AmendStatusRejected, AmendStatusCancelled:
		return nil, ErrEIRAmendInvalidTransition(string(proposal.Status), string(AmendStatusCancelled))
	default:
		return nil, ErrEIRAmendInvalidTransition(string(proposal.Status), string(AmendStatusCancelled))
	}

	// 5. Persist in tx.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("cancel: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := s.amendRepo.Cancel(ctx, tx, req.AmendmentID, req.CancelReason, req.ActorID); err != nil {
		return nil, fmt.Errorf("cancel: update proposal: %w", err)
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: req.ActorID,
		Action:      "EIR.AMEND_CANCELLED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    req.AmendmentID,
		BeforeJSON: map[string]interface{}{
			"status": string(proposal.Status),
		},
		AfterJSON: map[string]interface{}{
			"status":        string(AmendStatusCancelled),
			"cancel_reason": req.CancelReason,
			"cancelled_by":  req.ActorID.String(),
		},
		TenantID: req.TenantID,
	}); err != nil {
		return nil, fmt.Errorf("cancel: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("cancel: commit: %w", err)
	}

	// Return updated proposal.
	now := time.Now().UTC()
	proposal.Status = AmendStatusCancelled
	proposal.CancelledAt = &now
	proposal.CancelReason = &req.CancelReason
	proposal.CancelledBy = &req.ActorID

	s.logger.Info("eir amendment cancelled",
		slog.String("proposal_id", req.AmendmentID.String()),
		slog.String("cancelled_by", req.ActorID.String()),
	)
	return proposal, nil
}

// UpdateCashflows patches the revised cashflows on a DRAFT amendment proposal.
// Only callable while proposal is in DRAFT status. Only maker may update.
// Does not re-run NR solver — solver runs at Review step.
func (s *DetectionService) UpdateCashflows(ctx context.Context, req UpdateCashflowsRequest) (*AmendmentProposal, error) {
	// 1. Load proposal.
	proposal, err := s.amendRepo.GetByID(ctx, req.AmendmentID)
	if err != nil {
		return nil, fmt.Errorf("update cashflows: load proposal: %w", err)
	}
	if proposal == nil {
		return nil, ErrEIRAmendNotFound(req.AmendmentID.String())
	}

	// 2. State check: DRAFT only.
	if proposal.Status != AmendStatusDraft {
		return nil, ErrEIRAmendInvalidTransition(string(proposal.Status), "UPDATE_CASHFLOWS")
	}

	// 3. SoD: only maker may update cashflows.
	if proposal.MakerID == nil || *proposal.MakerID != req.ActorID {
		return nil, ErrEIRAmendmentCancelForbidden(
			"hanya maker yang dapat mengubah cashflow (SoD enforcement)")
	}

	// 4. Validate cashflows.
	if len(req.RevisedCashflows) == 0 {
		return nil, ErrEIRCashflowInvalid("revised cashflows tidak boleh kosong")
	}
	if !req.RevisedCashflows[0].AmountIDR.IsNegative() {
		return nil, ErrEIRCashflowSignMismatch()
	}

	cashflowJSON, err := marshalCashflows(req.RevisedCashflows)
	if err != nil {
		return nil, fmt.Errorf("update cashflows: marshal: %w", err)
	}

	// 5. Persist in tx (only modifikasi_terms_json + audit cols update).
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("update cashflows: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	q := `UPDATE ecl.eir_reestimation_log
		SET modifikasi_terms_json = $1,
		    updated_by            = $2,
		    updated_at            = now(),
		    row_version           = row_version + 1
		WHERE id = $3 AND deleted_at IS NULL`
	if _, err := tx.ExecContext(ctx, q, cashflowJSON, req.ActorID, req.AmendmentID); err != nil {
		return nil, fmt.Errorf("update cashflows: exec: %w", err)
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: req.ActorID,
		Action:      "EIR.AMEND_CASHFLOWS_UPDATED",
		EntityType:  "ecl.eir_reestimation_log",
		EntityID:    req.AmendmentID,
		AfterJSON: map[string]interface{}{
			"cashflow_count": len(req.RevisedCashflows),
			"updated_by":     req.ActorID.String(),
		},
		TenantID: req.TenantID,
	}); err != nil {
		return nil, fmt.Errorf("update cashflows: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("update cashflows: commit: %w", err)
	}

	proposal.RevisedCashflowJSON = cashflowJSON
	return proposal, nil
}

// isEIRApplicableForDetection returns true if the instrument is eligible for EIR amendment detection.
// Same logic as AmendmentService.isEIRApplicable but exported for reuse.
func isEIRApplicableForDetection(inst *InstrumenForEIR) bool {
	if inst == nil || inst.DeletedAt != nil {
		return false
	}
	if !inst.EIRMethodFlag {
		return false
	}
	switch inst.KlasifikasiPsak71 {
	case "AC", "FVOCI":
		return true
	default:
		return false
	}
}
