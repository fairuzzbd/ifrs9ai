---
description: Generate release notes + suggest SemVer bump + create signed tag dari conventional commits sejak last tag
argument-hint: <target version, contoh "v1.4.0" atau "auto" untuk auto-bump>
allowed-tools: Read, Grep, Glob, Write, Edit, Bash
---

Lakukan release workflow BLIPS sesuai @.claude/memory/git-conventions.md.

**Target version:** $ARGUMENTS

## Langkah

### 1. Validate state
```bash
!git status                          # working tree harus clean
!git rev-parse --abbrev-ref HEAD     # harus di develop atau release/*
!git tag -l "v*" --sort=-v:refname | head -5   # lihat last tag
```
Jika working tree tidak clean atau tidak di develop/release branch → **STOP**, lapor ke user.

### 2. Detect commits since last tag
```bash
!LAST_TAG=$(git tag -l "v*" --sort=-v:refname | head -1)
!git log "$LAST_TAG"..HEAD --pretty=format:"%h %s%n%b%n---" --no-merges
```

### 3. Parse + classify per conventional type
Group commits ke:
- **Added** — `feat(...)`
- **Fixed** — `fix(...)`
- **Security** — `feat(sec)`, `fix(sec)`, atau body mention "CVE"/"vulnerability"
- **Changed** — `refactor(...)`, `perf(...)`
- **Breaking** — apa pun dengan `!` atau footer `BREAKING CHANGE:`
- **Deps** — `chore(deps)`
- **Docs** — `docs(...)` (ringkas saja di footer)

### 4. Suggest SemVer bump
- Ada BREAKING / `feat!` / `BREAKING CHANGE:` → **MAJOR**
- Ada `feat(...)` saja → **MINOR**
- Hanya `fix` / `perf` / `security patch` → **PATCH**
- Lainnya → no bump (refuse release jika hanya docs)

**Khusus BLIPS — auto-bump ke MAJOR jika:**
- Touch `internal/ecl/` atau `internal/eir/` dengan `feat` (regulatory recompute risk)
- Touch SPPI/BM matrix
- API breaking change

Jika user provide explicit version → respect-it tapi warn jika tidak sesuai detection.

### 5. Generate CHANGELOG.md entry
Format Keep a Changelog. Cek apakah ada `## [Unreleased]` section di CHANGELOG.md; replace dengan `## [<version>] - <date>`. Tambah `## [Unreleased]` kosong di atasnya.

Format setiap entry:
```
- (<scope>) <subject> ([<short-hash>])
```

Kalau ada BLIPS-XXXX di footer commit, append `(BLIPS-1234)`.
Kalau ada `BREAKING CHANGE:`, tag `**BREAKING**`.

Tambah link `[<version>]: https://gitlab.tugu-re.com/blips/blips-ifrs9/-/compare/<prev>...<version>` di bottom.

### 6. Bump version files
Update version string di:
- `web/package.json` → `"version": "<x.y.z>"` (tanpa `v` prefix)
- `cmd/api/version.go` → `const Version = "<x.y.z>"`
- `cmd/worker/version.go` → same

Jika file belum ada, skip tapi catat di output (release notes akan mention "version files not yet wired").

### 7. Commit version bump
```bash
!git add CHANGELOG.md web/package.json cmd/api/version.go cmd/worker/version.go 2>/dev/null
!git commit -S -m "chore(repo): prepare <version>"
```

### 8. Push + create tag (signed)
```bash
!git push origin HEAD
!git tag -s <version> -m "release: <version>"
!git push origin <version>
```

### 9. Output release notes ke user
Format Markdown yang siap di-paste ke GitLab Release:

```markdown
# Release <version> — <date>

## Summary
<1-2 sentence summary dari changes terbesar>

## Highlights
- Top 3 user-visible changes

## Changelog

### Added
- ...

### Fixed
- ...

### Security
- ...

### Changed
- ...

### Breaking changes ⚠️
- ...

### Dependencies
- ...

## Deployment

- Migration required: yes/no
- Downtime expected: ...
- Rollback procedure: see docs/runbooks/rollback-<version>.md

## Compliance attestation

- [ ] ifrs9-compliance-reviewer signed off
- [ ] security-engineer signed off
- [ ] Decision Log entries respected: DEC-NN, DEC-NN

---
Tagged by: <git user>
Signed: yes
```

### 10. Reminder ke user
Setelah tag pushed:
1. Buat MR `release/<version>` → `main` (jika belum)
2. Setelah merge ke main, MR back-merge `release/<version>` → `develop`
3. Trigger deploy prod via `/deploy production`
4. Update GitLab Release page dengan notes yang di-generate
5. Announce ke stakeholder (email + Slack #blips-releases jika ada)

## Anti-actions (jangan lakukan)
- ❌ Tag tanpa signed
- ❌ Tag pada commit yang belum push
- ❌ Skip CHANGELOG update
- ❌ Bump version arbitrary tanpa link ke commits
- ❌ Force-push branch protected
- ❌ Release dari feature branch (harus develop atau release/*)

## Reference
- @.claude/memory/git-conventions.md
- @.claude/memory/locked-decisions.md (untuk validate compliance)
