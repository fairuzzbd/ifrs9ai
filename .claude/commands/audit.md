---
description: Jalankan MDA conformance audit (advisory, non-blocking) atas pekerjaan terhadap dokumen sumber kebenaran
argument-hint: <scope — mis. "diff sejak last commit", modul APP-C, PR #12, atau kosong = working tree>
allowed-tools: Read, Grep, Glob, Bash, Task
---

Jalankan **conformance audit** via subagent `mda` (advisory, NON-blocking).

**Scope:** $ARGUMENTS
(kosong = audit perubahan di working tree / diff sejak commit terakhir)

Tugas Anda (sebagai main Claude / orchestrator):
1. Panggil subagent `mda` via Task tool.
2. Berikan scope + konteks: apa yang baru dikerjakan subagent mana, modul yang tersentuh (APP-A/B/C/D/E), apakah menyentuh ECL/EIR/SPPI/BM/auth/audit.
3. MDA akan: baca diff + dokumen relevan → bandingkan → tulis laporan drift di `docs/audit/AUDIT-{yyyymmdd-HHMM}.md` (temuan + severity + saran, BUKAN verdict blocking).
4. Ringkas hasil ke user: jumlah temuan per severity (HIGH/MEDIUM/LOW) + 3 prioritas teratas.
5. **Jangan hentikan kerja** karena audit ini — ini advisory. Temuan HIGH disarankan ditangani sebelum modul berikutnya; MEDIUM/LOW masuk backlog.

Catatan: MDA tidak memblok. Untuk path regulated (ECL/EIR/SPPI/BM → `ifrs9-compliance-reviewer`; auth/PII/audit → `security-engineer`), gate BLOCKING tetap milik agent itu — MDA hanya menandai bila perlu dipanggil.
