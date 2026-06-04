package coa

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for mst.chart_of_accounts.
// It is called by the workflow service post-commit on every state transition,
// keeping workflow_status in sync with the generic workflow engine.
//
// Dispatch is non-fatal (warn on error) per workflow/service.go convention.
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

// Ensure WorkflowHook satisfies workflow.EntityHook at compile time.
var _ workflow.EntityHook = (*WorkflowHook)(nil)

// OnTransition is called by workflow.Service after a successful state transition.
// It syncs the entity-side workflow_status column.
func (h *WorkflowHook) OnTransition(ctx context.Context, evt workflow.HookEvent) error {
	entityID, err := uuid.Parse(evt.EntityID.String())
	if err != nil {
		return fmt.Errorf("coa.WorkflowHook.OnTransition: parse entity ID: %w", err)
	}
	return h.svc.SyncWorkflowStatus(ctx, entityID, evt.NewState, evt.Action)
}
