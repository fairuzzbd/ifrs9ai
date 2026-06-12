package core

import "github.com/gin-gonic/gin"

// routes.go — route registration for ECL core endpoints.
//
// Called from main.go:
//
//	core.RegisterRoutes(router.Group("/api/v1"), coreHandler)
//
// The parent group must already have JWT auth middleware applied.
// Permission checks are inside each handler function.
//
// OpenAPI spec: api/openapi/app-c-ecl-core.yaml

// RegisterRoutes mounts all ECL core HTTP endpoints under the given RouterGroup.
// The group is expected to already have JWT auth middleware.
//
//	POST   /ecl/compute
//	POST   /ecl/compute/bulk
//	GET    /ecl/results/:calcRunId
//	GET    /ecl/results/:calcRunId/instrumen/:instrumenId
//	GET    /ecl/results/:calcRunId/portofolio/:portofolioId/summary
//	GET    /ecl/results/:calcRunId/roll-forward
//	POST   /ecl/recompute/ad-hoc
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	ecl := rg.Group("/ecl")

	// Compute endpoints.
	ecl.POST("/compute", h.ComputeSingle)
	ecl.POST("/compute/bulk", h.ComputeBulk)

	// Result read endpoints.
	ecl.GET("/results/:calcRunId", h.ListResults)
	ecl.GET("/results/:calcRunId/instrumen/:instrumenId", h.GetSingleResult)
	ecl.GET("/results/:calcRunId/portofolio/:portofolioId/summary", h.GetPortfolioSummary)
	ecl.GET("/results/:calcRunId/roll-forward", h.GetRollForward)

	// Ad-hoc recompute.
	ecl.POST("/recompute/ad-hoc", h.RecomputeAdHoc)
}
