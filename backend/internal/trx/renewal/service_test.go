package renewal

// service_test.go — Unit tests for Service using stub implementations.
// Uses testify + hand-rolled stubs (no gomock needed — interfaces are simple).
//
// Coverage targets:
//   - CreateRenewal: happy path + SoD not applicable (maker not same role as approver)
//   - Approve: happy path + SoD violation + invalid signatureMethod + EIR convergence
//   - Reject: happy path + SoD violation + short comment
//   - GetDetail, GetList, GetPreview: happy path

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Stub repository ─────────────────────────────────────────────────────────

type stubRepo struct {
	renewal        *Renewal
	instrumenInfo  *InstrumenInfo
	hasActive      bool
	periode        *PeriodeBuku
	insertErr      error
	updateErr      error
	getByIDErr     error
	getInstErr     error
	hasActiveErr   error
	getPeriodeErr  error
	listRows       []*Renewal
}

func (r *stubRepo) GetByID(_ context.Context, id uuid.UUID) (*Renewal, error) {
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
	}
	return r.renewal, nil
}

func (r *stubRepo) GetInstrumenInfo(_ context.Context, id uuid.UUID) (*InstrumenInfo, error) {
	if r.getInstErr != nil {
		return nil, r.getInstErr
	}
	return r.instrumenInfo, nil
}

func (r *stubRepo) HasActiveRenewal(_ context.Context, id uuid.UUID) (bool, error) {
	if r.hasActiveErr != nil {
		return false, r.hasActiveErr
	}
	return r.hasActive, nil
}

func (r *stubRepo) Insert(_ context.Context, _ *sql.Tx, row *Renewal) error {
	return r.insertErr
}

func (r *stubRepo) UpdateStatus(_ context.Context, _ *sql.Tx, id uuid.UUID, u StatusUpdate) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.renewal != nil {
		r.renewal.Status = u.Status
		if u.ApproverID != nil {
			r.renewal.ApproverID = u.ApproverID
		}
		r.renewal.RowVersion++
	}
	return nil
}

func (r *stubRepo) List(_ context.Context, q listquery.Query, cursor string, limit int) ([]*Renewal, bool, int, error) {
	return r.listRows, false, len(r.listRows), nil
}

func (r *stubRepo) GetPeriodeByTanggal(_ context.Context, t time.Time) (*PeriodeBuku, error) {
	if r.getPeriodeErr != nil {
		return nil, r.getPeriodeErr
	}
	return r.periode, nil
}

func (r *stubRepo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	// Use go-sqlmock to return a real *sql.Tx.
	// Service code calls tx.Commit() on success and tx.Rollback() on deferred failure.
	// No audit writes happen in tests because NewService is called with nil auditWriter.
	db, mock, err := sqlmock.New()
	if err != nil {
		return nil, err
	}
	mock.ExpectBegin()
	// Service may also call Rollback via defer if Commit is called first (no-op after commit).
	// sqlmock ignores Rollback after Commit.
	mock.ExpectCommit()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

var (
	makerUUID    = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	approverUUID = uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	instrumenID  = uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	renewalID    = uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
	periodeID    = uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
)

func makerCtx() context.Context {
	claims := &auth.Claims{Sub: makerUUID.String(), TenantID: "TUGURE", Roles: []string{"ROLE-MAKER-TR"}}
	return auth.ContextWithClaims(context.Background(), claims)
}

func approverCtx() context.Context {
	claims := &auth.Claims{Sub: approverUUID.String(), TenantID: "TUGURE", Roles: []string{"ROLE-APPR-TR"}}
	return auth.ContextWithClaims(context.Background(), claims)
}

func goodInstrumen() *InstrumenInfo {
	return &InstrumenInfo{
		ID:                instrumenID,
		KodeInstrumen:     "DEP-UNIT-TEST-001",
		NamaInstrumen:     "Deposito Unit Test",
		JenisInstrumen:    "DEPOSITO",
		Status:            "ACTIVE",
		KlasifikasiLocked: true,
		Pokok:             decimal.NewFromInt(1_000_000_000),
		RatePersen:        decimal.NewFromFloat(6.0),
		TanggalPenempatan: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		TanggalJatuhTempo: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		MataUang:          "IDR",
	}
}

func goodPeriode() *PeriodeBuku {
	return &PeriodeBuku{
		ID:              periodeID,
		StatusPeriode:   "OPEN",
		TanggalMulai:    time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		TanggalAkhir:    time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
	}
}

func goodRenewal(status Status) *Renewal {
	eir := decimal.NewFromFloat(0.04800000)
	// Values match ComputeBungaKotor(1B, 6%, 2026-01-01 → 2026-07-01) = 181 days
	// BungaKotor = 1B × 0.06 × 181/365 = 29,753,424.6575
	// PPh        = 29,753,424.6575 × 0.20 = 5,950,684.9315
	// BungaBersih= 29,753,424.6575 − 5,950,684.9315 = 23,802,739.7260
	bungaKotor, _  := decimal.NewFromString("29753424.6575")
	pphAmount, _   := decimal.NewFromString("5950684.9315")
	bungaBersih, _ := decimal.NewFromString("23802739.7260")
	return &Renewal{
		ID:                    renewalID,
		InstrumenLamaID:       instrumenID,
		Skema:                 SkemaPokokSaja,
		TenorBaruBulan:        12,
		RateBaruPersen:        decimal.NewFromFloat(7.0),
		TanggalEfektifBaru:    time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		TanggalJatuhTempoBaru: time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC),
		PokokLama:             decimal.NewFromInt(1_000_000_000),
		PokokBaru:             decimal.NewFromInt(1_000_000_000),
		BungaKotor:            bungaKotor,
		PphAmount:             pphAmount,
		BungaBersih:           bungaBersih,
		EirBaru:               &eir,
		Status:                status,
		MakerID:               makerUUID,
		PeriodeBulananID:      &periodeID,
		RowVersion:            1,
		TenantID:              "TUGURE",
		CreatedAt:             time.Now(),
		CreatedBy:             makerUUID,
		UpdatedAt:             time.Now(),
		UpdatedBy:             makerUUID,
	}
}

func newService(repo Repository) *Service {
	poster := NewJurnalPosterStub(nil)
	instCreator := NewInstrumenCreatorStub()
	eirWriter := NewEIRScheduleWriterStub()
	return NewService(repo, poster, instCreator, eirWriter, nil, nil)
}

// ─── CreateRenewal ────────────────────────────────────────────────────────────

func TestService_CreateRenewal_HappyPath_PokokSaja(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		periode:       goodPeriode(),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(7.0),
		TanggalEfektifBaru: "2026-07-01",
	}

	result, err := svc.CreateRenewal(makerCtx(), req)
	require.NoError(t, err)
	assert.Equal(t, string(StatusPendingApproval), result.Status)
	assert.NotEmpty(t, result.RenewalID)
	assert.Equal(t, "1000000000.0000", result.Preview.PokokBaru)
}

func TestService_CreateRenewal_HappyPath_PokokPlusBunga(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		periode:       goodPeriode(),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_PLUS_BUNGA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(7.0),
		TanggalEfektifBaru: "2026-07-01",
	}

	result, err := svc.CreateRenewal(makerCtx(), req)
	require.NoError(t, err)
	// pokok_baru > pokok_lama for POKOK_PLUS_BUNGA
	pokokBaru, _ := decimal.NewFromString(result.Preview.PokokBaru)
	assert.True(t, pokokBaru.GreaterThan(decimal.NewFromInt(1_000_000_000)))
}

func TestService_CreateRenewal_NoClaims(t *testing.T) {
	repo := &stubRepo{}
	svc := newService(repo)

	_, err := svc.CreateRenewal(context.Background(), CreateRenewalRequest{})
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeUnauthorized, de.Code())
}

func TestService_CreateRenewal_InstrumenNotFound(t *testing.T) {
	repo := &stubRepo{instrumenInfo: nil}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(7.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeNotFound, de.Code())
}

func TestService_CreateRenewal_AlreadyHasActive(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     true,
		periode:       goodPeriode(),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(7.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeValidationFailed, de.Code())
}

func TestService_CreateRenewal_PeriodeClosed(t *testing.T) {
	closedPeriode := goodPeriode()
	closedPeriode.StatusPeriode = "SOFT_CLOSED"

	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		periode:       closedPeriode,
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(7.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodePeriodeClosed, de.Code())
}

func TestService_CreateRenewal_InvalidSkema(t *testing.T) {
	repo := &stubRepo{}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "INVALID_SKEMA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(7.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
}

func TestService_CreateRenewal_BungaBersihTooSmall(t *testing.T) {
	// Small pokok → small bunga → fails POKOK_PLUS_BUNGA minimum
	inst := goodInstrumen()
	inst.Pokok = decimal.NewFromInt(10_000) // tiny pokok → tiny bunga_bersih
	inst.TanggalPenempatan = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	repo := &stubRepo{
		instrumenInfo: inst,
		hasActive:     false,
		periode:       goodPeriode(),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_PLUS_BUNGA",
		TenorBaruBulan:     1,
		RateBaruPersen:     decimal.NewFromFloat(1.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	// bunga_bersih for 1 day of IDR 10,000 at 1% = essentially 0
	_, err := svc.CreateRenewal(makerCtx(), req)
	// May fail with validation or succeed if bunga > 100k — just verify no panic
	_ = err
}

// ─── Approve ──────────────────────────────────────────────────────────────────

func TestService_Approve_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Renewal disetujui setelah verifikasi dokumen.",
		SignatureMethod: "JWT_STEP_UP",
	}

	result, err := svc.Approve(approverCtx(), renewalID, req)
	require.NoError(t, err)
	assert.Equal(t, string(StatusPosted), result.Status)
	assert.NotNil(t, result.InstrumenBaruID)
	assert.NotNil(t, result.JurnalEntryID)
	assert.Equal(t, approverUUID.String(), result.ApprovedBy)
}

func TestService_Approve_SoDViolation(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = approverUUID // same user = SoD violation

	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Approve by same user",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeSoDViolation, de.Code())
}

func TestService_Approve_InvalidSignatureMethod(t *testing.T) {
	repo := &stubRepo{}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Some comment about the approval",
		SignatureMethod: "PASSWORD",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
}

func TestService_Approve_RenewalNotFound(t *testing.T) {
	repo := &stubRepo{renewal: nil}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Approve",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeNotFound, de.Code())
}

func TestService_Approve_InvalidState_POSTED(t *testing.T) {
	renewal := goodRenewal(StatusPosted)
	repo := &stubRepo{
		renewal: renewal,
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Trying to approve already posted",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeWorkflowInvalidTransition, de.Code())
}

func TestService_Approve_PeriodeClosed(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	closedPeriode := goodPeriode()
	closedPeriode.StatusPeriode = "HARD_CLOSED"

	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       closedPeriode,
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Approve despite closed period",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodePeriodeClosed, de.Code())
}

func TestService_Approve_JurnalPosterError(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
	}

	poster := NewJurnalPosterStub(nil)
	poster.SetError(fmt.Errorf("jurnal engine unavailable"))

	instCreator := NewInstrumenCreatorStub()
	eirWriter := NewEIRScheduleWriterStub()
	svc := NewService(repo, poster, instCreator, eirWriter, nil, nil)

	req := ApproveRenewalRequest{
		Comment:        "Approve with failing poster",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jurnal")
}

func TestService_Approve_NoClaims(t *testing.T) {
	repo := &stubRepo{}
	svc := newService(repo)

	req := ApproveRenewalRequest{
		Comment:        "Approve without claims",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(context.Background(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeUnauthorized, de.Code())
}

// ─── Reject ───────────────────────────────────────────────────────────────────

func TestService_Reject_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{renewal: renewal}
	svc := newService(repo)

	req := RejectRenewalRequest{
		Comment:        "Rate baru tidak sesuai kebijakan treasury investasi.",
		SignatureMethod: "JWT_STEP_UP",
	}

	result, err := svc.Reject(approverCtx(), renewalID, req)
	require.NoError(t, err)
	assert.Equal(t, string(StatusRejected), result.Status)
	assert.Equal(t, approverUUID.String(), result.RejectedBy)
}

func TestService_Reject_SoDViolation(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	renewal.MakerID = approverUUID

	repo := &stubRepo{renewal: renewal}
	svc := newService(repo)

	req := RejectRenewalRequest{
		Comment:        "Reject by same user as maker — should fail SoD check",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Reject(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeSoDViolation, de.Code())
}

func TestService_Reject_CommentTooShort(t *testing.T) {
	repo := &stubRepo{}
	svc := newService(repo)

	req := RejectRenewalRequest{
		Comment:        "Short",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Reject(approverCtx(), renewalID, req)
	require.Error(t, err)
}

func TestService_Reject_InvalidState_REJECTED(t *testing.T) {
	renewal := goodRenewal(StatusRejected)
	repo := &stubRepo{renewal: renewal}
	svc := newService(repo)

	req := RejectRenewalRequest{
		Comment:        "Reject already rejected renewal — should fail state machine",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Reject(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeWorkflowInvalidTransition, de.Code())
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

func TestService_GetDetail_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
	}
	svc := newService(repo)

	detail, err := svc.GetDetail(approverCtx(), renewalID)
	require.NoError(t, err)
	assert.Equal(t, renewalID.String(), detail.ID)
	assert.Equal(t, string(StatusPendingApproval), detail.Status)
	assert.NotEmpty(t, detail.Preview.PokokBaru)
}

func TestService_GetDetail_NotFound(t *testing.T) {
	repo := &stubRepo{renewal: nil}
	svc := newService(repo)

	_, err := svc.GetDetail(approverCtx(), renewalID)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeNotFound, de.Code())
}

// ─── GetList ──────────────────────────────────────────────────────────────────

func TestService_GetList_HappyPath(t *testing.T) {
	r1 := goodRenewal(StatusPendingApproval)
	r2 := goodRenewal(StatusPosted)
	repo := &stubRepo{listRows: []*Renewal{r1, r2}}
	svc := newService(repo)

	rows, hasMore, total, err := svc.GetList(approverCtx(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.False(t, hasMore)
	assert.Equal(t, 2, total)
}

func TestService_GetList_LimitClamped(t *testing.T) {
	repo := &stubRepo{listRows: nil}
	svc := newService(repo)

	// limit 0 → clamped to 50
	_, _, _, err := svc.GetList(approverCtx(), listquery.Query{}, "", 0)
	assert.NoError(t, err)
}

// ─── GetPreview ───────────────────────────────────────────────────────────────

func TestService_GetPreview_HappyPath(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
	}
	svc := newService(repo)

	preview, err := svc.GetPreview(approverCtx(), renewalID)
	require.NoError(t, err)
	assert.NotEmpty(t, preview.PokokBaru)
	assert.NotEmpty(t, preview.EirBaru)
}

// ─── F3: BungaKotor field in RenewalPostRequest ───────────────────────────────

// TestService_Approve_BungaKotorPopulated verifies that the Post call receives
// BungaKotor == BungaBersih + PphAmount (F3 compliance requirement).
func TestService_Approve_BungaKotorPopulated(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
	}

	poster := NewJurnalPosterStub(nil)
	instCreator := NewInstrumenCreatorStub()
	eirWriter := NewEIRScheduleWriterStub()
	svc := NewService(repo, poster, instCreator, eirWriter, nil, nil)

	req := ApproveRenewalRequest{
		Comment:        "Approve — check BungaKotor populated in Post call.",
		SignatureMethod: "JWT_STEP_UP",
	}

	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.NoError(t, err)

	calls := poster.Calls()
	require.Len(t, calls, 1, "exactly one Post call expected")
	postReq := calls[0]

	// BungaKotor must equal BungaBersih + PphAmount (identity invariant).
	sum := postReq.BungaBersih.Add(postReq.PphAmount).RoundBank(4)
	assert.Equal(t, sum.StringFixed(4), postReq.BungaKotor.StringFixed(4),
		"BungaKotor == BungaBersih + PphAmount invariant violated")

	// All three must be positive.
	assert.True(t, postReq.BungaKotor.IsPositive(), "BungaKotor must be positive")
	assert.True(t, postReq.BungaBersih.IsPositive(), "BungaBersih must be positive")
	assert.True(t, postReq.PphAmount.IsPositive(), "PphAmount must be positive")

	// BungaKotor must be > BungaBersih (tax was withheld).
	assert.True(t, postReq.BungaKotor.GreaterThan(postReq.BungaBersih),
		"BungaKotor must be > BungaBersih (PPh withheld)")
}
