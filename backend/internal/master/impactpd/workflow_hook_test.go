package impactpd_test

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestWorkflowHook_BeforeCommit_UpdatesStatus verifies that BeforeCommit
// calls UpdateWorkflowStatusTx with the correct mapped state.
func TestWorkflowHook_BeforeCommit_UpdatesStatus(t *testing.T) {
	var lastStatus impactpd.WorkflowStatus
	repo := &hookRepoStub{onUpdateStatus: func(s impactpd.WorkflowStatus) { lastStatus = s }}

	svc := impactpd.NewService(repo, nil, nil)
	hook := impactpd.NewWorkflowHook(svc, repo)

	evt := workflow.HookEvent{
		EntityID: uuid.New(),
		NewState: "APPROVED",
	}

	if err := hook.BeforeCommit(context.Background(), nil, evt); err != nil {
		t.Fatalf("BeforeCommit unexpected error: %v", err)
	}
	if lastStatus != impactpd.WorkflowStatusApproved {
		t.Errorf("expected APPROVED got %s", lastStatus)
	}
}

type hookRepoStub struct {
	onUpdateStatus func(impactpd.WorkflowStatus)
}

var _ impactpd.Repository = (*hookRepoStub)(nil)

func (s *hookRepoStub) Create(_ context.Context, _ *sql.Tx, _ *impactpd.ImpactPd) error {
	return nil
}
func (s *hookRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactpd.UpdateFields) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, status impactpd.WorkflowStatus) error {
	if s.onUpdateStatus != nil {
		s.onUpdateStatus(status)
	}
	return nil
}
func (s *hookRepoStub) CountDuplicate(_ context.Context, _ uuid.UUID, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (s *hookRepoStub) GetActive(_ context.Context, _ uuid.UUID) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) BeginTx(_ context.Context) (*sql.Tx, error) { return nil, nil }
func (s *hookRepoStub) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactpd.AuditHistoryItem, bool, error) {
	return nil, false, nil
}
func (s *hookRepoStub) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	return nil, 0, nil
}
