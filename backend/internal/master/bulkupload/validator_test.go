package bulkupload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Stub CrossRefLookup ──────────────────────────────────────────────────────

type stubCrossRef struct {
	counterparties map[string]bool
	banks          map[string]bool
	mataUang       map[string]bool
	instrumenKodes map[string]bool
}

func (s *stubCrossRef) CounterpartyExists(id, _ string) (bool, error) {
	return s.counterparties[id], nil
}
func (s *stubCrossRef) BankExists(id, _ string) (bool, error) { return s.banks[id], nil }
func (s *stubCrossRef) MataUangExists(kode, _ string) (bool, error) {
	return s.mataUang[kode], nil
}
func (s *stubCrossRef) InstrumenKodeExists(kode, _ string) (bool, error) {
	return s.instrumenKodes[kode], nil
}

func newFullStubCrossRef() *stubCrossRef {
	return &stubCrossRef{
		counterparties: map[string]bool{"CP-001": true, "ISS-001": true, "EMT-001": true, "MJ-001": true},
		banks:          map[string]bool{"BCA": true, "BNI": true},
		mataUang:       map[string]bool{"IDR": true, "USD": true},
		instrumenKodes: map[string]bool{}, // empty — no conflicts
	}
}

// ─── Happy path: all stages pass ─────────────────────────────────────────────

func TestRunDryRun_AllStagesPass(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito,
			RowNumber: 2,
			Data: map[string]interface{}{
				"kode":               "DEP-001",
				"counterparty_id":    "CP-001",
				"bank_id":            "BCA",
				"mata_uang":          "IDR",
				"saldo":              "1000000000",
				"tanggal_penempatan": "2026-01-01",
				"jatuh_tempo":        "2027-01-01",
				"bunga":              "0.065",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	require.NotNil(t, result)
	assert.Equal(t, StatusDryRunPassed, result.Status)
	assert.Equal(t, 1, result.TotalRows)
	assert.Equal(t, 0, result.InvalidRows)
	assert.Equal(t, "PASS", result.StageSummary.Stage1.Status)
	assert.Equal(t, "PASS", result.StageSummary.Stage2.Status)
	assert.Equal(t, "PASS", result.StageSummary.Stage3.Status)
}

// ─── Stage 1: format validation ──────────────────────────────────────────────

func TestRunDryRun_Stage1_ParseErrors(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito,
			RowNumber: 2,
			Data:      map[string]interface{}{"kode": "DEP-001"},
			ParseErrors: []RowError{
				{Sheet: SheetDeposito, Row: 2, Stage: 1, Col: "saldo", Error: "saldo is required"},
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
	assert.Equal(t, "FAIL", result.StageSummary.Stage1.Status)
	assert.Greater(t, result.StageSummary.Stage1.ErrorCount, 0)
	assert.Equal(t, 1, result.InvalidRows)
}

// ─── Stage 2: business rules ──────────────────────────────────────────────────

func TestRunDryRun_Stage2_DuplicateKode(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-DUP", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
		{
			SheetName: SheetDeposito, RowNumber: 3,
			Data: map[string]interface{}{
				"kode": "DEP-DUP", // duplicate
				"counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "2000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
	assert.Equal(t, "FAIL", result.StageSummary.Stage2.Status)
}

func TestRunDryRun_Stage2_DateLogic_JatuhTempoBefore(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2027-01-01",
				"jatuh_tempo":        "2026-01-01", // before tanggal_penempatan
				"bunga":              "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

func TestRunDryRun_Stage2_InvalidRate(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01",
				"bunga": "1.5", // > 1.0 — invalid
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

func TestRunDryRun_Stage2_NegativeSaldo(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "-1000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

func TestRunDryRun_Stage2_Saham_NegativeJumlahLembar(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetSaham, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "SAH-001", "emiten_id": "EMT-001", "mata_uang": "IDR",
				"jumlah_lembar": "-100", "harga_beli": "7500",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

func TestRunDryRun_Stage2_Obligasi_DateCheck(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetObligasi, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "OBL-001", "issuer_id": "ISS-001", "mata_uang": "IDR",
				"nilai_nominal": "5000000000", "kupon": "0.08",
				"tanggal_penerbitan": "2030-06-01",
				"jatuh_tempo":        "2025-06-01", // before penerbitan
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

// ─── Stage 3: cross-ref lookups ──────────────────────────────────────────────

func TestRunDryRun_Stage3_BankNotFound(t *testing.T) {
	crossRef := &stubCrossRef{
		counterparties: map[string]bool{"CP-001": true},
		banks:          map[string]bool{}, // BCA not present
		mataUang:       map[string]bool{"IDR": true},
		instrumenKodes: map[string]bool{},
	}

	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), crossRef, "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
	assert.Equal(t, "FAIL", result.StageSummary.Stage3.Status)
}

func TestRunDryRun_Stage3_KodeAlreadyExists(t *testing.T) {
	crossRef := &stubCrossRef{
		counterparties: map[string]bool{"CP-001": true},
		banks:          map[string]bool{"BCA": true},
		mataUang:       map[string]bool{"IDR": true},
		instrumenKodes: map[string]bool{"DEP-001": true}, // already in DB (exact case as in row data)
	}

	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), crossRef, "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

func TestRunDryRun_Stage3_MataUangNotFound(t *testing.T) {
	crossRef := &stubCrossRef{
		counterparties: map[string]bool{"CP-001": true},
		banks:          map[string]bool{"BCA": true},
		mataUang:       map[string]bool{}, // IDR not present
		instrumenKodes: map[string]bool{},
	}

	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), crossRef, "TUGURE")
	assert.Equal(t, StatusDryRunFailed, result.Status)
}

// ─── Stage 4: SPPI+BM eval ───────────────────────────────────────────────────

func TestRunDryRun_Stage4_StubEvaluator(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunPassed, result.Status)
	// Stage 4 stub evaluates to AC with no ambiguity
	assert.Equal(t, 1, result.StageSummary.Stage4.Evaluated)
	assert.Equal(t, 1, result.StageSummary.Stage4.Classified)
}

func TestRunDryRun_Stage4_NilEvaluator(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, nil, newFullStubCrossRef(), "TUGURE")
	// Stage 4 unavailable — still DRY_RUN_PASSED (Stage 4 doesn't fail)
	assert.Equal(t, StatusDryRunPassed, result.Status)
	assert.Equal(t, "UNAVAILABLE", result.StageSummary.Stage4.Status)
}

func TestRunDryRun_NilCrossRef(t *testing.T) {
	// Stage 3 skipped when crossRef is nil (unit test mode)
	rows := []ParsedRow{
		{
			SheetName: SheetDeposito, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "DEP-001", "counterparty_id": "CP-001", "bank_id": "BCA",
				"mata_uang": "IDR", "saldo": "1000000",
				"tanggal_penempatan": "2026-01-01", "jatuh_tempo": "2027-01-01", "bunga": "0.05",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), nil, "TUGURE")
	require.NotNil(t, result)
	// Stage 3 not run — result depends on stages 1+2
	assert.Equal(t, "PASS", result.StageSummary.Stage1.Status)
	assert.Equal(t, "PASS", result.StageSummary.Stage2.Status)
}

func TestRunDryRun_EmptyRows(t *testing.T) {
	result := RunDryRun([]ParsedRow{}, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	require.NotNil(t, result)
	assert.Equal(t, 0, result.TotalRows)
	assert.Equal(t, StatusDryRunPassed, result.Status) // no rows = no errors
}

func TestRunDryRun_MultipleSheetTypes(t *testing.T) {
	rows := []ParsedRow{
		{
			SheetName: SheetObligasi, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "OBL-001", "issuer_id": "ISS-001", "mata_uang": "IDR",
				"nilai_nominal": "5000000000", "kupon": "0.08",
				"tanggal_penerbitan": "2025-06-01", "jatuh_tempo": "2030-06-01",
			},
		},
		{
			SheetName: SheetSaham, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "SAH-001", "emiten_id": "EMT-001", "mata_uang": "IDR",
				"jumlah_lembar": "10000", "harga_beli": "7500",
			},
		},
		{
			SheetName: SheetReksadana, RowNumber: 2,
			Data: map[string]interface{}{
				"kode": "RD-001", "manajer_id": "MJ-001", "mata_uang": "IDR",
				"nilai_investasi": "2000000000", "tanggal_investasi": "2026-03-01",
			},
		},
	}

	result := RunDryRun(rows, NewStubSPPIBMEvaluator(), newFullStubCrossRef(), "TUGURE")
	assert.Equal(t, StatusDryRunPassed, result.Status)
	assert.Equal(t, 3, result.TotalRows)
	assert.Equal(t, 0, result.InvalidRows)
}

// ─── IsValidSignatureMethod ───────────────────────────────────────────────────

func TestIsValidSignatureMethod_Valid(t *testing.T) {
	err := IsValidSignatureMethod("JWT_STEP_UP")
	assert.NoError(t, err)
}

func TestIsValidSignatureMethod_Invalid(t *testing.T) {
	err := IsValidSignatureMethod("PASSWORD")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signatureMethod")
}

func TestIsValidSignatureMethod_Empty(t *testing.T) {
	err := IsValidSignatureMethod("")
	require.Error(t, err)
}
