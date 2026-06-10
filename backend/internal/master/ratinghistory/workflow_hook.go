// Package ratinghistory — EntityHook for the workflow engine.
//
// BeforeCommit is called inside the workflow transaction (AFTER state+signature+audit
// are written but BEFORE commit). The hook:
//  1. Syncs mst.rating_history_counterparty.workflow_status atomically.
//  2. On APPROVED: runs SICR computation, closes the previous active rating,
//     and updates counterparty.rating_pefindo_current — all within the same tx.
//
// Registered in cmd/api/main.go via workflow.Service.RegisterEntityHook("RATING_HISTORY", hook).
package ratinghistory

import (
	"context"
	"database/sql"
	"fmt"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for RATING_HISTORY entities.
// On APPROVED transition the hook computes SICR + closes previous rating +
// updates counterparty.rating_pefindo_current cache — all in the workflow tx.
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

// BeforeCommit syncs workflow_status and, on APPROVED, executes SICR side-effects.
// tx may be nil when using InMemoryRepository in unit tests — skip DB ops in that case.
func (h *WorkflowHook) BeforeCommit(ctx context.Context, tx *sql.Tx, evt workflow.HookEvent) error {
	newStatus := mapWorkflowState(string(evt.NewState))

	if tx == nil {
		// In-memory test mode — no DB ops possible.
		return nil
	}

	// 1. Sync workflow_status on mst.rating_history_counterparty.
	if err := h.repo.UpdateWorkflowStatus(ctx, tx, evt.EntityID, newStatus, evt.ActorID); err != nil {
		return fmt.Errorf("ratinghistory.WorkflowHook.BeforeCommit: update workflow_status: %w", err)
	}

	// 2. On APPROVED: compute SICR, close previous active rating, update counterparty cache.
	if newStatus == WorkflowStatusApproved {
		rh, err := h.repo.GetByID(ctx, evt.EntityID)
		if err != nil {
			return fmt.Errorf("ratinghistory.WorkflowHook.BeforeCommit: load entity: %w", err)
		}
		if rh == nil {
			return fmt.Errorf("ratinghistory.WorkflowHook.BeforeCommit: entity not found: %s", evt.EntityID)
		}
		if err := h.svc.handleApproveTransition(ctx, tx, rh, evt.ActorID); err != nil {
			return fmt.Errorf("ratinghistory.WorkflowHook.BeforeCommit: approve side-effects: %w", err)
		}
	}

	return nil
}
