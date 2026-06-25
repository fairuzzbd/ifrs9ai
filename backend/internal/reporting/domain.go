// Package reporting implements P5-M13 APP-E Reporting MV Foundation:
// 8 MV refresh (Asynq + PG advisory lock), export engine (CSV/XLSX/PDF +
// watermark + SHA-256), async export (MinIO + SMTP notif), scheduled email.
//
// References: P5-M13-S1..S5, DEC-007 (Asynq), DEC-018 (audit in-tx), DEC-021, DEC-022.
package reporting

import (
	"time"

	"github.com/google/uuid"
)

// AllMVNames lists all 8 materialized views managed by P5-M13.
var AllMVNames = []string{
	"rpt.mv_status_periode",
	"rpt.mv_jurnal_summary",
	"rpt.mv_gl_delivery_status",
	"rpt.mv_mtm_daily_summary",
	"rpt.mv_akrual_summary",
	"rpt.mv_renewal_summary",
	"rpt.mv_penjualan_summary",
	"rpt.mv_poci_delta_summary",
}

// ValidReportSlugs maps URL slug → MV name.
var ValidReportSlugs = map[string]string{
	"mv-status-periode":    "rpt.mv_status_periode",
	"mv-jurnal-summary":    "rpt.mv_jurnal_summary",
	"mv-gl-delivery-status": "rpt.mv_gl_delivery_status",
	"mv-mtm-daily-summary": "rpt.mv_mtm_daily_summary",
	"mv-akrual-summary":    "rpt.mv_akrual_summary",
	"mv-renewal-summary":   "rpt.mv_renewal_summary",
	"mv-penjualan-summary": "rpt.mv_penjualan_summary",
	"mv-poci-delta-summary": "rpt.mv_poci_delta_summary",
}

// TriggeredBy enumerates refresh trigger sources.
type TriggeredBy string

const (
	TriggeredByCron      TriggeredBy = "CRON"
	TriggeredByHardClose TriggeredBy = "HARD_CLOSE"
	TriggeredByManual    TriggeredBy = "MANUAL"
)

// MVRefreshStatus is the state of a materialized view refresh.
type MVRefreshStatus string

const (
	MVStatusIdle       MVRefreshStatus = "IDLE"
	MVStatusRefreshing MVRefreshStatus = "REFRESHING"
	MVStatusFailed     MVRefreshStatus = "FAILED"
)

// ExportFormat enumerates supported export formats.
type ExportFormat string

const (
	FormatCSV  ExportFormat = "csv"
	FormatXLSX ExportFormat = "xlsx"
	FormatPDF  ExportFormat = "pdf"
)

// IsValid returns true if format is a supported export format.
func (f ExportFormat) IsValid() bool {
	switch f {
	case FormatCSV, FormatXLSX, FormatPDF:
		return true
	}
	return false
}

// ExportStatus tracks async export job state.
type ExportStatus string

const (
	ExportStatusRequested ExportStatus = "REQUESTED"
	ExportStatusComputing ExportStatus = "COMPUTING"
	ExportStatusCompleted ExportStatus = "COMPLETED"
	ExportStatusFailed    ExportStatus = "FAILED"
)

// MVStatusItem is the API response for one MV's metadata.
type MVStatusItem struct {
	MVName        string          `json:"mvName"`
	Status        MVRefreshStatus `json:"status"`
	LastRefreshAt *time.Time      `json:"lastRefreshAt"`
	RowCount      *int64          `json:"rowCount"`
	LastError     *string         `json:"lastError"`
	TriggeredBy   *TriggeredBy    `json:"triggeredBy"`
}

// MVRefreshRequest is the handler request for POST /admin/mv-refresh.
type MVRefreshRequest struct {
	MVName *string `json:"mvName"` // nil = refresh all 8
}

// ExportRequest captures parameters for RequestExport.
type ExportRequest struct {
	ReportSlug string
	Format     ExportFormat
	PeriodeID  *uuid.UUID
	ActorID    uuid.UUID
	ActorRole  string
	TenantID   string
}

// ExportLogItem is the API response for one sys.export_log row.
type ExportLogItem struct {
	ID          uuid.UUID    `json:"id"`
	ReportSlug  string       `json:"reportSlug"`
	Format      ExportFormat `json:"format"`
	Status      ExportStatus `json:"status"`
	RowCount    *int64       `json:"rowCount"`
	FileSHA256  *string      `json:"fileSha256"`
	MinioPath   *string      `json:"minioPath"`
	ExpiresAt   *time.Time   `json:"expiresAt"`
	RequestedBy uuid.UUID    `json:"requestedBy"`
	RequestedAt time.Time    `json:"requestedAt"`
	CompletedAt *time.Time   `json:"completedAt"`
	DownloadedAt *time.Time  `json:"downloadedAt"`
}

// ScheduledEmailFrequency enumerates send frequencies.
type ScheduledEmailFrequency string

const (
	FreqDaily   ScheduledEmailFrequency = "daily"
	FreqWeekly  ScheduledEmailFrequency = "weekly"
	FreqMonthly ScheduledEmailFrequency = "monthly"
)

// ScheduledEmailCreateReq is the body for POST /reports/scheduled-emails.
type ScheduledEmailCreateReq struct {
	ReportSlug      string                  `json:"reportSlug"      binding:"required"`
	Format          ExportFormat            `json:"format"          binding:"required"`
	Frequency       ScheduledEmailFrequency `json:"frequency"       binding:"required"`
	SendTime        string                  `json:"sendTime"        binding:"required"`
	Recipients      []string                `json:"recipients"      binding:"required,min=1,max=50,dive,email"`
	Active          bool                    `json:"active"`
	SubjectTemplate string                  `json:"subjectTemplate"`
	BodyTemplate    string                  `json:"bodyTemplate"`
}

// ScheduledEmailItem is the API response for one sys.scheduled_email row.
type ScheduledEmailItem struct {
	ID             uuid.UUID               `json:"id"`
	ReportSlug     string                  `json:"reportSlug"`
	Format         ExportFormat            `json:"format"`
	Frequency      ScheduledEmailFrequency `json:"frequency"`
	SendTime       string                  `json:"sendTime"`
	Recipients     []string                `json:"recipients"`
	Active         bool                    `json:"active"`
	LastSentAt     *time.Time              `json:"lastSentAt"`
	LastStatus     *string                 `json:"lastStatus"`
	OptOutCount    int                     `json:"optOutCount"`
	CreatedAt      time.Time               `json:"createdAt"`
	CreatedBy      uuid.UUID               `json:"createdBy"`
}

// AsyncJobRef is returned from endpoints that enqueue Asynq jobs.
type AsyncJobRef struct {
	JobID     string `json:"jobId"`
	StatusURL string `json:"statusUrl"`
	StreamURL string `json:"streamUrl"`
}

// MVRefreshLog maps to sys.mv_refresh_log row.
type MVRefreshLog struct {
	ID          uuid.UUID   `db:"id"`
	MVName      string      `db:"mv_name"`
	TriggeredBy TriggeredBy `db:"triggered_by"`
	TriggerActor *uuid.UUID `db:"trigger_actor"`
	Status      string      `db:"status"`
	StartedAt   time.Time   `db:"started_at"`
	CompletedAt *time.Time  `db:"completed_at"`
	RowCount    *int64      `db:"row_count"`
	ErrorDetail *string     `db:"error_detail"`
	TenantID    string      `db:"tenant_id"`
}

// ExportLogRow maps to sys.export_log row.
type ExportLogRow struct {
	ID           uuid.UUID    `db:"id"`
	ReportSlug   string       `db:"report_slug"`
	Format       ExportFormat `db:"format"`
	Status       ExportStatus `db:"status"`
	RowCount     *int64       `db:"row_count"`
	MinioPath    *string      `db:"file_minio_path"`
	SHA256Hash   *string      `db:"sha256_hash"`
	SignedURL    *string      `db:"signed_url"`
	RequestedBy  uuid.UUID    `db:"requested_by"`
	RequestedAt  time.Time    `db:"requested_at"`
	CompletedAt  *time.Time   `db:"completed_at"`
	ExpiresAt    *time.Time   `db:"expires_at"`
	DownloadedAt *time.Time   `db:"downloaded_at"`
	JobID        *string      `db:"job_id"`
	TenantID     string       `db:"tenant_id"`
}

// ScheduledEmailRow maps to sys.scheduled_email row.
type ScheduledEmailRow struct {
	ID              uuid.UUID               `db:"id"`
	ReportSlug      string                  `db:"report_slug"`
	Format          ExportFormat            `db:"format"`
	Frequency       ScheduledEmailFrequency `db:"frequency"`
	SendTime        string                  `db:"send_time"`
	RecipientsJSON  []byte                  `db:"recipients_jsonb"`
	Active          bool                    `db:"active"`
	SubjectTemplate *string                 `db:"subject_template"`
	BodyTemplate    *string                 `db:"body_template"`
	LastSentAt      *time.Time              `db:"last_sent_at"`
	LastStatus      *string                 `db:"last_status"`
	LastError       *string                 `db:"last_error"`
	CreatedAt       time.Time               `db:"created_at"`
	CreatedBy       uuid.UUID               `db:"created_by"`
	TenantID        string                  `db:"tenant_id"`
}

// ReadIntent signals whether the caller needs primary or replica.
type ReadIntent int

const (
	// ReadIntentPrimary for mutations and critical reads.
	ReadIntentPrimary ReadIntent = 0
	// ReadIntentReporting for MV reads (→ replica).
	ReadIntentReporting ReadIntent = 1
)

// ExportWorkerPayload is the Asynq task payload for reporting:export-async.
type ExportWorkerPayload struct {
	ExportLogID string `json:"export_log_id"`
	ReportSlug  string `json:"report_slug"`
	Format      string `json:"format"`
	TenantID    string `json:"tenant_id"`
	ActorID     string `json:"actor_id"`
}

// MVRefreshPayload is the Asynq task payload for reporting:mv-refresh.
type MVRefreshPayload struct {
	MVName      string `json:"mv_name"`       // specific MV; "" = all
	TriggeredBy string `json:"triggered_by"`
	TriggerActor string `json:"trigger_actor"` // UUID string; "" for CRON
	TenantID    string `json:"tenant_id"`
}

// ScheduledEmailPayload is the Asynq task payload for reporting:scheduled-email-send.
type ScheduledEmailPayload struct {
	ScheduledEmailID string `json:"scheduled_email_id"`
	TenantID         string `json:"tenant_id"`
}

// OptOutRequest is the validated opt-out request (no auth required).
type OptOutRequest struct {
	ScheduledEmailID uuid.UUID
	Email            string
	Token            string
}
