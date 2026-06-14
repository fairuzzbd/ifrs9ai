// Package helpers — Gin route registration for ECL helper endpoints.
//
// Called from main.go after creating the helpers.Handler:
//
//	svc := helpers.NewServices(db, auditWriter)
//	helpers.RegisterRoutes(router.Group("/api/v1/ecl"), helpers.NewHandler(svc))
//
// All routes require JWT auth middleware already applied on the parent group.
// No Idempotency-Key middleware on these routes (read-only per DEC-017).
package helpers

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts all 10 ECL helper paths on the given RouterGroup.
// The group is expected to already have JWT auth middleware.
//
//	GET  helpers/pd
//	POST helpers/pd/bulk
//	GET  helpers/lgd
//	POST helpers/lgd/bulk
//	GET  helpers/ead
//	POST helpers/ead/bulk
//	GET  helpers/ccf
//	GET  helpers/preview
//	GET  helpers/preview/export
//	POST helpers/bulk-lookup
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/helpers")

	// Story APP-C-PAR-001 — PD
	g.GET("/pd", h.GetPD)
	g.POST("/pd/bulk", h.BulkGetPD)

	// Story APP-C-PAR-002 — LGD
	g.GET("/lgd", h.GetLGD)
	g.POST("/lgd/bulk", h.BulkGetLGD)

	// Story APP-C-PAR-003 — EAD
	g.GET("/ead", h.GetEAD)
	g.POST("/ead/bulk", h.BulkGetEAD)

	// Story APP-C-PAR-004 — CCF
	g.GET("/ccf", h.GetCCF)

	// Story APP-C-PAR-005 — Preview (ecl_helpers.preview permission)
	g.GET("/preview", h.GetPreview)
	g.GET("/preview/export", h.ExportPreview)

	// Story APP-C-PAR-006 — Combined bulk (ecl_helpers.read permission)
	g.POST("/bulk-lookup", h.BulkLookup)
}
