package bobotskenario_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/bobotskenario"
)

// repoAdapter is a flexible Repository stub for unit tests.
// Set only the sub-stubs needed for each test; unset stubs use zero-value behavior.
type repoAdapter struct {
	createStub           *stubCreate
	getByIDStub          *stubGetByID
	listStub             *stubList
	updateStub           *stubUpdate
	softDeleteStub       *stubSoftDelete
	exportStub           *stubExport
	countRefStub         *stubCountRef
	countOverlapStub     *stubCountOverlap
	countDuplicateStub   *stubCountDuplicate
	countByPeriodStub    *stubCountByPeriod
	sumByPeriodStub      *stubSumByPeriod
	sumByPeriodTxStub    *stubSumByPeriod
	updateWFStatusTxStub *stubUpdateWFStatusTx
}

var _ bobotskenario.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *bobotskenario.BobotSkenario) error {
	if a.createStub != nil {
		return a.createStub.err
	}
	return nil
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*bobotskenario.BobotSkenario, error) {
	if a.getByIDStub != nil {
		return a.getByIDStub.result, a.getByIDStub.err
	}
	if a.updateStub != nil {
		return a.updateStub.getResult, a.updateStub.getErr
	}
	if a.softDeleteStub != nil {
		return a.softDeleteStub.getResult, a.softDeleteStub.getErr
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*bobotskenario.BobotSkenario, error) {
	if a.listStub != nil {
		return a.listStub.items, a.listStub.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ bobotskenario.UpdateFields) (*bobotskenario.BobotSkenario, error) {
	if a.updateStub != nil {
		return a.updateStub.updateResult, a.updateStub.updateErr
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*bobotskenario.BobotSkenario, error) {
	if a.softDeleteStub != nil {
		return a.softDeleteStub.deleteResult, a.softDeleteStub.deleteErr
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ bobotskenario.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	if a.countRefStub != nil {
		return a.countRefStub.count, a.countRefStub.err
	}
	return 0, nil
}

func (a *repoAdapter) CountOverlap(_ context.Context, _ bobotskenario.Skenario, _ string, _ *string, _ uuid.UUID) (int64, error) {
	if a.countOverlapStub != nil {
		return a.countOverlapStub.count, a.countOverlapStub.err
	}
	return 0, nil
}

func (a *repoAdapter) CountDuplicate(_ context.Context, _ bobotskenario.Skenario, _ string, _ *string, _ uuid.UUID) (int64, error) {
	if a.countDuplicateStub != nil {
		return a.countDuplicateStub.count, a.countDuplicateStub.err
	}
	return 0, nil
}

func (a *repoAdapter) CountByPeriod(_ context.Context, _ string, _ *string) (int64, error) {
	if a.countByPeriodStub != nil {
		return a.countByPeriodStub.count, a.countByPeriodStub.err
	}
	return 0, nil
}

func (a *repoAdapter) SumByPeriod(_ context.Context, _ string, _ *string, _ uuid.UUID) (decimal.Decimal, error) {
	if a.sumByPeriodStub != nil {
		return a.sumByPeriodStub.sum, a.sumByPeriodStub.err
	}
	return decimal.Zero, nil
}

func (a *repoAdapter) SumByPeriodTx(_ context.Context, _ *sql.Tx, _ string, _ *string, _ uuid.UUID) (decimal.Decimal, error) {
	if a.sumByPeriodTxStub != nil {
		return a.sumByPeriodTxStub.sum, a.sumByPeriodTxStub.err
	}
	if a.sumByPeriodStub != nil {
		return a.sumByPeriodStub.sum, a.sumByPeriodStub.err
	}
	return decimal.Zero, nil
}

func (a *repoAdapter) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ bobotskenario.WorkflowStatus, _ uuid.UUID) error {
	if a.updateWFStatusTxStub != nil {
		return a.updateWFStatusTxStub.err
	}
	return nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return non-nil error so service code that reaches BeginTx after guard
	// checks fails gracefully (never panic on nil tx).
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]bobotskenario.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.exportStub != nil {
		return a.exportStub.reader, a.exportStub.count, a.exportStub.err
	}
	return nil, 0, nil
}

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Stub types ───────────────────────────────────────────────────────────────

type stubCreate struct {
	err error
}

type stubGetByID struct {
	result *bobotskenario.BobotSkenario
	err    error
}

type stubList struct {
	items []*bobotskenario.BobotSkenario
	err   error
}

type stubUpdate struct {
	getResult    *bobotskenario.BobotSkenario
	getErr       error
	updateResult *bobotskenario.BobotSkenario
	updateErr    error
}

type stubSoftDelete struct {
	getResult    *bobotskenario.BobotSkenario
	getErr       error
	deleteResult *bobotskenario.BobotSkenario
	deleteErr    error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

type stubCountRef struct {
	count int64
	err   error
}

type stubCountOverlap struct {
	count int64
	err   error
}

type stubCountDuplicate struct {
	count int64
	err   error
}

type stubCountByPeriod struct {
	count int64
	err   error
}

type stubSumByPeriod struct {
	sum decimal.Decimal
	err error
}

type stubUpdateWFStatusTx struct {
	err error
}

// ─── Test data builders ───────────────────────────────────────────────────────

// testBobotSkenario returns a sample entity with valid field values.
func testBobotSkenario() *bobotskenario.BobotSkenario {
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	makerID := createdBy
	return &bobotskenario.BobotSkenario{
		ID:                 uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		Skenario:           bobotskenario.SkenarioNormal,
		Bobot:              decimal.RequireFromString("0.50000000"),
		PeriodeBerlakuDari: "2026-01-01",
		MakerID:            makerID,
		WorkflowStatus:     bobotskenario.WorkflowStatusDraft,
		CreatedAt:          time.Now(),
		CreatedBy:          &createdBy,
		RowVersion:         1,
		TenantID:           "TUGURE",
	}
}

// testBobotSkenarioWith returns a sample with overrideable fields.
func testBobotSkenarioWith(sk bobotskenario.Skenario, bobot string, status bobotskenario.WorkflowStatus) *bobotskenario.BobotSkenario {
	e := testBobotSkenario()
	e.ID = uuid.New()
	e.Skenario = sk
	e.Bobot = decimal.RequireFromString(bobot)
	e.WorkflowStatus = status
	return e
}

// ptr is a helper to create a string pointer.
func ptr(s string) *string { return &s }
