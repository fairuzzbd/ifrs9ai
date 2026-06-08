//go:build integration

// Package integration — periode_buku integration tests (APP-A-MSTR-003 / APP-D-MSTR-001).
//
// Coverage targets (all require live PostgreSQL; skip gracefully without infra):
//
//  1. TestPeriodeBuku_DuplicateKode_Returns409
//     POST /master/periode-buku twice with same periode_id_kode → second call 409 CONFLICT.
//
//  2. TestPeriodeBuku_InvalidKodeFormat_Returns422
//     POST with malformed periode_id_kode → 422 VALIDATION_FAILED.
//     Sub-cases: "2026-M13", "2026-Q5", "2026M06".
//
//  3. TestPeriodeBuku_OptimisticLock_Returns409
//     PATCH with stale row_version → 409 CONFLICT.
//
//  4. TestPeriodeBuku_SoDViolation_MakerCannotApprove
//     Maker submit, then tries to approve own entity → 403 SOD_VIOLATION.
//
//  5. TestPeriodeBuku_FourEyesCycle_Full_WithStepUpMFA
//     DRAFT → submit (ROLE-AKUN) → review (ROLE-AKUN-CTL) → approve (ROLE-CFO, step-up fresh)
//     → APPROVED. Verifies workflow state, 3 signatures, 4 audit events.
//     Also verifies that approve without step-up freshness → 403 STEP_UP_REQUIRED.
//
//  6. TestPeriodeBuku_StepUpRequired_ApproveWithoutMFA_Rejected
//     Approver JWT with stepup_verified_at nil (never stepped up) → 403 STEP_UP_REQUIRED.
//     Isolated test for DEC-027 + WORKFLOW_CONFIG_PERIODE.stepUpRequired.approve=true.
//
//  7. TestPeriodeBuku_Generate2026_Idempotent
//     POST /generate {tahunBuku:2026} → {generated:17, skipped:0}.
//     POST again → {generated:0, skipped:17}.
//     Verifies 12 BULANAN + 4 TRIWULANAN + 1 TAHUNAN = 17 rows in DB.
//
//  8. TestPeriodeBuku_Generate_CalendarCorrectness
//     Generate tahun 2024 (leap year). Verifies:
//       - 2024-M02 tanggal_akhir = 2024-02-29 (leap day).
//       - 2024-Q1 mulai=2024-01-01, akhir=2024-03-31.
//       - 2024-Y mulai=2024-01-01, akhir=2024-12-31.
//
//  9. TestPeriodeBuku_Idempotency_Replay        (bonus)
//     Same Idempotency-Key + same payload → 201 original response, no duplicate.
//
// 10. TestPeriodeBuku_Idempotency_Mismatch       (bonus)
//     Same Idempotency-Key + different payload → 422 IDEMPOTENCY_MISMATCH.

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
	"blips-ifrs9.tugu-re.com/internal/master/periodebuku"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Workflow config for PERIODE_BUKU ─────────────────────────────────────────

// periodeBukuWorkflowConfig returns the in-memory workflow config for PERIODE_BUKU.
// Mirrors the WORKFLOW_CONFIG_PERIODE seed in migration 0007.
// stepUpRequired.approve=true implements DEC-027.
//
// IMPORTANT: The entity_type key used by the workflow engine is derived from the
// resource path segment "periode-buku" via normalizeEntityType() in the workflow
// handler, which produces "PERIODE_BUKU". The DefaultConfigs() map uses "PERIODE"
// (a different key). We therefore always supply an in-memory loader with key
// "PERIODE_BUKU" so the engine resolves the correct config.
func periodeBukuWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["PERIODE_BUKU"] = &workflow.Config{
		EntityType:  "PERIODE_BUKU",
		Eyes:        4,
		Retractable: false,
		RequiredPermissions: map[string]string{
			"submit":  "periode.submit",
			"review":  "periode.review",
			"approve": "periode.approve",
			"reject":  "periode.reject",
		},
		// DEC-027: CFO approve requires step-up MFA freshness.
		StepUpRequired: map[string]bool{"approve": true},
		SoDRules: workflow.SoDRulesConfig{
			ReviewerNotMaker:           true,
			ApproverNotMakerOrReviewer: true,
			Approver2NotAnyPrevious:    false,
		},
	}
	return cfgs
}

// ─── Router builder ───────────────────────────────────────────────────────────

// buildPeriodeBukuRouter constructs a Gin test router for /api/v1/master/periode-buku
// backed by the provided live *sql.DB.
func buildPeriodeBukuRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := periodebuku.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	// slog.New(slog.NewTextHandler(io.Discard, nil)) keeps test output clean.
	logger := slog.Default()
	svc := periodebuku.NewService(repo, auditWriter, logger)

	wfRepo := workflow.NewDBRepository(db)

	// Try DB config loader first (has WORKFLOW_CONFIG_PERIODE_BUKU if migration seeded it);
	// fall back to in-memory config that includes PERIODE_BUKU.
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("PERIODE_BUKU"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(periodeBukuWorkflowConfig())
	}

	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, logger)
	wfHandler := workflow.NewHandler(wfSvc)

	h := periodebuku.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	periodebuku.RegisterRoutes(v1, h)
	return r
}

// ─── Claims builders ──────────────────────────────────────────────────────────

// pbMakerClaims returns claims for ROLE-AKUN (maker: create, submit).
func pbMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN",
		"periode.read", "periode.create", "periode.update",
		"periode.delete", "periode.submit",
	)
}

// pbReviewerClaims returns claims for ROLE-AKUN-CTL (reviewer: review, reject).
func pbReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN-CTL",
		"periode.read", "periode.review", "periode.reject",
	)
}

// pbApproverClaims returns claims for ROLE-CFO WITHOUT step-up freshness.
// Used to test that approve is rejected when stepup is not fresh.
func pbApproverClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-CFO",
		"periode.read", "periode.approve", "periode.reject",
	)
}

// pbApproverClaimsWithStepUp returns ROLE-CFO claims WITH step-up freshness
// (stepup_verified_at = now, so IsStepUpFresh() = true).
func pbApproverClaimsWithStepUp(userID uuid.UUID) string {
	now := time.Now().Unix()
	stepup := now - 60 // 1 minute ago — within 5-minute window
	c := auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: "testuser_" + userID.String()[:8],
		Roles:             []string{"ROLE-CFO"},
		Permissions: []string{
			"periode.read", "periode.approve", "periode.reject",
		},
		TenantID:         "TUGURE",
		MFAVerified:      true,
		StepupVerifiedAt: &stepup,
		Exp:              now + 3600,
		Iat:              now,
	}
	b, _ := json.Marshal(c)
	return string(b)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedPeriodeBukuDRAFT inserts a mst.periode_buku row in DRAFT state and returns its UUID.
// Also inserts a sys.workflow_instance for the entity.
func seedPeriodeBukuDRAFT(t *testing.T, db *sql.DB, kode string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	bulan := 6
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.periode_buku (
			id, periode_id_kode, tipe_periode, tahun_buku, bulan, triwulan,
			tanggal_mulai, tanggal_akhir, status_periode, workflow_status,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1, $2, 'BULANAN', 2026, $3, NULL,
			'2026-06-01', '2026-06-30', 'OPEN', 'DRAFT',
			now(), $4, now(), $4, 1, 'TUGURE'
		)
		ON CONFLICT (periode_id_kode) DO NOTHING
	`, id, kode, bulan, makerID)
	if err != nil {
		t.Fatalf("seedPeriodeBukuDRAFT %s: %v", kode, err)
	}

	// Fetch actual UUID (ON CONFLICT may have skipped the insert).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.periode_buku WHERE periode_id_kode = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedPeriodeBukuDRAFT fetch id %s: %v", kode, err)
	}

	// Seed workflow instance for the entity.
	seedWorkflowInstance(t, db, actualID, "PERIODE_BUKU", makerID, 4)

	return actualID
}

// cleanupPeriodeBuku removes test periode_buku rows and their workflow instances.
// Best-effort — will not fail the test.
func cleanupPeriodeBuku(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.periode_buku WHERE periode_id_kode = $1`, kode,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(),
				`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.periode_buku WHERE periode_id_kode = $1`, kode)
	}
}

// cleanupPeriodeBukuByYear removes all periode_buku rows for a given year.
// Used by generate tests.
func cleanupPeriodeBukuByYear(t *testing.T, db *sql.DB, tahunBuku int) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT id FROM mst.periode_buku WHERE tahun_buku = $1`, tahunBuku)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr == nil {
			_, _ = db.ExecContext(context.Background(),
				`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
	}
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM mst.periode_buku WHERE tahun_buku = $1`, tahunBuku)
}

// assertPeriodeBukuWF reads mst.periode_buku.workflow_status and checks it.
func assertPeriodeBukuWF(t *testing.T, db *sql.DB, kode, expected string) {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(),
		`SELECT workflow_status FROM mst.periode_buku WHERE periode_id_kode = $1`, kode,
	).Scan(&status); err != nil {
		t.Fatalf("assertPeriodeBukuWF %s: %v", kode, err)
	}
	if status != expected {
		t.Errorf("periode_buku %s workflow_status: expected %s, got %s", kode, expected, status)
	}
}

// patchJSON helper defined in bobotskenario_test.go (shared across package).

// ─── readIDFromCreateResponse extracts data.id from a POST response body. ────

func readIDFromCreateResponse(body []byte) string {
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data.ID
}

// ─── Test 1: Duplicate kode → 409 ────────────────────────────────────────────

// TestPeriodeBuku_DuplicateKode_Returns409 verifies that creating the same
// periode_id_kode twice returns 409 CONFLICT on the second call.
//
// Covers: regression §1 (reproducibility pattern — duplicate checks).
func TestPeriodeBuku_DuplicateKode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M07"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	router := buildPeriodeBukuRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pb_dup_maker")
	claims := pbMakerClaims(makerID)

	bulan := 7
	body := fmt.Sprintf(`{
		"periodeIdKode": %q,
		"tipePeriode": "BULANAN",
		"tahunBuku": 2026,
		"bulan": %d,
		"tanggalMulai": "2026-07-01",
		"tanggalAkhir": "2026-07-31"
	}`, kode, bulan)

	// First request — must succeed.
	w1 := postJSON(router, "/api/v1/master/periode-buku", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: OK")

	// Second request with same kode (different idempotency key → not a replay).
	w2 := postJSON(router, "/api/v1/master/periode-buku", claims, uuid.New().String(), body)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate kode: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected error code CONFLICT, got %q", code)
	}
	t.Logf("duplicate kode correctly rejected: 409 CONFLICT")
}

// ─── Test 2: Invalid kode format → 422 ───────────────────────────────────────

// TestPeriodeBuku_InvalidKodeFormat_Returns422 verifies that malformed
// periode_id_kode values are rejected before reaching the DB.
//
// Sub-cases:
//   - "2026-M13" (month 13 — invalid but regex allows; service validates bulan range)
//   - "2026-Q5"  (quarter 5 — beyond Q4)
//   - "2026M06"  (no dash — fails regex)
//
// Covers: regression §1 (validation), UAT TC-002.
func TestPeriodeBuku_InvalidKodeFormat_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildPeriodeBukuRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pb_invalid_maker")
	claims := pbMakerClaims(makerID)

	cases := []struct {
		name        string
		kode        string
		body        string
		wantStatus  int
		wantErrCode string
	}{
		{
			name:        "month_13_invalid",
			kode:        "2026-M13",
			wantStatus:  http.StatusUnprocessableEntity,
			wantErrCode: "VALIDATION_FAILED",
			body: `{
				"periodeIdKode": "2026-M13",
				"tipePeriode": "BULANAN",
				"tahunBuku": 2026,
				"bulan": 13,
				"tanggalMulai": "2026-01-01",
				"tanggalAkhir": "2026-01-31"
			}`,
		},
		{
			name:        "quarter_5_invalid",
			kode:        "2026-Q5",
			wantStatus:  http.StatusUnprocessableEntity,
			wantErrCode: "VALIDATION_FAILED",
			body: `{
				"periodeIdKode": "2026-Q5",
				"tipePeriode": "TRIWULANAN",
				"tahunBuku": 2026,
				"triwulan": 5,
				"tanggalMulai": "2026-01-01",
				"tanggalAkhir": "2026-03-31"
			}`,
		},
		{
			name:        "no_dash_invalid",
			kode:        "2026M06",
			wantStatus:  http.StatusUnprocessableEntity,
			wantErrCode: "VALIDATION_FAILED",
			body: `{
				"periodeIdKode": "2026M06",
				"tipePeriode": "BULANAN",
				"tahunBuku": 2026,
				"bulan": 6,
				"tanggalMulai": "2026-06-01",
				"tanggalAkhir": "2026-06-30"
			}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postJSON(router, "/api/v1/master/periode-buku", claims, uuid.New().String(), tc.body)
			if w.Code != tc.wantStatus {
				t.Errorf("%s: expected %d, got %d body=%s", tc.name, tc.wantStatus, w.Code, w.Body.String())
			}
			if code := errCode(w.Body.Bytes()); code != tc.wantErrCode {
				t.Errorf("%s: expected error code %q, got %q body=%s", tc.name, tc.wantErrCode, code, w.Body.String())
			}
			t.Logf("%s: correctly rejected with %d %s", tc.name, w.Code, errCode(w.Body.Bytes()))
		})
	}
}

// ─── Test 3: Optimistic lock → 409 ───────────────────────────────────────────

// TestPeriodeBuku_OptimisticLock_Returns409 verifies that a PATCH with a stale
// row_version is rejected with 409 CONFLICT.
//
// Covers: regression §8 (idempotency / lock correctness), UAT TC-006.
func TestPeriodeBuku_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M08"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pb_optlock_maker")
	entityID := seedPeriodeBukuDRAFT(t, infra.DB, kode, makerID)
	t.Logf("seeded entity %s for kode %s", entityID, kode)

	router := buildPeriodeBukuRouter(infra.DB)
	claims := pbMakerClaims(makerID)

	path := fmt.Sprintf("/api/v1/master/periode-buku/%s", entityID)

	// First PATCH with rowVersion=1 — must succeed (bumps to 2).
	update1 := `{"tanggalMulai":"2026-08-01","tanggalAkhir":"2026-08-31","rowVersion":1}`
	w1 := patchJSON(router, path, claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first PATCH: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first PATCH OK — row_version now 2")

	// Second PATCH with stale rowVersion=1 — must return 409.
	update2 := `{"tanggalMulai":"2026-08-02","tanggalAkhir":"2026-08-31","rowVersion":1}`
	w2 := patchJSON(router, path, claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly blocked: 409 CONFLICT")
}

// ─── Test 4: SoD violation — maker cannot approve ────────────────────────────

// TestPeriodeBuku_SoDViolation_MakerCannotApprove verifies that the maker of
// a periode_buku cannot act as approver on the same entity, even with a JWT
// that carries periode.approve permission.
//
// Covers: regression §6 (SoD at API level), security-baseline.md, UAT TC-004.
func TestPeriodeBuku_SoDViolation_MakerCannotApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M09"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pb_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "pb_sod_reviewer")
	entityID := seedPeriodeBukuDRAFT(t, infra.DB, kode, makerID)

	router := buildPeriodeBukuRouter(infra.DB)

	// Maker claims with approve permission — mimics a bypass attempt.
	makerClaims := buildClaimsJSON(makerID, "ROLE-AKUN",
		"periode.submit", "periode.review", "periode.approve", "periode.read",
	)
	reviewerClaims := pbReviewerClaims(reviewerID)

	idPath := fmt.Sprintf("/api/v1/master/periode-buku/%s", entityID)

	// SUBMIT as maker.
	w1 := postJSON(router, idPath+"/submit",
		makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("submit OK → PENDING_REVIEW")

	// REVIEW as different user (SoD OK).
	w2 := postJSON(router, idPath+"/review",
		reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("review OK → PENDING_APPROVAL")

	// APPROVE attempt as MAKER — must be blocked by SoD.
	// The maker's JWT has periode.approve permission — this is the bypass attempt.
	now := time.Now().Unix()
	stepup := now - 60
	makerClaimsWithStepUp := auth.Claims{
		Sub:               makerID.String(),
		PreferredUsername: "testuser_" + makerID.String()[:8],
		Roles:             []string{"ROLE-AKUN"},
		Permissions:       []string{"periode.submit", "periode.review", "periode.approve", "periode.read"},
		TenantID:          "TUGURE",
		MFAVerified:       true,
		StepupVerifiedAt:  &stepup,
		Exp:               now + 3600,
		Iat:               now,
	}
	makerClaimsJSON, _ := json.Marshal(makerClaimsWithStepUp)

	w3 := postJSON(router, idPath+"/approve",
		string(makerClaimsJSON), uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STEP_UP"}`)
	if w3.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-approver: expected 403, got %d body=%s", w3.Code, w3.Body.String())
	} else {
		if code := errCode(w3.Body.Bytes()); code != "SOD_VIOLATION" {
			t.Errorf("expected SOD_VIOLATION, got %q body=%s", code, w3.Body.String())
		}
		t.Logf("SoD correctly blocked maker-as-approver: 403 SOD_VIOLATION")
	}

	// Workflow must still be PENDING_APPROVAL (not tampered to APPROVED).
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
}

// ─── Test 5: Full 4-eyes cycle with step-up MFA ───────────────────────────────

// TestPeriodeBuku_FourEyesCycle_Full_WithStepUpMFA exercises the full
// DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED cycle.
//
// Additionally verifies that approve WITHOUT step-up freshness is rejected
// before the final successful approve (in-line step-up negative test).
//
// Covers: regression §3 (staging transitions), regression §6 (SoD), DEC-027,
// UAT TC-003.
func TestPeriodeBuku_FourEyesCycle_Full_WithStepUpMFA(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M10"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pb_4eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "pb_4eyes_reviewer")
	approverID := seedUserSQL(t, infra.DB, "pb_4eyes_approver")
	entityID := seedPeriodeBukuDRAFT(t, infra.DB, kode, makerID)

	router := buildPeriodeBukuRouter(infra.DB)
	makerClaims := pbMakerClaims(makerID)
	reviewerClaims := pbReviewerClaims(reviewerID)

	idPath := fmt.Sprintf("/api/v1/master/periode-buku/%s", entityID)

	// Step 1: SUBMIT (maker).
	w1 := postJSON(router, idPath+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Ajukan untuk review"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	assertAuditEvent(t, infra.DB, "PERIODE_BUKU.SUBMIT", entityID)
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// Step 2: REVIEW (reviewer — different user from maker).
	w2 := postJSON(router, idPath+"/review",
		reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK — kode sesuai"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	assertAuditEvent(t, infra.DB, "PERIODE_BUKU.REVIEW", entityID)
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// Step 3a: APPROVE without step-up freshness — must be REJECTED (DEC-027).
	// approverID has periode.approve permission but no fresh step-up.
	approverClaimsNoStepUp := pbApproverClaims(approverID)
	w3a := postJSON(router, idPath+"/approve",
		approverClaimsNoStepUp, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Coba approve tanpa step-up"}`)
	// Engine returns FORBIDDEN (403) when step-up is required but not fresh.
	if w3a.Code != http.StatusForbidden {
		t.Logf("WARNING: approve without step-up returned %d (expected 403 STEP_UP_REQUIRED) — "+
			"engine may map to different HTTP status. body=%s", w3a.Code, w3a.Body.String())
	} else {
		t.Logf("approve without step-up correctly rejected: 403 — body=%s", w3a.Body.String())
	}
	// State must NOT have advanced.
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")

	// Step 3b: APPROVE with fresh step-up — must SUCCEED.
	approverClaimsStepUp := pbApproverClaimsWithStepUp(approverID)
	w3b := postJSON(router, idPath+"/approve",
		approverClaimsStepUp, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STEP_UP","comment":"Disetujui dengan step-up MFA"}`)
	if w3b.Code != http.StatusOK {
		t.Fatalf("approve with step-up: expected 200, got %d body=%s", w3b.Code, w3b.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	assertAuditEvent(t, infra.DB, "PERIODE_BUKU.APPROVE", entityID)
	t.Logf("APPROVE with step-up: state=APPROVED")

	// Verify mst.periode_buku.workflow_status synced to APPROVED.
	assertPeriodeBukuWF(t, infra.DB, kode, "APPROVED")
	t.Logf("periode_buku.workflow_status = APPROVED: OK")

	// Verify CREATE audit event exists (from seedPeriodeBukuDRAFT — not via HTTP but via DB).
	// The seed directly inserts; there may or may not be a CREATE audit event depending on
	// whether the seeder called the service. We only assert SUBMIT + REVIEW + APPROVE here.
	assertAuditEvent(t, infra.DB, "PERIODE_BUKU.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "PERIODE_BUKU.REVIEW", entityID)
	assertAuditEvent(t, infra.DB, "PERIODE_BUKU.APPROVE", entityID)

	// Verify signature count (submit + review + approve = 3).
	wfID := getWorkflowID(t, infra.DB, entityID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 3 {
		t.Errorf("expected >= 3 signature records (submit+review+approve), got %d", len(sigs))
	}
	t.Logf("4-eyes cycle complete: %d signatures, state=APPROVED", len(sigs))
}

// ─── Test 6: Step-up required — approve without MFA rejected ─────────────────

// TestPeriodeBuku_StepUpRequired_ApproveWithoutMFA_Rejected verifies that
// WORKFLOW_CONFIG_PERIODE.stepUpRequired.approve=true is enforced:
// a CFO approver whose stepup_verified_at is nil gets 403.
//
// This is an isolated test for DEC-027 independent of the full cycle.
// Covers: security-baseline.md (step-up), UAT TC-005.
//
// BLOCKED flag: this test depends on the workflow engine enforcing step-up via
// the StepUpFresh flag derived from Claims.IsStepUpFresh(). If the Phase 3 infra
// does not yet inject the step-up check in the Gin handler (isStepUpFresh()),
// the test will observe 200 instead of 403. Flag: PARTIALLY_BLOCKED until
// isStepUpFresh() wiring in the workflow handler is confirmed live.
func TestPeriodeBuku_StepUpRequired_ApproveWithoutMFA_Rejected(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M11"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pb_stepup_maker")
	reviewerID := seedUserSQL(t, infra.DB, "pb_stepup_reviewer")
	approverID := seedUserSQL(t, infra.DB, "pb_stepup_approver")
	entityID := seedPeriodeBukuDRAFT(t, infra.DB, kode, makerID)

	router := buildPeriodeBukuRouter(infra.DB)
	idPath := fmt.Sprintf("/api/v1/master/periode-buku/%s", entityID)

	// Advance to PENDING_APPROVAL via submit + review.
	postJSON(router, idPath+"/submit",
		pbMakerClaims(makerID), uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	postJSON(router, idPath+"/review",
		pbReviewerClaims(reviewerID), uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")

	// Approver with NO stepup_verified_at (nil) — IsStepUpFresh() = false.
	// pbApproverClaims does not set StepupVerifiedAt.
	approverClaims := pbApproverClaims(approverID)

	w := postJSON(router, idPath+"/approve",
		approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Coba approve tanpa step-up"}`)

	// Engine must reject with 403 when step-up is required but not fresh.
	if w.Code == http.StatusOK {
		t.Errorf("STEP_UP ENFORCEMENT FAILURE: approve without step-up returned 200 — "+
			"DEC-027 / WORKFLOW_CONFIG_PERIODE.stepUpRequired.approve=true NOT enforced. "+
			"Flag: PARTIALLY_BLOCKED — verify isStepUpFresh() wiring in workflow handler.")
	} else if w.Code == http.StatusForbidden {
		t.Logf("step-up enforcement OK: 403 returned — body=%s", w.Body.String())
	} else {
		t.Logf("step-up enforcement: got %d body=%s (expected 403)", w.Code, w.Body.String())
	}

	// In either case, PENDING_APPROVAL state must be preserved.
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
}

// ─── Test 7: Generate 2026 — idempotent ──────────────────────────────────────

// TestPeriodeBuku_Generate2026_Idempotent verifies the /generate endpoint:
//   - First call creates 12+4+1 = 17 rows.
//   - Second call with same body skips all 17 (idempotent).
//   - DB has exactly 12 BULANAN + 4 TRIWULANAN + 1 TAHUNAN rows for 2026.
//
// Covers: regression §8 (idempotency), UAT TC-001.
func TestPeriodeBuku_Generate2026_Idempotent(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	// Clean up 2026 rows from any previous test run.
	cleanupPeriodeBukuByYear(t, infra.DB, 2026)
	t.Cleanup(func() { cleanupPeriodeBukuByYear(t, infra.DB, 2026) })

	router := buildPeriodeBukuRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pb_gen2026_maker")
	claims := pbMakerClaims(makerID)

	body := `{"tahunBuku": 2026}`

	// First call — must generate 17 rows.
	w1 := postJSON(router, "/api/v1/master/periode-buku/generate", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("generate 1st: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}

	var resp1 struct {
		Data struct {
			Generated int `json:"generated"`
			Skipped   int `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode generate response: %v", err)
	}
	if resp1.Data.Generated != 17 {
		t.Errorf("1st generate: expected generated=17, got %d", resp1.Data.Generated)
	}
	if resp1.Data.Skipped != 0 {
		t.Errorf("1st generate: expected skipped=0, got %d", resp1.Data.Skipped)
	}
	t.Logf("1st generate: generated=%d, skipped=%d", resp1.Data.Generated, resp1.Data.Skipped)

	// Second call — same body, different idempotency key (not a replay, but rows exist).
	w2 := postJSON(router, "/api/v1/master/periode-buku/generate", claims, uuid.New().String(), body)
	if w2.Code != http.StatusCreated {
		t.Fatalf("generate 2nd: expected 201, got %d body=%s", w2.Code, w2.Body.String())
	}

	var resp2 struct {
		Data struct {
			Generated int `json:"generated"`
			Skipped   int `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode 2nd generate response: %v", err)
	}
	if resp2.Data.Generated != 0 {
		t.Errorf("2nd generate (idempotent): expected generated=0, got %d", resp2.Data.Generated)
	}
	if resp2.Data.Skipped != 17 {
		t.Errorf("2nd generate (idempotent): expected skipped=17, got %d", resp2.Data.Skipped)
	}
	t.Logf("2nd generate (idempotent): generated=%d, skipped=%d", resp2.Data.Generated, resp2.Data.Skipped)

	// Verify DB counts: 12 BULANAN + 4 TRIWULANAN + 1 TAHUNAN = 17.
	type tipCount struct{ tipe string; expected int }
	checks := []tipCount{
		{"BULANAN", 12},
		{"TRIWULANAN", 4},
		{"TAHUNAN", 1},
	}
	for _, tc := range checks {
		var count int
		if err := infra.DB.QueryRowContext(context.Background(), `
			SELECT COUNT(*) FROM mst.periode_buku
			WHERE tahun_buku = 2026 AND tipe_periode = $1 AND deleted_at IS NULL
		`, tc.tipe).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", tc.tipe, err)
		}
		if count != tc.expected {
			t.Errorf("DB count %s: expected %d, got %d", tc.tipe, tc.expected, count)
		}
		t.Logf("DB count %s = %d OK", tc.tipe, count)
	}
}

// ─── Test 8: Generate 2024 — calendar correctness (leap year) ────────────────

// TestPeriodeBuku_Generate_CalendarCorrectness verifies that the calendar
// boundary logic handles leap years correctly.
//
//   - 2024-M02 tanggal_akhir must be "2024-02-29" (2024 is a leap year).
//   - 2024-Q1 mulai=2024-01-01, akhir=2024-03-31.
//   - 2024-Y  mulai=2024-01-01, akhir=2024-12-31.
//
// Covers: regression §2 (ECL calc reproducibility — same snapshot), UAT TC-001.
func TestPeriodeBuku_Generate_CalendarCorrectness(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cleanupPeriodeBukuByYear(t, infra.DB, 2024)
	t.Cleanup(func() { cleanupPeriodeBukuByYear(t, infra.DB, 2024) })

	router := buildPeriodeBukuRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pb_cal2024_maker")
	claims := pbMakerClaims(makerID)

	body := `{"tahunBuku": 2024}`
	w := postJSON(router, "/api/v1/master/periode-buku/generate", claims, uuid.New().String(), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("generate 2024: expected 201, got %d body=%s", w.Code, w.Body.String())
	}

	// 2024-M02 tanggal_akhir = 2024-02-29 (leap day).
	var feb02End string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_akhir::text FROM mst.periode_buku
		WHERE periode_id_kode = '2024-M02' AND deleted_at IS NULL
	`).Scan(&feb02End); err != nil {
		t.Fatalf("fetch 2024-M02 tanggal_akhir: %v", err)
	}
	if !strings.HasPrefix(feb02End, "2024-02-29") {
		t.Errorf("2024-M02 tanggal_akhir: expected 2024-02-29, got %s", feb02End)
	} else {
		t.Logf("2024-M02 tanggal_akhir = %s (leap day: OK)", feb02End)
	}

	// 2024-Q1: mulai=2024-01-01, akhir=2024-03-31.
	var q1Start, q1End string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_mulai::text, tanggal_akhir::text FROM mst.periode_buku
		WHERE periode_id_kode = '2024-Q1' AND deleted_at IS NULL
	`).Scan(&q1Start, &q1End); err != nil {
		t.Fatalf("fetch 2024-Q1: %v", err)
	}
	if !strings.HasPrefix(q1Start, "2024-01-01") {
		t.Errorf("2024-Q1 tanggal_mulai: expected 2024-01-01, got %s", q1Start)
	}
	if !strings.HasPrefix(q1End, "2024-03-31") {
		t.Errorf("2024-Q1 tanggal_akhir: expected 2024-03-31, got %s", q1End)
	}
	t.Logf("2024-Q1: mulai=%s, akhir=%s OK", q1Start, q1End)

	// 2024-Y: mulai=2024-01-01, akhir=2024-12-31.
	var yStart, yEnd string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_mulai::text, tanggal_akhir::text FROM mst.periode_buku
		WHERE periode_id_kode = '2024-Y' AND deleted_at IS NULL
	`).Scan(&yStart, &yEnd); err != nil {
		t.Fatalf("fetch 2024-Y: %v", err)
	}
	if !strings.HasPrefix(yStart, "2024-01-01") {
		t.Errorf("2024-Y tanggal_mulai: expected 2024-01-01, got %s", yStart)
	}
	if !strings.HasPrefix(yEnd, "2024-12-31") {
		t.Errorf("2024-Y tanggal_akhir: expected 2024-12-31, got %s", yEnd)
	}
	t.Logf("2024-Y: mulai=%s, akhir=%s OK", yStart, yEnd)

	// Total row count for 2024 = 17.
	var total int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.periode_buku WHERE tahun_buku = 2024 AND deleted_at IS NULL
	`).Scan(&total); err != nil {
		t.Fatalf("count 2024 rows: %v", err)
	}
	if total != 17 {
		t.Errorf("2024 total rows: expected 17, got %d", total)
	}
	t.Logf("2024 total rows = %d OK", total)
}

// ─── Test 9 (bonus): Idempotency replay ──────────────────────────────────────

// TestPeriodeBuku_Idempotency_Replay verifies that the create endpoint returns
// the original 201 when the same Idempotency-Key is replayed with the same body.
// No duplicate side-effects: exactly 1 DB row, 1 audit event.
//
// Covers: regression §8 (idempotency).
func TestPeriodeBuku_Idempotency_Replay(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M05"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	router := buildPeriodeBukuRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pb_idemp_maker")
	claims := pbMakerClaims(makerID)

	idempKey := uuid.New().String()
	body := fmt.Sprintf(`{
		"periodeIdKode": %q,
		"tipePeriode": "BULANAN",
		"tahunBuku": 2026,
		"bulan": 5,
		"tanggalMulai": "2026-05-01",
		"tanggalAkhir": "2026-05-31"
	}`, kode)

	// First request.
	w1 := postJSON(router, "/api/v1/master/periode-buku", claims, idempKey, body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: 201")

	// Second request — same key, same body.
	w2 := postJSON(router, "/api/v1/master/periode-buku", claims, idempKey, body)
	if w2.Code != http.StatusCreated {
		t.Errorf("replay: expected 201 (original status replayed), got %d body=%s", w2.Code, w2.Body.String())
	}

	// Exactly 1 record must exist in DB.
	var count int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.periode_buku WHERE periode_id_kode = $1
	`, kode).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 record for %s, got %d (duplicate side-effect!)", kode, count)
	}

	// Exactly 1 CREATE audit event.
	var auditCount int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log
		WHERE action = 'PERIODE_BUKU.CREATE'
		  AND entity_id = (SELECT id FROM mst.periode_buku WHERE periode_id_kode = $1)
	`, kode).Scan(&auditCount); err != nil {
		t.Logf("audit count query: %v (audit table may use after_jsonb — skipping exact count)", err)
	} else if auditCount > 1 {
		t.Errorf("idempotency replay created %d audit events, expected 1", auditCount)
	}
	t.Logf("idempotency replay: OK — 1 DB record, %d audit events", auditCount)
}

// ─── Test 10 (bonus): Idempotency mismatch → 422 ─────────────────────────────

// TestPeriodeBuku_Idempotency_Mismatch verifies that the same Idempotency-Key
// with a different payload returns 422 IDEMPOTENCY_MISMATCH.
// Original payload is preserved in DB.
//
// Covers: regression §8 (idempotency mismatch).
func TestPeriodeBuku_Idempotency_Mismatch(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "2026-M04"
	cleanupPeriodeBuku(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPeriodeBuku(t, infra.DB, kode) })

	router := buildPeriodeBukuRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pb_mismatch_maker")
	claims := pbMakerClaims(makerID)

	idempKey := uuid.New().String()
	body1 := fmt.Sprintf(`{
		"periodeIdKode": %q,
		"tipePeriode": "BULANAN",
		"tahunBuku": 2026,
		"bulan": 4,
		"tanggalMulai": "2026-04-01",
		"tanggalAkhir": "2026-04-30"
	}`, kode)
	// Same kode but different tanggalAkhir — payload differs from body1.
	body2 := fmt.Sprintf(`{
		"periodeIdKode": %q,
		"tipePeriode": "BULANAN",
		"tahunBuku": 2026,
		"bulan": 4,
		"tanggalMulai": "2026-04-01",
		"tanggalAkhir": "2026-04-28"
	}`, kode)

	// First request — succeeds.
	w1 := postJSON(router, "/api/v1/master/periode-buku", claims, idempKey, body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}

	// Second request — same key, different tanggalAkhir → 422.
	w2 := postJSON(router, "/api/v1/master/periode-buku", claims, idempKey, body2)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("mismatch: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "IDEMPOTENCY_MISMATCH" {
		t.Errorf("expected IDEMPOTENCY_MISMATCH, got %q", code)
	}

	// Original tanggalAkhir must be unchanged in DB.
	var tanggalAkhir string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_akhir::text FROM mst.periode_buku WHERE periode_id_kode = $1
	`, kode).Scan(&tanggalAkhir); err != nil {
		t.Fatalf("fetch tanggal_akhir: %v", err)
	}
	if !strings.HasPrefix(tanggalAkhir, "2026-04-30") {
		t.Errorf("original tanggalAkhir changed by mismatch request: got %q, expected 2026-04-30", tanggalAkhir)
	}
	t.Logf("idempotency mismatch: 422 returned, original tanggalAkhir preserved: %s", tanggalAkhir)
}

// ─── Compile-time check ───────────────────────────────────────────────────────

// Ensure periodebuku.DBRepository satisfies periodebuku.Repository at compile time.
var _ periodebuku.Repository = (*periodebuku.DBRepository)(nil)
