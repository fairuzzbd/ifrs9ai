// Package helpers — repository nil-DB (dev fallback) tests.
//
// These tests exercise the dev fallback paths (db == nil) that exist in all
// DB repo implementations. They cover the correct defaults and ensure
// no panics occur when repos are constructed without a real DB connection.
//
// DB-backed tests require a live PostgreSQL connection (integration tests)
// and are out of scope for the unit test suite.
package helpers

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── DBPDRepository nil-DB tests ──────────────────────────────────────────────

func TestDBPDRepo_NilDB_BatchLoadPDCurves_Empty(t *testing.T) {
	r := NewDBPDRepository(nil)
	result, err := r.BatchLoadPDCurves(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("BatchLoadPDCurves nil-db error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil empty map")
	}
}

func TestDBPDRepo_NilDB_BatchLoadImpactMevPD_Empty(t *testing.T) {
	r := NewDBPDRepository(nil)
	result, err := r.BatchLoadImpactMevPD(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("BatchLoadImpactMevPD nil-db error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(result))
	}
}

func TestDBPDRepo_NilDB_BatchLoadRatings_Empty(t *testing.T) {
	r := NewDBPDRepository(nil)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	result, err := r.BatchLoadRatings(context.Background(), ids, time.Now())
	if err != nil {
		t.Fatalf("BatchLoadRatings nil-db error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty map, got %d entries", len(result))
	}
}

func TestDBPDRepo_NilDB_GetActiveImpactPD_Nil(t *testing.T) {
	r := NewDBPDRepository(nil)
	result, err := r.GetActiveImpactPD(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("GetActiveImpactPD nil-db error: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil for nil-db GetActiveImpactPD, got %+v", result)
	}
}

// ─── DBLGDRepository nil-DB tests ─────────────────────────────────────────────

func TestDBLGDRepo_NilDB_GetLGDMapping_DefaultFallback(t *testing.T) {
	r := NewDBLGDRepository(nil)
	mapping, err := r.GetLGDMapping(context.Background())
	if err != nil {
		t.Fatalf("GetLGDMapping nil-db error: %v", err)
	}
	if _, ok := mapping["BANK"]; !ok {
		t.Error("Expected BANK key in dev fallback LGD mapping")
	}
}

func TestDBLGDRepo_NilDB_BatchLoadLGDPools_Empty(t *testing.T) {
	r := NewDBLGDRepository(nil)
	result, err := r.BatchLoadLGDPools(context.Background(), "PBUKU-2026-06")
	if err != nil {
		t.Fatalf("BatchLoadLGDPools nil-db error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil empty map")
	}
}

// ─── DBKursRepository nil-DB tests ────────────────────────────────────────────

func TestDBKursRepo_NilDB_GetByDate_Nil(t *testing.T) {
	r := NewDBKursRepository(nil)
	result, err := r.GetByDate(context.Background(), "USD", time.Now())
	if err != nil {
		t.Fatalf("GetByDate nil-db error: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil for nil-db GetByDate, got %+v", result)
	}
}

func TestDBKursRepo_NilDB_BatchLoadKurs_Empty(t *testing.T) {
	r := NewDBKursRepository(nil)
	result, err := r.BatchLoadKurs(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("BatchLoadKurs nil-db error: %v", err)
	}
	if result == nil {
		t.Error("Expected non-nil empty map")
	}
}

// ─── DBInstrumenSnapshotRepo nil-DB tests ─────────────────────────────────────

func TestDBInstrumenRepo_NilDB_GetEADInputs_Nil(t *testing.T) {
	r := NewDBInstrumenSnapshotRepo(nil)
	result, err := r.GetEADInputs(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetEADInputs nil-db error: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil for nil-db GetEADInputs, got %+v", result)
	}
}

func TestDBInstrumenRepo_NilDB_GetCurrentStage_Stage1(t *testing.T) {
	r := NewDBInstrumenSnapshotRepo(nil)
	stage, err := r.GetCurrentStage(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetCurrentStage nil-db error: %v", err)
	}
	if stage != Stage1 {
		t.Errorf("Expected Stage1 default, got %s", stage)
	}
}

func TestDBInstrumenRepo_NilDB_BatchLoadInstruments_Empty(t *testing.T) {
	r := NewDBInstrumenSnapshotRepo(nil)
	ids := []uuid.UUID{uuid.New()}
	result, err := r.BatchLoadInstruments(context.Background(), ids)
	if err != nil {
		t.Fatalf("BatchLoadInstruments nil-db error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result for nil-db, got %d entries", len(result))
	}
}

// ─── DBCCFConfigRepo nil-DB tests ─────────────────────────────────────────────

func TestDBCCFRepo_NilDB_GetCCFTable_DevFallback(t *testing.T) {
	r := NewDBCCFConfigRepo(nil)
	table, err := r.GetCCFTable(context.Background())
	if err != nil {
		t.Fatalf("GetCCFTable nil-db error: %v", err)
	}
	// Dev fallback: DEPOSITO → 0
	depositoCCF, ok := table["DEPOSITO"]
	if !ok {
		t.Error("Expected DEPOSITO key in dev fallback CCF table")
	}
	if !depositoCCF.IsZero() {
		t.Errorf("Expected CCF = 0 for DEPOSITO in dev fallback, got %s", depositoCCF)
	}
}

// ─── DBCounterpartyRepo nil-DB tests ──────────────────────────────────────────

func TestDBCounterpartyRepo_NilDB_GetTipeCounterparty_Empty(t *testing.T) {
	r := NewDBCounterpartyRepo(nil)
	tipe, err := r.GetTipeCounterparty(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetTipeCounterparty nil-db error: %v", err)
	}
	if tipe != "" {
		t.Errorf("Expected empty string for nil-db, got %s", tipe)
	}
}

func TestDBCounterpartyRepo_NilDB_BatchLoadCounterparties_Empty(t *testing.T) {
	r := NewDBCounterpartyRepo(nil)
	ids := []uuid.UUID{uuid.New()}
	result, err := r.BatchLoadCounterparties(context.Background(), ids)
	if err != nil {
		t.Fatalf("BatchLoadCounterparties nil-db error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("Expected empty result for nil-db, got %d entries", len(result))
	}
}

// ─── Batch param functions (anti-N+1) ─────────────────────────────────────────

func TestGetPDFromBatchParams_Stage1(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	jatuhTempo := testEvalDate.AddDate(2, 0, 0)
	inst := InstrumenRow{
		ID:                instrID,
		CounterpartyID:    cpID,
		KlasifikasiPsak71: "AC",
		TanggalJatuhTempo: &jatuhTempo,
	}

	params := &BatchParams{
		Ratings: map[uuid.UUID]string{cpID: "idAA"},
		PDCurves: map[string]PDCurveRow{
			"idAA": {Rating: "idAA", PD12Month: d("0.00350000")},
		},
		ImpactPD: &ImpactPDRow{ImpactMultiplier: d("1.05000000")},
		ImpactMevPD: map[string]ImpactMevPDRow{},
		Counterparties: map[uuid.UUID]CounterpartyRow{cpID: {ID: cpID}},
	}

	pd, detail, err := GetPDFromBatchParams(instrID, Stage1, ScenarioNormal, inst, params, testEvalDate)
	if err != nil {
		t.Fatalf("GetPDFromBatchParams error: %v", err)
	}
	// PD = 0.0035 × 1.05 × 1.0 = 0.003675
	expected := d("0.00367500")
	if !pd.Equal(expected) {
		t.Errorf("PD want %s got %s", expected, pd)
	}
	if !detail.NormalMultiplierIsDefault {
		t.Error("Expected NormalMultiplierIsDefault = true")
	}
}

func TestGetPDFromBatchParams_Stage3_FixedOne(t *testing.T) {
	instrID := uuid.New()
	inst := InstrumenRow{ID: instrID, KlasifikasiPsak71: "AC"}
	params := &BatchParams{
		Ratings:     map[uuid.UUID]string{},
		PDCurves:    map[string]PDCurveRow{},
		ImpactMevPD: map[string]ImpactMevPDRow{},
	}

	pd, _, err := GetPDFromBatchParams(instrID, Stage3, ScenarioGood, inst, params, testEvalDate)
	if err != nil {
		t.Fatalf("GetPDFromBatchParams Stage3 error: %v", err)
	}
	if !pd.Equal(d("1.00000000")) {
		t.Errorf("Stage3 PD want 1.0 got %s", pd)
	}
}

func TestGetLGDFromBatchParams_Basic(t *testing.T) {
	instrID := uuid.New()
	cpID := uuid.New()

	inst := InstrumenRow{ID: instrID, TipeInstrumen: "DEPOSITO", KlasifikasiPsak71: "AC"}
	cp := CounterpartyRow{ID: cpID, TipeCounterparty: "BANK"}

	params := &BatchParams{
		LGDPools:   map[string]LGDBaselRow{"BANK": {TipeEksposur: "BANK", LGD: d("0.45000000")}},
		LGDMapping: map[string]string{"BANK": "BANK"},
	}

	lgd, detail, err := GetLGDFromBatchParams(instrID, inst, cp, params, testPeriode)
	if err != nil {
		t.Fatalf("GetLGDFromBatchParams error: %v", err)
	}
	expected := d("0.45000000")
	if !lgd.Equal(expected) {
		t.Errorf("LGD want %s got %s", expected, lgd)
	}
	if detail.PoolUsed != "BANK" {
		t.Errorf("PoolUsed want BANK got %s", detail.PoolUsed)
	}
}

func TestComputeEADFromBatchParams_IDR(t *testing.T) {
	instrID := uuid.New()
	inst := InstrumenRow{
		ID:           instrID,
		MatauangKode: "IDR",
		Nominal:      d("500000000.0000"),
		TipeInstrumen: "DEPOSITO",
	}
	params := &BatchParams{
		FXRates:      map[string]KursRow{},
		EIRSchedules: map[uuid.UUID]EIRScheduleRow{},
		CCFTable:     map[string]decimal.Decimal{"DEPOSITO": d("0")},
	}

	eadIDR, bd, err := ComputeEADFromBatchParams(instrID, inst, params, testEvalDate)
	if err != nil {
		t.Fatalf("ComputeEADFromBatchParams error: %v", err)
	}

	expected := d("500000000.0000")
	if !eadIDR.Equal(expected) {
		t.Errorf("EAD_IDR want %s got %s", expected, eadIDR)
	}
	if bd.Currency != "IDR" {
		t.Errorf("Currency want IDR got %s", bd.Currency)
	}
}
