// Package eir — tests for computeCatchUpAdjustment and Approve catch-up integration.
//
// IFRS 9 §5.4.3: when an EIR amendment is approved the carrying amount must be
// remeasured to the NPV of revised cashflows discounted at the ORIGINAL EIR.
// The difference (catch-up) is recognized immediately in P&L.
//
//	catch_up = NPV(revisedCF @ originalEIR) − grossCarrying(at amendmentDate)
//
// Positive = P&L gain (NPV > carrying).
// Negative = P&L loss (NPV < carrying).
//
// Jurnal P&L booking is deferred to Phase 5 (APP-D jurnal module, M7+).
// For now: value stored in ecl.eir_reestimation_log.catch_up_adjustment and
// emitted in EIR.AMEND_APPROVED audit event.
//
// Precision: shopspring/decimal throughout — NEVER float64 (DEC-016).
// Ref: FSD-APP-C-ECL-EIR-v1.0.docx §4.3, SoW §4.
package eir

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Ensure sqlmock is used (it is used by newMockDB via service_test.go helpers in same package).
var _ = sqlmock.New

// ─── Unit tests for computeCatchUpAdjustment ─────────────────────────────────

// TestComputeCatchUpAdjustment_HappyPath verifies that the catch-up equals
// NPV(revisedCF @ originalEIR) − grossCarrying with known values.
//
// Setup:
//
//	originalEIR  = 0.08 (8% annual)
//	grossCarrying = 1_000_000_000.0000
//	revisedCF:   one future inflow of 1_100_000_000 in 365 days
//	NPV          = 1_100_000_000 / (1 + 0.08)^1.0 ≈ 1_018_518_519  (approx)
//	catch_up     = NPV − 1_000_000_000 > 0  (gain)
func TestComputeCatchUpAdjustment_HappyPath(t *testing.T) {
	amendDate := date(2026, 6, 1)
	originalEIR := mustDec("0.08")
	grossCarrying := mustDec("1000000000.0000")

	revisedCFs := []CashflowItem{
		{Date: amendDate.AddDate(0, 0, -10), AmountIDR: mustDec("-500000000")}, // past, ignored
		{Date: amendDate.AddDate(0, 0, 365), AmountIDR: mustDec("1100000000")}, // future inflow
	}

	svc := &AmendmentService{
		schedRepo: &stubScheduleRepo{grossCarrying: grossCarrying},
		logger:    testLogger(),
	}

	catchUp, err := svc.computeCatchUpAdjustment(context.Background(), uuid.New(), revisedCFs, originalEIR, amendDate)
	if err != nil {
		t.Fatalf("computeCatchUpAdjustment: %v", err)
	}

	// NPV ≈ 1_100_000_000 / 1.08 ≈ 1_018_518_518.5185...
	// catch_up = NPV - 1_000_000_000 should be > 0
	if !catchUp.IsPositive() {
		t.Errorf("catch_up should be positive (gain), got %s", catchUp.StringFixed(4))
	}

	// Sanity: catch_up ≈ 18_518_519 ± 100 (rounding tolerance)
	lo := mustDec("18000000")
	hi := mustDec("19000000")
	if catchUp.LessThan(lo) || catchUp.GreaterThan(hi) {
		t.Errorf("catch_up %s out of expected range [%s, %s]",
			catchUp.StringFixed(4), lo.StringFixed(0), hi.StringFixed(0))
	}

	// Must be rounded to 4 decimal places (HALF_EVEN per DEC-016)
	if !catchUp.Equal(catchUp.RoundBank(4)) {
		t.Errorf("catch_up not rounded to 4dp: %s", catchUp.String())
	}
	t.Logf("catch_up = %s", catchUp.StringFixed(4))
}

// TestComputeCatchUpAdjustment_NPVLessThanGross_NegativeCatchUp confirms that a
// reduced future cashflow produces a negative catch-up (P&L loss scenario).
//
// Setup:
//
//	originalEIR  = 0.08
//	grossCarrying = 1_000_000_000.0000
//	revisedCF:   one future inflow of 900_000_000 in 365 days
//	NPV          = 900_000_000 / 1.08 ≈ 833_333_333  < 1_000_000_000
//	catch_up     < 0  (loss)
func TestComputeCatchUpAdjustment_NPVLessThanGross_NegativeCatchUp(t *testing.T) {
	amendDate := date(2026, 6, 1)
	originalEIR := mustDec("0.08")
	grossCarrying := mustDec("1000000000.0000")

	revisedCFs := []CashflowItem{
		{Date: amendDate.AddDate(0, 0, 365), AmountIDR: mustDec("900000000")},
	}

	svc := &AmendmentService{
		schedRepo: &stubScheduleRepo{grossCarrying: grossCarrying},
		logger:    testLogger(),
	}

	catchUp, err := svc.computeCatchUpAdjustment(context.Background(), uuid.New(), revisedCFs, originalEIR, amendDate)
	if err != nil {
		t.Fatalf("computeCatchUpAdjustment: %v", err)
	}

	if !catchUp.IsNegative() {
		t.Errorf("catch_up should be negative (loss), got %s", catchUp.StringFixed(4))
	}
	t.Logf("catch_up (loss) = %s", catchUp.StringFixed(4))
}

// TestComputeCatchUpAdjustment_NPVGreaterThanGross_PositiveCatchUp confirms gain scenario
// explicitly with multi-cashflow series.
func TestComputeCatchUpAdjustment_NPVGreaterThanGross_PositiveCatchUp(t *testing.T) {
	amendDate := date(2026, 6, 1)
	originalEIR := mustDec("0.05")
	grossCarrying := mustDec("800000000.0000")

	// Two future inflows summing to > grossCarrying in PV terms
	revisedCFs := []CashflowItem{
		{Date: amendDate.AddDate(0, 0, 182), AmountIDR: mustDec("500000000")},
		{Date: amendDate.AddDate(0, 0, 365), AmountIDR: mustDec("500000000")},
	}

	svc := &AmendmentService{
		schedRepo: &stubScheduleRepo{grossCarrying: grossCarrying},
		logger:    testLogger(),
	}

	catchUp, err := svc.computeCatchUpAdjustment(context.Background(), uuid.New(), revisedCFs, originalEIR, amendDate)
	if err != nil {
		t.Fatalf("computeCatchUpAdjustment: %v", err)
	}

	if !catchUp.IsPositive() {
		t.Errorf("catch_up should be positive (gain), got %s", catchUp.StringFixed(4))
	}
	t.Logf("catch_up (gain, multi-CF) = %s", catchUp.StringFixed(4))
}

// TestComputeCatchUpAdjustment_PastCFsIgnored verifies that cashflows before
// the amendmentDate do not contribute to the NPV.
func TestComputeCatchUpAdjustment_PastCFsIgnored(t *testing.T) {
	amendDate := date(2026, 6, 1)
	originalEIR := mustDec("0.08")
	grossCarrying := mustDec("1000000000.0000")

	// Only a past CF — nothing in the future; NPV = 0
	revisedCFs := []CashflowItem{
		{Date: amendDate.AddDate(0, 0, -100), AmountIDR: mustDec("500000000")},
	}

	svc := &AmendmentService{
		schedRepo: &stubScheduleRepo{grossCarrying: grossCarrying},
		logger:    testLogger(),
	}

	catchUp, err := svc.computeCatchUpAdjustment(context.Background(), uuid.New(), revisedCFs, originalEIR, amendDate)
	if err != nil {
		t.Fatalf("computeCatchUpAdjustment: %v", err)
	}

	// NPV = 0 (no future CFs), catch_up = 0 − 1_000_000_000 < 0
	if !catchUp.IsNegative() {
		t.Errorf("catch_up should be negative when no future CFs, got %s", catchUp.StringFixed(4))
	}
	expected := grossCarrying.Neg().RoundBank(4)
	if !catchUp.Equal(expected) {
		t.Errorf("catch_up: want %s, got %s", expected.StringFixed(4), catchUp.StringFixed(4))
	}
}

// TestComputeCatchUpAdjustment_ScheduleNotFound verifies graceful error propagation
// when GetGrossCarryingAtDate returns an error.
func TestComputeCatchUpAdjustment_ScheduleNotFound(t *testing.T) {
	amendDate := date(2026, 6, 1)
	errSched := ErrEIRScheduleNotFound("test-id")
	svc := &AmendmentService{
		schedRepo: &stubScheduleRepo{errGrossCarry: errSched},
		logger:    testLogger(),
	}

	_, err := svc.computeCatchUpAdjustment(context.Background(), uuid.New(),
		[]CashflowItem{{Date: amendDate.AddDate(0, 0, 365), AmountIDR: mustDec("1000000000")}},
		mustDec("0.08"), amendDate)

	if err == nil {
		t.Fatal("expected error from GetGrossCarryingAtDate, got nil")
	}
}

// ─── Integration tests for Approve storing catch_up ──────────────────────────

// TestApproveAmendment_StoresCatchUpInLog verifies that Approve writes
// catch_up_adjustment to the proposal and calls amendRepo.Update with it set.
func TestApproveAmendment_StoresCatchUpInLog(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	base := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	base.ReviewerID = &reviewerID
	amendRepo.proposals[base.ID] = &base
	amendRepo.activeForID[instrID] = true

	grossCarrying := mustDec("1005000000.0000") // abs(CF[0]) from obligasiAtDiscount2
	schedRepo := &stubScheduleRepo{maxSeq: 10, grossCarrying: grossCarrying}
	auditW := &stubAuditWriter{}

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}

	approved, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: base.ID,
		Comment:     "alco approved",
		StepUpToken: "mfa-token",
	}, approverID, "ROLE-ALCO")

	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if approved.Status != AmendStatusApproved {
		t.Errorf("expected APPROVED, got %s", approved.Status)
	}

	// catch_up_adjustment must be set on returned proposal
	if approved.CatchUpAdjustment == nil {
		t.Fatal("CatchUpAdjustment must not be nil after Approve")
	}

	// catch_up must be rounded to 4 decimal places (NUMERIC(20,4) storage)
	cu := *approved.CatchUpAdjustment
	if !cu.Equal(cu.RoundBank(4)) {
		t.Errorf("CatchUpAdjustment not rounded to 4dp: %s", cu.String())
	}

	// amendRepo.Update must have been called with catch_up set
	stored := amendRepo.proposals[base.ID]
	if stored == nil || stored.CatchUpAdjustment == nil {
		t.Fatal("CatchUpAdjustment not persisted in amendRepo")
	}
	if !stored.CatchUpAdjustment.Equal(*approved.CatchUpAdjustment) {
		t.Errorf("stored catch_up %s != returned %s",
			stored.CatchUpAdjustment.StringFixed(4), approved.CatchUpAdjustment.StringFixed(4))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
	t.Logf("catch_up_adjustment = %s", approved.CatchUpAdjustment.StringFixed(4))
}

// TestApproveAmendment_AuditEventIncludesCatchUp verifies that the EIR.AMEND_APPROVED
// audit event payload contains the catch_up_adjustment field (DEC-018).
func TestApproveAmendment_AuditEventIncludesCatchUp(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	base := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	base.ReviewerID = &reviewerID
	amendRepo.proposals[base.ID] = &base
	amendRepo.activeForID[instrID] = true

	schedRepo := &stubScheduleRepo{maxSeq: 5, grossCarrying: mustDec("1005000000.0000")}
	auditW := &stubAuditWriter{}

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}

	_, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: base.ID,
		Comment:     "approved",
		StepUpToken: "mfa-token",
	}, approverID, "ROLE-ALCO")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	// Find the EIR.AMEND_APPROVED audit event
	var approvedEvt *AuditEvent
	for i := range auditW.events {
		if auditW.events[i].Action == "EIR.AMEND_APPROVED" {
			approvedEvt = &auditW.events[i]
			break
		}
	}
	if approvedEvt == nil {
		t.Fatal("EIR.AMEND_APPROVED audit event not found")
	}

	afterMap, ok := approvedEvt.AfterJSON.(map[string]any)
	if !ok {
		t.Fatalf("AfterJSON is not map[string]any, got %T", approvedEvt.AfterJSON)
	}

	cuVal, ok := afterMap["catch_up_adjustment"]
	if !ok {
		t.Fatal("audit AfterJSON missing 'catch_up_adjustment' key")
	}

	cuStr, ok := cuVal.(string)
	if !ok || cuStr == "" {
		t.Errorf("audit catch_up_adjustment must be non-empty string, got %v (%T)", cuVal, cuVal)
	}

	// Validate the string is a parseable decimal
	if _, err := decimal.NewFromString(cuStr); err != nil {
		t.Errorf("audit catch_up_adjustment is not a valid decimal: %q — %v", cuStr, err)
	}

	t.Logf("audit catch_up_adjustment = %q", cuStr)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestApproveAmendment_NoPreviousSchedule_CatchUpZero verifies that when no prior
// schedule exists (GetGrossCarryingAtDate errors), Approve still succeeds and
// catch_up_adjustment is stored as zero (non-fatal degradation).
func TestApproveAmendment_NoPreviousSchedule_CatchUpZero(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	base := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	base.ReviewerID = &reviewerID
	amendRepo.proposals[base.ID] = &base
	amendRepo.activeForID[instrID] = true

	// schedRepo returns error from GetGrossCarryingAtDate (no prior schedule)
	schedRepo := &stubScheduleRepo{
		maxSeq:        0,
		errGrossCarry: ErrEIRScheduleNotFound(instrID.String()),
	}
	auditW := &stubAuditWriter{}

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}

	approved, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: base.ID,
		Comment:     "approved",
		StepUpToken: "mfa-token",
	}, approverID, "ROLE-ALCO")

	// Approve must still succeed
	if err != nil {
		t.Fatalf("Approve should succeed even when no prior schedule: %v", err)
	}
	if approved.Status != AmendStatusApproved {
		t.Errorf("expected APPROVED, got %s", approved.Status)
	}
	// catch_up_adjustment must be set (to zero as fallback)
	if approved.CatchUpAdjustment == nil {
		t.Fatal("CatchUpAdjustment must not be nil (should be zero fallback)")
	}
	if !approved.CatchUpAdjustment.IsZero() {
		t.Errorf("CatchUpAdjustment should be zero when no prior schedule, got %s",
			approved.CatchUpAdjustment.StringFixed(4))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock: %v", err)
	}
}

// TestApproveAmendment_CatchUpRounded verifies HALF_EVEN rounding to 4dp is applied.
// This guards against storing more than NUMERIC(20,4) precision (DEC-016).
func TestApproveAmendment_CatchUpRounded(t *testing.T) {
	instrID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	eirVal := mustDec("0.08")

	instrRepo := newStubInstrumenRepo()
	instrRepo.put(actInstrumen(instrID, "AC", &eirVal))

	amendRepo := newStubAmendmentRepo()
	base := makeProposal(instrID, makerID, AmendStatusPendingApproval, mustMarshalCF(t, obligasiAtDiscount2()))
	base.ReviewerID = &reviewerID
	amendRepo.proposals[base.ID] = &base
	amendRepo.activeForID[instrID] = true

	schedRepo := &stubScheduleRepo{maxSeq: 3, grossCarrying: mustDec("999999999.1234")}
	auditW := &stubAuditWriter{}

	db, mock := newMockDB(t)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := &AmendmentService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		amendRepo:   amendRepo,
		solver:      NewSolver(),
		auditWriter: auditW,
		logger:      testLogger(),
	}

	approved, err := svc.Approve(context.Background(), ApproveRequest{
		AmendmentID: base.ID,
		Comment:     "approved",
		StepUpToken: "mfa-token",
	}, approverID, "ROLE-ALCO")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}

	cu := approved.CatchUpAdjustment
	if cu == nil {
		t.Fatal("CatchUpAdjustment nil")
	}
	// Must equal its own 4dp-rounded value
	if !cu.Equal(cu.RoundBank(4)) {
		t.Errorf("CatchUpAdjustment has more than 4dp: %s", cu.String())
	}

	_ = mock.ExpectationsWereMet()
}
