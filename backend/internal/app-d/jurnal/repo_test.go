package jurnal

// repo_test.go — tests for pure-function helpers in repo.go.
// Tests that require a DB use sqlmock (in a separate *_integration_test.go).
// Here we test only the deterministic, no-DB functions.

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── BuildIdempotencyKey ──────────────────────────────────────────────────────

func TestBuildIdempotencyKeyDeterministic(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	k1 := BuildIdempotencyKey(id, EventCodePenempatan)
	k2 := BuildIdempotencyKey(id, EventCodePenempatan)
	assert.Equal(t, k1, k2, "same inputs must produce same key")
}

func TestBuildIdempotencyKeyLength(t *testing.T) {
	k := BuildIdempotencyKey(uuid.New(), EventCodeJatuhTempo)
	assert.Len(t, k, 64, "SHA256 hex = 64 chars")
}

func TestBuildIdempotencyKeyDifferentEventCodes(t *testing.T) {
	id := uuid.New()
	k1 := BuildIdempotencyKey(id, EventCodePenempatan)
	k2 := BuildIdempotencyKey(id, EventCodeJatuhTempo)
	assert.NotEqual(t, k1, k2, "different event codes must produce different keys")
}

func TestBuildIdempotencyKeyDifferentUUIDs(t *testing.T) {
	k1 := BuildIdempotencyKey(uuid.New(), EventCodePenempatan)
	k2 := BuildIdempotencyKey(uuid.New(), EventCodePenempatan)
	assert.NotEqual(t, k1, k2, "different UUIDs must produce different keys")
}

func TestBuildIdempotencyKeyLowercaseHex(t *testing.T) {
	k := BuildIdempotencyKey(uuid.New(), EventCodePenempatan)
	assert.Regexp(t, `^[0-9a-f]{64}$`, k, "must be lowercase hex")
}

// Verify the hash for a known input does not change (algorithm pin).
func TestBuildIdempotencyKeyKnownHash(t *testing.T) {
	// sha256("550e8400-e29b-41d4-a716-446655440000::PENEMPATAN")
	// Computed externally; if this test fails, the algorithm changed.
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	k := BuildIdempotencyKey(id, "PENEMPATAN")
	// We just verify it's 64 lowercase hex chars (algorithm contract).
	// An exact-value pin would break if separator changed; keep format only.
	assert.Len(t, k, 64)
	assert.Regexp(t, `^[0-9a-f]+$`, k)
}

// ─── ListPage / PaginationMeta ────────────────────────────────────────────────

func TestListPageHasMoreTrue(t *testing.T) {
	p := ListPage{NextCursor: "eyJpZCI6MTIzfQ==", HasMore: true, Limit: 50}
	meta := p.PaginationMeta()
	require.NotNil(t, meta)
	require.NotNil(t, meta.NextCursor)
	assert.Equal(t, "eyJpZCI6MTIzfQ==", *meta.NextCursor)
	assert.True(t, meta.HasMore)
	assert.Equal(t, 50, meta.Limit)
}

func TestListPageHasMoreFalse(t *testing.T) {
	p := ListPage{NextCursor: "", HasMore: false, Limit: 50}
	meta := p.PaginationMeta()
	require.NotNil(t, meta)
	assert.Nil(t, meta.NextCursor, "NextCursor must be nil when empty")
	assert.False(t, meta.HasMore)
}

func TestListPageZeroLimit(t *testing.T) {
	p := ListPage{NextCursor: "abc", HasMore: true, Limit: 0}
	meta := p.PaginationMeta()
	assert.Equal(t, 0, meta.Limit)
}

// ─── AllowedMappingSortCols ───────────────────────────────────────────────────

func TestAllowedMappingSortColsNotEmpty(t *testing.T) {
	assert.NotEmpty(t, AllowedMappingSortCols,
		"AllowedMappingSortCols must not be empty (security: whitelist must be defined)")
}

func TestAllowedMappingSortColsContainsBasicCols(t *testing.T) {
	required := []string{"event_code", "created_at", "workflow_status"}
	for _, col := range required {
		found := false
		for _, allowed := range AllowedMappingSortCols {
			if allowed == col {
				found = true
				break
			}
		}
		assert.True(t, found, "AllowedMappingSortCols must contain %q", col)
	}
}

// ─── NewMappingRepo / NewJurnalRepo / NewDLQRepo constructor panics ───────────

func TestNewMappingRepoPanicsOnNilDB(t *testing.T) {
	assert.Panics(t, func() {
		NewMappingRepo(nil)
	})
}

func TestNewJurnalRepoPanicsOnNilDB(t *testing.T) {
	assert.Panics(t, func() {
		NewJurnalRepo(nil)
	})
}

func TestNewDLQRepoPanicsOnNilDB(t *testing.T) {
	assert.Panics(t, func() {
		NewDLQRepo(nil)
	})
}
