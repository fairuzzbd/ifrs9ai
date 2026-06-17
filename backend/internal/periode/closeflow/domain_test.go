package closeflow_test

// domain_test.go — Unit tests for pure domain functions (CanTransition, status helpers).
// No DB, no context, no mocks — fully deterministic.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── PeriodeStatus helpers ────────────────────────────────────────────────────

func TestPeriodeStatus_IsValid(t *testing.T) {
	valid := []closeflow.PeriodeStatus{
		closeflow.PeriodeStatusOpen,
		closeflow.PeriodeStatusSoftClosed,
		closeflow.PeriodeStatusHardClosePending,
		closeflow.PeriodeStatusClosed,
	}
	for _, s := range valid {
		assert.True(t, s.IsValid(), "expected %s to be valid", s)
	}
	assert.False(t, closeflow.PeriodeStatus("INVALID").IsValid())
}

func TestPeriodeStatus_AllowsMutation(t *testing.T) {
	assert.True(t, closeflow.PeriodeStatusOpen.AllowsMutation())
	assert.False(t, closeflow.PeriodeStatusSoftClosed.AllowsMutation())
	assert.False(t, closeflow.PeriodeStatusHardClosePending.AllowsMutation())
	assert.False(t, closeflow.PeriodeStatusClosed.AllowsMutation())
}

func TestPeriodeStatus_IsTerminal(t *testing.T) {
	assert.True(t, closeflow.PeriodeStatusClosed.IsTerminal())
	assert.False(t, closeflow.PeriodeStatusOpen.IsTerminal())
	assert.False(t, closeflow.PeriodeStatusSoftClosed.IsTerminal())
}

// ─── CanTransition ────────────────────────────────────────────────────────────

func TestCanTransition_SoftCloseRequest_HappyPath(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusOpen, "soft-close-request", false, false)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_SoftCloseRequest_WrongStatus(t *testing.T) {
	for _, s := range []closeflow.PeriodeStatus{
		closeflow.PeriodeStatusSoftClosed,
		closeflow.PeriodeStatusHardClosePending,
		closeflow.PeriodeStatusClosed,
	} {
		ok, err := closeflow.CanTransition(s, "soft-close-request", false, false)
		assert.False(t, ok, "expected OPEN required, got %s", s)
		assert.Error(t, err)
	}
}

func TestCanTransition_SoftCloseApprove_HappyPath(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusOpen, "soft-close-approve", false, false)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_HardCloseRequest_HappyPath(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusSoftClosed, "hard-close-request", false, false)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_HardCloseRequest_WrongStatus(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusOpen, "hard-close-request", false, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_HardCloseApprove_WithStepUp(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusHardClosePending, "hard-close-approve", true, false)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_HardCloseApprove_NoStepUp(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusHardClosePending, "hard-close-approve", false, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_HardCloseApprove_WrongStatus(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusSoftClosed, "hard-close-approve", true, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_HardCloseReject_HappyPath(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusHardClosePending, "hard-close-reject", false, false)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_HardCloseReject_WrongStatus(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusSoftClosed, "hard-close-reject", false, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_ReopenSoftClosedToOpen(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusSoftClosed, "reopen-soft-closed-to-open", false, false)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_ReopenSoftClosedToOpen_WrongStatus(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusOpen, "reopen-soft-closed-to-open", false, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_ReopenClosedToSoftClosed_HappyPath(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusClosed, "reopen-closed-to-soft-closed", true, true)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestCanTransition_ReopenClosedToSoftClosed_NoGrace(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusClosed, "reopen-closed-to-soft-closed", true, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_ReopenClosedToSoftClosed_NoStepUp(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusClosed, "reopen-closed-to-soft-closed", false, true)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestCanTransition_UnknownAction(t *testing.T) {
	ok, err := closeflow.CanTransition(closeflow.PeriodeStatusOpen, "unknown-action", false, false)
	assert.False(t, ok)
	assert.Error(t, err)
}

// ─── DefaultConfig ────────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := closeflow.DefaultConfig()
	assert.Equal(t, 24, cfg.SoftCloseChecklistStaleHours)
	assert.Equal(t, 48, cfg.HardCloseGraceWindowHours)
	assert.NotEmpty(t, cfg.SoftClosedMutationAllowlist)
}

// ─── JurnalBalancedThreshold (DEC-016) ───────────────────────────────────────

func TestJurnalBalancedThreshold_IsDecimal(t *testing.T) {
	// Threshold must be exactly IDR 0.01 (no float imprecision).
	assert.Equal(t, "0.01", closeflow.JurnalBalancedThreshold.String())
}

// ─── HashStepUpToken ─────────────────────────────────────────────────────────

func TestHashStepUpToken_Deterministic(t *testing.T) {
	a := closeflow.HashStepUpToken("token-abc-123")
	b := closeflow.HashStepUpToken("token-abc-123")
	assert.Equal(t, a, b)
	// Must produce valid hex string.
	assert.Len(t, a, 64) // SHA-256 → 32 bytes → 64 hex chars
}

func TestHashStepUpToken_DifferentInput(t *testing.T) {
	a := closeflow.HashStepUpToken("token-abc")
	b := closeflow.HashStepUpToken("token-xyz")
	assert.NotEqual(t, a, b)
}
