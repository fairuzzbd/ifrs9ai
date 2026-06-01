---
name: audit-trail-template
description: Template dan helper untuk menulis audit_log row yang konsisten (immutable, hash-chained, lengkap). Gunakan saat backend-engineer-go atau ecl-eir-engineer melakukan mutation yang harus di-audit.
---

# Audit Trail Template

## Tabel kanonik (`aud.audit_log`)

```sql
CREATE TABLE aud.audit_log (
  event_id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_time      TIMESTAMPTZ NOT NULL DEFAULT now(),
  actor_user_id   UUID NOT NULL,
  actor_role      TEXT NOT NULL,         -- e.g. 'ROLE-MAKER-TR'
  action          TEXT NOT NULL,         -- e.g. 'INSTRUMEN.CREATE'
  entity_type     TEXT NOT NULL,         -- e.g. 'mst.instrumen'
  entity_id       UUID NOT NULL,
  before_jsonb    JSONB,                  -- null on INSERT
  after_jsonb     JSONB,                  -- null on DELETE
  ip              INET,
  user_agent      TEXT,
  trace_id        TEXT,
  idempotency_key UUID,
  previous_hash   BYTEA,
  current_hash    BYTEA NOT NULL,
  tenant_id       TEXT NOT NULL DEFAULT 'TUGURE'
) PARTITION BY RANGE (event_time);

-- No DELETE, no UPDATE permission
REVOKE DELETE, UPDATE ON aud.audit_log FROM PUBLIC;
```

## Convention: `action` field

Format: `{ENTITY_UPPER}.{ACTION_UPPER}` atau `{MODULE}.{ENTITY}.{ACTION}`

Standard actions:
- `CREATE`, `UPDATE`, `SOFT_DELETE`, `READ_SENSITIVE` (untuk PII access)
- `SUBMIT`, `REVIEW`, `APPROVE`, `REJECT`, `SEAL`, `REOPEN`
- `POST` (jurnal), `RECONCILE`, `EXPORT`
- `LOGIN`, `LOGOUT`, `MFA_CHALLENGE`, `STEP_UP_AUTH`

Examples:
- `INSTRUMEN.CREATE`
- `SPPI.TEST_RUN.SUBMIT`
- `ECL.CALC_RUN.SEAL`
- `PERIODE.HARDCLOSE`
- `JRNL.POSTING.APPROVE`

## Go helper

```go
// internal/audit/audit.go
package audit

import (
    "context"
    "crypto/sha256"
    "encoding/json"
    "github.com/google/uuid"
)

type Event struct {
    Action         string      // e.g. "INSTRUMEN.CREATE"
    EntityType     string      // e.g. "mst.instrumen"
    EntityID       uuid.UUID
    Before         any         // nil for INSERT
    After          any         // nil for DELETE
    IdempotencyKey *uuid.UUID  // nil if not applicable
}

type Logger struct {
    tx Tx  // active business transaction
}

func (l *Logger) Write(ctx context.Context, e Event) error {
    actor := contextActor(ctx)         // user, role, tenant, trace, IP, user-agent
    prevHash := l.lastHashForActor(ctx) // bytea

    before, _ := json.Marshal(canonicalize(e.Before))
    after,  _ := json.Marshal(canonicalize(e.After))

    payload := struct {
        EventTime     string
        Actor         string
        Role          string
        Action        string
        EntityType    string
        EntityID      string
        Before        json.RawMessage
        After         json.RawMessage
        TraceID       string
        IdempotencyKey *uuid.UUID
        PreviousHash  []byte
    }{
        EventTime: nowISO(),
        Actor:     actor.UserID.String(),
        Role:      actor.Role,
        Action:    e.Action,
        EntityType: e.EntityType,
        EntityID:  e.EntityID.String(),
        Before:    before,
        After:     after,
        TraceID:   actor.TraceID,
        IdempotencyKey: e.IdempotencyKey,
        PreviousHash: prevHash,
    }
    canonical, _ := json.Marshal(payload)
    h := sha256.Sum256(canonical)

    _, err := l.tx.Exec(ctx, `
        INSERT INTO aud.audit_log
          (actor_user_id, actor_role, action, entity_type, entity_id,
           before_jsonb, after_jsonb, ip, user_agent, trace_id,
           idempotency_key, previous_hash, current_hash, tenant_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
    `,
        actor.UserID, actor.Role, e.Action, e.EntityType, e.EntityID,
        before, after, actor.IP, actor.UserAgent, actor.TraceID,
        e.IdempotencyKey, prevHash, h[:], actor.TenantID,
    )
    return err
}

// canonicalize ensures stable hash: sort keys, strip non-deterministic fields
func canonicalize(v any) any {
    if v == nil {
        return nil
    }
    // marshal → unmarshal to map[string]any → sort keys → return
    // Implementation: see audit/canonical.go
    return canonical(v)
}
```

## Usage pattern di service layer

**CRITICAL**: audit write WAJIB **di transaction yang sama** dengan business mutation. Atomicity = audit guarantee.

```go
// internal/instrumen/service.go
func (s *Service) Create(ctx context.Context, in CreateInput) (*Instrumen, error) {
    var inst *Instrumen

    err := s.uow.Run(ctx, func(tx Tx) error {
        // 1. Idempotency check
        if existing, _ := tx.IdempotencyLookup(ctx, in.IdempotencyKey); existing != nil {
            inst = existing.(*Instrumen)
            return nil
        }

        // 2. Business validation
        if err := validate(in); err != nil {
            return err
        }

        // 3. Insert business row
        var err error
        inst, err = tx.Insert(ctx, "mst.instrumen", in.ToEntity())
        if err != nil {
            return err
        }

        // 4. Audit log — SAME transaction
        logger := audit.New(tx)
        if err := logger.Write(ctx, audit.Event{
            Action:         "INSTRUMEN.CREATE",
            EntityType:     "mst.instrumen",
            EntityID:       inst.ID,
            Before:         nil,
            After:          inst,
            IdempotencyKey: &in.IdempotencyKey,
        }); err != nil {
            return err  // rollback whole tx if audit fails
        }

        // 5. Store idempotency response
        return tx.IdempotencyStore(ctx, in.IdempotencyKey, inst)
    })

    return inst, err
}
```

## Hash chain verification

Job harian (`cmd/audit-verify`):
```go
func VerifyChain(from, to time.Time) error {
    rows, _ := db.Query(`
        SELECT event_id, event_time, previous_hash, current_hash,
               actor_user_id, actor_role, action, entity_type, entity_id,
               before_jsonb, after_jsonb, trace_id, idempotency_key
        FROM aud.audit_log
        WHERE event_time BETWEEN $1 AND $2
        ORDER BY event_time, event_id
    `, from, to)

    var prevHash []byte
    for rows.Next() {
        var row AuditRow
        rows.Scan(&row...)

        if !bytes.Equal(row.PreviousHash, prevHash) {
            return fmt.Errorf("chain broken at %s: previous_hash mismatch", row.EventID)
        }

        recomputed := computeHash(row)
        if !bytes.Equal(row.CurrentHash, recomputed) {
            return fmt.Errorf("tamper detected at %s: hash mismatch", row.EventID)
        }

        prevHash = row.CurrentHash
    }
    return nil
}
```

## What gets logged
✅ Every business INSERT / UPDATE / SOFT_DELETE
✅ Workflow transitions (SUBMIT / REVIEW / APPROVE / REJECT / SEAL)
✅ Periode buku state changes (SOFT_CLOSE / HARD_CLOSE / REOPEN)
✅ ECL parameter changes (with old + new values)
✅ Auth events that affect business state (MFA_CHALLENGE_FAIL on sensitive action)
✅ Bulk operations: log per-row OR aggregate with `entity_id = batch_id`

## What does NOT get logged in `aud.audit_log`
❌ Plain READ operations (use `sec.access_log` if needed)
❌ Failed login attempts (use Keycloak event log)
❌ Internal cache hits / metrics

## Anti-patterns
- ❌ Audit write **after** `COMMIT` — race condition, audit can fail silently
- ❌ Audit write di goroutine async — order tidak terjamin
- ❌ Skip audit "karena bulk migration" — buat audit row aggregate dengan `action: 'BULK.MIGRATION'`
- ❌ Include password/JWT/PII raw value di `before/after` — gunakan masked representation
- ❌ Allow `UPDATE` ke `aud.audit_log` — REVOKE permission, hard rule

## Citation
- @.claude/memory/security-baseline.md
- @.claude/memory/db-conventions.md
- FSD-BLIPS-MASTER-v1.1.docx §6 (audit trail requirements)
- Decision Log DEC-018, DEC-028
