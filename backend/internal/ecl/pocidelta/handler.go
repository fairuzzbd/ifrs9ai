package pocidelta

// handler.go — Thin HTTP handlers for POCI delta ECL endpoints.
// No business logic — parse request → call service → map result to envelope.
//
// Endpoints:
//   GET  /poci/baseline                   → ListBaseline         (poci.baseline.read)
//   POST /poci/baseline                   → CaptureBaseline      (poci.baseline.create)
//   GET  /poci/baseline/:instrumen_id     → GetBaseline          (poci.baseline.read)
//   GET  /poci/delta-log                  → ListDeltaLog         (poci.delta.read)
//   GET  /poci/delta-history              → GetDeltaHistory      (poci.delta.read)
//   GET  /poci/delta-history/summary      → GetDeltaSummary      (poci.delta.read) STATIC
//   POST /poci/compute-delta-batch        → ComputeDeltaBatch    (poci.delta.compute) SensitiveRateLimit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// HTTPHandler is the pocidelta HTTP handler.
type HTTPHandler struct {
	svc         *Service
	asynqClient *asynq.Client // real client required in production (M9 B2 lesson)
}

// NewHTTPHandler creates a new pocidelta HTTPHandler.
// asynqClient must be a real *asynq.Client (not nil) in production — POST
// /poci/compute-delta-batch returns 501 if asynqClient is nil.
func NewHTTPHandler(svc *Service, asynqClient *asynq.Client) *HTTPHandler {
	return &HTTPHandler{svc: svc, asynqClient: asynqClient}
}

// AllowedBaselineSortCols is the whitelist for sort on GET /poci/baseline.
var AllowedBaselineSortCols = []string{
	"tanggal_baseline", "lifetime_ecl_at_origination", "instrumen_id", "created_at",
}

// AllowedBaselineFilterCols is the whitelist for filter on GET /poci/baseline.
var AllowedBaselineFilterCols = []string{
	"instrumen_id", "tanggal_baseline",
}

// AllowedDeltaLogSortCols is the whitelist for sort on GET /poci/delta-log.
var AllowedDeltaLogSortCols = []string{
	"tanggal_compute", "delta_ecl", "instrumen_id", "direction", "status", "created_at",
}

// AllowedDeltaLogFilterCols is the whitelist for filter on GET /poci/delta-log.
var AllowedDeltaLogFilterCols = []string{
	"calc_run_id", "instrumen_id", "direction", "status", "periode_bulanan_id",
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// actorAndTenant extracts actorID (UUID) and tenantID (string) from Gin context.
// Falls back to uuid.Nil / "TUGURE" if auth middleware is absent (unit test scenario).
func actorAndTenant(c *gin.Context) (uuid.UUID, string) {
	claims := auth.ClaimsFromContext(c.Request.Context())
	if claims == nil {
		return uuid.Nil, "TUGURE"
	}
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		actorID = uuid.Nil
	}
	tenantID := claims.TenantID
	if tenantID == "" {
		tenantID = "TUGURE"
	}
	return actorID, tenantID
}

// ─── GET /poci/baseline ───────────────────────────────────────────────────────

func (h *HTTPHandler) ListBaseline(c *gin.Context) {
	allCols := append(AllowedBaselineSortCols, AllowedBaselineFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	_, tenantID := actorAndTenant(c)
	rows, _, svcErr := h.svc.ListBaseline(c.Request.Context(), q, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	items := make([]BaselineListItem, 0, len(rows))
	for i := range rows {
		items = append(items, ToBaselineListItem(&rows[i], ""))
	}
	response.OK(c, items)
}

// ─── POST /poci/baseline ──────────────────────────────────────────────────────

func (h *HTTPHandler) CaptureBaseline(c *gin.Context) {
	var req CaptureBaselineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	actorID, tenantID := actorAndTenant(c)

	baseline, svcErr := h.svc.CaptureBaseline(c.Request.Context(), nil, req, actorID, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.Created(c, ToBaselineListItem(baseline, ""))
}

// ─── GET /poci/baseline/:instrumen_id ─────────────────────────────────────────

func (h *HTTPHandler) GetBaseline(c *gin.Context) {
	instrumenID, err := uuid.Parse(c.Param("instrumen_id"))
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "instrumen_id bukan UUID valid"))
		return
	}
	_, tenantID := actorAndTenant(c)
	baseline, svcErr := h.svc.GetBaselineByInstrumen(c.Request.Context(), instrumenID, tenantID)
	if svcErr != nil {
		if IsCodeErr(svcErr, CodePociBaselineMissing) {
			response.Error(c, domainerrors.New(domainerrors.CodeNotFound, svcErr.Error()))
			return
		}
		response.Error(c, svcErr)
		return
	}
	response.OK(c, ToBaselineListItem(baseline, ""))
}

// ─── GET /poci/delta-log ──────────────────────────────────────────────────────

func (h *HTTPHandler) ListDeltaLog(c *gin.Context) {
	allCols := append(AllowedDeltaLogSortCols, AllowedDeltaLogFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	_, tenantID := actorAndTenant(c)
	rows, _, svcErr := h.svc.ListDeltaLog(c.Request.Context(), q, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	threshold := decimal.NewFromFloat(500000000) // default; non-fatal if threshold fetch fails
	if h.svc.repo != nil {
		t, _ := h.svc.repo.GetLargeDeltaThreshold(c.Request.Context(), tenantID)
		if t.IsPositive() {
			threshold = t
		}
	}
	items := make([]DeltaLogItem, 0, len(rows))
	for i := range rows {
		items = append(items, ToDeltaLogItem(&rows[i], "", threshold))
	}
	response.OK(c, items)
}

// ─── GET /poci/delta-history ──────────────────────────────────────────────────

func (h *HTTPHandler) GetDeltaHistory(c *gin.Context) {
	instrumenIDStr := c.Query("instrumen_id")
	instrumenID, err := uuid.Parse(instrumenIDStr)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "instrumen_id wajib diisi dan harus UUID valid"))
		return
	}
	allCols := append(AllowedDeltaLogSortCols, AllowedDeltaLogFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	_, tenantID := actorAndTenant(c)
	rows, _, svcErr := h.svc.GetDeltaHistory(c.Request.Context(), instrumenID, q, tenantID)
	if svcErr != nil {
		if IsCodeErr(svcErr, CodePociBaselineMissing) {
			response.Error(c, domainerrors.New(domainerrors.CodeNotFound, svcErr.Error()))
			return
		}
		response.Error(c, svcErr)
		return
	}
	threshold := decimal.NewFromFloat(500000000)
	items := make([]DeltaLogItem, 0, len(rows))
	for i := range rows {
		items = append(items, ToDeltaLogItem(&rows[i], "", threshold))
	}
	response.OK(c, items)
}

// ─── GET /poci/delta-history/summary ─────────────────────────────────────────

func (h *HTTPHandler) GetDeltaSummary(c *gin.Context) {
	yearStr := c.Query("year")
	monthStr := c.Query("month")
	year := parseIntDefault(yearStr, 0)
	month := parseIntDefault(monthStr, 0)
	if year < 2020 || year > 2100 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "year parameter tidak valid (YYYY, range 2020-2100)"))
		return
	}
	if month < 1 || month > 12 {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "month parameter tidak valid (1-12)"))
		return
	}
	var portofolioID *uuid.UUID
	if pStr := c.Query("portofolio_id"); pStr != "" {
		pid, pErr := uuid.Parse(pStr)
		if pErr != nil {
			response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "portofolio_id tidak valid UUID"))
			return
		}
		portofolioID = &pid
	}
	_, tenantID := actorAndTenant(c)
	summary, svcErr := h.svc.GetDeltaSummary(c.Request.Context(), portofolioID, year, month, tenantID)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, summary)
}

// ─── POST /poci/compute-delta-batch ──────────────────────────────────────────

func (h *HTTPHandler) ComputeDeltaBatch(c *gin.Context) {
	if h.asynqClient == nil {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error": gin.H{
				"code":    "INTERNAL",
				"message": "Asynq client tidak terkonfigurasi — wire real client di main.go (P5-M10 B2 lesson)",
			},
		})
		return
	}

	var req ComputeDeltaBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}

	actorID, _ := actorAndTenant(c)
	jobID := uuid.New()

	// tenantUUID: single-tenant Phase 1 (TUGURE).
	// Wire from config / JWT in Phase 2 multi-tenant.
	tenantUUID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	task, err := NewComputeDeltaTask(req.CalcRunID, actorID, tenantUUID, jobID)
	if err != nil {
		response.Error(c, err)
		return
	}

	if _, err := h.asynqClient.Enqueue(task); err != nil {
		response.Error(c, err)
		return
	}

	response.Accepted(c, gin.H{
		"jobId":     jobID.String(),
		"type":      "POCI_COMPUTE_DELTA_BATCH",
		"statusUrl": "/api/v1/jobs/" + jobID.String(),
		"streamUrl": "/api/v1/jobs/" + jobID.String() + "/stream",
		"startedAt": time.Now().UTC().Format(time.RFC3339),
	})
}
