package renewal

// handler.go — Thin HTTP handlers for 6 renewal endpoints.
// No business logic — parse request → call service → map result to envelope.
//
// Endpoints:
//   GET    /trx/renewal                → List
//   POST   /trx/renewal                → Create      (renewal.create)
//   GET    /trx/renewal/:id            → GetByID     (renewal.read)
//   GET    /trx/renewal/:id/preview    → GetPreview  (renewal.read — STATIC before /:id actions)
//   POST   /trx/renewal/:id/approve    → Approve     (renewal.approve)
//   POST   /trx/renewal/:id/reject     → Reject      (renewal.reject)

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// HTTPHandler is the renewal HTTP handler.
type HTTPHandler struct {
	svc *Service
}

// NewHTTPHandler creates a new renewal HTTP handler.
func NewHTTPHandler(svc *Service) *HTTPHandler {
	return &HTTPHandler{svc: svc}
}

// ─── GET /trx/renewal ─────────────────────────────────────────────────────────

// List handles GET /api/v1/trx/renewal.
// Permission: renewal.read
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
	for _, r := range rows {
		items = append(items, ToListItem(r, ""))
	}

	totalEstimate := int64(total)
	pagination := &response.PaginationMeta{
		HasMore:       hasMore,
		TotalEstimate: &totalEstimate,
		Limit:         limit,
	}
	response.List(c, items, pagination, nil, nil)
}

// ─── POST /trx/renewal ────────────────────────────────────────────────────────

// Create handles POST /api/v1/trx/renewal.
// Permission: renewal.create
func (h *HTTPHandler) Create(c *gin.Context) {
	var req CreateRenewalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}
	result, svcErr := h.svc.CreateRenewal(c.Request.Context(), req)
	if svcErr != nil {
		response.Error(c, svcErr)
		return
	}
	response.Created(c, result)
}

// ─── GET /trx/renewal/:id ─────────────────────────────────────────────────────

// GetByID handles GET /api/v1/trx/renewal/:id.
// Permission: renewal.read
func (h *HTTPHandler) GetByID(c *gin.Context) {
	id, err := parseRenewalUUID(c, "id")
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

// ─── GET /trx/renewal/:id/preview ─────────────────────────────────────────────

// GetPreview handles GET /api/v1/trx/renewal/:id/preview.
// Read-only recompute — does not mutate state.
// Permission: renewal.read
func (h *HTTPHandler) GetPreview(c *gin.Context) {
	id, err := parseRenewalUUID(c, "id")
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

// ─── POST /trx/renewal/:id/approve ────────────────────────────────────────────

// Approve handles POST /api/v1/trx/renewal/:id/approve.
// SoD: approver_id ≠ maker_id enforced in service.
// signatureMethod: must be "JWT_STEP_UP".
// Permission: renewal.approve
func (h *HTTPHandler) Approve(c *gin.Context) {
	id, err := parseRenewalUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req ApproveRenewalRequest
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

// ─── POST /trx/renewal/:id/reject ─────────────────────────────────────────────

// Reject handles POST /api/v1/trx/renewal/:id/reject.
// comment: min 30 chars.
// Permission: renewal.reject
func (h *HTTPHandler) Reject(c *gin.Context) {
	id, err := parseRenewalUUID(c, "id")
	if err != nil {
		response.Error(c, err)
		return
	}
	var req RejectRenewalRequest
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

// parseRenewalUUID parses a UUID path parameter.
func parseRenewalUUID(c *gin.Context, param string) (uuid.UUID, error) {
	raw := c.Param(param)
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			"Parameter '"+param+"' harus berupa UUID v4 yang valid.")
	}
	return id, nil
}
