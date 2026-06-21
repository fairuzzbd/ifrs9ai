package pocidelta

// jurnal_poster.go — JurnalPoster interface for POCI delta posting.
// Mirrors M9 akrualmaturity pattern. Real P5-M2 implementation injected via main.go.
//
// Reference: docs/state-machines/p5-m10-poci-delta.md §4 (jurnal sign convention)

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrMappingJurnalDraft is returned when the mapping_jurnal rule for POCI_ECL_DELTA
// events is still in DRAFT status. Jurnal posting is blocked until the mapping is
// APPROVED by ROLE-AKUN-CTL (P5-M2 gate per DEC-P5-M1-002).
//
// See: internal/app-d/jurnal mapping_jurnal_header workflow (P5-M2).
var ErrMappingJurnalDraft = fmt.Errorf("mapping_jurnal for POCI_ECL_DELTA event is DRAFT — approve mapping via P5-M2 before posting")

// PociDeltaPostRequest carries data for posting POCI delta ECL jurnal.
type PociDeltaPostRequest struct {
	// EventCode is POCI_ECL_DELTA_INCREASE or POCI_ECL_DELTA_DECREASE.
	EventCode       string
	InstrumenID     uuid.UUID
	DeltaLogID      uuid.UUID
	CalcRunID       uuid.UUID
	PeriodeID       uuid.UUID
	TanggalCompute  time.Time
	// AmountIDR is abs(delta_ecl) — always positive. Direction from EventCode.
	AmountIDR       decimal.Decimal
	Direction       Direction
	ActorID         uuid.UUID
	TenantID        string
	IdempotencyKey  string // hash(calc_run_id + instrumen_id + "POCI_ECL_DELTA")
}

// PociDeltaPostResult holds the result of a POCI delta jurnal post.
type PociDeltaPostResult struct {
	JurnalHeaderID uuid.UUID
	EventCode      string
}

// JurnalPoster abstracts P5-M2 jurnal engine for POCI delta posting.
type JurnalPoster interface {
	// PostPociDelta posts the POCI ECL delta jurnal in the given transaction.
	// EventCode determines debit/kredit direction (INCREASE vs DECREASE).
	PostPociDelta(ctx context.Context, tx *sql.Tx, req PociDeltaPostRequest) (PociDeltaPostResult, error)
}

// ─── Stub implementation ─────────────────────────────────────────────────────

// JurnalPosterStub records calls and returns configurable results (for tests).
type JurnalPosterStub struct {
	mu      sync.Mutex
	calls   []PociDeltaPostRequest
	postErr error
	logger  *slog.Logger
}

// NewJurnalPosterStub creates a test stub.
func NewJurnalPosterStub(logger *slog.Logger) *JurnalPosterStub {
	if logger == nil {
		logger = slog.Default()
	}
	return &JurnalPosterStub{logger: logger}
}

// SetPostError configures PostPociDelta to return err.
func (s *JurnalPosterStub) SetPostError(err error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.postErr = err
}

// Calls returns a copy of all recorded PostPociDelta calls.
func (s *JurnalPosterStub) Calls() []PociDeltaPostRequest {
	s.mu.Lock(); defer s.mu.Unlock()
	cp := make([]PociDeltaPostRequest, len(s.calls))
	copy(cp, s.calls)
	return cp
}

// Reset clears all recorded calls and errors.
func (s *JurnalPosterStub) Reset() {
	s.mu.Lock(); defer s.mu.Unlock()
	s.calls = nil
	s.postErr = nil
}

// PostPociDelta implements JurnalPoster (stub).
func (s *JurnalPosterStub) PostPociDelta(ctx context.Context, _ *sql.Tx, req PociDeltaPostRequest) (PociDeltaPostResult, error) {
	s.mu.Lock(); defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	s.logger.DebugContext(ctx, "JurnalPosterStub.PostPociDelta",
		"event_code", req.EventCode,
		"direction", req.Direction,
		"amount", req.AmountIDR.StringFixed(4),
		"instrumen_id", req.InstrumenID,
	)
	if s.postErr != nil {
		return PociDeltaPostResult{}, s.postErr
	}
	return PociDeltaPostResult{JurnalHeaderID: uuid.New(), EventCode: req.EventCode}, nil
}

// ─── NoopJurnalPoster ────────────────────────────────────────────────────────

// NoopJurnalPoster is a no-op JurnalPoster for dev mode.
// Logs a WARN on every call. Wire P5-M2 adapter in main.go.
//
// m5 fix: if MappingJurnalStatus == "DRAFT", PostPociDelta returns ErrMappingJurnalDraft
// to guard against posting when the mapping rule is not yet approved (P5-M2 gate).
type NoopJurnalPoster struct {
	logger               *slog.Logger
	MappingJurnalStatus  string // default "": treat as approved; "DRAFT" → block
}

// NewNoopJurnalPoster creates a noop poster.
func NewNoopJurnalPoster(logger *slog.Logger) *NoopJurnalPoster {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn("pocidelta.NoopJurnalPoster: jurnal posting disabled. Wire P5-M2 adapter in main.go.")
	return &NoopJurnalPoster{logger: logger}
}

// PostPociDelta implements JurnalPoster (noop).
// Returns ErrMappingJurnalDraft if MappingJurnalStatus == "DRAFT" (m5 fix).
func (n *NoopJurnalPoster) PostPociDelta(ctx context.Context, _ *sql.Tx, req PociDeltaPostRequest) (PociDeltaPostResult, error) {
	// m5: block posting if mapping rule is still in DRAFT (P5-M2 gate).
	if n.MappingJurnalStatus == "DRAFT" {
		n.logger.WarnContext(ctx, "NoopJurnalPoster.PostPociDelta: blocked — mapping_jurnal DRAFT",
			"event_code", req.EventCode,
			"instrumen_id", req.InstrumenID,
		)
		return PociDeltaPostResult{}, ErrMappingJurnalDraft
	}
	n.logger.WarnContext(ctx, "NoopJurnalPoster.PostPociDelta: skipped (noop)",
		"event_code", req.EventCode,
		"instrumen_id", req.InstrumenID,
	)
	return PociDeltaPostResult{JurnalHeaderID: uuid.New(), EventCode: req.EventCode}, nil
}
