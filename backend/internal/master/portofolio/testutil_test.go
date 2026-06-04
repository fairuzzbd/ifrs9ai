package portofolio_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/portofolio"
)

// repoAdapter is a flexible Repository stub for unit tests.
// Unset sub-stubs use zero-value behaviour.
type repoAdapter struct {
	stub       *stubCreate
	getByKode  *stubGetByKode
	getByID    *stubGetByID
	list       *stubList
	update     *stubUpdate
	softDelete *stubSoftDelete
	export     *stubExport
}

var _ portofolio.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *portofolio.Portofolio) error {
	if a.stub != nil {
		return a.stub.createErr
	}
	return nil
}

func (a *repoAdapter) GetByKode(_ context.Context, _ string, _ bool) (*portofolio.Portofolio, error) {
	if a.getByKode != nil {
		return a.getByKode.result, a.getByKode.err
	}
	if a.update != nil {
		return a.update.getByKodeResult, nil
	}
	if a.softDelete != nil {
		return a.softDelete.getByKodeResult, nil
	}
	return nil, nil
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID) (*portofolio.Portofolio, error) {
	if a.getByID != nil {
		return a.getByID.result, a.getByID.err
	}
	if a.getByKode != nil {
		return a.getByKode.result, a.getByKode.err
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*portofolio.Portofolio, error) {
	if a.list != nil {
		return a.list.listResult, a.list.listErr
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ string, _ portofolio.UpdateFields) (*portofolio.Portofolio, error) {
	if a.update != nil {
		return a.update.updateResult, a.update.updateErr
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ string, _ uuid.UUID) (*portofolio.Portofolio, error) {
	if a.softDelete != nil {
		return a.softDelete.softDeleteResult, a.softDelete.softDeleteErr
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ portofolio.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountReferences(_ context.Context, _ string) (int64, error) {
	if a.softDelete != nil {
		return a.softDelete.countReferencesVal, nil
	}
	return 0, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return non-nil error so service paths that reach BeginTx after validation fail gracefully.
	return nil, errTestNoDB
}

var errTestNoDB = fmt.Errorf("test: no database available")

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]portofolio.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.export != nil {
		return a.export.reader, a.export.count, a.export.err
	}
	return nil, 0, nil
}

// ─── Stub types ───────────────────────────────────────────────────────────────

type stubCreate struct {
	createErr error
}

type stubGetByKode struct {
	result *portofolio.Portofolio
	err    error
}

type stubGetByID struct {
	result *portofolio.Portofolio
	err    error
}

type stubList struct {
	listResult []*portofolio.Portofolio
	listErr    error
}

type stubUpdate struct {
	getByKodeResult *portofolio.Portofolio
	updateErr       error
	updateResult    *portofolio.Portofolio
}

type stubSoftDelete struct {
	getByKodeResult    *portofolio.Portofolio
	countReferencesVal int64
	softDeleteErr      error
	softDeleteResult   *portofolio.Portofolio
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}
