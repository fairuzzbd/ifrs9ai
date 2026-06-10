package impactpd_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestWorkflowHook_BeforeCommit_UpdatesStatus verifies that BeforeCommit
// calls UpdateWorkflowStatusTx with the correct mapped state (non-APPROVED path).
func TestWorkflowHook_BeforeCommit_UpdatesStatus(t *testing.T) {
	var lastStatus impactpd.WorkflowStatus
	repo := &hookRepoStub{onUpdateStatus: func(s impactpd.WorkflowStatus) { lastStatus = s }}

	svc := impactpd.NewService(repo, nil, nil)
	hook := impactpd.NewWorkflowHook(svc, repo)

	evt := workflow.HookEvent{
		EntityID: uuid.New(),
		NewState: "PENDING_APPROVAL_2",
	}

	if err := hook.BeforeCommit(context.Background(), nil, evt); err != nil {
		t.Fatalf("BeforeCommit unexpected error: %v", err)
	}
	if lastStatus != impactpd.WorkflowStatusPendingApproval2 {
		t.Errorf("expected PENDING_APPROVAL_2 got %s", lastStatus)
	}
}

// TestWorkflowHook_BeforeCommit_Approved_NoDuplicate_Succeeds verifies that when
// no duplicate APPROVED row exists, the APPROVED transition proceeds without error.
func TestWorkflowHook_BeforeCommit_Approved_NoDuplicate_Succeeds(t *testing.T) {
	entityID := uuid.New()
	periodeID := uuid.New()
	entity := &impactpd.ImpactPd{
		ID:               entityID,
		PeriodeID:        periodeID,
		ImpactMultiplier: decimal.NewFromFloat(1.0),
		TenantID:         "TUGURE",
		WorkflowStatus:   impactpd.WorkflowStatusPendingApproval2,
	}

	var lastStatus impactpd.WorkflowStatus
	repo := &hookRepoStub{
		entity:         entity,
		dupTxCount:     0, // no duplicate
		onUpdateStatus: func(s impactpd.WorkflowStatus) { lastStatus = s },
	}

	svc := impactpd.NewService(repo, nil, nil)
	hook := impactpd.NewWorkflowHook(svc, repo)

	evt := workflow.HookEvent{
		EntityID: entityID,
		NewState: workflow.StateApproved,
		ActorID:  uuid.New(),
	}

	if err := hook.BeforeCommit(context.Background(), nil, evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if lastStatus != impactpd.WorkflowStatusApproved {
		t.Errorf("expected APPROVED got %s", lastStatus)
	}
}

// TestWorkflowHook_BeforeCommit_Approved_RejectsDuplicate verifies that when
// another APPROVED row for the same periode_id already exists, BeforeCommit
// returns FL_PERIODE_DUPLICATE so the workflow transaction is rolled back.
func TestWorkflowHook_BeforeCommit_Approved_RejectsDuplicate(t *testing.T) {
	entityID := uuid.New()
	periodeID := uuid.New()
	entity := &impactpd.ImpactPd{
		ID:               entityID,
		PeriodeID:        periodeID,
		ImpactMultiplier: decimal.NewFromFloat(1.2),
		TenantID:         "TUGURE",
		WorkflowStatus:   impactpd.WorkflowStatusPendingApproval2,
	}

	repo := &hookRepoStub{
		entity:     entity,
		dupTxCount: 1, // duplicate exists
	}

	svc := impactpd.NewService(repo, nil, nil)
	hook := impactpd.NewWorkflowHook(svc, repo)

	evt := workflow.HookEvent{
		EntityID: entityID,
		NewState: workflow.StateApproved,
		ActorID:  uuid.New(),
	}

	err := hook.BeforeCommit(context.Background(), nil, evt)
	if err == nil {
		t.Fatal("expected error for duplicate, got nil")
	}

	var de *domainerrors.DomainError
	if !errors.As(err, &de) {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeFLPeriodDuplicate {
		t.Errorf("expected FL_PERIODE_DUPLICATE, got %s", de.Code())
	}
}

type hookRepoStub struct {
	entity         *impactpd.ImpactPd
	dupTxCount     int64
	dupTxErr       error
	onUpdateStatus func(impactpd.WorkflowStatus)
}

var _ impactpd.Repository = (*hookRepoStub)(nil)

func (s *hookRepoStub) Create(_ context.Context, _ *sql.Tx, _ *impactpd.ImpactPd) error {
	return nil
}
func (s *hookRepoStub) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*impactpd.ImpactPd, error) {
	return s.entity, nil
}
func (s *hookRepoStub) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool, _ string) ([]*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactpd.UpdateFields, _ string) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ string) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, status impactpd.WorkflowStatus, _ string) error {
	if s.onUpdateStatus != nil {
		s.onUpdateStatus(status)
	}
	return nil
}
func (s *hookRepoStub) CountDuplicate(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string) (int64, error) {
	return 0, nil
}
func (s *hookRepoStub) CountDuplicateTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ string) (int64, error) {
	return s.dupTxCount, s.dupTxErr
}
func (s *hookRepoStub) GetActive(_ context.Context, _ uuid.UUID, _ string) (*impactpd.ImpactPd, error) {
	return nil, nil
}
func (s *hookRepoStub) BeginTx(_ context.Context) (*sql.Tx, error) { return nil, nil }
func (s *hookRepoStub) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactpd.AuditHistoryItem, bool, error) {
	return nil, false, nil
}
func (s *hookRepoStub) ExportAll(_ context.Context, _ listquery.Query, _ string) (io.Reader, int, error) {
	return nil, 0, nil
}
