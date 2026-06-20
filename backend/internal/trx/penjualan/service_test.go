package penjualan

// service_test.go — Unit tests for penjualan.Service using hand-rolled stubs.
//
// Coverage targets:
//   - CreatePenjualan: happy path AC, happy path FVOCI, happy path FVOCI_ELECTION
//   - CreatePenjualan: instrumen not found, not active, clasifikasi not locked, hasActive conflict
//   - CreatePenjualan: periode not found, periode closed
//   - Approve: happy path (AC), SoD violation, invalid signatureMethod, BM block, BM warn
//   - Approve: FVOCI recycle OCI, FVOCI_ELECTION no recycle (NoRecyclingNote)
//   - Approve: jurnal post error → rollback
//   - Reject: happy path, SoD violation, short reason
//   - GetDetail, GetPreview, GetList, ListBMAlerts: happy path

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
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Stub repository ──────────────────────────────────────────────────────────

type stubPenjualanRepo struct {
	penjualan            *Penjualan
	instrumenInfo        *InstrumenInfo
	hasActive            bool
	periode              *PeriodeBuku
	ociCumulative        decimal.Decimal
	amortizedCarrying    decimal.Decimal
	rolling12m           decimal.Decimal
	portofolioNilai      decimal.Decimal
	warnThreshold        decimal.Decimal
	blockThreshold       decimal.Decimal
	bmAlerts             []*BMAlertItem
	listRows             []*Penjualan
	insertErr            error
	updateErr            error
	getByIDErr           error
	getInstErr           error
	hasActiveErr         error
	getPeriodeErr        error
	getOCIErr            error
	getCarryingErr       error
	getRolling12mErr     error
	getPortofolioErr     error
	getBMConfigErr       error
	updateStatusCallsLog []StatusUpdate
}

func newDefaultStubRepo() *stubPenjualanRepo {
	return &stubPenjualanRepo{
		warnThreshold:  decimal.NewFromInt(5),
		blockThreshold: decimal.NewFromInt(10),
		portofolioNilai: decimal.NewFromInt(1000000000), // 1B IDR
	}
}

func (r *stubPenjualanRepo) GetByID(_ context.Context, id uuid.UUID) (*Penjualan, error) {
	if r.getByIDErr != nil { return nil, r.getByIDErr }
	return r.penjualan, nil
}

func (r *stubPenjualanRepo) GetInstrumenInfo(_ context.Context, _ uuid.UUID) (*InstrumenInfo, error) {
	if r.getInstErr != nil { return nil, r.getInstErr }
	return r.instrumenInfo, nil
}

func (r *stubPenjualanRepo) HasActivePenjualan(_ context.Context, _ uuid.UUID) (bool, error) {
	if r.hasActiveErr != nil { return false, r.hasActiveErr }
	return r.hasActive, nil
}

func (r *stubPenjualanRepo) GetOCICumulativeByInstrumen(_ context.Context, _ uuid.UUID) (decimal.Decimal, error) {
	return r.ociCumulative, r.getOCIErr
}

func (r *stubPenjualanRepo) GetAmortizedCarryingByInstrumen(_ context.Context, _ uuid.UUID, _ time.Time) (decimal.Decimal, error) {
	return r.amortizedCarrying, r.getCarryingErr
}

func (r *stubPenjualanRepo) GetRolling12mDisposalIDR(_ context.Context, _ uuid.UUID) (decimal.Decimal, error) {
	return r.rolling12m, r.getRolling12mErr
}

func (r *stubPenjualanRepo) GetPortofolioNilai(_ context.Context, _ uuid.UUID) (decimal.Decimal, error) {
	return r.portofolioNilai, r.getPortofolioErr
}

func (r *stubPenjualanRepo) GetBMConfigThresholds(_ context.Context) (decimal.Decimal, decimal.Decimal, error) {
	return r.warnThreshold, r.blockThreshold, r.getBMConfigErr
}

func (r *stubPenjualanRepo) Insert(_ context.Context, _ *sql.Tx, p *Penjualan) error {
	r.penjualan = p
	return r.insertErr
}

func (r *stubPenjualanRepo) UpdateStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, u StatusUpdate) error {
	if r.updateErr != nil { return r.updateErr }
	r.updateStatusCallsLog = append(r.updateStatusCallsLog, u)
	if r.penjualan != nil {
		r.penjualan.Status = u.Status
		r.penjualan.RowVersion++
		if u.ApproverID != nil { r.penjualan.ApproverID = u.ApproverID }
		if u.QtyHoldingPost != nil { r.penjualan.QtyHoldingPost = u.QtyHoldingPost }
	}
	return nil
}

func (r *stubPenjualanRepo) List(_ context.Context, _ listquery.Query, _ string, _ int) ([]*Penjualan, bool, int, error) {
	return r.listRows, false, len(r.listRows), nil
}

func (r *stubPenjualanRepo) ListBMAlerts(_ context.Context) ([]*BMAlertItem, error) {
	return r.bmAlerts, nil
}

func (r *stubPenjualanRepo) GetPeriodeByTanggal(_ context.Context, _ time.Time) (*PeriodeBuku, error) {
	if r.getPeriodeErr != nil { return nil, r.getPeriodeErr }
	return r.periode, nil
}

func (r *stubPenjualanRepo) BeginTx(_ context.Context) (*sql.Tx, error) {
	db, mock, err := sqlmock.New()
	if err != nil { return nil, fmt.Errorf("sqlmock: %w", err) }
	mock.ExpectBegin()
	mock.ExpectCommit()
	tx, err := db.Begin()
	if err != nil { return nil, err }
	return tx, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

var (
	testMakerID    = uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	testApproverID = uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	testInstrumenID = uuid.New()
	testPortofolioID = uuid.New()
)

func ctxWithMaker() context.Context {
	return auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:   testMakerID.String(),
		Roles: []string{"ROLE-MAKER-TR"},
	})
}

func ctxWithApprover() context.Context {
	return auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:   testApproverID.String(),
		Roles: []string{"ROLE-APPR-TR"},
	})
}

func defaultInstrumen(klasifikasi KlasifikasiPSAK71) *InstrumenInfo {
	return &InstrumenInfo{
		ID:                 testInstrumenID,
		KodeInstrumen:      "OBL-001",
		NamaInstrumen:      "Obligasi Test",
		Status:             "ACTIVE",
		KlasifikasiPSAK71:  string(klasifikasi),
		KlasifikasiLocked:  true,
		QtyHolding:         decimal.NewFromInt(1000),
		HargaPerolehan:     decimal.NewFromInt(1000000),
		PortofolioID:       testPortofolioID,
		BusinessModel:      "HTC&S", // Non-HTC to skip BM check by default
		MataUang:           "IDR",
		CounterpartyID:     uuid.New(),
	}
}

func defaultPeriode() *PeriodeBuku {
	return &PeriodeBuku{
		ID:            uuid.New(),
		StatusPeriode: "OPEN",
		TanggalMulai:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalAkhir:  time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
	}
}

func defaultCreateReq(klasifikasi string) CreatePenjualanRequest {
	return CreatePenjualanRequest{
		InstrumenID:      testInstrumenID,
		JenisDisposal:    "PARTIAL",
		QtyTerjual:       decimal.NewFromInt(500),
		HargaJualPerUnit: decimal.NewFromInt(1100),
		TanggalEksekusi:  "2026-06-15",
	}
}

func defaultApproveReq() ApprovePenjualanRequest {
	return ApprovePenjualanRequest{
		Comment:        "Harga sesuai market",
		SignatureMethod: "JWT_STEP_UP",
	}
}

func newTestService(repo *stubPenjualanRepo) *Service {
	poster := NewJurnalPosterStub(nil)
	instrUpdate := NewInstrumenUpdaterStub()
	riskNotif := NewRiskNotifierStub()
	return NewService(repo, poster, instrUpdate, riskNotif, nil, nil)
}

// ─── CreatePenjualan ──────────────────────────────────────────────────────────

func TestCreatePenjualan_AC_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000) // 900M total carrying

	svc := newTestService(repo)
	resp, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.NoError(t, err)
	assert.Equal(t, string(StatusPendingApproval), resp.Status)
	assert.NotEmpty(t, resp.PenjualanID)
	assert.Equal(t, "AC", resp.Preview.KlasifikasiPsak71)
	// proceed = 1100 × 500 = 550_000
	assert.Equal(t, "550000.0000", resp.Preview.ProceedIdr)
}

func TestCreatePenjualan_FVOCI_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiFVOCI)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(500000000)
	repo.ociCumulative = decimal.NewFromInt(10000000) // 10M cumulative OCI

	svc := newTestService(repo)
	resp, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("FVOCI"))
	require.NoError(t, err)
	assert.Equal(t, string(StatusPendingApproval), resp.Status)
	// FVOCI partial: OCI recycled proportionally for half holding
	assert.NotNil(t, resp.Preview.OciRecycled, "FVOCI must have OCI recycled in preview")
}

func TestCreatePenjualan_FVOCIElection_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiFVOCIElection)
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	resp, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("FVOCI_ELECTION"))
	require.NoError(t, err)
	assert.Equal(t, string(StatusPendingApproval), resp.Status)
	assert.Nil(t, resp.Preview.OciRecycled, "FVOCI_ELECTION: OCI must NOT be recycled")
	assert.NotNil(t, resp.Preview.NoRecyclingNote, "FVOCI_ELECTION must have no-recycling note")
}

func TestCreatePenjualan_InstrumenNotFound(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = nil // not found
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeNotFound, de.Code())
}

func TestCreatePenjualan_InstrumenNotActive(t *testing.T) {
	repo := newDefaultStubRepo()
	inst := defaultInstrumen(KlasifikasiAC)
	inst.Status = "DISPOSED"
	repo.instrumenInfo = inst
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
}

func TestCreatePenjualan_KlasifikasiNotLocked(t *testing.T) {
	repo := newDefaultStubRepo()
	inst := defaultInstrumen(KlasifikasiAC)
	inst.KlasifikasiLocked = false
	repo.instrumenInfo = inst
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
}

func TestCreatePenjualan_QtyExceedsHolding(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	req := defaultCreateReq("AC")
	req.QtyTerjual = decimal.NewFromInt(1001) // > 1000 holding
	_, err := svc.CreatePenjualan(ctxWithMaker(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "melebihi")
}

func TestCreatePenjualan_HasActivePenjualan_Error(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.hasActive = true
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aktif")
}

func TestCreatePenjualan_PeriodeNotFound(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = nil

	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "periode")
}

func TestCreatePenjualan_PeriodeClosed(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = &PeriodeBuku{
		ID:            uuid.New(),
		StatusPeriode: "HARD_CLOSED",
		TanggalMulai:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalAkhir:  time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
	}

	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(ctxWithMaker(), defaultCreateReq("AC"))
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodePeriodeClosed, de.Code())
}

func TestCreatePenjualan_NoJWTClaims_Error(t *testing.T) {
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	_, err := svc.CreatePenjualan(context.Background(), defaultCreateReq("AC"))
	require.Error(t, err)
}

func TestCreatePenjualan_InvalidTanggal_Error(t *testing.T) {
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	req := defaultCreateReq("AC")
	req.TanggalEksekusi = "not-a-date"
	_, err := svc.CreatePenjualan(ctxWithMaker(), req)
	require.Error(t, err)
}

// ─── Approve ─────────────────────────────────────────────────────────────────

func pendingPenjualan(makerID uuid.UUID, klasifikasi KlasifikasiPSAK71, jenis DisposalType) *Penjualan {
	now := time.Now().UTC()
	qty := decimal.NewFromInt(500)
	harga := decimal.NewFromInt(1100)
	proceed := harga.Mul(qty)
	cost := decimal.NewFromInt(490000)
	return &Penjualan{
		ID:                  uuid.New(),
		InstrumenID:         testInstrumenID,
		JenisDisposal:       jenis,
		QtyTerjual:          qty,
		QtyHoldingPre:       decimal.NewFromInt(1000),
		HargaJualPerUnit:    harga,
		Proceed:             proceed,
		CostBasis:           cost,
		RealizedGL:          proceed.Sub(cost),
		KlasifikasiSnapshot: klasifikasi,
		TanggalEksekusi:     time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		Status:              StatusPendingApproval,
		MakerID:             makerID,
		CreatedAt:           now,
		UpdatedAt:           now,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}
}

func TestApprove_AC_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)

	svc := newTestService(repo)
	resp, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err)
	assert.Equal(t, string(StatusPosted), resp.Status)
	assert.Equal(t, testApproverID.String(), resp.ApprovedBy)
	assert.NotNil(t, resp.JurnalEntryID)
	assert.Nil(t, resp.OCIRecycled, "AC must not recycle OCI")
}

func TestApprove_FVOCI_RecyclesOCI(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiFVOCI, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiFVOCI)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(500000000)
	repo.ociCumulative = decimal.NewFromInt(20000000)

	svc := newTestService(repo)
	resp, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err)
	assert.Equal(t, string(StatusPosted), resp.Status)
	assert.NotNil(t, resp.OCIRecycled, "FVOCI debt must recycle OCI")
}

func TestApprove_FVOCIElection_NoRecycling(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiFVOCIElection, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiFVOCIElection)
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	resp, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err)
	assert.Equal(t, string(StatusPosted), resp.Status)
	assert.Nil(t, resp.OCIRecycled, "FVOCI_ELECTION must NOT recycle OCI")
	assert.NotNil(t, resp.NoRecyclingNote, "FVOCI_ELECTION must set NoRecyclingNote")
	assert.Contains(t, resp.Warnings, CodePenjualanFVOCIElectionNoRecyclingWarn)
}

func TestApprove_FULL_SetsInstrumenDisposed(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalFull)
	pj.QtyHoldingPre = decimal.NewFromInt(1000)
	pj.QtyTerjual = decimal.NewFromInt(1000) // full disposal
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)

	instrUpdate := NewInstrumenUpdaterStub()
	svc := NewService(repo, NewJurnalPosterStub(nil), instrUpdate, NewRiskNotifierStub(), nil, nil)
	resp, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err)
	assert.Equal(t, string(StatusPosted), resp.Status)
	assert.Equal(t, 1, instrUpdate.DisposeCalls(), "SetDisposed must be called for FULL disposal")
	assert.Equal(t, 0, instrUpdate.QtyCalls(), "UpdateQty must NOT be called for FULL disposal")
	assert.Equal(t, "DISPOSED", *resp.InstrumenStatusAfter)
}

func TestApprove_PARTIAL_UpdatesQty(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)

	instrUpdate := NewInstrumenUpdaterStub()
	svc := NewService(repo, NewJurnalPosterStub(nil), instrUpdate, NewRiskNotifierStub(), nil, nil)
	_, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err)
	assert.Equal(t, 1, instrUpdate.QtyCalls(), "UpdateQty must be called for PARTIAL disposal")
	assert.Equal(t, 0, instrUpdate.DisposeCalls(), "SetDisposed must NOT be called for PARTIAL disposal")
}

func TestApprove_SoDViolation(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()

	svc := newTestService(repo)
	// Approver same as maker
	ctxSameUser := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub:   testMakerID.String(), // SAME as maker
		Roles: []string{"ROLE-APPR-TR"},
	})
	_, err := svc.Approve(ctxSameUser, pj.ID, defaultApproveReq())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeSoDViolation, de.Code())
}

func TestApprove_InvalidSignatureMethod(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj

	svc := newTestService(repo)
	req := defaultApproveReq()
	req.SignatureMethod = "PASSWORD"
	_, err := svc.Approve(ctxWithApprover(), pj.ID, req)
	require.Error(t, err)
}

func TestApprove_PenjualanNotFound(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.penjualan = nil

	svc := newTestService(repo)
	_, err := svc.Approve(ctxWithApprover(), uuid.New(), defaultApproveReq())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeNotFound, de.Code())
}

func TestApprove_PeriodeClosed(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = &PeriodeBuku{
		ID:            uuid.New(),
		StatusPeriode: "SOFT_CLOSED",
		TanggalMulai:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalAkhir:  time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC),
	}

	svc := newTestService(repo)
	_, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodePeriodeClosed, de.Code())
}

func TestApprove_BM_WarnThreshold(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	inst := defaultInstrumen(KlasifikasiAC)
	inst.BusinessModel = "HTC" // Must be HTC for BM check
	repo.instrumenInfo = inst
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)
	// rolling 12m + current proceed = 6% of 1B → warn (>5% but ≤10%)
	repo.rolling12m = decimal.NewFromInt(55450000)    // 55.45M + current 550k ≈ 6%
	repo.portofolioNilai = decimal.NewFromInt(1000000000) // 1B

	svc := newTestService(repo)
	resp, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.NoError(t, err, "BM warn must NOT block — still posts")
	assert.True(t, resp.BMViolationRisk, "BMViolationRisk must be true on warn")
	assert.Equal(t, string(StatusPosted), resp.Status, "warn: still POSTED")
}

func TestApprove_BM_BlockThreshold(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	inst := defaultInstrumen(KlasifikasiAC)
	inst.BusinessModel = "HTC"
	repo.instrumenInfo = inst
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)
	// rolling 12m makes total > 10% → block
	repo.rolling12m = decimal.NewFromInt(105000000)   // 105M + 550k > 10% of 1B
	repo.portofolioNilai = decimal.NewFromInt(1000000000)

	svc := newTestService(repo)
	_, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.Error(t, err, "BM block threshold must return error")
	assert.Contains(t, err.Error(), "BM")
	// penjualan should be in PENDING_BM_REVIEW state
	require.NotEmpty(t, repo.updateStatusCallsLog)
	var foundBMReview bool
	for _, u := range repo.updateStatusCallsLog {
		if u.Status == StatusPendingBMReview {
			foundBMReview = true
		}
	}
	assert.True(t, foundBMReview, "BM block: status must be set to PENDING_BM_REVIEW")
}

func TestApprove_JurnalPosterError_Rollback(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.periode = defaultPeriode()
	repo.amortizedCarrying = decimal.NewFromInt(900000000)

	poster := NewJurnalPosterStub(nil)
	poster.SetError(fmt.Errorf("jurnal engine unavailable"))
	svc := NewService(repo, poster, NewInstrumenUpdaterStub(), NewRiskNotifierStub(), nil, nil)

	_, err := svc.Approve(ctxWithApprover(), pj.ID, defaultApproveReq())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jurnal post")
}

// ─── Reject ──────────────────────────────────────────────────────────────────

func TestReject_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)

	svc := newTestService(repo)
	resp, err := svc.Reject(ctxWithApprover(), pj.ID, RejectPenjualanRequest{
		Reason:         "Harga jual terlalu rendah, tidak sesuai ketentuan treasury 2026.",
		SignatureMethod: "JWT_STEP_UP",
	})
	require.NoError(t, err)
	assert.Equal(t, string(StatusRejected), resp.Status)
	assert.Equal(t, testApproverID.String(), resp.RejectedBy)
}

func TestReject_SoDViolation(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj

	svc := newTestService(repo)
	ctxSameUser := auth.ContextWithClaims(context.Background(), &auth.Claims{
		Sub: testMakerID.String(),
	})
	_, err := svc.Reject(ctxSameUser, pj.ID, RejectPenjualanRequest{
		Reason:         "Alasan penolakan yang cukup panjang supaya lolos validasi min 30 karakter",
		SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeSoDViolation, de.Code())
}

func TestReject_ShortReason_Error(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj

	svc := newTestService(repo)
	_, err := svc.Reject(ctxWithApprover(), pj.ID, RejectPenjualanRequest{
		Reason:         "terlalu singkat",
		SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30")
}

func TestReject_WrongStatus_Error(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	pj.Status = StatusPosted // cannot reject POSTED
	repo.penjualan = pj

	svc := newTestService(repo)
	_, err := svc.Reject(ctxWithApprover(), pj.ID, RejectPenjualanRequest{
		Reason:         "Alasan penolakan yang cukup panjang supaya lolos validasi minimum 30 karakter",
		SignatureMethod: "JWT_STEP_UP",
	})
	require.Error(t, err)
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

func TestGetDetail_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.penjualan = pj
	repo.instrumenInfo = defaultInstrumen(KlasifikasiAC)
	repo.amortizedCarrying = decimal.NewFromInt(500000000)

	svc := newTestService(repo)
	detail, err := svc.GetDetail(context.Background(), pj.ID)
	require.NoError(t, err)
	assert.Equal(t, pj.ID.String(), detail.ID)
}

func TestGetDetail_NotFound(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.penjualan = nil

	svc := newTestService(repo)
	_, err := svc.GetDetail(context.Background(), uuid.New())
	require.Error(t, err)
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeNotFound, de.Code())
}

// ─── GetList ─────────────────────────────────────────────────────────────────

func TestGetList_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	pj := pendingPenjualan(testMakerID, KlasifikasiAC, DisposalPartial)
	repo.listRows = []*Penjualan{pj}

	svc := newTestService(repo)
	rows, hasMore, total, err := svc.GetList(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.False(t, hasMore)
	assert.Equal(t, 1, total)
}

func TestGetList_DefaultLimit(t *testing.T) {
	repo := newDefaultStubRepo()
	svc := newTestService(repo)
	_, _, _, err := svc.GetList(context.Background(), listquery.Query{}, "", 0) // invalid limit → 50
	require.NoError(t, err)
}

// ─── ListBMAlerts ─────────────────────────────────────────────────────────────

func TestListBMAlerts_HappyPath(t *testing.T) {
	repo := newDefaultStubRepo()
	repo.bmAlerts = []*BMAlertItem{
		{
			InstrumenID:          testInstrumenID.String(),
			InstrumenKode:        "OBL-001",
			CumulativeSold12mPct: "6.5000",
			FlagStatus:           "BM_VIOLATION_RISK",
		},
	}

	svc := newTestService(repo)
	alerts, err := svc.ListBMAlerts(context.Background())
	require.NoError(t, err)
	assert.Len(t, alerts, 1)
	assert.Equal(t, "OBL-001", alerts[0].InstrumenKode)
}
