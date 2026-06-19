package kurs

// export_test.go — exports internal symbols for white-box testing.
// File is part of package `kurs` (not kurs_test), so it can access unexported symbols.
// Only compiled during `go test`. Must NOT be imported by production code.

import (
	"context"

	"github.com/hibiken/asynq"
)

// HandleJisdorFetchTaskPublic exposes the unexported handler for testing.
func (w *FxJisdorWorker) HandleJisdorFetchTaskPublic(ctx context.Context, t *asynq.Task) error {
	return w.HandleJisdorFetchTask(ctx, t)
}

// HandleUploadProcessTaskPublic exposes the unexported handler for testing.
func (w *FxJisdorWorker) HandleUploadProcessTaskPublic(ctx context.Context, t *asynq.Task) error {
	return w.HandleUploadProcessTask(ctx, t)
}

// NewRawTask constructs an asynq.Task with raw payload (for negative-path tests).
func NewRawTask(taskType string, payload []byte) *asynq.Task {
	return asynq.NewTask(taskType, payload)
}
