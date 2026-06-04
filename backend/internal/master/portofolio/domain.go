// Package portofolio implements the mst.portofolio master-data module (APP-A).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Follows the exact pattern established by internal/master/matauang.
//
// Schema drift note: the underlying table uses version+is_deleted from migration 0001.
// Migration 0018 adds deleted_at, deleted_by, tenant_id, workflow_status.
// This package uses deleted_at (not is_deleted) for soft-delete; version is aliased
// row_version for optimistic locking.
package portofolio

import (
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ─────────────────────────────────────────────────────────────

const (
	// CodePortofolioDuplicateKode is returned when kode_portofolio already exists.
	// HTTP 409.
	CodePortofolioDuplicateKode = string(domainerrors.CodePortofolioDuplicateKode)

	// CodePortofolioInvalidKodeFormat is returned when kode_portofolio fails regex.
	// HTTP 400.
	CodePortofolioInvalidKodeFormat = string(domainerrors.CodePortofolioInvalidKodeFormat)

	// CodePortofolioInvalidBMCategory is returned when bm_category_default is invalid.
	// HTTP 400.
	CodePortofolioInvalidBMCategory = string(domainerrors.CodePortofolioInvalidBMCategory)

	// CodeMasterApprovedNoEdit is re-exported from domainerrors for handler use.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)

	// CodeEntityInUse is re-exported from domainerrors.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)
)

// ─── WorkflowStatus ──────────────────────────────────────────────────────────

// WorkflowStatus mirrors enum values in mst.portofolio.workflow_status.
type WorkflowStatus string

const (
	WorkflowStatusDraft           WorkflowStatus = "DRAFT"
	WorkflowStatusPendingReview   WorkflowStatus = "PENDING_REVIEW"
	WorkflowStatusPendingApproval WorkflowStatus = "PENDING_APPROVAL"
	WorkflowStatusApproved        WorkflowStatus = "APPROVED"
	WorkflowStatusRejected        WorkflowStatus = "REJECTED"
	WorkflowStatusReturned        WorkflowStatus = "RETURNED"
)

// editableStatuses is the set of workflow_status values that allow data edits.
var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusReturned: true,
	WorkflowStatusRejected: true,
}

// IsEditable returns true if the record can be edited.
func (s WorkflowStatus) IsEditable() bool { return editableStatuses[s] }

// ─── BMCategory ──────────────────────────────────────────────────────────────

// BMCategory represents the bm_category_default values.
type BMCategory string

const (
	BMCategoryHTC   BMCategory = "HTC"
	BMCategoryHTCS  BMCategory = "HTCS"
	BMCategoryOther BMCategory = "OTHER"
)

var validBMCategories = map[BMCategory]bool{
	BMCategoryHTC:   true,
	BMCategoryHTCS:  true,
	BMCategoryOther: true,
}

// IsValid returns true if the BMCategory value is in the allowed set.
func (b BMCategory) IsValid() bool { return validBMCategories[b] }

// ─── Domain entity ────────────────────────────────────────────────────────────

// Portofolio is the domain entity for mst.portofolio.
// DB columns: version is used as row_version for optimistic locking.
// is_deleted is preserved in DB but NOT used by application code (uses deleted_at instead).
type Portofolio struct {
	// Surrogate UUID — used as workflow entity_id.
	ID uuid.UUID `db:"id"`

	// Business key — immutable after create.
	KodePortofolio string `db:"kode_portofolio"`

	// Core fields.
	Nama                    string     `db:"nama"`
	TujuanPengelolaan       *string    `db:"tujuan_pengelolaan"`
	BMCategoryDefault       BMCategory `db:"bm_category_default"`
	Benchmark               *string    `db:"benchmark"`
	KompensasiManagerBasis  *string    `db:"kompensasi_manager_basis"`
	PeriodeReviewTerakhir   *string    `db:"periode_review_terakhir"` // DATE → "YYYY-MM-DD"
	AktifFlag               bool       `db:"aktif_flag"`

	// Workflow.
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit.

	// Audit fields (canonical set from db-conventions.md + migration 0018).
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  *uuid.UUID `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"` // maps to DB column "version"
	TenantID   string     `db:"tenant_id"`
}

// ─── Request / Response types ────────────────────────────────────────────────

// CreateRequest is the POST /master/portofolio request body.
type CreateRequest struct {
	KodePortofolio          string  `json:"kodePortofolio"          binding:"required"`
	Nama                    string  `json:"nama"                    binding:"required,min=3,max=200"`
	TujuanPengelolaan       *string `json:"tujuanPengelolaan"`
	BMCategoryDefault       string  `json:"bmCategoryDefault"       binding:"required,oneof=HTC HTCS OTHER"`
	Benchmark               *string `json:"benchmark"               binding:"omitempty,max=100"`
	KompensasiManagerBasis  *string `json:"kompensasiManagerBasis"  binding:"omitempty,max=50"`
	PeriodeReviewTerakhir   *string `json:"periodeReviewTerakhir"`
	AktifFlag               *bool   `json:"aktifFlag"`
}

// UpdateRequest is the PUT /master/portofolio/:kode request body.
// kode_portofolio is immutable — not in body.
type UpdateRequest struct {
	Nama                   *string `json:"nama"                   binding:"omitempty,min=3,max=200"`
	TujuanPengelolaan      *string `json:"tujuanPengelolaan"`
	BMCategoryDefault      *string `json:"bmCategoryDefault"      binding:"omitempty,oneof=HTC HTCS OTHER"`
	Benchmark              *string `json:"benchmark"              binding:"omitempty,max=100"`
	KompensasiManagerBasis *string `json:"kompensasiManagerBasis" binding:"omitempty,max=50"`
	PeriodeReviewTerakhir  *string `json:"periodeReviewTerakhir"`
	AktifFlag              *bool   `json:"aktifFlag"`
	RowVersion             int64   `json:"rowVersion"             binding:"required"`
}

// Response is the JSON representation returned by all CRUD and workflow endpoints.
type Response struct {
	ID                     string  `json:"id"`
	KodePortofolio         string  `json:"kodePortofolio"`
	Nama                   string  `json:"nama"`
	TujuanPengelolaan      *string `json:"tujuanPengelolaan"`
	BMCategoryDefault      string  `json:"bmCategoryDefault"`
	Benchmark              *string `json:"benchmark"`
	KompensasiManagerBasis *string `json:"kompensasiManagerBasis"`
	PeriodeReviewTerakhir  *string `json:"periodeReviewTerakhir"`
	AktifFlag              bool    `json:"aktifFlag"`
	WorkflowStatus         string  `json:"workflowStatus"`
	WorkflowInstanceID     *string `json:"workflowInstanceId"`
	RowVersion             int64   `json:"rowVersion"`
	CreatedAt              string  `json:"createdAt"`
	CreatedBy              *string `json:"createdBy"`
	UpdatedAt              *string `json:"updatedAt"`
	UpdatedBy              *string `json:"updatedBy"`
	DeletedAt              *string `json:"deletedAt"`
	DeletedBy              *string `json:"deletedBy"`
}

// ToResponse converts a domain entity to the JSON response shape.
func ToResponse(p *Portofolio) Response {
	r := Response{
		ID:                p.ID.String(),
		KodePortofolio:    p.KodePortofolio,
		Nama:              p.Nama,
		BMCategoryDefault: string(p.BMCategoryDefault),
		AktifFlag:         p.AktifFlag,
		WorkflowStatus:    string(displayWorkflowStatus(p.WorkflowStatus)),
		RowVersion:        p.RowVersion,
		CreatedAt:         p.CreatedAt.Format(time.RFC3339),
	}

	// Nullable fields.
	if p.TujuanPengelolaan != nil {
		r.TujuanPengelolaan = p.TujuanPengelolaan
	}
	if p.Benchmark != nil {
		r.Benchmark = p.Benchmark
	}
	if p.KompensasiManagerBasis != nil {
		r.KompensasiManagerBasis = p.KompensasiManagerBasis
	}
	if p.PeriodeReviewTerakhir != nil {
		r.PeriodeReviewTerakhir = p.PeriodeReviewTerakhir
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
	if p.DeletedBy != nil {
		s := p.DeletedBy.String()
		r.DeletedBy = &s
	}
	return r
}

// displayWorkflowStatus maps internal DB value to API-visible status.
// REJECTED → RETURNED (master entity can re-submit).
func displayWorkflowStatus(s WorkflowStatus) WorkflowStatus {
	if s == WorkflowStatusRejected {
		return WorkflowStatusReturned
	}
	return s
}

// DeleteResponse is returned by the soft-delete endpoint.
type DeleteResponse struct {
	Deleted        bool   `json:"deleted"`
	DeletedAt      string `json:"deletedAt"`
	KodePortofolio string `json:"kodePortofolio"`
}

// AuditHistoryItem represents one aud.audit_log row for portofolio.
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

// AllowedSortCols is the whitelist of sort-able columns.
var AllowedSortCols = []string{
	"kode_portofolio",
	"nama",
	"bm_category_default",
	"aktif_flag",
	"created_at",
	"workflow_status",
}

// AllowedFilterCols is the whitelist of filter-able columns.
var AllowedFilterCols = []string{
	"aktif_flag",
	"bm_category_default",
	"kode_portofolio",
	"workflow_status",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"kode_portofolio", "nama"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// WorkflowActionRequest is the request body for submit/review/approve/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}
