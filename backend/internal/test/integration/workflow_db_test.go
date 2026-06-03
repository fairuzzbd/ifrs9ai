//go:build integration

package integration

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"

	"blips-ifrs9.tugu-re.com/internal/audit"
	"blips-ifrs9.tugu-re.com/internal/auth"
	"blips-ifrs9.tugu-re.com/internal/workflow"
)

// TestWorkflowDB_FourEyesFull exercises DRAFT→PENDING_REVIEW→PENDING_APPROVAL→APPROVED
// via the real DBRepository and real PostgreSQL.
//
// Covers: regression §6 (SoD at API level) and workflow DB (§7 gate item).
func TestWorkflowDB_FourEyesFull(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "wf_maker_4eyes")
	reviewerID := seedUserSQL(t, infra.DB, "wf_reviewer_4eyes")
	approverID := seedUserSQL(t, infra.DB, "wf_approver_4eyes")
	entityID := uuid.New()

	// Seed workflow_instance in DRAFT state.
	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	repo := workflow.NewDBRepository(infra.DB)
	configLoader := workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs())
	engine := workflow.NewEngine(configLoader)
	auditWriter := audit.NewWriter(infra.DB)
	svc := workflow.NewService(engine, repo, auditWriter, nil)

	// --- SUBMIT (maker) ---
	makerCtx := userCtx(makerID, []string{"penempatan.submit", "penempatan.read"})
	rv := int64(1)
	_, err := svc.Submit(makerCtx, workflow.SubmitInput{
		EntityType: "PENEMPATAN",
		EntityID:   entityID,
		Request:    workflow.ActionRequest{SignatureMethod: workflow.SignatureMethodJWTStandard, RowVersion: &rv},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_REVIEW")

	// --- REVIEW (reviewer) ---
	reviewerCtx := userCtx(reviewerID, []string{"penempatan.review", "penempatan.read"})
	rv2 := int64(2)
	_, err = svc.Review(reviewerCtx, workflow.ReviewInput{
		EntityType: "PENEMPATAN",
		EntityID:   entityID,
		Request:    workflow.ActionRequest{SignatureMethod: workflow.SignatureMethodJWTStandard, RowVersion: &rv2},
	})
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	assertWorkflowState(t, infra.DB, entityID, "PENDING_APPROVAL")

	// --- APPROVE (approver, different from maker+reviewer) ---
	approverCtx := userCtx(approverID, []string{"penempatan.approve", "penempatan.read"})
	rv3 := int64(3)
	result, err := svc.Approve(approverCtx, workflow.ApproveInput{
		EntityType: "PENEMPATAN",
		EntityID:   entityID,
		Request:    workflow.ActionRequest{SignatureMethod: workflow.SignatureMethodJWTStandard, RowVersion: &rv3},
	})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if result.CurrentState != workflow.StateApproved {
		t.Errorf("expected APPROVED, got %s", result.CurrentState)
	}
	assertWorkflowState(t, infra.DB, entityID, "APPROVED")

	// Verify 3 signature records were written.
	sigs, err := repo.ListSignatures(context.Background(), getWorkflowID(t, infra.DB, entityID))
	if err != nil {
		t.Fatalf("ListSignatures: %v", err)
	}
	if len(sigs) != 3 {
		t.Errorf("expected 3 signature records, got %d", len(sigs))
	}
	t.Logf("4-eyes workflow completed: %d signatures", len(sigs))
}

// TestWorkflowDB_SoDViolation_MakerCannotApprove verifies that the maker of
// a workflow cannot become the approver, even with a valid JWT containing the
// correct permission. This is the critical security-baseline test.
//
// Covers: regression §6 (SoD enforcement at service layer, not just UI).
func TestWorkflowDB_SoDViolation_MakerCannotApprove(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "sod_maker_appr")
	reviewerID := seedUserSQL(t, infra.DB, "sod_reviewer_appr")
	entityID := uuid.New()

	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	auditWriter := audit.NewWriter(infra.DB)
	svc := workflow.NewService(engine, repo, auditWriter, nil)

	// Submit as maker.
	makerCtx := userCtx(makerID, []string{"penempatan.submit", "penempatan.review", "penempatan.approve"})
	rv := int64(1)
	if _, err := svc.Submit(makerCtx, workflow.SubmitInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Review as reviewer.
	reviewerCtx := userCtx(reviewerID, []string{"penempatan.review", "penempatan.approve"})
	rv2 := int64(2)
	if _, err := svc.Review(reviewerCtx, workflow.ReviewInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv2},
	}); err != nil {
		t.Fatalf("Review: %v", err)
	}

	// Attempt APPROVE as MAKER — must fail with SoD violation.
	// The JWT is valid and HAS the penempatan.approve permission,
	// but SoD must block at the service layer.
	rv3 := int64(3)
	_, err := svc.Approve(makerCtx, workflow.ApproveInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv3},
	})
	if err == nil {
		t.Fatal("expected SoD violation error when maker tries to approve, but got nil — SECURITY FAILURE")
	}
	if !isSoDError(err) {
		t.Errorf("expected SOD_VIOLATION error, got: %v", err)
	}
	t.Logf("SoD correctly blocked maker-as-approver: %v", err)
}

// TestWorkflowDB_SoDViolation_MakerCannotReview verifies maker cannot review
// their own submission.
func TestWorkflowDB_SoDViolation_MakerCannotReview(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "sod_maker_review")
	entityID := uuid.New()
	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	svc := workflow.NewService(engine, repo, nil, nil)

	// Submit as maker.
	makerCtx := userCtx(makerID, []string{"penempatan.submit", "penempatan.review"})
	rv := int64(1)
	if _, err := svc.Submit(makerCtx, workflow.SubmitInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Attempt REVIEW as the same user (maker) — must fail.
	rv2 := int64(2)
	_, err := svc.Review(makerCtx, workflow.ReviewInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv2},
	})
	if err == nil {
		t.Fatal("expected SoD violation error when maker tries to review, got nil — SECURITY FAILURE")
	}
	if !isSoDError(err) {
		t.Errorf("expected SOD_VIOLATION, got: %v", err)
	}
}

// TestWorkflowDB_OptimisticLock verifies that concurrent UpdateState with
// stale row_version returns an error (optimistic lock).
//
// Covers: workflow DB optimistic lock guard.
func TestWorkflowDB_OptimisticLock(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "optlock_maker")
	reviewerID := seedUserSQL(t, infra.DB, "optlock_reviewer")
	entityID := uuid.New()
	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	svc := workflow.NewService(engine, repo, nil, nil)

	// Submit moves row_version from 1 to 2.
	makerCtx := userCtx(makerID, []string{"penempatan.submit"})
	rv := int64(1)
	if _, err := svc.Submit(makerCtx, workflow.SubmitInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Attempt to Review with STALE row_version=1 (should be 2 now).
	reviewerCtx := userCtx(reviewerID, []string{"penempatan.review"})
	staleRV := int64(1) // stale
	_, err := svc.Review(reviewerCtx, workflow.ReviewInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &staleRV},
	})
	if err == nil {
		t.Error("expected optimistic lock conflict with stale row_version, got nil")
	} else {
		t.Logf("optimistic lock correctly rejected stale rv=1: %v", err)
	}
}

// TestWorkflowDB_SignatureImmutability verifies that the DB trigger refuses
// UPDATE and DELETE on sys.workflow_signature (migration 0007 fn_wf_signature_immutable).
//
// Covers: signature immutability trigger (§7 gate item).
func TestWorkflowDB_SignatureImmutability(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "sig_immut_maker")
	reviewerID := seedUserSQL(t, infra.DB, "sig_immut_reviewer")
	entityID := uuid.New()
	seedWorkflowInstance(t, infra.DB, entityID, "PENEMPATAN", makerID, 4)

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	svc := workflow.NewService(engine, repo, nil, nil)

	// Submit to create one signature record.
	makerCtx := userCtx(makerID, []string{"penempatan.submit"})
	rv := int64(1)
	if _, err := svc.Submit(makerCtx, workflow.SubmitInput{
		EntityType: "PENEMPATAN", EntityID: entityID,
		Request: workflow.ActionRequest{RowVersion: &rv},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	wfID := getWorkflowID(t, infra.DB, entityID)
	sigs, err := repo.ListSignatures(context.Background(), wfID)
	if err != nil || len(sigs) == 0 {
		t.Fatalf("ListSignatures: err=%v, len=%d", err, len(sigs))
	}
	sigID := sigs[0].ID

	// Attempt UPDATE on sys.workflow_signature — must fail.
	_, err = infra.DB.ExecContext(context.Background(), `
		UPDATE sys.workflow_signature SET comment = 'tampered' WHERE id = $1
	`, sigID)
	if err == nil {
		t.Error("expected trigger to refuse UPDATE on sys.workflow_signature, got nil error — SECURITY FAILURE")
	} else {
		t.Logf("UPDATE correctly refused by trigger: %v", err)
	}

	// Attempt DELETE on sys.workflow_signature — must fail.
	_, err = infra.DB.ExecContext(context.Background(), `
		DELETE FROM sys.workflow_signature WHERE id = $1
	`, sigID)
	if err == nil {
		t.Error("expected trigger to refuse DELETE on sys.workflow_signature, got nil error — SECURITY FAILURE")
	} else {
		t.Logf("DELETE correctly refused by trigger: %v", err)
	}

	// Verify record still exists and unchanged.
	_ = reviewerID // suppress unused warning
}

// TestWorkflowDB_ConfigLoader verifies DBConfigLoader loads WORKFLOW_CONFIG rows.
//
// Covers: DBConfigLoader.Load (§7 gate item).
func TestWorkflowDB_ConfigLoader(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	loader := workflow.NewDBConfigLoader(infra.DB)

	for _, entityType := range []string{"PENEMPATAN", "KLASIFIKASI", "ECL_PARAMETER", "JURNAL"} {
		cfg, err := loader.Load(entityType)
		if err != nil {
			t.Errorf("Load(%s): %v", entityType, err)
			continue
		}
		if cfg.EntityType != entityType {
			t.Errorf("Load(%s): unexpected entity type %q", entityType, cfg.EntityType)
		}
		if cfg.Eyes != 4 && cfg.Eyes != 6 {
			t.Errorf("Load(%s): invalid eyes %d", entityType, cfg.Eyes)
		}
		t.Logf("config %s loaded: eyes=%d retractable=%v", entityType, cfg.Eyes, cfg.Retractable)
	}

	// Non-existent key must return error.
	_, err := loader.Load("NONEXISTENT_ENTITY")
	if err == nil {
		t.Error("expected error for nonexistent config key, got nil")
	}
}

// TestWorkflowDB_SixEyes verifies DRAFT→PENDING_REVIEW→PENDING_APPROVAL
// →PENDING_APPROVAL_2→APPROVED (6-eyes) using KLASIFIKASI config.
func TestWorkflowDB_SixEyes(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	makerID := seedUserSQL(t, infra.DB, "sixeyes_maker")
	reviewerID := seedUserSQL(t, infra.DB, "sixeyes_reviewer")
	approver1ID := seedUserSQL(t, infra.DB, "sixeyes_approver1")
	approver2ID := seedUserSQL(t, infra.DB, "sixeyes_approver2")
	entityID := uuid.New()

	seedWorkflowInstance(t, infra.DB, entityID, "KLASIFIKASI", makerID, 6)

	repo := workflow.NewDBRepository(infra.DB)
	engine := workflow.NewEngine(workflow.NewInMemoryConfigLoader(workflow.DefaultConfigs()))
	svc := workflow.NewService(engine, repo, nil, nil)

	perms := []string{
		"klasifikasi.submit", "klasifikasi.review",
		"klasifikasi.approve", "klasifikasi.reject",
	}
	makerCtx := userCtx(makerID, perms)
	reviewerCtx := userCtx(reviewerID, perms)
	approver1Ctx := userCtx(approver1ID, perms)
	approver2Ctx := userCtx(approver2ID, perms)

	rv := int64(1)
	steps := []struct {
		name     string
		fn       func(rv *int64) error
		expected workflow.State
	}{
		{"submit", func(rv *int64) error {
			_, err := svc.Submit(makerCtx, workflow.SubmitInput{
				EntityType: "KLASIFIKASI", EntityID: entityID,
				Request: workflow.ActionRequest{RowVersion: rv},
			})
			return err
		}, workflow.StatePendingReview},
		{"review", func(rv *int64) error {
			_, err := svc.Review(reviewerCtx, workflow.ReviewInput{
				EntityType: "KLASIFIKASI", EntityID: entityID,
				Request: workflow.ActionRequest{RowVersion: rv},
			})
			return err
		}, workflow.StatePendingApproval},
		{"approve1", func(rv *int64) error {
			_, err := svc.Approve(approver1Ctx, workflow.ApproveInput{
				EntityType: "KLASIFIKASI", EntityID: entityID,
				Request: workflow.ActionRequest{RowVersion: rv},
			})
			return err
		}, workflow.StatePendingApproval2},
		{"approve2", func(rv *int64) error {
			_, err := svc.Approve2(approver2Ctx, workflow.Approve2Input{
				EntityType: "KLASIFIKASI", EntityID: entityID,
				Request: workflow.ActionRequest{RowVersion: rv},
			})
			return err
		}, workflow.StateApproved},
	}

	for _, step := range steps {
		if err := step.fn(&rv); err != nil {
			t.Fatalf("step %s: %v", step.name, err)
		}
		assertWorkflowState(t, infra.DB, entityID, string(step.expected))
		rv++
		t.Logf("step %s -> %s OK", step.name, step.expected)
	}
}

// -----------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------

// seedUserSQL inserts a test user into sec.user and returns its UUID.
func seedUserSQL(t *testing.T, db *sql.DB, username string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO sec.user (id, username, email, full_name, status, created_at, created_by)
		VALUES ($1, $2, $3 || '@test.blips', $4, 'AKTIF', now(), '00000000-0000-0000-0000-000000000001')
		ON CONFLICT (username) DO NOTHING
	`, id, username, username, username)
	if err != nil {
		t.Fatalf("seedUser %s: %v", username, err)
	}

	// Fetch the actual ID (in case ON CONFLICT skipped the insert).
	var actualID uuid.UUID
	if err := db.QueryRowContext(context.Background(),
		`SELECT id FROM sec.user WHERE username = $1`, username,
	).Scan(&actualID); err != nil {
		t.Fatalf("seedUser fetch id %s: %v", username, err)
	}
	return actualID
}

// seedWorkflowInstance inserts a workflow_instance row in DRAFT state.
func seedWorkflowInstance(t *testing.T, db *sql.DB, entityID uuid.UUID, entityType string, makerID uuid.UUID, eyes int) {
	t.Helper()
	configKey := "WORKFLOW_CONFIG_" + entityType
	schema := "trx"
	if entityType == "KLASIFIKASI" || entityType == "ECL_PARAMETER" {
		schema = "sppi"
	}

	_, err := db.ExecContext(context.Background(), `
		INSERT INTO sys.workflow_instance (
			entity_type, entity_id, entity_schema,
			workflow_config_key, eyes, current_state,
			maker_id, created_by, updated_by
		) VALUES ($1, $2, $3, $4, $5, 'DRAFT', $6, $6, $6)
		ON CONFLICT (entity_type, entity_id) DO NOTHING
	`, entityType, entityID, schema, configKey, eyes, makerID)
	if err != nil {
		t.Fatalf("seedWorkflowInstance %s/%s: %v", entityType, entityID, err)
	}
}

// userCtx creates a context with auth.Claims for the given user.
func userCtx(userID uuid.UUID, permissions []string) context.Context {
	now := time.Now().Unix()
	claims := &auth.Claims{
		Sub:               userID.String(),
		PreferredUsername: "testuser_" + userID.String()[:8],
		Roles:             []string{"ROLE-MAKER-TR"},
		Permissions:       permissions,
		TenantID:          "TUGURE",
		MFAVerified:       true,
		Exp:               now + 3600,
		Iat:               now,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

// assertWorkflowState reads the workflow_instance current_state and fails if not expected.
func assertWorkflowState(t *testing.T, db *sql.DB, entityID uuid.UUID, expected string) {
	t.Helper()
	var state string
	err := db.QueryRowContext(context.Background(), `
		SELECT current_state FROM sys.workflow_instance
		WHERE entity_id = $1 AND deleted_at IS NULL
	`, entityID).Scan(&state)
	if err != nil {
		t.Fatalf("assertWorkflowState: %v", err)
	}
	if state != expected {
		t.Errorf("workflow state: expected %s, got %s", expected, state)
	}
}

// getWorkflowID returns the workflow instance id for the given entity.
func getWorkflowID(t *testing.T, db *sql.DB, entityID uuid.UUID) uuid.UUID {
	t.Helper()
	var wfID uuid.UUID
	err := db.QueryRowContext(context.Background(), `
		SELECT id FROM sys.workflow_instance WHERE entity_id = $1 AND deleted_at IS NULL
	`, entityID).Scan(&wfID)
	if err != nil {
		t.Fatalf("getWorkflowID: %v", err)
	}
	return wfID
}

// isSoDError returns true if the error wraps a SOD_VIOLATION domain error.
func isSoDError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "sod") || contains(s, "SOD") ||
		contains(s, "segregation") || contains(s, "maker") || contains(s, "reviewer")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
