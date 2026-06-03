package workflow

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler handles all generic workflow HTTP endpoints.
// Routes registered via RegisterRoutes.
type Handler struct {
	svc *Service
}

// NewHandler creates a workflow HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes wires the workflow endpoints under the given router group.
// The group should already have auth.Middleware applied.
//
// Pattern: POST /api/v1/{resource}/{id}/{action}
// GET  /api/v1/{resource}/{id}/workflow
//
// Permission check is done in each handler using the resource config.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	// All workflow mutation endpoints.
	rg.POST("/:resource/:id/submit", h.Submit)
	rg.POST("/:resource/:id/review", h.Review)
	rg.POST("/:resource/:id/approve", h.Approve)
	rg.POST("/:resource/:id/approve2", h.Approve2)
	rg.POST("/:resource/:id/reject", h.Reject)
	rg.GET("/:resource/:id/workflow", h.GetStatus)
}

// -----------------------------------------------------------------------
// Action handlers
// -----------------------------------------------------------------------

// Submit handles POST /{resource}/{id}/submit (DRAFT → PENDING_REVIEW).
func (h *Handler) Submit(c *gin.Context) {
	resource, entityID, ok := extractPathParams(c)
	if !ok {
		return
	}

	var body workflowActionBody
	if !bindJSON(c, &body) {
		return
	}

	// Permission check: {entity}.submit
	if !checkPermission(c, resource, "submit") {
		return
	}

	req := ActionRequest{
		Comment:         body.Comment,
		SignatureMethod:  parseSignatureMethod(body.SignatureMethod),
		RowVersion:      body.RowVersion,
	}

	result, err := h.svc.Submit(c.Request.Context(), SubmitInput{
		EntityType:  normalizeEntityType(resource),
		EntityID:    entityID,
		Request:     req,
		StepUpFresh: isStepUpFresh(c),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, toActionResultJSON(result))
}

// Review handles POST /{resource}/{id}/review (PENDING_REVIEW → PENDING_APPROVAL).
func (h *Handler) Review(c *gin.Context) {
	resource, entityID, ok := extractPathParams(c)
	if !ok {
		return
	}

	var body workflowActionBody
	if !bindJSON(c, &body) {
		return
	}

	if !checkPermission(c, resource, "review") {
		return
	}

	req := ActionRequest{
		Comment:         body.Comment,
		SignatureMethod:  parseSignatureMethod(body.SignatureMethod),
		RowVersion:      body.RowVersion,
	}

	result, err := h.svc.Review(c.Request.Context(), ReviewInput{
		EntityType:  normalizeEntityType(resource),
		EntityID:    entityID,
		Request:     req,
		StepUpFresh: isStepUpFresh(c),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, toActionResultJSON(result))
}

// Approve handles POST /{resource}/{id}/approve.
// For 4-eyes: PENDING_APPROVAL → APPROVED.
// For 6-eyes: PENDING_APPROVAL → PENDING_APPROVAL_2.
func (h *Handler) Approve(c *gin.Context) {
	resource, entityID, ok := extractPathParams(c)
	if !ok {
		return
	}

	var body workflowActionBody
	if !bindJSON(c, &body) {
		return
	}

	if !checkPermission(c, resource, "approve") {
		return
	}

	req := ActionRequest{
		Comment:         body.Comment,
		SignatureMethod:  parseSignatureMethod(body.SignatureMethod),
		RowVersion:      body.RowVersion,
	}

	result, err := h.svc.Approve(c.Request.Context(), ApproveInput{
		EntityType:  normalizeEntityType(resource),
		EntityID:    entityID,
		Request:     req,
		StepUpFresh: isStepUpFresh(c),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, toActionResultJSON(result))
}

// Approve2 handles POST /{resource}/{id}/approve2 (6-eyes second approver).
func (h *Handler) Approve2(c *gin.Context) {
	resource, entityID, ok := extractPathParams(c)
	if !ok {
		return
	}

	var body workflowActionBody
	if !bindJSON(c, &body) {
		return
	}

	if !checkPermission(c, resource, "approve") { // approve2 uses same permission as approve
		return
	}

	req := ActionRequest{
		Comment:         body.Comment,
		SignatureMethod:  parseSignatureMethod(body.SignatureMethod),
		RowVersion:      body.RowVersion,
	}

	result, err := h.svc.Approve2(c.Request.Context(), Approve2Input{
		EntityType:  normalizeEntityType(resource),
		EntityID:    entityID,
		Request:     req,
		StepUpFresh: isStepUpFresh(c),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, toActionResultJSON(result))
}

// Reject handles POST /{resource}/{id}/reject (any PENDING_* → REJECTED).
func (h *Handler) Reject(c *gin.Context) {
	resource, entityID, ok := extractPathParams(c)
	if !ok {
		return
	}

	var body workflowRejectBody
	if !bindJSON(c, &body) {
		return
	}

	// Reject: comment mandatory (minLength 10).
	if body.Comment == nil || len(*body.Comment) < 10 {
		traceID, _ := c.Get(response.TraceIDKey)
		c.JSON(http.StatusBadRequest, response.ErrorEnvelope{
			Error: response.ErrorBody{
				Code:    string(domainerrors.CodeValidationFailed),
				Message: "Alasan penolakan wajib diisi (minimal 10 karakter).",
				Details: []domainerrors.Detail{{
					Field:   "body.comment",
					Rule:    "required,min=10",
					Message: "Alasan penolakan minimal 10 karakter",
				}},
				TraceID: traceIDStr(traceID),
			},
		})
		return
	}

	if !checkPermission(c, resource, "reject") {
		return
	}

	rejectReq := RejectRequest{
		Comment:         *body.Comment,
		SignatureMethod:  parseSignatureMethod(body.SignatureMethod),
		RowVersion:      body.RowVersion,
	}

	result, err := h.svc.Reject(c.Request.Context(), RejectInput{
		EntityType:    normalizeEntityType(resource),
		EntityID:      entityID,
		RejectRequest: rejectReq,
		StepUpFresh:   isStepUpFresh(c),
	})
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, toActionResultJSON(result))
}

// GetStatus handles GET /{resource}/{id}/workflow.
func (h *Handler) GetStatus(c *gin.Context) {
	resource, entityID, ok := extractPathParams(c)
	if !ok {
		return
	}

	if !checkPermission(c, resource, "read") {
		return
	}

	status, err := h.svc.GetStatus(c.Request.Context(), normalizeEntityType(resource), entityID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.OK(c, toStatusResponseJSON(status))
}

// -----------------------------------------------------------------------
// Request/response JSON shapes (aligned with OpenAPI workflow.yaml)
// -----------------------------------------------------------------------

type workflowActionBody struct {
	Comment         *string `json:"comment"`
	SignatureMethod  string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

type workflowRejectBody struct {
	Comment         *string `json:"comment"`
	SignatureMethod  string  `json:"signatureMethod"`
	RowVersion      *int64  `json:"rowVersion"`
}

type actionResultJSON struct {
	EntityID        string   `json:"entityId"`
	EntityType      string   `json:"entityType"`
	PreviousState   string   `json:"previousState"`
	CurrentState    string   `json:"currentState"`
	Action          string   `json:"action"`
	PerformedBy     string   `json:"performedBy"`
	PerformedAt     string   `json:"performedAt"`
	Signature       sigJSON  `json:"signature"`
	NextActions     []string `json:"nextActions"`
	WorkflowEyes    int      `json:"workflowEyes"`
}

type sigJSON struct {
	SignatureHash   string `json:"signatureHash"`
	SignatureMethod string `json:"signatureMethod"`
}

func toActionResultJSON(r *ActionResult) actionResultJSON {
	return actionResultJSON{
		EntityID:       r.EntityID.String(),
		EntityType:     r.EntityType,
		PreviousState:  string(r.PreviousState),
		CurrentState:   string(r.CurrentState),
		Action:         string(r.Action),
		PerformedBy:    r.PerformedBy,
		PerformedAt:    r.PerformedAt.Format("2006-01-02T15:04:05Z07:00"),
		Signature:      sigJSON{SignatureHash: r.SignatureHash, SignatureMethod: string(r.SignatureMethod)},
		NextActions:    r.NextActions,
		WorkflowEyes:   r.WorkflowEyes,
	}
}

type statusResponseJSON struct {
	EntityID     string              `json:"entityId"`
	EntityType   string              `json:"entityType"`
	CurrentState string              `json:"currentState"`
	WorkflowEyes int                 `json:"workflowEyes"`
	MakerID      *string             `json:"makerId,omitempty"`
	ReviewerID   *string             `json:"reviewerId,omitempty"`
	Approver1ID  *string             `json:"approverId,omitempty"`
	Approver2ID  *string             `json:"approver2Id,omitempty"`
	History      []sigRecordJSON     `json:"history"`
}

type sigRecordJSON struct {
	Action          string  `json:"action"`
	UserID          string  `json:"userId"`
	Username        string  `json:"username"`
	Role            string  `json:"role"`
	SignedAt        string  `json:"signedAt"`
	SignatureHash   string  `json:"signatureHash"`
	SignatureMethod string  `json:"signatureMethod"`
	Comment         *string `json:"comment,omitempty"`
}

func toStatusResponseJSON(r *StatusResponse) statusResponseJSON {
	history := make([]sigRecordJSON, 0, len(r.History))
	for _, s := range r.History {
		history = append(history, sigRecordJSON{
			Action:          string(s.Action),
			UserID:          s.UserID.String(),
			Username:        s.Username,
			Role:            s.RoleAtTime,
			SignedAt:        s.SignedAt.Format("2006-01-02T15:04:05Z07:00"),
			SignatureHash:   s.SignatureHash,
			SignatureMethod: string(s.SignatureMethod),
			Comment:         s.Comment,
		})
	}

	j := statusResponseJSON{
		EntityID:     r.EntityID.String(),
		EntityType:   r.EntityType,
		CurrentState: string(r.CurrentState),
		WorkflowEyes: r.WorkflowEyes,
		History:      history,
	}
	if r.MakerID != nil {
		s := r.MakerID.String()
		j.MakerID = &s
	}
	if r.ReviewerID != nil {
		s := r.ReviewerID.String()
		j.ReviewerID = &s
	}
	if r.Approver1ID != nil {
		s := r.Approver1ID.String()
		j.Approver1ID = &s
	}
	if r.Approver2ID != nil {
		s := r.Approver2ID.String()
		j.Approver2ID = &s
	}
	return j
}

// -----------------------------------------------------------------------
// Handler helpers
// -----------------------------------------------------------------------

func extractPathParams(c *gin.Context) (resource string, entityID uuid.UUID, ok bool) {
	resource = c.Param("resource")
	idStr := c.Param("id")

	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Error(c, domainerrors.New(
			domainerrors.CodeValidationFailed,
			"Parameter 'id' harus berformat UUID v4.",
			domainerrors.Detail{Field: "path.id", Rule: "uuid", Message: "Format UUID tidak valid"},
		))
		return "", uuid.Nil, false
	}
	return resource, id, true
}

// checkPermission verifies {resource}.{action} permission from JWT claims.
// Returns false and writes 403 response if not permitted.
func checkPermission(c *gin.Context, resource, action string) bool {
	claims := auth.ClaimsFromGin(c)
	if claims == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeUnauthorized, "Claims tidak ada."))
		return false
	}

	permission := permissionFor(resource, action)
	if !claims.HasPermission(permission) {
		response.Error(c, domainerrors.ErrForbidden(permission))
		return false
	}
	return true
}

// permissionFor builds the permission string for a resource+action.
// e.g. resource="penempatan", action="submit" → "penempatan.submit"
func permissionFor(resource, action string) string {
	// Normalize kebab-case resource to snake_case permission prefix.
	r := strings.ReplaceAll(strings.ToLower(resource), "-", "_")
	return r + "." + strings.ToLower(action)
}

// normalizeEntityType converts a kebab-case resource param to upper snake_case
// entity type used in WorkflowConfig lookups.
// "ecl-parameter" → "ECL_PARAMETER"
func normalizeEntityType(resource string) string {
	return strings.ToUpper(strings.ReplaceAll(resource, "-", "_"))
}

// parseSignatureMethod converts a string to SignatureMethod, defaulting to JWT_STANDARD.
func parseSignatureMethod(s string) SignatureMethod {
	switch strings.ToUpper(s) {
	case "JWT_STEP_UP":
		return SignatureMethodJWTStepUp
	default:
		return SignatureMethodJWTStandard
	}
}

// isStepUpFresh checks the JWT claims in the Gin context for step-up freshness.
func isStepUpFresh(c *gin.Context) bool {
	claims := auth.ClaimsFromGin(c)
	if claims == nil {
		return false
	}
	return claims.IsStepUpFresh()
}

// bindJSON binds and validates the request body. Returns false and writes 400 if
// binding fails.
func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		traceID, _ := c.Get(response.TraceIDKey)
		c.JSON(http.StatusBadRequest, response.ErrorEnvelope{
			Error: response.ErrorBody{
				Code:    string(domainerrors.CodeValidationFailed),
				Message: "Request body tidak valid: " + err.Error(),
				Details: []domainerrors.Detail{},
				TraceID: traceIDStr(traceID),
			},
		})
		return false
	}
	return true
}

func traceIDStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
