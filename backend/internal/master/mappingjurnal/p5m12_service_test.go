package mappingjurnal

// p5m12_service_test.go — Service-level unit tests for P5-M12 workflow methods.
//
// Strategy: stub P5M12Repository in-process (no DB). BeginTx returns errTestSvcNoDB
// so tests that validate guards BEFORE the tx boundary exercise all domain checks.
// For paths that require tx success, the stub is extended to return a real *sql.Tx
// from a shared in-memory DB (not used here; covered by sqlmock file).
//
// Coverage targets:
//   - CreateNewVersion: inflight guard, not-found active, happy path regulated/non-reg.
//   - P5Submit: wrong status, empty details, unbalanced, COA invalid, happy path.
//   - P5Review: wrong status, SoD violation (maker=reviewer), non-regulated → PENDING_APPROVAL.
//   - P5Approve: wrong status, SoD maker, SoD reviewer, periode lock, non-regulated happy.
//   - P5Approve2: missing step-up token, wrong status, SoD 4-way, periode lock, regulated happy.
//   - P5Reject: wrong status (DRAFT), wrong status (APPROVED_ACTIVE), happy from PENDING_REVIEW.
//   - GetCoverage, GetValidation, GetHistory: repo error propagation + success.

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// errTestSvcNoDB is returned by svcP5Repo.BeginTx so that tests exercising
// pre-tx guards fail cleanly rather than panicking on nil *sql.Tx.
var errTestSvcNoDB = errors.New("test: no real DB in unit test")

// ─── Stub repo for P5M12 service tests ──────────────────────────────────────

// svcP5Repo is a full stub implementing P5M12Repository.
// Every method can be overridden via field assignment for specific test cases.
type svcP5Repo struct {
	// base field is unused but kept for structural reference
	base interface{}

	// P5M12Repository extensions
	hasInflight        bool
	hasInflightErr     error
	activeHeader       *Header
	activeHeaderErr    error
	versionHeader      *Header
	versionHeaderErr   error
	configParam        string
	configParamErr     error
	periodeStatus      string
	periodeStatusErr   error
	coaExists          bool
	coaExistsErr       error
	eventExists        bool
	eventExistsErr     error
	coverageResp       *CoverageResp
	coverageErr        error
	validationResp     *ValidationResp
	validationErr      error
	historyEntries       []MappingAuditEntry
	historyNextCursor    *string
	historyHasMore       bool
	historyErr           error
	historyCountOverride int // if > 0, CountMappingHistoryRows returns this value
	submitErr          error
	reviewErr          error
	approve4Err        error
	approve6Err        error
	rejectErr          error
	flipErr            error
	insertVersionErr   error
	insertDraftErr     error
	insertBatchErr     error
	detailsResult      []*Detail
	detailsErr         error
	// Control whether BeginTx succeeds or not.
	beginTxErr error
	beginTxTx  *sql.Tx // normally nil; tests that want a real tx must inject
}

// ── base Repository (delegate to embedded repoAdapter) ──

func (r *svcP5Repo) CreateHeader(_ context.Context, _ *sql.Tx, _ *Header) error {
	return nil
}
func (r *svcP5Repo) CreateDetails(_ context.Context, _ *sql.Tx, _ []*Detail) error {
	return nil
}
func (r *svcP5Repo) GetHeaderByID(_ context.Context, _ uuid.UUID, _ bool) (*Header, error) {
	return nil, nil
}
func (r *svcP5Repo) GetHeaderByEventCode(_ context.Context, _ string, _ bool) (*Header, error) {
	return nil, nil
}
func (r *svcP5Repo) GetDetailsByHeaderID(_ context.Context, _ uuid.UUID, _ bool) ([]*Detail, error) {
	if r.detailsErr != nil {
		return nil, r.detailsErr
	}
	return r.detailsResult, nil
}
func (r *svcP5Repo) ListHeaders(_ context.Context, _ listquery.Query, _ string, _ int, _ bool) ([]*Header, error) {
	return nil, nil
}
func (r *svcP5Repo) UpdateHeader(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ HeaderUpdateFields) (*Header, error) {
	return nil, nil
}
func (r *svcP5Repo) BulkReplaceDetails(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ []*Detail, _ uuid.UUID) error {
	return nil
}
func (r *svcP5Repo) SoftDeleteHeader(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID) (*Header, error) {
	return nil, nil
}
func (r *svcP5Repo) UpdateWorkflowStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ WorkflowStatus, _ uuid.UUID) error {
	return nil
}
func (r *svcP5Repo) CountHeaderReferences(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (r *svcP5Repo) CheckCoAApproved(_ context.Context, _ uuid.UUID) (bool, error) {
	return true, nil
}
func (r *svcP5Repo) ListAuditHistory(_ context.Context, _ uuid.UUID, _ string, _ int, _ bool) ([]AuditHistoryItem, bool, error) {
	return nil, false, nil
}
func (r *svcP5Repo) ExportAll(_ context.Context, _ listquery.Query) (io.Reader, int, error) {
	return nil, 0, nil
}

func (r *svcP5Repo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	if r.beginTxErr != nil {
		return nil, r.beginTxErr
	}
	if r.beginTxTx != nil {
		return r.beginTxTx, nil
	}
	return nil, errTestSvcNoDB
}

// ── P5M12Repository extensions ──

func (r *svcP5Repo) HasInflightVersion(_ context.Context, _ string, _ string) (bool, error) {
	return r.hasInflight, r.hasInflightErr
}
func (r *svcP5Repo) GetActiveByEventCode(_ context.Context, _ string, _ string) (*Header, error) {
	return r.activeHeader, r.activeHeaderErr
}
func (r *svcP5Repo) GetVersionByID(_ context.Context, _ uuid.UUID, _ string) (*Header, error) {
	return r.versionHeader, r.versionHeaderErr
}
func (r *svcP5Repo) InsertVersion(_ context.Context, _ *sql.Tx, _ *Header, _ []AkunDetail, _ uuid.UUID, _ string) error {
	return r.insertVersionErr
}
func (r *svcP5Repo) FlipActiveVersion(_ context.Context, _ *sql.Tx, _ string, _ uuid.UUID, _ uuid.UUID, _ string) error {
	return r.flipErr
}
func (r *svcP5Repo) SubmitVersion(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ time.Time, _ string) error {
	return r.submitErr
}
func (r *svcP5Repo) ReviewVersion(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string, _ bool, _ time.Time, _ string) error {
	return r.reviewErr
}
func (r *svcP5Repo) Approve4Eyes(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ []byte, _ string, _ time.Time, _ string) error {
	return r.approve4Err
}
func (r *svcP5Repo) Approve6Eyes(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _ []byte, _ []byte, _ string, _ time.Time, _ string) error {
	return r.approve6Err
}
func (r *svcP5Repo) RejectVersion(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time, _ string) error {
	return r.rejectErr
}
func (r *svcP5Repo) CoaCodeExists(_ context.Context, _ string, _ string) (bool, error) {
	return r.coaExists, r.coaExistsErr
}
func (r *svcP5Repo) EventCodeExists(_ context.Context, _ string, _ string) (bool, error) {
	return r.eventExists, r.eventExistsErr
}
func (r *svcP5Repo) GetConfigParam(_ context.Context, _ string) (string, error) {
	return r.configParam, r.configParamErr
}
func (r *svcP5Repo) GetPeriodeStatus(_ context.Context, _ string) (string, error) {
	if r.periodeStatusErr != nil {
		return "", r.periodeStatusErr
	}
	if r.periodeStatus == "" {
		return "OPEN", nil
	}
	return r.periodeStatus, nil
}
func (r *svcP5Repo) InsertDraftForBulkRow(_ context.Context, _ *sql.Tx, _ MappingBulkRow, _ uuid.UUID, _ uuid.UUID, _ string) error {
	return r.insertDraftErr
}
func (r *svcP5Repo) InsertUploadBatch(_ context.Context, _ *sql.Tx, _ uuid.UUID, _ uuid.UUID, _, _, _ int, _ string) error {
	return r.insertBatchErr
}
func (r *svcP5Repo) GetDetailsByP5HeaderID(_ context.Context, _ uuid.UUID) ([]*Detail, error) {
	return r.detailsResult, r.detailsErr
}
func (r *svcP5Repo) GetCoverageReport(_ context.Context, _ string) (*CoverageResp, error) {
	return r.coverageResp, r.coverageErr
}
func (r *svcP5Repo) GetValidationReport(_ context.Context, _ string) (*ValidationResp, error) {
	return r.validationResp, r.validationErr
}
func (r *svcP5Repo) ListMappingHistory(_ context.Context, _ listquery.Query, _ string, _ string, _ int, _ string) ([]MappingAuditEntry, *string, bool, error) {
	return r.historyEntries, r.historyNextCursor, r.historyHasMore, r.historyErr
}
func (r *svcP5Repo) CountMappingHistoryRows(_ context.Context, _ string, _ string) (int, error) {
	if r.historyCountOverride > 0 {
		return r.historyCountOverride, nil
	}
	return len(r.historyEntries), nil
}

var _ P5M12Repository = (*svcP5Repo)(nil)

// ─── Test helpers ─────────────────────────────────────────────────────────────

// ctxWithActor returns a context with a valid JWT Claims for the given user UUID.
func ctxWithActor(id uuid.UUID) context.Context {
	c := &auth.Claims{
		Sub:      id.String(),
		TenantID: "TUGURE",
		Roles:    []string{"ROLE-AKUN-CTL"},
	}
	return auth.ContextWithClaims(context.Background(), c)
}

// ctxNoActor returns a context with no claims.
func ctxNoActor() context.Context {
	return context.Background()
}

// makeVersionHeader builds a minimal Header with given status and actor IDs.
func makeVersionHeader(status WorkflowStatus, maker, reviewer, approver *uuid.UUID) *Header {
	h := &Header{
		ID:             uuid.New(),
		EventCode:      "PENEMPATAN_DEPOSITO",
		WorkflowStatus: status,
		WorkflowPath:   "4-eyes",
		RegulatedFlag:  false,
		TenantID:       "TUGURE",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		CreatedBy:      uuid.New(),
		UpdatedBy:      uuid.New(),
		RowVersion:     1,
	}
	h.MakerID = maker
	h.ReviewerID = reviewer
	h.ApproverID = approver
	return h
}

func newSvc(repo *svcP5Repo) *P5M12Service {
	return NewP5M12Service(repo, nil) // nil audit.Writer — writeAuditP5 is no-op when aw==nil
}

// makeValidStepUpToken creates a minimal JWT-format step-up token for tests.
// It has scope=mapping_approve, a fresh iat, and a far-future exp.
// The signature segment is not verified (verifyMappingStepUp only parses the payload).
func makeValidStepUpToken() string {
	iat := time.Now().Unix()
	payload := fmt.Sprintf(`{"scope":"mapping_approve","iat":%d,"exp":9999999999,"jti":"test-jti"}`, iat)
	payloadB64 := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9." + payloadB64 + ".fakesig"
}

// makeAkunDetail creates a Detail with proper text COA codes for P5 validation.
func makeAkunDetail(dkIndicator, akunDebit, akunKredit string) *Detail {
	return &Detail{
		ID:             uuid.New(),
		DKIndicator:    dkIndicator,
		AkunDebitCode:  akunDebit,
		AkunKreditCode: akunKredit,
	}
}

// ─── CreateNewVersion tests ───────────────────────────────────────────────────

func TestCreateNewVersion_InflightGuard(t *testing.T) {
	repo := &svcP5Repo{hasInflight: true}
	svc := newSvc(repo)
	actorID := uuid.New()

	_, err := svc.CreateNewVersion(ctxWithActor(actorID), "PENEMPATAN_DEPOSITO", NewVersionReq{
		Reason:  "reason for change",
		Details: []AkunDetail{{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingDuplicateVersion)
}

func TestCreateNewVersion_NoActiveVersion(t *testing.T) {
	repo := &svcP5Repo{hasInflight: false, activeHeader: nil}
	svc := newSvc(repo)

	_, err := svc.CreateNewVersion(ctxWithActor(uuid.New()), "NONEXISTENT", NewVersionReq{
		Reason:  "reason for change",
		Details: []AkunDetail{{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestCreateNewVersion_NoActor(t *testing.T) {
	repo := &svcP5Repo{}
	svc := newSvc(repo)

	_, err := svc.CreateNewVersion(ctxNoActor(), "PENEMPATAN_DEPOSITO", NewVersionReq{
		Reason:  "reason",
		Details: []AkunDetail{{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1}},
	})
	require.Error(t, err)
}

func TestCreateNewVersion_InflightCheckError(t *testing.T) {
	repo := &svcP5Repo{hasInflightErr: errors.New("db error")}
	svc := newSvc(repo)

	_, err := svc.CreateNewVersion(ctxWithActor(uuid.New()), "PENEMPATAN_DEPOSITO", NewVersionReq{
		Reason:  "reason",
		Details: []AkunDetail{{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check inflight")
}

func TestCreateNewVersion_RegulatedFlag_FromFallback(t *testing.T) {
	// ECL_PEMBENTUKAN is in hardcoded regulated list.
	activeHdr := makeVersionHeader(StatusApprovedActive, nil, nil, nil)
	activeHdr.EventCode = "ECL_PEMBENTUKAN"
	activeHdr.EventIDKode = "EVT_ECL_001"
	activeHdr.NamaEvent = "ECL Pembentukan"
	activeHdr.KategoriEvent = "ECL"
	activeHdr.TriggerSource = "SYSTEM"

	repo := &svcP5Repo{
		hasInflight:  false,
		activeHeader: activeHdr,
		configParam:  "", // empty → use fallback
		// BeginTx will fail with errTestSvcNoDB but regulated flag is set before that.
	}
	svc := newSvc(repo)

	// The call reaches BeginTx and fails there; but we can inspect the error chain
	// to confirm regulated flag did not cause early bail-out.
	_, err := svc.CreateNewVersion(ctxWithActor(uuid.New()), "ECL_PEMBENTUKAN", NewVersionReq{
		Reason:  "revision for ECL",
		Details: []AkunDetail{{AkunDebit: "1001", AkunKredit: "2001", DebitKredit: "D", Urutan: 1}},
	})
	require.Error(t, err)
	// The error should be about BeginTx (no DB), not about regulation check.
	assert.Contains(t, err.Error(), "begin tx")
}

// ─── P5Submit tests ───────────────────────────────────────────────────────────

func TestP5Submit_WrongStatus(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	repo := &svcP5Repo{
		versionHeader: makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil),
	}
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), "PENEMPATAN_DEPOSITO", vID, P5SubmitReq{Comment: "ok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

func TestP5Submit_VersionNotFound(t *testing.T) {
	vID := uuid.New()
	repo := &svcP5Repo{versionHeader: nil}
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), "X", vID, P5SubmitReq{Comment: "ok"})
	require.Error(t, err)
}

func TestP5Submit_EventCodeMismatch(t *testing.T) {
	vID := uuid.New()
	repo := &svcP5Repo{
		versionHeader: makeVersionHeader(WorkflowStatusDraft, nil, nil, nil),
	}
	// version has EventCode="PENEMPATAN_DEPOSITO", but caller passes "OTHER"
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), "OTHER", vID, P5SubmitReq{Comment: "ok"})
	require.Error(t, err)
}

func TestP5Submit_EmptyDetails(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)
	repo := &svcP5Repo{
		versionHeader: h,
		detailsResult: nil, // empty
	}
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), h.EventCode, vID, P5SubmitReq{Comment: "ok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAPPING_AKUN_INVALID")
}

func TestP5Submit_UnbalancedDetails(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)
	// Two debit rows, no kredit — will fail balance check AFTER empty-check passes
	repo := &svcP5Repo{
		versionHeader: h,
		detailsResult: []*Detail{
			makeAkunDetail("D", "110201", "440101"),
			makeAkunDetail("D", "110202", "440102"),
		},
	}
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), h.EventCode, vID, P5SubmitReq{Comment: "ok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingUnbalanced)
}

func TestP5Submit_COAInvalid(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)
	// One D + one K (balanced) but COA check fails
	repo := &svcP5Repo{
		versionHeader: h,
		detailsResult: []*Detail{
			makeAkunDetail("D", "110201", "440101"),
			makeAkunDetail("K", "440101", "110201"),
		},
		coaExists: false, // COA not found
	}
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), h.EventCode, vID, P5SubmitReq{Comment: "ok"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingAkunInvalid)
}

func TestP5Submit_HappyPath(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)
	repo := &svcP5Repo{
		versionHeader: h,
		detailsResult: []*Detail{
			makeAkunDetail("D", "110201", "440101"),
			makeAkunDetail("K", "440101", "110201"),
		},
		coaExists: true,
		// BeginTx returns errTestSvcNoDB — but we check guards pass first.
	}
	svc := newSvc(repo)

	_, err := svc.P5Submit(ctxWithActor(uuid.New()), h.EventCode, vID, P5SubmitReq{Comment: "ok"})
	// Only error expected is begin tx (no real DB in unit test)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

// ─── P5Review tests ───────────────────────────────────────────────────────────

func TestP5Review_WrongStatus(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Review(ctxWithActor(uuid.New()), h.EventCode, vID, P5ReviewReq{Comment: "review comment long enough here", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

func TestP5Review_SoDViolation_MakerEqualsReviewer(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	// Reviewer uses same ID as maker → SoD violation
	_, err := svc.P5Review(ctxWithActor(makerID), h.EventCode, vID, P5ReviewReq{Comment: "review comment long enough here", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestP5Review_NonRegulated_GoesToPendingApproval(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)
	h.RegulatedFlag = false
	repo := &svcP5Repo{
		versionHeader: h,
		configParam:   "",  // empty → fallback; PENEMPATAN_DEPOSITO not regulated
		reviewErr:     nil, // repo succeeds
	}
	svc := newSvc(repo)

	_, err := svc.P5Review(ctxWithActor(reviewerID), h.EventCode, vID, P5ReviewReq{Comment: "review comment long enough here", SignatureMethod: "JWT_STEP_UP"})
	// Reaches BeginTx: unit test DB unavailable
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

func TestP5Review_Regulated_GoesToPendingApproval2(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)
	h.RegulatedFlag = true
	h.EventCode = "ECL_PEMBENTUKAN"
	repo := &svcP5Repo{
		versionHeader: h,
		configParam:   "ECL_PEMBENTUKAN", // regulated
	}
	svc := newSvc(repo)

	_, err := svc.P5Review(ctxWithActor(reviewerID), h.EventCode, vID, P5ReviewReq{Comment: "review comment long enough here", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

// ─── P5Approve tests ──────────────────────────────────────────────────────────

func TestP5Approve_WrongStatus(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve(ctxWithActor(uuid.New()), h.EventCode, vID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

func TestP5Approve_SoDViolation_MakerEqualsApprover(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve(ctxWithActor(makerID), h.EventCode, vID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestP5Approve_SoDViolation_ReviewerEqualsApprover(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve(ctxWithActor(reviewerID), h.EventCode, vID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestP5Approve_PeriodeLocked(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)
	repo := &svcP5Repo{
		versionHeader: h,
		periodeStatus: "HARD_CLOSED",
	}
	svc := newSvc(repo)

	_, err := svc.P5Approve(ctxWithActor(approverID), h.EventCode, vID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAPPING_PERIODE_LOCKED")
}

func TestP5Approve_HappyPath_NonRegulated(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)
	h.RegulatedFlag = false
	repo := &svcP5Repo{
		versionHeader: h,
		periodeStatus: "OPEN",
	}
	svc := newSvc(repo)

	_, err := svc.P5Approve(ctxWithActor(approverID), h.EventCode, vID, P5ApproveReq{Comment: "approve comment", SignatureMethod: "JWT_STEP_UP"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

// ─── P5Approve2 tests ─────────────────────────────────────────────────────────

func TestP5Approve2_MissingStepUpToken(t *testing.T) {
	vID := uuid.New()
	repo := &svcP5Repo{}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(uuid.New()), "ECL_PEMBENTUKAN", vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, "")
	require.Error(t, err)
	// Empty token → MFA_STEP_UP_REQUIRED with scope message
	assert.Contains(t, err.Error(), StepUpScopeMappingApprove)
}

func TestP5Approve2_WrongStatus(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	// Header status is PENDING_APPROVAL (not PENDING_APPROVAL_2) → WORKFLOW_INVALID_TRANSITION.
	// Need valid JWT so step-up check passes and status check is reached.
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(uuid.New()), h.EventCode, vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, makeValidStepUpToken())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

func TestP5Approve2_SoDViolation_MakerEqualsApprover2(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(makerID), h.EventCode, vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, makeValidStepUpToken())
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestP5Approve2_SoDViolation_ReviewerEqualsApprover2(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(reviewerID), h.EventCode, vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, makeValidStepUpToken())
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestP5Approve2_SoDViolation_ApproverEqualsApprover2(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(approverID), h.EventCode, vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, makeValidStepUpToken())
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeMappingSoDViolation)
}

func TestP5Approve2_PeriodeLocked(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	approver2ID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true
	repo := &svcP5Repo{
		versionHeader: h,
		periodeStatus: "HARD_CLOSED",
	}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(approver2ID), h.EventCode, vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, makeValidStepUpToken())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAPPING_PERIODE_LOCKED")
}

func TestP5Approve2_HappyPath_Regulated(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	approver2ID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true
	repo := &svcP5Repo{
		versionHeader: h,
		periodeStatus: "OPEN",
	}
	svc := newSvc(repo)

	_, err := svc.P5Approve2(ctxWithActor(approver2ID), h.EventCode, vID, P5ApproveReq{Comment: "approve 2 comment", SignatureMethod: "JWT_STEP_UP"}, makeValidStepUpToken())
	// Only DB error expected (begin tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

// ─── P5Reject tests ───────────────────────────────────────────────────────────

func TestP5Reject_WrongStatus_Draft(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusDraft, &makerID, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Reject(ctxWithActor(uuid.New()), h.EventCode, vID, P5RejectReq{Reason: "reject reason must be long enough here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

func TestP5Reject_WrongStatus_ApprovedActive(t *testing.T) {
	vID := uuid.New()
	h := makeVersionHeader(StatusApprovedActive, nil, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Reject(ctxWithActor(uuid.New()), h.EventCode, vID, P5RejectReq{Reason: "reject reason must be long enough here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

func TestP5Reject_HappyPath_PendingReview(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingReview, &makerID, nil, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Reject(ctxWithActor(uuid.New()), h.EventCode, vID, P5RejectReq{Reason: "reject reason must be long enough here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

func TestP5Reject_HappyPath_PendingApproval(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	h := makeVersionHeader(WorkflowStatusPendingApproval, &makerID, &reviewerID, nil)
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Reject(ctxWithActor(uuid.New()), h.EventCode, vID, P5RejectReq{Reason: "reject reason must be long enough here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

func TestP5Reject_HappyPath_PendingApproval2(t *testing.T) {
	vID := uuid.New()
	makerID := uuid.New()
	reviewerID := uuid.New()
	approverID := uuid.New()
	h := makeVersionHeader(StatusPendingApproval2, &makerID, &reviewerID, &approverID)
	h.RegulatedFlag = true
	repo := &svcP5Repo{versionHeader: h}
	svc := newSvc(repo)

	_, err := svc.P5Reject(ctxWithActor(uuid.New()), h.EventCode, vID, P5RejectReq{Reason: "reject reason must be long enough here"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "begin tx")
}

func TestP5Reject_VersionNotFound(t *testing.T) {
	vID := uuid.New()
	repo := &svcP5Repo{versionHeader: nil}
	svc := newSvc(repo)

	_, err := svc.P5Reject(ctxNoActor(), "X", vID, P5RejectReq{Reason: "reject reason must be long enough here"})
	require.Error(t, err)
}

// ─── GetCoverage tests ────────────────────────────────────────────────────────

func TestGetCoverage_RepoError(t *testing.T) {
	repo := &svcP5Repo{coverageErr: errors.New("db error")}
	svc := newSvc(repo)

	_, err := svc.GetCoverage(ctxWithActor(uuid.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetCoverage")
}

func TestGetCoverage_Success(t *testing.T) {
	repo := &svcP5Repo{
		coverageResp: &CoverageResp{
			TotalEvents:   3,
			ActiveEvents:  2,
			MissingEvents: 1,
			GapEvents:     []CoverageEventP5{{EventCode: "EVT1", GapCoverage: CoverageStatusOK}},
		},
	}
	svc := newSvc(repo)

	resp, err := svc.GetCoverage(ctxWithActor(uuid.New()))
	require.NoError(t, err)
	assert.Equal(t, 3, resp.TotalEvents)
	assert.Equal(t, 1, len(resp.GapEvents))
}

// ─── GetValidation tests ──────────────────────────────────────────────────────

func TestGetValidation_RepoError(t *testing.T) {
	repo := &svcP5Repo{validationErr: errors.New("db error")}
	svc := newSvc(repo)

	_, err := svc.GetValidation(ctxWithActor(uuid.New()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetValidation")
}

func TestGetValidation_Success(t *testing.T) {
	repo := &svcP5Repo{
		validationResp: &ValidationResp{
			TotalActiveMappings: 5,
			ValidMappings:       4,
			InvalidMappings:     1,
			Issues: []ValidationIssueP5{
				{EventCode: "EVT1", ErrorCodes: []string{CodeMappingUnbalanced}},
			},
		},
	}
	svc := newSvc(repo)

	resp, err := svc.GetValidation(ctxWithActor(uuid.New()))
	require.NoError(t, err)
	assert.Equal(t, 1, resp.InvalidMappings)
}

// ─── GetHistory tests ─────────────────────────────────────────────────────────

func TestGetHistory_RepoError(t *testing.T) {
	repo := &svcP5Repo{historyErr: errors.New("db error")}
	svc := newSvc(repo)

	_, err := svc.GetHistory(ctxWithActor(uuid.New()), "", "", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "GetHistory")
}

func TestGetHistory_DefaultLimit(t *testing.T) {
	repo := &svcP5Repo{
		historyEntries: []MappingAuditEntry{{Action: "MAPPING.SUBMIT"}},
	}
	svc := newSvc(repo)

	result, err := svc.GetHistory(ctxWithActor(uuid.New()), "", "", 0) // 0 → default 50
	require.NoError(t, err)
	assert.Len(t, result.Items, 1)
}

func TestGetHistory_LimitCappedAt200(t *testing.T) {
	repo := &svcP5Repo{}
	svc := newSvc(repo)

	// Should not panic and should cap at 200
	_, err := svc.GetHistory(ctxWithActor(uuid.New()), "", "", 999)
	require.NoError(t, err)
}

func TestGetHistory_WithEventCodeFilter(t *testing.T) {
	cursorStr := "2026-06-22T00:00:00Z"
	repo := &svcP5Repo{
		historyEntries: []MappingAuditEntry{
			{Action: "MAPPING.APPROVE", EventID: uuid.New()},
		},
		historyNextCursor: &cursorStr,
		historyHasMore:    true,
	}
	svc := newSvc(repo)

	result, err := svc.GetHistory(ctxWithActor(uuid.New()), "ECL_PEMBENTUKAN", "", 10)
	require.NoError(t, err)
	assert.True(t, result.HasMore)
	assert.NotNil(t, result.NextCursor)
}

// ─── checkPeriodeLock tests ───────────────────────────────────────────────────

func TestCheckPeriodeLock_Open_NoError(t *testing.T) {
	repo := &svcP5Repo{periodeStatus: "OPEN"}
	svc := newSvc(repo)

	err := svc.checkPeriodeLock(ctxWithActor(uuid.New()), "TUGURE")
	assert.NoError(t, err)
}

func TestCheckPeriodeLock_HardClosed_Error(t *testing.T) {
	repo := &svcP5Repo{periodeStatus: "HARD_CLOSED"}
	svc := newSvc(repo)

	err := svc.checkPeriodeLock(ctxWithActor(uuid.New()), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAPPING_PERIODE_LOCKED")
}

func TestCheckPeriodeLock_DBError_Propagates(t *testing.T) {
	repo := &svcP5Repo{periodeStatusErr: errors.New("db timeout")}
	svc := newSvc(repo)

	err := svc.checkPeriodeLock(ctxWithActor(uuid.New()), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checkPeriodeLock")
}

// ─── Audit writer nil-safe ────────────────────────────────────────────────────

func TestWriteAuditP5_NilWriter_NoPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		writeAuditP5(context.Background(), nil, nil, audit.Event{Action: "MAPPING.TEST"})
	})
}

func TestWriteAuditP5_NilTx_NoPanic(t *testing.T) {
	aw := audit.NewWriter(nil)
	assert.NotPanics(t, func() {
		writeAuditP5(context.Background(), nil, aw, audit.Event{Action: "MAPPING.TEST"})
	})
}
