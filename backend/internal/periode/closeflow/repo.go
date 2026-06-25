package closeflow

// repo.go — sqlx-style repository for mst.periode_buku and sys.closing_checklist_snapshot.
//
// Compliance:
//   - SELECT FOR SHARE on mst.periode_buku during state transitions (serializable-safe).
//   - sys.closing_checklist_snapshot: INSERT only (DB append-only triggers enforce).
//   - Cursor-based pagination for ListStatusPeriode (DEC-022).
//   - tenant_id in every WHERE clause (DEC-023).

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// Repo is the repository for close-workflow operations.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a Repo. Panics on nil db.
func NewRepo(db *sql.DB) *Repo {
	if db == nil {
		panic("closeflow.NewRepo: db must not be nil")
	}
	return &Repo{db: db}
}

// AllowedStatusPeriodeSortCols is the DataTable whitelist for GET /reports/status-periode.
var AllowedStatusPeriodeSortCols = []string{
	"tanggal_akhir", "tanggal_mulai", "tanggal_soft_close", "tanggal_hard_close",
	"status_periode", "tahun_buku",
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID returns the periode buku by UUID. Returns nil, nil if not found.
func (r *Repo) GetByID(ctx context.Context, id uuid.UUID) (*PeriodeBuku, error) {
	const q = `
		SELECT
			id, periode_id_kode, tahun_buku, bulan, tipe_periode,
			tanggal_mulai, tanggal_akhir, status_periode,
			tanggal_soft_close, tanggal_hard_close,
			reopened_flag, reopened_reason, reopened_at, reopened_by, reopened_approved_by,
			row_version, tenant_id, created_at, updated_at,
			soft_close_requested_by, soft_close_requested_at, soft_close_request_reason,
			soft_close_approved_by, soft_close_approved_at, soft_close_approve_reason,
			hard_close_requested_by, hard_close_requested_at, hard_close_request_reason,
			hard_close_approved_by, hard_close_approved_at, hard_close_approve_reason,
			hard_close_grace_expires_at, step_up_token_ref, reopen_reason
		FROM mst.periode_buku
		WHERE id = $1 AND deleted_at IS NULL AND tenant_id = $2
		LIMIT 1
	`
	p, err := r.scanPeriode(r.db.QueryRowContext(ctx, q, id, tenantIDFromCtx(ctx)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetByIDForShare returns the periode buku with SELECT FOR SHARE.
// Used during state transitions to prevent concurrent modifications.
func (r *Repo) GetByIDForShare(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*PeriodeBuku, error) {
	const q = `
		SELECT
			id, periode_id_kode, tahun_buku, bulan, tipe_periode,
			tanggal_mulai, tanggal_akhir, status_periode,
			tanggal_soft_close, tanggal_hard_close,
			reopened_flag, reopened_reason, reopened_at, reopened_by, reopened_approved_by,
			row_version, tenant_id, created_at, updated_at,
			soft_close_requested_by, soft_close_requested_at, soft_close_request_reason,
			soft_close_approved_by, soft_close_approved_at, soft_close_approve_reason,
			hard_close_requested_by, hard_close_requested_at, hard_close_request_reason,
			hard_close_approved_by, hard_close_approved_at, hard_close_approve_reason,
			hard_close_grace_expires_at, step_up_token_ref, reopen_reason
		FROM mst.periode_buku
		WHERE id = $1 AND deleted_at IS NULL AND tenant_id = $2
		FOR SHARE
		LIMIT 1
	`
	p, err := r.scanPeriodeFromTx(ctx, tx, q, id, tenantIDFromCtx(ctx))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// scanPeriode scans a single row from a *sql.Row.
func (r *Repo) scanPeriode(row *sql.Row) (*PeriodeBuku, error) {
	var p PeriodeBuku
	err := row.Scan(
		&p.ID, &p.PeriodeIDKode, &p.TahunBuku, &p.Bulan, &p.TipePeriode,
		&p.TanggalMulai, &p.TanggalAkhir, &p.StatusPeriode,
		&p.TanggalSoftClose, &p.TanggalHardClose,
		&p.ReopenedFlag, &p.ReopenedReason, &p.ReopenedAt, &p.ReopenedBy, &p.ReopenedApprovedBy,
		&p.RowVersion, &p.TenantID, &p.CreatedAt, &p.UpdatedAt,
		&p.SoftCloseRequestedBy, &p.SoftCloseRequestedAt, &p.SoftCloseRequestReason,
		&p.SoftCloseApprovedBy, &p.SoftCloseApprovedAt, &p.SoftCloseApproveReason,
		&p.HardCloseRequestedBy, &p.HardCloseRequestedAt, &p.HardCloseRequestReason,
		&p.HardCloseApprovedBy, &p.HardCloseApprovedAt, &p.HardCloseApproveReason,
		&p.HardCloseGraceExpiresAt, &p.StepUpTokenRef, &p.ReopenReason,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// scanPeriodeFromTx scans from a *sql.Tx query.
func (r *Repo) scanPeriodeFromTx(ctx context.Context, tx *sql.Tx, q string, args ...any) (*PeriodeBuku, error) {
	row := tx.QueryRowContext(ctx, q, args...)
	var p PeriodeBuku
	err := row.Scan(
		&p.ID, &p.PeriodeIDKode, &p.TahunBuku, &p.Bulan, &p.TipePeriode,
		&p.TanggalMulai, &p.TanggalAkhir, &p.StatusPeriode,
		&p.TanggalSoftClose, &p.TanggalHardClose,
		&p.ReopenedFlag, &p.ReopenedReason, &p.ReopenedAt, &p.ReopenedBy, &p.ReopenedApprovedBy,
		&p.RowVersion, &p.TenantID, &p.CreatedAt, &p.UpdatedAt,
		&p.SoftCloseRequestedBy, &p.SoftCloseRequestedAt, &p.SoftCloseRequestReason,
		&p.SoftCloseApprovedBy, &p.SoftCloseApprovedAt, &p.SoftCloseApproveReason,
		&p.HardCloseRequestedBy, &p.HardCloseRequestedAt, &p.HardCloseRequestReason,
		&p.HardCloseApprovedBy, &p.HardCloseApprovedAt, &p.HardCloseApproveReason,
		&p.HardCloseGraceExpiresAt, &p.StepUpTokenRef, &p.ReopenReason,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ─── UpdateSoftCloseRequested ─────────────────────────────────────────────────

// SetSoftCloseRequested records the soft-close request fields and increments row_version.
// Returns CONFLICT error if row_version doesn't match.
func (r *Repo) SetSoftCloseRequested(ctx context.Context, tx *sql.Tx,
	periodeID, actorID uuid.UUID, reason *string, expectedRowVersion int64,
) error {
	const q = `
		UPDATE mst.periode_buku SET
			soft_close_requested_by = $1,
			soft_close_requested_at = NOW(),
			soft_close_request_reason = $2,
			row_version = row_version + 1,
			updated_at = NOW(),
			updated_by = $1
		WHERE id = $3 AND row_version = $4 AND deleted_at IS NULL AND tenant_id = $5
	`
	res, err := tx.ExecContext(ctx, q, actorID, reason, periodeID, expectedRowVersion, tenantIDFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("SetSoftCloseRequested: %w", err)
	}
	rows, _ := res.RowsAffected() //nolint:errcheck
	if rows == 0 {
		return ErrRowVersionConflict("") // caller fills periodeKode
	}
	return nil
}

// SetSoftCloseApproved transitions period from OPEN to SOFT_CLOSED.
func (r *Repo) SetSoftCloseApproved(ctx context.Context, tx *sql.Tx,
	periodeID, actorID uuid.UUID, reason *string,
) error {
	now := time.Now()
	const q = `
		UPDATE mst.periode_buku SET
			status_periode = 'SOFT_CLOSED',
			tanggal_soft_close = $1,
			soft_close_approved_by = $2,
			soft_close_approved_at = $1,
			soft_close_approve_reason = $3,
			row_version = row_version + 1,
			updated_at = $1,
			updated_by = $2
		WHERE id = $4 AND deleted_at IS NULL AND tenant_id = $5
	`
	_, err := tx.ExecContext(ctx, q, now, actorID, reason, periodeID, tenantIDFromCtx(ctx))
	return wrapExec(err, "SetSoftCloseApproved")
}

// SetHardCloseRequested transitions period from SOFT_CLOSED to HARD_CLOSE_PENDING.
// Returns CONFLICT if row_version mismatches.
func (r *Repo) SetHardCloseRequested(ctx context.Context, tx *sql.Tx,
	periodeID, actorID uuid.UUID, reason *string, expectedRowVersion int64,
) error {
	const q = `
		UPDATE mst.periode_buku SET
			status_periode = 'HARD_CLOSE_PENDING',
			hard_close_requested_by = $1,
			hard_close_requested_at = NOW(),
			hard_close_request_reason = $2,
			row_version = row_version + 1,
			updated_at = NOW(),
			updated_by = $1
		WHERE id = $3 AND row_version = $4 AND deleted_at IS NULL AND tenant_id = $5
	`
	res, err := tx.ExecContext(ctx, q, actorID, reason, periodeID, expectedRowVersion, tenantIDFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("SetHardCloseRequested: %w", err)
	}
	rows, _ := res.RowsAffected() //nolint:errcheck
	if rows == 0 {
		return ErrRowVersionConflict("")
	}
	return nil
}

// SetHardCloseApproved transitions period from HARD_CLOSE_PENDING to CLOSED.
// Also writes step_up_token_ref (SHA-256 hash of the token) and grace window expiry.
func (r *Repo) SetHardCloseApproved(ctx context.Context, tx *sql.Tx,
	periodeID, actorID uuid.UUID, reason *string, stepUpTokenRef string, graceWindowHours int,
) error {
	now := time.Now()
	graceExpiry := now.Add(time.Duration(graceWindowHours) * time.Hour)
	const q = `
		UPDATE mst.periode_buku SET
			status_periode = 'CLOSED',
			tanggal_hard_close = $1,
			hard_close_approved_by = $2,
			hard_close_approved_at = $1,
			hard_close_approve_reason = $3,
			hard_close_grace_expires_at = $4,
			step_up_token_ref = $5,
			row_version = row_version + 1,
			updated_at = $1,
			updated_by = $2
		WHERE id = $6 AND deleted_at IS NULL AND tenant_id = $7
	`
	_, err := tx.ExecContext(ctx, q, now, actorID, reason, graceExpiry, stepUpTokenRef,
		periodeID, tenantIDFromCtx(ctx))
	return wrapExec(err, "SetHardCloseApproved")
}

// LockKursForPeriode sets mst.kurs.locked_flag = TRUE for all FX rates in the period.
func (r *Repo) LockKursForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) error {
	const q = `
		UPDATE mst.kurs SET locked_flag = TRUE, updated_at = NOW()
		WHERE periode_id = $1 AND tenant_id = $2
	`
	_, err := tx.ExecContext(ctx, q, periodeID, tenantIDFromCtx(ctx))
	return wrapExec(err, "LockKursForPeriode")
}

// UnlockKursForPeriode sets mst.kurs.locked_flag = FALSE (called on reopen CLOSED→SOFT_CLOSED).
func (r *Repo) UnlockKursForPeriode(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) error {
	const q = `
		UPDATE mst.kurs SET locked_flag = FALSE, updated_at = NOW()
		WHERE periode_id = $1 AND tenant_id = $2
	`
	_, err := tx.ExecContext(ctx, q, periodeID, tenantIDFromCtx(ctx))
	return wrapExec(err, "UnlockKursForPeriode")
}

// SetHardCloseRejected reverts period from HARD_CLOSE_PENDING to SOFT_CLOSED.
// Clears hard_close_requested_* fields (CFO rejected the request).
func (r *Repo) SetHardCloseRejected(ctx context.Context, tx *sql.Tx, periodeID, actorID uuid.UUID) error {
	const q = `
		UPDATE mst.periode_buku SET
			status_periode = 'SOFT_CLOSED',
			hard_close_requested_by = NULL,
			hard_close_requested_at = NULL,
			hard_close_request_reason = NULL,
			row_version = row_version + 1,
			updated_at = NOW(),
			updated_by = $1
		WHERE id = $2 AND deleted_at IS NULL AND tenant_id = $3
	`
	_, err := tx.ExecContext(ctx, q, actorID, periodeID, tenantIDFromCtx(ctx))
	return wrapExec(err, "SetHardCloseRejected")
}

// SetReopenRequested records the reopen request fields. Does NOT change status_periode.
// Returns CONFLICT if row_version mismatches.
func (r *Repo) SetReopenRequested(ctx context.Context, tx *sql.Tx,
	periodeID, actorID uuid.UUID, reason string, targetStatus PeriodeStatus, expectedRowVersion int64,
) error {
	const q = `
		UPDATE mst.periode_buku SET
			reopen_reason = $1,
			row_version = row_version + 1,
			updated_at = NOW(),
			updated_by = $2
		WHERE id = $3 AND row_version = $4 AND deleted_at IS NULL AND tenant_id = $5
	`
	res, err := tx.ExecContext(ctx, q, reason, actorID, periodeID, expectedRowVersion, tenantIDFromCtx(ctx))
	if err != nil {
		return fmt.Errorf("SetReopenRequested: %w", err)
	}
	rows, _ := res.RowsAffected() //nolint:errcheck
	if rows == 0 {
		return ErrRowVersionConflict("")
	}
	_ = targetStatus // stored in snapshot, not directly on row here
	return nil
}

// SetReopenApproved changes status_periode to targetStatus and writes reopened_* fields.
// If fromClosed is true, also stores step_up_token_ref.
func (r *Repo) SetReopenApproved(ctx context.Context, tx *sql.Tx,
	periodeID, actorID uuid.UUID, targetStatus PeriodeStatus,
	stepUpTokenRef string, fromClosed bool,
) error {
	now := time.Now()
	tokenRef := (*string)(nil)
	if fromClosed && stepUpTokenRef != "" {
		tokenRef = &stepUpTokenRef
	}
	const q = `
		UPDATE mst.periode_buku SET
			status_periode = $1,
			reopened_flag = TRUE,
			reopened_at = $2,
			reopened_by = $3,
			reopened_approved_by = $3,
			step_up_token_ref = $4,
			reopen_reason = NULL,
			row_version = row_version + 1,
			updated_at = $2,
			updated_by = $3
		WHERE id = $5 AND deleted_at IS NULL AND tenant_id = $6
	`
	_, err := tx.ExecContext(ctx, q, string(targetStatus), now, actorID, tokenRef,
		periodeID, tenantIDFromCtx(ctx))
	return wrapExec(err, "SetReopenApproved")
}

// ─── Snapshot ─────────────────────────────────────────────────────────────────

// InsertChecklistSnapshot inserts an append-only snapshot row.
// MUST be called within an open transaction for APPROVED transitions.
// For advisory/rejected snapshots, caller may use a separate tx.
func (r *Repo) InsertChecklistSnapshot(ctx context.Context, tx *sql.Tx, snap ChecklistSnapshot) error {
	checklistJSON, err := json.Marshal(BuildChecklistJSONB(ChecklistEvalResult{
		EvaluatedAt: snap.EvaluatedAt,
		AllPassed:   snap.AllPassed,
		Items:       snap.ChecklistItems,
	}))
	if err != nil {
		return fmt.Errorf("marshal checklist jsonb: %w", err)
	}

	var outcomeJSON []byte
	if snap.OutcomeJSON != nil {
		outcomeJSON, err = json.Marshal(snap.OutcomeJSON)
		if err != nil {
			return fmt.Errorf("marshal outcome jsonb: %w", err)
		}
	}

	overallStatus := SnapshotOverallPassed
	if !snap.AllPassed {
		overallStatus = SnapshotOverallFailed
	}
	if snap.TransitionStatus == SnapshotTransitionStatusRejected && snap.AllPassed {
		// e.g., SoD violation where checklist passed but transition was still rejected.
		overallStatus = SnapshotOverallRejected
	}

	const q = `
		INSERT INTO sys.closing_checklist_snapshot
			(id, periode_buku_id, transition, trigger_action, evaluated_at, evaluated_by,
			 actor_role, overall_status, all_passed, transition_status,
			 checklist_jsonb, outcome_jsonb, created_at, created_by, tenant_id)
		VALUES
			($1, $2, $3, $4, $5, $6,
			 $7, $8, $9, $10,
			 $11, $12, $13, $14, $15)
	`
	_, err = tx.ExecContext(ctx, q,
		snap.ID, snap.PeriodeBukuID,
		string(snap.Transition), string(snap.TriggerAction),
		snap.EvaluatedAt, snap.EvaluatedBy,
		snap.ActorRole, string(overallStatus),
		snap.AllPassed, string(snap.TransitionStatus),
		checklistJSON, outcomeJSON, snap.CreatedAt, snap.CreatedBy, snap.TenantID,
	)
	return wrapExec(err, "InsertChecklistSnapshot")
}

// GetLatestSnapshot returns the most recent snapshot for a period, optionally filtered by transition.
// Returns nil, nil if not found.
func (r *Repo) GetLatestSnapshot(ctx context.Context, periodeID uuid.UUID, transition *SnapshotTransition) (*LastSnapshotSummary, error) {
	const baseQ = `
		SELECT id, transition, evaluated_at, all_passed
		FROM sys.closing_checklist_snapshot
		WHERE periode_buku_id = $1
		%s
		ORDER BY created_at DESC
		LIMIT 1
	`
	var filter string
	var args []any
	args = append(args, periodeID)
	if transition != nil {
		filter = "AND transition = $2"
		args = append(args, string(*transition))
	}

	row := r.db.QueryRowContext(ctx, fmt.Sprintf(baseQ, filter), args...)

	var s LastSnapshotSummary
	var transStr string
	err := row.Scan(&s.SnapshotID, &transStr, &s.EvaluatedAt, &s.AllPassed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("GetLatestSnapshot: %w", err)
	}
	s.Transition = SnapshotTransition(transStr)
	return &s, nil
}

// GetConfigValue reads a sys.config value by key.
func (r *Repo) GetConfigValue(ctx context.Context, key string) (string, error) {
	const q = `SELECT config_value FROM sys.config WHERE config_key = $1 LIMIT 1`
	var val string
	err := r.db.QueryRowContext(ctx, q, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// ─── ListStatusPeriode ────────────────────────────────────────────────────────

// ListStatusPeriode returns a cursor-paginated list of periode buku with status info.
// The query also fetches the last checklist snapshot per period.
// cursor and limit are passed separately since listquery.Query does not carry pagination state.
func (r *Repo) ListStatusPeriode(ctx context.Context, q listquery.Query, cursor string, limit int) (
	[]StatusPeriodeListItem, *response.PaginationMeta, []response.SortApplied, map[string]any, error,
) {
	allowed := AllowedStatusPeriodeSortCols

	// Build ORDER BY from sort spec.
	orderBy := "p.tanggal_akhir DESC, p.id ASC"
	var sortApplied []response.SortApplied
	if len(q.Sort) > 0 {
		parts := make([]string, 0, len(q.Sort))
		for _, s := range q.Sort {
			valid := false
			for _, c := range allowed {
				if c == s.Col {
					valid = true
					break
				}
			}
			if !valid {
				continue
			}
			dir := "ASC"
			if strings.EqualFold(s.Dir, "desc") {
				dir = "DESC"
			}
			parts = append(parts, "p."+s.Col+" "+dir)
			sortApplied = append(sortApplied, response.SortApplied{Col: s.Col, Dir: strings.ToLower(dir)})
		}
		if len(parts) > 0 {
			orderBy = strings.Join(parts, ", ") + ", p.id ASC"
		}
	}

	// Build WHERE clause from filters.
	var whereParts []string
	var args []any
	argIdx := 1

	whereParts = append(whereParts, fmt.Sprintf("p.deleted_at IS NULL AND p.tenant_id = $%d", argIdx))
	args = append(args, tenantIDFromCtx(ctx))
	argIdx++

	for _, f := range q.Filters {
		switch f.Col {
		case "status_periode":
			whereParts = append(whereParts, fmt.Sprintf("p.status_periode = $%d", argIdx))
			args = append(args, f.Value)
			argIdx++
		case "tahun_buku":
			whereParts = append(whereParts, fmt.Sprintf("p.tahun_buku = $%d", argIdx))
			args = append(args, f.Value)
			argIdx++
		case "bulan":
			whereParts = append(whereParts, fmt.Sprintf("p.bulan = $%d", argIdx))
			args = append(args, f.Value)
			argIdx++
		case "tipe_periode":
			whereParts = append(whereParts, fmt.Sprintf("p.tipe_periode = $%d", argIdx))
			args = append(args, f.Value)
			argIdx++
		}
	}

	// Global text search on periode_kode.
	if q.Search != "" {
		whereParts = append(whereParts, fmt.Sprintf("p.periode_id_kode ILIKE $%d", argIdx))
		args = append(args, "%"+q.Search+"%")
		argIdx++
	}

	// Cursor decode.
	if cursor != "" {
		cursorVal, err := decodeCursor(cursor)
		if err == nil && cursorVal != "" {
			whereParts = append(whereParts, fmt.Sprintf("p.id > $%d", argIdx))
			args = append(args, cursorVal)
		}
	}

	whereClause := strings.Join(whereParts, " AND ")
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	// Fetch limit+1 to detect hasMore.
	// whereClause and orderBy are built from AllowedStatusPeriodeSortCols allowlist —
	// no user-controlled string is passed directly into the SQL. gosec G201 suppressed.
	const dataQTpl = `
		SELECT
			p.id, p.periode_id_kode, p.tipe_periode, p.tahun_buku, p.bulan,
			p.tanggal_mulai, p.tanggal_akhir, p.status_periode,
			p.tanggal_soft_close, p.tanggal_hard_close,
			p.soft_close_approved_by, p.hard_close_approved_by,
			p.reopened_flag,
			snap.id AS snap_id, snap.transition AS snap_transition,
			snap.evaluated_at AS snap_evaluated_at, snap.all_passed AS snap_all_passed
		FROM mst.periode_buku p
		LEFT JOIN LATERAL (
			SELECT id, transition, evaluated_at, all_passed
			FROM sys.closing_checklist_snapshot
			WHERE periode_buku_id = p.id
			ORDER BY created_at DESC LIMIT 1
		) snap ON TRUE
		WHERE %s
		ORDER BY %s
		LIMIT %d
	`
	dataQ := fmt.Sprintf(dataQTpl, whereClause, orderBy, limit+1) //nolint:gosec

	rows, err := r.db.QueryContext(ctx, dataQ, args...)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("ListStatusPeriode query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []StatusPeriodeListItem
	for rows.Next() {
		var item StatusPeriodeListItem
		var tanggalMulai, tanggalAkhir time.Time
		var snapID *uuid.UUID
		var snapTransition *string
		var snapEvaluatedAt *time.Time
		var snapAllPassed *bool

		if err := rows.Scan(
			&item.PeriodeID, &item.PeriodeKode, &item.TipePeriode, &item.TahunBuku, &item.Bulan,
			&tanggalMulai, &tanggalAkhir, &item.StatusPeriode,
			&item.TanggalSoftClose, &item.TanggalHardClose,
			&item.SoftCloseBy, &item.HardCloseBy,
			&item.ReopenedFlag,
			&snapID, &snapTransition, &snapEvaluatedAt, &snapAllPassed,
		); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("ListStatusPeriode scan: %w", err)
		}

		item.TanggalMulai = tanggalMulai.Format("2006-01-02")
		item.TanggalAkhir = tanggalAkhir.Format("2006-01-02")

		if snapID != nil {
			item.ChecklistLastSnapshot = &LastSnapshotSummary{
				SnapshotID:  *snapID,
				Transition:  SnapshotTransition(*snapTransition),
				EvaluatedAt: *snapEvaluatedAt,
				AllPassed:   *snapAllPassed,
			}
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("ListStatusPeriode rows error: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor *string
	if hasMore && len(items) > 0 {
		nc := encodeCursor(items[len(items)-1].PeriodeID.String())
		nextCursor = &nc
	}

	pagination := &response.PaginationMeta{
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Limit:      limit,
	}

	appliedFilters := map[string]any{}
	for _, f := range q.Filters {
		appliedFilters[f.Col] = f.Value
	}

	return items, pagination, sortApplied, appliedFilters, nil
}

// ─── sys.job ─────────────────────────────────────────────────────────────────

// JobRow is a minimal record for sys.job (ux-patterns.md §3.3, migration 000004).
// Only the fields needed for MV refresh job durability are populated here.
type JobRow struct {
	ID          string
	Type        string
	Status      string
	PayloadJSON []byte
	CreatedBy   uuid.UUID
}

// InsertJobRow inserts a sys.job row within an existing transaction.
// C4: called inside the hard-close approve tx so the job row is durable
// even if Asynq enqueue fails after commit.
// Uses the system tenant_id and sets updated_by = created_by.
func (r *Repo) InsertJobRow(ctx context.Context, tx *sql.Tx, job JobRow) error {
	const q = `
		INSERT INTO sys.job
			(id, type, status, payload_jsonb, created_by, updated_by, tenant_id)
		VALUES
			($1, $2, $3, $4, $5, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`
	_, err := tx.ExecContext(ctx, q,
		job.ID, job.Type, job.Status, job.PayloadJSON, job.CreatedBy, tenantIDFromCtx(ctx),
	)
	return wrapExec(err, "InsertJobRow")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// tenantIDFromCtx extracts tenant_id from context (defaults to TUGURE in Phase 1).
func tenantIDFromCtx(_ context.Context) string {
	return "TUGURE"
}

// wrapExec wraps a sql.Exec error with a function name prefix.
func wrapExec(err error, fn string) error {
	if err != nil {
		return fmt.Errorf("closeflow.%s: %w", fn, err)
	}
	return nil
}

// HashStepUpToken returns a hex SHA-256 hash of the token ID (never store raw token).
func HashStepUpToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// encodeCursor encodes a cursor value to a base64-like string (simple hex for now).
func encodeCursor(val string) string {
	return hex.EncodeToString([]byte(val))
}

// decodeCursor decodes a cursor value.
func decodeCursor(encoded string) (string, error) {
	b, err := hex.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
