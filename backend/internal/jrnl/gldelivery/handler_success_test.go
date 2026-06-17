package gldelivery_test

// Handler success-path tests: exercises the downstream service calls so that
// handler branches past permission check (service call → repo → response) are covered.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

// newHandlerRouterWithMock returns (router, mock) — mock is exposed so callers can set expectations.
func newHandlerRouterWithMock(t *testing.T, perms ...string) (*gin.Engine, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	jurnalRepo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter()

	delivery := NewDeliveryService(jurnalRepo, dlqRepo, stub, aw, nil, DefaultConfig(), nil)
	dlqSvc := NewDLQService(dlqRepo, jurnalRepo, delivery, aw, nil, nil)
	recon := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stub, aw, nil, DefaultConfig(), nil)
	h := NewHandler(delivery, dlqSvc, recon)

	userID := uuid.New()
	claims := &auth.Claims{
		Sub:         userID.String(),
		Roles:       []string{"ROLE-IT-ADMIN"},
		Permissions: perms,
		TenantID:    "TUGURE",
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	v1 := router.Group("/api/v1")
	v1.GET("/jurnal/header/:id/gl-delivery-status", h.GetDeliveryStatus)
	v1.POST("/jurnal/header/:id/retry-gl-delivery", h.RetryGLDelivery)
	v1.GET("/jurnal/gl-delivery-dlq", h.ListDLQ)
	v1.GET("/jurnal/gl-delivery-dlq/:id", h.GetDLQEntry)
	v1.POST("/jurnal/gl-delivery-dlq/:id/replay", h.ReplayDLQEntry)
	v1.POST("/jurnal/gl-delivery-dlq/:id/discard", h.DiscardDLQEntry)
	v1.POST("/jurnal/reconciliation/run", h.RunReconciliation)
	v1.GET("/jurnal/reconciliation/history", h.ListReconciliationHistory)
	v1.GET("/jurnal/reconciliation/:date", h.GetReconciliationReport)

	return router, mock
}

// ─── GetDeliveryStatus — 404 when not found ───────────────────────────────────

func TestGetDeliveryStatus_NotFound_404(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryRead)
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(sqlmock.NewRows(nil))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/gl-delivery-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDeliveryStatus_Delivered_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryRead)
	statusID := uuid.New()
	rows := mockGLStatusRows(statusID, GlHostStatusDelivered, 0)
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/gl-delivery-status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	_, hasData := body["data"]
	assert.True(t, hasData, "response must have 'data' key")
}

// ─── ListDLQ — 200 OK with empty list ────────────────────────────────────────

func TestListDLQ_WithPerm_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryRead)
	mock.ExpectQuery(`SELECT d\.id`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "jurnal_header_id", "error_code", "error_message",
		"error_category", "retry_count", "last_retry_at", "status",
		"created_at", "no_jurnal", "event_code", "tanggal_posting",
		"gl_host_status",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/gl-delivery-dlq", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	_, hasData := body["data"]
	assert.True(t, hasData, "response must have 'data' key")
}

// ─── GetDLQEntry — not found 404 ─────────────────────────────────────────────

func TestGetDLQEntry_NotFound_404(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryRead)
	mock.ExpectQuery(`SELECT d\.id`).WillReturnRows(sqlmock.NewRows(nil))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDLQEntry_Found_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryRead)
	dlqID, headerID := uuid.New(), uuid.New()
	rows := addDLQEntryRow(sqlmock.NewRows(dlqEntryColumns()), dlqID, headerID, DLQStatusFailed)
	mock.ExpectQuery(`SELECT d\.id`).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/gl-delivery-dlq/"+dlqID.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── RunReconciliation — in-progress 409 ─────────────────────────────────────

func TestRunReconciliationHandler_InProgress_409(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermReconciliationRun)
	mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(1),
	)

	body := `{"date":"2026-06-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jurnal/reconciliation/run",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestRunReconciliationHandler_Success_202(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermReconciliationRun)

	// IsInProgress → 0.
	mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(
		sqlmock.NewRows([]string{"count"}).AddRow(0),
	)

	// TriggerAsync: Insert report + audit.
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body := `{"date":"2026-06-15"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/jurnal/reconciliation/run",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

// ─── GetReconciliationReport ──────────────────────────────────────────────────

func TestGetReconciliationReportHandler_NotFound_404(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermReconciliationRead)
	mock.ExpectQuery(`SELECT id`).WillReturnRows(sqlmock.NewRows(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/2026-06-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetReconciliationReportHandler_Found_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermReconciliationRead)

	reportID := uuid.New()
	callerID := uuid.New()
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	jobID := uuid.New().String()
	completedAt := time.Now()

	reportRows := sqlmock.NewRows(reconReportColumns()).AddRow(
		reportID, date, "MANUAL", callerID, jobID,
		"COMPLETED", date, completedAt,
		"5000000.0000", "5000000.0000",
		0, "1.0000",
		nil, []byte("{}"),
	)
	mock.ExpectQuery(`SELECT id`).WillReturnRows(reportRows)

	// GetByReportID for mismatches.
	mock.ExpectQuery(`SELECT m\.id`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "report_id", "akun_id", "blips_amount_idr", "gl_host_amount_idr", "delta_idr",
		"mismatch_type", "jurnal_header_ids", "note", "kode_akun", "nama_akun",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/2026-06-15", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── ListReconciliationHistory ────────────────────────────────────────────────

func TestListReconciliationHistoryHandler_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermReconciliationRead)

	mock.ExpectQuery(`SELECT id`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "tanggal_run", "status", "mismatch_count", "completed_at", "asynq_job_id",
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/jurnal/reconciliation/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── RetryGLDelivery — reason too short → 400 ────────────────────────────────

func TestRetryGLDeliveryHandler_ReasonTooShort_400(t *testing.T) {
	router, _ := newHandlerRouterWithMock(t, PermGlDeliveryRetry)
	// GetDeliveryStatus: return FAILED so transition is valid, but reason is too short.
	// Actually: ManualRetry checks reason FIRST before DB. So no mock needed.
	body := `{"reason":"short"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/header/"+uuid.New().String()+"/retry-gl-delivery",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── ReplayDLQEntry — reason too short → 400 ─────────────────────────────────

func TestReplayDLQEntryHandler_ReasonTooShort_400(t *testing.T) {
	router, _ := newHandlerRouterWithMock(t, PermGlDeliveryReplay)
	body := `{"reason":"short"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/replay",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── DiscardDLQEntry — reason too short → 400 ────────────────────────────────

func TestDiscardDLQEntryHandler_ReasonTooShort_400(t *testing.T) {
	router, _ := newHandlerRouterWithMock(t, PermGlDeliveryDiscard)
	body := `{"reason":"short"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+uuid.New().String()+"/discard",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ─── DLQ entry found: replay 400 (invalid state) ─────────────────────────────

func TestReplayDLQEntry_AlreadyReplayed_422(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryReplay)

	dlqID, headerID := uuid.New(), uuid.New()
	// Return entry in REPLAYED_OK status (CanReplay=false).
	rows := addDLQEntryRow(sqlmock.NewRows(dlqEntryColumns()), dlqID, headerID, DLQStatusReplayedOK)
	mock.ExpectQuery(`SELECT d\.id`).WillReturnRows(rows)

	body := `{"reason":"this is a valid replay reason over thirty characters"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+dlqID.String()+"/replay",
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// ─── Worker HandleDeliverTask — success path ─────────────────────────────────

func TestHandleDeliverTask_Success_Delivered(t *testing.T) {
	db, mock := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter() // success stub
	delivery := NewDeliveryService(jurnalRepo, dlqRepo, stub, aw, nil, DefaultConfig(), nil)
	recon := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stub, aw, nil, DefaultConfig(), nil)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	headerID, statusID := uuid.New(), uuid.New()
	mockHeaderAndDetail(t, mock, headerID, statusID, GlHostStatusPendingDelivery, 0)
	expectStatusUpdateTx(mock) // IN_FLIGHT
	expectStatusUpdateTx(mock) // DELIVERED

	taskPayload, err := json.Marshal(map[string]string{"jurnal_header_id": headerID.String()})
	require.NoError(t, err)
	task := asynq.NewTask(TaskGLDelivery, taskPayload)

	workerErr := worker.HandleDeliverTask(context.Background(), task)
	require.NoError(t, workerErr)
	assert.Len(t, stub.Calls(), 1)
}
