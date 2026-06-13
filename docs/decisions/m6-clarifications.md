# M6 EIR Amendment Lifecycle — Locked Clarifications

Status: LOCKED. Resolves Issues #67 + #68 from PR #66 compliance review.

## DEC-M6-001 — Drift cron Location is UTC

**Decision**: Asynq scheduler initialized with `Location: time.UTC`. Schedule string `"0 19 * * *"` evaluates as 19:00 UTC, which equals 02:00 WIB (UTC+7) — the FSD-APP-C M6-002 AC §7 requirement of "daily run at 02:00 WIB".

**Rationale**: Locking the scheduler to UTC instead of `Asia/Jakarta` makes the cron schedule deterministic across:
- DST changes in other timezones (irrelevant for Jakarta, but eliminates drift in mirrored DR sites)
- Server timezone misconfiguration (UTC is the canonical baseline per SRE convention)
- Cross-region deployments (Phase 2 DR in different region)

**Evidence in code**:
- `backend/internal/ecl/eir/worker_tasks.go:32` constant `TaskDriftCron` doc comment explicitly states "19:00 UTC = 02:00 WIB".
- `backend/cmd/api/main.go:823-825` `asynq.NewScheduler(..., &asynq.SchedulerOpts{Location: time.UTC})` with rationale comment "so '0 19 * * *' = 19:00 UTC = 02:00 WIB (state-machine §7)".
- `backend/cmd/api/main.go:835` startup info log includes `"schedule": "0 19 * * * UTC"`.

**Implication for operators**: ANY change to the cron schedule string MUST preserve the 02:00 WIB time. If future infra moves scheduler to Asia/Jakarta location, change schedule to `"0 2 * * *"` simultaneously.

Closes Issue #67.

---

## DEC-M6-002 — Auto-created amendment proposal maker identity

**Decision**: When DriftService auto-creates an EIR amendment proposal (CRON_DAILY or MANUAL_AD_HOC drift detection that exceeds `drift_high_threshold`), the proposal's `maker_id` is set as follows:

| Trigger source | maker_id value |
|---|---|
| `CRON_DAILY` (Asynq cron, no actor) | System sentinel UUID `00000000-0000-0000-0000-000000000001` |
| `MANUAL_AD_HOC` (operator-triggered) | The triggering operator's user UUID |
| `PRE_ECL_GATE` (called from M7 calc-run preflight) | The calc-run creator's user UUID |
| `DOCUMENT_UPLOAD` (M6-001 detection) | The uploader's user UUID |

**Rationale**: System-originated proposals (CRON_DAILY only) cannot be cancelled by a human maker because `CancelAmendment` enforces `*proposal.MakerID == req.ActorID` (the maker-only cancel rule from DEC-017 SoD). For these system-originated proposals:
- They CAN be rejected via `RejectAmendment` by RISK or ALCO during normal review flow.
- They cannot be cancelled in the strict sense; rejection is the operator escape hatch.

**Future enhancement** (Phase 5 backlog): introduce a dedicated `eir.amendment.cancel.system_origin` permission allowing ROLE-RISK to cancel system-originated drafts as a maker-substitute.

**Evidence in code**:
- `backend/internal/ecl/eir/drift_service.go` `autoCreateProposalFromDrift` — passes `actorID` from cron handler (which is sentinel UUID when triggered by Asynq scheduler).
- `backend/internal/ecl/eir/detection_service.go:182` `CancelAmendment` enforces maker-only.

**Implication for UI** (M9 amendment queue): system-originated proposals render with a "AUTO (Drift Detection)" badge and the "Cancel" button is hidden/disabled for non-system actors. Rejection remains the only operator action.

Closes Issue #68.

---

References:
- PR #66 compliance review (CONDITIONAL-PASS verdict from ifrs9-compliance-reviewer).
- Issues #67, #68 (this document closes both).
- FSD-APP-C M6-001..005 AC.
- DEC-017 (SoD enforcement), DEC-018 (audit trail), DEC-007 (Asynq).
- `docs/state-machines/p4-m6-amendment-lifecycle.md` §6 (cron schedule) and §7 (state machine).
