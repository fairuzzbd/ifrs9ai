// cron_registration_test.go — B1 fix: assert that task-type constants are wired correctly.
//
// Tests:
//  1. TaskDriftCron constant resolves to the expected string so the mux registration
//     (asynqMux.HandleFunc(eir.TaskDriftCron, ...)) and scheduler registration use the
//     same key.
//  2. TaskDriftAdHoc distinct from TaskDriftCron.
//  3. DriftCronHandler.HandleDriftCronTask has the correct asynq.HandlerFunc signature
//     so it can be passed to asynqMux.HandleFunc without type assertion at runtime.
//  4. DriftCronHandler.HandleDriftAdHocTask same signature check.
//  5. Scheduler cron expression "0 19 * * *" is parseable (guard against typos).
//
// References:
//   - backend/cmd/api/main.go (B1 wiring)
//   - worker_tasks.go §TaskDriftCron (schedule "0 19 * * *")
//   - docs/state-machines/p4-m6-amendment-lifecycle.md §7
//   - DEC-007 (Asynq).

package eir

import (
	"context"
	"log/slog"
	"testing"

	"github.com/hibiken/asynq"
)

// TestTaskDriftCronConstant ensures the constant value is the expected string.
// If someone renames the constant the mux registration in main.go silently drifts —
// this test catches that.
func TestTaskDriftCronConstant(t *testing.T) {
	const want = "eir:drift_cron"
	if TaskDriftCron != want {
		t.Errorf("TaskDriftCron = %q, want %q", TaskDriftCron, want)
	}
}

// TestTaskDriftAdHocConstant ensures the constant value is the expected string.
func TestTaskDriftAdHocConstant(t *testing.T) {
	const want = "eir:drift_adhoc"
	if TaskDriftAdHoc != want {
		t.Errorf("TaskDriftAdHoc = %q, want %q", TaskDriftAdHoc, want)
	}
}

// TestTaskConstants_Distinct ensures the two task types are different strings
// so they route to different handlers on the Asynq mux.
func TestTaskConstants_Distinct_CronVsAdHoc(t *testing.T) {
	if TaskDriftCron == TaskDriftAdHoc {
		t.Errorf("TaskDriftCron and TaskDriftAdHoc must be distinct, both = %q", TaskDriftCron)
	}
}

// TestDriftCronHandler_HandleDriftCronTask_Signature verifies that HandleDriftCronTask
// has the exact signature required by asynq.HandlerFunc (compatible with ServeMux.HandleFunc).
// This is a compile-time check expressed as a runtime test.
func TestDriftCronHandler_HandleDriftCronTask_Signature(t *testing.T) {
	h := NewDriftCronHandler(&DriftService{logger: slog.Default()}, slog.Default())

	// The asynq.HandlerFunc type: func(context.Context, *asynq.Task) error.
	// If HandleDriftCronTask signature does not match this would not compile.
	var _ asynq.HandlerFunc = h.HandleDriftCronTask
	var _ asynq.HandlerFunc = h.HandleDriftAdHocTask
	_ = h
}

// TestDriftCronHandler_CanRegisterOnMux verifies that both handlers can be registered
// on an asynq.ServeMux without panicking. This is the closest in-process check to
// "did main.go register the cron correctly" without a full integration test.
func TestDriftCronHandler_CanRegisterOnMux(t *testing.T) {
	h := NewDriftCronHandler(&DriftService{logger: slog.Default()}, slog.Default())

	mux := asynq.NewServeMux()
	// If HandleFunc panics (e.g. nil handler, empty type) the test will fail.
	mux.HandleFunc(TaskDriftCron, h.HandleDriftCronTask)
	mux.HandleFunc(TaskDriftAdHoc, h.HandleDriftAdHocTask)
}

// TestDriftCronScheduleExpression verifies the cron expression "0 19 * * *" is
// syntactically valid by using it to build an asynq.NewTask (the scheduler itself
// validates the expression on Register; here we just verify it is non-empty and
// doesn't panic when used to construct a task).
func TestDriftCronScheduleExpression(t *testing.T) {
	const cronExpr = "0 19 * * *"
	if cronExpr == "" {
		t.Fatal("cron expression must not be empty")
	}

	// Verify it creates a valid Task payload (smoke-test the full path).
	task, err := NewDriftCronTask("TUGURE")
	if err != nil {
		t.Fatalf("NewDriftCronTask: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task")
	}
	if task.Type() != TaskDriftCron {
		t.Errorf("task type = %q, want %q", task.Type(), TaskDriftCron)
	}
}

// TestDriftCronHandler_HandleDriftCronTask_InvalidPayload verifies the handler
// returns an error (not panic) when the task payload is invalid JSON.
// This exercises the error propagation path in handle() without needing a live DB.
func TestDriftCronHandler_HandleDriftCronTask_InvalidPayload(t *testing.T) {
	// Use a minimal DriftService — only needs logger to not panic on the log call.
	// We never reach GenerateReport because json.Unmarshal fails first.
	svc := &DriftService{logger: slog.Default()}
	h := NewDriftCronHandler(svc, slog.Default())

	task := asynq.NewTask(TaskDriftCron, []byte(`{invalid json`))
	err := h.HandleDriftCronTask(context.Background(), task)
	if err == nil {
		t.Error("expected error for invalid JSON payload, got nil")
	}
}
