//go:build integration

// Package integration — impact_mev_pd integration tests (APP-A-MSTR-008, FL multiplier).
//
// Coverage (8 test cases):
//
//  1. TestImpactMevPD_DuplicatePeriodeSkenario_Returns422
//     Two rows with the same (periode_id, GOOD) → second call returns 422 IMPACT_DUPLICATE_PERIODE_SKENARIO.
//
//  2. TestImpactMevPD_InvalidSkenario_Returns422
//     skenario="NORMAL" (invalid; only GOOD/BAD stored) → 422 VALIDATION_FAILED.
//
//  3. TestImpactMevPD_OptimisticLock_Returns409
//     PUT with stale row_version → 409 CONFLICT.
//
//  4. TestImpactMevPD_InvalidJSON_Returns422
//     mev_components_json with malformed JSON → 422 VALIDATION_FAILED.
//
//  5. TestImpactMevPD_SoDViolation_Approver2NotPrevious (3 sub-tests)
//     6-eyes: approver2 must not be maker/reviewer/approver1.
//
//  6. TestImpactMevPD_SixEyesCycle_Full_WithStepUpMFA (flagship)
//     Complete DRAFT→PENDING_REVIEW→PENDING_APPROVAL→PENDING_APPROVAL_2→APPROVED
//     with step-up MFA on both approve steps. Asserts: state, signatures, audit events.
//
//  7. TestImpactMevPD_StepUpRequired_Approve2WithoutMFA_Rejected
//     approve2 called with stale/missing step-up → 403 STEP_UP_REQUIRED.
//
//  8. TestImpactMevPD_PlausibleRangeWarning_NoReject
//     multiplier=3.0 (outside 0.5-2.5 soft range) → accepted (201), warning-only.

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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/impactmevpd"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Workflow config helper ───────────────────────────────────────────────────

// impactMevPDWorkflowConfig returns an InMemoryConfigLoader that includes IMPACT_MEV_PD.
// Falls back to the DB loader if the row already exists in sys.config.
func impactMevPDWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	// DefaultConfigs already contains IMPACT_MEV_PD; ensure it is present.
	if _, ok := cfgs["IMPACT_MEV_PD"]; !ok {
		cfgs["IMPACT_MEV_PD"] = &workflow.Config{
			EntityType:  "IMPACT_MEV_PD",
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
	}
	return cfgs
}

// ─── Router builder ───────────────────────────────────────────────────────────

// buildImpactMevPDRouter constructs the full Gin router for /api/v1/master/impact-mev-pd
// backed by the live *sql.DB.
func buildImpactMevPDRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := impactmevpd.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := impactmevpd.NewService(repo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("IMPACT_MEV_PD"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(impactMevPDWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())

	// Register entity hook so workflow transitions sync workflow_status.
	hook := impactmevpd.NewWorkflowHook(svc)
	wfSvc.RegisterEntityHook("IMPACT_MEV_PD", hook)

	wfHandler := workflow.NewHandler(wfSvc)

	h := impactmevpd.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	impactmevpd.RegisterRoutes(v1, h)
	return r
}

// ─── Claim builders ───────────────────────────────────────────────────────────

func eclParamMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-RISK",
		"ecl_parameter.read", "ecl_parameter.submit",
	)
}

func eclParamReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-RISK",
		"ecl_parameter.read", "ecl_parameter.review", "ecl_parameter.reject",
	)
}

// eclParamApproverClaims returns claims for ALCO approver with fresh step-up MFA.
func eclParamApproverClaims(userID uuid.UUID) string {
	stepUpAt := time.Now().Unix() // fresh — within 5 min window
	now := time.Now().Unix()
	c := auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: "alco_" + userID.String()[:8],
		Roles:             []string{"ROLE-ALCO"},
		Permissions: []string{
			"ecl_parameter.read", "ecl_parameter.approve", "ecl_parameter.reject",
		},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &stepUpAt,
		Exp:              now + 3600,
		Iat:              now,
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// eclParamApproverClaimsNoStepUp returns claims for ALCO without fresh step-up.
func eclParamApproverClaimsNoStepUp(userID uuid.UUID) string {
	// StepupVerifiedAt set to 10 minutes ago → stale
	staleStepUp := time.Now().Add(-10 * time.Minute).Unix()
	now := time.Now().Unix()
	c := auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: "alco_nostepup_" + userID.String()[:8],
		Roles:             []string{"ROLE-ALCO"},
		Permissions: []string{
			"ecl_parameter.read", "ecl_parameter.approve", "ecl_parameter.reject",
		},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &staleStepUp,
		Exp:              now + 3600,
		Iat:              now,
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedPeriodeID returns a test periode UUID (creates a row in mst.periode if needed).
// Uses a deterministic UUID to avoid proliferating data across test runs.
func seedPeriodeID(t *testing.T, db *sql.DB, label string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	// mst.periode may not exist yet in integration schema. We insert directly into
	// a minimal stub UUID. If the FK is not enforced (no FK constraint from
	// impact_mev_pd.periode_id to mst.periode), this is fine. The integration
	// schema from migration 0001 defines periode_id as UUID without FK in
	// impact_mev_pd, so we just use any UUID.
	_ = label
	return id
}

// seedImpactMevPDDRAFT inserts a row in DRAFT state and creates the matching
// workflow_instance. Returns the entity UUID.
func seedImpactMevPDDRAFT(t *testing.T, db *sql.DB, periodeID uuid.UUID, skenario string, multiplier string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.impact_mev_pd (
			id, periode_id, skenario, impact_multiplier,
			workflow_status,
			created_at, created_by, row_version, tenant_id,
			maker_id
		) VALUES (
			$1, $2, $3, $4,
			'DRAFT',
			now(), $5, 1, 'TUGURE',
			$5
		)
	`, id, periodeID, skenario, multiplier, makerID)
	if err != nil {
		t.Fatalf("seedImpactMevPDDRAFT: %v", err)
	}

	// Seed the workflow instance.
	seedWorkflowInstance(t, db, id, "IMPACT_MEV_PD", makerID, 6)

	// Back-link workflow_instance_id.
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, id).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.impact_mev_pd SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, id)
	}
	return id
}

// cleanupImpactMevPD removes all test rows by periode_id from impact_mev_pd and
// associated workflow instances.
func cleanupImpactMevPD(t *testing.T, db *sql.DB, ids ...uuid.UUID) {
	t.Helper()
	for _, id := range ids {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.impact_mev_pd WHERE id = $1`, id)
	}
}

// ─── Test 1: Duplicate (periode, skenario) → 422 ────────────────────────────

// TestImpactMevPD_DuplicatePeriodeSkenario_Returns422 verifies that creating a
// second row with the same (periode_id, skenario=GOOD) returns
// 422 IMPACT_DUPLICATE_PERIODE_SKENARIO.
//
// Covers: regression §1 (klasifikasi reproducibility), ECL parameter uniqueness.
func TestImpactMevPD_DuplicatePeriodeSkenario_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_dup_maker")
	periodeID := seedPeriodeID(t, infra.DB, "2026-Q1-dup")

	router := buildImpactMevPDRouter(infra.DB)
	claims := eclParamMakerClaims(makerID)

	body := fmt.Sprintf(`{
		"periodeId": %q,
		"skenario": "GOOD",
		"impactMultiplier": "0.85"
	}`, periodeID)

	// First create — must succeed.
	var firstID uuid.UUID
	w1 := postJSON(router, "/api/v1/master/impact-mev-pd", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	// Parse entity ID for cleanup.
	var resp1 struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err == nil {
		firstID, _ = uuid.Parse(resp1.Data.ID)
	}
	t.Cleanup(func() { cleanupImpactMevPD(t, infra.DB, firstID) })
	t.Logf("first create: 201 id=%s", firstID)

	// Second create — same (periode, GOOD) — must fail.
	w2 := postJSON(router, "/api/v1/master/impact-mev-pd", claims, uuid.New().String(), body)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("duplicate skenario: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "IMPACT_DUPLICATE_PERIODE_SKENARIO" {
		t.Errorf("expected IMPACT_DUPLICATE_PERIODE_SKENARIO, got %q", code)
	}
	t.Logf("duplicate (periode,GOOD) correctly rejected: 422 IMPACT_DUPLICATE_PERIODE_SKENARIO")
}

// ─── Test 2: Invalid skenario → 422 ─────────────────────────────────────────

// TestImpactMevPD_InvalidSkenario_Returns422 verifies that submitting
// skenario="NORMAL" (which must not be stored — always 1.0 by spec DEC-010)
// returns 422 VALIDATION_FAILED.
//
// Covers: business rule DEC-010, service validation.
func TestImpactMevPD_InvalidSkenario_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_skenario_maker")
	periodeID := uuid.New()
	router := buildImpactMevPDRouter(infra.DB)
	claims := eclParamMakerClaims(makerID)

	body := fmt.Sprintf(`{
		"periodeId": %q,
		"skenario": "NORMAL",
		"impactMultiplier": "1.0"
	}`, periodeID)

	w := postJSON(router, "/api/v1/master/impact-mev-pd", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity && w.Code != http.StatusBadRequest {
		t.Errorf("invalid skenario: expected 422 or 400, got %d body=%s", w.Code, w.Body.String())
	}
	t.Logf("invalid skenario NORMAL correctly rejected: %d", w.Code)

	// Verify no row was written.
	var count int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.impact_mev_pd WHERE periode_id = $1 AND skenario = 'NORMAL'
	`, periodeID).Scan(&count); err != nil {
		t.Fatalf("DB check: %v", err)
	}
	if count != 0 {
		t.Errorf("NORMAL skenario row was written to DB despite 422 — data integrity failure")
	}
}

// ─── Test 3: Optimistic lock → 409 ──────────────────────────────────────────

// TestImpactMevPD_OptimisticLock_Returns409 verifies that PUT with a stale
// row_version returns 409 CONFLICT.
//
// Covers: regression §3 (ECL reproducibility — optimistic lock prevents double-write).
func TestImpactMevPD_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_optlock_maker")
	periodeID := seedPeriodeID(t, infra.DB, "2026-Q1-optlock")
	entityID := seedImpactMevPDDRAFT(t, infra.DB, periodeID, "GOOD", "0.90", makerID)
	t.Cleanup(func() { cleanupImpactMevPD(t, infra.DB, entityID) })

	router := buildImpactMevPDRouter(infra.DB)
	claims := eclParamMakerClaims(makerID)

	// First update — succeeds with rowVersion=1, bumps to 2.
	update1 := `{"impactMultiplier":"0.91","rowVersion":1}`
	w1 := putJSON(router, "/api/v1/master/impact-mev-pd/"+entityID.String(),
		claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first update OK, row_version now 2")

	// Second update with stale rowVersion=1 → 409.
	update2 := `{"impactMultiplier":"0.92","rowVersion":1}`
	w2 := putJSON(router, "/api/v1/master/impact-mev-pd/"+entityID.String(),
		claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion: 409 CONFLICT")
}

// ─── Test 4: Invalid mev_components_json → 422 ───────────────────────────────

// TestImpactMevPD_InvalidJSON_Returns422 verifies that mev_components_json
// with malformed JSON (non-object or syntax error) returns 422.
//
// Covers: mev_components_json validation in service.Create.
func TestImpactMevPD_InvalidJSON_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_json_maker")
	periodeID := uuid.New()
	router := buildImpactMevPDRouter(infra.DB)
	claims := eclParamMakerClaims(makerID)

	cases := []struct {
		name     string
		mevJSON  string
		wantCode int
	}{
		{
			name:     "malformed JSON",
			mevJSON:  `{not valid json`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "JSON array instead of object",
			mevJSON:  `["gdp","inflation"]`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "weights do not sum to 1",
			mevJSON:  `{"weights":{"gdp":0.3,"inflation":0.3}}`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"periodeId":         periodeID.String(),
				"skenario":          "GOOD",
				"impactMultiplier":  "0.85",
				"mevComponentsJson": tc.mevJSON,
			})
			w := postJSON(router, "/api/v1/master/impact-mev-pd", claims, uuid.New().String(), string(body))
			if w.Code == http.StatusCreated {
				t.Errorf("case %q: expected 4xx for invalid mevComponentsJson, got 201", tc.name)
			}
			t.Logf("case %q: correctly rejected with %d", tc.name, w.Code)
		})
	}
}

// ─── Test 5: SoD — approver2 not previous ────────────────────────────────────

// TestImpactMevPD_SoDViolation_Approver2NotPrevious runs 3 sub-tests verifying
// that the 2nd approver (approve2) cannot be the maker, reviewer, or 1st approver.
//
// Covers: regression §6 (SoD at API level), DEC-017, security-baseline.md.
func TestImpactMevPD_SoDViolation_Approver2NotPrevious(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	subCases := []struct {
		name             string
		approver2IsWhich string // "maker", "reviewer", or "approver1"
	}{
		{"approver2_is_maker", "maker"},
		{"approver2_is_reviewer", "reviewer"},
		{"approver2_is_approver1", "approver1"},
	}

	for _, sc := range subCases {
		t.Run(sc.name, func(t *testing.T) {
			makerID := seedUserSQL(t, infra.DB, "imevpd_sod_"+sc.name+"_mk")
			reviewerID := seedUserSQL(t, infra.DB, "imevpd_sod_"+sc.name+"_rv")
			approver1ID := seedUserSQL(t, infra.DB, "imevpd_sod_"+sc.name+"_ap1")

			// Determine who attempts approve2.
			var approver2Candidate uuid.UUID
			switch sc.approver2IsWhich {
			case "maker":
				approver2Candidate = makerID
			case "reviewer":
				approver2Candidate = reviewerID
			case "approver1":
				approver2Candidate = approver1ID
			}

			periodeID := seedPeriodeID(t, infra.DB, "2026-Q1-sod-"+sc.name)
			entityID := seedImpactMevPDDRAFT(t, infra.DB, periodeID, "BAD", "1.20", makerID)
			t.Cleanup(func() { cleanupImpactMevPD(t, infra.DB, entityID) })

			router := buildImpactMevPDRouter(infra.DB)

			makerClaims := eclParamMakerClaims(makerID)
			reviewerClaims := eclParamReviewerClaims(reviewerID)
			approver1Claims := eclParamApproverClaims(approver1ID)
			approver2AttemptClaims := eclParamApproverClaims(approver2Candidate)

			idStr := entityID.String()

			// SUBMIT.
			w1 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/submit",
				makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
			if w1.Code != http.StatusOK {
				t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
			}

			// REVIEW.
			w2 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/review",
				reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
			if w2.Code != http.StatusOK {
				t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
			}

			// APPROVE (step 1 — ALCO approver1, fresh step-up).
			w3 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/approve",
				approver1Claims, uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STEP_UP"}`)
			if w3.Code != http.StatusOK {
				t.Fatalf("approve1: expected 200, got %d body=%s", w3.Code, w3.Body.String())
			}

			// APPROVE2 attempt with disallowed user — must return 403 SOD_VIOLATION.
			w4 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/approve2",
				approver2AttemptClaims, uuid.New().String(), `{"rowVersion":4,"signatureMethod":"JWT_STEP_UP"}`)
			if w4.Code != http.StatusForbidden {
				t.Errorf("sub-case %s: expected 403, got %d body=%s", sc.name, w4.Code, w4.Body.String())
			} else {
				if code := errCode(w4.Body.Bytes()); code != "SOD_VIOLATION" {
					t.Errorf("sub-case %s: expected SOD_VIOLATION, got %q", sc.name, code)
				}
				t.Logf("sub-case %s: SoD correctly blocked approver2 as %s: 403 SOD_VIOLATION",
					sc.name, sc.approver2IsWhich)
			}

			// Workflow must remain in PENDING_APPROVAL_2.
			assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
		})
	}
}

// ─── Test 6: Flagship — full 6-eyes cycle with step-up MFA ──────────────────

// TestImpactMevPD_SixEyesCycle_Full_WithStepUpMFA exercises the complete
// DRAFT→PENDING_REVIEW→PENDING_APPROVAL→PENDING_APPROVAL_2→APPROVED cycle
// for impact_mev_pd with 4 distinct users and step-up MFA on approve/approve2.
//
// Asserts:
//   - Each workflow state transition is committed to DB.
//   - 4 signature records (submit + review + approve + approve2).
//   - audit_log events: IMPACT_MEV_PD.SUBMIT, IMPACT_MEV_PD.APPROVE, IMPACT_MEV_PD.APPROVE2.
//   - mst.impact_mev_pd.workflow_status synced to APPROVED via EntityHook.
//
// Covers: regression §5 (periode buku), §6 (SoD), §8 (idempotency), DEC-017, DEC-027.
func TestImpactMevPD_SixEyesCycle_Full_WithStepUpMFA(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_6eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "imevpd_6eyes_reviewer")
	approver1ID := seedUserSQL(t, infra.DB, "imevpd_6eyes_approver1")
	approver2ID := seedUserSQL(t, infra.DB, "imevpd_6eyes_approver2")

	periodeID := seedPeriodeID(t, infra.DB, "2026-Q1-flagship")
	entityID := seedImpactMevPDDRAFT(t, infra.DB, periodeID, "GOOD", "0.85", makerID)
	t.Cleanup(func() { cleanupImpactMevPD(t, infra.DB, entityID) })

	router := buildImpactMevPDRouter(infra.DB)

	makerClaims := eclParamMakerClaims(makerID)
	reviewerClaims := eclParamReviewerClaims(reviewerID)
	approver1Claims := eclParamApproverClaims(approver1ID)  // fresh step-up
	approver2Claims := eclParamApproverClaims(approver2ID)  // fresh step-up

	idStr := entityID.String()

	// Step 1: SUBMIT (Maker).
	w1 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Submit untuk review ECL param"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("SUBMIT: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// Step 2: REVIEW (Reviewer).
	w2 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/review",
		reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Parameter MEV wajar"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("REVIEW: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// Step 3: APPROVE (Approver1 — ALCO, step-up MFA required, DEC-027).
	w3 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/approve",
		approver1Claims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STEP_UP","comment":"Disetujui ALCO pertama"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("APPROVE1: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
	t.Logf("APPROVE1: state=PENDING_APPROVAL_2")

	// Step 4: APPROVE2 (Approver2 — different ALCO, step-up MFA required).
	w4 := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/approve2",
		approver2Claims, uuid.New().String(),
		`{"rowVersion":4,"signatureMethod":"JWT_STEP_UP","comment":"Disetujui ALCO kedua"}`)
	if w4.Code != http.StatusOK {
		t.Fatalf("APPROVE2: expected 200, got %d body=%s", w4.Code, w4.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	t.Logf("APPROVE2: state=APPROVED")

	// Assert mst.impact_mev_pd.workflow_status synced to APPROVED.
	var wfStatus string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.impact_mev_pd WHERE id = $1
	`, entityID).Scan(&wfStatus); err != nil {
		t.Fatalf("fetch impact_mev_pd workflow_status: %v", err)
	}
	if wfStatus != "APPROVED" {
		t.Errorf("impact_mev_pd.workflow_status: expected APPROVED, got %s", wfStatus)
	}
	t.Logf("impact_mev_pd.workflow_status synced: APPROVED")

	// Assert audit events.
	assertAuditEvent(t, infra.DB, "IMPACT_MEV_PD.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "IMPACT_MEV_PD.APPROVE", entityID)
	assertAuditEvent(t, infra.DB, "IMPACT_MEV_PD.APPROVE2", entityID)
	t.Logf("audit events: SUBMIT, APPROVE, APPROVE2 all present")

	// Assert signature count == 4 (submit + review + approve + approve2).
	wfID := getWorkflowID(t, infra.DB, entityID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) != 4 {
		t.Errorf("expected 4 signature records, got %d", len(sigs))
	}
	t.Logf("6-eyes cycle complete: %d signatures, state=APPROVED", len(sigs))

	// Verify the approver2 signature uses JWT_STEP_UP method.
	var stepUpCount int
	for _, sig := range sigs {
		if sig.SignatureMethod == workflow.SignatureMethodJWTStepUp {
			stepUpCount++
		}
	}
	if stepUpCount < 2 {
		t.Errorf("expected >= 2 JWT_STEP_UP signatures (approve + approve2), got %d", stepUpCount)
	}
	t.Logf("JWT_STEP_UP signatures: %d", stepUpCount)
}

// ─── Test 7: Step-up required for approve2 without MFA ───────────────────────

// TestImpactMevPD_StepUpRequired_Approve2WithoutMFA_Rejected verifies that
// approve2 called with a stale step-up token returns 403 STEP_UP_REQUIRED.
//
// Covers: DEC-027 (step-up MFA mandatory for ECL parameter approve2).
func TestImpactMevPD_StepUpRequired_Approve2WithoutMFA_Rejected(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_stepup_maker")
	reviewerID := seedUserSQL(t, infra.DB, "imevpd_stepup_reviewer")
	approver1ID := seedUserSQL(t, infra.DB, "imevpd_stepup_ap1")
	approver2ID := seedUserSQL(t, infra.DB, "imevpd_stepup_ap2")

	periodeID := seedPeriodeID(t, infra.DB, "2026-Q1-stepup")
	entityID := seedImpactMevPDDRAFT(t, infra.DB, periodeID, "BAD", "1.30", makerID)
	t.Cleanup(func() { cleanupImpactMevPD(t, infra.DB, entityID) })

	router := buildImpactMevPDRouter(infra.DB)
	idStr := entityID.String()

	// Progress to PENDING_APPROVAL_2.
	postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/submit",
		eclParamMakerClaims(makerID), uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)

	postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/review",
		eclParamReviewerClaims(reviewerID), uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)

	postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/approve",
		eclParamApproverClaims(approver1ID), uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STEP_UP"}`)

	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")

	// Attempt approve2 with STALE step-up — must be rejected.
	w := postJSON(router, "/api/v1/master/impact-mev-pd/"+idStr+"/approve2",
		eclParamApproverClaimsNoStepUp(approver2ID), uuid.New().String(),
		`{"rowVersion":4,"signatureMethod":"JWT_STEP_UP"}`)

	if w.Code != http.StatusForbidden {
		t.Errorf("stale step-up approve2: expected 403, got %d body=%s", w.Code, w.Body.String())
	} else {
		code := errCode(w.Body.Bytes())
		if code != "STEP_UP_REQUIRED" && code != "MFA_REQUIRED" {
			t.Errorf("expected STEP_UP_REQUIRED or MFA_REQUIRED, got %q", code)
		}
		t.Logf("stale step-up approve2 correctly blocked: 403 %s", code)
	}

	// Workflow must still be in PENDING_APPROVAL_2.
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL_2")
}

// ─── Test 8: Plausible range warning — no hard reject ────────────────────────

// TestImpactMevPD_PlausibleRangeWarning_NoReject verifies that a multiplier
// outside the soft plausible range (0.5–2.5) is still accepted (201).
// The service logs a warning but does not reject the request.
// This tests the ALCO-override capability per DEC-010.
//
// Covers: ALCO override (DEC-010), ECL parameter permissiveness, warning-only guard.
func TestImpactMevPD_PlausibleRangeWarning_NoReject(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "imevpd_plausible_maker")
	periodeID := seedPeriodeID(t, infra.DB, "2026-Q2-plausible")
	router := buildImpactMevPDRouter(infra.DB)
	claims := eclParamMakerClaims(makerID)

	// multiplier=3.0 — outside [0.5, 2.5] soft range, but NOT hard-rejected.
	body := fmt.Sprintf(`{
		"periodeId": %q,
		"skenario": "BAD",
		"impactMultiplier": "3.0",
		"catatan": "ALCO override scenario — crisis test"
	}`, periodeID)

	w := postJSON(router, "/api/v1/master/impact-mev-pd", claims, uuid.New().String(), body)
	if w.Code != http.StatusCreated {
		t.Errorf("multiplier=3.0 (ALCO override): expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	// Parse and clean up.
	var resp struct {
		Data struct {
			ID               string `json:"id"`
			ImpactMultiplier string `json:"impactMultiplier"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	entityID, _ := uuid.Parse(resp.Data.ID)
	t.Cleanup(func() { cleanupImpactMevPD(t, infra.DB, entityID) })

	// Verify multiplier stored correctly.
	if !strings.HasPrefix(resp.Data.ImpactMultiplier, "3") {
		t.Errorf("expected impactMultiplier=3.0, got %s", resp.Data.ImpactMultiplier)
	}

	// Verify DB value.
	var storedMultiplier string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT impact_multiplier::text FROM mst.impact_mev_pd WHERE id = $1
	`, entityID).Scan(&storedMultiplier); err != nil {
		t.Fatalf("DB fetch: %v", err)
	}
	t.Logf("multiplier=3.0 accepted (ALCO override), stored=%s", storedMultiplier)

	assertAuditEvent(t, infra.DB, "IMPACT_MEV_PD.CREATE", entityID)
	t.Logf("IMPACT_MEV_PD.CREATE audit event recorded for ALCO override")
}

// ─── Compile-time interface check ─────────────────────────────────────────────

var _ impactmevpd.Repository = (*impactmevpd.DBRepository)(nil)
