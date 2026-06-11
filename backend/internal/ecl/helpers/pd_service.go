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

// daysPerYear is 365.25 as a decimal constant — avoids float64 in tenor conversion.
// Using integer arithmetic: 36525 / 100 = 365.25 exactly (DEC-016).
var daysPerYear = decimal.NewFromInt(36525).Div(decimal.NewFromInt(100))

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

	// 2b. Guard POCI — requires credit-adjusted EIR from P4-M7 (F3, FSD-APP-C §3.5).
	if isPOCI(inst.KlasifikasiPsak71) {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePOCIDeferredToM7,
			fmt.Sprintf("Instrumen %s adalah POCI — memerlukan credit-adjusted EIR dari P4-M7. "+
				"Deferred ke modul P4-M7.", instrumenID))
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
		remainingDuration := inst.TanggalJatuhTempo.Sub(evaluationDate)
		if remainingDuration < 0 {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupTenorOutOfRange,
				fmt.Sprintf("Instrumen %s memiliki tanggal_jatuh_tempo di masa lalu (%s). "+
					"Anomali — verifikasi status instrumen.",
					instrumenID, inst.TanggalJatuhTempo.Format("2006-01-02")))
		}
		// F1 (DEC-016): compute tenor in integer days then convert to decimal years.
		// Avoids float64 arithmetic: int(hours/24) gives exact integer day count.
		tenorDaysInt := int64(remainingDuration.Hours() / 24)
		tenorYearsDec := decimal.NewFromInt(tenorDaysInt).Div(daysPerYear)
		tenorMonths := int(decimal.NewFromInt(tenorDaysInt).Div(daysPerYear).Mul(decimal.NewFromInt(12)).IntPart())
		detail.TenorMonthsUsed = &tenorMonths

		var warn *HelperWarning
		pdBase, warn = interpolateLifetimePD(*curve, tenorYearsDec)
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
// F1 (DEC-016): tenorYears is decimal.Decimal — no float64 in formula path.
// Interpolation ratio t = (tenor - lo) / (hi - lo) computed entirely in decimal:
//
//	pd = lo.pd + t × (hi.pd - lo.pd)  — all decimal arithmetic, RoundBank(8)
//
// Ref: FSD-APP-C §3, formulas.md §"EAD (Exposure at Default)", OQ-M2-1.
func interpolateLifetimePD(curve PDCurveRow, tenorYears decimal.Decimal) (decimal.Decimal, *HelperWarning) {
	// Tenor buckets as (years, pd) pairs — all decimal, no float64.
	type bucket struct {
		years decimal.Decimal
		pd    decimal.Decimal
	}
	buckets := []bucket{
		{decimal.NewFromInt(3), curve.PDLifetime3Y},
		{decimal.NewFromInt(5), curve.PDLifetime5Y},
		{decimal.NewFromInt(7), curve.PDLifetime7Y},
		{decimal.NewFromInt(10), curve.PDLifetime10Y},
	}

	// Non-monotone check (OQ-PAR-1a).
	var warn *HelperWarning
	for i := 1; i < len(buckets); i++ {
		if buckets[i].pd.LessThan(buckets[i-1].pd) {
			warn = &HelperWarning{
				Code: WarnPDInterpolationNonMonotone,
				Message: fmt.Sprintf(
					"PD lifetime buckets tidak monoton (pd_%sy=%s > pd_%sy=%s). "+
						"Interpolasi tetap dilanjutkan (OQ-PAR-1a).",
					buckets[i-1].years.String(), buckets[i-1].pd.StringFixed(8),
					buckets[i].years.String(), buckets[i].pd.StringFixed(8)),
			}
			break
		}
	}

	// Boundary conditions.
	if tenorYears.LessThanOrEqual(buckets[0].years) {
		return buckets[0].pd, warn
	}
	if tenorYears.GreaterThanOrEqual(buckets[3].years) {
		return buckets[3].pd, warn
	}

	// Find surrounding buckets and interpolate entirely in decimal.
	// t = (tenorYears - lo.years) / (hi.years - lo.years)
	// pd = lo.pd + t × (hi.pd - lo.pd)
	for i := 0; i < len(buckets)-1; i++ {
		lo, hi := buckets[i], buckets[i+1]
		if tenorYears.GreaterThanOrEqual(lo.years) && tenorYears.LessThan(hi.years) {
			t := tenorYears.Sub(lo.years).Div(hi.years.Sub(lo.years))
			pd := lo.pd.Add(t.Mul(hi.pd.Sub(lo.pd))).RoundBank(8)
			return pd, warn
		}
	}

	// Should be unreachable given the boundary checks above.
	return buckets[3].pd, warn
}

// isECLNotApplicable returns true for classifications that do not require ECL.
// FVTPL and FVOCI_ELECTION are excluded per IFRS9 §5.5.1.
// POCI requires credit-adjusted EIR (P4-M7) and is guarded separately via isPOCI.
func isECLNotApplicable(klasifikasi string) bool {
	return klasifikasi == "FVTPL" || klasifikasi == "FVOCI_ELECTION"
}

// isPOCI returns true for Purchased or Originated Credit-Impaired instruments.
// POCI requires a credit-adjusted EIR from P4-M7; not handled in M2 helpers.
// F3 (FSD-APP-C §3.5, IFRS9 §5.5.13): POCI is deferred to P4-M7.
func isPOCI(klasifikasi string) bool {
	return klasifikasi == "POCI"
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

	// F3: POCI guard — deferred to P4-M7 (credit-adjusted EIR required).
	if isPOCI(inst.KlasifikasiPsak71) {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodePOCIDeferredToM7,
			fmt.Sprintf("Instrumen %s adalah POCI — memerlukan credit-adjusted EIR dari P4-M7. "+
				"Deferred ke modul P4-M7.", instrID))
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
		remainingDuration := inst.TanggalJatuhTempo.Sub(evaluationDate)
		if remainingDuration < 0 {
			return decimal.Zero, detail, domainerrors.New(domainerrors.CodePDLookupTenorOutOfRange,
				fmt.Sprintf("Instrumen %s: tanggal_jatuh_tempo di masa lalu.", instrID))
		}
		// F1 (DEC-016): integer days → decimal years. No float64 in formula path.
		tenorDaysInt := int64(remainingDuration.Hours() / 24)
		tenorYearsDec := decimal.NewFromInt(tenorDaysInt).Div(daysPerYear)
		tenorMonths := int(decimal.NewFromInt(tenorDaysInt).Div(daysPerYear).Mul(decimal.NewFromInt(12)).IntPart())
		detail.TenorMonthsUsed = &tenorMonths

		var warn *HelperWarning
		pdBase, warn = interpolateLifetimePD(curve, tenorYearsDec)
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
