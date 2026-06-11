// Package helpers — LGD lookup service.
//
// Implements LGDLookupService per APP-C-PAR-002 decision tree (p4-m2-helpers.md §1.2).
//
// Formula (formulas.md, FSD-APP-C §3):
//
//	LGD_eff = LGD_pool × (1 - collateral_haircut)
//
// Phase 1: collateral_haircut = 0 for all instruments (deposito/obligasi/saham).
//
// Mapping tipe_counterparty → tipe_eksposur via sys.config LGD_COUNTERPARTY_TYPE_MAPPING.
// REKSADANA instruments return LGD_LOOKUP_USE_LOOKTHROUGH (look-through via P4-M4).
//
// All arithmetic uses decimal.Decimal. No float64.
// Rounding: RoundHalfEven to 8dp after haircut multiplication.
package helpers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// lgdService implements LGDLookupService.
type lgdService struct {
	lgdRepo   LGDRepository
	instrRepo InstrumenSnapshotRepo
	cpRepo    CounterpartyRepo
}

// NewLGDLookupService creates a LGDLookupService.
func NewLGDLookupService(lgdRepo LGDRepository, instrRepo InstrumenSnapshotRepo, cpRepo CounterpartyRepo) LGDLookupService {
	return &lgdService{lgdRepo: lgdRepo, instrRepo: instrRepo, cpRepo: cpRepo}
}

// GetLGD returns effective LGD for an instrument.
// See domain.go LGDLookupService for full contract.
func (s *lgdService) GetLGD(ctx context.Context, instrumenID uuid.UUID, periodeID string) (decimal.Decimal, LGDDetail, error) {
	var detail LGDDetail
	detail.Warnings = []HelperWarning{}

	// 1. Load instrument.
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

	// 3. Guard REKSADANA.
	if inst.TipeInstrumen == "REKSADANA" {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeLGDLookupUseLookthrough,
			"Instrumen REKSADANA menggunakan mekanisme look-through ECL (P4-M4). "+
				"LGD tidak di-lookup per pool tunggal.")
	}

	// 4. Get tipe_counterparty.
	tipeCP, err := s.cpRepo.GetTipeCounterparty(ctx, inst.CounterpartyID)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca tipe_counterparty", err)
	}
	detail.TipeCounterparty = tipeCP

	// 5. Map tipe_counterparty → tipe_eksposur.
	lgdMapping, err := s.lgdRepo.GetLGDMapping(ctx)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca LGD_COUNTERPARTY_TYPE_MAPPING", err)
	}
	tipeEksposur, ok := lgdMapping[tipeCP]
	if !ok {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeLGDLookupMappingNotFound,
			fmt.Sprintf("Tipe counterparty '%s' tidak memiliki mapping LGD pool. "+
				"Konfigurasikan di sys.config LGD_COUNTERPARTY_TYPE_MAPPING.", tipeCP))
	}
	detail.PoolUsed = tipeEksposur

	// 6. Load LGD pool.
	pool, err := s.lgdRepo.GetByPool(ctx, tipeEksposur, periodeID)
	if err != nil {
		return decimal.Zero, detail, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca mst.lgd_basel", err)
	}
	if pool == nil {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeLGDLookupPoolNotFound,
			fmt.Sprintf("LGD pool untuk tipe_eksposur %s tidak ditemukan atau belum di-approve ALCO untuk periode %s.",
				tipeEksposur, periodeID))
	}
	detail.BaseLGD = pool.LGD

	// 7. Collateral haircut (Phase 1: 0).
	// In Phase 1 all instruments have no collateral.
	// Phase 2+: look up collateral type from mst.instrumen extension table.
	haircut := decimal.Zero
	detail.CollateralHaircut = haircut

	// 8. LGD_eff = LGD_pool × (1 - haircut). Rounded to 8dp (DEC-016).
	lgdEff := pool.LGD.Mul(one.Sub(haircut)).RoundBank(8)
	detail.LGDEffective = lgdEff
	detail.LGD = lgdEff

	return lgdEff, detail, nil
}

// GetLGDFromBatchParams resolves LGD for one instrument from pre-loaded BatchParams.
func GetLGDFromBatchParams(
	instrID uuid.UUID,
	inst InstrumenRow,
	cp CounterpartyRow,
	params *BatchParams,
	periodeID string,
) (decimal.Decimal, LGDDetail, error) {
	var detail LGDDetail
	detail.Warnings = []HelperWarning{}

	if inst.TipeInstrumen == "REKSADANA" {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeLGDLookupUseLookthrough,
			"Instrumen REKSADANA menggunakan mekanisme look-through ECL (P4-M4).")
	}

	detail.TipeCounterparty = cp.TipeCounterparty

	tipeEksposur, ok := params.LGDMapping[cp.TipeCounterparty]
	if !ok {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeLGDLookupMappingNotFound,
			fmt.Sprintf("Tipe counterparty '%s' tidak ada di LGD mapping.", cp.TipeCounterparty))
	}
	detail.PoolUsed = tipeEksposur

	pool, ok := params.LGDPools[tipeEksposur]
	if !ok {
		return decimal.Zero, detail, domainerrors.New(domainerrors.CodeLGDLookupPoolNotFound,
			fmt.Sprintf("LGD pool '%s' tidak tersedia untuk periode %s.", tipeEksposur, periodeID))
	}
	detail.BaseLGD = pool.LGD
	detail.CollateralHaircut = decimal.Zero
	lgdEff := pool.LGD.RoundBank(8)
	detail.LGDEffective = lgdEff
	detail.LGD = lgdEff
	return lgdEff, detail, nil
}
