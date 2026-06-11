// Package lps — workflow engine hook for LPS exclusion override entity.
//
// Registered at startup so the workflow engine calls BeforeCommit before
// any ecl.lps_exclusion_override mutation is committed (same-tx pattern DEC-018).
//
// Hook is advisory-only for LPS (4-eyes, not 6-eyes) — it validates state
// machine transitions and SoD but does not independently persist anything.
// Audit writing is handled by OverrideService, not the hook.
package lps

import (
	"context"
	"database/sql"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// OverrideWorkflowHook implements workflow.EntityHook for LPS exclusion overrides.
// Registered under entity type "LPS_EXCLUSION_OVERRIDE".
//
// Lifecycle:
//
//	Submit  → PENDING_APPROVAL   (maker: ROLE-RISK)
//	Approve → APPROVED_ACTIVE    (approver: ROLE-ALCO, SoD: approver ≠ maker)
//	Reject  → REJECTED           (actor: ROLE-ALCO or ROLE-RISK)
//
// On re-entrance: if override is already APPROVED_ACTIVE or REJECTED (terminal),
// BeforeCommit returns ErrLPSOverrideInvalidTransition.
type OverrideWorkflowHook struct {
	overrideRepo OverrideRepoIface
}

// NewOverrideWorkflowHook creates a hook with the override repository injected.
func NewOverrideWorkflowHook(repo OverrideRepoIface) *OverrideWorkflowHook {
	return &OverrideWorkflowHook{overrideRepo: repo}
}

// BeforeCommit is invoked by the workflow engine inside the transition transaction,
// AFTER state+signature+audit are written but BEFORE commit.
// tx may be nil for InMemory tests (workflow.EntityHook contract).
//
// Validations:
//   - Transition must be valid per state machine.
//   - APPROVE (NewState == "APPROVED_ACTIVE"): SoD must hold (approver ≠ maker).
//   - Terminal states (APPROVED_ACTIVE, REJECTED, EXPIRED): refuse any further transition.
func (hook *OverrideWorkflowHook) BeforeCommit(ctx context.Context, _ *sql.Tx, evt workflow.HookEvent) error {
	if hook.overrideRepo == nil {
		return nil
	}

	existing, err := hook.overrideRepo.GetByID(ctx, evt.EntityID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrLPSOverrideInstrumenNotFound(evt.EntityID.String())
	}

	// Refuse transition if already terminal.
	if existing.WorkflowStatus.IsTerminal() {
		return ErrLPSOverrideInvalidTransition(existing.WorkflowStatus.String(), string(evt.NewState))
	}

	// For APPROVE transitions: validate SoD.
	if string(evt.NewState) == string(WorkflowStatusApprovedActive) {
		if evt.ActorID == existing.MakerID {
			return ErrLPSOverrideSoDViolation()
		}
	}

	// For REJECT transitions: source must be PENDING_APPROVAL.
	if string(evt.NewState) == string(WorkflowStatusRejected) {
		if existing.WorkflowStatus != WorkflowStatusPendingApproval {
			return ErrLPSOverrideInvalidTransition(existing.WorkflowStatus.String(), string(evt.NewState))
		}
	}

	return nil
}

// EntityType returns the entity type string for hook registration.
func (hook *OverrideWorkflowHook) EntityType() string {
	return "LPS_EXCLUSION_OVERRIDE"
}
