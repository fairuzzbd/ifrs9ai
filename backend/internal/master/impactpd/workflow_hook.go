package impactpd

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for IMPACT_PD entities.
// BeforeCommit runs inside the workflow transaction to keep
// mst.impact_pd.workflow_status in sync atomically (pre-commit, tx-aware).
type WorkflowHook struct {
	svc  *Service
	repo Repository
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service, repo Repository) *WorkflowHook {
	return &WorkflowHook{svc: svc, repo: repo}
}

// BeforeCommit syncs workflow_status on mst.impact_pd inside the workflow tx.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatusTx(ctx, tx, evt.EntityID, wfStatus); err != nil {
		return fmt.Errorf("impactpd.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
