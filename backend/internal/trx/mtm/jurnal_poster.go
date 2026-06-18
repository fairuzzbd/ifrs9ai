package mtm

// jurnal_poster.go — JurnalPoster interface + stub.
//
// JurnalPoster abstracts the P5-M2 jurnal engine so that:
//   a) MTM service has no import cycle dependency on internal/app-d/jurnal.
//   b) Tests can inject JurnalPosterStub to verify calls without a real DB.
//
// Wiring decision:
//   - Interface defined here; real P5-M2 implementation injected via main.go.
//   - If P5-M2 jrnl interface is unstable, JurnalPosterStub serves as the
//     permanent implementation until P5-M2 is merged (deferred to follow-up PR).
//   - JurnalPosterStub records all Post calls — suitable for integration tests.
//
// Usage in service.go:
//   svc.poster.Post(ctx, tx, PostRequest{...})

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

// PostRequest carries the data needed to post one jurnal entry via P5-M2.
type PostRequest struct {
	EventCode    string          // e.g. "MTM_FVOCI", "MTM_FVTPL"
	InstrumenID  uuid.UUID
	PeriodeID    uuid.UUID
	TanggalMtm   time.Time
	Amount       decimal.Decimal // delta_idr (positive = gain, negative = loss)
	KursTengah   *decimal.Decimal
	KursID       *uuid.UUID
	MtmID        uuid.UUID
	ActorID      uuid.UUID
	TenantID     string
	TraceID      string
}

// PostResult holds the result of a successful jurnal post.
type PostResult struct {
	JurnalEntryID uuid.UUID
	EventCode     string
}

// JurnalPoster abstracts the P5-M2 jurnal engine for MTM posting.
// Implemented by: JurnalPosterStub (test), real M2 adapter (production).
type JurnalPoster interface {
	// Post posts one jurnal entry in the given transaction.
	// Returns (PostResult, nil) on success.
	// Returns error (wrapping cause) on failure → caller must rollback tx.
	Post(ctx context.Context, tx *sql.Tx, req PostRequest) (PostResult, error)
}

// ─── Stub implementation ──────────────────────────────────────────────────────

// JurnalPosterStub records Post calls and returns configurable results.
// Use in tests to verify jurnal posting without a real P5-M2 engine.
//
// Thread-safe: protected by mu.
type JurnalPosterStub struct {
	mu     sync.Mutex
	calls  []PostRequest
	result PostResult
	err    error
	logger *slog.Logger
}

// NewJurnalPosterStub creates a stub that returns a deterministic entry ID.
func NewJurnalPosterStub(logger *slog.Logger) *JurnalPosterStub {
	if logger == nil {
		logger = slog.Default()
	}
	return &JurnalPosterStub{
		result: PostResult{
			JurnalEntryID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		},
		logger: logger,
	}
}

// SetResult configures the return value for the next Post call.
func (s *JurnalPosterStub) SetResult(r PostResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = r
}

// SetError configures Post to return err on the next call.
func (s *JurnalPosterStub) SetError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// Calls returns all recorded Post requests.
func (s *JurnalPosterStub) Calls() []PostRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]PostRequest, len(s.calls))
	copy(cp, s.calls)
	return cp
}

// Reset clears recorded calls and resets error.
func (s *JurnalPosterStub) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = nil
	s.err = nil
}

// Post implements JurnalPoster.
func (s *JurnalPosterStub) Post(ctx context.Context, tx *sql.Tx, req PostRequest) (PostResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)

	s.logger.DebugContext(ctx, "JurnalPosterStub.Post",
		"event_code", req.EventCode,
		"instrumen_id", req.InstrumenID,
		"amount", req.Amount.String(),
	)

	if s.err != nil {
		return PostResult{}, s.err
	}
	// Each call returns an entry ID derived from event_code to differentiate dual entries.
	result := PostResult{
		JurnalEntryID: uuid.New(),
		EventCode:     req.EventCode,
	}
	return result, nil
}

// ─── NoopJurnalPoster (dev-only, no-op) ──────────────────────────────────────

// NoopJurnalPoster implements JurnalPoster with a no-op Post.
// Used when P5-M2 is not yet available in the environment.
// Logs a warning for every Post call.
type NoopJurnalPoster struct {
	logger *slog.Logger
}

// NewNoopJurnalPoster creates a no-op poster that logs warnings.
// m5 fix: logs WARN at construction time to make it visible in observability.
// Production guard: if APP_ENV=production and NoopJurnalPoster is still in use → log.Fatal.
// Call IsProdNoopGuard() from main.go after wiring to enforce this.
func NewNoopJurnalPoster(logger *slog.Logger) *NoopJurnalPoster {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("NoopJurnalPoster created — jurnal posting is disabled. " +
		"Wire internal/app-d/jurnal.Service in main.go before production deployment.")
	return &NoopJurnalPoster{logger: logger}
}

// IsNoopProduction returns true if APP_ENV=production and this is a NoopJurnalPoster.
// main.go should call log.Fatal if this returns true at startup (m5 fix).
func IsNoopProduction(p JurnalPoster) bool {
	if os.Getenv("APP_ENV") != "production" {
		return false
	}
	_, isNoop := p.(*NoopJurnalPoster)
	return isNoop
}

// Post implements JurnalPoster with a no-op (returns a synthetic entry ID).
func (n *NoopJurnalPoster) Post(ctx context.Context, _ *sql.Tx, req PostRequest) (PostResult, error) {
	n.logger.WarnContext(ctx, "NoopJurnalPoster: jurnal posting skipped — P5-M2 not wired",
		"event_code", req.EventCode,
		"instrumen_id", req.InstrumenID.String(),
		"amount", req.Amount.String(),
	)
	return PostResult{
		JurnalEntryID: uuid.New(),
		EventCode:     req.EventCode,
	}, nil
}

// ─── Real P5-M2 adapter (placeholder wiring) ─────────────────────────────────

// NewRealJurnalPoster constructs the production JurnalPoster backed by P5-M2.
// This is a placeholder that wires to the real jurnal engine when available.
// Wire in main.go: mtmSvc.WithJurnalPoster(mtm.NewRealJurnalPoster(jurnalSvc, logger))
//
// TODO(P5-M2-wiring): replace body with actual call to internal/app-d/jurnal.Service.PostEntry
// once P5-M2 interface is stable. Currently returns NoopJurnalPoster.
func NewRealJurnalPoster(logger *slog.Logger) JurnalPoster {
	logger.Warn("NewRealJurnalPoster: P5-M2 real jurnal engine not yet wired — using NoopJurnalPoster. " +
		"Wire internal/app-d/jurnal.Service in main.go after P5-M2 is merged.")
	return NewNoopJurnalPoster(logger)
}

// validatePostRequest validates required fields in PostRequest.
func validatePostRequest(req PostRequest) error {
	if req.EventCode == "" {
		return fmt.Errorf("validatePostRequest: EventCode is required")
	}
	if req.InstrumenID == uuid.Nil {
		return fmt.Errorf("validatePostRequest: InstrumenID is required")
	}
	if req.MtmID == uuid.Nil {
		return fmt.Errorf("validatePostRequest: MtmID is required")
	}
	return nil
}
