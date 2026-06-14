package jurnal

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Handler holds all HTTP handlers for the jurnal engine (20 endpoints).
type Handler struct {
	mapping  *MappingService
	resolver *ResolverService
	posting  *PostingService
	dlq      *DLQService
}

// NewHandler creates a Handler. Panics on nil services.
func NewHandler(mapping *MappingService, resolver *ResolverService, posting *PostingService, dlq *DLQService) *Handler {
	if mapping == nil || resolver == nil || posting == nil || dlq == nil {
		panic("jurnal.NewHandler: all services must be non-nil")
	}
	return &Handler{mapping: mapping, resolver: resolver, posting: posting, dlq: dlq}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func claimsFromCtx(c *gin.Context) *auth.Claims {
	return auth.ClaimsFromContext(c.Request.Context())
}

func callerUUID(c *gin.Context) (uuid.UUID, error) {
	cl := claimsFromCtx(c)
	if cl == nil {
		return uuid.Nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	id, err := uuid.Parse(cl.Sub)
	if err != nil {
		return uuid.Nil, domainerrors.ErrUnauthorized("Sub claim bukan UUID valid.")
	}
	return id, nil
}

func parsePathUUID(c *gin.Context) (uuid.UUID, error) {
	raw := c.Param("id")
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Path param 'id' bukan UUID valid: %s", raw))
	}
	return id, nil
}

func parseListQuery(c *gin.Context, allowed []string) (listquery.Query, int, error) {
	q, err := listquery.ParseFromRequest(c.Request, allowed)
	if err != nil {
		return q, 0, err
	}
	limit := 50
	if l := c.Query("limit"); l != "" {
		v, e := strconv.Atoi(l)
		if e != nil || v < 1 || v > 200 {
			v = 50
		}
		limit = v
	}
	return q, limit, nil
}

func requirePermission(c *gin.Context, perm string) bool {
	cl := claimsFromCtx(c)
	if cl == nil || !cl.HasPermission(perm) {
		response.Error(c, domainerrors.ErrForbidden(perm))
		return false
	}
	return true
}

// ─── Mapping Header endpoints ──────────────────────────────────────────────────

// CreateMappingHeader POST /jurnal/mapping-headers
func (h *Handler) CreateMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingCreate) {
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req MappingHeaderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.mapping.Create(c.Request.Context(), req, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, result)
}

// ListMappingHeaders GET /jurnal/mapping-headers
func (h *Handler) ListMappingHeaders(c *gin.Context) {
	if !requirePermission(c, PermMappingRead) {
		return
	}
	q, limit, err := parseListQuery(c, AllowedMappingSortCols)
	if err != nil {
		response.Error(c, err)
		return
	}

	items, page, err := h.mapping.repo.ListSummary(c.Request.Context(), q, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, items, page.PaginationMeta(), toSortApplied(q), q.AppliedFilter())
}

// GetMappingHeader GET /jurnal/mapping-headers/:id
func (h *Handler) GetMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingRead) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, err := h.mapping.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if result == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan."))
		return
	}
	response.OK(c, result)
}

// EditMappingHeader PATCH /jurnal/mapping-headers/:id
func (h *Handler) EditMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingCreate) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req MappingHeaderEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	existing, err := h.mapping.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if existing == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Mapping header tidak ditemukan."))
		return
	}
	if existing.WorkflowStatus != MappingStatusDraft {
		response.Error(c, domainerrors.New(domainerrors.CodeJurnalInvalidTransition,
			"Hanya DRAFT yang bisa di-edit."))
		return
	}
	// Apply patch.
	if req.NamaEvent != nil {
		existing.NamaEvent = *req.NamaEvent
	}
	if req.KategoriEvent != nil {
		existing.KategoriEvent = *req.KategoriEvent
	}
	if req.KlasifikasiBerlaku != nil {
		existing.KlasifikasiBerlaku = req.KlasifikasiBerlaku
	}
	if req.Deskripsi != nil {
		existing.Deskripsi = req.Deskripsi
	}
	existing.RowVersion = req.RowVersion
	existing.UpdatedBy = callerID
	if len(req.DetailRows) > 0 {
		existing.DetailRows = nil
		for _, di := range req.DetailRows {
			mult := decimal.NewFromFloat(1.0)
			if di.Multiplier != nil {
				mult = *di.Multiplier
			}
			existing.DetailRows = append(existing.DetailRows, MappingDetailRow{
				ID:                uuid.New(),
				EventHeaderID:     existing.ID,
				Urutan:            di.Urutan,
				KodeAkunID:        di.KodeAkunID,
				DKIndicator:       di.DKIndicator,
				SumberAmount:      di.SumberAmount,
				KlasifikasiFilter: di.KlasifikasiFilter,
				Multiplier:        mult,
				Catatan:           di.Catatan,
				AktifFlag:         true,
			})
		}
	}
	tx, txErr := h.mapping.repo.BeginTx(c.Request.Context())
	if txErr != nil {
		response.Error(c, txErr)
		return
	}
	defer rollbackTx(tx) //nolint:errcheck
	if err := h.mapping.repo.UpdateDraft(c.Request.Context(), tx, existing); err != nil {
		if err.Error() == "jurnal.MappingRepo.UpdateDraft: row_version conflict or not found" {
			response.Error(c, domainerrors.ErrConflict())
			return
		}
		response.Error(c, err)
		return
	}
	if err := h.mapping.auditWriter.WithTx(tx).Write(c.Request.Context(),
		audit.EventFromContext(c.Request.Context(), audit.Event{
			Action:     "JURNAL_MAPPING.EDIT",
			EntityType: "mst.mapping_jurnal_header",
			EntityID:   existing.ID,
			After:      existing,
		})); err != nil {
		response.Error(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, existing)
}

// ─── Mapping Workflow endpoints ────────────────────────────────────────────────

// SubmitMappingHeader POST /jurnal/mapping-headers/:id/submit
func (h *Handler) SubmitMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingCreate) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, err := h.mapping.Submit(c.Request.Context(), id, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, WorkflowTransitionResponse{
		ID: result.ID, WorkflowStatus: result.WorkflowStatus, AktifFlag: result.AktifFlag,
	})
}

// ReviewMappingHeader POST /jurnal/mapping-headers/:id/review
func (h *Handler) ReviewMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingReview) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req WorkflowSigningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.mapping.Review(c.Request.Context(), id, req, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, WorkflowTransitionResponse{
		ID: result.ID, WorkflowStatus: result.WorkflowStatus, AktifFlag: result.AktifFlag,
	})
}

// ApproveMappingHeader POST /jurnal/mapping-headers/:id/approve
func (h *Handler) ApproveMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingApprove) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	claims := claimsFromCtx(c)
	var req WorkflowSigningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.mapping.Approve(c.Request.Context(), id, req, callerID, claims)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, WorkflowTransitionResponse{
		ID: result.ID, WorkflowStatus: result.WorkflowStatus, AktifFlag: result.AktifFlag,
	})
}

// ApproveMappingHeader2 POST /jurnal/mapping-headers/:id/approve-2
func (h *Handler) ApproveMappingHeader2(c *gin.Context) {
	if !requirePermission(c, PermMappingApprove2) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	claims := claimsFromCtx(c)
	var req WorkflowSigningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.mapping.Approve2(c.Request.Context(), id, req, callerID, claims)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, WorkflowTransitionResponse{
		ID: result.ID, WorkflowStatus: result.WorkflowStatus, AktifFlag: result.AktifFlag,
	})
}

// RejectMappingHeader POST /jurnal/mapping-headers/:id/reject
func (h *Handler) RejectMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingReview) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req WorkflowRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.mapping.Reject(c.Request.Context(), id, req, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, WorkflowTransitionResponse{
		ID: result.ID, WorkflowStatus: result.WorkflowStatus, AktifFlag: result.AktifFlag,
	})
}

// WithdrawMappingHeader POST /jurnal/mapping-headers/:id/withdraw
func (h *Handler) WithdrawMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingCreate) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	if err := h.mapping.Withdraw(c.Request.Context(), id, callerID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

// DeactivateMappingHeader POST /jurnal/mapping-headers/:id/deactivate
func (h *Handler) DeactivateMappingHeader(c *gin.Context) {
	if !requirePermission(c, PermMappingApprove) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, err := h.mapping.Deactivate(c.Request.Context(), id, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, WorkflowTransitionResponse{
		ID: result.ID, WorkflowStatus: result.WorkflowStatus, AktifFlag: result.AktifFlag,
	})
}

// ExportMappingHeaders GET /jurnal/mapping-headers/export
func (h *Handler) ExportMappingHeaders(c *gin.Context) {
	if !requirePermission(c, PermMappingExport) {
		return
	}
	// Stub: respond 202 with job ref (async export per UX rule §3).
	c.JSON(http.StatusAccepted, gin.H{
		"message": "Export async belum diimplementasikan dalam phase ini.",
	})
}

// ─── Resolver endpoint ─────────────────────────────────────────────────────────

// ResolveJurnal POST /jurnal/resolve
func (h *Handler) ResolveJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalRead) {
		return
	}
	var req ResolverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.resolver.Resolve(c.Request.Context(), req)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, result)
}

// ─── Manual Posting endpoints ──────────────────────────────────────────────────

// PostManualJurnal POST /jurnal/post
func (h *Handler) PostManualJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalPost) {
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req ManualPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.posting.CreateManualDraft(c.Request.Context(), req, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Created(c, result)
}

// SubmitManualJurnal POST /jurnal/:id/submit
func (h *Handler) SubmitManualJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalPost) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, err := h.posting.SubmitManual(c.Request.Context(), id, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, JurnalWorkflowTransitionResponse{
		ID: result.ID, NoJurnal: result.NoJurnal, StatusInternal: result.StatusInternal,
	})
}

// ApproveManualJurnal POST /jurnal/:id/approve
func (h *Handler) ApproveManualJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalApprove) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	// Get maker from existing header.
	existing, dbErr := h.posting.jurnalRepo.GetByID(c.Request.Context(), id)
	if dbErr != nil {
		response.Error(c, dbErr)
		return
	}
	if existing == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Jurnal header tidak ditemukan."))
		return
	}
	result, err := h.posting.ApproveManual(c.Request.Context(), id, callerID, existing.CreatedBy)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, JurnalWorkflowTransitionResponse{
		ID: result.ID, NoJurnal: result.NoJurnal, StatusInternal: result.StatusInternal,
	})
}

// RejectManualJurnal POST /jurnal/:id/reject
func (h *Handler) RejectManualJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalApprove) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req WorkflowRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	result, err := h.posting.RejectManual(c.Request.Context(), id, req.RejectReason, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, JurnalWorkflowTransitionResponse{
		ID: result.ID, NoJurnal: result.NoJurnal, StatusInternal: result.StatusInternal,
	})
}

// ─── Jurnal Read endpoints ─────────────────────────────────────────────────────

// ListJurnal GET /jurnal
func (h *Handler) ListJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalRead) {
		return
	}
	q, limit, err := parseListQuery(c, AllowedJurnalSortCols)
	if err != nil {
		response.Error(c, err)
		return
	}
	items, page, err := h.posting.jurnalRepo.ListSummary(c.Request.Context(), q, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, items, page.PaginationMeta(), toSortApplied(q), q.AppliedFilter())
}

// GetJurnal GET /jurnal/:id
func (h *Handler) GetJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalRead) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, err := h.posting.jurnalRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if result == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeJurnalHeaderNotFound, "Jurnal header tidak ditemukan."))
		return
	}
	response.OK(c, result)
}

// ExportJurnal GET /jurnal/:id/export
func (h *Handler) ExportJurnal(c *gin.Context) {
	if !requirePermission(c, PermJurnalExport) {
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Export async belum diimplementasikan."})
}

// ExportJurnalList GET /jurnal/export
func (h *Handler) ExportJurnalList(c *gin.Context) {
	if !requirePermission(c, PermJurnalExport) {
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Export async belum diimplementasikan."})
}

// ─── DLQ endpoints ────────────────────────────────────────────────────────────

// ListDLQ GET /jurnal/dlq
func (h *Handler) ListDLQ(c *gin.Context) {
	if !requirePermission(c, PermDLQRead) {
		return
	}
	q, limit, err := parseListQuery(c, AllowedDLQSortCols)
	if err != nil {
		response.Error(c, err)
		return
	}
	items, page, err := h.dlq.dlqRepo.ListSummary(c.Request.Context(), q, limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.List(c, items, page.PaginationMeta(), toSortApplied(q), q.AppliedFilter())
}

// GetDLQ GET /jurnal/dlq/:id
func (h *Handler) GetDLQ(c *gin.Context) {
	if !requirePermission(c, PermDLQRead) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	entry, err := h.dlq.dlqRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		response.Error(c, err)
		return
	}
	if entry == nil {
		response.Error(c, domainerrors.New(domainerrors.CodeJurnalDlqNotFound, "DLQ entry tidak ditemukan."))
		return
	}
	response.OK(c, entry)
}

// ReplayDLQ POST /jurnal/dlq/:id/replay
func (h *Handler) ReplayDLQ(c *gin.Context) {
	if !requirePermission(c, PermDLQReplay) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	result, err := h.dlq.Replay(c.Request.Context(), id, callerID)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.OK(c, result)
}

// DiscardDLQ POST /jurnal/dlq/:id/discard
func (h *Handler) DiscardDLQ(c *gin.Context) {
	if !requirePermission(c, PermDLQDiscard) {
		return
	}
	id, err := parsePathUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	callerID, err := callerUUID(c)
	if err != nil {
		response.Error(c, err)
		return
	}
	var req DLQDiscardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed, err.Error()))
		return
	}
	if err := h.dlq.Discard(c.Request.Context(), id, req, callerID); err != nil {
		response.Error(c, err)
		return
	}
	response.NoContent(c)
}

// ─── Private helpers ───────────────────────────────────────────────────────────

func toSortApplied(q listquery.Query) []response.SortApplied {
	out := make([]response.SortApplied, 0, len(q.Sort))
	for _, s := range q.Sort {
		out = append(out, response.SortApplied{Col: s.Col, Dir: s.Dir})
	}
	return out
}
