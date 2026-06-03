//go:build integration

// Package integration — lgd_basel integration tests (APP-A-MSTR-004 / APP-C ECL Parameter).
//
// Coverage targets (all require live PostgreSQL; skip gracefully without infra):
//
//  1. TestLGDBasel_DuplicateTipeOverlap_Returns422
//     POST CORPORATE period 2024-01-01..null. POST CORPORATE 2025-01-01..null → 422 LGD_PERIOD_OVERLAP.
//
//  2. TestLGDBasel_LGDOutOfRange_Returns422
//     POST lgd="1.5000" → 422 VALIDATION_FAILED.
//     POST lgd="-0.1"   → 422 VALIDATION_FAILED.
//
//  3. TestLGDBasel_InvalidTipeEksposur_Returns422
//     POST tipeEksposur="UNKNOWN_TYPE" → 422 VALIDATION_FAILED.
//
//  4. TestLGDBasel_PeriodOrderInvalid_Returns422
//     POST periodeBerlakuSampai < periodeBerlakuDari → 422 VALIDATION_FAILED.
//
//  5. TestLGDBasel_OptimisticLock_Returns409
//     PATCH with stale row_version → 409 CONFLICT.
//
//  6. TestLGDBasel_SoDViolation_MakerCannotReview
//     Maker create + submit, same user tries review → 403 SOD_VIOLATION.
//
//  7. TestLGDBasel_SoDViolation_Approver2NotAnyPrevious
//     Maker → reviewer → approver1 (step-up). Same user as any prior actor tries approve2 → 403 SOD_VIOLATION.
//
//  8. TestLGDBasel_SixEyesCycle_Full_WithStepUpMFA (flagship)
//     DRAFT → submit → review → approve (step-up) → approve2 (step-up, different user) → APPROVED.
//     Verifies workflow state, 5 audit events, 4 signatures.
//     Without step-up token → approve returns 403 STEP_UP_REQUIRED.
//
//  9. TestLGDBasel_StepUpRequired_Approve2WithoutMFA_Rejected
//     Full path to PENDING_APPROVAL_2. Approver2 JWT without fresh step-up → 403 STEP_UP_REQUIRED.
//     Status remains PENDING_APPROVAL_2.
//
// 10. TestLGDBasel_DeleteWithECLReference_Returns409
//     Simulate ECL reference. DELETE → 409 ENTITY_IN_USE.
//
// 11. TestLGDBasel_Idempotency_Replay
//     Same Idempotency-Key + same payload → second call returns original 201, 1 DB row.
//
// 12. TestLGDBasel_Idempotency_Mismatch
//     Same Idempotency-Key + different lgd value → 422 IDEMPOTENCY_MISMATCH.

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/lgdbasel"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Workflow config ──────────────────────────────────────────────────────────

// lgdBaselWorkflowConfig returns the in-memory fallback Config for LGD_BASEL.
// Mirrors WORKFLOW_CONFIG_LGD_BASEL seeded in migration 0008 / DefaultConfigs().
// 6-eyes, both approve steps require step-up MFA, Approver2NotAnyPrevious=true.
func lgdBaselWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	// DefaultConfigs already includes LGD_BASEL; add it explicitly so this
	// function is self-documenting and the test is not silently relying on
	// DefaultConfigs being updated.
	cfgs["LGD_BASEL"] = &workflow.Config{
		EntityType:  "LGD_BASEL",
		Eyes:        6,
		Retractable: false,
		RequiredPermissions: map[string]string{
			"submit":   "ecl_parameter.submit",
			"review":   "ecl_parameter.review",
			"approve":  "ecl_parameter.approve",
			"approve2": "ecl_parameter.approve",
			"reject":   "ecl_parameter.reject",
		},
		// Both approve steps require step-up MFA (DEC-027, security-baseline §approve).
		StepUpRequired: map[string]bool{"approve": true, "approve2": true},
		SoDRules: workflow.SoDRulesConfig{
			ReviewerNotMaker:           true,
			ApproverNotMakerOrReviewer: true,
			Approver2NotAnyPrevious:    true, // approver2 ≠ maker ∧ ≠ reviewer ∧ ≠ approver1
		},
	}
	return cfgs
}

// ─── Router builder ───────────────────────────────────────────────────────────

// buildLGDBaselRouter constructs the full Gin test router for /api/v1/master/lgd-basel.
func buildLGDBaselRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware) // injects auth.Claims from X-Test-Claims header

	repo := lgdbasel.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := lgdbasel.NewService(repo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	// Try DB config first; fall back to in-memory (which always has LGD_BASEL).
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("LGD_BASEL"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(lgdBaselWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	h := lgdbasel.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	lgdbasel.RegisterRoutes(v1, h)
	return r
}

// ─── Claims builders ──────────────────────────────────────────────────────────

// lgdMakerClaims builds claims for ROLE-RISK (maker in 6-eyes LGD workflow).
func lgdMakerClaims(userID uuid.UUID) string {
	return lgdClaimsJSON(userID, "ROLE-RISK", false,
		"ecl_parameter.create", "ecl_parameter.read", "ecl_parameter.update",
		"ecl_parameter.delete", "ecl_parameter.submit", "ecl_parameter.export",
	)
}

// lgdReviewerClaims builds claims for ROLE-AKUN-CTL (reviewer in 6-eyes LGD workflow).
func lgdReviewerClaims(userID uuid.UUID) string {
	return lgdClaimsJSON(userID, "ROLE-AKUN-CTL", false,
		"ecl_parameter.read", "ecl_parameter.review", "ecl_parameter.reject",
	)
}

// lgdApproverClaims builds claims for ROLE-ALCO (approver1/approver2) WITH fresh step-up.
func lgdApproverClaims(userID uuid.UUID) string {
	return lgdClaimsJSON(userID, "ROLE-ALCO", true,
		"ecl_parameter.read", "ecl_parameter.approve", "ecl_parameter.reject",
	)
}

// lgdApproverClaimsNoStepUp builds ROLE-ALCO claims WITHOUT fresh step-up (stale).
func lgdApproverClaimsNoStepUp(userID uuid.UUID) string {
	return lgdClaimsJSON(userID, "ROLE-ALCO", false,
		"ecl_parameter.read", "ecl_parameter.approve", "ecl_parameter.reject",
	)
}

// lgdClaimsJSON serialises auth.Claims to JSON for use in X-Test-Claims.
// stepUpFresh=true sets StepupVerifiedAt to now (< 5 min → IsStepUpFresh() == true).
// stepUpFresh=false leaves StepupVerifiedAt nil (NeedsStepUp() == true).
func lgdClaimsJSON(userID uuid.UUID, role string, stepUpFresh bool, permissions ...string) string {
	now := time.Now().Unix()
	c := auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: "testuser_" + userID.String()[:8],
		Roles:             []string{role},
		Permissions:       permissions,
		TenantID:          "TUGURE",
		MFAVerified:       true,
		Exp:               now + 3600,
		Iat:               now,
	}
	if stepUpFresh {
		ts := now // verified right now (< 5 min threshold)
		c.StepupVerifiedAt = &ts
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedLGDBaselDRAFT inserts an mst.lgd_basel row in DRAFT state and seeds a
// matching workflow_instance. Returns the entity UUID.
func seedLGDBaselDRAFT(t *testing.T, db *sql.DB, tipe, dari string, sampai *string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()

	sampaiVal := "NULL"
	if sampai != nil {
		sampaiVal = fmt.Sprintf("'%s'", *sampai)
	}

	query := fmt.Sprintf(`
		INSERT INTO mst.lgd_basel (
			id, tipe_eksposur, lgd, karakteristik,
			periode_berlaku_dari, periode_berlaku_sampai,
			sumber, maker_id, approver_id, workflow_status,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			'%s', '%s', 0.4500, NULL,
			'%s', %s,
			'BASEL_III_IRB', '%s', NULL, 'DRAFT',
			now(), '%s', now(), '%s',
			1, 'TUGURE'
		)
	`, id, tipe, dari, sampaiVal, makerID, makerID, makerID)

	if _, err := db.ExecContext(context.Background(), query); err != nil {
		t.Fatalf("seedLGDBaselDRAFT insert: %v", err)
	}

	// Seed workflow instance. LGD_BASEL lives in the mst schema.
	seedLGDWorkflowInstance(t, db, id, makerID)

	return id
}

// seedLGDWorkflowInstance inserts a 6-eyes workflow_instance for LGD_BASEL.
func seedLGDWorkflowInstance(t *testing.T, db *sql.DB, entityID uuid.UUID, makerID uuid.UUID) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO sys.workflow_instance (
			entity_type, entity_id, entity_schema,
			workflow_config_key, eyes, current_state,
			maker_id, created_by, updated_by
		) VALUES ('LGD_BASEL', $1, 'mst', 'WORKFLOW_CONFIG_LGD_BASEL', 6, 'DRAFT', $2, $2, $2)
		ON CONFLICT (entity_type, entity_id) DO NOTHING
	`, entityID, makerID)
	if err != nil {
		t.Fatalf("seedLGDWorkflowInstance: %v", err)
	}
}

// cleanupLGDBasel removes test rows. Best-effort (won't fail the test).
func cleanupLGDBasel(t *testing.T, db *sql.DB, ids ...uuid.UUID) {
	t.Helper()
	for _, id := range ids {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.lgd_basel WHERE id = $1`, id)
	}
}

// cleanupLGDBaselByTipe removes all non-deleted rows for a tipe (test teardown).
func cleanupLGDBaselByTipe(t *testing.T, db *sql.DB, tipe string) {
	t.Helper()
	var ids []uuid.UUID
	rows, err := db.QueryContext(context.Background(),
		`SELECT id FROM mst.lgd_basel WHERE tipe_eksposur = $1`, tipe)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	cleanupLGDBasel(t, db, ids...)
}

// insertECLReference simulates an active ECL calc-result line referencing the LGD pool.
// This exercises the CountReferences guard in SoftDelete.
func insertECLReference(t *testing.T, db *sql.DB, lgdID uuid.UUID) uuid.UUID {
	t.Helper()
	refID := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO ecl.ecl_calc_result_line (
			id, lgd_pool_id,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, $2,
			now(), '00000000-0000-0000-0000-000000000001',
			now(), '00000000-0000-0000-0000-000000000001',
			1, 'TUGURE'
		)
	`, refID, lgdID)
	if err != nil {
		t.Fatalf("insertECLReference: %v\n(table may not exist yet — migration 0010 required)", err)
	}
	return refID
}

// cleanupECLReference removes the test ECL reference row.
func cleanupECLReference(t *testing.T, db *sql.DB, refID uuid.UUID) {
	t.Helper()
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM ecl.ecl_calc_result_line WHERE id = $1`, refID)
}

// lgdPath returns the LGD-Basel API path for a given entity UUID.
func lgdPath(entityID uuid.UUID) string {
	return "/api/v1/master/lgd-basel/" + entityID.String()
}

// ─── HTTP helpers (lgd-specific shortcuts) ───────────────────────────────────

// patchJSON sends PATCH with JSON body.
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

// ─── Test 1: Period overlap → 422 ─────────────────────────────────────────────

// TestLGDBasel_DuplicateTipeOverlap_Returns422 verifies that creating a second
// active (period open-ended) LGD entry for the same tipe_eksposur returns 422
// LGD_PERIOD_OVERLAP.
//
// Covers: regression §2 (ECL reproducibility — duplicate LGD pools must be blocked).
func TestLGDBasel_DuplicateTipeOverlap_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const tipe = "CORPORATE"
	cleanupLGDBaselByTipe(t, infra.DB, tipe)
	t.Cleanup(func() { cleanupLGDBaselByTipe(t, infra.DB, tipe) })

	router := buildLGDBaselRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "lgd_overlap_maker")
	claims := lgdMakerClaims(makerID)

	// First entry: CORPORATE active from 2024-01-01 (open-ended).
	body1 := `{
		"tipeEksposur": "CORPORATE",
		"lgd": "0.4500",
		"periodeBerlakuDari": "2024-01-01",
		"sumber": "BASEL_III_IRB"
	}`
	w1 := postJSON(router, "/api/v1/master/lgd-basel", claims, uuid.New().String(), body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first CORPORATE entry created OK")

	// Second entry: CORPORATE active from 2025-01-01 (also open-ended) — must overlap.
	body2 := `{
		"tipeEksposur": "CORPORATE",
		"lgd": "0.5000",
		"periodeBerlakuDari": "2025-01-01",
		"sumber": "BASEL_III_IRB"
	}`
	w2 := postJSON(router, "/api/v1/master/lgd-basel", claims, uuid.New().String(), body2)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("overlap: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "LGD_PERIOD_OVERLAP" {
		t.Errorf("expected error code LGD_PERIOD_OVERLAP, got %q", code)
	}
	t.Logf("period overlap correctly rejected: 422 LGD_PERIOD_OVERLAP")
}

// ─── Test 2: LGD out of range → 422 ──────────────────────────────────────────

// TestLGDBasel_LGDOutOfRange_Returns422 verifies server-side range validation
// for the lgd field (must be [0, 1]).
//
// Covers: regression §2 (ECL formula correctness — LGD ∈ [0,1] is a mathematical
// precondition for the ECL formula).
func TestLGDBasel_LGDOutOfRange_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildLGDBaselRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "lgd_range_maker")
	claims := lgdMakerClaims(makerID)

	cases := []struct {
		name string
		lgd  string
	}{
		{"above 1", "1.5000"},
		{"negative", "-0.1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{
				"tipeEksposur": "BANK",
				"lgd": "%s",
				"periodeBerlakuDari": "2026-01-01"
			}`, tc.lgd)
			w := postJSON(router, "/api/v1/master/lgd-basel", claims, uuid.New().String(), body)
			if w.Code != http.StatusUnprocessableEntity {
				t.Errorf("lgd=%s: expected 422, got %d body=%s", tc.lgd, w.Code, w.Body.String())
			}
			if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
				t.Errorf("lgd=%s: expected VALIDATION_FAILED, got %q", tc.lgd, code)
			}
			t.Logf("lgd=%s correctly rejected: 422 VALIDATION_FAILED", tc.lgd)
		})
	}
}

// ─── Test 3: Invalid tipe_eksposur → 422 ─────────────────────────────────────

// TestLGDBasel_InvalidTipeEksposur_Returns422 verifies whitelist enforcement for
// the tipe_eksposur field.
//
// Covers: service-layer enum validation (prevents garbage data from reaching ECL engine).
func TestLGDBasel_InvalidTipeEksposur_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildLGDBaselRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "lgd_tipe_maker")
	claims := lgdMakerClaims(makerID)

	body := `{
		"tipeEksposur": "UNKNOWN_TYPE",
		"lgd": "0.4500",
		"periodeBerlakuDari": "2026-01-01"
	}`
	w := postJSON(router, "/api/v1/master/lgd-basel", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid tipe: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("invalid tipeEksposur correctly rejected: 422 VALIDATION_FAILED")
}

// ─── Test 4: Period order invalid → 422 ──────────────────────────────────────

// TestLGDBasel_PeriodOrderInvalid_Returns422 verifies that submitting a request
// where periodeBerlakuSampai < periodeBerlakuDari is rejected with 422.
func TestLGDBasel_PeriodOrderInvalid_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildLGDBaselRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "lgd_period_order_maker")
	claims := lgdMakerClaims(makerID)

	body := `{
		"tipeEksposur": "SOVEREIGN",
		"lgd": "0.1000",
		"periodeBerlakuDari": "2026-12-31",
		"periodeBerlakuSampai": "2026-01-01"
	}`
	w := postJSON(router, "/api/v1/master/lgd-basel", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("period order invalid: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("period order invalid correctly rejected: 422 VALIDATION_FAILED")
}

// ─── Test 5: Optimistic lock → 409 ───────────────────────────────────────────

// TestLGDBasel_OptimisticLock_Returns409 verifies that a PATCH with a stale
// row_version returns 409 CONFLICT.
//
// Covers: regression §1 (ECL reproducibility — stale-write blocked).
func TestLGDBasel_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "lgd_optlock_maker")
	entityID := seedLGDBaselDRAFT(t, infra.DB, "RETAIL", "2026-01-01", nil, makerID)
	t.Cleanup(func() { cleanupLGDBasel(t, infra.DB, entityID) })

	router := buildLGDBaselRouter(infra.DB)
	claims := lgdMakerClaims(makerID)

	// First PATCH with rowVersion=1 — should succeed (bumps to 2).
	update1 := `{"lgd": "0.4600", "rowVersion": 1}`
	w1 := patchJSON(router, lgdPath(entityID), claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first patch: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first patch OK — row_version now 2")

	// Second PATCH with stale rowVersion=1 — must return 409.
	update2 := `{"lgd": "0.5000", "rowVersion": 1}`
	w2 := patchJSON(router, lgdPath(entityID), claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion: 409 CONFLICT")
}

// ─── Test 6: SoD — maker cannot review ───────────────────────────────────────

// TestLGDBasel_SoDViolation_MakerCannotReview verifies that the maker of a
// lgd_basel entry cannot act as reviewer on the same entity (server-side SoD).
//
// Covers: regression §6 (SoD at API level, not just UI), security-baseline.md.
func TestLGDBasel_SoDViolation_MakerCannotReview(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "lgd_sod_mk_rev_maker")
	entityID := seedLGDBaselDRAFT(t, infra.DB, "EQUITY", "2026-01-01", nil, makerID)
	t.Cleanup(func() { cleanupLGDBasel(t, infra.DB, entityID) })

	router := buildLGDBaselRouter(infra.DB)

	// Maker JWT also has review permission — simulates bypass attempt.
	makerClaims := lgdClaimsJSON(makerID, "ROLE-RISK", false,
		"ecl_parameter.create", "ecl_parameter.read", "ecl_parameter.submit",
		"ecl_parameter.review", "ecl_parameter.approve",
	)

	// Step 1: SUBMIT as maker → PENDING_REVIEW.
	rv1 := `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`
	w1 := postJSON(router, lgdPath(entityID)+"/submit", makerClaims, uuid.New().String(), rv1)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("submit OK → PENDING_REVIEW")

	// Step 2: REVIEW attempt as same maker — must be blocked by SoD.
	rv2 := `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`
	w2 := postJSON(router, lgdPath(entityID)+"/review", makerClaims, uuid.New().String(), rv2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-reviewer: expected 403 SOD_VIOLATION, got %d body=%s",
			w2.Code, w2.Body.String())
	} else {
		if code := errCode(w2.Body.Bytes()); code != "SOD_VIOLATION" {
			t.Errorf("expected SOD_VIOLATION, got %q", code)
		}
		t.Logf("SoD correctly blocked maker-as-reviewer: 403 SOD_VIOLATION")
	}

	// State must still be PENDING_REVIEW (not tampered).
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
}

// ─── Test 7: SoD — approver2 cannot be any previous actor ────────────────────

// TestLGDBasel_SoDViolation_Approver2NotAnyPrevious verifies that a user who
// participated as maker, reviewer, or approver1 is blocked from acting as approver2.
//
// Covers: regression §6 (6-eyes SoD — Approver2NotAnyPrevious=true),
// security-baseline.md.
func TestLGDBasel_SoDViolation_Approver2NotAnyPrevious(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "lgd_sod_a2_maker")
	reviewerID := seedUserSQL(t, infra.DB, "lgd_sod_a2_reviewer")
	approver1ID := seedUserSQL(t, infra.DB, "lgd_sod_a2_approver1")

	entityID := seedLGDBaselDRAFT(t, infra.DB, "REINSURANCE", "2026-01-01", nil, makerID)
	t.Cleanup(func() { cleanupLGDBasel(t, infra.DB, entityID) })

	router := buildLGDBaselRouter(infra.DB)

	makerClaims := lgdMakerClaims(makerID)
	reviewerClaims := lgdReviewerClaims(reviewerID)
	approver1Claims := lgdApproverClaims(approver1ID) // with step-up

	// SUBMIT → PENDING_REVIEW.
	w1 := postJSON(router, lgdPath(entityID)+"/submit",
		makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", w1.Code, w1.Body.String())
	}

	// REVIEW → PENDING_APPROVAL.
	w2 := postJSON(router, lgdPath(entityID)+"/review",
		reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STEP_UP"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: %d %s", w2.Code, w2.Body.String())
	}

	// APPROVE (approver1, with step-up) → PENDING_APPROVAL_2.
	w3 := postJSON(router, lgdPath(entityID)+"/approve",
		approver1Claims, uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STEP_UP"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve (approver1): expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
	t.Logf("approve1 OK → PENDING_APPROVAL_2")

	// APPROVE2 attempts by previous actors — all must be blocked (Approver2NotAnyPrevious=true).
	type sodCase struct {
		name   string
		claims string
	}
	sodCases := []sodCase{
		{"maker as approver2", lgdApproverClaims(makerID)},
		{"reviewer as approver2", lgdApproverClaims(reviewerID)},
		{"approver1 as approver2", lgdApproverClaims(approver1ID)},
	}

	for i, sc := range sodCases {
		t.Run(sc.name, func(t *testing.T) {
			rv := int64(4 + i) // row_version advances with each attempt IF it were to succeed — but it won't
			body := fmt.Sprintf(`{"rowVersion":%d,"signatureMethod":"JWT_STEP_UP"}`, rv)
			w := postJSON(router, lgdPath(entityID)+"/approve2", sc.claims, uuid.New().String(), body)
			if w.Code != http.StatusForbidden {
				t.Errorf("%s: expected 403 SOD_VIOLATION, got %d body=%s",
					sc.name, w.Code, w.Body.String())
			} else {
				if code := errCode(w.Body.Bytes()); code != "SOD_VIOLATION" {
					t.Errorf("%s: expected SOD_VIOLATION, got %q", sc.name, code)
				}
				t.Logf("%s: blocked 403 SOD_VIOLATION", sc.name)
			}
		})
		// State must remain PENDING_APPROVAL_2 throughout.
		assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
	}
}

// ─── Test 8: Full 6-eyes cycle with step-up MFA (flagship) ───────────────────

// TestLGDBasel_SixEyesCycle_Full_WithStepUpMFA exercises the complete 6-eyes
// workflow for lgd_basel:
//
//	DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED
//
// Verifies:
//   - workflow_instance.current_state at each step
//   - 5 audit events: LGD_BASEL.CREATE, LGD_BASEL.SUBMIT, LGD_BASEL.REVIEW,
//     LGD_BASEL.APPROVE, LGD_BASEL.APPROVE_2 (or APPROVE2)
//   - 4 workflow_signature rows (submit + review + approve + approve2)
//   - Without fresh step-up token → approve returns 403 STEP_UP_REQUIRED
//
// Covers: regression §6 (SoD), §7 (audit trail), security-baseline.md DEC-027.
func TestLGDBasel_SixEyesCycle_Full_WithStepUpMFA(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "lgd_6eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "lgd_6eyes_reviewer")
	approver1ID := seedUserSQL(t, infra.DB, "lgd_6eyes_appr1")
	approver2ID := seedUserSQL(t, infra.DB, "lgd_6eyes_appr2")

	entityID := seedLGDBaselDRAFT(t, infra.DB, "BANK", "2026-06-01", nil, makerID)
	t.Cleanup(func() { cleanupLGDBasel(t, infra.DB, entityID) })

	router := buildLGDBaselRouter(infra.DB)

	makerClaims := lgdMakerClaims(makerID)
	reviewerClaims := lgdReviewerClaims(reviewerID)
	approver1WithStepUp := lgdApproverClaims(approver1ID)    // StepupVerifiedAt = now
	approver1NoStepUp := lgdApproverClaimsNoStepUp(approver1ID) // no step-up
	approver2WithStepUp := lgdApproverClaims(approver2ID)    // StepupVerifiedAt = now

	// ── Pre-flight: approve without step-up must return 403 STEP_UP_REQUIRED ──
	// We need to be in PENDING_APPROVAL state first. Use a fresh entity for this
	// sub-check so the main flow entity stays clean.
	{
		preID := seedLGDBaselDRAFT(t, infra.DB, "SOVEREIGN", "2026-05-01", nil, makerID)
		t.Cleanup(func() { cleanupLGDBasel(t, infra.DB, preID) })

		r2ID := seedUserSQL(t, infra.DB, "lgd_6eyes_pre_reviewer")
		preReviewerClaims := lgdReviewerClaims(r2ID)

		_ = postJSON(router, lgdPath(preID)+"/submit",
			makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
		_ = postJSON(router, lgdPath(preID)+"/review",
			preReviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
		assertWorkflowState(t, infra.DB, preID, "PENDING_APPROVAL")

		// Approve without step-up → must return 403 STEP_UP_REQUIRED.
		wNoStep := postJSON(router, lgdPath(preID)+"/approve",
			approver1NoStepUp, uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
		if wNoStep.Code != http.StatusForbidden {
			t.Errorf("approve without step-up: expected 403, got %d body=%s",
				wNoStep.Code, wNoStep.Body.String())
		} else {
			if code := errCode(wNoStep.Body.Bytes()); code != "STEP_UP_REQUIRED" {
				t.Errorf("expected STEP_UP_REQUIRED, got %q", code)
			}
			t.Logf("approve without step-up correctly rejected: 403 STEP_UP_REQUIRED")
		}
		// Pre-flight entity state must still be PENDING_APPROVAL.
		assertWorkflowState(t, infra.DB, preID, "PENDING_APPROVAL")
	}

	// ── Main 6-eyes happy path ────────────────────────────────────────────────

	// Step 1: SUBMIT (maker) → PENDING_REVIEW.
	w1 := postJSON(router, lgdPath(entityID)+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Submit LGD BANK 45%"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT → PENDING_REVIEW")

	// Step 2: REVIEW (reviewer) → PENDING_APPROVAL.
	w2 := postJSON(router, lgdPath(entityID)+"/review",
		reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK — LGD sesuai Basel III IRB"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("REVIEW → PENDING_APPROVAL")

	// Step 3: APPROVE (approver1, with step-up) → PENDING_APPROVAL_2.
	// Mock step-up: StepupVerifiedAt = now (< 5 min → IsStepUpFresh() = true).
	w3 := postJSON(router, lgdPath(entityID)+"/approve",
		approver1WithStepUp, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STEP_UP","comment":"ALCO approval 1: LGD 45% disetujui"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve (step1): expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
	t.Logf("APPROVE (approver1, step-up) → PENDING_APPROVAL_2")

	// Step 4: APPROVE2 without step-up (approver2) → must fail 403 STEP_UP_REQUIRED.
	wNoStep2 := postJSON(router, lgdPath(entityID)+"/approve2",
		lgdApproverClaimsNoStepUp(approver2ID), uuid.New().String(),
		`{"rowVersion":4,"signatureMethod":"JWT_STANDARD"}`)
	if wNoStep2.Code != http.StatusForbidden {
		t.Errorf("approve2 without step-up: expected 403, got %d body=%s",
			wNoStep2.Code, wNoStep2.Body.String())
	} else {
		if code := errCode(wNoStep2.Body.Bytes()); code != "STEP_UP_REQUIRED" {
			t.Errorf("expected STEP_UP_REQUIRED, got %q", code)
		}
		t.Logf("approve2 without step-up correctly rejected: 403 STEP_UP_REQUIRED")
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")

	// Step 5: APPROVE2 (approver2, fresh step-up, different from all prior actors) → APPROVED.
	w5 := postJSON(router, lgdPath(entityID)+"/approve2",
		approver2WithStepUp, uuid.New().String(),
		`{"rowVersion":4,"signatureMethod":"JWT_STEP_UP","comment":"ALCO approval 2: konfirmasi LGD BANK 45%"}`)
	if w5.Code != http.StatusOK {
		t.Fatalf("approve2 (step-up): expected 200, got %d body=%s", w5.Code, w5.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	t.Logf("APPROVE2 (approver2, step-up) → APPROVED")

	// ── Verifications ─────────────────────────────────────────────────────────

	// Verify mst.lgd_basel.workflow_status synced to APPROVED.
	var dbStatus string
	if err := infra.DB.QueryRowContext(context.Background(),
		`SELECT workflow_status FROM mst.lgd_basel WHERE id = $1`, entityID,
	).Scan(&dbStatus); err != nil {
		t.Fatalf("fetch lgd_basel workflow_status: %v", err)
	}
	if dbStatus != "APPROVED" {
		t.Errorf("mst.lgd_basel.workflow_status: expected APPROVED, got %s", dbStatus)
	}

	// Verify audit events: CREATE + SUBMIT + REVIEW + APPROVE + APPROVE_2 (or APPROVE2).
	for _, action := range []string{
		"LGD_BASEL.CREATE",
		"LGD_BASEL.SUBMIT",
		"LGD_BASEL.REVIEW",
		"LGD_BASEL.APPROVE",
	} {
		assertAuditEvent(t, infra.DB, action, entityID)
	}
	// The approve2 audit event name may be APPROVE_2 or APPROVE2 depending on implementation.
	var approve2Count int
	_ = infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log
		WHERE (action = 'LGD_BASEL.APPROVE_2' OR action = 'LGD_BASEL.APPROVE2')
		  AND entity_id = $1
	`, entityID).Scan(&approve2Count)
	if approve2Count == 0 {
		t.Errorf("audit_log: expected APPROVE_2/APPROVE2 event for entity %s, got 0", entityID)
	}

	// Verify 4 workflow_signature rows.
	wfID := getWorkflowID(t, infra.DB, entityID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 4 {
		t.Errorf("expected >= 4 signature records (submit+review+approve+approve2), got %d", len(sigs))
	}
	t.Logf("6-eyes cycle complete: %d signatures, state=APPROVED", len(sigs))
}

// ─── Test 9: Approve2 without step-up leaves state unchanged ─────────────────

// TestLGDBasel_StepUpRequired_Approve2WithoutMFA_Rejected verifies that when
// approver2 JWT does not have a fresh step-up token, the approve2 endpoint returns
// 403 STEP_UP_REQUIRED and workflow state remains PENDING_APPROVAL_2.
//
// Covers: regression §7 (audit trail immutability indirectly — no state change
// without proper MFA), DEC-027, security-baseline §step-up.
func TestLGDBasel_StepUpRequired_Approve2WithoutMFA_Rejected(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "lgd_stepup2_maker")
	reviewerID := seedUserSQL(t, infra.DB, "lgd_stepup2_reviewer")
	approver1ID := seedUserSQL(t, infra.DB, "lgd_stepup2_appr1")
	approver2ID := seedUserSQL(t, infra.DB, "lgd_stepup2_appr2")

	entityID := seedLGDBaselDRAFT(t, infra.DB, "EQUITY", "2026-07-01", nil, makerID)
	t.Cleanup(func() { cleanupLGDBasel(t, infra.DB, entityID) })

	router := buildLGDBaselRouter(infra.DB)

	// Advance to PENDING_APPROVAL_2.
	_ = postJSON(router, lgdPath(entityID)+"/submit",
		lgdMakerClaims(makerID), uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")

	_ = postJSON(router, lgdPath(entityID)+"/review",
		lgdReviewerClaims(reviewerID), uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")

	_ = postJSON(router, lgdPath(entityID)+"/approve",
		lgdApproverClaims(approver1ID), uuid.New().String(), // with step-up
		`{"rowVersion":3,"signatureMethod":"JWT_STEP_UP"}`)
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
	t.Logf("setup complete: state=PENDING_APPROVAL_2")

	// Approver2 JWT WITHOUT fresh step-up (StepupVerifiedAt = nil → NeedsStepUp() = true).
	w := postJSON(router, lgdPath(entityID)+"/approve2",
		lgdApproverClaimsNoStepUp(approver2ID), uuid.New().String(),
		`{"rowVersion":4,"signatureMethod":"JWT_STANDARD"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("approve2 without step-up: expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "STEP_UP_REQUIRED" {
		t.Errorf("expected STEP_UP_REQUIRED, got %q", code)
	}
	t.Logf("approve2 without step-up correctly rejected: 403 STEP_UP_REQUIRED")

	// State must remain PENDING_APPROVAL_2.
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
}

// ─── Test 10: Delete with ECL reference → 409 ────────────────────────────────

// TestLGDBasel_DeleteWithECLReference_Returns409 verifies that soft-deleting an
// lgd_basel row that is referenced by an active ecl_calc_result_line is blocked
// with 409 ENTITY_IN_USE.
//
// Covers: referential integrity guard, prevents orphaned ECL results.
func TestLGDBasel_DeleteWithECLReference_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "lgd_delref_maker")
	entityID := seedLGDBaselDRAFT(t, infra.DB, "CORPORATE", "2025-01-01", nil, makerID)

	// Insert ECL reference BEFORE registering cleanup so it is removed first.
	refID := insertECLReference(t, infra.DB, entityID)
	t.Cleanup(func() {
		cleanupECLReference(t, infra.DB, refID)
		cleanupLGDBasel(t, infra.DB, entityID)
	})

	router := buildLGDBaselRouter(infra.DB)
	claims := lgdMakerClaims(makerID)

	w := deleteReq(router, lgdPath(entityID), claims, uuid.New().String())
	if w.Code != http.StatusConflict {
		t.Errorf("delete with ECL ref: expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "ENTITY_IN_USE" {
		t.Errorf("expected ENTITY_IN_USE, got %q", code)
	}
	t.Logf("delete with ECL reference correctly blocked: 409 ENTITY_IN_USE")

	// Row must still exist (not soft-deleted).
	var deletedAt *time.Time
	if err := infra.DB.QueryRowContext(context.Background(),
		`SELECT deleted_at FROM mst.lgd_basel WHERE id = $1`, entityID,
	).Scan(&deletedAt); err != nil {
		t.Fatalf("DB check: %v", err)
	}
	if deletedAt != nil {
		t.Errorf("row was soft-deleted despite 409 guard: deleted_at=%v", deletedAt)
	}
}

// ─── Test 11: Idempotency replay ─────────────────────────────────────────────

// TestLGDBasel_Idempotency_Replay verifies that replaying a POST create with the
// same Idempotency-Key and payload returns the original 201 and creates exactly
// one database row.
//
// Covers: regression §8 (idempotency — no duplicate side-effects).
func TestLGDBasel_Idempotency_Replay(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const tipe = "SOVEREIGN"
	cleanupLGDBaselByTipe(t, infra.DB, tipe)
	t.Cleanup(func() { cleanupLGDBaselByTipe(t, infra.DB, tipe) })

	router := buildLGDBaselRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "lgd_idemp_replay_maker")
	claims := lgdMakerClaims(makerID)

	idempKey := uuid.New().String()
	body := `{
		"tipeEksposur": "SOVEREIGN",
		"lgd": "0.0500",
		"periodeBerlakuDari": "2026-06-01",
		"sumber": "BASEL_III_IRB"
	}`

	// First request.
	w1 := postJSON(router, "/api/v1/master/lgd-basel", claims, idempKey, body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: 201")

	// Second request — same key, same body → must return original 201.
	w2 := postJSON(router, "/api/v1/master/lgd-basel", claims, idempKey, body)
	if w2.Code != http.StatusCreated {
		t.Errorf("replay: expected 201 (original replayed), got %d body=%s", w2.Code, w2.Body.String())
	}

	// Only 1 row must exist for SOVEREIGN.
	var count int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.lgd_basel
		WHERE tipe_eksposur = 'SOVEREIGN' AND deleted_at IS NULL
	`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 SOVEREIGN row, got %d (duplicate side-effect!)", count)
	}

	// Only 1 audit event for LGD_BASEL.CREATE for SOVEREIGN.
	var auditCount int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log
		WHERE action = 'LGD_BASEL.CREATE'
		  AND after_value::text LIKE '%SOVEREIGN%'
	`).Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount > 1 {
		t.Errorf("idempotency replay created %d audit events, expected 1", auditCount)
	}
	t.Logf("idempotency replay: OK — 1 DB row, %d audit events", auditCount)
}

// ─── Test 12: Idempotency mismatch → 422 ─────────────────────────────────────

// TestLGDBasel_Idempotency_Mismatch verifies that replaying the same
// Idempotency-Key with a different lgd value returns 422 IDEMPOTENCY_MISMATCH
// and leaves the original record unchanged.
//
// Covers: regression §8 (idempotency mismatch protection).
func TestLGDBasel_Idempotency_Mismatch(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	const tipe = "RETAIL"
	cleanupLGDBaselByTipe(t, infra.DB, tipe)
	t.Cleanup(func() { cleanupLGDBaselByTipe(t, infra.DB, tipe) })

	router := buildLGDBaselRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "lgd_idemp_mismatch_maker")
	claims := lgdMakerClaims(makerID)

	idempKey := uuid.New().String()
	body1 := `{
		"tipeEksposur": "RETAIL",
		"lgd": "0.3000",
		"periodeBerlakuDari": "2026-06-01",
		"sumber": "BASEL_III_IRB"
	}`
	body2 := `{
		"tipeEksposur": "RETAIL",
		"lgd": "0.9999",
		"periodeBerlakuDari": "2026-06-01",
		"sumber": "BASEL_III_IRB"
	}`

	// First request — succeeds.
	w1 := postJSON(router, "/api/v1/master/lgd-basel", claims, idempKey, body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}

	// Second request — same key, different lgd → 422.
	w2 := postJSON(router, "/api/v1/master/lgd-basel", claims, idempKey, body2)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("mismatch: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "IDEMPOTENCY_MISMATCH" {
		t.Errorf("expected IDEMPOTENCY_MISMATCH, got %q", code)
	}

	// Original lgd must be unchanged.
	var lgdVal string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT lgd::text FROM mst.lgd_basel
		WHERE tipe_eksposur = 'RETAIL' AND deleted_at IS NULL
	`).Scan(&lgdVal); err != nil {
		t.Fatalf("fetch lgd: %v", err)
	}
	// LGD 0.3000 stored as NUMERIC; accept prefix match.
	if !strings.HasPrefix(lgdVal, "0.3") {
		t.Errorf("original lgd changed by mismatch request: got %q, expected ~0.3000", lgdVal)
	}
	t.Logf("idempotency mismatch: 422 returned, original lgd=%s preserved", lgdVal)
}

// ─── Compile-time interface check ─────────────────────────────────────────────

// Verify DBRepository satisfies lgdbasel.Repository at compile time.
var _ lgdbasel.Repository = (*lgdbasel.DBRepository)(nil)
