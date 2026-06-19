package renewal

// calc.go — Pure computation functions for renewal preview.
//
// All functions are stateless and use shopspring/decimal (DEC-016).
// 100% unit test coverage required (calc_test.go).
//
// Formula references:
//   bunga_kotor  = pokok_lama × (rate_lama_persen / 100) × (hari_berjalan / 365)
//   PPh_20pct    = bunga_kotor × 0.20  (PP No. 131/2000)
//   bunga_bersih = bunga_kotor - PPh_20pct
//   pokok_baru   = pokok_lama (POKOK_SAJA) | pokok_lama + bunga_bersih (POKOK_PLUS_BUNGA)
//   EIR_baru     = Newton-Raphson IRR (after-tax cashflow) — see eir.go

import (
	"fmt"
	"math"
	"time"

	"github.com/shopspring/decimal"
)

var (
	decimalDays365 = decimal.NewFromInt(365)
	decimalHundred = decimal.NewFromInt(100)
	decimalTwelve  = decimal.NewFromInt(12)
)

// ComputeBungaKotor computes gross interest accrual on the old instrument.
//
//	bunga_kotor = pokok_lama × (rate_lama_persen / 100) × (hari_berjalan / 365)
//
// hari_berjalan is computed from tanggal_penempatan_lama to tanggal_efektif_baru.
// DEC-016: NUMERIC(20,4) — RoundBank(4).
func ComputeBungaKotor(pokokLama decimal.Decimal, rateLamaPersen decimal.Decimal,
	tanggalPenempatanLama time.Time, tanggalEfektifBaru time.Time) decimal.Decimal {

	days := tanggalEfektifBaru.Sub(tanggalPenempatanLama).Hours() / 24
	if days <= 0 {
		return decimal.Zero
	}
	hariDecimal := decimal.NewFromFloat(days)
	rate := rateLamaPersen.Div(decimalHundred)
	result := pokokLama.Mul(rate).Mul(hariDecimal).Div(decimalDays365)
	return result.RoundBank(4)
}

// ComputePPh computes PPh 20% on bunga_kotor.
//
//	PPh_20pct = bunga_kotor × 0.20  (PP No. 131/2000)
//
// Result rounded HALF_EVEN to 4 decimal places.
func ComputePPh(bungaKotor decimal.Decimal) decimal.Decimal {
	return bungaKotor.Mul(PphRate).RoundBank(4)
}

// ComputeBungaBersih computes net interest after PPh.
//
//	bunga_bersih = bunga_kotor - PPh_20pct
func ComputeBungaBersih(bungaKotor, pphAmount decimal.Decimal) decimal.Decimal {
	return bungaKotor.Sub(pphAmount).RoundBank(4)
}

// ComputePokokBaru computes the principal of the new instrument based on skema.
//
//	POKOK_SAJA:       pokok_baru = pokok_lama
//	POKOK_PLUS_BUNGA: pokok_baru = pokok_lama + bunga_bersih
func ComputePokokBaru(skema Skema, pokokLama, bungaBersih decimal.Decimal) (decimal.Decimal, error) {
	switch skema {
	case SkemaPokokSaja:
		return pokokLama.RoundBank(4), nil
	case SkemaPokokPlusBunga:
		return pokokLama.Add(bungaBersih).RoundBank(4), nil
	default:
		return decimal.Zero, fmt.Errorf("ComputePokokBaru: skema tidak valid: %s", skema)
	}
}

// BuildCashflowsAfterTax builds the cashflow array for EIR Newton-Raphson.
//
// Index 0:     -pokok_baru (initial outflow)
// Index 1..n-1: +kupon_bersih_per_bulan (monthly coupon after PPh 20%)
// Index n:     +pokok_baru + kupon_bersih_per_bulan (terminal)
//
// kupon_bersih_per_bulan = pokok_baru × (rate_baru_persen/100/12) × 0.80
//
// ifrs9-compliance-reviewer BLOCKING: after-PPh cashflow per PSAK 71 §5.4.1 + PP 131/2000.
func BuildCashflowsAfterTax(pokokBaru decimal.Decimal, rateBaruPersen decimal.Decimal, tenorBulan int) []decimal.Decimal {
	n := tenorBulan
	if n < 1 {
		return nil
	}

	oneMinusPph := decimal.NewFromFloat(0.80) // 1 - 0.20
	kuponKotor := pokokBaru.Mul(rateBaruPersen).Div(decimalHundred).Div(decimalTwelve)
	kuponBersih := kuponKotor.Mul(oneMinusPph).RoundBank(4)

	cfs := make([]decimal.Decimal, n+1)
	cfs[0] = pokokBaru.Neg()

	for i := 1; i < n; i++ {
		cfs[i] = kuponBersih
	}
	cfs[n] = pokokBaru.Add(kuponBersih) // terminal: principal + last coupon

	return cfs
}

// ComputePreview builds a complete PreviewResult from instrument and renewal parameters.
// Returns error if EIR solver fails to converge.
func ComputePreview(
	pokokLama decimal.Decimal,
	rateLamaPersen decimal.Decimal,
	tanggalPenempatanLama time.Time,
	skema Skema,
	rateBaruPersen decimal.Decimal,
	tenorBaruBulan int,
	tanggalEfektifBaru time.Time,
) (PreviewResult, error) {
	bungaKotor := ComputeBungaKotor(pokokLama, rateLamaPersen, tanggalPenempatanLama, tanggalEfektifBaru)
	pphAmount := ComputePPh(bungaKotor)
	bungaBersih := ComputeBungaBersih(bungaKotor, pphAmount)

	pokokBaru, err := ComputePokokBaru(skema, pokokLama, bungaBersih)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("ComputePreview: %w", err)
	}

	cfs := BuildCashflowsAfterTax(pokokBaru, rateBaruPersen, tenorBaruBulan)
	// Seed: monthly coupon rate as initial guess
	initial := rateBaruPersen.Div(decimalHundred).Div(decimalTwelve)
	eirMonthly, err := NewtonRaphsonIRR(cfs, initial)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("ComputePreview: EIR solver: %w", err)
	}

	// Annualize: EIR_annual = (1 + r_monthly)^12 - 1
	rMonthlyF64, _ := eirMonthly.Float64()
	eirAnnualF64 := math.Pow(1+rMonthlyF64, 12) - 1
	eirAnnualDec := decimal.NewFromFloat(eirAnnualF64).RoundBank(8)

	tanggalJatuhTempoBaru := AddMonths(tanggalEfektifBaru, tenorBaruBulan)

	return PreviewResult{
		PokokLama:             pokokLama,
		BungaKotor:            bungaKotor,
		Pph20pct:              pphAmount,
		BungaBersih:           bungaBersih,
		PokokBaru:             pokokBaru,
		EirBaru:               eirAnnualDec,
		TanggalJatuhTempoBaru: tanggalJatuhTempoBaru,
	}, nil
}
