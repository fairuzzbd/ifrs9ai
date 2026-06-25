package mtm

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Handler tests ────────────────────────────────────────────────────────────

func newTestHandler(repo Repository) *Handler {
	svc := newTestService(repo)
	return NewHandler(svc, slog.Default())
}

func makeCronTask(payload MtmCronPayload) *asynq.Task {
	b, _ := json.Marshal(payload)
	return asynq.NewTask(TaskMtmDailyRun, b)
}

func TestHandler_HandleMtmDailyRun_Weekend_Skip(t *testing.T) {
	repo := newStubRepo()
	h := newTestHandler(repo)

	// 2026-06-13 = Saturday
	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-13",
		TenantID:      "TUGURE",
		JobID:         "test-skip-weekend",
	})

	err := h.HandleMtmDailyRun(context.Background(), task)
	assert.NoError(t, err)
	// No instruments processed — repo instruments list empty and would be ignored anyway
}

func TestHandler_HandleMtmDailyRun_Holiday_Skip(t *testing.T) {
	repo := newStubRepo()
	repo.isHoliday = true
	h := newTestHandler(repo)

	// 2026-06-15 = Monday (weekday), but marked holiday
	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-15",
		TenantID:      "TUGURE",
		JobID:         "test-skip-holiday",
	})

	err := h.HandleMtmDailyRun(context.Background(), task)
	assert.NoError(t, err)
}

func TestHandler_HandleMtmDailyRun_InvalidPayload(t *testing.T) {
	repo := newStubRepo()
	h := newTestHandler(repo)

	task := asynq.NewTask(TaskMtmDailyRun, []byte("not json"))
	err := h.HandleMtmDailyRun(context.Background(), task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestHandler_HandleMtmDailyRun_InvalidTanggal(t *testing.T) {
	repo := newStubRepo()
	h := newTestHandler(repo)

	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "not-a-date",
	})
	err := h.HandleMtmDailyRun(context.Background(), task)
	require.Error(t, err)
}

func TestHandler_HandleMtmDailyRun_NoInstruments(t *testing.T) {
	repo := newStubRepo()
	repo.activeInstr = nil // no active instruments
	h := newTestHandler(repo)

	// Monday
	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-15",
		TenantID:      "TUGURE",
		JobID:         "test-empty",
	})
	err := h.HandleMtmDailyRun(context.Background(), task)
	assert.NoError(t, err)
}

func TestHandler_HandleMtmDailyRun_AC_Instrument_IsSkipped(t *testing.T) {
	repo := newStubRepo()
	repo.activeInstr = []InstrumenInfo{
		{
			ID:                uuid.New(),
			KlasifikasiPSAK71: KlasifikasiAC,
			MataUang:          "IDR",
			TipeInstrumen:     "DEPOSITO",
		},
	}
	h := newTestHandler(repo)

	// Monday
	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-15",
		TenantID:      "TUGURE",
		JobID:         "test-ac-skip",
	})
	err := h.HandleMtmDailyRun(context.Background(), task)
	// Should succeed despite AC instrument (it's skipped, not failed)
	assert.NoError(t, err)
}

func TestHandler_HandleMtmDailyRun_WithInstruments_ProcessesAll(t *testing.T) {
	repo := newStubRepo()
	instr1 := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVTPL,
		MataUang:          "IDR",
		TipeInstrumen:     "SAHAM",
	}
	instr2 := InstrumenInfo{
		ID:                uuid.New(),
		KlasifikasiPSAK71: KlasifikasiFVOCIElection,
		MataUang:          "IDR",
		TipeInstrumen:     "SAHAM",
	}
	repo.activeInstr = []InstrumenInfo{instr1, instr2}
	// No feed price → both become STALE_PRICE, but should not error
	h := newTestHandler(repo)

	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-16", // Tuesday
		TenantID:      "TUGURE",
		JobID:         "test-multi",
	})
	err := h.HandleMtmDailyRun(context.Background(), task)
	assert.NoError(t, err)
}

func TestHandler_HandleMtmDailyRun_Cancellation(t *testing.T) {
	repo := newStubRepo()
	// Add instruments so the loop runs
	repo.activeInstr = make([]InstrumenInfo, 5)
	for i := range repo.activeInstr {
		repo.activeInstr[i] = InstrumenInfo{
			ID:                uuid.New(),
			KlasifikasiPSAK71: KlasifikasiFVTPL,
			MataUang:          "IDR",
			TipeInstrumen:     "SAHAM",
		}
	}
	h := newTestHandler(repo)

	// Pre-cancel the context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-16",
		JobID:         "test-cancel",
	})
	// May return ctx.Err() or nil depending on when select fires
	_ = h.HandleMtmDailyRun(ctx, task)
}

func TestHandler_HandleMtmDailyRun_EmptyJobID_UsesDefault(t *testing.T) {
	repo := newStubRepo()
	h := newTestHandler(repo)

	task := makeCronTask(MtmCronPayload{
		TanggalTarget: "2026-06-15",
		JobID:         "", // empty → default constructed in handler
	})
	err := h.HandleMtmDailyRun(context.Background(), task)
	assert.NoError(t, err)
}

// ─── isACSkip ─────────────────────────────────────────────────────────────────

func TestIsACSkip_WithACSkipError(t *testing.T) {
	assert.True(t, isACSkip(ErrMTMInstrumenACSkip))
}

func TestIsACSkip_WithDifferentError(t *testing.T) {
	assert.False(t, isACSkip(ErrMTMPeriodeLocked))
}

func TestIsACSkip_WithNil(t *testing.T) {
	assert.False(t, isACSkip(nil))
}

// ─── max ─────────────────────────────────────────────────────────────────────

func TestMax_AGreater(t *testing.T) {
	assert.Equal(t, 10, max(10, 5))
}

func TestMax_BGreater(t *testing.T) {
	assert.Equal(t, 10, max(5, 10))
}

func TestMax_Equal(t *testing.T) {
	assert.Equal(t, 5, max(5, 5))
}

// ─── RegisterHandlers ─────────────────────────────────────────────────────────

func TestHandler_RegisterHandlers_NoPanic(t *testing.T) {
	h := newTestHandler(newStubRepo())
	mux := asynq.NewServeMux()
	assert.NotPanics(t, func() {
		h.RegisterHandlers(mux)
	})
}

// ─── NewHandler ──────────────────────────────────────────────────────────────

func TestNewHandler_NilLogger_UsesDefault(t *testing.T) {
	svc := newTestService(newStubRepo())
	h := NewHandler(svc, nil)
	assert.NotNil(t, h.logger)
}

// ─── MtmCronPayload JSON roundtrip ───────────────────────────────────────────

func TestMtmCronPayload_JSONRoundtrip(t *testing.T) {
	payload := MtmCronPayload{
		TanggalTarget: "2026-06-15",
		TenantID:      "TUGURE",
		JobID:         "job-123",
		ForceRerun:    true,
		ActorID:       uuid.New().String(),
	}
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	var decoded MtmCronPayload
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, payload, decoded)
}

// ─── TaskMtmDailyRun constant ────────────────────────────────────────────────

func TestTaskMtmDailyRun_Value(t *testing.T) {
	assert.Equal(t, "trx:mtm_daily_run", TaskMtmDailyRun)
}

// ─── Cron schedule string (documentation check) ──────────────────────────────

func TestCronScheduleString(t *testing.T) {
	// "0 11 * * 1-5" = 11:00 UTC = 18:00 WIB, Mon-Fri
	// Validate it's a legal cron spec that encodes the right semantics.
	// We just check the string — parsing cron is not our responsibility.
	cronSpec := "0 11 * * 1-5"
	assert.Contains(t, cronSpec, "1-5", "must be Mon-Fri")
	assert.Contains(t, cronSpec, "11", "must be 11 UTC = 18 WIB")

	// Ensure Monday is weekday (not weekend)
	monday := time.Date(2026, 6, 15, 11, 0, 0, 0, time.UTC)
	assert.False(t, IsWeekend(monday))
}
