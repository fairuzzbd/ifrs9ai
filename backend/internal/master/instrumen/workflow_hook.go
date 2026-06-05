package instrumen

import (
	"context"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for INSTRUMEN.
// Called post-commit by workflow.Service to sync mst.instrumen.workflow_status.
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

// OnTransition syncs workflow_status on mst.instrumen after a workflow transition.
func (h *WorkflowHook) OnTransition(ctx context.Context, evt workflow.HookEvent) error {
	return h.svc.SyncWorkflowStatus(ctx, evt.EntityID, evt.NewState, evt.Action)
}
