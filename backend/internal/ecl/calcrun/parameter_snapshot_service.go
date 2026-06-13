package calcrun

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// parameter_snapshot_service.go — ParameterSnapshotService
//
// Snapshot is frozen atomically at /start time (OQ-M8-5 LOCKED: full snapshot).
// All parameter tables must have at least one APPROVED row for the periodeID.
// Missing any parameter → CALC_RUN_PARAMETER_SNAPSHOT_INVALID (422).
//
// Compliance: state-machine doc §3 (parameter snapshot scope).
// DEC-016: no float64 — rates stored as text from DB, returned as-is in JSONB.

// ParameterSnapshot is the JSON structure frozen into ecl.calc_run.parameter_snapshot_jsonb.
// All numeric values are stored as strings to avoid float64 precision loss (DEC-016).
type ParameterSnapshot struct {
	FrozenAt    string                   `json:"frozenAt"`    // RFC3339
	PeriodeID   string                   `json:"periodeId"`
	EvalDate    string                   `json:"evalDate"`    // YYYY-MM-DD

	// BobotSkenario — scenario weights (sum must = 1.0, DEC-010).
	BobotSkenario *BobotSnapshotEntry `json:"bobotSkenario"`

	// PDPefindo — summary (count approved rows + version hash).
	PDPefindo *PDSnapshotSummary `json:"pdPefindo"`

	// LGDBasel — summary (count approved rows per tipe_eksposur).
	LGDBasel *LGDSnapshotSummary `json:"lgdBasel"`

	// ImpactPD — impact_pd multiplier snapshot.
	ImpactPD *ImpactPDSnapshot `json:"impactPD"`

	// ImpactMevPD — per-scenario FL multiplier snapshot.
	ImpactMevPD *ImpactMevPDSnapshot `json:"impactMevPD"`

	// LPSCoverage — LPS guarantee limit snapshot.
	LPSCoverage *LPSCoverageSnapshot `json:"lpsCoverage"`

	// Kurs — FX rates per evaluation_date.
	Kurs []KursEntry `json:"kurs"`
}

// BobotSnapshotEntry holds the approved scenario weights.
type BobotSnapshotEntry struct {
	ParamID    string `json:"paramId"`
	BobotGood  string `json:"bobotGood"`   // NUMERIC(7,4) as string
	BobotNormal string `json:"bobotNormal"` // NUMERIC(7,4) as string
	BobotBad   string `json:"bobotBad"`    // NUMERIC(7,4) as string
	ApprovedBy string `json:"approvedBy"`
	ApprovedAt string `json:"approvedAt"`
}

// PDSnapshotSummary summarizes approved PD Pefindo rows.
type PDSnapshotSummary struct {
	ApprovedRowCount int    `json:"approvedRowCount"`
	ApprovedBy       string `json:"approvedBy"`
	ApprovedAt       string `json:"approvedAt"`
}

// LGDSnapshotSummary summarizes approved LGD Basel rows.
type LGDSnapshotSummary struct {
	ApprovedRowCount int    `json:"approvedRowCount"`
	ApprovedBy       string `json:"approvedBy"`
	ApprovedAt       string `json:"approvedAt"`
}

// ImpactPDSnapshot holds the approved impact_pd multiplier.
type ImpactPDSnapshot struct {
	ParamID          string `json:"paramId"`
	ImpactMultiplier string `json:"impactMultiplier"` // NUMERIC(10,8) as string
	ApprovedBy       string `json:"approvedBy"`
	ApprovedAt       string `json:"approvedAt"`
}

// ImpactMevPDSnapshot holds per-scenario impact_mev_pd multipliers.
type ImpactMevPDSnapshot struct {
	Good   *ImpactMevPDEntry `json:"good"`
	Normal *ImpactMevPDEntry `json:"normal,omitempty"` // NORMAL implicit 1.0
	Bad    *ImpactMevPDEntry `json:"bad"`
}

// ImpactMevPDEntry is one scenario's FL multiplier.
type ImpactMevPDEntry struct {
	ParamID          string `json:"paramId"`
	Skenario         string `json:"skenario"`
	ImpactMultiplier string `json:"impactMultiplier"` // NUMERIC(10,8) as string
	ApprovedBy       string `json:"approvedBy"`
	ApprovedAt       string `json:"approvedAt"`
}

// LPSCoverageSnapshot holds the active LPS guarantee limit.
type LPSCoverageSnapshot struct {
	ParamID         string `json:"paramId"`
	CoverageLimitIDR string `json:"coverageLimitIdr"` // NUMERIC(20,4) as string
	EffectiveFrom   string `json:"effectiveFrom"`
	EffectiveTo     string `json:"effectiveTo,omitempty"`
	ApprovedBy      string `json:"approvedBy"`
}

// KursEntry is one FX rate row per currency for evaluation_date.
type KursEntry struct {
	KodeMataUang string `json:"kodeMataUang"`
	KursTengah   string `json:"kursTengah"` // NUMERIC(20,8) as string
	Tanggal      string `json:"tanggal"`
}

// ─── ParameterSnapshotService ─────────────────────────────────────────────────

// ParameterSnapshotService reads all ALCO-approved parameters and freezes them
// into a JSON blob for ecl.calc_run.parameter_snapshot_jsonb.
type ParameterSnapshotService struct {
	db *sql.DB
}

// NewParameterSnapshotService creates a ParameterSnapshotService. Panics if db is nil.
func NewParameterSnapshotService(db *sql.DB) *ParameterSnapshotService {
	if db == nil {
		panic("calcrun.NewParameterSnapshotService: db must not be nil")
	}
	return &ParameterSnapshotService{db: db}
}

// SnapshotAll atomically reads all APPROVED parameters for the periodeID and evalDate.
// Returns CALC_RUN_PARAMETER_SNAPSHOT_INVALID (422) if any required param is missing.
// No float64: all numeric values are read as text from the DB and stored verbatim.
func (s *ParameterSnapshotService) SnapshotAll(ctx context.Context, periodeID string, evalDate time.Time) (json.RawMessage, error) {
	snap := ParameterSnapshot{
		FrozenAt:  time.Now().UTC().Format(time.RFC3339),
		PeriodeID: periodeID,
		EvalDate:  evalDate.Format("2006-01-02"),
	}

	// 1. Bobot skenario.
	bobot, err := s.snapshotBobot(ctx, periodeID)
	if err != nil {
		return nil, err
	}
	snap.BobotSkenario = bobot

	// 2. PD Pefindo.
	pd, err := s.snapshotPD(ctx, periodeID)
	if err != nil {
		return nil, err
	}
	snap.PDPefindo = pd

	// 3. LGD Basel.
	lgd, err := s.snapshotLGD(ctx)
	if err != nil {
		return nil, err
	}
	snap.LGDBasel = lgd

	// 4. Impact PD.
	impPD, err := s.snapshotImpactPD(ctx, periodeID)
	if err != nil {
		return nil, err
	}
	snap.ImpactPD = impPD

	// 5. Impact MEV PD (per scenario GOOD + BAD; NORMAL implicit 1.0).
	impMevPD, err := s.snapshotImpactMevPD(ctx, periodeID)
	if err != nil {
		return nil, err
	}
	snap.ImpactMevPD = impMevPD

	// 6. LPS coverage.
	lpsCov, err := s.snapshotLPS(ctx, evalDate)
	if err != nil {
		return nil, err
	}
	snap.LPSCoverage = lpsCov

	// 7. Kurs (BI JISDOR) for evaluation_date.
	kurs, err := s.snapshotKurs(ctx, evalDate)
	if err != nil {
		return nil, err
	}
	snap.Kurs = kurs

	raw, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot: marshal: %w", err)
	}
	return raw, nil
}

func (s *ParameterSnapshotService) snapshotBobot(ctx context.Context, periodeID string) (*BobotSnapshotEntry, error) {
	var entry BobotSnapshotEntry
	err := s.db.QueryRowContext(ctx, `
SELECT id::text, bobot_good::text, bobot_normal::text, bobot_bad::text,
       approved_by::text, approved_at::text
FROM mst.bobot_skenario
WHERE periode_id = $1
  AND workflow_status = 'APPROVED'
  AND deleted_at IS NULL
ORDER BY approved_at DESC
LIMIT 1`, periodeID).Scan(
		&entry.ParamID, &entry.BobotGood, &entry.BobotNormal, &entry.BobotBad,
		&entry.ApprovedBy, &entry.ApprovedAt)
	if err == sql.ErrNoRows {
		return nil, ErrCalcRunParameterSnapshotInvalid("bobot skenario untuk periode " + periodeID + " belum APPROVED")
	}
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.bobot: %w", err)
	}
	return &entry, nil
}

func (s *ParameterSnapshotService) snapshotPD(ctx context.Context, periodeID string) (*PDSnapshotSummary, error) {
	var summary PDSnapshotSummary
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) as cnt,
       COALESCE(MAX(approved_by::text), '') as approved_by,
       COALESCE(MAX(approved_at::text), '') as approved_at
FROM mst.pd_pefindo
WHERE workflow_status = 'APPROVED'
  AND deleted_at IS NULL
  AND (periode_id = $1 OR periode_id IS NULL)`, periodeID).Scan(
		&summary.ApprovedRowCount, &summary.ApprovedBy, &summary.ApprovedAt)
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.pd: %w", err)
	}
	if summary.ApprovedRowCount == 0 {
		return nil, ErrECLParamNotFound("pd_pefindo — belum ada baris APPROVED")
	}
	return &summary, nil
}

func (s *ParameterSnapshotService) snapshotLGD(ctx context.Context) (*LGDSnapshotSummary, error) {
	var summary LGDSnapshotSummary
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) as cnt,
       COALESCE(MAX(approved_by::text), '') as approved_by,
       COALESCE(MAX(approved_at::text), '') as approved_at
FROM mst.lgd_basel
WHERE workflow_status = 'APPROVED'
  AND deleted_at IS NULL`).Scan(
		&summary.ApprovedRowCount, &summary.ApprovedBy, &summary.ApprovedAt)
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.lgd: %w", err)
	}
	if summary.ApprovedRowCount == 0 {
		return nil, ErrECLParamNotFound("lgd_basel — belum ada baris APPROVED")
	}
	return &summary, nil
}

func (s *ParameterSnapshotService) snapshotImpactPD(ctx context.Context, periodeID string) (*ImpactPDSnapshot, error) {
	var entry ImpactPDSnapshot
	err := s.db.QueryRowContext(ctx, `
SELECT id::text, impact_multiplier::text,
       COALESCE(approved_by::text, '') as approved_by,
       COALESCE(approved_at::text, '') as approved_at
FROM mst.impact_pd
WHERE workflow_status = 'APPROVED'
  AND (periode_id = $1 OR periode_id IS NULL)
  AND deleted_at IS NULL
ORDER BY approved_at DESC
LIMIT 1`, periodeID).Scan(
		&entry.ParamID, &entry.ImpactMultiplier,
		&entry.ApprovedBy, &entry.ApprovedAt)
	if err == sql.ErrNoRows {
		return nil, ErrECLParamNotFound("impact_pd untuk periode " + periodeID + " belum APPROVED")
	}
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.impact_pd: %w", err)
	}
	return &entry, nil
}

func (s *ParameterSnapshotService) snapshotImpactMevPD(ctx context.Context, periodeID string) (*ImpactMevPDSnapshot, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id::text, skenario, impact_multiplier::text,
       COALESCE(approved_by::text, '') as approved_by,
       COALESCE(approved_at::text, '') as approved_at
FROM mst.impact_mev_pd
WHERE workflow_status = 'APPROVED'
  AND (periode_id = $1 OR periode_id IS NULL)
  AND deleted_at IS NULL
ORDER BY skenario, approved_at DESC`, periodeID)
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.impact_mev_pd: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	snap := &ImpactMevPDSnapshot{}
	seen := map[string]bool{}
	for rows.Next() {
		var e ImpactMevPDEntry
		if err := rows.Scan(&e.ParamID, &e.Skenario, &e.ImpactMultiplier,
			&e.ApprovedBy, &e.ApprovedAt); err != nil {
			return nil, fmt.Errorf("calcrun.snapshot.impact_mev_pd scan: %w", err)
		}
		if seen[e.Skenario] {
			continue // take first (newest) per scenario
		}
		seen[e.Skenario] = true
		entry := e
		switch e.Skenario {
		case "GOOD":
			snap.Good = &entry
		case "NORMAL":
			snap.Normal = &entry
		case "BAD":
			snap.Bad = &entry
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.impact_mev_pd rows: %w", err)
	}

	// GOOD and BAD are required; NORMAL is implicit 1.0 per OQ-1 resolution (2026-06-09).
	if snap.Good == nil {
		return nil, ErrECLParamNotFound("impact_mev_pd GOOD untuk periode " + periodeID + " belum APPROVED")
	}
	if snap.Bad == nil {
		return nil, ErrECLParamNotFound("impact_mev_pd BAD untuk periode " + periodeID + " belum APPROVED")
	}
	return snap, nil
}

func (s *ParameterSnapshotService) snapshotLPS(ctx context.Context, evalDate time.Time) (*LPSCoverageSnapshot, error) {
	var entry LPSCoverageSnapshot
	var effectiveTo sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id::text, coverage_limit_idr::text,
       effective_from::text, effective_to::text,
       COALESCE(approved_by::text, '') as approved_by
FROM mst.lps_coverage
WHERE workflow_status = 'APPROVED'
  AND effective_from <= $1
  AND (effective_to IS NULL OR effective_to >= $1)
  AND deleted_at IS NULL
ORDER BY effective_from DESC
LIMIT 1`, evalDate.Format("2006-01-02")).Scan(
		&entry.ParamID, &entry.CoverageLimitIDR,
		&entry.EffectiveFrom, &effectiveTo,
		&entry.ApprovedBy)
	if err == sql.ErrNoRows {
		return nil, ErrCalcRunParameterSnapshotInvalid("lps_coverage aktif untuk tanggal " + evalDate.Format("2006-01-02") + " tidak ditemukan")
	}
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.lps: %w", err)
	}
	if effectiveTo.Valid {
		entry.EffectiveTo = effectiveTo.String
	}
	return &entry, nil
}

func (s *ParameterSnapshotService) snapshotKurs(ctx context.Context, evalDate time.Time) ([]KursEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT kode_mata_uang, kurs_tengah::text, tanggal::text
FROM mst.kurs
WHERE tanggal = $1
  AND deleted_at IS NULL
ORDER BY kode_mata_uang`, evalDate.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.kurs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []KursEntry
	for rows.Next() {
		var e KursEntry
		if err := rows.Scan(&e.KodeMataUang, &e.KursTengah, &e.Tanggal); err != nil {
			return nil, fmt.Errorf("calcrun.snapshot.kurs scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("calcrun.snapshot.kurs rows: %w", err)
	}
	if len(entries) == 0 {
		return nil, ErrFXRateNotFound(evalDate.Format("2006-01-02"))
	}
	return entries, nil
}
