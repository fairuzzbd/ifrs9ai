package kurs

// provider_jisdor.go — JISDORAdapter stub implementing FxRateProvider.
//
// Phase 5 stub: delegates to the existing internal/integration/jisdor package.
// Full BI JISDOR scraping logic lives there (routes to integration-engineer).
// This adapter wraps it to satisfy the FxRateProvider interface.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	jisdorpkg "blips-ifrs9.tugu-re.com/internal/integration/jisdor"
)

// JISDORAdapter implements FxRateProvider via BI JISDOR web scraper.
// Internally delegates to jisdorpkg.HTTPFetcher.
type JISDORAdapter struct {
	fetcher *jisdorpkg.HTTPFetcher
}

// NewJISDORAdapter creates a new JISDORAdapter.
func NewJISDORAdapter() *JISDORAdapter {
	return &JISDORAdapter{fetcher: jisdorpkg.NewHTTPFetcher()}
}

// Name returns the stable provider identifier.
func (a *JISDORAdapter) Name() string { return "JISDOR" }

// FetchRates calls the JISDOR HTTP fetcher for the given date and converts
// the result to []JisdorRateRow.
//
// The underlying jisdorpkg.HTTPFetcher is a stub in Phase 3 that always returns
// ErrNotImplemented. When integration-engineer completes the real fetcher, this
// adapter will automatically benefit from it.
func (a *JISDORAdapter) FetchRates(tanggalBerlaku string) ([]JisdorRateRow, error) {
	// Validate date format before delegating.
	if _, err := time.Parse("2006-01-02", tanggalBerlaku); err != nil {
		return nil, fmt.Errorf("JISDORAdapter.FetchRates: invalid date %q: %w", tanggalBerlaku, err)
	}

	result, err := a.fetcher.Fetch(context.Background(), tanggalBerlaku)
	if err != nil {
		return nil, fmt.Errorf("JISDORAdapter.FetchRates: %w", err)
	}

	rows := make([]JisdorRateRow, 0, len(result))
	for _, r := range result {
		tengah, decErr := decimal.NewFromString(r.KursTengah)
		if decErr != nil {
			return nil, fmt.Errorf("JISDORAdapter: parse kurs_tengah for %s: %w", r.KodeMataUang, decErr)
		}

		row := JisdorRateRow{
			KodeMataUang: strings.ToUpper(r.KodeMataUang),
			KursTengah:   tengah,
		}
		if r.KursBeli != nil && *r.KursBeli != "" {
			v, err := decimal.NewFromString(*r.KursBeli)
			if err == nil {
				row.KursBeli = &v
			}
		}
		if r.KursJual != nil && *r.KursJual != "" {
			v, err := decimal.NewFromString(*r.KursJual)
			if err == nil {
				row.KursJual = &v
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
