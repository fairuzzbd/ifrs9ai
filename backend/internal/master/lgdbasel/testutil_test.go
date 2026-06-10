package lgdbasel_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/lgdbasel"
)

// repoAdapter is a flexible Repository stub for unit tests.
// Set only the sub-stubs needed for each test; unset stubs use zero-value behavior.
type repoAdapter struct {
	createStub       *stubCreate
	getByIDStub      *stubGetByID
	listStub         *stubList
	updateStub       *stubUpdate
	softDeleteStub   *stubSoftDelete
	exportStub       *stubExport
	countRefStub     *stubCountRef
	countOverlapStub *stubCountOverlap
}

var _ lgdbasel.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *lgdbasel.LGDBasel) error {
	if a.createStub != nil {
		return a.createStub.err
	}
	return nil
}

func (a *repoAdapter) GetByID(_ context.Context, id uuid.UUID, _ bool) (*lgdbasel.LGDBasel, error) {
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

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*lgdbasel.LGDBasel, error) {
	if a.listStub != nil {
		return a.listStub.items, a.listStub.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ lgdbasel.UpdateFields) (*lgdbasel.LGDBasel, error) {
	if a.updateStub != nil {
		return a.updateStub.updateResult, a.updateStub.updateErr
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*lgdbasel.LGDBasel, error) {
	if a.softDeleteStub != nil {
		return a.softDeleteStub.deleteResult, a.softDeleteStub.deleteErr
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ lgdbasel.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	if a.countRefStub != nil {
		return a.countRefStub.count, a.countRefStub.err
	}
	return 0, nil
}

func (a *repoAdapter) CountOverlap(_ context.Context, _ lgdbasel.TipeEksposur, _ string, _ *string, _ uuid.UUID) (int64, error) {
	if a.countOverlapStub != nil {
		return a.countOverlapStub.count, a.countOverlapStub.err
	}
	return 0, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return non-nil error so service code that reaches BeginTx after guard
	// checks fails gracefully (never panic on nil tx).
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]lgdbasel.AuditHistoryItem, bool, error) {
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
	result *lgdbasel.LGDBasel
	err    error
}

type stubList struct {
	items []*lgdbasel.LGDBasel
	err   error
}

type stubUpdate struct {
	getResult    *lgdbasel.LGDBasel
	getErr       error
	updateResult *lgdbasel.LGDBasel
	updateErr    error
}

type stubSoftDelete struct {
	getResult    *lgdbasel.LGDBasel
	getErr       error
	deleteResult *lgdbasel.LGDBasel
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

// ─── Test data builders ───────────────────────────────────────────────────────

// testLGDBasel returns a sample entity with valid field values.
func testLGDBasel() *lgdbasel.LGDBasel {
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	makerID := createdBy
	return &lgdbasel.LGDBasel{
		ID:                 uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		TipeEksposur:       lgdbasel.TipeEksposurCorporate,
		LGD:                decimal.RequireFromString("0.4500"),
		PeriodeBerlakuDari: "2026-01-01",
		Sumber:             "BASEL_III_IRB",
		MakerID:            makerID,
		WorkflowStatus:     lgdbasel.WorkflowStatusDraft,
		CreatedBy:          &createdBy,
		RowVersion:         1,
		TenantID:           "TUGURE",
	}
}
