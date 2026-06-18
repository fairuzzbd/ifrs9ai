package mtm

// coverage_boost_test.go — additional tests to reach ≥85% overall coverage.
// Targets: service.OverrideApprove, service.OverrideReject, domain.ToDetail/ToListItem,
//          jurnal_poster helpers, domain errors.

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeClaimsContext(userID string) context.Context {
	claims := &auth.Claims{
		Sub:         userID,
		Permissions: []string{"mtm.read", "mtm.create", "mtm.override", "mtm.trigger"},
		TenantID:    "TUGURE",
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

func makePendingMtm() *Mtm {
	uploaderID := uuid.New()
	m := makeMtm(StatusPendingReview)
	m.UploaderID = &uploaderID
	m.DeviationFlag = true
	m.LockedFlag = false
	m.MataUang = "IDR" // B2 fix: required non-empty for OverrideApprove routing
	return m
}

// ─── OverrideApprove ─────────────────────────────────────────────────────────

func TestService_OverrideApprove_Success(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	// Use a different user as approver (not the uploader)
	approverID := uuid.New().String()
	ctx := makeClaimsContext(approverID)

	result, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment: "Override justified: Bloomberg price confirmed by phone with dealer.",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "APPROVED", result.Status)
}

func TestService_OverrideApprove_ShortComment_Rejected(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{Comment: "too short"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30")
}

func TestService_OverrideApprove_LockedFlag_Rejected(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	m.LockedFlag = true
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment: "Override justified: Bloomberg price confirmed by phone with dealer.",
	})
	require.Error(t, err)
	assert.NotNil(t, err)
}

func TestService_OverrideApprove_SOD_Uploader_Is_Approver(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	// Set approver == uploader
	approverID, _ := uuid.Parse(uuid.New().String())
	approverStr := approverID.String()
	m.UploaderID = &approverID
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(approverStr)
	_, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment: "Override justified: Bloomberg price confirmed by phone with dealer.",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}

func TestService_OverrideApprove_WrongStatus_AutoPosted_Rejected(t *testing.T) {
	repo := newStubRepo()
	m := makeMtm(StatusAutoPOSTED) // cannot override AUTO_POSTED
	m.LockedFlag = false
	uploaderID := uuid.New()
	m.UploaderID = &uploaderID
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment: "Override justified: Bloomberg price confirmed by phone with dealer.",
	})
	require.Error(t, err)
	// WorkflowInvalidTransition — error contains the from/to state
	assert.Contains(t, err.Error(), "AUTO_POSTED")
}

func TestService_OverrideApprove_MTMNotFound(t *testing.T) {
	repo := newStubRepo() // mtm = nil
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideApprove(ctx, uuid.New(), OverrideApproveRequest{
		Comment: "Override justified: Bloomberg price confirmed by phone with dealer.",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestService_OverrideApprove_FVOCIDebt_FCY_TwoJurnalEntries(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	m.KlasifikasiSnapshot = KlasifikasiFVOCIDebt
	m.MataUang = "USD" // B2 fix: OverrideApprove uses m.MataUang (not hardcoded "IDR")
	repo.mtm = m
	svc := newTestService(repo)
	poster := svc.poster.(*JurnalPosterStub)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment: "Override justified: Bloomberg price confirmed by phone with dealer.",
	})
	// B2 fix: FVOCI_DEBT FCY → 2 entries (MTM_FVOCI + MTM_FX_OCI_RESERVE)
	require.NoError(t, err)
	calls := poster.Calls()
	assert.Len(t, calls, 2, "FVOCI_DEBT FCY must produce 2 jurnal entries (§B5.7.2A)")
	assert.Equal(t, EventCodeMTMFVOCI, calls[0].EventCode)
	assert.Equal(t, EventCodeMTMFXOCIReserve, calls[1].EventCode)
}

// ─── OverrideReject ───────────────────────────────────────────────────────────

func TestService_OverrideReject_Success(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	result, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "REJECTED", result.Status)
}

func TestService_OverrideReject_NilClaims(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	_, err := svc.OverrideReject(context.Background(), m.ID, OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tidak ditemukan")
}

func TestService_OverrideReject_LockedFlag(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	m.LockedFlag = true
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	require.Error(t, err)
}

func TestService_OverrideReject_ShortComment(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{Comment: "short"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30")
}

func TestService_OverrideReject_WrongStatus(t *testing.T) {
	repo := newStubRepo()
	m := makeMtm(StatusApproved) // cannot reject APPROVED
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	require.Error(t, err)
}

func TestService_OverrideReject_NotFound(t *testing.T) {
	repo := newStubRepo()
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideReject(ctx, uuid.New(), OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	require.Error(t, err)
}

// ─── domain.ToDetail / ToListItem ────────────────────────────────────────────

func TestToListItem_Basic(t *testing.T) {
	m := makeMtm(StatusAutoPOSTED)
	item := ToListItem(m)
	assert.Equal(t, m.ID.String(), item.ID)
	assert.Equal(t, string(StatusAutoPOSTED), item.Status)
}

func TestToListItem_WithOptionalFields(t *testing.T) {
	m := makeMtm(StatusPendingReview)
	kursID := uuid.New()
	m.KursID = &kursID
	kurs := decimal.NewFromFloat(15000)
	m.KursTengah = &kurs
	jEntryID := uuid.New()
	m.JurnalEntryID = &jEntryID
	ec := "MTM_FVOCI"
	m.JurnalEventCode = &ec
	item := ToListItem(m)
	require.NotNil(t, item.JurnalEntryID)
	require.NotNil(t, item.JurnalEventCode)
}

func TestToDetail_WithOptionals(t *testing.T) {
	m := makeMtm(StatusApproved)
	approverID := uuid.New()
	m.OverrideApproverID = &approverID
	comment := "Approved by risk team"
	m.OverrideComment = &comment
	now := time.Now()
	m.OverrideAt = &now
	m.JurnalEntryID2 = &uuid.UUID{}
	ec2 := "MTM_FX_OCI_RESERVE"
	m.JurnalEventCode2 = &ec2
	jEntryID := uuid.New()
	m.JurnalEntryID = &jEntryID
	ec := "MTM_FVOCI"
	m.JurnalEventCode = &ec

	detail := ToDetail(m)
	assert.Equal(t, m.ID.String(), detail.ID)
	assert.Equal(t, string(StatusApproved), detail.Status)
	require.NotNil(t, detail.OverrideApproverID)
	require.NotNil(t, detail.OverrideComment)
	require.NotNil(t, detail.JurnalEntryID)
	// JurnalEntryID2 is folded into JurnalEventCodes in Detail
	assert.NotEmpty(t, detail.JurnalEventCodes)
}

func TestToDetail_NilOptionals(t *testing.T) {
	m := makeMtm(StatusStalePrice)
	detail := ToDetail(m)
	assert.Nil(t, detail.OverrideApproverID)
	assert.Nil(t, detail.KursID)
	assert.Nil(t, detail.JurnalEntryID)
}

// ─── domain errors ───────────────────────────────────────────────────────────

func TestMtmErr_Error(t *testing.T) {
	assert.Contains(t, ErrMTMInstrumenACSkip.Error(), "AC")
}

func TestMtmErr_ACSkip_HTTPStatus(t *testing.T) {
	// newMTMErr for CodeMTMInstrumenACSkip uses the default fallback → VALIDATION_FAILED → 400.
	// This is intentional — AC skip is a client-side validation error.
	status := ErrMTMInstrumenACSkip.HTTPStatus()
	assert.True(t, status >= 400 && status < 500, "expect 4xx status, got %d", status)
}

func TestMtmErr_Locked_HTTPStatus(t *testing.T) {
	assert.Equal(t, 423, ErrMTMPeriodeLocked.HTTPStatus())
}

func TestMtmErr_SOD_HTTPStatus(t *testing.T) {
	assert.Equal(t, 403, ErrMTMOverrideSODViolation.HTTPStatus())
}

func TestMtmErr_AllMessagesNonEmpty(t *testing.T) {
	assert.NotEmpty(t, ErrMTMInstrumenACSkip.Error())
	assert.NotEmpty(t, ErrMTMPeriodeLocked.Error())
	assert.NotEmpty(t, ErrMTMOverrideSODViolation.Error())
}

// ─── JurnalPosterStub helpers ────────────────────────────────────────────────

func TestJurnalPosterStub_SetError(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetError(nil)
	result, err := stub.Post(context.Background(), (*sql.Tx)(nil), PostRequest{
		EventCode: EventCodeMTMFVTPL,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JurnalEntryID)
}

func TestJurnalPosterStub_SetResult_Override(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	// Post always generates a new UUID — SetResult stores it but Post ignores it.
	// Test just verifies no panic and no error.
	stub.SetResult(PostResult{EventCode: "MTM_TEST"})
	result, err := stub.Post(context.Background(), (*sql.Tx)(nil), PostRequest{EventCode: "MTM_TEST"})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JurnalEntryID)
}

func TestJurnalPosterStub_SetError_Causes_PostError(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetError(assert.AnError)
	_, err := stub.Post(context.Background(), (*sql.Tx)(nil), PostRequest{EventCode: EventCodeMTMFVTPL})
	require.Error(t, err)
}

func TestJurnalPosterStub_Reset(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	_, _ = stub.Post(context.Background(), (*sql.Tx)(nil), PostRequest{EventCode: EventCodeMTMFVTPL})
	assert.Len(t, stub.Calls(), 1)
	stub.Reset()
	assert.Len(t, stub.Calls(), 0)
}

func TestNoopJurnalPoster_Post(t *testing.T) {
	poster := NewNoopJurnalPoster(nil)
	result, err := poster.Post(context.Background(), (*sql.Tx)(nil), PostRequest{EventCode: EventCodeMTMFVTPL})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JurnalEntryID)
}

func TestNewRealJurnalPoster_ReturnsNoop(t *testing.T) {
	poster := NewRealJurnalPoster(slog.Default())
	assert.NotNil(t, poster)
}

// ─── domain.Status.CanOverride ────────────────────────────────────────────────

func TestStatus_CanOverride_AllStatuses(t *testing.T) {
	// m3 fix: CanOverride no longer allows STALE_PRICE (must use CanReject for that).
	cases := []struct {
		s    Status
		want bool
	}{
		{StatusPendingReview, true},
		{StatusStalePrice, false},  // m3 fix: STALE_PRICE uses CanReject, not CanOverride
		{StatusAutoPOSTED, false},
		{StatusApproved, false},
		{StatusRejected, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.s), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.s.CanOverride())
		})
	}
}

func TestStatus_CanReject_AllStatuses(t *testing.T) {
	// m3 fix: CanReject allows both PENDING_REVIEW and STALE_PRICE.
	cases := []struct {
		s    Status
		want bool
	}{
		{StatusPendingReview, true},
		{StatusStalePrice, true},  // m3 fix: STALE_PRICE can be rejected
		{StatusAutoPOSTED, false},
		{StatusApproved, false},
		{StatusRejected, false},
	}
	for _, tc := range cases {
		t.Run(string(tc.s), func(t *testing.T) {
			assert.Equal(t, tc.want, tc.s.CanReject())
		})
	}
}

// ─── HargaSumber whitelist ────────────────────────────────────────────────────

func TestIsValidHargaSumber(t *testing.T) {
	assert.True(t, IsValidHargaSumber(string(HargaSumberIBPA)))
	assert.True(t, IsValidHargaSumber(string(HargaSumberBEI)))
	assert.True(t, IsValidHargaSumber(string(HargaSumberKSEI)))
	assert.True(t, IsValidHargaSumber(string(HargaSumberManual)))
	assert.False(t, IsValidHargaSumber("UNKNOWN"))
	assert.False(t, IsValidHargaSumber(""))
}

// ─── configInt / configFloat ──────────────────────────────────────────────────

func TestService_ConfigInt_NotFound_ReturnsDefault(t *testing.T) {
	svc := newTestService(newStubRepo())
	// configValues is empty → returns defaultVal
	val := svc.configInt(context.Background(), "MTM_PRICE_STALE_DAYS", DefaultStalePriceDays)
	assert.Equal(t, DefaultStalePriceDays, val)
}

func TestService_ConfigInt_Found_ParsedCorrectly(t *testing.T) {
	repo := newStubRepo()
	repo.configValues["MTM_PRICE_STALE_DAYS"] = "10"
	svc := newTestService(repo)
	val := svc.configInt(context.Background(), "MTM_PRICE_STALE_DAYS", DefaultStalePriceDays)
	assert.Equal(t, 10, val)
}

func TestService_ConfigFloat_NotFound_ReturnsDefault(t *testing.T) {
	svc := newTestService(newStubRepo())
	val := svc.configFloat(context.Background(), "MTM_PRICE_DEVIATION_THRESHOLD_PCT", DefaultDeviationThresholdPct)
	expected := decimal.NewFromFloat(DefaultDeviationThresholdPct)
	assert.True(t, expected.Equal(val), "expected %s got %s", expected, val)
}

func TestService_ConfigFloat_Found_ParsedCorrectly(t *testing.T) {
	repo := newStubRepo()
	repo.configValues["MTM_PRICE_DEVIATION_THRESHOLD_PCT"] = "3.5"
	svc := newTestService(repo)
	val := svc.configFloat(context.Background(), "MTM_PRICE_DEVIATION_THRESHOLD_PCT", DefaultDeviationThresholdPct)
	expected := decimal.NewFromFloat(3.5)
	assert.True(t, expected.Equal(val), "expected %s got %s", expected, val)
}

// ─── UnlockMtmForPeriode ─────────────────────────────────────────────────────

func TestService_UnlockMtmForPeriode_InvalidCtx(t *testing.T) {
	svc := newTestService(newStubRepo())
	err := svc.UnlockMtmForPeriode("not-ctx", nil, uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context.Context")
}

func TestService_UnlockMtmForPeriode_InvalidTx(t *testing.T) {
	svc := newTestService(newStubRepo())
	err := svc.UnlockMtmForPeriode(context.Background(), "bad-tx", uuid.New(), uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "*sql.Tx")
}

// ─── processUploadRow — edge cases ───────────────────────────────────────────

func TestService_UploadManual_InvalidHargaSumber(t *testing.T) {
	svc := newTestService(newStubRepo())
	rows := []UploadFileRow{
		{
			LineNumber:    1,
			KodeInstrumen: "OBL-001",
			TanggalMtm:    "2026-06-10",
			HargaPasar:    "100",
			HargaSumber:   "UNKNOWN_SOURCE",
		},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 0, result.RowsValid)
	assert.Equal(t, 1, result.RowsInvalid)
}

func TestService_UploadManual_InvalidHargaPasarNotNumber(t *testing.T) {
	svc := newTestService(newStubRepo())
	rows := []UploadFileRow{
		{
			LineNumber:    1,
			KodeInstrumen: "OBL-001",
			TanggalMtm:    "2026-06-10",
			HargaPasar:    "not-a-number",
			HargaSumber:   "MANUAL",
		},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 0, result.RowsValid)
}

func TestService_UploadManual_MultipleRows(t *testing.T) {
	svc := newTestService(newStubRepo())
	rows := []UploadFileRow{
		{LineNumber: 1, KodeInstrumen: "OBL-001", TanggalMtm: "2026-06-10", HargaPasar: "100", HargaSumber: "MANUAL"},
		{LineNumber: 2, KodeInstrumen: "OBL-002", TanggalMtm: "2026-06-10", HargaPasar: "200", HargaSumber: "IBPA"},
		{LineNumber: 3, KodeInstrumen: "OBL-003", TanggalMtm: "2026-06-10", HargaPasar: "-1", HargaSumber: "MANUAL"},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 3, result.RowsParsed)
	assert.Equal(t, 2, result.RowsValid)
	assert.Equal(t, 1, result.RowsInvalid)
}

// ─── StalePriceAlert response shape ──────────────────────────────────────────

func TestStalePriceAlertItem_EskalasiFlag(t *testing.T) {
	// age = 8 > escalation_days = 7 → eskalasi = true
	stale := makeMtm(StatusStalePrice)
	stale.StalePriceFlag = true
	stale.HargaAgeDays = int16(DefaultStaleEscalationDays + 1)
	repo := newStubRepo()
	repo.staleRows = []*Mtm{stale}
	svc := newTestService(repo)

	items, _, _, err := svc.GetStalePriceAlerts(context.Background(), "TUGURE", 50)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.True(t, items[0].EskalasiFag)
}

func TestStalePriceAlertItem_NoEskalasiFlag(t *testing.T) {
	stale := makeMtm(StatusStalePrice)
	stale.StalePriceFlag = true
	stale.HargaAgeDays = int16(DefaultStalePriceDays + 1) // > 5 but ≤ 7 → no eskalasi
	repo := newStubRepo()
	repo.staleRows = []*Mtm{stale}
	svc := newTestService(repo)

	items, _, _, err := svc.GetStalePriceAlerts(context.Background(), "TUGURE", 50)
	require.NoError(t, err)
	assert.False(t, items[0].EskalasiFag)
}

// ─── validatePostRequest ──────────────────────────────────────────────────────

func TestValidatePostRequest_Valid(t *testing.T) {
	err := validatePostRequest(PostRequest{
		EventCode:   EventCodeMTMFVTPL,
		InstrumenID: uuid.New(),
		MtmID:       uuid.New(),
	})
	require.NoError(t, err)
}

func TestValidatePostRequest_EmptyEventCode(t *testing.T) {
	err := validatePostRequest(PostRequest{
		EventCode:   "",
		InstrumenID: uuid.New(),
		MtmID:       uuid.New(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EventCode")
}

func TestValidatePostRequest_NilInstrumenID(t *testing.T) {
	err := validatePostRequest(PostRequest{
		EventCode:   EventCodeMTMFVTPL,
		InstrumenID: uuid.Nil,
		MtmID:       uuid.New(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "InstrumenID")
}

func TestValidatePostRequest_NilMtmID(t *testing.T) {
	err := validatePostRequest(PostRequest{
		EventCode:   EventCodeMTMFVTPL,
		InstrumenID: uuid.New(),
		MtmID:       uuid.Nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MtmID")
}

// ─── service.GetUploadBatch error path ───────────────────────────────────────

type errorRepo struct {
	*stubRepo
}

func (r *errorRepo) GetUploadBatch(_ context.Context, _ uuid.UUID) (*UploadBatch, error) {
	return nil, fmt.Errorf("db error")
}

func TestService_GetUploadBatch_RepoError(t *testing.T) {
	svc := newTestService(&errorRepo{newStubRepo()})
	_, err := svc.GetUploadBatch(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── domain.ToListItem full optional field coverage ──────────────────────────

func TestToListItem_AllOptionals(t *testing.T) {
	m := makeMtm(StatusApproved)
	approverID := uuid.New()
	m.OverrideApproverID = &approverID
	now := time.Now()
	m.OverrideAt = &now
	jID := uuid.New()
	m.JurnalEntryID = &jID
	ec := EventCodeMTMFVTPL
	m.JurnalEventCode = &ec
	uploaderID := uuid.New()
	m.UploaderID = &uploaderID

	item := ToListItem(m)
	require.NotNil(t, item.OverrideApproverID)
	require.NotNil(t, item.OverrideAt)
	require.NotNil(t, item.JurnalEntryID)
	require.NotNil(t, item.JurnalEventCode)
	require.NotNil(t, item.UploaderID)
}

// ─── domain.ToDetail full optional field coverage ────────────────────────────

func TestToDetail_AllOptionals(t *testing.T) {
	m := makeMtm(StatusApproved)
	approverID := uuid.New()
	m.OverrideApproverID = &approverID
	comment := "Approved after review"
	m.OverrideComment = &comment
	now := time.Now()
	m.OverrideAt = &now
	jID := uuid.New()
	m.JurnalEntryID = &jID
	ec := EventCodeMTMFVTPL
	m.JurnalEventCode = &ec
	jID2 := uuid.New()
	m.JurnalEntryID2 = &jID2
	ec2 := EventCodeMTMFXOCIReserve
	m.JurnalEventCode2 = &ec2
	uploaderID := uuid.New()
	m.UploaderID = &uploaderID
	batchID := uuid.New()
	m.UploadBatchID = &batchID
	cronJobID := "job-abc"
	m.CronJobID = &cronJobID
	fcy := decimal.NewFromFloat(1000)
	m.HargaPasarFcy = &fcy

	detail := ToDetail(m)
	require.NotNil(t, detail.OverrideApproverID)
	require.NotNil(t, detail.OverrideComment)
	require.NotNil(t, detail.JurnalEntryID)
	require.NotNil(t, detail.UploaderID)
	require.NotNil(t, detail.UploadBatchID)
	require.NotNil(t, detail.CronJobID)
	require.NotNil(t, detail.HargaPasarFcy)
	assert.Len(t, detail.JurnalEventCodes, 2)
}

// ─── newMTMErr fallback (default case) ───────────────────────────────────────

func TestNewMTMErr_FallbackToValidation(t *testing.T) {
	// CodeMTMBatchNotFound → no switch case → falls through to default
	err := newMTMErr(CodeMTMBatchNotFound, "batch not found")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "batch not found")
}

// ─── NewDailyRunTask (domain.go:475) — 0% before this test ──────────────────

func TestNewDailyRunTask_HappyPath(t *testing.T) {
	task, err := NewDailyRunTask("2026-06-18", "TUGURE")
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.Equal(t, TaskMtmDailyRun, task.Type())
	// Payload must be non-empty JSON
	require.NotEmpty(t, task.Payload())
}

func TestNewDailyRunTask_EmptyFields(t *testing.T) {
	// Even empty strings must marshal without error (JSON handles them fine)
	task, err := NewDailyRunTask("", "")
	require.NoError(t, err)
	require.NotNil(t, task)
}

// ─── NoopEnqueuer (service.go:974) — 0% before this test ────────────────────

func TestNoopEnqueuer_ReturnsError(t *testing.T) {
	n := NoopEnqueuer{}
	err := n.Enqueue(context.Background(), TaskMtmDailyRun, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Redis not configured")
}

// ─── isACSkipErr (service.go) — M2 fix ───────────────────────────────────────

func TestIsACSkipErr_True(t *testing.T) {
	assert.True(t, isACSkipErr(ErrMTMInstrumenACSkip))
}

func TestIsACSkipErr_False_DifferentErr(t *testing.T) {
	assert.False(t, isACSkipErr(ErrMTMPeriodeLocked))
}

func TestIsACSkipErr_False_Nil(t *testing.T) {
	assert.False(t, isACSkipErr(nil))
}

// OverrideApprove with AC klasifikasi triggers isACSkipErr path.
func TestService_OverrideApprove_ACKlasifikasi_SkipJurnalStillApproved(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	m.KlasifikasiSnapshot = KlasifikasiAC
	m.MataUang = "IDR"
	repo.mtm = m
	svc := newTestService(repo)
	poster := svc.poster.(*JurnalPosterStub)

	ctx := makeClaimsContext(uuid.New().String())
	result, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment: "Override justified: AC instrument incorrectly routed, skipping jurnal post.",
	})
	// M2 fix: AC skip → APPROVED without jurnal post (not an error)
	require.NoError(t, err)
	assert.Equal(t, "APPROVED", result.Status)
	assert.Len(t, poster.Calls(), 0, "AC instrument must not produce jurnal entry")
}

// ─── OverrideReject — STALE_PRICE status (m3 fix) ────────────────────────────

func TestService_OverrideReject_StalePrice_Succeeds(t *testing.T) {
	repo := newStubRepo()
	m := makeMtm(StatusStalePrice)
	m.LockedFlag = false
	uploaderID := uuid.New()
	m.UploaderID = &uploaderID
	m.MataUang = "IDR"
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	result, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	// m3 fix: STALE_PRICE can be rejected via CanReject()
	require.NoError(t, err)
	assert.Equal(t, "REJECTED", result.Status)
}

// ─── IsNoopProduction (jurnal_poster.go) — m5 fix ────────────────────────────

func TestIsNoopProduction_NonProduction_AlwaysFalse(t *testing.T) {
	// In test env, APP_ENV is not "production"
	noop := NewNoopJurnalPoster(nil)
	// Should return false since APP_ENV != "production"
	assert.False(t, IsNoopProduction(noop))
}

func TestIsNoopProduction_RealPoster_False(t *testing.T) {
	real := NewRealJurnalPoster(slog.Default())
	assert.False(t, IsNoopProduction(real))
}

func TestIsNoopProduction_Stub_False(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	assert.False(t, IsNoopProduction(stub))
}

// ─── resolvePeriodeDates — found path (M1 fix) ───────────────────────────────

func TestService_resolvePeriodeDates_Found(t *testing.T) {
	repo := newStubRepo()
	targetID := uuid.New()
	repo.periodeBukuRef = &PeriodeBukuRef{
		ID:             targetID,
		TanggalMulai:  time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		TanggalAkhir:  time.Date(2026, 6, 30, 23, 59, 59, 0, time.UTC),
		StatusPeriode: "OPEN",
	}
	svc := newTestService(repo)

	from, to, err := svc.resolvePeriodeDates(context.Background(), targetID)
	require.NoError(t, err)
	assert.Equal(t, 2026, from.Year())
	assert.Equal(t, time.June, from.Month())
	assert.Equal(t, 2026, to.Year())
}

func TestService_resolvePeriodeDates_NotFound_Fallback(t *testing.T) {
	repo := newStubRepo()
	repo.periodeBukuRef = nil // GetPeriodeByTanggal returns nil
	svc := newTestService(repo)

	// Non-existent periodeID → fallback to 2000-2100
	from, to, err := svc.resolvePeriodeDates(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.Equal(t, 2000, from.Year())
	assert.Equal(t, 2100, to.Year())
}

// ─── GetPeriodeByTanggal stub coverage ───────────────────────────────────────

func TestStubRepo_GetPeriodeByTanggal_Nil(t *testing.T) {
	repo := newStubRepo()
	p, err := repo.GetPeriodeByTanggal(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Nil(t, p)
}

func TestStubRepo_GetPeriodeByTanggal_Set(t *testing.T) {
	repo := newStubRepo()
	pID := uuid.New()
	repo.periodeBukuRef = &PeriodeBukuRef{ID: pID, StatusPeriode: "OPEN"}
	p, err := repo.GetPeriodeByTanggal(context.Background(), time.Now())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, pID, p.ID)
}

// ─── TriggerCron — ForceRerun validation (m7 fix) ────────────────────────────

func TestService_TriggerCron_ForceRerun_ShortReason_Rejected(t *testing.T) {
	svc := newTestService(newStubRepo())
	eq := &stubEnqueuer{}

	_, err := svc.TriggerCron(context.Background(), eq, CronTriggerRequest{
		TanggalTarget:    "2026-06-10",
		ForceRerun:       true,
		ForceRerunReason: "too short", // < 30 chars
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "30")
	assert.Len(t, eq.calls, 0)
}

func TestService_TriggerCron_ForceRerun_ValidReason_Enqueued(t *testing.T) {
	svc := newTestService(newStubRepo())
	eq := &stubEnqueuer{}

	result, err := svc.TriggerCron(context.Background(), eq, CronTriggerRequest{
		TanggalTarget:    "2026-06-10",
		ForceRerun:       true,
		ForceRerunReason: "Recompute needed due to IBPA feed corruption on this date.",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result.JobID)
	assert.Len(t, eq.calls, 1)
}

// ─── OverrideApprove — invalid SignatureMethod (m6 fix) ──────────────────────

func TestService_OverrideApprove_InvalidSignatureMethod(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment:         "Override justified: Bloomberg price confirmed by phone with dealer.",
		SignatureMethod: "INVALID_METHOD",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signatureMethod")
}

func TestService_OverrideApprove_JWTStepUp_Accepted(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	result, err := svc.OverrideApprove(ctx, m.ID, OverrideApproveRequest{
		Comment:         "Override justified: Bloomberg price confirmed by phone with dealer.",
		SignatureMethod:  "JWT_STEP_UP",
	})
	require.NoError(t, err)
	assert.Equal(t, "APPROVED", result.Status)
}

// ─── OverrideReject — invalid SignatureMethod (m6 fix) ───────────────────────

func TestService_OverrideReject_InvalidSignatureMethod(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(uuid.New().String())
	_, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{
		Comment:         "Price is clearly wrong — reverting to stale price for re-verification.",
		SignatureMethod: "INVALID_METHOD",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signatureMethod")
}

// ─── OverrideReject — SoD violation path ─────────────────────────────────────

func TestService_OverrideReject_SOD_Violation(t *testing.T) {
	repo := newStubRepo()
	m := makePendingMtm()
	// uploader_id == rejecter_id → SoD violation
	rejecterID := uuid.New()
	m.UploaderID = &rejecterID
	repo.mtm = m
	svc := newTestService(repo)

	ctx := makeClaimsContext(rejecterID.String())
	_, err := svc.OverrideReject(ctx, m.ID, OverrideRejectRequest{
		Comment: "Price is clearly wrong — reverting to stale price for re-verification.",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload")
}

// ─── UploadManual — insert error path (processUploadRow) ─────────────────────

func TestService_UploadManual_InsertError(t *testing.T) {
	repo := newStubRepo()
	repo.insertErr = fmt.Errorf("database insert failed")
	svc := newTestService(repo)

	rows := []UploadFileRow{
		{LineNumber: 1, KodeInstrumen: "OBL-001", TanggalMtm: "2026-06-10", HargaPasar: "100", HargaSumber: "MANUAL"},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	// Insert error → row counted as invalid
	assert.Equal(t, 1, result.RowsInvalid)
}

// ─── GetStalePriceAlerts — error from repo ───────────────────────────────────

type errStaleRepo struct {
	*stubRepo
}

func (r *errStaleRepo) ListStaleAlerts(_ context.Context, _ string, _ int) ([]*Mtm, bool, int, error) {
	return nil, false, 0, fmt.Errorf("stale repo error")
}

func TestService_GetStalePriceAlerts_RepoError(t *testing.T) {
	svc := newTestService(&errStaleRepo{newStubRepo()})
	_, _, _, err := svc.GetStalePriceAlerts(context.Background(), "TUGURE", 50)
	require.Error(t, err)
}

// ─── GetDetail — repo error path ─────────────────────────────────────────────

type errGetByIDRepo struct {
	*stubRepo
}

func (r *errGetByIDRepo) GetByID(_ context.Context, _ uuid.UUID) (*Mtm, error) {
	return nil, fmt.Errorf("db read error")
}

func TestService_GetDetail_RepoError(t *testing.T) {
	svc := newTestService(&errGetByIDRepo{newStubRepo()})
	_, err := svc.GetDetail(context.Background(), uuid.New())
	require.Error(t, err)
}

// ─── UploadManual — deviation warning path ────────────────────────────────────

func TestService_UploadManual_DeviationWarning_Captured(t *testing.T) {
	// Large deviation → status=PENDING_REVIEW, devWarn populated
	repo := newStubRepo()
	bv := decimal.NewFromFloat(100)
	repo.hargaBukuIdr = &bv
	svc := newTestService(repo)

	rows := []UploadFileRow{
		{LineNumber: 1, KodeInstrumen: "OBL-001", TanggalMtm: "2026-06-10", HargaPasar: "200", HargaSumber: "MANUAL"},
	}
	result, err := svc.UploadManual(context.Background(), uuid.New(), rows, "")
	require.NoError(t, err)
	assert.Equal(t, 1, result.RowsValid)
	// DeviationWarnings should be populated
	assert.NotEmpty(t, result.DeviationWarnings)
}
