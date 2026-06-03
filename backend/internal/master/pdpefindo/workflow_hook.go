package pdpefindo

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// WorkflowHook implements the workflow.EntityHook interface for mst.pd_pefindo.
// It keeps workflow_status on the entity row in sync with sys.workflow_instance.
//
// Pattern: after each successful workflow transition, the workflow.Service calls
// OnTransition on all registered entity hooks. This hook updates
// mst.pd_pefindo.workflow_status so CRUD endpoints can filter by it without
// joining to sys.workflow_instance on every query.
//
// Registration in main.go:
//
//	pdPefindoHook := pdpefindo.NewWorkflowHook(pdPefindoSvc)
//	wfService.RegisterEntityHook("PD_PEFINDO", pdPefindoHook)
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook constructs a WorkflowHook backed by the pd_pefindo Service.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

// OnTransition is called by workflow.Service after each state transition commit.
// It syncs mst.pd_pefindo.workflow_status to match the new workflow state.
//
// The ctx passed here already contains the actor's JWT claims (propagated from
// the original HTTP request context), so requireActor() in service will succeed.
func (h *WorkflowHook) OnTransition(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	if err := h.svc.SyncWorkflowStatus(ctx, entityID, newState, action); err != nil {
		return fmt.Errorf("pdpefindo WorkflowHook.OnTransition: %w", err)
	}
	return nil
}
