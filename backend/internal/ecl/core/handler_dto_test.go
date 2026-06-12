package core

// handler_dto_test.go — additional handler tests targeting uncovered DTO mappers
// and pure utility functions.
//
// Coverage targets:
//   - resultLineToDTO
//   - resultLineRowToDTO
//   - respondInternal
//   - parseIntQuery + parseInt
//   - claimsUserUUID with UUID type in context

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// ─── resultLineToDTO ──────────────────────────────────────────────────────────

func TestResultLineToDTO_AllFields(t *testing.T) {
	t.Parallel()

	ead := decimal.NewFromFloat(1_000_000_000.0)
	ecl := decimal.NewFromFloat(4_000_000.0)
	now := time.Now()

	line := ResultLine{
		ID:             uuid.New(),
		InstrumenID:    uuid.New(),
		CalcRunID:      uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Stage:          Stage2,
		RoutingPath:    RoutingStandard,
		FlagPOCI:       true,
		EADIDR:         &ead,
		ECLWeightedIDR: &ecl,
		SealedAt:       &now,
		CreatedAt:      now,
	}

	dto := resultLineToDTO(line)

	if dto["stage"] != 2 {
		t.Errorf("stage: want 2, got %v", dto["stage"])
	}
	if dto["routingPath"] != "STANDARD" {
		t.Errorf("routingPath: want STANDARD, got %v", dto["routingPath"])
	}
	if dto["eadIdr"] != "1000000000.0000" {
		t.Errorf("eadIdr: want '1000000000.0000', got %v", dto["eadIdr"])
	}
	if dto["eclWeightedIdr"] != "4000000.0000" {
		t.Errorf("eclWeightedIdr: want '4000000.0000', got %v", dto["eclWeightedIdr"])
	}
	if dto["flagPoci"] != true {
		t.Errorf("flagPoci: want true, got %v", dto["flagPoci"])
	}
	// sealedAt should be present.
	if _, ok := dto["sealedAt"]; !ok {
		t.Error("sealedAt: expected in dto when not nil")
	}
}

func TestResultLineToDTO_NilOptionals(t *testing.T) {
	t.Parallel()

	line := ResultLine{
		ID:             uuid.New(),
		InstrumenID:    uuid.New(),
		CalcRunID:      uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Stage:          Stage1,
		RoutingPath:    RoutingStandard,
		CreatedAt:      time.Now(),
		// EADIDR, ECLWeightedIDR, SealedAt all nil.
	}

	dto := resultLineToDTO(line)
	if _, ok := dto["eadIdr"]; ok {
		t.Error("eadIdr: should not be present when nil")
	}
	if _, ok := dto["eclWeightedIdr"]; ok {
		t.Error("eclWeightedIdr: should not be present when nil")
	}
	if _, ok := dto["sealedAt"]; ok {
		t.Error("sealedAt: should not be present when nil")
	}
}

// ─── resultLineRowToDTO ───────────────────────────────────────────────────────

func TestResultLineRowToDTO_FullRow(t *testing.T) {
	t.Parallel()

	pd := decimal.NewFromFloat(0.02)
	lgd := decimal.NewFromFloat(0.40)
	fl := decimal.NewFromFloat(1.10)
	ead := decimal.NewFromInt(1_000_000_000)
	ecl := decimal.NewFromFloat(8_800_000.0)
	netCarrying := decimal.NewFromFloat(991_200_000.0)
	priorECL := decimal.NewFromFloat(8_000_000.0)

	row := &ResultLineRow{
		ID:                uuid.New(),
		CalcRunID:         uuid.New(),
		InstrumenID:       uuid.New(),
		EvaluationDate:    time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:         "JUNI-2026",
		Stage:             Stage3,
		RoutingPath:       RoutingStandard,
		EADIDR:            ead,
		PDGood:            &pd,
		PDNormal:          &pd,
		PDBad:             &pd,
		LGDUsed:           &lgd,
		FLGood:            &fl,
		FLNormal:          &fl,
		FLBad:             &fl,
		ECLGoodIDR:        decimal.NewFromFloat(8_000_000.0),
		ECLNormalIDR:      decimal.NewFromFloat(8_000_000.0),
		ECLBadIDR:         decimal.NewFromFloat(8_000_000.0),
		ECLFLGoodIDR:      decimal.NewFromFloat(8_800_000.0),
		ECLFLNormalIDR:    decimal.NewFromFloat(8_800_000.0),
		ECLFLBadIDR:       decimal.NewFromFloat(8_800_000.0),
		ECLWeightedIDR:    &ecl,
		BobotGood:         decimal.NewFromFloat(0.25),
		BobotNormal:       decimal.NewFromFloat(0.50),
		BobotBad:          decimal.NewFromFloat(0.25),
		NetCarryingIDR:    &netCarrying,
		PriorSealedECLIDR: &priorECL,
		FlagPOCI:          false,
		Warnings:          []string{WarnStage3NetCarryingFirstRun},
	}

	dto := resultLineRowToDTO(row)

	if dto["stage"] != 3 {
		t.Errorf("stage: want 3, got %v", dto["stage"])
	}
	if dto["eadIdr"] != "1000000000.0000" {
		t.Errorf("eadIdr: want '1000000000.0000', got %v", dto["eadIdr"])
	}
	if dto["eclWeightedIdr"] != "8800000.0000" {
		t.Errorf("eclWeightedIdr: want '8800000.0000', got %v", dto["eclWeightedIdr"])
	}
	if dto["netCarryingIdr"] != "991200000.0000" {
		t.Errorf("netCarryingIdr: want '991200000.0000', got %v", dto["netCarryingIdr"])
	}
	if dto["priorSealedEclIdr"] != "8000000.0000" {
		t.Errorf("priorSealedEclIdr: want '8000000.0000', got %v", dto["priorSealedEclIdr"])
	}
	if dto["pdUsedGood"] != "0.02000000" {
		t.Errorf("pdUsedGood: want '0.02000000', got %v", dto["pdUsedGood"])
	}
	if dto["lgdUsed"] != "0.40000000" {
		t.Errorf("lgdUsed: want '0.40000000', got %v", dto["lgdUsed"])
	}
	if dto["flMultiplierGood"] != "1.10000000" {
		t.Errorf("flMultiplierGood: want '1.10000000', got %v", dto["flMultiplierGood"])
	}
}

func TestResultLineRowToDTO_POCI_NilECL(t *testing.T) {
	t.Parallel()

	row := &ResultLineRow{
		ID:             uuid.New(),
		CalcRunID:      uuid.New(),
		InstrumenID:    uuid.New(),
		EvaluationDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		PeriodeID:      "JUNI-2026",
		Stage:          Stage1,
		RoutingPath:    RoutingPOCIDeferred,
		EADIDR:         decimal.NewFromInt(1_000_000_000),
		ECLGoodIDR:     decimal.Zero,
		ECLNormalIDR:   decimal.Zero,
		ECLBadIDR:      decimal.Zero,
		ECLFLGoodIDR:   decimal.Zero,
		ECLFLNormalIDR: decimal.Zero,
		ECLFLBadIDR:    decimal.Zero,
		ECLWeightedIDR: nil, // POCI
		BobotGood:      decimal.NewFromFloat(0.25),
		BobotNormal:    decimal.NewFromFloat(0.50),
		BobotBad:       decimal.NewFromFloat(0.25),
		FlagPOCI:       true,
	}

	dto := resultLineRowToDTO(row)

	// POCI: eclWeightedIdr present but nil value.
	if v, ok := dto["eclWeightedIdr"]; !ok {
		t.Error("eclWeightedIdr: should be present (explicitly null) for POCI")
	} else if v != nil {
		t.Errorf("eclWeightedIdr: want nil for POCI, got %v", v)
	}
}

// ─── respondInternal ─────────────────────────────────────────────────────────

func TestRespondInternal(t *testing.T) {
	t.Parallel()

	w, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	respondInternal(c, nil)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("respondInternal: want 500, got %d", w.Code)
	}
}

// ─── parseIntQuery + parseInt ─────────────────────────────────────────────────

func TestParseIntQuery_Default(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/?limit=100", nil)
	// parseIntQuery("limit", 50) should return 100.
	got := parseIntQuery(c, "limit", 50)
	if got != 100 {
		t.Errorf("parseIntQuery with value: want 100, got %d", got)
	}
}

func TestParseIntQuery_MissingKey(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	got := parseIntQuery(c, "limit", 50)
	if got != 50 {
		t.Errorf("parseIntQuery missing key: want default 50, got %d", got)
	}
}

func TestParseIntQuery_InvalidValue(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/?limit=abc", nil)
	got := parseIntQuery(c, "limit", 50)
	if got != 50 {
		t.Errorf("parseIntQuery invalid value: want default 50, got %d", got)
	}
}

// ─── claimsUserUUID ───────────────────────────────────────────────────────────

func TestClaimsUserUUID_FromContextUUID(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	want := uuid.New()
	c.Set("user_id", want) // UUID type directly
	c.Set("claims", &auth.Claims{Sub: want.String()})

	got := claimsUserUUID(c)
	if got != want {
		t.Errorf("claimsUserUUID UUID: want %s, got %s", want, got)
	}
}

func TestClaimsUserUUID_FromContextString(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	want := uuid.New()
	c.Set("user_id", want.String()) // string type
	c.Set("claims", &auth.Claims{Sub: want.String()})

	got := claimsUserUUID(c)
	if got != want {
		t.Errorf("claimsUserUUID string: want %s, got %s", want, got)
	}
}

func TestClaimsUserUUID_FallbackToClaimsSub(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	want := uuid.New()
	// No user_id in context — fallback to claims.Sub.
	c.Set("claims", &auth.Claims{Sub: want.String()})

	got := claimsUserUUID(c)
	if got != want {
		t.Errorf("claimsUserUUID fallback: want %s, got %s", want, got)
	}
}

func TestClaimsUserUUID_NoContext(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	// No user_id, no claims → uuid.Nil.
	got := claimsUserUUID(c)
	if got != uuid.Nil {
		t.Errorf("claimsUserUUID no context: want uuid.Nil, got %s", got)
	}
}

// ─── traceID ─────────────────────────────────────────────────────────────────

func TestTraceID_FromContextKey(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Set("trace_id", "trace-abc")

	got := traceID(c)
	if got != "trace-abc" {
		t.Errorf("traceID: want 'trace-abc', got %q", got)
	}
}

func TestTraceID_FallbackToHeader(t *testing.T) {
	t.Parallel()

	_, c := newTestGinContext()
	c.Request, _ = http.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("X-Trace-Id", "trace-xyz")

	got := traceID(c)
	if got != "trace-xyz" {
		t.Errorf("traceID: want 'trace-xyz', got %q", got)
	}
}

// ─── Helper ──────────────────────────────────────────────────────────────────

// newTestGinContext creates a minimal gin context for unit testing without a router.
func newTestGinContext() (*httptest.ResponseRecorder, *gin.Context) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return w, c
}
