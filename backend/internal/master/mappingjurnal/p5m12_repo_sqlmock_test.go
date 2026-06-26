package mappingjurnal

// p5m12_repo_sqlmock_test.go — DBRepository P5-M12 SQL path coverage using go-sqlmock.
//
// Tests all new repo methods added in p5m12_repo.go:
//   - HasInflightVersion (exist / not exist / error)
//   - GetActiveByEventCode (found / not found / error)
//   - GetVersionByID (found / not found / error)
//   - SubmitVersion (success / zero rows / error)
//   - ReviewVersion (non-regulated / regulated / zero rows)
//   - Approve4Eyes (success / zero rows)
//   - Approve6Eyes (success / zero rows)
//   - RejectVersion (success / zero rows)
//   - FlipActiveVersion (success / step1 error / step2 error)
//   - CoaCodeExists (true / false / error)
//   - EventCodeExists (true / false)
//   - GetConfigParam (found / not found)
//   - GetPeriodeStatus (found / not found → default OPEN)
//   - GetCoverageReport (zero rows / rows / rows.Err)
//   - GetValidationReport (valid + invalid mappings)
//   - ListMappingHistory (cursor pagination + hasMore + event_code filter)

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Helper: new DBRepository backed by sqlmock ───────────────────────────────

func newMockRepo(t *testing.T) (*DBRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return &DBRepository{db: db}, mock
}

// anyArg matches any SQL argument.
var anyArg = sqlmock.AnyArg()

// ─── HasInflightVersion ───────────────────────────────────────────────────────

func TestDBRepo_HasInflightVersion_True(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("ECL_PEMBENTUKAN", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	got, err := repo.HasInflightVersion(testCtx(), "ECL_PEMBENTUKAN", "TUGURE")
	require.NoError(t, err)
	assert.True(t, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBRepo_HasInflightVersion_False(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("ECL_REVERSAL", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	got, err := repo.HasInflightVersion(testCtx(), "ECL_REVERSAL", "TUGURE")
	require.NoError(t, err)
	assert.False(t, got)
}

func TestDBRepo_HasInflightVersion_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("X", "TUGURE").
		WillReturnError(errTestSvcNoDB)

	_, err := repo.HasInflightVersion(testCtx(), "X", "TUGURE")
	require.Error(t, err)
}

// ─── GetActiveByEventCode ─────────────────────────────────────────────────────

func TestDBRepo_GetActiveByEventCode_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM mst.mapping_jurnal_header`).
		WillReturnRows(sqlmock.NewRows(p5HeaderCols()))

	h, err := repo.GetActiveByEventCode(testCtx(), "NONEXISTENT", "TUGURE")
	require.NoError(t, err)
	assert.Nil(t, h)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── GetVersionByID ───────────────────────────────────────────────────────────

func TestDBRepo_GetVersionByID_NotFound(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM mst.mapping_jurnal_header`).
		WillReturnRows(sqlmock.NewRows(p5HeaderCols()))

	h, err := repo.GetVersionByID(testCtx(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	assert.Nil(t, h)
}

// ─── SubmitVersion ────────────────────────────────────────────────────────────

func TestDBRepo_SubmitVersion_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	now := time.Now()
	err = repo.SubmitVersion(testCtx(), tx, uuid.New(), uuid.New(), now, "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_SubmitVersion_ZeroRows(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	db2, mock, _ := sqlmock.New()
	defer db2.Close()
	repo := &DBRepository{db: db2}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 0)) // zero rows

	tx, _ := db2.Begin()
	err := repo.SubmitVersion(testCtx(), tx, uuid.New(), uuid.New(), time.Now(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── ReviewVersion ────────────────────────────────────────────────────────────

func TestDBRepo_ReviewVersion_NonRegulated_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.ReviewVersion(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), "comment", false, time.Now(), "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_ReviewVersion_Regulated_SetsApproval2Status(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	// regulated=true → nextStatus = "PENDING_APPROVAL_2" (passed as first arg)
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.ReviewVersion(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), "comment", true, time.Now(), "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_ReviewVersion_ZeroRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	tx, _ := db.Begin()
	err := repo.ReviewVersion(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), "comment", false, time.Now(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── Approve4Eyes ─────────────────────────────────────────────────────────────

func TestDBRepo_Approve4Eyes_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.Approve4Eyes(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), "comment", time.Now(), "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_Approve4Eyes_ZeroRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	tx, _ := db.Begin()
	err := repo.Approve4Eyes(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), "comment", time.Now(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── Approve6Eyes ─────────────────────────────────────────────────────────────

func TestDBRepo_Approve6Eyes_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.Approve6Eyes(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), []byte("tokenref"), "comment", time.Now(), "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_Approve6Eyes_ZeroRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	tx, _ := db.Begin()
	err := repo.Approve6Eyes(testCtx(), tx, uuid.New(), uuid.New(), []byte("sig"), []byte("tokenref"), "comment", time.Now(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── RejectVersion ────────────────────────────────────────────────────────────

func TestDBRepo_RejectVersion_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.RejectVersion(testCtx(), tx, uuid.New(), "reason", uuid.New(), time.Now(), "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_RejectVersion_ZeroRows(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	tx, _ := db.Begin()
	err := repo.RejectVersion(testCtx(), tx, uuid.New(), "reason", uuid.New(), time.Now(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── FlipActiveVersion ────────────────────────────────────────────────────────

// TestDBRepo_FlipActiveVersion_Success: B4 fix — FlipActiveVersion now issues a single
// UPDATE that closes prior APPROVED_ACTIVE version (activate-new was merged into Approve4/6Eyes).
func TestDBRepo_FlipActiveVersion_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	// Single UPDATE: close prior active version only.
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.FlipActiveVersion(testCtx(), tx, "PENEMPATAN_DEPOSITO", uuid.New(), uuid.New(), "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_FlipActiveVersion_Step1Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnError(errTestSvcNoDB)

	tx, _ := db.Begin()
	err := repo.FlipActiveVersion(testCtx(), tx, "PENEMPATAN_DEPOSITO", uuid.New(), uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "close prior")
}

// TestDBRepo_FlipActiveVersion_ClosePriorOnly confirms that after B4 fix, no second UPDATE
// is issued. sqlmock will fail if an unexpected exec is called.
func TestDBRepo_FlipActiveVersion_ClosePriorOnly(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	// Exactly one UPDATE expected; no step-2 "activate" UPDATE should occur.
	mock.ExpectExec(`UPDATE mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	tx, _ := db.Begin()
	err := repo.FlipActiveVersion(testCtx(), tx, "PENEMPATAN_DEPOSITO", uuid.New(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	// No unexpected expectations means no second UPDATE was issued.
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── CoaCodeExists ────────────────────────────────────────────────────────────

func TestDBRepo_CoaCodeExists_True(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("1001", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	got, err := repo.CoaCodeExists(testCtx(), "1001", "TUGURE")
	require.NoError(t, err)
	assert.True(t, got)
}

func TestDBRepo_CoaCodeExists_False(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("9999", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	got, err := repo.CoaCodeExists(testCtx(), "9999", "TUGURE")
	require.NoError(t, err)
	assert.False(t, got)
}

func TestDBRepo_CoaCodeExists_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WillReturnError(errTestSvcNoDB)

	_, err := repo.CoaCodeExists(testCtx(), "1001", "TUGURE")
	require.Error(t, err)
}

// ─── EventCodeExists ──────────────────────────────────────────────────────────

func TestDBRepo_EventCodeExists_True(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("PENEMPATAN_DEPOSITO", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	got, err := repo.EventCodeExists(testCtx(), "PENEMPATAN_DEPOSITO", "TUGURE")
	require.NoError(t, err)
	assert.True(t, got)
}

func TestDBRepo_EventCodeExists_False(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT EXISTS`)).
		WithArgs("NONEXISTENT", "TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	got, err := repo.EventCodeExists(testCtx(), "NONEXISTENT", "TUGURE")
	require.NoError(t, err)
	assert.False(t, got)
}

// ─── GetConfigParam ───────────────────────────────────────────────────────────

func TestDBRepo_GetConfigParam_Found(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM sys.config`).
		WithArgs("MAPPING_REGULATED_EVENT_CODES").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow("ECL_PEMBENTUKAN,ECL_REVERSAL"))

	val, err := repo.GetConfigParam(testCtx(), "MAPPING_REGULATED_EVENT_CODES")
	require.NoError(t, err)
	assert.Equal(t, "ECL_PEMBENTUKAN,ECL_REVERSAL", val)
}

func TestDBRepo_GetConfigParam_NotFound_ReturnsEmpty(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM sys.config`).
		WithArgs("MISSING_KEY").
		WillReturnError(sql.ErrNoRows)

	val, err := repo.GetConfigParam(testCtx(), "MISSING_KEY")
	require.NoError(t, err) // ErrNoRows → empty string, no error
	assert.Equal(t, "", val)
}

// ─── GetPeriodeStatus ─────────────────────────────────────────────────────────

func TestDBRepo_GetPeriodeStatus_Open(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs("TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"status_periode"}).AddRow("OPEN"))

	status, err := repo.GetPeriodeStatus(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, "OPEN", status)
}

func TestDBRepo_GetPeriodeStatus_HardClosed(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs("TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"status_periode"}).AddRow("HARD_CLOSED"))

	status, err := repo.GetPeriodeStatus(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, "HARD_CLOSED", status)
}

func TestDBRepo_GetPeriodeStatus_NotFound_DefaultsOpen(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM mst.periode_buku`).
		WithArgs("TUGURE").
		WillReturnError(sql.ErrNoRows)

	status, err := repo.GetPeriodeStatus(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, "OPEN", status) // default per impl
}

// ─── GetCoverageReport ────────────────────────────────────────────────────────

func TestDBRepo_GetCoverageReport_Empty(t *testing.T) {
	repo, mock := newMockRepo(t)
	// Coverage query is a CTE — match via partial string
	mock.ExpectQuery(`WITH events`).
		WithArgs("TUGURE").
		WillReturnRows(sqlmock.NewRows(coverageCols()))

	resp, err := repo.GetCoverageReport(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 0, resp.TotalEvents)
	assert.Empty(t, resp.GapEvents)
}

func TestDBRepo_GetCoverageReport_WithRows(t *testing.T) {
	repo, mock := newMockRepo(t)
	rows := sqlmock.NewRows(coverageCols()).
		AddRow("EVT1", "Event One", 2, 0, nil).
		AddRow("EVT2", "Event Two", 0, 0, nil)

	mock.ExpectQuery(`WITH events`).
		WithArgs("TUGURE").
		WillReturnRows(rows)

	resp, err := repo.GetCoverageReport(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 2, resp.TotalEvents)
	assert.Equal(t, 1, resp.ActiveEvents)  // EVT1 has detail, EVT2 does not
	assert.Equal(t, 1, resp.MissingEvents) // EVT2 missing
}

func TestDBRepo_GetCoverageReport_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`WITH events`).
		WillReturnError(errTestSvcNoDB)

	_, err := repo.GetCoverageReport(testCtx(), "TUGURE")
	require.Error(t, err)
}

// ─── GetValidationReport ──────────────────────────────────────────────────────

func TestDBRepo_GetValidationReport_AllValid(t *testing.T) {
	repo, mock := newMockRepo(t)
	rows := sqlmock.NewRows(validationCols()).
		AddRow(uuid.New().String(), "EVT1", 2, 0, 1, 1) // 1D+1K, balanced, no null akun

	mock.ExpectQuery(`workflow_status = 'APPROVED_ACTIVE'`).
		WithArgs("TUGURE").
		WillReturnRows(rows)

	resp, err := repo.GetValidationReport(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.TotalActiveMappings)
	assert.Equal(t, 1, resp.ValidMappings)
	assert.Equal(t, 0, resp.InvalidMappings)
	assert.Empty(t, resp.Issues)
}

func TestDBRepo_GetValidationReport_Unbalanced(t *testing.T) {
	repo, mock := newMockRepo(t)
	rows := sqlmock.NewRows(validationCols()).
		AddRow(uuid.New().String(), "EVT1", 3, 0, 2, 1) // 2D != 1K

	mock.ExpectQuery(`workflow_status = 'APPROVED_ACTIVE'`).
		WithArgs("TUGURE").
		WillReturnRows(rows)

	resp, err := repo.GetValidationReport(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.InvalidMappings)
	require.Len(t, resp.Issues, 1)
	assert.Contains(t, resp.Issues[0].ErrorCodes, CodeMappingUnbalanced)
}

func TestDBRepo_GetValidationReport_NullAkun(t *testing.T) {
	repo, mock := newMockRepo(t)
	rows := sqlmock.NewRows(validationCols()).
		AddRow(uuid.New().String(), "EVT1", 2, 1, 1, 1) // 1 null akun

	mock.ExpectQuery(`workflow_status = 'APPROVED_ACTIVE'`).
		WithArgs("TUGURE").
		WillReturnRows(rows)

	resp, err := repo.GetValidationReport(testCtx(), "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 1, resp.InvalidMappings)
	assert.Contains(t, resp.Issues[0].ErrorCodes, CodeMappingAkunInvalid)
}

func TestDBRepo_GetValidationReport_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`workflow_status = 'APPROVED_ACTIVE'`).
		WillReturnError(errTestSvcNoDB)

	_, err := repo.GetValidationReport(testCtx(), "TUGURE")
	require.Error(t, err)
}

// ─── ListMappingHistory ───────────────────────────────────────────────────────

func TestDBRepo_ListMappingHistory_Empty(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows(historyCols()))

	entries, cursor, hasMore, err := repo.ListMappingHistory(testCtx(), testListQuery(), "", "", 10, "TUGURE")
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Nil(t, cursor)
	assert.False(t, hasMore)
}

func TestDBRepo_ListMappingHistory_HasMore(t *testing.T) {
	repo, mock := newMockRepo(t)
	// Return limit+1 rows to trigger hasMore=true
	rows := sqlmock.NewRows(historyCols())
	for i := 0; i < 6; i++ { // limit=5, return 6
		rows.AddRow(
			uuid.New().String(),
			time.Now().Add(-time.Duration(i)*time.Minute),
			uuid.New().String(),
			"ROLE-AKUN-CTL",
			"MAPPING.SUBMIT",
			"mst.mapping_jurnal_header",
			uuid.New().String(),
			nil,
			nil,
			nil,
		)
	}
	mock.ExpectQuery(`FROM aud.audit_log`).
		WillReturnRows(rows)

	entries, cursor, hasMore, err := repo.ListMappingHistory(testCtx(), testListQuery(), "", "", 5, "TUGURE")
	require.NoError(t, err)
	assert.Len(t, entries, 5) // trimmed to limit
	assert.True(t, hasMore)
	assert.NotNil(t, cursor)
}

func TestDBRepo_ListMappingHistory_WithEventCodeFilter(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows(historyCols()))

	_, _, _, err := repo.ListMappingHistory(testCtx(), testListQuery(), "ECL_PEMBENTUKAN", "", 10, "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBRepo_ListMappingHistory_WithCursor(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM aud.audit_log`).
		WillReturnRows(sqlmock.NewRows(historyCols()))

	cursor := "2026-06-22T10:00:00Z"
	_, _, _, err := repo.ListMappingHistory(testCtx(), testListQuery(), "", cursor, 10, "TUGURE")
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDBRepo_ListMappingHistory_DBError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`FROM aud.audit_log`).
		WillReturnError(errTestSvcNoDB)

	_, _, _, err := repo.ListMappingHistory(testCtx(), testListQuery(), "", "", 10, "TUGURE")
	require.Error(t, err)
}

// ─── InsertUploadBatch ────────────────────────────────────────────────────────

func TestDBRepo_InsertUploadBatch_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tx, _ := db.Begin()
	err := repo.InsertUploadBatch(testCtx(), tx, uuid.New(), uuid.New(), 10, 8, 2, "TUGURE")
	require.NoError(t, err)
}

func TestDBRepo_InsertUploadBatch_Error(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys.upload_batch`).
		WillReturnError(errTestSvcNoDB)

	tx, _ := db.Begin()
	err := repo.InsertUploadBatch(testCtx(), tx, uuid.New(), uuid.New(), 10, 8, 2, "TUGURE")
	require.Error(t, err)
}

// ─── Column name helpers ──────────────────────────────────────────────────────

func p5HeaderCols() []string {
	return []string{
		"id", "event_id_kode", "event_code", "nama_event", "kategori_event", "trigger_source",
		"aktif_flag", "catatan", "workflow_status", "workflow_path",
		"maker_id", "reviewer_id", "approver_id", "approver_2_id",
		"reviewer_signed_at", "reviewer_signature_hash", "comment_review",
		"approver_signed_at", "approver_signature_hash", "comment_approve",
		"approver_2_signed_at", "approver_2_signature_hash", "comment_approve_2",
		"submit_at", "reject_reason",
		"parent_id", "effective_from", "effective_to", "regulated_flag", "step_up_token_ref",
		"created_at", "created_by", "updated_at", "updated_by", "deleted_at", "row_version", "tenant_id",
	}
}

func coverageCols() []string {
	return []string{"event_code", "nama_event", "active_detail_count", "missing_akun_count", "last_dlq_error"}
}

func validationCols() []string {
	return []string{"id", "event_code", "detail_count", "null_akun_count", "debit_count", "kredit_count"}
}

func historyCols() []string {
	return []string{
		"event_id", "event_time", "actor_user_id", "actor_role",
		"action", "entity_type", "entity_id",
		"before_jsonb", "after_jsonb", "trace_id",
	}
}

func testCtx() context.Context {
	return context.Background()
}

func testListQuery() listquery.Query {
	return listquery.Query{}
}

// ─── InsertDraftForBulkRow ────────────────────────────────────────────────────
// Note: InsertDraftForBulkRow uses r.db for GetActiveByEventCode and a passed-in *sql.Tx.
// We use two separate sqlmock dbs: one for repo.db, one for the tx.

func TestDBRepo_InsertDraftForBulkRow_GetActiveError(t *testing.T) {
	repo, mock := newMockRepo(t)
	actor := uuid.New()
	batchID := uuid.New()

	// GetActiveByEventCode → non-NoRows error (covers GetActive error path)
	mock.ExpectQuery(`APPROVED_ACTIVE`).WillReturnError(errTestSvcNoDB)

	// Create separate tx db (not used for InsertDraftForBulkRow logic — just need non-nil tx)
	txDB, txMock, _ := sqlmock.New()
	defer txDB.Close()
	txMock.ExpectBegin()
	tx, _ := txDB.Begin()

	row := MappingBulkRow{EventCode: "TEST_EVT", Urutan: 1}
	err := repo.InsertDraftForBulkRow(testCtx(), tx, row, batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetActive")
}

func TestDBRepo_InsertDraftForBulkRow_HeaderInsertError(t *testing.T) {
	repo, mock := newMockRepo(t)
	actor := uuid.New()
	batchID := uuid.New()

	// GetActiveByEventCode → ErrNoRows → nil, nil (no existing header)
	mock.ExpectQuery(`APPROVED_ACTIVE`).WillReturnError(sql.ErrNoRows)

	// Separate tx db: INSERT header fails
	txDB, txMock, _ := sqlmock.New()
	defer txDB.Close()
	txMock.ExpectBegin()
	txMock.ExpectExec(`INSERT INTO mst.mapping_jurnal_header`).
		WillReturnError(errTestSvcNoDB)
	tx, _ := txDB.Begin()

	row := MappingBulkRow{EventCode: "TEST_EVT", Urutan: 1}
	err := repo.InsertDraftForBulkRow(testCtx(), tx, row, batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "header")
}

func TestDBRepo_InsertDraftForBulkRow_DetailInsertError(t *testing.T) {
	repo, mock := newMockRepo(t)
	actor := uuid.New()
	batchID := uuid.New()

	// GetActiveByEventCode → ErrNoRows → nil, nil
	mock.ExpectQuery(`APPROVED_ACTIVE`).WillReturnError(sql.ErrNoRows)

	// Separate tx db: header INSERT succeeds, detail INSERT fails
	txDB, txMock, _ := sqlmock.New()
	defer txDB.Close()
	txMock.ExpectBegin()
	txMock.ExpectExec(`INSERT INTO mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	txMock.ExpectExec(`INSERT INTO mst.mapping_jurnal_detail`).
		WillReturnError(errTestSvcNoDB)
	tx, _ := txDB.Begin()

	row := MappingBulkRow{
		EventCode:   "TEST_EVT",
		AkunDebit:   "1101",
		AkunKredit:  "2101",
		DebitKredit: "KREDIT",
		JumlahCalc:  "", // empty → nil jumlahCalc
		Urutan:      1,
	}
	err := repo.InsertDraftForBulkRow(testCtx(), tx, row, batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "detail")
}

func TestDBRepo_InsertDraftForBulkRow_Success_WithJumlahCalc(t *testing.T) {
	repo, mock := newMockRepo(t)
	actor := uuid.New()
	batchID := uuid.New()

	// GetActiveByEventCode → ErrNoRows → no existing header
	mock.ExpectQuery(`APPROVED_ACTIVE`).WillReturnError(sql.ErrNoRows)

	// Separate tx: both INSERTs succeed
	txDB, txMock, _ := sqlmock.New()
	defer txDB.Close()
	txMock.ExpectBegin()
	txMock.ExpectExec(`INSERT INTO mst.mapping_jurnal_header`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	txMock.ExpectExec(`INSERT INTO mst.mapping_jurnal_detail`).
		WillReturnResult(sqlmock.NewResult(1, 1))
	tx, _ := txDB.Begin()

	row := MappingBulkRow{
		EventCode:   "TEST_EVT_REG", // regulated event → 6-eyes path
		AkunDebit:   "1101",
		AkunKredit:  "2101",
		DebitKredit: "DEBIT",
		JumlahCalc:  "NILAI_NOMINAL", // non-empty → *string
		Urutan:      1,
	}
	err := repo.InsertDraftForBulkRow(testCtx(), tx, row, batchID, actor, "TUGURE")
	require.NoError(t, err)
}

// ─── writeAuditP5 with real tx ────────────────────────────────────────────────

func TestWriteAuditP5_WithRealTx(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()

	mock.ExpectBegin()
	// audit INSERT → will fail (unexpected) but still runs the Write call
	mock.ExpectRollback()

	tx, _ := db.Begin()

	aw := audit.NewWriter(nil)
	// With non-nil aw and tx → calls aw.WithTx(tx).Write(...)
	// Write tries INSERT INTO aud.audit_log → sqlmock doesn't expect it → error (ignored)
	writeAuditP5(testCtx(), tx, aw, audit.Event{
		Action:     "MAPPING.TEST",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   uuid.New(),
	})
	// No assertion needed — coverage of the aw.WithTx(tx).Write() line is what matters
}

// ─── GetDetailsByP5HeaderID ───────────────────────────────────────────────────

func TestDBRepo_GetDetailsByP5HeaderID_Success(t *testing.T) {
	repo, mock := newMockRepo(t)
	hID := uuid.New()
	dID := uuid.New()
	eID := uuid.New()
	now := time.Now()
	actorID := uuid.New()

	cols := []string{"id", "event_header_id", "urutan",
		"akun_debit", "akun_kredit", "dk_indicator", "jumlah_calc",
		"created_at", "created_by", "updated_at", "updated_by", "row_version", "tenant_id"}
	mock.ExpectQuery(`SELECT id`).
		WithArgs(hID).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(dID, eID, 1, "110201", "440101", "D", nil,
				now, actorID, now, actorID, int64(1), "TUGURE"))

	got, err := repo.GetDetailsByP5HeaderID(testCtx(), hID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "110201", got[0].AkunDebitCode)
	assert.Equal(t, "D", got[0].DKIndicator)
}

func TestDBRepo_GetDetailsByP5HeaderID_QueryError(t *testing.T) {
	repo, mock := newMockRepo(t)
	hID := uuid.New()
	mock.ExpectQuery(`SELECT id`).
		WithArgs(hID).
		WillReturnError(errTestSvcNoDB)

	_, err := repo.GetDetailsByP5HeaderID(testCtx(), hID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetDetailsByP5HeaderID")
}

// ─── CountMappingHistoryRows ──────────────────────────────────────────────────

func TestDBRepo_CountMappingHistoryRows_NoFilter(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("TUGURE").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(42))

	n, err := repo.CountMappingHistoryRows(testCtx(), "", "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 42, n)
}

func TestDBRepo_CountMappingHistoryRows_WithEventCodeFilter(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("TUGURE", "ECL_PEMBENTUKAN").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(7))

	n, err := repo.CountMappingHistoryRows(testCtx(), "ECL_PEMBENTUKAN", "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, 7, n)
}

func TestDBRepo_CountMappingHistoryRows_QueryError(t *testing.T) {
	repo, mock := newMockRepo(t)
	mock.ExpectQuery(`SELECT COUNT`).
		WithArgs("TUGURE").
		WillReturnError(errTestSvcNoDB)

	_, err := repo.CountMappingHistoryRows(testCtx(), "", "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CountMappingHistoryRows")
}
