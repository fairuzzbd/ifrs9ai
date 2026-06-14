package penempatan

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// ─── Repo ─────────────────────────────────────────────────────────────────────

// Repo wraps database access for trx.penempatan_deposito and sys.settlement_account_balance.
// All SQL uses parameterized queries — never string concat.
// No SQL in handlers or service: only in repo.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a new Repo.
func NewRepo(db *sql.DB) *Repo {
	if db == nil {
		panic("penempatan.NewRepo: db must not be nil")
	}
	return &Repo{db: db}
}

// AllowedSortCols is the sort/filter whitelist for the list endpoint (DataTable).
var AllowedSortCols = []string{
	"kode_transaksi",
	"tanggal_penempatan",
	"nominal_idr",
	"workflow_status",
	"kupon_persen",
	"tanggal_jatuh_tempo",
	"created_at",
}

// ─── Create ──────────────────────────────────────────────────────────────────

// Create inserts a new penempatan_deposito row (DRAFT) within the given transaction.
// kode_transaksi must already be computed by the service layer.
func (r *Repo) Create(ctx context.Context, tx *sql.Tx, p *Penempatan) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO trx.penempatan_deposito (
			id, kode_transaksi,
			instrumen_id, counterparty_bank_id, periode_id, mata_uang_id,
			tanggal_penempatan, tanggal_jatuh_tempo,
			nominal_idr, nominal_fcy, kurs_penempatan,
			tenor_bulan, kupon_persen, biaya_transaksi_idr,
			nomor_referensi_bank, settlement_account, catatan,
			kontrak_doc_id,
			workflow_status, maker_id,
			created_by, updated_by, tenant_id
		) VALUES (
			$1, $2,
			$3, $4, $5, $6,
			$7, $8,
			$9, $10, $11,
			$12, $13, $14,
			$15, $16, $17,
			$18,
			'DRAFT'::trx.penempatan_workflow_status, $19,
			$20, $21, $22
		)`,
		p.ID, p.KodeTransaksi,
		p.InstrumenID, p.CounterpartyBankID, p.PeriodeID, p.MataUangID,
		p.TanggalPenempatan, p.TanggalJatuhTempo,
		p.NominalIDR, p.NominalFCY, p.KursPenempatan,
		p.TenorBulan, p.KuponPersen, p.BiayaTransaksiIDR,
		p.NomorReferensiBank, p.SettlementAccount, p.Catatan,
		p.KontrakDocID,
		p.MakerID,
		p.CreatedBy, p.UpdatedBy, p.TenantID,
	)
	if err != nil {
		return fmt.Errorf("penempatan.Repo.Create: %w", err)
	}
	return nil
}

// NextKodeSeq reads the next value from trx.penempatan_kode_seq and formats kode_transaksi.
// Format: PNP-{YYYYMM}-{seq:06d} (service layer assembles from month of tanggalPenempatan).
func (r *Repo) NextKodeSeq(ctx context.Context, tx *sql.Tx, bulan time.Time) (string, error) {
	var seq int64
	err := tx.QueryRowContext(ctx, `SELECT nextval('trx.penempatan_kode_seq')`).Scan(&seq)
	if err != nil {
		return "", fmt.Errorf("penempatan.Repo.NextKodeSeq: %w", err)
	}
	return fmt.Sprintf("PNP-%s-%06d", bulan.Format("200601"), seq), nil
}

// ─── Get ──────────────────────────────────────────────────────────────────────

// Get reads one penempatan by ID (includes joined fields from instrumen + counterparty).
// Does NOT filter deleted_at — caller decides if deleted records are allowed.
func (r *Repo) Get(ctx context.Context, id uuid.UUID, tenantID string) (*Penempatan, error) {
	p := &Penempatan{}
	var (
		nominalFCY           *string
		kursPenempatan       *string
		eirAwal              *string
		carryingAmountAwal   *string
		realizedGL           *string
		reviewerID           *string
		approverID           *string
		reviewerSignedAt     *time.Time
		approverSignedAt     *time.Time
		reviewerSigHash      []byte
		approverSigHash      []byte
		kontrakDocID         *string
		dokTerminasiID       *string
		terminateMakerID     *string
		terminateRevID       *string
		terminateApprID      *string
		terminateRevSignAt   *time.Time
		terminateApprSignAt  *time.Time
		terminateRevSigHash  []byte
		terminateApprSigHash []byte
		terminateReqReason   *string
		terminateRevComment  *string
		terminateApprComment *string
		terminateRejReason   *string
		terminatedAt         *time.Time
		maturedAt            *time.Time
		rejectReason         *string
		commentReview        *string
		commentApprove       *string
		nomorRefBank         *string
		settlementAccount    *string
		catatan              *string
		deletedAt            *time.Time
		deletedBy            *string
	)

	err := r.db.QueryRowContext(ctx, `
		SELECT
			p.id, p.kode_transaksi,
			p.instrumen_id, p.counterparty_bank_id, p.periode_id, p.mata_uang_id,
			p.tanggal_penempatan, p.tanggal_jatuh_tempo,
			p.nominal_idr::text, p.nominal_fcy::text, p.kurs_penempatan::text,
			p.tenor_bulan, p.kupon_persen::text, p.biaya_transaksi_idr::text,
			p.nomor_referensi_bank, p.settlement_account, p.catatan,
			p.eir_awal::text, p.carrying_amount_awal::text,
			p.kontrak_doc_id::text, p.dokumen_terminasi_id::text,
			p.workflow_status::text,
			p.maker_id,
			p.reviewer_id::text, p.approver_id::text,
			p.reviewer_signed_at, p.approver_signed_at,
			p.reviewer_signature_hash, p.approver_signature_hash,
			p.reject_reason, p.comment_review, p.comment_approve,
			p.terminate_maker_id::text, p.terminate_reviewer_id::text, p.terminate_approver_id::text,
			p.terminate_reviewer_signed_at, p.terminate_approver_signed_at,
			p.terminate_reviewer_signature_hash, p.terminate_approver_signature_hash,
			p.terminate_request_reason, p.terminate_review_comment,
			p.terminate_approve_comment, p.terminate_reject_reason,
			p.terminated_at, p.matured_at, p.realized_gain_loss_idr::text,
			p.created_at, p.created_by, p.updated_at, p.updated_by,
			p.deleted_at, p.deleted_by::text,
			p.row_version, p.tenant_id,
			COALESCE(i.nama, ''), COALESCE(i.klasifikasi_psak71::text, ''),
			COALESCE(i.tipe_instrumen::text, ''),
			COALESCE(cp.nama, '')
		FROM trx.penempatan_deposito p
		LEFT JOIN mst.instrumen i ON i.id = p.instrumen_id
		LEFT JOIN mst.counterparty cp ON cp.id = p.counterparty_bank_id
		WHERE p.id = $1 AND p.tenant_id = $2`,
		id, tenantID,
	).Scan(
		&p.ID, &p.KodeTransaksi,
		&p.InstrumenID, &p.CounterpartyBankID, &p.PeriodeID, &p.MataUangID,
		&p.TanggalPenempatan, &p.TanggalJatuhTempo,
		new(string), &nominalFCY, &kursPenempatan,
		&p.TenorBulan, new(string), new(string),
		&nomorRefBank, &settlementAccount, &catatan,
		&eirAwal, &carryingAmountAwal,
		&kontrakDocID, &dokTerminasiID,
		new(string),
		&p.MakerID,
		&reviewerID, &approverID,
		&reviewerSignedAt, &approverSignedAt,
		&reviewerSigHash, &approverSigHash,
		&rejectReason, &commentReview, &commentApprove,
		&terminateMakerID, &terminateRevID, &terminateApprID,
		&terminateRevSignAt, &terminateApprSignAt,
		&terminateRevSigHash, &terminateApprSigHash,
		&terminateReqReason, &terminateRevComment,
		&terminateApprComment, &terminateRejReason,
		&terminatedAt, &maturedAt, &realizedGL,
		&p.CreatedAt, &p.CreatedBy, &p.UpdatedAt, &p.UpdatedBy,
		&deletedAt, &deletedBy,
		&p.RowVersion, &p.TenantID,
		&p.NamaInstrumen, &p.KlasifikasiPSAK71, &p.TipeInstrumen,
		&p.NamaCounterparty,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.Get: %w", err)
	}

	// Re-scan numeric fields with proper types.
	if scanErr := r.db.QueryRowContext(ctx, `
		SELECT
			p.nominal_idr, p.nominal_fcy, p.kurs_penempatan,
			p.kupon_persen, p.biaya_transaksi_idr,
			p.eir_awal, p.carrying_amount_awal, p.realized_gain_loss_idr,
			p.workflow_status::text
		FROM trx.penempatan_deposito p WHERE p.id = $1`, id).Scan(
		&p.NominalIDR, &p.NominalFCY, &p.KursPenempatan,
		&p.KuponPersen, &p.BiayaTransaksiIDR,
		&p.EIRAwal, &p.CarryingAmountAwal, &p.RealizedGainLossIDR,
		(*string)(nil),
	); scanErr != nil && scanErr != sql.ErrNoRows {
		return nil, fmt.Errorf("penempatan.Repo.Get: rescan numeric: %w", scanErr)
	}

	// Re-read status properly.
	var statusStr string
	if scanErr := r.db.QueryRowContext(ctx, `SELECT workflow_status::text FROM trx.penempatan_deposito WHERE id=$1`, id).Scan(&statusStr); scanErr != nil && scanErr != sql.ErrNoRows {
		return nil, fmt.Errorf("penempatan.Repo.Get: rescan status: %w", scanErr)
	}
	p.WorkflowStatus = Status(statusStr)

	parseOptUUID := func(s *string) *uuid.UUID {
		if s == nil {
			return nil
		}
		u, err := uuid.Parse(*s)
		if err != nil {
			return nil
		}
		return &u
	}

	p.ReviewerID = parseOptUUID(reviewerID)
	p.ApproverID = parseOptUUID(approverID)
	p.ReviewerSignedAt = reviewerSignedAt
	p.ApproverSignedAt = approverSignedAt
	p.ReviewerSignatureHash = reviewerSigHash
	p.ApproverSignatureHash = approverSigHash
	p.RejectReason = rejectReason
	p.CommentReview = commentReview
	p.CommentApprove = commentApprove
	p.NomorReferensiBank = nomorRefBank
	p.SettlementAccount = settlementAccount
	p.Catatan = catatan
	p.DeletedAt = deletedAt
	p.TerminatedAt = terminatedAt
	p.MaturedAt = maturedAt
	p.TerminateReqReason(terminateReqReason)
	p.TerminateReviewerSignedAt = terminateRevSignAt
	p.TerminateApproverSignedAt = terminateApprSignAt
	p.TerminateReviewerSignatureHash = terminateRevSigHash
	p.TerminateApproverSignatureHash = terminateApprSigHash
	p.TerminateReviewComment = terminateRevComment
	p.TerminateApproveComment = terminateApprComment
	p.TerminateRejectReason = terminateRejReason
	p.TerminateMakerID = parseOptUUID(terminateMakerID)
	p.TerminateReviewerID = parseOptUUID(terminateRevID)
	p.TerminateApproverID = parseOptUUID(terminateApprID)
	if deletedBy != nil {
		u, err := uuid.Parse(*deletedBy)
		if err == nil {
			p.DeletedBy = &u
		}
	}
	if kontrakDocID != nil {
		u, err := uuid.Parse(*kontrakDocID)
		if err == nil {
			p.KontrakDocID = &u
		}
	}
	if dokTerminasiID != nil {
		u, err := uuid.Parse(*dokTerminasiID)
		if err == nil {
			p.DokumenTerminasiID = &u
		}
	}

	return p, nil
}

// GetForUpdate reads a penempatan row with FOR UPDATE lock within a transaction.
func (r *Repo) GetForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID, tenantID string) (*Penempatan, error) {
	p := &Penempatan{}
	var (
		reviewerID         *string
		approverID         *string
		terminateMakerID   *string
		terminateRevID     *string
		terminateApprID    *string
		rejectReason       *string
		terminateReqReason *string
		deletedAt          *time.Time
		deletedBy          *string
		statusStr          string
	)

	err := tx.QueryRowContext(ctx, `
		SELECT
			p.id, p.kode_transaksi,
			p.instrumen_id, p.counterparty_bank_id, p.periode_id, p.mata_uang_id,
			p.tanggal_penempatan, p.tanggal_jatuh_tempo,
			p.nominal_idr, p.nominal_fcy, p.kurs_penempatan,
			p.tenor_bulan, p.kupon_persen, p.biaya_transaksi_idr,
			p.eir_awal, p.carrying_amount_awal,
			p.workflow_status::text,
			p.maker_id,
			p.reviewer_id::text, p.approver_id::text,
			p.terminate_maker_id::text, p.terminate_reviewer_id::text, p.terminate_approver_id::text,
			p.reject_reason,
			p.terminate_request_reason,
			p.deleted_at, p.deleted_by::text,
			p.row_version, p.tenant_id
		FROM trx.penempatan_deposito p
		WHERE p.id = $1 AND p.tenant_id = $2
		FOR UPDATE`,
		id, tenantID,
	).Scan(
		&p.ID, &p.KodeTransaksi,
		&p.InstrumenID, &p.CounterpartyBankID, &p.PeriodeID, &p.MataUangID,
		&p.TanggalPenempatan, &p.TanggalJatuhTempo,
		&p.NominalIDR, &p.NominalFCY, &p.KursPenempatan,
		&p.TenorBulan, &p.KuponPersen, &p.BiayaTransaksiIDR,
		&p.EIRAwal, &p.CarryingAmountAwal,
		&statusStr,
		&p.MakerID,
		&reviewerID, &approverID,
		&terminateMakerID, &terminateRevID, &terminateApprID,
		&rejectReason,
		&terminateReqReason,
		&deletedAt, &deletedBy,
		&p.RowVersion, &p.TenantID,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.GetForUpdate: %w", err)
	}

	p.WorkflowStatus = Status(statusStr)
	p.RejectReason = rejectReason
	p.DeletedAt = deletedAt
	p.TerminateRequestReason = terminateReqReason

	parseOptUUID := func(s *string) *uuid.UUID {
		if s == nil {
			return nil
		}
		u, err := uuid.Parse(*s)
		if err != nil {
			return nil
		}
		return &u
	}
	p.ReviewerID = parseOptUUID(reviewerID)
	p.ApproverID = parseOptUUID(approverID)
	p.TerminateMakerID = parseOptUUID(terminateMakerID)
	p.TerminateReviewerID = parseOptUUID(terminateRevID)
	p.TerminateApproverID = parseOptUUID(terminateApprID)
	if deletedBy != nil {
		u, err := uuid.Parse(*deletedBy)
		if err == nil {
			p.DeletedBy = &u
		}
	}

	return p, nil
}

// ─── Update (DRAFT fields only) ──────────────────────────────────────────────

// UpdateDraft updates editable fields on a DRAFT penempatan within a transaction.
// row_version optimistic lock enforced: fails with 0 rows updated if mismatch.
func (r *Repo) UpdateDraft(ctx context.Context, tx *sql.Tx, id uuid.UUID, req UpdateRequest,
	tanggalJatuhTempo time.Time, updatedBy uuid.UUID, tenantID string) (int64, error) {

	setClauses := []string{
		"tanggal_jatuh_tempo = $1",
		"updated_by = $2",
	}
	args := []any{tanggalJatuhTempo, updatedBy}
	argIdx := 3

	if req.TanggalPenempatan != nil {
		t, parseErr := time.Parse("2006-01-02", *req.TanggalPenempatan)
		if parseErr != nil {
			return 0, fmt.Errorf("penempatan.Repo.UpdateDraft: parse tanggal_penempatan: %w", parseErr)
		}
		setClauses = append(setClauses, fmt.Sprintf("tanggal_penempatan = $%d", argIdx))
		args = append(args, t)
		argIdx++
	}
	if req.NominalIDR != nil {
		setClauses = append(setClauses, fmt.Sprintf("nominal_idr = $%d", argIdx))
		args = append(args, *req.NominalIDR)
		argIdx++
	}
	if req.NominalFCY != nil {
		setClauses = append(setClauses, fmt.Sprintf("nominal_fcy = $%d", argIdx))
		args = append(args, *req.NominalFCY)
		argIdx++
	}
	if req.KuponPersen != nil {
		setClauses = append(setClauses, fmt.Sprintf("kupon_persen = $%d", argIdx))
		args = append(args, *req.KuponPersen)
		argIdx++
	}
	if req.TenorBulan != nil {
		setClauses = append(setClauses, fmt.Sprintf("tenor_bulan = $%d", argIdx))
		args = append(args, *req.TenorBulan)
		argIdx++
	}
	if req.BiayaTransaksiIDR != nil {
		setClauses = append(setClauses, fmt.Sprintf("biaya_transaksi_idr = $%d", argIdx))
		args = append(args, *req.BiayaTransaksiIDR)
		argIdx++
	}
	if req.NomorReferensiBank != nil {
		setClauses = append(setClauses, fmt.Sprintf("nomor_referensi_bank = $%d", argIdx))
		args = append(args, *req.NomorReferensiBank)
		argIdx++
	}
	if req.SettlementAccount != nil {
		setClauses = append(setClauses, fmt.Sprintf("settlement_account = $%d", argIdx))
		args = append(args, *req.SettlementAccount)
		argIdx++
	}
	if req.Catatan != nil {
		setClauses = append(setClauses, fmt.Sprintf("catatan = $%d", argIdx))
		args = append(args, *req.Catatan)
		argIdx++
	}
	if req.KontrakDocID != nil {
		setClauses = append(setClauses, fmt.Sprintf("kontrak_doc_id = $%d", argIdx))
		args = append(args, *req.KontrakDocID)
		argIdx++
	}

	// WHERE clause params
	args = append(args, id, req.RowVersion, tenantID)
	idxID := argIdx
	idxVer := argIdx + 1
	idxTen := argIdx + 2

	query := fmt.Sprintf( //nolint:gosec // set-clause cols come from hardcoded setClauses slice (validated allowlist), not user input
		`UPDATE trx.penempatan_deposito SET %s WHERE id = $%d AND row_version = $%d AND tenant_id = $%d AND workflow_status = 'DRAFT'::trx.penempatan_workflow_status`,
		strings.Join(setClauses, ", "), idxID, idxVer, idxTen,
	)

	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("penempatan.Repo.UpdateDraft: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("penempatan.Repo.UpdateDraft: rows affected: %w", err)
	}
	return rows, nil
}

// ─── UpdateStatus (workflow transitions) ─────────────────────────────────────

// StatusUpdate holds fields for a workflow status transition.
type StatusUpdate struct {
	NewStatus             Status
	ReviewerID            *uuid.UUID
	ReviewerSignedAt      *time.Time
	ReviewerSignatureHash []byte
	ApproverID            *uuid.UUID
	ApproverSignedAt      *time.Time
	ApproverSignatureHash []byte
	RejectReason          *string
	CommentReview         *string
	CommentApprove        *string
	// Terminate workflow fields
	TerminateMakerID               *uuid.UUID
	TerminateReviewerID            *uuid.UUID
	TerminateApproverID            *uuid.UUID
	TerminateReviewerSignedAt      *time.Time
	TerminateApproverSignedAt      *time.Time
	TerminateReviewerSignatureHash []byte
	TerminateApproverSignatureHash []byte
	TerminateRequestReason         *string
	TerminateReviewComment         *string
	TerminateApproveComment        *string
	TerminateRejectReason          *string
	TerminatedAt                   *time.Time
	MaturedAt                      *time.Time
	DeletedAt                      *time.Time
	DeletedBy                      *uuid.UUID
	UpdatedBy                      uuid.UUID
}

// UpdateStatus applies a workflow status transition within a transaction.
func (r *Repo) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, upd StatusUpdate, tenantID string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE trx.penempatan_deposito SET
			workflow_status = $1::trx.penempatan_workflow_status,
			reviewer_id = COALESCE($2, reviewer_id),
			reviewer_signed_at = COALESCE($3, reviewer_signed_at),
			reviewer_signature_hash = COALESCE($4, reviewer_signature_hash),
			approver_id = COALESCE($5, approver_id),
			approver_signed_at = COALESCE($6, approver_signed_at),
			approver_signature_hash = COALESCE($7, approver_signature_hash),
			reject_reason = COALESCE($8, reject_reason),
			comment_review = COALESCE($9, comment_review),
			comment_approve = COALESCE($10, comment_approve),
			terminate_maker_id = COALESCE($11, terminate_maker_id),
			terminate_reviewer_id = $12,
			terminate_approver_id = COALESCE($13, terminate_approver_id),
			terminate_reviewer_signed_at = COALESCE($14, terminate_reviewer_signed_at),
			terminate_approver_signed_at = COALESCE($15, terminate_approver_signed_at),
			terminate_reviewer_signature_hash = COALESCE($16, terminate_reviewer_signature_hash),
			terminate_approver_signature_hash = COALESCE($17, terminate_approver_signature_hash),
			terminate_request_reason = COALESCE($18, terminate_request_reason),
			terminate_review_comment = COALESCE($19, terminate_review_comment),
			terminate_approve_comment = COALESCE($20, terminate_approve_comment),
			terminate_reject_reason = COALESCE($21, terminate_reject_reason),
			terminated_at = COALESCE($22, terminated_at),
			matured_at = COALESCE($23, matured_at),
			deleted_at = COALESCE($24, deleted_at),
			deleted_by = COALESCE($25, deleted_by),
			updated_by = $26
		WHERE id = $27 AND tenant_id = $28`,
		string(upd.NewStatus),
		upd.ReviewerID, upd.ReviewerSignedAt, upd.ReviewerSignatureHash,
		upd.ApproverID, upd.ApproverSignedAt, upd.ApproverSignatureHash,
		upd.RejectReason, upd.CommentReview, upd.CommentApprove,
		upd.TerminateMakerID, upd.TerminateReviewerID,
		upd.TerminateApproverID,
		upd.TerminateReviewerSignedAt, upd.TerminateApproverSignedAt,
		upd.TerminateReviewerSignatureHash, upd.TerminateApproverSignatureHash,
		upd.TerminateRequestReason, upd.TerminateReviewComment,
		upd.TerminateApproveComment, upd.TerminateRejectReason,
		upd.TerminatedAt, upd.MaturedAt,
		upd.DeletedAt, upd.DeletedBy,
		upd.UpdatedBy,
		id, tenantID,
	)
	if err != nil {
		return fmt.Errorf("penempatan.Repo.UpdateStatus: %w", err)
	}
	return nil
}

// ResetReviewer resets reviewer_id and optionally approver_id to NULL (used in reject transitions).
func (r *Repo) ResetReviewer(ctx context.Context, tx *sql.Tx, id uuid.UUID, resetApprover bool, tenantID string) error {
	if resetApprover {
		_, err := tx.ExecContext(ctx,
			`UPDATE trx.penempatan_deposito SET reviewer_id = NULL, approver_id = NULL WHERE id = $1 AND tenant_id = $2`,
			id, tenantID)
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE trx.penempatan_deposito SET reviewer_id = NULL WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return err
}

// ResetTerminateReviewer resets terminate_reviewer_id to NULL (used in terminate reject).
func (r *Repo) ResetTerminateReviewer(ctx context.Context, tx *sql.Tx, id uuid.UUID, tenantID string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE trx.penempatan_deposito SET terminate_reviewer_id = NULL WHERE id = $1 AND tenant_id = $2`,
		id, tenantID)
	return err
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult holds list + cursor metadata.
type ListResult struct {
	Items      []ListItem
	NextCursor *string
	HasMore    bool
	TotalEst   int64
}

// List queries penempatan_deposito with DataTable pattern (sort + filter + cursor).
func (r *Repo) List(ctx context.Context, q listquery.Query, includeDeleted bool, tenantID string) (ListResult, error) {
	allowed := AllowedSortCols
	lq := q.WithAllowed(allowed)

	where, args, orderBy := lq.ToSQL("p")

	var conditions []string
	if !includeDeleted {
		conditions = append(conditions, "p.deleted_at IS NULL")
	}
	conditions = append(conditions, fmt.Sprintf("p.tenant_id = $%d", len(args)+1))
	args = append(args, tenantID)

	if where != "" {
		conditions = append([]string{where}, conditions...)
	}
	whereClause := strings.Join(conditions, " AND ")
	if whereClause != "" {
		whereClause = "WHERE " + whereClause
	}

	if orderBy == "" {
		orderBy = "p.created_at DESC"
	}

	// limit+1 trick for hasMore detection (avoid COUNT(*))
	limit := 50
	// Note: cursor-based pagination — simplified implementation without offset
	// Full implementation would decode cursor to get last_id + last_value for keyset pagination.
	query := fmt.Sprintf( //nolint:gosec // whereClause built from validated allowlist cols; orderBy from same whitelist; limit is int literal
		`SELECT
			p.id, p.kode_transaksi, p.workflow_status::text,
			p.nominal_idr, p.tanggal_penempatan, p.tanggal_jatuh_tempo,
			p.kupon_persen, p.tenor_bulan,
			p.maker_id, p.created_at, p.deleted_at,
			COALESCE(i.nama, ''), COALESCE(i.klasifikasi_psak71::text, ''),
			COALESCE(i.tipe_instrumen::text, ''),
			COALESCE(cp.nama, '')
		FROM trx.penempatan_deposito p
		LEFT JOIN mst.instrumen i ON i.id = p.instrumen_id
		LEFT JOIN mst.counterparty cp ON cp.id = p.counterparty_bank_id
		%s
		ORDER BY %s
		LIMIT %d`, whereClause, orderBy, limit+1)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ListResult{}, fmt.Errorf("penempatan.Repo.List: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is non-actionable on read queries

	var items []ListItem
	for rows.Next() {
		var item ListItem
		var statusStr string
		var deletedAt *time.Time
		if err := rows.Scan(
			&item.ID, &item.KodeTransaksi, &statusStr,
			&item.NominalIDR, &item.TanggalPenempatan, &item.TanggalJatuhTempo,
			&item.KuponPersen, &item.TenorBulan,
			&item.MakerID, &item.CreatedAt, &deletedAt,
			&item.NamaInstrumen, &item.KlasifikasiPSAK71, &item.TipeInstrumen,
			&item.NamaCounterparty,
		); err != nil {
			return ListResult{}, fmt.Errorf("penempatan.Repo.List scan: %w", err)
		}
		item.WorkflowStatus = Status(statusStr)
		item.DeletedAt = deletedAt
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResult{}, fmt.Errorf("penempatan.Repo.List rows: %w", err)
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	var nextCursor *string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		cursorData, cursorErr := json.Marshal(map[string]any{
			"id":         last.ID.String(),
			"created_at": last.CreatedAt.Unix(),
		})
		if cursorErr != nil {
			return ListResult{}, fmt.Errorf("penempatan.Repo.List: cursor marshal: %w", cursorErr)
		}
		encoded := base64.StdEncoding.EncodeToString(cursorData)
		nextCursor = &encoded
	}

	// Estimate total via fast count (without full scan for large tables)
	var totalEst int64
	// Best-effort count estimate; ignore error (non-critical, returns 0 on failure).
	if countErr := r.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM trx.penempatan_deposito p %s`, whereClause), args...).Scan(&totalEst); countErr != nil { //nolint:gosec // whereClause from validated allowlist
		totalEst = 0
	}

	return ListResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		TotalEst:   totalEst,
	}, nil
}

// ─── Maturity scan ───────────────────────────────────────────────────────────

// GetMaturingInstruments returns APPROVED_ACTIVE penempatan with tanggal_jatuh_tempo ≤ asOfDate.
// Used by the Asynq maturity-checker cron (uses idx_penempatan_jatuh_tempo_active).
func (r *Repo) GetMaturingInstruments(ctx context.Context, asOfDate time.Time, tenantID string) ([]Penempatan, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			p.id, p.kode_transaksi,
			p.instrumen_id, p.periode_id,
			p.nominal_idr, p.eir_awal,
			p.maker_id, p.tenant_id,
			COALESCE(i.klasifikasi_psak71::text, '')
		FROM trx.penempatan_deposito p
		LEFT JOIN mst.instrumen i ON i.id = p.instrumen_id
		WHERE p.workflow_status = 'APPROVED_ACTIVE'::trx.penempatan_workflow_status
		  AND p.tanggal_jatuh_tempo <= $1
		  AND p.deleted_at IS NULL
		  AND p.tenant_id = $2`,
		asOfDate, tenantID)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.GetMaturingInstruments: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is non-actionable on read queries

	var result []Penempatan
	for rows.Next() {
		var p Penempatan
		if err := rows.Scan(
			&p.ID, &p.KodeTransaksi,
			&p.InstrumenID, &p.PeriodeID,
			&p.NominalIDR, &p.EIRAwal,
			&p.MakerID, &p.TenantID,
			&p.KlasifikasiPSAK71,
		); err != nil {
			return nil, fmt.Errorf("penempatan.Repo.GetMaturingInstruments scan: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ─── Period validation ────────────────────────────────────────────────────────

// IsPeriodeOpen checks if the given periode_id has status_periode = 'OPEN'.
func (r *Repo) IsPeriodeOpen(ctx context.Context, tx *sql.Tx, periodeID uuid.UUID) (bool, error) {
	var status string
	var q interface {
		QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	} = r.db
	if tx != nil {
		q = tx
	}
	err := q.QueryRowContext(ctx,
		`SELECT status_periode FROM mst.periode_buku WHERE id = $1`, periodeID,
	).Scan(&status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("penempatan.Repo.IsPeriodeOpen: %w", err)
	}
	return status == "OPEN", nil
}

// ─── Instrumen validation ─────────────────────────────────────────────────────

// InstrumenInfo holds fields needed for penempatan create validation.
type InstrumenInfo struct {
	ID                uuid.UUID
	Nama              string
	WorkflowStatus    string
	KlasifikasiPSAK71 string
	TipeInstrumen     string
	StatusAktif       string
}

// GetInstrumenInfo reads instrumen fields needed for validation and EIR dispatch.
func (r *Repo) GetInstrumenInfo(ctx context.Context, instrumenID uuid.UUID) (*InstrumenInfo, error) {
	var info InstrumenInfo
	var klasifikasi, tipe *string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, nama, COALESCE(workflow_status::text, ''), COALESCE(klasifikasi_psak71::text, ''),
		       COALESCE(tipe_instrumen::text, ''), COALESCE(status::text, '')
		FROM mst.instrumen WHERE id = $1 AND deleted_at IS NULL`, instrumenID,
	).Scan(&info.ID, &info.Nama, &info.WorkflowStatus, &info.KlasifikasiPSAK71, &info.TipeInstrumen, &info.StatusAktif)
	_ = klasifikasi
	_ = tipe
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.GetInstrumenInfo: %w", err)
	}
	return &info, nil
}

// ─── FX Rate lookup ──────────────────────────────────────────────────────────

// GetKursJISDOR reads BI JISDOR rate for a currency on a given date.
func (r *Repo) GetKursJISDOR(ctx context.Context, mataUangID uuid.UUID, tanggal time.Time) (*decimal.Decimal, error) {
	var kurs decimal.Decimal
	err := r.db.QueryRowContext(ctx, `
		SELECT kurs_tengah FROM mst.kurs
		WHERE mata_uang_id = $1 AND tanggal_berlaku = $2 AND workflow_status = 'APPROVED' AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT 1`,
		mataUangID, tanggal.Format("2006-01-02"),
	).Scan(&kurs)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.GetKursJISDOR: %w", err)
	}
	return &kurs, nil
}

// ─── Settlement balance hint ──────────────────────────────────────────────────

// GetSettlementBalanceHint reads sys.settlement_account_balance for a settlement account.
// Returns nil if no record exists (informational only, never blocks — DEC-P5-M1-004).
func (r *Repo) GetSettlementBalanceHint(ctx context.Context, accountCode string, tenantID string) (*SettlementBalanceHint, error) {
	var balance decimal.Decimal
	var asOfDate time.Time
	var updatedAt time.Time

	err := r.db.QueryRowContext(ctx, `
		SELECT balance, as_of_date, updated_at
		FROM sys.settlement_account_balance
		WHERE account_code = $1 AND currency = 'IDR' AND tenant_id = $2 AND deleted_at IS NULL
		LIMIT 1`,
		accountCode, tenantID,
	).Scan(&balance, &asOfDate, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.GetSettlementBalanceHint: %w", err)
	}

	isStale := time.Since(updatedAt) > 24*time.Hour
	return &SettlementBalanceHint{
		LastKnownIDR: balance,
		AsOfDate:     asOfDate,
		IsStale:      isStale,
		IsSufficient: nil, // always nil in Phase 5 (DEC-P5-M1-004)
	}, nil
}

// ─── ECL stage_history INSERT ────────────────────────────────────────────────

// InsertStageHistory inserts an initial Stage 1 row into ecl.stage_history (in-tx, DEC-P5-M1-001).
// Only called for AC/FVOCI/POCI instruments at approve time.
func (r *Repo) InsertStageHistory(ctx context.Context, tx *sql.Tx, instrumenID uuid.UUID, penempatanID uuid.UUID, periodeID uuid.UUID, createdBy uuid.UUID, tenantID string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ecl.stage_history (
			id, instrumen_id, stage_sesudah, trigger_type,
			penempatan_id, periode_id, status_approval,
			created_at, created_by, updated_at, updated_by, tenant_id
		) VALUES (
			gen_random_uuid(), $1, 'STAGE_1', 'INITIAL_PLACEMENT',
			$2, $3, 'AUTO',
			now(), $4, now(), $4, $5
		)`,
		instrumenID, penempatanID, periodeID, createdBy, tenantID,
	)
	if err != nil {
		return fmt.Errorf("penempatan.Repo.InsertStageHistory: %w", err)
	}
	return nil
}

// ─── DB transaction helper ───────────────────────────────────────────────────

// BeginTx begins a database transaction.
func (r *Repo) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return r.db.BeginTx(ctx, nil)
}

// ─── Audit timeline ──────────────────────────────────────────────────────────

// GetAuditTimeline queries aud.audit_log for a penempatan entity.
// Non-AUDIT roles receive events with before/after redacted (nil).
func (r *Repo) GetAuditTimeline(ctx context.Context, id uuid.UUID, includePayload bool, tenantID string) ([]AuditTimelineEvent, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, timestamp, actor_user_id, actor_role, action,
		       before_value, after_value, COALESCE(trace_id, '')
		FROM aud.audit_log
		WHERE entity_type = 'trx.penempatan_deposito'
		  AND entity_id = $1
		  AND tenant_id = $2
		ORDER BY timestamp ASC
		LIMIT 500`,
		id, tenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("penempatan.Repo.GetAuditTimeline: %w", err)
	}
	defer rows.Close() //nolint:errcheck // rows.Close error is non-actionable on read queries

	var events []AuditTimelineEvent
	for rows.Next() {
		var evt AuditTimelineEvent
		var actorID string
		var before, after []byte
		var traceID string
		if err := rows.Scan(
			&evt.EventID, &evt.EventTime, &actorID, &evt.ActorRole, &evt.Action,
			&before, &after, &traceID,
		); err != nil {
			return nil, fmt.Errorf("penempatan.Repo.GetAuditTimeline scan: %w", err)
		}
		parsed, pErr := uuid.Parse(actorID)
		if pErr == nil {
			evt.ActorUserID = parsed
		}
		evt.TraceID = traceID
		if includePayload {
			if before != nil {
				s := string(before)
				evt.BeforeJSON = &s
			}
			if after != nil {
				s := string(after)
				evt.AfterJSON = &s
			}
		}
		events = append(events, evt)
	}
	return events, rows.Err()
}

// ─── helpers on Penempatan (avoid field access ambiguity) ─────────────────────

// TerminateReqReason is a helper to set TerminateRequestReason from a pointer.
func (p *Penempatan) TerminateReqReason(s *string) {
	p.TerminateRequestReason = s
}
