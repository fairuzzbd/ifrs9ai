// Package counterparty — EntityHook for generic workflow engine.
//
// BeforeCommit is called inside the workflow transaction (AFTER state+signature+audit
// are written but BEFORE commit). The hook syncs workflow_status back to
// mst.counterparty in the same transaction, making the status update atomic
// with the workflow transition.
//
// Pattern: same as bobotskenario.WorkflowHook. Registered in cmd/api/main.go
// via workflow.Service.RegisterEntityHook("COUNTERPARTY", hook).
package counterparty

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for COUNTERPARTY entities.
// Called pre-commit by workflow.Service to sync mst.counterparty.workflow_status
// inside the workflow transaction.
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

// BeforeCommit syncs workflow_status on mst.counterparty inside the workflow tx.
// tx may be nil when using InMemoryRepository in unit tests — skip DB ops in that case.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	newStatus := mapWorkflowState(string(evt.NewState))

	if tx != nil {
		if err := h.repo.UpdateWorkflowStatus(ctx, tx, evt.EntityID, newStatus, evt.ActorID); err != nil {
			return fmt.Errorf("counterparty.WorkflowHook.BeforeCommit: update workflow_status: %w", err)
		}
	}
	return nil
}
