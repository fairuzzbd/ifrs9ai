---
description: Implement / modify ECL atau EIR logic via ecl-eir-engineer (compliance core)
argument-hint: <fitur ECL/EIR, contoh "implement Stage 2 SICR trigger" atau "EIR re-estimation on amendment">
allowed-tools: Read, Grep, Glob, Write, Edit, Bash, Task
---

Panggil subagent `ecl-eir-engineer`.

**Task:** $ARGUMENTS

Wajib:
1. Baca @FSD-APP-C-ECL-EIR-v1.0.docx (atau Read tool jika docx-skill diperlukan).
2. Baca @.claude/memory/formulas.md untuk reference formula.
3. Pakai `shopspring/decimal` — **never** `float64` untuk money/rate.
4. Precision 8 desimal, rounding per spec di tiap step (dokumentasikan dalam komen).
5. Implement dengan unit test extensive: SPPI fail → FVTPL, staging cure, EIR irregular cashflow, POCI, LPS aggregation, look-through Reksadana.
6. Property-based test untuk IRR solver (skill `eir-newton-raphson` @.claude/skills/eir-newton-raphson/SKILL.md).
7. **Wajib** akhiri dengan panggil `ifrs9-compliance-reviewer` sebelum declare done.

Output: Go code + tests + brief math comment cite SoW/FSD section.
