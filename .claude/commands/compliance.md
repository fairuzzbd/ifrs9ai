---
description: Trigger IFRS9 / PSAK 71 compliance review gate sebelum merge
argument-hint: <files / branch / PR yang akan di-review>
allowed-tools: Read, Grep, Glob, Task
---

Panggil subagent `ifrs9-compliance-reviewer` untuk gate review.

**Scope:** $ARGUMENTS

Reviewer akan menjalankan checklist (@.claude/skills/compliance-checklist/SKILL.md):
- SPPI Test (Q1–Q10, FVTPL routing, versioning, re-test on amendment)
- Business Model (3 kategori, matrix SPPI×BM, FVOCI Election irrevocable)
- ECL (presisi 8 desimal, 3 skenario × bobot, dual FL multiplier, Stage 3 Net Carrying, SICR triggers, cure, calc-run reproducibility, LPS aggregator, look-through Reksadana)
- EIR (Newton-Raphson tol 1e-10, amendment versioning, POCI credit-adjusted)
- Audit & Workflow (ALCO approval, Komite 6-eyes, calc-run immutability, roll-forward reconcile)
- Jurnal IFRS9 transitions (AC↔FVOCI, OCI gain/loss, no recycling equity FVOCI, Stage transitions)

Output format:
```
VERDICT: PASS | CONDITIONAL-PASS | BLOCK
FINDINGS: [{severity, description, standard_ref, required_fix}, ...]
```

**BLOCKING**: tidak boleh merge jika BLOCKER unresolved. CONDITIONAL-PASS hanya dengan follow-up issue + orchestrator approval.
