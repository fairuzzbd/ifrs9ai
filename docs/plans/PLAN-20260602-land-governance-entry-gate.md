# PLAN-20260602 — Land governance/tooling working-tree changes via legitimate GitFlow

> Source: MDA entry-gate decision `MDA-LEDGER-0004` (2026-06-02T15:10+07:00), verdict **APPROVED bersyarat**.
> Orchestrator: tech-lead-orchestrator. Git executor: devops-engineer.

## Goal

Land the uncommitted working-tree changes on `develop` (pure governance/tooling layer) into
`origin/develop` through the **legitimate GitFlow path** — feature/chore branch → signed commits →
PR → squash merge. **No admin direct push, no `--no-verify`, no disabling branch protection.**

## Classification

- **Type**: chore / docs (repo tooling + governance model). NON-regulated.
- **Scope**: `.claude/**` + `CLAUDE.md` only. Does not touch ECL/EIR/SPPI/BM/klasifikasi/audit/PII
  or any application code. No DEC-001..029 violated.
- **Modules affected**: none (APP-A..E untouched). Schema untouched.

## Affected files (working tree on `develop`)

| File | Status | Commit group |
|---|---|---|
| `CLAUDE.md` | M | a — governance |
| `.claude/AGENT-TEAM.md` | M | a — governance |
| `.claude/agents/mda.md` | M | a — governance |
| `.claude/agents/tech-lead-orchestrator.md` | M | a — governance |
| `.claude/settings.json` | ?? | a — governance |
| `.claude/hooks/multica-orchestrator-start.sh` | M | b — hook |
| `.claude/hooks/multica-orchestrator-posttool.sh` | ?? | b — hook |
| `.claude/hooks/multica-orchestrator-stop.sh` | D | b — hook |
| `.claude/memory/mda-ledger.md` | M | c — ledger/docs |

## Agents involved + handoff order

1. **tech-lead-orchestrator** (me) — plan + brief user on P0-1 precondition + reconcile.
2. **devops-engineer** — git executor: branch, atomic signed commits, push, PR, squash merge, verify signature.
3. (No BA/SA/data-modeler/backend/frontend/QA needed — content is repo tooling, no contract/schema/code change.)
4. **Compliance / security gates**: NOT triggered (no BLOCKING path touched). Confirmed against AGENT-TEAM §3.

## Blocking dependencies

- **P0-1 (PRECONDITION, BLOCKING)** — `commit.gpgsign=true` + `user.signingkey` must be configured
  **before** the landing commit. Currently both UNSET (per MDA-LEDGER-0003 AUD-010 and -0004
  situational note). This is a **developer/user action** — neither MDA, orchestrator, nor
  devops-engineer can generate a signing key. If not resolvable now → **DEFER landing**, do not bypass.
- **C4 (review)** — `.claude/**` CODEOWNERS → `@fairuzzbd` (placeholder for @tech-lead-orchestrator);
  develop protection requires 1 approval + Code Owner review.

## Commit granularity (atomic, Conventional Commits, scope `repo`)

1. `chore(repo): refine mda entry-gate governance model`
   → `.claude/agents/mda.md`, `CLAUDE.md`, `.claude/AGENT-TEAM.md`,
     `.claude/agents/tech-lead-orchestrator.md`, `.claude/settings.json`
2. `chore(repo): redesign multica subagent hook to PostToolUse`
   → `.claude/hooks/multica-orchestrator-start.sh`, `.claude/hooks/multica-orchestrator-posttool.sh`,
     remove `.claude/hooks/multica-orchestrator-stop.sh`
3. `docs(repo): record MDA-LEDGER-0003/0004 audit & landing decision`
   → `.claude/memory/mda-ledger.md`

## CI

- Only blocking check on `develop` is `backend-lint`.
- This PR touches no `backend/**` and no `.github/workflows/ci.yml` → `paths-filter` skips the lint
  step body, but the **job still reports green** as a status check. C3 satisfied.

## Merge

- **Squash and merge** (chore/docs → develop) per git-conventions §Merge strategy. No force-push.

## Risk + rollback

- **Risk**: very low — tooling/docs only, no runtime/schema/contract impact.
- **Rollback**: if the squashed landing commit needs reverting, open a normal `git revert` PR to
  develop. No data implications. Branch protection stays on throughout.

## Verification step (definition of done)

1. Branch `chore/governance-entry-gate-mda` created from `develop`; working changes moved onto it.
2. 3 atomic commits, all **signed** (`git log --show-signature` shows `Good signature` / `G`).
3. PR opened to `develop`; `backend-lint` green; Code Owner + 1 approval.
4. Squash-merged to `develop`; `origin/develop` advanced.
5. `git log --show-signature` on the landing (squash) commit = signed.

## Boundary (carried from MDA-LEDGER-0002 #3, zero tolerance)

No admin direct push to any protected branch without fresh MDA pre-approval. MDA explicitly
**refused** a new exception in -0004. If P0-1 cannot be met, report back to MDA — do not bypass.

## Execution status (2026-06-02) — DEFERRED at P0-1

P0-1 PRECONDITION verified **UNMET** by orchestrator (read-only):
- `/home/tugure/projects/ifrs9ai/.git/config` — no `[commit] gpgsign`, no `user.signingkey`.
- `/home/tugure/.gitconfig` — only GitHub credential helpers; no signing config.

Therefore the signed landing commit required by `develop` branch protection cannot be produced.
Per MDA-LEDGER-0004 C2 + instruction ("Jika P0-1 tidak bisa diselesaikan sekarang → TUNDA landing,
JANGAN bypass"), landing is **DEFERRED**. Configuring a GPG/SSH signing key is a developer/user
action — it is NOT something MDA, orchestrator, or devops-engineer may fabricate.

Next action: user configures signing key (see brief below), then re-run this plan from STEP 1
(branch → atomic signed commits → PR → squash merge). No bypass.
