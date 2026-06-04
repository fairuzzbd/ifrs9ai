package mappingjurnal_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/mappingjurnal"
)

// ─── repoAdapter stub ────────────────────────────────────────────────────────

// repoAdapter is a flexible Repository stub that delegates each call to the
// sub-stub that is set; unset stubs return zero values / nil errors.
type repoAdapter struct {
	createHeader    *stubCreateHeader
	getHeader       *stubGetHeader
	getDetails      *stubGetDetails
	listHeaders     *stubListHeaders
	updateHeader    *stubUpdateHeader
	softDelete      *stubSoftDelete
	countRefs       *stubCountRefs
	checkCoA        *stubCheckCoA
	workflowStatus  *stubWorkflowStatus
	bulkReplace     *stubBulkReplace
	exportStub      *stubExport
	auditHistory    *stubAuditHistory
}

var _ mappingjurnal.Repository = (*repoAdapter)(nil)

func (a *repoAdapter) CreateHeader(_ context.Context, _ *sql.Tx, _ *mappingjurnal.Header) error {
	if a.createHeader != nil {
		return a.createHeader.err
	}
	return nil
}

func (a *repoAdapter) CreateDetails(_ context.Context, _ *sql.Tx, _ []*mappingjurnal.Detail) error {
	return nil
}

func (a *repoAdapter) GetHeaderByID(_ context.Context, _ uuid.UUID, _ bool) (*mappingjurnal.Header, error) {
	if a.getHeader != nil {
		return a.getHeader.header, a.getHeader.err
	}
	return nil, nil
}

func (a *repoAdapter) GetHeaderByEventCode(_ context.Context, _ string, _ bool) (*mappingjurnal.Header, error) {
	if a.getHeader != nil {
		return a.getHeader.header, a.getHeader.err
	}
	return nil, nil
}

func (a *repoAdapter) GetDetailsByHeaderID(_ context.Context, _ uuid.UUID, _ bool) ([]*mappingjurnal.Detail, error) {
	if a.getDetails != nil {
		return a.getDetails.details, a.getDetails.err
	}
	return []*mappingjurnal.Detail{}, nil
}

func (a *repoAdapter) ListHeaders(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*mappingjurnal.Header, error) {
	if a.listHeaders != nil {
		return a.listHeaders.items, a.listHeaders.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateHeader(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ mappingjurnal.HeaderUpdateFields) (*mappingjurnal.Header, error) {
	if a.updateHeader != nil {
		return a.updateHeader.header, a.updateHeader.err
	}
	return nil, nil
}

func (a *repoAdapter) BulkReplaceDetails(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ []*mappingjurnal.Detail, _ uuid.UUID) error {
	if a.bulkReplace != nil {
		return a.bulkReplace.err
	}
	return nil
}

func (a *repoAdapter) SoftDeleteHeader(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*mappingjurnal.Header, error) {
	if a.softDelete != nil {
		return a.softDelete.header, a.softDelete.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ mappingjurnal.WorkflowStatus, _ uuid.UUID) error {
	if a.workflowStatus != nil {
		return a.workflowStatus.err
	}
	return nil
}

func (a *repoAdapter) CountHeaderReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	if a.countRefs != nil {
		return a.countRefs.count, a.countRefs.err
	}
	return 0, nil
}

func (a *repoAdapter) CheckCoAApproved(_ context.Context, _ uuid.UUID) (bool, error) {
	if a.checkCoA != nil {
		return a.checkCoA.approved, a.checkCoA.err
	}
	return true, nil // default: approved (simplify tests that don't care about CoA)
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return errTestNoDB so service code that reaches BeginTx (after passing guard checks)
	// fails gracefully without panicking on nil tx.
	return nil, errTestNoDB
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]mappingjurnal.AuditHistoryItem, bool, error) {
	if a.auditHistory != nil {
		return a.auditHistory.items, a.auditHistory.hasMore, a.auditHistory.err
	}
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.exportStub != nil {
		return a.exportStub.reader, a.exportStub.count, a.exportStub.err
	}
	return nil, 0, nil
}

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Sub-stubs ────────────────────────────────────────────────────────────────

type stubCreateHeader struct {
	err error
}

type stubGetHeader struct {
	header *mappingjurnal.Header
	err    error
}

type stubGetDetails struct {
	details []*mappingjurnal.Detail
	err     error
}

type stubListHeaders struct {
	items []*mappingjurnal.Header
	err   error
}

type stubUpdateHeader struct {
	header *mappingjurnal.Header
	err    error
}

type stubSoftDelete struct {
	header *mappingjurnal.Header
	err    error
}

type stubCountRefs struct {
	count int64
	err   error
}

type stubCheckCoA struct {
	approved bool
	err      error
}

type stubWorkflowStatus struct {
	err error
}

type stubBulkReplace struct {
	err error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

type stubAuditHistory struct {
	items   []mappingjurnal.AuditHistoryItem
	hasMore bool
	err     error
}
