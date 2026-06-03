package impactmevpd_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
)

// repoAdapter is a flexible test stub that implements impactmevpd.Repository.
type repoAdapter struct {
	getByID                        *stubGetByID
	list                           *stubList
	update                         *stubUpdate
	softDelete                     *stubSoftDelete
	createErr                      error
	countByPeriodSkenario          int64
	countByPeriodSkenarioErr       error
	countByPeriodSkenarioExcluding int64
	export                         *stubExport
}

var _ impactmevpd.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *impactmevpd.ImpactMevPD) error {
	return a.createErr
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID) (*impactmevpd.ImpactMevPD, error) {
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

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*impactmevpd.ImpactMevPD, error) {
	if a.list != nil {
		return a.list.items, a.list.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactmevpd.UpdateFields) (*impactmevpd.ImpactMevPD, error) {
	if a.update != nil {
		return a.update.result, a.update.err
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*impactmevpd.ImpactMevPD, error) {
	if a.softDelete != nil {
		return a.softDelete.result, a.softDelete.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactmevpd.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountByPeriodSkenario(_ context.Context, _ uuid.UUID, _ impactmevpd.Skenario) (int64, error) {
	return a.countByPeriodSkenario, a.countByPeriodSkenarioErr
}

func (a *repoAdapter) CountByPeriodSkenarioExcluding(_ context.Context, _ uuid.UUID, _ impactmevpd.Skenario, _ uuid.UUID) (int64, error) {
	return a.countByPeriodSkenarioExcluding, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactmevpd.AuditHistoryItem, bool, error) {
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
	result *impactmevpd.ImpactMevPD
	err    error
}

type stubList struct {
	items []*impactmevpd.ImpactMevPD
	err   error
}

type stubUpdate struct {
	getByIDResult *impactmevpd.ImpactMevPD
	result        *impactmevpd.ImpactMevPD
	err           error
}

type stubSoftDelete struct {
	getByIDResult *impactmevpd.ImpactMevPD
	result        *impactmevpd.ImpactMevPD
	err           error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

var errTestNoDB = fmt.Errorf("test: no database available")
