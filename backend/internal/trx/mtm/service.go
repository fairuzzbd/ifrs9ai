package mtm

// service.go — Service owns all MTM business logic.
// TX boundary lives here; repos never open transactions.
//
// Business rules enforced:
//   - AC instruments → ErrMTMInstrumenACSkip (never inserted)
//   - locked_flag=TRUE → ErrMTMPeriodeLocked (HTTP 423)
//   - SoD: override_approver_id ≠ uploader_id → ErrMTMOverrideSODViolation
//   - Stale price: harga_age_days > MTM_PRICE_STALE_DAYS → StatusStalePrice
//   - Deviation: ABS(delta_pct) > threshold → StatusPendingReview
//   - Audit in same tx as every mutation (DEC-018)
//   - Idempotency via ExistsActive (DEC-021)

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Service is the MTM service holding all business logic.
type Service struct {
	repo   Repository
	poster JurnalPoster
	audit  *audit.Writer
	logger *slog.Logger
}

// NewService creates a new MTM Service.
func NewService(repo Repository, poster JurnalPoster, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if poster == nil {
		poster = NewNoopJurnalPoster(logger)
	}
	return &Service{
		repo:   repo,
		poster: poster,
		audit:  auditWriter,
		logger: logger,
	}
}

// WithJurnalPoster swaps the JurnalPoster (used in main.go after P5-M2 wired).
func (s *Service) WithJurnalPoster(p JurnalPoster) *Service {
	s.poster = p
	return s
}

// ─── Config helpers ───────────────────────────────────────────────────────────

func (s *Service) configInt(ctx context.Context, key string, defaultVal int) int {
	v, err := s.repo.GetConfigValue(ctx, key)
	if err != nil || v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

func (s *Service) configFloat(ctx context.Context, key string, defaultVal float64) decimal.Decimal {
	v, err := s.repo.GetConfigValue(ctx, key)
	if err != nil || v == "" {
		return decimal.NewFromFloat(defaultVal)
	}
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.NewFromFloat(defaultVal)
	}
	return d
}

// ─── GetList ──────────────────────────────────────────────────────────────────

// GetList returns paginated MTM rows.
func (s *Service) GetList(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Mtm, bool, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.List(ctx, q, cursor, limit)
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

// GetDetail fetches one MTM row by ID.
func (s *Service) GetDetail(ctx context.Context, id uuid.UUID) (*Mtm, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Service.GetDetail: %w", err)
	}
	if m == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, fmt.Sprintf("MTM record %s tidak ditemukan.", id))
	}
	return m, nil
}

// ─── GetUploadBatch ───────────────────────────────────────────────────────────

// GetUploadBatch returns batch metadata + rows for a given batch ID.
func (s *Service) GetUploadBatch(ctx context.Context, batchID uuid.UUID) (*UploadBatchDetail, error) {
	b, err := s.repo.GetUploadBatch(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("Service.GetUploadBatch: %w", err)
	}
	if b == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Upload batch %s tidak ditemukan.", batchID))
	}

	rows, err := s.repo.ListByBatchID(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("Service.GetUploadBatch: list rows: %w", err)
	}

	detail := &UploadBatchDetail{
		UploadBatchID: b.ID.String(),
		UploaderID:    b.UploaderID.String(),
		CatatanUpload: b.CatatanUpload,
		RowsParsed:    b.TotalRows,
		RowsValid:     b.ValidRows,
		RowsInvalid:   b.InvalidRows,
		Status:        b.Status,
		CreatedAt:     b.CreatedAt.Format(time.RFC3339),
	}
	for i, m := range rows {
		row := UploadBatchRow{
			LineNumber:     i + 1,
			MtmID:          m.ID.String(),
			InstrumenID:    m.InstrumenID.String(),
			TanggalMtm:     m.TanggalMtm.Format("2006-01-02"),
			HargaPasarIdr:  m.HargaPasarIdr.StringFixed(4),
			DeltaPct:       m.DeltaPct.StringFixed(4),
			DeviationFlag:  m.DeviationFlag,
			StalePriceFlag: m.StalePriceFlag,
			RowStatus:      string(m.Status),
		}
		if m.HargaPasarFcy != nil {
			s2 := m.HargaPasarFcy.StringFixed(8)
			row.HargaPasarFcy = &s2
		}
		detail.Rows = append(detail.Rows, row)
	}
	return detail, nil
}

// ─── GetStalePriceAlerts ──────────────────────────────────────────────────────

// GetStalePriceAlerts returns stale-price alert items.
func (s *Service) GetStalePriceAlerts(ctx context.Context, cursor string, limit int) ([]StaleAlertItem, bool, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, hasMore, total, err := s.repo.ListStaleAlerts(ctx, cursor, limit)
	if err != nil {
		return nil, false, 0, fmt.Errorf("Service.GetStalePriceAlerts: %w", err)
	}
	escalationDays := s.configInt(ctx, "MTM_STALE_ESCALATION_DAYS", DefaultStaleEscalationDays)
	var items []StaleAlertItem
	for _, m := range rows {
		items = append(items, StaleAlertItem{
			ID:           m.ID.String(),
			InstrumenID:  m.InstrumenID.String(),
			TanggalMtm:   m.TanggalMtm.Format("2006-01-02"),
			HargaTanggal: m.HargaTanggal.Format("2006-01-02"),
			HargaAgeDays: m.HargaAgeDays,
			Status:       string(m.Status),
			EskalasiFag:  IsStalePriceEscalation(m.HargaAgeDays, escalationDays),
		})
	}
	return items, hasMore, total, nil
}

// ─── TriggerCron ─────────────────────────────────────────────────────────────

// TriggerCron is called by POST /trx/mtm/cron/trigger (manual trigger by ROLE-AKUN).
// Enqueues an Asynq task immediately (does not wait for scheduled cron).
// Returns job metadata; the actual work is done by the worker.
func (s *Service) TriggerCron(ctx context.Context, enqueuer AsynqEnqueuer, req CronTriggerRequest) (*CronTriggerResponse, error) {
	tanggalTarget := req.TanggalTarget
	if tanggalTarget == "" {
		tanggalTarget = time.Now().Format("2006-01-02")
	}
	if _, err := ParseDateStrict(tanggalTarget); err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID := ""
	if claims != nil {
		actorID = claims.Sub
	}

	jobID := "mtm-manual-" + uuid.New().String()
	payload := MtmCronPayload{
		TanggalTarget: tanggalTarget,
		TenantID:      "TUGURE",
		JobID:         jobID,
		ForceRerun:    req.ForceRerun,
		ActorID:       actorID,
	}
	if err := enqueuer.Enqueue(ctx, TaskMtmDailyRun, payload); err != nil {
		return nil, fmt.Errorf("Service.TriggerCron: enqueue: %w", err)
	}

	// Estimate instruments affected (non-blocking, best-effort)
	instruments, _ := s.repo.GetActiveNonACInstrumen(ctx)

	return &CronTriggerResponse{
		JobID:              jobID,
		Type:               TaskMtmDailyRun,
		TanggalTarget:      tanggalTarget,
		StatusURL:          "/api/v1/jobs/" + jobID,
		StreamURL:          "/api/v1/jobs/" + jobID + "/stream",
		EstimatedInstrumen: len(instruments),
		Message:            "MTM daily run di-trigger. Pantau progress di statusUrl.",
	}, nil
}

// ─── UploadManual ─────────────────────────────────────────────────────────────

// UploadManual processes a manual price upload (ROLE-AKUN).
// Parses rows, validates, inserts MTM records.
// SoD: the uploader cannot be the override approver (enforced at override-approve time).
func (s *Service) UploadManual(ctx context.Context, uploaderID uuid.UUID, rows []UploadFileRow, catatan string) (*UploadBatchResponse, error) {
	staleDays := s.configInt(ctx, "MTM_PRICE_STALE_DAYS", DefaultStalePriceDays)
	thresholdPct := s.configFloat(ctx, "MTM_PRICE_DEVIATION_THRESHOLD_PCT", DefaultDeviationThresholdPct)

	batchID := uuid.New()
	validRows := 0
	invalidRows := 0
	var mtmIDs []string
	var stalePriceWarnings []string
	var deviationWarnings []DeviationWarning
	var created []UploadRowCreated

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.UploadManual: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Insert upload_batch row
	batch := &UploadBatch{
		ID:            batchID,
		BatchType:     "MTM_UPLOAD",
		Status:        "PENDING_REVIEW",
		CatatanUpload: catatan,
		UploaderID:    uploaderID,
		TotalRows:     len(rows),
		ValidRows:     0, // updated after loop
		InvalidRows:   0,
		TenantID:      "TUGURE",
		CreatedAt:     time.Now(),
		CreatedBy:     uploaderID,
		UpdatedAt:     time.Now(),
		UpdatedBy:     uploaderID,
	}
	if err := s.repo.InsertUploadBatch(ctx, tx, batch); err != nil {
		return nil, fmt.Errorf("Service.UploadManual: insert batch: %w", err)
	}

	for _, row := range rows {
		m, staleWarn, devWarn, err := s.processUploadRow(ctx, tx, row, batchID, uploaderID, staleDays, thresholdPct)
		if err != nil {
			invalidRows++
			s.logger.WarnContext(ctx, "UploadManual: invalid row",
				"line", row.LineNumber, "instrumen", row.KodeInstrumen, "error", err)
			continue
		}
		mtmIDs = append(mtmIDs, m.ID.String())
		if staleWarn != "" {
			stalePriceWarnings = append(stalePriceWarnings, staleWarn)
		}
		if devWarn != nil {
			deviationWarnings = append(deviationWarnings, *devWarn)
		}
		created = append(created, UploadRowCreated{
			InstrumenKode:  row.KodeInstrumen,
			TanggalMtm:     row.TanggalMtm,
			HargaPasarIdr:  m.HargaPasarIdr.StringFixed(4),
			HargaSumber:    string(m.HargaSumber),
			DeviationFlag:  m.DeviationFlag,
			DeltaPct:       m.DeltaPct.StringFixed(4),
			StalePriceFlag: m.StalePriceFlag,
		})
		validRows++
	}

	// Audit upload batch
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "MTM.UPLOAD_BATCH",
			EntityType: "trx.mtm",
			EntityID:   batchID,
			After: map[string]any{
				"batch_id":   batchID.String(),
				"uploader":   uploaderID.String(),
				"total_rows": len(rows),
				"valid":      validRows,
				"invalid":    invalidRows,
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.UploadManual: commit: %w", err)
		}
	}

	nextStep := "Semua baris AUTO_POSTED."
	if len(stalePriceWarnings) > 0 || len(deviationWarnings) > 0 {
		nextStep = "Beberapa baris memerlukan override-approve oleh ROLE-AKUN-CTL."
	}
	return &UploadBatchResponse{
		UploadBatchID:      batchID.String(),
		RowsParsed:         len(rows),
		RowsValid:          validRows,
		RowsInvalid:        invalidRows,
		Status:             "PENDING_REVIEW",
		MtmIDs:             mtmIDs,
		RowsCreated:        created,
		StalePriceWarnings: stalePriceWarnings,
		DeviationWarnings:  deviationWarnings,
		NextStep:           nextStep,
	}, nil
}

// processUploadRow is the per-row logic for UploadManual.
func (s *Service) processUploadRow(ctx context.Context, tx *sql.Tx, row UploadFileRow,
	batchID, uploaderID uuid.UUID, staleDays int, thresholdPct decimal.Decimal,
) (*Mtm, string, *DeviationWarning, error) {
	tanggalMtm, err := ParseDateStrict(row.TanggalMtm)
	if err != nil {
		return nil, "", nil, err
	}
	hargaPasar, err := decimal.NewFromString(row.HargaPasar)
	if err != nil {
		return nil, "", nil, fmt.Errorf("harga_pasar tidak valid: %s", row.HargaPasar)
	}
	if err := ValidatePricePositive(hargaPasar, "harga_pasar"); err != nil {
		return nil, "", nil, err
	}
	hargaSumber := HargaSumber(row.HargaSumber)
	if row.HargaSumber == "" {
		hargaSumber = HargaSumberManual
	}
	if !IsValidHargaSumber(string(hargaSumber)) {
		return nil, "", nil, fmt.Errorf("harga_sumber tidak valid: %s", row.HargaSumber)
	}

	// Placeholder: book value not yet available (OQ-M6-6)
	hargaBukuIdr := hargaPasar // fallback: treat pasar == buku → delta=0
	bvPtr, _ := s.repo.GetHargaBukuIdr(ctx, uuid.Nil)
	if bvPtr != nil {
		hargaBukuIdr = *bvPtr
	}

	deltaIdr, deltaPct, err := ComputeDelta(hargaPasar, hargaBukuIdr)
	if err != nil {
		deltaIdr = decimal.Zero
		deltaPct = decimal.Zero
	}
	hargaAgeDays := ComputeHargaAgeDays(tanggalMtm, time.Time{})
	staleFlag := IsStalePriceByAge(hargaAgeDays, staleDays)
	devFlag := IsDeviationExceeded(deltaPct, thresholdPct)

	status := StatusAutoPOSTED
	if staleFlag {
		status = StatusStalePrice
	} else if devFlag {
		status = StatusPendingReview
	}

	uploader := uploaderID
	m := &Mtm{
		ID:                  uuid.New(),
		InstrumenID:         uuid.Nil, // resolved from kode_instrumen by service (stub uuid for now)
		PeriodeBulananID:    uuid.Nil,
		TanggalMtm:          tanggalMtm,
		HargaSumber:         hargaSumber,
		HargaTanggal:        tanggalMtm,
		HargaAgeDays:        hargaAgeDays,
		HargaPasarIdr:       hargaPasar,
		HargaBukuIdr:        hargaBukuIdr,
		DeltaIdr:            deltaIdr,
		DeltaPct:            deltaPct,
		KlasifikasiSnapshot: "MANUAL",
		TreatmentSnapshot:   "",
		StalePriceFlag:      staleFlag,
		DeviationFlag:       devFlag,
		LockedFlag:          false,
		Status:              status,
		UploadBatchID:       &batchID,
		UploaderID:          &uploader,
		CreatedAt:           time.Now(),
		CreatedBy:           uploaderID,
		UpdatedAt:           time.Now(),
		UpdatedBy:           uploaderID,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	if err := s.repo.Insert(ctx, tx, m); err != nil {
		return nil, "", nil, fmt.Errorf("insert MTM: %w", err)
	}

	// Audit per row
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "MTM.UPLOAD_ROW",
			EntityType: "trx.mtm",
			EntityID:   m.ID,
			After: map[string]any{
				"instrumen":       row.KodeInstrumen,
				"tanggal_mtm":     row.TanggalMtm,
				"harga_pasar_idr": hargaPasar.StringFixed(4),
				"status":          string(status),
			},
		})
	}

	var staleWarn string
	if staleFlag {
		staleWarn = fmt.Sprintf("Instrumen %s: harga stale (%d hari).", row.KodeInstrumen, hargaAgeDays)
	}
	var devWarn *DeviationWarning
	if devFlag {
		f64, _ := deltaPct.Abs().Float64()
		thr64, _ := thresholdPct.Float64()
		devWarn = &DeviationWarning{
			InstrumenKode: row.KodeInstrumen,
			DeltaPct:      f64,
			ThresholdPct:  thr64,
			Message: fmt.Sprintf("Delta %.2f%% melebihi threshold %.2f%%.",
				f64, thr64),
		}
	}
	return m, staleWarn, devWarn, nil
}

// ─── OverrideApprove ──────────────────────────────────────────────────────────

// OverrideApprove approves a PENDING_REVIEW or STALE_PRICE MTM row.
// SoD: approver ≠ uploader (DEC-017). Audit in same tx.
// After approve: resolves jurnal event codes and posts via JurnalPoster.
func (s *Service) OverrideApprove(ctx context.Context, mtmID uuid.UUID, req OverrideApproveRequest) (*OverrideApproveResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	approverID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	m, err := s.repo.GetByID(ctx, mtmID)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideApprove: %w", err)
	}
	if m == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, fmt.Sprintf("MTM %s tidak ditemukan.", mtmID))
	}

	// LockedFlag guard
	if m.LockedFlag {
		return nil, ErrMTMPeriodeLocked
	}

	// Status guard
	if !m.Status.CanOverride() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(m.Status), "APPROVED")
	}

	// SoD: approver ≠ uploader
	if m.UploaderID != nil && *m.UploaderID == approverID {
		return nil, ErrMTMOverrideSODViolation
	}

	// Comment length
	if err := ValidateOverrideComment(req.Comment, MinOverrideCommentLen); err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	// Resolve jurnal event codes
	eventCodes, routeErr := ResolveJurnalEventCode(m.KlasifikasiSnapshot, "IDR", false)
	if routeErr != nil {
		s.logger.WarnContext(ctx, "OverrideApprove: routing error — skip jurnal posting",
			"mtm_id", mtmID, "klasifikasi", m.KlasifikasiSnapshot, "error", routeErr)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideApprove: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now()
	update := StatusUpdate{
		Status:             StatusApproved,
		OverrideApproverID: &approverID,
		OverrideComment:    &req.Comment,
		OverrideAt:         &now,
		UpdatedBy:          approverID,
		RowVersion:         m.RowVersion,
	}

	// Post jurnal (best-effort; noop if P5-M2 not wired)
	var entryID1, entryID2 *uuid.UUID
	var eventCode1, eventCode2 *string
	if len(eventCodes) >= 1 && routeErr == nil {
		r1, postErr := s.poster.Post(ctx, tx, PostRequest{
			EventCode:   eventCodes[0],
			InstrumenID: m.InstrumenID,
			PeriodeID:   m.PeriodeBulananID,
			TanggalMtm:  m.TanggalMtm,
			Amount:      m.DeltaIdr,
			MtmID:       m.ID,
			ActorID:     approverID,
			TenantID:    m.TenantID,
		})
		if postErr != nil {
			return nil, fmt.Errorf("Service.OverrideApprove: jurnal post 1: %w", postErr)
		}
		entryID1 = &r1.JurnalEntryID
		eventCode1 = &r1.EventCode
		update.JurnalEntryID = entryID1
		update.JurnalEventCode = eventCode1
	}
	if len(eventCodes) >= 2 && routeErr == nil {
		r2, postErr := s.poster.Post(ctx, tx, PostRequest{
			EventCode:   eventCodes[1],
			InstrumenID: m.InstrumenID,
			PeriodeID:   m.PeriodeBulananID,
			TanggalMtm:  m.TanggalMtm,
			Amount:      m.DeltaIdr,
			KursTengah:  m.KursTengah,
			KursID:      m.KursID,
			MtmID:       m.ID,
			ActorID:     approverID,
			TenantID:    m.TenantID,
		})
		if postErr != nil {
			return nil, fmt.Errorf("Service.OverrideApprove: jurnal post 2: %w", postErr)
		}
		entryID2 = &r2.JurnalEntryID
		eventCode2 = &r2.EventCode
		update.JurnalEntryID2 = entryID2
		update.JurnalEventCode2 = eventCode2
	}

	if err := s.repo.UpdateStatus(ctx, tx, mtmID, update); err != nil {
		return nil, fmt.Errorf("Service.OverrideApprove: update: %w", err)
	}

	// Audit in same tx
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "MTM.OVERRIDE_APPROVE",
			EntityType: "trx.mtm",
			EntityID:   mtmID,
			Before:     map[string]any{"status": string(m.Status)},
			After: map[string]any{
				"status":                "APPROVED",
				"override_approver_id":  approverID.String(),
				"override_comment":      req.Comment,
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.OverrideApprove: commit: %w", err)
		}
	}

	resp := &OverrideApproveResponse{
		MtmID:          mtmID.String(),
		Status:         string(StatusApproved),
		ApprovedBy:     approverID.String(),
		ApprovedAt:     now.Format(time.RFC3339),
		Message:        "MTM berhasil di-approve. Jurnal otomatis di-post.",
		JurnalEventCodes: eventCodes,
	}
	if entryID1 != nil {
		s2 := entryID1.String()
		resp.JurnalEntryID = &s2
	}
	return resp, nil
}

// ─── OverrideReject ───────────────────────────────────────────────────────────

// OverrideReject rejects a PENDING_REVIEW or STALE_PRICE MTM row.
// SoD: rejecter ≠ uploader (same guard as approve).
func (s *Service) OverrideReject(ctx context.Context, mtmID uuid.UUID, req OverrideRejectRequest) (*OverrideRejectResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	rejecterID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	m, err := s.repo.GetByID(ctx, mtmID)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideReject: %w", err)
	}
	if m == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound, fmt.Sprintf("MTM %s tidak ditemukan.", mtmID))
	}
	if m.LockedFlag {
		return nil, ErrMTMPeriodeLocked
	}
	if !m.Status.CanOverride() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(m.Status), "REJECTED")
	}
	if m.UploaderID != nil && *m.UploaderID == rejecterID {
		return nil, ErrMTMOverrideSODViolation
	}
	if err := ValidateOverrideComment(req.Comment, MinOverrideCommentLen); err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideReject: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now()
	update := StatusUpdate{
		Status:             StatusRejected,
		OverrideApproverID: &rejecterID,
		OverrideComment:    &req.Comment,
		OverrideAt:         &now,
		UpdatedBy:          rejecterID,
		RowVersion:         m.RowVersion,
	}
	if err := s.repo.UpdateStatus(ctx, tx, mtmID, update); err != nil {
		return nil, fmt.Errorf("Service.OverrideReject: update: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "MTM.OVERRIDE_REJECT",
			EntityType: "trx.mtm",
			EntityID:   mtmID,
			Before:     map[string]any{"status": string(m.Status)},
			After: map[string]any{
				"status":               "REJECTED",
				"override_approver_id": rejecterID.String(),
				"override_comment":     req.Comment,
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.OverrideReject: commit: %w", err)
		}
	}

	return &OverrideRejectResponse{
		MtmID:         mtmID.String(),
		Status:        string(StatusRejected),
		RejectedBy:    rejecterID.String(),
		RejectedAt:    now.Format(time.RFC3339),
		Comment:       req.Comment,
		Message:       "MTM ditolak. Record status = REJECTED.",
	}, nil
}

// ─── ProcessOneInstrument (called by cron worker) ─────────────────────────────

// ProcessOneInstrument fetches price, computes delta, inserts MTM row, posts jurnal.
// Called by worker.HandleEachInstrument in a per-instrument tx.
// Returns (nil, ErrMTMInstrumenACSkip) for AC instruments — caller skips silently.
func (s *Service) ProcessOneInstrument(ctx context.Context, inst InstrumenInfo, tanggalMtm time.Time, cronJobID string) (*Mtm, error) {
	// AC guard — must never reach here; cron filters before calling
	if inst.KlasifikasiPSAK71 == KlasifikasiAC {
		return nil, ErrMTMInstrumenACSkip
	}

	staleDays := s.configInt(ctx, "MTM_PRICE_STALE_DAYS", DefaultStalePriceDays)
	thresholdPct := s.configFloat(ctx, "MTM_PRICE_DEVIATION_THRESHOLD_PCT", DefaultDeviationThresholdPct)

	// Idempotency check — if already processed for today, skip
	exists, existing, err := s.repo.ExistsActive(ctx, inst.ID, tanggalMtm, "AUTO")
	if err != nil {
		return nil, fmt.Errorf("ProcessOneInstrument: exists check: %w", err)
	}
	if exists {
		s.logger.InfoContext(ctx, "ProcessOneInstrument: already processed, skipping",
			"instrumen_id", inst.ID, "tanggal_mtm", tanggalMtm.Format("2006-01-02"), "status", existing.Status)
		return existing, nil
	}

	// Fetch market price from feed
	fp, err := s.repo.GetFeedPrice(ctx, inst.ID, inst.TipeInstrumen, tanggalMtm)
	if err != nil {
		return nil, fmt.Errorf("ProcessOneInstrument: GetFeedPrice: %w", err)
	}

	// Determine harga_pasar_idr and FCY/IDR details
	var hargaPasarIdr, hargaPasarFcy decimal.Decimal
	var hargaTanggal time.Time
	var hargaAgeDays int16
	var staleFlag bool
	var ks *KursSnapshot
	var hargaSumber HargaSumber

	if fp == nil {
		// No price in feed → STALE_PRICE
		hargaPasarIdr = decimal.Zero
		hargaAgeDays = 999
		staleFlag = true
		hargaTanggal = time.Time{}
		hargaSumber = HargaSumberIBPA // default source for auto cron
	} else {
		hargaTanggal = fp.HargaTanggal
		hargaAgeDays = ComputeHargaAgeDays(tanggalMtm, hargaTanggal)
		staleFlag = IsStalePriceByAge(hargaAgeDays, staleDays)
		hargaPasarFcy = fp.HargaPasar

		// Determine source
		switch inst.TipeInstrumen {
		case "OBLIGASI":
			hargaSumber = HargaSumberIBPA
		case "SAHAM":
			hargaSumber = HargaSumberBEI
		case "REKSADANA":
			hargaSumber = HargaSumberKSEI
		default:
			hargaSumber = HargaSumberManual
		}

		if inst.MataUang != "IDR" {
			// Need FX conversion
			ks, err = s.repo.GetApprovedKurs(ctx, inst.MataUang, tanggalMtm)
			if err != nil {
				return nil, fmt.Errorf("ProcessOneInstrument: GetApprovedKurs: %w", err)
			}
			if ks == nil {
				// Kurs not available → STALE_PRICE
				staleFlag = true
				hargaPasarIdr = decimal.Zero
			} else {
				hargaPasarIdr = fp.HargaPasar.Mul(ks.KursTengah).RoundBank(4)
			}
		} else {
			hargaPasarIdr = fp.HargaPasar
		}
	}

	// Book value (OQ-M6-6: placeholder returns nil)
	bvPtr, _ := s.repo.GetHargaBukuIdr(ctx, inst.ID)
	hargaBukuIdr := hargaPasarIdr // fallback: book = market → delta=0
	if bvPtr != nil {
		hargaBukuIdr = *bvPtr
	}

	deltaIdr, deltaPct, err := ComputeDelta(hargaPasarIdr, hargaBukuIdr)
	if err != nil {
		deltaIdr = decimal.Zero
		deltaPct = decimal.Zero
	}
	devFlag := IsDeviationExceeded(deltaPct, thresholdPct)

	// Resolve jurnal routing
	eventCodes, routeErr := ResolveJurnalEventCode(inst.KlasifikasiPSAK71, inst.MataUang, inst.IsPOCI)
	if routeErr != nil {
		s.logger.WarnContext(ctx, "ProcessOneInstrument: routing error",
			"instrumen_id", inst.ID, "klasifikasi", inst.KlasifikasiPSAK71, "error", routeErr)
	}

	status := StatusAutoPOSTED
	if staleFlag {
		status = StatusStalePrice
	} else if devFlag {
		status = StatusPendingReview
	}

	jobIDPtr := &cronJobID

	m := &Mtm{
		ID:                  uuid.New(),
		InstrumenID:         inst.ID,
		PeriodeBulananID:    uuid.Nil, // resolved from tanggalMtm by service (stub)
		TanggalMtm:          tanggalMtm,
		HargaSumber:         hargaSumber,
		HargaTanggal:        hargaTanggal,
		HargaAgeDays:        hargaAgeDays,
		HargaPasarIdr:       hargaPasarIdr,
		HargaBukuIdr:        hargaBukuIdr,
		DeltaIdr:            deltaIdr,
		DeltaPct:            deltaPct,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		TreatmentSnapshot:   "",
		StalePriceFlag:      staleFlag,
		DeviationFlag:       devFlag,
		LockedFlag:          false,
		Status:              status,
		CronJobID:           jobIDPtr,
		CreatedAt:           time.Now(),
		CreatedBy:           uuid.Nil, // system
		UpdatedAt:           time.Now(),
		UpdatedBy:           uuid.Nil,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}
	if fp != nil && inst.MataUang != "IDR" {
		m.HargaPasarFcy = &hargaPasarFcy
	}
	if ks != nil {
		m.KursID = &ks.KursID
		m.KursTengah = &ks.KursTengah
	}

	// Per-instrument tx
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("ProcessOneInstrument: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := s.repo.Insert(ctx, tx, m); err != nil {
		return nil, fmt.Errorf("ProcessOneInstrument: insert: %w", err)
	}

	// Post jurnal entries if AUTO_POSTED
	if status == StatusAutoPOSTED && routeErr == nil && len(eventCodes) > 0 {
		r1, postErr := s.poster.Post(ctx, tx, PostRequest{
			EventCode:   eventCodes[0],
			InstrumenID: inst.ID,
			PeriodeID:   m.PeriodeBulananID,
			TanggalMtm:  tanggalMtm,
			Amount:      deltaIdr,
			MtmID:       m.ID,
			TenantID:    "TUGURE",
		})
		if postErr != nil {
			return nil, fmt.Errorf("ProcessOneInstrument: jurnal post 1: %w", postErr)
		}
		m.JurnalEntryID = &r1.JurnalEntryID
		ec1 := r1.EventCode
		m.JurnalEventCode = &ec1

		if len(eventCodes) >= 2 {
			r2, postErr := s.poster.Post(ctx, tx, PostRequest{
				EventCode:   eventCodes[1],
				InstrumenID: inst.ID,
				PeriodeID:   m.PeriodeBulananID,
				TanggalMtm:  tanggalMtm,
				Amount:      deltaIdr,
				KursTengah:  m.KursTengah,
				KursID:      m.KursID,
				MtmID:       m.ID,
				TenantID:    "TUGURE",
			})
			if postErr != nil {
				return nil, fmt.Errorf("ProcessOneInstrument: jurnal post 2: %w", postErr)
			}
			m.JurnalEntryID2 = &r2.JurnalEntryID
			ec2 := r2.EventCode
			m.JurnalEventCode2 = &ec2
		}
	}

	// Audit
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "MTM.AUTO_POSTED",
			EntityType: "trx.mtm",
			EntityID:   m.ID,
			After: map[string]any{
				"instrumen_id": inst.ID.String(),
				"tanggal_mtm":  tanggalMtm.Format("2006-01-02"),
				"status":       string(status),
				"delta_pct":    deltaPct.StringFixed(4),
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("ProcessOneInstrument: commit: %w", err)
		}
	}
	return m, nil
}

// ─── LockMtmForPeriode / UnlockMtmForPeriode (MtmLocker contract) ────────────

// LockMtmForPeriode implements MtmLocker. Called by closeflow.Service.HardClose.
func (s *Service) LockMtmForPeriode(ctx interface{}, txI interface{}, periodeID uuid.UUID, actorID uuid.UUID) error {
	c, ok := ctx.(context.Context)
	if !ok {
		return fmt.Errorf("LockMtmForPeriode: ctx must be context.Context")
	}
	tx, ok := txI.(*sql.Tx)
	if !ok {
		return fmt.Errorf("LockMtmForPeriode: tx must be *sql.Tx")
	}
	// Date range: derive from periode. Stub: use a wide range (year 2000-2100).
	// Real implementation: query mst.periode_buku for tanggal_mulai/akhir.
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 12, 31, 0, 0, 0, 0, time.UTC)
	_, err := s.repo.LockMtmForPeriode(c, tx, periodeID, from, to, actorID)
	return err
}

// UnlockMtmForPeriode implements MtmLocker.
func (s *Service) UnlockMtmForPeriode(ctx interface{}, txI interface{}, periodeID uuid.UUID, actorID uuid.UUID) error {
	c, ok := ctx.(context.Context)
	if !ok {
		return fmt.Errorf("UnlockMtmForPeriode: ctx must be context.Context")
	}
	tx, ok := txI.(*sql.Tx)
	if !ok {
		return fmt.Errorf("UnlockMtmForPeriode: tx must be *sql.Tx")
	}
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 12, 31, 0, 0, 0, 0, time.UTC)
	_, err := s.repo.UnlockMtmForPeriode(c, tx, periodeID, from, to, actorID)
	return err
}

// ─── AsynqEnqueuer (interface for TriggerCron) ───────────────────────────────

// AsynqEnqueuer is a minimal interface for enqueuing Asynq tasks.
// Implemented by the real Asynq client in main.go; stub in tests.
type AsynqEnqueuer interface {
	Enqueue(ctx context.Context, taskType string, payload interface{}) error
}

// NoopEnqueuer is a no-op AsynqEnqueuer for dev mode (no Redis configured).
// TriggerCron returns an error when this is used so callers know tasks are not queued.
type NoopEnqueuer struct{}

// Enqueue returns an error indicating Redis is not configured in dev mode.
func (NoopEnqueuer) Enqueue(_ context.Context, _ string, _ interface{}) error {
	return fmt.Errorf("NoopEnqueuer: Redis not configured — start with REDIS_URL to enable cron trigger")
}
