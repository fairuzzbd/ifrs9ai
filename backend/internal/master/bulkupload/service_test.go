package bulkupload

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Mock repository ──────────────────────────────────────────────────────────

type mockRepo struct {
	batchByID    map[uuid.UUID]*Batch
	rowsByBatch  map[uuid.UUID][]BatchRow
	configParams map[string]int
	periodeStatus string
	// Call tracking
	insertBatchCalled       bool
	updateBatchStatusCalled bool
	updateDryRunCalled      bool
	updateCommittedCalled   bool
	updateApprovedCalled    bool
	updateRollbackPendCalled bool
	updateRolledBackCalled  bool
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		batchByID:    make(map[uuid.UUID]*Batch),
		rowsByBatch:  make(map[uuid.UUID][]BatchRow),
		configParams: map[string]int{"BULK_FILE_MAX_MB": 50, "BULK_DRY_RUN_TTL_SECONDS": 3600, "BULK_ROLLBACK_GRACE_DAYS": 7},
		periodeStatus: "OPEN",
	}
}

func (m *mockRepo) InsertBatch(_ context.Context, _ *sql.Tx, b *Batch) error {
	m.insertBatchCalled = true
	m.batchByID[b.ID] = b
	return nil
}

func (m *mockRepo) GetBatch(_ context.Context, id uuid.UUID, _ string) (*Batch, error) {
	b, ok := m.batchByID[id]
	if !ok {
		return nil, nil
	}
	return b, nil
}

func (m *mockRepo) UpdateBatchStatus(_ context.Context, _ *sql.Tx, id uuid.UUID, status BatchStatus, _ uuid.UUID) error {
	m.updateBatchStatusCalled = true
	if b, ok := m.batchByID[id]; ok {
		b.Status = status
	}
	return nil
}

func (m *mockRepo) UpdateBatchDryRun(_ context.Context, _ *sql.Tx, id uuid.UUID, result *DryRunResult, expiresAt time.Time, _ uuid.UUID) error {
	m.updateDryRunCalled = true
	if b, ok := m.batchByID[id]; ok {
		b.Status = result.Status
		b.DryRunExpiresAt = &expiresAt
	}
	return nil
}

func (m *mockRepo) UpdateBatchCommitted(_ context.Context, _ *sql.Tx, id uuid.UUID, committed, failed, _ int, _ uuid.UUID) error {
	m.updateCommittedCalled = true
	if b, ok := m.batchByID[id]; ok {
		b.CommittedRows = committed
		b.FailedRows = failed
		if failed > 0 {
			b.Status = StatusPartialCommit
		} else {
			b.Status = StatusCommitted
		}
	}
	return nil
}

func (m *mockRepo) UpdateBatchApproved(_ context.Context, _ *sql.Tx, id uuid.UUID, _ uuid.UUID, count int) error {
	m.updateApprovedCalled = true
	if b, ok := m.batchByID[id]; ok {
		b.Status = StatusApproved
	}
	return nil
}

func (m *mockRepo) UpdateBatchRollbackPending(_ context.Context, _ *sql.Tx, id uuid.UUID, _ string, _ uuid.UUID) error {
	m.updateRollbackPendCalled = true
	if b, ok := m.batchByID[id]; ok {
		b.Status = StatusRollbackPending
	}
	return nil
}

func (m *mockRepo) UpdateBatchRolledBack(_ context.Context, _ *sql.Tx, id uuid.UUID, _ uuid.UUID, _ int) error {
	m.updateRolledBackCalled = true
	if b, ok := m.batchByID[id]; ok {
		b.Status = StatusRolledBack
	}
	return nil
}

func (m *mockRepo) InsertBatchRows(_ context.Context, _ *sql.Tx, rows []BatchRow) error {
	for _, r := range rows {
		m.rowsByBatch[r.BatchID] = append(m.rowsByBatch[r.BatchID], r)
	}
	return nil
}

func (m *mockRepo) ListBatchRows(_ context.Context, batchID uuid.UUID, _ listquery.Query, _ string) ([]BatchRow, Pagination, error) {
	rows := m.rowsByBatch[batchID]
	return rows, Pagination{HasMore: false, Limit: 50}, nil
}

func (m *mockRepo) GetBatchRowsByStatus(_ context.Context, batchID uuid.UUID, status RowStatus) ([]BatchRow, error) {
	var out []BatchRow
	for _, r := range m.rowsByBatch[batchID] {
		if r.RowStatus == status {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *mockRepo) UpdateRowStatus(_ context.Context, _ *sql.Tx, id uuid.UUID, status RowStatus, instrumenID *uuid.UUID, _ *json.RawMessage) error {
	for batchID, rows := range m.rowsByBatch {
		for i, r := range rows {
			if r.ID == id {
				m.rowsByBatch[batchID][i].RowStatus = status
				if instrumenID != nil {
					m.rowsByBatch[batchID][i].BulkInstrumenID = instrumenID
				}
			}
		}
	}
	return nil
}

func (m *mockRepo) UpdateRowsRolledBack(_ context.Context, _ *sql.Tx, batchID uuid.UUID) (int, error) {
	count := 0
	for i := range m.rowsByBatch[batchID] {
		if m.rowsByBatch[batchID][i].RowStatus == RowStatusCommitted {
			m.rowsByBatch[batchID][i].RowStatus = RowStatusRolledBack
			count++
		}
	}
	return count, nil
}

func (m *mockRepo) InsertInstrumen(_ context.Context, _ *sql.Tx, _ RowValidationResult, _ uuid.UUID, _ uuid.UUID, _ string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (m *mockRepo) ActivateInstrumenByBatch(_ context.Context, _ *sql.Tx, _ uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockRepo) SoftDeleteInstrumenByBatch(_ context.Context, _ *sql.Tx, batchID uuid.UUID, _ uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockRepo) CountPendingManualByBatch(_ context.Context, _ uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockRepo) GetConfigParamInt(_ context.Context, key string, defaultVal int) (int, error) {
	if v, ok := m.configParams[key]; ok {
		return v, nil
	}
	return defaultVal, nil
}

func (m *mockRepo) GetActivePeriodeStatus(_ context.Context, _ string) (string, error) {
	return m.periodeStatus, nil
}

func (m *mockRepo) CounterpartyExists(id, _ string) (bool, error) { return true, nil }
func (m *mockRepo) BankExists(id, _ string) (bool, error)          { return true, nil }
func (m *mockRepo) MataUangExists(kode, _ string) (bool, error)    { return true, nil }
func (m *mockRepo) InstrumenKodeExists(kode, _ string) (bool, error) {
	return false, nil // no conflict by default
}


// ─── Mock DBTxBeginner that returns a no-op TX ───────────────────────────────

// noopTx implements *sql.Tx-compatible interface for testing.
// We can't create real *sql.Tx without a DB, so we use a real sql.DB with sqlite-like stub.
// Instead, we skip transaction tests and test service logic paths via integration mocking.

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestXLSXBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	f.NewSheet("Deposito")
	f.SetCellValue("Deposito", "A1", "kode")
	f.SetCellValue("Deposito", "B1", "counterparty_id")
	f.SetCellValue("Deposito", "C1", "bank_id")
	f.SetCellValue("Deposito", "D1", "mata_uang")
	f.SetCellValue("Deposito", "E1", "saldo")
	f.SetCellValue("Deposito", "F1", "tanggal_penempatan")
	f.SetCellValue("Deposito", "G1", "jatuh_tempo")
	f.SetCellValue("Deposito", "H1", "bunga")
	f.SetCellValue("Deposito", "A2", "DEP-001")
	f.SetCellValue("Deposito", "B2", "CP-001")
	f.SetCellValue("Deposito", "C2", "BCA")
	f.SetCellValue("Deposito", "D2", "IDR")
	f.SetCellValue("Deposito", "E2", "1000000000")
	f.SetCellValue("Deposito", "F2", "2026-01-01")
	f.SetCellValue("Deposito", "G2", "2027-01-01")
	f.SetCellValue("Deposito", "H2", "0.065")
	f.DeleteSheet("Sheet1")

	buf := new(bytes.Buffer)
	require.NoError(t, f.Write(buf))
	return buf.Bytes()
}

// ─── UploadBatch unit tests ───────────────────────────────────────────────────

func TestService_UploadBatch_FileSizeTooLarge(t *testing.T) {
	repo := newMockRepo()
	repo.configParams["BULK_FILE_MAX_MB"] = 1 // 1MB limit
	svc := NewService(repo, nil, nil, nil)

	// 2MB data
	fileData := make([]byte, 2*1024*1024+1)
	fileData[0] = 'P'
	fileData[1] = 'K'
	fileData[2] = 0x03
	fileData[3] = 0x04

	_, err := svc.UploadBatch(context.Background(), "test.xlsx", fileData, uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkFileTooLarge)
}

func TestService_UploadBatch_InvalidMIME(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil, nil)

	// Not XLSX — wrong magic bytes
	fileData := []byte("this is not xlsx content at all and much more text")
	_, err := svc.UploadBatch(context.Background(), "test.xlsx", fileData, uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkMimeInvalid)
}

func TestService_UploadBatch_PeriodeLocked(t *testing.T) {
	repo := newMockRepo()
	repo.periodeStatus = "HARD_CLOSED"
	svc := NewService(repo, nil, nil, nil)

	// Valid XLSX magic + content
	fileData := newTestXLSXBytes(t)
	_, err := svc.UploadBatch(context.Background(), "test.xlsx", fileData, uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkPeriodeLocked)
}

func TestService_UploadBatch_TxBeginNotWired(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil, nil)
	// NewService stubs txBegin → returns error

	fileData := newTestXLSXBytes(t)
	_, err := svc.UploadBatch(context.Background(), "test.xlsx", fileData, uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "txBegin")
}

// ─── DryRun unit tests ────────────────────────────────────────────────────────

func TestService_DryRun_BatchNotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.DryRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestService_DryRun_WrongStatus(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusCommitting, // wrong status
		UploadedBy: actor,
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.DryRun(context.Background(), batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PARSED")
}

func TestService_DryRun_WrongActor(t *testing.T) {
	repo := newMockRepo()
	maker := uuid.New()
	other := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusParsed,
		UploadedBy: maker,
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.DryRun(context.Background(), batchID, other, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FORBIDDEN")
}

// ─── Commit unit tests ────────────────────────────────────────────────────────

func TestService_Commit_DryRunExpired(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	expired := time.Now().UTC().Add(-2 * time.Hour)
	repo.batchByID[batchID] = &Batch{
		ID:              batchID,
		Status:          StatusDryRunPassed,
		UploadedBy:      actor,
		DryRunExpiresAt: &expired, // expired 2 hours ago
		TenantID:        "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Commit(context.Background(), batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkDryRunExpired)
}

func TestService_Commit_DryRunFailed(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusDryRunFailed,
		UploadedBy: actor,
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Commit(context.Background(), batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkDryRunFailed)
}

func TestService_Commit_PeriodeLocked(t *testing.T) {
	repo := newMockRepo()
	repo.periodeStatus = "CLOSED"
	actor := uuid.New()
	batchID := uuid.New()
	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	repo.batchByID[batchID] = &Batch{
		ID:              batchID,
		Status:          StatusDryRunPassed,
		UploadedBy:      actor,
		DryRunExpiresAt: &expiresAt,
		TenantID:        "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Commit(context.Background(), batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkPeriodeLocked)
}

func TestService_Commit_NotOwner(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	other := uuid.New()
	batchID := uuid.New()
	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	repo.batchByID[batchID] = &Batch{
		ID:              batchID,
		Status:          StatusDryRunPassed,
		UploadedBy:      other, // different user
		DryRunExpiresAt: &expiresAt,
		TenantID:        "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Commit(context.Background(), batchID, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FORBIDDEN")
}

// ─── Approve unit tests ───────────────────────────────────────────────────────

func TestService_Approve_InvalidSignatureMethod(t *testing.T) {
	repo := newMockRepo()
	batchID := uuid.New()
	actor := uuid.New()
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Approve(context.Background(), batchID, ApproveRequest{
		Comment:        "Approved as per committee",
		SignatureMethod: "PASSWORD", // invalid
	}, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signatureMethod")
}

func TestService_Approve_SoDViolation(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New() // same actor as maker
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusCommitted,
		UploadedBy: actor, // actor = maker
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Approve(context.Background(), batchID, ApproveRequest{
		Comment:        "Trying to approve own batch",
		SignatureMethod: "JWT_STEP_UP",
	}, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkApproveSoDViolation)
}

func TestService_Approve_WrongStatus(t *testing.T) {
	repo := newMockRepo()
	maker := uuid.New()
	approver := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusParsed, // wrong status
		UploadedBy: maker,
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.Approve(context.Background(), batchID, ApproveRequest{
		Comment:        "Approved as per policy",
		SignatureMethod: "JWT_STEP_UP",
	}, approver, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── RollbackRequest unit tests ───────────────────────────────────────────────

func TestService_RollbackRequest_GraceWindowExpired(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	expired := time.Now().UTC().Add(-24 * time.Hour) // already expired
	repo.batchByID[batchID] = &Batch{
		ID:                     batchID,
		Status:                 StatusApproved,
		UploadedBy:             uuid.New(),
		RollbackGraceExpiresAt: &expired,
		TenantID:               "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	err := svc.RollbackRequest(context.Background(), batchID, RollbackRequestBody{
		Reason: "Alasan rollback yang cukup panjang untuk memenuhi syarat minimal 50 karakter disini",
	}, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkRollbackGraceExpired)
}

func TestService_RollbackRequest_WrongStatus(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusCommitted, // must be APPROVED
		UploadedBy: uuid.New(),
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	err := svc.RollbackRequest(context.Background(), batchID, RollbackRequestBody{
		Reason: "Alasan rollback yang cukup panjang untuk memenuhi syarat minimal 50 karakter disini",
	}, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WORKFLOW_INVALID_TRANSITION")
}

// ─── RollbackApprove unit tests ───────────────────────────────────────────────

func TestService_RollbackApprove_InvalidSignatureMethod(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.RollbackApprove(context.Background(), uuid.New(), RollbackApproveBody{
		Comment:        "CFO approves rollback",
		SignatureMethod: "TOTP", // invalid
	}, uuid.New(), "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signatureMethod")
}

func TestService_RollbackApprove_WrongStatus(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusApproved, // must be ROLLBACK_PENDING
		UploadedBy: uuid.New(),
		TenantID:   "TUGURE",
	}
	svc := NewService(repo, nil, nil, nil)

	_, err := svc.RollbackApprove(context.Background(), batchID, RollbackApproveBody{
		Comment:        "CFO approves rollback",
		SignatureMethod: "JWT_STEP_UP",
	}, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ROLLBACK_PENDING")
}

// ─── Domain method tests ──────────────────────────────────────────────────────

func TestBatch_IsDryRunPassedAndValid(t *testing.T) {
	future := time.Now().UTC().Add(1 * time.Hour)
	past := time.Now().UTC().Add(-1 * time.Hour)

	b := &Batch{Status: StatusDryRunPassed, DryRunExpiresAt: &future}
	assert.True(t, b.IsDryRunPassedAndValid(time.Now().UTC()))

	b2 := &Batch{Status: StatusDryRunPassed, DryRunExpiresAt: &past}
	assert.False(t, b2.IsDryRunPassedAndValid(time.Now().UTC()))

	b3 := &Batch{Status: StatusDryRunFailed, DryRunExpiresAt: &future}
	assert.False(t, b3.IsDryRunPassedAndValid(time.Now().UTC()))

	b4 := &Batch{Status: StatusDryRunPassed, DryRunExpiresAt: nil}
	assert.False(t, b4.IsDryRunPassedAndValid(time.Now().UTC()))
}

func TestBatch_IsInGraceWindow(t *testing.T) {
	future := time.Now().UTC().Add(24 * time.Hour)
	past := time.Now().UTC().Add(-24 * time.Hour)

	b := &Batch{RollbackGraceExpiresAt: &future}
	assert.True(t, b.IsInGraceWindow(time.Now().UTC()))

	b2 := &Batch{RollbackGraceExpiresAt: &past}
	assert.False(t, b2.IsInGraceWindow(time.Now().UTC()))

	b3 := &Batch{RollbackGraceExpiresAt: nil}
	assert.False(t, b3.IsInGraceWindow(time.Now().UTC()))
}

// ─── GetBatch / ListBatchRows passthrough ─────────────────────────────────────

func TestService_GetBatch_NotFound(t *testing.T) {
	repo := newMockRepo()
	svc := NewService(repo, nil, nil, nil)

	result, err := svc.GetBatch(context.Background(), uuid.New(), "TUGURE")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestService_GetBatch_Found(t *testing.T) {
	repo := newMockRepo()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{ID: batchID, Status: StatusParsed, TenantID: "TUGURE"}
	svc := NewService(repo, nil, nil, nil)

	result, err := svc.GetBatch(context.Background(), batchID, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, batchID, result.ID)
}

// ─── sha256Hex stub ───────────────────────────────────────────────────────────

func TestSHA256Hex_Stub(t *testing.T) {
	// Just ensures it doesn't panic
	result := sha256Hex([]byte("test data"))
	assert.NotEmpty(t, result)
}

// ─── min helper ──────────────────────────────────────────────────────────────

func TestMin(t *testing.T) {
	assert.Equal(t, 1, min(1, 5))
	assert.Equal(t, 1, min(5, 1))
	assert.Equal(t, 0, min(0, 0))
}

// ─── error wrapping compatibility ────────────────────────────────────────────

func TestService_BatchNotFound_ReturnsNilErr(t *testing.T) {
	// GetBatch returns (nil, nil) when not found — service handles this
	repo := newMockRepo()
	svc := NewService(repo, nil, nil, nil)

	// DryRun should get nil batch and return error
	_, err := svc.DryRun(context.Background(), uuid.New(), uuid.New(), "TUGURE")
	require.Error(t, err)
}

// ─── txBegin helpers ─────────────────────────────────────────────────────────

// mockTxDB creates a sqlmock DB + expected Begin/Exec/Commit for a single-statement happy path.
func newMockTxDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, mock
}

// wireTxBegin replaces svc.txBegin with a function that starts a TX from db (sqlmock).
func wireTxBegin(svc *Service, db *sql.DB) {
	svc.txBegin = func(ctx context.Context) (*sql.Tx, error) {
		return db.BeginTx(ctx, nil)
	}
}

// ─── Service DryRun happy path ────────────────────────────────────────────────

func TestService_DryRun_HappyPath(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	rowID := uuid.New()

	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusParsed,
		UploadedBy: actor,
		TenantID:   "TUGURE",
	}
	repo.rowsByBatch[batchID] = []BatchRow{
		{
			ID:          rowID,
			BatchID:     batchID,
			SheetName:   string(SheetDeposito),
			RowNumber:   2,
			RowStatus:   RowStatusPending,
			RowDataJson: []byte(`{"kode":"DEP-001","mata_uang":"IDR","counterparty_id":"CP-1","bank_id":"BCA","saldo":"1000000","tanggal_penempatan":"2026-01-01","jatuh_tempo":"2027-01-01","bunga":"0.065"}`),
		},
	}

	// Repo mock's methods don't use the real *sql.Tx, so only Begin+Commit needed
	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	result, err := svc.DryRun(context.Background(), batchID, actor, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.ExpiresAt.IsZero())
}

// ─── Service Commit happy path ────────────────────────────────────────────────

func TestService_Commit_HappyPath(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	future := time.Now().UTC().Add(1 * time.Hour)

	repo.batchByID[batchID] = &Batch{
		ID:              batchID,
		Status:          StatusDryRunPassed,
		UploadedBy:      actor,
		DryRunExpiresAt: &future,
		TenantID:        "TUGURE",
	}

	// Repo mock's UpdateBatchStatus doesn't use the real *sql.Tx, so only Begin+Commit needed
	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	jobID, err := svc.Commit(context.Background(), batchID, actor, "TUGURE")
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, jobID)
}

// ─── Service Approve happy path ───────────────────────────────────────────────

func TestService_Approve_HappyPath(t *testing.T) {
	repo := newMockRepo()
	maker := uuid.New()
	approver := uuid.New()
	batchID := uuid.New()

	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusCommitted,
		UploadedBy: maker,
		TenantID:   "TUGURE",
	}

	// Repo mock's methods don't use the real *sql.Tx, so only Begin+Commit needed
	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	result, err := svc.Approve(context.Background(), batchID, ApproveRequest{
		Comment:        "Disetujui sesuai kebijakan",
		SignatureMethod: "JWT_STEP_UP",
	}, approver, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Greater(t, result.ActivatedCount, -1)
}

// ─── Service RollbackRequest happy path ──────────────────────────────────────

func TestService_RollbackRequest_HappyPath(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	future := time.Now().UTC().Add(24 * time.Hour)

	repo.batchByID[batchID] = &Batch{
		ID:                     batchID,
		Status:                 StatusApproved,
		UploadedBy:             actor,
		RollbackGraceExpiresAt: &future,
		TenantID:               "TUGURE",
	}

	// Repo mock's methods don't use the real *sql.Tx, so only Begin+Commit needed
	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	err := svc.RollbackRequest(context.Background(), batchID, RollbackRequestBody{
		Reason: "Alasan rollback yang cukup panjang untuk memenuhi syarat minimal 50 karakter disini",
	}, actor, "TUGURE")
	require.NoError(t, err)
}

// ─── Service RollbackApprove happy path ──────────────────────────────────────

func TestService_RollbackApprove_HappyPath(t *testing.T) {
	repo := newMockRepo()
	cfo := uuid.New()
	batchID := uuid.New()

	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusRollbackPending,
		UploadedBy: uuid.New(),
		TenantID:   "TUGURE",
	}
	// seed some committed rows so UpdateRowsRolledBack returns > 0
	rowID := uuid.New()
	repo.rowsByBatch[batchID] = []BatchRow{
		{ID: rowID, BatchID: batchID, RowStatus: RowStatusCommitted},
	}

	// Repo mock's methods don't use the real *sql.Tx, so only Begin+Commit needed
	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	result, err := svc.RollbackApprove(context.Background(), batchID, RollbackApproveBody{
		Comment:        "CFO approves rollback per RACI",
		SignatureMethod: "JWT_STEP_UP",
	}, cfo, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, result)
}

// ─── Service UploadBatch happy path ──────────────────────────────────────────

func TestService_UploadBatch_HappyPath(t *testing.T) {
	repo := newMockRepo()
	actor := uuid.New()

	fileData := newTestXLSXBytes(t)

	// Repo mock's methods don't use the real *sql.Tx, so only Begin+Commit needed
	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	result, err := svc.UploadBatch(context.Background(), "test.xlsx", fileData, actor, "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.BatchID)
}

// ─── writeAuditInTx coverage ─────────────────────────────────────────────────

func TestService_WriteAuditInTx_NilAudit(t *testing.T) {
	svc := NewService(newMockRepo(), nil, nil, nil)
	// nil audit → should not panic
	svc.writeAuditInTx(context.Background(), nil, audit.Event{Action: "TEST"}, uuid.New())
}

func TestService_WriteAuditInTx_NilTx(t *testing.T) {
	svc := NewService(newMockRepo(), nil, nil, nil)
	// nil tx → should not panic
	svc.writeAuditInTx(context.Background(), nil, audit.Event{Action: "TEST"}, uuid.New())
}

func TestService_WriteAuditInTx_NonNilAuditAndTx(t *testing.T) {
	// Cover the ActorUserID assignment + WithTx().Write() call path
	// Writer will fail (no real DB), but that's covered in the error log path
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Expect Begin + (write will fail with no table) + Rollback
	mock.ExpectBegin()
	mock.ExpectCommit()

	auditWriter := audit.NewWriter(db)
	svc := NewService(newMockRepo(), nil, auditWriter, nil)
	wireTxBegin(svc, db)

	tx, txErr := svc.txBegin(context.Background())
	require.NoError(t, txErr)

	// This will try to INSERT into aud.audit_log which doesn't exist in sqlmock → error logged, no panic
	svc.writeAuditInTx(context.Background(), tx, audit.Event{
		Action:     "TEST.ACTION",
		EntityType: "test",
		EntityID:   uuid.New(),
	}, uuid.New())

	_ = tx.Commit() // no-op, complete the sqlmock expectation
}

// ─── Validator helper coverage ────────────────────────────────────────────────

func TestValidatePositiveNumeric_Valid(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"amount": "123456.78"}
	validatePositiveNumeric(&errs, SheetDeposito, 2, row, "amount")
	assert.Empty(t, errs)
}

func TestValidatePositiveNumeric_Invalid(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"amount": "-100"}
	validatePositiveNumeric(&errs, SheetDeposito, 2, row, "amount")
	assert.NotEmpty(t, errs)
}

func TestValidateRateRange_Valid(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"rate": "0.065"}
	validateRateRange(&errs, SheetDeposito, 2, row, "rate")
	assert.Empty(t, errs)
}

func TestValidateRateRange_Invalid(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"rate": "2.5"} // > 1.0
	validateRateRange(&errs, SheetDeposito, 2, row, "rate")
	assert.NotEmpty(t, errs)
}

func TestValidatePositiveInteger_Valid(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"count": "100"}
	validatePositiveInteger(&errs, SheetDeposito, 2, row, "count")
	assert.Empty(t, errs)
}

func TestValidatePositiveInteger_Invalid(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"count": "-5"}
	validatePositiveInteger(&errs, SheetDeposito, 2, row, "count")
	assert.NotEmpty(t, errs)
}

func TestValidatePositiveNumeric_NotANumber(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"amount": "not-a-number"}
	validatePositiveNumeric(&errs, SheetDeposito, 2, row, "amount")
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error, "NUMERIC")
}

func TestValidateRateRange_NotANumber(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"rate": "abc"}
	validateRateRange(&errs, SheetDeposito, 2, row, "rate")
	assert.NotEmpty(t, errs)
}

func TestValidatePositiveInteger_NotAnInt(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{"count": "3.14"}
	validatePositiveInteger(&errs, SheetDeposito, 2, row, "count")
	assert.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error, "INTEGER")
}

func TestValidatePositiveNumeric_Empty(t *testing.T) {
	var errs []RowError
	row := map[string]interface{}{}
	validatePositiveNumeric(&errs, SheetDeposito, 2, row, "missing_col")
	assert.Empty(t, errs) // empty col → skip
}

// ─── Service Approve SoD happy tx path ───────────────────────────────────────

func TestService_Approve_SoD_TxSucceeds(t *testing.T) {
	// Cover the SoD violation path where txBegin succeeds (audit logged then rejected)
	repo := newMockRepo()
	actor := uuid.New()
	batchID := uuid.New()
	repo.batchByID[batchID] = &Batch{
		ID:         batchID,
		Status:     StatusCommitted,
		UploadedBy: actor, // same as actor → SoD violation
		TenantID:   "TUGURE",
	}

	db, mock := newMockTxDB(t)
	mock.ExpectBegin()
	mock.ExpectCommit()

	svc := NewService(repo, nil, nil, nil)
	wireTxBegin(svc, db)

	_, err := svc.Approve(context.Background(), batchID, ApproveRequest{
		Comment:        "Trying to approve own",
		SignatureMethod: "JWT_STEP_UP",
	}, actor, "TUGURE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), CodeBulkApproveSoDViolation)
}

// ─── NewServiceWithDB ─────────────────────────────────────────────────────────

func TestNewServiceWithDB_NilDB(t *testing.T) {
	repo := newMockRepo()
	svc := NewServiceWithDB(repo, nil, nil, nil, nil)
	require.NotNil(t, svc)
	// txBegin still points to stub when db=nil
	_, err := svc.txBegin(context.Background())
	require.Error(t, err) // stub returns error
}

type fakeTxDB struct{ db *sql.DB }
func (f *fakeTxDB) BeginTxContext(ctx context.Context) (*sql.Tx, error) {
	return f.db.BeginTx(ctx, nil)
}

func TestNewServiceWithDB_WithDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	repo := newMockRepo()
	svc := NewServiceWithDB(repo, &fakeTxDB{db: db}, nil, nil, nil)
	require.NotNil(t, svc)

	tx, err := svc.txBegin(context.Background())
	require.NoError(t, err)
	require.NotNil(t, tx)
	_ = tx.Commit()
}

// ─── RegisterRoutes coverage ──────────────────────────────────────────────────

func TestRegisterRoutes_Compiles(t *testing.T) {
	// Just verify RegisterRoutes doesn't panic when called
	// (routes compile + register correctly)
	require.NotPanics(t, func() {
		r := gin.New()
		h := NewHTTPHandler(NewService(newMockRepo(), nil, nil, nil), nil)
		v1 := r.Group("/api/v1")
		RegisterRoutes(v1, h) // no rdb
	})
}

// ─── NewCommitTask ────────────────────────────────────────────────────────────

func TestNewCommitTask_CreatesValidTask(t *testing.T) {
	batchID := uuid.New()
	actorID := uuid.New()
	jobID := uuid.New()

	task, err := NewCommitTask(batchID, actorID, "TUGURE", jobID)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskCommitInstrumen, task.Type())

	var payload CommitJobPayload
	err = errors.New("ok")
	_ = err
	require.NoError(t, func() error {
		return json.Unmarshal(task.Payload(), &payload)
	}())
	assert.Equal(t, batchID.String(), payload.BatchID)
	assert.Equal(t, actorID.String(), payload.ActorID)
	assert.Equal(t, "TUGURE", payload.TenantID)
	assert.Equal(t, jobID.String(), payload.JobID)
}
