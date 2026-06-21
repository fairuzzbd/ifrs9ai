package akrualmaturity

// worker_test.go — Tests for Asynq worker handlers.
// Verifies holiday skip, happy path, DLQ fallback, task factory output.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Task factory tests ───────────────────────────────────────────────────────

func TestNewMaturityTask_Valid(t *testing.T) {
	tanggal := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	task, err := NewMaturityTask(tanggal, "job-001")
	require.NoError(t, err)
	assert.Equal(t, TaskMaturityProcess, task.Type())

	var p MaturityPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, "2026-06-20", p.Tanggal)
	assert.Equal(t, "job-001", p.JobID)
}

func TestNewAkrualTask_Valid(t *testing.T) {
	tanggal := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	task, err := NewAkrualTask(tanggal, "job-002")
	require.NoError(t, err)
	assert.Equal(t, TaskDailyAccrual, task.Type())

	var p AkrualPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, "2026-06-20", p.Tanggal)
	assert.Equal(t, "job-002", p.JobID)
}

func TestNewAmortisasiTask_Valid(t *testing.T) {
	tanggal := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	task, err := NewAmortisasiTask(tanggal, "job-003")
	require.NoError(t, err)
	assert.Equal(t, TaskAmortisasiPD, task.Type())

	var p AmortisasiPayload
	require.NoError(t, json.Unmarshal(task.Payload(), &p))
	assert.Equal(t, "2026-06-20", p.Tanggal)
}

// ─── CronEntries ─────────────────────────────────────────────────────────────

func TestCronEntries_ThreeEntries(t *testing.T) {
	entries := CronEntries()
	assert.Len(t, entries, 3)

	types := make([]string, 0, 3)
	for _, e := range entries {
		types = append(types, e.Task.Type())
	}
	assert.Contains(t, types, TaskMaturityProcess)
	assert.Contains(t, types, TaskDailyAccrual)
	assert.Contains(t, types, TaskAmortisasiPD)
}

func TestCronEntries_CronSpecs(t *testing.T) {
	entries := CronEntries()
	specsByType := map[string]string{}
	for _, e := range entries {
		specsByType[e.Task.Type()] = e.CronSpec
	}
	assert.Equal(t, CronMaturity, specsByType[TaskMaturityProcess])
	assert.Equal(t, CronAkrual, specsByType[TaskDailyAccrual])
	assert.Equal(t, CronAmortisasi, specsByType[TaskAmortisasiPD])
}

// ─── Worker.HandleMaturityProcess ────────────────────────────────────────────

func buildWorker(repo Repository) *Worker {
	svc := NewService(repo, NewJurnalPosterStub(slog.Default()), NewInstrumenStatusUpdaterStub(), nil, slog.Default())
	return NewWorker(svc, nil, slog.Default()) // nil redis — progress skipped
}

func makeTask(typ string, payload interface{}) *asynq.Task {
	b, _ := json.Marshal(payload)
	return asynq.NewTask(typ, b)
}

func TestHandleMaturityProcess_HolidaySkip(t *testing.T) {
	repo := &stubRepo{isHoliday: true}
	w := buildWorker(repo)

	task := makeTask(TaskMaturityProcess, MaturityPayload{
		Tanggal: "2026-06-20",
		JobID:   "test-job",
	})
	err := w.HandleMaturityProcess(context.Background(), task)
	require.NoError(t, err, "holiday skip must not return task error")
}

func TestHandleMaturityProcess_InvalidPayload(t *testing.T) {
	repo := &stubRepo{}
	w := buildWorker(repo)

	task := asynq.NewTask(TaskMaturityProcess, []byte("{invalid"))
	err := w.HandleMaturityProcess(context.Background(), task)
	require.Error(t, err, "invalid payload must return error")
}

func TestHandleMaturityProcess_InvalidTanggal(t *testing.T) {
	repo := &stubRepo{}
	w := buildWorker(repo)

	task := makeTask(TaskMaturityProcess, MaturityPayload{Tanggal: "not-a-date"})
	err := w.HandleMaturityProcess(context.Background(), task)
	require.Error(t, err)
}

func TestHandleMaturityProcess_PeriodeClosed_NoTaskError(t *testing.T) {
	// Periode closed is a service-level skip → worker logs WARN, no Asynq error
	repo := &stubRepo{
		isHoliday: false,
		periode: &PeriodeBuku{
			ID:            mustNewUUID(),
			StatusPeriode: "HARD_CLOSED",
		},
	}
	w := buildWorker(repo)

	task := makeTask(TaskMaturityProcess, MaturityPayload{
		Tanggal: "2026-06-20",
		JobID:   "test-job",
	})
	err := w.HandleMaturityProcess(context.Background(), task)
	require.NoError(t, err, "period closed is handled as skip, not fatal error")
}

func TestHandleMaturityProcess_HappyPath_NoJobID(t *testing.T) {
	// No instruments maturing today → success with 0 processed
	repo := &stubRepo{
		isHoliday:      false,
		periode:        openPeriode(),
		activeMaturity: nil,
	}
	w := buildWorker(repo)

	task := makeTask(TaskMaturityProcess, MaturityPayload{
		Tanggal: "2026-06-20",
		// No JobID — progress update skip
	})
	err := w.HandleMaturityProcess(context.Background(), task)
	require.NoError(t, err)
}

// ─── Worker.HandleDailyAccrual ────────────────────────────────────────────────

func TestHandleDailyAccrual_HolidaySkip(t *testing.T) {
	repo := &stubRepo{isHoliday: true}
	w := buildWorker(repo)

	task := makeTask(TaskDailyAccrual, AkrualPayload{
		Tanggal: "2026-06-20",
	})
	err := w.HandleDailyAccrual(context.Background(), task)
	require.NoError(t, err)
}

func TestHandleDailyAccrual_InvalidPayload(t *testing.T) {
	w := buildWorker(&stubRepo{})
	task := asynq.NewTask(TaskDailyAccrual, []byte("{bad"))
	err := w.HandleDailyAccrual(context.Background(), task)
	require.Error(t, err)
}

func TestHandleDailyAccrual_EmptyInstruments(t *testing.T) {
	repo := &stubRepo{
		isHoliday:      false,
		staleDays:      30,
		periode:        openPeriode(),
		activeAccruing: nil,
	}
	w := buildWorker(repo)

	task := makeTask(TaskDailyAccrual, AkrualPayload{Tanggal: "2026-06-20"})
	err := w.HandleDailyAccrual(context.Background(), task)
	require.NoError(t, err)
}

// ─── Worker.HandleAmortisasiPD ────────────────────────────────────────────────

func TestHandleAmortisasiPD_HolidaySkip(t *testing.T) {
	repo := &stubRepo{isHoliday: true}
	w := buildWorker(repo)

	task := makeTask(TaskAmortisasiPD, AmortisasiPayload{Tanggal: "2026-06-20"})
	err := w.HandleAmortisasiPD(context.Background(), task)
	require.NoError(t, err)
}

func TestHandleAmortisasiPD_InvalidPayload(t *testing.T) {
	w := buildWorker(&stubRepo{})
	task := asynq.NewTask(TaskAmortisasiPD, []byte("{bad"))
	err := w.HandleAmortisasiPD(context.Background(), task)
	require.Error(t, err)
}

func TestHandleAmortisasiPD_InvalidTanggal(t *testing.T) {
	w := buildWorker(&stubRepo{})
	task := makeTask(TaskAmortisasiPD, AmortisasiPayload{Tanggal: "invalid-date"})
	err := w.HandleAmortisasiPD(context.Background(), task)
	require.Error(t, err)
}

func TestHandleAmortisasiPD_EmptyInstruments(t *testing.T) {
	// Happy path: no instruments → success
	repo := &stubRepo{
		isHoliday:      false,
		periode:        openPeriode(),
		activeAccruing: nil,
	}
	w := buildWorker(repo)
	task := makeTask(TaskAmortisasiPD, AmortisasiPayload{Tanggal: "2026-06-20"})
	err := w.HandleAmortisasiPD(context.Background(), task)
	require.NoError(t, err)
}

func TestHandleAmortisasiPD_ServiceError_NoTaskError(t *testing.T) {
	// RunDailyAmortisasiCron returns error (e.g., holiday check DB down)
	// → worker logs + returns nil (non-fatal)
	repo := &stubRepo{holidayErr: errors.New("db down")}
	w := buildWorker(repo)
	task := makeTask(TaskAmortisasiPD, AmortisasiPayload{Tanggal: "2026-06-20"})
	err := w.HandleAmortisasiPD(context.Background(), task)
	require.NoError(t, err, "service-level error must not fail the Asynq task (logged + continued)")
}

func TestHandleDailyAccrual_InvalidTanggal(t *testing.T) {
	w := buildWorker(&stubRepo{})
	task := makeTask(TaskDailyAccrual, AkrualPayload{Tanggal: "not-a-date"})
	err := w.HandleDailyAccrual(context.Background(), task)
	require.Error(t, err)
}

func TestHandleDailyAccrual_ServiceError_NoTaskError(t *testing.T) {
	// RunDailyAkrualCron returns error → worker returns nil (non-fatal)
	repo := &stubRepo{holidayErr: errors.New("db down")}
	w := buildWorker(repo)
	task := makeTask(TaskDailyAccrual, AkrualPayload{Tanggal: "2026-06-20"})
	err := w.HandleDailyAccrual(context.Background(), task)
	require.NoError(t, err, "service-level error must not fail the Asynq task")
}

func TestHandleMaturityProcess_ServiceError_NoTaskError(t *testing.T) {
	// RunDailyMaturityCron returns error → worker returns nil (non-fatal)
	repo := &stubRepo{holidayErr: errors.New("db down")}
	w := buildWorker(repo)
	task := makeTask(TaskMaturityProcess, MaturityPayload{Tanggal: "2026-06-20"})
	err := w.HandleMaturityProcess(context.Background(), task)
	require.NoError(t, err, "service-level error must not fail the Asynq task")
}

// ─── Helper ──────────────────────────────────────────────────────────────────

func mustNewUUID() uuid.UUID {
	return uuid.MustParse("00000000-0000-0000-0000-000000000002")
}
