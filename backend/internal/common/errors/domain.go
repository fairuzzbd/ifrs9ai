// Package errors menyediakan domain error BLIPS yang ter-map ke stable error codes
// sesuai api-conventions.md §"Error codes". Semua error yang expose ke API WAJIB
// menggunakan tipe-tipe ini; jangan pakai fmt.Errorf langsung di handler.
package errors

import (
	"fmt"
	"net/http"
)

// Code adalah stable error code string (tidak pernah berubah antar minor version).
// Nilai wajib cocok dengan catalogue di _common.yaml §ErrorCode.
type Code string

const (
	CodeValidationFailed           Code = "VALIDATION_FAILED"
	CodeUnauthorized               Code = "UNAUTHORIZED"
	CodeIdleTimeout                Code = "IDLE_TIMEOUT"
	CodeForbidden                  Code = "FORBIDDEN"
	CodeSoDViolation               Code = "SOD_VIOLATION"
	CodeMFARequired                Code = "MFA_REQUIRED"
	CodeMFAChallengeFailed         Code = "MFA_CHALLENGE_FAILED"
	CodeNotFound                   Code = "NOT_FOUND"
	CodeConflict                   Code = "CONFLICT"
	CodeIdempotencyReplay          Code = "IDEMPOTENCY_REPLAY"
	CodeIdempotencyMismatch        Code = "IDEMPOTENCY_MISMATCH"
	CodeWorkflowInvalidTransition  Code = "WORKFLOW_INVALID_TRANSITION"
	CodePeriodeClosed              Code = "PERIODE_CLOSED"
	CodeECLParamFrozen             Code = "ECL_PARAM_FROZEN"
	CodeRateLimited                Code = "RATE_LIMITED"
	CodeInternal                   Code = "INTERNAL"
	CodeSPPITestIncomplete         Code = "SPPI_TEST_INCOMPLETE"
	CodeBMAssessmentRequired       Code = "BM_ASSESSMENT_REQUIRED"
	CodeSoDApprover1SameAsMaker    Code = "SOD_APPROVER1_SAME_AS_MAKER"
	CodeSoDApprover2SameAsReviewer Code = "SOD_APPROVER2_SAME_AS_REVIEWER"
	CodeInvalidSortCol             Code = "INVALID_SORT_COL"
	CodeStepUpRequired             Code = "STEP_UP_REQUIRED"
	CodeStepUpExpired              Code = "STEP_UP_EXPIRED"
	CodeJobNotCancellable          Code = "JOB_NOT_CANCELLABLE"
	CodeJobNotFound                Code = "JOB_NOT_FOUND"
	// Master-data module codes (shared across all mst.* modules)
	CodeSystemCurrencyProtected Code = "SYSTEM_CURRENCY_PROTECTED"
	CodeEntityInUse             Code = "ENTITY_IN_USE"
	CodeMasterApprovedNoEdit    Code = "MASTER_APPROVED_NO_EDIT"

	// Chart of Accounts specific codes
	CodeCoADuplicateKode     Code = "COA_DUPLICATE_KODE"
	CodeCoAInvalidKodeFormat Code = "COA_INVALID_KODE_FORMAT"
	CodeCoAParentNotFound    Code = "COA_PARENT_NOT_FOUND"

	// Instrumen-specific codes
	CodeInstrumenDuplicateKode           Code = "INSTRUMEN_DUPLICATE_KODE"
	CodeInstrumenCounterpartyNotApproved Code = "INSTRUMEN_COUNTERPARTY_NOT_APPROVED"
	CodeInstrumenPortofolioNotApproved   Code = "INSTRUMEN_PORTOFOLIO_NOT_APPROVED"
	CodeInstrumenMataUangNotApproved     Code = "INSTRUMEN_MATA_UANG_NOT_APPROVED"
	CodeInstrumenInvalidTipe             Code = "INSTRUMEN_INVALID_TIPE"
	CodeInstrumenKlasifikasiLocked       Code = "INSTRUMEN_KLASIFIKASI_LOCKED"
	CodeInstrumenMissingKustodian        Code = "INSTRUMEN_MISSING_KUSTODIAN"

	// Portofolio-specific codes
	CodePortofolioDuplicateKode     Code = "PORTOFOLIO_DUPLICATE_KODE"
	CodePortofolioInvalidBMCategory Code = "PORTOFOLIO_INVALID_BM_CATEGORY"
	CodePortofolioInvalidKodeFormat Code = "PORTOFOLIO_INVALID_KODE_FORMAT"

	// ECL parameter module codes (mst.pd_pefindo, mst.lgd_basel, etc.) — HTTP 422.
	CodePDMonotonicityViolated       Code = "PD_MONOTONICITY_VIOLATED"
	CodePDPeriodOverlap              Code = "PD_PERIOD_OVERLAP"
	CodeLGDPeriodOverlap             Code = "LGD_PERIOD_OVERLAP"
	CodeBobotSumInvariantViolated    Code = "BOBOT_SUM_INVARIANT_VIOLATED"
	CodeBobotPeriodOverlap           Code = "BOBOT_PERIOD_OVERLAP"
	CodeBobotDuplicateSkenarioPeriod Code = "BOBOT_DUPLICATE_SKENARIO_PERIOD"
	CodeLPSPeriodOverlap             Code = "LPS_PERIOD_OVERLAP"

	// FL Multiplier module codes (mst.impact_mev_pd, mst.impact_pd) — HTTP 422.
	CodeFLPeriodDuplicate Code = "FL_PERIODE_DUPLICATE"       // (periode_id[,skenario]) already active
	CodeFLMultiplierRange Code = "FL_MULTIPLIER_OUT_OF_RANGE" // impact_pd outside [0.5,2.0]

	// Mapping jurnal module codes (APP-D) — HTTP 422.
	CodeMappingJurnalDebitCreditMismatch Code = "MAPPING_JURNAL_DEBIT_CREDIT_MISMATCH" //nolint:gosec
	CodeMappingJurnalKodeAkunNotApproved Code = "MAPPING_JURNAL_KODE_AKUN_NOT_APPROVED"

	// P4-M2 ECL Helpers — PD lookup codes (APP-C-PAR-001) — HTTP 422/404.
	CodePDLookupRatingMissing     Code = "PD_LOOKUP_RATING_MISSING"     // counterparty has no active rating per evaluationDate
	CodePDLookupCurveNotFound     Code = "PD_LOOKUP_CURVE_NOT_FOUND"    // no APPROVED pd_pefindo row for rating
	CodePDLookupParameterInactive Code = "PD_LOOKUP_PARAMETER_INACTIVE" // impact_pd not APPROVED for periodeId
	CodePDLookupFLParamMissing    Code = "PD_LOOKUP_FL_PARAM_MISSING"   // impact_mev_pd (GOOD/BAD) not APPROVED
	CodePDLookupTenorOutOfRange   Code = "PD_LOOKUP_TENOR_OUT_OF_RANGE" // tanggal_jatuh_tempo in the past (anomaly)

	// P4-M2 ECL Helpers — LGD lookup codes (APP-C-PAR-002) — HTTP 422.
	CodeLGDLookupPoolNotFound       Code = "LGD_LOOKUP_POOL_NOT_FOUND"      // no APPROVED lgd_basel row for tipe_eksposur
	CodeLGDLookupMappingNotFound    Code = "LGD_LOOKUP_MAPPING_NOT_FOUND"   // tipe_counterparty not in LGD_COUNTERPARTY_TYPE_MAPPING
	CodeLGDCollateralHaircutInvalid Code = "LGD_COLLATERAL_HAIRCUT_INVALID" // collateral haircut out of [0,1) range
	CodeLGDLookupUseLookthrough     Code = "LGD_LOOKUP_USE_LOOKTHROUGH"     // REKSADANA must use P4-M4 look-through

	// P4-M2 ECL Helpers — EAD computation codes (APP-C-PAR-003) — HTTP 422/404.
	CodeEADFXRateMissing     Code = "EAD_FX_RATE_MISSING"      // no kurs BI JISDOR for currency per evaluationDate
	CodeEADFXRateNotApproved Code = "EAD_FX_RATE_NOT_APPROVED" // kurs found but workflow_status != APPROVED
	CodeEADInstrumenNotFound Code = "EAD_INSTRUMEN_NOT_FOUND"  // instrumenId not found in mst.instrumen

	// P4-M2 ECL Helpers — CCF lookup codes (APP-C-PAR-004) — HTTP 422.
	CodeCCFConfigMissing        Code = "CCF_CONFIG_MISSING"         // sys.config CCF_TABLE not found
	CodeCCFInstrumenTypeUnknown Code = "CCF_INSTRUMEN_TYPE_UNKNOWN" // tipe_instrumen not in TipeInstrumen enum

	// P4-M2 ECL Helpers — bulk / cross-cutting codes.
	CodeHelpersBulkTooLarge              Code = "HELPERS_BULK_TOO_LARGE"              // > 1000 instruments per batch (HTTP 413)
	CodeHelpersParameterSnapshotMismatch Code = "HELPERS_PARAMETER_SNAPSHOT_MISMATCH" // calc run sealed with old snapshot (HTTP 409)
	CodeInstrumentECLNotApplicable       Code = "INSTRUMENT_ECL_NOT_APPLICABLE"       // FVTPL / FVOCI_ELECTION (HTTP 422)
	CodeECLParamNotReady                 Code = "ECL_PARAM_NOT_READY"                 // parameters not all APPROVED for periodeId (HTTP 422)

	// F3: POCI instruments require credit-adjusted EIR from P4-M7 (FSD-APP-C §3.5, IFRS9 §5.5.13).
	CodePOCIDeferredToM7 Code = "POCI_DEFERRED_TO_M7" // POCI instrument — deferred to P4-M7 (HTTP 422)

	// P4-M3 LPS Aggregator codes (APP-C-LPS-001..005) — docs/state-machines/p4-m3-lps.md §4.
	CodeLPSCoverageNoActiveParam         Code = "LPS_COVERAGE_NO_ACTIVE_PARAM"         // no APPROVED mst.lps_coverage for evalDate (HTTP 422)
	CodeLPSOverrideInstrumenNotFound     Code = "LPS_OVERRIDE_INSTRUMEN_NOT_FOUND"     // instrumenId not found (HTTP 404)
	CodeLPSOverrideReasonTooShort        Code = "LPS_OVERRIDE_REASON_TOO_SHORT"        // exclusion_reason < 30 chars (HTTP 422)
	CodeLPSOverrideInvalidTransition     Code = "LPS_OVERRIDE_INVALID_TRANSITION"      // invalid workflow state transition (HTTP 422)
	CodeLPSOverrideExpired               Code = "LPS_OVERRIDE_EXPIRED"                 // override effectiveTo already passed (HTTP 410)
	CodeLPSOverrideSoDViolation          Code = "LPS_OVERRIDE_SOD_VIOLATION"           // approver == maker (HTTP 403)
	CodeLPSOverridePeriodeInvalid        Code = "LPS_OVERRIDE_PERIODE_INVALID"         // effectiveFrom > effectiveTo (HTTP 422)
	CodeLPSAggregateInstrumenNotDeposito Code = "LPS_AGGREGATE_INSTRUMEN_NOT_DEPOSITO" // instrument not DEPOSITO type (HTTP 422)
	CodeLPSAggregateBulkTooLarge         Code = "LPS_AGGREGATE_BULK_TOO_LARGE"         // instrument scope > 50000 (HTTP 413)

	// P4-M4 Look-through ECL codes (APP-C-LKT-001..005) — docs/state-machines/p4-m4-lookthrough.md §4.
	CodeLookthroughFundCompositionMissing             Code = "LOOKTHROUGH_FUND_COMPOSITION_MISSING"              // no APPROVED_ACTIVE composition for instrumenID on evalDate (HTTP 422)
	CodeLookthroughNABMissing                         Code = "LOOKTHROUGH_NAB_MISSING"                           // mst.instrumen.nominal_nab_idr IS NULL (HTTP 422)
	CodeLookthroughWeightInvalid                      Code = "LOOKTHROUGH_WEIGHT_INVALID"                        // Σ weight_pct ≠ 100% ± 0.01% (HTTP 422)
	CodeLookthroughInstrumenNotReksadana              Code = "LOOKTHROUGH_INSTRUMEN_NOT_REKSADANA"               // tipe_instrumen ≠ REKSADANA (HTTP 422)
	CodeLookthroughAssetClassUnknown                  Code = "LOOKTHROUGH_ASSET_CLASS_UNKNOWN"                   // unknown asset_class enum value (HTTP 422)
	CodeLookthroughPDLGDClassMissing                  Code = "LOOKTHROUGH_PD_LGD_CLASS_MISSING"                  // PD/LGD lookup failed for asset class (HTTP 422)
	CodeLookthroughCompositionReviewInvalidTransition Code = "LOOKTHROUGH_COMPOSITION_REVIEW_INVALID_TRANSITION" // invalid workflow state transition (HTTP 422)
	CodeLookthroughCompositionSoDViolation            Code = "LOOKTHROUGH_COMPOSITION_SOD_VIOLATION"             // SoD violation in composition workflow (HTTP 403)
	CodeLookthroughBulkTooLarge                       Code = "LOOKTHROUGH_BULK_TOO_LARGE"                        // REKSADANA scope > 10000 instruments (HTTP 413)
	CodeLookthroughPOCIDeferred                       Code = "LOOKTHROUGH_POCI_DEFERRED"                         // POCI Reksadana deferred to Phase 5 (HTTP 422)
)

// HTTPStatus memetakan Code ke HTTP status code.
func (c Code) HTTPStatus() int {
	switch c {
	case CodeValidationFailed, CodeInvalidSortCol:
		return http.StatusBadRequest
	case CodeUnauthorized, CodeIdleTimeout:
		return http.StatusUnauthorized
	case CodeForbidden, CodeSoDViolation, CodeMFARequired, CodeMFAChallengeFailed,
		CodeSoDApprover1SameAsMaker, CodeSoDApprover2SameAsReviewer,
		CodeStepUpRequired, CodeStepUpExpired:
		return http.StatusForbidden
	case CodeSystemCurrencyProtected, CodeMasterApprovedNoEdit:
		return http.StatusForbidden
	case CodeCoADuplicateKode, CodeCoAInvalidKodeFormat, CodeCoAParentNotFound:
		return http.StatusUnprocessableEntity
	case CodeInstrumenDuplicateKode:
		return http.StatusConflict
	case CodeInstrumenCounterpartyNotApproved, CodeInstrumenPortofolioNotApproved,
		CodeInstrumenMataUangNotApproved, CodeInstrumenInvalidTipe,
		CodeInstrumenMissingKustodian:
		return http.StatusUnprocessableEntity
	case CodeInstrumenKlasifikasiLocked:
		return 423 // Locked
	case CodeEntityInUse, CodePortofolioDuplicateKode:
		return http.StatusConflict
	case CodePortofolioInvalidKodeFormat, CodePortofolioInvalidBMCategory:
		return http.StatusBadRequest
	case CodeNotFound, CodeJobNotFound:
		return http.StatusNotFound
	case CodeConflict:
		return http.StatusConflict
	case CodeIdempotencyReplay:
		return http.StatusOK // replay: return original status
	case CodeIdempotencyMismatch, CodeWorkflowInvalidTransition,
		CodeSPPITestIncomplete, CodeBMAssessmentRequired, CodeJobNotCancellable,
		CodePDMonotonicityViolated, CodePDPeriodOverlap,
		CodeBobotSumInvariantViolated, CodeBobotPeriodOverlap, CodeBobotDuplicateSkenarioPeriod,
		CodeLGDPeriodOverlap, CodeLPSPeriodOverlap,
		CodeFLPeriodDuplicate, CodeFLMultiplierRange,
		CodeMappingJurnalDebitCreditMismatch, CodeMappingJurnalKodeAkunNotApproved:
		return http.StatusUnprocessableEntity
	// P4-M2 ECL Helpers error codes.
	case CodePDLookupRatingMissing, CodePDLookupCurveNotFound,
		CodePDLookupParameterInactive, CodePDLookupFLParamMissing,
		CodePDLookupTenorOutOfRange,
		CodeLGDLookupPoolNotFound, CodeLGDLookupMappingNotFound,
		CodeLGDCollateralHaircutInvalid, CodeLGDLookupUseLookthrough,
		CodeEADFXRateMissing, CodeEADFXRateNotApproved,
		CodeCCFConfigMissing, CodeCCFInstrumenTypeUnknown,
		CodeInstrumentECLNotApplicable, CodeECLParamNotReady,
		CodeHelpersParameterSnapshotMismatch,
		CodePOCIDeferredToM7:
		return http.StatusUnprocessableEntity
	case CodeEADInstrumenNotFound:
		return http.StatusNotFound
	case CodeHelpersBulkTooLarge, CodeLPSAggregateBulkTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeLPSOverrideExpired:
		return http.StatusGone // 410
	case CodeLPSOverrideSoDViolation:
		return http.StatusForbidden
	case CodeLPSOverrideInstrumenNotFound:
		return http.StatusNotFound
	case CodeLPSCoverageNoActiveParam, CodeLPSOverrideReasonTooShort,
		CodeLPSOverrideInvalidTransition, CodeLPSOverridePeriodeInvalid,
		CodeLPSAggregateInstrumenNotDeposito:
		return http.StatusUnprocessableEntity
	// P4-M4 Look-through ECL error codes.
	case CodeLookthroughFundCompositionMissing, CodeLookthroughNABMissing,
		CodeLookthroughWeightInvalid, CodeLookthroughInstrumenNotReksadana,
		CodeLookthroughAssetClassUnknown, CodeLookthroughPDLGDClassMissing,
		CodeLookthroughCompositionReviewInvalidTransition, CodeLookthroughPOCIDeferred:
		return http.StatusUnprocessableEntity
	case CodeLookthroughCompositionSoDViolation:
		return http.StatusForbidden
	case CodeLookthroughBulkTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodePeriodeClosed, CodeECLParamFrozen:
		return 423 // Locked
	case CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

// Detail adalah field-level error detail (sesuai OpenAPI ErrorDetail).
type Detail struct {
	Field   string `json:"field,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Message string `json:"message,omitempty"`
}

// DomainError adalah error yang aman untuk diekspos ke client melalui API.
// Implements error interface.
type DomainError struct {
	code    Code
	message string
	details []Detail
	cause   error
}

func (e *DomainError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Unwrap memungkinkan errors.Is/As bekerja dengan cause chain.
func (e *DomainError) Unwrap() error { return e.cause }

// Code mengembalikan stable error code.
func (e *DomainError) Code() Code { return e.code }

// Message mengembalikan pesan yang aman untuk client.
func (e *DomainError) Message() string { return e.message }

// Details mengembalikan field-level error details.
func (e *DomainError) Details() []Detail { return e.details }

// HTTPStatus mengembalikan HTTP status code yang sesuai.
func (e *DomainError) HTTPStatus() int { return e.code.HTTPStatus() }

// New membuat DomainError baru.
func New(code Code, message string, details ...Detail) *DomainError {
	return &DomainError{code: code, message: message, details: details}
}

// Wrap membungkus error underlying dengan domain error.
func Wrap(code Code, message string, cause error) *DomainError {
	return &DomainError{code: code, message: message, cause: cause}
}

// IsDomainError mengecek apakah error adalah DomainError.
func IsDomainError(err error) (*DomainError, bool) {
	var de *DomainError
	if ok := asError(err, &de); ok {
		return de, true
	}
	return nil, false
}

// asError adalah helper untuk menghindari import errors stdlib pada level ini.
func asError(err error, target **DomainError) bool {
	if err == nil {
		return false
	}
	if de, ok := err.(*DomainError); ok {
		*target = de
		return true
	}
	// Walk cause chain
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return asError(u.Unwrap(), target)
	}
	return false
}

// --- Convenience constructors ---

// ErrUnauthorized returns 401 UNAUTHORIZED.
func ErrUnauthorized(msg string) *DomainError {
	return New(CodeUnauthorized, msg)
}

// ErrIdleTimeout returns 401 IDLE_TIMEOUT.
func ErrIdleTimeout() *DomainError {
	return New(CodeIdleTimeout, "Sesi idle lebih dari 15 menit. Silakan login kembali.")
}

// ErrForbidden returns 403 FORBIDDEN.
func ErrForbidden(permission string) *DomainError {
	return New(CodeForbidden,
		fmt.Sprintf("Anda tidak memiliki permission '%s'", permission))
}

// ErrSoDViolation returns 403 SOD_VIOLATION.
func ErrSoDViolation(msg string) *DomainError {
	return New(CodeSoDViolation, msg)
}

// ErrStepUpRequired returns 403 STEP_UP_REQUIRED.
func ErrStepUpRequired(action string) *DomainError {
	return New(CodeStepUpRequired,
		fmt.Sprintf("Action '%s' membutuhkan step-up MFA. Hubungi /auth/step-up.", action))
}

// ErrStepUpExpired returns 403 STEP_UP_EXPIRED.
func ErrStepUpExpired() *DomainError {
	return New(CodeStepUpExpired, "Step-up MFA sudah expired (> 5 menit). Ulangi /auth/step-up.")
}

// ErrNotFound returns 404 NOT_FOUND.
func ErrNotFound(entity string) *DomainError {
	return New(CodeNotFound, fmt.Sprintf("%s tidak ditemukan.", entity))
}

// ErrConflict returns 409 CONFLICT (optimistic lock).
func ErrConflict() *DomainError {
	return New(CodeConflict, "Data sudah diubah oleh pengguna lain. Refresh dan ulangi.")
}

// ErrIdempotencyMismatch returns 422 IDEMPOTENCY_MISMATCH.
func ErrIdempotencyMismatch() *DomainError {
	return New(CodeIdempotencyMismatch, "Idempotency-Key sudah dipakai dengan payload berbeda dari request sebelumnya.")
}

// ErrRateLimited returns 429 RATE_LIMITED.
func ErrRateLimited() *DomainError {
	return New(CodeRateLimited, "Terlalu banyak request. Coba lagi dalam 60 detik.")
}

// ErrInternal returns 500 INTERNAL.
func ErrInternal(cause error) *DomainError {
	return Wrap(CodeInternal, "Terjadi kesalahan internal. Hubungi admin dengan traceId.", cause)
}

// ErrMFARequired returns 403 MFA_REQUIRED.
func ErrMFARequired() *DomainError {
	return New(CodeMFARequired, "Action ini membutuhkan verifikasi MFA.")
}

// ErrWorkflowInvalidTransition returns 422 WORKFLOW_INVALID_TRANSITION.
func ErrWorkflowInvalidTransition(from, to string) *DomainError {
	return New(CodeWorkflowInvalidTransition,
		fmt.Sprintf("Transisi dari '%s' ke '%s' tidak valid.", from, to),
		Detail{Field: "state", Rule: "invalid_transition",
			Message: fmt.Sprintf("Transition %s → %s tidak valid", from, to)})
}
