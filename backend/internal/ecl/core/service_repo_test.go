package core

// service_repo_test.go — Tests for ECLOrchestrator wrapper methods that
// delegate to CalcResultLineRepo (GetResult, ListResults, GetPortfolioSummary,
// GetRollForward, loadLatestStoredResult).
//
// Uses go-sqlmock to provide a mock DB to the orchestrator's resultRepo.
//
// Worker task handler (Handle) and NewBulkWorker also covered here.

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
)

// buildOrchestratorWithDB creates an ECLOrchestrator with a real sqlmock DB.
// Needed for methods that call resultRepo (GetResult, ListResults, etc.).
func buildOrchestratorWithDB(t *testing.T) (*ECLOrchestrator, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	aw := audit.NewWriter(nil) // audit not called in these tests
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	svc := buildMockHelpers(
		decimal.NewFromFloat(0.02),
		decimal.NewFromFloat(1.0),
		decimal.NewFromFloat(0.4),
		decimal.NewFromInt(1_000_000_000),
	)

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: aw,
		helpers:     svc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}
	return orch, mock
}

// ─── GetResult ────────────────────────────────────────────────────────────────

func TestOrchestrator_GetResult_Found(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	calcRunID := uuid.New()
	instrID := uuid.New()
	lineID := uuid.New()
	evalDate := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	createdAt := time.Now()

	cols := resultLineRowCols()
	rows := sqlmock.NewRows(cols).AddRow(
		lineID, calcRunID, instrID, evalDate, "JUNI-2026", 1, "STANDARD",
		"1000000000.0000", "0.02000000", "0.02000000", "0.02000000", "0.40000000",
		"1.10000000", "1.00000000", "0.90000000",
		"8000000.0000", "8000000.0000", "8000000.0000",
		"8800000.0000", "8000000.0000", "7200000.0000",
		"8200000.0000", "0.2500", "0.5000", "0.2500",
		nil, nil, false, nil,
		"[]", nil, createdAt,
		FormulaVersionM7, // F8 fix: formula_version column
	)

	mock.ExpectQuery(`SELECT id, calc_run_id`).
		WithArgs(calcRunID, instrID).
		WillReturnRows(rows)

	row, err := orch.GetResult(context.Background(), calcRunID, instrID)
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if row == nil {
		t.Fatal("expected non-nil row")
	}
	if row.Stage != Stage1 {
		t.Errorf("Stage: want 1, got %d", row.Stage)
	}
}

func TestOrchestrator_GetResult_NotFound(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	calcRunID, instrID := uuid.New(), uuid.New()
	mock.ExpectQuery(`SELECT id, calc_run_id`).
		WithArgs(calcRunID, instrID).
		WillReturnError(sql.ErrNoRows)

	row, err := orch.GetResult(context.Background(), calcRunID, instrID)
	if err != nil {
		t.Fatalf("GetResult not found: %v", err)
	}
	if row != nil {
		t.Error("expected nil row")
	}
}

// ─── ListResults ──────────────────────────────────────────────────────────────

func TestOrchestrator_ListResults_Empty(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	calcRunID := uuid.New()
	cols := []string{"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id",
		"stage", "routing_path", "ead_idr", "ecl_weighted_idr", "flag_poci", "sealed_at", "created_at"}
	mock.ExpectQuery(`SELECT id, calc_run_id, instrumen_id`).
		WithArgs(calcRunID, 51).
		WillReturnRows(sqlmock.NewRows(cols))

	req := ListResultsRequest{CalcRunID: calcRunID, Limit: 50}
	resp, err := orch.ListResults(context.Background(), req)
	if err != nil {
		t.Fatalf("ListResults: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("items: want 0, got %d", len(resp.Items))
	}
}

// ─── GetPortfolioSummary ──────────────────────────────────────────────────────

func TestOrchestrator_GetPortfolioSummary_OK(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	calcRunID, portfolioID := uuid.New(), uuid.New()
	cols := []string{"stage_label", "cnt", "ead_total", "ecl_total"}
	rows := sqlmock.NewRows(cols).
		AddRow("STAGE_1", 5, "500000000.0000", "2000000.0000").
		AddRow("STAGE_2", 1, "100000000.0000", "500000.0000")

	mock.ExpectQuery(`SELECT`).
		WithArgs(calcRunID, portfolioID).
		WillReturnRows(rows)

	req := PortfolioSummaryRequest{
		PortofolioID: portfolioID,
		CalcRunID:    calcRunID,
		ActorID:      uuid.New(),
	}

	summary, err := orch.GetPortfolioSummary(context.Background(), req)
	if err != nil {
		t.Fatalf("GetPortfolioSummary: %v", err)
	}
	if len(summary.SummaryByStage) != 3 { // STAGE_1, STAGE_2, TOTAL
		t.Errorf("SummaryByStage: want 3, got %d", len(summary.SummaryByStage))
	}
	// Total ECL = 2000000 + 500000 = 2500000.
	want := decimal.NewFromFloat(2_500_000.0)
	if !summary.ECLWeightedIDRTotal.Equal(want) {
		t.Errorf("ECLWeightedIDRTotal: want %s, got %s", want, summary.ECLWeightedIDRTotal)
	}
}

// ─── GetRollForward ───────────────────────────────────────────────────────────

func TestOrchestrator_GetRollForward_Minimal(t *testing.T) {
	t.Parallel()
	orch, mock := buildOrchestratorWithDB(t)

	calcRunID := uuid.New()

	// GetRollForward calls GetCalcRunECLTotal for current run.
	mock.ExpectQuery(`SELECT COALESCE`).
		WithArgs(calcRunID).
		WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow("5000000.0000"))

	req := RollForwardRequest{
		CalcRunID: calcRunID,
		ActorID:   uuid.New(),
	}

	report, err := orch.GetRollForward(context.Background(), req)
	if err != nil {
		t.Fatalf("GetRollForward: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	// ClosingECL = current run total.
	want := decimal.NewFromFloat(5_000_000.0)
	if !report.ClosingECLIDR.Equal(want) {
		t.Errorf("ClosingECLIDR: want %s, got %s", want, report.ClosingECLIDR)
	}
}

// ─── Worker task Handle ───────────────────────────────────────────────────────

func TestBulkWorker_Handle_EmptyScope(t *testing.T) {
	t.Parallel()

	reader := &mockInstrumenReader{
		byID:        map[uuid.UUID]*InstrumenSnapshot{},
		activeScope: []InstrumenSnapshot{},
	}
	svc := buildMockHelpers(
		decimal.NewFromFloat(0.01),
		decimal.NewFromFloat(1.0),
		decimal.NewFromFloat(0.4),
		decimal.NewFromInt(1e9),
	)
	orch := &ECLOrchestrator{
		db:          nil,
		auditWriter: nil,
		helpers:     svc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  nil,
		logger:      slog.Default(),
	}

	payload := TaskECLBulkComputePayload{
		JobID:          uuid.New().String(),
		CalcRunID:      uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		ActorID:        uuid.New(),
	}
	b, _ := json.Marshal(payload)
	task := asynq.NewTask(TaskNameECLBulkCompute, b)

	worker := NewBulkWorker(orch, nil, slog.Default())
	if err := worker.Handle(context.Background(), task); err != nil {
		t.Fatalf("Handle empty scope: %v", err)
	}
}

func TestBulkWorker_Handle_InvalidPayload(t *testing.T) {
	t.Parallel()

	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	orch := buildOrchestratorForTest(reader, nil, nil)
	// Override logger to avoid nil panic.
	orch.logger = slog.Default()

	task := asynq.NewTask(TaskNameECLBulkCompute, []byte("not-valid-json"))
	worker := NewBulkWorker(orch, nil, slog.Default())

	err := worker.Handle(context.Background(), task)
	if err == nil {
		t.Fatal("Handle invalid payload: expected error")
	}
}

// ─── NewECLBulkComputeTask ────────────────────────────────────────────────────

func TestNewECLBulkComputeTask_OK(t *testing.T) {
	t.Parallel()

	payload := TaskECLBulkComputePayload{
		JobID:          uuid.New().String(),
		CalcRunID:      uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
	}
	task, err := NewECLBulkComputeTask(payload)
	if err != nil {
		t.Fatalf("NewECLBulkComputeTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.Type() != TaskNameECLBulkCompute {
		t.Errorf("task type: want %s, got %s", TaskNameECLBulkCompute, task.Type())
	}
	// Payload must be valid JSON.
	var roundtrip TaskECLBulkComputePayload
	if err := json.Unmarshal(task.Payload(), &roundtrip); err != nil {
		t.Fatalf("payload roundtrip: %v", err)
	}
	if roundtrip.CalcRunID != payload.CalcRunID {
		t.Errorf("CalcRunID: want %s, got %s", payload.CalcRunID, roundtrip.CalcRunID)
	}
}

// ─── RegisterRoutes ───────────────────────────────────────────────────────────

func TestRegisterRoutes_OK(t *testing.T) {
	t.Parallel()
	// Verify RegisterRoutes panics on nil handler OR registers cleanly.
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	orch := buildOrchestratorForTest(reader, nil, nil)
	orch.logger = slog.Default()

	h := NewHandler(orch)

	// Create a gin router and register routes.
	// gin.SetMode is set in handler_test.go init() — no need to call it again here
	// as it causes a race condition with parallel tests.
	router := gin.New()
	rg := router.Group("/api/v1")
	RegisterRoutes(rg, h)

	// Verify at least one route is registered.
	routes := router.Routes()
	if len(routes) == 0 {
		t.Error("RegisterRoutes: no routes registered")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func resultLineRowCols() []string {
	return []string{
		"id", "calc_run_id", "instrumen_id", "evaluation_date", "periode_id", "stage", "routing_path",
		"ead_idr", "pd_used_good", "pd_used_normal", "pd_used_bad", "lgd_used",
		"fl_multiplier_good", "fl_multiplier_normal", "fl_multiplier_bad",
		"ecl_good_idr", "ecl_normal_idr", "ecl_bad_idr",
		"ecl_fl_good_idr", "ecl_fl_normal_idr", "ecl_fl_bad_idr",
		"ecl_weighted_idr", "bobot_good", "bobot_normal", "bobot_bad",
		"net_carrying_idr", "prior_sealed_ecl_idr", "flag_poci", "parameter_snapshot_id",
		"warnings_json", "sealed_at", "created_at",
		"formula_version", // F8 fix: migration 000030
	}
}
