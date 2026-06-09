package impactpd_test

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactpd"
)

var errTestNoDB = errors.New("no database in unit test")

type repoAdapter struct {
	createErr    error
	getByIDRow   *impactpd.ImpactPd
	getByIDErr   error
	listRows     []*impactpd.ImpactPd
	listErr      error
	updateRow    *impactpd.ImpactPd
	updateErr    error
	deleteRow    *impactpd.ImpactPd
	deleteErr    error
	dupCount     int64
	dupErr       error
	activeRow    *impactpd.ImpactPd
	activeErr    error
	exportReader io.Reader
	exportCount  int
	exportErr    error
}

var _ impactpd.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *impactpd.ImpactPd) error {
	return a.createErr
}
func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*impactpd.ImpactPd, error) {
	return a.getByIDRow, a.getByIDErr
}
func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*impactpd.ImpactPd, error) {
	return a.listRows, a.listErr
}
func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactpd.UpdateFields) (*impactpd.ImpactPd, error) {
	return a.updateRow, a.updateErr
}
func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*impactpd.ImpactPd, error) {
	return a.deleteRow, a.deleteErr
}
func (a *repoAdapter) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactpd.WorkflowStatus) error {
	return nil
}
func (a *repoAdapter) CountDuplicate(_ context.Context, _ uuid.UUID, _ uuid.UUID) (int64, error) {
	return a.dupCount, a.dupErr
}
func (a *repoAdapter) GetActive(_ context.Context, _ uuid.UUID) (*impactpd.ImpactPd, error) {
	return a.activeRow, a.activeErr
}
func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}
func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactpd.AuditHistoryItem, bool, error) {
	return nil, false, nil
}
func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	return a.exportReader, a.exportCount, a.exportErr
}
