package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// RateLimitConfig mendefinisikan aturan rate limit untuk satu bucket.
type RateLimitConfig struct {
	// RequestsPerMinute adalah jumlah request yang diizinkan per menit per user.
	RequestsPerMinute int
	// KeyPrefix dipakai untuk namespace Redis key agar tidak collision antar endpoint.
	KeyPrefix string
}

// DefaultRateLimit adalah 100 req/min/user (api-conventions.md §Rate limiting).
var DefaultRateLimit = RateLimitConfig{RequestsPerMinute: 100, KeyPrefix: "rl:default"}

// SensitiveRateLimit adalah 10 req/min untuk endpoint sensitif (hard-close, param approve).
var SensitiveRateLimit = RateLimitConfig{RequestsPerMinute: 10, KeyPrefix: "rl:sensitive"}

// AuditRoleRateLimit adalah 500 req/min untuk ROLE-AUDIT (read-only, bypass normal limit).
var AuditRoleRateLimit = RateLimitConfig{RequestsPerMinute: 500, KeyPrefix: "rl:audit"}

// RateLimiter mengembalikan middleware rate limiter berbasis Redis token bucket (sliding window).
// Jika redis nil, rate limiting di-skip (untuk testing tanpa Redis).
func RateLimiter(rdb *redis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}

		// Key: {prefix}:{userId atau IP kalau belum auth}
		userID, _ := c.Get("userId")
		var bucketKey string
		if uid, ok := userID.(string); ok && uid != "" {
			bucketKey = fmt.Sprintf("%s:%s", cfg.KeyPrefix, uid)
		} else {
			bucketKey = fmt.Sprintf("%s:ip:%s", cfg.KeyPrefix, c.ClientIP())
		}

		allowed, remaining, resetAt, err := slidingWindowCheck(c.Request.Context(), rdb, bucketKey, cfg.RequestsPerMinute)
		if err != nil {
			// Redis error → fail open (jangan block user karena infrastruktur down).
			c.Next()
			return
		}

		// Set standard rate limit headers.
		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.RequestsPerMinute))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetAt, 10))

		if !allowed {
			retryAfter := int64(60) - (time.Now().Unix() - resetAt + 60)
			if retryAfter < 0 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.FormatInt(retryAfter, 10))
			traceID, _ := c.Get(response.TraceIDKey)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, map[string]any{
				"error": map[string]any{
					"code":    "RATE_LIMITED",
					"message": "Terlalu banyak request. Coba lagi dalam 60 detik.",
					"details": []any{},
					"traceId": traceID,
				},
			})
			return
		}

		c.Next()
	}
}

// RateLimiterForRole mengembalikan middleware yang memilih config berdasarkan role user.
// Cek dilakukan SETELAH auth middleware meng-set roles di context.
func RateLimiterForRole(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}

		cfg := DefaultRateLimit
		roles, _ := c.Get("roles")
		if roleList, ok := roles.([]string); ok {
			for _, r := range roleList {
				if r == "ROLE-AUDIT" {
					cfg = AuditRoleRateLimit
					break
				}
			}
		}

		RateLimiter(rdb, cfg)(c)
	}
}

// slidingWindowCheck mengimplementasikan sliding window rate limiting menggunakan Redis sorted set.
// Returns: (allowed, remaining, windowStartUnix, error).
func slidingWindowCheck(ctx context.Context, rdb *redis.Client, key string, limit int) (bool, int, int64, error) {
	now := time.Now()
	windowStart := now.Add(-time.Minute).UnixNano()
	nowNano := now.UnixNano()

	pipe := rdb.Pipeline()
	// Hapus entri di luar window.
	pipe.ZRemRangeByScore(ctx, key, "0", strconv.FormatInt(windowStart, 10))
	// Hitung entri dalam window.
	countCmd := pipe.ZCard(ctx, key)
	// Tambah entri saat ini.
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowNano), Member: nowNano})
	// Set expiry 70 detik (window 60 detik + buffer).
	pipe.Expire(ctx, key, 70*time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		return true, limit, now.Unix(), fmt.Errorf("rate limit redis: %w", err)
	}

	count := int(countCmd.Val())
	remaining := limit - count - 1 // -1 untuk request ini
	if remaining < 0 {
		remaining = 0
	}

	if count >= limit {
		// Rollback: hapus entry yang baru di-add karena di-reject.
		rdb.ZRem(ctx, key, nowNano)
		return false, 0, now.Unix(), nil
	}

	return true, remaining, now.Unix(), nil
}
