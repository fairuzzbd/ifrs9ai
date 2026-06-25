package bulkupload

// validator.go — 4-stage DRY_RUN validation pipeline for bulk instrumen upload.
//
// Stage 1: cell types + mandatory columns (also done during parse; Stage 1 re-verifies)
// Stage 2: business rules (range, enum, date logic, duplicate kode within batch)
// Stage 3: cross-sheet references (counterparty, bank, mata_uang exist in master data)
// Stage 4: SPPI+BM auto-eval (via SPPIBMEvaluator interface — Phase 3 stub in P5-M11)
//
// All stages collect errors per-row (non-halting).
// DRY_RUN status: DRY_RUN_PASSED if Stage 1-3 all pass (Stage 4 flagged ≠ failure).
//                 DRY_RUN_FAILED if any Stage 1-3 errors exist.
//
// References: P5-M11-S2, docs/state-machines/p5-m11-bulk-upload.md §4.

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CrossRefLookup is the interface for Stage 3 reference checks.
// Implemented by the repository.
type CrossRefLookup interface {
	CounterpartyExists(id string, tenantID string) (bool, error)
	BankExists(id string, tenantID string) (bool, error)
	MataUangExists(kode string, tenantID string) (bool, error)
	InstrumenKodeExists(kode string, tenantID string) (bool, error)
}

// ValidationResult is the output of running the 4-stage pipeline.
type ValidationResult struct {
	Status       BatchStatus
	TotalRows    int
	ValidRows    int
	InvalidRows  int
	FlaggedRows  int
	StageSummary StageSummary
	ErrorsPerRow []RowError
	RowResults   []RowValidationResult
}

// RowValidationResult holds the per-row validation outcome.
type RowValidationResult struct {
	SheetName         SheetName
	RowNumber         int
	RowData           map[string]interface{}
	Status            RowStatus
	KlasifikasiPsak71 *string
	SppiResult        string
	BmResult          string
	FlagReason        string
	Errors            []RowError
}

// RunDryRun executes the 4-stage validation pipeline on parsed rows.
// evaluator may be nil (Stage 4 skipped with UNAVAILABLE status).
// crossRef may be nil (Stage 3 skipped — used in unit tests without DB).
func RunDryRun(rows []ParsedRow, evaluator SPPIBMEvaluator, crossRef CrossRefLookup, tenantID string) *ValidationResult {
	result := &ValidationResult{
		TotalRows: len(rows),
	}

	var allErrors []RowError
	rowResults := make([]RowValidationResult, 0, len(rows))

	// Track kode_instrumen across rows for duplicate detection (Stage 2)
	seenKode := make(map[string]int) // kode → first rowNumber

	stage1Errors := 0
	stage2Errors := 0
	stage3Errors := 0
	stage4Flagged := 0
	stage4Evaluated := 0
	stage4Classified := 0
	sppiUnavailable := false

	for _, pr := range rows {
		var rowErrors []RowError

		// Stage 1: format validation (re-verify parse errors)
		for _, pe := range pr.ParseErrors {
			rowErrors = append(rowErrors, pe)
			stage1Errors++
		}

		// Stage 2: business rules (only if no Stage 1 errors for this row)
		if len(pr.ParseErrors) == 0 {
			s2Errs := validateStage2(pr, seenKode)
			if len(s2Errs) > 0 {
				rowErrors = append(rowErrors, s2Errs...)
				stage2Errors += len(s2Errs)
			}
		}

		// Track kode for duplicate detection
		if kode, ok := pr.Data["kode"].(string); ok && kode != "" {
			seenKode[strings.ToLower(kode)] = pr.RowNumber
		}

		// Stage 3: cross-ref lookups
		if crossRef != nil && len(rowErrors) == 0 {
			s3Errs := validateStage3(pr, crossRef, tenantID)
			if len(s3Errs) > 0 {
				rowErrors = append(rowErrors, s3Errs...)
				stage3Errors += len(s3Errs)
			}
		}

		// Stage 4: SPPI+BM auto-eval
		var klsf *string
		var sppiResult, bmResult, flagReason string
		rowStatus := RowStatusPending
		if len(rowErrors) > 0 {
			rowStatus = RowStatusFailed
		} else if evaluator != nil {
			stage4Evaluated++
			evalResult, evalErr := evaluator.Evaluate(pr.SheetName, pr.Data)
			if evalErr != nil {
				// Service unavailable — flag row as NEEDS_MANUAL_REVIEW
				sppiUnavailable = true
				rowStatus = RowStatusFlaggedManualReview
				flagReason = fmt.Sprintf("SPPI_SERVICE_UNAVAILABLE: %v", evalErr)
				stage4Flagged++
			} else if evalResult.Ambiguous {
				rowStatus = RowStatusFlaggedManualReview
				flagReason = evalResult.FlagReason
				stage4Flagged++
			} else {
				klsf = evalResult.KlasifikasiPsak71
				sppiResult = evalResult.SppiResult
				bmResult = evalResult.BmResult
				stage4Classified++
			}
		} else {
			// No evaluator — skip Stage 4 (unit test / stub mode)
			sppiUnavailable = true
		}

		allErrors = append(allErrors, rowErrors...)
		rowResults = append(rowResults, RowValidationResult{
			SheetName:         pr.SheetName,
			RowNumber:         pr.RowNumber,
			RowData:           pr.Data,
			Status:            rowStatus,
			KlasifikasiPsak71: klsf,
			SppiResult:        sppiResult,
			BmResult:          bmResult,
			FlagReason:        flagReason,
			Errors:            rowErrors,
		})
	}

	// Compute summary
	invalidRows := 0
	for _, rr := range rowResults {
		if rr.Status == RowStatusFailed {
			invalidRows++
		}
	}
	validRows := len(rows) - invalidRows - stage4Flagged

	result.ValidRows = validRows
	result.InvalidRows = invalidRows
	result.FlaggedRows = stage4Flagged
	result.ErrorsPerRow = allErrors
	result.RowResults = rowResults

	// Stage 1-3 status
	s1Status := "PASS"
	if stage1Errors > 0 {
		s1Status = "FAIL"
	}
	s2Status := "PASS"
	if stage2Errors > 0 {
		s2Status = "FAIL"
	}
	s3Status := "PASS"
	if stage3Errors > 0 {
		s3Status = "FAIL"
	}

	s4Status := "PASS"
	if sppiUnavailable {
		s4Status = "UNAVAILABLE"
	} else if stage4Flagged > 0 {
		s4Status = "PARTIAL"
	}

	result.StageSummary = StageSummary{
		Stage1: StageResult{Status: s1Status, ErrorCount: stage1Errors},
		Stage2: StageResult{Status: s2Status, ErrorCount: stage2Errors},
		Stage3: StageResult{Status: s3Status, ErrorCount: stage3Errors},
		Stage4: Stage4Result{
			Status:                 s4Status,
			Evaluated:              stage4Evaluated,
			Classified:             stage4Classified,
			Flagged:                stage4Flagged,
			SppiServiceUnavailable: sppiUnavailable,
		},
	}

	// DRY_RUN_PASSED only if Stage 1-3 all pass
	if stage1Errors == 0 && stage2Errors == 0 && stage3Errors == 0 {
		result.Status = StatusDryRunPassed
	} else {
		result.Status = StatusDryRunFailed
	}

	return result
}

// validateStage2 checks business rules for one row (range, enum, date logic, duplicate kode).
func validateStage2(pr ParsedRow, seenKode map[string]int) []RowError {
	var errs []RowError
	data := pr.Data
	sheet := pr.SheetName
	row := pr.RowNumber

	// Duplicate kode_instrumen within batch
	if kode, ok := data["kode"].(string); ok && kode != "" {
		if firstRow, seen := seenKode[strings.ToLower(kode)]; seen {
			errs = append(errs, RowError{
				Sheet: sheet, Row: row, Stage: 2, Col: "kode",
				Error: fmt.Sprintf("Duplikat kode instrumen '%s' — juga ada di baris %d", kode, firstRow),
			})
		}
	}

	// Date validation: jatuh_tempo > tanggal_penempatan / tanggal_penerbitan
	dateCheck := func(col1, col2 string) {
		d1 := getStr(data, col1)
		d2 := getStr(data, col2)
		if d1 != "" && d2 != "" {
			t1, err1 := time.Parse("2006-01-02", d1)
			t2, err2 := time.Parse("2006-01-02", d2)
			if err1 != nil {
				errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col1,
					Error: fmt.Sprintf("Format tanggal '%s' tidak valid (harus YYYY-MM-DD)", d1)})
			} else if err2 != nil {
				errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col2,
					Error: fmt.Sprintf("Format tanggal '%s' tidak valid (harus YYYY-MM-DD)", d2)})
			} else if !t2.After(t1) {
				errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 2,
					Col:   col2,
					Error: fmt.Sprintf("%s (%s) harus setelah %s (%s)", col2, d2, col1, d1)})
			}
		}
	}

	switch sheet {
	case SheetDeposito, SheetTabungan:
		dateCheck("tanggal_penempatan", "jatuh_tempo")
		validatePositiveNumeric(&errs, sheet, row, data, "saldo")
		validateRateRange(&errs, sheet, row, data, "bunga")

	case SheetObligasi:
		dateCheck("tanggal_penerbitan", "jatuh_tempo")
		validatePositiveNumeric(&errs, sheet, row, data, "nilai_nominal")
		validateRateRange(&errs, sheet, row, data, "kupon")

	case SheetSaham:
		validatePositiveInteger(&errs, sheet, row, data, "jumlah_lembar")
		validatePositiveNumeric(&errs, sheet, row, data, "harga_beli")

	case SheetReksadana:
		validatePositiveNumeric(&errs, sheet, row, data, "nilai_investasi")
	}

	return errs
}

// validateStage3 checks cross-sheet reference existence.
func validateStage3(pr ParsedRow, crossRef CrossRefLookup, tenantID string) []RowError {
	var errs []RowError
	data := pr.Data
	sheet := pr.SheetName
	row := pr.RowNumber

	checkFK := func(col string, fn func(string, string) (bool, error)) {
		val := getStr(data, col)
		if val == "" {
			return // already caught by Stage 1
		}
		exists, err := fn(val, tenantID)
		if err != nil {
			errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 3, Col: col,
				Error: fmt.Sprintf("Gagal validasi referensi %s: %v", col, err)})
			return
		}
		if !exists {
			errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 3, Col: col,
				Error: fmt.Sprintf("%s '%s' tidak ditemukan di master data atau tidak APPROVED", col, val)})
		}
	}

	// mata_uang: all sheets
	checkFK("mata_uang", crossRef.MataUangExists)

	// Counterparty / bank per sheet
	switch sheet {
	case SheetDeposito, SheetTabungan:
		checkFK("bank_id", crossRef.BankExists)
	case SheetObligasi:
		checkFK("issuer_id", crossRef.CounterpartyExists)
	case SheetSaham:
		checkFK("emiten_id", crossRef.CounterpartyExists)
	case SheetReksadana:
		checkFK("manajer_id", crossRef.CounterpartyExists)
	}

	// kode_instrumen must NOT already exist in mst.instrumen (conflict check)
	if kode := getStr(data, "kode"); kode != "" {
		exists, err := crossRef.InstrumenKodeExists(kode, tenantID)
		if err != nil {
			errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 3, Col: "kode",
				Error: fmt.Sprintf("Gagal cek duplikat kode instrumen: %v", err)})
		} else if exists {
			errs = append(errs, RowError{Sheet: sheet, Row: row, Stage: 3, Col: "kode",
				Error: fmt.Sprintf("Kode instrumen '%s' sudah ada di mst.instrumen. Gunakan kode unik.", kode)})
		}
	}

	return errs
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func getStr(data map[string]interface{}, key string) string {
	v, ok := data[strings.ToLower(key)]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func validatePositiveNumeric(errs *[]RowError, sheet SheetName, row int, data map[string]interface{}, col string) {
	val := getStr(data, col)
	if val == "" {
		return
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		*errs = append(*errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col,
			Error: fmt.Sprintf("Expected NUMERIC, got TEXT '%s' di kolom %s", val, col)})
		return
	}
	if f <= 0 {
		*errs = append(*errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col,
			Error: fmt.Sprintf("%s harus > 0, got %s", col, val)})
	}
}

func validateRateRange(errs *[]RowError, sheet SheetName, row int, data map[string]interface{}, col string) {
	val := getStr(data, col)
	if val == "" {
		return
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		*errs = append(*errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col,
			Error: fmt.Sprintf("Expected NUMERIC, got TEXT '%s' di kolom %s", val, col)})
		return
	}
	if f < 0 || f > 1 {
		*errs = append(*errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col,
			Error: fmt.Sprintf("%s harus dalam range [0, 1] (rate desimal), got %s", col, val)})
	}
}

func validatePositiveInteger(errs *[]RowError, sheet SheetName, row int, data map[string]interface{}, col string) {
	val := getStr(data, col)
	if val == "" {
		return
	}
	n, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		*errs = append(*errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col,
			Error: fmt.Sprintf("Expected INTEGER, got '%s' di kolom %s", val, col)})
		return
	}
	if n <= 0 {
		*errs = append(*errs, RowError{Sheet: sheet, Row: row, Stage: 2, Col: col,
			Error: fmt.Sprintf("%s harus > 0, got %d", col, n)})
	}
}
