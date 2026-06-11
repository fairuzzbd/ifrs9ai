// Package helpers — CCF lookup service.
//
// Implements CCFLookupService per APP-C-PAR-004 decision tree (p4-m2-helpers.md §1.4).
//
// Phase 1: all instruments return CCF = 0.0000 per OQ-E resolution.
// Source "PHASE_1_HARDCODED" for known types; "SYS_CONFIG" if a type has CCF > 0.
//
// sys.config cache: Redis key ecl:params:ccf_table TTL 1h.
// Phase 1: without Redis, values from sys.config or dev fallback.
//
// Precision: NUMERIC(7,4) → RoundHalfEven(4).
package helpers

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ValidTipeInstrumen is the set of valid values for tipe_instrumen (TipeInstrumen enum).
// Ref: api/openapi/app-c-helpers.yaml §TipeInstrumen.
var ValidTipeInstrumen = map[string]struct{}{
	"DEPOSITO":   {},
	"OBLIGASI":   {},
	"SAHAM":      {},
	"REKSADANA":  {},
	"SBI":        {},
	"COMMITMENT": {},
}

// ccfService implements CCFLookupService.
type ccfService struct {
	cfgRepo CCFConfigRepo
}

// NewCCFLookupService creates a CCFLookupService.
func NewCCFLookupService(cfgRepo CCFConfigRepo) CCFLookupService {
	return &ccfService{cfgRepo: cfgRepo}
}

// GetCCF returns CCF for a given tipe_instrumen.
// Phase 1: DEPOSITO/OBLIGASI/SAHAM/REKSADANA/SBI = 0.0000.
// Unknown type → CCF_INSTRUMEN_TYPE_UNKNOWN.
// Missing config → CCF_CONFIG_MISSING.
func (s *ccfService) GetCCF(ctx context.Context, instrumenType string) (decimal.Decimal, CCFDetail, error) {
	var detail CCFDetail
	detail.Warnings = []HelperWarning{}

	// 1. Validate enum.
	if _, valid := ValidTipeInstrumen[instrumenType]; !valid {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeCCFInstrumenTypeUnknown,
			fmt.Sprintf("Tipe instrumen '%s' tidak dikenali. Pastikan nilai berasal dari enum TipeInstrumen.", instrumenType))
	}

	// 2. Read sys.config CCF_TABLE.
	table, err := s.cfgRepo.GetCCFTable(ctx)
	if err != nil {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeCCFConfigMissing,
			"sys.config 'CCF_TABLE' tidak ditemukan. Pastikan seed data config sudah dijalankan.")
	}

	// 3. Lookup value.
	ccf, ok := table[instrumenType]
	if !ok {
		// Type in enum but not in config → default 0 with warning.
		detail.CCF = decimal.Zero
		detail.Source = "PHASE_1_HARDCODED"
		detail.Warnings = append(detail.Warnings, HelperWarning{
			Code: WarnCCFTypeNotInConfig,
			Message: fmt.Sprintf("Tipe instrumen %s tidak ada di CCF_TABLE config. "+
				"CCF default 0 digunakan.", instrumenType),
		})
		return decimal.Zero, detail, nil
	}

	ccfRounded := ccf.RoundBank(4)
	detail.CCF = ccfRounded

	// Phase 1: all instruments = 0, source = PHASE_1_HARDCODED.
	if ccfRounded.IsZero() {
		detail.Source = "PHASE_1_HARDCODED"
	} else {
		detail.Source = "SYS_CONFIG"
	}

	return ccfRounded, detail, nil
}
