// Package coa implements the mst.chart_of_accounts master-data module (APP-A-MSTR-COA).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// Pattern follows internal/master/matauang/ — the PILOT for the generic master-data pattern.
//
// Schema note: the table uses 'version' INT (from migration 0001) as the optimistic-lock
// column. Migration 0016 adds audit columns (deleted_at, deleted_by, tenant_id,
// workflow_status) but defers the rename version → row_version to Phase 5.
// This package therefore reads/writes 'version' and exposes it as 'rowVersion' in the API.
package coa

import (
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ─────────────────────────────────────────────────────────────

const (
	// CodeCoADuplicateKode is returned when kode_akun already exists. HTTP 422.
	CodeCoADuplicateKode = string(domainerrors.CodeCoADuplicateKode)

	// CodeCoAInvalidKodeFormat is returned when kode_akun does not match the dotted
	// hierarchy pattern. HTTP 422.
	CodeCoAInvalidKodeFormat = string(domainerrors.CodeCoAInvalidKodeFormat)

	// CodeCoAParentNotFound is returned when the specified parent_akun_id does not
	// exist (or is not APPROVED). HTTP 422.
	CodeCoAParentNotFound = string(domainerrors.CodeCoAParentNotFound)

	// CodeMasterApprovedNoEdit is returned when caller tries to UPDATE an APPROVED record
	// without going through the workflow. HTTP 403.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)

	// CodeEntityInUse is returned when soft-delete is blocked by active references. HTTP 409.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)
)

// ─── Enum types ──────────────────────────────────────────────────────────────

// TipeAkun is the account type enum (DB CHECK constraint).
type TipeAkun string

const (
	TipeAkunAset       TipeAkun = "ASET"
	TipeAkunLiabilitas TipeAkun = "LIABILITAS"
	TipeAkunEkuitas    TipeAkun = "EKUITAS"
	TipeAkunPendapatan TipeAkun = "PENDAPATAN"
	TipeAkunBeban      TipeAkun = "BEBAN"
	TipeAkunKontinjen  TipeAkun = "KONTINJEN"
)

// validTipeAkun is the canonical whitelist.
var validTipeAkun = map[TipeAkun]bool{
	TipeAkunAset:       true,
	TipeAkunLiabilitas: true,
	TipeAkunEkuitas:    true,
	TipeAkunPendapatan: true,
	TipeAkunBeban:      true,
	TipeAkunKontinjen:  true,
}

// PosisiNormal is the normal balance side enum (DB CHECK constraint).
type PosisiNormal string

const (
	PosisiNormalDebit  PosisiNormal = "DEBIT"
	PosisiNormalKredit PosisiNormal = "KREDIT"
)

// validPosisiNormal is the canonical whitelist.
var validPosisiNormal = map[PosisiNormal]bool{
	PosisiNormalDebit:  true,
	PosisiNormalKredit: true,
}

// WorkflowStatus mirrors the enum values allowed in mst.chart_of_accounts.workflow_status.
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
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// ChartOfAccount is the domain entity for mst.chart_of_accounts.
// Fields match migration 0001 + additions from migration 0016.
//
// Note: the DB column for optimistic lock is 'version' INT (Phase 5 will rename to row_version).
// Internally we call it Version; the API exposes it as rowVersion.
type ChartOfAccount struct {
	ID                uuid.UUID    `db:"id"`
	KodeAkun          string       `db:"kode_akun"`
	NamaAkun          string       `db:"nama_akun"`
	TipeAkun          TipeAkun     `db:"tipe_akun"`
	SubTipeAkun       string       `db:"sub_tipe_akun"`
	KategoriInvestasi *string      `db:"kategori_investasi"`
	MataUangNative    string       `db:"mata_uang_native"`
	PosisiNormal      PosisiNormal `db:"posisi_normal"`
	AktifFlag         bool         `db:"aktif_flag"`
	ParentAkunID      *uuid.UUID   `db:"parent_akun_id"`
	SumberCoa         string       `db:"sumber_coa"`
	TanggalMulaiAktif string       `db:"tanggal_mulai_aktif"` // DATE → "YYYY-MM-DD"

	// Audit fields (from migration 0001 + 0016)
	CreatedBy uuid.UUID  `db:"created_by"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedBy *uuid.UUID `db:"updated_by"`
	UpdatedAt *time.Time `db:"updated_at"`
	DeletedAt *time.Time `db:"deleted_at"`
	DeletedBy *uuid.UUID `db:"deleted_by"`
	Version   int        `db:"version"` // optimistic lock; Phase 5 renames to row_version
	TenantID  string     `db:"tenant_id"`

	// Workflow (from migration 0016)
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit
}

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST /master/coa request body.
type CreateRequest struct {
	KodeAkun          string  `json:"kodeAkun"          binding:"required,max=20"`
	NamaAkun          string  `json:"namaAkun"          binding:"required,min=2,max=200"`
	TipeAkun          string  `json:"tipeAkun"          binding:"required"`
	SubTipeAkun       string  `json:"subTipeAkun"       binding:"required,max=30"`
	KategoriInvestasi *string `json:"kategoriInvestasi" binding:"omitempty,max=20"`
	MataUangNative    string  `json:"mataUangNative"    binding:"omitempty,len=3"`
	PosisiNormal      string  `json:"posisiNormal"      binding:"required"`
	AktifFlag         *bool   `json:"aktifFlag"`
	ParentAkunKode    *string `json:"parentAkunKode"   binding:"omitempty,max=20"`
	SumberCoa         string  `json:"sumberCoa"         binding:"required,max=30"`
	TanggalMulaiAktif string  `json:"tanggalMulaiAktif" binding:"required"`
}

// UpdateRequest is the PATCH /master/coa/:id request body.
type UpdateRequest struct {
	NamaAkun          *string `json:"namaAkun"          binding:"omitempty,min=2,max=200"`
	SubTipeAkun       *string `json:"subTipeAkun"       binding:"omitempty,max=30"`
	KategoriInvestasi *string `json:"kategoriInvestasi" binding:"omitempty,max=20"`
	MataUangNative    *string `json:"mataUangNative"    binding:"omitempty,len=3"`
	PosisiNormal      *string `json:"posisiNormal"`
	AktifFlag         *bool   `json:"aktifFlag"`
	ParentAkunKode    *string `json:"parentAkunKode"    binding:"omitempty,max=20"`
	SumberCoa         *string `json:"sumberCoa"         binding:"omitempty,max=30"`
	TanggalMulaiAktif *string `json:"tanggalMulaiAktif"`
	RowVersion        int     `json:"rowVersion"        binding:"required"`
}

// Response is the JSON representation returned by all CRUD and workflow endpoints.
type Response struct {
	ID                 string  `json:"id"`
	KodeAkun           string  `json:"kodeAkun"`
	NamaAkun           string  `json:"namaAkun"`
	TipeAkun           string  `json:"tipeAkun"`
	SubTipeAkun        string  `json:"subTipeAkun"`
	KategoriInvestasi  *string `json:"kategoriInvestasi"`
	MataUangNative     string  `json:"mataUangNative"`
	PosisiNormal       string  `json:"posisiNormal"`
	AktifFlag          bool    `json:"aktifFlag"`
	ParentAkunID       *string `json:"parentAkunId"`
	SumberCoa          string  `json:"sumberCoa"`
	TanggalMulaiAktif  string  `json:"tanggalMulaiAktif"`
	WorkflowStatus     string  `json:"workflowStatus"`
	WorkflowInstanceID *string `json:"workflowInstanceId"`
	RowVersion         int     `json:"rowVersion"`
	CreatedAt          string  `json:"createdAt"`
	CreatedBy          string  `json:"createdBy"`
	UpdatedAt          *string `json:"updatedAt"`
	UpdatedBy          *string `json:"updatedBy"`
	DeletedAt          *string `json:"deletedAt"`
	DeletedBy          *string `json:"deletedBy"`
}

// ToResponse converts a domain entity to the JSON response shape.
func ToResponse(c *ChartOfAccount) Response {
	r := Response{
		ID:                c.ID.String(),
		KodeAkun:          c.KodeAkun,
		NamaAkun:          c.NamaAkun,
		TipeAkun:          string(c.TipeAkun),
		SubTipeAkun:       c.SubTipeAkun,
		KategoriInvestasi: c.KategoriInvestasi,
		MataUangNative:    c.MataUangNative,
		PosisiNormal:      string(c.PosisiNormal),
		AktifFlag:         c.AktifFlag,
		SumberCoa:         c.SumberCoa,
		TanggalMulaiAktif: c.TanggalMulaiAktif,
		WorkflowStatus:    string(displayWorkflowStatus(c.WorkflowStatus)),
		RowVersion:        c.Version,
		CreatedAt:         c.CreatedAt.Format(time.RFC3339),
		CreatedBy:         c.CreatedBy.String(),
	}
	if c.ParentAkunID != nil {
		s := c.ParentAkunID.String()
		r.ParentAkunID = &s
	}
	if c.WorkflowInstanceID != nil {
		s := c.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if c.UpdatedAt != nil {
		s := c.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if c.UpdatedBy != nil {
		s := c.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if c.DeletedAt != nil {
		s := c.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	if c.DeletedBy != nil {
		s := c.DeletedBy.String()
		r.DeletedBy = &s
	}
	return r
}

// displayWorkflowStatus maps internal REJECTED → RETURNED for API consumers.
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

// AuditHistoryItem represents one aud.audit_log row for chart_of_accounts.
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

// WorkflowActionRequest is the request body for submit/review/approve.
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

// ─── Allowed column lists (for listquery + export) ────────────────────────────

// AllowedSortCols is the whitelist of sort-able columns.
var AllowedSortCols = []string{
	"kode_akun",
	"nama_akun",
	"tipe_akun",
	"aktif_flag",
	"created_at",
	"tanggal_mulai_aktif",
	"workflow_status",
}

// AllowedFilterCols is the whitelist of filter-able columns.
var AllowedFilterCols = []string{
	"tipe_akun",
	"posisi_normal",
	"aktif_flag",
	"workflow_status",
	"mata_uang_native",
	"sumber_coa",
	"kode_akun",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"kode_akun", "nama_akun", "sub_tipe_akun"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Import types ─────────────────────────────────────────────────────────────

// ImportXLSXRequest is the multipart/form-data request for XLSX import.
// The file is parsed separately; this captures the text fields.
type ImportXLSXRequest struct {
	SumberCoa string // form field "sumber_coa"
}

// ImportJobResponse is the 202 response body for XLSX import.
type ImportJobResponse struct {
	JobID     string `json:"jobId"`
	StatusURL string `json:"statusUrl"`
}

// ImportJobStatusResponse is the GET /import-jobs/:jobId response body.
type ImportJobStatusResponse struct {
	JobID       string  `json:"jobId"`
	Type        string  `json:"type"`
	Status      string  `json:"status"`
	Progress    int     `json:"progress"`
	CurrentStep string  `json:"currentStep"`
	RowsTotal   int     `json:"rowsTotal"`
	RowsDone    int     `json:"rowsDone"`
	RowsError   int     `json:"rowsError"`
	ErrorDetail *string `json:"errorDetail"`
	CreatedAt   string  `json:"createdAt"`
	CompletedAt *string `json:"completedAt"`
}

// XLSXRow is one parsed row from the import XLSX template.
type XLSXRow struct {
	RowNum            int
	KodeAkun          string
	NamaAkun          string
	TipeAkun          string
	SubTipeAkun       string
	KategoriInvestasi string
	MataUangNative    string
	PosisiNormal      string
	ParentAkunKode    string // resolved to UUID by service
}
