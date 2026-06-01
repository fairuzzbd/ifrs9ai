<!-- GitLab MR template — BLIPS IFRS9. Auto-loaded saat open MR. -->

## What
<!-- 1-3 kalimat: apa yang berubah. -->


## Why
<!-- Motivasi: link ke story/ticket BRD/FSD. Jika menyentuh Decision Log, sebut DEC-NN. -->

- Refs: BLIPS-
- FSD section: 
- Decision Log: 

## How tested
<!-- Daftar test layer + perintah untuk reproduce. -->

- [ ] Unit: `go test ./internal/<modul>/... -race`
- [ ] Integration: `go test ./internal/test/integration/... -tags=integration`
- [ ] E2E: `pnpm --filter web playwright test <spec>` (jika UI berubah)
- [ ] Load: `k6 run tests/load/<scenario>.js` (jika perf-sensitive)
- [ ] Manual UAT: lihat `docs/uat/<modul>/<process>-uat-*.md`

Test result summary:
```
PASS: ...
FAIL: ...
```

---

## Modul tersentuh
<!-- Centang yang relevan. -->

- [ ] APP-A — Master Data + SPPI + BM
- [ ] APP-B — Transaction Lifecycle
- [ ] APP-C — ECL + EIR  ⚠️ butuh `ifrs9-compliance-reviewer`
- [ ] APP-D — Periode Buku + FX + Mapping
- [ ] APP-E — Reporting & Dashboard
- [ ] Cross-cutting: `sec`, `db`, `api`, `web`, `worker`, `integ`, `ci`, `infra`

---

## Required reviewers (BLOCKING)
<!-- Centang reviewer yang sudah sign-off + assign di GitLab. -->

### Always
- [ ] `tech-lead-orchestrator` atau senior peer

### Conditional (centang jika applicable)
- [ ] `ifrs9-compliance-reviewer` — touch ECL/EIR/SPPI/BM/klasifikasi/reklasifikasi/jurnal-transition
- [ ] `security-engineer` — touch auth, RBAC, audit_log, encryption, PII, secrets, signed-action
- [ ] `data-modeler` — schema change (migration di `db/migrations/`)
- [ ] `system-analyst` — API contract change (`api/openapi/*.yaml`)
- [ ] `ecl-eir-engineer` — touch `internal/ecl/`, `internal/eir/`
- [ ] `integration-engineer` — touch `internal/integration/`
- [ ] `frontend-engineer-nextjs` — touch `web/`
- [ ] `devops-engineer` — touch `deploy/`, `.gitlab-ci.yml`, `terraform/`, `ansible/`

---

## Pre-merge checklist

### Code quality
- [ ] `golangci-lint run` pass
- [ ] `pnpm lint` pass (untuk web)
- [ ] No `TODO` / `FIXME` baru tanpa link issue
- [ ] No `console.log`, `fmt.Println`, atau debug print

### Compliance (jika menyentuh ECL/EIR/SPPI/BM)
- [ ] Pakai `shopspring/decimal`, **tidak ada `float64`** untuk money/rate
- [ ] Precision ≥ 8 decimal places
- [ ] Calc run input frozen di snapshot table
- [ ] Same snapshot → bit-identical result (test ada)
- [ ] Stage 3 bunga di Net Carrying
- [ ] Reference SoW/FSD section di code comment

### Security
- [ ] Permission check `{entity}.{action}` (bukan role string)
- [ ] Idempotency-Key validated untuk endpoint mutating
- [ ] SoD: `maker_id ≠ reviewer_id ≠ approver_id` di service layer
- [ ] Audit log row tulis di **same transaction** dengan mutation
- [ ] No PII di log/error message
- [ ] No secret/credential di code

### Database
- [ ] Migration `up` + `down` keduanya ada
- [ ] Audit columns di semua tabel baru (kecuali `aud.*`)
- [ ] No hard delete di `aud`, `jrnl`, `ecl`
- [ ] FK punya index
- [ ] Money fields `NUMERIC` (bukan `FLOAT`)

### API
- [ ] OpenAPI yaml di-update
- [ ] Error codes terdaftar di `mem-api-conventions.md`
- [ ] Cursor pagination (bukan offset)
- [ ] Headers wajib documented

### Frontend
- [ ] Generated API client dari OpenAPI (bukan handwritten)
- [ ] Zod schema mirror backend validation
- [ ] Workflow UX consistent (`MakerReviewerApproverPanel` dst.)
- [ ] WCAG 2.1 AA

### Tests
- [ ] Unit test coverage ≥ 80% untuk file yang diubah
- [ ] Integration test untuk workflow happy path + 1 failure
- [ ] E2E jika UI berubah
- [ ] Test untuk skenario edge case yang relevan

### Docs
- [ ] CHANGELOG.md updated (atau auto-generated via `/release`)
- [ ] Runbook updated jika operational behavior berubah
- [ ] FSD/BRD delta doc di `docs/{stories,erd,decisions}/` jika applicable

### Commit hygiene
- [ ] Commits squashable (atau sudah squashed)
- [ ] Conventional Commits format dengan scope BLIPS
- [ ] Signed commits (`git log --show-signature`)
- [ ] Branch rebased on latest `develop`

---

## Deployment notes
<!-- Ada yang perlu dijalankan di luar normal CI? -->

- [ ] Migration runtime estimasi: 
- [ ] Feature flag: 
- [ ] Config/env var baru: 
- [ ] Asynq job baru yang harus di-register: 
- [ ] Manual data backfill: 
- [ ] Rollback procedure: 

---

## Screenshots / videos
<!-- UI change wajib screenshot before/after. -->


---

## Rollback plan
<!-- Apa yang dilakukan jika ada masalah setelah merge? -->

1. 
2. 

---

## Self-review questions
<!-- Jawab honestly sebelum minta reviewer. -->

- Apakah kode saya memenuhi semua AC di story?
- Apakah ada cara untuk membuat ini lebih simple?
- Apakah saya menambah complexity yang tidak perlu?
- Apakah saya men-test happy path **dan** edge cases?
- Apakah saya leave the code better than I found it?

---

**Reminder**: MR yang menyentuh ECL/EIR/SPPI/BM tanpa centang `ifrs9-compliance-reviewer` akan di-BLOCK oleh CI gate. Lihat @.claude/memory/git-conventions.md.
