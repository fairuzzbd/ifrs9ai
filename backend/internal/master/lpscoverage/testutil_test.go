package lpscoverage_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/lpscoverage"
)

// repoAdapter is a flexible Repository stub for unit tests.
// Each sub-stub controls the behaviour of one method group.
type repoAdapter struct {
	stub       *stubCreate
	getByID    *stubGetByID
	listStub   *stubList
	updateStub *stubUpdate
	deleteStub *stubDelete
	exportStub *stubExport
	overlapCnt int64
	overlapErr error
	refCount   int64
	refErr     error
}

var _ lpscoverage.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *lpscoverage.LPSCoverage) error {
	if a.stub != nil {
		return a.stub.err
	}
	return nil
}

func (a *repoAdapter) GetByID(_ context.Context, id uuid.UUID, _ bool) (*lpscoverage.LPSCoverage, error) {
	if a.getByID != nil {
		return a.getByID.result, a.getByID.err
	}
	if a.updateStub != nil {
		return a.updateStub.getResult, a.updateStub.getErr
	}
	if a.deleteStub != nil {
		return a.deleteStub.getResult, a.deleteStub.getErr
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*lpscoverage.LPSCoverage, error) {
	if a.listStub != nil {
		return a.listStub.result, a.listStub.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ lpscoverage.UpdateFields) (*lpscoverage.LPSCoverage, error) {
	if a.updateStub != nil {
		return a.updateStub.updateResult, a.updateStub.updateErr
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*lpscoverage.LPSCoverage, error) {
	if a.deleteStub != nil {
		return a.deleteStub.deleteResult, a.deleteStub.deleteErr
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ lpscoverage.WorkflowStatus) error {
	return nil
}

func (a *repoAdapter) CountOverlap(_ context.Context, _ string, _ *string, _ uuid.UUID) (int64, error) {
	return a.overlapCnt, a.overlapErr
}

func (a *repoAdapter) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	return a.refCount, a.refErr
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]lpscoverage.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.exportStub != nil {
		return a.exportStub.reader, a.exportStub.count, a.exportStub.err
	}
	return nil, 0, nil
}

// ─── Sub-stubs ────────────────────────────────────────────────────────────────

type stubCreate struct{ err error }

type stubGetByID struct {
	result *lpscoverage.LPSCoverage
	err    error
}

type stubList struct {
	result []*lpscoverage.LPSCoverage
	err    error
}

type stubUpdate struct {
	getResult    *lpscoverage.LPSCoverage
	getErr       error
	updateResult *lpscoverage.LPSCoverage
	updateErr    error
}

type stubDelete struct {
	getResult    *lpscoverage.LPSCoverage
	getErr       error
	deleteResult *lpscoverage.LPSCoverage
	deleteErr    error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Factory helpers ──────────────────────────────────────────────────────────

// testLPSCoverage returns a sample LPSCoverage entity for tests.
func testLPSCoverage() *lpscoverage.LPSCoverage {
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	return &lpscoverage.LPSCoverage{
		ID:                 uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		CoverageAmount:     decimal.NewFromInt(2_000_000_000),
		MataUang:           "IDR",
		PeriodeBerlakuDari: "2026-01-01",
		WorkflowStatus:     lpscoverage.WorkflowStatusDraft,
		MakerID:            actorID,
		CreatedBy:          &actorID,
		RowVersion:         1,
		TenantID:           "TUGURE",
	}
}

// approvedLPSCoverage returns a sample APPROVED record.
func approvedLPSCoverage() *lpscoverage.LPSCoverage {
	lc := testLPSCoverage()
	lc.WorkflowStatus = lpscoverage.WorkflowStatusApproved
	return lc
}
