package akrualmaturity

// jurnal_poster.go — JurnalPoster interface for akrualmaturity package.
// Mirrors M6/M7/M8 pattern. Real P5-M2 implementation injected via main.go.

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AkrualPostRequest carries data for posting an akrual jurnal entry.
type AkrualPostRequest struct {
	EventCode       string
	InstrumenID     uuid.UUID
	AkrualID        uuid.UUID
	PeriodeID       uuid.UUID
	TanggalAkrual   time.Time
	BungaKotor      decimal.Decimal
	PPh             decimal.Decimal
	BungaBersih     decimal.Decimal
	Stage           int
	Jenis           AkrualJenis
	KlasifikasiSnapshot string
	ActorID         uuid.UUID
	TenantID        string
}

// AkrualPostResult holds the result of an akrual jurnal post.
type AkrualPostResult struct {
	JurnalEntryID uuid.UUID
	EventCode     string
}

// MaturityPostRequest carries data for posting a maturity settlement jurnal entry.
type MaturityPostRequest struct {
	EventCode           string
	InstrumenID         uuid.UUID
	JatuhTempoID        uuid.UUID
	PeriodeID           uuid.UUID
	TanggalJatuhTempo   time.Time
	PokokIDR            decimal.Decimal
	BungaLastIDR        decimal.Decimal
	PPhIDR              decimal.Decimal
	NetKasIDR           decimal.Decimal
	KlasifikasiSnapshot string
	Jenis               string
	ActorID             uuid.UUID
	TenantID            string
}

// MaturityPostResult holds the result of maturity jurnal post.
type MaturityPostResult struct {
	JurnalEntryID uuid.UUID
	EventCode     string
}

// DividenPostRequest carries data for posting dividen jurnal entry.
type DividenPostRequest struct {
	EventCode           string
	InstrumenID         uuid.UUID
	DividenID           uuid.UUID
	PeriodeID           uuid.UUID
	TanggalTerima       time.Time
	JumlahKotor         decimal.Decimal
	PPh                 decimal.Decimal
	JumlahBersih        decimal.Decimal
	KlasifikasiSnapshot string
	Treatment           string // "P&L" or "OCI"
	ActorID             uuid.UUID
	TenantID            string
}

// DividenPostResult holds the result of dividen jurnal post.
type DividenPostResult struct {
	JurnalEntryID uuid.UUID
	EventCode     string
}

// JurnalPoster abstracts P5-M2 jurnal engine for akrualmaturity posting.
type JurnalPoster interface {
	// PostAkrual posts daily accrual interest jurnal in given tx.
	PostAkrual(ctx context.Context, tx *sql.Tx, req AkrualPostRequest) (AkrualPostResult, error)

	// PostMaturity posts maturity settlement jurnal in given tx.
	PostMaturity(ctx context.Context, tx *sql.Tx, req MaturityPostRequest) (MaturityPostResult, error)

	// PostDividen posts dividend jurnal in given tx.
	PostDividen(ctx context.Context, tx *sql.Tx, req DividenPostRequest) (DividenPostResult, error)
}

// ─── Stub implementation ─────────────────────────────────────────────────────

// JurnalPosterStub records calls and returns configurable results.
type JurnalPosterStub struct {
	mu            sync.Mutex
	akrualCalls   []AkrualPostRequest
	maturityCalls []MaturityPostRequest
	dividenCalls  []DividenPostRequest
	akrualErr     error
	maturityErr   error
	dividenErr    error
	logger        *slog.Logger
}

// NewJurnalPosterStub creates a stub.
func NewJurnalPosterStub(logger *slog.Logger) *JurnalPosterStub {
	if logger == nil {
		logger = slog.Default()
	}
	return &JurnalPosterStub{logger: logger}
}

// SetAkrualError configures PostAkrual to return err.
func (s *JurnalPosterStub) SetAkrualError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.akrualErr = err
}

// SetMaturityError configures PostMaturity to return err.
func (s *JurnalPosterStub) SetMaturityError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.maturityErr = err
}

// SetDividenError configures PostDividen to return err.
func (s *JurnalPosterStub) SetDividenError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.dividenErr = err
}

// AkrualCalls returns a copy of recorded PostAkrual requests.
func (s *JurnalPosterStub) AkrualCalls() []AkrualPostRequest {
	s.mu.Lock(); defer s.mu.Unlock()
	cp := make([]AkrualPostRequest, len(s.akrualCalls))
	copy(cp, s.akrualCalls)
	return cp
}

// MaturityCalls returns a copy of recorded PostMaturity requests.
func (s *JurnalPosterStub) MaturityCalls() []MaturityPostRequest {
	s.mu.Lock(); defer s.mu.Unlock()
	cp := make([]MaturityPostRequest, len(s.maturityCalls))
	copy(cp, s.maturityCalls)
	return cp
}

// DividenCalls returns a copy of recorded PostDividen requests.
func (s *JurnalPosterStub) DividenCalls() []DividenPostRequest {
	s.mu.Lock(); defer s.mu.Unlock()
	cp := make([]DividenPostRequest, len(s.dividenCalls))
	copy(cp, s.dividenCalls)
	return cp
}

// Reset clears all recorded calls and errors.
func (s *JurnalPosterStub) Reset() {
	s.mu.Lock(); defer s.mu.Unlock()
	s.akrualCalls = nil
	s.maturityCalls = nil
	s.dividenCalls = nil
	s.akrualErr = nil
	s.maturityErr = nil
	s.dividenErr = nil
}

// PostAkrual implements JurnalPoster.
func (s *JurnalPosterStub) PostAkrual(ctx context.Context, _ *sql.Tx, req AkrualPostRequest) (AkrualPostResult, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.akrualCalls = append(s.akrualCalls, req)
	s.logger.DebugContext(ctx, "JurnalPosterStub.PostAkrual",
		"event_code", req.EventCode, "akrual_id", req.AkrualID, "stage", req.Stage)
	if s.akrualErr != nil {
		return AkrualPostResult{}, s.akrualErr
	}
	return AkrualPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// PostMaturity implements JurnalPoster.
func (s *JurnalPosterStub) PostMaturity(ctx context.Context, _ *sql.Tx, req MaturityPostRequest) (MaturityPostResult, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.maturityCalls = append(s.maturityCalls, req)
	s.logger.DebugContext(ctx, "JurnalPosterStub.PostMaturity",
		"event_code", req.EventCode, "jatuh_tempo_id", req.JatuhTempoID)
	if s.maturityErr != nil {
		return MaturityPostResult{}, s.maturityErr
	}
	return MaturityPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// PostDividen implements JurnalPoster.
func (s *JurnalPosterStub) PostDividen(ctx context.Context, _ *sql.Tx, req DividenPostRequest) (DividenPostResult, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.dividenCalls = append(s.dividenCalls, req)
	s.logger.DebugContext(ctx, "JurnalPosterStub.PostDividen",
		"event_code", req.EventCode, "dividen_id", req.DividenID)
	if s.dividenErr != nil {
		return DividenPostResult{}, s.dividenErr
	}
	return DividenPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// ─── NoopJurnalPoster ────────────────────────────────────────────────────────

// NoopJurnalPoster is a no-op JurnalPoster for dev mode.
type NoopJurnalPoster struct {
	logger *slog.Logger
}

// NewNoopJurnalPoster creates a noop poster with a WARN log.
func NewNoopJurnalPoster(logger *slog.Logger) *NoopJurnalPoster {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("akrualmaturity.NoopJurnalPoster created — jurnal posting disabled. Wire P5-M2 adapter in main.go.")
	return &NoopJurnalPoster{logger: logger}
}

// PostAkrual implements JurnalPoster (noop).
func (n *NoopJurnalPoster) PostAkrual(ctx context.Context, _ *sql.Tx, req AkrualPostRequest) (AkrualPostResult, error) {
	n.logger.WarnContext(ctx, "NoopJurnalPoster.PostAkrual: skipped", "event_code", req.EventCode)
	return AkrualPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// PostMaturity implements JurnalPoster (noop).
func (n *NoopJurnalPoster) PostMaturity(ctx context.Context, _ *sql.Tx, req MaturityPostRequest) (MaturityPostResult, error) {
	n.logger.WarnContext(ctx, "NoopJurnalPoster.PostMaturity: skipped", "event_code", req.EventCode)
	return MaturityPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// PostDividen implements JurnalPoster (noop).
func (n *NoopJurnalPoster) PostDividen(ctx context.Context, _ *sql.Tx, req DividenPostRequest) (DividenPostResult, error) {
	n.logger.WarnContext(ctx, "NoopJurnalPoster.PostDividen: skipped", "event_code", req.EventCode)
	return DividenPostResult{JurnalEntryID: uuid.New(), EventCode: req.EventCode}, nil
}

// ─── InstrumenUpdater interface ───────────────────────────────────────────────

// InstrumenStatusUpdater abstracts mst.instrumen status changes on maturity.
type InstrumenStatusUpdater interface {
	// SetMatured sets mst.instrumen.status = 'MATURED' for a matured instrumen.
	SetMatured(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, actorID uuid.UUID) error
}

// InstrumenStatusUpdaterStub is a test stub.
type InstrumenStatusUpdaterStub struct {
	mu       sync.Mutex
	calls    int
	matureErr error
}

// NewInstrumenStatusUpdaterStub creates a stub.
func NewInstrumenStatusUpdaterStub() *InstrumenStatusUpdaterStub {
	return &InstrumenStatusUpdaterStub{}
}

// SetMatureError configures SetMatured to return err.
func (s *InstrumenStatusUpdaterStub) SetMatureError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.matureErr = err
}

// Calls returns the count of SetMatured calls.
func (s *InstrumenStatusUpdaterStub) Calls() int {
	s.mu.Lock(); defer s.mu.Unlock()
	return s.calls
}

// SetMatured implements InstrumenStatusUpdater.
func (s *InstrumenStatusUpdaterStub) SetMatured(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) error {
	s.mu.Lock(); defer s.mu.Unlock()
	s.calls++
	return s.matureErr
}
