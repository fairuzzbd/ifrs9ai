package pocidelta

// service.go — POCI Delta ECL Service.
// TX boundary lives here; repos never open transactions.
//
// Business rules enforced:
//   - POCI instruments: staging engine BYPASSED, stage_marker = 'POCI'
//   - Baseline: WORM — ValidateBaselineNotExists before INSERT (DEC-018)
//   - Delta: current_ecl − baseline_ecl (shopspring/decimal, HALF_EVEN)
//   - Jurnal: direction-based event code; ValidateJurnalDirection bug-guard before post
//   - Idempotency: check GetDeltaLogByRunAndInstrumen before INSERT
//   - Periode lock: ValidatePeriodeLocked before jurnal post
//   - Audit: all events in-transaction (DEC-018)
//   - Large delta: alert to ROLE-CFO if |delta| > POCI_LARGE_DELTA_THRESHOLD (once per run×instrumen)
//   - Baseline violation attempt: audit even on rejection (S1-AC2)
//
// Citation: PSAK 71 §5.5.13-14, FSD-APP-C-ECL-EIR-v1.0 §5-6, DEC-010/013/016/017/018.

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// systemActorID is the UUID for the cron / system service account in audit logs.
var systemActorID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// DBTxBeginner is the interface satisfied by *sql.DB (via a thin adapter)
// or any test double. Service uses it to open transactions without holding
// *sql.DB directly (avoids import cycle and enables unit-test substitution).
type DBTxBeginner interface {
	BeginTxContext(ctx context.Context) (*sql.Tx, error)
}

// Service owns POCI delta ECL business logic.
type Service struct {
	repo    Repository
	poster  JurnalPoster
	audit   *audit.Writer
	logger  *slog.Logger
	txBegin func(ctx context.Context) (*sql.Tx, error)
}

// NewService creates a new pocidelta Service without a real DB transaction beginner.
// txBegin will return an error — use NewServiceWithDB in production.
func NewService(
	repo Repository,
	poster JurnalPoster,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if poster == nil {
		poster = NewNoopJurnalPoster(logger)
	}
	svc := &Service{
		repo:   repo,
		poster: poster,
		audit:  auditWriter,
		logger: logger,
	}
	// Default stub — returns explicit error; replaced by NewServiceWithDB in production.
	svc.txBegin = func(_ context.Context) (*sql.Tx, error) {
		return nil, fmt.Errorf("txBegin: DBProvider not wired — inject via NewServiceWithDB in main.go (P5-M10)")
	}
	return svc
}

// NewServiceWithDB creates a production-ready Service wired with a real DBTxBeginner.
// Call this from main.go instead of NewService when a real *sql.DB is available.
func NewServiceWithDB(
	repo Repository,
	db DBTxBeginner,
	poster JurnalPoster,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	svc := NewService(repo, poster, auditWriter, logger)
	if db != nil {
		svc.txBegin = db.BeginTxContext
	}
	return svc
}

// ─── CaptureBaseline ──────────────────────────────────────────────────────────

// CaptureBaseline captures the immutable lifetime ECL baseline for a POCI instrument.
// Called from penempatan approve flow (P5-M1) within the same DB transaction.
//
// Audit: POCI.BASELINE_CAPTURED (success) or POCI.BASELINE_VIOLATION_ATTEMPT (on rejection).
// Both audits are in-transaction (S1-AC1, S1-AC2).
func (s *Service) CaptureBaseline(
	ctx context.Context,
	tx *sql.Tx,
	req CaptureBaselineRequest,
	actor uuid.UUID,
	tenantID string,
) (*Baseline, error) {
	// Validate request fields
	if err := ValidateCaptureBaselineRequest(req); err != nil {
		return nil, err
	}

	// Load instrumen to verify is_poci
	info, err := s.repo.GetInstrumenPociInfo(ctx, req.InstrumenID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("CaptureBaseline: GetInstrumenPociInfo: %w", err)
	}
	if info == nil {
		return nil, fmt.Errorf("VALIDATION_FAILED: instrumen %s tidak ditemukan", req.InstrumenID)
	}
	if err := ValidateInstrumenIsPoci(req.InstrumenID, info.IsPoci); err != nil {
		return nil, err
	}

	// WORM check — load existing baseline
	existing, err := s.repo.GetBaselineByInstrumen(ctx, req.InstrumenID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("CaptureBaseline: GetBaselineByInstrumen: %w", err)
	}
	if err := ValidateBaselineNotExists(req.InstrumenID, existing); err != nil {
		// Audit violation attempt even on rejection (S1-AC2 security requirement)
		s.writeAuditInTx(ctx, tx, audit.Event{
			Action:     "POCI.BASELINE_VIOLATION_ATTEMPT",
			EntityType: "ecl.poci_baseline",
			EntityID:   req.InstrumenID,
			After: map[string]interface{}{
				"instrumen_id":         req.InstrumenID,
				"attempt_ecl":          req.LifetimeECLAtOrigination.StringFixed(4),
				"existing_origination": existing.TanggalBaseline.Format("2006-01-02"),
			},
		})
		return nil, err
	}

	// Build baseline entity
	tanggalBaseline := time.Now().UTC().Truncate(24 * time.Hour) // DATE
	if req.TanggalBaseline != nil {
		if parsed, pErr := time.Parse("2006-01-02", *req.TanggalBaseline); pErr == nil {
			tanggalBaseline = parsed
		}
	}
	b := &Baseline{
		ID:                       uuid.New(),
		InstrumenID:              req.InstrumenID,
		TanggalBaseline:         tanggalBaseline,
		LifetimeECLAtOrigination: req.LifetimeECLAtOrigination.RoundBank(4),
		CashflowExpektasiJsonb:  req.CashflowExpektasiJsonb,
		CreditAdjustedEIR:       req.CreditAdjustedEIR.RoundBank(8),
		OriginationDate:         tanggalBaseline,
		CreatedAt:               time.Now().UTC(),
		CreatedBy:               actor,
		TenantID:                tenantID,
	}

	if err := s.repo.InsertBaseline(ctx, tx, b); err != nil {
		return nil, fmt.Errorf("CaptureBaseline: InsertBaseline: %w", err)
	}

	// Audit in-transaction (S1-AC1)
	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "POCI.BASELINE_CAPTURED",
		EntityType: "ecl.poci_baseline",
		EntityID:   b.ID,
		After: map[string]interface{}{
			"instrumen_id":               b.InstrumenID,
			"lifetime_ecl_at_origination": b.LifetimeECLAtOrigination.StringFixed(4),
			"credit_adjusted_eir":        b.CreditAdjustedEIR.StringFixed(8),
			"tanggal_baseline":           b.TanggalBaseline.Format("2006-01-02"),
		},
	})

	return b, nil
}

// ─── ComputeDeltaForCalcRun ───────────────────────────────────────────────────

// ComputeDeltaForCalcRun computes and persists POCI delta ECL for all POCI
// instruments in a given calc run, then posts jurnal entries for non-zero deltas.
//
// Per-instrument errors are recorded in the returned CalcRunErrors slice;
// the batch does NOT halt on individual failures (S2-AC3).
// Idempotency: duplicate per (calc_run_id, instrumen_id) → skip + log (S2-AC4).
//
// Audit: POCI.DELTA_COMPUTED + POCI.DELTA_POSTED per instrument in-transaction.
func (s *Service) ComputeDeltaForCalcRun(
	ctx context.Context,
	calcRunID uuid.UUID,
	actor uuid.UUID,
	tenantID string,
) ([]CalcRunError, error) {
	// Verify calc run is SEALED or COMPLETED
	runStatus, err := s.repo.GetCalcRunStatus(ctx, calcRunID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ComputeDeltaForCalcRun: GetCalcRunStatus: %w", err)
	}
	if vErr := ValidateCalcRunSealed(calcRunID, runStatus); vErr != nil {
		return nil, vErr
	}

	// Get all POCI instruments in the run
	instrumens, err := s.repo.ListPociInstrumenByCalcRun(ctx, calcRunID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("ComputeDeltaForCalcRun: ListPociInstrumenByCalcRun: %w", err)
	}

	threshold, _ := s.repo.GetLargeDeltaThreshold(ctx, tenantID)
	var calcErrors []CalcRunError

	for _, inst := range instrumens {
		if cErr := s.processOnePociInstrumen(ctx, calcRunID, inst, threshold, actor, tenantID); cErr != nil {
			calcErrors = append(calcErrors, *cErr)
			s.logger.WarnContext(ctx, "ComputeDeltaForCalcRun: instrument error",
				"instrumen_id", inst.ID,
				"error_code", cErr.ErrorCode,
			)
		}
	}

	return calcErrors, nil
}

// processOnePociInstrumen handles delta computation for one POCI instrument.
// Returns non-nil CalcRunError on per-instrument failure (does not halt batch).
func (s *Service) processOnePociInstrumen(
	ctx context.Context,
	calcRunID uuid.UUID,
	inst InstrumenPociInfo,
	largeDeltaThreshold decimal.Decimal,
	actor uuid.UUID,
	tenantID string,
) *CalcRunError {
	// Idempotency check — skip if already computed for this (run × instrumen)
	existing, err := s.repo.GetDeltaLogByRunAndInstrumen(ctx, calcRunID, inst.ID, tenantID)
	if err != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: err.Error()}
	}
	if existing != nil {
		return &CalcRunError{
			InstrumenID: inst.ID,
			ErrorCode:   CodePociDeltaDuplicate,
			ErrorDetail: fmt.Sprintf("delta_log sudah ada untuk (calc_run_id=%s, instrumen_id=%s)", calcRunID, inst.ID),
		}
	}

	// Load baseline
	baseline, err := s.repo.GetBaselineByInstrumen(ctx, inst.ID, tenantID)
	if err != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: err.Error()}
	}
	if baseline == nil {
		return &CalcRunError{
			InstrumenID: inst.ID,
			ErrorCode:   CodePociBaselineMissing,
			ErrorDetail: fmt.Sprintf("ecl.poci_baseline tidak ada untuk instrumen %s. Pastikan penempatan POCI sudah di-approve (P5-M10-S1).", inst.ID),
		}
	}

	// Load current ECL from Phase 4 calc_run_result_line
	currentECL, err := s.repo.GetCurrentECLForPociInstrumen(ctx, calcRunID, inst.ID, tenantID)
	if err != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: err.Error()}
	}

	// Compute delta (pure, decimal)
	delta, dir := ComputeDelta(currentECL, baseline.LifetimeECLAtOrigination)

	// Load cumulative prior delta
	priorCumulative, _ := s.repo.GetCumulativeDelta(ctx, inst.ID, time.Now().UTC(), tenantID)

	// B2: Fetch periode_bulanan_id from the calc run, then validate it is not locked.
	periodeBulananID, err := s.repo.GetPeriodeBulananIDForCalcRun(ctx, calcRunID, tenantID)
	if err != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: err.Error()}
	}
	periodeStatus, err := s.repo.GetPeriodeStatus(ctx, periodeBulananID, tenantID)
	if err != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: err.Error()}
	}

	// Build DeltaLog row
	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)
	status := StatusComputed
	if dir == DirectionZero {
		status = StatusSkippedZero
	}

	dl := &DeltaLog{
		ID:                   uuid.New(),
		CalcRunID:            calcRunID,
		InstrumenID:          inst.ID,
		TanggalCompute:      today,
		BaselineECL:         baseline.LifetimeECLAtOrigination,
		CurrentECL:          currentECL,
		DeltaECL:            delta,
		Direction:           dir,
		PriorDeltaCumulative: &priorCumulative,
		PeriodeBulananID:    &periodeBulananID,
		Status:              status,
		CreatedAt:           now,
		CreatedBy:           actor,
		UpdatedAt:           now,
		UpdatedBy:           actor,
		RowVersion:          1,
		TenantID:            tenantID,
	}

	// B2: Validate periode is not locked before posting jurnal.
	if periodeErr := ValidatePeriodeLocked(periodeStatus); periodeErr != nil {
		// Periode is closed — record delta but skip jurnal post.
		dl.Status = StatusBlockedPeriodeClosed
		// Open tx to persist the delta log record for audit trail.
		tx, txErr := s.txBegin(ctx)
		if txErr != nil {
			return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: txErr.Error()}
		}
		defer func() { _ = tx.Rollback() }()
		if insertErr := s.repo.InsertDeltaLog(ctx, tx, dl); insertErr != nil {
			return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: insertErr.Error()}
		}
		s.writeAuditInTx(ctx, tx, audit.Event{
			Action:     "POCI.DELTA_BLOCKED_PERIODE_CLOSED",
			EntityType: "ecl.poci_delta_log",
			EntityID:   dl.ID,
			After: map[string]interface{}{
				"calc_run_id":        calcRunID,
				"instrumen_id":       inst.ID,
				"delta_ecl":          dl.DeltaECL.StringFixed(4),
				"periode_bulanan_id": periodeBulananID,
				"periode_status":     periodeStatus,
			},
		})
		_ = tx.Commit()
		return &CalcRunError{
			InstrumenID: inst.ID,
			ErrorCode:   "POCI_PERIODE_LOCKED",
			ErrorDetail: periodeErr.Error(),
		}
	}

	// Open transaction for atomic insert + audit + jurnal
	tx, txErr := s.txBegin(ctx)
	if txErr != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: txErr.Error()}
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.repo.InsertDeltaLog(ctx, tx, dl); err != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: err.Error()}
	}

	// Audit POCI.DELTA_COMPUTED in-transaction
	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "POCI.DELTA_COMPUTED",
		EntityType: "ecl.poci_delta_log",
		EntityID:   dl.ID,
		After: map[string]interface{}{
			"calc_run_id":        calcRunID,
			"instrumen_id":       inst.ID,
			"baseline_ecl":       dl.BaselineECL.StringFixed(4),
			"current_ecl":        dl.CurrentECL.StringFixed(4),
			"delta_ecl":          dl.DeltaECL.StringFixed(4),
			"direction":          string(dir),
			"periode_bulanan_id": periodeBulananID,
		},
	})

	// Post jurnal if direction != ZERO
	if dir != DirectionZero {
		if jErr := s.postJurnalForDelta(ctx, tx, dl, actor, tenantID); jErr != nil {
			return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: jErr.Error()}
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return &CalcRunError{InstrumenID: inst.ID, ErrorCode: "INTERNAL", ErrorDetail: commitErr.Error()}
	}

	// Large delta alert (outside transaction — notification channel)
	if delta.Abs().GreaterThan(largeDeltaThreshold) {
		s.logger.WarnContext(ctx, "POCI large delta detected",
			"instrumen_id", inst.ID,
			"calc_run_id", calcRunID,
			"delta_ecl", delta.StringFixed(4),
			"direction", string(dir),
			"threshold", largeDeltaThreshold.StringFixed(4),
		)
		// TODO(P5-M10): wire notification channel to ROLE-CFO (S5-AC3)
	}

	return nil
}

// postJurnalForDelta calls jurnal_poster after ValidateJurnalDirection guard.
func (s *Service) postJurnalForDelta(
	ctx context.Context,
	tx *sql.Tx,
	dl *DeltaLog,
	actor uuid.UUID,
	tenantID string,
) error {
	// Bug guard: direction must match delta_ecl sign (S3-AC4)
	if err := ValidateJurnalDirection(dl.DeltaECL, dl.Direction); err != nil {
		// Audit direction mismatch
		s.writeAuditInTx(ctx, tx, audit.Event{
			Action:     "POCI.DIRECTION_MISMATCH_DETECTED",
			EntityType: "ecl.poci_delta_log",
			EntityID:   dl.ID,
			After: map[string]interface{}{
				"delta_ecl": dl.DeltaECL.StringFixed(4),
				"direction": string(dl.Direction),
				"error":     err.Error(),
			},
		})
		return err
	}

	eventCode, resolveErr := ResolveJurnalEventCode(dl.Direction)
	if resolveErr != nil {
		// Should not reach here after ValidateJurnalDirection check
		return resolveErr
	}

	amount := AbsDeltaForJurnal(dl.DeltaECL)
	req := PociDeltaPostRequest{
		EventCode:      eventCode,
		InstrumenID:    dl.InstrumenID,
		DeltaLogID:     dl.ID,
		CalcRunID:      dl.CalcRunID,
		TanggalCompute: dl.TanggalCompute,
		AmountIDR:      amount,
		Direction:      dl.Direction,
		ActorID:        actor,
		TenantID:       tenantID,
		IdempotencyKey: fmt.Sprintf("%s:%s:POCI_ECL_DELTA", dl.CalcRunID, dl.InstrumenID),
	}

	result, postErr := s.poster.PostPociDelta(ctx, tx, req)
	if postErr != nil {
		return fmt.Errorf("PostPociDelta: %w", postErr)
	}

	// Update delta_log status to POSTED
	if upErr := s.repo.UpdateDeltaLogStatus(ctx, tx, dl.ID, dl.TanggalCompute,
		StatusPosted, &result.JurnalHeaderID, actor); upErr != nil {
		return fmt.Errorf("UpdateDeltaLogStatus: %w", upErr)
	}

	// Audit POCI.DELTA_POSTED in-transaction
	s.writeAuditInTx(ctx, tx, audit.Event{
		Action:     "POCI.DELTA_POSTED",
		EntityType: "ecl.poci_delta_log",
		EntityID:   dl.ID,
		After: map[string]interface{}{
			"calc_run_id":      dl.CalcRunID,
			"instrumen_id":     dl.InstrumenID,
			"delta_ecl":        dl.DeltaECL.StringFixed(4),
			"direction":        string(dl.Direction),
			"jurnal_header_id": result.JurnalHeaderID,
		},
	})

	return nil
}

// ─── Read operations ──────────────────────────────────────────────────────────

// ListBaseline returns POCI baselines with sort/filter/export.
func (s *Service) ListBaseline(ctx context.Context, q listquery.Query, tenantID string) ([]Baseline, Pagination, error) {
	return s.repo.ListBaselines(ctx, q, tenantID)
}

// GetBaselineByInstrumen returns the baseline for one instrument or nil if not found.
func (s *Service) GetBaselineByInstrumen(ctx context.Context, instrumenID uuid.UUID, tenantID string) (*Baseline, error) {
	b, err := s.repo.GetBaselineByInstrumen(ctx, instrumenID, tenantID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, fmt.Errorf("%s: instrumen %s tidak memiliki POCI baseline", CodePociBaselineMissing, instrumenID)
	}
	return b, nil
}

// ListDeltaLog returns delta log rows with sort/filter/export.
func (s *Service) ListDeltaLog(ctx context.Context, q listquery.Query, tenantID string) ([]DeltaLog, Pagination, error) {
	return s.repo.ListDeltaLogs(ctx, q, tenantID)
}

// GetDeltaHistory returns cumulative delta history for one instrument.
func (s *Service) GetDeltaHistory(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, tenantID string) ([]DeltaLog, Pagination, error) {
	// Verify baseline exists
	b, err := s.repo.GetBaselineByInstrumen(ctx, instrumenID, tenantID)
	if err != nil {
		return nil, Pagination{}, err
	}
	if b == nil {
		return nil, Pagination{},
			fmt.Errorf("%s: instrumen %s tidak memiliki baseline POCI", CodePociBaselineMissing, instrumenID)
	}
	return s.repo.GetDeltaHistoryByInstrumen(ctx, instrumenID, q, tenantID)
}

// GetDeltaSummary returns MTD/YTD aggregate delta per portofolio.
func (s *Service) GetDeltaSummary(ctx context.Context, portofolioID *uuid.UUID, year, month int, tenantID string) (*DeltaSummary, error) {
	return s.repo.GetDeltaSummary(ctx, portofolioID, year, month, tenantID)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// CalcRunError captures a per-instrument error in ComputeDeltaForCalcRun batch.
type CalcRunError struct {
	InstrumenID uuid.UUID
	ErrorCode   string
	ErrorDetail string
}

// writeAuditInTx writes an audit event in-transaction (DEC-018).
// Handles nil s.audit gracefully (noop in unit tests without a real DB).
func (s *Service) writeAuditInTx(ctx context.Context, tx *sql.Tx, evt audit.Event) {
	if s.audit == nil || tx == nil {
		// noop in unit tests — log at DEBUG level
		s.logger.DebugContext(ctx, "audit.writeAuditInTx: skipped (nil audit writer or tx)",
			"action", evt.Action)
		return
	}
	if err := s.audit.WithTx(tx).Write(ctx, evt); err != nil {
		// Audit failure should not halt business flow — log as error
		s.logger.ErrorContext(ctx, "audit.writeAuditInTx: failed",
			"action", evt.Action,
			"error", err.Error(),
		)
	}
}

