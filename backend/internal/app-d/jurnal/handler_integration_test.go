package jurnal

// handler_integration_test.go — HTTP handler guard tests using sqlmock-backed services.
//
// Tests cover the handler guard paths without a live DB:
//   - 401 when JWT claims missing from context.
//   - 403 when permission not in claims.permissions[].
//   - 400 for malformed UUID path params.
//   - 400 for missing required JSON body fields.
//
// These tests do NOT require a live PostgreSQL instance; sqlmock satisfies all
// repo constructors. Service logic that hits DB is not exercised here (separate
// integration tests cover that path).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Test setup helpers ───────────────────────────────────────────────────────

// buildJurnalTestRouter creates a gin router with all jurnal handlers wired to
// sqlmock-backed services.
func buildJurnalTestRouter(t *testing.T, claims *auth.Claims) *gin.Engine {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err, "sqlmock.New failed")
	t.Cleanup(func() { _ = db.Close() })

	// Satisfy mock expectations for audit writer (it pings DB on Write).
	// Suppress unexpected call panics by setting MatchExpectationsInOrder=false.
	mock.MatchExpectationsInOrder(false)

	mappingRepo := NewMappingRepo(db)
	jurnalRepo := NewJurnalRepo(db)
	dlqRepo := NewDLQRepo(db)
	aw := audit.NewWriter(db)

	mappingSvc := NewMappingService(mappingRepo, aw, nil)
	resolverSvc := NewResolverService(mappingRepo, db, nil)
	postingSvc := NewPostingService(jurnalRepo, dlqRepo, resolverSvc, aw, nil)
	dlqSvc := NewDLQService(dlqRepo, postingSvc, aw, nil)

	h := NewHandler(mappingSvc, resolverSvc, postingSvc, dlqSvc)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		if claims != nil {
			ctx := auth.ContextWithClaims(c.Request.Context(), claims)
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	})

	// Register all jurnal routes manually (mirrors routes.go).
	v := r.Group("/api/v1")
	{
		mh := v.Group("/jurnal/mapping-headers")
		mh.POST("", h.CreateMappingHeader)
		mh.GET("", h.ListMappingHeaders)
		mh.GET("/:id", h.GetMappingHeader)
		mh.PATCH("/:id", h.EditMappingHeader)
		mh.POST("/:id/submit", h.SubmitMappingHeader)
		mh.POST("/:id/review", h.ReviewMappingHeader)
		mh.POST("/:id/approve", h.ApproveMappingHeader)
		mh.POST("/:id/approve-2", h.ApproveMappingHeader2)
		mh.POST("/:id/reject", h.RejectMappingHeader)
		mh.POST("/:id/withdraw", h.WithdrawMappingHeader)
		mh.POST("/:id/deactivate", h.DeactivateMappingHeader)
		mh.GET("/export", h.ExportMappingHeaders)
	}
	{
		jh := v.Group("/jurnal")
		jh.POST("/resolve", h.ResolveJurnal)
		jh.POST("/post", h.PostManualJurnal)
		jh.GET("", h.ListJurnal)
		jh.GET("/:id", h.GetJurnal)
		jh.POST("/:id/submit", h.SubmitManualJurnal)
		jh.POST("/:id/approve", h.ApproveManualJurnal)
		jh.POST("/:id/reject", h.RejectManualJurnal)
		jh.GET("/export", h.ExportJurnal)
	}
	{
		dh := v.Group("/jurnal/dlq")
		dh.GET("", h.ListDLQ)
		dh.GET("/:id", h.GetDLQ)
		dh.POST("/:id/replay", h.ReplayDLQ)
		dh.POST("/:id/discard", h.DiscardDLQ)
	}

	return r
}

func jurnalMakeReq(method, path, body string) *http.Request {
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	return req
}

func jurnalClaimsWithPerms(perms ...string) *auth.Claims {
	ts := nowUnix()
	return &auth.Claims{
		Sub:              uuid.New().String(),
		Permissions:      perms,
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &ts,
	}
}

func nowUnix() int64 {
	return int64(^uint64(0) >> 1) // max int64 — step-up always fresh in tests
}

// ─── No claims → 403 (permission check fires before callerUUID) ───────────────
//
// The jurnal handler calls requirePermission() BEFORE callerUUID(), so when
// claims are nil the permission check returns FORBIDDEN (403) first.
// The 401 path only fires from the JWT middleware (not present in this test setup).

func TestJurnalHandler_NoClaims_403(t *testing.T) {
	r := buildJurnalTestRouter(t, nil) // no claims injected

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/api/v1/jurnal/mapping-headers", `{"eventCode":"PENEMPATAN","namaEvent":"x","kategoriEvent":"y","triggerSource":"USER_INPUT","detailRows":[{"urutan":1,"kodeAkunId":"00000000-0000-0000-0000-000000000001","dkIndicator":"DEBIT","sumberAmount":"AMOUNT"},{"urutan":2,"kodeAkunId":"00000000-0000-0000-0000-000000000002","dkIndicator":"KREDIT","sumberAmount":"AMOUNT"}]}`},
		{"GET", "/api/v1/jurnal/mapping-headers", ""},
		{"GET", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001", ""},
		{"PATCH", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001", `{"rowVersion":1}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/submit", "{}"},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/review", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/approve", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/approve-2", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/reject", `{"rejectReason":"this rejection reason is longer than 30 chars yes","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/withdraw", "{}"},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/deactivate", "{}"},
		{"POST", "/api/v1/jurnal/resolve", `{"eventCode":"PENEMPATAN","klasifikasiPsak71":"AC","periodeId":"00000000-0000-0000-0000-000000000001","sourceEventId":"00000000-0000-0000-0000-000000000002","sourceEventType":"test"}`},
		{"GET", "/api/v1/jurnal", ""},
		{"GET", "/api/v1/jurnal/00000000-0000-0000-0000-000000000001", ""},
		{"GET", "/api/v1/jurnal/dlq", ""},
		{"GET", "/api/v1/jurnal/dlq/00000000-0000-0000-0000-000000000001", ""},
		{"POST", "/api/v1/jurnal/dlq/00000000-0000-0000-0000-000000000001/replay", "{}"},
		{"POST", "/api/v1/jurnal/dlq/00000000-0000-0000-0000-000000000001/discard", `{"discardReason":"this is a reason long enough to pass validation check"}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq(ep.method, ep.path, ep.body))
			// requirePermission returns 403 when claims == nil (nil.HasPermission = false).
			assert.Equal(t, http.StatusForbidden, w.Code,
				"%s %s: expected 403, got %d; body=%s", ep.method, ep.path, w.Code, w.Body.String())
			var resp map[string]any
			assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "body must be JSON")
		})
	}
}

// ─── 403: claims present but missing permissions ───────────────────────────────

func TestJurnalHandler_NoPermissions_403(t *testing.T) {
	// Claims with no permissions.
	claims := jurnalClaimsWithPerms() // empty perms
	r := buildJurnalTestRouter(t, claims)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/api/v1/jurnal/mapping-headers", `{"eventCode":"PENEMPATAN","namaEvent":"x","kategoriEvent":"y","triggerSource":"USER_INPUT","detailRows":[{"urutan":1,"kodeAkunId":"00000000-0000-0000-0000-000000000001","dkIndicator":"DEBIT","sumberAmount":"AMOUNT"},{"urutan":2,"kodeAkunId":"00000000-0000-0000-0000-000000000002","dkIndicator":"KREDIT","sumberAmount":"AMOUNT"}]}`},
		{"GET", "/api/v1/jurnal/mapping-headers", ""},
		{"GET", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001", ""},
		{"PATCH", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001", `{"rowVersion":1}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/review", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/approve", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/approve-2", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/00000000-0000-0000-0000-000000000001/reject", `{"rejectReason":"this rejection reason is longer than 30 chars yes","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/resolve", `{"eventCode":"PENEMPATAN","klasifikasiPsak71":"AC","periodeId":"00000000-0000-0000-0000-000000000001","sourceEventId":"00000000-0000-0000-0000-000000000002","sourceEventType":"test"}`},
		{"GET", "/api/v1/jurnal", ""},
		{"GET", "/api/v1/jurnal/00000000-0000-0000-0000-000000000001", ""},
		{"GET", "/api/v1/jurnal/dlq", ""},
		{"GET", "/api/v1/jurnal/dlq/00000000-0000-0000-0000-000000000001", ""},
		{"POST", "/api/v1/jurnal/dlq/00000000-0000-0000-0000-000000000001/replay", "{}"},
		{"POST", "/api/v1/jurnal/dlq/00000000-0000-0000-0000-000000000001/discard", `{"discardReason":"this is a reason long enough to pass validation check"}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq(ep.method, ep.path, ep.body))
			assert.Equal(t, http.StatusForbidden, w.Code,
				"%s %s: expected 403, got %d; body=%s", ep.method, ep.path, w.Code, w.Body.String())
		})
	}
}

// ─── 400: bad UUID in :id path parameter ─────────────────────────────────────

func TestJurnalHandler_BadUUID_400(t *testing.T) {
	claims := jurnalClaimsWithPerms(
		PermMappingRead, PermMappingCreate, PermMappingReview, PermMappingApprove,
		PermMappingApprove2, PermJurnalRead, PermJurnalPost, PermJurnalApprove,
		PermDLQRead, PermDLQReplay, PermDLQDiscard,
	)
	r := buildJurnalTestRouter(t, claims)

	badID := "not-a-valid-uuid"
	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/jurnal/mapping-headers/" + badID, ""},
		{"PATCH", "/api/v1/jurnal/mapping-headers/" + badID, `{"rowVersion":1}`},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/submit", "{}"},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/review", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/approve", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/approve-2", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/reject", `{"rejectReason":"this rejection reason is longer than 30 chars yes","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/withdraw", "{}"},
		{"POST", "/api/v1/jurnal/mapping-headers/" + badID + "/deactivate", "{}"},
		{"GET", "/api/v1/jurnal/" + badID, ""},
		{"POST", "/api/v1/jurnal/" + badID + "/submit", "{}"},
		{"POST", "/api/v1/jurnal/" + badID + "/approve", `{"comment":"ok","signatureMethod":"JWT_STEP_UP"}`},
		{"POST", "/api/v1/jurnal/" + badID + "/reject", `{"rejectReason":"r","signatureMethod":"JWT_STEP_UP"}`},
		{"GET", "/api/v1/jurnal/dlq/" + badID, ""},
		{"POST", "/api/v1/jurnal/dlq/" + badID + "/replay", "{}"},
		{"POST", "/api/v1/jurnal/dlq/" + badID + "/discard", `{"discardReason":"this is a reason long enough to pass validation check"}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq(ep.method, ep.path, ep.body))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"%s %s: expected 400, got %d; body=%s", ep.method, ep.path, w.Code, w.Body.String())
			var resp map[string]any
			assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), "response body must be JSON")
		})
	}
}

// ─── 400: missing required JSON body fields ───────────────────────────────────

func TestJurnalHandler_CreateMappingHeader_MissingBody_400(t *testing.T) {
	claims := jurnalClaimsWithPerms(PermMappingCreate)
	r := buildJurnalTestRouter(t, claims)

	cases := []struct {
		name string
		body string
	}{
		{"empty_body", "{}"},
		{"missing_detail_rows", `{"eventCode":"PENEMPATAN","namaEvent":"x","kategoriEvent":"cat","triggerSource":"USER_INPUT"}`},
		{"only_one_detail_row", `{"eventCode":"PENEMPATAN","namaEvent":"x","kategoriEvent":"cat","triggerSource":"USER_INPUT","detailRows":[{"urutan":1,"kodeAkunId":"00000000-0000-0000-0000-000000000001","dkIndicator":"DEBIT","sumberAmount":"AMOUNT"}]}`},
		{"invalid_trigger_source", `{"eventCode":"PENEMPATAN","namaEvent":"x","kategoriEvent":"cat","triggerSource":"INVALID","detailRows":[{"urutan":1,"kodeAkunId":"00000000-0000-0000-0000-000000000001","dkIndicator":"DEBIT","sumberAmount":"AMOUNT"},{"urutan":2,"kodeAkunId":"00000000-0000-0000-0000-000000000002","dkIndicator":"KREDIT","sumberAmount":"AMOUNT"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq("POST", "/api/v1/jurnal/mapping-headers", tc.body))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"expected 400; body=%s", w.Body.String())
		})
	}
}

func TestJurnalHandler_ReviewMapping_MissingBody_400(t *testing.T) {
	claims := jurnalClaimsWithPerms(PermMappingReview)
	r := buildJurnalTestRouter(t, claims)

	validID := "00000000-0000-0000-0000-000000000001"
	// review requires comment (min=1) + signatureMethod.
	cases := []struct {
		name string
		body string
	}{
		{"empty_body", "{}"},
		{"missing_signature_method", `{"comment":"ok"}`},
		{"missing_comment", `{"signatureMethod":"JWT_STEP_UP"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq("POST", "/api/v1/jurnal/mapping-headers/"+validID+"/review", tc.body))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"expected 400 for %s; got %d; body=%s", tc.name, w.Code, w.Body.String())
		})
	}
}

func TestJurnalHandler_RejectMapping_ShortReason_400(t *testing.T) {
	claims := jurnalClaimsWithPerms(PermMappingReview)
	r := buildJurnalTestRouter(t, claims)

	validID := "00000000-0000-0000-0000-000000000001"
	// rejectReason min=30.
	cases := []struct {
		name string
		body string
	}{
		{"too_short_reason", `{"rejectReason":"short","signatureMethod":"JWT_STEP_UP"}`},
		{"empty_reason", `{"rejectReason":"","signatureMethod":"JWT_STEP_UP"}`},
		{"missing_reason", `{"signatureMethod":"JWT_STEP_UP"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq("POST", "/api/v1/jurnal/mapping-headers/"+validID+"/reject", tc.body))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"expected 400 for %s; got %d; body=%s", tc.name, w.Code, w.Body.String())
		})
	}
}

func TestJurnalHandler_Resolve_MissingBody_400(t *testing.T) {
	claims := jurnalClaimsWithPerms(PermJurnalRead)
	r := buildJurnalTestRouter(t, claims)

	cases := []struct {
		name string
		body string
	}{
		{"empty_body", "{}"},
		{"missing_event_code", `{"klasifikasiPsak71":"AC","periodeId":"00000000-0000-0000-0000-000000000001","sourceEventId":"00000000-0000-0000-0000-000000000002","sourceEventType":"test"}`},
		{"missing_source_event_id", `{"eventCode":"PENEMPATAN","klasifikasiPsak71":"AC","periodeId":"00000000-0000-0000-0000-000000000001","sourceEventType":"test"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq("POST", "/api/v1/jurnal/resolve", tc.body))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"expected 400 for %s; got %d; body=%s", tc.name, w.Code, w.Body.String())
		})
	}
}

func TestJurnalHandler_DiscardDLQ_ShortReason_400(t *testing.T) {
	claims := jurnalClaimsWithPerms(PermDLQDiscard)
	r := buildJurnalTestRouter(t, claims)

	validID := "00000000-0000-0000-0000-000000000001"
	cases := []struct {
		name string
		body string
	}{
		{"too_short_reason", `{"discardReason":"too short"}`},
		{"empty_reason", `{"discardReason":""}`},
		{"missing_reason", "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, jurnalMakeReq("POST", "/api/v1/jurnal/dlq/"+validID+"/discard", tc.body))
			assert.Equal(t, http.StatusBadRequest, w.Code,
				"expected 400 for %s; got %d; body=%s", tc.name, w.Code, w.Body.String())
		})
	}
}

// ─── NewHandler panic on nil services ────────────────────────────────────────

func TestJurnalHandler_NewHandlerPanicOnNilMapping(t *testing.T) {
	assert.Panics(t, func() {
		NewHandler(nil, nil, nil, nil)
	})
}
