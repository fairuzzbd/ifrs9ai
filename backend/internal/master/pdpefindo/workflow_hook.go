package pdpefindo

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for PD_PEFINDO entities.
// BeforeCommit runs inside the workflow transaction to keep
// mst.pd_pefindo.workflow_status in sync atomically.
type WorkflowHook struct {
	svc  *Service
	repo Repository
}

// NewWorkflowHook constructs a WorkflowHook bound to the given service + repo.
func NewWorkflowHook(svc *Service, repo Repository) *WorkflowHook {
	return &WorkflowHook{svc: svc, repo: repo}
}

// BeforeCommit syncs workflow_status on mst.pd_pefindo inside the workflow tx.
//
// F-006: when transitioning to APPROVED (approve2 final step), enforce the
// single-active invariant: no other APPROVED record for the same rating may
// overlap the period. If overlap is detected, BeforeCommit returns an error
// which causes the workflow transaction to rollback — the record stays in
// PENDING_APPROVAL_2 and the caller receives 422 PD_PERIOD_OVERLAP.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	// F-006: overlap guard on approve2 → APPROVED transition.
	if evt.NewState == workflow.StateApproved && h.svc != nil {
		entity, err := h.repo.GetByID(ctx, evt.EntityID, false)
		if err != nil {
			return fmt.Errorf("pdpefindo.WorkflowHook.BeforeCommit: load entity: %w", err)
		}
		if entity != nil {
			if overlapErr := h.svc.AssertNoApprovedOverlapForRating(
				ctx,
				entity.Rating,
				entity.PeriodeBerlakuDari,
				entity.PeriodeBerlakuSampai,
				evt.EntityID,
			); overlapErr != nil {
				return fmt.Errorf("pdpefindo.WorkflowHook.BeforeCommit: %w", overlapErr)
			}
		}
	}

	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatus(ctx, tx, evt.EntityID, wfStatus, evt.ActorID); err != nil {
		return fmt.Errorf("pdpefindo.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
