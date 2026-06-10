package ratinghistory_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/master/ratinghistory"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router setup ─────────────────────────────────────────────────────────────

func newRouter(svc *ratinghistory.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "risk.officer",
		Roles:             []string{"ROLE-RISK"},
		Permissions: []string{
			"rating_history.read", "rating_history.create", "rating_history.update",
			"rating_history.delete", "rating_history.submit", "rating_history.review",
			"rating_history.approve", "rating_history.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := ratinghistory.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	ratinghistory.RegisterRoutes(v1, h)
	ratinghistory.RegisterCounterpartyNestedRoutes(v1, h)
	return r
}

func buildSvc(repo *repoAdapter) *ratinghistory.Service {
	return ratinghistory.NewService(repo, &stubCPRepo{}, audit.NewWriter(nil), slog.Default())
}

// ─── POST /master/rating-history ──────────────────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/rating-history",
		bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestCreate_InvalidActionType_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := `{
		"ratingHistoryIdKode":"RH-001",
		"counterpartyId":"00000000-0000-0000-0000-000000000010",
		"tanggalBerlaku":"2026-01-01",
		"ratingPefindo":"idA",
		"sumberRating":"PEFINDO",
		"tanggalPublikasiRating":"2026-01-01",
		"actionType":"INVALID_TYPE"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/rating-history",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid action_type, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_InvalidDateFormat_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body := `{
		"ratingHistoryIdKode":"RH-001",
		"counterpartyId":"00000000-0000-0000-0000-000000000010",
		"tanggalBerlaku":"01/01/2026",
		"ratingPefindo":"idA",
		"sumberRating":"PEFINDO",
		"tanggalPublikasiRating":"2026-01-01",
		"actionType":"INITIAL"
	}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/rating-history",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid date, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/rating-history/:id ───────────────────────────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/rating-history/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{rh: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/rating-history/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	rh := testRatingHistory()
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{rh: rh},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/rating-history/"+rh.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID                  string `json:"id"`
			RatingHistoryIDKode string `json:"ratingHistoryIdKode"`
			IsInvestmentGrade   bool   `json:"isInvestmentGrade"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.RatingHistoryIDKode != "RH-2026-001" {
		t.Errorf("expected RH-2026-001, got %s", resp.Data.RatingHistoryIDKode)
	}
	// idA is Investment Grade
	if !resp.Data.IsInvestmentGrade {
		t.Errorf("expected isInvestmentGrade=true for idA")
	}
}

// ─── GET /master/rating-history ───────────────────────────────────────────────

func TestList_Returns200(t *testing.T) {
	rh := testRatingHistory()
	r := newRouter(buildSvc(&repoAdapter{
		list: &stubList{items: []*ratinghistory.RatingHistory{rh}},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/rating-history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestList_InvalidSortCol_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/rating-history?sort=invalid_col:asc", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid sort col, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/counterparty/:counterpartyId/rating-history ──────────────────

func TestListByCounterparty_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/counterparty/not-a-uuid/rating-history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestListByCounterparty_ValidUUID_Returns200(t *testing.T) {
	rh := testRatingHistory()
	r := newRouter(buildSvc(&repoAdapter{
		list: &stubList{items: []*ratinghistory.RatingHistory{rh}},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/master/counterparty/"+rh.CounterpartyID.String()+"/rating-history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── DELETE /master/rating-history/:id ────────────────────────────────────────

func TestDelete_ApprovedRecord_Returns403(t *testing.T) {
	rh := testRatingHistory()
	rh.WorkflowStatus = ratinghistory.WorkflowStatusApproved
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{rh: rh},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/rating-history/"+rh.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for deleting APPROVED rating history, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── GET /master/rating-history/export ────────────────────────────────────────

func TestExport_CSV_Returns200(t *testing.T) {
	csvData := "\xef\xbb\xbfKode,Rating\r\nRH-001,idA\r\n"
	r := newRouter(buildSvc(&repoAdapter{export: &stubExport{
		reader: strings.NewReader(csvData),
		count:  1,
	}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/rating-history/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── PUT /master/rating-history/:id ───────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	rh := testRatingHistory()
	rh.WorkflowStatus = ratinghistory.WorkflowStatusApproved
	r := newRouter(buildSvc(&repoAdapter{
		getByID: &stubGetByID{rh: rh},
	}))
	body := `{"ratingPefindo":"idAA","rowVersion":1}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/master/rating-history/"+rh.ID.String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for updating APPROVED rating history, got %d; body=%s", rec.Code, rec.Body.String())
	}
}
