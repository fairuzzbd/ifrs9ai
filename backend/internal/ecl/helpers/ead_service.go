// Package helpers — EAD computation service.
//
// Implements EADService per APP-C-PAR-003 decision tree (p4-m2-helpers.md §1.3).
//
// Formula (FSD-APP-C §4, formulas.md §"EAD"):
//
//	EAD_FCY = Outstanding_Principal_FCY + Accrued_Interest_FCY + (Undrawn × CCF)
//	EAD_IDR = EAD_FCY × kurs_BI_JISDOR(evaluationDate)
//
// Phase 1 simplification (OQ-E):
//
//	Undrawn = 0, CCF = 0 → EAD_FCY = Outstanding + Accrued
//
// Outstanding Principal sources (OQ-M2-3):
//   - DEPOSITO: mst.instrumen.nominal (bullet, no amortization)
//   - OBLIGASI: ecl.eir_amortization_schedule.principal_outstanding if P4-M5 available,
//     else mst.instrumen.nominal with OUTSTANDING_FALLBACK_TO_NOMINAL warning.
//
// Accrued Interest (OQ-M2-3 dependency on P4-M5):
//   - If ecl.eir_amortization_schedule available: bunga_akrual from latest row ≤ evaluationDate.
//   - If not available: 0 with ACCRUED_INTEREST_ZERO_EIR_SCHEDULE_MISSING warning.
//
// FX (OQ-M2-6 resolved: evaluationDate kurs, not tanggal_penempatan):
//   - IDR instruments: FX = 1 (no conversion), fxRate = nil.
//   - FCY instruments: kurs BI_JISDOR per evaluationDate.
//
// Precision (DEC-016): all money NUMERIC(20,4) → RoundHalfEven(4).
//
//	FX NUMERIC(20,8) → stored at 8dp.
package helpers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// eadService implements EADService.
type eadService struct {
	instrRepo InstrumenSnapshotRepo
	kursRepo  KursRepository
	ccfSvc    CCFLookupService
}

// NewEADService creates an EADService.
func NewEADService(instrRepo InstrumenSnapshotRepo, kursRepo KursRepository, ccfSvc CCFLookupService) EADService {
	return &eadService{instrRepo: instrRepo, kursRepo: kursRepo, ccfSvc: ccfSvc}
}

// ComputeEAD returns EAD_IDR and full breakdown for an instrument.
// See domain.go EADService for full contract.
func (s *eadService) ComputeEAD(ctx context.Context, instrumenID uuid.UUID, evaluationDate time.Time) (decimal.Decimal, EADBreakdown, error) {
	var bd EADBreakdown
	bd.Warnings = []HelperWarning{}

	// 1. Load instrument.
	inst, err := s.instrRepo.GetEADInputs(ctx, instrumenID)
	if err != nil {
		return decimal.Zero, bd, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca instrumen "+instrumenID.String(), err)
	}
	if inst == nil {
		return decimal.Zero, bd, domainerrors.New(domainerrors.CodeEADInstrumenNotFound,
			fmt.Sprintf("Instrumen %s tidak ditemukan di mst.instrumen.", instrumenID))
	}
	bd.Currency = inst.MatauangKode

	// 2. Guard FVTPL / FVOCI_ELECTION.
	if isECLNotApplicable(inst.KlasifikasiPsak71) {
		return decimal.Zero, bd, domainerrors.New(domainerrors.CodeInstrumentECLNotApplicable,
			"Instrumen FVTPL tidak memerlukan ECL/EAD calculation.")
	}

	// 3. Get FX rate if non-IDR.
	var fxRate decimal.Decimal = decimal.NewFromInt(1)
	if inst.MatauangKode != "IDR" {
		kr, err := s.kursRepo.GetByDate(ctx, inst.MatauangKode, evaluationDate)
		if err != nil {
			return decimal.Zero, bd, domainerrors.Wrap(domainerrors.CodeInternal,
				"gagal membaca kurs", err)
		}
		if kr == nil {
			return decimal.Zero, bd, domainerrors.New(domainerrors.CodeEADFXRateMissing,
				fmt.Sprintf("Kurs BI JISDOR untuk %s tidak tersedia per %s. "+
					"Upload kurs manual atau tunggu feed BI.",
					inst.MatauangKode, evaluationDate.Format("2006-01-02")))
		}
		if kr.WorkflowStatus != "APPROVED" {
			return decimal.Zero, bd, domainerrors.New(domainerrors.CodeEADFXRateNotApproved,
				fmt.Sprintf("Kurs %s per %s belum di-approve (status: %s). "+
					"Kurs harus APPROVED sebelum dipakai ECL.",
					inst.MatauangKode, evaluationDate.Format("2006-01-02"), kr.WorkflowStatus))
		}
		fxRate = kr.NilaiKurs
		bd.FXRate = &fxRate
		bd.FXSource = "BI_JISDOR"

		// F2 (OQ-M2-6): emit warning when FX rate is stale (> 3 calendar days old).
		// Business-day calculation deferred to Phase 2; calendar days used here per Phase 1 scope.
		staleDays := evaluationDate.Sub(kr.TanggalBerlaku).Hours() / 24
		if staleDays > 3 {
			bd.Warnings = append(bd.Warnings, HelperWarning{
				Code: WarnFXRateStale,
				Message: fmt.Sprintf("Kurs BI JISDOR untuk %s tertanggal %s (%.0f hari sebelum evaluationDate %s). "+
					"Pertimbangkan update kurs manual.",
					inst.MatauangKode,
					kr.TanggalBerlaku.Format("2006-01-02"),
					staleDays,
					evaluationDate.Format("2006-01-02")),
			})
		}
	}

	// 4. Outstanding principal.
	var outstanding decimal.Decimal
	eirRow, err := s.instrRepo.GetEIRScheduleRow(ctx, instrumenID, evaluationDate)
	if err != nil {
		return decimal.Zero, bd, domainerrors.Wrap(domainerrors.CodeInternal,
			"gagal membaca EIR schedule", err)
	}

	if eirRow != nil && eirRow.PrincipalOutstanding.IsPositive() {
		outstanding = eirRow.PrincipalOutstanding
	} else {
		// Fallback to nominal (OQ-M2-3).
		outstanding = inst.Nominal
		bd.Warnings = append(bd.Warnings, HelperWarning{
			Code: WarnOutstandingFallbackToNominal,
			Message: fmt.Sprintf("Outstanding fallback ke mst.instrumen.nominal karena "+
				"EIR schedule belum tersedia (P4-M5). Instrumen: %s.", instrumenID),
		})
	}

	// 5. Accrued interest.
	var accrued decimal.Decimal
	if eirRow != nil {
		accrued = eirRow.BungaAkrual
		bd.AccruedInterestSource = "EIR_SCHEDULE"
	} else {
		accrued = decimal.Zero
		bd.AccruedInterestSource = "ZERO_FALLBACK"
		bd.Warnings = append(bd.Warnings, HelperWarning{
			Code: WarnAccruedZeroEIRScheduleMissing,
			Message: fmt.Sprintf("Accrued Interest = 0 karena EIR schedule P4-M5 belum tersedia. "+
				"Instrumen: %s.", instrumenID),
		})
	}

	// 6. CCF and undrawn commitment.
	// Phase 1: CCF = 0, undrawn = 0 per OQ-E.
	ccf, _, err := s.ccfSvc.GetCCF(ctx, inst.TipeInstrumen)
	if err != nil {
		// Non-fatal: use 0 with warning (not a hard error for EAD).
		ccf = decimal.Zero
		bd.Warnings = append(bd.Warnings, HelperWarning{
			Code:    WarnCCFTypeNotInConfig,
			Message: "CCF lookup gagal; default 0 digunakan untuk EAD.",
		})
	}
	undrawn := decimal.Zero

	// 7. EAD_FCY = outstanding + accrued + (undrawn × ccf).
	// Round each component to 4dp (DEC-016 NUMERIC(20,4)).
	outstandingR := outstanding.RoundBank(4)
	accruedR := accrued.RoundBank(4)
	undrawnCCF := undrawn.Mul(ccf).RoundBank(4)

	eadFCY := outstandingR.Add(accruedR).Add(undrawnCCF)

	bd.OutstandingPrincipalFCY = outstandingR
	bd.AccruedInterestFCY = accruedR
	bd.CommittedUndrawnFCY = undrawn.RoundBank(4)
	bd.CCF = ccf.RoundBank(4)
	bd.EADFCY = eadFCY

	// 8. EAD_IDR = EAD_FCY × fxRate.
	eadIDR := eadFCY.Mul(fxRate).RoundBank(4)
	bd.EADIDR = eadIDR

	return eadIDR, bd, nil
}

// ComputeEADFromBatchParams computes EAD using pre-loaded BatchParams (anti-N+1).
func ComputeEADFromBatchParams(
	instrID uuid.UUID,
	inst InstrumenRow,
	params *BatchParams,
	evaluationDate time.Time,
) (decimal.Decimal, EADBreakdown, error) {
	var bd EADBreakdown
	bd.Currency = inst.MatauangKode
	bd.Warnings = []HelperWarning{}

	// FX.
	var fxRate decimal.Decimal = decimal.NewFromInt(1)
	if inst.MatauangKode != "IDR" {
		kr, ok := params.FXRates[inst.MatauangKode]
		if !ok {
			return decimal.Zero, bd, domainerrors.New(domainerrors.CodeEADFXRateMissing,
				fmt.Sprintf("Kurs BI JISDOR untuk %s tidak tersedia per %s.",
					inst.MatauangKode, evaluationDate.Format("2006-01-02")))
		}
		if kr.WorkflowStatus != "APPROVED" {
			return decimal.Zero, bd, domainerrors.New(domainerrors.CodeEADFXRateNotApproved,
				fmt.Sprintf("Kurs %s belum di-approve (status: %s).",
					inst.MatauangKode, kr.WorkflowStatus))
		}
		fxRate = kr.NilaiKurs
		bd.FXRate = &fxRate
		bd.FXSource = "BI_JISDOR"

		// F2 (OQ-M2-6): emit warning when FX rate is stale (> 3 calendar days old).
		staleDays := evaluationDate.Sub(kr.TanggalBerlaku).Hours() / 24
		if staleDays > 3 {
			bd.Warnings = append(bd.Warnings, HelperWarning{
				Code: WarnFXRateStale,
				Message: fmt.Sprintf("Kurs BI JISDOR untuk %s tertanggal %s (%.0f hari sebelum evaluationDate %s). "+
					"Pertimbangkan update kurs manual.",
					inst.MatauangKode,
					kr.TanggalBerlaku.Format("2006-01-02"),
					staleDays,
					evaluationDate.Format("2006-01-02")),
			})
		}
	}

	// Outstanding.
	var outstanding decimal.Decimal
	eirRow, hasEIR := params.EIRSchedules[instrID]
	if hasEIR && eirRow.PrincipalOutstanding.IsPositive() {
		outstanding = eirRow.PrincipalOutstanding
	} else {
		outstanding = inst.Nominal
		bd.Warnings = append(bd.Warnings, HelperWarning{
			Code:    WarnOutstandingFallbackToNominal,
			Message: fmt.Sprintf("Outstanding fallback ke nominal. Instrumen: %s.", instrID),
		})
	}

	// Accrued.
	var accrued decimal.Decimal
	if hasEIR {
		accrued = eirRow.BungaAkrual
		bd.AccruedInterestSource = "EIR_SCHEDULE"
	} else {
		accrued = decimal.Zero
		bd.AccruedInterestSource = "ZERO_FALLBACK"
		bd.Warnings = append(bd.Warnings, HelperWarning{
			Code:    WarnAccruedZeroEIRScheduleMissing,
			Message: fmt.Sprintf("Accrued Interest = 0 (P4-M5 belum tersedia). Instrumen: %s.", instrID),
		})
	}

	// CCF.
	ccf := decimal.Zero
	if v, ok := params.CCFTable[inst.TipeInstrumen]; ok {
		ccf = v
	}
	undrawn := decimal.Zero

	outstandingR := outstanding.RoundBank(4)
	accruedR := accrued.RoundBank(4)
	undrawnCCF := undrawn.Mul(ccf).RoundBank(4)
	eadFCY := outstandingR.Add(accruedR).Add(undrawnCCF)

	bd.OutstandingPrincipalFCY = outstandingR
	bd.AccruedInterestFCY = accruedR
	bd.CommittedUndrawnFCY = decimal.Zero
	bd.CCF = ccf.RoundBank(4)
	bd.EADFCY = eadFCY
	bd.EADIDR = eadFCY.Mul(fxRate).RoundBank(4)

	return bd.EADIDR, bd, nil
}
