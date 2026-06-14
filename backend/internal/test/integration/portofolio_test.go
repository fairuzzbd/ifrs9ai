//go:build integration

// Package integration — portofolio integration tests (APP-A-MSTR-012).
//
// Coverage:
//
//  1. TestPortofolio_DuplicateKode_Returns409
//  2. TestPortofolio_InvalidKodeFormat_Returns422
//  3. TestPortofolio_InvalidBMCategory_Returns422
//  4. TestPortofolio_FourEyesCycle_Full
//  5. TestPortofolio_SoDViolation_MakerCannotApprove
//  6. TestPortofolio_OptimisticLock_Returns409

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
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/portofolio"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Workflow config ──────────────────────────────────────────────────────────

func portofolioWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["PORTOFOLIO"] = &workflow.Config{
		EntityType:  "PORTOFOLIO",
		Eyes:        4,
		Retractable: false,
		RequiredPermissions: map[string]string{
			"submit":  "portofolio.submit",
			"review":  "portofolio.review",
			"approve": "portofolio.approve",
			"reject":  "portofolio.reject",
		},
		StepUpRequired: map[string]bool{"approve": false},
		SoDRules: workflow.SoDRulesConfig{
			ReviewerNotMaker:           true,
			ApproverNotMakerOrReviewer: true,
			Approver2NotAnyPrevious:    false,
		},
	}
	return cfgs
}

// ─── Router builder ───────────────────────────────────────────────────────────

// buildPortofolioRouter constructs the full Gin router for /api/v1/master/portofolio
// backed by the provided live *sql.DB.
func buildPortofolioRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := portofolio.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := portofolio.NewService(repo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("PORTOFOLIO"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(portofolioWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	h := portofolio.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	portofolio.RegisterRoutes(v1, h)
	return r
}

// ─── Claim builders ───────────────────────────────────────────────────────────

func portofolioMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-MAKER-TR",
		"portofolio.create", "portofolio.read", "portofolio.update",
		"portofolio.delete", "portofolio.submit",
	)
}

func portofolioReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-RISK",
		"portofolio.read", "portofolio.review",
	)
}

func portofolioApproverClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-APPR-TR",
		"portofolio.read", "portofolio.review", "portofolio.approve", "portofolio.reject",
	)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedPortofolioDRAFT inserts a portofolio row in DRAFT state and a matching
// workflow_instance. Returns the entity UUID.
func seedPortofolioDRAFT(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.portofolio (
			id, kode_portofolio, nama, bm_category_default, aktif_flag,
			workflow_status,
			created_at, created_by, updated_at, updated_by,
			version, tenant_id, is_deleted
		) VALUES (
			$1, $2, $3, 'HTC', true,
			'DRAFT',
			now(), $4, now(), $4,
			1, 'TUGURE', false
		)
		ON CONFLICT (kode_portofolio) DO NOTHING
	`, id, kode, nama, makerID)
	if err != nil {
		t.Fatalf("seedPortofolioDRAFT %s: %v", kode, err)
	}

	// Fetch actual UUID (ON CONFLICT may have skipped the insert).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.portofolio WHERE kode_portofolio = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedPortofolioDRAFT fetch id %s: %v", kode, err)
	}

	// Seed workflow instance.
	seedWorkflowInstance(t, db, actualID, "PORTOFOLIO", makerID, 4)

	// Update mst.portofolio.workflow_instance_id back-reference if column exists.
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, actualID).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.portofolio SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, actualID)
	}

	return actualID
}

// cleanupPortofolio removes test data for the given kode(s). Best-effort.
func cleanupPortofolio(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.portofolio WHERE kode_portofolio = $1`, kode,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(), `
				DELETE FROM sys.workflow_instance WHERE entity_id = $1
			`, id)
		}
		_, _ = db.ExecContext(context.Background(), `
			DELETE FROM mst.portofolio WHERE kode_portofolio = $1
		`, kode)
	}
}

// postPortofolioCreate sends a POST /api/v1/master/portofolio request.
func postPortofolioCreate(router *gin.Engine, claimsJSON, idempKey, body string) *httptest.ResponseRecorder {
	return postJSON(router, "/api/v1/master/portofolio", claimsJSON, idempKey, body)
}

// portofolioBody builds a minimal valid create request body.
func portofolioBody(kode, nama, bmCategory string) string {
	return fmt.Sprintf(`{
		"kodePortofolio": %q,
		"nama": %q,
		"bmCategoryDefault": %q,
		"aktifFlag": true
	}`, kode, nama, bmCategory)
}

// ─── Test 1: Duplicate kode → 409 ────────────────────────────────────────────

// TestPortofolio_DuplicateKode_Returns409 verifies that creating the same
// kode_portofolio twice returns 409 PORTOFOLIO_DUPLICATE_KODE on the second call.
//
// Regression: §1 SPPI/BM klasifikasi → AP/FVOCI/FVTPL; duplicate key guard.
func TestPortofolio_DuplicateKode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZPQ_DUP"
	cleanupPortofolio(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPortofolio(t, infra.DB, kode) })

	router := buildPortofolioRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pt_dup_maker")
	claims := portofolioMakerClaims(makerID)
	body := portofolioBody(kode, "Portofolio Duplikat Test", "HTC")

	// First request — must succeed.
	w1 := postPortofolioCreate(router, claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: OK")

	// Second request — different idempotency key, same kode.
	w2 := postPortofolioCreate(router, claims, uuid.New().String(), body)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate kode: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	code := errCode(w2.Body.Bytes())
	if code != "PORTOFOLIO_DUPLICATE_KODE" && code != "CONFLICT" {
		t.Errorf("expected PORTOFOLIO_DUPLICATE_KODE or CONFLICT error code, got %q", code)
	}
	t.Logf("duplicate kode correctly rejected: 409 code=%s", code)

	// Confirm exactly 1 row in DB.
	var count int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.portofolio WHERE kode_portofolio = $1 AND deleted_at IS NULL
	`, kode).Scan(&count); err != nil {
		t.Fatalf("DB count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d — duplicate may have been inserted", count)
	}
}

// ─── Test 2: Invalid kode format → 422 ───────────────────────────────────────

// TestPortofolio_InvalidKodeFormat_Returns422 verifies that kode_portofolio that
// fails the ^[A-Z0-9_]{1,20}$ regex is rejected at 400/422 with VALIDATION_FAILED
// or PORTOFOLIO_INVALID_KODE_FORMAT.
//
// Regression: §1 klasifikasi master data — invalid key prevents ghost records.
func TestPortofolio_InvalidKodeFormat_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildPortofolioRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pt_kode_maker")
	claims := portofolioMakerClaims(makerID)

	invalidKodes := []struct {
		kode   string
		reason string
	}{
		{"lower-case", "lowercase letters not allowed"},
		{"has space", "spaces not allowed"},
		{"TOOLONGKODEXYZ_12345!", "exceeds 20 chars and has special char"},
		{"", "empty string"},
		{"abc", "lowercase"},
	}

	for _, tc := range invalidKodes {
		body := fmt.Sprintf(`{
			"kodePortofolio": %q,
			"nama": "Test Portofolio",
			"bmCategoryDefault": "HTC"
		}`, tc.kode)

		w := postPortofolioCreate(router, claims, uuid.New().String(), body)
		// Expect 400 (validation) or 422 (unprocessable) — both mean invalid input.
		if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
			t.Errorf("kode=%q (%s): expected 400 or 422, got %d body=%s",
				tc.kode, tc.reason, w.Code, w.Body.String())
		}
		code := errCode(w.Body.Bytes())
		if code != "VALIDATION_FAILED" && code != "PORTOFOLIO_INVALID_KODE_FORMAT" {
			t.Errorf("kode=%q: expected VALIDATION_FAILED or PORTOFOLIO_INVALID_KODE_FORMAT, got %q",
				tc.kode, code)
		}
		t.Logf("invalid kode=%q correctly rejected: %d code=%s", tc.kode, w.Code, code)
	}
}

// ─── Test 3: Invalid BM Category → 422 ───────────────────────────────────────

// TestPortofolio_InvalidBMCategory_Returns422 verifies that bm_category_default
// values outside {HTC, HTCS, OTHER} are rejected.
//
// Regression: §1 SPPI×BM matrix — BM must be one of three valid values.
func TestPortofolio_InvalidBMCategory_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildPortofolioRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "pt_bm_maker")
	claims := portofolioMakerClaims(makerID)

	invalidCategories := []string{"HTC-X", "HOLD", "FVTPL", "HTD", "", "htc"}
	for _, bm := range invalidCategories {
		kode := "ZPQ_BM_TST"
		cleanupPortofolio(t, infra.DB, kode)

		body := fmt.Sprintf(`{
			"kodePortofolio": %q,
			"nama": "BM Category Test",
			"bmCategoryDefault": %q
		}`, kode, bm)

		w := postPortofolioCreate(router, claims, uuid.New().String(), body)
		if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
			t.Errorf("bm=%q: expected 400 or 422, got %d body=%s", bm, w.Code, w.Body.String())
		}
		code := errCode(w.Body.Bytes())
		if code != "VALIDATION_FAILED" && code != "PORTOFOLIO_INVALID_BM_CATEGORY" {
			t.Errorf("bm=%q: expected VALIDATION_FAILED or PORTOFOLIO_INVALID_BM_CATEGORY, got %q", bm, code)
		}
		t.Logf("invalid bm=%q correctly rejected: %d code=%s", bm, w.Code, code)
	}

	// Confirm no ghost rows.
	var count int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.portofolio WHERE kode_portofolio = 'ZPQ_BM_TST' AND deleted_at IS NULL
	`).Scan(&count); err != nil {
		t.Fatalf("DB count: %v", err)
	}
	if count > 0 {
		cleanupPortofolio(t, infra.DB, "ZPQ_BM_TST")
		t.Errorf("invalid BM category test left %d ghost row(s) in DB", count)
	}
}

// ─── Test 4: Full 4-eyes cycle ────────────────────────────────────────────────

// TestPortofolio_FourEyesCycle_Full exercises DRAFT → PENDING_REVIEW →
// PENDING_APPROVAL → APPROVED for portofolio. Verifies:
//   - workflow_instance state transitions at each step
//   - mst.portofolio.workflow_status synced to APPROVED
//   - audit_log events for SUBMIT and APPROVE written
//   - at least 2 signature records (review + approve)
//
// Regression: §3 staging transitions, §6 SoD, UAT S-004 (4-eyes).
func TestPortofolio_FourEyesCycle_Full(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZPQ_4EYES"
	cleanupPortofolio(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPortofolio(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pt_4eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "pt_4eyes_reviewer")
	approverID := seedUserSQL(t, infra.DB, "pt_4eyes_approver")
	entityID := seedPortofolioDRAFT(t, infra.DB, kode, "Portofolio Ekuitas 4-Eyes", makerID)

	router := buildPortofolioRouter(infra.DB)
	makerClaims := portofolioMakerClaims(makerID)
	reviewerClaims := portofolioReviewerClaims(reviewerID)
	approverClaims := portofolioApproverClaims(approverID)

	wfPath := "/api/v1/master/portofolio/" + kode

	// Step 1: SUBMIT (maker)
	w1 := postJSON(router, wfPath+"/submit", makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Ajukan untuk review"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// Step 2: REVIEW (reviewer — distinct from maker)
	w2 := postJSON(router, wfPath+"/review", reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review selesai, OK"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// Step 3: APPROVE (approver — distinct from maker and reviewer)
	w3 := postJSON(router, wfPath+"/approve", approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Disetujui"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	t.Logf("APPROVE: state=APPROVED")

	// Verify mst.portofolio.workflow_status synced.
	var wfStatus string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.portofolio WHERE kode_portofolio = $1
	`, kode).Scan(&wfStatus); err != nil {
		t.Fatalf("fetch portofolio workflow_status: %v", err)
	}
	if wfStatus != "APPROVED" {
		t.Errorf("mst.portofolio.workflow_status: expected APPROVED, got %s", wfStatus)
	}

	// Verify audit events.
	assertAuditEvent(t, infra.DB, "PORTOFOLIO.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "PORTOFOLIO.APPROVE", entityID)

	// Verify signature count >= 2 (review + approve; submit may or may not record sig).
	wfID := getWorkflowID(t, infra.DB, entityID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 2 {
		t.Errorf("expected >= 2 signature records, got %d", len(sigs))
	}
	t.Logf("4-eyes cycle complete: %d signatures, state=APPROVED", len(sigs))
}

// ─── Test 5: SoD violation — maker cannot approve ─────────────────────────────

// TestPortofolio_SoDViolation_MakerCannotApprove verifies that the maker of
// a portofolio cannot act as approver, even with a JWT that has portofolio.approve
// permission. The SoD check is enforced at the service layer, not just the UI.
//
// Regression: §6 SoD enforcement at API level, security-baseline.md.
func TestPortofolio_SoDViolation_MakerCannotApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZPQ_SOD"
	cleanupPortofolio(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPortofolio(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pt_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "pt_sod_reviewer")
	entityID := seedPortofolioDRAFT(t, infra.DB, kode, "SoD Test Portofolio", makerID)

	router := buildPortofolioRouter(infra.DB)

	// Maker is given ALL permissions including approve — this is the bypass attempt.
	makerAllClaims := buildClaimsJSON(makerID, "ROLE-MAKER-TR",
		"portofolio.submit", "portofolio.review", "portofolio.approve",
		"portofolio.read", "portofolio.create",
	)
	reviewerClaims := portofolioReviewerClaims(reviewerID)

	wfPath := "/api/v1/master/portofolio/" + kode

	// SUBMIT as maker.
	w1 := postJSON(router, wfPath+"/submit", makerAllClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT: OK entity=%s", entityID)

	// REVIEW as different user (SoD OK).
	w2 := postJSON(router, wfPath+"/review", reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("REVIEW: OK state=PENDING_APPROVAL")

	// APPROVE attempt as MAKER — must be blocked at service layer.
	w3 := postJSON(router, wfPath+"/approve", makerAllClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
	if w3.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-approver: expected 403 SOD_VIOLATION, got %d body=%s",
			w3.Code, w3.Body.String())
	} else {
		code := errCode(w3.Body.Bytes())
		if code != "SOD_VIOLATION" {
			t.Errorf("expected SOD_VIOLATION, got %q", code)
		}
		t.Logf("SoD correctly blocked maker-as-approver: 403 SOD_VIOLATION")
	}

	// Workflow must remain in PENDING_APPROVAL (not promoted to APPROVED by bypass).
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("workflow state verified: still PENDING_APPROVAL — no bypass")
}

// ─── Test 6: Optimistic lock → 409 ───────────────────────────────────────────

// TestPortofolio_OptimisticLock_Returns409 verifies that a PUT with a stale
// rowVersion returns 409 CONFLICT (optimistic lock guard).
//
// Regression: §2 ECL calc-run reproducibility uses same version guard pattern.
func TestPortofolio_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZPQ_OPTL"
	cleanupPortofolio(t, infra.DB, kode)
	t.Cleanup(func() { cleanupPortofolio(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "pt_optl_maker")
	seedPortofolioDRAFT(t, infra.DB, kode, "Optlock Test Portofolio", makerID)

	router := buildPortofolioRouter(infra.DB)
	claims := portofolioMakerClaims(makerID)

	// First update with rowVersion=1 — succeeds, bumps to rowVersion=2.
	update1 := `{"nama":"Optlock Test Updated","rowVersion":1}`
	w1 := putJSON(router, "/api/v1/master/portofolio/"+kode, claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first update OK; row_version now 2")

	// Verify rowVersion in response is 2.
	var resp1 struct {
		Data struct {
			RowVersion int64 `json:"rowVersion"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &resp1); err == nil {
		if resp1.Data.RowVersion != 2 {
			t.Logf("warning: expected rowVersion=2 in response, got %d", resp1.Data.RowVersion)
		}
	}

	// Second update with stale rowVersion=1 — must return 409 CONFLICT.
	update2 := `{"nama":"Optlock Stale Update","rowVersion":1}`
	w2 := putJSON(router, "/api/v1/master/portofolio/"+kode, claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	code := errCode(w2.Body.Bytes())
	if code != "CONFLICT" {
		t.Errorf("expected CONFLICT error code, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion=1: 409 CONFLICT")

	// Confirm the name was NOT changed by the stale update.
	var actualNama string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT nama FROM mst.portofolio WHERE kode_portofolio = $1 AND deleted_at IS NULL
	`, kode).Scan(&actualNama); err != nil {
		t.Fatalf("fetch nama: %v", err)
	}
	if strings.Contains(actualNama, "Stale") {
		t.Errorf("stale update mutated the name to %q — optimistic lock failed to prevent write", actualNama)
	}
	t.Logf("nama unchanged by stale update: %q", actualNama)
}

// ─── assertPortofolioAuditEvent ───────────────────────────────────────────────

// assertPortofolioAuditEvent checks that at least one audit_log row with the
// given action and entity_id exists. Portofolio-specific wrapper around the
// shared assertAuditEvent to make test output clearer.
func assertPortofolioAuditEvent(t *testing.T, db *sql.DB, action string, entityID uuid.UUID) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log WHERE action = $1 AND entity_id = $2
	`, action, entityID).Scan(&count); err != nil {
		t.Fatalf("assertPortofolioAuditEvent query %s: %v", action, err)
	}
	if count == 0 {
		t.Errorf("audit_log: expected event %q for entity %s, got 0 rows", action, entityID)
	}
}

// Compile-time check: DBRepository satisfies Repository.
var _ portofolio.Repository = (*portofolio.DBRepository)(nil)

// buildPortofolioClaimsJSON is a thin alias for use in this file.
func buildPortofolioClaimsJSON(userID uuid.UUID, role string, permissions ...string) string {
	now := time.Now().Unix()
	type minClaims struct {
		Sub               string   `json:"sub"`
		PreferredUsername string   `json:"preferred_username"`
		Roles             []string `json:"roles"`
		Permissions       []string `json:"permissions"`
		TenantID          string   `json:"tenant_id"`
		MFAVerified       bool     `json:"mfa_verified"`
		Exp               int64    `json:"exp"`
		Iat               int64    `json:"iat"`
	}
	c := minClaims{
		Sub:               userID.String(),
		PreferredUsername: "pt_test_" + userID.String()[:8],
		Roles:             []string{role},
		Permissions:       permissions,
		TenantID:          "TUGURE",
		MFAVerified:       true,
		Exp:               now + 3600,
		Iat:               now,
	}
	b, _ := json.Marshal(c)
	return string(b)
}
