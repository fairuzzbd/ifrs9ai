// Package pdpefindo — route registration for mst.pd_pefindo endpoints.
//
// REUSE PATTERN: same convention as internal/master/matauang.
package pdpefindo

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterRoutes registers all pd_pefindo HTTP routes under the given /api/v1 router group.
//
// Route layout:
//
//	GET    /master/pd-pefindo                               → List              (ecl_parameter.read)
//	POST   /master/pd-pefindo                               → Create            (ecl_parameter.submit)
//	GET    /master/pd-pefindo/export                        → Export CSV        (ecl_parameter.read)
//	POST   /master/pd-pefindo/upload-xlsx                   → UploadXLSX        (ecl_parameter.submit)
//	GET    /master/pd-pefindo/upload-jobs/:jobId            → GetUploadJobStatus(ecl_parameter.read)
//	GET    /master/pd-pefindo/:id                           → GetByID           (ecl_parameter.read)
//	PATCH  /master/pd-pefindo/:id                           → Update            (ecl_parameter.submit)
//	DELETE /master/pd-pefindo/:id                           → Delete            (ecl_parameter.submit)
//	GET    /master/pd-pefindo/:id/history                   → History           (ecl_parameter.read)
//	GET    /master/pd-pefindo/:id/workflow                  → WorkflowStatus    (ecl_parameter.read)
//	POST   /master/pd-pefindo/:id/submit                    → Submit            (ecl_parameter.submit)
//	POST   /master/pd-pefindo/:id/review                    → Review            (ecl_parameter.review)
//	POST   /master/pd-pefindo/:id/approve                   → Approve           (ecl_parameter.approve)
//	POST   /master/pd-pefindo/:id/approve2                  → Approve2          (ecl_parameter.approve)
//	POST   /master/pd-pefindo/:id/reject                    → Reject            (ecl_parameter.reject)
//
// IMPORTANT: static sub-paths (/export, /upload-xlsx, /upload-jobs/:jobId) MUST be
// registered BEFORE the /:id wildcard to avoid Gin routing ambiguity.
func RegisterRoutes(v1 *gin.RouterGroup, h *Handler) {
	mg := v1.Group("/master/pd-pefindo")

	// ── Collection endpoints ───────────────────────────────────────────────────
	mg.GET("", auth.RequirePermission("ecl_parameter.read"), h.List)
	mg.POST("", auth.RequirePermission("ecl_parameter.submit"), h.Create)

	// Static sub-paths — registered BEFORE /:id
	mg.GET("/export", auth.RequirePermission("ecl_parameter.read"), h.Export)
	mg.POST("/upload-xlsx", auth.RequirePermission("ecl_parameter.submit"), h.UploadXLSX)
	mg.GET("/upload-jobs/:jobId", auth.RequirePermission("ecl_parameter.read"), h.GetUploadJobStatus)

	// ── Single-record endpoints ────────────────────────────────────────────────
	mg.GET("/:id", auth.RequirePermission("ecl_parameter.read"), h.GetByID)
	mg.PATCH("/:id", auth.RequirePermission("ecl_parameter.submit"), h.Update)
	mg.DELETE("/:id", auth.RequirePermission("ecl_parameter.submit"), h.Delete)

	// ── Sub-resources ──────────────────────────────────────────────────────────
	mg.GET("/:id/history", auth.RequirePermission("ecl_parameter.read"), h.History)
	mg.GET("/:id/workflow", auth.RequirePermission("ecl_parameter.read"), h.WorkflowStatus)

	// Workflow mutation endpoints.
	mg.POST("/:id/submit", auth.RequirePermission("ecl_parameter.submit"), h.Submit)
	mg.POST("/:id/review", auth.RequirePermission("ecl_parameter.review"), h.Review)
	mg.POST("/:id/approve", auth.RequirePermission("ecl_parameter.approve"), h.Approve)
	mg.POST("/:id/approve2", auth.RequirePermission("ecl_parameter.approve"), h.Approve2)
	mg.POST("/:id/reject", auth.RequirePermission("ecl_parameter.reject"), h.Reject)
}
