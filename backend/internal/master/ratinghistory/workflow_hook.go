// Package ratinghistory — EntityHook for the workflow engine.
//
// On APPROVED: SICR computation fires, counterparty.rating_pefindo_current is updated.
// Registered in cmd/api/main.go via workflow.Service.RegisterHook("RATING_HISTORY", hook).
package ratinghistory

import (
	"context"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for RATING_HISTORY entities.
// On APPROVED transition the service computes SICR + closes previous rating +
// updates counterparty.rating_pefindo_current cache.
type WorkflowHook struct {
	svc *Service
}

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(svc *Service) *WorkflowHook {
	return &WorkflowHook{svc: svc}
}

// OnTransition is called post-commit by workflow.Service.
func (h *WorkflowHook) OnTransition(ctx context.Context, evt workflow.HookEvent) error {
	return h.svc.SyncWorkflowStatus(ctx, evt.EntityID, evt.NewState, evt.Action)
}
