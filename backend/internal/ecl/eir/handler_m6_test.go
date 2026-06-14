// handler_m6_test.go — HTTP handler tests for 8 new M6 endpoints (P4-M6).
//
// Tests cover: permission checks (403), JSON parse (400), success (200/201/202),
// and domain-error mapping for DetectAmendment, CancelAmendment, UpdateCashflows,
// ListAmendmentQueue, ExportAmendmentQueue, ListDriftReports, GetDriftReport,
// GenerateDriftReport, and NewHandlerM6 constructor.
//
// Uses the same stub+router pattern as handler_test.go.
// DEC-016: no float64 in assertions.
package eir

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ─── M6 handler router builder ────────────────────────────────────────────────

// buildRouterM6 sets up a Gin router with all M6 handler methods registered.
// Detection service and drift service use stub deps.
func buildRouterM6(h *Handler, perms []string) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", uuid.New().String())
		c.Set("role", "ROLE-RISK")
		c.Set("permissions", perms)
		c.Set("mfa_verified", false)
		c.Set("claims", makeClaims(perms, false))
		c.Next()
	})
	v1 := r.Group("/api/v1/ecl/eir")
	// M6 routes (order: static before :id to avoid routing conflicts)
	v1.POST("/amendments/detect", h.DetectAmendment)
	v1.GET("/amendments/queue", h.ListAmendmentQueue)
	v1.GET("/amendments/queue/export", h.ExportAmendmentQueue)
	v1.POST("/amendments/:id/cancel", h.CancelAmendment)
	v1.PATCH("/amendments/:id/cashflows", h.UpdateCashflows)
	v1.POST("/drift-reports/generate", h.GenerateDriftReport)
	v1.GET("/drift-reports", h.ListDriftReports)
	v1.GET("/drift-reports/:id", h.GetDriftReport)
	return r
}

// allM6Perms returns all M6-related permissions.
func allM6Perms() []string {
	return []string{
		PermEIRAmendDetect,
		PermEIRAmendCancel,
		PermEIRAmendUpdateCF,
		PermEIRAmendReviewRead,
		PermEIRDriftReportRead,
		PermEIRDriftGenerate,
	}
}

// stubListQueueAmendRepo wraps stubAmendmentRepo + provides seeded queue rows.
type stubListQueueAmendRepo struct {
	*stubAmendmentRepo
	queueRows []QueueRow
}

func newStubListQueueAmendRepo() *stubListQueueAmendRepo {
	return &stubListQueueAmendRepo{
		stubAmendmentRepo: newStubAmendmentRepo(),
		queueRows:         nil,
	}
}

func (r *stubListQueueAmendRepo) ListQueue(_ context.Context, _ listquery.Query, _ string, limit int) ([]QueueRow, *response.PaginationMeta, error) {
	return r.queueRows, &response.PaginationMeta{Limit: limit}, nil
}

// ─── buildHandlerM6 ───────────────────────────────────────────────────────────

// buildHandlerM6 constructs a Handler with M6 services using stubs.
// The db parameter is used only by DetectionService/DriftService for BeginTx.
func buildHandlerM6(db *sql.DB, instrRepo *stubInstrumenRepo, amendRepo *stubListQueueAmendRepo) *Handler {
	auditW := &stubAuditWriter{}
	schedRepo := &stubScheduleRepo{}
	driftRepo := newStubDriftRepo()

	eirSvc := &Service{
		instrRepo:   instrRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}
	schedSvc := &ScheduleService{
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}
	amendSvc := &AmendmentService{
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}
	bulkSvc := NewBulkService(nil, instrRepo, schedRepo, nil, nil, testLogger())

	detectionSvc := NewDetectionService(db, instrRepo, amendRepo, auditW, testLogger())
	driftSvc := NewDriftService(db, &driftInstrRepo{}, schedRepo, amendRepo, driftRepo, NewSolver(), auditW, testLogger())

	return NewHandlerM6(eirSvc, schedSvc, amendSvc, bulkSvc, detectionSvc, driftSvc)
}

// ─── NewHandlerM6 constructor ─────────────────────────────────────────────────

func TestNewHandlerM6_NotNil(t *testing.T) {
	auditW := &stubAuditWriter{}
	db, _, _ := sqlmock.New()
	defer db.Close()

	instrRepo := newStubInstrumenRepo()
	amendRepo := newStubAmendmentRepo()
	schedRepo := &stubScheduleRepo{}

	eirSvc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()}
	schedSvc := &ScheduleService{instrRepo: instrRepo, schedRepo: schedRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()}
	amendSvc := &AmendmentService{instrRepo: instrRepo, schedRepo: schedRepo, amendRepo: amendRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()}
	bulkSvc := NewBulkService(nil, instrRepo, schedRepo, nil, nil, testLogger())
	driftRepo := newStubDriftRepo()

	detectionSvc := NewDetectionService(db, instrRepo, amendRepo, auditW, testLogger())
	driftSvc := NewDriftService(db, &driftInstrRepo{}, schedRepo, amendRepo, driftRepo, NewSolver(), auditW, testLogger())

	h := NewHandlerM6(eirSvc, schedSvc, amendSvc, bulkSvc, detectionSvc, driftSvc)
	if h == nil {
		t.Fatal("NewHandlerM6 returned nil")
	}
	if h.detectionSvc == nil {
		t.Error("detectionSvc must be wired")
	}
	if h.driftSvc == nil {
		t.Error("driftSvc must be wired")
	}
}

// ─── DetectAmendment ──────────────────────────────────────────────────────────

func TestHandler_DetectAmendment_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{}) // no perms

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/detect", map[string]interface{}{
		"instrumenId": uuid.New().String(),
		"documentId":  uuid.New().String(),
	})
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_DetectAmendment_BadJSON_400(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	// Missing required fields (instrumenId, documentId)
	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/detect", map[string]interface{}{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_DetectAmendment_InvalidUUID_400(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/detect", map[string]interface{}{
		"instrumenId":    "not-a-uuid",
		"documentId":     uuid.New().String(),
		"alasanDetected": "Test",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid instrumenId, got %d", w.Code)
	}
}

func TestHandler_DetectAmendment_NotFound_404(t *testing.T) {
	// instrRepo has no instrument → service returns EIR_INSTRUMEN_NOT_FOUND → 404
	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	instrRepo := newStubInstrumenRepo()
	h := buildHandlerM6(db, instrRepo, newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/detect", map[string]interface{}{
		"instrumenId":    uuid.New().String(),
		"documentId":     uuid.New().String(),
		"alasanDetected": "Test",
	})
	// EIR_INSTRUMEN_NOT_FOUND → 404
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_DetectAmendment_Success_201(t *testing.T) {
	instrID := uuid.New()
	eirVal := decimal.NewFromFloat(0.08)
	instrRepo := newStubInstrumenRepo()
	instrRepo.put(InstrumenForEIR{
		ID: instrID, KodeInstrumen: "BOND-001",
		KlasifikasiPsak71: "AC", EIRMethodFlag: true,
		EIRAwal: &eirVal, Status: "ACTIVE", TenantID: "TUGURE",
	})
	amendRepo := newStubListQueueAmendRepo()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	h := buildHandlerM6(db, instrRepo, amendRepo)
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "POST", "/api/v1/ecl/eir/amendments/detect", map[string]interface{}{
		"instrumenId":    instrID.String(),
		"documentId":     uuid.New().String(),
		"alasanDetected": "Kontrak diamandemen",
	})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── CancelAmendment ──────────────────────────────────────────────────────────

func TestHandler_CancelAmendment_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{}) // no perms

	w := doRequest(r, "POST",
		fmt.Sprintf("/api/v1/ecl/eir/amendments/%s/cancel", uuid.New()),
		map[string]interface{}{"cancelReason": "Test reason long enough"})
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_CancelAmendment_InvalidUUID_400(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "POST",
		"/api/v1/ecl/eir/amendments/not-a-uuid/cancel",
		map[string]interface{}{"cancelReason": "Test reason long enough"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandler_CancelAmendment_ReasonTooShort_422(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubListQueueAmendRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db, _, _ := sqlmock.New()
	defer db.Close()

	h := buildHandlerM6(db, newStubInstrumenRepo(), amendRepo)
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "POST",
		fmt.Sprintf("/api/v1/ecl/eir/amendments/%s/cancel", proposal.ID),
		map[string]interface{}{"cancelReason": "short"})
	// EIR_AMENDMENT_CANCEL_REASON_TOO_SHORT → 422
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_CancelAmendment_Success_200(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubListQueueAmendRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	instrRepo := newStubInstrumenRepo()

	// The handler reads actorID from context user_id. We need it to match makerID.
	// Override router to inject makerID as user_id.
	auditW := &stubAuditWriter{}
	detectionSvc := NewDetectionService(db, instrRepo, amendRepo, auditW, testLogger())
	driftSvc := NewDriftService(db, &driftInstrRepo{}, &stubScheduleRepo{}, amendRepo, newStubDriftRepo(), NewSolver(), auditW, testLogger())
	h := NewHandlerM6(
		&Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()},
		&ScheduleService{instrRepo: instrRepo, schedRepo: &stubScheduleRepo{}, solver: NewSolver(), auditWriter: auditW, logger: testLogger()},
		&AmendmentService{instrRepo: instrRepo, schedRepo: &stubScheduleRepo{}, amendRepo: amendRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()},
		NewBulkService(nil, instrRepo, &stubScheduleRepo{}, nil, nil, testLogger()),
		detectionSvc, driftSvc,
	)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Inject makerID as user_id so SoD check passes.
		c.Set("user_id", makerID.String())
		c.Set("role", "ROLE-RISK")
		c.Set("permissions", allM6Perms())
		c.Set("mfa_verified", false)
		c.Set("claims", makeClaims(allM6Perms(), false))
		c.Next()
	})
	v1 := r.Group("/api/v1/ecl/eir")
	v1.POST("/amendments/:id/cancel", h.CancelAmendment)

	w := doRequest(r, "POST",
		fmt.Sprintf("/api/v1/ecl/eir/amendments/%s/cancel", proposal.ID),
		map[string]interface{}{"cancelReason": "Pembatalan karena perubahan regulasi baru"})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── UpdateCashflows ──────────────────────────────────────────────────────────

func TestHandler_UpdateCashflows_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{})

	w := doRequest(r, "PATCH",
		fmt.Sprintf("/api/v1/ecl/eir/amendments/%s/cashflows", uuid.New()),
		map[string]interface{}{"revisedCashflows": cashflowJSONItems()})
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_UpdateCashflows_InvalidUUID_400(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "PATCH",
		"/api/v1/ecl/eir/amendments/not-a-uuid/cashflows",
		map[string]interface{}{"revisedCashflows": cashflowJSONItems()})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandler_UpdateCashflows_Success_200(t *testing.T) {
	makerID := uuid.New()
	proposal := makeProposalM6(uuid.New(), AmendStatusDraft, makerID)
	amendRepo := newStubListQueueAmendRepo()
	amendRepo.proposals[proposal.ID] = proposal

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE ecl.eir_reestimation_log")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	instrRepo := newStubInstrumenRepo()
	auditW := &stubAuditWriter{}
	detectionSvc := NewDetectionService(db, instrRepo, amendRepo, auditW, testLogger())
	driftSvc := NewDriftService(db, &driftInstrRepo{}, &stubScheduleRepo{}, amendRepo, newStubDriftRepo(), NewSolver(), auditW, testLogger())
	h := NewHandlerM6(
		&Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()},
		&ScheduleService{instrRepo: instrRepo, schedRepo: &stubScheduleRepo{}, solver: NewSolver(), auditWriter: auditW, logger: testLogger()},
		&AmendmentService{instrRepo: instrRepo, schedRepo: &stubScheduleRepo{}, amendRepo: amendRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()},
		NewBulkService(nil, instrRepo, &stubScheduleRepo{}, nil, nil, testLogger()),
		detectionSvc, driftSvc,
	)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", makerID.String())
		c.Set("role", "ROLE-RISK")
		c.Set("permissions", allM6Perms())
		c.Set("mfa_verified", false)
		c.Set("claims", makeClaims(allM6Perms(), false))
		c.Next()
	})
	r.PATCH("/api/v1/ecl/eir/amendments/:id/cashflows", h.UpdateCashflows)

	w := doRequest(r, "PATCH",
		fmt.Sprintf("/api/v1/ecl/eir/amendments/%s/cashflows", proposal.ID),
		map[string]interface{}{"revisedCashflows": cashflowJSONItems()})
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── ListAmendmentQueue ───────────────────────────────────────────────────────

func TestHandler_ListAmendmentQueue_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{})

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/queue", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_ListAmendmentQueue_EmptyList_200(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/queue", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_ListAmendmentQueue_WithRows_200(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	amendID := uuid.New()
	eirVal := decimal.NewFromFloat(0.08)
	amendRepo := newStubListQueueAmendRepo()
	amendRepo.queueRows = []QueueRow{
		{
			AmendmentID:      amendID,
			InstrumenID:      instrID,
			KodeInstrumen:    "BOND-Q001",
			Status:           AmendStatusPendingReview,
			TriggerSource:    AmendTriggerDocumentUpload,
			TanggalAmandemen: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			EIRLama:          &eirVal,
			MakerID:          &makerID,
			CreatedAt:        time.Now(),
		},
	}

	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), amendRepo)
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/queue", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── ExportAmendmentQueue ─────────────────────────────────────────────────────

func TestHandler_ExportAmendmentQueue_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{})

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/queue/export", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_ExportAmendmentQueue_Success_202(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET", "/api/v1/ecl/eir/amendments/queue/export", nil)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── ListDriftReports ─────────────────────────────────────────────────────────

func TestHandler_ListDriftReports_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{})

	w := doRequest(r, "GET", "/api/v1/ecl/eir/drift-reports", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_ListDriftReports_EmptyList_200(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET", "/api/v1/ecl/eir/drift-reports", nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandler_ListDriftReports_BadSort_400 triggers the parseErr != nil branch
// by passing a sort column that is not in AllowedColsDriftReport.
func TestHandler_ListDriftReports_BadSort_400(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET", "/api/v1/ecl/eir/drift-reports?sort=invalid_col:asc", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad sort col, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GetDriftReport ───────────────────────────────────────────────────────────

func TestHandler_GetDriftReport_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{})

	w := doRequest(r, "GET",
		fmt.Sprintf("/api/v1/ecl/eir/drift-reports/%s", uuid.New()), nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_GetDriftReport_InvalidUUID_400(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET", "/api/v1/ecl/eir/drift-reports/not-a-uuid", nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandler_GetDriftReport_NotFound_404(t *testing.T) {
	// driftRepo stub has no reports → GetReport returns 404.
	db, mock, _ := sqlmock.New()
	defer db.Close()
	// GetReport calls db.QueryRowContext after checking driftRepo — but since
	// driftRepo.GetByID returns nil, the service returns 404 before QueryRowContext.
	_ = mock

	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET",
		fmt.Sprintf("/api/v1/ecl/eir/drift-reports/%s", uuid.New()), nil)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandler_GetDriftReport_Success_200(t *testing.T) {
	reportID := uuid.New()
	// Need to seed a report in the stub driftRepo used by driftSvc.
	// The driftSvc is built inside buildHandlerM6. We need to wire a custom driftRepo.

	db, mock, _ := sqlmock.New()
	defer db.Close()

	// GetReport: driftRepo.GetByID finds report → db.QueryRowContext for entries.
	// Mock the QueryRowContext for the entries JSON query.
	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(reportID).
		WillReturnRows(sqlmock.NewRows([]string{"drift", "missing", "error"}).
			AddRow("[]", "[]", "[]"))

	instrRepo := newStubInstrumenRepo()
	amendRepo := newStubListQueueAmendRepo()
	auditW := &stubAuditWriter{}
	schedRepo := &stubScheduleRepo{}
	driftRepo := newStubDriftRepo()

	// Seed a completed report in driftRepo.
	driftReport := &DriftReport{
		ID:                   reportID,
		TanggalRun:           time.Now().Truncate(24 * time.Hour),
		TriggerSource:        DriftTriggerManualAdHoc,
		Status:               DriftStatusCompleted,
		TotalInstrumen:       5,
		DriftLowCount:        2,
		DriftHighCount:       1,
		MissingScheduleCount: 0,
		ErrorCount:           0,
		DriftFlagThreshold:   decimal.NewFromFloat(0.0001),
		DriftHighThreshold:   decimal.NewFromFloat(0.001),
		TenantID:             "TUGURE",
	}
	now := time.Now()
	driftReport.StartedAt = &now
	driftReport.CompletedAt = &now
	driftRepo.reports[reportID] = driftReport

	eirSvc := &Service{instrRepo: instrRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()}
	schedSvc := &ScheduleService{instrRepo: instrRepo, schedRepo: schedRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()}
	amendSvc := &AmendmentService{instrRepo: instrRepo, schedRepo: schedRepo, amendRepo: amendRepo, solver: NewSolver(), auditWriter: auditW, logger: testLogger()}
	bulkSvc := NewBulkService(nil, instrRepo, schedRepo, nil, nil, testLogger())
	detectionSvc := NewDetectionService(db, instrRepo, amendRepo, auditW, testLogger())
	driftSvc := NewDriftService(db, &driftInstrRepo{}, schedRepo, amendRepo, driftRepo, NewSolver(), auditW, testLogger())

	h := NewHandlerM6(eirSvc, schedSvc, amendSvc, bulkSvc, detectionSvc, driftSvc)
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "GET",
		fmt.Sprintf("/api/v1/ecl/eir/drift-reports/%s", reportID), nil)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GenerateDriftReport ──────────────────────────────────────────────────────

func TestHandler_GenerateDriftReport_NoPermission_403(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, []string{})

	w := doRequest(r, "POST", "/api/v1/ecl/eir/drift-reports/generate", nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestHandler_GenerateDriftReport_Success_202(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	h := buildHandlerM6(db, newStubInstrumenRepo(), newStubListQueueAmendRepo())
	r := buildRouterM6(h, allM6Perms())

	w := doRequest(r, "POST", "/api/v1/ecl/eir/drift-reports/generate", nil)
	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── driftReportToJSON serializer ─────────────────────────────────────────────

func TestDriftReportToJSON_AllFields(t *testing.T) {
	now := time.Now()
	jobID := "asynq-job-001"
	triggered := uuid.New()
	errSummary := "1 errors"
	dr := DriftReport{
		ID:                   uuid.New(),
		TanggalRun:           now.Truncate(24 * time.Hour),
		TriggerSource:        DriftTriggerManualAdHoc,
		TriggeredBy:          &triggered,
		AsynqJobID:           &jobID,
		Status:               DriftStatusCompleted,
		TotalInstrumen:       10,
		DriftLowCount:        2,
		DriftHighCount:       1,
		MissingScheduleCount: 1,
		ErrorCount:           1,
		ErrorSummary:         &errSummary,
		DriftFlagThreshold:   decimal.NewFromFloat(0.0001),
		DriftHighThreshold:   decimal.NewFromFloat(0.001),
		StartedAt:            &now,
		CompletedAt:          &now,
		TenantID:             "TUGURE",
	}
	dr.CreatedAt = now
	dr.UpdatedAt = now

	m := driftReportToJSON(dr)
	if m == nil {
		t.Fatal("expected non-nil map")
	}
	if m["status"] != string(DriftStatusCompleted) {
		t.Errorf("status mismatch")
	}
	if m["asynqJobId"] == nil {
		t.Error("asynqJobId should be set")
	}
	if m["triggeredBy"] == nil {
		t.Error("triggeredBy should be set")
	}
	if m["errorSummary"] == nil {
		t.Error("errorSummary should be set")
	}
}
