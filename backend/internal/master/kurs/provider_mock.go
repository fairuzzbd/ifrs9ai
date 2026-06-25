package kurs

// provider_mock.go — MockAdapter implementing FxRateProvider for tests.
//
// Usage:
//
//	mock := NewMockAdapter()
//	mock.SeedRate("USD", "2026-06-18", "15500.00000000", nil, nil)
//	rows, err := mock.FetchRates("2026-06-18")

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// MockAdapter is a seedable FxRateProvider for unit tests.
type MockAdapter struct {
	// rates stores seeded rows keyed by (kode + "|" + tanggal).
	rates map[string]JisdorRateRow

	// ForceError, when non-nil, is returned by FetchRates unconditionally.
	ForceError error
}

// NewMockAdapter creates an empty MockAdapter.
func NewMockAdapter() *MockAdapter {
	return &MockAdapter{rates: make(map[string]JisdorRateRow)}
}

// Name returns "MOCK".
func (m *MockAdapter) Name() string { return "MOCK" }

// SeedRate adds a rate row for a specific currency and date.
// tengah, beli, jual are decimal strings (e.g. "15432.12345678").
func (m *MockAdapter) SeedRate(kode, tanggal, tengah string, beli, jual *string) {
	t, err := decimal.NewFromString(tengah)
	if err != nil {
		panic(fmt.Sprintf("MockAdapter.SeedRate: invalid tengah %q: %v", tengah, err))
	}

	row := JisdorRateRow{
		KodeMataUang: kode,
		KursTengah:   t,
	}
	if beli != nil {
		v, err := decimal.NewFromString(*beli)
		if err == nil {
			row.KursBeli = &v
		}
	}
	if jual != nil {
		v, err := decimal.NewFromString(*jual)
		if err == nil {
			row.KursJual = &v
		}
	}
	m.rates[kode+"|"+tanggal] = row
}

// FetchRates returns all seeded rows whose key ends with the given tanggal.
// If ForceError is set, returns that error instead.
func (m *MockAdapter) FetchRates(tanggalBerlaku string) ([]JisdorRateRow, error) {
	if m.ForceError != nil {
		return nil, m.ForceError
	}

	var rows []JisdorRateRow
	for k, row := range m.rates {
		if len(k) > len(tanggalBerlaku) && k[len(k)-len(tanggalBerlaku):] == tanggalBerlaku {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

// Ensure MockAdapter satisfies FxRateProvider at compile time.
var _ FxRateProvider = (*MockAdapter)(nil)
