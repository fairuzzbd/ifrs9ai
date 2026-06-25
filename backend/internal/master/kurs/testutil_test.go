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

// Ensure repoStub also satisfies RepositoryP5M5 at compile time.
var _ kurs.RepositoryP5M5 = (*repoStub)(nil)

// ─── Repo stub ────────────────────────────────────────────────────────────────

// repoStub is a flexible Repository stub that delegates to whichever sub-stubs are set.
// It implements both Repository and RepositoryP5M5.
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

	// P5-M5 stub fields
	batchRows          []*kurs.Kurs
	isHoliday          bool
	configParams       map[string]string
	previousRate       *decimal.Decimal
	instrumenKlasifikasi string
	instrumenMataUang  string
}

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

// ─── RepositoryP5M5 stub methods ─────────────────────────────────────────────

func (r *repoStub) InsertBatch(_ context.Context, _ *sql.Tx, _ []*kurs.Kurs) error {
	return r.createErr
}

func (r *repoStub) GetBatchByID(_ context.Context, _ uuid.UUID) ([]*kurs.Kurs, error) {
	return r.batchRows, nil
}

func (r *repoStub) SetBatchApproved(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (int64, error) {
	return int64(len(r.batchRows)), nil
}

func (r *repoStub) SetBatchRejected(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ string, _ uuid.UUID) (int64, error) {
	return int64(len(r.batchRows)), nil
}

func (r *repoStub) GetPreviousActiveRate(_ context.Context, _ string, _ time.Time) (*decimal.Decimal, error) {
	return r.previousRate, nil
}

func (r *repoStub) IsHoliday(_ context.Context, _ time.Time) (bool, error) {
	return r.isHoliday, nil
}

func (r *repoStub) GetConfigParam(_ context.Context, key string) (string, error) {
	if r.configParams != nil {
		if v, ok := r.configParams[key]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("config key %q not found in stub", key)
}

func (r *repoStub) GetInstrumenForTreatment(_ context.Context, _ uuid.UUID) (string, string, error) {
	return r.instrumenKlasifikasi, r.instrumenMataUang, nil
}

func (r *repoStub) InsertDLQEntry(_ context.Context, _ time.Time, _, _, _ string, _ []byte) error {
	return nil
}

func (r *repoStub) LockRatesForPeriode(_ context.Context, _ *sql.Tx, _ uuid.UUID) error {
	return nil
}

func (r *repoStub) UnlockRatesForPeriode(_ context.Context, _ *sql.Tx, _ uuid.UUID) error {
	return nil
}

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
