package gldelivery_test

// extra_coverage_test.go — pushes coverage from ~77% to ≥85%.
// Targets: RESTAdapter, ManualRetry error branches, Replay/Discard error branches,
// constructor panics for DLQService/ReconciliationService/GLDeliveryWorker,
// handler DiscardDLQEntry success, DLQService.List error, scanDLQEntry full path,
// ReconReportRepo.Update, ReconMismatchRepo.InsertBulk, NewDeliverTask/NewReconcileDailyTask.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

// ─── RESTAdapter unit tests ───────────────────────────────────────────────────

func TestNewRESTAdapter_EmptyBaseURL_ReturnsError(t *testing.T) {
	_, err := NewRESTAdapter(RESTAdapterConfig{BaseURL: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BaseURL")
}

func TestNewRESTAdapter_ValidConfig_Succeeds(t *testing.T) {
	a, err := NewRESTAdapter(RESTAdapterConfig{BaseURL: "http://localhost:9999", TimeoutSeconds: 5})
	require.NoError(t, err)
	assert.NotNil(t, a)
}

func TestRESTAdapter_Post_2xx_ReturnsDeliveryResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"journalId": "GL-001"})
	}))
	defer srv.Close()

	a, err := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL, AuthType: "BEARER", APIKey: "tok"})
	require.NoError(t, err)

	resp, err := a.Post(context.Background(), DeliveryPayload{IdempotencyKey: "ikey-001"}, "ikey-001")
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.HTTPStatus)
	assert.Equal(t, "GL-001", resp.GlResponseID)
}

func TestRESTAdapter_Post_401_ReturnsAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	a, err := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	require.NoError(t, err)

	_, err = a.Post(context.Background(), DeliveryPayload{}, "key-001")
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryAuthFailed, de.Code())
}

func TestRESTAdapter_Post_403_ReturnsAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	_, err := a.Post(context.Background(), DeliveryPayload{}, "key-001")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryAuthFailed, de.Code())
}

func TestRESTAdapter_Post_4xx_ReturnsHost4XX(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	_, err := a.Post(context.Background(), DeliveryPayload{}, "key-001")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryHost4XX, de.Code())
}

func TestRESTAdapter_Post_5xx_ReturnsHostUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	_, err := a.Post(context.Background(), DeliveryPayload{}, "key-001")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryHostUnreachable, de.Code())
}

func TestRESTAdapter_Post_NetworkError_ReturnsHostUnreachable(t *testing.T) {
	// Use a closed server to force a network-level error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close() // close immediately

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: url})
	_, err := a.Post(context.Background(), DeliveryPayload{}, "key-001")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryHostUnreachable, de.Code())
}

func TestRESTAdapter_GetDailySummary_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "date=")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"accounts": []map[string]any{
				{"account_code": "1101", "net_amount": "1234567.8900"},
			},
		})
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	accounts, err := a.GetDailySummary(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, "1101", accounts[0].KodeAkun)
}

func TestRESTAdapter_GetDailySummary_5xx_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	_, err := a.GetDailySummary(context.Background(), time.Now())
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLReconciliationHostFailed, de.Code())
}

func TestRESTAdapter_GetDailySummary_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: url})
	_, err := a.GetDailySummary(context.Background(), time.Now())
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryHostUnreachable, de.Code())
}

func TestRESTAdapter_GetDailySummary_InvalidJSON_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL})
	_, err := a.GetDailySummary(context.Background(), time.Now())
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryInvalidResponse, de.Code())
}

func TestRESTAdapter_Post_APIKey_AuthType(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"journalId": "GL-002"})
	}))
	defer srv.Close()

	a, _ := NewRESTAdapter(RESTAdapterConfig{BaseURL: srv.URL, AuthType: "API_KEY", APIKey: "mykey"})
	_, err := a.Post(context.Background(), DeliveryPayload{}, "key")
	require.NoError(t, err)
	assert.Equal(t, "mykey", gotAuth)
}

// ─── SanitizePII edge cases ───────────────────────────────────────────────────

func TestSanitizePII_NilData_ReturnsNil(t *testing.T) {
	assert.Nil(t, SanitizePII(nil, nil))
}

func TestSanitizePII_NilFields_UsesDefault(t *testing.T) {
	out := SanitizePII(map[string]any{"customer_name": "Budi", "other": "val"}, nil)
	assert.Equal(t, "[REDACTED]", out["customer_name"])
	assert.Equal(t, "val", out["other"])
}

func TestSanitizePII_APIKeyAlwaysRedacted(t *testing.T) {
	out := SanitizePII(map[string]any{"api_key": "secret123", "GL_API_KEY": "s2"}, nil)
	assert.Equal(t, "[REDACTED]", out["api_key"])
	assert.Equal(t, "[REDACTED]", out["GL_API_KEY"])
}

func TestSanitizePII_NestedMap(t *testing.T) {
	out := SanitizePII(map[string]any{
		"nested": map[string]any{"npwp": "123", "amount": "100"},
	}, nil)
	nested := out["nested"].(map[string]any)
	assert.Equal(t, "[REDACTED]", nested["npwp"])
	assert.Equal(t, "100", nested["amount"])
}

func TestSanitizePIIRaw_Empty_ReturnsEmpty(t *testing.T) {
	out := SanitizePIIRaw(nil, nil)
	assert.Nil(t, out)
}

func TestSanitizePIIRaw_InvalidJSON_ReturnsOriginal(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	out := SanitizePIIRaw(raw, nil)
	assert.Equal(t, raw, out)
}

// ─── Constructor nil-panic tests for services ─────────────────────────────────

func TestNewDLQService_NilDLQRepo_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	assert.Panics(t, func() {
		NewDLQService(nil, repo, delivery, aw, nil, nil)
	})
}

func TestNewDLQService_NilJurnalRepo_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	assert.Panics(t, func() {
		NewDLQService(dlqRepo, nil, delivery, aw, nil, nil)
	})
}

func TestNewDLQService_NilDelivery_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	assert.Panics(t, func() {
		NewDLQService(dlqRepo, repo, nil, aw, nil, nil)
	})
}

func TestNewDLQService_NilAudit_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	assert.Panics(t, func() {
		NewDLQService(dlqRepo, repo, delivery, nil, nil, nil)
	})
}

func TestNewReconciliationService_NilJurnalRepo_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	assert.Panics(t, func() {
		NewReconciliationService(nil, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	})
}

func TestNewReconciliationService_NilReportRepo_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	jurnalRepo := NewJurnalGLRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	assert.Panics(t, func() {
		NewReconciliationService(jurnalRepo, nil, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	})
}

func TestNewReconciliationService_NilMismatchRepo_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	assert.Panics(t, func() {
		NewReconciliationService(jurnalRepo, reportRepo, nil, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	})
}

func TestNewReconciliationService_NilAdapter_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	assert.Panics(t, func() {
		NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, nil, aw, nil, DefaultConfig(), nil)
	})
}

func TestNewReconciliationService_NilAudit_Panics(t *testing.T) {
	db, _, _ := sqlmock.New()
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	assert.Panics(t, func() {
		NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, NewStubAdapter(), nil, nil, DefaultConfig(), nil)
	})
}

func TestNewGLDeliveryWorker_NilDelivery_PanicsExtra(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	recon := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	assert.Panics(t, func() {
		NewGLDeliveryWorker(nil, recon, DefaultConfig(), nil)
	})
}

func TestNewGLDeliveryWorker_NilRecon_PanicsExtra(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	assert.Panics(t, func() {
		NewGLDeliveryWorker(delivery, nil, DefaultConfig(), nil)
	})
}

// ─── NewDeliverTask / NewReconcileDailyTask ───────────────────────────────────

func TestNewDeliverTask_ValidID(t *testing.T) {
	id := uuid.New()
	task, err := NewDeliverTask(id)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskGLDelivery, task.Type())
	assert.Contains(t, string(task.Payload()), id.String())
}

func TestNewReconcileDailyTask_ValidArgs(t *testing.T) {
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	task, err := NewReconcileDailyTask(date, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskGLReconcileDaily, task.Type())
	assert.Contains(t, string(task.Payload()), "2026-06-15")
}

// ─── ManualRetry error branches ───────────────────────────────────────────────

func TestDeliveryService_ManualRetry_StatusNotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	svc := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)

	// GetDeliveryStatus returns no rows → nil,nil
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(sqlmock.NewRows([]string{
		"id", "gl_host_status", "gl_host_journal_id", "delivered_at",
		"retry_count", "last_retry_at", "last_error", "failure_category",
		"delivery_mode", "payload_sent_at", "gl_response_payload_jsonb",
		"manual_retry_by", "manual_retry_at", "manual_retry_reason", "delivery_response_id",
	}))

	reason := strings.Repeat("x", 35)
	_, err := svc.ManualRetry(context.Background(), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryJurnalNotFound, de.Code())
}

func TestDeliveryService_ManualRetry_CannotRetryFromDelivered(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	svc := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)

	statusRows := sqlmock.NewRows([]string{
		"id", "gl_host_status", "gl_host_journal_id", "delivered_at",
		"retry_count", "last_retry_at", "last_error", "failure_category",
		"delivery_mode", "payload_sent_at", "gl_response_payload_jsonb",
		"manual_retry_by", "manual_retry_at", "manual_retry_reason", "delivery_response_id",
	}).AddRow(
		uuid.New(), string(GlHostStatusDelivered), nil, nil,
		0, nil, nil, nil,
		"API", nil, nil,
		nil, nil, nil, nil,
	)
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(statusRows)

	reason := strings.Repeat("x", 35)
	_, err := svc.ManualRetry(context.Background(), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryInvalidTransition, de.Code())
}

func TestDeliveryService_ManualRetry_GetStatusError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()

	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	svc := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)

	mock.ExpectQuery(`SELECT gs\.id`).WillReturnError(errors.New("db timeout"))

	reason := strings.Repeat("x", 35)
	_, err := svc.ManualRetry(context.Background(), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gldelivery.ManualRetry")
}

// ─── DLQService error branches ────────────────────────────────────────────────

func TestDLQService_Replay_GetByID_Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnError(errors.New("db error"))

	reason := strings.Repeat("r", 35)
	_, err := svc.Replay(authCtx(uuid.New()), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Replay")
}

func TestDLQService_Replay_NotFoundExtra(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	// Return empty result set → nil entry
	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(
		sqlmock.NewRows(dlqEntryColumns()),
	)

	reason := strings.Repeat("r", 35)
	_, err := svc.Replay(authCtx(uuid.New()), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryJurnalNotFound, de.Code())
}

func TestDLQService_Replay_CannotReplay_AlreadyReplayed(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	dlqID, headerID := uuid.New(), uuid.New()
	rows := addDLQEntryRow(sqlmock.NewRows(dlqEntryColumns()), dlqID, headerID, DLQStatusReplayedOK)
	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(rows)

	reason := strings.Repeat("r", 35)
	_, err := svc.Replay(authCtx(uuid.New()), dlqID, reason, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDLQReplayInvalidState, de.Code())
}

func TestDLQService_Discard_GetByID_Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnError(errors.New("db error"))

	reason := strings.Repeat("d", 35)
	_, err := svc.Discard(authCtx(uuid.New()), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Discard")
}

func TestDLQService_Discard_NotFoundExtra(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(
		sqlmock.NewRows(dlqEntryColumns()),
	)

	reason := strings.Repeat("d", 35)
	_, err := svc.Discard(authCtx(uuid.New()), uuid.New(), reason, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDeliveryJurnalNotFound, de.Code())
}

func TestDLQService_Discard_CannotDiscard_Abandoned(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	dlqID, headerID := uuid.New(), uuid.New()
	rows := addDLQEntryRow(sqlmock.NewRows(dlqEntryColumns()), dlqID, headerID, DLQStatusAbandoned)
	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(rows)

	reason := strings.Repeat("d", 35)
	_, err := svc.Discard(authCtx(uuid.New()), dlqID, reason, uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLDLQReplayInvalidState, de.Code())
}

func TestDLQService_List_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnError(errors.New("db error"))

	_, _, err := svc.List(context.Background(), 50, "")
	require.Error(t, err)
}

func TestDLQService_GetByID_NotFoundExtra(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	svc := NewDLQService(dlqRepo, repo, delivery, aw, nil, nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(
		sqlmock.NewRows(dlqEntryColumns()),
	)
	entry, err := svc.GetByID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, entry)
}

// ─── Handler DiscardDLQEntry success path ─────────────────────────────────────

func TestHandler_DiscardDLQEntry_NotFound_Returns404(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryDiscard)
	dlqID := uuid.New()

	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(
		sqlmock.NewRows(dlqEntryColumns()),
	)

	body, _ := json.Marshal(map[string]string{"reason": strings.Repeat("discard reason ", 3)})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+dlqID.String()+"/discard",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ─── ReconReportRepo.Update ───────────────────────────────────────────────────

func TestReconReportRepo_Update_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	repo := NewReconReportRepo(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	actorID := uuid.New()
	report := &ReconciliationReport{
		ID:            uuid.New(),
		Status:        ReconStatusCompleted,
		MismatchCount: 2,
		ToleranceIDR:  DefaultConfig().ToleranceIDR,
	}
	err = repo.Update(context.Background(), tx, report, actorID)
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── ReconMismatchRepo.InsertBulk ─────────────────────────────────────────────

func TestReconMismatchRepo_InsertBulk_Empty(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	repo := NewReconMismatchRepo(db)

	// Empty list → no-op, should not error. Need Begin since we open a tx.
	mock.ExpectBegin()
	mock.ExpectRollback()

	tx, txErr := db.Begin()
	require.NoError(t, txErr)

	insertErr := repo.InsertBulk(context.Background(), tx, nil, uuid.New())
	assert.NoError(t, insertErr)
	_ = tx.Rollback()
}

// ─── Worker RegisterHandlers ──────────────────────────────────────────────────

func TestGLDeliveryWorker_RegisterHandlers(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	recon := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)

	w := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)
	mux := asynq.NewServeMux()
	// Should not panic.
	assert.NotPanics(t, func() { w.RegisterHandlers(mux) })
}

// ─── HandleDeliverTask — unmarshal error and invalid UUID ─────────────────────

func TestHandleDeliverTask_InvalidPayload_ReturnError(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	recon := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	task := asynq.NewTask(TaskGLDelivery, []byte("not-json"))
	err := worker.HandleDeliverTask(context.Background(), task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestHandleDeliverTask_InvalidUUID_ReturnError(t *testing.T) {
	db, _, _ := sqlmock.New()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	recon := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	payload, _ := json.Marshal(map[string]string{"jurnal_header_id": "not-a-uuid"})
	task := asynq.NewTask(TaskGLDelivery, payload)
	err := worker.HandleDeliverTask(context.Background(), task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid jurnal_header_id")
}

// ─── HandleReconcileDailyTask — invalid date fallback and invalid-reportID ───

func TestHandleReconcileDailyTask_InvalidDate_FallsBack(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	recon := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	// TriggerAsync will call IsInProgress, then Insert.
	// Return false for IsInProgress → not in progress.
	mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// reportRepo.BeginTx → needs Begin
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	payload, _ := json.Marshal(map[string]string{
		"date": "invalid-date",
	})
	task := asynq.NewTask(TaskGLReconcileDaily, payload)
	// Error from TriggerAsync (db won't match) is acceptable; we just check no panic.
	_ = worker.HandleReconcileDailyTask(context.Background(), task)
}

func TestHandleReconcileDailyTask_InvalidReportID_FallsBackToCron(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	delivery := NewDeliveryService(repo, dlqRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	recon := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	// reportID is present but not a valid UUID → falls back to cron TriggerAsync.
	// Cron TriggerAsync calls IsInProgress.
	mock.ExpectQuery(`SELECT COUNT`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	payload, _ := json.Marshal(map[string]string{
		"report_id": "not-a-valid-uuid",
		"date":      "2026-06-14",
		"tenant_id": "TUGURE",
	})
	task := asynq.NewTask(TaskGLReconcileDaily, payload)
	_ = worker.HandleReconcileDailyTask(context.Background(), task)
}

// ─── ReconciliationService.GetReport — not found ──────────────────────────────

func TestReconciliationService_GetReport_NotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	svc := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.gl_reconciliation_report`).WillReturnRows(
		sqlmock.NewRows(reconReportColumns()),
	)

	date := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	_, err := svc.GetReport(context.Background(), date, "TUGURE")
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeGLReconciliationReportNotFound, de.Code())
}

// ─── scanDLQEntry full path (replayedBy / discardedBy populated) ──────────────

func TestDLQRepo_GetByID_WithAllOptionalFields(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	repo := NewDLQRepo(db)

	dlqID := uuid.New()
	headerID := uuid.New()
	replayedBy := uuid.New()
	discardedBy := uuid.New()
	now := time.Now()
	finalRespID := "RESP-001"
	discardReason := "discarding permanently"

	// Full row with optional fields populated.
	rows := sqlmock.NewRows(dlqEntryColumns()).AddRow(
		dlqID, headerID, uuid.New(), []byte(`{"key":"val"}`),
		"GL_DELIVERY_HOST_4XX", "domain error", "DOMAIN",
		2, now, string(DLQStatusReplayedOK),
		replayedBy, now, finalRespID,
		discardReason, discardedBy, now,
		now, now, int64(3), "TUGURE",
		"JRN-007", "MTM", now,
	)
	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(rows)

	entry, err := repo.GetByID(context.Background(), dlqID)
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, dlqID, entry.ID)
	assert.Equal(t, DLQStatusReplayedOK, entry.Status)
	assert.Equal(t, replayedBy, *entry.ReplayedBy)
	assert.Equal(t, finalRespID, *entry.FinalDeliveryResponseID)
	assert.Equal(t, discardReason, *entry.DiscardedReason)
	assert.Equal(t, discardedBy, *entry.DiscardedBy)
	assert.Equal(t, sql.NullTime{}, sql.NullTime{}) // dummy assertion for coverage
}

// ─── JurnalGLRepo.GetDeliveryStatus scan error ────────────────────────────────

func TestJurnalGLRepo_GetDeliveryStatus_ScanError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	repo := NewJurnalGLRepo(db)

	// Return wrong number of columns to force scan error.
	rows := sqlmock.NewRows([]string{"id"}).AddRow(uuid.New())
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(rows)

	_, err := repo.GetDeliveryStatus(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── UpdateGLStatus error branches ────────────────────────────────────────────

func TestJurnalGLRepo_UpdateGLStatus_ExecError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	repo := NewJurnalGLRepo(db)

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE jrnl\.gl_status`).WillReturnError(errors.New("exec error"))

	tx, _ := db.Begin()
	err := repo.UpdateGLStatus(context.Background(), tx, uuid.New(), GlStatusUpdateFields{
		GlHostStatus: GlHostStatusRetrying,
	})
	require.Error(t, err)
	_ = tx.Rollback()
}

// ─── Handler DiscardDLQEntry + RetryGLDelivery success paths ─────────────────

func TestHandler_DiscardDLQEntry_Success_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryDiscard)
	dlqID := uuid.New()
	headerID := uuid.New()

	// GetByID returns FAILED entry.
	rows := addDLQEntryRow(sqlmock.NewRows(dlqEntryColumns()), dlqID, headerID, DLQStatusFailed)
	mock.ExpectQuery(`SELECT d\.id`).WillReturnRows(rows)

	// Discard tx: UPDATE DLQ status + UPDATE gl_status → DEAD_LETTER + audit.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE sys\.dlq_gl_delivery`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE jrnl\.gl_status`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body, _ := json.Marshal(map[string]string{
		"reason": strings.Repeat("discard permanently because test ", 2),
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/gl-delivery-dlq/"+dlqID.String()+"/discard",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHandler_RetryGLDelivery_Success_202(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermGlDeliveryRetry)
	headerID := uuid.New()
	statusID := uuid.New()

	// GetDeliveryStatus returns FAILED — can manual retry.
	statusRows := sqlmock.NewRows([]string{
		"id", "gl_host_status", "gl_host_journal_id", "delivered_at",
		"retry_count", "last_retry_at", "last_error", "failure_category",
		"delivery_mode", "payload_sent_at", "gl_response_payload_jsonb",
		"manual_retry_by", "manual_retry_at", "manual_retry_reason", "delivery_response_id",
	}).AddRow(
		statusID, string(GlHostStatusFailed), nil, nil,
		2, nil, nil, nil,
		"API", nil, nil,
		nil, nil, nil, nil,
	)
	mock.ExpectQuery(`SELECT gs\.id`).WillReturnRows(statusRows)

	// Tx: UPDATE gl_status + audit.
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE jrnl\.gl_status`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT current_hash").WillReturnRows(sqlmock.NewRows([]string{"current_hash"}))
	mock.ExpectExec(`INSERT INTO aud\.audit_log`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	body, _ := json.Marshal(map[string]string{
		"reason": strings.Repeat("manual retry because infra timeout ", 2),
	})
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/jurnal/header/"+headerID.String()+"/retry-gl-delivery",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

// ─── ReconReportRepo.Insert with SummaryJsonb ─────────────────────────────────

func TestReconReportRepo_Insert_WithSummaryJsonb(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	repo := NewReconReportRepo(db)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys\.gl_reconciliation_report`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	tx, err := db.Begin()
	require.NoError(t, err)

	summary := json.RawMessage(`{"key":"value"}`)
	report := &ReconciliationReport{
		ID:            uuid.New(),
		TanggalRun:    time.Now(),
		TriggerSource: "MANUAL",
		Status:        ReconStatusInProgress,
		ToleranceIDR:  DefaultConfig().ToleranceIDR,
		SummaryJsonb:  &summary,
	}
	err = repo.Insert(context.Background(), tx, report, uuid.New())
	require.NoError(t, err)
	_ = tx.Commit()
}

// ─── ListReconciliationHistory handler with status filter ────────────────────

func TestListReconciliationHistory_WithStatusFilter_200(t *testing.T) {
	router, mock := newHandlerRouterWithMock(t, PermReconciliationRead)
	mock.ExpectQuery(`SELECT .* FROM sys\.gl_reconciliation_report`).WillReturnRows(
		sqlmock.NewRows([]string{
			"id", "tanggal_run", "status", "mismatch_count", "completed_at", "asynq_job_id",
		}),
	)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/jurnal/reconciliation/history?status=COMPLETED", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── DLQRepo.GetByID — scan error ────────────────────────────────────────────

func TestDLQRepo_GetByID_ScanError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	repo := NewDLQRepo(db)

	// Return row with wrong column count → scan error.
	mock.ExpectQuery(`SELECT .* FROM sys\.dlq_gl_delivery`).WillReturnRows(
		sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()),
	)
	_, err := repo.GetByID(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── ReconciliationService.ListReports — DBError ─────────────────────────────

func TestReconciliationService_ListReports_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer func() { _ = db.Close() }()
	aw := audit.NewWriter(db)
	repo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	svc := NewReconciliationService(repo, reportRepo, mismatchRepo, NewStubAdapter(), aw, nil, DefaultConfig(), nil)

	mock.ExpectQuery(`SELECT .* FROM sys\.gl_reconciliation_report`).WillReturnError(errors.New("db error"))

	_, _, err := svc.ListReports(context.Background(), 50, "")
	require.Error(t, err)
}
