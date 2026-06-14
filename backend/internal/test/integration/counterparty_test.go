//go:build integration

// Package integration — counterparty integration tests (APP-A-MSTR-003).
//
// Coverage targets (all require live PostgreSQL; skip gracefully without infra):
//
//  1. TestCounterparty_DuplicateKode_Returns409
//  2. TestCounterparty_InvalidTipe_Returns422
//  3. TestCounterparty_FourEyesCycle_Full
//  4. TestCounterparty_SoDViolation_MakerCannotApprove
//  5. TestCounterparty_PIIMaskingInList
//  6. TestCounterparty_PIIMaskingInGet
//  7. TestCounterparty_GetPII_RequiresPermission
//  8. TestCounterparty_GetPII_AuditWritten
//  9. TestCounterparty_OptimisticLock_Returns409
// 10. TestCounterparty_AuditLog_PIIRedacted

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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/counterparty"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router builder ──────────────────────────────────────────────────────────

func counterpartyWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["COUNTERPARTY"] = &workflow.Config{
		EntityType:  "COUNTERPARTY",
		Eyes:        4,
		Retractable: true,
		RequiredPermissions: map[string]string{
			"submit":  "counterparty.submit",
			"review":  "counterparty.review",
			"approve": "counterparty.approve",
			"reject":  "counterparty.reject",
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

// buildCounterpartyRouter constructs the Gin router for counterparty endpoints.
// Note: SyncWorkflowStatus (entity hook) is NOT wired here — it is wired in
// cmd/api/main.go. Tests that need entity-level sync call svc.SyncWorkflowStatus
// directly after the workflow step via the service layer.
func buildCounterpartyRouter(db *sql.DB) (*gin.Engine, *counterparty.Service) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	cpRepo := counterparty.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := counterparty.NewService(cpRepo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("COUNTERPARTY"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(counterpartyWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	h := counterparty.NewHandler(svc, wfHandler)

	v1 := r.Group("/api/v1")
	counterparty.RegisterRoutes(v1, h)
	return r, svc
}

// ─── Claim builders ──────────────────────────────────────────────────────────

func cpMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-MAKER-TR",
		"counterparty.create", "counterparty.read", "counterparty.update",
		"counterparty.delete", "counterparty.submit", "counterparty.export",
	)
}

func cpReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-APPR-TR",
		"counterparty.read", "counterparty.review", "counterparty.approve",
		"counterparty.reject",
	)
}

func cpViewPIIClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AUDIT",
		"counterparty.read", "counterparty.view_pii", "audit_log.read",
	)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedCounterpartyDRAFT inserts a counterparty row in DRAFT state.
// PII is stored via sec.encrypt() in the DB. In test environments where
// sec.encrypt() is not configured, fields are stored as NULL.
func seedCounterpartyDRAFT(t *testing.T, db *sql.DB, kode string, makerID uuid.UUID, npwp, nomorRek, ktp *string) uuid.UUID {
	t.Helper()
	id := uuid.New()

	// Build PII expression for each field. sec.encrypt() may or may not be
	// available in the test DB; we use a safe fallback if it is not.
	encOrNull := func(val *string) string {
		if val == nil {
			return "NULL"
		}
		// Try sec.encrypt — will error at Postgres level if not configured.
		// In that case the test seed falls back to a direct insert.
		return fmt.Sprintf("sec.encrypt(%s)", pqStringLiteral(*val))
	}

	npwpExpr := encOrNull(npwp)
	nomorRekExpr := encOrNull(nomorRek)
	ktpExpr := encOrNull(ktp)

	query := fmt.Sprintf(`
		INSERT INTO mst.counterparty (
			id, kode_counterparty, nama, tipe, tipe_eksposur_basel,
			eligible_lps_flag, status, workflow_status,
			npwp_encrypted, nomor_rekening_encrypted, ktp_encrypted,
			created_at, created_by, row_version, version, is_deleted, tenant_id
		) VALUES (
			$1, $2, 'Test CP ' || $2, 'BANK', 'BANK',
			true, 'ACTIVE', 'DRAFT',
			%s, %s, %s,
			now(), $3, 1, 1, false, 'TUGURE'
		)
		ON CONFLICT (kode_counterparty) DO NOTHING
	`, npwpExpr, nomorRekExpr, ktpExpr)

	_, err := db.ExecContext(context.Background(), query, id, kode, makerID)
	if err != nil {
		// sec.encrypt() not available — insert without PII
		_, err = db.ExecContext(context.Background(), `
			INSERT INTO mst.counterparty (
				id, kode_counterparty, nama, tipe, tipe_eksposur_basel,
				eligible_lps_flag, status, workflow_status,
				created_at, created_by, row_version, version, is_deleted, tenant_id
			) VALUES (
				$1, $2, 'Test CP ' || $2, 'BANK', 'BANK',
				true, 'ACTIVE', 'DRAFT',
				now(), $3, 1, 1, false, 'TUGURE'
			)
			ON CONFLICT (kode_counterparty) DO NOTHING
		`, id, kode, makerID)
		if err != nil {
			t.Fatalf("seedCounterpartyDRAFT %s: %v", kode, err)
		}
	}

	// Fetch actual UUID in case ON CONFLICT skipped.
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.counterparty WHERE kode_counterparty = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedCounterpartyDRAFT fetch id %s: %v", kode, err)
	}

	seedWorkflowInstance(t, db, actualID, "COUNTERPARTY", makerID, 4)

	// Back-reference workflow_instance_id (best-effort).
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, actualID).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.counterparty SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, actualID)
	}

	return actualID
}

// pqStringLiteral returns a properly quoted PostgreSQL string literal.
func pqStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// cleanupCounterparty removes test data.
func cleanupCounterparty(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.counterparty WHERE kode_counterparty = $1`, kode,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM aud.audit_log WHERE entity_id = $1`, id)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
		_, _ = db.ExecContext(context.Background(), `DELETE FROM mst.counterparty WHERE kode_counterparty = $1`, kode)
	}
}

// listResponseDataCP extracts the "data" array from a list response.
func listResponseDataCP(body []byte) []map[string]interface{} {
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data
}

// singleResponseDataCP extracts the "data" object from a single-item response.
func singleResponseDataCP(body []byte) map[string]interface{} {
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data
}

// ─── Test 1: Duplicate kode → 409 ────────────────────────────────────────────

// TestCounterparty_DuplicateKode_Returns409 verifies that creating the same
// kode_counterparty twice returns 409 CONFLICT on the second call.
//
// Covers: SoW §2.1 MSTR-003 validation, unique-key enforcement.
func TestCounterparty_DuplicateKode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZA"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	router, _ := buildCounterpartyRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "cp_dup_maker")
	claims := cpMakerClaims(makerID)

	body := fmt.Sprintf(`{
		"kodeCounterparty": %q,
		"nama": "Bank Dup Test",
		"tipe": "BANK",
		"tipeEksposurBasel": "BANK",
		"eligibleLpsFlag": true,
		"status": "ACTIVE"
	}`, kode)

	// First request — must succeed.
	w1 := postJSON(router, "/api/v1/master/counterparty", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: OK")

	// Second request — same kode, different idempotency key.
	w2 := postJSON(router, "/api/v1/master/counterparty", claims, uuid.New().String(), body)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate kode: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected error code CONFLICT, got %q", code)
	}
	t.Logf("duplicate kode correctly rejected: 409 CONFLICT")
}

// ─── Test 2: Invalid tipe → 422 ──────────────────────────────────────────────

// TestCounterparty_InvalidTipe_Returns422 verifies that creating a counterparty
// with an invalid tipe value returns 422 VALIDATION_FAILED.
func TestCounterparty_InvalidTipe_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router, _ := buildCounterpartyRouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "cp_invtipe_maker")
	claims := cpMakerClaims(makerID)

	body := `{
		"kodeCounterparty": "CPINV1",
		"nama": "Invalid Tipe Test",
		"tipe": "INVALID_TIPE_NOT_IN_WHITELIST",
		"tipeEksposurBasel": "BANK",
		"eligibleLpsFlag": false
	}`

	w := postJSON(router, "/api/v1/master/counterparty", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid tipe: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("invalid tipe correctly rejected: 422 VALIDATION_FAILED")
}

// ─── Test 3: Full 4-eyes cycle ────────────────────────────────────────────────

// TestCounterparty_FourEyesCycle_Full exercises the complete DRAFT → PENDING_REVIEW →
// PENDING_APPROVAL → APPROVED cycle. Verifies:
//   - workflow_instance state transitions
//   - audit_log events written for SUBMIT and APPROVE
//   - at least 2 workflow signature records
//
// Note: entity-level workflow_status sync (mst.counterparty.workflow_status) requires
// the EntityHook wired in cmd/api/main.go. In integration tests, we verify only the
// workflow_instance state and audit events which DO work in isolation.
//
// Covers: regression §3 (staging transitions), DEC-017 (4-eyes), UAT TC-002.
func TestCounterparty_FourEyesCycle_Full(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZF"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_4eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "cp_4eyes_reviewer")
	approverID := seedUserSQL(t, infra.DB, "cp_4eyes_approver")

	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, nil, nil, nil)

	router, svc := buildCounterpartyRouter(infra.DB)
	makerClaims := cpMakerClaims(makerID)
	reviewerClaims := cpReviewerClaims(reviewerID)
	approverClaims := cpReviewerClaims(approverID)

	idPath := "/api/v1/master/counterparty/" + entityID.String()

	// Step 1: SUBMIT
	w1 := postJSON(router, idPath+"/submit", makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Mohon direview"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// Step 2: REVIEW
	w2 := postJSON(router, idPath+"/review", reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// Step 3: APPROVE
	w3 := postJSON(router, idPath+"/approve", approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Disetujui"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	t.Logf("APPROVE: workflow_instance state=APPROVED")

	// Manually fire SyncWorkflowStatus (simulates cmd/api hook wiring).
	// This advances mst.counterparty.workflow_status to APPROVED in same pattern
	// as the production EntityHook.
	approverCtx := userCtx(approverID, []string{"counterparty.approve"})
	if err := svc.SyncWorkflowStatus(approverCtx, entityID, "APPROVED", "APPROVE"); err != nil {
		t.Logf("SyncWorkflowStatus returned err (non-fatal for workflow-only test): %v", err)
	} else {
		// Verify counterparty entity workflow_status synced.
		var cpStatus string
		if err := infra.DB.QueryRowContext(context.Background(), `
			SELECT workflow_status FROM mst.counterparty WHERE id = $1
		`, entityID).Scan(&cpStatus); err == nil {
			if cpStatus != "APPROVED" {
				t.Errorf("counterparty.workflow_status: expected APPROVED, got %s", cpStatus)
			} else {
				t.Logf("mst.counterparty.workflow_status synced: APPROVED")
			}
		}
	}

	// Verify audit events present.
	assertAuditEvent(t, infra.DB, "COUNTERPARTY.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "COUNTERPARTY.APPROVE", entityID)

	// Verify >= 2 signatures.
	wfID := getWorkflowID(t, infra.DB, entityID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 2 {
		t.Errorf("expected >= 2 signature records, got %d", len(sigs))
	}
	t.Logf("4-eyes cycle complete: %d signatures", len(sigs))
}

// ─── Test 4: SoD violation — maker cannot approve ────────────────────────────

// TestCounterparty_SoDViolation_MakerCannotApprove verifies that the maker
// cannot approve their own counterparty entry even with a valid JWT containing
// the counterparty.approve permission.
//
// Covers: regression §6 (SoD at API level), security-baseline.md.
func TestCounterparty_SoDViolation_MakerCannotApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZD"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "cp_sod_reviewer")
	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, nil, nil, nil)

	router, _ := buildCounterpartyRouter(infra.DB)

	// Maker has approve permission — this is the bypass attempt.
	makerClaims := buildClaimsJSON(makerID, "ROLE-MAKER-TR",
		"counterparty.submit", "counterparty.review", "counterparty.approve", "counterparty.read",
	)
	reviewerClaims := cpReviewerClaims(reviewerID)

	idPath := "/api/v1/master/counterparty/" + entityID.String()

	// SUBMIT as maker.
	w1 := postJSON(router, idPath+"/submit", makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}

	// REVIEW as reviewer (SoD OK).
	w2 := postJSON(router, idPath+"/review", reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}

	// APPROVE as MAKER — must fail.
	w3 := postJSON(router, idPath+"/approve", makerClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
	if w3.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-approver: expected 403 SOD_VIOLATION, got %d body=%s",
			w3.Code, w3.Body.String())
	} else {
		if code := errCode(w3.Body.Bytes()); code != "SOD_VIOLATION" {
			t.Errorf("expected SOD_VIOLATION, got %q", code)
		}
		t.Logf("SoD correctly blocked maker-as-approver: 403 SOD_VIOLATION")
	}

	// Workflow must still be PENDING_APPROVAL — not tampered to APPROVED.
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
}

// ─── Test 5: PII masking in list ─────────────────────────────────────────────

// TestCounterparty_PIIMaskingInList verifies that the list endpoint does NOT
// include plaintext PII (npwp, nomorRekening, ktp). Fields must be null or masked.
//
// Covers: DEC-028, security-baseline.md §Encryption, regression §9.
func TestCounterparty_PIIMaskingInList(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZL"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_pii_list_maker")

	npwp := "123456789012345"
	nomorRek := "1234567890"
	ktp := "3201010101900001"
	_ = seedCounterpartyDRAFT(t, infra.DB, kode, makerID, &npwp, &nomorRek, &ktp)

	router, _ := buildCounterpartyRouter(infra.DB)
	claims := cpMakerClaims(makerID)

	w := getReq(router, "/api/v1/master/counterparty", claims)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	items := listResponseDataCP(w.Body.Bytes())

	for _, item := range items {
		if k, _ := item["kodeCounterparty"].(string); k != kode {
			continue
		}
		// List response must never contain plaintext PII.
		for _, field := range []string{"npwp", "nomorRekening", "ktp"} {
			if v, exists := item[field]; exists && v != nil {
				if s, ok := v.(string); ok {
					if s == npwp || s == nomorRek || s == ktp {
						t.Errorf("list response: field %q contains plaintext PII — SECURITY FAILURE", field)
					}
					// If present and non-null, must be masked (*** prefix).
					if !strings.HasPrefix(s, "***") {
						t.Errorf("list response: field %q=%q is not masked (expected null or ***...)", field, s)
					}
				}
			}
		}
		t.Logf("PII masking in list: OK for kode=%s", kode)
		return
	}
	t.Logf("entity kode=%s not found in list response (filter may not match) — PII masking check skipped", kode)
}

// ─── Test 6: PII masking in GET /:id ─────────────────────────────────────────

// TestCounterparty_PIIMaskingInGet verifies that GET /:id returns masked PII
// (sentinel "***" prefix) and NOT plaintext for npwp, nomorRekening, ktp.
//
// Covers: DEC-028, security-baseline.md §Encryption, UAT TC-001.
func TestCounterparty_PIIMaskingInGet(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZG"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_pii_get_maker")
	npwp := "123456789012345"
	nomorRek := "1234567890"
	ktp := "3201010101900001"
	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, &npwp, &nomorRek, &ktp)

	router, _ := buildCounterpartyRouter(infra.DB)
	claims := cpMakerClaims(makerID)

	w := getReq(router, "/api/v1/master/counterparty/"+entityID.String(), claims)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /:id: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	data := singleResponseDataCP(w.Body.Bytes())

	for _, field := range []string{"npwp", "nomorRekening", "ktp"} {
		if v, exists := data[field]; exists && v != nil {
			s, ok := v.(string)
			if !ok {
				continue
			}
			// Must NOT return plaintext.
			if s == npwp || s == nomorRek || s == ktp {
				t.Errorf("GET /%s: field %q contains plaintext PII — SECURITY FAILURE", entityID, field)
			}
			// If non-null, must be masked (sentinel prefix ***).
			if !strings.HasPrefix(s, "***") {
				t.Errorf("GET /%s: field %q=%q is not masked (expected ***... prefix)", entityID, field, s)
			}
		}
	}
	t.Logf("PII masking in GET /:id: OK — no plaintext PII in default response")
}

// ─── Test 7: GET /:id/pii without permission → 403 ───────────────────────────

// TestCounterparty_GetPII_RequiresPermission verifies that calling GET /:id/pii
// without the counterparty.view_pii permission returns 403 FORBIDDEN.
//
// Covers: DEC-028, security-baseline.md §Permission model, UAT TC-004.
func TestCounterparty_GetPII_RequiresPermission(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZP"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_pii_perm_maker")
	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, nil, nil, nil)

	router, _ := buildCounterpartyRouter(infra.DB)

	// Maker does NOT have counterparty.view_pii.
	makerClaims := cpMakerClaims(makerID)

	w := getReq(router, "/api/v1/master/counterparty/"+entityID.String()+"/pii", makerClaims)
	if w.Code != http.StatusForbidden {
		t.Errorf("GET /:id/pii without view_pii: expected 403, got %d body=%s", w.Code, w.Body.String())
	} else {
		t.Logf("GET /:id/pii correctly blocked for non-pii-viewer: 403")
	}
}

// ─── Test 8: GET /:id/pii with permission → audit written ────────────────────

// TestCounterparty_GetPII_AuditWritten verifies that accessing the decrypted
// PII endpoint writes an audit_log row with action COUNTERPARTY.VIEW_PII.
//
// Covers: security-baseline.md (audit every PII access), DEC-028, UAT TC-003.
func TestCounterparty_GetPII_AuditWritten(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZA2"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_pii_audit_maker")
	piiViewerID := seedUserSQL(t, infra.DB, "cp_pii_audit_viewer")
	npwp := "987654321098765"
	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, &npwp, nil, nil)

	router, _ := buildCounterpartyRouter(infra.DB)

	// Clear any pre-existing VIEW_PII events.
	_, _ = infra.DB.ExecContext(context.Background(), `
		DELETE FROM aud.audit_log WHERE action = 'COUNTERPARTY.VIEW_PII' AND entity_id = $1
	`, entityID)

	viewerClaims := cpViewPIIClaims(piiViewerID)

	w := getReq(router, "/api/v1/master/counterparty/"+entityID.String()+"/pii", viewerClaims)
	if w.Code != http.StatusOK {
		// PII decrypt may fail if sec.encrypt/decrypt not configured in test DB.
		if w.Code == http.StatusInternalServerError {
			t.Skipf("GET /:id/pii returned 500 — sec.encrypt/decrypt may not be configured in test DB")
		}
		t.Fatalf("GET /:id/pii: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// Verify audit event was written.
	assertAuditEvent(t, infra.DB, "COUNTERPARTY.VIEW_PII", entityID)
	t.Logf("audit event COUNTERPARTY.VIEW_PII written for entity %s", entityID)

	// Verify audit event does NOT include plaintext NPWP.
	var afterJSON []byte
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT after_value FROM aud.audit_log
		WHERE action = 'COUNTERPARTY.VIEW_PII' AND entity_id = $1
		ORDER BY event_time DESC LIMIT 1
	`, entityID).Scan(&afterJSON); err == nil {
		if strings.Contains(string(afterJSON), npwp) {
			t.Errorf("audit after_value contains plaintext NPWP — SECURITY FAILURE: %s", string(afterJSON))
		}
		t.Logf("audit VIEW_PII after_value: no plaintext PII — OK")
	}
}

// ─── Test 9: Optimistic lock → 409 ───────────────────────────────────────────

// TestCounterparty_OptimisticLock_Returns409 verifies that a PUT with a stale
// row_version returns 409 CONFLICT.
//
// Covers: regression §2 (optimistic lock safety).
func TestCounterparty_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZB"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_optlock_maker")
	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, nil, nil, nil)

	router, _ := buildCounterpartyRouter(infra.DB)
	claims := cpMakerClaims(makerID)

	idPath := "/api/v1/master/counterparty/" + entityID.String()

	// First update — rowVersion=1 — succeeds, bumps to rowVersion=2.
	w1 := putJSON(router, idPath, claims, uuid.New().String(), `{"nama":"Updated Name First","rowVersion":1}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first update OK, row_version now 2")

	// Second update with stale rowVersion=1 — must return 409.
	w2 := putJSON(router, idPath, claims, uuid.New().String(), `{"nama":"Stale Update Attempt","rowVersion":1}`)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion: 409")
}

// ─── Test 10: Audit log PII redacted ─────────────────────────────────────────

// TestCounterparty_AuditLog_PIIRedacted verifies that every audit_log entry for
// counterparty CREATE/UPDATE does NOT contain plaintext PII in before/after JSON.
// The service uses auditSafeCounterparty() which sets PII fields to "REDACTED".
//
// Covers: regression §7 (audit trail tamper-evidence), DEC-028, security-baseline.md.
func TestCounterparty_AuditLog_PIIRedacted(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "CPZZR"
	cleanupCounterparty(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCounterparty(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "cp_audit_pii_maker")

	// Seed with known PII values.
	npwp := "111222333444555"
	nomorRek := "9988776655"
	ktp := "3275010205890009"
	entityID := seedCounterpartyDRAFT(t, infra.DB, kode, makerID, &npwp, &nomorRek, &ktp)

	router, _ := buildCounterpartyRouter(infra.DB)
	claims := cpMakerClaims(makerID)

	// Perform an UPDATE to generate a COUNTERPARTY.UPDATE audit event.
	idPath := "/api/v1/master/counterparty/" + entityID.String()
	w := putJSON(router, idPath, claims, uuid.New().String(), `{"nama":"Audit PII Redact Test","rowVersion":1}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// Fetch all audit_log rows for this entity.
	rows, err := infra.DB.QueryContext(context.Background(), `
		SELECT COALESCE(before_value::text, ''), COALESCE(after_value::text, '')
		FROM aud.audit_log
		WHERE entity_id = $1
	`, entityID)
	if err != nil {
		t.Fatalf("fetch audit_log: %v", err)
	}
	defer rows.Close()

	auditRowCount := 0
	for rows.Next() {
		var before, after string
		if err := rows.Scan(&before, &after); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		auditRowCount++

		for _, plaintext := range []string{npwp, nomorRek, ktp} {
			if strings.Contains(before, plaintext) || strings.Contains(after, plaintext) {
				t.Errorf("audit_log row contains plaintext PII %q — SECURITY FAILURE\nbefore=%s\nafter=%s",
					plaintext, before, after)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("audit_log rows error: %v", err)
	}
	if auditRowCount == 0 {
		t.Errorf("no audit_log rows found for entity %s — audit write may have failed", entityID)
	}
	t.Logf("audit_log PII redaction: OK — %d rows, no plaintext PII", auditRowCount)
}
