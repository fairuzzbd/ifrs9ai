// Package pdpefindo implements the mst.pd_pefindo master-data module (APP-A-MSTR ECL Param).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Follows the same directory shape as internal/master/matauang (pilot pattern).
//
// Key differences from matauang:
//   - Uses decimal.Decimal for all PD fields (DEC-016 — no float64 for rates)
//   - 6-eyes workflow (ROLE-RISK → ROLE-AKUN-CTL → ROLE-ALCO × 2)
//   - Both approve + approve2 require step-up MFA (DEC-027)
//   - PD monotonicity validation: pd_12month ≤ pd_3y ≤ pd_5y ≤ pd_7y ≤ pd_10y
//   - XLSX batch upload via Asynq async job (UX rule §3)
package pdpefindo

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ─────────────────────────────────────────────────────────────

const (
	// CodePDMonotonicityViolated is returned when PD values violate the monotonicity
	// constraint: pd_12month ≤ pd_lifetime_3y ≤ pd_lifetime_5y ≤ pd_lifetime_7y ≤ pd_lifetime_10y.
	// HTTP 422.
	CodePDMonotonicityViolated = string(domainerrors.CodePDMonotonicityViolated)

	// CodePDPeriodOverlap is returned when a new PD record would overlap an existing
	// period for the same rating. HTTP 422.
	CodePDPeriodOverlap = string(domainerrors.CodePDPeriodOverlap)

	// CodeEntityInUse is returned when soft-delete is blocked by active references.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)

	// CodeMasterApprovedNoEdit is returned when caller tries to UPDATE an APPROVED record.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)
)

// ─── Pefindo rating whitelist ─────────────────────────────────────────────────

// PefindoRatings is the complete ordered whitelist of Pefindo ratings from
// Pefindo_Annual_Default_Study_2007-2025 appendix. Enforced at service layer.
// DB CHECK deferred to migration 0014.
//
// idD is a special case: both pd_12month and all pd_lifetime_* MUST be 1.0 (certain default).
var PefindoRatings = []string{
	"idAAA",
	"idAA+", "idAA", "idAA-",
	"idA+", "idA", "idA-",
	"idBBB+", "idBBB", "idBBB-",
	"idBB+", "idBB", "idBB-",
	"idB+", "idB", "idB-",
	"idCCC",
	"idCC",
	"idC",
	"idD",
}

// pefindoRatingSet is a pre-built set for O(1) lookup.
var pefindoRatingSet = func() map[string]bool {
	m := make(map[string]bool, len(PefindoRatings))
	for _, r := range PefindoRatings {
		m[r] = true
	}
	return m
}()

// IsValidPefindoRating reports whether r is in the Pefindo whitelist.
func IsValidPefindoRating(r string) bool {
	return pefindoRatingSet[r]
}

// ─── WorkflowStatus ───────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values allowed in mst.pd_pefindo.workflow_status.
type WorkflowStatus string

const (
	WorkflowStatusDraft            WorkflowStatus = "DRAFT"
	WorkflowStatusPendingReview    WorkflowStatus = "PENDING_REVIEW"
	WorkflowStatusPendingApproval  WorkflowStatus = "PENDING_APPROVAL"
	WorkflowStatusPendingApproval2 WorkflowStatus = "PENDING_APPROVAL_2"
	WorkflowStatusApproved         WorkflowStatus = "APPROVED"
	WorkflowStatusRejected         WorkflowStatus = "REJECTED"
	WorkflowStatusReturned         WorkflowStatus = "RETURNED"
)

// IsEditable returns true if the record may be updated (not yet in review/approval).
func (s WorkflowStatus) IsEditable() bool {
	return s == WorkflowStatusDraft || s == WorkflowStatusRejected || s == WorkflowStatusReturned
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// PDPefindo is the domain entity for mst.pd_pefindo.
// All PD fields use decimal.Decimal per DEC-016 (no float64 for rates).
type PDPefindo struct {
	ID uuid.UUID `db:"id"`

	// Business key
	Rating string `db:"rating"`

	// PD values — NUMERIC(10,8) in DB → decimal.Decimal in Go
	PD12Month     decimal.Decimal  `db:"pd_12month"`
	PDLifetime3Y  *decimal.Decimal `db:"pd_lifetime_3y"`
	PDLifetime5Y  *decimal.Decimal `db:"pd_lifetime_5y"`
	PDLifetime7Y  *decimal.Decimal `db:"pd_lifetime_7y"`
	PDLifetime10Y *decimal.Decimal `db:"pd_lifetime_10y"`

	// Metadata
	Sumber               string  `db:"sumber"`
	TanggalPublikasi     *string `db:"tanggal_publikasi"`      // DATE → "YYYY-MM-DD"
	PeriodeBerlakuDari   string  `db:"periode_berlaku_dari"`   // DATE
	PeriodeBerlakuSampai *string `db:"periode_berlaku_sampai"` // DATE, nullable

	// Document reference
	DokumenPendukungID *uuid.UUID `db:"dokumen_pendukung_id"`

	// Legacy fields (read-only, from 0001 schema — kept as history)
	UploadedBy uuid.UUID  `db:"uploaded_by"`
	UploadedAt time.Time  `db:"uploaded_at"`
	ApprovedBy *uuid.UUID `db:"approved_by"`
	ApprovedAt *time.Time `db:"approved_at"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit

	// Standard audit fields (added by migration 0013)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  *uuid.UUID `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST body for creating a pd_pefindo record.
// The sumber field defaults to PEFINDO_ANNUAL_DEFAULT_STUDY when empty.
type CreateRequest struct {
	Rating               string           `json:"rating"                binding:"required"`
	PD12Month            decimal.Decimal  `json:"pd12Month"             binding:"required"`
	PDLifetime3Y         *decimal.Decimal `json:"pdLifetime3Y"`
	PDLifetime5Y         *decimal.Decimal `json:"pdLifetime5Y"`
	PDLifetime7Y         *decimal.Decimal `json:"pdLifetime7Y"`
	PDLifetime10Y        *decimal.Decimal `json:"pdLifetime10Y"`
	Sumber               string           `json:"sumber"`
	TanggalPublikasi     *string          `json:"tanggalPublikasi"`
	PeriodeBerlakuDari   string           `json:"periodeBerlakuDari"   binding:"required"`
	PeriodeBerlakuSampai *string          `json:"periodeBerlakuSampai"`
	DokumenPendukungID   *string          `json:"dokumenPendukungId"`
}

// UpdateRequest is the PATCH body. All PD fields are pointer (omitempty = no update).
type UpdateRequest struct {
	PD12Month            *decimal.Decimal `json:"pd12Month"`
	PDLifetime3Y         *decimal.Decimal `json:"pdLifetime3Y"`
	PDLifetime5Y         *decimal.Decimal `json:"pdLifetime5Y"`
	PDLifetime7Y         *decimal.Decimal `json:"pdLifetime7Y"`
	PDLifetime10Y        *decimal.Decimal `json:"pdLifetime10Y"`
	Sumber               *string          `json:"sumber"`
	TanggalPublikasi     *string          `json:"tanggalPublikasi"`
	PeriodeBerlakuDari   *string          `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string          `json:"periodeBerlakuSampai"`
	DokumenPendukungID   *string          `json:"dokumenPendukungId"`
	RowVersion           int64            `json:"rowVersion"            binding:"required"`
}

// Response is the JSON representation returned by CRUD + workflow endpoints.
type Response struct {
	ID                   string  `json:"id"`
	Rating               string  `json:"rating"`
	PD12Month            string  `json:"pd12Month"`
	PDLifetime3Y         *string `json:"pdLifetime3Y"`
	PDLifetime5Y         *string `json:"pdLifetime5Y"`
	PDLifetime7Y         *string `json:"pdLifetime7Y"`
	PDLifetime10Y        *string `json:"pdLifetime10Y"`
	Sumber               string  `json:"sumber"`
	TanggalPublikasi     *string `json:"tanggalPublikasi"`
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	DokumenPendukungID   *string `json:"dokumenPendukungId"`
	WorkflowStatus       string  `json:"workflowStatus"`
	WorkflowInstanceID   *string `json:"workflowInstanceId"`
	RowVersion           int64   `json:"rowVersion"`
	CreatedAt            string  `json:"createdAt"`
	CreatedBy            *string `json:"createdBy"`
	UpdatedAt            *string `json:"updatedAt"`
	UpdatedBy            *string `json:"updatedBy"`
	DeletedAt            *string `json:"deletedAt"`
}

// ToResponse converts a domain entity to JSON response shape.
func ToResponse(p *PDPefindo) Response {
	r := Response{
		ID:                 p.ID.String(),
		Rating:             p.Rating,
		PD12Month:          p.PD12Month.String(),
		Sumber:             p.Sumber,
		PeriodeBerlakuDari: p.PeriodeBerlakuDari,
		WorkflowStatus:     string(displayWorkflowStatus(p.WorkflowStatus)),
		RowVersion:         p.RowVersion,
		CreatedAt:          p.CreatedAt.Format(time.RFC3339),
	}
	if p.PDLifetime3Y != nil {
		s := p.PDLifetime3Y.String()
		r.PDLifetime3Y = &s
	}
	if p.PDLifetime5Y != nil {
		s := p.PDLifetime5Y.String()
		r.PDLifetime5Y = &s
	}
	if p.PDLifetime7Y != nil {
		s := p.PDLifetime7Y.String()
		r.PDLifetime7Y = &s
	}
	if p.PDLifetime10Y != nil {
		s := p.PDLifetime10Y.String()
		r.PDLifetime10Y = &s
	}
	if p.TanggalPublikasi != nil {
		r.TanggalPublikasi = p.TanggalPublikasi
	}
	if p.PeriodeBerlakuSampai != nil {
		r.PeriodeBerlakuSampai = p.PeriodeBerlakuSampai
	}
	if p.DokumenPendukungID != nil {
		s := p.DokumenPendukungID.String()
		r.DokumenPendukungID = &s
	}
	if p.WorkflowInstanceID != nil {
		s := p.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if p.CreatedBy != nil {
		s := p.CreatedBy.String()
		r.CreatedBy = &s
	}
	if p.UpdatedAt != nil {
		s := p.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if p.UpdatedBy != nil {
		s := p.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if p.DeletedAt != nil {
		s := p.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	return r
}

// displayWorkflowStatus maps REJECTED → RETURNED for external visibility.
func displayWorkflowStatus(s WorkflowStatus) WorkflowStatus {
	if s == WorkflowStatusRejected {
		return WorkflowStatusReturned
	}
	return s
}

// DeleteResponse is returned by soft-delete endpoint.
type DeleteResponse struct {
	Deleted   bool   `json:"deleted"`
	DeletedAt string `json:"deletedAt"`
	EntityID  string `json:"entityId"`
}

// ActiveCurveResponse is one item in the GET /master/pd-pefindo/active response.
// Shape is designed for ECL engine consumption (Phase 4 placeholder contract).
type ActiveCurveResponse struct {
	ID                   string  `json:"id"`
	Rating               string  `json:"rating"`
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	PD12Month            string  `json:"pd12Month"`
	PDLifetime3Y         *string `json:"pdLifetime3Y"`
	PDLifetime5Y         *string `json:"pdLifetime5Y"`
	PDLifetime7Y         *string `json:"pdLifetime7Y"`
	PDLifetime10Y        *string `json:"pdLifetime10Y"`
	WorkflowStatus       string  `json:"workflowStatus"`
}

// ToActiveCurveResponse converts a PDPefindo entity to the active-curve response shape.
func ToActiveCurveResponse(p *PDPefindo) ActiveCurveResponse {
	r := ActiveCurveResponse{
		ID:                 p.ID.String(),
		Rating:             p.Rating,
		PeriodeBerlakuDari: p.PeriodeBerlakuDari,
		PD12Month:          p.PD12Month.String(),
		WorkflowStatus:     string(p.WorkflowStatus),
	}
	if p.PeriodeBerlakuSampai != nil {
		r.PeriodeBerlakuSampai = p.PeriodeBerlakuSampai
	}
	if p.PDLifetime3Y != nil {
		s := p.PDLifetime3Y.String()
		r.PDLifetime3Y = &s
	}
	if p.PDLifetime5Y != nil {
		s := p.PDLifetime5Y.String()
		r.PDLifetime5Y = &s
	}
	if p.PDLifetime7Y != nil {
		s := p.PDLifetime7Y.String()
		r.PDLifetime7Y = &s
	}
	if p.PDLifetime10Y != nil {
		s := p.PDLifetime10Y.String()
		r.PDLifetime10Y = &s
	}
	return r
}

// AuditHistoryItem is one aud.audit_log row for pd_pefindo.
type AuditHistoryItem struct {
	EventID     string      `json:"eventId"`
	EventTime   string      `json:"eventTime"`
	ActorUserID string      `json:"actorUserId"`
	ActorRole   string      `json:"actorRole"`
	Action      string      `json:"action"`
	BeforeJSONB interface{} `json:"beforeJsonb"`
	AfterJSONB  interface{} `json:"afterJsonb"`
	IP          *string     `json:"ip"`
	TraceID     *string     `json:"traceId"`
}

// UploadXLSXResponse is returned by the upload-xlsx endpoint (202 Accepted).
type UploadXLSXResponse struct {
	JobID     string `json:"jobId"`
	StatusURL string `json:"statusUrl"`
	StreamURL string `json:"streamUrl"`
}

// JobStatusResponse is returned by GET /upload-jobs/:jobId.
type JobStatusResponse struct {
	JobID                 string      `json:"jobId"`
	Type                  string      `json:"type"`
	Status                string      `json:"status"`
	Progress              int         `json:"progress"`
	CurrentStep           *string     `json:"currentStep"`
	StartedAt             *string     `json:"startedAt"`
	EstimatedCompletionAt *string     `json:"estimatedCompletionAt"`
	Result                interface{} `json:"result"`
	Error                 interface{} `json:"error"`
	CanCancel             bool        `json:"canCancel"`
}

// WorkflowActionRequest is the request body for submit/review/approve/approve2/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds mandatory comment for reject.
type WorkflowRejectRequest struct {
	Comment         string `json:"comment"        binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod"`
	RowVersion      *int64 `json:"rowVersion"`
}

// ─── Query config ─────────────────────────────────────────────────────────────

// AllowedSortCols is the whitelist of sortable columns.
var AllowedSortCols = []string{
	"id",
	"rating",
	"pd_12month",
	"sumber",
	"tanggal_publikasi",
	"periode_berlaku_dari",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist of filterable columns.
var AllowedFilterCols = []string{
	"rating",
	"sumber",
	"workflow_status",
	"periode_berlaku_dari",
}

// SearchCols are the columns scanned by ?q= text search.
var SearchCols = []string{"rating", "sumber"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = func() []string {
	combined := make([]string, 0, len(AllowedSortCols)+len(AllowedFilterCols))
	combined = append(combined, AllowedSortCols...)
	combined = append(combined, AllowedFilterCols...)
	return combined
}()

// DefaultSumber is the default value for the sumber field.
const DefaultSumber = "PEFINDO_ANNUAL_DEFAULT_STUDY"
