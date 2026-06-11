// Package lookthrough — workflow engine hook for fund composition entity.
//
// Registered at startup so the workflow engine can validate composition transitions.
// The hook is advisory for state machine consistency but CompositionService handles
// the actual state transitions and audit writing (DEC-018).
package lookthrough

import (
	"context"
	"database/sql"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// CompositionWorkflowHook implements workflow.EntityHook for mst.fund_composition.
// Registered under entity type "LOOKTHROUGH_COMPOSITION".
//
// 6-eyes workflow:
//
//	Submit       → PENDING_REVIEW    (maker: ROLE-AKUN)
//	Review       → PENDING_APPROVAL  (reviewer: ROLE-RISK, SoD: reviewer ≠ maker)
//	Approve      → APPROVED_ACTIVE   (approver: ROLE-ALCO, SoD: approver ≠ maker ≠ reviewer)
//	Reject       → REJECTED          (reviewer or approver, SoD: actor ≠ maker)
//	Supersede    → SUPERSEDED        (system-only, on amendment approve)
//
// BeforeCommit validates state machine transitions; refuses terminal transitions.
// SoD is also enforced by CompositionService.Approve / Review — the hook is a secondary guard.
type CompositionWorkflowHook struct {
	compRepo FundCompositionRepo
}

// NewCompositionWorkflowHook creates a hook with the composition repository injected.
func NewCompositionWorkflowHook(repo FundCompositionRepo) *CompositionWorkflowHook {
	return &CompositionWorkflowHook{compRepo: repo}
}

// BeforeCommit is invoked by the workflow engine inside the transition transaction.
// tx may be nil for in-memory tests (workflow.EntityHook contract).
//
// Validations:
//   - Loading the existing composition confirms it exists.
//   - Terminal states (APPROVED_ACTIVE, SUPERSEDED, REJECTED): refuse any further transition.
//   - APPROVE: SoD check approver ≠ maker.
//   - REVIEW:  SoD check reviewer ≠ maker.
func (hook *CompositionWorkflowHook) BeforeCommit(ctx context.Context, _ *sql.Tx, evt workflow.HookEvent) error {
	if hook.compRepo == nil {
		return nil
	}

	existing, err := hook.compRepo.GetByID(ctx, evt.EntityID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrCompositionInvalidTransition("NOT_FOUND", string(evt.NewState))
	}

	// Refuse transition if already terminal.
	if existing.WorkflowStatus.IsTerminal() {
		return ErrCompositionInvalidTransition(string(existing.WorkflowStatus), string(evt.NewState))
	}

	// Validate the transition is legal per state machine.
	nextStatus := WorkflowStatus(evt.NewState)
	if !existing.WorkflowStatus.CanTransitionTo(nextStatus) {
		return ErrCompositionInvalidTransition(string(existing.WorkflowStatus), string(evt.NewState))
	}

	// For APPROVE transitions: validate SoD (approver ≠ maker).
	if nextStatus == WorkflowStatusApprovedActive {
		if evt.ActorID == existing.MakerID {
			return ErrCompositionSoDViolation("ROLE-ALCO", existing.ID.String())
		}
		if existing.ReviewerID != nil && evt.ActorID == *existing.ReviewerID {
			return ErrCompositionSoDViolation("ROLE-ALCO (approver ≠ reviewer)", existing.ID.String())
		}
	}

	// For REVIEW transitions: validate SoD (reviewer ≠ maker).
	if nextStatus == WorkflowStatusPendingApproval {
		if evt.ActorID == existing.MakerID {
			return ErrCompositionSoDViolation("ROLE-RISK", existing.ID.String())
		}
	}

	return nil
}

// EntityType returns the entity type string for hook registration.
func (hook *CompositionWorkflowHook) EntityType() string {
	return "LOOKTHROUGH_COMPOSITION"
}
