# P4-M3 — LPS Aggregator: User Stories

**Story Set ID**: P4-M3
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 1)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-11
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3 (LPS Aggregator), §4 (EAD pre-processing)
**Linked BRD**: BRD §8.3 (ECL Requirements — LPS penjaminan), RACI: ROLE-RISK (R), ROLE-ALCO (A/approval override), ROLE-AKUN-CTL (C), ROLE-AUDIT (I)
**Linked Decision Log**:
- DEC-014 — LPS cap IDR 2 miliar per nasabah per bank, applied SEBELUM ECL
- DEC-010 — ECL hanya atas excess portion; covered portion ECL = 0
- DEC-016 — NUMERIC precision: IDR `NUMERIC(20,4)`, rate `NUMERIC(10,8)`
- DEC-017 — Workflow 4-eyes untuk operasional (LPS exclusion override)
- DEC-018 — Audit trail append-only

**Depends on**: P4-M1 (staging engine), P4-M2 (PD/LGD/EAD helpers), Phase 3 migration 0012 (`mst.lps_coverage`)

**Handoff berikutnya**: `system-analyst` (Go interface contract `LPSAggregatorService` + endpoint contract), lalu `data-modeler` (migration 000023: tabel `ecl.lps_exclusion_override` baru — lihat Story 4), lalu `ecl-eir-engineer` (implementasi domain logic `backend/internal/ecl/lps/`)

---

## Konteks & Dependensi

### Skema yang dikonsumsi

| Tabel | Kolom Kunci | Catatan |
|---|---|---|
| `mst.lps_coverage` | `coverage_amount NUMERIC(20,4)`, `mata_uang = 'IDR'`, `periode_berlaku_dari`, `periode_berlaku_sampai`, `workflow_status = 'APPROVED'` | Migration 0012. Satu record aktif per waktu (WHERE `periode_berlaku_sampai IS NULL` OR evaluationDate BETWEEN `periode_berlaku_dari` AND `periode_berlaku_sampai`). |
| `mst.instrumen` | `id`, `tipe_instrumen IN ('DEPOSITO')`, `counterparty_id`, `nominal NUMERIC(20,2)`, `mata_uang`, `status = 'AKTIF'`, `is_deleted = FALSE` | Init schema 0001 + migration 0019. Catatan: tipe `'CASH'` tidak ada di CHECK constraint saat ini — lihat GAP-CASH di bawah. |
| `mst.counterparty` | `id`, `tipe IN ('BANK')`, `eligible_lps_flag BOOLEAN`, `nama`, `kode_counterparty` | `eligible_lps_flag = TRUE` menandai bank yang terdaftar di LPS. |
| `ecl.calc_header` | `instrumen_id`, `ead_idr`, `lps_covered_idr`, `lps_excess_idr` (kolom perlu schema-fix — lihat DDL Gap) | Hasil per instrumen per calc run. |
| `ecl.lps_exclusion_override` | **TABEL BARU** — lihat Story 4 | Workflow 4-eyes exclusion per instrumen. |
| `mst.kurs` | `kode_mata_uang`, `nilai_kurs NUMERIC(20,8)`, `sumber_kurs = 'BI_JISDOR'`, `tanggal_berlaku` | Untuk konversi EAD FCY → IDR (migration 0020). |

### GAP-CASH: Tipe Instrumen 'CASH' Belum Ada

**Severity: MEDIUM**

`mst.instrumen.tipe_instrumen` CHECK constraint (migration 0019) hanya memperbolehkan:
`'DEPOSITO', 'OBLIGASI', 'SAHAM', 'REKSADANA', 'SBN', 'SPN', 'SUKUK'`

Tidak ada tipe `'CASH'` atau `'GIRO'`. LPS Aggregator per formulanya mencakup "Cash + Deposito".

**Kemungkinan penafsiran**:
1. Cash di BLIPS direpresentasikan sebagai instrumen DEPOSITO dengan sub-tipe = 'GIRO' atau sub-tipe = 'REKENING_KORAN'
2. Cash dimasukkan sebagai tipe terpisah dan perlu tambah ke CHECK constraint

**Flag ke `system-analyst` dan `data-modeler`**: Konfirmasi representasi Cash di `mst.instrumen` sebelum implementasi filter di Story 1. Sampai resolved, LPS Aggregator di-scope pada `tipe_instrumen = 'DEPOSITO'` saja (yang paling material). Lihat OQ-M3-1.

### DDL Gap: Kolom LPS di ecl.calc_header

`ecl.calc_header` (init schema 0001) tidak memiliki kolom:
- `lps_covered_idr NUMERIC(20,4)` — porsi yang dijamin LPS
- `lps_excess_idr NUMERIC(20,4)` — porsi excess untuk ECL
- `lps_covered_flag BOOLEAN` — apakah instrumen ini full-covered

Kolom ini diperlukan agar Story 2 dapat menyimpan audit breakdown per instrumen.
**Flag ke `data-modeler`**: tambahkan di migration 000023 (bersama `ecl.lps_exclusion_override`).

---

## Open Questions (M3-spesifik)

| ID | Pertanyaan | Asumsi Default | Perlu Konfirmasi |
|---|---|---|---|
| OQ-M3-1 | Apakah Cash (rekening giro, rekening koran, call money) masuk scope LPS Aggregator, dan bagaimana tipe_instrumen-nya di BLIPS? | Hanya `DEPOSITO` untuk Phase 4; Cash dimasukkan Phase 5 jika APP-B memperkenalkan tipe baru | `system-analyst` + FSD-APP-C + data-modeler |
| OQ-M3-2 | Apakah nasabah (counterparty) yang bukan Bank (mis. KORPORASI) bisa menjadi "bank" dalam konteks LPS? (i.e., apakah LPS hanya berlaku untuk `counterparty.tipe = 'BANK'`?) | Ya, hanya berlaku jika `counterparty.eligible_lps_flag = TRUE` AND `counterparty.tipe = 'BANK'` | FSD-APP-C §3 + ifrs9-compliance-reviewer |
| OQ-M3-3 | Jika instrumen multi-currency (mis. deposito USD di bank lokal): apakah cap IDR 2M diapply sebelum atau setelah konversi ke IDR? | Setelah konversi ke IDR (kurs BI JISDOR tanggal evaluasi) — sama dengan EAD calculation M2 | ifrs9-compliance-reviewer + DEC-014 wording |
| OQ-M3-4 | Apakah instrumen dengan `klasifikasi_psak71 = 'FVTPL'` masuk scope LPS Aggregator? (FVTPL skip ECL, tapi LPS coverage masih relevan untuk exposure size) | Skip untuk LPS aggregation juga — hanya AC + FVOCI_DEBT yang masuk ECL pipeline termasuk LPS. Konsisten dengan OQ-G di plan. | ifrs9-compliance-reviewer |
| OQ-M3-5 | Override exclusion Story 4 — apakah juga perlu step-up MFA untuk ALCO saat approve? Atau cukup MFA wajib standar ALCO? | Tidak perlu step-up MFA khusus (step-up reserved untuk seal ECL run dan hard-close periode per DEC-027). MFA wajib ALCO sudah meng-cover ini. | security-engineer + DEC-027 check |
| OQ-M3-6 | Scope nasabah: "nasabah" dalam konteks LPS = `counterparty_id` di `mst.instrumen`. Apakah satu counterparty bisa menjadi nasabah di banyak bank sekaligus? | Ya — looping per (counterparty_id, bank_counterparty_id) pasangan. | konfirmasi by design |

---

## Story APP-C-LPS-001 — Aggregate Exposure per (Nasabah, Bank)

**Actor**: ECL calc engine (Asynq worker — `internal/ecl/lps`)
**Trigger**: Dipanggil oleh P4-M7 ECL engine sebelum ECL calculation per instrumen tipe DEPOSITO. Juga dipanggil langsung via internal service method untuk Story 5 bulk preview.
**Goal**: Menghitung total exposure, covered amount, dan excess EAD per pasangan (nasabah/counterparty, bank/counterparty) berdasarkan cap aktif dari `mst.lps_coverage`; mengembalikan breakdown per instrumen untuk audit trail.

**Pre-conditions**:
- `mst.lps_coverage` memiliki minimal satu record dengan `workflow_status = 'APPROVED'` yang berlaku pada `evaluationDate`
- Instrumen dalam scope: `tipe_instrumen = 'DEPOSITO'`, `status = 'AKTIF'`, `is_deleted = FALSE`, `workflow_status = 'APPROVED'`
- `mst.counterparty.eligible_lps_flag = TRUE` untuk bank yang relevan
- Kurs BI JISDOR tersedia di `mst.kurs` untuk `evaluationDate` (jika ada instrumen non-IDR)

**Permissions**: `lps_aggregator.compute` (internal engine — tidak di-expose ke UI)

**Audit events**: Tidak menulis `aud.audit_log` secara langsung (read-only aggregation); audit ditulis oleh P4-M7/M8 pada saat calc run disimpan.

---

### AC Gherkin — APP-C-LPS-001

**Scenario 1 (Happy Path — Nasabah dengan exposure > cap di satu bank)**

```gherkin
GIVEN nasabah "Asuransi Tugu" (counterparty_id = "CP-001")
  AND bank "Bank BCA" (counterparty_id = "CP-BANK-001", eligible_lps_flag = TRUE)
  AND instrumen aktif:
    - DEPOSITO-001: nominal IDR 1.500.000.000 (di CP-BANK-001)
    - DEPOSITO-002: nominal IDR 1.000.000.000 (di CP-BANK-001)
  AND mst.lps_coverage active record: coverage_amount = IDR 2.000.000.000
  AND evaluationDate = 2026-06-30
WHEN LPS Aggregator.Aggregate(evaluationDate, [DEPOSITO-001, DEPOSITO-002]) dipanggil
THEN result untuk pasangan (CP-001, CP-BANK-001) adalah:
    total_exposure   = IDR 2.500.000.000
    covered          = IDR 2.000.000.000
    excess           = IDR 500.000.000
  AND instrumen_breakdown berisi:
    - { instrumen_id: DEPOSITO-001, ead_instrumen: 1.500.000.000, covered_porsi: 1.500.000.000, excess_porsi: 0 }
    - { instrumen_id: DEPOSITO-002, ead_instrumen: 1.000.000.000, covered_porsi: 500.000.000, excess_porsi: 500.000.000 }
  AND alokasi covered dilakukan secara proporsional berdasarkan urutan instrumen_id (atau tanggal_penempatan DESC sebagai tiebreak)
```

**Scenario 2 (Happy Path — Nasabah dengan exposure ≤ cap: full covered)**

```gherkin
GIVEN nasabah "Asuransi Tugu" (counterparty_id = "CP-001")
  AND bank "Bank Mandiri" (counterparty_id = "CP-BANK-002", eligible_lps_flag = TRUE)
  AND instrumen aktif:
    - DEPOSITO-003: nominal IDR 800.000.000 (di CP-BANK-002)
  AND mst.lps_coverage active: coverage_amount = IDR 2.000.000.000
  AND evaluationDate = 2026-06-30
WHEN LPS Aggregator.Aggregate(evaluationDate, [DEPOSITO-003]) dipanggil
THEN result untuk pasangan (CP-001, CP-BANK-002):
    total_exposure = IDR 800.000.000
    covered        = IDR 800.000.000
    excess         = IDR 0
  AND instrumen_breakdown:
    - { instrumen_id: DEPOSITO-003, ead_instrumen: 800.000.000, covered_porsi: 800.000.000, excess_porsi: 0, lps_full_covered: true }
```

**Scenario 3 (Edge — nasabah sama, dua bank berbeda: cap berlaku per bank)**

```gherkin
GIVEN nasabah "Asuransi Tugu" (counterparty_id = "CP-001")
  AND bank A (CP-BANK-001, eligible_lps_flag = TRUE): DEPOSITO-001 nominal IDR 2.500.000.000
  AND bank B (CP-BANK-002, eligible_lps_flag = TRUE): DEPOSITO-004 nominal IDR 1.800.000.000
  AND coverage_amount = IDR 2.000.000.000
WHEN LPS Aggregator.Aggregate(evaluationDate, [DEPOSITO-001, DEPOSITO-004]) dipanggil
THEN untuk pasangan (CP-001, CP-BANK-001):
    total_exposure = 2.500.000.000, covered = 2.000.000.000, excess = 500.000.000
  AND untuk pasangan (CP-001, CP-BANK-002):
    total_exposure = 1.800.000.000, covered = 1.800.000.000, excess = 0
  AND cap TIDAK di-share antar bank — masing-masing pasangan dihitung independen
```

**Scenario 4 (Edge — bank yang sama sebagai issuer instrument DAN pemegang saldo nasabah lain)**

```gherkin
GIVEN instrumen DEPOSITO-005: counterparty_id = "CP-BANK-003" (BNI, tipe BANK, eligible_lps_flag = TRUE)
  AND instrumen ini merepresentasikan penempatan deposito Tugure DI Bank BNI
  AND tidak ada instrumen lain ke CP-BANK-003 dari Tugure
  AND coverage_amount = IDR 2.000.000.000
WHEN LPS Aggregator.Aggregate(evaluationDate, [DEPOSITO-005]) dipanggil
THEN pasangan dievaluasi sebagai (Tugure / CP-001, Bank-BNI / CP-BANK-003)
  AND aggregasi berjalan normal — bank sebagai counterparty issuer sekaligus penerima deposit tidak menyebabkan exclusion
```

**Scenario 5 (Error — tidak ada lps_coverage aktif)**

```gherkin
GIVEN evaluationDate = 2026-06-30
  AND mst.lps_coverage tidak memiliki record dengan workflow_status = 'APPROVED'
      yang berlaku pada evaluationDate
WHEN LPS Aggregator.Aggregate(evaluationDate, [...]) dipanggil
THEN error dikembalikan:
    code: "LPS_COVERAGE_NO_ACTIVE_PARAM"
    message: "Tidak ditemukan LPS coverage parameter yang APPROVED untuk tanggal 2026-06-30"
  AND calc run untuk periode ini GAGAL dengan reason "LPS_COVERAGE_NO_ACTIVE_PARAM"
  AND sys.job.status = 'failed', sys.job.error_jsonb mencantumkan error code + evaluationDate
```

**Scenario 6 (Edge — instrumen FCY, konversi ke IDR dulu)**

```gherkin
GIVEN DEPOSITO-006: nominal USD 100.000, counterparty_id = CP-BANK-001
  AND mst.kurs: USD/IDR = 15.800,00000000 (BI JISDOR, tanggal evaluationDate)
  AND coverage_amount = IDR 2.000.000.000
WHEN LPS Aggregator.Aggregate(evaluationDate, [DEPOSITO-006]) dipanggil
THEN ead_instrumen_idr = 100.000 × 15.800 = IDR 1.580.000.000,0000
  AND covered = IDR 1.580.000.000,0000 (karena < cap)
  AND excess  = IDR 0
  AND konversi menggunakan kurs JISDOR tanggal evaluationDate, bukan tanggal penempatan
```

**Scenario 7 (Edge — instrumen di-exclude via override aktif — lihat Story 4)**

```gherkin
GIVEN DEPOSITO-007 memiliki entry aktif di ecl.lps_exclusion_override
    dengan workflow_status = 'APPROVED' dan effective_from ≤ evaluationDate
WHEN LPS Aggregator.Aggregate(evaluationDate, [DEPOSITO-007, DEPOSITO-001]) dipanggil
THEN DEPOSITO-007 di-skip dari pool LPS untuk pasangan nasabah-bank-nya
  AND DEPOSITO-007 di-ECL-kan penuh (EAD penuh, tanpa LPS coverage benefit)
  AND instrumen_breakdown mencantumkan: { instrumen_id: DEPOSITO-007, lps_excluded: true, exclusion_reason: "..." }
```

---

## Story APP-C-LPS-002 — Compute ECL on Excess Only

**Actor**: ECL calc engine (Asynq worker — `internal/ecl/engine`, via P4-M7)
**Trigger**: Dipanggil oleh M7 ECL engine setelah Story 1 menghasilkan breakdown per instrumen. M2 PD/LGD helpers sudah tersedia.
**Goal**: Menghasilkan ECL **hanya atas excess portion** per instrumen. Covered portion → ECL = 0 (ditetapkan secara eksplisit, bukan default implisit). Hasil tersimpan di `ecl.calc_header` dengan flag traceability LPS.

**Pre-conditions**:
- Story 1 `Aggregate()` sudah dipanggil dan menghasilkan `instrumen_breakdown` yang valid
- P4-M2 PD lookup, LGD lookup tersedia (sudah approved untuk periodeID)
- `ecl.calc_header` memiliki kolom `lps_covered_idr`, `lps_excess_idr`, `lps_covered_flag` (perlu migration 000023)

**Permissions**: `lps_aggregator.compute` (internal engine)

**Audit events**: ECL result ditulis ke `ecl.calc_header` dan `ecl.calc_detail_skenario` oleh M7/M8 engine. LPS breakdown tersimpan di `ecl.calc_header.lps_covered_idr`, `ecl.calc_header.lps_excess_idr`.

---

### AC Gherkin — APP-C-LPS-002

**Scenario 1 (Happy Path — Partial covered: ECL hanya atas excess)**

```gherkin
GIVEN instrumen DEPOSITO-002: ead_instrumen = IDR 1.000.000.000
  AND hasil Story 1: covered_porsi = 500.000.000, excess_porsi = 500.000.000
  AND PD_12M (Stage 1) = 0,01500000 (1,5%), LGD = 0,45000000 (45%)
  AND bobot: GOOD=0.25, NORMAL=0.50, BAD=0.25; FL multiplier per OQ-A default
WHEN ECL engine compute untuk DEPOSITO-002
THEN EAD_for_ECL = excess_porsi = IDR 500.000.000 (bukan 1.000.000.000)
  AND ECL_skenario_NORMAL = 500.000.000 × 0,0150 × 0,45 = IDR 3.375.000,0000
  AND ECL_weighted = Σ(ECL_FL_skenario × bobot)
  AND ecl.calc_header:
    - lps_covered_idr = 500.000.000,0000
    - lps_excess_idr  = 500.000.000,0000
    - lps_covered_flag = false  (hanya partial)
  AND ecl.calc_detail_skenario: 3 rows (GOOD/NORMAL/BAD) masing-masing dengan ead_skenario = 500.000.000
```

**Scenario 2 (Happy Path — Full covered: ECL = 0 secara eksplisit)**

```gherkin
GIVEN instrumen DEPOSITO-003: ead_instrumen = IDR 800.000.000
  AND hasil Story 1: covered_porsi = 800.000.000, excess_porsi = 0
WHEN ECL engine compute untuk DEPOSITO-003
THEN EAD_for_ECL = 0 (excess = 0)
  AND ECL_weighted = IDR 0,0000 (dihitung, bukan di-skip)
  AND ecl.calc_header:
    - lps_covered_idr = 800.000.000,0000
    - lps_excess_idr  = 0,0000
    - lps_covered_flag = true
  AND ecl.calc_detail_skenario: 3 rows dengan ecl_skenario_idr = 0,0000 untuk semua skenario
  AND TIDAK ada row yang di-skip atau di-null — eksplisit nol tercatat untuk audit
```

**Scenario 3 (Edge — instrumen di-exclude via override: ECL atas full EAD)**

```gherkin
GIVEN instrumen DEPOSITO-007: ead_instrumen = IDR 1.200.000.000
  AND Story 1 menandai DEPOSITO-007 sebagai lps_excluded = true (override aktif)
WHEN ECL engine compute untuk DEPOSITO-007
THEN EAD_for_ECL = IDR 1.200.000.000 (penuh, tanpa LPS coverage)
  AND ecl.calc_header:
    - lps_covered_idr = 0,0000
    - lps_excess_idr  = 1.200.000.000,0000
    - lps_covered_flag = false
  AND ecl.calc_header.catatan mencantumkan "LPS_EXCLUDED via override [override_id]"
```

**Scenario 4 (Audit integrity — field lps_covered_flag wajib ada)**

```gherkin
GIVEN ECL calc run selesai untuk periode Juni 2026
  AND ada 50 instrumen DEPOSITO dalam scope
WHEN calc run di-seal (P4-M8)
THEN SETIAP row di ecl.calc_header untuk instrumen DEPOSITO
    memiliki lps_covered_idr IS NOT NULL
    AND lps_excess_idr IS NOT NULL
    AND lps_covered_flag IS NOT NULL
  AND tidak ada row DEPOSITO dengan lps_covered_idr + lps_excess_idr ≠ ead_idr (mismatch ditolak)
  AND ifrs9-compliance-reviewer dapat memverifikasi: total ECL DEPOSITO = ECL(excess_only), bukan ECL(full_EAD)
```

---

## Story APP-C-LPS-003 — Preview LPS Coverage Utilization

**Actor**: ROLE-RISK
**Trigger**: Navigasi ke halaman `/ecl/lps/preview` di UI; atau filter per periode/bank dari halaman ECL Run Detail
**Goal**: Melihat ringkasan LPS coverage utilization — per pasangan (nasabah, bank) — sebelum atau setelah calc run. Mendukung analisis "bank mana yang paling exposed" dan "nasabah mana yang excess".

**Pre-conditions**:
- User login dengan role ROLE-RISK, token valid
- `mst.lps_coverage` memiliki record APPROVED
- Minimal ada instrumen DEPOSITO AKTIF di mst.instrumen

**Permissions**: `lps_aggregator.preview` (read-only)

**Audit events**: `LPS_AGGREGATOR.PREVIEW` ditulis ke `aud.audit_log` setiap kali endpoint dipanggil (untuk export khususnya: `LPS_AGGREGATOR.EXPORT`)

---

### AC Gherkin — APP-C-LPS-003

**Scenario 1 (Happy Path — Load preview tabel)**

```gherkin
GIVEN ROLE-RISK user Budi login dengan mfa_verified = false (MFA tidak wajib untuk RISK)
  AND permission lps_aggregator.preview ada di JWT claims
  AND evaluationDate = 2026-06-30
WHEN Budi navigasi ke GET /api/v1/ecl/lps/preview?evaluation_date=2026-06-30&limit=50
THEN response 200 dengan DataTable:
    - Kolom: nasabah_nama, bank_nama, total_exposure_idr, cap_idr, covered_idr, excess_idr, covered_pct, jumlah_instrumen
    - covered_pct = (covered / total_exposure) × 100, diformat 2 desimal
    - Sort default: excess_idr DESC (paling berisiko di atas)
    - Cursor pagination (DEC-022)
    - Total estimate tersedia di pagination.totalEstimate
  AND tabel mendukung: sort multi-kolom + filter per nasabah/bank + text search + export CSV/XLSX
  AND audit log entry ditulis: action = 'LPS_AGGREGATOR.PREVIEW', actor = Budi's user_id
```

**Scenario 2 (Happy Path — Filter per bank)**

```gherkin
GIVEN ROLE-RISK user Budi
  AND request: GET /api/v1/ecl/lps/preview?evaluation_date=2026-06-30&filter[bank_id]=CP-BANK-001
WHEN request diproses
THEN response berisi hanya pasangan dengan bank CP-BANK-001
  AND filter chips "Bank BCA" tampil di UI
  AND URL state: /ecl/lps/preview?evaluation_date=2026-06-30&filter[bank_id]=CP-BANK-001 (deep-link)
```

**Scenario 3 (Export — dataset < 10k row: inline)**

```gherkin
GIVEN jumlah pasangan (nasabah, bank) = 250
  AND ROLE-RISK user Budi klik Export → XLSX dengan filter aktif evaluation_date=2026-06-30
WHEN GET /api/v1/ecl/lps/preview/export?format=xlsx&evaluation_date=2026-06-30 dipanggil
THEN response Content-Disposition: attachment; filename="lps-preview-20260630.xlsx"
  AND XLSX memuat semua 250 baris dengan filter yang sama aktif
  AND header row: "Nasabah", "Bank", "Total Exposure (IDR)", "Cap LPS (IDR)", "Covered (IDR)", "Excess (IDR)", "Covered %", "Jumlah Instrumen"
  AND audit log: action = 'LPS_AGGREGATOR.EXPORT', entity_type = 'ecl.lps_aggregator', after_jsonb = {format, row_count, evaluation_date}
```

**Scenario 4 (Error — tanggal tidak memiliki kurs JISDOR)**

```gherkin
GIVEN evaluationDate = 2026-06-28 (hari Minggu — tidak ada kurs BI JISDOR)
  AND ada instrumen DEPOSITO USD yang perlu konversi
WHEN GET /api/v1/ecl/lps/preview?evaluation_date=2026-06-28
THEN response 422:
    code: "FX_RATE_NOT_FOUND"
    message: "Kurs BI JISDOR untuk USD tidak tersedia pada 2026-06-28. Gunakan tanggal hari kerja terakhir yang tersedia."
    details: [{ field: "evaluation_date", rule: "fx_rate_required_for_fcy_instruments" }]
```

**Scenario 5 (Permission denied — ROLE yang tidak berwenang)**

```gherkin
GIVEN user dengan role ROLE-MAKER-TR (tidak memiliki lps_aggregator.preview)
WHEN GET /api/v1/ecl/lps/preview?evaluation_date=2026-06-30
THEN response 403:
    code: "FORBIDDEN"
    message: "Permission lps_aggregator.preview tidak terpenuhi"
```

---

## Story APP-C-LPS-004 — Override: Exclude Instrument dari LPS Pool

**Actor**: ROLE-RISK (proposer/maker) + ROLE-ALCO (approver)
**Trigger**: ROLE-RISK identifikasi instrumen yang seharusnya di-exclude dari LPS pool (mis. interbank placement bilateral, structured deposit yang tidak eligible LPS). Membuat proposal exclusion.
**Goal**: Mengizinkan pengecualian instrumen tertentu dari LPS coverage pool via workflow 4-eyes yang teraudit, sehingga instrumen tersebut di-ECL penuh (tanpa LPS benefit).

**Pre-conditions**:
- Instrumen target memiliki `tipe_instrumen = 'DEPOSITO'`, `status = 'AKTIF'`
- `mst.counterparty.eligible_lps_flag = TRUE` untuk bank terkait (artinya default masuk LPS pool)
- ROLE-RISK memiliki permission `lps_aggregator.override`
- ROLE-ALCO memiliki permission `lps_aggregator.override.approve`
- Tabel `ecl.lps_exclusion_override` sudah ada (migration 000023)

**Workflow**: 4-eyes (Maker=ROLE-RISK → Approver=ROLE-ALCO). Tidak perlu Reviewer terpisah (operasional, bukan parameter master). SoD: `maker_id ≠ approver_id`.

**Audit events**: `LPS_EXCLUSION.PROPOSE` (saat submit), `LPS_EXCLUSION.APPROVE` (saat ALCO approve), `LPS_EXCLUSION.REJECT` (saat reject). Semua ditulis ke `aud.audit_log` in-transaction.

**DDL Required (migration 000023)**:
```sql
CREATE TABLE ecl.lps_exclusion_override (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    instrumen_id     UUID NOT NULL REFERENCES mst.instrumen(id),
    alasan           TEXT NOT NULL,   -- CHECK: length(alasan) >= 30
    effective_from   DATE NOT NULL,
    effective_to     DATE,            -- NULL = berlaku selamanya
    workflow_status  VARCHAR(30) NOT NULL DEFAULT 'DRAFT',
    maker_id         UUID NOT NULL REFERENCES sec.user(id),
    approver_id      UUID REFERENCES sec.user(id),
    approved_at      TIMESTAMPTZ,
    rejected_at      TIMESTAMPTZ,
    rejection_reason TEXT,
    -- audit cols
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by  UUID NOT NULL REFERENCES sec.user(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by  UUID NOT NULL REFERENCES sec.user(id),
    deleted_at  TIMESTAMPTZ,
    deleted_by  UUID REFERENCES sec.user(id),
    row_version BIGINT NOT NULL DEFAULT 1,
    tenant_id   TEXT NOT NULL DEFAULT 'TUGURE',
    CONSTRAINT chk_lps_override_alasan_min_len CHECK (length(alasan) >= 30),
    CONSTRAINT chk_lps_override_sod CHECK (maker_id <> approver_id),
    CONSTRAINT chk_lps_override_workflow CHECK (
        workflow_status IN ('DRAFT','PENDING_APPROVAL','APPROVED','REJECTED','REVOKED')
    )
);
CREATE INDEX idx_lps_exclusion_instrumen ON ecl.lps_exclusion_override(instrumen_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_lps_exclusion_active ON ecl.lps_exclusion_override(instrumen_id, effective_from)
    WHERE workflow_status = 'APPROVED' AND deleted_at IS NULL;
```

---

### AC Gherkin — APP-C-LPS-004

**Scenario 1 (Happy Path — Propose dan Approve exclusion)**

```gherkin
GIVEN ROLE-RISK user Andi (user_id = "U-RISK-001") login
  AND instrumen DEPOSITO-007 saat ini masuk LPS pool (eligible_lps_flag = TRUE di bank-nya)
  AND tidak ada exclusion override aktif untuk DEPOSITO-007
WHEN Andi POST /api/v1/ecl/lps/exclusion-overrides dengan:
  {
    "instrumen_id": "DEPOSITO-007",
    "alasan": "Interbank placement bilateral perjanjian khusus — tidak eligible LPS per konfirmasi legal",
    "effective_from": "2026-07-01"
  }
THEN response 201:
    { "data": { "id": "OVR-001", "workflow_status": "PENDING_APPROVAL", ... } }
  AND toast UI: "Proposal exclusion LPS untuk DEPOSITO-007 berhasil disubmit. Menunggu persetujuan ROLE-ALCO."
  AND audit log: action = 'LPS_EXCLUSION.PROPOSE', entity_id = "OVR-001"
  AND ROLE-ALCO mendapat notifikasi (via notification queue) untuk mereview

GIVEN ROLE-ALCO user Diana (user_id = "U-ALCO-001") mereview proposal OVR-001
  AND Diana ≠ Andi (SoD terpenuhi)
WHEN Diana POST /api/v1/ecl/lps/exclusion-overrides/OVR-001/approve dengan:
  { "comment": "Disetujui, sesuai kontrak bilateral ref: XXX-2026" }
THEN response 200: { "workflow_status": "APPROVED", "approved_at": "..." }
  AND audit log: action = 'LPS_EXCLUSION.APPROVE', entity_id = "OVR-001", actor = Diana
  AND effective: sejak ECL calc run berikutnya dengan evaluationDate ≥ 2026-07-01,
      DEPOSITO-007 di-exclude dari LPS pool
```

**Scenario 2 (Error — alasan terlalu pendek)**

```gherkin
GIVEN ROLE-RISK user Andi POST proposal dengan alasan = "Tidak layak"  (22 karakter < 30)
WHEN request diproses
THEN response 400:
    code: "VALIDATION_FAILED"
    details: [{ field: "alasan", rule: "min_length_30", message: "Alasan exclusion minimal 30 karakter" }]
  AND tidak ada record dibuat di ecl.lps_exclusion_override
```

**Scenario 3 (Error — SoD violation: approver = maker)**

```gherkin
GIVEN ROLE-RISK user Andi membuat proposal OVR-002
  AND Andi juga memiliki role ROLE-ALCO (dual-role user)
WHEN Andi mencoba POST /api/v1/ecl/lps/exclusion-overrides/OVR-002/approve via API langsung
THEN response 403:
    code: "SOD_VIOLATION"
    message: "Maker tidak dapat menjadi approver untuk proposal exclusion yang sama"
  AND proposal tetap di status PENDING_APPROVAL
```

**Scenario 4 (Reject — ALCO menolak proposal)**

```gherkin
GIVEN proposal OVR-003 dalam status PENDING_APPROVAL
  AND ROLE-ALCO user Diana mereview dan menolak
WHEN Diana POST /api/v1/ecl/lps/exclusion-overrides/OVR-003/reject dengan:
  { "comment": "Instrumen ini eligible LPS sesuai PBI No. 22/XX — tidak perlu exclude" }
THEN response 200: { "workflow_status": "REJECTED", "rejected_at": "..." }
  AND DEPOSITO target tetap masuk LPS pool
  AND audit log: action = 'LPS_EXCLUSION.REJECT', entity_id = "OVR-003"
  AND ROLE-RISK mendapat notifikasi rejection dengan reason tersebut
```

**Scenario 5 (Edge — proposal duplikat: instrumen sudah punya exclusion aktif)**

```gherkin
GIVEN instrumen DEPOSITO-007 sudah memiliki exclusion override dengan workflow_status = 'APPROVED'
  AND masih dalam periode effective (effective_to IS NULL)
WHEN ROLE-RISK submit proposal baru untuk DEPOSITO-007
THEN response 409:
    code: "CONFLICT"
    message: "DEPOSITO-007 sudah memiliki LPS exclusion override yang aktif (OVR-001). Revoke terlebih dahulu sebelum membuat override baru."
```

---

## Story APP-C-LPS-005 — Bulk Aggregate untuk Calc Run

**Actor**: ECL calc engine (Asynq worker — `internal/ecl/lps`), dipanggil oleh P4-M7
**Trigger**: Asynq job ECL calc run dimulai. Sebelum loop instrumen individu, engine memanggil `BulkAggregate()` sekali untuk seluruh scope instrumen DEPOSITO dalam periodeID.
**Goal**: Mengembalikan seluruh `instrumen_breakdown` untuk semua pasangan (nasabah, bank) aktif dalam satu panggilan; menghindari N+1 query per instrumen. Performance SLA ≤ 1 detik untuk 5.000 instrumen (P95).

**Pre-conditions**:
- Sama dengan Story 1
- Kurs IDR semua mata uang yang digunakan tersedia di `mst.kurs` untuk evaluationDate

**Permissions**: `lps_aggregator.compute` (internal — tidak ada HTTP endpoint publik; dipanggil Go-to-Go via service interface)

**Audit events**: Tidak menulis audit langsung. Pemanggil (P4-M7/M8) yang menulis audit saat hasil disimpan.

---

### AC Gherkin — APP-C-LPS-005

**Scenario 1 (Happy Path — Bulk call untuk semua instrumen aktif)**

```gherkin
GIVEN periode Juni 2026, evaluationDate = 2026-06-30
  AND 5.000 instrumen DEPOSITO AKTIF dengan berbagai pasangan (nasabah, bank)
  AND mst.lps_coverage active record tersedia
  AND mst.kurs lengkap untuk semua mata uang yang digunakan
WHEN LPS Aggregator.BulkAggregate(ctx, periodeID, evaluationDate) dipanggil
THEN dikembalikan map[instrumen_id]LPSBreakdown untuk 5.000 instrumen
  AND durasi eksekusi ≤ 1 detik (P95 target, diukur via metrik Prometheus)
  AND TIDAK ada N+1 query — implementasi menggunakan batch JOIN query ke mst.instrumen
      + mst.counterparty + mst.lps_coverage + mst.kurs dalam satu atau beberapa query batch
  AND setiap LPSBreakdown berisi: {ead_idr, covered_idr, excess_idr, lps_excluded, covered_flag}
```

**Scenario 2 (Consistency — BulkAggregate vs single Aggregate: hasil harus identik)**

```gherkin
GIVEN instrumen DEPOSITO-001, DEPOSITO-002 (seperti di Story 1 Scenario 1)
  AND evaluationDate = 2026-06-30
WHEN:
  A: BulkAggregate dipanggil dengan [DEPOSITO-001, DEPOSITO-002, ...all instruments]
  B: Aggregate dipanggil per pasangan secara individual untuk DEPOSITO-001 dan DEPOSITO-002
THEN hasil covered, excess, breakdown untuk DEPOSITO-001 dan DEPOSITO-002
    dari hasil A = hasil B (deterministic, tidak ada perbedaan numerik)
```

**Scenario 3 (Edge — zero instrumen dalam scope)**

```gherkin
GIVEN periodeID valid tetapi tidak ada instrumen DEPOSITO AKTIF yang masuk scope
    (mis. semua instrumen FVTPL atau bukan DEPOSITO)
WHEN BulkAggregate(ctx, periodeID, evaluationDate) dipanggil
THEN dikembalikan empty map (bukan error)
  AND calc run melanjutkan proses untuk tipe instrumen lain
  AND log entry: "LPS Aggregator: 0 DEPOSITO instruments in scope for periode [periodeID]"
```

**Scenario 4 (Error — partial kurs missing: fail fast)**

```gherkin
GIVEN 4.990 instrumen IDR dan 10 instrumen USD
  AND kurs USD tidak tersedia di mst.kurs untuk evaluationDate
WHEN BulkAggregate(ctx, periodeID, evaluationDate) dipanggil
THEN error dikembalikan:
    code: "FX_RATE_NOT_FOUND"
    message: "Kurs BI JISDOR untuk USD tidak tersedia pada [evaluationDate]. 10 instrumen DEPOSITO USD tidak dapat di-proses."
  AND calc run job gagal dengan status 'failed' (tidak partial-complete)
  AND error detail mencantumkan daftar instrumen_id yang terdampak (max 10 untuk preview, total count di error)
```

---

## Ringkasan Data References

| Story | Tabel Read | Tabel Write | Permission |
|---|---|---|---|
| LPS-001 | `mst.instrumen`, `mst.counterparty`, `mst.lps_coverage`, `mst.kurs`, `ecl.lps_exclusion_override` | — (read-only service) | `lps_aggregator.compute` |
| LPS-002 | Hasil LPS-001, `mst.pd_pefindo`, `mst.lgd_basel`, `mst.bobot_skenario`, `mst.impact_mev_pd` | `ecl.calc_header` (lps cols), `ecl.calc_detail_skenario` | `lps_aggregator.compute` |
| LPS-003 | `mst.instrumen`, `mst.counterparty`, `mst.lps_coverage`, `mst.kurs` (on-the-fly) | `aud.audit_log` | `lps_aggregator.preview` |
| LPS-004 | `mst.instrumen`, `ecl.lps_exclusion_override` | `ecl.lps_exclusion_override`, `aud.audit_log` | `lps_aggregator.override` (propose), `lps_aggregator.override.approve` (ALCO) |
| LPS-005 | Sama dengan LPS-001 (bulk) | — | `lps_aggregator.compute` |

## Ringkasan Audit Events

| Event | Kapan | Actor |
|---|---|---|
| `LPS_AGGREGATOR.PREVIEW` | Setiap panggilan GET /ecl/lps/preview | ROLE-RISK |
| `LPS_AGGREGATOR.EXPORT` | Setiap export (CSV/XLSX) dari preview | ROLE-RISK |
| `LPS_EXCLUSION.PROPOSE` | Submit proposal exclusion | ROLE-RISK |
| `LPS_EXCLUSION.APPROVE` | ALCO menyetujui proposal | ROLE-ALCO |
| `LPS_EXCLUSION.REJECT` | ALCO menolak proposal | ROLE-ALCO |
| `LPS_EXCLUSION.REVOKE` | Pencabutan override aktif (out-of-scope, tercantum untuk completeness) | ROLE-RISK atau ROLE-ALCO |

---

## Handoff Notes untuk System Analyst

1. **Go interface yang diperlukan** untuk `ecl-eir-engineer`:

```go
type LPSAggregatorService interface {
    // Aggregate per pasangan (counterpartyID, bankID) untuk list instrumen tertentu
    Aggregate(ctx context.Context, evaluationDate time.Time, instrumenIDs []uuid.UUID) ([]PairAggregation, error)

    // Bulk: satu call untuk semua instrumen DEPOSITO aktif dalam periode
    BulkAggregate(ctx context.Context, periodeID uuid.UUID, evaluationDate time.Time) (map[uuid.UUID]LPSBreakdown, error)
}

type PairAggregation struct {
    CounterpartyID   uuid.UUID
    BankID           uuid.UUID
    TotalExposureIDR decimal.Decimal
    CoveredIDR       decimal.Decimal
    ExcessIDR        decimal.Decimal
    Breakdown        []InstrumenBreakdown
}

type InstrumenBreakdown struct {
    InstrumenID    uuid.UUID
    EAD_IDR        decimal.Decimal
    CoveredPorsi   decimal.Decimal
    ExcessPorsi    decimal.Decimal
    LPSExcluded    bool
    ExclusionReason string  // kosong jika tidak excluded
    LPSFullCovered bool
}

type LPSBreakdown struct {
    EAD_IDR        decimal.Decimal
    CoveredIDR     decimal.Decimal
    ExcessIDR      decimal.Decimal
    LPSExcluded    bool
    LPSFullCovered bool
}
```

2. **Error codes baru yang perlu ditambah ke `api-conventions.md`**:
   - `LPS_COVERAGE_NO_ACTIVE_PARAM` (500/422 — blocking calc run)
   - `FX_RATE_NOT_FOUND` (422)
   - `LPS_EXCLUSION_ALREADY_ACTIVE` (409)

3. **Migration 000023** perlu mencakup:
   - CREATE TABLE `ecl.lps_exclusion_override` (DDL di Story 4)
   - ALTER TABLE `ecl.calc_header` ADD COLUMNS `lps_covered_idr`, `lps_excess_idr`, `lps_covered_flag`
   - Precision fix jika `ecl.calc_header` masih punya NUMERIC(20,2) (init schema 0001 pakai (20,2) — perlu widening ke (20,4) per DEC-016)

4. **Alokasi covered per instrumen** (Story 1 Scenario 1): diperlukan keputusan ordering. Rekomendasi: urut `tanggal_penempatan ASC` (FIFO — instrumen tertua di-cover lebih dulu). Jika FIFO tidak ditetapkan di FSD-APP-C, flag sebagai OQ-M3-7 untuk dikonfirmasi.
