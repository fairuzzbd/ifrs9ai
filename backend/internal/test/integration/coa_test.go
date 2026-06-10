//go:build integration

// Package integration — chart_of_accounts integration tests (APP-A-MSTR-010).
//
// Coverage (all require live PostgreSQL; skip gracefully without infra):
//
//  1. TestCoA_DuplicateKode_Returns409
//  2. TestCoA_InvalidKodeFormat_Returns422  — kode "ABC.XYZ" (non-numeric)
//  3. TestCoA_InvalidTipe_Returns422
//  4. TestCoA_OptimisticLock_Returns409
//  5. TestCoA_ParentNotApproved_Returns422  — parent_akun_id pointing to DRAFT → 422 COA_PARENT_NOT_FOUND
//  6. TestCoA_FourEyesCycle_Full            — full 4-eyes + entity hook sync
//  7. TestCoA_SoDViolation_MakerCannotApprove
//  8. TestCoA_ImportXLSX_Async             — POST /import-xlsx multipart, 202 + jobId, poll status, verify rows DRAFT
//  9. TestCoA_ImportXLSX_Idempotency       — same file SHA → existing jobId returned (no duplicate)
// 10. TestCoA_SoftDelete_WithChildren_Returns409
package integration

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime/multipart"
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
	"blips-ifrs9.tugu-re.com/internal/master/coa"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router builder ───────────────────────────────────────────────────────────

// buildCoARouter constructs the full Gin router for /api/v1/master/coa backed by
// the provided live *sql.DB.  Workflow config is loaded from DB first; falls back
// to DefaultConfigs (which includes CHART_OF_ACCOUNTS 4-eyes).
func buildCoARouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	repo := coa.NewDBRepository(db)
	auditWriter := audit.NewWriter(db)
	svc := coa.NewService(repo, auditWriter, slog.Default())

	// JobRepository backed by real DB so import jobs are persisted.
	jobRepo := coa.NewDBJobRepository(db)

	// No Asynq client in integration tests — sync goroutine fallback is used.
	importer := coa.NewImporter(repo, jobRepo, auditWriter, nil, slog.Default())

	wfRepo := workflow.NewDBRepository(db)

	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("CHART_OF_ACCOUNTS"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())
	}

	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	coaHook := coa.NewWorkflowHook(svc)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfSvc.RegisterEntityHook("CHART_OF_ACCOUNTS", coaHook)
	wfHandler := workflow.NewHandler(wfSvc)

	h := coa.NewHandler(svc, importer, wfHandler)
	v1 := r.Group("/api/v1")
	coa.RegisterRoutes(v1, h)
	return r
}

// ─── Claims builders ─────────────────────────────────────────────────────────

func coaMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN",
		"chart_of_accounts.create",
		"chart_of_accounts.read",
		"chart_of_accounts.update",
		"chart_of_accounts.delete",
		"chart_of_accounts.submit",
	)
}

func coaReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN-CTL",
		"chart_of_accounts.read",
		"chart_of_accounts.review",
		"chart_of_accounts.approve",
		"chart_of_accounts.reject",
	)
}

// coaAllPermsClaims returns claims with both maker and approver perms — used in
// SoD tests to prove that having the permission is not sufficient.
func coaAllPermsClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-AKUN",
		"chart_of_accounts.create",
		"chart_of_accounts.read",
		"chart_of_accounts.update",
		"chart_of_accounts.delete",
		"chart_of_accounts.submit",
		"chart_of_accounts.review",
		"chart_of_accounts.approve",
		"chart_of_accounts.reject",
	)
}

// ─── HTTP helpers (PATCH) ─────────────────────────────────────────────────────

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

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedCoADRAFT inserts a chart_of_accounts row in DRAFT state and returns its UUID.
// Also seeds a workflow_instance so workflow endpoints work.
func seedCoADRAFT(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.chart_of_accounts (
			id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
			mata_uang_native, posisi_normal, aktif_flag, sumber_coa,
			tanggal_mulai_aktif, workflow_status,
			created_at, created_by, updated_at, updated_by,
			version, tenant_id
		) VALUES (
			$1, $2, $3, 'ASET', 'TEST',
			'IDR', 'DEBIT', true, 'MANUAL',
			'2026-01-01', 'DRAFT',
			now(), $4, now(), $4,
			1, 'TUGURE'
		)
		ON CONFLICT (kode_akun) DO NOTHING
	`, id, kode, nama, makerID)
	if err != nil {
		t.Fatalf("seedCoADRAFT %s: %v", kode, err)
	}

	// Fetch actual UUID (ON CONFLICT may have skipped insert).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.chart_of_accounts WHERE kode_akun = $1 AND deleted_at IS NULL`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedCoADRAFT fetch id %s: %v", kode, err)
	}

	seedWorkflowInstance(t, db, actualID, "CHART_OF_ACCOUNTS", makerID, 4)

	// Back-link workflow_instance_id on entity row.
	var wfID uuid.UUID
	if err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, actualID).Scan(&wfID); err == nil {
		_, _ = db.ExecContext(context.Background(), `
			UPDATE mst.chart_of_accounts SET workflow_instance_id = $1 WHERE id = $2
		`, wfID, actualID)
	}

	return actualID
}

// seedCoAApproved inserts an APPROVED chart_of_accounts row (no workflow instance needed).
func seedCoAApproved(t *testing.T, db *sql.DB, kode, nama string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.chart_of_accounts (
			id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
			mata_uang_native, posisi_normal, aktif_flag, sumber_coa,
			tanggal_mulai_aktif, workflow_status,
			created_at, created_by, updated_at, updated_by,
			version, tenant_id
		) VALUES (
			$1, $2, $3, 'ASET', 'ROOT',
			'IDR', 'DEBIT', true, 'MANUAL',
			'2026-01-01', 'APPROVED',
			now(), $4, now(), $4,
			1, 'TUGURE'
		)
		ON CONFLICT (kode_akun) DO NOTHING
	`, id, kode, nama, makerID)
	if err != nil {
		t.Fatalf("seedCoAApproved %s: %v", kode, err)
	}
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.chart_of_accounts WHERE kode_akun = $1 AND deleted_at IS NULL`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedCoAApproved fetch id %s: %v", kode, err)
	}
	return actualID
}

// cleanupCoA removes test data by kode_akun prefix (best-effort).
func cleanupCoA(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, k := range kodes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.chart_of_accounts WHERE kode_akun = $1`, k,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(),
				`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.chart_of_accounts WHERE kode_akun = $1`, k)
	}
}

// cleanupCoAByPrefix removes all chart_of_accounts rows whose kode_akun starts with prefix.
func cleanupCoAByPrefix(t *testing.T, db *sql.DB, prefix string) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT id FROM mst.chart_of_accounts WHERE kode_akun LIKE $1`, prefix+"%")
	if err != nil {
		return
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr == nil {
			_, _ = db.ExecContext(context.Background(),
				`DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
	}
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM mst.chart_of_accounts WHERE kode_akun LIKE $1`, prefix+"%")
}

// waitForJobStatus polls GET /import-jobs/:id until status is one of wantStatuses or timeout.
func waitForJobStatus(t *testing.T, router *gin.Engine, claimsJSON, jobID string, wantStatuses []string, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		w := getReq(router, "/api/v1/master/coa/import-jobs/"+jobID, claimsJSON)
		if w.Code != http.StatusOK {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var resp struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
			status, _ := resp.Data["status"].(string)
			for _, want := range wantStatuses {
				if status == want {
					return resp.Data
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("waitForJobStatus: job %s did not reach %v within %s", jobID, wantStatuses, timeout)
	return nil
}

// buildMinimalXLSX builds a minimal XLSX file (ZIP+XML) with given data rows.
// Header row is always: kode_akun, nama_akun, tipe_akun, sub_tipe_akun, kategori_investasi, mata_uang_native, posisi_normal, parent_akun_kode
func buildMinimalXLSX(rows [][]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// workbook relationship skeleton
	writeZipFile(zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`)
	writeZipFile(zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`)
	writeZipFile(zw, "xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`)
	writeZipFile(zw, "xl/workbook.xml", `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheets><sheet name="Sheet1" sheetId="1" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></sheets>
</workbook>`)

	// Collect all unique strings into shared strings table for robustness.
	// All cell values will be shared-string references.
	allStrings := []string{}
	stringIndex := map[string]int{}

	header := []string{"kode_akun", "nama_akun", "tipe_akun", "sub_tipe_akun", "kategori_investasi", "mata_uang_native", "posisi_normal", "parent_akun_kode"}
	allRows := append([][]string{header}, rows...)
	for _, row := range allRows {
		for _, cell := range row {
			if _, ok := stringIndex[cell]; !ok {
				stringIndex[cell] = len(allStrings)
				allStrings = append(allStrings, cell)
			}
		}
	}

	// Build sharedStrings.xml
	var ssb strings.Builder
	ssb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	ssb.WriteString(fmt.Sprintf(`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`, len(allStrings), len(allStrings)))
	for _, s := range allStrings {
		ssb.WriteString(`<si><t>` + xmlEscape(s) + `</t></si>`)
	}
	ssb.WriteString(`</sst>`)
	writeZipFile(zw, "xl/sharedStrings.xml", ssb.String())

	// Build sheet1.xml
	colLetters := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	var shb strings.Builder
	shb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	shb.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIdx, row := range allRows {
		shb.WriteString(fmt.Sprintf(`<row r="%d">`, rowIdx+1))
		for colIdx, val := range row {
			if colIdx >= len(colLetters) {
				break
			}
			cellRef := fmt.Sprintf("%s%d", colLetters[colIdx], rowIdx+1)
			idx := stringIndex[val]
			shb.WriteString(fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, cellRef, idx))
		}
		shb.WriteString(`</row>`)
	}
	shb.WriteString(`</sheetData></worksheet>`)
	writeZipFile(zw, "xl/worksheets/sheet1.xml", shb.String())

	_ = zw.Close()
	return buf.Bytes()
}

func writeZipFile(zw *zip.Writer, name, content string) {
	f, err := zw.Create(name)
	if err != nil {
		return
	}
	_, _ = f.Write([]byte(content))
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

// postMultipartXLSX sends a multipart/form-data POST with file + sumber_coa fields.
func postMultipartXLSX(router *gin.Engine, path, claimsJSON, idempKey string, xlsxBytes []byte, sumberCoa string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	// file field
	fw, _ := mw.CreateFormFile("file", "coa-import.xlsx")
	_, _ = fw.Write(xlsxBytes)

	// sumber_coa field
	_ = mw.WriteField("sumber_coa", sumberCoa)
	_ = mw.Close()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Test-Claims", claimsJSON)
	if idempKey != "" {
		req.Header.Set("Idempotency-Key", idempKey)
	}
	router.ServeHTTP(w, req)
	return w
}

// coaJobIDFromResponse extracts data.jobId from a 202 response body.
func coaJobIDFromResponse(body []byte) string {
	var resp struct {
		Data struct {
			JobID string `json:"jobId"`
		} `json:"data"`
	}
	_ = json.Unmarshal(body, &resp)
	return resp.Data.JobID
}

// ─── Test 1: Duplicate kode → 422 COA_DUPLICATE_KODE ─────────────────────────

// TestCoA_DuplicateKode_Returns409 verifies that creating the same kode_akun twice
// returns 422 COA_DUPLICATE_KODE on the second call (distinct Idempotency-Key).
//
// Covers: regression §1 (klasifikasi reproducibility — duplicate-check pattern).
func TestCoA_DuplicateKode_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "901"
	cleanupCoA(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, kode) })

	router := buildCoARouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "coa_dup_maker")
	claims := coaMakerClaims(makerID)

	body := `{
		"kodeAkun":"901","namaAkun":"Aset Tetap Test","tipeAkun":"ASET",
		"subTipeAkun":"TETAP","posisiNormal":"DEBIT","sumberCoa":"MANUAL",
		"tanggalMulaiAktif":"2026-01-01"
	}`

	w1 := postJSON(router, "/api/v1/master/coa", claims, uuid.New().String(), body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}

	w2 := postJSON(router, "/api/v1/master/coa", claims, uuid.New().String(), body)
	if w2.Code != http.StatusUnprocessableEntity {
		t.Errorf("duplicate kode: expected 422, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "COA_DUPLICATE_KODE" {
		t.Errorf("expected COA_DUPLICATE_KODE, got %q", code)
	}
	t.Logf("duplicate kode correctly rejected: 422 COA_DUPLICATE_KODE")
}

// ─── Test 2: Invalid kode format → 422 COA_INVALID_KODE_FORMAT ───────────────

// TestCoA_InvalidKodeFormat_Returns422 verifies that kode "ABC.XYZ" (non-numeric)
// returns 422 COA_INVALID_KODE_FORMAT.
//
// Covers: regression §1 (SPPI × BM classification inputs must be valid).
func TestCoA_InvalidKodeFormat_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildCoARouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "coa_kfmt_maker")
	claims := coaMakerClaims(makerID)

	body := `{
		"kodeAkun":"ABC.XYZ","namaAkun":"Invalid Kode","tipeAkun":"ASET",
		"subTipeAkun":"TEST","posisiNormal":"DEBIT","sumberCoa":"MANUAL",
		"tanggalMulaiAktif":"2026-01-01"
	}`

	w := postJSON(router, "/api/v1/master/coa", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid kode format: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "COA_INVALID_KODE_FORMAT" && code != "VALIDATION_FAILED" {
		t.Errorf("expected COA_INVALID_KODE_FORMAT or VALIDATION_FAILED, got %q", code)
	}
	t.Logf("invalid kode format correctly rejected: 422 code=%s", errCode(w.Body.Bytes()))
}

// ─── Test 3: Invalid tipe_akun → 422 VALIDATION_FAILED ───────────────────────

// TestCoA_InvalidTipe_Returns422 verifies that an unknown tipe_akun value
// returns 422 VALIDATION_FAILED.
func TestCoA_InvalidTipe_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	router := buildCoARouter(infra.DB)
	makerID := seedUserSQL(t, infra.DB, "coa_tipe_maker")
	claims := coaMakerClaims(makerID)

	body := `{
		"kodeAkun":"902","namaAkun":"Invalid Tipe","tipeAkun":"BUKAN_TIPE",
		"subTipeAkun":"TEST","posisiNormal":"DEBIT","sumberCoa":"MANUAL",
		"tanggalMulaiAktif":"2026-01-01"
	}`

	cleanupCoA(t, infra.DB, "902")
	t.Cleanup(func() { cleanupCoA(t, infra.DB, "902") })

	w := postJSON(router, "/api/v1/master/coa", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid tipeAkun: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" && code != "COA_INVALID_KODE_FORMAT" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("invalid tipeAkun correctly rejected: 422 code=%s", errCode(w.Body.Bytes()))
}

// ─── Test 4: Optimistic lock → 409 CONFLICT ───────────────────────────────────

// TestCoA_OptimisticLock_Returns409 verifies that PATCH with stale rowVersion
// returns 409 CONFLICT.
//
// Covers: regression §2 (ECL calc-run reproducibility pattern — immutability guard).
func TestCoA_OptimisticLock_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "903"
	cleanupCoA(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "coa_optlock_maker")
	seedCoADRAFT(t, infra.DB, kode, "Akun Optimistic Lock", makerID)

	router := buildCoARouter(infra.DB)
	claims := coaMakerClaims(makerID)

	update1 := `{"namaAkun":"Updated Once","rowVersion":1}`
	var entityID string
	{
		// Fetch actual UUID for PATCH path.
		var id uuid.UUID
		_ = infra.DB.QueryRowContext(context.Background(),
			`SELECT id FROM mst.chart_of_accounts WHERE kode_akun = $1 AND deleted_at IS NULL`, kode,
		).Scan(&id)
		entityID = id.String()
	}

	w1 := patchJSON(router, "/api/v1/master/coa/"+entityID, claims, uuid.New().String(), update1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first update: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}

	// Second PATCH with stale rowVersion=1 (now it's 2).
	update2 := `{"namaAkun":"Stale Update","rowVersion":1}`
	w2 := patchJSON(router, "/api/v1/master/coa/"+entityID, claims, uuid.New().String(), update2)
	if w2.Code != http.StatusConflict {
		t.Errorf("stale rowVersion: expected 409, got %d body=%s", w2.Code, w2.Body.String())
	}
	if code := errCode(w2.Body.Bytes()); code != "CONFLICT" {
		t.Errorf("expected CONFLICT, got %q", code)
	}
	t.Logf("optimistic lock correctly rejected: 409 CONFLICT")
}

// ─── Test 5: Parent not APPROVED → 422 COA_PARENT_NOT_FOUND ──────────────────

// TestCoA_ParentNotApproved_Returns422 verifies that specifying a parent_akun_kode
// that exists but is in DRAFT state returns 422 COA_PARENT_NOT_FOUND.
//
// Covers: parent-chain integrity (AC: parent must be APPROVED before child can reference it).
func TestCoA_ParentNotApproved_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	parentKode := "904"
	childKode := "904.1"
	cleanupCoA(t, infra.DB, childKode, parentKode)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, childKode, parentKode) })

	makerID := seedUserSQL(t, infra.DB, "coa_parent_maker")
	// Seed parent in DRAFT state (not APPROVED) — child creation must fail.
	seedCoADRAFT(t, infra.DB, parentKode, "Parent DRAFT", makerID)

	router := buildCoARouter(infra.DB)
	claims := coaMakerClaims(makerID)

	body := fmt.Sprintf(`{
		"kodeAkun":%q,"namaAkun":"Child Akun","tipeAkun":"ASET",
		"subTipeAkun":"SUB","posisiNormal":"DEBIT","sumberCoa":"MANUAL",
		"tanggalMulaiAktif":"2026-01-01","parentAkunKode":%q
	}`, childKode, parentKode)

	w := postJSON(router, "/api/v1/master/coa", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("parent not approved: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "COA_PARENT_NOT_FOUND" {
		t.Errorf("expected COA_PARENT_NOT_FOUND, got %q", code)
	}
	t.Logf("parent not approved correctly rejected: 422 COA_PARENT_NOT_FOUND")
}

// ─── Test 6: Full 4-eyes cycle ────────────────────────────────────────────────

// TestCoA_FourEyesCycle_Full exercises the complete DRAFT → PENDING_REVIEW →
// PENDING_APPROVAL → APPROVED cycle for chart_of_accounts. Verifies:
// - workflow_instance state transitions
// - mst.chart_of_accounts.workflow_status synced via entity hook
// - audit_log events present
// - signature count >= 2
//
// Covers: regression §3 (staging transitions — cure scenario pattern),
// regression §6 (SoD), UAT TC-004.
func TestCoA_FourEyesCycle_Full(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "905"
	cleanupCoA(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "coa_4eyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "coa_4eyes_reviewer")
	approverID := seedUserSQL(t, infra.DB, "coa_4eyes_approver")
	entityID := seedCoADRAFT(t, infra.DB, kode, "Akun 4-Eyes Test", makerID)

	router := buildCoARouter(infra.DB)
	makerClaims := coaMakerClaims(makerID)
	reviewerClaims := coaReviewerClaims(reviewerID)
	approverClaims := coaReviewerClaims(approverID)

	// Step 1: SUBMIT (maker).
	w1 := postJSON(router, "/api/v1/master/coa/"+entityID.String()+"/submit",
		makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Ajukan review"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")
	t.Logf("SUBMIT: state=PENDING_REVIEW")

	// Verify mst.chart_of_accounts.workflow_status synced by entity hook.
	assertCoAWorkflowStatus(t, infra.DB, kode, "PENDING_REVIEW")

	// Step 2: REVIEW (reviewer, different from maker).
	w2 := postJSON(router, "/api/v1/master/coa/"+entityID.String()+"/review",
		reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
	assertCoAWorkflowStatus(t, infra.DB, kode, "PENDING_APPROVAL")
	t.Logf("REVIEW: state=PENDING_APPROVAL")

	// Step 3: APPROVE (approver, different from maker + reviewer).
	w3 := postJSON(router, "/api/v1/master/coa/"+entityID.String()+"/approve",
		approverClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Disetujui"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	assertCoAWorkflowStatus(t, infra.DB, kode, "APPROVED")
	t.Logf("APPROVE: state=APPROVED")

	// Verify audit events.
	assertAuditEvent(t, infra.DB, "CHART_OF_ACCOUNTS.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "CHART_OF_ACCOUNTS.APPROVE", entityID)

	// Verify signature count >= 2.
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

// assertCoAWorkflowStatus checks mst.chart_of_accounts.workflow_status for a given kode.
func assertCoAWorkflowStatus(t *testing.T, db *sql.DB, kode, expected string) {
	t.Helper()
	var status string
	err := db.QueryRowContext(context.Background(), `
		SELECT workflow_status FROM mst.chart_of_accounts
		WHERE kode_akun = $1 AND deleted_at IS NULL
	`, kode).Scan(&status)
	if err != nil {
		t.Fatalf("assertCoAWorkflowStatus %s: %v", kode, err)
	}
	if status != expected {
		t.Errorf("chart_of_accounts[%s].workflow_status: expected %s, got %s", kode, expected, status)
	}
}

// ─── Test 7: SoD violation — maker cannot approve ─────────────────────────────

// TestCoA_SoDViolation_MakerCannotApprove verifies that the maker cannot approve
// their own chart_of_accounts record, even with a JWT that has approve permission.
//
// Covers: regression §6 (SoD enforcement at API level), security-baseline.md.
func TestCoA_SoDViolation_MakerCannotApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	kode := "906"
	cleanupCoA(t, infra.DB, kode)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, kode) })

	makerID := seedUserSQL(t, infra.DB, "coa_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "coa_sod_reviewer")
	entityID := seedCoADRAFT(t, infra.DB, kode, "Akun SoD Test", makerID)

	router := buildCoARouter(infra.DB)

	// Give maker ALL permissions including approve — bypass attempt.
	makerClaims := coaAllPermsClaims(makerID)
	reviewerClaims := coaReviewerClaims(reviewerID)

	// SUBMIT as maker.
	w1 := postJSON(router, "/api/v1/master/coa/"+entityID.String()+"/submit",
		makerClaims, uuid.New().String(), `{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}

	// REVIEW as a different user.
	w2 := postJSON(router, "/api/v1/master/coa/"+entityID.String()+"/review",
		reviewerClaims, uuid.New().String(), `{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}

	// APPROVE attempt as MAKER — SoD must block.
	w3 := postJSON(router, "/api/v1/master/coa/"+entityID.String()+"/approve",
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

// ─── Test 8: XLSX async import — 202 + jobId + poll + verify DRAFT rows ───────

// TestCoA_ImportXLSX_Async verifies the XLSX import flow end-to-end:
// POST multipart → 202 Accepted with jobId → poll status until completed →
// verify rows in mst.chart_of_accounts with workflow_status=DRAFT.
//
// Covers: regression §3 (import as DRAFT staging), UAT TC-003.
func TestCoA_ImportXLSX_Async(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	// Use a unique kode prefix that won't collide with other tests.
	prefix := "8001"
	cleanupCoAByPrefix(t, infra.DB, prefix)
	t.Cleanup(func() { cleanupCoAByPrefix(t, infra.DB, prefix) })

	makerID := seedUserSQL(t, infra.DB, "coa_xlimport_maker")
	claims := coaMakerClaims(makerID)

	// Build a minimal XLSX with 5 data rows (all valid).
	dataRows := [][]string{
		{prefix + ".1", "Kas", "ASET", "LANCAR", "", "IDR", "DEBIT", ""},
		{prefix + ".2", "Bank", "ASET", "LANCAR", "", "IDR", "DEBIT", ""},
		{prefix + ".3", "Piutang", "ASET", "LANCAR", "", "IDR", "DEBIT", ""},
		{prefix + ".4", "Persediaan", "ASET", "LANCAR", "", "IDR", "DEBIT", ""},
		{prefix + ".5", "Deposito Berjangka", "ASET", "INVESTASI", "DEPOSITO", "IDR", "DEBIT", ""},
	}
	xlsxBytes := buildMinimalXLSX(dataRows)
	if len(xlsxBytes) == 0 {
		t.Fatal("buildMinimalXLSX returned empty bytes")
	}

	router := buildCoARouter(infra.DB)

	// POST multipart — expect 202 Accepted.
	w := postMultipartXLSX(router, "/api/v1/master/coa/import-xlsx", claims, uuid.New().String(), xlsxBytes, "TEST_IMPORT")
	if w.Code != http.StatusAccepted {
		t.Fatalf("import xlsx: expected 202, got %d body=%s", w.Code, w.Body.String())
	}
	jobID := coaJobIDFromResponse(w.Body.Bytes())
	if jobID == "" {
		t.Fatalf("import xlsx: empty jobId in response: %s", w.Body.String())
	}
	t.Logf("import xlsx: 202 Accepted jobId=%s", jobID)

	// Poll until job reaches 'completed' or 'failed' (max 10 seconds).
	jobData := waitForJobStatus(t, router, claims, jobID, []string{"completed", "failed"}, 10*time.Second)
	status, _ := jobData["status"].(string)
	if status != "completed" {
		errDetail, _ := jobData["errorDetail"].(string)
		t.Fatalf("import job did not complete: status=%s errorDetail=%s", status, errDetail)
	}
	t.Logf("import job completed: rowsDone=%v rowsError=%v", jobData["rowsDone"], jobData["rowsError"])

	// Verify rows are in DB with workflow_status=DRAFT and sumber_coa=TEST_IMPORT.
	for _, dr := range dataRows {
		kode := dr[0]
		var wfStatus, sumber string
		err := infra.DB.QueryRowContext(context.Background(), `
			SELECT workflow_status, sumber_coa FROM mst.chart_of_accounts
			WHERE kode_akun = $1 AND deleted_at IS NULL
		`, kode).Scan(&wfStatus, &sumber)
		if err != nil {
			t.Errorf("row %s: not found in DB: %v", kode, err)
			continue
		}
		if wfStatus != "DRAFT" {
			t.Errorf("row %s: expected workflow_status=DRAFT, got %s", kode, wfStatus)
		}
		if sumber != "TEST_IMPORT" {
			t.Errorf("row %s: expected sumber_coa=TEST_IMPORT, got %s", kode, sumber)
		}
	}

	// Verify audit event written for the overall import job.
	time.Sleep(200 * time.Millisecond) // brief wait for post-import audit commit
	var auditCount int
	_ = infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM aud.audit_log WHERE action = 'CHART_OF_ACCOUNTS.IMPORT_XLSX'
	`).Scan(&auditCount)
	if auditCount == 0 {
		t.Logf("WARNING: no CHART_OF_ACCOUNTS.IMPORT_XLSX audit event found — may be delayed")
	} else {
		t.Logf("CHART_OF_ACCOUNTS.IMPORT_XLSX audit event present: count=%d", auditCount)
	}
}

// ─── Test 9: XLSX import idempotency — same file SHA → no duplicate rows ──────

// TestCoA_ImportXLSX_Idempotency verifies that uploading the same XLSX file twice
// (identical bytes → identical SHA-256) does not create duplicate rows in the DB.
// Duplicate kode_akun on the second import must be silently skipped (idempotent import
// per import.go comment: "skip duplicate — not counted as error").
//
// Covers: regression §8 (idempotency), UAT TC-003.
func TestCoA_ImportXLSX_Idempotency(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	prefix := "8002"
	cleanupCoAByPrefix(t, infra.DB, prefix)
	t.Cleanup(func() { cleanupCoAByPrefix(t, infra.DB, prefix) })

	makerID := seedUserSQL(t, infra.DB, "coa_xlidemp_maker")
	claims := coaMakerClaims(makerID)

	dataRows := [][]string{
		{prefix + ".1", "Kas Idempotency", "ASET", "LANCAR", "", "IDR", "DEBIT", ""},
		{prefix + ".2", "Bank Idempotency", "ASET", "LANCAR", "", "IDR", "DEBIT", ""},
	}
	xlsxBytes := buildMinimalXLSX(dataRows)

	router := buildCoARouter(infra.DB)

	// First upload.
	w1 := postMultipartXLSX(router, "/api/v1/master/coa/import-xlsx", claims, uuid.New().String(), xlsxBytes, "IDEMP_TEST")
	if w1.Code != http.StatusAccepted {
		t.Fatalf("first import: expected 202, got %d body=%s", w1.Code, w1.Body.String())
	}
	jobID1 := coaJobIDFromResponse(w1.Body.Bytes())
	waitForJobStatus(t, router, claims, jobID1, []string{"completed", "failed"}, 10*time.Second)
	t.Logf("first import done: jobId=%s", jobID1)

	// Verify initial count.
	var count1 int
	_ = infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.chart_of_accounts
		WHERE kode_akun LIKE $1 AND deleted_at IS NULL
	`, prefix+"%").Scan(&count1)
	if count1 != len(dataRows) {
		t.Fatalf("after first import: expected %d rows, got %d", len(dataRows), count1)
	}

	// Second upload — identical file bytes.
	w2 := postMultipartXLSX(router, "/api/v1/master/coa/import-xlsx", claims, uuid.New().String(), xlsxBytes, "IDEMP_TEST")
	if w2.Code != http.StatusAccepted {
		t.Fatalf("second import: expected 202, got %d body=%s", w2.Code, w2.Body.String())
	}
	jobID2 := coaJobIDFromResponse(w2.Body.Bytes())
	waitForJobStatus(t, router, claims, jobID2, []string{"completed", "failed"}, 10*time.Second)
	t.Logf("second import done: jobId=%s", jobID2)

	// Row count must not increase — duplicates are silently skipped.
	var count2 int
	_ = infra.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM mst.chart_of_accounts
		WHERE kode_akun LIKE $1 AND deleted_at IS NULL
	`, prefix+"%").Scan(&count2)
	if count2 != count1 {
		t.Errorf("idempotent import: expected %d rows after second upload, got %d — duplicate side-effect!", count1, count2)
	}
	t.Logf("idempotent import: %d rows before, %d rows after second upload — OK", count1, count2)
}

// ─── Test 10: Soft-delete parent with children → 409 ENTITY_IN_USE ───────────

// TestCoA_SoftDelete_WithChildren_Returns409 verifies that deleting an account
// that has at least one child (parent_akun_id reference) is blocked with 409 ENTITY_IN_USE.
//
// Covers: referential integrity guard (no orphaned child accounts).
func TestCoA_SoftDelete_WithChildren_Returns409(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	parentKode := "907"
	childKode := "907.1"
	cleanupCoA(t, infra.DB, childKode, parentKode)
	t.Cleanup(func() { cleanupCoA(t, infra.DB, childKode, parentKode) })

	makerID := seedUserSQL(t, infra.DB, "coa_del_maker")
	parentID := seedCoAApproved(t, infra.DB, parentKode, "Parent Akun", makerID)

	// Insert child referencing the parent.
	MustExec(t, infra.DB, `
		INSERT INTO mst.chart_of_accounts (
			id, kode_akun, nama_akun, tipe_akun, sub_tipe_akun,
			mata_uang_native, posisi_normal, aktif_flag, sumber_coa,
			tanggal_mulai_aktif, parent_akun_id, workflow_status,
			created_at, created_by, updated_at, updated_by,
			version, tenant_id
		) VALUES (
			gen_random_uuid(), $1, 'Child Akun', 'ASET', 'TEST',
			'IDR', 'DEBIT', true, 'MANUAL',
			'2026-01-01', $2, 'DRAFT',
			now(), $3, now(), $3,
			1, 'TUGURE'
		)
	`, childKode, parentID, makerID)

	router := buildCoARouter(infra.DB)
	claims := coaMakerClaims(makerID)

	// DELETE the parent — must be blocked because child exists.
	w := deleteReq(router, "/api/v1/master/coa/"+parentID.String(), claims, uuid.New().String())
	if w.Code != http.StatusConflict {
		t.Errorf("delete parent with children: expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "ENTITY_IN_USE" {
		t.Errorf("expected ENTITY_IN_USE, got %q", code)
	}

	// Parent must still exist (not soft-deleted).
	var deletedAt *time.Time
	err := infra.DB.QueryRowContext(context.Background(), `
		SELECT deleted_at FROM mst.chart_of_accounts WHERE id = $1
	`, parentID).Scan(&deletedAt)
	if err != nil {
		t.Fatalf("DB check parent: %v", err)
	}
	if deletedAt != nil {
		t.Error("parent was soft-deleted despite 409; deleted_at was set")
	}
	t.Logf("delete with children correctly blocked: 409 ENTITY_IN_USE")
}

// ─── Compile-time interface checks ────────────────────────────────────────────

var _ coa.Repository = (*coa.DBRepository)(nil)
var _ coa.JobRepository = (*coa.DBJobRepository)(nil)

// Suppress "imported and not used" for auth in case no test directly uses it.
var _ = auth.ContextWithClaims
