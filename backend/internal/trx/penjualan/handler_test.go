package penjualan

// handler_test.go — HTTP handler tests via httptest.
// Tests 7 endpoints: List, Create, GetByID, GetPreview, GetBMAlerts, Approve, Reject.
// Includes SoD bypass attempt via direct API call (verifies server-side enforcement).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newPenjualanEngine(repo *stubPenjualanRepo, sub string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := newTestService(repo)
	h := NewHTTPHandler(svc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		userSub := sub
		if v := c.GetHeader("X-Test-Sub"); v != "" {
			userSub = v
		}
		claims := &auth.Claims{
			Sub:      userSub,
			TenantID: "TUGURE",
			Roles:    []string{"ROLE-APPR-TR"},
			Permissions: []string{
				"penjualan.create", "penjualan.read",
				"penjualan.approve", "penjualan.reject",
			},
		}
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	v1 := r.Group("/api/v1")
	rg := v1.Group("/trx/penjualan")
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/bm-frequency-alerts", h.GetBMAlerts)
	rg.GET("/:id", h.GetByID)
	rg.GET("/:id/preview", h.GetPreview)
	rg.POST("/:id/approve", h.Approve)
	rg.POST("/:id/reject", h.Reject)
	return r
}

func penjualanJSONBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

// ─── GET /api/v1/trx/penjualan ───────────────────────────────────────────────

func TestHandlerList_Empty(t *testing.T) {
	repo := newDefaultStubRepo()
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Contains(t, resp, "data")
}

func TestHandlerList_WithItems(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.listRows = []*Penjualan{pj}
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan?limit=50", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── POST /api/v1/trx/penjualan ──────────────────────────────────────────────

func TestHandlerCreate_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)
	engine := newPenjualanEngine(repo, testMakerID.String())

	body := map[string]any{
		"instrumenId":      testInstrumenID.String(),
		"jenisDisposal":    "PARTIAL",
		"qtyTerjual":       500,
		"hargaJualPerUnit": 1100,
		"tanggalEksekusi":  "2026-06-15",
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/penjualan", penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestHandlerCreate_BadJSON_400(t *testing.T) {
	repo := newDefaultStubRepo()
	engine := newPenjualanEngine(repo, testMakerID.String())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/penjualan",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ─── GET /api/v1/trx/penjualan/bm-frequency-alerts ───────────────────────────

func TestHandlerGetBMAlerts_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.bmAlerts = []*BMAlertItem{
		{InstrumenKode: "OBL-001", FlagStatus: "BM_VIOLATION_RISK", CumulativeSold12mPct: "6.5000"},
	}
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/bm-frequency-alerts", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	// Verify the static route wins over /:id
	assert.NotEqual(t, http.StatusBadRequest, rec.Code, "bm-frequency-alerts must not be parsed as UUID")
}

func TestHandlerGetBMAlerts_Empty(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.bmAlerts = nil
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/bm-frequency-alerts", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	data := resp["data"]
	// Must return empty array, not null
	assert.NotNil(t, data)
}

// ─── GET /api/v1/trx/penjualan/:id ──────────────────────────────────────────

func TestHandlerGetByID_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.amortizedCarrying = decimal.NewFromInt(500000000)
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/"+pj.ID.String(), nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerGetByID_InvalidUUID_400(t *testing.T) {
	repo := newDefaultStubRepo()
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandlerGetByID_NotFound_404(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.penjualan = nil
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── GET /api/v1/trx/penjualan/:id/preview ──────────────────────────────────

func TestHandlerGetPreview_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.amortizedCarrying = decimal.NewFromInt(500000000)
	engine := newPenjualanEngine(repo, testApproverID.String())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/penjualan/"+pj.ID.String()+"/preview", nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ─── POST /api/v1/trx/penjualan/:id/approve ─────────────────────────────────

func TestHandlerApprove_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)
	engine := newPenjualanEngine(repo, testApproverID.String())

	body := map[string]string{
		"comment":         "Approved after market check",
		"signatureMethod": "JWT_STEP_UP",
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/trx/penjualan/"+pj.ID.String()+"/approve",
		penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerApprove_SoDBypass_403(t *testing.T) {
	// Maker tries to approve their own penjualan via direct API call — must be blocked server-side.
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	// Use MAKER's sub for the approver — SoD violation
	engine := newPenjualanEngine(repo, testMakerID.String())

	body := map[string]string{
		"comment":         "Self approval attempt",
		"signatureMethod": "JWT_STEP_UP",
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/trx/penjualan/"+pj.ID.String()+"/approve",
		penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code, "SoD violation must return 403")
}

func TestHandlerApprove_BadSignature_400(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	engine := newPenjualanEngine(repo, testApproverID.String())

	body := map[string]string{
		"comment":         "Approved",
		"signatureMethod": "PASSWORD", // invalid
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/trx/penjualan/"+pj.ID.String()+"/approve",
		penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ─── POST /api/v1/trx/penjualan/:id/reject ──────────────────────────────────

func TestHandlerReject_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	engine := newPenjualanEngine(repo, testApproverID.String())

	body := map[string]string{
		"reason":          "Harga jual terlalu rendah, tidak sesuai ketentuan treasury bulan Juni 2026.",
		"signatureMethod": "JWT_STEP_UP",
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/trx/penjualan/"+pj.ID.String()+"/reject",
		penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHandlerReject_SoDBypass_403(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	// Maker attempts to reject own penjualan
	engine := newPenjualanEngine(repo, testMakerID.String())

	body := map[string]string{
		"reason":          "Saya ingin membatalkan sendiri tanpa persetujuan approver sebenarnya.",
		"signatureMethod": "JWT_STEP_UP",
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/trx/penjualan/"+pj.ID.String()+"/reject",
		penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code, "SoD violation on reject must return 403")
}

func TestHandlerReject_ShortReason_400(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	engine := newPenjualanEngine(repo, testApproverID.String())

	body := map[string]string{
		"reason":          "terlalu singkat",
		"signatureMethod": "JWT_STEP_UP",
	}
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/trx/penjualan/"+pj.ID.String()+"/reject",
		penjualanJSONBody(t, body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// ─── compile-time unused imports guard ───────────────────────────────────────

var (
	_ = context.Background
	_ = time.Now
	_ = uuid.New
	_ = decimal.Zero
)
