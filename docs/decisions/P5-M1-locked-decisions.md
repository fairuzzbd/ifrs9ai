# Locked Decisions — Phase 5 Module 1 (P5-M1)

**Document ID**: DEC-P5-M1-001, DEC-P5-M1-002, DEC-P5-M1-003
**Date**: 2026-06-14
**Issued by**: ifrs9-compliance-reviewer (PR #99/100 follow-up)
**Status**: LOCKED — pending Komite Investasi acknowledgment per RACI (BRD §6.2) and ALCO sign-off for DEC-P5-M1-002 ECL event codes

Resolves BLOCKING OQs from P5-M1 stories (PR #100) + Phase 5 roadmap (PR #99).

---

## DEC-P5-M1-001 — FVTPL Initial Staging Rule

**Decision**: OPTION A — FVTPL and FVOCI_ELECTION instruments do NOT receive an `ecl.stage_history` row at penempatan approval. No EIR computation is triggered. A single `aud.audit_log` entry with action `PENEMPATAN.STAGING_SKIPPED_FVTPL` MUST be written in-transaction at approve time for audit completeness.

**Rationale**: PSAK 71 §5.5.15 removes FVTPL from ECL scope. Stage history exists solely to determine ECL calculation basis (PD horizon, Gross vs Net Carrying). Writing a stage record for FVTPL creates false audit evidence, corrupts cure counters, and pollutes roll-forward reconciliation. Option B (NULL stage marker) and Option C (Stage 1 + flag) are both non-compliant.

**PSAK 71 §§**: §5.5.1, §5.5.15, §2.1
**FSD References**: FSD-APP-C §1.2, §3.3
**Decision Log Cross-links**: DEC-010, DEC-018
**Phase 4 Consistency**: `docs/state-machines/p4-m7-ecl-core.md` §1 SKIP_FVTPL routing + `docs/state-machines/p4-m1-staging.md` §5.2 SKIP clause (both already LOCKED in Phase 4)

**Implementation Guard Required in P5-M1-S2 approve path**:

```
IF instrumen.klasifikasi_psak71 IN ('AC', 'FVOCI'):
  INSERT ecl.stage_history (stage_sesudah=STAGE_1, trigger_type=INITIAL_PLACEMENT)
  Emit PENEMPATAN.STAGING_INITIAL to aud.audit_log (same tx)
  Trigger EIR initial schedule generation (P4-M5)

IF instrumen.klasifikasi_psak71 IN ('FVTPL', 'FVOCI_ELECTION'):
  Skip ecl.stage_history (no row)
  Emit PENEMPATAN.STAGING_SKIPPED_FVTPL to aud.audit_log (same tx)
  Skip EIR computation trigger entirely
```

**Resolved OQ**: OQ-M1-2d (`docs/stories/phase-5/P5-M1-penempatan-deposito.md` Story S2).

---

## DEC-P5-M1-002 — Master Event Code List (27 codes)

**Decision**: 27 event codes are LOCKED as the complete seed for `mst.mapping_jurnal_header` in migration 000029 (P5-M2). All 27 must have approved mapping detail rows (debit/kredit per klasifikasi) before P5-M2 merge gate.

| # | event_code | kategori_event | trigger_source | klasifikasi_berlaku | PSAK 71 / FSD Reference |
|---|---|---|---|---|---|
| 1 | PENEMPATAN | PENEMPATAN | USER_INPUT | ALL | FSD-APP-B §1; SoW §5.2 |
| 2 | AKRUAL_BUNGA | AKRUAL | SYSTEM_JOB | AC, FVOCI | PSAK 71 §5.4.1; FSD-APP-C §11 |
| 3 | AMORTISASI_PREMI_DISKONTO | AKRUAL | SYSTEM_JOB | AC, FVOCI | PSAK 71 §B5.4.1; FSD-APP-C §9 |
| 4 | EIR_CATCH_UP_ADJUSTMENT (NEW) | AKRUAL | SYSTEM_JOB | AC, FVOCI | PSAK 71 §B5.4.6; FSD-APP-C §10.1 |
| 5 | MTM_FVOCI | MUTASI_MTM | SYSTEM_JOB | FVOCI | PSAK 71 §5.7.1 |
| 6 | MTM_FVTPL | MUTASI_MTM | SYSTEM_JOB | FVTPL | PSAK 71 §5.7.1 |
| 7 | MTM_FVOCI_ELECTION (NEW) | MUTASI_MTM | SYSTEM_JOB | FVOCI_ELECTION | PSAK 71 §5.7.5 no recycling |
| 8 | PEMBAYARAN_BUNGA | PENDAPATAN | USER_INPUT | AC, FVOCI | PSAK 71 §5.4.1 |
| 9 | PEMBAYARAN_KUPON | PENDAPATAN | USER_INPUT | AC, FVOCI | FSD-APP-D §3.5 |
| 10 | PENERIMAAN_DIVIDEN | PENDAPATAN | USER_INPUT | FVTPL, FVOCI_ELECTION | SoW line 960 |
| 11 | DISTRIBUSI_REKSADANA | PENDAPATAN | USER_INPUT | FVOCI, FVTPL | SoW line 961 |
| 12 | ECL_PEMBENTUKAN | ECL | SYSTEM_JOB | AC, FVOCI | PSAK 71 §5.5.8 |
| 13 | ECL_REVERSAL | ECL | SYSTEM_JOB | AC, FVOCI | PSAK 71 §5.5.8 |
| 14 | STAGE_MIGRATION | STAGE_MIGRATION | SYSTEM_JOB | AC, FVOCI | PSAK 71 §5.5.9 |
| 15 | POCI_DELTA_ECL (NEW) | ECL | SYSTEM_JOB | POCI | PSAK 71 §5.5.13-14; DEC-POCI-002 |
| 16 | PENJUALAN_PENCAIRAN | CLOSURE | USER_INPUT | ALL | PSAK 71 §3.2.12; FSD-APP-B §4 |
| 17 | REKLAS_OCI_PL | REKLASIFIKASI | SYSTEM_JOB | FVOCI (debt only) | PSAK 71 §5.7.3 |
| 18 | JATUH_TEMPO | CLOSURE | SYSTEM_JOB | ALL | FSD-APP-B §5 |
| 19 | RENEWAL_DEPOSITO | PENEMPATAN | USER_INPUT | AC (deposito) | FSD-APP-B §3; P5-M7 |
| 20 | MODIFIKASI_MATERIAL (NEW) | CLOSURE | USER_INPUT | AC, FVOCI | PSAK 71 §3.3.2; FSD-APP-C §10.1 |
| 21 | FX_UNREALIZED | FX | SYSTEM_JOB | ALL FCY | PSAK 71 §B5.7.3 |
| 22 | FX_REALIZED | FX | USER_INPUT | ALL FCY | FSD-APP-D §2.5 |
| 23 | PENGHAPUSAN (NEW) | CLOSURE | USER_INPUT | AC (Stage 3) | PSAK 71 §5.4.4 |
| 24 | PERIODE_ADJUSTMENT | PERIODE_ADJUSTMENT | USER_INPUT | ALL | FSD-APP-D §1.4 |
| 25 | CORRECTION_PERIODE_CLOSED | PERIODE_ADJUSTMENT | USER_INPUT | ALL | PSAK 25 |
| 26 | REKLASIFIKASI_AC_FVOCI (NEW) | REKLASIFIKASI | USER_INPUT | TRANSITION | PSAK 71 §4.4.1; FSD-APP-C §12 |
| 27 | REKLASIFIKASI_FVOCI_AC (NEW) | REKLASIFIKASI | USER_INPUT | TRANSITION | PSAK 71 §4.4.1; FSD-APP-C §12 |

Notes:
- (NEW) marks 7 additions beyond SoW §5.1.10 baseline.
- `EIR_REESTIMATION` from SoW line 973 is confirmed NOT a direct journal event — computation trigger only. NOT seeded.
- ALCO approval required for ECL-category codes (12, 13, 14, 15) per DEC-027 before activation in any live calc run.
- Kepala Akuntansi sign-off required for all 27 templates before migration 000029 runs.

**Resolved OQ**: OQ-P5-B (phase-5-roadmap.md §7); supersedes DEC-034 in roadmap §6.

**FSD References**: FSD-APP-D §3.2; SoW_v1.4 §5.1.10
**Decision Log Cross-links**: DEC-010, DEC-017, DEC-018, DEC-034 (superseded)

---

## DEC-P5-M1-003 — Mapping Jurnal CRUD = 6-eyes

**Decision**: Any mapping_jurnal_header CRUD that affects event codes touching ECL, EIR, or PSAK 71 classification (codes 2-7, 12-17, 20, 26-27 from DEC-P5-M1-002) is 6-eyes workflow: ROLE-AKUN (Maker) → ROLE-AKUN-CTL (Reviewer) → ROLE-RISK (Second Approver).

Non-regulated event codes (1, 8-11, 18-19, 21-25) follow 4-eyes baseline (Maker → Reviewer → Approver) per existing Phase 3 mapping_jurnal pattern.

**Rationale**: Mapping jurnal templates are effectively parameters determining P&L and B/S financial impact per regulated event. DEC-017 specifies 6-eyes for parameter master. ECL/EIR/classification jurnal templates are higher-risk than operational templates.

**PSAK 71 §§**: §5.5 disclosure integrity
**Decision Log Cross-links**: DEC-017, DEC-027

**Resolved OQ**: OQ-P5-C (phase-5-roadmap.md §7).

---

## Implementation Checklist

- [ ] Update `docs/stories/phase-5/P5-M1-penempatan-deposito.md` Story S2:
  - Mark OQ-M1-2d RESOLVED (DEC-P5-M1-001)
  - Add Gherkin scenario "Instrumen FVTPL diapprove — tidak ada ecl.stage_history INSERT, audit log PENEMPATAN.STAGING_SKIPPED_FVTPL ditulis"
  - Add Gherkin scenario "Instrumen FVOCI_ELECTION diapprove — tidak ada ecl.stage_history INSERT, audit log STAGING_SKIPPED_FVTPL ditulis"
- [ ] Update `docs/plans/phase-5-roadmap.md` §7:
  - Mark OQ-P5-B RESOLVED (DEC-P5-M1-002)
  - Mark OQ-P5-C RESOLVED (DEC-P5-M1-003)
- [ ] Update `docs/plans/phase-5-roadmap.md` §6 DEC-034 marked as SUPERSEDED by DEC-P5-M1-002
- [ ] P5-M2 migration 000029 must seed all 27 event codes per DEC-P5-M1-002 table
- [ ] system-analyst before opening P5-M2 OpenAPI fragment: mapping_jurnal_header endpoints must distinguish 6-eyes vs 4-eyes per DEC-P5-M1-003
- [ ] ALCO + Kepala Akuntansi sign-off before migration 000029 deploy

---

## References

- Compliance review (PR #99/#100 follow-up, 2026-06-14)
- PSAK 71 §5.5.1, §5.5.15, §2.1 (FVTPL scope exclusion)
- PSAK 71 §3.2.12, §3.3.2, §4.4.1, §5.4.1, §5.4.4, §5.5.8, §5.5.9, §5.5.13-14, §5.7.1, §5.7.3, §5.7.5, §B5.4.1, §B5.4.6, §B5.7.3
- PSAK 25 (correction periode closed)
- BRD §6.2 RACI
- SoW_v1.4 §5.1.10, §5.2, §5.10, lines 854-857, 959-974
- FSD-APP-B §1, §2, §3, §4, §5
- FSD-APP-C §1.2, §3, §6, §7.4, §9, §10.1, §11, §12
- FSD-APP-D §1.4, §2.5, §3.2, §3.5
- Decision Log: DEC-010, DEC-017, DEC-018, DEC-027, DEC-034 (superseded), DEC-POCI-002
- Phase 4 state machines: `docs/state-machines/p4-m1-staging.md`, `p4-m7-ecl-core.md`
- UAT: `docs/uat/phase-4/UAT-APP-C-003-eir-amendment.md`
