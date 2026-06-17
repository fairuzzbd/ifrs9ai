package closeflow_test

// middleware_test.go — Tests for PeriodeLockMiddleware.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/periode/closeflow"
)

func setupMiddlewareRouter(t *testing.T, mw *closeflow.PeriodeLockMiddleware) (*gin.Engine, string) {
	t.Helper()
	r := gin.New()
	// A test route that uses the middleware via route param.
	periodeGroup := r.Group("/api/v1/transaksi/:periode_id", mw.Handler())
	periodeGroup.POST("/foo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r, ""
}

func TestPeriodeLockMiddleware_OPEN_AllowsMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000001"
	now := testNow()

	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").WillReturnRows(openPeriodeRows(periodeID, now))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r, _ := setupMiddlewareRouter(t, mw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPeriodeLockMiddleware_SOFT_CLOSED_BlocksMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000002"
	now := testNow()

	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").WillReturnRows(softClosedPeriodeRows(periodeID, now))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r, _ := setupMiddlewareRouter(t, mw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 423, w.Code)
}

func TestPeriodeLockMiddleware_CLOSED_BlocksMutation(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000003"
	now := testNow()

	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").WillReturnRows(closedPeriodeRows(periodeID, now))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r, _ := setupMiddlewareRouter(t, mw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, 423, w.Code)
}

func TestPeriodeLockMiddleware_GET_AllowsThrough(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	// GET should bypass middleware entirely (no DB query expected).
	periodeID := "00000000-0000-0000-0000-000000000004"

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r := gin.New()
	periodeGroup := r.Group("/api/v1/transaksi/:periode_id", mw.Handler())
	periodeGroup.GET("/foo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPeriodeLockMiddleware_NoPeriodeID_AllowsThrough(t *testing.T) {
	db, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	r := gin.New()
	// Route WITHOUT :periode_id param.
	r.POST("/api/v1/system/ping", mw.Handler(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPeriodeLockMiddleware_SOFT_CLOSED_WithAllowlistedAction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close() //nolint:errcheck

	periodeID := "00000000-0000-0000-0000-000000000005"
	now := testNow()

	mock.ExpectQuery(`SELECT`).WithArgs(sqlmock.AnyArg(), "TUGURE").WillReturnRows(softClosedPeriodeRows(periodeID, now))

	repo := closeflow.NewRepo(db)
	cfg := closeflow.DefaultConfig()
	mw := closeflow.NewPeriodeLockMiddleware(repo, cfg)

	// F-06: The client X-Close-Workflow-Action header is rejected; the allowlist
	// action must be injected via server-side Gin context by a route-specific handler
	// or upstream middleware (not the client). Simulate that here with a pre-middleware.
	r := gin.New()
	periodeGroup := r.Group("/api/v1/transaksi/:periode_id")
	periodeGroup.Use(func(c *gin.Context) {
		// Simulates what an upstream route handler sets (e.g. jurnal retry handler).
		c.Set("close_workflow_action", "JURNAL_RETRY_GL_DELIVERY")
		c.Next()
	})
	periodeGroup.Use(mw.Handler())
	periodeGroup.POST("/foo", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transaksi/"+periodeID+"/foo", nil)
	// No X-Close-Workflow-Action header — that header is now rejected by F-06.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ─── Row helpers ──────────────────────────────────────────────────────────────

func testNow() time.Time {
	return time.Now()
}

func periodeRowCols() []string {
	return []string{
		"id", "periode_id_kode", "tahun_buku", "bulan", "tipe_periode",
		"tanggal_mulai", "tanggal_akhir", "status_periode",
		"tanggal_soft_close", "tanggal_hard_close",
		"reopened_flag", "reopened_reason", "reopened_at", "reopened_by", "reopened_approved_by",
		"row_version", "tenant_id", "created_at", "updated_at",
		"soft_close_requested_by", "soft_close_requested_at", "soft_close_request_reason",
		"soft_close_approved_by", "soft_close_approved_at", "soft_close_approve_reason",
		"hard_close_requested_by", "hard_close_requested_at", "hard_close_request_reason",
		"hard_close_approved_by", "hard_close_approved_at", "hard_close_approve_reason",
		"hard_close_grace_expires_at", "step_up_token_ref", "reopen_reason",
	}
}

func openPeriodeRows(id string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(periodeRowCols()).AddRow(
		id, "2026-06", 2026, nil, "BULANAN",
		now, now, "OPEN",
		nil, nil,
		false, nil, nil, nil, nil,
		int64(1), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
}

func softClosedPeriodeRows(id string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(periodeRowCols()).AddRow(
		id, "2026-05", 2026, nil, "BULANAN",
		now, now, "SOFT_CLOSED",
		now, nil,
		false, nil, nil, nil, nil,
		int64(2), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
}

func closedPeriodeRows(id string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows(periodeRowCols()).AddRow(
		id, "2026-04", 2026, nil, "BULANAN",
		now, now, "CLOSED",
		now, now,
		false, nil, nil, nil, nil,
		int64(3), "TUGURE", now, now,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil,
		nil, nil, nil,
	)
}
