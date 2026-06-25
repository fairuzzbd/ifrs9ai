package gldelivery_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

func TestGlHostStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		status   GlHostStatus
		terminal bool
	}{
		{GlHostStatusDelivered, true},
		{GlHostStatusDeadLetter, true},
		{GlHostStatusPendingDelivery, false},
		{GlHostStatusDeliveryInFlight, false},
		{GlHostStatusRetrying, false},
		{GlHostStatusFailed, false},
	}
	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			assert.Equal(t, tc.terminal, tc.status.IsTerminal())
		})
	}
}

func TestGlHostStatus_CanManualRetry(t *testing.T) {
	assert.True(t, GlHostStatusFailed.CanManualRetry())
	assert.False(t, GlHostStatusDelivered.CanManualRetry())
	assert.False(t, GlHostStatusDeadLetter.CanManualRetry())
	assert.False(t, GlHostStatusPendingDelivery.CanManualRetry())
	assert.False(t, GlHostStatusRetrying.CanManualRetry())
}

func TestDLQStatus_CanReplay(t *testing.T) {
	assert.True(t, DLQStatusFailed.CanReplay())
	assert.False(t, DLQStatusReplaying.CanReplay())
	assert.False(t, DLQStatusReplayedOK.CanReplay())
	assert.False(t, DLQStatusAbandoned.CanReplay())
}

func TestDLQStatus_CanDiscard(t *testing.T) {
	assert.True(t, DLQStatusFailed.CanDiscard())
	assert.False(t, DLQStatusReplaying.CanDiscard())
	assert.False(t, DLQStatusReplayedOK.CanDiscard())
	assert.False(t, DLQStatusAbandoned.CanDiscard())
}

func TestGlHostStatus_Constants(t *testing.T) {
	// Ensure string values match OpenAPI spec.
	assert.Equal(t, GlHostStatus("PENDING_DELIVERY"), GlHostStatusPendingDelivery)
	assert.Equal(t, GlHostStatus("DELIVERY_IN_FLIGHT"), GlHostStatusDeliveryInFlight)
	assert.Equal(t, GlHostStatus("DELIVERED"), GlHostStatusDelivered)
	assert.Equal(t, GlHostStatus("RETRYING"), GlHostStatusRetrying)
	assert.Equal(t, GlHostStatus("FAILED"), GlHostStatusFailed)
	assert.Equal(t, GlHostStatus("DEAD_LETTER"), GlHostStatusDeadLetter)
}

func TestMismatchType_Constants(t *testing.T) {
	assert.Equal(t, MismatchType("BLIPS_ONLY"), MismatchTypeBlipsOnly)
	assert.Equal(t, MismatchType("GL_ONLY"), MismatchTypeGLOnly)
	assert.Equal(t, MismatchType("AMOUNT_DIFF"), MismatchTypeAmountDiff)
}

func TestPermissionConstants(t *testing.T) {
	// Ensure permission constants are non-empty and follow {entity}.{action} pattern.
	perms := []string{
		PermGlDeliveryRead,
		PermGlDeliveryReadRaw,
		PermGlDeliveryRetry,
		PermGlDeliveryReplay,
		PermGlDeliveryDiscard,
		PermReconciliationRead,
		PermReconciliationRun,
	}
	for _, p := range perms {
		assert.NotEmpty(t, p, "permission constant must not be empty")
		assert.Contains(t, p, ".", "permission must follow entity.action pattern")
	}
}
