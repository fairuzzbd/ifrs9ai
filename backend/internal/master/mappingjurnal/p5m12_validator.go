package mappingjurnal

// p5m12_validator.go — P5-M12 validators for:
//   - COA akun existence check (MAPPING_AKUN_INVALID)
//   - Debit/kredit line balance check (MAPPING_UNBALANCED)
//   - Regulated flag detection from REGULATED_EVENT_CODES config
//   - Duplicate in-flight version check (MAPPING_DUPLICATE_VERSION)
//   - Periode lock check (MAPPING_PERIODE_LOCKED)
//
// References: P5-M12-S1-AC3, S2-AC1, S3-AC3/AC4.

import (
	"context"
	"fmt"
	"strings"
)

// Validator holds P5-M12 domain validation logic.
// Satisfies DEC-016 (no float64), DEC-017 (SoD), DEC-018 (audit-grade immutability).
type Validator struct {
	repo P5M12Repository
}

// NewValidator creates a Validator.
func NewValidator(repo P5M12Repository) *Validator {
	return &Validator{repo: repo}
}

// ValidateAkunDetails validates that all akun_debit and akun_kredit codes exist in COA.
// Returns (errCode, message) or ("", "") if valid.
// S1-AC3, S3-AC3.
func (v *Validator) ValidateAkunDetails(ctx context.Context, details []AkunDetail, tenantID string) []ImportRowErr {
	var errs []ImportRowErr
	for i, d := range details {
		if ok, _ := v.repo.CoaCodeExists(ctx, d.AkunDebit, tenantID); !ok {
			errs = append(errs, ImportRowErr{
				Row:       i + 1,
				Col:       "akunDebit",
				ErrorCode: CodeMappingAkunInvalid,
				Error:     fmt.Sprintf("MAPPING_AKUN_INVALID: akun_debit '%s' tidak ditemukan di Chart of Accounts.", d.AkunDebit),
			})
		}
		if ok, _ := v.repo.CoaCodeExists(ctx, d.AkunKredit, tenantID); !ok {
			errs = append(errs, ImportRowErr{
				Row:       i + 1,
				Col:       "akunKredit",
				ErrorCode: CodeMappingAkunInvalid,
				Error:     fmt.Sprintf("MAPPING_AKUN_INVALID: akun_kredit '%s' tidak ditemukan di Chart of Accounts.", d.AkunKredit),
			})
		}
	}
	return errs
}

// ValidateBalance checks that total count of debit lines equals kredit lines.
// Per P5-M12-S3-AC4: MAPPING_UNBALANCED if debit line count ≠ kredit line count.
func ValidateBalance(details []AkunDetail) error {
	var debitCount, kreditCount int
	for _, d := range details {
		switch d.DebitKredit {
		case "D":
			debitCount++
		case "K":
			kreditCount++
		}
	}
	if debitCount == 0 || kreditCount == 0 || debitCount != kreditCount {
		return fmt.Errorf("%s: total debit %d lines ≠ total kredit %d lines. Jurnal harus balanced.",
			CodeMappingUnbalanced, debitCount, kreditCount)
	}
	return nil
}

// IsRegulated returns true if the given event_code is in the REGULATED_EVENT_CODES config.
// Reads sys.config from repo. Falls back to hardcoded list on error (belt-and-suspenders).
func (v *Validator) IsRegulated(ctx context.Context, eventCode string, tenantID string) bool {
	codesStr, err := v.repo.GetConfigParam(ctx, "MAPPING_REGULATED_EVENT_CODES")
	if err != nil || codesStr == "" {
		// Fallback hardcoded list (migration 000049 seed)
		return isRegulatedFallback(eventCode)
	}
	for _, code := range strings.Split(codesStr, ",") {
		if strings.TrimSpace(code) == eventCode {
			return true
		}
	}
	return false
}

// isRegulatedFallback is the hardcoded fallback when sys.config is unavailable.
// Must match migration 000049 seed value of MAPPING_REGULATED_EVENT_CODES.
func isRegulatedFallback(eventCode string) bool {
	regulated := map[string]bool{
		"ECL_PEMBENTUKAN":        true,
		"ECL_REVERSAL":           true,
		"POCI_DELTA_ECL":         true,
		"MTM_FVTPL":              true,
		"MTM_FVOCI":              true,
		"MTM_FVOCI_ELECTION":     true,
		"REKLAS_OCI_PL":          true,
		"REKLASIFIKASI_AC_FVOCI": true,
		"REKLASIFIKASI_FVOCI_AC": true,
		"MODIFIKASI_MATERIAL":    true,
		"EIR_CATCH_UP_ADJUSTMENT": true,
		"STAGE_MIGRATION":        true,
		"FX_UNREALIZED":          true,
	}
	return regulated[eventCode]
}

// ValidateSoD4Way enforces SoD: maker≠reviewer≠approver≠approver2.
// Returns CodeMappingSoDViolation error or nil.
// DEC-017: service layer is primary enforcement; DB CHECK is belt-and-suspenders.
func ValidateSoD4Way(makerID, reviewerID, approverID, approver2ID *string, currentActorID string, step string) error {
	switch step {
	case "review":
		if makerID != nil && *makerID == currentActorID {
			return fmt.Errorf("%s: SoD: reviewer tidak dapat sama dengan maker untuk mapping ini (DEC-017). "+
				"Hubungi ROLE-AKUN-CTL lain untuk melakukan review.", CodeMappingSoDViolation)
		}
	case "approve":
		if makerID != nil && *makerID == currentActorID {
			return fmt.Errorf("%s: SoD: approver tidak dapat sama dengan maker (DEC-017).", CodeMappingSoDViolation)
		}
		if reviewerID != nil && *reviewerID == currentActorID {
			return fmt.Errorf("%s: SoD: reviewer tidak dapat menjadi approver untuk mapping yang sama (DEC-017).", CodeMappingSoDViolation)
		}
	case "approve-2":
		if makerID != nil && *makerID == currentActorID {
			return fmt.Errorf("%s: SoD: approver-2 tidak dapat sama dengan maker (DEC-017).", CodeMappingSoDViolation)
		}
		if reviewerID != nil && *reviewerID == currentActorID {
			return fmt.Errorf("%s: SoD: approver-2 tidak dapat sama dengan reviewer (DEC-017).", CodeMappingSoDViolation)
		}
		if approverID != nil && *approverID == currentActorID {
			return fmt.Errorf("%s: SoD: approver-2 tidak dapat sama dengan approver (DEC-017).", CodeMappingSoDViolation)
		}
	}
	return nil
}

// ValidateDetailsNotEmpty checks that all akun fields are non-empty before submit.
// Returns error with first violation found.
// S2 pre-condition 1: all detail rows have non-null akun_debit/kredit.
func ValidateDetailsNotEmpty(details []AkunDetail) error {
	for i, d := range details {
		if d.AkunDebit == "" {
			return fmt.Errorf("%s: detail row %d: akun_debit kosong. Isi semua akun sebelum submit.",
				CodeMappingAkunInvalid, i+1)
		}
		if d.AkunKredit == "" {
			return fmt.Errorf("%s: detail row %d: akun_kredit kosong. Isi semua akun sebelum submit.",
				CodeMappingAkunInvalid, i+1)
		}
	}
	return nil
}

// ValidateBulkRow validates a single MappingBulkRow for the 4-stage import validation.
// Returns []ImportRowErr (empty = valid row).
func (v *Validator) ValidateBulkRow(ctx context.Context, row MappingBulkRow, tenantID string) []ImportRowErr {
	var errs []ImportRowErr

	// Stage 1: required fields
	if row.EventCode == "" {
		errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "event_code", ErrorCode: "VALIDATION_FAILED", Error: "event_code wajib diisi."})
	}
	if row.AkunDebit == "" {
		errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "akun_debit", ErrorCode: "VALIDATION_FAILED", Error: "akun_debit wajib diisi."})
	}
	if row.AkunKredit == "" {
		errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "akun_kredit", ErrorCode: "VALIDATION_FAILED", Error: "akun_kredit wajib diisi."})
	}
	if row.DebitKredit != "D" && row.DebitKredit != "K" {
		errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "debit_kredit", ErrorCode: "VALIDATION_FAILED", Error: "debit_kredit harus 'D' atau 'K'."})
	}
	if len(errs) > 0 {
		return errs
	}

	// Stage 2: event_code exists in header table
	exists, _ := v.repo.EventCodeExists(ctx, row.EventCode, tenantID)
	if !exists {
		errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "event_code", ErrorCode: CodeMappingEventNotFound,
			Error: fmt.Sprintf("MAPPING_EVENT_NOT_FOUND: event_code '%s' tidak ditemukan di mst.mapping_jurnal_header.", row.EventCode)})
		return errs
	}

	// Stage 3: COA cross-reference
	if row.AkunDebit != "" {
		if ok, _ := v.repo.CoaCodeExists(ctx, row.AkunDebit, tenantID); !ok {
			errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "akun_debit", ErrorCode: CodeMappingAkunInvalid,
				Error: fmt.Sprintf("MAPPING_AKUN_INVALID: akun_debit '%s' tidak ditemukan di Chart of Accounts.", row.AkunDebit)})
		}
	}
	if row.AkunKredit != "" {
		if ok, _ := v.repo.CoaCodeExists(ctx, row.AkunKredit, tenantID); !ok {
			errs = append(errs, ImportRowErr{Row: row.RowNumber, Col: "akun_kredit", ErrorCode: CodeMappingAkunInvalid,
				Error: fmt.Sprintf("MAPPING_AKUN_INVALID: akun_kredit '%s' tidak ditemukan di Chart of Accounts.", row.AkunKredit)})
		}
	}

	return errs
}
