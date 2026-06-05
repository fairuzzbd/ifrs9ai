package instrumen_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/master/instrumen"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Test router helper ───────────────────────────────────────────────────────

func testClaims() *auth.Claims {
	return &auth.Claims{
		Sub:               testActorID.String(),
		PreferredUsername: "tr.maker",
		Roles:             []string{"ROLE-MAKER-TR"},
		Permissions: []string{
			"instrumen.read",
			"instrumen.create",
			"instrumen.update",
			"instrumen.delete",
			"instrumen.submit",
			"instrumen.review",
			"instrumen.approve",
			"instrumen.reject",
		},
		TenantID:    "TUGURE",
		MFAVerified: false,
	}
}

func newRouter(repo instrumen.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	claims := testClaims()
	r.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		// auth.RequirePermission reads from gin context key "claims"
		c.Set("claims", claims)
		c.Next()
	})

	auditWriter := audit.NewWriter(nil) // nil DB, best-effort

	wfCfg := workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())
	wfEngine := workflow.NewEngine(wfCfg)
	wfRepo := workflow.NewDBRepository(nil)
	wfSvc := workflow.NewService(wfEngine, wfRepo, auditWriter, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	svc := instrumen.NewService(repo, auditWriter, slog.Default())
	h := instrumen.NewHandler(svc, wfHandler)
	instrumen.RegisterRoutes(r.Group("/api/v1"), h)
	return r
}

func doRequest(r *gin.Engine, method, path string, body interface{}) *httptest.ResponseRecorder {
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseBody(w *httptest.ResponseRecorder) map[string]interface{} {
	var m map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func getErrorCode(w *httptest.ResponseRecorder) string {
	body := parseBody(w)
	if errObj, ok := body["error"].(map[string]interface{}); ok {
		if code, ok := errObj["code"].(string); ok {
			return code
		}
	}
	return ""
}

// ─── GET /master/instrumen tests ──────────────────────────────────────────────

func TestList_EmptyResult(t *testing.T) {
	repo := &stubRepo{listResult: nil}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestList_WithItems(t *testing.T) {
	repo := &stubRepo{listResult: []*instrumen.Instrumen{testInstrumen()}}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := parseBody(w)
	data, ok := body["data"].([]interface{})
	if !ok || len(data) == 0 {
		t.Fatal("expected non-empty data array")
	}
}

func TestList_RepoError(t *testing.T) {
	repo := &stubRepo{listErr: fmt.Errorf("db error")}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen", nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// ─── GET /master/instrumen/:id tests ──────────────────────────────────────────

func TestGetByID_Found(t *testing.T) {
	repo := &stubRepo{getByIDResult: testInstrumen()}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen/"+testInstrumenID.String(), nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetByID_NotFound(t *testing.T) {
	repo := &stubRepo{getByIDResult: nil}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen/"+testInstrumenID.String(), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if got := getErrorCode(w); got != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %q", got)
	}
}

func TestGetByID_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if got := getErrorCode(w); got != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED, got %q", got)
	}
}

// ─── POST /master/instrumen tests ─────────────────────────────────────────────

func TestCreate_DraftResult(t *testing.T) {
	repo := approvedStub()
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-TEST-001",
		"tipeInstrumen":     "DEPOSITO",
		"subTipe":           "Deposito Berjangka",
		"nama":              "Test Deposito",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "5000000000.00",
		"tanggalPenempatan": "2026-01-15",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	// Service will fail at BeginTx (no real DB) but validation passes before that.
	// Test verifies validation → expected either 201 or 500 from tx failure (not 400/422).
	if w.Code == http.StatusBadRequest || w.Code == http.StatusUnprocessableEntity {
		t.Fatalf("unexpected validation error %d: %s", w.Code, w.Body.String())
	}
}

func TestCreate_InvalidTipe(t *testing.T) {
	repo := approvedStub()
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-X",
		"tipeInstrumen":     "INVALID_TYPE",
		"subTipe":           "X",
		"nama":              "Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "1000000",
		"tanggalPenempatan": "2026-01-01",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED for invalid tipe, got %q", got)
	}
}

func TestCreate_SahamMissingKustodian(t *testing.T) {
	repo := approvedStub()
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "SAHAM-001",
		"tipeInstrumen":     "SAHAM",
		"subTipe":           "Saham Biasa",
		"nama":              "Saham Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "1000000000",
		"tanggalPenempatan": "2026-01-01",
		// bankKustodianId intentionally omitted
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED for missing kustodian, got %q", got)
	}
}

func TestCreate_ReksadanaMissingManajer(t *testing.T) {
	repo := approvedStub()
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "RD-001",
		"tipeInstrumen":     "REKSADANA",
		"subTipe":           "Reksadana Campuran",
		"nama":              "RD Test",
		"counterpartyId":    testCounterpartyID.String(),
		"bankKustodianId":   testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "500000000",
		"tanggalPenempatan": "2026-01-01",
		// manajerInvestasiId intentionally omitted
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED for missing manajer, got %q", got)
	}
}

func TestCreate_CounterpartyNotApproved(t *testing.T) {
	repo := &stubRepo{
		checkCPApprovedResult:       false, // counterparty NOT approved
		checkPortoApprovedResult:    true,
		checkMataUangApprovedResult: true,
	}
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-CP-001",
		"tipeInstrumen":     "DEPOSITO",
		"subTipe":           "Deposito",
		"nama":              "Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "1000000000",
		"tanggalPenempatan": "2026-01-01",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != string(domainerrors.CodeInstrumenCounterpartyNotApproved) {
		t.Fatalf("expected INSTRUMEN_COUNTERPARTY_NOT_APPROVED, got %q", got)
	}
}

func TestCreate_PortofolioNotApproved(t *testing.T) {
	repo := &stubRepo{
		checkCPApprovedResult:       true,
		checkPortoApprovedResult:    false, // portofolio NOT approved
		checkMataUangApprovedResult: true,
	}
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-PO-001",
		"tipeInstrumen":     "DEPOSITO",
		"subTipe":           "Deposito",
		"nama":              "Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "1000000000",
		"tanggalPenempatan": "2026-01-01",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != string(domainerrors.CodeInstrumenPortofolioNotApproved) {
		t.Fatalf("expected INSTRUMEN_PORTOFOLIO_NOT_APPROVED, got %q", got)
	}
}

func TestCreate_MataUangNotApproved(t *testing.T) {
	repo := &stubRepo{
		checkCPApprovedResult:       true,
		checkPortoApprovedResult:    true,
		checkMataUangApprovedResult: false, // mata_uang NOT approved
	}
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-MU-001",
		"tipeInstrumen":     "DEPOSITO",
		"subTipe":           "Deposito",
		"nama":              "Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "USD",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "1000000",
		"tanggalPenempatan": "2026-01-01",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != string(domainerrors.CodeInstrumenMataUangNotApproved) {
		t.Fatalf("expected INSTRUMEN_MATA_UANG_NOT_APPROVED, got %q", got)
	}
}

func TestCreate_InvalidNominal(t *testing.T) {
	repo := approvedStub()
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-NOM",
		"tipeInstrumen":     "DEPOSITO",
		"subTipe":           "D",
		"nama":              "Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "-1000000", // NEGATIVE
		"tanggalPenempatan": "2026-01-01",
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED, got %q", got)
	}
}

func TestCreate_InvalidEirRange(t *testing.T) {
	repo := approvedStub()
	r := newRouter(repo)
	body := map[string]interface{}{
		"kodeInstrumen":     "INST-EIR",
		"tipeInstrumen":     "OBLIGASI",
		"subTipe":           "Obligasi Korporasi",
		"nama":              "Obligasi Test",
		"counterpartyId":    testCounterpartyID.String(),
		"mataUang":          "IDR",
		"portofolioId":      testPortofolioID.String(),
		"nominal":           "1000000000",
		"tanggalPenempatan": "2026-01-01",
		"eirAwal":           "1.5", // INVALID — must be < 1
	}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", body)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400/422, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "VALIDATION_FAILED" {
		t.Fatalf("expected VALIDATION_FAILED, got %q", got)
	}
}

func TestCreate_MissingRequiredFields(t *testing.T) {
	repo := &stubRepo{}
	r := newRouter(repo)
	// Empty body → should fail binding
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen", map[string]interface{}{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── PUT /master/instrumen/:id tests ──────────────────────────────────────────

func TestUpdate_NotFound(t *testing.T) {
	repo := &stubRepo{getByIDResult: nil}
	r := newRouter(repo)
	body := map[string]interface{}{"rowVersion": 1}
	w := doRequest(r, http.MethodPut, "/api/v1/master/instrumen/"+testInstrumenID.String(), body)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdate_ApprovedRecord(t *testing.T) {
	repo := &stubRepo{getByIDResult: testApprovedInstrumen()}
	r := newRouter(repo)
	body := map[string]interface{}{
		"nama":       "Updated Name",
		"rowVersion": 1,
	}
	w := doRequest(r, http.MethodPut, "/api/v1/master/instrumen/"+testInstrumenID.String(), body)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "MASTER_APPROVED_NO_EDIT" {
		t.Fatalf("expected MASTER_APPROVED_NO_EDIT, got %q", got)
	}
}

func TestUpdate_KlasifikasiLocked(t *testing.T) {
	repo := &stubRepo{getByIDResult: testLockedInstrumen()}
	r := newRouter(repo)
	bmCat := "HTC"
	body := map[string]interface{}{
		"bmCategory": bmCat,
		"rowVersion": 1,
	}
	w := doRequest(r, http.MethodPut, "/api/v1/master/instrumen/"+testInstrumenID.String(), body)
	if w.Code != 423 {
		t.Fatalf("expected 423 Locked, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "INSTRUMEN_KLASIFIKASI_LOCKED" {
		t.Fatalf("expected INSTRUMEN_KLASIFIKASI_LOCKED, got %q", got)
	}
}

func TestUpdate_FvociElectionOnLockedRecord(t *testing.T) {
	repo := &stubRepo{getByIDResult: testLockedInstrumen()}
	r := newRouter(repo)
	fvoci := true
	body := map[string]interface{}{
		"fvociElection": fvoci,
		"rowVersion":    1,
	}
	w := doRequest(r, http.MethodPut, "/api/v1/master/instrumen/"+testInstrumenID.String(), body)
	if w.Code != 423 {
		t.Fatalf("expected 423, got %d", w.Code)
	}
}

func TestUpdate_MissingRowVersion(t *testing.T) {
	repo := &stubRepo{getByIDResult: testInstrumen()}
	r := newRouter(repo)
	body := map[string]interface{}{"nama": "New Name"}
	// rowVersion missing → binding may or may not fail; service should reject
	w := doRequest(r, http.MethodPut, "/api/v1/master/instrumen/"+testInstrumenID.String(), body)
	// Accept either 400 (binding) or 422 (validation) or 500 (tx fails)
	if w.Code == http.StatusOK {
		t.Fatal("should not succeed without rowVersion")
	}
}

func TestUpdate_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	r := newRouter(repo)
	body := map[string]interface{}{"rowVersion": 1}
	w := doRequest(r, http.MethodPut, "/api/v1/master/instrumen/bad-uuid", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── DELETE /master/instrumen/:id tests ───────────────────────────────────────

func TestDelete_NotFound(t *testing.T) {
	repo := &stubRepo{getByIDResult: nil}
	r := newRouter(repo)
	w := doRequest(r, http.MethodDelete, "/api/v1/master/instrumen/"+testInstrumenID.String(), nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDelete_EntityInUse(t *testing.T) {
	repo := &stubRepo{
		getByIDResult:    testInstrumen(),
		countActiveTxVal: 3, // 3 active transactions
	}
	r := newRouter(repo)
	w := doRequest(r, http.MethodDelete, "/api/v1/master/instrumen/"+testInstrumenID.String(), nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 ENTITY_IN_USE, got %d: %s", w.Code, w.Body.String())
	}
	if got := getErrorCode(w); got != "ENTITY_IN_USE" {
		t.Fatalf("expected ENTITY_IN_USE, got %q", got)
	}
}

func TestDelete_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	r := newRouter(repo)
	w := doRequest(r, http.MethodDelete, "/api/v1/master/instrumen/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── GET /master/instrumen/:id/history tests ──────────────────────────────────

func TestHistory_NotFound(t *testing.T) {
	repo := &stubRepo{getByIDResult: nil}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen/"+testInstrumenID.String()+"/history", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHistory_ReturnsItems(t *testing.T) {
	repo := &stubRepo{
		getByIDResult: testInstrumen(),
		auditItems: []instrumen.AuditHistoryItem{
			{EventID: uuid.New().String(), Action: "INSTRUMEN.CREATE"},
		},
	}
	r := newRouter(repo)
	w := doRequest(r, http.MethodGet, "/api/v1/master/instrumen/"+testInstrumenID.String()+"/history", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ─── Workflow action tests ─────────────────────────────────────────────────────

func TestSubmit_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	r := newRouter(repo)
	body := map[string]interface{}{"signatureMethod": "JWT_STEP_UP"}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen/not-a-uuid/submit", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApprove_InvalidUUID(t *testing.T) {
	repo := &stubRepo{}
	r := newRouter(repo)
	body := map[string]interface{}{"signatureMethod": "JWT_STEP_UP"}
	w := doRequest(r, http.MethodPost, "/api/v1/master/instrumen/not-a-uuid/approve", body)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ─── Service unit tests (bypasses HTTP) ───────────────────────────────────────

func TestService_Create_AllowedTipeVariants(t *testing.T) {
	tipes := []string{"DEPOSITO", "OBLIGASI", "SBN", "SPN", "SUKUK"}
	for _, tipe := range tipes {
		t.Run(tipe, func(t *testing.T) {
			repo := approvedStub()
			svc := newTestService(repo)
			req := baseCreateRequest()
			req.TipeInstrumen = tipe
			_, err := svc.Create(ctxWithClaims(), req)
			// Should NOT fail with INSTRUMEN_INVALID_TIPE
			if err != nil {
				de, ok := domainerrors.IsDomainError(err)
				if ok && de.Code() == domainerrors.CodeInstrumenInvalidTipe {
					t.Errorf("tipe %s should be valid, got INSTRUMEN_INVALID_TIPE", tipe)
				}
				// Other errors (e.g. tx fail) are acceptable in unit tests
			}
		})
	}
}

func TestService_Create_SahamWithKustodian(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	req.TipeInstrumen = "SAHAM"
	kustodian := testCounterpartyID.String()
	req.BankKustodianID = &kustodian
	_, err := svc.Create(ctxWithClaims(), req)
	// Should not fail with INSTRUMEN_MISSING_KUSTODIAN
	if err != nil {
		de, ok := domainerrors.IsDomainError(err)
		if ok && de.Code() == domainerrors.CodeInstrumenMissingKustodian {
			t.Error("SAHAM with bankKustodianId should not fail with MISSING_KUSTODIAN")
		}
	}
}

func TestService_Create_ReksadanaWithBothFKs(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	req.TipeInstrumen = "REKSADANA"
	kustodian := testCounterpartyID.String()
	req.BankKustodianID = &kustodian
	manajer := testCounterpartyID.String()
	req.ManajerInvestasiID = &manajer
	_, err := svc.Create(ctxWithClaims(), req)
	if err != nil {
		de, ok := domainerrors.IsDomainError(err)
		if ok && (de.Code() == domainerrors.CodeInstrumenMissingKustodian ||
			de.Code() == domainerrors.CodeValidationFailed) {
			t.Errorf("REKSADANA with both FKs should not fail with domain validation, got: %v", err)
		}
	}
}

func TestService_Update_KlasifikasiLockedRejectsFvoci(t *testing.T) {
	repo := &stubRepo{
		getByIDResult:           testLockedInstrumen(),
		checkMataUangApprovedResult: true,
	}
	svc := newTestService(repo)
	fvoci := true
	req := instrumen.UpdateRequest{
		FvociElection: &fvoci,
		RowVersion:    1,
	}
	_, err := svc.Update(ctxWithClaims(), testInstrumenID, req)
	if err == nil {
		t.Fatal("expected error for locked klasifikasi, got nil")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeInstrumenKlasifikasiLocked {
		t.Fatalf("expected INSTRUMEN_KLASIFIKASI_LOCKED, got %v", err)
	}
}

func TestService_Update_KlasifikasiLockedRejectsBmCategory(t *testing.T) {
	repo := &stubRepo{getByIDResult: testLockedInstrumen()}
	svc := newTestService(repo)
	cat := "HTC"
	req := instrumen.UpdateRequest{
		BmCategory: &cat,
		RowVersion: 1,
	}
	_, err := svc.Update(ctxWithClaims(), testInstrumenID, req)
	if err == nil {
		t.Fatal("expected INSTRUMEN_KLASIFIKASI_LOCKED")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeInstrumenKlasifikasiLocked {
		t.Fatalf("expected INSTRUMEN_KLASIFIKASI_LOCKED, got %v", err)
	}
}

func TestService_Update_KlasifikasiLockedAllowsOtherFields(t *testing.T) {
	locked := testLockedInstrumen()
	updated := *locked
	updated.Nama = "New Name"
	repo := &stubRepo{
		getByIDResult:           locked,
		updateResult:            &updated,
		checkMataUangApprovedResult: true,
	}
	svc := newTestService(repo)
	newNama := "New Name"
	req := instrumen.UpdateRequest{
		Nama:       &newNama,
		RowVersion: 1,
	}
	_, err := svc.Update(ctxWithClaims(), testInstrumenID, req)
	// Should not fail with KLASIFIKASI_LOCKED (only fvoci/bm changes are blocked)
	if err != nil {
		de, ok := domainerrors.IsDomainError(err)
		if ok && de.Code() == domainerrors.CodeInstrumenKlasifikasiLocked {
			t.Errorf("updating nama on locked record should not fail with KLASIFIKASI_LOCKED")
		}
		// Other errors (tx fail) are ok in unit test
	}
}

func TestService_SoftDelete_EntityInUse(t *testing.T) {
	repo := &stubRepo{
		getByIDResult:    testInstrumen(),
		countActiveTxVal: 2,
	}
	svc := newTestService(repo)
	err := svc.SoftDelete(ctxWithClaims(), testInstrumenID)
	if err == nil {
		t.Fatal("expected ENTITY_IN_USE error")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeEntityInUse {
		t.Fatalf("expected ENTITY_IN_USE, got %v", err)
	}
}

func TestService_SoftDelete_NotFound(t *testing.T) {
	repo := &stubRepo{getByIDResult: nil}
	svc := newTestService(repo)
	err := svc.SoftDelete(ctxWithClaims(), testInstrumenID)
	if err == nil {
		t.Fatal("expected NOT_FOUND error")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestService_GetByID_NotFound(t *testing.T) {
	repo := &stubRepo{getByIDResult: nil}
	svc := newTestService(repo)
	_, err := svc.GetByID(ctxWithClaims(), testInstrumenID, false)
	if err == nil {
		t.Fatal("expected NOT_FOUND")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestService_Create_InvalidNominalZero(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	req.Nominal = "0"
	_, err := svc.Create(ctxWithClaims(), req)
	if err == nil {
		t.Fatal("expected error for zero nominal")
	}
	de, ok := domainerrors.IsDomainError(err)
	if !ok || de.Code() != domainerrors.CodeValidationFailed {
		t.Fatalf("expected VALIDATION_FAILED, got %v", err)
	}
}

func TestService_Create_InvalidNominalNegative(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	req.Nominal = "-500000"
	_, err := svc.Create(ctxWithClaims(), req)
	if err == nil {
		t.Fatal("expected error for negative nominal")
	}
}

func TestService_Create_EirAtExactlyZero_Valid(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	req.TipeInstrumen = "OBLIGASI"
	eir := "0"
	req.EirAwal = &eir
	_, err := svc.Create(ctxWithClaims(), req)
	// eir=0 is valid (range [0, 1))
	if err != nil {
		de, ok := domainerrors.IsDomainError(err)
		if ok && de.Code() == domainerrors.CodeValidationFailed {
			t.Errorf("eir=0 should be valid, got VALIDATION_FAILED: %v", err)
		}
	}
}

func TestService_Create_EirAtOne_Invalid(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	req.TipeInstrumen = "OBLIGASI"
	eir := "1.0"
	req.EirAwal = &eir
	_, err := svc.Create(ctxWithClaims(), req)
	if err == nil {
		t.Fatal("expected error for eir=1.0 (must be < 1)")
	}
}

func TestService_Create_NegativeKupon_Invalid(t *testing.T) {
	repo := approvedStub()
	svc := newTestService(repo)
	req := baseCreateRequest()
	kupon := "-0.05"
	req.Kupon = &kupon
	_, err := svc.Create(ctxWithClaims(), req)
	if err == nil {
		t.Fatal("expected error for negative kupon")
	}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestService(repo instrumen.Repository) *instrumen.Service {
	return instrumen.NewService(repo, audit.NewWriter(nil), slog.Default())
}

func ctxWithClaims() context.Context {
	return auth.ContextWithClaims(context.Background(), testClaims())
}

func baseCreateRequest() instrumen.CreateRequest {
	return instrumen.CreateRequest{
		KodeInstrumen:     "TEST-001",
		TipeInstrumen:     "DEPOSITO",
		SubTipe:           "Deposito Berjangka",
		Nama:              "Test Deposito BCA",
		CounterpartyID:    testCounterpartyID.String(),
		MataUang:          "IDR",
		PortofolioID:      testPortofolioID.String(),
		Nominal:           "1000000000.00",
		TanggalPenempatan: "2026-01-15",
	}
}
