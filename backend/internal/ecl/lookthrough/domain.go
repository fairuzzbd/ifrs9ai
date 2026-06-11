// Package lookthrough implements the Look-through ECL engine for Reksadana instruments
// (APP-C-LKT-001..005, P4-M4).
//
// Formula (SoW §4, FSD-APP-C §3, DEC-015):
//
//	For each asset class in the active fund composition:
//	  NAB_portion = NAB_IDR × weight_pct / 100
//	  ECL_skenario = NAB_portion × PD_skenario × LGD           (DEC-010, 3-skenario)
//	  ECL_FL_skenario = ECL_skenario × FL_multiplier_skenario  (dual FL multiplier)
//	  ECL_weighted = Σ(ECL_FL_skenario × bobot_skenario)
//	  TotalECL_IDR = Σ(ECL_weighted across all asset classes)
//
// Scope rules (OQ-M4-3, per ifrs9-compliance-reviewer):
//   - tipe_instrumen = 'REKSADANA' only
//   - klasifikasi_psak71 IN ('AC','FVOCI') — FVTPL di-skip (ECL = 0, bukan error)
//   - POCI Reksadana di-defer ke M7 (non-fatal skip per instrument)
//
// Fund composition workflow: 6-eyes (ROLE-AKUN maker → ROLE-RISK reviewer → ROLE-ALCO approver).
// SoD: maker_id ≠ reviewer_id ≠ approver_id (DEC-017).
// MFA: ROLE-ALCO wajib MFA (DEC-026). Step-up MFA NOT required for fund_composition.approve
// per state-machine doc §5.2 (DEC-027 does not list fund_composition.approve).
//
// Decimal precision (DEC-016):
//   - IDR amounts: NUMERIC(20,4) — StringFixed(4)
//   - PD / LGD / FL multiplier / bobot: NUMERIC(10,8) — StringFixed(8)
//   - weight_pct: NUMERIC(7,4) — StringFixed(4)
//   - NEVER float64 for money or rates.
//
// Decisions: DEC-010, DEC-015, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022.
// State machine: docs/state-machines/p4-m4-lookthrough.md.
// Stories: docs/stories/phase-4/M4-look-through-reksadana.md.
// Migration: db/migrations/000024_fund_composition_lookthrough.up.sql.
package lookthrough

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Permission constants ──────────────────────────────────────────────────────

const (
	// PermFundCompositionCreate is required by ROLE-AKUN to submit + amend compositions.
	PermFundCompositionCreate = "fund_composition.create"
	// PermFundCompositionReview is required by ROLE-RISK to sign review.
	PermFundCompositionReview = "fund_composition.review"
	// PermFundCompositionApprove is required by ROLE-ALCO to sign approve (MFA wajib DEC-026).
	PermFundCompositionApprove = "fund_composition.approve"
	// PermFundCompositionRead is required for list / detail reads.
	PermFundCompositionRead = "fund_composition.read"
	// PermLookthroughCompute is the internal engine permission (not exposed via HTTP to users).
	PermLookthroughCompute = "lookthrough.compute"
	// PermLookthroughPreview is required by ROLE-RISK / ROLE-AKUN-CTL / ROLE-CFO / ROLE-AUDIT.
	PermLookthroughPreview = "lookthrough.preview"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeLookthroughFundCompositionMissing is returned when no APPROVED composition exists
	// for instrumenID on evaluationDate. HTTP 422.
	CodeLookthroughFundCompositionMissing = "LOOKTHROUGH_FUND_COMPOSITION_MISSING"
	// CodeLookthroughNABMissing is returned when mst.instrumen.nominal_nab_idr IS NULL. HTTP 422.
	CodeLookthroughNABMissing = "LOOKTHROUGH_NAB_MISSING"
	// CodeLookthroughWeightInvalid is returned when Σ weight_pct ≠ 100% ± 0.01%. HTTP 422.
	CodeLookthroughWeightInvalid = "LOOKTHROUGH_WEIGHT_INVALID"
	// CodeLookthroughInstrumenNotReksadana is returned when tipe_instrumen ≠ REKSADANA. HTTP 422.
	CodeLookthroughInstrumenNotReksadana = "LOOKTHROUGH_INSTRUMEN_NOT_REKSADANA"
	// CodeLookthroughAssetClassUnknown is returned for unknown asset_class values. HTTP 422.
	CodeLookthroughAssetClassUnknown = "LOOKTHROUGH_ASSET_CLASS_UNKNOWN"
	// CodeLookthroughPDLGDClassMissing is returned when PD/LGD lookup fails for asset class. HTTP 422.
	CodeLookthroughPDLGDClassMissing = "LOOKTHROUGH_PD_LGD_CLASS_MISSING"
	// CodeLookthroughCompositionReviewInvalidTransition is returned for invalid workflow transitions. HTTP 422.
	CodeLookthroughCompositionReviewInvalidTransition = "LOOKTHROUGH_COMPOSITION_REVIEW_INVALID_TRANSITION"
	// CodeLookthroughCompositionSoDViolation is returned for SoD violations in composition workflow. HTTP 403.
	CodeLookthroughCompositionSoDViolation = "LOOKTHROUGH_COMPOSITION_SOD_VIOLATION"
	// CodeLookthroughBulkTooLarge is returned when scope exceeds 10.000 instruments. HTTP 413.
	CodeLookthroughBulkTooLarge = "LOOKTHROUGH_BULK_TOO_LARGE"
	// CodeLookthroughPOCIDeferred is returned for POCI Reksadana — non-fatal skip. HTTP 422.
	CodeLookthroughPOCIDeferred = "LOOKTHROUGH_POCI_DEFERRED"
)

// errLookthrough builds a DomainError with a lookthrough-specific code.
func errLookthrough(code string, msg string, details ...domainerrors.Detail) *domainerrors.DomainError {
	return domainerrors.New(domainerrors.Code(code), msg, details...)
}

// ErrFundCompositionMissing returns LOOKTHROUGH_FUND_COMPOSITION_MISSING (422).
func ErrFundCompositionMissing(instrumenID, evalDate string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughFundCompositionMissing,
		"Tidak ditemukan fund composition APPROVED untuk instrumen "+instrumenID+" per tanggal "+evalDate+".",
		domainerrors.Detail{
			Field:   "instrumenId",
			Rule:    "active_approved_composition_required",
			Message: "Tidak ada composition APPROVED berlaku pada " + evalDate,
		})
}

// ErrNABMissing returns LOOKTHROUGH_NAB_MISSING (422).
func ErrNABMissing(instrumenID, evalDate string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughNABMissing,
		"NAB untuk instrumen "+instrumenID+" tidak tersedia per "+evalDate+". Pastikan feed NAB harian KSEI/MI telah diupload.",
		domainerrors.Detail{
			Field:   "instrumenId",
			Rule:    "nab_idr_required",
			Message: "mst.instrumen.nominal_nab_idr IS NULL",
		})
}

// ErrWeightInvalid returns LOOKTHROUGH_WEIGHT_INVALID (422).
func ErrWeightInvalid(compositionID string, total decimal.Decimal) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughWeightInvalid,
		"Fund composition "+compositionID+" memiliki total weight "+total.StringFixed(4)+"% (expected 100% ± 0.01%). Data integrity issue — hubungi IT Admin.",
		domainerrors.Detail{
			Field:   "fundCompositionId",
			Rule:    "weight_sum_100pct",
			Message: "Integrity violation pada " + compositionID,
		})
}

// ErrWeightSumInvalidSubmit returns LOOKTHROUGH_WEIGHT_INVALID for submit validation (422).
func ErrWeightSumInvalidSubmit(total decimal.Decimal) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughWeightInvalid,
		"Total weight_pct harus 100% ± 0.01%. Saat ini: "+total.StringFixed(4)+"%",
		domainerrors.Detail{
			Field:   "body.lines",
			Rule:    "weight_sum_100pct",
			Message: "Total weight_pct = " + total.StringFixed(4) + "%, expected 100% ± 0.01%",
		})
}

// ErrInstrumenNotReksadana returns LOOKTHROUGH_INSTRUMEN_NOT_REKSADANA (422).
func ErrInstrumenNotReksadana(instrumenID, tipe string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughInstrumenNotReksadana,
		"Fund composition hanya berlaku untuk instrumen tipe REKSADANA. Instrumen "+instrumenID+" bertipe "+tipe+".",
		domainerrors.Detail{
			Field:   "body.instrumenId",
			Rule:    "tipe_instrumen_reksadana",
			Message: "tipe_instrumen harus REKSADANA",
		})
}

// ErrAssetClassUnknown returns LOOKTHROUGH_ASSET_CLASS_UNKNOWN (422).
func ErrAssetClassUnknown(assetClass string, idx int) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughAssetClassUnknown,
		"Asset class "+assetClass+" tidak valid. Nilai yang diterima: GOVT_BOND, CORP_BOND, CASH, EQUITY, OTHER",
		domainerrors.Detail{
			Field:   fmt.Sprintf("body.lines[%d].assetClass", idx),
			Rule:    "enum",
			Message: "Nilai " + assetClass + " tidak ada dalam enum asset class yang valid",
		})
}

// ErrPDLGDClassMissing returns LOOKTHROUGH_PD_LGD_CLASS_MISSING (422).
func ErrPDLGDClassMissing(assetClass, periodeID string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughPDLGDClassMissing,
		"PD/LGD lookup gagal untuk asset class "+assetClass+" — tidak ada parameter APPROVED untuk periodeId "+periodeID+".",
		domainerrors.Detail{
			Field:   "assetClass." + assetClass,
			Rule:    "pd_lgd_parameter_required",
			Message: "mst.pd_pefindo atau mst.lgd_basel tidak tersedia",
		})
}

// ErrCompositionInvalidTransition returns LOOKTHROUGH_COMPOSITION_REVIEW_INVALID_TRANSITION (422).
func ErrCompositionInvalidTransition(from, to string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughCompositionReviewInvalidTransition,
		"Tidak bisa "+to+" dari state "+from+". Transisi workflow tidak valid.",
		domainerrors.Detail{
			Field:   "workflowStatus",
			Rule:    "invalid_transition",
			Message: "Transition " + from + " → " + to + " tidak valid",
		})
}

// ErrCompositionSoDViolation returns LOOKTHROUGH_COMPOSITION_SOD_VIOLATION (403) with role context.
func ErrCompositionSoDViolation(role, compositionID string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughCompositionSoDViolation,
		role+" tidak dapat menjadi reviewer/approver. maker_id = actor_id untuk composition "+compositionID+".",
		domainerrors.Detail{
			Field:   "reviewer",
			Rule:    "reviewer_not_maker",
			Message: "SoD: actor_id ≠ maker_id",
		})
}

// ErrBulkTooLarge returns LOOKTHROUGH_BULK_TOO_LARGE (413).
func ErrBulkTooLarge(count int) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughBulkTooLarge,
		"Jumlah instrumen REKSADANA aktif dalam scope ("+itoa(count)+") melebihi batas 10.000. Hubungi IT Admin untuk mempartisi.")
}

// ErrPOCIDeferred returns LOOKTHROUGH_POCI_DEFERRED (422) — non-fatal skip per instrument.
func ErrPOCIDeferred(instrumenID string) *domainerrors.DomainError {
	return errLookthrough(CodeLookthroughPOCIDeferred,
		"Instrumen "+instrumenID+" adalah POCI. Look-through ECL untuk POCI Reksadana di-defer ke Phase 5. Instrument di-skip dari calc run ini.",
		domainerrors.Detail{
			Field:   "instrumenId",
			Rule:    "poci_reksadana_deferred",
			Message: "poci_flag = TRUE pada instrumen ini",
		})
}

// ─── AssetClass enum ─────────────────────────────────────────────────────────

// AssetClass identifies the type of underlying asset in a fund composition.
// Values match the CHECK constraint in migration 000024 mst.fund_composition_detail.
type AssetClass string

const (
	// AssetClassGovtBond is Obligasi Pemerintah (SBN, SPN, ORI, sukuk negara).
	// PD = 0 (sovereign IDR — hardcoded per OQ-M4-4 resolution, DEC-015).
	// LGD pool: 'SOVEREIGN'.
	AssetClassGovtBond AssetClass = "GOVT_BOND"
	// AssetClassCorpBond is Obligasi Korporasi.
	// PD from mst.pd_pefindo per rating. LGD pool: 'UNSECURED_CORP'.
	AssetClassCorpBond AssetClass = "CORP_BOND"
	// AssetClassCash is Kas, deposito, pasar uang.
	// PD from bank counterparty. LGD pool: 'DEPOSITO'.
	AssetClassCash AssetClass = "CASH"
	// AssetClassEquity is Saham.
	// PD from sektor average. LGD pool: 'EQUITY'.
	AssetClassEquity AssetClass = "EQUITY"
	// AssetClassOther is other asset classes; conservative PD/LGD.
	// Flagged for manual review. LGD pool: 'UNSECURED_CORP' (highest conservative).
	AssetClassOther AssetClass = "OTHER"
)

// IsValid returns true if the AssetClass is one of the five allowed values.
func (a AssetClass) IsValid() bool {
	switch a {
	case AssetClassGovtBond, AssetClassCorpBond, AssetClassCash, AssetClassEquity, AssetClassOther:
		return true
	}
	return false
}

// String implements fmt.Stringer.
func (a AssetClass) String() string { return string(a) }

// LGDPoolTipeEksposur maps an AssetClass to the tipe_eksposur key in mst.lgd_basel.
// Per state-machine doc §2.3.
func (a AssetClass) LGDPoolTipeEksposur() string {
	switch a {
	case AssetClassGovtBond:
		return "SOVEREIGN"
	case AssetClassCorpBond:
		return "UNSECURED_CORP"
	case AssetClassCash:
		return "DEPOSITO"
	case AssetClassEquity:
		return "EQUITY"
	default: // OTHER
		return "UNSECURED_CORP" // conservative default
	}
}

// IsSovereignZeroPD returns true for asset classes where PD is always 0.
// Per OQ-M4-4: GOVT_BOND sovereign IDR → PD = 0.00000000.
func (a AssetClass) IsSovereignZeroPD() bool {
	return a == AssetClassGovtBond
}

// ─── WorkflowStatus ──────────────────────────────────────────────────────────

// WorkflowStatus is the state machine for mst.fund_composition.
// Values match the CHECK constraint in migration 000024.
// State machine: docs/state-machines/p4-m4-lookthrough.md §1.
type WorkflowStatus string

const (
	// WorkflowStatusDraft is the DB-level initial state (not exposed via API).
	WorkflowStatusDraft WorkflowStatus = "DRAFT"
	// WorkflowStatusPendingReview is set immediately on submit (ROLE-AKUN → ROLE-RISK queue).
	WorkflowStatusPendingReview WorkflowStatus = "PENDING_REVIEW"
	// WorkflowStatusPendingApproval is set after ROLE-RISK review (→ ROLE-ALCO queue).
	WorkflowStatusPendingApproval WorkflowStatus = "PENDING_APPROVAL"
	// WorkflowStatusApprovedActive is the final approved state; ECL engine may use this composition.
	WorkflowStatusApprovedActive WorkflowStatus = "APPROVED_ACTIVE"
	// WorkflowStatusSuperseded is set atomically when a newer amendment is approved.
	WorkflowStatusSuperseded WorkflowStatus = "SUPERSEDED"
	// WorkflowStatusRejected is a terminal state — composition was rejected.
	WorkflowStatusRejected WorkflowStatus = "REJECTED"
)

// IsTerminal returns true if no further transitions are allowed.
func (s WorkflowStatus) IsTerminal() bool {
	return s == WorkflowStatusRejected || s == WorkflowStatusSuperseded
}

// IsActive returns true if this composition can be used by the ECL engine.
func (s WorkflowStatus) IsActive() bool {
	return s == WorkflowStatusApprovedActive
}

// String implements fmt.Stringer.
func (s WorkflowStatus) String() string { return string(s) }

// ValidTransitions returns the allowed next states from the current state.
// Per state-machine doc §1.2.
func (s WorkflowStatus) ValidTransitions() []WorkflowStatus {
	switch s {
	case WorkflowStatusPendingReview:
		return []WorkflowStatus{WorkflowStatusPendingApproval, WorkflowStatusRejected}
	case WorkflowStatusPendingApproval:
		return []WorkflowStatus{WorkflowStatusApprovedActive, WorkflowStatusRejected}
	case WorkflowStatusApprovedActive:
		return []WorkflowStatus{WorkflowStatusSuperseded} // only via system (amendment approve)
	default:
		return nil // DRAFT, SUPERSEDED, REJECTED — no transitions
	}
}

// CanTransitionTo returns true if the transition from s to next is valid.
func (s WorkflowStatus) CanTransitionTo(next WorkflowStatus) bool {
	for _, v := range s.ValidTransitions() {
		if v == next {
			return true
		}
	}
	return false
}

// ─── Domain types ─────────────────────────────────────────────────────────────

// FundComposition mirrors a mst.fund_composition header row.
// One composition group spans multiple FundCompositionDetail rows (1 per asset class).
//
// Versioning: effective_from / effective_to form non-overlapping date ranges per instrumen.
// Immutable after APPROVED_ACTIVE. Amendment creates a new composition group;
// old one is SUPERSEDED atomically.
type FundComposition struct {
	ID                   uuid.UUID      `db:"id"`
	InstrumenID          uuid.UUID      `db:"instrumen_id"`
	EffectiveFrom        time.Time      `db:"effective_from"` // DATE — business date
	EffectiveTo          time.Time      `db:"effective_to"`   // DATE — '9999-12-31' means open-ended
	WorkflowStatus       WorkflowStatus `db:"workflow_status"`
	MakerID              uuid.UUID      `db:"maker_id"`
	ReviewerID           *uuid.UUID     `db:"reviewer_id"`
	ApproverID           *uuid.UUID     `db:"approver_id"`
	SignedAtReview       *time.Time     `db:"signed_at_review"`
	SignatureHashReview  []byte         `db:"signature_hash_review"`
	CommentReview        *string        `db:"comment_review"`
	SignedAtApprove      *time.Time     `db:"signed_at_approve"`
	SignatureHashApprove []byte         `db:"signature_hash_approve"`
	CommentApprove       *string        `db:"comment_approve"`
	RejectReason         *string        `db:"reject_reason"`
	SourceDocID          *uuid.UUID     `db:"source_doc_id"`
	// Audit columns:
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// FundCompositionDetail mirrors a mst.fund_composition_detail row.
// One row per asset class per composition version.
// Precision: weight_pct NUMERIC(7,4) per DEC-016.
type FundCompositionDetail struct {
	ID                uuid.UUID       `db:"id"`
	FundCompositionID uuid.UUID       `db:"fund_composition_id"`
	AssetClass        AssetClass      `db:"asset_class"`
	WeightPct         decimal.Decimal `db:"weight_pct"` // NUMERIC(7,4), range [0,100]
	Position          int             `db:"position"`
	// Audit columns:
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// CompositionGroup aggregates a FundComposition header with its detail lines.
// Returned from service Submit, Review, Approve, and GetActive.
type CompositionGroup struct {
	Header  FundComposition
	Details []FundCompositionDetail
	// InstrumenNama is denormalized from mst.instrumen for display purposes.
	InstrumenNama string
	// TotalWeightPct is the sum of all detail WeightPct values (NUMERIC(7,4)).
	// Must equal 100.0000 ± 0.0100 for APPROVED_ACTIVE compositions.
	TotalWeightPct decimal.Decimal
	// IsAmendment is true if this composition supersedes a prior version.
	IsAmendment bool
	// SupersedesCompositionID is the ID of the composition this version replaces (if amendment).
	SupersedesCompositionID *uuid.UUID
}

// InstrumenReksadanaRow holds fields from mst.instrumen needed for look-through ECL.
// Read-only projection — never mutated by lookthrough package.
type InstrumenReksadanaRow struct {
	ID                uuid.UUID
	KodeInstrumen     string
	NamaInstrumen     string
	TipeInstrumen     string
	KlasifikasiPsak71 string           // AC | FVOCI | FVTPL
	NominalNABIDR     *decimal.Decimal // NUMERIC(20,4); NULL if feed not yet received
	POCIFlag          bool
	Status            string // AKTIF | TIDAK_AKTIF
	WorkflowStatus    string // APPROVED | etc.
	TenantID          string
}

// IsFVTPL returns true if the instrument should be skipped (ECL = 0).
// Per OQ-M4-3: FVTPL Reksadana skip — ECL = 0, not an error.
func (r *InstrumenReksadanaRow) IsFVTPL() bool {
	return r.KlasifikasiPsak71 == "FVTPL"
}

// PDLGDParams holds PD per scenario + LGD for one asset class.
// Loaded from mst.pd_pefindo + mst.lgd_basel via LookthroughParamRow.
// Precision: NUMERIC(10,8) per DEC-016.
type PDLGDParams struct {
	AssetClass AssetClass
	PDGood     decimal.Decimal // NUMERIC(10,8)
	PDNormal   decimal.Decimal // NUMERIC(10,8)
	PDBad      decimal.Decimal // NUMERIC(10,8)
	LGD        decimal.Decimal // NUMERIC(10,8)
}

// ScenarioWeights holds the ALCO-approved scenario weights.
// Default: Good=0.25, Normal=0.50, Bad=0.25 (DEC-010).
// Precision: NUMERIC(10,8) per DEC-016.
type ScenarioWeights struct {
	Good   decimal.Decimal // NUMERIC(10,8)
	Normal decimal.Decimal // NUMERIC(10,8)
	Bad    decimal.Decimal // NUMERIC(10,8)
}

// FLMultipliers holds the dual forward-looking multipliers per scenario.
// From mst.impact_mev_pd APPROVED for periodeID.
// Precision: NUMERIC(10,8) per DEC-016.
type FLMultipliers struct {
	Good   decimal.Decimal // NUMERIC(10,8)
	Normal decimal.Decimal // NUMERIC(10,8)
	Bad    decimal.Decimal // NUMERIC(10,8)
}

// BreakdownLine is the per-asset-class ECL computation result.
// Stores ALL intermediate values for audit trail (FSD-APP-C §3, DEC-018).
//
// Formula per line (DEC-015):
//
//	NABPortionIDR = NABIDR × WeightPct / 100
//	ECLSkenariosGoodIDR   = NABPortionIDR × PDGood   × LGD
//	ECLSkenariosNormalIDR = NABPortionIDR × PDNormal × LGD
//	ECLSkenariosBadIDR    = NABPortionIDR × PDBad    × LGD
//	ECLFLGoodIDR   = ECLSkenariosGoodIDR   × FLGood
//	ECLFLNormalIDR = ECLSkenariosNormalIDR × FLNormal
//	ECLFLBadIDR    = ECLSkenariosBadIDR    × FLBad
//	ECLWeightedIDR = ECLFLGoodIDR × WGood + ECLFLNormalIDR × WNormal + ECLFLBadIDR × WBad
//
// Precision: IDR fields NUMERIC(20,4) → StringFixed(4).
// PD/LGD/FL/weight fields NUMERIC(10,8) → StringFixed(8).
type BreakdownLine struct {
	AssetClass AssetClass
	// WeightPct is the portfolio weight (NUMERIC(7,4)).
	WeightPct decimal.Decimal
	// NABPortionIDR = NABIDR × WeightPct / 100 (NUMERIC(20,4)).
	NABPortionIDR decimal.Decimal
	// PD per scenario (NUMERIC(10,8)):
	PDGood   decimal.Decimal
	PDNormal decimal.Decimal
	PDBad    decimal.Decimal
	// LGD (NUMERIC(10,8)):
	LGD decimal.Decimal
	// ECL per scenario before FL multiplier (NUMERIC(20,4)):
	ECLSkenariosGoodIDR   decimal.Decimal
	ECLSkenariosNormalIDR decimal.Decimal
	ECLSkenariosBadIDR    decimal.Decimal
	// ECL per scenario after FL multiplier (NUMERIC(20,4)):
	ECLFLGoodIDR   decimal.Decimal
	ECLFLNormalIDR decimal.Decimal
	ECLFLBadIDR    decimal.Decimal
	// ECL_weighted = Σ(ECL_FL × bobot) (NUMERIC(20,4)):
	ECLWeightedIDR decimal.Decimal
}

// Result is the complete output of a look-through ECL computation
// for one Reksadana instrument. Returned by Compute, BulkCompute, and Preview.
//
// FundCompositionID is the UUID of the mst.fund_composition version used.
// The Breakdown slice contains one entry per asset class in the composition.
// TotalECLIDR = Σ(ECLWeightedIDR across all Breakdown lines).
//
// For FVTPL instruments: TotalECLIDR = 0, FVTPLSkipped = true.
// For POCI instruments: error LOOKTHROUGH_POCI_DEFERRED is returned instead.
type Result struct {
	InstrumenID                  uuid.UUID
	InstrumenNama                string
	KlasifikasiPsak71            string
	NABIDR                       decimal.Decimal // NUMERIC(20,4)
	FundCompositionID            uuid.UUID
	FundCompositionEffectiveFrom time.Time
	TotalECLIDR                  decimal.Decimal // NUMERIC(20,4) = Σ breakdown ECLWeightedIDR
	Breakdown                    []BreakdownLine
	// FVTPLSkipped is true when tipe=REKSADANA but klasifikasi=FVTPL (ECL=0 skip).
	FVTPLSkipped bool
	// Warning holds the FVTPL_SKIP_ECL note when FVTPLSkipped=true.
	Warning string
}

// ComputeBreakdownLine calculates ECL for a single asset class line.
// All arithmetic uses shopspring/decimal (DEC-016 — never float64).
//
// Steps (FSD-APP-C §3, DEC-015):
//  1. NABPortionIDR = nabIDR × weightPct / 100   [NUMERIC(20,4)]
//  2. ECL_S = NABPortionIDR × PD_S × LGD         [NUMERIC(20,4)]
//  3. ECL_FL_S = ECL_S × FL_S                    [NUMERIC(20,4)]
//  4. ECL_w = Σ(ECL_FL_S × bobot_S)             [NUMERIC(20,4)]
//
// Rounding: truncate to 4 decimal places per IDR NUMERIC(20,4) spec.
func ComputeBreakdownLine(
	assetClass AssetClass,
	weightPct decimal.Decimal,
	nabIDR decimal.Decimal,
	pd PDLGDParams,
	fl FLMultipliers,
	bobot ScenarioWeights,
) BreakdownLine {
	hundred := decimal.NewFromInt(100)

	// Step 1: NAB portion (NUMERIC(20,4) — HALF_EVEN per SoW §4 / DEC-016).
	// NABPortionIDR = nabIDR × weightPct / 100
	nabPortion := nabIDR.Mul(weightPct).Div(hundred).RoundBank(4)

	// Step 2: ECL per scenario (before FL multiplier).
	// ECL_S = NABPortionIDR × PD_S × LGD
	eclGood := nabPortion.Mul(pd.PDGood).Mul(pd.LGD).RoundBank(4)
	eclNormal := nabPortion.Mul(pd.PDNormal).Mul(pd.LGD).RoundBank(4)
	eclBad := nabPortion.Mul(pd.PDBad).Mul(pd.LGD).RoundBank(4)

	// Step 3: Apply dual FL multiplier (DEC-010).
	// ECL_FL_S = ECL_S × FL_S
	eclFLGood := eclGood.Mul(fl.Good).RoundBank(4)
	eclFLNormal := eclNormal.Mul(fl.Normal).RoundBank(4)
	eclFLBad := eclBad.Mul(fl.Bad).RoundBank(4)

	// Step 4: Weighted aggregate.
	// ECL_w = ECL_FL_Good × W_Good + ECL_FL_Normal × W_Normal + ECL_FL_Bad × W_Bad
	eclWeighted := eclFLGood.Mul(bobot.Good).
		Add(eclFLNormal.Mul(bobot.Normal)).
		Add(eclFLBad.Mul(bobot.Bad)).
		RoundBank(4)

	return BreakdownLine{
		AssetClass:            assetClass,
		WeightPct:             weightPct,
		NABPortionIDR:         nabPortion,
		PDGood:                pd.PDGood,
		PDNormal:              pd.PDNormal,
		PDBad:                 pd.PDBad,
		LGD:                   pd.LGD,
		ECLSkenariosGoodIDR:   eclGood,
		ECLSkenariosNormalIDR: eclNormal,
		ECLSkenariosBadIDR:    eclBad,
		ECLFLGoodIDR:          eclFLGood,
		ECLFLNormalIDR:        eclFLNormal,
		ECLFLBadIDR:           eclFLBad,
		ECLWeightedIDR:        eclWeighted,
	}
}

// WeightTolerance is the allowed deviation from 100% in sum(weight_pct).
// ±0.0100 per OQ-M4-6 / state-machine doc §5.1.
var WeightTolerance = decimal.NewFromFloat(0.0100)

// Hundred is 100.0000 used for weight sum validation.
var Hundred = decimal.NewFromInt(100)

// ValidateWeightSum returns an error if Σ weight_pct ≠ 100% ± WeightTolerance.
// Called both at submit time and as a defensive check in Compute.
func ValidateWeightSum(details []FundCompositionDetail, compositionID string) error {
	var total decimal.Decimal
	for i := range details {
		total = total.Add(details[i].WeightPct)
	}
	diff := total.Sub(Hundred).Abs()
	if diff.GreaterThan(WeightTolerance) {
		return ErrWeightInvalid(compositionID, total)
	}
	return nil
}

// ValidateWeightSumFromPcts validates weight sum from raw decimal slice.
// Used by service.Submit before creating DB rows.
func ValidateWeightSumFromPcts(weightPcts []decimal.Decimal) error {
	var total decimal.Decimal
	for _, w := range weightPcts {
		total = total.Add(w)
	}
	diff := total.Sub(Hundred).Abs()
	if diff.GreaterThan(WeightTolerance) {
		return ErrWeightSumInvalidSubmit(total)
	}
	return nil
}

// ─── Request/Response types ───────────────────────────────────────────────────

// SubmitCompositionRequest is the input for CompositionService.Submit.
type SubmitCompositionRequest struct {
	InstrumenID             uuid.UUID
	EffectiveFrom           time.Time
	Lines                   []CompositionLineInput
	SourceDocID             *uuid.UUID
	Catatan                 string
	IsAmendment             bool
	SupersedesCompositionID *uuid.UUID
}

// CompositionLineInput is one line (asset class + weight) within a submit request.
type CompositionLineInput struct {
	AssetClass AssetClass
	WeightPct  decimal.Decimal // NUMERIC(7,4)
	Position   int
}

// WorkflowActionRequest is shared by Review, Approve, Reject service methods.
type WorkflowActionRequest struct {
	CompositionID   uuid.UUID
	ActorID         uuid.UUID
	ActorRole       string
	Comment         string
	SignatureMethod string // JWT_STANDARD | JWT_STEP_UP
	TenantID        string
}

// PreviewSummaryRow is one row in the look-through preview DataTable.
// Returned by LookthroughService.Preview (list view).
type PreviewSummaryRow struct {
	InstrumenID                  uuid.UUID
	InstrumenNama                string
	KlasifikasiPsak71            string
	NABIDRStr                    string // StringFixed(4)
	FundCompositionID            *uuid.UUID
	FundCompositionEffectiveFrom *time.Time
	HasComposition               bool
	TotalECLEstimateIDRStr       *string // StringFixed(4); nil if no composition or NAB missing
	IsPreview                    bool
	Warnings                     []PreviewWarning
}

// PreviewWarning is a non-blocking warning in a preview row.
type PreviewWarning struct {
	Code    string
	Message string
}

// ─── Signature hash ───────────────────────────────────────────────────────────

// ComputeReviewSignatureHash computes SHA-256(reviewerID || "REVIEW" || compositionID || signedAt || comment).
// Returns raw 32 bytes stored as BYTEA in mst.fund_composition.signature_hash_review.
// Per migration 000024 comment on signature_hash_review column.
func ComputeReviewSignatureHash(reviewerID, compositionID uuid.UUID, signedAt time.Time, comment string) []byte {
	payload := reviewerID.String() + "REVIEW" + compositionID.String() +
		signedAt.UTC().Format(time.RFC3339Nano) + comment
	sum := sha256.Sum256([]byte(payload))
	return sum[:]
}

// ComputeApproveSignatureHash computes SHA-256(approverID || "APPROVE" || compositionID || signedAt || comment).
// Returns raw 32 bytes stored as BYTEA in mst.fund_composition.signature_hash_approve.
// Per migration 000024 comment on signature_hash_approve column.
func ComputeApproveSignatureHash(approverID, compositionID uuid.UUID, signedAt time.Time, comment string) []byte {
	payload := approverID.String() + "APPROVE" + compositionID.String() +
		signedAt.UTC().Format(time.RFC3339Nano) + comment
	sum := sha256.Sum256([]byte(payload))
	return sum[:]
}

// ─── DataTable allowed columns ────────────────────────────────────────────────

// AllowedSortColsComposition is the whitelist of sortable columns for composition DataTable.
var AllowedSortColsComposition = []string{
	"created_at", "effective_from", "effective_to",
	"workflow_status", "instrumen_id",
}

// AllowedFilterColsComposition is the whitelist of filterable columns for composition DataTable.
var AllowedFilterColsComposition = []string{
	"instrumen_id", "workflow_status", "effective_from", "effective_to", "maker_id",
}

// AllowedColsComposition is the union for listquery.ParseFromRequest.
var AllowedColsComposition = append(
	append([]string{}, AllowedSortColsComposition...),
	AllowedFilterColsComposition...,
)

// AllowedSortColsPreview is the whitelist of sortable columns for preview DataTable.
var AllowedSortColsPreview = []string{
	"instrumen_nama", "nab_idr", "total_ecl_estimate_idr", "fund_composition_effective_from",
}

// AllowedFilterColsPreview is the whitelist of filterable columns for preview DataTable.
var AllowedFilterColsPreview = []string{
	"instrumen_id", "klasifikasi", "has_composition", "nab_idr",
}

// AllowedColsPreview is the union for listquery.ParseFromRequest.
var AllowedColsPreview = append(
	append([]string{}, AllowedSortColsPreview...),
	AllowedFilterColsPreview...,
)

// ─── Internal utilities ───────────────────────────────────────────────────────

// itoa converts int to string without importing strconv at domain layer.
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

// strPtr returns a pointer to s.
func strPtr(s string) *string { return &s }

// decimalPtr returns a pointer to d.
func decimalPtr(d decimal.Decimal) *decimal.Decimal { return &d }

// fmtDate formats a time.Time as YYYY-MM-DD string.
func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

// isOpenEnded returns true if effectiveTo is the sentinel '9999-12-31' (open-ended composition).
func isOpenEnded(effectiveTo time.Time) bool {
	return effectiveTo.Year() >= 9999
}

// datePtrStr returns a string pointer to YYYY-MM-DD, or nil if t is zero.
func datePtrStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

// uuidPtr returns a pointer to uuid.UUID, or nil if id is zero.
func uuidPtr(id uuid.UUID) *uuid.UUID {
	if id == (uuid.UUID{}) {
		return nil
	}
	return &id
}

// panicIfNil panics with the given message if ptr is nil.
// Used in constructors to enforce mandatory dependencies (audit writer).
func panicIfNil(ptr interface{}, msg string) {
	if ptr == nil {
		panic("lookthrough: " + msg)
	}
}

// ─── Prometheus metrics (stubs — wired at main.go level) ─────────────────────

// MetricsRecorder is the interface for recording observability metrics.
// Injected from main.go; no-op in tests.
type MetricsRecorder interface {
	// RecordBulkDuration records the wall-clock duration of BulkCompute.
	RecordBulkDuration(durationSeconds float64)
	// RecordBulkInstrumentCount records how many instruments were processed.
	RecordBulkInstrumentCount(count int)
	// RecordBulkErrors records the number of instruments that errored in bulk.
	RecordBulkErrors(count int)
}

// noopMetrics is a no-op MetricsRecorder used when no Prometheus registry is available.
type noopMetrics struct{}

func (noopMetrics) RecordBulkDuration(_ float64)    {}
func (noopMetrics) RecordBulkInstrumentCount(_ int) {}
func (noopMetrics) RecordBulkErrors(_ int)          {}

// NoopMetrics returns a MetricsRecorder that does nothing. Used for unit tests.
func NoopMetrics() MetricsRecorder { return noopMetrics{} }
