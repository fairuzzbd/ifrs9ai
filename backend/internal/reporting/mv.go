package reporting

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// MVRepo handles sys.mv_refresh_log persistence and PG advisory lock.
type MVRepo struct {
	primary *sql.DB
	replica *sql.DB
}

// NewMVRepo creates a MVRepo.
func NewMVRepo(primary, replica *sql.DB) *MVRepo {
	return &MVRepo{primary: primary, replica: replica}
}

// IsRefreshRunning checks sys.mv_refresh_log for a RUNNING row for mvName.
// Used as guard before attempting advisory lock.
func (r *MVRepo) IsRefreshRunning(ctx context.Context, mvName, tenantID string) (bool, *MVRefreshLog, error) {
	row := r.primary.QueryRowContext(ctx, `
		SELECT id, mv_name, triggered_by, trigger_actor, status, started_at, tenant_id
		FROM sys.mv_refresh_log
		WHERE mv_name = $1 AND tenant_id = $2 AND status = 'RUNNING'
		  AND deleted_at IS NULL
		ORDER BY started_at DESC
		LIMIT 1
	`, mvName, tenantID)

	var log MVRefreshLog
	var triggerActor sql.NullString
	err := row.Scan(
		&log.ID, &log.MVName, &log.TriggeredBy, &triggerActor,
		&log.Status, &log.StartedAt, &log.TenantID,
	)
	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("mv.IsRefreshRunning: %w", err)
	}
	if triggerActor.Valid {
		uid, _ := uuid.Parse(triggerActor.String)
		log.TriggerActor = &uid
	}
	return true, &log, nil
}

// InsertRefreshLog inserts a new RUNNING row into sys.mv_refresh_log.
func (r *MVRepo) InsertRefreshLog(ctx context.Context, tx *sql.Tx, log *MVRefreshLog) error {
	var triggerActorVal *string
	if log.TriggerActor != nil {
		s := log.TriggerActor.String()
		triggerActorVal = &s
	}
	actorID := log.ID // created_by reuse
	_, err := tx.ExecContext(ctx, `
		INSERT INTO sys.mv_refresh_log
		    (id, mv_name, triggered_by, trigger_actor, status, started_at,
		     created_at, created_by, updated_at, updated_by, row_version, tenant_id)
		VALUES ($1,$2,$3,$4,'RUNNING',$5,now(),$6,now(),$6,1,$7)
	`, log.ID, log.MVName, string(log.TriggeredBy), triggerActorVal, log.StartedAt,
		actorID, log.TenantID)
	if err != nil {
		return fmt.Errorf("mv.InsertRefreshLog: %w", err)
	}
	return nil
}

// UpdateRefreshLog updates status + completed_at + row_count + error_detail.
func (r *MVRepo) UpdateRefreshLog(ctx context.Context, logID uuid.UUID, status string, rowCount *int64, errDetail *string, tenantID string) error {
	_, err := r.primary.ExecContext(ctx, `
		UPDATE sys.mv_refresh_log
		SET status = $2, completed_at = now(), row_count = $3, error_detail = $4,
		    updated_at = now(), row_version = row_version + 1
		WHERE id = $1 AND tenant_id = $5
	`, logID, status, rowCount, errDetail, tenantID)
	if err != nil {
		return fmt.Errorf("mv.UpdateRefreshLog: %w", err)
	}
	return nil
}

// RefreshConcurrent executes REFRESH MATERIALIZED VIEW CONCURRENTLY for mvName.
// Uses PG pg_try_advisory_lock (non-blocking). If lock not acquired → MV_REFRESH_LOCKED.
// Returns rowCount after refresh.
func RefreshConcurrent(ctx context.Context, db *sql.DB, mvName string) (int64, error) {
	// Validate mvName is in AllMVNames (safety: prevent SQL injection via MV name).
	if !isValidMVName(mvName) {
		return 0, fmt.Errorf("RefreshConcurrent: unknown mv_name %q", mvName)
	}

	// Advisory lock: non-blocking.
	var lockAcquired bool
	err := db.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock(hashtext($1))`, mvName,
	).Scan(&lockAcquired)
	if err != nil {
		return 0, fmt.Errorf("RefreshConcurrent: advisory lock check: %w", err)
	}
	if !lockAcquired {
		return 0, domainerrors.New(domainerrors.CodeMVRefreshLocked,
			fmt.Sprintf("Refresh %s sedang berjalan. Coba lagi setelah selesai.", mvName))
	}
	defer func() {
		// Release advisory lock (best-effort).
		_, _ = db.ExecContext(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, mvName)
	}()

	// REFRESH MATERIALIZED VIEW CONCURRENTLY requires unique index per MV.
	// #nosec G202 — mvName validated against AllMVNames allowlist above.
	_, err = db.ExecContext(ctx, fmt.Sprintf("REFRESH MATERIALIZED VIEW CONCURRENTLY %s", mvName))
	if err != nil {
		return 0, fmt.Errorf("RefreshConcurrent: refresh %s: %w", mvName, err)
	}

	// Count rows in refreshed MV.
	// #nosec G202 — mvName validated above.
	var rowCount int64
	err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", mvName)).Scan(&rowCount)
	if err != nil {
		// Non-fatal: count failure doesn't invalidate the refresh.
		rowCount = -1
	}

	return rowCount, nil
}

// ListMVStatus returns MVStatusItem for all 8 MVs.
// Reads from sys.mv_refresh_log (latest per MV) via primary (authoritative).
func ListMVStatus(ctx context.Context, db *sql.DB, tenantID string) ([]MVStatusItem, error) {
	rows, err := db.QueryContext(ctx, `
		WITH latest AS (
		    SELECT DISTINCT ON (mv_name)
		           mv_name, status, completed_at, row_count, error_detail, triggered_by
		    FROM sys.mv_refresh_log
		    WHERE tenant_id = $1 AND deleted_at IS NULL
		    ORDER BY mv_name, started_at DESC
		)
		SELECT l.mv_name,
		       COALESCE(l.status, 'IDLE') AS status,
		       l.completed_at,
		       l.row_count,
		       l.error_detail,
		       l.triggered_by
		FROM unnest($2::TEXT[]) AS m(mv_name)
		LEFT JOIN latest l ON l.mv_name = m.mv_name
		ORDER BY m.mv_name
	`, tenantID, AllMVNames)
	if err != nil {
		return nil, fmt.Errorf("ListMVStatus: %w", err)
	}
	defer rows.Close()

	var items []MVStatusItem
	for rows.Next() {
		var item MVStatusItem
		var status, triggeredBy sql.NullString
		var lastRefreshAt sql.NullTime
		var rowCount sql.NullInt64
		var lastError sql.NullString
		if err := rows.Scan(&item.MVName, &status, &lastRefreshAt, &rowCount, &lastError, &triggeredBy); err != nil {
			return nil, fmt.Errorf("ListMVStatus: scan: %w", err)
		}
		if lastRefreshAt.Valid {
			t := lastRefreshAt.Time
			item.LastRefreshAt = &t
		}
		if rowCount.Valid {
			rc := rowCount.Int64
			item.RowCount = &rc
		}
		if lastError.Valid {
			item.LastError = &lastError.String
		}
		if status.Valid {
			item.Status = MVRefreshStatus(status.String)
		} else {
			item.Status = MVStatusIdle
		}
		if triggeredBy.Valid {
			tb := TriggeredBy(triggeredBy.String)
			item.TriggeredBy = &tb
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// isValidMVName checks mvName is in AllMVNames allowlist.
func isValidMVName(mvName string) bool {
	for _, n := range AllMVNames {
		if n == mvName {
			return true
		}
	}
	return false
}

// Now returns current time (mockable in tests).
var Now = func() time.Time { return time.Now() }
