package kurs_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/master/kurs"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// hookRepoStub extends repoStub to track UpdateWorkflowStatus calls.
type hookRepoStub struct {
	repoStub
	lastID     uuid.UUID
	lastStatus kurs.WorkflowStatus
	updateErr  error
}

func (h *hookRepoStub) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, id uuid.UUID, status kurs.WorkflowStatus, _ uuid.UUID) error {
	h.lastID = id
	h.lastStatus = status
	return h.updateErr
}

func mkKursEvt(id uuid.UUID, newState string) workflow.HookEvent {
	return workflow.HookEvent{EntityID: id, NewState: workflow.State(newState)}
}

// ─── WorkflowHook.BeforeCommit ────────────────────────────────────────────────

func TestWorkflowHook_BeforeCommit_SyncsDraft(t *testing.T) {
	repo := &hookRepoStub{}
	hook := kurs.NewWorkflowHook(nil, repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkKursEvt(entityID, "DRAFT"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastID != entityID {
		t.Errorf("expected entityID=%s, got %s", entityID, repo.lastID)
	}
	if repo.lastStatus != kurs.WorkflowStatusDraft {
		t.Errorf("expected DRAFT status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_SyncsApproved(t *testing.T) {
	repo := &hookRepoStub{}
	hook := kurs.NewWorkflowHook(nil, repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkKursEvt(entityID, "APPROVED"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != kurs.WorkflowStatusApproved {
		t.Errorf("expected APPROVED status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_SyncsPendingReview(t *testing.T) {
	repo := &hookRepoStub{}
	hook := kurs.NewWorkflowHook(nil, repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkKursEvt(entityID, "PENDING_REVIEW"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != kurs.WorkflowStatusPendingReview {
		t.Errorf("expected PENDING_REVIEW status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_SyncsPendingApproval(t *testing.T) {
	repo := &hookRepoStub{}
	hook := kurs.NewWorkflowHook(nil, repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkKursEvt(entityID, "PENDING_APPROVAL"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != kurs.WorkflowStatusPendingApproval {
		t.Errorf("expected PENDING_APPROVAL status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_SyncsRejected(t *testing.T) {
	repo := &hookRepoStub{}
	hook := kurs.NewWorkflowHook(nil, repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkKursEvt(entityID, "REJECTED"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != kurs.WorkflowStatusRejected {
		t.Errorf("expected REJECTED status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_PropagatesRepoError(t *testing.T) {
	expectedErr := fmt.Errorf("db write failed")
	repo := &hookRepoStub{updateErr: expectedErr}
	hook := kurs.NewWorkflowHook(nil, repo)

	err := hook.BeforeCommit(context.Background(), nil, mkKursEvt(uuid.New(), "APPROVED"))
	if err == nil {
		t.Fatal("expected error propagation from repo, got nil")
	}
}
