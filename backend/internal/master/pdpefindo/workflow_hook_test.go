package pdpefindo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/pdpefindo"
	"log/slog"
)

// TestWorkflowHook_NewWorkflowHook verifies constructor returns non-nil.
func TestWorkflowHook_NewWorkflowHook(t *testing.T) {
	svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc)
	if hook == nil {
		t.Error("NewWorkflowHook returned nil")
	}
}

// TestWorkflowHook_OnTransition_MissingClaims_ReturnsError verifies that calling
// OnTransition without JWT claims in context returns an error (not a panic).
func TestWorkflowHook_OnTransition_MissingClaims_ReturnsError(t *testing.T) {
	svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc)

	// No claims in context
	err := hook.OnTransition(context.Background(), uuid.New(), "PENDING_REVIEW", "SUBMIT")
	if err == nil {
		t.Error("expected error when claims missing, got nil")
	}
}

// TestWorkflowHook_OnTransition_WithClaims_PropagatesError verifies that when
// the entity is not found, the hook returns an error (not found at repo level).
func TestWorkflowHook_OnTransition_WithClaims_EntityNotFound(t *testing.T) {
	// Repo returns nil entity → SyncWorkflowStatus returns ErrNotFound
	adapter := &repoAdapter{
		getByID: &stubGetByID{result: nil},
	}
	svc := pdpefindo.NewService(adapter, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc)

	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := hook.OnTransition(ctx, uuid.New(), "APPROVED", "APPROVE2")
	if err == nil {
		t.Error("expected error for missing entity, got nil")
	}
}
