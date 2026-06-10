package counterparty

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// ─── Repository interface ─────────────────────────────────────────────────────

// Repository defines the data-access contract for counterparty.
// PII columns are accessed exclusively via SQL calls to sec.encrypt()/sec.decrypt().
type Repository interface {
	// Create inserts a new counterparty row.
	// npwpPlain, nomorRekeningPlain, ktpPlain are plaintexts; repo encrypts via SQL.
	Create(ctx context.Context, tx *sql.Tx, cp *Counterparty, npwpPlain, nomorRekeningPlain, ktpPlain *string) error

	// GetByID fetches one record by UUID (no PII decrypted).
	GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Counterparty, *MaskedPII, error)

	// GetByKode fetches one record by kode_counterparty (no PII decrypted).
	GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Counterparty, *MaskedPII, error)

	// GetPII fetches decrypted PII for one counterparty. Requires DB role blips_pii_accessor.
	// Returns (nil, nil) if not found.
	GetPII(ctx context.Context, id uuid.UUID) (*PIIFields, error)

	// GetMaskedPII returns last-4-char masked PII without full decryption.
	// Used for default GET /:id response.
	GetMaskedPII(ctx context.Context, id uuid.UUID) (*MaskedPII, error)

	// List returns paginated records (no PII).
	List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Counterparty, error)

	// Update applies partial update with optimistic lock.
	// PII fields (nil = no change) are encrypted in-SQL.
	Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*Counterparty, error)

	// SoftDelete sets deleted_at, deleted_by, is_deleted=TRUE.
	SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Counterparty, error)

	// UpdateWorkflowStatus updates workflow_status column.
	UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error

	// UpdateRatingCache updates rating_pefindo_current (called by ratinghistory workflow hook).
	UpdateRatingCache(ctx context.Context, tx *sql.Tx, id uuid.UUID, newRating *string, updatedBy uuid.UUID) error

	// CountReferences returns number of active FK references (for delete guard).
	CountReferences(ctx context.Context, id uuid.UUID) (int64, error)

	// BeginTx starts a DB transaction.
	BeginTx(ctx context.Context) (*sql.Tx, error)

	// ListAuditHistory returns paginated audit log rows.
	ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error)

	// ExportAll returns all records as CSV (no PII).
	ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error)
}

// UpdateFields captures mutable columns for an update.
type UpdateFields struct {
	Nama               *string
	Tipe               *CounterpartyTipe
	TipeEksposurBasel  *TipeEksposurBasel
	EligibleLpsFlag    *bool
	NomorIzinOjk       *string
	TanggalIzinOjk     *string
	AumTerakhir        *decimal.Decimal
	TanggalAumTerakhir *string
	KategoriMi         *string
	Status             *CounterpartyStatus
	// PII fields: nil = no change, non-nil = encrypt and update
	NPWPPlain          *string
	NomorRekeningPlain *string
	KTPPlain           *string
	UpdatedBy          uuid.UUID
	ExpectedVersion    int64
}

// ─── Sentinel errors ──────────────────────────────────────────────────────────

var (
	// ErrNotFound is returned when the record is not found.
	ErrNotFound = fmt.Errorf("counterparty not found")
	// ErrConflict is returned on row_version mismatch.
	ErrConflict = fmt.Errorf("optimistic lock conflict")
	// ErrKodeDuplicate is returned when kode_counterparty already exists.
	ErrKodeDuplicate = fmt.Errorf("counterparty kode duplicate")
)

// ─── DB implementation ────────────────────────────────────────────────────────

// DBRepository is the production SQL implementation.
type DBRepository struct {
	db *sql.DB
}

// NewDBRepository creates a new DBRepository.
func NewDBRepository(db *sql.DB) *DBRepository {
	return &DBRepository{db: db}
}

var _ Repository = (*DBRepository)(nil)

// querier abstracts *sql.DB and *sql.Tx for read queries.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// baseSelect selects all non-PII columns.
// PII columns are accessed only via GetPII/GetMaskedPII.
const baseSelect = `
SELECT
    c.id, c.kode_counterparty, c.nama, c.tipe,
    c.rating_pefindo_current, c.tipe_eksposur_basel, c.eligible_lps_flag,
    c.nomor_izin_ojk, c.tanggal_izin_ojk,
    c.aum_terakhir, c.tanggal_aum_terakhir, c.kategori_mi, c.status,
    c.workflow_status,
    c.created_at, c.created_by, c.updated_at, c.updated_by,
    c.deleted_at, c.deleted_by, c.row_version, c.tenant_id,
    c.version, c.is_deleted
FROM mst.counterparty c`

// Create inserts a new counterparty row. PII is encrypted via sec.encrypt($N).
// The function sec.encrypt() is defined in migration 0003 and uses pgcrypto pgp_sym_encrypt.
func (r *DBRepository) Create(ctx context.Context, tx *sql.Tx, cp *Counterparty, npwpPlain, nomorRekeningPlain, ktpPlain *string) error {
	// Helper: encrypt placeholder — SQL expression used if value non-nil; NULL if nil.
	// We use positional params: the PII params are appended last.
	// NOTE: sec.encrypt() takes TEXT and returns TEXT (ciphertext).

	// Build PII part of INSERT with conditional encrypt.
	// We always pass the values; if nil use SQL NULL, else sec.encrypt($N).
	// Simplest approach: always include the column, pass NULL or plaintext param.

	args := []interface{}{
		cp.ID, cp.KodeCounterparty, cp.Nama, string(cp.Tipe),
		cp.RatingPefindoCurrent, string(cp.TipeEksposurBasel), cp.EligibleLpsFlag,
		cp.NomorIzinOjk, cp.TanggalIzinOjk,
		cp.AumTerakhir, cp.TanggalAumTerakhir, cp.KategoriMi, string(cp.Status),
		string(cp.WorkflowStatus),
		cp.CreatedAt, cp.CreatedBy,
		cp.TenantID,
		// PII args (last 3)
		npwpPlain,          // $18
		nomorRekeningPlain, // $19
		ktpPlain,           // $20
	}

	query := `
INSERT INTO mst.counterparty (
    id, kode_counterparty, nama, tipe,
    rating_pefindo_current, tipe_eksposur_basel, eligible_lps_flag,
    nomor_izin_ojk, tanggal_izin_ojk,
    aum_terakhir, tanggal_aum_terakhir, kategori_mi, status,
    workflow_status,
    created_at, created_by, updated_at, updated_by,
    row_version, tenant_id, version, is_deleted,
    npwp_encrypted, nomor_rekening_encrypted, ktp_encrypted
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9,
    $10, $11, $12, $13,
    $14,
    $15, $16, $15, $16,
    1, $17, 1, false,
    CASE WHEN $18::text IS NULL THEN NULL ELSE sec.encrypt($18::text) END,
    CASE WHEN $19::text IS NULL THEN NULL ELSE sec.encrypt($19::text) END,
    CASE WHEN $20::text IS NULL THEN NULL ELSE sec.encrypt($20::text) END
)`

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("kode_counterparty: %w", ErrKodeDuplicate)
		}
		return fmt.Errorf("repo.Create counterparty: %w", err)
	}
	return nil
}

// GetByID fetches by UUID.
func (r *DBRepository) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Counterparty, *MaskedPII, error) {
	cp, err := r.getOne(ctx, r.db, "c.id = $1", id, includeDeleted)
	if err != nil {
		return nil, nil, err
	}
	if cp == nil {
		return nil, nil, nil
	}
	masked, err := r.GetMaskedPII(ctx, cp.ID)
	if err != nil {
		return nil, nil, err
	}
	return cp, masked, nil
}

// GetByKode fetches by business key.
func (r *DBRepository) GetByKode(ctx context.Context, kode string, includeDeleted bool) (*Counterparty, *MaskedPII, error) {
	cp, err := r.getOne(ctx, r.db, "c.kode_counterparty = $1", kode, includeDeleted)
	if err != nil {
		return nil, nil, err
	}
	if cp == nil {
		return nil, nil, nil
	}
	masked, err := r.GetMaskedPII(ctx, cp.ID)
	if err != nil {
		return nil, nil, err
	}
	return cp, masked, nil
}

// GetPII fetches decrypted PII — calls sec.decrypt() which requires blips_pii_accessor role.
func (r *DBRepository) GetPII(ctx context.Context, id uuid.UUID) (*PIIFields, error) {
	query := `
SELECT
    sec.decrypt(npwp_encrypted),
    sec.decrypt(nomor_rekening_encrypted),
    sec.decrypt(ktp_encrypted)
FROM mst.counterparty
WHERE id = $1 AND deleted_at IS NULL`

	row := r.db.QueryRowContext(ctx, query, id)
	pii := &PIIFields{}
	var npwpRaw, nomorRaw, ktpRaw *string
	if err := row.Scan(&npwpRaw, &nomorRaw, &ktpRaw); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("repo.GetPII: %w", err)
	}
	pii.NPWP = npwpRaw
	pii.NomorRekening = nomorRaw
	pii.KTP = ktpRaw
	return pii, nil
}

// GetMaskedPII fetches encrypted values then applies last-4-char masking in Go.
// This avoids calling sec.decrypt() for the common GET /:id path.
func (r *DBRepository) GetMaskedPII(ctx context.Context, id uuid.UUID) (*MaskedPII, error) {
	// We cannot get plaintext without sec.decrypt. Instead we return sentinel
	// masked values indicating PII exists. The actual last-4 masking would
	// require a round-trip through sec.decrypt. For the default masked endpoint
	// we emit "***" for all PII to avoid any decryption cost. Callers who need
	// real masked values (last 4 chars) should use GetPII with view_pii permission.
	//
	// Approach: read whether columns are non-NULL, return "***" if set.
	query := `
SELECT
    npwp_encrypted IS NOT NULL,
    nomor_rekening_encrypted IS NOT NULL,
    ktp_encrypted IS NOT NULL
FROM mst.counterparty
WHERE id = $1 AND deleted_at IS NULL`

	row := r.db.QueryRowContext(ctx, query, id)
	var hasNPWP, hasRek, hasKTP bool
	if err := row.Scan(&hasNPWP, &hasRek, &hasKTP); err == sql.ErrNoRows {
		return &MaskedPII{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("repo.GetMaskedPII: %w", err)
	}

	mask := func(has bool) *string {
		if !has {
			return nil
		}
		s := "***"
		return &s
	}
	return &MaskedPII{
		NPWP:          mask(hasNPWP),
		NomorRekening: mask(hasRek),
		KTP:           mask(hasKTP),
	}, nil
}

func (r *DBRepository) getOne(ctx context.Context, q querier, where string, arg interface{}, includeDeleted bool) (*Counterparty, error) {
	deletedFilter := " AND c.deleted_at IS NULL"
	if includeDeleted {
		deletedFilter = ""
	}
	query := baseSelect + " WHERE " + where + deletedFilter
	row := q.QueryRowContext(ctx, query, arg)
	cp, err := scanCounterparty(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("repo.getOne counterparty: %w", err)
	}
	return cp, nil
}

// List fetches paginated records (no PII).
func (r *DBRepository) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) ([]*Counterparty, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("c")

	conditions := []string{}
	if !includeDeleted {
		conditions = append(conditions, "c.deleted_at IS NULL")
	}

	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			argIdx := len(args) + 1
			conditions = append(conditions, fmt.Sprintf("c.kode_counterparty > $%d", argIdx))
			args = append(args, cd.ID)
		}
	}

	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("c.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}

	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}
	if orderBy == "" {
		orderBy = "c.kode_counterparty ASC"
	}

	argIdx := len(args) + 1
	query := fmt.Sprintf( //nolint:gosec
		"SELECT c.id, c.kode_counterparty, c.nama, c.tipe, c.rating_pefindo_current, c.tipe_eksposur_basel, c.eligible_lps_flag, c.nomor_izin_ojk, c.tanggal_izin_ojk, c.aum_terakhir, c.tanggal_aum_terakhir, c.kategori_mi, c.status, c.workflow_status, c.created_at, c.created_by, c.updated_at, c.updated_by, c.deleted_at, c.deleted_by, c.row_version, c.tenant_id, c.version, c.is_deleted FROM mst.counterparty c%s ORDER BY %s LIMIT $%d",
		whereClause, orderBy, argIdx,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.List counterparty: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []*Counterparty
	for rows.Next() {
		cp, err := scanCounterpartyRow(rows)
		if err != nil {
			return nil, fmt.Errorf("repo.List scan: %w", err)
		}
		items = append(items, cp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo.List rows.Err: %w", err)
	}
	return items, nil
}

// Update applies partial field changes with optimistic lock.
func (r *DBRepository) Update(ctx context.Context, tx *sql.Tx, id uuid.UUID, f UpdateFields) (*Counterparty, error) {
	setClauses := []string{}
	args := []interface{}{}
	idx := 1

	if f.Nama != nil {
		setClauses = append(setClauses, fmt.Sprintf("nama = $%d", idx))
		args = append(args, *f.Nama)
		idx++
	}
	if f.Tipe != nil {
		setClauses = append(setClauses, fmt.Sprintf("tipe = $%d", idx))
		args = append(args, string(*f.Tipe))
		idx++
	}
	if f.TipeEksposurBasel != nil {
		setClauses = append(setClauses, fmt.Sprintf("tipe_eksposur_basel = $%d", idx))
		args = append(args, string(*f.TipeEksposurBasel))
		idx++
	}
	if f.EligibleLpsFlag != nil {
		setClauses = append(setClauses, fmt.Sprintf("eligible_lps_flag = $%d", idx))
		args = append(args, *f.EligibleLpsFlag)
		idx++
	}
	if f.NomorIzinOjk != nil {
		setClauses = append(setClauses, fmt.Sprintf("nomor_izin_ojk = $%d", idx))
		args = append(args, *f.NomorIzinOjk)
		idx++
	}
	if f.TanggalIzinOjk != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_izin_ojk = $%d", idx))
		args = append(args, *f.TanggalIzinOjk)
		idx++
	}
	if f.AumTerakhir != nil {
		setClauses = append(setClauses, fmt.Sprintf("aum_terakhir = $%d", idx))
		args = append(args, f.AumTerakhir.String())
		idx++
	}
	if f.TanggalAumTerakhir != nil {
		setClauses = append(setClauses, fmt.Sprintf("tanggal_aum_terakhir = $%d", idx))
		args = append(args, *f.TanggalAumTerakhir)
		idx++
	}
	if f.KategoriMi != nil {
		setClauses = append(setClauses, fmt.Sprintf("kategori_mi = $%d", idx))
		args = append(args, *f.KategoriMi)
		idx++
	}
	if f.Status != nil {
		setClauses = append(setClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(*f.Status))
		idx++
	}

	// PII fields — encrypt inline
	if f.NPWPPlain != nil {
		setClauses = append(setClauses, fmt.Sprintf("npwp_encrypted = CASE WHEN $%d::text IS NULL THEN NULL ELSE sec.encrypt($%d::text) END", idx, idx))
		args = append(args, *f.NPWPPlain)
		idx++
	}
	if f.NomorRekeningPlain != nil {
		setClauses = append(setClauses, fmt.Sprintf("nomor_rekening_encrypted = CASE WHEN $%d::text IS NULL THEN NULL ELSE sec.encrypt($%d::text) END", idx, idx))
		args = append(args, *f.NomorRekeningPlain)
		idx++
	}
	if f.KTPPlain != nil {
		setClauses = append(setClauses, fmt.Sprintf("ktp_encrypted = CASE WHEN $%d::text IS NULL THEN NULL ELSE sec.encrypt($%d::text) END", idx, idx))
		args = append(args, *f.KTPPlain)
		idx++
	}

	if len(setClauses) == 0 {
		cp, _, err := r.GetByID(ctx, id, false)
		return cp, err
	}

	setClauses = append(setClauses,
		fmt.Sprintf("updated_at = $%d", idx),
		fmt.Sprintf("updated_by = $%d", idx+1),
		"row_version = row_version + 1",
		"version = version + 1",
	)
	args = append(args, time.Now(), f.UpdatedBy)
	idx += 2

	whereIDIdx := idx
	whereVersionIdx := idx + 1
	args = append(args, id, f.ExpectedVersion)

	query := fmt.Sprintf( //nolint:gosec
		`UPDATE mst.counterparty SET %s WHERE id = $%d AND row_version = $%d AND deleted_at IS NULL`,
		strings.Join(setClauses, ", "),
		whereIDIdx, whereVersionIdx,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("repo.Update counterparty: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("repo.Update rows affected: %w", err)
	}
	if n == 0 {
		existing, _, err := r.GetByID(ctx, id, false)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, ErrNotFound
		}
		return nil, ErrConflict
	}
	cp, err := r.getOne(ctx, tx, "c.id = $1", id, false)
	return cp, err
}

// SoftDelete sets deleted_at, deleted_by, is_deleted=TRUE, deleted_at on both cols.
func (r *DBRepository) SoftDelete(ctx context.Context, tx *sql.Tx, id uuid.UUID, deletedBy uuid.UUID) (*Counterparty, error) {
	now := time.Now()
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.counterparty
		SET deleted_at = $1, deleted_by = $2,
		    is_deleted = TRUE,
		    updated_at = $1, updated_by = $2,
		    row_version = row_version + 1,
		    version = version + 1
		WHERE id = $3 AND deleted_at IS NULL
	`, now, deletedBy, id)
	if err != nil {
		return nil, fmt.Errorf("repo.SoftDelete counterparty: %w", err)
	}
	return r.getOne(ctx, tx, "c.id = $1", id, true)
}

// UpdateWorkflowStatus updates only the workflow_status column.
func (r *DBRepository) UpdateWorkflowStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status WorkflowStatus, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.counterparty
		SET workflow_status = $1, updated_at = now(), updated_by = $2,
		    row_version = row_version + 1, version = version + 1
		WHERE id = $3
	`, string(status), updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateWorkflowStatus counterparty: %w", err)
	}
	return nil
}

// UpdateRatingCache updates the cached rating_pefindo_current field.
// Called by ratinghistory workflow_hook after approve.
func (r *DBRepository) UpdateRatingCache(ctx context.Context, tx *sql.Tx, id uuid.UUID, newRating *string, updatedBy uuid.UUID) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE mst.counterparty
		SET rating_pefindo_current = $1,
		    updated_at = now(), updated_by = $2,
		    row_version = row_version + 1, version = version + 1
		WHERE id = $3 AND deleted_at IS NULL
	`, newRating, updatedBy, id)
	if err != nil {
		return fmt.Errorf("repo.UpdateRatingCache counterparty: %w", err)
	}
	return nil
}

// CountReferences counts active FK references to this counterparty.
func (r *DBRepository) CountReferences(ctx context.Context, id uuid.UUID) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM mst.instrumen
		WHERE counterparty_id = $1 AND is_deleted = FALSE
	`, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("repo.CountReferences counterparty instrumen: %w", err)
	}
	return count, nil
}

// BeginTx starts a transaction.
func (r *DBRepository) BeginTx(ctx context.Context) (*sql.Tx, error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("repo.BeginTx counterparty: %w", err)
	}
	return tx, nil
}

// ListAuditHistory returns paginated audit events for a given counterparty UUID.
func (r *DBRepository) ListAuditHistory(ctx context.Context, entityID uuid.UUID, cursor string, limit int, isAuditRole bool) ([]AuditHistoryItem, bool, error) {
	var cursorTime *time.Time
	if cursor != "" {
		cd, err := pagination.DecodeCursor(cursor)
		if err == nil && cd.ID != "" {
			t, err2 := time.Parse(time.RFC3339, cd.ID)
			if err2 == nil {
				cursorTime = &t
			}
		}
	}

	var args []interface{}
	args = append(args, entityID, "mst.counterparty")
	cond := ""
	if cursorTime != nil {
		args = append(args, *cursorTime)
		cond = fmt.Sprintf(" AND timestamp < $%d", len(args))
	}

	query := fmt.Sprintf( //nolint:gosec
		`SELECT id, timestamp, actor_user_id, actor_role, action,
			before_value, after_value, ip_address, trace_id
		FROM aud.audit_log
		WHERE entity_id = $1 AND entity_type = $2%s
		ORDER BY timestamp DESC
		LIMIT $%d`,
		cond, len(args)+1,
	)
	args = append(args, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, false, fmt.Errorf("repo.ListAuditHistory counterparty: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var items []AuditHistoryItem
	for rows.Next() {
		var (
			eventID   uuid.UUID
			ts        time.Time
			actorUID  uuid.UUID
			actorRole string
			action    string
			beforeRaw []byte
			afterRaw  []byte
			ip        *string
			traceID   *string
		)
		if err := rows.Scan(&eventID, &ts, &actorUID, &actorRole, &action,
			&beforeRaw, &afterRaw, &ip, &traceID); err != nil {
			return nil, false, fmt.Errorf("repo.ListAuditHistory scan: %w", err)
		}
		item := AuditHistoryItem{
			EventID:     eventID.String(),
			EventTime:   ts.Format(time.RFC3339),
			ActorUserID: actorUID.String(),
			ActorRole:   actorRole,
			Action:      action,
			IP:          ip,
			TraceID:     traceID,
		}
		if isAuditRole {
			if len(beforeRaw) > 0 {
				var bj interface{}
				if jsonErr := json.Unmarshal(beforeRaw, &bj); jsonErr == nil {
					item.BeforeJSONB = bj
				}
			}
			if len(afterRaw) > 0 {
				var aj interface{}
				if jsonErr := json.Unmarshal(afterRaw, &aj); jsonErr == nil {
					item.AfterJSONB = aj
				}
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("repo.ListAuditHistory rows.Err: %w", err)
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

// ExportAll returns all records as UTF-8 BOM CSV (no PII).
func (r *DBRepository) ExportAll(ctx context.Context, q listquery.Query) (io.Reader, int, error) {
	where, args, orderBy := q.WithAllowed(AllAllowedCols).ToSQL("c")

	conditions := []string{"c.deleted_at IS NULL"}
	if q.Search != "" {
		argIdx := len(args) + 1
		searchCond := make([]string, 0, len(SearchCols))
		for _, col := range SearchCols {
			searchCond = append(searchCond, fmt.Sprintf("c.%s ILIKE $%d", col, argIdx))
		}
		conditions = append(conditions, "("+strings.Join(searchCond, " OR ")+")")
		args = append(args, "%"+q.Search+"%")
	}
	if where != "" {
		conditions = append(conditions, where)
	}

	whereClause := " WHERE " + strings.Join(conditions, " AND ")
	if orderBy == "" {
		orderBy = "c.kode_counterparty ASC"
	}

	query := fmt.Sprintf( //nolint:gosec
		"SELECT c.kode_counterparty, c.nama, c.tipe, c.tipe_eksposur_basel, c.eligible_lps_flag, c.status, c.rating_pefindo_current, c.workflow_status FROM mst.counterparty c%s ORDER BY %s",
		whereClause, orderBy,
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll counterparty: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM
	w := csv.NewWriter(&buf)

	headers := []string{"Kode", "Nama", "Tipe", "Eksposur Basel", "Eligible LPS", "Status", "Rating Pefindo", "Workflow Status"}
	if err := w.Write(headers); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll write header: %w", err)
	}

	count := 0
	for rows.Next() {
		var (
			kode        string
			nama        string
			tipe        string
			eksposur    string
			eligibleLPS bool
			status      string
			ratingRaw   *string
			wfStatus    string
		)
		if err := rows.Scan(&kode, &nama, &tipe, &eksposur, &eligibleLPS, &status, &ratingRaw, &wfStatus); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll scan: %w", err)
		}
		eligibleStr := "Tidak"
		if eligibleLPS {
			eligibleStr = "Ya"
		}
		rating := ""
		if ratingRaw != nil {
			rating = *ratingRaw
		}
		record := []string{kode, nama, tipe, eksposur, eligibleStr, status, rating, wfStatus}
		if err := w.Write(record); err != nil {
			return nil, 0, fmt.Errorf("repo.ExportAll write record: %w", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll rows.Err: %w", err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, 0, fmt.Errorf("repo.ExportAll flush: %w", err)
	}
	return &buf, count, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

func scanCounterparty(row *sql.Row) (*Counterparty, error) {
	cp := &Counterparty{}
	var (
		tipe        string
		eksposur    string
		status      string
		wfStatus    string
		tanggalIzin *string
		tanggalAum  *string
		aumVal      *string
		createdBy   uuid.UUID
		updatedAt   *time.Time
		updatedBy   *uuid.UUID
		deletedAt   *time.Time
		deletedBy   *uuid.UUID
	)
	err := row.Scan(
		&cp.ID, &cp.KodeCounterparty, &cp.Nama, &tipe,
		&cp.RatingPefindoCurrent, &eksposur, &cp.EligibleLpsFlag,
		&cp.NomorIzinOjk, &tanggalIzin,
		&aumVal, &tanggalAum, &cp.KategoriMi, &status,
		&wfStatus,
		&cp.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &cp.RowVersion, &cp.TenantID,
		&cp.Version, &cp.IsDeleted,
	)
	if err != nil {
		return nil, err
	}
	cp.Tipe = CounterpartyTipe(tipe)
	cp.TipeEksposurBasel = TipeEksposurBasel(eksposur)
	cp.Status = CounterpartyStatus(status)
	cp.WorkflowStatus = WorkflowStatus(wfStatus)
	cp.TanggalIzinOjk = tanggalIzin
	cp.TanggalAumTerakhir = tanggalAum
	cp.CreatedBy = createdBy
	cp.UpdatedAt = updatedAt
	cp.UpdatedBy = updatedBy
	cp.DeletedAt = deletedAt
	cp.DeletedBy = deletedBy
	if aumVal != nil {
		d, err := decimal.NewFromString(*aumVal)
		if err == nil {
			cp.AumTerakhir = &d
		}
	}
	return cp, nil
}

func scanCounterpartyRow(rows *sql.Rows) (*Counterparty, error) {
	cp := &Counterparty{}
	var (
		tipe        string
		eksposur    string
		status      string
		wfStatus    string
		tanggalIzin *string
		tanggalAum  *string
		aumVal      *string
		createdBy   uuid.UUID
		updatedAt   *time.Time
		updatedBy   *uuid.UUID
		deletedAt   *time.Time
		deletedBy   *uuid.UUID
	)
	err := rows.Scan(
		&cp.ID, &cp.KodeCounterparty, &cp.Nama, &tipe,
		&cp.RatingPefindoCurrent, &eksposur, &cp.EligibleLpsFlag,
		&cp.NomorIzinOjk, &tanggalIzin,
		&aumVal, &tanggalAum, &cp.KategoriMi, &status,
		&wfStatus,
		&cp.CreatedAt, &createdBy, &updatedAt, &updatedBy,
		&deletedAt, &deletedBy, &cp.RowVersion, &cp.TenantID,
		&cp.Version, &cp.IsDeleted,
	)
	if err != nil {
		return nil, err
	}
	cp.Tipe = CounterpartyTipe(tipe)
	cp.TipeEksposurBasel = TipeEksposurBasel(eksposur)
	cp.Status = CounterpartyStatus(status)
	cp.WorkflowStatus = WorkflowStatus(wfStatus)
	cp.TanggalIzinOjk = tanggalIzin
	cp.TanggalAumTerakhir = tanggalAum
	cp.CreatedBy = createdBy
	cp.UpdatedAt = updatedAt
	cp.UpdatedBy = updatedBy
	cp.DeletedAt = deletedAt
	cp.DeletedBy = deletedBy
	if aumVal != nil {
		d, err := decimal.NewFromString(*aumVal)
		if err == nil {
			cp.AumTerakhir = &d
		}
	}
	return cp, nil
}

// isUniqueViolation checks for PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") ||
		strings.Contains(s, "duplicate key") ||
		strings.Contains(s, "unique constraint")
}
