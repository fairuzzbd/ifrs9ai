package mappingjurnal

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for MAPPING_JURNAL.
// After each workflow state transition, it calls service.SyncWorkflowStatus to keep
// mst.mapping_jurnal_header.workflow_status in sync with sys.workflow_instance.current_state.
// On APPROVE it also triggers the debit=credit multiplier invariant check + CoA approval check.
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

// Ensure WorkflowHook satisfies workflow.EntityHook at compile time.
var _ workflow.EntityHook = (*WorkflowHook)(nil)

// OnTransition is called by the workflow service after each successful state transition.
// entityID is the mst.mapping_jurnal_header.id (UUID).
// newState is the workflow engine state string (e.g. "APPROVED").
// action is the action that caused the transition (e.g. "APPROVE").
func (h *WorkflowHook) OnTransition(ctx context.Context, entityType string, entityID uuid.UUID, newState string, action string) error {
	if err := h.svc.SyncWorkflowStatus(ctx, entityID, newState, action); err != nil {
		return fmt.Errorf("mapping_jurnal workflow hook: %w", err)
	}
	return nil
}
