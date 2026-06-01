# Git Conventions — BLIPS IFRS9

> Branching: **GitFlow** · Commit: **Conventional Commits dengan scope modul** · Tagging: **SemVer 2.0**

## Mengapa GitFlow?
BLIPS adalah regulated financial software dengan:
- Release berkala (bulanan/triwulanan) yang harus di-UAT dulu.
- Hotfix untuk prod issue tanpa membawa fitur in-progress.
- Long-lived develop branch untuk integration UAT.
- Compliance + security gate yang butuh waktu — short-lived trunk tidak cocok.

Trunk-based ditolak karena: tidak ada window untuk regression test ECL/EIR antar release, hotfix bisa membawa fitur half-baked.

---

## Branching strategy

### Branch utama (long-lived, protected)
| Branch | Deploy ke | Yang boleh push langsung |
|---|---|---|
| `main` | Production | Tidak ada (PR only, requires release tag) |
| `develop` | UAT (auto-deploy via CI) | Tidak ada (PR only) |

### Branch kerja (short-lived)
| Pattern | Dari | Ke | Lifetime |
|---|---|---|---|
| `feature/{modul}-{ticket}-{slug}` | `develop` | `develop` (squash) | ≤ 2 minggu |
| `release/v{major}.{minor}.{patch}` | `develop` | `main` (merge commit) + `develop` (merge commit) | ≤ 1 minggu |
| `hotfix/v{major}.{minor}.{patch}` | `main` | `main` (merge commit) + `develop` (merge commit) | ≤ 2 hari |
| `chore/{slug}` | `develop` | `develop` (squash) | ≤ 1 minggu |
| `docs/{slug}` | `develop` | `develop` (squash) | ≤ 3 hari |

**Branch naming examples:**
- `feature/app-c-sicr-trigger`
- `feature/app-b-penempatan-deposito-renewal`
- `release/v1.4.0`
- `hotfix/v1.3.2-ecl-stage3-net-carrying`
- `chore/upgrade-go-1.22`
- `docs/runbook-pefindo-feed`

**Branch yang dilarang:**
- `master` (pakai `main`)
- Branch tanpa prefix (`my-fix`, `temp`, `wip-...`)
- Branch dengan space atau karakter aneh
- Branch personal di repo utama (pakai fork untuk eksplorasi)

### Branch protection rules (set di GitLab)

**`main`:**
- ✅ No direct push (semua via MR)
- ✅ Required approvers: 2 (minimum: 1 tech-lead + 1 domain-specific reviewer)
- ✅ Required pipeline pass: lint + unit + integration + security-scan + e2e
- ✅ Required compliance review (untuk MR yang touch APP-A/APP-C atau path `internal/ecl/`, `internal/sppi/`)
- ✅ Required security review (untuk MR yang touch `internal/auth/`, `internal/audit/`, atau touch PII columns)
- ✅ Force-push: forbidden
- ✅ Delete: forbidden
- ✅ Signed commits required

**`develop`:**
- ✅ No direct push (MR only)
- ✅ Required approvers: 1
- ✅ Required pipeline pass: lint + unit + integration + security-scan
- ✅ Compliance review required jika touch ECL/EIR/SPPI/BM
- ✅ Force-push: forbidden
- ✅ Signed commits required

**`release/*`:**
- ✅ Hanya tech-lead-orchestrator atau ROLE-IT-ADMIN yang boleh create.
- ✅ Hotfix branches: hanya untuk severity ≥ HIGH dengan incident ID.

---

## Commit message — Conventional Commits + scope modul

### Format
```
<type>(<scope>): <subject>

[optional body explaining "why"]

[optional footer with Refs/Closes/Co-authored-by]
```

### Types
| Type | Kapan | Triggers SemVer bump |
|---|---|---|
| `feat` | Fitur baru | MINOR |
| `fix` | Bug fix | PATCH |
| `perf` | Performance tanpa change behavior | PATCH |
| `refactor` | Restructure tanpa change behavior | (none) |
| `docs` | Documentation only | (none) |
| `test` | Add/update tests | (none) |
| `chore` | Maintenance (deps, tooling) | (none) |
| `build` | Build system, CI changes | (none) |
| `revert` | Revert previous commit | PATCH |
| `breaking` | Breaking change (atau `feat!` / `BREAKING CHANGE:` footer) | MAJOR |

### Scopes (BLIPS-specific)
Modul:
- `app-a` — Master Data + SPPI + BM
- `app-b` — Transaction Lifecycle
- `app-c` — ECL + EIR
- `app-d` — Periode Buku + FX + Mapping
- `app-e` — Reporting & Dashboard

Cross-cutting:
- `sec` — Security (auth, RBAC, encryption, audit)
- `db` — Schema, migrations
- `api` — OpenAPI, REST contract
- `web` — Frontend Next.js
- `worker` — Asynq jobs
- `integ` — External integration (Pefindo, IBPA, KSEI, BI, GL)
- `ci` — Pipeline, deployment
- `infra` — Terraform, Ansible, K8s
- `deps` — Dependency updates
- `repo` — Repo tooling, .claude/, conventions

### Subject rules
- Imperative mood: "add", "fix", "remove" (bukan "added", "fixes", "removing")
- Bahasa Inggris (konsisten dengan code identifier)
- Lowercase, no trailing period
- Max 72 karakter
- Reference ticket di footer (bukan di subject)

### Body
- Wajib untuk: `feat`, `fix` non-trivial, `breaking`, `revert`
- Jelaskan **why** (motivasi), bukan **what** (sudah ada di diff)
- Wrap pada 100 karakter
- Boleh markdown sederhana (bullet, code fence)

### Footer
```
Refs: BLIPS-1234
Closes: #45
Co-authored-by: Nama <nama@tugu-re.com>
BREAKING CHANGE: API endpoint /api/v1/ecl renamed to /api/v1/ecl-runs
```

### Contoh good commits
```
feat(app-c): implement SICR trigger for Stage 2 transition

Implements 3 SICR triggers per FSD-APP-C §3.2:
- rating downgrade ≥ 2 notch
- IG to non-IG transition
- DPD ≥ 30

Triggered immediately on rating import (Pefindo feed) and DPD recalc job.
Cure logic deferred to separate PR (BLIPS-1235).

Refs: BLIPS-1234
```

```
fix(app-c): correct Stage 3 interest calculation to use Net Carrying

Per PSAK 71 §5.4.1(b) interest on Stage 3 must accrue on Net Carrying
(Gross - ECL), not Gross. Previous code used Gross which over-stated
interest income by ~12% for Stage 3 instruments.

Detected by ifrs9-compliance-reviewer on PR !245.

Refs: BLIPS-1289
Closes: #189
```

```
feat(sec)!: enforce MFA step-up on hard-close periode buku

BREAKING CHANGE: POST /api/v1/periode/{id}/hardclose now requires
step-up MFA. Clients must call /auth/step-up first and include the
stepup_token in X-Step-Up-Token header.

Refs: BLIPS-1450
```

### Contoh BAD commits (rejected by hook)
```
❌ "fix bug"                          → no type, no scope, vague
❌ "Fixed ECL"                        → past tense, no scope
❌ "feat(app-c): added new feature."  → past tense + trailing period
❌ "wip"                              → not informative
❌ "WIP: ecl stage 2"                 → use draft MR, not commit type
❌ "feat: refactor everything"        → no scope, vague
```

---

## Merge strategy

| Source → Target | Strategy | Why |
|---|---|---|
| `feature/*` → `develop` | **Squash + merge** | Keep develop linear, 1 commit per feature |
| `release/*` → `main` | **Merge commit (no FF)** | Preserve release branch history |
| `release/*` → `develop` (back-merge) | **Merge commit (no FF)** | |
| `hotfix/*` → `main` | **Merge commit (no FF)** | |
| `hotfix/*` → `develop` (back-merge) | **Merge commit (no FF)** | |

**Always** rebase feature branch on `develop` sebelum MR open jika develop sudah maju.

**No fast-forward** untuk release/hotfix supaya `git log --first-parent main` menampilkan release timeline yang clean.

---

## Signed commits

Wajib di `main`, `develop`, `release/*`, `hotfix/*`. Opsional (tapi recommended) di feature branches.

### Setup GPG
```bash
# Generate key
gpg --full-generate-key  # RSA 4096, no expiry
gpg --list-secret-keys --keyid-format=long  # ambil KEY_ID

# Upload public key ke GitLab
gpg --armor --export <KEY_ID>
# paste ke GitLab → User Settings → GPG Keys

# Configure git
git config --global user.signingkey <KEY_ID>
git config --global commit.gpgsign true
git config --global tag.gpgsign true
```

### Setup SSH signing (alternative, lebih simpel)
```bash
git config --global gpg.format ssh
git config --global user.signingkey ~/.ssh/id_ed25519.pub
git config --global commit.gpgsign true
```

CI akan verify signature dan reject MR jika ada unsigned commit di branch target protected.

---

## Hotfix workflow

```
1. Incident in prod (severity HIGH+)
   ↓
2. ROLE-IT-ADMIN atau tech-lead-orchestrator create:
   git checkout main
   git checkout -b hotfix/v1.3.2-ecl-stage3-net-carrying
   ↓
3. Fix + test minimum sufficient (unit + integration; full E2E nanti)
   ↓
4. MR ke main:
   - Required approvers: 2 (1 tech-lead + 1 domain reviewer)
   - Compliance review jika ECL/SPPI/BM (mandatory, blocking)
   - Security review jika auth/PII/audit (mandatory, blocking)
   - CI pipeline pass: lint + unit + integration + security-scan
   ↓
5. Merge to main (merge commit, no-FF)
   ↓
6. Tag: git tag -s v1.3.2 -m "hotfix: ECL Stage 3 net carrying"
   ↓
7. Deploy prod (ROLE-IT-ADMIN trigger via /deploy slash command)
   ↓
8. Back-merge to develop (MR, no-FF) — supaya hotfix tidak hilang saat next release
   ↓
9. Post-mortem doc di docs/incidents/{YYYY-MM-DD}-{slug}.md
```

**Hotfix bukan tempat untuk fitur.** Jika tergoda menambah fitur "sekalian", pisah jadi feature branch normal di develop.

---

## Release workflow

```
1. tech-lead-orchestrator decide to cut release (biasanya end of sprint)
   ↓
2. git checkout develop && git pull
   git checkout -b release/v1.4.0
   ↓
3. Bump version files (web/package.json, cmd/api/version.go, etc.)
   Generate CHANGELOG.md entry (pakai /release slash command)
   Commit: chore(repo): prepare v1.4.0
   ↓
4. Push & MR ke main:
   - All gates: lint + unit + integration + security-scan + e2e
   - Compliance review (mandatory)
   - Security review (mandatory)
   - 2 approvers minimum
   ↓
5. Merge to main (merge commit, no-FF)
   ↓
6. Tag (signed): git tag -s v1.4.0 -m "release: v1.4.0"
   git push origin v1.4.0
   ↓
7. Back-merge release/v1.4.0 → develop (MR, no-FF)
   ↓
8. Delete release branch
   ↓
9. Deploy ke prod via /deploy production
   ↓
10. Announce: GitLab Release notes + email stakeholder
```

---

## SemVer 2.0

`v{MAJOR}.{MINOR}.{PATCH}`

| Bump | Trigger |
|---|---|
| MAJOR | Breaking API change, schema breaking, regulatory recompute |
| MINOR | New feature, new endpoint, new module, additive schema |
| PATCH | Bug fix, security patch, doc-only change |

**MAJOR untuk regulatory recompute**: jika perubahan menyebabkan ECL/EIR previously calculated berubah → MAJOR + dokumentasi reconcile + ALCO approval + back-fill plan.

Pre-release tags: `v1.4.0-rc.1`, `v1.4.0-rc.2` untuk UAT cycle sebelum prod.

---

## Required reviewers per file path (CODEOWNERS-equivalent)

Karena belum ada CODEOWNERS file, sebagai konvensi MR template membutuhkan reviewer berikut berdasarkan path:

| Path | Required reviewers |
|---|---|
| `internal/ecl/**`, `internal/eir/**` | `ecl-eir-engineer` + `ifrs9-compliance-reviewer` (BLOCKING) |
| `internal/sppi/**`, `internal/bm/**` | `business-analyst` + `ifrs9-compliance-reviewer` (BLOCKING) |
| `internal/auth/**`, `internal/audit/**` | `security-engineer` (BLOCKING) |
| `db/migrations/**` | `data-modeler` + `security-engineer` (jika touch `aud/sec`) |
| `api/openapi/**` | `system-analyst` |
| `internal/integration/**` | `integration-engineer` |
| `web/**` | `frontend-engineer-nextjs` |
| `deploy/**`, `.gitlab-ci.yml`, `terraform/**` | `devops-engineer` + `security-engineer` (jika destructive) |
| `.claude/**` | `tech-lead-orchestrator` |

Untuk migration ke CODEOWNERS GitLab file, lihat `.gitlab/CODEOWNERS` (akan ditambah saat ROLE-IT-ADMIN siap).

---

## Pre-commit hooks (recommended)

Pakai [pre-commit](https://pre-commit.com/) framework. Config di `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.5.0
    hooks:
      - id: trailing-whitespace
      - id: end-of-file-fixer
      - id: check-yaml
      - id: check-added-large-files
        args: ['--maxkb=500']
      - id: detect-private-key

  - repo: https://github.com/gitleaks/gitleaks
    rev: v8.18.0
    hooks:
      - id: gitleaks

  - repo: https://github.com/golangci/golangci-lint
    rev: v1.55.0
    hooks:
      - id: golangci-lint

  - repo: local
    hooks:
      - id: conventional-commit
        name: conventional commit message
        entry: scripts/check-commit-msg.sh
        language: script
        stages: [commit-msg]
```

Install:
```bash
pip install pre-commit && pre-commit install --install-hooks
```

---

## CHANGELOG.md

Auto-generated dari conventional commits via `/release` slash command. Format [Keep a Changelog](https://keepachangelog.com/):

```markdown
# Changelog

## [Unreleased]

## [1.4.0] - 2026-06-15
### Added
- (app-c) SICR trigger for Stage 2 transition (BLIPS-1234)
- (app-b) Renewal deposito workflow (BLIPS-1240)

### Fixed
- (app-c) Stage 3 interest now uses Net Carrying (BLIPS-1289)

### Security
- (sec) MFA step-up enforced on hard-close (BLIPS-1450) **BREAKING**

### Changed
- (db) ecl.amortisasi_schedule partition strategy switched to monthly

[1.4.0]: https://gitlab.tugu-re.com/blips/blips-ifrs9/-/compare/v1.3.5...v1.4.0
```

---

## Anti-patterns yang ditolak CI / reviewer

- Force-push ke protected branch
- `git commit --amend` setelah push
- Commit yang berisi binary blob > 500KB tanpa Git LFS
- Commit yang berisi `.env`, `secrets.yaml`, atau file dengan high-entropy strings (gitleaks)
- Commit dengan message `fix`, `wip`, `update`, atau format non-conventional
- Branch dari `main` langsung untuk feature (harus dari `develop`)
- Merge `develop` → `main` tanpa release branch (skipping process)
- Skipping compliance/security review dengan `--no-verify`
- Squashing release atau hotfix branch (harus merge commit)

---

## Lookup cepat

```bash
# Lihat siapa yang touch ECL terakhir
git log --oneline -- internal/ecl/

# Cari commit yang ubah staging logic
git log --all --grep="SICR\|staging"

# List release tags
git tag -l "v*" --sort=-v:refname

# Sign verify
git log --show-signature

# Pre-commit run manual
pre-commit run --all-files
```

## Citation
- Conventional Commits 1.0.0
- SemVer 2.0.0
- GitFlow (Vincent Driessen, 2010)
- @.claude/memory/security-baseline.md (signed commits + secrets policy)
- @.claude/AGENT-TEAM.md (decision rights & reviewer assignment)
