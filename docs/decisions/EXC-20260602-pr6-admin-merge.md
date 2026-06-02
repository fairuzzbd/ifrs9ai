# Exception Record — PR #6 admin merge (governance entry-gate landing)

**Tanggal**: 2026-06-02
**Otorisasi**: MDA-LEDGER-0012 (`.claude/memory/mda-ledger.md`)
**Tipe**: One-time authorized exception (admin merge), bukan governance erosion.

## Ringkasan

PR #6 (`chore/governance-entry-gate-mda` → `develop`, squash commit `5f3ac33`)
di-merge via `gh pr merge 6 --squash --admin`, melewati gate
`required_approving_review_count: 1`.

## Mengapa diizinkan (per MDA-LEDGER-0012 decision #1)

Semua gate substantif TERPENUHI:
- ✅ Semua commit **signed + GitHub-Verified** (SSH ed25519)
- ✅ CI **4/4 hijau** (backend-lint blocking, + backend-test, frontend-lint, frontend-build)
- ✅ Konten **non-regulated** — hanya `.claude/**` + `CLAUDE.md`, tidak ada DEC-001..029 yang dilanggar
- ✅ Gate BLOCKING `ifrs9-compliance-reviewer` & `security-engineer` tidak terpicu (path tidak disentuh)

Yang di-bypass HANYA approval count, karena **structural impossibility**:
GitHub melarang author meng-approve PR sendiri; `@fairuzzbd` adalah author
**dan** satu-satunya Code Owner (solo-dev personal repo). Tidak ada manusia
kedua yang secara teknis bisa memberi approval.

## Beda dari MDA-LEDGER-0002 (bootstrap admin direct push)

LEDGER-0002 = admin **direct push** (bypass PR + CI + signed semua sekaligus).
EXC ini = admin **merge PR** yang sudah lolos CI + signed commits; hanya
review-count yang di-skip. Boundary LEDGER-0002 #3 (zero-tolerance direct push)
**tidak dilanggar** (klarifikasi MDA-LEDGER-0012 decision #3).

## Precedent rule (MDA-LEDGER-0012 decision #5)

PR governance/tooling solo-dev berikutnya boleh admin-merge TANPA re-eskalasi MDA
**jika dan hanya jika SEMUA**: (a) konten hanya `.claude/**`, `CLAUDE.md`, `docs/**`,
`*.md`, `.github/` governance — nol kode aplikasi; (b) CI semua hijau termasuk
backend-lint; (c) semua commit signed + Verified; (d) structural impossibility
terbukti (single collaborator = author); (e) tidak menyentuh path BLOCKING
(ecl, eir, sppi, bm, auth, audit, db/migrations). Wajib append ledger entry singkat.
Jika satu kondisi tidak terpenuhi → eskalasi MDA wajib. Rule gugur otomatis saat
tim berkembang (2+ collaborator independen).
