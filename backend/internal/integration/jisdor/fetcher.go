// Package jisdor provides the integration adapter for BI JISDOR (Jakarta Interbank Spot Dollar Rate).
//
// Architecture:
//   - Fetcher interface — contract for fetching JISDOR rates from Bank Indonesia.
//   - HTTPFetcher — production implementation (Phase 4; stub in Phase 3).
//   - Rate — value object returned by Fetch.
//
// Phase 3 status: HTTPFetcher.Fetch always returns ErrNotImplemented.
// The stub is intentional — the BI JISDOR endpoint specification and
// authentication mechanism are confirmed in Phase 4 discovery.
// Manual entry via POST /api/v1/master/kurs is the primary path for Phase 3.
//
// When Phase 4 implements the real fetcher:
//  1. Replace the stub body in HTTPFetcher.Fetch.
//  2. Add real HTTP client, retry logic, and SFTP/web-scrape parsing.
//  3. Update integration tests.
//  4. Wire fetcher into jisdor/cron.go.
package jisdor

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented is returned by the stub fetcher in Phase 3.
var ErrNotImplemented = errors.New("JISDOR fetch not yet implemented — use manual entry via POST /api/v1/master/kurs")

// Rate is the value object returned by Fetcher.Fetch for one currency.
type Rate struct {
	// KodeMataUang is the ISO 4217 currency code (e.g. "USD", "EUR").
	KodeMataUang string

	// TanggalBerlaku is the effective date of the rate (from BI publication).
	TanggalBerlaku time.Time

	// KursTengah is the mid-rate (BI JISDOR value). Always populated.
	// Using string to preserve exact precision from source; caller parses with decimal.NewFromString.
	KursTengah string

	// KursBeli is the buy rate. Populated only if BI publishes it alongside JISDOR.
	KursBeli *string

	// KursJual is the sell rate. Populated only if BI publishes it alongside JISDOR.
	KursJual *string
}

// Fetcher is the interface for fetching JISDOR rates from Bank Indonesia.
//
// Implementations:
//   - HTTPFetcher — production HTTP/SFTP client (stub in Phase 3)
//
// Caller responsibilities:
//   - Parse each Rate.KursTengah with decimal.NewFromString.
//   - Call kurs.Service.CreateApproved for each Rate (trusted source, auto-approved).
//   - Handle duplicate date conflicts (idempotent — skip if already exists).
type Fetcher interface {
	// Fetch returns all available JISDOR rates for the given date.
	// Returns empty slice (not error) if BI has not published rates for that date yet.
	// Returns ErrNotImplemented in Phase 3.
	Fetch(ctx context.Context, tanggal string) ([]Rate, error)
}

// HTTPFetcher is the production implementation (stub in Phase 3).
// It will hit the BI JISDOR endpoint or parse the SFTP CSV in Phase 4.
type HTTPFetcher struct {
	// baseURL will be the BI API or SFTP endpoint (Phase 4).
	baseURL string
}

// NewHTTPFetcher creates an HTTPFetcher.
// In Phase 3, baseURL is ignored since the fetcher is a stub.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		baseURL: "https://www.bi.go.id/id/statistik/informasi-kurs/jisdor", // Phase 4 target
	}
}

// Fetch is the stub implementation — always returns ErrNotImplemented in Phase 3.
// Replace this body in Phase 4 with the real HTTP/SFTP parsing logic.
func (f *HTTPFetcher) Fetch(_ context.Context, _ string) ([]Rate, error) {
	return nil, ErrNotImplemented
}

// Ensure HTTPFetcher satisfies Fetcher at compile time.
var _ Fetcher = (*HTTPFetcher)(nil)
