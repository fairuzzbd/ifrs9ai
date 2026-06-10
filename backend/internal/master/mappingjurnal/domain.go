// Package mappingjurnal implements the mst.mapping_jurnal_header + mst.mapping_jurnal_detail
// master-data module (APP-D).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Pattern: clone of internal/master/matauang/ — no parent-child CoA hierarchy, but has
// a child detail table (one header : many details) managed transactionally.
//
// Workflow is attached to the header only; detail rows inherit header workflow_status.
// The debit=credit multiplier invariant is enforced in service.Approve.
package mappingjurnal

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeMasterApprovedNoEdit is returned when caller tries to edit an APPROVED header.
	CodeMasterApprovedNoEdit = string(domainerrors.CodeMasterApprovedNoEdit)

	// CodeEntityInUse is returned when soft-delete is blocked by active references.
	CodeEntityInUse = string(domainerrors.CodeEntityInUse)

	// CodeMappingJurnalDebitCreditMismatch is returned when the sum of DEBIT multipliers
	// does not equal the sum of KREDIT multipliers on approve. HTTP 422.
	CodeMappingJurnalDebitCreditMismatch = string(domainerrors.CodeMappingJurnalDebitCreditMismatch)

	// CodeMappingJurnalKodeAkunNotApproved is returned when a detail references a CoA row
	// that is not yet APPROVED. HTTP 422.
	CodeMappingJurnalKodeAkunNotApproved = string(domainerrors.CodeMappingJurnalKodeAkunNotApproved)
)

// ─── Workflow status ──────────────────────────────────────────────────────────

// WorkflowStatus mirrors the allowed values for mst.mapping_jurnal_header.workflow_status.
type WorkflowStatus string

const (
	WorkflowStatusDraft            WorkflowStatus = "DRAFT"
	WorkflowStatusPendingReview    WorkflowStatus = "PENDING_REVIEW"
	WorkflowStatusPendingApproval  WorkflowStatus = "PENDING_APPROVAL"
	WorkflowStatusPendingApproval2 WorkflowStatus = "PENDING_APPROVAL_2"
	WorkflowStatusApproved         WorkflowStatus = "APPROVED"
	WorkflowStatusRejected         WorkflowStatus = "REJECTED"
	WorkflowStatusReturned         WorkflowStatus = "RETURNED" // display alias for REJECTED
)

// editableStatuses: only DRAFT or RETURNED (DB: REJECTED) can be edited.
var editableStatuses = map[WorkflowStatus]bool{
	WorkflowStatusDraft:    true,
	WorkflowStatusReturned: true,
	WorkflowStatusRejected: true,
}

// IsEditable returns true if the header can be edited.
func (s WorkflowStatus) IsEditable() bool {
	return editableStatuses[s]
}

// ─── Domain entities ──────────────────────────────────────────────────────────

// Header is the domain entity for mst.mapping_jurnal_header.
type Header struct {
	ID                   uuid.UUID      `db:"id"`
	EventIDKode          string         `db:"event_id_kode"`
	EventCode            string         `db:"event_code"`
	NamaEvent            string         `db:"nama_event"`
	KategoriEvent        string         `db:"kategori_event"`
	TriggerSource        string         `db:"trigger_source"`
	TipeInstrumenBerlaku []string       `db:"tipe_instrumen_berlaku"`
	KlasifikasiBerlaku   []string       `db:"klasifikasi_berlaku"`
	AktifFlag            bool           `db:"aktif_flag"`
	Catatan              *string        `db:"catatan"`
	WorkflowStatus       WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID   *uuid.UUID     `db:"workflow_instance_id"`

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

// Detail is the domain entity for mst.mapping_jurnal_detail.
// Detail rows do NOT carry workflow_status — they inherit from the header.
type Detail struct {
	ID                   uuid.UUID       `db:"id"`
	EventHeaderID        uuid.UUID       `db:"event_header_id"`
	Urutan               int             `db:"urutan"`
	KodeAkunID           uuid.UUID       `db:"kode_akun_id"`
	DKIndicator          string          `db:"dk_indicator"` // DEBIT | KREDIT
	SumberAmount         string          `db:"sumber_amount"`
	KlasifikasiFilter    *string         `db:"klasifikasi_filter"`
	TipeInstrumenFilter  []string        `db:"tipe_instrumen_filter"`
	UnderlyingTypeFilter *string         `db:"underlying_type_filter"`
	Multiplier           decimal.Decimal `db:"multiplier"` // NUMERIC(8,4) — shopspring/decimal
	MataUangPosting      string          `db:"mata_uang_posting"`
	AktifFlag            bool            `db:"aktif_flag"`
	Catatan              *string         `db:"catatan"`

	// Audit fields (no workflow_status — detail inherits from header)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  *uuid.UUID `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// HeaderWithDetails bundles header + its detail rows for transactional operations.
type HeaderWithDetails struct {
	Header  *Header
	Details []*Detail
}

// ─── Request / Response types ─────────────────────────────────────────────────

// DetailRequest is the detail row sub-object in create/update requests.
type DetailRequest struct {
	Urutan               int      `json:"urutan"              binding:"required,min=1"`
	KodeAkunID           string   `json:"kodeAkunId"          binding:"required,uuid"`
	DKIndicator          string   `json:"dkIndicator"         binding:"required,oneof=DEBIT KREDIT"`
	SumberAmount         string   `json:"sumberAmount"        binding:"required,min=1,max=50"`
	KlasifikasiFilter    *string  `json:"klasifikasiFilter"`
	TipeInstrumenFilter  []string `json:"tipeInstrumenFilter"`
	UnderlyingTypeFilter *string  `json:"underlyingTypeFilter"`
	Multiplier           string   `json:"multiplier"          binding:"required"` // parsed as decimal
	MataUangPosting      string   `json:"mataUangPosting"     binding:"required,len=3"`
	AktifFlag            *bool    `json:"aktifFlag"`
	Catatan              *string  `json:"catatan"`
}

// CreateRequest is the POST /master/mapping-jurnal request body.
// A single tx creates header + all details atomically.
type CreateRequest struct {
	EventIDKode          string          `json:"eventIdKode"          binding:"required,min=1,max=40"`
	EventCode            string          `json:"eventCode"            binding:"required,min=1,max=40"`
	NamaEvent            string          `json:"namaEvent"            binding:"required,min=3,max=120"`
	KategoriEvent        string          `json:"kategoriEvent"        binding:"required,min=1,max=30"`
	TriggerSource        string          `json:"triggerSource"        binding:"required,oneof=SYSTEM MANUAL FEED"`
	TipeInstrumenBerlaku []string        `json:"tipeInstrumenBerlaku"`
	KlasifikasiBerlaku   []string        `json:"klasifikasiBerlaku"`
	AktifFlag            *bool           `json:"aktifFlag"`
	Catatan              *string         `json:"catatan"`
	Details              []DetailRequest `json:"details"              binding:"required,min=2"`
}

// UpdateRequest is the PATCH /master/mapping-jurnal/:id request body.
// Replaces ALL detail rows atomically (bulk replace semantics).
type UpdateRequest struct {
	EventIDKode          *string         `json:"eventIdKode"          binding:"omitempty,min=1,max=40"`
	EventCode            *string         `json:"eventCode"            binding:"omitempty,min=1,max=40"`
	NamaEvent            *string         `json:"namaEvent"            binding:"omitempty,min=3,max=120"`
	KategoriEvent        *string         `json:"kategoriEvent"        binding:"omitempty,min=1,max=30"`
	TriggerSource        *string         `json:"triggerSource"        binding:"omitempty,oneof=SYSTEM MANUAL FEED"`
	TipeInstrumenBerlaku []string        `json:"tipeInstrumenBerlaku"`
	KlasifikasiBerlaku   []string        `json:"klasifikasiBerlaku"`
	AktifFlag            *bool           `json:"aktifFlag"`
	Catatan              *string         `json:"catatan"`
	Details              []DetailRequest `json:"details"`
	RowVersion           int64           `json:"rowVersion"           binding:"required"`
}

// ─── Response types ───────────────────────────────────────────────────────────

// DetailResponse is the JSON shape for one detail row.
type DetailResponse struct {
	ID                   string   `json:"id"`
	EventHeaderID        string   `json:"eventHeaderId"`
	Urutan               int      `json:"urutan"`
	KodeAkunID           string   `json:"kodeAkunId"`
	DKIndicator          string   `json:"dkIndicator"`
	SumberAmount         string   `json:"sumberAmount"`
	KlasifikasiFilter    *string  `json:"klasifikasiFilter"`
	TipeInstrumenFilter  []string `json:"tipeInstrumenFilter"`
	UnderlyingTypeFilter *string  `json:"underlyingTypeFilter"`
	Multiplier           string   `json:"multiplier"` // decimal string, 4dp
	MataUangPosting      string   `json:"mataUangPosting"`
	AktifFlag            bool     `json:"aktifFlag"`
	Catatan              *string  `json:"catatan"`
	RowVersion           int64    `json:"rowVersion"`
}

// HeaderResponse is the JSON shape for a header (with nested details).
type HeaderResponse struct {
	ID                   string           `json:"id"`
	EventIDKode          string           `json:"eventIdKode"`
	EventCode            string           `json:"eventCode"`
	NamaEvent            string           `json:"namaEvent"`
	KategoriEvent        string           `json:"kategoriEvent"`
	TriggerSource        string           `json:"triggerSource"`
	TipeInstrumenBerlaku []string         `json:"tipeInstrumenBerlaku"`
	KlasifikasiBerlaku   []string         `json:"klasifikasiBerlaku"`
	AktifFlag            bool             `json:"aktifFlag"`
	Catatan              *string          `json:"catatan"`
	WorkflowStatus       string           `json:"workflowStatus"`
	WorkflowInstanceID   *string          `json:"workflowInstanceId"`
	Details              []DetailResponse `json:"details"`
	RowVersion           int64            `json:"rowVersion"`
	CreatedAt            string           `json:"createdAt"`
	CreatedBy            *string          `json:"createdBy"`
	UpdatedAt            *string          `json:"updatedAt"`
	UpdatedBy            *string          `json:"updatedBy"`
	DeletedAt            *string          `json:"deletedAt"`
	DeletedBy            *string          `json:"deletedBy"`
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
	BeforeJSONB interface{} `json:"beforeJsonb"`
	AfterJSONB  interface{} `json:"afterJsonb"`
	IP          *string     `json:"ip"`
	TraceID     *string     `json:"traceId"`
}

// ─── Allowed columns for listquery ───────────────────────────────────────────

// AllowedSortCols is the whitelist of sortable columns.
var AllowedSortCols = []string{
	"id",
	"event_code",
	"event_id_kode",
	"nama_event",
	"kategori_event",
	"trigger_source",
	"aktif_flag",
	"created_at",
	"workflow_status",
}

// AllowedFilterCols is the whitelist of filterable columns.
var AllowedFilterCols = []string{
	"aktif_flag",
	"kategori_event",
	"trigger_source",
	"workflow_status",
	"event_code",
}

// SearchCols are the columns scanned by ?q= text search.
var SearchCols = []string{"event_code", "event_id_kode", "nama_event", "kategori_event"}

// AllAllowedCols is the union used for listquery.ParseFromRequest.
var AllAllowedCols = func() []string {
	seen := make(map[string]bool)
	var all []string
	for _, c := range AllowedSortCols {
		if !seen[c] {
			seen[c] = true
			all = append(all, c)
		}
	}
	for _, c := range AllowedFilterCols {
		if !seen[c] {
			seen[c] = true
			all = append(all, c)
		}
	}
	return all
}()

// ─── Converters ───────────────────────────────────────────────────────────────

// ToDetailResponse converts a domain Detail to DetailResponse.
func ToDetailResponse(d *Detail) DetailResponse {
	r := DetailResponse{
		ID:                   d.ID.String(),
		EventHeaderID:        d.EventHeaderID.String(),
		Urutan:               d.Urutan,
		KodeAkunID:           d.KodeAkunID.String(),
		DKIndicator:          d.DKIndicator,
		SumberAmount:         d.SumberAmount,
		KlasifikasiFilter:    d.KlasifikasiFilter,
		TipeInstrumenFilter:  d.TipeInstrumenFilter,
		UnderlyingTypeFilter: d.UnderlyingTypeFilter,
		Multiplier:           d.Multiplier.StringFixed(4),
		MataUangPosting:      d.MataUangPosting,
		AktifFlag:            d.AktifFlag,
		Catatan:              d.Catatan,
		RowVersion:           d.RowVersion,
	}
	if r.TipeInstrumenFilter == nil {
		r.TipeInstrumenFilter = []string{}
	}
	return r
}

// ToHeaderResponse converts a HeaderWithDetails to HeaderResponse.
func ToHeaderResponse(hd *HeaderWithDetails) HeaderResponse {
	h := hd.Header
	r := HeaderResponse{
		ID:                   h.ID.String(),
		EventIDKode:          h.EventIDKode,
		EventCode:            h.EventCode,
		NamaEvent:            h.NamaEvent,
		KategoriEvent:        h.KategoriEvent,
		TriggerSource:        h.TriggerSource,
		TipeInstrumenBerlaku: h.TipeInstrumenBerlaku,
		KlasifikasiBerlaku:   h.KlasifikasiBerlaku,
		AktifFlag:            h.AktifFlag,
		Catatan:              h.Catatan,
		WorkflowStatus:       string(displayWorkflowStatus(h.WorkflowStatus)),
		RowVersion:           h.RowVersion,
		CreatedAt:            h.CreatedAt.Format(time.RFC3339),
	}

	if r.TipeInstrumenBerlaku == nil {
		r.TipeInstrumenBerlaku = []string{}
	}
	if r.KlasifikasiBerlaku == nil {
		r.KlasifikasiBerlaku = []string{}
	}

	if h.WorkflowInstanceID != nil {
		s := h.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if h.CreatedBy != nil {
		s := h.CreatedBy.String()
		r.CreatedBy = &s
	}
	if h.UpdatedAt != nil {
		s := h.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if h.UpdatedBy != nil {
		s := h.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if h.DeletedAt != nil {
		s := h.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}
	if h.DeletedBy != nil {
		s := h.DeletedBy.String()
		r.DeletedBy = &s
	}

	details := make([]DetailResponse, 0, len(hd.Details))
	for _, d := range hd.Details {
		details = append(details, ToDetailResponse(d))
	}
	r.Details = details
	return r
}

// ToHeaderResponseNoDetails converts a Header (without fetching details) for list endpoints.
func ToHeaderResponseNoDetails(h *Header) HeaderResponse {
	return ToHeaderResponse(&HeaderWithDetails{Header: h, Details: nil})
}

// displayWorkflowStatus maps REJECTED → RETURNED for API output.
func displayWorkflowStatus(s WorkflowStatus) WorkflowStatus {
	if s == WorkflowStatusRejected {
		return WorkflowStatusReturned
	}
	return s
}

// WorkflowActionRequest is the body for submit/review/approve/reject.
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
