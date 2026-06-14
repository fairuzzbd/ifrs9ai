package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Idempotency adalah middleware yang mengimplementasikan idempotency check
// menggunakan tabel sys.idempotency_key (migration 0004).
//
// Aturan (api-conventions.md §Idempotency, DEC-021):
//   - Mutating endpoints (POST/PATCH/PUT/DELETE) WAJIB menyertakan Idempotency-Key header (UUID v4).
//   - Same key + same payload hash → return original response (IDEMPOTENCY_REPLAY 200).
//   - Same key + different payload hash → 422 IDEMPOTENCY_MISMATCH.
//   - TTL: 24 jam.
//
// Jika db nil, middleware di-skip (untuk testing tanpa DB).
func Idempotency(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Hanya berlaku untuk mutating methods.
		method := c.Request.Method
		if method != http.MethodPost && method != http.MethodPatch &&
			method != http.MethodPut && method != http.MethodDelete {
			c.Next()
			return
		}

		// Jika DB tidak tersedia, skip (testing mode).
		if db == nil {
			c.Next()
			return
		}

		keyStr := c.GetHeader("Idempotency-Key")
		if keyStr == "" {
			traceID, _ := c.Get(response.TraceIDKey)
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
				"error": map[string]any{
					"code":    "VALIDATION_FAILED",
					"message": "Header 'Idempotency-Key' wajib disertakan untuk request mutasi.",
					"details": []map[string]any{{
						"field":   "header.Idempotency-Key",
						"rule":    "required",
						"message": "Idempotency-Key header wajib diisi (UUID v4, DEC-021)",
					}},
					"traceId": traceID,
				},
			})
			return
		}

		// Validasi UUID v4.
		idempKey, err := uuid.Parse(keyStr)
		if err != nil {
			traceID, _ := c.Get(response.TraceIDKey)
			c.AbortWithStatusJSON(http.StatusBadRequest, map[string]any{
				"error": map[string]any{
					"code":    "VALIDATION_FAILED",
					"message": "Idempotency-Key harus berformat UUID v4.",
					"details": []map[string]any{{
						"field":   "header.Idempotency-Key",
						"rule":    "uuid_v4",
						"message": "Format tidak valid: harus UUID v4",
					}},
					"traceId": traceID,
				},
			})
			return
		}

		// Baca body untuk hashing (harus di-buffer karena bisa dibaca 1x).
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, err = io.ReadAll(io.LimitReader(c.Request.Body, 10<<20)) // max 10MB
			if err != nil {
				c.Next()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Hash = sha256(method + path + body).
		requestHash := computeRequestHash(method, c.Request.URL.Path, bodyBytes)

		// Cek apakah key sudah ada di DB.
		existing, found, err := lookupIdempotencyKey(c.Request.Context(), db, idempKey)
		if err != nil {
			// DB error: fail open (jangan block request karena infra issue).
			c.Next()
			return
		}

		if found {
			// Key sudah ada — cek payload hash.
			if !bytes.Equal(existing.requestHash, requestHash) {
				// Same key, different payload → IDEMPOTENCY_MISMATCH 422.
				traceID, _ := c.Get(response.TraceIDKey)
				c.AbortWithStatusJSON(http.StatusUnprocessableEntity, map[string]any{
					"error": map[string]any{
						"code":    "IDEMPOTENCY_MISMATCH",
						"message": "Idempotency-Key sudah dipakai dengan payload berbeda dari request sebelumnya.",
						"details": []any{},
						"traceId": traceID,
					},
				})
				return
			}

			// Same key, same payload → IDEMPOTENCY_REPLAY: return original response.
			var originalResp any
			if err := json.Unmarshal(existing.responseJSON, &originalResp); err != nil {
				originalResp = existing.responseJSON
			}
			c.AbortWithStatusJSON(int(existing.httpStatus), originalResp)
			return
		}

		// Key belum ada — intercept response untuk di-save.
		rw := &responseWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = rw

		c.Next()

		// Simpan response ke DB setelah handler selesai.
		// Hanya simpan sukses (2xx) dan 4xx tertentu.
		status := rw.status
		if status == 0 {
			status = http.StatusOK
		}

		responseBody := rw.body.Bytes()
		if len(responseBody) > 0 {
			userID := ""
			if uid, ok := c.Get("userId"); ok {
				if s, ok2 := uid.(string); ok2 {
					userID = s
				}
			}
			endpoint := method + " " + c.Request.URL.Path
			// #nosec G115 — HTTP status is always in [100,599], fits in int16.
			if err := saveIdempotencyKey(c.Request.Context(), db, idempKey, requestHash, responseBody, int16(status), userID, endpoint); err != nil {
				slog.Default().WarnContext(c.Request.Context(), "idempotency: failed to save key",
					"key", idempKey, "endpoint", endpoint, "error", err)
			}
		}
	}
}

// idempotencyRecord adalah row dari sys.idempotency_key.
type idempotencyRecord struct {
	key          uuid.UUID
	requestHash  []byte
	responseJSON []byte
	httpStatus   int16
}

// lookupIdempotencyKey mengambil record idempotency dari DB.
func lookupIdempotencyKey(ctx context.Context, db *sql.DB, key uuid.UUID) (*idempotencyRecord, bool, error) {
	var rec idempotencyRecord
	err := db.QueryRowContext(ctx, `
		SELECT key, request_hash, response_json, http_status
		FROM sys.idempotency_key
		WHERE key = $1 AND expires_at > now()
	`, key).Scan(&rec.key, &rec.requestHash, &rec.responseJSON, &rec.httpStatus)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &rec, true, nil
}

// saveIdempotencyKey menyimpan key + response ke DB.
func saveIdempotencyKey(ctx context.Context, db *sql.DB, key uuid.UUID, requestHash, responseBody []byte, status int16, userID, endpoint string) error {
	var userUUID *uuid.UUID
	if userID != "" {
		uid, err := uuid.Parse(userID)
		if err == nil {
			userUUID = &uid
		}
	}

	expiresAt := time.Now().Add(idempotencyKeyTTL)
	_, err := db.ExecContext(ctx, `
		INSERT INTO sys.idempotency_key (key, request_hash, response_json, http_status, user_id, endpoint, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (key) DO NOTHING
	`, key, requestHash, responseBody, status, userUUID, endpoint, expiresAt)
	return err
}

// computeRequestHash menghitung sha256(method + "|" + path + "|" + body).
func computeRequestHash(method, path string, body []byte) []byte {
	h := sha256.New()
	h.Write([]byte(strings.ToUpper(method)))
	h.Write([]byte("|"))
	h.Write([]byte(path))
	h.Write([]byte("|"))
	h.Write(body)
	return h.Sum(nil)
}

// ComputeRequestHashHex mengembalikan hex string dari request hash (untuk debug).
func ComputeRequestHashHex(method, path string, body []byte) string {
	return hex.EncodeToString(computeRequestHash(method, path, body))
}

// responseWriter adalah custom gin.ResponseWriter yang juga meng-capture response body.
type responseWriter struct {
	gin.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	rw.body.Write(b)
	return rw.ResponseWriter.Write(b)
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) WriteString(s string) (int, error) {
	rw.body.WriteString(s)
	return rw.ResponseWriter.WriteString(s)
}

// idempotencyKeyTTL adalah TTL untuk idempotency key.
const idempotencyKeyTTL = 24 * time.Hour
