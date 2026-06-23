package penjualan

// calc.go — Pure computation functions for penjualan financials.
//
// All functions are stateless and use shopspring/decimal (DEC-016 — never float64).
// 100% test coverage required (compliance-critical alongside routing.go).
//
// Rounding: HALF_EVEN (banker's rounding) per SoW_v1.4 §4.
// - IDR amounts: StringFixed(4) = NUMERIC(20,4)
// - Qty: StringFixed(8) = NUMERIC(20,8)
// - Pct: StringFixed(4) = NUMERIC(7,4)

import (
	"fmt"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

var (
	decimalZero    = decimal.Zero
	decimalHundred = decimal.NewFromInt(100)
)

// ComputeProceed calculates the disposal proceeds.
// proceed = harga_jual_per_unit × qty_terjual
// HALF_EVEN rounding, 4 decimal places.
func ComputeProceed(hargaJualPerUnit, qtyTerjual decimal.Decimal) decimal.Decimal {
	return hargaJualPerUnit.Mul(qtyTerjual).RoundBank(4)
}

// ComputeCostBasis calculates the cost basis for partial disposal.
// For PARTIAL: cost_basis = total_cost_basis × (qty_terjual / qty_holding_pre)
// For FULL:    cost_basis = total_cost_basis
// totalCostBasis is per-klasifikasi:
//   AC/POCI/FVOCI: amortized carrying from ecl.amortisasi_schedule
//   FVTPL:         MTM fair value terkini
//   FVOCI_ELECTION: original acquisition cost (harga_perolehan)
func ComputeCostBasis(
	totalCostBasis decimal.Decimal,
	qtyTerjual decimal.Decimal,
	qtyHoldingPre decimal.Decimal,
	jenis DisposalType,
) (decimal.Decimal, error) {
	if qtyHoldingPre.IsZero() {
		return decimalZero, fmt.Errorf("ComputeCostBasis: qty_holding_pre tidak boleh 0")
	}
	if jenis == DisposalFull {
		return totalCostBasis.RoundBank(4), nil
	}
	// PARTIAL: proportional
	ratio := qtyTerjual.Div(qtyHoldingPre)
	return totalCostBasis.Mul(ratio).RoundBank(4), nil
}

// ComputeRealizedGL calculates realized gain or loss.
// realized_gl = proceed - cost_basis
// Positive = gain; negative = loss.
func ComputeRealizedGL(proceed, costBasis decimal.Decimal) decimal.Decimal {
	return proceed.Sub(costBasis).RoundBank(4)
}

// ComputeOCIRecycle calculates the OCI amount to be recycled to P&L.
// Only applicable for FVOCI debt (klasifikasi=FVOCI, recycleOCI=true).
// For FVOCI_ELECTION: returns nil (no recycling per §B5.7.1).
// For PARTIAL: oci_recycled = oci_cumulative × (qty_terjual / qty_holding_pre)
// For FULL:    oci_recycled = oci_cumulative
// ociCumulative can be positive (gain) or negative (loss) — both get recycled.
// Returns (nil, nil) if recycleOCI=false.
func ComputeOCIRecycle(
	recycleOCI bool,
	ociCumulative decimal.Decimal,
	qtyTerjual decimal.Decimal,
	qtyHoldingPre decimal.Decimal,
	jenis DisposalType,
) (*decimal.Decimal, error) {
	if !recycleOCI {
		return nil, nil
	}
	if qtyHoldingPre.IsZero() {
		return nil, fmt.Errorf("ComputeOCIRecycle: qty_holding_pre tidak boleh 0")
	}

	var recycled decimal.Decimal
	if jenis == DisposalFull {
		recycled = ociCumulative.RoundBank(4)
	} else {
		ratio := qtyTerjual.Div(qtyHoldingPre)
		recycled = ociCumulative.Mul(ratio).RoundBank(4)
	}
	return &recycled, nil
}

// ComputeBMFrequency calculates the rolling 12-month cumulative disposal percentage
// for an HTC portfolio.
//
// pct = (cumulative_sold_12m_idr + current_proceed_idr) / total_portofolio_nilai × 100
//
// Returns (pct, nil) on success. Returns error if totalPortofolioNilai is zero.
// Non-HTC portfolios: callers should skip this function (not relevant per S4-AC3).
func ComputeBMFrequency(
	cumulativeSold12mIDR decimal.Decimal, // existing cumulative disposed IDR in last 12 months
	currentProceedIDR decimal.Decimal,    // current disposal proceed
	totalPortofolioNilai decimal.Decimal, // total portfolio value IDR
) (decimal.Decimal, error) {
	if totalPortofolioNilai.IsZero() {
		return decimalZero, domainerrors.New(domainerrors.CodeValidationFailed,
			"ComputeBMFrequency: total_nilai_portofolio tidak boleh 0 untuk BM frequency check.")
	}
	total := cumulativeSold12mIDR.Add(currentProceedIDR)
	pct := total.Div(totalPortofolioNilai).Mul(decimalHundred).RoundBank(4)
	return pct, nil
}

// ValidateBMThresholds checks pct against warn and block thresholds.
// Returns (isBMWarning, isBMBlock).
// warnThreshold and blockThreshold are in percent (e.g. 5.0 for 5%).
func ValidateBMThresholds(pct, warnThreshold, blockThreshold decimal.Decimal) (warn bool, block bool) {
	warn = pct.GreaterThan(warnThreshold)
	block = pct.GreaterThan(blockThreshold)
	return warn, block
}
