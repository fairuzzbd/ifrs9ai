// Package impactpd implements the mst.impact_pd master-data module.
//
// impact_pd stores a single forward-looking (FL) PD multiplier per periode.
// Exactly 1 active row per periode_id (UNIQUE constraint in DB).
// impact_multiplier range [0.5, 2.0] — enforced by DB CHECK and service validation.
//
// Workflow: 6-eyes (DEC-017), both approve steps require step-up MFA (DEC-027).
// Permissions reuse ecl_parameter.* (same ALCO-controlled parameter family).
//
// Architecture follows matauang pilot pattern.
package impactpd

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// Error code aliases.
const (
	// CodePDOutOfRange is returned when impact_multiplier is outside [0.5, 2.0].
	CodePDOutOfRange = string(domainerrors.CodeImpactPDOutOfRange)

	// CodePDPeriodeExists is returned when a row for the given periode already exists.
	CodePDPeriodeExists = string(domainerrors.CodeImpactPDPeriodeExists)
)

// DB-enforced range for impact_multiplier.
var (
	MultiplierMin = decimal.NewFromFloat(0.5)
	MultiplierMax = decimal.NewFromFloat(2.0)
)

// WorkflowStatus mirrors mst.impact_pd.workflow_status.
type WorkflowStatus string

const (
	WorkflowStatusDraft            WorkflowStatus = "DRAFT"
	WorkflowStatusPendingReview    WorkflowStatus = "PENDING_REVIEW"
	WorkflowStatusPendingApproval  WorkflowStatus = "PENDING_APPROVAL"
	WorkflowStatusPendingApproval2 WorkflowStatus = "PENDING_APPROVAL_2"
	WorkflowStatusApproved         WorkflowStatus = "APPROVED"
	WorkflowStatusRejected         WorkflowStatus = "REJECTED"
)

var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusRejected: true,
}

// IsEditable returns true when the record can be edited.
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// ImpactPD is the domain entity for mst.impact_pd.
type ImpactPD struct {
	ID               uuid.UUID       `db:"id"`
	PeriodeID        uuid.UUID       `db:"periode_id"`
	ImpactMultiplier decimal.Decimal `db:"impact_multiplier"` // NUMERIC(10,8), [0.5, 2.0]
	Catatan          *string         `db:"catatan"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"`

	// Audit fields (from migration 0014)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  *uuid.UUID `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// AllowedSortCols is the whitelist of sortable columns.
var AllowedSortCols = []string{
	"id",
	"periode_id",
	"impact_multiplier",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist of filterable columns.
var AllowedFilterCols = []string{
	"periode_id",
	"workflow_status",
}

// SearchCols are scanned by ?q= text search.
var SearchCols = []string{"catatan"}

// AllAllowedCols is the union passed to listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST body.
type CreateRequest struct {
	PeriodeID        string  `json:"periodeId"        binding:"required"`
	ImpactMultiplier string  `json:"impactMultiplier" binding:"required"`
	Catatan          *string `json:"catatan"`
}

// UpdateRequest is the PUT body.
type UpdateRequest struct {
	ImpactMultiplier *string `json:"impactMultiplier"`
	Catatan          *string `json:"catatan"`
	RowVersion       int64   `json:"rowVersion" binding:"required"`
}

// Response is returned by all CRUD endpoints.
type Response struct {
	ID                 string  `json:"id"`
	PeriodeID          string  `json:"periodeId"`
	ImpactMultiplier   string  `json:"impactMultiplier"` // decimal string — DEC-016
	Catatan            *string `json:"catatan,omitempty"`
	WorkflowStatus     string  `json:"workflowStatus"`
	WorkflowInstanceID *string `json:"workflowInstanceId,omitempty"`
	RowVersion         int64   `json:"rowVersion"`
	CreatedAt          string  `json:"createdAt"`
	CreatedBy          *string `json:"createdBy,omitempty"`
	UpdatedAt          *string `json:"updatedAt,omitempty"`
	UpdatedBy          *string `json:"updatedBy,omitempty"`
	DeletedAt          *string `json:"deletedAt,omitempty"`
	DeletedBy          *string `json:"deletedBy,omitempty"`
}

// ToResponse converts a domain entity to the JSON response.
func ToResponse(m *ImpactPD) Response {
	r := Response{
		ID:               m.ID.String(),
		PeriodeID:        m.PeriodeID.String(),
		ImpactMultiplier: m.ImpactMultiplier.String(),
		Catatan:          m.Catatan,
		WorkflowStatus:   string(m.WorkflowStatus),
		RowVersion:       m.RowVersion,
		CreatedAt:        m.CreatedAt.Format(time.RFC3339),
	}
	if m.WorkflowInstanceID != nil {
		s := m.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if m.CreatedBy != nil {
		s := m.CreatedBy.String()
		r.CreatedBy = &s
	}
	if m.UpdatedAt != nil {
		s := m.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if m.UpdatedBy != nil {
		s := m.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if m.DeletedAt != nil {
		s := m.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	if m.DeletedBy != nil {
		s := m.DeletedBy.String()
		r.DeletedBy = &s
	}
	return r
}

// DeleteResponse is returned by the soft-delete endpoint.
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
	BeforeJSONB interface{} `json:"beforeJsonb,omitempty"`
	AfterJSONB  interface{} `json:"afterJsonb,omitempty"`
	IP          *string     `json:"ip,omitempty"`
	TraceID     *string     `json:"traceId,omitempty"`
}

// WorkflowActionRequest is the body for submit/review/approve/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod  string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds mandatory comment.
type WorkflowRejectRequest struct {
	Comment         string  `json:"comment"        binding:"required,min=10"`
	SignatureMethod  string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}
