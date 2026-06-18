// Package mtm implements the trx.mtm module — MTM Daily Job + Manual Upload (P5-M6).
//
// Architecture: thin handler → service (business logic, tx boundary) → repo (SQL only).
// Every service method takes context.Context; trace/tenant/user propagated via ctx.
//
// Package shape:
//
//	internal/trx/mtm/
//	  domain.go          — entity structs, status enum, error codes, request/response types
//	  routing.go         — pure function resolveJurnalEventCode (ifrs9-compliance-reviewer BLOCKING)
//	  validator.go       — price range, deviation, stale threshold checks
//	  repo.go            — sqlx CRUD; no business logic
//	  service.go         — tx boundary, business rules, audit write, workflow hooks
//	  jurnal_poster.go   — JurnalPoster interface + stub (real M2 wired in main.go)
//	  worker.go          — Asynq cron handler (trx:mtm_daily_run)
//	  handler.go         — thin HTTP handlers (8 endpoints)
//	  routes.go          — RegisterRoutes(*gin.RouterGroup, ...)
//
// Domain rules (DEC-016/017/018/021/022):
//   - AC instruments are NEVER inserted into trx.mtm (ErrMTMInstrumenACSkip)
//   - SoD: override_approver_id ≠ uploader_id enforced at service layer + DB constraint
//   - locked_flag = true → 423 MTM_PERIODE_LOCKED (app layer MtmLockMiddleware + DB trigger)
//   - Idempotency-Key mandatory on all mutating endpoints
//   - All prices/amounts: shopspring/decimal (never float64), DEC-016
//   - Audit writes in same tx as mutation, DEC-018
package mtm

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes (stable strings per api-conventions.md) ─────────────────────

const (
	// CodeMTMPriceStale — stale_price_flag=TRUE; jurnal tidak bisa diposting otomatis. HTTP 422.
	CodeMTMPriceStale = "MTM_PRICE_STALE"

	// CodeMTMPriceDeviationRejected — informational code untuk notifikasi saat override-reject
	// baris dengan deviation_flag=TRUE. Bukan HTTP response code (override-reject return 200). HTTP 422.
	CodeMTMPriceDeviationRejected = "MTM_PRICE_DEVIATION_REJECTED"

	// CodeMTMBatchNotFound — upload_batch_id tidak ditemukan. HTTP 404.
	CodeMTMBatchNotFound = "MTM_BATCH_NOT_FOUND"

	// CodeMTMOverrideSODViolation — override_approver_id == uploader_id, SoD violation. HTTP 403.
	CodeMTMOverrideSODViolation = "MTM_OVERRIDE_SOD_VIOLATION"

	// CodeMTMInstrumenACSkip — instrumen AC dimasukkan ke MTM; PSAK 71 §4.1.2 AC has no MTM. HTTP 422.
	CodeMTMInstrumenACSkip = "MTM_INSTRUMEN_AC_SKIP"

	// CodeMTMPeriodeLocked — mutasi trx.mtm saat locked_flag=TRUE (periode CLOSED). HTTP 423.
	CodeMTMPeriodeLocked = "MTM_PERIODE_LOCKED"
)

// newMTMErr builds a *domainerrors.DomainError with the appropriate HTTP status for MTM codes.
func newMTMErr(code string, message string, details ...domainerrors.Detail) *domainerrors.DomainError {
	switch code {
	case CodeMTMPeriodeLocked:
		return domainerrors.New(domainerrors.CodePeriodeClosed, message, details...)
	case CodeMTMOverrideSODViolation:
		return domainerrors.New(domainerrors.CodeSoDViolation, message, details...)
	case CodeMTMBatchNotFound:
		return domainerrors.New(domainerrors.CodeNotFound, message, details...)
	case CodeMTMInstrumenACSkip, CodeMTMPriceStale:
		return domainerrors.New(domainerrors.CodeValidationFailed, message, details...)
	default:
		return domainerrors.New(domainerrors.CodeValidationFailed, message, details...)
	}
}

// ─── Sentinel errors (service layer checks) ──────────────────────────────────

// ErrMTMInstrumenACSkip is returned by resolveJurnalEventCode when klasifikasi == "AC".
// The caller (cron worker, upload service) must NOT insert a trx.mtm row.
var ErrMTMInstrumenACSkip = newMTMErr(CodeMTMInstrumenACSkip,
	"Instrumen berklasifikasi AC — tidak ada MTM per PSAK 71 §4.1.2.")

// ErrMTMPeriodeLocked is returned when locked_flag=TRUE.
var ErrMTMPeriodeLocked = newMTMErr(CodeMTMPeriodeLocked,
	"MTM ini dikunci karena periode buku sudah hard-closed.")

// ErrMTMOverrideSODViolation is returned when override_approver_id == uploader_id.
var ErrMTMOverrideSODViolation = newMTMErr(CodeMTMOverrideSODViolation,
	"Anda tidak dapat meng-approve MTM yang Anda upload sendiri. SoD: override_approver_id ≠ uploader_id (DEC-017).")

// ─── Status enum ──────────────────────────────────────────────────────────────

// Status represents trx.mtm.status.
type Status string

const (
	StatusAutoPOSTED    Status = "AUTO_POSTED"
	StatusPendingReview Status = "PENDING_REVIEW"
	StatusApproved      Status = "APPROVED"
	StatusRejected      Status = "REJECTED"
	StatusStalePrice    Status = "STALE_PRICE"
)

// CanOverride returns true when the status allows override-approve or override-reject.
func (s Status) CanOverride() bool {
	return s == StatusPendingReview || s == StatusStalePrice
}

// ─── Harga sumber whitelist ───────────────────────────────────────────────────

// HargaSumber whitelist per DB CHECK constraint chk_mtm_harga_sumber.
type HargaSumber string

const (
	HargaSumberIBPA      HargaSumber = "IBPA"
	HargaSumberBEI       HargaSumber = "BEI"
	HargaSumberKSEI      HargaSumber = "KSEI"
	HargaSumberManual    HargaSumber = "MANUAL"
	HargaSumberIBPAManual HargaSumber = "IBPA_MANUAL"
	HargaSumberBEIManual HargaSumber = "BEI_MANUAL"
)

// validHargaSumber is the allowed set.
var validHargaSumber = map[HargaSumber]bool{
	HargaSumberIBPA:       true,
	HargaSumberBEI:        true,
	HargaSumberKSEI:       true,
	HargaSumberManual:     true,
	HargaSumberIBPAManual: true,
	HargaSumberBEIManual:  true,
}

// IsValidHargaSumber checks whether s is in the whitelist.
func IsValidHargaSumber(s string) bool {
	return validHargaSumber[HargaSumber(s)]
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// Mtm is the domain entity for one trx.mtm row.
// All money/rate fields use decimal.Decimal (DEC-016 — never float64).
type Mtm struct {
	ID                uuid.UUID        `db:"id"`
	InstrumenID       uuid.UUID        `db:"instrumen_id"`
	PeriodeBulananID  uuid.UUID        `db:"periode_bulanan_id"`
	TanggalMtm        time.Time        `db:"tanggal_mtm"` // DATE stored as time.Time (midnight UTC)

	// Price data
	HargaSumber    HargaSumber      `db:"harga_sumber"`
	HargaTanggal   time.Time        `db:"harga_tanggal"`
	HargaAgeDays   int16            `db:"harga_age_days"`
	HargaPasarFcy  *decimal.Decimal `db:"harga_pasar_fcy"` // NULL for IDR instruments
	HargaPasarIdr  decimal.Decimal  `db:"harga_pasar_idr"` // NUMERIC(20,4)
	HargaBukuIdr   decimal.Decimal  `db:"harga_buku_idr"`  // NUMERIC(20,4)
	DeltaIdr       decimal.Decimal  `db:"delta_idr"`       // NUMERIC(20,4)
	DeltaPct       decimal.Decimal  `db:"delta_pct"`       // NUMERIC(7,4)

	// FX
	KursID     *uuid.UUID       `db:"kurs_id"`     // NULL for IDR
	KursTengah *decimal.Decimal `db:"kurs_tengah"` // NUMERIC(20,8), NULL for IDR

	// Classification snapshot
	KlasifikasiSnapshot string `db:"klasifikasi_snapshot"`
	TreatmentSnapshot   string `db:"treatment_snapshot"` // e.g. "OCI_FOREIGN_EXCHANGE_RESERVE"

	// Jurnal linkage
	JurnalEntryID    *uuid.UUID `db:"jurnal_entry_id"`
	JurnalEntryID2   *uuid.UUID `db:"jurnal_entry_id_2"`   // secondary for FVOCI_DEBT FCY
	JurnalEventCode  *string    `db:"jurnal_event_code"`   // primary event code
	JurnalEventCode2 *string    `db:"jurnal_event_code_2"` // MTM_FX_OCI_RESERVE for FVOCI_DEBT FCY

	// Flags
	StalePriceFlag bool `db:"stale_price_flag"`
	DeviationFlag  bool `db:"deviation_flag"`
	LockedFlag     bool `db:"locked_flag"`

	// Workflow status
	Status Status `db:"status"`

	// Upload/cron linkage
	UploadBatchID *uuid.UUID `db:"upload_batch_id"`
	UploaderID    *uuid.UUID `db:"uploader_id"`
	CronJobID     *string    `db:"cron_job_id"`

	// Override
	OverrideApproverID *uuid.UUID `db:"override_approver_id"`
	OverrideComment    *string    `db:"override_comment"`
	OverrideAt         *time.Time `db:"override_at"`

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

// ─── Instrumen minimal struct for routing/cron ────────────────────────────────

// InstrumenInfo holds only the fields MTM service needs from mst.instrumen.
// Full instrumen domain is in internal/master/instrumen — we read via repo adapter.
type InstrumenInfo struct {
	ID               uuid.UUID
	KodeInstrumen    string
	NamaInstrumen    string
	KlasifikasiPSAK71 string // "FVOCI_DEBT" | "FVTPL" | "FVOCI_ELECTION" | "POCI" | "AC"
	KlasifikasiLocked bool
	MataUang         string // ISO 4217, e.g. "IDR", "USD"
	IsPOCI           bool
	TipeInstrumen    string // "OBLIGASI" | "SAHAM" | "REKSADANA" | "DEPOSITO" | etc.
}

// ─── Sorted list response types ───────────────────────────────────────────────

// AllowedSortCols is the whitelist of sort-able columns for GET /trx/mtm.
var AllowedSortCols = []string{
	"tanggal_mtm",
	"instrumen_kode",
	"klasifikasi_psak71",
	"delta_pct",
	"harga_age_days",
	"status",
	"harga_sumber",
	"created_at",
}

// AllowedFilterCols is the whitelist of filter-able columns for GET /trx/mtm.
var AllowedFilterCols = []string{
	"instrumen_id",
	"instrumen_kode",
	"tanggal_mtm",
	"status",
	"klasifikasi_snapshot",
	"deviation_flag",
	"stale_price_flag",
	"harga_sumber",
	"upload_batch_id",
	"periode_bulanan_id",
}

// ─── Request / Response types ─────────────────────────────────────────────────

// ListItem is one row in the GET /trx/mtm list response.
type ListItem struct {
	ID                  string   `json:"id"`
	InstrumenID         string   `json:"instrumenId"`
	InstrumenKode       string   `json:"instrumenKode"`
	InstrumenNama       string   `json:"instrumenNama"`
	TanggalMtm          string   `json:"tanggalMtm"`
	HargaSumber         string   `json:"hargaSumber"`
	HargaPasarIdr       string   `json:"hargaPasarIdr"`
	HargaBukuIdr        string   `json:"hargaBukuIdr"`
	DeltaIdr            string   `json:"deltaIdr"`
	DeltaPct            string   `json:"deltaPct"`
	HargaAgeDays        int16    `json:"hargaAgeDays"`
	StalePriceFlag      bool     `json:"stalePriceFlag"`
	DeviationFlag       bool     `json:"deviationFlag"`
	Status              string   `json:"status"`
	KlasifikasiSnapshot string   `json:"klasifikasiSnapshot"`
	JurnalEventCode     *string  `json:"jurnalEventCode"`
	JurnalEntryID       *string  `json:"jurnalEntryId"`
	UploaderID          *string  `json:"uploaderId"`
	OverrideApproverID  *string  `json:"overrideApproverId"`
	OverrideAt          *string  `json:"overrideAt"`
	LockedFlag          bool     `json:"lockedFlag"`
	CreatedAt           string   `json:"createdAt"`
}

// Detail is the full response for GET /trx/mtm/{id}.
type Detail struct {
	ID                  string    `json:"id"`
	InstrumenID         string    `json:"instrumenId"`
	InstrumenKode       string    `json:"instrumenKode"`
	InstrumenNama       string    `json:"instrumenNama"`
	PeriodeBulananID    string    `json:"periodeBulananId"`
	TanggalMtm          string    `json:"tanggalMtm"`
	HargaSumber         string    `json:"hargaSumber"`
	HargaTanggal        string    `json:"hargaTanggal"`
	HargaAgeDays        int16     `json:"hargaAgeDays"`
	HargaPasarFcy       *string   `json:"hargaPasarFcy"`
	HargaPasarIdr       string    `json:"hargaPasarIdr"`
	HargaBukuIdr        string    `json:"hargaBukuIdr"`
	DeltaIdr            string    `json:"deltaIdr"`
	DeltaPct            string    `json:"deltaPct"`
	KursID              *string   `json:"kursId"`
	KursTengah          *string   `json:"kursTengah"`
	StalePriceFlag      bool      `json:"stalePriceFlag"`
	DeviationFlag       bool      `json:"deviationFlag"`
	Status              string    `json:"status"`
	KlasifikasiSnapshot string    `json:"klasifikasiSnapshot"`
	TreatmentSnapshot   string    `json:"treatmentSnapshot"`
	JurnalEventCodes    []string  `json:"jurnalEventCodes"`
	JurnalEntryID       *string   `json:"jurnalEntryId"`
	UploadBatchID       *string   `json:"uploadBatchId"`
	UploaderID          *string   `json:"uploaderId"`
	OverrideApproverID  *string   `json:"overrideApproverId"`
	OverrideComment     *string   `json:"overrideComment"`
	OverrideAt          *string   `json:"overrideAt"`
	LockedFlag          bool      `json:"lockedFlag"`
	CronJobID           *string   `json:"cronJobId"`
	CreatedAt           string    `json:"createdAt"`
	CreatedBy           string    `json:"createdBy"`
	UpdatedAt           string    `json:"updatedAt"`
	UpdatedBy           string    `json:"updatedBy"`
	RowVersion          int64     `json:"rowVersion"`
}

// OverrideApproveRequest is the body for POST /trx/mtm/{id}/override-approve.
type OverrideApproveRequest struct {
	Comment         string `json:"comment"         binding:"required,min=30"`
	SignatureMethod string  `json:"signatureMethod"`
}

// OverrideApproveResponse is returned on successful override-approve.
type OverrideApproveResponse struct {
	MtmID           string   `json:"mtmId"`
	InstrumenKode   string   `json:"instrumenKode"`
	Status          string   `json:"status"`
	JurnalEntryID   *string  `json:"jurnalEntryId"`
	JurnalEventCodes []string `json:"jurnalEventCodes"`
	ApprovedBy      string   `json:"approvedBy"`
	ApprovedAt      string   `json:"approvedAt"`
	Message         string   `json:"message"`
}

// OverrideRejectRequest is the body for POST /trx/mtm/{id}/override-reject.
type OverrideRejectRequest struct {
	Comment         string `json:"comment"         binding:"required,min=30"`
	SignatureMethod string  `json:"signatureMethod"`
}

// OverrideRejectResponse is returned on successful override-reject.
type OverrideRejectResponse struct {
	MtmID         string `json:"mtmId"`
	InstrumenKode string `json:"instrumenKode"`
	Status        string `json:"status"`
	RejectedBy    string `json:"rejectedBy"`
	RejectedAt    string `json:"rejectedAt"`
	Comment       string `json:"comment"`
	Message       string `json:"message"`
}

// CronTriggerRequest is the body for POST /trx/mtm/cron/trigger.
type CronTriggerRequest struct {
	TanggalTarget string `json:"tanggalTarget"` // "YYYY-MM-DD", default: today
	ForceRerun    bool   `json:"forceRerun"`
}

// CronTriggerResponse is returned by POST /trx/mtm/cron/trigger.
type CronTriggerResponse struct {
	JobID              string `json:"jobId"`
	Type               string `json:"type"`
	TanggalTarget      string `json:"tanggalTarget"`
	StatusURL          string `json:"statusUrl"`
	StreamURL          string `json:"streamUrl"`
	EstimatedInstrumen int    `json:"estimatedInstrumen"`
	Message            string `json:"message"`
}

// UploadBatchResponse is returned by POST /trx/mtm/upload/batch.
type UploadBatchResponse struct {
	UploadBatchID      string              `json:"uploadBatchId"`
	RowsParsed         int                 `json:"rowsParsed"`
	RowsValid          int                 `json:"rowsValid"`
	RowsInvalid        int                 `json:"rowsInvalid"`
	Status             string              `json:"status"`
	MtmIDs             []string            `json:"mtmIds"`
	RowsCreated        []UploadRowCreated  `json:"rowsCreated"`
	StalePriceWarnings []string            `json:"stalePriceWarnings"`
	DeviationWarnings  []DeviationWarning  `json:"deviationWarnings"`
	NextStep           string              `json:"nextStep"`
}

// UploadRowCreated summarises one successfully created MTM row from upload.
type UploadRowCreated struct {
	InstrumenKode  string `json:"instrumenKode"`
	TanggalMtm     string `json:"tanggalMtm"`
	HargaPasarFcy  string `json:"hargaPasarFcy,omitempty"`
	HargaPasarIdr  string `json:"hargaPasarIdr"`
	HargaSumber    string `json:"hargaSumber"`
	DeviationFlag  bool   `json:"deviationFlag"`
	DeltaPct       string `json:"deltaPct"`
	StalePriceFlag bool   `json:"stalePriceFlag"`
}

// DeviationWarning describes a row whose delta_pct exceeds the threshold.
type DeviationWarning struct {
	InstrumenKode string  `json:"instrumenKode"`
	DeltaPct      float64 `json:"deltaPct"`
	ThresholdPct  float64 `json:"thresholdPct"`
	Message       string  `json:"message"`
}

// UploadBatchDetail is returned by GET /trx/mtm/upload/batch/{batch_id}.
type UploadBatchDetail struct {
	UploadBatchID  string              `json:"uploadBatchId"`
	UploaderID     string              `json:"uploaderId"`
	UploaderName   string              `json:"uploaderName"`
	CatatanUpload  string              `json:"catatanUpload"`
	RowsParsed     int                 `json:"rowsParsed"`
	RowsValid      int                 `json:"rowsValid"`
	RowsInvalid    int                 `json:"rowsInvalid"`
	Status         string              `json:"status"`
	CreatedAt      string              `json:"createdAt"`
	Rows           []UploadBatchRow    `json:"rows"`
}

// UploadBatchRow is one row in the batch detail.
type UploadBatchRow struct {
	LineNumber     int     `json:"lineNumber"`
	MtmID          string  `json:"mtmId"`
	InstrumenKode  string  `json:"instrumenKode"`
	InstrumenID    string  `json:"instrumenId"`
	TanggalMtm     string  `json:"tanggalMtm"`
	HargaPasarFcy  *string `json:"hargaPasarFcy"`
	HargaPasarIdr  string  `json:"hargaPasarIdr"`
	DeltaPct       string  `json:"deltaPct"`
	DeviationFlag  bool    `json:"deviationFlag"`
	StalePriceFlag bool    `json:"stalePriceFlag"`
	RowStatus      string  `json:"rowStatus"`
	RowErrorMsg    *string `json:"rowErrorMsg"`
}

// StaleAlertItem is one row in GET /trx/mtm/alerts/stale-price.
type StaleAlertItem struct {
	ID                string `json:"id"`
	InstrumenID       string `json:"instrumenId"`
	InstrumenKode     string `json:"instrumenKode"`
	KlasifikasiPSAK71 string `json:"klasifikasiPsak71"`
	TanggalMtm        string `json:"tanggalMtm"`
	HargaTanggal      string `json:"hargaTanggal"`
	HargaAgeDays      int16  `json:"hargaAgeDays"`
	Status            string `json:"status"`
	StalePriceReason  string `json:"stalePriceReason"` // "HARGA_TIDAK_TERSEDIA" | "KURS_FCY_TIDAK_TERSEDIA"
	EskalasiFag       bool   `json:"eskalasiFlag"`
}

// ─── Upload file row ──────────────────────────────────────────────────────────

// UploadFileRow is one parsed row from an XLSX/CSV upload file.
type UploadFileRow struct {
	LineNumber    int
	KodeInstrumen string
	TanggalMtm    string // "YYYY-MM-DD"
	HargaPasar    string // raw string, parsed to decimal
	HargaSumber   string // optional, default "MANUAL"
	Catatan       string // optional
}

// ─── Asynq task types ─────────────────────────────────────────────────────────

const (
	// TaskMtmDailyRun is the Asynq task type for the daily MTM cron (18:00 WIB).
	TaskMtmDailyRun = "trx:mtm_daily_run"
)

// MtmCronPayload is the Asynq task payload for trx:mtm_daily_run.
type MtmCronPayload struct {
	TanggalTarget string `json:"tanggal_target"` // "YYYY-MM-DD"
	TenantID      string `json:"tenant_id"`
	JobID         string `json:"job_id"`
	ForceRerun    bool   `json:"force_rerun"`
	ActorID       string `json:"actor_id"` // empty for scheduled cron; set for manual trigger
}

// NewDailyRunTask creates an *asynq.Task for the MTM daily run.
// tanggalTarget must be "YYYY-MM-DD"; tenantID is typically "TUGURE".
// Returns error if payload cannot be marshalled (should never happen in practice).
func NewDailyRunTask(tanggalTarget, tenantID string) (*asynq.Task, error) {
	payload, err := json.Marshal(MtmCronPayload{
		TanggalTarget: tanggalTarget,
		TenantID:      tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("mtm.NewDailyRunTask: marshal payload: %w", err)
	}
	return asynq.NewTask(TaskMtmDailyRun, payload), nil
}

// ─── MtmLocker interface (P5-M4 → P5-M6 contract) ───────────────────────────

// MtmLocker is implemented by mtm.Service. closeflow.Service will call this
// inside the same *sql.Tx to lock/unlock MTM rows when a periode is hard-closed/reopened.
type MtmLocker interface {
	// LockMtmForPeriode sets locked_flag=TRUE on all trx.mtm rows for the periode.
	// Must be called inside the same tx as the hard-close commit (same tx).
	LockMtmForPeriode(ctx interface{}, tx interface{}, periodeID uuid.UUID, actorID uuid.UUID) error

	// UnlockMtmForPeriode sets locked_flag=FALSE (on CLOSED → SOFT_CLOSED reopen).
	UnlockMtmForPeriode(ctx interface{}, tx interface{}, periodeID uuid.UUID, actorID uuid.UUID) error
}

// ─── ToListItem converts a domain entity to API list item ────────────────────

// ToListItem converts *Mtm to ListItem.
func ToListItem(m *Mtm) ListItem {
	li := ListItem{
		ID:                  m.ID.String(),
		InstrumenID:         m.InstrumenID.String(),
		TanggalMtm:          m.TanggalMtm.Format("2006-01-02"),
		HargaSumber:         string(m.HargaSumber),
		HargaPasarIdr:       m.HargaPasarIdr.StringFixed(4),
		HargaBukuIdr:        m.HargaBukuIdr.StringFixed(4),
		DeltaIdr:            m.DeltaIdr.StringFixed(4),
		DeltaPct:            m.DeltaPct.StringFixed(4),
		HargaAgeDays:        m.HargaAgeDays,
		StalePriceFlag:      m.StalePriceFlag,
		DeviationFlag:       m.DeviationFlag,
		Status:              string(m.Status),
		KlasifikasiSnapshot: m.KlasifikasiSnapshot,
		LockedFlag:          m.LockedFlag,
		CreatedAt:           m.CreatedAt.Format(time.RFC3339),
	}
	if m.JurnalEventCode != nil {
		li.JurnalEventCode = m.JurnalEventCode
	}
	if m.JurnalEntryID != nil {
		s := m.JurnalEntryID.String()
		li.JurnalEntryID = &s
	}
	if m.UploaderID != nil {
		s := m.UploaderID.String()
		li.UploaderID = &s
	}
	if m.OverrideApproverID != nil {
		s := m.OverrideApproverID.String()
		li.OverrideApproverID = &s
	}
	if m.OverrideAt != nil {
		s := m.OverrideAt.Format(time.RFC3339)
		li.OverrideAt = &s
	}
	return li
}

// ToDetail converts *Mtm to Detail.
func ToDetail(m *Mtm) Detail {
	d := Detail{
		ID:                  m.ID.String(),
		InstrumenID:         m.InstrumenID.String(),
		PeriodeBulananID:    m.PeriodeBulananID.String(),
		TanggalMtm:          m.TanggalMtm.Format("2006-01-02"),
		HargaSumber:         string(m.HargaSumber),
		HargaTanggal:        m.HargaTanggal.Format("2006-01-02"),
		HargaAgeDays:        m.HargaAgeDays,
		HargaPasarIdr:       m.HargaPasarIdr.StringFixed(4),
		HargaBukuIdr:        m.HargaBukuIdr.StringFixed(4),
		DeltaIdr:            m.DeltaIdr.StringFixed(4),
		DeltaPct:            m.DeltaPct.StringFixed(4),
		StalePriceFlag:      m.StalePriceFlag,
		DeviationFlag:       m.DeviationFlag,
		Status:              string(m.Status),
		KlasifikasiSnapshot: m.KlasifikasiSnapshot,
		TreatmentSnapshot:   m.TreatmentSnapshot,
		LockedFlag:          m.LockedFlag,
		CreatedAt:           m.CreatedAt.Format(time.RFC3339),
		CreatedBy:           m.CreatedBy.String(),
		UpdatedAt:           m.UpdatedAt.Format(time.RFC3339),
		UpdatedBy:           m.UpdatedBy.String(),
		RowVersion:          m.RowVersion,
	}
	if m.HargaPasarFcy != nil {
		s := m.HargaPasarFcy.StringFixed(8)
		d.HargaPasarFcy = &s
	}
	if m.KursID != nil {
		s := m.KursID.String()
		d.KursID = &s
	}
	if m.KursTengah != nil {
		s := m.KursTengah.StringFixed(8)
		d.KursTengah = &s
	}
	// Collect event codes
	if m.JurnalEventCode != nil {
		d.JurnalEventCodes = append(d.JurnalEventCodes, *m.JurnalEventCode)
	}
	if m.JurnalEventCode2 != nil {
		d.JurnalEventCodes = append(d.JurnalEventCodes, *m.JurnalEventCode2)
	}
	if m.JurnalEntryID != nil {
		s := m.JurnalEntryID.String()
		d.JurnalEntryID = &s
	}
	if m.UploadBatchID != nil {
		s := m.UploadBatchID.String()
		d.UploadBatchID = &s
	}
	if m.UploaderID != nil {
		s := m.UploaderID.String()
		d.UploaderID = &s
	}
	if m.OverrideApproverID != nil {
		s := m.OverrideApproverID.String()
		d.OverrideApproverID = &s
	}
	if m.OverrideComment != nil {
		d.OverrideComment = m.OverrideComment
	}
	if m.OverrideAt != nil {
		s := m.OverrideAt.Format(time.RFC3339)
		d.OverrideAt = &s
	}
	if m.CronJobID != nil {
		d.CronJobID = m.CronJobID
	}
	return d
}
