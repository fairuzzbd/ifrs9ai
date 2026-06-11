// Package lps — service layer for LPS Aggregator.
//
// AggregatorService: single-pair + bulk aggregation + preview DataTable.
// OverrideService: submit / approve / reject LPS exclusion overrides.
//
// Precision: all IDR arithmetic uses shopspring/decimal (DEC-016).
// Audit: SubmitOverride / ApproveOverride / RejectOverride write to aud.audit_log
//
//	in the same DB transaction as the override mutation (DEC-018).
//
// References:
//   - FSD-APP-C §3.3 (LPS Aggregator)
//   - SoW §4.3
//   - DEC-010, DEC-014, DEC-016, DEC-017, DEC-018, DEC-021, DEC-022
//   - docs/state-machines/p4-m3-lps.md §2 (state machine) + §5 (bulk SQL)
//   - docs/stories/phase-4/M3-lps-aggregator.md (27 AC)
package lps

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// rollbackTx attempts a transaction rollback and logs any error at Warn level.
// Safe to call even if tx is nil (post-commit).
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "lps: tx rollback failed", "error", err)
	}
}

// ─── Constants ────────────────────────────────────────────────────────────────

const (
	// maxBulkInstruments is the hard limit for bulk aggregate (HTTP 413 beyond).
	maxBulkInstruments = 50_000
	// bulkSemaphoreSize controls concurrency for pair fan-out.
	bulkSemaphoreSize = 16
	// overrideReasonMinLen enforces AC-LPS-019: exclusion reason must be ≥ 30 chars.
	overrideReasonMinLen = 30
)

// ─── AuditWriterIface ─────────────────────────────────────────────────────────

// AuditWriterIface is the minimal interface the service needs from the audit package.
// Full audit.Writer is injected at wire time via NewAuditWriterAdapter.
// Tests can inject mockAuditWriter which also satisfies this interface.
type AuditWriterIface interface {
	Write(ctx context.Context, tx *sql.Tx, event AuditEvent) error
}

// AuditEvent is a simplified audit event passed to the audit writer.
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

// AuditWriterAdapter bridges *audit.Writer (which uses TxWriter pattern) → AuditWriterIface.
// Wire in main.go: lps.NewAuditWriterAdapter(audit.NewWriter(db)).
type AuditWriterAdapter struct {
	w *audit.Writer
}

// NewAuditWriterAdapter creates an AuditWriterIface adapter from *audit.Writer.
func NewAuditWriterAdapter(w *audit.Writer) *AuditWriterAdapter {
	return &AuditWriterAdapter{w: w}
}

// Write implements AuditWriterIface. Converts lps.AuditEvent → audit.Event and
// calls writer.WithTx(tx).Write(ctx, evt) (DEC-018: same-tx audit write).
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

// KursRepoIface is the minimal interface to fetch a BI JISDOR kurs.
// Matches DBKursRepository.GetByDate (helpers/repo.go).
type KursRepoIface interface {
	GetByDate(ctx context.Context, currency string, date time.Time) (decimal.Decimal, error)
}

// PeriodeBukuRepoIface checks periode ordering for override validity.
type PeriodeBukuRepoIface interface {
	// GetStartEndDate returns (tanggal_mulai, tanggal_akhir) for a periode_buku by ID.
	// Returns (zero, zero, nil) if not found.
	GetStartEndDate(ctx context.Context, id uuid.UUID) (time.Time, time.Time, error)
}

// DBPeriodeBukuReader is a thin adapter from periodebuku.Repository → PeriodeBukuRepoIface.
// Avoids importing the periodebuku package into service.go (no circular imports).
// Wire in main.go: lps.NewDBPeriodeBukuReader(periodeBukuRepo).
type DBPeriodeBukuReader struct {
	// fetcher is the periodebuku.Repository.GetByID-compatible function.
	// Accepts interface{} to avoid importing the periodebuku package.
	fetcher func(ctx context.Context, id uuid.UUID) (tanggalMulaiStr, tanggalAkhirStr string, found bool, err error)
}

// NewDBPeriodeBukuReader creates a PeriodeBukuRepoIface adapter.
// fn must return (tanggalMulaiStr "YYYY-MM-DD", tanggalAkhirStr "YYYY-MM-DD", found bool, err).
func NewDBPeriodeBukuReader(fn func(ctx context.Context, id uuid.UUID) (string, string, bool, error)) *DBPeriodeBukuReader {
	return &DBPeriodeBukuReader{fetcher: fn}
}

// GetStartEndDate implements PeriodeBukuRepoIface.
func (r *DBPeriodeBukuReader) GetStartEndDate(ctx context.Context, id uuid.UUID) (time.Time, time.Time, error) {
	mulaiStr, akhirStr, found, err := r.fetcher(ctx, id)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if !found {
		return time.Time{}, time.Time{}, nil
	}
	mulai, e1 := time.Parse("2006-01-02", mulaiStr)
	akhir, e2 := time.Parse("2006-01-02", akhirStr)
	if e1 != nil || e2 != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("lps: parse periode dates: %v / %v", e1, e2)
	}
	return mulai, akhir, nil
}

// KursAdapter adapts the kurs.Repository.GetByKodeAndDate → KursRepoIface.
// Wire in main.go: lps.NewKursAdapter(kursRepo).
type KursAdapter struct {
	fetcher func(ctx context.Context, kode string, tanggal time.Time) (rateStr string, found bool, err error)
}

// NewKursAdapter creates a KursRepoIface adapter from kurs.Repository.GetByKodeAndDate-compatible fn.
// fn must return (nilai_kurs as StringFixed(8), found bool, err).
func NewKursAdapter(fn func(ctx context.Context, kode string, tanggal time.Time) (string, bool, error)) *KursAdapter {
	return &KursAdapter{fetcher: fn}
}

// GetByDate implements KursRepoIface.
func (a *KursAdapter) GetByDate(ctx context.Context, currency string, date time.Time) (decimal.Decimal, error) {
	rateStr, found, err := a.fetcher(ctx, currency, date)
	if err != nil {
		return decimal.Zero, err
	}
	if !found {
		return decimal.Zero, fmt.Errorf("kurs not found: %s %s", currency, date.Format("2006-01-02"))
	}
	return decimal.NewFromString(rateStr)
}

// ─── AggregatorService ────────────────────────────────────────────────────────

// AggregatorService computes LPS coverage for DEPOSITO instruments.
// Safe for concurrent use.
type AggregatorService struct {
	coverageRepo LPSCoverageRepoIface
	depositoRepo DepositoInstrumenRepoIface
	overrideRepo OverrideRepoIface
	kursRepo     KursRepoIface
}

// NewAggregatorService creates an AggregatorService.
func NewAggregatorService(
	coverageRepo LPSCoverageRepoIface,
	depositoRepo DepositoInstrumenRepoIface,
	overrideRepo OverrideRepoIface,
	kursRepo KursRepoIface,
) *AggregatorService {
	return &AggregatorService{
		coverageRepo: coverageRepo,
		depositoRepo: depositoRepo,
		overrideRepo: overrideRepo,
		kursRepo:     kursRepo,
	}
}

// Aggregate computes LPS coverage for a single (nasabah, bank) pair on evaluationDate.
//
// Algorithm (SoW §4.3, FSD-APP-C §3.3):
//  1. Fetch APPROVED lps_coverage cap for evalDate.
//  2. Fetch DEPOSITO instruments for pair, FIFO order (tanggal_penempatan ASC, id ASC).
//  3. For each instrument in FIFO order:
//     a. Convert nominal to IDR via BI JISDOR kurs (if FCY).
//     b. If instrument has APPROVED_ACTIVE exclusion override → lps_excluded=true;
//     full EAD goes to excess.
//     c. Walk cap: covered += min(EAD_IDR, remaining_cap); excess = EAD_IDR - covered.
//  4. INVARIANT: AllocatedToCovered + AllocatedToExcess == EAD_IDR per instrument.
func (s *AggregatorService) Aggregate(ctx context.Context, nasabahID, bankID uuid.UUID, evalDate time.Time) (*PairAggregation, error) {
	// Step 1: LPS coverage param.
	coverageRow, err := s.coverageRepo.GetActiveByEvaluationDate(ctx, evalDate)
	if err != nil {
		return nil, err
	}
	if coverageRow == nil {
		return nil, ErrLPSCoverageNoActiveParam(evalDate.Format("2006-01-02"))
	}

	// Step 2: instruments FIFO.
	instruments, err := s.depositoRepo.ListByNasabahBank(ctx, nasabahID, bankID, evalDate)
	if err != nil {
		return nil, err
	}

	// Step 3: for each instrument compute EAD_IDR.
	eadsIDR := make([]decimal.Decimal, len(instruments))
	for i := range instruments {
		inst := &instruments[i]
		ead, fxErr := s.toIDR(ctx, inst.Nominal, inst.MataUang, evalDate)
		if fxErr != nil {
			return nil, fxErr
		}
		eadsIDR[i] = ead
	}

	// Fetch overrides for all instruments in batch.
	ids := make([]uuid.UUID, len(instruments))
	for i := range instruments {
		ids[i] = instruments[i].ID
	}
	overrideSet, err := s.overrideRepo.GetActiveSetForInstrumens(ctx, ids, evalDate)
	if err != nil {
		return nil, err
	}

	return s.allocatePair(nasabahID, bankID, instruments, eadsIDR, overrideSet, coverageRow), nil
}

// AggregateBulk computes LPS coverage for all active DEPOSITO (nasabah, bank) pairs.
// Uses the batch JOIN query (state-machine doc §5) to avoid N+1. Concurrent fan-out,
// semaphore-limited to bulkSemaphoreSize goroutines.
// Returns map[instrumenID]LPSBreakdown for M7 ECL engine.
func (s *AggregatorService) AggregateBulk(ctx context.Context, evalDate time.Time) (map[uuid.UUID]LPSBreakdown, error) {
	// Step 1: LPS coverage param.
	coverageRow, err := s.coverageRepo.GetActiveByEvaluationDate(ctx, evalDate)
	if err != nil {
		return nil, err
	}
	if coverageRow == nil {
		return nil, ErrLPSCoverageNoActiveParam(evalDate.Format("2006-01-02"))
	}

	// Step 2: Single batch query — all DEPOSITO instruments with FX pre-joined.
	bulkRows, err := s.depositoRepo.BulkListDepositoForAggregate(ctx, evalDate)
	if err != nil {
		return nil, err
	}

	if len(bulkRows) > maxBulkInstruments {
		return nil, ErrLPSAggregateBulkTooLarge(len(bulkRows))
	}

	// Group by (nasabah, bank) — order preserved (bulkRows already sorted by nasabah, bank, FIFO).
	type groupKey struct {
		nasabah uuid.UUID
		bank    uuid.UUID
	}
	type pairGroup struct {
		rows []BulkDepositoRow
	}
	order := []groupKey{}
	groups := map[groupKey]*pairGroup{}
	for i := range bulkRows {
		row := &bulkRows[i]
		key := groupKey{row.NasabahID, row.BankID}
		if _, ok := groups[key]; !ok {
			groups[key] = &pairGroup{}
			order = append(order, key)
		}
		groups[key].rows = append(groups[key].rows, *row)
	}

	// Concurrent fan-out with semaphore.
	type pairResult struct {
		key groupKey
		agg *PairAggregation
		err error
	}
	sem := make(chan struct{}, bulkSemaphoreSize)
	resultCh := make(chan pairResult, len(order))
	var wg sync.WaitGroup

	for _, key := range order {
		key := key
		pg := groups[key]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			agg, err := s.aggregatePairFromBulkRows(pg.rows, coverageRow, evalDate)
			resultCh <- pairResult{key: key, agg: agg, err: err}
		}()
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results.
	breakdown := make(map[uuid.UUID]LPSBreakdown, len(bulkRows))
	for res := range resultCh {
		if res.err != nil {
			return nil, res.err
		}
		for i := range res.agg.Breakdown {
			b := &res.agg.Breakdown[i]
			breakdown[b.InstrumenID] = LPSBreakdown{
				EAD_IDR:        b.EAD_IDR,
				CoveredIDR:     b.AllocatedToCovered,
				ExcessIDR:      b.AllocatedToExcess,
				LPSExcluded:    b.LPSExcluded,
				LPSFullCovered: b.LPSFullCovered,
			}
		}
	}
	return breakdown, nil
}

// aggregatePairFromBulkRows processes one pair's pre-loaded BulkDepositoRows.
func (s *AggregatorService) aggregatePairFromBulkRows(rows []BulkDepositoRow, capRow *LPSCoverageRow, evalDate time.Time) (*PairAggregation, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	nasabahID := rows[0].NasabahID
	bankID := rows[0].BankID

	instruments := make([]InstrumenDepositoRow, len(rows))
	eadsIDR := make([]decimal.Decimal, len(rows))
	overrideSet := make(map[uuid.UUID]*LPSExclusionOverride)

	for i := range rows {
		row := &rows[i]
		instruments[i] = InstrumenDepositoRow{
			ID:                 row.InstrumenID,
			KodeInstrumen:      row.KodeInstrumen,
			CounterpartyID:     row.NasabahID,
			BankCounterpartyID: row.BankID,
			Nominal:            row.Nominal,
			MataUang:           row.MataUang,
			TanggalPenempatan:  row.TanggalPenempatan,
			KlasifikasiPsak71:  row.KlasifikasiPsak71,
			TenantID:           row.TenantID,
		}
		// FX: pre-joined in bulk query.
		if row.MataUang == "IDR" {
			eadsIDR[i] = row.Nominal
		} else {
			if row.FXRate == nil {
				return nil, ErrFXRateNotFound(row.MataUang, evalDate.Format("2006-01-02"))
			}
			// EAD_IDR = nominal × fx_rate (both NUMERIC(20,4); product truncated to 4dp)
			eadsIDR[i] = row.Nominal.Mul(*row.FXRate).Truncate(4)
		}
		// Build override set from pre-joined data.
		if row.OverrideID != nil {
			reason := ""
			if row.ExclusionReason != nil {
				reason = *row.ExclusionReason
			}
			overrideSet[row.InstrumenID] = &LPSExclusionOverride{
				ID:              *row.OverrideID,
				InstrumenID:     row.InstrumenID,
				ExclusionReason: reason,
				WorkflowStatus:  WorkflowStatusApprovedActive,
			}
		}
	}

	return s.allocatePair(nasabahID, bankID, instruments, eadsIDR, overrideSet, capRow), nil
}

// allocatePair performs the FIFO cap allocation for a pair.
// Accepts pre-computed EAD_IDR slice (same order as instruments) + override set.
// INVARIANT: AllocatedToCovered.Add(AllocatedToExcess).Equal(EAD_IDR) for each instrument.
func (s *AggregatorService) allocatePair(
	nasabahID, bankID uuid.UUID,
	instruments []InstrumenDepositoRow,
	eadsIDR []decimal.Decimal,
	overrideSet map[uuid.UUID]*LPSExclusionOverride,
	capRow *LPSCoverageRow,
) *PairAggregation {
	capIDR := capRow.CoverageAmountIDR
	remaining := capIDR // decremented as we allocate

	breakdown := make([]InstrumenBreakdown, 0, len(instruments))
	var totalExposure, totalCovered, totalExcess decimal.Decimal

	for rank := range instruments {
		inst := &instruments[rank]
		ead := eadsIDR[rank]
		totalExposure = totalExposure.Add(ead)

		var covered, excess decimal.Decimal
		var excluded bool
		var exclusionReason string
		var overrideID *uuid.UUID

		if ov, hasOv := overrideSet[inst.ID]; hasOv {
			// Excluded: full EAD goes to excess regardless of cap.
			excluded = true
			exclusionReason = ov.ExclusionReason
			overrideID = &ov.ID
			covered = decimal.Zero
			excess = ead
		} else {
			// FIFO allocation: covered = min(ead, remaining_cap).
			if remaining.GreaterThan(decimal.Zero) {
				if ead.LessThanOrEqual(remaining) {
					covered = ead
				} else {
					covered = remaining
				}
				remaining = remaining.Sub(covered)
			}
			excess = ead.Sub(covered)
		}

		totalCovered = totalCovered.Add(covered)
		totalExcess = totalExcess.Add(excess)

		breakdown = append(breakdown, InstrumenBreakdown{
			InstrumenID:         inst.ID,
			KodeInstrumen:       inst.KodeInstrumen,
			EAD_IDR:             ead,
			FIFORank:            rank + 1,
			TanggalPenempatan:   inst.TanggalPenempatan,
			AllocatedToCovered:  covered,
			AllocatedToExcess:   excess,
			LPSExcluded:         excluded,
			ExclusionReason:     exclusionReason,
			LPSFullCovered:      excess.IsZero() && !excluded,
			ExclusionOverrideID: overrideID,
		})
	}

	jumlahExcluded := 0
	for i := range breakdown {
		if breakdown[i].LPSExcluded {
			jumlahExcluded++
		}
	}

	return &PairAggregation{
		CounterpartyID:     nasabahID,
		BankID:             bankID,
		TotalExposureIDR:   totalExposure,
		CoveredIDR:         totalCovered,
		ExcessIDR:          totalExcess,
		LPSCapIDR:          capIDR,
		LPSCoverageParamID: capRow.ID,
		JumlahInstrumen:    len(instruments),
		JumlahExcluded:     jumlahExcluded,
		Breakdown:          breakdown,
		Warnings:           nil,
	}
}

// Preview returns DataTable rows for coverage utilization (APP-C-LPS-003).
// Runs AggregateBulk internally and aggregates to pair-level PreviewRow.
func (s *AggregatorService) Preview(ctx context.Context, evalDate time.Time, _, _ string, cursor string, limit int) ([]PreviewRow, string, bool, error) {
	breakdown, err := s.AggregateBulk(ctx, evalDate)
	if err != nil {
		return nil, "", false, err
	}

	// Re-aggregate per pair from LPSBreakdown.
	// We need names — re-run pair list (low cost).
	pairs, err := s.depositoRepo.ListAllActivePairs(ctx, evalDate)
	if err != nil {
		return nil, "", false, err
	}

	coverageRow, err := s.coverageRepo.GetActiveByEvaluationDate(ctx, evalDate)
	if err != nil {
		return nil, "", false, err
	}
	if coverageRow == nil {
		return nil, "", false, ErrLPSCoverageNoActiveParam(evalDate.Format("2006-01-02"))
	}

	// Build preview rows from bulk breakdown.
	// Group instruments by (nasabah, bank) from the bulk rows.
	bulkRows, err := s.depositoRepo.BulkListDepositoForAggregate(ctx, evalDate)
	if err != nil {
		return nil, "", false, err
	}

	type pairKey struct {
		n uuid.UUID
		b uuid.UUID
	}
	type pairAccum struct {
		nasabahNama     string
		bankNama        string
		totalExposure   decimal.Decimal
		covered         decimal.Decimal
		excess          decimal.Decimal
		jumlahInstrumen int
		jumlahExcluded  int
	}
	order := []pairKey{}
	accums := map[pairKey]*pairAccum{}
	for i := range bulkRows {
		row := &bulkRows[i]
		k := pairKey{row.NasabahID, row.BankID}
		if _, ok := accums[k]; !ok {
			accums[k] = &pairAccum{
				nasabahNama: row.NasabahNama,
				bankNama:    row.BankNama,
			}
			order = append(order, k)
		}
		b, ok := breakdown[row.InstrumenID]
		if !ok {
			continue
		}
		acc := accums[k]
		acc.totalExposure = acc.totalExposure.Add(b.EAD_IDR)
		acc.covered = acc.covered.Add(b.CoveredIDR)
		acc.excess = acc.excess.Add(b.ExcessIDR)
		acc.jumlahInstrumen++
		if b.LPSExcluded {
			acc.jumlahExcluded++
		}
	}
	_ = pairs // ListAllActivePairs used for FX validation earlier; accums built from bulkRows

	rows := make([]PreviewRow, 0, len(order))
	for _, k := range order {
		acc := accums[k]
		var coveredPct decimal.Decimal
		if acc.totalExposure.GreaterThan(decimal.Zero) {
			coveredPct = acc.covered.Div(acc.totalExposure).Mul(decimal.NewFromInt(100)).Truncate(2)
		}
		rows = append(rows, PreviewRow{
			NasabahID:        k.n,
			NasabahNama:      acc.nasabahNama,
			BankID:           k.b,
			BankNama:         acc.bankNama,
			LPSCapIDR:        coverageRow.CoverageAmountIDR,
			TotalExposureIDR: acc.totalExposure,
			CoveredIDR:       acc.covered,
			ExcessIDR:        acc.excess,
			CoveredPct:       coveredPct,
			JumlahInstrumen:  acc.jumlahInstrumen,
			JumlahExcluded:   acc.jumlahExcluded,
			EvaluationDate:   evalDate,
		})
	}

	// Apply cursor-based pagination.
	// Cursor is the index into rows (opaque; encoded as row index string).
	startIdx := 0
	if cursor != "" {
		for i := range rows {
			if rows[i].NasabahID.String()+":"+rows[i].BankID.String() == cursor {
				startIdx = i + 1
				break
			}
		}
	}
	end := startIdx + limit
	hasMore := false
	if end < len(rows) {
		hasMore = true
	} else {
		end = len(rows)
	}
	page := rows[startIdx:end]
	nextCursor := ""
	if hasMore && len(page) > 0 {
		last := page[len(page)-1]
		nextCursor = last.NasabahID.String() + ":" + last.BankID.String()
	}
	return page, nextCursor, hasMore, nil
}

// toIDR converts an amount from the given currency to IDR using BI JISDOR kurs.
// If currency is IDR, returns amount unchanged.
// Precision: multiply Nominal × kurs, truncate to 4 decimal places.
func (s *AggregatorService) toIDR(ctx context.Context, amount decimal.Decimal, currency string, evalDate time.Time) (decimal.Decimal, error) {
	if currency == "IDR" {
		return amount, nil
	}
	rate, err := s.kursRepo.GetByDate(ctx, currency, evalDate)
	if err != nil {
		return decimal.Zero, ErrFXRateNotFound(currency, evalDate.Format("2006-01-02"))
	}
	if rate.IsZero() {
		return decimal.Zero, ErrFXRateNotFound(currency, evalDate.Format("2006-01-02"))
	}
	// EAD_IDR = nominal × fx_rate, truncated to NUMERIC(20,4).
	return amount.Mul(rate).Truncate(4), nil
}

// ─── OverrideService ──────────────────────────────────────────────────────────

// OverrideService manages LPS exclusion override proposals (4-eyes workflow).
// Maker: ROLE-RISK (perm: lps_aggregator.override)
// Approver: ROLE-ALCO (perm: lps_aggregator.override.approve)
// SoD: maker_id ≠ approver_id (DEC-017).
type OverrideService struct {
	db           *sql.DB
	overrideRepo OverrideRepoIface
	periodeRepo  PeriodeBukuRepoIface
	auditWriter  AuditWriterIface
	logger       *slog.Logger
}

// NewOverrideService creates an OverrideService.
// Panics if auditWriter is nil — a nil audit writer in production silently skips
// the legally-required audit trail (DEC-018), which is a compliance violation.
// Logger falls back to slog.Default() if nil (safe; audit is not).
func NewOverrideService(
	db *sql.DB,
	overrideRepo OverrideRepoIface,
	periodeRepo PeriodeBukuRepoIface,
	auditWriter AuditWriterIface,
) *OverrideService {
	if auditWriter == nil {
		panic("lps: auditWriter must not be nil — audit trail is mandatory (DEC-018)")
	}
	logger := slog.Default()
	return &OverrideService{
		db:           db,
		overrideRepo: overrideRepo,
		periodeRepo:  periodeRepo,
		auditWriter:  auditWriter,
		logger:       logger,
	}
}

// Submit proposes a new LPS exclusion override.
// Enforces: instrumen existence + DEPOSITO type, reason ≥ 30 chars, periode ordering,
// no duplicate active/pending override.
// Inserts in PENDING_APPROVAL state. Audits in same transaction (DEC-018).
func (s *OverrideService) Submit(ctx context.Context, req SubmitOverrideRequest, makerID uuid.UUID, actorRole, tenantID string) (*LPSExclusionOverride, error) {
	// F1+F3: Verify instrumen exists and is DEPOSITO BEFORE opening a transaction.
	// This prevents opaque FK-violation 500s (F3) and silently orphaned non-DEPOSITO
	// overrides (F1). Per FSD-APP-C §3.3 and AC-LPS-019.
	tipe, err := s.overrideRepo.GetInstrumenTipe(ctx, req.InstrumenID)
	if err != nil {
		if de, ok := err.(*domainerrors.DomainError); ok && de.Code() == domainerrors.CodeNotFound {
			return nil, ErrLPSOverrideInstrumenNotFound(req.InstrumenID.String())
		}
		return nil, fmt.Errorf("lps: submit lookup instrumen: %w", err)
	}
	if tipe != "DEPOSITO" {
		return nil, ErrLPSAggregateInstrumenNotDeposito(tipe)
	}

	// Validate reason length (AC-LPS-019).
	if len(req.ExclusionReason) < overrideReasonMinLen {
		return nil, ErrLPSOverrideReasonTooShort(len(req.ExclusionReason))
	}

	// Validate periode ordering: validFrom.tanggal_mulai <= validTo.tanggal_akhir.
	fromStart, _, err := s.periodeRepo.GetStartEndDate(ctx, req.ValidFromPeriodeID)
	if err != nil {
		return nil, fmt.Errorf("lps: get valid_from_periode: %w", err)
	}
	_, toEnd, err := s.periodeRepo.GetStartEndDate(ctx, req.ValidToPeriodeID)
	if err != nil {
		return nil, fmt.Errorf("lps: get valid_to_periode: %w", err)
	}
	if fromStart.IsZero() || toEnd.IsZero() {
		return nil, domainerrors.ErrNotFound("periode_buku")
	}
	if fromStart.After(toEnd) {
		return nil, ErrLPSOverridePeriodeInvalid()
	}

	// Prevent duplicate active/pending override for same instrument.
	exists, conflictID, err := s.overrideRepo.HasActiveOrPendingForInstrumen(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("lps: check existing override: %w", err)
	}
	if exists {
		return nil, domainerrors.New(
			domainerrors.Code(CodeLPSOverrideInvalidTransition),
			"Instrumen "+req.InstrumenID.String()+" sudah punya override aktif/pending (id="+conflictID+"). Approve/reject dulu sebelum submit baru.",
		)
	}

	override := &LPSExclusionOverride{
		InstrumenID:        req.InstrumenID,
		ExclusionReason:    req.ExclusionReason,
		ValidFromPeriodeID: req.ValidFromPeriodeID,
		ValidToPeriodeID:   req.ValidToPeriodeID,
		WorkflowStatus:     WorkflowStatusPendingApproval,
		MakerID:            makerID,
		CreatedBy:          makerID,
		UpdatedBy:          makerID,
		TenantID:           tenantID,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lps: begin tx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err = s.overrideRepo.Create(ctx, tx, override); err != nil {
		return nil, fmt.Errorf("lps: create override: %w", err)
	}

	if auditErr := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: makerID,
		ActorRole:   actorRole,
		Action:      "LPS_EXCLUSION_OVERRIDE.SUBMIT",
		EntityType:  "ecl.lps_exclusion_override",
		EntityID:    override.ID,
		BeforeJSON:  nil,
		AfterJSON:   override,
		TenantID:    tenantID,
	}); auditErr != nil {
		return nil, fmt.Errorf("lps: audit submit: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("lps: commit override submit: %w", err)
	}
	return override, nil
}

// ApproveOverride transitions an override from PENDING_APPROVAL → APPROVED_ACTIVE.
// Enforces SoD: approverID ≠ makerID (DEC-017).
// No step-up MFA at service layer — OQ-M3-5 resolved: MFA wajib ALCO (DEC-026) sufficient.
func (s *OverrideService) ApproveOverride(ctx context.Context, overrideID uuid.UUID, approverID uuid.UUID, actorRole, comment, tenantID string) (*LPSExclusionOverride, error) {
	existing, err := s.overrideRepo.GetByID(ctx, overrideID)
	if err != nil {
		return nil, fmt.Errorf("lps: get override: %w", err)
	}
	if existing == nil {
		return nil, ErrLPSOverrideInstrumenNotFound(overrideID.String())
	}
	if existing.WorkflowStatus != WorkflowStatusPendingApproval {
		return nil, ErrLPSOverrideInvalidTransition(existing.WorkflowStatus.String(), "APPROVE")
	}
	// SoD check.
	if approverID == existing.MakerID {
		return nil, ErrLPSOverrideSoDViolation()
	}

	signedAt := time.Now().UTC()
	sigHash := ComputeApproveSignatureHash(approverID, overrideID, signedAt, comment)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lps: begin tx approve: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err = s.overrideRepo.Approve(ctx, tx, overrideID, approverID, signedAt, sigHash, comment, approverID); err != nil {
		return nil, fmt.Errorf("lps: approve override: %w", err)
	}

	if auditErr := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: approverID,
		ActorRole:   actorRole,
		Action:      "LPS_EXCLUSION_OVERRIDE.APPROVE",
		EntityType:  "ecl.lps_exclusion_override",
		EntityID:    overrideID,
		BeforeJSON:  map[string]string{"workflowStatus": string(WorkflowStatusPendingApproval)},
		AfterJSON:   map[string]string{"workflowStatus": string(WorkflowStatusApprovedActive), "comment": comment},
		TenantID:    tenantID,
	}); auditErr != nil {
		return nil, fmt.Errorf("lps: audit approve: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("lps: commit approve: %w", err)
	}

	return s.overrideRepo.GetByID(ctx, overrideID)
}

// RejectOverride transitions an override from PENDING_APPROVAL → REJECTED.
// Can be called by both ROLE-ALCO (approver) and ROLE-RISK (maker recall).
func (s *OverrideService) RejectOverride(ctx context.Context, overrideID uuid.UUID, actorID uuid.UUID, actorRole, rejectReason, tenantID string) (*LPSExclusionOverride, error) {
	existing, err := s.overrideRepo.GetByID(ctx, overrideID)
	if err != nil {
		return nil, fmt.Errorf("lps: get override for reject: %w", err)
	}
	if existing == nil {
		return nil, ErrLPSOverrideInstrumenNotFound(overrideID.String())
	}
	if existing.WorkflowStatus != WorkflowStatusPendingApproval {
		return nil, ErrLPSOverrideInvalidTransition(existing.WorkflowStatus.String(), "REJECT")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("lps: begin tx reject: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	if err = s.overrideRepo.Reject(ctx, tx, overrideID, actorID, rejectReason, actorID); err != nil {
		return nil, fmt.Errorf("lps: reject override: %w", err)
	}

	if auditErr := s.auditWriter.Write(ctx, tx, AuditEvent{
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Action:      "LPS_EXCLUSION_OVERRIDE.REJECT",
		EntityType:  "ecl.lps_exclusion_override",
		EntityID:    overrideID,
		BeforeJSON:  map[string]string{"workflowStatus": string(WorkflowStatusPendingApproval)},
		AfterJSON:   map[string]string{"workflowStatus": string(WorkflowStatusRejected), "rejectReason": rejectReason},
		TenantID:    tenantID,
	}); auditErr != nil {
		return nil, fmt.Errorf("lps: audit reject: %w", auditErr)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("lps: commit reject: %w", err)
	}

	return s.overrideRepo.GetByID(ctx, overrideID)
}

// ListOverrides returns paginated override list for DataTable.
func (s *OverrideService) ListOverrides(ctx context.Context,
	filterWorkflowStatus, filterInstrumenID, filterMakerID, search,
	sortCol, sortDir, cursor string, limit int,
) ([]LPSExclusionOverride, string, bool, error) {
	return s.overrideRepo.List(ctx,
		filterWorkflowStatus, filterInstrumenID, filterMakerID,
		search, sortCol, sortDir, cursor, limit)
}
