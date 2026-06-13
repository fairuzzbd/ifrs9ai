package calcrun_test

// handler_integration_test.go — HTTP handler tests using sqlmock-backed *Service.
// Tests cover all 10 handler guard paths:
//   - Missing claims → 401
//   - Missing permission → 403
//   - Bad UUID → 400
//   - Bad JSON body → 400
//   - Happy path (mocked service response)
//
// Handler methods are also tested for the ApproveSeal step-up MFA enforcement.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/ecl/calcrun"
)

// ─── Test router builder ──────────────────────────────────────────────────────

func buildHandlerTestSetup(t *testing.T, claims *auth.Claims) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// Build service with sqlmock DB.
	repo := calcrun.NewRepo(db)
	snap := calcrun.NewParameterSnapshotService(db)
	aw := audit.NewWriter(db)
	svc := calcrun.NewService(repo, snap, aw, nil, nil, nil)
	h := calcrun.NewHandler(svc)

	r := gin.New()
	// Inject claims via middleware if provided.
	r.Use(func(c *gin.Context) {
		if claims != nil {
			c.Set("claims", claims)
		}
		c.Next()
	})
	r.POST("/ecl/calc-runs", h.CreateCalcRun)
	r.GET("/ecl/calc-runs", h.ListCalcRuns)
	r.GET("/ecl/calc-runs/:id", h.GetCalcRun)
	r.POST("/ecl/calc-runs/:id/start", h.StartCalcRun)
	r.POST("/ecl/calc-runs/:id/cancel", h.CancelCalcRun)
	r.GET("/ecl/calc-runs/:id/parameter-snapshot", h.GetParameterSnapshot)
	r.POST("/ecl/calc-runs/:id/seal/request", h.RequestSeal)
	r.POST("/ecl/calc-runs/:id/seal/approve", h.ApproveSeal)
	r.POST("/ecl/calc-runs/:id/seal/reject", h.RejectSeal)
	r.GET("/ecl/calc-runs/:id/result-lines", h.ListResultLines)
	return r, mock
}

func makeReq(method, path, body string) *http.Request {
	req, _ := http.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// ─── Helper: claims with given permissions ────────────────────────────────────

func claimsWithPerms(t *testing.T, perms ...string) *auth.Claims {
	t.Helper()
	return &auth.Claims{
		Sub:         uuid.New().String(),
		Permissions: perms,
	}
}

// ─── 401: missing claims ──────────────────────────────────────────────────────

func TestHandler_AllEndpoints_NoClaims_401(t *testing.T) {
	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/ecl/calc-runs", `{"periodeId":"p","evaluationDate":"2026-06-13","scope":"ALL_ACTIVE"}`},
		{"GET", "/ecl/calc-runs", ""},
		{"GET", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001", ""},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/start", ""},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/cancel", `{"cancelReason":"reason that is long enough here"}`},
		{"GET", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/parameter-snapshot", ""},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/seal/request", `{"comment":"test comment"}`},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/seal/approve", `{"comment":"test comment"}`},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/seal/reject", `{"rejectReason":"test reason"}`},
		{"GET", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/result-lines", ""},
	}

	// Build router with nil claims (no injection).
	r, _ := buildHandlerTestSetup(t, nil)

	for _, ep := range endpoints {
		ep := ep
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, makeReq(ep.method, ep.path, ep.body))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s %s: status = %d; want 401", ep.method, ep.path, w.Code)
			}
			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Errorf("response not JSON: %v", w.Body.String())
			}
		})
	}
}

// ─── 403: missing permissions ─────────────────────────────────────────────────

func TestHandler_AllEndpoints_NoPermission_403(t *testing.T) {
	// Claims with NO permissions.
	claims := claimsWithPerms(t) // empty permissions
	r, _ := buildHandlerTestSetup(t, claims)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"POST", "/ecl/calc-runs", `{"periodeId":"p","evaluationDate":"2026-06-13","scope":"ALL_ACTIVE"}`},
		{"GET", "/ecl/calc-runs", ""},
		{"GET", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001", ""},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/start", ""},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/cancel", `{"cancelReason":"reason that is long enough here"}`},
		{"GET", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/parameter-snapshot", ""},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/seal/request", `{"comment":"test comment"}`},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/seal/approve", `{"comment":"test comment"}`},
		{"POST", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/seal/reject", `{"rejectReason":"test reason"}`},
		{"GET", "/ecl/calc-runs/00000000-0000-0000-0000-000000000001/result-lines", ""},
	}
	for _, ep := range endpoints {
		ep := ep
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, makeReq(ep.method, ep.path, ep.body))
			if w.Code != http.StatusForbidden {
				t.Errorf("%s %s: status = %d; want 403", ep.method, ep.path, w.Code)
			}
		})
	}
}

// ─── 400: bad UUID in :id param ───────────────────────────────────────────────

func TestHandler_BadUUID_400(t *testing.T) {
	claims := claimsWithPerms(t,
		calcrun.PermCalcRunCreate, calcrun.PermCalcRunRead, calcrun.PermCalcRunStart,
		calcrun.PermCalcRunCancel, calcrun.PermCalcRunSealRequest, calcrun.PermCalcRunSealApprove,
	)
	r, _ := buildHandlerTestSetup(t, claims)

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/ecl/calc-runs/not-a-uuid", ""},
		{"POST", "/ecl/calc-runs/not-a-uuid/start", ""},
		{"POST", "/ecl/calc-runs/not-a-uuid/cancel", `{"cancelReason":"reason that is long enough here"}`},
		{"GET", "/ecl/calc-runs/not-a-uuid/parameter-snapshot", ""},
		{"POST", "/ecl/calc-runs/not-a-uuid/seal/request", `{"comment":"test comment"}`},
		{"POST", "/ecl/calc-runs/not-a-uuid/seal/approve", `{"comment":"test comment"}`},
		{"POST", "/ecl/calc-runs/not-a-uuid/seal/reject", `{"rejectReason":"test reason"}`},
		{"GET", "/ecl/calc-runs/not-a-uuid/result-lines", ""},
	}
	for _, ep := range endpoints {
		ep := ep
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, makeReq(ep.method, ep.path, ep.body))
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %s: status = %d; want 400", ep.method, ep.path, w.Code)
			}
			var resp map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Errorf("response not JSON: %v", w.Body.String())
			}
		})
	}
}

// ─── 400: missing required body fields ───────────────────────────────────────

func TestHandler_Create_MissingBody_400(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunCreate)
	r, _ := buildHandlerTestSetup(t, claims)

	w := httptest.NewRecorder()
	// Empty body → binding fails (periodeId required).
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestHandler_Create_CommentTooLong_400(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunCreate)
	r, _ := buildHandlerTestSetup(t, claims)

	longComment := string(make([]byte, 501)) // 501 chars
	body, _ := json.Marshal(map[string]any{
		"periodeId":      "p",
		"evaluationDate": "2026-06-13",
		"scope":          "ALL_ACTIVE",
		"comment":        longComment,
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs", string(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestHandler_Cancel_MissingBody_400(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunCancel)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id+"/cancel", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestHandler_SealRequest_MissingBody_400(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunSealRequest)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id+"/seal/request", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestHandler_SealApprove_MissingBody_400(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunSealApprove)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id+"/seal/approve", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

func TestHandler_SealReject_MissingBody_400(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunSealApprove)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id+"/seal/reject", `{}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d; want 400", w.Code)
	}
}

// ─── ApproveSeal: step-up MFA missing → 403 ──────────────────────────────────

func TestHandler_ApproveSeal_MissingStepUp_403(t *testing.T) {
	// Claims with permission but step-up NOT fresh (nil StepupVerifiedAt).
	claims := claimsWithPerms(t, calcrun.PermCalcRunSealApprove)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	body := `{"comment":"ALCO approves this calc run seal."}`
	req := makeReq("POST", "/ecl/calc-runs/"+id+"/seal/approve", body)
	// No X-Step-Up-Token header.

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// stepUpFresh = false because:
	//   a) claims.NeedsStepUp() = true (StepupVerifiedAt nil), AND
	//   b) X-Step-Up-Token header absent.
	// service.ApproveSeal(... stepUpFresh=false) → ErrCalcRunSealStepUpRequired → 403.
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403 (step-up required)", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' key in response")
	}
	if errObj["code"] != "CALC_RUN_SEAL_STEP_UP_REQUIRED" {
		t.Errorf("code = %q; want CALC_RUN_SEAL_STEP_UP_REQUIRED", errObj["code"])
	}
}

// ─── ApproveSeal: NeedsStepUp=false but no X-Step-Up-Token header → 403 ──────

func TestHandler_ApproveSeal_StepUpFreshButNoHeader_403(t *testing.T) {
	// Step-up was done within 5 min (fresh) but header missing.
	now := time.Now().Unix()
	claims := &auth.Claims{
		Sub:              uuid.New().String(),
		Permissions:      []string{calcrun.PermCalcRunSealApprove},
		StepupVerifiedAt: &now,
	}
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	body := `{"comment":"ALCO approves this calc run seal."}`
	req := makeReq("POST", "/ecl/calc-runs/"+id+"/seal/approve", body)
	// No X-Step-Up-Token header → stepUpFresh = false even with fresh claims.

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d; want 403 (header missing)", w.Code)
	}
}

// ─── GetCalcRun: not found → 404 (via writeCalcRunError) ─────────────────────

func TestHandler_GetCalcRun_NotFound_404(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	// Expect the SELECT query that getByID makes.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty → not found

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id.String(), ""))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != "CALC_RUN_NOT_FOUND" {
		t.Errorf("code = %q; want CALC_RUN_NOT_FOUND", errObj["code"])
	}
}

// ─── ListCalcRuns: empty list → 200 ──────────────────────────────────────────

func TestHandler_ListCalcRuns_Empty_200(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"processed_count", "error_count", "total_instrumen",
			"started_at", "completed_at", "sealed_at",
			"created_at", "created_by",
		}))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs", ""))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	if _, ok := resp["data"]; !ok {
		t.Error("expected 'data' key in response")
	}
	if _, ok := resp["pagination"]; !ok {
		t.Error("expected 'pagination' key in response")
	}
}

// ─── ListResultLines: returns redirect message → 200 ─────────────────────────

func TestHandler_ListResultLines_200(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id+"/result-lines", ""))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	// data.calcRunId should be present
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'data' object in response")
	}
	if data["calcRunId"] == nil {
		t.Error("expected 'calcRunId' in data")
	}
}

// ─── Cancel: reason short → 422 ──────────────────────────────────────────────

func TestHandler_CancelCalcRun_ShortReason_422(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunCancel)
	r, _ := buildHandlerTestSetup(t, claims)

	id := uuid.New().String()
	body := `{"cancelReason":"Short"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id+"/cancel", body))
	// The binding.min=30 validation fails → 400.
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d; want 400 or 422", w.Code)
	}
}

// ─── GetParameterSnapshot: run not found → 404 ───────────────────────────────

func TestHandler_GetParameterSnapshot_NotFound_404(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id.String()+"/parameter-snapshot", ""))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// ─── Create: periode not found (no HARD_CLOSED row) + no IN_PROGRESS + no SEALED → succeeds up to INSERT ──────────────────────────────────────────────────────

func TestHandler_Create_HardClosed_423(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunCreate)
	r, mock := buildHandlerTestSetup(t, claims)

	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(`SELECT status FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("HARD_CLOSED"))

	body := `{"periodeId":"periode-2026-06","evaluationDate":"2026-06-13","scope":"ALL_ACTIVE"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs", body))
	if w.Code != http.StatusLocked {
		t.Errorf("status = %d; want 423", w.Code)
	}
}

// ─── Start: no claims → 401 ──────────────────────────────────────────────────

func TestHandler_StartCalcRun_NoClaims_401(t *testing.T) {
	r, _ := buildHandlerTestSetup(t, nil)
	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id+"/start", ""))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

// ─── Start: run not found → 404 (writeCalcRunError path) ─────────────────────

func TestHandler_StartCalcRun_RunNotFound_404(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunStart)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/start", ""))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// ─── Cancel: run not found → 404 (writeCalcRunError path) ────────────────────

func TestHandler_CancelCalcRun_RunNotFound_404(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunCancel)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	mock.MatchExpectationsInOrder(false)
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := `{"cancelReason":"Alasan pembatalan cukup panjang untuk memenuhi validasi minimal."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/cancel", body))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// ─── GetParameterSnapshot: run with snapshot → 200 ───────────────────────────

func TestHandler_GetParameterSnapshot_Found_200(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	createdBy := uuid.New()
	snapshotJSON := []byte(`{"periodeId":"p-2026-06","frozenAt":"2026-06-13T00:00:00Z"}`)

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			id, "p-2026-06", evalDate, "ALL_ACTIVE", "IN_PROGRESS",
			nil, nil, 0, 0,
			nil, nil, snapshotJSON,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, createdBy, now, createdBy, 1, "TUGURE",
		))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id.String()+"/parameter-snapshot", ""))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
}

// ─── RequestSeal: run not found → 404 ────────────────────────────────────────

func TestHandler_RequestSeal_RunNotFound_404(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunSealRequest)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := `{"comment":"Request seal untuk audit."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/seal/request", body))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// ─── RejectSeal: run not found → 404 ─────────────────────────────────────────

func TestHandler_RejectSeal_RunNotFound_404(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunSealApprove)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	body := `{"rejectReason":"Data belum lengkap."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/seal/reject", body))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d; want 404", w.Code)
	}
}

// ─── ListCalcRuns: DB error → 500 via writeCalcRunError fallback ─────────────
//
// This test verifies writeCalcRunError falls through to the 500 path when the
// error is NOT a calcRunError (stdlib/DB error). Covers handler.go:447.

func TestHandler_ListCalcRuns_DBError_500(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	// Return a non-calcRunError DB error so writeCalcRunError takes the 500 branch.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnError(errDB("connection refused"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs", ""))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500 (writeCalcRunError fallback)", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
}

// ─── claimsFromCtx: wrong type in context → 401 ──────────────────────────────
//
// Exercises the second 401 branch in claimsFromCtx (handler.go:427):
// context has "claims" key but value is not *auth.Claims.

func TestHandler_ClaimsFromCtx_WrongType_401(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	_ = mock

	repo := calcrun.NewRepo(db)
	snap := calcrun.NewParameterSnapshotService(db)
	aw := audit.NewWriter(db)
	svc := calcrun.NewService(repo, snap, aw, nil, nil, nil)
	h := calcrun.NewHandler(svc)

	r := gin.New()
	// Inject a string (wrong type) as "claims" — not *auth.Claims.
	r.Use(func(c *gin.Context) {
		c.Set("claims", "this-is-not-an-auth-claims-struct")
		c.Next()
	})
	r.GET("/ecl/calc-runs/:id", h.GetCalcRun)

	id := uuid.New().String()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id, ""))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401 (claimsFromCtx wrong type)", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
}

// ─── GetParameterSnapshot: run exists, snapshot is invalid JSON → 500 ────────
//
// Exercises handler.go:259-263 — json.Unmarshal(raw, &snap) fails.

func TestHandler_GetParameterSnapshot_InvalidJSON_500(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	now := time.Now().UTC()
	createdBy := uuid.New()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	// Invalid JSON bytes — not parseable as any JSON value.
	invalidSnapshotJSON := []byte("{invalid json[}")

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			id, "p-2026-06", evalDate, "ALL_ACTIVE", "IN_PROGRESS",
			nil, nil, 0, 0,
			nil, nil, invalidSnapshotJSON, // parameter_snapshot_jsonb = invalid JSON
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, createdBy, now, createdBy, 1, "TUGURE",
		))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id.String()+"/parameter-snapshot", ""))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500 (invalid snapshot JSON); body = %s", w.Code, w.Body.String())
	}
}

// ─── GetParameterSnapshot: run exists, snapshot is nil → 200 with data:null ──
//
// Exercises handler.go:250 — the raw == nil branch.

func TestHandler_GetParameterSnapshot_NilSnapshot_200(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	now := time.Now().UTC()
	createdBy := uuid.New()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Return a run with parameter_snapshot_jsonb = nil.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			id, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT",
			nil, nil, 0, 0,
			nil, nil, nil, // parameter_snapshot_jsonb = nil
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, createdBy, now, createdBy, 1, "TUGURE",
		))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id.String()+"/parameter-snapshot", ""))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	// data must be nil (null snapshot).
	if resp["data"] != nil {
		t.Errorf("expected data=null for nil snapshot; got %v", resp["data"])
	}
}

// ─── CreateCalcRun: claims.Sub is not a valid UUID → 500 ─────────────────────
//
// Exercises handler.go:75 — uuid.Parse(claims.Sub) fails before svc.Create.

func TestHandler_CreateCalcRun_InvalidSubUUID_500(t *testing.T) {
	claims := &auth.Claims{
		Sub:         "not-a-valid-uuid",
		Permissions: []string{calcrun.PermCalcRunCreate},
	}
	// No DB queries fire: uuid.Parse fails before svc.Create is called.
	r, _ := buildHandlerTestSetup(t, claims)

	body, _ := json.Marshal(map[string]any{
		"periodeId":      "p-2026-06",
		"evaluationDate": "2026-06-13",
		"scope":          "ALL_ACTIVE",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs", string(body)))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d; want 500 (invalid Sub UUID)", w.Code)
	}
}

// ─── GetCalcRun: happy path → 200 ────────────────────────────────────────────

func TestHandler_GetCalcRun_Found_200(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	id := uuid.New()
	now := time.Now().UTC()
	createdBy := uuid.New()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			id, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT",
			nil, nil, 0, 0, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, createdBy, now, createdBy, 1, "TUGURE",
		))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs/"+id.String(), ""))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
}

// ─── ListCalcRuns: with nextCursor → 200 ──────────────────────────────────────
//
// Exercises handler.go:114 — nc = &nextCursor when List returns a non-empty cursor.

func TestHandler_ListCalcRuns_WithCursor_200(t *testing.T) {
	claims := claimsWithPerms(t, calcrun.PermCalcRunRead)
	r, mock := buildHandlerTestSetup(t, claims)

	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	id1 := uuid.New()
	id2 := uuid.New()
	// Return limit+1 rows to trigger hasMore=true and cursor population.
	// List is called with default limit=50; return 2 rows with limit+1 trick.
	// Since we can't easily control the internal limit in handler (hardcoded 50),
	// we return 51 rows by returning 51+1 from DB. But that's complex.
	// Instead: just return 2 rows to verify nextCursor=""  (no hasMore).
	// To test nc != "" we need List to return hasMore=true + a cursor.
	// The simplest way: return 51 rows from the mock (limit+1 trick in repo).
	// For the List scan in repo: it needs 14-column rows.
	// Return 51 minimal rows to trigger hasMore=true.
	rows := sqlmock.NewRows([]string{
		"id", "periode_id", "evaluation_date", "scope", "status",
		"processed_count", "error_count", "total_instrumen",
		"started_at", "completed_at", "sealed_at",
		"created_at", "created_by",
	})
	// Add 51 rows (> limit 50) to get hasMore=true and nextCursor populated.
	for i := 0; i < 51; i++ {
		uid := uuid.New()
		rows.AddRow(uid, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT",
			0, 0, nil, nil, nil, nil, now, id1)
	}
	// Keep id1, id2 defined for reference (avoid unused variable error).
	_ = id1
	_ = id2

	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(rows)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("GET", "/ecl/calc-runs", ""))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	pagination, ok := resp["pagination"].(map[string]any)
	if !ok {
		t.Fatal("expected pagination object")
	}
	if pagination["hasMore"] != true {
		t.Errorf("hasMore = %v; want true", pagination["hasMore"])
	}
	// nextCursor must be non-null when hasMore=true.
	if pagination["nextCursor"] == nil {
		t.Error("nextCursor is nil; want populated cursor")
	}
}

// ─── StartCalcRun: happy path → 202 ──────────────────────────────────────────
//
// Exercises handler.go:184 — response.Accepted(c, resp).
// Full Start mock chain: Get + periodeCheck + SnapshotAll(7) + countInstruments
// + BeginTx + UpdateStartFields + getByIDTx + insertSysJob + audit(2) + Commit.

func TestHandler_StartCalcRun_HappyPath_202(t *testing.T) {
	actorID := uuid.New()
	claims := &auth.Claims{
		Sub:         actorID.String(),
		Permissions: []string{calcrun.PermCalcRunStart},
	}
	r, mock := buildHandlerTestSetup(t, claims)
	mock.MatchExpectationsInOrder(false)

	runID := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Get run (DRAFT).
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT",
			nil, nil, 0, 0, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 1, "TUGURE",
		))

	// checkPeriodeNotHardClosed → OPEN.
	mock.ExpectQuery(`SELECT status FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("OPEN"))

	// SnapshotAll (7 sub-queries).
	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("bs-1", "0.2500", "0.5000", "0.2500", "alco-user", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(10, "alco-user", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(5, "alco-user", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.05000000", "alco-user", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.90000000", "alco-user", "2026-06-01").
			AddRow("imev-2", "BAD", "1.10000000", "alco-user", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}).
			AddRow("lps-1", "2000000000.0000", "2020-01-01", nil, "alco-user"))
	mock.ExpectQuery(`SELECT .+ FROM mst.kurs`).
		WillReturnRows(sqlmock.NewRows([]string{"kode_mata_uang", "kurs_tengah", "tanggal"}).
			AddRow("USD", "15800.00000000", "2026-06-13"))

	// countActiveInstruments.
	mock.ExpectQuery(`SELECT COUNT.* FROM mst.instrumen`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(50))

	// BeginTx + UpdateStartFields + getByIDTx + insertSysJob + audit(2) + Commit.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "IN_PROGRESS",
			"job-abc-123", 50, 0, 0, now, nil, []byte(`{"snap":true}`),
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 2, "TUGURE",
		))
	mock.ExpectExec(`INSERT INTO sys.job`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+runID.String()+"/start", ""))
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d; want 202; body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Errorf("response not JSON: %v", w.Body.String())
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data object in response")
	}
	if data["jobId"] == nil {
		t.Error("expected jobId in response data")
	}
}

// ─── CancelCalcRun: happy path → 200 ─────────────────────────────────────────

func TestHandler_CancelCalcRun_HappyPath_200(t *testing.T) {
	actorID := uuid.New()
	claims := &auth.Claims{
		Sub:         actorID.String(),
		Permissions: []string{calcrun.PermCalcRunCancel},
	}
	r, mock := buildHandlerTestSetup(t, claims)
	mock.MatchExpectationsInOrder(false)

	runID := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Get run (DRAFT, createdBy = actorID so maker check passes).
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT",
			nil, nil, 0, 0, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 1, "TUGURE",
		))
	// BeginTx.
	mock.ExpectBegin()
	// UpdateCancel EXEC.
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// getByIDTx SELECT.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "CANCELLED",
			nil, nil, 0, 0, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
			actorID, now, "Test cancel reason",
			nil,
			now, actorID, now, actorID, 2, "TUGURE",
		))
	// Audit.
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// Commit.
	mock.ExpectCommit()

	body := `{"cancelReason":"Test cancel reason that is long enough to meet the minimum requirement."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+runID.String()+"/cancel", body))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
}

// ─── RequestSeal: happy path → 200 ───────────────────────────────────────────

func TestHandler_RequestSeal_HappyPath_200(t *testing.T) {
	actorID := uuid.New()
	claims := &auth.Claims{
		Sub:         actorID.String(),
		Permissions: []string{calcrun.PermCalcRunSealRequest},
	}
	r, mock := buildHandlerTestSetup(t, claims)
	mock.MatchExpectationsInOrder(false)

	runID := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Get run (COMPLETED, no errors).
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "COMPLETED",
			nil, nil, 0, 0, nil, now, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 1, "TUGURE",
		))
	// CheckExistingSealed → no existing sealed run.
	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	// BeginTx.
	mock.ExpectBegin()
	// UpdateSealRequest EXEC.
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// getByIDTx SELECT.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "SEAL_REQUESTED",
			nil, nil, 0, 0, nil, now, nil,
			actorID, now, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 2, "TUGURE",
		))
	// Audit.
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// Commit.
	mock.ExpectCommit()

	body := `{"comment":"Requesting seal for ECL run periode Juni 2026."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+runID.String()+"/seal/request", body))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
}

// ─── RejectSeal: happy path → 200 ────────────────────────────────────────────

func TestHandler_RejectSeal_HappyPath_200(t *testing.T) {
	actorID := uuid.New()
	requesterID := uuid.New() // different from actorID (SoD)
	claims := &auth.Claims{
		Sub:         actorID.String(),
		Permissions: []string{calcrun.PermCalcRunSealApprove},
	}
	r, mock := buildHandlerTestSetup(t, claims)
	mock.MatchExpectationsInOrder(false)

	runID := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Get run (SEAL_REQUESTED, seal_requested_by = requesterID ≠ actorID).
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "SEAL_REQUESTED",
			nil, nil, 0, 0, nil, now, nil,
			requesterID, now, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, requesterID, now, requesterID, 2, "TUGURE",
		))
	// BeginTx.
	mock.ExpectBegin()
	// UpdateSealReject EXEC.
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// getByIDTx SELECT.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "COMPLETED",
			nil, nil, 0, 0, nil, now, nil,
			nil, nil, nil, nil, nil, nil,
			actorID, now, "Data PD tidak final",
			nil, nil, nil, nil,
			now, actorID, now, actorID, 3, "TUGURE",
		))
	// Audit.
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// Commit.
	mock.ExpectCommit()

	body := `{"rejectReason":"Data PD tidak final, perlu revisi."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+runID.String()+"/seal/reject", body))
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
}

// ─── ApproveSeal: happy path → 200 ───────────────────────────────────────────

func TestHandler_ApproveSeal_HappyPath_200(t *testing.T) {
	actorID := uuid.New()
	requesterID := uuid.New() // different from actorID (SoD)
	stepupAt := time.Now().Unix()
	claims := &auth.Claims{
		Sub:              actorID.String(),
		Permissions:      []string{calcrun.PermCalcRunSealApprove},
		StepupVerifiedAt: &stepupAt,
	}
	r, mock := buildHandlerTestSetup(t, claims)
	mock.MatchExpectationsInOrder(false)

	runID := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)

	// Get run (SEAL_REQUESTED, requesterID ≠ actorID).
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WithArgs(runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "SEAL_REQUESTED",
			nil, nil, 0, 0, nil, now, nil,
			requesterID, now, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, requesterID, now, requesterID, 2, "TUGURE",
		))
	// BeginTx.
	mock.ExpectBegin()
	// UpdateSealApprove EXEC.
	mock.ExpectExec(`UPDATE ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// getByIDTx SELECT.
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			runID, "p-2026-06", evalDate, "ALL_ACTIVE", "SEALED",
			nil, nil, 0, 0, nil, now, nil,
			requesterID, now, actorID, now, now, []byte("sig"),
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 3, "TUGURE",
		))
	// 2 audit writes (SEAL_APPROVED + SEALED).
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	// Commit.
	mock.ExpectCommit()

	body := `{"comment":"ALCO approves this ECL calc run seal for periode Juni 2026."}`
	req := makeReq("POST", "/ecl/calc-runs/"+runID.String()+"/seal/approve", body)
	req.Header.Set("X-Step-Up-Token", "valid-mfa-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200; body = %s", w.Code, w.Body.String())
	}
}

// ─── CreateCalcRun: happy path → 201 ─────────────────────────────────────────

func TestHandler_CreateCalcRun_HappyPath_201(t *testing.T) {
	actorID := uuid.New()
	claims := &auth.Claims{
		Sub:         actorID.String(),
		Permissions: []string{calcrun.PermCalcRunCreate},
	}
	r, mock := buildHandlerTestSetup(t, claims)
	mock.MatchExpectationsInOrder(false)

	// checkPeriodeNotHardClosed → OPEN
	mock.ExpectQuery(`SELECT status FROM mst.periode_buku`).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("OPEN"))
	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(`SELECT id FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ecl.calc_run`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	newRunID := uuid.New()
	now := time.Now().UTC()
	evalDate := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT .+ FROM ecl.calc_run`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "periode_id", "evaluation_date", "scope", "status",
			"job_id", "total_instrumen", "processed_count", "error_count",
			"started_at", "completed_at", "parameter_snapshot_jsonb",
			"seal_requested_by", "seal_requested_at",
			"seal_approved_by", "seal_approved_at",
			"sealed_at", "signature_hash_seal",
			"seal_rejected_by", "seal_rejected_at", "reject_reason",
			"cancelled_by", "cancelled_at", "cancel_reason",
			"superseded_by_run_id",
			"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id",
		}).AddRow(
			newRunID, "p-2026-06", evalDate, "ALL_ACTIVE", "DRAFT",
			nil, nil, 0, 0,
			nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil,
			now, actorID, now, actorID, 1, "TUGURE",
		))
	mock.ExpectQuery(`SELECT current_hash FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud.audit_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body, _ := json.Marshal(map[string]any{
		"periodeId":      "p-2026-06",
		"evaluationDate": "2026-06-13",
		"scope":          "ALL_ACTIVE",
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs", string(body)))
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d; want 201; body = %s", w.Code, w.Body.String())
	}
}

// ─── StartCalcRun: invalid actor UUID in Sub → 400 VALIDATION_FAILED ─────────
//
// Exercises handler.go — new uuid.Parse(claims.Sub) guard in StartCalcRun.
// The id parse succeeds (valid UUID); actor parse fails before svc.Start is called.

func TestHandler_StartCalcRun_InvalidActorUUID_400(t *testing.T) {
	id := uuid.New()
	claims := &auth.Claims{
		Sub:         "not-a-uuid",
		Permissions: []string{calcrun.PermCalcRunStart},
	}
	// No DB expectations: actor parse fails before any service call.
	r, _ := buildHandlerTestSetup(t, claims)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/start", ""))
	if w.Code != http.StatusBadRequest {
		t.Errorf("StartCalcRun invalid actor: status = %d; want 400", w.Code)
	}
}

// ─── CancelCalcRun: invalid actor UUID in Sub → 400 VALIDATION_FAILED ────────

func TestHandler_CancelCalcRun_InvalidActorUUID_400(t *testing.T) {
	id := uuid.New()
	claims := &auth.Claims{
		Sub:         "not-a-uuid",
		Permissions: []string{calcrun.PermCalcRunCancel},
	}
	r, _ := buildHandlerTestSetup(t, claims)

	body := `{"cancelReason":"Alasan pembatalan yang cukup panjang untuk memenuhi syarat minimum."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/cancel", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("CancelCalcRun invalid actor: status = %d; want 400", w.Code)
	}
}

// ─── RequestSeal: invalid actor UUID in Sub → 400 VALIDATION_FAILED ──────────

func TestHandler_RequestSeal_InvalidActorUUID_400(t *testing.T) {
	id := uuid.New()
	claims := &auth.Claims{
		Sub:         "not-a-uuid",
		Permissions: []string{calcrun.PermCalcRunSealRequest},
	}
	r, _ := buildHandlerTestSetup(t, claims)

	body := `{"comment":"Permintaan seal dengan alasan yang cukup."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/seal/request", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("RequestSeal invalid actor: status = %d; want 400", w.Code)
	}
}

// ─── ApproveSeal: invalid actor UUID in Sub → 400 VALIDATION_FAILED ──────────

func TestHandler_ApproveSeal_InvalidActorUUID_400(t *testing.T) {
	id := uuid.New()
	now := int64(1748000000)
	claims := &auth.Claims{
		Sub:              "not-a-uuid",
		Permissions:      []string{calcrun.PermCalcRunSealApprove},
		StepupVerifiedAt: &now,
	}
	r, _ := buildHandlerTestSetup(t, claims)

	req, _ := http.NewRequest("POST", "/ecl/calc-runs/"+id.String()+"/seal/approve",
		bytes.NewBufferString(`{"comment":"Approved by ALCO committee."}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Step-Up-Token", "valid-step-up-token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ApproveSeal invalid actor: status = %d; want 400", w.Code)
	}
}

// ─── RejectSeal: invalid actor UUID in Sub → 400 VALIDATION_FAILED ───────────

func TestHandler_RejectSeal_InvalidActorUUID_400(t *testing.T) {
	id := uuid.New()
	claims := &auth.Claims{
		Sub:         "not-a-uuid",
		Permissions: []string{calcrun.PermCalcRunSealApprove},
	}
	r, _ := buildHandlerTestSetup(t, claims)

	body := `{"rejectReason":"Data tidak lengkap dan perlu dikoreksi."}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, makeReq("POST", "/ecl/calc-runs/"+id.String()+"/seal/reject", body))
	if w.Code != http.StatusBadRequest {
		t.Errorf("RejectSeal invalid actor: status = %d; want 400", w.Code)
	}
}
