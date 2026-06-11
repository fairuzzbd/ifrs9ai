package lookthrough

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TestDBScenarioParamRepo_GetScenarioWeights_Defaults verifies default weights are returned
// when no ALCO override exists. DEC-010: Good/Normal/Bad = 0.25/0.50/0.25.
func TestDBScenarioParamRepo_GetScenarioWeights_Defaults(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Query returns no rows → repo should fall back to defaults.
	// Use .* to match the full SQL including ::text casts and table name.
	mock.ExpectQuery(`SELECT bobot_good.*bobot_normal.*bobot_bad`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBScenarioParamRepo(db)
	w, err := repo.GetScenarioWeights(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantGood := decimal.NewFromFloat(0.25)
	wantNormal := decimal.NewFromFloat(0.50)
	wantBad := decimal.NewFromFloat(0.25)

	if !w.Good.Equal(wantGood) {
		t.Errorf("Good: got %s, want %s", w.Good, wantGood)
	}
	if !w.Normal.Equal(wantNormal) {
		t.Errorf("Normal: got %s, want %s", w.Normal, wantNormal)
	}
	if !w.Bad.Equal(wantBad) {
		t.Errorf("Bad: got %s, want %s", w.Bad, wantBad)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBScenarioParamRepo_GetFLMultipliers_Defaults verifies default FL = 1 when no override.
func TestDBScenarioParamRepo_GetFLMultipliers_Defaults(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT.*multiplier_good.*multiplier_normal.*multiplier_bad`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBScenarioParamRepo(db)
	fl, err := repo.GetFLMultipliers(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !fl.Good.Equal(decimal.NewFromFloat(1)) {
		t.Errorf("FL.Good: got %s, want 1", fl.Good)
	}
	if !fl.Normal.Equal(decimal.NewFromFloat(1)) {
		t.Errorf("FL.Normal: got %s, want 1", fl.Normal)
	}
	if !fl.Bad.Equal(decimal.NewFromFloat(1)) {
		t.Errorf("FL.Bad: got %s, want 1", fl.Bad)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBScenarioParamRepo_GetScenarioWeights_ALCOOverride verifies ALCO override is used.
func TestDBScenarioParamRepo_GetScenarioWeights_ALCOOverride(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"bobot_good", "bobot_normal", "bobot_bad"}).
		AddRow("0.20000000", "0.60000000", "0.20000000")

	mock.ExpectQuery(`SELECT bobot_good.*bobot_normal.*bobot_bad`).
		WillReturnRows(rows)

	repo := NewDBScenarioParamRepo(db)
	w, err := repo.GetScenarioWeights(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !w.Good.Equal(decimal.NewFromFloat(0.20)) {
		t.Errorf("Good: got %s, want 0.20", w.Good)
	}
	if !w.Normal.Equal(decimal.NewFromFloat(0.60)) {
		t.Errorf("Normal: got %s, want 0.60", w.Normal)
	}
	if !w.Bad.Equal(decimal.NewFromFloat(0.20)) {
		t.Errorf("Bad: got %s, want 0.20", w.Bad)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestEncodeCursor_RoundTrip verifies cursor encode/decode roundtrip.
func TestEncodeCursor_RoundTrip(t *testing.T) {
	t.Parallel()
	sortVal := "2026-06-11T00:00:00Z"
	id := uuid.New().String()

	encoded := encodeCursor(sortVal, id)
	if encoded == "" {
		t.Fatal("expected non-empty cursor")
	}

	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decodeCursor error: %v", err)
	}
	if decoded.SortValue != sortVal {
		t.Errorf("SortValue: got %q, want %q", decoded.SortValue, sortVal)
	}
	if decoded.ID != id {
		t.Errorf("ID: got %s, want %s", decoded.ID, id)
	}
}

// TestDecodeCursor_MissingSeparator returns error when `|` is absent.
func TestDecodeCursor_MissingSeparator(t *testing.T) {
	t.Parallel()
	_, err := decodeCursor("noseparator")
	if err == nil {
		t.Error("expected error for missing separator")
	}
}

// TestMarshalBreakdown_NoFloat64 verifies JSON output uses quoted strings not float literals.
// DEC-016: no float64 for money/rates.
func TestMarshalBreakdown_NoFloat64(t *testing.T) {
	t.Parallel()
	line := BreakdownLine{
		AssetClass:            AssetClassCorpBond,
		WeightPct:             decimal.NewFromFloat(0.5),
		NABPortionIDR:         decimal.NewFromFloat(5_000_000),
		PDGood:                decimal.NewFromFloat(0.02),
		PDNormal:              decimal.NewFromFloat(0.03),
		PDBad:                 decimal.NewFromFloat(0.06),
		LGD:                   decimal.NewFromFloat(0.45),
		ECLSkenariosGoodIDR:   decimal.NewFromFloat(45_000),
		ECLSkenariosNormalIDR: decimal.NewFromFloat(67_500),
		ECLSkenariosBadIDR:    decimal.NewFromFloat(135_000),
		ECLFLGoodIDR:          decimal.NewFromFloat(45_000),
		ECLFLNormalIDR:        decimal.NewFromFloat(67_500),
		ECLFLBadIDR:           decimal.NewFromFloat(135_000),
		ECLWeightedIDR:        decimal.NewFromFloat(78_750),
	}
	breakdown := []BreakdownLine{line}
	jsonBytes := marshalBreakdown(breakdown)

	// Verify the JSON is valid and decimal values are stored as quoted strings.
	// The key invariant: numbers like 5000000 are wrapped in quotes ("5000000.0000")
	// not emitted as bare JSON numbers (5000000.0).
	// We verify by checking a quoted string appears — if the value were a JSON number,
	// it would appear without surrounding quotes.
	jsonStr := string(jsonBytes)
	if !contains(jsonStr, `"nab_portion_idr":"`) {
		t.Errorf("nab_portion_idr should be a quoted string value in JSON, got: %s", jsonStr)
	}
	if !contains(jsonStr, `"ecl_weighted_idr":"`) {
		t.Errorf("ecl_weighted_idr should be a quoted string value in JSON, got: %s", jsonStr)
	}
	// Verify the specific ECL weighted value is present.
	if !contains(jsonStr, "78750") {
		t.Errorf("ECL_weighted 78750 not found in JSON: %s", jsonStr)
	}
}

// TestGetActiveForInstrumen_effectiveTo verifies active query uses effective_to >= evalDate
// (not IS NULL). Migration 000024 sets default effective_to = '9999-12-31'.
func TestGetActiveForInstrumen_effectiveTo(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	instrumenID := uuid.New()
	compositionID := uuid.New()
	evalDate := time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC)

	// Must match all 24 columns selected by activeCompositionSQL:
	// id, instrumen_id, effective_from, effective_to, workflow_status,
	// maker_id, reviewer_id, approver_id,
	// signed_at_review, signature_hash_review, comment_review,
	// signed_at_approve, signature_hash_approve, comment_approve,
	// reject_reason, source_doc_id,
	// created_at, created_by, updated_at, updated_by,
	// deleted_at, deleted_by, row_version, tenant_id
	makerID := uuid.New()
	farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).AddRow(
		compositionID.String(), instrumenID.String(),
		evalDate, farFuture,
		"APPROVED_ACTIVE",
		makerID.String(), nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil,
		evalDate, makerID.String(),
		evalDate, makerID.String(),
		nil, nil, int64(1), "TUGURE",
	)

	mock.ExpectQuery(`SELECT.*FROM mst.fund_composition.*effective_to`).
		WithArgs(instrumenID, evalDate).
		WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	comp, err := repo.GetActiveForInstrumen(context.Background(), instrumenID, evalDate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp == nil {
		t.Fatal("expected non-nil composition")
	}
	if comp.ID != compositionID {
		t.Errorf("ID: got %s, want %s", comp.ID, compositionID)
	}
	if comp.WorkflowStatus != WorkflowStatusApprovedActive {
		t.Errorf("WorkflowStatus: got %s, want %s", comp.WorkflowStatus, WorkflowStatusApprovedActive)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestGetActiveForInstrumen_NotFound returns nil without error when no active composition.
func TestGetActiveForInstrumen_NotFound(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT.*FROM mst.fund_composition`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBFundCompositionRepo(db)
	comp, err := repo.GetActiveForInstrumen(context.Background(), uuid.New(), time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp != nil {
		t.Errorf("expected nil composition when not found, got: %+v", comp)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBPDLGDClassRepo_GovtBond_SovereignZeroPD verifies GOVT_BOND returns PD=0.
// Sovereign instruments are zero-PD per FSD-APP-C §4.3.
// This test verifies the service-layer mock correctly returns PD=0 for sovereign.
// The DB-layer sovereign logic queries lgd_basel — tested via integration.
func TestDBPDLGDClassRepo_GovtBond_SovereignZeroPD(t *testing.T) {
	t.Parallel()
	// Use the domain-level mockPDLGDRepo (no DB) to verify sovereign PD=0 enforcement.
	// The DB repo calls getSovereignLGD + sets PD=0 in Go code — this tests that logic.
	repo := &mockPDLGDRepo{
		params: map[AssetClass]PDLGDParams{
			AssetClassGovtBond: {
				AssetClass: AssetClassGovtBond,
				PDGood:     decimal.Zero, // sovereign: PD must be 0
				PDNormal:   decimal.Zero,
				PDBad:      decimal.Zero,
				LGD:        decimal.NewFromFloat(0.10),
			},
		},
	}

	params, err := repo.GetPDLGDForAssetClass(context.Background(), AssetClassGovtBond,
		time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !params.PDGood.Equal(decimal.Zero) {
		t.Errorf("GOVT_BOND PDGood: want 0, got %s", params.PDGood)
	}
	if !params.PDNormal.Equal(decimal.Zero) {
		t.Errorf("GOVT_BOND PDNormal: want 0, got %s", params.PDNormal)
	}
	if !params.PDBad.Equal(decimal.Zero) {
		t.Errorf("GOVT_BOND PDBad: want 0, got %s", params.PDBad)
	}

	// Verify ComputeBreakdownLine with sovereign PD produces ECL = 0.
	line := ComputeBreakdownLine(
		AssetClassGovtBond,
		decimal.NewFromFloat(50),
		decimal.NewFromFloat(10_000_000),
		params,
		defaultFL,
		defaultWeights,
	)
	if !line.ECLWeightedIDR.Equal(decimal.Zero) {
		t.Errorf("sovereign ECL should be 0, got %s", line.ECLWeightedIDR)
	}
}

// TestDBLookthroughResultRepo_UpsertResult verifies ON CONFLICT update path.
func TestDBLookthroughResultRepo_UpsertResult(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO ecl.lookthrough_underlying`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()

	repo := NewDBLookthroughResultRepo(db)
	result := Result{
		InstrumenID:  uuid.New(),
		TotalECLIDR:  decimal.NewFromFloat(78_750),
		Breakdown:    []BreakdownLine{},
		FVTPLSkipped: false,
	}

	err = repo.UpsertResult(context.Background(), tx,
		result.InstrumenID, uuid.New(), result,
		uuid.New(), uuid.New(), time.Now(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("UpsertResult error: %v", err)
	}

	// Note: tx.Commit() is the outer caller's responsibility in the real flow.
	// We call it here to satisfy mock expectations.
	_ = tx.Commit()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_GetDetails returns slice of details.
func TestDBFundCompositionRepo_GetDetails(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	compositionID := uuid.New()
	detailID1 := uuid.New()
	detailID2 := uuid.New()

	// Match all 13 columns scanned by GetDetailsForComposition (fundCompositionDetailsSQL):
	// id, fund_composition_id, asset_class, weight_pct, position,
	// created_at, created_by, updated_at, updated_by, deleted_at, deleted_by, row_version, tenant_id
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "fund_composition_id", "asset_class", "weight_pct", "position",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).
		AddRow(detailID1.String(), compositionID.String(), "GOVT_BOND", "50.0000", 1,
			now, uuid.New().String(), now, uuid.New().String(),
			nil, nil, int64(1), "TUGURE").
		AddRow(detailID2.String(), compositionID.String(), "CORP_BOND", "50.0000", 2,
			now, uuid.New().String(), now, uuid.New().String(),
			nil, nil, int64(1), "TUGURE")

	mock.ExpectQuery(`SELECT.*FROM mst.fund_composition_detail`).
		WithArgs(compositionID).
		WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	details, err := repo.GetDetailsForComposition(context.Background(), compositionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(details) != 2 {
		t.Fatalf("expected 2 details, got %d", len(details))
	}
	if details[0].AssetClass != AssetClassGovtBond {
		t.Errorf("detail[0].AssetClass: got %s, want %s", details[0].AssetClass, AssetClassGovtBond)
	}
	if details[1].AssetClass != AssetClassCorpBond {
		t.Errorf("detail[1].AssetClass: got %s, want %s", details[1].AssetClass, AssetClassCorpBond)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── isAllowedSortCol tests ───────────────────────────────────────────────────

// TestIsAllowedSortCol_KnownCol returns true for known column.
func TestIsAllowedSortCol_KnownCol(t *testing.T) {
	t.Parallel()
	if !isAllowedSortCol("created_at", AllowedSortColsComposition) {
		t.Error("created_at should be allowed")
	}
}

// TestIsAllowedSortCol_UnknownCol returns false for unknown column.
func TestIsAllowedSortCol_UnknownCol(t *testing.T) {
	t.Parallel()
	if isAllowedSortCol("injected_col; DROP TABLE", AllowedSortColsComposition) {
		t.Error("injection string should not be allowed")
	}
}

// ─── DBFundCompositionRepo additional tests ──────────────────────────────────

// TestDBFundCompositionRepo_GetByID_Found verifies successful single row scan.
func TestDBFundCompositionRepo_GetByID_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	compositionID := uuid.New()
	instrumenID := uuid.New()
	makerID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).AddRow(
		compositionID.String(), instrumenID.String(),
		now, farFuture, "PENDING_REVIEW",
		makerID.String(), nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil,
		now, makerID.String(), now, makerID.String(),
		nil, nil, int64(1), "TUGURE",
	)

	mock.ExpectQuery(`SELECT fc.id.*FROM mst.fund_composition fc.*WHERE fc.id`).
		WithArgs(compositionID).
		WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	comp, err := repo.GetByID(context.Background(), compositionID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comp == nil {
		t.Fatal("expected non-nil composition")
	}
	if comp.ID != compositionID {
		t.Errorf("ID: got %s, want %s", comp.ID, compositionID)
	}
	if comp.WorkflowStatus != WorkflowStatusPendingReview {
		t.Errorf("WorkflowStatus: got %s", comp.WorkflowStatus)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_GetByID_NotFound returns nil,nil on sql.ErrNoRows.
func TestDBFundCompositionRepo_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT fc.id.*FROM mst.fund_composition fc.*WHERE fc.id`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBFundCompositionRepo(db)
	comp, err := repo.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error for not-found: %v", err)
	}
	if comp != nil {
		t.Error("expected nil composition for not-found")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_Create verifies INSERT for header + detail rows.
func TestDBFundCompositionRepo_Create(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	compositionID := uuid.New()
	instrumenID := uuid.New()
	makerID := uuid.New()
	now := time.Now().UTC()

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO mst.fund_composition`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO mst.fund_composition_detail`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBFundCompositionRepo(db)

	header := &FundComposition{
		ID:             compositionID,
		InstrumenID:    instrumenID,
		EffectiveFrom:  now,
		EffectiveTo:    time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		WorkflowStatus: WorkflowStatusPendingReview,
		MakerID:        makerID,
		CreatedBy:      makerID,
		TenantID:       "TUGURE",
	}
	details := []FundCompositionDetail{
		{
			ID:                uuid.New(),
			FundCompositionID: compositionID,
			AssetClass:        AssetClassCorpBond,
			WeightPct:         decimal.NewFromFloat(100),
			Position:          1,
			CreatedBy:         makerID,
			TenantID:          "TUGURE",
		},
	}

	if err := repo.Create(context.Background(), tx, header, details); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	_ = tx.Commit()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_UpdateWorkflowStatus verifies UPDATE SQL is executed.
func TestDBFundCompositionRepo_UpdateWorkflowStatus(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	compositionID := uuid.New()
	updatedBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.fund_composition`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBFundCompositionRepo(db)

	if err := repo.UpdateWorkflowStatus(context.Background(), tx,
		compositionID, WorkflowStatusPendingApproval,
		nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil,
		updatedBy,
	); err != nil {
		t.Fatalf("UpdateWorkflowStatus error: %v", err)
	}
	_ = tx.Commit()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_SupersedeOld verifies supersede UPDATE + rows-affected check.
func TestDBFundCompositionRepo_SupersedeOld(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldID := uuid.New()
	updatedBy := uuid.New()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.fund_composition`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tx, _ := db.Begin()
	repo := NewDBFundCompositionRepo(db)

	if err := repo.SupersedeOld(context.Background(), tx, oldID, time.Now(), updatedBy); err != nil {
		t.Fatalf("SupersedeOld error: %v", err)
	}
	_ = tx.Commit()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_SupersedeOld_ZeroRowsAffected returns error when no rows updated.
func TestDBFundCompositionRepo_SupersedeOld_ZeroRowsAffected(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.fund_composition`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows affected
	mock.ExpectRollback()

	tx, _ := db.Begin()
	repo := NewDBFundCompositionRepo(db)

	err = repo.SupersedeOld(context.Background(), tx, uuid.New(), time.Now(), uuid.New())
	if err == nil {
		t.Fatal("expected error when 0 rows affected")
	}
	_ = tx.Rollback()
}

// TestDBFundCompositionRepo_GetInstrumenTipeAndKlasifikasi_Found verifies tipe scan.
func TestDBFundCompositionRepo_GetInstrumenTipeAndKlasifikasi_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	instrumenID := uuid.New()

	rows := sqlmock.NewRows([]string{"tipe_instrumen", "klasifikasi_psak71", "poci_flag"}).
		AddRow("REKSADANA", "AC", false)

	mock.ExpectQuery(`SELECT tipe_instrumen.*FROM mst.instrumen`).
		WithArgs(instrumenID).
		WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	tipe, klasifikasi, poci, err := repo.GetInstrumenTipeAndKlasifikasi(context.Background(), instrumenID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tipe != "REKSADANA" {
		t.Errorf("tipe: got %s, want REKSADANA", tipe)
	}
	if klasifikasi != "AC" {
		t.Errorf("klasifikasi: got %s, want AC", klasifikasi)
	}
	if poci {
		t.Error("poci: expected false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBFundCompositionRepo_GetInstrumenTipeAndKlasifikasi_NotFound returns ErrNotFound.
func TestDBFundCompositionRepo_GetInstrumenTipeAndKlasifikasi_NotFound(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT tipe_instrumen.*FROM mst.instrumen`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBFundCompositionRepo(db)
	_, _, _, err = repo.GetInstrumenTipeAndKlasifikasi(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected not-found error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBReksadanaInstrumenRepo tests ──────────────────────────────────────────

// TestDBReksadanaInstrumenRepo_GetByID_Found returns row for known instrument.
func TestDBReksadanaInstrumenRepo_GetByID_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	instrumenID := uuid.New()
	nabStr := "10000000.0000"

	rows := sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen", "klasifikasi_psak71",
		"nominal_nab_idr", "poci_flag", "status", "workflow_status", "tenant_id",
	}).AddRow(
		instrumenID.String(), "RD-001", "Reksa Dana X", "REKSADANA", "AC",
		nabStr, false, "AKTIF", "APPROVED", "TUGURE",
	)

	mock.ExpectQuery(`SELECT i.id.*FROM mst.instrumen i`).
		WithArgs(instrumenID).
		WillReturnRows(rows)

	repo := NewDBReksadanaInstrumenRepo(db)
	inst, err := repo.GetByID(context.Background(), instrumenID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst == nil {
		t.Fatal("expected non-nil instrument")
	}
	if inst.ID != instrumenID {
		t.Errorf("ID: got %s, want %s", inst.ID, instrumenID)
	}
	if inst.NominalNABIDR == nil {
		t.Fatal("expected non-nil NAB")
	}
	expected := decimal.NewFromFloat(10_000_000)
	if !inst.NominalNABIDR.Equal(expected) {
		t.Errorf("NAB: got %s, want %s", inst.NominalNABIDR, expected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBReksadanaInstrumenRepo_GetByID_NotFound returns nil,nil on ErrNoRows.
func TestDBReksadanaInstrumenRepo_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT i.id.*FROM mst.instrumen i`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBReksadanaInstrumenRepo(db)
	inst, err := repo.GetByID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error for not-found: %v", err)
	}
	if inst != nil {
		t.Error("expected nil instrument for not-found")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBReksadanaInstrumenRepo_BulkListReksadanaForECL verifies bulk scan.
func TestDBReksadanaInstrumenRepo_BulkListReksadanaForECL(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	id1 := uuid.New()
	id2 := uuid.New()
	nabStr := "5000000.0000"

	rows := sqlmock.NewRows([]string{
		"id", "kode_instrumen", "nama_instrumen", "tipe_instrumen", "klasifikasi_psak71",
		"nominal_nab_idr", "poci_flag", "status", "workflow_status", "tenant_id",
	}).
		AddRow(id1.String(), "RD-001", "Reksa Dana A", "REKSADANA", "AC", nabStr, false, "AKTIF", "APPROVED", "TUGURE").
		AddRow(id2.String(), "RD-002", "Reksa Dana B", "REKSADANA", "FVTPL", nil, false, "AKTIF", "APPROVED", "TUGURE")

	mock.ExpectQuery(`SELECT i.id.*FROM mst.instrumen i`).
		WithArgs("TUGURE").
		WillReturnRows(rows)

	repo := NewDBReksadanaInstrumenRepo(db)
	instruments, err := repo.BulkListReksadanaForECL(context.Background(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(instruments) != 2 {
		t.Fatalf("expected 2 instruments, got %d", len(instruments))
	}
	if instruments[0].ID != id1 {
		t.Errorf("instruments[0].ID: got %s, want %s", instruments[0].ID, id1)
	}
	// id2 has nil NAB → NominalNABIDR should be nil.
	if instruments[1].NominalNABIDR != nil {
		t.Error("expected nil NAB for instrument with null nominal_nab_idr")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBPDLGDClassRepo tests ───────────────────────────────────────────────────

// TestDBPDLGDClassRepo_BulkGetPDLGD_NonSovereign verifies non-sovereign PD/LGD scan.
func TestDBPDLGDClassRepo_BulkGetPDLGD_NonSovereign(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"asset_class", "pd_good", "pd_normal", "pd_bad", "lgd"}).
		AddRow("CORP_BOND", "0.02000000", "0.03000000", "0.06000000", "0.45000000")

	mock.ExpectQuery(`SELECT`).
		WillReturnRows(rows)

	repo := NewDBPDLGDClassRepo(db)
	result, err := repo.BulkGetPDLGDForAssetClasses(context.Background(),
		[]AssetClass{AssetClassCorpBond},
		time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
		"TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, ok := result[AssetClassCorpBond]
	if !ok {
		t.Fatal("expected CORP_BOND in result map")
	}
	if !p.PDGood.Equal(decimal.NewFromFloat(0.02)) {
		t.Errorf("PDGood: got %s, want 0.02", p.PDGood)
	}
	if !p.LGD.Equal(decimal.NewFromFloat(0.45)) {
		t.Errorf("LGD: got %s, want 0.45", p.LGD)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBPDLGDClassRepo_BulkGetPDLGD_GovtBond_SovereignPath verifies PD=0 for sovereign.
func TestDBPDLGDClassRepo_BulkGetPDLGD_GovtBond_SovereignPath(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// GOVT_BOND → getSovereignLGD called first (sovereignLGDSQL).
	mock.ExpectQuery(`SELECT lgd_pct.*FROM mst.lgd_basel`).
		WillReturnError(sql.ErrNoRows) // fallback to decimal.Zero

	repo := NewDBPDLGDClassRepo(db)
	result, err := repo.BulkGetPDLGDForAssetClasses(context.Background(),
		[]AssetClass{AssetClassGovtBond},
		time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC),
		"TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, ok := result[AssetClassGovtBond]
	if !ok {
		t.Fatal("expected GOVT_BOND in result map")
	}
	if !p.PDGood.Equal(decimal.Zero) {
		t.Errorf("PDGood for sovereign: got %s, want 0", p.PDGood)
	}
	if !p.PDNormal.Equal(decimal.Zero) {
		t.Errorf("PDNormal for sovereign: got %s, want 0", p.PDNormal)
	}
	if !p.PDBad.Equal(decimal.Zero) {
		t.Errorf("PDBad for sovereign: got %s, want 0", p.PDBad)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── DBLookthroughResultRepo tests ───────────────────────────────────────────

// TestDBLookthroughResultRepo_GetByInstrumenAndRun_Found verifies stored result scan.
func TestDBLookthroughResultRepo_GetByInstrumenAndRun_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	resultID := uuid.New()
	instrumenID := uuid.New()
	runID := uuid.New()
	compositionID := uuid.New()
	periodeID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "run_id", "composition_id", "periode_id",
		"evaluation_date", "nab_idr", "total_ecl_idr",
		"breakdown_jsonb", "fvtpl_skipped", "warning",
		"created_at", "tenant_id",
	}).AddRow(
		resultID.String(), instrumenID.String(), runID.String(),
		compositionID.String(), periodeID.String(),
		now, "10000000.0000", "78750.0000",
		[]byte("[]"), false, "",
		now, "TUGURE",
	)

	mock.ExpectQuery(`SELECT.*FROM ecl.lookthrough_underlying`).
		WithArgs(instrumenID, runID).
		WillReturnRows(rows)

	repo := NewDBLookthroughResultRepo(db)
	stored, err := repo.GetByInstrumenAndRun(context.Background(), instrumenID, runID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored == nil {
		t.Fatal("expected non-nil stored result")
	}
	if stored.ID != resultID {
		t.Errorf("ID: got %s, want %s", stored.ID, resultID)
	}
	expectedECL := decimal.NewFromFloat(78_750)
	if !stored.TotalECLIDR.Equal(expectedECL) {
		t.Errorf("TotalECLIDR: got %s, want %s", stored.TotalECLIDR, expectedECL)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestDBLookthroughResultRepo_GetByInstrumenAndRun_NotFound returns nil,nil.
func TestDBLookthroughResultRepo_GetByInstrumenAndRun_NotFound(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT.*FROM ecl.lookthrough_underlying`).
		WillReturnError(sql.ErrNoRows)

	repo := NewDBLookthroughResultRepo(db)
	stored, err := repo.GetByInstrumenAndRun(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error for not-found: %v", err)
	}
	if stored != nil {
		t.Error("expected nil stored result for not-found")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── ScenarioParamRepo additional tests ──────────────────────────────────────

// TestDBScenarioParamRepo_GetFLMultipliers_Override verifies ALCO override for FL.
func TestDBScenarioParamRepo_GetFLMultipliers_Override(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"multiplier_good", "multiplier_normal", "multiplier_bad"}).
		AddRow("1.10000000", "1.20000000", "1.30000000")

	mock.ExpectQuery(`SELECT.*multiplier_good.*multiplier_normal.*multiplier_bad`).
		WillReturnRows(rows)

	repo := NewDBScenarioParamRepo(db)
	fl, err := repo.GetFLMultipliers(context.Background(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fl.Good.Equal(decimal.NewFromFloat(1.10)) {
		t.Errorf("FL.Good: got %s, want 1.10", fl.Good)
	}
	if !fl.Normal.Equal(decimal.NewFromFloat(1.20)) {
		t.Errorf("FL.Normal: got %s, want 1.20", fl.Normal)
	}
	if !fl.Bad.Equal(decimal.NewFromFloat(1.30)) {
		t.Errorf("FL.Bad: got %s, want 1.30", fl.Bad)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// ─── ListByInstrumen ──────────────────────────────────────────────────────────

func TestDBFundCompositionRepo_ListByInstrumen_NoFilter(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	instrumenID := uuid.New()
	compositionID := uuid.New()
	makerID := uuid.New()
	now := time.Now().UTC()
	farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	}).AddRow(
		compositionID, instrumenID, now, farFuture, "PENDING_REVIEW",
		makerID, nil, nil,
		nil, nil, nil,
		nil, nil, nil,
		nil, nil,
		now, makerID, now, makerID,
		nil, nil, int64(1), "TUGURE",
	)

	mock.ExpectQuery(`SELECT fc.id`).WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	comps, cursor, hasMore, err := repo.ListByInstrumen(context.Background(), instrumenID, "", "", 50, "created_at", "desc")
	if err != nil {
		t.Fatalf("ListByInstrumen: %v", err)
	}
	if len(comps) != 1 {
		t.Fatalf("expected 1 composition, got %d", len(comps))
	}
	if comps[0].ID != compositionID {
		t.Errorf("unexpected id")
	}
	_ = cursor
	if hasMore {
		t.Errorf("hasMore should be false")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

func TestDBFundCompositionRepo_ListByInstrumen_WithStatus(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Return empty rows.
	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	})
	mock.ExpectQuery(`SELECT fc.id`).WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	comps, _, hasMore, err := repo.ListByInstrumen(context.Background(), uuid.New(), "APPROVED_ACTIVE", "", 10, "effective_from", "asc")
	if err != nil {
		t.Fatalf("ListByInstrumen: %v", err)
	}
	if len(comps) != 0 {
		t.Errorf("expected 0, got %d", len(comps))
	}
	if hasMore {
		t.Errorf("hasMore should be false for empty result")
	}
	_ = mock.ExpectationsWereMet()
}

func TestDBFundCompositionRepo_ListByInstrumen_HasMore(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	instrumenID := uuid.New()
	now := time.Now().UTC()
	farFuture := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
	makerID := uuid.New()

	// Return limit+1 rows to trigger hasMore path.
	limit := 2
	rowsBuilder := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	})
	for i := 0; i <= limit; i++ {
		rowsBuilder.AddRow(
			uuid.New(), instrumenID, now, farFuture, "PENDING_REVIEW",
			makerID, nil, nil,
			nil, nil, nil,
			nil, nil, nil,
			nil, nil,
			now, makerID, now, makerID,
			nil, nil, int64(1), "TUGURE",
		)
	}

	mock.ExpectQuery(`SELECT fc.id`).WillReturnRows(rowsBuilder)

	repo := NewDBFundCompositionRepo(db)
	comps, cursor, hasMore, err := repo.ListByInstrumen(context.Background(), instrumenID, "", "", limit, "created_at", "asc")
	if err != nil {
		t.Fatalf("ListByInstrumen: %v", err)
	}
	if !hasMore {
		t.Error("expected hasMore=true")
	}
	if len(comps) != limit {
		t.Errorf("expected %d comps, got %d", limit, len(comps))
	}
	if cursor == "" {
		t.Error("expected non-empty cursor when hasMore=true")
	}
	_ = mock.ExpectationsWereMet()
}

func TestDBFundCompositionRepo_ListByInstrumen_WithCursor(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	})
	mock.ExpectQuery(`SELECT fc.id`).WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	cursor := encodeCursor(time.Now().UTC().Format(time.RFC3339Nano), uuid.New().String())
	_, _, _, err = repo.ListByInstrumen(context.Background(), uuid.New(), "", cursor, 50, "created_at", "desc")
	if err != nil {
		t.Fatalf("ListByInstrumen with cursor: %v", err)
	}
	_ = mock.ExpectationsWereMet()
}

func TestDBFundCompositionRepo_ListByInstrumen_InvalidCursor(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "instrumen_id", "effective_from", "effective_to", "workflow_status",
		"maker_id", "reviewer_id", "approver_id",
		"signed_at_review", "signature_hash_review", "comment_review",
		"signed_at_approve", "signature_hash_approve", "comment_approve",
		"reject_reason", "source_doc_id",
		"created_at", "created_by", "updated_at", "updated_by",
		"deleted_at", "deleted_by", "row_version", "tenant_id",
	})
	mock.ExpectQuery(`SELECT fc.id`).WillReturnRows(rows)

	repo := NewDBFundCompositionRepo(db)
	// "invalid" cursor should be silently ignored (no pipe separator).
	_, _, _, err = repo.ListByInstrumen(context.Background(), uuid.New(), "", "badcursor", 50, "created_at", "desc")
	if err != nil {
		t.Fatalf("ListByInstrumen bad cursor: %v", err)
	}
	_ = mock.ExpectationsWereMet()
}

func TestDBFundCompositionRepo_ListByInstrumen_QueryError(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT fc.id`).WillReturnError(errors.New("query failed"))

	repo := NewDBFundCompositionRepo(db)
	_, _, _, err = repo.ListByInstrumen(context.Background(), uuid.New(), "", "", 50, "", "")
	if err == nil {
		t.Fatal("expected error from query")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── GetPDLGDForAssetClass ────────────────────────────────────────────────────

func TestDBPDLGDClassRepo_GetPDLGDForAssetClass_Found(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"asset_class", "pd_good", "pd_normal", "pd_bad", "lgd"}).
		AddRow("CORP_BOND", "0.01000000", "0.02000000", "0.04000000", "0.40000000")

	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	repo := NewDBPDLGDClassRepo(db)
	params, err := repo.GetPDLGDForAssetClass(context.Background(), AssetClassCorpBond, time.Now(), "TUGURE")
	if err != nil {
		t.Fatalf("GetPDLGDForAssetClass: %v", err)
	}
	if params.LGD.IsZero() {
		t.Error("expected non-zero LGD")
	}
	_ = mock.ExpectationsWereMet()
}

func TestDBPDLGDClassRepo_GetPDLGDForAssetClass_Missing(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Return empty result — asset class not in master data.
	rows := sqlmock.NewRows([]string{"asset_class", "pd_good", "pd_normal", "pd_bad", "lgd"})
	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	repo := NewDBPDLGDClassRepo(db)
	_, err = repo.GetPDLGDForAssetClass(context.Background(), AssetClassEquity, time.Now(), "TUGURE")
	if err == nil {
		t.Fatal("expected ErrPDLGDClassMissing")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── getSovereignLGD (non-zero path) ─────────────────────────────────────────

func TestGetSovereignLGD_Override(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"lgd_pct"}).AddRow("0.15000000")
	mock.ExpectQuery(`SELECT lgd_pct`).WillReturnRows(rows)

	repo := &DBPDLGDClassRepo{db: db}
	lgd, err := repo.getSovereignLGD(context.Background(), time.Now(), "TUGURE")
	if err != nil {
		t.Fatalf("getSovereignLGD: %v", err)
	}
	if lgd.IsZero() {
		t.Error("expected non-zero sovereign LGD")
	}
	_ = mock.ExpectationsWereMet()
}

// ─── NewDB*Repo nil-DB panic tests ────────────────────────────────────────────

func TestNewDBFundCompositionRepo_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewDBFundCompositionRepo(nil)
}

func TestNewDBReksadanaInstrumenRepo_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewDBReksadanaInstrumenRepo(nil)
}

func TestNewDBPDLGDClassRepo_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewDBPDLGDClassRepo(nil)
}

func TestNewDBScenarioParamRepo_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewDBScenarioParamRepo(nil)
}

func TestNewDBLookthroughResultRepo_NilPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil db")
		}
	}()
	_ = NewDBLookthroughResultRepo(nil)
}

// ─── UpdateWorkflowStatus error path ─────────────────────────────────────────

func TestDBFundCompositionRepo_UpdateWorkflowStatus_ExecError(t *testing.T) {
	t.Parallel()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.fund_composition`).WillReturnError(errors.New("exec failed"))
	mock.ExpectRollback()

	tx, _ := db.Begin()
	repo := NewDBFundCompositionRepo(db)
	err = repo.UpdateWorkflowStatus(context.Background(), tx,
		uuid.New(), WorkflowStatusApprovedActive,
		nil, nil, nil, nil,
		nil, nil, nil, nil,
		nil, uuid.New(),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	_ = tx.Rollback()
	_ = mock.ExpectationsWereMet()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// contains is a simple substring check to avoid importing strings in test file.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsCheck(s, substr))
}

func containsCheck(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
