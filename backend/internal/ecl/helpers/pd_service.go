// Package helpers — PD lookup service.
//
// Implements PDLookupService per APP-C-PAR-001 decision tree (p4-m2-helpers.md §1.1).
//
// Formula (formulas.md, FSD-APP-C §3):
//
//	Stage 1: PD_base = pd_12month
//	Stage 2: PD_base = linear_interpolate(pd_lifetime_Xy, tenor_remaining)
//	Stage 3: PD = 1.00000000 (fixed, FL not applied — DEC-010)
//
//	For GOOD/BAD: PD_FL = PD_base × impact_pd × impact_mev_pd(skenario)
//	For NORMAL:   PD_FL = PD_base × impact_pd × 1.0  (OQ-A default)
//
// Monotonicity check on lifetime bucket data per OQ-PAR-1a:
//
//	Warning emitted if pd_lifetime_3y > pd_lifetime_5y > … (non-monotone).
//	Interpolation proceeds regardless.
//
// Tenor handling (OQ-PAR-1b):
//
//	Negative tenor_remaining → PD_LOOKUP_TENOR_OUT_OF_RANGE error.
//
// All arithmetic uses decimal.Decimal. No float64.
// Rounding: RoundHalfEven to 8 decimal places after each multiplication step
// (HALF_EVEN = banker's rounding per formulas.md §"Presisi & Rounding").
package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// one is the decimal constant 1.0 — used for NORMAL FL multiplier (OQ-A).
var one = decimal.NewFromInt(1)

// pdService implements PDLookupService.
type pdService struct {
	pdRepo    PDRepository
	instrRepo InstrumenSnapshotRepo
}

// NewPDLookupService creates a PDLookupService.
func NewPDLookupService(pdRepo PDRepository, instrRepo InstrumenSnapshotRepo) PDLookupService {
	return &pdService{pdRepo: pdRepo, instrRepo: instrRepo}
}

// GetPD returns PD for one instrument per stage/scenario per period.
// See domain.go PDLookupService for full contract.
func (s *pdService) GetPD(ctx context.Context, instrumenID uuid.UUID, stage EclStage,
	scenario EclScenario, periodeID string, evaluationDate time.Time) (decimal.Decimal, PDDetail, error) {

	var detail PDDetail
	detail.Stage = stage
	detail.Scenario = scenario
	detail.Warnings = []HelperWarning{}

	// 1. Load instrumen.
	inst, err := s.instrRepo.GetEADInputs(ctx, instrumenID)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca instrumen "+instrumenID.String(), err)
	}
	if inst == nil {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeEADInstrumenNotFound,
			fmt.Sprintf("Instrumen %s tidak ditemukan di mst.instrumen.", instrumenID))
	}

	// 2. Guard FVTPL / FVOCI_ELECTION.
	if isECLNotApplicable(inst.KlasifikasiPsak71) {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeInstrumentECLNotApplicable,
			fmt.Sprintf("Instrumen %s klasifikasi %s tidak memerlukan ECL (IFRS9 §5.5.1).",
				instrumenID, inst.KlasifikasiPsak71))
	}

	// 3. Stage 3 → PD = 1.0, no FL.
	if stage == Stage3 {
		pd3 := decimal.NewFromInt(1)
		detail.PD = pd3
		detail.PDBase = pd3
		detail.ImpactPDMultiplier = decimal.Zero
		detail.ImpactMevPDMultiplier = decimal.Zero
		detail.NormalMultiplierIsDefault = false
		return pd3, detail, nil
	}

	// 4. Look up active rating for the instrument's counterparty.
	rating, err := s.pdRepo.GetActiveRating(ctx, inst.CounterpartyID, evaluationDate)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca rating counterparty", err)
	}
	if rating == "" {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupRatingMissing,
			fmt.Sprintf("Rating aktif untuk counterparty %s tidak ditemukan per tanggal %s. "+
				"Perlu upload rating Pefindo terbaru.",
				inst.CounterpartyID, evaluationDate.Format("2006-01-02")))
	}
	detail.RatingUsed = rating

	// 5. Load PD curve for this rating.
	curve, err := s.pdRepo.GetPefindoCurve(ctx, rating, periodeID)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca kurva PD", err)
	}
	if curve == nil {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupCurveNotFound,
			fmt.Sprintf("Kurva PD tidak ditemukan untuk rating %s per periode %s. "+
				"Pastikan mst.pd_pefindo sudah diisi dan di-approve ALCO.", rating, periodeID))
	}

	// 6. Compute PD_base.
	var pdBase decimal.Decimal
	switch stage {
	case Stage1:
		pdBase = curve.PD12Month
		pd12Copy := curve.PD12Month
		detail.SourcePD12M = &pd12Copy

	case Stage2:
		// Need remaining tenor from tanggal_jatuh_tempo.
		if inst.TanggalJatuhTempo == nil {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupTenorOutOfRange,
				fmt.Sprintf("Instrumen %s tidak memiliki tanggal_jatuh_tempo.", instrumenID))
		}
		tenorDays := inst.TanggalJatuhTempo.Sub(evaluationDate)
		if tenorDays < 0 {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupTenorOutOfRange,
				fmt.Sprintf("Instrumen %s memiliki tanggal_jatuh_tempo di masa lalu (%s). "+
					"Anomali — verifikasi status instrumen.",
					instrumenID, inst.TanggalJatuhTempo.Format("2006-01-02")))
		}
		tenorYears := float64(tenorDays) / float64(365.25*24*time.Hour)
		tenorMonths := int(tenorYears * 12)
		detail.TenorMonthsUsed = &tenorMonths

		var warn *HelperWarning
		pdBase, warn = interpolateLifetimePD(*curve, tenorYears)
		if warn != nil {
			detail.Warnings = append(detail.Warnings, *warn)
		}
		pdLifeCopy := pdBase
		detail.SourcePDLifetime = &pdLifeCopy
	}

	detail.PDBase = pdBase

	// 7. Load impact_pd.
	impPD, err := s.pdRepo.GetActiveImpactPD(ctx, periodeID)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca impact_pd", err)
	}
	if impPD == nil {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupParameterInactive,
			fmt.Sprintf("Forward-looking multiplier (impact_pd) tidak ditemukan untuk periode %s. "+
				"Parameter ECL belum disetujui ALCO.", periodeID))
	}
	detail.ImpactPDMultiplier = impPD.ImpactMultiplier

	// 8. Load impact_mev_pd per scenario.
	var impMevMultiplier decimal.Decimal
	switch scenario {
	case ScenarioNormal:
		// OQ-A: NORMAL = 1.0 default; no row stored in mst.impact_mev_pd.
		impMevMultiplier = one
		detail.NormalMultiplierIsDefault = true
		detail.Warnings = append(detail.Warnings, HelperWarning{
			Code:    WarnNormalFLMultiplierDefault,
			Message: "NORMAL scenario FL multiplier = 1.0 (default per OQ-A — belum dikonfirmasi ALCO).",
		})
	case ScenarioGood, ScenarioBad:
		impMev, err := s.pdRepo.GetActiveImpactMevPD(ctx, string(scenario), periodeID)
		if err != nil {
			return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
				"gagal membaca impact_mev_pd", err)
		}
		if impMev == nil {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupFLParamMissing,
				fmt.Sprintf("Forward-looking MEV multiplier tidak ditemukan untuk skenario %s periode %s.",
					scenario, periodeID))
		}
		impMevMultiplier = impMev.ImpactMultiplier
	}
	detail.ImpactMevPDMultiplier = impMevMultiplier

	// 9. PD_FL = PD_base × impact_pd × impact_mev_pd.
	// Round each step to 8dp with HALF_EVEN (DEC-016).
	pdFL := pdBase.
		Mul(impPD.ImpactMultiplier).
		RoundBank(8).
		Mul(impMevMultiplier).
		RoundBank(8)

	detail.PD = pdFL
	return pdFL, detail, nil
}

// interpolateLifetimePD performs linear interpolation over the tenor buckets:
//
//	Buckets: 3y, 5y, 7y, 10y (from mst.pd_pefindo)
//	If tenor ≤ 3y → use pd_lifetime_3y
//	If tenor > 10y → use pd_lifetime_10y
//	Otherwise: linear interpolation between surrounding buckets.
//
// Returns (pdLifetime, warning|nil).
// Non-monotone check per OQ-PAR-1a: warning emitted, interpolation proceeds.
//
// Ref: FSD-APP-C §3, formulas.md §"EAD (Exposure at Default)", OQ-M2-1.
func interpolateLifetimePD(curve PDCurveRow, tenorYears float64) (decimal.Decimal, *HelperWarning) {
	// Tenor buckets as (years, pd) pairs — all decimal, no float64 for PD values.
	type bucket struct {
		years float64
		pd    decimal.Decimal
	}
	buckets := []bucket{
		{3.0, curve.PDLifetime3Y},
		{5.0, curve.PDLifetime5Y},
		{7.0, curve.PDLifetime7Y},
		{10.0, curve.PDLifetime10Y},
	}

	// Non-monotone check (OQ-PAR-1a).
	var warn *HelperWarning
	for i := 1; i < len(buckets); i++ {
		if buckets[i].pd.LessThan(buckets[i-1].pd) {
			warn = &HelperWarning{
				Code: WarnPDInterpolationNonMonotone,
				Message: fmt.Sprintf(
					"PD lifetime buckets tidak monoton (pd_%dy=%.8s > pd_%dy=%.8s). "+
						"Interpolasi tetap dilanjutkan (OQ-PAR-1a).",
					int(buckets[i-1].years), buckets[i-1].pd.String(),
					int(buckets[i].years), buckets[i].pd.String()),
			}
			break
		}
	}

	// Boundary conditions.
	if tenorYears <= 3.0 {
		return buckets[0].pd, warn
	}
	if tenorYears >= 10.0 {
		return buckets[3].pd, warn
	}

	// Find surrounding buckets and interpolate.
	// t = (tenorYears - lo.years) / (hi.years - lo.years)
	// pd = lo.pd + t × (hi.pd - lo.pd)
	for i := 0; i < len(buckets)-1; i++ {
		lo, hi := buckets[i], buckets[i+1]
		if tenorYears >= lo.years && tenorYears < hi.years {
			// t is a plain float for the bucket ratio — not money, safe.
			t := (tenorYears - lo.years) / (hi.years - lo.years)
			tDec := decimal.NewFromFloat(t)
			diff := hi.pd.Sub(lo.pd)
			pd := lo.pd.Add(tDec.Mul(diff)).RoundBank(8)
			return pd, warn
		}
	}

	// Should be unreachable given the boundary checks above.
	return buckets[3].pd, warn
}

// isECLNotApplicable returns true for classifications that do not require ECL.
// FVTPL and FVOCI_ELECTION are excluded per IFRS9 §5.5.1.
func isECLNotApplicable(klasifikasi string) bool {
	return klasifikasi == "FVTPL" || klasifikasi == "FVOCI_ELECTION"
}

// GetPDFromBatchParams resolves PD for one instrument from pre-loaded BatchParams.
// Used by BulkLookupService to avoid repeated DB calls.
// Returns (pd, detail, warning|nil, error).
func GetPDFromBatchParams(
	instrID uuid.UUID,
	stage EclStage,
	scenario EclScenario,
	inst InstrumenRow,
	params *BatchParams,
	evaluationDate time.Time,
) (decimal.Decimal, PDDetail, error) {

	var detail PDDetail
	detail.Stage = stage
	detail.Scenario = scenario
	detail.Warnings = []HelperWarning{}

	// Stage 3 — PD fixed 1.0.
	if stage == Stage3 {
		pd3 := decimal.NewFromInt(1)
		detail.PD = pd3
		detail.PDBase = pd3
		return pd3, detail, nil
	}

	// Active rating.
	rating, ok := params.Ratings[inst.CounterpartyID]
	if !ok || rating == "" {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupRatingMissing,
			fmt.Sprintf("Rating aktif untuk counterparty %s tidak ditemukan per tanggal %s.",
				inst.CounterpartyID, evaluationDate.Format("2006-01-02")))
	}
	detail.RatingUsed = rating

	// PD curve.
	curve, ok := params.PDCurves[rating]
	if !ok {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupCurveNotFound,
			fmt.Sprintf("Kurva PD tidak ditemukan untuk rating %s.", rating))
	}

	// impact_pd.
	if params.ImpactPD == nil {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupParameterInactive,
			"impact_pd tidak tersedia untuk periode ini.")
	}
	detail.ImpactPDMultiplier = params.ImpactPD.ImpactMultiplier

	// PD_base.
	var pdBase decimal.Decimal
	switch stage {
	case Stage1:
		pdBase = curve.PD12Month
		pd12Copy := curve.PD12Month
		detail.SourcePD12M = &pd12Copy
	case Stage2:
		if inst.TanggalJatuhTempo == nil {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupTenorOutOfRange,
				fmt.Sprintf("Instrumen %s tidak memiliki tanggal_jatuh_tempo.", instrID))
		}
		tenorDays := inst.TanggalJatuhTempo.Sub(evaluationDate)
		if tenorDays < 0 {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupTenorOutOfRange,
				fmt.Sprintf("Instrumen %s: tanggal_jatuh_tempo di masa lalu.", instrID))
		}
		tenorYears := float64(tenorDays) / float64(365.25*24*time.Hour)
		tenorMonths := int(tenorYears * 12)
		detail.TenorMonthsUsed = &tenorMonths

		var warn *HelperWarning
		pdBase, warn = interpolateLifetimePD(curve, tenorYears)
		if warn != nil {
			detail.Warnings = append(detail.Warnings, *warn)
		}
		pdLifeCopy := pdBase
		detail.SourcePDLifetime = &pdLifeCopy
	}
	detail.PDBase = pdBase

	// impact_mev_pd.
	var impMevMultiplier decimal.Decimal
	switch scenario {
	case ScenarioNormal:
		impMevMultiplier = one
		detail.NormalMultiplierIsDefault = true
		detail.Warnings = append(detail.Warnings, HelperWarning{
			Code:    WarnNormalFLMultiplierDefault,
			Message: "NORMAL scenario FL multiplier = 1.0 (default per OQ-A).",
		})
	case ScenarioGood, ScenarioBad:
		mev, ok := params.ImpactMevPD[string(scenario)]
		if !ok {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupFLParamMissing,
				fmt.Sprintf("impact_mev_pd untuk skenario %s tidak tersedia.", scenario))
		}
		impMevMultiplier = mev.ImpactMultiplier
	}
	detail.ImpactMevPDMultiplier = impMevMultiplier

	// PD_FL computation.
	pdFL := pdBase.
		Mul(params.ImpactPD.ImpactMultiplier).
		RoundBank(8).
		Mul(impMevMultiplier).
		RoundBank(8)
	detail.PD = pdFL
	return pdFL, detail, nil
}
