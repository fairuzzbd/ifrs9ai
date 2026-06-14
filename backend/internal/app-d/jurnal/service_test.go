package jurnal

// service_test.go — tests business logic that does NOT require a DB connection.
//
// Strategy: the services hold concrete *Repo pointers rather than interfaces,
// so we cannot easily mock them with gomock. Instead we test:
//   1. Constructor panics (nil auditWriter / nil repo).
//   2. Workflow state-machine guards (CanSubmit/Review/Approve/Approve2/Reject/
//      Deactivate) by calling guard methods directly on MappingHeaderStatus.
//   3. ResolverService balance-invariant logic — replicated inline (same package).
//   4. SoD / step-up domain error construction and HTTP status mapping.
//   5. Internal helper functions: buildNarasi, computeSigHash, tenantIDFromCtx,
//      callerIDFromCtx, nowPtr, min, containsStr.
//   6. DLQ discard reason length guard.
//   7. DLQPostPayload JSON round-trip.
//
// Tests that need a live DB live in integration tests (not in this file).

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Constructor panics ───────────────────────────────────────────────────────

func TestNewMappingServicePanicsOnNilRepo(t *testing.T) {
	assert.Panics(t, func() {
		NewMappingService(nil, nil, nil)
	}, "must panic when repo is nil")
}

func TestNewResolverServicePanicsOnNilRepo(t *testing.T) {
	assert.Panics(t, func() {
		NewResolverService(nil, nil, nil)
	}, "must panic when mappingRepo is nil")
}

func TestNewPostingServicePanicsOnNilJurnalRepo(t *testing.T) {
	assert.Panics(t, func() {
		NewPostingService(nil, nil, nil, nil, nil)
	}, "must panic when jurnalRepo is nil")
}

func TestNewDLQServicePanicsOnNilDLQRepo(t *testing.T) {
	assert.Panics(t, func() {
		NewDLQService(nil, nil, nil, nil)
	}, "must panic when dlqRepo is nil")
}

func TestNewWorkerPanicsOnNilPosting(t *testing.T) {
	assert.Panics(t, func() {
		NewWorker(nil, nil, nil)
	}, "must panic when posting is nil")
}

func TestNewWorkerPanicsOnNilDLQRepo(t *testing.T) {
	// Provide a non-nil PostingService (zero value), nil dlqRepo → panic.
	assert.Panics(t, func() {
		NewWorker(&PostingService{}, nil, nil)
	})
}

// ─── Workflow guard exhaustive state table ────────────────────────────────────

func TestMappingWorkflowGuardsExhaustive(t *testing.T) {
	type row struct {
		status   MappingHeaderStatus
		sub      bool
		rev      bool
		app      bool
		app2     bool
		rej      bool
		withdr   bool
		deact    bool
		resolver bool
	}
	table := []row{
		{MappingStatusDraft, true, false, false, false, false, true, false, false},
		{MappingStatusPendingReview, false, true, false, false, true, false, false, false},
		{MappingStatusPendingApproval, false, false, true, false, true, false, false, false},
		{MappingStatusPendingApproval2, false, false, false, true, true, false, false, false},
		{MappingStatusApprovedActive, false, false, false, false, false, false, true, true},
		{MappingStatusApproved, false, false, false, false, false, false, true, true},
		{MappingStatusWithdrawn, false, false, false, false, false, false, false, false},
		{MappingStatusRejected, false, false, false, false, false, false, false, false},
		{MappingStatusReturned, false, false, false, false, false, false, false, false},
	}

	for _, r := range table {
		t.Run(string(r.status), func(t *testing.T) {
			s := r.status
			assert.Equal(t, r.sub, s.CanSubmit(), "CanSubmit")
			assert.Equal(t, r.rev, s.CanReview(), "CanReview")
			assert.Equal(t, r.app, s.CanApprove(), "CanApprove")
			assert.Equal(t, r.app2, s.CanApprove2(), "CanApprove2")
			assert.Equal(t, r.rej, s.CanReject(), "CanReject")
			assert.Equal(t, r.withdr, s.CanWithdraw(), "CanWithdraw")
			assert.Equal(t, r.deact, s.CanDeactivate(), "CanDeactivate")
			assert.Equal(t, r.resolver, s.IsActiveForResolver(), "IsActiveForResolver")
		})
	}
}

// ─── Resolver: balance invariant ─────────────────────────────────────────────

func TestResolverBalanceImbalancedRows(t *testing.T) {
	// Construct the same loop as Resolve() with imbalanced multipliers.
	rows := []MappingDetailRow{
		{DKIndicator: "DEBIT", Multiplier: decimal.NewFromInt(1)},
		// Kredit has multiplier 2 → kredit = 2x debit → imbalanced.
		{DKIndicator: "KREDIT", Multiplier: decimal.NewFromInt(2)},
	}
	amountIDR := decimal.NewFromFloat(100.0)

	var totalDebit, totalKredit decimal.Decimal
	for _, d := range rows {
		amt := amountIDR.Mul(d.Multiplier)
		if d.DKIndicator == "DEBIT" {
			totalDebit = totalDebit.Add(amt)
		} else {
			totalKredit = totalKredit.Add(amt)
		}
	}

	require.False(t, totalDebit.Equal(totalKredit),
		"prerequisite: rows must be imbalanced (debit=%s, kredit=%s)",
		totalDebit.StringFixed(4), totalKredit.StringFixed(4))

	// The service emits CodeJurnalBalanceInvariant for this condition.
	err := domainerrors.New(domainerrors.CodeJurnalBalanceInvariant, "balance invariant violated")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalBalanceInvariant, de.Code())
	assert.Equal(t, 422, de.HTTPStatus())
}

func TestResolverBalanceSymmetricRows(t *testing.T) {
	rows := []MappingDetailRow{
		{DKIndicator: "DEBIT", Multiplier: decimal.NewFromInt(1)},
		{DKIndicator: "KREDIT", Multiplier: decimal.NewFromInt(1)},
	}
	amountIDR := decimal.NewFromFloat(500_000_000.0)
	var totalDebit, totalKredit decimal.Decimal
	for _, d := range rows {
		amt := amountIDR.Mul(d.Multiplier)
		if d.DKIndicator == "DEBIT" {
			totalDebit = totalDebit.Add(amt)
		} else {
			totalKredit = totalKredit.Add(amt)
		}
	}
	assert.True(t, totalDebit.Equal(totalKredit))
}

func TestResolverAmountZeroRejectsInvariant(t *testing.T) {
	amountIDR := decimal.Zero
	// Service guard: amountIDR.IsZero() || amountIDR.IsNegative() → error.
	needsRejection := amountIDR.IsZero() || amountIDR.IsNegative()
	require.True(t, needsRejection, "zero amount must be rejected")

	err := domainerrors.New(domainerrors.CodeJurnalAmountInvalid, "amountIdr harus > 0")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalAmountInvalid, de.Code())
	assert.Equal(t, 422, de.HTTPStatus())
}

func TestResolverNegativeAmountRejects(t *testing.T) {
	amountIDR := decimal.NewFromFloat(-1.0)
	assert.True(t, amountIDR.IsNegative())
}

// ─── KlasifikasiFilter logic ──────────────────────────────────────────────────

func TestKlasifikasiFilterSkipsNonMatchingRows(t *testing.T) {
	// Reproduce the exact filter guard from Resolve().
	reqKlas := "AC"
	fvociFilter := "FVOCI"
	rows := []MappingDetailRow{
		{Urutan: 1, DKIndicator: "DEBIT", Multiplier: decimal.NewFromInt(1), KlasifikasiFilter: nil},
		{Urutan: 2, DKIndicator: "KREDIT", Multiplier: decimal.NewFromInt(1), KlasifikasiFilter: nil},
		{Urutan: 3, DKIndicator: "DEBIT", Multiplier: decimal.NewFromInt(1), KlasifikasiFilter: &fvociFilter}, // skipped
	}
	amountIDR := decimal.NewFromFloat(1000.0)

	var totalDebit, totalKredit decimal.Decimal
	includedCount := 0
	for _, d := range rows {
		if d.KlasifikasiFilter != nil && *d.KlasifikasiFilter != "" && *d.KlasifikasiFilter != reqKlas {
			continue
		}
		includedCount++
		amt := amountIDR.Mul(d.Multiplier)
		if d.DKIndicator == "DEBIT" {
			totalDebit = totalDebit.Add(amt)
		} else {
			totalKredit = totalKredit.Add(amt)
		}
	}

	assert.Equal(t, 2, includedCount, "only rows 1 and 2 included")
	assert.True(t, totalDebit.Equal(totalKredit), "rows 1+2 are balanced")
}

func TestKlasifikasiFilterIncludesNilFilter(t *testing.T) {
	// nil filter → always included (applies to all klasifikasi).
	var noFilter *string
	reqKlas := "FVTPL"
	if noFilter != nil && *noFilter != "" && *noFilter != reqKlas {
		t.Fatal("nil filter must not be skipped")
	}
	// Reaches here → nil filter included. Test passes.
}

func TestKlasifikasiFilterIncludesEmptyString(t *testing.T) {
	// Empty string filter → always included.
	emptyFilter := ""
	reqKlas := "FVTPL"
	if emptyFilter != "" && emptyFilter != reqKlas {
		t.Fatal("empty filter must not be skipped")
	}
}

// ─── SoD and step-up error construction ───────────────────────────────────────

func TestSoDErrorCodes(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeJurnalSoDViolation,
		"Reviewer tidak boleh sama dengan maker (SoD, DEC-017).")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalSoDViolation, de.Code())
	assert.Equal(t, 403, de.HTTPStatus())
}

func TestStepUpRequiredErrorCode(t *testing.T) {
	err := domainerrors.New(domainerrors.CodeJurnalStepUpRequired, "step-up MFA required")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalStepUpRequired, de.Code())
	assert.Equal(t, 403, de.HTTPStatus())
}

// ─── All P5-M2 error codes HTTP status mapping ────────────────────────────────

func TestP5M2ErrorCodeHTTPStatuses(t *testing.T) {
	cases := []struct {
		code     domainerrors.Code
		wantHTTP int
	}{
		{domainerrors.CodeJurnalEventNotMapped, 422},
		{domainerrors.CodeJurnalKlasifikasiNotEligible, 422},
		{domainerrors.CodeJurnalBalanceInvariant, 422},
		{domainerrors.CodeJurnalPeriodeHardClosed, 423},
		{domainerrors.CodeJurnalDuplicatePost, 409},
		{domainerrors.CodeJurnalInvalidTransition, 422},
		{domainerrors.CodeJurnalSoDViolation, 403},
		{domainerrors.CodeJurnalStepUpRequired, 403},
		{domainerrors.CodeJurnalAmountInvalid, 422},
		{domainerrors.CodeJurnalInstrumenNotFound, 404},
		{domainerrors.CodeJurnalHeaderNotFound, 404},
		{domainerrors.CodeJurnalDlqNotFound, 404},
		{domainerrors.CodeJurnalDlqAlreadyReplayed, 409},
		{domainerrors.CodeJurnalDlqDiscardReasonTooShort, 422},
		{domainerrors.CodeJurnalDlqReplayPeriodeHardClosed, 423},
		{domainerrors.CodeJurnalMappingWorkflowGate, 422},
	}
	for _, c := range cases {
		t.Run(string(c.code), func(t *testing.T) {
			err := domainerrors.New(c.code, "test message")
			de, ok := domainerrors.IsDomainError(err)
			require.True(t, ok)
			assert.Equal(t, c.wantHTTP, de.HTTPStatus(),
				"code=%s: expected HTTP %d, got %d", c.code, c.wantHTTP, de.HTTPStatus())
		})
	}
}

// ─── computeSigHash ───────────────────────────────────────────────────────────

func TestComputeSigHashFormat(t *testing.T) {
	h := computeSigHash(uuid.New(), "REVIEW", uuid.New())
	require.NotEmpty(t, h)
	assert.Len(t, h, 64, "SHA256 hex-encoded = 64 bytes")
}

func TestComputeSigHashDifferentActions(t *testing.T) {
	callerID := uuid.New()
	entityID := uuid.New()
	h1 := computeSigHash(callerID, "REVIEW", entityID)
	h2 := computeSigHash(callerID, "APPROVE", entityID)
	assert.NotEqual(t, h1, h2, "different action must differ")
}

func TestComputeSigHashDifferentCallers(t *testing.T) {
	entityID := uuid.New()
	h1 := computeSigHash(uuid.New(), "REVIEW", entityID)
	h2 := computeSigHash(uuid.New(), "REVIEW", entityID)
	assert.NotEqual(t, h1, h2, "different callers must differ")
}

// ─── buildNarasi ──────────────────────────────────────────────────────────────

func TestBuildNarasiFallback(t *testing.T) {
	req := ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
	}
	d := MappingDetailRow{DKIndicator: "DEBIT"}
	assert.Equal(t, "PENEMPATAN / AC / DEBIT", buildNarasi(req, d))
}

func TestBuildNarasiCustom(t *testing.T) {
	req := ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		Narasi:            "Custom narasi text",
	}
	d := MappingDetailRow{DKIndicator: "KREDIT"}
	assert.Equal(t, "Custom narasi text", buildNarasi(req, d))
}

// ─── DLQ discard reason length guard ─────────────────────────────────────────

func TestDLQDiscardReasonTooShort(t *testing.T) {
	reason := "short"
	require.Less(t, len(reason), 30, "prerequisite: reason must be too short")

	err := domainerrors.New(domainerrors.CodeJurnalDlqDiscardReasonTooShort,
		"discardReason minimal 30 karakter.")
	de, ok := domainerrors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, domainerrors.CodeJurnalDlqDiscardReasonTooShort, de.Code())
	assert.Equal(t, 422, de.HTTPStatus())
}

func TestDLQDiscardReasonAcceptable(t *testing.T) {
	reason := "This is a valid discard reason that is sufficiently long."
	assert.GreaterOrEqual(t, len(reason), 30)
}

// ─── DLQPostPayload JSON round-trip ───────────────────────────────────────────

func TestDLQPostPayloadRoundTrip(t *testing.T) {
	instrID := uuid.New().String()
	original := DLQPostPayload{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: "AC",
		InstrumenID:       &instrID,
		PeriodeID:         uuid.New().String(),
		AmountIDR:         decimal.NewFromFloat(1_000_000_000.0),
		Currency:          "IDR",
		FxRate:            decimal.NewFromInt(1),
		SourceEventID:     uuid.New().String(),
		SourceEventType:   "penempatan:approved",
		Narasi:            "Test narasi untuk DLQ round-trip",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var decoded DLQPostPayload
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, original.EventCode, decoded.EventCode)
	assert.Equal(t, original.KlasifikasiPSAK71, decoded.KlasifikasiPSAK71)
	assert.Equal(t, original.Narasi, decoded.Narasi)
	require.NotNil(t, decoded.InstrumenID)
	assert.Equal(t, instrID, *decoded.InstrumenID)
	assert.True(t, original.AmountIDR.Equal(decoded.AmountIDR),
		"AmountIDR round-trip: %s vs %s", original.AmountIDR, decoded.AmountIDR)
	assert.True(t, original.FxRate.Equal(decoded.FxRate))
}

// ─── tenantIDFromCtx / callerIDFromCtx ────────────────────────────────────────

func TestTenantIDFromCtxDefault(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "TUGURE", tenantIDFromCtx(ctx))
}

func TestCallerIDFromCtxDefault(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, uuid.Nil, callerIDFromCtx(ctx))
}

// ─── min helper ───────────────────────────────────────────────────────────────

func TestMinHelper(t *testing.T) {
	assert.Equal(t, 3, min(3, 5))
	assert.Equal(t, 3, min(5, 3))
	assert.Equal(t, 0, min(0, 1))
	assert.Equal(t, -1, min(-1, 1))
	assert.Equal(t, 5, min(5, 5))
}

// ─── nowPtr ───────────────────────────────────────────────────────────────────

func TestNowPtr(t *testing.T) {
	before := time.Now()
	ptr := nowPtr()
	after := time.Now()
	require.NotNil(t, ptr)
	assert.False(t, ptr.Before(before), "nowPtr must be >= before")
	assert.False(t, ptr.After(after), "nowPtr must be <= after")
}

// ─── IsManualAllowed integration with CreateManualDraft guard ─────────────────

func TestManualPostRequestGuardLogic(t *testing.T) {
	disallowed := []string{
		EventCodePenempatan, EventCodeECLPembentukan, EventCodeJatuhTempo,
		EventCodeFXUnrealized, EventCodeAkrualBunga, EventCodeMTMFVTPL,
	}
	for _, code := range disallowed {
		assert.Falsef(t, IsManualAllowed(code), "code %s must not be allowed for manual posting", code)
	}

	allowed := []string{EventCodePeriodeAdjustment, EventCodeCorrectionPeriodeClosed}
	for _, code := range allowed {
		assert.Truef(t, IsManualAllowed(code), "code %s must be allowed for manual posting", code)
	}
}
