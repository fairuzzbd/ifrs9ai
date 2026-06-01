---
description: Plan a non-trivial change via tech-lead-orchestrator — delegates the full handoff flow
argument-hint: <deskripsi request, modul, scope>
allowed-tools: Read, Grep, Glob, Write, Edit, Task
---

Anda akan mendelegasi request berikut ke `tech-lead-orchestrator`:

**Request:** $ARGUMENTS

Tugas Anda (sebagai main Claude):
1. Panggil subagent `tech-lead-orchestrator` via Task tool.
2. Berikan request lengkap + konteks: modul yang terlibat (APP-A/B/C/D/E), role yang relevan, apakah menyentuh ECL/EIR/SPPI/BM (yang butuh compliance gate).
3. Tunggu plan doc dihasilkan (`docs/plans/PLAN-{yyyymmdd}-{slug}.md`).
4. Setelah plan selesai, ringkas ke user: agent mana yang akan dipanggil, urutan, risiko, dan minta konfirmasi sebelum eksekusi.

Reference:
- @.claude/AGENT-TEAM.md — handoff order
- @.claude/memory/locked-decisions.md — locked decisions
