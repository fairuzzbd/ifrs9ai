package mappingjurnal

// p5m12_routes.go — P5-M12 route registration for 6-eyes workflow, bulk import, RPT-19/20/21.
//
// These routes are registered SEPARATELY from routes.go to keep base CRUD routes intact.
// Register after RegisterRoutes() in the API server setup.
//
// Route layout (relative to /api/v1):
//
//	POST   /master/mapping-jurnal/bulk-import                               → BulkImport
//	POST   /master/mapping-jurnal/:event_code/new-version                   → NewVersion
//	POST   /master/mapping-jurnal/:event_code/version/:version_id/submit    → Submit
//	POST   /master/mapping-jurnal/:event_code/version/:version_id/review    → Review
//	POST   /master/mapping-jurnal/:event_code/version/:version_id/approve   → Approve (4-eyes)
//	POST   /master/mapping-jurnal/:event_code/version/:version_id/approve-2 → Approve2 (6-eyes)
//	POST   /master/mapping-jurnal/:event_code/version/:version_id/reject    → Reject
//	GET    /reports/rpt-19-mapping-coverage                                 → GetCoverage
//	GET    /reports/rpt-19-mapping-coverage/export                          → ExportCoverage
//	GET    /reports/rpt-20-mapping-validation                               → GetValidation
//	GET    /reports/rpt-21-mapping-history                                  → GetHistory
//	GET    /reports/rpt-21-mapping-history/export                           → ExportHistory

import (
	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
)

// RegisterP5M12Routes registers P5-M12 routes on the given /api/v1 router group.
// Requires: P5M12Handler and report access requires mapping_jurnal.read permission.
func RegisterP5M12Routes(v1 *gin.RouterGroup, h *P5M12Handler) {
	mg := v1.Group("/master/mapping-jurnal")

	// Bulk import — registered before /:event_code to avoid Gin treating "bulk-import" as :event_code.
	mg.POST("/bulk-import", auth.RequirePermission("mapping_jurnal.create"), h.BulkImport)

	// P5-M12 version chain endpoints
	eventGroup := mg.Group("/:event_code")
	{
		eventGroup.POST("/new-version",
			auth.RequirePermission("mapping_jurnal.create"), h.NewVersion)

		versionGroup := eventGroup.Group("/version/:version_id")
		{
			versionGroup.POST("/submit",
				auth.RequirePermission("mapping_jurnal.submit"), h.Submit)
			versionGroup.POST("/review",
				auth.RequirePermission("mapping_jurnal.review"), h.Review)
			versionGroup.POST("/approve",
				auth.RequirePermission("mapping_jurnal.approve"), h.Approve)
			versionGroup.POST("/approve-2",
				auth.RequirePermission("mapping_jurnal.approve"), h.Approve2)
			versionGroup.POST("/reject",
				auth.RequirePermission("mapping_jurnal.reject"), h.Reject)
		}
	}

	// Reports
	rpt := v1.Group("/reports")
	{
		// RPT-19
		rpt.GET("/rpt-19-mapping-coverage",
			auth.RequirePermission("mapping_jurnal.read"), h.GetCoverage)
		rpt.GET("/rpt-19-mapping-coverage/export",
			auth.RequirePermission("mapping_jurnal.read"), h.ExportCoverage)

		// RPT-20
		rpt.GET("/rpt-20-mapping-validation",
			auth.RequirePermission("mapping_jurnal.read"), h.GetValidation)

		// RPT-21
		rpt.GET("/rpt-21-mapping-history",
			auth.RequirePermission("mapping_jurnal.read"), h.GetHistory)
		rpt.GET("/rpt-21-mapping-history/export",
			auth.RequirePermission("mapping_jurnal.read"), h.ExportHistory)
	}
}
