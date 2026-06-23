package penjualan

// jurnal_poster.go — JurnalPoster interface + stubs (mirror M7 pattern).
//
// Wiring decision (same as M6/M7):
//   - Interface defined here; real P5-M2 implementation injected via main.go.
//   - JurnalPosterStub records calls for service_test.go.
//   - NoopJurnalPoster for dev mode (logs WARN).
//   - Production guard via IsNoopProduction().

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PenjualanPostRequest carries data needed to post penjualan jurnal entries.
// EventCodes may contain multiple codes (e.g. [PENJUALAN_FVOCI_DEBT, REKLAS_OCI_PL]).
type PenjualanPostRequest struct {
	EventCodes          []string         // primary + secondary event codes from routing
	InstrumenID         uuid.UUID
	PenjualanID         uuid.UUID
	PeriodeID           uuid.UUID
	TanggalEksekusi     time.Time
	KlasifikasiSnapshot KlasifikasiPSAK71
	JenisDisposal       DisposalType
	ProceedIDR          decimal.Decimal
	CostBasis           decimal.Decimal
	RealizedGL          decimal.Decimal
	OCIRecycled         *decimal.Decimal // nil if not FVOCI debt
	QtyTerjual          decimal.Decimal
	ActorID             uuid.UUID
	TenantID            string
	TraceID             string
}

// PenjualanPostResult holds the result of a successful jurnal post.
type PenjualanPostResult struct {
	JurnalEntryID uuid.UUID
	EventCodes    []string
}

// JurnalPoster abstracts the P5-M2 jurnal engine for penjualan posting.
type JurnalPoster interface {
	// Post posts penjualan jurnal entries (multi-leg per klasifikasi) in the given transaction.
	// Returns (PenjualanPostResult, nil) on success.
	Post(ctx context.Context, tx *sql.Tx, req PenjualanPostRequest) (PenjualanPostResult, error)
}

// ─── Stub implementation (for tests) ─────────────────────────────────────────

// JurnalPosterStub records Post calls and returns configurable results.
type JurnalPosterStub struct {
	mu     sync.Mutex
	calls  []PenjualanPostRequest
	result PenjualanPostResult
	err    error
	logger *slog.Logger
}

// NewJurnalPosterStub creates a stub that returns a deterministic entry ID.
func NewJurnalPosterStub(logger *slog.Logger) *JurnalPosterStub {
	if logger == nil {
		logger = slog.Default()
	}
	return &JurnalPosterStub{
		result: PenjualanPostResult{
			JurnalEntryID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			EventCodes:    []string{"PENJUALAN_AC"},
		},
		logger: logger,
	}
}

// SetResult configures Post return value.
func (s *JurnalPosterStub) SetResult(r PenjualanPostResult) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.result = r
}

// SetError configures Post to return err.
func (s *JurnalPosterStub) SetError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.err = err
}

// Calls returns a thread-safe copy of recorded Post requests.
func (s *JurnalPosterStub) Calls() []PenjualanPostRequest {
	s.mu.Lock(); defer s.mu.Unlock()
	cp := make([]PenjualanPostRequest, len(s.calls))
	copy(cp, s.calls)
	return cp
}

// Reset clears calls and error.
func (s *JurnalPosterStub) Reset() {
	s.mu.Lock(); defer s.mu.Unlock()
	s.calls = nil
	s.err = nil
}

// Post implements JurnalPoster.
func (s *JurnalPosterStub) Post(ctx context.Context, tx *sql.Tx, req PenjualanPostRequest) (PenjualanPostResult, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	s.logger.DebugContext(ctx, "JurnalPosterStub.Post",
		"event_codes", req.EventCodes,
		"penjualan_id", req.PenjualanID,
		"proceed_idr", req.ProceedIDR.String(),
	)
	if s.err != nil {
		return PenjualanPostResult{}, s.err
	}
	return PenjualanPostResult{JurnalEntryID: uuid.New(), EventCodes: req.EventCodes}, nil
}

// ─── NoopJurnalPoster (dev-only) ─────────────────────────────────────────────

// NoopJurnalPoster is a no-op JurnalPoster for dev environments without P5-M2.
type NoopJurnalPoster struct {
	logger *slog.Logger
}

// NewNoopJurnalPoster creates a no-op poster with a WARN log at construction time.
func NewNoopJurnalPoster(logger *slog.Logger) *NoopJurnalPoster {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("penjualan.NoopJurnalPoster created — jurnal posting disabled. Wire P5-M2 adapter in main.go.")
	return &NoopJurnalPoster{logger: logger}
}

// IsNoopProduction returns true if APP_ENV=production and this is a NoopJurnalPoster.
func IsNoopProduction(p JurnalPoster) bool {
	if os.Getenv("APP_ENV") != "production" {
		return false
	}
	_, isNoop := p.(*NoopJurnalPoster)
	return isNoop
}

// Post implements JurnalPoster with a no-op.
func (n *NoopJurnalPoster) Post(ctx context.Context, _ *sql.Tx, req PenjualanPostRequest) (PenjualanPostResult, error) {
	n.logger.WarnContext(ctx, "penjualan.NoopJurnalPoster: jurnal posting skipped — P5-M2 not wired",
		"event_codes", req.EventCodes,
		"penjualan_id", req.PenjualanID.String(),
	)
	return PenjualanPostResult{JurnalEntryID: uuid.New(), EventCodes: req.EventCodes}, nil
}

// ─── InstrumenUpdater interface ───────────────────────────────────────────────

// InstrumenUpdater abstracts updating mst.instrumen on disposal.
// Both methods must be called in the same *sql.Tx.
type InstrumenUpdater interface {
	// UpdateQty reduces qty_holding by qtyTerjual for PARTIAL disposal.
	// mst.instrumen.status remains ACTIVE.
	UpdateQty(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, qtyTerjual decimal.Decimal, actorID uuid.UUID) (decimal.Decimal, error)

	// SetDisposed sets mst.instrumen.status = 'DISPOSED' for FULL disposal.
	SetDisposed(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, actorID uuid.UUID) error
}

// InstrumenUpdaterStub is a test stub for InstrumenUpdater.
type InstrumenUpdaterStub struct {
	mu           sync.Mutex
	qtyCalls     int
	disposeCalls int
	qtyResult    decimal.Decimal
	qtyErr       error
	disposeErr   error
}

// NewInstrumenUpdaterStub creates a stub.
func NewInstrumenUpdaterStub() *InstrumenUpdaterStub {
	return &InstrumenUpdaterStub{
		qtyResult: decimal.NewFromInt(500), // default leftover qty
	}
}

// SetQtyError configures UpdateQty to return err.
func (s *InstrumenUpdaterStub) SetQtyError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.qtyErr = err
}

// SetDisposeError configures SetDisposed to return err.
func (s *InstrumenUpdaterStub) SetDisposeError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.disposeErr = err
}

// QtyCalls returns the number of UpdateQty calls.
func (s *InstrumenUpdaterStub) QtyCalls() int {
	s.mu.Lock(); defer s.mu.Unlock()
	return s.qtyCalls
}

// DisposeCalls returns the number of SetDisposed calls.
func (s *InstrumenUpdaterStub) DisposeCalls() int {
	s.mu.Lock(); defer s.mu.Unlock()
	return s.disposeCalls
}

// UpdateQty implements InstrumenUpdater.
func (s *InstrumenUpdaterStub) UpdateQty(_ context.Context, _ *sql.Tx, _ uuid.UUID, qtyTerjual decimal.Decimal, _ uuid.UUID) (decimal.Decimal, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.qtyCalls++
	if s.qtyErr != nil {
		return decimal.Zero, s.qtyErr
	}
	return s.qtyResult, nil
}

// SetDisposed implements InstrumenUpdater.
func (s *InstrumenUpdaterStub) SetDisposed(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.disposeCalls++
	return s.disposeErr
}

// ─── RiskNotifier interface ───────────────────────────────────────────────────

// RiskNotifier abstracts notification to ROLE-RISK on BM violation.
type RiskNotifier interface {
	// NotifyBMViolation sends a BM frequency violation notification to all ROLE-RISK users.
	NotifyBMViolation(ctx context.Context, portofolioID uuid.UUID, penjualanID uuid.UUID, pct decimal.Decimal, isBlock bool) error
}

// RiskNotifierStub is a test stub for RiskNotifier.
type RiskNotifierStub struct {
	mu    sync.Mutex
	calls int
	err   error
}

// NewRiskNotifierStub creates a stub.
func NewRiskNotifierStub() *RiskNotifierStub { return &RiskNotifierStub{} }

// SetError configures NotifyBMViolation to return err.
func (s *RiskNotifierStub) SetError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.err = err
}

// Calls returns the number of NotifyBMViolation calls.
func (s *RiskNotifierStub) Calls() int {
	s.mu.Lock(); defer s.mu.Unlock()
	return s.calls
}

// NotifyBMViolation implements RiskNotifier.
func (s *RiskNotifierStub) NotifyBMViolation(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ decimal.Decimal, _ bool) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.calls++
	return s.err
}
