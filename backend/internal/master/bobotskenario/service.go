package bobotskenario

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx attempts to rollback a transaction, logging any error.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "bobotskenario service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for bobot_skenario.
// It manages transaction boundaries; repo methods must be called with a tx when inside one.
//
// ECL parameter module — BLOCKING ifrs9-compliance-reviewer gate on every PR (DEC-010).
type Service struct {
	repo        Repository
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewService constructs a Service.
func NewService(repo Repository, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, auditWriter: auditWriter, logger: logger}
}

// dateRe validates YYYY-MM-DD date string.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// bobotMin and bobotMax are the inclusive bounds for bobot (DEC-016 + DB CHECK constraint).
var (
	bobotMin = decimal.NewFromInt(0)
	bobotMax = decimal.NewFromInt(1)
)

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new BobotSkenario record in DRAFT state.
// Writes audit BOBOT_SKENARIO.CREATE in the same transaction.
//
// Validation includes:
//  1. Skenario whitelist.
//  2. Bobot [0,1] range.
//  3. Date format + sampai >= dari.
//  4. Duplicate (skenario, period) check → BOBOT_DUPLICATE_SKENARIO_PERIOD.
//  5. Period overlap per skenario → BOBOT_PERIOD_OVERLAP.
//
// Note: Sum invariant (DEC-010) is checked by the Approve transition, not on every Create,
// because individual skenario rows are created one at a time. The sum=1.0 constraint is
// meaningful only when all 3 rows exist for a period.
func (s *Service) Create(ctx context.Context, req CreateRequest) (*BobotSkenario, error) {
	bobotDec, err := s.validateCreate(req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Duplicate check: same (skenario, period) tuple must not exist.
	dupCount, err := s.repo.CountDuplicate(ctx, Skenario(req.Skenario), req.PeriodeBerlakuDari, req.PeriodeBerlakuSampai, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("service.Create duplicate check: %w", err)
	}
	if dupCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeBobotDuplicateSkenarioPeriod,
			fmt.Sprintf("Skenario %s sudah ada untuk periode berlaku_dari=%s. "+
				"Gunakan Update untuk mengubah nilai bobot yang ada.",
				req.Skenario, req.PeriodeBerlakuDari),
			domainerrors.Detail{
				Field:   "body.skenario",
				Rule:    "unique_per_period",
				Message: fmt.Sprintf("Skenario %s sudah ada untuk periode ini", req.Skenario),
			},
		)
	}

	// Overlap check: same skenario, overlapping period range.
	overlapCount, err := s.repo.CountOverlap(ctx, Skenario(req.Skenario), req.PeriodeBerlakuDari, req.PeriodeBerlakuSampai, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("service.Create overlap check: %w", err)
	}
	if overlapCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeBobotPeriodOverlap,
			fmt.Sprintf("Skenario %s sudah memiliki %d entri dengan periode yang overlap. "+
				"Verifikasi dengan ALCO sebelum lanjut.", req.Skenario, overlapCount),
			domainerrors.Detail{
				Field:   "body.periodeBerlakuDari",
				Rule:    "no_overlap",
				Message: fmt.Sprintf("%d entri dengan periode overlapping ditemukan", overlapCount),
			},
		)
	}

	now := time.Now()
	e := &BobotSkenario{
		ID:                   uuid.New(),
		Skenario:             Skenario(req.Skenario),
		Bobot:                bobotDec,
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		Catatan:              req.Catatan,
		// Legacy columns: set maker_id = currentUser for DB NOT NULL constraint.
		MakerID:        actorID,
		ApproverID:     nil,
		ApprovedAt:     nil,
		WorkflowStatus: WorkflowStatusDraft,
		CreatedAt:      now,
		CreatedBy:      &actorID,
		RowVersion:     1,
		TenantID:       tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, e); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "BOBOT_SKENARIO.CREATE",
		EntityType: "mst.bobot_skenario",
		EntityID:   e.ID,
		After:      e,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}
	return e, nil
}

// ─── GetByID ──────────────────────────────────────────────────────────────────

// GetByID fetches one record, returning ErrNotFound if absent.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*BobotSkenario, error) {
	e, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if e == nil {
		return nil, domainerrors.ErrNotFound("Bobot Skenario " + id.String())
	}
	return e, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*BobotSkenario
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List: %w", err)
	}

	fetchedCount := len(items)
	lastID := ""
	if fetchedCount > limit {
		items = items[:limit]
		lastID = items[limit-1].ID.String()
	}

	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies a partial update to an existing record.
// Guard: workflow_status MUST be DRAFT or RETURNED (REJECTED at DB level).
// Guard: row_version optimistic lock.
// Validation: duplicate + overlap checks on effective post-update values.
// Writes audit BOBOT_SKENARIO.UPDATE same-tx.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*BobotSkenario, error) {
	bobotPtr, err := s.validateUpdate(req)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Bobot Skenario " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Bobot Skenario %s sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan amandemen melalui workflow.", id.String()),
		)
	}

	before := *current

	// Determine effective post-update values for validation.
	effectiveSkenario := current.Skenario
	if req.Skenario != nil {
		effectiveSkenario = Skenario(*req.Skenario)
	}
	effectiveDari := current.PeriodeBerlakuDari
	if req.PeriodeBerlakuDari != nil {
		effectiveDari = *req.PeriodeBerlakuDari
	}
	effectiveSampai := current.PeriodeBerlakuSampai
	if req.PeriodeBerlakuSampai != nil {
		effectiveSampai = req.PeriodeBerlakuSampai
	}

	// Duplicate check: ensure the new (skenario, period) combination is unique.
	dupCount, err := s.repo.CountDuplicate(ctx, effectiveSkenario, effectiveDari, effectiveSampai, id)
	if err != nil {
		return nil, fmt.Errorf("service.Update duplicate check: %w", err)
	}
	if dupCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeBobotDuplicateSkenarioPeriod,
			fmt.Sprintf("Skenario %s sudah ada untuk periode ini. Update ditolak.", effectiveSkenario),
			domainerrors.Detail{
				Field:   "body.skenario",
				Rule:    "unique_per_period",
				Message: fmt.Sprintf("Skenario %s sudah ada untuk periode berlaku_dari=%s", effectiveSkenario, effectiveDari),
			},
		)
	}

	// Overlap check.
	overlapCount, err := s.repo.CountOverlap(ctx, effectiveSkenario, effectiveDari, effectiveSampai, id)
	if err != nil {
		return nil, fmt.Errorf("service.Update overlap check: %w", err)
	}
	if overlapCount > 0 {
		return nil, domainerrors.New(
			domainerrors.CodeBobotPeriodOverlap,
			fmt.Sprintf("Skenario %s sudah memiliki %d entri dengan periode yang overlap setelah pembaruan ini.", effectiveSkenario, overlapCount),
			domainerrors.Detail{
				Field:   "body.periodeBerlakuDari",
				Rule:    "no_overlap",
				Message: fmt.Sprintf("%d entri dengan periode overlapping", overlapCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	var skenarioPtr *Skenario
	if req.Skenario != nil {
		sk := Skenario(*req.Skenario)
		skenarioPtr = &sk
	}

	fields := UpdateFields{
		Skenario:             skenarioPtr,
		Bobot:                bobotPtr,
		PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
		PeriodeBerlakuSampai: req.PeriodeBerlakuSampai,
		Catatan:              req.Catatan,
		UpdatedBy:            actorID,
		ExpectedVersion:      req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Bobot Skenario " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "BOBOT_SKENARIO.UPDATE",
		EntityType: "mst.bobot_skenario",
		EntityID:   updated.ID,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
// Guard: active ECL calc-result lines reference this record → ENTITY_IN_USE (409).
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Bobot Skenario " + id.String())
	}

	refCount, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Bobot Skenario %s tidak bisa dihapus karena masih direferensikan oleh %d calc result line. "+
				"Tutup atau archive calc run terkait terlebih dahulu.", id.String(), refCount),
			domainerrors.Detail{
				Field:   "id",
				Rule:    "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d ECL calc result line", refCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "BOBOT_SKENARIO.DELETE",
		EntityType: "mst.bobot_skenario",
		EntityID:   deleted.ID,
		Before:     existing,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete: commit: %w", err)
	}
	return nil
}

// ─── SeedDefault ─────────────────────────────────────────────────────────────

// SeedDefault creates 3 DRAFT rows (GOOD=0.25, NORMAL=0.50, BAD=0.25) for the
// given periode_berlaku_dari. Idempotent: if all 3 already exist, skip.
//
// Permission: ecl_parameter.submit (per task spec).
// DEC-010: default G/N/B = 0.25/0.50/0.25.
func (s *Service) SeedDefault(ctx context.Context, req SeedDefaultRequest) (*SeedDefaultResult, error) {
	if !dateRe.MatchString(req.PeriodeBerlakuDari) {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"periodeBerlakuDari harus dalam format YYYY-MM-DD",
			domainerrors.Detail{Field: "body.periodeBerlakuDari", Rule: "format",
				Message: "Format tanggal harus YYYY-MM-DD"},
		)
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Idempotency: check how many rows already exist for this period.
	// seed-default uses open-ended period (sampai=nil).
	existing, err := s.repo.CountByPeriod(ctx, req.PeriodeBerlakuDari, nil)
	if err != nil {
		return nil, fmt.Errorf("service.SeedDefault count by period: %w", err)
	}
	if existing >= 3 {
		return &SeedDefaultResult{Created: 0, IDs: []string{}, Skipped: true}, nil
	}

	now := time.Now()
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.SeedDefault: begin tx: %w", err)
	}

	var createdIDs []string
	for _, sk := range AllSkenarios {
		e := &BobotSkenario{
			ID:                   uuid.New(),
			Skenario:             sk,
			Bobot:                DefaultBobot(sk),
			PeriodeBerlakuDari:   req.PeriodeBerlakuDari,
			PeriodeBerlakuSampai: nil, // open-ended
			MakerID:              actorID,
			ApproverID:           nil,
			ApprovedAt:           nil,
			WorkflowStatus:       WorkflowStatusDraft,
			CreatedAt:            now,
			CreatedBy:            &actorID,
			RowVersion:           1,
			TenantID:             tenantID(claims),
		}

		if err := s.repo.Create(ctx, tx, e); err != nil {
			rollbackTx(ctx, tx, s.logger)
			return nil, fmt.Errorf("service.SeedDefault create %s: %w", sk, err)
		}

		if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:     "BOBOT_SKENARIO.CREATE",
			EntityType: "mst.bobot_skenario",
			EntityID:   e.ID,
			After:      e,
		}); err != nil {
			rollbackTx(ctx, tx, s.logger)
			return nil, fmt.Errorf("service.SeedDefault: audit write %s: %w", sk, err)
		}

		createdIDs = append(createdIDs, e.ID.String())
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.SeedDefault: commit: %w", err)
	}

	return &SeedDefaultResult{
		Created: len(createdIDs),
		IDs:     createdIDs,
		Skipped: false,
	}, nil
}

// ─── SyncWorkflowStatus ───────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine callback after a state transition.
// For APPROVE → APPROVED transition, enforces the DEC-010 sum=1.0 invariant
// before allowing the status change.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	e, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if e == nil {
		return domainerrors.ErrNotFound("Bobot Skenario entity")
	}

	wfStatus := mapWorkflowState(newState)

	// CRITICAL DEC-010 invariant: before setting APPROVED, verify sum(bobot) = 1.0
	// for all active rows in this entity's period. This is the definitive check.
	if wfStatus == WorkflowStatusApproved {
		if err := s.checkSumInvariantForApprove(ctx, e); err != nil {
			return err
		}
	}

	auditAction := "BOBOT_SKENARIO." + action

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.bobot_skenario",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(e.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": string(wfStatus)},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: commit: %w", err)
	}
	return nil
}

// checkSumInvariantForApprove verifies that G+N+B sum = 1.0 for the entity's period.
//
// DEC-010: sum of GOOD + NORMAL + BAD bobot must equal exactly 1.0 within tolerance
// SumTolerance (0.00000001). This function:
//  1. Fetches the current bobot of the entity being approved.
//  2. Sums all other active rows for the same period (using SumByPeriod with excludeID=this row).
//  3. Adds this row's bobot to get totalSum.
//  4. Checks |totalSum - 1.0| <= SumTolerance.
//
// If sum < 1.0 (e.g. 0.95): "Kurang dari 1.0 (current sum: 0.95)"
// If sum > 1.0 (e.g. 1.05): "Lebih dari 1.0 (current sum: 1.05)"
// Both cases return 422 BOBOT_SUM_INVARIANT_VIOLATED.
func (s *Service) checkSumInvariantForApprove(ctx context.Context, e *BobotSkenario) error {
	otherSum, err := s.repo.SumByPeriod(ctx, e.PeriodeBerlakuDari, e.PeriodeBerlakuSampai, e.ID)
	if err != nil {
		return fmt.Errorf("service.checkSumInvariant: sum by period: %w", err)
	}

	totalSum := otherSum.Add(e.Bobot)
	diff := totalSum.Sub(SumTarget).Abs()

	if diff.GreaterThan(SumTolerance) {
		direction := "Kurang dari"
		if totalSum.GreaterThan(SumTarget) {
			direction = "Lebih dari"
		}
		return domainerrors.New(
			domainerrors.CodeBobotSumInvariantViolated,
			fmt.Sprintf("Total bobot G+N+B untuk periode %s harus = 1.0 (current sum: %s). "+
				"%s 1.0. DEC-010.",
				e.PeriodeBerlakuDari, totalSum.StringFixed(8), direction),
			domainerrors.Detail{
				Field:   "bobot",
				Rule:    "sum_invariant",
				Message: fmt.Sprintf("Sum bobot = %s, expected 1.0 (tolerance 0.00000001)", totalSum.StringFixed(8)),
			},
		)
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given bobot_skenario ID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Bobot Skenario " + id.String())
	}

	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit BOBOT_SKENARIO.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV: %w", err)
	}

	// Audit write best-effort (export is read-only).
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "BOBOT_SKENARIO.EXPORT",
			EntityType:  "mst.bobot_skenario",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "bobotskenario ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "bobotskenario ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

// validateCreate runs all field-level + cross-field validation for create.
// Returns the parsed bobot decimal on success.
func (s *Service) validateCreate(req CreateRequest) (decimal.Decimal, error) {
	var details []domainerrors.Detail

	// Skenario whitelist.
	if !IsValidSkenario(Skenario(req.Skenario)) {
		details = append(details, domainerrors.Detail{
			Field:   "body.skenario",
			Rule:    "oneof",
			Message: fmt.Sprintf("skenario harus salah satu dari: GOOD, NORMAL, BAD. Diterima: %q", req.Skenario),
		})
	}

	// Bobot: parse decimal and validate range [0,1].
	bobotDec, bobotErr := decimal.NewFromString(req.Bobot)
	if bobotErr != nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.bobot",
			Rule:    "decimal",
			Message: "bobot harus berupa angka desimal (contoh: \"0.25000000\")",
		})
	} else if bobotDec.LessThan(bobotMin) || bobotDec.GreaterThan(bobotMax) {
		details = append(details, domainerrors.Detail{
			Field:   "body.bobot",
			Rule:    "range",
			Message: "bobot harus antara 0 dan 1 (inklusif)",
		})
	}

	// Date format.
	if !dateRe.MatchString(req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "periodeBerlakuDari harus dalam format YYYY-MM-DD",
		})
	}

	// sampai must be >= dari (if present).
	if req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai != "" {
		if !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "format",
				Message: "periodeBerlakuSampai harus dalam format YYYY-MM-DD",
			})
		} else if req.PeriodeBerlakuDari != "" && *req.PeriodeBerlakuSampai < req.PeriodeBerlakuDari {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "min",
				Message: "periodeBerlakuSampai harus >= periodeBerlakuDari",
			})
		}
	}

	if len(details) > 0 {
		return decimal.Zero, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return bobotDec, nil
}

// validateUpdate runs validation for update request.
// Returns the parsed bobot decimal pointer (nil if bobot not in request).
func (s *Service) validateUpdate(req UpdateRequest) (*decimal.Decimal, error) {
	var details []domainerrors.Detail

	if req.Skenario != nil && !IsValidSkenario(Skenario(*req.Skenario)) {
		details = append(details, domainerrors.Detail{
			Field:   "body.skenario",
			Rule:    "oneof",
			Message: fmt.Sprintf("skenario tidak valid: %q. Harus GOOD, NORMAL, atau BAD.", *req.Skenario),
		})
	}

	var bobotPtr *decimal.Decimal
	if req.Bobot != nil {
		d, err := decimal.NewFromString(*req.Bobot)
		if err != nil {
			details = append(details, domainerrors.Detail{
				Field:   "body.bobot",
				Rule:    "decimal",
				Message: "bobot harus berupa angka desimal",
			})
		} else if d.LessThan(bobotMin) || d.GreaterThan(bobotMax) {
			details = append(details, domainerrors.Detail{
				Field:   "body.bobot",
				Rule:    "range",
				Message: "bobot harus antara 0 dan 1 (inklusif)",
			})
		} else {
			bobotPtr = &d
		}
	}

	if req.PeriodeBerlakuDari != nil && !dateRe.MatchString(*req.PeriodeBerlakuDari) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeBerlakuDari",
			Rule:    "format",
			Message: "periodeBerlakuDari harus dalam format YYYY-MM-DD",
		})
	}

	if req.PeriodeBerlakuSampai != nil && *req.PeriodeBerlakuSampai != "" {
		if !dateRe.MatchString(*req.PeriodeBerlakuSampai) {
			details = append(details, domainerrors.Detail{
				Field:   "body.periodeBerlakuSampai",
				Rule:    "format",
				Message: "periodeBerlakuSampai harus dalam format YYYY-MM-DD",
			})
		}
	}

	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
		})
	}

	if len(details) > 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return bobotPtr, nil
}

// ─── Private helpers ──────────────────────────────────────────────────────────

// requireActor extracts the actor UUID from JWT claims.
func requireActor(claims *auth.Claims) (uuid.UUID, error) {
	if claims == nil || claims.Sub == "" {
		return uuid.Nil, domainerrors.ErrUnauthorized("Claims tidak ditemukan.")
	}
	id, err := uuid.Parse(claims.Sub)
	if err != nil {
		return uuid.Nil, domainerrors.ErrUnauthorized("Sub claim bukan UUID valid.")
	}
	return id, nil
}

// tenantID extracts tenant_id from claims, defaulting to TUGURE.
func tenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

// mapWorkflowState converts workflow engine state string to BobotSkenario WorkflowStatus.
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
