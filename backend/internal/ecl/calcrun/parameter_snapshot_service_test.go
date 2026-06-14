package calcrun_test

// parameter_snapshot_service_test.go — Tests for ParameterSnapshotService.
// All sub-snapshot functions tested for:
//   - Success (mock returns data → snapshot populated correctly)
//   - Missing data (ErrNoRows / zero count → CALC_RUN_PARAMETER_SNAPSHOT_INVALID or CALC_RUN_ECL_PARAM_NOT_FOUND)
//   - DB error propagation
//
// DEC-016: No float64 — all numeric values read as text from DB and stored verbatim.

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"blips-ifrs9.tugu-re.com/internal/ecl/calcrun"
)

// ─── NewParameterSnapshotService panic guard ──────────────────────────────────

func TestNewParameterSnapshotService_PanicOnNilDB(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil db")
		}
	}()
	calcrun.NewParameterSnapshotService(nil)
}

// ─── Shared helper ────────────────────────────────────────────────────────────

func newSnapSvc(t *testing.T) (*calcrun.ParameterSnapshotService, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := calcrun.NewParameterSnapshotService(db)
	return svc, mock
}

func evalDate() time.Time {
	return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
}

// ─── SnapshotAll: bobot missing → 422 ────────────────────────────────────────

func TestSnapshotAll_BobotMissing_422(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	// bobot query returns no rows.
	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when bobot missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "CALC_RUN_PARAMETER_SNAPSHOT_INVALID" {
		t.Errorf("code = %q; want CALC_RUN_PARAMETER_SNAPSHOT_INVALID", ce.Code())
	}
	if ce.HTTPStatus() != 422 {
		t.Errorf("http = %d; want 422", ce.HTTPStatus())
	}
}

// ─── SnapshotAll: PD missing → CALC_RUN_ECL_PARAM_NOT_FOUND ─────────────────

func TestSnapshotAll_PDMissing_422(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	// bobot returns data.
	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("param-1", "0.2500", "0.5000", "0.2500", "user-1", "2026-06-01T00:00:00Z"))

	// PD returns zero count.
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(0, "", ""))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when pd missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "ECL_PARAM_NOT_FOUND" {
		t.Errorf("code = %q; want ECL_PARAM_NOT_FOUND", ce.Code())
	}
}

// ─── SnapshotAll: LGD missing → CALC_RUN_ECL_PARAM_NOT_FOUND ─────────────────

func TestSnapshotAll_LGDMissing_422(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))

	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(5, "u1", "2026-06-01"))

	// LGD zero count.
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(0, "", ""))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when lgd missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "ECL_PARAM_NOT_FOUND" {
		t.Errorf("code = %q; want ECL_PARAM_NOT_FOUND", ce.Code())
	}
}

// ─── SnapshotAll: impactPD missing → CALC_RUN_ECL_PARAM_NOT_FOUND ────────────

func TestSnapshotAll_ImpactPDMissing_422(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))

	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(5, "u1", "2026-06-01"))

	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(3, "u1", "2026-06-01"))

	// impact_pd no rows.
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when impact_pd missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "ECL_PARAM_NOT_FOUND" {
		t.Errorf("code = %q; want ECL_PARAM_NOT_FOUND", ce.Code())
	}
}

// ─── SnapshotAll: impactMevPD GOOD missing → CALC_RUN_ECL_PARAM_NOT_FOUND ────

func TestSnapshotAll_ImpactMevPD_GOODMissing(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(5, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(3, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))

	// impact_mev_pd returns only BAD (no GOOD).
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "BAD", "1.20000000", "u1", "2026-06-01"))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when impact_mev_pd GOOD missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "ECL_PARAM_NOT_FOUND" {
		t.Errorf("code = %q; want ECL_PARAM_NOT_FOUND", ce.Code())
	}
}

// ─── SnapshotAll: impactMevPD BAD missing → CALC_RUN_ECL_PARAM_NOT_FOUND ────

func TestSnapshotAll_ImpactMevPD_BADMissing(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(5, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(3, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))

	// impact_mev_pd returns only GOOD (no BAD).
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.80000000", "u1", "2026-06-01"))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when impact_mev_pd BAD missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "ECL_PARAM_NOT_FOUND" {
		t.Errorf("code = %q; want ECL_PARAM_NOT_FOUND", ce.Code())
	}
}

// ─── SnapshotAll: LPS missing → CALC_RUN_PARAMETER_SNAPSHOT_INVALID ──────────

func TestSnapshotAll_LPSMissing_422(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(5, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(3, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.80000000", "u1", "2026-06-01").
			AddRow("imev-2", "BAD", "1.20000000", "u1", "2026-06-01"))

	// LPS no rows.
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when lps missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "CALC_RUN_PARAMETER_SNAPSHOT_INVALID" {
		t.Errorf("code = %q; want CALC_RUN_PARAMETER_SNAPSHOT_INVALID", ce.Code())
	}
}

// ─── SnapshotAll: kurs missing → CALC_RUN_FX_RATE_NOT_FOUND ─────────────────

func TestSnapshotAll_KursMissing(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(5, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(3, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.80000000", "u1", "2026-06-01").
			AddRow("imev-2", "BAD", "1.20000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}).
			AddRow("lps-1", "2000000000.0000", "2020-01-01", nil, "u1"))

	// kurs empty for evalDate.
	mock.ExpectQuery(`SELECT .+ FROM mst.kurs`).
		WillReturnRows(sqlmock.NewRows([]string{"kode_mata_uang", "kurs_tengah", "tanggal"}))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error when kurs missing")
	}
	ce, ok := calcrun.IsCalcRunError(err)
	if !ok {
		t.Fatalf("expected calcRunError; got %T: %v", err, err)
	}
	if ce.Code() != "FX_RATE_NOT_FOUND" {
		t.Errorf("code = %q; want FX_RATE_NOT_FOUND", ce.Code())
	}
}

// ─── SnapshotAll: success path ────────────────────────────────────────────────

func TestSnapshotAll_Success(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "user-alco", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(10, "user-alco", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).
			AddRow(5, "user-alco", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.05000000", "user-alco", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.90000000", "user-alco", "2026-06-01").
			AddRow("imev-2", "NORMAL", "1.00000000", "user-alco", "2026-06-01").
			AddRow("imev-3", "BAD", "1.10000000", "user-alco", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}).
			AddRow("lps-1", "2000000000.0000", "2020-01-01", nil, "user-alco"))
	mock.ExpectQuery(`SELECT .+ FROM mst.kurs`).
		WillReturnRows(sqlmock.NewRows([]string{"kode_mata_uang", "kurs_tengah", "tanggal"}).
			AddRow("USD", "15800.00000000", "2026-06-13").
			AddRow("EUR", "17200.00000000", "2026-06-13"))

	raw, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil raw JSON")
	}

	// Verify JSON structure.
	var snap calcrun.ParameterSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.PeriodeID != "p-2026-06" {
		t.Errorf("periodeId = %q; want p-2026-06", snap.PeriodeID)
	}
	if snap.EvalDate != "2026-06-13" {
		t.Errorf("evalDate = %q; want 2026-06-13", snap.EvalDate)
	}
	if snap.BobotSkenario == nil {
		t.Error("expected BobotSkenario non-nil")
	} else {
		if snap.BobotSkenario.BobotGood != "0.2500" {
			t.Errorf("bobotGood = %q; want 0.2500", snap.BobotSkenario.BobotGood)
		}
		// DEC-016: values must be string, not float.
		if snap.BobotSkenario.BobotNormal != "0.5000" {
			t.Errorf("bobotNormal = %q; want 0.5000", snap.BobotSkenario.BobotNormal)
		}
		if snap.BobotSkenario.BobotBad != "0.2500" {
			t.Errorf("bobotBad = %q; want 0.2500", snap.BobotSkenario.BobotBad)
		}
	}
	if snap.PDPefindo == nil {
		t.Error("expected PDPefindo non-nil")
	} else if snap.PDPefindo.ApprovedRowCount != 10 {
		t.Errorf("PDPefindo.count = %d; want 10", snap.PDPefindo.ApprovedRowCount)
	}
	if snap.LGDBasel == nil {
		t.Error("expected LGDBasel non-nil")
	}
	if snap.ImpactPD == nil {
		t.Error("expected ImpactPD non-nil")
	} else if snap.ImpactPD.ImpactMultiplier != "1.05000000" {
		t.Errorf("impactMultiplier = %q; want 1.05000000", snap.ImpactPD.ImpactMultiplier)
	}
	if snap.ImpactMevPD == nil {
		t.Error("expected ImpactMevPD non-nil")
	} else {
		if snap.ImpactMevPD.Good == nil || snap.ImpactMevPD.Good.ImpactMultiplier != "0.90000000" {
			t.Errorf("ImpactMevPD.Good = %v", snap.ImpactMevPD.Good)
		}
		if snap.ImpactMevPD.Bad == nil || snap.ImpactMevPD.Bad.ImpactMultiplier != "1.10000000" {
			t.Errorf("ImpactMevPD.Bad = %v", snap.ImpactMevPD.Bad)
		}
	}
	if snap.LPSCoverage == nil {
		t.Error("expected LPSCoverage non-nil")
	} else if snap.LPSCoverage.CoverageLimitIDR != "2000000000.0000" {
		t.Errorf("coverageLimitIdr = %q; want 2000000000.0000", snap.LPSCoverage.CoverageLimitIDR)
	}
	if len(snap.Kurs) != 2 {
		t.Errorf("kurs count = %d; want 2", len(snap.Kurs))
	}
}

// ─── SnapshotAll: bobot DB error propagates (not a calcRunError) ──────────────

func TestSnapshotAll_BobotDBError_Propagates(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnError(errDB("connection timeout"))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error on DB failure")
	}
	// Should NOT be a calcRunError — it's a raw DB error wrapped by fmt.Errorf.
	_, ok := calcrun.IsCalcRunError(err)
	if ok {
		t.Error("DB error should not be wrapped as calcRunError")
	}
}

// ─── SnapshotAll: kurs DB error propagates ────────────────────────────────────

func TestSnapshotAll_KursDBError_Propagates(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(5, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(3, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.80000000", "u1", "2026-06-01").
			AddRow("imev-2", "BAD", "1.20000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}).
			AddRow("lps-1", "2000000000.0000", "2020-01-01", nil, "u1"))
	mock.ExpectQuery(`SELECT .+ FROM mst.kurs`).
		WillReturnError(errDB("disk full"))

	_, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err == nil {
		t.Fatal("expected error on kurs DB failure")
	}
}

// ─── SnapshotAll: ImpactMevPD duplicate skenario → takes first (newest) ───────

func TestSnapshotAll_ImpactMevPD_DuplicateSkenario_TakesFirst(t *testing.T) {
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(5, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(3, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))
	// Two GOOD rows — first wins.
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.80000000", "u1", "2026-06-02"). // first row (newest)
			AddRow("imev-2", "GOOD", "0.75000000", "u1", "2026-06-01"). // second, should be skipped
			AddRow("imev-3", "BAD", "1.20000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}).
			AddRow("lps-1", "2000000000.0000", "2020-01-01", nil, "u1"))
	mock.ExpectQuery(`SELECT .+ FROM mst.kurs`).
		WillReturnRows(sqlmock.NewRows([]string{"kode_mata_uang", "kurs_tengah", "tanggal"}).
			AddRow("USD", "15800.00000000", "2026-06-13"))

	raw, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var snap calcrun.ParameterSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should have taken GOOD with "0.80000000" (first row).
	if snap.ImpactMevPD == nil || snap.ImpactMevPD.Good == nil {
		t.Fatal("expected ImpactMevPD.Good non-nil")
	}
	if snap.ImpactMevPD.Good.ImpactMultiplier != "0.80000000" {
		t.Errorf("GOOD multiplier = %q; want 0.80000000 (first row)", snap.ImpactMevPD.Good.ImpactMultiplier)
	}
}

// ─── Verify DEC-016: no float64 in snapshot JSON ─────────────────────────────

func TestSnapshotAll_NoBobotNumericsNotFloat(t *testing.T) {
	// Regression: if any numeric field is returned as float64 from JSON
	// (i.e. stored as number not string), the round-trip will lose precision.
	// BobotGood "0.2500" must stay "0.2500" after JSON marshal/unmarshal.
	svc, mock := newSnapSvc(t)
	mock.MatchExpectationsInOrder(false)

	mock.ExpectQuery(`SELECT .+ FROM mst.bobot_skenario`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bobot_good", "bobot_normal", "bobot_bad", "approved_by", "approved_at"}).
			AddRow("p1", "0.2500", "0.5000", "0.2500", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.pd_pefindo`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(1, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT COUNT.+ FROM mst.lgd_basel`).
		WillReturnRows(sqlmock.NewRows([]string{"cnt", "approved_by", "approved_at"}).AddRow(1, "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("ip-1", "1.00000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.impact_mev_pd`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "skenario", "impact_multiplier", "approved_by", "approved_at"}).
			AddRow("imev-1", "GOOD", "0.80000000", "u1", "2026-06-01").
			AddRow("imev-2", "BAD", "1.20000000", "u1", "2026-06-01"))
	mock.ExpectQuery(`SELECT .+ FROM mst.lps_coverage`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "coverage_limit_idr", "effective_from", "effective_to", "approved_by"}).
			AddRow("lps-1", "2000000000.0000", "2020-01-01", nil, "u1"))
	mock.ExpectQuery(`SELECT .+ FROM mst.kurs`).
		WillReturnRows(sqlmock.NewRows([]string{"kode_mata_uang", "kurs_tengah", "tanggal"}).
			AddRow("USD", "15800.00000000", "2026-06-13"))

	raw, err := svc.SnapshotAll(context.Background(), "p-2026-06", evalDate())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Decode using json.Number to detect float64 storage.
	var decoded map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	bobot, ok := decoded["bobotSkenario"].(map[string]any)
	if !ok {
		t.Fatal("expected bobotSkenario map")
	}
	// bobotGood must be string "0.2500", not json.Number (which would indicate it was stored as number).
	bg, ok := bobot["bobotGood"].(string)
	if !ok {
		t.Errorf("bobotGood is not string type: %T = %v", bobot["bobotGood"], bobot["bobotGood"])
	} else if bg != "0.2500" {
		t.Errorf("bobotGood = %q; want 0.2500 (exact string, no float precision loss)", bg)
	}
}
