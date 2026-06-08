package pdpefindo_test

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/master/pdpefindo"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// hookRepoAdapter is a Repository stub for WorkflowHook tests.
// It records the last UpdateWorkflowStatus call so tests can assert state mapping.
type hookRepoAdapter struct {
	repoAdapter
	lastID     uuid.UUID
	lastStatus pdpefindo.WorkflowStatus
	lastActor  uuid.UUID
	updateErr  error
}

var _ pdpefindo.Repository = (*hookRepoAdapter)(nil)

func (h *hookRepoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, id uuid.UUID, status pdpefindo.WorkflowStatus, updatedBy uuid.UUID) error {
	h.lastID = id
	h.lastStatus = status
	h.lastActor = updatedBy
	return h.updateErr
}

func mkEvt(id uuid.UUID, newState string) workflow.HookEvent {
	return workflow.HookEvent{
		EntityID: id,
		NewState: workflow.State(newState),
		ActorID:  uuid.New(),
	}
}

func TestWorkflowHook_NewWorkflowHook(t *testing.T) {
	svc := pdpefindo.NewService(&repoAdapter{}, audit.NewWriter(nil), slog.Default())
	hook := pdpefindo.NewWorkflowHook(svc, &repoAdapter{})
	if hook == nil {
		t.Error("NewWorkflowHook returned nil")
	}
}

func TestWorkflowHook_BeforeCommit_SyncsDraft(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := pdpefindo.NewWorkflowHook(nil, repo)

	entityID := uuid.New()
	err := hook.BeforeCommit(context.Background(), nil, mkEvt(entityID, "DRAFT"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastID != entityID {
		t.Errorf("expected entityID=%s, got %s", entityID, repo.lastID)
	}
	if repo.lastStatus != pdpefindo.WorkflowStatusDraft {
		t.Errorf("expected DRAFT status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_SyncsApproved(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := pdpefindo.NewWorkflowHook(nil, repo)

	err := hook.BeforeCommit(context.Background(), nil, mkEvt(uuid.New(), "APPROVED"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != pdpefindo.WorkflowStatusApproved {
		t.Errorf("expected APPROVED status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_PendingApproval2(t *testing.T) {
	repo := &hookRepoAdapter{}
	hook := pdpefindo.NewWorkflowHook(nil, repo)

	err := hook.BeforeCommit(context.Background(), nil, mkEvt(uuid.New(), "PENDING_APPROVAL_2"))
	if err != nil {
		t.Fatalf("BeforeCommit returned error: %v", err)
	}
	if repo.lastStatus != pdpefindo.WorkflowStatusPendingApproval2 {
		t.Errorf("expected PENDING_APPROVAL_2 status, got %s", repo.lastStatus)
	}
}

func TestWorkflowHook_BeforeCommit_PropagatesRepoError(t *testing.T) {
	wantErr := errors.New("test repo error")
	repo := &hookRepoAdapter{updateErr: wantErr}
	hook := pdpefindo.NewWorkflowHook(nil, repo)

	err := hook.BeforeCommit(context.Background(), nil, mkEvt(uuid.New(), "APPROVED"))
	if err == nil {
		t.Fatal("expected error propagation from repo, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped error containing repo error, got %v", err)
	}
}
