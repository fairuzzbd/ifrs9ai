package gldelivery_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"blips-ifrs9.tugu-re.com/internal/audit"
	. "blips-ifrs9.tugu-re.com/internal/jrnl/gldelivery"
)

func TestNewGLDeliveryWorker_NilDelivery_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	reportRepo := NewReconReportRepo(db)
	mismatchRepo := NewReconMismatchRepo(db)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter()
	recon := NewReconciliationService(jurnalRepo, reportRepo, mismatchRepo, stub, aw, nil, DefaultConfig(), nil)

	assert.Panics(t, func() {
		NewGLDeliveryWorker(nil, recon, DefaultConfig(), nil)
	})
}

func TestNewGLDeliveryWorker_NilRecon_Panics(t *testing.T) {
	db, _ := newTestDB(t)
	jurnalRepo := NewJurnalGLRepo(db)
	dlqRepo := NewDLQRepo(db)
	aw := audit.NewWriter(db)
	stub := NewStubAdapter()
	delivery := NewDeliveryService(jurnalRepo, dlqRepo, stub, aw, nil, DefaultConfig(), nil)

	assert.Panics(t, func() {
		NewGLDeliveryWorker(delivery, nil, DefaultConfig(), nil)
	})
}

func TestHandleDeliverTask_BadPayload(t *testing.T) {
	_, delivery, _, _, recon, _ := newTestDelivery(t)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	task := asynq.NewTask(TaskGLDelivery, []byte("not-json"))
	err := worker.HandleDeliverTask(context.Background(), task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestHandleDeliverTask_BadUUID(t *testing.T) {
	_, delivery, _, _, recon, _ := newTestDelivery(t)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	payload, _ := json.Marshal(map[string]string{"jurnal_header_id": "not-a-uuid"})
	task := asynq.NewTask(TaskGLDelivery, payload)
	err := worker.HandleDeliverTask(context.Background(), task)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid jurnal_header_id")
}

func TestHandleReconcileDailyTask_EmptyPayload(t *testing.T) {
	_, delivery, _, _, recon, _ := newTestDelivery(t)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)

	// Empty payload → should default to yesterday, call TriggerAsync (will fail at DB level).
	task := asynq.NewTask(TaskGLReconcileDaily, []byte(""))
	// Just ensure no panic; DB error expected.
	err := worker.HandleReconcileDailyTask(context.Background(), task)
	_ = err
}

func TestNewDeliverTask_CreatesTask(t *testing.T) {
	headerID := uuid.New()
	task, err := NewDeliverTask(headerID)
	require.NoError(t, err)
	assert.Equal(t, TaskGLDelivery, task.Type())
	assert.Contains(t, string(task.Payload()), headerID.String())
}

func TestNewReconcileDailyTask_CreatesTask(t *testing.T) {
	date := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	task, err := NewReconcileDailyTask(date, "TUGURE")
	require.NoError(t, err)
	assert.Equal(t, TaskGLReconcileDaily, task.Type())
	assert.Contains(t, string(task.Payload()), "2026-06-15")
	assert.Contains(t, string(task.Payload()), "TUGURE")
}

func TestWorker_RegisterHandlers(t *testing.T) {
	_, delivery, _, _, recon, _ := newTestDelivery(t)
	worker := NewGLDeliveryWorker(delivery, recon, DefaultConfig(), nil)
	mux := asynq.NewServeMux()
	// Should not panic.
	worker.RegisterHandlers(mux)
}
