package lps

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestHandler creates a Handler with mock services.
func newTestHandler(
	cov LPSCoverageRepoIface,
	dep DepositoInstrumenRepoIface,
	ovRepo OverrideRepoIface,
	kurs KursRepoIface,
	periodeRepo PeriodeBukuRepoIface,
) *Handler {
	agg := NewAggregatorService(cov, dep, ovRepo, kurs)
	ovSvc := &OverrideService{
		db:           nil,
		overrideRepo: ovRepo,
		periodeRepo:  periodeRepo,
		auditWriter:  &mockAuditWriter{},
	}
	return NewHandler(agg, ovSvc)
}

// setupRouter creates a test Gin router with all permissions injected.
func setupRouter(h *Handler, perms ...string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if len(perms) == 0 {
			c.Set("permissions", []string{
				PermLPSCompute, PermLPSPreview, PermLPSOverride,
				PermLPSOverrideApprove, PermLPSOverrideReject,
			})
		} else {
			c.Set("permissions", perms)
		}
		c.Set("user_id", uuid.New().String())
		c.Set("mfa_verified", true)
		c.Set("roles", []string{"ROLE-ALCO"})
		c.Set("tenant_id", "TUGURE")
		c.Next()
	})
	v1 := r.Group("/api/v1")
	lps := v1.Group("/ecl/lps")
	lps.POST("/aggregate", h.AggregateSingle)
	lps.POST("/aggregate/bulk", h.AggregateBulk)
	lps.GET("/preview", h.ListPreview)
	lps.GET("/preview/export", h.ExportPreview)
	lps.GET("/overrides", h.ListOverrides)
	lps.GET("/overrides/:id", h.GetOverride)
	override := lps.Group("/override")
	override.POST("/submit", h.SubmitOverride)
	override.POST("/:id/approve", h.ApproveOverride)
	override.POST("/:id/reject", h.RejectOverride)
	return r
}

// ─── AggregateSingle tests ────────────────────────────────────────────────────

func TestHandlerAggregateSingle_200(t *testing.T) {
	nasabah := uuid.New()
	bank := uuid.New()
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	h := newTestHandler(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{byNasabahBank: []InstrumenDepositoRow{
			{ID: uuid.New(), KodeInstrumen: "DEP-001", Nominal: decimal.NewFromInt(500_000_000), MataUang: "IDR",
				TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		}},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId": nasabah.String(), "bankId": bank.String(), "evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data := resp["data"].(map[string]interface{})
	if data["totalExposureIdr"] != "500000000.0000" {
		t.Errorf("totalExposureIdr = %v", data["totalExposureIdr"])
	}
	if data["coveredIdr"] != "500000000.0000" {
		t.Errorf("coveredIdr = %v", data["coveredIdr"])
	}
	if data["excessIdr"] != "0.0000" {
		t.Errorf("excessIdr = %v", data["excessIdr"])
	}
}

func TestHandlerAggregateSingle_NoCoverage422(t *testing.T) {
	h := newTestHandler(
		&mockCoverageRepo{row: nil},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId": uuid.New().String(), "bankId": uuid.New().String(), "evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
	errObj := resp["error"].(map[string]interface{})
	if errObj["code"] != CodeLPSCoverageNoActiveParam {
		t.Errorf("code = %v, want %s", errObj["code"], CodeLPSCoverageNoActiveParam)
	}
}

func TestHandlerAggregateSingle_BadUUID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId": "not-a-uuid", "bankId": uuid.New().String(), "evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ─── AggregateBulk tests ──────────────────────────────────────────────────────

func TestHandlerAggregateBulk_202(t *testing.T) {
	h := newTestHandler(
		&mockCoverageRepo{row: &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"periodeId": uuid.New().String(), "evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202. Body: %s", w.Code, w.Body.String())
	}
}

// ─── Preview tests ────────────────────────────────────────────────────────────

func TestHandlerListPreview_200(t *testing.T) {
	capRow := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	nasabah := uuid.New()
	bank := uuid.New()
	h := newTestHandler(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{
			bulkRows: []BulkDepositoRow{
				{
					InstrumenID: uuid.New(), KodeInstrumen: "DEP-001",
					NasabahID: nasabah, BankID: bank,
					Nominal: decimal.NewFromInt(500_000_000), MataUang: "IDR",
					TanggalPenempatan:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					KlasifikasiPsak71:  "AC",
					LPSCoverageParamID: capRow.ID,
					LPSCapIDR:          capRow.CoverageAmountIDR,
					NasabahNama:        "PT Test", BankNama: "Bank Test",
					TenantID: "TUGURE",
				},
			},
			allPairs: []NasabahBankPair{{NasabahID: nasabah, BankID: bank}},
		},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview?evaluation_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerListPreview_MissingDate(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// ─── Override submit tests ────────────────────────────────────────────────────

func TestHandlerSubmitOverride_ReasonTooShort(t *testing.T) {
	fromID := uuid.New()
	toID := uuid.New()
	periodeRepo := &mockPeriodeRepo{
		starts: map[uuid.UUID]time.Time{fromID: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		ends:   map[uuid.UUID]time.Time{toID: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, periodeRepo)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"instrumenId":        uuid.New().String(),
		"alasan":             "short",
		"validFromPeriodeId": fromID.String(),
		"validToPeriodeId":   toID.String(),
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422. Body: %s", w.Code, w.Body.String())
	}
}

// ─── GetOverride tests ────────────────────────────────────────────────────────

func TestHandlerGetOverride_404(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{},
		&mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{}},
		&mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/overrides/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandlerGetOverride_200(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:                 ovID,
		InstrumenID:        uuid.New(),
		ExclusionReason:    "A reason that is at least 30 characters long",
		ValidFromPeriodeID: uuid.New(),
		ValidToPeriodeID:   uuid.New(),
		WorkflowStatus:     WorkflowStatusPendingApproval,
		MakerID:            uuid.New(),
		CreatedBy:          uuid.New(),
		UpdatedBy:          uuid.New(),
		TenantID:           "TUGURE",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
		RowVersion:         1,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/overrides/"+ovID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
	data := resp["data"].(map[string]interface{})
	if data["id"] != ovID.String() {
		t.Errorf("id = %v, want %s", data["id"], ovID)
	}
}

// ─── ForbiddenWithoutPermission ───────────────────────────────────────────────

func TestHandlerAggregateSingle_ForbiddenWithoutPermission(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	// No permissions injected.
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{}) // empty permissions
		c.Set("user_id", uuid.New().String())
		c.Set("mfa_verified", true)
		c.Set("roles", []string{"ROLE-AUDIT"})
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/aggregate", h.AggregateSingle)

	body, _ := json.Marshal(map[string]string{
		"nasabahId": uuid.New().String(), "bankId": uuid.New().String(), "evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

// ─── ApproveOverride MFA check ────────────────────────────────────────────────

func TestHandlerApproveOverride_MFARequired(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        uuid.New(),
		WorkflowStatus: WorkflowStatusPendingApproval,
		TenantID:       "TUGURE",
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverrideApprove})
		c.Set("user_id", uuid.New().String())
		c.Set("mfa_verified", false) // MFA not verified!
		c.Set("roles", []string{"ROLE-ALCO"})
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/:id/approve", h.ApproveOverride)

	body, _ := json.Marshal(map[string]string{"comment": "approved", "signatureMethod": "JWT_STANDARD"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (MFA required)", w.Code)
	}
}

// ─── Export tests ─────────────────────────────────────────────────────────────

func TestHandlerExportPreview_CSV(t *testing.T) {
	capRow := &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}
	nasabah := uuid.New()
	bank := uuid.New()
	h := newTestHandler(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{
			bulkRows: []BulkDepositoRow{
				{
					InstrumenID: uuid.New(), KodeInstrumen: "DEP-001",
					NasabahID: nasabah, BankID: bank,
					Nominal: decimal.NewFromInt(500_000_000), MataUang: "IDR",
					TanggalPenempatan:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
					KlasifikasiPsak71:  "AC",
					LPSCoverageParamID: capRow.ID,
					LPSCapIDR:          capRow.CoverageAmountIDR,
					NasabahNama:        "PT Test", BankNama: "Bank Test",
					TenantID: "TUGURE",
				},
			},
			allPairs: []NasabahBankPair{{NasabahID: nasabah, BankID: bank}},
		},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview/export?format=csv&evaluation_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if cd == "" {
		t.Error("Content-Disposition header missing")
	}
}

// ─── parseSortParam ───────────────────────────────────────────────────────────

func TestParseSortParam(t *testing.T) {
	allowed := []string{"excess_idr", "covered_pct", "created_at"}
	tests := []struct {
		input   string
		wantCol string
		wantDir string
	}{
		{"excess_idr:desc", "excess_idr", "desc"},
		{"covered_pct:asc", "covered_pct", "asc"},
		{"invalid_col:asc", "excess_idr", "desc"}, // falls back to default
		{"", "excess_idr", "desc"},
		{"excess_idr", "excess_idr", "asc"}, // no dir → default asc
	}
	for _, tc := range tests {
		sp := parseSortParam(tc.input, allowed, "excess_idr", "desc")
		if sp.col != tc.wantCol || sp.dir != tc.wantDir {
			t.Errorf("parseSortParam(%q) = (%s,%s), want (%s,%s)",
				tc.input, sp.col, sp.dir, tc.wantCol, tc.wantDir)
		}
	}
}
