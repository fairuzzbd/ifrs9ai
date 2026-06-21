package pocidelta

// service_extended_test.go — Extended service-level coverage for uncovered branches.
// Covers: CaptureBaseline happy paths, ListBaseline, ListDeltaLog,
// ComputeDeltaForCalcRun error branches, domain helpers, stub accessors.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── CaptureBaseline happy path ───────────────────────────────────────────────

func TestCaptureBaseline_HappyPath_TanggalBaselineDefault(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{
			ID:     instrID,
			IsPoci: true,
			Status: "ACTIVE",
		},
		baseline: nil,
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              instrID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
		TanggalBaseline:         nil,
	}
	b, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b.InstrumenID != instrID {
		t.Fatal("instrumen_id mismatch")
	}
	want := decimal.NewFromFloat(1250000000).RoundBank(4)
	if !b.LifetimeECLAtOrigination.Equal(want) {
		t.Fatalf("ECL rounding: got %s, want %s", b.LifetimeECLAtOrigination, want)
	}
}

func TestCaptureBaseline_HappyPath_TanggalBaselineExplicit(t *testing.T) {
	instrID := uuid.New()
	dateStr := "2026-01-15"
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{ID: instrID, IsPoci: true, Status: "ACTIVE"},
		baseline:      nil,
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              instrID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(500000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.063),
		TanggalBaseline:         &dateStr,
	}
	b, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.TanggalBaseline.Format("2006-01-02") != dateStr {
		t.Fatalf("tanggal_baseline: got %s, want %s", b.TanggalBaseline.Format("2006-01-02"), dateStr)
	}
}

func TestCaptureBaseline_BadTanggalBaselineFallsBack(t *testing.T) {
	instrID := uuid.New()
	badDate := "not-a-date"
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{ID: instrID, IsPoci: true, Status: "ACTIVE"},
		baseline:      nil,
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              instrID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(100000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.05),
		TanggalBaseline:         &badDate,
	}
	b, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
}

// ─── ListBaseline ─────────────────────────────────────────────────────────────

func TestListBaseline_Empty(t *testing.T) {
	repo := &stubRepo{baseline: nil}
	svc := makeService(repo)
	rows, pag, err := svc.ListBaseline(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
	_ = pag
}

func TestListBaseline_WithData(t *testing.T) {
	repo := &stubRepo{baseline: &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
	}}
	svc := makeService(repo)
	rows, _, err := svc.ListBaseline(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

// ─── ListDeltaLog ─────────────────────────────────────────────────────────────

func TestListDeltaLog_Empty(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	rows, _, err := svc.ListDeltaLog(context.Background(), listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

// ─── GetDeltaSummary with portofolioID ────────────────────────────────────────

func TestGetDeltaSummary_WithPortofolioID(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	pid := uuid.New()
	sum, err := svc.GetDeltaSummary(context.Background(), &pid, 2026, 6, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum == nil {
		t.Fatal("expected non-nil summary")
	}
}

// ─── ComputeDeltaForCalcRun — txBegin error path ─────────────────────────────

func TestComputeDeltaForCalcRun_TxBeginFails_CollectedAsError(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		calcRunStatus: "SEALED",
		pociList: []InstrumenPociInfo{
			{ID: instrID, KodeInstrumen: "INSTR-001", IsPoci: true, Status: "ACTIVE"},
		},
		baseline: &Baseline{
			ID:                       uuid.New(),
			InstrumenID:              instrID,
			LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		},
		currentECL:      decimal.NewFromFloat(1200000),
		cumulativeDelta: decimal.Zero,
		deltaLog:        nil,
	}
	svc := makeService(repo)
	errs, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("expected nil global error, got: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected at least 1 per-instrument error from txBegin failure")
	}
	if errs[0].ErrorCode != "INTERNAL" {
		t.Fatalf("expected INTERNAL error code, got %s", errs[0].ErrorCode)
	}
}

func TestComputeDeltaForCalcRun_DeltaDuplicate(t *testing.T) {
	instrID := uuid.New()
	existingLog := &DeltaLog{
		ID:          uuid.New(),
		InstrumenID: instrID,
		Status:      StatusComputed,
	}
	repo := &stubRepo{
		calcRunStatus: "SEALED",
		pociList: []InstrumenPociInfo{
			{ID: instrID, KodeInstrumen: "INSTR-001", IsPoci: true, Status: "ACTIVE"},
		},
		deltaLog: existingLog,
	}
	svc := makeService(repo)
	errs, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected global error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected POCI_DELTA_DUPLICATE error")
	}
	if errs[0].ErrorCode != CodePociDeltaDuplicate {
		t.Fatalf("expected %s, got %s", CodePociDeltaDuplicate, errs[0].ErrorCode)
	}
}

func TestComputeDeltaForCalcRun_BaselineMissing(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		calcRunStatus: "COMPLETED",
		pociList: []InstrumenPociInfo{
			{ID: instrID, KodeInstrumen: "INSTR-001", IsPoci: true, Status: "ACTIVE"},
		},
		baseline: nil,
		deltaLog: nil,
	}
	svc := makeService(repo)
	errs, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected global error: %v", err)
	}
	if len(errs) == 0 {
		t.Fatal("expected POCI_BASELINE_MISSING error")
	}
	if errs[0].ErrorCode != CodePociBaselineMissing {
		t.Fatalf("expected %s, got %s", CodePociBaselineMissing, errs[0].ErrorCode)
	}
}

func TestComputeDeltaForCalcRun_EmptyPociList(t *testing.T) {
	repo := &stubRepo{
		calcRunStatus: "SEALED",
		pociList:      []InstrumenPociInfo{},
	}
	svc := makeService(repo)
	errs, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors for empty POCI list, got %d", len(errs))
	}
}

func TestComputeDeltaForCalcRun_GetCalcRunStatusError(t *testing.T) {
	repo := &stubRepoGetCalcRunErr{}
	svc := makeService(repo)
	_, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error when GetCalcRunStatus fails")
	}
}

func TestComputeDeltaForCalcRun_ListPociInstrumenError(t *testing.T) {
	repo := &stubRepoListPociErr{calcRunStatus: "SEALED"}
	svc := makeService(repo)
	_, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error when ListPociInstrumenByCalcRun fails")
	}
}

// ─── Specialized stubs for error injection ────────────────────────────────────

type stubRepoGetCalcRunErr struct {
	stubRepo
}

func (r *stubRepoGetCalcRunErr) GetCalcRunStatus(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return "", errors.New("db error getting calc run status")
}

type stubRepoListPociErr struct {
	stubRepo
	calcRunStatus string
}

func (r *stubRepoListPociErr) GetCalcRunStatus(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return r.calcRunStatus, nil
}

func (r *stubRepoListPociErr) ListPociInstrumenByCalcRun(_ context.Context, _ uuid.UUID, _ string) ([]InstrumenPociInfo, error) {
	return nil, errors.New("db error listing poci instruments")
}

// ─── writeAuditInTx — nil audit writer ───────────────────────────────────────

func TestWriteAuditInTx_NilAuditWriter(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	// Should not panic — nil s.audit is handled gracefully
	svc.writeAuditInTx(context.Background(), nil, audit.Event{
		Action:     "POCI.TEST",
		EntityType: "test",
		EntityID:   uuid.New(),
	})
}

// ─── JurnalPosterStub helpers ─────────────────────────────────────────────────

func TestJurnalPosterStub_SetPostError(t *testing.T) {
	stub := NewJurnalPosterStub(slog.Default())
	stub.SetPostError(errors.New("jurnal error"))
	_, err := stub.PostPociDelta(context.Background(), nil, PociDeltaPostRequest{
		EventCode: "POCI_ECL_DELTA_INCREASE",
		Direction: DirectionIncrease,
	})
	if err == nil {
		t.Fatal("expected error from SetPostError")
	}
}

func TestJurnalPosterStub_Calls(t *testing.T) {
	stub := NewJurnalPosterStub(slog.Default())
	req := PociDeltaPostRequest{
		EventCode:   "POCI_ECL_DELTA_INCREASE",
		InstrumenID: uuid.New(),
		Direction:   DirectionIncrease,
		AmountIDR:   decimal.NewFromFloat(200000),
	}
	stub.PostPociDelta(context.Background(), nil, req) //nolint:errcheck
	calls := stub.Calls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].EventCode != "POCI_ECL_DELTA_INCREASE" {
		t.Fatalf("unexpected event code: %s", calls[0].EventCode)
	}
}

func TestJurnalPosterStub_Reset(t *testing.T) {
	stub := NewJurnalPosterStub(slog.Default())
	stub.PostPociDelta(context.Background(), nil, PociDeltaPostRequest{}) //nolint:errcheck
	stub.Reset()
	calls := stub.Calls()
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls after Reset, got %d", len(calls))
	}
}

func TestJurnalPosterStub_PostPociDelta_Success(t *testing.T) {
	stub := NewJurnalPosterStub(slog.Default())
	result, err := stub.PostPociDelta(context.Background(), nil, PociDeltaPostRequest{
		EventCode:   "POCI_ECL_DELTA_DECREASE",
		InstrumenID: uuid.New(),
		Direction:   DirectionDecrease,
		AmountIDR:   decimal.NewFromFloat(100000),
		TenantID:    "TUGURE",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.JurnalHeaderID == uuid.Nil {
		t.Fatal("expected non-nil JurnalHeaderID")
	}
}

// ─── NoopJurnalPoster ─────────────────────────────────────────────────────────

func TestNoopJurnalPoster_PostPociDelta(t *testing.T) {
	noop := NewNoopJurnalPoster(slog.Default())
	result, err := noop.PostPociDelta(context.Background(), nil, PociDeltaPostRequest{
		EventCode: "POCI_ECL_DELTA_INCREASE",
		Direction: DirectionIncrease,
		AmountIDR: decimal.NewFromFloat(500000),
	})
	if err != nil {
		t.Fatalf("expected no error from noop poster, got: %v", err)
	}
	if result.JurnalHeaderID == uuid.Nil {
		t.Fatal("expected non-nil JurnalHeaderID from noop")
	}
}

// ─── Domain helpers ───────────────────────────────────────────────────────────

func TestDirectionValid(t *testing.T) {
	cases := []struct {
		d     Direction
		valid bool
	}{
		{DirectionIncrease, true},
		{DirectionDecrease, true},
		{DirectionZero, true},
		{Direction("INVALID"), false},
		{Direction(""), false},
	}
	for _, c := range cases {
		got := c.d.Valid()
		if got != c.valid {
			t.Errorf("Direction(%q).Valid() = %v, want %v", c.d, got, c.valid)
		}
	}
}

func TestToDeltaLogItem_WithNilOptionals(t *testing.T) {
	d := &DeltaLog{
		ID:             uuid.New(),
		CalcRunID:      uuid.New(),
		InstrumenID:    uuid.New(),
		TanggalCompute: time.Now(),
		BaselineECL:    decimal.NewFromFloat(1000000),
		CurrentECL:     decimal.NewFromFloat(1200000),
		DeltaECL:       decimal.NewFromFloat(200000),
		Direction:      DirectionIncrease,
		Status:         StatusComputed,
		CreatedAt:      time.Now(),
	}
	threshold := decimal.NewFromFloat(500000000)
	item := ToDeltaLogItem(d, "INSTR-001", threshold)
	if item.PriorDeltaCumulative != nil {
		t.Fatal("expected nil PriorDeltaCumulative")
	}
	if item.JurnalHeaderId != nil {
		t.Fatal("expected nil JurnalHeaderId")
	}
	if item.LargeDeltaFlag {
		t.Fatal("expected LargeDeltaFlag=false for delta < threshold")
	}
	if item.InstrumenKode != "INSTR-001" {
		t.Fatalf("unexpected InstrumenKode: %s", item.InstrumenKode)
	}
}

func TestToDeltaLogItem_LargeDeltaFlag(t *testing.T) {
	d := &DeltaLog{
		ID:          uuid.New(),
		CalcRunID:   uuid.New(),
		InstrumenID: uuid.New(),
		DeltaECL:    decimal.NewFromFloat(600000000),
		Direction:   DirectionIncrease,
		Status:      StatusPosted,
		CreatedAt:   time.Now(),
	}
	threshold := decimal.NewFromFloat(500000000)
	item := ToDeltaLogItem(d, "INSTR-002", threshold)
	if !item.LargeDeltaFlag {
		t.Fatal("expected LargeDeltaFlag=true for delta > threshold")
	}
}

func TestToDeltaLogItem_WithPriorAndJurnalHeader(t *testing.T) {
	prior := decimal.NewFromFloat(100000)
	jurnalID := uuid.New()
	d := &DeltaLog{
		ID:                   uuid.New(),
		CalcRunID:            uuid.New(),
		InstrumenID:          uuid.New(),
		TanggalCompute:      time.Now(),
		BaselineECL:         decimal.NewFromFloat(1000000),
		CurrentECL:          decimal.NewFromFloat(850000),
		DeltaECL:            decimal.NewFromFloat(-150000),
		Direction:           DirectionDecrease,
		PriorDeltaCumulative: &prior,
		JurnalHeaderID:      &jurnalID,
		Status:              StatusPosted,
		CreatedAt:           time.Now(),
	}
	threshold := decimal.NewFromFloat(500000000)
	item := ToDeltaLogItem(d, "INSTR-003", threshold)
	if item.PriorDeltaCumulative == nil {
		t.Fatal("expected non-nil PriorDeltaCumulative")
	}
	if item.JurnalHeaderId == nil {
		t.Fatal("expected non-nil JurnalHeaderId")
	}
}

func TestIsCodeErr_NilError(t *testing.T) {
	if IsCodeErr(nil, CodePociBaselineMissing) {
		t.Fatal("expected false for nil error")
	}
}

func TestIsCodeErr_ShortError(t *testing.T) {
	if IsCodeErr(errors.New("X"), CodePociBaselineMissing) {
		t.Fatal("expected false for short error string")
	}
}

func TestToBaselineListItem(t *testing.T) {
	b := &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              uuid.New(),
		TanggalBaseline:         time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.04567890),
		CreatedAt:               time.Now(),
	}
	item := ToBaselineListItem(b, "INSTR-TEST")
	if item.InstrumenKode != "INSTR-TEST" {
		t.Fatalf("unexpected kode: %s", item.InstrumenKode)
	}
	if item.TanggalBaseline != "2026-01-15" {
		t.Fatalf("unexpected date: %s", item.TanggalBaseline)
	}
	if item.LifetimeEclAtOrigination != "1250000000.0000" {
		t.Fatalf("unexpected ecl: %s", item.LifetimeEclAtOrigination)
	}
}
