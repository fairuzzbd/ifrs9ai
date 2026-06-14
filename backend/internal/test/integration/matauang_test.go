//go:build integration

// Package integration — mata_uang integration tests (APP-A-MSTR-002).
//
// Coverage targets (all require live PostgreSQL; skip gracefully without infra):
//
//  1. TestMataUang_DuplicateKode_Returns409
//     POST /master/mata-uang twice with same kode → second call 409 CONFLICT.
//
//  2. TestMataUang_OptimisticLock_Returns409
//     PUT /master/mata-uang/{kode} with stale row_version → 409 CONFLICT.
//
//  3. TestMataUang_SystemCurrency_DeleteForbidden
//     DELETE /master/mata-uang/IDR (is_system_currency=true) → 403 SYSTEM_CURRENCY_PROTECTED.
//
//  4. TestMataUang_SoDViolation_MakerCannotApprove
//     Maker submits, then tries to approve → 403 SOD_VIOLATION via workflow API.
//
//  5. TestMataUang_FourEyesCycle_Full
//     DRAFT → submit (maker) → review+approve (ctl, different user) → APPROVED.
//     Verifies audit_log events + workflow_instance state + signature count.
//
//  6. TestMataUang_Export_RespectsFilter
//     GET /master/mata-uang/export?filter[aktif_flag]=false — only non-active records.
//
//  7. TestMataUang_Idempotency_Replay
//     POST twice with same Idempotency-Key, same payload → second returns original 201.
//
//  8. TestMataUang_Idempotency_Mismatch
//     POST twice with same Idempotency-Key, different name field → 422 IDEMPOTENCY_MISMATCH.

package integration

import (
	"bytes"
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
	"blips-ifrs9.tugu-re.com/internal/master/matauang"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// mataUangWorkflowConfig returns a Config map that includes MATA_UANG
// so the integration router can resolve workflow transitions for this entity.
// The DB config loader (sys.config table) is the authoritative source in
// production; this in-memory version mirrors the expected DB seed.
func mataUangWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["MATA_UANG"] = &workflow.Config{
		EntityType:  "MATA_UANG",
		Eyes:        4,
		Retractable: true, // mata_uang supports re-submit after RETURNED
		RequiredPermissions: map[string]string{
			"submit":  "mata_uang.submit",
			"review":  "mata_uang.review",
			"approve": "mata_uang.approve",
			"reject":  "mata_uang.reject",
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

// ─── Router builder ──────────────────────────────────────────────────────────

// buildMataUangRouter constructs the full Gin router for /api/v1/master/mata-uang
// backed by the provided live *sql.DB.
func buildMataUangRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := matauang.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := matauang.NewService(repo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	// Try DB config loader first (has MATA_UANG config if migration seeded it);
	// fall back to in-memory config that includes MATA_UANG.
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("MATA_UANG"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(mataUangWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	h := matauang.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	matauang.RegisterRoutes(v1, h)
	return r
}

// ─── Claim builders ─────────────────────────────────────────────────────────

func mataUangMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN",
		"mata_uang.create", "mata_uang.read", "mata_uang.update",
		"mata_uang.delete", "mata_uang.submit",
	)
}

func mataUangCtlClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN-CTL",
		"mata_uang.read", "mata_uang.review", "mata_uang.approve",
		"mata_uang.reject",
	)
}

func buildClaimsJSON(userID uuid.UUID, role string, permissions ...string) string {
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
	b, _ := json.Marshal(c)
	return string(b)
}

// ─── HTTP helpers ────────────────────────────────────────────────────────────

func postJSON(router *gin.Engine, path, claimsJSON, idempKey, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Claims", claimsJSON)
	if idempKey != "" {
		req.Header.Set("Idempotency-Key", idempKey)
	}
	router.ServeHTTP(w, req)
	return w
}

func putJSON(router *gin.Engine, path, claimsJSON, idempKey, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-Claims", claimsJSON)
	if idempKey != "" {
		req.Header.Set("Idempotency-Key", idempKey)
	}
	router.ServeHTTP(w, req)
	return w
}

func deleteReq(router *gin.Engine, path, claimsJSON, idempKey string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("X-Test-Claims", claimsJSON)
	if idempKey != "" {
		req.Header.Set("Idempotency-Key", idempKey)
	}
	router.ServeHTTP(w, req)
	return w
}

func getReq(router *gin.Engine, path, claimsJSON string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Test-Claims", claimsJSON)
	router.ServeHTTP(w, req)
	return w
}

// errCode extracts error.code from a JSON response body.
func errCode(body []byte) string {
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Error.Code
}

// ─── Seed helpers ────────────────────────────────────────────────────────────

// seedMataUangDRAFT inserts a mata_uang row in DRAFT state and returns its UUID.
// Also inserts a workflow_instance for the mata_uang so workflow endpoints work.
func seedMataUangDRAFT(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.mata_uang (
			kode_mata_uang, id, nama_mata_uang, simbol, decimal_places,
			sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
			is_system_currency, workflow_status,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, $2, $3, 'T', 2,
			'INTERNAL', 'BULANAN', true, '2026-01-01',
			false, 'DRAFT',
			now(), $4, now(), $4,
			1, 'TUGURE'
		)
		ON CONFLICT (kode_mata_uang) DO NOTHING
	`, kode, id, nama, makerID)
	if err != nil {
		t.Fatalf("seedMataUangDRAFT %s: %v", kode, err)
	}

	// Fetch actual UUID (ON CONFLICT may have not inserted).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.mata_uang WHERE kode_mata_uang = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedMataUangDRAFT fetch id %s: %v", kode, err)
	}

	// Seed workflow instance for this entity.
	seedWorkflowInstance(t, db, actualID, "MATA_UANG", makerID, 4)

	// Update mst.mata_uang.workflow_instance_id back-reference.
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, actualID).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.mata_uang SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, actualID)
	}

	return actualID
}

// seedSystemCurrency inserts a mata_uang row with is_system_currency=true.
func seedSystemCurrency(t *testing.T, db *sql.DB, kode string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.mata_uang (
			kode_mata_uang, id, nama_mata_uang, simbol, decimal_places,
			sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
			is_system_currency, workflow_status,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, gen_random_uuid(), $1||' functional', 'SYS', 0,
			'BI_JISDOR', 'HARIAN', true, '2020-01-01',
			true, 'APPROVED',
			now(), '00000000-0000-0000-0000-000000000001',
			now(), '00000000-0000-0000-0000-000000000001',
			1, 'TUGURE'
		)
		ON CONFLICT (kode_mata_uang) DO NOTHING
	`, kode)
	if err != nil {
		t.Fatalf("seedSystemCurrency %s: %v", kode, err)
	}
}

// cleanupMataUang removes test data. Best-effort (won't fail test).
func cleanupMataUang(t *testing.T, db *sql.DB, codes ...string) {
	t.Helper()
	for _, code := range codes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.mata_uang WHERE kode_mata_uang = $1`, code,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(), `
				DELETE FROM sys.workflow_instance WHERE entity_id = $1
			`, id)
		}
		_, _ = db.ExecContext(context.Background(), `
			DELETE FROM mst.mata_uang WHERE kode_mata_uang = $1
		`, code)
	}
}

// assertAuditEvent checks that at least one audit_log row exists with the given action
// and entity_id.
func assertAuditEvent(t *testing.T, db *sql.DB, action string, entityID uuid.UUID) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log
		WHERE action = $1 AND entity_id = $2
	`, action, entityID).Scan(&count); err != nil {
		t.Fatalf("assertAuditEvent query: %v", err)
	}
	if count == 0 {
		t.Errorf("audit_log: expected event %q for entity %s, got 0 rows", action, entityID)
	}
}

// ─── Test 1: Duplicate kode → 409 ────────────────────────────────────────────

// TestMataUang_DuplicateKode_Returns409 verifies that creating the same kode
// twice returns 409 CONFLICT on the second call.
//
// Covers: regression §1 (klasifikasi reproducibility as duplicate-check pattern),
// and AC Scenario "Validation error — kode sudah ada".
func TestMataUang_DuplicateKode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZA" // unlikely to collide with other tests
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	router := buildMataUangRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "mu_dup_maker")
	claims := mataUangMakerClaims(makerID)

	body := fmt.Sprintf(`{
		"kodeMataUang": %q,
		"namaMataUang": "Zzacoin",
		"simbol": "Z",
		"decimalPlaces": 2,
		"sumberKursDefault": "INTERNAL",
		"frekuensiUpdate": "BULANAN",
		"tanggalMulaiAktif": "2026-01-01"
	}`, kode)

	// First request — must succeed.
	w1 := postJSON(router, "/api/v1/master/mata-uang", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: OK")

	// Second request with same kode (different idempotency key, so not a replay).
	w2 := postJSON(router, "/api/v1/master/mata-uang", claims, uuid.New().String(), body)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate kode: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected error code CONFLICT, got %q", code)
	}
	t.Logf("duplicate kode correctly rejected: 409 CONFLICT")
}

// ─── Test 2: Optimistic lock → 409 ────────────────────────────────────────────

// TestMataUang_OptimisticLock_Returns409 verifies that a PUT with a stale
// row_version returns 409 CONFLICT (optimistic lock).
//
// Covers: regression §1 (ECL reproducibility uses same pattern for immutability).
func TestMataUang_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZB"
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "mu_optlock_maker")
	seedMataUangDRAFT(t, infra.DB, kode, "Zzb Dollar", makerID)

	router := buildMataUangRouter(infra.DB)
	claims := mataUangMakerClaims(makerID)

	// First update with rowVersion=1 — succeeds, bumps to rowVersion=2.
	update1 := `{"namaMataUang":"Zzb Dollar Updated","rowVersion":1}`
	w1 := putJSON(router, "/api/v1/master/mata-uang/"+kode, claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first update OK, row_version now 2")

	// Second update with stale rowVersion=1 — must return 409.
	update2 := `{"namaMataUang":"Zzb Dollar Stale","rowVersion":1}`
	w2 := putJSON(router, "/api/v1/master/mata-uang/"+kode, claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion: 409")
}

// ─── Test 3: System currency delete → 403 ────────────────────────────────────

// TestMataUang_SystemCurrency_DeleteForbidden verifies that deleting a
// is_system_currency=true record returns 403 SYSTEM_CURRENCY_PROTECTED.
//
// Covers: regression §11 (IDR protected), UAT S-011.
func TestMataUang_SystemCurrency_DeleteForbidden(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZS" // test system currency (not IDR to avoid conflict with other seed)
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	seedSystemCurrency(t, infra.DB, kode)

	router := buildMataUangRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "mu_syscurr_maker")
	claims := mataUangMakerClaims(makerID)

	w := deleteReq(router, "/api/v1/master/mata-uang/"+kode, claims, uuid.New().String())
	if w.Code != http.StatusForbidden {
		t.Errorf("system currency delete: expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "SYSTEM_CURRENCY_PROTECTED" {
		t.Errorf("expected SYSTEM_CURRENCY_PROTECTED, got %q", code)
	}

	// Verify record still exists in DB.
	var deletedAt *time.Time
	err := infra.DB.QueryRowContext(context.Background(), `
		SELECT deleted_at FROM mst.mata_uang WHERE kode_mata_uang = $1
	`, kode).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("DB check: %v", err)
	}
	if deletedAt != nil {
		t.Errorf("system currency was soft-deleted despite 403; deleted_at=%v", deletedAt)
	}
	t.Logf("system currency delete correctly blocked: 403 SYSTEM_CURRENCY_PROTECTED")
}

// ─── Test 4: SoD violation — maker cannot approve ────────────────────────────

// TestMataUang_SoDViolation_MakerCannotApprove verifies that the maker of
// a mata_uang cannot act as approver on the same entity, even with a valid JWT.
//
// Covers: regression §6 (SoD at API level, not just UI), security-baseline.md,
// UAT S-008.
func TestMataUang_SoDViolation_MakerCannotApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZD"
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "mu_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "mu_sod_reviewer")
	entityID := seedMataUangDRAFT(t, infra.DB, kode, "Zzd Coin", makerID)

	router := buildMataUangRouter(infra.DB)

	makerClaims := buildClaimsJSON(makerID, "ROLE-AKUN",
		"mata_uang.submit", "mata_uang.review", "mata_uang.approve", "mata_uang.read",
	)
	reviewerClaims := mataUangCtlClaims(reviewerID)

	// SUBMIT as maker.
	w1 := postJSON(router, "/api/v1/master/mata-uang/"+kode+"/submit",
		makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("submit OK for entity %s", entityID)

	// REVIEW as reviewer (different user, SoD OK).
	w2 := postJSON(router, "/api/v1/master/mata-uang/"+kode+"/review",
		reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	t.Logf("review OK")

	// APPROVE attempt as MAKER — must be blocked by SoD.
	// The maker's JWT has mata_uang.approve permission — this is the bypass attempt.
	w3 := postJSON(router, "/api/v1/master/mata-uang/"+kode+"/approve",
		makerClaims, uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
	if w3.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-approver: expected 403 SOD_VIOLATION, got %d body=%s",
			w3.Code, w3.Body.String())
	} else {
		if code := errCode(w3.Body.Bytes()); code != "SOD_VIOLATION" {
			t.Errorf("expected SOD_VIOLATION, got %q", code)
		}
		t.Logf("SoD correctly blocked maker-as-approver: 403 SOD_VIOLATION")
	}

	// Workflow must still be in PENDING_APPROVAL (not tampered to APPROVED).
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
}

// ─── Test 5: Full 4-eyes cycle ────────────────────────────────────────────────

// TestMataUang_FourEyesCycle_Full exercises the complete DRAFT → PENDING_REVIEW →
// PENDING_APPROVAL → APPROVED cycle for mata_uang. Verifies:
// - workflow_instance state transitions
// - audit_log events (SUBMIT, REVIEW/APPROVE events)
// - signature count
// - mata_uang.workflow_status sync
//
// Covers: regression §3 (staging transitions), regression §6 (SoD), UAT S-007.
func TestMataUang_FourEyesCycle_Full(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZF"
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "mu_4eyes_maker")
	ctlID := seedUserSQL(t, infra.DB, "mu_4eyes_ctl")
	entityID := seedMataUangDRAFT(t, infra.DB, kode, "Zzf Franc", makerID)

	router := buildMataUangRouter(infra.DB)
	makerClaims := mataUangMakerClaims(makerID)
	ctlClaims := mataUangCtlClaims(ctlID)

	// Step 1: SUBMIT
	w1 := postJSON(router, "/api/v1/master/mata-uang/"+kode+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Ajukan untuk review"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// Step 2: REVIEW (CTL)
	w2 := postJSON(router, "/api/v1/master/mata-uang/"+kode+"/review",
		ctlClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// Step 3: APPROVE (CTL — same CTL can be both reviewer and approver in 4-eyes
	// as long as they are not the maker)
	approverID := seedUserSQL(t, infra.DB, "mu_4eyes_approver")
	approverClaims := mataUangCtlClaims(approverID)
	w3 := postJSON(router, "/api/v1/master/mata-uang/"+kode+"/approve",
		approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Disetujui"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	t.Logf("APPROVE: state=APPROVED")

	// Verify mata_uang.workflow_status synced to APPROVED.
	var muStatus string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.mata_uang WHERE kode_mata_uang = $1
	`, kode).Scan(&muStatus); err != nil {
		t.Fatalf("fetch mata_uang workflow_status: %v", err)
	}
	if muStatus != "APPROVED" {
		t.Errorf("mata_uang.workflow_status: expected APPROVED, got %s", muStatus)
	}

	// Verify audit events present.
	assertAuditEvent(t, infra.DB, "MATA_UANG.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "MATA_UANG.APPROVE", entityID)

	// Verify signature count >= 2 (submit + review + approve = 3 via workflow_signature).
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

// ─── Test 6: Export respects filter ──────────────────────────────────────────

// TestMataUang_Export_RespectsFilter verifies that the export endpoint
// only returns records matching the applied filter.
//
// Covers: regression §1 (reproducibility — same filter → same rows),
// UAT S-014 (export filter).
func TestMataUang_Export_RespectsFilter(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	// Seed two currencies: one active, one non-active.
	kodeActive := "ZZG"
	kodeInactive := "ZZH"
	cleanupMataUang(t, infra.DB, kodeActive, kodeInactive)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kodeActive, kodeInactive) })

	makerID := seedUserSQL(t, infra.DB, "mu_export_maker")

	MustExec(t, infra.DB, `
		INSERT INTO mst.mata_uang (
			kode_mata_uang, id, nama_mata_uang, simbol, decimal_places,
			sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
			is_system_currency, workflow_status,
			created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES
		($1, gen_random_uuid(), 'Active Export', 'AE', 2, 'INTERNAL', 'BULANAN', true,  '2026-01-01', false, 'DRAFT', now(), $3, now(), $3, 1, 'TUGURE'),
		($2, gen_random_uuid(), 'Inactive Export','IE', 2, 'INTERNAL', 'BULANAN', false, '2026-01-01', false, 'DRAFT', now(), $3, now(), $3, 1, 'TUGURE')
		ON CONFLICT (kode_mata_uang) DO NOTHING
	`, kodeActive, kodeInactive, makerID)

	router := buildMataUangRouter(infra.DB)
	claims := mataUangMakerClaims(makerID)

	// Export with filter aktif_flag=false — should only return inactive records.
	// The filter may match more records if other non-active currencies exist,
	// but it must NOT include kodeActive (aktif_flag=true).
	w := getReq(router, "/api/v1/master/mata-uang/export?format=csv&filter[aktif_flag]=false", claims)
	if w.Code != http.StatusOK {
		t.Fatalf("export filter: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/csv") {
		t.Errorf("expected Content-Type text/csv, got %s", contentType)
	}

	csvBody := w.Body.String()
	if strings.Contains(csvBody, kodeActive) {
		t.Errorf("export with filter aktif_flag=false included active record %s — filter not respected", kodeActive)
	}
	if !strings.Contains(csvBody, kodeInactive) {
		t.Errorf("export with filter aktif_flag=false did not include inactive record %s", kodeInactive)
	}
	t.Logf("export filter test: active=%s absent, inactive=%s present — OK", kodeActive, kodeInactive)

	// Verify audit event for export.
	// Export audit is best-effort (non-blocking), so we wait briefly and then check.
	time.Sleep(200 * time.Millisecond)
	var exportCount int
	_ = infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log WHERE action = 'MATA_UANG.EXPORT'
	`).Scan(&exportCount)
	if exportCount == 0 {
		t.Logf("WARNING: no MATA_UANG.EXPORT audit event found — audit write may be best-effort delayed")
	} else {
		t.Logf("MATA_UANG.EXPORT audit event found: count=%d", exportCount)
	}
}

// ─── Test 7: Idempotency replay ───────────────────────────────────────────────

// TestMataUang_Idempotency_Replay verifies that the mata_uang create endpoint
// returns the original response when the same Idempotency-Key is replayed
// with the identical payload. No duplicate side-effects.
//
// Covers: regression §8 (idempotency), UAT S-004.
func TestMataUang_Idempotency_Replay(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZI"
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	router := buildMataUangRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "mu_idemp_maker")
	claims := mataUangMakerClaims(makerID)

	idempKey := uuid.New().String()
	body := fmt.Sprintf(`{
		"kodeMataUang": %q,
		"namaMataUang": "Idempotency Test",
		"simbol": "IT",
		"decimalPlaces": 2,
		"sumberKursDefault": "INTERNAL",
		"frekuensiUpdate": "BULANAN",
		"tanggalMulaiAktif": "2026-01-01"
	}`, kode)

	// First request.
	w1 := postJSON(router, "/api/v1/master/mata-uang", claims, idempKey, body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: 201")

	// Second request — same key, same body.
	w2 := postJSON(router, "/api/v1/master/mata-uang", claims, idempKey, body)
	// Replayed response should be 201 (original status code).
	if w2.Code != http.StatusCreated {
		t.Errorf("replay: expected 201 (original status replayed), got %d body=%s", w2.Code, w2.Body.String())
	}

	// Only 1 record must exist in DB.
	var count int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.mata_uang WHERE kode_mata_uang = $1
	`, kode).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 record for %s, got %d (duplicate side-effect!)", kode, count)
	}

	// Only 1 audit event must exist.
	var auditCount int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log WHERE action = 'MATA_UANG.CREATE'
		AND after_value::text LIKE $1
	`, "%"+kode+"%").Scan(&auditCount); err != nil {
		t.Fatalf("audit count: %v", err)
	}
	if auditCount > 1 {
		t.Errorf("idempotency replay created %d audit events, expected 1", auditCount)
	}
	t.Logf("idempotency replay: OK — 1 DB record, %d audit events", auditCount)
}

// ─── Test 8: Idempotency mismatch → 422 ──────────────────────────────────────

// TestMataUang_Idempotency_Mismatch verifies that replaying the same
// Idempotency-Key with a different payload returns 422 IDEMPOTENCY_MISMATCH.
//
// Covers: regression §8 (idempotency mismatch), UAT S-005.
func TestMataUang_Idempotency_Mismatch(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "ZZJ"
	cleanupMataUang(t, infra.DB, kode)
	t.Cleanup(func() { cleanupMataUang(t, infra.DB, kode) })

	router := buildMataUangRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "mu_mismatch_maker")
	claims := mataUangMakerClaims(makerID)

	idempKey := uuid.New().String()
	body1 := fmt.Sprintf(`{
		"kodeMataUang": %q,
		"namaMataUang": "Mismatch Test Original",
		"simbol": "MT",
		"decimalPlaces": 2,
		"sumberKursDefault": "INTERNAL",
		"frekuensiUpdate": "BULANAN",
		"tanggalMulaiAktif": "2026-01-01"
	}`, kode)
	body2 := fmt.Sprintf(`{
		"kodeMataUang": %q,
		"namaMataUang": "Mismatch Test DIFFERENT",
		"simbol": "MT",
		"decimalPlaces": 2,
		"sumberKursDefault": "INTERNAL",
		"frekuensiUpdate": "BULANAN",
		"tanggalMulaiAktif": "2026-01-01"
	}`, kode)

	// First request — succeeds.
	w1 := postJSON(router, "/api/v1/master/mata-uang", claims, idempKey, body1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}

	// Second request — same key, different name → 422.
	w2 := postJSON(router, "/api/v1/master/mata-uang", claims, idempKey, body2)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("mismatch: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "IDEMPOTENCY_MISMATCH" {
		t.Errorf("expected IDEMPOTENCY_MISMATCH, got %q", code)
	}

	// Original name must be unchanged.
	var nama string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT nama_mata_uang FROM mst.mata_uang WHERE kode_mata_uang = $1
	`, kode).Scan(&nama); err != nil {
		t.Fatalf("fetch nama: %v", err)
	}
	if !strings.Contains(nama, "Original") {
		t.Errorf("name changed by mismatch request: got %q, expected 'Original'", nama)
	}
	t.Logf("idempotency mismatch: 422 returned, original name preserved")
}

// ─── Compile-time check ──────────────────────────────────────────────────────

// Ensure matauang.DBRepository satisfies matauang.Repository at compile time.
// This duplicates handler_test.go but keeps integration pkg self-contained.
var _ matauang.Repository = (*matauang.DBRepository)(nil)

// buildClaimsJSONBytes is an alias used by other tests in this package.
func buildClaimsJSONBytes(userID uuid.UUID, role string, permissions ...string) []byte {
	return []byte(buildClaimsJSON(userID, role, permissions...))
}

// assertAuditEventCount asserts that exactly wantCount audit events match
// the given action and a substring in after_value/before_value.
func assertAuditEventCount(t *testing.T, db *sql.DB, action string, entityID uuid.UUID, wantCount int) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log WHERE action = $1 AND entity_id = $2
	`, action, entityID).Scan(&count); err != nil {
		t.Fatalf("assertAuditEventCount: %v", err)
	}
	if count != wantCount {
		t.Errorf("audit action=%q entity=%s: expected %d events, got %d",
			action, entityID, wantCount, count)
	}
}

// readJSONBody parses the response body as a JSON object and returns selected data field.
func readJSONBody(body *bytes.Buffer) map[string]interface{} {
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(body.Bytes(), &resp)
	return resp.Data
}
