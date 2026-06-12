// worker_tasks_test.go — tests for Asynq task builders (P4-M6).
package eir

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestNewDriftCronTask_ValidJSON(t *testing.T) {
	task, err := NewDriftCronTask("TUGURE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskDriftCron {
		t.Errorf("expected type %s, got %s", TaskDriftCron, task.Type())
	}
	var payload DriftJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.TriggerSource != string(DriftTriggerCronDaily) {
		t.Errorf("expected CRON_DAILY, got %s", payload.TriggerSource)
	}
	if payload.TenantID != "TUGURE" {
		t.Errorf("expected TUGURE, got %s", payload.TenantID)
	}
	if payload.TriggeredBy != nil {
		t.Error("expected triggered_by nil for cron task")
	}
}

func TestNewDriftAdHocTask_ValidJSON(t *testing.T) {
	actorID := uuid.New()
	task, err := NewDriftAdHocTask("TUGURE", actorID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Type() != TaskDriftAdHoc {
		t.Errorf("expected type %s, got %s", TaskDriftAdHoc, task.Type())
	}
	var payload DriftJobPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.TriggerSource != string(DriftTriggerManualAdHoc) {
		t.Errorf("expected MANUAL_AD_HOC, got %s", payload.TriggerSource)
	}
	if payload.TriggeredBy == nil {
		t.Fatal("expected triggered_by to be set")
	}
	if *payload.TriggeredBy != actorID.String() {
		t.Errorf("expected %s, got %s", actorID.String(), *payload.TriggeredBy)
	}
}

func TestTaskConstants_NotEmpty(t *testing.T) {
	if TaskDriftCron == "" {
		t.Error("TaskDriftCron must not be empty")
	}
	if TaskDriftAdHoc == "" {
		t.Error("TaskDriftAdHoc must not be empty")
	}
	if TaskDriftCron == TaskDriftAdHoc {
		t.Error("TaskDriftCron and TaskDriftAdHoc must be distinct")
	}
}
