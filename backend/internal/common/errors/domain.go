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

	// P4-M5 EIR Newton-Raphson + Schedule + Amendment codes (APP-C-EIR-001..005).
	CodeEIRNonConvergent             Code = "EIR_NON_CONVERGENT"                // NR exceeded 100 iterations (HTTP 422)
	CodeEIRDivergent                 Code = "EIR_DIVERGENT"                     // f'(r)≈0 or residual growing (HTTP 422)
	CodeEIRCashflowInvalid           Code = "EIR_CASHFLOW_INVALID"              // cashflow null/empty/missing (HTTP 422)
	CodeEIRCashflowSignMismatch      Code = "EIR_CASHFLOW_SIGN_MISMATCH"        // CF[0] must be negative (HTTP 422)
	CodeEIRInstrumenFVTPLNoEIR       Code = "EIR_INSTRUMEN_FVTPL_NO_EIR"        // FVTPL/FVOCI_ELECTION (HTTP 422)
	CodeEIRScheduleNotFound          Code = "EIR_SCHEDULE_NOT_FOUND"            // schedule rows not found (HTTP 404)
	CodeEIRSchedulePeriodeOutOfRange Code = "EIR_SCHEDULE_PERIODE_OUT_OF_RANGE" // periode beyond maturity (HTTP 422)
	CodeEIRDuplicateScheduleVersion  Code = "EIR_DUPLICATE_SCHEDULE_VERSION"    // active schedule exists (HTTP 409)
	CodeEIRPOCIRequiresPDAdjustedCF  Code = "EIR_POCI_REQUIRES_PD_ADJUSTED_CF"  // POCI without PD-adj CF (HTTP 422)
	CodeEIRBulkRecomputeInvalidScope Code = "EIR_BULK_RECOMPUTE_INVALID_SCOPE"  // invalid scope (HTTP 400)
	CodeEIRInstrumenNotFound         Code = "EIR_INSTRUMEN_NOT_FOUND"           // instrument not found (HTTP 404)
	CodeEIRAlreadyComputed           Code = "EIR_ALREADY_COMPUTED"              // eir_awal already set (HTTP 409)
	CodeEIRNotYetComputed            Code = "EIR_NOT_YET_COMPUTED"              // eir_awal IS NULL (HTTP 422)
	CodeEIRAmendNotFound             Code = "EIR_AMEND_NOT_FOUND"               // amendment not found (HTTP 404)
	CodeEIRAmendActiveExists         Code = "EIR_AMEND_ACTIVE_EXISTS"           // active proposal exists (HTTP 409)
	CodeEIRAmendInvalidTransition    Code = "EIR_AMEND_INVALID_TRANSITION"      // invalid state transition (HTTP 422)
	CodeEIRMFAStepUpRequired         Code = "EIR_MFA_STEP_UP_REQUIRED"          // step-up MFA missing (HTTP 403)

	// P4-M6 EIR Amendment Lifecycle codes (APP-C-M6-001..005).
	// State machine: docs/state-machines/p4-m6-amendment-lifecycle.md §6.
	CodeEIRAmendmentDetectionNoMatch    Code = "EIR_AMENDMENT_DETECTION_NO_MATCH"      // 422 — instrumen not eligible / doc type wrong / proposal active
	CodeEIRAmendmentCancelForbidden     Code = "EIR_AMENDMENT_CANCEL_FORBIDDEN"        // 403 — caller not maker, or reviewer already signed
	CodeEIRAmendmentCancelReasonShort   Code = "EIR_AMENDMENT_CANCEL_REASON_TOO_SHORT" // 422 — cancelReason < 20 chars
	CodeEIRDriftReportNotFound          Code = "EIR_DRIFT_REPORT_NOT_FOUND"            // 404 — sys.drift_report row missing
	CodeEIRDriftReportPeriodeOutOfRange Code = "EIR_DRIFT_REPORT_PERIODE_OUT_OF_RANGE" // 422 — periode param out of data range
	CodeEIRDriftGenerationInProgress    Code = "EIR_DRIFT_GENERATION_IN_PROGRESS"      // 409 — concurrent drift job running
	CodeEIRDriftThresholdInvalid        Code = "EIR_DRIFT_THRESHOLD_INVALID"           // 422 — sys.parameter threshold values invalid

	// P4-M8 Calc Run Lifecycle + Seal codes (APP-C-CALC-RUN-001..017).
	// State machine: docs/state-machines/p4-m8-calc-run.md §6.
	CodeCalcRunNotFound                 Code = "CALC_RUN_NOT_FOUND"                  // 404 — ecl.calc_run row not found
	CodeCalcRunInvalidTransition        Code = "CALC_RUN_INVALID_TRANSITION"         // 422 — state machine reject
	CodeCalcRunDuplicateInProgress      Code = "CALC_RUN_DUPLICATE_IN_PROGRESS"      // 409 — another IN_PROGRESS for same periode
	CodeCalcRunPeriodeAlreadySealed     Code = "CALC_RUN_PERIODE_ALREADY_SEALED"     // 409 — a SEALED run exists for same periode
	CodeCalcRunPeriodeHardClosed        Code = "CALC_RUN_PERIODE_HARD_CLOSED"        // 423 — periode is hard-closed
	CodeCalcRunParameterSnapshotInvalid Code = "CALC_RUN_PARAMETER_SNAPSHOT_INVALID" // 422 — required ALCO params missing/not APPROVED
	CodeCalcRunSealRequiresCompleted    Code = "CALC_RUN_SEAL_REQUIRES_COMPLETED"    // 422 — seal request but run not COMPLETED
	CodeCalcRunSealNotRequested         Code = "CALC_RUN_SEAL_NOT_REQUESTED"         // 422 — approve/reject but not in SEAL_REQUESTED
	CodeCalcRunSealSoDViolation         Code = "CALC_RUN_SEAL_SOD_VIOLATION"         // 403 — seal requester == approver
	CodeCalcRunSealStepUpRequired       Code = "CALC_RUN_SEAL_STEP_UP_REQUIRED"      // 403 — step-up MFA missing for seal approve
	CodeCalcRunHasErrors                Code = "CALC_RUN_HAS_ERRORS"                 // 422 — error_count > 0, cannot seal
	CodeCalcRunCancelReasonTooShort     Code = "CALC_RUN_CANCEL_REASON_TOO_SHORT"    // 422 — cancel_reason < 30 chars
	CodeCalcRunCancelAfterCompleted     Code = "CALC_RUN_CANCEL_AFTER_COMPLETED"     // 422 — cannot cancel COMPLETED/SEALED run
	CodeCalcRunECLParamNotFound         Code = "CALC_RUN_ECL_PARAM_NOT_FOUND"        // 422 — ALCO param lookup failed
	CodeCalcRunFXRateNotFound           Code = "CALC_RUN_FX_RATE_NOT_FOUND"          // 422 — BI JISDOR rate missing for eval date
	CodeCalcRunForbiddenNotMaker        Code = "CALC_RUN_FORBIDDEN_NOT_MAKER"        // 403 — cancel attempted by non-maker
	CodeCalcRunSealed                   Code = "CALC_RUN_SEALED"                     // 423 — run is SEALED, immutable

	// P5-M1 Penempatan Deposito codes (APP-B-PEN-001..014).
	// Domain error codes — stable strings matching OpenAPI PenempatanErrorCode enum.
	CodePenempatanInstrumenNotFound           Code = "PENEMPATAN_INSTRUMEN_NOT_FOUND"            // 404
	CodePenempatanInstrumenInvalidKlasifikasi Code = "PENEMPATAN_INSTRUMEN_INVALID_KLASIFIKASI"  // 422
	CodePenempatanTanggalInvalid              Code = "PENEMPATAN_TANGGAL_PENEMPATAN_INVALID"     // 422
	CodePenempatanTenorInvalid                Code = "PENEMPATAN_TENOR_INVALID"                  // 422
	CodePenempatanKuponInvalid                Code = "PENEMPATAN_KUPON_INVALID"                  // 422
	CodePenempatanInvalidTransition           Code = "PENEMPATAN_INVALID_TRANSITION"             // 422
	CodePenempatanSoDViolation                Code = "PENEMPATAN_SOD_VIOLATION"                  // 403
	CodePenempatanStepUpRequired              Code = "PENEMPATAN_STEP_UP_REQUIRED"               // 403
	CodePenempatanReasonTooShort              Code = "PENEMPATAN_REASON_TOO_SHORT"               // 422
	CodePenempatanEditLocked                  Code = "PENEMPATAN_EDIT_LOCKED"                    // 422
	CodePenempatanPeriodeHardClosed           Code = "PENEMPATAN_PERIODE_HARD_CLOSED"            // 423
	CodePenempatanTerminateForbiddenNotActive Code = "PENEMPATAN_TERMINATE_FORBIDDEN_NOT_ACTIVE" // 422
	CodePenempatanCalc2010                    Code = "ERR_CALC_2010"                             // 422 — EIR preview error
	CodePenempatanNotFound                    Code = "PENEMPATAN_NOT_FOUND"                      // 404

	// P4-M11 Roll-Forward CKPN codes (APP-C-M11-001..006).
	// State machine: docs/state-machines/p4-m11-roll-forward.md §8.
	CodeRollForwardPriorNotFound           Code = "ROLL_FORWARD_PRIOR_NOT_FOUND"           // 404 — priorCalcRunId not found
	CodeRollForwardPriorNotSealed          Code = "ROLL_FORWARD_PRIOR_NOT_SEALED"          // 422 — prior not SEALED
	CodeRollForwardCurrentInvalidState     Code = "ROLL_FORWARD_CURRENT_INVALID_STATE"     // 422 — current not COMPLETED/SEALED
	CodeRollForwardPeriodeMismatch         Code = "ROLL_FORWARD_PERIODE_MISMATCH"          // 422 — current periode not after prior
	CodeRollForwardDetectionMethodInvalid  Code = "ROLL_FORWARD_DETECTION_METHOD_INVALID"  // 422 — unknown detectionMethod
	CodeRollForwardExportMismatchForbidden Code = "ROLL_FORWARD_EXPORT_MISMATCH_FORBIDDEN" // 422 — export blocked on MISMATCH
	CodeRollForwardPortfolioNotFound       Code = "ROLL_FORWARD_PORTFOLIO_NOT_FOUND"       // 404 — portofolioId not found
	CodeRollForwardTrendInsufficientData   Code = "ROLL_FORWARD_TREND_INSUFFICIENT_DATA"   // 422 — < 2 SEALED runs
	CodeRollForwardScopeMismatch           Code = "ROLL_FORWARD_SCOPE_MISMATCH"            // 422 — run scope incompatible
	CodeRollForwardInvalidCalcRunStatus    Code = "ROLL_FORWARD_INVALID_CALC_RUN_STATUS"   // 422 — alias CURRENT_INVALID_STATE
	CodeRollForwardInvalidPriorPeriod      Code = "ROLL_FORWARD_INVALID_PRIOR_PERIOD"      // 422 — alias PERIODE_MISMATCH
	CodeRollForwardMismatch                Code = "ROLL_FORWARD_MISMATCH"                  // 422 — reconcile delta ≥ IDR 1.0000

	// P5-M3 GL Delivery codes (APP-D-GL-001..016).
	// State machine: docs/state-machines/p5-m3-gl-delivery.md §6.
	CodeGLDeliveryJurnalNotFound         Code = "GL_DELIVERY_JURNAL_NOT_FOUND"          // 404 — jrnl.header or DLQ entry not found
	CodeGLDeliveryInvalidTransition      Code = "GL_DELIVERY_INVALID_TRANSITION"        // 422 — gl_host_status transition not allowed
	CodeGLDeliveryReasonTooShort         Code = "GL_DELIVERY_REASON_TOO_SHORT"          // 400 — reason < 30 chars
	CodeGLDeliveryMaxAttemptsExceeded    Code = "GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED"     // 422 — total attempts ≥ max
	CodeGLDeliveryPermissionDenied       Code = "GL_DELIVERY_PERMISSION_DENIED"         // 403 — permission denied
	CodeGLDeliveryHostUnreachable        Code = "GL_DELIVERY_HOST_UNREACHABLE"          // delivery: 5xx/timeout (stored in DLQ error_code)
	CodeGLDeliveryHost4XX                Code = "GL_DELIVERY_HOST_4XX"                  // delivery: 4xx domain error (stored in DLQ error_code)
	CodeGLDeliveryInvalidResponse        Code = "GL_DELIVERY_INVALID_RESPONSE"          // delivery: unparseable response
	CodeGLDeliveryTimeout                Code = "GL_DELIVERY_TIMEOUT"                   // delivery: HTTP timeout
	CodeGLDeliveryAuthFailed             Code = "GL_DELIVERY_AUTH_FAILED"               // delivery: auth rejected (401/403)
	CodeGLDLQReplayInvalidState          Code = "GL_DLQ_REPLAY_INVALID_STATE"           // 422 — DLQ entry not in FAILED status
	CodeGLReconciliationReportNotFound   Code = "GL_RECONCILIATION_REPORT_NOT_FOUND"    // 404 — no report for date
	CodeGLReconciliationDateInvalid      Code = "GL_RECONCILIATION_DATE_INVALID"        // 422 — not a business day or invalid format
	CodeGLReconciliationInProgress       Code = "GL_RECONCILIATION_IN_PROGRESS"         // 409 — recon already running for date
	CodeGLReconciliationHostFailed       Code = "GL_RECONCILIATION_HOST_FAILED"         // 500 — GL Host unreachable during recon
	CodeGLStatusTerminalImmutable        Code = "GL_STATUS_TERMINAL_IMMUTABLE"          // 423 — DELIVERED/DEAD_LETTER is immutable

	// P5-M4 Periode Buku Close Workflow codes (APP-D-CLOSE-001..007).
	// State machine: docs/state-machines/p5-m4-periode-close.md §6.
	CodeClosingChecklistFailed Code = "CLOSING_CHECKLIST_FAILED"  // 422 — pre-condition failed
	CodeClosingChecklistStale  Code = "CLOSING_CHECKLIST_STALE"   // 422 — stale + re-run needed
	CodePeriodeSoftClosed      Code = "PERIODE_SOFT_CLOSED"       // 423 — mutations blocked (SOFT_CLOSED)
	CodeMFAStepUpRequired      Code = "MFA_STEP_UP_REQUIRED"      // 401 — step-up token missing
	CodeMFAStepUpExpired       Code = "MFA_STEP_UP_EXPIRED"       // 401 — step-up token > 5 min
	CodePeriodeGraceExpired    Code = "PERIODE_GRACE_EXPIRED"     // 423 — grace window expired
	CodeSoftClosePendingExists Code = "SOFT_CLOSE_PENDING_EXISTS" // 409 — duplicate pending request

	// P5-M2 Jurnal Engine codes (APP-D-JRNL-001..016).
	// State machine: docs/state-machines/p5-m2-jurnal-engine.md §6.
	CodeJurnalEventNotMapped             Code = "JURNAL_EVENT_NOT_MAPPED"               // 422 — no APPROVED_ACTIVE mapping for eventCode
	CodeJurnalKlasifikasiNotEligible     Code = "JURNAL_KLASIFIKASI_NOT_ELIGIBLE"       // 422 — instrument klasifikasi not in mapping.klasifikasi_berlaku
	CodeJurnalBalanceInvariant           Code = "JURNAL_BALANCE_INVARIANT"              // 422 — Σ DEBIT ≠ Σ KREDIT
	CodeJurnalPeriodeHardClosed          Code = "JURNAL_PERIODE_HARD_CLOSED"            // 423 — periode is hard-closed
	CodeJurnalDuplicatePost              Code = "JURNAL_DUPLICATE_POST"                 // 409 — idempotency key already posted
	CodeJurnalInvalidTransition          Code = "JURNAL_INVALID_TRANSITION"             // 422 — state machine reject
	CodeJurnalSoDViolation               Code = "JURNAL_SOD_VIOLATION"                  // 403 — maker = approver (4-eyes SoD)
	CodeJurnalStepUpRequired             Code = "JURNAL_STEP_UP_REQUIRED"               // 403 — step-up MFA missing for regulated approve
	CodeJurnalAmountInvalid              Code = "JURNAL_AMOUNT_INVALID"                 // 422 — amountIdr ≤ 0 or missing
	CodeJurnalInstrumenNotFound          Code = "JURNAL_INSTRUMEN_NOT_FOUND"            // 404 — instrumenId not in mst.instrumen
	CodeJurnalHeaderNotFound             Code = "JURNAL_HEADER_NOT_FOUND"               // 404 — jrnl.header row not found
	CodeJurnalDlqNotFound                Code = "JURNAL_DLQ_NOT_FOUND"                  // 404 — sys.dlq_jurnal_post row not found
	CodeJurnalDlqAlreadyReplayed         Code = "JURNAL_DLQ_ALREADY_REPLAYED"           // 409 — DLQ entry already REPLAYED_OK or REPLAYING
	CodeJurnalDlqDiscardReasonTooShort   Code = "JURNAL_DLQ_DISCARD_REASON_TOO_SHORT"   // 422 — discardReason < 30 chars
	CodeJurnalDlqReplayPeriodeHardClosed Code = "JURNAL_DLQ_REPLAY_PERIODE_HARD_CLOSED" // 423 — replay target periode is hard-closed
	CodeJurnalMappingWorkflowGate        Code = "JURNAL_MAPPING_WORKFLOW_GATE"          // 422 — mapping header not in APPROVED_ACTIVE for resolver
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
	// P4-M5 EIR codes.
	case CodeEIRNonConvergent, CodeEIRDivergent, CodeEIRCashflowInvalid,
		CodeEIRCashflowSignMismatch, CodeEIRInstrumenFVTPLNoEIR,
		CodeEIRSchedulePeriodeOutOfRange, CodeEIRPOCIRequiresPDAdjustedCF,
		CodeEIRNotYetComputed, CodeEIRAmendInvalidTransition:
		return http.StatusUnprocessableEntity
	case CodeEIRScheduleNotFound, CodeEIRInstrumenNotFound, CodeEIRAmendNotFound:
		return http.StatusNotFound
	case CodeEIRDuplicateScheduleVersion, CodeEIRAlreadyComputed, CodeEIRAmendActiveExists:
		return http.StatusConflict
	case CodeEIRBulkRecomputeInvalidScope:
		return http.StatusBadRequest
	case CodeEIRMFAStepUpRequired:
		return http.StatusForbidden
	// P4-M6 EIR Amendment Lifecycle codes.
	case CodeEIRAmendmentDetectionNoMatch,
		CodeEIRAmendmentCancelReasonShort,
		CodeEIRDriftReportPeriodeOutOfRange,
		CodeEIRDriftThresholdInvalid:
		return http.StatusUnprocessableEntity
	case CodeEIRAmendmentCancelForbidden:
		return http.StatusForbidden
	case CodeEIRDriftReportNotFound:
		return http.StatusNotFound
	case CodeEIRDriftGenerationInProgress:
		return http.StatusConflict
	// P4-M8 Calc Run codes.
	case CodeCalcRunNotFound:
		return http.StatusNotFound
	case CodeCalcRunDuplicateInProgress, CodeCalcRunPeriodeAlreadySealed:
		return http.StatusConflict
	case CodeCalcRunPeriodeHardClosed, CodeCalcRunSealed:
		return 423 // Locked
	case CodeCalcRunInvalidTransition, CodeCalcRunParameterSnapshotInvalid,
		CodeCalcRunSealRequiresCompleted, CodeCalcRunSealNotRequested,
		CodeCalcRunHasErrors, CodeCalcRunCancelReasonTooShort,
		CodeCalcRunCancelAfterCompleted, CodeCalcRunECLParamNotFound,
		CodeCalcRunFXRateNotFound:
		return http.StatusUnprocessableEntity
	case CodeCalcRunSealSoDViolation, CodeCalcRunSealStepUpRequired,
		CodeCalcRunForbiddenNotMaker:
		return http.StatusForbidden
	// P4-M11 Roll-Forward CKPN codes.
	case CodeRollForwardPriorNotFound, CodeRollForwardPortfolioNotFound:
		return http.StatusNotFound
	case CodeRollForwardPriorNotSealed, CodeRollForwardCurrentInvalidState,
		CodeRollForwardPeriodeMismatch, CodeRollForwardDetectionMethodInvalid,
		CodeRollForwardExportMismatchForbidden, CodeRollForwardTrendInsufficientData,
		CodeRollForwardScopeMismatch, CodeRollForwardInvalidCalcRunStatus,
		CodeRollForwardInvalidPriorPeriod, CodeRollForwardMismatch:
		return http.StatusUnprocessableEntity
	// P5-M1 Penempatan Deposito codes.
	case CodePenempatanInstrumenNotFound, CodePenempatanNotFound:
		return http.StatusNotFound
	case CodePenempatanInstrumenInvalidKlasifikasi, CodePenempatanTanggalInvalid,
		CodePenempatanTenorInvalid, CodePenempatanKuponInvalid,
		CodePenempatanInvalidTransition, CodePenempatanReasonTooShort,
		CodePenempatanEditLocked, CodePenempatanTerminateForbiddenNotActive,
		CodePenempatanCalc2010:
		return http.StatusUnprocessableEntity
	case CodePenempatanSoDViolation, CodePenempatanStepUpRequired:
		return http.StatusForbidden
	case CodePenempatanPeriodeHardClosed:
		return 423 // Locked
	// P5-M3 GL Delivery codes.
	case CodeGLDeliveryJurnalNotFound, CodeGLReconciliationReportNotFound:
		return http.StatusNotFound
	case CodeGLDeliveryPermissionDenied:
		return http.StatusForbidden
	case CodeGLReconciliationInProgress:
		return http.StatusConflict
	case CodeGLStatusTerminalImmutable:
		return 423 // Locked
	case CodeGLDeliveryInvalidTransition, CodeGLDeliveryMaxAttemptsExceeded,
		CodeGLDLQReplayInvalidState, CodeGLReconciliationDateInvalid:
		return http.StatusUnprocessableEntity
	case CodeGLDeliveryReasonTooShort:
		return http.StatusBadRequest

	// P5-M4 Periode Close Workflow codes.
	case CodeMFAStepUpRequired, CodeMFAStepUpExpired:
		return http.StatusUnauthorized // 401
	case CodeClosingChecklistFailed, CodeClosingChecklistStale:
		return http.StatusUnprocessableEntity // 422
	case CodePeriodeSoftClosed, CodePeriodeGraceExpired:
		return 423 // Locked
	case CodeSoftClosePendingExists:
		return http.StatusConflict // 409

	// P5-M5 FX Rate codes.
	case "FX_RATE_LOCKED":
		return 423 // Locked
	case "KURS_UPLOAD_VALIDATION_FAILED", "KURS_PERIODE_MISMATCH", "KLASIFIKASI_NOT_LOCKED":
		return http.StatusUnprocessableEntity

	// P5-M2 Jurnal Engine codes.
	case CodeJurnalInstrumenNotFound, CodeJurnalHeaderNotFound, CodeJurnalDlqNotFound:
		return http.StatusNotFound
	case CodeJurnalDuplicatePost, CodeJurnalDlqAlreadyReplayed:
		return http.StatusConflict
	case CodeJurnalPeriodeHardClosed, CodeJurnalDlqReplayPeriodeHardClosed:
		return 423 // Locked
	case CodeJurnalSoDViolation, CodeJurnalStepUpRequired:
		return http.StatusForbidden
	case CodeJurnalEventNotMapped, CodeJurnalKlasifikasiNotEligible,
		CodeJurnalBalanceInvariant, CodeJurnalInvalidTransition,
		CodeJurnalAmountInvalid, CodeJurnalDlqDiscardReasonTooShort,
		CodeJurnalMappingWorkflowGate:
		return http.StatusUnprocessableEntity

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

// NewDomainError creates a DomainError with any registered Code and a message.
// Convenience alias for New, allowing callers to import only this package.
func NewDomainError(code Code, message string) *DomainError {
	return New(code, message)
}
