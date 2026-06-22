package mappingjurnal

// p5m12_service_tx_test.go — Service tests with sqlmock-backed BeginTx so that the full
// tx flow (BeginTx → repo ops → Commit/Rollback) gets executed.
//
// Strategy: svcP5RepoTx embeds svcP5Repo (all repo methods are stubs) and overrides
// BeginTx to return a real *sql.Tx from sqlmock. The stub repo methods do not issue any
// real SQL, so sqlmock only needs ExpectBegin + ExpectCommit/Rollback.
//
// For rollback tests we set the stub's error field (submitErr, approve4Err, etc.) so
// the stub returns an error → service rolls back.
//
// Additional coverage:
//   - IsRegulated from config DB path
//   - ValidateBulkRow all branch paths
//   - scanP5Header full row scan via GetVersionByID sqlmock
//   - ListMappingHistory with non-nil before_jsonb / after_jsonb
//   - detailsToAkunDetail indicator normalisation (DEBIT/KREDIT/D/K/unknown)
//   - ImportBulk no-actor error + invalid XLSX error

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"mime/multipart"
	"net/textproto"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── svcP5RepoTx: svcP5Repo with real sqlmock BeginTx ────────────────────────

type svcP5RepoTx struct {
	svcP5Repo
	db   *sql.DB
	mock sqlmock.Sqlmock
}

func (r *svcP5RepoTx) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newTxRepo(t *testing.T) *svcP5RepoTx {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return &svcP5RepoTx{db: db, mock: mock}
}

func newSvcTx(repo *svcP5RepoTx) *P5M12Service {
	return NewP5M12Service(repo, nil)
}

var errStubFail = errors.New("stub: simulated repo failure")

// ─── CreateNewVersion full commit ─────────────────────────────────────────────

func TestCreateNewVersion_CommitPath(t *testing.T) {
	activeHdr := makeVersionHeader(StatusApprovedActive, nil, nil, nil)
	activeHdr.EventIDKode = "EVT_001"
	activeHdr.NamaEvent = "Test Event"
	activeHdr.KategoriEvent = "PENEMPATAN"
	activeHdr.TriggerSource = "SYSTEM"

	repo := newTxRepo(t)
	repo.hasInflight = false
	repo.activeHeader = activeHdr
	repo.configParam = "" // PENEMPATAN_DEPOSITO not regulated by fallback list

	// svcP5Repo stubs handle the real InsertVersion — no Exec hits sqlmock.
	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	result, err := svc.CreateNewVersion(ctx, activeHdr.EventCode, NewVersionReq{
		Reason: "reason for update",
		Details: []AkunDetail{
			{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, WorkflowStatusDraft, result.WorkflowStatus)
	assert.False(t, result.AktifFlag)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestCreateNewVersion_InsertError_Rollback(t *testing.T) {
	activeHdr := makeVersionHeader(StatusApprovedActive, nil, nil, nil)
	activeHdr.EventIDKode = "EVT_001"
	activeHdr.NamaEvent = "Test Event"
	activeHdr.KategoriEvent = "PENEMPATAN"
	activeHdr.TriggerSource = "SYSTEM"

	repo := newTxRepo(t)
	repo.hasInflight = false
	repo.activeHeader = activeHdr
	repo.insertVersionErr = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	_, err := svc.CreateNewVersion(ctx, activeHdr.EventCode, NewVersionReq{
		Reason: "reason",
		Details: []AkunDetail{
			{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1},
		},
	})
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

// ─── P5Submit full commit ─────────────────────────────────────────────────────

func TestP5Submit_CommitPath(t *testing.T) {
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.coaExists = true
	repo.detailsResult = []*Detail{
		{DKIndicator: "D", KodeAkunID: uuid.New()},
		{DKIndicator: "K", KodeAkunID: uuid.New()},
	}

	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	result, err := svc.P5Submit(ctx, h.EventCode, h.ID, P5SubmitReq{Comment: "submitting"})
	require.NoError(t, err)
	assert.Equal(t, WorkflowStatusPendingReview, result.WorkflowStatus)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Submit_SubmitVersionError_Rollback(t *testing.T) {
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.coaExists = true
	repo.detailsResult = []*Detail{
		{DKIndicator: "D", KodeAkunID: uuid.New()},
		{DKIndicator: "K", KodeAkunID: uuid.New()},
	}
	repo.submitErr = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	_, err := svc.P5Submit(ctx, h.EventCode, h.ID, P5SubmitReq{Comment: "submitting"})
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

// ─── P5Review full commit ─────────────────────────────────────────────────────

func TestP5Review_CommitPath_NonRegulated(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.configParam = "" // PENEMPATAN_DEPOSITO not in hardcoded regulated list

	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: reviewerID.String(), TenantID: "TUGURE"})

	result, err := svc.P5Review(ctx, h.EventCode, h.ID, P5ReviewReq{Comment: "review comment ok long enough", SignatureMethod: "JWT_STEP_UP"})
	require.NoError(t, err)
	assert.Equal(t, WorkflowStatusPendingApproval, result.WorkflowStatus)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Review_CommitPath_Regulated(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)
	h.EventCode = "ECL_PEMBENTUKAN"

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.configParam = "ECL_PEMBENTUKAN"

	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: reviewerID.String(), TenantID: "TUGURE"})

	result, err := svc.P5Review(ctx, h.EventCode, h.ID, P5ReviewReq{Comment: "review comment ok long enough", SignatureMethod: "JWT_STEP_UP"})
	require.NoError(t, err)
	assert.Equal(t, StatusPendingApproval2, result.WorkflowStatus)
	assert.True(t, result.RegulatedFlag)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Review_ReviewError_Rollback(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.reviewErr = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: reviewerID.String(), TenantID: "TUGURE"})

	_, err := svc.P5Review(ctx, h.EventCode, h.ID, P5ReviewReq{Comment: "review comment ok long enough", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

// ─── P5Approve full commit ────────────────────────────────────────────────────

func TestP5Approve_CommitPath(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.periodeStatus = "OPEN"

	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: approverID.String(), TenantID: "TUGURE"})

	result, err := svc.P5Approve(ctx, h.EventCode, h.ID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.NoError(t, err)
	assert.Equal(t, StatusApprovedActive, result.WorkflowStatus)
	assert.True(t, result.AktifFlag)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Approve_Approve4EyesError_Rollback(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.periodeStatus = "OPEN"
	repo.approve4Err = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: approverID.String(), TenantID: "TUGURE"})

	_, err := svc.P5Approve(ctx, h.EventCode, h.ID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Approve_FlipError_Rollback(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.periodeStatus = "OPEN"
	repo.flipErr = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: approverID.String(), TenantID: "TUGURE"})

	_, err := svc.P5Approve(ctx, h.EventCode, h.ID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

// ─── P5Approve2 full commit ───────────────────────────────────────────────────

func TestP5Approve2_CommitPath(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	approver2ID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.periodeStatus = "OPEN"

	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: approver2ID.String(), TenantID: "TUGURE"})

	result, err := svc.P5Approve2(ctx, h.EventCode, h.ID, P5ApproveReq{Comment: "approve-2 comment", SignatureMethod: "JWT_STEP_UP"}, "valid-step-up-token")
	require.NoError(t, err)
	assert.Equal(t, StatusApprovedActive, result.WorkflowStatus)
	assert.True(t, result.AktifFlag)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Approve2_Approve6EyesError_Rollback(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	approver2ID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.periodeStatus = "OPEN"
	repo.approve6Err = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: approver2ID.String(), TenantID: "TUGURE"})

	_, err := svc.P5Approve2(ctx, h.EventCode, h.ID, P5ApproveReq{Comment: "approve-2 comment", SignatureMethod: "JWT_STEP_UP"}, "valid-step-up-token")
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

// ─── P5Reject full commit ─────────────────────────────────────────────────────

func TestP5Reject_CommitPath(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h

	repo.mock.ExpectBegin()
	repo.mock.ExpectCommit()

	svc := newSvcTx(repo)
	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	result, err := svc.P5Reject(ctx, h.EventCode, h.ID, P5RejectReq{Reason: "reject reason must be at least thirty chars here"})
	require.NoError(t, err)
	assert.Equal(t, WorkflowStatusDraft, result.WorkflowStatus)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

func TestP5Reject_RejectError_Rollback(t *testing.T) {
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)

	repo := newTxRepo(t)
	repo.versionHeader = h
	repo.rejectErr = errStubFail

	repo.mock.ExpectBegin()
	repo.mock.ExpectRollback()

	svc := newSvcTx(repo)
	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	_, err := svc.P5Reject(ctx, h.EventCode, h.ID, P5RejectReq{Reason: "reject reason must be at least thirty chars here"})
	require.Error(t, err)
	assert.NoError(t, repo.mock.ExpectationsWereMet())
}

// ─── ImportBulk ───────────────────────────────────────────────────────────────

func TestImportBulk_NoActor(t *testing.T) {
	repo := newTxRepo(t)
	svc := newSvcTx(repo)

	content := []byte("fake content")
	fhdr := createFakeFileHeader("test.xlsx", content)

	_, err := svc.ImportBulk(context.Background(), fhdr) // no claims → actor error
	require.Error(t, err)
}

func TestImportBulk_InvalidXLSX(t *testing.T) {
	repo := newTxRepo(t)
	svc := newSvcTx(repo)

	actorID := uuid.New()
	ctx := auth.ContextWithClaims(context.Background(), &auth.Claims{Sub: actorID.String(), TenantID: "TUGURE"})

	content := []byte("not real xlsx content")
	fhdr := createFakeFileHeader("test.xlsx", content)

	_, err := svc.ImportBulk(ctx, fhdr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "XLSX")
}

// ─── IsRegulated from config path ─────────────────────────────────────────────

func TestIsRegulated_FromConfig_Positive(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectQuery(`FROM sys.config`).
		WithArgs("MAPPING_REGULATED_EVENT_CODES").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow("MY_CUSTOM_EVT,ECL_PEMBENTUKAN"))

	v := NewValidator(repo)
	result := v.IsRegulated(testCtx(), "MY_CUSTOM_EVT", "TUGURE")
	assert.True(t, result)
}

func TestIsRegulated_FromConfig_Negative(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectQuery(`FROM sys.config`).
		WithArgs("MAPPING_REGULATED_EVENT_CODES").
		WillReturnRows(sqlmock.NewRows([]string{"config_value"}).AddRow("ECL_PEMBENTUKAN"))

	v := NewValidator(repo)
	result := v.IsRegulated(testCtx(), "BIAYA_ADMIN", "TUGURE")
	assert.False(t, result)
}

func TestIsRegulated_ConfigError_FallsBackToHardcoded(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	mock.ExpectQuery(`FROM sys.config`).
		WithArgs("MAPPING_REGULATED_EVENT_CODES").
		WillReturnError(errStubFail)

	v := NewValidator(repo)
	// ECL_PEMBENTUKAN is in the hardcoded fallback list
	result := v.IsRegulated(testCtx(), "ECL_PEMBENTUKAN", "TUGURE")
	assert.True(t, result)
}

// ─── ValidateBulkRow — all branches ──────────────────────────────────────────

func TestValidateBulkRow_MissingEventCode(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	row := MappingBulkRow{RowNumber: 2, AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	assert.Equal(t, "event_code", errs[0].Col)
}

func TestValidateBulkRow_MissingAkunDebit(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	row := MappingBulkRow{RowNumber: 2, EventCode: "EVT1", AkunKredit: "2001", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	assert.Equal(t, "akun_debit", errs[0].Col)
}

func TestValidateBulkRow_MissingAkunKredit(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	row := MappingBulkRow{RowNumber: 2, EventCode: "EVT1", AkunDebit: "1001", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Col == "akun_kredit" {
			found = true
		}
	}
	assert.True(t, found, "expected akun_kredit error")
}

func TestValidateBulkRow_InvalidDebitKredit(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	// "X" is not D/K — stage-1 validation fails before any DB call
	row := MappingBulkRow{RowNumber: 2, EventCode: "EVT1", AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "X"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Col == "debit_kredit" {
			found = true
		}
	}
	assert.True(t, found, "expected debit_kredit error")
}

func TestValidateBulkRow_EventCodeNotFound(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	mock.ExpectQuery(`SELECT EXISTS`).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	row := MappingBulkRow{RowNumber: 2, EventCode: "NONEXISTENT", AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	assert.Equal(t, CodeMappingEventNotFound, errs[0].ErrorCode)
}

func TestValidateBulkRow_COADebitInvalid(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))  // event_code
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // akun_debit NOT in COA
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))  // akun_kredit in COA

	row := MappingBulkRow{RowNumber: 2, EventCode: "EVT1", AkunDebit: "BADACCT", AkunKredit: "2001", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	assert.Equal(t, CodeMappingAkunInvalid, errs[0].ErrorCode)
	assert.Equal(t, "akun_debit", errs[0].Col)
}

func TestValidateBulkRow_COAKreditInvalid(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true)) // event_code
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true)) // akun_debit valid
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false)) // akun_kredit NOT in COA

	row := MappingBulkRow{RowNumber: 2, EventCode: "EVT1", AkunDebit: "1001", AkunKredit: "BADKREDIT", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	require.NotEmpty(t, errs)
	found := false
	for _, e := range errs {
		if e.Col == "akun_kredit" && e.ErrorCode == CodeMappingAkunInvalid {
			found = true
		}
	}
	assert.True(t, found, "expected akun_kredit COA invalid error")
}

func TestValidateBulkRow_Valid(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	v := NewValidator(&DBRepository{db: db})

	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true)) // event_code
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true)) // akun_debit
	mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true)) // akun_kredit

	row := MappingBulkRow{RowNumber: 2, EventCode: "EVT1", AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D"}
	errs := v.ValidateBulkRow(testCtx(), row, "TUGURE")
	assert.Empty(t, errs)
}

// ─── ListMappingHistory with non-nil before/after jsonb ───────────────────────

func TestDBRepo_ListMappingHistory_WithBeforeAfterJSON(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	rows := sqlmock.NewRows(historyCols()).
		AddRow(
			uuid.New().String(),
			time.Now(),
			uuid.New().String(),
			"ROLE-AKUN-CTL",
			"MAPPING.APPROVE",
			"mst.mapping_jurnal_header",
			uuid.New().String(),
			`{"workflow_status":"PENDING_APPROVAL"}`,
			`{"workflow_status":"APPROVED_ACTIVE"}`,
			nil,
		)

	mock.ExpectQuery(`FROM aud.audit_log`).WillReturnRows(rows)

	entries, _, _, err := repo.ListMappingHistory(testCtx(), listquery.Query{}, "", "", 10, "TUGURE")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.NotNil(t, entries[0].BeforeJsonb)
	assert.NotNil(t, entries[0].AfterJsonb)
}

// ─── scanP5Header full row scan ───────────────────────────────────────────────

func TestDBRepo_GetVersionByID_FullRow(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	repo := &DBRepository{db: db}

	vID := uuid.New()
	makerID := uuid.New()
	now := time.Now()

	row := sqlmock.NewRows(p5HeaderCols()).AddRow(
		vID.String(),           // id
		"EVT_001",              // event_id_kode
		"PENEMPATAN_DEPOSITO",  // event_code
		"Penempatan",           // nama_event
		"PENEMPATAN",           // kategori_event
		"SYSTEM",               // trigger_source
		false,                  // aktif_flag
		"catatan tes",          // catatan
		"DRAFT",                // workflow_status
		"4-eyes",               // workflow_path
		makerID.String(),       // maker_id
		nil,                    // reviewer_id
		nil,                    // approver_id
		nil,                    // approver_2_id
		nil,                    // reviewer_signed_at
		nil,                    // reviewer_signature_hash
		nil,                    // comment_review
		nil,                    // approver_signed_at
		nil,                    // approver_signature_hash
		nil,                    // comment_approve
		nil,                    // approver_2_signed_at
		nil,                    // approver_2_signature_hash
		nil,                    // comment_approve_2
		now,                    // submit_at
		nil,                    // reject_reason
		nil,                    // parent_id
		now,                    // effective_from
		now,                    // effective_to
		false,                  // regulated_flag
		nil,                    // step_up_token_ref
		now,                    // created_at
		makerID.String(),       // created_by
		now,                    // updated_at
		makerID.String(),       // updated_by
		nil,                    // deleted_at
		int64(1),               // row_version
		"TUGURE",               // tenant_id
	)

	mock.ExpectQuery(`FROM mst.mapping_jurnal_header`).
		WithArgs(vID, "TUGURE").
		WillReturnRows(row)

	h, err := repo.GetVersionByID(testCtx(), vID, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, h)
	assert.Equal(t, vID, h.ID)
	assert.Equal(t, "PENEMPATAN_DEPOSITO", h.EventCode)
	assert.Equal(t, WorkflowStatusDraft, h.WorkflowStatus)
	require.NotNil(t, h.MakerID)
	assert.Equal(t, makerID, *h.MakerID)
}

// ─── detailsToAkunDetail indicator normalisation ──────────────────────────────

func TestDetailsToAkunDetail_AllIndicators(t *testing.T) {
	details := []*Detail{
		{DKIndicator: "DEBIT", KodeAkunID: uuid.New(), Urutan: 1},
		{DKIndicator: "KREDIT", KodeAkunID: uuid.New(), Urutan: 2},
		{DKIndicator: "D", KodeAkunID: uuid.New(), Urutan: 3},
		{DKIndicator: "K", KodeAkunID: uuid.New(), Urutan: 4},
		{DKIndicator: "X", KodeAkunID: uuid.New(), Urutan: 5},
	}
	result := detailsToAkunDetail(details)
	require.Len(t, result, 5)
	assert.Equal(t, "D", result[0].DebitKredit)
	assert.Equal(t, "K", result[1].DebitKredit)
	assert.Equal(t, "D", result[2].DebitKredit)
	assert.Equal(t, "K", result[3].DebitKredit)
	assert.Equal(t, "X", result[4].DebitKredit)
}

// ─── createFakeFileHeader ─────────────────────────────────────────────────────

func createFakeFileHeader(filename string, content []byte) *multipart.FileHeader {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	fw, _ := w.CreateFormFile("file", filename)
	fw.Write(content)
	w.Close()

	r := multipart.NewReader(body, w.Boundary())
	form, _ := r.ReadForm(1 << 20)
	if form != nil && len(form.File["file"]) > 0 {
		return form.File["file"][0]
	}
	mh := make(textproto.MIMEHeader)
	mh.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	return &multipart.FileHeader{Filename: filename, Header: mh, Size: int64(len(content))}
}
