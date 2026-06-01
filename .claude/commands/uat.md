---
description: Generate UAT script dari user story via qa-engineer
argument-hint: <story-id atau process name, modul>
allowed-tools: Read, Grep, Glob, Write, Edit, Bash, Task
---

Panggil subagent `qa-engineer` untuk menulis UAT script.

**Input:** $ARGUMENTS

UAT format wajib (Bahasa Indonesia, sesuai konvensi BLIPS):
1. **Pre-conditions** — data seed, role assignments, periode buku state, parameter ECL aktif
2. **Step-by-step** — instruksi aktor (mis. "Login sebagai user Treasury Maker → buka menu Penempatan Deposito → isi form dengan data ...")
3. **Expected results** — angka spesifik dari contoh SoW (cite section)
4. **Audit checks** — `aud.audit_log` row muncul, `signature_hash` tercatat, workflow advanced ke step berikutnya
5. **Rollback / cleanup**

Map AC ke test layer:
- Field validation → unit/component test
- Workflow transition → integration test (testcontainers)
- E2E happy path → Playwright

Output: `docs/uat/{module}/{process}-uat-{id}.md` + test files di `tests/`.

Jika menemukan domain bug → route ke `ifrs9-compliance-reviewer`. Security issue → `security-engineer`.
