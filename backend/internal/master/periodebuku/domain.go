// Package periodebuku implements the mst.periode_buku master-data module (APP-D-MSTR-001).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// Pattern follows backend/internal/master/matauang/ (pilot module).
//
// NOT in scope for this PR (Phase 5 / APP-D):
//   - POST /:id/softclose   — Soft-close period (status_periode OPEN→SOFT_CLOSED)
//   - POST /:id/hardclose   — Hard-close period (status_periode SOFT_CLOSED→CLOSED), CFO MFA step-up
//   - POST /:id/reopen      — Reopen soft-closed period (SOFT_CLOSED→OPEN)
//
// status_periode (OPEN/SOFT_CLOSED/CLOSED) has its own domain state-machine, managed by
// the APP-D Periode Buku module (Phase 5). This package MUST NOT mutate status_periode
// via CRUD — it is always OPEN on insert.
package periodebuku

import (
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes (aliases to canonical domainerrors constants) ────────────────

const (
	// CodeEntityInUse is returned when soft-delete is blocked by active references.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)

	// CodeMasterApprovedNoEdit is returned when caller tries to UPDATE an APPROVED record.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)
)

// ─── Domain enums ─────────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values allowed in mst.periode_buku.workflow_status.
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

// IsEditable returns true if the record can be edited (before PENDING_REVIEW or after RETURNED).
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// TipePeriode is the type of accounting period.
type TipePeriode string

const (
	TipePeriodeBulanan    TipePeriode = "BULANAN"
	TipePeriodeTriwulanan TipePeriode = "TRIWULANAN"
	TipePeriodeTahunan    TipePeriode = "TAHUNAN"
)

// StatusPeriode tracks the financial lifecycle state (NOT the workflow approval state).
// This is managed exclusively by APP-D Phase 5. CRUD only ever inserts OPEN.
type StatusPeriode string

const (
	StatusPeriodeOpen       StatusPeriode = "OPEN"
	StatusPeriodeSoftClosed StatusPeriode = "SOFT_CLOSED"
	StatusPeriodeClosed     StatusPeriode = "CLOSED"
)

// ─── Domain entity ────────────────────────────────────────────────────────────

// PeriodeBuku is the domain entity for mst.periode_buku.
// Fields match migration 000001 (base) + 000009 (audit cols + workflow_status).
type PeriodeBuku struct {
	// Surrogate UUID — used as workflow entity_id and API :id.
	ID uuid.UUID `db:"id"`

	// Business key (UNIQUE, immutable after create)
	// Format examples: "2026-M06", "2026-Q2", "2026-Y"
	PeriodeIDKode string `db:"periode_id_kode"`

	// Period classification
	TipePeriode TipePeriode `db:"tipe_periode"`
	TahunBuku   int         `db:"tahun_buku"`
	Bulan       *int        `db:"bulan"`    // not null for BULANAN
	Triwulan    *int        `db:"triwulan"` // not null for TRIWULANAN

	// Calendar bounds
	TanggalMulai string `db:"tanggal_mulai"` // DATE as "YYYY-MM-DD"
	TanggalAkhir string `db:"tanggal_akhir"` // DATE as "YYYY-MM-DD"

	// Domain lifecycle status — managed by APP-D Phase 5, never mutated here.
	StatusPeriode StatusPeriode `db:"status_periode"`

	// Soft/hard close fields (read-only from CRUD perspective — Phase 5 populates these)
	TanggalSoftClose    *time.Time `db:"tanggal_soft_close"`
	TanggalHardClose    *time.Time `db:"tanggal_hard_close"`
	UserCloserID        *uuid.UUID `db:"user_closer_id"`
	UserApproverCloseID *uuid.UUID `db:"user_approver_close_id"`
	CatatanClosing      *string    `db:"catatan_closing"`

	// Reopen fields (read-only from CRUD perspective)
	ReopenedFlag       bool       `db:"reopened_flag"`
	ReopenedReason     *string    `db:"reopened_reason"`
	ReopenedAt         *time.Time `db:"reopened_at"`
	ReopenedBy         *uuid.UUID `db:"reopened_by"`
	ReopenedApprovedBy *uuid.UUID `db:"reopened_approved_by"`

	// Workflow
	WorkflowStatus WorkflowStatus `db:"workflow_status"`

	// Audit fields (from migration 000009)
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

// CreateRequest is the POST /master/periode-buku request body.
type CreateRequest struct {
	PeriodeIDKode string      `json:"periodeIdKode" binding:"required,min=3,max=20"`
	TipePeriode   TipePeriode `json:"tipePeriode"   binding:"required,oneof=BULANAN TRIWULANAN TAHUNAN"`
	TahunBuku     int         `json:"tahunBuku"     binding:"required,min=2000,max=2100"`
	Bulan         *int        `json:"bulan"`
	Triwulan      *int        `json:"triwulan"`
	TanggalMulai  string      `json:"tanggalMulai"  binding:"required"`
	TanggalAkhir  string      `json:"tanggalAkhir"  binding:"required"`
}

// UpdateRequest is the PATCH /master/periode-buku/:id request body.
// id (UUID) is immutable. PeriodeIDKode is immutable. TipePeriode is immutable.
type UpdateRequest struct {
	TahunBuku    *int    `json:"tahunBuku"    binding:"omitempty,min=2000,max=2100"`
	Bulan        *int    `json:"bulan"`
	Triwulan     *int    `json:"triwulan"`
	TanggalMulai *string `json:"tanggalMulai"`
	TanggalAkhir *string `json:"tanggalAkhir"`
	RowVersion   int64   `json:"rowVersion"   binding:"required"`
}

// GenerateRequest is the POST /master/periode-buku/generate request body.
type GenerateRequest struct {
	TahunBuku int           `json:"tahunBuku" binding:"required,min=2000,max=2100"`
	Tipe      []TipePeriode `json:"tipe"` // defaults to all three if empty
}

// GenerateResult is the response body for the generate endpoint.
type GenerateResult struct {
	Generated int        `json:"generated"`
	Skipped   int        `json:"skipped"`
	Rows      []Response `json:"rows"`
}

// Response is the JSON representation returned by all CRUD and workflow endpoints.
type Response struct {
	ID             string  `json:"id"`
	PeriodeIDKode  string  `json:"periodeIdKode"`
	TipePeriode    string  `json:"tipePeriode"`
	TahunBuku      int     `json:"tahunBuku"`
	Bulan          *int    `json:"bulan"`
	Triwulan       *int    `json:"triwulan"`
	TanggalMulai   string  `json:"tanggalMulai"`
	TanggalAkhir   string  `json:"tanggalAkhir"`
	StatusPeriode  string  `json:"statusPeriode"`
	WorkflowStatus string  `json:"workflowStatus"`
	RowVersion     int64   `json:"rowVersion"`
	CreatedAt      string  `json:"createdAt"`
	CreatedBy      *string `json:"createdBy"`
	UpdatedAt      *string `json:"updatedAt"`
	UpdatedBy      *string `json:"updatedBy"`
	DeletedAt      *string `json:"deletedAt"`
	DeletedBy      *string `json:"deletedBy"`
}

// ToResponse converts a domain entity to the JSON response shape.
func ToResponse(p *PeriodeBuku) Response {
	r := Response{
		ID:             p.ID.String(),
		PeriodeIDKode:  p.PeriodeIDKode,
		TipePeriode:    string(p.TipePeriode),
		TahunBuku:      p.TahunBuku,
		Bulan:          p.Bulan,
		Triwulan:       p.Triwulan,
		TanggalMulai:   p.TanggalMulai,
		TanggalAkhir:   p.TanggalAkhir,
		StatusPeriode:  string(p.StatusPeriode),
		WorkflowStatus: string(displayWorkflowStatus(p.WorkflowStatus)),
		RowVersion:     p.RowVersion,
		CreatedAt:      p.CreatedAt.Format(time.RFC3339),
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
	EntityID  string `json:"entityId"` // UUID string
}

// AuditHistoryItem represents one aud.audit_log row for periode_buku.
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

// ─── Allowed column whitelists (per api-conventions.md / ux-patterns.md §1) ──

// AllowedSortCols is the whitelist of sortable columns per task spec.
var AllowedSortCols = []string{
	"periode_id_kode",
	"tahun_buku",
	"bulan",
	"tipe_periode",
	"status_periode",
	"workflow_status",
	"created_at",
}

// AllowedFilterCols is the whitelist of filterable columns per task spec.
var AllowedFilterCols = []string{
	"tipe_periode",
	"status_periode",
	"tahun_buku",
	"workflow_status",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"periode_id_kode"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ExportFormat is the output format for export endpoints.
type ExportFormat string

const (
	ExportFormatCSV  ExportFormat = "csv"
	ExportFormatXLSX ExportFormat = "xlsx"
)
