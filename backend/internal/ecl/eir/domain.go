// Package eir implements the EIR Newton-Raphson solver, amortization schedule
// builder, amendment re-estimation workflow (4-eyes), bulk re-compute job (P4-M5),
// and amendment lifecycle detection / drift reporting / cancel (P4-M6).
//
// Formula (formulas.md §EIR Newton-Raphson, FSD-APP-C §4):
//
//	Find r (per-period rate) such that:
//	  Σ CF_t / (1+r)^t = 0   where CF_0 < 0 (initial outflow)
//
//	Newton-Raphson iteration:
//	  f(r)  = Σ CF_t / (1+r)^t
//	  f'(r) = -Σ t × CF_t / (1+r)^(t+1)
//	  r_{n+1} = r_n - f(r_n) / f'(r_n)
//
// Precision: shopspring/decimal throughout — NEVER float64 for money or rates.
// Rounding: HALF_EVEN (RoundBank) per SoW §4.
// Storage: NUMERIC(10,8) for EIR, NUMERIC(20,4) for IDR amounts (DEC-016).
//
// Decisions: DEC-013, DEC-016, DEC-017, DEC-018.
// Migrations: db/migrations/000026_eir_schema_fix.up.sql, 000027_drift_report_and_amendment_lifecycle.up.sql.
// Stories: docs/stories/phase-4/M5-eir-solver.md, M6-eir-amendment-lifecycle.md.
// State machines: docs/state-machines/p4-m5-eir.md, p4-m6-amendment-lifecycle.md.
package eir

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Permission constants ──────────────────────────────────────────────────────

const (
	// PermEIRCompute is the permission for computing EIR (ROLE-RISK, System).
	PermEIRCompute = "eir.compute"
	// PermEIRPreview is the permission for reading EIR schedule (ROLE-RISK, ROLE-AKUN, ROLE-AUDIT).
	PermEIRPreview = "eir.preview"
	// PermEIRAmendPropose is the permission for proposing an amendment (ROLE-AKUN).
	PermEIRAmendPropose = "eir.amend.propose"
	// PermEIRAmendReview is the permission for reviewing an amendment (ROLE-RISK).
	PermEIRAmendReview = "eir.amend.review"
	// PermEIRAmendApprove is the permission for approving an amendment (ROLE-ALCO, step-up MFA).
	PermEIRAmendApprove = "eir.amend.approve"
	// PermEIRBulkRecompute is the permission for triggering bulk re-compute (ROLE-RISK, System).
	PermEIRBulkRecompute = "eir.bulk_recompute"
)

// ─── Error codes (P4-M5) ──────────────────────────────────────────────────────
// These match the 14 EIR error codes in api/openapi/app-c-eir.yaml and the
// HTTPStatus map in backend/internal/common/errors/domain.go.

const (
	// CodeEIRNonConvergent: Newton-Raphson exceeded max 100 iterations. HTTP 422.
	CodeEIRNonConvergent = "EIR_NON_CONVERGENT"
	// CodeEIRDivergent: residual growing / f'(r) ≈ 0. HTTP 422.
	CodeEIRDivergent = "EIR_DIVERGENT"
	// CodeEIRCashflowInvalid: cashflow null/empty/missing fields. HTTP 422.
	CodeEIRCashflowInvalid = "EIR_CASHFLOW_INVALID"
	// CodeEIRCashflowSignMismatch: CF[0] must be negative. HTTP 422.
	CodeEIRCashflowSignMismatch = "EIR_CASHFLOW_SIGN_MISMATCH"
	// CodeEIRInstrumenFVTPLNoEIR: FVTPL/FVOCI_ELECTION instrument. HTTP 422.
	CodeEIRInstrumenFVTPLNoEIR = "EIR_INSTRUMEN_FVTPL_NO_EIR"
	// CodeEIRScheduleNotFound: schedule not found. HTTP 404.
	CodeEIRScheduleNotFound = "EIR_SCHEDULE_NOT_FOUND"
	// CodeEIRSchedulePeriodeOutOfRange: periode beyond maturity date. HTTP 422.
	CodeEIRSchedulePeriodeOutOfRange = "EIR_SCHEDULE_PERIODE_OUT_OF_RANGE"
	// CodeEIRDuplicateScheduleVersion: active schedule already exists. HTTP 409.
	CodeEIRDuplicateScheduleVersion = "EIR_DUPLICATE_SCHEDULE_VERSION"
	// CodeEIRPOCIRequiresPDAdjustedCF: POCI instrument without PD-adjusted CF. HTTP 422.
	CodeEIRPOCIRequiresPDAdjustedCF = "EIR_POCI_REQUIRES_PD_ADJUSTED_CF"
	// CodeEIRBulkRecomputeInvalidScope: invalid scope parameter. HTTP 400.
	CodeEIRBulkRecomputeInvalidScope = "EIR_BULK_RECOMPUTE_INVALID_SCOPE"
	// CodeEIRInstrumenNotFound: instrument not found. HTTP 404.
	CodeEIRInstrumenNotFound = "EIR_INSTRUMEN_NOT_FOUND"
	// CodeEIRAlreadyComputed: eir_awal already set. HTTP 409.
	CodeEIRAlreadyComputed = "EIR_ALREADY_COMPUTED"
	// CodeEIRNotYetComputed: generate schedule called before EIR is computed. HTTP 422.
	CodeEIRNotYetComputed = "EIR_NOT_YET_COMPUTED"
	// CodeEIRAmendNotFound: amendment proposal not found. HTTP 404.
	CodeEIRAmendNotFound = "EIR_AMEND_NOT_FOUND"
	// CodeEIRAmendActiveExists: another active (non-terminal) proposal exists. HTTP 409.
	CodeEIRAmendActiveExists = "EIR_AMEND_ACTIVE_EXISTS"
	// CodeEIRAmendInvalidTransition: workflow state transition not valid. HTTP 422.
	CodeEIRAmendInvalidTransition = "EIR_AMEND_INVALID_TRANSITION"
	// CodeEIRMFAStepUpRequired: step-up MFA not provided for approve. HTTP 403.
	CodeEIRMFAStepUpRequired = "EIR_MFA_STEP_UP_REQUIRED"
)

// errEIR builds a DomainError with an EIR-specific code string.
func errEIR(code string, msg string, details ...domainerrors.Detail) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.Code(code), msg, details...)
}

// ErrEIRNonConvergent returns 422 EIR_NON_CONVERGENT.
func ErrEIRNonConvergent(residual decimal.Decimal) *domainerrors.DomainError {
	return errEIR(CodeEIRNonConvergent,
		"Newton-Raphson tidak konvergen dalam 100 iterasi. Periksa cashflow projection.",
		domainerrors.Detail{
			Field:   "body.cashflowProjection",
			Rule:    "convergence",
			Message: "Residual terakhir: " + residual.String() + " melebihi tolerance 1e-10",
		})
}

// ErrEIRDivergent returns 422 EIR_DIVERGENT.
func ErrEIRDivergent(reason string) *domainerrors.DomainError {
	return errEIR(CodeEIRDivergent, "Solver EIR divergen. "+reason)
}

// ErrEIRCashflowInvalid returns 422 EIR_CASHFLOW_INVALID.
func ErrEIRCashflowInvalid(detail string) *domainerrors.DomainError {
	return errEIR(CodeEIRCashflowInvalid,
		"cashflowProjection tidak valid: "+detail,
		domainerrors.Detail{Field: "body.cashflowProjection", Rule: "required", Message: detail})
}

// ErrEIRCashflowSignMismatch returns 422 EIR_CASHFLOW_SIGN_MISMATCH.
func ErrEIRCashflowSignMismatch() *domainerrors.DomainError {
	return errEIR(CodeEIRCashflowSignMismatch,
		"cashflowProjection[0].amountIdr harus negatif (initial outflow/investment)",
		domainerrors.Detail{
			Field:   "body.cashflowProjection[0].amountIdr",
			Rule:    "sign_negative",
			Message: "CF_0 harus merepresentasikan initial investment (negatif)",
		})
}

// ErrEIRInstrumenFVTPLNoEIR returns 422 EIR_INSTRUMEN_FVTPL_NO_EIR.
func ErrEIRInstrumenFVTPLNoEIR(klasifikasi string) *domainerrors.DomainError {
	return errEIR(CodeEIRInstrumenFVTPLNoEIR,
		"EIR tidak berlaku untuk instrumen "+klasifikasi+". Hanya AC dan FVOCI debt yang menggunakan amortized cost.")
}

// ErrEIRInstrumenNotFound returns 404 EIR_INSTRUMEN_NOT_FOUND.
func ErrEIRInstrumenNotFound(instrumenID string) *domainerrors.DomainError {
	return errEIR(CodeEIRInstrumenNotFound,
		"Instrumen tidak ditemukan atau sudah dihapus",
		domainerrors.Detail{Field: "instrumenId", Rule: "exists",
			Message: "instrumenId tidak ada di mst.instrumen: " + instrumenID})
}

// ErrEIRAlreadyComputed returns 409 EIR_ALREADY_COMPUTED.
func ErrEIRAlreadyComputed() *domainerrors.DomainError {
	return errEIR(CodeEIRAlreadyComputed,
		"EIR sudah dihitung. Gunakan amendment flow untuk re-estimasi, atau set forceRecompute=true untuk koreksi origination.")
}

// ErrEIRNotYetComputed returns 422 EIR_NOT_YET_COMPUTED.
func ErrEIRNotYetComputed() *domainerrors.DomainError {
	return errEIR(CodeEIRNotYetComputed,
		"mst.instrumen.eir_awal IS NULL. Compute EIR terlebih dahulu sebelum generate schedule.",
		domainerrors.Detail{Field: "instrumen.eirAwal", Rule: "required", Message: "Nilai EIR belum tersedia"})
}

// ErrEIRDuplicateScheduleVersion returns 409 EIR_DUPLICATE_SCHEDULE_VERSION.
func ErrEIRDuplicateScheduleVersion(kode string) *domainerrors.DomainError {
	return errEIR(CodeEIRDuplicateScheduleVersion,
		"Instrumen "+kode+" sudah punya schedule aktif. Gunakan amendment workflow untuk re-estimasi.")
}

// ErrEIRScheduleNotFound returns 404 EIR_SCHEDULE_NOT_FOUND.
func ErrEIRScheduleNotFound(instrumenID string) *domainerrors.DomainError {
	return errEIR(CodeEIRScheduleNotFound,
		"Schedule EIR tidak ditemukan untuk instrumen ini",
		domainerrors.Detail{Field: "instrumenId", Rule: "schedule_required",
			Message: "Instrumen " + instrumenID + " belum punya schedule EIR"})
}

// ErrEIRPOCIRequiresPDAdjustedCF returns 422 EIR_POCI_REQUIRES_PD_ADJUSTED_CF.
func ErrEIRPOCIRequiresPDAdjustedCF() *domainerrors.DomainError {
	return errEIR(CodeEIRPOCIRequiresPDAdjustedCF,
		"Instrumen POCI membutuhkan cashflow yang sudah PD-adjusted. Set pociMode=true dan sediakan cashflow PD-adjusted.")
}

// ErrEIRBulkRecomputeInvalidScope returns 400 EIR_BULK_RECOMPUTE_INVALID_SCOPE.
func ErrEIRBulkRecomputeInvalidScope(scope string) *domainerrors.DomainError {
	return errEIR(CodeEIRBulkRecomputeInvalidScope,
		"scope '"+scope+"' tidak valid. Harus ALL_ACTIVE atau SUBSET dengan instrumenIds yang valid.")
}

// ErrEIRAmendNotFound returns 404 EIR_AMEND_NOT_FOUND.
func ErrEIRAmendNotFound(amendID string) *domainerrors.DomainError {
	return errEIR(CodeEIRAmendNotFound,
		"Amendment proposal tidak ditemukan: "+amendID)
}

// ErrEIRAmendActiveExists returns 409 EIR_AMEND_ACTIVE_EXISTS.
func ErrEIRAmendActiveExists(instrumenID string) *domainerrors.DomainError {
	return errEIR(CodeEIRAmendActiveExists,
		"Sudah ada amendment proposal aktif untuk instrumen "+instrumenID+". Selesaikan atau tolak proposal tersebut sebelum membuat yang baru.")
}

// ErrEIRAmendInvalidTransition returns 422 EIR_AMEND_INVALID_TRANSITION.
func ErrEIRAmendInvalidTransition(from, to string) *domainerrors.DomainError {
	return errEIR(CodeEIRAmendInvalidTransition,
		fmt.Sprintf("Transisi amendment dari '%s' ke '%s' tidak valid.", from, to),
		domainerrors.Detail{Field: "status", Rule: "invalid_transition",
			Message: from + " → " + to + " tidak valid"})
}

// ErrEIRMFAStepUpRequired returns 403 EIR_MFA_STEP_UP_REQUIRED.
func ErrEIRMFAStepUpRequired() *domainerrors.DomainError {
	return errEIR(CodeEIRMFAStepUpRequired,
		"Approve amendment EIR membutuhkan step-up MFA. Sertakan X-Step-Up-Token dari /auth/step-up.")
}

// ─── CashflowItem ─────────────────────────────────────────────────────────────

// CashflowItem is one element of the cashflow projection fed to the EIR solver.
// CF[0] must be negative (initial outflow / investment).
// Subsequent CFs are positive inflows (coupon + principal repayments).
// AmountIDR uses shopspring/decimal — NEVER float64 (DEC-016).
type CashflowItem struct {
	Date      time.Time       // payment date
	AmountIDR decimal.Decimal // NUMERIC(20,4); negative = outflow
}

// SolveDetail carries Newton-Raphson solver metadata for audit and debugging.
type SolveDetail struct {
	// IterationsUsed is the number of NR iterations performed.
	IterationsUsed int
	// ConvergenceResidual is |f(r)| at the last iteration.
	ConvergenceResidual decimal.Decimal
	// Converged indicates whether the solver met the tolerance criterion.
	Converged bool
}

// ─── ScheduleRow ──────────────────────────────────────────────────────────────

// ScheduleRow is one row in ecl.eir_amortization_schedule.
// All decimal fields: IDR = NUMERIC(20,4); EIRPeriode = NUMERIC(10,8).
// Immutable after INSERT (DB trigger tg_eir_schedule_amounts_immutable + service-level guard).
// Ref: db/migrations/000026_eir_schema_fix.up.sql §A-6.
type ScheduleRow struct {
	ID                 uuid.UUID       `db:"id"`
	InstrumenID        uuid.UUID       `db:"instrumen_id"`
	PeriodeSeq         int             `db:"periode_seq"`
	TanggalPosting     time.Time       `db:"tanggal_posting"`
	OpeningCarrying    decimal.Decimal `db:"opening_carrying"`     // NUMERIC(20,4)
	CashInflow         decimal.Decimal `db:"cash_inflow"`          // NUMERIC(20,4)
	PendapatanBungaEIR decimal.Decimal `db:"pendapatan_bunga_eir"` // NUMERIC(20,4)
	AmortisasiPD       decimal.Decimal `db:"amortisasi_p_d"`       // NUMERIC(20,4)
	PelunasanPokok     decimal.Decimal `db:"pelunasan_pokok"`      // NUMERIC(20,4)
	ClosingCarrying    decimal.Decimal `db:"closing_carrying"`     // NUMERIC(20,4)
	EIRPeriode         decimal.Decimal `db:"eir_periode"`          // NUMERIC(10,8)
	StageSaatPosting   string          `db:"stage_saat_posting"`
	StatusPosting      string          `db:"status_posting"`
	RecomputedFromSeq  *int            `db:"recomputed_from_seq"` // nil = active row; set = superseded
	FlagPOCI           bool            `db:"flag_poci"`
	// Audit columns (from migration 000026)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	TenantID   string     `db:"tenant_id"`
	RowVersion int64      `db:"row_version"`
}

// ─── ComputeResult ────────────────────────────────────────────────────────────

// ComputeResult is the response of Service.Compute.
type ComputeResult struct {
	InstrumenID         uuid.UUID
	EIRPerPeriod        decimal.Decimal // NUMERIC(10,8) — r per period
	EIRAnnualEquivalent decimal.Decimal // NUMERIC(10,8) — (1+r)^periodesPerYear - 1
	IterationsUsed      int
	ConvergenceResidual decimal.Decimal
	FlagPOCI            bool
	EIRType             string // "STANDARD" or "CREDIT_ADJUSTED"
	Persisted           bool   // true if eir_awal saved to mst.instrumen
	ComputedAt          time.Time
}

// EIR type constants.
const (
	EIRTypeStandard       = "STANDARD"
	EIRTypeCreditAdjusted = "CREDIT_ADJUSTED"
)

// ─── ScheduleGenerateResult ───────────────────────────────────────────────────

// ScheduleGenerateResult is the response of ScheduleService.Generate.
type ScheduleGenerateResult struct {
	InstrumenID          uuid.UUID
	TotalRows            int
	EIRPerPeriod         decimal.Decimal
	OpeningCarryingFirst decimal.Decimal
	ClosingCarryingLast  decimal.Decimal
	ClosingRoundingDelta decimal.Decimal // abs(closing_carrying of last row); should be ~0
	GeneratedAt          time.Time
	Rows                 []ScheduleRow
}

// ─── Amendment types ──────────────────────────────────────────────────────────

// AmendmentStatus mirrors the workflow_status CHECK constraint in ecl.eir_reestimation_log.
// Values: DRAFT | PENDING_REVIEW | PENDING_APPROVAL | APPROVED | REJECTED.
type AmendmentStatus string

const (
	AmendStatusDraft           AmendmentStatus = "DRAFT"
	AmendStatusPendingReview   AmendmentStatus = "PENDING_REVIEW"
	AmendStatusPendingApproval AmendmentStatus = "PENDING_APPROVAL"
	AmendStatusApproved        AmendmentStatus = "APPROVED"
	AmendStatusRejected        AmendmentStatus = "REJECTED"
	// AmendStatusCancelled: added P4-M6. Maker can cancel DRAFT or PENDING_REVIEW (no reviewer signed).
	// State machine: docs/state-machines/p4-m6-amendment-lifecycle.md §4.
	AmendStatusCancelled AmendmentStatus = "CANCELLED"
)

// IsTerminal returns true for APPROVED, REJECTED, or CANCELLED (no further transitions allowed).
// CANCELLED added in P4-M6 (story M6-005).
func (s AmendmentStatus) IsTerminal() bool {
	return s == AmendStatusApproved || s == AmendStatusRejected || s == AmendStatusCancelled
}

// AmendmentProposal mirrors ecl.eir_reestimation_log with all columns
// added by migrations 000001 + 000026.
//
// DB columns (authoritative): see db/migrations/000001_init_schema.up.sql:1197
// and db/migrations/000026_eir_schema_fix.up.sql §B.
type AmendmentProposal struct {
	ID                    uuid.UUID        // ecl.eir_reestimation_log.id
	InstrumenID           uuid.UUID        // instrumen_id FK → mst.instrumen
	TanggalAmandemen      time.Time        // tanggal_re_estimation DATE (business date of contract modification)
	TanggalReEstimasi     time.Time        // tanggal_re_estimation copy used for audit
	AlasanAmandemen       string           // stored in modifikasi_terms_json → key "alasan"
	RevisedCashflowJSON   string           // modifikasi_terms_json as serialized cashflows
	EIRLama               *decimal.Decimal // eir_sebelum NUMERIC(10,8) — old EIR before amendment
	EIRBaru               *decimal.Decimal // eir_sesudah NUMERIC(10,8) — new EIR after approval; nil until APPROVED
	CarryingSebelum       *decimal.Decimal // carrying_sebelum NUMERIC(20,4)
	CarryingSesudah       *decimal.Decimal // carrying_sesudah NUMERIC(20,4)
	CatchUpAdjustment     *decimal.Decimal // catch_up_adjustment NUMERIC(20,4)
	DokumenPendukungID    *uuid.UUID       // dokumen_pendukung_id FK → doc.upload
	MakerID               *uuid.UUID       // maker_id FK → sec.user
	ReviewerID            *uuid.UUID       // reviewer_id FK → sec.user (set at Review step)
	ApproverID            *uuid.UUID       // approver_id FK → sec.user (set at Approve step)
	Status                AmendmentStatus  // workflow_status
	ReviewerComment       *string          // reviewer_comment added in migration 000026
	ApproverComment       *string          // approver_comment added in migration 000026
	RejectReason          *string          // reject_reason added in migration 000026
	ReviewerSignatureHash *string          // reviewer_signature_hash added in migration 000026
	ApproverSignatureHash *string          // approver_signature_hash added in migration 000026
	ReviewedAt            *time.Time       // derived (not stored directly; latest updated_at when status=PENDING_APPROVAL)
	ApprovedAt            *time.Time       // approved_at TIMESTAMPTZ
	RejectedAt            *time.Time       // rejected_at added in migration 000026
	// Audit columns (from migration 000026)
	CreatedAt  time.Time
	CreatedBy  uuid.UUID
	UpdatedAt  time.Time
	UpdatedBy  uuid.UUID
	TenantID   string
	RowVersion int64
	// M6 additions (migration 000027)
	CancelledAt   *time.Time         // cancelled_at TIMESTAMPTZ; nil until CANCELLED
	CancelReason  *string            // cancel_reason TEXT; min 20 chars
	CancelledBy   *uuid.UUID         // cancelled_by UUID FK→sec.user
	TriggerSource AmendTriggerSource // trigger_source: MANUAL|DOCUMENT_UPLOAD|DRIFT_DETECTION_AUTO|PRE_ECL_GATE
	DriftReportID *uuid.UUID         // drift_report_id FK→sys.drift_report; set for DRIFT_DETECTION_AUTO
	DocumentID    *uuid.UUID         // document_id FK→doc.document; set for DOCUMENT_UPLOAD
}

// ProposeRequest is the input for AmendmentService.Propose.
type ProposeRequest struct {
	InstrumenID               uuid.UUID
	TanggalAmandemen          time.Time
	RevisedCashflowProjection []CashflowItem // revised cashflows after contract modification
	AlasanAmandemen           string         // reason for amendment (stored in modifikasi_terms_json)
	DokumenPendukungID        *uuid.UUID     // supporting document reference
}

// ReviewRequest is the input for AmendmentService.Review.
type ReviewRequest struct {
	AmendmentID uuid.UUID
	Comment     string
}

// ApproveRequest is the input for AmendmentService.Approve.
type ApproveRequest struct {
	AmendmentID uuid.UUID
	Comment     string
	StepUpToken string // X-Step-Up-Token from /auth/step-up (DEC-027)
}

// WorkflowAction is a generic reject/approve request for AmendmentService.Reject.
type WorkflowAction struct {
	AmendmentID uuid.UUID
	Comment     string
}

// ─── P4-M6 permission constants ───────────────────────────────────────────────

const (
	// PermEIRAmendDetect: trigger detect-from-document (ROLE-RISK, ROLE-AKUN).
	PermEIRAmendDetect = "eir.amendment.detect"
	// PermEIRAmendCancel: cancel own DRAFT / unsigned PENDING_REVIEW (ROLE-AKUN, owner).
	PermEIRAmendCancel = "eir.amendment.cancel"
	// PermEIRAmendReviewRead: read the amendment review queue (ROLE-RISK, ROLE-ALCO, ROLE-AUDIT).
	PermEIRAmendReviewRead = "eir.amendment_review.read"
	// PermEIRDriftReportRead: read drift reports (ROLE-RISK, ROLE-AUDIT, ROLE-ALCO).
	PermEIRDriftReportRead = "eir.drift_report.read"
	// PermEIRAmendUpdateCF: update cashflows on DRAFT amendment (ROLE-AKUN, owner).
	PermEIRAmendUpdateCF = "eir.amendment.update_cashflows"
	// PermEIRDriftGenerate: trigger ad-hoc drift generation (ROLE-RISK, ROLE-IT-ADMIN).
	PermEIRDriftGenerate = "eir.drift_report.generate"
)

// ─── P4-M6 error codes (matching domain.go CodeEIR* constants) ────────────────

const (
	// CodeEIRAmendmentDetectionNoMatch: 422 — doc type wrong / proposal active / ineligible.
	CodeEIRAmendmentDetectionNoMatch = "EIR_AMENDMENT_DETECTION_NO_MATCH"
	// CodeEIRAmendmentCancelForbidden: 403 — caller not maker, or reviewer already signed.
	CodeEIRAmendmentCancelForbidden = "EIR_AMENDMENT_CANCEL_FORBIDDEN"
	// CodeEIRAmendmentCancelReasonShort: 422 — cancelReason < 20 chars (DB CHECK constraint).
	CodeEIRAmendmentCancelReasonShort = "EIR_AMENDMENT_CANCEL_REASON_TOO_SHORT"
	// CodeEIRDriftReportNotFound: 404 — sys.drift_report row not found.
	CodeEIRDriftReportNotFound = "EIR_DRIFT_REPORT_NOT_FOUND"
	// CodeEIRDriftReportPeriodeOutOfRange: 422 — periode query param out of data range.
	CodeEIRDriftReportPeriodeOutOfRange = "EIR_DRIFT_REPORT_PERIODE_OUT_OF_RANGE"
	// CodeEIRDriftGenerationInProgress: 409 — a concurrent drift job is still running.
	CodeEIRDriftGenerationInProgress = "EIR_DRIFT_GENERATION_IN_PROGRESS"
	// CodeEIRDriftThresholdInvalid: 422 — sys.parameter threshold row malformed.
	CodeEIRDriftThresholdInvalid = "EIR_DRIFT_THRESHOLD_INVALID"
)

// M6 error constructors.

// ErrEIRAmendmentDetectionNoMatch returns 422 EIR_AMENDMENT_DETECTION_NO_MATCH.
func ErrEIRAmendmentDetectionNoMatch(reason string) *domainerrors.DomainError {
	return errEIR(CodeEIRAmendmentDetectionNoMatch,
		"Deteksi amandemen tidak cocok: "+reason)
}

// ErrEIRAmendmentCancelForbidden returns 403 EIR_AMENDMENT_CANCEL_FORBIDDEN.
func ErrEIRAmendmentCancelForbidden(reason string) *domainerrors.DomainError {
	return errEIR(CodeEIRAmendmentCancelForbidden,
		"Pembatalan tidak diizinkan: "+reason)
}

// ErrEIRAmendmentCancelReasonShort returns 422 EIR_AMENDMENT_CANCEL_REASON_TOO_SHORT.
func ErrEIRAmendmentCancelReasonShort() *domainerrors.DomainError {
	return errEIR(CodeEIRAmendmentCancelReasonShort,
		"cancelReason minimal 20 karakter (DB CHECK constraint)",
		domainerrors.Detail{Field: "body.cancelReason", Rule: "min_length", Message: "min 20 karakter"})
}

// ErrEIRDriftReportNotFound returns 404 EIR_DRIFT_REPORT_NOT_FOUND.
func ErrEIRDriftReportNotFound(reportID string) *domainerrors.DomainError {
	return errEIR(CodeEIRDriftReportNotFound,
		"Drift report tidak ditemukan: "+reportID)
}

// ErrEIRDriftReportPeriodeOutOfRange returns 422.
func ErrEIRDriftReportPeriodeOutOfRange(periode string) *domainerrors.DomainError {
	return errEIR(CodeEIRDriftReportPeriodeOutOfRange,
		"Periode '"+periode+"' tidak ada dalam data drift report",
		domainerrors.Detail{Field: "query.periode", Rule: "in_range", Message: "No drift reports for this periode"})
}

// ErrEIRDriftGenerationInProgress returns 409 EIR_DRIFT_GENERATION_IN_PROGRESS.
func ErrEIRDriftGenerationInProgress(jobID string) *domainerrors.DomainError {
	return errEIR(CodeEIRDriftGenerationInProgress,
		"Drift detection sedang berjalan (job "+jobID+"). Tunggu selesai sebelum trigger yang baru.")
}

// ErrEIRDriftThresholdInvalid returns 422 EIR_DRIFT_THRESHOLD_INVALID.
func ErrEIRDriftThresholdInvalid(reason string) *domainerrors.DomainError {
	return errEIR(CodeEIRDriftThresholdInvalid,
		"sys.parameter threshold tidak valid: "+reason)
}

// ─── P4-M6 DriftReport types ──────────────────────────────────────────────────

// DriftTriggerSource values for sys.drift_report.trigger_source.
// CHECK constraint: ('CRON_DAILY','MANUAL_AD_HOC','PRE_ECL_CALC_RUN').
// See db/migrations/000027_drift_report_and_amendment_lifecycle.up.sql.
type DriftTriggerSource string

const (
	DriftTriggerCronDaily     DriftTriggerSource = "CRON_DAILY"
	DriftTriggerManualAdHoc   DriftTriggerSource = "MANUAL_AD_HOC"
	DriftTriggerPreECLCalcRun DriftTriggerSource = "PRE_ECL_CALC_RUN"
)

// DriftReportStatus values for sys.drift_report.status.
// CHECK constraint: ('IN_PROGRESS','COMPLETED','FAILED').
type DriftReportStatus string

const (
	DriftStatusInProgress DriftReportStatus = "IN_PROGRESS"
	DriftStatusCompleted  DriftReportStatus = "COMPLETED"
	DriftStatusFailed     DriftReportStatus = "FAILED"
)

// DriftSeverity classifies the magnitude of EIR drift.
// LOW: abs_diff > drift_low_threshold (default 0.0001 = 1bp) — flag only.
// HIGH: abs_diff > drift_high_threshold (default 0.001 = 10bp) — auto-create proposal.
type DriftSeverity string

const (
	DriftSeverityLow  DriftSeverity = "LOW"
	DriftSeverityHigh DriftSeverity = "HIGH"
)

// DriftReport mirrors sys.drift_report schema from migration 000027.
// All decimal fields use shopspring/decimal (never float64).
type DriftReport struct {
	ID                   uuid.UUID          `db:"id"`
	TanggalRun           time.Time          `db:"tanggal_run"` // DATE
	TriggerSource        DriftTriggerSource `db:"trigger_source"`
	TriggeredBy          *uuid.UUID         `db:"triggered_by"` // nullable for CRON
	AsynqJobID           *string            `db:"asynq_job_id"`
	Status               DriftReportStatus  `db:"status"`
	StartedAt            *time.Time         `db:"started_at"`
	CompletedAt          *time.Time         `db:"completed_at"`
	TotalInstrumen       int                `db:"total_instrumen"`
	DriftLowCount        int                `db:"drift_low_count"`
	DriftHighCount       int                `db:"drift_high_count"`
	MissingScheduleCount int                `db:"missing_schedule_count"`
	ErrorCount           int                `db:"error_count"`
	ErrorSummary         *string            `db:"error_summary"`
	DriftFlagThreshold   decimal.Decimal    `db:"drift_flag_threshold"` // NUMERIC(10,8) default 0.0001
	DriftHighThreshold   decimal.Decimal    `db:"drift_high_threshold"` // NUMERIC(10,8) default 0.001
	// Audit columns
	CreatedAt  time.Time `db:"created_at"`
	CreatedBy  uuid.UUID `db:"created_by"`
	UpdatedAt  time.Time `db:"updated_at"`
	UpdatedBy  uuid.UUID `db:"updated_by"`
	TenantID   string    `db:"tenant_id"`
	RowVersion int64     `db:"row_version"`
}

// DriftReportEntry is one instrument result row for the drift report detail.
// Stored as drift_entries_json in sys.drift_report or computed in-process.
type DriftReportEntry struct {
	InstrumenID     uuid.UUID       `json:"instrumen_id"`
	KodeInstrumen   string          `json:"kode_instrumen"`
	EIRAwal         decimal.Decimal `json:"eir_awal"`       // NUMERIC(10,8)
	EIRRecomputed   decimal.Decimal `json:"eir_recomputed"` // NUMERIC(10,8)
	AbsDiff         decimal.Decimal `json:"abs_diff"`       // NUMERIC(10,8)
	BasisPoints     decimal.Decimal `json:"basis_points"`
	Severity        DriftSeverity   `json:"severity"`
	ProposalCreated bool            `json:"proposal_created"` // true if HIGH auto-created proposal
	ProposalID      *uuid.UUID      `json:"proposal_id,omitempty"`
}

// DriftMissingEntry records an instrument with no active schedule during drift run.
type DriftMissingEntry struct {
	InstrumenID   uuid.UUID `json:"instrumen_id"`
	KodeInstrumen string    `json:"kode_instrumen"`
	Reason        string    `json:"reason"` // "eir_awal IS NULL" | "No active schedule rows"
}

// DriftErrorEntry records an instrument that errored during drift run.
type DriftErrorEntry struct {
	InstrumenID   uuid.UUID `json:"instrumen_id"`
	KodeInstrumen string    `json:"kode_instrumen"`
	ErrorCode     string    `json:"error_code"`
	ErrorMessage  string    `json:"error_message"`
}

// DriftReportResult is the fully populated detail for a single drift report,
// returned by DriftService.GetReport with all embedded entry slices.
type DriftReportResult struct {
	Report         DriftReport
	DriftEntries   []DriftReportEntry
	MissingEntries []DriftMissingEntry
	ErrorEntries   []DriftErrorEntry
}

// DriftGenerateRequest is the input for DriftService.GenerateReport (ad-hoc / pre-ECL trigger).
type DriftGenerateRequest struct {
	TriggerSource DriftTriggerSource
	TriggeredBy   *uuid.UUID // nil for CRON
	TenantID      string
}

// AmendTriggerSource for ecl.eir_reestimation_log.trigger_source (M6 column).
// CHECK constraint: ('MANUAL','DOCUMENT_UPLOAD','DRIFT_DETECTION_AUTO','PRE_ECL_GATE').
// See migration 000027.
type AmendTriggerSource string

const (
	AmendTriggerManual             AmendTriggerSource = "MANUAL"
	AmendTriggerDocumentUpload     AmendTriggerSource = "DOCUMENT_UPLOAD"
	AmendTriggerDriftDetectionAuto AmendTriggerSource = "DRIFT_DETECTION_AUTO"
	AmendTriggerPreECLGate         AmendTriggerSource = "PRE_ECL_GATE"
)

// DetectAmendmentRequest is the input for DetectionService.DetectFromDocument (M6-001).
type DetectAmendmentRequest struct {
	DocumentID  uuid.UUID // doc.document.id (uploaded contract amendment PDF/XLSX)
	InstrumenID uuid.UUID // target instrument
	ActorID     uuid.UUID
	TenantID    string
	// Optional overrides from document parsing (doc type check done by service).
	AlasanDetected    string
	OverrideCashflows []CashflowItem // if document contains parsed cashflows
}

// CancelAmendmentRequest is the input for DetectionService.CancelAmendment (M6-005).
type CancelAmendmentRequest struct {
	AmendmentID  uuid.UUID
	CancelReason string // min 20 chars per DB CHECK
	ActorID      uuid.UUID
	TenantID     string
}

// UpdateCashflowsRequest is the input for DetectionService.UpdateCashflows (M6-003 PATCH).
type UpdateCashflowsRequest struct {
	AmendmentID      uuid.UUID
	RevisedCashflows []CashflowItem
	ActorID          uuid.UUID
	TenantID         string
}

// QueueRow is a lightweight row returned by the amendment review queue list endpoint.
type QueueRow struct {
	AmendmentID      uuid.UUID
	InstrumenID      uuid.UUID
	KodeInstrumen    string
	Status           AmendmentStatus
	TriggerSource    AmendTriggerSource
	AbsDiff          *decimal.Decimal // nil if not yet recomputed
	EIRLama          *decimal.Decimal
	DriftReportID    *uuid.UUID
	DocumentID       *uuid.UUID
	MakerID          *uuid.UUID
	ReviewerID       *uuid.UUID
	TanggalAmandemen time.Time
	CreatedAt        time.Time
}

// ─── Bulk types ───────────────────────────────────────────────────────────────

// BulkScope controls which instruments are re-computed in bulk.
type BulkScope string

const (
	BulkScopeAllActive BulkScope = "ALL_ACTIVE"
	BulkScopeSubset    BulkScope = "SUBSET"
)

// BulkRecomputeResult aggregates the outcome of a bulk re-compute job (report-only).
type BulkRecomputeResult struct {
	JobID            string
	Scope            BulkScope
	RunAt            time.Time
	TotalInstruments int
	ProcessedOK      int
	DriftCount       int
	MissingCount     int
	ErrorCount       int
	Canceled         bool
	ElapsedMs        int64
	Drifts           []DriftEntry
	Missing          []MissingScheduleEntry
	Errors           []BulkErrorEntry
}

// DriftEntry describes one instrument where stored EIR differs from re-computed EIR.
// Threshold: |EIR_stored - EIR_recomputed| > 1 bp (0.0001).
type DriftEntry struct {
	InstrumenID   uuid.UUID
	KodeInstrumen string
	EIRAwal       decimal.Decimal // stored in mst.instrumen.eir_awal
	EIRRecomputed decimal.Decimal // NR result from schedule reconstruction
	AbsDiff       decimal.Decimal
	BasisPoints   decimal.Decimal // AbsDiff × 10000, rounded to 2dp
}

// MissingScheduleEntry describes one instrument with no active schedule.
type MissingScheduleEntry struct {
	InstrumenID   uuid.UUID
	KodeInstrumen string
	Reason        string // "eir_awal IS NULL" or "No active schedule rows"
}

// BulkErrorEntry describes one instrument that caused an error during bulk.
type BulkErrorEntry struct {
	InstrumenID   uuid.UUID
	KodeInstrumen string
	ErrorCode     string
	ErrorMessage  string
}

// BulkRecomputePayload is the Asynq job payload for TypeEIRBulkRecompute.
type BulkRecomputePayload struct {
	JobID        string      `json:"job_id"`
	Scope        BulkScope   `json:"scope"`
	InstrumenIDs []uuid.UUID `json:"instrumen_ids,omitempty"`
	PeriodeID    *uuid.UUID  `json:"periode_id,omitempty"`
	Reason       string      `json:"reason,omitempty"`
	TenantID     string      `json:"tenant_id"`
	ActorID      string      `json:"actor_id"` // UUID as string for JSON marshaling
}

// ─── InstrumenForEIR ──────────────────────────────────────────────────────────

// InstrumenForEIR is a projection of mst.instrumen with only the fields needed by EIR services.
// Loaded by InstrumenEIRRepo.GetByID.
type InstrumenForEIR struct {
	ID                        uuid.UUID
	KodeInstrumen             string
	KlasifikasiPsak71         string           // AC | FVOCI | FVTPL | FVOCI_ELECTION
	EIRMethodFlag             bool             // mst.instrumen.eir_method_flag
	EIRAwal                   *decimal.Decimal // NUMERIC(10,8); nil = not yet computed
	FlagPOCI                  bool             // BOOLEAN DEFAULT false (migration 000026 §D)
	Nominal                   decimal.Decimal  // NUMERIC(20,4)
	BiayaTransaksiCapitalized decimal.Decimal  // NUMERIC(20,4); initial transaction cost
	Kupon                     *decimal.Decimal // NUMERIC(7,4); annual coupon rate; nil for ZCB
	TanggalPenempatan         time.Time        // DATE; origin date for ACT/365 period calculation
	TanggalJatuhTempo         *time.Time       // DATE
	Status                    string           // ACTIVE | MATURED | SOLD | etc.
	DeletedAt                 *time.Time
	TenantID                  string
}

// ─── Allowed column whitelists ────────────────────────────────────────────────

// AllowedColsSchedule are the allowed sort/filter columns for schedule DataTable.
// Used by listquery.WithAllowed to prevent SQL injection.
var AllowedColsSchedule = []string{
	"periode_seq", "tanggal_posting", "opening_carrying", "closing_carrying",
	"pendapatan_bunga_eir", "amortisasi_p_d", "eir_periode",
	"status_posting", "recomputed_from_seq",
}

// AllowedColsAmendment are the allowed sort/filter columns for amendment list DataTable.
var AllowedColsAmendment = []string{
	"created_at", "tanggal_re_estimation", "workflow_status",
	"eir_sebelum", "eir_sesudah", "instrumen_id",
}

// AllowedColsQueue are the allowed sort/filter columns for the amendment review queue (M6-004).
var AllowedColsQueue = []string{
	"created_at", "tanggal_re_estimation", "workflow_status",
	"trigger_source", "instrumen_id", "abs_diff", "drift_report_id",
}

// AllowedColsDriftReport are the allowed sort/filter columns for the drift report list (M6-002).
var AllowedColsDriftReport = []string{
	"tanggal_run", "trigger_source", "status",
	"drift_high_count", "drift_low_count", "total_instrumen",
	"created_at",
}

// ─── Signature hash ───────────────────────────────────────────────────────────

// ComputeReviewerSignatureHash computes SHA-256 hex of:
//
//	proposalID || reviewerID || comment
//
// Stored in ecl.eir_reestimation_log.reviewer_signature_hash (DEC-018, state-machine §3).
func ComputeReviewerSignatureHash(proposalID uuid.UUID, reviewerID uuid.UUID, comment string) string {
	payload := proposalID.String() + reviewerID.String() + comment
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum[:])
}

// ComputeApproverSignatureHash computes SHA-256 hex of:
//
//	proposalID || approverID || comment || newEIR
//
// Stored in ecl.eir_reestimation_log.approver_signature_hash (DEC-018, DEC-027).
func ComputeApproverSignatureHash(proposalID uuid.UUID, approverID uuid.UUID, comment string, newEIR decimal.Decimal) string {
	payload := proposalID.String() + approverID.String() + comment + newEIR.StringFixed(8)
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("%x", sum[:])
}

// ─── JSON helpers ─────────────────────────────────────────────────────────────

// cashflowItemJSON is the JSON representation of CashflowItem (for JSONB storage).
type cashflowItemJSON struct {
	Date      string `json:"date"`       // RFC3339
	AmountIDR string `json:"amount_idr"` // StringFixed(4)
}

// marshalCashflows serializes []CashflowItem to a JSON string for storage in modifikasi_terms_json.
func marshalCashflows(cfs []CashflowItem) (string, error) {
	items := make([]cashflowItemJSON, len(cfs))
	for i, cf := range cfs {
		items[i] = cashflowItemJSON{
			Date:      cf.Date.UTC().Format(time.RFC3339),
			AmountIDR: cf.AmountIDR.StringFixed(4),
		}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshalCashflows: %w", err)
	}
	return string(b), nil
}

// unmarshalCashflows parses a JSON string back to []CashflowItem.
func unmarshalCashflows(jsonStr string) ([]CashflowItem, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var items []cashflowItemJSON
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, fmt.Errorf("unmarshalCashflows: %w", err)
	}
	cfs := make([]CashflowItem, len(items))
	for i, item := range items {
		t, err := time.Parse(time.RFC3339, item.Date)
		if err != nil {
			return nil, fmt.Errorf("unmarshalCashflows[%d].date: %w", i, err)
		}
		amt, err := decimal.NewFromString(item.AmountIDR)
		if err != nil {
			return nil, fmt.Errorf("unmarshalCashflows[%d].amount_idr: %w", i, err)
		}
		cfs[i] = CashflowItem{Date: t, AmountIDR: amt}
	}
	return cfs, nil
}

// ─── Internal utilities ───────────────────────────────────────────────────────

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }

// decPtr returns a pointer to d.
func decPtr(d decimal.Decimal) *decimal.Decimal { return &d }

// uuidPtr returns a pointer to u.
func uuidPtr(u uuid.UUID) *uuid.UUID { return &u }

// NewDomainError proxies domainerrors.NewDomainError for use in this package.
// Added to avoid direct import from amendment_service.go.
func NewDomainError(code domainerrors.Code, msg string) *domainerrors.DomainError {
	return domainerrors.New(code, msg)
}
