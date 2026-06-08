package lpscoverage

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for LPS_COVERAGE entities.
// BeforeCommit runs inside the workflow transaction to keep
// mst.lps_coverage.workflow_status in sync atomically.
type WorkflowHook struct {
	svc  *Service
	repo Repository
}

// NewWorkflowHook creates a WorkflowHook bound to the given service.
func NewWorkflowHook(svc *Service, repo Repository) *WorkflowHook {
	return &WorkflowHook{svc: svc, repo: repo}
}

// BeforeCommit syncs workflow_status on mst.lps_coverage inside the workflow tx.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatusTx(ctx, tx, evt.EntityID, wfStatus); err != nil {
		return fmt.Errorf("lpscoverage.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
