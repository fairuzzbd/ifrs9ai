// Package eir — EIRService orchestrates the Newton-Raphson solver, schedule
// generation + persistence, and amendment re-estimation workflow.
//
// Precision: all IDR arithmetic uses shopspring/decimal (DEC-016 — never float64).
// Audit-in-tx: every mutation writes to aud.audit_log in the same DB transaction (DEC-018).
// auditWriter must NOT be nil — constructor panics if nil (per M3/M4 pattern).
//
// Decisions: DEC-013, DEC-016, DEC-017, DEC-018.
// Stories: APP-C-EIR-001..005.
// State machine: docs/state-machines/p4-m5-eir.md §1, §2, §3.
package eir

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Audit writer ─────────────────────────────────────────────────────────────

// AuditWriterIface is the minimal interface the service needs from the audit package.
// Constructor panics if nil (DEC-018 enforcement).
type AuditWriterIface interface {
	Write(ctx context.Context, tx *sql.Tx, event AuditEvent) error
}

// AuditEvent is the audit event passed to the audit writer.
type AuditEvent struct {
	ActorUserID uuid.UUID
	ActorRole   string
	Action      string
	EntityType  string
	EntityID    uuid.UUID
	BeforeJSON  interface{}
	AfterJSON   interface{}
	TenantID    string
}

// AuditWriterAdapter bridges *audit.Writer → AuditWriterIface.
type AuditWriterAdapter struct {
	w *audit.Writer
}

// NewAuditWriterAdapter creates an AuditWriterIface from *audit.Writer.
func NewAuditWriterAdapter(w *audit.Writer) *AuditWriterAdapter {
	return &AuditWriterAdapter{w: w}
}

// Write implements AuditWriterIface.
func (a *AuditWriterAdapter) Write(ctx context.Context, tx *sql.Tx, evt AuditEvent) error {
	return a.w.WithTx(tx).Write(ctx, audit.Event{
		Action:      evt.Action,
		EntityType:  evt.EntityType,
		EntityID:    evt.EntityID,
		Before:      evt.BeforeJSON,
		After:       evt.AfterJSON,
		ActorUserID: evt.ActorUserID.String(),
		ActorRole:   evt.ActorRole,
	})
}

// ─── Compute request / response ───────────────────────────────────────────────

// EIRComputeRequest is the input for EIRService.Compute.
type EIRComputeRequest struct {
	InstrumenID      uuid.UUID
	CashflowProjection []CashflowItem
	CouponRate       *decimal.Decimal // seed for solver; nil → 0.10
	PersistResult    bool             // if true, save eir_awal + audit
	ForceRecompute   bool             // bypass EIR_ALREADY_COMPUTED guard
	POCIMode         bool             // true = PD-adjusted cashflow
}

// ─── EIRService ───────────────────────────────────────────────────────────────

// EIRService orchestrates EIR computation (Story 1: APP-C-EIR-001).
type EIRService struct {
	db          *sql.DB
	instrRepo   InstrumenEIRRepoIface
	solver      *EIRSolver
	auditWriter AuditWriterIface
	logger      *slog.Logger
}

// NewEIRService creates an EIRService.
// Panics if auditWriter is nil (DEC-018 compliance guard).
func NewEIRService(db *sql.DB, instrRepo InstrumenEIRRepoIface, auditWriter AuditWriterIface, logger *slog.Logger) *EIRService {
	if auditWriter == nil {
		panic("eir.NewEIRService: auditWriter must not be nil (DEC-018)")
	}
	return &EIRService{
		db:          db,
		instrRepo:   instrRepo,
		solver:      NewEIRSolver(),
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// Compute validates the instrument, runs Newton-Raphson, optionally persists.
// Implements APP-C-EIR-001 Story 1.
//
// Precision: solver returns RoundBank(8); annual equivalent computed in decimal (DEC-016).
// Audit: EIR.COMPUTE (success) or EIR.COMPUTE_FAILED written in-transaction when PersistResult=true.
func (s *EIRService) Compute(ctx context.Context, req EIRComputeRequest, actorID uuid.UUID, actorRole string) (ComputeResult, error) {
	// 1. Load instrument
	inst, err := s.instrRepo.GetByID(ctx, req.InstrumenID)
	if err != nil {
		return ComputeResult{}, fmt.Errorf("eir.Compute: load instrumen: %w", err)
	}
	if inst == nil || inst.DeletedAt != nil {
		return ComputeResult{}, ErrEIRInstrumenNotFound(req.InstrumenID.String())
	}

	// 2. FVTPL / FVOCI_ELECTION rejection (OQ-H; EIR only for AC + FVOCI debt)
	if inst.KlasifikasiPsak71 == "FVTPL" || inst.KlasifikasiPsak71 == "FVOCI_ELECTION" {
		return ComputeResult{}, ErrEIRInstrumenFVTPLNoEIR(inst.KlasifikasiPsak71)
	}
	if !inst.EIRMethodFlag {
		return ComputeResult{}, ErrEIRInstrumenFVTPLNoEIR("eir_method_flag=FALSE")
	}

	// 3. Duplicate guard for persistResult=true
	if req.PersistResult && !req.ForceRecompute && inst.EIRAwal != nil {
		return ComputeResult{}, ErrEIRAlreadyComputed()
	}

	// 4. POCI cross-field validation
	if req.POCIMode && !inst.FlagPOCI {
		return ComputeResult{}, ErrEIRPOCIRequiresPDAdjustedCF()
	}
	if !req.POCIMode && inst.FlagPOCI {
		return ComputeResult{}, ErrEIRPOCIRequiresPDAdjustedCF()
	}

	// 5. Cashflow validation
	if len(req.CashflowProjection) < 2 {
		return ComputeResult{}, ErrEIRCashflowInvalid("Minimal 2 cashflow items (CF_0 negatif + setidaknya 1 inflow)")
	}

	// 6. Determine seed: provided couponRate OR 0.10 (DEC-013)
	var seed *decimal.Decimal
	if req.CouponRate != nil && req.CouponRate.GreaterThan(zero) {
		seed = req.CouponRate
	} else if inst.Kupon != nil {
		seed = inst.Kupon
	}

	// 7. Run Newton-Raphson
	eirPerPeriod, detail, solveErr := s.solver.Solve(req.CashflowProjection, seed)

	// 8. If persistResult: open tx for audit + update
	if req.PersistResult {
		tx, txErr := s.db.BeginTx(ctx, nil)
		if txErr != nil {
			return ComputeResult{}, fmt.Errorf("eir.Compute: begin tx: %w", txErr)
		}
		defer rollbackTx(ctx, tx, s.logger)

		if solveErr != nil {
			// Write EIR.COMPUTE_FAILED audit even on solver failure
			_ = s.auditWriter.Write(ctx, tx, AuditEvent{
				ActorUserID: actorID,
				ActorRole:   actorRole,
				Action:      "EIR.COMPUTE_FAILED",
				EntityType:  "mst.instrumen",
				EntityID:    req.InstrumenID,
				BeforeJSON:  map[string]any{"eir_awal": nil},
				AfterJSON:   map[string]any{"error": solveErr.Error()},
				TenantID:    inst.TenantID,
			})
			_ = tx.Commit()
			return ComputeResult{}, solveErr
		}

		// Update eir_awal on mst.instrumen
		if err := s.instrRepo.UpdateEIRAwal(ctx, tx, req.InstrumenID, eirPerPeriod, actorID); err != nil {
			return ComputeResult{}, fmt.Errorf("eir.Compute: update eir_awal: %w", err)
		}

		// Write EIR.COMPUTE audit
		eirType := EIRTypeStandard
		if req.POCIMode {
			eirType = EIRTypeCreditAdjusted
		}
		if err := s.auditWriter.Write(ctx, tx, AuditEvent{
			ActorUserID: actorID,
			ActorRole:   actorRole,
			Action:      "EIR.COMPUTE",
			EntityType:  "mst.instrumen",
			EntityID:    req.InstrumenID,
			BeforeJSON:  map[string]any{"eir_awal": nil},
			AfterJSON: map[string]any{
				"eir_awal":   eirPerPeriod.StringFixed(8),
				"eir_type":   eirType,
				"poci_mode":  req.POCIMode,
				"iterations": detail.IterationsUsed,
				"residual":   detail.ConvergenceResidual.String(),
			},
			TenantID: inst.TenantID,
		}); err != nil {
			return ComputeResult{}, fmt.Errorf("eir.Compute: audit write: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return ComputeResult{}, fmt.Errorf("eir.Compute: commit: %w", err)
		}
	} else if solveErr != nil {
		return ComputeResult{}, solveErr
	}

	// 9. Compute annual equivalent: (1+r)^periodesPerYear - 1
	// For ACT/365 with a generic instrument, we use periodesPerYear = 1
	// (annual equivalent = annualized form of per-period rate).
	// This is a conservative approach; for monthly instruments periodePerYear=12 etc.
	// The service stores eirPerPeriod; annual equivalent is informational.
	annualEquiv := decimalPow(one.Add(eirPerPeriod), one).Sub(one).RoundBank(8)

	eirType := EIRTypeStandard
	if req.POCIMode {
		eirType = EIRTypeCreditAdjusted
	}

	return ComputeResult{
		InstrumenID:         req.InstrumenID,
		EIRPerPeriod:        eirPerPeriod,
		EIRAnnualEquivalent: annualEquiv,
		IterationsUsed:      detail.IterationsUsed,
		ConvergenceResidual: detail.ConvergenceResidual,
		FlagPOCI:            inst.FlagPOCI,
		EIRType:             eirType,
		Persisted:           req.PersistResult,
		ComputedAt:          time.Now(),
	}, nil
}

// rollbackTx rolls back tx and logs any error.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		if logger != nil {
			logger.WarnContext(ctx, "eir: tx rollback failed", "error", err)
		}
	}
}

// ─── ScheduleService ──────────────────────────────────────────────────────────

// GenerateScheduleRequest is the input for ScheduleService.Generate.
type GenerateScheduleRequest struct {
	InstrumenID      uuid.UUID
	CashflowProjection []CashflowItem
	ForceRegenerate  bool
}

// ScheduleService handles amortisation schedule generation and lookup.
// Implements APP-C-EIR-002 (generate) and APP-C-EIR-003 (read/DataTable).
type ScheduleService struct {
	db          *sql.DB
	instrRepo   InstrumenEIRRepoIface
	schedRepo   EIRScheduleRepoIface
	solver      *EIRSolver
	auditWriter AuditWriterIface
	logger      *slog.Logger
}

// NewScheduleService creates a ScheduleService.
// Panics if auditWriter is nil.
func NewScheduleService(
	db *sql.DB,
	instrRepo InstrumenEIRRepoIface,
	schedRepo EIRScheduleRepoIface,
	auditWriter AuditWriterIface,
	logger *slog.Logger,
) *ScheduleService {
	if auditWriter == nil {
		panic("eir.NewScheduleService: auditWriter must not be nil (DEC-018)")
	}
	return &ScheduleService{
		db:          db,
		instrRepo:   instrRepo,
		schedRepo:   schedRepo,
		solver:      NewEIRSolver(),
		auditWriter: auditWriter,
		logger:      logger,
	}
}

// Generate builds and persists an amortisation schedule from origination to maturity.
// Formula (state-machine doc §2, formulas.md §EIR):
//
//	opening_carrying_1 = nominal + biaya_transaksi_capitalized
//	For t = 1..N:
//	  pendapatan_bunga_eir_t = opening_carrying_{t-1} × eir_per_periode
//	  cash_inflow_t          = kupon_kontraktual_t (from cashflowProjection)
//	  amortisasi_p_d_t       = pendapatan_bunga_eir_t - cash_inflow_t
//	  pelunasan_pokok_t      = nominal if t==N else 0
//	  closing_carrying_t     = opening_carrying_{t-1} + amortisasi_p_d_t - pelunasan_pokok_t
//
// All amounts rounded HALF_EVEN (RoundBank) to 4 decimal places (DEC-016).
// Schedule rows are immutable after INSERT (DB trigger + service guard).
// Audit: EIR.SCHEDULE_GENERATED written in-transaction.
func (s *ScheduleService) Generate(ctx context.Context, req GenerateScheduleRequest, actorID uuid.UUID, actorRole string) (ScheduleGenerateResult, error) {
	// 1. Load instrument
	inst, err := s.instrRepo.GetByID(ctx, req.InstrumenID)
	if err != nil {
		return ScheduleGenerateResult{}, fmt.Errorf("eir.Generate: load instrumen: %w", err)
	}
	if inst == nil || inst.DeletedAt != nil {
		return ScheduleGenerateResult{}, ErrEIRInstrumenNotFound(req.InstrumenID.String())
	}

	// 2. EIR must be computed first
	if inst.EIRAwal == nil {
		return ScheduleGenerateResult{}, ErrEIRNotYetComputed()
	}

	// 3. Duplicate guard
	if !req.ForceRegenerate {
		hasRows, err := s.schedRepo.HasActiveRows(ctx, req.InstrumenID)
		if err != nil {
			return ScheduleGenerateResult{}, fmt.Errorf("eir.Generate: check existing rows: %w", err)
		}
		if hasRows {
			return ScheduleGenerateResult{}, ErrEIRDuplicateScheduleVersion(inst.KodeInstrumen)
		}
	}

	// 4. Build schedule rows from cashflow projection
	eirPerPeriod := *inst.EIRAwal
	rows, closingDelta := buildScheduleRows(req.InstrumenID, eirPerPeriod, req.CashflowProjection, inst, actorID)

	if len(rows) == 0 {
		return ScheduleGenerateResult{}, ErrEIRCashflowInvalid("Cashflow projection tidak menghasilkan schedule rows")
	}

	// 5. Persist in single transaction + audit
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ScheduleGenerateResult{}, fmt.Errorf("eir.Generate: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err := s.schedRepo.InsertBatch(ctx, tx, rows); err != nil {
		return ScheduleGenerateResult{}, fmt.Errorf("eir.Generate: insert batch: %w", err)
	}

	// Audit: EIR.SCHEDULE_GENERATED (DEC-018: same tx)
	if err := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "EIR.SCHEDULE_GENERATED",
		EntityType:  "mst.instrumen",
		EntityID:    req.InstrumenID,
		AfterJSON: map[string]any{
			"total_rows":      len(rows),
			"eir_per_period":  eirPerPeriod.StringFixed(8),
			"closing_delta":   closingDelta.StringFixed(4),
		},
		TenantID: inst.TenantID,
	}); err != nil {
		return ScheduleGenerateResult{}, fmt.Errorf("eir.Generate: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ScheduleGenerateResult{}, fmt.Errorf("eir.Generate: commit: %w", err)
	}

	// Log warning if closing delta > IDR 1 (per state machine doc §2)
	one_idr := decimal.NewFromFloat(1.0)
	if closingDelta.GreaterThan(one_idr) {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "eir.Generate: closing carrying delta > IDR 1",
				"instrumen_id", req.InstrumenID,
				"delta", closingDelta.StringFixed(4))
		}
	}

	return ScheduleGenerateResult{
		InstrumenID:          req.InstrumenID,
		TotalRows:            len(rows),
		EIRPerPeriod:         eirPerPeriod,
		OpeningCarryingFirst: rows[0].OpeningCarrying,
		ClosingCarryingLast:  rows[len(rows)-1].ClosingCarrying,
		ClosingRoundingDelta: closingDelta,
		GeneratedAt:          time.Now(),
		Rows:                 rows,
	}, nil
}

// buildScheduleRows constructs the amortisation schedule rows from cashflow data.
// Formula per state-machine doc §2. All amounts rounded HALF_EVEN to 4 dp (DEC-016).
//
// The cashflow projection provides:
//   - CF[0]: initial outflow (negative) — opening carrying = abs(CF[0])
//   - CF[1..N-1]: coupon cash inflows
//   - CF[N]: last coupon + principal repayment
//
// Principal repayment = abs(CF[0]) - coupon portion of CF[N].
func buildScheduleRows(instrumenID uuid.UUID, eirPerPeriod decimal.Decimal, cfs []CashflowItem, inst *InstrumenForEIR, createdBy uuid.UUID) ([]ScheduleRow, decimal.Decimal) {
	if len(cfs) < 2 {
		return nil, zero
	}

	// Opening carrying = abs(initial outflow) = nominal + biaya_transaksi
	openingCarrying := cfs[0].AmountIDR.Abs().RoundBank(4)

	// Determine principal (last CF likely includes principal return)
	// We treat nominal as the principal repayment (bullet bond assumption)
	nominal := inst.Nominal

	tenantID := inst.TenantID

	rows := make([]ScheduleRow, 0, len(cfs)-1)
	carrying := openingCarrying

	for i := 1; i < len(cfs); i++ {
		cf := cfs[i]
		cashInflow := cf.AmountIDR.RoundBank(4)

		// pendapatan_bunga_eir = opening × eir_per_periode  [HALF_EVEN 4dp]
		pendapatan := carrying.Mul(eirPerPeriod).RoundBank(4)

		// amortisasi_p_d = pendapatan - cash_inflow
		amortisasi := pendapatan.Sub(cashInflow).RoundBank(4)

		// pelunasan_pokok: only on last period (bullet bond)
		var pelunasan decimal.Decimal
		isLast := i == len(cfs)-1
		if isLast {
			// Remove principal component from cash inflow
			couponLast := cashInflow.Sub(nominal)
			if couponLast.LessThan(zero) {
				// CF last might be principal + coupon or just principal
				pelunasan = nominal
			} else {
				pelunasan = nominal
			}
		}

		// closing_carrying = opening + amortisasi - pelunasan  [HALF_EVEN 4dp]
		closing := carrying.Add(amortisasi).Sub(pelunasan).RoundBank(4)

		row := ScheduleRow{
			ID:                 uuid.New(),
			InstrumenID:        instrumenID,
			PeriodeSeq:         i,
			TanggalPosting:     cf.Date,
			OpeningCarrying:    carrying,
			CashInflow:         cashInflow,
			PendapatanBungaEIR: pendapatan,
			AmortisasiPD:       amortisasi,
			PelunasanPokok:     pelunasan,
			ClosingCarrying:    closing,
			EIRPeriode:         eirPerPeriod,
			StageSaatPosting:   "STAGE_1",
			StatusPosting:      "PROYEKSI",
			FlagPOCI:           inst.FlagPOCI,
			CreatedBy:          createdBy,
			UpdatedBy:          createdBy,
			TenantID:           tenantID,
		}
		rows = append(rows, row)

		// Update carrying for next period
		carrying = closing
	}

	// closing delta = abs(closing carrying of last row)
	var closingDelta decimal.Decimal
	if len(rows) > 0 {
		closingDelta = rows[len(rows)-1].ClosingCarrying.Abs()
	}
	return rows, closingDelta
}

// ─── re-estimation helpers ────────────────────────────────────────────────────

// computeCatchUpAdjustment computes the NPV difference at amendmentDate.
// NPV_difference = Σ (new_CF_t - old_CF_t) / (1 + eir_old)^t
// For M5 stub, we compute it as the difference between old and new schedule's
// opening carrying at amendment date.
func computeCatchUpAdjustment(oldEIR, newEIR decimal.Decimal, openingCarryingAtAmendment decimal.Decimal) decimal.Decimal {
	// Simplified: catch_up = carrying × (newEIR - oldEIR)
	// Full IFRS 9 §5.4.3 catch-up = difference of gross carrying amounts
	// recalculated using old vs new EIR, discounted to amendment date.
	// This simplified version is acceptable for M5 stub; M7 refines.
	diff := newEIR.Sub(oldEIR)
	return openingCarryingAtAmendment.Mul(diff).RoundBank(4)
}

// isEIRApplicable returns true if the instrument classification requires EIR.
func isEIRApplicable(klasifikasi string, methodFlag bool) bool {
	return methodFlag && (klasifikasi == "AC" || klasifikasi == "FVOCI")
}

// domainerrors shortcut to avoid unused import
var _ = domainerrors.CodeEIRNonConvergent
