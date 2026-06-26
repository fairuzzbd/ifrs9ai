# UAT-FULL-003 — POCI End-to-End: Origination → Delta ECL → Jurnal P&L → Roll-Forward

**Modul**: Cross-Modul (APP-B + APP-C + APP-D)
**Story**: Instrument POCI dari origination sampai verifikasi delta ECL ke P&L dan CKPN roll-forward
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-003 |
| Referensi test | `TestP5M18_POCI_Originated_Credit_Impaired` |
| Referensi FSD | FSD-APP-C §5 (POCI), FSD-APP-D §4 (jurnal P&L) |
| PSAK 71 | §5.5.13–14 (POCI no Stage 1, delta to P&L) |
| DEC terkait | DEC-POCI-002, DEC-010, DEC-013, DEC-016 |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | Originate POCI instrument |
| Treasury Approver | ROLE-APPR-TR | Approve + baseline capture |
| Risk Officer | ROLE-RISK | ECL delta computation |
| Akuntansi | ROLE-AKUN | Verifikasi jurnal P&L |
| Finance Controller | ROLE-AKUN-CTL | Approve posting jurnal |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Instrumen dengan flag `is_poci = true` sudah ada di master data |
| P2 | Mapping jurnal POCI_ECL_DELTA_INCREASE dan POCI_ECL_DELTA_DECREASE APPROVED |
| P3 | Periode OPEN |
| P4 | Sys.parameter `large_delta_threshold_idr = 500000000` tersedia |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Instrumen | POCI-OBL-001 (obligasi POCI, credit-impaired sejak awal) |
| Klasifikasi | POCI |
| Nominal | IDR 5.000.000.000 |
| PD lifetime saat origination | 35% |
| Kupon gross | 6,50% p.a. |
| Credit-adjusted EIR (estimasi) | ≈ 4,75% p.a. (setelah PD adjustment) |
| Baseline ECL at origination | IDR 1.500.000.000 |
| Current lifetime ECL (setelah 3 bulan) | IDR 1.800.000.000 |
| Delta ECL | IDR 300.000.000 (INCREASE) |

---

## 5. Langkah-Langkah

### Fase 1: Origination + Baseline Capture

**Step 1.1 — Approve penempatan POCI (Treasury Approver)**

1. Proses penempatan POCI-OBL-001 melalui 4-eyes workflow.
2. Pada approve → sistem harus otomatis capture ECL baseline.

Hasil yang diharapkan:
- [ ] Pada approve: `ecl.poci_delta_log` baris baseline INSERT (bukan UPDATE).
- [ ] `stage_marker = "POCI"` (bukan STAGE_1).
- [ ] Credit-adjusted EIR terhitung via Newton-Raphson.
- [ ] Audit: `POCI.BASELINE_CAPTURED` dengan `baseline_ecl_idr: "1500000000.0000"`.
- [ ] **KRITIS**: Instrumen POCI tidak pernah mendapat `STAGE_1_ASSIGNED` — `staging_action` harus `"POCI_SKIP_STAGE1"`.

**Step 1.2 — Verifikasi baseline immutability**

1. Coba PATCH `/api/v1/poci/baseline/{id}` (simulasi tampering).

Hasil yang diharapkan:
- [ ] Response: `403 { code: "POCI_BASELINE_IMMUTABLE_VIOLATION" }`.
- [ ] Audit: `POCI.BASELINE_VIOLATION_ATTEMPT`.

### Fase 2: ECL Delta Computation

**Step 2.1 — Jalankan ECL calc run setelah 3 bulan**

1. Jalankan calc run untuk instrumen POCI-OBL-001.

Hasil yang diharapkan:
- [ ] Current lifetime ECL dihitung: IDR 1.800.000.000.
- [ ] Delta = current − baseline = IDR 300.000.000 (INCREASE).
- [ ] `delta_direction = "INCREASE"`.
- [ ] Audit: `POCI.DELTA_COMPUTED`.

### Fase 3: Jurnal P&L Booking

**Step 3.1 — Posting jurnal delta P&L**

1. Sistem auto-post jurnal POCI_ECL_DELTA_INCREASE.

Hasil yang diharapkan:
- [ ] Jurnal POCI_ECL_DELTA_INCREASE ter-post:
  - D Beban Penurunan Nilai (6001): IDR 300.000.000
  - K Cadangan ECL (1901): IDR 300.000.000
- [ ] Jurnal balanced.
- [ ] **KRITIS**: Tidak ada jurnal ke OCI. Delta POCI masuk langsung ke P&L (PSAK 71 §5.5.14).
- [ ] Audit: `POCI.DELTA_POSTED`.
- [ ] GL status: `DELIVERED`.

**Step 3.2 — Skenario delta DECREASE**

1. Simulasikan perbaikan: current ECL turun ke IDR 1.350.000.000.
2. Delta = -150.000.000 (DECREASE).

Hasil yang diharapkan:
- [ ] Jurnal POCI_ECL_DELTA_DECREASE:
  - D Cadangan ECL (1901): IDR 150.000.000
  - K Pendapatan Pemulihan (4001): IDR 150.000.000
- [ ] Balanced.
- [ ] `delta_direction = "DECREASE"`.

### Fase 4: CKPN Roll-Forward Reconcile

**Step 4.1 — Verifikasi roll-forward**

1. Buka `/reports/RPT-05-ckpn-rollforward?periode=PBUKU-2026-06`.

Hasil yang diharapkan:
- [ ] Opening CKPN + Δ = Closing CKPN.
- [ ] POCI delta line item tampil terpisah dari ECL Stage 1/2/3 movement.
- [ ] Formula: `Closing = Opening + INCREASE − DECREASE`.
- [ ] Nilai numerik reconcile hingga 4 desimal (NUMERIC(20,4)).
- [ ] Export CSV tersedia dan valid.

---

## 6. Audit Checks

| Aksi | Audit Action |
|---|---|
| Approve penempatan | `POCI.BASELINE_CAPTURED` |
| Tamper attempt | `POCI.BASELINE_VIOLATION_ATTEMPT` |
| Delta computed | `POCI.DELTA_COMPUTED` |
| Delta posted P&L | `POCI.DELTA_POSTED` |

---

## 7. Rollback / Cleanup

1. Baseline `ecl.poci_delta_log` tidak dapat di-delete (DEC-018).
2. Jika posting salah → jurnal koreksi manual.
3. Soft-delete instrumen hanya dari ROLE-IT-ADMIN dengan approval chain.

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| ifrs9-compliance-reviewer | | | APPROVED / REJECT | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
