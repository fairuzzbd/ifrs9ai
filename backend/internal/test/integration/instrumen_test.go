//go:build integration

// Package integration — instrumen integration tests (APP-A-MSTR-011).
//
// Coverage (10 cases):
//
//  1. TestInstrumen_DuplicateKode_Returns409
//  2. TestInstrumen_InvalidTipe_Returns422
//  3. TestInstrumen_CounterpartyNotApproved_Returns422
//  4. TestInstrumen_PortofolioNotApproved_Returns422
//  5. TestInstrumen_MataUangNotApproved_Returns422
//  6. TestInstrumen_RegisterReksadana_RequiresManajerInvestasi
//  7. TestInstrumen_SahamRegister_RequiresBankKustodian
//  8. TestInstrumen_FourEyesCycle_Full
//  9. TestInstrumen_KlasifikasiLocked_CannotEdit
// 10. TestInstrumen_OptimisticLock_Returns409
//
// All tests require a live PostgreSQL via the dev stack (infra.Setup).
// Tests skip gracefully if the stack is not reachable.

package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/instrumen"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Known seed UUIDs (from migration 0002) ─────────────────────────────────

// seeded counterparties (migration 0002) — we patch their workflow_status to APPROVED.
const (
	seedCPBankMandiri   = "11111111-0000-0000-0000-000000000002" // BANK — eligible LPS
	seedCPManajerSchrod = "11111111-0000-0000-0000-000000000020" // MANAJER_INVESTASI
	seedCPKustodianStd  = "11111111-0000-0000-0000-000000000030" // BANK_KUSTODIAN
	seedCPEmiten        = "11111111-0000-0000-0000-000000000040" // EMITEN_SAHAM

	seedPortoHTC = "22222222-0000-0000-0000-000000000001" // PORT-TR-LIQ (HTC)
)

// ─── Router builder ──────────────────────────────────────────────────────────

// buildInstrumenRouter constructs a full Gin router for /api/v1/master/instrumen
// backed by the provided live *sql.DB. The workflow hook is wired so that
// workflow transitions propagate workflow_status back to mst.instrumen.
func buildInstrumenRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := instrumen.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := instrumen.NewService(repo, auditWriter, slog.Default())
	hook := instrumen.NewWorkflowHook(svc)

	wfRepo := workflow.NewDBRepository(db)
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("INSTRUMEN"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(instrumenWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfSvc.RegisterEntityHook("INSTRUMEN", hook)
	wfHandler := workflow.NewHandler(wfSvc)

	h := instrumen.NewHandler(svc, wfHandler)
	v1 := r.Group("/api/v1")
	instrumen.RegisterRoutes(v1, h)
	return r
}

// instrumenWorkflowConfig returns an in-memory workflow config for INSTRUMEN
// matching the DB seed from migration 0019.
func instrumenWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["INSTRUMEN"] = &workflow.Config{
		EntityType:  "INSTRUMEN",
		Eyes:        4,
		Retractable: false,
		RequiredPermissions: map[string]string{
			"submit":  "instrumen.submit",
			"review":  "instrumen.review",
			"approve": "instrumen.approve",
			"reject":  "instrumen.reject",
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

// ─── Claim builders ──────────────────────────────────────────────────────────

func instrumenMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-MAKER-TR",
		"instrumen.create", "instrumen.read", "instrumen.update",
		"instrumen.delete", "instrumen.submit",
	)
}

func instrumenReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-RISK",
		"instrumen.read", "instrumen.review",
	)
}

func instrumenApproverClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-APPR-TR",
		"instrumen.read", "instrumen.approve",
	)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// ensureWorkflowStatusCols ensures mst.counterparty and mst.portofolio have
// a workflow_status column (these are not in the original migration 0001 schema
// but are required by the CheckCounterpartyApproved / CheckPortofolioApproved
// repo queries). This is idempotent — safe to call from every test.
func ensureWorkflowStatusCols(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`ALTER TABLE mst.counterparty ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'APPROVED'`,
		`ALTER TABLE mst.portofolio   ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'APPROVED'`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("ensureWorkflowStatusCols: %v", err)
		}
	}
}

// seedApprovedCounterparty inserts (or updates) a counterparty with
// workflow_status = 'APPROVED'. Returns the UUID.
func seedApprovedCounterparty(t *testing.T, db *sql.DB, kode, nama, tipe string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	ensureWorkflowStatusCols(t, db)
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.counterparty (
			id, kode_counterparty, nama, tipe, tipe_eksposur_basel,
			eligible_lps_flag, status, workflow_status,
			created_at, created_by
		) VALUES (
			$1, $2, $3, $4, 'SENIOR_UNSECURED',
			FALSE, 'AKTIF', 'APPROVED',
			now(), $5
		)
		ON CONFLICT DO NOTHING
	`, id, kode, nama, tipe, makerID)
	if err != nil {
		t.Fatalf("seedApprovedCounterparty %s: %v", kode, err)
	}
	// Fetch actual UUID (ON CONFLICT may have not inserted).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.counterparty WHERE kode_counterparty = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedApprovedCounterparty fetch id %s: %v", kode, err)
	}
	return actualID
}

// seedDraftCounterparty inserts a counterparty with workflow_status = 'DRAFT'.
func seedDraftCounterparty(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	ensureWorkflowStatusCols(t, db)
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.counterparty (
			id, kode_counterparty, nama, tipe, tipe_eksposur_basel,
			eligible_lps_flag, status, workflow_status,
			created_at, created_by
		) VALUES (
			$1, $2, $3, 'BANK', 'SENIOR_UNSECURED',
			FALSE, 'AKTIF', 'DRAFT',
			now(), $4
		)
		ON CONFLICT DO NOTHING
	`, id, kode, nama, makerID)
	if err != nil {
		t.Fatalf("seedDraftCounterparty %s: %v", kode, err)
	}
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.counterparty WHERE kode_counterparty = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedDraftCounterparty fetch id %s: %v", kode, err)
	}
	return actualID
}

// seedApprovedPortofolio inserts a portofolio with workflow_status = 'APPROVED'.
func seedApprovedPortofolio(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	ensureWorkflowStatusCols(t, db)
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.portofolio (
			id, kode_portofolio, nama, bm_category_default,
			workflow_status, created_at, created_by
		) VALUES (
			$1, $2, $3, 'HTC',
			'APPROVED', now(), $4
		)
		ON CONFLICT DO NOTHING
	`, id, kode, nama, makerID)
	if err != nil {
		t.Fatalf("seedApprovedPortofolio %s: %v", kode, err)
	}
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.portofolio WHERE kode_portofolio = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedApprovedPortofolio fetch id %s: %v", kode, err)
	}
	return actualID
}

// seedDraftPortofolio inserts a portofolio with workflow_status = 'DRAFT'.
func seedDraftPortofolio(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	ensureWorkflowStatusCols(t, db)
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.portofolio (
			id, kode_portofolio, nama, bm_category_default,
			workflow_status, created_at, created_by
		) VALUES (
			$1, $2, $3, 'HTC',
			'DRAFT', now(), $4
		)
		ON CONFLICT DO NOTHING
	`, id, kode, nama, makerID)
	if err != nil {
		t.Fatalf("seedDraftPortofolio %s: %v", kode, err)
	}
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.portofolio WHERE kode_portofolio = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedDraftPortofolio fetch id %s: %v", kode, err)
	}
	return actualID
}

// seedApprovedMataUang ensures a mata_uang row exists with workflow_status='APPROVED'.
// The IDR from migration 0002 may or may not have workflow_status; this ensures it.
func seedApprovedMataUang(t *testing.T, db *sql.DB, kode string) {
	t.Helper()
	// Ensure column exists.
	_, _ = db.ExecContext(context.Background(),
		`ALTER TABLE mst.mata_uang ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'APPROVED'`)
	// Upsert or set status to APPROVED for the given kode.
	_, err := db.ExecContext(context.Background(), `
		UPDATE mst.mata_uang SET workflow_status = 'APPROVED' WHERE kode_mata_uang = $1
	`, kode)
	if err != nil {
		t.Fatalf("seedApprovedMataUang %s: %v", kode, err)
	}
}

// seedDraftMataUang inserts or updates a mata_uang row with workflow_status='DRAFT'.
func seedDraftMataUang(t *testing.T, db *sql.DB, kode string) {
	t.Helper()
	_, _ = db.ExecContext(context.Background(),
		`ALTER TABLE mst.mata_uang ADD COLUMN IF NOT EXISTS workflow_status VARCHAR(30) NOT NULL DEFAULT 'APPROVED'`)
	_, _ = db.ExecContext(context.Background(), `
		INSERT INTO mst.mata_uang (
			kode_mata_uang, nama_mata_uang, simbol, decimal_places,
			sumber_kurs_default, frekuensi_update, aktif_flag, tanggal_mulai_aktif,
			workflow_status, created_at, created_by, updated_at, updated_by, row_version, tenant_id
		) VALUES (
			$1, $1||' Draft Currency', 'D', 2,
			'INTERNAL', 'BULANAN', true, '2026-01-01',
			'DRAFT', now(), '00000000-0000-0000-0000-000000000001',
			now(), '00000000-0000-0000-0000-000000000001', 1, 'TUGURE'
		)
		ON CONFLICT (kode_mata_uang) DO UPDATE SET workflow_status = 'DRAFT'
	`, kode)
}

// seedInstrumenDRAFT inserts an instrumen row in DRAFT state and seeds a
// workflow_instance. Returns the instrumen UUID.
func seedInstrumenDRAFT(t *testing.T, db *sql.DB, kode string, cpID, portoID uuid.UUID, mataUang string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.instrumen (
			id, kode_instrumen, tipe_instrumen, sub_tipe, nama,
			counterparty_id, mata_uang, portofolio_id,
			nominal, tanggal_penempatan, auto_renewal_flag, fvoci_election,
			premium_diskonto_awal, biaya_transaksi_capitalized,
			eir_method_flag, day_count_convention,
			status, workflow_status,
			created_at, created_by, updated_at, updated_by,
			row_version, tenant_id, version, is_deleted
		) VALUES (
			$1, $2, 'DEPOSITO', 'Deposito Berjangka', 'Test Instrumen '||$2,
			$3, $4, $5,
			1000000000.00, '2026-01-01', FALSE, FALSE,
			0, 0,
			TRUE, 'ACT/365',
			'AKTIF', 'DRAFT',
			now(), $6, now(), $6,
			1, 'TUGURE', 1, FALSE
		)
		ON CONFLICT DO NOTHING
	`, id, kode, cpID, mataUang, portoID, makerID)
	if err != nil {
		t.Fatalf("seedInstrumenDRAFT %s: %v", kode, err)
	}

	// Fetch actual UUID (ON CONFLICT may have not inserted).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.instrumen WHERE kode_instrumen = $1 AND is_deleted = FALSE`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedInstrumenDRAFT fetch id %s: %v", kode, err)
	}

	// Seed workflow instance.
	seedWorkflowInstance(t, db, actualID, "INSTRUMEN", makerID, 4)

	// Back-reference workflow_instance_id.
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, actualID).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.instrumen SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, actualID)
	}

	return actualID
}

// cleanupInstrumen removes test instrumen rows (best-effort).
func cleanupInstrumen(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.instrumen WHERE kode_instrumen = $1`, kode,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(),
				`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.instrumen WHERE kode_instrumen = $1`, kode)
	}
}

// cleanupCounterparty removes test counterparty rows (best-effort).
func cleanupCounterparty(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.counterparty WHERE kode_counterparty = $1`, kode)
	}
}

// cleanupPortofolio removes test portofolio rows (best-effort).
func cleanupPortofolio(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.portofolio WHERE kode_portofolio = $1`, kode)
	}
}

// instrumenCreateBody builds a minimal valid JSON body for POST /master/instrumen.
func instrumenCreateBody(kode, tipe, subTipe, cpID, portoID, mataUang string, extra ...string) string {
	extraJSON := ""
	for _, e := range extra {
		extraJSON += ", " + e
	}
	return fmt.Sprintf(`{
		"kodeInstrumen": %q,
		"tipeInstrumen": %q,
		"subTipe":       %q,
		"nama":          "Test %s",
		"counterpartyId": %q,
		"mataUang":      %q,
		"portofolioId":  %q,
		"nominal":       "1000000000.00",
		"tanggalPenempatan": "2026-06-01"
		%s
	}`, kode, tipe, subTipe, kode, cpID, mataUang, portoID, extraJSON)
}

// instrumenRespID extracts data.id from a JSON response.
func instrumenRespID(body []byte) string {
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data.ID
}

// instrumenRespRowVersion extracts data.rowVersion from a JSON response.
func instrumenRespRowVersion(body []byte) int64 {
	var resp struct {
		Data struct {
			RowVersion int64 `json:"rowVersion"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data.RowVersion
}

// ─── Test 1: Duplicate kode → 409 ────────────────────────────────────────────

// TestInstrumen_DuplicateKode_Returns409 verifies that creating the same
// kode_instrumen twice (different idempotency key) returns 409 INSTRUMEN_DUPLICATE_KODE.
//
// Regression §1 (klasifikasi uniqueness pattern) + UAT S-001.
func TestInstrumen_DuplicateKode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	kode := "INST-DUP-001"
	cleanupInstrumen(t, infra.DB, kode)
	t.Cleanup(func() { cleanupInstrumen(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "inst_dup_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-DUP-01", "Bank Dup Test", "BANK", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-DUP-01", "Porto Dup Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-DUP-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-DUP-01")
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)
	body := instrumenCreateBody(kode, "DEPOSITO", "Deposito Berjangka", cpID.String(), portID.String(), "IDR")

	// First create — must succeed.
	w1 := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first create: OK")

	// Second create — same kode, different idempotency key → must return 409.
	w2 := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w2.Code != http.StatusConflict {
		t.Errorf("duplicate kode: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "INSTRUMEN_DUPLICATE_KODE" {
		t.Errorf("expected INSTRUMEN_DUPLICATE_KODE, got %q", code)
	}
	t.Logf("duplicate kode correctly rejected: 409 INSTRUMEN_DUPLICATE_KODE")
}

// ─── Test 2: Invalid tipe → 422 ──────────────────────────────────────────────

// TestInstrumen_InvalidTipe_Returns422 verifies that a tipe_instrumen value not
// in the whitelist returns 422 VALIDATION_FAILED.
//
// Regression §1 (SPPI × BM classification matrix requires valid tipe).
func TestInstrumen_InvalidTipe_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	makerID := seedUserSQL(t, infra.DB, "inst_tipe_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-TIPE-01", "Bank Tipe Test", "BANK", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-TIPE-01", "Porto Tipe Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-TIPE-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-TIPE-01")
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)
	body := instrumenCreateBody("INST-TIPE-999", "INVALID_TIPE_XYZ", "Sub Test",
		cpID.String(), portID.String(), "IDR")

	w := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid tipe: expected 400/422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" && code != "INSTRUMEN_INVALID_TIPE" {
		t.Errorf("expected VALIDATION_FAILED or INSTRUMEN_INVALID_TIPE, got %q", code)
	}
	t.Logf("invalid tipe correctly rejected: %d %s", w.Code, errCode(w.Body.Bytes()))
}

// ─── Test 3: Counterparty not APPROVED → 422 ─────────────────────────────────

// TestInstrumen_CounterpartyNotApproved_Returns422 verifies that referencing a
// counterparty in DRAFT state returns 422 INSTRUMEN_COUNTERPARTY_NOT_APPROVED.
//
// Regression §1 FK validation.
func TestInstrumen_CounterpartyNotApproved_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	makerID := seedUserSQL(t, infra.DB, "inst_cpna_maker")
	draftCPID := seedDraftCounterparty(t, infra.DB, "CP-T-DRAFT-01", "Draft Bank Test", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-CPNA-01", "Porto CPNA Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-DRAFT-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-CPNA-01")
		cleanupInstrumen(t, infra.DB, "INST-CPNA-001")
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)
	body := instrumenCreateBody("INST-CPNA-001", "DEPOSITO", "Deposito Berjangka",
		draftCPID.String(), portID.String(), "IDR")

	w := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("draft counterparty: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "INSTRUMEN_COUNTERPARTY_NOT_APPROVED" {
		t.Errorf("expected INSTRUMEN_COUNTERPARTY_NOT_APPROVED, got %q", code)
	}
	t.Logf("draft counterparty correctly rejected: 422 INSTRUMEN_COUNTERPARTY_NOT_APPROVED")
}

// ─── Test 4: Portofolio not APPROVED → 422 ───────────────────────────────────

// TestInstrumen_PortofolioNotApproved_Returns422 verifies that referencing a
// portofolio in DRAFT state returns 422 INSTRUMEN_PORTOFOLIO_NOT_APPROVED.
func TestInstrumen_PortofolioNotApproved_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	makerID := seedUserSQL(t, infra.DB, "inst_portona_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-PORTONA-01", "Bank Porto NA Test", "BANK", makerID)
	draftPortID := seedDraftPortofolio(t, infra.DB, "PORT-T-DRAFT-01", "Draft Porto Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-PORTONA-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-DRAFT-01")
		cleanupInstrumen(t, infra.DB, "INST-PORTONA-001")
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)
	body := instrumenCreateBody("INST-PORTONA-001", "DEPOSITO", "Deposito Berjangka",
		cpID.String(), draftPortID.String(), "IDR")

	w := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("draft portofolio: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "INSTRUMEN_PORTOFOLIO_NOT_APPROVED" {
		t.Errorf("expected INSTRUMEN_PORTOFOLIO_NOT_APPROVED, got %q", code)
	}
	t.Logf("draft portofolio correctly rejected: 422 INSTRUMEN_PORTOFOLIO_NOT_APPROVED")
}

// ─── Test 5: Mata uang not APPROVED → 422 ────────────────────────────────────

// TestInstrumen_MataUangNotApproved_Returns422 verifies that referencing a
// mata_uang in DRAFT state returns 422 INSTRUMEN_MATA_UANG_NOT_APPROVED.
func TestInstrumen_MataUangNotApproved_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	makerID := seedUserSQL(t, infra.DB, "inst_muna_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-MUNA-01", "Bank MUNA Test", "BANK", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-MUNA-01", "Porto MUNA Test", makerID)
	// Seed a draft mata_uang using a rare code unlikely to collide.
	seedDraftMataUang(t, infra.DB, "ZZZ")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-MUNA-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-MUNA-01")
		cleanupInstrumen(t, infra.DB, "INST-MUNA-001")
		_, _ = infra.DB.ExecContext(context.Background(),
			`DELETE FROM mst.mata_uang WHERE kode_mata_uang = 'ZZZ'`)
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)
	body := instrumenCreateBody("INST-MUNA-001", "DEPOSITO", "Deposito Berjangka",
		cpID.String(), portID.String(), "ZZZ")

	w := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("draft mata_uang: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "INSTRUMEN_MATA_UANG_NOT_APPROVED" {
		t.Errorf("expected INSTRUMEN_MATA_UANG_NOT_APPROVED, got %q", code)
	}
	t.Logf("draft mata_uang correctly rejected: 422 INSTRUMEN_MATA_UANG_NOT_APPROVED")
}

// ─── Test 6: REKSADANA requires manajer_investasi_id ────────────────────────

// TestInstrumen_RegisterReksadana_RequiresManajerInvestasi verifies that
// tipe=REKSADANA without manajer_investasi_id returns 422 VALIDATION_FAILED
// (field body.manajerInvestasiId required_for_tipe).
//
// Also requires bank_kustodian_id (REKSADANA is in TipeInstrumenRequiresKustodian too).
// This test checks ONLY the missing manajer case (bank_kustodian_id is provided).
//
// Covers: domain.go TipeInstrumenRequiresManajerInvestasi.
func TestInstrumen_RegisterReksadana_RequiresManajerInvestasi(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	makerID := seedUserSQL(t, infra.DB, "inst_reksa_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-REKSA-01", "Emiten Reksa Test", "EMITEN_SAHAM", makerID)
	kustodianID := seedApprovedCounterparty(t, infra.DB, "CP-T-KUST-REKSA-01", "Kustodian Reksa Test", "BANK_KUSTODIAN", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-REKSA-01", "Porto Reksa Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-REKSA-01", "CP-T-KUST-REKSA-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-REKSA-01")
		cleanupInstrumen(t, infra.DB, "INST-REKSA-001")
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)

	// Provide bank_kustodian_id but omit manajer_investasi_id → must be rejected.
	body := instrumenCreateBody("INST-REKSA-001", "REKSADANA", "Reksadana Pendapatan Tetap",
		cpID.String(), portID.String(), "IDR",
		fmt.Sprintf(`"bankKustodianId": %q`, kustodianID.String()),
	)

	w := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Errorf("REKSADANA no manajer: expected 400/422, got %d body=%s", w.Code, w.Body.String())
	}
	code := errCode(w.Body.Bytes())
	if code != "VALIDATION_FAILED" && code != "INSTRUMEN_MISSING_KUSTODIAN" {
		t.Errorf("expected VALIDATION_FAILED or INSTRUMEN_MISSING_KUSTODIAN, got %q", code)
	}
	t.Logf("REKSADANA without manajerInvestasiId correctly rejected: %d %s", w.Code, code)
}

// ─── Test 7: SAHAM requires bank_kustodian_id ────────────────────────────────

// TestInstrumen_SahamRegister_RequiresBankKustodian verifies that
// tipe=SAHAM without bank_kustodian_id returns 422 VALIDATION_FAILED.
//
// Covers: domain.go TipeInstrumenRequiresKustodian.
func TestInstrumen_SahamRegister_RequiresBankKustodian(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	makerID := seedUserSQL(t, infra.DB, "inst_saham_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-SAHAM-01", "Emiten Saham Test", "EMITEN_SAHAM", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-SAHAM-01", "Porto Saham Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-SAHAM-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-SAHAM-01")
		cleanupInstrumen(t, infra.DB, "INST-SAHAM-001")
	})

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)

	// No bankKustodianId provided for SAHAM → must be rejected.
	body := instrumenCreateBody("INST-SAHAM-001", "SAHAM", "Saham Biasa",
		cpID.String(), portID.String(), "IDR")

	w := postJSON(router, "/api/v1/master/instrumen", claims, uuid.New().String(), body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Errorf("SAHAM no kustodian: expected 400/422, got %d body=%s", w.Code, w.Body.String())
	}
	code := errCode(w.Body.Bytes())
	if code != "VALIDATION_FAILED" && code != "INSTRUMEN_MISSING_KUSTODIAN" {
		t.Errorf("expected VALIDATION_FAILED or INSTRUMEN_MISSING_KUSTODIAN, got %q", code)
	}
	t.Logf("SAHAM without bankKustodianId correctly rejected: %d %s", w.Code, code)
}

// ─── Test 8: Full 4-eyes cycle ────────────────────────────────────────────────

// TestInstrumen_FourEyesCycle_Full exercises the complete
// DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED 4-eyes flow via HTTP.
// Verifies:
//   - Each state transition via API
//   - mst.instrumen.workflow_status synced via WorkflowHook
//   - audit_log events: INSTRUMEN.CREATE, INSTRUMEN.SUBMIT, INSTRUMEN.APPROVE
//   - Signature count >= 2
//   - SoD: maker cannot be reviewer (validated via separate sub-step)
//
// Covers: regression §3 (staging transitions), §6 (SoD), UAT S-007.
func TestInstrumen_FourEyesCycle_Full(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	kode := "INST-4EYE-001"
	cleanupInstrumen(t, infra.DB, kode)
	t.Cleanup(func() { cleanupInstrumen(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "inst_4eye_maker")
	reviewerID := seedUserSQL(t, infra.DB, "inst_4eye_reviewer")
	approverID := seedUserSQL(t, infra.DB, "inst_4eye_approver")

	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-4EYE-01", "Bank 4-eyes Test", "BANK", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-4EYE-01", "Porto 4-eyes Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-4EYE-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-4EYE-01")
	})

	router := buildInstrumenRouter(infra.DB)
	makerClaims := instrumenMakerClaims(makerID)
	reviewerClaims := instrumenReviewerClaims(reviewerID)
	approverClaims := instrumenApproverClaims(approverID)

	// STEP 1: CREATE (DRAFT).
	body := instrumenCreateBody(kode, "DEPOSITO", "Deposito Berjangka",
		cpID.String(), portID.String(), "IDR")
	w1 := postJSON(router, "/api/v1/master/instrumen", makerClaims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("CREATE: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}
	instrumenID := instrumenRespID(w1.Body.Bytes())
	if instrumenID == "" {
		t.Fatal("CREATE: response did not include id")
	}
	t.Logf("CREATE: instrumenID=%s, state=DRAFT", instrumenID)

	// Verify INSTRUMEN.CREATE audit event.
	parsedID, _ := uuid.Parse(instrumenID)
	assertAuditEvent(t, infra.DB, "INSTRUMEN.CREATE", parsedID)

	// STEP 2: SUBMIT (maker).
	w2 := postJSON(router, "/api/v1/master/instrumen/"+instrumenID+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Mengajukan untuk review"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("SUBMIT: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, parsedID, "PENDING_REVIEW")
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// SoD sub-check: maker trying to review own instrumen → must be blocked.
	wSoD := postJSON(router, "/api/v1/master/instrumen/"+instrumenID+"/review",
		// give maker the review permission to test bypass
		buildClaimsJSON(makerID, "ROLE-MAKER-TR",
			"instrumen.create", "instrumen.review", "instrumen.approve", "instrumen.read"),
		uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if wSoD.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-reviewer should be 403, got %d body=%s", wSoD.Code, wSoD.Body.String())
	} else {
		t.Logf("SoD: maker cannot review own submission — blocked: 403")
	}

	// STEP 3: REVIEW (reviewer — different user from maker).
	w3 := postJSON(router, "/api/v1/master/instrumen/"+instrumenID+"/review",
		reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("REVIEW: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, parsedID, "PENDING_APPROVAL")
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// STEP 4: APPROVE (approver — different from maker AND reviewer).
	w4 := postJSON(router, "/api/v1/master/instrumen/"+instrumenID+"/approve",
		approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Disetujui"}`)
	if w4.Code != http.StatusOK {
		t.Fatalf("APPROVE: expected 200, got %d body=%s", w4.Code, w4.Body.String())
	}
	assertWorkflowState(t, infra.DB, parsedID, "APPROVED")
	t.Logf("APPROVE: state=APPROVED")

	// Verify mst.instrumen.workflow_status synced via WorkflowHook.
	var instStatus string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.instrumen WHERE id = $1
	`, parsedID).Scan(&instStatus); err != nil {
		t.Fatalf("fetch instrumen workflow_status: %v", err)
	}
	if instStatus != "APPROVED" {
		t.Errorf("instrumen.workflow_status: expected APPROVED, got %s", instStatus)
	}

	// Verify audit events.
	assertAuditEvent(t, infra.DB, "INSTRUMEN.SUBMIT", parsedID)
	assertAuditEvent(t, infra.DB, "INSTRUMEN.APPROVE", parsedID)

	// Verify signature count >= 2 (submit + review + approve = 3).
	wfID := getWorkflowID(t, infra.DB, parsedID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 2 {
		t.Errorf("expected >= 2 signatures, got %d", len(sigs))
	}
	t.Logf("4-eyes cycle complete: instrumenID=%s, signatures=%d, status=APPROVED", instrumenID, len(sigs))
}

// ─── Test 9: Klasifikasi locked — cannot edit ─────────────────────────────────

// TestInstrumen_KlasifikasiLocked_CannotEdit verifies that once
// klasifikasi_locked_at IS NOT NULL, attempting to change fvoci_election or
// bm_category via PUT returns 423 INSTRUMEN_KLASIFIKASI_LOCKED.
//
// Regression §4 (EIR preserves prior version) + SPPI/BM locking semantics.
func TestInstrumen_KlasifikasiLocked_CannotEdit(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	kode := "INST-LOCK-001"
	cleanupInstrumen(t, infra.DB, kode)
	t.Cleanup(func() { cleanupInstrumen(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "inst_lock_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-LOCK-01", "Bank Lock Test", "BANK", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-LOCK-01", "Porto Lock Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-LOCK-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-LOCK-01")
	})

	// Seed instrumen with klasifikasi_locked_at set directly in DB.
	instrumenID := seedInstrumenDRAFT(t, infra.DB, kode, cpID, portID, "IDR", makerID)

	// Set klasifikasi_locked_at via direct SQL (as if Phase 4 SPPI workflow approved it).
	lockedAt := time.Now().UTC()
	_, err := infra.DB.ExecContext(context.Background(), `
		UPDATE mst.instrumen
		SET klasifikasi_locked_at = $1, klasifikasi_locked_by = $2,
		    klasifikasi_psak71 = 'AC', bm_category = 'HTC'
		WHERE id = $3
	`, lockedAt, makerID, instrumenID)
	if err != nil {
		t.Fatalf("lock klasifikasi: %v", err)
	}
	t.Logf("klasifikasi locked at %s for instrumen %s", lockedAt.Format(time.RFC3339), instrumenID)

	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)

	// Attempt to change bm_category on a locked instrumen → must return 423.
	updateBody := `{"bmCategory":"HTC_S","rowVersion":1}`
	w := putJSON(router, "/api/v1/master/instrumen/"+instrumenID.String(), claims, uuid.New().String(), updateBody)
	if w.Code != http.StatusLocked && w.Code != http.StatusForbidden && w.Code != http.StatusUnprocessableEntity {
		t.Errorf("klasifikasi locked edit: expected 423/403/422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "INSTRUMEN_KLASIFIKASI_LOCKED" {
		t.Errorf("expected INSTRUMEN_KLASIFIKASI_LOCKED, got %q", code)
	}
	t.Logf("klasifikasi locked correctly rejected: %d INSTRUMEN_KLASIFIKASI_LOCKED", w.Code)

	// Attempt to change fvoci_election on a locked instrumen → must also return 423.
	updateBody2 := `{"fvociElection":true,"rowVersion":1}`
	w2 := putJSON(router, "/api/v1/master/instrumen/"+instrumenID.String(), claims, uuid.New().String(), updateBody2)
	if code := errCode(w2.Body.Bytes()); code != "INSTRUMEN_KLASIFIKASI_LOCKED" {
		t.Errorf("fvociElection locked edit: expected INSTRUMEN_KLASIFIKASI_LOCKED, got %q", code)
	}
	t.Logf("fvociElection locked correctly rejected: %d INSTRUMEN_KLASIFIKASI_LOCKED", w2.Code)

	// Non-klasifikasi field update (nama) on a locked instrumen → MUST succeed.
	// (Lock only applies to klasifikasi fields, not all fields.)
	updateBody3 := `{"nama":"Updated Name After Lock","rowVersion":1}`
	w3 := putJSON(router, "/api/v1/master/instrumen/"+instrumenID.String(), claims, uuid.New().String(), updateBody3)
	if w3.Code != http.StatusOK {
		t.Errorf("non-klasifikasi field update should succeed on locked instrumen: expected 200, got %d body=%s",
			w3.Code, w3.Body.String())
	} else {
		t.Logf("non-klasifikasi field update on locked instrumen: OK (200)")
	}
}

// ─── Test 10: Optimistic lock → 409 ──────────────────────────────────────────

// TestInstrumen_OptimisticLock_Returns409 verifies that a PUT with a stale
// row_version returns 409 CONFLICT.
//
// Regression §2 (ECL calc-run reproducibility uses same lock pattern).
func TestInstrumen_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)
	ensureWorkflowStatusCols(t, infra.DB)

	kode := "INST-OPTLOCK-001"
	cleanupInstrumen(t, infra.DB, kode)
	t.Cleanup(func() { cleanupInstrumen(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "inst_optlock_maker")
	cpID := seedApprovedCounterparty(t, infra.DB, "CP-T-OPTLOCK-01", "Bank OptLock Test", "BANK", makerID)
	portID := seedApprovedPortofolio(t, infra.DB, "PORT-T-OPTLOCK-01", "Porto OptLock Test", makerID)
	seedApprovedMataUang(t, infra.DB, "IDR")
	t.Cleanup(func() {
		cleanupCounterparty(t, infra.DB, "CP-T-OPTLOCK-01")
		cleanupPortofolio(t, infra.DB, "PORT-T-OPTLOCK-01")
	})

	instrumenID := seedInstrumenDRAFT(t, infra.DB, kode, cpID, portID, "IDR", makerID)
	router := buildInstrumenRouter(infra.DB)
	claims := instrumenMakerClaims(makerID)

	// First update: rowVersion=1 → succeeds, bumps row_version to 2.
	update1 := `{"nama":"Updated Name v2","rowVersion":1}`
	w1 := putJSON(router, "/api/v1/master/instrumen/"+instrumenID.String(), claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	t.Logf("first update OK (rowVersion now 2)")

	// Second update: stale rowVersion=1 → must return 409 CONFLICT.
	update2 := `{"nama":"Updated Name v3 stale","rowVersion":1}`
	w2 := putJSON(router, "/api/v1/master/instrumen/"+instrumenID.String(), claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected stale rowVersion=1: 409 CONFLICT")

	// Verify DB still has the v2 name (not tampered by stale update).
	var dbNama string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT nama FROM mst.instrumen WHERE id = $1
	`, instrumenID).Scan(&dbNama); err != nil {
		t.Fatalf("fetch nama: %v", err)
	}
	if dbNama != "Updated Name v2" {
		t.Errorf("DB nama: expected 'Updated Name v2', got %q", dbNama)
	}
	t.Logf("DB integrity verified: nama=%q", dbNama)
}
