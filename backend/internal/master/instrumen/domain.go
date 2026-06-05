// Package instrumen implements the mst.instrumen master-data module (APP-A-MSTR-011).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// This is the most complex master module: it has 4+ FK references (counterparty,
// portofolio, mata_uang, bank_kustodian, manajer_investasi), cross-FK APPROVED-state
// validation, klasifikasi locking semantics, and tipe-instrumen business rules.
//
// Klasifikasi locking: once SPPI+BM workflow is APPROVED (Phase 4), the
// klasifikasi_locked_at field is set. After that, the CRUD service rejects any
// attempt to change klasifikasi_psak71, fvoci_election, or bm_category.
// Phase 3: locking fields exist but the lock is never triggered via this service.
package instrumen

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeInstrumenDuplicateKode is returned when kode_instrumen already exists. HTTP 409.
	CodeInstrumenDuplicateKode = string(domainerrors.CodeInstrumenDuplicateKode)

	// CodeInstrumenCounterpartyNotApproved is returned when counterparty_id does not
	// exist or is not in APPROVED workflow_status. HTTP 422.
	CodeInstrumenCounterpartyNotApproved = string(domainerrors.CodeInstrumenCounterpartyNotApproved)

	// CodeInstrumenPortofolioNotApproved is returned when portofolio_id does not
	// exist or is not in APPROVED workflow_status. HTTP 422.
	CodeInstrumenPortofolioNotApproved = string(domainerrors.CodeInstrumenPortofolioNotApproved)

	// CodeInstrumenMataUangNotApproved is returned when mata_uang does not
	// exist or is not in APPROVED workflow_status. HTTP 422.
	CodeInstrumenMataUangNotApproved = string(domainerrors.CodeInstrumenMataUangNotApproved)

	// CodeInstrumenInvalidTipe is returned when tipe_instrumen is not in the whitelist.
	// HTTP 422.
	CodeInstrumenInvalidTipe = string(domainerrors.CodeInstrumenInvalidTipe)

	// CodeInstrumenKlasifikasiLocked is returned when caller tries to modify
	// klasifikasi_psak71, bm_category, or fvoci_election on a locked record.
	// HTTP 423.
	CodeInstrumenKlasifikasiLocked = string(domainerrors.CodeInstrumenKlasifikasiLocked)

	// CodeInstrumenMissingKustodian is returned when tipe_instrumen is SAHAM or
	// REKSADANA but bank_kustodian_id is not provided. HTTP 422.
	CodeInstrumenMissingKustodian = string(domainerrors.CodeInstrumenMissingKustodian)
)

// ─── Allowed enum values ──────────────────────────────────────────────────────

// AllowedTipeInstrumen is the server-side whitelist enforced by service validation.
// Must match ck_instrumen_tipe in migration 0019.
var AllowedTipeInstrumen = map[string]bool{
	"DEPOSITO": true,
	"OBLIGASI": true,
	"SAHAM":    true,
	"REKSADANA": true,
	"SBN":      true,
	"SPN":      true,
	"SUKUK":    true,
}

// TipeInstrumenRequiresKustodian lists tipe values that require bank_kustodian_id.
var TipeInstrumenRequiresKustodian = map[string]bool{
	"SAHAM":     true,
	"REKSADANA": true,
}

// TipeInstrumenRequiresManajerInvestasi lists tipe values that require manajer_investasi_id.
var TipeInstrumenRequiresManajerInvestasi = map[string]bool{
	"REKSADANA": true,
}

// AllowedKlasifikasi is the whitelist for klasifikasi_psak71 (matches DB CHECK).
var AllowedKlasifikasi = map[string]bool{
	"AC":             true,
	"FVOCI":          true,
	"FVOCI_ELECTION": true,
	"FVTPL":          true,
}

// WorkflowStatus mirrors the 7-state enum used in mst.instrumen.workflow_status.
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

// IsEditable returns true if the record can be edited via CRUD (DRAFT or RETURNED/REJECTED).
func (s WorkflowStatus) IsEditable() bool {
	return s == WorkflowStatusDraft || s == WorkflowStatusRejected || s == WorkflowStatusReturned
}

// displayWorkflowStatus maps internal DB value to API-visible value.
func displayWorkflowStatus(s WorkflowStatus) WorkflowStatus {
	if s == WorkflowStatusRejected {
		return WorkflowStatusReturned
	}
	return s
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// Instrumen is the domain entity for mst.instrumen.
// NUMERIC(20,2) money columns → decimal.Decimal (DEC-016, never float64).
type Instrumen struct {
	// Primary key
	ID uuid.UUID `db:"id"`

	// Core identifiers
	KodeInstrumen string `db:"kode_instrumen"`
	TipeInstrumen string `db:"tipe_instrumen"`
	SubTipe       string `db:"sub_tipe"`
	Nama          string `db:"nama"`
	ISIN          *string `db:"isin"`

	// Foreign keys
	CounterpartyID        uuid.UUID  `db:"counterparty_id"`
	ManajerInvestasiID    *uuid.UUID `db:"manajer_investasi_id"`
	BankKustodianID       *uuid.UUID `db:"bank_kustodian_id"`
	MataUang              string     `db:"mata_uang"` // CHAR(3) FK → mst.mata_uang
	PortofolioID          uuid.UUID  `db:"portofolio_id"`

	// Financial fields — decimal to avoid float64 (DEC-016)
	Nominal                  decimal.Decimal  `db:"nominal"`
	JumlahLot                *decimal.Decimal `db:"jumlah_lot"`
	TanggalPenempatan        string           `db:"tanggal_penempatan"` // DATE → YYYY-MM-DD
	TanggalJatuhTempo        *string          `db:"tanggal_jatuh_tempo"`
	Kupon                    *decimal.Decimal `db:"kupon"`
	FrekuensiBunga           *string          `db:"frekuensi_bunga"`
	AutoRenewalFlag          bool             `db:"auto_renewal_flag"`

	// PSAK 71 classification fields
	FvociElection          bool    `db:"fvoci_election"`
	SppiResult             *string `db:"sppi_result"`
	BmCategory             *string `db:"bm_category"`
	KlasifikasiPsak71      *string `db:"klasifikasi_psak71"`
	KlasifikasiLockedAt    *time.Time `db:"klasifikasi_locked_at"`
	KlasifikasiLockedBy    *uuid.UUID `db:"klasifikasi_locked_by"`
	SppiBmLastReviewDate   *string    `db:"sppi_bm_last_review_date"`

	// EIR / amortization fields
	EirAwal                  *decimal.Decimal `db:"eir_awal"`
	TanggalEirComputed       *string          `db:"tanggal_eir_computed"`
	PremiumDiskonto          decimal.Decimal  `db:"premium_diskonto_awal"`
	BiayaTransaksi           decimal.Decimal  `db:"biaya_transaksi_capitalized"`
	EirMethodFlag            bool             `db:"eir_method_flag"`
	DayCountConvention       string           `db:"day_count_convention"`
	AmortizationFrequency    *string          `db:"amortization_frequency"`

	// Status
	Status         string `db:"status"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"`

	// Audit columns (from migration 0019 + 0001 originals)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`

	// Legacy fields (from 0001; kept for query compatibility)
	Version   int  `db:"version"`
	IsDeleted bool `db:"is_deleted"`
}

// ─── Request types ────────────────────────────────────────────────────────────

// CreateRequest is the POST /master/instrumen request body.
type CreateRequest struct {
	KodeInstrumen          string   `json:"kodeInstrumen"          binding:"required,min=2,max=20"`
	TipeInstrumen          string   `json:"tipeInstrumen"          binding:"required"`
	SubTipe                string   `json:"subTipe"                binding:"required,max=50"`
	Nama                   string   `json:"nama"                   binding:"required,min=2,max=200"`
	ISIN                   *string  `json:"isin"                   binding:"omitempty,max=20"`
	CounterpartyID         string   `json:"counterpartyId"         binding:"required,uuid"`
	ManajerInvestasiID     *string  `json:"manajerInvestasiId"     binding:"omitempty,uuid"`
	BankKustodianID        *string  `json:"bankKustodianId"        binding:"omitempty,uuid"`
	MataUang               string   `json:"mataUang"               binding:"required,len=3"`
	PortofolioID           string   `json:"portofolioId"           binding:"required,uuid"`
	Nominal                string   `json:"nominal"                binding:"required"`
	JumlahLot              *string  `json:"jumlahLot"              binding:"omitempty"`
	TanggalPenempatan      string   `json:"tanggalPenempatan"      binding:"required"`
	TanggalJatuhTempo      *string  `json:"tanggalJatuhTempo"      binding:"omitempty"`
	Kupon                  *string  `json:"kupon"                  binding:"omitempty"`
	FrekuensiBunga         *string  `json:"frekuensiBunga"         binding:"omitempty,max=20"`
	AutoRenewalFlag        *bool    `json:"autoRenewalFlag"`
	FvociElection          *bool    `json:"fvociElection"`
	BmCategory             *string  `json:"bmCategory"             binding:"omitempty,oneof=HTC HTC_S OTHER"`
	EirAwal                *string  `json:"eirAwal"                binding:"omitempty"`
	PremiumDiskonto        *string  `json:"premiumDiskonto"        binding:"omitempty"`
	BiayaTransaksi         *string  `json:"biayaTransaksi"         binding:"omitempty"`
	EirMethodFlag          *bool    `json:"eirMethodFlag"`
	DayCountConvention     *string  `json:"dayCountConvention"     binding:"omitempty,max=10"`
	AmortizationFrequency  *string  `json:"amortizationFrequency"  binding:"omitempty,max=20"`
	Status                 *string  `json:"status"                 binding:"omitempty,oneof=AKTIF TIDAK_AKTIF MATURED SOLD"`
}

// UpdateRequest is the PUT /master/instrumen/:id request body.
// klasifikasi_psak71, bm_category, fvoci_election are present but service rejects
// if klasifikasi_locked_at IS NOT NULL.
type UpdateRequest struct {
	SubTipe                *string `json:"subTipe"                binding:"omitempty,max=50"`
	Nama                   *string `json:"nama"                   binding:"omitempty,min=2,max=200"`
	ISIN                   *string `json:"isin"                   binding:"omitempty,max=20"`
	ManajerInvestasiID     *string `json:"manajerInvestasiId"     binding:"omitempty,uuid"`
	BankKustodianID        *string `json:"bankKustodianId"        binding:"omitempty,uuid"`
	MataUang               *string `json:"mataUang"               binding:"omitempty,len=3"`
	Kupon                  *string `json:"kupon"                  binding:"omitempty"`
	FrekuensiBunga         *string `json:"frekuensiBunga"         binding:"omitempty,max=20"`
	AutoRenewalFlag        *bool   `json:"autoRenewalFlag"`
	FvociElection          *bool   `json:"fvociElection"`
	BmCategory             *string `json:"bmCategory"             binding:"omitempty,oneof=HTC HTC_S OTHER"`
	EirAwal                *string `json:"eirAwal"                binding:"omitempty"`
	DayCountConvention     *string `json:"dayCountConvention"     binding:"omitempty,max=10"`
	AmortizationFrequency  *string `json:"amortizationFrequency"  binding:"omitempty,max=20"`
	Status                 *string `json:"status"                 binding:"omitempty,oneof=AKTIF TIDAK_AKTIF MATURED SOLD"`
	RowVersion             int64   `json:"rowVersion"             binding:"required"`
}

// WorkflowActionRequest is the request body for workflow state transitions.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds mandatory comment.
type WorkflowRejectRequest struct {
	Comment         string  `json:"comment"        binding:"required,min=10"`
	SignatureMethod string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// ─── Response type ────────────────────────────────────────────────────────────

// Response is the JSON shape returned by all CRUD and workflow endpoints.
type Response struct {
	ID                     string  `json:"id"`
	KodeInstrumen          string  `json:"kodeInstrumen"`
	TipeInstrumen          string  `json:"tipeInstrumen"`
	SubTipe                string  `json:"subTipe"`
	Nama                   string  `json:"nama"`
	ISIN                   *string `json:"isin"`
	CounterpartyID         string  `json:"counterpartyId"`
	ManajerInvestasiID     *string `json:"manajerInvestasiId"`
	BankKustodianID        *string `json:"bankKustodianId"`
	MataUang               string  `json:"mataUang"`
	PortofolioID           string  `json:"portofolioId"`
	Nominal                string  `json:"nominal"`
	JumlahLot              *string `json:"jumlahLot"`
	TanggalPenempatan      string  `json:"tanggalPenempatan"`
	TanggalJatuhTempo      *string `json:"tanggalJatuhTempo"`
	Kupon                  *string `json:"kupon"`
	FrekuensiBunga         *string `json:"frekuensiBunga"`
	AutoRenewalFlag        bool    `json:"autoRenewalFlag"`
	FvociElection          bool    `json:"fvociElection"`
	SppiResult             *string `json:"sppiResult"`
	BmCategory             *string `json:"bmCategory"`
	KlasifikasiPsak71      *string `json:"klasifikasiPsak71"`
	KlasifikasiLockedAt    *string `json:"klasifikasiLockedAt"`
	EirAwal                *string `json:"eirAwal"`
	TanggalEirComputed     *string `json:"tanggalEirComputed"`
	PremiumDiskonto        string  `json:"premiumDiskonto"`
	BiayaTransaksi         string  `json:"biayaTransaksi"`
	EirMethodFlag          bool    `json:"eirMethodFlag"`
	DayCountConvention     string  `json:"dayCountConvention"`
	AmortizationFrequency  *string `json:"amortizationFrequency"`
	Status                 string  `json:"status"`
	WorkflowStatus         string  `json:"workflowStatus"`
	WorkflowInstanceID     *string `json:"workflowInstanceId"`
	RowVersion             int64   `json:"rowVersion"`
	CreatedAt              string  `json:"createdAt"`
	CreatedBy              string  `json:"createdBy"`
	UpdatedAt              *string `json:"updatedAt"`
	UpdatedBy              *string `json:"updatedBy"`
	DeletedAt              *string `json:"deletedAt"`
}

// ToResponse converts a domain entity to the JSON response shape.
// decimal.Decimal is serialized as string to preserve precision (avoid float rounding).
func ToResponse(m *Instrumen) Response {
	r := Response{
		ID:                    m.ID.String(),
		KodeInstrumen:         m.KodeInstrumen,
		TipeInstrumen:         m.TipeInstrumen,
		SubTipe:               m.SubTipe,
		Nama:                  m.Nama,
		ISIN:                  m.ISIN,
		CounterpartyID:        m.CounterpartyID.String(),
		MataUang:              m.MataUang,
		PortofolioID:          m.PortofolioID.String(),
		Nominal:               m.Nominal.StringFixed(4),
		TanggalPenempatan:     m.TanggalPenempatan,
		TanggalJatuhTempo:     m.TanggalJatuhTempo,
		FrekuensiBunga:        m.FrekuensiBunga,
		AutoRenewalFlag:       m.AutoRenewalFlag,
		FvociElection:         m.FvociElection,
		SppiResult:            m.SppiResult,
		BmCategory:            m.BmCategory,
		KlasifikasiPsak71:     m.KlasifikasiPsak71,
		PremiumDiskonto:       m.PremiumDiskonto.StringFixed(4),
		BiayaTransaksi:        m.BiayaTransaksi.StringFixed(4),
		EirMethodFlag:         m.EirMethodFlag,
		DayCountConvention:    m.DayCountConvention,
		AmortizationFrequency: m.AmortizationFrequency,
		Status:                m.Status,
		WorkflowStatus:        string(displayWorkflowStatus(m.WorkflowStatus)),
		RowVersion:            m.RowVersion,
		CreatedAt:             m.CreatedAt.Format(time.RFC3339),
		CreatedBy:             m.CreatedBy.String(),
	}

	if m.ManajerInvestasiID != nil {
		s := m.ManajerInvestasiID.String()
		r.ManajerInvestasiID = &s
	}
	if m.BankKustodianID != nil {
		s := m.BankKustodianID.String()
		r.BankKustodianID = &s
	}
	if m.JumlahLot != nil {
		s := m.JumlahLot.String()
		r.JumlahLot = &s
	}
	if m.Kupon != nil {
		s := m.Kupon.StringFixed(4)
		r.Kupon = &s
	}
	if m.KlasifikasiLockedAt != nil {
		s := m.KlasifikasiLockedAt.Format(time.RFC3339)
		r.KlasifikasiLockedAt = &s
	}
	if m.EirAwal != nil {
		s := m.EirAwal.StringFixed(8)
		r.EirAwal = &s
	}
	if m.TanggalEirComputed != nil {
		r.TanggalEirComputed = m.TanggalEirComputed
	}
	if m.WorkflowInstanceID != nil {
		s := m.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
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
	return r
}

// DeleteResponse is returned by the soft-delete endpoint.
type DeleteResponse struct {
	Deleted   bool   `json:"deleted"`
	DeletedAt string `json:"deletedAt"`
	EntityID  string `json:"entityId"`
}

// AuditHistoryItem represents one aud.audit_log row for instrumen.
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

// ─── List config ─────────────────────────────────────────────────────────────

// AllowedSortCols is the sort-column whitelist.
var AllowedSortCols = []string{
	"kode_instrumen", "tipe_instrumen", "nama", "status",
	"tanggal_penempatan", "tanggal_jatuh_tempo",
	"created_at", "workflow_status",
}

// AllowedFilterCols is the filter-column whitelist.
var AllowedFilterCols = []string{
	"tipe_instrumen", "status", "workflow_status",
	"mata_uang", "portofolio_id", "counterparty_id",
	"klasifikasi_psak71", "sppi_result", "bm_category",
}

// SearchCols are scanned by ?q= text search.
var SearchCols = []string{"kode_instrumen", "nama", "isin"}

// AllAllowedCols is the union for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)
