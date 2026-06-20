package akrualmaturity

// handler.go — Thin HTTP handlers for 7 akrualmaturity endpoints.
// No business logic — parse request → call service → map result to envelope.
//
// Endpoints:
//   GET  /transaksi/akrual                    → ListAkrual      (akrual.read)
//   GET  /transaksi/akrual/dashboard          → GetDashboard    (akrual.read) STATIC — before /:id
//   GET  /transaksi/akrual/:id                → GetAkrualByID   (akrual.read)
//   POST /transaksi/akrual/:id/override-stale → OverrideStale   (akrual.override_stale)
//   GET  /transaksi/jatuh-tempo               → ListJatuhTempo  (maturity.read)
//   POST /transaksi/jatuh-tempo/cron-trigger  → TriggerMaturity (sys.cron.trigger)
//   POST /transaksi/akrual/cron-trigger       → TriggerAkrual   (sys.cron.trigger)

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// HTTPHandler is the akrualmaturity HTTP handler.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new akrualmaturity HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// AllowedAkrualSortCols is the whitelist for sort on GET /transaksi/akrual.
var AllowedAkrualSortCols = []string{
	"tanggal_akrual", "akrual_idr", "instrumen_id", "stage", "status", "created_at",
}

// AllowedAkrualFilterCols is the whitelist for filter on GET /transaksi/akrual.
var AllowedAkrualFilterCols = []string{
	"instrumen_id", "tanggal_akrual", "stage", "status", "jenis",
	"has_stale_flag", "periode_bulanan_id",
}

// AllowedJatuhTempoSortCols is the whitelist for sort on GET /transaksi/jatuh-tempo.
var AllowedJatuhTempoSortCols = []string{
	"tanggal_jatuh_tempo", "instrumen_id", "status", "created_at",
}

// ─── GET /transaksi/akrual ────────────────────────────────────────────────────

// ListAkrual handles GET /api/v1/transaksi/akrual.
// Permission: akrual.read
func (h *HTTPHandler) ListAkrual(c *gin.Context) {
	allCols := append(AllowedAkrualSortCols, AllowedAkrualFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	cursor := c.Query("cursor")
	limit := parseLimit(c)

	rows, hasMore, total, svcErr := h.svc.ListAkrual(c.Request.Context(), q, cursor, limit)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	items := make([]AkrualListItem, 0, len(rows))
	for _, a := range rows {
		items = append(items, ToAkrualListItem(a, ""))
	}

	// Count stale items
	staleCount := 0
	for _, a := range rows {
		if a.StaleStagingFlag {
			staleCount++
		}
	}

	totalEst := int64(total)
	pagination := &response.PaginationMeta{
		HasMore:       hasMore,
		TotalEstimate: &totalEst,
		Limit:         limit,
	}
	c.JSON(200, gin.H{
		"data":       items,
		"pagination": pagination,
		"staleCount": staleCount,
		"meta":       gin.H{"traceId": traceIDFromCtx(c)},
	})
}

// ─── GET /transaksi/akrual/dashboard ─────────────────────────────────────────

// GetDashboard handles GET /api/v1/transaksi/akrual/dashboard.
// STATIC — must be registered before /:id.
// Permission: akrual.read
func (h *HTTPHandler) GetDashboard(c *gin.Context) {
	var instrumenID *uuid.UUID
	var portofolioID *uuid.UUID

	if s := c.Query("instrumen_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "instrumen_id bukan UUID valid."))
			return
		}
		instrumenID = &id
	}
	if s := c.Query("portofolio_id"); s != "" {
		id, err := uuid.Parse(s)
		if err != nil {
			response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, "portofolio_id bukan UUID valid."))
			return
		}
		portofolioID = &id
	}

	year := time.Now().Year()
	month := int(time.Now().Month())
	if v := c.Query("year"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			year = n
		}
	}
	if v := c.Query("month"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 12 {
			month = n
		}
	}

	dash, svcErr := h.svc.GetDashboard(c.Request.Context(), instrumenID, portofolioID, year, month)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, dash)
}

// ─── GET /transaksi/akrual/cron-trigger ──────────────────────────────────────

// TriggerAkrualCron handles POST /api/v1/transaksi/akrual/cron-trigger.
// STATIC — must be registered before /:id.
// Returns 202 Accepted + mock jobId (real Asynq enqueue in main.go).
// Permission: sys.cron.trigger (ROLE-IT-ADMIN)
func (h *HTTPHandler) TriggerAkrualCron(c *gin.Context) {
	tanggal := time.Now().UTC()
	if s := c.Query("tanggal"); s != "" {
		if t, err := ParseDateStrict(s); err == nil {
			tanggal = t
		}
	}
	// Enqueue via service — for now returns a synthetic job ID.
	// Production: inject Asynq client and enqueue actual job.
	jobID := uuid.New().String()
	c.JSON(202, gin.H{
		"data": gin.H{
			"jobId":      "job_" + jobID,
			"type":       "DAILY_ACCRUAL_JOB",
			"statusUrl":  "/api/v1/jobs/job_" + jobID,
			"streamUrl":  "/api/v1/jobs/job_" + jobID + "/stream",
			"tanggal":    tanggal.Format("2006-01-02"),
		},
		"meta": gin.H{"traceId": traceIDFromCtx(c)},
	})
}

// ─── GET /transaksi/akrual/:id ────────────────────────────────────────────────

// GetAkrualByID handles GET /api/v1/transaksi/akrual/:id.
// Permission: akrual.read
func (h *HTTPHandler) GetAkrualByID(c *gin.Context) {
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	a, svcErr := h.svc.GetAkrualByID(c.Request.Context(), id)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, ToAkrualListItem(a, ""))
}

// ─── POST /transaksi/akrual/:id/override-stale ───────────────────────────────

// OverrideStale handles POST /api/v1/transaksi/akrual/:id/override-stale.
// Permission: akrual.override_stale (ROLE-AKUN-CTL)
func (h *HTTPHandler) OverrideStale(c *gin.Context) {
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	idempKey := c.GetHeader("Idempotency-Key")
	if idempKey == "" {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Idempotency-Key header wajib (DEC-021)."))
		return
	}

	var req OverrideStaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	updated, svcErr := h.svc.OverrideStaleAkrual(c.Request.Context(), id, req, idempKey)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, gin.H{
		"akrualId":      updated.ID.String(),
		"status":        string(updated.Status),
		"akrualIdr":     updated.BungaBersih.StringFixed(4),
		"jurnalEntryId": func() string {
			if updated.JurnalHeaderID != nil { return updated.JurnalHeaderID.String() }
			return ""
		}(),
	})
}

// ─── GET /transaksi/jatuh-tempo ───────────────────────────────────────────────

// ListJatuhTempo handles GET /api/v1/transaksi/jatuh-tempo.
// Permission: maturity.read
func (h *HTTPHandler) ListJatuhTempo(c *gin.Context) {
	allCols := append(AllowedJatuhTempoSortCols, "jenis", "status", "tanggal_jatuh_tempo") //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	cursor := c.Query("cursor")
	limit := parseLimit(c)

	rows, hasMore, total, svcErr := h.svc.ListJatuhTempo(c.Request.Context(), q, cursor, limit)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	items := make([]JatuhTempoListItem, 0, len(rows))
	for _, jt := range rows {
		items = append(items, ToJatuhTempoListItem(jt, ""))
	}

	totalEst := int64(total)
	pagination := &response.PaginationMeta{
		HasMore:       hasMore,
		TotalEstimate: &totalEst,
		Limit:         limit,
	}
	response.List(c, items, pagination, nil, nil)
}

// ─── POST /transaksi/jatuh-tempo/cron-trigger ────────────────────────────────

// TriggerMaturityCron handles POST /api/v1/transaksi/jatuh-tempo/cron-trigger.
// STATIC — must be registered before any /:id route.
// Permission: sys.cron.trigger (ROLE-IT-ADMIN)
func (h *HTTPHandler) TriggerMaturityCron(c *gin.Context) {
	var body struct {
		Tanggal string `json:"tanggal"`
	}
	_ = c.ShouldBindJSON(&body)

	tanggal := time.Now().UTC()
	if body.Tanggal != "" {
		if t, err := ParseDateStrict(body.Tanggal); err == nil {
			tanggal = t
		}
	}

	jobID := uuid.New().String()
	c.JSON(202, gin.H{
		"data": gin.H{
			"jobId":     "job_" + jobID,
			"type":      "MATURITY_PROCESS_JOB",
			"statusUrl": "/api/v1/jobs/job_" + jobID,
			"streamUrl": "/api/v1/jobs/job_" + jobID + "/stream",
			"tanggal":   tanggal.Format("2006-01-02"),
		},
		"meta": gin.H{"traceId": traceIDFromCtx(c)},
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func parseUUIDParam(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter '"+param+"' harus berupa UUID v4 yang valid.")
	}
	return id, nil
}

func parseLimit(c *gin.Context) int {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	return limit
}

func traceIDFromCtx(c *gin.Context) string {
	if v := c.GetHeader("X-Trace-Id"); v != "" {
		return v
	}
	return ""
}
