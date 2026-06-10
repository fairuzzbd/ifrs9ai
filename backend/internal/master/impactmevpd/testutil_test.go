package impactmevpd_test

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
)

// errTestNoDB is returned by BeginTx to simulate no-DB environment in unit tests.
var errTestNoDB = errors.New("no database in unit test")

// repoAdapter is a flexible Repository stub for handler/service unit tests.
type repoAdapter struct {
	createErr    error
	getByIDRow   *impactmevpd.ImpactMevPd
	getByIDErr   error
	listRows     []*impactmevpd.ImpactMevPd
	listErr      error
	updateRow    *impactmevpd.ImpactMevPd
	updateErr    error
	deleteRow    *impactmevpd.ImpactMevPd
	deleteErr    error
	dupCount     int64
	dupErr       error
	dupTxCount   int64
	dupTxErr     error
	activeRows   []*impactmevpd.ImpactMevPd
	activeErr    error
	exportReader io.Reader
	exportCount  int
	exportErr    error
}

var _ impactmevpd.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *impactmevpd.ImpactMevPd) error {
	return a.createErr
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*impactmevpd.ImpactMevPd, error) {
	return a.getByIDRow, a.getByIDErr
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool, _ string) ([]*impactmevpd.ImpactMevPd, error) {
	return a.listRows, a.listErr
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactmevpd.UpdateFields, _ string) (*impactmevpd.ImpactMevPd, error) {
	return a.updateRow, a.updateErr
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ string) (*impactmevpd.ImpactMevPd, error) {
	return a.deleteRow, a.deleteErr
}

func (a *repoAdapter) UpdateWorkflowStatusTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactmevpd.WorkflowStatus, _ string) error {
	return nil
}

func (a *repoAdapter) CountDuplicate(_ context.Context, _ uuid.UUID, _ impactmevpd.Skenario, _ uuid.UUID, _ string) (int64, error) {
	return a.dupCount, a.dupErr
}

func (a *repoAdapter) CountDuplicateTx(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ impactmevpd.Skenario, _ uuid.UUID, _ string) (int64, error) {
	return a.dupTxCount, a.dupTxErr
}

func (a *repoAdapter) GetActive(_ context.Context, _ uuid.UUID, _ string) ([]*impactmevpd.ImpactMevPd, error) {
	return a.activeRows, a.activeErr
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]impactmevpd.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query, _ string) (io.Reader, int, error) {
	return a.exportReader, a.exportCount, a.exportErr
}
