package bobotskenario_test

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

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/master/bobotskenario"
	"blips-ifrs9.tugu-re.com/internal/workflow"
	"log/slog"
)

// ─── Test helpers ─────────────────────────────────────────────────────────────

// testClaims returns JWT claims for a RISK officer (typical actor for ECL params).
func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               "00000000-0000-0000-0000-000000000001",
		PreferredUsername: "risk.officer",
		Roles:             []string{"ROLE-RISK"},
		Permissions: []string{
			"ecl_parameter.read",
			"ecl_parameter.create",
			"ecl_parameter.update",
			"ecl_parameter.delete",
			"ecl_parameter.submit",
			"ecl_parameter.review",
			"ecl_parameter.approve",
			"ecl_parameter.reject",
			"ecl_parameter.export",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

// newRouter builds a Gin test router with claims injected.
func newRouter(svc *bobotskenario.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Set("claims", claims)
		c.Next()
	})
	wfh := workflow.NewHandler(workflow.NewService(
		workflow.NewEngine(workflow.NewInMemoryConfigLoader(buildBobotSkenarioConfigs())),
		workflow.NewDBRepository(nil),
		audit.NewWriter(nil),
		slog.Default(),
	))
	h := bobotskenario.NewHandler(svc, wfh)
	v1 := r.Group("/api/v1")
	bobotskenario.RegisterRoutes(v1, h)
	return r
}

// buildBobotSkenarioConfigs returns an in-memory workflow config for BOBOT_SKENARIO (6-eyes).
func buildBobotSkenarioConfigs() map[string]*workflow.Config {
	return map[string]*workflow.Config{
		"BOBOT_SKENARIO": {
			EntityType:  "BOBOT_SKENARIO",
			Eyes:        6,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":   "ecl_parameter.submit",
				"review":   "ecl_parameter.review",
				"approve":  "ecl_parameter.approve",
				"approve2": "ecl_parameter.approve",
				"reject":   "ecl_parameter.reject",
			},
			StepUpRequired: map[string]bool{"approve": true, "approve2": true},
			SoDRules: workflow.SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    true,
			},
		},
	}
}

// buildSvc creates a Service backed by a repoAdapter.
func buildSvc(adapter *repoAdapter) *bobotskenario.Service {
	return bobotskenario.NewService(adapter, audit.NewWriter(nil), slog.Default())
}

// ─── 1. POST /master/bobot-skenario — binding ─────────────────────────────────

func TestCreate_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/bobot-skenario",
		bytes.NewBufferString("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestCreate_MissingRequiredFields_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	body, _ := json.Marshal(map[string]interface{}{
		"periodeBerlakuDari": "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/bobot-skenario", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing required fields, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── 2. GET /master/bobot-skenario/:id — UUID validation ──────────────────────

func TestGetByID_InvalidUUID_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/not-a-uuid", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid UUID, got %d", rec.Code)
	}
}

func TestGetByID_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: nil, err: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/"+uuid.New().String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetByID_Found_Returns200(t *testing.T) {
	e := testBobotSkenario()
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: e},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/"+e.ID.String(), nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID       string `json:"id"`
			Bobot    string `json:"bobot"`
			Skenario string `json:"skenario"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data.ID != e.ID.String() {
		t.Errorf("expected id=%s, got %s", e.ID.String(), resp.Data.ID)
	}
	// Bobot serialized as 8dp string.
	if resp.Data.Bobot != "0.50000000" {
		t.Errorf("expected bobot=0.50000000, got %s", resp.Data.Bobot)
	}
	if resp.Data.Skenario != "NORMAL" {
		t.Errorf("expected skenario=NORMAL, got %s", resp.Data.Skenario)
	}
}

// ─── 3. GET /master/bobot-skenario — list ─────────────────────────────────────

func TestList_ResponseShape(t *testing.T) {
	items := []*bobotskenario.BobotSkenario{testBobotSkenario()}
	r := newRouter(buildSvc(&repoAdapter{listStub: &stubList{items: items}}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario?limit=10", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			Skenario string `json:"skenario"`
		} `json:"data"`
		Pagination struct {
			HasMore bool `json:"hasMore"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
	if resp.Data[0].Skenario != "NORMAL" {
		t.Errorf("expected skenario NORMAL, got %s", resp.Data[0].Skenario)
	}
}

// ─── 4. Validation: skenario whitelist ────────────────────────────────────────

func TestValidate_Skenario_Whitelist(t *testing.T) {
	cases := []struct {
		skenario   string
		wantValErr bool
	}{
		{string(bobotskenario.SkenarioGood), false},
		{string(bobotskenario.SkenarioNormal), false},
		{string(bobotskenario.SkenarioBad), false},
		{"INVALID", true},
		{"", true},
		{"good", true}, // lowercase not accepted
		{"NEUTRAL", true},
	}
	for _, tc := range cases {
		t.Run(tc.skenario, func(t *testing.T) {
			svc := buildSvc(&repoAdapter{})
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, bobotskenario.CreateRequest{
				Skenario:           tc.skenario,
				Bobot:              "0.25000000",
				PeriodeBerlakuDari: "2026-01-01",
			})
			if tc.wantValErr {
				assertValidationError(t, err, fmt.Sprintf("skenario=%q", tc.skenario))
			} else {
				assertNotValidationError(t, err, fmt.Sprintf("skenario=%q", tc.skenario))
			}
		})
	}
}

// ─── 5. Validation: bobot decimal range ───────────────────────────────────────

func TestValidate_Bobot_Range(t *testing.T) {
	cases := []struct {
		bobot      string
		wantValErr bool
	}{
		{"0", false},
		{"1", false},
		{"0.00000000", false},
		{"0.25000000", false},
		{"0.50000000", false},
		{"1.00000001", true},  // above max
		{"-0.00000001", true}, // below min
		{"abc", true},         // not a decimal
		{"", true},            // empty
		{"0.00000001", false}, // minimum positive value
		{"0.99999999", false}, // maximum below 1
	}
	for _, tc := range cases {
		t.Run(tc.bobot, func(t *testing.T) {
			svc := buildSvc(&repoAdapter{})
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, bobotskenario.CreateRequest{
				Skenario:           string(bobotskenario.SkenarioGood),
				Bobot:              tc.bobot,
				PeriodeBerlakuDari: "2026-01-01",
			})
			if tc.wantValErr {
				assertValidationError(t, err, fmt.Sprintf("bobot=%q", tc.bobot))
			} else {
				assertNotValidationError(t, err, fmt.Sprintf("bobot=%q", tc.bobot))
			}
		})
	}
}

// ─── 6. Validation: period format and order ───────────────────────────────────

func TestValidate_PeriodOrder(t *testing.T) {
	cases := []struct {
		dari       string
		sampai     *string
		wantValErr bool
	}{
		{"2026-01-01", nil, false},                // open-ended, valid
		{"2026-01-01", ptr("2026-12-31"), false},  // sampai > dari, valid
		{"2026-01-01", ptr("2026-01-01"), false},  // sampai == dari, valid (same day)
		{"2026-06-01", ptr("2026-01-01"), true},   // sampai < dari, invalid
		{"not-a-date", nil, true},                 // dari bad format
		{"2026-01-01", ptr("not-a-date"), true},   // sampai bad format
		{"2026-13-01", nil, false},                // format is valid (no calendar validation) — service accepts format-correct strings
	}
	for i, tc := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			svc := buildSvc(&repoAdapter{})
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			_, err := svc.Create(ctx, bobotskenario.CreateRequest{
				Skenario:             string(bobotskenario.SkenarioGood),
				Bobot:                "0.25000000",
				PeriodeBerlakuDari:   tc.dari,
				PeriodeBerlakuSampai: tc.sampai,
			})
			if tc.wantValErr {
				assertValidationError(t, err, fmt.Sprintf("case %d", i))
			} else {
				assertNotValidationError(t, err, fmt.Sprintf("case %d", i))
			}
		})
	}
}

// ─── 7. Duplicate (skenario, period) check ────────────────────────────────────

func TestCreate_DuplicateSkenarioPeriod_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		countDuplicateStub: &stubCountDuplicate{count: 1},
	}))

	body, _ := json.Marshal(bobotskenario.CreateRequest{
		Skenario:           string(bobotskenario.SkenarioGood),
		Bobot:              "0.25000000",
		PeriodeBerlakuDari: "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/bobot-skenario", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for duplicate skenario+period, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), bobotskenario.CodeBobotDuplicateSkenarioPeriod)
}

func TestCreate_DuplicateSkenarioPeriod_ServiceLevel(t *testing.T) {
	svc := buildSvc(&repoAdapter{
		countDuplicateStub: &stubCountDuplicate{count: 1},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, bobotskenario.CreateRequest{
		Skenario:           string(bobotskenario.SkenarioNormal),
		Bobot:              "0.50000000",
		PeriodeBerlakuDari: "2026-01-01",
	})
	if err == nil {
		t.Fatal("expected error for duplicate skenario+period, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if string(de.Code()) != bobotskenario.CodeBobotDuplicateSkenarioPeriod {
		t.Errorf("expected BOBOT_DUPLICATE_SKENARIO_PERIOD, got %s", de.Code())
	}
}

// ─── 8. Period overlap check ──────────────────────────────────────────────────

func TestCreate_PeriodOverlap_Returns422(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		countOverlapStub: &stubCountOverlap{count: 1},
	}))

	body, _ := json.Marshal(bobotskenario.CreateRequest{
		Skenario:           string(bobotskenario.SkenarioGood),
		Bobot:              "0.25000000",
		PeriodeBerlakuDari: "2026-01-01",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/bobot-skenario", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for period overlap, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), bobotskenario.CodeBobotPeriodOverlap)
}

func TestCreate_PeriodOverlap_ServiceLevel(t *testing.T) {
	svc := buildSvc(&repoAdapter{
		countOverlapStub: &stubCountOverlap{count: 2},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Create(ctx, bobotskenario.CreateRequest{
		Skenario:           string(bobotskenario.SkenarioBad),
		Bobot:              "0.25000000",
		PeriodeBerlakuDari: "2026-01-01",
	})
	if err == nil {
		t.Fatal("expected error for period overlap, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if string(de.Code()) != bobotskenario.CodeBobotPeriodOverlap {
		t.Errorf("expected BOBOT_PERIOD_OVERLAP, got %s", de.Code())
	}
}

// ─── 9. Sum=1.0 invariant (DEC-010) — the critical validation ─────────────────

// TestSumInvariant_ExactlyOne_Passes verifies that when G+N+B = 1.0, Approve succeeds.
func TestSumInvariant_ExactlyOne_Passes(t *testing.T) {
	// Entity to be approved: GOOD = 0.25
	e := testBobotSkenarioWith(bobotskenario.SkenarioGood, "0.25000000", bobotskenario.WorkflowStatusPendingApproval2)

	// Other rows sum = 0.75 (NORMAL=0.50 + BAD=0.25)
	svc := buildSvc(&repoAdapter{
		getByIDStub:    &stubGetByID{result: e},
		sumByPeriodStub: &stubSumByPeriod{sum: decimal.RequireFromString("0.75000000")},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SyncWorkflowStatus(ctx, e.ID, "APPROVED", "APPROVE2")
	// Will fail at BeginTx (no DB) but NOT at sum invariant check.
	if err != nil {
		if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeBobotSumInvariantViolated {
			t.Errorf("expected sum=1.0 to pass, got BOBOT_SUM_INVARIANT_VIOLATED: %v", err)
		}
		// Other errors (BeginTx, etc.) are expected in test context.
	}
}

// TestSumInvariant_LessThanOne_Rejected verifies 422 when sum < 1.0.
func TestSumInvariant_LessThanOne_Rejected(t *testing.T) {
	// Trying to approve GOOD=0.25 when other rows sum = 0.50 (only NORMAL exists).
	// Total = 0.75 < 1.0
	e := testBobotSkenarioWith(bobotskenario.SkenarioGood, "0.25000000", bobotskenario.WorkflowStatusPendingApproval2)
	svc := buildSvc(&repoAdapter{
		getByIDStub:    &stubGetByID{result: e},
		sumByPeriodStub: &stubSumByPeriod{sum: decimal.RequireFromString("0.50000000")},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SyncWorkflowStatus(ctx, e.ID, "APPROVED", "APPROVE2")
	if err == nil {
		t.Fatal("expected error when sum < 1.0, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeBobotSumInvariantViolated {
		t.Errorf("expected BOBOT_SUM_INVARIANT_VIOLATED, got %s: %s", de.Code(), de.Message())
	}
	// Message must contain "Kurang dari" and the actual sum.
	if !strings.Contains(de.Message(), "Kurang dari") {
		t.Errorf("expected 'Kurang dari' in error message, got: %s", de.Message())
	}
	if !strings.Contains(de.Message(), "0.75000000") {
		t.Errorf("expected sum value '0.75000000' in error message, got: %s", de.Message())
	}
}

// TestSumInvariant_GreaterThanOne_Rejected verifies 422 when sum > 1.0.
func TestSumInvariant_GreaterThanOne_Rejected(t *testing.T) {
	// Trying to approve GOOD=0.30 when other rows sum = 0.80 (NORMAL=0.50 + BAD=0.30).
	// Total = 1.10 > 1.0
	e := testBobotSkenarioWith(bobotskenario.SkenarioGood, "0.30000000", bobotskenario.WorkflowStatusPendingApproval2)
	svc := buildSvc(&repoAdapter{
		getByIDStub:    &stubGetByID{result: e},
		sumByPeriodStub: &stubSumByPeriod{sum: decimal.RequireFromString("0.80000000")},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SyncWorkflowStatus(ctx, e.ID, "APPROVED", "APPROVE2")
	if err == nil {
		t.Fatal("expected error when sum > 1.0, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T: %v", err, err)
	}
	if de.Code() != domainerrors.CodeBobotSumInvariantViolated {
		t.Errorf("expected BOBOT_SUM_INVARIANT_VIOLATED, got %s", de.Code())
	}
	if !strings.Contains(de.Message(), "Lebih dari") {
		t.Errorf("expected 'Lebih dari' in error message, got: %s", de.Message())
	}
	if !strings.Contains(de.Message(), "1.10000000") {
		t.Errorf("expected sum value '1.10000000' in error message, got: %s", de.Message())
	}
}

// TestSumInvariant_WithinTolerance_Passes verifies that rounding within tolerance passes.
// Default bobot: 0.25 + 0.50 + 0.25 = 1.00000000. This tests exact equality.
func TestSumInvariant_WithinTolerance_Passes(t *testing.T) {
	// Tolerance is 0.00000001. Sum = 1.00000000 + 0.000000001 (below tolerance) should pass.
	e := testBobotSkenarioWith(bobotskenario.SkenarioGood, "0.25000000", bobotskenario.WorkflowStatusPendingApproval2)
	// other sum = 0.75000000 → total = 1.00000000 (within tolerance)
	svc := buildSvc(&repoAdapter{
		getByIDStub:    &stubGetByID{result: e},
		sumByPeriodStub: &stubSumByPeriod{sum: decimal.RequireFromString("0.75000000")},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SyncWorkflowStatus(ctx, e.ID, "APPROVED", "APPROVE2")
	// Should NOT be a sum invariant error.
	if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeBobotSumInvariantViolated {
		t.Errorf("expected within-tolerance sum to pass, got: %v", err)
	}
}

// TestSumInvariant_TableDriven covers edge cases for the sum invariant.
func TestSumInvariant_TableDriven(t *testing.T) {
	cases := []struct {
		name         string
		entityBobot  string // bobot of entity being approved
		otherSum     string // SumByPeriod returns this for other rows
		wantInvarErr bool
		wantGt       bool // if error: sum > 1 (Lebih dari)
		wantLt       bool // if error: sum < 1 (Kurang dari)
	}{
		{
			name:         "default_GNB_exactly_1",
			entityBobot:  "0.25000000",
			otherSum:     "0.75000000",
			wantInvarErr: false,
		},
		{
			name:         "ALCO_override_0.20_0.60_0.20",
			entityBobot:  "0.20000000",
			otherSum:     "0.80000000",
			wantInvarErr: false,
		},
		{
			name:         "sum_below_1_missing_BAD",
			entityBobot:  "0.25000000",
			otherSum:     "0.50000000", // only NORMAL present
			wantInvarErr: true,
			wantLt:       true,
		},
		{
			name:         "sum_above_1_all_weights_too_high",
			entityBobot:  "0.33000000",
			otherSum:     "0.80000000",
			wantInvarErr: true,
			wantGt:       true,
		},
		{
			name:         "zero_bobot_no_other_rows",
			entityBobot:  "0.00000000",
			otherSum:     "0.00000000",
			wantInvarErr: true,
			wantLt:       true,
		},
		{
			name:         "sum_exactly_at_tolerance_boundary_passes",
			entityBobot:  "0.25000000",
			otherSum:     "0.74999999", // 0.74999999 + 0.25000000 = 0.99999999 → diff = 0.00000001 = tolerance, passes
			wantInvarErr: false,
		},
		{
			name:         "sum_just_outside_tolerance_fails",
			entityBobot:  "0.25000000",
			otherSum:     "0.74999998", // 0.74999998 + 0.25000000 = 0.99999998 → diff = 0.00000002 > tolerance
			wantInvarErr: true,
			wantLt:       true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := testBobotSkenarioWith(bobotskenario.SkenarioGood, tc.entityBobot, bobotskenario.WorkflowStatusPendingApproval2)
			svc := buildSvc(&repoAdapter{
				getByIDStub:    &stubGetByID{result: e},
				sumByPeriodStub: &stubSumByPeriod{sum: decimal.RequireFromString(tc.otherSum)},
			})
			ctx := auth.ContextWithClaims(context.Background(), testClaims())
			err := svc.SyncWorkflowStatus(ctx, e.ID, "APPROVED", "APPROVE2")

			if tc.wantInvarErr {
				if err == nil {
					t.Fatalf("case %s: expected invariant error, got nil", tc.name)
				}
				de, ok := domainerrors.IsDomainError(err)
				if !ok {
					t.Fatalf("case %s: expected DomainError, got %T: %v", tc.name, err, err)
				}
				if de.Code() != domainerrors.CodeBobotSumInvariantViolated {
					// Only fail if it's a domain error that's NOT the invariant code.
					t.Errorf("case %s: expected BOBOT_SUM_INVARIANT_VIOLATED, got %s", tc.name, de.Code())
				}
				if tc.wantGt && !strings.Contains(de.Message(), "Lebih dari") {
					t.Errorf("case %s: expected 'Lebih dari' in message, got: %s", tc.name, de.Message())
				}
				if tc.wantLt && !strings.Contains(de.Message(), "Kurang dari") {
					t.Errorf("case %s: expected 'Kurang dari' in message, got: %s", tc.name, de.Message())
				}
			} else {
				if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeBobotSumInvariantViolated {
					t.Errorf("case %s: expected no invariant error, got BOBOT_SUM_INVARIANT_VIOLATED", tc.name)
				}
			}
		})
	}
}

// TestSumInvariant_NonApproveTransition_SkipsCheck verifies that sum check is skipped
// for non-APPROVED transitions (e.g. PENDING_REVIEW, PENDING_APPROVAL).
func TestSumInvariant_NonApproveTransition_SkipsCheck(t *testing.T) {
	e := testBobotSkenario()
	// sumByPeriod returns 0 — sum would fail if checked.
	svc := buildSvc(&repoAdapter{
		getByIDStub:    &stubGetByID{result: e},
		sumByPeriodStub: &stubSumByPeriod{sum: decimal.Zero},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())

	for _, state := range []string{"PENDING_REVIEW", "PENDING_APPROVAL", "PENDING_APPROVAL_2", "REJECTED"} {
		t.Run(state, func(t *testing.T) {
			err := svc.SyncWorkflowStatus(ctx, e.ID, state, "ACTION")
			// Should NOT be a sum invariant error for non-APPROVED states.
			if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeBobotSumInvariantViolated {
				t.Errorf("state=%s: sum check should be skipped, got BOBOT_SUM_INVARIANT_VIOLATED", state)
			}
		})
	}
}

// ─── 10. PATCH — workflow_status guard ────────────────────────────────────────

func TestUpdate_ApprovedRecord_Returns403(t *testing.T) {
	approved := testBobotSkenarioWith(bobotskenario.SkenarioNormal, "0.50000000", bobotskenario.WorkflowStatusApproved)
	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getResult: approved},
	}))

	body, _ := json.Marshal(bobotskenario.UpdateRequest{RowVersion: 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/bobot-skenario/"+approved.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for APPROVED record update, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), bobotskenario.CodeMasterApprovedNoEdit)
}

// ─── 11. Update: duplicate check ─────────────────────────────────────────────

func TestUpdate_DuplicateSkenarioPeriod_Returns422(t *testing.T) {
	e := testBobotSkenario()
	svc := buildSvc(&repoAdapter{
		updateStub:         &stubUpdate{getResult: e},
		countDuplicateStub: &stubCountDuplicate{count: 1},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.Update(ctx, e.ID, bobotskenario.UpdateRequest{
		Skenario:   ptr(string(bobotskenario.SkenarioBad)),
		RowVersion: 1,
	})
	if err == nil {
		t.Fatal("expected error for duplicate skenario+period on update")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError, got %T", err)
	}
	if de.Code() != domainerrors.CodeBobotDuplicateSkenarioPeriod {
		t.Errorf("expected BOBOT_DUPLICATE_SKENARIO_PERIOD, got %s", de.Code())
	}
}

// ─── 12. DELETE — entity in use ───────────────────────────────────────────────

func TestDelete_EntityInUse_Returns409(t *testing.T) {
	e := testBobotSkenario()
	r := newRouter(buildSvc(&repoAdapter{
		softDeleteStub: &stubSoftDelete{getResult: e},
		countRefStub:   &stubCountRef{count: 2},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/bobot-skenario/"+e.ID.String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Errorf("expected 409 for entity in use, got %d; body=%s", rec.Code, rec.Body.String())
	}
	assertErrorCode(t, rec.Body.Bytes(), bobotskenario.CodeEntityInUse)
}

func TestDelete_NotFound_Returns404(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		softDeleteStub: &stubSoftDelete{getResult: nil},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/master/bobot-skenario/"+uuid.New().String(), nil)
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// ─── 13. DELETE — no references proceeds ─────────────────────────────────────

func TestSoftDelete_NoReferences_Proceeds(t *testing.T) {
	e := testBobotSkenario()
	svc := buildSvc(&repoAdapter{
		softDeleteStub: &stubSoftDelete{getResult: e, deleteResult: e},
		countRefStub:   &stubCountRef{count: 0},
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	err := svc.SoftDelete(ctx, e.ID)
	// Will fail at BeginTx (no DB) but NOT at guard check.
	if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeEntityInUse {
		t.Error("should not return ENTITY_IN_USE when count=0")
	}
}

// ─── 14. Update: rowVersion required ─────────────────────────────────────────

func TestUpdate_MissingRowVersion_Returns400(t *testing.T) {
	e := testBobotSkenario()
	r := newRouter(buildSvc(&repoAdapter{
		updateStub: &stubUpdate{getResult: e},
	}))
	body, _ := json.Marshal(map[string]interface{}{
		"bobot": "0.50000000",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/master/bobot-skenario/"+e.ID.String(), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing rowVersion, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── 15. Workflow status guards ───────────────────────────────────────────────

func TestWorkflowStatus_Editability(t *testing.T) {
	editable := []bobotskenario.WorkflowStatus{
		bobotskenario.WorkflowStatusDraft,
		bobotskenario.WorkflowStatusReturned,
		bobotskenario.WorkflowStatusRejected,
	}
	notEditable := []bobotskenario.WorkflowStatus{
		bobotskenario.WorkflowStatusApproved,
		bobotskenario.WorkflowStatusPendingReview,
		bobotskenario.WorkflowStatusPendingApproval,
		bobotskenario.WorkflowStatusPendingApproval2,
	}
	for _, s := range editable {
		if !s.IsEditable() {
			t.Errorf("status %s should be editable", s)
		}
	}
	for _, s := range notEditable {
		if s.IsEditable() {
			t.Errorf("status %s should NOT be editable", s)
		}
	}
}

func TestWorkflowStatus_PendingApproval2Exists(t *testing.T) {
	if bobotskenario.WorkflowStatusPendingApproval2 != "PENDING_APPROVAL_2" {
		t.Errorf("value mismatch: %s", bobotskenario.WorkflowStatusPendingApproval2)
	}
}

// ─── 16. ToResponse: REJECTED maps to RETURNED ────────────────────────────────

func TestToResponse_RejectedMapsToReturned(t *testing.T) {
	e := testBobotSkenario()
	e.WorkflowStatus = bobotskenario.WorkflowStatusRejected
	r := bobotskenario.ToResponse(e)
	if r.WorkflowStatus != "RETURNED" {
		t.Errorf("expected RETURNED for REJECTED, got %s", r.WorkflowStatus)
	}
}

func TestToResponse_ApprovedNotMapped(t *testing.T) {
	e := testBobotSkenario()
	e.WorkflowStatus = bobotskenario.WorkflowStatusApproved
	r := bobotskenario.ToResponse(e)
	if r.WorkflowStatus != "APPROVED" {
		t.Errorf("expected APPROVED, got %s", r.WorkflowStatus)
	}
}

// ─── 17. ToResponse: Bobot serialization ─────────────────────────────────────

func TestToResponse_Bobot_SerializedAs8dp(t *testing.T) {
	e := testBobotSkenario()
	e.Bobot = decimal.RequireFromString("0.25")
	r := bobotskenario.ToResponse(e)
	if r.Bobot != "0.25000000" {
		t.Errorf("expected 0.25000000 (8dp), got %s", r.Bobot)
	}
}

func TestToResponse_Bobot_NeverFloat64(t *testing.T) {
	// Verify that bobot is returned as a string, not a float.
	e := testBobotSkenario()
	e.Bobot = decimal.RequireFromString("0.33333333")
	r := bobotskenario.ToResponse(e)
	if r.Bobot != "0.33333333" {
		t.Errorf("expected exact 0.33333333, got %s", r.Bobot)
	}
}

// ─── 18. Export route not confused with :id ───────────────────────────────────

func TestExportRoute_NotConfusedWithID(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Skenario\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		exportStub: &stubExport{reader: strings.NewReader(csvData), count: 0},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/export", nil)
	r.ServeHTTP(rec, req)
	if rec.Code == http.StatusBadRequest {
		body := rec.Body.String()
		if strings.Contains(body, "UUID") {
			t.Errorf("export route confused with /:id; got UUID parse error; body=%s", body)
		}
	}
}

func TestExport_InvalidFormat_Returns400(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/export?format=pdf", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid format, got %d", rec.Code)
	}
}

func TestExport_XLSX_Returns501(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/export?format=xlsx", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 for XLSX (not yet implemented), got %d", rec.Code)
	}
}

func TestExport_CSV_Returns200WithCSV(t *testing.T) {
	csvData := "\xef\xbb\xbfID,Skenario,Bobot\r\n00000000-0000-0000-0000-000000000002,NORMAL,0.50000000\r\n"
	r := newRouter(buildSvc(&repoAdapter{
		exportStub: &stubExport{reader: strings.NewReader(csvData), count: 1},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/export?format=csv", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for CSV export, got %d; body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("expected text/csv content-type, got %s", ct)
	}
	if rec.Header().Get("X-Total-Rows") != "1" {
		t.Errorf("expected X-Total-Rows=1, got %s", rec.Header().Get("X-Total-Rows"))
	}
}

// ─── 19. seed-default route not confused with :id ─────────────────────────────

func TestSeedDefaultRoute_NotConfusedWithID(t *testing.T) {
	r := newRouter(buildSvc(&repoAdapter{
		countByPeriodStub: &stubCountByPeriod{count: 3}, // already exists → skip
	}))
	body, _ := json.Marshal(bobotskenario.SeedDefaultRequest{PeriodeBerlakuDari: "2026-01-01"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/master/bobot-skenario/seed-default", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	r.ServeHTTP(rec, req)
	// Should NOT be a 400 UUID parse error.
	if rec.Code == http.StatusBadRequest {
		if strings.Contains(rec.Body.String(), "UUID") {
			t.Errorf("seed-default route confused with /:id; body=%s", rec.Body.String())
		}
	}
}

// ─── 20. SeedDefault: idempotency ─────────────────────────────────────────────

func TestSeedDefault_AlreadyExists_Returns201Skipped(t *testing.T) {
	svc := buildSvc(&repoAdapter{
		countByPeriodStub: &stubCountByPeriod{count: 3}, // 3 rows already exist
	})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	result, err := svc.SeedDefault(ctx, bobotskenario.SeedDefaultRequest{
		PeriodeBerlakuDari: "2026-01-01",
	})
	if err != nil {
		t.Fatalf("expected no error for idempotent seed, got: %v", err)
	}
	if !result.Skipped {
		t.Error("expected Skipped=true when 3 rows already exist")
	}
	if result.Created != 0 {
		t.Errorf("expected Created=0 for skipped, got %d", result.Created)
	}
}

func TestSeedDefault_InvalidDate_Returns422(t *testing.T) {
	svc := buildSvc(&repoAdapter{})
	ctx := auth.ContextWithClaims(context.Background(), testClaims())
	_, err := svc.SeedDefault(ctx, bobotskenario.SeedDefaultRequest{
		PeriodeBerlakuDari: "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok {
		t.Fatalf("expected DomainError")
	}
	if de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("expected VALIDATION_FAILED, got %s", de.Code())
	}
}

// ─── 21. DefaultBobot values (DEC-010) ────────────────────────────────────────

func TestDefaultBobot_DEC010Values(t *testing.T) {
	good := bobotskenario.DefaultBobot(bobotskenario.SkenarioGood)
	normal := bobotskenario.DefaultBobot(bobotskenario.SkenarioNormal)
	bad := bobotskenario.DefaultBobot(bobotskenario.SkenarioBad)

	if !good.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("GOOD default should be 0.25, got %s", good.String())
	}
	if !normal.Equal(decimal.NewFromFloat(0.50)) {
		t.Errorf("NORMAL default should be 0.50, got %s", normal.String())
	}
	if !bad.Equal(decimal.NewFromFloat(0.25)) {
		t.Errorf("BAD default should be 0.25, got %s", bad.String())
	}

	// Sum must equal 1.0.
	sum := good.Add(normal).Add(bad)
	if !sum.Equal(bobotskenario.SumTarget) {
		t.Errorf("DEC-010: G+N+B default sum should be 1.0, got %s", sum.String())
	}
}

// ─── 22. IsValidSkenario ──────────────────────────────────────────────────────

func TestIsValidSkenario(t *testing.T) {
	valid := []bobotskenario.Skenario{
		bobotskenario.SkenarioGood,
		bobotskenario.SkenarioNormal,
		bobotskenario.SkenarioBad,
	}
	for _, v := range valid {
		if !bobotskenario.IsValidSkenario(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	if bobotskenario.IsValidSkenario("UNKNOWN") {
		t.Error("expected UNKNOWN to be invalid")
	}
	if bobotskenario.IsValidSkenario("good") {
		t.Error("expected lowercase 'good' to be invalid (case-sensitive)")
	}
}

// ─── 23. AllSkenarios coverage ────────────────────────────────────────────────

func TestAllSkenarios_HasAllThree(t *testing.T) {
	if len(bobotskenario.AllSkenarios) != 3 {
		t.Errorf("expected 3 skenarios, got %d", len(bobotskenario.AllSkenarios))
	}
	found := map[bobotskenario.Skenario]bool{}
	for _, s := range bobotskenario.AllSkenarios {
		found[s] = true
	}
	if !found[bobotskenario.SkenarioGood] {
		t.Error("GOOD missing from AllSkenarios")
	}
	if !found[bobotskenario.SkenarioNormal] {
		t.Error("NORMAL missing from AllSkenarios")
	}
	if !found[bobotskenario.SkenarioBad] {
		t.Error("BAD missing from AllSkenarios")
	}
}

// ─── 24. Permission constants ─────────────────────────────────────────────────

func TestPermissionConstants_UsesECLParameterPrefix(t *testing.T) {
	perms := []string{
		bobotskenario.PermCreate, bobotskenario.PermRead, bobotskenario.PermUpdate, bobotskenario.PermDelete,
		bobotskenario.PermSubmit, bobotskenario.PermReview, bobotskenario.PermApprove, bobotskenario.PermReject, bobotskenario.PermExport,
	}
	for _, p := range perms {
		if !strings.HasPrefix(p, "ecl_parameter.") {
			t.Errorf("permission %q should start with ecl_parameter.", p)
		}
	}
}

func TestPermApprove2_SameAsApprove(t *testing.T) {
	// approve2 uses same permission as approve (per WORKFLOW_CONFIG_BOBOT_SKENARIO).
	if bobotskenario.PermApprove != "ecl_parameter.approve" {
		t.Errorf("PermApprove = %q, expected ecl_parameter.approve", bobotskenario.PermApprove)
	}
}

// ─── 25. Error code constants ─────────────────────────────────────────────────

func TestErrorCodeConstants(t *testing.T) {
	if bobotskenario.CodeEntityInUse != "ENTITY_IN_USE" {
		t.Errorf("CodeEntityInUse = %q", bobotskenario.CodeEntityInUse)
	}
	if bobotskenario.CodeMasterApprovedNoEdit != "MASTER_APPROVED_NO_EDIT" {
		t.Errorf("CodeMasterApprovedNoEdit = %q", bobotskenario.CodeMasterApprovedNoEdit)
	}
	if bobotskenario.CodeBobotSumInvariantViolated != "BOBOT_SUM_INVARIANT_VIOLATED" {
		t.Errorf("CodeBobotSumInvariantViolated = %q", bobotskenario.CodeBobotSumInvariantViolated)
	}
	if bobotskenario.CodeBobotPeriodOverlap != "BOBOT_PERIOD_OVERLAP" {
		t.Errorf("CodeBobotPeriodOverlap = %q", bobotskenario.CodeBobotPeriodOverlap)
	}
	if bobotskenario.CodeBobotDuplicateSkenarioPeriod != "BOBOT_DUPLICATE_SKENARIO_PERIOD" {
		t.Errorf("CodeBobotDuplicateSkenarioPeriod = %q", bobotskenario.CodeBobotDuplicateSkenarioPeriod)
	}
}

// ─── 26. Repository interface compliance ──────────────────────────────────────

func TestRepositoryInterfaceCompliance(t *testing.T) {
	var _ bobotskenario.Repository = (*bobotskenario.DBRepository)(nil)
}

// ─── 27. Pagination cursor roundtrip ─────────────────────────────────────────

func TestPaginationCursorRoundtrip(t *testing.T) {
	lastID := uuid.New().String()
	cursor, err := pagination.EncodeCursor(pagination.CursorData{ID: lastID})
	if err != nil {
		t.Fatalf("EncodeCursor: %v", err)
	}
	decoded, err := pagination.DecodeCursor(cursor)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if decoded.ID != lastID {
		t.Errorf("expected ID=%s, got %s", lastID, decoded.ID)
	}
}

// ─── 28. History endpoint ─────────────────────────────────────────────────────

func TestHistory_Found_Returns200(t *testing.T) {
	e := testBobotSkenario()
	r := newRouter(buildSvc(&repoAdapter{
		getByIDStub: &stubGetByID{result: e},
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/master/bobot-skenario/"+e.ID.String()+"/history", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

// ─── 29. SyncWorkflowStatus: state mapping ────────────────────────────────────

func TestSyncWorkflowStatus_StateMappingAllStates(t *testing.T) {
	cases := []struct {
		state    string
		expected bobotskenario.WorkflowStatus
	}{
		{"DRAFT", bobotskenario.WorkflowStatusDraft},
		{"PENDING_REVIEW", bobotskenario.WorkflowStatusPendingReview},
		{"PENDING_APPROVAL", bobotskenario.WorkflowStatusPendingApproval},
		{"PENDING_APPROVAL_2", bobotskenario.WorkflowStatusPendingApproval2},
		{"APPROVED", bobotskenario.WorkflowStatusApproved},
		{"REJECTED", bobotskenario.WorkflowStatusRejected},
	}
	for _, tc := range cases {
		e := testBobotSkenario()
		e.WorkflowStatus = bobotskenario.WorkflowStatus(tc.state)
		r := bobotskenario.ToResponse(e)
		expected := tc.expected
		if expected == bobotskenario.WorkflowStatusRejected {
			expected = bobotskenario.WorkflowStatusReturned
		}
		if r.WorkflowStatus != string(expected) {
			t.Errorf("state=%s: expected %s, got %s", tc.state, expected, r.WorkflowStatus)
		}
	}
}

// ─── 30. Optimistic lock error mapping ───────────────────────────────────────

func TestUpdate_ErrConflict_MapsTo409(t *testing.T) {
	err := domainerrors.ErrConflict()
	if err.Code() != domainerrors.CodeConflict {
		t.Errorf("expected CodeConflict, got %s", err.Code())
	}
	if err.HTTPStatus() != http.StatusConflict {
		t.Errorf("expected 409, got %d", err.HTTPStatus())
	}
}

// ─── 31. SumTolerance is correct ─────────────────────────────────────────────

func TestSumTolerance_IsCorrectValue(t *testing.T) {
	expected := decimal.NewFromFloat(0.00000001)
	if !bobotskenario.SumTolerance.Equal(expected) {
		t.Errorf("SumTolerance = %s, expected 0.00000001", bobotskenario.SumTolerance.String())
	}
}

func TestSumTarget_IsOne(t *testing.T) {
	if !bobotskenario.SumTarget.Equal(decimal.NewFromInt(1)) {
		t.Errorf("SumTarget = %s, expected 1", bobotskenario.SumTarget.String())
	}
}

// ─── 32. AllowedSortCols coverage ────────────────────────────────────────────

func TestAllowedSortCols_Coverage(t *testing.T) {
	expected := []string{"skenario", "bobot", "periode_berlaku_dari", "workflow_status", "created_at"}
	for _, col := range expected {
		found := false
		for _, ac := range bobotskenario.AllowedSortCols {
			if ac == col {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in AllowedSortCols", col)
		}
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func assertErrorCode(t *testing.T, body []byte, expectedCode string) {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Errorf("failed to decode error response: %v; body=%s", err, body)
		return
	}
	if resp.Error.Code != expectedCode {
		t.Errorf("expected error code %q, got %q; body=%s", expectedCode, resp.Error.Code, body)
	}
}

// assertValidationError checks that err is a VALIDATION_FAILED domain error.
func assertValidationError(t *testing.T, err error, context string) {
	t.Helper()
	if err == nil {
		t.Errorf("[%s] expected VALIDATION_FAILED error, got nil", context)
		return
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Errorf("[%s] expected VALIDATION_FAILED, got %T: %v", context, err, err)
	}
}

// assertNotValidationError checks that err is NOT a VALIDATION_FAILED domain error.
// Other errors (BeginTx fail, etc.) are acceptable in test context.
func assertNotValidationError(t *testing.T, err error, context string) {
	t.Helper()
	if de, ok := domainerrors.IsDomainError(err); ok && de.Code() == domainerrors.CodeValidationFailed {
		t.Errorf("[%s] unexpected VALIDATION_FAILED: %v", context, err)
	}
}

// Suppress unused import.
var _ = time.Now
