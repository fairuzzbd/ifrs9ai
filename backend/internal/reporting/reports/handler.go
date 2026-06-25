package reports

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ReportHandler provides the 3 HTTP endpoints for P5-M14 reports:
//
//	GET  /reports/:slug         → GetReport (list)
//	GET  /reports/:slug/export  → ExportReport (inline or async)
//	POST /reports/rpt-28/export → ExportRegulatorPackHandler (special)
type ReportHandler struct {
	svc *ReportService
}

// NewReportHandler creates a ReportHandler.
func NewReportHandler(svc *ReportService) *ReportHandler {
	return &ReportHandler{svc: svc}
}

// ─── GET /reports/:slug ───────────────────────────────────────────────────────

// GetReport handles list requests for any of the 25 report slugs.
func (h *ReportHandler) GetReport(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}

	slug := c.Param("slug")
	params := parseQueryParams(c)

	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	result, err := h.svc.List(ctx, slug, params)
	if err != nil {
		writeDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": result.Rows,
		"pagination": gin.H{
			"nextCursor":    result.Pagination.NextCursor,
			"hasMore":       result.Pagination.HasMore,
			"totalEstimate": result.Pagination.TotalEstimate,
			"limit":         result.Pagination.Limit,
		},
		"appliedSort":   result.AppliedSort,
		"appliedFilter": result.AppliedFilter,
		"meta":          gin.H{"traceId": c.GetString("X-Trace-Id")},
	})
}

// ─── GET /reports/:slug/export ────────────────────────────────────────────────

// ExportReport handles export for any of the 25 report slugs.
func (h *ReportHandler) ExportReport(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}

	slug := c.Param("slug")
	format := strings.ToLower(c.DefaultQuery("format", "csv"))
	params := parseQueryParams(c)

	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	result, err := h.svc.Export(ctx, slug, params, format)
	if err != nil {
		writeDomainError(c, err)
		return
	}

	if result.Job != nil {
		response.Accepted(c, result.Job)
		return
	}

	if result.Inline != nil {
		c.Header("Content-Disposition", `attachment; filename="`+result.Inline.Filename+`"`)
		c.Header("X-SHA256", result.Inline.SHA256Hex)
		c.Data(http.StatusOK, result.Inline.ContentType, result.Inline.Bytes)
		return
	}

	response.OK(c, gin.H{"message": "export complete"})
}

// ─── POST /reports/rpt-28/export ─────────────────────────────────────────────

// ExportRegulatorPackHandler handles the RPT-28 special composite export.
// Requires X-Step-Up-Token header and report.rpt-28.export permission.
func (h *ReportHandler) ExportRegulatorPackHandler(c *gin.Context) {
	claims := claimsFromGin(c)
	if claims == nil {
		return
	}

	// Inject step-up token into claims if present.
	stepUpToken := c.GetHeader("X-Step-Up-Token")
	if stepUpToken != "" {
		now := time.Now().Unix()
		claims.StepupVerifiedAt = &now
	}

	var req RegulatorPackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithStatus(c, http.StatusBadRequest,
			domainerrors.CodeValidationFailed, err.Error(), nil)
		return
	}

	ctx := auth.ContextWithClaims(c.Request.Context(), claims)
	jobRef, err := h.svc.ExportRegulatorPack(ctx, req, claims)
	if err != nil {
		writeDomainError(c, err)
		return
	}

	response.Accepted(c, jobRef)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// parseQueryParams extracts cursor, limit, sort, filters from Gin context.
func parseQueryParams(c *gin.Context) QueryParams {
	cursor := c.Query("cursor")
	limit := 50
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 {
		limit = l
	}

	var sort []SortSpec
	if sortStr := c.Query("sort"); sortStr != "" {
		for _, part := range strings.Split(sortStr, ",") {
			kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
			spec := SortSpec{Col: kv[0], Dir: "asc"}
			if len(kv) == 2 {
				spec.Dir = kv[1]
			}
			sort = append(sort, spec)
		}
	}

	// Parse filter[col]=val or filter[col]=op:val from query string.
	var filters []FilterSpec
	for key, vals := range c.Request.URL.Query() {
		if !strings.HasPrefix(key, "filter[") || !strings.HasSuffix(key, "]") {
			continue
		}
		col := key[7 : len(key)-1]
		for _, val := range vals {
			op := "eq"
			value := val
			if idx := strings.Index(val, ":"); idx > 0 {
				prefix := val[:idx]
				switch prefix {
				case "eq", "ne", "gt", "gte", "lt", "lte", "like", "is_null", "is_not_null", "between", "in":
					op = prefix
					value = val[idx+1:]
				}
			}
			filters = append(filters, FilterSpec{Col: col, Op: op, Value: value})
		}
	}

	// Extra params for report-specific logic.
	extra := map[string]string{
		"calc_run_id":      c.Query("calc_run_id"),
		"periode_id":       c.Query("periode_id"),
		"instrumen_id":     c.Query("instrumen_id"),
		"schedule_version": c.Query("schedule_version"),
		"w_good":           c.Query("w_good"),
		"w_normal":         c.Query("w_normal"),
		"w_bad":            c.Query("w_bad"),
		"q":                c.Query("q"),
	}

	return QueryParams{
		Cursor:  cursor,
		Limit:   limit,
		Sort:    sort,
		Filters: filters,
		Search:  c.Query("q"),
		Extra:   extra,
	}
}

// claimsFromGin extracts JWT claims from Gin context.
func claimsFromGin(c *gin.Context) *auth.Claims {
	val, ok := c.Get("claims")
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "JWT claims tidak ditemukan.", nil)
		return nil
	}
	claims, ok := val.(*auth.Claims)
	if !ok {
		response.ErrorWithStatus(c, http.StatusUnauthorized,
			domainerrors.CodeUnauthorized, "JWT claims format tidak valid.", nil)
		return nil
	}
	return claims
}

// writeDomainError maps a domain error to HTTP response.
func writeDomainError(c *gin.Context, err error) {
	if de, ok := domainerrors.IsDomainError(err); ok {
		c.JSON(de.HTTPStatus(), gin.H{
			"error": gin.H{
				"code":    de.Code(),
				"message": de.Error(),
				"traceId": c.GetString("X-Trace-Id"),
			},
		})
		return
	}
	response.ErrorWithStatus(c, http.StatusInternalServerError,
		domainerrors.CodeInternal, "Terjadi kesalahan internal.", nil)
}
