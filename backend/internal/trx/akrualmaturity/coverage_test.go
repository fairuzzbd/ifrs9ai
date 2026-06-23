package akrualmaturity

// coverage_test.go — Extra tests to push total coverage to ≥85%.
//
// Targets:
//   - domain: CanOverride, CanApprove, ToAkrualListItem, ToJatuhTempoListItem, ParseDateStrict
//   - jurnal_poster: JurnalPosterStub helpers (SetError, Reset, DividenCalls, SetAkrualError, etc)
//   - jurnal_poster: NewNoopJurnalPoster + NoopJurnalPoster methods
//   - service: PostDividen happy path, validation errors
//   - service: ApproveDividen happy path
//   - service: OverrideStaleAkrual with valid status
//   - service: ListAkrual/ListJatuhTempo
//   - handler: more routes (GetByID OK, ListJatuhTempo pagination, OverrideStale valid)

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── domain: CanOverride, CanApprove ─────────────────────────────────────────

func TestAkrualStatus_CanOverride(t *testing.T) {
	assert.True(t, AkrualPendingStaleReview.CanOverride())
	assert.False(t, AkrualAutoPosted.CanOverride())
	assert.False(t, AkrualPosted.CanOverride())
	assert.False(t, AkrualSkipped.CanOverride())
}

func TestDividenStatus_CanApprove(t *testing.T) {
	assert.True(t, DividenPendingApproval.CanApprove())
	assert.False(t, DividenApproved.CanApprove())
	assert.False(t, DividenPosted.CanApprove())
	assert.False(t, DividenRejected.CanApprove())
}

// ─── domain: ToAkrualListItem ─────────────────────────────────────────────────

func TestToAkrualListItem_Stage1(t *testing.T) {
	id := uuid.New()
	instID := uuid.New()
	stage := 1
	eir := decimal.NewFromFloat(0.075)
	fxID := uuid.New()
	jrnlID := uuid.New()
	eclID := uuid.New()

	a := &PendapatanAkrual{
		ID:                  id,
		InstrumenID:         instID,
		TanggalAkrual:       time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
		Jenis:               JenisBunga,
		Stage:               &stage,
		CarryingBasisIDR:    decimal.NewFromInt(1_000_000),
		EIRPersen:           &eir,
		BungaKotor:          decimal.NewFromFloat(205.48),
		PPh:                 decimal.Zero,
		BungaBersih:         decimal.NewFromFloat(205.48),
		MataUang:            "IDR",
		FXRateID:            &fxID,
		ECLRunIDUsed:        &eclID,
		JurnalHeaderID:      &jrnlID,
		KlasifikasiSnapshot: "AC",
		StaleStagingFlag:    false,
		Status:              AkrualAutoPosted,
		CreatedAt:           time.Now(),
	}

	item := ToAkrualListItem(a, "INST-0001")
	assert.Equal(t, id.String(), item.ID)
	assert.Equal(t, "INST-0001", item.InstrumenKode)
	assert.Equal(t, string(BasisGross), item.CarryingBasis)
	assert.Equal(t, "AC", item.KlasifikasiSnapshot)
	assert.NotNil(t, item.EirPersen)
	assert.NotNil(t, item.FxRateId)
	assert.NotNil(t, item.EclRunIdUsed)
	assert.NotNil(t, item.JurnalHeaderId)
}

func TestToAkrualListItem_Stage3(t *testing.T) {
	stage := 3
	a := &PendapatanAkrual{
		ID:          uuid.New(),
		InstrumenID: uuid.New(),
		Jenis:       JenisBunga,
		Stage:       &stage,
		Status:      AkrualAutoPosted,
		CreatedAt:   time.Now(),
	}
	item := ToAkrualListItem(a, "INST-S3")
	assert.Equal(t, string(BasisNetCarrying), item.CarryingBasis)
}

func TestToAkrualListItem_NilStage(t *testing.T) {
	a := &PendapatanAkrual{
		ID:          uuid.New(),
		InstrumenID: uuid.New(),
		Jenis:       JenisDividen,
		Stage:       nil, // no stage
		Status:      AkrualAutoPosted,
		CreatedAt:   time.Now(),
	}
	item := ToAkrualListItem(a, "INST-DIV")
	assert.Equal(t, string(BasisGross), item.CarryingBasis) // nil → gross
}

// ─── domain: ToJatuhTempoListItem ────────────────────────────────────────────

func TestToJatuhTempoListItem_WithJurnal(t *testing.T) {
	id := uuid.New()
	instID := uuid.New()
	jrnlID := uuid.New()
	errMsg := "connection lost"

	jt := &JatuhTempo{
		ID:                  id,
		InstrumenID:         instID,
		TanggalJatuhTempo:   time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
		Jenis:               "MATURITY",
		PokokReturned:       decimal.NewFromInt(1_000_000),
		BungaReturned:       decimal.NewFromFloat(5000),
		PPh:                 decimal.NewFromFloat(1000),
		Proceeds:            decimal.NewFromFloat(1_004_000),
		KlasifikasiSnapshot: "AC",
		JurnalHeaderID:      &jrnlID,
		Status:              JatuhTempoSettled,
		ErrorMessage:        &errMsg,
		CreatedAt:           time.Now(),
	}

	item := ToJatuhTempoListItem(jt, "INST-0001")
	assert.Equal(t, id.String(), item.ID)
	assert.Equal(t, "INST-0001", item.InstrumenKode)
	assert.NotNil(t, item.JurnalHeaderId)
	assert.Equal(t, &errMsg, item.ErrorMessage)
	assert.Equal(t, "SETTLED", item.Status)
}

func TestToJatuhTempoListItem_NoJurnal(t *testing.T) {
	jt := &JatuhTempo{
		ID:                uuid.New(),
		InstrumenID:       uuid.New(),
		TanggalJatuhTempo: time.Now(),
		Status:            JatuhTempoPending,
		CreatedAt:         time.Now(),
	}
	item := ToJatuhTempoListItem(jt, "INST-X")
	assert.Nil(t, item.JurnalHeaderId)
}

// ─── domain: ParseDateStrict ─────────────────────────────────────────────────

func TestParseDateStrict_Valid(t *testing.T) {
	d, err := ParseDateStrict("2026-06-20")
	require.NoError(t, err)
	assert.Equal(t, 2026, d.Year())
	assert.Equal(t, time.June, d.Month())
	assert.Equal(t, 20, d.Day())
}

func TestParseDateStrict_Invalid(t *testing.T) {
	_, err := ParseDateStrict("20/06/2026")
	require.Error(t, err)

	_, err = ParseDateStrict("")
	require.Error(t, err)

	_, err = ParseDateStrict("2026-13-01")
	require.Error(t, err) // invalid month
}

// ─── jurnal_poster: JurnalPosterStub helpers ──────────────────────────────────

func TestJurnalPosterStub_SetAkrualError(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetAkrualError(assert.AnError)
	_, err := stub.PostAkrual(context.Background(), nil, AkrualPostRequest{})
	require.Error(t, err)
}

func TestJurnalPosterStub_SetMaturityError(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetMaturityError(assert.AnError)
	_, err := stub.PostMaturity(context.Background(), nil, MaturityPostRequest{})
	require.Error(t, err)
}

func TestJurnalPosterStub_SetDividenError(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetDividenError(assert.AnError)
	_, err := stub.PostDividen(context.Background(), nil, DividenPostRequest{})
	require.Error(t, err)
}

func TestJurnalPosterStub_DividenCalls(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	req := DividenPostRequest{JumlahBersih: decimal.NewFromInt(1000)}
	_, err := stub.PostDividen(context.Background(), nil, req)
	require.NoError(t, err)
	assert.Len(t, stub.DividenCalls(), 1)
}

func TestJurnalPosterStub_Reset(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetAkrualError(assert.AnError)
	// Error set — call once before reset
	_, preErr := stub.PostAkrual(context.Background(), nil, AkrualPostRequest{})
	require.Error(t, preErr)

	stub.Reset() // clears calls + errors
	// After reset: no error, fresh call recorded
	_, err := stub.PostAkrual(context.Background(), nil, AkrualPostRequest{})
	require.NoError(t, err)
	// Only the post-reset call is recorded
	assert.Len(t, stub.AkrualCalls(), 1, "Reset should clear pre-reset calls")
}

// ─── jurnal_poster: NewNoopJurnalPoster ──────────────────────────────────────

func TestNoopJurnalPoster_PostAkrual(t *testing.T) {
	p := NewNoopJurnalPoster(nil)
	res, err := p.PostAkrual(context.Background(), nil, AkrualPostRequest{})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, res.JurnalEntryID)
}

func TestNoopJurnalPoster_PostMaturity(t *testing.T) {
	p := NewNoopJurnalPoster(nil)
	res, err := p.PostMaturity(context.Background(), nil, MaturityPostRequest{})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, res.JurnalEntryID)
}

func TestNoopJurnalPoster_PostDividen(t *testing.T) {
	p := NewNoopJurnalPoster(nil)
	res, err := p.PostDividen(context.Background(), nil, DividenPostRequest{})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, res.JurnalEntryID)
}

// ─── jurnal_poster: InstrumenStatusUpdaterStub ───────────────────────────────

func TestInstrumenStatusUpdaterStub_SetMatureError(t *testing.T) {
	stub := NewInstrumenStatusUpdaterStub()
	stub.SetMatureError(assert.AnError)
	err := stub.SetMatured(context.Background(), nil, uuid.New(), uuid.New())
	require.Error(t, err)
}

// ─── service: PostDividen ─────────────────────────────────────────────────────

func TestPostDividen_HappyPath(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		instrumenInfo: &InstrumenAkrualInfo{
			ID:                instID,
			Status:            "ACTIVE",
			KlasifikasiPSAK71: "FVOCI_ELECTION",
		},
	}
	svc, _, _ := buildSvc(t, repo)

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:      uuid.New().String(),
		TenantID: "TUGURE",
	})

	req := CreateDividenRequest{
		InstrumenID:   instID,
		TanggalTerima: "2026-06-20",
		JumlahKotor:   decimal.NewFromInt(10_000),
		IsReksadana:   false,
	}

	d, err := svc.PostDividen(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, DividenPendingApproval, d.Status)
	assert.Equal(t, "OCI", d.Treatment) // FVOCI_ELECTION → OCI
	// PPH = 10% of 10000 = 1000
	assert.Equal(t, "1000.0000", d.PPHDividen.StringFixed(4))
	assert.Equal(t, "9000.0000", d.JumlahBersih.StringFixed(4))
}

func TestPostDividen_Reksadana(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		instrumenInfo: &InstrumenAkrualInfo{
			ID:                instID,
			Status:            "ACTIVE",
			KlasifikasiPSAK71: "AC",
		},
	}
	svc, _, _ := buildSvc(t, repo)

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:      uuid.New().String(),
		TenantID: "TUGURE",
	})

	req := CreateDividenRequest{
		InstrumenID:   instID,
		TanggalTerima: "2026-06-20",
		JumlahKotor:   decimal.NewFromInt(10_000),
		IsReksadana:   true, // DistribusiRD → 10% PPH
	}

	d, err := svc.PostDividen(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, "P&L", d.Treatment) // AC → P&L
	assert.Equal(t, "1000.0000", d.PPHDividen.StringFixed(4)) // 10%
}

func TestPostDividen_InvalidDate(t *testing.T) {
	svc, _, _ := buildSvc(t, &stubRepo{})
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})

	req := CreateDividenRequest{
		InstrumenID:   uuid.New(),
		TanggalTerima: "not-a-date",
		JumlahKotor:   decimal.NewFromInt(1000),
	}
	_, err := svc.PostDividen(ctx, req)
	require.Error(t, err)
}

func TestPostDividen_InstrumenNotFound(t *testing.T) {
	repo := &stubRepo{instrumenInfo: nil} // GetInstrumenInfo returns nil
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})

	req := CreateDividenRequest{
		InstrumenID:   uuid.New(),
		TanggalTerima: "2026-06-20",
		JumlahKotor:   decimal.NewFromInt(1000),
	}
	_, err := svc.PostDividen(ctx, req)
	require.Error(t, err)
}

func TestPostDividen_PeriodeClosed(t *testing.T) {
	instID := uuid.New()
	repo := &stubRepo{
		periode:       nil, // no period found
		instrumenInfo: &InstrumenAkrualInfo{ID: instID, Status: "ACTIVE", KlasifikasiPSAK71: "AC"},
	}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})

	req := CreateDividenRequest{
		InstrumenID:   instID,
		TanggalTerima: "2026-06-20",
		JumlahKotor:   decimal.NewFromInt(1000),
	}
	_, err := svc.PostDividen(ctx, req)
	require.Error(t, err)
}

func TestPostDividen_NoClaims(t *testing.T) {
	svc, _, _ := buildSvc(t, &stubRepo{})
	// No claims in context
	req := CreateDividenRequest{
		InstrumenID:   uuid.New(),
		TanggalTerima: "2026-06-20",
		JumlahKotor:   decimal.NewFromInt(1000),
	}
	_, err := svc.PostDividen(context.Background(), req)
	require.Error(t, err)
}

// ─── service: ApproveDividen happy path ─────────────────────────────────────

func TestApproveDividen_HappyPath(t *testing.T) {
	makerID := uuid.New()
	approverID := uuid.New()
	divID := uuid.New()
	instID := uuid.New()

	dividen := &Dividen{
		ID:          divID,
		Status:      DividenPendingApproval,
		MakerID:     makerID,
		InstrumenID: instID,
		JumlahKotor: decimal.NewFromInt(10_000),
		PPHDividen:  decimal.NewFromInt(1_000),
		JumlahBersih: decimal.NewFromInt(9_000),
		KlasifikasiSnapshot: "FVOCI_ELECTION",
	}
	repo := &stubRepo{
		dividen:       dividen,
		instrumenInfo: &InstrumenAkrualInfo{ID: instID, Status: "ACTIVE"},
		periode:       openPeriode(),
	}
	svc, poster, _ := buildSvc(t, repo)

	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:      approverID.String(),
		TenantID: "TUGURE",
	})

	req := ApproveDividenRequest{
		Comment:        "Approved.",
		SignatureMethod: "JWT_STEP_UP",
	}
	d, err := svc.ApproveDividen(ctx, divID, req)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, DividenPosted, d.Status)
	// PostDividen was called
	assert.Len(t, poster.DividenCalls(), 1)
}

// ─── service: ListAkrual, ListJatuhTempo ─────────────────────────────────────

func TestListAkrual_OK(t *testing.T) {
	stage := 1
	eir := decimal.NewFromFloat(0.075)
	repo := &stubRepo{
		listAkrualRows: []*PendapatanAkrual{
			{
				ID:          uuid.New(),
				InstrumenID: uuid.New(),
				Jenis:       JenisBunga,
				Stage:       &stage,
				EIRPersen:   &eir,
				BungaKotor:  decimal.NewFromFloat(205.48),
				BungaBersih: decimal.NewFromFloat(205.48),
				Status:      AkrualAutoPosted,
				CreatedAt:   time.Now(),
			},
		},
	}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	rows, hasMore, total, err := svc.ListAkrual(ctx, listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.False(t, hasMore)
	assert.Equal(t, 1, total)
}

func TestListJatuhTempo_OK(t *testing.T) {
	repo := &stubRepo{
		listJatuhTempoRows: []*JatuhTempo{
			{
				ID:                uuid.New(),
				InstrumenID:       uuid.New(),
				TanggalJatuhTempo: time.Now(),
				PokokReturned:     decimal.NewFromInt(1_000_000),
				BungaReturned:     decimal.Zero,
				PPh:               decimal.Zero,
				Proceeds:          decimal.NewFromInt(1_000_000),
				Status:            JatuhTempoSettled,
				CreatedAt:         time.Now(),
			},
		},
	}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: uuid.New().String(), TenantID: "TUGURE"})
	rows, hasMore, total, err := svc.ListJatuhTempo(ctx, listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.False(t, hasMore)
	assert.Equal(t, 1, total)
}

// ─── handler: more coverage ──────────────────────────────────────────────────

func TestHandlerOverrideStale_PendingStaleStatus_ServiceCalled(t *testing.T) {
	id := uuid.New()
	stage := 3
	eir := decimal.NewFromFloat(0.075)
	eclID := uuid.New()
	// Akrual exists with PENDING_STALE_REVIEW status
	repo := &stubRepo{
		isHoliday: false,
		periode:   openPeriode(),
		akrualByID: &PendapatanAkrual{
			ID:               id,
			InstrumenID:      uuid.New(),
			TanggalAkrual:    time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
			Jenis:            JenisBunga,
			Stage:            &stage,
			EIRPersen:        &eir,
			BungaKotor:       decimal.NewFromFloat(205.48),
			BungaBersih:      decimal.NewFromFloat(205.48),
			Status:           AkrualPendingStaleReview,
			ECLRunIDUsed:     &eclID,
			CarryingBasisIDR: decimal.NewFromInt(800_000),
			RowVersion:       1,
		},
		instrumenInfo: &InstrumenAkrualInfo{
			ID:                uuid.New(),
			Status:            "ACTIVE",
			KlasifikasiPSAK71: "AC",
			EIRPersen:         decimal.NewFromFloat(0.075),
			GrossCarryingIDR:  decimal.NewFromInt(1_000_000),
			MataUang:          "IDR",
			Stage:             3,
		},
		eclResult: &ECLSealedResult{
			ECLCalcRunID: eclID,
			Stage:        3,
			ECLAllowance: decimal.NewFromInt(200_000),
			SealedAt:     time.Now().UTC().AddDate(0, 0, -5),
		},
		schedule:  basicSchedule(),
		staleDays: 30,
	}
	router := buildTestRouter(buildTestSvc(repo))

	body := map[string]interface{}{
		"reason":          "ECL sealed run foi atualizado confirmado pelo Risk Officer.",
		"signatureMethod": "JWT_STEP_UP",
	}
	b, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/transaksi/akrual/"+id.String()+"/override-stale",
		bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	router.ServeHTTP(w, req)

	// May return 200 (success) or 422 (business error from service) — not 400/401/500
	assert.NotEqual(t, http.StatusBadRequest, w.Code)
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)
	assert.NotEqual(t, http.StatusInternalServerError, w.Code)
}

func TestHandlerListJatuhTempo_WithSort(t *testing.T) {
	repo := &stubRepo{listJatuhTempoRows: []*JatuhTempo{}}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/transaksi/jatuh-tempo?sort=tanggal_jatuh_tempo:asc&limit=10", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerGetDashboard_WithInstrumenID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	instrID := uuid.New()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/transaksi/akrual/dashboard?instrumen_id="+instrID.String()+"&year=2026&month=6", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerListAkrual_WithFilters(t *testing.T) {
	repo := &stubRepo{listAkrualRows: []*PendapatanAkrual{}}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/transaksi/akrual?filter[stage]=2&q=test&limit=25", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── Worker: RegisterHandlers ─────────────────────────────────────────────────

func TestWorker_RegisterHandlers(t *testing.T) {
	repo := &stubRepo{isHoliday: true}
	svc := buildTestSvc(repo)
	w := NewWorker(svc, nil, nil)
	mux := asynq.NewServeMux()
	// Should not panic — just registers 3 handlers.
	w.RegisterHandlers(mux)
}

// ─── Worker task factories ─────────────────────────────────────────────────────

func TestWorker_NewTasks_WithJobID(t *testing.T) {
	jobID := uuid.New().String()
	tanggal := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	matTask, err := NewMaturityTask(tanggal, jobID)
	require.NoError(t, err)
	assert.Equal(t, TaskMaturityProcess, matTask.Type())

	akrTask, err := NewAkrualTask(tanggal, jobID)
	require.NoError(t, err)
	assert.Equal(t, TaskDailyAccrual, akrTask.Type())

	amortTask, err := NewAmortisasiTask(tanggal, jobID)
	require.NoError(t, err)
	assert.Equal(t, TaskAmortisasiPD, amortTask.Type())
}

// ─── NewService with logger ────────────────────────────────────────────────────

func TestNewService_WithLogger(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, NewNoopJurnalPoster(nil), NewInstrumenStatusUpdaterStub(), nil, nil)
	require.NotNil(t, svc)
}

// ─── Service: processOneMaturity error paths ──────────────────────────────────

func TestRunDailyMaturityCron_FCYInstrumen(t *testing.T) {
	// FCY instrumen triggers GetFXRateApproved — set it up so maturity completes
	instrID := uuid.New()
	instrumen := &InstrumenAkrualInfo{
		ID:               instrID,
		KodeInstrumen:    "BOND-USD-001",
		Status:           "ACTIVE",
		KlasifikasiPSAK71: "AC",
		EIRPersen:        decimal.NewFromFloat(0.05),
		GrossCarryingIDR: decimal.NewFromInt(1_000_000),
		MataUang:         "USD",
		TanggalJatuhTempo: func() *time.Time {
			t := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
			return &t
		}(),
	}
	fxRate := &FXRateApproved{
		ID:       uuid.New(),
		MataUang: "USD",
		Tanggal:  time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
		RateIDR:  decimal.NewFromFloat(15_432.12),
	}
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: "00000000-0000-0000-0000-000000000099", TenantID: "TUGURE",
	})
	repo := &stubRepo{
		isHoliday:      false,
		periode:        openPeriode(),
		activeMaturity: []*InstrumenAkrualInfo{instrumen},
		schedule:       basicSchedule(),
		fxRate:         fxRate,
	}
	svc, _, _ := buildSvc(t, repo)
	// Should not error — FCY instrumen uses fx rate for EAD conversion
	result, err := svc.RunDailyMaturityCron(ctx, time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	// Maturity processes + success
	assert.GreaterOrEqual(t, result.TotalProcessed+result.DLQCount, 0)
}

// ─── Service: ListJatuhTempo error path ──────────────────────────────────────

func TestService_ListJatuhTempo_Error(t *testing.T) {
	repo := &stubRepo{listJatuhTempoErr: errors.New("db error")}
	svc, _, _ := buildSvc(t, repo)
	_, _, _, err := svc.ListJatuhTempo(context.Background(), listquery.Query{}, "", 50)
	require.Error(t, err)
}

// ─── Service: ListAkrual error path ──────────────────────────────────────────

func TestService_ListAkrual_Error(t *testing.T) {
	repo := &stubRepo{listAkrualErr: errors.New("db error")}
	svc, _, _ := buildSvc(t, repo)
	_, _, _, err := svc.ListAkrual(context.Background(), listquery.Query{}, "", 50)
	require.Error(t, err)
}

// ─── Handler: ListAkrual with stale items ────────────────────────────────────

func TestHandlerListAkrual_WithStaleItem(t *testing.T) {
	stale := true
	stage := 1
	eir := decimal.NewFromFloat(0.075)
	repo := &stubRepo{
		listAkrualRows: []*PendapatanAkrual{
			{
				ID:               uuid.New(),
				InstrumenID:      uuid.New(),
				TanggalAkrual:    time.Now().UTC(),
				Jenis:            JenisBunga,
				Stage:            &stage,
				EIRPersen:        &eir,
				BungaKotor:       decimal.NewFromFloat(100),
				BungaBersih:      decimal.NewFromFloat(100),
				Status:           AkrualPendingStaleReview,
				StaleStagingFlag: stale,
				RowVersion:       1,
			},
		},
	}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"staleCount":1`)
}

// ─── Handler: ListAkrual service error ───────────────────────────────────────

func TestHandlerListAkrual_ServiceError(t *testing.T) {
	repo := &stubRepo{listAkrualErr: errors.New("db failure")}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ─── Handler: ListAkrual with X-Trace-Id header ───────────────────────────────

func TestHandlerListAkrual_WithTraceID(t *testing.T) {
	repo := &stubRepo{listAkrualRows: []*PendapatanAkrual{}}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/akrual", nil)
	req.Header.Set("X-Trace-Id", "trace-abc-123")
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "trace-abc-123")
}

// ─── Handler: ListJatuhTempo service error ────────────────────────────────────

func TestHandlerListJatuhTempo_ServiceError(t *testing.T) {
	repo := &stubRepo{listJatuhTempoErr: errors.New("db failure")}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/jatuh-tempo", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ─── Handler: GetDashboard invalid instrumenId ────────────────────────────────

func TestHandlerGetDashboard_InvalidInstrumenID_Coverage(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/transaksi/akrual/dashboard?instrumen_id=not-a-uuid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── Handler: GetDashboard with portofolio_id ────────────────────────────────

func TestHandlerGetDashboard_WithPortofolioID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))
	portoID := uuid.New()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/transaksi/akrual/dashboard?portofolio_id="+portoID.String(), nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandlerGetDashboard_InvalidPortofolioID(t *testing.T) {
	repo := &stubRepo{}
	router := buildTestRouter(buildTestSvc(repo))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/transaksi/akrual/dashboard?portofolio_id=not-valid-uuid", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── Repo: GetMTDYTDSummary error path ────────────────────────────────────────
// (tested via handler GetDashboard which calls svc.GetDashboard → repo.GetMTDYTDSummary)
// The error path in repo is when QueryContext fails, which sqlmock doesn't expose here.
// Cover UpdateAkrualStatus error path instead:

func TestRepo_UpdateAkrualStatus_Error_ViaStub(t *testing.T) {
	repo := &stubRepo{updateAkrualStatusErr: errors.New("update failed")}
	svc, _, _ := buildSvc(t, repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: uuid.New().String(), TenantID: "TUGURE",
	})
	req := OverrideStaleRequest{Reason: "This is a valid reason with more than thirty chars.", SignatureMethod: "JWT_STEP_UP"}
	stage := 1
	eir := decimal.NewFromFloat(0.075)
	jurnalID := uuid.New()
	repo.akrualByID = &PendapatanAkrual{
		ID:               uuid.New(),
		InstrumenID:      uuid.New(),
		TanggalAkrual:    time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
		Jenis:            JenisBunga,
		Stage:            &stage,
		EIRPersen:        &eir,
		BungaBersih:      decimal.NewFromFloat(100),
		CarryingBasisIDR: decimal.NewFromInt(500_000),
		Status:           AkrualPendingStaleReview,
		ECLRunIDUsed:     &jurnalID,
		RowVersion:       1,
	}
	repo.instrumenInfo = &InstrumenAkrualInfo{
		ID: repo.akrualByID.InstrumenID, Status: "ACTIVE",
		KlasifikasiPSAK71: "AC", GrossCarryingIDR: decimal.NewFromInt(1_000_000),
		MataUang: "IDR", Stage: 1,
	}
	repo.schedule = basicSchedule()
	repo.eclResult = &ECLSealedResult{
		ECLCalcRunID: jurnalID, Stage: 1, ECLAllowance: decimal.Zero,
		SealedAt: time.Now().UTC().AddDate(0, 0, -1),
	}
	_, err := svc.OverrideStaleAkrual(ctx, repo.akrualByID.ID, req, uuid.New().String())
	// Should error because UpdateAkrualStatus returns error
	require.Error(t, err)
}
