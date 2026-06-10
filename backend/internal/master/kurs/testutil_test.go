package kurs_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/master/kurs"
)

// ─── Repo stub ────────────────────────────────────────────────────────────────

// repoStub is a flexible Repository stub that delegates to whichever sub-stubs are set.
type repoStub struct {
	createErr       error
	getByIDVal      *kurs.Kurs
	getByIDErr      error
	getByDateVal    *kurs.Kurs
	findPeriode     uuid.UUID
	findPeriodeErr  error
	findMataUang    bool
	findMataUangErr error
	listVal         []*kurs.Kurs
	listErr         error
	updateVal       *kurs.Kurs
	updateErr       error
	softDeleteVal   *kurs.Kurs
	softDeleteErr   error
	exportReader    io.Reader
	exportCount     int
	exportErr       error
}

var _ kurs.Repository = (*repoStub)(nil)

func (r *repoStub) Create(_ context.Context, _ *sql.Tx, _ *kurs.Kurs) error {
	return r.createErr
}

func (r *repoStub) GetByID(_ context.Context, _ uuid.UUID, _ bool) (*kurs.Kurs, error) {
	return r.getByIDVal, r.getByIDErr
}

func (r *repoStub) GetByKodeAndDate(_ context.Context, _ string, _ time.Time) (*kurs.Kurs, error) {
	return r.getByDateVal, nil
}

func (r *repoStub) FindActivePeriode(_ context.Context, _ time.Time) (uuid.UUID, error) {
	return r.findPeriode, r.findPeriodeErr
}

func (r *repoStub) FindMataUangApproved(_ context.Context, _ string) (bool, error) {
	return r.findMataUang, r.findMataUangErr
}

func (r *repoStub) List(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*kurs.Kurs, error) {
	return r.listVal, r.listErr
}

func (r *repoStub) Update(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ kurs.UpdateFields) (*kurs.Kurs, error) {
	return r.updateVal, r.updateErr
}

func (r *repoStub) SoftDelete(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*kurs.Kurs, error) {
	return r.softDeleteVal, r.softDeleteErr
}

func (r *repoStub) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ kurs.WorkflowStatus, _ uuid.UUID) error {
	return nil
}

func (r *repoStub) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return non-nil error so test paths that reach BeginTx fail cleanly.
	return nil, errTestNoDB
}

func (r *repoStub) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]kurs.AuditHistoryItem, bool, error) {
	return nil, false, nil
}

func (r *repoStub) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	return r.exportReader, r.exportCount, r.exportErr
}

var errTestNoDB = fmt.Errorf("test: no database available")

// ─── Test fixtures ────────────────────────────────────────────────────────────

// testPeriodeID is a fixed UUID used as fake periode_bulanan_id.
var testPeriodeID = uuid.MustParse("10000000-0000-0000-0000-000000000001")

// testKursID is a fixed UUID for a sample kurs record.
var testKursID = uuid.MustParse("20000000-0000-0000-0000-000000000001")

// testActorID is a fixed UUID for the actor user.
var testActorID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// sampleKurs returns a Kurs entity in DRAFT state for testing.
func sampleKurs() *kurs.Kurs {
	now := time.Now()
	tengah := decimal.NewFromFloat(15000.5)
	beli := decimal.NewFromFloat(14980.0)
	jual := decimal.NewFromFloat(15020.0)
	tanggal, _ := time.Parse("2006-01-02", "2026-06-05")
	return &kurs.Kurs{
		ID:               testKursID,
		FxRateIDKode:     "USD_20260605",
		KodeMataUang:     "USD",
		TanggalBerlaku:   tanggal,
		KursBeli:         &beli,
		KursJual:         &jual,
		KursTengah:       tengah,
		SumberKurs:       kurs.SumberKursManual,
		PeriodeBulananID: testPeriodeID,
		LockedFlag:       false,
		MakerID:          &testActorID,
		WorkflowStatus:   kurs.WorkflowStatusDraft,
		CreatedAt:        now,
		CreatedBy:        &testActorID,
		RowVersion:       1,
		TenantID:         "TUGURE",
	}
}

// approvedKurs returns a Kurs entity in APPROVED state.
func approvedKurs() *kurs.Kurs {
	k := sampleKurs()
	k.WorkflowStatus = kurs.WorkflowStatusApproved
	return k
}

// lockedKurs returns a Kurs entity with locked_flag = true.
func lockedKurs() *kurs.Kurs {
	k := sampleKurs()
	k.LockedFlag = true
	return k
}
