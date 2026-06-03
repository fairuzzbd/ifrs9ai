package lpscoverage

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for LPS_COVERAGE entities.
//
// Pattern: On every workflow state transition (submit/review/approve/approve2/reject),
// the workflow engine calls BeforeCommit inside the same transaction that updates
// sys.workflow_instance. This keeps mst.lps_coverage.workflow_status in sync
// atomically with the workflow state.
//
// LPS Coverage has NO cross-row sum invariant (unlike bobot_skenario),
// so BeforeCommit only syncs the status column — no additional validation needed.
type WorkflowHook struct {
	repo Repository
}

// NewWorkflowHook creates a WorkflowHook backed by the given repository.
func NewWorkflowHook(repo Repository) *WorkflowHook {
	return &WorkflowHook{repo: repo}
}

// BeforeCommit is called by the workflow service inside the active transaction
// before it commits the state transition. It syncs mst.lps_coverage.workflow_status
// to match the new workflow state.
//
// Returns an error to abort the transaction if the sync fails.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	wfStatus := mapWorkflowState(string(evt.NewState))
	if err := h.repo.UpdateWorkflowStatusTx(ctx, tx, evt.EntityID, wfStatus); err != nil {
		return fmt.Errorf("lpscoverage.WorkflowHook.BeforeCommit: %w", err)
	}
	return nil
}
