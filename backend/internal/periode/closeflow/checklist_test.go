package closeflow_test

// checklist_test.go — Unit tests for BuildChecklistJSONB and BuildChecklistDetails.
// ChecklistService.Evaluate() requires a real DB so those tests are in service_test.go
// via sqlmock.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

// ─── BuildChecklistJSONB ──────────────────────────────────────────────────────

func TestBuildChecklistJSONB_AllPassed(t *testing.T) {
	result := closeflow.ChecklistEvalResult{
		EvaluatedAt: time.Now(),
		AllPassed:   true,
		Items: []closeflow.ChecklistItem{
			{Key: closeflow.ChecklistKeyPendingApprovalZero, Label: "L1", Passed: true, Detail: "OK"},
			{Key: closeflow.ChecklistKeyJurnalBalanced, Label: "L2", Passed: true, Detail: "OK"},
		},
	}

	out := closeflow.BuildChecklistJSONB(result)
	assert.NotNil(t, out)
	items, ok := out["items"].([]map[string]any)
	require.True(t, ok)
	assert.Len(t, items, 2)
	assert.Equal(t, string(closeflow.ChecklistKeyPendingApprovalZero), items[0]["key"])
	assert.Equal(t, true, items[0]["passed"])
}

func TestBuildChecklistJSONB_WithActionURL(t *testing.T) {
	url := "/jurnal/gl-delivery-dlq"
	result := closeflow.ChecklistEvalResult{
		EvaluatedAt: time.Now(),
		AllPassed:   false,
		Items: []closeflow.ChecklistItem{
			{Key: closeflow.ChecklistKeyGLDelivered, Label: "GL", Passed: false, Detail: "FAILED", ActionURL: &url},
		},
	}

	out := closeflow.BuildChecklistJSONB(result)
	items := out["items"].([]map[string]any)
	assert.Equal(t, url, items[0]["action_url"])
}

func TestBuildChecklistJSONB_EvaluatedAtFormatted(t *testing.T) {
	ts := time.Date(2026, 6, 17, 10, 30, 0, 0, time.UTC)
	result := closeflow.ChecklistEvalResult{EvaluatedAt: ts, AllPassed: true, Items: nil}
	out := closeflow.BuildChecklistJSONB(result)
	assert.Equal(t, "2026-06-17T10:30:00Z", out["evaluated_at"])
}

// ─── BuildChecklistDetails ───────────────────────────────────────────────────

func TestBuildChecklistDetails_NoFailures(t *testing.T) {
	result := closeflow.ChecklistEvalResult{
		Items: []closeflow.ChecklistItem{
			{Key: closeflow.ChecklistKeyPendingApprovalZero, Passed: true},
			{Key: closeflow.ChecklistKeyJurnalBalanced, Passed: true},
		},
	}
	details := closeflow.BuildChecklistDetails(result)
	assert.Empty(t, details)
}

func TestBuildChecklistDetails_WithFailures(t *testing.T) {
	result := closeflow.ChecklistEvalResult{
		Items: []closeflow.ChecklistItem{
			{Key: closeflow.ChecklistKeyPendingApprovalZero, Passed: true, Detail: "OK"},
			{Key: closeflow.ChecklistKeyJurnalBalanced, Passed: false, Detail: "Delta IDR 500"},
			{Key: closeflow.ChecklistKeyGLDelivered, Passed: false, Detail: "3 FAILED"},
			{Key: closeflow.ChecklistKeyReconPass, Passed: true, Detail: "OK"},
		},
	}
	details := closeflow.BuildChecklistDetails(result)
	assert.Len(t, details, 2)

	// Check field names match the key strings.
	fieldSet := map[string]bool{}
	for _, d := range details {
		assert.IsType(t, domainerrors.Detail{}, d)
		fieldSet[d.Field] = true
	}
	assert.True(t, fieldSet[string(closeflow.ChecklistKeyJurnalBalanced)])
	assert.True(t, fieldSet[string(closeflow.ChecklistKeyGLDelivered)])
}

// ─── ChecklistKey constants ───────────────────────────────────────────────────

func TestChecklistKey_Values(t *testing.T) {
	assert.Equal(t, closeflow.ChecklistKey("PENDING_APPROVAL_ZERO"), closeflow.ChecklistKeyPendingApprovalZero)
	assert.Equal(t, closeflow.ChecklistKey("JURNAL_BALANCED"), closeflow.ChecklistKeyJurnalBalanced)
	assert.Equal(t, closeflow.ChecklistKey("GL_DELIVERED"), closeflow.ChecklistKeyGLDelivered)
	assert.Equal(t, closeflow.ChecklistKey("RECON_PASS"), closeflow.ChecklistKeyReconPass)
}
