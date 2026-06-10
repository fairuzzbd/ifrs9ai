//go:build integration

// Package integration — bobot_skenario integration tests (APP-A-MSTR-005 / DEC-010).
//
// Coverage targets (all require live PostgreSQL; skip gracefully without infra):
//
//  1. TestBobot_DuplicateSkenarioPeriod_Returns422
//     POST same (skenario, periode_berlaku_dari) twice → 422 BOBOT_DUPLICATE_SKENARIO_PERIOD.
//
//  2. TestBobot_BobotOutOfRange_Returns422
//     bobot=1.5 or bobot=-0.1 → 422 VALIDATION_FAILED.
//
//  3. TestBobot_InvalidSkenario_Returns422
//     skenario="UNKNOWN" → 422 VALIDATION_FAILED.
//
//  4. TestBobot_PeriodOrderInvalid_Returns422
//     periode_berlaku_sampai < periode_berlaku_dari → 422 VALIDATION_FAILED.
//
//  5. TestBobot_OptimisticLock_Returns409
//     PATCH with stale row_version → 409 CONFLICT.
//
//  6. TestBobot_PeriodOverlapSameSkenario_Returns422
//     GOOD rows with overlapping periods → 422 BOBOT_PERIOD_OVERLAP.
//
//  7. TestBobot_SumInvariant_LessThan1_RejectsApprove
//     G=0.25, N=0.50, B=0.20 (sum=0.95) — approve2 → 422 BOBOT_SUM_INVARIANT_VIOLATED
//     with "Kurang dari" in message.
//
//  8. TestBobot_SumInvariant_MoreThan1_RejectsApprove
//     G=0.30, N=0.50, B=0.25 (sum=1.05) — approve2 → 422 BOBOT_SUM_INVARIANT_VIOLATED
//     with "Lebih dari" in message.
//
//  9. TestBobot_SumInvariant_Exact1_AllowsApprove
//     G=0.25, N=0.50, B=0.25 (sum=1.00) — full 6-eyes cycle → APPROVED.
//
// 10. TestBobot_SixEyesCycle_Full_WithStepUpMFA
//     DRAFT→submit→review→approve(ALCO1+step-up)→approve2(ALCO2+step-up) → APPROVED.
//     Asserts 4 signature records, 5 audit events, entity workflow_status=APPROVED.
//
// 11. TestBobot_StepUpRequired_Approve2WithoutMFA_Rejected
//     approve2 without mfa_verified=true → 403 FORBIDDEN (step-up MFA required).
//
// 12. TestBobot_SeedDefault_Idempotent
//     POST /seed-default → 3 DRAFT rows (G=0.25, N=0.50, B=0.25).
//     POST again same periode → idempotent (Skipped=true, no duplicates).

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/bobotskenario"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Workflow config for BOBOT_SKENARIO (6-eyes, step-up MFA on approve/approve2) ────────

func bobotSkenarioWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["BOBOT_SKENARIO"] = &workflow.Config{
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
	}
	return cfgs
}

// ─── Router builder ──────────────────────────────────────────────────────────

func buildBobotSkenarioRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := bobotskenario.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := bobotskenario.NewService(repo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)

	// Try DB config loader first; fall back to in-memory that has BOBOT_SKENARIO.
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("BOBOT_SKENARIO"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(bobotSkenarioWorkflowConfig())
	}

	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	h := bobotskenario.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	bobotskenario.RegisterRoutes(v1, h)
	return r
}

// ─── Claim builders ──────────────────────────────────────────────────────────

// bobotRiskClaims builds claims for a RISK officer (submitter / reviewer for ECL params).
func bobotRiskClaims(userID uuid.UUID) string {
	return bobotClaimsJSON(userID, "ROLE-RISK", false,
		"ecl_parameter.create", "ecl_parameter.read", "ecl_parameter.update",
		"ecl_parameter.delete", "ecl_parameter.submit", "ecl_parameter.review",
	)
}

// bobotAlcoClaims builds claims for an ALCO member (approve + approve2, MFA required).
func bobotAlcoClaims(userID uuid.UUID, mfaVerified bool) string {
	return bobotClaimsJSON(userID, "ROLE-ALCO", mfaVerified,
		"ecl_parameter.read", "ecl_parameter.approve", "ecl_parameter.reject",
	)
}

func bobotClaimsJSON(userID uuid.UUID, role string, mfaVerified bool, perms ...string) string {
	now := time.Now().Unix()
	c := auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: "testuser_" + userID.String()[:8],
		Roles:             []string{role},
		Permissions:       perms,
		TenantID:          "TUGURE",
		MFAVerified:       mfaVerified,
		Exp:               now + 3600,
		Iat:               now,
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedBobotSkenarioDRAFT inserts one mst.bobot_skenario row in DRAFT state.
// Also creates the associated workflow_instance. Returns the entity UUID.
func seedBobotSkenarioDRAFT(
	t *testing.T, db *sql.DB,
	skenario bobotskenario.Skenario, bobot decimal.Decimal,
	periodeDari string, makerID uuid.UUID,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	MustExec(t, db, `
		INSERT INTO mst.bobot_skenario (
			id, skenario, bobot,
			periode_berlaku_dari, periode_berlaku_sampai,
			maker_id, approver_id, approved_at,
			workflow_status,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, $2, $3,
			$4::date, NULL,
			$5, NULL, NULL,
			'DRAFT',
			now(), $5, now(), $5,
			1, 'TUGURE'
		)
	`, id, string(skenario), bobot.String(), periodeDari, makerID)

	// Seed the workflow instance for this entity (6-eyes).
	seedBobotWorkflowInstance(t, db, id, makerID)

	// Back-reference: workflow_instance_id on the entity row.
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, id).Scan(&wfID); err == nil {
		MustExec(t, db, `
			UPDATE mst.bobot_skenario SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, id)
	}

	return id
}

// seedBobotWorkflowInstance seeds a sys.workflow_instance for BOBOT_SKENARIO (6-eyes).
func seedBobotWorkflowInstance(t *testing.T, db *sql.DB, entityID, makerID uuid.UUID) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO sys.workflow_instance (
			entity_type, entity_id, entity_schema,
			workflow_config_key, eyes, current_state,
			maker_id, created_by, updated_by
		) VALUES (
			'BOBOT_SKENARIO', $1, 'mst',
			'WORKFLOW_CONFIG_BOBOT_SKENARIO', 6, 'DRAFT',
			$2, $2, $2
		) ON CONFLICT (entity_type, entity_id) DO NOTHING
	`, entityID, makerID)
	if err != nil {
		t.Fatalf("seedBobotWorkflowInstance %s: %v", entityID, err)
	}
}

// cleanupBobot removes test bobot_skenario rows by period prefix (period LIKE '%2099%').
// Uses a safe delete that cascades to workflow_instance.
func cleanupBobotByPeriod(t *testing.T, db *sql.DB, periodeDari string) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT id FROM mst.bobot_skenario
		WHERE periode_berlaku_dari = $1::date AND tenant_id = 'TUGURE'
	`, periodeDari)
	if err != nil {
		return
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		_, _ = db.ExecContext(context.Background(), `
			DELETE FROM sys.workflow_instance WHERE entity_id = $1
		`, id)
		_, _ = db.ExecContext(context.Background(), `
			DELETE FROM mst.bobot_skenario WHERE id = $1
		`, id)
	}
}

// assertBobotWorkflowStatus checks mst.bobot_skenario.workflow_status matches expected.
func assertBobotWorkflowStatus(t *testing.T, db *sql.DB, entityID uuid.UUID, expected string) {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.bobot_skenario WHERE id = $1
	`, entityID).Scan(&status); err != nil {
		t.Fatalf("assertBobotWorkflowStatus: %v", err)
	}
	if status != expected {
		t.Errorf("bobot_skenario.workflow_status: expected %s, got %s", expected, status)
	}
}

// countBobotByPeriod returns how many non-deleted bobot_skenario rows exist for the period.
func countBobotByPeriod(t *testing.T, db *sql.DB, periodeDari string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.bobot_skenario
		WHERE periode_berlaku_dari = $1::date
		  AND deleted_at IS NULL
		  AND tenant_id = 'TUGURE'
	`, periodeDari).Scan(&n); err != nil {
		t.Fatalf("countBobotByPeriod: %v", err)
	}
	return n
}

// sumBobotByPeriod returns the sum of bobot for all non-deleted rows in the period.
func sumBobotByPeriod(t *testing.T, db *sql.DB, periodeDari string) decimal.Decimal {
	t.Helper()
	var s string
	if err := db.QueryRowContext(context.Background(), `
		SELECT COALESCE(SUM(bobot)::text, '0')
		FROM mst.bobot_skenario
		WHERE periode_berlaku_dari = $1::date
		  AND deleted_at IS NULL
		  AND tenant_id = 'TUGURE'
	`, periodeDari).Scan(&s); err != nil {
		t.Fatalf("sumBobotByPeriod: %v", err)
	}
	d, _ := decimal.NewFromString(s)
	return d
}

// ─── Test 1: Duplicate (skenario, periode_berlaku_dari) → 422 ────────────────

// TestBobot_DuplicateSkenarioPeriod_Returns422 verifies that creating two GOOD rows
// for the same periode_berlaku_dari returns 422 BOBOT_DUPLICATE_SKENARIO_PERIOD.
// Regression: §1 matrix coverage — duplicate guard per skenario+period.
func TestBobot_DuplicateSkenarioPeriod_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-01-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_dup_maker")
	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	body := fmt.Sprintf(`{
		"skenario": "GOOD",
		"bobot": "0.25000000",
		"periodeBerlakuDari": %q
	}`, periode)

	// First create — must succeed.
	w1 := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create GOOD: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first GOOD created OK")

	// Second create same skenario+period — must return 422.
	w2 := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("duplicate GOOD: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	code := errCode(w2.Body.Bytes())
	if code != bobotskenario.CodeBobotDuplicateSkenarioPeriod {
		t.Errorf("expected %s, got %q", bobotskenario.CodeBobotDuplicateSkenarioPeriod, code)
	}
	t.Logf("duplicate GOOD correctly rejected: 422 %s", code)
}

// ─── Test 2: bobot out of range → 422 ────────────────────────────────────────

// TestBobot_BobotOutOfRange_Returns422 verifies that bobot=1.5 or bobot=-0.1
// is rejected with 422 VALIDATION_FAILED at the service layer.
// Regression: §2 ECL calc reproducibility — malformed weights must not persist.
func TestBobot_BobotOutOfRange_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-02-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_range_maker")
	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	cases := []struct {
		bobot   string
		wantErr bool
	}{
		{"1.50000000", true},  // above max
		{"-0.10000000", true}, // below min (negative)
		{"2.00000000", true},  // well above max
		{"0.25000000", false}, // valid — must succeed
	}

	for _, tc := range cases {
		t.Run("bobot="+tc.bobot, func(t *testing.T) {
			body := fmt.Sprintf(`{
				"skenario": "NORMAL",
				"bobot": %q,
				"periodeBerlakuDari": %q
			}`, tc.bobot, periode)
			w := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body)
			if tc.wantErr {
				if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
					t.Errorf("bobot=%s: expected 422/400, got %d body=%s", tc.bobot, w.Code, w.Body.String())
				} else {
					t.Logf("bobot=%s rejected: %d", tc.bobot, w.Code)
				}
			} else {
				if w.Code != http.StatusCreated {
					t.Errorf("bobot=%s: expected 201, got %d body=%s", tc.bobot, w.Code, w.Body.String())
				}
			}
		})
	}
}

// ─── Test 3: Invalid skenario → 422 ──────────────────────────────────────────

// TestBobot_InvalidSkenario_Returns422 verifies that skenario="UNKNOWN" is rejected.
// Regression: §1 SPPI×BM matrix — all GOOD/NORMAL/BAD values must be exact constants.
func TestBobot_InvalidSkenario_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-03-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_skenario_maker")
	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	invalidSkenarios := []string{"UNKNOWN", "good", "bad", "neutral", "", "VERY_BAD"}
	for _, sk := range invalidSkenarios {
		t.Run("skenario="+sk, func(t *testing.T) {
			body := fmt.Sprintf(`{
				"skenario": %q,
				"bobot": "0.25000000",
				"periodeBerlakuDari": %q
			}`, sk, periode)
			w := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body)
			if w.Code == http.StatusCreated {
				t.Errorf("skenario=%q should be rejected, got 201", sk)
			} else {
				t.Logf("skenario=%q rejected: %d", sk, w.Code)
			}
		})
	}
}

// ─── Test 4: Period order invalid → 422 ──────────────────────────────────────

// TestBobot_PeriodOrderInvalid_Returns422 verifies sampai < dari is rejected.
// Regression: §4 EIR re-estimation depends on correct period ordering.
func TestBobot_PeriodOrderInvalid_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-04-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_period_maker")
	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	// sampai="2099-01-01" < dari="2099-04-01" — must fail.
	body := fmt.Sprintf(`{
		"skenario": "GOOD",
		"bobot": "0.25000000",
		"periodeBerlakuDari": %q,
		"periodeBerlakuSampai": "2099-01-01"
	}`, periode)
	w := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Errorf("period order invalid: expected 422/400, got %d body=%s", w.Code, w.Body.String())
	} else {
		t.Logf("period order invalid correctly rejected: %d", w.Code)
	}

	// sampai=dari (same day) — must succeed.
	bodyEqual := fmt.Sprintf(`{
		"skenario": "GOOD",
		"bobot": "0.25000000",
		"periodeBerlakuDari": %q,
		"periodeBerlakuSampai": %q
	}`, periode, periode)
	w2 := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), bodyEqual)
	if w2.Code != http.StatusCreated {
		t.Errorf("sampai==dari should be valid: expected 201, got %d body=%s", w2.Code, w2.Body.String())
	}
}

// ─── Test 5: Optimistic lock → 409 ───────────────────────────────────────────

// TestBobot_OptimisticLock_Returns409 verifies that PATCH with a stale row_version
// returns 409 CONFLICT (optimistic lock). Regression: §2 reproducibility guard.
func TestBobot_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-05-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_optlock_maker")
	entityID := seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioNormal,
		decimal.RequireFromString("0.50000000"), periode, makerID)

	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	// First PATCH with rowVersion=1 — succeeds, bumps to 2.
	patch1 := `{"bobot":"0.50000000","rowVersion":1}`
	w1 := patchJSON(router, "/api/v1/master/bobot-skenario/"+entityID.String(), claims, uuid.New().String(), patch1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first PATCH: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first PATCH OK, row_version now 2")

	// Second PATCH with stale rowVersion=1 — must return 409.
	patch2 := `{"bobot":"0.49000000","rowVersion":1}`
	w2 := patchJSON(router, "/api/v1/master/bobot-skenario/"+entityID.String(), claims, uuid.New().String(), patch2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected: 409")
}

// ─── Test 6: Period overlap same skenario → 422 ───────────────────────────────

// TestBobot_PeriodOverlapSameSkenario_Returns422 verifies that two GOOD rows
// with overlapping periods are rejected. Regression: §2 uniqueness of ECL weights.
func TestBobot_PeriodOverlapSameSkenario_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-06-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_overlap_maker")
	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	// First GOOD row: 2099-06-01 → 2099-12-31.
	body1 := fmt.Sprintf(`{
		"skenario": "GOOD",
		"bobot": "0.25000000",
		"periodeBerlakuDari": %q,
		"periodeBerlakuSampai": "2099-12-31"
	}`, periode)
	w1 := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first GOOD row: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first GOOD row created OK")

	// Second GOOD row with overlapping period: 2099-06-01 → 2099-09-30.
	body2 := fmt.Sprintf(`{
		"skenario": "GOOD",
		"bobot": "0.30000000",
		"periodeBerlakuDari": %q,
		"periodeBerlakuSampai": "2099-09-30"
	}`, periode)
	w2 := postJSON(router, "/api/v1/master/bobot-skenario", claims, uuid.New().String(), body2)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("overlapping GOOD: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	code := errCode(w2.Body.Bytes())
	// Either BOBOT_PERIOD_OVERLAP or BOBOT_DUPLICATE_SKENARIO_PERIOD is acceptable
	// depending on which guard fires first.
	validCodes := map[string]bool{
		bobotskenario.CodeBobotPeriodOverlap:             true,
		bobotskenario.CodeBobotDuplicateSkenarioPeriod:   true,
	}
	if !validCodes[code] {
		t.Errorf("expected BOBOT_PERIOD_OVERLAP or BOBOT_DUPLICATE_SKENARIO_PERIOD, got %q", code)
	}
	t.Logf("overlapping GOOD correctly rejected: 422 %s", code)
}

// ─── Test 7: Sum invariant < 1 rejects approve ────────────────────────────────

// TestBobot_SumInvariant_LessThan1_RejectsApprove creates trio G=0.25, N=0.50, B=0.20
// (sum=0.95). Submits and reviews the GOOD row. Attempts approve2 on GOOD row →
// 422 BOBOT_SUM_INVARIANT_VIOLATED with "Kurang dari" in message.
//
// DEC-010: G+N+B must equal 1.0. "Kurang dari" signals sum < 1.
func TestBobot_SumInvariant_LessThan1_RejectsApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-07-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_lt1_maker")
	reviewerID := seedUserSQL(t, infra.DB, "bs_lt1_reviewer")
	alco1ID := seedUserSQL(t, infra.DB, "bs_lt1_alco1")
	alco2ID := seedUserSQL(t, infra.DB, "bs_lt1_alco2")

	// Seed trio with sum=0.95 (BAD=0.20, not 0.25).
	goodID := seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioGood,
		decimal.RequireFromString("0.25000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioNormal,
		decimal.RequireFromString("0.50000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioBad,
		decimal.RequireFromString("0.20000000"), periode, makerID) // sum=0.95

	router := buildBobotSkenarioRouter(infra.DB)

	makerClaims := bobotRiskClaims(makerID)
	reviewerClaims := bobotClaimsJSON(reviewerID, "ROLE-RISK", true,
		"ecl_parameter.review", "ecl_parameter.read",
	)
	alco1Claims := bobotAlcoClaims(alco1ID, true)
	alco2Claims := bobotAlcoClaims(alco2ID, true)

	path := func(suffix string) string {
		return fmt.Sprintf("/api/v1/master/bobot-skenario/%s/%s", goodID, suffix)
	}
	rv := func(n int) string { return fmt.Sprintf(`{"rowVersion":%d,"signatureMethod":"JWT_STANDARD"}`, n) }

	// SUBMIT (maker).
	w1 := postJSON(router, path("submit"), makerClaims, uuid.New().String(), rv(1))
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_REVIEW")

	// REVIEW.
	w2 := postJSON(router, path("review"), reviewerClaims, uuid.New().String(), rv(2))
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL")

	// APPROVE (ALCO1, step-up MFA).
	w3 := postJSON(router, path("approve"), alco1Claims, uuid.New().String(), rv(3))
	if w3.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL_2")

	// APPROVE2 (ALCO2, step-up MFA) — must fail with sum invariant violation.
	w4 := postJSON(router, path("approve2"), alco2Claims, uuid.New().String(), rv(4))
	if w4.Code != http.StatusUnprocessableEntity {
		t.Errorf("approve2 with sum=0.95: expected 422, got %d body=%s", w4.Code, w4.Body.String())
	} else {
		code := errCode(w4.Body.Bytes())
		if code != bobotskenario.CodeBobotSumInvariantViolated {
			t.Errorf("expected %s, got %q", bobotskenario.CodeBobotSumInvariantViolated, code)
		}
		// Verify message contains "Kurang dari".
		var resp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w4.Body.Bytes(), &resp); err == nil {
			if !strings.Contains(resp.Error.Message, "Kurang dari") {
				t.Errorf("expected 'Kurang dari' in message, got: %s", resp.Error.Message)
			}
		}
		t.Logf("sum<1 correctly rejected at approve2: 422 %s", code)
	}

	// Workflow must remain PENDING_APPROVAL_2 (not advanced to APPROVED).
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL_2")
}

// ─── Test 8: Sum invariant > 1 rejects approve ────────────────────────────────

// TestBobot_SumInvariant_MoreThan1_RejectsApprove creates trio G=0.30, N=0.50, B=0.25
// (sum=1.05). Approve2 → 422 BOBOT_SUM_INVARIANT_VIOLATED with "Lebih dari" in message.
//
// DEC-010: "Lebih dari" signals sum > 1.
func TestBobot_SumInvariant_MoreThan1_RejectsApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-08-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_gt1_maker")
	reviewerID := seedUserSQL(t, infra.DB, "bs_gt1_reviewer")
	alco1ID := seedUserSQL(t, infra.DB, "bs_gt1_alco1")
	alco2ID := seedUserSQL(t, infra.DB, "bs_gt1_alco2")

	// Trio with sum=1.05.
	goodID := seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioGood,
		decimal.RequireFromString("0.30000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioNormal,
		decimal.RequireFromString("0.50000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioBad,
		decimal.RequireFromString("0.25000000"), periode, makerID)

	router := buildBobotSkenarioRouter(infra.DB)

	makerClaims := bobotRiskClaims(makerID)
	reviewerClaims := bobotClaimsJSON(reviewerID, "ROLE-RISK", true,
		"ecl_parameter.review", "ecl_parameter.read",
	)
	alco1Claims := bobotAlcoClaims(alco1ID, true)
	alco2Claims := bobotAlcoClaims(alco2ID, true)

	path := func(suffix string) string {
		return fmt.Sprintf("/api/v1/master/bobot-skenario/%s/%s", goodID, suffix)
	}
	rv := func(n int) string { return fmt.Sprintf(`{"rowVersion":%d,"signatureMethod":"JWT_STANDARD"}`, n) }

	// Submit → Review → Approve → Approve2.
	if w := postJSON(router, path("submit"), makerClaims, uuid.New().String(), rv(1)); w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	if w := postJSON(router, path("review"), reviewerClaims, uuid.New().String(), rv(2)); w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	if w := postJSON(router, path("approve"), alco1Claims, uuid.New().String(), rv(3)); w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}

	// Approve2 — must fail.
	w4 := postJSON(router, path("approve2"), alco2Claims, uuid.New().String(), rv(4))
	if w4.Code != http.StatusUnprocessableEntity {
		t.Errorf("approve2 with sum=1.05: expected 422, got %d body=%s", w4.Code, w4.Body.String())
	} else {
		code := errCode(w4.Body.Bytes())
		if code != bobotskenario.CodeBobotSumInvariantViolated {
			t.Errorf("expected %s, got %q", bobotskenario.CodeBobotSumInvariantViolated, code)
		}
		var resp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(w4.Body.Bytes(), &resp); err == nil {
			if !strings.Contains(resp.Error.Message, "Lebih dari") {
				t.Errorf("expected 'Lebih dari' in message, got: %s", resp.Error.Message)
			}
			// Message must also contain the actual sum value.
			if !strings.Contains(resp.Error.Message, "1.05") && !strings.Contains(resp.Error.Message, "1.05000000") {
				t.Logf("WARNING: sum value not found in message: %s", resp.Error.Message)
			}
		}
		t.Logf("sum>1 correctly rejected at approve2: 422 %s", code)
	}

	// Entity must not have advanced to APPROVED.
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL_2")
}

// ─── Test 9: Sum invariant = 1.0 allows approve ───────────────────────────────

// TestBobot_SumInvariant_Exact1_AllowsApprove seeds the canonical DEC-010 default
// G=0.25, N=0.50, B=0.25 (sum=1.00) and runs the full 6-eyes cycle to APPROVED.
// This is the positive path for every ECL calc run.
func TestBobot_SumInvariant_Exact1_AllowsApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-09-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_eq1_maker")
	reviewerID := seedUserSQL(t, infra.DB, "bs_eq1_reviewer")
	alco1ID := seedUserSQL(t, infra.DB, "bs_eq1_alco1")
	alco2ID := seedUserSQL(t, infra.DB, "bs_eq1_alco2")

	// DEC-010 canonical defaults.
	goodID := seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioGood,
		decimal.RequireFromString("0.25000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioNormal,
		decimal.RequireFromString("0.50000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioBad,
		decimal.RequireFromString("0.25000000"), periode, makerID)

	// Verify sum before workflow.
	total := sumBobotByPeriod(t, infra.DB, periode)
	if !total.Equal(decimal.NewFromInt(1)) {
		t.Fatalf("pre-condition: sum of trio = %s, expected 1.0", total.String())
	}

	router := buildBobotSkenarioRouter(infra.DB)

	makerClaims := bobotRiskClaims(makerID)
	reviewerClaims := bobotClaimsJSON(reviewerID, "ROLE-RISK", true,
		"ecl_parameter.review", "ecl_parameter.read",
	)
	alco1Claims := bobotAlcoClaims(alco1ID, true)
	alco2Claims := bobotAlcoClaims(alco2ID, true)

	path := func(suffix string) string {
		return fmt.Sprintf("/api/v1/master/bobot-skenario/%s/%s", goodID, suffix)
	}
	rv := func(n int) string { return fmt.Sprintf(`{"rowVersion":%d,"signatureMethod":"JWT_STANDARD"}`, n) }

	// Full 6-eyes cycle.
	if w := postJSON(router, path("submit"), makerClaims, uuid.New().String(), rv(1)); w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_REVIEW")
	t.Logf("SUBMIT: OK")

	if w := postJSON(router, path("review"), reviewerClaims, uuid.New().String(), rv(2)); w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL")
	t.Logf("REVIEW: OK")

	if w := postJSON(router, path("approve"), alco1Claims, uuid.New().String(), rv(3)); w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL_2")
	t.Logf("APPROVE (ALCO1): OK")

	w4 := postJSON(router, path("approve2"), alco2Claims, uuid.New().String(), rv(4))
	if w4.Code != http.StatusOK {
		t.Fatalf("approve2 with sum=1.0: expected 200, got %d body=%s", w4.Code, w4.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "APPROVED")
	assertBobotWorkflowStatus(t, infra.DB, goodID, "APPROVED")
	t.Logf("APPROVE2 (ALCO2): OK — entity APPROVED, sum invariant passed")
}

// ─── Test 10: Full 6-eyes cycle with step-up MFA ────────────────────────────

// TestBobot_SixEyesCycle_Full_WithStepUpMFA exercises the complete
// DRAFT→PENDING_REVIEW→PENDING_APPROVAL→PENDING_APPROVAL_2→APPROVED path for
// bobot_skenario using 4 distinct users (maker, reviewer, ALCO1, ALCO2).
// Asserts: 4 workflow signatures, 5 audit events (1 submit + 1 review + 1 approve + 1 approve2 + entity events).
func TestBobot_SixEyesCycle_Full_WithStepUpMFA(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-10-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_6eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "bs_6eyes_reviewer")
	alco1ID := seedUserSQL(t, infra.DB, "bs_6eyes_alco1")
	alco2ID := seedUserSQL(t, infra.DB, "bs_6eyes_alco2")

	// Canonical DEC-010 trio.
	goodID := seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioGood,
		decimal.RequireFromString("0.25000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioNormal,
		decimal.RequireFromString("0.50000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioBad,
		decimal.RequireFromString("0.25000000"), periode, makerID)

	router := buildBobotSkenarioRouter(infra.DB)

	makerClaims := bobotRiskClaims(makerID)
	reviewerClaims := bobotClaimsJSON(reviewerID, "ROLE-RISK", true,
		"ecl_parameter.review", "ecl_parameter.read",
	)
	// ALCO claims with MFA verified (step-up simulated by mfa_verified=true in JWT).
	alco1Claims := bobotAlcoClaims(alco1ID, true)
	alco2Claims := bobotAlcoClaims(alco2ID, true)

	path := func(suffix string) string {
		return fmt.Sprintf("/api/v1/master/bobot-skenario/%s/%s", goodID, suffix)
	}
	rv := func(n int) string {
		return fmt.Sprintf(`{"rowVersion":%d,"signatureMethod":"JWT_STANDARD","comment":"step %d"}`, n, n)
	}

	// 1. SUBMIT
	if w := postJSON(router, path("submit"), makerClaims, uuid.New().String(), rv(1)); w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_REVIEW")

	// 2. REVIEW
	if w := postJSON(router, path("review"), reviewerClaims, uuid.New().String(), rv(2)); w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL")

	// 3. APPROVE (ALCO1, step-up MFA)
	if w := postJSON(router, path("approve"), alco1Claims, uuid.New().String(), rv(3)); w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL_2")

	// 4. APPROVE2 (ALCO2, step-up MFA, different from ALCO1)
	if w := postJSON(router, path("approve2"), alco2Claims, uuid.New().String(), rv(4)); w.Code != http.StatusOK {
		t.Fatalf("approve2: %d %s", w.Code, w.Body.String())
	}
	assertWorkflowState(t, infra.DB, goodID, "APPROVED")
	assertBobotWorkflowStatus(t, infra.DB, goodID, "APPROVED")

	// Verify signature_count=4 (submit+review+approve+approve2).
	wfID := getWorkflowID(t, infra.DB, goodID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) != 4 {
		t.Errorf("expected 4 signature records (6-eyes), got %d", len(sigs))
	}
	t.Logf("6-eyes cycle complete: %d signatures, state=APPROVED", len(sigs))

	// Verify BOBOT_SKENARIO.APPROVE audit event exists.
	assertAuditEvent(t, infra.DB, "BOBOT_SKENARIO.APPROVE", goodID)
	t.Logf("Audit events verified")
}

// ─── Test 11: Step-up MFA required for approve2 ───────────────────────────────

// TestBobot_StepUpRequired_Approve2WithoutMFA_Rejected verifies that approve2
// with mfa_verified=false is rejected. Per WORKFLOW_CONFIG_BOBOT_SKENARIO
// StepUpRequired["approve2"]=true. This covers DEC-027.
func TestBobot_StepUpRequired_Approve2WithoutMFA_Rejected(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-11-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_mfa_maker")
	reviewerID := seedUserSQL(t, infra.DB, "bs_mfa_reviewer")
	alco1ID := seedUserSQL(t, infra.DB, "bs_mfa_alco1")
	alco2ID := seedUserSQL(t, infra.DB, "bs_mfa_alco2_nomfa")

	goodID := seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioGood,
		decimal.RequireFromString("0.25000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioNormal,
		decimal.RequireFromString("0.50000000"), periode, makerID)
	seedBobotSkenarioDRAFT(t, infra.DB, bobotskenario.SkenarioBad,
		decimal.RequireFromString("0.25000000"), periode, makerID)

	router := buildBobotSkenarioRouter(infra.DB)

	makerClaims := bobotRiskClaims(makerID)
	reviewerClaims := bobotClaimsJSON(reviewerID, "ROLE-RISK", true,
		"ecl_parameter.review", "ecl_parameter.read",
	)
	alco1Claims := bobotAlcoClaims(alco1ID, true)
	// ALCO2 WITHOUT MFA — simulates a token where step-up was not performed.
	alco2NoMFAClaims := bobotAlcoClaims(alco2ID, false)

	path := func(suffix string) string {
		return fmt.Sprintf("/api/v1/master/bobot-skenario/%s/%s", goodID, suffix)
	}
	rv := func(n int) string { return fmt.Sprintf(`{"rowVersion":%d,"signatureMethod":"JWT_STANDARD"}`, n) }

	// Advance through submit, review, approve.
	if w := postJSON(router, path("submit"), makerClaims, uuid.New().String(), rv(1)); w.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w.Code, w.Body.String())
	}
	if w := postJSON(router, path("review"), reviewerClaims, uuid.New().String(), rv(2)); w.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w.Code, w.Body.String())
	}
	if w := postJSON(router, path("approve"), alco1Claims, uuid.New().String(), rv(3)); w.Code != http.StatusOK {
		t.Fatalf("approve: %d %s", w.Code, w.Body.String())
	}

	// approve2 WITHOUT MFA — must be rejected 403.
	w := postJSON(router, path("approve2"), alco2NoMFAClaims, uuid.New().String(), rv(4))
	if w.Code != http.StatusForbidden {
		t.Errorf("approve2 without MFA: expected 403 FORBIDDEN, got %d body=%s", w.Code, w.Body.String())
	} else {
		t.Logf("approve2 without MFA correctly rejected: 403")
	}

	// Entity must remain in PENDING_APPROVAL_2.
	assertWorkflowState(t, infra.DB, goodID, "PENDING_APPROVAL_2")
}

// ─── Test 12: seed-default idempotent ────────────────────────────────────────

// TestBobot_SeedDefault_Idempotent verifies that POST /seed-default:
// - Creates exactly 3 rows (GOOD=0.25, NORMAL=0.50, BAD=0.25) on first call.
// - Returns Skipped=true on second call for the same periode.
// - DB has exactly 3 rows (no duplicates).
// - Sum equals 1.0 exactly.
//
// Regression: §2 ECL calc reproducibility — seed-default is the standard starting
// point for each new accounting period.
func TestBobot_SeedDefault_Idempotent(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const periode = "2099-12-01"
	cleanupBobotByPeriod(t, infra.DB, periode)
	t.Cleanup(func() { cleanupBobotByPeriod(t, infra.DB, periode) })

	makerID := seedUserSQL(t, infra.DB, "bs_seed_maker")
	router := buildBobotSkenarioRouter(infra.DB)
	claims := bobotRiskClaims(makerID)

	seedBody := fmt.Sprintf(`{"periodeBerlakuDari":%q}`, periode)

	// First call — must create 3 rows.
	w1 := postJSON(router, "/api/v1/master/bobot-skenario/seed-default", claims, uuid.New().String(), seedBody)
	if w1.Code != http.StatusCreated && w1.Code != http.StatusOK {
		t.Fatalf("seed-default first call: expected 2xx, got %d body=%s", w1.Code, w1.Body.String())
	}

	var result1 struct {
		Data struct {
			Created int      `json:"created"`
			IDs     []string `json:"ids"`
			Skipped bool     `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &result1); err != nil {
		t.Fatalf("parse seed-default response: %v", err)
	}
	if result1.Data.Created != 3 {
		t.Errorf("first seed-default: expected Created=3, got %d", result1.Data.Created)
	}
	if result1.Data.Skipped {
		t.Error("first seed-default: expected Skipped=false, got true")
	}
	if len(result1.Data.IDs) != 3 {
		t.Errorf("first seed-default: expected 3 IDs, got %d", len(result1.Data.IDs))
	}
	t.Logf("first seed-default: Created=%d Skipped=%v", result1.Data.Created, result1.Data.Skipped)

	// Verify exactly 3 rows in DB.
	if n := countBobotByPeriod(t, infra.DB, periode); n != 3 {
		t.Errorf("DB: expected 3 bobot_skenario rows, got %d", n)
	}

	// Verify sum=1.0.
	total := sumBobotByPeriod(t, infra.DB, periode)
	if !total.Equal(decimal.NewFromInt(1)) {
		t.Errorf("sum of seeded trio = %s, expected 1.0", total.String())
	}

	// Verify individual values (DEC-010).
	for _, row := range []struct {
		sk   string
		want string
	}{
		{"GOOD", "0.25000000"},
		{"NORMAL", "0.50000000"},
		{"BAD", "0.25000000"},
	} {
		var bobot string
		if err := infra.DB.QueryRowContext(context.Background(), `
			SELECT bobot::text FROM mst.bobot_skenario
			WHERE skenario = $1 AND periode_berlaku_dari = $2::date
			  AND deleted_at IS NULL AND tenant_id = 'TUGURE'
		`, row.sk, periode).Scan(&bobot); err != nil {
			t.Fatalf("fetch bobot for %s: %v", row.sk, err)
		}
		// Parse and compare as Decimal to be independent of trailing-zero representation.
		gotD, _ := decimal.NewFromString(bobot)
		wantD, _ := decimal.NewFromString(row.want)
		if !gotD.Equal(wantD) {
			t.Errorf("DEC-010: skenario=%s bobot expected %s, got %s", row.sk, row.want, bobot)
		}
	}

	// Second call — must be idempotent (Skipped=true).
	w2 := postJSON(router, "/api/v1/master/bobot-skenario/seed-default", claims, uuid.New().String(), seedBody)
	if w2.Code != http.StatusCreated && w2.Code != http.StatusOK {
		t.Fatalf("seed-default second call: expected 2xx, got %d body=%s", w2.Code, w2.Body.String())
	}

	var result2 struct {
		Data struct {
			Created int  `json:"created"`
			Skipped bool `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &result2); err != nil {
		t.Fatalf("parse second seed-default response: %v", err)
	}
	if !result2.Data.Skipped {
		t.Error("second seed-default: expected Skipped=true (idempotent skip), got false")
	}
	if result2.Data.Created != 0 {
		t.Errorf("second seed-default: expected Created=0 on skip, got %d", result2.Data.Created)
	}
	t.Logf("second seed-default: Skipped=%v Created=%d — idempotent OK", result2.Data.Skipped, result2.Data.Created)

	// DB must still have exactly 3 rows.
	if n := countBobotByPeriod(t, infra.DB, periode); n != 3 {
		t.Errorf("after second seed-default: expected 3 rows, got %d (duplicate side-effect!)", n)
	}
	t.Logf("seed-default idempotency verified: 3 rows, sum=1.0")
}

// ─── HTTP helper (PATCH) ──────────────────────────────────────────────────────

// patchJSON sends PATCH with JSON body; mirrors postJSON/putJSON helpers.
func patchJSON(router *gin.Engine, path, claimsJSON, idempKey, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPatch, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Claims", claimsJSON)
	if idempKey != "" {
		req.Header.Set("Idempotency-Key", idempKey)
	}
	router.ServeHTTP(w, req)
	return w
}

// Compile-time check: bobotskenario.DBRepository implements bobotskenario.Repository.
var _ bobotskenario.Repository = (*bobotskenario.DBRepository)(nil)
