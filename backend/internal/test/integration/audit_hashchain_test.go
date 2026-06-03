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
)

// TestAuditHashChain_WriteAndVerify writes several audit events for the same
// entity, verifies the chain is valid, then corrupts one row's current_hash
// and confirms VerifyHashChain detects the break.
//
// Covers: audit hash-chain integrity (regression §7 item 7).
func TestAuditHashChain_WriteAndVerify(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	ctx := makeAuditCtx()
	entityID := uuid.New()
	entityType := "mst.test_instrument_integ"

	writer := audit.NewWriter(infra.DB)
	start := time.Now().Add(-time.Second)

	// Write 3 events in separate transactions (as would happen across business ops).
	for i, action := range []string{"INSTR.CREATE", "INSTR.SUBMIT", "INSTR.APPROVE"} {
		tx, err := infra.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx[%d]: %v", i, err)
		}
		txw := writer.WithTx(tx)
		err = txw.Write(ctx, audit.Event{
			Action:      action,
			EntityType:  entityType,
			EntityID:    entityID,
			After:       map[string]any{"step": i},
			ActorUserID: SystemUserID,
			ActorRole:   "ROLE-MAKER-TR",
		})
		if err != nil {
			_ = tx.Rollback()
			t.Fatalf("Write event[%d] %s: %v", i, action, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit tx[%d]: %v", i, err)
		}
	}

	end := time.Now().Add(time.Second)

	// Verify chain is intact.
	result, err := audit.VerifyHashChain(ctx, infra.DB, start, end)
	if err != nil {
		t.Fatalf("VerifyHashChain: %v", err)
	}
	if !result.IsValid {
		for _, bc := range result.BrokenChains {
			if bc.EntityType == entityType {
				t.Errorf("UNEXPECTED broken chain: %s", bc.Message)
			}
		}
	}
	if result.ValidEvents < 3 {
		t.Errorf("expected at least 3 valid events, got %d", result.ValidEvents)
	}
	t.Logf("chain valid: %d events verified", result.ValidEvents)

	// Now tamper: update current_hash of the SECOND event to break the chain.
	_, err = infra.DB.ExecContext(ctx, `
		UPDATE aud.audit_log
		SET current_hash = '\xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef'::bytea
		WHERE entity_type = $1 AND entity_id = $2
		  AND action = 'INSTR.SUBMIT'
	`, entityType, entityID)
	if err != nil {
		t.Fatalf("tamper update: %v", err)
	}

	// Re-verify — chain must be detected as broken.
	result2, err := audit.VerifyHashChain(ctx, infra.DB, start, end)
	if err != nil {
		t.Fatalf("VerifyHashChain after tamper: %v", err)
	}
	if result2.IsValid {
		t.Error("expected chain to be detected as BROKEN after tamper, but IsValid=true")
	}

	found := false
	for _, bc := range result2.BrokenChains {
		if bc.EntityType == entityType && bc.EntityID == entityID.String() {
			found = true
			t.Logf("tamper detected correctly: %s", bc.Message)
		}
	}
	if !found {
		t.Errorf("tamper not detected for entity %s/%s", entityType, entityID)
	}
}

// TestAuditHashChain_EmptyRange verifies that VerifyHashChain returns IsValid=true
// for a time range with no events (not an error).
func TestAuditHashChain_EmptyRange(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	farFuture := time.Now().Add(365 * 24 * time.Hour)
	result, err := audit.VerifyHashChain(context.Background(), infra.DB,
		farFuture, farFuture.Add(time.Hour))
	if err != nil {
		t.Fatalf("VerifyHashChain on empty range: %v", err)
	}
	if !result.IsValid {
		t.Errorf("empty range should be valid, got IsValid=false")
	}
	if result.TotalEvents != 0 {
		t.Errorf("expected 0 events in far-future range, got %d", result.TotalEvents)
	}
}

// TestAuditHashChain_MultipleEntities verifies independent entity chains
// do not interfere with each other.
func TestAuditHashChain_MultipleEntities(t *testing.T) {
	SkipIfShort(t)
	infra := Setup(t)

	ctx := makeAuditCtx()
	writer := audit.NewWriter(infra.DB)
	start := time.Now().Add(-time.Second)

	for i := 0; i < 3; i++ {
		entityID := uuid.New()
		tx, err := infra.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx entity[%d]: %v", i, err)
		}
		txw := writer.WithTx(tx)
		if err := txw.Write(ctx, audit.Event{
			Action:      "ENTITY.CREATE",
			EntityType:  "mst.multi_entity_test",
			EntityID:    entityID,
			After:       map[string]any{"idx": i},
			ActorUserID: SystemUserID,
			ActorRole:   "ROLE-MAKER-TR",
		}); err != nil {
			_ = tx.Rollback()
			t.Fatalf("Write entity[%d]: %v", i, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit entity[%d]: %v", i, err)
		}
	}

	end := time.Now().Add(time.Second)
	result, err := audit.VerifyHashChain(ctx, infra.DB, start, end)
	if err != nil {
		t.Fatalf("VerifyHashChain: %v", err)
	}
	if !result.IsValid {
		t.Errorf("multi-entity chain expected valid, broken chains: %v", result.BrokenChains)
	}
}

// makeAuditCtx returns a context with minimal claims for audit writes.
func makeAuditCtx() context.Context {
	now := time.Now().Unix()
	exp := now + 3600
	claims := &auth.Claims{
		Sub:               SystemUserID,
		PreferredUsername: "system",
		Roles:             []string{"ROLE-MAKER-TR"},
		Permissions:       []string{"instrumen.create"},
		TenantID:          "TUGURE",
		MFAVerified:       false,
		Exp:               exp,
		Iat:               now,
	}
	return auth.ContextWithClaims(context.Background(), claims)
}

// ensureAuditLog inserts a raw audit_log row for tamper tests (bypasses writer).
func ensureAuditLog(t *testing.T, db *sql.DB, eventID, entityID uuid.UUID, entityType, action string, prevHash, curHash []byte) {
	t.Helper()
	actorUUID, _ := uuid.Parse(SystemUserID)
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO aud.audit_log (
			event_id, timestamp, actor_user_id, actor_role,
			action, entity_type, entity_id,
			trace_id, tenant_id,
			previous_hash, current_hash
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT DO NOTHING`,
		eventID, time.Now(), actorUUID, "ROLE-MAKER-TR",
		action, entityType, entityID,
		"test-trace", "TUGURE",
		prevHash, curHash,
	)
	if err != nil {
		t.Fatalf("ensureAuditLog: %v", err)
	}
}
