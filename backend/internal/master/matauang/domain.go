// Package matauang implements the mst.mata_uang master-data module (APP-A-MSTR-002).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// This package is the PILOT for the generic master-data pattern. Every other mst.*
// module should follow the same directory shape:
//
//	internal/master/{modul}/
//	  domain.go   — entity struct, request/response types, error codes
//	  repo.go     — SQL (GORM + sqlx for list); no business logic
//	  service.go  — tx boundary, validation, audit write, workflow hook
//	  handler.go  — thin HTTP; calls service; maps to response envelopes
//	  routes.go   — RegisterRoutes(v1 *gin.RouterGroup, ...)
//	  *_test.go   — unit tests (gomock service/repo boundary)
//
// Reusable for other modules:
//   - domain.go pattern: embed MasterAuditFields; add module-specific fields
//   - repo.go pattern: allowedCols whitelist + listquery.ToSQL
//   - service.go pattern: optimistic lock, workflow_status guard, audit write
//   - handler.go pattern: permission middleware + error mapping
//   - export.go pattern: sync CSV/XLSX stream
//
// NOT reusable (module-specific guards):
//   - is_system_currency protection (mata_uang only)
//   - ENTITY_IN_USE referential integrity check (module-specific FK list)
package matauang

import (
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes (per master-common.yaml §"Error codes tambahan") ────────────
//
// These are aliases to the canonical constants in domainerrors.
// They are re-exported here for convenience within the matauang package and tests.

const (
	// CodeSystemCurrencyProtected is returned when caller tries to delete/modify
	// is_system_currency=true record. HTTP 403.
	CodeSystemCurrencyProtected = string(domainerrors.CodeSystemCurrencyProtected)

	// CodeEntityInUse is returned when soft-delete is blocked by active references.
	// HTTP 409.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)

	// CodeMasterApprovedNoEdit is returned when caller tries to UPDATE an APPROVED record
	// without going through the workflow. HTTP 403.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)
)

// ─── Domain entity ────────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values allowed in mst.mata_uang.workflow_status.
// RETURNED is exposed in API response when underlying DB state is REJECTED (retractable).
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
// Only DRAFT or RETURNED (DB value: REJECTED with retractable semantics) are editable.
var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusReturned: true,
	WorkflowStatusRejected: true, // same as RETURNED at DB level
}

// IsEditable returns true if the record can be edited (before PENDING_REVIEW or after RETURNED).
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// MataUang is the domain entity for mst.mata_uang.
// Fields match migration 000001 + additional columns from 000008.
type MataUang struct {
	// Business PK (ISO 4217, immutable after create)
	KodeMataUang string `db:"kode_mata_uang"`

	// Surrogate UUID — used as workflow entity_id
	ID uuid.UUID `db:"id"`

	// Core business fields
	NamaMataUang      string `db:"nama_mata_uang"`
	Simbol            string `db:"simbol"`
	DecimalPlaces     int16  `db:"decimal_places"`
	SumberKursDefault string `db:"sumber_kurs_default"`
	FrekuensiUpdate   string `db:"frekuensi_update"`
	AktifFlag         bool   `db:"aktif_flag"`
	TanggalMulaiAktif string `db:"tanggal_mulai_aktif"` // DATE → stored as "YYYY-MM-DD"

	// Protected flag — set by migration seed for IDR; not mutable via API
	IsSystemCurrency bool `db:"is_system_currency"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit

	// Audit fields (from migration 000008)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  *uuid.UUID `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// ─── Request / Response types (aligned with mata-uang.yaml) ──────────────────

// CreateRequest is the POST /master/mata-uang request body.
type CreateRequest struct {
	KodeMataUang      string `json:"kodeMataUang"      binding:"required,len=3,uppercase"`
	NamaMataUang      string `json:"namaMataUang"      binding:"required,min=3,max=60"`
	Simbol            string `json:"simbol"            binding:"required,min=1,max=5"`
	DecimalPlaces     int16  `json:"decimalPlaces"     binding:"min=0,max=4"`
	SumberKursDefault string `json:"sumberKursDefault" binding:"required,oneof=BI_JISDOR BI_KURS_TENGAH INTERNAL"`
	FrekuensiUpdate   string `json:"frekuensiUpdate"   binding:"required,oneof=HARIAN INTRA_DAY BULANAN"`
	TanggalMulaiAktif string `json:"tanggalMulaiAktif" binding:"required"`
	AktifFlag         *bool  `json:"aktifFlag"` // optional, default true
}

// UpdateRequest is the PUT /master/mata-uang/{kode} request body.
// kode_mata_uang is immutable — not in request body.
type UpdateRequest struct {
	NamaMataUang      *string `json:"namaMataUang"      binding:"omitempty,min=3,max=60"`
	Simbol            *string `json:"simbol"            binding:"omitempty,min=1,max=5"`
	DecimalPlaces     *int16  `json:"decimalPlaces"     binding:"omitempty,min=0,max=4"`
	SumberKursDefault *string `json:"sumberKursDefault" binding:"omitempty,oneof=BI_JISDOR BI_KURS_TENGAH INTERNAL"`
	FrekuensiUpdate   *string `json:"frekuensiUpdate"   binding:"omitempty,oneof=HARIAN INTRA_DAY BULANAN"`
	AktifFlag         *bool   `json:"aktifFlag"`
	TanggalMulaiAktif *string `json:"tanggalMulaiAktif"`
	RowVersion        int64   `json:"rowVersion"        binding:"required"`
}

// Response is the JSON representation returned by all CRUD and workflow endpoints.
type Response struct {
	KodeMataUang       string  `json:"kodeMataUang"`
	ID                 string  `json:"id"`
	NamaMataUang       string  `json:"namaMataUang"`
	Simbol             string  `json:"simbol"`
	DecimalPlaces      int16   `json:"decimalPlaces"`
	SumberKursDefault  string  `json:"sumberKursDefault"`
	FrekuensiUpdate    string  `json:"frekuensiUpdate"`
	AktifFlag          bool    `json:"aktifFlag"`
	TanggalMulaiAktif  string  `json:"tanggalMulaiAktif"`
	IsSystemCurrency   bool    `json:"isSystemCurrency"`
	WorkflowStatus     string  `json:"workflowStatus"`
	WorkflowInstanceID *string `json:"workflowInstanceId"`
	RowVersion         int64   `json:"rowVersion"`
	CreatedAt          string  `json:"createdAt"`
	CreatedBy          *string `json:"createdBy"`
	UpdatedAt          *string `json:"updatedAt"`
	UpdatedBy          *string `json:"updatedBy"`
	DeletedAt          *string `json:"deletedAt"`
	DeletedBy          *string `json:"deletedBy"`
}

// ToResponse converts a domain entity to the JSON response shape.
func ToResponse(m *MataUang) Response {
	r := Response{
		KodeMataUang:      m.KodeMataUang,
		ID:                m.ID.String(),
		NamaMataUang:      m.NamaMataUang,
		Simbol:            m.Simbol,
		DecimalPlaces:     m.DecimalPlaces,
		SumberKursDefault: m.SumberKursDefault,
		FrekuensiUpdate:   m.FrekuensiUpdate,
		AktifFlag:         m.AktifFlag,
		TanggalMulaiAktif: m.TanggalMulaiAktif,
		IsSystemCurrency:  m.IsSystemCurrency,
		WorkflowStatus:    string(displayWorkflowStatus(m.WorkflowStatus)),
		RowVersion:        m.RowVersion,
		CreatedAt:         m.CreatedAt.Format(time.RFC3339),
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

// displayWorkflowStatus maps internal DB value to API-visible status.
// REJECTED (terminal in generic workflow) → RETURNED (master entity can re-submit).
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
	EntityID  string `json:"entityId"` // kode_mata_uang
}

// AuditHistoryItem represents one aud.audit_log row for mata_uang.
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

// ListParams holds the parsed and validated list query parameters.
type ListParams struct {
	Cursor         string
	Limit          int
	IncludeDeleted bool
}

// AllowedSortCols is the whitelist of sort-able columns (per mata-uang.yaml x-blips-allowed-cols).
// REUSE PATTERN: every module defines its own constant set here.
var AllowedSortCols = []string{
	"kode_mata_uang",
	"nama_mata_uang",
	"aktif_flag",
	"created_at",
	"tanggal_mulai_aktif",
	"workflow_status",
}

// AllowedFilterCols is the whitelist of filter-able columns.
var AllowedFilterCols = []string{
	"aktif_flag",
	"sumber_kurs_default",
	"frekuensi_update",
	"kode_mata_uang",
	"workflow_status",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"kode_mata_uang", "nama_mata_uang", "simbol"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ExportFormat is the output format for export endpoints.
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatXLSX ExportFormat = "xlsx"
)

// WorkflowActionRequest is the request body for submit/review/approve/reject.
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
