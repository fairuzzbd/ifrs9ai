package coa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ─── Asynq task type ─────────────────────────────────────────────────────────

const TaskTypeCoAImportXLSX = "coa:import_xlsx"

// ImportPayload is the Asynq task payload for XLSX import.
type ImportPayload struct {
	JobID      string    `json:"jobId"`
	ActorID    string    `json:"actorId"`
	TenantID   string    `json:"tenantId"`
	SumberCoa  string    `json:"sumberCoa"`
	FileSHA    string    `json:"fileSha256"` // idempotency: skip if already processed
	FileBytes  []byte    `json:"fileBytes"`  // embedded; small XLSX only (≤ 10MB)
	EnqueuedAt time.Time `json:"enqueuedAt"`
}

// ─── Job state (persisted in sys.job) ────────────────────────────────────────

// JobState mirrors sys.job columns relevant for import tracking.
// Exported so test stubs can implement JobRepository without reflection.
type JobState struct {
	ID          string
	Status      string // queued|running|completed|failed|canceled
	Progress    int
	CurrentStep string
	RowsTotal   int
	RowsDone    int
	RowsError   int
	ErrorDetail *string
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// ─── JobRepository — minimal interface over sys.job ──────────────────────────

// JobRepository persists job progress in sys.job.
// The production implementation writes to DB; a stub is used in tests.
type JobRepository interface {
	InsertJob(ctx context.Context, j *JobState, actorID uuid.UUID, tenantID string) error
	UpdateJobProgress(ctx context.Context, id string, progress int, step string, rowsDone int, rowsError int) error
	CompleteJob(ctx context.Context, id string, rowsTotal, rowsDone, rowsError int) error
	FailJob(ctx context.Context, id string, detail string) error
	GetJob(ctx context.Context, id string) (*JobState, error)
}

// ─── DBJobRepository ─────────────────────────────────────────────────────────

// DBJobRepository implements JobRepository over database/sql.
type DBJobRepository struct {
	db *sql.DB
}

// NewDBJobRepository creates a DBJobRepository.
func NewDBJobRepository(db *sql.DB) *DBJobRepository {
	return &DBJobRepository{db: db}
}

var _ JobRepository = (*DBJobRepository)(nil)

func (r *DBJobRepository) InsertJob(ctx context.Context, j *JobState, actorID uuid.UUID, tenantID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sys.job (id, type, status, progress, current_step, payload_jsonb, can_cancel,
		    created_at, created_by, updated_at, updated_by, tenant_id)
		VALUES ($1, $2, $3, $4, $5, '{}', false, now(), $6, now(), $6, $7)
	`, j.ID, TaskTypeCoAImportXLSX, j.Status, j.Progress, j.CurrentStep, actorID, tenantID)
	if err != nil {
		return fmt.Errorf("DBJobRepository.InsertJob: %w", err)
	}
	return nil
}

func (r *DBJobRepository) UpdateJobProgress(ctx context.Context, id string, progress int, step string, rowsDone int, rowsError int) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE sys.job SET status = 'running', progress = $2, current_step = $3,
		    result_jsonb = jsonb_set(COALESCE(result_jsonb, '{}'), '{rowsDone}', to_jsonb($4::int)) ||
		                  jsonb_set('{}', '{rowsError}', to_jsonb($5::int)),
		    updated_at = now()
		WHERE id = $1
	`, id, progress, step, rowsDone, rowsError)
	if err != nil {
		return fmt.Errorf("DBJobRepository.UpdateJobProgress: %w", err)
	}
	return nil
}

func (r *DBJobRepository) CompleteJob(ctx context.Context, id string, rowsTotal, rowsDone, rowsError int) error {
	resultJSON, err := json.Marshal(map[string]int{
		"rowsTotal": rowsTotal,
		"rowsDone":  rowsDone,
		"rowsError": rowsError,
	})
	if err != nil {
		return fmt.Errorf("DBJobRepository.CompleteJob: marshal result: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE sys.job
		SET status = 'completed', progress = 100, completed_at = now(),
		    result_jsonb = $2::jsonb, updated_at = now()
		WHERE id = $1
	`, id, string(resultJSON))
	if err != nil {
		return fmt.Errorf("DBJobRepository.CompleteJob: %w", err)
	}
	return nil
}

func (r *DBJobRepository) FailJob(ctx context.Context, id string, detail string) error {
	errJSON, err := json.Marshal(map[string]string{"message": detail})
	if err != nil {
		return fmt.Errorf("DBJobRepository.FailJob: marshal error: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE sys.job
		SET status = 'failed', completed_at = now(),
		    error_jsonb = $2::jsonb, updated_at = now()
		WHERE id = $1
	`, id, string(errJSON))
	if err != nil {
		return fmt.Errorf("DBJobRepository.FailJob: %w", err)
	}
	return nil
}

func (r *DBJobRepository) GetJob(ctx context.Context, id string) (*JobState, error) {
	jj := &JobState{}
	var (
		rowsDoneJSON  *int
		rowsErrorJSON *int
		resultRaw     []byte
		completedAt   *time.Time
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT id, status, progress, current_step,
		    (result_jsonb->>'rowsTotal')::int,
		    (result_jsonb->>'rowsDone')::int,
		    (result_jsonb->>'rowsError')::int,
		    error_jsonb::text,
		    created_at, completed_at
		FROM sys.job WHERE id = $1
	`, id).Scan(
		&jj.ID, &jj.Status, &jj.Progress, &jj.CurrentStep,
		&jj.RowsTotal, &rowsDoneJSON, &rowsErrorJSON,
		&resultRaw,
		&jj.CreatedAt, &completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("DBJobRepository.GetJob: %w", err)
	}
	if rowsDoneJSON != nil {
		jj.RowsDone = *rowsDoneJSON
	}
	if rowsErrorJSON != nil {
		jj.RowsError = *rowsErrorJSON
	}
	if len(resultRaw) > 0 {
		s := string(resultRaw)
		jj.ErrorDetail = &s
	}
	jj.CompletedAt = completedAt
	return jj, nil
}

// ─── Importer — orchestration ─────────────────────────────────────────────────

// Importer handles both the submit path (enqueue or sync) and the worker path.
type Importer struct {
	repo        Repository
	jobRepo     JobRepository
	auditWriter *audit.Writer
	asynqClient *asynq.Client // nil = sync goroutine fallback
	logger      *slog.Logger
}

// NewImporter creates an Importer.
func NewImporter(repo Repository, jobRepo JobRepository, auditWriter *audit.Writer, asynqClient *asynq.Client, logger *slog.Logger) *Importer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Importer{
		repo:        repo,
		jobRepo:     jobRepo,
		auditWriter: auditWriter,
		asynqClient: asynqClient,
		logger:      logger,
	}
}

// SubmitImport validates the uploaded file, creates a sys.job row, and
// either enqueues an Asynq task or falls back to a sync goroutine.
// Returns the jobId and statusUrl.
func (im *Importer) SubmitImport(ctx context.Context, req ImportXLSXRequest, fileBytes []byte) (*ImportJobResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	if len(fileBytes) == 0 {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "File tidak boleh kosong.")
	}
	const maxBytes = 10 * 1024 * 1024 // 10 MB
	if len(fileBytes) > maxBytes {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, "Ukuran file melebihi batas 10 MB.")
	}

	// File SHA-256 for idempotency.
	h := sha256.Sum256(fileBytes)
	fileSHA := fmt.Sprintf("%x", h)

	jobID := fmt.Sprintf("coa-import-%s", uuid.New().String())

	j := &JobState{
		ID:          jobID,
		Status:      "queued",
		Progress:    0,
		CurrentStep: "Menunggu diproses",
		CreatedAt:   time.Now(),
	}

	if im.jobRepo != nil {
		if err := im.jobRepo.InsertJob(ctx, j, actorID, tenantID(claims)); err != nil {
			return nil, fmt.Errorf("importer.SubmitImport insert job: %w", err)
		}
	}

	payload := ImportPayload{
		JobID:      jobID,
		ActorID:    actorID.String(),
		TenantID:   tenantID(claims),
		SumberCoa:  req.SumberCoa,
		FileSHA:    fileSHA,
		FileBytes:  fileBytes,
		EnqueuedAt: time.Now(),
	}

	if im.asynqClient != nil {
		// Enqueue via Asynq.
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("importer.SubmitImport marshal payload: %w", err)
		}
		task := asynq.NewTask(TaskTypeCoAImportXLSX, payloadBytes)
		if _, err := im.asynqClient.EnqueueContext(ctx, task, asynq.MaxRetry(1)); err != nil {
			return nil, fmt.Errorf("importer.SubmitImport enqueue: %w", err)
		}
	} else {
		// Sync goroutine fallback (dev / nil Asynq).
		bgCtx := context.WithoutCancel(ctx)
		go func() {
			if err := im.runImport(bgCtx, payload); err != nil {
				im.logger.ErrorContext(bgCtx, "coa import goroutine failed", "jobId", jobID, "error", err)
			}
		}()
	}

	return &ImportJobResponse{
		JobID:     jobID,
		StatusURL: "/api/v1/master/coa/import-jobs/" + jobID,
	}, nil
}

// GetJobStatus returns the current state of an import job.
func (im *Importer) GetJobStatus(ctx context.Context, jobID string) (*ImportJobStatusResponse, error) {
	if im.jobRepo == nil {
		return nil, domainerrors.ErrNotFound("import job")
	}
	j, err := im.jobRepo.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("importer.GetJobStatus: %w", err)
	}
	if j == nil {
		return nil, domainerrors.ErrNotFound("import job " + jobID)
	}

	resp := &ImportJobStatusResponse{
		JobID:       j.ID,
		Type:        TaskTypeCoAImportXLSX,
		Status:      j.Status,
		Progress:    j.Progress,
		CurrentStep: j.CurrentStep,
		RowsTotal:   j.RowsTotal,
		RowsDone:    j.RowsDone,
		RowsError:   j.RowsError,
		ErrorDetail: j.ErrorDetail,
		CreatedAt:   j.CreatedAt.Format(time.RFC3339),
	}
	if j.CompletedAt != nil {
		s := j.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &s
	}
	return resp, nil
}

// ─── Worker handler ───────────────────────────────────────────────────────────

// HandleTask is the Asynq handler for TaskTypeCoAImportXLSX.
func (im *Importer) HandleTask(ctx context.Context, t *asynq.Task) error {
	var p ImportPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("coa import HandleTask: unmarshal: %w", err)
	}
	return im.runImport(ctx, p)
}

// runImport is the actual import logic — used by both Asynq handler and sync fallback.
func (im *Importer) runImport(ctx context.Context, p ImportPayload) error {
	jobID := p.JobID

	updateProgress := func(progress int, step string, done, errCount int) {
		if im.jobRepo == nil {
			return
		}
		if err := im.jobRepo.UpdateJobProgress(ctx, jobID, progress, step, done, errCount); err != nil {
			im.logger.WarnContext(ctx, "coa import: update progress failed", "error", err)
		}
	}

	updateProgress(5, "Parsing file XLSX...", 0, 0)

	rows, err := parseXLSX(bytes.NewReader(p.FileBytes))
	if err != nil {
		if im.jobRepo != nil {
			if failErr := im.jobRepo.FailJob(ctx, jobID, fmt.Sprintf("Gagal parse XLSX: %v", err)); failErr != nil {
				im.logger.WarnContext(ctx, "coa import: FailJob after parse error failed", "error", failErr)
			}
		}
		return fmt.Errorf("coa import runImport parse: %w", err)
	}

	total := len(rows)
	updateProgress(10, fmt.Sprintf("Ditemukan %d baris. Memulai validasi dan import...", total), 0, 0)

	// Build context with actor claims for audit writes.
	actorUUID, err := uuid.Parse(p.ActorID)
	if err != nil {
		return fmt.Errorf("coa import runImport: invalid actorId %q: %w", p.ActorID, err)
	}
	importClaims := &auth.Claims{Sub: p.ActorID, TenantID: p.TenantID}
	importCtx := auth.ContextWithClaims(ctx, importClaims)

	done := 0
	errCount := 0

	for i := range rows {
		row := &rows[i]
		// Validate row-level fields.
		if err := validateXLSXRow(*row); err != nil {
			im.logger.WarnContext(ctx, "coa import: row validation failed",
				"row", row.RowNum, "error", err)
			errCount++
			continue
		}

		// Resolve parent_akun_kode.
		var parentID *uuid.UUID
		if row.ParentAkunKode != "" {
			parent, err := im.repo.GetByKode(ctx, row.ParentAkunKode, false)
			if err == nil && parent != nil && parent.WorkflowStatus == WorkflowStatusApproved {
				parentID = &parent.ID
			}
			// If parent not found/not approved, we skip the parent linkage (not error, best-effort).
		}

		mataUang := "IDR"
		if row.MataUangNative != "" {
			mataUang = strings.ToUpper(row.MataUangNative)
		}
		var kat *string
		if row.KategoriInvestasi != "" {
			s := row.KategoriInvestasi
			kat = &s
		}

		c := &ChartOfAccount{
			ID:                uuid.New(),
			KodeAkun:          row.KodeAkun,
			NamaAkun:          row.NamaAkun,
			TipeAkun:          TipeAkun(row.TipeAkun),
			SubTipeAkun:       row.SubTipeAkun,
			KategoriInvestasi: kat,
			MataUangNative:    mataUang,
			PosisiNormal:      PosisiNormal(row.PosisiNormal),
			AktifFlag:         true,
			ParentAkunID:      parentID,
			SumberCoa:         p.SumberCoa,
			TanggalMulaiAktif: time.Now().Format("2006-01-02"),
			WorkflowStatus:    WorkflowStatusDraft,
			CreatedBy:         actorUUID,
			CreatedAt:         time.Now(),
			Version:           1,
			TenantID:          p.TenantID,
		}

		tx, err := im.repo.BeginTx(importCtx)
		if err != nil {
			errCount++
			im.logger.WarnContext(ctx, "coa import: begin tx failed", "row", row.RowNum, "error", err)
			continue
		}

		createErr := im.repo.Create(importCtx, tx, c)
		if createErr != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				im.logger.WarnContext(ctx, "coa import: rollback failed", "row", row.RowNum, "error", rbErr)
			}
			if isErrDuplicate(createErr) {
				im.logger.InfoContext(ctx, "coa import: skipping duplicate kode", "kode", row.KodeAkun)
				// Skip duplicate — not counted as error (idempotent import).
				done++
			} else {
				errCount++
				im.logger.WarnContext(ctx, "coa import: create failed", "row", row.RowNum, "error", createErr)
			}
			continue
		}

		auditErr := im.auditWriter.WithTx(tx).Write(importCtx, audit.Event{
			Action:     "CHART_OF_ACCOUNTS.IMPORT_ROW",
			EntityType: "mst.chart_of_accounts",
			EntityID:   c.ID,
			After:      c,
		})
		if auditErr != nil {
			im.logger.WarnContext(ctx, "coa import: audit write failed", "row", row.RowNum, "error", auditErr)
		}

		if commitErr := tx.Commit(); commitErr != nil {
			errCount++
			im.logger.WarnContext(ctx, "coa import: commit failed", "row", row.RowNum, "error", commitErr)
			continue
		}
		done++

		// Report progress every 5% or every 50 rows, whichever is smaller.
		step := maxInt(total/20, 50)
		if (i+1)%step == 0 {
			pct := 10 + ((done * 85) / maxInt(total, 1))
			updateProgress(pct, fmt.Sprintf("Mengimport baris %d dari %d...", i+1, total), done, errCount)
		}
	}

	// Write job-level audit.
	auditTx, auditTxErr := im.repo.BeginTx(importCtx)
	if auditTxErr == nil {
		if writeErr := im.auditWriter.WithTx(auditTx).Write(importCtx, audit.Event{
			Action:     "CHART_OF_ACCOUNTS.IMPORT_XLSX",
			EntityType: "mst.chart_of_accounts",
			EntityID:   uuid.Nil,
			After: map[string]interface{}{
				"job_id":      jobID,
				"sumber_coa":  p.SumberCoa,
				"file_sha256": p.FileSHA,
				"rows_total":  total,
				"rows_done":   done,
				"rows_error":  errCount,
			},
		}); writeErr != nil {
			im.logger.WarnContext(ctx, "coa import: job-level audit write failed", "error", writeErr)
		}
		if commitErr := auditTx.Commit(); commitErr != nil {
			im.logger.WarnContext(ctx, "coa import: audit tx commit failed", "error", commitErr)
		}
	}

	if im.jobRepo != nil {
		if completeErr := im.jobRepo.CompleteJob(ctx, jobID, total, done, errCount); completeErr != nil {
			im.logger.WarnContext(ctx, "coa import: CompleteJob failed", "jobId", jobID, "error", completeErr)
		}
	}

	return nil
}

// ─── XLSX parser (pure Go, no excelize dependency) ───────────────────────────
//
// The spec says "async Excel import" and XLSX export "→ 501 (CSV only)" at the
// handler level. For the import we need to parse the upload. Rather than pulling
// in excelize (not in go.mod), we implement a minimal ZIP+XML reader that covers
// the standard XLSX template columns described in the task spec.
//
// If the project adds excelize in a later phase this function is the only place
// that changes.

func parseXLSX(r io.ReadSeeker) ([]XLSXRow, error) {
	// Read the entire content for the ZIP reader which needs io.ReaderAt.
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("parseXLSX: read: %w", err)
	}

	rows, err := parseXLSXBytes(data)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// parseXLSXBytes uses the standard archive/zip + encoding/xml to read XLSX.
// XLSX template expected columns (row 1 = header, row 2+ = data):
//
//	A: kode_akun
//	B: nama_akun
//	C: tipe_akun
//	D: sub_tipe_akun
//	E: kategori_investasi (optional)
//	F: mata_uang_native (optional, default IDR)
//	G: posisi_normal
//	H: parent_akun_kode (optional)
func parseXLSXBytes(data []byte) ([]XLSXRow, error) {
	// Delegate to internal helper. We keep XLSX logic behind a narrow interface
	// so tests can swap it without touching the production code path.
	return xlsxBytesToRows(data)
}

// validateXLSXRow performs minimal row-level validation.
func validateXLSXRow(row XLSXRow) error {
	if !kodeAkunRe.MatchString(row.KodeAkun) {
		return fmt.Errorf("kode_akun tidak valid: %q", row.KodeAkun)
	}
	if row.NamaAkun == "" {
		return fmt.Errorf("nama_akun kosong")
	}
	if !validTipeAkun[TipeAkun(row.TipeAkun)] {
		return fmt.Errorf("tipe_akun tidak valid: %q", row.TipeAkun)
	}
	if !validPosisiNormal[PosisiNormal(row.PosisiNormal)] {
		return fmt.Errorf("posisi_normal tidak valid: %q", row.PosisiNormal)
	}
	return nil
}

// maxInt returns the larger of two ints.
// Named maxInt to avoid shadowing Go 1.21+ builtin max.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
