package impactpd_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
)

// repoAdapter is a flexible test stub for impactpd.Repository.
type repoAdapter struct {
	getByID              *stubGetByID
	list                 *stubList
	update               *stubUpdate
	softDelete           *stubSoftDelete
	createErr            error
	countByPeriode       int64
	countByPeriodeErr    error
	countByPeriodeExcl   int64
	export               *stubExport
}

var _ impactpd.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *impactpd.ImpactPD) error {
	return a.createErr
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID) (*impactpd.ImpactPD, error) {
	if a.getByID != nil {
		return a.getByID.result, a.getByID.err
	}
	if a.update != nil {
		return a.update.getByIDResult, nil
	}
	if a.softDelete != nil {
		return a.softDelete.getByIDResult, nil
	}
	return nil, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*impactpd.ImpactPD, error) {
	if a.list != nil {
		return a.list.items, a.list.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactpd.UpdateFields) (*impactpd.ImpactPD, error) {
	if a.update != nil {
		return a.update.result, a.update.err
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*impactpd.ImpactPD, error) {
	if a.softDelete != nil {
		return a.softDelete.result, a.softDelete.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactpd.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountByPeriode(_ context.Context, _ uuid.UUID) (int64, error) {
	return a.countByPeriode, a.countByPeriodeErr
}

func (a *repoAdapter) CountByPeriodeExcluding(_ context.Context, _ uuid.UUID, _ uuid.UUID) (int64, error) {
	return a.countByPeriodeExcl, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactpd.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.export != nil {
		return a.export.reader, a.export.count, a.export.err
	}
	return nil, 0, nil
}

// ─── Stubs ────────────────────────────────────────────────────────────────────

type stubGetByID struct {
	result *impactpd.ImpactPD
	err    error
}

type stubList struct {
	items []*impactpd.ImpactPD
	err   error
}

type stubUpdate struct {
	getByIDResult *impactpd.ImpactPD
	result        *impactpd.ImpactPD
	err           error
}

type stubSoftDelete struct {
	getByIDResult *impactpd.ImpactPD
	result        *impactpd.ImpactPD
	err           error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

var errTestNoDB = fmt.Errorf("test: no database available")
