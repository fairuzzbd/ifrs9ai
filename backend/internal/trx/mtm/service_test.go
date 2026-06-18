package mtm

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Stub Repository ─────────────────────────────────────────────────────────

type stubRepo struct {
	mtm          *Mtm
	mtmList      []*Mtm
	batch        *UploadBatch
	batchRows    []*Mtm
	staleRows    []*Mtm
	activeInstr  []InstrumenInfo
	feedPrice    *FeedPrice
	kurs         *KursSnapshot
	hargaBukuIdr *decimal.Decimal
	configValues map[string]string
	insertErr    error
	updateErr    error
	existsActive bool
	existsMtm    *Mtm
	isHoliday    bool
}

func newStubRepo() *stubRepo {
	return &stubRepo{
		configValues: map[string]string{},
	}
}

func (r *stubRepo) Insert(_ context.Context, _ *sql.Tx, m *Mtm) error {
	if r.insertErr != nil {
		return r.insertErr
	}
	r.mtm = m
	return nil
}

func (r *stubRepo) GetByID(_ context.Context, _ uuid.UUID) (*Mtm, error) {
	return r.mtm, nil
}

func (r *stubRepo) UpdateStatus(_ context.Context, _ *sql.Tx, _ uuid.UUID, u StatusUpdate) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	if r.mtm != nil {
		r.mtm.Status = u.Status
	}
	return nil
}

func (r *stubRepo) ExistsActive(_ context.Context, _ uuid.UUID, _ time.Time, _ string) (bool, *Mtm, error) {
	return r.existsActive, r.existsMtm, nil
}

func (r *stubRepo) List(_ context.Context, _ listquery.Query, _ string, _ int) ([]*Mtm, bool, int, error) {
	return r.mtmList, false, len(r.mtmList), nil
}

func (r *stubRepo) ListByBatchID(_ context.Context, _ uuid.UUID) ([]*Mtm, error) {
	return r.batchRows, nil
}

func (r *stubRepo) ListStaleAlerts(_ context.Context, _ string, limit int) ([]*Mtm, bool, int, error) {
	if len(r.staleRows) > limit {
		return r.staleRows[:limit], true, limit, nil
	}
	return r.staleRows, false, len(r.staleRows), nil
}

func (r *stubRepo) LockMtmForPeriode(_ context.Context, _ *sql.Tx, _ uuid.UUID, _, _ time.Time, _ uuid.UUID) (int64, error) {
	return 5, nil
}

func (r *stubRepo) UnlockMtmForPeriode(_ context.Context, _ *sql.Tx, _ uuid.UUID, _, _ time.Time, _ uuid.UUID) (int64, error) {
	return 5, nil
}

func (r *stubRepo) BeginTx(_ context.Context) (*sql.Tx, error) {
	// Return nil tx — all repo methods accept nil tx in stub
	return nil, nil
}

func (r *stubRepo) GetConfigValue(_ context.Context, key string) (string, error) {
	return r.configValues[key], nil
}

func (r *stubRepo) IsHoliday(_ context.Context, _ time.Time) (bool, error) {
	return r.isHoliday, nil
}

func (r *stubRepo) GetActiveNonACInstrumen(_ context.Context) ([]InstrumenInfo, error) {
	return r.activeInstr, nil
}

func (r *stubRepo) GetFeedPrice(_ context.Context, _ uuid.UUID, _ string, _ time.Time) (*FeedPrice, error) {
	return r.feedPrice, nil
}

func (r *stubRepo) GetApprovedKurs(_ context.Context, _ string, _ time.Time) (*KursSnapshot, error) {
	return r.kurs, nil
}

func (r *stubRepo) GetHargaBukuIdr(_ context.Context, _ uuid.UUID) (*decimal.Decimal, error) {
	return r.hargaBukuIdr, nil
}

func (r *stubRepo) InsertUploadBatch(_ context.Context, _ *sql.Tx, b *UploadBatch) error {
	r.batch = b
	return nil
}

func (r *stubRepo) GetUploadBatch(_ context.Context, _ uuid.UUID) (*UploadBatch, error) {
	return r.batch, nil
}

// Ensure stubRepo satisfies Repository
var _ Repository = (*stubRepo)(nil)

// ─── Test helpers ─────────────────────────────────────────────────────────────

func newTestService(repo Repository) *Service {
	poster := NewJurnalPosterStub(slog.Default())
	return NewService(repo, poster, nil, slog.Default())
}

func makeMtm(status Status) *Mtm {
	uploader := uuid.New()
	return &Mtm{
		ID:                  uuid.New(),
		InstrumenID:         uuid.New(),
		PeriodeBulananID:    uuid.New(),
		TanggalMtm:          time.Now(),
		HargaSumber:         HargaSumberManual,
		HargaTanggal:        time.Now(),
		HargaAgeDays:        1,
		HargaPasarIdr:       decimal.NewFromFloat(100),
		HargaBukuIdr:        decimal.NewFromFloat(95),
		DeltaIdr:            decimal.NewFromFloat(5),
		DeltaPct:            decimal.NewFromFloat(5.26),
		KlasifikasiSnapshot: KlasifikasiFVTPL,
		StalePriceFlag:      false,
		DeviationFlag:       true,
		LockedFlag:          false,
		Status:              status,
		UploaderID:          &uploader,
		CreatedAt:           time.Now(),
		CreatedBy:           uuid.New(),
		UpdatedAt:           time.Now(),
		UpdatedBy:           uuid.New(),
		RowVersion:          1,
		TenantID:            "TUGURE",
	}
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

func TestService_GetDetail_Found(t *testing.T) {
	m := makeMtm(StatusAutoPOSTED)
	repo := newStubRepo()
	repo.mtm = m
	svc := newTestService(repo)

	result, err := svc.GetDetail(context.Background(), m.ID)
	require.NoError(t, err)
	assert.Equal(t, m.ID, result.ID)
}

func TestService_GetDetail_NotFound(t *testing.T) {
	repo := newStubRepo()
	repo.mtm = nil
	svc := newTestService(repo)

	_, err := svc.GetDetail(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

// ─── GetList ─────────────────────────────────────────────────────────────────

func TestService_GetList_ReturnsRows(t *testing.T) {
	repo := newStubRepo()
	repo.mtmList = []*Mtm{makeMtm(StatusAutoPOSTED), makeMtm(StatusPendingReview)}
	svc := newTestService(repo)

	rows, hasMore, total, err := svc.GetList(context.Background(), listquery.Query{}, "", 50)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
	assert.False(t, hasMore)
	assert.Equal(t, 2, total)
}

func TestService_GetList_ZeroLimit_DefaultsTo50(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)
	_, _, _, err := svc.GetList(context.Background(), listquery.Query{}, "", 0)
	assert.NoError(t, err)
}

// ─── GetUploadBatch ───────────────────────────────────────────────────────────

func TestService_GetUploadBatch_NotFound(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)

	_, err := svc.GetUploadBatch(context.Background(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestService_GetUploadBatch_Found(t *testing.T) {
	repo := newStubRepo()
	bID := uuid.New()
	repo.batch = &UploadBatch{
		ID:         bID,
		BatchType:  "MTM_UPLOAD",
		Status:     "PENDING_REVIEW",
		UploaderID: uuid.New(),
		TotalRows:  2,
		ValidRows:  2,
		CreatedAt:  time.Now(),
		CreatedBy:  uuid.New(),
	}
	repo.batchRows = []*Mtm{makeMtm(StatusPendingReview)}
	svc := newTestService(repo)

	detail, err := svc.GetUploadBatch(context.Background(), bID)
	require.NoError(t, err)
	assert.Equal(t, bID.String(), detail.UploadBatchID)
	assert.Len(t, detail.Rows, 1)
}

// ─── GetStalePriceAlerts ──────────────────────────────────────────────────────

func TestService_GetStalePriceAlerts_Empty(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)

	items, hasMore, total, err := svc.GetStalePriceAlerts(context.Background(), "", 50)
	require.NoError(t, err)
	assert.Nil(t, items)
	assert.False(t, hasMore)
	assert.Equal(t, 0, total)
}

func TestService_GetStalePriceAlerts_HasRows(t *testing.T) {
	repo := newStubRepo()
	stale := makeMtm(StatusStalePrice)
	stale.StalePriceFlag = true
	stale.HargaAgeDays = 8
	repo.staleRows = []*Mtm{stale}
	svc := newTestService(repo)

	items, hasMore, _, err := svc.GetStalePriceAlerts(context.Background(), "", 50)
	require.NoError(t, err)
	assert.Len(t, items, 1)
	assert.False(t, hasMore)
	assert.True(t, items[0].EskalasiFag, "age=8 > default escalation=7 → flag true")
}

// ─── OverrideApprove ─────────────────────────────────────────────────────────

func makeClaimsCtx(userID string) context.Context {
	return context.Background()
	// NOTE: real claims injection needs auth.ContextWithClaims; stub returns nil claims
	// so OverrideApprove will return ErrUnauthorized in unit tests.
	// Integration tests (handler_test.go) inject claims via gin.Context middleware.
}

func TestService_OverrideApprove_NilClaims_Unauthorized(t *testing.T) {
	repo := newStubRepo()
	repo.mtm = makeMtm(StatusPendingReview)
	svc := newTestService(repo)

	_, err := svc.OverrideApprove(context.Background(), repo.mtm.ID, OverrideApproveRequest{
		Comment: "This override is justified because the price reflects the true market value today.",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestService_OverrideApprove_NotFound(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)

	_, err := svc.OverrideApprove(context.Background(), uuid.New(), OverrideApproveRequest{
		Comment: "Justified override comment here with enough length.",
	})
	// Will error on nil claims first — expected
	require.Error(t, err)
}

func TestService_OverrideReject_NilClaims_Unauthorized(t *testing.T) {
	repo := newStubRepo()
	repo.mtm = makeMtm(StatusPendingReview)
	svc := newTestService(repo)

	_, err := svc.OverrideReject(context.Background(), repo.mtm.ID, OverrideRejectRequest{
		Comment: "Rejection comment must be at least 30 characters long per spec.",
	})
	require.Error(t, err)
}

// ─── ProcessOneInstrument ─────────────────────────────────────────────────────

func TestService_ProcessOneInstrument_ACSkip(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)

	acInst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiAC,
		MataUang:          "IDR",
		TipeInstrumen:     "OBLIGASI",
	}
	_, err := svc.ProcessOneInstrument(context.Background(), acInst, time.Now(), "job-123")
	require.Error(t, err)
	assert.True(t, isACSkip(err))
}

func TestService_ProcessOneInstrument_AlreadyExists_ReturnsExisting(t *testing.T) {
	repo := newStubRepo()
	existing := makeMtm(StatusAutoPOSTED)
	repo.existsActive = true
	repo.existsMtm = existing
	svc := newTestService(repo)

	inst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVTPL,
		MataUang:          "IDR",
		TipeInstrumen:     "SAHAM",
	}
	result, err := svc.ProcessOneInstrument(context.Background(), inst, time.Now(), "job-123")
	require.NoError(t, err)
	assert.Equal(t, existing.ID, result.ID, "should return existing when already processed")
}

func TestService_ProcessOneInstrument_NoFeedPrice_StalePrice(t *testing.T) {
	repo := newStubRepo()
	repo.feedPrice = nil // no price in feed
	svc := newTestService(repo)

	inst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVTPL,
		MataUang:          "IDR",
		TipeInstrumen:     "SAHAM",
	}
	result, err := svc.ProcessOneInstrument(context.Background(), inst, time.Now(), "job-456")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusStalePrice, result.Status)
	assert.True(t, result.StalePriceFlag)
}

func TestService_ProcessOneInstrument_DeviationExceeded_PendingReview(t *testing.T) {
	repo := newStubRepo()
	// Fresh price, large deviation
	repo.feedPrice = &FeedPrice{
		InstrumenID:  uuid.New(),
		HargaPasar:   decimal.NewFromFloat(200), // 100% up from book 100
		HargaTanggal: time.Now(),
		MataUang:     "IDR",
	}
	bv := decimal.NewFromFloat(100)
	repo.hargaBukuIdr = &bv
	// staleDays = 5; age = 0 → not stale
	// deviation = 100% > 5% threshold → PENDING_REVIEW
	svc := newTestService(repo)

	inst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVTPL,
		MataUang:          "IDR",
		TipeInstrumen:     "SAHAM",
	}
	result, err := svc.ProcessOneInstrument(context.Background(), inst, time.Now(), "job-789")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusPendingReview, result.Status)
	assert.True(t, result.DeviationFlag)
}

func TestService_ProcessOneInstrument_FVOCIDebt_IDR_AutoPosted(t *testing.T) {
	repo := newStubRepo()
	// Fresh price, small deviation → AUTO_POSTED
	repo.feedPrice = &FeedPrice{
		InstrumenID:  uuid.New(),
		HargaPasar:   decimal.NewFromFloat(101),
		HargaTanggal: time.Now(),
		MataUang:     "IDR",
	}
	bv := decimal.NewFromFloat(100)
	repo.hargaBukuIdr = &bv
	svc := newTestService(repo)

	poster := svc.poster.(*JurnalPosterStub)
	inst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVOCIDebt,
		MataUang:          "IDR",
		TipeInstrumen:     "OBLIGASI",
	}
	result, err := svc.ProcessOneInstrument(context.Background(), inst, time.Now(), "job-001")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusAutoPOSTED, result.Status)
	// Expect exactly 1 jurnal post (IDR → single MTM_FVOCI)
	calls := poster.Calls()
	assert.Len(t, calls, 1)
	assert.Equal(t, EventCodeMTMFVOCI, calls[0].EventCode)
}

func TestService_ProcessOneInstrument_FVOCIDebt_FCY_AutoPosted_TwoEntries(t *testing.T) {
	repo := newStubRepo()
	repo.feedPrice = &FeedPrice{
		InstrumenID:  uuid.New(),
		HargaPasar:   decimal.NewFromFloat(1000),
		HargaTanggal: time.Now(),
		MataUang:     "USD",
	}
	repo.kurs = &KursSnapshot{
		KursID:       uuid.New(),
		KodeMataUang: "USD",
		KursTengah:   decimal.NewFromFloat(15000),
	}
	bv := decimal.NewFromFloat(15_000_000) // 1000 USD × 15000 IDR/USD
	repo.hargaBukuIdr = &bv
	svc := newTestService(repo)

	poster := svc.poster.(*JurnalPosterStub)
	inst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVOCIDebt,
		MataUang:          "USD",
		TipeInstrumen:     "OBLIGASI",
	}
	result, err := svc.ProcessOneInstrument(context.Background(), inst, time.Now(), "job-002")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusAutoPOSTED, result.Status)
	// §B5.7.2A: TWO jurnal entries for FCY FVOCI_DEBT
	calls := poster.Calls()
	assert.Len(t, calls, 2)
	assert.Equal(t, EventCodeMTMFVOCI, calls[0].EventCode)
	assert.Equal(t, EventCodeMTMFXOCIReserve, calls[1].EventCode)
}

func TestService_ProcessOneInstrument_FCY_NoKurs_StalePrice(t *testing.T) {
	repo := newStubRepo()
	repo.feedPrice = &FeedPrice{
		InstrumenID:  uuid.New(),
		HargaPasar:   decimal.NewFromFloat(1000),
		HargaTanggal: time.Now(),
		MataUang:     "USD",
	}
	repo.kurs = nil // no approved kurs → STALE_PRICE
	svc := newTestService(repo)

	inst := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVTPL,
		MataUang:          "USD",
		TipeInstrumen:     "SAHAM",
	}
	result, err := svc.ProcessOneInstrument(context.Background(), inst, time.Now(), "job-003")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, StatusStalePrice, result.Status)
}

// ─── LockMtmForPeriode / UnlockMtmForPeriode ─────────────────────────────────

func TestService_LockMtmForPeriode_InvalidCtx(t *testing.T) {
	svc := newTestService(newStubRepo())
	err := svc.LockMtmForPeriode("not-a-context", nil, uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context.Context")
}

func TestService_LockMtmForPeriode_InvalidTx(t *testing.T) {
	svc := newTestService(newStubRepo())
	err := svc.LockMtmForPeriode(context.Background(), "not-a-tx", uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "*sql.Tx")
}

func TestService_LockMtmForPeriode_NilTx_CallsRepo(t *testing.T) {
	svc := newTestService(newStubRepo())
	err := svc.LockMtmForPeriode(context.Background(), (*sql.Tx)(nil), uuid.New(), uuid.New())
	// stubRepo.LockMtmForPeriode always succeeds
	assert.NoError(t, err)
}

func TestService_UnlockMtmForPeriode_NilTx_CallsRepo(t *testing.T) {
	svc := newTestService(newStubRepo())
	err := svc.UnlockMtmForPeriode(context.Background(), (*sql.Tx)(nil), uuid.New(), uuid.New())
	assert.NoError(t, err)
}

// ─── TriggerCron ─────────────────────────────────────────────────────────────

type stubEnqueuer struct {
	calls []string
	err   error
}

func (e *stubEnqueuer) Enqueue(_ context.Context, taskType string, _ interface{}) error {
	e.calls = append(e.calls, taskType)
	return e.err
}

func TestService_TriggerCron_EnqueuesTask(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)
	eq := &stubEnqueuer{}

	result, err := svc.TriggerCron(context.Background(), eq, CronTriggerRequest{
		TanggalTarget: "2026-06-10",
	})
	require.NoError(t, err)
	assert.Equal(t, TaskMtmDailyRun, eq.calls[0])
	assert.Equal(t, "2026-06-10", result.TanggalTarget)
	assert.NotEmpty(t, result.JobID)
}

func TestService_TriggerCron_InvalidDate(t *testing.T) {
	svc := newTestService(newStubRepo())
	eq := &stubEnqueuer{}

	_, err := svc.TriggerCron(context.Background(), eq, CronTriggerRequest{
		TanggalTarget: "not-a-date",
	})
	require.Error(t, err)
	assert.Len(t, eq.calls, 0)
}

func TestService_TriggerCron_EnqueueError(t *testing.T) {
	svc := newTestService(newStubRepo())
	eq := &stubEnqueuer{err: errors.New("redis down")}

	_, err := svc.TriggerCron(context.Background(), eq, CronTriggerRequest{
		TanggalTarget: "2026-06-10",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enqueue")
}

func TestService_TriggerCron_EmptyDateDefaults(t *testing.T) {
	svc := newTestService(newStubRepo())
	eq := &stubEnqueuer{}

	result, err := svc.TriggerCron(context.Background(), eq, CronTriggerRequest{})
	require.NoError(t, err)
	// Should default to today's date (format YYYY-MM-DD)
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, result.TanggalTarget)
}

// ─── UploadManual ─────────────────────────────────────────────────────────────

func TestService_UploadManual_EmptyRows(t *testing.T) {
	svc := newTestService(newStubRepo())

	result, err := svc.UploadManual(context.Background(), uuid.New(), nil, "test upload")
	require.NoError(t, err)
	assert.Equal(t, 0, result.RowsParsed)
}

func TestService_UploadManual_ValidRow(t *testing.T) {
	svc := newTestService(newStubRepo())

	rows := []UploadFileRow{
		{
			LineNumber:    1,
			KodeInstrumen: "OBL-001",
			TanggalMtm:    "2026-06-10",
			HargaPasar:    "100.5000",
			HargaSumber:   "MANUAL",
		},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowsParsed)
	assert.Equal(t, 1, result.RowsValid)
}

func TestService_UploadManual_InvalidRow_NegativePrice(t *testing.T) {
	svc := newTestService(newStubRepo())

	rows := []UploadFileRow{
		{
			LineNumber:    1,
			KodeInstrumen: "OBL-001",
			TanggalMtm:    "2026-06-10",
			HargaPasar:    "-50",
			HargaSumber:   "MANUAL",
		},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowsParsed)
	assert.Equal(t, 0, result.RowsValid)
	assert.Equal(t, 1, result.RowsInvalid)
}

func TestService_UploadManual_InvalidRow_BadDate(t *testing.T) {
	svc := newTestService(newStubRepo())

	rows := []UploadFileRow{
		{
			LineNumber:    1,
			KodeInstrumen: "OBL-001",
			TanggalMtm:    "2026-13-01", // invalid month
			HargaPasar:    "100",
			HargaSumber:   "MANUAL",
		},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 0, result.RowsValid)
}
