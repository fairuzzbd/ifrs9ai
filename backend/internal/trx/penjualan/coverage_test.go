package penjualan

// coverage_test.go — Extra tests to push total coverage to ≥85%.
//
// Targets:
//   - IsValidKlasifikasi (domain.go)
//   - Stub helper methods: SetResult, Reset, Calls, IsNoopProduction (jurnal_poster.go)
//   - NoopJurnalPoster.Post, NewNoopJurnalPoster
//   - InstrumenUpdaterStub error paths
//   - RiskNotifierStub SetError / Calls
//   - ToListItem / ToDetail with all optional pointer fields populated
//   - ToPreviewResponse with OCIRecycled + BMFreqImpactPct non-nil
//   - RegisterRoutes (routes.go)
//   - Service: FVTPL/POCI create, more Approve/Reject error branches
//   - Handler: GetPreview invalid UUID + not-found

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── IsValidKlasifikasi ───────────────────────────────────────────────────────

func TestIsValidKlasifikasi_Valid(t *testing.T) {
	for _, k := range []string{"AC", "FVOCI", "FVOCI_ELECTION", "FVTPL", "POCI"} {
		assert.Truef(t, IsValidKlasifikasi(k), "expected true for %s", k)
	}
}

func TestIsValidKlasifikasi_Invalid(t *testing.T) {
	for _, k := range []string{"", "UNKNOWN", "ac", "fvoci", "DEPOSITO"} {
		assert.Falsef(t, IsValidKlasifikasi(k), "expected false for %s", k)
	}
}

// ─── JurnalPosterStub helper methods ─────────────────────────────────────────

func TestJurnalPosterStub_SetResult(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	custom := PenjualanPostResult{
		JurnalEntryID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		EventCodes:    []string{"CUSTOM_EVENT"},
	}
	stub.SetResult(custom)
	result, err := stub.Post(context.TODO(), nil, PenjualanPostRequest{ProceedIDR: decimal.NewFromInt(1000)})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JurnalEntryID)
	assert.Len(t, stub.Calls(), 1)
}

func TestJurnalPosterStub_Reset(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetError(assert.AnError)
	// Verify error is set
	_, err := stub.Post(context.TODO(), nil, PenjualanPostRequest{ProceedIDR: decimal.NewFromInt(1)})
	require.Error(t, err)
	// Reset clears error and calls
	stub.Reset()
	_, err = stub.Post(context.TODO(), nil, PenjualanPostRequest{ProceedIDR: decimal.NewFromInt(1)})
	require.NoError(t, err, "after Reset error should be cleared")
	// Only 1 call recorded (the one after Reset)
	assert.Len(t, stub.Calls(), 1)
}

func TestJurnalPosterStub_SetError(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetError(assert.AnError)
	_, err := stub.Post(context.TODO(), nil, PenjualanPostRequest{ProceedIDR: decimal.NewFromInt(1)})
	require.Error(t, err)
}

// ─── NoopJurnalPoster ─────────────────────────────────────────────────────────

func TestNoopJurnalPoster_Post(t *testing.T) {
	noop := NewNoopJurnalPoster(nil)
	result, err := noop.Post(context.TODO(), nil, PenjualanPostRequest{
		EventCodes:  []string{"PENJUALAN_AC"},
		ProceedIDR:  decimal.NewFromInt(500000),
		PenjualanID: uuid.New(),
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JurnalEntryID)
}

func TestIsNoopProduction_NotProd(t *testing.T) {
	orig := os.Getenv("APP_ENV")
	_ = os.Unsetenv("APP_ENV")
	defer func() { _ = os.Setenv("APP_ENV", orig) }()

	noop := NewNoopJurnalPoster(nil)
	assert.False(t, IsNoopProduction(noop))
}

func TestIsNoopProduction_Prod_Noop(t *testing.T) {
	orig := os.Getenv("APP_ENV")
	_ = os.Setenv("APP_ENV", "production")
	defer func() { _ = os.Setenv("APP_ENV", orig) }()

	noop := NewNoopJurnalPoster(nil)
	assert.True(t, IsNoopProduction(noop))
}

func TestIsNoopProduction_Prod_NotNoop(t *testing.T) {
	orig := os.Getenv("APP_ENV")
	_ = os.Setenv("APP_ENV", "production")
	defer func() { _ = os.Setenv("APP_ENV", orig) }()

	stub := NewJurnalPosterStub(nil)
	assert.False(t, IsNoopProduction(stub))
}

// ─── InstrumenUpdaterStub error paths ────────────────────────────────────────

func TestInstrumenUpdaterStub_QtyError(t *testing.T) {
	s := NewInstrumenUpdaterStub()
	s.SetQtyError(assert.AnError)
	_, err := s.UpdateQty(context.TODO(), nil, uuid.New(), decimal.NewFromInt(100), uuid.New())
	require.Error(t, err)
	assert.Equal(t, 1, s.QtyCalls())
}

func TestInstrumenUpdaterStub_DisposeError(t *testing.T) {
	s := NewInstrumenUpdaterStub()
	s.SetDisposeError(assert.AnError)
	err := s.SetDisposed(context.TODO(), nil, uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Equal(t, 1, s.DisposeCalls())
}

// ─── RiskNotifierStub ────────────────────────────────────────────────────────

func TestRiskNotifierStub_SetError(t *testing.T) {
	s := NewRiskNotifierStub()
	s.SetError(assert.AnError)
	err := s.NotifyBMViolation(context.TODO(), uuid.New(), uuid.New(), decimal.NewFromInt(6), false)
	require.Error(t, err)
	assert.Equal(t, 1, s.Calls())
}

func TestRiskNotifierStub_NoError(t *testing.T) {
	s := NewRiskNotifierStub()
	err := s.NotifyBMViolation(context.TODO(), uuid.New(), uuid.New(), decimal.NewFromInt(6), false)
	require.NoError(t, err)
	assert.Equal(t, 1, s.Calls())
}

// ─── ToListItem with all optional pointer fields ──────────────────────────────

func TestToListItem_WithAllPointers(t *testing.T) {
	approverID := uuid.New()
	jurnalID := uuid.New()
	qtyPost := decimal.NewFromInt(500)

	pj := &Penjualan{
		ID:                  uuid.New(),
		InstrumenID:         uuid.New(),
		JenisDisposal:       DisposalFull,
		QtyTerjual:          decimal.NewFromInt(1000),
		QtyHoldingPre:       decimal.NewFromInt(1000),
		QtyHoldingPost:      &qtyPost,
		HargaJualPerUnit:    decimal.NewFromInt(1100),
		Proceed:             decimal.NewFromInt(1100000),
		CostBasis:           decimal.NewFromInt(1000000),
		RealizedGL:          decimal.NewFromInt(100000),
		KlasifikasiSnapshot: KlasifikasiFVOCI,
		Status:              StatusPosted,
		TanggalEksekusi:     time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		MakerID:             testMakerID,
		ApproverID:          &approverID,
		JurnalHeaderID:      &jurnalID,
		BMViolationRisk:     true,
		CreatedAt:           time.Now().UTC(),
		RowVersion:          2,
		TenantID:            "TUGURE",
	}
	li := ToListItem(pj, "OBL-COVER-001")

	require.NotNil(t, li.QtyHoldingPost)
	require.NotNil(t, li.ApproverID)
	require.NotNil(t, li.JurnalHeaderID)
	assert.Equal(t, approverID.String(), *li.ApproverID)
	assert.Equal(t, jurnalID.String(), *li.JurnalHeaderID)
	assert.True(t, li.BMViolationRisk)
}

// ─── ToDetail with all optional pointer fields ────────────────────────────────

func TestToDetail_WithAllOptionalPointers(t *testing.T) {
	ociCum := decimal.NewFromInt(50000)
	ociRec := decimal.NewFromInt(25000)
	bmPct := decimal.NewFromFloat(6.5)
	jurnalEvent := "PENJUALAN_FVOCI_DEBT,REKLAS_OCI_PL"
	periodeID := uuid.New()

	pj := &Penjualan{
		ID:                  uuid.New(),
		InstrumenID:         uuid.New(),
		JenisDisposal:       DisposalPartial,
		QtyTerjual:          decimal.NewFromInt(500),
		QtyHoldingPre:       decimal.NewFromInt(1000),
		HargaJualPerUnit:    decimal.NewFromInt(1100),
		Proceed:             decimal.NewFromInt(550000),
		CostBasis:           decimal.NewFromInt(490000),
		RealizedGL:          decimal.NewFromInt(60000),
		OCIRecycled:         &ociRec,
		OCICumulativeTotal:  &ociCum,
		KlasifikasiSnapshot: KlasifikasiFVOCI,
		JurnalEventCode:     &jurnalEvent,
		Status:              StatusPosted,
		TanggalEksekusi:     time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		MakerID:             testMakerID,
		BMViolationRisk:     true,
		BMViolationPct:      &bmPct,
		PeriodeBulananID:    &periodeID,
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		RowVersion:          3,
		TenantID:            "TUGURE",
	}
	preview := PreviewResult{
		KlasifikasiPSAK71: KlasifikasiFVOCI,
		ProceedIDR:        decimal.NewFromInt(550000),
		CostBasis:         decimal.NewFromInt(490000),
		RealizedGL:        decimal.NewFromInt(60000),
		OCIRecycled:       &ociRec,
	}
	d := ToDetail(pj, "OBL-COVER-002", preview)
	assert.NotNil(t, d.OciRecycled)
	assert.NotNil(t, d.OciCumulativeTotal)
	assert.NotNil(t, d.BMViolationPct)
	assert.NotNil(t, d.PeriodeBulananID)
}

// ─── ToPreviewResponse with both optional fields ──────────────────────────────

func TestToPreviewResponse_WithOptionalFields(t *testing.T) {
	oci := decimal.NewFromInt(12345)
	bmpct := decimal.NewFromFloat(7.5)
	note := "FVOCI_ELECTION: no OCI recycling per §B5.7.1"
	warn := "BM frequency approaching warn threshold"

	pr := ToPreviewResponse(PreviewResult{
		KlasifikasiPSAK71: KlasifikasiFVOCI,
		ProceedIDR:        decimal.NewFromInt(550000),
		CostBasis:         decimal.NewFromInt(490000),
		RealizedGL:        decimal.NewFromInt(60000),
		OCIRecycled:       &oci,
		NoRecyclingNote:   &note,
		BMFreqImpactPct:   &bmpct,
		BMFreqWarning:     &warn,
	})
	require.NotNil(t, pr.OciRecycled)
	require.NotNil(t, pr.BmFreqImpactPct)
	require.NotNil(t, pr.NoRecyclingNote)
	require.NotNil(t, pr.BmFreqWarning)
}

// ─── Service: FVTPL / POCI create happy path ─────────────────────────────────

func TestCreatePenjualan_FVTPL_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiFVTPL)
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	resp, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("FVTPL"))
	require.NoError(t, err)
	assert.Equal(t, string(StatusPendingApproval), resp.Status)
	assert.Equal(t, "FVTPL", resp.Preview.KlasifikasiPsak71)
	assert.Nil(t, resp.Preview.OciRecycled)
}

func TestCreatePenjualan_POCI_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiPOCI)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(200000000)

	svc := newTestService(repo)
	resp, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("POCI"))
	require.NoError(t, err)
	assert.Equal(t, string(StatusPendingApproval), resp.Status)
}

// ─── Service: Approve additional branches ────────────────────────────────────

func TestApprove_FVTPL_NoOCI(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiFVTPL, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiFVTPL)
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	resp, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err)
	assert.Equal(t, string(StatusPosted), resp.Status)
	assert.Nil(t, resp.OCIRecycled)
}

func TestApprove_GetByID_Err(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.getByIDErr = assert.AnError

	svc := newTestService(repo)
	_, err := svc.Approve(ctxWithApprover(), uuid.New(), defaultApproveReq())
	require.Error(t, err)
}

func TestApprove_WrongStatus(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	pj.Status = StatusPosted
	repo.penjualan = pj

	svc := newTestService(repo)
	_, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.Error(t, err)
}

// TestApprove_SoDViaApprover2 covers the SoD block where maker_id == approver_id.
// (maker set to testApproverID; approver context also testApproverID → SoD violation)
func TestApprove_SoDViaApprover2(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testApproverID, KlasifikasiAC, DisposalPartial) // maker = approver
	repo.penjualan = pj
	svc := newTestService(repo)
	_, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.Error(t, err, "maker == approver must be SoD violation")
}

// ─── Service: Reject branches ─────────────────────────────────────────────────

func TestReject_GetByID_Err(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.getByIDErr = assert.AnError

	svc := newTestService(repo)
	_, err := svc.Reject(ctxWithApprover(), uuid.New(), RejectPenjualanRequest{
		Reason:          "Penolakan dengan alasan yang panjang melebihi tiga puluh karakter untuk test.",
		SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err)
}

// TestReject_SoDViaRejector covers the SoD check path where maker tries to reject own penjualan.
func TestReject_SoDViaRejector(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testApproverID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	svc := newTestService(repo)
	_, err := svc.Reject(ctxWithApprover(), pj.ID, RejectPenjualanRequest{
		Reason:          "Penolakan dengan alasan yang panjang melebihi tiga puluh karakter untuk test.",
		SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err, "maker == rejector must be SoD violation")
}

// ─── Service: Create validation shortcircuits ────────────────────────────────

func TestCreatePenjualan_InvalidDisposalType_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	req := defaultCreateReq("AC")
	req.JenisDisposal = "INVALID"
	_, err := svc.CreatePenjualan(ctxWithMaker(), req)
	require.Error(t, err)
}

func TestCreatePenjualan_ZeroQty_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	req := defaultCreateReq("AC")
	req.QtyTerjual = decimal.Zero
	_, err := svc.CreatePenjualan(ctxWithMaker(), req)
	require.Error(t, err)
}

func TestCreatePenjualan_ZeroHarga_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	req := defaultCreateReq("AC")
	req.HargaJualPerUnit = decimal.Zero
	_, err := svc.CreatePenjualan(ctxWithMaker(), req)
	require.Error(t, err)
}

func TestCreatePenjualan_GetInstErr_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.getInstErr = assert.AnError
	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
}

func TestCreatePenjualan_HasActiveErr_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.hasActiveErr = assert.AnError
	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
}

func TestCreatePenjualan_PeriodeErr_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.getPeriodeErr = assert.AnError
	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
}

// ─── Service: GetPreview not-found ───────────────────────────────────────────

func TestGetPreview_NotFound_Cov(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.penjualan = nil
	svc := newTestService(repo)
	_, err := svc.GetPreview(ctxWithApprover(), uuid.New())
	require.Error(t, err)
}

// ─── RegisterRoutes smoke test ────────────────────────────────────────────────

func TestRegisterRoutes_NoRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	h := NewHTTPHandler(svc)

	r := gin.New()
	// RequirePermission uses c.Get("claims") so set it in Gin context directly.
	r.Use(func(c *gin.Context) {
		claims := &auth.Claims{
			Sub:      testApproverID.String(),
			TenantID: "TUGURE",
			Permissions: []string{
				"penjualan.read", "penjualan.create",
				"penjualan.approve", "penjualan.reject",
			},
		}
		c.Set("claims", claims)
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	v1 := r.Group("/api/v1")
	RegisterRoutes(v1, h) // no Redis → rate limit skipped in dev

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/bm-frequency-alerts", nil))
	assert.Equal(t, http.StatusOK, rec2.Code)
}

// ─── Handler: GetPreview invalid UUID + not-found ────────────────────────────

func TestHandlerGetPreview_InvalidUUID(t *testing.T) {
	repo := newDefaultStubRepo()
	engine := newPenjualanEngine(repo, testApproverID.String())

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/not-a-uuid/preview", nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlerGetPreview_NotFound(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.penjualan = nil
	engine := newPenjualanEngine(repo, testApproverID.String())

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/"+uuid.New().String()+"/preview", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
