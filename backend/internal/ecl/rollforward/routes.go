// Package rollforward — route registration for Roll-Forward CKPN endpoints (P4-M11).
package rollforward

import (
	"database/sql"

	"github.com/gin-gonic/gin"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// RegisterRoutes registers all Roll-Forward CKPN endpoints under the provided router group (/api/v1).
//
// Route layout (api/openapi/app-c-roll-forward.yaml):
//
//	POST   /ecl/roll-forward/compute                       computeRollForward        (M11-001)
//	GET    /ecl/roll-forward                               getRollForward            (M11-004)
//	GET    /ecl/roll-forward/:id/export                    exportDisclosure          (M11-005)
//	GET    /ecl/roll-forward/portfolios/:pid               getPortfolioRollForward   (M11-004)
//	GET    /ecl/roll-forward/portfolios/:pid/instruments   listPortfolioInstruments  (M11-004)
//	GET    /ecl/dashboard/ckpn-trend                       getCKPNTrend              (M11-006)
//
// Auth: all routes require JWT via auth.Middleware (DEC-025).
// Idempotency-Key: POST /compute requires header (DEC-021).
//
// Gin routing note: "/ecl/roll-forward/compute" is a static segment registered via POST,
// while "/ecl/roll-forward/:id/export" uses ":id". Gin resolves static before dynamic,
// so POST /compute is unambiguous from GET /:id/export.
// "/ecl/roll-forward/portfolios/:pid" is also static prefix — no ":id" ambiguity.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, v *auth.Verifier, db *sql.DB) {
	authed := rg.Group("", auth.Middleware(v))
	idmp := middleware.Idempotency(db)

	rfGroup := authed.Group("/ecl/roll-forward")
	{
		// Compute roll-forward (POST, idempotency required).
		rfGroup.POST("/compute", idmp, h.ComputeRollForward)

		// Read roll-forward (GET with query params).
		rfGroup.GET("", h.GetRollForward)

		// Export disclosure XLSX.
		rfGroup.GET("/:id/export", h.ExportDisclosure)

		// Per-portfolio breakdown.
		portfolioGroup := rfGroup.Group("/portfolios")
		{
			portfolioGroup.GET("/:pid", h.GetPortfolioRollForward)
			portfolioGroup.GET("/:pid/instruments", h.ListPortfolioInstruments)
		}
	}

	// Trend dashboard — separate prefix per OpenAPI spec.
	dashGroup := authed.Group("/ecl/dashboard")
	{
		dashGroup.GET("/ckpn-trend", h.GetCKPNTrend)
	}
}
