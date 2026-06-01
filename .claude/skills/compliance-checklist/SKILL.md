---
name: compliance-checklist
description: Checklist sistematis IFRS9 / PSAK 71 compliance review. Dipakai oleh ifrs9-compliance-reviewer sebagai gate sebelum merge ECL/EIR/SPPI/BM/klasifikasi changes. Output VERDICT PASS / CONDITIONAL-PASS / BLOCK dengan finding terstruktur.
---

# IFRS9 / PSAK 71 Compliance Checklist

## How to use
1. Identifikasi scope: file mana yang berubah, modul mana (SPPI, BM, ECL, EIR, klasifikasi, jurnal).
2. Run checklist per area yang tersentuh.
3. Set verdict per finding: BLOCKER / MAJOR / MINOR.
4. Aggregate: jika ANY BLOCKER → overall BLOCK. Jika hanya MAJOR/MINOR dengan follow-up issue → CONDITIONAL-PASS. Else PASS.

---

## A. SPPI Test
- [ ] **A1** Implementasi 10 questions (Q1–Q10) per FSD-APP-A?
- [ ] **A2** Failure of any question → klasifikasi otomatis ke FVTPL (no ECL)?
- [ ] **A3** `sppi.test_run` versioned per (instrumen_id, assessment_date) — bukan UPDATE?
- [ ] **A4** Re-test triggered saat amendment kontrak yang change cashflow character?
- [ ] **A5** UI menampilkan rationale per question (untuk audit + KOMITE review)?

**Standard ref**: PSAK 71 §B4.1.7–§B4.1.26, FSD-APP-A §SPPI

---

## B. Business Model Assessment
- [ ] **B1** Assessment per portofolio (not per instrument)?
- [ ] **B2** 3 kategori implemented: HTC / HTC_AND_SELL / OTHER?
- [ ] **B3** Matrix SPPI × BM yields klasifikasi correct (lihat psak71-classifier)?
- [ ] **B4** Reklasifikasi BM hanya prospective (no retro adjustment)?
- [ ] **B5** Reklasifikasi BM requires 6-eyes workflow + dokumentasi rationale?
- [ ] **B6** FVOCI Election untuk equity: irrevocable, no recycling P&L on disposal?

**Standard ref**: PSAK 71 §4.1.2, §B4.1.1–§B4.1.6, §5.7.5 (Election)

---

## C. ECL — Calculation
- [ ] **C1** Decimal precision ≥ 8 places (`NUMERIC(10,8)` PD/LGD, `NUMERIC(20,4)` IDR)? **No float64**?
- [ ] **C2** 3 skenario (Good/Normal/Bad), default weights 0.25/0.50/0.25?
- [ ] **C3** ALCO override capability dengan workflow approval?
- [ ] **C4** Dual forward-looking multiplier applied per skenario?
- [ ] **C5** Stage 3 bunga: di **Net Carrying Amount** (Gross − ECL), bukan Gross?
- [ ] **C6** SICR triggers (any of): rating turun ≥ 2 notch, IG→non-IG, DPD ≥ 30?
- [ ] **C7** Cure: 3 bulan berturut-turut criteria + history retained?
- [ ] **C8** Calc run inputs **frozen** di `ecl.ecl_calc_input_snapshot`?
- [ ] **C9** Same snapshot → re-run produces **bit-identical** result?
- [ ] **C10** Per-instrument result line stores ALL inputs + intermediates + final (auditable)?
- [ ] **C11** Sealed run truly immutable (no UPDATE permission)?

**Standard ref**: PSAK 71 §5.5, FSD-APP-C §3 + §4, SoW §4

---

## D. ECL — Special Handling
- [ ] **D1** LPS Aggregator applied **before** ECL (Cash + Deposito, IDR 2bn per nasabah per bank)?
- [ ] **D2** Excess di atas LPS cap di-distribute pro-rata across positions?
- [ ] **D3** Look-through Reksadana: decompose by underlying asset class?
- [ ] **D4** POCI: credit-adjusted EIR sejak inisiasi (no Stage 1)?
- [ ] **D5** Multi-currency: EAD_IDR pakai FX BI JISDOR di tanggal_assessment?

**Standard ref**: SoW §4.5 (LPS), FSD-APP-C §3.5 (look-through), §3.6 (POCI)

---

## E. EIR — Newton-Raphson
- [ ] **E1** Tolerance ≤ 1e-10, max 100 iter?
- [ ] **E2** Fail-safe on non-convergence — explicit error, no silent fallback?
- [ ] **E3** Pakai `shopspring/decimal` end-to-end?
- [ ] **E4** Amendment: insert new `schedule_version`, **never** UPDATE old rows?
- [ ] **E5** POCI: credit-adjusted EIR formula correct?
- [ ] **E6** Property-based test untuk solver (root-finding correctness)?

**Standard ref**: FSD-APP-C §4, SoW §5, Decision Log DEC-013

---

## F. Audit & Workflow
- [ ] **F1** ALCO approval workflow required sebelum activate PD curve / LGD pool / scenario weights / FL multiplier?
- [ ] **F2** Komite Investasi approval untuk klasifikasi PSAK 71 (6-eyes)?
- [ ] **F3** Calc run lifecycle: DRAFT → RUNNING → COMPLETED → SEALED (SEALED immutable)?
- [ ] **F4** Reklasifikasi creates audit chain (link old → new klasifikasi)?
- [ ] **F5** Roll-forward formula correct: opening + transfers + originations − derecognitions ± remeasurements = closing?
- [ ] **F6** Every regulated mutation writes to `aud.audit_log` di same tx?
- [ ] **F7** SoD enforced: maker ≠ reviewer ≠ approver?

**Standard ref**: FSD-MASTER §3 (workflow), §6 (audit), Decision Log DEC-017, DEC-018

---

## G. Jurnal — IFRS9 Transitions
- [ ] **G1** AC → FVOCI: recognize OCI gain/loss per matrix?
- [ ] **G2** FVOCI debt → AC: amortized cost reset (no P&L impact)?
- [ ] **G3** Equity FVOCI on disposal: gain/loss tetap di OCI, **no recycling** ke P&L?
- [ ] **G4** Stage 1 → Stage 2 transition: ECL movement di-book di P&L (impairment), carrying-amount AC tidak change?
- [ ] **G5** Posting jurnal hanya setelah approver sign + workflow state = APPROVED?
- [ ] **G6** Jurnal entry punya reference ke source (instrumen_id, calc_run_id) untuk traceability?

**Standard ref**: PSAK 71 §5.6 (reklasifikasi), Appendix A FSD-APP-D (mapping jurnal)

---

## H. Decision Log Compliance
- [ ] **H1** Tidak ada perubahan yang melanggar DEC-010..018 (ECL/EIR specs)?
- [ ] **H2** Tidak ada perubahan yang melanggar DEC-024..029 (security)?
- [ ] **H3** Jika supersede DEC dibutuhkan: ada RFC formal + stakeholder sign-off?

---

## Output format

```markdown
# IFRS9 Compliance Review — {date}
**Scope:** {files/branches/PR reviewed}
**Reviewer:** ifrs9-compliance-reviewer

## VERDICT: PASS | CONDITIONAL-PASS | BLOCK

## Summary
{1-2 sentences}

## Findings

### BLOCKERS (must fix before merge)
1. **[Section/file/line]** — {description}
   - Standard ref: PSAK 71 §X.Y / FSD §Z / Decision Log DEC-NN
   - Required fix: {specific change}

### MAJOR (should fix before merge; CONDITIONAL-PASS allowed with follow-up issue)
1. ...

### MINOR (track for cleanup)
1. ...

## Evidence of Correctness (if PASS or CONDITIONAL-PASS)
- Test coverage:
  - {test_file:test_name} — covers {scenario}
  - ...
- Decision Log entries confirming approach: DEC-NN, DEC-NN
- Cross-check vs SoW examples: {match/discrepancy}

## Follow-ups (for CONDITIONAL-PASS)
- [ ] Issue #NNN: {description}
- [ ] Orchestrator sign-off: {name}

---
Sign: ifrs9-compliance-reviewer
Date: {ISO 8601}
```

## Examples — common findings

### BLOCKER examples
- "ECL computation menggunakan `float64` di line {file:line} — violates DEC-016, PASS dilarang."
- "EIR amendment melakukan `UPDATE ecl.amortisasi_schedule SET ...` di line {file:line} — destroys audit trail. Wajib insert new `schedule_version` sebagai immutable history."
- "Stage 3 interest revenue dihitung pakai Gross Carrying di `service.go:120` — wajib pakai Net Carrying per PSAK 71 §5.4.1(b)."
- "Calc run dapat di-UPDATE setelah `status='SEALED'` di repo `ecl_run_repo.go` — wajib REVOKE UPDATE permission + service-layer guard."

### MAJOR examples
- "LPS aggregator diterapkan setelah ECL calc, harus sebelum. Saat ini per-position ECL berlebih kalau ada multiple deposito di bank yang sama."
- "Reklasifikasi UI memungkinkan reviewer = maker (cek `klasifikasi_review.go:45`). SoD harus di-enforce di service, bukan hanya UI."

### MINOR examples
- "Comment di formula skenario tidak cite SoW section — tambah `// per SoW v1.4 §4.2`."
- "Test name `TestECL_1` tidak deskriptif — rename ke `TestECL_Stage2_Lifetime_3Scenarios`."

---

## Citation tree
- PSAK 71 (full standard)
- FSD-BLIPS-MASTER-v1.1.docx
- FSD-APP-A, APP-C
- SoW_v1.4.docx
- BLIPS_Decision_Log_v1.0.docx
- @.claude/memory/formulas.md
- @.claude/memory/glossary.md
