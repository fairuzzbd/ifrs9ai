// Package counterparty — EntityHook for generic workflow engine.
//
// The workflow engine calls EntityHook.OnTransition after every state change.
// The hook syncs workflow_status back to mst.counterparty in a separate tx.
//
// Pattern: same as matauang.SyncWorkflowStatus. Registered in cmd/api/main.go
// via workflow.Service.RegisterHook("COUNTERPARTY", hook).
package counterparty

import (
	"context"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for COUNTERPARTY entities.
// Called post-commit by workflow.Service to sync mst.counterparty.workflow_status.
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

// OnTransition syncs workflow_status on mst.counterparty after a workflow
// transition has committed.
func (h *WorkflowHook) OnTransition(ctx context.Context, evt workflow.HookEvent) error {
	return h.svc.SyncWorkflowStatus(ctx, evt.EntityID, evt.NewState, evt.Action)
}
