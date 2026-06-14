// Package staging — WorkflowHook for STAGING_OVERRIDE entity type.
//
// The staging override proposal manages its own workflow state directly in
// ecl.staging_override_proposal.workflow_status (not via sys.workflow_instance).
//
// This hook is registered with the generic workflow.Service so that if any
// sys.workflow_instance is created for STAGING_OVERRIDE in the future, the
// BeforeCommit callback can sync the override proposal table atomically.
//
// Currently: no-op + compile-time interface check.
package staging

import (
	"context"
	"database/sql"

	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// WorkflowHook implements workflow.EntityHook for STAGING_OVERRIDE entity type.
type WorkflowHook struct {
	overrideRepo OverrideProposalRepository
}

// Ensure WorkflowHook satisfies workflow.EntityHook at compile time.
var _ workflow.EntityHook = (*WorkflowHook)(nil)

// NewWorkflowHook creates a WorkflowHook.
func NewWorkflowHook(overrideRepo OverrideProposalRepository) *WorkflowHook {
	return &WorkflowHook{overrideRepo: overrideRepo}
}

// BeforeCommit is called inside the workflow transaction after state + signature + audit
// are written but before commit.
//
// For STAGING_OVERRIDE the proposal table is updated directly in service.go
// (not via generic workflow engine), so this hook is a no-op.
// Left as extension point for Phase 5 if sys.workflow_instance is activated.
func (h *WorkflowHook) BeforeCommit(_ context.Context, _ *sql.Tx, _ workflow.HookEvent) error {
	return nil
}
