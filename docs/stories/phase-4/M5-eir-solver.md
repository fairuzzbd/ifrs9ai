# P4-M5 — EIR Newton-Raphson Solver + Amortisasi Schedule: User Stories

**Story Set ID**: P4-M5
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 2)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-11
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4 (EIR + amortisasi schedule, amendment re-estimation)
**Linked BRD**: BRD §8.4 (EIR Requirements), RACI: ROLE-AKUN (R/propose amandemen), ROLE-RISK (R/review + compute trigger), ROLE-ALCO (A/approve parameter EIR amendment), ROLE-AUDIT (I)
**Linked Decision Log**:
- DEC-013 — EIR: Newton-Raphson IRR, tolerance `1e-10`, max 100 iter, presisi 8 desimal
- DEC-016 — EIR stored as `NUMERIC(10,8)`. IDR amounts `NUMERIC(20,4)`. No float64.
- DEC-017 — Workflow 4-eyes untuk EIR amendment (plan §6). SoD `maker_id ≠ reviewer_id ≠ approver_id`.
- DEC-018 — Audit trail append-only. `ecl.*` no hard delete. Schedule rows immutable setelah insert.
- OQ-H (plan §7) — EIR berlaku untuk AC + FVOCI debt. FVTPL + FVOCI equity = no EIR.

**Depends on**: P4-M2 (EAD helpers — outstanding + accrued), Phase 3 (`mst.instrumen` dengan `kupon`, `tanggal_jatuh_tempo`, `biaya_transaksi_capitalized`, `eir_method_flag`, `day_count_convention`)

**Handoff berikutnya**:
- `system-analyst` — OpenAPI fragment + Go interface `EIRService` + state machine workflow amendment
- `data-modeler` — migration 000024: `ecl.eir_amortization_schedule` schema-fix (audit cols + precision) + `ecl.eir_reestimation_log` schema-fix. Lihat OQ-M5-1 di bawah.
- `ecl-eir-engineer` — implementasi `backend/internal/ecl/eir/` (solver + schedule builder)

---

## Konteks & Dependensi

### Skema yang digunakan (sudah ada di `000001_init_schema.up.sql`)

| Tabel | Kolom Kunci | Catatan |
|---|---|---|
| `ecl.eir_amortization_schedule` | `instrumen_id`, `periode_seq`, `opening_carrying`, `pendapatan_bunga_eir`, `amortisasi_p_d`, `pelunasan_pokok`, `closing_carrying`, `eir_periode NUMERIC(12,8)`, `status_posting`, `recomputed_from_seq` | **Sudah ada** di 000001. Butuh schema-fix (audit cols + precision). Partial UNIQUE index `WHERE recomputed_from_seq IS NULL` untuk active rows. |
| `ecl.eir_reestimation_log` | `instrumen_id`, `eir_sebelum`, `eir_sesudah`, `catch_up_adjustment`, `modifikasi_terms_json`, `maker_id`, `reviewer_id`, `approver_id`, `workflow_status` | **Sudah ada** di 000001. Butuh schema-fix (audit cols). 4-eyes workflow columns ada. |
| `trx.amortisasi` | `instrumen_id`, `schedule_periode_id` (FK ke `eir_amortization_schedule`), `amortisasi_premium_diskonto_idr` | **Sudah ada** di 000001. |
| `mst.instrumen` | `kupon NUMERIC(8,4)`, `tanggal_jatuh_tempo`, `biaya_transaksi_capitalized`, `eir_awal NUMERIC(12,8)`, `eir_method_flag`, `day_count_convention`, `klasifikasi_psak71`, `nominal` | Input source untuk cashflow projection. |

### Versi/amendment mechanism (penting)

Skema aktual `000001` menggunakan **`recomputed_from_seq`** (bukan `schedule_version` + `effective_from`/`effective_to` seperti di skill reference). Mekanisme:
- Rows aktif = `recomputed_from_seq IS NULL`
- Rows lama = `recomputed_from_seq = {periode_seq pertama dari schedule baru}`
- Partial UNIQUE INDEX: `(instrumen_id, periode_seq) WHERE recomputed_from_seq IS NULL`
- Saat amandemen: **INSERT** rows baru (future periods, `recomputed_from_seq = NULL`), **UPDATE** rows lama `recomputed_from_seq = periode_seq_mulai_amandemen` (ini adalah satu-satunya UPDATE yang diizinkan — mengisi kolom metadata, bukan mengubah nilai finansial)

Catatan: UPDATE `recomputed_from_seq` pada rows lama tidak mengubah nilai finansial (amounts tetap), hanya menandai bahwa row ini digantikan oleh schedule baru. Nilai `opening_carrying`, `pendapatan_bunga_eir`, dst di rows lama **TIDAK BOLEH DIUBAH**.

### Permissions baru (perlu didefinisikan di `sec.permission`)

| Permission | Holders | Deskripsi |
|---|---|---|
| `eir.compute` | ROLE-RISK (manual trigger), System (auto) | Compute EIR dari cashflow |
| `eir.preview` | ROLE-RISK, ROLE-AKUN, ROLE-AUDIT | Lihat schedule + hasil compute |
| `eir.amend.propose` | ROLE-AKUN | Submit proposal amandemen kontrak |
| `eir.amend.review` | ROLE-RISK | Review proposal amandemen |
| `eir.amend.approve` | ROLE-ALCO | Final approve amandemen (step-up MFA per DEC-027) |
| `eir.bulk_recompute` | ROLE-RISK, System | Trigger bulk re-compute batch |

---

## Story P4-M5-EIR-001 — Compute EIR saat Origination

**Actor**: System (auto-triggered) atau ROLE-RISK (manual trigger via API)
**Trigger**: `mst.instrumen.status` berubah menjadi `'AKTIF'` dan `klasifikasi_psak71 IN ('AC', 'FVOCI')` dan `eir_method_flag = TRUE` dan `eir_awal IS NULL`
**Goal**: Solver IRR Newton-Raphson menghasilkan EIR per periode, menyimpannya di `mst.instrumen.eir_awal`, dan mengembalikan metadata konvergensi.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.1 (EIR computation)
**Permission**: `eir.compute`
**Audit Event**: `EIR.COMPUTE` on `mst.instrumen` — ditulis ke `aud.audit_log` in-transaction

### Pre-conditions
- Instrumen telah di-approve dan berstatus `AKTIF`.
- `klasifikasi_psak71 IN ('AC', 'FVOCI')` (OQ-H: dikonfirmasi berlaku untuk kedua klasifikasi).
- `eir_method_flag = TRUE`.
- `eir_awal IS NULL` (belum pernah di-compute).
- Cashflow projection tersedia: dapat di-generate otomatis dari `nominal`, `kupon`, `frekuensi_bunga`, `tanggal_penempatan`, `tanggal_jatuh_tempo`, `biaya_transaksi_capitalized`. Atau disuplai eksplisit via API.
- `day_count_convention` tersedia di instrumen (default `'ACT/365'`).

### Steps
1. Ambil instrumen dari `mst.instrumen`.
2. Build cashflow array: `CF_0 = -(nominal + biaya_transaksi_capitalized)`, `CF_t = kupon_payment_t` untuk setiap periode, `CF_N += pelunasan_pokok`.
3. Seed: `r_initial = kupon` jika tersedia, else `0.10`.
4. Jalankan Newton-Raphson hingga `|f(r)| < 1e-10` atau `|r_n - r_{n-1}| < 1e-10` atau `iter >= 100`.
5. Jika konvergen: simpan `eir_awal` di `mst.instrumen`, simpan metadata (`iterations_used`, `convergence_residual`).
6. Tulis `EIR.COMPUTE` ke `aud.audit_log`.
7. Return response: `{ eir_per_period, eir_annual_equivalent, iterations_used, convergence_residual }`.

### Acceptance Criteria

```gherkin
Feature: Compute EIR on instrument origination

  Background:
    Given instrumen "OBL-2026-00001" tipe OBLIGASI dengan:
      | field                          | value               |
      | klasifikasi_psak71             | AC                  |
      | nominal                        | 1_000_000_000       |
      | kupon                          | 0.0800              |
      | frekuensi_bunga                | SEMESTERAN          |
      | tanggal_penempatan             | 2026-01-01          |
      | tanggal_jatuh_tempo            | 2031-01-01          |
      | biaya_transaksi_capitalized    | 5_000_000           |
      | eir_method_flag                | true                |
      | day_count_convention           | ACT/365             |
      | status                         | AKTIF               |
      | eir_awal                       | NULL                |

  Scenario: Happy path — obligasi at-discount, EIR konvergen
    Given cashflow array di-generate otomatis dari field instrumen di atas
    And seed r_initial = kupon = 0.0800 (semesteran → 0.04)
    When EIR compute dipanggil untuk instrumen "OBL-2026-00001"
    Then solver konvergen dalam ≤ 100 iterasi
    And nilai eir_per_period lebih besar dari 0.04 (karena biaya transaksi → discount)
    And mst.instrumen.eir_awal di-update dengan nilai tersebut
    And mst.instrumen.tanggal_eir_computed di-update ke tanggal hari ini
    And response berisi { eir_per_period, eir_annual_equivalent, iterations_used, convergence_residual }
    And convergence_residual < 1e-10
    And aud.audit_log berisi event "EIR.COMPUTE" dengan entity_id = instrumen.id
    And tidak ada float64 di computation path (verified via golangci-lint forbidigo)

  Scenario: Deposito tanpa biaya transaksi — EIR = kupon rate
    Given instrumen "DEP-2026-00001" tipe DEPOSITO dengan:
      | field                          | value               |
      | klasifikasi_psak71             | AC                  |
      | nominal                        | 500_000_000         |
      | kupon                          | 0.0600              |
      | frekuensi_bunga                | BULANAN             |
      | biaya_transaksi_capitalized    | 0                   |
      | eir_method_flag                | true                |
    When EIR compute dipanggil
    Then eir_per_period ≈ 0.06 / 12 = 0.005000000000 (presisi ≤ 1e-8)
    And iterations_used ≤ 5

  Scenario: FVTPL instrumen — EIR compute ditolak
    Given instrumen "SHM-2026-00001" dengan klasifikasi_psak71 = "FVTPL"
    When EIR compute dipanggil untuk instrumen tersebut
    Then response HTTP 422 dengan error code "EIR_NOT_APPLICABLE"
    And message "EIR tidak berlaku untuk instrumen FVTPL"
    And tidak ada perubahan di mst.instrumen

  Scenario: FVOCI_ELECTION (equity) — EIR compute ditolak
    Given instrumen dengan klasifikasi_psak71 = "FVOCI_ELECTION"
    When EIR compute dipanggil
    Then response HTTP 422 dengan error code "EIR_NOT_APPLICABLE"

  Scenario: Non-convergence setelah 100 iterasi
    Given cashflow dengan multiple sign-changes yang tidak punya solusi real tunggal
      | t | CF          |
      | 0 | -100        |
      | 1 | 230         |
      | 2 | -132        |
    When EIR compute dipanggil
    Then response HTTP 422 dengan error code "EIR_NON_CONVERGENT"
    And message "Newton-Raphson tidak konvergen dalam 100 iterasi"
    And iterations_used = 100
    And eir_awal di instrumen TIDAK berubah (tetap NULL)
    And event "EIR.COMPUTE_FAILED" ditulis ke aud.audit_log

  Scenario: Cashflow invalid — jatuh tempo NULL untuk instrumen yang butuh schedule
    Given instrumen OBLIGASI tanpa tanggal_jatuh_tempo (NULL)
    When EIR compute dipanggil
    Then response HTTP 422 dengan error code "EIR_CASHFLOW_INVALID"
    And message "tanggal_jatuh_tempo wajib diisi untuk compute EIR OBLIGASI"

  Scenario: Re-compute saat eir_awal sudah terisi (origination ulang diblok)
    Given instrumen "OBL-2026-00001" dengan eir_awal = 0.04123456
    When EIR compute dipanggil tanpa parameter force_recompute
    Then response HTTP 409 dengan error code "EIR_ALREADY_COMPUTED"
    And message "EIR sudah dihitung. Gunakan amendment flow untuk re-estimasi."

  Scenario: POCI flag — EIR compute dengan credit-adjusted cashflow
    Given instrumen dengan tipe_instrumen = "OBLIGASI" dan flag_poci = TRUE
    And cashflow PD-adjusted disuplai secara eksplisit di request body
    When EIR compute dipanggil dengan flag poci_mode = true
    Then solver berjalan dengan cashflow PD-adjusted
    And response berisi eir_type = "CREDIT_ADJUSTED"
    And mst.instrumen.eir_awal di-set dengan nilai credit-adjusted EIR
    And catatan "POCI: credit-adjusted EIR" ditulis di after_jsonb audit log
```

### Open Questions terkait Story 1
- **OQ-M5-2**: Apakah cashflow projection untuk DEPOSITO bisa selalu di-auto-generate dari `kupon` + `frekuensi_bunga` + `tenor`? Atau ada deposito dengan cashflow tidak reguler yang butuh input manual?
- **OQ-M5-3**: Field `flag_poci` belum ada di `mst.instrumen` schema 000001. Jika POCI di-scope ke Phase 4, data-modeler perlu add kolom di migration 000024. Jika defer, Story 1 Scenario POCI = stub yang selalu error `EIR_POCI_DEFERRED`.

---

## Story P4-M5-EIR-002 — Generate Amortisasi Schedule

**Actor**: EIR Service (auto-invoked setelah Story 1 EIR computed), atau ROLE-RISK (manual trigger via API)
**Trigger**: EIR berhasil di-compute (Story 1) dan `ecl.eir_amortization_schedule` belum ada rows aktif untuk instrumen ini
**Goal**: Generate seluruh schedule amortisasi dari origination sampai jatuh tempo, simpan ke `ecl.eir_amortization_schedule`. Schedule rows bersifat immutable setelah insert.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.2 (amortisasi schedule generation)
**Permission**: `eir.compute` (auto-trigger); `eir.preview` (baca)
**Audit Event**: `EIR.SCHEDULE_GENERATED` on `mst.instrumen`

### Pre-conditions
- `mst.instrumen.eir_awal` sudah terisi (Story 1 selesai).
- Tidak ada rows aktif di `ecl.eir_amortization_schedule` untuk instrumen ini (`recomputed_from_seq IS NULL`).
- Day count convention tersedia (`'ACT/365'` default).

### Steps
1. Load instrumen: nominal, kupon, frekuensi, tanggal_penempatan, tanggal_jatuh_tempo, biaya_transaksi_capitalized, eir_awal.
2. Hitung EIR per-periode dari `eir_awal` (annualized → convert ke rate per frekuensi).
3. Build schedule loop:
   ```
   opening_carrying_1 = nominal + biaya_transaksi_capitalized
   Untuk setiap periode t:
       pendapatan_bunga_eir_t  = opening_carrying_{t-1} × eir_per_periode
       cash_inflow_t           = kupon_kontraktual_t (dari instrumen)
       amortisasi_p_d_t        = pendapatan_bunga_eir_t − cash_inflow_t
       closing_carrying_t      = opening_carrying_{t-1} + amortisasi_p_d_t
       (pelunasan_pokok hanya pada saat pokok dibayar)
   Periode terakhir: closing_carrying_N ≈ 0 (toleransi rounding HALF_EVEN ≤ IDR 1)
   ```
4. INSERT rows ke `ecl.eir_amortization_schedule` dengan `recomputed_from_seq = NULL`, `status_posting = 'PROYEKSI'`.
5. Tulis `EIR.SCHEDULE_GENERATED` ke `aud.audit_log`.

### Acceptance Criteria

```gherkin
Feature: Generate amortisasi schedule untuk instrumen

  Background:
    Given instrumen "OBL-2026-00001" dengan:
      | eir_awal               | 0.04200000 (per semester)       |
      | nominal                | 1_000_000_000                   |
      | biaya_transaksi_cap    | 5_000_000                       |
      | kupon                  | 0.08 (8% p.a., semesteran)      |
      | tenor                  | 5 tahun (10 periode semesteran) |
    And tidak ada rows aktif di ecl.eir_amortization_schedule

  Scenario: Happy path — schedule ter-generate dengan balancing
    When schedule generation dipanggil
    Then ecl.eir_amortization_schedule berisi tepat 10 rows dengan recomputed_from_seq IS NULL
    And opening_carrying row-1 = nominal + biaya_transaksi = 1_005_000_000
    And setiap row: closing_carrying = opening_carrying + amortisasi_p_d
    And Σ amortisasi_p_d (semua periode) + principal = total cashflow (reconcile)
    And closing_carrying pada row ke-10 ≈ 0 (delta ≤ IDR 1, banker's rounding HALF_EVEN)
    And semua amounts bertipe NUMERIC(20,4) — tidak ada float64
    And semua rows berstatus status_posting = 'PROYEKSI'
    And semua rows memiliki eir_periode = 0.04200000
    And aud.audit_log berisi event "EIR.SCHEDULE_GENERATED"

  Scenario: Schedule sudah ada (duplikasi ditolak)
    Given instrumen "OBL-2026-00001" sudah punya rows aktif di eir_amortization_schedule
    When schedule generation dipanggil kembali tanpa parameter force
    Then response HTTP 409 dengan error code "EIR_SCHEDULE_DUPLICATE"
    And tidak ada rows baru di-insert

  Scenario: Amortisasi diskonto — carrying naik menuju par
    Given obligasi dibeli di bawah par (nominal 1B, harga beli 950 juta, diskonto 50 juta)
    And biaya_transaksi_capitalized = 0
    When schedule generation dipanggil
    Then opening_carrying row-1 = 950_000_000
    And setiap periode: amortisasi_p_d > 0 (diskonto diamortisasi naik ke par)
    And closing_carrying row-N ≈ 1_000_000_000 (par)

  Scenario: Amortisasi premium — carrying turun menuju par
    Given obligasi dibeli di atas par (harga beli 1.05B)
    When schedule generation dipanggil
    Then setiap periode: amortisasi_p_d < 0 (premium diamortisasi turun ke par)

  Scenario: Immutability — rows tidak dapat di-UPDATE setelah insert
    Given rows aktif sudah ada
    When UPDATE langsung dilakukan pada ecl.eir_amortization_schedule
    Then database trigger menolak UPDATE dan return error "ECL_SCHEDULE_IMMUTABLE"

  Scenario: EIR belum dihitung — schedule generation ditolak
    Given mst.instrumen.eir_awal IS NULL
    When schedule generation dipanggil
    Then response HTTP 422 dengan error code "EIR_NOT_YET_COMPUTED"
```

### Data contract (output row per periode)
```
instrumen_id            UUID
periode_seq             INT          -- 1, 2, ..., N
tanggal_posting         DATE         -- tanggal jatuh tempo per periode
opening_carrying        NUMERIC(20,4)
cash_inflow             NUMERIC(20,4) -- kupon kontraktual
pendapatan_bunga_eir    NUMERIC(20,4) -- opening × eir_periode
amortisasi_p_d          NUMERIC(20,4) -- pendapatan_bunga_eir − cash_inflow
pelunasan_pokok         NUMERIC(20,4) -- 0 kecuali periode pelunasan
closing_carrying        NUMERIC(20,4) -- opening + amortisasi_p_d − pelunasan_pokok
eir_periode             NUMERIC(10,8) -- EIR per periode (bukan annualized)
recomputed_from_seq     NULL         -- rows aktif selalu NULL
status_posting          'PROYEKSI'
```

---

## Story P4-M5-EIR-003 — Read Amortisasi Schedule untuk Instrumen + Periode

**Actor**: ECL Engine (konsumer internal untuk EAD accrued interest di P4-M2/M7) + ROLE-RISK (preview via UI) + ROLE-AUDIT (read-only)
**Trigger**: ECL calc run membutuhkan `pendapatan_bunga_eir` untuk periode tertentu; atau ROLE-RISK membuka schedule view di UI
**Goal**: Kembalikan row schedule aktif yang berlaku untuk tanggal/periode yang diminta
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.3 (schedule lookup)
**Permission**: `eir.preview`
**Audit Event**: baca tidak di-audit (read-only). Export di-audit dengan `EIR.SCHEDULE_EXPORT`.

### Pre-conditions
- Caller telah ter-autentikasi dengan permission `eir.preview`.
- `instrumen_id` valid dan tidak deleted.

### Query logic
```sql
SELECT *
FROM ecl.eir_amortization_schedule
WHERE instrumen_id = $1
  AND tanggal_posting >= $2          -- tanggal_mulai periode
  AND tanggal_posting <= $3          -- tanggal_akhir periode
  AND recomputed_from_seq IS NULL    -- active rows only
ORDER BY periode_seq ASC;
```

### Acceptance Criteria

```gherkin
Feature: Read amortisasi schedule per instrumen per periode

  Scenario: Happy path — single periode, satu row aktif
    Given instrumen "OBL-2026-00001" punya schedule aktif 10 rows
    When GET /api/v1/eir/schedule/{instrumen_id}?periode_id={periode_juni_2026}
    Then response HTTP 200
    And data berisi row yang tanggal_posting-nya jatuh dalam periode Juni 2026
    And hanya rows dengan recomputed_from_seq IS NULL yang dikembalikan
    And field eir_periode bertipe number (bukan string)

  Scenario: Instrumen belum punya schedule (amortization defer case)
    Given instrumen "DEP-2026-00099" belum punya rows di eir_amortization_schedule
    When GET /api/v1/eir/schedule/{instrumen_id}?periode_id={periode_juni_2026}
    Then response HTTP 200
    And data = [] (array kosong)
    And warning: "Instrumen ini belum punya schedule EIR. ECL engine akan menggunakan outstanding saldo sebagai carrying."
    And HTTP status TETAP 200 (bukan 4xx) — graceful, bukan error

  Scenario: Instrumen sudah punya dua versi schedule (setelah amandemen)
    Given instrumen "OBL-2026-00001" punya schedule version awal (periode 1-10)
    And setelah amandemen di periode 5, ada rows baru (periode 5-10, recomputed_from_seq = NULL)
    And rows lama periode 5-10 di-set recomputed_from_seq = 5
    When GET schedule untuk periode 7
    Then hanya satu row aktif untuk periode 7 yang dikembalikan (recomputed_from_seq IS NULL)
    And rows lama tidak muncul di default response

  Scenario: View riwayat semua versi (include superseded rows)
    Given instrumen "OBL-2026-00001" dengan riwayat amandemen
    When GET /api/v1/eir/schedule/{instrumen_id}?include_superseded=true
    Then response berisi semua rows — aktif dan superseded
    And superseded rows menampilkan field recomputed_from_seq terisi
    And hanya ROLE-RISK dan ROLE-AUDIT yang boleh pakai parameter include_superseded=true
    And ROLE-AKUN biasa yang coba include_superseded=true mendapat HTTP 403

  Scenario: Instrumen FVTPL — schedule tidak ada dan tidak perlu
    Given instrumen "SHM-2026-00001" dengan klasifikasi_psak71 = "FVTPL"
    When GET schedule
    Then response HTTP 200 dengan data = [] dan info "EIR tidak berlaku untuk FVTPL"

  Scenario: List schedule dengan sort + paging + filter (UX §1)
    When GET /api/v1/eir/schedule/{instrumen_id}?sort=periode_seq:asc&limit=5&cursor=...
    Then response berisi pagination.nextCursor dan hasMore
    And kolom sortable: periode_seq, tanggal_posting, opening_carrying, closing_carrying
    And filter: status_posting, recomputed_from_seq IS NULL / IS NOT NULL

  Scenario: Export schedule ke CSV (UX §1 export)
    When ROLE-RISK klik Export CSV di schedule table
    Then file CSV ter-download dengan header Bahasa Indonesia
    And aud.audit_log berisi event "EIR.SCHEDULE_EXPORT"
```

---

## Story P4-M5-EIR-004 — Amendment Re-estimation (EIR Re-compute)

**Actor**: ROLE-AKUN (propose) → ROLE-RISK (review) → ROLE-ALCO (approve, MFA step-up)
**Trigger**: Amandemen kontrak instrumen (perubahan kupon, tenor extension, restructure pokok)
**Goal**: Re-compute EIR berdasarkan cashflow revisi, insert schedule baru sebagai versi baru (immutable), marking rows lama, catat di `ecl.eir_reestimation_log` dengan full 4-eyes audit trail.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.4 (amendment re-estimation)
**Permission**: `eir.amend.propose` (ROLE-AKUN), `eir.amend.review` (ROLE-RISK), `eir.amend.approve` (ROLE-ALCO, MFA step-up)
**Audit Events**: `EIR.AMEND_PROPOSED`, `EIR.AMEND_REVIEWED`, `EIR.AMEND_APPROVED`, `EIR.AMEND_REJECTED`
**Workflow**: 4-eyes — ROLE-AKUN (maker) → ROLE-RISK (reviewer) → ROLE-ALCO (approver)

### Pre-conditions
- Instrumen berstatus `AKTIF` dan `eir_awal` sudah terisi.
- Terdapat perubahan kontraktual yang didokumentasikan (dokumen pendukung wajib di-upload ke `doc.upload`).
- `ecl.eir_reestimation_log` menampung proposal amandemen sebelum dieksekusi.

### State machine `eir_reestimation_log.workflow_status`
```
DRAFT → PENDING_REVIEW → PENDING_APPROVAL → APPROVED (schedule diterapkan) / REJECTED
```

### Steps (setelah APPROVED)
1. Load instrumen dan cashflow revisi dari proposal.
2. Re-run Newton-Raphson dengan cashflow baru, seed = `eir_lama` (best starting point).
3. Hitung `catch_up_adjustment` = NPV difference saat tanggal amandemen.
4. UPDATE rows lama di `ecl.eir_amortization_schedule` SET `recomputed_from_seq = first_new_periode_seq` — **hanya kolom tracking ini**, tidak ada perubahan pada amounts finansial.
5. INSERT rows baru untuk sisa tenor mulai `amendment_date` dengan `recomputed_from_seq = NULL`.
6. Update `mst.instrumen.eir_awal` dengan EIR baru.
7. Update `ecl.eir_reestimation_log.workflow_status = 'APPROVED'`, set `approved_at`, `approver_id`, `signature_hash`.
8. Tulis `EIR.AMEND_APPROVED` ke `aud.audit_log`.

### Acceptance Criteria

```gherkin
Feature: EIR Amendment re-estimation dengan 4-eyes workflow

  Background:
    Given instrumen "OBL-2026-00001" dengan eir_awal = 0.04200000
    And schedule aktif 10 rows (periode 1–10, all recomputed_from_seq IS NULL)
    And tanggal amandemen = 2028-01-01 (setelah periode 4 selesai)
    And user AKUN-01 (ROLE-AKUN), RISK-01 (ROLE-RISK), ALCO-01 (ROLE-ALCO) terdaftar

  Scenario: Happy path — full 4-eyes, amandemen berhasil diterapkan
    When AKUN-01 POST /api/v1/eir/amendments dengan:
      | instrumen_id       | OBL-2026-00001                   |
      | amendment_date     | 2028-01-01                       |
      | revised_cashflows  | [kupon baru 9% untuk periode 5+] |
      | alasan             | "Amandemen kupon sesuai negosiasi"|
      | dokumen_id         | {id dari doc.upload}             |
    Then ecl.eir_reestimation_log berisi row baru dengan workflow_status = 'DRAFT'
    And response berisi log_id_kode dan link ke workflow

    When RISK-01 POST /api/v1/eir/amendments/{id}/review dengan comment
    Then workflow_status → 'PENDING_APPROVAL'
    And audit event "EIR.AMEND_REVIEWED" ditulis

    When ALCO-01 POST /api/v1/eir/amendments/{id}/approve dengan MFA step-up token
    Then workflow_status → 'APPROVED'
    And rows lama periode 5–10 di ecl.eir_amortization_schedule di-set recomputed_from_seq = 5
    And amounts finansial pada rows lama TIDAK berubah (opening_carrying, pendapatan_bunga_eir, dst tetap identik)
    And rows baru periode 5–10 di-INSERT dengan recomputed_from_seq IS NULL
    And eir_periode pada rows baru ≠ eir_periode lama (EIR baru dari re-estimasi)
    And mst.instrumen.eir_awal diupdate dengan EIR baru
    And eir_reestimation_log.eir_sesudah berisi EIR baru
    And catch_up_adjustment terisi dengan NPV difference
    And aud.audit_log berisi "EIR.AMEND_APPROVED" dengan before/after EIR comparison
    And signature_hash_approve terisi (SHA-256)

  Scenario: SoD violation — AKUN-01 mencoba approve proposal miliknya sendiri
    Given AKUN-01 sudah submit proposal (maker)
    When AKUN-01 mencoba POST approve endpoint
    Then response HTTP 403 dengan error code "SOD_VIOLATION"
    And message "Maker tidak bisa menjadi approver"

  Scenario: RISK-01 mencoba approve (hanya ALCO yang bisa)
    Given proposal dalam status PENDING_APPROVAL
    When RISK-01 mencoba POST approve endpoint
    Then response HTTP 403 dengan error code "FORBIDDEN"
    And message "Permission eir.amend.approve diperlukan"

  Scenario: ALCO approve tanpa MFA step-up
    Given proposal dalam status PENDING_APPROVAL
    When ALCO-01 POST approve tanpa X-Step-Up-Token header
    Then response HTTP 401 dengan error code "MFA_STEPUP_REQUIRED"

  Scenario: Amandemen ditolak — reject oleh RISK
    When RISK-01 POST /api/v1/eir/amendments/{id}/reject dengan reject_reason
    Then workflow_status → 'REJECTED'
    And reject_reason tersimpan di eir_reestimation_log
    And schedule aktif di eir_amortization_schedule TIDAK berubah
    And aud.audit_log berisi "EIR.AMEND_REJECTED"

  Scenario: Amandemen dengan dokumen pendukung tidak diupload
    When AKUN-01 submit proposal tanpa dokumen_id
    Then response HTTP 422 dengan error code "VALIDATION_FAILED"
    And message "dokumen_pendukung_id wajib diisi untuk amendment proposal"

  Scenario: Re-estimasi gagal konvergen (cashflow amandemen tidak valid)
    Given cashflow revisi yang tidak menghasilkan solusi IRR unik
    When ALCO-01 approve
    Then approval di-rollback
    And workflow_status kembali ke PENDING_APPROVAL
    And error "EIR_NON_CONVERGENT" dicatat di eir_reestimation_log
    And aud.audit_log berisi event "EIR.REESTIMATION_FAILED"

  Scenario: Proposal amandemen tidak bisa dibuat jika instrumen periode hard-closed
    Given periode Juni 2026 sudah hard-closed
    And instrumen OBL-2026-00001 punya rows schedule yang tanggal_posting-nya ≤ Juni 2026
    When AKUN-01 submit proposal dengan amendment_date = 2026-06-15
    Then response HTTP 423 dengan error code "PERIODE_CLOSED"
```

---

## Story P4-M5-EIR-005 — Bulk EIR Re-compute (Batch Reconciliation)

**Actor**: Asynq job (scheduled atau ad-hoc), ROLE-RISK (manual trigger via API)
**Trigger**: ROLE-RISK trigger manual, atau scheduled cron (rekonsiliasi periodik), atau post-parameter-change
**Goal**: Re-validate semua instrumen aktif yang membutuhkan EIR, identify drift (instrumen yang punya `eir_awal` tapi schedule tidak ada / schedule tidak balance / schedule outdated), report tanpa auto-modify.
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §4.5 (bulk operations)
**Permission**: `eir.bulk_recompute` (ROLE-RISK + system)
**Long-running**: UX §3 mandatory — Asynq job + SSE progress + JobProgressPanel
**Audit Event**: `EIR.BULK_RECOMPUTE` (start + complete + per-instrument error)
**SLA**: ≤ 5 detik untuk 1000 instrumen (hanya compute + validation, bukan insert)

### Pre-conditions
- Caller punya permission `eir.bulk_recompute`.
- Tidak ada bulk re-compute job lain yang sedang berjalan untuk tenant yang sama (prevent concurrent runs).

### Steps
1. Query semua instrumen aktif dengan `eir_method_flag = TRUE` dan `klasifikasi_psak71 IN ('AC','FVOCI')`.
2. Untuk setiap instrumen:
   a. Re-compute EIR dari cashflow.
   b. Bandingkan dengan `eir_awal` yang tersimpan.
   c. Validate schedule: rows aktif ada, closing_carrying terakhir ≈ 0, no gaps.
3. Kumpulkan `drift_report`: instrumen dengan EIR drift > 1e-6 atau schedule missing/broken.
4. **Tidak mengubah** `eir_awal` atau schedule. Hanya reporting.
5. Simpan hasil di `sys.job.result_jsonb`.
6. Tulis `EIR.BULK_RECOMPUTE` ke `aud.audit_log`.

### Acceptance Criteria

```gherkin
Feature: Bulk EIR re-compute batch job

  Scenario: Happy path — semua instrumen valid, tidak ada drift
    Given 500 instrumen aktif yang semua eir_awal sudah benar
    When ROLE-RISK POST /api/v1/eir/bulk-recompute (manual trigger)
    Then response HTTP 202 dengan { jobId, statusUrl, streamUrl }
    And Asynq job berjalan di background
    And GET /api/v1/jobs/{jobId} mengembalikan { status: "running", progress: X }
    And SSE stream mengirim event progress setiap ~100 instrumen
    And job selesai dalam ≤ 5 detik untuk 500 instrumen
    And result berisi { total: 500, valid: 500, drift_count: 0, schedule_missing: 0 }
    And aud.audit_log berisi "EIR.BULK_RECOMPUTE" dengan summary

  Scenario: Drift detected — beberapa instrumen EIR mismatch
    Given instrumen "OBL-2026-00002" dengan eir_awal = 0.04000000
    And re-compute menghasilkan EIR = 0.04150000 (drift > 1e-6)
    When bulk re-compute selesai
    Then result.drift_count ≥ 1
    And result.drift_instruments berisi { instrumen_id: "OBL-2026-00002", eir_stored: 0.04000000, eir_computed: 0.04150000, delta: 0.00150000 }
    And eir_awal di mst.instrumen TIDAK berubah (read-only report)
    And ROLE-RISK mendapat notifikasi "EIR Drift ditemukan pada 1 instrumen. Tinjau laporan."

  Scenario: Schedule missing
    Given instrumen "DEP-2026-00050" punya eir_awal tapi tidak ada rows di eir_amortization_schedule
    When bulk re-compute selesai
    Then result.schedule_missing berisi instrumen tersebut
    And instrumen ditandai "ACTION_REQUIRED: generate schedule"

  Scenario: Concurrent job prevention
    Given satu bulk re-compute job sedang berjalan (status = "running")
    When ROLE-RISK POST trigger bulk re-compute kedua
    Then response HTTP 409 dengan error code "CONFLICT"
    And message "Bulk EIR re-compute sedang berjalan (jobId: ...). Tunggu hingga selesai."

  Scenario: SLA verification — 1000 instrumen ≤ 5 detik
    Given 1000 instrumen aktif dengan berbagai tipe dan tenor
    When bulk re-compute dijalankan
    Then completed_at − started_at ≤ 5 detik
    And memory footprint per instrument ≤ 10 KB (compute streaming, bukan load semua ke RAM)

  Scenario: Cancellation — ROLE-RISK cancel job yang berjalan
    Given bulk re-compute job sedang berjalan
    When ROLE-RISK POST /api/v1/jobs/{jobId}/cancel
    Then job berhenti dengan status "cancelled"
    And instrumen yang sudah diproses sebelum cancel tidak di-persist
    And aud.audit_log berisi "EIR.BULK_RECOMPUTE_CANCELLED"

  Scenario: Partial failure — satu instrumen error tidak menghentikan batch
    Given 1000 instrumen, instrumen ke-500 punya cashflow corrupt
    When bulk re-compute berjalan
    Then job tetap lanjut ke instrumen 501–1000
    And result.errors berisi { instrumen_id: "...", error: "EIR_CASHFLOW_INVALID" }
    And job selesai dengan status "completed_with_errors"
    And total_errors = 1
```

---

## Open Questions (OQ) untuk P4-M5

| ID | Pertanyaan | Default jika belum dijawab | Owner |
|---|---|---|---|
| **OQ-M5-1** | `ecl.eir_amortization_schedule` dan `ecl.eir_reestimation_log` sudah ada di `000001` tapi TIDAK punya audit cols standar (`created_by`, `updated_by`, `updated_at`, `deleted_at`, `row_version`, `tenant_id`) dan precision `NUMERIC(12,8)` untuk EIR berbeda dengan DEC-016 (`NUMERIC(10,8)`). Migration 000024 perlu ADD COLUMN + ALTER COLUMN. Data-modeler harus assess impact pada semua indexes dan constraints. | Schema-fix migration 000024 dibutuhkan sebelum P4-M5 implementasi dapat dimulai. | `data-modeler` |
| **OQ-M5-2** | Deposito dengan cashflow tidak reguler (mis. depositoberjenjang, cashback) — apakah cashflow bisa di-auto-generate atau perlu input manual via API? | Auto-generate untuk deposito reguler. Manual input tersedia sebagai opsional parameter. | `ifrs9-compliance-reviewer` + FSD-APP-C §4.1 |
| **OQ-M5-3** | `flag_poci` belum ada di `mst.instrumen`. Apakah P4-M5 perlu menambahkan kolom ini, atau POCI path sepenuhnya defer ke post-Phase 4? | Defer: Story 1 Scenario POCI = stub returning `EIR_POCI_DEFERRED` 501. Add `flag_poci BOOLEAN DEFAULT FALSE` di migration 000024 sebagai forward-compatibility. | `tech-lead-orchestrator` + `ifrs9-compliance-reviewer` |
| **OQ-M5-4** | Skill `eir-newton-raphson/SKILL.md` referensi skema `ecl.amortisasi_schedule` dengan kolom `schedule_version` + `effective_from`/`effective_to` — berbeda dengan schema aktual `000001` yang menggunakan `recomputed_from_seq`. **Mana yang menjadi truth?** Schema aktual `000001` adalah yang ter-implement di DB. Skill file perlu diupdate atau migration perlu menambah `schedule_version` + `effective_from`/`effective_to` sebagai alias/additional columns? | Schema aktual `000001` (recomputed_from_seq) adalah source of truth. Skill file perlu diupdate untuk reflect implementasi aktual. Tidak tambah kolom duplikat. | `tech-lead-orchestrator` + `data-modeler` (update SKILL.md) |
| **OQ-M5-5** | `trx.amortisasi` (jurnal amortisasi) tidak punya audit cols standar di 000001. Apakah migration 000024 juga include schema-fix untuk `trx.amortisasi`? | Perlu, karena `trx.amortisasi` ditulis oleh EIR service sebagai bagian dari schedule execution. Include di 000024 atau buat migration terpisah 000025. | `data-modeler` |
| **OQ-M5-6** | EIR untuk FVOCI debt: apakah `amortisasi_p_d` (premium/diskonto amortisasi) dialokasikan ke P&L atau OCI? Ini mempengaruhi bagaimana `trx.amortisasi` di-classify di jurnal. Jika ke P&L seperti AC → schedule generation sama; jika OCI-classified → butuh flag di schedule row. | FVOCI debt: amortisasi premium/diskonto ke P&L (sama dengan AC, per IFRS 9 §5.7.11). | `ifrs9-compliance-reviewer` + FSD-APP-C §4.2 |

---

## Ringkasan Handoff

```
P4-M5-EIR-001..005 selesai →
  system-analyst: OpenAPI fragment eir-solver.yaml
    - POST /api/v1/eir/compute/{instrumen_id}
    - GET  /api/v1/eir/schedule/{instrumen_id}
    - POST /api/v1/eir/amendments
    - POST /api/v1/eir/amendments/{id}/review
    - POST /api/v1/eir/amendments/{id}/approve
    - POST /api/v1/eir/amendments/{id}/reject
    - POST /api/v1/eir/bulk-recompute
    + Go interface EIRService + AmendmentService
    + state machine amendment workflow_status

  data-modeler (PARALEL): migration 000024
    - ecl.eir_amortization_schedule schema-fix (audit cols, NUMERIC precision fix, no-UPDATE trigger)
    - ecl.eir_reestimation_log schema-fix (audit cols, SoD CHECK)
    - trx.amortisasi schema-fix (audit cols)
    - ADD flag_poci BOOLEAN DEFAULT FALSE ke mst.instrumen (OQ-M5-3)
    - ADD updated_at trigger, no-hard-delete trigger ke eir_amortization_schedule

  ifrs9-compliance-reviewer: BLOCKING gate sebelum merge PR P4-M5
    - Verify DEC-013 (Newton-Raphson param)
    - Verify DEC-016 (NUMERIC precision — tidak ada float64)
    - Verify immutability: rows lama hanya recomputed_from_seq yang diupdate, amounts tidak berubah
    - Verify OQ-M5-6 (FVOCI amortisasi direction)
```
