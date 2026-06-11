// Service layer for the look-through ECL engine.
//
// Two services:
//
//  1. CompositionService — 6-eyes fund composition workflow:
//     Submit (ROLE-AKUN) → Review (ROLE-RISK) → Approve (ROLE-ALCO, MFA wajib) / Reject.
//     Amendment: Approve atomically supersedes previous APPROVED_ACTIVE composition.
//     SoD: maker_id ≠ reviewer_id ≠ approver_id enforced server-side (DEC-017).
//
//  2. LookthroughService — ECL computation per instrument:
//     Compute (single), BulkCompute (all REKSADANA, max 10_000), Preview (DataTable).
//     Semaphore-limited concurrency: 16 goroutines.
//
// Precision: all IDR/PD/LGD arithmetic uses shopspring/decimal (DEC-016 — never float64).
// Audit-in-tx: EVERY mutation writes to aud.audit_log in the same DB transaction (DEC-018).
// auditWriter must NOT be nil — constructor panics if nil (per LPS M3 pattern).
//
// References:
//   - FSD-APP-C §3.4 (Look-through ECL)
//   - SoW §4.4
//   - DEC-010, DEC-015, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022
//   - docs/state-machines/p4-m4-lookthrough.md
//   - docs/stories/phase-4/M4-look-through-reksadana.md (28 AC)
package lookthrough

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Audit writer types ───────────────────────────────────────────────────────

// AuditWriterIface is the minimal interface the service needs from the audit package.
// Tests inject mockAuditWriter; production injects AuditWriterAdapter.
// Constructor panics if nil (DEC-018 enforcement — no nil-guard).
type AuditWriterIface interface {
	Write(ctx context.Context, tx *sql.Tx, event AuditEvent) error
}

// AuditEvent is the audit event passed to the audit writer.
type AuditEvent struct {
	ActorUserID uuid.UUID
	ActorRole   string
	Action      string
	EntityType  string
	EntityID    uuid.UUID
	BeforeJSON  interface{}
	AfterJSON   interface{}
	TenantID    string
}

// AuditWriterAdapter bridges *audit.Writer → AuditWriterIface.
// Wire in main.go: lookthrough.NewAuditWriterAdapter(audit.NewWriter(db)).
type AuditWriterAdapter struct {
	w *audit.Writer
}

// NewAuditWriterAdapter creates an AuditWriterIface adapter from *audit.Writer.
func NewAuditWriterAdapter(w *audit.Writer) *AuditWriterAdapter {
	return &AuditWriterAdapter{w: w}
}

// Write implements AuditWriterIface. Writes audit event within the same transaction (DEC-018).
func (a *AuditWriterAdapter) Write(ctx context.Context, tx *sql.Tx, evt AuditEvent) error {
	return a.w.WithTx(tx).Write(ctx, audit.Event{
		Action:      evt.Action,
		EntityType:  evt.EntityType,
		EntityID:    evt.EntityID,
		Before:      evt.BeforeJSON,
		After:       evt.AfterJSON,
		ActorUserID: evt.ActorUserID.String(),
		ActorRole:   evt.ActorRole,
	})
}

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	// maxBulkReksadana is the hard limit for BulkCompute (ErrBulkTooLarge beyond).
	maxBulkReksadana = 10_000
	// bulkSemaphoreSize limits concurrent goroutines in BulkCompute.
	bulkSemaphoreSize = 16
	// compositionEntityType is the entity type string for audit log entries.
	compositionEntityType = "FUND_COMPOSITION"
	// lookthroughEntityType is the entity type for ECL compute audit entries.
	lookthroughEntityType = "LOOKTHROUGH_ECL"
	// defaultTenantID is the Phase 1 single-tenant ID (DEC-023).
	defaultTenantID = "TUGURE"
)

// rollbackTx attempts rollback; logs warn on unexpected error.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "lookthrough: tx rollback failed", "error", err)
	}
}

// ─── CompositionService ───────────────────────────────────────────────────────

// CompositionService manages the 6-eyes fund composition workflow.
// Safe for concurrent use.
type CompositionService struct {
	db          *sql.DB
	compRepo    FundCompositionRepo
	auditWriter AuditWriterIface
	logger      *slog.Logger
}

// NewCompositionService creates a CompositionService.
// Panics if db, compRepo, or auditWriter are nil (DEC-018 enforcement).
func NewCompositionService(
	db *sql.DB,
	compRepo FundCompositionRepo,
	auditWriter AuditWriterIface,
	logger *slog.Logger,
) *CompositionService {
	if db == nil {
		panic("lookthrough: CompositionService requires non-nil db")
	}
	if compRepo == nil {
		panic("lookthrough: CompositionService requires non-nil compRepo")
	}
	if auditWriter == nil {
		panic("lookthrough: auditWriter must not be nil — audit-in-tx is mandatory per DEC-018. Pass lookthrough.NewAuditWriterAdapter(audit.NewWriter(db)).")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &CompositionService{
		db:          db,
		compRepo:    compRepo,
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// Submit creates a new fund composition in PENDING_REVIEW state (ROLE-AKUN).
// Validates:
//   - instrumenID is REKSADANA type
//   - all asset class values are valid enum members
//   - Σ weight_pct = 100% ± 0.01%
//   - no duplicate asset_class within the same composition
//
// Writes audit FUND_COMPOSITION.SUBMIT within the same DB transaction.
// Returns the full CompositionGroup (header + details).
//
// AC: APP-C-LKT-002-AC01..AC07.
func (s *CompositionService) Submit(ctx context.Context, req SubmitCompositionRequest, actorID uuid.UUID, actorRole string) (*CompositionGroup, error) {
	// Validate instrumen is REKSADANA.
	tipe, _, _, err := s.compRepo.GetInstrumenTipeAndKlasifikasi(ctx, req.InstrumenID)
	if err != nil {
		return nil, err
	}
	if tipe != "REKSADANA" {
		return nil, ErrInstrumenNotReksadana(req.InstrumenID.String(), tipe)
	}

	// Validate asset class enum + no duplicates.
	seenAC := make(map[AssetClass]bool)
	weightPcts := make([]decimal.Decimal, len(req.Lines))
	for i, line := range req.Lines {
		if !line.AssetClass.IsValid() {
			return nil, ErrAssetClassUnknown(string(line.AssetClass), i)
		}
		if seenAC[line.AssetClass] {
			return nil, errLookthrough(CodeLookthroughAssetClassUnknown,
				fmt.Sprintf("Duplikat asset_class %s pada lines[%d]. Setiap asset class hanya boleh muncul sekali.", line.AssetClass, i))
		}
		seenAC[line.AssetClass] = true
		weightPcts[i] = line.WeightPct
	}

	// Validate weight sum = 100% ± 0.01%.
	if err := ValidateWeightSumFromPcts(weightPcts); err != nil {
		return nil, err
	}

	// Build domain objects.
	compositionID := uuid.New()
	now := time.Now().UTC()

	// effective_to: use '9999-12-31' (open-ended) for new compositions.
	effectiveTo := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	header := &FundComposition{
		ID:             compositionID,
		InstrumenID:    req.InstrumenID,
		EffectiveFrom:  req.EffectiveFrom,
		EffectiveTo:    effectiveTo,
		WorkflowStatus: WorkflowStatusPendingReview,
		MakerID:        actorID,
		SourceDocID:    req.SourceDocID,
		CreatedAt:      now,
		CreatedBy:      actorID,
		UpdatedAt:      now,
		UpdatedBy:      actorID,
		RowVersion:     1,
		TenantID:       defaultTenantID,
	}

	details := make([]FundCompositionDetail, len(req.Lines))
	for i, line := range req.Lines {
		details[i] = FundCompositionDetail{
			ID:                uuid.New(),
			FundCompositionID: compositionID,
			AssetClass:        line.AssetClass,
			WeightPct:         line.WeightPct,
			Position:          line.Position,
			CreatedAt:         now,
			CreatedBy:         actorID,
			UpdatedAt:         now,
			UpdatedBy:         actorID,
			RowVersion:        1,
			TenantID:          defaultTenantID,
		}
	}

	// Atomic: create composition + audit in same tx.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lookthrough submit begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.compRepo.Create(ctx, tx, header, details); err != nil {
		return nil, fmt.Errorf("lookthrough submit create: %w", err)
	}

	// Audit-in-tx (DEC-018): failure aborts tx.
	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "FUND_COMPOSITION.SUBMIT",
		EntityType:  compositionEntityType,
		EntityID:    compositionID,
		BeforeJSON:  nil,
		AfterJSON: map[string]interface{}{
			"instrumen_id":    req.InstrumenID.String(),
			"effective_from":  fmtDate(req.EffectiveFrom),
			"line_count":      len(req.Lines),
			"is_amendment":    req.IsAmendment,
			"workflow_status": string(WorkflowStatusPendingReview),
		},
		TenantID: defaultTenantID,
	}); err != nil {
		return nil, fmt.Errorf("lookthrough submit audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lookthrough submit commit: %w", err)
	}

	return &CompositionGroup{
		Header:  *header,
		Details: details,
		IsAmendment: req.IsAmendment,
		SupersedesCompositionID: req.SupersedesCompositionID,
	}, nil
}

// Review transitions composition from PENDING_REVIEW → PENDING_APPROVAL (ROLE-RISK).
// SoD check: reviewer_id ≠ maker_id.
// Writes FUND_COMPOSITION.REVIEW audit within same tx.
//
// AC: APP-C-LKT-002-AC08..AC12.
func (s *CompositionService) Review(ctx context.Context, req WorkflowActionRequest) (*FundComposition, error) {
	comp, err := s.compRepo.GetByID(ctx, req.CompositionID)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return nil, errLookthrough("NOT_FOUND", "Fund composition "+req.CompositionID.String()+" tidak ditemukan.")
	}

	// Validate transition.
	if !comp.WorkflowStatus.CanTransitionTo(WorkflowStatusPendingApproval) {
		return nil, ErrCompositionInvalidTransition(string(comp.WorkflowStatus), "REVIEW")
	}

	// SoD: reviewer ≠ maker.
	if comp.MakerID == req.ActorID {
		return nil, ErrCompositionSoDViolation("ROLE-RISK", req.CompositionID.String())
	}

	now := time.Now().UTC()
	sigHash := ComputeReviewSignatureHash(req.ActorID, req.CompositionID, now, req.Comment)
	comment := req.Comment

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lookthrough review begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.compRepo.UpdateWorkflowStatus(ctx, tx,
		req.CompositionID, WorkflowStatusPendingApproval,
		&req.ActorID, &now, sigHash, &comment,
		nil, nil, nil, nil, // approver fields unchanged
		nil, // rejectReason
		req.ActorID,
	); err != nil {
		return nil, err
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: req.ActorID,
		ActorRole:   req.ActorRole,
		Action:      "FUND_COMPOSITION.REVIEW",
		EntityType:  compositionEntityType,
		EntityID:    req.CompositionID,
		BeforeJSON: map[string]interface{}{
			"workflow_status": string(comp.WorkflowStatus),
		},
		AfterJSON: map[string]interface{}{
			"workflow_status":    string(WorkflowStatusPendingApproval),
			"reviewer_id":        req.ActorID.String(),
			"signed_at_review":   now.Format(time.RFC3339Nano),
			"signature_method":   req.SignatureMethod,
		},
		TenantID: defaultTenantID,
	}); err != nil {
		return nil, fmt.Errorf("lookthrough review audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lookthrough review commit: %w", err)
	}

	comp.WorkflowStatus = WorkflowStatusPendingApproval
	comp.ReviewerID = &req.ActorID
	comp.SignedAtReview = &now
	comp.SignatureHashReview = sigHash
	comp.CommentReview = &comment
	return comp, nil
}

// Approve transitions composition from PENDING_APPROVAL → APPROVED_ACTIVE (ROLE-ALCO, MFA wajib DEC-026).
// SoD checks: approver ≠ maker AND approver ≠ reviewer.
// If composition is an amendment (SupersedesCompositionID set), atomically supersedes old.
// Writes FUND_COMPOSITION.APPROVE audit within same tx.
//
// AC: APP-C-LKT-002-AC13..AC18.
func (s *CompositionService) Approve(ctx context.Context, req WorkflowActionRequest, supersedesID *uuid.UUID) (*FundComposition, error) {
	comp, err := s.compRepo.GetByID(ctx, req.CompositionID)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return nil, errLookthrough("NOT_FOUND", "Fund composition "+req.CompositionID.String()+" tidak ditemukan.")
	}

	// Validate transition.
	if !comp.WorkflowStatus.CanTransitionTo(WorkflowStatusApprovedActive) {
		return nil, ErrCompositionInvalidTransition(string(comp.WorkflowStatus), "APPROVE")
	}

	// SoD: approver ≠ maker.
	if comp.MakerID == req.ActorID {
		return nil, ErrCompositionSoDViolation("ROLE-ALCO (approver ≠ maker)", req.CompositionID.String())
	}
	// SoD: approver ≠ reviewer.
	if comp.ReviewerID != nil && *comp.ReviewerID == req.ActorID {
		return nil, ErrCompositionSoDViolation("ROLE-ALCO (approver ≠ reviewer)", req.CompositionID.String())
	}

	now := time.Now().UTC()
	sigHash := ComputeApproveSignatureHash(req.ActorID, req.CompositionID, now, req.Comment)
	comment := req.Comment

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lookthrough approve begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	// If amendment: supersede old composition first (atomic in same tx).
	if supersedesID != nil {
		if err := s.compRepo.SupersedeOld(ctx, tx, *supersedesID, comp.EffectiveFrom, req.ActorID); err != nil {
			return nil, fmt.Errorf("lookthrough approve supersede old: %w", err)
		}
	}

	if err := s.compRepo.UpdateWorkflowStatus(ctx, tx,
		req.CompositionID, WorkflowStatusApprovedActive,
		nil, nil, nil, nil, // reviewer fields unchanged
		&req.ActorID, &now, sigHash, &comment,
		nil, // rejectReason
		req.ActorID,
	); err != nil {
		return nil, err
	}

	afterData := map[string]interface{}{
		"workflow_status":     string(WorkflowStatusApprovedActive),
		"approver_id":         req.ActorID.String(),
		"signed_at_approve":   now.Format(time.RFC3339Nano),
		"signature_method":    req.SignatureMethod,
	}
	if supersedesID != nil {
		afterData["supersedes_composition_id"] = supersedesID.String()
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: req.ActorID,
		ActorRole:   req.ActorRole,
		Action:      "FUND_COMPOSITION.APPROVE",
		EntityType:  compositionEntityType,
		EntityID:    req.CompositionID,
		BeforeJSON: map[string]interface{}{
			"workflow_status": string(comp.WorkflowStatus),
		},
		AfterJSON: afterData,
		TenantID:  defaultTenantID,
	}); err != nil {
		return nil, fmt.Errorf("lookthrough approve audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lookthrough approve commit: %w", err)
	}

	comp.WorkflowStatus = WorkflowStatusApprovedActive
	comp.ApproverID = &req.ActorID
	comp.SignedAtApprove = &now
	comp.SignatureHashApprove = sigHash
	comp.CommentApprove = &comment
	return comp, nil
}

// Reject transitions composition from PENDING_REVIEW or PENDING_APPROVAL → REJECTED.
// SoD check: rejector ≠ maker (reviewer/approver should not be the maker).
// Writes FUND_COMPOSITION.REJECT audit within same tx.
//
// AC: APP-C-LKT-002-AC19..AC22.
func (s *CompositionService) Reject(ctx context.Context, req WorkflowActionRequest) (*FundComposition, error) {
	comp, err := s.compRepo.GetByID(ctx, req.CompositionID)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return nil, errLookthrough("NOT_FOUND", "Fund composition "+req.CompositionID.String()+" tidak ditemukan.")
	}

	if !comp.WorkflowStatus.CanTransitionTo(WorkflowStatusRejected) {
		return nil, ErrCompositionInvalidTransition(string(comp.WorkflowStatus), "REJECT")
	}

	// SoD: rejector ≠ maker.
	if comp.MakerID == req.ActorID {
		return nil, ErrCompositionSoDViolation("Rejector", req.CompositionID.String())
	}

	comment := req.Comment

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lookthrough reject begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.compRepo.UpdateWorkflowStatus(ctx, tx,
		req.CompositionID, WorkflowStatusRejected,
		nil, nil, nil, nil, // reviewer fields unchanged
		nil, nil, nil, nil, // approver fields unchanged
		&comment, // rejectReason
		req.ActorID,
	); err != nil {
		return nil, err
	}

	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: req.ActorID,
		ActorRole:   req.ActorRole,
		Action:      "FUND_COMPOSITION.REJECT",
		EntityType:  compositionEntityType,
		EntityID:    req.CompositionID,
		BeforeJSON: map[string]interface{}{
			"workflow_status": string(comp.WorkflowStatus),
		},
		AfterJSON: map[string]interface{}{
			"workflow_status": string(WorkflowStatusRejected),
			"reject_reason":   comment,
		},
		TenantID: defaultTenantID,
	}); err != nil {
		return nil, fmt.Errorf("lookthrough reject audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("lookthrough reject commit: %w", err)
	}

	comp.WorkflowStatus = WorkflowStatusRejected
	comp.RejectReason = &comment
	return comp, nil
}

// GetCompositionGroup loads the header + details for a given composition ID.
func (s *CompositionService) GetCompositionGroup(ctx context.Context, compositionID uuid.UUID) (*CompositionGroup, error) {
	header, err := s.compRepo.GetByID(ctx, compositionID)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, errLookthrough("NOT_FOUND", "Fund composition "+compositionID.String()+" tidak ditemukan.")
	}
	details, err := s.compRepo.GetDetailsForComposition(ctx, compositionID)
	if err != nil {
		return nil, err
	}
	var total decimal.Decimal
	for _, d := range details {
		total = total.Add(d.WeightPct)
	}
	return &CompositionGroup{
		Header:         *header,
		Details:        details,
		TotalWeightPct: total,
	}, nil
}

// ListCompositions returns paginated composition headers for instrumenID.
func (s *CompositionService) ListCompositions(ctx context.Context, instrumenID uuid.UUID,
	filterStatus, cursor string, limit int, sortCol, sortDir string,
) ([]FundComposition, string, bool, error) {
	return s.compRepo.ListByInstrumen(ctx, instrumenID, filterStatus, cursor, limit, sortCol, sortDir)
}

// ─── LookthroughService ───────────────────────────────────────────────────────

// LookthroughService computes look-through ECL for REKSADANA instruments.
// Safe for concurrent use.
type LookthroughService struct {
	db          *sql.DB
	instRepo    ReksadanaInstrumenRepo
	compRepo    FundCompositionRepo
	pdlgdRepo   PDLGDClassRepo
	paramRepo   ScenarioParamRepo
	resultRepo  LookthroughResultRepo
	auditWriter AuditWriterIface
	metrics     MetricsRecorder
	logger      *slog.Logger
}

// NewLookthroughService creates a LookthroughService.
// Panics if db, any repo, or auditWriter are nil (DEC-018 enforcement).
func NewLookthroughService(
	db *sql.DB,
	instRepo ReksadanaInstrumenRepo,
	compRepo FundCompositionRepo,
	pdlgdRepo PDLGDClassRepo,
	paramRepo ScenarioParamRepo,
	resultRepo LookthroughResultRepo,
	auditWriter AuditWriterIface,
	metrics MetricsRecorder,
	logger *slog.Logger,
) *LookthroughService {
	if db == nil {
		panic("lookthrough: LookthroughService requires non-nil db")
	}
	if instRepo == nil {
		panic("lookthrough: LookthroughService requires non-nil instRepo")
	}
	if compRepo == nil {
		panic("lookthrough: LookthroughService requires non-nil compRepo")
	}
	if pdlgdRepo == nil {
		panic("lookthrough: LookthroughService requires non-nil pdlgdRepo")
	}
	if paramRepo == nil {
		panic("lookthrough: LookthroughService requires non-nil paramRepo")
	}
	if resultRepo == nil {
		panic("lookthrough: LookthroughService requires non-nil resultRepo")
	}
	if auditWriter == nil {
		panic("lookthrough: auditWriter must not be nil — audit-in-tx is mandatory per DEC-018.")
	}
	if metrics == nil {
		metrics = NoopMetrics()
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &LookthroughService{
		db:          db,
		instRepo:    instRepo,
		compRepo:    compRepo,
		pdlgdRepo:   pdlgdRepo,
		paramRepo:   paramRepo,
		resultRepo:  resultRepo,
		auditWriter: auditWriter,
		metrics:     metrics,
		logger:      logger,
	}
}

// Compute calculates look-through ECL for a single REKSADANA instrument.
// This is the core method; BulkCompute fans out here per instrument.
//
// Steps (SoW §4.4, FSD-APP-C §3.4, DEC-015):
//  1. Load instrument — validate REKSADANA, FVTPL skip, POCI defer.
//  2. Get NAB_IDR — ErrNABMissing if nil.
//  3. Get APPROVED_ACTIVE fund composition for instrument on evaluationDate.
//  4. Load PD/LGD per asset class (batch).
//  5. Load scenario weights + FL multipliers for periodeID.
//  6. For each asset class line: ComputeBreakdownLine (pure function, decimal math).
//  7. Sum TotalECLIDR = Σ breakdown ECLWeightedIDR.
//  8. Write to ecl.lookthrough_underlying within tx (with audit).
//  9. Return LookthroughResult.
//
// AC: APP-C-LKT-001-AC01..AC10.
func (s *LookthroughService) Compute(ctx context.Context, instrumenID, runID, periodeID uuid.UUID, evaluationDate time.Time) (*LookthroughResult, error) {
	// Step 1: Load instrument.
	inst, err := s.instRepo.GetByID(ctx, instrumenID)
	if err != nil {
		return nil, err
	}
	if inst == nil {
		return nil, errLookthrough("NOT_FOUND", "Instrumen "+instrumenID.String()+" tidak ditemukan atau bukan REKSADANA.")
	}

	// Step 1a: POCI defer (non-fatal in BulkCompute — caller must distinguish).
	if inst.POCIFlag {
		return nil, ErrPOCIDeferred(instrumenID.String())
	}

	// Step 1b: FVTPL skip — ECL = 0, not an error.
	if inst.IsFVTPL() {
		result := &LookthroughResult{
			InstrumenID:       instrumenID,
			InstrumenNama:     inst.NamaInstrumen,
			KlasifikasiPsak71: inst.KlasifikasiPsak71,
			TotalECLIDR:       decimal.Zero,
			FVTPLSkipped:      true,
			Warning:           "FVTPL instrumen: ECL = 0 (skip per OQ-M4-3). Klasifikasi FVTPL tidak memerlukan ECL provisioning.",
		}
		return result, nil
	}

	// Step 2: NAB_IDR.
	if inst.NominalNABIDR == nil {
		return nil, ErrNABMissing(instrumenID.String(), evaluationDate.Format("2006-01-02"))
	}
	nabIDR := *inst.NominalNABIDR

	// Step 3: Active fund composition.
	comp, err := s.compRepo.GetActiveForInstrumen(ctx, instrumenID, evaluationDate)
	if err != nil {
		return nil, err
	}
	if comp == nil {
		return nil, ErrFundCompositionMissing(instrumenID.String(), evaluationDate.Format("2006-01-02"))
	}
	details, err := s.compRepo.GetDetailsForComposition(ctx, comp.ID)
	if err != nil {
		return nil, err
	}

	// Defensive weight validation (data integrity guard).
	if err := ValidateWeightSum(details, comp.ID.String()); err != nil {
		return nil, err
	}

	// Step 4: Load PD/LGD for all asset classes in batch.
	assetClasses := make([]AssetClass, len(details))
	for i, d := range details {
		assetClasses[i] = d.AssetClass
	}
	pdlgdMap, err := s.pdlgdRepo.BulkGetPDLGDForAssetClasses(ctx, assetClasses, evaluationDate, defaultTenantID)
	if err != nil {
		return nil, err
	}

	// Step 5: Scenario weights + FL multipliers.
	weights, err := s.paramRepo.GetScenarioWeights(ctx, periodeID, defaultTenantID)
	if err != nil {
		return nil, err
	}
	fl, err := s.paramRepo.GetFLMultipliers(ctx, periodeID, defaultTenantID)
	if err != nil {
		return nil, err
	}

	// Step 6: Compute breakdown per asset class.
	breakdown := make([]LookthroughBreakdownLine, 0, len(details))
	for _, d := range details {
		pd, ok := pdlgdMap[d.AssetClass]
		if !ok {
			return nil, ErrPDLGDClassMissing(string(d.AssetClass), periodeID.String())
		}
		line := ComputeBreakdownLine(d.AssetClass, d.WeightPct, nabIDR, pd, fl, weights)
		breakdown = append(breakdown, line)
	}

	// Step 7: Sum TotalECLIDR = Σ ECLWeightedIDR.
	// Each ECLWeightedIDR is already truncated to 4dp; sum truncated again to 4dp.
	var totalECL decimal.Decimal
	for _, b := range breakdown {
		totalECL = totalECL.Add(b.ECLWeightedIDR)
	}
	totalECL = totalECL.Truncate(4)

	result := &LookthroughResult{
		InstrumenID:                  instrumenID,
		InstrumenNama:                inst.NamaInstrumen,
		KlasifikasiPsak71:            inst.KlasifikasiPsak71,
		NABIDR:                       nabIDR,
		FundCompositionID:            comp.ID,
		FundCompositionEffectiveFrom: comp.EffectiveFrom,
		TotalECLIDR:                  totalECL,
		Breakdown:                    breakdown,
	}

	// Step 8: Persist result within tx + audit.
	if runID != (uuid.UUID{}) {
		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return nil, fmt.Errorf("lookthrough compute begin tx: %w", txErr)
		}
		defer rollbackTx(ctx, tx, s.logger)

		if upsertErr := s.resultRepo.UpsertResult(ctx, tx, instrumenID, runID, *result, comp.ID, periodeID, evaluationDate, defaultTenantID); upsertErr != nil {
			return nil, fmt.Errorf("lookthrough compute upsert result: %w", upsertErr)
		}

		if auditErr := s.auditWriter.Write(ctx, tx, AuditEvent{
			ActorUserID: instrumenID, // system actor — use instrumenID as placeholder
			ActorRole:   "SYSTEM",
			Action:      "LOOKTHROUGH_ECL.COMPUTE",
			EntityType:  lookthroughEntityType,
			EntityID:    instrumenID,
			AfterJSON: map[string]interface{}{
				"run_id":          runID.String(),
				"periode_id":      periodeID.String(),
				"evaluation_date": evaluationDate.Format("2006-01-02"),
				"total_ecl_idr":   totalECL.StringFixed(4),
				"composition_id":  comp.ID.String(),
			},
			TenantID: defaultTenantID,
		}); auditErr != nil {
			return nil, fmt.Errorf("lookthrough compute audit write: %w", auditErr)
		}

		if commitErr := tx.Commit(); commitErr != nil {
			return nil, fmt.Errorf("lookthrough compute commit: %w", commitErr)
		}
	}

	return result, nil
}

// BulkComputeResult wraps the outcome for one instrument in BulkCompute.
type BulkComputeResult struct {
	InstrumenID uuid.UUID
	Result      *LookthroughResult
	Err         error
	// IsPOCI is true when the error is LOOKTHROUGH_POCI_DEFERRED (non-fatal skip).
	IsPOCI bool
	// IsFVTPLSkipped is true when result.FVTPLSkipped = true (ECL = 0, not error).
	IsFVTPLSkipped bool
}

// BulkCompute calculates look-through ECL for all active REKSADANA instruments.
// Non-fatal skips: POCI instruments are skipped (IsPOCI=true) and FVTPL (IsFVTPLSkipped=true).
// Hard limit: 10_000 instruments. Returns ErrBulkTooLarge if exceeded.
// Concurrency: semaphore-limited to 16 goroutines.
// SLA target: ≤ 2s for 500 instruments (state-machine doc §6.2).
//
// AC: APP-C-LKT-001-AC11..AC16.
func (s *LookthroughService) BulkCompute(ctx context.Context, runID, periodeID uuid.UUID, evaluationDate time.Time) ([]BulkComputeResult, error) {
	startTime := time.Now()

	instruments, err := s.instRepo.BulkListReksadanaForECL(ctx, defaultTenantID)
	if err != nil {
		return nil, fmt.Errorf("lookthrough bulk list: %w", err)
	}

	// Hard limit check.
	if len(instruments) > maxBulkReksadana {
		return nil, ErrBulkTooLarge(len(instruments))
	}

	s.metrics.RecordBulkInstrumentCount(len(instruments))

	results := make([]BulkComputeResult, len(instruments))
	sem := make(chan struct{}, bulkSemaphoreSize)
	var wg sync.WaitGroup

	for i, inst := range instruments {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, instrumen InstrumenReksadanaRow) {
			defer wg.Done()
			defer func() { <-sem }()

			r, computeErr := s.Compute(ctx, instrumen.ID, runID, periodeID, evaluationDate)
			br := BulkComputeResult{InstrumenID: instrumen.ID}
			if computeErr != nil {
				if isDomainCode(computeErr, CodeLookthroughPOCIDeferred) {
					br.IsPOCI = true
					br.Err = computeErr
				} else {
					br.Err = computeErr
				}
			} else {
				br.Result = r
				if r.FVTPLSkipped {
					br.IsFVTPLSkipped = true
				}
			}
			results[idx] = br
		}(i, inst)
	}
	wg.Wait()

	// Count errors (excluding non-fatal POCI).
	var errorCount int
	for _, r := range results {
		if r.Err != nil && !r.IsPOCI {
			errorCount++
		}
	}
	s.metrics.RecordBulkErrors(errorCount)
	s.metrics.RecordBulkDuration(time.Since(startTime).Seconds())

	return results, nil
}

// Preview returns a DataTable-style list of REKSADANA instruments with estimated ECL.
// Does NOT persist results. Uses current NAB_IDR and APPROVED_ACTIVE composition.
// Missing NAB or composition → warning in PreviewSummaryRow.
//
// AC: APP-C-LKT-001-AC17..AC20.
func (s *LookthroughService) Preview(ctx context.Context, periodeID uuid.UUID, evaluationDate time.Time,
	cursor string, limit int,
) ([]PreviewSummaryRow, string, bool, error) {
	instruments, err := s.instRepo.BulkListReksadanaForECL(ctx, defaultTenantID)
	if err != nil {
		return nil, "", false, err
	}

	// Apply cursor-based pagination over in-memory slice (instruments already ordered by id).
	// For preview, cursor is a simple index offset encoded as hex.
	startIdx := 0
	if cursor != "" {
		// cursor is index into instruments slice (opaque to caller).
		var idx int
		if _, scanErr := fmt.Sscanf(cursor, "%d", &idx); scanErr == nil && idx >= 0 && idx < len(instruments) {
			startIdx = idx
		}
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	end := startIdx + limit + 1
	if end > len(instruments) {
		end = len(instruments)
	}
	page := instruments[startIdx:end]

	hasMore := len(page) > limit
	if hasMore {
		page = page[:limit]
	}
	var nextCursor string
	if hasMore {
		nextCursor = fmt.Sprintf("%d", startIdx+limit)
	}

	rows := make([]PreviewSummaryRow, 0, len(page))
	for _, inst := range page {
		row := PreviewSummaryRow{
			InstrumenID:       inst.ID,
			InstrumenNama:     inst.NamaInstrumen,
			KlasifikasiPsak71: inst.KlasifikasiPsak71,
			IsPreview:         true,
		}

		if inst.NominalNABIDR != nil {
			row.NABIDRStr = inst.NominalNABIDR.StringFixed(4)
		}

		// Try to find composition.
		comp, compErr := s.compRepo.GetActiveForInstrumen(ctx, inst.ID, evaluationDate)
		if compErr != nil || comp == nil {
			row.Warnings = append(row.Warnings, PreviewWarning{
				Code:    CodeLookthroughFundCompositionMissing,
				Message: "Tidak ada fund composition APPROVED_ACTIVE per " + evaluationDate.Format("2006-01-02"),
			})
			rows = append(rows, row)
			continue
		}
		row.HasComposition = true
		row.FundCompositionID = &comp.ID
		row.FundCompositionEffectiveFrom = &comp.EffectiveFrom

		// FVTPL skip: ECL estimate = 0.
		if inst.IsFVTPL() {
			ecl := decimal.Zero.StringFixed(4)
			row.TotalECLEstimateIDRStr = &ecl
			row.Warnings = append(row.Warnings, PreviewWarning{
				Code:    "FVTPL_SKIP",
				Message: "FVTPL instrumen: ECL estimate = 0",
			})
			rows = append(rows, row)
			continue
		}

		if inst.NominalNABIDR == nil {
			row.Warnings = append(row.Warnings, PreviewWarning{
				Code:    CodeLookthroughNABMissing,
				Message: "NAB_IDR belum tersedia per " + evaluationDate.Format("2006-01-02"),
			})
			rows = append(rows, row)
			continue
		}

		// Full preview compute (no persist — pass zero runID to skip upsert).
		previewResult, computeErr := s.Compute(ctx, inst.ID, uuid.UUID{}, periodeID, evaluationDate)
		if computeErr != nil {
			if isDomainCode(computeErr, CodeLookthroughPOCIDeferred) {
				row.Warnings = append(row.Warnings, PreviewWarning{
					Code:    CodeLookthroughPOCIDeferred,
					Message: "POCI instrumen — ECL deferred ke Phase 5.",
				})
			} else {
				row.Warnings = append(row.Warnings, PreviewWarning{
					Code:    "COMPUTE_WARN",
					Message: computeErr.Error(),
				})
			}
		} else {
			ecl := previewResult.TotalECLIDR.StringFixed(4)
			row.TotalECLEstimateIDRStr = &ecl
		}
		rows = append(rows, row)
	}

	return rows, nextCursor, hasMore, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// isDomainCode checks if err is a DomainError with the given code.
func isDomainCode(err error, code string) bool {
	if err == nil {
		return false
	}
	type coder interface{ Code() domainerrors.Code }
	if c, ok := err.(coder); ok {
		return string(c.Code()) == code
	}
	return false
}
