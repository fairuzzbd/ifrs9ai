//go:build integration

// Package integration — rating_history integration tests (APP-A-MSTR-003b).
//
// Coverage targets:
//
//  1. TestRatingHistory_FourEyesCycle_Full
//  2. TestRatingHistory_SICRTrigger_NotchChangeMinus2
//  3. TestRatingHistory_SICRTrigger_IGToNonIG
//  4. TestRatingHistory_DefaultTrigger_RatingD
//  5. TestRatingHistory_OnApprove_PreviousActiveClosed
//  6. TestRatingHistory_MultipleActive_Rejected
//  7. TestRatingHistory_SoDViolation
//  8. TestRatingHistory_InvalidActionType_Returns422

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/common/middleware"
	"blips-ifrs9.tugu-re.com/internal/master/counterparty"
	"blips-ifrs9.tugu-re.com/internal/master/ratinghistory"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// ─── Router builder ──────────────────────────────────────────────────────────

func ratingHistoryWorkflowConfig() map[string]*workflow.Config {
	cfgs := workflow.DefaultConfigs()
	cfgs["RATING_HISTORY"] = &workflow.Config{
		EntityType:  "RATING_HISTORY",
		Eyes:        4,
		Retractable: true,
		RequiredPermissions: map[string]string{
			"submit":  "rating_history.submit",
			"review":  "rating_history.review",
			"approve": "rating_history.approve",
			"reject":  "rating_history.reject",
		},
		StepUpRequired: map[string]bool{"approve": false},
		SoDRules: workflow.SoDRulesConfig{
			ReviewerNotMaker:           true,
			ApproverNotMakerOrReviewer: true,
			Approver2NotAnyPrevious:    false,
		},
	}
	return cfgs
}

// buildRatingHistoryRouter constructs the Gin router for rating_history endpoints.
// Returns the router and the ratinghistory.Service for direct hook calls in tests.
func buildRatingHistoryRouter(db *sql.DB) (*gin.Engine, *ratinghistory.Service) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Idempotency(db))
	r.Use(testClaimsMiddleware)

	cpRepo := counterparty.NewDBRepository(db)
	rhRepo := ratinghistory.NewDBRepository(db)
	rhAuditWriter := audit.NewWriter(db)
	rhSvc := ratinghistory.NewService(rhRepo, cpRepo, rhAuditWriter, slog.Default())

	wfRepo := workflow.NewDBRepository(db)
	var wfConfigLoader workflow.ConfigLoader
	dbLoader := workflow.NewDBConfigLoader(db)
	if _, err := dbLoader.Load("RATING_HISTORY"); err == nil {
		wfConfigLoader = dbLoader
	} else {
		wfConfigLoader = workflow.NewInMemoryConfigLoader(ratingHistoryWorkflowConfig())
	}
	wfEngine := workflow.NewEngine(wfConfigLoader)
	wfAudit := audit.NewWriter(db)
	wfSvc := workflow.NewService(wfEngine, wfRepo, wfAudit, slog.Default())
	wfHandler := workflow.NewHandler(wfSvc)

	rhHandler := ratinghistory.NewHandler(rhSvc, wfHandler)

	v1 := r.Group("/api/v1")
	ratinghistory.RegisterRoutes(v1, rhHandler)
	ratinghistory.RegisterCounterpartyNestedRoutes(v1, rhHandler)

	return r, rhSvc
}

// ─── Claim builders ──────────────────────────────────────────────────────────

func rhMakerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-MAKER-TR",
		"rating_history.create", "rating_history.read", "rating_history.update",
		"rating_history.delete", "rating_history.submit",
	)
}

func rhReviewerClaims(userID uuid.UUID) string {
	return buildClaimsJSON(userID, "ROLE-APPR-TR",
		"rating_history.read", "rating_history.review", "rating_history.approve",
		"rating_history.reject",
	)
}

// ─── Seed helpers ─────────────────────────────────────────────────────────────

// seedApprovedCounterpartyForRH seeds a counterparty in APPROVED state.
func seedApprovedCounterpartyForRH(t *testing.T, db *sql.DB, kode string, makerID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.counterparty (
			id, kode_counterparty, nama, tipe, tipe_eksposur_basel,
			eligible_lps_flag, status, workflow_status,
			created_at, created_by, row_version, version, is_deleted, tenant_id
		) VALUES (
			$1, $2, 'Counterparty for RH ' || $2, 'KORPORASI', 'CORPORATE',
			false, 'ACTIVE', 'APPROVED',
			now(), $3, 1, 1, false, 'TUGURE'
		)
		ON CONFLICT (kode_counterparty) DO NOTHING
	`, id, kode, makerID)
	if err != nil {
		t.Fatalf("seedApprovedCounterpartyForRH %s: %v", kode, err)
	}

	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.counterparty WHERE kode_counterparty = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedApprovedCounterpartyForRH fetch id %s: %v", kode, err)
	}
	return actualID
}

// seedRatingHistoryDRAFT inserts a rating_history row in DRAFT state.
func seedRatingHistoryDRAFT(t *testing.T, db *sql.DB, kode string, cpID uuid.UUID,
	makerID uuid.UUID, rating string, notchChange int, tanggalBerlaku string) uuid.UUID {
	t.Helper()
	id := uuid.New()

	if tanggalBerlaku == "" {
		tanggalBerlaku = time.Now().Format("2006-01-02")
	}

	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.rating_history_counterparty (
			id, rating_history_id_kode, counterparty_id,
			tanggal_berlaku, rating_pefindo, sumber_rating, tanggal_publikasi_rating,
			action_type, notch_change, sicr_triggered, default_triggered,
			maker_id, workflow_status,
			created_at, created_by, row_version, tenant_id
		) VALUES (
			$1, $2, $3,
			$4, $5, 'PEFINDO', $4,
			'DOWNGRADE', $6, false, false,
			$7, 'DRAFT',
			now(), $7, 1, 'TUGURE'
		)
		ON CONFLICT (rating_history_id_kode) DO NOTHING
	`, id, kode, cpID, tanggalBerlaku, rating, notchChange, makerID)
	if err != nil {
		t.Fatalf("seedRatingHistoryDRAFT %s: %v", kode, err)
	}

	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.rating_history_counterparty WHERE rating_history_id_kode = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedRatingHistoryDRAFT fetch id %s: %v", kode, err)
	}

	seedWorkflowInstance(t, db, actualID, "RATING_HISTORY", makerID, 4)

	// Back-reference workflow_instance_id (best-effort).
	_, _ = db.ExecContext(context.Background(), `
		UPDATE mst.rating_history_counterparty
		SET workflow_instance_id = (
			SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL LIMIT 1
		)
		WHERE id = $1
	`, actualID)

	return actualID
}

// seedActiveRatingForRH inserts an APPROVED active rating with no tanggal_berakhir.
func seedActiveRatingForRH(t *testing.T, db *sql.DB, kode string, cpID uuid.UUID, makerID uuid.UUID, rating string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	berlaku := time.Now().AddDate(0, -3, 0).Format("2006-01-02")

	_, err := db.ExecContext(context.Background(), `
		INSERT INTO mst.rating_history_counterparty (
			id, rating_history_id_kode, counterparty_id,
			tanggal_berlaku, rating_pefindo, sumber_rating, tanggal_publikasi_rating,
			action_type, notch_change, sicr_triggered, default_triggered,
			maker_id, workflow_status,
			created_at, created_by, row_version, tenant_id
		) VALUES (
			$1, $2, $3,
			$4, $5, 'PEFINDO', $4,
			'INITIAL', 0, false, false,
			$6, 'APPROVED',
			now(), $6, 1, 'TUGURE'
		)
		ON CONFLICT (rating_history_id_kode) DO NOTHING
	`, id, kode, cpID, berlaku, rating, makerID)
	if err != nil {
		t.Fatalf("seedActiveRatingForRH %s: %v", kode, err)
	}

	// Update counterparty rating cache.
	_, _ = db.ExecContext(context.Background(),
		`UPDATE mst.counterparty SET rating_pefindo_current = $1 WHERE id = $2`, rating, cpID)

	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.rating_history_counterparty WHERE rating_history_id_kode = $1`, kode,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedActiveRatingForRH fetch id %s: %v", kode, err)
	}
	return actualID
}

// cleanupRatingHistoryByKode removes test rating_history rows by kode.
func cleanupRatingHistoryByKode(t *testing.T, db *sql.DB, kodes ...string) {
	t.Helper()
	for _, kode := range kodes {
		var id uuid.UUID
		if err := db.QueryRowContext(context.Background(),
			`SELECT id FROM mst.rating_history_counterparty WHERE rating_history_id_kode = $1`, kode,
		).Scan(&id); err == nil {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM aud.audit_log WHERE entity_id = $1`, id)
			_, _ = db.ExecContext(context.Background(), `DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
		}
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.rating_history_counterparty WHERE rating_history_id_kode = $1`, kode)
	}
}

// cleanupCounterpartyAndRH removes a counterparty and all its rating history.
func cleanupCounterpartyAndRH(t *testing.T, db *sql.DB, cpKode string) {
	t.Helper()
	var id uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM mst.counterparty WHERE kode_counterparty = $1`, cpKode,
	).Scan(&id); err == nil {
		// Get all rating history IDs first for audit/wf cleanup.
		rows, _ := db.QueryContext(context.Background(),
			`SELECT id FROM mst.rating_history_counterparty WHERE counterparty_id = $1`, id)
		if rows != nil {
			for rows.Next() {
				var rhID uuid.UUID
				if err := rows.Scan(&rhID); err == nil {
					_, _ = db.ExecContext(context.Background(), `DELETE FROM aud.audit_log WHERE entity_id = $1`, rhID)
					_, _ = db.ExecContext(context.Background(), `DELETE FROM sys.workflow_instance WHERE entity_id = $1`, rhID)
				}
			}
			rows.Close()
		}
		_, _ = db.ExecContext(context.Background(),
			`DELETE FROM mst.rating_history_counterparty WHERE counterparty_id = $1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM aud.audit_log WHERE entity_id = $1`, id)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM sys.workflow_instance WHERE entity_id = $1`, id)
	}
	_, _ = db.ExecContext(context.Background(),
		`DELETE FROM mst.counterparty WHERE kode_counterparty = $1`, cpKode)
}

// runRHWorkflowToApproval runs the HTTP 4-eyes cycle submit→review→approve and
// then calls svc.SyncWorkflowStatus directly to fire SICR computation (simulating
// the cmd/api EntityHook wiring).
// Returns the approver context used for SyncWorkflowStatus.
func runRHWorkflowToApproval(t *testing.T, router *gin.Engine, svc *ratinghistory.Service,
	entityID uuid.UUID, makerID, reviewerID, approverID uuid.UUID) {
	t.Helper()
	idPath := "/api/v1/master/rating-history/" + entityID.String()

	w1 := postJSON(router, idPath+"/submit", rhMakerClaims(makerID), uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD","comment":"Submit rating"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("rh submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}

	w2 := postJSON(router, idPath+"/review", rhReviewerClaims(reviewerID), uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD","comment":"Review OK"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("rh review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}

	w3 := postJSON(router, idPath+"/approve", rhReviewerClaims(approverID), uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD","comment":"Approved"}`)
	if w3.Code != http.StatusOK {
		t.Fatalf("rh approve: expected 200, got %d body=%s", w3.Code, w3.Body.String())
	}

	// Manually fire SyncWorkflowStatus — simulates EntityHook (wired in cmd/api).
	// This triggers SICR computation + counterparty cache update.
	approverCtx := userCtx(approverID, []string{"rating_history.approve"})
	if err := svc.SyncWorkflowStatus(approverCtx, entityID, "APPROVED", "APPROVE"); err != nil {
		t.Fatalf("SyncWorkflowStatus: %v", err)
	}
}

// assertRHBoolField reads a boolean column from rating_history and asserts its value.
func assertRHBoolField(t *testing.T, db *sql.DB, entityID uuid.UUID, col string, expected bool) {
	t.Helper()
	var val bool
	if err := db.QueryRowContext(context.Background(),
		fmt.Sprintf(`SELECT %s FROM mst.rating_history_counterparty WHERE id = $1`, col),
		entityID,
	).Scan(&val); err != nil {
		t.Fatalf("assertRHBoolField %s: %v", col, err)
	}
	if val != expected {
		t.Errorf("rating_history.%s: expected %v, got %v", col, expected, val)
	}
}

// assertCounterpartyRatingField reads mst.counterparty.rating_pefindo_current.
func assertCounterpartyRatingField(t *testing.T, db *sql.DB, cpID uuid.UUID, expected string) {
	t.Helper()
	var rating *string
	if err := db.QueryRowContext(context.Background(), `
		SELECT rating_pefindo_current FROM mst.counterparty WHERE id = $1
	`, cpID).Scan(&rating); err != nil {
		t.Fatalf("assertCounterpartyRatingField: %v", err)
	}
	if rating == nil || *rating != expected {
		got := "<nil>"
		if rating != nil {
			got = *rating
		}
		t.Errorf("counterparty.rating_pefindo_current: expected %q, got %q", expected, got)
	}
}

// ─── Test 1: Full 4-eyes cycle ────────────────────────────────────────────────

// TestRatingHistory_FourEyesCycle_Full exercises DRAFT → PENDING_REVIEW →
// PENDING_APPROVAL → APPROVED. Verifies workflow_instance state transitions,
// audit events, and signature count.
//
// Covers: DEC-017 (4-eyes), regression §3, UAT TC-005/TC-006.
func TestRatingHistory_FourEyesCycle_Full(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP1"
	rhKode := "RH-CYCLE-001"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, rhKode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, rhKode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_cycle_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_cycle_reviewer")
	approverID := seedUserSQL(t, infra.DB, "rh_cycle_approver")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)
	entityID := seedRatingHistoryDRAFT(t, infra.DB, rhKode, cpID, makerID, "idBBB", 0, "")

	router, svc := buildRatingHistoryRouter(infra.DB)

	runRHWorkflowToApproval(t, router, svc, entityID, makerID, reviewerID, approverID)

	assertWorkflowState(t, infra.DB, entityID, "APPROVED")
	assertAuditEvent(t, infra.DB, "RATING_HISTORY.SUBMIT", entityID)
	assertAuditEvent(t, infra.DB, "RATING_HISTORY.APPROVE", entityID)

	wfID := getWorkflowID(t, infra.DB, entityID)
	wfRepo := workflow.NewDBRepository(infra.DB)
	sigs, err := wfRepo.ListSignatures(context.Background(), wfID)
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) < 2 {
		t.Errorf("expected >= 2 signatures, got %d", len(sigs))
	}
	t.Logf("RatingHistory 4-eyes cycle: APPROVED, %d signatures", len(sigs))
}

// ─── Test 2: SICR trigger — notch_change = -2 ────────────────────────────────

// TestRatingHistory_SICRTrigger_NotchChangeMinus2 creates a rating with
// notch_change=-2, runs through the workflow to APPROVED (including
// SyncWorkflowStatus), and verifies sicr_triggered=true is set.
// Also verifies mst.counterparty.rating_pefindo_current is updated.
//
// Covers: DEC-011 (SICR trigger), PSAK 71 §5.5.7, UAT TC-006.
func TestRatingHistory_SICRTrigger_NotchChangeMinus2(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP2"
	rhKode := "RH-SICR-NC-001"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, rhKode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, rhKode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_sicr_nc_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_sicr_nc_reviewer")
	approverID := seedUserSQL(t, infra.DB, "rh_sicr_nc_approver")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)

	// Seed with notch_change=-2 (2-notch downgrade).
	entityID := seedRatingHistoryDRAFT(t, infra.DB, rhKode, cpID, makerID, "idBBB", -2, "2026-06-01")

	// Ensure notch_change=-2 is set correctly.
	_, _ = infra.DB.ExecContext(context.Background(), `
		UPDATE mst.rating_history_counterparty
		SET action_type = 'DOWNGRADE', notch_change = -2
		WHERE id = $1
	`, entityID)

	router, svc := buildRatingHistoryRouter(infra.DB)
	runRHWorkflowToApproval(t, router, svc, entityID, makerID, reviewerID, approverID)

	assertWorkflowState(t, infra.DB, entityID, "APPROVED")

	// sicr_triggered must be true (DEC-011: notch_change <= -2).
	assertRHBoolField(t, infra.DB, entityID, "sicr_triggered", true)
	assertRHBoolField(t, infra.DB, entityID, "default_triggered", false)

	// Counterparty rating cache must be updated.
	assertCounterpartyRatingField(t, infra.DB, cpID, "idBBB")

	t.Logf("SICR trigger (notch_change=-2): sicr_triggered=true, counterparty rating=idBBB")
}

// ─── Test 3: SICR trigger — IG to Non-IG ─────────────────────────────────────

// TestRatingHistory_SICRTrigger_IGToNonIG creates a scenario where a counterparty
// previously had an IG rating (idAA) and now receives a non-IG rating (idBB+).
// After approval, sicr_triggered must be true even with notch_change=-1.
//
// Covers: DEC-011 (SICR IG→non-IG rule), UAT TC-007.
func TestRatingHistory_SICRTrigger_IGToNonIG(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP3"
	previousRHKode := "RH-PREV-IG-001"
	newRHKode := "RH-NONIG-001"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, previousRHKode, newRHKode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, previousRHKode, newRHKode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_ig_nonig_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_ig_nonig_reviewer")
	approverID := seedUserSQL(t, infra.DB, "rh_ig_nonig_approver")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)

	// Seed the previous active IG rating (idAA = Investment Grade).
	_ = seedActiveRatingForRH(t, infra.DB, previousRHKode, cpID, makerID, "idAA")

	// Seed the new non-IG rating (idBB+ = Non-IG / Speculative).
	// notch_change = -1 so the trigger must come from IG→non-IG rule alone.
	newBerlaku := time.Now().Format("2006-01-02")
	entityID := seedRatingHistoryDRAFT(t, infra.DB, newRHKode, cpID, makerID, "idBB+", -1, newBerlaku)

	router, svc := buildRatingHistoryRouter(infra.DB)
	runRHWorkflowToApproval(t, router, svc, entityID, makerID, reviewerID, approverID)

	assertWorkflowState(t, infra.DB, entityID, "APPROVED")

	// sicr_triggered must be true (IG → non-IG transition).
	assertRHBoolField(t, infra.DB, entityID, "sicr_triggered", true)
	assertRHBoolField(t, infra.DB, entityID, "default_triggered", false)

	// Counterparty rating cache must be updated to idBB+.
	assertCounterpartyRatingField(t, infra.DB, cpID, "idBB+")

	t.Logf("SICR trigger (IG→non-IG): previous=idAA, new=idBB+ → sicr_triggered=true")
}

// ─── Test 4: Default trigger — rating idD ────────────────────────────────────

// TestRatingHistory_DefaultTrigger_RatingD verifies that a rating of "idD"
// sets default_triggered=true after workflow approval.
//
// Covers: DEC-011 (default_triggered), PSAK 71 §5.5.3, UAT TC-008.
func TestRatingHistory_DefaultTrigger_RatingD(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP4"
	rhKode := "RH-DEFAULT-001"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, rhKode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, rhKode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_default_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_default_reviewer")
	approverID := seedUserSQL(t, infra.DB, "rh_default_approver")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)
	entityID := seedRatingHistoryDRAFT(t, infra.DB, rhKode, cpID, makerID, "idD", -5, "")

	// Ensure rating_pefindo = 'idD' and action_type = DOWNGRADE.
	_, _ = infra.DB.ExecContext(context.Background(), `
		UPDATE mst.rating_history_counterparty
		SET action_type = 'DOWNGRADE', rating_pefindo = 'idD', notch_change = -5
		WHERE id = $1
	`, entityID)

	router, svc := buildRatingHistoryRouter(infra.DB)
	runRHWorkflowToApproval(t, router, svc, entityID, makerID, reviewerID, approverID)

	assertWorkflowState(t, infra.DB, entityID, "APPROVED")

	// default_triggered must be true (rating == "idD").
	assertRHBoolField(t, infra.DB, entityID, "default_triggered", true)
	// sicr_triggered also true (notch_change=-5 ≤ -2).
	assertRHBoolField(t, infra.DB, entityID, "sicr_triggered", true)

	// Counterparty rating cache updated to idD.
	assertCounterpartyRatingField(t, infra.DB, cpID, "idD")

	t.Logf("Default trigger: rating=idD → default_triggered=true, sicr_triggered=true")
}

// ─── Test 5: On approve — previous active rating closed ──────────────────────

// TestRatingHistory_OnApprove_PreviousActiveClosed verifies that when a new rating
// is approved, the previous active rating's tanggal_berakhir is set to
// (new_tanggal_berlaku - 1 day).
//
// Covers: SoW §3.2 MSTR-003b rating lifecycle.
func TestRatingHistory_OnApprove_PreviousActiveClosed(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP5"
	prevRHKode := "RH-PREV-CLOSE-001"
	newRHKode := "RH-NEW-CLOSE-001"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, prevRHKode, newRHKode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, prevRHKode, newRHKode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_close_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_close_reviewer")
	approverID := seedUserSQL(t, infra.DB, "rh_close_approver")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)

	// Previous active rating (no tanggal_berakhir).
	prevID := seedActiveRatingForRH(t, infra.DB, prevRHKode, cpID, makerID, "idA")

	// New rating berlaku 2026-06-01 → previous must close on 2026-05-31.
	newBerlaku := "2026-06-01"
	expectedBerakhir := "2026-05-31"
	newEntityID := seedRatingHistoryDRAFT(t, infra.DB, newRHKode, cpID, makerID, "idBBB", -1, newBerlaku)

	router, svc := buildRatingHistoryRouter(infra.DB)
	runRHWorkflowToApproval(t, router, svc, newEntityID, makerID, reviewerID, approverID)

	assertWorkflowState(t, infra.DB, newEntityID, "APPROVED")

	// Verify previous rating is now closed.
	var prevBerakhir *string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_berakhir::text FROM mst.rating_history_counterparty WHERE id = $1
	`, prevID).Scan(&prevBerakhir); err != nil {
		t.Fatalf("fetch prev tanggal_berakhir: %v", err)
	}
	if prevBerakhir == nil {
		t.Errorf("previous active rating tanggal_berakhir is still NULL — not closed after approve")
	} else if *prevBerakhir != expectedBerakhir {
		t.Errorf("previous rating tanggal_berakhir: expected %s, got %s", expectedBerakhir, *prevBerakhir)
	} else {
		t.Logf("previous rating correctly closed: tanggal_berakhir=%s", *prevBerakhir)
	}
}

// ─── Test 6: Multiple active ratings — business guard ─────────────────────────

// TestRatingHistory_MultipleActive_Rejected verifies that the service prevents
// a situation where two ratings are simultaneously in active (no tanggal_berakhir)
// state after an approval, by either blocking create or closing the previous one
// at approve time.
//
// Implementation note: at the API level, the system allows DRAFT to coexist with
// an APPROVED rating. The guard fires at SyncWorkflowStatus (approve time) by
// closing the previous active rating. This test verifies that after two sequential
// approvals, only the most recent rating has tanggal_berakhir = NULL.
//
// Covers: SoW §3.2 MSTR-003b one-active-rating constraint.
func TestRatingHistory_MultipleActive_Rejected(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP6"
	rh1Kode := "RH-MULTI-1"
	rh2Kode := "RH-MULTI-2"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, rh1Kode, rh2Kode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, rh1Kode, rh2Kode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_mult_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_mult_reviewer")
	approverID := seedUserSQL(t, infra.DB, "rh_mult_approver")
	maker2ID := seedUserSQL(t, infra.DB, "rh_mult_maker2")
	reviewer2ID := seedUserSQL(t, infra.DB, "rh_mult_reviewer2")
	approver2ID := seedUserSQL(t, infra.DB, "rh_mult_approver2")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)

	// First rating.
	entity1ID := seedRatingHistoryDRAFT(t, infra.DB, rh1Kode, cpID, makerID, "idA", 0, "2026-01-01")
	_, _ = infra.DB.ExecContext(context.Background(), `
		UPDATE mst.rating_history_counterparty SET action_type = 'INITIAL' WHERE id = $1
	`, entity1ID)

	router, svc := buildRatingHistoryRouter(infra.DB)

	// Approve first rating.
	runRHWorkflowToApproval(t, router, svc, entity1ID, makerID, reviewerID, approverID)
	assertWorkflowState(t, infra.DB, entity1ID, "APPROVED")

	// Second rating.
	entity2ID := seedRatingHistoryDRAFT(t, infra.DB, rh2Kode, cpID, maker2ID, "idBBB", -1, "2026-07-01")

	// Approve second rating.
	runRHWorkflowToApproval(t, router, svc, entity2ID, maker2ID, reviewer2ID, approver2ID)
	assertWorkflowState(t, infra.DB, entity2ID, "APPROVED")

	// Verify: first rating now has tanggal_berakhir set (closed by second approval).
	var berakhir *string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_berakhir::text FROM mst.rating_history_counterparty WHERE id = $1
	`, entity1ID).Scan(&berakhir); err != nil {
		t.Fatalf("fetch first rating tanggal_berakhir: %v", err)
	}
	if berakhir == nil {
		t.Errorf("first rating tanggal_berakhir is NULL after second approval — multiple active ratings exist")
	} else {
		t.Logf("first rating correctly closed: tanggal_berakhir=%s", *berakhir)
	}

	// Verify: second rating has no tanggal_berakhir (it is the current active).
	var berakhir2 *string
	if err := infra.DB.QueryRowContext(context.Background(), `
		SELECT tanggal_berakhir FROM mst.rating_history_counterparty WHERE id = $1
	`, entity2ID).Scan(&berakhir2); err != nil {
		t.Fatalf("fetch second rating tanggal_berakhir: %v", err)
	}
	if berakhir2 != nil {
		t.Errorf("second (newest) rating has tanggal_berakhir=%s — should be NULL (active)", *berakhir2)
	}
	t.Logf("only one active rating after sequential approvals: OK")
}

// ─── Test 7: SoD violation ───────────────────────────────────────────────────

// TestRatingHistory_SoDViolation verifies that a rating_history maker cannot
// also be the approver, even with a JWT containing rating_history.approve.
//
// Covers: regression §6 (SoD), DEC-017, security-baseline.md.
func TestRatingHistory_SoDViolation(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP7"
	rhKode := "RH-SOD-001"
	t.Cleanup(func() {
		cleanupRatingHistoryByKode(t, infra.DB, rhKode)
		cleanupCounterpartyAndRH(t, infra.DB, cpKode)
	})
	cleanupRatingHistoryByKode(t, infra.DB, rhKode)
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_sod_maker")
	reviewerID := seedUserSQL(t, infra.DB, "rh_sod_reviewer")

	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)
	entityID := seedRatingHistoryDRAFT(t, infra.DB, rhKode, cpID, makerID, "idBBB", -1, "")

	router, _ := buildRatingHistoryRouter(infra.DB)

	// Maker has approve permission — bypass attempt.
	makerClaims := buildClaimsJSON(makerID, "ROLE-MAKER-TR",
		"rating_history.submit", "rating_history.review", "rating_history.approve", "rating_history.read",
	)
	reviewerClaims := rhReviewerClaims(reviewerID)

	idPath := "/api/v1/master/rating-history/" + entityID.String()

	w1 := postJSON(router, idPath+"/submit", makerClaims, uuid.New().String(),
		`{"rowVersion":1,"signatureMethod":"JWT_STANDARD"}`)
	if w1.Code != http.StatusOK {
		t.Fatalf("submit: expected 200, got %d body=%s", w1.Code, w1.Body.String())
	}

	w2 := postJSON(router, idPath+"/review", reviewerClaims, uuid.New().String(),
		`{"rowVersion":2,"signatureMethod":"JWT_STANDARD"}`)
	if w2.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d body=%s", w2.Code, w2.Body.String())
	}

	// APPROVE as MAKER — must fail.
	w3 := postJSON(router, idPath+"/approve", makerClaims, uuid.New().String(),
		`{"rowVersion":3,"signatureMethod":"JWT_STANDARD"}`)
	if w3.Code != http.StatusForbidden {
		t.Errorf("SoD: maker-as-approver: expected 403, got %d body=%s", w3.Code, w3.Body.String())
	} else {
		if code := errCode(w3.Body.Bytes()); code != "SOD_VIOLATION" {
			t.Errorf("expected SOD_VIOLATION, got %q", code)
		}
		t.Logf("SoD correctly blocked maker-as-approver for rating_history: 403 SOD_VIOLATION")
	}

	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")
}

// ─── Test 8: Invalid action_type → 422 ───────────────────────────────────────

// TestRatingHistory_InvalidActionType_Returns422 verifies that creating a
// rating_history with an invalid action_type returns 422 VALIDATION_FAILED.
//
// Covers: SoW §3.2 MSTR-003b validation, domain.IsValidActionType().
func TestRatingHistory_InvalidActionType_Returns422(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	cpKode := "RHTESTCP8"
	t.Cleanup(func() { cleanupCounterpartyAndRH(t, infra.DB, cpKode) })
	cleanupCounterpartyAndRH(t, infra.DB, cpKode)

	makerID := seedUserSQL(t, infra.DB, "rh_invact_maker")
	cpID := seedApprovedCounterpartyForRH(t, infra.DB, cpKode, makerID)

	router, _ := buildRatingHistoryRouter(infra.DB)
	claims := rhMakerClaims(makerID)

	body := fmt.Sprintf(`{
		"ratingHistoryIdKode": "RH-INV-ACT-001",
		"counterpartyId": %q,
		"tanggalBerlaku": "2026-06-01",
		"ratingPefindo": "idA",
		"sumberRating": "PEFINDO",
		"tanggalPublikasiRating": "2026-06-01",
		"actionType": "INVALID_ACTION_NOT_IN_WHITELIST",
		"notchChange": 0
	}`, cpID.String())

	w := postJSON(router, "/api/v1/master/rating-history", claims, uuid.New().String(), body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid actionType: expected 422, got %d body=%s", w.Code, w.Body.String())
	}
	if code := errCode(w.Body.Bytes()); code != "VALIDATION_FAILED" {
		t.Errorf("expected VALIDATION_FAILED, got %q", code)
	}
	t.Logf("invalid actionType correctly rejected: 422 VALIDATION_FAILED")
}
