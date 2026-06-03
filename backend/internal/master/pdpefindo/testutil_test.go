package pdpefindo_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/pdpefindo"
)

// repoAdapter is a flexible Repository stub for unit tests.
// Only set the stubs needed for each test; zero-value behaviour is safe.
type repoAdapter struct {
	getByID       *stubGetByID
	list          *stubList
	update        *stubUpdate
	softDelete    *stubSoftDelete
	export        *stubExport
	countOverlap  *stubCountOverlap
	getJobByID    *stubGetJobByID
}

var _ pdpefindo.Repository = (*repoAdapter)(nil)

// ─── Create ───────────────────────────────────────────────────────────────────

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, p *pdpefindo.PDPefindo) error {
	return nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

type stubGetByID struct {
	result *pdpefindo.PDPefindo
	err    error
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*pdpefindo.PDPefindo, error) {
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

// ─── List ─────────────────────────────────────────────────────────────────────

type stubList struct {
	result []*pdpefindo.PDPefindo
	err    error
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*pdpefindo.PDPefindo, error) {
	if a.list != nil {
		return a.list.result, a.list.err
	}
	return nil, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

type stubUpdate struct {
	getByIDResult *pdpefindo.PDPefindo
	updateResult  *pdpefindo.PDPefindo
	updateErr     error
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ pdpefindo.UpdateFields) (*pdpefindo.PDPefindo, error) {
	if a.update != nil {
		return a.update.updateResult, a.update.updateErr
	}
	return nil, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

type stubSoftDelete struct {
	getByIDResult     *pdpefindo.PDPefindo
	countRefsVal      int64
	softDeleteResult  *pdpefindo.PDPefindo
	softDeleteErr     error
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*pdpefindo.PDPefindo, error) {
	if a.softDelete != nil {
		return a.softDelete.softDeleteResult, a.softDelete.softDeleteErr
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ pdpefindo.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountOverlap(_ context.Context, _ string, _ string, _ *string, _ *uuid.UUID) (int64, error) {
	if a.countOverlap != nil {
		return a.countOverlap.count, a.countOverlap.err
	}
	return 0, nil
}

type stubCountOverlap struct {
	count int64
	err   error
}

func (a *repoAdapter) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	if a.softDelete != nil {
		return a.softDelete.countRefsVal, nil
	}
	return 0, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return error so service code that reaches BeginTx after passing guard checks
	// fails gracefully (tests don't need a real DB).
	return nil, errTestNoDB
}

var errTestNoDB = fmt.Errorf("test: no database available")

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]pdpefindo.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

// ─── Export ───────────────────────────────────────────────────────────────────

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.export != nil {
		return a.export.reader, a.export.count, a.export.err
	}
	return nil, 0, nil
}

// ─── BulkCreate ───────────────────────────────────────────────────────────────

func (a *repoAdapter) BulkCreate(_ context.Context, _ *sql.Tx, rows []*pdpefindo.PDPefindo) (int, error) {
	return len(rows), nil
}

// ─── Job tracking ─────────────────────────────────────────────────────────────

type stubGetJobByID struct {
	result *pdpefindo.JobRow
	err    error
}

func (a *repoAdapter) GetJobByID(_ context.Context, _ string) (*pdpefindo.JobRow, error) {
	if a.getJobByID != nil {
		return a.getJobByID.result, a.getJobByID.err
	}
	return nil, nil
}

func (a *repoAdapter) CreateJob(_ context.Context, _ *sql.Tx, _ *pdpefindo.JobRow) error {
	return nil
}

func (a *repoAdapter) UpdateJobProgress(_ context.Context, _ string, _ int, _ string) error {
	return nil
}

func (a *repoAdapter) CompleteJob(_ context.Context, _ string, _ []byte) error {
	return nil
}

func (a *repoAdapter) FailJob(_ context.Context, _ string, _ []byte) error {
	return nil
}

// ─── Test fixtures ────────────────────────────────────────────────────────────

func testPDPefindo() *pdpefindo.PDPefindo {
	now := time.Now()
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	pd3y := decimal.NewFromFloat(0.01500000)
	pd5y := decimal.NewFromFloat(0.02500000)
	pd7y := decimal.NewFromFloat(0.03500000)
	pd10y := decimal.NewFromFloat(0.05000000)
	sumber := pdpefindo.DefaultSumber
	tanggalPub := "2024-01-15"
	periodeSampai := "2024-12-31"
	return &pdpefindo.PDPefindo{
		ID:                   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		Rating:               "idAA",
		PD12Month:            decimal.NewFromFloat(0.00500000),
		PDLifetime3Y:         &pd3y,
		PDLifetime5Y:         &pd5y,
		PDLifetime7Y:         &pd7y,
		PDLifetime10Y:        &pd10y,
		Sumber:               sumber,
		TanggalPublikasi:     &tanggalPub,
		PeriodeBerlakuDari:   "2024-01-01",
		PeriodeBerlakuSampai: &periodeSampai,
		WorkflowStatus:       pdpefindo.WorkflowStatusDraft,
		UploadedBy:           createdBy,
		UploadedAt:           now,
		CreatedAt:            now,
		CreatedBy:            &createdBy,
		RowVersion:           1,
		TenantID:             "TUGURE",
	}
}
