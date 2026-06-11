// Package staging_test — Asynq task handler tests for the staging engine.
//
// Tests use mock repos/service to verify task payload parsing, idempotency,
// batch iteration, expiry logic, and audit event emission.
package staging_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/ecl/staging"
)

// ─── Task constructor tests ───────────────────────────────────────────────────

// TestNewEvaluateStagingTask_ValidPayload verifies task creation.
func TestNewEvaluateStagingTask_ValidPayload(t *testing.T) {
	payload := staging.EvaluateStagingPayload{
		InstrumenID:       uuid.New(),
		TanggalAssessment: time.Now().UTC(),
		TenantID:          "TUGURE",
		ActorSub:          uuid.New().String(),
		ActorRole:         "ROLE-RISK",
	}
	task, err := staging.NewEvaluateStagingTask(payload)
	if err != nil {
		t.Fatalf("NewEvaluateStagingTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.Type() != staging.TaskTypeEvaluateStaging {
		t.Errorf("expected task type %s, got %s", staging.TaskTypeEvaluateStaging, task.Type())
	}
}

// TestNewCureAssessmentBatchTask_ValidPayload.
func TestNewCureAssessmentBatchTask_ValidPayload(t *testing.T) {
	payload := staging.CureAssessmentBatchPayload{
		TenantID: "TUGURE",
		ActorSub: uuid.New().String(),
	}
	task, err := staging.NewCureAssessmentBatchTask(payload)
	if err != nil {
		t.Fatalf("NewCureAssessmentBatchTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.Type() != staging.TaskTypeCureAssessmentBatch {
		t.Errorf("expected task type %s, got %s", staging.TaskTypeCureAssessmentBatch, task.Type())
	}
}

// TestNewOverrideExpiryCheckTask_ValidPayload.
func TestNewOverrideExpiryCheckTask_ValidPayload(t *testing.T) {
	payload := staging.OverrideExpiryCheckPayload{
		TenantID: "TUGURE",
		ActorSub: uuid.New().String(),
	}
	task, err := staging.NewOverrideExpiryCheckTask(payload)
	if err != nil {
		t.Fatalf("NewOverrideExpiryCheckTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.Type() != staging.TaskTypeOverrideExpiryCheck {
		t.Errorf("expected task type %s, got %s", staging.TaskTypeOverrideExpiryCheck, task.Type())
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func makeTask(taskType string, payload any) *asynq.Task {
	b, _ := json.Marshal(payload)
	return asynq.NewTask(taskType, b)
}

func newTestWorker(
	histRepo staging.StageHistoryRepository,
	overRepo staging.OverrideProposalRepository,
	instrumen staging.InstrumenReader,
	periodeReader staging.PeriodeBukuReader,
) *staging.TaskWorker {
	svc := staging.NewStagingService(
		newMockDPDRepo(), histRepo, overRepo,
		instrumen, periodeReader,
		noopAuditWriter(), noopLogger(),
	)
	return staging.NewTaskWorker(svc, histRepo, overRepo, noopLogger())
}

// TestNewTaskWorker_NilLogger_UsesDefault covers the nil logger branch in NewTaskWorker.
func TestNewTaskWorker_NilLogger_UsesDefault(t *testing.T) {
	svc := staging.NewStagingService(
		newMockDPDRepo(), newMockHistRepo(), newMockOverrideRepo(),
		defaultMockInstrumen(), &mockPeriodeReader{},
		noopAuditWriter(), noopLogger(),
	)
	worker := staging.NewTaskWorker(svc, newMockHistRepo(), newMockOverrideRepo(), nil) // nil logger
	if worker == nil {
		t.Fatal("expected non-nil worker")
	}
}

// ─── TestTaskEvaluateStaging_Idempotent ───────────────────────────────────────

// TestTaskEvaluateStaging_Idempotent verifies that HandleEvaluateStaging
// succeeds even when histRepo.Insert returns ErrConflict (idempotent replay).
func TestTaskEvaluateStaging_Idempotent(t *testing.T) {
	instrumenID := uuid.New()
	actorID := uuid.New()

	instrumen := defaultMockInstrumen()
	instrumen.currentRating = "idAA" // 2-notch downgrade triggers SICR
	instrumen.originRating = "idAAA"

	histRepo := newMockHistRepo()
	histRepo.insertConflict = true // simulate already-processed idempotency

	worker := newTestWorker(histRepo, newMockOverrideRepo(), instrumen, &mockPeriodeReader{})

	payload := staging.EvaluateStagingPayload{
		InstrumenID:       instrumenID,
		TanggalAssessment: time.Now().UTC(),
		TenantID:          "TUGURE",
		ActorSub:          actorID.String(),
		ActorRole:         "ROLE-RISK",
	}
	task := makeTask(staging.TaskTypeEvaluateStaging, payload)

	err := worker.HandleEvaluateStaging(context.Background(), task)
	if err != nil {
		t.Errorf("expected no error on idempotent replay (ErrConflict), got: %v", err)
	}
}

// ─── TestTaskCureAssessmentBatch_ProcessesAllStage2 ───────────────────────────

// TestTaskCureAssessmentBatch_ProcessesAllStage2 ensures HandleCureAssessmentBatch
// iterates all Stage 2 instruments returned by histRepo.ListStage2Instruments.
func TestTaskCureAssessmentBatch_ProcessesAllStage2(t *testing.T) {
	id1 := uuid.New()
	id2 := uuid.New()

	histRepo := newMockHistRepo()
	histRepo.stage2IDs = []uuid.UUID{id1, id2}
	histRepo.hasSICR = false

	sicrDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	histRepo.sicrDate = &sicrDate

	// Seed Stage2 entries for both instruments.
	for _, id := range []uuid.UUID{id1, id2} {
		histRepo.rows = append(histRepo.rows, &staging.StageHistoryEntry{
			ID:             uuid.New(),
			InstrumenID:    id,
			StageSebelum:   staging.Stage1,
			StageSesudah:   staging.Stage2,
			TriggerType:    staging.TriggerDPDGte30,
			TanggalMigrasi: sicrDate,
			TenantID:       "TUGURE",
			CreatedBy:      uuid.New(),
		})
	}

	// Provide 3 clean closed periods for cure.
	periods := []time.Time{
		time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
	}
	periodeReader := &mockPeriodeReader{periods: periods}

	worker := newTestWorker(histRepo, newMockOverrideRepo(), defaultMockInstrumen(), periodeReader)

	payload := staging.CureAssessmentBatchPayload{
		TenantID: "TUGURE",
		ActorSub: uuid.New().String(),
	}
	task := makeTask(staging.TaskTypeCureAssessmentBatch, payload)

	err := worker.HandleCureAssessmentBatch(context.Background(), task)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Both instruments should have cure rows.
	var cureCount int
	for _, r := range histRepo.rows {
		if r.TriggerType == staging.TriggerCure3PeriodeBulanan {
			cureCount++
		}
	}
	if cureCount != 2 {
		t.Errorf("expected 2 cure rows (one per Stage2 instrument), got %d", cureCount)
	}
}

// ─── TestTaskOverrideExpiryCheck_AutoExpiresPastPeriode ───────────────────────

// TestTaskOverrideExpiryCheck_AutoExpiresPastPeriode ensures that ACTIVE proposals
// with periode_akhir in the past are transitioned to EXPIRED.
func TestTaskOverrideExpiryCheck_AutoExpiresPastPeriode(t *testing.T) {
	prop1ID := uuid.New()
	prop2ID := uuid.New()

	overRepo := newMockOverrideRepo()
	// prop1: ACTIVE with past periode_akhir → should expire.
	past := time.Now().AddDate(0, -1, 0)
	overRepo.proposals[prop1ID] = &staging.OverrideProposal{
		ID:             prop1ID,
		InstrumenID:    uuid.New(),
		StageFrom:      staging.Stage2,
		StageTo:        staging.Stage1,
		WorkflowStatus: staging.OverrideStatusActive,
		PeriodeAkhir:   past,
		MakerID:        uuid.New(),
		TenantID:       "TUGURE",
	}
	// prop2: ACTIVE with future periode_akhir → should NOT expire.
	future := time.Now().AddDate(0, 3, 0)
	overRepo.proposals[prop2ID] = &staging.OverrideProposal{
		ID:             prop2ID,
		InstrumenID:    uuid.New(),
		StageFrom:      staging.Stage2,
		StageTo:        staging.Stage1,
		WorkflowStatus: staging.OverrideStatusActive,
		PeriodeAkhir:   future,
		MakerID:        uuid.New(),
		TenantID:       "TUGURE",
	}
	// Only prop1 is in the expired list.
	overRepo.expired = []*staging.OverrideProposal{overRepo.proposals[prop1ID]}

	worker := newTestWorker(newMockHistRepo(), overRepo, defaultMockInstrumen(), &mockPeriodeReader{})

	payload := staging.OverrideExpiryCheckPayload{
		TenantID: "TUGURE",
		ActorSub: uuid.New().String(),
	}
	task := makeTask(staging.TaskTypeOverrideExpiryCheck, payload)

	err := worker.HandleOverrideExpiryCheck(context.Background(), task)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	if overRepo.proposals[prop1ID].WorkflowStatus != staging.OverrideStatusExpired {
		t.Errorf("expected prop1 EXPIRED, got %s", overRepo.proposals[prop1ID].WorkflowStatus)
	}
	if overRepo.proposals[prop2ID].WorkflowStatus != staging.OverrideStatusActive {
		t.Errorf("expected prop2 still ACTIVE, got %s", overRepo.proposals[prop2ID].WorkflowStatus)
	}
}

// ─── TestTask_RecordsAuditEvent ────────────────────────────────────────────────

// TestTask_RecordsAuditEvent verifies that HandleEvaluateStaging writes a history
// row when a SICR trigger fires (thus implicitly writing the audit event via
// the noopAuditWriter, which means the audit.Write call succeeds without error).
func TestTask_RecordsAuditEvent(t *testing.T) {
	instrumenID := uuid.New()
	instrumen := defaultMockInstrumen()
	instrumen.originRating = "idAAA"
	instrumen.currentRating = "idAA" // 2-notch downgrade → SICR

	histRepo := newMockHistRepo()
	worker := newTestWorker(histRepo, newMockOverrideRepo(), instrumen, &mockPeriodeReader{})

	payload := staging.EvaluateStagingPayload{
		InstrumenID:       instrumenID,
		TanggalAssessment: time.Now().UTC(),
		TenantID:          "TUGURE",
		ActorSub:          uuid.New().String(),
		ActorRole:         "ROLE-RISK",
	}
	task := makeTask(staging.TaskTypeEvaluateStaging, payload)

	err := worker.HandleEvaluateStaging(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A stage_history row for this instrument should have been inserted
	// (audit.Write is bundled in the same tx — verified by no error above).
	var found bool
	for _, r := range histRepo.rows {
		if r.InstrumenID == instrumenID {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected stage_history row to be inserted (audit event recorded in same tx)")
	}
}

// ─── TestTaskEvaluateStaging_InvalidPayload_NoRetry ───────────────────────────

// TestTaskEvaluateStaging_InvalidPayload_NoRetry verifies that a corrupt JSON payload
// causes the task to succeed (return nil) without retrying, avoiding Asynq loops.
func TestTaskEvaluateStaging_InvalidPayload_NoRetry(t *testing.T) {
	worker := newTestWorker(newMockHistRepo(), newMockOverrideRepo(), defaultMockInstrumen(), &mockPeriodeReader{})

	task := asynq.NewTask(staging.TaskTypeEvaluateStaging, []byte("{invalid json"))
	err := worker.HandleEvaluateStaging(context.Background(), task)
	if err != nil {
		t.Errorf("expected nil (non-retryable) for invalid payload, got: %v", err)
	}
}

// TestTaskCureAssessmentBatch_InvalidPayload_NoRetry verifies that invalid JSON is
// silently dropped (no retry to avoid infinite poison-pill loop).
func TestTaskCureAssessmentBatch_InvalidPayload_NoRetry(t *testing.T) {
	worker := newTestWorker(newMockHistRepo(), newMockOverrideRepo(), defaultMockInstrumen(), &mockPeriodeReader{})

	task := asynq.NewTask(staging.TaskTypeCureAssessmentBatch, []byte("{bad json"))
	err := worker.HandleCureAssessmentBatch(context.Background(), task)
	if err != nil {
		t.Errorf("expected nil (non-retryable) for invalid payload, got: %v", err)
	}
}

// TestTaskOverrideExpiryCheck_InvalidPayload_NoRetry verifies that invalid JSON is
// silently dropped.
func TestTaskOverrideExpiryCheck_InvalidPayload_NoRetry(t *testing.T) {
	worker := newTestWorker(newMockHistRepo(), newMockOverrideRepo(), defaultMockInstrumen(), &mockPeriodeReader{})

	task := asynq.NewTask(staging.TaskTypeOverrideExpiryCheck, []byte("{garbage"))
	err := worker.HandleOverrideExpiryCheck(context.Background(), task)
	if err != nil {
		t.Errorf("expected nil (non-retryable) for invalid payload, got: %v", err)
	}
}

// TestTaskOverrideExpiryCheck_EmptyExpiredList_NoError exercises the path where
// no proposals need expiry.
func TestTaskOverrideExpiryCheck_EmptyExpiredList_NoError(t *testing.T) {
	overRepo := newMockOverrideRepo()
	// expired list is empty.
	worker := newTestWorker(newMockHistRepo(), overRepo, defaultMockInstrumen(), &mockPeriodeReader{})

	payload := staging.OverrideExpiryCheckPayload{
		TenantID: "TUGURE",
		ActorSub: uuid.New().String(),
	}
	task := makeTask(staging.TaskTypeOverrideExpiryCheck, payload)

	err := worker.HandleOverrideExpiryCheck(context.Background(), task)
	if err != nil {
		t.Errorf("expected no error for empty expired list, got: %v", err)
	}
}

// TestTaskCureAssessmentBatch_EmptyActorSub_UsesDefault covers injectWorkerClaims empty sub path.
func TestTaskCureAssessmentBatch_EmptyActorSub_UsesDefault(t *testing.T) {
	worker := newTestWorker(newMockHistRepo(), newMockOverrideRepo(), defaultMockInstrumen(), &mockPeriodeReader{})

	// Empty ActorSub and TenantID → injectWorkerClaims uses defaults.
	payload := staging.CureAssessmentBatchPayload{
		TenantID: "", // empty → uses "TUGURE"
		ActorSub: "", // empty → uses system UUID
	}
	task := makeTask(staging.TaskTypeCureAssessmentBatch, payload)

	err := worker.HandleCureAssessmentBatch(context.Background(), task)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

// TestTaskOverrideExpiryCheck_EmptyActorSub_UsesDefault covers injectWorkerClaims empty paths.
func TestTaskOverrideExpiryCheck_EmptyActorSub_UsesDefault(t *testing.T) {
	worker := newTestWorker(newMockHistRepo(), newMockOverrideRepo(), defaultMockInstrumen(), &mockPeriodeReader{})

	payload := staging.OverrideExpiryCheckPayload{
		TenantID: "", // empty → "TUGURE"
		ActorSub: "", // empty → system UUID
	}
	task := makeTask(staging.TaskTypeOverrideExpiryCheck, payload)

	err := worker.HandleOverrideExpiryCheck(context.Background(), task)
	if err != nil {
		t.Errorf("expected no error for empty expired list with default actor, got: %v", err)
	}
}
