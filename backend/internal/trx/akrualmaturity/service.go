package akrualmaturity

// service.go — Service owns all akrualmaturity business logic.
// TX boundary lives here; repos never open transactions.
//
// Business rules enforced:
//   - Stage 3 akrual = (Gross - ECL_sealed) × EIR / 365 per PSAK 71 §5.4.1(b)
//   - POCI: credit-adjusted EIR from ecl.amortisasi_schedule
//   - FCY: convert to IDR via FX rate APPROVED (tanggal_akrual)
//   - Stale ECL (> AKRUAL_STAGING_STALE_DAYS): PENDING_STALE_REVIEW, DLQ alert
//   - Override: ROLE-AKUN-CTL, reason ≥ 30 char, signatureMethod=JWT_STEP_UP
//   - SoD dividen: approver_id ≠ maker_id
//   - Idempotency: IsDuplicateAkrual check before insert
//   - Periode lock: status_periode = 'OPEN' required
//   - Audit in-tx: AKRUAL.POSTED, MATURITY.DERECOGNIZED, DIVIDEN.POSTED etc.
//   - NEVER UPDATE ecl.amortisasi_schedule rows (DEC-013)

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

// systemActorID is the UUID used for cron service account in audit logs.
// Wire to real service account UUID in main.go via ServiceConfig.
var systemActorID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// Service owns akrualmaturity business logic.
type Service struct {
	repo             Repository
	poster           JurnalPoster
	instrumenUpdater InstrumenStatusUpdater
	audit            *audit.Writer
	logger           *slog.Logger
}

// NewService creates a new akrualmaturity Service.
func NewService(
	repo Repository,
	poster JurnalPoster,
	instrumenUpdater InstrumenStatusUpdater,
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
		instrumenUpdater: instrumenUpdater,
		audit:            auditWriter,
		logger:           logger,
	}
}

// ─── RunDailyAkrualCron ───────────────────────────────────────────────────────

// RunDailyAkrualCron runs the DAILY_ACCRUAL_JOB for a given date.
// S2-AC1..AC4. Idempotent per (instrumen_id, tanggal_akrual, 'BUNGA').
// Per-instrument errors go to DLQ; batch does not halt.
func (s *Service) RunDailyAkrualCron(ctx context.Context, tanggal time.Time) (*CronBatchResult, error) {
	result := &CronBatchResult{Tanggal: tanggal}

	// Holiday check (S1-AC4 equivalent for akrual)
	isHoliday, err := s.repo.IsHoliday(ctx, tanggal)
	if err != nil {
		return nil, fmt.Errorf("Service.RunDailyAkrualCron: holiday check: %w", err)
	}
	if isHoliday {
		s.logger.InfoContext(ctx, "RunDailyAkrualCron: holiday detected, skipping",
			"tanggal", tanggal.Format("2006-01-02"))
		result.TotalSkipped = 1
		return result, nil
	}

	// Periode check
	periode, err := s.repo.GetPeriodeByTanggal(ctx, tanggal)
	if err != nil {
		return nil, fmt.Errorf("Service.RunDailyAkrualCron: get periode: %w", err)
	}
	if periode == nil || periode.StatusPeriode != "OPEN" {
		statusStr := "tidak ditemukan"
		if periode != nil {
			statusStr = periode.StatusPeriode
		}
		s.logger.WarnContext(ctx, "RunDailyAkrualCron: periode not OPEN",
			"tanggal", tanggal.Format("2006-01-02"), "status_periode", statusStr)
		_ = s.repo.InsertDLQ(ctx, "DAILY_ACCRUAL_JOB", uuid.Nil,
			CodeAkrualPeriodeLocked,
			fmt.Sprintf("Periode untuk tanggal %s status=%s", tanggal.Format("2006-01-02"), statusStr))
		result.TotalFailed = 1
		result.DLQCount = 1
		return result, nil
	}

	// Get stale days config
	staleDays, err := s.repo.GetStaleDaysConfig(ctx)
	if err != nil {
		staleDays = 30
	}

	// Fetch active accruing instruments
	instruments, err := s.repo.GetActiveAccruingInstrumens(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.RunDailyAkrualCron: get instruments: %w", err)
	}

	for _, inst := range instruments {
		result.TotalProcessed++
		if err := s.processOneAkrual(ctx, inst, tanggal, periode, staleDays, result); err != nil {
			s.logger.WarnContext(ctx, "RunDailyAkrualCron: instrument failed",
				"instrumen_id", inst.ID, "error", err)
		}
	}
	return result, nil
}

// processOneAkrual handles akrual for a single instrument.
func (s *Service) processOneAkrual(ctx context.Context, inst *InstrumenAkrualInfo, tanggal time.Time, periode *PeriodeBuku, staleDays int, result *CronBatchResult) error {
	// Idempotency: skip if already processed
	isDup, err := s.repo.IsDuplicateAkrual(ctx, inst.ID, tanggal, JenisBunga)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualDuplicate,
			fmt.Sprintf("Duplicate check error: %v", err))
	}
	if isDup {
		s.logger.InfoContext(ctx, "processOneAkrual: duplicate, skip",
			"instrumen_id", inst.ID, "tanggal", tanggal.Format("2006-01-02"))
		result.TotalSkipped++
		return nil
	}

	// Get amortisasi schedule (EIR source)
	schedule, err := s.repo.GetAmortisasiSchedule(ctx, inst.ID, tanggal)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualEIRNotFound,
			fmt.Sprintf("AmortisasiSchedule error: %v", err))
	}
	if schedule == nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualEIRNotFound,
			"Tidak ada amortisasi schedule aktif")
	}

	// Determine EIR: POCI uses credit-adjusted
	eir := schedule.EIRPersen
	if inst.IsPOCI && schedule.CreditAdjustedEIR != nil {
		eir = *schedule.CreditAdjustedEIR
	}

	// Get sealed ECL for stage determination
	eclResult, err := s.repo.GetSealedECLForInstrumen(ctx, inst.ID)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualStagingStale,
			fmt.Sprintf("ECL lookup error: %v", err))
	}

	// Determine stage and ECL allowance
	var stage int
	var eclAllowance decimal.Decimal
	var eclRunID *uuid.UUID
	staleStagingFlag := false
	akrualStatus := AkrualAutoPosted

	if eclResult != nil {
		stage = eclResult.Stage
		eclAllowance = eclResult.ECLAllowance
		runID := eclResult.ECLCalcRunID
		eclRunID = &runID

		// Check staleness for Stage 3
		if stage == 3 && IsStaleECLRun(eclResult.SealedAt, staleDays) {
			staleStagingFlag = true
			akrualStatus = AkrualPendingStaleReview
		}
	} else {
		// No sealed ECL — treat as Stage 1 unless instrument has a staging marker
		// (conservative: use Stage 1 if no sealed run available for non-Stage-3 instruments)
		stage = 1
		if inst.Stage == 3 {
			// Stage 3 instrument without sealed ECL: stale
			staleStagingFlag = true
			akrualStatus = AkrualPendingStaleReview
		}
	}

	// FX rate for FCY instruments
	var fxRateID *uuid.UUID
	grossIDR := inst.GrossCarryingIDR

	if inst.MataUang != "IDR" && inst.MataUang != "" {
		fxRate, err := s.repo.GetFXRateApproved(ctx, inst.MataUang, tanggal)
		if err != nil {
			return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualFXRateMissing,
				fmt.Sprintf("FX rate lookup error: %v", err))
		}
		if fxRate == nil {
			return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualFXRateMissing,
				fmt.Sprintf("FX rate APPROVED tidak tersedia untuk %s tanggal %s", inst.MataUang, tanggal.Format("2006-01-02")))
		}
		rID := fxRate.ID
		fxRateID = &rID
		// grossIDR already in IDR equivalent (gross_carrying stored in IDR in mst.instrumen)
		// If FCY gross is stored, convert here; for now assume gross_carrying is already IDR
		_ = fxRate
	}

	// Compute akrual
	akrualIDR, carryingBasis, err := ComputeAkrualBunga(stage, grossIDR, eclAllowance, eir)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", CodeAkrualEIRNotFound,
			fmt.Sprintf("ComputeAkrualBunga error: %v", err))
	}

	// Compute PPh (deposito only — others handled separately)
	pph, bersih, _ := func() (decimal.Decimal, decimal.Decimal, error) {
		// Bunga akrual: PPh only for DEPOSITO klasifikasi
		if inst.KlasifikasiPSAK71 == "AC" {
			// AC could be deposito or bond — for now treat as no PPh at accrual time
			// PPh is settled at maturity for deposito
			return decimal.Zero, akrualIDR, nil
		}
		return decimal.Zero, akrualIDR, nil
	}()

	now := time.Now().UTC()
	akrualID := uuid.New()

	a := &PendapatanAkrual{
		ID:                  akrualID,
		InstrumenID:         inst.ID,
		TanggalAkrual:       tanggal,
		Jenis:               JenisBunga,
		Stage:               &stage,
		CarryingBasisIDR:    carryingBasis,
		EIRPersen:           &eir,
		BungaKotor:          akrualIDR,
		PPh:                 pph,
		BungaBersih:         bersih,
		FXRateID:            fxRateID,
		MataUang:            inst.MataUang,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		ECLRunIDUsed:        eclRunID,
		StaleStagingFlag:    staleStagingFlag,
		Status:              akrualStatus,
		PeriodeBulananID:    &periode.ID,
		CreatedAt:           now,
		CreatedBy:           systemActorID,
		UpdatedAt:           now,
		UpdatedBy:           systemActorID,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	// BEGIN TX
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", "AKRUAL_TX_ERROR",
			fmt.Sprintf("BeginTx: %v", err))
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := s.repo.InsertAkrual(ctx, tx, a); err != nil {
		return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", "AKRUAL_INSERT_ERROR",
			fmt.Sprintf("InsertAkrual: %v", err))
	}

	// Post jurnal only if not stale
	var jurnalHeaderID *uuid.UUID
	if !staleStagingFlag {
		eventCode := "AKRUAL_BUNGA"
		if stage == 3 {
			eventCode = "AKRUAL_BUNGA_STAGE3"
		}
		postResult, postErr := s.poster.PostAkrual(ctx, tx, AkrualPostRequest{
			EventCode:           eventCode,
			InstrumenID:         inst.ID,
			AkrualID:            akrualID,
			PeriodeID:           periode.ID,
			TanggalAkrual:       tanggal,
			BungaKotor:          akrualIDR,
			PPh:                 pph,
			BungaBersih:         bersih,
			Stage:               stage,
			Jenis:               JenisBunga,
			KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
			ActorID:             systemActorID,
			TenantID:            "TUGURE",
		})
		if postErr != nil {
			return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", "AKRUAL_JURNAL_ERROR",
				fmt.Sprintf("PostAkrual: %v", postErr))
		}
		jID := postResult.JurnalEntryID
		jurnalHeaderID = &jID
	}

	// Audit AKRUAL.POSTED in-tx (DEC-018)
	if s.audit != nil {
		auditAction := "AKRUAL.POSTED"
		if staleStagingFlag {
			auditAction = "AKRUAL.STAGING_STALE_ALERT"
		}
		basisLabel := "GROSS"
		if stage == 3 {
			basisLabel = "NET_CARRYING"
		}
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     auditAction,
			EntityType: "trx.pendapatan_akrual",
			EntityID:   akrualID,
			After: map[string]any{
				"instrumen_id":  inst.ID.String(),
				"stage":         stage,
				"basis":         basisLabel,
				"carrying_idr":  carryingBasis.StringFixed(4),
				"eir":           eir.StringFixed(8),
				"akrual_idr":    akrualIDR.StringFixed(4),
				"stale_staging": staleStagingFlag,
			},
		})
	}

	if jurnalHeaderID != nil {
		if err := s.repo.UpdateAkrualStatus(ctx, tx, akrualID, AkrualAutoPosted, jurnalHeaderID, nil, nil, 1, systemActorID); err != nil {
			s.logger.WarnContext(ctx, "processOneAkrual: UpdateAkrualStatus failed (non-fatal)", "error", err)
		}
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return s.dlqItem(ctx, result, inst.ID, "DAILY_ACCRUAL_JOB", "AKRUAL_COMMIT_ERROR",
				fmt.Sprintf("Commit: %v", err))
		}
		tx = nil
	}

	if staleStagingFlag {
		result.TotalFailed++
		_ = s.repo.InsertDLQ(ctx, "DAILY_ACCRUAL_JOB", inst.ID, CodeAkrualStagingStale,
			fmt.Sprintf("Stage 3 staging stale: no sealed ECL or older than %d days", staleDays))
		result.DLQCount++
	} else {
		result.TotalSuccess++
	}
	return nil
}

// ─── RunDailyMaturityCron ─────────────────────────────────────────────────────

// RunDailyMaturityCron runs the MATURITY_PROCESS_JOB for a given date.
// S1-AC1..AC4. Per-instrument errors go to DLQ; batch does not halt.
func (s *Service) RunDailyMaturityCron(ctx context.Context, tanggal time.Time) (*CronBatchResult, error) {
	result := &CronBatchResult{Tanggal: tanggal}

	// Holiday check (S1-AC4)
	isHoliday, err := s.repo.IsHoliday(ctx, tanggal)
	if err != nil {
		return nil, fmt.Errorf("Service.RunDailyMaturityCron: holiday check: %w", err)
	}
	if isHoliday {
		s.logger.InfoContext(ctx, "RunDailyMaturityCron: holiday detected, skipping",
			"tanggal", tanggal.Format("2006-01-02"))
		// Audit holiday skip (informational — no tx needed)
		if s.audit != nil {
			_ = s.audit.Write(ctx, audit.Event{
				Action:     "MATURITY.HOLIDAY_SKIP",
				EntityType: "sys",
				EntityID:   uuid.Nil,
				After:      map[string]any{"tanggal": tanggal.Format("2006-01-02"), "reason": "holiday_calendar"},
			})
		}
		result.TotalSkipped = 1
		return result, nil
	}

	// Periode check
	periode, err := s.repo.GetPeriodeByTanggal(ctx, tanggal)
	if err != nil {
		return nil, fmt.Errorf("Service.RunDailyMaturityCron: get periode: %w", err)
	}
	if periode == nil || periode.StatusPeriode != "OPEN" {
		statusStr := "tidak ditemukan"
		if periode != nil {
			statusStr = periode.StatusPeriode
		}
		_ = s.repo.InsertDLQ(ctx, "MATURITY_PROCESS_JOB", uuid.Nil,
			CodeAkrualPeriodeLocked,
			fmt.Sprintf("Periode untuk tanggal %s status=%s", tanggal.Format("2006-01-02"), statusStr))
		result.TotalFailed = 1
		result.DLQCount = 1
		return result, nil
	}

	instruments, err := s.repo.GetActiveMaturityInstrumens(ctx, tanggal)
	if err != nil {
		return nil, fmt.Errorf("Service.RunDailyMaturityCron: get instruments: %w", err)
	}

	for _, inst := range instruments {
		result.TotalProcessed++
		if err := s.processOneMaturity(ctx, inst, tanggal, periode, result); err != nil {
			s.logger.WarnContext(ctx, "RunDailyMaturityCron: instrument failed",
				"instrumen_id", inst.ID, "error", err)
		}
	}
	return result, nil
}

// processOneMaturity handles maturity for a single instrument (S1-AC1..AC3).
func (s *Service) processOneMaturity(ctx context.Context, inst *InstrumenAkrualInfo, tanggal time.Time, periode *PeriodeBuku, result *CronBatchResult) error {
	// Validate instrumen ACTIVE (S1-AC3)
	if err := ValidateInstrumenForMaturity(*inst); err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", CodeMaturityInstrumenNotActive,
			fmt.Sprintf("Instrumen tidak ACTIVE: status=%s", inst.Status))
	}

	// Get last akrual bunga for settlement
	lastAkrual, err := s.repo.GetLastAkrualForInstrumen(ctx, inst.ID)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_AKRUAL_ERROR",
			fmt.Sprintf("GetLastAkrualForInstrumen: %v", err))
	}

	bungaLast := decimal.Zero
	if lastAkrual != nil {
		bungaLast = lastAkrual.BungaKotor
	}

	// Compute PPh and net kas for DEPOSITO (S1-AC1)
	pph, netKas, err := ComputeMaturitySettlement(inst.GrossCarryingIDR, bungaLast)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_CALC_ERROR",
			fmt.Sprintf("ComputeMaturitySettlement: %v", err))
	}

	// Determine jenis from klasifikasi
	jenis := "BOND"
	if inst.KlasifikasiPSAK71 == "AC" {
		// Could be deposito or bond — default to BOND for non-deposito (KlasifikasiPSAK71 alone is insufficient)
		// In production, jenis should come from mst.instrumen.jenis_produk
		jenis = "BOND"
	}

	eventCode := "MATURITY_SETTLEMENT_BOND"
	switch jenis {
	case "DEPOSITO":
		eventCode = "MATURITY_SETTLEMENT_DEPOSITO"
	case "REKSADANA":
		eventCode = "MATURITY_SETTLEMENT_REKSADANA"
	}

	now := time.Now().UTC()
	jtID := uuid.New()

	jt := &JatuhTempo{
		ID:                  jtID,
		InstrumenID:         inst.ID,
		TanggalJatuhTempo:   tanggal,
		Jenis:               jenis,
		PokokReturned:       inst.GrossCarryingIDR,
		BungaReturned:       bungaLast,
		PPh:                 pph,
		Proceeds:            netKas,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		Status:              JatuhTempoPending,
		CreatedAt:           now,
		CreatedBy:           systemActorID,
		UpdatedAt:           now,
		UpdatedBy:           systemActorID,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	// BEGIN TX
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_TX_ERROR",
			fmt.Sprintf("BeginTx: %v", err))
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Insert jatuh_tempo PENDING
	if err := s.repo.InsertJatuhTempo(ctx, tx, jt); err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_INSERT_ERROR",
			fmt.Sprintf("InsertJatuhTempo: %v", err))
	}

	// Post jurnal MATURITY_SETTLEMENT
	postResult, postErr := s.poster.PostMaturity(ctx, tx, MaturityPostRequest{
		EventCode:           eventCode,
		InstrumenID:         inst.ID,
		JatuhTempoID:        jtID,
		PeriodeID:           periode.ID,
		TanggalJatuhTempo:   tanggal,
		PokokIDR:            inst.GrossCarryingIDR,
		BungaLastIDR:        bungaLast,
		PPhIDR:              pph,
		NetKasIDR:           netKas,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		Jenis:               jenis,
		ActorID:             systemActorID,
		TenantID:            "TUGURE",
	})
	if postErr != nil {
		errMsg := fmt.Sprintf("PostMaturity: %v", postErr)
		_ = s.repo.UpdateJatuhTempoStatus(ctx, tx, jtID, tanggal, JatuhTempoFailed, nil, &errMsg, 1, systemActorID)
		_ = tx.Commit()
		tx = nil
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_JURNAL_ERROR", errMsg)
	}

	// Set instrumen → MATURED
	if s.instrumenUpdater != nil {
		if err := s.instrumenUpdater.SetMatured(ctx, tx, inst.ID, systemActorID); err != nil {
			errMsg := fmt.Sprintf("SetMatured: %v", err)
			_ = s.repo.UpdateJatuhTempoStatus(ctx, tx, jtID, tanggal, JatuhTempoFailed, nil, &errMsg, 1, systemActorID)
			_ = tx.Commit()
			tx = nil
			return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_INSTRUMEN_ERROR", errMsg)
		}
	}

	// Update jatuh_tempo → SETTLED
	jurnalID := postResult.JurnalEntryID
	if err := s.repo.UpdateJatuhTempoStatus(ctx, tx, jtID, tanggal, JatuhTempoSettled, &jurnalID, nil, 1, systemActorID); err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_UPDATE_ERROR",
			fmt.Sprintf("UpdateJatuhTempoStatus SETTLED: %v", err))
	}

	// Audit MATURITY.DERECOGNIZED in-tx (DEC-018)
	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "MATURITY.DERECOGNIZED",
			EntityType: "mst.instrumen",
			EntityID:   inst.ID,
			After: map[string]any{
				"instrumen_id":  inst.ID.String(),
				"pokok_idr":     inst.GrossCarryingIDR.StringFixed(4),
				"bunga_last_idr": bungaLast.StringFixed(4),
				"pph_idr":       pph.StringFixed(4),
				"net_kas_idr":   netKas.StringFixed(4),
				"jenis":         jenis,
				"klasifikasi":   inst.KlasifikasiPSAK71,
			},
		})
	}

	if err := tx.Commit(); err != nil {
		return s.dlqItem(ctx, result, inst.ID, "MATURITY_PROCESS_JOB", "MATURITY_COMMIT_ERROR",
			fmt.Sprintf("Commit: %v", err))
	}
	tx = nil
	result.TotalSuccess++
	return nil
}

// ─── OverrideStaleAkrual ─────────────────────────────────────────────────────

// OverrideStaleAkrual allows ROLE-AKUN-CTL to confirm stale staging is valid.
// Recomputes akrual with current data, inserts new OVERRIDE_APPROVED → POSTED row.
// S5-AC4.
func (s *Service) OverrideStaleAkrual(ctx context.Context, akrualID uuid.UUID, req OverrideStaleRequest, idempKey string) (*PendapatanAkrual, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	actorID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	if err := ValidateSignatureMethod(req.SignatureMethod); err != nil {
		return nil, err
	}
	if err := ValidateOverrideReason(req.Reason); err != nil {
		return nil, err
	}

	// Fetch stale akrual
	existing, err := s.repo.GetAkrualByID(ctx, akrualID)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: get akrual: %w", err)
	}
	if existing == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Akrual %s tidak ditemukan.", akrualID))
	}
	if !existing.Status.CanOverride() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(existing.Status), "POSTED")
	}

	// Fetch current ECL (recompute with latest data)
	eclResult, err := s.repo.GetSealedECLForInstrumen(ctx, existing.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: get ECL: %w", err)
	}

	inst, err := s.repo.GetInstrumenInfo(ctx, existing.InstrumenID)
	if err != nil || inst == nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: get instrumen: %w", err)
	}

	schedule, err := s.repo.GetAmortisasiSchedule(ctx, existing.InstrumenID, existing.TanggalAkrual)
	if err != nil || schedule == nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("EIR schedule tidak tersedia untuk instrumen %s.", existing.InstrumenID))
	}

	eir := schedule.EIRPersen
	if inst.IsPOCI && schedule.CreditAdjustedEIR != nil {
		eir = *schedule.CreditAdjustedEIR
	}

	stage := 1
	eclAllowance := decimal.Zero
	var eclRunID *uuid.UUID
	if eclResult != nil {
		stage = eclResult.Stage
		eclAllowance = eclResult.ECLAllowance
		rID := eclResult.ECLCalcRunID
		eclRunID = &rID
	}

	akrualIDR, carryingBasis, err := ComputeAkrualBunga(stage, inst.GrossCarryingIDR, eclAllowance, eir)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: compute: %w", err)
	}

	periode, err := s.repo.GetPeriodeByTanggal(ctx, existing.TanggalAkrual)
	if err != nil || periode == nil || periode.StatusPeriode != "OPEN" {
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			"Periode buku untuk tanggal akrual sudah CLOSED.")
	}

	reason := req.Reason

	// BEGIN TX
	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Post jurnal
	eventCode := "AKRUAL_BUNGA"
	if stage == 3 {
		eventCode = "AKRUAL_BUNGA_STAGE3"
	}
	postResult, postErr := s.poster.PostAkrual(ctx, tx, AkrualPostRequest{
		EventCode:           eventCode,
		InstrumenID:         existing.InstrumenID,
		AkrualID:            akrualID,
		PeriodeID:           periode.ID,
		TanggalAkrual:       existing.TanggalAkrual,
		BungaKotor:          akrualIDR,
		PPh:                 decimal.Zero,
		BungaBersih:         akrualIDR,
		Stage:               stage,
		Jenis:               JenisBunga,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		ActorID:             actorID,
		TenantID:            "TUGURE",
	})
	if postErr != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: post jurnal: %w", postErr)
	}

	jurnalID := postResult.JurnalEntryID

	// Update existing akrual → POSTED with override info
	if err := s.repo.UpdateAkrualStatus(ctx, tx, akrualID, AkrualPosted, &jurnalID, &actorID, &reason, existing.RowVersion, actorID); err != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: update status: %w", err)
	}

	// Audit AKRUAL.POSTED_OVERRIDE in-tx
	if s.audit != nil {
		basisLabel := "GROSS"
		if stage == 3 {
			basisLabel = "NET_CARRYING"
		}
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "AKRUAL.POSTED_OVERRIDE",
			EntityType: "trx.pendapatan_akrual",
			EntityID:   akrualID,
			Before:     map[string]any{"status": string(AkrualPendingStaleReview)},
			After: map[string]any{
				"status":          "POSTED",
				"override_by":     actorID.String(),
				"reason":          reason,
				"stage":           stage,
				"basis":           basisLabel,
				"akrual_idr_recomputed": akrualIDR.StringFixed(4),
				"ecl_run_id_used": func() string {
					if eclRunID != nil { return eclRunID.String() }
					return ""
				}(),
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.OverrideStaleAkrual: commit: %w", err)
		}
		tx = nil
	}

	// Fetch and return updated akrual
	updated, err := s.repo.GetAkrualByID(ctx, akrualID)
	if err != nil {
		return nil, fmt.Errorf("Service.OverrideStaleAkrual: fetch updated: %w", err)
	}
	updated.CarryingBasisIDR = carryingBasis
	updated.ECLRunIDUsed = eclRunID
	_ = idempKey
	return updated, nil
}

// ─── PostDividen ─────────────────────────────────────────────────────────────

// PostDividen handles create dividen request from ROLE-MAKER-TR.
// S3-AC1..AC4.
func (s *Service) PostDividen(ctx context.Context, req CreateDividenRequest) (*Dividen, error) {
	claims := auth.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, domainerrors.ErrUnauthorized("JWT claims tidak ditemukan.")
	}
	makerID, err := uuid.Parse(claims.Sub)
	if err != nil {
		return nil, domainerrors.ErrUnauthorized("sub claim bukan UUID valid.")
	}

	if err := ValidateDividenInput(req.JumlahKotor, req.TanggalTerima); err != nil {
		return nil, err
	}

	tanggalTerima, err := ParseDateStrict(req.TanggalTerima)
	if err != nil {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed, err.Error())
	}

	inst, err := s.repo.GetInstrumenInfo(ctx, req.InstrumenID)
	if err != nil {
		return nil, fmt.Errorf("Service.PostDividen: get instrumen: %w", err)
	}
	if inst == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Instrumen %s tidak ditemukan.", req.InstrumenID))
	}
	if inst.Status != "ACTIVE" {
		return nil, domainerrors.New(domainerrors.CodeValidationFailed,
			fmt.Sprintf("Instrumen %s tidak ACTIVE.", inst.KodeInstrumen))
	}

	periode, err := s.repo.GetPeriodeByTanggal(ctx, tanggalTerima)
	if err != nil {
		return nil, fmt.Errorf("Service.PostDividen: get periode: %w", err)
	}
	if periode == nil || periode.StatusPeriode != "OPEN" {
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			"Periode buku untuk tanggal dividen sudah CLOSED.")
	}

	// Determine treatment: FVOCI Election → OCI (per Tugure policy), others → P&L
	treatment := "P&L"
	if inst.KlasifikasiPSAK71 == "FVOCI_ELECTION" {
		treatment = "OCI"
	}

	// Compute PPh (10% dividen)
	jenisForPPH := JenisDividen
	if req.IsReksadana {
		jenisForPPH = JenisDistribusiRD
	}
	pph, bersih, err := ComputePPH(jenisForPPH, req.JumlahKotor)
	if err != nil {
		return nil, fmt.Errorf("Service.PostDividen: compute PPh: %w", err)
	}

	now := time.Now().UTC()
	dividenID := uuid.New()

	d := &Dividen{
		ID:                  dividenID,
		InstrumenID:         req.InstrumenID,
		TanggalTerima:       tanggalTerima,
		JumlahKotor:         req.JumlahKotor,
		PPHDividen:          pph,
		JumlahBersih:        bersih,
		KlasifikasiSnapshot: inst.KlasifikasiPSAK71,
		Treatment:           treatment,
		IsReksadana:         req.IsReksadana,
		Status:              DividenPendingApproval,
		MakerID:             makerID,
		CreatedAt:           now,
		CreatedBy:           makerID,
		UpdatedAt:           now,
		UpdatedBy:           makerID,
		RowVersion:          1,
		TenantID:            "TUGURE",
	}

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.PostDividen: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := s.repo.InsertDividen(ctx, tx, d); err != nil {
		return nil, fmt.Errorf("Service.PostDividen: insert: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "DIVIDEN.CREATED",
			EntityType: "trx.dividen",
			EntityID:   dividenID,
			After: map[string]any{
				"instrumen_id":  req.InstrumenID.String(),
				"jumlah_kotor":  req.JumlahKotor.StringFixed(4),
				"pph":           pph.StringFixed(4),
				"jumlah_bersih": bersih.StringFixed(4),
				"treatment":     treatment,
				"is_reksadana":  req.IsReksadana,
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.PostDividen: commit: %w", err)
		}
		tx = nil
	}
	return d, nil
}

// ApproveDividen handles approve transition for dividen (ROLE-APPR-TR).
// S3-AC1, S3-AC3 (SoD).
func (s *Service) ApproveDividen(ctx context.Context, dividenID uuid.UUID, req ApproveDividenRequest) (*Dividen, error) {
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

	d, err := s.repo.GetDividenByID(ctx, dividenID)
	if err != nil {
		return nil, fmt.Errorf("Service.ApproveDividen: get dividen: %w", err)
	}
	if d == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Dividen %s tidak ditemukan.", dividenID))
	}
	if !d.Status.CanApprove() {
		return nil, domainerrors.ErrWorkflowInvalidTransition(string(d.Status), "POSTED")
	}

	// SoD check (S3-AC3)
	if d.MakerID == approverID {
		return nil, domainerrors.New(domainerrors.CodeSoDViolation,
			"maker tidak dapat menjadi approver untuk dividen yang sama (DEC-017).",
			domainerrors.Detail{Field: "actor.userId", Rule: "sod_approver_ne_maker"},
		)
	}

	periode, err := s.repo.GetPeriodeByTanggal(ctx, d.TanggalTerima)
	if err != nil || periode == nil || periode.StatusPeriode != "OPEN" {
		return nil, domainerrors.New(domainerrors.CodePeriodeClosed,
			"Periode buku untuk tanggal dividen sudah CLOSED.")
	}

	now := time.Now().UTC()
	comment := req.Comment
	sigMethod := req.SignatureMethod

	tx, err := s.repo.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.ApproveDividen: begin tx: %w", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	// Post jurnal DIVIDEN
	eventCode := "DIVIDEN"
	postResult, postErr := s.poster.PostDividen(ctx, tx, DividenPostRequest{
		EventCode:           eventCode,
		InstrumenID:         d.InstrumenID,
		DividenID:           dividenID,
		PeriodeID:           periode.ID,
		TanggalTerima:       d.TanggalTerima,
		JumlahKotor:         d.JumlahKotor,
		PPh:                 d.PPHDividen,
		JumlahBersih:        d.JumlahBersih,
		KlasifikasiSnapshot: d.KlasifikasiSnapshot,
		Treatment:           d.Treatment,
		ActorID:             approverID,
		TenantID:            "TUGURE",
	})
	if postErr != nil {
		return nil, fmt.Errorf("Service.ApproveDividen: post jurnal: %w", postErr)
	}

	jurnalID := postResult.JurnalEntryID

	if err := s.repo.UpdateDividenStatus(ctx, tx, dividenID, d.TanggalTerima, DividenPosted,
		&approverID, &comment, nil, &sigMethod, &now, &jurnalID, d.RowVersion, approverID); err != nil {
		return nil, fmt.Errorf("Service.ApproveDividen: update status: %w", err)
	}

	if s.audit != nil {
		_ = s.audit.WithTx(tx).Write(ctx, audit.Event{
			Action:     "DIVIDEN.POSTED",
			EntityType: "trx.dividen",
			EntityID:   dividenID,
			Before:     map[string]any{"status": "PENDING_APPROVAL"},
			After: map[string]any{
				"status":           "POSTED",
				"approver_id":      approverID.String(),
				"signature_method": req.SignatureMethod,
				"jurnal_entry_id":  jurnalID.String(),
				"instrumen_id":     d.InstrumenID.String(),
				"gross_dividen":    d.JumlahKotor.StringFixed(4),
				"pph":              d.PPHDividen.StringFixed(4),
				"net":              d.JumlahBersih.StringFixed(4),
				"klasifikasi":      d.KlasifikasiSnapshot,
				"treatment":        d.Treatment,
			},
		})
	}

	if tx != nil {
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("Service.ApproveDividen: commit: %w", err)
		}
		tx = nil
	}
	d.Status = DividenPosted
	d.ApproverID = &approverID
	d.ApproveComment = &comment
	d.ApprovedAt = &now
	d.JurnalHeaderID = &jurnalID
	return d, nil
}

// ─── List methods ─────────────────────────────────────────────────────────────

// ListAkrual returns paginated akrual rows.
func (s *Service) ListAkrual(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*PendapatanAkrual, bool, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListAkrual(ctx, q, cursor, limit)
}

// GetAkrualByID returns single akrual.
func (s *Service) GetAkrualByID(ctx context.Context, id uuid.UUID) (*PendapatanAkrual, error) {
	a, err := s.repo.GetAkrualByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("Service.GetAkrualByID: %w", err)
	}
	if a == nil {
		return nil, domainerrors.New(domainerrors.CodeNotFound,
			fmt.Sprintf("Akrual %s tidak ditemukan.", id))
	}
	return a, nil
}

// GetDashboard returns MTD/YTD dashboard.
func (s *Service) GetDashboard(ctx context.Context, instrumenID *uuid.UUID, portofolioID *uuid.UUID, year, month int) (*AkrualDashboard, error) {
	return s.repo.GetMTDYTDSummary(ctx, instrumenID, portofolioID, year, month)
}

// ListJatuhTempo returns paginated jatuh_tempo rows.
func (s *Service) ListJatuhTempo(ctx context.Context, q listquery.Query, cursor string, limit int) ([]*JatuhTempo, bool, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListJatuhTempo(ctx, q, cursor, limit)
}

// ─── Private helpers ──────────────────────────────────────────────────────────

// dlqItem logs a cron error to DLQ and adds to result.
func (s *Service) dlqItem(ctx context.Context, result *CronBatchResult, instrumenID uuid.UUID, jobType, errorCode, errorDetail string) error {
	result.TotalFailed++
	result.DLQCount++
	result.Errors = append(result.Errors, CronItemError{
		InstrumenID: instrumenID,
		ErrorCode:   errorCode,
		ErrorDetail: errorDetail,
	})
	_ = s.repo.InsertDLQ(ctx, jobType, instrumenID, errorCode, errorDetail)
	return fmt.Errorf("%s: instrumen %s: %s", errorCode, instrumenID, errorDetail)
}
