// Package staging implements the ECL Staging Engine (APP-C-STG-001..005).
//
// IFRS9 / PSAK 71 §5.5 staging logic:
//   - Stage 1: performing, 12-month PD, interest on Gross Carrying Amount.
//   - Stage 2: significant credit risk increase (SICR), Lifetime PD,
//     interest on Gross Carrying Amount.
//   - Stage 3: credit-impaired, PD = 1.0, interest on Net Carrying Amount
//     (Gross - ECL).
//
// Decision log refs:
//   - DEC-010: 3-stage ECL model.
//   - DEC-011: SICR triggers: rating downgrade ≥ 2 notch OR IG→non-IG OR DPD ≥ 30.
//   - DEC-012: Cure: 3 consecutive mst.periode_buku BULANAN without SICR trigger.
//   - DEC-016: No float64 for money/rates. DPD and notch delta are int (not money).
//   - DEC-017: 6-eyes for Stage 3 override (RISK + RISK + ALCO + KOMITE).
//   - DEC-018: ecl.stage_history append-only.
//   - DEC-021: Idempotency-Key wajib di POST endpoints.
//   - DEC-026: Step-up MFA for ALCO + KOMITE approve.
//
// See: state-machine docs/state-machines/p4-m1-staging.md §1-§2.
// See: stories docs/stories/phase-4/M1-staging-engine.md.
// See: migration db/migrations/000022_staging_engine_tables.up.sql.
package staging

import (
	"crypto/sha256"
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeStagingEvalInstrumenNotFound: instrumenId not found or soft-deleted.
	// State machine §6, HTTP 404.
	CodeStagingEvalInstrumenNotFound = "STAGING_EVAL_INSTRUMEN_NOT_FOUND"

	// CodeStagingOverrideInvalidTransition: stage transition disallowed.
	// State machine §6, HTTP 422.
	CodeStagingOverrideInvalidTransition = "STAGING_OVERRIDE_INVALID_TRANSITION"

	// CodeStagingDPDMissing: DPD record not found for instrument + periode.
	// State machine §6, HTTP 422.
	CodeStagingDPDMissing = "STAGING_DPD_MISSING"

	// CodeStagingCurePeriodeInsufficient: fewer than 3 consecutive closed periods.
	// State machine §6, HTTP 422.
	CodeStagingCurePeriodeInsufficient = "STAGING_CURE_PERIODE_INSUFFICIENT"

	// CodeStagingOverrideExpired: override proposal already expired.
	// State machine §6, HTTP 410.
	CodeStagingOverrideExpired = "STAGING_OVERRIDE_EXPIRED"

	// CodeStagingRatingBaselineMissing: no rating found at tanggal_penempatan.
	// State machine §6, HTTP 422.
	CodeStagingRatingBaselineMissing = "STAGING_RATING_BASELINE_MISSING"

	// CodeStagingCalcRunSealed: ECL calc run for this period already sealed.
	// State machine §6, HTTP 423.
	CodeStagingCalcRunSealed = "STAGING_CALC_RUN_SEALED"
)

// HTTPStatus returns the HTTP status code for a staging-specific error code string.
// Staging codes are not in the shared domainerrors.Code type; use this helper.
func stagingHTTPStatus(code string) int {
	switch code {
	case CodeStagingEvalInstrumenNotFound:
		return 404
	case CodeStagingOverrideInvalidTransition,
		CodeStagingDPDMissing,
		CodeStagingCurePeriodeInsufficient,
		CodeStagingRatingBaselineMissing:
		return 422
	case CodeStagingCalcRunSealed:
		return 423
	case CodeStagingOverrideExpired:
		return 410
	default:
		return 500
	}
}

// errStaging builds a DomainError with a staging-specific code string cast to domainerrors.Code.
// Callers relying on HTTP status should use ErrStaging* constructors below.
func errStaging(code string, msg string, details ...domainerrors.Detail) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.Code(code), msg, details...)
}

// ErrStagingInstrumenNotFound returns 404 STAGING_EVAL_INSTRUMEN_NOT_FOUND.
func ErrStagingInstrumenNotFound(instrumenID string) *domainerrors.DomainError {
	return errStaging(CodeStagingEvalInstrumenNotFound,
		"Instrumen tidak ditemukan: "+instrumenID,
		domainerrors.Detail{Field: "instrumenId", Rule: "exists", Message: "Instrumen tidak ditemukan atau sudah dihapus"})
}

// ErrStagingOverrideInvalidTransition returns 422 STAGING_OVERRIDE_INVALID_TRANSITION.
func ErrStagingOverrideInvalidTransition(from, to string) *domainerrors.DomainError {
	return errStaging(CodeStagingOverrideInvalidTransition,
		"Transisi stage tidak valid: "+from+" → "+to,
		domainerrors.Detail{Field: "stageTarget", Rule: "invalid_transition",
			Message: "Transisi " + from + " → " + to + " tidak diizinkan"})
}

// ErrStagingOverrideExpired returns 410 STAGING_OVERRIDE_EXPIRED.
func ErrStagingOverrideExpired() *domainerrors.DomainError {
	return errStaging(CodeStagingOverrideExpired,
		"Proposal override ini sudah kadaluarsa karena periode buku telah berakhir.")
}

// ErrStagingRatingBaselineMissing returns 422 STAGING_RATING_BASELINE_MISSING.
func ErrStagingRatingBaselineMissing(instrumenID string, tanggalPenempatan time.Time) *domainerrors.DomainError {
	return errStaging(CodeStagingRatingBaselineMissing,
		"Rating inisiasi instrumen tidak ditemukan untuk "+instrumenID+" pada "+tanggalPenempatan.Format("2006-01-02"),
		domainerrors.Detail{Field: "instrumenId", Rule: "rating_baseline_missing",
			Message: "Rating pada tanggal_penempatan tidak tersedia di mst.rating_history_counterparty"})
}

// ErrStagingCalcRunSealed returns 423 STAGING_CALC_RUN_SEALED.
func ErrStagingCalcRunSealed() *domainerrors.DomainError {
	return errStaging(CodeStagingCalcRunSealed,
		"ECL calc run periode ini sudah di-seal. Override baru tidak bisa diajukan.")
}

// ─── Stage type ───────────────────────────────────────────────────────────────

// Stage represents an ECL instrument stage per PSAK 71 §5.5.
// Values match the CHECK constraint in migration 000022.
type Stage string

const (
	Stage1 Stage = "STAGE_1"
	Stage2 Stage = "STAGE_2"
	Stage3 Stage = "STAGE_3"
)

// IsValid returns true if s is a defined stage.
func (s Stage) IsValid() bool {
	return s == Stage1 || s == Stage2 || s == Stage3
}

// String implements fmt.Stringer.
func (s Stage) String() string { return string(s) }

// ─── TriggerType ──────────────────────────────────────────────────────────────

// TriggerType identifies why a stage transition occurred.
// Values match the CHECK constraint in migration 000022.
type TriggerType string

const (
	TriggerRatingDowngrade    TriggerType = "RATING_DOWNGRADE"
	TriggerIGToNonIG          TriggerType = "IG_TO_NON_IG"
	TriggerRatingDefault      TriggerType = "RATING_DEFAULT"
	TriggerDPDGte30           TriggerType = "DPD_GTE_30"
	TriggerDPDGte90           TriggerType = "DPD_GTE_90"
	TriggerCure3PeriodeBulanan TriggerType = "CURE_3_PERIODE_BULANAN"
	TriggerManualOverride     TriggerType = "MANUAL_OVERRIDE"
	TriggerOverrideExpired    TriggerType = "OVERRIDE_EXPIRED"
	TriggerInitial            TriggerType = "INITIAL"
)

// IsValid returns true if t is a defined trigger.
func (t TriggerType) IsValid() bool {
	switch t {
	case TriggerRatingDowngrade, TriggerIGToNonIG, TriggerRatingDefault,
		TriggerDPDGte30, TriggerDPDGte90, TriggerCure3PeriodeBulanan,
		TriggerManualOverride, TriggerOverrideExpired, TriggerInitial:
		return true
	}
	return false
}

// ─── StatusApproval ───────────────────────────────────────────────────────────

// StatusApproval identifies who approved a stage_history row.
// Values match the CHECK constraint in migration 000022.
type StatusApproval string

const (
	StatusApprovalAuto            StatusApproval = "AUTO"
	StatusApprovalApproved        StatusApproval = "APPROVED"
	StatusApprovalOverrideExpired StatusApproval = "OVERRIDE_EXPIRED"
)

// ─── OverrideWorkflowStatus ───────────────────────────────────────────────────

// OverrideWorkflowStatus tracks the state machine for ecl.staging_override_proposal.
// Values match the CHECK constraint in migration 000022.
type OverrideWorkflowStatus string

const (
	OverrideStatusPendingReview  OverrideWorkflowStatus = "PENDING_REVIEW"
	OverrideStatusPendingApproval OverrideWorkflowStatus = "PENDING_APPROVAL"
	OverrideStatusApprovedALCO   OverrideWorkflowStatus = "APPROVED_ALCO"
	OverrideStatusActive         OverrideWorkflowStatus = "ACTIVE"
	OverrideStatusExpired        OverrideWorkflowStatus = "EXPIRED"
	OverrideStatusRejected       OverrideWorkflowStatus = "REJECTED"
)

// IsTerminal returns true if the status has reached a final state.
func (s OverrideWorkflowStatus) IsTerminal() bool {
	return s == OverrideStatusActive || s == OverrideStatusExpired || s == OverrideStatusRejected
}

// ─── Pefindo rating scale ─────────────────────────────────────────────────────

// pefindoScale is the Pefindo rating scale in ascending risk order (index = risk rank).
// Source: state-machine doc §1.3, story M1-staging-engine.md §"Notch Scale Pefindo".
// Index 0 = best (idAAA), Index 17 = worst (idD).
// DEC-011: delta_notch >= 2 → SICR.
var pefindoScale = []string{
	"idAAA",  // 0  — IG
	"idAA+",  // 1  — IG
	"idAA",   // 2  — IG
	"idAA-",  // 3  — IG
	"idA+",   // 4  — IG
	"idA",    // 5  — IG
	"idA-",   // 6  — IG
	"idBBB+", // 7  — IG
	"idBBB",  // 8  — IG
	"idBBB-", // 9  — IG  (boundary: last IG)
	"idBB+",  // 10 — non-IG (first non-IG)
	"idBB",   // 11 — non-IG
	"idBB-",  // 12 — non-IG
	"idB+",   // 13 — non-IG
	"idB",    // 14 — non-IG
	"idB-",   // 15 — non-IG
	"idCCC",  // 16 — non-IG
	"idD",    // 17 — default
}

// pefindoIndex maps rating string to its position in the scale.
// Initialized once at package init. Unknown ratings return -1 from RatingIndex().
var pefindoIndex map[string]int

func init() {
	pefindoIndex = make(map[string]int, len(pefindoScale))
	for i, r := range pefindoScale {
		pefindoIndex[r] = i
	}
}

// RatingIndex returns the 0-based risk rank of a Pefindo rating.
// Returns -1 if the rating is not in the scale (caller should treat as unknown).
func RatingIndex(rating string) int {
	if idx, ok := pefindoIndex[rating]; ok {
		return idx
	}
	return -1
}

// igBoundary is the index of idBBB- (last Investment Grade rating).
const igBoundary = 9

// IsInvestmentGrade returns true if the rating is in the IG tier (idAAA..idBBB-).
// Returns false for unknown ratings.
func IsInvestmentGrade(rating string) bool {
	idx := RatingIndex(rating)
	return idx >= 0 && idx <= igBoundary
}

// IsDefaultRating returns true if rating == "idD".
func IsDefaultRating(rating string) bool {
	return rating == "idD"
}

// DeltaNotch computes the signed notch change from ratingFrom to ratingTo.
// Positive value = downgrade (higher risk rank). Negative = upgrade.
// Returns 0 if either rating is unknown (treated as no change — caller validates).
//
// Formula: delta = index(ratingTo) - index(ratingFrom)
// Per IFRS9 §5.5.11: baseline = rating at initial recognition (tanggal_penempatan).
// Per state-machine doc §1.3: delta >= 2 → SICR.
func DeltaNotch(ratingFrom, ratingTo string) int {
	fromIdx := RatingIndex(ratingFrom)
	toIdx := RatingIndex(ratingTo)
	if fromIdx < 0 || toIdx < 0 {
		return 0
	}
	return toIdx - fromIdx
}

// ─── SICR logic ───────────────────────────────────────────────────────────────

// SICRResult holds the SICR evaluation outcome.
type SICRResult struct {
	// Triggered is true if any SICR condition is met.
	Triggered bool
	// TriggerType is the specific trigger that fired (only meaningful if Triggered=true).
	TriggerType TriggerType
	// DeltaNotch is the computed notch delta from origination.
	DeltaNotch int
	// IsDefault is true if the current rating = idD.
	IsDefault bool
	// Detail is a human-readable description of the trigger.
	Detail string
}

// EvaluateSICR applies the three DEC-011 conditions and returns the SICR result.
// Parameters:
//   - ratingAtOrigination: rating of the counterparty at instrument's tanggal_penempatan.
//   - ratingCurrent:       latest approved rating of the counterparty.
//   - ratingPrevious:      rating immediately before ratingCurrent (for IG→non-IG detection).
//   - dpdValue:            days-past-due (0 = current).
//
// The three SICR triggers (any one is sufficient per DEC-011):
//  1. Rating downgrade ≥ 2 notch from origination.
//  2. Rating moved from IG (≥ idBBB-) to non-IG (< idBBB-).
//  3. DPD ≥ 30 days.
//
// Default (idD or DPD ≥ 90) → stage 3 directly (handled in ComputeNewStage).
//
// Ref: stories §"Notch Scale Pefindo", state-machine §1.2.
func EvaluateSICR(ratingAtOrigination, ratingCurrent, ratingPrevious string, dpdValue int) SICRResult {
	var result SICRResult

	// Guard: default always overrides SICR check — handled separately.
	if IsDefaultRating(ratingCurrent) {
		result.Triggered = true
		result.TriggerType = TriggerRatingDefault
		result.IsDefault = true
		result.Detail = "Rating counterparty: " + ratingCurrent + " (default)"
		return result
	}

	// DPD ≥ 90 → Stage 3 directly (caller uses ComputeNewStage to build rows).
	// DPD ≥ 30 → Stage 2.
	if dpdValue >= 30 {
		result.Triggered = true
		result.TriggerType = TriggerDPDGte30
		result.Detail = "DPD = " + itoa(dpdValue) + " hari (≥ 30 threshold DEC-011)"
		return result
	}

	// Delta notch from origination — per IFRS9 §5.5.11 fixed baseline.
	delta := DeltaNotch(ratingAtOrigination, ratingCurrent)
	result.DeltaNotch = delta
	if delta >= 2 {
		result.Triggered = true
		result.TriggerType = TriggerRatingDowngrade
		result.Detail = "Rating " + ratingAtOrigination + " → " + ratingCurrent +
			" (" + itoa(delta) + " notch, ≥ 2 notch threshold DEC-011)"
		return result
	}

	// IG → non-IG transition (uses ratingPrevious as the "before" state).
	// Only triggers if the previous rating was IG and current is non-IG.
	if ratingPrevious != "" && IsInvestmentGrade(ratingPrevious) && !IsInvestmentGrade(ratingCurrent) {
		result.Triggered = true
		result.TriggerType = TriggerIGToNonIG
		result.Detail = "Rating berubah dari IG (" + ratingPrevious + ") ke non-IG (" + ratingCurrent + ")"
		return result
	}

	return result // Triggered=false
}

// ComputeNewStage determines the target stage after an evaluation.
// Returns the new stage and the rows to insert (1 row normally, 2 rows for DPD≥90 from Stage 1).
//
// Ref: state-machine §1.2 valid transitions table.
func ComputeNewStage(currentStage Stage, sicrResult SICRResult, dpdValue int) (newStage Stage, needsDoubleRow bool) {
	if sicrResult.IsDefault || dpdValue >= 90 {
		// Rating=idD or DPD≥90 → Stage 3.
		// If currently Stage 1, we insert 2 rows atomically (1→2 then 2→3).
		needsDoubleRow = currentStage == Stage1
		return Stage3, needsDoubleRow
	}
	if sicrResult.Triggered {
		// SICR → Stage 2 (only valid from Stage 1).
		if currentStage == Stage1 {
			return Stage2, false
		}
		// Already Stage 2: no new row (re-evaluation may update DPD detail).
		if currentStage == Stage2 {
			return Stage2, false // no transition; caller checks for no-op
		}
	}
	// No trigger: no transition.
	return currentStage, false
}

// ─── DPD domain types ─────────────────────────────────────────────────────────

// DPDRecord mirrors trx.dpd_record (migration 000022 §Section 1).
// DPD value is a plain int — not money, not decimal.
type DPDRecord struct {
	ID          uuid.UUID  `db:"id"`
	InstrumenID uuid.UUID  `db:"instrumen_id"`
	// Periode is the first day of the month (YYYY-MM-01) per migration comment.
	Periode     time.Time  `db:"periode"`
	DPDValue    int        `db:"dpd_value"`
	Source      string     `db:"source"` // MANUAL | APP_B
	Catatan     *string    `db:"catatan"`
	RecordedBy  uuid.UUID  `db:"recorded_by"`
	RecordedAt  time.Time  `db:"recorded_at"`
	CreatedAt   time.Time  `db:"created_at"`
	CreatedBy   uuid.UUID  `db:"created_by"`
	UpdatedAt   time.Time  `db:"updated_at"`
	UpdatedBy   uuid.UUID  `db:"updated_by"`
	DeletedAt   *time.Time `db:"deleted_at"`
	DeletedBy   *uuid.UUID `db:"deleted_by"`
	RowVersion  int64      `db:"row_version"`
	TenantID    string     `db:"tenant_id"`
}

// ─── StageHistoryEntry ────────────────────────────────────────────────────────

// StageHistoryEntry mirrors ecl.stage_history with P4-M1 augmented columns.
// This table is append-only (tg_ecl_stage_history_no_delete from migration 000005).
// Per DEC-018: no UPDATE or DELETE.
type StageHistoryEntry struct {
	ID                 uuid.UUID      `db:"id"`
	InstrumenID        uuid.UUID      `db:"instrumen_id"`
	StageSebelum       Stage          `db:"stage_sebelum"`
	StageSesudah       Stage          `db:"stage_sesudah"`
	TriggerType        TriggerType    `db:"trigger_type"`
	DetailTrigger      *string        `db:"detail_trigger"`
	RatingSaatMigrasi  *string        `db:"rating_saat_migrasi"`
	DPD                *int           `db:"dpd"`
	TanggalMigrasi     time.Time      `db:"tanggal_migrasi"`
	StatusApproval     StatusApproval `db:"status_approval"`
	UserApproverID     *uuid.UUID     `db:"user_approver_id"`
	DokumenPendukungID *uuid.UUID     `db:"dokumen_pendukung_id"`
	// P4-M1 augmented columns (migration 000022 §Section 3):
	OverrideProposalID *uuid.UUID `db:"override_proposal_id"`
	EvaluationJobID    *uuid.UUID `db:"evaluation_job_id"`
	TenantID           string     `db:"tenant_id"`
	// Standard audit columns:
	CreatedAt time.Time `db:"created_at"`
	CreatedBy uuid.UUID `db:"created_by"`
}

// ─── OverrideProposal ─────────────────────────────────────────────────────────

// OverrideProposal mirrors ecl.staging_override_proposal (migration 000022 §Section 2).
// 6-eyes workflow for Stage 3→2 (ALCO + KOMITE); 4-eyes for Stage 2→1 (ALCO only).
// Ref: state-machine doc §2.
type OverrideProposal struct {
	ID                       uuid.UUID              `db:"id"`
	InstrumenID              uuid.UUID              `db:"instrumen_id"`
	StageFrom                Stage                  `db:"stage_from"`
	StageTo                  Stage                  `db:"stage_to"`
	Alasan                   string                 `db:"alasan"`
	ReasonCategory           *string                `db:"reason_category"`
	DokumenPendukungID       *uuid.UUID             `db:"dokumen_pendukung_id"`
	PeriodeID                uuid.UUID              `db:"periode_id"`
	PeriodeAkhir             time.Time              `db:"periode_akhir"`
	WorkflowStatus           OverrideWorkflowStatus `db:"workflow_status"`
	CurrentStageAtSubmit     *Stage                 `db:"current_stage_at_submit"`
	MakerID                  uuid.UUID              `db:"maker_id"`
	ReviewerID               *uuid.UUID             `db:"reviewer_id"`
	SignedAtReview            *time.Time             `db:"signed_at_review"`
	SignatureHashReview       []byte                 `db:"signature_hash_review"`
	CommentReview            *string                `db:"comment_review"`
	ApproverALCOID           *uuid.UUID             `db:"approver_alco_id"`
	SignedAtApproveALCO       *time.Time             `db:"signed_at_approve_alco"`
	SignatureHashApproveALCO  []byte                 `db:"signature_hash_approve_alco"`
	CommentApproveALCO       *string                `db:"comment_approve_alco"`
	ApproverKomiteID         *uuid.UUID             `db:"approver_komite_id"`
	SignedAtApproveKomite     *time.Time             `db:"signed_at_approve_komite"`
	SignatureHashApproveKomite []byte                `db:"signature_hash_approve_komite"`
	CommentApproveKomite     *string                `db:"comment_approve_komite"`
	RejectReason             *string                `db:"reject_reason"`
	StageHistoryRowID        *uuid.UUID             `db:"stage_history_row_id"`
	ExpiresAfterPeriode      *time.Time             `db:"expires_after_periode"`
	// Standard audit columns:
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// Is6Eyes returns true when the transition requires the 6-eyes path (Stage 3 → 2).
// Per state-machine §2.2 and OQ-STG-4a.
func (p *OverrideProposal) Is6Eyes() bool {
	return p.StageFrom == Stage3 && p.StageTo == Stage2
}

// ─── Request / Response types ─────────────────────────────────────────────────

// StagingEvaluateRequest is the body for POST /ecl/staging/evaluate.
type StagingEvaluateRequest struct {
	// InstrumenIDs is an optional subset; empty = all active AC/FVOCI instruments.
	// Max 500 per OpenAPI spec.
	InstrumenIDs []uuid.UUID `json:"instrumenIds"`
	// TriggerType: "RATING", "DPD", or "ALL" (default).
	TriggerType string `json:"triggerType" binding:"required"`
	// PeriodeID context; nil = use currently active periode.
	PeriodeID *uuid.UUID `json:"periodeId"`
	// Reason for manual trigger (audit log).
	Reason *string `json:"reason"`
}

// OverrideSubmitRequest is the body for POST /ecl/staging/override/submit.
type OverrideSubmitRequest struct {
	InstrumenID        uuid.UUID  `json:"instrumenId"  binding:"required"`
	StageTarget        Stage      `json:"stageTarget"  binding:"required"`
	Alasan             string     `json:"alasan"       binding:"required,min=10,max=2000"`
	ReasonCategory     *string    `json:"reasonCategory"`
	DokumenPendukungID *uuid.UUID `json:"dokumenPendukungId"`
	PeriodeID          uuid.UUID  `json:"periodeId"    binding:"required"`
}

// WorkflowActionRequest is the body for review / approve / reject endpoints.
type WorkflowActionRequest struct {
	Action          string  `json:"action"          binding:"required"` // APPROVE | REJECT
	Comment         *string `json:"comment"`
	SignatureMethod  string  `json:"signatureMethod"`
}

// WorkflowRejectRequest is the body for the reject endpoint (comment required).
type WorkflowRejectRequest struct {
	Comment        string `json:"comment"        binding:"required,min=5"`
	SignatureMethod string `json:"signatureMethod"`
}

// DPDRecordCreateRequest is the body for POST /ecl/dpd/record.
// dpdValue is int (not money/decimal) per state-machine §8 key decimal precision note.
type DPDRecordCreateRequest struct {
	InstrumenID uuid.UUID `json:"instrumenId" binding:"required"`
	Periode     string    `json:"periode"     binding:"required"` // YYYY-MM-01
	DPDValue    int       `json:"dpdValue"    binding:"min=0"`
	Source      string    `json:"source"      binding:"required"` // must be MANUAL
	Catatan     *string   `json:"catatan"`
}

// StageStatus is the response for GET /ecl/staging/instrumen/{id}.
type StageStatus struct {
	InstrumenID             uuid.UUID  `json:"instrumenId"`
	KodeInstrumen           string     `json:"kodeInstrumen"`
	NamaInstrumen           string     `json:"namaInstrumen"`
	KlasifikasiPsak71       string     `json:"klasifikasiPsak71"`
	CurrentStage            *Stage     `json:"currentStage"` // nil if never evaluated
	LastTransitionDate      *time.Time `json:"lastTransitionDate"`
	LastTriggerType         *TriggerType `json:"lastTriggerType"`
	LastTriggerDetail       *string    `json:"lastTriggerDetail"`
	LastRatingSaatMigrasi   *string    `json:"lastRatingSaatMigrasi"`
	LastDPD                 *int       `json:"lastDpd"`
	LastStatusApproval      *StatusApproval `json:"lastStatusApproval"`
	ActiveOverrideID        *uuid.UUID `json:"activeOverrideId"`
	ActiveOverrideExpiresAt *time.Time `json:"activeOverrideExpiresAt"`
}

// EvaluationResult is returned by EvaluateInstrumen.
type EvaluationResult struct {
	InstrumenID     uuid.UUID
	PreviousStage   *Stage
	NewStage        *Stage
	SICRResult      SICRResult
	HistoryRowsInserted int
	Skipped         bool
	SkipReason      string
}

// CureResult summarizes a cure assessment job run.
type CureResult struct {
	PeriodeID       uuid.UUID
	TotalEvaluated  int
	TotalCured      int
	TotalIneligible int
	Warnings        []string
}

// ─── Allowed columns for list queries ────────────────────────────────────────

// AllowedSortColsHistory lists allowed sort columns for stage_history list.
var AllowedSortColsHistory = []string{
	"tanggal_migrasi", "stage_sebelum", "stage_sesudah",
	"trigger_type", "status_approval", "created_at",
}

// AllowedFilterColsHistory lists allowed filter columns for stage_history list.
var AllowedFilterColsHistory = []string{
	"trigger_type", "stage_sebelum", "stage_sesudah",
	"status_approval", "tanggal_migrasi",
}

// AllAllowedColsHistory is the union for listquery.ParseFromRequest.
var AllAllowedColsHistory = append(append([]string{}, AllowedSortColsHistory...), AllowedFilterColsHistory...)

// AllowedSortColsOverride lists allowed sort columns for override list.
var AllowedSortColsOverride = []string{
	"created_at", "periode_akhir", "workflow_status", "stage_from", "stage_to",
}

// AllowedFilterColsOverride lists allowed filter columns for override list.
var AllowedFilterColsOverride = []string{
	"workflow_status", "stage_from", "stage_to",
	"instrumen_id", "periode_id", "created_at",
}

// AllAllowedColsOverride is the union for listquery.ParseFromRequest.
var AllAllowedColsOverride = append(append([]string{}, AllowedSortColsOverride...), AllowedFilterColsOverride...)

// AllowedSortColsDPD lists allowed sort columns for DPD history list.
var AllowedSortColsDPD = []string{"periode", "dpd_value", "source", "recorded_at"}

// AllowedFilterColsDPD lists allowed filter columns for DPD history list.
var AllowedFilterColsDPD = []string{"periode", "source", "dpd_value"}

// AllAllowedColsDPD is the union for listquery.ParseFromRequest.
var AllAllowedColsDPD = append(append([]string{}, AllowedSortColsDPD...), AllowedFilterColsDPD...)

// ─── Utility ─────────────────────────────────────────────────────────────────

// itoa converts an int to string without importing strconv at domain layer.
// Limited to staging detail message construction.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// strPtr returns a pointer to s (helper for optional string fields).
func strPtr(s string) *string { return &s }

// ─── Signature hash computation ───────────────────────────────────────────────

// ComputeSignatureHash computes the SHA-256 signature hash for an override step.
// Formula (per migration comment):
//
//	SHA-256(userID || step || proposalID || signedAt.RFC3339Nano || comment)
//
// step values: "REVIEW", "APPROVE", "APPROVE2".
// Returns raw 32 bytes (stored as BYTEA in DB).
func ComputeSignatureHash(userID uuid.UUID, step string, proposalID uuid.UUID, signedAt time.Time, comment string) []byte {
	payload := userID.String() + step + proposalID.String() + signedAt.Format(time.RFC3339Nano) + comment
	sum := sha256.Sum256([]byte(payload))
	return sum[:]
}
