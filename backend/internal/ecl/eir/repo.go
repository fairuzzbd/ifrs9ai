// Package eir — repository layer for ecl.eir_amortization_schedule,
// ecl.eir_reestimation_log, mst.instrumen EIR fields, and sys.drift_report (P4-M6).
//
// Design principles:
//   - All queries use parameterized SQL (no string concat, no SQLi risk).
//   - NUMERIC columns read via ::text cast to avoid float64 (DEC-016).
//   - No hard-delete in ecl.* or sys.drift_report (DEC-018).
//   - Schedule rows: INSERT-only; amounts are immutable after insert.
//     Only recomputed_from_seq may be updated (DB trigger enforces, service guards too).
//
// References:
//   - db/migrations/000026_eir_schema_fix.up.sql (M5 schema details)
//   - db/migrations/000027_drift_report_and_amendment_lifecycle.up.sql (M6 additions)
//   - DEC-016, DEC-018, formulas.md §EIR Newton-Raphson
package eir

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
	"blips-ifrs9.tugu-re.com/internal/common/response"
)

// ─── Cursor helpers ───────────────────────────────────────────────────────────

// encodeCursorStr wraps pagination.EncodeCursor for string values.
func encodeCursorStr(val string) string {
	c, err := pagination.EncodeCursor(pagination.CursorData{ID: val})
	if err != nil {
		return ""
	}
	return c
}

// decodeCursorStr wraps pagination.DecodeCursor.
func decodeCursorStr(cursor string) (string, error) {
	data, err := pagination.DecodeCursor(cursor)
	if err != nil {
		return "", err
	}
	return data.ID, nil
}

// ─── ScheduleRepoIface ─────────────────────────────────────────────────────

// ScheduleRepoIface defines operations on ecl.eir_amortization_schedule.
type ScheduleRepoIface interface {
	// InsertBatch inserts all schedule rows within tx.
	InsertBatch(ctx context.Context, tx *sql.Tx, rows []ScheduleRow) error

	// MarkSuperseded sets recomputed_from_seq = firstNewSeq for active rows
	// where instrumen_id = instrumenID and periode_seq < firstNewSeq.
	// ONLY recomputed_from_seq and audit cols change (financial amounts immutable).
	MarkSuperseded(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, firstNewSeq int, updatedBy uuid.UUID) error

	// GetActiveByPeriode returns active rows ordered by periode_seq ASC.
	// If periodSeqFilter = 0, returns all active rows.
	GetActiveByPeriode(ctx context.Context, instrumenID uuid.UUID, periodSeqFilter int) ([]ScheduleRow, error)

	// GetMaxPeriodeSeq returns max periode_seq among active rows, or 0 if none.
	GetMaxPeriodeSeq(ctx context.Context, instrumenID uuid.UUID) (int, error)

	// HasActiveRows returns true if at least one active row exists.
	HasActiveRows(ctx context.Context, instrumenID uuid.UUID) (bool, error)

	// List returns paginated schedule rows (DataTable).
	List(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, includeSuperseded bool, cursor string, limit int) ([]ScheduleRow, *response.PaginationMeta, error)

	// GetGrossCarryingAtDate returns the closing_carrying of the latest active
	// schedule row whose tanggal_posting <= asOf, for use in catch-up adjustment.
	// Returns ErrEIRScheduleNotFound if no qualifying row exists.
	// Per IFRS 9 §5.4.3: gross carrying at amendment date = book value before remeasurement.
	// NUMERIC(20,4) read via ::text to avoid float64 (DEC-016).
	GetGrossCarryingAtDate(ctx context.Context, instrumenID uuid.UUID, asOf time.Time) (decimal.Decimal, error)
}

// DBEIRScheduleRepo implements ScheduleRepoIface against ecl.eir_amortization_schedule.
type DBEIRScheduleRepo struct {
	db *sql.DB
}

// NewDBEIRScheduleRepo creates a DBEIRScheduleRepo.
func NewDBEIRScheduleRepo(db *sql.DB) *DBEIRScheduleRepo {
	return &DBEIRScheduleRepo{db: db}
}

// InsertBatch inserts schedule rows in a single bulk INSERT within tx.
// 17 params per row: id, instrumen_id, periode_seq, tanggal_posting,
// opening_carrying, cash_inflow, pendapatan_bunga_eir, amortisasi_p_d,
// pelunasan_pokok, closing_carrying, eir_periode, stage_saat_posting,
// status_posting, flag_poci, created_by, updated_by, tenant_id.
func (r *DBEIRScheduleRepo) InsertBatch(ctx context.Context, tx *sql.Tx, rows []ScheduleRow) error {
	if len(rows) == 0 {
		return nil
	}

	placeholders := make([]string, len(rows))
	args := make([]interface{}, 0, len(rows)*17) //nolint:mnd // 17 cols per row
	idx := 1

	for i := range rows {
		r := &rows[i]
		placeholders[i] = fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d::NUMERIC(20,4),$%d::NUMERIC(20,4),$%d::NUMERIC(20,4),$%d::NUMERIC(20,4),$%d::NUMERIC(20,4),$%d::NUMERIC(20,4),$%d::NUMERIC(10,8),$%d,$%d,$%d,$%d,$%d,$%d)",
			idx, idx+1, idx+2, idx+3, idx+4, idx+5,
			idx+6, idx+7, idx+8, idx+9, idx+10,
			idx+11, idx+12, idx+13, idx+14, idx+15, idx+16,
		)
		args = append(args,
			r.ID,
			r.InstrumenID,
			r.PeriodeSeq,
			r.TanggalPosting,
			r.OpeningCarrying.StringFixed(4),
			r.CashInflow.StringFixed(4),
			r.PendapatanBungaEIR.StringFixed(4),
			r.AmortisasiPD.StringFixed(4),
			r.PelunasanPokok.StringFixed(4),
			r.ClosingCarrying.StringFixed(4),
			r.EIRPeriode.StringFixed(8),
			r.StageSaatPosting,
			r.StatusPosting,
			r.FlagPOCI,
			r.CreatedBy,
			r.UpdatedBy,
			r.TenantID,
		)
		idx += 17 //nolint:mnd // 17 params per row
	}

	// placeholders contains only $N positional params — no user input is interpolated.
	const insertPrefix = `INSERT INTO ecl.eir_amortization_schedule
		(id, instrumen_id, periode_seq, tanggal_posting,
		 opening_carrying, cash_inflow, pendapatan_bunga_eir,
		 amortisasi_p_d, pelunasan_pokok, closing_carrying,
		 eir_periode, stage_saat_posting, status_posting,
		 flag_poci, created_by, updated_by, tenant_id)
		VALUES `
	q := insertPrefix + strings.Join(placeholders, ",") //nolint:gosec // positional $N params only

	_, err := tx.ExecContext(ctx, q, args...)
	return err
}

// MarkSuperseded updates recomputed_from_seq for active rows with periode_seq < firstNewSeq.
// Financial amounts are NOT changed (DB trigger tg_eir_schedule_amounts_immutable also guards this).
func (r *DBEIRScheduleRepo) MarkSuperseded(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, firstNewSeq int, updatedBy uuid.UUID) error {
	q := `UPDATE ecl.eir_amortization_schedule
		SET recomputed_from_seq = $1,
		    updated_by = $2
		WHERE instrumen_id = $3
		  AND recomputed_from_seq IS NULL
		  AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, firstNewSeq, updatedBy, instrumenID)
	return err
}

// GetActiveByPeriode returns active schedule rows ordered by periode_seq ASC.
// If periodSeqFilter > 0, returns only rows with periode_seq <= periodSeqFilter.
func (r *DBEIRScheduleRepo) GetActiveByPeriode(ctx context.Context, instrumenID uuid.UUID, periodSeqFilter int) ([]ScheduleRow, error) {
	q := `SELECT id, instrumen_id, periode_seq, tanggal_posting,
		         opening_carrying::text, cash_inflow::text,
		         pendapatan_bunga_eir::text, amortisasi_p_d::text,
		         pelunasan_pokok::text, closing_carrying::text,
		         eir_periode::text, stage_saat_posting, status_posting,
		         flag_poci, recomputed_from_seq,
		         created_at, created_by, updated_at, updated_by,
		         deleted_at, tenant_id, row_version
		FROM ecl.eir_amortization_schedule
		WHERE instrumen_id = $1
		  AND recomputed_from_seq IS NULL
		  AND deleted_at IS NULL`

	args := []interface{}{instrumenID}
	if periodSeqFilter > 0 {
		q += " AND periode_seq <= $2"
		args = append(args, periodSeqFilter)
	}
	q += " ORDER BY periode_seq ASC"

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	return scanScheduleRows(rows)
}

// GetMaxPeriodeSeq returns max active periode_seq, or 0 if no active rows.
func (r *DBEIRScheduleRepo) GetMaxPeriodeSeq(ctx context.Context, instrumenID uuid.UUID) (int, error) {
	var maxSeq sql.NullInt64
	q := `SELECT MAX(periode_seq) FROM ecl.eir_amortization_schedule
		WHERE instrumen_id = $1 AND recomputed_from_seq IS NULL AND deleted_at IS NULL`
	if err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&maxSeq); err != nil {
		return 0, err
	}
	if !maxSeq.Valid {
		return 0, nil
	}
	return int(maxSeq.Int64), nil
}

// HasActiveRows returns true if at least one active row exists for instrumenID.
func (r *DBEIRScheduleRepo) HasActiveRows(ctx context.Context, instrumenID uuid.UUID) (bool, error) {
	var count int
	q := `SELECT COUNT(1) FROM ecl.eir_amortization_schedule
		WHERE instrumen_id = $1 AND recomputed_from_seq IS NULL AND deleted_at IS NULL LIMIT 1`
	if err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetGrossCarryingAtDate returns closing_carrying from the latest active schedule row
// with tanggal_posting <= asOf.  Used by computeCatchUpAdjustment (IFRS 9 §5.4.3).
// NUMERIC read via ::text to avoid float64 (DEC-016).
// Returns ErrEIRScheduleNotFound when no qualifying row exists.
func (r *DBEIRScheduleRepo) GetGrossCarryingAtDate(ctx context.Context, instrumenID uuid.UUID, asOf time.Time) (decimal.Decimal, error) {
	var closingStr string
	q := `SELECT closing_carrying::text
		FROM ecl.eir_amortization_schedule
		WHERE instrumen_id     = $1
		  AND recomputed_from_seq IS NULL
		  AND deleted_at          IS NULL
		  AND tanggal_posting     <= $2
		ORDER BY tanggal_posting DESC, periode_seq DESC
		LIMIT 1`
	err := r.db.QueryRowContext(ctx, q, instrumenID, asOf).Scan(&closingStr)
	if err == sql.ErrNoRows {
		return decimal.Zero, ErrEIRScheduleNotFound(instrumenID.String())
	}
	if err != nil {
		return decimal.Zero, fmt.Errorf("GetGrossCarryingAtDate: %w", err)
	}
	v, err := decimal.NewFromString(closingStr)
	if err != nil {
		return decimal.Zero, fmt.Errorf("GetGrossCarryingAtDate parse closing_carrying: %w", err)
	}
	return v, nil
}

// List returns a paginated list of schedule rows.
func (r *DBEIRScheduleRepo) List(ctx context.Context, instrumenID uuid.UUID, q listquery.Query, includeSuperseded bool, cursor string, limit int) ([]ScheduleRow, *response.PaginationMeta, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	whereClause, args, orderBy := q.WithAllowed(AllowedColsSchedule).ToSQL("s")
	argIdx := len(args) + 1

	baseWhere := fmt.Sprintf("s.instrumen_id = $%d AND s.deleted_at IS NULL", argIdx)
	args = append(args, instrumenID)
	argIdx++

	if !includeSuperseded {
		baseWhere += " AND s.recomputed_from_seq IS NULL"
	}

	if cursor != "" {
		if decoded, decErr := decodeCursorStr(cursor); decErr == nil && decoded != "" {
			baseWhere += fmt.Sprintf(" AND s.periode_seq > $%d", argIdx)
			args = append(args, decoded)
			argIdx++
		}
	}
	_ = argIdx

	fullWhere := baseWhere
	if whereClause != "" {
		fullWhere = baseWhere + " AND " + whereClause
	}
	if orderBy == "" {
		orderBy = "s.periode_seq ASC"
	}

	//nolint:gosec // fullWhere and orderBy contain only validated column names from AllowedColsSchedule whitelist
	query := fmt.Sprintf(`SELECT id, instrumen_id, periode_seq, tanggal_posting,
		         opening_carrying::text, cash_inflow::text,
		         pendapatan_bunga_eir::text, amortisasi_p_d::text,
		         pelunasan_pokok::text, closing_carrying::text,
		         eir_periode::text, stage_saat_posting, status_posting,
		         flag_poci, recomputed_from_seq,
		         created_at, created_by, updated_at, updated_by,
		         deleted_at, tenant_id, row_version
		FROM ecl.eir_amortization_schedule s
		WHERE %s
		ORDER BY %s
		LIMIT %d`, fullWhere, orderBy, limit+1)

	dbRows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer dbRows.Close() //nolint:errcheck

	result, err := scanScheduleRows(dbRows)
	if err != nil {
		return nil, nil, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}

	var nextCursor *string
	if hasMore && len(result) > 0 {
		last := result[len(result)-1]
		c := encodeCursorStr(fmt.Sprintf("%d", last.PeriodeSeq))
		nextCursor = &c
	}

	return result, &response.PaginationMeta{
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Limit:      limit,
	}, nil
}

// scanScheduleRows scans *sql.Rows into []ScheduleRow.
// All NUMERIC columns read as ::text to avoid float64 (DEC-016).
func scanScheduleRows(rows *sql.Rows) ([]ScheduleRow, error) {
	var result []ScheduleRow
	for rows.Next() {
		var row ScheduleRow
		var (
			openStr, cashStr, pendStr, amortStr, pelStr, closStr, eirStr string
			deletedAt                                                    *time.Time
			recomputedFromSeq                                            *int
		)
		if err := rows.Scan(
			&row.ID, &row.InstrumenID, &row.PeriodeSeq, &row.TanggalPosting,
			&openStr, &cashStr, &pendStr, &amortStr, &pelStr, &closStr,
			&eirStr, &row.StageSaatPosting, &row.StatusPosting,
			&row.FlagPOCI, &recomputedFromSeq,
			&row.CreatedAt, &row.CreatedBy, &row.UpdatedAt, &row.UpdatedBy,
			&deletedAt, &row.TenantID, &row.RowVersion,
		); err != nil {
			return nil, err
		}
		row.DeletedAt = deletedAt
		row.RecomputedFromSeq = recomputedFromSeq

		var err error
		if row.OpeningCarrying, err = decimal.NewFromString(openStr); err != nil {
			return nil, fmt.Errorf("opening_carrying: %w", err)
		}
		if row.CashInflow, err = decimal.NewFromString(cashStr); err != nil {
			return nil, fmt.Errorf("cash_inflow: %w", err)
		}
		if row.PendapatanBungaEIR, err = decimal.NewFromString(pendStr); err != nil {
			return nil, fmt.Errorf("pendapatan_bunga_eir: %w", err)
		}
		if row.AmortisasiPD, err = decimal.NewFromString(amortStr); err != nil {
			return nil, fmt.Errorf("amortisasi_p_d: %w", err)
		}
		if row.PelunasanPokok, err = decimal.NewFromString(pelStr); err != nil {
			return nil, fmt.Errorf("pelunasan_pokok: %w", err)
		}
		if row.ClosingCarrying, err = decimal.NewFromString(closStr); err != nil {
			return nil, fmt.Errorf("closing_carrying: %w", err)
		}
		if row.EIRPeriode, err = decimal.NewFromString(eirStr); err != nil {
			return nil, fmt.Errorf("eir_periode: %w", err)
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// ─── InstrumenEIRRepoIface ────────────────────────────────────────────────────

// InstrumenEIRRepoIface is the read + limited-write interface for mst.instrumen (EIR fields).
type InstrumenEIRRepoIface interface {
	// GetByID fetches the EIR-relevant projection from mst.instrumen.
	GetByID(ctx context.Context, instrumenID uuid.UUID) (*InstrumenForEIR, error)

	// ListActiveForBulk streams AC/FVOCI instruments with eir_method_flag=TRUE.
	// Streaming via channel for ≤10KB per instrument memory footprint.
	// The BulkScope is passed to filter by instrumen_ids if scope=SUBSET.
	ListActiveForBulk(ctx context.Context, scope BulkScope) (<-chan InstrumenForEIR, error)

	// UpdateEIRAwal sets mst.instrumen.eir_awal within tx (DEC-016: NUMERIC(10,8)).
	UpdateEIRAwal(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, eirAwal decimal.Decimal, updatedBy uuid.UUID) error
}

// DBInstrumenEIRRepo implements InstrumenEIRRepoIface.
type DBInstrumenEIRRepo struct {
	db *sql.DB
}

// NewDBInstrumenEIRRepo creates a DBInstrumenEIRRepo.
func NewDBInstrumenEIRRepo(db *sql.DB) *DBInstrumenEIRRepo {
	return &DBInstrumenEIRRepo{db: db}
}

const instrumenForEIRCols = `id, kode_instrumen, klasifikasi_psak71, eir_method_flag,
       eir_awal::text, flag_poci,
       nominal::text, biaya_transaksi_capitalized::text,
       kupon::text, tanggal_penempatan, tanggal_jatuh_tempo,
       status, deleted_at, tenant_id`

// GetByID fetches the EIR projection for an instrument.
func (r *DBInstrumenEIRRepo) GetByID(ctx context.Context, instrumenID uuid.UUID) (*InstrumenForEIR, error) {
	q := `SELECT ` + instrumenForEIRCols + ` FROM mst.instrumen WHERE id = $1`
	row := r.db.QueryRowContext(ctx, q, instrumenID)
	return scanInstrumenForEIR(row)
}

// ListActiveForBulk streams active AC/FVOCI EIR instruments via a channel.
// The channel is closed when all rows have been sent or an error occurs.
// scope parameter reserved; currently sends all matching rows.
func (r *DBInstrumenEIRRepo) ListActiveForBulk(ctx context.Context, _ BulkScope) (<-chan InstrumenForEIR, error) {
	q := `SELECT ` + instrumenForEIRCols + `
		FROM mst.instrumen
		WHERE eir_method_flag = TRUE
		  AND klasifikasi_psak71 IN ('AC','FVOCI')
		  AND deleted_at IS NULL
		ORDER BY id ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}

	ch := make(chan InstrumenForEIR)
	go func() {
		defer close(ch)
		defer rows.Close() //nolint:errcheck
		for rows.Next() {
			inst, err := scanInstrumenForEIR(rows)
			if err != nil || inst == nil {
				return
			}
			select {
			case ch <- *inst:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// UpdateEIRAwal sets eir_awal = NUMERIC(10,8) on mst.instrumen within tx.
func (r *DBInstrumenEIRRepo) UpdateEIRAwal(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, eirAwal decimal.Decimal, updatedBy uuid.UUID) error {
	q := `UPDATE mst.instrumen
		SET eir_awal = $1::NUMERIC(10,8),
		    updated_by = $2,
		    updated_at = now()
		WHERE id = $3`
	_, err := tx.ExecContext(ctx, q, eirAwal.StringFixed(8), updatedBy, instrumenID)
	return err
}

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

// scanInstrumenForEIR scans one mst.instrumen row into InstrumenForEIR.
// NUMERIC columns read via ::text (DEC-016).
func scanInstrumenForEIR(s scanner) (*InstrumenForEIR, error) {
	var inst InstrumenForEIR
	var (
		eirAwalNull, kuponNull sql.NullString
		nominalStr, biayaStr   string
	)
	err := s.Scan(
		&inst.ID, &inst.KodeInstrumen, &inst.KlasifikasiPsak71, &inst.EIRMethodFlag,
		&eirAwalNull, &inst.FlagPOCI,
		&nominalStr, &biayaStr,
		&kuponNull, &inst.TanggalPenempatan, &inst.TanggalJatuhTempo,
		&inst.Status, &inst.DeletedAt, &inst.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if eirAwalNull.Valid && eirAwalNull.String != "" {
		v, e := decimal.NewFromString(eirAwalNull.String)
		if e == nil {
			inst.EIRAwal = &v
		}
	}
	if inst.Nominal, err = decimal.NewFromString(nominalStr); err != nil {
		return nil, fmt.Errorf("nominal: %w", err)
	}
	if inst.BiayaTransaksiCapitalized, err = decimal.NewFromString(biayaStr); err != nil {
		return nil, fmt.Errorf("biaya_transaksi: %w", err)
	}
	if kuponNull.Valid && kuponNull.String != "" {
		v, e := decimal.NewFromString(kuponNull.String)
		if e == nil {
			inst.Kupon = &v
		}
	}
	return &inst, nil
}

// ─── AmendmentRepoIface ───────────────────────────────────────────────────────

// AmendmentRepoIface defines CRUD for ecl.eir_reestimation_log.
type AmendmentRepoIface interface {
	// Create inserts a new proposal row within tx.
	Create(ctx context.Context, tx *sql.Tx, proposal *AmendmentProposal) error

	// Update updates workflow state columns within tx.
	Update(ctx context.Context, tx *sql.Tx, proposal *AmendmentProposal) error

	// GetByID fetches a proposal by ID.
	GetByID(ctx context.Context, proposalID uuid.UUID) (*AmendmentProposal, error)

	// HasActiveProposal returns true if a DRAFT/PENDING_REVIEW/PENDING_APPROVAL proposal exists.
	HasActiveProposal(ctx context.Context, instrumenID uuid.UUID) (bool, error)

	// List returns paginated proposals.
	List(ctx context.Context, q listquery.Query, cursor string, limit int, actorID uuid.UUID, isAdmin bool) ([]AmendmentProposal, *response.PaginationMeta, error)

	// Cancel transitions proposal to CANCELLED within tx (P4-M6, story M6-005).
	// Sets cancelled_at, cancel_reason, cancelled_by in ecl.eir_reestimation_log.
	// DB CHECK: cancel_reason length >= 20.
	Cancel(ctx context.Context, tx *sql.Tx, proposalID uuid.UUID, cancelReason string, cancelledBy uuid.UUID) error

	// ListQueue returns paginated proposals for the review queue (P4-M6, story M6-004).
	// Returns non-terminal proposals enriched with kode_instrumen, abs_diff.
	ListQueue(ctx context.Context, q listquery.Query, cursor string, limit int) ([]QueueRow, *response.PaginationMeta, error)

	// GetByDocumentAndInstrumen returns the most-recent non-terminal DOCUMENT_UPLOAD
	// proposal for (documentID, instrumenID).  Used for idempotent detect (B3 fix).
	// Returns (nil, nil) if no such proposal exists.
	GetByDocumentAndInstrumen(ctx context.Context, documentID uuid.UUID, instrumenID uuid.UUID) (*AmendmentProposal, error)
}

// DBAmendmentRepo implements AmendmentRepoIface against ecl.eir_reestimation_log.
type DBAmendmentRepo struct {
	db *sql.DB
}

// NewDBAmendmentRepo creates a DBAmendmentRepo.
func NewDBAmendmentRepo(db *sql.DB) *DBAmendmentRepo {
	return &DBAmendmentRepo{db: db}
}

// Create inserts a new EIR amendment proposal.
// DB column name: tanggal_re_estimation (DATE) per init_schema migration.
// Cashflows stored as JSON in modifikasi_terms_json (JSONB).
// eir_sebelum maps to EIRLama in the Go struct.
func (r *DBAmendmentRepo) Create(ctx context.Context, tx *sql.Tx, p *AmendmentProposal) error {
	var eirSebelumStr string
	if p.EIRLama != nil {
		eirSebelumStr = p.EIRLama.StringFixed(8)
	} else {
		eirSebelumStr = "0.00000000"
	}

	q := `INSERT INTO ecl.eir_reestimation_log
		(id, instrumen_id, tanggal_re_estimation,
		 modifikasi_terms_json, trigger_type,
		 workflow_status, eir_sebelum, carrying_sebelum, carrying_sesudah,
		 maker_id, dokumen_pendukung_id,
		 created_by, updated_by, tenant_id)
		VALUES ($1,$2,$3,$4,'AMENDMENT',$5,$6::NUMERIC(10,8),$7::NUMERIC(20,4),$8::NUMERIC(20,4),$9,$10,$9,$9,$11)`

	var dokumenID interface{}
	if p.DokumenPendukungID != nil {
		dokumenID = *p.DokumenPendukungID
	}

	var makerID interface{}
	if p.MakerID != nil {
		makerID = *p.MakerID
	}

	carryingSebelum := "0.0000"
	if p.CarryingSebelum != nil {
		carryingSebelum = p.CarryingSebelum.StringFixed(4)
	}
	carryingSesudah := "0.0000"
	if p.CarryingSesudah != nil {
		carryingSesudah = p.CarryingSesudah.StringFixed(4)
	}

	_, err := tx.ExecContext(ctx, q,
		p.ID, p.InstrumenID, p.TanggalAmandemen,
		p.RevisedCashflowJSON,
		string(p.Status), eirSebelumStr,
		carryingSebelum, carryingSesudah,
		makerID, dokumenID,
		p.TenantID,
	)
	return err
}

// Update updates workflow state columns for a proposal within tx.
// eir_sesudah maps to EIRBaru; eir_sebelum is never updated (immutable after Create).
func (r *DBAmendmentRepo) Update(ctx context.Context, tx *sql.Tx, p *AmendmentProposal) error {
	var eirBaru interface{}
	if p.EIRBaru != nil {
		eirBaru = p.EIRBaru.StringFixed(8)
	}
	var catchUp interface{}
	if p.CatchUpAdjustment != nil {
		catchUp = p.CatchUpAdjustment.StringFixed(4)
	}

	q := `UPDATE ecl.eir_reestimation_log
		SET workflow_status         = $1,
		    reviewer_id             = $2,
		    approver_id             = $3,
		    reviewer_comment        = $4,
		    approver_comment        = $5,
		    reject_reason           = $6,
		    reviewer_signature_hash = $7,
		    approver_signature_hash = $8,
		    eir_sesudah             = $9::NUMERIC(10,8),
		    catch_up_adjustment     = $10::NUMERIC(20,4),
		    approved_at             = $11,
		    rejected_at             = $12,
		    updated_by              = $13,
		    updated_at              = now(),
		    row_version             = row_version + 1
		WHERE id = $14`

	var reviewerID, approverID interface{}
	if p.ReviewerID != nil {
		reviewerID = *p.ReviewerID
	}
	if p.ApproverID != nil {
		approverID = *p.ApproverID
	}

	_, err := tx.ExecContext(ctx, q,
		string(p.Status),
		reviewerID, approverID,
		p.ReviewerComment, p.ApproverComment,
		p.RejectReason,
		p.ReviewerSignatureHash, p.ApproverSignatureHash,
		eirBaru, catchUp,
		p.ApprovedAt, p.RejectedAt,
		p.UpdatedBy,
		p.ID,
	)
	return err
}

// amendmentCols is the SELECT column list matching scanAmendmentRow.
// Maps DB columns to AmendmentProposal fields.
// M6 additions (migration 000027): cancelled_at, cancel_reason, cancelled_by,
// trigger_source, drift_report_id, document_id, abs_diff.
const amendmentCols = `
	l.id, l.instrumen_id, l.tanggal_re_estimation,
	l.modifikasi_terms_json, l.workflow_status,
	l.eir_sebelum::text, l.eir_sesudah::text, l.catch_up_adjustment::text,
	l.maker_id, l.reviewer_id, l.approver_id,
	l.reviewer_comment, l.approver_comment, l.reject_reason,
	l.reviewer_signature_hash, l.approver_signature_hash,
	l.approved_at, l.rejected_at,
	l.dokumen_pendukung_id,
	l.created_at, l.created_by, l.updated_at, l.updated_by, l.tenant_id, l.row_version,
	l.cancelled_at, l.cancel_reason, l.cancelled_by,
	l.trigger_source, l.drift_report_id, l.document_id`

// GetByID fetches a proposal by ID.
func (r *DBAmendmentRepo) GetByID(ctx context.Context, proposalID uuid.UUID) (*AmendmentProposal, error) {
	q := `SELECT ` + amendmentCols + `
		FROM ecl.eir_reestimation_log l
		WHERE l.id = $1 AND l.deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, q, proposalID)
	p, err := scanAmendmentRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// HasActiveProposal checks for non-terminal proposals for instrumenID.
func (r *DBAmendmentRepo) HasActiveProposal(ctx context.Context, instrumenID uuid.UUID) (bool, error) {
	var count int
	q := `SELECT COUNT(1) FROM ecl.eir_reestimation_log
		WHERE instrumen_id = $1
		  AND workflow_status IN ('DRAFT','PENDING_REVIEW','PENDING_APPROVAL')
		  AND deleted_at IS NULL LIMIT 1`
	if err := r.db.QueryRowContext(ctx, q, instrumenID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByDocumentAndInstrumen returns the most-recent non-terminal DOCUMENT_UPLOAD
// proposal for (documentID, instrumenID).  Used to implement idempotent detect
// when migration 000028 partial unique index fires PG error 23505 (B3 fix).
// Returns (nil, nil) if no such proposal exists.
func (r *DBAmendmentRepo) GetByDocumentAndInstrumen(ctx context.Context, documentID uuid.UUID, instrumenID uuid.UUID) (*AmendmentProposal, error) {
	q := `SELECT ` + amendmentCols + `
		FROM ecl.eir_reestimation_log l
		WHERE l.document_id = $1
		  AND l.instrumen_id = $2
		  AND l.workflow_status NOT IN ('CANCELLED','REJECTED')
		  AND l.deleted_at IS NULL
		ORDER BY l.created_at DESC
		LIMIT 1`
	row := r.db.QueryRowContext(ctx, q, documentID, instrumenID)
	p, err := scanAmendmentRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// List returns paginated proposals ordered by created_at DESC.
func (r *DBAmendmentRepo) List(ctx context.Context, q listquery.Query, cursor string, limit int, actorID uuid.UUID, isAdmin bool) ([]AmendmentProposal, *response.PaginationMeta, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	whereClause, args, orderBy := q.WithAllowed(AllowedColsAmendment).ToSQL("l")
	argIdx := len(args) + 1

	baseWhere := "l.deleted_at IS NULL"
	if !isAdmin {
		baseWhere += fmt.Sprintf(" AND l.maker_id = $%d", argIdx)
		args = append(args, actorID)
		argIdx++
	}
	if cursor != "" {
		if decoded, decErr := decodeCursorStr(cursor); decErr == nil && decoded != "" {
			baseWhere += fmt.Sprintf(" AND l.created_at < $%d::TIMESTAMPTZ", argIdx)
			args = append(args, decoded)
			argIdx++
		}
	}
	_ = argIdx

	fullWhere := baseWhere
	if whereClause != "" {
		fullWhere = baseWhere + " AND " + whereClause
	}
	if orderBy == "" {
		orderBy = "l.created_at DESC"
	}

	//nolint:gosec // fullWhere and orderBy contain only validated column names from AllowedColsAmendment whitelist
	query := fmt.Sprintf(`SELECT `+amendmentCols+`
		FROM ecl.eir_reestimation_log l
		WHERE %s ORDER BY %s LIMIT %d`, fullWhere, orderBy, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []AmendmentProposal
	for rows.Next() {
		p, err := scanAmendmentRow(rows)
		if err != nil {
			return nil, nil, err
		}
		result = append(result, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}

	var nextCursor *string
	if hasMore && len(result) > 0 {
		last := result[len(result)-1]
		c := encodeCursorStr(last.CreatedAt.UTC().Format(time.RFC3339Nano))
		nextCursor = &c
	}

	return result, &response.PaginationMeta{
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Limit:      limit,
	}, nil
}

// scanAmendmentRow scans one ecl.eir_reestimation_log row into AmendmentProposal.
// Column order must match amendmentCols exactly, including M6 additions.
func scanAmendmentRow(s scanner) (*AmendmentProposal, error) {
	var p AmendmentProposal
	var (
		cashflowJSON                                   string
		eirLamaStr                                     string
		eirBaruStr, catchUpStr                         sql.NullString
		reviewerID, approverID, dokumenID              *uuid.UUID
		approvedAt, rejectedAt                         *time.Time
		reviewerComment, approverComment, rejectReason *string
		reviewerSig, approverSig                       *string
		makerIDVal                                     uuid.UUID
		statusStr                                      string
		tanggal                                        time.Time
		// M6 additions
		cancelledAt   *time.Time
		cancelReason  *string
		cancelledBy   *uuid.UUID
		triggerSource sql.NullString
		driftReportID *uuid.UUID
		documentID    *uuid.UUID
	)
	if err := s.Scan(
		&p.ID, &p.InstrumenID, &tanggal,
		&cashflowJSON, &statusStr,
		&eirLamaStr, &eirBaruStr, &catchUpStr,
		&makerIDVal, &reviewerID, &approverID,
		&reviewerComment, &approverComment, &rejectReason,
		&reviewerSig, &approverSig,
		&approvedAt, &rejectedAt,
		&dokumenID,
		&p.CreatedAt, &p.CreatedBy, &p.UpdatedAt, &p.UpdatedBy, &p.TenantID, &p.RowVersion,
		&cancelledAt, &cancelReason, &cancelledBy,
		&triggerSource, &driftReportID, &documentID,
	); err != nil {
		return nil, err
	}

	p.TanggalAmandemen = tanggal
	p.TanggalReEstimasi = tanggal
	p.Status = AmendmentStatus(statusStr)
	p.MakerID = &makerIDVal
	p.ReviewerID = reviewerID
	p.ApproverID = approverID
	p.DokumenPendukungID = dokumenID
	p.ReviewerComment = reviewerComment
	p.ApproverComment = approverComment
	p.RejectReason = rejectReason
	p.ReviewerSignatureHash = reviewerSig
	p.ApproverSignatureHash = approverSig
	p.ApprovedAt = approvedAt
	p.RejectedAt = rejectedAt
	p.RevisedCashflowJSON = cashflowJSON
	// M6
	p.CancelledAt = cancelledAt
	p.CancelReason = cancelReason
	p.CancelledBy = cancelledBy
	p.DriftReportID = driftReportID
	p.DocumentID = documentID
	if triggerSource.Valid {
		p.TriggerSource = AmendTriggerSource(triggerSource.String)
	} else {
		p.TriggerSource = AmendTriggerManual
	}

	var err error
	if p.EIRLama, err = func() (*decimal.Decimal, error) {
		if eirLamaStr == "" {
			return nil, nil
		}
		v, e := decimal.NewFromString(eirLamaStr)
		if e != nil {
			return nil, e
		}
		return &v, nil
	}(); err != nil {
		return nil, fmt.Errorf("eir_sebelum: %w", err)
	}
	if eirBaruStr.Valid && eirBaruStr.String != "" {
		v, e := decimal.NewFromString(eirBaruStr.String)
		if e == nil {
			p.EIRBaru = &v
		}
	}
	if catchUpStr.Valid && catchUpStr.String != "" {
		v, e := decimal.NewFromString(catchUpStr.String)
		if e == nil {
			p.CatchUpAdjustment = &v
		}
	}
	return &p, nil
}

// Cancel transitions a proposal to CANCELLED within tx.
// Sets cancelled_at=now(), cancel_reason, cancelled_by, workflow_status='CANCELLED'.
// caller must verify ownership and state machine rules before calling.
func (r *DBAmendmentRepo) Cancel(ctx context.Context, tx *sql.Tx, proposalID uuid.UUID, cancelReason string, cancelledBy uuid.UUID) error {
	q := `UPDATE ecl.eir_reestimation_log
		SET workflow_status = 'CANCELLED',
		    cancelled_at    = now(),
		    cancel_reason   = $1,
		    cancelled_by    = $2,
		    updated_by      = $2,
		    updated_at      = now(),
		    row_version     = row_version + 1
		WHERE id = $3 AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, cancelReason, cancelledBy, proposalID)
	return err
}

// ListQueue returns non-terminal proposals for the review queue,
// enriched with kode_instrumen from mst.instrumen join.
func (r *DBAmendmentRepo) ListQueue(ctx context.Context, q listquery.Query, cursor string, limit int) ([]QueueRow, *response.PaginationMeta, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	whereClause, args, orderBy := q.WithAllowed(AllowedColsQueue).ToSQL("l")
	argIdx := len(args) + 1

	baseWhere := "l.workflow_status NOT IN ('APPROVED','REJECTED','CANCELLED') AND l.deleted_at IS NULL"

	if cursor != "" {
		if decoded, decErr := decodeCursorStr(cursor); decErr == nil && decoded != "" {
			baseWhere += fmt.Sprintf(" AND l.created_at < $%d::TIMESTAMPTZ", argIdx)
			args = append(args, decoded)
			argIdx++
		}
	}
	_ = argIdx

	fullWhere := baseWhere
	if whereClause != "" {
		fullWhere = baseWhere + " AND " + whereClause
	}
	if orderBy == "" {
		orderBy = "l.created_at DESC"
	}

	//nolint:gosec // fullWhere and orderBy are validated column names from AllowedColsQueue whitelist
	query := fmt.Sprintf(`SELECT
		l.id, l.instrumen_id, i.kode_instrumen,
		l.workflow_status, l.trigger_source,
		l.eir_sebelum::text,
		l.maker_id, l.reviewer_id,
		l.tanggal_re_estimation,
		l.created_at,
		l.drift_report_id,
		l.document_id
	FROM ecl.eir_reestimation_log l
	JOIN mst.instrumen i ON i.id = l.instrumen_id
	WHERE %s ORDER BY %s LIMIT %d`, fullWhere, orderBy, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []QueueRow
	for rows.Next() {
		var qr QueueRow
		var (
			statusStr     string
			triggerSrc    sql.NullString
			eirSebelumStr sql.NullString
			driftID       *uuid.UUID
			docID         *uuid.UUID
			makerID       *uuid.UUID
			reviewerID    *uuid.UUID
		)
		if err := rows.Scan(
			&qr.AmendmentID, &qr.InstrumenID, &qr.KodeInstrumen,
			&statusStr, &triggerSrc,
			&eirSebelumStr,
			&makerID, &reviewerID,
			&qr.TanggalAmandemen,
			&qr.CreatedAt,
			&driftID,
			&docID,
		); err != nil {
			return nil, nil, err
		}
		qr.Status = AmendmentStatus(statusStr)
		qr.MakerID = makerID
		qr.ReviewerID = reviewerID
		qr.DriftReportID = driftID
		qr.DocumentID = docID
		if triggerSrc.Valid {
			qr.TriggerSource = AmendTriggerSource(triggerSrc.String)
		} else {
			qr.TriggerSource = AmendTriggerManual
		}
		if eirSebelumStr.Valid && eirSebelumStr.String != "" {
			v, e := decimal.NewFromString(eirSebelumStr.String)
			if e == nil {
				qr.EIRLama = &v
			}
		}
		result = append(result, qr)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}

	var nextCursor *string
	if hasMore && len(result) > 0 {
		last := result[len(result)-1]
		c := encodeCursorStr(last.CreatedAt.UTC().Format(time.RFC3339Nano))
		nextCursor = &c
	}

	return result, &response.PaginationMeta{
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Limit:      limit,
	}, nil
}

// ─── DriftReportRepoIface ─────────────────────────────────────────────────────

// DriftReportRepoIface defines CRUD for sys.drift_report.
type DriftReportRepoIface interface {
	// Create inserts a new drift report row (status=IN_PROGRESS) within tx.
	Create(ctx context.Context, tx *sql.Tx, report *DriftReport) error

	// Update updates status, counts, and completion timestamp within tx.
	Update(ctx context.Context, tx *sql.Tx, report *DriftReport) error

	// GetByID fetches a drift report by ID.
	GetByID(ctx context.Context, reportID uuid.UUID) (*DriftReport, error)

	// GetInProgressReport returns the first IN_PROGRESS report, or nil if none.
	// Used to guard against concurrent drift generation.
	GetInProgressReport(ctx context.Context) (*DriftReport, error)

	// List returns paginated drift reports for the list endpoint.
	List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]DriftReport, *response.PaginationMeta, error)

	// LoadThresholds reads drift_flag_threshold and drift_high_threshold from sys.parameter.
	// Returns (flagThreshold, highThreshold, error).
	LoadThresholds(ctx context.Context) (decimal.Decimal, decimal.Decimal, error)
}

// DBDriftReportRepo implements DriftReportRepoIface against sys.drift_report.
type DBDriftReportRepo struct {
	db *sql.DB
}

// NewDBDriftReportRepo creates a DBDriftReportRepo.
func NewDBDriftReportRepo(db *sql.DB) *DBDriftReportRepo {
	return &DBDriftReportRepo{db: db}
}

const driftReportCols = `
	r.id, r.tanggal_run, r.trigger_source, r.triggered_by,
	r.asynq_job_id, r.status,
	r.started_at, r.completed_at,
	r.total_instrumen, r.drift_low_count, r.drift_high_count,
	r.missing_schedule_count, r.error_count, r.error_summary,
	r.drift_flag_threshold::text, r.drift_high_threshold::text,
	r.created_at, r.created_by, r.updated_at, r.updated_by, r.tenant_id, r.row_version`

func (r *DBDriftReportRepo) Create(ctx context.Context, tx *sql.Tx, report *DriftReport) error {
	q := `INSERT INTO sys.drift_report
		(id, tanggal_run, trigger_source, triggered_by,
		 asynq_job_id, status, started_at,
		 drift_flag_threshold, drift_high_threshold,
		 total_instrumen, drift_low_count, drift_high_count,
		 missing_schedule_count, error_count,
		 created_by, updated_by, tenant_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,
		        $8::NUMERIC(10,8),$9::NUMERIC(10,8),
		        0,0,0,0,0,$10,$10,$11)`

	var triggeredBy interface{}
	if report.TriggeredBy != nil {
		triggeredBy = *report.TriggeredBy
	}
	_, err := tx.ExecContext(ctx, q,
		report.ID, report.TanggalRun,
		string(report.TriggerSource), triggeredBy,
		report.AsynqJobID, string(report.Status), report.StartedAt,
		report.DriftFlagThreshold.StringFixed(8),
		report.DriftHighThreshold.StringFixed(8),
		report.CreatedBy, report.TenantID,
	)
	return err
}

func (r *DBDriftReportRepo) Update(ctx context.Context, tx *sql.Tx, report *DriftReport) error {
	q := `UPDATE sys.drift_report
		SET status                = $1,
		    completed_at          = $2,
		    total_instrumen        = $3,
		    drift_low_count        = $4,
		    drift_high_count       = $5,
		    missing_schedule_count = $6,
		    error_count            = $7,
		    error_summary          = $8,
		    updated_by             = $9,
		    updated_at             = now(),
		    row_version            = row_version + 1
		WHERE id = $10`
	_, err := tx.ExecContext(ctx, q,
		string(report.Status), report.CompletedAt,
		report.TotalInstrumen, report.DriftLowCount, report.DriftHighCount,
		report.MissingScheduleCount, report.ErrorCount, report.ErrorSummary,
		report.UpdatedBy, report.ID,
	)
	return err
}

func (r *DBDriftReportRepo) GetByID(ctx context.Context, reportID uuid.UUID) (*DriftReport, error) {
	q := `SELECT ` + driftReportCols + ` FROM sys.drift_report r WHERE r.id = $1`
	row := r.db.QueryRowContext(ctx, q, reportID)
	dr, err := scanDriftReport(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return dr, err
}

func (r *DBDriftReportRepo) GetInProgressReport(ctx context.Context) (*DriftReport, error) {
	q := `SELECT ` + driftReportCols + ` FROM sys.drift_report r WHERE r.status = 'IN_PROGRESS' LIMIT 1`
	row := r.db.QueryRowContext(ctx, q)
	dr, err := scanDriftReport(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return dr, err
}

func (r *DBDriftReportRepo) List(ctx context.Context, q listquery.Query, cursor string, limit int) ([]DriftReport, *response.PaginationMeta, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	whereClause, args, orderBy := q.WithAllowed(AllowedColsDriftReport).ToSQL("r")
	argIdx := len(args) + 1

	baseWhere := "1=1"
	if cursor != "" {
		if decoded, decErr := decodeCursorStr(cursor); decErr == nil && decoded != "" {
			baseWhere += fmt.Sprintf(" AND r.created_at < $%d::TIMESTAMPTZ", argIdx)
			args = append(args, decoded)
			argIdx++
		}
	}
	_ = argIdx

	fullWhere := baseWhere
	if whereClause != "" {
		fullWhere = baseWhere + " AND " + whereClause
	}
	if orderBy == "" {
		orderBy = "r.tanggal_run DESC, r.created_at DESC"
	}

	//nolint:gosec // fullWhere validated via AllowedColsDriftReport whitelist
	query := fmt.Sprintf(`SELECT `+driftReportCols+`
		FROM sys.drift_report r
		WHERE %s ORDER BY %s LIMIT %d`, fullWhere, orderBy, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close() //nolint:errcheck

	var result []DriftReport
	for rows.Next() {
		dr, err := scanDriftReport(rows)
		if err != nil {
			return nil, nil, err
		}
		result = append(result, *dr)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	hasMore := len(result) > limit
	if hasMore {
		result = result[:limit]
	}

	var nextCursor *string
	if hasMore && len(result) > 0 {
		last := result[len(result)-1]
		c := encodeCursorStr(last.CreatedAt.UTC().Format(time.RFC3339Nano))
		nextCursor = &c
	}

	return result, &response.PaginationMeta{
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Limit:      limit,
	}, nil
}

// LoadThresholds reads drift thresholds from sys.parameter.
// Keys: 'drift_low_threshold', 'drift_high_threshold'.
// Both must exist and be valid decimals, else returns ErrEIRDriftThresholdInvalid.
func (r *DBDriftReportRepo) LoadThresholds(ctx context.Context) (decimal.Decimal, decimal.Decimal, error) {
	q := `SELECT key, value FROM sys.parameter
		WHERE key IN ('drift_low_threshold','drift_high_threshold')
		  AND effective_to IS NULL`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return decimal.Zero, decimal.Zero, err
	}
	defer rows.Close() //nolint:errcheck

	params := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return decimal.Zero, decimal.Zero, err
		}
		params[k] = v
	}

	flagStr, ok1 := params["drift_low_threshold"]
	highStr, ok2 := params["drift_high_threshold"]
	if !ok1 || !ok2 {
		return decimal.Zero, decimal.Zero,
			ErrEIRDriftThresholdInvalid("sys.parameter keys drift_low_threshold or drift_high_threshold missing")
	}

	flag, err := decimal.NewFromString(flagStr)
	if err != nil || flag.IsZero() || flag.IsNegative() {
		return decimal.Zero, decimal.Zero,
			ErrEIRDriftThresholdInvalid("drift_low_threshold invalid: " + flagStr)
	}

	high, err := decimal.NewFromString(highStr)
	if err != nil || high.IsZero() || high.IsNegative() || high.LessThanOrEqual(flag) {
		return decimal.Zero, decimal.Zero,
			ErrEIRDriftThresholdInvalid("drift_high_threshold invalid or <= low_threshold: " + highStr)
	}

	return flag, high, nil
}

// scanDriftReport scans one sys.drift_report row.
// NUMERIC columns read via ::text (DEC-016).
func scanDriftReport(s scanner) (*DriftReport, error) {
	var dr DriftReport
	var (
		triggerSrc                   string
		asynqJobID                   *string
		startedAt, completedAt       *time.Time
		triggeredBy                  *uuid.UUID
		errorSummary                 *string
		flagThreshStr, highThreshStr string
	)
	if err := s.Scan(
		&dr.ID, &dr.TanggalRun, &triggerSrc, &triggeredBy,
		&asynqJobID, &dr.Status,
		&startedAt, &completedAt,
		&dr.TotalInstrumen, &dr.DriftLowCount, &dr.DriftHighCount,
		&dr.MissingScheduleCount, &dr.ErrorCount, &errorSummary,
		&flagThreshStr, &highThreshStr,
		&dr.CreatedAt, &dr.CreatedBy, &dr.UpdatedAt, &dr.UpdatedBy, &dr.TenantID, &dr.RowVersion,
	); err != nil {
		return nil, err
	}
	dr.TriggerSource = DriftTriggerSource(triggerSrc)
	dr.TriggeredBy = triggeredBy
	dr.AsynqJobID = asynqJobID
	dr.StartedAt = startedAt
	dr.CompletedAt = completedAt
	dr.ErrorSummary = errorSummary

	var err error
	if dr.DriftFlagThreshold, err = decimal.NewFromString(flagThreshStr); err != nil {
		return nil, fmt.Errorf("drift_flag_threshold: %w", err)
	}
	if dr.DriftHighThreshold, err = decimal.NewFromString(highThreshStr); err != nil {
		return nil, fmt.Errorf("drift_high_threshold: %w", err)
	}
	return &dr, nil
}
