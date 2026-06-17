package closeflow

// middleware.go — PeriodeLockMiddleware: cross-cutting 423 enforcement.
//
// Per spec §4.2:
//   - Reads mst.periode_buku.status_periode FROM DB (SELECT FOR SHARE), NOT from JWT/session.
//   - SOFT_CLOSED/HARD_CLOSE_PENDING: blocks all mutations except those in the allowlist.
//   - CLOSED: blocks all mutations unconditionally.
//   - Allowlist comes from sys.config PERIODE_SOFT_CLOSED_MUTATION_ALLOWLIST (comma-separated).
//   - Allowlist is cached in memory with 5-minute TTL and refreshed at boot.
//   - Middleware is attached to ALL mutable routes (/api/v1/*) except the close-workflow
//     transitions themselves (those are registered outside the locked group).

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ─── Allowlist cache ──────────────────────────────────────────────────────────

// allowlistCache holds the parsed allowlist with a TTL.
type allowlistCache struct {
	mu          sync.RWMutex
	items       map[string]struct{}
	refreshedAt time.Time
	ttl         time.Duration
}

func newAllowlistCache(ttl time.Duration) *allowlistCache {
	return &allowlistCache{
		items: make(map[string]struct{}),
		ttl:   ttl,
	}
}

func (c *allowlistCache) load(raw string) {
	items := make(map[string]struct{})
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			items[s] = struct{}{}
		}
	}
	c.mu.Lock()
	c.items = items
	c.refreshedAt = time.Now()
	c.mu.Unlock()
}

func (c *allowlistCache) isAllowed(action string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.items[action]
	return ok
}

func (c *allowlistCache) isStale() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.refreshedAt) > c.ttl
}

// ─── PeriodeLockMiddleware ────────────────────────────────────────────────────

// PeriodeLockMiddleware enforces the mutation lock when a period is SOFT_CLOSED,
// HARD_CLOSE_PENDING, or CLOSED.
//
// Usage: mount on Gin router groups that carry a :periode_id route parameter.
// The middleware reads the period status from DB (SELECT FOR SHARE) on every request.
type PeriodeLockMiddleware struct {
	repo           *Repo
	allowlistCache *allowlistCache
	defaultCfg     Config
	mu             sync.Mutex // for refresh coordination
}

// NewPeriodeLockMiddleware creates a middleware instance and eagerly loads the allowlist
// from sys.config.
func NewPeriodeLockMiddleware(repo *Repo, cfg Config) *PeriodeLockMiddleware {
	m := &PeriodeLockMiddleware{
		repo:           repo,
		allowlistCache: newAllowlistCache(5 * time.Minute),
		defaultCfg:     cfg,
	}
	// Load initial allowlist from config.
	m.allowlistCache.load(cfg.SoftClosedMutationAllowlist)
	return m
}

// refreshAllowlistIfStale reloads the allowlist from sys.config if the cache has expired.
// Non-blocking: uses a background goroutine so request latency is not impacted.
func (m *PeriodeLockMiddleware) refreshAllowlistIfStale(ctx context.Context) {
	if !m.allowlistCache.isStale() {
		return
	}
	// Only one goroutine should refresh at a time.
	m.mu.Lock()
	if !m.allowlistCache.isStale() {
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	go func() {
		rawVal, err := m.repo.GetConfigValue(ctx, "PERIODE_SOFT_CLOSED_MUTATION_ALLOWLIST")
		if err != nil || rawVal == "" {
			// Fall back to compiled default.
			rawVal = m.defaultCfg.SoftClosedMutationAllowlist
		}
		m.allowlistCache.load(rawVal)
	}()
}

// Handler returns the Gin middleware function.
// Route groups that use this middleware MUST have a :periode_id parameter OR
// provide X-Periode-ID header.
func (m *PeriodeLockMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Refresh allowlist in background if stale.
		m.refreshAllowlistIfStale(c.Request.Context())

		// Only apply on mutating methods.
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		// Resolve periode ID from route param or header.
		periodeIDStr := c.Param("periode_id")
		if periodeIDStr == "" {
			periodeIDStr = c.GetHeader("X-Periode-ID")
		}
		if periodeIDStr == "" {
			// No periode context on this route — allow through (e.g. system routes).
			c.Next()
			return
		}

		periodeID, err := uuid.Parse(periodeIDStr)
		if err != nil {
			response.Error(c, fmt.Errorf("periode_id format tidak valid: %w", err))
			c.Abort()
			return
		}

		// Read current status from DB — never from cache/session (spec §4.2).
		periode, dbErr := m.repo.GetByID(c.Request.Context(), periodeID)
		if dbErr != nil {
			response.Error(c, dbErr)
			c.Abort()
			return
		}
		if periode == nil {
			// Periode not found — let downstream handler return 404.
			c.Next()
			return
		}

		// Store periode status in Gin context for downstream handlers.
		c.Set("periode_status", string(periode.StatusPeriode))
		c.Set("periode_kode", periode.PeriodeIDKode)

		switch periode.StatusPeriode {
		case PeriodeStatusClosed:
			// CLOSED blocks all mutations unconditionally.
			hcAt := ""
			if periode.TanggalHardClose != nil {
				hcAt = periode.TanggalHardClose.Format("2006-01-02")
			}
			response.Error(c, ErrPeriodeClosed(periode.PeriodeIDKode, hcAt))
			c.Abort()
			return

		case PeriodeStatusSoftClosed, PeriodeStatusHardClosePending:
			// SOFT_CLOSED / HARD_CLOSE_PENDING: block unless action is in allowlist.
			// The X-Close-Workflow-Action header signals allowlisted actions.
			action := c.GetHeader("X-Close-Workflow-Action")
			if action == "" {
				// Check the route's tagged action (set by route handler via context).
				if v, ok := c.Get("close_workflow_action"); ok {
					action, _ = v.(string) //nolint:errcheck
				}
			}

			if !m.allowlistCache.isAllowed(action) {
				response.Error(c, ErrPeriodeSoftClosed(periode.PeriodeIDKode))
				c.Abort()
				return
			}

			// Allowlisted action — proceed.
			c.Next()
			return

		default:
			// OPEN — allow all.
			c.Next()
		}
	}
}
