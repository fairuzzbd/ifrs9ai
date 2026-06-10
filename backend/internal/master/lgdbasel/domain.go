// Package lgdbasel implements the mst.lgd_basel master-data module (APP-C ECL Parameter).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// This is an ECL parameter module and therefore has a BLOCKING ifrs9-compliance-reviewer
// gate on every PR that touches this package.
//
// Workflow: 6-eyes (DRAFT→PENDING_REVIEW→PENDING_APPROVAL→PENDING_APPROVAL_2→APPROVED).
// Both approve and approve2 require step-up MFA (DEC-027).
// SoD: approver2 ≠ maker ∧ ≠ reviewer ∧ ≠ approver1 (approver2NotAnyPrevious=true).
//
// LGD field uses shopspring/decimal.Decimal (DEC-016, never float64).
//
// Legacy columns maker_id / approver_id in mst.lgd_basel remain in DB for FK integrity
// (migration 0010). Service sets maker_id=currentUser.ID on create to satisfy the NOT NULL
// DB constraint but treats sys.workflow_instance as the sole source of truth for
// the approval chain. See createLegacyMakerID comment in service.go.
package lgdbasel

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeEntityInUse is returned when soft-delete is blocked by active references.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)

	// CodeMasterApprovedNoEdit is returned when caller tries to UPDATE an APPROVED record.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)

	// CodeLGDPeriodOverlap is returned when tipe_eksposur has an overlapping period.
	// HTTP 422.
	CodeLGDPeriodOverlap = string(domainerrors.CodeLGDPeriodOverlap)
)

// ─── TipeEksposur enum ─────────────────────────────────────────────────────────

// TipeEksposur enumerates the whitelist values for mst.lgd_basel.tipe_eksposur.
// Enforced by service layer; DB CHECK constraint deferred to migration 0011.
type TipeEksposur string

const (
	TipeEksposurSovereign   TipeEksposur = "SOVEREIGN"
	TipeEksposurBank        TipeEksposur = "BANK"
	TipeEksposurCorporate   TipeEksposur = "CORPORATE"
	TipeEksposurRetail      TipeEksposur = "RETAIL"
	TipeEksposurEquity      TipeEksposur = "EQUITY"
	TipeEksposurReinsurance TipeEksposur = "REINSURANCE"
)

// validTipeEksposur is the service-layer whitelist.
var validTipeEksposur = map[TipeEksposur]bool{
	TipeEksposurSovereign:   true,
	TipeEksposurBank:        true,
	TipeEksposurCorporate:   true,
	TipeEksposurRetail:      true,
	TipeEksposurEquity:      true,
	TipeEksposurReinsurance: true,
}

// IsValidTipeEksposur returns true if t is in the whitelist.
func IsValidTipeEksposur(t TipeEksposur) bool {
	return validTipeEksposur[t]
}

// ─── WorkflowStatus ───────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values allowed in mst.lgd_basel.workflow_status.
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
	WorkflowStatusRejected: true,
}

// IsEditable returns true if the record can be edited.
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// ─── Permission constants ──────────────────────────────────────────────────────

// Permission constants follow the {entity}.{action} pattern (api-conventions.md).
// lgd_basel shares ecl_parameter permissions (per WORKFLOW_CONFIG_LGD_BASEL seed).
const (
	PermCreate  = "ecl_parameter.create"
	PermRead    = "ecl_parameter.read"
	PermUpdate  = "ecl_parameter.update"
	PermDelete  = "ecl_parameter.delete"
	PermSubmit  = "ecl_parameter.submit"
	PermReview  = "ecl_parameter.review"
	PermApprove = "ecl_parameter.approve"
	PermReject  = "ecl_parameter.reject"
	PermExport  = "ecl_parameter.export"
)

// ─── Domain entity ─────────────────────────────────────────────────────────────

// LGDBasel is the domain entity for mst.lgd_basel.
// DEC-016: lgd uses shopspring/decimal.Decimal — never float64.
type LGDBasel struct {
	// Surrogate UUID primary key.
	ID uuid.UUID `db:"id"`

	// Business fields.
	TipeEksposur         TipeEksposur    `db:"tipe_eksposur"`
	LGD                  decimal.Decimal `db:"lgd"`                    // NUMERIC(8,4), [0,1]
	Karakteristik        *string         `db:"karakteristik"`          // free text, nullable
	PeriodeBerlakuDari   string          `db:"periode_berlaku_dari"`   // DATE → "YYYY-MM-DD"
	PeriodeBerlakuSampai *string         `db:"periode_berlaku_sampai"` // nullable = currently active
	Sumber               string          `db:"sumber"`                 // default 'BASEL_III_IRB'
	DokumenPendukungID   *uuid.UUID      `db:"dokumen_pendukung_id"`   // nullable Phase 3

	// Legacy columns — NOT used as source of truth for workflow, kept for DB NOT NULL compat.
	// Service sets maker_id=currentUser.ID on create only.
	MakerID    uuid.UUID  `db:"maker_id"`
	ApproverID *uuid.UUID `db:"approver_id"` // nullable

	// Workflow.
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit

	// Audit columns (added by migration 0010).
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  *uuid.UUID `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// ─── Allowed column whitelists ────────────────────────────────────────────────

// AllowedSortCols is the whitelist for ?sort= query param.
var AllowedSortCols = []string{
	"tipe_eksposur",
	"lgd",
	"periode_berlaku_dari",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist for ?filter[col]= query param.
var AllowedFilterCols = []string{
	"tipe_eksposur",
	"sumber",
	"workflow_status",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"tipe_eksposur", "sumber", "karakteristik"}

// AllAllowedCols is the union for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST /master/lgd-basel request body.
type CreateRequest struct {
	TipeEksposur         string  `json:"tipeEksposur"         binding:"required"`
	LGD                  string  `json:"lgd"                  binding:"required"` // decimal as string to avoid float64
	Karakteristik        *string `json:"karakteristik"`
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"   binding:"required"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	Sumber               *string `json:"sumber"`
	DokumenPendukungID   *string `json:"dokumenPendukungId"`
}

// UpdateRequest is the PATCH /master/lgd-basel/:id request body.
// ID is path param. Row_version optimistic lock required.
type UpdateRequest struct {
	TipeEksposur         *string `json:"tipeEksposur"`
	LGD                  *string `json:"lgd"` // decimal as string
	Karakteristik        *string `json:"karakteristik"`
	PeriodeBerlakuDari   *string `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	Sumber               *string `json:"sumber"`
	RowVersion           int64   `json:"rowVersion"   binding:"required"`
}

// Response is the JSON representation returned by all endpoints.
type Response struct {
	ID                   string  `json:"id"`
	TipeEksposur         string  `json:"tipeEksposur"`
	LGD                  string  `json:"lgd"` // decimal serialized as string
	Karakteristik        *string `json:"karakteristik"`
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	Sumber               string  `json:"sumber"`
	DokumenPendukungID   *string `json:"dokumenPendukungId"`
	WorkflowStatus       string  `json:"workflowStatus"`
	WorkflowInstanceID   *string `json:"workflowInstanceId"`
	RowVersion           int64   `json:"rowVersion"`
	CreatedAt            string  `json:"createdAt"`
	CreatedBy            *string `json:"createdBy"`
	UpdatedAt            *string `json:"updatedAt"`
	UpdatedBy            *string `json:"updatedBy"`
	DeletedAt            *string `json:"deletedAt"`
	DeletedBy            *string `json:"deletedBy"`
}

// ToResponse converts a domain entity to the JSON response shape.
// LGD is serialized as a decimal string (4 decimal places) to avoid float64 precision issues.
func ToResponse(e *LGDBasel) Response {
	r := Response{
		ID:                 e.ID.String(),
		TipeEksposur:       string(e.TipeEksposur),
		LGD:                e.LGD.StringFixed(4),
		Karakteristik:      e.Karakteristik,
		PeriodeBerlakuDari: e.PeriodeBerlakuDari,
		Sumber:             e.Sumber,
		WorkflowStatus:     string(displayWorkflowStatus(e.WorkflowStatus)),
		RowVersion:         e.RowVersion,
		CreatedAt:          e.CreatedAt.Format(time.RFC3339),
	}

	if e.PeriodeBerlakuSampai != nil {
		s := *e.PeriodeBerlakuSampai
		r.PeriodeBerlakuSampai = &s
	}
	if e.DokumenPendukungID != nil {
		s := e.DokumenPendukungID.String()
		r.DokumenPendukungID = &s
	}
	if e.WorkflowInstanceID != nil {
		s := e.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if e.CreatedBy != nil {
		s := e.CreatedBy.String()
		r.CreatedBy = &s
	}
	if e.UpdatedAt != nil {
		s := e.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if e.UpdatedBy != nil {
		s := e.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if e.DeletedAt != nil {
		s := e.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	if e.DeletedBy != nil {
		s := e.DeletedBy.String()
		r.DeletedBy = &s
	}
	return r
}

// displayWorkflowStatus maps the engine-canonical REJECTED state to the
// user-facing RETURNED label for API responses.
//
// Canonical states (workflow/domain.go StateRejected): the workflow engine
// writes REJECTED to mst.lgd_basel.workflow_status when a reviewer or
// approver rejects with comment. There is no engine-level "RETURNED" state;
// it is purely a frontend/API label that distinguishes
// "rejected for revision" (maker can re-edit & resubmit) from a hard
// terminal reject. The CHECK constraint in 0010 allows both values for
// forward-compatibility with engine evolution; today only REJECTED is
// written. editableStatuses lists RETURNED so that, if a future engine
// revision starts persisting RETURNED directly, the editability invariant
// still holds without code change.
//
// Ref: docs/audit/COMPLIANCE-lgd-basel-*.md BLOCKER 3 — clarification per
// ifrs9-compliance-reviewer 2026-06-03.
func displayWorkflowStatus(s WorkflowStatus) WorkflowStatus {
	if s == WorkflowStatusRejected {
		return WorkflowStatusReturned
	}
	return s
}

// DeleteResponse is returned by the soft-delete endpoint.
type DeleteResponse struct {
	Deleted   bool   `json:"deleted"`
	DeletedAt string `json:"deletedAt"`
	EntityID  string `json:"entityId"`
}

// AuditHistoryItem represents one aud.audit_log row for lgd_basel.
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

// ExportFormat is the output format for export endpoints.
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatXLSX ExportFormat = "xlsx"
)

// WorkflowActionRequest is the request body for submit/review/approve/approve2/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds the mandatory comment for reject.
type WorkflowRejectRequest struct {
	Comment         string `json:"comment"        binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod"`
	RowVersion      *int64 `json:"rowVersion"`
}
