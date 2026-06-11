package staging_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/ecl/staging"
)

// ─── DeltaNotch tests (DEC-011, SoW §4) ──────────────────────────────────────

func TestDeltaNotch_SameRating(t *testing.T) {
	// Same rating → delta = 0, no downgrade.
	delta := staging.DeltaNotch("idA", "idA")
	if delta != 0 {
		t.Errorf("expected 0, got %d", delta)
	}
}

func TestDeltaNotch_OneNotchDown(t *testing.T) {
	// idAAA → idAA+ is 1 notch down.
	delta := staging.DeltaNotch("idAAA", "idAA+")
	if delta != 1 {
		t.Errorf("expected 1, got %d", delta)
	}
}

func TestDeltaNotch_TwoNotchesDown(t *testing.T) {
	// idAAA → idAA is exactly 2 notches down (SICR boundary).
	delta := staging.DeltaNotch("idAAA", "idAA")
	if delta != 2 {
		t.Errorf("expected 2, got %d", delta)
	}
}

func TestDeltaNotch_UpgradeIsNegative(t *testing.T) {
	// Upgrade should return negative delta (improvement → no SICR).
	delta := staging.DeltaNotch("idAA", "idAAA")
	if delta >= 0 {
		t.Errorf("expected negative delta for upgrade, got %d", delta)
	}
}

func TestDeltaNotch_InvalidRating(t *testing.T) {
	// Unknown rating returns 0 per DeltaNotch spec (no error return).
	delta := staging.DeltaNotch("INVALID_RATING", "idAAA")
	if delta != 0 {
		t.Errorf("expected 0 for unknown rating, got %d", delta)
	}
}

func TestDeltaNotch_IGToNonIG(t *testing.T) {
	// idBBB- to idBB+ crosses IG boundary.
	delta := staging.DeltaNotch("idBBB-", "idBB+")
	if delta < 1 {
		t.Errorf("expected positive delta (downgrade), got %d", delta)
	}
}

func TestDeltaNotch_BottomOfScale(t *testing.T) {
	// idD to idD → 0.
	delta := staging.DeltaNotch("idD", "idD")
	if delta != 0 {
		t.Errorf("expected 0, got %d", delta)
	}
}

func TestDeltaNotch_AllDowngrades(t *testing.T) {
	// Exhaustive: verify each downgrade has the correct positive delta.
	scale := []string{
		"idAAA", "idAA+", "idAA", "idAA-",
		"idA+", "idA", "idA-",
		"idBBB+", "idBBB", "idBBB-",
		"idBB+", "idBB", "idBB-",
		"idB+", "idB", "idB-",
		"idCCC", "idD",
	}
	for i := 0; i < len(scale); i++ {
		for j := i + 1; j < len(scale); j++ {
			delta := staging.DeltaNotch(scale[i], scale[j])
			expected := j - i
			if delta != expected {
				t.Errorf("DeltaNotch(%s, %s): expected %d, got %d", scale[i], scale[j], expected, delta)
			}
		}
	}
}

// ─── IsInvestmentGrade / IsDefaultRating tests ───────────────────────────────

func TestIsInvestmentGrade(t *testing.T) {
	cases := []struct {
		rating string
		want   bool
	}{
		{"idAAA", true},
		{"idAA+", true},
		{"idBBB-", true}, // floor of IG
		{"idBB+", false}, // first non-IG
		{"idBB", false},
		{"idD", false},
		{"", false},
	}
	for _, tc := range cases {
		got := staging.IsInvestmentGrade(tc.rating)
		if got != tc.want {
			t.Errorf("IsInvestmentGrade(%q): want %v got %v", tc.rating, tc.want, got)
		}
	}
}

func TestIsDefaultRating(t *testing.T) {
	if !staging.IsDefaultRating("idD") {
		t.Error("idD should be default rating")
	}
	if staging.IsDefaultRating("idCCC") {
		t.Error("idCCC should not be default rating")
	}
}

// ─── EvaluateSICR tests (DEC-011) ────────────────────────────────────────────

// EvaluateSICR takes (ratingAtOrigination, ratingCurrent, ratingPrevious string, dpdValue int).
// ratingPrevious is used for IG→non-IG detection; pass "" if unavailable.

func TestEvaluateSICR_NoTrigger(t *testing.T) {
	// Same rating, DPD < 30 → no SICR.
	result := staging.EvaluateSICR("idA", "idA", "", 15)
	if result.Triggered {
		t.Errorf("expected no SICR, got triggered: %+v", result)
	}
}

func TestEvaluateSICR_RatingDowngrade2Notch(t *testing.T) {
	// idAAA → idAA is 2 notches → SICR triggered.
	result := staging.EvaluateSICR("idAAA", "idAA", "", 0)
	if !result.Triggered {
		t.Error("expected SICR triggered (2 notch downgrade)")
	}
	if result.TriggerType != staging.TriggerRatingDowngrade {
		t.Errorf("expected TriggerRatingDowngrade, got %s", result.TriggerType)
	}
}

func TestEvaluateSICR_RatingDowngrade1NotchOnly(t *testing.T) {
	// 1 notch downgrade → no SICR (boundary).
	result := staging.EvaluateSICR("idAAA", "idAA+", "", 0)
	if result.Triggered {
		t.Errorf("expected no SICR for 1-notch downgrade, got triggered: %+v", result)
	}
}

func TestEvaluateSICR_IGToNonIG(t *testing.T) {
	// origination=idBBB-, current=idBB+ (1 notch down, crosses IG boundary),
	// ratingPrevious=idBBB- (was IG, now non-IG).
	// Delta=1 < 2 so the 2-notch rule doesn't fire; IG→non-IG fires instead.
	result := staging.EvaluateSICR("idBBB-", "idBB+", "idBBB-", 0)
	if !result.Triggered {
		t.Error("expected SICR triggered (IG → non-IG via ratingPrevious)")
	}
	if result.TriggerType != staging.TriggerIGToNonIG {
		t.Errorf("expected TriggerIGToNonIG, got %s", result.TriggerType)
	}
}

func TestEvaluateSICR_DPD30(t *testing.T) {
	// DPD = 30 → SICR triggered (DEC-011: DPD ≥ 30).
	result := staging.EvaluateSICR("idA", "idA", "", 30)
	if !result.Triggered {
		t.Error("expected SICR triggered (DPD ≥ 30)")
	}
	if result.TriggerType != staging.TriggerDPDGte30 {
		t.Errorf("expected TriggerDPDGte30, got %s", result.TriggerType)
	}
}

func TestEvaluateSICR_DPD29(t *testing.T) {
	// DPD = 29 → no SICR (boundary just below).
	result := staging.EvaluateSICR("idA", "idA", "", 29)
	if result.Triggered {
		t.Errorf("expected no SICR for DPD=29, got triggered: %+v", result)
	}
}

func TestEvaluateSICR_DPD90(t *testing.T) {
	// DPD = 90 → SICR triggered (DPD ≥ 30 fires first).
	result := staging.EvaluateSICR("idA", "idA", "", 90)
	if !result.Triggered {
		t.Error("expected SICR triggered (DPD ≥ 30)")
	}
}

func TestEvaluateSICR_NoOriginRating(t *testing.T) {
	// Empty origin rating → notch delta = 0 → only DPD matters.
	result := staging.EvaluateSICR("", "idA", "", 0)
	if result.Triggered {
		t.Errorf("expected no SICR when origin rating empty and DPD=0, got: %+v", result)
	}
}

func TestEvaluateSICR_EmptyCurrentRating_NoDPD(t *testing.T) {
	// Both ratings empty, DPD = 0 → no SICR.
	result := staging.EvaluateSICR("", "", "", 0)
	if result.Triggered {
		t.Errorf("expected no SICR when both ratings empty and DPD=0, got: %+v", result)
	}
}

func TestEvaluateSICR_DefaultRating_AlwaysSICR(t *testing.T) {
	// idD → TriggerRatingDefault (highest severity).
	result := staging.EvaluateSICR("idA", "idD", "", 0)
	if !result.Triggered {
		t.Error("expected SICR when current rating = idD")
	}
	if result.TriggerType != staging.TriggerRatingDefault {
		t.Errorf("expected TriggerRatingDefault, got %s", result.TriggerType)
	}
}

// ─── ComputeNewStage tests ────────────────────────────────────────────────────

// ComputeNewStage returns (newStage Stage, needsDoubleRow bool).

func TestComputeNewStage_Stage1_NoSICR(t *testing.T) {
	newStage, doubleRow := staging.ComputeNewStage(staging.Stage1, staging.SICRResult{Triggered: false}, 0)
	if newStage != staging.Stage1 {
		t.Errorf("expected Stage1 unchanged, got %s", newStage)
	}
	if doubleRow {
		t.Error("expected no double row when no SICR")
	}
}

func TestComputeNewStage_Stage1_SICR_ToStage2(t *testing.T) {
	newStage, doubleRow := staging.ComputeNewStage(staging.Stage1, staging.SICRResult{
		Triggered:   true,
		TriggerType: staging.TriggerRatingDowngrade,
	}, 25)
	if newStage != staging.Stage2 {
		t.Errorf("expected Stage2, got %s", newStage)
	}
	if doubleRow {
		t.Error("expected no double row for non-90 DPD SICR")
	}
}

func TestComputeNewStage_Stage1_DPD90_DoubleRow(t *testing.T) {
	// DPD ≥ 90 from Stage 1 → Stage3 with needsDoubleRow=true.
	newStage, doubleRow := staging.ComputeNewStage(staging.Stage1, staging.SICRResult{
		Triggered:   true,
		TriggerType: staging.TriggerDPDGte30,
	}, 90)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3, got %s", newStage)
	}
	if !doubleRow {
		t.Error("expected doubleRow=true for DPD=90 from Stage 1")
	}
}

func TestComputeNewStage_Stage2_NoSICR_NoChange(t *testing.T) {
	// Stage 2 without SICR → returns Stage2 (no escalation; cure is separate).
	newStage, doubleRow := staging.ComputeNewStage(staging.Stage2, staging.SICRResult{Triggered: false}, 10)
	if newStage != staging.Stage2 {
		t.Errorf("expected Stage2 unchanged, got %s", newStage)
	}
	if doubleRow {
		t.Error("expected no double row")
	}
}

func TestComputeNewStage_Stage2_DPD90_ToStage3(t *testing.T) {
	// DPD ≥ 90 from Stage 2 → Stage3 (no double row since already Stage2).
	newStage, doubleRow := staging.ComputeNewStage(staging.Stage2, staging.SICRResult{
		Triggered:   true,
		TriggerType: staging.TriggerDPDGte30,
	}, 90)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3, got %s", newStage)
	}
	if doubleRow {
		t.Error("expected no double row from Stage 2 (already at Stage 2)")
	}
}

func TestComputeNewStage_Stage3_NoChange(t *testing.T) {
	// Stage 3 DPD≥90 → stays Stage3 (already there).
	newStage, doubleRow := staging.ComputeNewStage(staging.Stage3, staging.SICRResult{Triggered: false}, 100)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3 unchanged, got %s", newStage)
	}
	if doubleRow {
		t.Error("expected no double row from Stage 3")
	}
}

// ─── ComputeSignatureHash ─────────────────────────────────────────────────────

func TestComputeSignatureHash_Deterministic(t *testing.T) {
	actorID := uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	propID := uuid.MustParse("b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a22")
	signedAt := mustParseTime("2026-06-01T10:00:00Z")
	comment := strPtr("test comment")

	hash1 := staging.ComputeSignatureHash(actorID, "REVIEW", propID, signedAt, *comment)
	hash2 := staging.ComputeSignatureHash(actorID, "REVIEW", propID, signedAt, *comment)
	if !bytes.Equal(hash1, hash2) {
		t.Error("ComputeSignatureHash should be deterministic")
	}
}

func TestComputeSignatureHash_DifferentInputs(t *testing.T) {
	actorID := uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	propID := uuid.MustParse("b1eebc99-9c0b-4ef8-bb6d-6bb9bd380a22")
	t1 := mustParseTime("2026-06-01T10:00:00Z")

	h1 := staging.ComputeSignatureHash(actorID, "REVIEW", propID, t1, "")
	h2 := staging.ComputeSignatureHash(actorID, "APPROVE_ALCO", propID, t1, "")
	if bytes.Equal(h1, h2) {
		t.Error("different steps should produce different hashes")
	}
}

func TestComputeSignatureHash_NotEmpty(t *testing.T) {
	actorID := uuid.New()
	propID := uuid.New()
	hash := staging.ComputeSignatureHash(actorID, "APPROVE_KOMITE", propID, mustParseTime("2026-06-01T10:00:00Z"), "")
	if len(hash) == 0 {
		t.Error("hash should not be empty")
	}
}

// ─── OverrideProposal.Is6Eyes ─────────────────────────────────────────────────

func TestIs6Eyes_Stage3ToStage2(t *testing.T) {
	prop := &staging.OverrideProposal{
		StageFrom: staging.Stage3,
		StageTo:   staging.Stage2,
	}
	if !prop.Is6Eyes() {
		t.Error("Stage3→Stage2 should require 6-eyes")
	}
}

func TestIs6Eyes_Stage2ToStage1(t *testing.T) {
	prop := &staging.OverrideProposal{
		StageFrom: staging.Stage2,
		StageTo:   staging.Stage1,
	}
	if prop.Is6Eyes() {
		t.Error("Stage2→Stage1 should require 4-eyes (not 6)")
	}
}

// ─── TriggerType.IsValid ─────────────────────────────────────────────────────

func TestTriggerType_IsValid_Known(t *testing.T) {
	triggers := []staging.TriggerType{
		staging.TriggerRatingDowngrade,
		staging.TriggerIGToNonIG,
		staging.TriggerRatingDefault,
		staging.TriggerDPDGte30,
		staging.TriggerDPDGte90,
		staging.TriggerCure3PeriodeBulanan,
		staging.TriggerManualOverride,
		staging.TriggerOverrideExpired,
		staging.TriggerInitial,
	}
	for _, tr := range triggers {
		if !tr.IsValid() {
			t.Errorf("expected %s to be valid", tr)
		}
	}
}

func TestTriggerType_IsValid_Unknown(t *testing.T) {
	tr := staging.TriggerType("UNKNOWN_TRIGGER")
	if tr.IsValid() {
		t.Error("expected unknown trigger to be invalid")
	}
}

// ─── OverrideWorkflowStatus.IsTerminal ────────────────────────────────────────

func TestIsTerminal_Active(t *testing.T) {
	if !staging.OverrideStatusActive.IsTerminal() {
		t.Error("ACTIVE should be terminal")
	}
}

func TestIsTerminal_Expired(t *testing.T) {
	if !staging.OverrideStatusExpired.IsTerminal() {
		t.Error("EXPIRED should be terminal")
	}
}

func TestIsTerminal_Rejected(t *testing.T) {
	if !staging.OverrideStatusRejected.IsTerminal() {
		t.Error("REJECTED should be terminal")
	}
}

func TestIsTerminal_PendingReview_NotTerminal(t *testing.T) {
	if staging.OverrideStatusPendingReview.IsTerminal() {
		t.Error("PENDING_REVIEW should not be terminal")
	}
}

func TestIsTerminal_PendingApproval_NotTerminal(t *testing.T) {
	if staging.OverrideStatusPendingApproval.IsTerminal() {
		t.Error("PENDING_APPROVAL should not be terminal")
	}
}

func TestIsTerminal_ApprovedALCO_NotTerminal(t *testing.T) {
	if staging.OverrideStatusApprovedALCO.IsTerminal() {
		t.Error("APPROVED_ALCO should not be terminal")
	}
}

// ─── Error constructors ───────────────────────────────────────────────────────

func TestErrStagingOverrideExpired_HasCode(t *testing.T) {
	err := staging.ErrStagingOverrideExpired()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if string(err.Code()) != staging.CodeStagingOverrideExpired {
		t.Errorf("expected code %s, got %s", staging.CodeStagingOverrideExpired, err.Code())
	}
}

func TestErrStagingRatingBaselineMissing_HasCode(t *testing.T) {
	err := staging.ErrStagingRatingBaselineMissing(uuid.New().String(), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if string(err.Code()) != staging.CodeStagingRatingBaselineMissing {
		t.Errorf("expected code %s, got %s", staging.CodeStagingRatingBaselineMissing, err.Code())
	}
}

func TestErrStagingCalcRunSealed_HasCode(t *testing.T) {
	err := staging.ErrStagingCalcRunSealed()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if string(err.Code()) != staging.CodeStagingCalcRunSealed {
		t.Errorf("expected code %s, got %s", staging.CodeStagingCalcRunSealed, err.Code())
	}
}

// ─── ComputeNewStage ─────────────────────────────────────────────────────────

// TestComputeNewStage_DefaultRating_FromStage1_NeedsDoubleRow
// Rating = idD → Stage 3 with double row (Stage1 → Stage2 → Stage3).
func TestComputeNewStage_DefaultRating_FromStage1_NeedsDoubleRow(t *testing.T) {
	sicrResult := staging.SICRResult{IsDefault: true}
	newStage, needsDouble := staging.ComputeNewStage(staging.Stage1, sicrResult, 0)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3, got %v", newStage)
	}
	if !needsDouble {
		t.Error("expected needsDoubleRow=true when transitioning from Stage1 to Stage3")
	}
}

// TestComputeNewStage_DefaultRating_FromStage2_NoDoubleRow.
func TestComputeNewStage_DefaultRating_FromStage2_NoDoubleRow(t *testing.T) {
	sicrResult := staging.SICRResult{IsDefault: true}
	newStage, needsDouble := staging.ComputeNewStage(staging.Stage2, sicrResult, 0)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3, got %v", newStage)
	}
	if needsDouble {
		t.Error("expected needsDoubleRow=false when transitioning from Stage2 to Stage3")
	}
}

// TestComputeNewStage_DPDGte90_FromStage1_NeedsDoubleRow.
func TestComputeNewStage_DPDGte90_FromStage1_NeedsDoubleRow(t *testing.T) {
	sicrResult := staging.SICRResult{IsDefault: false}
	newStage, needsDouble := staging.ComputeNewStage(staging.Stage1, sicrResult, 90)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3 for DPD≥90, got %v", newStage)
	}
	if !needsDouble {
		t.Error("expected needsDoubleRow=true for DPD≥90 from Stage1")
	}
}

// TestComputeNewStage_SICRTriggered_AlreadyStage2_NoTransition.
func TestComputeNewStage_SICRTriggered_AlreadyStage2_NoTransition(t *testing.T) {
	sicrResult := staging.SICRResult{Triggered: true, IsDefault: false}
	newStage, needsDouble := staging.ComputeNewStage(staging.Stage2, sicrResult, 15)
	if newStage != staging.Stage2 {
		t.Errorf("expected Stage2 (no change), got %v", newStage)
	}
	if needsDouble {
		t.Error("expected needsDoubleRow=false when already Stage2")
	}
}

// TestComputeNewStage_NoTrigger_Stage3_StaysStage3.
func TestComputeNewStage_NoTrigger_Stage3_StaysStage3(t *testing.T) {
	sicrResult := staging.SICRResult{Triggered: false, IsDefault: false}
	newStage, needsDouble := staging.ComputeNewStage(staging.Stage3, sicrResult, 0)
	if newStage != staging.Stage3 {
		t.Errorf("expected Stage3 (no change), got %v", newStage)
	}
	if needsDouble {
		t.Error("expected needsDoubleRow=false for no-trigger")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func strPtr(s string) *string { return &s }
