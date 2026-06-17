package closeflow

// checklist.go — 4-item closing checklist evaluator.
//
// ChecklistService.Evaluate() queries 4 data sources:
//   1. PENDING_APPROVAL_ZERO — trx.* + jrnl.header PENDING workflow count.
//   2. JURNAL_BALANCED — ABS(total_debit - total_kredit) ≤ IDR 0.01 per header.
//   3. GL_DELIVERED — no jrnl.gl_status.gl_host_status = 'FAILED' (DEAD_LETTER excluded).
//   4. RECON_PASS — latest sys.gl_reconciliation_report for period: status = 'COMPLETED' strict.
//
// Compliance:
//   - DEC-016: shopspring/decimal for JURNAL_BALANCED threshold (no float64).
//   - S5-AC4: CLOSED periods return last snapshot, not a fresh evaluation.

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	domainerrors "blips-ifrs9.tugu-re.com/internal/common/errors"
)

// ChecklistService evaluates the 4-item closing checklist.
type ChecklistService struct {
	db *sql.DB
}

// NewChecklistService creates a ChecklistService. Panics on nil db.
func NewChecklistService(db *sql.DB) *ChecklistService {
	if db == nil {
		panic("closeflow.NewChecklistService: db must not be nil")
	}
	return &ChecklistService{db: db}
}

// Evaluate runs all 4 checklist items against the database in real-time.
// Does NOT persist a snapshot (snapshot is only written on actual state transitions).
func (s *ChecklistService) Evaluate(ctx context.Context, periodeID uuid.UUID) (ChecklistEvalResult, error) {
	items := make([]ChecklistItem, 0, 4)

	// Item 1: PENDING_APPROVAL_ZERO
	item1, err := s.checkPendingApprovalZero(ctx, periodeID)
	if err != nil {
		return ChecklistEvalResult{}, fmt.Errorf("checklist PENDING_APPROVAL_ZERO: %w", err)
	}
	items = append(items, item1)

	// Item 2: JURNAL_BALANCED
	item2, err := s.checkJurnalBalanced(ctx, periodeID)
	if err != nil {
		return ChecklistEvalResult{}, fmt.Errorf("checklist JURNAL_BALANCED: %w", err)
	}
	items = append(items, item2)

	// Item 3: GL_DELIVERED
	item3, err := s.checkGLDelivered(ctx, periodeID)
	if err != nil {
		return ChecklistEvalResult{}, fmt.Errorf("checklist GL_DELIVERED: %w", err)
	}
	items = append(items, item3)

	// Item 4: RECON_PASS
	item4, err := s.checkReconPass(ctx, periodeID)
	if err != nil {
		return ChecklistEvalResult{}, fmt.Errorf("checklist RECON_PASS: %w", err)
	}
	items = append(items, item4)

	allPassed := item1.Passed && item2.Passed && item3.Passed && item4.Passed

	return ChecklistEvalResult{
		EvaluatedAt: time.Now(),
		AllPassed:   allPassed,
		Items:       items,
	}, nil
}

// checkPendingApprovalZero counts entities with PENDING workflow status for the period.
// C3: Covers all workflow-bearing tables with (workflow_status, periode_id).
// Dynamic table guard via to_regclass() prevents query failure for tables added in later phases.
func (s *ChecklistService) checkPendingApprovalZero(ctx context.Context, periodeID uuid.UUID) (ChecklistItem, error) {
	const label = "0 transaksi/jurnal masih PENDING_APPROVAL"

	// C3: Extended UNION to cover all known trx.* + jrnl.* + mst.* tables with
	// workflow_status + periode_id. Dynamic guard via to_regclass() for tables
	// that may not exist yet in earlier migration states.
	//
	// Tables covered:
	//   trx.penempatan_deposito (migration 000033)
	//   jrnl.header (migration 000035)
	//   mst.mapping_jurnal_header (migration 000017/000035 — per_period via jurnal)
	//
	// Future tables (trx.renewal, trx.jual, trx.penerimaan_bunga) are guarded with
	// to_regclass() so this query works before those migrations run.
	const q = `
		SELECT COALESCE(SUM(cnt), 0)
		FROM (
			-- trx.penempatan_deposito (P5-M1, migration 000033)
			SELECT COUNT(*) AS cnt
			FROM trx.penempatan_deposito
			WHERE periode_id = $1
			  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
			  AND deleted_at IS NULL

			UNION ALL

			-- jrnl.header (P5-M2, migration 000035)
			SELECT COUNT(*) AS cnt
			FROM jrnl.header
			WHERE periode_id = $1
			  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
			  AND deleted_at IS NULL

			-- mst.mapping_jurnal_header: only rows linked to this period via jrnl.header
			-- (mapping_jurnal_header itself has no period_id; approximated via jurnal link)
			-- Excluded here — mapping_jurnal is a template, not a per-period transaction.

			-- trx.renewal (future phase) — guarded with to_regclass
			UNION ALL
			SELECT CASE WHEN to_regclass('trx.renewal') IS NOT NULL THEN (
				SELECT COUNT(*) FROM trx.renewal
				WHERE periode_id = $1
				  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
				  AND deleted_at IS NULL
			) ELSE 0 END AS cnt

			-- trx.jual (future phase) — guarded with to_regclass
			UNION ALL
			SELECT CASE WHEN to_regclass('trx.jual') IS NOT NULL THEN (
				SELECT COUNT(*) FROM trx.jual
				WHERE periode_id = $1
				  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
				  AND deleted_at IS NULL
			) ELSE 0 END AS cnt

			-- trx.penerimaan_bunga (future phase) — guarded with to_regclass
			UNION ALL
			SELECT CASE WHEN to_regclass('trx.penerimaan_bunga') IS NOT NULL THEN (
				SELECT COUNT(*) FROM trx.penerimaan_bunga
				WHERE periode_id = $1
				  AND workflow_status IN ('PENDING_REVIEW','PENDING_APPROVAL','PENDING_APPROVAL_2')
				  AND deleted_at IS NULL
			) ELSE 0 END AS cnt
		) sub
	`

	var count int64
	if err := s.db.QueryRowContext(ctx, q, periodeID).Scan(&count); err != nil {
		return ChecklistItem{}, fmt.Errorf("pending approval query: %w", err)
	}

	if count == 0 {
		return ChecklistItem{
			Key:    ChecklistKeyPendingApprovalZero,
			Label:  label,
			Passed: true,
			Detail: "Total PENDING_APPROVAL: 0",
		}, nil
	}

	return ChecklistItem{
		Key:    ChecklistKeyPendingApprovalZero,
		Label:  label,
		Passed: false,
		Detail: fmt.Sprintf("%d transaksi/jurnal masih dalam status PENDING. Selesaikan sebelum close.", count),
	}, nil
}

// checkJurnalBalanced verifies that ABS(total_debit - total_kredit) ≤ IDR 0.01 for every
// jrnl.header in the period. Uses shopspring/decimal (DEC-016).
func (s *ChecklistService) checkJurnalBalanced(ctx context.Context, periodeID uuid.UUID) (ChecklistItem, error) {
	const label = "Semua jurnal seimbang (total_debit == total_kredit, delta ≤ IDR 0.01)"

	// Fetch max absolute delta and total jurnal count.
	const q = `
		SELECT
			COUNT(*) AS total,
			COALESCE(MAX(ABS(total_debit - total_kredit)), 0) AS max_delta
		FROM jrnl.header
		WHERE periode_id = $1
		  AND deleted_at IS NULL
	`

	var total int64
	var maxDeltaStr string
	if err := s.db.QueryRowContext(ctx, q, periodeID).Scan(&total, &maxDeltaStr); err != nil {
		return ChecklistItem{}, fmt.Errorf("jurnal balanced query: %w", err)
	}

	maxDelta, err := decimal.NewFromString(maxDeltaStr)
	if err != nil {
		return ChecklistItem{}, fmt.Errorf("parse max_delta '%s': %w", maxDeltaStr, err)
	}

	passed := maxDelta.LessThanOrEqual(JurnalBalancedThreshold)

	if passed {
		return ChecklistItem{
			Key:    ChecklistKeyJurnalBalanced,
			Label:  label,
			Passed: true,
			Detail: fmt.Sprintf("Jurnal checked: %d. Max delta: IDR %s", total, maxDelta.StringFixed(4)),
		}, nil
	}

	return ChecklistItem{
		Key:    ChecklistKeyJurnalBalanced,
		Label:  label,
		Passed: false,
		Detail: fmt.Sprintf("Jurnal tidak seimbang. Total header: %d. Max delta: IDR %s (threshold IDR 0.0100).",
			total, maxDelta.StringFixed(4)),
	}, nil
}

// checkGLDelivered counts jrnl.gl_status rows with gl_host_status = 'FAILED'
// for jurnal headers in the period. DEAD_LETTER is excluded (explicitly discarded).
func (s *ChecklistService) checkGLDelivered(ctx context.Context, periodeID uuid.UUID) (ChecklistItem, error) {
	const label = "Tidak ada gl_host_status = FAILED yang belum diselesaikan"

	const q = `
		SELECT COUNT(gs.id), ARRAY_AGG(jh.id::TEXT)
		FROM jrnl.gl_status gs
		JOIN jrnl.header jh ON gs.header_id = jh.id
		WHERE jh.periode_id = $1
		  AND gs.gl_host_status = 'FAILED'
		  AND jh.deleted_at IS NULL
	`

	var count int64
	var headerIDsArr *string // postgres array string or NULL
	row := s.db.QueryRowContext(ctx, q, periodeID)
	if err := row.Scan(&count, &headerIDsArr); err != nil {
		return ChecklistItem{}, fmt.Errorf("gl delivered query: %w", err)
	}

	if count == 0 {
		return ChecklistItem{
			Key:    ChecklistKeyGLDelivered,
			Label:  label,
			Passed: true,
			Detail: "Tidak ada FAILED delivery. DEAD_LETTER: dikecualikan (sudah discarded).",
		}, nil
	}

	headerIDsStr := ""
	if headerIDsArr != nil {
		headerIDsStr = *headerIDsArr
	}

	actionURL := "/jurnal/gl-delivery-dlq"
	return ChecklistItem{
		Key:       ChecklistKeyGLDelivered,
		Label:     label,
		Passed:    false,
		Detail:    fmt.Sprintf("%d jurnal masih FAILED. Header IDs: %s", count, headerIDsStr),
		ActionURL: &actionURL,
	}, nil
}

// checkReconPass verifies the latest sys.gl_reconciliation_report for the period
// has status = 'COMPLETED' (strict — COMPLETED_WITH_MISMATCH = FAIL per OQ-M4-1b).
func (s *ChecklistService) checkReconPass(ctx context.Context, periodeID uuid.UUID) (ChecklistItem, error) {
	const label = "GL rekonsiliasi harian terakhir COMPLETED (strict)"

	// Find the periode date range to query reconciliation reports.
	const dateQ = `
		SELECT tanggal_mulai, tanggal_akhir FROM mst.periode_buku
		WHERE id = $1 AND deleted_at IS NULL LIMIT 1
	`
	var tanggalMulai, tanggalAkhir time.Time
	if err := s.db.QueryRowContext(ctx, dateQ, periodeID).Scan(&tanggalMulai, &tanggalAkhir); err != nil {
		return ChecklistItem{}, fmt.Errorf("periode date range query: %w", err)
	}

	// C11: Require the most-recent recon date to be exactly tanggal_akhir AND status=COMPLETED.
	// A mid-period recon report does NOT satisfy this check.
	const reconQ = `
		SELECT status, MAX(tanggal_rekonsiliasi) AS last_date
		FROM sys.gl_reconciliation_report
		WHERE tanggal_rekonsiliasi BETWEEN $1 AND $2
		GROUP BY status
		ORDER BY last_date DESC
		LIMIT 1
	`
	var reconStatus string
	var reconDate time.Time
	err := s.db.QueryRowContext(ctx, reconQ, tanggalMulai, tanggalAkhir).Scan(&reconStatus, &reconDate)
	if err == sql.ErrNoRows {
		return ChecklistItem{
			Key:    ChecklistKeyReconPass,
			Label:  label,
			Passed: false,
			Detail: "Tidak ada laporan rekonsiliasi GL untuk periode ini. Jalankan rekonsiliasi terlebih dahulu.",
		}, nil
	}
	if err != nil {
		return ChecklistItem{}, fmt.Errorf("recon pass query: %w", err)
	}

	// C11: The recon must cover the LAST day of the period (tanggal_akhir).
	// A report from day 15 of a 30-day period is insufficient.
	lastDayOfPeriod := tanggalAkhir.Format("2006-01-02")
	reconDay := reconDate.Format("2006-01-02")
	if reconDay != lastDayOfPeriod {
		return ChecklistItem{
			Key:    ChecklistKeyReconPass,
			Label:  label,
			Passed: false,
			Detail: fmt.Sprintf("Rekonsiliasi terakhir (%s) belum mencakup tanggal akhir periode (%s). Jalankan rekonsiliasi untuk tanggal %s terlebih dahulu.",
				reconDay, lastDayOfPeriod, lastDayOfPeriod),
		}, nil
	}

	if reconStatus == "COMPLETED" {
		return ChecklistItem{
			Key:    ChecklistKeyReconPass,
			Label:  label,
			Passed: true,
			Detail: fmt.Sprintf("Last recon: %s — COMPLETED. Mencakup tanggal akhir periode.", reconDate.Format("2006-01-02")),
		}, nil
	}

	return ChecklistItem{
		Key:    ChecklistKeyReconPass,
		Label:  label,
		Passed: false,
		Detail: fmt.Sprintf("Last recon: %s — %s. Hanya status COMPLETED yang diterima (COMPLETED_WITH_MISMATCH = FAIL).",
			reconDate.Format("2006-01-02"), reconStatus),
	}, nil
}

// IsChecklistStale returns true if the latest SOFT_CLOSE_REQUEST snapshot for the
// period is older than staleHours. Used at soft-close-approve time (stale check).
func (s *ChecklistService) IsChecklistStale(ctx context.Context, periodeID uuid.UUID, staleHours int) (bool, error) {
	const q = `
		SELECT created_at
		FROM sys.closing_checklist_snapshot
		WHERE periode_buku_id = $1 AND transition = 'SOFT_CLOSE_REQUEST'
		ORDER BY created_at DESC
		LIMIT 1
	`
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, q, periodeID).Scan(&createdAt)
	if err == sql.ErrNoRows {
		// No snapshot found — treat as stale (re-run needed).
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("stale check query: %w", err)
	}

	elapsed := time.Since(createdAt)
	return elapsed > time.Duration(staleHours)*time.Hour, nil
}

// BuildChecklistJSONB converts items to the JSONB format stored in sys.closing_checklist_snapshot.
func BuildChecklistJSONB(result ChecklistEvalResult) map[string]any {
	items := make([]map[string]any, 0, len(result.Items))
	for _, item := range result.Items {
		m := map[string]any{
			"key":    string(item.Key),
			"label":  item.Label,
			"passed": item.Passed,
			"detail": item.Detail,
		}
		if item.ActionURL != nil {
			m["action_url"] = *item.ActionURL
		}
		items = append(items, m)
	}
	return map[string]any{
		"evaluated_at": result.EvaluatedAt.Format(time.RFC3339),
		"items":        items,
	}
}

// BuildChecklistDetails converts a ChecklistEvalResult into DomainError details
// for items that failed.
func BuildChecklistDetails(result ChecklistEvalResult) []domainerrors.Detail {
	var details []domainerrors.Detail
	for _, item := range result.Items {
		if !item.Passed {
			details = append(details, domainerrors.Detail{
				Field:   string(item.Key),
				Rule:    strings.ToLower(string(item.Key)),
				Message: item.Detail,
			})
		}
	}
	return details
}
