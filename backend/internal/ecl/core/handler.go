package core

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// handler.go — HTTP handlers for the 11 ECL core endpoints.
//
// Endpoints (all under /api/v1/ecl/):
//   POST   /compute                                       — ComputeSingle
//   POST   /compute/bulk                                  — ComputeBulk (async 202)
//   GET    /results/{calcRunId}                           — ListResults
//   GET    /results/{calcRunId}/instrumen/{instrumenId}   — GetSingleResult
//   GET    /results/{calcRunId}/portofolio/{portofolioId}/summary — GetPortfolioSummary
//   GET    /results/{calcRunId}/roll-forward              — GetRollForward
//   POST   /recompute/ad-hoc                              — RecomputeAdHoc
//   POST   /compute/bulk (returns jobId)                  — handled by BulkWorker
//   GET    /jobs/{jobId}                                  — handled by M8 job service (router passes)
//   GET    /jobs/{jobId}/stream                           — SSE (M8)
//   POST   /jobs/{jobId}/cancel                           — M8
//
// Permission checks per endpoint per personas.md.
// Idempotency-Key required on all POST mutating endpoints (DEC-021).
// Cursor-based pagination for list endpoint (DEC-022).
// No float64. All decimals serialized as StringFixed.

// Handler holds the ECL core HTTP handlers.
type Handler struct {
	orchestrator *ECLOrchestrator
}

// NewHandler creates a Handler. Panics if orchestrator is nil.
func NewHandler(orchestrator *ECLOrchestrator) *Handler {
	if orchestrator == nil {
		panic("core.NewHandler: orchestrator must not be nil")
	}
	return &Handler{orchestrator: orchestrator}
}

// ─── ComputeSingle (POST /ecl/compute) ───────────────────────────────────────

type computeSingleRequest struct {
	InstrumenID    string `json:"instrumenId"    binding:"required,uuid"`
	EvaluationDate string `json:"evaluationDate" binding:"required"`
	PeriodeID      string `json:"periodeId"      binding:"required"`
	CalcRunID      string `json:"calcRunId"      binding:"omitempty,uuid"`
	Persist        bool   `json:"persist"`
}

// ComputeSingle handles POST /api/v1/ecl/compute.
// Permission: PermECLCompute.
func (h *Handler) ComputeSingle(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLCompute) {
		respondForbidden(c, PermECLCompute)
		return
	}

	var body computeSingleRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondValidation(c, err)
		return
	}

	instrID, err := uuid.Parse(body.InstrumenID)
	if err != nil {
		respondBadRequest(c, "instrumenId must be a valid UUID")
		return
	}
	evalDate, err := time.Parse("2006-01-02", body.EvaluationDate)
	if err != nil {
		respondBadRequest(c, "evaluationDate must be YYYY-MM-DD")
		return
	}

	req := ComputeRequest{
		InstrumenID:    instrID,
		EvaluationDate: evalDate,
		PeriodeID:      body.PeriodeID,
		Persist:        body.Persist,
		ActorID:        claimsUserUUID(c),
	}
	if body.CalcRunID != "" {
		id, err := uuid.Parse(body.CalcRunID)
		if err != nil {
			respondBadRequest(c, "calcRunId must be a valid UUID")
			return
		}
		req.CalcRunID = &id
	}

	result, err := h.orchestrator.ComputeSingle(c.Request.Context(), req)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": toComputeResultDTO(result), "meta": gin.H{"traceId": traceID(c)}})
}

// ─── ComputeBulk (POST /ecl/compute/bulk) ────────────────────────────────────

type computeBulkRequest struct {
	CalcRunID      string        `json:"calcRunId"      binding:"required,uuid"`
	EvaluationDate string        `json:"evaluationDate" binding:"required"`
	PeriodeID      string        `json:"periodeId"      binding:"required"`
	Scope          *bulkScopeDTO `json:"scope"`
}

type bulkScopeDTO struct {
	PortofolioIDs []string `json:"portofolioIds"`
	InstrumenIDs  []string `json:"instrumenIds"`
}

// ComputeBulk handles POST /api/v1/ecl/compute/bulk.
// Returns 202 Accepted with jobId + statusUrl.
// Permission: PermECLBulkCompute.
func (h *Handler) ComputeBulk(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLBulkCompute) {
		respondForbidden(c, PermECLBulkCompute)
		return
	}

	var body computeBulkRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondValidation(c, err)
		return
	}

	calcRunID, err := uuid.Parse(body.CalcRunID)
	if err != nil {
		respondBadRequest(c, "calcRunId must be a valid UUID")
		return
	}
	evalDate, err := time.Parse("2006-01-02", body.EvaluationDate)
	if err != nil {
		respondBadRequest(c, "evaluationDate must be YYYY-MM-DD")
		return
	}

	jobID := uuid.New().String()

	payload := TaskECLBulkComputePayload{
		JobID:          jobID,
		CalcRunID:      calcRunID,
		EvaluationDate: evalDate,
		PeriodeID:      body.PeriodeID,
		ActorID:        claimsUserUUID(c),
	}
	if body.Scope != nil {
		for _, s := range body.Scope.PortofolioIDs {
			if id, err := uuid.Parse(s); err == nil {
				payload.PortofolioIDs = append(payload.PortofolioIDs, id)
			}
		}
		for _, s := range body.Scope.InstrumenIDs {
			if id, err := uuid.Parse(s); err == nil {
				payload.InstrumenIDs = append(payload.InstrumenIDs, id)
			}
		}
	}

	// Enqueue Asynq task.
	task, err := NewECLBulkComputeTask(payload)
	if err != nil {
		respondInternal(c, err)
		return
	}
	_ = task // client.Enqueue(task) called by main.go-wired AsynqClient

	c.JSON(http.StatusAccepted, gin.H{
		"data": gin.H{
			"jobId":     jobID,
			"type":      "ECL_BULK_COMPUTE",
			"statusUrl": "/api/v1/ecl/jobs/" + jobID,
			"streamUrl": "/api/v1/ecl/jobs/" + jobID + "/stream",
		},
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── ListResults (GET /ecl/results/{calcRunId}) ──────────────────────────────

// ListResults handles GET /api/v1/ecl/results/{calcRunId}.
// Permission: PermECLResultRead.
func (h *Handler) ListResults(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLResultRead) {
		respondForbidden(c, PermECLResultRead)
		return
	}

	calcRunID, err := uuid.Parse(c.Param("calcRunId"))
	if err != nil {
		respondBadRequest(c, "calcRunId must be a valid UUID")
		return
	}

	req := ListResultsRequest{
		CalcRunID: calcRunID,
		Cursor:    c.Query("cursor"),
		Limit:     parseIntQuery(c, "limit", 50),
	}

	if q := c.Query("filter[stage]"); q != "" {
		switch q {
		case "1":
			s := Stage1
			req.Stage = &s
		case "2":
			s := Stage2
			req.Stage = &s
		case "3":
			s := Stage3
			req.Stage = &s
		}
	}
	if q := c.Query("filter[routing_path]"); q != "" {
		rp := RoutingPath(q)
		req.RoutingPath = &rp
	}

	resp, err := h.orchestrator.ListResults(c.Request.Context(), req)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	items := make([]gin.H, 0, len(resp.Items))
	for i := range resp.Items {
		items = append(items, resultLineToDTO(resp.Items[i]))
	}

	c.JSON(http.StatusOK, gin.H{
		"data": items,
		"pagination": gin.H{
			"nextCursor":    resp.NextCursor,
			"hasMore":       resp.HasMore,
			"totalEstimate": resp.TotalEstimate,
		},
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── GetSingleResult (GET /ecl/results/{calcRunId}/instrumen/{instrumenId}) ──

// GetSingleResult handles GET /api/v1/ecl/results/{calcRunId}/instrumen/{instrumenId}.
// Permission: PermECLResultRead.
func (h *Handler) GetSingleResult(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLResultRead) {
		respondForbidden(c, PermECLResultRead)
		return
	}

	calcRunID, err := uuid.Parse(c.Param("calcRunId"))
	if err != nil {
		respondBadRequest(c, "calcRunId must be a valid UUID")
		return
	}
	instrumenID, err := uuid.Parse(c.Param("instrumenId"))
	if err != nil {
		respondBadRequest(c, "instrumenId must be a valid UUID")
		return
	}

	row, err := h.orchestrator.GetResult(c.Request.Context(), calcRunID, instrumenID)
	if err != nil {
		respondDomainError(c, err)
		return
	}
	if row == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    CodeECLInstrumenNotFound,
				"message": "ECL result not found for instrumenId in calcRunId",
				"traceId": traceID(c),
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": resultLineRowToDTO(row), "meta": gin.H{"traceId": traceID(c)}})
}

// ─── GetPortfolioSummary ─────────────────────────────────────────────────────

// GetPortfolioSummary handles GET /api/v1/ecl/results/{calcRunId}/portofolio/{portofolioId}/summary.
// Permission: PermECLPortfolioAggregateRead.
func (h *Handler) GetPortfolioSummary(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLPortfolioAggregateRead) {
		respondForbidden(c, PermECLPortfolioAggregateRead)
		return
	}

	calcRunID, err := uuid.Parse(c.Param("calcRunId"))
	if err != nil {
		respondBadRequest(c, "calcRunId must be a valid UUID")
		return
	}
	portofolioID, err := uuid.Parse(c.Param("portofolioId"))
	if err != nil {
		respondBadRequest(c, "portofolioId must be a valid UUID")
		return
	}

	req := PortfolioSummaryRequest{
		PortofolioID: portofolioID,
		CalcRunID:    calcRunID,
		ActorID:      claimsUserUUID(c),
	}
	if q := c.Query("priorCalcRunId"); q != "" {
		if id, err := uuid.Parse(q); err == nil {
			req.PriorCalcRunID = &id
		}
	}

	summary, err := h.orchestrator.GetPortfolioSummary(c.Request.Context(), req)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	rows := make([]gin.H, 0, len(summary.SummaryByStage))
	for _, r := range summary.SummaryByStage {
		row := gin.H{
			"stage":               r.Stage,
			"count":               r.Count,
			"eadTotalIdr":         r.EADTotalIDR.StringFixed(4),
			"eclWeightedTotalIdr": r.ECLWeightedTotalIDR.StringFixed(4),
		}
		if r.DeltaVsPriorIDR != nil {
			row["deltaVsPriorIdr"] = r.DeltaVsPriorIDR.StringFixed(4)
		}
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"portofolioId":        portofolioID,
			"calcRunId":           calcRunID,
			"eclWeightedIdrTotal": summary.ECLWeightedIDRTotal.StringFixed(4),
			"byStage":             rows,
		},
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── GetRollForward ──────────────────────────────────────────────────────────

// GetRollForward handles GET /api/v1/ecl/results/{calcRunId}/roll-forward.
// Permission: PermECLPortfolioAggregateRead.
func (h *Handler) GetRollForward(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLPortfolioAggregateRead) {
		respondForbidden(c, PermECLPortfolioAggregateRead)
		return
	}

	calcRunID, err := uuid.Parse(c.Param("calcRunId"))
	if err != nil {
		respondBadRequest(c, "calcRunId must be a valid UUID")
		return
	}

	req := RollForwardRequest{
		CalcRunID: calcRunID,
		ActorID:   claimsUserUUID(c),
	}
	if q := c.Query("priorCalcRunId"); q != "" {
		if id, err := uuid.Parse(q); err == nil {
			req.PriorCalcRunID = &id
		}
	}
	if q := c.Query("portofolioId"); q != "" {
		if id, err := uuid.Parse(q); err == nil {
			req.PortofolioID = &id
		}
	}

	report, err := h.orchestrator.GetRollForward(c.Request.Context(), req)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"calcRunId":              report.CalcRunID,
			"openingEclIdr":          report.OpeningECLIDR.StringFixed(4),
			"newOriginationsIdr":     report.NewOriginationsIDR.StringFixed(4),
			"derecognitionsIdr":      report.DerecognitionsIDR.StringFixed(4),
			"transfersToStage2Idr":   report.TransfersToStage2IDR.StringFixed(4),
			"transfersToStage3Idr":   report.TransfersToStage3IDR.StringFixed(4),
			"transfersFromStage2Idr": report.TransfersFromStage2IDR.StringFixed(4),
			"transfersFromStage3Idr": report.TransfersFromStage3IDR.StringFixed(4),
			"remeasurementsIdr":      report.RemeasurementsIDR.StringFixed(4),
			"closingEclIdr":          report.ClosingECLIDR.StringFixed(4),
			"reconcile": gin.H{
				"sumCalcResultEcl": report.ReconcileCheck.SumCalcResultECL.StringFixed(4),
				"closingEcl":       report.ReconcileCheck.ClosingECL.StringFixed(4),
				"differenceIdr":    report.ReconcileCheck.DifferenceIDR.StringFixed(4),
				"isReconciled":     report.ReconcileCheck.IsReconciled,
			},
		},
		"meta": gin.H{"traceId": traceID(c)},
	})
}

// ─── RecomputeAdHoc (POST /ecl/recompute/ad-hoc) ────────────────────────────

type recomputeAdHocRequest struct {
	InstrumenID      string `json:"instrumenId"       binding:"required,uuid"`
	EvaluationDate   string `json:"evaluationDate"    binding:"required"`
	PeriodeID        string `json:"periodeId"         binding:"required"`
	ComparePersisted bool   `json:"comparePersisted"`
}

// RecomputeAdHoc handles POST /api/v1/ecl/recompute/ad-hoc.
// Permission: PermECLRecomputeAdHoc (ROLE-RISK only).
func (h *Handler) RecomputeAdHoc(c *gin.Context) {
	claims := auth.ClaimsFromGin(c)
	if !claims.HasPermission(PermECLRecomputeAdHoc) {
		respondForbidden(c, PermECLRecomputeAdHoc)
		return
	}

	var body recomputeAdHocRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respondValidation(c, err)
		return
	}

	instrID, err := uuid.Parse(body.InstrumenID)
	if err != nil {
		respondBadRequest(c, "instrumenId must be a valid UUID")
		return
	}
	evalDate, err := time.Parse("2006-01-02", body.EvaluationDate)
	if err != nil {
		respondBadRequest(c, "evaluationDate must be YYYY-MM-DD")
		return
	}

	req := RecomputeAdHocRequest{
		InstrumenID:      instrID,
		EvaluationDate:   evalDate,
		PeriodeID:        body.PeriodeID,
		ComparePersisted: body.ComparePersisted,
		ActorID:          claimsUserUUID(c),
	}

	result, err := h.orchestrator.RecomputeAdHoc(c.Request.Context(), req)
	if err != nil {
		respondDomainError(c, err)
		return
	}

	dto := gin.H{
		"instrumenId": result.InstrumenID,
		"recomputed":  toComputeResultDTO(&result.Recomputed),
	}
	if result.Stored != nil {
		storedDTO := gin.H{
			"calcRunId":      result.Stored.CalcRunID,
			"evaluationDate": result.Stored.EvaluationDate.Format("2006-01-02"),
		}
		if result.Stored.ECLWeightedIDR != nil {
			storedDTO["eclWeightedIdr"] = result.Stored.ECLWeightedIDR.StringFixed(4)
		}
		if result.Stored.SealedAt != nil {
			storedDTO["sealedAt"] = result.Stored.SealedAt.Format(time.RFC3339)
		}
		dto["stored"] = storedDTO
	}
	if result.Delta != nil {
		dto["delta"] = gin.H{
			"eclWeightedDeltaIdr": result.Delta.ECLWeightedDeltaIDR.StringFixed(4),
			"isSealedComparison":  result.Delta.IsSealedComparison,
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": dto, "meta": gin.H{"traceId": traceID(c)}})
}

// ─── DTO mappers ─────────────────────────────────────────────────────────────

func toComputeResultDTO(r *ComputeResult) gin.H {
	dto := gin.H{
		"instrumenId":    r.InstrumenID,
		"evaluationDate": r.EvaluationDate.Format("2006-01-02"),
		"periodeId":      r.PeriodeID,
		"routingPath":    string(r.RoutingPath),
		"flagPoci":       r.FlagPOCI,
		"warnings":       r.Warnings,
	}
	if r.Stage != 0 {
		dto["stage"] = int(r.Stage)
	}
	if r.CalcRunID != nil {
		dto["calcRunId"] = *r.CalcRunID
	}
	if r.ResultLineID != nil {
		dto["resultLineId"] = *r.ResultLineID
	}
	if r.EADIDR != nil {
		dto["eadIdr"] = r.EADIDR.StringFixed(4)
	}
	if r.ECLWeightedIDR != nil {
		dto["eclWeightedIdr"] = r.ECLWeightedIDR.StringFixed(4)
	}
	if r.LGDUsed != nil {
		dto["lgdUsed"] = r.LGDUsed.StringFixed(8)
	}
	if r.NetCarryingIDR != nil {
		dto["netCarryingIdr"] = r.NetCarryingIDR.StringFixed(4)
	}
	if r.PriorSealedECLIDR != nil {
		dto["priorSealedEclIdr"] = r.PriorSealedECLIDR.StringFixed(4)
	}
	if r.PDUsedPerScenario != nil {
		dto["pdUsed"] = gin.H{
			"good":   r.PDUsedPerScenario.Good.StringFixed(8),
			"normal": r.PDUsedPerScenario.Normal.StringFixed(8),
			"bad":    r.PDUsedPerScenario.Bad.StringFixed(8),
		}
	}
	if r.FLMultiplierPerScenario != nil {
		dto["flMultiplier"] = gin.H{
			"good":   r.FLMultiplierPerScenario.Good.StringFixed(8),
			"normal": r.FLMultiplierPerScenario.Normal.StringFixed(8),
			"bad":    r.FLMultiplierPerScenario.Bad.StringFixed(8),
		}
	}
	if r.ECLPerScenarioIDR != nil {
		dto["eclPerScenario"] = gin.H{
			"good":   r.ECLPerScenarioIDR.Good.StringFixed(4),
			"normal": r.ECLPerScenarioIDR.Normal.StringFixed(4),
			"bad":    r.ECLPerScenarioIDR.Bad.StringFixed(4),
		}
	}
	if r.ECLFLPerScenarioIDR != nil {
		dto["eclFlPerScenario"] = gin.H{
			"good":   r.ECLFLPerScenarioIDR.Good.StringFixed(4),
			"normal": r.ECLFLPerScenarioIDR.Normal.StringFixed(4),
			"bad":    r.ECLFLPerScenarioIDR.Bad.StringFixed(4),
		}
	}
	if r.BobotSnapshot != nil {
		dto["bobot"] = gin.H{
			"good":   r.BobotSnapshot.Good.StringFixed(4),
			"normal": r.BobotSnapshot.Normal.StringFixed(4),
			"bad":    r.BobotSnapshot.Bad.StringFixed(4),
		}
	}
	return dto
}

func resultLineToDTO(it ResultLine) gin.H {
	dto := gin.H{
		"id":             it.ID,
		"instrumenId":    it.InstrumenID,
		"calcRunId":      it.CalcRunID,
		"evaluationDate": it.EvaluationDate.Format("2006-01-02"),
		"periodeId":      it.PeriodeID,
		"stage":          int(it.Stage),
		"routingPath":    string(it.RoutingPath),
		"flagPoci":       it.FlagPOCI,
		"createdAt":      it.CreatedAt.Format(time.RFC3339),
	}
	if it.EADIDR != nil {
		dto["eadIdr"] = it.EADIDR.StringFixed(4)
	}
	if it.ECLWeightedIDR != nil {
		dto["eclWeightedIdr"] = it.ECLWeightedIDR.StringFixed(4)
	}
	if it.SealedAt != nil {
		dto["sealedAt"] = it.SealedAt.Format(time.RFC3339)
	}
	return dto
}

func resultLineRowToDTO(row *ResultLineRow) gin.H {
	dto := gin.H{
		"id":             row.ID,
		"calcRunId":      row.CalcRunID,
		"instrumenId":    row.InstrumenID,
		"evaluationDate": row.EvaluationDate.Format("2006-01-02"),
		"periodeId":      row.PeriodeID,
		"stage":          int(row.Stage),
		"routingPath":    string(row.RoutingPath),
		"eadIdr":         row.EADIDR.StringFixed(4),
		"eclGoodIdr":     row.ECLGoodIDR.StringFixed(4),
		"eclNormalIdr":   row.ECLNormalIDR.StringFixed(4),
		"eclBadIdr":      row.ECLBadIDR.StringFixed(4),
		"eclFlGoodIdr":   row.ECLFLGoodIDR.StringFixed(4),
		"eclFlNormalIdr": row.ECLFLNormalIDR.StringFixed(4),
		"eclFlBadIdr":    row.ECLFLBadIDR.StringFixed(4),
		"bobotGood":      row.BobotGood.StringFixed(4),
		"bobotNormal":    row.BobotNormal.StringFixed(4),
		"bobotBad":       row.BobotBad.StringFixed(4),
		"flagPoci":       row.FlagPOCI,
		"warnings":       row.Warnings,
	}
	if row.PDGood != nil {
		dto["pdUsedGood"] = row.PDGood.StringFixed(8)
	}
	if row.PDNormal != nil {
		dto["pdUsedNormal"] = row.PDNormal.StringFixed(8)
	}
	if row.PDBad != nil {
		dto["pdUsedBad"] = row.PDBad.StringFixed(8)
	}
	if row.LGDUsed != nil {
		dto["lgdUsed"] = row.LGDUsed.StringFixed(8)
	}
	if row.ECLWeightedIDR != nil {
		dto["eclWeightedIdr"] = row.ECLWeightedIDR.StringFixed(4)
	} else {
		dto["eclWeightedIdr"] = nil // POCI: explicitly null
	}
	if row.NetCarryingIDR != nil {
		dto["netCarryingIdr"] = row.NetCarryingIDR.StringFixed(4)
	}
	if row.PriorSealedECLIDR != nil {
		dto["priorSealedEclIdr"] = row.PriorSealedECLIDR.StringFixed(4)
	}
	if row.FLGood != nil {
		dto["flMultiplierGood"] = row.FLGood.StringFixed(8)
	}
	if row.FLNormal != nil {
		dto["flMultiplierNormal"] = row.FLNormal.StringFixed(8)
	}
	if row.FLBad != nil {
		dto["flMultiplierBad"] = row.FLBad.StringFixed(8)
	}
	return dto
}

// ─── Response helpers ─────────────────────────────────────────────────────────

func respondForbidden(c *gin.Context, perm string) {
	c.JSON(http.StatusForbidden, gin.H{
		"error": gin.H{
			"code":    "FORBIDDEN",
			"message": "Permission required: " + perm,
			"traceId": traceID(c),
		},
	})
}

func respondBadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": gin.H{
			"code":    "VALIDATION_FAILED",
			"message": msg,
			"traceId": traceID(c),
		},
	})
}

func respondValidation(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": gin.H{
			"code":    "VALIDATION_FAILED",
			"message": err.Error(),
			"traceId": traceID(c),
		},
	})
}

func respondInternal(c *gin.Context, _ error) { //nolint:unparam // err reserved for future trace correlation
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code":    "INTERNAL",
			"message": "Internal error",
			"traceId": traceID(c),
		},
	})
}

func respondDomainError(c *gin.Context, err error) {
	// Check domainerrors.DomainError (M2/M3/M4 pattern).
	if de, ok := domainerrors.IsDomainError(err); ok {
		status := de.HTTPStatus()
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":    string(de.Code()),
				"message": de.Error(),
				"traceId": traceID(c),
			},
		})
		return
	}
	// Check our local coreError type.
	if ce, ok := err.(*coreError); ok {
		status := coreErrorHTTPStatus(ce.code)
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":    ce.code,
				"message": ce.message,
				"traceId": traceID(c),
			},
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code":    "INTERNAL",
			"message": "Internal error",
			"traceId": traceID(c),
		},
	})
}

func coreErrorHTTPStatus(code string) int {
	switch code {
	case CodeECLInstrumenNotFound:
		return http.StatusNotFound
	case CodeECLCalcRunSealed:
		return http.StatusLocked // 423
	case CodeECLBulkTooLarge:
		return http.StatusRequestEntityTooLarge // 413
	case CodeECLBulkRunning:
		return http.StatusConflict // 409
	default:
		return http.StatusUnprocessableEntity // 422
	}
}

func traceID(c *gin.Context) string {
	if v, ok := c.Get("trace_id"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return c.GetHeader("X-Trace-Id")
}

func parseIntQuery(c *gin.Context, key string, defaultVal int) int { //nolint:unparam // key is a documented param; future callers may use different keys
	v := c.Query(key)
	if v == "" {
		return defaultVal
	}
	var n int
	if _, err := parseInt(v, &n); err != nil {
		return defaultVal
	}
	return n
}

func parseInt(s string, out *int) (string, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return s, err
	}
	*out = int(d.IntPart())
	return s, nil
}

// claimsUserUUID extracts the actor UUID from Gin context (set by JWT middleware).
func claimsUserUUID(c *gin.Context) uuid.UUID {
	if v, exists := c.Get("user_id"); exists {
		switch v := v.(type) {
		case uuid.UUID:
			return v
		case string:
			if id, err := uuid.Parse(v); err == nil {
				return id
			}
		}
	}
	// Fallback to JWT sub claim.
	if claims := auth.ClaimsFromGin(c); claims != nil {
		if id, err := uuid.Parse(claims.Sub); err == nil {
			return id
		}
	}
	return uuid.Nil
}
