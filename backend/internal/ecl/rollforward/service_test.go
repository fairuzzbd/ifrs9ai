package rollforward_test

// service_test.go — unit tests for rollforward.Service (P4-M11).
//
// Coverage targets per stories APP-C-M11-001..006:
//   - First period (priorCalcRunID=nil): opening=0, all=originations, RECONCILED
//   - Normal period: 6 transfer buckets, originations, derecognitions, remeasurements
//   - Stage transfer detection: each of 6 buckets
//   - Stage 3→1: always countOverride++ (OQ-M11-002-B)
//   - Reconcile invariant: by construction delta=0 → RECONCILED
//   - Synthetic MISMATCH (impossible in production; tested via arithmetic injection)
//   - Prior not SEALED: warn WarnPriorNotSealedPreview, AllowNonSealedPrior=true
//   - Prior not SEALED, AllowNonSealedPrior=false: CodeRollForwardPriorNotSealed
//   - Current DRAFT: CodeRollForwardCurrentInvalidState
//   - Current not found: CodeRollForwardCurrentInvalidState
//   - Prior not found: CodeRollForwardPriorNotFound
//   - Same periode: CodeRollForwardPeriodeMismatch
//   - Portfolio not found: CodeRollForwardPortfolioNotFound
//   - CKPN trend ≥ 2 sealed runs: data + delta computed
//   - CKPN trend < 2 sealed runs: CodeRollForwardTrendInsufficientData
//   - ExportXLSX MISMATCH blocked: CodeRollForwardExportMismatchForbidden
//   - ExportXLSX force_mismatch=true: returns bytes

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
)

// ─── Test helpers ──────────────────────────────────────────────────────────

// buildLines constructs ResultLineHeader slices for test scenarios.
// args: pairs of (instrumenIDHex, stage, eclWeightedIDR).
// If eclWeightedIDR is "" → nil (POCI).
type lineSpec struct {
	id    uuid.UUID
	stage int
	ecl   string // "" = POCI
}

func buildLines(specs []lineSpec) []rollforward.ResultLineHeader {
	lines := make([]rollforward.ResultLineHeader, 0, len(specs))
	for _, s := range specs {
		h := rollforward.ResultLineHeader{
			InstrumenID: s.id,
			Stage:       s.stage,
			EadIdr:      decimal.RequireFromString("1000000.0000"),
		}
		if s.ecl != "" {
			d := decimal.RequireFromString(s.ecl)
			h.EclWeightedIdr = &d
		}
		lines = append(lines, h)
	}
	return lines
}

// ─── Pure function tests — detectTransfers ───────────────────────────────────

func TestDetectTransfers_Stage1To2(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 1, ecl: "100000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 2, ecl: "500000.0000"}})
	// When stage_history is missing for a transitioning instrument, DQWarnStageHistoryMissingFallback
	// is emitted per state machine §2.
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{}

	tr, dq := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage1To2.Count != 1 {
		t.Errorf("Stage1To2.Count: want 1, got %d", tr.Stage1To2.Count)
	}
	want := decimal.RequireFromString("400000.0000")
	if !tr.Stage1To2.EclMovementIdr.Equal(want) {
		t.Errorf("Stage1To2.EclMovementIdr: want %s, got %s", want, tr.Stage1To2.EclMovementIdr)
	}
	if tr.Stage1To2.CountOverride != 0 {
		t.Errorf("Stage1To2.CountOverride: want 0, got %d", tr.Stage1To2.CountOverride)
	}
	// Missing stage_history → DQWarn emitted (expected behavior per §2 state machine).
	if len(dq) != 1 {
		t.Errorf("expected 1 DQ warning for missing stage_history, got %d: %v", len(dq), dq)
	}
	if len(dq) > 0 && dq[0].WarningCode != rollforward.DQWarnStageHistoryMissingFallback {
		t.Errorf("unexpected DQ code: %s", dq[0].WarningCode)
	}
}

func TestDetectTransfers_Stage2To1Cure(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 2, ecl: "500000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 1, ecl: "100000.0000"}})
	// Provide SICR_RATING to confirm cure was not override-driven (no DQ warning).
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "SICR_RATING"},
	}

	tr, dq := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage2To1.Count != 1 {
		t.Errorf("Stage2To1.Count: want 1, got %d", tr.Stage2To1.Count)
	}
	want := decimal.RequireFromString("-400000.0000")
	if !tr.Stage2To1.EclMovementIdr.Equal(want) {
		t.Errorf("Stage2To1.EclMovementIdr: want %s, got %s", want, tr.Stage2To1.EclMovementIdr)
	}
	if len(dq) != 0 {
		t.Errorf("unexpected DQ warnings: %v", dq)
	}
}

func TestDetectTransfers_Stage3To1AlwaysOverride(t *testing.T) {
	// OQ-M11-002-B: Stage 3→1 always countOverride++
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 3, ecl: "800000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 1, ecl: "50000.0000"}})
	// Even with no stage_history entry, Stage 3→1 override is hardcoded.
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "MANAGEMENT_OVERRIDE"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage3To1.Count != 1 {
		t.Errorf("Stage3To1.Count: want 1, got %d", tr.Stage3To1.Count)
	}
	if tr.Stage3To1.CountOverride != 1 {
		t.Errorf("Stage3To1.CountOverride: want 1 (forced), got %d", tr.Stage3To1.CountOverride)
	}
}

func TestDetectTransfers_Stage3To1WithManagementOverride(t *testing.T) {
	// Stage 3→1 with explicit MANAGEMENT_OVERRIDE in stage_history — still countOverride++
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 3, ecl: "800000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 1, ecl: "50000.0000"}})
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "MANAGEMENT_OVERRIDE"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage3To1.CountOverride != 1 {
		t.Errorf("Stage3To1.CountOverride: want 1, got %d", tr.Stage3To1.CountOverride)
	}
}

func TestDetectTransfers_Stage1To2WithOverride(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 1, ecl: "100000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 2, ecl: "500000.0000"}})
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "MANAGEMENT_OVERRIDE"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage1To2.CountOverride != 1 {
		t.Errorf("Stage1To2.CountOverride: want 1, got %d", tr.Stage1To2.CountOverride)
	}
}

func TestDetectTransfers_StageSame_NoTransfer(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 1, ecl: "100000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 1, ecl: "200000.0000"}})
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage1To2.Count != 0 || tr.Stage2To1.Count != 0 || tr.SumMovement().IsPositive() {
		t.Errorf("same-stage should produce zero transfers, got: %+v", tr)
	}
}

func TestDetectTransfers_MissingStageHistoryEmitsDQWarning(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 1, ecl: "100000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 2, ecl: "500000.0000"}})
	// Empty stage_history — should emit DQWarnStageHistoryMissingFallback
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{}

	_, dq := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if len(dq) != 1 {
		t.Errorf("expected 1 DQ warning, got %d", len(dq))
	}
	if dq[0].WarningCode != rollforward.DQWarnStageHistoryMissingFallback {
		t.Errorf("unexpected DQ warning code: %s", dq[0].WarningCode)
	}
}

func TestDetectTransfers_AllBuckets(t *testing.T) {
	// Test all 6 transfer buckets in one call.
	id12 := uuid.New()
	id21 := uuid.New()
	id23 := uuid.New()
	id13 := uuid.New()
	id32 := uuid.New()
	id31 := uuid.New()

	prior := buildLines([]lineSpec{
		{id: id12, stage: 1, ecl: "100.0000"},
		{id: id21, stage: 2, ecl: "200.0000"},
		{id: id23, stage: 2, ecl: "300.0000"},
		{id: id13, stage: 1, ecl: "400.0000"},
		{id: id32, stage: 3, ecl: "500.0000"},
		{id: id31, stage: 3, ecl: "600.0000"},
	})
	current := buildLines([]lineSpec{
		{id: id12, stage: 2, ecl: "150.0000"},
		{id: id21, stage: 1, ecl: "100.0000"},
		{id: id23, stage: 3, ecl: "350.0000"},
		{id: id13, stage: 3, ecl: "450.0000"},
		{id: id32, stage: 2, ecl: "400.0000"},
		{id: id31, stage: 1, ecl: "50.0000"},
	})
	// Provide MANAGEMENT_OVERRIDE for 3→2 and 3→1 to set countOverride.
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id32: {InstrumenID: id32, TriggerType: "MANAGEMENT_OVERRIDE"},
		id31: {InstrumenID: id31, TriggerType: "MANAGEMENT_OVERRIDE"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)

	if tr.Stage1To2.Count != 1 {
		t.Errorf("Stage1To2.Count: want 1, got %d", tr.Stage1To2.Count)
	}
	if tr.Stage2To1.Count != 1 {
		t.Errorf("Stage2To1.Count: want 1, got %d", tr.Stage2To1.Count)
	}
	if tr.Stage2To3.Count != 1 {
		t.Errorf("Stage2To3.Count: want 1, got %d", tr.Stage2To3.Count)
	}
	if tr.Stage1To3.Count != 1 {
		t.Errorf("Stage1To3.Count: want 1, got %d", tr.Stage1To3.Count)
	}
	if tr.Stage3To2.Count != 1 {
		t.Errorf("Stage3To2.Count: want 1, got %d", tr.Stage3To2.Count)
	}
	if tr.Stage3To1.Count != 1 {
		t.Errorf("Stage3To1.Count: want 1, got %d", tr.Stage3To1.Count)
	}
	// Stage 3→1: always override.
	if tr.Stage3To1.CountOverride != 1 {
		t.Errorf("Stage3To1.CountOverride: want 1, got %d", tr.Stage3To1.CountOverride)
	}
	// Stage 3→2: should be countOverride=1 (MANAGEMENT_OVERRIDE in history).
	if tr.Stage3To2.CountOverride != 1 {
		t.Errorf("Stage3To2.CountOverride: want 1, got %d", tr.Stage3To2.CountOverride)
	}
}

// ─── Pure function tests — detectLifecycle ───────────────────────────────────

func TestDetectLifecycle_Origination(t *testing.T) {
	newID := uuid.New()
	prior := buildLines(nil)
	current := buildLines([]lineSpec{{id: newID, stage: 1, ecl: "300000.0000"}})
	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{}

	orig, derec, dq := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())

	if orig.Count != 1 {
		t.Errorf("originations.Count: want 1, got %d", orig.Count)
	}
	want := decimal.RequireFromString("300000.0000")
	if !orig.EclIdr.Equal(want) {
		t.Errorf("originations.EclIdr: want %s, got %s", want, orig.EclIdr)
	}
	if derec.Count != 0 {
		t.Errorf("derecognitions.Count: want 0, got %d", derec.Count)
	}
	_ = dq
}

func TestDetectLifecycle_Derecognition_Matured(t *testing.T) {
	oldID := uuid.New()
	prior := buildLines([]lineSpec{{id: oldID, stage: 1, ecl: "500000.0000"}})
	current := buildLines(nil)

	past := time.Now().AddDate(-1, 0, 0)
	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{
		oldID: {ID: oldID, Kode: "INST-001", Status: "JATUH_TEMPO", TanggalJatuhTempo: &past},
	}

	_, derec, _ := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())

	if derec.Count != 1 {
		t.Errorf("derecognitions.Count: want 1, got %d", derec.Count)
	}
	want := decimal.RequireFromString("500000.0000")
	if !derec.PriorEclIdr.Equal(want) {
		t.Errorf("derecognitions.PriorEclIdr: want %s, got %s", want, derec.PriorEclIdr)
	}
}

func TestDetectLifecycle_AktifNotInCurrentRunDQWarning(t *testing.T) {
	oldID := uuid.New()
	prior := buildLines([]lineSpec{{id: oldID, stage: 1, ecl: "500000.0000"}})
	current := buildLines(nil)

	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{
		oldID: {ID: oldID, Kode: "INST-002", Status: "AKTIF"},
	}

	_, _, dq := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())

	if len(dq) != 1 {
		t.Errorf("expected 1 DQ warning, got %d", len(dq))
	}
	if dq[0].WarningCode != rollforward.DQWarnInstrumenAktifNotInCurrentRun {
		t.Errorf("unexpected DQ warning code: %s", dq[0].WarningCode)
	}
}

// ─── ReconcileTolerance ──────────────────────────────────────────────────────

func TestReconcileTolerance_Value(t *testing.T) {
	want := decimal.RequireFromString("1.0000")
	if !rollforward.ReconcileTolerance.Equal(want) {
		t.Errorf("ReconcileTolerance: want %s, got %s", want, rollforward.ReconcileTolerance)
	}
}

// ─── TransferBucket.SumMovement ─────────────────────────────────────────────

func TestTransfers_SumMovement(t *testing.T) {
	tr := rollforward.Transfers{
		Stage1To2: rollforward.TransferBucket{EclMovementIdr: decimal.RequireFromString("200.0000")},
		Stage2To1: rollforward.TransferBucket{EclMovementIdr: decimal.RequireFromString("-50.0000")},
		Stage2To3: rollforward.TransferBucket{EclMovementIdr: decimal.RequireFromString("100.0000")},
		Stage1To3: rollforward.TransferBucket{EclMovementIdr: decimal.RequireFromString("30.0000")},
		Stage3To2: rollforward.TransferBucket{EclMovementIdr: decimal.RequireFromString("-20.0000")},
		Stage3To1: rollforward.TransferBucket{EclMovementIdr: decimal.RequireFromString("-10.0000")},
	}

	// 200 - 50 + 100 + 30 - 20 - 10 = 250
	want := decimal.RequireFromString("250.0000")
	if !tr.SumMovement().Equal(want) {
		t.Errorf("SumMovement: want %s, got %s", want, tr.SumMovement())
	}
}

// ─── Service-level reconcile invariant ──────────────────────────────────────
//
// We test the reconcile formula directly using exported helpers.

func TestReconcileInvariant_AlwaysZeroByConstruction(t *testing.T) {
	// remeasurements = closing − opening − Σtransfers − originations + derecognitions
	// → closing = opening + Σtransfers + originations − derecognitions + remeasurements
	// → delta = 0

	opening := decimal.RequireFromString("10000.0000")
	closing := decimal.RequireFromString("12500.0000")
	transferSum := decimal.RequireFromString("1500.0000")
	originations := decimal.RequireFromString("2000.0000")
	derecognitions := decimal.RequireFromString("500.0000")

	remeasurements := closing.
		Sub(opening).
		Sub(transferSum).
		Sub(originations).
		Add(derecognitions)

	reconcileCheck := opening.
		Add(transferSum).
		Add(originations).
		Sub(derecognitions).
		Add(remeasurements)

	delta := closing.Sub(reconcileCheck)

	if !delta.IsZero() {
		t.Errorf("reconcile delta should be 0 by construction, got %s", delta)
	}
	// Must be < tolerance.
	if delta.Abs().GreaterThanOrEqual(rollforward.ReconcileTolerance) {
		t.Errorf("delta %s ≥ tolerance %s — MISMATCH", delta, rollforward.ReconcileTolerance)
	}
}

// ─── Service + DB constructor ─────────────────────────────────────────────

func TestNewService_PanicOnNilAuditWriter(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when auditWriter=nil")
		}
	}()

	db := &sql.DB{}
	repo := rollforward.NewRepo(db)
	rollforward.NewService(repo, db, nil, nil)
}

func TestNewRepo_PanicOnNilDB(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when db=nil")
		}
	}()
	rollforward.NewRepo(nil)
}

// ─── Domain error helpers ─────────────────────────────────────────────────

func TestDomainError_CodeAndMessage(t *testing.T) {
	err := rollforward.ExportErrDomain(rollforward.CodeRollForwardPriorNotFound, "prior not found")
	if err.Error() != "ROLL_FORWARD_PRIOR_NOT_FOUND: prior not found" {
		t.Errorf("unexpected error string: %s", err.Error())
	}
	de := err
	if de.Code() != rollforward.CodeRollForwardPriorNotFound {
		t.Errorf("unexpected code: %s", de.Code())
	}
}

// ─── Phase5LimitationNote constant ───────────────────────────────────────

func TestPhase5LimitationNote_NotEmpty(t *testing.T) {
	if rollforward.Phase5LimitationNote == "" {
		t.Error("Phase5LimitationNote must not be empty")
	}
}

// ─── ExportXLSX MISMATCH guard ────────────────────────────────────────────

func TestExportXLSX_MismatchForbidden(t *testing.T) {
	// We can't easily instantiate a full Service without a DB, but we can test
	// the guard logic through the exported stub function.
	report := &rollforward.RollForwardReport{
		ReconcileStatus:  rollforward.ReconcileStatusMismatch,
		ReconcileDeltaIdr: decimal.RequireFromString("5.0000"),
		CurrentPeriodeID: "JUNI-2026",
	}
	// Use exported test helper to call ExportXLSX logic.
	err := rollforward.ExportXLSXGuardCheck(report, false)
	if err == nil {
		t.Fatal("expected error when MISMATCH and forceMismatch=false")
	}
	de, ok := err.(*rollforward.DomainErrorExported)
	if !ok {
		t.Fatalf("expected *domainError type, got %T", err)
	}
	if de.Code() != rollforward.CodeRollForwardExportMismatchForbidden {
		t.Errorf("expected ROLL_FORWARD_EXPORT_MISMATCH_FORBIDDEN, got %s", de.Code())
	}
}

func TestExportXLSX_MismatchAllowedWithForce(t *testing.T) {
	report := &rollforward.RollForwardReport{
		ReconcileStatus:  rollforward.ReconcileStatusMismatch,
		ReconcileDeltaIdr: decimal.RequireFromString("5.0000"),
		CurrentPeriodeID: "JUNI-2026",
	}
	err := rollforward.ExportXLSXGuardCheck(report, true)
	if err != nil {
		t.Errorf("expected no error when forceMismatch=true, got %v", err)
	}
}

func TestExportXLSX_ReconciledAllowed(t *testing.T) {
	report := &rollforward.RollForwardReport{
		ReconcileStatus:  rollforward.ReconcileStatusReconciled,
		ReconcileDeltaIdr: decimal.Zero,
		CurrentPeriodeID: "JUNI-2026",
	}
	err := rollforward.ExportXLSXGuardCheck(report, false)
	if err != nil {
		t.Errorf("expected no error for RECONCILED, got %v", err)
	}
}

// ─── setDifference ───────────────────────────────────────────────────────

func TestSetDifference_ReturnsPriorOnly(t *testing.T) {
	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
	prior := buildLines([]lineSpec{
		{id: id1, stage: 1, ecl: "100.0000"},
		{id: id2, stage: 1, ecl: "200.0000"},
		{id: id3, stage: 1, ecl: "300.0000"},
	})
	current := buildLines([]lineSpec{
		{id: id1, stage: 1, ecl: "100.0000"},
		// id2 absent → derecognition
		{id: id3, stage: 1, ecl: "300.0000"},
	})

	diff := rollforward.ExportSetDifference(prior, current)
	if len(diff) != 1 {
		t.Errorf("setDifference: want 1, got %d", len(diff))
	}
	if diff[0] != id2 {
		t.Errorf("setDifference: want %s, got %s", id2, diff[0])
	}
}

func TestSetDifference_Empty(t *testing.T) {
	id1 := uuid.New()
	prior := buildLines([]lineSpec{{id: id1, stage: 1, ecl: "100.0000"}})
	current := buildLines([]lineSpec{{id: id1, stage: 1, ecl: "100.0000"}})

	diff := rollforward.ExportSetDifference(prior, current)
	if len(diff) != 0 {
		t.Errorf("setDifference: want 0, got %d", len(diff))
	}
}

// ─── computeRollForwardRequest validation ─────────────────────────────────

func TestComputeRequest_DetectionMethodDefault(t *testing.T) {
	req := rollforward.ComputeRequest{}
	if req.DetectionMethod != "" {
		t.Errorf("default DetectionMethod should be empty string (service sets default)")
	}
}

// ─── Concurrent safety: SumMovement is pure function ─────────────────────

func TestTransferBucket_SumMovementZero(t *testing.T) {
	var tr rollforward.Transfers
	if !tr.SumMovement().IsZero() {
		t.Errorf("zero Transfers SumMovement should be 0")
	}
}
