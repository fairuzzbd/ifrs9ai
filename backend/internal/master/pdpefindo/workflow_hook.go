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
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatus(ctx, tx, evt.EntityID, wfStatus, evt.ActorID); err != nil {
		return fmt.Errorf("pdpefindo.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
