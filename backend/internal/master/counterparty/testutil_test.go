package counterparty_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/counterparty"
)

// ─── Repository stub ─────────────────────────────────────────────────────────

type repoAdapter struct {
	createErr    error
	getByID      *stubGetByID
	getPII       *stubGetPII
	list         *stubList
	update       *stubUpdate
	softDelete   *stubSoftDelete
	countRefsVal int64
	export       *stubExport
}

var _ counterparty.Repository = (*repoAdapter)(nil)

type stubGetByID struct {
	cp     *counterparty.Counterparty
	masked *counterparty.MaskedPII
	err    error
}

type stubGetPII struct {
	pii *counterparty.PIIFields
	err error
}

type stubList struct {
	items []*counterparty.Counterparty
	err   error
}

type stubUpdate struct {
	updated *counterparty.Counterparty
	err     error
}

type stubSoftDelete struct {
	deleted *counterparty.Counterparty
	err     error
}

type stubExport struct {
	reader io.Reader
	count  int
	err    error
}

// ─── Interface implementation ─────────────────────────────────────────────────

func (a *repoAdapter) Create(_ context.Context, _ *sql.Tx, _ *counterparty.Counterparty, _, _, _ *string) error {
	return a.createErr
}

func (a *repoAdapter) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*counterparty.Counterparty, *counterparty.MaskedPII, error) {
	if a.getByID == nil {
		return nil, nil, nil
	}
	return a.getByID.cp, a.getByID.masked, a.getByID.err
}

func (a *repoAdapter) GetByKode(_ context.Context, _ string, _ bool) (*counterparty.Counterparty, *counterparty.MaskedPII, error) {
	if a.getByID == nil {
		return nil, nil, nil
	}
	return a.getByID.cp, a.getByID.masked, a.getByID.err
}

func (a *repoAdapter) GetPII(_ context.Context, _ uuid.UUID) (*counterparty.PIIFields, error) {
	if a.getPII == nil {
		return nil, nil
	}
	return a.getPII.pii, a.getPII.err
}

func (a *repoAdapter) GetMaskedPII(_ context.Context, _ uuid.UUID) (*counterparty.MaskedPII, error) {
	if a.getByID != nil && a.getByID.masked != nil {
		return a.getByID.masked, nil
	}
	return &counterparty.MaskedPII{}, nil
}

func (a *repoAdapter) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*counterparty.Counterparty, error) {
	if a.list != nil {
		return a.list.items, a.list.err
	}
	return nil, nil
}

func (a *repoAdapter) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ counterparty.UpdateFields) (*counterparty.Counterparty, error) {
	if a.update != nil {
		return a.update.updated, a.update.err
	}
	return nil, nil
}

func (a *repoAdapter) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*counterparty.Counterparty, error) {
	if a.softDelete != nil {
		return a.softDelete.deleted, a.softDelete.err
	}
	return nil, nil
}

func (a *repoAdapter) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ counterparty.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) UpdateRatingCache(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ *string, _ uuid.UUID) error {
	return nil
}

func (a *repoAdapter) CountReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	return a.countRefsVal, nil
}

func (a *repoAdapter) BeginTx(_ context.Context) (*sql.Tx, error) {
	// In-memory test mode: return (nil, nil) — services treat tx=nil as
	// "no DB, skip audit write" while still proceeding with the main flow.
	// This avoids hitting the security-audit-mandated "deny if audit fails"
	// path in unit tests that have no DB available.
	return nil, nil
}

func (a *repoAdapter) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]counterparty.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (a *repoAdapter) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	if a.export != nil {
		return a.export.reader, a.export.count, a.export.err
	}
	return nil, 0, nil
}

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Test helpers ─────────────────────────────────────────────────────────────

func testCounterparty() *counterparty.Counterparty {
	createdBy := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	now := time.Now()
	return &counterparty.Counterparty{
		ID:                uuid.MustParse("00000000-0000-0000-0000-000000000010"),
		KodeCounterparty:  "CP001",
		Nama:              "Bank Mandiri",
		Tipe:              counterparty.TipeBank,
		TipeEksposurBasel: counterparty.EksposurBank,
		EligibleLpsFlag:   true,
		Status:            counterparty.StatusAktif,
		WorkflowStatus:    counterparty.WorkflowStatusApproved,
		CreatedAt:         now,
		CreatedBy:         createdBy,
		RowVersion:        1,
		TenantID:          "TUGURE",
		Version:           1,
		IsDeleted:         false,
	}
}

func newCSVReader(data string) io.Reader {
	return strings.NewReader(data)
}
