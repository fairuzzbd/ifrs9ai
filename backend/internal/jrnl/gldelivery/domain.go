// Package gldelivery implements P5-M3 — GL Host REST Delivery.
//
// Responsibilities:
//   - Asynq worker "gl_delivery:deliver" — subscribes to JURNAL.POSTED event,
//     POSTs jurnal to GL Host REST endpoint, manages retry + DLQ lifecycle.
//   - DeliveryService — business logic for delivery, manual retry, idempotency.
//   - ReconciliationService — daily BLIPS vs GL Host ledger comparison.
//   - DLQService — inspect / replay / discard sys.dlq_gl_delivery entries.
//   - 9 REST handlers per OpenAPI app-d-gl-delivery.yaml.
//
// Compliance decisions anchoring this package:
//   - DEC-005 RESOLVED: GL Integration Phase 2 REST real-time via Asynq (P5-M3).
//   - DEC-016: No float64 — shopspring/decimal for all amounts.
//   - DEC-018: Audit-in-tx mandatory for ALL mutations (constructor panics on nil auditWriter).
//   - DEC-021: Idempotency-Key mandatory on mutating endpoints.
//   - DEC-030 RESOLVED: GL delivery mode = Async REST via Asynq.
//   - DEC-031 PENDING: GL Host vendor — adapter interface is swap-safe.
//
// Invariants:
//   - jrnl.gl_status: terminal states DELIVERED and DEAD_LETTER are immutable (DB trigger 000037).
//   - sys.dlq_gl_delivery, sys.gl_reconciliation_report, sys.gl_recon_mismatch: no hard delete.
//   - Audit GL_DELIVERY.MANUAL_RETRY_INITIATED written BEFORE Asynq task enqueue.
//   - PII sanitization (customer_name, account_no, npwp, ktp) applied before any JSONB persist.
//   - Constructor panics on nil auditWriter (DEC-018).
package gldelivery

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── GL Host Status enum ──────────────────────────────────────────────────────

// GlHostStatus is the lifecycle state of jrnl.gl_status.gl_host_status.
// State machine (P5-M3):
//
//	PENDING_DELIVERY → DELIVERY_IN_FLIGHT → DELIVERED           (terminal ok)
//	PENDING_DELIVERY → RETRYING (up to 3x infra) → FAILED      (→ DLQ)
//	PENDING_DELIVERY → FAILED (domain 4xx, no retry)            (→ DLQ)
//	FAILED → PENDING_DELIVERY (manual retry)
//	FAILED → DEAD_LETTER (explicit discard by ROLE-IT-ADMIN)    (terminal fail)
type GlHostStatus string

const (
	GlHostStatusPendingDelivery  GlHostStatus = "PENDING_DELIVERY"
	GlHostStatusDeliveryInFlight GlHostStatus = "DELIVERY_IN_FLIGHT"
	GlHostStatusDelivered        GlHostStatus = "DELIVERED"
	GlHostStatusRetrying         GlHostStatus = "RETRYING"
	GlHostStatusFailed           GlHostStatus = "FAILED"
	GlHostStatusDeadLetter       GlHostStatus = "DEAD_LETTER"
)

// IsTerminal returns true for immutable terminal states.
func (s GlHostStatus) IsTerminal() bool {
	return s == GlHostStatusDelivered || s == GlHostStatusDeadLetter
}

// CanManualRetry returns true if a manual retry is allowed from this status.
func (s GlHostStatus) CanManualRetry() bool {
	return s == GlHostStatusFailed
}

// ─── DLQ Status enum ─────────────────────────────────────────────────────────

// DLQStatus is the lifecycle state of sys.dlq_gl_delivery.status.
type DLQStatus string

const (
	DLQStatusFailed     DLQStatus = "FAILED"
	DLQStatusReplaying  DLQStatus = "REPLAYING"
	DLQStatusReplayedOK DLQStatus = "REPLAYED_OK"
	DLQStatusAbandoned  DLQStatus = "ABANDONED"
)

// CanReplay returns true if the DLQ entry can be replayed.
func (s DLQStatus) CanReplay() bool { return s == DLQStatusFailed }

// CanDiscard returns true if the DLQ entry can be discarded.
func (s DLQStatus) CanDiscard() bool { return s == DLQStatusFailed }

// ─── Failure category ────────────────────────────────────────────────────────

const (
	FailureCategoryDomain = "DOMAIN" // 4xx — business rule rejection by GL Host
	FailureCategoryInfra  = "INFRA"  // 5xx / timeout / network
)

// ─── Permission constants ─────────────────────────────────────────────────────

const (
	PermGlDeliveryRead     = "jurnal.gl_delivery.read"
	PermGlDeliveryReadRaw  = "jurnal.gl_delivery.read_raw"
	PermGlDeliveryRetry    = "jurnal.gl_delivery.retry"
	PermGlDeliveryReplay   = "jurnal.gl_delivery.replay"
	PermGlDeliveryDiscard  = "jurnal.gl_delivery.discard"
	PermReconciliationRead = "jurnal.reconciliation.read"
	PermReconciliationRun  = "jurnal.reconciliation.run"
)

// ─── Recon report status ─────────────────────────────────────────────────────

type ReconStatus string

const (
	ReconStatusInProgress            ReconStatus = "IN_PROGRESS"
	ReconStatusCompleted             ReconStatus = "COMPLETED"
	ReconStatusCompletedWithMismatch ReconStatus = "COMPLETED_WITH_MISMATCH"
	ReconStatusFailed                ReconStatus = "FAILED"
)

// MismatchType classifies a reconciliation mismatch.
type MismatchType string

const (
	MismatchTypeBlipsOnly  MismatchType = "BLIPS_ONLY"
	MismatchTypeGLOnly     MismatchType = "GL_ONLY"
	MismatchTypeAmountDiff MismatchType = "AMOUNT_DIFF"
)

// ─── Domain types ─────────────────────────────────────────────────────────────

// DeliveryStatus is the enriched delivery_status sub-object for GET /jurnal/header/{id}/gl-delivery-status.
type DeliveryStatus struct {
	GlStatusID             uuid.UUID                `json:"glStatusId"`
	GlHostStatus           GlHostStatus             `json:"glHostStatus"`
	GlHostJournalID        *string                  `json:"glHostJournalId,omitempty"`
	DeliveredAt            *time.Time               `json:"deliveredAt,omitempty"`
	RetryCount             int                      `json:"retryCount"`
	LastRetryAt            *time.Time               `json:"lastRetryAt,omitempty"`
	LastError              *string                  `json:"lastError,omitempty"`
	FailureCategory        *string                  `json:"failureCategory,omitempty"`
	DeliveryMode           string                   `json:"deliveryMode"`
	PayloadSentAt          *time.Time               `json:"payloadSentAt,omitempty"`
	CanRetry               bool                     `json:"canRetry"`
	GlResponsePayloadJsonb *json.RawMessage         `json:"glResponsePayloadJsonb,omitempty"` // ROLE-IT-ADMIN only
	ManualRetryHistory     []ManualRetryHistoryItem `json:"manualRetryHistory,omitempty"`
	// Internal fields used by service layer.
	JurnalHeaderID uuid.UUID `json:"-"`
}

// ManualRetryHistoryItem is one entry in the manual retry history.
type ManualRetryHistoryItem struct {
	RetriedBy uuid.UUID `json:"retriedBy"`
	RetriedAt time.Time `json:"retriedAt"`
	Reason    string    `json:"reason"`
}

// DLQEntry maps to sys.dlq_gl_delivery.
type DLQEntry struct {
	ID                      uuid.UUID       `json:"id"`
	JurnalHeaderID          uuid.UUID       `json:"jurnalHeaderId"`
	GlStatusID              *uuid.UUID      `json:"glStatusId,omitempty"`
	PayloadJsonb            json.RawMessage `json:"payloadJsonb,omitempty"`
	ErrorCode               string          `json:"errorCode"`
	ErrorMessage            string          `json:"errorMessage"`
	ErrorCategory           string          `json:"errorCategory"`
	RetryCount              int             `json:"retryCount"`
	LastRetryAt             *time.Time      `json:"lastRetryAt,omitempty"`
	Status                  DLQStatus       `json:"status"`
	ReplayedBy              *uuid.UUID      `json:"replayedBy,omitempty"`
	ReplayedAt              *time.Time      `json:"replayedAt,omitempty"`
	FinalDeliveryResponseID *string         `json:"finalDeliveryResponseId,omitempty"`
	DiscardedReason         *string         `json:"discardedReason,omitempty"`
	DiscardedBy             *uuid.UUID      `json:"discardedBy,omitempty"`
	DiscardedAt             *time.Time      `json:"discardedAt,omitempty"`
	CreatedAt               time.Time       `json:"createdAt"`
	UpdatedAt               time.Time       `json:"updatedAt"`
	RowVersion              int64           `json:"rowVersion"`
	TenantID                string          `json:"tenantId"`

	// Denormalized from jrnl.header JOIN (for list view)
	NoJurnal       string     `json:"noJurnal,omitempty"`
	EventCode      string     `json:"eventCode,omitempty"`
	TanggalPosting *time.Time `json:"tanggalPosting,omitempty"`
}

// DLQEntrySummary is the lightweight list item for GET /jurnal/gl-delivery-dlq.
type DLQEntrySummary struct {
	DLQEntryID      uuid.UUID    `json:"dlqEntryId"`
	JurnalHeaderID  uuid.UUID    `json:"jurnalHeaderId"`
	NoJurnal        string       `json:"noJurnal"`
	EventCode       string       `json:"eventCode"`
	TanggalPosting  *time.Time   `json:"tanggalPosting,omitempty"`
	GlHostStatus    GlHostStatus `json:"glHostStatus"`
	Status          DLQStatus    `json:"status"`
	FailureCategory string       `json:"failureCategory"`
	ErrorCode       string       `json:"errorCode"`
	ErrorMessage    *string      `json:"errorMessage,omitempty"`
	RetryCount      int          `json:"retryCount"`
	LastRetryAt     *time.Time   `json:"lastRetryAt,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
	CanReplay       bool         `json:"canReplay"`
	CanDiscard      bool         `json:"canDiscard"`
}

// ReconciliationReport maps to sys.gl_reconciliation_report.
type ReconciliationReport struct {
	ID                  uuid.UUID        `json:"reportId"`
	TanggalRun          time.Time        `json:"tanggalRekonsiliasi"`
	TriggerSource       string           `json:"triggerSource"`
	TriggeredBy         *uuid.UUID       `json:"triggeredBy,omitempty"`
	AsynqJobID          *string          `json:"jobId,omitempty"`
	Status              ReconStatus      `json:"status"`
	StartedAt           time.Time        `json:"startedAt"`
	CompletedAt         *time.Time       `json:"generatedAt,omitempty"`
	TotalJurnalIDR      decimal.Decimal  `json:"blipsTotalIdr"`
	GlHostTotalIDR      *decimal.Decimal `json:"glHostTotalIdr,omitempty"`
	MismatchCount       int              `json:"totalMismatchCount"`
	ToleranceIDR        decimal.Decimal  `json:"toleranceIdr"`
	ErrorSummary        *string          `json:"errorSummary,omitempty"`
	SummaryJsonb        *json.RawMessage `json:"summaryJsonb,omitempty"`
	GlHostSnapshotJsonb *json.RawMessage `json:"glHostSnapshotJsonb,omitempty"`
	MismatchLines       []ReconMismatch  `json:"mismatchLines,omitempty"`

	// Computed for response
	TotalAkunChecked       int             `json:"totalAkunChecked"`
	TotalMismatchAmountIDR decimal.Decimal `json:"totalMismatchAmountIdr"`
	DeltaIDR               decimal.Decimal `json:"deltaIdr"`
}

// ReconSummaryItem is the lightweight list item for GET /jurnal/reconciliation/history.
type ReconSummaryItem struct {
	ReportID               uuid.UUID        `json:"reportId"`
	TanggalRekonsiliasi    time.Time        `json:"tanggalRekonsiliasi"`
	Status                 ReconStatus      `json:"status"`
	TotalAkunChecked       int              `json:"totalAkunChecked"`
	TotalMismatchCount     int              `json:"totalMismatchCount"`
	TotalMismatchAmountIDR *decimal.Decimal `json:"totalMismatchAmountIdr,omitempty"`
	DeltaIDR               *decimal.Decimal `json:"deltaIdr,omitempty"`
	GeneratedAt            *time.Time       `json:"generatedAt,omitempty"`
	JobID                  *string          `json:"jobId,omitempty"`
}

// ReconMismatch maps to sys.gl_recon_mismatch.
type ReconMismatch struct {
	ID              uuid.UUID       `json:"id"`
	ReportID        uuid.UUID       `json:"reportId"`
	AkunID          uuid.UUID       `json:"akunId"`
	KodeAkun        string          `json:"kodeAkun"`
	NamaAkun        *string         `json:"namaAkun,omitempty"`
	BlipsAmountIDR  decimal.Decimal `json:"blipsAmountIdr"`
	GlHostAmountIDR decimal.Decimal `json:"glHostAmountIdr"`
	DeltaIDR        decimal.Decimal `json:"deltaIdr"`
	MismatchType    MismatchType    `json:"mismatchType"`
	JurnalHeaderIDs []uuid.UUID     `json:"jurnalHeaderIds,omitempty"`
	Note            *string         `json:"note,omitempty"`
}

// AkunTotal is one account total from GL Host daily summary.
type AkunTotal struct {
	KodeAkun     string          `json:"kodeAkun"`
	NetAmountIDR decimal.Decimal `json:"netAmountIdr"`
}

// ─── GL Host Payload ──────────────────────────────────────────────────────────

// DeliveryPayload is the request body sent to the GL Host REST endpoint.
// PII fields MUST be sanitized before this struct is sent or persisted.
type DeliveryPayload struct {
	IdempotencyKey string         `json:"idempotency_key"`
	JournalDate    string         `json:"journal_date"` // YYYY-MM-DD
	Reference      string         `json:"reference"`    // no_jurnal
	EventCode      string         `json:"event_code"`
	Narrative      string         `json:"narrative"`
	Lines          []DeliveryLine `json:"lines"`
	Metadata       map[string]any `json:"metadata"`
}

// DeliveryLine is one debit/kredit line in the GL Host payload.
type DeliveryLine struct {
	AccountCode string          `json:"account_code"`
	Debit       decimal.Decimal `json:"debit"`
	Kredit      decimal.Decimal `json:"kredit"`
	Currency    string          `json:"currency"`
	Narasi      string          `json:"narasi,omitempty"`
}

// DeliveryResponse is the response from GL Host on successful delivery.
type DeliveryResponse struct {
	GlResponseID     string          `json:"gl_response_id"`
	HTTPStatus       int             `json:"http_status"`
	RawResponseJsonb json.RawMessage `json:"raw_response_jsonb,omitempty"`
}

// ─── Request/Response types ───────────────────────────────────────────────────

// RetryGlDeliveryRequest is the body for POST /jurnal/header/{id}/retry-gl-delivery.
type RetryGlDeliveryRequest struct {
	Reason string `json:"reason" binding:"required,min=30,max=1000"`
}

// RetryGlDeliveryResponse is the response for a successful retry enqueue.
type RetryGlDeliveryResponse struct {
	JobID              string       `json:"jobId"`
	StatusURL          string       `json:"statusUrl"`
	GlStatusID         uuid.UUID    `json:"glStatusId"`
	PreviousStatus     GlHostStatus `json:"previousStatus"`
	NewStatus          GlHostStatus `json:"newStatus"`
	RetryAttemptNumber int          `json:"retryAttemptNumber"`
}

// RunReconciliationRequest is the body for POST /jurnal/reconciliation/run.
type RunReconciliationRequest struct {
	Date   string  `json:"date" binding:"required"`
	Reason *string `json:"reason,omitempty"`
}

// RunReconciliationResponse is the response for a reconciliation run trigger.
type RunReconciliationResponse struct {
	JobID               string `json:"jobId"`
	StatusURL           string `json:"statusUrl"`
	StreamURL           string `json:"streamUrl"`
	TanggalRekonsiliasi string `json:"tanggalRekonsiliasi"`
}

// DlqActionRequest is the body for DLQ replay and discard endpoints.
type DlqActionRequest struct {
	Reason string `json:"reason" binding:"required,min=30,max=1000"`
}

// DlqReplayResponse is the response for a successful DLQ replay.
type DlqReplayResponse struct {
	JobID          string       `json:"jobId"`
	StatusURL      string       `json:"statusUrl"`
	DLQEntryID     uuid.UUID    `json:"dlqEntryId"`
	JurnalHeaderID uuid.UUID    `json:"jurnalHeaderId"`
	NoJurnal       string       `json:"noJurnal"`
	PreviousStatus GlHostStatus `json:"previousStatus"`
	NewStatus      GlHostStatus `json:"newStatus"`
}

// DlqDiscardResponse is the response for a successful DLQ discard.
type DlqDiscardResponse struct {
	DLQEntryID     uuid.UUID    `json:"dlqEntryId"`
	JurnalHeaderID uuid.UUID    `json:"jurnalHeaderId"`
	NoJurnal       string       `json:"noJurnal"`
	PreviousStatus GlHostStatus `json:"previousStatus"`
	NewStatus      GlHostStatus `json:"newStatus"`
	DiscardedAt    time.Time    `json:"discardedAt"`
	DiscardedBy    uuid.UUID    `json:"discardedBy"`
}
