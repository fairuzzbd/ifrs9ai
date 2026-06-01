<!-- GitHub PR template untuk HOTFIX — pakai kalau PR target `main` dari branch `hotfix/*`.
     Auto-load saat URL berisi ?template=hotfix.md
     Contoh: https://github.com/fairuzzbd/ifrs9ai/compare/main...hotfix/v1.3.2-ecl-stage3?template=hotfix.md -->

# 🚨 HOTFIX

## Incident
- **Incident ID**: INC-
- **Severity**: ⬜ CRITICAL  ⬜ HIGH  ⬜ MEDIUM
- **Detected at**:
- **Affected services**:
- **Affected users / roles**:
- **Reported by**:

## Root cause
<!-- 1-3 kalimat. Cite line/file penyebab. -->


## Fix
<!-- 1-3 kalimat. Apa yang diubah. -->


## Why hotfix (bukan feature branch ke develop)?
<!-- Justifikasi: tidak bisa nunggu next release. Contoh: produksi salah hitung ECL, atau security CVE. -->


---

## Testing — minimum sufficient

- [ ] Unit test untuk fix (mandatory)
- [ ] Integration test untuk fix path (mandatory)
- [ ] Manual verification di staging dengan data prod-like
- [ ] E2E akan dijalankan post-merge sebagai smoke

**Tidak butuh full regression** untuk hotfix — fokus pada fix correctness + tidak break compile/contract.

---

## Required approvers — accelerated path

- [ ] `tech-lead-orchestrator` (mandatory)
- [ ] `ROLE-IT-ADMIN` representative (mandatory untuk prod deploy)

### Conditional (centang jika applicable)
- [ ] `ifrs9-compliance-reviewer` — touch ECL/EIR/SPPI/BM ⚠️ **STILL BLOCKING**
- [ ] `security-engineer` — touch auth/PII/audit ⚠️ **STILL BLOCKING**

**Penting**: hotfix tidak men-bypass compliance/security blocking gate. Tetap wajib review jika scope-nya kena.

---

## Pre-merge checklist (ringkas)

- [ ] CI pipeline pass: backend-lint + backend-test + (security-scan, integration setelah ada)
- [ ] Commit signed (GPG/SSH)
- [ ] Branch up-to-date dengan `main`
- [ ] Conventional Commits format (`fix(scope): subject`)
- [ ] Migration (jika ada) reversible
- [ ] CHANGELOG.md entry untuk patch version

---

## Deployment

- [ ] Migration runtime estimasi:
- [ ] Rollback plan tersedia di bawah
- [ ] Comms ke stakeholder:
- [ ] Maintenance window required: ⬜ ya  ⬜ tidak

---

## Post-merge action items

- [ ] Tag `v{major}.{minor}.{patch+1}` (signed) — pakai `gh release create` atau `git tag -s`
- [ ] Deploy prod via `/deploy production`
- [ ] **Back-merge** `hotfix/*` → `develop` (PR terpisah, **Create a merge commit** strategy)
- [ ] Post-mortem doc di `docs/incidents/{YYYY-MM-DD}-{slug}.md` dalam 48 jam
- [ ] Update relevant runbook
- [ ] If regression bug: add regression test ke nightly suite

---

## Rollback plan

Spesifik untuk hotfix ini:
1.
2.

Generic:
```bash
# Re-tag prior version
kubectl set image deployment/blips-api blips-api=registry.tugu-re.com/blips-api:v{previous_tag}
# Verify
kubectl rollout status deployment/blips-api
```

---

**Reminder**: hotfix bukan tempat untuk fitur. Jika ingin "sekalian" tambah sesuatu — buat PR terpisah ke develop sesudahnya.

**Merge strategy**: PR ini WAJIB pakai **Create a merge commit** (no fast-forward) supaya release timeline di `git log --first-parent main` tetap clean. JANGAN squash.
