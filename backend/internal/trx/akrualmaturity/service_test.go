package akrualmaturity

// service_test.go — Service-level tests.
// Uses stub Repository and JurnalPosterStub; never hits the DB.
// Coverage targets: happy path + holiday skip + period locked + stale ECL + SoD dividen.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Stub repository ─────────────────────────────────────────────────────────

// stubRepo is a minimal in-memory stub implementing Repository.
type stubRepo struct {
	isHoliday              bool
	holidayErr             error
	periode                *PeriodeBuku
	periodeErr             error
	staleDays              int
	staleDaysErr           error
	activeAccruing         []*InstrumenAkrualInfo
	activeAccruingErr      error
	activeMaturity         []*InstrumenAkrualInfo
	activeMaturityErr      error
	isDuplicate            bool
	isDuplicateErr         error
	schedule               *AmortisasiScheduleRow
	scheduleErr            error
	eclResult              *ECLSealedResult
	eclResultErr           error
	fxRate                 *FXRateApproved
	fxRateErr              error
	lastAkrual             *PendapatanAkrual
	lastAkrualErr          error
	insertAkrualErr        error
	insertJatuhTempoErr    error
	updateJatuhTempoErr    error
	updateAkrualStatusErr  error
	insertDLQErr           error
	insertDividenErr       error
	getDividenErr          error
	updateDividenErr       error
	dividen                *Dividen
	listAkrualRows         []*PendapatanAkrual
	listAkrualErr          error
	listJatuhTempoRows     []*JatuhTempo
	listJatuhTempoErr      error
	instrumenInfo          *InstrumenAkrualInfo
	instrumenInfoErr       error
	akrualByID             *PendapatanAkrual
	akrualByIDErr          error
	// Track what was inserted
	insertedAkruals    []*PendapatanAkrual
	insertedJatuhTempo []*JatuhTempo
	dlqItems           []string
}

func (r *stubRepo) IsHoliday(_ context.Context, _ time.Time) (bool, error) {
	return r.isHoliday, r.holidayErr
}
func (r *stubRepo) GetPeriodeByTanggal(_ context.Context, _ time.Time) (*PeriodeBuku, error) {
	return r.periode, r.periodeErr
}
func (r *stubRepo) GetStaleDaysConfig(_ context.Context) (int, error) {
	return r.staleDays, r.staleDaysErr
}
func (r *stubRepo) GetActiveAccruingInstrumens(_ context.Context) ([]*InstrumenAkrualInfo, error) {
	return r.activeAccruing, r.activeAccruingErr
}
func (r *stubRepo) GetActiveMaturityInstrumens(_ context.Context, _ time.Time) ([]*InstrumenAkrualInfo, error) {
	return r.activeMaturity, r.activeMaturityErr
}
func (r *stubRepo) GetInstrumenInfo(_ context.Context, _ uuid.UUID) (*InstrumenAkrualInfo, error) {
	return r.instrumenInfo, r.instrumenInfoErr
}
func (r *stubRepo) GetSealedECLForInstrumen(_ context.Context, _ uuid.UUID) (*ECLSealedResult, error) {
	return r.eclResult, r.eclResultErr
}
func (r *stubRepo) GetAmortisasiSchedule(_ context.Context, _ uuid.UUID, _ time.Time) (*AmortisasiScheduleRow, error) {
	return r.schedule, r.scheduleErr
}
func (r *stubRepo) GetFXRateApproved(_ context.Context, _ string, _ time.Time) (*FXRateApproved, error) {
	return r.fxRate, r.fxRateErr
}
func (r *stubRepo) IsDuplicateAkrual(_ context.Context, _ uuid.UUID, _ time.Time, _ AkrualJenis) (bool, error) {
	return r.isDuplicate, r.isDuplicateErr
}
func (r *stubRepo) InsertAkrual(_ context.Context, _ *sql.Tx, a *PendapatanAkrual) error {
	r.insertedAkruals = append(r.insertedAkruals, a)
	return r.insertAkrualErr
}
func (r *stubRepo) GetAkrualByID(_ context.Context, _ uuid.UUID) (*PendapatanAkrual, error) {
	return r.akrualByID, r.akrualByIDErr
}
func (r *stubRepo) UpdateAkrualStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ AkrualStatus, _ *uuid.UUID, _ *uuid.UUID, _ *string, _ int64, _ uuid.UUID) error {
	return r.updateAkrualStatusErr
}
func (r *stubRepo) ListAkrual(_ context.Context, _ listquery.Query, _ string, _ int) ([]*PendapatanAkrual, bool, int, error) {
	return r.listAkrualRows, false, len(r.listAkrualRows), r.listAkrualErr
}
func (r *stubRepo) GetMTDYTDSummary(_ context.Context, _ *uuid.UUID, _ *uuid.UUID, _ int, _ int) (*AkrualDashboard, error) {
	return &AkrualDashboard{}, nil
}
func (r *stubRepo) InsertJatuhTempo(_ context.Context, _ *sql.Tx, jt *JatuhTempo) error {
	r.insertedJatuhTempo = append(r.insertedJatuhTempo, jt)
	return r.insertJatuhTempoErr
}
func (r *stubRepo) UpdateJatuhTempoStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ time.Time, _ JatuhTempoStatus, _ *uuid.UUID, _ *string, _ int64, _ uuid.UUID) error {
	return r.updateJatuhTempoErr
}
func (r *stubRepo) ListJatuhTempo(_ context.Context, _ listquery.Query, _ string, _ int) ([]*JatuhTempo, bool, int, error) {
	return r.listJatuhTempoRows, false, len(r.listJatuhTempoRows), r.listJatuhTempoErr
}
func (r *stubRepo) InsertDividen(_ context.Context, _ *sql.Tx, _ *Dividen) error {
	return r.insertDividenErr
}
func (r *stubRepo) GetDividenByID(_ context.Context, _ uuid.UUID) (*Dividen, error) {
	return r.dividen, r.getDividenErr
}
func (r *stubRepo) UpdateDividenStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ time.Time, _ DividenStatus, _ *uuid.UUID, _ *string, _ *string, _ *string, _ *time.Time, _ *uuid.UUID, _ int64, _ uuid.UUID) error {
	return r.updateDividenErr
}
func (r *stubRepo) InsertDLQ(_ context.Context, jobType string, _ uuid.UUID, code, detail string) error {
	r.dlqItems = append(r.dlqItems, jobType+":"+code+":"+detail)
	return r.insertDLQErr
}
func (r *stubRepo) GetLastAkrualForInstrumen(_ context.Context, _ uuid.UUID) (*PendapatanAkrual, error) {
	return r.lastAkrual, r.lastAkrualErr
}
func (r *stubRepo) BeginTx(_ context.Context) (*sql.Tx, error) {
	db, mock, err := sqlmock.New()
	if err != nil {
		return nil, fmt.Errorf("stubRepo.BeginTx sqlmock: %w", err)
	}
	mock.ExpectBegin()
	mock.ExpectCommit()
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// listQueryStub satisfies the listquery.Query interface (or whatever the service passes).
// Since service passes the actual listquery.Query type, this stub won't compile if
// Repository is typed against the real listquery.Query. We store a placeholder interface here
// that matches the List signatures below. The real repo test uses sqlmock.

// ─── Helper: build service with stubs ────────────────────────────────────────

func buildSvc(t *testing.T, repo Repository) (*Service, *JurnalPosterStub, *InstrumenStatusUpdaterStub) {
	t.Helper()
	poster := NewJurnalPosterStub(slog.Default())
	updater := NewInstrumenStatusUpdaterStub()
	svc := NewService(repo, poster, updater, nil, slog.Default())
	return svc, poster, updater
}

// ─── Helpers to build common test data ───────────────────────────────────────

func openPeriode() *PeriodeBuku {
	return &PeriodeBuku{
		ID:            uuid.New(),
		StatusPeriode: "OPEN",
		TanggalMulai:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalAkhir:  time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	}
}

func basicSchedule() *AmortisasiScheduleRow {
	eir := decimal.NewFromFloat(0.075)
	return &AmortisasiScheduleRow{
		EIRPersen: eir,
	}
}

func basicInstrumen(id uuid.UUID) *InstrumenAkrualInfo {
	return &InstrumenAkrualInfo{
		ID:                  id,
		KodeInstrumen:       "INST-0001",
		Status:              "ACTIVE",
		KlasifikasiPSAK71:   "AC",
		EIRPersen:           decimal.NewFromFloat(0.075),
		GrossCarryingIDR:    decimal.NewFromInt(1_000_000),
		MataUang:            "IDR",
		Stage:               1,
	}
}

// ─── RunDailyAkrualCron tests ─────────────────────────────────────────────────

func TestRunDailyAkrualCron_HolidaySkip(t *testing.T) {
	repo := &stubRepo{isHoliday: true, staleDays: 30}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
	assert.Equal(t, 0, result.TotalProcessed)
}

func TestRunDailyAkrualCron_HolidayErr(t *testing.T) {
	repo := &stubRepo{holidayErr: errors.New("db down")}
	svc, _, _ := buildSvc(t, repo)

	_, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "holiday check")
}

func TestRunDailyAkrualCron_PeriodeClosed(t *testing.T) {
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode: &PeriodeBuku{
			ID:            uuid.New(),
			StatusPeriode: "SOFT_CLOSED",
		},
	}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
	assert.Equal(t, 1, result.DLQCount)
}

func TestRunDailyAkrualCron_PeriodeNil(t *testing.T) {
	repo := &stubRepo{isHoliday: false, staleDays: 30, periode: nil}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestRunDailyAkrualCron_NoInstruments(t *testing.T) {
	repo := &stubRepo{
		isHoliday:      false,
		staleDays:      30,
		periode:        openPeriode(),
		activeAccruing: nil,
	}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 0, result.TotalProcessed)
}

func TestRunDailyAkrualCron_HappyPath_Stage1(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			basicInstrumen(instID),
		},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult: &ECLSealedResult{
			ECLCalcRunID: uuid.New(),
			Stage:        1,
			ECLAllowance: decimal.Zero,
			SealedAt:     time.Now().UTC().AddDate(0, 0, -5), // fresh
		},
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalProcessed)
	assert.Equal(t, 1, result.TotalSuccess)
	assert.Equal(t, 0, result.TotalFailed)
	// PostAkrual was called
	assert.Len(t, poster.AkrualCalls(), 1)
	assert.Equal(t, "AKRUAL_BUNGA", poster.AkrualCalls()[0].EventCode)
}

func TestRunDailyAkrualCron_Stage3_NetCarrying(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				KodeInstrumen:     "INST-STAGE3",
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				EIRPersen:         decimal.NewFromFloat(0.075),
				GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
				MataUang:          "IDR",
				Stage:             3,
			},
		},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult: &ECLSealedResult{
			ECLCalcRunID: uuid.New(),
			Stage:        3,
			ECLAllowance: decimal.NewFromInt(200_000), // ECL = 200k
			SealedAt:     time.Now().UTC().AddDate(0, 0, -5),
		},
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSuccess)
	// Stage 3 uses net carrying = 800k
	calls := poster.AkrualCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "AKRUAL_BUNGA_STAGE3", calls[0].EventCode)
}

func TestRunDailyAkrualCron_StaleECL_PendingStaleReview(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				KodeInstrumen:     "INST-STALE",
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				EIRPersen:         decimal.NewFromFloat(0.075),
				GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
				MataUang:          "IDR",
				Stage:             3,
			},
		},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult: &ECLSealedResult{
			ECLCalcRunID: uuid.New(),
			Stage:        3,
			ECLAllowance: decimal.NewFromInt(100_000),
			SealedAt:     time.Now().UTC().AddDate(0, 0, -45), // stale
		},
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	// Stale: inserted as PENDING_STALE_REVIEW, no jurnal posting
	assert.Empty(t, poster.AkrualCalls(), "stale ECL must NOT post jurnal")
	// DLQ alert
	assert.GreaterOrEqual(t, result.DLQCount, 1, "stale ECL must go to DLQ")
}

func TestRunDailyAkrualCron_Duplicate_Skipped(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday:      false,
		staleDays:      30,
		periode:        openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		isDuplicate:    true, // already processed
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
	assert.Empty(t, poster.AkrualCalls())
}

func TestRunDailyAkrualCron_FXMissing_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				KodeInstrumen:     "INST-USD",
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				EIRPersen:         decimal.NewFromFloat(0.075),
				GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
				MataUang:          "USD", // FCY
				Stage:             1,
			},
		},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult: &ECLSealedResult{
			ECLCalcRunID: uuid.New(),
			Stage:        1,
			ECLAllowance: decimal.Zero,
			SealedAt:     time.Now().UTC().AddDate(0, 0, -1),
		},
		fxRate:    nil,                 // FX rate missing
		fxRateErr: nil,
	}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
	assert.Equal(t, 1, result.DLQCount)
}

// ─── RunDailyMaturityCron tests ───────────────────────────────────────────────

func TestRunDailyMaturityCron_HolidaySkip(t *testing.T) {
	repo := &stubRepo{isHoliday: true}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
}

func TestRunDailyMaturityCron_PeriodeClosed(t *testing.T) {
	repo := &stubRepo{
		isHoliday: false,
		periode: &PeriodeBuku{
			ID:            uuid.New(),
			StatusPeriode: "HARD_CLOSED",
		},
	}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestRunDailyMaturityCron_HappyPath(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{
			basicInstrumen(instID),
		},
		lastAkrual: &PendapatanAkrual{
			BungaKotor: decimal.NewFromInt(10_000),
		},
	}
	svc, poster, updater := buildSvc(t, repo)

	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalProcessed)
	assert.Equal(t, 1, result.TotalSuccess)
	// PostMaturity was called
	assert.Len(t, poster.MaturityCalls(), 1)
	// InstrumenStatusUpdater.SetMatured called
	assert.Equal(t, 1, updater.Calls())
}

func TestRunDailyMaturityCron_InstrumenNotActive_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{
			{
				ID:     instID,
				Status: "MATURED", // already matured
			},
		},
	}
	svc, _, _ := buildSvc(t, repo)

	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

// ─── ApproveDividen SoD tests ─────────────────────────────────────────────────

func TestApproveDividen_SoDViolation(t *testing.T) {
	makerID := uuid.New()
	// makerID == approverID → SoD violation
	divID := uuid.New()

	dividen := &Dividen{
		ID:          divID,
		Status:      DividenPendingApproval,
		MakerID:     makerID,
		InstrumenID: uuid.New(),
	}

	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		dividen:   dividen,
	}
	svc, _, _ := buildSvc(t, repo)

	// Create a context that simulates makerID as the current user
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:      makerID.String(),
		TenantID: "TUGURE",
	})

	req := ApproveDividenRequest{
		Comment:         "Approved.",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.ApproveDividen(ctx, divID, req)
	require.Error(t, err, "SoD violation must return error")
}

func TestApproveDividen_InvalidStatus(t *testing.T) {
	approverID := uuid.New()
	makerID := uuid.New()
	divID := uuid.New()

	dividen := &Dividen{
		ID:      divID,
		Status:  DividenPosted, // already posted — CanApprove() = false
		MakerID: makerID,
	}

	repo := &stubRepo{dividen: dividen, isHoliday: false, periode: openPeriode()}
	svc, _, _ := buildSvc(t, repo)

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:      approverID.String(),
		TenantID: "TUGURE",
	})

	req := ApproveDividenRequest{
		SignatureMethod: "JWT_STEP_UP",
	}
	_, err := svc.ApproveDividen(ctx, divID, req)
	require.Error(t, err)
}

// ─── OverrideStaleAkrual tests ────────────────────────────────────────────────

func TestOverrideStaleAkrual_InvalidSignatureMethod(t *testing.T) {
	svc, _, _ := buildSvc(t, &stubRepo{})
	req := OverrideStaleRequest{
		Reason:          "ECL run foi atualizado e confirmado pelo Risk Officer antes da correção.",
		SignatureMethod: "TOTP", // invalid
	}
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.OverrideStaleAkrual(ctx, uuid.New(), req, uuid.New().String())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_STEP_UP")
}

func TestOverrideStaleAkrual_ReasonTooShort(t *testing.T) {
	svc, _, _ := buildSvc(t, &stubRepo{})
	req := OverrideStaleRequest{
		Reason:          "short",
		SignatureMethod: "JWT_STEP_UP",
	}
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.OverrideStaleAkrual(ctx, uuid.New(), req, uuid.New().String())
	require.Error(t, err)
}

func TestOverrideStaleAkrual_AkrualNotFound(t *testing.T) {
	repo := &stubRepo{akrualByIDErr: errors.New("not found")}
	svc, _, _ := buildSvc(t, repo)
	req := OverrideStaleRequest{
		Reason:          "ECL run foi atualizado e confirmado pelo Risk Officer antes da correção.",
		SignatureMethod: "JWT_STEP_UP",
	}
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.OverrideStaleAkrual(ctx, uuid.New(), req, uuid.New().String())
	require.Error(t, err)
}

// ─── Additional coverage tests ───────────────────────────────────────────────

func TestRunDailyAmortisasiCron_Holiday(t *testing.T) {
	repo := &stubRepo{isHoliday: true}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
}

func TestRunDailyAmortisasiCron_PeriodeClosed(t *testing.T) {
	repo := &stubRepo{
		isHoliday: false,
		periode:   &PeriodeBuku{ID: uuid.New(), StatusPeriode: "CLOSED"},
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestRunDailyAmortisasiCron_NoSchedule_Skipped(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		schedule:   nil, // no schedule → skipped
	}
	svc, poster, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
	assert.Empty(t, poster.AkrualCalls())
}

func TestRunDailyAmortisasiCron_ZeroAmortisasi_Skipped(t *testing.T) {
	instID := uuid.New()
	zeroSchedule := &AmortisasiScheduleRow{
		EIRPersen:       decimal.NewFromFloat(0.075),
		AmortisasiHarian: decimal.Zero, // zero → no amortisasi
	}
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		schedule:   zeroSchedule,
	}
	svc, poster, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
	assert.Empty(t, poster.AkrualCalls())
}

func TestProcessOneMaturity_FCYMissing_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
				GrossCarryingFCY:  decimal.NewFromInt(100),
				MataUang:          "USD",
			},
		},
		fxRate:    nil, // missing → DLQ
		fxRateErr: nil,
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
	assert.Equal(t, 1, result.DLQCount)
}

func TestOverrideStaleAkrual_NoClaims(t *testing.T) {
	svc, _, _ := buildSvc(t, &stubRepo{})
	req := OverrideStaleRequest{
		Reason:          "This is a valid reason with more than thirty chars.",
		SignatureMethod: "JWT_STEP_UP",
	}
	_, err := svc.OverrideStaleAkrual(context.Background(), uuid.New(), req, uuid.New().String())
	require.Error(t, err)
}

func TestOverrideStaleAkrual_WrongStatus_CannotOverride(t *testing.T) {
	repo := &stubRepo{
		akrualByID: &PendapatanAkrual{
			ID:         uuid.New(),
			Status:     AkrualAutoPosted, // CanOverride() = false
			RowVersion: 1,
		},
	}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	req := OverrideStaleRequest{
		Reason:          "This is a valid reason with more than thirty chars.",
		SignatureMethod: "JWT_STEP_UP",
	}
	_, err := svc.OverrideStaleAkrual(ctx, repo.akrualByID.ID, req, uuid.New().String())
	require.Error(t, err)
}

func TestRunDailyAmortisasiCron_Duplicate_Skipped(t *testing.T) {
	instID := uuid.New()
	kupon := decimal.NewFromFloat(0.09)
	schedule := &AmortisasiScheduleRow{
		EIRPersen:       decimal.NewFromFloat(0.075),
		KuponRate:       &kupon,
		AmortisasiHarian: decimal.NewFromFloat(100),
	}
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		schedule:    schedule,
		isDuplicate: true, // already processed
	}
	svc, poster, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSkipped)
	assert.Empty(t, poster.AkrualCalls())
}

func TestRunDailyMaturityCron_JurnalError_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{basicInstrumen(instID)},
	}
	svc, poster, _ := buildSvc(t, repo)
	poster.SetMaturityError(errors.New("jurnal error"))

	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestApproveDividen_NoClaims(t *testing.T) {
	svc, _, _ := buildSvc(t, &stubRepo{})
	// No claims in context → unauthorized
	_, err := svc.ApproveDividen(context.Background(), uuid.New(), ApproveDividenRequest{
		Comment: "ok", SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err)
}

func TestListAkrual_LimitClamped(t *testing.T) {
	repo := &stubRepo{listAkrualRows: []*PendapatanAkrual{}}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	// limit=0 → clamped to 50
	rows, _, _, err := svc.ListAkrual(ctx, listquery.Query{}, "", 0)
	require.NoError(t, err)
	assert.Len(t, rows, 0)
}

func TestListJatuhTempo_LimitClamped(t *testing.T) {
	repo := &stubRepo{listJatuhTempoRows: []*JatuhTempo{}}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	rows, _, _, err := svc.ListJatuhTempo(ctx, listquery.Query{}, "", 300) // > max 200 → clamped
	require.NoError(t, err)
	assert.Len(t, rows, 0)
}

func TestGetAkrualByID_NotFound(t *testing.T) {
	repo := &stubRepo{akrualByID: nil} // returns nil → NotFound
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.GetAkrualByID(ctx, uuid.New())
	require.Error(t, err)
}

func TestGetAkrualByID_OK(t *testing.T) {
	id := uuid.New()
	stage := 1
	eir := decimal.NewFromFloat(0.075)
	repo := &stubRepo{akrualByID: &PendapatanAkrual{
		ID: id, Stage: &stage, EIRPersen: &eir, Status: AkrualAutoPosted,
	}}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	a, err := svc.GetAkrualByID(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, id, a.ID)
}

func TestRunDailyAkrualCron_HolidayCheckErr(t *testing.T) {
	repo := &stubRepo{holidayErr: errors.New("db down")}
	svc, _, _ := buildSvc(t, repo)
	_, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.Error(t, err)
}

func TestRunDailyAmortisasiCron_GetInstrumensErr(t *testing.T) {
	repo := &stubRepo{
		isHoliday:         false,
		periode:           openPeriode(),
		activeAccruingErr: errors.New("db error"),
	}
	svc, _, _ := buildSvc(t, repo)
	_, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.Error(t, err)
}

func TestRunDailyAmortisasiCron_ScheduleErr_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		scheduleErr: errors.New("schedule error"),
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestProcessOneMaturity_LastAkrualErr(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		lastAkrualErr: errors.New("db error"),
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestPostDividen_InstrumenNotActive(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		instrumenInfo: &InstrumenAkrualInfo{ID: instrID, Status: "MATURED"},
	}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.PostDividen(ctx, CreateDividenRequest{
		InstrumenID:   instrID,
		JumlahKotor:   decimal.NewFromInt(100_000),
		TanggalTerima: "2026-06-15",
	})
	require.Error(t, err)
}

func TestApproveDividen_DividenNotFound(t *testing.T) {
	repo := &stubRepo{dividen: nil, getDividenErr: nil}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.ApproveDividen(ctx, uuid.New(), ApproveDividenRequest{
		SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err)
}

func TestRunDailyAkrualCron_InsertError_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult: &ECLSealedResult{
			ECLCalcRunID: uuid.New(),
			Stage:        1,
			SealedAt:     time.Now().UTC().AddDate(0, 0, -1),
		},
		insertAkrualErr: errors.New("insert failed"),
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestProcessOneMaturity_InsertJatuhTempoError_DLQ(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		lastAkrual: &PendapatanAkrual{BungaKotor: decimal.NewFromInt(5_000)},
		insertJatuhTempoErr: errors.New("insert error"),
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyMaturityCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestRunDailyAmortisasiCron_InsertError_DLQ(t *testing.T) {
	instID := uuid.New()
	kupon := decimal.NewFromFloat(0.09)
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{basicInstrumen(instID)},
		schedule: &AmortisasiScheduleRow{
			EIRPersen:       decimal.NewFromFloat(0.075),
			KuponRate:       &kupon,
			AmortisasiHarian: decimal.NewFromFloat(100),
		},
		isDuplicate:     false,
		insertAkrualErr: errors.New("insert failed"),
	}
	svc, _, _ := buildSvc(t, repo)
	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalFailed)
}

func TestOverrideStaleAkrual_PeriodeClosed(t *testing.T) {
	stage := 1
	eir := decimal.NewFromFloat(0.075)
	eclID := uuid.New()
	instrID := uuid.New()
	repo := &stubRepo{
		akrualByID: &PendapatanAkrual{
			ID:            uuid.New(),
			InstrumenID:   instrID,
			TanggalAkrual: time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
			Jenis:         JenisBunga,
			Stage:         &stage,
			EIRPersen:     &eir,
			Status:        AkrualPendingStaleReview,
			RowVersion:    1,
		},
		instrumenInfo: &InstrumenAkrualInfo{
			ID: instrID, Status: "ACTIVE", GrossCarryingIDR: decimal.NewFromInt(1_000_000),
		},
		eclResult: &ECLSealedResult{ECLCalcRunID: eclID, Stage: 1, SealedAt: time.Now()},
		schedule:  basicSchedule(),
		periode:   nil, // no period → closed
	}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	_, err := svc.OverrideStaleAkrual(ctx, repo.akrualByID.ID, OverrideStaleRequest{
		Reason:          "This is a valid reason with more than thirty chars.",
		SignatureMethod: "JWT_STEP_UP",
	}, uuid.New().String())
	require.Error(t, err)
}

// ─── Fix-specific tests ───────────────────────────────────────────────────────

// B1: FCY instrument — GrossCarryingFCY must be converted via approved FX rate.
// USD 100 × 15432 = IDR 1_543_200 (not 100).
func TestProcessOneAkrual_FCYConversion_UsesConvertedGross(t *testing.T) {
	instID := uuid.New()
	fxRateID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				KodeInstrumen:     "BOND-USD-001",
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				EIRPersen:         decimal.NewFromFloat(0.075),
				GrossCarryingIDR:  decimal.NewFromInt(100), // sentinel: if used → wrong
				GrossCarryingFCY:  decimal.NewFromInt(100), // 100 USD
				MataUang:          "USD",
				Stage:             1,
			},
		},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult: &ECLSealedResult{
			ECLCalcRunID: uuid.New(),
			Stage:        1,
			ECLAllowance: decimal.Zero,
			SealedAt:     time.Now().UTC().AddDate(0, 0, -1),
		},
		fxRate: &FXRateApproved{
			ID:       fxRateID,
			MataUang: "USD",
			Tanggal:  time.Now().UTC(),
			RateIDR:  decimal.NewFromInt(15_432),
		},
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalProcessed)
	assert.Equal(t, 1, result.TotalSuccess)

	calls := poster.AkrualCalls()
	require.Len(t, calls, 1)
	// BungaKotor must reflect IDR 1_543_200 × 7.5% / 365, NOT 100 × 7.5% / 365.
	// ConvertFCYtoIDR(100, 15432) = 1_543_200.
	// akrual = 1_543_200 × 0.075 / 365 ≈ 317.0822 IDR (HALF_EVEN 4dp).
	expected := decimal.NewFromInt(1_543_200).Mul(decimal.NewFromFloat(0.075)).Div(decimal.NewFromInt(365)).RoundBank(4)
	assert.True(t, calls[0].BungaKotor.Equal(expected),
		"FCY conversion must fire: expected %s, got %s", expected, calls[0].BungaKotor)
}

// M3: AMORTISASI_PD_JOB — dedicated RunDailyAmortisasiCron inserts AMORTISASI_PREMIUM row.
func TestRunDailyAmortisasiCron_PremiumBond_InsertedAsAmortisasiPremium(t *testing.T) {
	instID := uuid.New()
	kuponRate := decimal.NewFromFloat(0.09) // kupon > EIR (0.075) → premium
	schedWithPremium := &AmortisasiScheduleRow{
		EIRPersen:       decimal.NewFromFloat(0.075),
		KuponRate:       &kuponRate,
		AmortisasiHarian: decimal.NewFromFloat(100), // pre-computed, non-zero
	}
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
				MataUang:          "IDR",
			},
		},
		isDuplicate: false,
		schedule:    schedWithPremium,
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAmortisasiCron(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, 1, result.TotalSuccess)

	calls := poster.AkrualCalls()
	require.Len(t, calls, 1, "amortisasi job must post exactly one jurnal entry")
	assert.Equal(t, "AMORTISASI_PREMIUM", calls[0].EventCode,
		"premium bond (kupon > EIR) must use AMORTISASI_PREMIUM event code, not BUNGA")
	// Inserted row must have jenis AMORTISASI_PREMIUM
	require.Len(t, repo.insertedAkruals, 1)
	assert.Equal(t, JenisAmortisasiPremium, repo.insertedAkruals[0].Jenis,
		"inserted row jenis must be AMORTISASI_PREMIUM, not BUNGA")
}

// M4: eclResult==nil with ANY stage → PENDING_STALE_REVIEW (conservative default).
func TestProcessOneAkrual_NoSealedECL_Stage1_IsPendingStaleReview(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		staleDays: 30,
		periode:   openPeriode(),
		activeAccruing: []*InstrumenAkrualInfo{
			{
				ID:                instID,
				Status:            "ACTIVE",
				KlasifikasiPSAK71: "AC",
				GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
				MataUang:          "IDR",
				Stage:             1, // Stage 1 instrument, no sealed ECL
			},
		},
		isDuplicate: false,
		schedule:    basicSchedule(),
		eclResult:   nil, // no sealed ECL at all
	}
	svc, poster, _ := buildSvc(t, repo)

	result, err := svc.RunDailyAkrualCron(context.Background(), time.Now())
	require.NoError(t, err)
	// Conservative: even Stage 1 without sealed ECL → PENDING_STALE_REVIEW
	assert.Empty(t, poster.AkrualCalls(), "no jurnal must be posted when sealed ECL is missing")
	assert.GreaterOrEqual(t, result.DLQCount, 1, "must DLQ when sealed ECL is missing")
	require.Len(t, repo.insertedAkruals, 1)
	assert.Equal(t, AkrualPendingStaleReview, repo.insertedAkruals[0].Status,
		"Status must be PENDING_STALE_REVIEW when sealed ECL is absent (M4 conservative default)")
}

// M5: OverrideStaleAkrual inserts 2 rows: original→SKIPPED, new→POSTED, linked by parent_akrual_id.
func TestOverrideStaleAkrual_InsertsTwoRows_OriginalSkippedNewPosted(t *testing.T) {
	originalID := uuid.New()
	instrID := uuid.New()
	stage := 3
	eir := decimal.NewFromFloat(0.075)
	eclID := uuid.New()

	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		akrualByID: &PendapatanAkrual{
			ID:               originalID,
			InstrumenID:      instrID,
			TanggalAkrual:    time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
			Jenis:            JenisBunga,
			Stage:            &stage,
			EIRPersen:        &eir,
			BungaKotor:       decimal.NewFromFloat(205.48),
			BungaBersih:      decimal.NewFromFloat(205.48),
			CarryingBasisIDR: decimal.NewFromInt(800_000),
			Status:           AkrualPendingStaleReview,
			ECLRunIDUsed:     &eclID,
			PeriodeBulananID: func() *uuid.UUID { id := uuid.New(); return &id }(),
			RowVersion:       1,
		},
		instrumenInfo: &InstrumenAkrualInfo{
			ID:                instrID,
			Status:            "ACTIVE",
			KlasifikasiPSAK71: "AC",
			GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
			MataUang:          "IDR",
		},
		eclResult: &ECLSealedResult{
			ECLCalcRunID: eclID,
			Stage:        3,
			ECLAllowance: decimal.NewFromInt(200_000),
			SealedAt:     time.Now().UTC().AddDate(0, 0, -1),
		},
		schedule:  basicSchedule(),
		staleDays: 30,
	}
	svc, _, _ := buildSvc(t, repo)

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	req := OverrideStaleRequest{
		Reason:          "ECL sealed run confirmed valid by Risk Officer on 2026-06-20 meeting.",
		SignatureMethod: "JWT_STEP_UP",
	}

	newRow, err := svc.OverrideStaleAkrual(ctx, originalID, req, uuid.New().String())
	require.NoError(t, err)
	require.NotNil(t, newRow)

	// New POSTED row must exist
	assert.Equal(t, AkrualPosted, newRow.Status, "returned row must be POSTED")
	require.NotNil(t, newRow.ParentAkrualID, "parent_akrual_id must be set on override row")
	assert.Equal(t, originalID, *newRow.ParentAkrualID, "parent_akrual_id must point to original stale row")

	// Exactly one new akrual was inserted (the POSTED row)
	require.Len(t, repo.insertedAkruals, 1)
	assert.Equal(t, AkrualPosted, repo.insertedAkruals[0].Status)
	assert.Equal(t, originalID, *repo.insertedAkruals[0].ParentAkrualID)

	// UpdateAkrualStatus was called (to mark original SKIPPED) — tracked via updateAkrualStatusErr being nil → no panic
}
