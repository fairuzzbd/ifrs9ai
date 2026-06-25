package reports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/reporting/exporter"
)

// ReportService is the generic service that wraps any Report implementation.
// It handles permission, audit, export (inline + async), and regulated-flag audit.
type ReportService struct {
	primary         *sql.DB
	replica         *sql.DB
	asynqClient     *asynq.Client
	aw              *audit.Writer
	logger          *slog.Logger
	inlineThreshold int64
	maxRows         int64
}

// NewReportService constructs a ReportService.
// replica may be nil (falls back to primary with WARN log).
func NewReportService(
	primary, replica *sql.DB,
	asynqClient *asynq.Client,
	aw *audit.Writer,
	logger *slog.Logger,
) *ReportService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReportService{
		primary:         primary,
		replica:         replica,
		asynqClient:     asynqClient,
		aw:              aw,
		logger:          logger,
		inlineThreshold: 10_000,
		maxRows:         100_000,
	}
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is returned from List.
type ListResult struct {
	Rows          []map[string]any
	Pagination    Pagination
	AppliedSort   []SortSpec
	AppliedFilter []FilterSpec
}

// List executes report.Query on the read-replica, checks permission,
// writes regulated-flag audit in-tx if needed.
// Returns REPORT_NOT_FOUND, REPORT_PERMISSION_DENIED, REPORT_PARAMS_INVALID on guard failures.
func (s *ReportService) List(ctx context.Context, slug string, params QueryParams) (*ListResult, error) {
	r, err := s.lookupReport(slug)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	if err = s.checkReadPermission(claims, r); err != nil {
		return nil, err
	}

	if err = ValidateSortFilter(r, params); err != nil {
		return nil, domainerrors.New(domainerrors.CodeReportParamsInvalid, err.Error())
	}

	db := s.chooseDB()
	if db == nil {
		return nil, domainerrors.New(domainerrors.CodeInternal, "database not configured")
	}

	// Set statement timeout via session-level SET.
	if _, sErr := db.ExecContext(ctx, "SET LOCAL statement_timeout = '30s'"); sErr != nil {
		s.logger.WarnContext(ctx, "could not set statement_timeout", "err", sErr)
	}

	seq, totalEstimate, qErr := r.Query(ctx, db, params)
	if qErr != nil {
		return nil, s.mapQueryError(qErr, slug)
	}

	// Collect rows (capped at limit+1 for hasMore detection).
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var rows []map[string]any
	var nextCursor *string
	for row := range seq {
		rows = append(rows, row)
		if len(rows) > limit {
			// hasMore: remove the extra row and set cursor.
			rows = rows[:limit]
			cur := buildCursor(rows)
			nextCursor = &cur
			break
		}
	}

	// Regulated flag audit — in its own tx (no existing tx in list path).
	if r.RegulatedFlag() && s.aw != nil {
		actorID := uuid.Nil
		if claims != nil {
			actorID, _ = uuid.Parse(claims.Sub)
		}
		action := "REPORT." + strings.ToUpper(strings.ReplaceAll(slug, "-", "_")) + "_VIEWED"
		_ = s.aw.Write(ctx, audit.Event{
			Action:     action,
			EntityType: "rpt.report",
			EntityID:   actorID,
			After: map[string]any{
				"slug":           slug,
				"params_filters": params.Filters,
				"params_sort":    params.Sort,
			},
		})
	}

	return &ListResult{
		Rows: rows,
		Pagination: Pagination{
			NextCursor:    nextCursor,
			HasMore:       nextCursor != nil,
			TotalEstimate: totalEstimate,
			Limit:         limit,
		},
		AppliedSort:   params.Sort,
		AppliedFilter: params.Filters,
	}, nil
}

// ─── Export ──────────────────────────────────────────────────────────────────

// ExportAsyncPayload is the Asynq task payload for report async exports.
type ExportAsyncPayload struct {
	Slug     string `json:"slug"`
	Format   string `json:"format"`
	ActorID  string `json:"actor_id"`
	TenantID string `json:"tenant_id"`
	Params   string `json:"params_json"` // JSON-encoded QueryParams
}

const TaskReportExportAsync = "reports:export-async"

// ExportInlineResult holds the inline export bytes.
type ExportInlineResult struct {
	Bytes       []byte
	SHA256Hex   string
	ContentType string
	Filename    string
}

// ExportResult is either inline or async.
type ExportResult struct {
	Inline *ExportInlineResult
	Job    *AsyncJobRef
}

// AsyncJobRef mirrors reporting.AsyncJobRef to avoid circular imports.
type AsyncJobRef struct {
	JobID     string `json:"jobId"`
	StatusURL string `json:"statusUrl"`
	StreamURL string `json:"streamUrl"`
}

// Export performs permission check, estimates count, then dispatches inline or async.
// Writes EXPORT.GENERATED audit in-tx for inline path.
// For async path, audit is written by the worker on completion.
func (s *ReportService) Export(ctx context.Context, slug string, params QueryParams, format string) (*ExportResult, error) {
	r, err := s.lookupReport(slug)
	if err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	if err = s.checkExportPermission(claims, r); err != nil {
		return nil, err
	}

	if !isValidFormat(format) {
		return nil, domainerrors.New(domainerrors.CodeExportFormatUnsupported,
			"Format '"+format+"' tidak didukung. Gunakan csv, xlsx, atau pdf.")
	}

	db := s.chooseDB()
	_, totalEstimate, qErr := r.Query(ctx, db, params)
	if qErr != nil {
		return nil, s.mapQueryError(qErr, slug)
	}

	if totalEstimate > s.maxRows {
		return nil, domainerrors.New(domainerrors.CodeExportTooLarge,
			fmt.Sprintf("Dataset %d rows melebihi batas %d. Gunakan filter.", totalEstimate, s.maxRows))
	}

	if totalEstimate <= s.inlineThreshold {
		return s.buildInline(ctx, r, db, params, format, claims)
	}
	return s.enqueueAsync(ctx, slug, format, params, claims)
}

func (s *ReportService) buildInline(ctx context.Context, r Report, db *sql.DB, params QueryParams, format string, claims *auth.Claims) (*ExportResult, error) {
	seq, _, err := r.Query(ctx, db, params)
	if err != nil {
		return nil, s.mapQueryError(err, r.Slug())
	}

	cols := r.Columns()
	headers := make([]string, len(cols))
	for i, c := range cols {
		headers[i] = c.Header
	}

	var rows [][]string
	for row := range seq {
		row2 := make([]string, len(cols))
		for i, c := range cols {
			row2[i] = fmt.Sprintf("%v", row[c.Key])
		}
		rows = append(rows, row2)
	}

	username := ""
	if claims != nil {
		username = claims.PreferredUsername
	}
	exportedAt := time.Now()

	var fileBytes []byte
	var sha256Hex string
	var contentType string

	switch format {
	case "csv":
		var buf bytes.Buffer
		sha256Hex, err = exporter.ExportCSV(&buf, exporter.ExportCSVOptions{
			Headers:    headers,
			Rows:       rows,
			ExportedAt: exportedAt,
			Username:   username,
		})
		if err != nil {
			return nil, fmt.Errorf("buildInline csv: %w", err)
		}
		fileBytes = buf.Bytes()
		contentType = "text/csv; charset=UTF-8"

	case "xlsx":
		fileBytes, sha256Hex, err = exporter.ExportXLSX(exporter.ExportXLSXOptions{
			SheetName:  r.Slug(),
			Headers:    headers,
			Rows:       rows,
			ExportedAt: exportedAt,
			Username:   username,
		})
		if err != nil {
			return nil, fmt.Errorf("buildInline xlsx: %w", err)
		}
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

	case "pdf":
		fileBytes, sha256Hex, err = exporter.ExportPDF(exporter.ExportPDFOptions{
			Title:      r.Slug(),
			Headers:    headers,
			Rows:       rows,
			ExportedAt: exportedAt,
			Username:   username,
		})
		if err != nil {
			return nil, fmt.Errorf("buildInline pdf: %w", err)
		}
		contentType = "application/pdf"
	}

	// Write EXPORT.GENERATED audit (best-effort, separate auto-commit tx).
	if s.aw != nil {
		_ = s.aw.Write(ctx, audit.Event{
			Action:     "EXPORT.GENERATED",
			EntityType: "sys.export_log",
			EntityID:   uuid.Nil,
			After: map[string]any{
				"report_slug": r.Slug(),
				"format":      format,
				"row_count":   len(rows),
				"sha256":      sha256Hex,
			},
		})
	}

	filename := r.Slug() + "-" + time.Now().Format("20060102") + "." + format
	return &ExportResult{
		Inline: &ExportInlineResult{
			Bytes:       fileBytes,
			SHA256Hex:   sha256Hex,
			ContentType: contentType,
			Filename:    filename,
		},
	}, nil
}

func (s *ReportService) enqueueAsync(ctx context.Context, slug, format string, params QueryParams, claims *auth.Claims) (*ExportResult, error) {
	if s.asynqClient == nil {
		return nil, domainerrors.New(domainerrors.CodeInternal, "asynq client not configured")
	}

	actorID := ""
	tenantID := "TUGURE"
	if claims != nil {
		actorID = claims.Sub
		if claims.TenantID != "" {
			tenantID = claims.TenantID
		}
	}

	paramsJSON, _ := json.Marshal(params)
	payload := ExportAsyncPayload{
		Slug:     slug,
		Format:   format,
		ActorID:  actorID,
		TenantID: tenantID,
		Params:   string(paramsJSON),
	}
	payloadBytes, _ := json.Marshal(payload)

	jobID := uuid.New().String()
	task := asynq.NewTask(TaskReportExportAsync, payloadBytes,
		asynq.MaxRetry(1),
		asynq.Timeout(60*time.Minute),
		asynq.TaskID(jobID),
	)
	if _, err := s.asynqClient.EnqueueContext(ctx, task); err != nil {
		return nil, fmt.Errorf("enqueueAsync: %w", err)
	}

	return &ExportResult{
		Job: &AsyncJobRef{
			JobID:     jobID,
			StatusURL: "/api/v1/jobs/" + jobID,
			StreamURL: "/api/v1/jobs/" + jobID + "/stream",
		},
	}, nil
}

// ─── RPT-28 Regulator Pack ───────────────────────────────────────────────────

// RegulatorPackRequest is the parsed body for POST /reports/rpt-28/export.
type RegulatorPackRequest struct {
	PeriodeID     string   `json:"periode_id"     binding:"required"`
	Format        string   `json:"format"`
	IncludeSheets []string `json:"include_sheets"`
}

// ExportRegulatorPack validates step-up MFA, checks permission, enqueues always-async job.
func (s *ReportService) ExportRegulatorPack(ctx context.Context, req RegulatorPackRequest, claims *auth.Claims) (*AsyncJobRef, error) {
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan")
	}

	// Step-up MFA check (DEC-027).
	if claims.NeedsStepUp() {
		return nil, domainerrors.New(domainerrors.CodeStepUpRequired,
			"Step-up MFA wajib untuk RPT-28 export.")
	}

	// Permission: report.rpt-28.export (ROLE-CFO only in practice).
	if !claims.HasPermission("report.rpt-28.export") && !claims.HasPermission("report.*.export") {
		return nil, domainerrors.New(domainerrors.CodeReportPermissionDenied,
			"Tidak punya permission 'report.rpt-28.export'. Hanya ROLE-CFO.")
	}

	if req.PeriodeID == "" {
		return nil, domainerrors.New(domainerrors.CodeReportParamsInvalid,
			"periode_id wajib diisi untuk RPT-28 export.")
	}
	if req.Format == "" {
		req.Format = "xlsx"
	}
	if req.Format != "xlsx" {
		return nil, domainerrors.New(domainerrors.CodeReportParamsInvalid,
			"RPT-28 hanya mendukung format xlsx.")
	}

	if s.asynqClient == nil {
		return nil, domainerrors.New(domainerrors.CodeInternal, "asynq client not configured")
	}

	tenantID := "TUGURE"
	if claims.TenantID != "" {
		tenantID = claims.TenantID
	}

	payload := map[string]any{
		"slug":           "rpt-28",
		"periode_id":     req.PeriodeID,
		"format":         req.Format,
		"include_sheets": req.IncludeSheets,
		"actor_id":       claims.Sub,
		"tenant_id":      tenantID,
		"mfa_method":     claims.MFAMethod,
	}
	payloadBytes, _ := json.Marshal(payload)

	jobID := uuid.New().String()
	task := asynq.NewTask("reports:rpt28-regulator-pack", payloadBytes,
		asynq.MaxRetry(1),
		asynq.Timeout(120*time.Minute),
		asynq.TaskID(jobID),
	)
	if _, err := s.asynqClient.EnqueueContext(ctx, task); err != nil {
		return nil, fmt.Errorf("ExportRegulatorPack: enqueue: %w", err)
	}

	// Audit in-tx: EXPORT.REGULATOR_PACK_GENERATED (DEC-018).
	if s.aw != nil {
		actorID, _ := uuid.Parse(claims.Sub)
		_ = s.aw.Write(ctx, audit.Event{
			Action:     "EXPORT.REGULATOR_PACK_GENERATED",
			EntityType: "sys.export_log",
			EntityID:   actorID,
			After: map[string]any{
				"job_id":     jobID,
				"periode_id": req.PeriodeID,
				"mfa_method": claims.MFAMethod,
				"actor":      claims.Sub,
			},
		})
	}

	return &AsyncJobRef{
		JobID:     jobID,
		StatusURL: "/api/v1/jobs/" + jobID,
		StreamURL: "/api/v1/jobs/" + jobID + "/stream",
	}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *ReportService) lookupReport(slug string) (Report, error) {
	r, ok := Registry[slug]
	if !ok {
		return nil, domainerrors.New(domainerrors.CodeReportNotFound,
			"Laporan '"+slug+"' tidak ditemukan.")
	}
	return r, nil
}

func (s *ReportService) checkReadPermission(claims *auth.Claims, r Report) error {
	if claims == nil {
		return domainerrors.ErrUnauthorized("JWT claims tidak ditemukan")
	}
	// ROLE-AUDIT wildcard.
	if claims.HasPermission("report.*.read") || claims.HasPermission("audit_log.read") {
		return nil
	}
	if !claims.HasPermission(r.Permission()) {
		return domainerrors.New(domainerrors.CodeReportPermissionDenied,
			"Tidak punya permission '"+r.Permission()+"'.")
	}
	return nil
}

func (s *ReportService) checkExportPermission(claims *auth.Claims, r Report) error {
	if claims == nil {
		return domainerrors.ErrUnauthorized("JWT claims tidak ditemukan")
	}
	if claims.HasPermission("report.*.export") || claims.HasPermission("audit_log.read") {
		return nil
	}
	if !claims.HasPermission(r.ExportPermission()) {
		return domainerrors.New(domainerrors.CodeReportPermissionDenied,
			"Tidak punya permission '"+r.ExportPermission()+"'.")
	}
	return nil
}

func (s *ReportService) chooseDB() *sql.DB {
	if s.replica != nil {
		return s.replica
	}
	s.logger.Warn("MV_DSN not set — falling back to primary DSN for report query")
	return s.primary
}

func (s *ReportService) mapQueryError(err error, slug string) error {
	msg := err.Error()
	if strings.Contains(msg, "statement timeout") || strings.Contains(msg, "canceling statement due to statement timeout") {
		return domainerrors.New(domainerrors.CodeReportQueryTimeout,
			"Query melebihi batas 30 detik. Tambahkan filter periode_id atau instrumen_id untuk mempersempit data laporan "+slug+".")
	}
	return fmt.Errorf("report %s query: %w", slug, err)
}

func isValidFormat(f string) bool {
	switch f {
	case "csv", "xlsx", "pdf":
		return true
	}
	return false
}

func buildCursor(rows []map[string]any) string {
	if len(rows) == 0 {
		return ""
	}
	last := rows[len(rows)-1]
	id := last["id"]
	h := sha256.Sum256([]byte(fmt.Sprintf("%v", id)))
	return hex.EncodeToString(h[:16])
}
