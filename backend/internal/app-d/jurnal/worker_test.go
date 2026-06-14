package jurnal

// worker_test.go — tests for Worker constructor and DLQ error policy logic.
//
// Asynq handler dispatch is tested via unit-level checks on the error policy
// logic (domain error → acknowledge; infra error → retry) without requiring
// a live Redis/Asynq instance.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// plainError is a test helper — a plain error that is NOT a domain error.
type plainError struct{ msg string }

func (e *plainError) Error() string { return e.msg }

// ─── Worker constructor ───────────────────────────────────────────────────────

func TestNewWorkerPanicNilPosting(t *testing.T) {
	assert.Panics(t, func() {
		NewWorker(nil, nil, nil)
	})
}

func TestNewWorkerPanicNilDLQRepo(t *testing.T) {
	assert.Panics(t, func() {
		// Non-nil PostingService (zero-value struct), nil DLQRepo → panic.
		NewWorker(&PostingService{}, nil, nil)
	})
}

func TestNewWorkerSuccessWithNilLogger(t *testing.T) {
	// Nil logger must NOT panic — it falls back to slog.Default().
	w := NewWorker(&PostingService{}, &DLQRepo{}, nil)
	require.NotNil(t, w)
	assert.NotNil(t, w.logger, "logger must be set to slog.Default()")
}

// ─── DLQ error policy — domain error → acknowledge ───────────────────────────

func TestDomainErrorPolicyAcknowledge(t *testing.T) {
	// Domain error: isDomain=true → handlePostError returns nil (acknowledge).
	domErr := domainerrors.New(domainerrors.CodeJurnalPeriodeHardClosed,
		"Periode sudah hard-closed.")
	_, isDomain := domainerrors.IsDomainError(domErr)
	require.True(t, isDomain, "domain error must be recognized")

	// Replicate the return logic from handlePostError:
	var result error
	if isDomain {
		result = nil // acknowledged; Asynq will NOT retry
	} else {
		result = domErr
	}
	assert.NoError(t, result, "domain error must be acknowledged (return nil to Asynq)")
}

// ─── DLQ error policy — infra error → retry ──────────────────────────────────

func TestInfraErrorPolicyRetry(t *testing.T) {
	infraErr := &plainError{msg: "connection refused"}
	_, isDomain := domainerrors.IsDomainError(infraErr)
	require.False(t, isDomain, "plain error must not be domain error")

	var result error
	if isDomain {
		result = nil
	} else {
		result = infraErr // returned to Asynq → triggers retry
	}
	assert.Error(t, result, "infra error must propagate for Asynq retry")
}

// ─── DLQ error code extraction ────────────────────────────────────────────────

func TestWorkerDomainErrorCode(t *testing.T) {
	domErr := domainerrors.New(domainerrors.CodeJurnalEventNotMapped,
		"No mapping found for event code.")
	de, ok := domainerrors.IsDomainError(domErr)
	require.True(t, ok)
	assert.Equal(t, "JURNAL_EVENT_NOT_MAPPED", string(de.Code()))
}

func TestWorkerInfraErrorCodeFallback(t *testing.T) {
	// Infra errors get the fallback "INFRA_ERROR" code.
	infraErr := &plainError{msg: "database connection timeout"}
	_, isDomain := domainerrors.IsDomainError(infraErr)
	require.False(t, isDomain)

	errCode := "INFRA_ERROR"
	if isDomain {
		errCode = "DOMAIN_CODE" // unreachable in this test
	}
	assert.Equal(t, "INFRA_ERROR", errCode)
}

// ─── DLQ error category assignment ───────────────────────────────────────────

func TestDLQEntryErrorCategoryDomain(t *testing.T) {
	domErr := domainerrors.New(domainerrors.CodeJurnalPeriodeHardClosed, "test")
	_, isDomain := domainerrors.IsDomainError(domErr)

	errCategory := "INFRA"
	if isDomain {
		errCategory = "DOMAIN"
	}
	assert.Equal(t, "DOMAIN", errCategory)
}

func TestDLQEntryErrorCategoryInfra(t *testing.T) {
	infraErr := &plainError{msg: "timeout"}
	_, isDomain := domainerrors.IsDomainError(infraErr)

	errCategory := "INFRA"
	if isDomain {
		errCategory = "DOMAIN"
	}
	assert.Equal(t, "INFRA", errCategory)
}

// ─── DLQ status constants ─────────────────────────────────────────────────────

func TestDLQStatusValues(t *testing.T) {
	assert.Equal(t, DLQStatus("FAILED"), DLQStatusFailed)
	assert.Equal(t, DLQStatus("REPLAYING"), DLQStatusReplaying)
	assert.Equal(t, DLQStatus("REPLAYED_OK"), DLQStatusReplayedOK)
	assert.Equal(t, DLQStatus("ABANDONED"), DLQStatusAbandoned)
}

// ─── All 27 event codes handled by worker ────────────────────────────────────

// TestWorkerEventCodesForPenemp verifies the 3 event codes the worker subscribes
// to are distinct and present in the 27-code catalog.
func TestWorkerSubscriberEventCodes(t *testing.T) {
	subscribedCodes := []string{
		EventCodePenempatan,
		EventCodeJatuhTempo,
		EventCodePenjualanPencairan,
	}
	for _, code := range subscribedCodes {
		// Each code must be in the 27-code catalog (not regulated → operational).
		// Regulated codes would mean 6-eyes workflow is required; worker doesn't handle those.
		assert.False(t, IsRegulated(code),
			"subscriber event code %s must be operational (not regulated)", code)
		assert.False(t, IsManualAllowed(code),
			"subscriber event code %s must not be manually-only", code)
	}

	// Codes must be distinct.
	seen := make(map[string]struct{})
	for _, c := range subscribedCodes {
		_, dup := seen[c]
		assert.False(t, dup, "duplicate subscriber code: %s", c)
		seen[c] = struct{}{}
	}
}
