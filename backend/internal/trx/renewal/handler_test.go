package renewal

// handler_test.go — HTTP handler tests via httptest.
// Injects auth claims via auth.ContextWithClaims middleware (same as prod).
// Tests 6 endpoints: List, Create, GetByID, GetPreview, Approve, Reject.

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

func newEngine(repo *stubRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := newService(repo)
	h := NewHTTPHandler(svc)

	r := gin.New()
	// Auth middleware stub: inject approverUUID claims
	r.Use(func(c *gin.Context) {
		sub := approverUUID.String()
		// override via header
		if v := c.GetHeader("X-Test-Sub"); v != "" {
			sub = v
		}
		claims := &auth.Claims{
			Sub:      sub,
			TenantID: "TUGURE",
			Roles:    []string{"ROLE-APPR-TR"},
			Permissions: []string{
				"renewal.create", "renewal.read", "renewal.approve", "renewal.reject",
			},
		}
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	v1 := r.Group("/api/v1")
	rg := v1.Group("/trx/renewal")
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/:id", h.GetByID)
	rg.GET("/:id/preview", h.GetPreview)
	rg.POST("/:id/approve", h.Approve)
	rg.POST("/:id/reject", h.Reject)
	return r
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

func decodeBody(t *testing.T, b []byte, v any) {
	require.NoError(t, json.Unmarshal(b, v))
}

// ─── GET /api/v1/trx/renewal ─────────────────────────────────────────────────

func TestHandler_List_Empty(t *testing.T) {
	repo := &stubRepo{}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []ListItem `json:"data"`
	}
	decodeBody(t, w.Body.Bytes(), &resp)
	assert.Empty(t, resp.Data)
}

func TestHandler_List_WithRows(t *testing.T) {
	r1 := goodRenewal(StatusPendingApproval)
	r2 := goodRenewal(StatusPosted)
	repo := &stubRepo{listRows: []*Renewal{r1, r2}}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal?limit=10", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── POST /api/v1/trx/renewal ────────────────────────────────────────────────

func TestHandler_Create_HappyPath(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		periode:       goodPeriode(),
	}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal",
		jsonBody(t, map[string]any{
			"instrumenId":        instrumenID.String(),
			"skema":              "POKOK_SAJA",
			"tenorBaruBulan":     12,
			"rateBaruPersen":     7.0,
			"tanggalEfektifBaru": "2026-07-01",
		}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Sub", approverUUID.String()) // approverUUID acts as maker here
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp struct {
		Data CreateRenewalResponse `json:"data"`
	}
	decodeBody(t, w.Body.Bytes(), &resp)
	assert.Equal(t, string(StatusPendingApproval), resp.Data.Status)
}

func TestHandler_Create_BadBody(t *testing.T) {
	repo := &stubRepo{}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal",
		bytes.NewBufferString("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Create_InvalidSkema(t *testing.T) {
	repo := &stubRepo{instrumenInfo: goodInstrumen(), hasActive: false, periode: goodPeriode()}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal",
		jsonBody(t, map[string]any{
			"instrumenId":        instrumenID.String(),
			"skema":              "INVALID",
			"tenorBaruBulan":     12,
			"rateBaruPersen":     7.0,
			"tanggalEfektifBaru": "2026-07-01",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── GET /api/v1/trx/renewal/:id ─────────────────────────────────────────────

func TestHandler_GetByID_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{renewal: renewal, instrumenInfo: goodInstrumen()}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal/"+renewalID.String(), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_GetByID_NotFound(t *testing.T) {
	repo := &stubRepo{renewal: nil}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal/"+renewalID.String(), nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandler_GetByID_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal/not-a-uuid", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── GET /api/v1/trx/renewal/:id/preview ─────────────────────────────────────

func TestHandler_GetPreview_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{renewal: renewal, instrumenInfo: goodInstrumen()}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal/"+renewalID.String()+"/preview", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data PreviewResponse `json:"data"`
	}
	decodeBody(t, w.Body.Bytes(), &resp)
	assert.NotEmpty(t, resp.Data.PokokBaru)
}

func TestHandler_GetPreview_NotFound(t *testing.T) {
	repo := &stubRepo{renewal: nil}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal/"+renewalID.String()+"/preview", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── POST /api/v1/trx/renewal/:id/approve ────────────────────────────────────

func TestHandler_Approve_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = makerUUID // approverUUID in ctx is different

	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
	}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/"+renewalID.String()+"/approve",
		jsonBody(t, map[string]any{
			"comment":         "Renewal disetujui setelah verifikasi kelengkapan.",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data ApproveRenewalResponse `json:"data"`
	}
	decodeBody(t, w.Body.Bytes(), &resp)
	assert.Equal(t, string(StatusPosted), resp.Data.Status)
}

func TestHandler_Approve_SoDViolation(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = approverUUID // same as ctx sub → SoD violation

	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
	}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/"+renewalID.String()+"/approve",
		jsonBody(t, map[string]any{
			"comment":         "Try to approve own renewal — SoD violation.",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeBody(t, w.Body.Bytes(), &errResp)
	assert.Equal(t, "SOD_VIOLATION", errResp.Error.Code)
}

func TestHandler_Approve_InvalidSignatureMethod(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = makerUUID
	repo := &stubRepo{renewal: renewal, instrumenInfo: goodInstrumen(), periode: goodPeriode()}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/"+renewalID.String()+"/approve",
		jsonBody(t, map[string]any{
			"comment":         "Approve with wrong signature method.",
			"signatureMethod": "TOTP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Approve_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/invalid-uuid/approve",
		jsonBody(t, map[string]any{
			"comment":         "Some approve comment",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── POST /api/v1/trx/renewal/:id/reject ─────────────────────────────────────

func TestHandler_Reject_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = makerUUID

	repo := &stubRepo{renewal: renewal}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/"+renewalID.String()+"/reject",
		jsonBody(t, map[string]any{
			"comment":         "Rate terlalu rendah. Tidak sesuai kebijakan manajemen risiko ALM.",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_Reject_SoDViolation(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = approverUUID // SoD

	repo := &stubRepo{renewal: renewal}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/"+renewalID.String()+"/reject",
		jsonBody(t, map[string]any{
			"comment":         "Reject own renewal — should fail SoD check properly.",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestHandler_Reject_CommentTooShort(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = makerUUID
	repo := &stubRepo{renewal: renewal}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/"+renewalID.String()+"/reject",
		jsonBody(t, map[string]any{
			"comment":         "Too short",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandler_Reject_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	engine := newEngine(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trx/renewal/bad-uuid/reject",
		jsonBody(t, map[string]any{
			"comment":         "Some reject reason that is long enough to pass.",
			"signatureMethod": "JWT_STEP_UP",
		}))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Prevent unused import errors
var (
	_ = context.Background
	_ = time.Now
	_ = uuid.Nil
	_ = decimal.Zero
)
