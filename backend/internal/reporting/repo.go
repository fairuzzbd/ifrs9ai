package reporting

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository handles all database operations for the reporting package.
type Repository struct {
	primary *sql.DB
	replica *sql.DB
}

// NewRepository creates a Repository.
func NewRepository(primary, replica *sql.DB) *Repository {
	return &Repository{primary: primary, replica: replica}
}

// BeginTx begins a transaction on the primary DB.
func (r *Repository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.primary.BeginTx(ctx, nil)
}

// InsertExportLog inserts a new sys.export_log row in the given tx.
func (r *Repository) InsertExportLog(ctx context.Context, tx *sql.Tx, row ExportLogRow) error {
	db := dbOrTx{tx: tx}
	_, err := db.ExecContext(ctx, `
		INSERT INTO sys.export_log
		    (id, report_slug, format, status, requested_by, requested_at,
		     created_at, created_by, updated_at, updated_by, row_version, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6,now(),$5,now(),$5,1,$7)
	`, row.ID, row.ReportSlug, string(row.Format), string(row.Status),
		row.RequestedBy, row.RequestedAt, row.TenantID)
	if err != nil {
		return fmt.Errorf("repo.InsertExportLog: %w", err)
	}
	return nil
}

// UpdateExportLogCompleted updates sys.export_log after export completes.
func (r *Repository) UpdateExportLogCompleted(ctx context.Context, id uuid.UUID, rowCount int64, minioPath, sha256Hash, signedURL string, expiresAt time.Time, tenantID string) error {
	_, err := r.primary.ExecContext(ctx, `
		UPDATE sys.export_log
		SET status='COMPLETED', row_count=$2, file_minio_path=$3,
		    sha256_hash=$4, signed_url=$5, expires_at=$6, completed_at=now(),
		    updated_at=now(), row_version=row_version+1
		WHERE id=$1 AND tenant_id=$7
	`, id, rowCount, minioPath, sha256Hash, signedURL, expiresAt, tenantID)
	if err != nil {
		return fmt.Errorf("repo.UpdateExportLogCompleted: %w", err)
	}
	return nil
}

// UpdateExportLogFailed updates sys.export_log when export fails.
func (r *Repository) UpdateExportLogFailed(ctx context.Context, id uuid.UUID, errDetail, tenantID string) error {
	_, err := r.primary.ExecContext(ctx, `
		UPDATE sys.export_log
		SET status='FAILED', error_detail=$2, updated_at=now(), row_version=row_version+1
		WHERE id=$1 AND tenant_id=$3
	`, id, errDetail, tenantID)
	if err != nil {
		return fmt.Errorf("repo.UpdateExportLogFailed: %w", err)
	}
	return nil
}

// UpdateExportLogDownloaded sets downloaded_at on sys.export_log.
func (r *Repository) UpdateExportLogDownloaded(ctx context.Context, tx *sql.Tx, id uuid.UUID, tenantID string) error {
	db := dbOrTx{tx: tx}
	_, err := db.ExecContext(ctx, `
		UPDATE sys.export_log
		SET downloaded_at=now(), updated_at=now(), row_version=row_version+1
		WHERE id=$1 AND tenant_id=$2
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("repo.UpdateExportLogDownloaded: %w", err)
	}
	return nil
}

// GetExportLog fetches one sys.export_log row by ID.
func (r *Repository) GetExportLog(ctx context.Context, id uuid.UUID, tenantID string) (*ExportLogRow, error) {
	row := r.primary.QueryRowContext(ctx, `
		SELECT id, report_slug, format, status, row_count, file_minio_path,
		       sha256_hash, signed_url, requested_by, requested_at, completed_at,
		       expires_at, downloaded_at, job_id, tenant_id
		FROM sys.export_log
		WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL
	`, id, tenantID)
	return scanExportLogRow(row)
}

// ListExportLogs returns a cursor-paged list of sys.export_log rows.
func (r *Repository) ListExportLogs(ctx context.Context, cursor string, limit int, tenantID string) ([]ExportLogItem, *string, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	fetch := limit + 1

	var rows *sql.Rows
	var err error
	if cursor == "" {
		rows, err = r.primary.QueryContext(ctx, `
			SELECT id, report_slug, format, status, row_count, sha256_hash,
			       file_minio_path, expires_at, requested_by, requested_at, completed_at, downloaded_at
			FROM sys.export_log
			WHERE tenant_id=$1 AND deleted_at IS NULL
			ORDER BY requested_at DESC
			LIMIT $2
		`, tenantID, fetch)
	} else {
		cursorTime, parseErr := time.Parse(time.RFC3339Nano, cursor)
		if parseErr != nil {
			cursorTime = time.Now()
		}
		rows, err = r.primary.QueryContext(ctx, `
			SELECT id, report_slug, format, status, row_count, sha256_hash,
			       file_minio_path, expires_at, requested_by, requested_at, completed_at, downloaded_at
			FROM sys.export_log
			WHERE tenant_id=$1 AND deleted_at IS NULL AND requested_at < $2
			ORDER BY requested_at DESC
			LIMIT $3
		`, tenantID, cursorTime, fetch)
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("repo.ListExportLogs: %w", err)
	}
	defer rows.Close()

	var items []ExportLogItem
	for rows.Next() {
		var item ExportLogItem
		var rowCount sql.NullInt64
		var sha256Hash, minioPath sql.NullString
		var expiresAt, completedAt, downloadedAt sql.NullTime
		if err := rows.Scan(&item.ID, &item.ReportSlug, &item.Format, &item.Status,
			&rowCount, &sha256Hash, &minioPath, &expiresAt,
			&item.RequestedBy, &item.RequestedAt, &completedAt, &downloadedAt); err != nil {
			return nil, nil, false, fmt.Errorf("repo.ListExportLogs scan: %w", err)
		}
		if rowCount.Valid {
			rc := rowCount.Int64
			item.RowCount = &rc
		}
		if sha256Hash.Valid {
			item.FileSHA256 = &sha256Hash.String
		}
		if minioPath.Valid {
			item.MinioPath = &minioPath.String
		}
		if expiresAt.Valid {
			t := expiresAt.Time
			item.ExpiresAt = &t
		}
		if completedAt.Valid {
			t := completedAt.Time
			item.CompletedAt = &t
		}
		if downloadedAt.Valid {
			t := downloadedAt.Time
			item.DownloadedAt = &t
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, false, fmt.Errorf("repo.ListExportLogs rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	var nextCursor *string
	if hasMore && len(items) > 0 {
		c := items[len(items)-1].RequestedAt.UTC().Format(time.RFC3339Nano)
		nextCursor = &c
	}
	return items, nextCursor, hasMore, nil
}

// InsertScheduledEmail inserts a new sys.scheduled_email row.
func (r *Repository) InsertScheduledEmail(ctx context.Context, tx *sql.Tx, row ScheduledEmailRow, recipients []string) error {
	recipJSON, err := json.Marshal(recipients)
	if err != nil {
		return fmt.Errorf("repo.InsertScheduledEmail: marshal recipients: %w", err)
	}
	db := dbOrTx{tx: tx}
	_, err = db.ExecContext(ctx, `
		INSERT INTO sys.scheduled_email
		    (id, report_slug, format, frequency, send_time, recipients_jsonb, active,
		     subject_template, body_template,
		     created_at, created_by, updated_at, updated_by, row_version, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),$10,now(),$10,1,$11)
	`, row.ID, row.ReportSlug, string(row.Format), string(row.Frequency),
		row.SendTime, recipJSON, row.Active,
		row.SubjectTemplate, row.BodyTemplate,
		row.CreatedBy, row.TenantID)
	if err != nil {
		return fmt.Errorf("repo.InsertScheduledEmail: %w", err)
	}
	return nil
}

// SoftDeleteScheduledEmail sets deleted_at on sys.scheduled_email.
func (r *Repository) SoftDeleteScheduledEmail(ctx context.Context, tx *sql.Tx, id uuid.UUID, actorID uuid.UUID, tenantID string) error {
	db := dbOrTx{tx: tx}
	_, err := db.ExecContext(ctx, `
		UPDATE sys.scheduled_email
		SET deleted_at=now(), deleted_by=$2, updated_at=now(),
		    updated_by=$2, row_version=row_version+1
		WHERE id=$1 AND tenant_id=$3 AND deleted_at IS NULL
	`, id, actorID, tenantID)
	if err != nil {
		return fmt.Errorf("repo.SoftDeleteScheduledEmail: %w", err)
	}
	return nil
}

// GetScheduledEmail fetches one sys.scheduled_email row.
func (r *Repository) GetScheduledEmail(ctx context.Context, id uuid.UUID, tenantID string) (*ScheduledEmailRow, []string, error) {
	row := r.primary.QueryRowContext(ctx, `
		SELECT id, report_slug, format, frequency, send_time, recipients_jsonb,
		       active, subject_template, body_template, last_sent_at, last_status,
		       created_at, created_by, tenant_id
		FROM sys.scheduled_email
		WHERE id=$1 AND tenant_id=$2 AND deleted_at IS NULL
	`, id, tenantID)

	var se ScheduledEmailRow
	var subj, body sql.NullString
	var lastSentAt sql.NullTime
	var lastStatus sql.NullString
	if err := row.Scan(&se.ID, &se.ReportSlug, &se.Format, &se.Frequency, &se.SendTime,
		&se.RecipientsJSON, &se.Active, &subj, &body, &lastSentAt, &lastStatus,
		&se.CreatedAt, &se.CreatedBy, &se.TenantID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("repo.GetScheduledEmail scan: %w", err)
	}
	if subj.Valid {
		se.SubjectTemplate = &subj.String
	}
	if body.Valid {
		se.BodyTemplate = &body.String
	}
	if lastSentAt.Valid {
		t := lastSentAt.Time
		se.LastSentAt = &t
	}
	if lastStatus.Valid {
		se.LastStatus = &lastStatus.String
	}

	var recipients []string
	if err := json.Unmarshal(se.RecipientsJSON, &recipients); err != nil {
		return nil, nil, fmt.Errorf("repo.GetScheduledEmail unmarshal: %w", err)
	}
	return &se, recipients, nil
}

// ListActiveScheduledEmails returns all active (not deleted) scheduled emails.
func (r *Repository) ListActiveScheduledEmails(ctx context.Context, tenantID string) ([]ScheduledEmailRow, error) {
	rows, err := r.primary.QueryContext(ctx, `
		SELECT id, report_slug, format, frequency, send_time, recipients_jsonb,
		       active, subject_template, body_template, last_sent_at, last_status,
		       created_at, created_by, tenant_id
		FROM sys.scheduled_email
		WHERE tenant_id=$1 AND active=true AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("repo.ListActiveScheduledEmails: %w", err)
	}
	defer rows.Close()

	var result []ScheduledEmailRow
	for rows.Next() {
		var se ScheduledEmailRow
		var subj, body, lastStatus sql.NullString
		var lastSentAt sql.NullTime
		if err := rows.Scan(&se.ID, &se.ReportSlug, &se.Format, &se.Frequency, &se.SendTime,
			&se.RecipientsJSON, &se.Active, &subj, &body, &lastSentAt, &lastStatus,
			&se.CreatedAt, &se.CreatedBy, &se.TenantID); err != nil {
			return nil, fmt.Errorf("repo.ListActiveScheduledEmails scan: %w", err)
		}
		result = append(result, se)
	}
	return result, rows.Err()
}

// InsertOptOut inserts a sys.scheduled_email_optout row (idempotent via ON CONFLICT DO NOTHING).
func (r *Repository) InsertOptOut(ctx context.Context, scheduledEmailID uuid.UUID, email, tokenHash, tenantID string) error {
	_, err := r.primary.ExecContext(ctx, `
		INSERT INTO sys.scheduled_email_optout
		    (id, scheduled_email_id, email, opted_out_at, token_hash, tenant_id)
		VALUES (gen_random_uuid(), $1, $2, now(), $3, $4)
		ON CONFLICT (scheduled_email_id, email) DO NOTHING
	`, scheduledEmailID, email, tokenHash, tenantID)
	if err != nil {
		return fmt.Errorf("repo.InsertOptOut: %w", err)
	}
	return nil
}

// GetOptOuts returns the list of opted-out emails for a scheduled_email_id.
func (r *Repository) GetOptOuts(ctx context.Context, scheduledEmailID uuid.UUID) ([]string, error) {
	rows, err := r.primary.QueryContext(ctx, `
		SELECT email FROM sys.scheduled_email_optout
		WHERE scheduled_email_id=$1
		ORDER BY opted_out_at
	`, scheduledEmailID)
	if err != nil {
		return nil, fmt.Errorf("repo.GetOptOuts: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("repo.GetOptOuts scan: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}

// UpdateScheduledEmailLastSent updates last_sent_at + last_status.
func (r *Repository) UpdateScheduledEmailLastSent(ctx context.Context, id uuid.UUID, status, tenantID string) error {
	_, err := r.primary.ExecContext(ctx, `
		UPDATE sys.scheduled_email
		SET last_sent_at=now(), last_status=$2,
		    updated_at=now(), row_version=row_version+1
		WHERE id=$1 AND tenant_id=$3
	`, id, status, tenantID)
	if err != nil {
		return fmt.Errorf("repo.UpdateScheduledEmailLastSent: %w", err)
	}
	return nil
}

// CountMVRows returns approximate row count for a given MV (for export threshold check).
// Uses pg_class.reltuples (fast estimate, not exact).
func (r *Repository) CountMVRows(ctx context.Context, mvName string) (int64, error) {
	if !isValidMVName(mvName) {
		return 0, fmt.Errorf("repo.CountMVRows: unknown mv_name %q", mvName)
	}
	// Extract schema.table parts.
	var schema, table string
	if _, err := fmt.Sscanf(mvName, "%4s.%s", &schema, &table); err != nil {
		// Fallback: just use COUNT(*) which is always correct.
		var count int64
		// #nosec G202 — mvName validated above.
		err2 := r.replica.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", mvName)).Scan(&count)
		return count, err2
	}

	var estimate float64
	err := r.primary.QueryRowContext(ctx, `
		SELECT COALESCE(reltuples, 0) FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2
	`, schema, table).Scan(&estimate)
	if err != nil {
		return 0, fmt.Errorf("repo.CountMVRows estimate: %w", err)
	}
	return int64(estimate), nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// scanExportLogRow scans a sql.Row into ExportLogRow.
func scanExportLogRow(row *sql.Row) (*ExportLogRow, error) {
	var r ExportLogRow
	var rowCount sql.NullInt64
	var minioPath, sha256Hash, signedURL, jobID sql.NullString
	var completedAt, expiresAt, downloadedAt sql.NullTime
	if err := row.Scan(&r.ID, &r.ReportSlug, &r.Format, &r.Status,
		&rowCount, &minioPath, &sha256Hash, &signedURL,
		&r.RequestedBy, &r.RequestedAt, &completedAt, &expiresAt, &downloadedAt,
		&jobID, &r.TenantID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("scanExportLogRow: %w", err)
	}
	if rowCount.Valid {
		rc := rowCount.Int64
		r.RowCount = &rc
	}
	if minioPath.Valid {
		r.MinioPath = &minioPath.String
	}
	if sha256Hash.Valid {
		r.SHA256Hash = &sha256Hash.String
	}
	if signedURL.Valid {
		r.SignedURL = &signedURL.String
	}
	if completedAt.Valid {
		t := completedAt.Time
		r.CompletedAt = &t
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		r.ExpiresAt = &t
	}
	if downloadedAt.Valid {
		t := downloadedAt.Time
		r.DownloadedAt = &t
	}
	if jobID.Valid {
		r.JobID = &jobID.String
	}
	return &r, nil
}

// dbOrTx is a helper that uses tx if non-nil, else falls back to db-level ops.
// Useful for repo methods that can operate inside or outside a transaction.
type dbOrTx struct {
	db *sql.DB
	tx *sql.Tx
}

func (d dbOrTx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if d.tx != nil {
		return d.tx.ExecContext(ctx, query, args...)
	}
	return d.db.ExecContext(ctx, query, args...)
}
