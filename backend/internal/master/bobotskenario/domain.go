// Package bobotskenario implements the mst.bobot_skenario master-data module (APP-C ECL Parameter).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// This is an ECL parameter module (DEC-010) and therefore has a BLOCKING ifrs9-compliance-reviewer
// gate on every PR that touches this package.
//
// Workflow: 6-eyes (DRAFT→PENDING_REVIEW→PENDING_APPROVAL→PENDING_APPROVAL_2→APPROVED).
// Both approve and approve2 require step-up MFA (DEC-027).
// SoD: approver2 ≠ maker ∧ ≠ reviewer ∧ ≠ approver1 (approver2NotAnyPrevious=true).
//
// CRITICAL: Cross-row sum=1.0 invariant (DEC-010).
// For every (periode_berlaku_dari, periode_berlaku_sampai) tuple, the sum of bobot for
// the three skenarios (GOOD, NORMAL, BAD) MUST equal 1.0 within tolerance 0.00000001.
// This is enforced in service layer within the same transaction. No DB trigger.
//
// Bobot field uses shopspring/decimal.Decimal (DEC-016, never float64).
//
// Legacy columns maker_id / approver_id in mst.bobot_skenario remain in DB for FK integrity
// (migration 0001). Service sets maker_id=currentUser.ID on create to satisfy the NOT NULL
// DB constraint but treats sys.workflow_instance as the sole source of truth for
// the approval chain.
package bobotskenario

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

	// CodeBobotSumInvariantViolated is returned when G+N+B bobot ≠ 1.0 (DEC-010). HTTP 422.
	CodeBobotSumInvariantViolated = string(domainerrors.CodeBobotSumInvariantViolated)

	// CodeBobotPeriodOverlap is returned when skenario has an overlapping period. HTTP 422.
	CodeBobotPeriodOverlap = string(domainerrors.CodeBobotPeriodOverlap)

	// CodeBobotDuplicateSkenarioPeriod is returned when same (skenario, period) tuple exists. HTTP 422.
	CodeBobotDuplicateSkenarioPeriod = string(domainerrors.CodeBobotDuplicateSkenarioPeriod)
)

// ─── Skenario enum ─────────────────────────────────────────────────────────────

// Skenario enumerates the whitelist values for mst.bobot_skenario.skenario.
// Enforced by service layer; DB CHECK constraint ck_skenario in migration 0001.
type Skenario string

const (
	SkenarioGood   Skenario = "GOOD"
	SkenarioNormal Skenario = "NORMAL"
	SkenarioBad    Skenario = "BAD"
)

// validSkenario is the service-layer whitelist.
var validSkenario = map[Skenario]bool{
	SkenarioGood:   true,
	SkenarioNormal: true,
	SkenarioBad:    true,
}

// AllSkenarios is the ordered set of all skenario values.
var AllSkenarios = []Skenario{SkenarioGood, SkenarioNormal, SkenarioBad}

// IsValidSkenario returns true if s is in the whitelist.
func IsValidSkenario(s Skenario) bool {
	return validSkenario[s]
}

// ─── Sum invariant tolerance ────────────────────────────────────────────────────

// SumTolerance is the tolerance for the G+N+B = 1.0 invariant (DEC-010).
// Two decimals are considered equal if |a - b| <= SumTolerance.
var SumTolerance = decimal.NewFromFloat(0.00000001)

// SumTarget is the expected sum of all three bobot values.
var SumTarget = decimal.NewFromInt(1)

// ─── WorkflowStatus ───────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values allowed in mst.bobot_skenario.workflow_status.
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
// bobot_skenario shares ecl_parameter permissions (per WORKFLOW_CONFIG_BOBOT_SKENARIO seed).
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

// ─── Default bobot values (DEC-010) ───────────────────────────────────────────

// DefaultBobot returns the DEC-010 default bobot for a skenario.
// GOOD=0.25, NORMAL=0.50, BAD=0.25.
func DefaultBobot(s Skenario) decimal.Decimal {
	switch s {
	case SkenarioGood:
		return decimal.NewFromFloat(0.25)
	case SkenarioNormal:
		return decimal.NewFromFloat(0.50)
	case SkenarioBad:
		return decimal.NewFromFloat(0.25)
	default:
		return decimal.Zero
	}
}

// ─── Domain entity ─────────────────────────────────────────────────────────────

// BobotSkenario is the domain entity for mst.bobot_skenario.
// DEC-016: bobot uses shopspring/decimal.Decimal — never float64.
type BobotSkenario struct {
	// Surrogate UUID primary key.
	ID uuid.UUID `db:"id"`

	// Business fields.
	Skenario             Skenario        `db:"skenario"`
	Bobot                decimal.Decimal `db:"bobot"`   // NUMERIC(10,8), [0,1]
	PeriodeBerlakuDari   string          `db:"periode_berlaku_dari"`   // DATE → "YYYY-MM-DD"
	PeriodeBerlakuSampai *string         `db:"periode_berlaku_sampai"` // nullable = currently active
	Catatan              *string         `db:"catatan"`                // free text, nullable

	// Legacy columns — NOT used as source of truth for workflow, kept for DB NOT NULL compat.
	// Service sets maker_id=currentUser.ID on create only.
	MakerID    uuid.UUID  `db:"maker_id"`
	ApproverID *uuid.UUID `db:"approver_id"` // nullable
	ApprovedAt *time.Time `db:"approved_at"` // nullable legacy column

	// Workflow.
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit

	// Audit columns (added by migration 0011).
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
	"skenario",
	"bobot",
	"periode_berlaku_dari",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist for ?filter[col]= query param.
var AllowedFilterCols = []string{
	"skenario",
	"workflow_status",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"skenario", "catatan"}

// AllAllowedCols is the union for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST /master/bobot-skenario request body.
type CreateRequest struct {
	Skenario             string  `json:"skenario"             binding:"required"`
	Bobot                string  `json:"bobot"                binding:"required"` // decimal as string to avoid float64
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"   binding:"required"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	Catatan              *string `json:"catatan"`
}

// UpdateRequest is the PATCH /master/bobot-skenario/:id request body.
// ID is path param. Row_version optimistic lock required.
type UpdateRequest struct {
	Skenario             *string `json:"skenario"`
	Bobot                *string `json:"bobot"` // decimal as string
	PeriodeBerlakuDari   *string `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	Catatan              *string `json:"catatan"`
	RowVersion           int64   `json:"rowVersion" binding:"required"`
}

// SeedDefaultRequest is the POST /master/bobot-skenario/seed-default request body.
type SeedDefaultRequest struct {
	PeriodeBerlakuDari string `json:"periodeBerlakuDari" binding:"required"`
}

// Response is the JSON representation returned by all endpoints.
type Response struct {
	ID                   string  `json:"id"`
	Skenario             string  `json:"skenario"`
	Bobot                string  `json:"bobot"` // decimal serialized as string (8dp)
	PeriodeBerlakuDari   string  `json:"periodeBerlakuDari"`
	PeriodeBerlakuSampai *string `json:"periodeBerlakuSampai"`
	Catatan              *string `json:"catatan"`
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
// Bobot is serialized as a decimal string (8 decimal places) to avoid float64 precision issues.
func ToResponse(e *BobotSkenario) Response {
	r := Response{
		ID:                 e.ID.String(),
		Skenario:           string(e.Skenario),
		Bobot:              e.Bobot.StringFixed(8),
		PeriodeBerlakuDari: e.PeriodeBerlakuDari,
		Catatan:            e.Catatan,
		WorkflowStatus:     string(displayWorkflowStatus(e.WorkflowStatus)),
		RowVersion:         e.RowVersion,
		CreatedAt:          e.CreatedAt.Format(time.RFC3339),
	}

	if e.PeriodeBerlakuSampai != nil {
		s := *e.PeriodeBerlakuSampai
		r.PeriodeBerlakuSampai = &s
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

// AuditHistoryItem represents one aud.audit_log row for bobot_skenario.
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

// WorkflowActionRequest is the request body for submit/review/approve/approve2/reject.
type WorkflowActionRequest struct {
	Comment        *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion     *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds the mandatory comment for reject.
type WorkflowRejectRequest struct {
	Comment        string `json:"comment"        binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod"`
	RowVersion     *int64 `json:"rowVersion"`
}

// SeedDefaultResult is returned by the seed-default endpoint.
type SeedDefaultResult struct {
	Created int      `json:"created"` // number of rows created (0 if idempotent skip)
	IDs     []string `json:"ids"`
	Skipped bool     `json:"skipped"` // true if 3 rows already exist for the period
}
