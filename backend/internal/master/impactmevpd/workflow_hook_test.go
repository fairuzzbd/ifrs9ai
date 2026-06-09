package impactmevpd_test

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestWorkflowHook_BeforeCommit_UpdatesStatus verifies that BeforeCommit
// calls UpdateWorkflowStatusTx with the correct mapped state.
func TestWorkflowHook_BeforeCommit_UpdatesStatus(t *testing.T) {
	var lastStatus impactmevpd.WorkflowStatus
	repo := &hookRepoStub{onUpdateStatus: func(s impactmevpd.WorkflowStatus) { lastStatus = s }}

	svc := impactmevpd.NewService(repo, nil, nil)
	hook := impactmevpd.NewWorkflowHook(svc, repo)

	evt := workflow.HookEvent{
		EntityID: uuid.New(),
		NewState: "PENDING_REVIEW",
	}

	if err := hook.BeforeCommit(context.Background(), nil, evt); err != nil {
		t.Fatalf("BeforeCommit unexpected error: %v", err)
	}
	if lastStatus != impactmevpd.WorkflowStatusPendingReview {
		t.Errorf("expected PENDING_REVIEW got %s", lastStatus)
	}
}

// hookRepoStub is a full Repository stub for workflow hook tests.
type hookRepoStub struct {
	onUpdateStatus func(impactmevpd.WorkflowStatus)
}

var _ impactmevpd.Repository = (*hookRepoStub)(nil)

func (s *hookRepoStub) Create(_ context.Context, _ *sql.Tx, _ *impactmevpd.ImpactMevPd) error {
	return nil
}
func (s *hookRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*impactmevpd.ImpactMevPd, error) {
	return nil, nil
}
func (s *hookRepoStub) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*impactmevpd.ImpactMevPd, error) {
	return nil, nil
}
func (s *hookRepoStub) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactmevpd.UpdateFields) (*impactmevpd.ImpactMevPd, error) {
	return nil, nil
}
func (s *hookRepoStub) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*impactmevpd.ImpactMevPd, error) {
	return nil, nil
}
func (s *hookRepoStub) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, status impactmevpd.WorkflowStatus) error {
	if s.onUpdateStatus != nil {
		s.onUpdateStatus(status)
	}
	return nil
}
func (s *hookRepoStub) CountDuplicate(_ context.Context, _ uuid.UUID, _ impactmevpd.Skenario, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (s *hookRepoStub) GetActive(_ context.Context, _ uuid.UUID) ([]*impactmevpd.ImpactMevPd, error) {
	return nil, nil
}
func (s *hookRepoStub) BeginTx(_ context.Context) (*sql.Tx, error) { return nil, nil }
func (s *hookRepoStub) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactmevpd.AuditHistoryItem, bool, error) {
	return nil, false, nil
}
func (s *hookRepoStub) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	return nil, 0, nil
}
