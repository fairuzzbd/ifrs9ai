// Package kurs implements the mst.kurs master-data module (APP-A-MSTR-009).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// Directory shape follows the matauang pilot pattern:
//
//	internal/master/kurs/
//	  domain.go         — entity struct, request/response types, error codes
//	  repo.go           — SQL; no business logic
//	  service.go        — tx boundary, validation, audit write, workflow hook
//	  handler.go        — thin HTTP; calls service; maps to response envelopes
//	  routes.go         — RegisterRoutes(v1 *gin.RouterGroup, ...)
//	  jisdor_sync.go    — POST /jisdor-sync endpoint + integration call
//	  handler_test.go   — service-level unit tests (stub repo)
//	  testutil_test.go  — shared test helpers
//
// Domain rules enforced at service layer:
//   - kode_mata_uang MUST NOT be 'IDR' (self-referential rate makes no sense, 422)
//   - kurs_tengah > 0
//   - if kurs_beli and kurs_jual both provided: kurs_beli ≤ kurs_tengah ≤ kurs_jual
//   - tanggal_berlaku ≤ today + 1 day (sanity, 422)
//   - locked_flag = true → UPDATE rejected (DB trigger also enforces this)
//   - sumber_kurs whitelist: BI_JISDOR, BI_KURS_TENGAH, INTERNAL, MANUAL
//   - UNIQUE (kode_mata_uang, tanggal_berlaku) — PG unique constraint, 409 on conflict
package kurs

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ─────────────────────────────────────────────────────────────

const (
	// CodeKursInvalidCurrency is returned when kode_mata_uang == 'IDR'. HTTP 422.
	CodeKursInvalidCurrency = "KURS_INVALID_CURRENCY"

	// CodeKursInvalidRates is returned when rate relationships are invalid. HTTP 422.
	CodeKursInvalidRates = "KURS_INVALID_RATES"

	// CodeKursLocked is returned when locked_flag = true (periode CLOSED). HTTP 423.
	CodeKursLocked = "KURS_LOCKED"

	// CodeKursDuplicateDate is returned when (kode_mata_uang, tanggal_berlaku) already exists. HTTP 409.
	CodeKursDuplicateDate = "KURS_DUPLICATE_DATE"

	// CodeKursPeriodeNotFound is returned when tanggal_berlaku does not fall in any active periode. HTTP 422.
	CodeKursPeriodeNotFound = "KURS_PERIODE_NOT_FOUND"
)

// kursError is a local DomainError alias carrying kurs-specific codes.
func newKursErr(code string, message string, details ...domainerrors.Detail) *domainerrors.DomainError {
	// Map kurs-specific codes to standard domain codes where appropriate.
	switch code {
	case CodeKursDuplicateDate:
		return domainerrors.New(domainerrors.CodeConflict, message, details...)
	case CodeKursLocked:
		return domainerrors.New(domainerrors.CodePeriodeClosed, message, details...)
	default:
		return domainerrors.New(domainerrors.CodeValidationFailed, message, details...)
	}
}

// ─── Workflow status ──────────────────────────────────────────────────────────

// WorkflowStatus mirrors the enum values allowed in mst.kurs.workflow_status.
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

// ─── Sumber kurs whitelist ────────────────────────────────────────────────────

// SumberKurs whitelist values (aligned with CHECK constraint in migration).
type SumberKurs string

const (
	SumberKursJISDOR   SumberKurs = "BI_JISDOR"
	SumberKursTengah   SumberKurs = "BI_KURS_TENGAH"
	SumberKursInternal SumberKurs = "INTERNAL"
	SumberKursManual   SumberKurs = "MANUAL"
)

// validSumberKurs is the allowed set.
var validSumberKurs = map[SumberKurs]bool{
	SumberKursJISDOR:   true,
	SumberKursTengah:   true,
	SumberKursInternal: true,
	SumberKursManual:   true,
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// Kurs is the domain entity for mst.kurs.
// Monetary fields use decimal.Decimal (DEC-016 — no float64 for rates).
type Kurs struct {
	// Surrogate UUID (primary key)
	ID uuid.UUID `db:"id"`

	// Business identifier (auto-generated: {kode_mata_uang}_{YYYYMMDD})
	FxRateIDKode string `db:"fx_rate_id_kode"`

	// FK to mst.mata_uang — must be APPROVED and != 'IDR'
	KodeMataUang string `db:"kode_mata_uang"`

	// Rate date
	TanggalBerlaku time.Time `db:"tanggal_berlaku"`

	// Rate values — all NUMERIC(15,4) in DB, decimal.Decimal in Go (DEC-016)
	KursBeli   *decimal.Decimal `db:"kurs_beli"`   // nullable
	KursJual   *decimal.Decimal `db:"kurs_jual"`   // nullable
	KursTengah decimal.Decimal  `db:"kurs_tengah"` // NOT NULL

	// Rate source
	SumberKurs SumberKurs `db:"sumber_kurs"`

	// FK to mst.periode_buku — resolved from tanggal_berlaku
	PeriodeBulananID uuid.UUID `db:"periode_bulanan_id"`

	// Lock flag — set true by periode-buku CLOSE process
	LockedFlag bool `db:"locked_flag"`

	// Legacy columns (kept from 000001; set via workflow or JISDOR feed)
	MakerID    *uuid.UUID `db:"maker_id"`
	ApproverID *uuid.UUID `db:"approver_id"`
	ApprovedAt *time.Time `db:"approved_at"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"`

	// Audit fields (from migration 000020)
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

// CreateRequest is the POST /master/kurs request body.
type CreateRequest struct {
	KodeMataUang   string  `json:"kodeMataUang"   binding:"required"`
	TanggalBerlaku string  `json:"tanggalBerlaku" binding:"required"` // "YYYY-MM-DD"
	KursBeli       *string `json:"kursBeli"`                          // optional, decimal string
	KursJual       *string `json:"kursJual"`                          // optional, decimal string
	KursTengah     string  `json:"kursTengah"     binding:"required"` // decimal string
	SumberKurs     string  `json:"sumberKurs"     binding:"required"`
}

// UpdateRequest is the PUT /master/kurs/:id request body.
type UpdateRequest struct {
	KursBeli   *string `json:"kursBeli"`
	KursJual   *string `json:"kursJual"`
	KursTengah *string `json:"kursTengah"`
	SumberKurs *string `json:"sumberKurs"`
	RowVersion int64   `json:"rowVersion" binding:"required"`
}

// Response is the JSON representation returned by all endpoints.
type Response struct {
	ID                 string  `json:"id"`
	FxRateIDKode       string  `json:"fxRateIdKode"`
	KodeMataUang       string  `json:"kodeMataUang"`
	TanggalBerlaku     string  `json:"tanggalBerlaku"` // "YYYY-MM-DD"
	KursBeli           *string `json:"kursBeli"`
	KursJual           *string `json:"kursJual"`
	KursTengah         string  `json:"kursTengah"`
	SumberKurs         string  `json:"sumberKurs"`
	PeriodeBulananID   string  `json:"periodeBulananId"`
	LockedFlag         bool    `json:"lockedFlag"`
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

// ToResponse converts a domain entity to the API response shape.
func ToResponse(k *Kurs) Response {
	r := Response{
		ID:               k.ID.String(),
		FxRateIDKode:     k.FxRateIDKode,
		KodeMataUang:     k.KodeMataUang,
		TanggalBerlaku:   k.TanggalBerlaku.Format("2006-01-02"),
		KursTengah:       k.KursTengah.StringFixed(4),
		SumberKurs:       string(k.SumberKurs),
		PeriodeBulananID: k.PeriodeBulananID.String(),
		LockedFlag:       k.LockedFlag,
		WorkflowStatus:   string(displayWorkflowStatus(k.WorkflowStatus)),
		RowVersion:       k.RowVersion,
		CreatedAt:        k.CreatedAt.Format(time.RFC3339),
	}

	if k.KursBeli != nil {
		s := k.KursBeli.StringFixed(4)
		r.KursBeli = &s
	}
	if k.KursJual != nil {
		s := k.KursJual.StringFixed(4)
		r.KursJual = &s
	}
	if k.WorkflowInstanceID != nil {
		s := k.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if k.CreatedBy != nil {
		s := k.CreatedBy.String()
		r.CreatedBy = &s
	}
	if k.UpdatedAt != nil {
		s := k.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if k.UpdatedBy != nil {
		s := k.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if k.DeletedAt != nil {
		s := k.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	if k.DeletedBy != nil {
		s := k.DeletedBy.String()
		r.DeletedBy = &s
	}
	return r
}

// displayWorkflowStatus maps REJECTED → RETURNED for API consumers.
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

// AuditHistoryItem represents one aud.audit_log row for kurs.
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

// JISDORSyncRequest is the POST /master/kurs/jisdor-sync request body.
type JISDORSyncRequest struct {
	TanggalBerlaku string `json:"tanggalBerlaku" binding:"required"` // "YYYY-MM-DD"
}

// JISDORSyncResponse is returned by the jisdor-sync endpoint.
type JISDORSyncResponse struct {
	JobID     string `json:"jobId"`
	StatusURL string `json:"statusUrl"`
	Message   string `json:"message"`
}

// ─── List / sort / filter config ─────────────────────────────────────────────

// AllowedSortCols is the whitelist of sort-able columns.
var AllowedSortCols = []string{
	"tanggal_berlaku",
	"kode_mata_uang",
	"kurs_tengah",
	"sumber_kurs",
	"created_at",
	"workflow_status",
}

// AllowedFilterCols is the whitelist of filter-able columns.
var AllowedFilterCols = []string{
	"kode_mata_uang",
	"sumber_kurs",
	"workflow_status",
	"locked_flag",
	"tanggal_berlaku",
	"periode_bulanan_id",
}

// SearchCols are the columns scanned by the ?q= text search.
var SearchCols = []string{"kode_mata_uang", "fx_rate_id_kode", "sumber_kurs"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)
