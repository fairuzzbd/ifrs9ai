package pocidelta

// worker_test.go — Asynq worker handler tests.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func TestNewComputeDeltaTask_ValidPayload(t *testing.T) {
	task, err := NewComputeDeltaTask(uuid.New(), uuid.New(), "TUGURE", uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskComputeDeltaBatch {
		t.Fatalf("wrong task type: %s", task.Type())
	}
	var p ComputeDeltaPayload
	if err := json.Unmarshal(task.Payload(), &p); err != nil {
		t.Fatalf("cannot unmarshal payload: %v", err)
	}
	if p.CalcRunID == "" {
		t.Fatal("calc_run_id empty in payload")
	}
}

func TestHandleComputeDeltaBatch_InvalidPayload(t *testing.T) {
	repo := &stubRepo{calcRunStatus: "SEALED"}
	svc := makeService(repo)
	w := NewWorker(svc, nil, slog.Default())

	task := asynq.NewTask(TaskComputeDeltaBatch, []byte("not-json"))
	err := w.HandleComputeDeltaBatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid JSON payload")
	}
}

func TestHandleComputeDeltaBatch_InvalidCalcRunID(t *testing.T) {
	repo := &stubRepo{calcRunStatus: "SEALED"}
	svc := makeService(repo)
	w := NewWorker(svc, nil, slog.Default())

	p, _ := json.Marshal(ComputeDeltaPayload{
		CalcRunID: "not-a-uuid",
		ActorID:   uuid.New().String(),
		TenantID:  "TUGURE",
		JobID:     uuid.New().String(),
	})
	task := asynq.NewTask(TaskComputeDeltaBatch, p)
	err := w.HandleComputeDeltaBatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for invalid calc_run_id")
	}
}

func TestHandleComputeDeltaBatch_CalcRunNotSealed(t *testing.T) {
	repo := &stubRepo{calcRunStatus: "DRAFT"} // not SEALED/COMPLETED
	svc := makeService(repo)
	w := NewWorker(svc, nil, slog.Default())

	p, _ := json.Marshal(ComputeDeltaPayload{
		CalcRunID: uuid.New().String(),
		ActorID:   uuid.New().String(),
		TenantID:  "TUGURE",
		JobID:     uuid.New().String(),
	})
	task := asynq.NewTask(TaskComputeDeltaBatch, p)
	err := w.HandleComputeDeltaBatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for non-sealed run")
	}
}
