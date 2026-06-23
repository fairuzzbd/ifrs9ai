package mappingjurnal

// p5m12_domain.go — P5-M12 domain types extending the base mappingjurnal package.
// New error codes, workflow status aliases, version-chain types, report types,
// and request/response types for 6-eyes workflow + bulk import + RPT-19/20/21.
//
// References: P5-M12-S1..S5, DEC-017/018/021/022/027.

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ─── P5-M12 Error codes ───────────────────────────────────────────────────────

const (
	// CodeMappingEventNotFound: event_code not found in mst.mapping_jurnal_header (404)
	CodeMappingEventNotFound = "MAPPING_EVENT_NOT_FOUND"
	// CodeMappingAkunInvalid: akun_debit/kredit not in mst.chart_of_accounts (422)
	CodeMappingAkunInvalid = "MAPPING_AKUN_INVALID"
	// CodeMappingUnbalanced: total debit lines ≠ kredit lines per event (422)
	CodeMappingUnbalanced = "MAPPING_UNBALANCED"
	// CodeMappingRegulatedRequiresRisk: regulated event sent to 4-eyes path or approve-2 by non-ROLE-RISK (422)
	CodeMappingRegulatedRequiresRisk = "MAPPING_REGULATED_REQUIRES_RISK"
	// CodeMappingDuplicateVersion: event already has an in-flight DRAFT/PENDING_* version (409)
	CodeMappingDuplicateVersion = "MAPPING_DUPLICATE_VERSION"
	// CodeMappingSoDViolation: SoD 4-way breach M≠R≠A≠A2 (403)
	CodeMappingSoDViolation = "MAPPING_SOD_VIOLATION"
	// CodeMappingPeriodeLocked: approve/approve-2 during HARD_CLOSED periode (423)
	CodeMappingPeriodeLocked = "MAPPING_PERIODE_LOCKED"
)

// ─── P5-M12 WorkflowStatus aliases (full set including APPROVED_ACTIVE) ──────

const (
	StatusApprovedActive   WorkflowStatus = "APPROVED_ACTIVE"
	StatusPendingApproval2 WorkflowStatus = "PENDING_APPROVAL_2"
)

// ─── Version chain types ──────────────────────────────────────────────────────

// VersionDetail is a simplified header summary for version history lists.
type VersionDetail struct {
	ID             uuid.UUID      `db:"id" json:"id"`
	WorkflowStatus WorkflowStatus `db:"workflow_status" json:"workflowStatus"`
	WorkflowPath   string         `db:"workflow_path" json:"workflowPath"`
	RegulatedFlag  bool           `db:"regulated_flag" json:"regulatedFlag"`
	AktifFlag      bool           `db:"aktif_flag" json:"aktifFlag"`
	EffectiveFrom  *time.Time     `db:"effective_from" json:"effectiveFrom"`
	EffectiveTo    *time.Time     `db:"effective_to" json:"effectiveTo"`
	ParentID       *uuid.UUID     `db:"parent_id" json:"parentId"`
	MakerID        *uuid.UUID     `db:"maker_id" json:"makerId"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updatedAt"`
}

// NewVersionReq is the request body for POST /{event_code}/new-version.
// Creates a new DRAFT version of an APPROVED_ACTIVE mapping (immutable versioning DEC-018).
type NewVersionReq struct {
	Reason  string       `json:"reason"  binding:"required,min=10"`
	Details []AkunDetail `json:"details" binding:"required,min=1"`
}

// AkunDetail is one COA mapping row in a new version request.
type AkunDetail struct {
	AkunDebit   string  `json:"akunDebit"   binding:"required"`
	AkunKredit  string  `json:"akunKredit"  binding:"required"`
	DebitKredit string  `json:"debitKredit" binding:"required,oneof=D K"`
	JumlahCalc  *string `json:"jumlahCalc"`
	Urutan      int     `json:"urutan"      binding:"required,min=1"`
}

// ─── P5-M12 workflow request/response ────────────────────────────────────────

// P5SubmitReq is the body for POST /{event_code}/version/{version_id}/submit.
type P5SubmitReq struct {
	Comment string `json:"comment" binding:"required,min=1"`
}

// P5ReviewReq is the body for POST /{event_code}/version/{version_id}/review.
type P5ReviewReq struct {
	Comment        string `json:"comment"        binding:"required,min=30"`
	SignatureMethod string `json:"signatureMethod" binding:"required,oneof=JWT_STEP_UP"`
}

// P5ApproveReq is the body for POST /approve and /approve-2.
type P5ApproveReq struct {
	Comment        string `json:"comment"        binding:"required,min=10"`
	SignatureMethod string `json:"signatureMethod" binding:"required,oneof=JWT_STEP_UP"`
}

// P5RejectReq is the body for POST /reject.
type P5RejectReq struct {
	Reason string `json:"reason" binding:"required,min=30"`
}

// P5WorkflowResult is the standard workflow transition response for P5-M12.
type P5WorkflowResult struct {
	ID             string         `json:"id"`
	EventCode      string         `json:"eventCode"`
	WorkflowStatus WorkflowStatus `json:"workflowStatus"`
	WorkflowPath   string         `json:"workflowPath"`
	AktifFlag      bool           `json:"aktifFlag"`
	RegulatedFlag  bool           `json:"regulatedFlag"`
	UpdatedAt      string         `json:"updatedAt"`
}

// ─── Bulk import types ────────────────────────────────────────────────────────

// BulkImportResp is returned from POST /bulk-import.
type BulkImportResp struct {
	BatchID     string        `json:"batchId"`
	BatchType   string        `json:"batchType"`
	TotalRows   int           `json:"totalRows"`
	ValidRows   int           `json:"validRows"`
	InvalidRows int           `json:"invalidRows"`
	Errors      []ImportRowErr `json:"errors"`
}

// ImportRowErr represents one row-level validation error from bulk import.
type ImportRowErr struct {
	Row       int    `json:"row"`
	Col       string `json:"col,omitempty"`
	ErrorCode string `json:"errorCode"`
	Error     string `json:"error"`
}

// MappingBulkRow is one parsed row from the import XLSX file.
type MappingBulkRow struct {
	RowNumber   int
	EventCode   string
	AkunDebit   string
	AkunKredit  string
	DebitKredit string
	JumlahCalc  string
	Urutan      int
}

// ─── Coverage (RPT-19) ────────────────────────────────────────────────────────

// CoverageStatusP5 represents the GAP_COVERAGE badge state.
type CoverageStatusP5 string

const (
	CoverageStatusOK         CoverageStatusP5 = "OK"
	CoverageStatusMissing    CoverageStatusP5 = "MISSING"
	CoverageStatusIncomplete CoverageStatusP5 = "INCOMPLETE"
)

// CoverageEventP5 is one row in the RPT-19 coverage dashboard.
type CoverageEventP5 struct {
	EventCode         string           `json:"eventCode"`
	NamaEvent         string           `json:"namaEvent"`
	WorkflowStatus    *string          `json:"workflowStatus"`
	ActiveDetailCount int              `json:"activeDetailCount"`
	MissingAkunCount  int              `json:"missingAkunCount"`
	LastDlqError      *time.Time       `json:"lastDlqError"`
	GapCoverage       CoverageStatusP5 `json:"gapCoverage"`
}

// CoverageResp is the RPT-19 response.
type CoverageResp struct {
	TotalEvents   int               `json:"totalEvents"`
	ActiveEvents  int               `json:"activeEvents"`
	MissingEvents int               `json:"missingEvents"`
	GapEvents     []CoverageEventP5 `json:"gapEvents"`
}

// ─── Validation (RPT-20) ──────────────────────────────────────────────────────

// ValidationIssueP5 is one validation problem in RPT-20.
type ValidationIssueP5 struct {
	EventCode  string   `json:"eventCode"`
	HeaderID   string   `json:"headerId"`
	ErrorCodes []string `json:"errorCodes"`
	Details    string   `json:"details"`
}

// ValidationResp is the RPT-20 response.
type ValidationResp struct {
	TotalActiveMappings int                  `json:"totalActiveMappings"`
	ValidMappings       int                  `json:"validMappings"`
	InvalidMappings     int                  `json:"invalidMappings"`
	Issues              []ValidationIssueP5  `json:"issues"`
}

// ─── Audit log (RPT-21) ──────────────────────────────────────────────────────

// MappingAuditEntry is one row from aud.audit_log for MAPPING.* actions.
type MappingAuditEntry struct {
	EventID     uuid.UUID        `db:"event_id" json:"eventId"`
	EventTime   time.Time        `db:"event_time" json:"eventTime"`
	ActorUserID uuid.UUID        `db:"actor_user_id" json:"actorUserId"`
	ActorRole   string           `db:"actor_role" json:"actorRole"`
	Action      string           `db:"action" json:"action"`
	EntityType  string           `db:"entity_type" json:"entityType"`
	EntityID    uuid.UUID        `db:"entity_id" json:"entityId"`
	BeforeJsonb *json.RawMessage `db:"before_jsonb" json:"beforeJsonb"`
	AfterJsonb  *json.RawMessage `db:"after_jsonb" json:"afterJsonb"`
	TraceID     *string          `db:"trace_id" json:"traceId"`
}

// MappingAuditListResult holds paginated audit entries for RPT-21.
type MappingAuditListResult struct {
	Items      []MappingAuditEntry
	NextCursor *string
	HasMore    bool
}

// ─── Signature hash computation ───────────────────────────────────────────────

// signatureInput is the canonical input for SHA-256 signature hash computation.
// Format: {actorID}|{action}|{entityID}|{timestamp}|{comment}
// Matches convention from migration 000035 comments.
func signatureInputString(actorID uuid.UUID, action string, entityID uuid.UUID, at time.Time, comment string) string {
	return actorID.String() + "|" + action + "|" + entityID.String() + "|" + at.UTC().Format(time.RFC3339Nano) + "|" + comment
}
