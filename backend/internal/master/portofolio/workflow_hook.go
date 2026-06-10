package portofolio

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for mst.portofolio.
// BeforeCommit runs inside the workflow transaction to keep
// mst.portofolio.workflow_status in sync atomically.
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

// BeforeCommit syncs workflow_status on mst.portofolio inside the workflow tx.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatus(ctx, tx, evt.EntityID, wfStatus, evt.ActorID); err != nil {
		return fmt.Errorf("portofolio.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
