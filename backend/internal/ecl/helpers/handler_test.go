// Package helpers — HTTP handler tests.
//
// Tests cover the 10 endpoints using httptest + Gin.
// Stubs are inline to avoid DB dependency.
//
// Verified paths:
//   - 200 happy path per endpoint
//   - 400 missing / invalid query params
//   - 403 missing permission
//   - 413 bulk too large
//   - 422 domain errors (e.g. CCF_INSTRUMEN_TYPE_UNKNOWN)
//   - 202 for export endpoint
package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ─── Stub services ────────────────────────────────────────────────────────────

type stubPDSvc struct {
	pd     decimal.Decimal
	detail PDDetail
	err    error
}

func (s *stubPDSvc) GetPD(_ context.Context, _ uuid.UUID, _ EclStage, _ EclScenario, _ string, _ time.Time) (decimal.Decimal, PDDetail, error) {
	return s.pd, s.detail, s.err
}

type stubLGDSvc struct {
	lgd    decimal.Decimal
	detail LGDDetail
	err    error
}

func (s *stubLGDSvc) GetLGD(_ context.Context, _ uuid.UUID, _ string) (decimal.Decimal, LGDDetail, error) {
	return s.lgd, s.detail, s.err
}

type stubEADSvc struct {
	eadIDR decimal.Decimal
	bd     EADBreakdown
	err    error
}

func (s *stubEADSvc) ComputeEAD(_ context.Context, _ uuid.UUID, _ time.Time) (decimal.Decimal, EADBreakdown, error) {
	return s.eadIDR, s.bd, s.err
}

type stubCCFSvc struct {
	ccf    decimal.Decimal
	detail CCFDetail
	err    error
}

func (s *stubCCFSvc) GetCCF(_ context.Context, _ string) (decimal.Decimal, CCFDetail, error) {
	return s.ccf, s.detail, s.err
}

type stubBulkSvc struct {
	results []BulkResult
	summary BulkSummary
	errors  []BulkError
	skipped []BulkSkipped
	err     error
}

func (s *stubBulkSvc) BulkLookup(_ context.Context, _ []BulkRequest, _ string, _ time.Time) ([]BulkResult, BulkSummary, []BulkError, []BulkSkipped, error) {
	return s.results, s.summary, s.errors, s.skipped, s.err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newTestHandler(pdSvc PDLookupService, lgdSvc LGDLookupService, eadSvc EADService, ccfSvc CCFLookupService, bulkSvc BulkHelperService) *Handler {
	svc := &Services{PD: pdSvc, LGD: lgdSvc, EAD: eadSvc, CCF: ccfSvc, Bulk: bulkSvc}
	return &Handler{svc: svc}
}

func newRouter(h *Handler) *gin.Engine {
	r := gin.New()
	// Test middleware to inject permissions from X-Test-Perms header.
	r.Use(func(c *gin.Context) {
		perms := c.GetHeader("X-Test-Perms")
		if perms != "" {
			c.Set("permissions", strings.Split(perms, ","))
		}
		c.Next()
	})
	RegisterRoutes(r.Group("/api/v1/ecl"), h)
	return r
}

func do(r *gin.Engine, method, url string, body interface{}, perms ...string) *httptest.ResponseRecorder {
	var bodyReader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, url, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	if len(perms) > 0 {
		req.Header.Set("X-Test-Perms", strings.Join(perms, ","))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─── GET /pd tests ────────────────────────────────────────────────────────────

func TestHandler_GetPD_200(t *testing.T) {
	pd := d("0.00367500")
	detail := PDDetail{
		PD: pd, PDBase: d("0.00350000"), RatingUsed: "idAA",
		ImpactPDMultiplier: d("1.05000000"), ImpactMevPDMultiplier: d("1.00000000"),
		NormalMultiplierIsDefault: true, Warnings: []HelperWarning{},
	}
	h := newTestHandler(&stubPDSvc{pd: pd, detail: detail}, nil, nil, nil, nil)
	r := newRouter(h)

	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)

	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetPD_400_MissingInstrumenId(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_GetPD_403_NoPermission(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil) // no perms
	if w.Code != http.StatusForbidden {
		t.Errorf("Status want 403 got %d", w.Code)
	}
}

func TestHandler_GetPD_422_DomainError(t *testing.T) {
	h := newTestHandler(&stubPDSvc{err: domainerrors.New(domainerrors.CodePDLookupRatingMissing, "No rating")}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("Status want 422 got %d", w.Code)
	}
}

// ─── GET /lgd tests ───────────────────────────────────────────────────────────

func TestHandler_GetLGD_200(t *testing.T) {
	detail := LGDDetail{
		LGD: d("0.45000000"), BaseLGD: d("0.45000000"),
		CollateralHaircut: decimal.Zero, LGDEffective: d("0.45000000"),
		PoolUsed: "BANK", TipeCounterparty: "BANK", Warnings: []HelperWarning{},
	}
	h := newTestHandler(nil, &stubLGDSvc{lgd: d("0.45000000"), detail: detail}, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/lgd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GET /ead tests ───────────────────────────────────────────────────────────

func TestHandler_GetEAD_200(t *testing.T) {
	bd := EADBreakdown{
		OutstandingPrincipalFCY: d("1000000000.0000"),
		AccruedInterestFCY:      decimal.Zero,
		CommittedUndrawnFCY:     decimal.Zero,
		CCF:                     decimal.Zero,
		EADFCY:                  d("1000000000.0000"),
		EADIDR:                  d("1000000000.0000"),
		Currency:                "IDR",
		AccruedInterestSource:   "ZERO_FALLBACK",
		Warnings:                []HelperWarning{},
	}
	h := newTestHandler(nil, nil, &stubEADSvc{eadIDR: d("1000000000.0000"), bd: bd}, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/ead?instrumenId=01927f6c-0000-7000-8000-000000000001&evaluationDate=2026-06-30", nil, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GET /ccf tests ───────────────────────────────────────────────────────────

func TestHandler_GetCCF_200(t *testing.T) {
	detail := CCFDetail{CCF: decimal.Zero, Source: "PHASE_1_HARDCODED", Warnings: []HelperWarning{}}
	h := newTestHandler(nil, nil, nil, &stubCCFSvc{ccf: decimal.Zero, detail: detail}, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/ccf?tipeInstrumen=DEPOSITO", nil, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetCCF_422_UnknownType(t *testing.T) {
	h := newTestHandler(nil, nil, nil,
		&stubCCFSvc{err: domainerrors.New(domainerrors.CodeCCFInstrumenTypeUnknown, "Unknown type")},
		nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/ccf?tipeInstrumen=UNKNOWN", nil, PermECLHelpersRead)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("Status want 422 got %d", w.Code)
	}
}

// ─── POST /bulk-lookup tests ──────────────────────────────────────────────────

func TestHandler_BulkLookup_413_TooLarge(t *testing.T) {
	items := make([]BulkRequest, maxBulkItems+1)
	for i := range items {
		items[i] = BulkRequest{InstrumenID: uuid.New()}
	}
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items":          items,
	}
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{})
	r := newRouter(h)
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", body, PermECLHelpersRead)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Status want 413 got %d", w.Code)
	}
}

func TestHandler_BulkLookup_200_Empty(t *testing.T) {
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items":          []BulkRequest{},
	}
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{
		results: []BulkResult{},
		summary: BulkSummary{},
	})
	r := newRouter(h)
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GET /preview/export tests ────────────────────────────────────────────────

func TestHandler_ExportPreview_202(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview/export?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&format=csv", nil, PermECLHelpersPreview)
	if w.Code != http.StatusAccepted {
		t.Errorf("Status want 202 got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]interface{})
	if data == nil || data["jobId"] == nil {
		t.Error("Expected jobId in response")
	}
}

func TestHandler_ExportPreview_403_WrongPermission(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	// ecl_helpers.read is NOT enough for preview/export
	w := do(r, "GET", "/api/v1/ecl/helpers/preview/export?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30", nil, PermECLHelpersRead)
	if w.Code != http.StatusForbidden {
		t.Errorf("Status want 403 got %d", w.Code)
	}
}

// ─── BulkGetPD tests ──────────────────────────────────────────────────────────

func TestHandler_BulkGetPD_200(t *testing.T) {
	pd := d("0.00367500")
	detail := PDDetail{
		PD: pd, PDBase: pd, RatingUsed: "idAA",
		ImpactPDMultiplier: d("1.05000000"), ImpactMevPDMultiplier: d("1.00000000"),
		NormalMultiplierIsDefault: true, Warnings: []HelperWarning{},
	}
	h := newTestHandler(&stubPDSvc{pd: pd, detail: detail}, nil, nil, nil, nil)
	r := newRouter(h)

	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items": []map[string]string{
			{"instrumenId": "01927f6c-0000-7000-8000-000000000001", "stage": "STAGE_1", "scenario": "NORMAL"},
		},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/pd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("BulkGetPD status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_BulkGetPD_413_TooLarge(t *testing.T) {
	items := make([]map[string]string, maxBulkItems+1)
	for i := range items {
		items[i] = map[string]string{
			"instrumenId": uuid.New().String(), "stage": "STAGE_1", "scenario": "NORMAL",
		}
	}
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items":          items,
	}
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "POST", "/api/v1/ecl/helpers/pd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Status want 413 got %d", w.Code)
	}
}

func TestHandler_BulkGetPD_200_InvalidUUID_InResult(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items": []map[string]string{
			{"instrumenId": "not-a-uuid", "stage": "STAGE_1", "scenario": "NORMAL"},
		},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/pd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
	// Item should have error field set.
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	results := data["results"].([]interface{})
	item := results[0].(map[string]interface{})
	if item["error"] == nil {
		t.Error("Expected error field for invalid UUID")
	}
}

// ─── BulkGetLGD tests ─────────────────────────────────────────────────────────

func TestHandler_BulkGetLGD_200(t *testing.T) {
	detail := LGDDetail{
		LGD: d("0.45000000"), BaseLGD: d("0.45000000"),
		LGDEffective: d("0.45000000"), PoolUsed: "BANK",
		Warnings: []HelperWarning{},
	}
	h := newTestHandler(nil, &stubLGDSvc{lgd: d("0.45000000"), detail: detail}, nil, nil, nil)
	r := newRouter(h)

	body := map[string]interface{}{
		"periodeId": "PBUKU-2026-06",
		"items": []map[string]string{
			{"instrumenId": "01927f6c-0000-7000-8000-000000000001"},
		},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/lgd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("BulkGetLGD status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_BulkGetLGD_413(t *testing.T) {
	items := make([]map[string]string, maxBulkItems+1)
	for i := range items {
		items[i] = map[string]string{"instrumenId": uuid.New().String()}
	}
	h := newTestHandler(nil, &stubLGDSvc{}, nil, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId": "PBUKU-2026-06",
		"items":     items,
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/lgd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Status want 413 got %d", w.Code)
	}
}

// ─── BulkGetEAD tests ─────────────────────────────────────────────────────────

func TestHandler_BulkGetEAD_200(t *testing.T) {
	bd := EADBreakdown{
		EADIDR: d("1000000000.0000"), Currency: "IDR",
		Warnings: []HelperWarning{},
	}
	h := newTestHandler(nil, nil, &stubEADSvc{eadIDR: d("1000000000.0000"), bd: bd}, nil, nil)
	r := newRouter(h)

	body := map[string]interface{}{
		"evaluationDate": "2026-06-30",
		"items": []map[string]string{
			{"instrumenId": "01927f6c-0000-7000-8000-000000000001"},
		},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/ead/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("BulkGetEAD status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_BulkGetEAD_413(t *testing.T) {
	items := make([]map[string]string, maxBulkItems+1)
	for i := range items {
		items[i] = map[string]string{"instrumenId": uuid.New().String()}
	}
	h := newTestHandler(nil, nil, &stubEADSvc{}, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"evaluationDate": "2026-06-30",
		"items":          items,
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/ead/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Status want 413 got %d", w.Code)
	}
}

// ─── GetPreview tests ─────────────────────────────────────────────────────────

func TestHandler_GetPreview_200_NilRepo_EmptyList(t *testing.T) {
	// previewRepo is nil when newTestHandler is used — should return empty list.
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30", nil, PermECLHelpersPreview)
	if w.Code != http.StatusOK {
		t.Errorf("GetPreview nil-repo status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetPreview_403_NoPermission(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30", nil, PermECLHelpersRead)
	if w.Code != http.StatusForbidden {
		t.Errorf("GetPreview 403 want 403 got %d", w.Code)
	}
}

func TestHandler_GetPreview_400_MissingPeriode(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview?evaluationDate=2026-06-30", nil, PermECLHelpersPreview)
	if w.Code != http.StatusBadRequest {
		t.Errorf("GetPreview 400 want 400 got %d", w.Code)
	}
}

// ─── NewHandler / NewServices tests ──────────────────────────────────────────

func TestNewHandler_WiresPreviewRepo(t *testing.T) {
	// instrRepo that implements previewInstrumentLister via nil-DB.
	instrRepo := NewDBInstrumenSnapshotRepo(nil)
	svc := &Services{
		PD:        &stubPDSvc{},
		LGD:       &stubLGDSvc{},
		EAD:       &stubEADSvc{},
		CCF:       &stubCCFSvc{},
		Bulk:      &stubBulkSvc{},
		instrRepo: instrRepo,
	}

	h := NewHandler(svc)
	// DBInstrumenSnapshotRepo implements previewInstrumentLister.
	// The preview repo should be wired (it will return nil results on nil DB calls).
	if h.previewRepo == nil {
		t.Error("Expected previewRepo to be wired when instrRepo implements previewInstrumentLister")
	}
}

func TestNewHandler_NoPreviewRepo_WhenInstrRepoNil(t *testing.T) {
	svc := &Services{
		PD: &stubPDSvc{}, LGD: &stubLGDSvc{},
		EAD: &stubEADSvc{}, CCF: &stubCCFSvc{}, Bulk: &stubBulkSvc{},
	}
	h := NewHandler(svc)
	// svc.instrRepo is nil → no previewRepo.
	if h.previewRepo != nil {
		t.Error("Expected previewRepo to be nil when instrRepo is nil")
	}
}

// ─── parseLimitParam / domainErrMsg / parseStage / parseScenario coverage ────

func TestHandler_GetLGD_400_MissingPeriodeId(t *testing.T) {
	h := newTestHandler(nil, &stubLGDSvc{}, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/lgd?instrumenId=01927f6c-0000-7000-8000-000000000001", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_GetEAD_400_BadDate(t *testing.T) {
	h := newTestHandler(nil, nil, &stubEADSvc{}, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/ead?instrumenId=01927f6c-0000-7000-8000-000000000001&evaluationDate=not-a-date", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_GetCCF_400_MissingTipe(t *testing.T) {
	h := newTestHandler(nil, nil, nil, &stubCCFSvc{}, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/ccf", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_GetPD_400_InvalidStage(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=INVALID&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_GetPD_400_InvalidScenario(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=INVALID&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkLookup_200_SingleItem(t *testing.T) {
	results := []BulkResult{{
		InstrumenID: uuid.New(),
		PDGood:      d("0.00367500"),
		PDNormal:    d("0.00367500"),
		PDBad:       d("0.00367500"),
		LGD:         d("0.45000000"),
		EADIDR:      d("1000000000.0000"),
	}}
	summary := BulkSummary{Total: 1, Success: 1}
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{results: results, summary: summary})
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items":          []map[string]interface{}{{"instrumenId": uuid.New().String()}},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetPD_500_InternalError(t *testing.T) {
	h := newTestHandler(&stubPDSvc{err: fmt.Errorf("unexpected db error")}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status want 500 got %d", w.Code)
	}
}

// ─── Additional handler coverage ─────────────────────────────────────────────

func TestHandler_GetLGD_422_DomainError(t *testing.T) {
	h := newTestHandler(nil,
		&stubLGDSvc{err: domainerrors.New(domainerrors.CodeLGDLookupMappingNotFound, "no mapping")},
		nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/lgd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("GetLGD 422 want 422 got %d", w.Code)
	}
}

func TestHandler_GetLGD_500_InternalError(t *testing.T) {
	h := newTestHandler(nil,
		&stubLGDSvc{err: fmt.Errorf("db error")},
		nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/lgd?instrumenId=01927f6c-0000-7000-8000-000000000001&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("GetLGD 500 want 500 got %d", w.Code)
	}
}

func TestHandler_GetEAD_422_DomainError(t *testing.T) {
	h := newTestHandler(nil, nil,
		&stubEADSvc{err: domainerrors.New(domainerrors.CodeEADFXRateMissing, "no FX rate")},
		nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/ead?instrumenId=01927f6c-0000-7000-8000-000000000001&evaluationDate=2026-06-30", nil, PermECLHelpersRead)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("GetEAD 422 want 422 got %d", w.Code)
	}
}

func TestHandler_GetPD_400_MissingPeriodeId(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_GetPD_400_MissingEvalDate(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&periodeId=PBUKU-2026-06", nil, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkGetPD_400_BadBody(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	// Missing required fields
	w := do(r, "POST", "/api/v1/ecl/helpers/pd/bulk", map[string]interface{}{}, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkGetPD_200_DomainError_InResult(t *testing.T) {
	// Domain error during PD lookup for one item → stored in error field of result item.
	h := newTestHandler(
		&stubPDSvc{err: domainerrors.New(domainerrors.CodePDLookupRatingMissing, "no rating")},
		nil, nil, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items": []map[string]string{
			{"instrumenId": "01927f6c-0000-7000-8000-000000000001", "stage": "STAGE_1", "scenario": "NORMAL"},
		},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/pd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
}

func TestHandler_BulkGetPD_200_InvalidStage_InResult(t *testing.T) {
	h := newTestHandler(&stubPDSvc{}, nil, nil, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items": []map[string]string{
			{"instrumenId": "01927f6c-0000-7000-8000-000000000001", "stage": "INVALID", "scenario": "NORMAL"},
		},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/pd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
}

func TestHandler_BulkGetLGD_200_DomainError_InResult(t *testing.T) {
	h := newTestHandler(nil,
		&stubLGDSvc{err: domainerrors.New(domainerrors.CodeLGDLookupMappingNotFound, "no map")},
		nil, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId": "PBUKU-2026-06",
		"items":     []map[string]string{{"instrumenId": "01927f6c-0000-7000-8000-000000000001"}},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/lgd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
}

func TestHandler_BulkGetLGD_400_BadBody(t *testing.T) {
	h := newTestHandler(nil, &stubLGDSvc{}, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "POST", "/api/v1/ecl/helpers/lgd/bulk", map[string]interface{}{}, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkGetLGD_200_InvalidUUID_InResult(t *testing.T) {
	h := newTestHandler(nil, &stubLGDSvc{}, nil, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId": "PBUKU-2026-06",
		"items":     []map[string]string{{"instrumenId": "not-uuid"}},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/lgd/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
}

func TestHandler_BulkGetEAD_400_BadBody(t *testing.T) {
	h := newTestHandler(nil, nil, &stubEADSvc{}, nil, nil)
	r := newRouter(h)
	w := do(r, "POST", "/api/v1/ecl/helpers/ead/bulk", map[string]interface{}{}, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkGetEAD_400_BadEvalDate(t *testing.T) {
	h := newTestHandler(nil, nil, &stubEADSvc{}, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"evaluationDate": "bad-date",
		"items":          []map[string]string{{"instrumenId": "01927f6c-0000-7000-8000-000000000001"}},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/ead/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkGetEAD_200_DomainError_InResult(t *testing.T) {
	h := newTestHandler(nil, nil,
		&stubEADSvc{err: domainerrors.New(domainerrors.CodeEADFXRateMissing, "no fx")},
		nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"evaluationDate": "2026-06-30",
		"items":          []map[string]string{{"instrumenId": "01927f6c-0000-7000-8000-000000000001"}},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/ead/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
}

func TestHandler_BulkGetEAD_200_InvalidUUID_InResult(t *testing.T) {
	h := newTestHandler(nil, nil, &stubEADSvc{}, nil, nil)
	r := newRouter(h)
	body := map[string]interface{}{
		"evaluationDate": "2026-06-30",
		"items":          []map[string]string{{"instrumenId": "not-uuid"}},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/ead/bulk", body, PermECLHelpersRead)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 got %d", w.Code)
	}
}

func TestHandler_ExportPreview_400_MissingPeriode(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview/export?evaluationDate=2026-06-30&format=csv", nil, PermECLHelpersPreview)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_ExportPreview_400_MissingEvalDate(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview/export?periodeId=PBUKU-2026-06&format=csv", nil, PermECLHelpersPreview)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_ExportPreview_400_BadFormat(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, nil)
	r := newRouter(h)
	w := do(r, "GET", "/api/v1/ecl/helpers/preview/export?periodeId=PBUKU-2026-06&evaluationDate=2026-06-30&format=pdf", nil, PermECLHelpersPreview)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkLookup_403(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{})
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items":          []map[string]interface{}{},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", body) // no perms
	if w.Code != http.StatusForbidden {
		t.Errorf("Status want 403 got %d", w.Code)
	}
}

func TestHandler_BulkLookup_400_BadBody(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{})
	r := newRouter(h)
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", map[string]interface{}{}, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkLookup_400_BadEvalDate(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{})
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "not-a-date",
		"items":          []map[string]interface{}{},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", body, PermECLHelpersRead)
	if w.Code != http.StatusBadRequest {
		t.Errorf("Status want 400 got %d", w.Code)
	}
}

func TestHandler_BulkLookup_500_ServiceError(t *testing.T) {
	h := newTestHandler(nil, nil, nil, nil, &stubBulkSvc{err: fmt.Errorf("db error")})
	r := newRouter(h)
	body := map[string]interface{}{
		"periodeId":      "PBUKU-2026-06",
		"evaluationDate": "2026-06-30",
		"items":          []map[string]interface{}{},
	}
	w := do(r, "POST", "/api/v1/ecl/helpers/bulk-lookup", body, PermECLHelpersRead)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Status want 500 got %d", w.Code)
	}
}

func TestHandler_buildPDResult_WithSourcePD12M(t *testing.T) {
	// buildPDResult with SourcePD12M set (Stage 1 path).
	instrID := uuid.MustParse("01927f6c-0000-7000-8000-000000000001")
	pd12 := d("0.00350000")
	detail := PDDetail{
		PD: d("0.00367500"), PDBase: d("0.00350000"), RatingUsed: "idAA",
		ImpactPDMultiplier: d("1.05000000"), ImpactMevPDMultiplier: d("1.00000000"),
		NormalMultiplierIsDefault: true,
		SourcePD12M:               &pd12,
		Warnings:                  []HelperWarning{},
	}
	result := buildPDResult(instrID, Stage1, ScenarioNormal, detail)
	if result == nil {
		t.Fatal("Expected non-nil result from buildPDResult")
	}
}

func TestHandler_buildPDResult_WithSourcePDLifetime(t *testing.T) {
	// buildPDResult with SourcePDLifetime set (Stage 2 path).
	instrID := uuid.MustParse("01927f6c-0000-7000-8000-000000000001")
	lifetime := d("0.02500000")
	detail := PDDetail{
		PD: d("0.02625000"), PDBase: d("0.02500000"), RatingUsed: "idAA",
		ImpactPDMultiplier: d("1.05000000"), ImpactMevPDMultiplier: d("1.00000000"),
		NormalMultiplierIsDefault: true,
		SourcePDLifetime:          &lifetime,
		Warnings:                  []HelperWarning{},
	}
	result := buildPDResult(instrID, Stage2, ScenarioNormal, detail)
	if result == nil {
		t.Fatal("Expected non-nil result from buildPDResult")
	}
}

func TestHandler_hasPermission_PermissionsAsInterface(t *testing.T) {
	// Tests the []interface{} branch in hasPermission.
	h := newTestHandler(&stubPDSvc{pd: d("0.003"), detail: PDDetail{Warnings: []HelperWarning{}}}, nil, nil, nil, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Inject as []interface{} instead of []string.
		c.Set("permissions", []interface{}{PermECLHelpersRead, "other.perm"})
		c.Next()
	})
	RegisterRoutes(r.Group("/api/v1/ecl"), h)

	req, _ := http.NewRequest("GET",
		"/api/v1/ecl/helpers/pd?instrumenId=01927f6c-0000-7000-8000-000000000001&stage=STAGE_1&scenario=NORMAL&evaluationDate=2026-06-30&periodeId=PBUKU-2026-06",
		nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Status want 200 with []interface{} perms, got %d: %s", w.Code, w.Body.String())
	}
}
