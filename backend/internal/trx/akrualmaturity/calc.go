package akrualmaturity

// calc.go — Pure computation functions for akrual + jatuh tempo financials.
//
// All functions are stateless and use shopspring/decimal (DEC-016 — never float64).
// 100% test coverage required (compliance-critical per PSAK 71 §5.4.1(b)).
//
// Rounding: HALF_EVEN (banker's rounding) per SoW_v1.4 §4.
// daysInYear is always 365 (integer, not float).

import (
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

var (
	decZero        = decimal.Zero
	decHundred     = decimal.NewFromInt(100)
	decDays365     = decimal.NewFromInt(365)
	decPPHDeposito = decimal.NewFromFloat(0.20) // 20% bunga deposito
	decPPHDividen  = decimal.NewFromFloat(0.10) // 10% dividen (UU PPh §17 2c)
)

// ComputeAkrualBunga computes daily accrual interest for a given instrument.
//
// Stage 1 and Stage 2 (basis = GROSS):
//
//	akrual = gross × eir / 365   (HALF_EVEN, 4 decimal places)
//
// Stage 3 (basis = NET_CARRYING per PSAK 71 §5.4.1(b)):
//
//	net = max(gross - ecl, 0)    (ECL clamp prevents negative carrying)
//	akrual = net × eir / 365
//
// Returns (akrualIDR, carryingBasis, error).
// carryingBasis is the actual IDR amount used (gross or net).
func ComputeAkrualBunga(
	stage int,
	gross decimal.Decimal, // Gross Carrying Amount IDR
	ecl decimal.Decimal,   // ECL allowance from latest sealed run (0 for Stage 1/2)
	eirAnnual decimal.Decimal, // Annual EIR (e.g. 0.07500000)
) (akrualIDR decimal.Decimal, carryingBasis decimal.Decimal, err error) {
	if eirAnnual.IsNegative() {
		return decZero, decZero, fmt.Errorf("ComputeAkrualBunga: eir_annual tidak boleh negatif, got %s", eirAnnual)
	}
	if gross.IsNegative() {
		return decZero, decZero, fmt.Errorf("ComputeAkrualBunga: gross tidak boleh negatif, got %s", gross)
	}
	if ecl.IsNegative() {
		return decZero, decZero, fmt.Errorf("ComputeAkrualBunga: ecl tidak boleh negatif, got %s", ecl)
	}

	switch stage {
	case 1, 2:
		// Stage 1 and Stage 2: bunga berdasarkan Gross Carrying Amount
		carryingBasis = gross
	case 3:
		// Stage 3: bunga berdasarkan Net Carrying Amount per PSAK 71 §5.4.1(b)
		// net = max(gross - ecl, 0) — ECL clamp ≥ 0
		net := gross.Sub(ecl)
		if net.IsNegative() {
			net = decZero
		}
		carryingBasis = net
	default:
		return decZero, decZero, fmt.Errorf("ComputeAkrualBunga: stage tidak valid: %d (valid: 1, 2, 3)", stage)
	}

	// akrual = carryingBasis × eir / 365
	// Integer 365 — never float.
	akrualIDR = carryingBasis.Mul(eirAnnual).Div(decDays365).RoundBank(4)
	return akrualIDR, carryingBasis, nil
}

// ComputePPH computes the withholding tax (PPh) for a given jenis.
//
// Deposito bunga: PPh final 20% (tarif deposito).
// Dividen / distribusi reksadana: PPh final 10% (UU PPh §17 ayat 2c).
// Other (amortisasi, bond bunga): no PPh at accrual time → 0.
//
// Returns (pphAmount, netAmount, error).
func ComputePPH(jenis AkrualJenis, grossAmount decimal.Decimal) (pph decimal.Decimal, net decimal.Decimal, err error) {
	if grossAmount.IsNegative() {
		return decZero, decZero, fmt.Errorf("ComputePPH: grossAmount tidak boleh negatif, got %s", grossAmount)
	}

	switch jenis {
	case JenisBunga:
		// Deposito bunga: PPh final 20% per peraturan perpajakan deposito
		pph = grossAmount.Mul(decPPHDeposito).RoundBank(4)
	case JenisDividen, JenisDistribusiRD:
		// Dividen / distribusi: PPh final 10% (UU PPh §17 ayat 2c)
		pph = grossAmount.Mul(decPPHDividen).RoundBank(4)
	case JenisAmortisasiPremium, JenisAmortisasiDiskon:
		// Amortisasi: tidak ada PPh di level akrual
		pph = decZero
	default:
		return decZero, decZero, fmt.Errorf("ComputePPH: jenis tidak dikenal: %s", jenis)
	}

	net = grossAmount.Sub(pph).RoundBank(4)
	return pph, net, nil
}

// ComputeAmortisasi computes the amortisation delta for one period.
//
// For a PREMIUM bond (kupon > EIR): amortisasi reduces carrying amount.
//   delta = carryingAmountPrev × eir / 365 - kuponHarian
//   if delta < 0 (premium): amortisasi = |delta| (debit P&L, credit asset)
//
// For a DISCOUNT bond (EIR > kupon): amortisasi increases carrying amount.
//   delta = carryingAmountPrev × eir / 365 - kuponHarian
//   if delta > 0 (diskon): amortisasi = delta (debit asset, credit income P&L)
//
// Returns (amortisasiAmount, jenis, error).
// jenis is either JenisAmortisasiPremium or JenisAmortisasiDiskon.
// The absolute value is always returned; jenis indicates direction.
func ComputeAmortisasi(
	row AmortisasiScheduleRow,
	prevCarrying decimal.Decimal, // carrying amount at start of this period
) (amount decimal.Decimal, jenis AkrualJenis, err error) {
	if prevCarrying.IsNegative() {
		return decZero, JenisAmortisasiPremium, fmt.Errorf("ComputeAmortisasi: prevCarrying tidak boleh negatif")
	}

	// Use pre-computed amortisasi_harian from schedule if available.
	// This is the standard EIR amortisation amount per period.
	if !row.AmortisasiHarian.IsZero() {
		amount = row.AmortisasiHarian.Abs().RoundBank(4)
		// Determine direction from kupon vs EIR.
		kupon := decZero
		if row.KuponRate != nil {
			kupon = *row.KuponRate
		}
		if row.EIRPersen.GreaterThan(kupon) {
			// EIR > kupon = discount bond: carrying increases
			jenis = JenisAmortisasiDiskon
		} else {
			// EIR < kupon = premium bond: carrying decreases
			jenis = JenisAmortisasiPremium
		}
		return amount, jenis, nil
	}

	// Fallback: compute from first principles.
	// EIR method: interest income = carrying × EIR / 365
	// Cash coupon = carrying_awal × kupon / 365
	// Amortisation delta = interest_income - cash_coupon
	//   positive delta: discount (carrying ↑)
	//   negative delta: premium (carrying ↓)
	eir := row.EIRPersen
	if row.CreditAdjustedEIR != nil {
		eir = *row.CreditAdjustedEIR // POCI: use credit-adjusted EIR
	}
	interestIncome := prevCarrying.Mul(eir).Div(decDays365).RoundBank(4)

	kupon := decZero
	if row.KuponRate != nil {
		kupon = *row.KuponRate
	}
	couponCash := prevCarrying.Mul(kupon).Div(decDays365).RoundBank(4)

	delta := interestIncome.Sub(couponCash).RoundBank(4)

	if delta.IsPositive() {
		return delta, JenisAmortisasiDiskon, nil
	}
	if delta.IsNegative() {
		return delta.Abs(), JenisAmortisasiPremium, nil
	}
	// delta == 0: no amortisation (par bond or fully amortised)
	return decZero, JenisAmortisasiDiskon, nil
}

// ComputeMaturitySettlement computes jatuh tempo net proceeds for a DEPOSITO.
//
// net_kas = pokok + bunga_last - pph_final
// pph_final = bunga_last × PPh_DEPOSITO (20%)
//
// Returns (pphAmount, netKasIDR, error).
func ComputeMaturitySettlement(
	pokokIDR decimal.Decimal,
	bungaLastIDR decimal.Decimal,
) (pph decimal.Decimal, netKas decimal.Decimal, err error) {
	if pokokIDR.IsNegative() {
		return decZero, decZero, fmt.Errorf("ComputeMaturitySettlement: pokokIDR tidak boleh negatif")
	}
	if bungaLastIDR.IsNegative() {
		return decZero, decZero, fmt.Errorf("ComputeMaturitySettlement: bungaLastIDR tidak boleh negatif")
	}

	pph = bungaLastIDR.Mul(decPPHDeposito).RoundBank(4)
	netKas = pokokIDR.Add(bungaLastIDR).Sub(pph).RoundBank(4)
	return pph, netKas, nil
}

// ConvertFCYtoIDR converts a foreign currency amount to IDR using the approved FX rate.
//
// Returns (amountIDR, error).
func ConvertFCYtoIDR(amountFCY decimal.Decimal, rateIDR decimal.Decimal) (decimal.Decimal, error) {
	if rateIDR.IsNegative() || rateIDR.IsZero() {
		return decZero, fmt.Errorf("ConvertFCYtoIDR: rateIDR harus > 0, got %s", rateIDR)
	}
	return amountFCY.Mul(rateIDR).RoundBank(4), nil
}

// IsStaleECLRun returns true if the ECL sealed run is stale (older than staleDays).
func IsStaleECLRun(sealedAt time.Time, staleDays int) bool {
	cutoff := time.Now().UTC().AddDate(0, 0, -staleDays)
	return sealedAt.Before(cutoff)
}
