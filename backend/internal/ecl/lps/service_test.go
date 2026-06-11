package lps

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Test doubles ─────────────────────────────────────────────────────────────

// mockCoverageRepo implements LPSCoverageRepoIface.
type mockCoverageRepo struct {
	row *LPSCoverageRow
	err error
}

func (m *mockCoverageRepo) GetActiveByEvaluationDate(ctx context.Context, evalDate time.Time) (*LPSCoverageRow, error) {
	return m.row, m.err
}

// mockDepositoRepo implements DepositoInstrumenRepoIface.
type mockDepositoRepo struct {
	byNasabahBank []InstrumenDepositoRow
	allPairs      []NasabahBankPair
	bulkRows      []BulkDepositoRow
	err           error
}

func (m *mockDepositoRepo) ListByNasabahBank(ctx context.Context, nasabahID, bankID uuid.UUID, evalDate time.Time) ([]InstrumenDepositoRow, error) {
	return m.byNasabahBank, m.err
}
func (m *mockDepositoRepo) ListAllActivePairs(ctx context.Context, evalDate time.Time) ([]NasabahBankPair, error) {
	return m.allPairs, m.err
}
func (m *mockDepositoRepo) BulkListDepositoForAggregate(ctx context.Context, evalDate time.Time) ([]BulkDepositoRow, error) {
	return m.bulkRows, m.err
}

// mockOverrideRepo implements OverrideRepoIface.
type mockOverrideRepo struct {
	overrides     map[uuid.UUID]*LPSExclusionOverride
	activeSet     map[uuid.UUID]*LPSExclusionOverride
	hasActivePend bool
	conflictID    string
	createErr     error
	approveErr    error
	rejectErr     error
}

func (m *mockOverrideRepo) GetByID(ctx context.Context, id uuid.UUID) (*LPSExclusionOverride, error) {
	if ov, ok := m.overrides[id]; ok {
		return ov, nil
	}
	return nil, nil
}
func (m *mockOverrideRepo) GetActiveForInstrumen(ctx context.Context, instrumenID uuid.UUID, evalDate time.Time) (*LPSExclusionOverride, error) {
	if ov, ok := m.activeSet[instrumenID]; ok {
		return ov, nil
	}
	return nil, nil
}
func (m *mockOverrideRepo) GetActiveSetForInstrumens(ctx context.Context, ids []uuid.UUID, evalDate time.Time) (map[uuid.UUID]*LPSExclusionOverride, error) {
	result := map[uuid.UUID]*LPSExclusionOverride{}
	for _, id := range ids {
		if ov, ok := m.activeSet[id]; ok {
			result[id] = ov
		}
	}
	return result, nil
}
func (m *mockOverrideRepo) HasActiveOrPendingForInstrumen(ctx context.Context, instrumenID uuid.UUID) (bool, string, error) {
	return m.hasActivePend, m.conflictID, nil
}
func (m *mockOverrideRepo) Create(ctx context.Context, tx *sql.Tx, o *LPSExclusionOverride) error {
	if m.createErr != nil {
		return m.createErr
	}
	o.ID = uuid.New()
	o.CreatedAt = time.Now()
	o.UpdatedAt = time.Now()
	o.RowVersion = 1
	if m.overrides == nil {
		m.overrides = map[uuid.UUID]*LPSExclusionOverride{}
	}
	cp := *o
	m.overrides[o.ID] = &cp
	return nil
}
func (m *mockOverrideRepo) Approve(ctx context.Context, tx *sql.Tx, id uuid.UUID, approverID uuid.UUID, signedAt time.Time, sigHash []byte, comment string, updatedBy uuid.UUID) error {
	if m.approveErr != nil {
		return m.approveErr
	}
	if ov, ok := m.overrides[id]; ok {
		ov.WorkflowStatus = WorkflowStatusApprovedActive
		ov.ApproverID = &approverID
		ov.SignedAtApprove = &signedAt
		ov.SignatureHashApprove = sigHash
		ov.CommentApprove = &comment
	}
	return nil
}
func (m *mockOverrideRepo) Reject(ctx context.Context, tx *sql.Tx, id uuid.UUID, actorID uuid.UUID, rejectReason string, updatedBy uuid.UUID) error {
	if m.rejectErr != nil {
		return m.rejectErr
	}
	if ov, ok := m.overrides[id]; ok {
		ov.WorkflowStatus = WorkflowStatusRejected
		ov.RejectReason = &rejectReason
	}
	return nil
}
func (m *mockOverrideRepo) List(ctx context.Context, filterWF, filterInstr, filterMaker, search, sortCol, sortDir, cursor string, limit int) ([]LPSExclusionOverride, string, bool, error) {
	result := make([]LPSExclusionOverride, 0, len(m.overrides))
	for _, ov := range m.overrides {
		result = append(result, *ov)
	}
	return result, "", false, nil
}

// mockKursRepo implements KursRepoIface.
type mockKursRepo struct {
	rates map[string]decimal.Decimal
	err   error
}

func (m *mockKursRepo) GetByDate(ctx context.Context, currency string, date time.Time) (decimal.Decimal, error) {
	if m.err != nil {
		return decimal.Zero, m.err
	}
	if rate, ok := m.rates[currency]; ok {
		return rate, nil
	}
	return decimal.Zero, errors.New("not found")
}

// mockPeriodeRepo implements PeriodeBukuRepoIface.
type mockPeriodeRepo struct {
	starts map[uuid.UUID]time.Time
	ends   map[uuid.UUID]time.Time
}

func (m *mockPeriodeRepo) GetStartEndDate(ctx context.Context, id uuid.UUID) (time.Time, time.Time, error) {
	return m.starts[id], m.ends[id], nil
}

// mockAuditWriter implements AuditWriterIface.
type mockAuditWriter struct {
	events []AuditEvent
}

func (m *mockAuditWriter) Write(ctx context.Context, tx *sql.Tx, evt AuditEvent) error {
	m.events = append(m.events, evt)
	return nil
}

// ─── AggregatorService.allocatePair tests ─────────────────────────────────────

func TestAllocatePair_SingleInstrumentFullyCovered(t *testing.T) {
	// AC-LPS-004: instrument below cap → fully covered, excess=0.
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	nasabah := uuid.New()
	bank := uuid.New()
	instr := InstrumenDepositoRow{
		ID:                uuid.New(),
		KodeInstrumen:     "DEP-001",
		Nominal:           decimal.NewFromInt(1_000_000_000),
		MataUang:          "IDR",
		TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	eads := []decimal.Decimal{instr.Nominal}
	overrides := map[uuid.UUID]*LPSExclusionOverride{}

	result := svc.allocatePair(nasabah, bank, []InstrumenDepositoRow{instr}, eads, overrides, capRow)

	if result.JumlahInstrumen != 1 {
		t.Errorf("JumlahInstrumen = %d, want 1", result.JumlahInstrumen)
	}
	if !result.CoveredIDR.Equal(decimal.NewFromInt(1_000_000_000)) {
		t.Errorf("CoveredIDR = %s, want 1000000000", result.CoveredIDR)
	}
	if !result.ExcessIDR.IsZero() {
		t.Errorf("ExcessIDR should be 0, got %s", result.ExcessIDR)
	}
	b := result.Breakdown[0]
	if !b.LPSFullCovered {
		t.Error("LPSFullCovered should be true")
	}
	// INVARIANT check
	if !b.AllocatedToCovered.Add(b.AllocatedToExcess).Equal(b.EAD_IDR) {
		t.Error("INVARIANT broken: covered + excess != EAD")
	}
}

func TestAllocatePair_SingleInstrumentExceedsCapPartially(t *testing.T) {
	// AC-LPS-004: single instrument exceeds cap → partial covered, partial excess.
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	nasabah := uuid.New()
	bank := uuid.New()
	instr := InstrumenDepositoRow{
		ID:            uuid.New(),
		KodeInstrumen: "DEP-001",
		Nominal:       decimal.NewFromInt(2_500_000_000),
		MataUang:      "IDR",
	}
	eads := []decimal.Decimal{instr.Nominal}
	result := svc.allocatePair(nasabah, bank, []InstrumenDepositoRow{instr}, eads, map[uuid.UUID]*LPSExclusionOverride{}, capRow)

	if !result.CoveredIDR.Equal(decimal.NewFromInt(2_000_000_000)) {
		t.Errorf("CoveredIDR = %s, want 2000000000", result.CoveredIDR)
	}
	if !result.ExcessIDR.Equal(decimal.NewFromInt(500_000_000)) {
		t.Errorf("ExcessIDR = %s, want 500000000", result.ExcessIDR)
	}
	if !result.Breakdown[0].AllocatedToCovered.Add(result.Breakdown[0].AllocatedToExcess).Equal(instr.Nominal) {
		t.Error("INVARIANT broken")
	}
}

func TestAllocatePair_FIFOOrder(t *testing.T) {
	// AC-LPS-007: FIFO — older instrument gets covered first.
	// Two instruments, cap=2B: older(1.8B) + newer(1.0B). Older fully covered, newer partial.
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	nasabah := uuid.New()
	bank := uuid.New()
	older := InstrumenDepositoRow{ID: uuid.New(), KodeInstrumen: "DEP-001",
		Nominal: decimal.NewFromInt(1_800_000_000), MataUang: "IDR",
		TanggalPenempatan: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	newer := InstrumenDepositoRow{ID: uuid.New(), KodeInstrumen: "DEP-002",
		Nominal: decimal.NewFromInt(1_000_000_000), MataUang: "IDR",
		TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	eads := []decimal.Decimal{older.Nominal, newer.Nominal}

	result := svc.allocatePair(nasabah, bank, []InstrumenDepositoRow{older, newer}, eads, map[uuid.UUID]*LPSExclusionOverride{}, capRow)

	// Older: fully covered (1.8B <= 2B)
	if !result.Breakdown[0].AllocatedToCovered.Equal(decimal.NewFromInt(1_800_000_000)) {
		t.Errorf("older covered = %s, want 1800000000", result.Breakdown[0].AllocatedToCovered)
	}
	if !result.Breakdown[0].AllocatedToExcess.IsZero() {
		t.Errorf("older excess should be 0, got %s", result.Breakdown[0].AllocatedToExcess)
	}
	// Newer: remaining cap = 200M. covered=200M, excess=800M.
	if !result.Breakdown[1].AllocatedToCovered.Equal(decimal.NewFromInt(200_000_000)) {
		t.Errorf("newer covered = %s, want 200000000", result.Breakdown[1].AllocatedToCovered)
	}
	if !result.Breakdown[1].AllocatedToExcess.Equal(decimal.NewFromInt(800_000_000)) {
		t.Errorf("newer excess = %s, want 800000000", result.Breakdown[1].AllocatedToExcess)
	}
	// INVARIANT
	for _, b := range result.Breakdown {
		if !b.AllocatedToCovered.Add(b.AllocatedToExcess).Equal(b.EAD_IDR) {
			t.Errorf("INVARIANT broken for %s", b.KodeInstrumen)
		}
	}
}

func TestAllocatePair_ExclusionOverride(t *testing.T) {
	// AC-LPS-010: excluded instrument → full EAD goes to excess regardless of cap.
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	nasabah := uuid.New()
	bank := uuid.New()
	instr := InstrumenDepositoRow{
		ID:            uuid.New(),
		KodeInstrumen: "DEP-X",
		Nominal:       decimal.NewFromInt(500_000_000),
		MataUang:      "IDR",
	}
	eads := []decimal.Decimal{instr.Nominal}
	overrideID := uuid.New()
	overrides := map[uuid.UUID]*LPSExclusionOverride{
		instr.ID: {
			ID:              overrideID,
			InstrumenID:     instr.ID,
			ExclusionReason: "Bilateral interbank per LEG-2026-042",
			WorkflowStatus:  WorkflowStatusApprovedActive,
		},
	}

	result := svc.allocatePair(nasabah, bank, []InstrumenDepositoRow{instr}, eads, overrides, capRow)

	b := result.Breakdown[0]
	if !b.LPSExcluded {
		t.Error("LPSExcluded should be true")
	}
	if !b.AllocatedToCovered.IsZero() {
		t.Errorf("covered should be 0 for excluded, got %s", b.AllocatedToCovered)
	}
	if !b.AllocatedToExcess.Equal(instr.Nominal) {
		t.Errorf("excess should equal EAD for excluded, got %s", b.AllocatedToExcess)
	}
	if b.ExclusionReason != "Bilateral interbank per LEG-2026-042" {
		t.Errorf("ExclusionReason = %q", b.ExclusionReason)
	}
	if result.JumlahExcluded != 1 {
		t.Errorf("JumlahExcluded = %d, want 1", result.JumlahExcluded)
	}
}

func TestAllocatePair_CapExhaustedAfterFirst(t *testing.T) {
	// Second instrument gets no coverage (cap exhausted by first).
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	nasabah := uuid.New()
	bank := uuid.New()
	first := InstrumenDepositoRow{ID: uuid.New(), Nominal: decimal.NewFromInt(2_000_000_000), MataUang: "IDR",
		TanggalPenempatan: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	second := InstrumenDepositoRow{ID: uuid.New(), Nominal: decimal.NewFromInt(500_000_000), MataUang: "IDR",
		TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	eads := []decimal.Decimal{first.Nominal, second.Nominal}

	result := svc.allocatePair(nasabah, bank, []InstrumenDepositoRow{first, second}, eads, map[uuid.UUID]*LPSExclusionOverride{}, capRow)

	if !result.Breakdown[1].AllocatedToCovered.IsZero() {
		t.Errorf("second instrument covered should be 0, got %s", result.Breakdown[1].AllocatedToCovered)
	}
	if !result.Breakdown[1].AllocatedToExcess.Equal(second.Nominal) {
		t.Errorf("second instrument excess should equal its nominal, got %s", result.Breakdown[1].AllocatedToExcess)
	}
}

func TestAllocatePair_ZeroInstruments(t *testing.T) {
	svc := &AggregatorService{}
	capRow := &LPSCoverageRow{
		ID:                uuid.New(),
		CoverageAmountIDR: decimal.NewFromInt(2_000_000_000),
	}
	nasabah := uuid.New()
	bank := uuid.New()
	result := svc.allocatePair(nasabah, bank, nil, nil, map[uuid.UUID]*LPSExclusionOverride{}, capRow)
	if result.JumlahInstrumen != 0 {
		t.Errorf("JumlahInstrumen = %d, want 0", result.JumlahInstrumen)
	}
	if !result.TotalExposureIDR.IsZero() {
		t.Errorf("TotalExposureIDR should be 0")
	}
}

// ─── AggregatorService.Aggregate tests ───────────────────────────────────────

func TestAggregate_NoCoverageParam(t *testing.T) {
	svc := NewAggregatorService(
		&mockCoverageRepo{row: nil},
		&mockDepositoRepo{},
		&mockOverrideRepo{},
		&mockKursRepo{},
	)
	_, err := svc.Aggregate(context.Background(), uuid.New(), uuid.New(), time.Now())
	if err == nil {
		t.Fatal("expected error for missing coverage param")
	}
	if de, ok := err.(interface {
		Code() interface{ String() string }
	}); ok {
		if de.Code().String() != CodeLPSCoverageNoActiveParam {
			t.Errorf("expected %s, got %s", CodeLPSCoverageNoActiveParam, de.Code().String())
		}
	}
}

func TestAggregate_FCYConversion(t *testing.T) {
	// Instrument USD 100_000 × kurs 15_500.0000 → IDR 1_550_000_000.0000
	nasabah := uuid.New()
	bank := uuid.New()
	instrID := uuid.New()
	cap := decimal.NewFromInt(2_000_000_000)

	svc := NewAggregatorService(
		&mockCoverageRepo{row: &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: cap}},
		&mockDepositoRepo{byNasabahBank: []InstrumenDepositoRow{
			{ID: instrID, KodeInstrumen: "DEP-USD", Nominal: decimal.NewFromInt(100_000), MataUang: "USD",
				TanggalPenempatan: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
		}},
		&mockOverrideRepo{},
		&mockKursRepo{rates: map[string]decimal.Decimal{"USD": decimal.New(155_000, -1)}},
	)

	result, err := svc.Aggregate(context.Background(), nasabah, bank, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// EAD_IDR = 100000 × 15500 = 1_550_000_000 (truncated to 4dp)
	expectedIDR, _ := decimal.NewFromString("1550000000.0000")
	if !result.Breakdown[0].EAD_IDR.Equal(expectedIDR) {
		t.Errorf("EAD_IDR = %s, want %s", result.Breakdown[0].EAD_IDR, expectedIDR)
	}
	if !result.Breakdown[0].LPSFullCovered {
		t.Error("should be fully covered (1.55B < 2B cap)")
	}
}

func TestAggregate_FCYMissingKurs(t *testing.T) {
	// Missing kurs → ErrFXRateNotFound (using EADFXRateMissing code from M2).
	svc := NewAggregatorService(
		&mockCoverageRepo{row: &LPSCoverageRow{ID: uuid.New(), CoverageAmountIDR: decimal.NewFromInt(2_000_000_000)}},
		&mockDepositoRepo{byNasabahBank: []InstrumenDepositoRow{
			{ID: uuid.New(), Nominal: decimal.NewFromInt(100), MataUang: "USD"},
		}},
		&mockOverrideRepo{},
		&mockKursRepo{err: errors.New("not found")},
	)
	_, err := svc.Aggregate(context.Background(), uuid.New(), uuid.New(), time.Now())
	if err == nil {
		t.Fatal("expected error for missing FX rate")
	}
}

// ─── OverrideService tests ────────────────────────────────────────────────────

func newTestOverrideService(ovRepo OverrideRepoIface, periodeRepo PeriodeBukuRepoIface) *OverrideService {
	// Use nil db — Submit/Approve/Reject use the tx param passed by repo.
	// For unit tests, we skip DB transaction by using mockRepo which ignores tx.
	return &OverrideService{
		db:           nil,
		overrideRepo: ovRepo,
		periodeRepo:  periodeRepo,
		auditWriter:  &mockAuditWriter{},
	}
}

func TestSubmitOverride_ReasonTooShort(t *testing.T) {
	svc := newTestOverrideService(&mockOverrideRepo{}, nil)
	req := SubmitOverrideRequest{
		InstrumenID:        uuid.New(),
		ExclusionReason:    "too short",
		ValidFromPeriodeID: uuid.New(),
		ValidToPeriodeID:   uuid.New(),
	}
	_, err := svc.Submit(context.Background(), req, uuid.New(), "ROLE-RISK", "TUGURE")
	if err == nil {
		t.Fatal("expected error for short reason")
	}
	if de, ok := err.(interface {
		Code() interface{ String() string }
	}); ok {
		if de.Code().String() != CodeLPSOverrideReasonTooShort {
			t.Errorf("expected %s, got %s", CodeLPSOverrideReasonTooShort, de.Code().String())
		}
	}
}

func TestSubmitOverride_PeriodeInvalid(t *testing.T) {
	// fromStart > toEnd → ErrLPSOverridePeriodeInvalid.
	fromID := uuid.New()
	toID := uuid.New()
	periodeRepo := &mockPeriodeRepo{
		starts: map[uuid.UUID]time.Time{fromID: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		ends:   map[uuid.UUID]time.Time{toID: time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)},
	}
	svc := newTestOverrideService(&mockOverrideRepo{}, periodeRepo)
	req := SubmitOverrideRequest{
		InstrumenID:        uuid.New(),
		ExclusionReason:    "This reason is definitely longer than 30 characters, valid",
		ValidFromPeriodeID: fromID,
		ValidToPeriodeID:   toID,
	}
	_, err := svc.Submit(context.Background(), req, uuid.New(), "ROLE-RISK", "TUGURE")
	if err == nil {
		t.Fatal("expected error for invalid periode order")
	}
}

func TestSubmitOverride_DuplicateActive(t *testing.T) {
	// Duplicate active override → LPS_OVERRIDE_INVALID_TRANSITION.
	fromID := uuid.New()
	toID := uuid.New()
	periodeRepo := &mockPeriodeRepo{
		starts: map[uuid.UUID]time.Time{fromID: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		ends:   map[uuid.UUID]time.Time{toID: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
	}
	ovRepo := &mockOverrideRepo{hasActivePend: true, conflictID: "existing-id"}
	svc := newTestOverrideService(ovRepo, periodeRepo)
	req := SubmitOverrideRequest{
		InstrumenID:        uuid.New(),
		ExclusionReason:    "This reason is definitely longer than 30 characters, valid",
		ValidFromPeriodeID: fromID,
		ValidToPeriodeID:   toID,
	}
	_, err := svc.Submit(context.Background(), req, uuid.New(), "ROLE-RISK", "TUGURE")
	if err == nil {
		t.Fatal("expected error for duplicate active override")
	}
}

func TestApproveOverride_SoDViolation(t *testing.T) {
	makerID := uuid.New()
	overrideID := uuid.New()
	ovRepo := &mockOverrideRepo{
		overrides: map[uuid.UUID]*LPSExclusionOverride{
			overrideID: {
				ID:             overrideID,
				MakerID:        makerID,
				WorkflowStatus: WorkflowStatusPendingApproval,
			},
		},
	}
	svc := &OverrideService{overrideRepo: ovRepo, auditWriter: &mockAuditWriter{}}
	// Approver == maker → SoD violation.
	_, err := svc.ApproveOverride(context.Background(), overrideID, makerID, "ROLE-ALCO", "approved", "TUGURE")
	if err == nil {
		t.Fatal("expected SoD violation error")
	}
	if de, ok := err.(interface {
		Code() interface{ String() string }
	}); ok {
		if de.Code().String() != CodeLPSOverrideSoDViolation {
			t.Errorf("expected %s, got %s", CodeLPSOverrideSoDViolation, de.Code().String())
		}
	}
}

func TestApproveOverride_AlreadyRejected(t *testing.T) {
	overrideID := uuid.New()
	ovRepo := &mockOverrideRepo{
		overrides: map[uuid.UUID]*LPSExclusionOverride{
			overrideID: {
				ID:             overrideID,
				MakerID:        uuid.New(),
				WorkflowStatus: WorkflowStatusRejected, // terminal
			},
		},
	}
	svc := &OverrideService{overrideRepo: ovRepo, auditWriter: &mockAuditWriter{}}
	_, err := svc.ApproveOverride(context.Background(), overrideID, uuid.New(), "ROLE-ALCO", "", "TUGURE")
	if err == nil {
		t.Fatal("expected error for transition from REJECTED")
	}
}

func TestRejectOverride_NotPending(t *testing.T) {
	overrideID := uuid.New()
	ovRepo := &mockOverrideRepo{
		overrides: map[uuid.UUID]*LPSExclusionOverride{
			overrideID: {
				ID:             overrideID,
				MakerID:        uuid.New(),
				WorkflowStatus: WorkflowStatusApprovedActive, // already approved
			},
		},
	}
	svc := &OverrideService{overrideRepo: ovRepo, auditWriter: &mockAuditWriter{}}
	_, err := svc.RejectOverride(context.Background(), overrideID, uuid.New(), "ROLE-ALCO", "rejected", "TUGURE")
	if err == nil {
		t.Fatal("expected error for reject from APPROVED_ACTIVE")
	}
}

func TestRejectOverride_NotFound(t *testing.T) {
	ovRepo := &mockOverrideRepo{overrides: map[uuid.UUID]*LPSExclusionOverride{}}
	svc := &OverrideService{overrideRepo: ovRepo, auditWriter: &mockAuditWriter{}}
	_, err := svc.RejectOverride(context.Background(), uuid.New(), uuid.New(), "ROLE-ALCO", "rejected", "TUGURE")
	if err == nil {
		t.Fatal("expected error for not found override")
	}
}

// ─── toIDR tests ──────────────────────────────────────────────────────────────

func TestToIDR_IDR(t *testing.T) {
	svc := &AggregatorService{kursRepo: &mockKursRepo{}}
	amount := decimal.NewFromInt(1_000_000)
	got, err := svc.toIDR(context.Background(), amount, "IDR", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Equal(amount) {
		t.Errorf("IDR passthrough: got %s, want %s", got, amount)
	}
}

func TestToIDR_FCY_Precision(t *testing.T) {
	// USD 100000 × 15432.12345678 = 1543212345.678 → truncated to 4dp = 1543212345.6780
	rate, _ := decimal.NewFromString("15432.12345678")
	svc := &AggregatorService{kursRepo: &mockKursRepo{rates: map[string]decimal.Decimal{"USD": rate}}}
	amount := decimal.NewFromInt(100_000)
	got, err := svc.toIDR(context.Background(), amount, "USD", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := decimal.NewFromString("1543212345.6780")
	if !got.Equal(expected) {
		t.Errorf("FCY conversion: got %s, want %s", got, expected)
	}
}

// ─── AggregateBulkTooLarge ────────────────────────────────────────────────────

func TestAggregateBulk_TooLarge(t *testing.T) {
	// Build 50001 bulk rows.
	rows := make([]BulkDepositoRow, maxBulkInstruments+1)
	nasabah := uuid.New()
	bank := uuid.New()
	capID := uuid.New()
	cap := decimal.NewFromInt(2_000_000_000)
	for i := range rows {
		rows[i] = BulkDepositoRow{
			InstrumenID:        uuid.New(),
			NasabahID:          nasabah,
			BankID:             bank,
			Nominal:            decimal.NewFromInt(1_000),
			MataUang:           "IDR",
			LPSCoverageParamID: capID,
			LPSCapIDR:          cap,
		}
	}
	svc := NewAggregatorService(
		&mockCoverageRepo{row: &LPSCoverageRow{ID: capID, CoverageAmountIDR: cap}},
		&mockDepositoRepo{bulkRows: rows},
		&mockOverrideRepo{},
		&mockKursRepo{},
	)
	_, err := svc.AggregateBulk(context.Background(), time.Now())
	if err == nil {
		t.Fatal("expected bulk too large error")
	}
}
