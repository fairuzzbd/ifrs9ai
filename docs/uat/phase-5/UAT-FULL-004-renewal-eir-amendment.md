# UAT-FULL-004 — Renewal Deposito: EIR Amendment + Schedule Versioning

**Modul**: APP-B (Renewal) + APP-C (EIR)
**Story**: Renewal deposito menghasilkan EIR_v2 via Newton-Raphson, schedule versi baru INSERT (bukan UPDATE)
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-004 |
| Referensi test | `TestP5M18_Renewal_EIR_RoundTrip` |
| Referensi FSD | FSD-APP-B §3 (Renewal), FSD-APP-C §4 (EIR) |
| PSAK 71 | §5.4.2 (EIR method), §B5.4.5 (amendment re-estimation) |
| DEC terkait | DEC-013 (Newton-Raphson), DEC-018 (audit immutability) |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | Buat renewal |
| Treasury Approver | ROLE-APPR-TR | Approve renewal (EIR trigger) |
| Risk Officer | ROLE-RISK | Verifikasi EIR schedule versioning |
| Internal Auditor | ROLE-AUDIT | Verifikasi immutability v1 |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Deposito DEP-BCA-001 status APPROVED_ACTIVE, tenor asli 12 bulan, rate 5,25%, EIR_v1 sudah terhitung |
| P2 | Periode OPEN |
| P3 | `eir_tolerance = 1e-10`, `eir_max_iter = 100` di sys.parameter |
| P4 | Mapping jurnal RENEWAL_DEPOSITO APPROVED |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Instrumen asal | DEP-BCA-001 |
| Nominal | IDR 10.000.000.000 |
| EIR_v1 (asli) | ≈ 4,20% p.a. (setelah PPh 20%) |
| Tanggal amendment | 2026-09-01 |
| Skema renewal | POKOK_PLUS_BUNGA |
| Tenor baru | 6 bulan |
| Rate baru | 5,50% gross p.a. |
| EIR_v2 (diharapkan) | > EIR_v1 (rate naik) |
| PPh | 20% final |
| Bunga bersih | ≥ IDR 100.000 (validasi minimum) |

---

## 5. Langkah-Langkah

### Fase 1: EIR_v1 Verification (Pre-Renewal)

**Step 1.1 — Verifikasi EIR_v1 di schedule**

1. `GET /api/v1/ecl/amortisasi-schedule?instrumen_id=DEP-BCA-001&active=true`.

Hasil yang diharapkan:
- [ ] Satu baris aktif: `schedule_version = 1`, `effective_to = NULL` (infinity).
- [ ] `eir_persen` memiliki presisi 8 desimal (DEC-016).
- [ ] Konvergensi Newton-Raphson: `iterations ≤ 100`, `tolerance = 1e-10`.

### Fase 2: Buat Renewal

**Step 2.1 — Create renewal (Treasury Maker)**

1. Buka `/transaksi/renewal/new`.
2. Pilih DEP-BCA-001, skema POKOK_PLUS_BUNGA, tenor 6 bulan, rate 5,50%.
3. Preview: sistem tampilkan `bunga_bersih_idr`, `pph_idr`, `eir_preview`.

Hasil yang diharapkan:
- [ ] Preview EIR_v2 tampil (estimasi saat create, final saat approve).
- [ ] `bunga_bersih ≥ IDR 100.000` (validasi server).
- [ ] Audit: `RENEWAL.CREATED`.

**Step 2.2 — Approve renewal (Treasury Approver + MFA step-up)**

1. Login reviewer, review renewal.
2. Login approver (berbeda dari maker dan reviewer), approve.
3. MFA step-up prompt → masukkan OTP.

Hasil yang diharapkan saat approve:
- [ ] Sistem INSERT baris baru di `ecl.amortisasi_schedule`:
  - `schedule_version = 2`
  - `effective_from = 2026-09-01`
  - `effective_to = NULL` (infinity)
  - `eir_persen` dihitung ulang via Newton-Raphson
- [ ] Baris v1 di-UPDATE **hanya** field `effective_to = 2026-09-01`. Tidak ada field lain yang berubah.
- [ ] EIR_v2 > EIR_v1 (rate naik dari 5,25% → 5,50%).
- [ ] Instrumen lama (DEP-BCA-001) status → MATURED.
- [ ] Instrumen baru (DEP-BCA-002) dibuat otomatis dengan data yang diwariskan.
- [ ] Jurnal RENEWAL_DEPOSITO ter-post (4 leg). Balanced.
- [ ] Audit: `RENEWAL.APPROVED`, `EIR.RECOMPUTED`, `INSTRUMEN.CREATED`, `INSTRUMEN.MATURED`.

### Fase 3: Verifikasi EIR Schedule Versioning

**Step 3.1 — Verifikasi chain v1 → v2**

1. `GET /api/v1/ecl/amortisasi-schedule?instrumen_id=DEP-BCA-001&include_inactive=true`.

Hasil yang diharapkan:
- [ ] Dua baris: `schedule_version = 1` dan `schedule_version = 2`.
- [ ] v1: `effective_from = 2026-06-01`, `effective_to = 2026-09-01`.
- [ ] v2: `effective_from = 2026-09-01`, `effective_to = NULL`.
- [ ] **v1.effective_to == v2.effective_from** (chain tanpa gap, tanpa overlap).
- [ ] v1 EIR tidak berubah (immutable DEC-018).
- [ ] v2 EIR memiliki presisi 8 desimal.

**Step 3.2 — Verifikasi immutability v1 (Auditor)**

1. Login sebagai `usr-audit-01`.
2. `GET /api/v1/audit/log?entity_type=EIR_SCHEDULE&entity_id=DEP-BCA-001-v1`.

Hasil yang diharapkan:
- [ ] Audit hanya menunjukkan `EIR.SCHEDULE_VERSION_INSERTED` (2x: v1 + v2).
- [ ] Tidak ada `EIR.SCHEDULE_UPDATED` (tidak ada UPDATE, hanya INSERT).
- [ ] Auditor TIDAK dapat PATCH schedule: `403 FORBIDDEN`.

### Fase 4: Verifikasi Newton-Raphson Precision

**Step 4.1 — Check EIR computation log**

1. `GET /api/v1/ecl/eir-compute-log?instrumen_id=DEP-BCA-001`.

Hasil yang diharapkan:
- [ ] `converged: true`.
- [ ] `iterations ≤ 100` (DEC-013).
- [ ] `final_residual < 1e-10` (tolerance).
- [ ] Cashflow array menggunakan kupon after-PPh (× 0.80), bukan gross.

---

## 6. Audit Checks

| Aksi | Audit Action |
|---|---|
| Create renewal | `RENEWAL.CREATED` |
| Approve renewal | `RENEWAL.APPROVED` + `RENEWAL.POSTED` |
| EIR v1 insert (awal) | `EIR.SCHEDULE_VERSION_INSERTED` (version=1) |
| EIR v2 insert (amendment) | `EIR.SCHEDULE_VERSION_INSERTED` (version=2) |
| Instrumen baru | `INSTRUMEN.CREATED` |
| Instrumen lama | `INSTRUMEN.MATURED` |

Tidak boleh ada `EIR.SCHEDULE_UPDATED` di audit log.

---

## 7. Rollback / Cleanup

1. EIR schedule rows tidak dapat di-delete (DEC-018).
2. Jika renewal salah: reject sebelum approve. Setelah POSTED tidak bisa rollback.
3. Instrumen baru yang auto-created: soft-delete hanya via approval chain.

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| ifrs9-compliance-reviewer | | | APPROVED / REJECT | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
