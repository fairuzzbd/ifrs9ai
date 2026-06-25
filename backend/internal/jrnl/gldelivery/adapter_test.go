package gldelivery_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

func makeTestPayload() DeliveryPayload {
	return DeliveryPayload{
		IdempotencyKey: "test-idempotency-key",
		JournalDate:    "2026-06-15",
		Reference:      "JRN-001",
		EventCode:      "PENEMPATAN",
		Narrative:      "Test journal",
		Lines: []DeliveryLine{
			{AccountCode: "1101", Debit: decimal.NewFromInt(1000000), Kredit: decimal.Zero, Currency: "IDR"},
			{AccountCode: "3101", Debit: decimal.Zero, Kredit: decimal.NewFromInt(1000000), Currency: "IDR"},
		},
	}
}

func TestStubAdapter_Post_Success(t *testing.T) {
	stub := NewStubAdapter()
	payload := makeTestPayload()

	resp, err := stub.Post(context.Background(), payload, "test-key-123456789")
	require.NoError(t, err)
	assert.Equal(t, 201, resp.HTTPStatus)
	assert.Contains(t, resp.GlResponseID, "STUB-JRN-")
	assert.NotEmpty(t, resp.RawResponseJsonb)
}

func TestStubAdapter_Post_RecordsCalls(t *testing.T) {
	stub := NewStubAdapter()
	payload := makeTestPayload()

	_, _ = stub.Post(context.Background(), payload, "key1")
	_, _ = stub.Post(context.Background(), payload, "key2")

	calls := stub.Calls()
	assert.Len(t, calls, 2)
}

func TestStubAdapter_Post_4xxError(t *testing.T) {
	stub := NewStubAdapter(StubConfig{FailHTTPStatus: 422, FailMessage: "domain error"})
	_, err := stub.Post(context.Background(), makeTestPayload(), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "422")
}

func TestStubAdapter_Post_5xxError(t *testing.T) {
	stub := NewStubAdapter(StubConfig{FailHTTPStatus: 503, FailMessage: "service unavailable"})
	_, err := stub.Post(context.Background(), makeTestPayload(), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestStubAdapter_Post_Timeout(t *testing.T) {
	stub := NewStubAdapter(StubConfig{Timeout: true})
	_, err := stub.Post(context.Background(), makeTestPayload(), "key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

func TestStubAdapter_GetDailySummary_Success(t *testing.T) {
	accounts := []AkunTotal{
		{KodeAkun: "1101", NetAmountIDR: decimal.NewFromInt(5000000)},
		{KodeAkun: "3101", NetAmountIDR: decimal.NewFromInt(-5000000)},
	}
	stub := NewStubAdapter(StubConfig{SummaryAccounts: accounts})

	totals, err := stub.GetDailySummary(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Len(t, totals, 2)
	assert.Equal(t, "1101", totals[0].KodeAkun)
}

func TestStubAdapter_GetDailySummary_Error(t *testing.T) {
	stub := NewStubAdapter(StubConfig{FailHTTPStatus: 500, FailMessage: "GL down"})
	_, err := stub.GetDailySummary(context.Background(), time.Now())
	require.Error(t, err)
}

func TestSanitizePII_RedactsConfiguredFields(t *testing.T) {
	data := map[string]any{
		"customer_name": "John Doe",
		"account_no":    "12345678",
		"npwp":          "111222333444",
		"ktp":           "3300000000001",
		"amount":        1000000,
		"reference":     "JRN-001",
	}

	cleaned := SanitizePII(data, PIIFieldsDefault)

	assert.Equal(t, "[REDACTED]", cleaned["customer_name"])
	assert.Equal(t, "[REDACTED]", cleaned["account_no"])
	assert.Equal(t, "[REDACTED]", cleaned["npwp"])
	assert.Equal(t, "[REDACTED]", cleaned["ktp"])
	assert.Equal(t, 1000000, cleaned["amount"])
	assert.Equal(t, "JRN-001", cleaned["reference"])
}

func TestSanitizePII_AlwaysRedactsAPIKey(t *testing.T) {
	data := map[string]any{
		"api_key":  "super-secret-key",
		"apikey":   "another-secret",
		"password": "p@ssw0rd",
		"secret":   "mysecret",
		"safe":     "visible",
	}

	cleaned := SanitizePII(data, map[string]struct{}{})

	assert.Equal(t, "[REDACTED]", cleaned["api_key"])
	assert.Equal(t, "[REDACTED]", cleaned["apikey"])
	assert.Equal(t, "[REDACTED]", cleaned["password"])
	assert.Equal(t, "[REDACTED]", cleaned["secret"])
	assert.Equal(t, "visible", cleaned["safe"])
}

func TestSanitizePII_Recursive(t *testing.T) {
	data := map[string]any{
		"metadata": map[string]any{
			"customer_name": "Jane Doe",
			"reference":     "REF-001",
		},
		"amount": 500000,
	}

	cleaned := SanitizePII(data, PIIFieldsDefault)

	metadata, ok := cleaned["metadata"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "[REDACTED]", metadata["customer_name"])
	assert.Equal(t, "REF-001", metadata["reference"])
}

func TestSanitizePII_NilInput(t *testing.T) {
	result := SanitizePII(nil, PIIFieldsDefault)
	assert.Nil(t, result)
}

func TestSanitizePIIRaw_ValidJSON(t *testing.T) {
	raw := json.RawMessage(`{"customer_name":"John","amount":1000,"api_key":"secret"}`)
	cleaned := SanitizePIIRaw(raw, PIIFieldsDefault)

	var result map[string]any
	require.NoError(t, json.Unmarshal(cleaned, &result))
	assert.Equal(t, "[REDACTED]", result["customer_name"])
	assert.Equal(t, float64(1000), result["amount"])
	assert.Equal(t, "[REDACTED]", result["api_key"])
}

func TestSanitizePIIRaw_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	// Should return original when unparseable.
	result := SanitizePIIRaw(raw, PIIFieldsDefault)
	assert.Equal(t, raw, result)
}

func TestSanitizePIIRaw_Empty(t *testing.T) {
	result := SanitizePIIRaw(nil, PIIFieldsDefault)
	assert.Nil(t, result)
}
