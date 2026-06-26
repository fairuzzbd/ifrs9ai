package kurs

// validator_p5m5.go — P5-M5 FX rate validation helpers.
//
// Responsibilities:
//   - Rate range bounds per currency (hard-coded per state machine §4.2).
//   - Deviation computation from prior ACTIVE rate.
//   - Business-day validation (weekends + sys.holiday_calendar lookup).
//   - Upload file row parsing + batch-level error aggregation.
//
// All monetary arithmetic uses shopspring/decimal (DEC-016).

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ─── Rate range bounds ────────────────────────────────────────────────────────

// rateBounds defines min/max kurs_tengah per ISO 4217 code.
// Source: state machine spec §4.2 (FX_RATE_RANGE_BOUNDS, hardcoded for Phase 1).
// Units: IDR per 1 unit of foreign currency.
type rateBounds struct {
	Min decimal.Decimal
	Max decimal.Decimal
}

// currencyBounds maps kode_mata_uang → allowed range for kurs_tengah.
// Extend here when FX_JISDOR_CURRENCIES config is updated.
var currencyBounds = map[string]rateBounds{
	"USD": {Min: decimal.NewFromInt(5_000), Max: decimal.NewFromInt(50_000)},
	"EUR": {Min: decimal.NewFromInt(5_000), Max: decimal.NewFromInt(50_000)},
	"JPY": {Min: decimal.NewFromInt(50), Max: decimal.NewFromInt(500)},
	"SGD": {Min: decimal.NewFromInt(5_000), Max: decimal.NewFromInt(30_000)},
	"AUD": {Min: decimal.NewFromInt(5_000), Max: decimal.NewFromInt(25_000)},
	"GBP": {Min: decimal.NewFromInt(5_000), Max: decimal.NewFromInt(40_000)},
}

// ValidateRateRange checks whether kursTengah falls within the known bounds for kode.
// If the currency is not in currencyBounds (unlisted), validation is skipped (returns nil).
// Returns error string describing the violation, or nil if valid.
func ValidateRateRange(kode string, kursTengah decimal.Decimal) error {
	bounds, ok := currencyBounds[strings.ToUpper(kode)]
	if !ok {
		return nil // unknown currencies are not range-checked
	}
	if kursTengah.LessThan(bounds.Min) {
		return fmt.Errorf("kurs_tengah %s untuk %s lebih kecil dari minimum yang diizinkan %s",
			kursTengah.StringFixed(4), kode, bounds.Min.StringFixed(0))
	}
	if kursTengah.GreaterThan(bounds.Max) {
		return fmt.Errorf("kurs_tengah %s untuk %s melebihi maksimum yang diizinkan %s",
			kursTengah.StringFixed(4), kode, bounds.Max.StringFixed(0))
	}
	return nil
}

// ─── Deviation computation ────────────────────────────────────────────────────

// ComputeDeviation computes the percentage deviation of newRate from priorRate.
// Returns (deviationPct, deviationFlag, nil) where deviationFlag=true when
// abs(deviationPct) > threshold.
//
// Formula: (newRate - priorRate) / priorRate × 100
//
// If priorRate is zero or nil (no prior rate exists), returns (0, false, nil) — no
// flagging for first occurrence.
func ComputeDeviation(newRate decimal.Decimal, priorRate *decimal.Decimal, thresholdPct float64) (decimal.Decimal, bool, error) {
	if priorRate == nil || priorRate.IsZero() {
		return decimal.Zero, false, nil
	}

	diff := newRate.Sub(*priorRate)
	pct := diff.Div(*priorRate).Mul(decimal.NewFromInt(100))

	// |pct| > threshold → flag
	pctFloat, _ := pct.Float64()
	flagged := math.Abs(pctFloat) > thresholdPct

	return pct, flagged, nil
}

// ─── Business-day validation ──────────────────────────────────────────────────

// IsWeekend returns true if t falls on Saturday (6) or Sunday (0 in Go's Weekday).
func IsWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// ParseDateStrict parses "YYYY-MM-DD" strictly (no partial dates, no timezone drift).
func ParseDateStrict(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal tidak valid %q: format harus YYYY-MM-DD: %w", s, err)
	}
	return t, nil
}

// ValidateTanggalBerlaku checks date is not too far in the future (> today + 1).
// Returns error if the date is in the future beyond tomorrow.
func ValidateTanggalBerlaku(tanggal time.Time) error {
	tomorrow := time.Now().AddDate(0, 0, 1)
	if tanggal.After(tomorrow) {
		return fmt.Errorf("tanggal_berlaku %s tidak boleh lebih dari 1 hari ke depan (maks %s)",
			tanggal.Format("2006-01-02"),
			tomorrow.Format("2006-01-02"))
	}
	return nil
}

// ─── Upload file row parsing ──────────────────────────────────────────────────

// RawUploadRow is one parsed row from a manually uploaded CSV/XLSX file.
type RawUploadRow struct {
	RowNumber    int
	KodeMataUang string
	Tanggal      string // raw string from file
	KursTengah   string
	KursBeli     string
	KursJual     string
	SumberKurs   string
}

// ValidatedUploadRow is a successfully validated row ready for DB insert.
type ValidatedUploadRow struct {
	RowNumber    int
	KodeMataUang string
	Tanggal      time.Time
	KursTengah   decimal.Decimal
	KursBeli     *decimal.Decimal
	KursJual     *decimal.Decimal
	SumberKurs   SumberKurs
}

// ValidateUploadRow validates one raw upload row.
// Returns (ValidatedUploadRow, nil) on success; (nil, UploadRowError) on failure.
func ValidateUploadRow(row RawUploadRow) (*ValidatedUploadRow, *UploadRowError) {
	// kode_mata_uang
	kode := strings.ToUpper(strings.TrimSpace(row.KodeMataUang))
	if kode == "" {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kodeMataUang", Error: "wajib diisi"}
	}
	if kode == "IDR" {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kodeMataUang", Error: "IDR tidak boleh digunakan sebagai kurs (self-referential)"}
	}

	// tanggal_berlaku
	tanggal, err := ParseDateStrict(strings.TrimSpace(row.Tanggal))
	if err != nil {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "tanggalBerlaku", Error: err.Error()}
	}
	if IsWeekend(tanggal) {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "tanggalBerlaku",
			Error: "tanggal jatuh pada hari Sabtu/Minggu — kurs hanya untuk hari kerja"}
	}
	if err := ValidateTanggalBerlaku(tanggal); err != nil {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "tanggalBerlaku", Error: err.Error()}
	}

	// kurs_tengah
	tengahRaw := strings.TrimSpace(row.KursTengah)
	if tengahRaw == "" {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursTengah", Error: "wajib diisi"}
	}
	tengah, decErr := decimal.NewFromString(tengahRaw)
	if decErr != nil {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursTengah",
			Error: "bukan angka desimal yang valid: " + decErr.Error()}
	}
	if !tengah.IsPositive() {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursTengah", Error: "harus lebih besar dari 0"}
	}
	if rangeErr := ValidateRateRange(kode, tengah); rangeErr != nil {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursTengah", Error: rangeErr.Error()}
	}

	// kurs_beli (optional)
	var beliPtr *decimal.Decimal
	if raw := strings.TrimSpace(row.KursBeli); raw != "" {
		v, err := decimal.NewFromString(raw)
		if err != nil {
			return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursBeli",
				Error: "bukan angka desimal: " + err.Error()}
		}
		beliPtr = &v
	}

	// kurs_jual (optional)
	var jualPtr *decimal.Decimal
	if raw := strings.TrimSpace(row.KursJual); raw != "" {
		v, err := decimal.NewFromString(raw)
		if err != nil {
			return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursJual",
				Error: "bukan angka desimal: " + err.Error()}
		}
		jualPtr = &v
	}

	// Cross-validate beli ≤ tengah ≤ jual
	if beliPtr != nil && beliPtr.GreaterThan(tengah) {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursBeli",
			Error: "kurs_beli harus ≤ kurs_tengah"}
	}
	if jualPtr != nil && tengah.GreaterThan(*jualPtr) {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "kursJual",
			Error: "kurs_tengah harus ≤ kurs_jual"}
	}

	// sumber_kurs
	sumber := SumberKurs(strings.ToUpper(strings.TrimSpace(row.SumberKurs)))
	if sumber == "" {
		sumber = SumberKursManual
	}
	if !validSumberKurs[sumber] {
		return nil, &UploadRowError{RowNumber: row.RowNumber, Field: "sumberKurs",
			Error: fmt.Sprintf("sumber_kurs %q tidak valid; pilih: BI_JISDOR, BI_KURS_TENGAH, INTERNAL, MANUAL", sumber)}
	}

	return &ValidatedUploadRow{
		RowNumber:    row.RowNumber,
		KodeMataUang: kode,
		Tanggal:      tanggal,
		KursTengah:   tengah,
		KursBeli:     beliPtr,
		KursJual:     jualPtr,
		SumberKurs:   sumber,
	}, nil
}
