package ratinghistory_test

import (
	"testing"

	"blips-ifrs9.tugu-re.com/internal/master/ratinghistory"
)

// ─── ComputeSICR unit tests ───────────────────────────────────────────────────

func TestComputeSICR_TwoNotchDowngrade_TriggersICR(t *testing.T) {
	sicr, def := ratinghistory.ComputeSICR(-2, "idAA", "idA")
	if !sicr {
		t.Error("expected SICR=true for 2-notch downgrade")
	}
	if def {
		t.Error("expected default=false for idA")
	}
}

func TestComputeSICR_ThreeNotchDowngrade_TriggersICR(t *testing.T) {
	sicr, def := ratinghistory.ComputeSICR(-3, "idAAA", "idBBB")
	if !sicr {
		t.Error("expected SICR=true for 3-notch downgrade")
	}
	if def {
		t.Error("expected default=false")
	}
}

func TestComputeSICR_OneNotchDowngrade_NoICR(t *testing.T) {
	sicr, def := ratinghistory.ComputeSICR(-1, "idAA", "idAA-")
	if sicr {
		t.Error("expected SICR=false for 1-notch downgrade")
	}
	if def {
		t.Error("expected default=false")
	}
}

func TestComputeSICR_IGtoNonIG_TriggersICR(t *testing.T) {
	// idBBB- (IG) → idBB+ (non-IG): triggers SICR even if notch_change = -1
	sicr, def := ratinghistory.ComputeSICR(-1, "idBBB-", "idBB+")
	if !sicr {
		t.Error("expected SICR=true for IG→non-IG transition")
	}
	if def {
		t.Error("expected default=false")
	}
}

func TestComputeSICR_BothRulesTriggered(t *testing.T) {
	// 2-notch downgrade AND IG→non-IG
	sicr, def := ratinghistory.ComputeSICR(-2, "idBBB-", "idB")
	if !sicr {
		t.Error("expected SICR=true")
	}
	if def {
		t.Error("expected default=false for idB")
	}
}

func TestComputeSICR_DefaultRating_TriggersDefault(t *testing.T) {
	sicr, def := ratinghistory.ComputeSICR(-5, "idCCC", "idD")
	if !sicr {
		t.Error("expected SICR=true (5-notch downgrade)")
	}
	if !def {
		t.Error("expected default=true for idD")
	}
}

func TestComputeSICR_DefaultRatingLowercase_TriggersDefault(t *testing.T) {
	sicr, def := ratinghistory.ComputeSICR(-2, "idCC", "idD")
	if !def {
		t.Error("expected default=true for 'idD' (lowercase rating match)")
	}
	_ = sicr
}

func TestComputeSICR_AffirmedIG_NoTrigger(t *testing.T) {
	// AFFIRMED within IG, notch_change = 0
	sicr, def := ratinghistory.ComputeSICR(0, "idA", "idA")
	if sicr {
		t.Error("expected SICR=false for affirmed IG")
	}
	if def {
		t.Error("expected default=false for idA")
	}
}

func TestComputeSICR_Upgrade_NoTrigger(t *testing.T) {
	// Upgrade should not trigger SICR
	sicr, def := ratinghistory.ComputeSICR(2, "idBBB", "idA")
	if sicr {
		t.Error("expected SICR=false for upgrade")
	}
	if def {
		t.Error("expected default=false")
	}
}

func TestComputeSICR_InitialRating_NoPreviousRating(t *testing.T) {
	// No previous rating — IG→non-IG rule cannot fire (previousRating="")
	// But 2-notch rule can still fire if notchChange <= -2
	sicr, def := ratinghistory.ComputeSICR(-2, "", "idBB+")
	if !sicr {
		t.Error("expected SICR=true for notch_change=-2 on initial")
	}
	_ = def
}

func TestComputeSICR_InitialRatingNoNotch_NoICR(t *testing.T) {
	// Initial: no previous, notchChange=0
	sicr, _ := ratinghistory.ComputeSICR(0, "", "idA")
	if sicr {
		t.Error("expected SICR=false for initial with notch=0 and no previous")
	}
}

// ─── IsInvestmentGrade unit tests ─────────────────────────────────────────────

func TestIsInvestmentGrade_IGRatings(t *testing.T) {
	igRatings := []string{
		"idAAA", "idAA+", "idAA", "idAA-",
		"idA+", "idA", "idA-",
		"idBBB+", "idBBB", "idBBB-",
	}
	for _, r := range igRatings {
		if !ratinghistory.IsInvestmentGrade(r) {
			t.Errorf("expected %s to be Investment Grade", r)
		}
	}
}

func TestIsInvestmentGrade_NonIGRatings(t *testing.T) {
	nonIG := []string{
		"idBB+", "idBB", "idBB-",
		"idB+", "idB", "idB-",
		"idCCC+", "idCCC", "idCCC-",
		"idCC", "idC", "idSD", "idD",
		"WITHDRAWN",
	}
	for _, r := range nonIG {
		if ratinghistory.IsInvestmentGrade(r) {
			t.Errorf("expected %s to be non-Investment Grade", r)
		}
	}
}

func TestIsInvestmentGrade_CaseInsensitive(t *testing.T) {
	if !ratinghistory.IsInvestmentGrade("IDAAA") {
		t.Error("expected idAAA (uppercase) to be IG")
	}
	if !ratinghistory.IsInvestmentGrade("IdBBB-") {
		t.Error("expected idBBB- (mixed case) to be IG")
	}
}

func TestIsInvestmentGrade_EmptyString_NotIG(t *testing.T) {
	if ratinghistory.IsInvestmentGrade("") {
		t.Error("expected empty string to be non-IG")
	}
}

// ─── IsValidActionType unit tests ─────────────────────────────────────────────

func TestIsValidActionType_Valid(t *testing.T) {
	vals := []string{"INITIAL", "UPGRADE", "DOWNGRADE", "AFFIRMED", "WITHDRAWN", "CORRECTION"}
	for _, v := range vals {
		if !ratinghistory.IsValidActionType(v) {
			t.Errorf("expected %s to be valid action_type", v)
		}
	}
}

func TestIsValidActionType_Invalid(t *testing.T) {
	if ratinghistory.IsValidActionType("RESET") {
		t.Error("expected RESET to be invalid action_type")
	}
}

// ─── ToResponse unit tests ────────────────────────────────────────────────────

func TestToResponse_IGFieldComputedCorrectly(t *testing.T) {
	rh := testRatingHistory()
	rh.RatingPefindo = "idBBB" // IG
	resp := ratinghistory.ToResponse(rh)
	if !resp.IsInvestmentGrade {
		t.Error("expected isInvestmentGrade=true for idBBB")
	}
}

func TestToResponse_NonIG_FieldComputedCorrectly(t *testing.T) {
	rh := testRatingHistory()
	rh.RatingPefindo = "idBB+" // non-IG
	resp := ratinghistory.ToResponse(rh)
	if resp.IsInvestmentGrade {
		t.Error("expected isInvestmentGrade=false for idBB+")
	}
}

func TestToResponse_WorkflowStatusRejected_DisplaysAsReturned(t *testing.T) {
	rh := testRatingHistory()
	rh.WorkflowStatus = ratinghistory.WorkflowStatusRejected
	resp := ratinghistory.ToResponse(rh)
	if resp.WorkflowStatus != "RETURNED" {
		t.Errorf("expected RETURNED, got %s", resp.WorkflowStatus)
	}
}
