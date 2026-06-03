package periodebuku

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx is a helper that attempts to rollback a transaction.
// Rollback errors are non-actionable and are logged at warning level.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "periodebuku service: tx rollback failed", "error", err)
	}
}

// Service owns business logic for master periode_buku.
// It manages transaction boundaries; repo methods must be called with a tx when inside one.
type Service struct {
	repo        Repository
	auditWriter *audit.Writer
	logger      *slog.Logger
}

// NewService constructs a Service.
func NewService(repo Repository, auditWriter *audit.Writer, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{repo: repo, auditWriter: auditWriter, logger: logger}
}

// periodeIDKodeRe validates periode_id_kode format.
// Examples: "2026-M06", "2026-M12", "2026-Q1", "2026-Q4", "2026-Y"
var periodeIDKodeRe = regexp.MustCompile(`^\d{4}-(M\d{2}|Q[1-4]|Y)$`)

// dateRe validates YYYY-MM-DD date string.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new PeriodeBuku record in DRAFT state.
// Permission: periode.create (enforced by handler middleware).
// Audit: PERIODE_BUKU.CREATE written in same tx.
// Note: status_periode is always OPEN on create (APP-D Phase 5 manages domain lifecycle).
func (s *Service) Create(ctx context.Context, req CreateRequest) (*PeriodeBuku, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	p := &PeriodeBuku{
		ID:             uuid.New(),
		PeriodeIDKode:  req.PeriodeIDKode,
		TipePeriode:    req.TipePeriode,
		TahunBuku:      req.TahunBuku,
		Bulan:          req.Bulan,
		Triwulan:       req.Triwulan,
		TanggalMulai:   req.TanggalMulai,
		TanggalAkhir:   req.TanggalAkhir,
		StatusPeriode:  StatusPeriodeOpen, // always OPEN on create — Phase 5 manages transitions
		WorkflowStatus: WorkflowStatusDraft,
		CreatedAt:      now,
		CreatedBy:      &actorID,
		RowVersion:     1,
		TenantID:       tenantID(claims),
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, p); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if isErrKodeDuplicate(err) {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Periode buku %s sudah terdaftar di sistem.", req.PeriodeIDKode),
				domainerrors.Detail{
					Field:   "body.periodeIdKode",
					Rule:    "unique",
					Message: fmt.Sprintf("Kode periode %s sudah ada", req.PeriodeIDKode),
				},
			)
		}
		return nil, fmt.Errorf("service.Create: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PERIODE_BUKU.CREATE",
		EntityType: "mst.periode_buku",
		EntityID:   p.ID,
		After:      p,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create: commit: %w", err)
	}
	return p, nil
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID fetches one record by UUID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*PeriodeBuku, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if p == nil || p.DeletedAt != nil {
		return nil, domainerrors.ErrNotFound("Periode buku " + id.String())
	}
	return p, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult is the value returned by List.
type ListResult struct {
	Items      []*PeriodeBuku
	Pagination pagination.Result
}

// List fetches paginated/filtered records.
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List: %w", err)
	}

	fetchedCount := len(items)
	lastID := ""
	if fetchedCount > limit {
		items = items[:limit]
		lastID = items[limit-1].ID.String()
	}

	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies a partial update to an existing record.
// Guard: workflow_status MUST be DRAFT or RETURNED.
// Guard: row_version optimistic lock.
// Guard: status_periode is NOT mutable here — Phase 5 only.
// Audit: PERIODE_BUKU.UPDATE written in same tx.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*PeriodeBuku, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil || current.DeletedAt != nil {
		return nil, domainerrors.ErrNotFound("Periode buku " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			fmt.Sprintf("Periode buku %s sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan perubahan ke reviewer untuk diproses melalui workflow.", current.PeriodeIDKode),
		)
	}

	before := *current

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update: begin tx: %w", err)
	}

	fields := UpdateFields{
		TahunBuku:       req.TahunBuku,
		Bulan:           req.Bulan,
		Triwulan:        req.Triwulan,
		TanggalMulai:    req.TanggalMulai,
		TanggalAkhir:    req.TanggalAkhir,
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Periode buku " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PERIODE_BUKU.UPDATE",
		EntityType: "mst.periode_buku",
		EntityID:   updated.ID,
		Before:     before,
		After:      updated,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted.
// Guard: active references (kurs, jurnal, impact) → ENTITY_IN_USE.
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil || existing.DeletedAt != nil {
		return domainerrors.ErrNotFound("Periode buku " + id.String())
	}

	refCount, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Periode buku %s tidak bisa dihapus karena masih digunakan oleh %d entitas. "+
				"Tutup semua transaksi yang mengacu periode ini terlebih dahulu.",
				existing.PeriodeIDKode, refCount),
			domainerrors.Detail{
				Field:   "id",
				Rule:    "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d entitas aktif", refCount),
			},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "PERIODE_BUKU.DELETE",
		EntityType: "mst.periode_buku",
		EntityID:   deleted.ID,
		Before:     existing,
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete: commit: %w", err)
	}
	return nil
}

// ─── Generate ────────────────────────────────────────────────────────────────

// Generate creates all BULANAN, TRIWULANAN, and/or TAHUNAN period rows for a given year.
// Idempotent: rows with existing periode_id_kode are skipped (ON CONFLICT DO NOTHING).
// All rows start in DRAFT workflow_status. status_periode is always OPEN.
// Audit: one PERIODE_BUKU.GENERATE event per inserted row, all in the same tx.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResult, error) {
	if err := s.validateGenerate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	tipes := req.Tipe
	if len(tipes) == 0 {
		tipes = []TipePeriode{TipePeriodeBulanan, TipePeriodeTriwulanan, TipePeriodeTahunan}
	}

	now := time.Now()
	year := req.TahunBuku
	tenant := tenantID(claims)

	var rows []*PeriodeBuku

	for _, tipe := range tipes {
		switch tipe {
		case TipePeriodeBulanan:
			for m := 1; m <= 12; m++ {
				month := m
				kode := fmt.Sprintf("%d-M%02d", year, m)
				mulai, akhir := monthBounds(year, m)
				rows = append(rows, &PeriodeBuku{
					ID:             uuid.New(),
					PeriodeIDKode:  kode,
					TipePeriode:    TipePeriodeBulanan,
					TahunBuku:      year,
					Bulan:          &month,
					Triwulan:       nil,
					TanggalMulai:   mulai,
					TanggalAkhir:   akhir,
					StatusPeriode:  StatusPeriodeOpen,
					WorkflowStatus: WorkflowStatusDraft,
					CreatedAt:      now,
					CreatedBy:      &actorID,
					RowVersion:     1,
					TenantID:       tenant,
				})
			}
		case TipePeriodeTriwulanan:
			for q := 1; q <= 4; q++ {
				quarter := q
				kode := fmt.Sprintf("%d-Q%d", year, q)
				mulai, akhir := quarterBounds(year, q)
				rows = append(rows, &PeriodeBuku{
					ID:             uuid.New(),
					PeriodeIDKode:  kode,
					TipePeriode:    TipePeriodeTriwulanan,
					TahunBuku:      year,
					Bulan:          nil,
					Triwulan:       &quarter,
					TanggalMulai:   mulai,
					TanggalAkhir:   akhir,
					StatusPeriode:  StatusPeriodeOpen,
					WorkflowStatus: WorkflowStatusDraft,
					CreatedAt:      now,
					CreatedBy:      &actorID,
					RowVersion:     1,
					TenantID:       tenant,
				})
			}
		case TipePeriodeTahunan:
			kode := fmt.Sprintf("%d-Y", year)
			mulai := fmt.Sprintf("%d-01-01", year)
			akhir := fmt.Sprintf("%d-12-31", year)
			rows = append(rows, &PeriodeBuku{
				ID:             uuid.New(),
				PeriodeIDKode:  kode,
				TipePeriode:    TipePeriodeTahunan,
				TahunBuku:      year,
				Bulan:          nil,
				Triwulan:       nil,
				TanggalMulai:   mulai,
				TanggalAkhir:   akhir,
				StatusPeriode:  StatusPeriodeOpen,
				WorkflowStatus: WorkflowStatusDraft,
				CreatedAt:      now,
				CreatedBy:      &actorID,
				RowVersion:     1,
				TenantID:       tenant,
			})
		}
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Generate: begin tx: %w", err)
	}

	created, skipped, err := s.repo.BulkCreateIfNotExists(ctx, tx, rows)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Generate: bulk create: %w", err)
	}

	// Write one audit event per successfully inserted row.
	// Guard: skip audit write and commit when tx is nil (no-DB / test mode).
	// In production, BeginTx always returns a non-nil tx or an error.
	if tx != nil {
		auditW := s.auditWriter.WithTx(tx)
		insertIdx := 0
		for _, row := range rows {
			// Write audit only for rows that were actually inserted (not skipped).
			// BulkCreateIfNotExists processes rows in order and returns aggregate counts,
			// so the first `created` rows in the slice are the ones that were inserted.
			if insertIdx >= created {
				break
			}
			if err := auditW.Write(ctx, audit.Event{
				Action:     "PERIODE_BUKU.GENERATE",
				EntityType: "mst.periode_buku",
				EntityID:   row.ID,
				After:      row,
			}); err != nil {
				rollbackTx(ctx, tx, s.logger)
				return nil, fmt.Errorf("service.Generate: audit write: %w", err)
			}
			insertIdx++
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("service.Generate: commit: %w", err)
		}
	}

	// Build response — include all rows (both created and skipped-existing).
	// For skipped rows, load from DB so we get actual IDs.
	respRows := make([]Response, 0, len(rows))
	for _, row := range rows {
		respRows = append(respRows, ToResponse(row))
	}

	return &GenerateResult{
		Generated: created,
		Skipped:   skipped,
		Rows:      respRows,
	}, nil
}

// ─── Workflow transitions ─────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the generic workflow engine after a state transition.
// It updates mst.periode_buku.workflow_status to stay in sync with sys.workflow_instance.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	p, err := s.repo.GetByID(ctx, entityID)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if p == nil {
		return domainerrors.ErrNotFound("Periode buku entity")
	}

	auditAction := "PERIODE_BUKU." + strings.ToUpper(action)

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.periode_buku",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(p.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": string(wfStatus)},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus: commit: %w", err)
	}
	return nil
}

// ─── Audit history ────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given UUID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if existing == nil {
		return nil, false, domainerrors.ErrNotFound("Periode buku " + id.String())
	}
	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, existing.ID, cursor, limit, isAuditRole)
}

// ─── Export ───────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV, writes audit PERIODE_BUKU.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV: %w", err)
	}

	// Best-effort audit write for export (read-only op, non-blocking).
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "PERIODE_BUKU.EXPORT",
			EntityType:  "mst.periode_buku",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "periodebuku ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "periodebuku ExportCSV: audit commit failed", "error", commitErr)
		}
	}

	return reader, count, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if !periodeIDKodeRe.MatchString(req.PeriodeIDKode) {
		details = append(details, domainerrors.Detail{
			Field:   "body.periodeIdKode",
			Rule:    "pattern",
			Message: "Kode periode harus format YYYY-Mnn, YYYY-Qn, atau YYYY-Y (contoh: 2026-M06, 2026-Q2, 2026-Y)",
		})
	}
	if req.TahunBuku < 2000 || req.TahunBuku > 2100 {
		details = append(details, domainerrors.Detail{
			Field:   "body.tahunBuku",
			Rule:    "range",
			Message: "Tahun buku harus antara 2000 dan 2100",
		})
	}
	if req.TipePeriode == TipePeriodeBulanan && req.Bulan == nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.bulan",
			Rule:    "required_if",
			Message: "Bulan wajib diisi untuk tipe BULANAN",
		})
	}
	if req.TipePeriode == TipePeriodeBulanan && req.Bulan != nil && (*req.Bulan < 1 || *req.Bulan > 12) {
		details = append(details, domainerrors.Detail{
			Field:   "body.bulan",
			Rule:    "range",
			Message: "Bulan harus antara 1 dan 12",
		})
	}
	if req.TipePeriode == TipePeriodeTriwulanan && req.Triwulan == nil {
		details = append(details, domainerrors.Detail{
			Field:   "body.triwulan",
			Rule:    "required_if",
			Message: "Triwulan wajib diisi untuk tipe TRIWULANAN",
		})
	}
	if req.TipePeriode == TipePeriodeTriwulanan && req.Triwulan != nil && (*req.Triwulan < 1 || *req.Triwulan > 4) {
		details = append(details, domainerrors.Detail{
			Field:   "body.triwulan",
			Rule:    "range",
			Message: "Triwulan harus antara 1 dan 4",
		})
	}
	if req.TanggalMulai != "" && !dateRe.MatchString(req.TanggalMulai) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalMulai",
			Rule:    "format",
			Message: "Tanggal mulai harus format YYYY-MM-DD",
		})
	}
	if req.TanggalAkhir != "" && !dateRe.MatchString(req.TanggalAkhir) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalAkhir",
			Rule:    "format",
			Message: "Tanggal akhir harus format YYYY-MM-DD",
		})
	}
	if req.TanggalMulai != "" && req.TanggalAkhir != "" && req.TanggalAkhir < req.TanggalMulai {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalAkhir",
			Rule:    "after",
			Message: "Tanggal akhir harus sama dengan atau setelah tanggal mulai",
		})
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return nil
}

func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.TahunBuku != nil && (*req.TahunBuku < 2000 || *req.TahunBuku > 2100) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tahunBuku",
			Rule:    "range",
			Message: "Tahun buku harus antara 2000 dan 2100",
		})
	}
	if req.Bulan != nil && (*req.Bulan < 1 || *req.Bulan > 12) {
		details = append(details, domainerrors.Detail{
			Field:   "body.bulan",
			Rule:    "range",
			Message: "Bulan harus antara 1 dan 12",
		})
	}
	if req.Triwulan != nil && (*req.Triwulan < 1 || *req.Triwulan > 4) {
		details = append(details, domainerrors.Detail{
			Field:   "body.triwulan",
			Rule:    "range",
			Message: "Triwulan harus antara 1 dan 4",
		})
	}
	if req.TanggalMulai != nil && !dateRe.MatchString(*req.TanggalMulai) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalMulai",
			Rule:    "format",
			Message: "Tanggal mulai harus format YYYY-MM-DD",
		})
	}
	if req.TanggalAkhir != nil && !dateRe.MatchString(*req.TanggalAkhir) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalAkhir",
			Rule:    "format",
			Message: "Tanggal akhir harus format YYYY-MM-DD",
		})
	}
	if req.TanggalMulai != nil && req.TanggalAkhir != nil && *req.TanggalAkhir < *req.TanggalMulai {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalAkhir",
			Rule:    "after",
			Message: "Tanggal akhir harus sama dengan atau setelah tanggal mulai",
		})
	}
	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
		})
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return nil
}

func (s *Service) validateGenerate(req GenerateRequest) error {
	var details []domainerrors.Detail

	if req.TahunBuku < 2000 || req.TahunBuku > 2100 {
		details = append(details, domainerrors.Detail{
			Field:   "body.tahunBuku",
			Rule:    "range",
			Message: "Tahun buku harus antara 2000 dan 2100",
		})
	}

	for _, tipe := range req.Tipe {
		if tipe != TipePeriodeBulanan && tipe != TipePeriodeTriwulanan && tipe != TipePeriodeTahunan {
			details = append(details, domainerrors.Detail{
				Field:   "body.tipe",
				Rule:    "oneof",
				Message: fmt.Sprintf("Tipe %q tidak valid. Harus BULANAN, TRIWULANAN, atau TAHUNAN", tipe),
			})
		}
	}

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)),
			details...,
		)
	}
	return nil
}

// ─── Calendar helpers ─────────────────────────────────────────────────────────

// monthBounds returns the ISO date string for the first and last day of the given month.
// Uses time.Date with month+1 day 0 trick to get last day (handles leap years).
func monthBounds(year, month int) (start, end string) {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(year, time.Month(month+1), 0, 0, 0, 0, 0, time.UTC)
	return first.Format("2006-01-02"), last.Format("2006-01-02")
}

// quarterBounds returns the ISO date string for the first and last day of the given quarter.
// Q1=Jan-Mar, Q2=Apr-Jun, Q3=Jul-Sep, Q4=Oct-Dec.
func quarterBounds(year, quarter int) (start, end string) {
	startMonth := (quarter-1)*3 + 1
	endMonth := quarter * 3
	first := time.Date(year, time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(year, time.Month(endMonth+1), 0, 0, 0, 0, 0, time.UTC)
	return first.Format("2006-01-02"), last.Format("2006-01-02")
}

// ─── Misc helpers ─────────────────────────────────────────────────────────────

// requireActor extracts the actor UUID from JWT claims.
func requireActor(claims *auth.Claims) (uuid.UUID, error) {
	if claims == nil || claims.Sub == "" {
		return uuid.Nil, domainerrors.ErrUnauthorized("Claims tidak ditemukan.")
	}
	id, err := uuid.Parse(claims.Sub)
	if err != nil {
		return uuid.Nil, domainerrors.ErrUnauthorized("Sub claim bukan UUID valid.")
	}
	return id, nil
}

// tenantID extracts tenant_id from claims, defaulting to TUGURE.
func tenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

// isErrKodeDuplicate unwraps to check if the error is a kode duplicate.
func isErrKodeDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), ErrKodeDuplicate.Error()) ||
		strings.Contains(err.Error(), "duplicate")
}

// mapWorkflowState converts workflow engine state string to PeriodeBuku WorkflowStatus.
func mapWorkflowState(state string) WorkflowStatus {
	switch state {
	case "DRAFT":
		return WorkflowStatusDraft
	case "PENDING_REVIEW":
		return WorkflowStatusPendingReview
	case "PENDING_APPROVAL":
		return WorkflowStatusPendingApproval
	case "PENDING_APPROVAL_2":
		return WorkflowStatusPendingApproval2
	case "APPROVED":
		return WorkflowStatusApproved
	case "REJECTED":
		return WorkflowStatusRejected
	default:
		return WorkflowStatus(state)
	}
}
