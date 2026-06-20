package renewal

// coverage_test.go — Additional tests to reach ≥85% package coverage.
// Covers domain helpers, jurnal_poster stubs, service error paths, and routes.

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── domain.go helpers ───────────────────────────────────────────────────────

func TestToListItem_AllFieldsNil(t *testing.T) {
	r := goodRenewal(StatusPendingApproval)
	r.InstrumenBaruID = nil
	r.ApproverID = nil
	r.JurnalHeaderID = nil

	li := ToListItem(r, "DEP-001")
	assert.Equal(t, r.ID.String(), li.ID)
	assert.Equal(t, "DEP-001", li.InstrumenLamaKode)
	assert.Nil(t, li.InstrumenBaruID)
	assert.Nil(t, li.ApproverID)
	assert.Nil(t, li.JurnalEntryID)
}

func TestToListItem_AllFieldsSet(t *testing.T) {
	r := goodRenewal(StatusPosted)
	ibID := uuid.New()
	apID := uuid.New()
	jhID := uuid.New()
	r.InstrumenBaruID = &ibID
	r.ApproverID = &apID
	r.JurnalHeaderID = &jhID

	li := ToListItem(r, "DEP-002")
	require.NotNil(t, li.InstrumenBaruID)
	require.NotNil(t, li.ApproverID)
	require.NotNil(t, li.JurnalEntryID)
	assert.Equal(t, ibID.String(), *li.InstrumenBaruID)
	assert.Equal(t, apID.String(), *li.ApproverID)
	assert.Equal(t, jhID.String(), *li.JurnalEntryID)
}

func TestParseDateStrict_Valid(t *testing.T) {
	d, err := ParseDateStrict("2026-07-01")
	require.NoError(t, err)
	assert.Equal(t, 2026, d.Year())
	assert.Equal(t, time.July, d.Month())
	assert.Equal(t, 1, d.Day())
}

func TestParseDateStrict_Invalid(t *testing.T) {
	cases := []string{"", "01-07-2026", "2026/07/01", "not-a-date", "2026-13-01"}
	for _, c := range cases {
		_, err := ParseDateStrict(c)
		assert.Error(t, err, "input=%q", c)
	}
}

func TestAddMonths(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := AddMonths(start, 6)
	assert.Equal(t, 2026, result.Year())
	assert.Equal(t, time.July, result.Month())
	assert.Equal(t, 1, result.Day())
}

func TestToDetail_EirAndPeriodeNil(t *testing.T) {
	r := goodRenewal(StatusPendingApproval)
	r.EirBaru = nil
	r.PeriodeBulananID = nil

	preview := PreviewResult{
		PokokLama:             r.PokokLama,
		BungaKotor:            r.BungaKotor,
		Pph20pct:              r.PphAmount,
		BungaBersih:           r.BungaBersih,
		PokokBaru:             r.PokokBaru,
		TanggalJatuhTempoBaru: time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	d := ToDetail(r, "DEP-003", preview)
	assert.Nil(t, d.EirBaru)
	assert.Nil(t, d.PeriodeBulananID)
	assert.NotEmpty(t, d.BungaKotor)
}

func TestToDetail_EirAndPeriodeSet(t *testing.T) {
	r := goodRenewal(StatusPosted)
	eir := decimal.NewFromFloat(0.056)
	pid := uuid.New()
	r.EirBaru = &eir
	r.PeriodeBulananID = &pid

	preview := PreviewResult{}
	d := ToDetail(r, "DEP-004", preview)
	require.NotNil(t, d.EirBaru)
	require.NotNil(t, d.PeriodeBulananID)
	assert.Equal(t, "0.05600000", *d.EirBaru)
}

// ─── jurnal_poster.go helper methods ─────────────────────────────────────────

func TestJurnalPosterStub_SetResultAndCalls(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	customResult := RenewalPostResult{JurnalEntryID: uuid.New()}
	stub.SetResult(customResult)

	ctx := approverCtx()
	_, err := stub.Post(ctx, nil, RenewalPostRequest{})
	require.NoError(t, err)
	assert.Equal(t, 1, len(stub.Calls()))
}

func TestJurnalPosterStub_Reset(t *testing.T) {
	stub := NewJurnalPosterStub(nil)
	stub.SetError(fmt.Errorf("some error"))
	stub.Reset()

	ctx := approverCtx()
	_, err := stub.Post(ctx, nil, RenewalPostRequest{})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(stub.Calls()))
}

func TestNoopJurnalPoster_Post(t *testing.T) {
	svc := NewService(
		&stubRepo{
			instrumenInfo: goodInstrumen(),
			hasActive:     false,
			periode:       goodPeriode(),
		},
		nil, // nil → NewService replaces with NoopJurnalPoster
		NewInstrumenCreatorStub(),
		NewEIRScheduleWriterStub(),
		nil,
		nil,
	)
	assert.NotNil(t, svc)
	// IsNoopProduction is a package-level function
	noop := NewNoopJurnalPoster(nil)
	// In test env (APP_ENV != "production"), IsNoopProduction returns false
	assert.False(t, IsNoopProduction(noop))
	ctx := approverCtx()
	result, err := noop.Post(ctx, nil, RenewalPostRequest{EventCode: "RENEWAL_DEPOSITO"})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, result.JurnalEntryID)
}

func TestInstrumenCreatorStub_ErrorPaths(t *testing.T) {
	stub := NewInstrumenCreatorStub()
	stub.SetCreateError(fmt.Errorf("create failed"))

	ctx := approverCtx()
	inst := goodInstrumen()
	r := goodRenewal(StatusPendingApproval)
	_, err := stub.CreateInstrumenBaru(ctx, nil, *inst, r)
	assert.Error(t, err)
	assert.Equal(t, 1, stub.CreateCalls())

	stub.SetMaturedError(fmt.Errorf("matured failed"))
	err = stub.MaturedInstrumenLama(ctx, nil, instrumenID, approverUUID)
	assert.Error(t, err)
	assert.Equal(t, 1, stub.MaturedCalls())
}

func TestEIRScheduleWriterStub_ErrorPaths(t *testing.T) {
	stub := NewEIRScheduleWriterStub()
	stub.SetInsertError(fmt.Errorf("insert failed"))
	stub.SetCloseError(fmt.Errorf("close failed"))

	ctx := approverCtx()
	eir := decimal.NewFromFloat(0.048)
	err := stub.InsertScheduleBaru(ctx, nil, uuid.New(), eir, time.Now(), approverUUID)
	assert.Error(t, err)
	assert.Equal(t, 1, stub.InsertCalls())

	err = stub.CloseScheduleLama(ctx, nil, uuid.New(), time.Now(), approverUUID)
	assert.Error(t, err)
	assert.Equal(t, 1, stub.CloseCalls())
}

// ─── repo helper (package-level function) ────────────────────────────────────

func TestDecimalPtrToStr(t *testing.T) {
	// nil pointer → should return nil (not panic)
	result := decimalPtrToStr(nil, 4)
	assert.Nil(t, result)

	// non-nil → return string representation
	d := decimal.NewFromFloat(12345.6789)
	result = decimalPtrToStr(&d, 4)
	require.NotNil(t, result)
	s, ok := result.(string)
	require.True(t, ok)
	assert.Contains(t, s, "12345.6789")
}

// ─── routes.go (basic smoke test) ────────────────────────────────────────────

func TestRegisterRoutes_Smoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		claims := &auth.Claims{Sub: approverUUID.String(), TenantID: "TUGURE"}
		ctx := auth.ContextWithClaims(c.Request.Context(), claims)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	repo := &stubRepo{listRows: nil}
	svc := newService(repo)
	h := NewHTTPHandler(svc)

	v1 := engine.Group("/api/v1")
	RegisterRoutes(v1, h) // no redis → no rate limiter (graceful skip)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trx/renewal", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	// Should route correctly — 200 (empty list) or 401
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

// ─── service.go additional error branches ────────────────────────────────────

func TestService_CreateRenewal_GetPeriodeError(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		getPeriodeErr: fmt.Errorf("db connection lost"),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(6.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db connection lost")
}

func TestService_CreateRenewal_NoPeriode(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		periode:       nil, // no periode found
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(6.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodeValidationFailed, de.Code())
}

func TestService_CreateRenewal_InsertError(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActive:     false,
		periode:       goodPeriode(),
		insertErr:     fmt.Errorf("unique constraint violation"),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(6.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unique constraint")
}

func TestService_CreateRenewal_HasActiveError(t *testing.T) {
	repo := &stubRepo{
		instrumenInfo: goodInstrumen(),
		hasActiveErr:  fmt.Errorf("db error checking active renewal"),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(6.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error checking active renewal")
}

func TestService_CreateRenewal_GetInstError(t *testing.T) {
	repo := &stubRepo{
		getInstErr: fmt.Errorf("instrumen fetch failed"),
	}
	svc := newService(repo)

	req := CreateRenewalRequest{
		InstrumenID:        instrumenID,
		Skema:              "POKOK_SAJA",
		TenorBaruBulan:     12,
		RateBaruPersen:     decimal.NewFromFloat(6.0),
		TanggalEfektifBaru: "2026-07-01",
	}
	_, err := svc.CreateRenewal(makerCtx(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "instrumen fetch failed")
}

func TestService_Approve_GetInstError(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:    renewal,
		getInstErr: fmt.Errorf("instrumen not found in DB"),
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{Comment: "approve", SignatureMethod: "JWT_STEP_UP"}
	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "instrumen not found in DB")
}

func TestService_Approve_GetPeriodeError(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		getPeriodeErr: fmt.Errorf("periode db error"),
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{Comment: "approve", SignatureMethod: "JWT_STEP_UP"}
	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "periode db error")
}

func TestService_Approve_NoPeriode(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       nil, // no periode
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{Comment: "approve", SignatureMethod: "JWT_STEP_UP"}
	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	de, ok := errors.IsDomainError(err)
	require.True(t, ok)
	assert.Equal(t, errors.CodePeriodeClosed, de.Code())
}

func TestService_Approve_UpdateStatusError(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:       renewal,
		instrumenInfo: goodInstrumen(),
		periode:       goodPeriode(),
		updateErr:     fmt.Errorf("optimistic lock failure"),
	}
	svc := newService(repo)

	req := ApproveRenewalRequest{Comment: "approve", SignatureMethod: "JWT_STEP_UP"}
	_, err := svc.Approve(approverCtx(), renewalID, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "optimistic lock failure")
}

func TestService_Reject_UpdateStatusError(t *testing.T) {
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:   renewal,
		updateErr: fmt.Errorf("reject update failed"),
	}
	svc := newService(repo)

	req := RejectRenewalRequest{
		Comment:        "Reject due to system constraints. Retry later.",
		SignatureMethod: "JWT_STEP_UP",
	}
	_, err := svc.Reject(approverCtx(), renewalID, req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reject update failed")
}

func TestService_GetDetail_GetInstError(t *testing.T) {
	// GetDetail suppresses GetInstrumenInfo errors gracefully (uses stored values)
	renewal := goodRenewal(StatusPendingApproval)
	repo := &stubRepo{
		renewal:    renewal,
		getInstErr: fmt.Errorf("inst not found"), // suppressed
	}
	svc := newService(repo)

	detail, err := svc.GetDetail(approverCtx(), renewalID)
	// GetDetail should still succeed using stored values as fallback
	require.NoError(t, err)
	assert.Equal(t, renewalID.String(), detail.ID)
}

func TestService_GetDetail_RepoError(t *testing.T) {
	repo := &stubRepo{
		getByIDErr: fmt.Errorf("db error on GetByID"),
	}
	svc := newService(repo)

	_, err := svc.GetDetail(approverCtx(), renewalID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error on GetByID")
}

// ─── SQL helpers in repo (via Repo type) ─────────────────────────────────────

func TestRepo_BeginTx_MockDB(t *testing.T) {
	// Verify Repo.BeginTx delegates to db.BeginTx
	// Uses go-sqlmock
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	repo := NewRepo(db)
	tx, err := repo.BeginTx(makerCtx())
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
}

// Prevent unused import
var _ *sql.Tx
