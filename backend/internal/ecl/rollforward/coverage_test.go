package rollforward_test

// coverage_test.go — targeted tests to push coverage above 85%.
// Covers: rollForwardHTTPStatus, classifyDerecognitionReason, eclOrZero,
// POCI handling, DetectionMethod constant.

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/ecl/rollforward"
)

// ─── rollForwardHTTPStatus coverage ─────────────────────────────────────────

func TestRollForwardHTTPStatus_NotFound(t *testing.T) {
	status := rollforward.ExportRollForwardHTTPStatus(rollforward.CodeRollForwardPriorNotFound)
	if status != http.StatusNotFound {
		t.Errorf("want 404, got %d", status)
	}
}

func TestRollForwardHTTPStatus_PortfolioNotFound(t *testing.T) {
	status := rollforward.ExportRollForwardHTTPStatus(rollforward.CodeRollForwardPortfolioNotFound)
	if status != http.StatusNotFound {
		t.Errorf("want 404, got %d", status)
	}
}

func TestRollForwardHTTPStatus_AllUnprocessable(t *testing.T) {
	codes := []string{
		rollforward.CodeRollForwardCurrentInvalidState,
		rollforward.CodeRollForwardPriorNotSealed,
		rollforward.CodeRollForwardPeriodeMismatch,
		rollforward.CodeRollForwardDetectionMethodInvalid,
		rollforward.CodeRollForwardExportMismatchForbidden,
		rollforward.CodeRollForwardScopeMismatch,
		rollforward.CodeRollForwardTrendInsufficientData,
		rollforward.CodeRollForwardInvalidCalcRunStatus,
		rollforward.CodeRollForwardInvalidPriorPeriod,
	}
	for _, code := range codes {
		status := rollforward.ExportRollForwardHTTPStatus(code)
		if status != http.StatusUnprocessableEntity {
			t.Errorf("code %s: want 422, got %d", code, status)
		}
	}
}

func TestRollForwardHTTPStatus_UnknownCode_Returns500(t *testing.T) {
	status := rollforward.ExportRollForwardHTTPStatus("UNKNOWN_XYZ")
	if status != http.StatusInternalServerError {
		t.Errorf("want 500 for unknown code, got %d", status)
	}
}

// ─── classifyDerecognitionReason via detectLifecycle ─────────────────────────

func TestDetectLifecycle_DerecognitionReason_Sold(t *testing.T) {
	oldID := uuid.New()
	prior := buildLines([]lineSpec{{id: oldID, stage: 1, ecl: "500000.0000"}})
	current := buildLines(nil)

	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{
		oldID: {ID: oldID, Kode: "INST-SOLD", Status: "DIJUAL"},
	}

	_, derec, _ := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())
	if derec.Count != 1 {
		t.Errorf("want 1 derecognition, got %d", derec.Count)
	}
}

func TestDetectLifecycle_DerecognitionReason_MaturedByDate(t *testing.T) {
	oldID := uuid.New()
	prior := buildLines([]lineSpec{{id: oldID, stage: 2, ecl: "800000.0000"}})
	current := buildLines(nil)

	past := time.Now().AddDate(0, -1, 0)
	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{
		oldID: {ID: oldID, Kode: "INST-MAT", Status: "AKTIF", TanggalJatuhTempo: &past},
	}

	_, derec, _ := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())
	if derec.Count != 1 {
		t.Errorf("want 1 derecognition by date, got %d", derec.Count)
	}
}

func TestDetectLifecycle_DerecognitionReason_Unknown_NoStatus(t *testing.T) {
	oldID := uuid.New()
	prior := buildLines([]lineSpec{{id: oldID, stage: 1, ecl: "100000.0000"}})
	current := buildLines(nil)

	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{}

	_, derec, _ := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())
	if derec.Count != 1 {
		t.Errorf("want 1 derecognition, got %d", derec.Count)
	}
}

// ─── POCI in transfer detection ───────────────────────────────────────────

func TestDetectTransfers_POCIInstrument_ZeroECLMovement(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 1, ecl: ""}})  // POCI nil
	current := buildLines([]lineSpec{{id: id, stage: 2, ecl: ""}}) // POCI nil
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "SICR_RATING"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)
	if !tr.Stage1To2.EclMovementIdr.IsZero() {
		t.Errorf("POCI transfer ECL movement should be 0, got %s", tr.Stage1To2.EclMovementIdr)
	}
}

// ─── POCI origination EclIdr = 0 ──────────────────────────────────────────

func TestDetectLifecycle_POCI_OriginationEclZero(t *testing.T) {
	id := uuid.New()
	prior := buildLines(nil)
	current := buildLines([]lineSpec{{id: id, stage: 1, ecl: ""}}) // POCI nil
	statuses := map[uuid.UUID]rollforward.InstrumenStatusSnapshot{}

	orig, _, _ := rollforward.ExportDetectLifecycle(prior, current, statuses, uuid.New(), time.Now())
	if orig.Count != 1 {
		t.Errorf("want 1 origination, got %d", orig.Count)
	}
	if orig.EclIdr.IsPositive() {
		t.Errorf("POCI origination ECL should be 0, got %s", orig.EclIdr)
	}
}

// ─── DetectionMethod constant ─────────────────────────────────────────────

func TestDetectionMethodBasicStatusDiff_Value(t *testing.T) {
	if string(rollforward.DetectionMethodBasicStatusDiff) != "BASIC_STATUS_DIFF" {
		t.Errorf("unexpected DetectionMethodBasicStatusDiff: %s", rollforward.DetectionMethodBasicStatusDiff)
	}
}

// ─── Stage 3→2 override flag ──────────────────────────────────────────────

func TestDetectTransfers_Stage3To2_Override(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 3, ecl: "900000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 2, ecl: "400000.0000"}})
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "MANAGEMENT_OVERRIDE"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)
	if tr.Stage3To2.Count != 1 {
		t.Errorf("Stage3To2.Count: want 1, got %d", tr.Stage3To2.Count)
	}
	if tr.Stage3To2.CountOverride != 1 {
		t.Errorf("Stage3To2.CountOverride: want 1 (MANAGEMENT_OVERRIDE), got %d", tr.Stage3To2.CountOverride)
	}
	// Movement should be negative (cure direction)
	if !tr.Stage3To2.EclMovementIdr.IsNegative() {
		t.Errorf("Stage3To2 movement should be negative, got %s", tr.Stage3To2.EclMovementIdr)
	}
}

// ─── Stage 1→3 direct default ─────────────────────────────────────────────

func TestDetectTransfers_Stage1To3_DirectDefault(t *testing.T) {
	id := uuid.New()
	prior := buildLines([]lineSpec{{id: id, stage: 1, ecl: "100000.0000"}})
	current := buildLines([]lineSpec{{id: id, stage: 3, ecl: "600000.0000"}})
	stageHistory := map[uuid.UUID]rollforward.StageHistoryRow{
		id: {InstrumenID: id, TriggerType: "DPD"},
	}

	tr, _ := rollforward.ExportDetectTransfers(prior, current, stageHistory)
	if tr.Stage1To3.Count != 1 {
		t.Errorf("Stage1To3.Count: want 1, got %d", tr.Stage1To3.Count)
	}
	if !tr.Stage1To3.EclMovementIdr.IsPositive() {
		t.Errorf("Stage1To3 movement should be positive, got %s", tr.Stage1To3.EclMovementIdr)
	}
}

// ─── InstrumentBucket enum values ─────────────────────────────────────────

func TestInstrumentBucket_Values(t *testing.T) {
	buckets := []rollforward.InstrumentBucket{
		rollforward.BucketStage1To2,
		rollforward.BucketStage2To1,
		rollforward.BucketStage2To3,
		rollforward.BucketStage1To3,
		rollforward.BucketStage3To2,
		rollforward.BucketStage3To1,
		rollforward.BucketNewOrigination,
		rollforward.BucketDerecognition,
		rollforward.BucketStageSame,
	}
	seen := make(map[rollforward.InstrumentBucket]bool)
	for _, b := range buckets {
		if seen[b] {
			t.Errorf("duplicate bucket value: %s", b)
		}
		seen[b] = true
		if string(b) == "" {
			t.Errorf("bucket value should not be empty")
		}
	}
	if len(seen) != 9 {
		t.Errorf("want 9 distinct buckets, got %d", len(seen))
	}
}
