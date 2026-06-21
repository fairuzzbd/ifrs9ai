package penjualan

// service.go — Service owns all penjualan business logic.
// TX boundary lives here; repos never open transactions.
//
// Business rules enforced:
//   - Instrumen must be ACTIVE with klasifikasi_locked=TRUE
//   - 1 active disposal per instrumen (HasActivePenjualan check)
//   - SoD: approver_id ≠ maker_id → SOD_VIOLATION (DEC-017)
//   - signatureMethod must be "JWT_STEP_UP"
//   - Periode buku must be OPEN at posting time
//   - OCI recycling: FVOCI debt → REKLAS_OCI_PL; FVOCI_ELECTION → no recycle (§B5.7.1)
//   - BM frequency: warn + block thresholds from sys.config
//   - Approve side-effects in single tx: OCI + BM + jurnal + instrumen update
//   - Audit in same tx (DEC-018)
//   - Idempotency-Key mandatory on mutating endpoints — DEC-021

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
	"blips-ifrs9.tugu-re.com/internal/common/listquery"
)

// Service owns penjualan business logic.
type Service struct {
	repo            Repository
	poster          JurnalPoster
	instrumenUpdate InstrumenUpdater
	riskNotifier    RiskNotifier
	audit           *audit.Writer
	logger          *slog.Logger
}

// NewService creates a new penjualan Service.
func NewService(
	repo Repository,
	poster JurnalPoster,
	instrumenUpdate InstrumenUpdater,
	riskNotifier RiskNotifier,
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
		repo:            repo,
		poster:          poster,
		instrumenUpdate: instrumenUpdate,
		riskNotifier:    riskNotifier,
		audit:           auditWriter,
		logger:          logger,
	}
}

// ─── CreatePenjualan ──────────────────────────────────────────────────────────

// CreatePenjualan validates inputs, computes preview, and inserts PENDING_APPROVAL.
// S1-AC1..AC4.
func (s *Service) CreatePenjualan(ctx context.Context, req CreatePenjualanRequest) (*CreatePenjualanResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	makerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	// ── Scalar validation ────────────────────────────────────────────
	if err := ValidateDisposalType(req.JenisDisposal); err != nil {
		return nil, err
	}
	if err := ValidateHarga(req.HargaJualPerUnit); err != nil {
		return nil, err
	}
	if err := ValidateQtyPositive(req.QtyTerjual); err != nil {
		return nil, err
	}
	tanggalEksekusi, err := ParseDateStrict(req.TanggalEksekusi)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	// ── Fetch instrumen ───────────────────────────────────────────────
	inst, err := s.repo.GetInstrumenInfo(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: fetch instrumen: %w", err)
	}
	if inst == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Instrumen %s tidak ditemukan.", req.InstrumenID))
	}

	// ── Instrumen eligibility ────────────────────────────────────────
	if err := ValidateInstrumenEligibility(*inst); err != nil {
		return nil, err
	}

	// ── Qty vs holding validation ────────────────────────────────────
	jenis := DisposalType(req.JenisDisposal)
	if err := ValidateQtyVsHolding(req.QtyTerjual, inst.QtyHolding, jenis); err != nil {
		return nil, err
	}

	// ── No active disposal check ─────────────────────────────────────
	hasActive, err := s.repo.HasActivePenjualan(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: check active: %w", err)
	}
	if hasActive {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s sudah memiliki disposal aktif (PENDING_APPROVAL/APPROVED). "+
				"Selesaikan atau reject terlebih dahulu.", inst.KodeInstrumen))
	}

	// ── Periode buku check ───────────────────────────────────────────
	periode, err := s.repo.GetPeriodeByTanggal(ctx, tanggalEksekusi)
	if err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: get periode: %w", err)
	}
	if periode == nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Tidak ada periode buku untuk tanggal %s.", req.TanggalEksekusi))
	}
	if periode.StatusPeriode != "OPEN" {
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			fmt.Sprintf("Periode buku untuk tanggal %s sudah %s. Penjualan hanya untuk periode OPEN.",
				req.TanggalEksekusi, periode.StatusPeriode))
	}

	// ── Klasifikasi routing (validates locked) ────────────────────────
	klasifikasi := KlasifikasiPSAK71(inst.KlasifikasiPSAK71)
	routing, err := ResolveJurnalEventCode(klasifikasi, inst.KlasifikasiLocked, jenis)
	if err != nil {
		return nil, err
	}

	// ── Compute preview ──────────────────────────────────────────────
	preview, err := s.computePreview(ctx, req.InstrumenID, *inst, jenis, routing, req.HargaJualPerUnit, req.QtyTerjual)
	if err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: compute preview: %w", err)
	}

	// ── Build entity ─────────────────────────────────────────────────
	penjualanID := uuid.New()
	now := time.Now().UTC()
	periodeID := &periode.ID
	eventCode := joinEventCodes(routing.EventCodes)

	p := &Penjualan{
		ID:                  penjualanID,
		InstrumenID:         req.InstrumenID,
		JenisDisposal:       jenis,
		QtyTerjual:          req.QtyTerjual,
		QtyHoldingPre:       inst.QtyHolding,
		HargaJualPerUnit:    req.HargaJualPerUnit,
		Proceed:             preview.ProceedIDR,
		CostBasis:           preview.CostBasis,
		RealizedGL:          preview.RealizedGL,
		OCIRecycled:         preview.OCIRecycled,
		KlasifikasiSnapshot: klasifikasi,
		JurnalEventCode:     &eventCode,
		TanggalEksekusi:     tanggalEksekusi,
		Status:              StatusPendingApproval,
		MakerID:             makerID,
		PeriodeBulananID:    periodeID,
		CreatedAt:           now,
		CreatedBy:           makerID,
		UpdatedAt:           now,
		UpdatedBy:           makerID,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}
	if oci := preview.OCIRecycled; oci != nil {
		cum, _ := s.repo.GetOCICumulativeByInstrumen(ctx, req.InstrumenID)
		p.OCICumulativeTotal = &cum
	}

	// ── BEGIN tx ──────────────────────────────────────────────────────
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := s.repo.Insert(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: insert: %w", err)
	}

	// Audit PENJUALAN.CREATED in-tx (DEC-018)
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "PENJUALAN.CREATED",
			EntityType: "trx.penjualan",
			EntityID:   penjualanID,
			After: map[string]any{
				"instrumen_id":       req.InstrumenID.String(),
				"jenis_disposal":     req.JenisDisposal,
				"qty_terjual":        req.QtyTerjual.StringFixed(8),
				"harga_jual":         req.HargaJualPerUnit.StringFixed(4),
				"proceed_idr":        preview.ProceedIDR.StringFixed(4),
				"cost_basis":         preview.CostBasis.StringFixed(4),
				"realized_gl":        preview.RealizedGL.StringFixed(4),
				"klasifikasi":        string(klasifikasi),
				"tanggal_eksekusi":   req.TanggalEksekusi,
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Service.CreatePenjualan: commit: %w", err)
	}
	tx = nil

	return &CreatePenjualanResponse{
		PenjualanID: penjualanID.String(),
		Status:      string(StatusPendingApproval),
		Preview:     ToPreviewResponse(preview),
		NextStep:    "Menunggu approval ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker.",
	}, nil
}

// ─── Approve ─────────────────────────────────────────────────────────────────

// Approve executes the approve transition including all side-effects in one tx.
// S2-AC1..AC4, S3-AC1..AC4, S4-AC1..AC4, S5-AC1..AC4.
func (s *Service) Approve(ctx context.Context, penjualanID uuid.UUID, req ApprovePenjualanRequest) (*ApprovePenjualanResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	approverID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	// ── signatureMethod ──────────────────────────────────────────────
	if err := ValidateSignatureMethod(req.SignatureMethod); err != nil {
		return nil, err
	}

	// ── Fetch penjualan ──────────────────────────────────────────────
	pj, err := s.repo.GetByID(ctx, penjualanID)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: get penjualan: %w", err)
	}
	if pj == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Penjualan %s tidak ditemukan.", penjualanID))
	}

	// ── State check ───────────────────────────────────────────────────
	if !pj.Status.CanApprove() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(pj.Status), "POSTED")
	}

	// ── SoD (S2-AC2) ─────────────────────────────────────────────────
	if pj.MakerID == approverID {
		s.logger.WarnContext(ctx, "Service.Approve: SoD violation attempt",
			"penjualan_id", penjualanID, "maker_id", pj.MakerID, "approver_id", approverID)
		return nil, domainerrors.New(domainerrors.CodeSoDViolation,
			"maker tidak dapat menjadi approver untuk penjualan yang sama (DEC-017).",
			domainerrors.Detail{Field: "actor.userId", Rule: "sod_approver_ne_maker",
				Message: "approver_id == maker_id."},
		)
	}

	// ── Fetch instrumen ───────────────────────────────────────────────
	inst, err := s.repo.GetInstrumenInfo(ctx, pj.InstrumenID)
	if err != nil || inst == nil {
		return nil, fmt.Errorf("Service.Approve: get instrumen: %w", err)
	}

	// ── Periode buku check at posting time (S2-AC3) ───────────────────
	periode, err := s.repo.GetPeriodeByTanggal(ctx, pj.TanggalEksekusi)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: get periode: %w", err)
	}
	if periode == nil || periode.StatusPeriode != "OPEN" {
		statusStr := "tidak ditemukan"
		if periode != nil {
			statusStr = periode.StatusPeriode
		}
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			fmt.Sprintf("Periode buku untuk tanggal %s status=%s. Posting penjualan tidak bisa dilakukan.",
				pj.TanggalEksekusi.Format("2006-01-02"), statusStr))
	}

	// ── Server re-verify cost_basis ───────────────────────────────────
	routing, err := ResolveJurnalEventCode(pj.KlasifikasiSnapshot, inst.KlasifikasiLocked, pj.JenisDisposal)
	if err != nil {
		return nil, err
	}
	preview, err := s.computePreview(ctx, pj.InstrumenID, *inst, pj.JenisDisposal, routing,
		pj.HargaJualPerUnit, pj.QtyTerjual)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: recompute preview: %w", err)
	}

	// ── BM frequency check (HTC only, S4) ────────────────────────────
	warnThreshold, blockThreshold, err := s.repo.GetBMConfigThresholds(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.Approve: get BM config: %w", err)
	}

	var bmPct decimal.Decimal
	var bmWarn, bmBlock bool
	if inst.BusinessModel == "HTC" {
		cumIDR, err2 := s.repo.GetRolling12mDisposalIDR(ctx, inst.PortofolioID)
		if err2 != nil {
			return nil, fmt.Errorf("Service.Approve: get rolling 12m disposal: %w", err2)
		}
		portNilai, err2 := s.repo.GetPortofolioNilai(ctx, inst.PortofolioID)
		if err2 != nil {
			return nil, fmt.Errorf("Service.Approve: get portofolio nilai: %w", err2)
		}
		if !portNilai.IsZero() {
			bmPct, _ = ComputeBMFrequency(cumIDR, preview.ProceedIDR, portNilai)
			bmWarn, bmBlock = ValidateBMThresholds(bmPct, warnThreshold, blockThreshold)
		}
	}

	// ── BEGIN tx ──────────────────────────────────────────────────────
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
	approveComment := req.Comment

	// Step 1: UPDATE penjualan → APPROVED (interim)
	u := StatusUpdate{
		Status:         StatusApproved,
		ApproverID:     &approverID,
		ApproveComment: &approveComment,
		SignatureMethod: &sigMethod,
		ApprovedAt:     &now,
		BMViolationRisk: bmWarn,
		UpdatedBy:      approverID,
		RowVersion:     pj.RowVersion,
	}
	if bmBlock || bmWarn {
		bmPctCopy := bmPct
		u.BMViolationPct = &bmPctCopy
	}
	if err := s.repo.UpdateStatus(ctx, tx, penjualanID, u); err != nil {
		return nil, fmt.Errorf("Service.Approve: set APPROVED: %w", err)
	}

	// Audit PENJUALAN.APPROVED in-tx
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "PENJUALAN.APPROVED",
			EntityType: "trx.penjualan",
			EntityID:   penjualanID,
			Before:     map[string]any{"status": string(StatusPendingApproval)},
			After: map[string]any{
				"status":           "APPROVED",
				"approver_id":      approverID.String(),
				"signature_method": req.SignatureMethod,
			},
		})
	}

	// ── BM hard block path ────────────────────────────────────────────
	if bmBlock {
		// Update to PENDING_BM_REVIEW (not POSTED)
		uBM := StatusUpdate{
			Status:          StatusPendingBMReview,
			BMViolationRisk: true,
			BMViolationPct:  &bmPct,
			UpdatedBy:       approverID,
			RowVersion:      pj.RowVersion + 1,
		}
		if err := s.repo.UpdateStatus(ctx, tx, penjualanID, uBM); err != nil {
			return nil, fmt.Errorf("Service.Approve: set PENDING_BM_REVIEW: %w", err)
		}
		// Audit BM block in-tx
		if s.audit != nil {
			_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
				Action:     "PENJUALAN.BM_FREQUENCY_FLAG",
				EntityType: "trx.penjualan",
				EntityID:   penjualanID,
				After: map[string]any{
					"portofolio_id":    inst.PortofolioID.String(),
					"pct_terjual":      bmPct.StringFixed(4),
					"threshold_block":  blockThreshold.StringFixed(4),
					"flag":             "BM_VIOLATION_BLOCK",
				},
			})
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.Approve: commit BM block: %w", err)
		}
		tx = nil
		// Notify ROLE-RISK (async, best-effort — does not block tx)
		if s.riskNotifier != nil {
			_ = s.riskNotifier.NotifyBMViolation(ctx, inst.PortofolioID, penjualanID, bmPct, true)
		}
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Penjualan %s menyebabkan cumulative disposal 12-bulan = %s%% (hard limit: %s%%). "+
				"Approval ROLE-RISK diperlukan sebelum penjualan ini bisa diposting. Status → PENDING_BM_REVIEW.",
				penjualanID, bmPct.StringFixed(2), blockThreshold.StringFixed(2)),
		)
	}

	// ── OCI recycling (S3) ────────────────────────────────────────────
	var ociAuditAction string
	if routing.RecycleOCI {
		ociAuditAction = "PENJUALAN.OCI_RECYCLED"
		if s.audit != nil {
			ociDir := "GAIN"
			if preview.OCIRecycled != nil && preview.OCIRecycled.IsNegative() {
				ociDir = "LOSS"
			}
			ociAmt := "0"
			if preview.OCIRecycled != nil {
				ociAmt = preview.OCIRecycled.StringFixed(4)
			}
			ociCumulative := "0"
			if pj.OCICumulativeTotal != nil {
				ociCumulative = pj.OCICumulativeTotal.StringFixed(4)
			}
			_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
				Action:     ociAuditAction,
				EntityType: "trx.penjualan",
				EntityID:   penjualanID,
				After: map[string]any{
					"instrumen_id":   pj.InstrumenID.String(),
					"oci_cumulative": ociCumulative,
					"oci_recycled":   ociAmt,
					"direction":      ociDir,
					"klasifikasi":    string(pj.KlasifikasiSnapshot),
				},
			})
		}
	} else if routing.NoRecyclingFlag {
		ociAuditAction = "PENJUALAN.OCI_NO_RECYCLE"
		if s.audit != nil {
			ociCumStr := "0"
			if pj.OCICumulativeTotal != nil {
				ociCumStr = pj.OCICumulativeTotal.StringFixed(4)
			}
			_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
				Action:     ociAuditAction,
				EntityType: "trx.penjualan",
				EntityID:   penjualanID,
				After: map[string]any{
					"instrumen_id":  pj.InstrumenID.String(),
					"oci_cumulative": ociCumStr,
					"reason":        "FVOCI_ELECTION_NO_RECYCLE_PSAK71_B5.7.1",
				},
			})
		}
	}

	// ── BM warn (non-blocking — log + notify, then continue) ─────────
	if bmWarn {
		if s.audit != nil {
			_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
				Action:     "PENJUALAN.BM_FREQUENCY_FLAG",
				EntityType: "trx.penjualan",
				EntityID:   penjualanID,
				After: map[string]any{
					"portofolio_id":   inst.PortofolioID.String(),
					"pct_terjual":     bmPct.StringFixed(4),
					"threshold_warn":  warnThreshold.StringFixed(4),
					"flag":            "BM_VIOLATION_RISK",
				},
			})
		}
		if s.riskNotifier != nil {
			_ = s.riskNotifier.NotifyBMViolation(ctx, inst.PortofolioID, penjualanID, bmPct, false)
		}
	}

	// ── Post jurnal via P5-M2 (S5) ────────────────────────────────────
	postResult, postErr := s.poster.Post(ctx, tx, PenjualanPostRequest{
		EventCodes:          routing.EventCodes,
		InstrumenID:         pj.InstrumenID,
		PenjualanID:         penjualanID,
		PeriodeID:           periode.ID,
		TanggalEksekusi:     pj.TanggalEksekusi,
		KlasifikasiSnapshot: pj.KlasifikasiSnapshot,
		JenisDisposal:       pj.JenisDisposal,
		ProceedIDR:          preview.ProceedIDR,
		CostBasis:           preview.CostBasis,
		RealizedGL:          preview.RealizedGL,
		OCIRecycled:         preview.OCIRecycled,
		QtyTerjual:          pj.QtyTerjual,
		ActorID:             approverID,
		TenantID:            "TUGURE",
	})
	if postErr != nil {
		return nil, fmt.Errorf("Service.Approve: jurnal post (S5): %w", postErr)
	}

	// ── Instrumen update / derecognition (S5) ─────────────────────────
	var qtyHoldingPost *decimal.Decimal
	instrumenStatusAfter := "ACTIVE"
	if pj.JenisDisposal == DisposalFull {
		if s.instrumenUpdate != nil {
			if err := s.instrumenUpdate.SetDisposed(ctx, tx, pj.InstrumenID, approverID); err != nil {
				return nil, fmt.Errorf("Service.Approve: set disposed (S5): %w", err)
			}
		}
		instrumenStatusAfter = "DISPOSED"
	} else {
		if s.instrumenUpdate != nil {
			remaining, err := s.instrumenUpdate.UpdateQty(ctx, tx, pj.InstrumenID, pj.QtyTerjual, approverID)
			if err != nil {
				return nil, fmt.Errorf("Service.Approve: update qty (S5): %w", err)
			}
			qtyHoldingPost = &remaining
		} else {
			r := pj.QtyHoldingPre.Sub(pj.QtyTerjual)
			qtyHoldingPost = &r
		}
	}

	// ── UPDATE penjualan → POSTED ─────────────────────────────────────
	jurnalID := postResult.JurnalEntryID
	eventCode := joinEventCodes(routing.EventCodes)
	uPosted := StatusUpdate{
		Status:               StatusPosted,
		JurnalHeaderID:       &jurnalID,
		QtyHoldingPost:       qtyHoldingPost,
		OCIRecycled:          preview.OCIRecycled,
		BMViolationRisk:      bmWarn,
		JurnalEventCode:      &eventCode,
		InstrumenStatusAfter: &instrumenStatusAfter,
		UpdatedBy:            approverID,
		RowVersion:           pj.RowVersion + 1,
	}
	if bmWarn {
		uPosted.BMViolationPct = &bmPct
	}
	if err := s.repo.UpdateStatus(ctx, tx, penjualanID, uPosted); err != nil {
		return nil, fmt.Errorf("Service.Approve: set POSTED: %w", err)
	}

	// ── Audit DERECOGNIZED + POSTED in-tx ────────────────────────────
	if s.audit != nil {
		qtyHoldingAfterStr := "0"
		if qtyHoldingPost != nil {
			qtyHoldingAfterStr = qtyHoldingPost.StringFixed(8)
		}
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "PENJUALAN.DERECOGNIZED",
			EntityType: "mst.instrumen",
			EntityID:   pj.InstrumenID,
			After: map[string]any{
				"instrumen_id":          pj.InstrumenID.String(),
				"jenis_disposal":        string(pj.JenisDisposal),
				"qty_terjual":           pj.QtyTerjual.StringFixed(8),
				"qty_holding_after":     qtyHoldingAfterStr,
				"instrumen_status_after": instrumenStatusAfter,
			},
		})
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "PENJUALAN.POSTED",
			EntityType: "trx.penjualan",
			EntityID:   penjualanID,
			After: map[string]any{
				"status":          "POSTED",
				"jurnal_entry_id": jurnalID.String(),
				"bm_warning":      bmWarn,
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Service.Approve: commit: %w", err)
	}
	tx = nil

	// Build response
	warnings := []string{}
	if routing.NoRecyclingFlag {
		warnings = append(warnings, CodePenjualanFVOCIElectionNoRecyclingWarn)
	}

	jurnalStr := jurnalID.String()
	var ociRecStr *string
	if preview.OCIRecycled != nil {
		s2 := preview.OCIRecycled.StringFixed(4)
		ociRecStr = &s2
	}

	return &ApprovePenjualanResponse{
		PenjualanID:          penjualanID.String(),
		Status:               string(StatusPosted),
		JurnalEntryID:        &jurnalStr,
		InstrumenStatusAfter: &instrumenStatusAfter,
		ApprovedBy:           approverID.String(),
		ApprovedAt:           now.Format(time.RFC3339),
		OCIRecycled:          ociRecStr,
		NoRecyclingNote:      preview.NoRecyclingNote,
		BMViolationRisk:      bmWarn,
		Warnings:             warnings,
	}, nil
}

// ─── Reject ───────────────────────────────────────────────────────────────────

// Reject transitions a penjualan from PENDING_APPROVAL to REJECTED.
func (s *Service) Reject(ctx context.Context, penjualanID uuid.UUID, req RejectPenjualanRequest) (*RejectPenjualanResponse, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	approverID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	if err := ValidateSignatureMethod(req.SignatureMethod); err != nil {
		return nil, err
	}
	if err := ValidateRejectReason(req.Reason); err != nil {
		return nil, err
	}

	pj, err := s.repo.GetByID(ctx, penjualanID)
	if err != nil {
		return nil, fmt.Errorf("Service.Reject: get penjualan: %w", err)
	}
	if pj == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Penjualan %s tidak ditemukan.", penjualanID))
	}
	if !pj.Status.CanReject() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(pj.Status), "REJECTED")
	}
	if pj.MakerID == approverID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation,
			"maker tidak dapat menjadi rejecter untuk penjualan yang sama (DEC-017).",
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
	rejectReason := req.Reason
	sigMethod := req.SignatureMethod
	u := StatusUpdate{
		Status:         StatusRejected,
		ApproverID:     &approverID,
		RejectReason:   &rejectReason,
		SignatureMethod: &sigMethod,
		ApprovedAt:     &now,
		UpdatedBy:      approverID,
		RowVersion:     pj.RowVersion,
	}
	if err := s.repo.UpdateStatus(ctx, tx, penjualanID, u); err != nil {
		return nil, fmt.Errorf("Service.Reject: update status: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "PENJUALAN.REJECTED",
			EntityType: "trx.penjualan",
			EntityID:   penjualanID,
			Before:     map[string]any{"status": string(pj.Status)},
			After: map[string]any{
				"status":           "REJECTED",
				"approver_id":      approverID.String(),
				"reject_reason":    req.Reason,
				"signature_method": req.SignatureMethod,
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("Service.Reject: commit: %w", err)
	}
	tx = nil

	return &RejectPenjualanResponse{
		PenjualanID: penjualanID.String(),
		Status:      string(StatusRejected),
		RejectedBy:  approverID.String(),
		RejectedAt:  now.Format(time.RFC3339),
		Reason:      req.Reason,
	}, nil
}

// ─── GetDetail ────────────────────────────────────────────────────────────────

// GetDetail fetches one penjualan by ID and returns Detail with recomputed preview.
func (s *Service) GetDetail(ctx context.Context, id uuid.UUID) (*Detail, error) {
	pj, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Service.GetDetail: %w", err)
	}
	if pj == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Penjualan %s tidak ditemukan.", id))
	}

	inst, _ := s.repo.GetInstrumenInfo(ctx, pj.InstrumenID)

	var preview PreviewResult
	if inst != nil {
		routing, _ := ResolveJurnalEventCode(pj.KlasifikasiSnapshot, inst.KlasifikasiLocked, pj.JenisDisposal)
		preview, _ = s.computePreview(ctx, pj.InstrumenID, *inst, pj.JenisDisposal, routing,
			pj.HargaJualPerUnit, pj.QtyTerjual)
	}
	if preview.ProceedIDR.IsZero() {
		// Fallback to stored values
		preview = PreviewResult{
			KlasifikasiPSAK71: pj.KlasifikasiSnapshot,
			ProceedIDR:        pj.Proceed,
			CostBasis:         pj.CostBasis,
			RealizedGL:        pj.RealizedGL,
			OCIRecycled:       pj.OCIRecycled,
		}
	}

	instrumenKode := ""
	if inst != nil {
		instrumenKode = inst.KodeInstrumen
	}
	d := ToDetail(pj, instrumenKode, preview)
	return &d, nil
}

// GetPreview recomputes and returns the preview for an existing penjualan.
func (s *Service) GetPreview(ctx context.Context, id uuid.UUID) (*PreviewResponse, error) {
	d, err := s.GetDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	return &d.Preview, nil
}

// GetList returns paginated penjualan rows.
func (s *Service) GetList(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*Penjualan, bool, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.List(ctx, q, cursor, limit)
}

// ListBMAlerts returns instrumen with BM violation risk.
// Thresholds are read from sys.config_param so the alert table reflects live ALCO-configured values.
func (s *Service) ListBMAlerts(ctx context.Context) ([]*BMAlertItem, error) {
	warnT, blockT, err := s.repo.GetBMConfigThresholds(ctx)
	if err != nil {
		s.logger.WarnContext(ctx, "ListBMAlerts: BM config unavailable, using defaults",
			"error", err)
		warnT = decimal.NewFromInt(5)
		blockT = decimal.NewFromInt(10)
	}
	return s.repo.ListBMAlerts(ctx, warnT, blockT)
}

// ─── Private helpers ──────────────────────────────────────────────────────────

// computePreview centralizes preview computation for create + approve re-verify.
func (s *Service) computePreview(
	ctx context.Context,
	instrumenID uuid.UUID,
	inst InstrumenInfo,
	jenis DisposalType,
	routing RoutingResult,
	hargaJual decimal.Decimal,
	qtyTerjual decimal.Decimal,
) (PreviewResult, error) {
	proceed := ComputeProceed(hargaJual, qtyTerjual)

	// Determine total cost_basis based on klasifikasi
	var totalCostBasis decimal.Decimal
	switch inst.KlasifikasiPSAK71 {
	case string(KlasifikasiFVTPL):
		// FVTPL: use MTM fair value (oci_cumulative not applicable; use proceed as fallback if no MTM)
		// For FVTPL, carrying amount = latest fair value from trx.mtm
		mtmCarrying, _, err := s.repo.GetAmortizedCarryingByInstrumen(ctx, instrumenID, time.Now())
		if err != nil || mtmCarrying.IsZero() {
			// Fallback: use harga_perolehan if MTM unavailable
			totalCostBasis = inst.HargaPerolehan
		} else {
			totalCostBasis = mtmCarrying
		}
	case string(KlasifikasiFVOCIElection):
		// FVOCI_ELECTION: cost_basis = original acquisition cost
		totalCostBasis = inst.HargaPerolehan
	default:
		// AC, FVOCI, POCI: amortized carrying from ecl.amortisasi_schedule.
		// For Stage 3: carrying is net (gross - sealed ECL) per PSAK 71 §5.4.1(b).
		carrying, stageUsed, err := s.repo.GetAmortizedCarryingByInstrumen(ctx, instrumenID, time.Now())
		if err != nil || carrying.IsZero() {
			// Fallback to harga_perolehan
			totalCostBasis = inst.HargaPerolehan
		} else {
			totalCostBasis = carrying
		}
		if stageUsed == 0 && err == nil {
			s.logger.WarnContext(ctx, "GetAmortizedCarryingByInstrumen: no sealed ECL run found, using gross carrying",
				"instrumen_id", instrumenID)
		}
	}

	costBasis, err := ComputeCostBasis(totalCostBasis, qtyTerjual, inst.QtyHolding, jenis)
	if err != nil {
		return PreviewResult{}, fmt.Errorf("computePreview: cost_basis: %w", err)
	}

	realizedGL := ComputeRealizedGL(proceed, costBasis)

	// OCI recycled (FVOCI debt only)
	var ociRecycled *decimal.Decimal
	if routing.RecycleOCI {
		ociCumulative, _ := s.repo.GetOCICumulativeByInstrumen(ctx, instrumenID)
		ociRecycled, err = ComputeOCIRecycle(true, ociCumulative, qtyTerjual, inst.QtyHolding, jenis)
		if err != nil {
			return PreviewResult{}, fmt.Errorf("computePreview: OCI recycle: %w", err)
		}
	}

	// No-recycling note (FVOCI_ELECTION)
	var noRecyclingNote *string
	if routing.NoRecyclingFlag {
		note := NoRecyclingNoteText(realizedGL)
		noRecyclingNote = &note
	}

	// BM frequency impact (HTC only, informational in preview)
	var bmFreqImpactPct *decimal.Decimal
	var bmFreqWarning *string
	if inst.BusinessModel == "HTC" {
		cumIDR, _ := s.repo.GetRolling12mDisposalIDR(ctx, inst.PortofolioID)
		portNilai, _ := s.repo.GetPortofolioNilai(ctx, inst.PortofolioID)
		if !portNilai.IsZero() {
			pct, _ := ComputeBMFrequency(cumIDR, proceed, portNilai)
			bmFreqImpactPct = &pct
			warnT, blockT, configErr := s.repo.GetBMConfigThresholds(ctx)
			if configErr != nil {
				// Defaults match seed values; if ALCO override active and config read fails,
				// preview may show stale thresholds — flagged in log.
				warnT = decimal.NewFromInt(5)
				blockT = decimal.NewFromInt(10)
				s.logger.WarnContext(ctx, "BM config unavailable in preview, using defaults",
					"error", configErr)
			}
			bmWarn, bmBlock := ValidateBMThresholds(pct, warnT, blockT)
			if bmBlock {
				msg := fmt.Sprintf("Perhatian: disposal ini akan menyebabkan BM HTC disposal %.2f%% (block threshold: %.2f%%).",
					pct.InexactFloat64(), blockT.InexactFloat64())
				bmFreqWarning = &msg
			} else if bmWarn {
				msg := fmt.Sprintf("Peringatan: disposal ini akan menyebabkan BM HTC disposal %.2f%% (warn threshold: %.2f%%).",
					pct.InexactFloat64(), warnT.InexactFloat64())
				bmFreqWarning = &msg
			}
		}
	}

	return PreviewResult{
		KlasifikasiPSAK71: KlasifikasiPSAK71(inst.KlasifikasiPSAK71),
		ProceedIDR:        proceed,
		CostBasis:         costBasis,
		RealizedGL:        realizedGL,
		OCIRecycled:       ociRecycled,
		NoRecyclingNote:   noRecyclingNote,
		BMFreqImpactPct:   bmFreqImpactPct,
		BMFreqWarning:     bmFreqWarning,
	}, nil
}

// joinEventCodes joins event codes with comma separator for storage.
func joinEventCodes(codes []string) string {
	result := ""
	for i, c := range codes {
		if i > 0 {
			result += ","
		}
		result += c
	}
	return result
}
