package mappingjurnal

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for MAPPING_JURNAL.
// BeforeCommit runs inside the workflow transaction to keep
// mst.mapping_jurnal_header.workflow_status in sync atomically.
// On APPROVE it also enforces:
//   - sum(DEBIT multiplier) == sum(KREDIT multiplier) ±0.0001
//   - All referenced CoA rows must have workflow_status='APPROVED'
type WorkflowHook struct {
	svc  *Service
	repo Repository
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service, repo Repository) *WorkflowHook {
	return &WorkflowHook{svc: svc, repo: repo}
}

// Ensure WorkflowHook satisfies workflow.EntityHook at compile time.
var _ workflow.EntityHook = (*WorkflowHook)(nil)

// BeforeCommit syncs workflow_status on mst.mapping_jurnal_header inside the workflow tx.
// On APPROVE it validates debit=credit invariant and CoA approval status before committing.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	wfStatus := mapWorkflowState(string(evt.NewState))

	// On APPROVE: enforce debit=credit invariant + CoA APPROVED check before commit.
	if wfStatus == WorkflowStatusApproved {
		if err := h.svc.validateApproveInvariants(ctx, evt.EntityID); err != nil {
			return fmt.Errorf("mapping_jurnal.WorkflowHook.BeforeCommit: invariant check: %w", err)
		}
	}

	if err := h.repo.UpdateWorkflowStatus(ctx, tx, evt.EntityID, wfStatus, evt.ActorID); err != nil {
		return fmt.Errorf("mapping_jurnal.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
