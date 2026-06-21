package pocidelta

// service_test.go — Service-level tests (happy + error paths per AC).
// Uses in-memory stubs for repo + jurnal_poster; no DB required.

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Stub repo ───────────────────────────────────────────────────────────────

type stubRepo struct {
	baseline            *Baseline
	deltaLog            *DeltaLog
	instrumenInfo       *InstrumenPociInfo
	pociList            []InstrumenPociInfo
	periodeStatus       string
	calcRunStatus       string
	currentECL          decimal.Decimal
	threshold           decimal.Decimal
	cumulativeDelta     decimal.Decimal
	periodeBulananID    uuid.UUID // B2: periode_bulanan_id returned by GetPeriodeBulananIDForCalcRun
}

func (r *stubRepo) InsertBaseline(_ context.Context, _ *sql.Tx, b *Baseline) error { return nil }
func (r *stubRepo) GetBaselineByInstrumen(_ context.Context, _ uuid.UUID, _ string) (*Baseline, error) {
	return r.baseline, nil
}
func (r *stubRepo) ListBaselines(_ context.Context, _ listquery.Query, _ string) ([]Baseline, Pagination, error) {
	if r.baseline != nil {
		return []Baseline{*r.baseline}, Pagination{}, nil
	}
	return nil, Pagination{}, nil
}
func (r *stubRepo) InsertDeltaLog(_ context.Context, _ *sql.Tx, _ *DeltaLog) error { return nil }
func (r *stubRepo) GetDeltaLogByRunAndInstrumen(_ context.Context, _, _ uuid.UUID, _ string) (*DeltaLog, error) {
	return r.deltaLog, nil
}
func (r *stubRepo) UpdateDeltaLogStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ time.Time, _ DeltaStatus, _ *uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (r *stubRepo) ListDeltaLogs(_ context.Context, _ listquery.Query, _ string) ([]DeltaLog, Pagination, error) {
	return nil, Pagination{}, nil
}
func (r *stubRepo) GetDeltaHistoryByInstrumen(_ context.Context, _ uuid.UUID, _ listquery.Query, _ string) ([]DeltaLog, Pagination, error) {
	if r.deltaLog != nil {
		return []DeltaLog{*r.deltaLog}, Pagination{}, nil
	}
	return nil, Pagination{}, nil
}
func (r *stubRepo) GetCumulativeDelta(_ context.Context, _ uuid.UUID, _ time.Time, _ string) (decimal.Decimal, error) {
	return r.cumulativeDelta, nil
}
func (r *stubRepo) GetDeltaSummary(_ context.Context, _ *uuid.UUID, _, _ int, _ string) (*DeltaSummary, error) {
	return &DeltaSummary{Year: 2026, Month: 6}, nil
}
func (r *stubRepo) GetInstrumenPociInfo(_ context.Context, _ uuid.UUID, _ string) (*InstrumenPociInfo, error) {
	return r.instrumenInfo, nil
}
func (r *stubRepo) ListPociInstrumenByCalcRun(_ context.Context, _ uuid.UUID, _ string) ([]InstrumenPociInfo, error) {
	return r.pociList, nil
}
func (r *stubRepo) GetPeriodeStatus(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return r.periodeStatus, nil
}
func (r *stubRepo) GetPeriodeBulananIDForCalcRun(_ context.Context, _ uuid.UUID, _ string) (uuid.UUID, error) {
	return r.periodeBulananID, nil
}
func (r *stubRepo) GetCalcRunStatus(_ context.Context, _ uuid.UUID, _ string) (string, error) {
	return r.calcRunStatus, nil
}
func (r *stubRepo) GetCurrentECLForPociInstrumen(_ context.Context, _, _ uuid.UUID, _ string) (decimal.Decimal, error) {
	return r.currentECL, nil
}
func (r *stubRepo) GetLargeDeltaThreshold(_ context.Context, _ string) (decimal.Decimal, error) {
	if r.threshold.IsZero() {
		return decimal.NewFromFloat(500000000), nil
	}
	return r.threshold, nil
}

// stubRepo also needs to satisfy the Repository interface for tx methods.
// The service.txBegin is a stub that returns error; direct DB tests use integration suite.
// For unit tests we test service logic through the exported non-tx methods.

func makeService(repo Repository) *Service {
	poster := NewJurnalPosterStub(slog.Default())
	// Pass nil audit.Writer — service should handle nil gracefully (NOOP audit in tests)
	return NewService(repo, poster, nil, slog.Default())
}

// ─── CaptureBaseline tests ───────────────────────────────────────────────────

func TestCaptureBaseline_InstrumenNotPoci(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{
			ID:    uuid.New(),
			IsPoci: false,
		},
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected POCI_INSTRUMEN_NOT_POCI error")
	}
	if !IsCodeErr(err, CodePociInstrumenNotPoci) {
		t.Fatalf("wrong code: %v", err)
	}
}

func TestCaptureBaseline_BaselineAlreadyExists(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		instrumenInfo: &InstrumenPociInfo{
			ID:    instrID,
			IsPoci: true,
			Status: "ACTIVE",
		},
		baseline: &Baseline{
			ID:                       uuid.New(),
			InstrumenID:              instrID,
			TanggalBaseline:         time.Now(),
			LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000),
		},
	}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              instrID,
		LifetimeECLAtOrigination: decimal.NewFromFloat(999999),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected POCI_BASELINE_IMMUTABLE_VIOLATION")
	}
	if !IsCodeErr(err, CodePociBaselineImmutableViolation) {
		t.Fatalf("wrong code: %v", err)
	}
}

func TestCaptureBaseline_InstrumenNotFound(t *testing.T) {
	repo := &stubRepo{instrumenInfo: nil} // nil = not found
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1000000),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error when instrumen not found")
	}
}

func TestCaptureBaseline_InvalidRequest_NegativeECL(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	req := CaptureBaselineRequest{
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(-1),
		CreditAdjustedEIR:       decimal.NewFromFloat(0.045),
	}
	_, err := svc.CaptureBaseline(context.Background(), nil, req, uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected VALIDATION_FAILED for negative ECL")
	}
}

// ─── GetBaselineByInstrumen tests ────────────────────────────────────────────

func TestGetBaselineByInstrumen_NotFound(t *testing.T) {
	repo := &stubRepo{baseline: nil}
	svc := makeService(repo)
	_, err := svc.GetBaselineByInstrumen(context.Background(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected POCI_BASELINE_MISSING")
	}
	if !IsCodeErr(err, CodePociBaselineMissing) {
		t.Fatalf("wrong code: %v", err)
	}
}

func TestGetBaselineByInstrumen_Found(t *testing.T) {
	b := &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              uuid.New(),
		LifetimeECLAtOrigination: decimal.NewFromFloat(1250000000),
	}
	repo := &stubRepo{baseline: b}
	svc := makeService(repo)
	got, err := svc.GetBaselineByInstrumen(context.Background(), b.InstrumenID, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.LifetimeECLAtOrigination.Equal(b.LifetimeECLAtOrigination) {
		t.Fatal("ECL mismatch")
	}
}

// ─── GetDeltaHistory tests ───────────────────────────────────────────────────

func TestGetDeltaHistory_BaselineMissing(t *testing.T) {
	repo := &stubRepo{baseline: nil}
	svc := makeService(repo)
	_, _, err := svc.GetDeltaHistory(context.Background(), uuid.New(), listquery.Query{}, "TUGURE")
	if err == nil {
		t.Fatal("expected POCI_BASELINE_MISSING")
	}
}

func TestGetDeltaHistory_Found(t *testing.T) {
	instrID := uuid.New()
	repo := &stubRepo{
		baseline: &Baseline{InstrumenID: instrID, LifetimeECLAtOrigination: decimal.NewFromFloat(1000000)},
		deltaLog: &DeltaLog{InstrumenID: instrID, DeltaECL: decimal.NewFromFloat(50000), Direction: DirectionIncrease},
	}
	svc := makeService(repo)
	rows, _, err := svc.GetDeltaHistory(context.Background(), instrID, listquery.Query{}, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least 1 row")
	}
}

// ─── GetDeltaSummary test ────────────────────────────────────────────────────

func TestGetDeltaSummary_ReturnsData(t *testing.T) {
	repo := &stubRepo{}
	svc := makeService(repo)
	sum, err := svc.GetDeltaSummary(context.Background(), nil, 2026, 6, "TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sum == nil {
		t.Fatal("expected non-nil summary")
	}
}

// ─── ComputeDeltaForCalcRun — validation paths ───────────────────────────────

func TestComputeDeltaForCalcRun_CalcRunNotSealed(t *testing.T) {
	repo := &stubRepo{calcRunStatus: "RUNNING"}
	svc := makeService(repo)
	_, err := svc.ComputeDeltaForCalcRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	if err == nil {
		t.Fatal("expected error for non-sealed calc run")
	}
}
