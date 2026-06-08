package periodebuku_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/periodebuku"
)

// repoAdapter is a flexible Repository stub that delegates to whichever
// sub-stub is set. Unset stubs use zero-value behavior.
type repoAdapter struct {
	createStub  *stubCreate
	getByIDStub *stubGetByID
	getByKode   *stubGetByKode
	listStub    *stubList
	updateStub  *stubUpdate
	deleteStub  *stubSoftDelete
	exportStub  *stubExport
	bulkCreate  *stubBulkCreate
	// allowTx=true makes BeginTx return (nil, nil) so service code that opens a tx
	// can proceed past BeginTx without panicking. The tx passed to repo methods will
	// be nil; repo stubs must tolerate a nil *sql.Tx.
	allowTx bool
}

var _ periodebuku.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *periodebuku.PeriodeBuku) error {
	if a.createStub != nil {
		return a.createStub.err
	}
	return nil
}

func (a *repoAdapter) GetByID(_ context.Context, id uuid.UUID) (*periodebuku.PeriodeBuku, error) {
	if a.getByIDStub != nil {
		return a.getByIDStub.result, a.getByIDStub.err
	}
	if a.updateStub != nil {
		return a.updateStub.getByIDResult, nil
	}
	if a.deleteStub != nil {
		return a.deleteStub.getByIDResult, nil
	}
	return nil, nil
}

func (a *repoAdapter) GetByKode(_ context.Context, _ string) (*periodebuku.PeriodeBuku, error) {
	if a.getByKode != nil {
		return a.getByKode.result, a.getByKode.err
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*periodebuku.PeriodeBuku, error) {
	if a.listStub != nil {
		return a.listStub.result, a.listStub.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ periodebuku.UpdateFields) (*periodebuku.PeriodeBuku, error) {
	if a.updateStub != nil {
		return a.updateStub.result, a.updateStub.err
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*periodebuku.PeriodeBuku, error) {
	if a.deleteStub != nil {
		return a.deleteStub.result, a.deleteStub.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ periodebuku.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) BulkCreateIfNotExists(_ context.Context, _ *sql.Tx, rows []*periodebuku.PeriodeBuku) (int, int, error) {
	if a.bulkCreate != nil {
		return a.bulkCreate.created, a.bulkCreate.skipped, a.bulkCreate.err
	}
	// Default: all created, none skipped.
	return len(rows), 0, nil
}

func (a *repoAdapter) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	if a.deleteStub != nil {
		return a.deleteStub.refCount, nil
	}
	return 0, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	if a.allowTx {
		// Return (nil, nil): service code proceeds, repo stubs receive nil *sql.Tx.
		// Safe because stub methods ignore the tx argument.
		return nil, nil
	}
	// Default: non-nil error so service guard-path tests fail cleanly.
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]periodebuku.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.exportStub != nil {
		return a.exportStub.reader, a.exportStub.count, a.exportStub.err
	}
	return nil, 0, nil
}

// ─── errTestNoDB ─────────────────────────────────────────────────────────────

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Stub structs ─────────────────────────────────────────────────────────────

type stubCreate struct {
	err error
}

type stubGetByID struct {
	result *periodebuku.PeriodeBuku
	err    error
}

type stubGetByKode struct {
	result *periodebuku.PeriodeBuku
	err    error
}

type stubList struct {
	result []*periodebuku.PeriodeBuku
	err    error
}

type stubUpdate struct {
	getByIDResult *periodebuku.PeriodeBuku
	result        *periodebuku.PeriodeBuku
	err           error
}

type stubSoftDelete struct {
	getByIDResult *periodebuku.PeriodeBuku
	refCount      int64
	result        *periodebuku.PeriodeBuku
	err           error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

type stubBulkCreate struct {
	created int
	skipped int
	err     error
}
