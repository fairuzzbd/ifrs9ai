// Package staging — service layer for ECL Staging Engine.
//
// StagingService owns all business logic:
//   - SICR evaluation per DEC-011 (3 triggers).
//   - Stage transition with append-only ecl.stage_history rows.
//   - Cure assessment per DEC-012 (3 consecutive closed BULANAN periods).
//   - Override proposal 6-eyes workflow (RISK maker → RISK reviewer → ALCO → KOMITE).
//   - DPD record upsert (trx.dpd_record).
//
// Compliance constraints:
//   - Every mutation writes aud.audit_log in SAME transaction via auditWriter.WithTx(tx).Write().
//   - SoD enforced: maker ≠ reviewer ≠ approver_alco ≠ approver_komite.
//   - Step-up MFA required for ApproveALCO + ApproveKomite (DEC-026/027).
//   - Hard-delete forbidden in ecl.*: only soft-delete via deleted_at.
//   - No float64: DPD and notch delta are int; money fields use shopspring/decimal.
//   - staging_history is append-only: no Update/Delete paths exist in this service.
//
// State-machine source of truth: docs/state-machines/p4-m1-staging.md.
package staging

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx attempts a transaction rollback; logs errors (does not re-panic).
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "staging: tx rollback failed", "error", err)
	}
}

// requireActor extracts the actor UUID from JWT claims.
func requireActor(claims *auth.Claims) (uuid.UUID, error) {
	if claims == nil || claims.Sub == "" {
		return uuid.Nil, domainerrors.New(domainerrors.CodeUnauthorized, "unauthenticated request")
	}
	id, err := uuid.Parse(claims.Sub)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeUnauthorized, "invalid actor UUID in token")
	}
	return id, nil
}

// tenantFromClaims extracts tenant_id from JWT claims (falls back to TUGURE).
func tenantFromClaims(claims *auth.Claims) string {
	if claims == nil || claims.TenantID == "" {
		return "TUGURE"
	}
	return claims.TenantID
}

// ─── InstrumenReader and PeriodeBukuReader (read-only dependency interfaces) ──

// InstrumenReader is the minimal read interface for mst.instrumen that staging needs.
// Avoids circular import with the master/instrumen package.
type InstrumenReader interface {
	// GetByID fetches an instrumen (non-deleted).
	GetByID(ctx context.Context, id uuid.UUID) (*InstrumenSnapshot, error)

	// GetRatingAtDate fetches the Pefindo rating for an instrument on a given date.
	// Returns ("", nil) when no rating data found.
	GetRatingAtDate(ctx context.Context, instrumenID uuid.UUID, asOf time.Time) (string, error)

	// GetOriginationDate returns tanggal_penempatan for an instrument.
	GetOriginationDate(ctx context.Context, instrumenID uuid.UUID) (time.Time, error)
}

// PeriodeBukuReader fetches closed BULANAN periods for cure assessment.
type PeriodeBukuReader interface {
	// ListClosedBulananSince returns closed BULANAN periode_buku with tanggal_mulai >= from,
	// ordered ascending. Used by cure assessment (§5.3 step 2).
	ListClosedBulananSince(ctx context.Context, from time.Time, tenantID string) ([]time.Time, error)
}

// InstrumenSnapshot is a minimal read-only view from mst.instrumen.
type InstrumenSnapshot struct {
	ID                uuid.UUID
	KlasifikasiPSAK71 string // AC | FVOCI | FVTPL
	Status            string // AKTIF | MATURED | etc.
	TanggalPenempatan time.Time
	TenantID          string
}

// ─── Service ─────────────────────────────────────────────────────────────────

// Service is the business logic layer for the staging engine.
//
// Previously named StagingService; renamed to avoid revive stutter (package staging + type StagingService).
type Service struct {
	dpdRepo         DPDRepository
	histRepo        StageHistoryRepository
	overrideRepo    OverrideProposalRepository
	instrumenReader InstrumenReader
	periodeReader   PeriodeBukuReader
	auditWriter     *audit.Writer
	logger          *slog.Logger
}

// StagingService is an alias kept for backwards compatibility with cmd/api wiring.
// New code should use *Service directly.
type StagingService = Service

// NewStagingService constructs a Service (alias: StagingService).
func NewStagingService(
	dpdRepo DPDRepository,
	histRepo StageHistoryRepository,
	overrideRepo OverrideProposalRepository,
	instrumenReader InstrumenReader,
	periodeReader PeriodeBukuReader,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		dpdRepo:         dpdRepo,
		histRepo:        histRepo,
		overrideRepo:    overrideRepo,
		instrumenReader: instrumenReader,
		periodeReader:   periodeReader,
		auditWriter:     auditWriter,
		logger:          logger,
	}
}

// txBeginnerHist is satisfied by DBStageHistoryRepository for service to begin tx.
type txBeginnerHist interface {
	BeginTx(ctx context.Context) (*sql.Tx, error)
}

// ─── EvaluateSingleInstrumen ──────────────────────────────────────────────────

// EvaluateSingleInstrumen executes SICR logic for one instrument as of tanggalAssessment,
// inserts the resulting stage_history row(s), and returns the evaluation result.
//
// Per state-machine §3 transition table and FSD-APP-C §3.1.
//
// FVTPL instruments are skipped (no ECL per PSAK 71 §5.5.15).
// Stage 3 auto-evaluation is a no-op — cure requires manual override.
// DPD ≥ 90 from Stage 1 → atomic double-insert (Stage1→2, Stage2→3).
func (s *Service) EvaluateSingleInstrumen(ctx context.Context, instrumenID uuid.UUID, tanggalAssessment time.Time, jobID *uuid.UUID) (*EvaluationResult, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tenantID := tenantFromClaims(claims)

	instrumen, err := s.instrumenReader.GetByID(ctx, instrumenID)
	if err != nil {
		return nil, ErrStagingInstrumenNotFound(instrumenID.String())
	}

	// FVTPL → no ECL → skip.
	if instrumen.KlasifikasiPSAK71 == "FVTPL" {
		return &EvaluationResult{
			InstrumenID: instrumenID,
			Skipped:     true,
			SkipReason:  "FVTPL instrument — no ECL staging required",
		}, nil
	}

	// Get current stage (latest stage_history row).
	cur, err := s.histRepo.GetCurrentStage(ctx, instrumenID)
	if err != nil {
		return nil, fmt.Errorf("staging EvaluateSingleInstrumen: GetCurrentStage: %w", err)
	}

	currentStage := Stage1
	if cur != nil {
		currentStage = cur.StageSesudah
	}

	// Stage 3 → only manual override can move to Stage 2.
	if currentStage == Stage3 {
		return &EvaluationResult{
			InstrumenID:   instrumenID,
			PreviousStage: &currentStage,
			NewStage:      &currentStage,
			Skipped:       true,
			SkipReason:    "Stage 3 instruments require manual override for cure",
		}, nil
	}

	// Get current DPD.
	dpdRecord, dpdErr := s.dpdRepo.GetLatestDPD(ctx, instrumenID)
	currentDPD := 0
	if dpdErr == nil && dpdRecord != nil {
		currentDPD = dpdRecord.DPDValue
	}

	// Get origination rating (baseline per IFRS9 §5.5.11).
	origDate, origErr := s.instrumenReader.GetOriginationDate(ctx, instrumenID)
	originRating := ""
	currentRating := ""
	if origErr == nil {
		r, ratingErr := s.instrumenReader.GetRatingAtDate(ctx, instrumenID, origDate)
		if ratingErr != nil {
			s.logger.WarnContext(ctx, "staging: GetRatingAtDate (origination) failed; continuing without origin rating",
				"instrumen_id", instrumenID, "error", ratingErr)
		}
		originRating = r
	}
	r2, ratingErr2 := s.instrumenReader.GetRatingAtDate(ctx, instrumenID, tanggalAssessment)
	if ratingErr2 != nil {
		s.logger.WarnContext(ctx, "staging: GetRatingAtDate (current) failed; continuing without current rating",
			"instrumen_id", instrumenID, "error", ratingErr2)
	}
	currentRating = r2

	// Run SICR evaluation (DEC-011).
	// ratingPrevious is empty here (batch mode); IG→non-IG relies on notch delta as proxy.
	sicrResult := EvaluateSICR(originRating, currentRating, "", currentDPD)

	// Compute target stage.
	newStage, needsDoubleRow := ComputeNewStage(currentStage, sicrResult, currentDPD)

	changed := newStage != currentStage || needsDoubleRow

	if !changed {
		return &EvaluationResult{
			InstrumenID:   instrumenID,
			PreviousStage: &currentStage,
			NewStage:      &newStage,
			SICRResult:    sicrResult,
			Skipped:       false,
		}, nil
	}

	// Begin tx for atomic insertion.
	txBeginner, ok := s.histRepo.(txBeginnerHist)
	if !ok {
		return nil, fmt.Errorf("staging EvaluateSingleInstrumen: histRepo does not implement BeginTx")
	}
	tx, err := txBeginner.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging EvaluateSingleInstrumen: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	now := time.Now().UTC()
	rowsInserted := 0

	// Determine trigger type for first row.
	firstTrigger := sicrResult.TriggerType
	if needsDoubleRow {
		// Stage 1 → Stage 2 (DPD ≥ 30) then Stage 2 → Stage 3 (DPD ≥ 90).
		firstTrigger = TriggerDPDGte30
	}

	entry1 := &StageHistoryEntry{
		ID:                uuid.New(),
		InstrumenID:       instrumenID,
		StageSebelum:      currentStage,
		StageSesudah:      Stage2,
		TriggerType:       firstTrigger,
		DetailTrigger:     strPtr(sicrResult.Detail),
		RatingSaatMigrasi: strIfNotEmpty(currentRating),
		DPD:               &currentDPD,
		TanggalMigrasi:    tanggalAssessment,
		StatusApproval:    StatusApprovalAuto,
		TenantID:          tenantID,
		EvaluationJobID:   jobID,
		CreatedAt:         now,
		CreatedBy:         actorID,
	}

	// If going directly to Stage 3 without double-row (Stage 2 → Stage 3).
	if !needsDoubleRow && newStage == Stage3 {
		entry1.StageSesudah = Stage3
		entry1.TriggerType = TriggerDPDGte90
	}
	// If going Stage 1 → Stage 2 (non-DPD SICR).
	if !needsDoubleRow && newStage == Stage2 {
		entry1.StageSesudah = Stage2
	}

	inserted1, err := s.histRepo.Insert(ctx, tx, entry1)
	if err != nil {
		if err == ErrConflict {
			rollbackTx(ctx, tx, s.logger)
			rowsInserted = 0
			return &EvaluationResult{
				InstrumenID:         instrumenID,
				PreviousStage:       &currentStage,
				NewStage:            &newStage,
				SICRResult:          sicrResult,
				HistoryRowsInserted: rowsInserted,
			}, nil
		}
		return nil, fmt.Errorf("staging EvaluateSingleInstrumen: Insert entry1: %w", err)
	}
	rowsInserted++

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.EVALUATE",
		EntityType:  "ecl.stage_history",
		EntityID:    inserted1.ID,
		After:       stageHistoryAuditMap(inserted1),
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging EvaluateSingleInstrumen: audit entry1: %w", err)
	}

	// Second row: Stage 2 → Stage 3 for DPD ≥ 90 from Stage 1.
	if needsDoubleRow {
		entry2 := &StageHistoryEntry{
			ID:                uuid.New(),
			InstrumenID:       instrumenID,
			StageSebelum:      Stage2,
			StageSesudah:      Stage3,
			TriggerType:       TriggerDPDGte90,
			DetailTrigger:     strPtr(fmt.Sprintf("DPD≥90 auto-Stage3: dpd=%d", currentDPD)),
			RatingSaatMigrasi: strIfNotEmpty(currentRating),
			DPD:               &currentDPD,
			TanggalMigrasi:    tanggalAssessment,
			StatusApproval:    StatusApprovalAuto,
			TenantID:          tenantID,
			EvaluationJobID:   jobID,
			CreatedAt:         now,
			CreatedBy:         actorID,
		}
		inserted2, err := s.histRepo.Insert(ctx, tx, entry2)
		if err != nil && err != ErrConflict {
			return nil, fmt.Errorf("staging EvaluateSingleInstrumen: Insert entry2: %w", err)
		}
		if inserted2 != nil {
			rowsInserted++
			if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
				Action:      "STAGING.EVALUATE",
				EntityType:  "ecl.stage_history",
				EntityID:    inserted2.ID,
				After:       stageHistoryAuditMap(inserted2),
				ActorUserID: actorID.String(),
				ActorRole:   claimsRole(claims),
			}); err != nil {
				return nil, fmt.Errorf("staging EvaluateSingleInstrumen: audit entry2: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging EvaluateSingleInstrumen: commit: %w", err)
	}

	return &EvaluationResult{
		InstrumenID:         instrumenID,
		PreviousStage:       &currentStage,
		NewStage:            &newStage,
		SICRResult:          sicrResult,
		HistoryRowsInserted: rowsInserted,
	}, nil
}

// ─── RecordDPD ────────────────────────────────────────────────────────────────

// RecordDPD upserts a DPD record for (instrumen_id, periode).
// Source must be 'MANUAL' or 'APP_B' per migration 000022 CHECK constraint.
func (s *Service) RecordDPD(ctx context.Context, instrumenID uuid.UUID, periode time.Time, dpdValue int, source string, catatan *string) (*DPDRecord, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tenantID := tenantFromClaims(claims)

	now := time.Now().UTC()
	rec := &DPDRecord{
		ID:          uuid.New(),
		InstrumenID: instrumenID,
		Periode:     periode,
		DPDValue:    dpdValue,
		Source:      source,
		Catatan:     catatan,
		RecordedBy:  actorID,
		RecordedAt:  now,
		CreatedAt:   now,
		CreatedBy:   actorID,
		UpdatedAt:   now,
		UpdatedBy:   actorID,
		RowVersion:  1,
		TenantID:    tenantID,
	}

	tx, err := s.dpdRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging RecordDPD: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	saved, err := s.dpdRepo.UpsertDPD(ctx, tx, rec)
	if err != nil {
		return nil, fmt.Errorf("staging RecordDPD: UpsertDPD: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.DPD_RECORD",
		EntityType:  "trx.dpd_record",
		EntityID:    saved.ID,
		After:       dpdAuditMap(saved),
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging RecordDPD: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging RecordDPD: commit: %w", err)
	}
	return saved, nil
}

// ─── GetCurrentStage ─────────────────────────────────────────────────────────

// GetCurrentStage returns the current staging status for an instrument.
// Returns a StageStatus with CurrentStage=nil when the instrument has never been evaluated.
func (s *Service) GetCurrentStage(ctx context.Context, instrumenID uuid.UUID) (*StageStatus, error) {
	entry, err := s.histRepo.GetCurrentStage(ctx, instrumenID)
	if err != nil {
		return nil, fmt.Errorf("staging GetCurrentStage: %w", err)
	}

	ss := &StageStatus{InstrumenID: instrumenID}
	if entry != nil {
		ss.CurrentStage = &entry.StageSesudah
		ss.LastTransitionDate = &entry.TanggalMigrasi
		ss.LastTriggerType = &entry.TriggerType
		ss.LastTriggerDetail = entry.DetailTrigger
		ss.LastRatingSaatMigrasi = entry.RatingSaatMigrasi
		ss.LastDPD = entry.DPD
		ss.LastStatusApproval = &entry.StatusApproval
	}
	return ss, nil
}

// ─── GetHistory ──────────────────────────────────────────────────────────────

// GetHistory returns paginated stage_history for an instrument (newest first).
func (s *Service) GetHistory(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, cursor string, limit int) ([]*StageHistoryEntry, pagination.Result, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.histRepo.ListHistory(ctx, instrumenID, q, cursor, limit, false)
}

// ─── AssessCure ──────────────────────────────────────────────────────────────

// AssessCure runs the cure algorithm (state-machine §5.3) for a Stage 2 instrument.
//
// Algorithm (DEC-012):
//  1. Confirm current stage is STAGE_2.
//  2. Find last SICR transition date.
//  3. Collect closed BULANAN periods after that date.
//  4. Check the 3 most recent: DPD < 30 AND no SICR event.
//  5. If all 3 pass → insert Stage2 → Stage1 transition (TriggerCure3PeriodeBulanan).
//
// Stage 3 cure is MANUAL ONLY (override proposal required).
func (s *Service) AssessCure(ctx context.Context, instrumenID uuid.UUID) (cured bool, err error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return false, err
	}
	tenantID := tenantFromClaims(claims)

	entry, err := s.histRepo.GetCurrentStage(ctx, instrumenID)
	if err != nil {
		return false, fmt.Errorf("staging AssessCure: GetCurrentStage: %w", err)
	}
	if entry == nil || entry.StageSesudah != Stage2 {
		return false, nil // not in Stage 2, skip
	}

	sicrDate, err := s.histRepo.GetLastSICRDate(ctx, instrumenID)
	if err != nil {
		return false, fmt.Errorf("staging AssessCure: GetLastSICRDate: %w", err)
	}
	if sicrDate == nil {
		return false, nil
	}

	closedPeriods, err := s.periodeReader.ListClosedBulananSince(ctx, *sicrDate, tenantID)
	if err != nil {
		return false, fmt.Errorf("staging AssessCure: ListClosedBulananSince: %w", err)
	}
	if len(closedPeriods) < 3 {
		return false, nil // insufficient history
	}

	recent := closedPeriods[len(closedPeriods)-3:]
	for i, p := range recent {
		periodEnd := firstOfNextMonth(p)

		aboveDPD, err := s.dpdRepo.CountDPDAboveThreshold(ctx, instrumenID, p, periodEnd, 30)
		if err != nil {
			return false, fmt.Errorf("staging AssessCure: CountDPDAboveThreshold period %d: %w", i, err)
		}
		if aboveDPD > 0 {
			return false, nil // DPD ≥ 30 in one of the 3 periods
		}

		hasSICR, err := s.histRepo.HasSICRInPeriode(ctx, instrumenID, p, periodEnd)
		if err != nil {
			return false, fmt.Errorf("staging AssessCure: HasSICRInPeriode period %d: %w", i, err)
		}
		if hasSICR {
			return false, nil // SICR event in one of the 3 periods
		}
	}

	// All 3 periods clean → cure.
	now := time.Now().UTC()
	cureEntry := &StageHistoryEntry{
		ID:             uuid.New(),
		InstrumenID:    instrumenID,
		StageSebelum:   Stage2,
		StageSesudah:   Stage1,
		TriggerType:    TriggerCure3PeriodeBulanan,
		DetailTrigger:  strPtr("3 consecutive closed periods without SICR/DPD≥30 (DEC-012)"),
		TanggalMigrasi: recent[2],
		StatusApproval: StatusApprovalAuto,
		TenantID:       tenantID,
		CreatedAt:      now,
		CreatedBy:      actorID,
	}

	txBeginner, ok := s.histRepo.(txBeginnerHist)
	if !ok {
		return false, fmt.Errorf("staging AssessCure: histRepo does not implement BeginTx")
	}
	tx, err := txBeginner.BeginTx(ctx)
	if err != nil {
		return false, fmt.Errorf("staging AssessCure: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	inserted, err := s.histRepo.Insert(ctx, tx, cureEntry)
	if err != nil {
		if err == ErrConflict {
			rollbackTx(ctx, tx, s.logger)
			return true, nil // idempotent
		}
		return false, fmt.Errorf("staging AssessCure: Insert: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.CURE",
		EntityType:  "ecl.stage_history",
		EntityID:    inserted.ID,
		After:       stageHistoryAuditMap(inserted),
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return false, fmt.Errorf("staging AssessCure: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("staging AssessCure: commit: %w", err)
	}
	return true, nil
}

// ─── SubmitOverride ──────────────────────────────────────────────────────────

// SubmitOverride creates a new OverrideProposal (status=PENDING_REVIEW).
//
// Stage 3→2: 6-eyes (RISK maker + RISK reviewer + ALCO + KOMITE).
// Stage 2→1: 4-eyes (RISK maker + RISK reviewer + ALCO).
//
// Enforces: one active proposal per instrument; valid stage transition.
func (s *Service) SubmitOverride(ctx context.Context, req OverrideSubmitRequest) (*OverrideProposal, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	tenantID := tenantFromClaims(claims)

	// Derive StageFrom/StageTo from current stage and StageTarget.
	cur, err := s.histRepo.GetCurrentStage(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("staging SubmitOverride: GetCurrentStage: %w", err)
	}
	currentStage := Stage1
	if cur != nil {
		currentStage = cur.StageSesudah
	}

	stageFrom := currentStage
	stageTo := req.StageTarget

	if err := validateOverrideTransition(stageFrom, stageTo); err != nil {
		return nil, err
	}

	// One active proposal per instrument.
	active, err := s.overrideRepo.ListActiveForInstrumen(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("staging SubmitOverride: ListActiveForInstrumen: %w", err)
	}
	if len(active) > 0 {
		return nil, domainerrors.New(domainerrors.CodeConflict,
			"an active override proposal already exists for this instrument")
	}

	now := time.Now().UTC()
	// periodeAkhir defaults to 1 year from now if not provided by request.
	// In production this would come from mst.periode_buku.
	periodeAkhir := now.AddDate(1, 0, 0)

	prop := &OverrideProposal{
		ID:                   uuid.New(),
		InstrumenID:          req.InstrumenID,
		StageFrom:            stageFrom,
		StageTo:              stageTo,
		Alasan:               req.Alasan,
		ReasonCategory:       req.ReasonCategory,
		DokumenPendukungID:   req.DokumenPendukungID,
		PeriodeID:            req.PeriodeID,
		PeriodeAkhir:         periodeAkhir,
		WorkflowStatus:       OverrideStatusPendingReview,
		CurrentStageAtSubmit: &currentStage,
		MakerID:              actorID,
		CreatedAt:            now,
		CreatedBy:            actorID,
		UpdatedAt:            now,
		UpdatedBy:            actorID,
		RowVersion:           1,
		TenantID:             tenantID,
	}

	tx, err := s.overrideRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging SubmitOverride: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	saved, err := s.overrideRepo.Create(ctx, tx, prop)
	if err != nil {
		return nil, fmt.Errorf("staging SubmitOverride: Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.OVERRIDE_SUBMIT",
		EntityType:  "ecl.staging_override_proposal",
		EntityID:    saved.ID,
		After:       overrideAuditMap(saved),
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging SubmitOverride: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging SubmitOverride: commit: %w", err)
	}
	return saved, nil
}

// ─── ReviewOverride ──────────────────────────────────────────────────────────

// ReviewOverride transitions PENDING_REVIEW → PENDING_APPROVAL.
// SoD: reviewer ≠ maker.
func (s *Service) ReviewOverride(ctx context.Context, proposalID uuid.UUID, req WorkflowActionRequest) (*OverrideProposal, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	prop, err := s.overrideRepo.GetByID(ctx, proposalID, false)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, "override proposal not found")
	}
	if prop.WorkflowStatus != OverrideStatusPendingReview {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("expected PENDING_REVIEW, got %s", prop.WorkflowStatus))
	}
	if actorID == prop.MakerID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "reviewer cannot be the same as maker (DEC-017)")
	}

	now := time.Now().UTC()
	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}
	sigHash := ComputeSignatureHash(actorID, "REVIEW", proposalID, now, comment)

	tx, err := s.overrideRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging ReviewOverride: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	_, err = tx.ExecContext(ctx, `
		UPDATE ecl.staging_override_proposal
		SET workflow_status='PENDING_APPROVAL',
		    reviewer_id=$1,
		    signed_at_review=$2,
		    signature_hash_review=$3,
		    comment_review=$4,
		    updated_at=now(), updated_by=$1, row_version=row_version+1
		WHERE id=$5 AND deleted_at IS NULL`,
		actorID, now, sigHash, req.Comment, proposalID,
	)
	if err != nil {
		return nil, fmt.Errorf("staging ReviewOverride: update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.OVERRIDE_REVIEW",
		EntityType:  "ecl.staging_override_proposal",
		EntityID:    proposalID,
		Before:      map[string]any{"workflow_status": string(prop.WorkflowStatus)},
		After:       map[string]any{"workflow_status": "PENDING_APPROVAL", "reviewer_id": actorID.String()},
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging ReviewOverride: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging ReviewOverride: commit: %w", err)
	}
	return s.overrideRepo.GetByID(ctx, proposalID, false)
}

// ─── ApproveALCO ─────────────────────────────────────────────────────────────

// ApproveALCO transitions PENDING_APPROVAL → APPROVED_ALCO.
//
// Requires step-up MFA (DEC-027). SoD: approver_alco ≠ reviewer ≠ maker.
// For Stage 2→1 (4-eyes), this is the final approval → activates override.
func (s *Service) ApproveALCO(ctx context.Context, proposalID uuid.UUID, req WorkflowActionRequest) (*OverrideProposal, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Step-up MFA mandatory for ALCO approval (DEC-026/027).
	if claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.CodeStepUpRequired, "step-up MFA required for ALCO approval")
	}

	prop, err := s.overrideRepo.GetByID(ctx, proposalID, false)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, "override proposal not found")
	}
	if prop.WorkflowStatus != OverrideStatusPendingApproval {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("expected PENDING_APPROVAL, got %s", prop.WorkflowStatus))
	}
	if actorID == prop.MakerID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "ALCO approver cannot be the maker")
	}
	if prop.ReviewerID != nil && actorID == *prop.ReviewerID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "ALCO approver cannot be the reviewer")
	}

	now := time.Now().UTC()
	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}
	sigHash := ComputeSignatureHash(actorID, "APPROVE", proposalID, now, comment)

	is6 := prop.Is6Eyes()
	newStatus := OverrideStatusApprovedALCO
	if !is6 {
		newStatus = OverrideStatusActive
	}

	tx, err := s.overrideRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging ApproveALCO: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	_, err = tx.ExecContext(ctx, `
		UPDATE ecl.staging_override_proposal
		SET workflow_status=$1,
		    approver_alco_id=$2,
		    signed_at_approve_alco=$3,
		    signature_hash_approve_alco=$4,
		    comment_approve_alco=$5,
		    updated_at=now(), updated_by=$2, row_version=row_version+1
		WHERE id=$6 AND deleted_at IS NULL`,
		string(newStatus), actorID, now, sigHash, req.Comment, proposalID,
	)
	if err != nil {
		return nil, fmt.Errorf("staging ApproveALCO: update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.OVERRIDE_APPROVE_ALCO",
		EntityType:  "ecl.staging_override_proposal",
		EntityID:    proposalID,
		Before:      map[string]any{"workflow_status": string(prop.WorkflowStatus)},
		After:       map[string]any{"workflow_status": string(newStatus), "approver_alco_id": actorID.String()},
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging ApproveALCO: audit: %w", err)
	}

	// 4-eyes: activate immediately — insert stage_history row.
	if !is6 {
		histID, err := s.activateOverride(ctx, tx, prop, actorID, tenantFromClaims(claims))
		if err != nil {
			return nil, fmt.Errorf("staging ApproveALCO: activateOverride: %w", err)
		}
		if err := s.overrideRepo.ActivateWithHistoryRow(ctx, tx, proposalID, histID, actorID); err != nil {
			return nil, fmt.Errorf("staging ApproveALCO: ActivateWithHistoryRow: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging ApproveALCO: commit: %w", err)
	}
	return s.overrideRepo.GetByID(ctx, proposalID, false)
}

// ─── ApproveKomite ────────────────────────────────────────────────────────────

// ApproveKomite transitions APPROVED_ALCO → ACTIVE (6-eyes final).
//
// Only for Stage 3→2. Requires step-up MFA. SoD: 4 different people.
func (s *Service) ApproveKomite(ctx context.Context, proposalID uuid.UUID, req WorkflowActionRequest) (*OverrideProposal, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	// Step-up MFA mandatory for KOMITE approval (DEC-026/027).
	if claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.CodeStepUpRequired, "step-up MFA required for KOMITE approval")
	}

	prop, err := s.overrideRepo.GetByID(ctx, proposalID, false)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, "override proposal not found")
	}
	if prop.WorkflowStatus != OverrideStatusApprovedALCO {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("expected APPROVED_ALCO, got %s", prop.WorkflowStatus))
	}
	if !prop.Is6Eyes() {
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			"KOMITE approval only required for Stage 3→Stage 2 (6-eyes)")
	}
	if actorID == prop.MakerID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "KOMITE approver cannot be the maker")
	}
	if prop.ReviewerID != nil && actorID == *prop.ReviewerID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "KOMITE approver cannot be the reviewer")
	}
	if prop.ApproverALCOID != nil && actorID == *prop.ApproverALCOID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "KOMITE approver cannot be the ALCO approver")
	}

	now := time.Now().UTC()
	comment := ""
	if req.Comment != nil {
		comment = *req.Comment
	}
	sigHash := ComputeSignatureHash(actorID, "APPROVE2", proposalID, now, comment)

	tx, err := s.overrideRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging ApproveKomite: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	_, err = tx.ExecContext(ctx, `
		UPDATE ecl.staging_override_proposal
		SET workflow_status='ACTIVE',
		    approver_komite_id=$1,
		    signed_at_approve_komite=$2,
		    signature_hash_approve_komite=$3,
		    comment_approve_komite=$4,
		    updated_at=now(), updated_by=$1, row_version=row_version+1
		WHERE id=$5 AND deleted_at IS NULL`,
		actorID, now, sigHash, req.Comment, proposalID,
	)
	if err != nil {
		return nil, fmt.Errorf("staging ApproveKomite: update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.OVERRIDE_APPROVE_KOMITE",
		EntityType:  "ecl.staging_override_proposal",
		EntityID:    proposalID,
		Before:      map[string]any{"workflow_status": string(prop.WorkflowStatus)},
		After:       map[string]any{"workflow_status": "ACTIVE", "approver_komite_id": actorID.String()},
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging ApproveKomite: audit: %w", err)
	}

	histID, err := s.activateOverride(ctx, tx, prop, actorID, tenantFromClaims(claims))
	if err != nil {
		return nil, fmt.Errorf("staging ApproveKomite: activateOverride: %w", err)
	}
	if err := s.overrideRepo.ActivateWithHistoryRow(ctx, tx, proposalID, histID, actorID); err != nil {
		return nil, fmt.Errorf("staging ApproveKomite: ActivateWithHistoryRow: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging ApproveKomite: commit: %w", err)
	}
	return s.overrideRepo.GetByID(ctx, proposalID, false)
}

// ─── RejectOverride ───────────────────────────────────────────────────────────

// RejectOverride transitions any in-progress state → REJECTED.
// SoD: rejecter ≠ maker.
func (s *Service) RejectOverride(ctx context.Context, proposalID uuid.UUID, req WorkflowRejectRequest) (*OverrideProposal, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	prop, err := s.overrideRepo.GetByID(ctx, proposalID, false)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, "override proposal not found")
	}

	switch prop.WorkflowStatus {
	case OverrideStatusPendingReview, OverrideStatusPendingApproval, OverrideStatusApprovedALCO:
		// ok
	default:
		return nil, domainerrors.New(domainerrors.CodeWorkflowInvalidTransition,
			fmt.Sprintf("cannot reject from status %s", prop.WorkflowStatus))
	}

	if actorID == prop.MakerID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation, "maker cannot reject own proposal")
	}

	tx, err := s.overrideRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("staging RejectOverride: BeginTx: %w", err)
	}
	defer rollbackTx(ctx, tx, s.logger)

	_, err = tx.ExecContext(ctx, `
		UPDATE ecl.staging_override_proposal
		SET workflow_status='REJECTED',
		    reject_reason=$1,
		    updated_at=now(), updated_by=$2, row_version=row_version+1
		WHERE id=$3 AND deleted_at IS NULL`,
		req.Comment, actorID, proposalID,
	)
	if err != nil {
		return nil, fmt.Errorf("staging RejectOverride: update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.OVERRIDE_REJECT",
		EntityType:  "ecl.staging_override_proposal",
		EntityID:    proposalID,
		Before:      map[string]any{"workflow_status": string(prop.WorkflowStatus)},
		After:       map[string]any{"workflow_status": "REJECTED", "reject_reason": req.Comment},
		ActorUserID: actorID.String(),
		ActorRole:   claimsRole(claims),
	}); err != nil {
		return nil, fmt.Errorf("staging RejectOverride: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("staging RejectOverride: commit: %w", err)
	}
	return s.overrideRepo.GetByID(ctx, proposalID, false)
}

// ─── GetOverride / ListOverrides ──────────────────────────────────────────────

// GetOverride returns a single OverrideProposal by ID.
func (s *Service) GetOverride(ctx context.Context, id uuid.UUID) (*OverrideProposal, error) {
	prop, err := s.overrideRepo.GetByID(ctx, id, false)
	if err != nil {
		if err == ErrNotFound {
			return nil, domainerrors.New(domainerrors.CodeNotFound, "override proposal not found")
		}
		return nil, fmt.Errorf("staging GetOverride: %w", err)
	}
	return prop, nil
}

// ListOverrides returns paginated override proposals with filter/sort.
func (s *Service) ListOverrides(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*OverrideProposal, pagination.Result, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.overrideRepo.ListOverrides(ctx, q, cursor, limit)
}

// GetDPDHistory returns paginated DPD records for an instrument.
func (s *Service) GetDPDHistory(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, cursor string, limit int) ([]*DPDRecord, pagination.Result, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.dpdRepo.ListDPD(ctx, instrumenID, q, cursor, limit)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// activateOverride inserts the stage_history row for an approved override (within tx).
// Returns the new history row ID.
func (s *Service) activateOverride(ctx context.Context, tx *sql.Tx, prop *OverrideProposal, actorID uuid.UUID, tenantID string) (uuid.UUID, error) {
	now := time.Now().UTC()
	entry := &StageHistoryEntry{
		ID:                 uuid.New(),
		InstrumenID:        prop.InstrumenID,
		StageSebelum:       prop.StageFrom,
		StageSesudah:       prop.StageTo,
		TriggerType:        TriggerManualOverride,
		DetailTrigger:      strPtr(fmt.Sprintf("override proposal %s approved", prop.ID.String())),
		TanggalMigrasi:     now,
		StatusApproval:     StatusApprovalApproved,
		UserApproverID:     &actorID,
		OverrideProposalID: &prop.ID,
		TenantID:           tenantID,
		CreatedAt:          now,
		CreatedBy:          actorID,
	}

	inserted, err := s.histRepo.Insert(ctx, tx, entry)
	if err != nil {
		return uuid.Nil, fmt.Errorf("activateOverride: Insert: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:      "STAGING.OVERRIDE_ACTIVATED",
		EntityType:  "ecl.stage_history",
		EntityID:    inserted.ID,
		After:       stageHistoryAuditMap(inserted),
		ActorUserID: actorID.String(),
	}); err != nil {
		return uuid.Nil, fmt.Errorf("activateOverride audit: %w", err)
	}

	return inserted.ID, nil
}

// validateOverrideTransition ensures only allowed stage overrides are submitted.
// Per state-machine §2.5.
func validateOverrideTransition(from, to Stage) error {
	allowed := map[Stage]Stage{
		Stage3: Stage2, // 6-eyes
		Stage2: Stage1, // 4-eyes (ALCO only)
	}
	expected, ok := allowed[from]
	if !ok || expected != to {
		return ErrStagingOverrideInvalidTransition(string(from), string(to))
	}
	return nil
}

// firstOfNextMonth returns the first day of the month after d.
func firstOfNextMonth(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month()+1, 1, 0, 0, 0, 0, time.UTC)
}

// claimsRole returns the first role from JWT claims (for audit.Event.ActorRole).
func claimsRole(claims *auth.Claims) string {
	if claims == nil || len(claims.Roles) == 0 {
		return ""
	}
	return claims.Roles[0]
}

// strIfNotEmpty returns a pointer to s if non-empty, nil otherwise.
func strIfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ─── Audit map helpers ────────────────────────────────────────────────────────

func stageHistoryAuditMap(e *StageHistoryEntry) map[string]any {
	m := map[string]any{
		"id":              e.ID.String(),
		"instrumen_id":    e.InstrumenID.String(),
		"stage_sebelum":   string(e.StageSebelum),
		"stage_sesudah":   string(e.StageSesudah),
		"trigger_type":    string(e.TriggerType),
		"tanggal_migrasi": e.TanggalMigrasi.Format("2006-01-02"),
		"status_approval": string(e.StatusApproval),
	}
	if e.DPD != nil {
		m["dpd"] = *e.DPD
	}
	if e.RatingSaatMigrasi != nil {
		m["rating_saat_migrasi"] = *e.RatingSaatMigrasi
	}
	return m
}

func dpdAuditMap(r *DPDRecord) map[string]any {
	return map[string]any{
		"id":           r.ID.String(),
		"instrumen_id": r.InstrumenID.String(),
		"periode":      r.Periode.Format("2006-01-02"),
		"dpd_value":    r.DPDValue,
		"source":       r.Source,
	}
}

func overrideAuditMap(p *OverrideProposal) map[string]any {
	m := map[string]any{
		"id":              p.ID.String(),
		"instrumen_id":    p.InstrumenID.String(),
		"stage_from":      string(p.StageFrom),
		"stage_to":        string(p.StageTo),
		"workflow_status": string(p.WorkflowStatus),
		"maker_id":        p.MakerID.String(),
	}
	return m
}
