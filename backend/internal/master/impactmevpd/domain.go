// Package impactmevpd implements the mst.impact_mev_pd master-data module.
//
// impact_mev_pd stores the dual forward-looking (FL) multiplier for MEV PD
// adjustments per (periode_id, skenario) pair. skenario ∈ {GOOD, BAD};
// NORMAL is implicit (multiplier = 1.0 per DEC-010).
//
// Workflow: 6-eyes (DEC-017), both approve steps require step-up MFA (DEC-027).
// Permissions reuse ecl_parameter.* (same ALCO-controlled parameter family).
//
// Architecture follows the matauang pilot pattern:
//
//	domain.go   — entity, request/response, allowed cols
//	repo.go     — SQL (database/sql, no ORM); Repository interface
//	service.go  — tx boundary, validation, audit
//	handler.go  — thin HTTP; calls service
//	routes.go   — RegisterRoutes
//	workflow_hook.go — EntityHook post-commit status sync
package impactmevpd

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// Error code aliases for this package.
const (
	// CodeDuplicatePeriodeSkenario is returned when (periode_id, skenario) already exists.
	CodeDuplicatePeriodeSkenario = string(domainerrors.CodeImpactDuplicatePeriodeSkenario)
)

// Skenario is the allowed set of forward-looking scenarios.
// NORMAL is not stored — its multiplier is always 1.0.
type Skenario string

const (
	SkenarioGood Skenario = "GOOD"
	SkenarioBad  Skenario = "BAD"
)

// ValidSkenario returns true if s is a recognised scenario value.
func ValidSkenario(s string) bool {
	return s == string(SkenarioGood) || s == string(SkenarioBad)
}

// WorkflowStatus mirrors the mst.impact_mev_pd.workflow_status column.
type WorkflowStatus string

const (
	WorkflowStatusDraft             WorkflowStatus = "DRAFT"
	WorkflowStatusPendingReview     WorkflowStatus = "PENDING_REVIEW"
	WorkflowStatusPendingApproval   WorkflowStatus = "PENDING_APPROVAL"
	WorkflowStatusPendingApproval2  WorkflowStatus = "PENDING_APPROVAL_2"
	WorkflowStatusApproved          WorkflowStatus = "APPROVED"
	WorkflowStatusRejected          WorkflowStatus = "REJECTED"
)

// editableStatuses defines which states allow field edits.
var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusRejected: true,
}

// IsEditable returns true when the record can be edited.
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// ImpactMevPD is the domain entity for mst.impact_mev_pd.
type ImpactMevPD struct {
	ID               uuid.UUID       `db:"id"`
	PeriodeID        uuid.UUID       `db:"periode_id"`
	Skenario         Skenario        `db:"skenario"`
	ImpactMultiplier decimal.Decimal `db:"impact_multiplier"` // NUMERIC(10,8)
	MevComponentsJSON *string        `db:"mev_components_json"` // JSONB, nil if absent
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
	"skenario",
	"impact_multiplier",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist of filterable columns.
var AllowedFilterCols = []string{
	"periode_id",
	"skenario",
	"workflow_status",
}

// SearchCols are scanned by ?q= text search.
var SearchCols = []string{"skenario", "catatan"}

// AllAllowedCols is the union passed to listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST body.
type CreateRequest struct {
	PeriodeID        string  `json:"periodeId"        binding:"required"`
	Skenario         string  `json:"skenario"         binding:"required"`
	ImpactMultiplier string  `json:"impactMultiplier" binding:"required"`
	MevComponentsJSON *string `json:"mevComponentsJson"`
	Catatan          *string `json:"catatan"`
}

// UpdateRequest is the PUT body. ImpactMultiplier and other fields are optional.
type UpdateRequest struct {
	ImpactMultiplier *string  `json:"impactMultiplier"`
	MevComponentsJSON *string `json:"mevComponentsJson"`
	Catatan          *string  `json:"catatan"`
	RowVersion       int64    `json:"rowVersion" binding:"required"`
}

// Response is the JSON shape returned by all CRUD endpoints.
type Response struct {
	ID                 string  `json:"id"`
	PeriodeID          string  `json:"periodeId"`
	Skenario           string  `json:"skenario"`
	ImpactMultiplier   string  `json:"impactMultiplier"` // decimal string — DEC-016
	MevComponentsJSON  *string `json:"mevComponentsJson,omitempty"`
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
func ToResponse(m *ImpactMevPD) Response {
	r := Response{
		ID:               m.ID.String(),
		PeriodeID:        m.PeriodeID.String(),
		Skenario:         string(m.Skenario),
		ImpactMultiplier: m.ImpactMultiplier.String(),
		MevComponentsJSON: m.MevComponentsJSON,
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

// ─── MEV components validation helpers ───────────────────────────────────────

// ValidateMevComponentsJSON validates the mev_components_json field.
// Rules:
//   - Must be valid JSON
//   - Must be a JSON object (not array/scalar)
//   - If "weights" key is present, values must sum to 1.0 (± 0.001 tolerance)
//
// Returns nil if valid, an error detail slice otherwise.
func ValidateMevComponentsJSON(raw string) []domainerrors.Detail {
	if raw == "" {
		return nil
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return []domainerrors.Detail{{
			Field:   "body.mevComponentsJson",
			Rule:    "json_object",
			Message: "mevComponentsJson harus berupa JSON object yang valid",
		}}
	}

	// If "weights" key present, validate sum = 1.0
	if weightsRaw, ok := obj["weights"]; ok {
		weightsMap, ok := weightsRaw.(map[string]interface{})
		if !ok {
			return []domainerrors.Detail{{
				Field:   "body.mevComponentsJson.weights",
				Rule:    "weights_object",
				Message: "mevComponentsJson.weights harus berupa JSON object",
			}}
		}

		sum := decimal.Zero
		for k, v := range weightsMap {
			var val decimal.Decimal
			switch tv := v.(type) {
			case float64:
				val = decimal.NewFromFloat(tv)
			case string:
				var parseErr error
				val, parseErr = decimal.NewFromString(tv)
				if parseErr != nil {
					return []domainerrors.Detail{{
						Field:   "body.mevComponentsJson.weights." + k,
						Rule:    "numeric",
						Message: "Nilai weight '" + k + "' harus berupa angka",
					}}
				}
			default:
				return []domainerrors.Detail{{
					Field:   "body.mevComponentsJson.weights." + k,
					Rule:    "numeric",
					Message: "Nilai weight '" + k + "' harus berupa angka",
				}}
			}
			sum = sum.Add(val)
		}

		// tolerance ± 0.001
		tolerance := decimal.NewFromFloat(0.001)
		one := decimal.NewFromInt(1)
		diff := sum.Sub(one).Abs()
		if diff.GreaterThan(tolerance) {
			return []domainerrors.Detail{{
				Field:   "body.mevComponentsJson.weights",
				Rule:    "weights_sum",
				Message: "Jumlah weights harus = 1.0 (saat ini: " + sum.String() + ")",
			}}
		}
	}

	return nil
}
