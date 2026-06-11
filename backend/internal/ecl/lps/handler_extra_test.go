package lps

// handler_extra_test.go — additional handler coverage for RejectOverride,
// ListOverrides, ApproveOverride success path (service mock, not nil db).

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── RejectOverride handler ──────────────────────────────────────────────────

func TestHandlerRejectOverride_NotPending(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        uuid.New(),
		WorkflowStatus: WorkflowStatusApprovedActive,
		TenantID:       "TUGURE",
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{"rejectReason": "too late"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/reject", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerRejectOverride_BadUUID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{"rejectReason": "reason"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/not-a-uuid/reject", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400. Body: %s", w.Code, w.Body.String())
	}
}

// ─── ListOverrides handler ───────────────────────────────────────────────────

func TestHandlerListOverrides_200(t *testing.T) {
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:              ovID,
		InstrumenID:     uuid.New(),
		ExclusionReason: "Some override reason longer than thirty chars",
		WorkflowStatus:  WorkflowStatusPendingApproval,
		MakerID:         uuid.New(),
		TenantID:        "TUGURE",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		RowVersion:      1,
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/overrides", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	data, ok := resp["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Errorf("expected non-empty data array, got: %v", resp["data"])
	}
}

func TestHandlerListOverrides_EmptyResult(t *testing.T) {
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/overrides?workflow_status=REJECTED", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200. Body: %s", w.Code, w.Body.String())
	}
}

// ─── ApproveOverride handler ─────────────────────────────────────────────────

func TestHandlerApproveOverride_SoDViolation(t *testing.T) {
	// The handler calls the service which checks SoD.
	// Handler injects user_id from context. Set user_id == makerID.
	makerID := uuid.New()
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        makerID,
		WorkflowStatus: WorkflowStatusPendingApproval,
		TenantID:       "TUGURE",
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverrideApprove})
		c.Set("user_id", makerID.String()) // user == maker → SoD violation
		c.Set("mfa_verified", true)
		c.Set("roles", []string{"ROLE-ALCO"})
		c.Set("tenant_id", "TUGURE")
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/:id/approve", h.ApproveOverride)

	body, _ := json.Marshal(map[string]string{"comment": "approved"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (SoD violation). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerApproveOverride_NotPending(t *testing.T) {
	approverID := uuid.New()
	ovID := uuid.New()
	ov := &LPSExclusionOverride{
		ID:             ovID,
		MakerID:        uuid.New(), // different from approver
		WorkflowStatus: WorkflowStatusRejected,
		TenantID:       "TUGURE",
	}
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{ovID: ov}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverrideApprove})
		c.Set("user_id", approverID.String())
		c.Set("mfa_verified", true)
		c.Set("roles", []string{"ROLE-ALCO"})
		c.Set("tenant_id", "TUGURE")
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/:id/approve", h.ApproveOverride)

	body, _ := json.Marshal(map[string]string{"comment": "approved"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422. Body: %s", w.Code, w.Body.String())
	}
}

// ─── SubmitOverride additional validation branches ───────────────────────────

func TestHandlerSubmitOverride_BadInstrumenID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"instrumenId":        "not-a-uuid",
		"alasan":             "This reason is definitely longer than thirty characters",
		"validFromPeriodeId": uuid.New().String(),
		"validToPeriodeId":   uuid.New().String(),
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid instrumenId). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerSubmitOverride_BadFromID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"instrumenId":        uuid.New().String(),
		"alasan":             "This reason is definitely longer than thirty characters",
		"validFromPeriodeId": "not-a-uuid",
		"validToPeriodeId":   uuid.New().String(),
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid validFromPeriodeId). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerSubmitOverride_BadToID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"instrumenId":        uuid.New().String(),
		"alasan":             "This reason is definitely longer than thirty characters",
		"validFromPeriodeId": uuid.New().String(),
		"validToPeriodeId":   "not-a-uuid",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid validToPeriodeId). Body: %s", w.Code, w.Body.String())
	}
}

// ─── GetOverride with approver set (covers toOverrideDTO approverID branch) ──

func TestHandlerGetOverride_WithApprover(t *testing.T) {
	ovID := uuid.New()
	approverID := uuid.New()
	comment := "looks good"
	now := time.Now()
	ov := &LPSExclusionOverride{
		ID:                 ovID,
		InstrumenID:        uuid.New(),
		ExclusionReason:    "Reason longer than 30 chars for this test",
		ValidFromPeriodeID: uuid.New(),
		ValidToPeriodeID:   uuid.New(),
		WorkflowStatus:     WorkflowStatusApprovedActive,
		MakerID:            uuid.New(),
		ApproverID:         &approverID, // approver set → covers toOverrideDTO nil branch
		CommentApprove:     &comment,
		CreatedBy:          uuid.New(),
		UpdatedBy:          uuid.New(),
		TenantID:           "TUGURE",
		CreatedAt:          now,
		UpdatedAt:          now,
		RowVersion:         2,
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
	if data["approverId"] == nil {
		t.Error("expected non-nil approverId in response")
	}
}

// ─── AggregateSingle — service error path ────────────────────────────────────

func TestHandlerAggregateSingle_NoCoverage(t *testing.T) {
	h := newTestHandler(
		&mockCoverageRepo{row: nil},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId":      uuid.New().String(),
		"bankId":         uuid.New().String(),
		"evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422. Body: %s", w.Code, w.Body.String())
	}
}

// ─── ExportPreview — missing date ────────────────────────────────────────────

func TestHandlerExportPreview_MissingDate(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview/export?format=csv", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400. Body: %s", w.Code, w.Body.String())
	}
}

// ─── AggregateBulk handler coverage ─────────────────────────────────────────

func TestHandlerAggregateBulk_202_WithData(t *testing.T) {
	// Same as TestHandlerAggregateBulk_202 in handler_test.go but verifies jobId field.
	// AggregateBulk returns 202 Accepted (async job pattern per ux-patterns §3).
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	h := newTestHandler(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	periodeID := uuid.New().String()
	body, _ := json.Marshal(map[string]string{
		"periodeId":      periodeID,
		"evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202. Body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp) //nolint:errcheck
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected response shape: %v", resp)
	}
	if _, ok := data["jobId"]; !ok {
		t.Error("expected jobId in response")
	}
}

func TestHandlerAggregateBulk_NoCoverage(t *testing.T) {
	// coverage nil + err → handleDomainError → 422.
	h := newTestHandler(
		&mockCoverageRepo{row: nil, err: ErrLPSCoverageNoActiveParam("2026-06-30")},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	periodeID := uuid.New().String()
	body, _ := json.Marshal(map[string]string{
		"periodeId":      periodeID,
		"evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		t.Errorf("got 500, unexpected server error. Body: %s", w.Body.String())
	}
}

// ─── AggregateBulk — bad UUID and bad date ───────────────────────────────────

func TestHandlerAggregateBulk_BadPeriodeID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"periodeId":      "not-a-uuid",
		"evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (bad periodeId). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerAggregateBulk_BadEvalDate(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"periodeId":      uuid.New().String(),
		"evaluationDate": "not-a-date",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate/bulk", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (bad evalDate). Body: %s", w.Code, w.Body.String())
	}
}

// ─── AggregateSingle — bad nasabahId and bad bankId ──────────────────────────

func TestHandlerAggregateSingle_BadNasabahID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId":      "not-a-uuid",
		"bankId":         uuid.New().String(),
		"evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerAggregateSingle_BadBankID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId":      uuid.New().String(),
		"bankId":         "not-a-uuid",
		"evaluationDate": "2026-06-30",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400. Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerAggregateSingle_BadEvalDate(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	body, _ := json.Marshal(map[string]string{
		"nasabahId":      uuid.New().String(),
		"bankId":         uuid.New().String(),
		"evaluationDate": "not-a-date",
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/aggregate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400. Body: %s", w.Code, w.Body.String())
	}
}

// ─── ExportPreview — bad format, invalid date, CSV with data ─────────────────

func TestHandlerExportPreview_BadFormat(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview/export?format=pdf&evaluation_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (bad format). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerExportPreview_InvalidDate(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview/export?format=csv&evaluation_date=bad-date", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (bad date). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerExportPreview_XLSX_Returns202(t *testing.T) {
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	h := newTestHandler(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview/export?format=xlsx&evaluation_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// XLSX always returns 202 (Phase 5 async).
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202 (XLSX async). Body: %s", w.Code, w.Body.String())
	}
}

func TestHandlerExportPreview_CSVWithData(t *testing.T) {
	// Use a nasabah-bank pair so Preview returns at least one aggregate row.
	nasabahID := uuid.New()
	bankID := uuid.New()
	instrID := uuid.New()
	capID := uuid.New()
	capRow := &LPSCoverageRow{
		ID:                capID,
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	bulk := []BulkDepositoRow{{
		InstrumenID:        instrID,
		KodeInstrumen:      "DEP-001",
		TanggalPenempatan:  time.Now().AddDate(-1, 0, 0),
		NasabahID:          nasabahID,
		BankID:             bankID,
		Nominal:            decimal.NewFromInt(500_000_000),
		MataUang:           "IDR",
		KlasifikasiPsak71:  "AC",
		FXRate:             nil,
		LPSCoverageParamID: capID,
		LPSCapIDR:          decimal.NewFromInt(2_000_000_000),
		OverrideID:         nil,
		ExclusionReason:    nil,
		NasabahNama:        "PT Test Nasabah",
		BankNama:           "Bank Test",
		TenantID:           "TUGURE",
	}}
	pairs := []NasabahBankPair{{NasabahID: nasabahID, BankID: bankID}}
	h := newTestHandler(
		&mockCoverageRepo{row: capRow},
		&mockDepositoRepo{bulkRows: bulk, allPairs: pairs},
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
	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header for CSV export")
	}
}

// ─── ApproveOverride — MFA not verified ──────────────────────────────────────

func TestHandlerApproveOverride_NoMFA(t *testing.T) {
	ovID := uuid.New()
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverrideApprove})
		c.Set("user_id", uuid.New().String())
		c.Set("mfa_verified", false) // MFA not done
		c.Set("roles", []string{"ROLE-ALCO"})
		c.Set("tenant_id", "TUGURE")
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/:id/approve", h.ApproveOverride)

	body, _ := json.Marshal(map[string]string{"comment": "approved"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 (MFA required). Body: %s", w.Code, w.Body.String())
	}
}

// ─── RejectOverride — no user ID in context ──────────────────────────────────

func TestHandlerRejectOverride_NoUserID(t *testing.T) {
	ovID := uuid.New()
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverrideReject})
		// user_id NOT set → currentUserID returns false
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/:id/reject", h.RejectOverride)

	body, _ := json.Marshal(map[string]string{"comment": "rejected"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/reject", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401. Body: %s", w.Code, w.Body.String())
	}
}

// ─── SubmitOverride — no user ID in context ───────────────────────────────────

func TestHandlerSubmitOverride_NoUserID(t *testing.T) {
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverride})
		// user_id NOT set → currentUserID returns false
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/submit", h.SubmitOverride)

	body, _ := json.Marshal(map[string]string{
		"instrumenId":        uuid.New().String(),
		"alasan":             "This is a reason that is definitely longer than 30 chars",
		"validFromPeriodeId": uuid.New().String(),
		"validToPeriodeId":   uuid.New().String(),
	})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/submit", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401. Body: %s", w.Code, w.Body.String())
	}
}

// ─── handleDomainError with non-domain error ─────────────────────────────────

func TestHandleDomainError_NonDomainError(t *testing.T) {
	// Covers handleDomainError path where err is NOT a domain error → 500.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		handleDomainError(c, errors.New("unexpected internal failure"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500. Body: %s", w.Code, w.Body.String())
	}
}

// ─── currentUserRole — []interface{} branch ──────────────────────────────────

func TestCurrentUserRole_InterfaceSlice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got string
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		// Set roles as []interface{} (simulates JWT decode without type assertion).
		c.Set("roles", []interface{}{"ROLE-RISK", "ROLE-AKUN"})
		got = currentUserRole(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != "ROLE-RISK" {
		t.Errorf("currentUserRole = %q, want ROLE-RISK", got)
	}
}

// ─── traceID — reads from context ────────────────────────────────────────────

func TestTraceID_FromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got string
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("trace_id", "ctx-trace-abc")
		got = traceID(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != "ctx-trace-abc" {
		t.Errorf("traceID = %q, want ctx-trace-abc", got)
	}
}

// ─── parseLimitQuery — over max and invalid ───────────────────────────────────

func TestParseLimitQuery_OverMax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got int
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		got = parseLimitQuery(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test?limit=500", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != 200 {
		t.Errorf("limit = %d, want 200 (clamped)", got)
	}
}

func TestParseLimitQuery_Invalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got int
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		got = parseLimitQuery(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test?limit=abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != 50 {
		t.Errorf("limit = %d, want 50 (default)", got)
	}
}

// ─── parseDateQuery — optional + not found ───────────────────────────────────

func TestParseDateQuery_Optional_Empty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotOK bool
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		_, gotOK = parseDateQuery(c, "date", false)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !gotOK {
		t.Error("optional missing param should return ok=true")
	}
	_ = w
}

// ─── GetOverride — not found ──────────────────────────────────────────────────

func TestHandlerGetOverride_NotFound(t *testing.T) {
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{}}
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, ovRepo, &mockKursRepo{}, nil)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/overrides/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404. Body: %s", w.Code, w.Body.String())
	}
}

// ─── hasPermission — []interface{} branch ────────────────────────────────────

func TestHasPermission_InterfaceSlice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got bool
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("permissions", []interface{}{PermLPSCompute, "other.perm"})
		got = hasPermission(c, PermLPSCompute)
		if got {
			c.Status(http.StatusOK)
		}
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !got {
		t.Error("hasPermission should return true for []interface{} permissions slice")
	}
	_ = w
}

func TestHasPermission_NoPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		// permissions key exists but doesn't contain the perm
		c.Set("permissions", []string{"other.perm"})
		hasPermission(c, PermLPSCompute) //nolint:errcheck
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

// ─── hasMFAVerified — non-bool value and not-exists ──────────────────────────

func TestHasMFAVerified_NonBoolValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got bool
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("mfa_verified", "true") // string not bool → returns false
		got = hasMFAVerified(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got {
		t.Error("hasMFAVerified should return false for non-bool value")
	}
}

func TestHasMFAVerified_NotExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got bool
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		// mfa_verified not set
		got = hasMFAVerified(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got {
		t.Error("hasMFAVerified should return false if key does not exist")
	}
}

// ─── currentUserID — string branch ───────────────────────────────────────────

func TestCurrentUserID_StringBranch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wantID := uuid.New()
	var gotID uuid.UUID
	var gotOK bool
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("user_id", wantID.String()) // string format
		gotID, gotOK = currentUserID(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !gotOK || gotID != wantID {
		t.Errorf("currentUserID = %s ok=%v, want %s ok=true", gotID, gotOK, wantID)
	}
	_ = w
}

// ─── tenantID — custom tenant ─────────────────────────────────────────────────

func TestTenantID_Custom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got string
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		c.Set("tenant_id", "CUSTOM_TENANT")
		got = tenantID(c)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got != "CUSTOM_TENANT" {
		t.Errorf("tenantID = %q, want CUSTOM_TENANT", got)
	}
	_ = w
}

// ─── ApproveOverride — no user_id → 401 ──────────────────────────────────────

func TestHandlerApproveOverride_NoUserID(t *testing.T) {
	ovID := uuid.New()
	h := newTestHandler(&mockCoverageRepo{}, &mockDepositoRepo{}, &mockOverrideRepo{}, &mockKursRepo{}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("permissions", []string{PermLPSOverrideApprove})
		c.Set("mfa_verified", true)
		// user_id NOT set
		c.Next()
	})
	r.POST("/api/v1/ecl/lps/override/:id/approve", h.ApproveOverride)

	body, _ := json.Marshal(map[string]string{"comment": "approved"})
	req := httptest.NewRequest("POST", "/api/v1/ecl/lps/override/"+ovID.String()+"/approve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 (no user_id). Body: %s", w.Code, w.Body.String())
	}
}

// ─── ListPreview — service error ─────────────────────────────────────────────

func TestHandlerListPreview_ServiceError(t *testing.T) {
	// coverage repo returns error → Preview fails → handleDomainError → 422
	h := newTestHandler(
		&mockCoverageRepo{row: nil, err: ErrLPSCoverageNoActiveParam("2026-06-30")},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
		nil,
	)
	r := setupRouter(h)

	req := httptest.NewRequest("GET", "/api/v1/ecl/lps/preview?evaluation_date=2026-06-30", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusInternalServerError {
		t.Errorf("got 500, expected domain error. Body: %s", w.Body.String())
	}
	if w.Code == http.StatusOK {
		t.Errorf("got 200, expected error status. Body: %s", w.Body.String())
	}
}
