// Package counterparty implements the mst.counterparty master-data module (APP-A-MSTR-003).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
//
// PII handling (DEC-028):
//   - npwp, nomor_rekening, ktp stored encrypted via sec.encrypt() PostgreSQL function.
//   - Read default: masked (last 4 chars only).
//   - Full decrypt: GET /:id/pii — requires counterparty.view_pii permission + audit.
//   - NEVER plaintext PII in logs or audit before/after JSON.
//
// Schema drift note (migration 0015):
//   - Table uses legacy `version INT` and `is_deleted BOOLEAN` from 0001.
//   - Migration 0015 added `row_version`, `deleted_at`, `deleted_by`, `workflow_status`.
//   - Service always sets BOTH is_deleted=TRUE AND deleted_at=now() on soft-delete.
//   - row_version is authoritative for optimistic lock (version is legacy).
package counterparty

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeCounterpartyDuplicateKode returned when kode_counterparty already exists.
	CodeCounterpartyDuplicateKode = string(domainerrors.CodeConflict)

	// CodeCounterpartyInvalidTipe returned when tipe value not in whitelist.
	CodeCounterpartyInvalidTipe = string(domainerrors.CodeValidationFailed)

	// CodeCounterpartyPIIDecryptFailed returned when PII decryption fails.
	CodeCounterpartyPIIDecryptFailed = string(domainerrors.CodeInternal)
)

// ─── Enums ────────────────────────────────────────────────────────────────────

// WorkflowStatus mirrors workflow_status column values.
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

// IsEditable returns true when a record can be data-edited (not yet in review flow).
func (s WorkflowStatus) IsEditable() bool {
	return s == WorkflowStatusDraft || s == WorkflowStatusRejected || s == WorkflowStatusReturned
}

// CounterpartyTipe is the tipe whitelist per migration 0015.
type CounterpartyTipe string

const (
	TipeBank          CounterpartyTipe = "BANK"
	TipeBankKustodian CounterpartyTipe = "BANK_KUSTODIAN"
	TipeKorporasi     CounterpartyTipe = "KORPORASI"
	TipePemerintah    CounterpartyTipe = "PEMERINTAH"
	TipeManajerInv    CounterpartyTipe = "MANAJER_INVESTASI"
	TipeEmiten        CounterpartyTipe = "EMITEN_SAHAM"
	TipeMultilateral  CounterpartyTipe = "MULTILATERAL"
	TipeKorporasiBumn CounterpartyTipe = "KORPORASI_BUMN"
	TipeIndividu      CounterpartyTipe = "INDIVIDU"
	TipeReasuradur    CounterpartyTipe = "REASURADUR"
)

// validTipes is the set of allowed tipe values (migration 0015 CHECK constraint).
var validTipes = map[CounterpartyTipe]bool{
	TipeBank: true, TipeBankKustodian: true, TipeKorporasi: true,
	TipePemerintah: true, TipeManajerInv: true, TipeEmiten: true,
	TipeMultilateral: true, TipeKorporasiBumn: true, TipeIndividu: true,
	TipeReasuradur: true,
}

// IsValidTipe returns true if t is in the whitelist.
func IsValidTipe(t string) bool {
	return validTipes[CounterpartyTipe(t)]
}

// CounterpartyStatus is the status column whitelist.
type CounterpartyStatus string

const (
	StatusAktif     CounterpartyStatus = "ACTIVE"
	StatusInaktif   CounterpartyStatus = "INACTIVE"
	StatusSuspended CounterpartyStatus = "SUSPENDED"
)

// validStatuses maps allowed status values.
var validStatuses = map[CounterpartyStatus]bool{
	StatusAktif: true, StatusInaktif: true, StatusSuspended: true,
}

// TipeEksposurBasel is the tipe_eksposur_basel whitelist (maps to LGD pool).
type TipeEksposurBasel string

const (
	EksposurSovereign       TipeEksposurBasel = "SOVEREIGN"
	EksposurSeniorSecured   TipeEksposurBasel = "SENIOR_SECURED"
	EksposurSeniorUnsecured TipeEksposurBasel = "SENIOR_UNSECURED"
	EksposurSubordinated    TipeEksposurBasel = "SUBORDINATED"
	EksposurCorporate       TipeEksposurBasel = "CORPORATE"
	EksposurBank            TipeEksposurBasel = "BANK"
	EksposurRetail          TipeEksposurBasel = "RETAIL"
)

// validEksposurBasel is the whitelist for tipe_eksposur_basel.
var validEksposurBasel = map[TipeEksposurBasel]bool{
	EksposurSovereign: true, EksposurSeniorSecured: true, EksposurSeniorUnsecured: true,
	EksposurSubordinated: true, EksposurCorporate: true, EksposurBank: true, EksposurRetail: true,
}

// IsValidEksposurBasel returns true if t is in the whitelist.
func IsValidEksposurBasel(t string) bool {
	return validEksposurBasel[TipeEksposurBasel(t)]
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// Counterparty is the domain entity for mst.counterparty.
// PII fields (NPWP, NomorRekening, KTP) are always nil in this struct;
// they are populated only by PIIResult from the view_pii endpoint.
type Counterparty struct {
	ID               uuid.UUID `db:"id"`
	KodeCounterparty string    `db:"kode_counterparty"`

	// Core fields
	Nama                 string             `db:"nama"`
	Tipe                 CounterpartyTipe   `db:"tipe"`
	RatingPefindoCurrent *string            `db:"rating_pefindo_current"`
	TipeEksposurBasel    TipeEksposurBasel  `db:"tipe_eksposur_basel"`
	EligibleLpsFlag      bool               `db:"eligible_lps_flag"`
	NomorIzinOjk         *string            `db:"nomor_izin_ojk"`
	TanggalIzinOjk       *string            `db:"tanggal_izin_ojk"` // DATE "YYYY-MM-DD"
	AumTerakhir          *decimal.Decimal   `db:"aum_terakhir"`
	TanggalAumTerakhir   *string            `db:"tanggal_aum_terakhir"` // DATE
	KategoriMi           *string            `db:"kategori_mi"`
	Status               CounterpartyStatus `db:"status"`

	// Workflow
	WorkflowStatus     WorkflowStatus `db:"workflow_status"`
	WorkflowInstanceID *uuid.UUID     `db:"workflow_instance_id"` // nil before first submit

	// Audit columns (0015 additions + 0001 legacy coexist)
	CreatedAt  time.Time  `db:"created_at"`
	CreatedBy  uuid.UUID  `db:"created_by"`
	UpdatedAt  *time.Time `db:"updated_at"`
	UpdatedBy  *uuid.UUID `db:"updated_by"`
	DeletedAt  *time.Time `db:"deleted_at"`
	DeletedBy  *uuid.UUID `db:"deleted_by"`
	RowVersion int64      `db:"row_version"`
	TenantID   string     `db:"tenant_id"`

	// Legacy drift columns (0001) — kept in sync on write
	Version   int  `db:"version"`
	IsDeleted bool `db:"is_deleted"`
}

// PIIFields holds decrypted PII data — only returned by view_pii endpoint.
// NEVER log or return via list/default get endpoint.
type PIIFields struct {
	NPWP          *string `json:"npwp"`
	NomorRekening *string `json:"nomorRekening"`
	KTP           *string `json:"ktp"`
}

// MaskedPII returns the masked representation of PII fields.
// Format: last 4 chars visible, rest replaced by ***
// Example: "***1234" for any string with >=4 chars.
// If string < 4 chars: fully masked as "***".
func MaskString(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	if len(v) == 0 {
		return s
	}
	if len(v) < 4 {
		masked := "***"
		return &masked
	}
	masked := "***" + v[len(v)-4:]
	return &masked
}

// ─── Allowed columns ──────────────────────────────────────────────────────────

// AllowedSortCols for list endpoint.
var AllowedSortCols = []string{
	"kode_counterparty", "nama", "tipe", "tipe_eksposur_basel",
	"status", "rating_pefindo_current", "created_at", "workflow_status",
}

// AllowedFilterCols for list endpoint.
var AllowedFilterCols = []string{
	"tipe", "tipe_eksposur_basel", "status", "eligible_lps_flag",
	"workflow_status", "kode_counterparty",
}

// SearchCols scanned by ?q= text search.
var SearchCols = []string{"kode_counterparty", "nama", "nomor_izin_ojk"}

// AllAllowedCols is the union for listquery.ParseFromRequest.
var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST /master/counterparty body.
// PII fields (npwp, nomorRekening, ktp) accepted in plaintext — service encrypts.
type CreateRequest struct {
	KodeCounterparty  string  `json:"kodeCounterparty"  binding:"required,min=2,max=20"`
	Nama              string  `json:"nama"              binding:"required,min=3,max=200"`
	Tipe              string  `json:"tipe"              binding:"required"`
	TipeEksposurBasel string  `json:"tipeEksposurBasel" binding:"required"`
	EligibleLpsFlag   bool    `json:"eligibleLpsFlag"`
	NomorIzinOjk      *string `json:"nomorIzinOjk"`
	TanggalIzinOjk    *string `json:"tanggalIzinOjk"`
	AumTerakhir       *string `json:"aumTerakhir"` // decimal string
	TanggalAumTerakhir *string `json:"tanggalAumTerakhir"`
	KategoriMi        *string `json:"kategoriMi"`
	Status            string  `json:"status"` // default ACTIVE
	// PII — plaintext at request boundary; encrypted before storage
	NPWP          *string `json:"npwp"`
	NomorRekening *string `json:"nomorRekening"`
	KTP           *string `json:"ktp"`
}

// UpdateRequest is the PATCH /master/counterparty/:id body.
type UpdateRequest struct {
	Nama              *string `json:"nama"              binding:"omitempty,min=3,max=200"`
	Tipe              *string `json:"tipe"`
	TipeEksposurBasel *string `json:"tipeEksposurBasel"`
	EligibleLpsFlag   *bool   `json:"eligibleLpsFlag"`
	NomorIzinOjk      *string `json:"nomorIzinOjk"`
	TanggalIzinOjk    *string `json:"tanggalIzinOjk"`
	AumTerakhir       *string `json:"aumTerakhir"`
	TanggalAumTerakhir *string `json:"tanggalAumTerakhir"`
	KategoriMi        *string `json:"kategoriMi"`
	Status            *string `json:"status"`
	// PII update — nil = do not change
	NPWP          *string `json:"npwp"`
	NomorRekening *string `json:"nomorRekening"`
	KTP           *string `json:"ktp"`
	RowVersion    int64   `json:"rowVersion" binding:"required"`
}

// Response is the default JSON representation (no PII decrypted).
// PII is masked to last 4 chars.
type Response struct {
	ID               string  `json:"id"`
	KodeCounterparty string  `json:"kodeCounterparty"`
	Nama             string  `json:"nama"`
	Tipe             string  `json:"tipe"`
	RatingPefindoCurrent *string `json:"ratingPefindoCurrent"`
	TipeEksposurBasel string  `json:"tipeEksposurBasel"`
	EligibleLpsFlag  bool    `json:"eligibleLpsFlag"`
	// PII masked
	NPWP          *string `json:"npwp"`
	NomorRekening *string `json:"nomorRekening"`
	KTP           *string `json:"ktp"`
	// Non-PII
	NomorIzinOjk       *string `json:"nomorIzinOjk"`
	TanggalIzinOjk     *string `json:"tanggalIzinOjk"`
	AumTerakhir        *string `json:"aumTerakhir"`
	TanggalAumTerakhir *string `json:"tanggalAumTerakhir"`
	KategoriMi         *string `json:"kategoriMi"`
	Status             string  `json:"status"`
	WorkflowStatus     string  `json:"workflowStatus"`
	WorkflowInstanceID *string `json:"workflowInstanceId"`
	RowVersion         int64   `json:"rowVersion"`
	CreatedAt          string  `json:"createdAt"`
	CreatedBy          string  `json:"createdBy"`
	UpdatedAt          *string `json:"updatedAt"`
	UpdatedBy          *string `json:"updatedBy"`
	DeletedAt          *string `json:"deletedAt"`
}

// PIIResponse is returned by GET /:id/pii (full decrypt, permission-gated).
type PIIResponse struct {
	ID               string  `json:"id"`
	KodeCounterparty string  `json:"kodeCounterparty"`
	// Decrypted PII
	NPWP          *string `json:"npwp"`
	NomorRekening *string `json:"nomorRekening"`
	KTP           *string `json:"ktp"`
}

// DeleteResponse returned by soft-delete endpoint.
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

// WorkflowActionRequest is the body for submit/review/approve/reject.
type WorkflowActionRequest struct {
	Comment         *string `json:"comment"`
	SignatureMethod  string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

// WorkflowRejectRequest adds mandatory comment.
type WorkflowRejectRequest struct {
	Comment         string `json:"comment"        binding:"required,min=10"`
	SignatureMethod  string `json:"signatureMethod"`
	RowVersion      *int64 `json:"rowVersion"`
}

// ToResponse converts domain entity to JSON response (PII masked).
func ToResponse(cp *Counterparty, maskedPII *MaskedPII) Response {
	r := Response{
		ID:               cp.ID.String(),
		KodeCounterparty: cp.KodeCounterparty,
		Nama:             cp.Nama,
		Tipe:             string(cp.Tipe),
		RatingPefindoCurrent: cp.RatingPefindoCurrent,
		TipeEksposurBasel: string(cp.TipeEksposurBasel),
		EligibleLpsFlag:  cp.EligibleLpsFlag,
		NomorIzinOjk:     cp.NomorIzinOjk,
		TanggalIzinOjk:   cp.TanggalIzinOjk,
		KategoriMi:       cp.KategoriMi,
		Status:           string(cp.Status),
		WorkflowStatus:   displayWorkflowStatus(cp.WorkflowStatus),
		RowVersion:       cp.RowVersion,
		CreatedAt:        cp.CreatedAt.Format(time.RFC3339),
		CreatedBy:        cp.CreatedBy.String(),
	}
	if cp.AumTerakhir != nil {
		s := cp.AumTerakhir.String()
		r.AumTerakhir = &s
	}
	if cp.TanggalAumTerakhir != nil {
		r.TanggalAumTerakhir = cp.TanggalAumTerakhir
	}
	if cp.WorkflowInstanceID != nil {
		s := cp.WorkflowInstanceID.String()
		r.WorkflowInstanceID = &s
	}
	if cp.UpdatedAt != nil {
		s := cp.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	if cp.UpdatedBy != nil {
		s := cp.UpdatedBy.String()
		r.UpdatedBy = &s
	}
	if cp.DeletedAt != nil {
		s := cp.DeletedAt.Format(time.RFC3339)
		r.DeletedAt = &s
	}

	// Apply masked PII
	if maskedPII != nil {
		r.NPWP = maskedPII.NPWP
		r.NomorRekening = maskedPII.NomorRekening
		r.KTP = maskedPII.KTP
	}
	return r
}

// MaskedPII holds already-masked PII strings from repo.
// Fields are exported so external test packages can construct stubs.
type MaskedPII struct {
	NPWP          *string
	NomorRekening *string
	KTP           *string
}

// displayWorkflowStatus maps internal REJECTED → RETURNED for API consumers.
func displayWorkflowStatus(s WorkflowStatus) string {
	if s == WorkflowStatusRejected {
		return string(WorkflowStatusReturned)
	}
	return string(s)
}

// mapWorkflowState converts workflow engine state string to counterparty WorkflowStatus.
func mapWorkflowState(state string) WorkflowStatus {
	switch state {
	case "DRAFT":
		return WorkflowStatusDraft
	case "PENDING_REVIEW":
		return WorkflowStatusPendingReview
	case "PENDING_APPROVAL":
		return WorkflowStatusPendingApproval
	case "PENDING_APPROVAL_2":
		return WorkflowStatusPendingApproval2
	case "APPROVED":
		return WorkflowStatusApproved
	case "REJECTED":
		return WorkflowStatusRejected
	default:
		return WorkflowStatus(state)
	}
}

// redactedPII is used in audit log before/after to avoid logging plaintext PII.
const redactedPII = "REDACTED"

// auditSafeCounterparty builds an audit-safe map from Counterparty (no PII).
func auditSafeCounterparty(cp *Counterparty) map[string]interface{} {
	m := map[string]interface{}{
		"id":                  cp.ID.String(),
		"kode_counterparty":   cp.KodeCounterparty,
		"nama":                cp.Nama,
		"tipe":                string(cp.Tipe),
		"tipe_eksposur_basel": string(cp.TipeEksposurBasel),
		"eligible_lps_flag":   cp.EligibleLpsFlag,
		"status":              string(cp.Status),
		"workflow_status":     string(cp.WorkflowStatus),
		"row_version":         cp.RowVersion,
		// PII always REDACTED in audit log
		"npwp_encrypted":          redactedPII,
		"nomor_rekening_encrypted": redactedPII,
		"ktp_encrypted":           redactedPII,
	}
	if cp.RatingPefindoCurrent != nil {
		m["rating_pefindo_current"] = *cp.RatingPefindoCurrent
	}
	return m
}
