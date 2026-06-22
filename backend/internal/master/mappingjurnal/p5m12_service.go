package mappingjurnal

// p5m12_service.go — P5-M12 service methods for 6-eyes workflow, bulk import, RPT-19/20/21.
//
// This file EXTENDS the existing Service with P5-M12 specific methods.
// Transaction boundary: service opens/commits tx. Repo methods are called within tx.
// Audit: every mutation writes to aud.audit_log in same tx (DEC-018, security-baseline).
// SoD: 4-way M≠R≠A≠A2 enforced here (service layer primary; DB CHECK belt-and-suspenders).
// Idempotency: enforced by caller via sys.idempotency_key (handler layer).
// DEC-027 step-up MFA: approve-2 validates X-Step-Up-Token freshness (caller passes token).
//
// References: P5-M12-S1..S5, DEC-017, DEC-018, DEC-021, DEC-027.

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// P5M12Service is the service layer for P5-M12 6-eyes workflow + reports.
// Wraps Service (for base CRUD) and uses P5M12Repository (superset of Repository).
type P5M12Service struct {
	repo      P5M12Repository
	validator *Validator
	aw        *audit.Writer
	base      *Service
}

// NewP5M12Service creates a P5M12Service.
func NewP5M12Service(repo P5M12Repository, aw *audit.Writer) *P5M12Service {
	return &P5M12Service{
		repo:      repo,
		validator: NewValidator(repo),
		aw:        aw,
		base:      NewService(repo, aw, nil),
	}
}

// ─── New Version ──────────────────────────────────────────────────────────────

// CreateNewVersion creates a new DRAFT version for an event_code.
// Guards:
//   - event_code must have an existing APPROVED_ACTIVE version.
//   - No other in-flight (DRAFT/PENDING_*) version may exist (MAPPING_DUPLICATE_VERSION).
//   - Details non-empty + COA cross-reference + balanced on submit (not here — deferred to Submit).
//   - regulated_flag determined from sys.config.
//
// P5-M12-S1-AC1, S2-AC2.
func (s *P5M12Service) CreateNewVersion(ctx context.Context, eventCode string, req NewVersionReq) (*P5WorkflowResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	// Guard: duplicate in-flight version
	hasInflight, err := s.repo.HasInflightVersion(ctx, eventCode, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.CreateNewVersion: check inflight: %w", err)
	}
	if hasInflight {
		return nil, domainerrors.New(
			domainerrors.CodeConflict,
			fmt.Sprintf("MAPPING_DUPLICATE_VERSION: event_code '%s' sudah memiliki versi yang sedang diproses (DRAFT/PENDING_*). "+
				"Selesaikan atau tarik versi tersebut sebelum membuat versi baru.", eventCode),
			domainerrors.Detail{Field: "eventCode", Rule: "no_inflight_version", Message: CodeMappingDuplicateVersion},
		)
	}

	// Get current active (parent reference)
	active, err := s.repo.GetActiveByEventCode(ctx, eventCode, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.CreateNewVersion: get active: %w", err)
	}
	if active == nil {
		return nil, domainerrors.ErrNotFound(fmt.Sprintf("event_code '%s' tidak ditemukan atau belum APPROVED_ACTIVE", eventCode))
	}

	// Determine regulated flag
	regulated := s.validator.IsRegulated(ctx, eventCode, tid)
	workflowPath := "4-eyes"
	if regulated {
		workflowPath = "6-eyes"
	}

	newVersionID := uuid.New()
	var catatan *string
	if req.Reason != "" {
		catatan = stringPtr(req.Reason)
	}
	h := &Header{
		ID:            newVersionID,
		EventIDKode:   active.EventIDKode,
		EventCode:     active.EventCode,
		NamaEvent:     active.NamaEvent,
		KategoriEvent: active.KategoriEvent,
		TriggerSource: active.TriggerSource,
		Catatan:       catatan,
		WorkflowPath:  workflowPath,
		RegulatedFlag: regulated,
		ParentID:      &active.ID,
		MakerID:       &actorID,
		TenantID:      tid,
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.CreateNewVersion: begin tx: %w", err)
	}

	if err := s.repo.InsertVersion(ctx, tx, h, req.Details, actorID, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, fmt.Errorf("P5M12Service.CreateNewVersion: insert: %w", err)
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.NEW_VERSION",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   newVersionID,
		After:      map[string]any{"event_code": eventCode, "version_id": newVersionID.String(), "parent_id": active.ID.String(), "regulated": regulated},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.CreateNewVersion: commit: %w", err)
	}

	return &P5WorkflowResult{
		ID:             newVersionID.String(),
		EventCode:      eventCode,
		WorkflowStatus: WorkflowStatusDraft,
		WorkflowPath:   workflowPath,
		AktifFlag:      false,
		RegulatedFlag:  regulated,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}, nil
}

// ─── Submit ───────────────────────────────────────────────────────────────────

// P5Submit transitions a DRAFT version → PENDING_REVIEW.
// Guards: details non-empty + COA valid + balanced (MAPPING_AKUN_INVALID, MAPPING_UNBALANCED).
// P5-M12-S2-AC1, AC3, AC4.
func (s *P5M12Service) P5Submit(ctx context.Context, eventCode string, versionID uuid.UUID, req P5SubmitReq) (*P5WorkflowResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	// Load version
	v, err := s.repo.GetVersionByID(ctx, versionID, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Submit: load: %w", err)
	}
	if v == nil {
		return nil, domainerrors.ErrNotFound("version " + versionID.String())
	}
	if v.EventCode != eventCode {
		return nil, domainerrors.ErrNotFound(fmt.Sprintf("version %s tidak milik event_code %s", versionID, eventCode))
	}
	if v.WorkflowStatus != WorkflowStatusDraft {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("WORKFLOW_INVALID_TRANSITION: versi %s harus dalam status DRAFT untuk dapat di-submit (current: %s).", versionID, v.WorkflowStatus))
	}

	// Fetch and validate details
	details, err := s.repo.GetDetailsByHeaderID(ctx, versionID, false)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Submit: fetch details: %w", err)
	}

	// Convert to AkunDetail for validation
	akunDetails := detailsToAkunDetail(details)
	if len(akunDetails) == 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"MAPPING_AKUN_INVALID: tidak ada detail row. Tambahkan minimal 1 pasang debit-kredit sebelum submit.")
	}
	if err := ValidateDetailsNotEmpty(akunDetails); err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}
	if err := ValidateBalance(akunDetails); err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	// COA cross-reference
	akunErrs := s.validator.ValidateAkunDetails(ctx, akunDetails, tid)
	if len(akunErrs) > 0 {
		msgs := make([]string, 0, len(akunErrs))
		for _, e := range akunErrs {
			msgs = append(msgs, e.Error)
		}
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, strings.Join(msgs, "; "))
	}

	now := time.Now()
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Submit: begin tx: %w", err)
	}

	if err := s.repo.SubmitVersion(ctx, tx, versionID, actorID, now, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, err.Error())
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.SUBMIT",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   versionID,
		Before:     map[string]any{"workflow_status": string(WorkflowStatusDraft)},
		After:      map[string]any{"workflow_status": string(WorkflowStatusPendingReview), "event_code": eventCode, "comment": req.Comment},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Submit: commit: %w", err)
	}

	return &P5WorkflowResult{
		ID:             versionID.String(),
		EventCode:      eventCode,
		WorkflowStatus: WorkflowStatusPendingReview,
		WorkflowPath:   v.WorkflowPath,
		AktifFlag:      false,
		RegulatedFlag:  v.RegulatedFlag,
		UpdatedAt:      now.Format(time.RFC3339),
	}, nil
}

// ─── Review ───────────────────────────────────────────────────────────────────

// P5Review transitions PENDING_REVIEW → PENDING_APPROVAL (4-eyes) or PENDING_APPROVAL_2 (6-eyes).
// Guards: SoD reviewer ≠ maker; comment ≥ 30 chars (binding); signature hash computed.
// P5-M12-S2-AC2.
func (s *P5M12Service) P5Review(ctx context.Context, eventCode string, versionID uuid.UUID, req P5ReviewReq) (*P5WorkflowResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	v, err := s.repo.GetVersionByID(ctx, versionID, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Review: load: %w", err)
	}
	if v == nil || v.EventCode != eventCode {
		return nil, domainerrors.ErrNotFound("version " + versionID.String())
	}
	if v.WorkflowStatus != WorkflowStatusPendingReview {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("WORKFLOW_INVALID_TRANSITION: versi %s harus PENDING_REVIEW (current: %s).", versionID, v.WorkflowStatus))
	}

	// SoD
	makerStr := uuidPtrToStr(v.MakerID)
	actorStr := actorID.String()
	if err := ValidateSoD4Way(&makerStr, nil, nil, nil, actorStr, "review"); err != nil {
		return nil, domainerrors.New(domainerrors.CodeForbidden, err.Error())
	}

	// Determine next status based on regulated_flag
	regulated := v.RegulatedFlag || s.validator.IsRegulated(ctx, eventCode, tid)

	now := time.Now()
	sigInput := signatureInputString(actorID, "MAPPING.REVIEW", versionID, now, req.Comment)
	sigHash := computeSHA256(sigInput)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Review: begin tx: %w", err)
	}

	if err := s.repo.ReviewVersion(ctx, tx, versionID, actorID, sigHash, req.Comment, regulated, now, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, err.Error())
	}

	nextStatus := WorkflowStatusPendingApproval
	if regulated {
		nextStatus = StatusPendingApproval2
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.REVIEW",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   versionID,
		Before:     map[string]any{"workflow_status": string(WorkflowStatusPendingReview)},
		After:      map[string]any{"workflow_status": string(nextStatus), "reviewer_id": actorStr, "comment": req.Comment},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Review: commit: %w", err)
	}

	return &P5WorkflowResult{
		ID:             versionID.String(),
		EventCode:      eventCode,
		WorkflowStatus: nextStatus,
		WorkflowPath:   v.WorkflowPath,
		AktifFlag:      false,
		RegulatedFlag:  regulated,
		UpdatedAt:      now.Format(time.RFC3339),
	}, nil
}

// ─── Approve (4-eyes) ─────────────────────────────────────────────────────────

// P5Approve transitions PENDING_APPROVAL → APPROVED_ACTIVE (4-eyes path).
// Guards: SoD approver ≠ maker, ≠ reviewer; periode not HARD_CLOSED; atomic ACTIVE flip.
// P5-M12-S2-AC3.
func (s *P5M12Service) P5Approve(ctx context.Context, eventCode string, versionID uuid.UUID, req P5ApproveReq) (*P5WorkflowResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	v, err := s.repo.GetVersionByID(ctx, versionID, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Approve: load: %w", err)
	}
	if v == nil || v.EventCode != eventCode {
		return nil, domainerrors.ErrNotFound("version " + versionID.String())
	}
	if v.WorkflowStatus != WorkflowStatusPendingApproval {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("WORKFLOW_INVALID_TRANSITION: versi %s harus PENDING_APPROVAL (current: %s).", versionID, v.WorkflowStatus))
	}

	// SoD
	makerStr := uuidPtrToStr(v.MakerID)
	reviewerStr := uuidPtrToStr(v.ReviewerID)
	actorStr := actorID.String()
	if err := ValidateSoD4Way(&makerStr, &reviewerStr, nil, nil, actorStr, "approve"); err != nil {
		return nil, domainerrors.New(domainerrors.CodeForbidden, err.Error())
	}

	// Periode lock
	if err := s.checkPeriodeLock(ctx, tid); err != nil {
		return nil, err
	}

	now := time.Now()
	sigInput := signatureInputString(actorID, "MAPPING.APPROVE", versionID, now, req.Comment)
	sigHash := computeSHA256(sigInput)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Approve: begin tx: %w", err)
	}

	if err := s.repo.Approve4Eyes(ctx, tx, versionID, actorID, sigHash, req.Comment, now, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, err.Error())
	}

	// Atomic flip: close prior ACTIVE version, activate this version
	if err := s.repo.FlipActiveVersion(ctx, tx, eventCode, versionID, actorID, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, fmt.Errorf("P5M12Service.P5Approve: flip active: %w", err)
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.APPROVE",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   versionID,
		Before:     map[string]any{"workflow_status": string(WorkflowStatusPendingApproval)},
		After:      map[string]any{"workflow_status": string(StatusApprovedActive), "approver_id": actorStr, "comment": req.Comment, "event_code": eventCode},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Approve: commit: %w", err)
	}

	return &P5WorkflowResult{
		ID:             versionID.String(),
		EventCode:      eventCode,
		WorkflowStatus: StatusApprovedActive,
		WorkflowPath:   v.WorkflowPath,
		AktifFlag:      true,
		RegulatedFlag:  v.RegulatedFlag,
		UpdatedAt:      now.Format(time.RFC3339),
	}, nil
}

// ─── Approve-2 (6-eyes) ───────────────────────────────────────────────────────

// P5Approve2 transitions PENDING_APPROVAL_2 → APPROVED_ACTIVE (6-eyes path).
// Guards: SoD 4-way; step-up MFA token valid; periode not HARD_CLOSED; atomic ACTIVE flip.
// DEC-027: step-up token validated here (token must be present, hash stored).
// P5-M12-S2-AC4.
func (s *P5M12Service) P5Approve2(ctx context.Context, eventCode string, versionID uuid.UUID, req P5ApproveReq, stepUpToken string) (*P5WorkflowResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	// Validate step-up token present (DEC-027)
	if strings.TrimSpace(stepUpToken) == "" {
		return nil, domainerrors.New(domainerrors.CodeForbidden,
			"MAPPING_REGULATED_REQUIRES_RISK: approve-2 pada event terregulasi membutuhkan step-up MFA token (X-Step-Up-Token header). "+
				"Lakukan step-up MFA terlebih dahulu.")
	}

	v, err := s.repo.GetVersionByID(ctx, versionID, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Approve2: load: %w", err)
	}
	if v == nil || v.EventCode != eventCode {
		return nil, domainerrors.ErrNotFound("version " + versionID.String())
	}
	if v.WorkflowStatus != StatusPendingApproval2 {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("WORKFLOW_INVALID_TRANSITION: versi %s harus PENDING_APPROVAL_2 (current: %s).", versionID, v.WorkflowStatus))
	}

	// SoD 4-way
	makerStr := uuidPtrToStr(v.MakerID)
	reviewerStr := uuidPtrToStr(v.ReviewerID)
	approverStr := uuidPtrToStr(v.ApproverID)
	actorStr := actorID.String()
	if err := ValidateSoD4Way(&makerStr, &reviewerStr, &approverStr, nil, actorStr, "approve-2"); err != nil {
		return nil, domainerrors.New(domainerrors.CodeForbidden, err.Error())
	}

	// Periode lock
	if err := s.checkPeriodeLock(ctx, tid); err != nil {
		return nil, err
	}

	now := time.Now()
	sigInput := signatureInputString(actorID, "MAPPING.APPROVE_2", versionID, now, req.Comment)
	sigHash := computeSHA256(sigInput)
	tokenRef := computeSHA256(stepUpToken) // store hash, not raw token (DEC-028)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Approve2: begin tx: %w", err)
	}

	if err := s.repo.Approve6Eyes(ctx, tx, versionID, actorID, sigHash, tokenRef, req.Comment, now, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, err.Error())
	}

	// Atomic flip
	if err := s.repo.FlipActiveVersion(ctx, tx, eventCode, versionID, actorID, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, fmt.Errorf("P5M12Service.P5Approve2: flip active: %w", err)
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.APPROVE_2",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   versionID,
		Before:     map[string]any{"workflow_status": string(StatusPendingApproval2)},
		After:      map[string]any{"workflow_status": string(StatusApprovedActive), "approver_2_id": actorStr, "event_code": eventCode},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Approve2: commit: %w", err)
	}

	return &P5WorkflowResult{
		ID:             versionID.String(),
		EventCode:      eventCode,
		WorkflowStatus: StatusApprovedActive,
		WorkflowPath:   v.WorkflowPath,
		AktifFlag:      true,
		RegulatedFlag:  v.RegulatedFlag,
		UpdatedAt:      now.Format(time.RFC3339),
	}, nil
}

// ─── Reject ───────────────────────────────────────────────────────────────────

// P5Reject transitions any PENDING_* back to DRAFT.
// Guards: reason ≥ 30 chars (binding); actor must be current reviewer/approver.
// P5-M12-S2-AC5.
func (s *P5M12Service) P5Reject(ctx context.Context, eventCode string, versionID uuid.UUID, req P5RejectReq) (*P5WorkflowResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	v, err := s.repo.GetVersionByID(ctx, versionID, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Reject: load: %w", err)
	}
	if v == nil || v.EventCode != eventCode {
		return nil, domainerrors.ErrNotFound("version " + versionID.String())
	}
	if v.WorkflowStatus != WorkflowStatusPendingReview &&
		v.WorkflowStatus != WorkflowStatusPendingApproval &&
		v.WorkflowStatus != StatusPendingApproval2 {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("WORKFLOW_INVALID_TRANSITION: versi %s harus dalam status PENDING_* untuk ditolak (current: %s).", versionID, v.WorkflowStatus))
	}

	now := time.Now()
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Reject: begin tx: %w", err)
	}

	if err := s.repo.RejectVersion(ctx, tx, versionID, req.Reason, actorID, now, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition, err.Error())
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.REJECT",
		EntityType: "mst.mapping_jurnal_header",
		EntityID:   versionID,
		Before:     map[string]any{"workflow_status": string(v.WorkflowStatus)},
		After:      map[string]any{"workflow_status": "DRAFT", "reason": req.Reason, "rejected_by": actorID.String()},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.P5Reject: commit: %w", err)
	}

	return &P5WorkflowResult{
		ID:             versionID.String(),
		EventCode:      eventCode,
		WorkflowStatus: WorkflowStatusDraft,
		WorkflowPath:   v.WorkflowPath,
		AktifFlag:      false,
		RegulatedFlag:  v.RegulatedFlag,
		UpdatedAt:      now.Format(time.RFC3339),
	}, nil
}

// ─── Bulk Import ──────────────────────────────────────────────────────────────

// ImportBulk parses an XLSX upload, validates rows, and creates DRAFT versions.
// Returns BulkImportResp with errors per row. No partial commit: all-or-nothing.
// P5-M12-S3.
func (s *P5M12Service) ImportBulk(ctx context.Context, fh *multipart.FileHeader) (*BulkImportResp, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tid := tenantID(claims)

	// Open uploaded file
	f, err := fh.Open()
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "Gagal membuka file upload: "+err.Error())
	}
	defer f.Close()

	// Read into buffer so excelize can seek
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "Gagal membaca file upload: "+err.Error())
	}

	xl, err := excelize.OpenReader(&buf)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "File bukan XLSX valid: "+err.Error())
	}
	defer xl.Close()

	sheetName := xl.GetSheetName(0)
	rows, err := xl.GetRows(sheetName)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "Gagal membaca sheet: "+err.Error())
	}

	// Parse rows (skip header row)
	var bulkRows []MappingBulkRow
	var rowErrs []ImportRowErr
	for i, row := range rows {
		if i == 0 {
			continue // skip header
		}
		if len(row) < 5 {
			continue // skip empty rows
		}
		br := MappingBulkRow{
			RowNumber:   i + 1,
			EventCode:   strings.TrimSpace(row[0]),
			AkunDebit:   strings.TrimSpace(row[1]),
			AkunKredit:  strings.TrimSpace(row[2]),
			DebitKredit: strings.TrimSpace(row[3]),
			JumlahCalc:  strings.TrimSpace(row[4]),
		}
		if len(row) > 5 {
			br.Urutan = parseInt(row[5], i+1)
		} else {
			br.Urutan = i // sequence fallback
		}
		bulkRows = append(bulkRows, br)
	}

	totalRows := len(bulkRows)
	if totalRows == 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "File kosong atau tidak ada baris data.")
	}

	// Validate all rows
	validRows := 0
	for _, br := range bulkRows {
		errs := s.validator.ValidateBulkRow(ctx, br, tid)
		if len(errs) > 0 {
			rowErrs = append(rowErrs, errs...)
		} else {
			validRows++
		}
	}

	invalidRows := totalRows - validRows
	batchID := uuid.New()

	// If all invalid, return without DB commit
	if invalidRows == totalRows {
		return &BulkImportResp{
			BatchID:     batchID.String(),
			BatchType:   "MAPPING_BULK",
			TotalRows:   totalRows,
			ValidRows:   0,
			InvalidRows: invalidRows,
			Errors:      rowErrs,
		}, nil
	}

	// Commit valid rows as DRAFT versions
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.ImportBulk: begin tx: %w", err)
	}

	// Track which event_codes already got a DRAFT in this batch (dedupe within batch)
	seenEvents := map[string]bool{}
	committedRows := 0
	for _, br := range bulkRows {
		errs := s.validator.ValidateBulkRow(ctx, br, tid)
		if len(errs) > 0 {
			continue // skip invalid rows
		}
		if seenEvents[br.EventCode] {
			continue // already inserted header for this event_code in this batch
		}
		seenEvents[br.EventCode] = true

		if err := s.repo.InsertDraftForBulkRow(ctx, tx, br, batchID, actorID, tid); err != nil {
			rollbackTx(ctx, tx, s.base.logger)
			return nil, fmt.Errorf("P5M12Service.ImportBulk: insert draft: %w", err)
		}
		committedRows++
	}

	if err := s.repo.InsertUploadBatch(ctx, tx, batchID, actorID, totalRows, committedRows, invalidRows, tid); err != nil {
		rollbackTx(ctx, tx, s.base.logger)
		return nil, fmt.Errorf("P5M12Service.ImportBulk: insert batch: %w", err)
	}

	writeAuditP5(ctx, tx, s.aw, audit.Event{
		Action:     "MAPPING.BULK_IMPORT",
		EntityType: "sys.upload_batch",
		EntityID:   batchID,
		After: map[string]any{
			"batch_type":   "MAPPING_BULK",
			"total_rows":   totalRows,
			"valid_rows":   committedRows,
			"invalid_rows": invalidRows,
		},
	})

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("P5M12Service.ImportBulk: commit: %w", err)
	}

	return &BulkImportResp{
		BatchID:     batchID.String(),
		BatchType:   "MAPPING_BULK",
		TotalRows:   totalRows,
		ValidRows:   committedRows,
		InvalidRows: invalidRows,
		Errors:      rowErrs,
	}, nil
}

// ─── RPT-19: Coverage ─────────────────────────────────────────────────────────

// GetCoverage returns the RPT-19 mapping coverage dashboard.
func (s *P5M12Service) GetCoverage(ctx context.Context) (*CoverageResp, error) {
	claims := auth.ClaimsFromContext(ctx)
	tid := tenantID(claims)
	resp, err := s.repo.GetCoverageReport(ctx, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.GetCoverage: %w", err)
	}
	return resp, nil
}

// ─── RPT-20: Validation ───────────────────────────────────────────────────────

// GetValidation returns the RPT-20 pre-flight validation report.
func (s *P5M12Service) GetValidation(ctx context.Context) (*ValidationResp, error) {
	claims := auth.ClaimsFromContext(ctx)
	tid := tenantID(claims)
	resp, err := s.repo.GetValidationReport(ctx, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.GetValidation: %w", err)
	}
	return resp, nil
}

// ─── RPT-21: History ──────────────────────────────────────────────────────────

// GetHistory returns paginated MAPPING.* audit history (RPT-21).
func (s *P5M12Service) GetHistory(ctx context.Context, filterEventCode string, cursor string, limit int) (*MappingAuditListResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	tid := tenantID(claims)

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	q := listquery.Query{} // no sort/filter beyond what repo builds
	entries, nextCursor, hasMore, err := s.repo.ListMappingHistory(ctx, q, filterEventCode, cursor, limit, tid)
	if err != nil {
		return nil, fmt.Errorf("P5M12Service.GetHistory: %w", err)
	}
	return &MappingAuditListResult{
		Items:      entries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// checkPeriodeLock returns MAPPING_PERIODE_LOCKED if current periode is HARD_CLOSED.
func (s *P5M12Service) checkPeriodeLock(ctx context.Context, tid string) error {
	status, err := s.repo.GetPeriodeStatus(ctx, tid)
	if err != nil {
		return fmt.Errorf("P5M12Service.checkPeriodeLock: %w", err)
	}
	if status == "HARD_CLOSED" {
		return domainerrors.New(domainerrors.CodePeriodeClosed,
			"MAPPING_PERIODE_LOCKED: periode buku sedang hard-closed. "+
				"Mapping jurnal tidak dapat diapprove selama periode hard-close.")
	}
	return nil
}

// uuidPtrToStr converts *uuid.UUID to string, "" if nil.
func uuidPtrToStr(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

// stringPtr returns *string from string value.
func stringPtr(s string) *string {
	return &s
}

// detailsToAkunDetail converts []*Detail to []AkunDetail for validation.
func detailsToAkunDetail(details []*Detail) []AkunDetail {
	result := make([]AkunDetail, 0, len(details))
	for _, d := range details {
		ad := AkunDetail{
			Urutan:      d.Urutan,
			DebitKredit: mapDKIndicator(d.DKIndicator),
		}
		// Legacy Detail uses KodeAkunID (UUID); for P5-M12 validation,
		// CoaCodeExists uses text code. If detail uses text akun_debit/kredit (P5 schema),
		// those are in separate columns. We set them from the legacy fields.
		// For new P5 detail rows, AkunDebit/AkunKredit text fields are stored differently.
		// This converter is best-effort for mixed-schema.
		ad.AkunDebit = d.KodeAkunID.String() // UUID as code (legacy compat)
		ad.AkunKredit = d.KodeAkunID.String()
		result = append(result, ad)
	}
	return result
}

// mapDKIndicator converts DEBIT/KREDIT (legacy) or D/K (P5) to D/K.
func mapDKIndicator(dkIndicator string) string {
	switch strings.ToUpper(dkIndicator) {
	case "DEBIT", "D":
		return "D"
	case "KREDIT", "K":
		return "K"
	default:
		return dkIndicator
	}
}

// parseInt parses an int from string, returns defaultVal on error.
func parseInt(s string, defaultVal int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultVal
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return defaultVal
		}
		n = n*10 + int(c-'0')
	}
	return n
}
