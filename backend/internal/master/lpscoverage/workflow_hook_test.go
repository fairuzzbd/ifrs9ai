package lpscoverage_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/master/lpscoverage"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

func mkEvt(id uuid.UUID, newState string) workflow.HookEvent {
	return workflow.HookEvent{EntityID: id, NewState: workflow.State(newState)}
}

// hookRepoAdapter is a minimal repo stub for WorkflowHook tests.
// It tracks calls to UpdateWorkflowStatusTx.
type hookRepoAdapter struct {
	repoAdapter
	lastID     uuid.UUID
	lastStatus lpscoverage.WorkflowStatus
	updateErr  error
}

var _ lpscoverage.Repository = (*hookRepoAdapter)(nil)

func (h *hookRepoAdapter) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, id uuid.UUID, status lpscoverage.WorkflowStatus) error {
	h.lastID = id
	h.lastStatus = status
	return h.updateErr
}

// ─── WorkflowHook.BeforeCommit ────────────────────────────────────────────────

func TestWorkflowHook_BeforeCommit_SyncsDraft(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := lpscoverage.NewWorkflowHook(repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkEvt(entityID, "DRAFT"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastID != entityID {
		t.Errorf("expected entityID=%s, got %s", entityID, repo.lastID)
	}
	if repo.lastStatus != lpscoverage.WorkflowStatusDraft {
		t.Errorf("expected DRAFT status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_SyncsApproved(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := lpscoverage.NewWorkflowHook(repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkEvt(entityID, "APPROVED"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != lpscoverage.WorkflowStatusApproved {
		t.Errorf("expected APPROVED status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_PropagatesRepoError(t *testing.T) {
	expectedErr := errTestNoDB
	repo := &hookRepoAdapter{updateErr: expectedErr}
	hook := lpscoverage.NewWorkflowHook(repo)

	err := hook.BeforeCommit(context.Background(), nil, mkEvt(uuid.New(), "APPROVED"))
	if err == nil {
		t.Fatal("expected error propagation from repo, got nil")
	}
	if err == expectedErr {
		// Direct error wrapped — acceptable.
	} else if err.Error() != expectedErr.Error() && err.Error() == "" {
		t.Errorf("expected wrapped error containing repo error, got %v", err)
	}
}

func TestWorkflowHook_BeforeCommit_PendingApproval2(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := lpscoverage.NewWorkflowHook(repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkEvt(entityID, "PENDING_APPROVAL_2"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != lpscoverage.WorkflowStatusPendingApproval2 {
		t.Errorf("expected PENDING_APPROVAL_2 status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_RejectedState(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := lpscoverage.NewWorkflowHook(repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkEvt(entityID, "REJECTED"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != lpscoverage.WorkflowStatusRejected {
		t.Errorf("expected REJECTED status, got %s", repo.lastStatus)
	}
}
