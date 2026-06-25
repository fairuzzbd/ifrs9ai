package kurs

// P5-M5 extension to domain.go.
// Adds: Treatment enum, JisdorFetchResult, upload-batch request/response types,
//       new error codes, FxLocker interface.
// Do NOT add import cycles — only std-lib + approved deps used here.

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ─── Additional error codes (P5-M5) ──────────────────────────────────────────

const (
	// CodeFxRateLocked is returned when periode hard-closed blocks FX mutation. HTTP 423.
	CodeFxRateLocked = "FX_RATE_LOCKED"

	// CodeKursUploadValidationFailed is returned when manual upload CSV/XLSX has row errors. HTTP 422.
	CodeKursUploadValidationFailed = "KURS_UPLOAD_VALIDATION_FAILED"

	// CodeKlasiPeriodeMismatch is returned when instrumen klasifikasi periode mismatches. HTTP 422.
	CodeKlasiPeriodeMismatch = "KURS_PERIODE_MISMATCH"

	// CodeKlasiNotLocked is returned when instrumen klasifikasi not yet locked/approved. HTTP 422.
	CodeKlasiNotLocked = "KLASIFIKASI_NOT_LOCKED"
)

// HTTPStatus returns the HTTP status code for P5-M5 kurs-specific codes.
// The callers (newKursErrP5M5) use this. For registration in the central
// domainerrors switch, see backend/internal/common/errors/domain.go.
func kursCodeHTTPStatus(code string) int {
	switch code {
	case CodeFxRateLocked:
		return 423
	case CodeKursUploadValidationFailed, CodeKlasiPeriodeMismatch, CodeKlasiNotLocked:
		return 422
	default:
		return 500
	}
}

// ─── Treatment enum (PSAK 71 FX accounting treatment) ─────────────────────────

// Treatment describes how an FX-denominated instrument records currency movements.
type Treatment string

const (
	// TreatmentPnL means: FX gain/loss goes to Profit & Loss.
	// Applies to: AC+FCY, FVTPL+FCY.
	TreatmentPnL Treatment = "P_AND_L"

	// TreatmentOCIRecyclable means: FX gain/loss goes to OCI (recyclable on disposal).
	// Applies to: FVOCI_DEBT+FCY.
	TreatmentOCIRecyclable Treatment = "OCI_RECYCLABLE"

	// TreatmentOCINoRecycle means: FX gain/loss goes to OCI (no recycling to P&L).
	// Applies to: FVOCI_ELECTION equity instrument + FCY.
	TreatmentOCINoRecycle Treatment = "OCI_NO_RECYCLE"

	// TreatmentNoFX means: instrument is IDR-denominated; no FX treatment needed.
	TreatmentNoFX Treatment = "NO_FX_TREATMENT"
)

// ─── FxLocker interface (P5-M4 ↔ P5-M5 contract) ────────────────────────────

// FxLocker is implemented by kurs.Service to lock/unlock FX rates for a periode.
// closeflow.Service depends on this interface so the kurs package owns the SQL.
// Injected via closeflow.NewService(..., fxLocker FxLocker).
//
// NOTE: ctx and tx must be context.Context and *database/sql.Tx respectively.
// Using named imports here to avoid an import cycle since kurs and closeflow
// both live under internal/. The concrete implementations in service_p5m5.go
// use the correct types.
type FxLocker interface {
	// LockRatesForPeriode sets locked_flag=TRUE on all mst.kurs rows for the periode.
	// Must be called INSIDE the same *sql.Tx as the hard-close commit (same tx).
	LockRatesForPeriode(periodeID uuid.UUID) error

	// UnlockRatesForPeriode sets locked_flag=FALSE (on CLOSED → SOFT_CLOSED reopen).
	UnlockRatesForPeriode(periodeID uuid.UUID) error
}

// ─── Provider interface ───────────────────────────────────────────────────────

// FxRateProvider is the abstraction for JISDOR and other external FX rate sources.
// Implemented by JISDORAdapter (real) and MockAdapter (test).
type FxRateProvider interface {
	// FetchRates fetches rates for all configured currencies for the given date.
	// tanggalBerlaku must be "YYYY-MM-DD". Returns one JisdorRateRow per currency.
	FetchRates(tanggalBerlaku string) ([]JisdorRateRow, error)

	// Name returns a stable identifier string (e.g. "JISDOR", "MOCK").
	Name() string
}

// JisdorRateRow is one currency result from the provider.
type JisdorRateRow struct {
	KodeMataUang string          // 3-char ISO 4217
	KursTengah   decimal.Decimal // mid rate; required
	KursBeli     *decimal.Decimal
	KursJual     *decimal.Decimal
}

// ─── JISDOR fetch result ──────────────────────────────────────────────────────

// JisdorFetchResult is the aggregate outcome for one JISDORFetchAll invocation.
type JisdorFetchResult struct {
	TanggalBerlaku string              `json:"tanggalBerlaku"`
	JobID          string              `json:"jobId"`
	StatusURL      string              `json:"statusUrl"`
	TotalRequested int                 `json:"totalRequested"`
	Inserted       int                 `json:"inserted"`
	Skipped        int                 `json:"skipped"` // duplicate dates
	Errors         []JisdorFetchError  `json:"errors"`
	AutoApproved   bool                `json:"autoApproved"`
}

// JisdorFetchError captures a per-currency failure.
type JisdorFetchError struct {
	KodeMataUang string `json:"kodeMataUang"`
	Error        string `json:"error"`
}

// ─── Upload batch types ───────────────────────────────────────────────────────

// UploadBatchResponse is returned by POST /master/kurs/upload.
type UploadBatchResponse struct {
	BatchID        string              `json:"batchId"`
	TotalRows      int                 `json:"totalRows"`
	ValidRows      int                 `json:"validRows"`
	InvalidRows    int                 `json:"invalidRows"`
	ValidationErrs []UploadRowError    `json:"validationErrors,omitempty"`
	Message        string              `json:"message"`
}

// UploadRowError describes one invalid row in an uploaded file.
type UploadRowError struct {
	RowNumber int    `json:"rowNumber"`
	Field     string `json:"field"`
	Error     string `json:"error"`
}

// BatchApproveRequest is the body for POST /master/kurs/upload/{batch_id}/approve.
type BatchApproveRequest struct {
	Comment         string `json:"comment"`
	SignatureMethod  string `json:"signatureMethod"`
}

// BatchApproveResponse is returned after approving a batch.
type BatchApproveResponse struct {
	BatchID       string `json:"batchId"`
	ApprovedCount int    `json:"approvedCount"`
	Message       string `json:"message"`
}

// BatchRejectRequest is the body for POST /master/kurs/upload/{batch_id}/reject.
type BatchRejectRequest struct {
	RejectReason   string `json:"rejectReason"   binding:"required,min=20"`
	SignatureMethod string `json:"signatureMethod"`
}

// BatchRejectResponse is returned after rejecting a batch.
type BatchRejectResponse struct {
	BatchID      string `json:"batchId"`
	RejectedCount int   `json:"rejectedCount"`
	Message      string `json:"message"`
}

// ─── Treatment response ───────────────────────────────────────────────────────

// TreatmentResponse is returned by GET /master/kurs/treatment/{instrumen_id}.
type TreatmentResponse struct {
	InstrumenID    string    `json:"instrumenId"`
	KodeMataUang   string    `json:"kodeMataUang"`
	Klasifikasi    string    `json:"klasifikasi"`
	Treatment      Treatment `json:"treatment"`
	Reasoning      string    `json:"reasoning"`
}

// ─── Kurs P5-M5 extended fields (added by migration 000039) ──────────────────

// KursP5M5Fields holds the new columns from migration 000039.
// Embedded or composed into Kurs for P5-M5 scans.
type KursP5M5Fields struct {
	DeviationFlag        bool             `db:"deviation_flag"`
	RateDeviationPct     *decimal.Decimal `db:"rate_deviation_pct"`
	JisdorFetchMetadata  *[]byte          `db:"jisdor_fetch_metadata"` // raw JSONB
	RejectReason         *string          `db:"reject_reason"`
	UploadBatchID        *uuid.UUID       `db:"upload_batch_id"`
}

// ─── Asynq task types ─────────────────────────────────────────────────────────

const (
	// TaskFxJisdorFetch is the Asynq task type for the daily JISDOR cron.
	TaskFxJisdorFetch = "fx:jisdor-fetch"

	// TaskFxUploadProcess is the Asynq task type for async upload processing.
	TaskFxUploadProcess = "fx:upload-process"
)
