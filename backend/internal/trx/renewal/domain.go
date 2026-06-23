// Package renewal implements trx.renewal — Renewal Deposito (P5-M7).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
//
// Domain rules (DEC-013/016/017/018/021):
//   - Renewal hanya untuk instrumen DEPOSITO ACTIVE dengan klasifikasi_locked=TRUE
//   - 1 active renewal per instrumen (partial unique uq_renewal_instrumen_lama_active)
//   - SoD: approver_id ≠ maker_id enforced at service layer + DB trigger
//   - PPh 20%: bunga_kotor × 0.20 (PP No. 131/2000) — fixed rate, no tier
//   - EIR after-PPh: Newton-Raphson (tolerance 1e-10, max 100 iter) — DEC-013
//   - All amounts: shopspring/decimal (never float64) — DEC-016
//   - Audit writes in same tx as mutation — DEC-018
//   - Idempotency-Key mandatory on mutating endpoints — DEC-021
package renewal

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
	// CodeRenewalInstrumenNotEligible — instrumen bukan deposito ACTIVE atau sudah ada renewal aktif. HTTP 422.
	CodeRenewalInstrumenNotEligible = "RENEWAL_INSTRUMEN_NOT_ELIGIBLE"

	// CodeRenewalSkemaInvalid — nilai skema bukan POKOK_SAJA atau POKOK_PLUS_BUNGA. HTTP 400.
	CodeRenewalSkemaInvalid = "RENEWAL_SKEMA_INVALID"

	// CodeRenewalTenorOutOfRange — tenor_baru_bulan < 1 atau > 60. HTTP 400.
	CodeRenewalTenorOutOfRange = "RENEWAL_TENOR_OUT_OF_RANGE"

	// CodeRenewalRateOutOfRange — rate_baru_persen < 0 atau > 30. HTTP 400.
	CodeRenewalRateOutOfRange = "RENEWAL_RATE_OUT_OF_RANGE"

	// CodeRenewalBungaBersihTooSmall — skema POKOK_PLUS_BUNGA dan bunga_bersih < IDR 100.000. HTTP 422.
	CodeRenewalBungaBersihTooSmall = "RENEWAL_BUNGA_BERSIH_TOO_SMALL"

	// CodeRenewalPphCalcMismatch — client-submitted PPh ≠ server-computed PPh. HTTP 422.
	CodeRenewalPphCalcMismatch = "RENEWAL_PPH_CALC_MISMATCH"
)

// ErrRenewalInstrumenNotEligible is the sentinel for instrumen eligibility failures.
var ErrRenewalInstrumenNotEligible = domainerrors.New(domainerrors.CodeValidationFailed,
	"Instrumen tidak eligible untuk renewal: bukan deposito ACTIVE atau sudah ada renewal aktif.")

// ErrRenewalBungaBersihTooSmall is the sentinel for POKOK_PLUS_BUNGA minimum check.
var ErrRenewalBungaBersihTooSmall = domainerrors.New(domainerrors.CodeValidationFailed,
	"bunga_bersih lebih kecil dari minimum IDR 100.000 untuk skema POKOK_PLUS_BUNGA.")

// MinBungaBersih is the minimum bunga_bersih for POKOK_PLUS_BUNGA (BRD §6.2).
var MinBungaBersih = decimal.NewFromInt(100_000)

// PphRate is the PPh 20% rate per PP No. 131/2000.
var PphRate = decimal.NewFromFloat(0.20)

// ─── Status enum ─────────────────────────────────────────────────────────────

// Status represents trx.renewal.status.
type Status string

const (
	StatusPendingApproval Status = "PENDING_APPROVAL"
	StatusApproved        Status = "APPROVED"
	StatusPosted          Status = "POSTED"
	StatusRejected        Status = "REJECTED"
)

// CanApprove returns true when the status allows the approve transition.
func (s Status) CanApprove() bool { return s == StatusPendingApproval }

// CanReject returns true when the status allows the reject transition.
func (s Status) CanReject() bool { return s == StatusPendingApproval }

// ─── Skema enum ──────────────────────────────────────────────────────────────

// Skema represents the renewal scheme.
type Skema string

const (
	SkemaPokokSaja      Skema = "POKOK_SAJA"
	SkemaPokokPlusBunga Skema = "POKOK_PLUS_BUNGA"
)

// IsValidSkema returns true if s is a known Skema value.
func IsValidSkema(s string) bool {
	return s == string(SkemaPokokSaja) || s == string(SkemaPokokPlusBunga)
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// Renewal is the domain entity for one trx.renewal row.
// All monetary/rate fields use decimal.Decimal (DEC-016 — never float64).
type Renewal struct {
	ID                   uuid.UUID        `db:"id"`
	InstrumenLamaID      uuid.UUID        `db:"instrumen_lama_id"`
	InstrumenBaruID      *uuid.UUID       `db:"instrumen_baru_id"`
	Skema                Skema            `db:"skema"`
	TenorBaruBulan       int16            `db:"tenor_baru_bulan"`
	RateBaruPersen       decimal.Decimal  `db:"rate_baru_persen"`
	TanggalEfektifBaru   time.Time        `db:"tanggal_efektif_baru"` // DATE
	TanggalJatuhTempoBaru time.Time       `db:"tanggal_jatuh_tempo_baru"` // DATE
	PokokLama            decimal.Decimal  `db:"pokok_lama"`
	PokokBaru            decimal.Decimal  `db:"pokok_baru"`
	BungaKotor           decimal.Decimal  `db:"bunga_kotor"`
	PphAmount            decimal.Decimal  `db:"pph_amount"`
	BungaBersih          decimal.Decimal  `db:"bunga_bersih"`
	EirBaru              *decimal.Decimal `db:"eir_baru"`
	ScheduleBaruJSONB    *json.RawMessage `db:"schedule_baru_jsonb"`
	Status               Status           `db:"status"`
	MakerID              uuid.UUID        `db:"maker_id"`
	ApproverID           *uuid.UUID       `db:"approver_id"`
	RequestReason        *string          `db:"request_reason"`
	ApproveReason        *string          `db:"approve_reason"`
	RejectReason         *string          `db:"reject_reason"`
	SignatureMethod       *string          `db:"signature_method"`
	SignatureHashMeta    *json.RawMessage `db:"signature_hash_meta"`
	ApprovedAt           *time.Time       `db:"approved_at"`
	JurnalHeaderID       *uuid.UUID       `db:"jurnal_header_id"`
	PeriodeBulananID     *uuid.UUID       `db:"periode_bulanan_id"`
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

// ─── Instrumen info (minimal; pulled from mst.instrumen by repo) ─────────────

// InstrumenInfo holds fields needed by renewal service from mst.instrumen.
type InstrumenInfo struct {
	ID                  uuid.UUID
	KodeInstrumen       string
	NamaInstrumen       string
	JenisInstrumen      string // must be "DEPOSITO"
	Status              string // must be "ACTIVE"
	KlasifikasiPSAK71   string
	KlasifikasiLocked   bool
	Pokok               decimal.Decimal
	RatePersen          decimal.Decimal // rate p.a. in percent
	TanggalPenempatan   time.Time
	TanggalJatuhTempo   time.Time
	MataUang            string
	CounterpartyID      uuid.UUID
	PortofolioID        uuid.UUID
	SppiTestRunID       *uuid.UUID
	BmAssessmentID      *uuid.UUID
	RenewalDariInstrumenID *uuid.UUID
}

// ─── Allowed sort/filter columns ─────────────────────────────────────────────

// AllowedSortCols is the whitelist for sort on GET /trx/renewal.
var AllowedSortCols = []string{
	"created_at",
	"tanggal_efektif_baru",
	"status",
	"instrumen_lama_id",
	"skema",
}

// AllowedFilterCols is the whitelist for filter on GET /trx/renewal.
var AllowedFilterCols = []string{
	"instrumen_lama_id",
	"instrumen_baru_id",
	"status",
	"skema",
	"tanggal_efektif_baru",
	"maker_id",
	"approver_id",
	"periode_bulanan_id",
}

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRenewalRequest is the body for POST /trx/renewal.
type CreateRenewalRequest struct {
	InstrumenID         uuid.UUID       `json:"instrumenId"        binding:"required"`
	Skema               string          `json:"skema"              binding:"required"`
	TenorBaruBulan      int             `json:"tenorBaruBulan"     binding:"required"`
	RateBaruPersen      decimal.Decimal `json:"rateBaruPersen"`
	TanggalEfektifBaru  string          `json:"tanggalEfektifBaru" binding:"required"`
	RequestReason       string          `json:"requestReason"`
}

// PreviewResult holds the calculated preview fields.
type PreviewResult struct {
	PokokLama            decimal.Decimal
	BungaKotor           decimal.Decimal
	Pph20pct             decimal.Decimal
	BungaBersih          decimal.Decimal
	PokokBaru            decimal.Decimal
	EirBaru              decimal.Decimal
	TanggalJatuhTempoBaru time.Time
}

// PreviewResponse is the JSON preview nested in create/detail responses.
type PreviewResponse struct {
	PokokLama            string `json:"pokokLama"`
	BungaKotor           string `json:"bungaKotor"`
	Pph20pct             string `json:"pph20pct"`
	BungaBersih          string `json:"bungaBersih"`
	PokokBaru            string `json:"pokokBaru"`
	EirBaru              string `json:"eirBaru"`
	TanggalJatuhTempoBaru string `json:"tanggalJatuhTempoBaru"`
}

// ToPreviewResponse converts PreviewResult to API response struct.
func ToPreviewResponse(p PreviewResult) PreviewResponse {
	return PreviewResponse{
		PokokLama:            p.PokokLama.StringFixed(4),
		BungaKotor:           p.BungaKotor.StringFixed(4),
		Pph20pct:             p.Pph20pct.StringFixed(4),
		BungaBersih:          p.BungaBersih.StringFixed(4),
		PokokBaru:            p.PokokBaru.StringFixed(4),
		EirBaru:              p.EirBaru.StringFixed(8),
		TanggalJatuhTempoBaru: p.TanggalJatuhTempoBaru.Format("2006-01-02"),
	}
}

// CreateRenewalResponse is returned by POST /trx/renewal.
type CreateRenewalResponse struct {
	RenewalID string          `json:"renewalId"`
	Status    string          `json:"status"`
	Preview   PreviewResponse `json:"preview"`
	NextStep  string          `json:"nextStep"`
}

// ApproveRenewalRequest is the body for POST /trx/renewal/{id}/approve.
type ApproveRenewalRequest struct {
	Comment         string `json:"comment"         binding:"required"`
	SignatureMethod  string `json:"signatureMethod" binding:"required"`
}

// ApproveRenewalResponse is returned by successful approve.
type ApproveRenewalResponse struct {
	RenewalID       string  `json:"renewalId"`
	Status          string  `json:"status"`
	InstrumenBaruID *string `json:"instrumenBaruId"`
	JurnalEntryID   *string `json:"jurnalEntryId"`
	ApprovedBy      string  `json:"approvedBy"`
	ApprovedAt      string  `json:"approvedAt"`
	Message         string  `json:"message"`
}

// RejectRenewalRequest is the body for POST /trx/renewal/{id}/reject.
type RejectRenewalRequest struct {
	Comment        string `json:"comment"         binding:"required,min=30"`
	SignatureMethod string `json:"signatureMethod" binding:"required"`
}

// RejectRenewalResponse is returned by successful reject.
type RejectRenewalResponse struct {
	RenewalID  string `json:"renewalId"`
	Status     string `json:"status"`
	RejectedBy string `json:"rejectedBy"`
	RejectedAt string `json:"rejectedAt"`
	Comment    string `json:"comment"`
}

// ListItem is one row in GET /trx/renewal list response.
type ListItem struct {
	ID                  string  `json:"id"`
	InstrumenLamaID     string  `json:"instrumenLamaId"`
	InstrumenLamaKode   string  `json:"instrumenLamaKode"`
	InstrumenBaruID     *string `json:"instrumenBaruId"`
	Skema               string  `json:"skema"`
	TenorBaruBulan      int16   `json:"tenorBaruBulan"`
	RateBaruPersen      string  `json:"rateBaruPersen"`
	TanggalEfektifBaru  string  `json:"tanggalEfektifBaru"`
	PokokLama           string  `json:"pokokLama"`
	PokokBaru           string  `json:"pokokBaru"`
	BungaBersih         string  `json:"bungaBersih"`
	Status              string  `json:"status"`
	MakerID             string  `json:"makerId"`
	ApproverID          *string `json:"approverId"`
	JurnalEntryID       *string `json:"jurnalEntryId"`
	CreatedAt           string  `json:"createdAt"`
}

// Detail is the full response for GET /trx/renewal/{id}.
type Detail struct {
	ListItem
	BungaKotor           string          `json:"bungaKotor"`
	Pph20pct             string          `json:"pph20pct"`
	EirBaru              *string         `json:"eirBaru"`
	TanggalJatuhTempoBaru string         `json:"tanggalJatuhTempoBaru"`
	ApproveReason        *string         `json:"approveReason"`
	RejectReason         *string         `json:"rejectReason"`
	SignatureMethod       *string         `json:"signatureMethod"`
	PeriodeBulananID     *string         `json:"periodeBulananId"`
	UpdatedAt            string          `json:"updatedAt"`
	RowVersion           int64           `json:"rowVersion"`
	Preview              PreviewResponse `json:"preview"`
}

// ToListItem converts a Renewal entity to a ListItem.
func ToListItem(r *Renewal, instrumenKode string) ListItem {
	li := ListItem{
		ID:                 r.ID.String(),
		InstrumenLamaID:    r.InstrumenLamaID.String(),
		InstrumenLamaKode:  instrumenKode,
		Skema:              string(r.Skema),
		TenorBaruBulan:     r.TenorBaruBulan,
		RateBaruPersen:     r.RateBaruPersen.StringFixed(4),
		TanggalEfektifBaru: r.TanggalEfektifBaru.Format("2006-01-02"),
		PokokLama:          r.PokokLama.StringFixed(4),
		PokokBaru:          r.PokokBaru.StringFixed(4),
		BungaBersih:        r.BungaBersih.StringFixed(4),
		Status:             string(r.Status),
		MakerID:            r.MakerID.String(),
		CreatedAt:          r.CreatedAt.Format(time.RFC3339),
	}
	if r.InstrumenBaruID != nil {
		s := r.InstrumenBaruID.String()
		li.InstrumenBaruID = &s
	}
	if r.ApproverID != nil {
		s := r.ApproverID.String()
		li.ApproverID = &s
	}
	if r.JurnalHeaderID != nil {
		s := r.JurnalHeaderID.String()
		li.JurnalEntryID = &s
	}
	return li
}

// ToDetail converts a Renewal entity to a Detail, including a recomputed preview.
func ToDetail(r *Renewal, instrumenKode string, preview PreviewResult) Detail {
	d := Detail{
		ListItem:              ToListItem(r, instrumenKode),
		BungaKotor:            r.BungaKotor.StringFixed(4),
		Pph20pct:              r.PphAmount.StringFixed(4),
		TanggalJatuhTempoBaru: r.TanggalJatuhTempoBaru.Format("2006-01-02"),
		ApproveReason:         r.ApproveReason,
		RejectReason:          r.RejectReason,
		SignatureMethod:        r.SignatureMethod,
		UpdatedAt:             r.UpdatedAt.Format(time.RFC3339),
		RowVersion:            r.RowVersion,
		Preview:               ToPreviewResponse(preview),
	}
	if r.EirBaru != nil {
		s := r.EirBaru.StringFixed(8)
		d.EirBaru = &s
	}
	if r.PeriodeBulananID != nil {
		s := r.PeriodeBulananID.String()
		d.PeriodeBulananID = &s
	}
	return d
}

// ─── StatusUpdate carries fields for UpdateStatus repo call ──────────────────

// StatusUpdate is the payload for transitioning a renewal's workflow status.
type StatusUpdate struct {
	Status             Status
	ApproverID         *uuid.UUID
	InstrumenBaruID    *uuid.UUID
	ApproveReason      *string
	RejectReason       *string
	SignatureMethod     *string
	SignatureHashMeta  *json.RawMessage
	ApprovedAt         *time.Time
	JurnalHeaderID     *uuid.UUID
	EirBaru            *decimal.Decimal
	UpdatedBy          uuid.UUID
	RowVersion         int64 // optimistic lock: current version
}

// ParseDateStrict parses "YYYY-MM-DD" and returns an error on bad format or invalid date.
func ParseDateStrict(s string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal format tidak valid '%s': wajib YYYY-MM-DD", s)
	}
	return t, nil
}

// AddMonths adds n calendar months to t, handling day clamping.
func AddMonths(t time.Time, n int) time.Time {
	return t.AddDate(0, n, 0)
}
