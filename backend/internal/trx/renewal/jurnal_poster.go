package renewal

// jurnal_poster.go — JurnalPoster interface + stub (mirror MTM pattern).
//
// Wiring decision (same as M6):
//   - Interface defined here; real P5-M2 implementation injected via main.go.
//   - JurnalPosterStub records all Post calls — used in service_test.go.
//   - NoopJurnalPoster for dev mode (logs WARN on each call).
//   - Production guard via IsNoopProduction().

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RenewalPostRequest carries data needed to post one RENEWAL_DEPOSITO jurnal entry.
//
// Leg 4 (Beban Bunga Deposito gross) requires BungaKotor separately from BungaBersih so the
// jurnal engine can book:
//
//	Leg 3: D Beban Bunga  / K Hutang PPh = PphAmount
//	Leg 4: D Beban Bunga  / K Kas        = BungaBersih  (net cash paid)
//
// BungaKotor == BungaBersih + PphAmount; carry it explicitly to avoid re-derivation at posting.
type RenewalPostRequest struct {
	EventCode       string          // "RENEWAL_DEPOSITO"
	InstrumenLamaID uuid.UUID
	InstrumenBaruID uuid.UUID
	RenewalID       uuid.UUID
	PeriodeID       uuid.UUID
	TanggalEfektif  time.Time
	PokokLama       decimal.Decimal
	PokokBaru       decimal.Decimal
	BungaKotor      decimal.Decimal // gross interest before PPh (Leg 4 reference)
	BungaBersih     decimal.Decimal // net cash paid to counterparty
	PphAmount       decimal.Decimal // withholding tax 20%
	ActorID         uuid.UUID
	TenantID        string
	TraceID         string
}

// RenewalPostResult holds the result of a successful jurnal post.
type RenewalPostResult struct {
	JurnalEntryID uuid.UUID
	EventCode     string
}

// JurnalPoster abstracts the P5-M2 jurnal engine for renewal posting.
// Implemented by: JurnalPosterStub (test), NoopJurnalPoster (dev), real M2 adapter (prod).
type JurnalPoster interface {
	// Post posts RENEWAL_DEPOSITO jurnal entry (4 legs) in the given transaction.
	// Returns (RenewalPostResult, nil) on success.
	// Returns error (wrapping cause) on failure → caller must rollback tx.
	Post(ctx context.Context, tx *sql.Tx, req RenewalPostRequest) (RenewalPostResult, error)
}

// ─── Stub implementation (for tests) ─────────────────────────────────────────

// JurnalPosterStub records Post calls and returns configurable results.
type JurnalPosterStub struct {
	mu     sync.Mutex
	calls  []RenewalPostRequest
	result RenewalPostResult
	err    error
	logger *slog.Logger
}

// NewJurnalPosterStub creates a stub that returns a deterministic entry ID.
func NewJurnalPosterStub(logger *slog.Logger) *JurnalPosterStub {
	if logger == nil {
		logger = slog.Default()
	}
	return &JurnalPosterStub{
		result: RenewalPostResult{
			JurnalEntryID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			EventCode:     "RENEWAL_DEPOSITO",
		},
		logger: logger,
	}
}

// SetResult configures the return value for Post.
func (s *JurnalPosterStub) SetResult(r RenewalPostResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = r
}

// SetError configures Post to return err.
func (s *JurnalPosterStub) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// Calls returns all recorded Post requests (thread-safe copy).
func (s *JurnalPosterStub) Calls() []RenewalPostRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]RenewalPostRequest, len(s.calls))
	copy(cp, s.calls)
	return cp
}

// Reset clears calls and error.
func (s *JurnalPosterStub) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
	s.err = nil
}

// Post implements JurnalPoster.
func (s *JurnalPosterStub) Post(ctx context.Context, tx *sql.Tx, req RenewalPostRequest) (RenewalPostResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)

	s.logger.DebugContext(ctx, "JurnalPosterStub.Post",
		"event_code", req.EventCode,
		"renewal_id", req.RenewalID,
		"pokok_baru", req.PokokBaru.String(),
	)

	if s.err != nil {
		return RenewalPostResult{}, s.err
	}
	return RenewalPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
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
	logger.Warn("renewal.NoopJurnalPoster created — jurnal posting disabled. Wire P5-M2 adapter in main.go.")
	return &NoopJurnalPoster{logger: logger}
}

// IsNoopProduction returns true if APP_ENV=production and this is a NoopJurnalPoster.
// main.go should log.Fatal if this returns true at startup.
func IsNoopProduction(p JurnalPoster) bool {
	if os.Getenv("APP_ENV") != "production" {
		return false
	}
	_, isNoop := p.(*NoopJurnalPoster)
	return isNoop
}

// Post implements JurnalPoster with a no-op.
func (n *NoopJurnalPoster) Post(ctx context.Context, _ *sql.Tx, req RenewalPostRequest) (RenewalPostResult, error) {
	n.logger.WarnContext(ctx, "renewal.NoopJurnalPoster: jurnal posting skipped — P5-M2 not wired",
		"event_code", req.EventCode,
		"renewal_id", req.RenewalID.String(),
	)
	return RenewalPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// ─── InstrumenCreator interface ──────────────────────────────────────────────

// InstrumenCreator abstracts the creation of a new mst.instrumen row and marking
// the old one as MATURED. Both operations must be called in the same *sql.Tx.
// Implemented by a thin adapter over internal/master/instrumen repository.
type InstrumenCreator interface {
	// CreateInstrumenBaru inserts a new instrumen inherited from the old one.
	// Returns the new instrumen's UUID.
	CreateInstrumenBaru(ctx context.Context, tx *sql.Tx, old InstrumenInfo, renewal *Renewal) (uuid.UUID, error)

	// MaturedInstrumenLama sets instrumen_lama.status = 'MATURED'.
	MaturedInstrumenLama(ctx context.Context, tx *sql.Tx, instrumenLamaID uuid.UUID, actorID uuid.UUID) error
}

// InstrumenCreatorStub is a test stub for InstrumenCreator.
type InstrumenCreatorStub struct {
	mu            sync.Mutex
	createCalls   int
	maturedCalls  int
	createResult  uuid.UUID
	createErr     error
	maturedErr    error
}

// NewInstrumenCreatorStub creates a stub with deterministic UUID response.
func NewInstrumenCreatorStub() *InstrumenCreatorStub {
	return &InstrumenCreatorStub{
		createResult: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	}
}

// SetCreateError configures CreateInstrumenBaru to return err.
func (s *InstrumenCreatorStub) SetCreateError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createErr = err
}

// SetMaturedError configures MaturedInstrumenLama to return err.
func (s *InstrumenCreatorStub) SetMaturedError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maturedErr = err
}

// CreateCalls returns the number of CreateInstrumenBaru calls.
func (s *InstrumenCreatorStub) CreateCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCalls
}

// MaturedCalls returns the number of MaturedInstrumenLama calls.
func (s *InstrumenCreatorStub) MaturedCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maturedCalls
}

// CreateInstrumenBaru implements InstrumenCreator.
func (s *InstrumenCreatorStub) CreateInstrumenBaru(ctx context.Context, tx *sql.Tx, old InstrumenInfo, r *Renewal) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	if s.createErr != nil {
		return uuid.Nil, s.createErr
	}
	return uuid.New(), nil
}

// MaturedInstrumenLama implements InstrumenCreator.
func (s *InstrumenCreatorStub) MaturedInstrumenLama(ctx context.Context, tx *sql.Tx, instrumenLamaID uuid.UUID, actorID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maturedCalls++
	if s.maturedErr != nil {
		return fmt.Errorf("MaturedInstrumenLama: %w", s.maturedErr)
	}
	return nil
}

// EIRScheduleWriter abstracts writing ecl.amortisasi_schedule rows.
// Both INSERT new + UPDATE effective_to of old must happen in same tx.
type EIRScheduleWriter interface {
	// InsertScheduleBaru inserts a new amortisasi_schedule row for the new instrumen.
	InsertScheduleBaru(ctx context.Context, tx *sql.Tx, instrumenBaruID uuid.UUID,
		eirBaru decimal.Decimal, effectiveFrom time.Time, actorID uuid.UUID) error

	// CloseScheduleLama updates effective_to = effectiveFrom on existing schedule for old instrumen.
	// NEVER updates eir_persen value — immutability rule (PSAK 71 §B5.4.6).
	CloseScheduleLama(ctx context.Context, tx *sql.Tx, instrumenLamaID uuid.UUID,
		effectiveTo time.Time, actorID uuid.UUID) error
}

// EIRScheduleWriterStub is a test stub for EIRScheduleWriter.
type EIRScheduleWriterStub struct {
	mu          sync.Mutex
	insertCalls int
	closeCalls  int
	insertErr   error
	closeErr    error
}

// NewEIRScheduleWriterStub creates a nop stub.
func NewEIRScheduleWriterStub() *EIRScheduleWriterStub {
	return &EIRScheduleWriterStub{}
}

// SetInsertError configures InsertScheduleBaru to return err.
func (s *EIRScheduleWriterStub) SetInsertError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertErr = err
}

// SetCloseError configures CloseScheduleLama to return err.
func (s *EIRScheduleWriterStub) SetCloseError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeErr = err
}

// InsertCalls returns the number of InsertScheduleBaru calls.
func (s *EIRScheduleWriterStub) InsertCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.insertCalls
}

// CloseCalls returns the number of CloseScheduleLama calls.
func (s *EIRScheduleWriterStub) CloseCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeCalls
}

// InsertScheduleBaru implements EIRScheduleWriter.
func (s *EIRScheduleWriterStub) InsertScheduleBaru(_ context.Context, _ *sql.Tx,
	instrumenBaruID uuid.UUID, eirBaru decimal.Decimal, effectiveFrom time.Time, actorID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.insertCalls++
	return s.insertErr
}

// CloseScheduleLama implements EIRScheduleWriter.
func (s *EIRScheduleWriterStub) CloseScheduleLama(_ context.Context, _ *sql.Tx,
	instrumenLamaID uuid.UUID, effectiveTo time.Time, actorID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCalls++
	return s.closeErr
}
