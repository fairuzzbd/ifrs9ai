// Package penjualan implements trx.penjualan — Penjualan/Pencairan Instrumen (P5-M8).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
//
// Domain rules (DEC-013/016/017/018/021):
//   - Penjualan hanya untuk instrumen ACTIVE dengan klasifikasi_locked=TRUE
//   - 1 active disposal per instrumen (partial unique uq_penjualan_instrumen_active)
//   - SoD: approver_id ≠ maker_id enforced at service layer + DB trigger
//   - OCI recycling: FVOCI debt → REKLAS_OCI_PL; FVOCI_ELECTION → no recycling (§B5.7.1)
//   - BM frequency: HTC portofolio rolling 12-month disposal check
//   - All amounts: shopspring/decimal (never float64) — DEC-016
//   - Audit writes in same tx as mutation — DEC-018
//   - Idempotency-Key mandatory on mutating endpoints — DEC-021
package penjualan

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ─────────────────────────────────────────────────────────────

const (
	// CodePenjualanInstrumenNotActive — instrumen tidak ACTIVE atau klasifikasi_locked=FALSE. HTTP 422.
	CodePenjualanInstrumenNotActive = "PENJUALAN_INSTRUMEN_NOT_ACTIVE"

	// CodePenjualanQtyExceedsHolding — qty_terjual > qty_holding atau PARTIAL qty = holding. HTTP 422.
	CodePenjualanQtyExceedsHolding = "PENJUALAN_QTY_EXCEEDS_HOLDING"

	// CodePenjualanKlasifikasiNotLocked — klasifikasi_locked=FALSE saat routing. HTTP 422.
	CodePenjualanKlasifikasiNotLocked = "PENJUALAN_KLASIFIKASI_NOT_LOCKED"

	// CodePenjualanHargaInvalid — harga_jual_per_unit ≤ 0. HTTP 400.
	CodePenjualanHargaInvalid = "PENJUALAN_HARGA_INVALID"

	// CodePenjualanPeriodeLocked — periode_buku.status_periode != 'OPEN'. HTTP 423.
	CodePenjualanPeriodeLocked = "PENJUALAN_PERIODE_LOCKED"

	// CodePenjualanBMViolationBlock — cumulative disposal > block_threshold; → PENDING_BM_REVIEW. HTTP 422.
	CodePenjualanBMViolationBlock = "PENJUALAN_BM_VIOLATION_BLOCK"

	// CodePenjualanFVOCIElectionNoRecyclingWarn — warning embedded in 200 response body. Not an error.
	CodePenjualanFVOCIElectionNoRecyclingWarn = "PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN"
)

// ─── Status enum ─────────────────────────────────────────────────────────────

// Status represents trx.penjualan.status.
type Status string

const (
	StatusPendingApproval Status = "PENDING_APPROVAL"
	StatusApproved        Status = "APPROVED"
	StatusPosted          Status = "POSTED"
	StatusRejected        Status = "REJECTED"
	StatusPendingBMReview Status = "PENDING_BM_REVIEW"
)

// CanApprove returns true when the status allows the approve transition.
func (s Status) CanApprove() bool { return s == StatusPendingApproval }

// CanReject returns true when the status allows the reject transition.
func (s Status) CanReject() bool { return s == StatusPendingApproval }

// ─── Disposal type enum ───────────────────────────────────────────────────────

// DisposalType represents PARTIAL or FULL disposal.
type DisposalType string

const (
	DisposalPartial DisposalType = "PARTIAL"
	DisposalFull    DisposalType = "FULL"
)

// IsValidDisposalType returns true if d is a known DisposalType value.
func IsValidDisposalType(d string) bool {
	return d == string(DisposalPartial) || d == string(DisposalFull)
}

// ─── Klasifikasi PSAK71 enum (snapshot values) ───────────────────────────────

// KlasifikasiPSAK71 represents the classification used at disposal time.
type KlasifikasiPSAK71 string

const (
	KlasifikasiAC           KlasifikasiPSAK71 = "AC"
	KlasifikasiFVOCI        KlasifikasiPSAK71 = "FVOCI"
	KlasifikasiFVOCIElection KlasifikasiPSAK71 = "FVOCI_ELECTION"
	KlasifikasiFVTPL        KlasifikasiPSAK71 = "FVTPL"
	KlasifikasiPOCI         KlasifikasiPSAK71 = "POCI"
)

// IsValidKlasifikasi returns true if k is a recognized classification.
func IsValidKlasifikasi(k string) bool {
	switch KlasifikasiPSAK71(k) {
	case KlasifikasiAC, KlasifikasiFVOCI, KlasifikasiFVOCIElection, KlasifikasiFVTPL, KlasifikasiPOCI:
		return true
	}
	return false
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// Penjualan is the domain entity for one trx.penjualan row.
// All monetary/rate fields use decimal.Decimal (DEC-016 — never float64).
type Penjualan struct {
	ID                  uuid.UUID        `db:"id"`
	InstrumenID         uuid.UUID        `db:"instrumen_id"`
	JenisDisposal       DisposalType     `db:"jenis_disposal"`
	QtyTerjual          decimal.Decimal  `db:"qty_terjual"`
	QtyHoldingPre       decimal.Decimal  `db:"qty_holding_pre"`
	QtyHoldingPost      *decimal.Decimal `db:"qty_holding_post"`
	HargaJualPerUnit    decimal.Decimal  `db:"harga_jual_per_unit"`
	Proceed             decimal.Decimal  `db:"proceed"`
	CostBasis           decimal.Decimal  `db:"cost_basis"`
	RealizedGL          decimal.Decimal  `db:"realized_gl"`
	OCIRecycled         *decimal.Decimal `db:"oci_recycled"`
	OCICumulativeTotal  *decimal.Decimal `db:"oci_cumulative_total"`
	KlasifikasiSnapshot KlasifikasiPSAK71 `db:"klasifikasi_snapshot"`
	JurnalEventCode     *string          `db:"jurnal_event_code"`
	TanggalEksekusi     time.Time        `db:"tanggal_eksekusi"` // DATE
	BMViolationRisk     bool             `db:"bm_violation_risk"`
	BMViolationPct      *decimal.Decimal `db:"bm_violation_pct"`
	Status              Status           `db:"status"`
	MakerID             uuid.UUID        `db:"maker_id"`
	ApproverID          *uuid.UUID       `db:"approver_id"`
	ApproveComment      *string          `db:"approve_comment"`
	RejectReason        *string          `db:"reject_reason"`
	SignatureMethod      *string          `db:"signature_method"`
	SignatureHashMeta   *json.RawMessage `db:"signature_hash_meta"`
	ApprovedAt          *time.Time       `db:"approved_at"`
	JurnalHeaderID      *uuid.UUID       `db:"jurnal_header_id"`
	PeriodeBulananID    *uuid.UUID       `db:"periode_bulanan_id"`
	InstrumenStatusAfter *string         `db:"instrumen_status_after"`
	// Standard audit columns
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  time.Time  `db:"updated_at"`
	UpdatedBy  uuid.UUID  `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`
}

// ─── Instrumen info ───────────────────────────────────────────────────────────

// InstrumenInfo holds fields needed by penjualan service from mst.instrumen.
type InstrumenInfo struct {
	ID                  uuid.UUID
	KodeInstrumen       string
	NamaInstrumen       string
	Status              string // must be "ACTIVE"
	KlasifikasiPSAK71   string
	KlasifikasiLocked   bool
	QtyHolding          decimal.Decimal
	HargaPerolehan      decimal.Decimal // original acquisition cost (for FVOCI_ELECTION cost_basis)
	PortofolioID        uuid.UUID
	BusinessModel       string // "HTC" | "HTC&S" | "Other"
	MataUang            string
	CounterpartyID      uuid.UUID
	SppiTestRunID       *uuid.UUID
	BmAssessmentID      *uuid.UUID
}

// ─── Allowed sort/filter columns ─────────────────────────────────────────────

// AllowedSortCols is the whitelist for sort on GET /trx/penjualan.
var AllowedSortCols = []string{
	"created_at",
	"tanggal_eksekusi",
	"status",
	"instrumen_id",
	"jenis_disposal",
	"klasifikasi_snapshot",
}

// AllowedFilterCols is the whitelist for filter on GET /trx/penjualan.
var AllowedFilterCols = []string{
	"instrumen_id",
	"status",
	"jenis_disposal",
	"tanggal_eksekusi",
	"maker_id",
	"approver_id",
	"klasifikasi_snapshot",
	"bm_violation_risk",
	"periode_bulanan_id",
}

// ─── Request / Response types ─────────────────────────────────────────────────

// CreatePenjualanRequest is the body for POST /trx/penjualan.
type CreatePenjualanRequest struct {
	InstrumenID      uuid.UUID       `json:"instrumenId"       binding:"required"`
	JenisDisposal    string          `json:"jenisDisposal"     binding:"required"`
	QtyTerjual       decimal.Decimal `json:"qtyTerjual"`
	HargaJualPerUnit decimal.Decimal `json:"hargaJualPerUnit"`
	TanggalEksekusi  string          `json:"tanggalEksekusi"   binding:"required"`
}

// PreviewResult holds computed preview fields.
type PreviewResult struct {
	KlasifikasiPSAK71 KlasifikasiPSAK71
	ProceedIDR        decimal.Decimal
	CostBasis         decimal.Decimal
	RealizedGL        decimal.Decimal
	OCIRecycled       *decimal.Decimal // nil for non-FVOCI-debt
	NoRecyclingNote   *string          // non-nil for FVOCI_ELECTION
	BMFreqImpactPct   *decimal.Decimal // nil if non-HTC
	BMFreqWarning     *string          // non-nil if approaching warn threshold
}

// PreviewResponse is the JSON preview nested in create/detail responses.
type PreviewResponse struct {
	KlasifikasiPsak71 string  `json:"klasifikasiPsak71"`
	ProceedIdr        string  `json:"proceedIdr"`
	CostBasis         string  `json:"costBasis"`
	RealizedGl        string  `json:"realizedGl"`
	OciRecycled       *string `json:"ociRecycled"`
	NoRecyclingNote   *string `json:"noRecyclingNote"`
	BmFreqImpactPct   *string `json:"bmFreqImpactPct"`
	BmFreqWarning     *string `json:"bmFreqWarning"`
}

// ToPreviewResponse converts PreviewResult to API response struct.
func ToPreviewResponse(p PreviewResult) PreviewResponse {
	resp := PreviewResponse{
		KlasifikasiPsak71: string(p.KlasifikasiPSAK71),
		ProceedIdr:        p.ProceedIDR.StringFixed(4),
		CostBasis:         p.CostBasis.StringFixed(4),
		RealizedGl:        p.RealizedGL.StringFixed(4),
		NoRecyclingNote:   p.NoRecyclingNote,
		BmFreqWarning:     p.BMFreqWarning,
	}
	if p.OCIRecycled != nil {
		s := p.OCIRecycled.StringFixed(4)
		resp.OciRecycled = &s
	}
	if p.BMFreqImpactPct != nil {
		s := p.BMFreqImpactPct.StringFixed(4)
		resp.BmFreqImpactPct = &s
	}
	return resp
}

// CreatePenjualanResponse is returned by POST /trx/penjualan.
type CreatePenjualanResponse struct {
	PenjualanID string          `json:"penjualanId"`
	Status      string          `json:"status"`
	Preview     PreviewResponse `json:"preview"`
	NextStep    string          `json:"nextStep"`
}

// ApprovePenjualanRequest is the body for POST /trx/penjualan/{id}/approve.
type ApprovePenjualanRequest struct {
	Comment         string `json:"comment"         binding:"required"`
	SignatureMethod  string `json:"signatureMethod" binding:"required"`
}

// ApprovePenjualanResponse is returned by successful approve.
type ApprovePenjualanResponse struct {
	PenjualanID          string   `json:"penjualanId"`
	Status               string   `json:"status"`
	JurnalEntryID        *string  `json:"jurnalEntryId"`
	InstrumenStatusAfter *string  `json:"instrumenStatusAfter"`
	ApprovedBy           string   `json:"approvedBy"`
	ApprovedAt           string   `json:"approvedAt"`
	OCIRecycled          *string  `json:"ociRecycled"`
	NoRecyclingNote      *string  `json:"noRecyclingNote"`
	BMViolationRisk      bool     `json:"bmViolationRisk"`
	Warnings             []string `json:"warnings"`
}

// RejectPenjualanRequest is the body for POST /trx/penjualan/{id}/reject.
type RejectPenjualanRequest struct {
	Reason         string `json:"reason"          binding:"required,min=30"`
	SignatureMethod string `json:"signatureMethod" binding:"required"`
}

// RejectPenjualanResponse is returned by successful reject.
type RejectPenjualanResponse struct {
	PenjualanID string `json:"penjualanId"`
	Status      string `json:"status"`
	RejectedBy  string `json:"rejectedBy"`
	RejectedAt  string `json:"rejectedAt"`
	Reason      string `json:"reason"`
}

// ListItem is one row in GET /trx/penjualan list response.
type ListItem struct {
	ID                  string  `json:"id"`
	InstrumenID         string  `json:"instrumenId"`
	InstrumenKode       string  `json:"instrumenKode"`
	JenisDisposal       string  `json:"jenisDisposal"`
	QtyTerjual          string  `json:"qtyTerjual"`
	QtyHoldingPre       string  `json:"qtyHoldingPre"`
	QtyHoldingPost      *string `json:"qtyHoldingPost"`
	ProceedIdr          string  `json:"proceedIdr"`
	RealizedGl          *string `json:"realizedGl"`
	KlasifikasiSnapshot string  `json:"klasifikasiSnapshot"`
	Status              string  `json:"status"`
	TanggalEksekusi     string  `json:"tanggalEksekusi"`
	MakerID             string  `json:"makerId"`
	ApproverID          *string `json:"approverId"`
	JurnalHeaderID      *string `json:"jurnalHeaderId"`
	BMViolationRisk     bool    `json:"bmViolationRisk"`
	CreatedAt           string  `json:"createdAt"`
}

// Detail is the full response for GET /trx/penjualan/{id}.
type Detail struct {
	ListItem
	CostBasis           string          `json:"costBasis"`
	OciRecycled         *string         `json:"ociRecycled"`
	OciCumulativeTotal  *string         `json:"ociCumulativeTotal"`
	NoRecyclingNote     *string         `json:"noRecyclingNote"`
	JurnalEventCode     *string         `json:"jurnalEventCode"`
	ApproveComment      *string         `json:"approveComment"`
	RejectReason        *string         `json:"rejectReason"`
	SignatureMethod      *string         `json:"signatureMethod"`
	PeriodeBulananID    *string         `json:"periodeBulananId"`
	BMViolationPct      *string         `json:"bmViolationPct"`
	UpdatedAt           string          `json:"updatedAt"`
	RowVersion          int64           `json:"rowVersion"`
	Preview             PreviewResponse `json:"preview"`
}

// BMAlertItem is one row in GET /trx/penjualan/bm-frequency-alerts.
type BMAlertItem struct {
	InstrumenID          string `json:"instrumenId"`
	InstrumenKode        string `json:"instrumenKode"`
	PortofolioID         string `json:"portofolioId"`
	PortofolioNama       string `json:"portofolioNama"`
	CumulativeSold12mPct string `json:"cumulativeSold12mPct"`
	WarnThresholdPct     string `json:"warnThresholdPct"`
	BlockThresholdPct    string `json:"blockThresholdPct"`
	FlagStatus           string `json:"flagStatus"`
	LastUpdated          string `json:"lastUpdated"`
}

// ToListItem converts a Penjualan entity to a ListItem.
func ToListItem(p *Penjualan, instrumenKode string) ListItem {
	li := ListItem{
		ID:                  p.ID.String(),
		InstrumenID:         p.InstrumenID.String(),
		InstrumenKode:       instrumenKode,
		JenisDisposal:       string(p.JenisDisposal),
		QtyTerjual:          p.QtyTerjual.StringFixed(8),
		QtyHoldingPre:       p.QtyHoldingPre.StringFixed(8),
		ProceedIdr:          p.Proceed.StringFixed(4),
		KlasifikasiSnapshot: string(p.KlasifikasiSnapshot),
		Status:              string(p.Status),
		TanggalEksekusi:     p.TanggalEksekusi.Format("2006-01-02"),
		MakerID:             p.MakerID.String(),
		BMViolationRisk:     p.BMViolationRisk,
		CreatedAt:           p.CreatedAt.Format(time.RFC3339),
	}
	if p.QtyHoldingPost != nil {
		s := p.QtyHoldingPost.StringFixed(8)
		li.QtyHoldingPost = &s
	}
	rlStr := p.RealizedGL.StringFixed(4)
	li.RealizedGl = &rlStr
	if p.ApproverID != nil {
		s := p.ApproverID.String()
		li.ApproverID = &s
	}
	if p.JurnalHeaderID != nil {
		s := p.JurnalHeaderID.String()
		li.JurnalHeaderID = &s
	}
	return li
}

// ToDetail converts a Penjualan entity to Detail including a recomputed preview.
func ToDetail(p *Penjualan, instrumenKode string, preview PreviewResult) Detail {
	d := Detail{
		ListItem:     ToListItem(p, instrumenKode),
		CostBasis:    p.CostBasis.StringFixed(4),
		JurnalEventCode: p.JurnalEventCode,
		ApproveComment:  p.ApproveComment,
		RejectReason:    p.RejectReason,
		SignatureMethod:  p.SignatureMethod,
		UpdatedAt:    p.UpdatedAt.Format(time.RFC3339),
		RowVersion:   p.RowVersion,
		Preview:      ToPreviewResponse(preview),
	}
	if p.OCIRecycled != nil {
		s := p.OCIRecycled.StringFixed(4)
		d.OciRecycled = &s
	}
	if p.OCICumulativeTotal != nil {
		s := p.OCICumulativeTotal.StringFixed(4)
		d.OciCumulativeTotal = &s
	}
	if p.PeriodeBulananID != nil {
		s := p.PeriodeBulananID.String()
		d.PeriodeBulananID = &s
	}
	if p.BMViolationPct != nil {
		s := p.BMViolationPct.StringFixed(4)
		d.BMViolationPct = &s
	}
	return d
}

// StatusUpdate carries fields for UpdateStatus repo call.
type StatusUpdate struct {
	Status               Status
	ApproverID           *uuid.UUID
	ApproveComment       *string
	RejectReason         *string
	SignatureMethod       *string
	SignatureHashMeta     *json.RawMessage
	ApprovedAt           *time.Time
	JurnalHeaderID       *uuid.UUID
	QtyHoldingPost       *decimal.Decimal
	OCIRecycled          *decimal.Decimal
	BMViolationRisk      bool
	BMViolationPct       *decimal.Decimal
	JurnalEventCode      *string
	InstrumenStatusAfter *string
	UpdatedBy            uuid.UUID
	RowVersion           int64 // current version for optimistic lock
}

// ParseDateStrict parses "YYYY-MM-DD" and returns an error on bad format or invalid date.
func ParseDateStrict(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal format tidak valid '%s': wajib YYYY-MM-DD", s)
	}
	return t, nil
}

// NoRecyclingNoteText returns the standard FVOCI_ELECTION no-recycling note text.
func NoRecyclingNoteText(glAmount decimal.Decimal) string {
	return fmt.Sprintf("Gain/loss IDR %s tetap di OCI per PSAK 71 §B5.7.1. Tidak direkognisi di P&L.",
		glAmount.StringFixed(0))
}

// ErrPenjualanInstrumenNotActive is the sentinel for instrumen eligibility failures.
var ErrPenjualanInstrumenNotActive = domainerrors.New(domainerrors.CodeValidationFailed,
	"Instrumen tidak eligible untuk penjualan: tidak ACTIVE atau klasifikasi belum locked.")
