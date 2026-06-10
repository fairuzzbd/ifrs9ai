// Package lpscoverage implements the mst.lps_coverage master-data module (APP-A).
//
// LPS Coverage is the IDR 2-miliar per-nasabah-per-bank guarantee cap from
// Lembaga Penjamin Simpanan (DEC-014). This record drives the LPS Aggregator
// in the ECL engine: balances up to coverage_amount are excluded from ECL;
// only the excess is risk-weighted.
//
// Workflow: 6-eyes (DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED)
// Both APPROVE steps require step-up MFA (ALCO approvers).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// Domain rules enforced here (service layer mirrors DB CHECK constraints):
//  1. coverage_amount > 0
//  2. mata_uang = 'IDR' (Indonesia-only per DEC-014)
//  3. periode_berlaku_sampai >= periode_berlaku_dari (when both present)
//  4. Period single-active invariant: no two APPROVED rows may have overlapping date ranges.
//     New row can only be created after the prior active row has its periode_berlaku_sampai set.
//     HTTP 422 LPS_PERIOD_OVERLAP when violated.
package lpscoverage

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeLPSPeriodOverlap is returned when a new record would create an overlapping
	// active period with an existing APPROVED record. HTTP 422.
	CodeLPSPeriodOverlap = string(domainerrors.CodeLPSPeriodOverlap)

	// CodeMasterApprovedNoEdit is returned when editing an APPROVED record. HTTP 403.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)

	// CodeEntityInUse is returned when soft-delete is blocked. HTTP 409.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)
)

// ─── Workflow status ──────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values in mst.lps_coverage.workflow_status.
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

// editableStatuses is the set of workflow_status values that allow data edits.
var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusReturned: true,
	WorkflowStatusRejected: true, // same as RETURNED at DB level
}

// IsEditable returns true if the record is in an editable state.
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// LPSCoverage is the domain entity for mst.lps_coverage.
// coverage_amount uses shopspring/decimal (DEC-016 — no float64 for money).
type LPSCoverage struct {
	ID uuid.UUID `db:"id"`

	// Core business fields
	CoverageAmount       decimal.Decimal `db:"coverage_amount"`
	MataUang             string          `db:"mata_uang"`
	PeriodeBerlakuDari   string          `db:"periode_berlaku_dari"`   // DATE → "YYYY-MM-DD"
	PeriodeBerlakuSampai *string         `db:"periode_berlaku_sampai"` // DATE nullable
	RegulasiReferensi    *string         `db:"regulasi_referensi"`
	DokumenPendukungID   *uuid.UUID      `db:"dokumen_pendukung_id"`

	// Legacy fields (kept, not actively used in service logic)
	MakerID    uuid.UUID  `db:"maker_id"`
	ApproverID *uuid.UUID `db:"approver_id"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"`

	// Audit fields
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

// CreateRequest is the POST /master/lps-coverage request body.
type CreateRequest struct {
	// CoverageAmount is the LPS guarantee cap in IDR.
	// Must be positive. Default per DEC-014: 2_000_000_000.
	CoverageAmount       string  `json:"coverageAmount"      binding:"required"`
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"  binding:"required"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"` // optional
	RegulasiReferensi    *string `json:"regulasiReferensi"`    // optional, max 200 chars
	DokumenPendukungID   *string `json:"dokumenPendukungId"`   // optional UUID
}

// UpdateRequest is the PUT /master/lps-coverage/:id request body.
type UpdateRequest struct {
	CoverageAmount       *string `json:"coverageAmount"`
	PeriodeBerlakuDari   *string `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"` // set to null to clear
	RegulasiReferensi    *string `json:"regulasiReferensi"`
	DokumenPendukungID   *string `json:"dokumenPendukungId"`
	RowVersion           int64   `json:"rowVersion"           binding:"required"`
}

// Response is the JSON representation returned by CRUD and workflow endpoints.
type Response struct {
	ID                   string  `json:"id"`
	CoverageAmount       string  `json:"coverageAmount"`
	MataUang             string  `json:"mataUang"`
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	RegulasiReferensi    *string `json:"regulasiReferensi"`
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

// ToResponse converts a domain entity to the JSON response shape.
func ToResponse(lc *LPSCoverage) Response {
	r := Response{
		ID:                 lc.ID.String(),
		CoverageAmount:     lc.CoverageAmount.StringFixed(4),
		MataUang:           lc.MataUang,
		PeriodeBerlakuDari: lc.PeriodeBerlakuDari,
		WorkflowStatus:     string(displayWorkflowStatus(lc.WorkflowStatus)),
		RowVersion:         lc.RowVersion,
		CreatedAt:          lc.CreatedAt.Format(time.RFC3339),
	}
	r.PeriodeBerlakuSampai = lc.PeriodeBerlakuSampai
	r.RegulasiReferensi = lc.RegulasiReferensi
	if lc.DokumenPendukungID != nil {
		s := lc.DokumenPendukungID.String()
		r.DokumenPendukungID = &s
	}
	if lc.WorkflowInstanceID != nil {
		s := lc.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if lc.CreatedBy != nil {
		s := lc.CreatedBy.String()
		r.CreatedBy = &s
	}
	if lc.UpdatedAt != nil {
		s := lc.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if lc.UpdatedBy != nil {
		s := lc.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if lc.DeletedAt != nil {
		s := lc.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	return r
}

// displayWorkflowStatus maps DB REJECTED → API RETURNED.
func displayWorkflowStatus(s WorkflowStatus) WorkflowStatus {
	if s == WorkflowStatusRejected {
		return WorkflowStatusReturned
	}
	return s
}

// DeleteResponse is returned by soft-delete.
type DeleteResponse struct {
	Deleted   bool   `json:"deleted"`
	DeletedAt string `json:"deletedAt"`
	EntityID  string `json:"entityId"`
}

// AuditHistoryItem represents one aud.audit_log row for lps_coverage.
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

// ─── Allowed columns (whitelist) ──────────────────────────────────────────────

// AllowedSortCols is the whitelist of sortable columns.
var AllowedSortCols = []string{
	"id",
	"coverage_amount",
	"periode_berlaku_dari",
	"periode_berlaku_sampai",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist of filterable columns.
var AllowedFilterCols = []string{
	"workflow_status",
	"mata_uang",
	"periode_berlaku_dari",
	"periode_berlaku_sampai",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"regulasi_referensi"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// WorkflowActionRequest is the request body for submit/review/approve/approve2/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds the mandatory comment for reject.
type WorkflowRejectRequest struct {
	Comment         string `json:"comment"         binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod"`
	RowVersion      *int64 `json:"rowVersion"`
}
