package pocidelta

// coverage_boost_test.go — Additional tests targeting uncovered branches to reach ≥85%.
// Targets: NewService nil args, actorAndTenant, writeAuditInTx tx-path,
// ListBaselines rows.Err, GetDeltaHistoryByInstrumen rows.Err,
// scanDeltaLog bad current/delta/prior parse, worker helpers, RegisterHandlers,
// ComputeDeltaBatch parse path, GetCurrentECLForPociInstrumen bad parse.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── NewService with nil logger (uses slog.Default()) ────────────────────────

func TestNewService_NilLogger(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, nil, nil, nil) // nil logger and nil poster
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.logger == nil {
		t.Fatal("expected non-nil logger (defaulted to slog.Default())")
	}
}

func TestNewService_NilPoster(t *testing.T) {
	repo := &stubRepo{}
	svc := NewService(repo, nil, nil, slog.Default())
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	// poster should default to NoopJurnalPoster
	if svc.poster == nil {
		t.Fatal("expected non-nil poster (defaulted to Noop)")
	}
}

// ─── actorAndTenant — with valid JWT claims ───────────────────────────────────

func TestActorAndTenant_WithClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		actorID, tenantID := actorAndTenant(c)
		c.JSON(http.StatusOK, gin.H{
			"actor":  actorID.String(),
			"tenant": tenantID,
		})
	})

	// Build a gin context with claims injected
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		Sub:      uuid.New().String(),
		TenantID: "TUGURE-TEST",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["tenant"] != "TUGURE-TEST" {
		t.Fatalf("unexpected tenant: %s", resp["tenant"])
	}
}

func TestActorAndTenant_WithClaims_EmptyTenantFallsBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		_, tenantID := actorAndTenant(c)
		c.JSON(http.StatusOK, gin.H{"tenant": tenantID})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		Sub:      uuid.New().String(),
		TenantID: "", // empty → fallback to "TUGURE"
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["tenant"] != "TUGURE" {
		t.Fatalf("expected fallback tenant TUGURE, got %s", resp["tenant"])
	}
}

func TestActorAndTenant_WithClaims_BadSubUUID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", func(c *gin.Context) {
		actorID, _ := actorAndTenant(c)
		c.JSON(http.StatusOK, gin.H{"actor": actorID.String()})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		Sub:      "not-a-uuid", // bad UUID → uuid.Nil
		TenantID: "TUGURE",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["actor"] != uuid.Nil.String() {
		t.Fatalf("expected uuid.Nil, got %s", resp["actor"])
	}
}

// ─── writeAuditInTx — with tx but nil audit writer ───────────────────────────

func TestWriteAuditInTx_WithTxNilWriter(t *testing.T) {
	db, mock := newTestDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	tx, _ := db.Begin()
	defer tx.Commit() //nolint:errcheck

	repo := &stubRepo{}
	svc := NewService(repo, nil, nil, slog.Default()) // nil audit writer
	// Should not panic — tx non-nil but audit nil → skip
	svc.writeAuditInTx(context.Background(), tx, audit.Event{
		Action:     "POCI.TEST",
		EntityType: "test",
		EntityID:   uuid.New(),
	})
}

// ─── RegisterHandlers (worker) ────────────────────────────────────────────────

func TestRegisterHandlers_RegistersTask(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	w := NewWorker(svc, nil, slog.Default())
	mux := asynq.NewServeMux()
	// Should not panic
	w.RegisterHandlers(mux)
}

// ─── RegisterRoutes coverage ──────────────────────────────────────────────────

func TestRegisterRoutes_RegistersWithoutRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	v1 := engine.Group("/api/v1")

	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil)
	// Call with no redis client (variadic)
	RegisterRoutes(v1, h)
	// If it panics, test fails. Success = no panic.
}

// ─── ListBaselines — rows scan error path ────────────────────────────────────

func TestListBaselines_ScanError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	// Provide wrong number of columns to trigger scan error
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(
		sqlmock.NewRows(cols[:3]).AddRow(uuid.New(), uuid.New(), time.Now()),
	)

	_, _, err := r.ListBaselines(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// ─── GetDeltaHistoryByInstrumen — has rows ────────────────────────────────────

func TestGetDeltaHistoryByInstrumen_WithRows(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	instrID := uuid.New()
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), instrID, now,
			"1000.0000", "1200.0000", "200.0000", "INCREASE",
			nil, nil, nil,
			"COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		),
	)

	rows, pag, err := r.GetDeltaHistoryByInstrumen(context.Background(), instrID, listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if pag.HasMore {
		t.Fatal("expected HasMore=false")
	}
}

// ─── scanDeltaLog — bad current_ecl and delta_ecl parses ────────────────────

func TestScanDeltaLog_BadCurrentECL(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), uuid.New(), now,
			"1000.0000", "BAD_CURRENT", "200.0000", "INCREASE",
			nil, nil, nil, "COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		),
	)

	_, _, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad current_ecl")
	}
}

func TestScanDeltaLog_BadDeltaECL(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), uuid.New(), now,
			"1000.0000", "1200.0000", "BAD_DELTA", "INCREASE",
			nil, nil, nil, "COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		),
	)

	_, _, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad delta_ecl")
	}
}

func TestScanDeltaLog_BadPriorCumulative(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "calc_run_id", "instrumen_id", "tanggal_compute",
		"baseline_ecl", "current_ecl", "delta_ecl", "direction",
		"prior_delta_cumulative", "jurnal_header_id", "periode_bulanan_id",
		"status", "created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}
	now := time.Now()
	badPrior := "NOT_A_DECIMAL"
	mock.ExpectQuery("SELECT id, calc_run_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), uuid.New(), now,
			"1000.0000", "1200.0000", "200.0000", "INCREASE",
			&badPrior, nil, nil, "COMPUTED",
			now, uuid.New(), now, uuid.New(),
			nil, nil, int64(1), "TUGURE",
		),
	)

	_, _, err := r.ListDeltaLogs(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad prior_delta_cumulative")
	}
}

// ─── GetCurrentECLForPociInstrumen — bad decimal parse ───────────────────────

func TestGetCurrentECLForPociInstrumen_BadParse(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT ecl_weighted").WillReturnRows(
		sqlmock.NewRows([]string{"ecl_weighted"}).AddRow("NOT_A_NUMBER"),
	)

	_, err := r.GetCurrentECLForPociInstrumen(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad ecl_weighted")
	}
}

// ─── GetCumulativeDelta — bad decimal parse ────────────────────────────────

func TestGetCumulativeDelta_BadParse(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(
		sqlmock.NewRows([]string{"sum"}).AddRow("NOT_A_NUMBER"),
	)

	_, err := r.GetCumulativeDelta(context.Background(), uuid.New(), time.Now(), "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad cumulative delta")
	}
}

// ─── GetBaselineByInstrumen — bad EIR parse ──────────────────────────────────

func TestGetBaselineByInstrumen_BadEIRParse(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), time.Now(), "1000000.0000",
			nil, "NOT_A_RATE", time.Now(),
			time.Now(), uuid.New(), "TUGURE",
		),
	)

	_, err := r.GetBaselineByInstrumen(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad eir")
	}
}

// ─── ListBaselines — bad EIR parse on scan ───────────────────────────────────

func TestListBaselines_BadEIRParse(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	cols := []string{
		"id", "instrumen_id", "tanggal_baseline", "lifetime_ecl_at_origination",
		"cashflow_expectasi_jsonb", "credit_adjusted_eir", "origination_date",
		"created_at", "created_by", "tenant_id",
	}
	mock.ExpectQuery("SELECT id, instrumen_id").WillReturnRows(
		sqlmock.NewRows(cols).AddRow(
			uuid.New(), uuid.New(), time.Now(), "1000000.0000",
			nil, "BAD_EIR", time.Now(),
			time.Now(), uuid.New(), "TUGURE",
		),
	)

	_, _, err := r.ListBaselines(context.Background(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected parse error for bad eir in ListBaselines scan")
	}
}

// ─── ListPociInstrumenByCalcRun — scan error ──────────────────────────────────

func TestListPociInstrumenByCalcRun_ScanError(t *testing.T) {
	db, mock := newTestDB(t)
	r := &sqlRepo{db: db}

	// Provide only 2 of the 5 expected columns to cause scan error
	mock.ExpectQuery("SELECT DISTINCT i.id").WillReturnRows(
		sqlmock.NewRows([]string{"id", "kode_instrumen"}).
			AddRow(uuid.New(), "INSTR-001"),
	)

	_, err := r.ListPociInstrumenByCalcRun(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected scan error")
	}
}

// ─── ComputeDeltaForCalcRun — GetDeltaLogByRunAndInstrumen error ─────────────

func TestComputeDeltaForCalcRun_GetDeltaLogError(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepoDeltaLogErr{
		stubRepo: stubRepo{
			calcRunStatus: "SEALED",
			pociList: []InstrumenPociInfo{
				{ID: instrID, KodeInstrumen: "INSTR-001", IsPoci: true, Status: "ACTIVE"},
			},
		},
	}
	svc := makeService(repo)
	errs, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected global error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected per-instrument INTERNAL error from GetDeltaLogByRunAndInstrumen")
	}
	if errs[0].ErrorCode != "INTERNAL" {
		t.Fatalf("expected INTERNAL, got %s", errs[0].ErrorCode)
	}
}

type stubRepoDeltaLogErr struct {
	stubRepo
}

func (r *stubRepoDeltaLogErr) GetDeltaLogByRunAndInstrumen(_ context.Context, _, _ uuid.UUID, _ string) (*DeltaLog, error) {
	return nil, errors.New("db error getting delta log")
}

// ─── ComputeDeltaForCalcRun — GetCurrentECL error ────────────────────────────

func TestComputeDeltaForCalcRun_GetCurrentECLError(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepoCurrentECLErr{
		stubRepo: stubRepo{
			calcRunStatus: "SEALED",
			pociList: []InstrumenPociInfo{
				{ID: instrID, KodeInstrumen: "INSTR-001", IsPoci: true, Status: "ACTIVE"},
			},
			baseline: &Baseline{
				ID:                       uuid.New(),
				InstrumenID:              instrID,
				LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
			},
			deltaLog: nil,
		},
	}
	svc := makeService(repo)
	errs, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected global error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected per-instrument error from GetCurrentECLForPociInstrumen")
	}
	if errs[0].ErrorCode != "INTERNAL" {
		t.Fatalf("expected INTERNAL, got %s", errs[0].ErrorCode)
	}
}

type stubRepoCurrentECLErr struct {
	stubRepo
}

func (r *stubRepoCurrentECLErr) GetCurrentECLForPociInstrumen(_ context.Context, _, _ uuid.UUID, _ string) (decimal.Decimal, error) {
	return decimal.Zero, errors.New("db error getting current ECL")
}

// ─── Worker — updateJobProgress/Complete/Failed with empty jobID ─────────────

func TestWorkerProgressHelpers_EmptyJobID(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	w := NewWorker(svc, nil, slog.Default())

	// Empty jobID → helpers return early, no panic
	w.updateJobProgress(context.Background(), "", 50, "step")
	w.updateJobComplete(context.Background(), "", 0)
	w.updateJobFailed(context.Background(), "", "error")
}

// ─── NewComputeDeltaTask — coverage for non-zero uuid fields ─────────────────

func TestNewComputeDeltaTask_FullPayloadRoundtrip(t *testing.T) {
	calcRunID := uuid.New()
	actorID := uuid.New()
	tenantID := "TUGURE" // M1 fix: tenantID is string, not uuid.UUID
	jobID := uuid.New()

	task, err := NewComputeDeltaTask(calcRunID, actorID, tenantID, jobID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var p ComputeDeltaPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if p.CalcRunID != calcRunID.String() {
		t.Fatalf("calc_run_id mismatch")
	}
	if p.JobID != jobID.String() {
		t.Fatalf("job_id mismatch")
	}
}

// ─── ComputeDeltaBatch handler — valid JSON body with nil asynq → 501 ─────────

func TestComputeDeltaBatch_ValidJSON_NilAsynq_Returns501(t *testing.T) {
	repo := &stubRepo{}
	h := NewHTTPHandler(makeService(repo), nil) // nil asynq
	r := setupRouter(h)

	body, _ := json.Marshal(ComputeDeltaBatchRequest{CalcRunID: uuid.New()})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/poci/compute-delta-batch",
		bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", w.Code, w.Body.String())
	}
}

// ─── GetDeltaHistory — service non-code error path (generic) ─────────────────

func TestGetDeltaHistory_ServiceError_Returns500(t *testing.T) {
	instrID := uuid.New()
	// baseline exists but GetDeltaHistoryByInstrumen returns internal error
	repo := &stubRepoHistoryErr{
		stubRepo: stubRepo{
			baseline: &Baseline{InstrumenID: instrID},
		},
	}
	h := NewHTTPHandler(makeService(repo), nil)
	r := setupRouter(h)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/poci/delta-history?instrumen_id="+instrID.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code < 400 {
		t.Fatalf("expected error status, got %d", w.Code)
	}
}

type stubRepoHistoryErr struct {
	stubRepo
}

func (r *stubRepoHistoryErr) GetDeltaHistoryByInstrumen(_ context.Context, _ uuid.UUID, _ listquery.Query, _ string) ([]DeltaLog, Pagination, error) {
	return nil, Pagination{}, errors.New("internal db error")
}

// ─── CaptureBaseline — GetBaselineByInstrumen error ──────────────────────────

func TestCaptureBaseline_GetBaselineError(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepoBaselineCheckErr{
		stubRepo: stubRepo{
			instrumenInfo: &InstrumenPociInfo{ID: instrID, IsPoci: true, Status: "ACTIVE"},
		},
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              instrID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.05),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error when GetBaselineByInstrumen fails")
	}
}

type stubRepoBaselineCheckErr struct {
	stubRepo
}

func (r *stubRepoBaselineCheckErr) GetBaselineByInstrumen(_ context.Context, _ uuid.UUID, _ string) (*Baseline, error) {
	return nil, errors.New("db error")
}

// ─── InsertBaseline — GetInstrumenPociInfo error ──────────────────────────────

func TestCaptureBaseline_GetInstrumenError(t *testing.T) {
	repo := &stubRepoInstrErr{}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.05),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error when GetInstrumenPociInfo fails")
	}
}

type stubRepoInstrErr struct {
	stubRepo
}

func (r *stubRepoInstrErr) GetInstrumenPociInfo(_ context.Context, _ uuid.UUID, _ string) (*InstrumenPociInfo, error) {
	return nil, errors.New("db error")
}

// ─── InsertBaseline via CaptureBaseline — InsertBaseline error ────────────────

func TestCaptureBaseline_InsertBaselineError(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepoInsertBaselineErr{
		stubRepo: stubRepo{
			instrumenInfo: &InstrumenPociInfo{ID: instrID, IsPoci: true, Status: "ACTIVE"},
			baseline:      nil,
		},
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              instrID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.05),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error when InsertBaseline fails")
	}
}

type stubRepoInsertBaselineErr struct {
	stubRepo
}

func (r *stubRepoInsertBaselineErr) InsertBaseline(_ context.Context, _ *sql.Tx, _ *Baseline) error {
	return errors.New("constraint violation")
}

// ─── db_adapter.go — NewSQLDBAdapter + BeginTxContext (B1, 0% → covered) ─────

func TestSQLDBAdapter_NewAndBeginTx(t *testing.T) {
	// Use sqlmock to get a *sql.DB (driver-level; BeginTx on sqlmock db works fine).
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock open: %v", err)
	}
	defer db.Close()

	adapter := NewSQLDBAdapter(db)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}

	// BeginTxContext — happy path
	mock.ExpectBegin()
	tx, err := adapter.BeginTxContext(context.Background())
	if err != nil {
		t.Fatalf("BeginTxContext: %v", err)
	}
	if tx == nil {
		t.Fatal("expected non-nil tx")
	}
	mock.ExpectCommit()
	_ = tx.Commit()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLDBAdapter_NilDB_ReturnsNil(t *testing.T) {
	adapter := NewSQLDBAdapter(nil)
	if adapter != nil {
		t.Fatal("expected nil adapter for nil db")
	}
}

// ─── NewJurnalPosterStub nil-logger branch (66.7% → 100%) ────────────────────

func TestNewJurnalPosterStub_NilLogger_Defaults(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	if stub == nil {
		t.Fatal("expected non-nil stub")
	}
	if stub.logger == nil {
		t.Fatal("expected logger defaulted to slog.Default()")
	}
}

// ─── NewNoopJurnalPoster nil-logger branch (75% → 100%) ──────────────────────

func TestNewNoopJurnalPoster_NilLogger_Defaults(t *testing.T) {
	noop := NewNoopJurnalPoster(nil)
	if noop == nil {
		t.Fatal("expected non-nil noop")
	}
	if noop.logger == nil {
		t.Fatal("expected logger defaulted to slog.Default()")
	}
}

// ─── postJurnalForDelta — direction mismatch (0% → partial covered) ──────────

// TestPostJurnalForDelta_DirectionMismatch triggers ValidateJurnalDirection error path.
// Delta > 0 but direction = DECREASE → mismatch.
func TestPostJurnalForDelta_DirectionMismatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, _ := db.Begin()
	defer func() { _ = tx.Rollback() }()

	repo := &stubRepo{}
	svc := makeService(repo)

	dl := &DeltaLog{
		ID:             uuid.New(),
		CalcRunID:      uuid.New(),
		InstrumenID:    uuid.New(),
		TanggalCompute: time.Now(),
		BaselineECL:    decimal.NewFromFloat(1000000),
		CurrentECL:     decimal.NewFromFloat(1200000),
		// delta > 0 but direction says DECREASE → mismatch
		DeltaECL:  decimal.NewFromFloat(200000),
		Direction: DirectionDecrease,
	}

	postErr := svc.postJurnalForDelta(context.Background(), tx, dl, uuid.New(), "TUGURE")
	if postErr == nil {
		t.Fatal("expected direction mismatch error")
	}
}

// TestPostJurnalForDelta_HappyPath exercises the non-mismatch flow via noop poster.
// Delta > 0 with direction INCREASE → poster is called → UpdateDeltaLogStatus called.
func TestPostJurnalForDelta_HappyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, _ := db.Begin()
	defer func() { _ = tx.Rollback() }()

	// Stub repo that succeeds UpdateDeltaLogStatus
	repo := &stubRepo{}
	svc := makeService(repo)

	dl := &DeltaLog{
		ID:             uuid.New(),
		CalcRunID:      uuid.New(),
		InstrumenID:    uuid.New(),
		TanggalCompute: time.Now(),
		BaselineECL:    decimal.NewFromFloat(1000000),
		CurrentECL:     decimal.NewFromFloat(1200000),
		DeltaECL:       decimal.NewFromFloat(200000),
		Direction:      DirectionIncrease,
	}

	// The noop poster succeeds; repo.UpdateDeltaLogStatus is a noop stub.
	postErr := svc.postJurnalForDelta(context.Background(), tx, dl, uuid.New(), "TUGURE")
	if postErr != nil {
		t.Fatalf("unexpected error: %v", postErr)
	}
}
