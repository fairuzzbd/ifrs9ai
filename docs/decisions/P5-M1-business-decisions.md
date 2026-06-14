# Business Decisions — Phase 5 Module 1 (P5-M1)

**Document ID**: DEC-P5-M1-004, DEC-P5-M1-005
**Date**: 2026-06-14
**Issued by**: business-analyst
**Status**: PROPOSED — pending formal stakeholder sign-off per RACI (BRD §3, §6.2)
**Resolves**: OQ-M1-1a (settlement balance validation), OQ-M1-5a (terminate workflow eyes)
**Predecessor doc**: `docs/decisions/P5-M1-locked-decisions.md` (DEC-P5-M1-001..003)

---

## Simulated Stakeholder Consultation (per RACI BRD §3)

Business decisions below were derived through simulated consultation with the following RACI owners. Formal sign-off from these parties is required before implementation begins.

| Decision | Consulted (C) | Accountable (A) | Informed (I) |
|---|---|---|---|
| DEC-P5-M1-004 (settlement validation) | Kepala Treasury, Treasury Manager | Kepala Treasury | ROLE-IT-ADMIN, ROLE-RISK, ROLE-AKUN-CTL |
| DEC-P5-M1-005 (terminate workflow) | Kepala Treasury, ROLE-RISK, ROLE-AKUN-CTL | Kepala Treasury | ROLE-AUDIT, ROLE-CFO |

Consultation notes:
- GL Host integration (P5-M14) is explicitly deferred to Phase 6 per Phase 5 roadmap (PR #99). Real-time balance check is architecturally impossible in Phase 5 Sprint 1 without this dependency.
- Treasury operating practice: termination of a deposito before maturity is not a routine daily event but a structured lifecycle event with material financial impact, placing it firmly in the same control tier as origination.

---

## DEC-P5-M1-004 — Settlement Balance Validation Policy

**Status**: PROPOSED
**OQ resolved**: OQ-M1-1a
**Story affected**: P5-M1-S1 (Create Penempatan Deposito)

### Decision: Option B — Informational display only; no blocking on balance

Settlement balance is displayed as an informational hint on the Create Penempatan form using the most recent manually entered balance figure from `sys.settlement_account_balance`. The system does NOT block submission or creation if the nominal exceeds the displayed balance. ERR-VAL-2002 (insufficient balance hard block) is deferred and will not be implemented in Phase 5.

The scenario "Create gagal karena nominal melebihi saldo rekening settlement (ERR-VAL-2002)" in P5-M1-S1 is REMOVED from the Acceptance Criteria for Phase 5. It is logged as a future enhancement for Phase 6, contingent on P5-M14 GL Host integration going live.

### Rationale — Why Option B

**Option A (real-time GL block)** is architecturally blocked in Phase 5. P5-M14 GL Host integration (REST real-time) is scheduled for Phase 6 per Decision DEC-005 (GL Integration Phase 1 deferred) and the Phase 5 roadmap (PR #99). Implementing a hard real-time block without P5-M14 would require either (a) polling a stale manual balance table — which creates false security — or (b) blocking Treasury operations when the check system is unavailable, introducing unacceptable operational risk.

**Option C (threshold-based IDR 50 miliar)** is rejected because it introduces inconsistent behavior: the same form behaves differently depending on the nominal value, creating confusion for Makers and inconsistent audit trail semantics. A partial real-time check against a stale balance store is not meaningfully safer than no check at all.

**Option B (informational)** is correct for Phase 5 because:
1. Treasury Manager reviews and approves each penempatan under the 4-eyes workflow (DEC-017). Human review is the primary control at this lifecycle stage, not system enforcement.
2. Displaying the last-known balance provides useful context without creating a false sense of automated control that does not exist.
3. It does not block Treasury operations when no GL Host query is possible.
4. When P5-M14 GL Host goes live, the informational display can be upgraded to a real-time check with a blocking rule in a targeted Phase 6 story, without any schema or API contract change (the `settlement_account_id` field and balance display component are already in place).

### Phase 6 Upgrade Path

When P5-M14 GL Host integration is operational:
- New story: P6-M14-Sx "Real-time settlement balance validation on penempatan create/approve"
- ERR-VAL-2002 re-activated as a hard block at CREATE (or optionally at APPROVE)
- Threshold-based Option C can be re-evaluated at that time if ALCO deems it appropriate for large-nominal instruments

### Schema impact

No dedicated balance-check table is required in Phase 5. A lightweight `sys.settlement_account_balance` record per `settlement_account_id` (last_known_balance, as_of_date, updated_by) is sufficient for the informational display. Data-modeler to include as part of migration 000028 or as a standalone patch migration.

| Column | Type | Note |
|---|---|---|
| `settlement_account_id` | TEXT PK | Matches `trx.penempatan_deposito.settlement_account` |
| `last_known_balance_idr` | NUMERIC(20,4) | Manually entered or future GL feed |
| `as_of_date` | DATE | Date of last update |
| `updated_by` | UUID FK `sec.user.id` | ROLE-AKUN who entered it |
| `updated_at` | TIMESTAMPTZ | |

### Implementation guard

```
At penempatan CREATE:
  IF sys.settlement_account_balance EXISTS for settlement_account:
    Return balance hint in response body:
      "settlement_balance_hint": {
        "last_known_idr": 3000000000.0000,
        "as_of_date": "2026-06-13",
        "is_sufficient": null   ← always null (no blocking check)
      }
  ELSE:
    Return balance hint absent → UI displays "Saldo tidak tersedia"

No HTTP 422 / ERR-VAL-2002 raised. No block.
```

### Cross-references

- DEC-005 (GL Integration Phase 1 deferred)
- DEC-017 (4-eyes workflow = primary control)
- DEC-018 (audit trail)
- DEC-021 (Idempotency-Key on create)
- P5-M14 (GL Host integration, Phase 6 prerequisite)

---

## DEC-P5-M1-005 — Termination Workflow Eyes

**Status**: PROPOSED
**OQ resolved**: OQ-M1-5a
**Story affected**: P5-M1-S5 (Mature / Terminate)

### Decision: Option B — Full 4-eyes consistent with penempatan create workflow

Manual early termination of a penempatan deposito (before `tanggal_jatuh_tempo`) requires the same 4-eyes workflow as creation: Maker (ROLE-MAKER-TR) → Reviewer (ROLE-APPR-TR) → Approver (ROLE-APPR-TR, Treasury Manager). SoD enforced: `maker_id ≠ reviewer_id ≠ approver_id`.

The P5-M1-S5 story state machine is updated: `APPROVED_ACTIVE → TERMINATION_PENDING_REVIEW` (not directly to `TERMINATION_PENDING_APPROVAL`). A separate reviewer sign-off step is inserted before the Treasury Manager's approval.

### Rationale — Why Option B

**Early termination has material financial impact** across three systems that are all downstream of P5-M1:
1. **EIR re-computation**: amortization schedule is terminated mid-stream; a catch-up EIR_CATCH_UP_ADJUSTMENT entry (event code 4 in DEC-P5-M1-002) is triggered.
2. **ECL derecognition**: `ecl.stage_history` must record derecognition; any Stage 2/3 instrument with ECL reserves requires reversal (ECL_REVERSAL, event code 13).
3. **Realized gain/loss**: journal entry PENJUALAN_PENCAIRAN (event code 16) is emitted; P&L impact is material and immediately visible in period reporting.

Given these downstream effects, a 2-eyes shortcut (Option A) would bypass the independent reviewer who checks that the termination rationale is adequate, the supporting documents are present, and the financial impact is understood before the Treasury Manager approves. This is a weaker control than origination, which is inconsistent: it is harder to justify to auditors why acquiring IDR 5 bn requires 4-eyes but terminating it early requires only 2-eyes.

**DEC-017 classification**: The phrase "4-eyes for routine transactions" in DEC-017 refers to the *workflow tier* (not to frequency). Both origination and termination are routine lifecycle events per instrument — they happen once each per instrument lifecycle. Neither is an "exceptional override." DEC-017 further specifies 6-eyes only for klasifikasi PSAK 71 and parameter master. Termination is a transaction, not a classification or parameter change, so 6-eyes (Option C) would be disproportionate unless ALCO deems the nominal material enough to warrant it.

**Why not Option A (2-eyes)**:
- Weaker than origination control — asymmetric risk management is difficult to defend in audit.
- Treasury Manager approval alone, without an independent reviewer, increases the risk of a unilateral decision on a potentially significant P&L event.
- Inconsistent UX: Makers and Approvers already know the 4-eyes pattern from create; a different pattern for terminate adds cognitive load and training cost.

**Why not Option C (6-eyes with ALCO)**:
- PSAK 71 §3.3.2 (modification/derecognition) requires accounting judgment but does NOT mandate a committee approval for individual instrument termination. ALCO approves *parameters* (DEC-017) and is not the right governance layer for individual transaction termination.
- ALCO sign-off for every termination would create an unworkable operational bottleneck, especially for portfolio rebalancing scenarios.
- Option C is reserved for future consideration if risk appetite changes or if Kepala Treasury requests escalation for large-nominal terminations (see Phase 6 enhancement below).

### State machine update

```
APPROVED_ACTIVE
  ├─ [Maker propose terminate] ──────────────────► TERMINATION_PENDING_REVIEW
  │                                                        │
  │                              [Reviewer sign-off] ──────┤
  │                                                        ▼
  │                                          TERMINATION_PENDING_APPROVAL
  │                                                        │
  │                         [Approver (TM) approve] ───────┼──► TERMINATED
  │                                                        │
  │                              [Reject at any step] ─────►  APPROVED_ACTIVE
  │                                                             (proposal dropped, instrument resumes normal)
```

New states added to `workflow_status` enum:
- `TERMINATION_PENDING_REVIEW`
- `TERMINATION_PENDING_APPROVAL`

These are in addition to the existing `TERMINATED` state. Data-modeler to add to migration 000028 `trx.penempatan_deposito.workflow_status` CHECK constraint.

### New fields required

| Column | Type | Note |
|---|---|---|
| `terminate_reviewer_id` | UUID FK `sec.user.id` | Reviewer for termination (≠ maker, ≠ terminate_approver) |
| `terminate_reviewer_signed_at` | TIMESTAMPTZ | |
| `terminate_reviewer_signature_hash` | TEXT | SHA-256 |
| `terminate_approver_id` | UUID FK `sec.user.id` | Treasury Manager approver |
| `terminate_approver_signed_at` | TIMESTAMPTZ | |
| `terminate_approver_signature_hash` | TEXT | |

Note: The existing `terminate_reason` (≥ 30 chars) and `terminated_at` columns in the migration 000028 draft are retained.

### New endpoints

| Method | Path | Actor | Note |
|---|---|---|---|
| `POST` | `/api/v1/transaksi/penempatan/{id}/terminate-request` | ROLE-MAKER-TR | Propose terminate, move to TERMINATION_PENDING_REVIEW |
| `POST` | `/api/v1/transaksi/penempatan/{id}/terminate-review` | ROLE-APPR-TR (≠ maker) | Review step, move to TERMINATION_PENDING_APPROVAL |
| `POST` | `/api/v1/transaksi/penempatan/{id}/terminate-approve` | ROLE-APPR-TR (TM, ≠ maker, ≠ reviewer) | Final approve, move to TERMINATED |
| `POST` | `/api/v1/transaksi/penempatan/{id}/terminate-reject` | ROLE-APPR-TR | Reject at review or approve; move back to APPROVED_ACTIVE |

Idempotency-Key wajib pada semua 4 endpoint (DEC-021).

### SoD enforcement

```go
// Service layer — terminate-review step
if penempatan.MakerID == currentUser.ID {
    return ErrSoDViolation("maker cannot review termination proposal")
}

// Service layer — terminate-approve step
if penempatan.MakerID == currentUser.ID || penempatan.TerminateReviewerID == currentUser.ID {
    return ErrSoDViolation("maker or termination reviewer cannot approve termination")
}
```

### Audit events (additions to existing list in P5-M1-S5)

| Action | Trigger |
|---|---|
| `PENEMPATAN.TERMINATE_PROPOSED` | Maker propose (exists) |
| `PENEMPATAN.TERMINATE_REVIEWED` | NEW — Reviewer sign-off |
| `PENEMPATAN.TERMINATE_APPROVED` | Approver final (exists) |
| `PENEMPATAN.TERMINATE_REJECTED` | Reject at any step (exists) |
| `PENEMPATAN.DERECOGNITION_QUEUED` | Post-approve event to P5-M9 (exists) |

### Phase 6 enhancement (future, not scope of P5-M1)

If Kepala Treasury requests escalation control for large-nominal terminations (e.g., nominal > IDR 100 miliar), a future story can add a threshold-based 6-eyes path where ALCO co-signs the `terminate-approve` step. This does not require a schema change — it requires only a configurable nominal threshold in `sys.parameter` and a conditional workflow branch.

### Cross-references

- DEC-017 (4-eyes SoD — termination is a "routine transaction")
- DEC-018 (audit trail immutable)
- DEC-021 (Idempotency-Key on all mutating endpoints)
- DEC-027 (step-up MFA not triggered for terminate; termination is not in the MFA step-up list — only hard-close, ECL parameter approve, klasifikasi approve, calc-run seal)
- DEC-P5-M1-002 (event codes 4, 13, 16 triggered by termination)
- P5-M9 (derecognition + realized G/L consumer of PenempatanTerminatedEvent)
- PSAK 71 §3.3.2 (modification / derecognition accounting)

---

## Implementation Checklist

- [ ] Update `docs/stories/phase-5/P5-M1-penempatan-deposito.md`:
  - Mark OQ-M1-1a RESOLVED with ref DEC-P5-M1-004
  - Remove ERR-VAL-2002 scenario from S1 Acceptance Criteria (or mark as Phase 6 deferred)
  - Add Gherkin scenario: settlement balance displayed as informational hint
  - Mark OQ-M1-5a RESOLVED with ref DEC-P5-M1-005
  - Update S5 state machine diagram: insert TERMINATION_PENDING_REVIEW step
  - Add Gherkin scenarios for terminate-review step (new) and terminate-reject-at-review step
  - Update S5 permissions table: add `transaksi.review` for terminate-review step
  - Update S5 audit events: add PENEMPATAN.TERMINATE_REVIEWED
- [ ] Update `docs/decisions/P5-M1-locked-decisions.md` Implementation Checklist:
  - Add line: OQ-M1-1a resolved (DEC-P5-M1-004) + OQ-M1-5a resolved (DEC-P5-M1-005)
- [ ] Hand off to `system-analyst`:
  - 2 additional endpoints for terminate-review + terminate-reject-at-review step
  - Updated state machine with TERMINATION_PENDING_REVIEW state
  - `settlement_balance_hint` response field in POST /api/v1/transaksi/penempatan (S1)
  - New error removed: ERR-VAL-2002 no longer in scope for Phase 5
- [ ] Hand off to `data-modeler` for migration 000028:
  - New `workflow_status` enum values: `TERMINATION_PENDING_REVIEW`, `TERMINATION_PENDING_APPROVAL`
  - New columns: `terminate_reviewer_id`, `terminate_reviewer_signed_at`, `terminate_reviewer_signature_hash`, `terminate_approver_id`, `terminate_approver_signed_at`, `terminate_approver_signature_hash`
  - New table: `sys.settlement_account_balance` (or as patch migration 000028a)
- [ ] Kepala Treasury formal sign-off on DEC-P5-M1-004 before S1 backend implementation
- [ ] Kepala Treasury + ROLE-RISK + ROLE-AKUN-CTL formal sign-off on DEC-P5-M1-005 before S5 backend implementation
- [ ] `security-engineer` review: new SoD assertions for terminate-review and terminate-approve
- [ ] `ifrs9-compliance-reviewer` confirms: 4-eyes terminate is sufficient for PSAK 71 §3.3.2 derecognition — no additional gate required beyond the existing EIR/ECL downstream event chain

---

## Stakeholder Sign-off Section

The decisions above are PROPOSED. They become LOCKED once the following sign-offs are collected and recorded here.

| Role | Person | Decision(s) | Sign-off method | Date | Status |
|---|---|---|---|---|---|
| Kepala Treasury | — | DEC-P5-M1-004, DEC-P5-M1-005 | Email to project email + recorded in minutes | — | PENDING |
| Treasury Manager | — | DEC-P5-M1-004 | Email | — | PENDING |
| ROLE-RISK | — | DEC-P5-M1-005 | Email | — | PENDING |
| ROLE-AKUN-CTL | — | DEC-P5-M1-005 | Email | — | PENDING |
| `ifrs9-compliance-reviewer` | (agent gate) | DEC-P5-M1-005 (PSAK 71 §3.3.2 adequacy) | PR review comment | — | PENDING |
| `security-engineer` | (agent gate) | DEC-P5-M1-005 (SoD + audit) | PR review comment | — | PENDING |

Once all PENDING rows are filled, update the Status field in this document header to LOCKED and add the supersession reference in `BLIPS_Decision_Log_v1.0.docx`.

---

## References

- BRD_BLIPS_IFRS9_v1.1.docx §3 (RACI), §6.2 (APP-B Transaction Lifecycle)
- SoW_v1.4.docx §5.2 (penempatan fields), §5.10 (termination / derecognition)
- FSD-APP-B-TransactionLifecycle-v1.1.docx §1 (penempatan), §4 (pencairan/termination), §5 (jatuh tempo)
- FSD-APP-C §10.1 (EIR catch-up on amendment/termination)
- PSAK 71 §3.3.2 (modification and derecognition), §5.4.1 (EIR), §5.5.8 (ECL reversal)
- BLIPS_Decision_Log_v1.0.docx: DEC-005, DEC-017, DEC-018, DEC-021, DEC-027
- `docs/decisions/P5-M1-locked-decisions.md`: DEC-P5-M1-001 (FVTPL staging), DEC-P5-M1-002 (event codes), DEC-P5-M1-003 (6-eyes jurnal mapping)
- Phase 5 roadmap: `docs/plans/phase-5-roadmap.md` (PR #99)
- P5-M1 stories: `docs/stories/phase-5/P5-M1-penempatan-deposito.md` (PR #100)
- P5-M9 (derecognition consumer), P5-M14 (GL Host, Phase 6 prerequisite for DEC-P5-M1-004 upgrade)
