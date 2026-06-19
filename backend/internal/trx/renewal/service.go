package renewal

// service.go — Service owns all renewal business logic.
// TX boundary lives here; repos never open transactions.
//
// Business rules enforced:
//   - Instrumen must be DEPOSITO ACTIVE with klasifikasi_locked=TRUE
//   - 1 active renewal per instrumen (HasActiveRenewal check)
//   - SoD: approver_id ≠ maker_id → SOD_VIOLATION (DEC-017)
//   - signatureMethod must be "JWT_STEP_UP"
//   - PPh re-verify server-side on approve (prevents client-side manipulation)
//   - bunga_bersih ≥ IDR 100.000 for POKOK_PLUS_BUNGA (BRD §6.2) — checked on create AND approve
//   - Periode buku must be OPEN at posting time
//   - EIR Newton-Raphson must converge (fail-safe: rollback + error on no-convergence)
//   - Approve side-effects in single tx: instrumen baru + matured lama + EIR schedule + jurnal
//   - Audit in same tx (DEC-018)
//   - Idempotency via sys.idempotency_key (DEC-021) — checked at handler/middleware layer

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Service owns renewal business logic.
type Service struct {
	repo              Repository
	poster            JurnalPoster
	instrumenCreator  InstrumenCreator
	eirWriter         EIRScheduleWriter
	audit             *audit.Writer
	logger            *slog.Logger
}

// NewService creates a new renewal Service.
func NewService(
	repo Repository,
	poster JurnalPoster,
	instrumenCreator InstrumenCreator,
	eirWriter EIRScheduleWriter,
	auditWriter *audit.Writer,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	if poster == nil {
		poster = NewNoopJurnalPoster(logger)
	}
	return &Service{
		repo:             repo,
		poster:           poster,
		instrumenCreator: instrumenCreator,
		eirWriter:        eirWriter,
		audit:            auditWriter,
		logger:           logger,
	}
}

// ─── CreateRenewal ────────────────────────────────────────────────────────────

// CreateRenewal validates, computes preview, and inserts a renewal in PENDING_APPROVAL state.
// S1-AC1..AC4.
func (s *Service) CreateRenewal(ctx context.Context, req CreateRenewalRequest) (*CreateRenewalResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	makerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	// ── Validate scalar inputs ───────────────────────────────────────
	if err := ValidateSkema(req.Skema); err != nil {
		return nil, err
	}
	if err := ValidateTenor(req.TenorBaruBulan); err != nil {
		return nil, err
	}
	if err := ValidateRate(req.RateBaruPersen); err != nil {
		return nil, err
	}
	tanggalEfektif, err := ParseDateStrict(req.TanggalEfektifBaru)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	// ── Fetch instrumen ───────────────────────────────────────────────
	inst, err := s.repo.GetInstrumenInfo(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: fetch instrumen: %w", err)
	}
	if inst == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Instrumen %s tidak ditemukan.", req.InstrumenID))
	}

	// ── Instrumen eligibility (S1-AC4) ───────────────────────────────
	if err := ValidateInstrumenEligibility(*inst); err != nil {
		return nil, err
	}

	// ── No active renewal check ────────────────────────────────────────
	hasActive, err := s.repo.HasActiveRenewal(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: check active renewal: %w", err)
	}
	if hasActive {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s sudah memiliki renewal aktif (PENDING_APPROVAL/APPROVED/POSTED). "+
				"Selesaikan atau reject renewal sebelumnya terlebih dahulu.", inst.KodeInstrumen),
			domainerrors.Detail{Field: "instrumenId", Rule: "no_active_renewal",
				Message: "1 active renewal per instrumen (partial unique constraint)."},
		)
	}

	// ── Periode buku check ────────────────────────────────────────────
	periode, err := s.repo.GetPeriodeByTanggal(ctx, tanggalEfektif)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: get periode: %w", err)
	}
	if periode == nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Tidak ada periode buku untuk tanggal %s.", req.TanggalEfektifBaru))
	}
	if periode.StatusPeriode != "OPEN" {
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			fmt.Sprintf("Periode buku untuk tanggal %s sudah %s. Renewal hanya untuk periode OPEN.",
				req.TanggalEfektifBaru, periode.StatusPeriode))
	}

	// ── Compute preview ───────────────────────────────────────────────
	skema := Skema(req.Skema)
	preview, err := ComputePreview(
		inst.Pokok,
		inst.RatePersen,
		inst.TanggalPenempatan,
		skema,
		req.RateBaruPersen,
		req.TenorBaruBulan,
		tanggalEfektif,
	)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: compute preview: %w", err)
	}

	// ── bunga_bersih minimum (S1, also re-checked at approve) ────────
	if err := ValidateBungaBersihMinimum(skema, preview.BungaBersih); err != nil {
		return nil, err
	}

	// ── Build Renewal entity ──────────────────────────────────────────
	renewalID := uuid.New()
	now := time.Now().UTC()
	periodeID := &periode.ID

	r := &Renewal{
		ID:                    renewalID,
		InstrumenLamaID:       req.InstrumenID,
		Skema:                 skema,
		TenorBaruBulan:        int16(req.TenorBaruBulan),
		RateBaruPersen:        req.RateBaruPersen,
		TanggalEfektifBaru:    tanggalEfektif,
		TanggalJatuhTempoBaru: preview.TanggalJatuhTempoBaru,
		PokokLama:             preview.PokokLama,
		PokokBaru:             preview.PokokBaru,
		BungaKotor:            preview.BungaKotor,
		PphAmount:             preview.Pph20pct,
		BungaBersih:           preview.BungaBersih,
		EirBaru:               &preview.EirBaru,
		Status:                StatusPendingApproval,
		MakerID:               makerID,
		PeriodeBulananID:      periodeID,
		CreatedAt:             now,
		CreatedBy:             makerID,
		UpdatedAt:             now,
		UpdatedBy:             makerID,
		RowVersion:            1,
		TenantID:              "TUGURE",
	}
	if req.RequestReason != "" {
		r.RequestReason = &req.RequestReason
	}

	// ── BEGIN tx ──────────────────────────────────────────────────────
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := s.repo.Insert(ctx, tx, r); err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: insert: %w", err)
	}

	// Audit RENEWAL.CREATED in-tx (DEC-018)
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "RENEWAL.CREATED",
			EntityType: "trx.renewal",
			EntityID:   renewalID,
			After: map[string]any{
				"instrumen_id":          req.InstrumenID.String(),
				"skema":                 req.Skema,
				"tenor_baru_bulan":      req.TenorBaruBulan,
				"rate_baru_persen":      req.RateBaruPersen.StringFixed(4),
				"pokok_baru":            preview.PokokBaru.StringFixed(4),
				"eir_baru":              preview.EirBaru.StringFixed(8),
				"tanggal_efektif_baru":  req.TanggalEfektifBaru,
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Service.CreateRenewal: commit: %w", err)
	}
	tx = nil // prevent defer rollback

	return &CreateRenewalResponse{
		RenewalID: renewalID.String(),
		Status:    string(StatusPendingApproval),
		Preview:   ToPreviewResponse(preview),
		NextStep:  "Menunggu approval ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker.",
	}, nil
}

// ─── Approve ─────────────────────────────────────────────────────────────────

// Approve executes the approve transition including all side-effects in one tx.
// S2-AC1..AC4, S3-AC1..AC4, S4-AC1..AC4, S5-AC1..AC4.
func (s *Service) Approve(ctx context.Context, renewalID uuid.UUID, req ApproveRenewalRequest) (*ApproveRenewalResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	approverID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	// ── Validate signatureMethod ──────────────────────────────────────
	if err := ValidateSignatureMethod(req.SignatureMethod); err != nil {
		return nil, err
	}

	// ── Fetch renewal ─────────────────────────────────────────────────
	renewal, err := s.repo.GetByID(ctx, renewalID)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: get renewal: %w", err)
	}
	if renewal == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Renewal %s tidak ditemukan.", renewalID))
	}

	// ── State check ───────────────────────────────────────────────────
	if !renewal.Status.CanApprove() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(renewal.Status), "POSTED")
	}

	// ── SoD: approver ≠ maker (S2-AC3) ───────────────────────────────
	if renewal.MakerID == approverID {
		// Advisory audit of SOD attempt
		s.logger.WarnContext(ctx, "Service.Approve: SoD violation attempt",
			"renewal_id", renewalID, "maker_id", renewal.MakerID, "approver_id", approverID)
		return nil, domainerrors.New(domainerrors.CodeSoDViolation,
			"maker tidak dapat menjadi approver untuk renewal yang sama (DEC-017).",
			domainerrors.Detail{Field: "actor.userId", Rule: "sod_approver_ne_maker",
				Message: "approver_id == maker_id."},
		)
	}

	// ── Fetch instrumen lama ──────────────────────────────────────────
	inst, err := s.repo.GetInstrumenInfo(ctx, renewal.InstrumenLamaID)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: get instrumen: %w", err)
	}
	if inst == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Instrumen lama %s tidak ditemukan.", renewal.InstrumenLamaID))
	}

	// ── Server re-verify kalkulasi ────────────────────────────────────
	skema := renewal.Skema
	tanggalEfektif := renewal.TanggalEfektifBaru

	bungaKotor := ComputeBungaKotor(inst.Pokok, inst.RatePersen, inst.TanggalPenempatan, tanggalEfektif)
	pph := ComputePPh(bungaKotor)

	// PPh consistency check (S4-AC3)
	if err := ValidatePphConsistency(renewal.PphAmount, bungaKotor); err != nil {
		return nil, err
	}

	bungaBersih := ComputeBungaBersih(bungaKotor, pph)

	// bunga_bersih minimum re-check (S2-AC2)
	if err := ValidateBungaBersihMinimum(skema, bungaBersih); err != nil {
		return nil, err
	}

	pokokBaru, err := ComputePokokBaru(skema, inst.Pokok, bungaBersih)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: compute pokok_baru: %w", err)
	}

	// ── EIR Newton-Raphson (S4-AC1..AC4) ─────────────────────────────
	cfs := BuildCashflowsAfterTax(pokokBaru, renewal.RateBaruPersen, int(renewal.TenorBaruBulan))
	initial := renewal.RateBaruPersen.Div(decimalHundred).Div(decimalTwelve)
	eirMonthly, err := NewtonRaphsonIRR(cfs, initial)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: EIR solver failed (%w) — renewal will NOT be posted. "+
			"Check cashflow inputs for renewal %s.", err, renewalID)
	}
	// Annualize: EIR_annual = (1 + r_monthly)^12 - 1
	rMonthlyF64, _ := eirMonthly.Float64()
	eirAnnualF64 := math.Pow(1+rMonthlyF64, 12) - 1
	eirAnnualDec := decimal.NewFromFloat(eirAnnualF64).RoundBank(8)

	// ── Periode buku check at posting time (S5-AC3) ──────────────────
	periode, err := s.repo.GetPeriodeByTanggal(ctx, tanggalEfektif)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: get periode: %w", err)
	}
	if periode == nil || periode.StatusPeriode != "OPEN" {
		statusStr := "tidak ditemukan"
		if periode != nil {
			statusStr = periode.StatusPeriode
		}
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			fmt.Sprintf("Periode buku untuk tanggal %s status=%s. Renewal tidak dapat diposting.",
				tanggalEfektif.Format("2006-01-02"), statusStr))
	}

	// ── BEGIN tx (all side-effects) ───────────────────────────────────
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	sigMethod := req.SignatureMethod

	// Step 2: UPDATE renewal status → APPROVED (interim)
	approveReason := req.Comment
	update := StatusUpdate{
		Status:         StatusApproved,
		ApproverID:     &approverID,
		ApproveReason:  &approveReason,
		SignatureMethod: &sigMethod,
		ApprovedAt:     &now,
		EirBaru:        &eirAnnualDec,
		UpdatedBy:      approverID,
		RowVersion:     renewal.RowVersion,
	}
	if err := s.repo.UpdateStatus(ctx, tx, renewalID, update); err != nil {
		return nil, fmt.Errorf("Service.Approve: set APPROVED: %w", err)
	}

	// Step 3+4: INSERT instrumen baru + UPDATE instrumen lama (S3)
	var instrumenBaruID uuid.UUID
	if s.instrumenCreator != nil {
		// Refresh renewal state to pass into creator
		renewal.PokokBaru = pokokBaru
		renewal.EirBaru = &eirAnnualDec
		instrumenBaruID, err = s.instrumenCreator.CreateInstrumenBaru(ctx, tx, *inst, renewal)
		if err != nil {
			return nil, fmt.Errorf("Service.Approve: create instrumen baru (S3): %w", err)
		}
		if err := s.instrumenCreator.MaturedInstrumenLama(ctx, tx, renewal.InstrumenLamaID, approverID); err != nil {
			return nil, fmt.Errorf("Service.Approve: matured instrumen lama (S3): %w", err)
		}
	} else {
		// No-op stub path: generate synthetic ID for test environments
		instrumenBaruID = uuid.New()
	}

	// Step 5: EIR schedule (S4)
	if s.eirWriter != nil {
		if err := s.eirWriter.InsertScheduleBaru(ctx, tx, instrumenBaruID, eirAnnualDec, tanggalEfektif, approverID); err != nil {
			return nil, fmt.Errorf("Service.Approve: insert EIR schedule baru (S4): %w", err)
		}
		if err := s.eirWriter.CloseScheduleLama(ctx, tx, renewal.InstrumenLamaID, tanggalEfektif, approverID); err != nil {
			return nil, fmt.Errorf("Service.Approve: close EIR schedule lama (S4): %w", err)
		}
	}

	// Step 6: POST jurnal RENEWAL_DEPOSITO (S5)
	postResult, postErr := s.poster.Post(ctx, tx, RenewalPostRequest{
		EventCode:       "RENEWAL_DEPOSITO",
		InstrumenLamaID: renewal.InstrumenLamaID,
		InstrumenBaruID: instrumenBaruID,
		RenewalID:       renewalID,
		PeriodeID:       periode.ID,
		TanggalEfektif:  tanggalEfektif,
		PokokLama:       inst.Pokok,
		PokokBaru:       pokokBaru,
		BungaBersih:     bungaBersih,
		PphAmount:       pph,
		ActorID:         approverID,
		TenantID:        "TUGURE",
	})
	if postErr != nil {
		return nil, fmt.Errorf("Service.Approve: jurnal post (S5): %w", postErr)
	}

	// Step 7: UPDATE renewal → POSTED, populate instrumen_baru_id + jurnal_header_id
	jurnalID := postResult.JurnalEntryID
	ibID := instrumenBaruID
	updatePosted := StatusUpdate{
		Status:          StatusPosted,
		InstrumenBaruID: &ibID,
		JurnalHeaderID:  &jurnalID,
		UpdatedBy:       approverID,
		RowVersion:      renewal.RowVersion + 1, // incremented by previous UpdateStatus
	}
	if err := s.repo.UpdateStatus(ctx, tx, renewalID, updatePosted); err != nil {
		return nil, fmt.Errorf("Service.Approve: set POSTED: %w", err)
	}

	// Step 8: Audit (in-tx)
	if s.audit != nil {
		eirF64, _ := eirAnnualDec.Float64()
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "RENEWAL.APPROVED",
			EntityType: "trx.renewal",
			EntityID:   renewalID,
			Before:     map[string]any{"status": string(StatusPendingApproval)},
			After: map[string]any{
				"status":           "APPROVED",
				"approver_id":      approverID.String(),
				"approve_reason":   req.Comment,
				"signature_method": req.SignatureMethod,
			},
		})
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "RENEWAL.POSTED",
			EntityType: "trx.renewal",
			EntityID:   renewalID,
			After: map[string]any{
				"status":             "POSTED",
				"instrumen_baru_id":  instrumenBaruID.String(),
				"jurnal_header_id":   jurnalID.String(),
				"eir_baru":           eirF64,
			},
		})
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "INSTRUMEN.CREATED",
			EntityType: "mst.instrumen",
			EntityID:   instrumenBaruID,
			After: map[string]any{
				"renewal_dari_instrumen_id": renewal.InstrumenLamaID.String(),
				"pokok_baru":                pokokBaru.StringFixed(4),
				"eir_baru":                  eirAnnualDec.StringFixed(8),
			},
		})
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "INSTRUMEN.MATURED",
			EntityType: "mst.instrumen",
			EntityID:   renewal.InstrumenLamaID,
			Before:     map[string]any{"status": inst.Status},
			After:      map[string]any{"status": "MATURED"},
		})
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "EIR.RECOMPUTED",
			EntityType: "ecl.amortisasi_schedule",
			EntityID:   instrumenBaruID,
			After: map[string]any{
				"instrumen_baru_id": instrumenBaruID.String(),
				"schedule_version":  1,
				"eir_baru":          eirAnnualDec.StringFixed(8),
				"effective_from":    tanggalEfektif.Format("2006-01-02"),
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Service.Approve: commit: %w", err)
	}
	tx = nil

	ibStr := instrumenBaruID.String()
	jurnalStr := jurnalID.String()
	return &ApproveRenewalResponse{
		RenewalID:       renewalID.String(),
		Status:          string(StatusPosted),
		InstrumenBaruID: &ibStr,
		JurnalEntryID:   &jurnalStr,
		ApprovedBy:      approverID.String(),
		ApprovedAt:      now.Format(time.RFC3339),
		Message: fmt.Sprintf("Renewal disetujui dan diposting. Instrumen baru %s dibuat.", ibStr),
	}, nil
}

// ─── Reject ───────────────────────────────────────────────────────────────────

// Reject transitions a renewal from PENDING_APPROVAL to REJECTED.
func (s *Service) Reject(ctx context.Context, renewalID uuid.UUID, req RejectRenewalRequest) (*RejectRenewalResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	approverID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	// Validate signatureMethod and comment
	if err := ValidateSignatureMethod(req.SignatureMethod); err != nil {
		return nil, err
	}
	if err := ValidateRejectComment(req.Comment); err != nil {
		return nil, err
	}

	renewal, err := s.repo.GetByID(ctx, renewalID)
	if err != nil {
		return nil, fmt.Errorf("Service.Reject: get renewal: %w", err)
	}
	if renewal == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Renewal %s tidak ditemukan.", renewalID))
	}

	if !renewal.Status.CanReject() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(renewal.Status), "REJECTED")
	}

	if renewal.MakerID == approverID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation,
			"maker tidak dapat menjadi rejecter untuk renewal yang sama (DEC-017).",
			domainerrors.Detail{Field: "actor.userId", Rule: "sod_approver_ne_maker",
				Message: "approver_id == maker_id."},
		)
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.Reject: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	rejectReason := req.Comment
	sigMethod := req.SignatureMethod
	update := StatusUpdate{
		Status:         StatusRejected,
		ApproverID:     &approverID,
		RejectReason:   &rejectReason,
		SignatureMethod: &sigMethod,
		ApprovedAt:     &now,
		UpdatedBy:      approverID,
		RowVersion:     renewal.RowVersion,
	}
	if err := s.repo.UpdateStatus(ctx, tx, renewalID, update); err != nil {
		return nil, fmt.Errorf("Service.Reject: update status: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "RENEWAL.REJECTED",
			EntityType: "trx.renewal",
			EntityID:   renewalID,
			Before:     map[string]any{"status": string(renewal.Status)},
			After: map[string]any{
				"status":           "REJECTED",
				"approver_id":      approverID.String(),
				"reject_reason":    req.Comment,
				"signature_method": req.SignatureMethod,
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Service.Reject: commit: %w", err)
	}
	tx = nil

	return &RejectRenewalResponse{
		RenewalID:  renewalID.String(),
		Status:     string(StatusRejected),
		RejectedBy: approverID.String(),
		RejectedAt: now.Format(time.RFC3339),
		Comment:    req.Comment,
	}, nil
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

// GetDetail fetches one renewal by ID, recomputes preview, and returns Detail.
func (s *Service) GetDetail(ctx context.Context, id uuid.UUID) (*Detail, error) {
	renewal, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Service.GetDetail: %w", err)
	}
	if renewal == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Renewal %s tidak ditemukan.", id))
	}

	inst, _ := s.repo.GetInstrumenInfo(ctx, renewal.InstrumenLamaID)

	var preview PreviewResult
	if inst != nil {
		preview, _ = ComputePreview(
			inst.Pokok,
			inst.RatePersen,
			inst.TanggalPenempatan,
			renewal.Skema,
			renewal.RateBaruPersen,
			int(renewal.TenorBaruBulan),
			renewal.TanggalEfektifBaru,
		)
	}
	// Fallback to stored values when preview recompute fails or inst nil
	if preview.PokokLama.IsZero() {
		preview = PreviewResult{
			PokokLama:             renewal.PokokLama,
			BungaKotor:            renewal.BungaKotor,
			Pph20pct:              renewal.PphAmount,
			BungaBersih:           renewal.BungaBersih,
			PokokBaru:             renewal.PokokBaru,
			TanggalJatuhTempoBaru: renewal.TanggalJatuhTempoBaru,
		}
		if renewal.EirBaru != nil {
			preview.EirBaru = *renewal.EirBaru
		}
	}

	instrumenKode := ""
	if inst != nil {
		instrumenKode = inst.KodeInstrumen
	}
	d := ToDetail(renewal, instrumenKode, preview)
	return &d, nil
}

// ─── GetList ──────────────────────────────────────────────────────────────────

// GetList returns paginated renewal rows.
func (s *Service) GetList(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Renewal, bool, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.List(ctx, q, cursor, limit)
}

// GetPreview recomputes and returns the preview for an existing renewal.
func (s *Service) GetPreview(ctx context.Context, renewalID uuid.UUID) (*PreviewResponse, error) {
	d, err := s.GetDetail(ctx, renewalID)
	if err != nil {
		return nil, err
	}
	return &d.Preview, nil
}
