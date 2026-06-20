package penjualan

// handler.go — Thin HTTP handlers for 7 penjualan endpoints.
// No business logic — parse request → call service → map result to envelope.
//
// Endpoints:
//   GET    /trx/penjualan                          → List
//   POST   /trx/penjualan                          → Create      (penjualan.create)
//   GET    /trx/penjualan/bm-frequency-alerts      → GetBMAlerts (penjualan.read) STATIC — must register before /:id
//   GET    /trx/penjualan/:id                      → GetByID     (penjualan.read)
//   GET    /trx/penjualan/:id/preview              → GetPreview  (penjualan.read)
//   POST   /trx/penjualan/:id/approve              → Approve     (penjualan.approve)
//   POST   /trx/penjualan/:id/reject               → Reject      (penjualan.reject)

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// HTTPHandler is the penjualan HTTP handler.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new penjualan HTTPHandler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// ─── GET /trx/penjualan ──────────────────────────────────────────────────────

// List handles GET /api/v1/trx/penjualan.
// Permission: penjualan.read
func (h *HTTPHandler) List(c *gin.Context) {
	allCols := append(AllowedSortCols, AllowedFilterCols...) //nolint:gocritic
	q, err := listquery.ParseFromRequest(c.Request, allCols)
	if err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	cursor := c.Query("cursor")
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, parseErr := strconv.Atoi(v); parseErr == nil {
			limit = n
		}
	}

	rows, hasMore, total, svcErr := h.svc.GetList(c.Request.Context(), q, cursor, limit)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}

	items := make([]ListItem, 0, len(rows))
	for _, p := range rows {
		items = append(items, ToListItem(p, ""))
	}

	totalEstimate := int64(total)
	pagination := &response.PaginationMeta{
		HasMore:       hasMore,
		TotalEstimate: &totalEstimate,
		Limit:         limit,
	}
	response.List(c, items, pagination, nil, nil)
}

// ─── POST /trx/penjualan ─────────────────────────────────────────────────────

// Create handles POST /api/v1/trx/penjualan.
// Permission: penjualan.create
func (h *HTTPHandler) Create(c *gin.Context) {
	var req CreatePenjualanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	result, svcErr := h.svc.CreatePenjualan(c.Request.Context(), req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.Created(c, result)
}

// ─── GET /trx/penjualan/bm-frequency-alerts ──────────────────────────────────
// IMPORTANT: This STATIC route must be registered before /:id in routes.go
// to prevent Gin matching "bm-frequency-alerts" as a UUID path parameter.

// GetBMAlerts handles GET /api/v1/trx/penjualan/bm-frequency-alerts.
// Returns instrumen with BM violation risk for ROLE-RISK dashboard.
// Permission: penjualan.read
func (h *HTTPHandler) GetBMAlerts(c *gin.Context) {
	alerts, svcErr := h.svc.ListBMAlerts(c.Request.Context())
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	if alerts == nil {
		alerts = []*BMAlertItem{}
	}
	response.OK(c, alerts)
}

// ─── GET /trx/penjualan/:id ──────────────────────────────────────────────────

// GetByID handles GET /api/v1/trx/penjualan/:id.
// Permission: penjualan.read
func (h *HTTPHandler) GetByID(c *gin.Context) {
	id, err := parsePenjualanUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	detail, svcErr := h.svc.GetDetail(c.Request.Context(), id)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, detail)
}

// ─── GET /trx/penjualan/:id/preview ──────────────────────────────────────────

// GetPreview handles GET /api/v1/trx/penjualan/:id/preview.
// Read-only recompute — does not mutate state.
// Permission: penjualan.read
func (h *HTTPHandler) GetPreview(c *gin.Context) {
	id, err := parsePenjualanUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	preview, svcErr := h.svc.GetPreview(c.Request.Context(), id)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, preview)
}

// ─── POST /trx/penjualan/:id/approve ─────────────────────────────────────────

// Approve handles POST /api/v1/trx/penjualan/:id/approve.
// SoD: approver_id ≠ maker_id enforced in service (DEC-017).
// signatureMethod must be "JWT_STEP_UP".
// Permission: penjualan.approve
func (h *HTTPHandler) Approve(c *gin.Context) {
	id, err := parsePenjualanUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req ApprovePenjualanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	result, svcErr := h.svc.Approve(c.Request.Context(), id, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── POST /trx/penjualan/:id/reject ──────────────────────────────────────────

// Reject handles POST /api/v1/trx/penjualan/:id/reject.
// reason: min 30 chars.
// Permission: penjualan.reject
func (h *HTTPHandler) Reject(c *gin.Context) {
	id, err := parsePenjualanUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req RejectPenjualanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	result, svcErr := h.svc.Reject(c.Request.Context(), id, req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.OK(c, result)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// parsePenjualanUUID parses a UUID path parameter and returns a domain error on bad input.
func parsePenjualanUUID(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter '"+param+"' harus berupa UUID v4 yang valid.")
	}
	return id, nil
}
