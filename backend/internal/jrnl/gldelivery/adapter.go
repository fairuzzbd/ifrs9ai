package gldelivery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// GLHostAdapter is the interface for communicating with the external GL Host system.
// Implementations are swap-safe (DEC-031 vendor TBD).
type GLHostAdapter interface {
	// Post sends a journal entry to GL Host and returns the delivery response.
	// idempotencyKey is passed as header to GL Host (dedup on their side).
	Post(ctx context.Context, payload DeliveryPayload, idempotencyKey string) (DeliveryResponse, error)
	// GetDailySummary fetches GL Host per-account net amounts for a given date.
	GetDailySummary(ctx context.Context, date time.Time) ([]AkunTotal, error)
}

// ─── PII Sanitizer ────────────────────────────────────────────────────────────

// PIIFieldsDefault is the default set of fields to redact.
// Overridden by the GL_HOST_PII_FIELDS_TO_REDACT config (migration 000037).
var PIIFieldsDefault = map[string]struct{}{
	"customer_name": {},
	"account_no":    {},
	"npwp":          {},
	"ktp":           {},
}

// SanitizePII replaces PII field values in a map with "[REDACTED]".
// Works recursively on nested maps. API keys are always redacted.
func SanitizePII(data map[string]any, fieldsToRedact map[string]struct{}) map[string]any {
	if data == nil {
		return nil
	}
	if fieldsToRedact == nil {
		fieldsToRedact = PIIFieldsDefault
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		lk := strings.ToLower(k)
		// Always redact credentials regardless of config.
		if strings.Contains(lk, "api_key") || strings.Contains(lk, "apikey") ||
			strings.Contains(lk, "secret") || strings.Contains(lk, "password") {
			out[k] = "[REDACTED]"
			continue
		}
		if _, redact := fieldsToRedact[lk]; redact {
			out[k] = "[REDACTED]"
			continue
		}
		// Recurse into nested maps.
		if nested, ok := v.(map[string]any); ok {
			out[k] = SanitizePII(nested, fieldsToRedact)
			continue
		}
		out[k] = v
	}
	return out
}

// SanitizePIIRaw sanitizes a JSON raw message.
func SanitizePIIRaw(raw json.RawMessage, fields map[string]struct{}) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw // cannot parse — return as-is (conservative)
	}
	cleaned := SanitizePII(m, fields)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return out
}

// ─── StubAdapter (dev/test) ───────────────────────────────────────────────────

// StubConfig controls stub adapter behavior for tests.
type StubConfig struct {
	// FailHTTPStatus: if > 0, Post returns this HTTP status as an error.
	FailHTTPStatus int
	// FailMessage: error message when FailHTTPStatus is set.
	FailMessage string
	// SummaryAccounts: per-account totals returned by GetDailySummary.
	SummaryAccounts []AkunTotal
	// Timeout: if true, Post returns a timeout error.
	Timeout bool
}

// StubAdapter is an in-memory GL Host adapter for dev and tests.
type StubAdapter struct {
	cfg   StubConfig
	calls []DeliveryPayload
}

// NewStubAdapter creates a StubAdapter with optional failure config.
func NewStubAdapter(cfg ...StubConfig) *StubAdapter {
	s := &StubAdapter{}
	if len(cfg) > 0 {
		s.cfg = cfg[0]
	}
	return s
}

// Post implements GLHostAdapter for the stub.
func (a *StubAdapter) Post(_ context.Context, payload DeliveryPayload, idempotencyKey string) (DeliveryResponse, error) {
	a.calls = append(a.calls, payload)
	if a.cfg.Timeout {
		return DeliveryResponse{}, domainerrors.New(
			domainerrors.CodeGLDeliveryTimeout,
			"GL Host timeout (stub)",
		)
	}
	if a.cfg.FailHTTPStatus > 0 {
		raw, _ := json.Marshal(map[string]any{ //nolint:errcheck
			"error":   a.cfg.FailMessage,
			"message": a.cfg.FailMessage,
		})
		resp := DeliveryResponse{
			HTTPStatus:       a.cfg.FailHTTPStatus,
			RawResponseJsonb: raw,
		}
		if a.cfg.FailHTTPStatus >= 500 {
			return resp, domainerrors.New(domainerrors.CodeGLDeliveryHostUnreachable,
				fmt.Sprintf("GL Host returned %d: %s", a.cfg.FailHTTPStatus, a.cfg.FailMessage))
		}
		return resp, domainerrors.New(domainerrors.CodeGLDeliveryHost4XX,
			fmt.Sprintf("GL Host returned %d: %s", a.cfg.FailHTTPStatus, a.cfg.FailMessage))
	}

	suffix := idempotencyKey
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	raw, _ := json.Marshal(map[string]any{ //nolint:errcheck
		"journalId": "STUB-JRN-" + suffix,
		"status":    "ACCEPTED",
	})
	return DeliveryResponse{
		GlResponseID:     "STUB-JRN-" + suffix,
		HTTPStatus:       201,
		RawResponseJsonb: raw,
	}, nil
}

// GetDailySummary implements GLHostAdapter for the stub.
func (a *StubAdapter) GetDailySummary(_ context.Context, _ time.Time) ([]AkunTotal, error) {
	if a.cfg.FailHTTPStatus >= 500 {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryHostUnreachable,
			fmt.Sprintf("GL Host daily-summary returned %d", a.cfg.FailHTTPStatus))
	}
	return a.cfg.SummaryAccounts, nil
}

// Calls returns a copy of all recorded Post payloads (for test assertions).
func (a *StubAdapter) Calls() []DeliveryPayload {
	return append([]DeliveryPayload{}, a.calls...)
}

// ─── RESTAdapter (production) ─────────────────────────────────────────────────

// RESTAdapterConfig holds runtime config for the production REST adapter.
type RESTAdapterConfig struct {
	BaseURL        string
	AuthType       string // BEARER | API_KEY
	APIKey         string // resolved from Vault at boot time — never logged
	TimeoutSeconds int
	PIIFields      map[string]struct{} // from GL_HOST_PII_FIELDS_TO_REDACT
}

// RESTAdapter is the production GL Host adapter using net/http.
type RESTAdapter struct {
	cfg    RESTAdapterConfig
	client *http.Client
}

// NewRESTAdapter creates a RESTAdapter. Returns error if BaseURL is empty.
func NewRESTAdapter(cfg RESTAdapterConfig) (*RESTAdapter, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("gldelivery.NewRESTAdapter: BaseURL is required")
	}
	timeout := 30 * time.Second
	if cfg.TimeoutSeconds > 0 {
		timeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	}
	if cfg.PIIFields == nil {
		cfg.PIIFields = PIIFieldsDefault
	}
	return &RESTAdapter{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}, nil
}

// Post sends a journal entry to GL Host via HTTP POST.
func (a *RESTAdapter) Post(ctx context.Context, payload DeliveryPayload, idempotencyKey string) (DeliveryResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return DeliveryResponse{}, fmt.Errorf("gldelivery.RESTAdapter.Post: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return DeliveryResponse{}, fmt.Errorf("gldelivery.RESTAdapter.Post: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", "BLIPS-"+idempotencyKey)
	a.setAuth(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return DeliveryResponse{}, domainerrors.New(domainerrors.CodeGLDeliveryHostUnreachable,
			fmt.Sprintf("GL Host unreachable: %v", err))
	}
	defer resp.Body.Close() //nolint:errcheck

	rawBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		rawBody = []byte(`{"error":"response body unreadable"}`)
	}
	sanitized := SanitizePIIRaw(rawBody, a.cfg.PIIFields)

	dr := DeliveryResponse{HTTPStatus: resp.StatusCode, RawResponseJsonb: sanitized}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var parsed struct {
			JournalID string `json:"journalId"`
		}
		if jsonErr := json.Unmarshal(rawBody, &parsed); jsonErr == nil {
			dr.GlResponseID = parsed.JournalID
		}
		return dr, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return dr, domainerrors.New(domainerrors.CodeGLDeliveryAuthFailed,
			fmt.Sprintf("GL Host auth rejected: HTTP %d", resp.StatusCode))
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return dr, domainerrors.New(domainerrors.CodeGLDeliveryHost4XX,
			fmt.Sprintf("GL Host domain error: HTTP %d", resp.StatusCode))
	}
	return dr, domainerrors.New(domainerrors.CodeGLDeliveryHostUnreachable,
		fmt.Sprintf("GL Host server error: HTTP %d", resp.StatusCode))
}

// GetDailySummary fetches GL Host per-account net amounts for a given date.
func (a *RESTAdapter) GetDailySummary(ctx context.Context, date time.Time) ([]AkunTotal, error) {
	url := a.cfg.BaseURL + "/daily-summary?date=" + date.Format("2006-01-02")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.RESTAdapter.GetDailySummary: build request: %w", err)
	}
	a.setAuth(req)

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryHostUnreachable,
			fmt.Sprintf("GL Host daily-summary unreachable: %v", err))
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, domainerrors.New(domainerrors.CodeGLReconciliationHostFailed,
			fmt.Sprintf("GL Host daily-summary returned HTTP %d", resp.StatusCode))
	}

	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) //nolint:errcheck
	var parsed struct {
		Accounts []struct {
			AccountCode string          `json:"account_code"`
			NetAmount   decimal.Decimal `json:"net_amount"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(rawBody, &parsed); err != nil {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryInvalidResponse,
			fmt.Sprintf("GL Host daily-summary unparseable: %v", err))
	}
	accounts := make([]AkunTotal, 0, len(parsed.Accounts))
	for _, acc := range parsed.Accounts {
		accounts = append(accounts, AkunTotal{
			KodeAkun:     acc.AccountCode,
			NetAmountIDR: acc.NetAmount,
		})
	}
	return accounts, nil
}

func (a *RESTAdapter) setAuth(req *http.Request) {
	switch a.cfg.AuthType {
	case "API_KEY":
		req.Header.Set("X-API-Key", a.cfg.APIKey)
	default: // BEARER
		req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	}
}
