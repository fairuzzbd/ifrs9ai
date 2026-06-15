// Package jurnal — Worker: Asynq subscribers for P5-M1 penempatan events.
//
// Subscribes to:
//   - "penempatan:approved"   → EventCodePenempatan (PENEMPATAN journal entry)
//   - "penempatan:matured"    → EventCodeJatuhTempo  (JATUH_TEMPO journal entry)
//   - "penempatan:terminated" → EventCodePenjualanPencairan (PENJUALAN_PENCAIRAN entry)
//
// Error policy (per state machine docs/state-machines/p5-m2-jurnal-engine.md §5):
//   - Domain errors  → acknowledge immediately (no retry), write DLQ.
//   - Infra errors   → return error to Asynq for 3x retry (30s/60s/120s), then DLQ.
//
// Idempotency: PostResolved returns existing jurnalHeaderID if already posted.
// Audit-in-tx: PostingService.PostResolved writes JURNAL.POST in same tx (DEC-018).
package jurnal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	penemp "blips-ifrs9.tugu-re.com/internal/app-b/penempatan"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// systemWorkerID is the actor UUID stamped on audit rows written by the worker
// (no interactive user — system-level actor).
var systemWorkerID = uuid.MustParse("00000000-0000-0000-0000-000000000002")

// Worker subscribes to Asynq task queues for P5-M1 penempatan events.
type Worker struct {
	posting      *PostingService
	dlqRepo      *DLQRepo
	logger       *slog.Logger
	systemUserID uuid.UUID
}

// NewWorker creates a Worker. Panics on nil posting or dlqRepo.
// systemUserID is the actor UUID stamped on audit rows (use uuid.Nil to fall back to
// systemWorkerID sentinel).
func NewWorker(posting *PostingService, dlqRepo *DLQRepo, logger *slog.Logger, systemUserID ...uuid.UUID) *Worker {
	if posting == nil {
		panic("jurnal.NewWorker: posting must not be nil")
	}
	if dlqRepo == nil {
		panic("jurnal.NewWorker: dlqRepo must not be nil")
	}
	if logger == nil {
		logger = slog.Default()
	}
	sysID := systemWorkerID
	if len(systemUserID) > 0 && systemUserID[0] != uuid.Nil {
		sysID = systemUserID[0]
	}
	return &Worker{posting: posting, dlqRepo: dlqRepo, logger: logger, systemUserID: sysID}
}

// withSystemClaims injects a minimal system-actor claims into ctx so that
// audit.Write can resolve actorUserID for worker-initiated postings.
func (w *Worker) withSystemClaims(ctx context.Context) context.Context {
	return auth.ContextWithClaims(ctx, &auth.Claims{Sub: w.systemUserID.String()})
}

// RegisterHandlers registers all task handlers on the given Asynq ServeMux.
func (w *Worker) RegisterHandlers(mux *asynq.ServeMux) {
	mux.HandleFunc(penemp.PenempatanApprovedTaskType, w.HandlePenempatanApproved)
	mux.HandleFunc(penemp.PenempatanMaturedTaskType, w.HandlePenempatanMatured)
	mux.HandleFunc(penemp.PenempatanTerminatedTaskType, w.HandlePenempatanTerminated)
}

// HandlePenempatanApproved handles "penempatan:approved" tasks → PENEMPATAN journal.
func (w *Worker) HandlePenempatanApproved(ctx context.Context, t *asynq.Task) error {
	var evt penemp.ApprovedEvent
	if err := json.Unmarshal(t.Payload(), &evt); err != nil {
		return fmt.Errorf("jurnal.Worker.HandlePenempatanApproved: unmarshal: %w", err)
	}

	req := ResolverRequest{
		EventCode:         EventCodePenempatan,
		KlasifikasiPSAK71: evt.KlasifikasiPSAK71,
		InstrumenID:       &evt.InstrumenID,
		PeriodeID:         evt.PeriodeID,
		AmountIDR:         evt.NominalIDR,
		Currency:          evt.MataUangKode,
		SourceEventID:     evt.PenempatanID,
		SourceEventType:   penemp.PenempatanApprovedTaskType,
		Narasi:            fmt.Sprintf("Penempatan deposito %s / %s", evt.KodeTransaksi, evt.KlasifikasiPSAK71),
	}
	if evt.KursPenempatan != nil {
		req.FxRate = *evt.KursPenempatan
	}

	sysCtx := w.withSystemClaims(ctx)
	_, err := w.posting.PostResolved(sysCtx, req)
	if err != nil {
		return w.handlePostError(ctx, err, req, evt.PenempatanID, penemp.PenempatanApprovedTaskType, "HandlePenempatanApproved")
	}
	w.logger.InfoContext(ctx, "jurnal.Worker.HandlePenempatanApproved: posted",
		"penempatanId", evt.PenempatanID,
		"kodeTransaksi", evt.KodeTransaksi,
		"eventCode", EventCodePenempatan,
	)
	return nil
}

// HandlePenempatanMatured handles "penempatan:matured" tasks → JATUH_TEMPO journal.
func (w *Worker) HandlePenempatanMatured(ctx context.Context, t *asynq.Task) error {
	var evt penemp.MaturedEvent
	if err := json.Unmarshal(t.Payload(), &evt); err != nil {
		return fmt.Errorf("jurnal.Worker.HandlePenempatanMatured: unmarshal: %w", err)
	}

	req := ResolverRequest{
		EventCode:         EventCodeJatuhTempo,
		KlasifikasiPSAK71: evt.KlasifikasiPSAK71,
		InstrumenID:       &evt.InstrumenID,
		PeriodeID:         evt.PeriodeID,
		AmountIDR:         evt.NominalIDR,
		Currency:          "IDR",
		SourceEventID:     evt.PenempatanID,
		SourceEventType:   penemp.PenempatanMaturedTaskType,
		Narasi:            fmt.Sprintf("Jatuh tempo deposito %s / %s", evt.KodeTransaksi, evt.KlasifikasiPSAK71),
	}

	sysCtx := w.withSystemClaims(ctx)
	_, err := w.posting.PostResolved(sysCtx, req)
	if err != nil {
		return w.handlePostError(ctx, err, req, evt.PenempatanID, penemp.PenempatanMaturedTaskType, "HandlePenempatanMatured")
	}
	w.logger.InfoContext(ctx, "jurnal.Worker.HandlePenempatanMatured: posted",
		"penempatanId", evt.PenempatanID,
		"kodeTransaksi", evt.KodeTransaksi,
		"eventCode", EventCodeJatuhTempo,
	)
	return nil
}

// HandlePenempatanTerminated handles "penempatan:terminated" tasks → PENJUALAN_PENCAIRAN journal.
func (w *Worker) HandlePenempatanTerminated(ctx context.Context, t *asynq.Task) error {
	var evt penemp.TerminatedEvent
	if err := json.Unmarshal(t.Payload(), &evt); err != nil {
		return fmt.Errorf("jurnal.Worker.HandlePenempatanTerminated: unmarshal: %w", err)
	}

	req := ResolverRequest{
		EventCode:         EventCodePenjualanPencairan,
		KlasifikasiPSAK71: evt.KlasifikasiPSAK71,
		InstrumenID:       &evt.InstrumenID,
		PeriodeID:         evt.PeriodeID,
		AmountIDR:         evt.NominalIDR,
		Currency:          "IDR",
		SourceEventID:     evt.PenempatanID,
		SourceEventType:   penemp.PenempatanTerminatedTaskType,
		Narasi:            fmt.Sprintf("Terminasi/pencairan deposito %s / %s", evt.KodeTransaksi, evt.KlasifikasiPSAK71),
	}

	sysCtx := w.withSystemClaims(ctx)
	_, err := w.posting.PostResolved(sysCtx, req)
	if err != nil {
		return w.handlePostError(ctx, err, req, evt.PenempatanID, penemp.PenempatanTerminatedTaskType, "HandlePenempatanTerminated")
	}
	w.logger.InfoContext(ctx, "jurnal.Worker.HandlePenempatanTerminated: posted",
		"penempatanId", evt.PenempatanID,
		"kodeTransaksi", evt.KodeTransaksi,
		"eventCode", EventCodePenjualanPencairan,
	)
	return nil
}

// handlePostError handles posting errors per DLQ policy:
//   - Domain errors → write DLQ, return nil (acknowledge task, no Asynq retry).
//   - Infra errors  → return error to Asynq for retry.
func (w *Worker) handlePostError(
	ctx context.Context,
	err error,
	req ResolverRequest,
	sourceEventID uuid.UUID,
	sourceEventType string,
	handlerName string,
) error {
	de, isDomain := domainerrors.IsDomainError(err)

	errCode := "INFRA_ERROR"
	errCategory := "INFRA"
	if isDomain {
		errCode = string(de.Code())
		errCategory = "DOMAIN"
	}

	w.logger.ErrorContext(ctx, "jurnal.Worker."+handlerName+": posting failed",
		"eventCode", req.EventCode,
		"sourceEventId", sourceEventID,
		"errCode", errCode,
		"errCategory", errCategory,
		"error", err,
	)

	// Build DLQ payload.
	var instrIDStr *string
	if req.InstrumenID != nil {
		s := req.InstrumenID.String()
		instrIDStr = &s
	}
	payload := DLQPostPayload{
		EventCode:         req.EventCode,
		KlasifikasiPSAK71: req.KlasifikasiPSAK71,
		InstrumenID:       instrIDStr,
		PeriodeID:         req.PeriodeID.String(),
		AmountIDR:         req.AmountIDR,
		Currency:          req.Currency,
		FxRate:            req.FxRate,
		SourceEventID:     req.SourceEventID.String(),
		SourceEventType:   req.SourceEventType,
		MetadataJSON:      req.MetadataJSON,
		Narasi:            req.Narasi,
	}
	payloadJSON, jsonErr := json.Marshal(payload)
	if jsonErr != nil {
		w.logger.ErrorContext(ctx, "jurnal.Worker.handlePostError: marshal payload failed", "error", jsonErr)
		payloadJSON = []byte("{}")
	}

	var instrID *uuid.UUID
	if req.InstrumenID != nil {
		cp := *req.InstrumenID
		instrID = &cp
	}
	periodeID := req.PeriodeID

	entry := &DLQEntry{
		ID:              uuid.New(),
		SourceEventID:   sourceEventID,
		SourceEventType: sourceEventType,
		EventCode:       req.EventCode,
		InstrumenID:     instrID,
		PeriodeID:       &periodeID,
		PayloadJSONB:    payloadJSON,
		ErrorCode:       errCode,
		ErrorMessage:    err.Error(),
		ErrorCategory:   errCategory,
		RetryCount:      0,
		Status:          DLQStatusFailed,
	}

	if dlqErr := w.dlqRepo.Insert(ctx, nil, entry); dlqErr != nil {
		w.logger.ErrorContext(ctx, "jurnal.Worker.handlePostError: DLQ insert failed",
			"error", dlqErr,
			"originalErr", err,
		)
		// If domain error: we already know we can't post, so don't retry.
		// If DLQ insert itself fails on infra error: fall through to retry logic below.
		if !isDomain {
			return fmt.Errorf("jurnal.Worker.%s: posting+DLQ failed: %w", handlerName, err)
		}
		// Domain error + DLQ insert failed: log but acknowledge (can't retry domain errors).
		return nil
	}

	if isDomain {
		// Domain error → acknowledge immediately (Asynq won't retry).
		return nil
	}
	// Infra error → return to Asynq for retry policy (3x: 30s/60s/120s then DLQ discard).
	return fmt.Errorf("jurnal.Worker.%s: infra error (Asynq will retry): %w", handlerName, err)
}
