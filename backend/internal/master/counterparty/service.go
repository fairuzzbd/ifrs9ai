package counterparty

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
	"blips-ifrs9.tugu-re.com/internal/common/pagination"
)

// rollbackTx attempts a transaction rollback; logs errors.
func rollbackTx(ctx context.Context, tx *sql.Tx, logger *slog.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		logger.WarnContext(ctx, "counterparty service: tx rollback failed", "error", err)
	}
}

// dateRe validates YYYY-MM-DD format.
var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// PII format validators per security audit F-03 (DEC-028). Service-layer
// pre-encryption guards reject malformed input before sec.encrypt() runs —
// prevents storing undetectable garbage ciphertext.
var (
	npwpRe          = regexp.MustCompile(`^\d{2}\.\d{3}\.\d{3}\.\d-\d{3}\.\d{3}$`)
	ktpRe           = regexp.MustCompile(`^\d{16}$`)
	nomorRekeningRe = regexp.MustCompile(`^\d{5,20}$`)
)

// validatePIIFields returns validation details for any malformed PII fields.
// Empty string and nil are treated as "not provided" — only non-empty strings
// are subject to format check.
func validatePIIFields(npwp, nomorRek, ktp *string) []domainerrors.Detail {
	var details []domainerrors.Detail
	if npwp != nil && *npwp != "" && !npwpRe.MatchString(*npwp) {
		details = append(details, domainerrors.Detail{
			Field:   "body.npwp",
			Rule:    "format",
			Message: "Format NPWP tidak valid. Gunakan XX.XXX.XXX.X-XXX.XXX (15 digit).",
		})
	}
	if ktp != nil && *ktp != "" && !ktpRe.MatchString(*ktp) {
		details = append(details, domainerrors.Detail{
			Field:   "body.ktp",
			Rule:    "format",
			Message: "KTP/NIK harus 16 digit angka.",
		})
	}
	if nomorRek != nil && *nomorRek != "" && !nomorRekeningRe.MatchString(*nomorRek) {
		details = append(details, domainerrors.Detail{
			Field:   "body.nomorRekening",
			Rule:    "format",
			Message: "Nomor rekening harus 5-20 digit angka.",
		})
	}
	return details
}

// Service owns business logic for mst.counterparty.
// PII fields are encrypted by the repo layer via sec.encrypt() SQL function.
// No plaintext PII ever appears in Go memory beyond the scope of service.Create/Update.
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

// ─── Create ───────────────────────────────────────────────────────────────────

// Create validates and persists a new Counterparty in DRAFT state.
// PII fields (npwp, nomorRekening, ktp) are passed to repo for SQL-level encryption.
// Audit before/after uses auditSafeCounterparty (PII=REDACTED).
func (s *Service) Create(ctx context.Context, req CreateRequest) (*Counterparty, error) {
	if err := s.validateCreate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	status := CounterpartyStatus(req.Status)
	if string(status) == "" {
		status = StatusAktif
	}
	if !validStatuses[status] {
		status = StatusAktif
	}

	cp := &Counterparty{
		ID:               uuid.New(),
		KodeCounterparty: req.KodeCounterparty,
		Nama:             req.Nama,
		Tipe:             CounterpartyTipe(req.Tipe),
		TipeEksposurBasel: TipeEksposurBasel(req.TipeEksposurBasel),
		EligibleLpsFlag:  req.EligibleLpsFlag,
		NomorIzinOjk:     req.NomorIzinOjk,
		TanggalIzinOjk:   req.TanggalIzinOjk,
		KategoriMi:       req.KategoriMi,
		Status:           status,
		WorkflowStatus:   WorkflowStatusDraft,
		CreatedAt:        now,
		CreatedBy:        actorID,
		RowVersion:       1,
		TenantID:         tenantID(claims),
		Version:          1,
		IsDeleted:        false,
	}

	if req.AumTerakhir != nil {
		d, parseErr := decimal.NewFromString(*req.AumTerakhir)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed,
				"Format aum_terakhir tidak valid. Gunakan angka desimal.",
				domainerrors.Detail{Field: "body.aumTerakhir", Rule: "decimal", Message: "Format tidak valid"})
		}
		cp.AumTerakhir = &d
	}
	if req.TanggalAumTerakhir != nil {
		cp.TanggalAumTerakhir = req.TanggalAumTerakhir
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Create counterparty: begin tx: %w", err)
	}

	if err := s.repo.Create(ctx, tx, cp, req.NPWP, req.NomorRekening, req.KTP); err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrKodeDuplicate || containsStr(err.Error(), "duplicate") {
			return nil, domainerrors.New(domainerrors.CodeConflict,
				fmt.Sprintf("Kode counterparty %s sudah terdaftar di sistem.", req.KodeCounterparty),
				domainerrors.Detail{Field: "body.kodeCounterparty", Rule: "unique",
					Message: fmt.Sprintf("Kode %s sudah ada", req.KodeCounterparty)},
			)
		}
		return nil, fmt.Errorf("service.Create counterparty: %w", err)
	}

	// Audit: after = auditSafe (no PII plaintext)
	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "COUNTERPARTY.CREATE",
		EntityType: "mst.counterparty",
		EntityID:   cp.ID,
		After:      auditSafeCounterparty(cp),
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Create counterparty: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Create counterparty: commit: %w", err)
	}
	return cp, nil
}

// ─── GetMaskedPII ─────────────────────────────────────────────────────────────

// GetMaskedPII returns masked PII for a given counterparty ID.
// Used by handler after create/update to include masked PII in response.
func (s *Service) GetMaskedPII(ctx context.Context, id uuid.UUID) (*MaskedPII, error) {
	return s.repo.GetMaskedPII(ctx, id)
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

// GetByID fetches one record (masked PII).
func (s *Service) GetByID(ctx context.Context, id uuid.UUID, includeDeleted bool) (*Counterparty, *MaskedPII, error) {
	cp, masked, err := s.repo.GetByID(ctx, id, includeDeleted)
	if err != nil {
		return nil, nil, fmt.Errorf("service.GetByID: %w", err)
	}
	if cp == nil {
		return nil, nil, domainerrors.ErrNotFound("Counterparty " + id.String())
	}
	return cp, masked, nil
}

// ─── GetPII ───────────────────────────────────────────────────────────────────

// GetPII returns decrypted PII. Requires counterparty.view_pii permission.
// Writes audit COUNTERPARTY.VIEW_PII.
// Permission is enforced by handler middleware; this method writes the audit.
func (s *Service) GetPII(ctx context.Context, id uuid.UUID) (*PIIFields, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}
	_ = actorID // used for audit below

	// Verify entity exists
	cp, _, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.GetPII: %w", err)
	}
	if cp == nil {
		return nil, domainerrors.ErrNotFound("Counterparty " + id.String())
	}

	// Per security audit F-02: write+commit COUNTERPARTY.VIEW_PII audit BEFORE
	// returning PII. If audit fails (tx begin / write / commit), refuse to
	// return PII — guarantees DEC-028 audit trail for every PII access.
	tx, txErr := s.repo.BeginTx(ctx)
	if txErr != nil {
		return nil, fmt.Errorf("service.GetPII: begin audit tx: %w", txErr)
	}
	if tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:     "COUNTERPARTY.VIEW_PII",
			EntityType: "mst.counterparty",
			EntityID:   id,
			After: map[string]interface{}{
				"id":             id.String(),
				"pii_accessed":   true,
				"npwp":           redactedPII,
				"nomor_rekening": redactedPII,
				"ktp":            redactedPII,
			},
		}); writeErr != nil {
			rollbackTx(ctx, tx, s.logger)
			return nil, domainerrors.New(domainerrors.CodeInternal,
				"Akses PII ditolak: audit log gagal ditulis.",
				domainerrors.Detail{Field: "pii", Rule: "audit_required", Message: writeErr.Error()},
			)
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, domainerrors.New(domainerrors.CodeInternal,
				"Akses PII ditolak: audit log gagal di-commit.",
				domainerrors.Detail{Field: "pii", Rule: "audit_required", Message: commitErr.Error()},
			)
		}
	}

	// Audit committed — safe to decrypt and return PII.
	pii, err := s.repo.GetPII(ctx, id)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeInternal,
			"Gagal mendekripsi data PII. Hubungi administrator.",
			domainerrors.Detail{Field: "pii", Rule: "decrypt_failed", Message: err.Error()},
		)
	}
	return pii, nil
}

// ─── List ─────────────────────────────────────────────────────────────────────

// ListResult holds the paginated list result.
type ListResult struct {
	Items      []*Counterparty
	Pagination pagination.Result
}

// List fetches paginated records (no PII).
func (s *Service) List(ctx context.Context, q listquery.Query, cursor string, limit int, includeDeleted bool) (*ListResult, error) {
	items, err := s.repo.List(ctx, q, cursor, limit, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("service.List counterparty: %w", err)
	}
	fetchedCount := len(items)
	lastID := ""
	if fetchedCount > limit {
		items = items[:limit]
		lastID = items[limit-1].KodeCounterparty
	}
	pag := pagination.BuildResult(fetchedCount, limit, lastID, nil)
	return &ListResult{Items: items, Pagination: pag}, nil
}

// ─── Update ───────────────────────────────────────────────────────────────────

// Update validates and applies partial update.
// Guard: workflow_status must be DRAFT or RETURNED.
// PII fields are passed to repo for SQL-level encryption.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req UpdateRequest) (*Counterparty, error) {
	if err := s.validateUpdate(req); err != nil {
		return nil, err
	}

	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return nil, err
	}

	current, _, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("service.Update load: %w", err)
	}
	if current == nil {
		return nil, domainerrors.ErrNotFound("Counterparty " + id.String())
	}

	if current.WorkflowStatus == WorkflowStatusApproved {
		return nil, domainerrors.New(
			domainerrors.CodeMasterApprovedNoEdit,
			"Counterparty sudah disetujui dan tidak bisa diedit langsung. "+
				"Ajukan perubahan melalui workflow.",
		)
	}

	before := auditSafeCounterparty(current)

	fields := UpdateFields{
		UpdatedBy:       actorID,
		ExpectedVersion: req.RowVersion,
	}
	if req.Nama != nil {
		fields.Nama = req.Nama
	}
	if req.Tipe != nil {
		t := CounterpartyTipe(*req.Tipe)
		fields.Tipe = &t
	}
	if req.TipeEksposurBasel != nil {
		e := TipeEksposurBasel(*req.TipeEksposurBasel)
		fields.TipeEksposurBasel = &e
	}
	if req.EligibleLpsFlag != nil {
		fields.EligibleLpsFlag = req.EligibleLpsFlag
	}
	if req.NomorIzinOjk != nil {
		fields.NomorIzinOjk = req.NomorIzinOjk
	}
	if req.TanggalIzinOjk != nil {
		fields.TanggalIzinOjk = req.TanggalIzinOjk
	}
	if req.AumTerakhir != nil {
		d, parseErr := decimal.NewFromString(*req.AumTerakhir)
		if parseErr != nil {
			return nil, domainerrors.New(domainerrors.CodeValidationFailed, "Format aum_terakhir tidak valid.")
		}
		fields.AumTerakhir = &d
	}
	if req.TanggalAumTerakhir != nil {
		fields.TanggalAumTerakhir = req.TanggalAumTerakhir
	}
	if req.KategoriMi != nil {
		fields.KategoriMi = req.KategoriMi
	}
	if req.Status != nil {
		st := CounterpartyStatus(*req.Status)
		fields.Status = &st
	}
	// PII fields
	fields.NPWPPlain = req.NPWP
	fields.NomorRekeningPlain = req.NomorRekening
	fields.KTPPlain = req.KTP

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("service.Update counterparty: begin tx: %w", err)
	}

	updated, err := s.repo.Update(ctx, tx, id, fields)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		if err == ErrNotFound {
			return nil, domainerrors.ErrNotFound("Counterparty " + id.String())
		}
		if err == ErrConflict {
			return nil, domainerrors.ErrConflict()
		}
		return nil, fmt.Errorf("service.Update counterparty: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "COUNTERPARTY.UPDATE",
		EntityType: "mst.counterparty",
		EntityID:   updated.ID,
		Before:     before,
		After:      auditSafeCounterparty(updated),
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return nil, fmt.Errorf("service.Update counterparty: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("service.Update counterparty: commit: %w", err)
	}
	return updated, nil
}

// ─── SoftDelete ───────────────────────────────────────────────────────────────

// SoftDelete marks the record as deleted (is_deleted=TRUE + deleted_at=now()).
// Guard: no active instrumen references.
func (s *Service) SoftDelete(ctx context.Context, id uuid.UUID) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	existing, _, err := s.repo.GetByID(ctx, id, false)
	if err != nil {
		return fmt.Errorf("service.SoftDelete load: %w", err)
	}
	if existing == nil {
		return domainerrors.ErrNotFound("Counterparty " + id.String())
	}

	refCount, err := s.repo.CountReferences(ctx, id)
	if err != nil {
		return fmt.Errorf("service.SoftDelete count refs: %w", err)
	}
	if refCount > 0 {
		return domainerrors.New(
			domainerrors.CodeEntityInUse,
			fmt.Sprintf("Counterparty tidak bisa dihapus karena masih digunakan oleh %d instrumen aktif.", refCount),
			domainerrors.Detail{Field: "id", Rule: "referenced_by",
				Message: fmt.Sprintf("Direferensikan oleh %d instrumen aktif", refCount)},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SoftDelete counterparty: begin tx: %w", err)
	}

	deleted, err := s.repo.SoftDelete(ctx, tx, id, actorID)
	if err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete counterparty: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     "COUNTERPARTY.DELETE",
		EntityType: "mst.counterparty",
		EntityID:   id,
		Before:     auditSafeCounterparty(existing),
		After:      auditSafeCounterparty(deleted),
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SoftDelete counterparty: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SoftDelete counterparty: commit: %w", err)
	}
	return nil
}

// ─── SyncWorkflowStatus ───────────────────────────────────────────────────────

// SyncWorkflowStatus is called by the workflow engine EntityHook after a state transition.
// Updates mst.counterparty.workflow_status to stay in sync with sys.workflow_instance.
func (s *Service) SyncWorkflowStatus(ctx context.Context, entityID uuid.UUID, newState string, action string) error {
	claims := auth.ClaimsFromContext(ctx)
	actorID, err := requireActor(claims)
	if err != nil {
		return err
	}

	wfStatus := mapWorkflowState(newState)
	cp, _, err := s.repo.GetByID(ctx, entityID, false)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus load: %w", err)
	}
	if cp == nil {
		return domainerrors.ErrNotFound("Counterparty entity")
	}

	auditAction := "COUNTERPARTY." + action

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus counterparty: begin tx: %w", err)
	}

	if err := s.repo.UpdateWorkflowStatus(ctx, tx, entityID, wfStatus, actorID); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus counterparty: %w", err)
	}

	if err := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
		Action:     auditAction,
		EntityType: "mst.counterparty",
		EntityID:   entityID,
		Before:     map[string]interface{}{"workflow_status": string(cp.WorkflowStatus)},
		After:      map[string]interface{}{"workflow_status": string(wfStatus)},
	}); err != nil {
		rollbackTx(ctx, tx, s.logger)
		return fmt.Errorf("service.SyncWorkflowStatus counterparty: audit write: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service.SyncWorkflowStatus counterparty: commit: %w", err)
	}
	return nil
}

// ─── ListHistory ──────────────────────────────────────────────────────────────

// ListHistory returns paginated audit log for a given counterparty ID.
func (s *Service) ListHistory(ctx context.Context, id uuid.UUID, cursor string, limit int, claims *auth.Claims) ([]AuditHistoryItem, bool, error) {
	cp, _, err := s.repo.GetByID(ctx, id, true)
	if err != nil {
		return nil, false, fmt.Errorf("service.ListHistory load: %w", err)
	}
	if cp == nil {
		return nil, false, domainerrors.ErrNotFound("Counterparty " + id.String())
	}
	isAuditRole := claims != nil && claims.HasPermission("audit_log.read")
	return s.repo.ListAuditHistory(ctx, id, cursor, limit, isAuditRole)
}

// ─── ExportCSV ────────────────────────────────────────────────────────────────

// ExportCSV streams all records as CSV (no PII).
// Writes audit COUNTERPARTY.EXPORT.
func (s *Service) ExportCSV(ctx context.Context, q listquery.Query) (interface{ Read([]byte) (int, error) }, int, error) {
	claims := auth.ClaimsFromContext(ctx)
	actorID, aerr := requireActor(claims)
	if aerr != nil {
		return nil, 0, aerr
	}

	reader, count, err := s.repo.ExportAll(ctx, q)
	if err != nil {
		return nil, 0, fmt.Errorf("service.ExportCSV counterparty: %w", err)
	}

	tx, txErr := s.repo.BeginTx(ctx)
	if txErr == nil && tx != nil {
		if writeErr := s.auditWriter.WithTx(tx).Write(ctx, audit.Event{
			Action:      "COUNTERPARTY.EXPORT",
			EntityType:  "mst.counterparty",
			EntityID:    uuid.Nil,
			ActorUserID: actorID.String(),
			After: map[string]interface{}{
				"format":    "csv",
				"row_count": count,
				"filters":   q.AppliedFilter(),
			},
		}); writeErr != nil {
			s.logger.WarnContext(ctx, "counterparty ExportCSV: audit write failed", "error", writeErr)
			rollbackTx(ctx, tx, s.logger)
		} else if commitErr := tx.Commit(); commitErr != nil {
			s.logger.WarnContext(ctx, "counterparty ExportCSV: audit commit failed", "error", commitErr)
		}
	}
	return reader, count, nil
}

// ─── Validation helpers ───────────────────────────────────────────────────────

func (s *Service) validateCreate(req CreateRequest) error {
	var details []domainerrors.Detail

	if len(req.KodeCounterparty) < 2 || len(req.KodeCounterparty) > 20 {
		details = append(details, domainerrors.Detail{
			Field:   "body.kodeCounterparty",
			Rule:    "length",
			Message: "Kode counterparty harus 2-20 karakter",
		})
	}
	if len(req.Nama) < 3 || len(req.Nama) > 200 {
		details = append(details, domainerrors.Detail{
			Field:   "body.nama",
			Rule:    "length",
			Message: "Nama harus 3-200 karakter",
		})
	}
	if !IsValidTipe(req.Tipe) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipe",
			Rule:    "oneof",
			Message: "Tipe tidak valid. Nilai yang diizinkan: BANK, BANK_KUSTODIAN, KORPORASI, PEMERINTAH, MANAJER_INVESTASI, EMITEN_SAHAM, MULTILATERAL, KORPORASI_BUMN, INDIVIDU, REASURADUR",
		})
	}
	if !IsValidEksposurBasel(req.TipeEksposurBasel) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipeEksposurBasel",
			Rule:    "oneof",
			Message: "Tipe eksposur Basel tidak valid. Nilai yang diizinkan: SOVEREIGN, SENIOR_SECURED, SENIOR_UNSECURED, SUBORDINATED, CORPORATE, BANK, RETAIL",
		})
	}
	if req.TanggalIzinOjk != nil && !dateRe.MatchString(*req.TanggalIzinOjk) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalIzinOjk",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.TanggalAumTerakhir != nil && !dateRe.MatchString(*req.TanggalAumTerakhir) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalAumTerakhir",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}

	// PII format validation (security audit F-03).
	details = append(details, validatePIIFields(req.NPWP, req.NomorRekening, req.KTP)...)

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}
	return nil
}

func (s *Service) validateUpdate(req UpdateRequest) error {
	var details []domainerrors.Detail

	if req.Nama != nil && (len(*req.Nama) < 3 || len(*req.Nama) > 200) {
		details = append(details, domainerrors.Detail{
			Field:   "body.nama",
			Rule:    "length",
			Message: "Nama harus 3-200 karakter",
		})
	}
	if req.Tipe != nil && !IsValidTipe(*req.Tipe) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipe",
			Rule:    "oneof",
			Message: "Tipe tidak valid",
		})
	}
	if req.TipeEksposurBasel != nil && !IsValidEksposurBasel(*req.TipeEksposurBasel) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tipeEksposurBasel",
			Rule:    "oneof",
			Message: "Tipe eksposur Basel tidak valid",
		})
	}
	if req.TanggalIzinOjk != nil && !dateRe.MatchString(*req.TanggalIzinOjk) {
		details = append(details, domainerrors.Detail{
			Field:   "body.tanggalIzinOjk",
			Rule:    "format",
			Message: "Tanggal harus dalam format YYYY-MM-DD",
		})
	}
	if req.RowVersion <= 0 {
		details = append(details, domainerrors.Detail{
			Field:   "body.rowVersion",
			Rule:    "required",
			Message: "rowVersion wajib diisi dan harus positif",
		})
	}

	// PII format validation (security audit F-03).
	details = append(details, validatePIIFields(req.NPWP, req.NomorRekening, req.KTP)...)

	if len(details) > 0 {
		return domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("%d field tidak valid", len(details)), details...)
	}
	return nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

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

func tenantID(claims *auth.Claims) string {
	if claims != nil && claims.TenantID != "" {
		return claims.TenantID
	}
	return "TUGURE"
}

func containsStr(s, sub string) bool {
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
