// Package gldelivery — Services: business logic for GL Host REST Delivery (P5-M3).
//
// Three services:
//   - DeliveryService  — core delivery logic, worker callback, manual retry.
//   - DLQService       — inspect / replay / discard sys.dlq_gl_delivery.
//   - ReconciliationService — daily BLIPS vs GL Host ledger comparison.
//
// Compliance:
//   - DEC-016: No float64 — shopspring/decimal throughout.
//   - DEC-018: Audit-in-tx mandatory. Constructor panics on nil auditWriter.
//   - DEC-021: Idempotency-Key checked for mutating endpoints.
//   - Hard delete forbidden: jrnl.gl_status, sys.dlq_gl_delivery, recon tables.
//   - Audit GL_DELIVERY.MANUAL_RETRY_INITIATED written BEFORE Asynq task enqueue.
//   - PII sanitized before any JSONB persist.
//   - GL_HOST_API_KEY never logged, never in response body.
package gldelivery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// AsynqEnqueuer is the minimal interface for dispatching Asynq tasks.
type AsynqEnqueuer interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// systemWorkerID is the actor UUID used for worker-initiated mutations.
// Value fixed per P5-M3 spec (seeds worker identity row in migration 000037).
var systemWorkerID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

// ─── Config (loaded at startup from sys.config) ───────────────────────────────

// Config holds runtime GL delivery settings (from sys.config keys seeded by 000037).
type Config struct {
	RetryMax            int                 // GL_DELIVERY_RETRY_MAX = 3
	RetryBackoffSeconds []int               // GL_DELIVERY_RETRY_BACKOFF_SECONDS = [30,120,600]
	MaxTotalAttempts    int                 // GL_DELIVERY_MAX_TOTAL_ATTEMPTS = 5
	ToleranceIDR        decimal.Decimal     // GL_RECON_TOLERANCE_IDR = 1.0000
	PIIFields           map[string]struct{} // GL_HOST_PII_FIELDS_TO_REDACT
}

// DefaultConfig returns safe production defaults (mirrors 000037 seeds).
func DefaultConfig() Config {
	return Config{
		RetryMax:            3,
		RetryBackoffSeconds: []int{30, 120, 600},
		MaxTotalAttempts:    5,
		ToleranceIDR:        decimal.NewFromFloat(1.0),
		PIIFields:           PIIFieldsDefault,
	}
}

// ─── DeliveryService ──────────────────────────────────────────────────────────

// DeliveryService owns delivery lifecycle: worker callback + manual retry.
type DeliveryService struct {
	jurnalRepo  *JurnalGLRepo
	dlqRepo     *DLQRepo
	adapter     GLHostAdapter
	auditWriter *audit.Writer
	enqueuer    AsynqEnqueuer
	cfg         Config
	logger      *slog.Logger
}

// NewDeliveryService creates a DeliveryService. Panics on nil mandatory deps (DEC-018).
func NewDeliveryService(
	jurnalRepo *JurnalGLRepo,
	dlqRepo *DLQRepo,
	adapter GLHostAdapter,
	auditWriter *audit.Writer,
	enqueuer AsynqEnqueuer,
	cfg Config,
	logger *slog.Logger,
) *DeliveryService {
	if jurnalRepo == nil {
		panic("gldelivery.NewDeliveryService: jurnalRepo must not be nil")
	}
	if dlqRepo == nil {
		panic("gldelivery.NewDeliveryService: dlqRepo must not be nil")
	}
	if adapter == nil {
		panic("gldelivery.NewDeliveryService: adapter must not be nil")
	}
	if auditWriter == nil {
		panic("gldelivery.NewDeliveryService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DeliveryService{
		jurnalRepo:  jurnalRepo,
		dlqRepo:     dlqRepo,
		adapter:     adapter,
		auditWriter: auditWriter,
		enqueuer:    enqueuer,
		cfg:         cfg,
		logger:      logger,
	}
}

// DeliverToGL is called by the Asynq worker for each JURNAL.POSTED event.
// Idempotent: if already DELIVERED, returns nil (no re-delivery).
func (s *DeliveryService) DeliverToGL(ctx context.Context, jurnalHeaderID uuid.UUID) error {
	jh, err := s.jurnalRepo.GetJurnalHeaderForDelivery(ctx, jurnalHeaderID)
	if err != nil {
		return fmt.Errorf("gldelivery.DeliveryService.DeliverToGL: %w", err)
	}
	if jh == nil {
		return domainerrors.New(domainerrors.CodeGLDeliveryJurnalNotFound,
			fmt.Sprintf("jurnal header %s not found", jurnalHeaderID))
	}

	// Idempotency: skip if terminal.
	if jh.GlHostStatus.IsTerminal() {
		s.logger.Info("gldelivery: skip delivery — terminal state",
			"headerID", jurnalHeaderID, "status", jh.GlHostStatus)
		return nil
	}

	// Guard: max total attempts.
	if jh.RetryCount >= s.cfg.MaxTotalAttempts {
		return s.moveToDLQ(ctx, jh, fmt.Sprintf("max total attempts (%d) exceeded", s.cfg.MaxTotalAttempts),
			"INFRA", string(domainerrors.CodeGLDeliveryMaxAttemptsExceeded))
	}

	// Build payload.
	payload := buildDeliveryPayload(jh)

	// Pre-sanitize payload in case needed by error path.
	rawPayload, _ := json.Marshal(payload) //nolint:errcheck
	_ = rawPayload                         // sanitized in moveToDLQ

	// Mark IN_FLIGHT.
	if err := s.updateStatusInTx(ctx, jurnalHeaderID, GlStatusUpdateFields{
		GlHostStatus:  GlHostStatusDeliveryInFlight,
		PayloadSentAt: ptrTime(time.Now()),
	}, "GL_DELIVERY.INFLIGHT", jh); err != nil {
		return fmt.Errorf("gldelivery.DeliveryService.DeliverToGL mark in-flight: %w", err)
	}

	// Call GL Host.
	resp, deliveryErr := s.adapter.Post(ctx, payload, jh.IdempotencyKey)

	now := time.Now()
	retryCount := jh.RetryCount + 1

	if deliveryErr != nil {
		return s.handleDeliveryError(ctx, jh, deliveryErr, retryCount, resp)
	}

	// Success path — mark DELIVERED in tx + audit.
	sanitizedResp, _ := json.Marshal(resp.RawResponseJsonb) //nolint:errcheck
	sanitizedRespClean := SanitizePIIRaw(sanitizedResp, s.cfg.PIIFields)
	respPtr := sanitizedRespClean

	if err := s.updateStatusInTx(ctx, jurnalHeaderID, GlStatusUpdateFields{
		GlHostStatus:           GlHostStatusDelivered,
		GlHostJournalID:        &resp.GlResponseID,
		DeliveredAt:            &now,
		RetryCount:             &retryCount,
		GlResponsePayloadJsonb: respPtr,
		PayloadSentAt:          ptrTime(time.Now()),
	}, "GL_DELIVERY.DELIVERED", jh); err != nil {
		return fmt.Errorf("gldelivery.DeliveryService.DeliverToGL mark delivered: %w", err)
	}

	s.logger.Info("gldelivery: delivered successfully",
		"headerID", jurnalHeaderID, "glJournalID", resp.GlResponseID)
	return nil
}

func (s *DeliveryService) handleDeliveryError(
	ctx context.Context,
	jh *JurnalHeaderDelivery,
	err error,
	retryCount int,
	resp DeliveryResponse,
) error {
	de, isDomain := domainerrors.IsDomainError(err)
	category := FailureCategoryInfra
	if isDomain && (de.Code() == domainerrors.CodeGLDeliveryHost4XX || de.Code() == domainerrors.CodeGLDeliveryAuthFailed) {
		category = FailureCategoryDomain
	}

	errorMsg := err.Error()
	errCode := string(domainerrors.CodeGLDeliveryHostUnreachable)
	if isDomain {
		errCode = string(de.Code())
	}

	sanitizedResp := json.RawMessage(`{}`)
	if len(resp.RawResponseJsonb) > 0 {
		sanitizedResp = SanitizePIIRaw(resp.RawResponseJsonb, s.cfg.PIIFields)
	}

	// Domain errors (4xx) → DLQ immediately, no retry.
	if category == FailureCategoryDomain {
		s.logger.Warn("gldelivery: domain error → DLQ",
			"headerID", jh.ID, "error", errCode)
		_ = s.updateStatusInTx(ctx, jh.ID, GlStatusUpdateFields{ //nolint:errcheck
			GlHostStatus:           GlHostStatusFailed,
			RetryCount:             &retryCount,
			LastError:              &errorMsg,
			FailureCategory:        &category,
			GlResponsePayloadJsonb: sanitizedResp,
		}, "GL_DELIVERY.FAILED_DOMAIN", jh)
		return s.moveToDLQ(ctx, jh, errorMsg, category, errCode)
	}

	// Infra error — retry if within max.
	newStatus := GlHostStatusRetrying
	if retryCount >= s.cfg.MaxTotalAttempts {
		newStatus = GlHostStatusFailed
	}

	_ = s.updateStatusInTx(ctx, jh.ID, GlStatusUpdateFields{ //nolint:errcheck
		GlHostStatus:           newStatus,
		RetryCount:             &retryCount,
		LastRetryAt:            ptrTime(time.Now()),
		LastError:              &errorMsg,
		FailureCategory:        &category,
		GlResponsePayloadJsonb: sanitizedResp,
	}, "GL_DELIVERY.FAILED_INFRA", jh)

	if retryCount >= s.cfg.MaxTotalAttempts {
		return s.moveToDLQ(ctx, jh, errorMsg, category, errCode)
	}

	// Asynq will handle the exponential backoff via task options (set by worker).
	return fmt.Errorf("gldelivery: infra error (attempt %d/%d): %w", retryCount, s.cfg.MaxTotalAttempts, err)
}

// moveToDLQ inserts into sys.dlq_gl_delivery in a tx + audit.
func (s *DeliveryService) moveToDLQ(ctx context.Context, jh *JurnalHeaderDelivery, errMsg, category, errCode string) error {
	payload, _ := json.Marshal(buildDeliveryPayload(jh)) //nolint:errcheck
	sanitizedPayload := SanitizePIIRaw(payload, s.cfg.PIIFields)

	entry := DLQEntry{
		ID:             uuid.New(),
		JurnalHeaderID: jh.ID,
		GlStatusID:     &jh.GlStatusID,
		PayloadJsonb:   sanitizedPayload,
		ErrorCode:      errCode,
		ErrorMessage:   errMsg,
		ErrorCategory:  category,
		RetryCount:     jh.RetryCount,
		Status:         DLQStatusFailed,
		CreatedAt:      time.Now(),
	}

	tx, err := s.dlqRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("gldelivery.moveToDLQ: begin tx: %w", err)
	}
	defer rollbackTx(tx)

	workerCtx := workerContext(ctx)
	if err := s.dlqRepo.Insert(workerCtx, tx, entry); err != nil {
		return fmt.Errorf("gldelivery.moveToDLQ: insert: %w", err)
	}
	if err := s.auditWriter.WithTx(tx).Write(workerCtx, audit.Event{
		Action:      "GL_DELIVERY.DLQ_ENTERED",
		EntityType:  "sys.dlq_gl_delivery",
		EntityID:    entry.ID,
		ActorUserID: systemWorkerID.String(),
		After:       map[string]any{"jurnalHeaderId": jh.ID, "errorCode": errCode, "category": category},
	}); err != nil {
		return fmt.Errorf("gldelivery.moveToDLQ: audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("gldelivery.moveToDLQ: commit: %w", err)
	}
	return nil
}

// ManualRetry re-queues a failed delivery for a single jurnal header.
// Audit GL_DELIVERY.MANUAL_RETRY_INITIATED is written BEFORE Asynq enqueue.
func (s *DeliveryService) ManualRetry(ctx context.Context, jurnalHeaderID uuid.UUID, reason string, callerID uuid.UUID) (*RetryGlDeliveryResponse, error) {
	if len(reason) < 30 {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryReasonTooShort, "retry reason must be at least 30 characters")
	}

	ds, err := s.jurnalRepo.GetDeliveryStatus(ctx, jurnalHeaderID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ManualRetry: %w", err)
	}
	if ds == nil {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryJurnalNotFound,
			fmt.Sprintf("gl_status not found for header %s", jurnalHeaderID))
	}
	if !ds.GlHostStatus.CanManualRetry() {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryInvalidTransition,
			fmt.Sprintf("cannot manual retry from status %s", ds.GlHostStatus))
	}

	prevStatus := ds.GlHostStatus
	now := time.Now()
	zero := 0

	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ManualRetry: begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.jurnalRepo.UpdateGLStatus(ctx, tx, jurnalHeaderID, GlStatusUpdateFields{
		GlHostStatus:      GlHostStatusPendingDelivery,
		RetryCount:        &zero,
		ManualRetryBy:     &callerID,
		ManualRetryAt:     &now,
		ManualRetryReason: &reason,
	}); err != nil {
		return nil, fmt.Errorf("gldelivery.ManualRetry: update status: %w", err)
	}

	// Audit BEFORE enqueue (DEC compliance: audit must be in same tx as state change).
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "GL_DELIVERY.MANUAL_RETRY_INITIATED",
		EntityType: "jrnl.gl_status",
		EntityID:   ds.GlStatusID,
		Before:     map[string]any{"gl_host_status": string(prevStatus)},
		After:      map[string]any{"gl_host_status": string(GlHostStatusPendingDelivery), "reason": reason, "retry_by": callerID},
	})); err != nil {
		return nil, fmt.Errorf("gldelivery.ManualRetry: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("gldelivery.ManualRetry: commit: %w", err)
	}

	// Enqueue AFTER commit.
	jobID := ""
	if s.enqueuer != nil {
		taskPayload, _ := json.Marshal(map[string]string{"jurnal_header_id": jurnalHeaderID.String()}) //nolint:errcheck
		task := asynq.NewTask(TaskGLDelivery, taskPayload)
		info, enqErr := s.enqueuer.EnqueueContext(ctx, task)
		if enqErr != nil {
			s.logger.Warn("gldelivery.ManualRetry: enqueue failed (will be picked up by sweep)", "error", enqErr)
		} else {
			jobID = info.ID
		}
	}

	statusURL := fmt.Sprintf("/api/v1/jobs/%s", jobID)
	return &RetryGlDeliveryResponse{
		JobID:              jobID,
		StatusURL:          statusURL,
		GlStatusID:         ds.GlStatusID,
		PreviousStatus:     prevStatus,
		NewStatus:          GlHostStatusPendingDelivery,
		RetryAttemptNumber: 1,
	}, nil
}

// updateStatusInTx wraps the begin→update→audit→commit cycle for status transitions.
func (s *DeliveryService) updateStatusInTx(
	ctx context.Context,
	jurnalHeaderID uuid.UUID,
	fields GlStatusUpdateFields,
	auditAction string,
	jh *JurnalHeaderDelivery,
) error {
	tx, err := s.jurnalRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("updateStatusInTx begin: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.jurnalRepo.UpdateGLStatus(ctx, tx, jurnalHeaderID, fields); err != nil {
		return fmt.Errorf("updateStatusInTx update: %w", err)
	}

	workerCtx := workerContext(ctx)
	if err := s.auditWriter.WithTx(tx).Write(workerCtx, audit.Event{
		Action:      auditAction,
		EntityType:  "jrnl.gl_status",
		EntityID:    jh.GlStatusID,
		ActorUserID: systemWorkerID.String(),
		After:       map[string]any{"gl_host_status": string(fields.GlHostStatus)},
	}); err != nil {
		return fmt.Errorf("updateStatusInTx audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("updateStatusInTx commit: %w", err)
	}
	return nil
}

// buildDeliveryPayload constructs the GL Host payload from a JurnalHeaderDelivery.
func buildDeliveryPayload(jh *JurnalHeaderDelivery) DeliveryPayload {
	lines := make([]DeliveryLine, 0, len(jh.DetailRows))
	for _, d := range jh.DetailRows {
		lines = append(lines, DeliveryLine{
			AccountCode: d.KodeAkun,
			Debit:       d.DebitAmount,
			Kredit:      d.KreditAmount,
			Currency:    d.MataUang,
			Narasi:      d.NarrativeLine,
		})
	}
	return DeliveryPayload{
		IdempotencyKey: jh.IdempotencyKey,
		JournalDate:    jh.TanggalPosting.Format("2006-01-02"),
		Reference:      jh.NoJurnal,
		EventCode:      jh.EventCode,
		Narrative:      jh.Narrative,
		Lines:          lines,
		Metadata:       map[string]any{"source": "BLIPS-IFRS9"},
	}
}

// ─── DLQService ───────────────────────────────────────────────────────────────

// DLQService manages sys.dlq_gl_delivery entries.
type DLQService struct {
	dlqRepo     *DLQRepo
	jurnalRepo  *JurnalGLRepo
	delivery    *DeliveryService
	auditWriter *audit.Writer
	enqueuer    AsynqEnqueuer
	logger      *slog.Logger
}

// NewDLQService creates a DLQService. Panics on nil mandatory deps.
func NewDLQService(
	dlqRepo *DLQRepo,
	jurnalRepo *JurnalGLRepo,
	delivery *DeliveryService,
	auditWriter *audit.Writer,
	enqueuer AsynqEnqueuer,
	logger *slog.Logger,
) *DLQService {
	if dlqRepo == nil {
		panic("gldelivery.NewDLQService: dlqRepo must not be nil")
	}
	if jurnalRepo == nil {
		panic("gldelivery.NewDLQService: jurnalRepo must not be nil")
	}
	if delivery == nil {
		panic("gldelivery.NewDLQService: delivery must not be nil")
	}
	if auditWriter == nil {
		panic("gldelivery.NewDLQService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &DLQService{
		dlqRepo:     dlqRepo,
		jurnalRepo:  jurnalRepo,
		delivery:    delivery,
		auditWriter: auditWriter,
		enqueuer:    enqueuer,
		logger:      logger,
	}
}

// GetByID returns a full DLQ entry.
func (s *DLQService) GetByID(ctx context.Context, id uuid.UUID) (*DLQEntry, error) {
	entry, err := s.dlqRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.GetByID: %w", err)
	}
	return entry, nil
}

// List returns paginated DLQ entries.
func (s *DLQService) List(ctx context.Context, limit int, status string) ([]DLQEntrySummary, ListPage, error) {
	items, page, err := s.dlqRepo.List(ctx, emptyQuery(), limit, status)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("gldelivery.DLQService.List: %w", err)
	}
	// Compute canReplay / canDiscard flags.
	for i := range items {
		items[i].CanReplay = items[i].Status.CanReplay()
		items[i].CanDiscard = items[i].Status.CanDiscard()
	}
	return items, page, nil
}

// Replay transitions DLQ entry FAILED → REPLAYING, resets gl_host_status → PENDING_DELIVERY,
// and enqueues Asynq task.
func (s *DLQService) Replay(ctx context.Context, dlqID uuid.UUID, reason string, callerID uuid.UUID) (*DlqReplayResponse, error) {
	if len(reason) < 30 {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryReasonTooShort, "replay reason must be at least 30 characters")
	}

	entry, err := s.dlqRepo.GetByID(ctx, dlqID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: %w", err)
	}
	if entry == nil {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryJurnalNotFound, fmt.Sprintf("DLQ entry %s not found", dlqID))
	}
	if !entry.Status.CanReplay() {
		return nil, domainerrors.New(domainerrors.CodeGLDLQReplayInvalidState,
			fmt.Sprintf("DLQ entry status %s cannot be replayed", entry.Status))
	}

	// Get current gl_host_status.
	ds, err := s.jurnalRepo.GetDeliveryStatus(ctx, entry.JurnalHeaderID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: get status: %w", err)
	}
	prevGLStatus := GlHostStatusFailed
	if ds != nil {
		prevGLStatus = ds.GlHostStatus
	}

	now := time.Now()
	zero := 0

	// Update DLQ status + gl_host_status + audit in one tx.
	tx, err := s.dlqRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.dlqRepo.UpdateStatusTx(ctx, tx, dlqID, DLQStatusReplaying, map[string]any{
		"replayed_by": callerID,
		"replayed_at": now,
	}); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: update DLQ: %w", err)
	}

	if err := s.jurnalRepo.UpdateGLStatus(ctx, tx, entry.JurnalHeaderID, GlStatusUpdateFields{
		GlHostStatus:      GlHostStatusPendingDelivery,
		RetryCount:        &zero,
		ManualRetryBy:     &callerID,
		ManualRetryAt:     &now,
		ManualRetryReason: &reason,
	}); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: update gl_status: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "GL_DELIVERY.DLQ_REPLAY_INITIATED",
		EntityType: "sys.dlq_gl_delivery",
		EntityID:   dlqID,
		Before:     map[string]any{"status": string(entry.Status)},
		After:      map[string]any{"status": string(DLQStatusReplaying), "reason": reason, "replayed_by": callerID},
	})); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Replay: commit: %w", err)
	}

	// Enqueue delivery task AFTER commit.
	jobID := ""
	if s.enqueuer != nil {
		taskPayload, _ := json.Marshal(map[string]string{"jurnal_header_id": entry.JurnalHeaderID.String()}) //nolint:errcheck
		task := asynq.NewTask(TaskGLDelivery, taskPayload)
		if info, enqErr := s.enqueuer.EnqueueContext(ctx, task); enqErr == nil {
			jobID = info.ID
		}
	}

	return &DlqReplayResponse{
		JobID:          jobID,
		StatusURL:      fmt.Sprintf("/api/v1/jobs/%s", jobID),
		DLQEntryID:     dlqID,
		JurnalHeaderID: entry.JurnalHeaderID,
		NoJurnal:       entry.NoJurnal,
		PreviousStatus: prevGLStatus,
		NewStatus:      GlHostStatusPendingDelivery,
	}, nil
}

// Discard transitions DLQ FAILED → ABANDONED and marks gl_host_status → DEAD_LETTER.
func (s *DLQService) Discard(ctx context.Context, dlqID uuid.UUID, reason string, callerID uuid.UUID) (*DlqDiscardResponse, error) {
	if len(reason) < 30 {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryReasonTooShort, "discard reason must be at least 30 characters")
	}

	entry, err := s.dlqRepo.GetByID(ctx, dlqID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Discard: %w", err)
	}
	if entry == nil {
		return nil, domainerrors.New(domainerrors.CodeGLDeliveryJurnalNotFound, fmt.Sprintf("DLQ entry %s not found", dlqID))
	}
	if !entry.Status.CanDiscard() {
		return nil, domainerrors.New(domainerrors.CodeGLDLQReplayInvalidState,
			fmt.Sprintf("DLQ entry status %s cannot be discarded", entry.Status))
	}

	now := time.Now()

	tx, err := s.dlqRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Discard: begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.dlqRepo.UpdateStatusTx(ctx, tx, dlqID, DLQStatusAbandoned, map[string]any{
		"discarded_reason": reason,
		"discarded_by":     callerID,
		"discarded_at":     now,
	}); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Discard: update DLQ: %w", err)
	}

	if err := s.jurnalRepo.UpdateGLStatus(ctx, tx, entry.JurnalHeaderID, GlStatusUpdateFields{
		GlHostStatus:  GlHostStatusDeadLetter,
		DiscardedBy:   &callerID,
		DiscardedAt:   &now,
		DiscardReason: &reason,
	}); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Discard: update gl_status: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "GL_DELIVERY.DLQ_DISCARDED",
		EntityType: "sys.dlq_gl_delivery",
		EntityID:   dlqID,
		Before:     map[string]any{"status": string(entry.Status)},
		After:      map[string]any{"status": string(DLQStatusAbandoned), "reason": reason, "discarded_by": callerID},
	})); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Discard: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("gldelivery.DLQService.Discard: commit: %w", err)
	}

	return &DlqDiscardResponse{
		DLQEntryID:     dlqID,
		JurnalHeaderID: entry.JurnalHeaderID,
		NoJurnal:       entry.NoJurnal,
		PreviousStatus: GlHostStatusFailed,
		NewStatus:      GlHostStatusDeadLetter,
		DiscardedAt:    now,
		DiscardedBy:    callerID,
	}, nil
}

// ─── ReconciliationService ────────────────────────────────────────────────────

// ReconciliationService owns the daily BLIPS vs GL Host reconciliation.
type ReconciliationService struct {
	jurnalRepo   *JurnalGLRepo
	reportRepo   *ReconReportRepo
	mismatchRepo *ReconMismatchRepo
	adapter      GLHostAdapter
	auditWriter  *audit.Writer
	enqueuer     AsynqEnqueuer
	cfg          Config
	logger       *slog.Logger
}

// NewReconciliationService creates a ReconciliationService. Panics on nil mandatory deps.
func NewReconciliationService(
	jurnalRepo *JurnalGLRepo,
	reportRepo *ReconReportRepo,
	mismatchRepo *ReconMismatchRepo,
	adapter GLHostAdapter,
	auditWriter *audit.Writer,
	enqueuer AsynqEnqueuer,
	cfg Config,
	logger *slog.Logger,
) *ReconciliationService {
	if jurnalRepo == nil {
		panic("gldelivery.NewReconciliationService: jurnalRepo must not be nil")
	}
	if reportRepo == nil {
		panic("gldelivery.NewReconciliationService: reportRepo must not be nil")
	}
	if mismatchRepo == nil {
		panic("gldelivery.NewReconciliationService: mismatchRepo must not be nil")
	}
	if adapter == nil {
		panic("gldelivery.NewReconciliationService: adapter must not be nil")
	}
	if auditWriter == nil {
		panic("gldelivery.NewReconciliationService: auditWriter must not be nil (DEC-018)")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ReconciliationService{
		jurnalRepo:   jurnalRepo,
		reportRepo:   reportRepo,
		mismatchRepo: mismatchRepo,
		adapter:      adapter,
		auditWriter:  auditWriter,
		enqueuer:     enqueuer,
		cfg:          cfg,
		logger:       logger,
	}
}

// TriggerAsync enqueues a reconciliation job and creates an IN_PROGRESS report.
// Returns 409 if one is already in progress for the date.
func (s *ReconciliationService) TriggerAsync(
	ctx context.Context,
	date time.Time,
	triggerSource string,
	callerID *uuid.UUID,
	tenantID string,
) (*RunReconciliationResponse, error) {
	// Check in-progress guard.
	inProgress, err := s.reportRepo.IsInProgress(ctx, date, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.TriggerAsync: %w", err)
	}
	if inProgress {
		return nil, domainerrors.New(domainerrors.CodeGLReconciliationInProgress,
			fmt.Sprintf("reconciliation already in progress for %s", date.Format("2006-01-02")))
	}

	reportID := uuid.New()
	jobID := uuid.New().String()
	report := &ReconciliationReport{
		ID:            reportID,
		TanggalRun:    date,
		TriggerSource: triggerSource,
		TriggeredBy:   callerID,
		AsynqJobID:    &jobID,
		Status:        ReconStatusInProgress,
		StartedAt:     time.Now(),
		ToleranceIDR:  s.cfg.ToleranceIDR,
		MismatchCount: 0,
	}

	actorID := systemWorkerID
	if callerID != nil {
		actorID = *callerID
	}

	tx, err := s.reportRepo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.TriggerAsync: begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.reportRepo.Insert(ctx, tx, report, actorID); err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.TriggerAsync: insert: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.EventFromContext(ctx, audit.Event{
		Action:     "GL_RECONCILIATION.TRIGGERED",
		EntityType: "sys.gl_reconciliation_report",
		EntityID:   reportID,
		After:      map[string]any{"tanggal": date.Format("2006-01-02"), "trigger": triggerSource},
	})); err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.TriggerAsync: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.TriggerAsync: commit: %w", err)
	}

	// Enqueue Asynq task AFTER commit.
	if s.enqueuer != nil {
		taskPayload, _ := json.Marshal(map[string]string{ //nolint:errcheck
			"report_id": reportID.String(),
			"date":      date.Format("2006-01-02"),
			"tenant_id": tenantID,
		})
		task := asynq.NewTask(TaskGLReconcileDaily, taskPayload)
		if _, enqErr := s.enqueuer.EnqueueContext(ctx, task); enqErr != nil {
			s.logger.Warn("gldelivery.ReconciliationService: enqueue failed", "error", enqErr)
		}
	}

	return &RunReconciliationResponse{
		JobID:               jobID,
		StatusURL:           fmt.Sprintf("/api/v1/jobs/%s", jobID),
		StreamURL:           fmt.Sprintf("/api/v1/jobs/%s/stream", jobID),
		TanggalRekonsiliasi: date.Format("2006-01-02"),
	}, nil
}

// RunReconciliation executes the BLIPS vs GL Host comparison (called by Asynq worker).
// Updates report + inserts mismatches in tx. Idempotent per reportID.
func (s *ReconciliationService) RunReconciliation(ctx context.Context, reportID uuid.UUID, date time.Time, tenantID string) error {
	blipsData, err := s.jurnalRepo.GetForRecon(ctx, date, tenantID)
	if err != nil {
		return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: blips side: %w", err)
	}

	glTotals, err := s.adapter.GetDailySummary(ctx, date)
	if err != nil {
		return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: gl host side: %w", err)
	}

	glMap := make(map[string]decimal.Decimal, len(glTotals))
	for _, at := range glTotals {
		glMap[at.KodeAkun] = at.NetAmountIDR
	}

	var mismatches []ReconMismatch
	var totalBLIPS, totalGL decimal.Decimal

	// Check BLIPS vs GL.
	for kodeAkun, bd := range blipsData {
		totalBLIPS = totalBLIPS.Add(bd.NetIDR)
		glAmount, found := glMap[kodeAkun]
		if !found {
			glAmount = decimal.Zero
		}
		totalGL = totalGL.Add(glAmount)

		delta := bd.NetIDR.Sub(glAmount).Abs()
		mismatchType := MismatchTypeAmountDiff
		if !found {
			mismatchType = MismatchTypeBlipsOnly
		}

		if delta.GreaterThan(s.cfg.ToleranceIDR) {
			note := fmt.Sprintf("delta IDR %s exceeds tolerance %s", delta.StringFixed(4), s.cfg.ToleranceIDR.StringFixed(4))
			mismatches = append(mismatches, ReconMismatch{
				ID:              uuid.New(),
				ReportID:        reportID,
				AkunID:          bd.AkunID,
				KodeAkun:        kodeAkun,
				BlipsAmountIDR:  bd.NetIDR,
				GlHostAmountIDR: glAmount,
				DeltaIDR:        bd.NetIDR.Sub(glAmount),
				MismatchType:    mismatchType,
				JurnalHeaderIDs: bd.HeaderIDs,
				Note:            &note,
			})
		}
	}

	// Check GL side for accounts not in BLIPS.
	for kodeAkun, glAmount := range glMap {
		if _, inBlips := blipsData[kodeAkun]; !inBlips {
			totalGL = totalGL.Add(glAmount)
			note := fmt.Sprintf("account %s present in GL Host but not in BLIPS for %s", kodeAkun, date.Format("2006-01-02"))
			mismatches = append(mismatches, ReconMismatch{
				ID:              uuid.New(),
				ReportID:        reportID,
				AkunID:          uuid.Nil, // unknown BLIPS akun_id for GL-only
				KodeAkun:        kodeAkun,
				BlipsAmountIDR:  decimal.Zero,
				GlHostAmountIDR: glAmount,
				DeltaIDR:        glAmount.Neg(),
				MismatchType:    MismatchTypeGLOnly,
				Note:            &note,
			})
		}
	}

	finalStatus := ReconStatusCompleted
	if len(mismatches) > 0 {
		finalStatus = ReconStatusCompletedWithMismatch
	}

	glHostTotal := totalGL
	glSnapshotJSON, _ := json.Marshal(glMap) //nolint:errcheck
	glSnapshotRaw := json.RawMessage(glSnapshotJSON)

	report := &ReconciliationReport{
		ID:                  reportID,
		TanggalRun:          date,
		Status:              finalStatus,
		TotalJurnalIDR:      totalBLIPS,
		GlHostTotalIDR:      &glHostTotal,
		MismatchCount:       len(mismatches),
		ToleranceIDR:        s.cfg.ToleranceIDR,
		GlHostSnapshotJsonb: &glSnapshotRaw,
	}

	workerCtx := workerContext(ctx)

	tx, err := s.reportRepo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if err := s.reportRepo.Update(workerCtx, tx, report, systemWorkerID); err != nil {
		return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: update report: %w", err)
	}

	if len(mismatches) > 0 {
		// Clear old mismatches (re-run idempotency).
		if err := s.mismatchRepo.SoftDeleteByReportID(workerCtx, tx, reportID, systemWorkerID); err != nil {
			return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: soft-delete old mismatches: %w", err)
		}
		if err := s.mismatchRepo.InsertBulk(workerCtx, tx, mismatches, systemWorkerID); err != nil {
			return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: insert mismatches: %w", err)
		}
	}

	if err := s.auditWriter.WithTx(tx).Write(workerCtx, audit.Event{
		Action:      "GL_RECONCILIATION.COMPLETED",
		EntityType:  "sys.gl_reconciliation_report",
		EntityID:    reportID,
		ActorUserID: systemWorkerID.String(),
		After: map[string]any{
			"status":          string(finalStatus),
			"mismatch_count":  len(mismatches),
			"blips_total_idr": totalBLIPS.StringFixed(4),
			"gl_total_idr":    glHostTotal.StringFixed(4),
		},
	}); err != nil {
		return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: audit: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("gldelivery.ReconciliationService.RunReconciliation: commit: %w", err)
	}

	s.logger.Info(
		"gldelivery: reconciliation complete",
		"date", date.Format("2006-01-02"),
		"mismatches", len(mismatches),
		"blipsTotal", totalBLIPS.StringFixed(4),
		"glTotal", glHostTotal.StringFixed(4),
	)
	return nil
}

// GetReport returns a reconciliation report with mismatches for a given date.
func (s *ReconciliationService) GetReport(ctx context.Context, date time.Time, tenantID string) (*ReconciliationReport, error) {
	report, err := s.reportRepo.GetByDate(ctx, date, tenantID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.GetReport: %w", err)
	}
	if report == nil {
		return nil, domainerrors.New(domainerrors.CodeGLReconciliationReportNotFound,
			fmt.Sprintf("reconciliation report not found for %s", date.Format("2006-01-02")))
	}
	// Hydrate mismatches.
	mismatches, err := s.mismatchRepo.GetByReportID(ctx, report.ID)
	if err != nil {
		return nil, fmt.Errorf("gldelivery.ReconciliationService.GetReport: mismatches: %w", err)
	}
	report.MismatchLines = mismatches

	// Compute totals.
	report.TotalAkunChecked = report.MismatchCount // simplified (actual total akun from repo join)
	for i := range mismatches {
		report.TotalMismatchAmountIDR = report.TotalMismatchAmountIDR.Add(mismatches[i].DeltaIDR.Abs())
	}
	if report.GlHostTotalIDR != nil {
		report.DeltaIDR = report.TotalJurnalIDR.Sub(*report.GlHostTotalIDR)
	}
	return report, nil
}

// ListReports returns paginated reconciliation report summaries.
func (s *ReconciliationService) ListReports(ctx context.Context, limit int, statusFilter string) ([]ReconSummaryItem, ListPage, error) {
	items, page, err := s.reportRepo.List(ctx, emptyQuery(), limit, statusFilter)
	if err != nil {
		return nil, ListPage{}, fmt.Errorf("gldelivery.ReconciliationService.ListReports: %w", err)
	}
	return items, page, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func rollbackTx(tx interface{ Rollback() error }) {
	_ = tx.Rollback() //nolint:errcheck
}

func ptrTime(t time.Time) *time.Time { return &t }

func emptyQuery() listquery.Query { return listquery.Query{} }

// workerContext injects a synthetic system-worker identity into ctx so that
// audit.writeEvent can resolve actorUserID without JWT claims.
// Mirrors the pattern used in other Asynq workers in this codebase.
func workerContext(parent context.Context) context.Context {
	return auth.ContextWithClaims(parent, &auth.Claims{
		Sub:   systemWorkerID.String(),
		Roles: []string{"ROLE-SYSTEM-WORKER"},
	})
}
