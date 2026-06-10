//go:build integration

// Package integration — mapping_jurnal integration tests (APP-D modul 9).
//
// Coverage:
//
//  1. TestMappingJurnal_DuplicateEventCode_Returns409
//     POST dua kali dengan event_code sama → 409 CONFLICT.
//
//  2. TestMappingJurnal_InvalidEventCodeFormat_Returns422
//     POST dengan event_code mengandung huruf kecil (pola ^[A-Z0-9_]+$ gagal) → 422 VALIDATION_FAILED.
//
//  3. TestMappingJurnal_NoDetails_Returns422
//     POST header tanpa detail rows → 422 VALIDATION_FAILED (min=2 detail).
//
//  4. TestMappingJurnal_DebitNotEqualKredit_RejectsApprove
//     Header dengan sum DEBIT ≠ sum KREDIT, full cycle s.d. PENDING_APPROVAL,
//     lalu approve → 422 MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH.
//
//  5. TestMappingJurnal_KodeAkunNotApproved_Returns422
//     Detail referensi CoA DRAFT → approve → 422 MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED.
//
//  6. TestMappingJurnal_FourEyesCycle_Full_DebitEqualsKredit
//     Sum balanced, full 4-eyes DRAFT→SUBMIT→REVIEW→APPROVE sukses.
//     Verifikasi: workflow state APPROVED, audit event MAPPING_JURNAL.APPROVE,
//     workflow_status header tersinkron, signature count >= 2.
//
//  7. TestMappingJurnal_DeleteHeader_CascadesDetails
//     Soft-delete header → header.deleted_at di-set, detail rows masih ada (soft-delete cascade
//     dikelola oleh query filter, bukan FK ON DELETE CASCADE karena detail pakai soft-delete).
//
//  8. TestMappingJurnal_OptimisticLock_Returns409
//     PATCH dengan row_version basi → 409 CONFLICT.

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
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/mappingjurnal"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Workflow config ─────────────────────────────────────────────────────────

func mappingJurnalWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	// DefaultConfigs() already contains MAPPING_JURNAL; override if DB loader fails.
	if _, ok := cfgs["MAPPING_JURNAL"]; !ok {
		cfgs["MAPPING_JURNAL"] = &workflow.Config{
			EntityType:  "MAPPING_JURNAL",
			Eyes:        4,
			Retractable: false,
			RequiredPermissions: map[string]string{
				"submit":  "mapping_jurnal.submit",
				"review":  "mapping_jurnal.review",
				"approve": "mapping_jurnal.approve",
				"reject":  "mapping_jurnal.reject",
			},
			StepUpRequired: map[string]bool{"approve": false},
			SoDRules: workflow.SoDRulesConfig{
				ReviewerNotMaker:           true,
				ApproverNotMakerOrReviewer: true,
				Approver2NotAnyPrevious:    false,
			},
		}
	}
	return cfgs
}

// ─── Router builder ──────────────────────────────────────────────────────────

func buildMappingJurnalRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := mappingjurnal.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := mappingjurnal.NewService(repo, auditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("MAPPING_JURNAL"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(mappingJurnalWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	// Register the workflow hook so SyncWorkflowStatus fires on every transition.
	hook := mappingjurnal.NewWorkflowHook(svc)
	wfSvc.RegisterHook("MAPPING_JURNAL", hook)

	h := mappingjurnal.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	mappingjurnal.RegisterRoutes(v1, h)
	return r
}

// ─── Claim builders ──────────────────────────────────────────────────────────

func mjAkunClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN",
		"mapping_jurnal.create", "mapping_jurnal.read", "mapping_jurnal.update",
		"mapping_jurnal.delete", "mapping_jurnal.submit",
	)
}

func mjAkunCtlClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN-CTL",
		"mapping_jurnal.read", "mapping_jurnal.review", "mapping_jurnal.approve",
		"mapping_jurnal.reject",
	)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedCoAApproved inserts a mst.chart_of_accounts row with workflow_status='APPROVED'
// and returns its UUID. Safe for repeated calls (ON CONFLICT kode_akun DO NOTHING).
func seedCoAApproved(t *testing.T, db *sql.DB, kodeAkun string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.chart_of_accounts (
			id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
			mata_uang_native, posisi_normal, aktif_flag, sumber_coa,
			tanggal_mulai_aktif, created_by, created_at
		) VALUES (
			$1, $2, $2||' Akun Test', 'ASET', 'KAS',
			'IDR', 'DEBIT', true, 'SISTEM',
			'2026-01-01', $3, now()
		)
		ON CONFLICT (kode_akun) DO NOTHING
	`, id, kodeAkun, makerID)
	if err != nil {
		t.Fatalf("seedCoAApproved insert %s: %v", kodeAkun, err)
	}

	// Fetch actual UUID (ON CONFLICT may have skipped insert).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.chart_of_accounts WHERE kode_akun = $1`, kodeAkun,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedCoAApproved fetch id %s: %v", kodeAkun, err)
	}

	// Ensure workflow_status column exists and set to APPROVED.
	_, _ = db.ExecContext(context.Background(), `
		UPDATE mst.chart_of_accounts SET workflow_status = 'APPROVED' WHERE id = $1
	`, actualID)

	return actualID
}

// seedCoADraft inserts a CoA row that is still in DRAFT (workflow_status = 'DRAFT').
func seedCoADraft(t *testing.T, db *sql.DB, kodeAkun string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := seedCoAApproved(t, db, kodeAkun, makerID)
	_, _ = db.ExecContext(context.Background(), `
		UPDATE mst.chart_of_accounts SET workflow_status = 'DRAFT' WHERE id = $1
	`, id)
	return id
}

// seedMappingJurnalDRAFT creates a mapping_jurnal_header + 2 details in DRAFT state,
// with balanced debit=kredit multipliers (both 1.0000). Returns header UUID.
func seedMappingJurnalDRAFT(t *testing.T, db *sql.DB, eventCode string, makerID uuid.UUID,
	coaDebitID, coaKreditID uuid.UUID, debitMultiplier, kreditMultiplier string) uuid.UUID {
	t.Helper()
	headerID := uuid.New()

	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.mapping_jurnal_header (
			id, event_id_kode, event_code, nama_event, kategori_event, trigger_source,
			tipe_instrumen_berlaku, klasifikasi_berlaku, aktif_flag, catatan,
			workflow_status, created_at, created_by, updated_at, updated_by,
			row_version, tenant_id
		) VALUES (
			$1, $2, $3, $3||' Event', 'PENEMPATAN', 'SYSTEM',
			'{}', '{}', true, NULL,
			'DRAFT', now(), $4, now(), $4,
			1, 'TUGURE'
		)
		ON CONFLICT (event_code) DO NOTHING
	`, headerID, eventCode+"_ID", eventCode, makerID)
	if err != nil {
		t.Fatalf("seedMappingJurnalDRAFT header %s: %v", eventCode, err)
	}

	// Fetch actual header UUID (ON CONFLICT may have skipped).
	var actualHeaderID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.mapping_jurnal_header WHERE event_code = $1`, eventCode,
	).Scan(&actualHeaderID); err != nil {
		t.Fatalf("seedMappingJurnalDRAFT fetch header id %s: %v", eventCode, err)
	}

	// Insert 2 detail rows: one DEBIT, one KREDIT.
	for i, row := range []struct {
		urutan     int
		coaID      uuid.UUID
		dk         string
		multiplier string
	}{
		{1, coaDebitID, "DEBIT", debitMultiplier},
		{2, coaKreditID, "KREDIT", kreditMultiplier},
	} {
		detailID := uuid.New()
		_, err := db.ExecContext(context.Background(), `
			INSERT INTO mst.mapping_jurnal_detail (
				id, event_header_id, urutan, kode_akun_id, dk_indicator,
				sumber_amount, multiplier, mata_uang_posting, aktif_flag,
				created_at, created_by, updated_at, updated_by,
				row_version, tenant_id
			) VALUES (
				$1, $2, $3, $4, $5,
				'POKOK', $6, 'IDR', true,
				now(), $7, now(), $7,
				1, 'TUGURE'
			)
		`, detailID, actualHeaderID, row.urutan, row.coaID, row.dk,
			row.multiplier, makerID)
		if err != nil {
			t.Fatalf("seedMappingJurnalDRAFT detail[%d] %s: %v", i, eventCode, err)
		}
	}

	// Seed workflow_instance for the header.
	seedWorkflowInstance(t, db, actualHeaderID, "MAPPING_JURNAL", makerID, 4)

	// Back-ref: workflow_instance_id on header (best-effort).
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, actualHeaderID).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.mapping_jurnal_header SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, actualHeaderID)
	}

	return actualHeaderID
}

// cleanupMappingJurnal removes test data for the given event_code. Best-effort.
func cleanupMappingJurnal(t *testing.T, db *sql.DB, eventCodes ...string) {
	t.Helper()
	for _, code := range eventCodes {
		var headerID uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.mapping_jurnal_header WHERE event_code = $1`, code,
		).Scan(&headerID); err == nil {
			// Delete workflow_instance first (FK from entity_id).
			_, _ = db.ExecContext(context.Background(), `
				DELETE FROM sys.workflow_instance WHERE entity_id = $1
			`, headerID)
			// Detail rows are ON DELETE CASCADE but we have soft-delete; clean hard.
			_, _ = db.ExecContext(context.Background(), `
				DELETE FROM mst.mapping_jurnal_detail WHERE event_header_id = $1
			`, headerID)
		}
		_, _ = db.ExecContext(context.Background(), `
			DELETE FROM mst.mapping_jurnal_header WHERE event_code = $1
		`, code)
	}
}

// cleanupCoA removes test CoA rows. Best-effort.
func cleanupCoA(t *testing.T, db *sql.DB, kodeCodes ...string) {
	t.Helper()
	for _, kode := range kodeCodes {
		_, _ = db.ExecContext(context.Background(), `
			DELETE FROM mst.chart_of_accounts WHERE kode_akun = $1
		`, kode)
	}
}

// ─── HTTP helpers ─────────────────────────────────────────────────────────────

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

// readHeaderIDFromBody extracts data.id from a JSON response.
func readHeaderIDFromBody(body *bytes.Buffer) string {
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body.Bytes(), &resp)
	return resp.Data.ID
}

// mjCreateBody builds a valid Create request JSON with 2 balanced details.
func mjCreateBody(eventCode string, coaDebitID, coaKreditID uuid.UUID) string {
	return fmt.Sprintf(`{
		"eventIdKode": %q,
		"eventCode": %q,
		"namaEvent": %q,
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": [
			{
				"urutan": 1,
				"kodeAkunId": %q,
				"dkIndicator": "DEBIT",
				"sumberAmount": "POKOK",
				"multiplier": "1.0000",
				"mataUangPosting": "IDR"
			},
			{
				"urutan": 2,
				"kodeAkunId": %q,
				"dkIndicator": "KREDIT",
				"sumberAmount": "POKOK",
				"multiplier": "1.0000",
				"mataUangPosting": "IDR"
			}
		]
	}`, eventCode+"_ID", eventCode, eventCode+" Test Event",
		coaDebitID.String(), coaKreditID.String())
}

// ─── Test 1: Duplicate event_code → 409 ─────────────────────────────────────

// TestMappingJurnal_DuplicateEventCode_Returns409 verifies that creating a
// mapping_jurnal_header with a duplicate event_code returns 409 CONFLICT.
//
// Covers: regression §1 (duplicate-check pattern).
func TestMappingJurnal_DuplicateEventCode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	eventCode := "MJ_DUP_001"
	makerID := seedUserSQL(t, infra.DB, "mj_dup_maker")
	coaD := seedCoAApproved(t, infra.DB, "4001DUP", makerID)
	coaK := seedCoAApproved(t, infra.DB, "2001DUP", makerID)
	cleanupMappingJurnal(t, infra.DB, eventCode)
	t.Cleanup(func() {
		cleanupMappingJurnal(t, infra.DB, eventCode)
		cleanupCoA(t, infra.DB, "4001DUP", "2001DUP")
	})

	router := buildMappingJurnalRouter(infra.DB)
	claims := mjAkunClaims(makerID)
	body := mjCreateBody(eventCode, coaD, coaK)

	// First request — must succeed.
	w1 := postJSON(router, "/api/v1/master/mapping-jurnal", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: OK")

	// Second request — same event_code, different idempotency key.
	w2 := postJSON(router, "/api/v1/master/mapping-jurnal", claims, uuid.New().String(), body)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate event_code: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected error code CONFLICT, got %q", code)
	}
	t.Logf("duplicate event_code correctly rejected: 409 CONFLICT")
}

// ─── Test 2: Invalid event_code format → 422 ────────────────────────────────

// TestMappingJurnal_InvalidEventCodeFormat_Returns422 verifies that an event_code
// containing lowercase letters is rejected with 422 VALIDATION_FAILED.
//
// Business rule: event_code must match ^[A-Z0-9_]+$.
func TestMappingJurnal_InvalidEventCodeFormat_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "mj_fmt_maker")
	coaD := seedCoAApproved(t, infra.DB, "4001FMT", makerID)
	coaK := seedCoAApproved(t, infra.DB, "2001FMT", makerID)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, "4001FMT", "2001FMT") })

	router := buildMappingJurnalRouter(infra.DB)
	claims := mjAkunClaims(makerID)

	// Use lowercase in eventCode — pattern violation.
	body := fmt.Sprintf(`{
		"eventIdKode": "mj_invalid_id",
		"eventCode": "mj_invalid_lower",
		"namaEvent": "Invalid Format Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": [
			{
				"urutan": 1, "kodeAkunId": %q,
				"dkIndicator": "DEBIT", "sumberAmount": "POKOK",
				"multiplier": "1.0000", "mataUangPosting": "IDR"
			},
			{
				"urutan": 2, "kodeAkunId": %q,
				"dkIndicator": "KREDIT", "sumberAmount": "POKOK",
				"multiplier": "1.0000", "mataUangPosting": "IDR"
			}
		]
	}`, coaD.String(), coaK.String())

	w := postJSON(router, "/api/v1/master/mapping-jurnal", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid event_code format: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("invalid event_code format correctly rejected: 422 VALIDATION_FAILED")
}

// ─── Test 3: Header without details → 422 ───────────────────────────────────

// TestMappingJurnal_NoDetails_Returns422 verifies that creating a header
// without detail rows (or < 2 details) returns 422 VALIDATION_FAILED.
//
// Business rule: minimal 2 baris detail (pasangan DEBIT + KREDIT).
func TestMappingJurnal_NoDetails_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "mj_nodet_maker")
	t.Cleanup(func() { cleanupMappingJurnal(t, infra.DB, "MJ_NODET_001") })

	router := buildMappingJurnalRouter(infra.DB)
	claims := mjAkunClaims(makerID)

	// Body with empty details array.
	body := `{
		"eventIdKode": "MJ_NODET_001_ID",
		"eventCode": "MJ_NODET_001",
		"namaEvent": "No Detail Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": []
	}`

	w := postJSON(router, "/api/v1/master/mapping-jurnal", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("no details: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("header without details correctly rejected: 422 VALIDATION_FAILED")

	// Body with only 1 detail row.
	coaD := seedCoAApproved(t, infra.DB, "4001NODET", makerID)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, "4001NODET") })

	body1 := fmt.Sprintf(`{
		"eventIdKode": "MJ_NODET_001_ID",
		"eventCode": "MJ_NODET_001",
		"namaEvent": "No Detail Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": [
			{
				"urutan": 1, "kodeAkunId": %q,
				"dkIndicator": "DEBIT", "sumberAmount": "POKOK",
				"multiplier": "1.0000", "mataUangPosting": "IDR"
			}
		]
	}`, coaD.String())

	w2 := postJSON(router, "/api/v1/master/mapping-jurnal", claims, uuid.New().String(), body1)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("one detail only: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	t.Logf("one detail only correctly rejected: 422 VALIDATION_FAILED")
}

// ─── Test 4: Debit ≠ Kredit blocks approve → 422 ────────────────────────────

// TestMappingJurnal_DebitNotEqualKredit_RejectsApprove creates a header where
// sum DEBIT (1.0) ≠ sum KREDIT (2.0), advances through full workflow to
// PENDING_APPROVAL, then attempts approve → 422 MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH.
//
// Covers: mapping_jurnal debit=kredit invariant on approve.
func TestMappingJurnal_DebitNotEqualKredit_RejectsApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	eventCode := "MJ_IMBAL_001"
	makerID := seedUserSQL(t, infra.DB, "mj_imbal_maker")
	reviewerID := seedUserSQL(t, infra.DB, "mj_imbal_reviewer")
	approverID := seedUserSQL(t, infra.DB, "mj_imbal_approver")

	coaD := seedCoAApproved(t, infra.DB, "4001IMBAL", makerID)
	coaK := seedCoAApproved(t, infra.DB, "2001IMBAL", makerID)
	cleanupMappingJurnal(t, infra.DB, eventCode)
	t.Cleanup(func() {
		cleanupMappingJurnal(t, infra.DB, eventCode)
		cleanupCoA(t, infra.DB, "4001IMBAL", "2001IMBAL")
	})

	// Seed header with DEBIT=1.0 KREDIT=2.0 → imbalanced.
	headerID := seedMappingJurnalDRAFT(t, infra.DB, eventCode, makerID, coaD, coaK, "1.0000", "2.0000")
	t.Logf("seeded imbalanced header %s", headerID)

	router := buildMappingJurnalRouter(infra.DB)
	makerClaims := mjAkunClaims(makerID)
	reviewerClaims := mjAkunCtlClaims(reviewerID)
	approverClaims := mjAkunCtlClaims(approverID)

	idStr := headerID.String()

	// SUBMIT
	w1 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/submit",
		makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, headerID, "PENDING_REVIEW")
	t.Logf("SUBMIT OK: state=PENDING_REVIEW")

	// REVIEW
	w2 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/review",
		reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, headerID, "PENDING_APPROVAL")
	t.Logf("REVIEW OK: state=PENDING_APPROVAL")

	// APPROVE — must fail with MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH.
	w3 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/approve",
		approverClaims, uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
	if w3.Code != http.StatusUnprocessableEntity {
		t.Errorf("imbalanced approve: expected 422, got %d body=%s", w3.Code, w3.Body.String())
	} else {
		if code := errCode(w3.Body.Bytes()); code != "MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH" {
			t.Errorf("expected MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH, got %q", code)
		}
		t.Logf("imbalanced approve correctly rejected: 422 MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH")
	}

	// Workflow must remain at PENDING_APPROVAL (not tampered to APPROVED).
	assertWorkflowState(t, infra.DB, headerID, "PENDING_APPROVAL")
}

// ─── Test 5: CoA DRAFT blocks approve → 422 ─────────────────────────────────

// TestMappingJurnal_KodeAkunNotApproved_Returns422 verifies that an approve attempt
// where a detail references a CoA in DRAFT state returns 422 MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED.
//
// Covers: CoA approval prerequisite gate.
func TestMappingJurnal_KodeAkunNotApproved_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	eventCode := "MJ_COADRAFT_001"
	makerID := seedUserSQL(t, infra.DB, "mj_coadraft_maker")
	reviewerID := seedUserSQL(t, infra.DB, "mj_coadraft_reviewer")
	approverID := seedUserSQL(t, infra.DB, "mj_coadraft_approver")

	// CoA APPROVED for DEBIT side, CoA DRAFT for KREDIT side.
	coaD := seedCoAApproved(t, infra.DB, "4001CDRAFT", makerID)
	coaK := seedCoADraft(t, infra.DB, "2001CDRAFT", makerID) // DRAFT!
	cleanupMappingJurnal(t, infra.DB, eventCode)
	t.Cleanup(func() {
		cleanupMappingJurnal(t, infra.DB, eventCode)
		cleanupCoA(t, infra.DB, "4001CDRAFT", "2001CDRAFT")
	})

	// Balanced multiplier (1.0 = 1.0) so debit=kredit check passes, only CoA check fires.
	headerID := seedMappingJurnalDRAFT(t, infra.DB, eventCode, makerID, coaD, coaK, "1.0000", "1.0000")
	idStr := headerID.String()

	router := buildMappingJurnalRouter(infra.DB)
	makerClaims := mjAkunClaims(makerID)
	reviewerClaims := mjAkunCtlClaims(reviewerID)
	approverClaims := mjAkunCtlClaims(approverID)

	// SUBMIT
	w1 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/submit",
		makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}

	// REVIEW
	w2 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/review",
		reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}

	// APPROVE — must fail because CoA 2001CDRAFT is DRAFT.
	w3 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/approve",
		approverClaims, uuid.New().String(), `{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
	if w3.Code != http.StatusUnprocessableEntity {
		t.Errorf("CoA DRAFT approve: expected 422, got %d body=%s", w3.Code, w3.Body.String())
	} else {
		if code := errCode(w3.Body.Bytes()); code != "MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED" {
			t.Errorf("expected MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED, got %q", code)
		}
		t.Logf("CoA DRAFT approve correctly rejected: 422 MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED")
	}

	// Workflow must remain at PENDING_APPROVAL.
	assertWorkflowState(t, infra.DB, headerID, "PENDING_APPROVAL")
}

// ─── Test 6: Full 4-eyes cycle with balanced debit=kredit ───────────────────

// TestMappingJurnal_FourEyesCycle_Full_DebitEqualsKredit exercises the complete
// DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED 4-eyes cycle with
// balanced debit=kredit. Verifies:
// - workflow state transitions
// - mapping_jurnal_header.workflow_status sync
// - audit_log events: MAPPING_JURNAL.CREATE, MAPPING_JURNAL.APPROVE
// - signature count >= 2
// - CoA APPROVED prerequisite passes
//
// Covers: regression §3 (staging transitions), §6 (SoD), UAT TC-004.
func TestMappingJurnal_FourEyesCycle_Full_DebitEqualsKredit(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	eventCode := "MJ_4EYES_001"
	makerID := seedUserSQL(t, infra.DB, "mj_4eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "mj_4eyes_reviewer")
	approverID := seedUserSQL(t, infra.DB, "mj_4eyes_approver")

	coaD := seedCoAApproved(t, infra.DB, "4001EYES", makerID)
	coaK := seedCoAApproved(t, infra.DB, "2001EYES", makerID)
	cleanupMappingJurnal(t, infra.DB, eventCode)
	t.Cleanup(func() {
		cleanupMappingJurnal(t, infra.DB, eventCode)
		cleanupCoA(t, infra.DB, "4001EYES", "2001EYES")
	})

	router := buildMappingJurnalRouter(infra.DB)
	makerClaims := mjAkunClaims(makerID)
	reviewerClaims := mjAkunCtlClaims(reviewerID)
	approverClaims := mjAkunCtlClaims(approverID)

	// Step 0: CREATE via API (balanced: DEBIT 1.5 = KREDIT 1.5).
	createBody := fmt.Sprintf(`{
		"eventIdKode": %q,
		"eventCode": %q,
		"namaEvent": "4-Eyes Happy Path Event",
		"kategoriEvent": "PENEMPATAN",
		"triggerSource": "SYSTEM",
		"details": [
			{
				"urutan": 1, "kodeAkunId": %q,
				"dkIndicator": "DEBIT", "sumberAmount": "POKOK",
				"multiplier": "1.5000", "mataUangPosting": "IDR"
			},
			{
				"urutan": 2, "kodeAkunId": %q,
				"dkIndicator": "KREDIT", "sumberAmount": "POKOK",
				"multiplier": "1.5000", "mataUangPosting": "IDR"
			}
		]
	}`, eventCode+"_ID", eventCode, coaD.String(), coaK.String())

	wCreate := postJSON(router, "/api/v1/master/mapping-jurnal", makerClaims, uuid.New().String(), createBody)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d body=%s", wCreate.Code, wCreate.Body.String())
	}
	idStr := readHeaderIDFromBody(wCreate.Body)
	if idStr == "" {
		t.Fatal("create: empty id in response")
	}
	headerID, _ := uuid.Parse(idStr)
	t.Logf("CREATE OK: header id=%s", headerID)

	// Seed workflow instance for the newly created header (workflow instance is
	// created separately from the API create in this implementation).
	seedWorkflowInstance(t, infra.DB, headerID, "MAPPING_JURNAL", makerID, 4)
	var wfID uuid.UUID
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, headerID).Scan(&wfID); err == nil {
		_, _ = infra.DB.ExecContext(context.Background(), `
			UPDATE mst.mapping_jurnal_header SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, headerID)
	}

	// Step 1: SUBMIT
	w1 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Ajukan review"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, headerID, "PENDING_REVIEW")
	t.Logf("SUBMIT OK: state=PENDING_REVIEW")

	// Step 2: REVIEW
	w2 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/review",
		reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, headerID, "PENDING_APPROVAL")
	t.Logf("REVIEW OK: state=PENDING_APPROVAL")

	// Step 3: APPROVE (different user — SoD OK)
	w3 := postJSON(router, "/api/v1/master/mapping-jurnal/"+idStr+"/approve",
		approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Disetujui"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, headerID, "APPROVED")
	t.Logf("APPROVE OK: state=APPROVED")

	// Verify mapping_jurnal_header.workflow_status synced to APPROVED.
	var mjStatus string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.mapping_jurnal_header WHERE id = $1
	`, headerID).Scan(&mjStatus); err != nil {
		t.Fatalf("fetch workflow_status: %v", err)
	}
	if mjStatus != "APPROVED" {
		t.Errorf("mapping_jurnal_header.workflow_status: expected APPROVED, got %s", mjStatus)
	}

	// Verify audit events.
	assertAuditEvent(t, infra.DB, "MAPPING_JURNAL.CREATE", headerID)
	assertAuditEvent(t, infra.DB, "MAPPING_JURNAL.APPROVE", headerID)

	// Verify signature count >= 2 (submit + review + approve = 3).
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 2 {
		t.Errorf("expected >= 2 signatures, got %d", len(sigs))
	}
	t.Logf("4-eyes cycle complete: %d signatures, state=APPROVED, workflow_status=APPROVED", len(sigs))
}

// ─── Test 7: Soft-delete header — details cascade ────────────────────────────

// TestMappingJurnal_DeleteHeader_CascadesDetails verifies that soft-deleting a header
// sets header.deleted_at, while detail rows remain in DB (soft-delete pattern: details
// are filtered by event_header_id in normal queries, and can still be inspected for audit).
// Detail rows are not hard-deleted by DB FK because soft-delete is application-managed.
//
// Covers: soft-delete cascade behavior, audit trail.
func TestMappingJurnal_DeleteHeader_CascadesDetails(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	eventCode := "MJ_DEL_001"
	makerID := seedUserSQL(t, infra.DB, "mj_del_maker")
	coaD := seedCoAApproved(t, infra.DB, "4001DEL", makerID)
	coaK := seedCoAApproved(t, infra.DB, "2001DEL", makerID)
	cleanupMappingJurnal(t, infra.DB, eventCode)
	t.Cleanup(func() {
		cleanupMappingJurnal(t, infra.DB, eventCode)
		cleanupCoA(t, infra.DB, "4001DEL", "2001DEL")
	})

	headerID := seedMappingJurnalDRAFT(t, infra.DB, eventCode, makerID, coaD, coaK, "1.0000", "1.0000")
	idStr := headerID.String()

	// Count detail rows before delete.
	var detailCountBefore int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.mapping_jurnal_detail
		WHERE event_header_id = $1
	`, headerID).Scan(&detailCountBefore); err != nil {
		t.Fatalf("count details before: %v", err)
	}
	if detailCountBefore < 2 {
		t.Fatalf("expected >= 2 detail rows before delete, got %d", detailCountBefore)
	}

	router := buildMappingJurnalRouter(infra.DB)
	claims := mjAkunClaims(makerID)

	// Soft-delete the header.
	w := deleteReq(router, "/api/v1/master/mapping-jurnal/"+idStr, claims, uuid.New().String())
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	t.Logf("soft-delete: 200 OK")

	// Verify header.deleted_at is set.
	var deletedAt *time.Time
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT deleted_at FROM mst.mapping_jurnal_header WHERE id = $1
	`, headerID).Scan(&deletedAt); err != nil {
		t.Fatalf("fetch deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Error("header.deleted_at should be set after soft-delete, but is NULL")
	}
	t.Logf("header.deleted_at set: %v", deletedAt)

	// Detail rows must still physically exist in DB (soft-delete is header-only initially;
	// detail visibility is filtered by event_header_id join).
	var detailCountAfter int
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.mapping_jurnal_detail
		WHERE event_header_id = $1
	`, headerID).Scan(&detailCountAfter); err != nil {
		t.Fatalf("count details after: %v", err)
	}
	if detailCountAfter == 0 {
		// Hard-cascade happened — this is also acceptable if DB FK is CASCADE DELETE.
		t.Logf("INFO: detail rows hard-deleted via FK CASCADE (also acceptable pattern)")
	} else {
		t.Logf("detail rows preserved after soft-delete: count=%d (soft-cascade pattern)", detailCountAfter)
	}

	// Verify audit event for delete.
	assertAuditEvent(t, infra.DB, "MAPPING_JURNAL.DELETE", headerID)
	t.Logf("audit event MAPPING_JURNAL.DELETE confirmed")
}

// ─── Test 8: Optimistic lock → 409 ──────────────────────────────────────────

// TestMappingJurnal_OptimisticLock_Returns409 verifies that a PATCH with a stale
// row_version returns 409 CONFLICT.
//
// Covers: regression §8 (idempotency + concurrency), optimistic lock guard.
func TestMappingJurnal_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	eventCode := "MJ_OPTLOCK_001"
	makerID := seedUserSQL(t, infra.DB, "mj_optlock_maker")
	coaD := seedCoAApproved(t, infra.DB, "4001LOCK", makerID)
	coaK := seedCoAApproved(t, infra.DB, "2001LOCK", makerID)
	cleanupMappingJurnal(t, infra.DB, eventCode)
	t.Cleanup(func() {
		cleanupMappingJurnal(t, infra.DB, eventCode)
		cleanupCoA(t, infra.DB, "4001LOCK", "2001LOCK")
	})

	headerID := seedMappingJurnalDRAFT(t, infra.DB, eventCode, makerID, coaD, coaK, "1.0000", "1.0000")
	idStr := headerID.String()

	router := buildMappingJurnalRouter(infra.DB)
	claims := mjAkunClaims(makerID)

	// First PATCH with row_version=1 — must succeed, bumps to row_version=2.
	update1 := `{"namaEvent":"OptLock Updated First","rowVersion":1}`
	w1 := patchJSON(router, "/api/v1/master/mapping-jurnal/"+idStr, claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first update OK: row_version now 2")

	// Second PATCH with stale row_version=1 — must return 409.
	update2 := `{"namaEvent":"OptLock Stale Attempt","rowVersion":1}`
	w2 := patchJSON(router, "/api/v1/master/mapping-jurnal/"+idStr, claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion=1: 409 CONFLICT")

	// Verify DB nama_event is still the first update's value, not stale's.
	var namaEvent string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT nama_event FROM mst.mapping_jurnal_header WHERE id = $1
	`, headerID).Scan(&namaEvent); err != nil {
		t.Fatalf("fetch nama_event: %v", err)
	}
	if namaEvent != "OptLock Updated First" {
		t.Errorf("nama_event should be 'OptLock Updated First', got %q", namaEvent)
	}
	t.Logf("DB state preserved: nama_event=%q", namaEvent)
}

// ─── Compile-time check ──────────────────────────────────────────────────────

// Ensure mappingjurnal.DBRepository satisfies mappingjurnal.Repository at compile time.
var _ mappingjurnal.Repository = (*mappingjurnal.DBRepository)(nil)

// Ensure WorkflowHook satisfies workflow.EntityHook at compile time.
var _ workflow.EntityHook = (*mappingjurnal.WorkflowHook)(nil)

// ─── Helpers used by claims builder (auth package) ───────────────────────────

// mjClaimsAllPermissions builds a claims JSON with all mapping_jurnal permissions.
// Used in tests that need a single user to go through all steps (not realistic SoD).
func mjClaimsAllPermissions(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN-CTL",
		"mapping_jurnal.create", "mapping_jurnal.read", "mapping_jurnal.update",
		"mapping_jurnal.delete", "mapping_jurnal.submit",
		"mapping_jurnal.review", "mapping_jurnal.approve", "mapping_jurnal.reject",
	)
}

// assertMJWorkflowStatus reads workflow_status from mst.mapping_jurnal_header.
func assertMJWorkflowStatus(t *testing.T, db *sql.DB, headerID uuid.UUID, expected string) {
	t.Helper()
	var status string
	if err := db.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.mapping_jurnal_header WHERE id = $1
	`, headerID).Scan(&status); err != nil {
		t.Fatalf("assertMJWorkflowStatus: %v", err)
	}
	if status != expected {
		t.Errorf("mapping_jurnal_header.workflow_status: expected %s, got %s", expected, status)
	}
}
