// Package ratinghistory implements the mst.rating_history_counterparty module (APP-A-MSTR-003b).
//
// SICR computation (DEC-011):
//   - notch_change <= -2 → sicr_triggered = true
//   - Rating change IG → non-IG → sicr_triggered = true (parse via IsInvestmentGrade)
//   - default_triggered = (rating == "idD")
//
// On workflow Approve:
//   - Close previous active rating (set tanggal_berakhir = tanggal_berlaku - 1)
//   - Set sicr_triggered, default_triggered based on computed rules
//   - Update counterparty.rating_pefindo_current (cache) in same tx
//
// IG Classification (Pefindo scale):
//
//	Investment Grade: idAAA, idAA+, idAA, idAA-, idA+, idA, idA-, idBBB+, idBBB, idBBB-
//	Non-IG (Speculative): idBB+ and below, idD
//
// DPD ≥ 30 SICR trigger: Phase 5 ECL engine scope — NOT implemented here.
package ratinghistory

import (
	"strings"
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Error codes ──────────────────────────────────────────────────────────────

const (
	// CodeRatingHistoryMultipleActive returned when trying to create a second active rating.
	CodeRatingHistoryMultipleActive = string(domainerrors.CodeValidationFailed)

	// CodeRatingHistoryInvalidActionType returned when action_type not in whitelist.
	CodeRatingHistoryInvalidActionType = string(domainerrors.CodeValidationFailed)
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

// IsEditable returns true for states allowing data edits.
func (s WorkflowStatus) IsEditable() bool {
	return s == WorkflowStatusDraft || s == WorkflowStatusRejected || s == WorkflowStatusReturned
}

// ActionType is the whitelist for action_type column.
type ActionType string

const (
	ActionInitial    ActionType = "INITIAL"
	ActionUpgrade    ActionType = "UPGRADE"
	ActionDowngrade  ActionType = "DOWNGRADE"
	ActionAffirmed   ActionType = "AFFIRMED"
	ActionWithdrawn  ActionType = "WITHDRAWN"
	ActionCorrection ActionType = "CORRECTION"
)

// validActionTypes is the whitelist.
var validActionTypes = map[ActionType]bool{
	ActionInitial: true, ActionUpgrade: true, ActionDowngrade: true,
	ActionAffirmed: true, ActionWithdrawn: true, ActionCorrection: true,
}

// IsValidActionType returns true if at is in the whitelist.
func IsValidActionType(at string) bool {
	return validActionTypes[ActionType(at)]
}

// ─── SICR computation (DEC-011) ───────────────────────────────────────────────

// investmentGradeRatings is the set of Pefindo IG rating symbols.
// Source: Pefindo Annual Default Study + FSD-APP-C §3.2 (SICR triggers).
// Scale (highest to lowest IG): idAAA, idAA+, idAA, idAA-, idA+, idA, idA-, idBBB+, idBBB, idBBB-
var investmentGradeRatings = map[string]bool{
	"idaaa":  true,
	"idaa+":  true,
	"idaa":   true,
	"idaa-":  true,
	"ida+":   true,
	"ida":    true,
	"ida-":   true,
	"idbbb+": true,
	"idbbb":  true,
	"idbbb-": true,
}

// IsInvestmentGrade returns true if the Pefindo rating is Investment Grade.
// Comparison is case-insensitive.
// WITHDRAWN ratings are treated as non-IG (no principal cash flows expected).
func IsInvestmentGrade(rating string) bool {
	return investmentGradeRatings[strings.ToLower(strings.TrimSpace(rating))]
}

// ComputeSICR determines whether a rating change triggers SICR (DEC-011).
//
// Returns (sicr_triggered bool, default_triggered bool).
//
// SICR is triggered if:
//  1. notch_change <= -2 (downgrade by 2+ notches)
//  2. previousRating was IG AND newRating is non-IG
//
// default_triggered is true only when newRating == "idD".
// previousRating = "" means INITIAL — no SICR from IG→non-IG transition
// (there is no previous rating to compare against), but notch rule still applies.
func ComputeSICR(notchChange int, previousRating, newRating string) (sicrTriggered, defaultTriggered bool) {
	newRatingLower := strings.ToLower(strings.TrimSpace(newRating))
	defaultTriggered = (newRatingLower == "idd")

	// Rule 1: notch_change <= -2 (at least 2-notch downgrade)
	if notchChange <= -2 {
		sicrTriggered = true
	}

	// Rule 2: IG → non-IG transition
	if previousRating != "" &&
		IsInvestmentGrade(previousRating) &&
		!IsInvestmentGrade(newRating) {
		sicrTriggered = true
	}

	return sicrTriggered, defaultTriggered
}

// ─── Domain entity ────────────────────────────────────────────────────────────

// RatingHistory is the domain entity for mst.rating_history_counterparty.
type RatingHistory struct {
	ID                     uuid.UUID  `db:"id"`
	RatingHistoryIDKode    string     `db:"rating_history_id_kode"`
	CounterpartyID         uuid.UUID  `db:"counterparty_id"`
	TanggalBerlaku         string     `db:"tanggal_berlaku"`  // DATE "YYYY-MM-DD"
	TanggalBerakhir        *string    `db:"tanggal_berakhir"` // DATE, nil = active
	RatingPefindo          string     `db:"rating_pefindo"`
	RatingOutlook          *string    `db:"rating_outlook"`
	SumberRating           string     `db:"sumber_rating"`
	TanggalPublikasiRating string     `db:"tanggal_publikasi_rating"` // DATE
	ActionType             ActionType `db:"action_type"`
	NotchChange            int        `db:"notch_change"`
	SicrTriggered          bool       `db:"sicr_triggered"`
	DefaultTriggered       bool       `db:"default_triggered"`
	DokumenBuktiID         *uuid.UUID `db:"dokumen_bukti_id"`

	// Legacy workflow cols (0001)
	MakerID    uuid.UUID  `db:"maker_id"`
	ApproverID *uuid.UUID `db:"approver_id"`
	ApprovedAt *time.Time `db:"approved_at"`

	// Standard audit cols (0015)
	CreatedAt      time.Time      `db:"created_at"`
	CreatedBy      *uuid.UUID     `db:"created_by"`
	UpdatedAt      *time.Time     `db:"updated_at"`
	UpdatedBy      *uuid.UUID     `db:"updated_by"`
	DeletedAt      *time.Time     `db:"deleted_at"`
	DeletedBy      *uuid.UUID     `db:"deleted_by"`
	RowVersion     int64          `db:"row_version"`
	TenantID       string         `db:"tenant_id"`
	WorkflowStatus WorkflowStatus `db:"workflow_status"`
}

// ─── Allowed columns ──────────────────────────────────────────────────────────

var AllowedSortCols = []string{
	"rating_history_id_kode", "counterparty_id", "tanggal_berlaku",
	"rating_pefindo", "action_type", "notch_change",
	"sicr_triggered", "default_triggered", "created_at", "workflow_status",
}

var AllowedFilterCols = []string{
	"counterparty_id", "action_type", "sicr_triggered",
	"default_triggered", "workflow_status",
}

var SearchCols = []string{"rating_history_id_kode", "rating_pefindo"}

var AllAllowedCols = append(append([]string{}, AllowedSortCols...), AllowedFilterCols...)

// ─── Request / Response types ─────────────────────────────────────────────────

// CreateRequest is the POST body.
type CreateRequest struct {
	RatingHistoryIDKode    string  `json:"ratingHistoryIdKode"    binding:"required,min=3,max=20"`
	CounterpartyID         string  `json:"counterpartyId"         binding:"required"`
	TanggalBerlaku         string  `json:"tanggalBerlaku"         binding:"required"`
	RatingPefindo          string  `json:"ratingPefindo"          binding:"required,min=1,max=8"`
	RatingOutlook          *string `json:"ratingOutlook"`
	SumberRating           string  `json:"sumberRating"           binding:"required"`
	TanggalPublikasiRating string  `json:"tanggalPublikasiRating" binding:"required"`
	ActionType             string  `json:"actionType"             binding:"required"`
	NotchChange            int     `json:"notchChange"`
	DokumenBuktiID         *string `json:"dokumenBuktiId"`
}

// UpdateRequest is the PUT body.
type UpdateRequest struct {
	RatingPefindo          *string `json:"ratingPefindo"          binding:"omitempty,min=1,max=8"`
	RatingOutlook          *string `json:"ratingOutlook"`
	SumberRating           *string `json:"sumberRating"`
	TanggalPublikasiRating *string `json:"tanggalPublikasiRating"`
	ActionType             *string `json:"actionType"`
	NotchChange            *int    `json:"notchChange"`
	DokumenBuktiID         *string `json:"dokumenBuktiId"`
	RowVersion             int64   `json:"rowVersion"             binding:"required"`
}

// Response is the JSON representation.
type Response struct {
	ID                     string  `json:"id"`
	RatingHistoryIDKode    string  `json:"ratingHistoryIdKode"`
	CounterpartyID         string  `json:"counterpartyId"`
	TanggalBerlaku         string  `json:"tanggalBerlaku"`
	TanggalBerakhir        *string `json:"tanggalBerakhir"`
	RatingPefindo          string  `json:"ratingPefindo"`
	IsInvestmentGrade      bool    `json:"isInvestmentGrade"`
	RatingOutlook          *string `json:"ratingOutlook"`
	SumberRating           string  `json:"sumberRating"`
	TanggalPublikasiRating string  `json:"tanggalPublikasiRating"`
	ActionType             string  `json:"actionType"`
	NotchChange            int     `json:"notchChange"`
	SicrTriggered          bool    `json:"sicrTriggered"`
	DefaultTriggered       bool    `json:"defaultTriggered"`
	DokumenBuktiID         *string `json:"dokumenBuktiId"`
	WorkflowStatus         string  `json:"workflowStatus"`
	WorkflowInstanceID     *string `json:"workflowInstanceId"`
	RowVersion             int64   `json:"rowVersion"`
	CreatedAt              string  `json:"createdAt"`
	CreatedBy              *string `json:"createdBy"`
	UpdatedAt              *string `json:"updatedAt"`
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

// ToResponse converts RatingHistory to the JSON response shape.
func ToResponse(rh *RatingHistory) Response {
	r := Response{
		ID:                     rh.ID.String(),
		RatingHistoryIDKode:    rh.RatingHistoryIDKode,
		CounterpartyID:         rh.CounterpartyID.String(),
		TanggalBerlaku:         rh.TanggalBerlaku,
		TanggalBerakhir:        rh.TanggalBerakhir,
		RatingPefindo:          rh.RatingPefindo,
		IsInvestmentGrade:      IsInvestmentGrade(rh.RatingPefindo),
		RatingOutlook:          rh.RatingOutlook,
		SumberRating:           rh.SumberRating,
		TanggalPublikasiRating: rh.TanggalPublikasiRating,
		ActionType:             string(rh.ActionType),
		NotchChange:            rh.NotchChange,
		SicrTriggered:          rh.SicrTriggered,
		DefaultTriggered:       rh.DefaultTriggered,
		WorkflowStatus:         displayWorkflowStatus(rh.WorkflowStatus),
		RowVersion:             rh.RowVersion,
		CreatedAt:              rh.CreatedAt.Format(time.RFC3339),
	}
	if rh.DokumenBuktiID != nil {
		s := rh.DokumenBuktiID.String()
		r.DokumenBuktiID = &s
	}
	if rh.CreatedBy != nil {
		s := rh.CreatedBy.String()
		r.CreatedBy = &s
	}
	if rh.UpdatedAt != nil {
		s := rh.UpdatedAt.Format(time.RFC3339)
		r.UpdatedAt = &s
	}
	return r
}

// displayWorkflowStatus maps REJECTED → RETURNED for API consumers.
func displayWorkflowStatus(s WorkflowStatus) string {
	if s == WorkflowStatusRejected {
		return string(WorkflowStatusReturned)
	}
	return string(s)
}

// mapWorkflowState converts workflow engine state string to RatingHistory WorkflowStatus.
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
