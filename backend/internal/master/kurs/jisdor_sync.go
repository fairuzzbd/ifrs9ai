package kurs

import (
	"net/http"

	"github.com/gin-gonic/gin"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/response"
	jisdor "blips-ifrs9.tugu-re.com/internal/integration/jisdor"
)

// JISDORSync handles POST /api/v1/master/kurs/jisdor-sync.
//
// Permission: kurs.jisdor_sync (enforced via middleware)
//
// In Phase 3, the JISDOR fetcher is a stub that returns an error.
// This endpoint logs the attempt and returns a 202 with a placeholder jobId.
// Phase 4 will replace the stub with the real HTTP fetcher.
func (h *Handler) JISDORSync(c *gin.Context) {
	var req JISDORSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, domainerrors.New(domainerrors.CodeValidationFailed,
			"Request body tidak valid: "+err.Error()))
		return
	}

	// Parse the date from the request.
	// The actual fetch is delegated to the JISDOR fetcher (stub in Phase 3).
	fetcher := jisdor.NewHTTPFetcher()
	_, fetchErr := fetcher.Fetch(c.Request.Context(), req.TanggalBerlaku)
	if fetchErr != nil {
		// Stub always returns not-implemented error in Phase 3.
		// Return 202 with explanation — the manual entry path is the fallback.
		c.JSON(http.StatusAccepted, gin.H{
			"data": JISDORSyncResponse{
				JobID:     "not-implemented",
				StatusURL: "",
				Message:   "JISDOR otomatis belum tersedia (Phase 4). " + fetchErr.Error() + " Gunakan input manual melalui POST /api/v1/master/kurs.",
			},
			"meta": gin.H{"traceId": c.GetString("traceId")},
		})
		return
	}

	// Future: when fetcher returns real data, enqueue Asynq job here.
	// For now the stub path is never reached (always returns error above).
	c.JSON(http.StatusAccepted, gin.H{
		"data": JISDORSyncResponse{
			JobID:     "not-implemented",
			StatusURL: "",
			Message:   "JISDOR sync job enqueued (placeholder Phase 4).",
		},
	})
}
