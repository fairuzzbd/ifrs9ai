# Runbook: GitHub Branch Protection Setup — BLIPS IFRS9

**Versi**: 1.0
**Tanggal**: 2026-06-02
**Author**: devops-engineer
**Target audience**: ROLE-IT-ADMIN (repo admin yang punya akses `Settings → Branches`)

---

## 1. Tujuan

Setup proteksi branch untuk `main`, `develop`, `release/*`, `hotfix/*` di GitHub repo BLIPS supaya enforce kontrak GitFlow + signed commits + required reviewers + status check yang sudah dijabarkan di `.claude/memory/git-conventions.md`.

Runbook ini menyediakan **dua jalur ekuivalen**:
- **Jalur A — Web UI** (direkomendasikan untuk first-time setup; mudah diaudit visual)
- **Jalur B — `gh api`** (untuk reproducible setup, dokumentasi-as-code, atau re-apply setelah disaster recovery)

Pilih satu jalur per branch; **jangan jalankan keduanya** karena akan overwrite satu sama lain.

---

## 2. Prerequisites

- Anda adalah **admin** repo `github.com/fairuzzbd/ifrs9ai` (cek di Settings → Collaborators)
- File `.github/workflows/ci.yml` sudah merged ke `develop` minimum sekali — supaya nama status check (`backend-lint`, `backend-test`, `frontend-lint`, `frontend-build`) sudah "known" oleh GitHub Branch Protection picker
- File `.github/CODEOWNERS` sudah merged ke `develop` — supaya "Require review from Code Owners" punya rules
- (Opsional, untuk Jalur B) `gh` CLI v2.40+ terinstall dan authenticated:
  ```bash
  gh --version
  gh auth status
  # Kalau belum login:
  gh auth login --scopes "repo,admin:org"
  ```

---

## 3. Jalur A — Web UI step-by-step

### 3.1 `main` branch — PRODUCTION

1. Buka `https://github.com/fairuzzbd/ifrs9ai/settings/branches`
2. Klik **Add branch protection rule** (atau Edit yang sudah ada)
3. **Branch name pattern**: `main`
4. Centang semua berikut:

| Setting | Value | Catatan |
|---|---|---|
| ☑ Require a pull request before merging | ON | |
| └ Require approvals | **2** | 1 tech-lead + 1 domain reviewer |
| └ Dismiss stale pull request approvals when new commits are pushed | ON | Force re-approval kalau ada change |
| └ Require review from Code Owners | ON | Trigger CODEOWNERS rules |
| ☑ Require status checks to pass before merging | ON | |
| └ Require branches to be up to date before merging | ON | Force rebase sebelum merge |
| └ Status checks that are required (ketik nama): | `backend-lint` | Phase 0 |
| └ (tambah saat tersedia) | `backend-test`, `frontend-lint`, `frontend-build` | Phase 2 |
| └ (tambah saat tersedia) | `integration`, `security-scan`, `e2e`, `compliance` | Phase 2+ |
| ☑ Require conversation resolution before merging | ON | |
| ☑ Require signed commits | ON | GPG/SSH |
| ☑ Require linear history | ON | No-FF merge commits tetap di-count linear di first-parent |
| ☑ Do not allow bypassing the above settings | ON | Admin tidak bisa override |
| ☑ Restrict pushes that create matching branches | ON | Force-push forbidden by default |
| ☑ Restrict deletions | ON | |
| ☐ Lock branch | OFF | Allow PRs to merge |
| ☐ Require deployments to succeed before merging | OFF (Phase 0) | Aktifkan Phase 11 setelah ada deploy gate |

5. Klik **Create** / **Save changes**

### 3.2 `develop` branch — UAT integration

Sama pattern, dengan adjustment lebih longgar (sesuai git-conventions.md):

| Setting | Value |
|---|---|
| ☑ Require a pull request before merging | ON |
| └ Require approvals | **1** |
| └ Require review from Code Owners | ON |
| ☑ Require status checks to pass before merging | ON |
| └ Required: `backend-lint` (+ rest as available) | |
| └ Require branches to be up to date | ON |
| ☑ Require conversation resolution | ON |
| ☑ Require signed commits | ON (landing commit; feature branch commits boleh unsigned) |
| ☐ Require linear history | OFF (allow squash/merge mix) |
| ☑ Restrict force-pushes | ON |
| ☑ Restrict deletions | ON |

### 3.3 `release/*` dan `hotfix/*` pattern

GitHub Branch Protection mendukung wildcard pattern. Buat 2 rule:

**Pattern**: `release/*`
- Same as develop minimum, plus:
- ☑ Restrict who can push to matching branches → Add allowlist (tech-lead-orchestrator + ROLE-IT-ADMIN representative GitHub username)

**Pattern**: `hotfix/*`
- Same as `release/*`
- Note: hotfix branches hanya untuk severity ≥ HIGH dengan incident ID di PR description (enforced lewat PR review, bukan branch protection mechanical)

### 3.4 Verifikasi setup

Setelah save, buka tab "Branches" di Settings. Anda akan melihat 4 rule:
```
Branch protection rules
  main        (1 rule applied)        Edit | Delete
  develop     (1 rule applied)        Edit | Delete
  release/*   (1 rule applied)        Edit | Delete
  hotfix/*    (1 rule applied)        Edit | Delete
```

Test cepat:
```bash
# Coba push langsung ke develop (harus REJECTED):
git push origin develop
# Expected: "remote: error: GH006: Protected branch update failed for refs/heads/develop"

# Coba force-push ke feature branch (allowed di feature, tapi kalau ke main/develop harus REJECTED):
git push --force origin develop
# Expected: rejected
```

---

## 4. Jalur B — `gh api` reproducible setup

Untuk **infrastructure-as-code** style setup. Idempotent: re-run akan replace rule existing.

### 4.1 Helper variable

```bash
OWNER=fairuzzbd
REPO=ifrs9ai
# Sanity check
gh api repos/$OWNER/$REPO --jq '.full_name'
# Expected: fairuzzbd/ifrs9ai
```

### 4.2 Protect `main`

```bash
gh api -X PUT repos/$OWNER/$REPO/branches/main/protection \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["backend-lint"]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": true,
    "required_approving_review_count": 2,
    "require_last_push_approval": true
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true,
  "lock_branch": false,
  "allow_fork_syncing": false,
  "required_signatures": true
}
EOF
```

Catatan field:
- `required_signatures: true` — diset via endpoint terpisah pada beberapa versi GitHub API. Kalau di-reject, jalankan tambahan:
  ```bash
  gh api -X POST repos/$OWNER/$REPO/branches/main/protection/required_signatures \
    -H "Accept: application/vnd.github+json"
  ```
- `contexts` harus persis sama dengan `jobs.<job-id>.name` di workflow file. Tambah job lain (`backend-test`, dll) setelah job tersebut hijau minimum sekali di develop.
- `enforce_admins: true` = admin tidak bisa bypass. Ini sengaja, sesuai DEC-018 (audit trail integrity).

### 4.3 Protect `develop`

```bash
gh api -X PUT repos/$OWNER/$REPO/branches/develop/protection \
  --input - <<'EOF'
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["backend-lint"]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": true,
    "required_approving_review_count": 1
  },
  "restrictions": null,
  "required_linear_history": false,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true,
  "required_signatures": true
}
EOF
```

### 4.4 Protect `release/*` dan `hotfix/*` (pattern)

GitHub REST API Branch Protection **per-branch only** — wildcard pattern hanya bisa lewat **Repository Rules** (newer API, v2). Untuk pattern-based protection, pakai:

```bash
# Rule "release-protection"
gh api -X POST repos/$OWNER/$REPO/rulesets \
  --input - <<'EOF'
{
  "name": "release-protection",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {
      "include": ["refs/heads/release/*", "refs/heads/hotfix/*"],
      "exclude": []
    }
  },
  "rules": [
    {"type": "deletion"},
    {"type": "non_fast_forward"},
    {"type": "required_signatures"},
    {
      "type": "pull_request",
      "parameters": {
        "required_approving_review_count": 1,
        "dismiss_stale_reviews_on_push": true,
        "require_code_owner_review": true,
        "required_review_thread_resolution": true
      }
    },
    {
      "type": "required_status_checks",
      "parameters": {
        "strict_required_status_checks_policy": true,
        "required_status_checks": [
          {"context": "backend-lint", "integration_id": null}
        ]
      }
    }
  ],
  "bypass_actors": []
}
EOF
```

(Repository Rules adalah penerus Branch Protection — lebih powerful, pattern-based, layered. Lihat https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets)

### 4.5 Verifikasi via API

```bash
# Dump current protection state untuk audit
gh api repos/$OWNER/$REPO/branches/main/protection | jq .
gh api repos/$OWNER/$REPO/branches/develop/protection | jq .
gh api repos/$OWNER/$REPO/rulesets | jq '.[] | {id, name, target, enforcement}'
```

---

## 5. CODEOWNERS file

File `.github/CODEOWNERS` sudah disediakan terpisah (`.github/CODEOWNERS`). Setelah file masuk ke `develop`:
1. GitHub akan validate sintaks otomatis (banner di file detail page)
2. Toggle "Require review from Code Owners" di Branch Protection mulai berfungsi
3. PR yang touch path BLOCKING tanpa approval Code Owner → tidak bisa merge (sampai owner sign-off)

**Test setup**:
```bash
# Buat dummy PR yang touch backend/internal/ecl/ dummy.go
git checkout -b test/codeowners-trigger
mkdir -p backend/internal/ecl && touch backend/internal/ecl/dummy.go
git add backend/internal/ecl/dummy.go
git commit -m "test: trigger codeowners ecl path"
git push origin test/codeowners-trigger
gh pr create --base develop --title "test codeowners" --body "ignore — testing"
# Cek di GitHub PR: harusnya auto-request review dari owner ecl path
# Hapus PR + branch setelah test:
gh pr close --delete-branch
```

---

## 6. Disaster recovery — re-apply protection

Skenario: protection rule terhapus tidak sengaja (admin error, organisation transfer, dll).

1. Pastikan `.github/workflows/ci.yml` + `.github/CODEOWNERS` masih ada di `develop`/`main`
2. Re-run perintah `gh api` di §4 untuk masing-masing branch
3. Verify via §4.5

**Recovery time**: <5 menit untuk 4 branch dengan API path.

---

## 7. Audit trail

Branch protection change di-log di `Audit log` (Organisation level — kalau punya GH Enterprise) atau `Activity → Audit log` (free tier). Cek periodically:

```bash
gh api repos/$OWNER/$REPO/actions/runs --jq '.workflow_runs[] | {name, status, conclusion}' | head -20
```

Untuk org-level audit log (kalau pakai GH Enterprise):
```bash
gh api orgs/$OWNER/audit-log --jq '.[] | select(.action | startswith("branch_protection")) | {action, actor, created_at, branch}'
```

---

## 8. Sign-off checklist (untuk ROLE-IT-ADMIN yang setup)

Centang setelah verifikasi:

- [ ] Branch protection rule `main` aktif dengan: 2 approvers, CODEOWNERS, status checks blocking, signed commits, linear history
- [ ] Branch protection rule `develop` aktif dengan: 1 approver, CODEOWNERS, status checks blocking, signed commits
- [ ] Rulesets `release/*` + `hotfix/*` aktif (atau dual Branch Protection rule)
- [ ] CODEOWNERS file (`.github/CODEOWNERS`) ada di `develop`, sintaks valid (no "Unknown owner" warning di file UI)
- [ ] Test push langsung ke `develop` → REJECTED
- [ ] Test force-push ke `develop` → REJECTED
- [ ] Test delete `develop` via web UI → button disabled / REJECTED
- [ ] Test bikin PR yang touch `backend/internal/ecl/` → auto-request review dari Code Owner
- [ ] Dump protection state via `gh api ... /branches/main/protection` di-archive sebagai bukti (paste ke `docs/runbooks/audit/branch-protection-{YYYY-MM-DD}.json`)

---

## 9. Output expectation

Setelah semua sign-off di-tick, sampaikan ke `tech-lead-orchestrator`:

> "GitHub Branch Protection aktif untuk main + develop + release/* + hotfix/* per runbook `docs/runbooks/github-branch-protection.md`. CODEOWNERS file aktif. Dump audit di `docs/runbooks/audit/branch-protection-{tanggal}.json`."

Selanjutnya: PR phase-0-finalize bisa di-merge dengan protection enforcement aktif sebagai gate ke-4 ("CI pipeline green") di sign-off checklist Phase 0 → Phase 2.

---

## 10. References

- `.claude/memory/git-conventions.md` — branch protection requirements + reviewer mapping
- `.github/CODEOWNERS` — path → owner mapping
- `.github/workflows/ci.yml` — status check job names
- https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/defining-the-mergeability-of-pull-requests/managing-a-branch-protection-rule
- https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets
- https://docs.github.com/en/rest/branches/branch-protection
