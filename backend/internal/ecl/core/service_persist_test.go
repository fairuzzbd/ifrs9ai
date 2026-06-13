package core

// service_persist_test.go — tests for persist=true paths in:
//   - handleSkipFVTPL (Persist=true + CalcRunID set → audit tx)
//   - handlePOCI      (Persist=true + CalcRunID set → audit tx)
//   - applyFormulaAndPersist (Stage1 Persist=true → INSERT + audit)
//   - applyFormulaAndPersist (Stage3 Persist=true → GetPriorSealedECL + INSERT + audit)
//   - ComputeBulk with FVTPL instrument (fan-out, idempotency, skippedFVTPL counter)
//   - NewOrchestrator valid construction (all non-nil required deps)
//   - resolveStage Stage2 and Stage3 branches

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/ecl/helpers"
)

// ─── NewOrchestrator valid construction ───────────────────────────────────────

func TestNewOrchestrator_Valid(t *testing.T) {
	t.Parallel()

	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	aw := audit.NewWriter(db)
	svc := buildMockHelpers(
		decimal.NewFromFloat(0.02),
		decimal.NewFromFloat(1.0),
		decimal.NewFromFloat(0.4),
		decimal.NewFromInt(1_000_000_000),
	)
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	bobotRepo := &mockBobotRepo{bobot: defaultBobot()}

	orch := NewOrchestrator(db, aw, svc, nil, nil, reader, bobotRepo, nil)
	if orch == nil {
		t.Fatal("NewOrchestrator: must not return nil")
	}
	// logger nil → defaults to slog.Default()
	if orch.logger == nil {
		t.Error("NewOrchestrator: logger must default to slog.Default() when nil")
	}
	if orch.resultRepo == nil {
		t.Error("NewOrchestrator: resultRepo must be wired")
	}
}

func TestNewOrchestrator_PanicsOnNilDB(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil db")
		}
	}()
	aw := audit.NewWriter(nil)
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	svc := buildMockHelpers(decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero)
	_ = NewOrchestrator(nil, aw, svc, nil, nil, reader, &mockBobotRepo{}, nil)
}

func TestNewOrchestrator_PanicsOnNilAuditWriterPersist(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil auditWriter")
		}
	}()
	db, _, _ := sqlmock.New()
	t.Cleanup(func() { db.Close() })
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	svc := buildMockHelpers(decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero)
	_ = NewOrchestrator(db, nil, svc, nil, nil, reader, &mockBobotRepo{}, nil)
}

func TestNewOrchestrator_PanicsOnNilHelpers(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil helperSvcs")
		}
	}()
	db, _, _ := sqlmock.New()
	t.Cleanup(func() { db.Close() })
	aw := audit.NewWriter(db)
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	_ = NewOrchestrator(db, aw, nil, nil, nil, reader, &mockBobotRepo{}, nil)
}

func TestNewOrchestrator_PanicsOnNilInstrReader(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil instrReader")
		}
	}()
	db, _, _ := sqlmock.New()
	t.Cleanup(func() { db.Close() })
	aw := audit.NewWriter(db)
	svc := buildMockHelpers(decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero)
	_ = NewOrchestrator(db, aw, svc, nil, nil, nil, &mockBobotRepo{}, nil)
}

func TestNewOrchestrator_PanicsOnNilBobotRepo(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil bobotRepo")
		}
	}()
	db, _, _ := sqlmock.New()
	t.Cleanup(func() { db.Close() })
	aw := audit.NewWriter(db)
	svc := buildMockHelpers(decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero)
	reader := &mockInstrumenReader{byID: map[uuid.UUID]*InstrumenSnapshot{}}
	_ = NewOrchestrator(db, aw, svc, nil, nil, reader, nil, nil)
}

// ─── handleSkipFVTPL persist=true ────────────────────────────────────────────

func TestComputeSingle_FVTPL_PersistTrue_AuditTx(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	calcRunID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "FVTPL",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// handleSkipFVTPL persist=true: BeginTx → audit hash query (swallowed) → INSERT audit → Commit.
	mock.ExpectBegin()
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers:     buildMockHelpers(decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		CalcRunID:      &calcRunID,
		Persist:        true,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("FVTPL persist=true: %v", err)
	}
	if result.RoutingPath != RoutingSkipFVTPL {
		t.Errorf("routing: want SKIP_FVTPL, got %s", result.RoutingPath)
	}
	if result.ECLWeightedIDR == nil || !result.ECLWeightedIDR.IsZero() {
		t.Error("FVTPL: ECLWeightedIDR must be zero")
	}
}

// ─── handlePOCI persist=true ─────────────────────────────────────────────────

func TestComputeSingle_POCI_PersistTrue_AuditTx(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	calcRunID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
				FlagPOCI:          true,
			},
		},
	}

	// F2 fix: POCI now computes via STANDARD path, so persist=true generates:
	// 1. BEGIN tx
	// 2. INSERT ecl.calc_result_line (32 args: $1-$31 data + $32 actor)
	// 3. INSERT aud.audit_log (15 args)
	// 4. COMMIT
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.calc_result_line").
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO aud.audit_log").
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(),
		).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers:     buildMockHelpers(decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		CalcRunID:      &calcRunID,
		Persist:        true,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("POCI persist=true: %v", err)
	}
	// Phase 4.5 (DEC-POCI-001): routing_path = POCI_COMPUTED (CA-EIR available, baseline persisted).
	if result.RoutingPath != RoutingPOCIComputed {
		t.Errorf("routing: want POCI_COMPUTED, got %s", result.RoutingPath)
	}
	// Phase 4.5: POCI computes baseline ECL via STANDARD → ECLWeightedIDR is non-nil.
	if result.ECLWeightedIDR == nil {
		t.Error("Phase 4.5: POCI ECLWeightedIDR must be non-nil (initial baseline ECL computed)")
	}
	// Verify both POCI warning codes are present.
	wantWarnings := []string{WarnPOCIRequiresFullCAEIR, WarnPOCIECLRepresentsInitialBaseline}
	for _, want := range wantWarnings {
		found := false
		for _, w := range result.Warnings {
			if w == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("POCI persist: expected warning %s, got %v", want, result.Warnings)
		}
	}
}

// ─── applyFormulaAndPersist Stage1 persist=true ───────────────────────────────

func TestComputeSingle_Standard_Stage1_PersistTrue(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	calcRunID := uuid.New()
	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// applyFormulaAndPersist persist=true (Stage1 — no GetPriorSealedECL):
	//   BeginTx → INSERT calc_result_line → INSERT aud.audit_log → Commit
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.calc_result_line").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO aud.audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers: buildMockHelpers(
			decimal.NewFromFloat(0.01),
			decimal.NewFromFloat(1.0),
			decimal.NewFromFloat(0.4),
			decimal.NewFromInt(1_000_000_000),
		),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		CalcRunID:      &calcRunID,
		Persist:        true,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("Stage1 persist=true: %v", err)
	}
	if result.ResultLineID == nil {
		t.Error("ResultLineID must be non-nil when Persist=true")
	}
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR must not be nil")
	}
	// ECL = 1_000_000_000 × 0.01 × 0.4 = 4_000_000
	want := decimal.NewFromFloat(4_000_000.0)
	if !result.ECLWeightedIDR.Equal(want) {
		t.Errorf("ECLWeightedIDR: want %s, got %s", want, result.ECLWeightedIDR)
	}
}

// ─── applyFormulaAndPersist Stage3 persist=true ───────────────────────────────

func TestComputeSingle_Standard_Stage3_PersistTrue(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	instrID := uuid.New()
	calcRunID := uuid.New()

	pdSvc := &mockPDServiceStage{
		pdBase: decimal.NewFromFloat(0.00), // will be overridden to 1.0 for Stage3
		flMult: decimal.NewFromFloat(1.0),
		stage:  helpers.Stage3,
	}
	svc := &helpers.Services{
		PD:   pdSvc,
		LGD:  &mockLGDService{lgd: decimal.NewFromFloat(0.50)},
		EAD:  &mockEADService{eadIDR: decimal.NewFromInt(500_000_000)},
		CCF:  &mockCCFService{},
		Bulk: &mockBulkHelperService{},
	}

	reader := &mockInstrumenReader{
		byID: map[uuid.UUID]*InstrumenSnapshot{
			instrID: {
				ID:                instrID,
				KlasifikasiPsak71: "AC",
				TipeInstrumen:     "OBLIGASI",
			},
		},
	}

	// Stage3: GetPriorSealedECL (SELECT → ErrNoRows swallowed as nil), then persist tx.
	// GetPriorSealedECL:
	mock.ExpectQuery("SELECT ecl_weighted_idr FROM ecl.calc_result_line").
		WithArgs(instrID).
		WillReturnError(sql.ErrNoRows)

	// applyFormulaAndPersist tx:
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO ecl.calc_result_line").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO aud.audit_log").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers:     svc,
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		CalcRunID:      &calcRunID,
		Persist:        true,
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeSingle(context.Background(), req)
	if err != nil {
		t.Fatalf("Stage3 persist=true: %v", err)
	}
	if result.Stage != Stage3 {
		t.Errorf("stage: want 3, got %d", result.Stage)
	}
	// Stage3: PD=1.0 fixed, FL not applied.
	// ECL = 500_000_000 × 1.0 × 0.50 = 250_000_000.
	if result.ECLWeightedIDR == nil {
		t.Fatal("ECLWeightedIDR: must not be nil for Stage3")
	}
	want := decimal.NewFromFloat(250_000_000.0)
	if !result.ECLWeightedIDR.Equal(want) {
		t.Errorf("ECLWeightedIDR Stage3: want %s, got %s", want, result.ECLWeightedIDR)
	}
	// Stage3: FLMultiplierPerScenario must be nil.
	if result.FLMultiplierPerScenario != nil {
		t.Error("FLMultiplierPerScenario: must be nil for Stage3")
	}
	// Stage3: warn WarnStage3NetCarryingFirstRun because prior ECL is nil.
	found := false
	for _, w := range result.Warnings {
		if w == WarnStage3NetCarryingFirstRun {
			found = true
		}
	}
	if !found {
		t.Errorf("Warnings: expected %s for Stage3 first run, got %v", WarnStage3NetCarryingFirstRun, result.Warnings)
	}
}

// ─── resolveStage Stage2 / Stage3 ────────────────────────────────────────────

func TestResolveStage_Stage2(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	pdSvc := &mockPDServiceStage{
		pdBase: decimal.NewFromFloat(0.05),
		flMult: decimal.NewFromFloat(1.1),
		stage:  helpers.Stage2,
	}
	orch := &ECLOrchestrator{
		helpers: &helpers.Services{
			PD:   pdSvc,
			LGD:  &mockLGDService{},
			EAD:  &mockEADService{},
			CCF:  &mockCCFService{},
			Bulk: &mockBulkHelperService{},
		},
	}

	stage, err := orch.resolveStage(context.Background(), instrID)
	if err != nil {
		t.Fatalf("resolveStage Stage2: %v", err)
	}
	if stage != Stage2 {
		t.Errorf("stage: want Stage2, got %d", stage)
	}
}

func TestResolveStage_Stage3(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	pdSvc := &mockPDServiceStage{
		pdBase: decimal.NewFromFloat(1.0),
		flMult: decimal.NewFromFloat(1.0),
		stage:  helpers.Stage3,
	}
	orch := &ECLOrchestrator{
		helpers: &helpers.Services{
			PD:   pdSvc,
			LGD:  &mockLGDService{},
			EAD:  &mockEADService{},
			CCF:  &mockCCFService{},
			Bulk: &mockBulkHelperService{},
		},
	}

	stage, err := orch.resolveStage(context.Background(), instrID)
	if err != nil {
		t.Fatalf("resolveStage Stage3: %v", err)
	}
	if stage != Stage3 {
		t.Errorf("stage: want Stage3, got %d", stage)
	}
}

// ─── ComputeBulk fan-out with FVTPL instrument ────────────────────────────────
//
// Covers: fan-out goroutine, idempotency check (ExistsResultLine), RoutingSkipFVTPL
// counter, completed_with_errors path.

// TestComputeBulk_WithSingleFVTPLInstrument covers: fan-out goroutine, idempotency check
// (ExistsResultLine=false → compute), RoutingSkipFVTPL counter.
// Uses a single instrument to avoid sqlmock ordered-expectations race with goroutines.
func TestComputeBulk_WithSingleFVTPLInstrument(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	instruments := []InstrumenSnapshot{
		{ID: instrID, KlasifikasiPsak71: "FVTPL", TipeInstrumen: "OBLIGASI"},
	}
	reader := &mockInstrumenReader{
		byID:        map[uuid.UUID]*InstrumenSnapshot{instrID: &instruments[0]},
		activeScope: instruments,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	calcRunID := uuid.New()
	// ExistsResultLine → false → proceed to compute.
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(calcRunID, instrID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	// FVTPL handleSkipFVTPL with Persist=true → BeginTx + audit INSERT + Commit.
	mock.ExpectBegin()
	mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := BulkComputeRequest{
		CalcRunID:      calcRunID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeBulk(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("ComputeBulk FVTPL: %v", err)
	}
	if result.TotalScanned != 1 {
		t.Errorf("TotalScanned: want 1, got %d", result.TotalScanned)
	}
	if result.TotalSkippedFVTPL != 1 {
		t.Errorf("TotalSkippedFVTPL: want 1, got %d", result.TotalSkippedFVTPL)
	}
}

// TestComputeBulk_WithIdempotencyDuplicate verifies that an already-computed
// instrument (ExistsResultLine=true) is counted as skippedDuplicate.
func TestComputeBulk_WithIdempotencyDuplicate(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	instruments := []InstrumenSnapshot{
		{ID: instrID, KlasifikasiPsak71: "FVTPL", TipeInstrumen: "OBLIGASI"},
	}
	reader := &mockInstrumenReader{
		byID:        map[uuid.UUID]*InstrumenSnapshot{instrID: &instruments[0]},
		activeScope: instruments,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	calcRunID := uuid.New()
	// ExistsResultLine → true (already computed).
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(calcRunID, instrID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := BulkComputeRequest{
		CalcRunID:      calcRunID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeBulk(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("ComputeBulk idempotency: %v", err)
	}
	if result.TotalSkippedDuplicate != 1 {
		t.Errorf("TotalSkippedDuplicate: want 1, got %d", result.TotalSkippedDuplicate)
	}
}

// TestComputeBulk_CompletedWithErrors verifies completed_with_errors status
// when ExistsResultLine query fails.
func TestComputeBulk_CompletedWithErrors(t *testing.T) {
	t.Parallel()

	instrID := uuid.New()
	instruments := []InstrumenSnapshot{
		{ID: instrID, KlasifikasiPsak71: "FVTPL", TipeInstrumen: "OBLIGASI"},
	}
	reader := &mockInstrumenReader{
		byID:        map[uuid.UUID]*InstrumenSnapshot{instrID: &instruments[0]},
		activeScope: instruments,
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	calcRunID := uuid.New()
	// ExistsResultLine → error → treated as error item.
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(calcRunID, instrID).
		WillReturnError(sql.ErrConnDone)

	orch := &ECLOrchestrator{
		db:          db,
		auditWriter: audit.NewWriter(db),
		helpers:     buildMockHelpers(decimal.NewFromFloat(0.01), decimal.NewFromFloat(1.0), decimal.NewFromFloat(0.4), decimal.NewFromInt(1e9)),
		instrReader: reader,
		bobotRepo:   &mockBobotRepo{bobot: defaultBobot()},
		resultRepo:  NewCalcResultLineRepo(db),
		logger:      slog.Default(),
	}

	req := BulkComputeRequest{
		CalcRunID:      calcRunID,
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		ActorID:        uuid.New(),
	}

	result, err := orch.ComputeBulk(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("ComputeBulk with errors: %v", err)
	}
	if result.Status != "completed_with_errors" {
		t.Errorf("status: want completed_with_errors, got %s", result.Status)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors: want 1, got %d", len(result.Errors))
	}
}
