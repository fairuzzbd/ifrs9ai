package coa_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/coa"
)

// ─── repoAdapter — flexible Repository stub ───────────────────────────────────

// repoAdapter implements coa.Repository via configurable sub-stubs.
type repoAdapter struct {
	stubCreate     *stubCreate
	stubGetByID    *stubGetByID
	stubGetByKode  *stubGetByKode
	stubList       *stubList
	stubUpdate     *stubUpdate
	stubSoftDelete *stubSoftDelete
	stubExport     *stubExport
}

var _ coa.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *coa.ChartOfAccount) error {
	if a.stubCreate != nil {
		return a.stubCreate.err
	}
	return nil
}

func (a *repoAdapter) BulkCreate(_ context.Context, _ *sql.Tx, _ []*coa.ChartOfAccount) error {
	return nil
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*coa.ChartOfAccount, error) {
	if a.stubGetByID != nil {
		return a.stubGetByID.result, a.stubGetByID.err
	}
	if a.stubUpdate != nil {
		return a.stubUpdate.getByIDResult, nil
	}
	if a.stubSoftDelete != nil {
		return a.stubSoftDelete.getByIDResult, nil
	}
	return nil, nil
}

func (a *repoAdapter) GetByKode(_ context.Context, _ string, _ bool) (*coa.ChartOfAccount, error) {
	if a.stubGetByKode != nil {
		return a.stubGetByKode.result, a.stubGetByKode.err
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*coa.ChartOfAccount, error) {
	if a.stubList != nil {
		return a.stubList.items, a.stubList.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ coa.UpdateFields) (*coa.ChartOfAccount, error) {
	if a.stubUpdate != nil {
		return a.stubUpdate.result, a.stubUpdate.err
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*coa.ChartOfAccount, error) {
	if a.stubSoftDelete != nil {
		return a.stubSoftDelete.result, a.stubSoftDelete.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ coa.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountChildrenOf(_ context.Context, _ uuid.UUID) (int64, error) {
	if a.stubSoftDelete != nil {
		return a.stubSoftDelete.childCount, nil
	}
	return 0, nil
}

// BeginTx returns (nil, errTestNoDB) so service guard paths (pre-tx) work without panic.
func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}

var errTestNoDB = fmt.Errorf("test: no database available")

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]coa.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.stubExport != nil {
		return a.stubExport.reader, a.stubExport.count, a.stubExport.err
	}
	return nil, 0, nil
}

// ─── Sub-stub types ────────────────────────────────────────────────────────────

type stubCreate struct{ err error }

type stubGetByID struct {
	result *coa.ChartOfAccount
	err    error
}

type stubGetByKode struct {
	result *coa.ChartOfAccount
	err    error
}

type stubList struct {
	items []*coa.ChartOfAccount
	err   error
}

type stubUpdate struct {
	getByIDResult *coa.ChartOfAccount
	result        *coa.ChartOfAccount
	err           error
}

type stubSoftDelete struct {
	getByIDResult *coa.ChartOfAccount
	childCount    int64
	result        *coa.ChartOfAccount
	err           error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

// ─── noopJobRepo — no-op JobRepository for tests that don't need job tracking ──

type noopJobRepo struct{}

var _ coa.JobRepository = (*noopJobRepo)(nil)

func (n *noopJobRepo) InsertJob(_ context.Context, _ *coa.JobState, _ uuid.UUID, _ string) error {
	return nil
}
func (n *noopJobRepo) UpdateJobProgress(_ context.Context, _ string, _ int, _ string, _ int, _ int) error {
	return nil
}
func (n *noopJobRepo) CompleteJob(_ context.Context, _ string, _, _, _ int) error { return nil }
func (n *noopJobRepo) FailJob(_ context.Context, _ string, _ string) error        { return nil }
func (n *noopJobRepo) GetJob(_ context.Context, _ string) (*coa.JobState, error)  { return nil, nil }
