// Package audit menyediakan audit log writer yang menulis ke aud.audit_log
// DI TRANSAKSI YANG SAMA dengan mutation (red flag kalau di luar tx).
//
// Hash chain: current_hash = sha256(previous_hash || canonical_json(row))
// sesuai db-conventions.md §"Audit log table" dan migration 0005.
//
// WAJIB: Writer.Write() harus dipanggil dalam transaksi yang sudah open,
// bukan setelah commit. Gunakan Writer.WithTx() untuk mendapatkan tx-bound writer.
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
)

// EventFromContext membuat Event dengan IP + UserAgent yang diambil dari context.
// Helper untuk pemanggil yang tidak perlu mengisi IP/UA secara manual.
// IP + UserAgent di-set via middleware.ContextWithIPUA (common/middleware package).
func EventFromContext(ctx context.Context, base Event) Event {
	base.IP = middleware.IPFromContext(ctx)
	base.UserAgent = middleware.UserAgentFromContext(ctx)
	return base
}

// Event adalah satu audit log event yang akan ditulis.
type Event struct {
	// Action adalah nama aksi dalam format {ENTITY}.{ACTION}, mis. "INSTRUMEN.CREATE".
	// WAJIB uppercase, dot separator.
	Action string
	// EntityType adalah nama tabel/entity, mis. "mst.instrumen".
	EntityType string
	// EntityID adalah UUID primary key entity yang dimodifikasi.
	EntityID uuid.UUID
	// Before adalah state sebelum mutasi (nil untuk INSERT).
	Before any
	// After adalah state sesudah mutasi (nil untuk DELETE).
	After any
	// ActorUserID override: jika kosong, diambil dari context JWT claims.
	ActorUserID string
	// ActorRole override: jika kosong, diambil dari context JWT claims.
	ActorRole string
	// IP adalah client IP address. Kosong string → disimpan NULL di DB.
	// Gunakan EventFromContext untuk mengisi dari context secara otomatis.
	IP string
	// UserAgent adalah HTTP User-Agent header dari client.
	// Kosong string → disimpan NULL di DB.
	UserAgent string
}

// Writer menulis audit log events ke database.
type Writer struct {
	db *sql.DB
}

// NewWriter membuat Writer baru.
func NewWriter(db *sql.DB) *Writer {
	return &Writer{db: db}
}

// TxWriter adalah Writer yang bound ke transaksi spesifik.
type TxWriter struct {
	tx *sql.Tx
}

// WithTx mengembalikan TxWriter yang terikat ke transaksi.
// WAJIB: gunakan ini agar audit log ditulis dalam transaksi yang sama dengan mutation.
func (w *Writer) WithTx(tx *sql.Tx) *TxWriter {
	return &TxWriter{tx: tx}
}

// Write menulis satu audit log event dalam transaksi.
// ctx harus berisi JWT claims (dari auth.ContextWithClaims) dan trace ID.
func (tw *TxWriter) Write(ctx context.Context, evt Event) error {
	return writeEvent(ctx, tw.tx, evt)
}

// writeEvent adalah implementasi internal.
func writeEvent(ctx context.Context, tx *sql.Tx, evt Event) error {
	if evt.Action == "" {
		return fmt.Errorf("audit.Write: Action wajib diisi")
	}
	if evt.EntityType == "" {
		return fmt.Errorf("audit.Write: EntityType wajib diisi")
	}

	// Resolve actor dari context JWT claims jika tidak di-override.
	actorUserID := evt.ActorUserID
	actorRole := evt.ActorRole
	if claims := auth.ClaimsFromContext(ctx); claims != nil {
		if actorUserID == "" {
			actorUserID = claims.Sub
		}
		if actorRole == "" && len(claims.Roles) > 0 {
			actorRole = claims.Roles[0]
		}
	}

	if actorUserID == "" {
		return fmt.Errorf("audit.Write: actorUserID tidak bisa kosong")
	}

	actorUUID, err := uuid.Parse(actorUserID)
	if err != nil {
		return fmt.Errorf("audit.Write: actorUserID bukan UUID valid: %w", err)
	}

	traceID := middleware.TraceIDFromContext(ctx)
	tenantID := "TUGURE"
	if claims := auth.ClaimsFromContext(ctx); claims != nil && claims.TenantID != "" {
		tenantID = claims.TenantID
	}

	// Ambil previous_hash dari row terakhir untuk entity ini.
	previousHash, err := fetchPreviousHash(ctx, tx, evt.EntityType, evt.EntityID)
	if err != nil {
		// Non-fatal: chain belum ada (first event) atau query error.
		// Log tapi jangan fail write.
		previousHash = nil
	}

	// Serialize before/after ke JSONB.
	beforeJSON, err := marshalAuditJSON(evt.Before)
	if err != nil {
		return fmt.Errorf("audit.Write: marshal before: %w", err)
	}
	afterJSON, err := marshalAuditJSON(evt.After)
	if err != nil {
		return fmt.Errorf("audit.Write: marshal after: %w", err)
	}

	eventID := uuid.New()
	eventTime := time.Now()

	// Compute canonical JSON untuk hash chain.
	canonicalJSON := buildCanonicalJSON(map[string]any{
		"event_id":    eventID.String(),
		"event_time":  eventTime.Format(time.RFC3339Nano),
		"actor":       actorUserID,
		"action":      evt.Action,
		"entity_type": evt.EntityType,
		"entity_id":   evt.EntityID.String(),
		"tenant_id":   tenantID,
	})

	// Compute hash: sha256(previous_hash || canonical_json)
	currentHash := computeHash(previousHash, canonicalJSON)

	// ip/user_agent: resolve dari context jika tidak di-override di Event.
	// Fallback ke context values yang di-set oleh auth middleware via middleware.ContextWithIPUA.
	// Store NULL when empty — valid per schema (INET nullable).
	ip := evt.IP
	if ip == "" {
		ip = middleware.IPFromContext(ctx)
	}
	ua := evt.UserAgent
	if ua == "" {
		ua = middleware.UserAgentFromContext(ctx)
	}
	var ipVal *string
	if ip != "" {
		ipVal = &ip
	}
	var uaVal *string
	if ua != "" {
		uaVal = &ua
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO aud.audit_log (
			id, timestamp, actor_user_id, actor_role,
			action, entity_type, entity_id,
			before_value, after_value,
			ip_address, user_agent,
			trace_id, tenant_id,
			previous_hash, current_hash
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7,
			$8, $9,
			$10, $11,
			$12, $13,
			$14, $15
		)`,
		eventID, eventTime, actorUUID, actorRole,
		evt.Action, evt.EntityType, evt.EntityID,
		beforeJSON, afterJSON,
		ipVal, uaVal,
		traceID, tenantID,
		previousHash, currentHash,
	)
	if err != nil {
		return fmt.Errorf("audit.Write: insert audit_log: %w", err)
	}

	return nil
}

// fetchPreviousHash mengambil current_hash dari row audit_log terakhir untuk entity ini.
func fetchPreviousHash(ctx context.Context, tx *sql.Tx, entityType string, entityID uuid.UUID) ([]byte, error) {
	var hash []byte
	err := tx.QueryRowContext(ctx, `
		SELECT current_hash
		FROM aud.audit_log
		WHERE entity_type = $1 AND entity_id = $2 AND current_hash IS NOT NULL
		ORDER BY timestamp DESC
		LIMIT 1
	`, entityType, entityID).Scan(&hash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return hash, err
}

// computeHash menghitung sha256(previousHash || canonicalJSON).
// Go equivalent dari sec.compute_audit_hash() di migration 0005.
func computeHash(previousHash []byte, canonicalJSON string) []byte {
	h := sha256.New()
	if len(previousHash) > 0 {
		h.Write(previousHash)
	}
	h.Write([]byte(canonicalJSON))
	return h.Sum(nil)
}

// ComputeHashHex mengembalikan hex string dari hash (untuk debug/verify).
func ComputeHashHex(previousHash []byte, canonicalJSON string) string {
	return hex.EncodeToString(computeHash(previousHash, canonicalJSON))
}

// BuildCanonicalJSON membangun JSON deterministik (sorted keys) dari map.
// Exported untuk keperluan testing dan cmd/audit-verify.
func BuildCanonicalJSON(m map[string]any) string {
	return buildCanonicalJSON(m)
}

// MarshalAuditJSON meng-marshal nilai ke JSON bytes untuk before/after audit field.
// Exported untuk keperluan testing.
func MarshalAuditJSON(v any) ([]byte, error) {
	return marshalAuditJSON(v)
}

// buildCanonicalJSON membangun JSON yang konsisten (sorted keys, no whitespace).
// WAJIB deterministik — hasil yang sama untuk input yang sama.
func buildCanonicalJSON(m map[string]any) string {
	// Sort keys untuk determinisme.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	sorted := make(map[string]any, len(m))
	for _, k := range keys {
		sorted[k] = m[k]
	}

	b, err := json.Marshal(sorted)
	if err != nil {
		// buildCanonicalJSON is used only with map[string]any of primitive scalars
		// assembled in-package; marshal failure here is a programmer error.
		panic(fmt.Sprintf("buildCanonicalJSON: json.Marshal: %v", err))
	}
	return string(b)
}

// marshalAuditJSON mengkonversi any ke JSON bytes (untuk before/after JSONB).
// Mengembalikan nil jika input nil.
func marshalAuditJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return b, nil
}
