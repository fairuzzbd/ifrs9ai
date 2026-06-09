// Package impactpd implements the mst.impact_pd master-data module (APP-A).
//
// Impact PD is the final Forward-Looking (FL) PD multiplier per periode_buku.
// It is applied by the ECL engine as:
//
//	ECL_FL_skenario = ECL_skenario × impact_pd.impact_multiplier
//
// where ECL_skenario = EAD × PD × LGD (per formulas.md DEC-010).
//
// This table is INDEPENDENT from mst.impact_mev_pd (OQ-2 resolved 2026-06-09):
// ALCO sets the final multiplier here directly without deriving from MEV analysis.
//
// Workflow: 6-eyes (DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED)
// Approver1 = ROLE-RISK, Approver2 = ROLE-ALCO. Both steps require step-up MFA (DEC-027).
//
// Unique invariant: at most one APPROVED row per periode_id.
//
// Domain rules:
//  1. impact_multiplier BETWEEN 0.5 AND 2.0 (per 0001 ck_impact_pd_range, OQ-3).
//  2. One row per periode_id (UNIQUE constraint uq_impact_pd_periode in 0001).
//  3. Soft-delete only for DRAFT/REJECTED/RETURNED rows.
//  4. APPROVED rows are immutable (CodeMasterApprovedNoEdit).
package impactpd

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeFLPeriodDuplicate is returned when periode_id already has an active/in-flight row. HTTP 422.
	CodeFLPeriodDuplicate = string(domainerrors.CodeFLPeriodDuplicate)

	// CodeFLMultiplierRange is returned when impact_multiplier is outside [0.5, 2.0]. HTTP 422.
	CodeFLMultiplierRange = string(domainerrors.CodeFLMultiplierRange)

	// CodeMasterApprovedNoEdit is returned when editing/deleting an APPROVED row. HTTP 403.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)
)

// Multiplier bounds per ck_impact_pd_range (0001 schema) and OQ-3 resolution.
var (
	multiplierMin = decimal.NewFromFloat(0.5)
	multiplierMax = decimal.NewFromFloat(2.0)
)

// ErrNotFound and ErrConflict are repo-internal sentinels.
var (
	ErrNotFound = domainerrors.ErrNotFound("impact_pd")
	ErrConflict = domainerrors.ErrConflict()
)

// ─── Workflow status ──────────────────────────────────────────────────────────

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

var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusReturned: true,
	WorkflowStatusRejected: true,
}

func (s WorkflowStatus) IsEditable() bool { return editableStatuses[s] }

// ─── Domain entity ────────────────────────────────────────────────────────────

// ImpactPd is the domain entity for mst.impact_pd.
// impact_multiplier uses shopspring/decimal (DEC-016 — no float64).
type ImpactPd struct {
	ID uuid.UUID `db:"id"`

	// Core business fields
	PeriodeID          uuid.UUID       `db:"periode_id"`
	ImpactMultiplier   decimal.Decimal `db:"impact_multiplier"`
	Catatan            *string         `db:"catatan"`
	DokumenPendukungID *uuid.UUID      `db:"dokumen_pendukung_id"`

	// Legacy fields
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

// CreateRequest is the POST /master/impact-pd request body.
type CreateRequest struct {
	PeriodeID          string  `json:"periodeId"          binding:"required"`
	ImpactMultiplier   string  `json:"impactMultiplier"   binding:"required"`
	Catatan            *string `json:"catatan"`
	DokumenPendukungID *string `json:"dokumenPendukungId"`
}

// UpdateRequest is the PUT /master/impact-pd/:id request body.
type UpdateRequest struct {
	ImpactMultiplier   *string `json:"impactMultiplier"`
	Catatan            *string `json:"catatan"`
	DokumenPendukungID *string `json:"dokumenPendukungId"`
	RowVersion         int64   `json:"rowVersion" binding:"required"`
}

// Response is the JSON representation.
type Response struct {
	ID                 string  `json:"id"`
	PeriodeID          string  `json:"periodeId"`
	ImpactMultiplier   string  `json:"impactMultiplier"`
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
func ToResponse(e *ImpactPd) Response {
	r := Response{
		ID:               e.ID.String(),
		PeriodeID:        e.PeriodeID.String(),
		ImpactMultiplier: e.ImpactMultiplier.StringFixed(8),
		Catatan:          e.Catatan,
		WorkflowStatus:   string(displayWorkflowStatus(e.WorkflowStatus)),
		RowVersion:       e.RowVersion,
		CreatedAt:        e.CreatedAt.Format(time.RFC3339),
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

// AuditHistoryItem represents one aud.audit_log row.
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

// ActiveResponse is returned by GET /master/impact-pd/active.
// Shape designed for ECL engine Phase 4 consumption (OQ-5).
type ActiveResponse struct {
	PeriodeID        string   `json:"periodeId"`
	ImpactMultiplier string   `json:"impactMultiplier"`
	Row              Response `json:"row"`
}

// WorkflowActionRequest is the body for workflow action endpoints.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds mandatory comment.
type WorkflowRejectRequest struct {
	Comment         string `json:"comment"         binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod"`
	RowVersion      *int64 `json:"rowVersion"`
}

// ─── Allowed columns ──────────────────────────────────────────────────────────

var AllowedSortCols = []string{
	"id",
	"periode_id",
	"impact_multiplier",
	"workflow_status",
	"created_at",
}

var AllowedFilterCols = []string{
	"workflow_status",
	"periode_id",
}

var SearchCols = []string{"catatan"}

var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)
