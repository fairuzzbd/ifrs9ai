package reporting_test

// final_boost_test.go — final nudge tests to push coverage above 80%.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/reporting"
)

// ─── GetReportExport — empty roles (covers firstRole "" branch) ──────────────
// Claims with empty Roles[] reaches firstRole() → returns "".

func TestHandler_GetReportExport_EmptyRoles(t *testing.T) {
	primaryDB, primaryMock, err := sqlmock.New()
	require.NoError(t, err)
	defer primaryDB.Close()

	replicaDB, replicaMock, err := sqlmock.New()
	require.NoError(t, err)
	defer replicaDB.Close()

	// CountMVRows → replica COUNT(*)
	replicaMock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(int64(10)))

	// BeginTx + InsertExportLog + Commit (inline path, rowCount=10 ≤ inlineThreshold)
	primaryMock.ExpectBegin()
	primaryMock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	primaryMock.ExpectCommit()

	// queryMVRows (inline build) → replica
	replicaMock.ExpectQuery(`SELECT \* FROM rpt.mv_status_periode`).
		WillReturnRows(sqlmock.NewRows([]string{"colA"}).AddRow("v1"))

	repo := reporting.NewRepository(primaryDB, replicaDB)
	mvRepo := reporting.NewMVRepo(primaryDB, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug/export", func(c *gin.Context) {
		// Empty Roles → firstRole returns ""
		c.Set("claims", &auth.Claims{
			Sub:               uuid.New().String(),
			PreferredUsername: "testuser",
			Roles:             []string{}, // empty — covers firstRole "" return
			TenantID:          "TUGURE",
			Permissions:       []string{"audit_log.read"},
		})
		h.GetReportExport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/mv-status-periode/export?format=csv", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, primaryMock.ExpectationsWereMet())
	assert.NoError(t, replicaMock.ExpectationsWereMet())
}

// ─── GetReportExport — inline XLSX path ──────────────────────────────────────

func TestHandler_GetReportExport_InlineXLSX(t *testing.T) {
	primaryDB, primaryMock, err := sqlmock.New()
	require.NoError(t, err)
	defer primaryDB.Close()

	replicaDB, replicaMock, err := sqlmock.New()
	require.NoError(t, err)
	defer replicaDB.Close()

	replicaMock.ExpectQuery(`SELECT COUNT\(\*\) FROM rpt.mv_akrual_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"c"}).AddRow(int64(5)))

	primaryMock.ExpectBegin()
	primaryMock.ExpectExec(`INSERT INTO sys.export_log`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	primaryMock.ExpectCommit()

	replicaMock.ExpectQuery(`SELECT \* FROM rpt.mv_akrual_summary`).
		WillReturnRows(sqlmock.NewRows([]string{"col1", "col2"}).AddRow("a", "b"))

	repo := reporting.NewRepository(primaryDB, replicaDB)
	mvRepo := reporting.NewMVRepo(primaryDB, nil)
	svc := reporting.NewService(repo, mvRepo, nil, nil, nil, nil, nil, []byte("s"))

	h := reporting.NewHandler(svc)
	r := gin.New()
	r.GET("/reports/:slug/export", func(c *gin.Context) {
		c.Set("claims", &auth.Claims{
			Sub:               uuid.New().String(),
			PreferredUsername: "user2",
			Roles:             []string{"ROLE-AKUN"},
			TenantID:          "TUGURE",
			Permissions:       []string{"audit_log.read"},
		})
		h.GetReportExport(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/reports/mv-akrual-summary/export?format=xlsx", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.NoError(t, primaryMock.ExpectationsWereMet())
	assert.NoError(t, replicaMock.ExpectationsWereMet())
}

// ─── ListActiveScheduledEmails — error propagation ────────────────────────────

func TestRepo_ListActiveScheduledEmails_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`FROM sys.scheduled_email`).
		WillReturnError(assert.AnError)

	repo := reporting.NewRepository(db, nil)
	_, err = repo.ListActiveScheduledEmails(context.Background(), "TUGURE")
	assert.Error(t, err)
}

// ─── GetOptOuts — with rows ───────────────────────────────────────────────────

func TestRepo_GetOptOuts_WithRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	schedID := uuid.New()
	mock.ExpectQuery(`FROM sys.scheduled_email_optout`).
		WithArgs(schedID).
		WillReturnRows(sqlmock.NewRows([]string{"email"}).
			AddRow("user1@tugu-re.com").
			AddRow("user2@tugu-re.com"))

	repo := reporting.NewRepository(db, nil)
	emails, err := repo.GetOptOuts(context.Background(), schedID)
	require.NoError(t, err)
	assert.Len(t, emails, 2)
}

// ─── ChooseDB — reporting intent with replica ─────────────────────────────────

func TestChooseDB_ReplicaIntent(t *testing.T) {
	db1, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db1.Close()

	db2, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db2.Close()

	result := reporting.ChooseDB(db1, db2, reporting.ReadIntentReporting)
	assert.Equal(t, db2, result)
}

func TestChooseDB_PrimaryIntent(t *testing.T) {
	db1, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db1.Close()

	db2, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db2.Close()

	result := reporting.ChooseDB(db1, db2, reporting.ReadIntentPrimary)
	assert.Equal(t, db1, result)
}

func TestChooseDB_NilReplica(t *testing.T) {
	db1, _, err := sqlmock.New()
	require.NoError(t, err)
	defer db1.Close()

	// nil replica → fallback to primary with WARN
	result := reporting.ChooseDB(db1, nil, reporting.ReadIntentReporting)
	assert.Equal(t, db1, result)
}
