package mtm

// repo_unit_test.go — unit-level tests for repo.go that do not require a real DB.
//
// All methods in DBRepository guard `r.db == nil` and return zero/error early.
// These tests exercise that path, plus compile-time interface compliance.
//
// DB integration tests (real PG + testcontainers) belong in repo_integration_test.go
// with build tag `//go:build integration`.

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// nilRepo is a DBRepository backed by a nil DB — exercises nil-guard paths.
func nilRepo() *DBRepository {
	return NewDBRepository(nil)
}

// ─── Compile-time interface compliance ───────────────────────────────────────

func TestDBRepository_ImplementsRepository(t *testing.T) {
	var _ Repository = (*DBRepository)(nil)
	assert.True(t, true, "compile-time check: DBRepository implements Repository")
}

// ─── NewDBRepository ─────────────────────────────────────────────────────────

func TestNewDBRepository_NilDB_NotNil(t *testing.T) {
	r := NewDBRepository(nil)
	assert.NotNil(t, r)
}

// ─── BeginTx nil-db ──────────────────────────────────────────────────────────

func TestDBRepository_BeginTx_NilDB_ReturnsError(t *testing.T) {
	_, err := nilRepo().BeginTx(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database not configured")
}

// ─── GetConfigValue nil-db ───────────────────────────────────────────────────

func TestDBRepository_GetConfigValue_NilDB_ReturnsEmpty(t *testing.T) {
	v, err := nilRepo().GetConfigValue(context.Background(), "ANY_KEY")
	require.NoError(t, err)
	assert.Empty(t, v)
}

// ─── IsHoliday nil-db ────────────────────────────────────────────────────────

func TestDBRepository_IsHoliday_NilDB_ReturnsFalse(t *testing.T) {
	ok, err := nilRepo().IsHoliday(context.Background(), time.Now())
	require.NoError(t, err)
	assert.False(t, ok)
}

// ─── GetActiveNonACInstrumen nil-db ──────────────────────────────────────────

func TestDBRepository_GetActiveNonACInstrumen_NilDB_ReturnsNil(t *testing.T) {
	rows, err := nilRepo().GetActiveNonACInstrumen(context.Background())
	require.NoError(t, err)
	assert.Nil(t, rows)
}

// ─── GetFeedPrice nil-db ─────────────────────────────────────────────────────

func TestDBRepository_GetFeedPrice_NilDB_ReturnsNil(t *testing.T) {
	fp, err := nilRepo().GetFeedPrice(context.Background(), uuid.New(), "IDR", time.Now())
	require.NoError(t, err)
	assert.Nil(t, fp)
}

// ─── GetApprovedKurs nil-db ──────────────────────────────────────────────────

func TestDBRepository_GetApprovedKurs_NilDB_ReturnsNil(t *testing.T) {
	ks, err := nilRepo().GetApprovedKurs(context.Background(), "USD", time.Now())
	require.NoError(t, err)
	assert.Nil(t, ks)
}

// ─── GetHargaBukuIdr nil-db ───────────────────────────────────────────────────

func TestDBRepository_GetHargaBukuIdr_NilDB_ReturnsNil(t *testing.T) {
	hb, err := nilRepo().GetHargaBukuIdr(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, hb)
}

// ─── Insert nil-db ───────────────────────────────────────────────────────────

func TestDBRepository_Insert_NilDB_ReturnsError(t *testing.T) {
	m := makeMtm(StatusAutoPOSTED)
	err := nilRepo().Insert(context.Background(), (*sql.Tx)(nil), m)
	require.Error(t, err)
}

// ─── GetByID nil-db ──────────────────────────────────────────────────────────

func TestDBRepository_GetByID_NilDB_ReturnsNil(t *testing.T) {
	m, err := nilRepo().GetByID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, m)
}

// ─── UpdateStatus nil-db ─────────────────────────────────────────────────────

func TestDBRepository_UpdateStatus_NilDB_ReturnsError(t *testing.T) {
	err := nilRepo().UpdateStatus(context.Background(), (*sql.Tx)(nil), uuid.New(), StatusUpdate{
		Status:    StatusApproved,
		UpdatedBy: uuid.New(),
	})
	require.Error(t, err)
}

// ─── ExistsActive nil-db ─────────────────────────────────────────────────────

func TestDBRepository_ExistsActive_NilDB_ReturnsFalse(t *testing.T) {
	ok, existing, err := nilRepo().ExistsActive(context.Background(), uuid.New(), time.Now(), "MANUAL")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, existing)
}

// ─── List nil-db ─────────────────────────────────────────────────────────────

func TestDBRepository_List_NilDB_ReturnsEmpty(t *testing.T) {
	rows, hasMore, total, err := nilRepo().List(context.Background(), listquery.Query{}, "TUGURE", 50)
	require.NoError(t, err)
	assert.Nil(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

// ─── ListByBatchID nil-db ────────────────────────────────────────────────────

func TestDBRepository_ListByBatchID_NilDB_ReturnsNil(t *testing.T) {
	rows, err := nilRepo().ListByBatchID(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, rows)
}

// ─── ListStaleAlerts nil-db ──────────────────────────────────────────────────

func TestDBRepository_ListStaleAlerts_NilDB_ReturnsEmpty(t *testing.T) {
	rows, hasMore, total, err := nilRepo().ListStaleAlerts(context.Background(), "TUGURE", 50)
	require.NoError(t, err)
	assert.Nil(t, rows)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

// ─── LockMtmForPeriode nil-db ────────────────────────────────────────────────

func TestDBRepository_LockMtmForPeriode_NilDB_ReturnsNoError(t *testing.T) {
	n, err := nilRepo().LockMtmForPeriode(context.Background(), (*sql.Tx)(nil),
		uuid.New(), time.Now(), time.Now().AddDate(0, 1, 0), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// ─── UnlockMtmForPeriode nil-db ──────────────────────────────────────────────

func TestDBRepository_UnlockMtmForPeriode_NilDB_ReturnsNoError(t *testing.T) {
	n, err := nilRepo().UnlockMtmForPeriode(context.Background(), (*sql.Tx)(nil),
		uuid.New(), time.Now(), time.Now().AddDate(0, 1, 0), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// ─── InsertUploadBatch nil-db ─────────────────────────────────────────────────

func TestDBRepository_InsertUploadBatch_NilDB_ReturnsNoError(t *testing.T) {
	b := &UploadBatch{
		ID:         uuid.New(),
		BatchType:  "MTM_UPLOAD",
		UploaderID: uuid.New(),
		Status:     "PENDING_REVIEW",
		CreatedAt:  time.Now(),
		CreatedBy:  uuid.New(),
	}
	err := nilRepo().InsertUploadBatch(context.Background(), (*sql.Tx)(nil), b)
	require.NoError(t, err)
}

// ─── GetUploadBatch nil-db ───────────────────────────────────────────────────

func TestDBRepository_GetUploadBatch_NilDB_ReturnsNil(t *testing.T) {
	b, err := nilRepo().GetUploadBatch(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, b)
}
