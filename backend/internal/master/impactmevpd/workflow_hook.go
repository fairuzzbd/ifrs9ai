package impactmevpd

import (
	"context"
	"strings"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for mst.impact_mev_pd.
// It syncs the workflow_status column after each transition via SyncWorkflowStatus.
// Called post-commit by workflow.Service; non-fatal on failure.
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook creates a WorkflowHook bound to the given service.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

var _ workflow.EntityHook = (*WorkflowHook)(nil)

// OnTransition is called by workflow.Service after a successful state transition.
func (h *WorkflowHook) OnTransition(ctx context.Context, event workflow.HookEvent) error {
	return h.svc.SyncWorkflowStatus(ctx, event.EntityID, event.NewState, strings.ToUpper(event.Action))
}
