package audit

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"fmt"
	"time"
)

// VerifyResult adalah hasil verifikasi hash chain untuk satu entity.
type VerifyResult struct {
	EntityType  string
	EntityID    string
	TotalEvents int
	BrokenAt    *string // event_id dimana chain rusak, nil jika OK
	IsValid     bool
	Message     string
}

// RangeResult adalah hasil verifikasi hash chain untuk semua entity dalam range waktu.
type RangeResult struct {
	StartDate    time.Time
	EndDate      time.Time
	TotalEvents  int
	ValidEvents  int
	BrokenChains []VerifyResult
	IsValid      bool
}

// VerifyHashChain memverifikasi integritas hash chain audit log dalam range waktu.
// Dipakai oleh cmd/audit-verify.
//
// Algoritma:
//  1. Fetch semua events dalam range, diurutkan per entity_type, entity_id, timestamp ASC.
//  2. Untuk setiap entity, recompute hash chain dan bandingkan dengan stored.
//  3. Report entities yang chain-nya rusak.
func VerifyHashChain(ctx context.Context, db *sql.DB, start, end time.Time) (*RangeResult, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			id::text,
			entity_type,
			entity_id::text,
			previous_hash,
			current_hash,
			actor_user_id::text,
			action,
			timestamp,
			tenant_id
		FROM aud.audit_log
		WHERE timestamp >= $1 AND timestamp < $2
		  AND current_hash IS NOT NULL
		ORDER BY entity_type, entity_id, timestamp ASC
	`, start, end)
	if err != nil {
		return nil, fmt.Errorf("verify: query audit_log: %w", err)
	}
	defer rows.Close()

	type eventRow struct {
		EventID      string
		EntityType   string
		EntityID     string
		PreviousHash []byte
		CurrentHash  []byte
		ActorUserID  string
		Action       string
		Timestamp    time.Time
		TenantID     string
	}

	var events []eventRow
	for rows.Next() {
		var e eventRow
		if err := rows.Scan(
			&e.EventID, &e.EntityType, &e.EntityID,
			&e.PreviousHash, &e.CurrentHash,
			&e.ActorUserID, &e.Action, &e.Timestamp, &e.TenantID,
		); err != nil {
			return nil, fmt.Errorf("verify: scan row: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("verify: rows error: %w", err)
	}

	result := &RangeResult{
		StartDate:   start,
		EndDate:     end,
		TotalEvents: len(events),
		IsValid:     true,
	}

	// Group by entity_type + entity_id.
	type entityKey struct{ typ, id string }
	groups := make(map[entityKey][]eventRow)
	for i := range events {
		k := entityKey{events[i].EntityType, events[i].EntityID}
		groups[k] = append(groups[k], events[i])
	}

	for _, entityEvents := range groups {
		var prevHash []byte
		for i := range entityEvents {
			e := &entityEvents[i]
			canonicalJSON := buildCanonicalJSON(map[string]any{
				"event_id":    e.EventID,
				"event_time":  e.Timestamp.Format(time.RFC3339Nano),
				"actor":       e.ActorUserID,
				"action":      e.Action,
				"entity_type": e.EntityType,
				"entity_id":   e.EntityID,
				"tenant_id":   e.TenantID,
			})

			expectedHash := computeHash(prevHash, canonicalJSON)

			// Compare computed vs stored.
			if !hashesEqual(expectedHash, e.CurrentHash) {
				evID := e.EventID
				vr := VerifyResult{
					EntityType:  e.EntityType,
					EntityID:    e.EntityID,
					TotalEvents: len(entityEvents),
					BrokenAt:    &evID,
					IsValid:     false,
					Message: fmt.Sprintf("Hash chain rusak pada event %s. Expected: %x, Stored: %x",
						e.EventID, expectedHash, e.CurrentHash),
				}
				result.BrokenChains = append(result.BrokenChains, vr)
				result.IsValid = false
				break
			}

			prevHash = e.CurrentHash
			result.ValidEvents++
		}
	}

	return result, nil
}

// hashesEqual membandingkan dua byte slice menggunakan constant-time comparison
// untuk mencegah timing side-channel attack (security-baseline.md §"Hash chain").
func hashesEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}
