// Package impactmevpd implements the mst.impact_mev_pd master-data module (APP-A).
//
// Impact MEV PD records the Macro-Economic Variable (MEV) to PD adjustment multiplier
// per scenario (GOOD or BAD) per periode_buku. NORMAL is implicitly 1.0 (no FL adjustment)
// and has no row in this table (OQ-1 resolved 2026-06-09).
//
// This table is an INDEPENDENT input from mst.impact_pd (OQ-2 resolved 2026-06-09):
// it serves as an audit trail of RISK Officer's MEV analysis per scenario. ALCO sets
// the final FL multiplier via mst.impact_pd independently.
//
// Workflow: 6-eyes (DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED)
// Approver1 = ROLE-RISK (validation), Approver2 = ROLE-ALCO (policy).
// Both APPROVE steps require step-up MFA (DEC-027).
//
// Unique invariant: at most one APPROVED row per (periode_id, skenario). Enforced by
// ValidatePeriodeUnique + partial index idx_impact_mev_pd_active.
//
// Domain rules:
//  1. skenario MUST be 'GOOD' or 'BAD' (OQ-1: NORMAL has no MEV impact row).
//  2. impact_multiplier > 0 (no upper bound — OQ-3 resolved 2026-06-09).
//  3. Soft-delete only for DRAFT/REJECTED/RETURNED rows.
//  4. APPROVED rows are immutable (CodeMasterApprovedNoEdit).
package impactmevpd

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeFLPeriodDuplicate is returned when (periode_id, skenario) already has an
	// APPROVED or in-flight row. HTTP 422.
	CodeFLPeriodDuplicate = string(domainerrors.CodeFLPeriodDuplicate)

	// CodeMasterApprovedNoEdit is returned when editing/deleting an APPROVED row. HTTP 403.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)
)

// ─── Sentinel errors (repo-internal) ─────────────────────────────────────────

// ErrNotFound and ErrConflict are repo-internal sentinels mapped to domain errors in service.
var (
	ErrNotFound = domainerrors.ErrNotFound("impact_mev_pd")
	ErrConflict = domainerrors.ErrConflict()
)

// ─── Skenario enum ────────────────────────────────────────────────────────────

// Skenario mirrors the CHECK constraint ck_impact_skenario in mst.impact_mev_pd.
// NORMAL is not stored here — its multiplier is implicitly 1.0 (OQ-1).
type Skenario string

const (
	SkenarioGood Skenario = "GOOD"
	SkenaroBad   Skenario = "BAD"
)

// validSkenario is the whitelist validated by service layer.
var validSkenario = map[Skenario]bool{
	SkenarioGood: true,
	SkenaroBad:   true,
}

// IsValid returns true if s is a valid skenario enum value.
func (s Skenario) IsValid() bool { return validSkenario[s] }

// ─── Workflow status ──────────────────────────────────────────────────────────

// WorkflowStatus mirrors mst.impact_mev_pd.workflow_status.
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

// editableStatuses is the set that allows data edits.
var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusReturned: true,
	WorkflowStatusRejected: true,
}

// IsEditable returns true if the record may be edited.
func (s WorkflowStatus) IsEditable() bool { return editableStatuses[s] }

// ─── Domain entity ────────────────────────────────────────────────────────────

// ImpactMevPd is the domain entity for mst.impact_mev_pd.
// impact_multiplier uses shopspring/decimal (DEC-016 — no float64 for rates).
type ImpactMevPd struct {
	ID uuid.UUID `db:"id"`

	// Core business fields
	PeriodeID          uuid.UUID       `db:"periode_id"`
	Skenario           Skenario        `db:"skenario"`
	ImpactMultiplier   decimal.Decimal `db:"impact_multiplier"`
	MevComponentsJSON  *string         `db:"mev_components_json"` // JSONB stored as text
	Catatan            *string         `db:"catatan"`
	DokumenPendukungID *uuid.UUID      `db:"dokumen_pendukung_id"`

	// Legacy fields (kept for backcompat, not actively used in service logic)
	MakerID    uuid.UUID  `db:"maker_id"`
	ApproverID *uuid.UUID `db:"approver_id"`
	ApprovedAt *time.Time `db:"approved_at"`

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

// CreateRequest is the POST /master/impact-mev-pd request body.
type CreateRequest struct {
	PeriodeID string `json:"periodeId"         binding:"required"`
	Skenario  string `json:"skenario"          binding:"required"`
	// ImpactMultiplier as string to preserve precision (shopspring/decimal parse).
	ImpactMultiplier   string  `json:"impactMultiplier"  binding:"required"`
	MevComponentsJSON  *string `json:"mevComponentsJson"`  // optional JSONB payload
	Catatan            *string `json:"catatan"`            // optional free text
	DokumenPendukungID *string `json:"dokumenPendukungId"` // optional UUID
}

// UpdateRequest is the PUT /master/impact-mev-pd/:id request body.
type UpdateRequest struct {
	ImpactMultiplier   *string `json:"impactMultiplier"`
	MevComponentsJSON  *string `json:"mevComponentsJson"`
	Catatan            *string `json:"catatan"`
	DokumenPendukungID *string `json:"dokumenPendukungId"`
	RowVersion         int64   `json:"rowVersion" binding:"required"`
}

// Response is the JSON representation returned by CRUD and workflow endpoints.
type Response struct {
	ID                 string  `json:"id"`
	PeriodeID          string  `json:"periodeId"`
	Skenario           string  `json:"skenario"`
	ImpactMultiplier   string  `json:"impactMultiplier"`
	MevComponentsJSON  *string `json:"mevComponentsJson"`
	Catatan            *string `json:"catatan"`
	DokumenPendukungID *string `json:"dokumenPendukungId"`
	WorkflowStatus     string  `json:"workflowStatus"`
	WorkflowInstanceID *string `json:"workflowInstanceId"`
	RowVersion         int64   `json:"rowVersion"`
	CreatedAt          string  `json:"createdAt"`
	CreatedBy          *string `json:"createdBy"`
	UpdatedAt          *string `json:"updatedAt"`
	UpdatedBy          *string `json:"updatedBy"`
	DeletedAt          *string `json:"deletedAt"`
}

// ToResponse converts a domain entity to the JSON response shape.
func ToResponse(e *ImpactMevPd) Response {
	r := Response{
		ID:        e.ID.String(),
		PeriodeID: e.PeriodeID.String(),
		Skenario:  string(e.Skenario),
		// 8 decimal places per DEC-016 NUMERIC(10,8)
		ImpactMultiplier:  e.ImpactMultiplier.StringFixed(8),
		MevComponentsJSON: e.MevComponentsJSON,
		Catatan:           e.Catatan,
		WorkflowStatus:    string(displayWorkflowStatus(e.WorkflowStatus)),
		RowVersion:        e.RowVersion,
		CreatedAt:         e.CreatedAt.Format(time.RFC3339),
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
	return r
}

// displayWorkflowStatus maps DB REJECTED → RETURNED for API consumers.
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

// AuditHistoryItem represents one aud.audit_log row for impact_mev_pd.
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

// ActiveResponse is returned by GET /master/impact-mev-pd/active.
// Shape is designed for ECL engine Phase 4 consumption (OQ-5).
// NORMAL is not included — ECL engine uses hardcoded 1.0 for NORMAL (OQ-1).
type ActiveResponse struct {
	PeriodeID string `json:"periodeId"`
	// Multipliers maps skenario ("GOOD"/"BAD") to the APPROVED impact_multiplier.
	Multipliers map[string]string `json:"multipliers"`
	// Rows contains the full APPROVED rows (for audit/display purposes).
	Rows []Response `json:"rows"`
}

// WorkflowActionRequest is the body for submit/review/approve/approve2/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds mandatory comment for reject.
type WorkflowRejectRequest struct {
	Comment         string `json:"comment"         binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod"`
	RowVersion      *int64 `json:"rowVersion"`
}

// ─── Allowed columns (whitelist for listquery) ────────────────────────────────

// AllowedSortCols is the whitelist of sortable columns.
var AllowedSortCols = []string{
	"id",
	"periode_id",
	"skenario",
	"impact_multiplier",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist of filterable columns.
var AllowedFilterCols = []string{
	"workflow_status",
	"periode_id",
	"skenario",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"catatan"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)
