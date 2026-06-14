# P4-M1 — Staging Engine + SICR/Cure: User Stories

**Story Set ID**: P4-M1
**Modul**: APP-C — ECL Engine (Phase 4, Sprint 1)
**Status**: DRAFT — menunggu review `ifrs9-compliance-reviewer` (BLOCKING gate)
**Author**: business-analyst
**Tanggal**: 2026-06-11
**Linked FSD**: FSD-APP-C-ECL-EIR-v1.0.docx §3 (Staging, SICR, Cure)
**Linked BRD**: BRD §8.3 (ECL Requirements), RACI: ROLE-RISK (R), ROLE-ALCO (A), ROLE-AKUN-CTL (C), ROLE-AUDIT (I)
**Linked Decision Log**: DEC-010 (3-stage ECL), DEC-011 (SICR triggers), DEC-012 (cure 3 bulan), DEC-017 (workflow 4-eyes/6-eyes), DEC-018 (audit trail append-only)

**Handoff berikutnya**: `system-analyst` (OpenAPI contract + state machine), lalu `data-modeler` (migration 000022), lalu `ecl-eir-engineer` (implementasi domain logic)

---

## Konteks & Dependensi Phase 3

Phase 3 telah mendeliver:
- `mst.instrumen` — CRUD + workflow, termasuk `klasifikasi_psak71`, `counterparty_id`, `tipe_instrumen`
- `mst.counterparty` + `mst.rating_history_counterparty` — rating Pefindo dengan kolom `notch_change`, `sicr_triggered`, `default_triggered`, workflow `APPROVED`
- `mst.pd_pefindo` — kurva PD 12M + Lifetime per rating, ALCO-approved
- `ecl.stage_history` (schema 000001) — tabel append-only untuk riwayat stage transition, kolom `trigger_type`, `dpd` (INT), `stage_sebelum`, `stage_sesudah`, `status_approval`

**CRITICAL GAP teridentifikasi**: `mst.instrumen` tidak memiliki kolom `current_stage` (stage aktif instrumen saat ini). Stage hanya tersimpan di `ecl.stage_history` sebagai append-only log. Staging engine perlu menentukan stage aktif via query `MAX(tanggal_migrasi)` per instrumen. Tidak ada kolom DPD di `mst.instrumen` maupun tabel tracking DPD terpisah — lihat **GAP-DPD** di bawah.

---

## GAP-DPD: DPD Tracking Belum Ada di Phase 3

**Severity: BLOCKING untuk Story 2 (Auto-staging on DPD)**

Berdasarkan review schema Phase 3:
- `mst.instrumen` tidak memiliki kolom `dpd_current` atau `tanggal_jatuh_pembayaran_terakhir`
- `trx.*` tabel (penempatan, pembayaran) belum ada di Phase 3 (APP-B = Phase 5)
- Kolom `dpd INT` di `ecl.stage_history` adalah kolom input (nilai DPD saat staging dicatat), bukan tracking source

**Implikasi**: Story 2 (DPD recalc → Stage transition) tidak dapat berjalan penuh sampai APP-B (transaksi lifecycle) tersedia di Phase 5. Untuk Phase 4, DPD assessment perlu workaround.

**Opsi yang diajukan ke `data-modeler` + `system-analyst`**:
1. **Opsi A (direkomendasikan)**: Tambah tabel `trx.dpd_record` di migration 000022 sebagai placeholder — diisi manual oleh ROLE-AKUN atau via upload sampai APP-B live. Staging engine membaca dari sini.
2. **Opsi B**: DPD input manual pada saat manual override (Story 4) saja untuk Phase 4; auto-DPD staging diaktifkan di Phase 5 saat APP-B live.
3. **Opsi C**: Field `dpd_hari_ini` ditambah ke `mst.instrumen` sebagai denormalized cache, di-update oleh batch job Phase 4.

**Flag untuk orchestrator**: Konfirmasi pilihan opsi sebelum `ecl-eir-engineer` mulai implementasi Story 2. Bila Opsi A dipilih, `data-modeler` tambah tabel ke migration 000022.

---

## Resolved Open Question

**OQ-B (dari Phase 4 Plan §7)**: "3 bulan berturut-turut" untuk cure — apakah 3 `mst.periode_buku` consecutive (tipe `BULANAN`) atau 3 calendar months?

**Resolusi (default asumed, perlu konfirmasi FSD-APP-C §3.2)**:
> **3 `mst.periode_buku` BULANAN consecutive** (bukan calendar month). Alasan: BLIPS dioperasikan per periode buku; cure assessment berjalan setelah closing periode bulanan; consistent dengan ritme ECL calc run. Jika FSD-APP-C menentukan calendar month, ini perlu RFC.

**Flag**: Treasury atau Finance Controller HARUS konfirmasi sebelum P4-M1 merge ke `develop`.

---

## Notch Scale Pefindo (DEC-011 clarification)

Urutan rating Pefindo dari terbaik ke terburuk (ascending risk):
`idAAA → idAA+ → idAA → idAA- → idA+ → idA → idA- → idBBB+ → idBBB → idBBB- → idBB+ → idBB → idBB- → idB+ → idB → idB- → idCCC → idD`

**Investment Grade (IG)**: `idAAA` s.d. `idBBB-` (inklusif).
**Non-IG**: `idBB+` ke bawah s.d. `idD`.
**Default**: `idD` (atau `idSD` selective default jika Pefindo menggunakannya).

Satu notch = satu langkah dalam skala di atas. Downgrade ≥ 2 notch = bergerak 2 atau lebih langkah ke arah risiko lebih tinggi sejak tanggal inisiasi instrumen.

**Flag**: ecl-eir-engineer harus verifikasi skala lengkap ini ke Pefindo Annual Default Study 2007–2025 sebelum implementasi. Jika Pefindo punya modifier yang berbeda, ikuti studi tersebut.

---

## Story APP-C-STG-001 — Auto-Staging on Rating Import (SICR Evaluation)

**Actor**: System (Asynq worker) — triggered otomatis; ROLE-RISK dapat juga trigger manual
**Trigger**: Rating baru di `mst.rating_history_counterparty` di-approve (workflow_status berubah ke `APPROVED`), atau Pefindo feed batch di-import dan di-commit
**Goal**: Setiap instrumen yang counterparty-nya memiliki rating baru dievaluasi terhadap 3 SICR trigger; jika terpenuhi, stage instrument berubah dari Stage 1 → Stage 2 dan dicatat di `ecl.stage_history`

### Pre-conditions
1. Instrumen `status = 'AKTIF'`, `klasifikasi_psak71 IN ('AC', 'FVOCI')` (instrumen FVTPL dan FVOCI_ELECTION di-skip, per OQ-G plan Phase 4)
2. Rating baru di `mst.rating_history_counterparty` sudah `workflow_status = 'APPROVED'` dan `tanggal_berlaku <= today`
3. Rating inisiasi instrumen (rating saat `tanggal_penempatan`) tersedia di `mst.rating_history_counterparty` untuk perhitungan delta notch
4. `mst.pd_pefindo` memiliki entri aktif untuk rating yang sedang dievaluasi

### Data References
| Tabel | Akses | Kolom Utama |
|---|---|---|
| `mst.instrumen` | READ | `id`, `counterparty_id`, `klasifikasi_psak71`, `status`, `tanggal_penempatan` |
| `mst.rating_history_counterparty` | READ | `counterparty_id`, `rating_pefindo`, `notch_change`, `sicr_triggered`, `default_triggered`, `tanggal_berlaku`, `workflow_status` |
| `ecl.stage_history` | READ + INSERT (append-only) | `instrumen_id`, `stage_sebelum`, `stage_sesudah`, `trigger_type`, `detail_trigger`, `rating_saat_migrasi`, `tanggal_migrasi`, `status_approval` |

### Permissions Needed
| Actor | Permission | Catatan |
|---|---|---|
| System worker | `instrumen.read`, `ecl_staging.write` | Service account, bukan human role |
| ROLE-RISK (manual trigger) | `instrumen.read`, `ecl_staging.evaluate` | Untuk trigger manual evaluation per instrumen |
| ROLE-RISK, ROLE-AUDIT | `ecl_staging.read` | Untuk membaca hasil evaluasi |

### Audit Events
| Action | Trigger | Entity |
|---|---|---|
| `ECL_STAGING.EVALUATE` | Setiap kali engine dievaluasi (otomatis atau manual) | `ecl.stage_history` |
| `ECL_STAGING.SICR_TRIGGERED` | Saat SICR terpenuhi dan stage berubah | `ecl.stage_history` |
| `ECL_STAGING.NO_CHANGE` | Saat evaluasi tidak menghasilkan perubahan stage | `aud.audit_log` (bukan stage_history — tidak ada row baru) |

### Acceptance Criteria

```gherkin
Feature: Auto-staging pada rating import — evaluasi SICR trigger

  Background:
    Given instrumen INST-0001 klasifikasi_psak71 = 'AC', status = 'AKTIF'
    And counterparty instrumen adalah CP-BANK-01
    And rating inisiasi CP-BANK-01 pada tanggal_penempatan INST-0001 adalah 'idAA'
    And ecl.stage_history terakhir untuk INST-0001 adalah Stage 1

  # ─── HAPPY PATH 1: Rating downgrade ≥ 2 notch ───────────────────────────

  Scenario: SICR trigger — rating turun ≥ 2 notch dari rating inisiasi → Stage 1 ke Stage 2
    Given rating baru CP-BANK-01 disetujui: rating_pefindo = 'idBBB+', tanggal_berlaku = today
    And delta dari 'idAA' ke 'idBBB+' adalah 3 notch downgrade (≥ 2 notch threshold DEC-011)
    When staging engine mengevaluasi INST-0001 (triggered by rating approval event)
    Then engine menginsert row baru ke ecl.stage_history:
      | instrumen_id     | INST-0001         |
      | stage_sebelum    | STAGE_1           |
      | stage_sesudah    | STAGE_2           |
      | trigger_type     | RATING_DOWNGRADE  |
      | detail_trigger   | "Rating CP-BANK-01 turun dari idAA ke idBBB+ (3 notch, >= 2 notch threshold)" |
      | rating_saat_migrasi | 'idBBB+'      |
      | tanggal_migrasi  | today             |
      | status_approval  | AUTO              |
    And row ini immutable (tidak bisa diupdate atau didelete)
    And audit log mencatat ECL_STAGING.SICR_TRIGGERED dengan entity_id = stage_history.id
    And engine melaporkan 1 instrumen di-transisi ke Stage 2

  # ─── HAPPY PATH 2: Rating IG → non-IG ────────────────────────────────────

  Scenario: SICR trigger — rating berubah dari Investment Grade ke non-IG → Stage 1 ke Stage 2
    Given rating baru CP-BANK-01 disetujui: rating_pefindo = 'idBB+', tanggal_berlaku = today
    And rating sebelumnya adalah 'idBBB-' (IG, threshold terbawah)
    And delta notch = 1 (kurang dari 2 notch threshold)
    But perubahan dari IG ke non-IG memenuhi trigger kedua DEC-011
    When staging engine mengevaluasi INST-0001
    Then engine menginsert row ke ecl.stage_history dengan trigger_type = 'IG_TO_NON_IG'
    And stage_sesudah = 'STAGE_2'
    And detail_trigger mencantumkan: "Rating berubah dari IG (idBBB-) ke non-IG (idBB+)"

  # ─── HAPPY PATH 3: Rating menuju default ──────────────────────────────────

  Scenario: Rating default — instrumen langsung Stage 3
    Given rating baru CP-BANK-01 disetujui: rating_pefindo = 'idD', default_triggered = TRUE
    When staging engine mengevaluasi INST-0001
    Then engine menginsert row ke ecl.stage_history dengan:
      | stage_sesudah  | STAGE_3            |
      | trigger_type   | RATING_DEFAULT     |
    And PD untuk INST-0001 pada calc run berikutnya = 1.0 (per DEC-010 Stage 3)
    And audit log mencatat ECL_STAGING.SICR_TRIGGERED

  # ─── EDGE CASE: Rating affirmed, tidak ada perubahan ─────────────────────

  Scenario: Rating affirmed — tidak ada SICR trigger → stage tetap
    Given rating baru CP-BANK-01 disetujui: action_type = 'AFFIRMED', rating_pefindo = 'idAA'
    And delta notch = 0
    When staging engine mengevaluasi INST-0001
    Then tidak ada row baru di ecl.stage_history
    And audit log mencatat ECL_STAGING.NO_CHANGE dengan detail "rating affirmed, no SICR trigger"
    And engine melaporkan 0 instrumen di-transisi

  # ─── EDGE CASE: Instrumen FVTPL di-skip ──────────────────────────────────

  Scenario: Instrumen FVTPL di-skip dari evaluasi SICR
    Given instrumen INST-0002 klasifikasi_psak71 = 'FVTPL', counterparty = CP-BANK-01
    And rating baru CP-BANK-01 memenuhi SICR trigger (downgrade 3 notch)
    When staging engine berjalan untuk semua instrumen CP-BANK-01
    Then INST-0002 tidak dievaluasi (di-skip)
    And tidak ada row stage_history untuk INST-0002
    And log engine mencatat "INST-0002 skipped: FVTPL tidak memerlukan ECL staging"

  # ─── ERROR CASE: Rating inisiasi tidak ditemukan ─────────────────────────

  Scenario: Rating inisiasi instrumen tidak tersedia — evaluasi gagal dengan warning
    Given instrumen INST-0003 tanggal_penempatan = '2020-01-15'
    And tidak ada row di mst.rating_history_counterparty untuk CP-BANK-01 pada atau sebelum '2020-01-15'
    When staging engine mencoba mengevaluasi INST-0003
    Then engine tidak menginsert row stage_history
    And engine mencatat warning ke sys.job.result_jsonb: "INST-0003: rating inisiasi tidak ditemukan, evaluasi dilewati"
    And engine tetap melanjutkan evaluasi instrumen lain (tidak abort batch)
    And ROLE-RISK mendapat notifikasi: "1 instrumen tidak dapat dievaluasi — data rating inisiasi tidak lengkap"

  # ─── ERROR CASE: Stage_history append-only — tidak bisa dimodifikasi ─────

  Scenario: Upaya modifikasi langsung stage_history ditolak
    Given row ecl.stage_history sudah ada dengan status_approval = 'AUTO'
    When sistem atau user mencoba UPDATE atau DELETE row tersebut via API atau DB langsung
    Then sistem mengembalikan error FORBIDDEN (via DB trigger tg_ecl_stage_history_no_delete)
    And row tetap tidak berubah
    And audit log mencatat upaya tersebut
```

### Open Questions — Story 1
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-STG-1a | Apakah delta notch dihitung dari **rating inisiasi instrumen** (fixed baseline) atau dari **rating periode sebelumnya** (rolling)? | Dari rating inisiasi (tanggal_penempatan), per IFRS9 §5.5.11 "since initial recognition". Flag ke ifrs9-compliance-reviewer untuk konfirmasi. |
| OQ-STG-1b | Jika satu counterparty memiliki banyak instrumen, apakah evaluasi SICR berjalan paralel per instrumen atau sekuensial? | Paralel (satu Asynq sub-task per instrumen), dengan batas concurrency 50 goroutine. |

---

## Story APP-C-STG-002 — Auto-Staging on DPD Recalculation

**Actor**: Asynq daily job (system worker, bukan human actor)
**Trigger**: DPD recalculation job berjalan harian (direncanakan di Phase 4; input data DPD tersedia — lihat GAP-DPD di atas)
**Goal**: Instrumen dengan DPD ≥ 30 hari transition Stage 1 → 2; instrumen dengan DPD ≥ 90 hari transition ke Stage 3 (proxy default); dicatat di `ecl.stage_history`

**DEPENDENCY FLAG**: Story ini fully operational hanya setelah GAP-DPD diselesaikan. Lihat opsi A/B/C di bagian GAP-DPD. Bila Opsi B dipilih, Story 2 di-scope-out dari Phase 4 dan dipindah ke Phase 5 backlog. Bila Opsi A dipilih, story ini valid untuk Phase 4 dengan sumber data `trx.dpd_record` (tabel baru).

### Pre-conditions
1. Sumber DPD tersedia: tabel `trx.dpd_record` (Opsi A) atau `mst.instrumen.dpd_current` (Opsi C) sudah ada dan diisi
2. Instrumen `status = 'AKTIF'`, `klasifikasi_psak71 IN ('AC', 'FVOCI')`
3. Job berjalan setelah pukul 00:00 WIB setiap hari kerja (dapat dikonfigurasi via `sys.config`)

### Data References
| Tabel | Akses | Kolom Utama |
|---|---|---|
| `mst.instrumen` | READ | `id`, `klasifikasi_psak71`, `status`, `counterparty_id` |
| `trx.dpd_record` *(baru, Opsi A)* | READ | `instrumen_id`, `dpd_hari`, `tanggal_hitung`, `sumber` |
| `ecl.stage_history` | READ + INSERT (append-only) | `instrumen_id`, `stage_sebelum`, `stage_sesudah`, `trigger_type`, `dpd`, `tanggal_migrasi` |

### Permissions Needed
| Actor | Permission |
|---|---|
| System worker (Asynq) | `instrumen.read`, `ecl_staging.write`, `dpd_record.read` |
| ROLE-RISK (view job result) | `ecl_staging.read`, `sys_job.read` |

### Audit Events
| Action | Trigger |
|---|---|
| `ECL_STAGING.DPD_SICR_TRIGGERED` | Saat DPD ≥ 30 menyebabkan Stage 1 → Stage 2 |
| `ECL_STAGING.DPD_DEFAULT_TRIGGERED` | Saat DPD ≥ 90 menyebabkan masuk Stage 3 |
| `ECL_STAGING.DPD_NO_CHANGE` | Saat DPD < 30 (tidak ada transisi) |

### Acceptance Criteria

```gherkin
Feature: Auto-staging berdasarkan DPD harian

  Background:
    Given Asynq DPD staging job berjalan pada pukul 01:00 WIB
    And instrumen INST-0010 klasifikasi_psak71 = 'AC', status = 'AKTIF'
    And ecl.stage_history terakhir untuk INST-0010 adalah Stage 1

  # ─── HAPPY PATH 1: DPD ≥ 30 → Stage 2 ───────────────────────────────────

  Scenario: DPD ≥ 30 hari → SICR trigger Stage 1 ke Stage 2
    Given trx.dpd_record untuk INST-0010 hari ini: dpd_hari = 35
    When DPD staging job berjalan
    Then engine menginsert row ke ecl.stage_history:
      | stage_sebelum  | STAGE_1      |
      | stage_sesudah  | STAGE_2      |
      | trigger_type   | DPD_GTE_30   |
      | dpd            | 35           |
      | tanggal_migrasi| today        |
      | status_approval| AUTO         |
    And audit log: ECL_STAGING.DPD_SICR_TRIGGERED

  # ─── HAPPY PATH 2: DPD ≥ 90 → Stage 3 (default proxy) ──────────────────

  Scenario: DPD ≥ 90 hari → Stage 3 (default proxy) langsung dari Stage 1
    Given trx.dpd_record untuk INST-0010 hari ini: dpd_hari = 92
    And ecl.stage_history terakhir untuk INST-0010 adalah Stage 1
    When DPD staging job berjalan
    Then engine menginsert 2 row ke ecl.stage_history secara atomik:
      Row 1: stage_sebelum=STAGE_1, stage_sesudah=STAGE_2, trigger_type=DPD_GTE_30, dpd=92
      Row 2: stage_sebelum=STAGE_2, stage_sesudah=STAGE_3, trigger_type=DPD_GTE_90, dpd=92
    And kedua row diinsert dalam satu transaksi database
    And audit log: ECL_STAGING.DPD_DEFAULT_TRIGGERED (2 entries)

  # ─── HAPPY PATH 3: DPD ≥ 90 dari Stage 2 ────────────────────────────────

  Scenario: Instrumen sudah Stage 2, DPD meningkat ke ≥ 90 → Stage 3
    Given ecl.stage_history terakhir untuk INST-0011 adalah Stage 2 (trigger sebelumnya: rating downgrade)
    And trx.dpd_record untuk INST-0011 hari ini: dpd_hari = 95
    When DPD staging job berjalan
    Then engine menginsert 1 row ke ecl.stage_history:
      | stage_sebelum  | STAGE_2      |
      | stage_sesudah  | STAGE_3      |
      | trigger_type   | DPD_GTE_90   |
      | dpd            | 95           |

  # ─── EDGE CASE: DPD < 30 — tidak ada transisi ────────────────────────────

  Scenario: DPD < 30 hari — tidak ada perubahan stage
    Given trx.dpd_record untuk INST-0010 hari ini: dpd_hari = 15
    When DPD staging job berjalan
    Then tidak ada row baru di ecl.stage_history untuk INST-0010
    And audit log: ECL_STAGING.DPD_NO_CHANGE

  # ─── EDGE CASE: DPD turun setelah sebelumnya ≥ 30, tapi stage tidak auto-cure ──

  Scenario: DPD turun di bawah 30 — stage TIDAK otomatis turun (cure assessment terpisah)
    Given ecl.stage_history terakhir INST-0010: Stage 2 (trigger: DPD_GTE_30, 3 hari lalu)
    And trx.dpd_record hari ini: dpd_hari = 10 (sudah dibayar sebagian)
    When DPD staging job berjalan
    Then tidak ada row baru di ecl.stage_history
    And stage INST-0010 tetap Stage 2
    And audit log: ECL_STAGING.DPD_NO_CHANGE dengan note "DPD berkurang tapi cure assessment menunggu 3 periode bulanan"

  # ─── ERROR CASE: Sumber DPD tidak tersedia ───────────────────────────────

  Scenario: Data DPD tidak tersedia untuk instrumen — job mencatat warning
    Given tidak ada row di trx.dpd_record untuk instrumen INST-0012 pada tanggal hari ini
    When DPD staging job berjalan
    Then INST-0012 di-skip dari evaluasi DPD
    And sys.job.result_jsonb mencatat warning: "INST-0012: DPD data tidak tersedia, evaluasi dilewati"
    And job tidak gagal total (partial failure allowed, continue lainnya)
    And ROLE-RISK mendapat summary: "N instrumen tidak dievaluasi karena DPD data tidak lengkap"
```

### Open Questions — Story 2
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-STG-2a | Apakah DPD ≥ 90 selalu → Stage 3 langsung, atau Stage 3 hanya via credit committee decision? | Proxy: DPD ≥ 90 → Stage 3 otomatis (IFRS9 §B5.5.37). Perlu konfirmasi Treasury + ifrs9-compliance-reviewer. |
| OQ-STG-2b | DPD dihitung dari tanggal berapa? (tanggal jatuh tempo cicilan, tanggal nilai buku, tanggal kontrak?) | Dari `tanggal_jatuh_tempo` instrumen untuk instrumen bullet; untuk cicilan butuh schedule dari `ecl.eir_amortization_schedule`. Flag ke system-analyst. |
| OQ-STG-2c | Sumber data DPD Phase 4: Opsi A, B, atau C (lihat GAP-DPD)? | Perlu keputusan orchestrator. Default: Opsi A (tabel baru `trx.dpd_record`). |

---

## Story APP-C-STG-003 — Cure Transition (Stage 2 → Stage 1)

**Actor**: Asynq monthly job (system worker); berjalan setelah `mst.periode_buku` soft-close untuk bulan berjalan
**Trigger**: Setelah soft-close periode buku bulanan, job cure assessment berjalan untuk semua instrumen Stage 2
**Goal**: Instrumen Stage 2 yang tidak memiliki SICR trigger aktif selama 3 `periode_buku BULANAN` consecutive di-transisi kembali ke Stage 1; history riwayat downgrade tetap tersimpan immutable di `ecl.stage_history`

### Pre-conditions
1. Periode buku bulan berjalan sudah `status_periode = 'SOFT_CLOSED'` atau `'HARD_CLOSED'`
2. Instrumen dalam `stage_sesudah = 'STAGE_2'` pada stage_history terbaru
3. 3 periode buku bulanan sebelumnya sudah `SOFT_CLOSED` atau `HARD_CLOSED` (bukan masih `AKTIF` atau `DRAFT`)
4. `mst.pd_pefindo` aktif tersedia untuk menentukan PD baru post-cure

### Definisi "Cure Criteria Terpenuhi" (per periode buku)
Satu periode buku dianggap memenuhi cure criteria jika dalam periode tersebut:
- Tidak ada SICR trigger baru (tidak ada row `stage_history` dengan `stage_sesudah = 'STAGE_2'` atau `'STAGE_3'` dan `tanggal_migrasi` jatuh dalam rentang periode itu)
- DPD instrumen di bawah 30 hari selama seluruh periode (tidak ada `dpd_hari >= 30` dalam periode)
- Rating tidak memenuhi SICR threshold (rating drop < 2 notch dari inisiasi DAN tidak IG→non-IG)

### Data References
| Tabel | Akses | Kolom Utama |
|---|---|---|
| `mst.instrumen` | READ | `id`, `klasifikasi_psak71`, `status` |
| `mst.periode_buku` | READ | `id`, `tipe_periode`, `status_periode`, `tanggal_awal`, `tanggal_akhir` |
| `ecl.stage_history` | READ + INSERT (append-only) | `instrumen_id`, `stage_sebelum`, `stage_sesudah`, `trigger_type`, `tanggal_migrasi` |
| `trx.dpd_record` | READ | `instrumen_id`, `dpd_hari`, `tanggal_hitung` |
| `mst.rating_history_counterparty` | READ | `counterparty_id`, `sicr_triggered`, `tanggal_berlaku` |

### Permissions Needed
| Actor | Permission |
|---|---|
| System worker (Asynq) | `instrumen.read`, `ecl_staging.write`, `periode_buku.read` |
| ROLE-RISK (view) | `ecl_staging.read` |

### Audit Events
| Action | Trigger |
|---|---|
| `ECL_STAGING.CURE_TRANSITION` | Instrumen berhasil cure dari Stage 2 → Stage 1 |
| `ECL_STAGING.CURE_INELIGIBLE` | Instrumen gagal cure criteria (masih ada SICR) |
| `ECL_STAGING.CURE_ASSESSMENT_RUN` | Job selesai berjalan (summary: N cured, M ineligible) |

### Acceptance Criteria

```gherkin
Feature: Cure transition Stage 2 ke Stage 1 setelah 3 periode buku consecutive

  Background:
    Given instrumen INST-0020 klasifikasi_psak71 = 'AC', status = 'AKTIF'
    And ecl.stage_history terbaru: stage_sesudah = 'STAGE_2' pada 2025-12-15 (trigger: RATING_DOWNGRADE)
    And mst.periode_buku BULANAN tersedia: Jan-2026, Feb-2026, Mar-2026 (semua SOFT_CLOSED)
    And periode buku aktif sekarang: Apr-2026 (baru saja SOFT_CLOSED, trigger cure job)

  # ─── HAPPY PATH: 3 periode consecutive tanpa SICR → Cure ke Stage 1 ─────

  Scenario: Instrumen berhasil cure setelah 3 periode buku consecutive tanpa SICR
    Given dalam periode Jan-2026: tidak ada SICR trigger baru, DPD < 30, rating stable
    And dalam periode Feb-2026: tidak ada SICR trigger baru, DPD < 30, rating stable
    And dalam periode Mar-2026: tidak ada SICR trigger baru, DPD < 30, rating stable
    When monthly cure job berjalan setelah soft-close Apr-2026
    Then engine menginsert row ke ecl.stage_history:
      | instrumen_id   | INST-0020              |
      | stage_sebelum  | STAGE_2                |
      | stage_sesudah  | STAGE_1                |
      | trigger_type   | CURE_3_PERIODE_BULANAN |
      | detail_trigger | "Cure criteria terpenuhi selama 3 periode: Jan-2026, Feb-2026, Mar-2026. Tidak ada SICR trigger, DPD < 30 sepanjang periode." |
      | tanggal_migrasi| tanggal soft-close Apr-2026 |
      | status_approval| AUTO                   |
    And row downgrade sebelumnya (Stage 2) tetap ada di stage_history (tidak dihapus)
    And audit log: ECL_STAGING.CURE_TRANSITION
    And engine melaporkan 1 instrumen berhasil di-cure

  # ─── EDGE CASE: Baru 2 periode consecutive — belum cure ──────────────────

  Scenario: Baru 2 periode consecutive tanpa SICR — belum memenuhi threshold
    Given dalam periode Jan-2026: tidak ada SICR trigger, DPD < 30
    And dalam periode Feb-2026: tidak ada SICR trigger, DPD < 30
    And Mar-2026 baru saja SOFT_CLOSED (periode ke-3 baru dimulai)
    When monthly cure job berjalan setelah soft-close Mar-2026
    Then tidak ada row baru di ecl.stage_history untuk INST-0020
    And audit log: ECL_STAGING.CURE_INELIGIBLE dengan detail "baru 2 dari 3 periode consecutive terpenuhi"
    And stage INST-0020 tetap Stage 2

  # ─── EDGE CASE: Ada SICR baru di tengah periode → reset counter ──────────

  Scenario: SICR baru muncul di bulan ke-2 — counter cure di-reset ke 0
    Given dalam periode Jan-2026: tidak ada SICR trigger (periode 1 valid)
    And dalam periode Feb-2026: SICR trigger baru muncul (rating downgrade lagi)
    And ecl.stage_history mencatat SICR baru pada 2026-02-10
    And dalam periode Mar-2026: tidak ada SICR trigger (periode 1 valid setelah reset)
    When monthly cure job berjalan setelah soft-close Apr-2026
    Then cure assessment menemukan counter baru dimulai dari Feb-2026 (SICR di Feb reset counter)
    And periode valid untuk cure hanya: Mar-2026 (1 periode, bukan 3)
    Then tidak ada row cure di ecl.stage_history
    And audit log: ECL_STAGING.CURE_INELIGIBLE dengan detail "counter di-reset pada 2026-02-10 karena SICR baru"

  # ─── EDGE CASE: Instrumen Stage 3 tidak di-cure via job ini ──────────────

  Scenario: Instrumen Stage 3 tidak termasuk dalam cure assessment Stage 2 → Stage 1
    Given instrumen INST-0030 stage_history terbaru: Stage 3 (trigger: DPD_GTE_90)
    When monthly cure job berjalan
    Then INST-0030 di-skip
    And tidak ada row stage_history untuk INST-0030
    And log engine: "INST-0030 skipped: Stage 3 cure tidak di-scope job ini (butuh manual override + ALCO)"

  # ─── ERROR CASE: periode_buku belum CLOSED saat job jalan ────────────────

  Scenario: Job berjalan tapi periode buku belum soft-closed — job abort
    Given periode buku Mar-2026 masih dalam status 'AKTIF' (belum soft-closed)
    When monthly cure job dipanggil untuk assessment bulan Mar-2026
    Then job abort dengan error: "Periode buku Mar-2026 belum di-close. Cure assessment tidak bisa dijalankan."
    And sys.job.status = 'FAILED'
    And tidak ada perubahan stage_history
    And ROLE-RISK mendapat notifikasi: "Cure job gagal: periode buku belum ditutup"
```

### Open Questions — Story 3
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-STG-3a | Cure dari Stage 3 ke Stage 2/1 — apakah bisa otomatis atau hanya via manual override? | Hanya manual override (Story 4). Stage 3 cure butuh credit committee decision. Flag ke ifrs9-compliance-reviewer. |
| OQ-STG-3b | "3 periode consecutive" dihitung dari tanggal SICR trigger atau dari tanggal awal periode berikutnya setelah trigger? | Dari tanggal awal periode buku PERTAMA setelah tanggal SICR trigger. Misalnya SICR terjadi 15 Des 2025 (di tengah Periode Des-2025), maka periode pertama yang bisa dihitung adalah Jan-2026. |
| OQ-STG-3c | Apakah DPD harus 0 selama 3 periode, atau cukup < 30? | Cukup DPD < 30 per DEC-011 threshold. |

---

## Story APP-C-STG-004 — Manual Stage Override (6-Eyes Workflow)

**Actor**: ROLE-RISK (proposer/maker), ROLE-RISK kedua sebagai reviewer, ROLE-ALCO (final approver)
**Trigger**: ROLE-RISK mengidentifikasi edge case yang tidak tertangani secara otomatis (mis. management overlay, instrumen restrukturisasi, Stage 3 cure manual)
**Goal**: Stage instrumen dapat di-override secara manual dengan justifikasi, melalui 6-eyes workflow, dengan audit trail lengkap; override memiliki masa berlaku (berlaku sampai akhir periode buku berjalan, kecuali diperpanjang secara eksplisit)

### Pre-conditions
1. User ter-autentikasi sebagai ROLE-RISK dengan permission `ecl_staging.override.propose`
2. Instrumen `status = 'AKTIF'`, sudah memiliki minimal 1 row di `ecl.stage_history`
3. User yang mengajukan (maker) harus berbeda dari reviewer dan approver (SoD, DEC-017)
4. Approver terakhir adalah ROLE-ALCO dengan MFA aktif (DEC-026, step-up MFA jika dibutuhkan)

### Data References
| Tabel | Akses | Kolom Utama |
|---|---|---|
| `ecl.stage_history` | READ + INSERT (append-only) | `instrumen_id`, `stage_sebelum`, `stage_sesudah`, `trigger_type`, `detail_trigger`, `status_approval`, `user_approver_id`, `dokumen_pendukung_id` |
| `mst.instrumen` | READ | `id`, `kode_instrumen` |
| `mst.periode_buku` | READ | `id`, `tanggal_akhir` (untuk menentukan expiry override) |
| `doc.upload` | READ | `id` (referensi dokumen pendukung wajib) |

### Permissions Needed
| Actor | Permission | MFA |
|---|---|---|
| ROLE-RISK (maker override) | `ecl_staging.override.propose` | Tidak wajib (kecuali Treasury Manager senior) |
| ROLE-RISK (reviewer) | `ecl_staging.override.review` | Tidak wajib |
| ROLE-ALCO (approver) | `ecl_staging.override.approve` | WAJIB (DEC-026) |
| ROLE-AUDIT | `ecl_staging.read` | Tidak wajib |

### Audit Events
| Action | Trigger |
|---|---|
| `ECL_STAGING.OVERRIDE_PROPOSED` | Maker mengajukan override |
| `ECL_STAGING.OVERRIDE_REVIEWED` | Reviewer sign-off |
| `ECL_STAGING.OVERRIDE_APPROVED` | ALCO final approve; stage berubah; signature_hash dicatat |
| `ECL_STAGING.OVERRIDE_REJECTED` | Reviewer atau ALCO menolak |
| `ECL_STAGING.OVERRIDE_EXPIRED` | Sistem menandai override kadaluarsa saat periode berakhir tanpa re-confirm |

### Acceptance Criteria

```gherkin
Feature: Manual stage override dengan 6-eyes workflow dan audit trail

  Background:
    Given instrumen INST-0040 stage_history terbaru: Stage 2
    And ROLE-RISK user A (maker), ROLE-RISK user B (reviewer), ROLE-ALCO user C (approver)
    And ketiga user adalah user yang berbeda (SoD)
    And periode buku aktif: Mei-2026 (tanggal_akhir = 2026-05-31)

  # ─── HAPPY PATH: Override Stage 2 → Stage 1 (management overlay) ────────

  Scenario: Override stage berhasil melalui 6-eyes workflow lengkap
    Given user A (ROLE-RISK) mengajukan override:
      | instrumen_id      | INST-0040                            |
      | stage_target      | STAGE_1                              |
      | alasan            | "Restrukturisasi berhasil diselesaikan, collateral baru diterima" |
      | dokumen_pendukung | upload SK Restrukturisasi (doc.upload.id) |
    When user A submit proposal (POST /ecl/staging/override)
    Then sistem membuat staging_override record dengan status 'PENDING_REVIEW'
    And audit log: ECL_STAGING.OVERRIDE_PROPOSED
    And notifikasi ke ROLE-RISK users lain: "Ada proposal override staging menunggu review"
    When user B (ROLE-RISK reviewer) menyetujui review
    Then status berubah ke 'PENDING_APPROVAL'
    And reviewer_id ≠ maker_id (SoD verified server-side)
    And audit log: ECL_STAGING.OVERRIDE_REVIEWED
    When user C (ROLE-ALCO approver) menyetujui dengan MFA aktif
    Then sistem menginsert row ke ecl.stage_history:
      | stage_sebelum    | STAGE_2                |
      | stage_sesudah    | STAGE_1                |
      | trigger_type     | MANUAL_OVERRIDE        |
      | detail_trigger   | "Override disetujui ALCO: Restrukturisasi berhasil..." |
      | status_approval  | APPROVED               |
      | user_approver_id | user C UUID            |
      | dokumen_pendukung_id | doc_id             |
    And row ini mencatat expiry: berlaku hingga akhir periode Mei-2026 (2026-05-31)
    And audit log: ECL_STAGING.OVERRIDE_APPROVED dengan signature_hash
    And toast sukses ke user A: "Override stage INST-0040 disetujui oleh ALCO. Berlaku hingga 2026-05-31."

  # ─── EDGE CASE: Override ditolak ALCO — stage tetap ─────────────────────

  Scenario: ALCO menolak override — stage tetap di Stage 2
    Given proposal override INST-0040 sudah melewati reviewer (PENDING_APPROVAL)
    When user C (ROLE-ALCO) menolak dengan komentar: "Dokumen belum cukup, perlu legal opinion"
    Then status proposal: REJECTED
    And tidak ada row baru di ecl.stage_history (stage tetap Stage 2)
    And audit log: ECL_STAGING.OVERRIDE_REJECTED dengan komentar penolakan
    And notifikasi ke user A: "Override INST-0040 ditolak ALCO: Dokumen belum cukup, perlu legal opinion"

  # ─── EDGE CASE: Override kadaluarsa akhir periode — tidak diperpanjang ──

  Scenario: Override berhasil tapi tidak diperpanjang di periode berikutnya
    Given override Stage 1 untuk INST-0040 berlaku hingga 2026-05-31
    And engine DPD/rating auto-staging tidak mendeteksi SICR baru dalam periode Jun-2026
    When periode Jun-2026 soft-close
    And ROLE-RISK tidak mengajukan re-confirm override untuk Jun-2026
    Then engine menandai override sebagai EXPIRED
    And menginsert row ke ecl.stage_history:
      | trigger_type   | OVERRIDE_EXPIRED                |
      | stage_sebelum  | STAGE_1                         |
      | stage_sesudah  | STAGE_1 (tidak berubah otomatis)|
      Dan catatan: "Override expired. Stage tetap Stage 1 karena tidak ada SICR trigger. Butuh re-assessment."
    And notifikasi ke ROLE-RISK: "Override stage INST-0040 kadaluarsa. Konfirmasi diperlukan untuk periode berikutnya."

  # ─── ERROR CASE: SoD violation — reviewer sama dengan maker ──────────────

  Scenario: Reviewer mencoba me-review proposal yang dia ajukan sendiri
    Given user A mengajukan proposal override (maker)
    When user A mencoba me-review proposal sendiri (PENDING_REVIEW → review)
    Then sistem mengembalikan HTTP 403 SOD_VIOLATION
    And pesan: "Anda tidak bisa menjadi reviewer untuk proposal yang Anda ajukan sendiri."
    And status proposal tetap PENDING_REVIEW
    And audit log mencatat upaya SoD violation

  # ─── ERROR CASE: Override Stage 3 tanpa dokumen pendukung ────────────────

  Scenario: Proposal override untuk Stage 3 wajib melampirkan dokumen pendukung
    Given INST-0050 dalam Stage 3
    When user A (ROLE-RISK) mengajukan override ke Stage 2 TANPA upload dokumen pendukung
    Then sistem mengembalikan HTTP 400 VALIDATION_FAILED
    And detail: "dokumen_pendukung_id: wajib untuk override instrumen Stage 3"
    And tidak ada proposal yang dibuat
```

### Open Questions — Story 4
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-STG-4a | Apakah override Stage 3 → Stage 2 diizinkan via jalur yang sama, atau butuh proses terpisah (mis. credit committee)? | Override Stage 3 diizinkan tapi butuh dokumen pendukung wajib + catatan: ekspor ke credit committee. Konfirmasi ke ROLE-KOMITE. |
| OQ-STG-4b | Jika override kadaluarsa dan tidak ada SICR trigger aktif, apakah stage otomatis kembali ke evaluasi terbaru (berpotensi Stage 2 lagi), atau tetap di stage override sampai ada trigger? | Stage tetap di posisi terakhir yang valid. Engine harus re-evaluate saat periode berikutnya. |
| OQ-STG-4c | Berapa lama masa berlaku override — per periode buku berjalan saja, atau bisa multi-periode? | Default: 1 periode buku berjalan. Bisa diperpanjang via re-confirm (proposal baru) per periode. Konfirmasi BRD §8.3. |

---

## Story APP-C-STG-005 — Read Staging History per Instrumen

**Actor**: ROLE-RISK (review & monitoring), ROLE-AUDIT (audit & export)
**Trigger**: User membuka halaman detail instrumen atau halaman staging history langsung
**Goal**: User dapat melihat timeline lengkap perubahan stage untuk satu instrumen, dengan drilldown ke detail trigger (rating event, DPD nilai, atau override reason), dilengkapi sort + filter + export sesuai UX rule §1

### Pre-conditions
1. User ter-autentikasi dengan permission `ecl_staging.read`
2. Instrumen ada dan `deleted_at IS NULL` (atau ROLE-AUDIT dengan `?include_deleted=true`)
3. `ecl.stage_history` untuk instrumen memiliki minimal 1 row

### Data References
| Tabel | Akses | Kolom yang Di-expose ke UI |
|---|---|---|
| `ecl.stage_history` | READ | `id`, `stage_history_id_kode`, `tanggal_migrasi`, `stage_sebelum`, `stage_sesudah`, `trigger_type`, `detail_trigger`, `rating_saat_migrasi`, `dpd`, `status_approval`, `user_approver_id`, `dokumen_pendukung_id` |
| `mst.instrumen` | READ | `kode_instrumen`, `nama`, `klasifikasi_psak71`, `counterparty_id` |
| `mst.counterparty` | READ | `nama_counterparty` (untuk display) |
| `sec.user` | READ | `nama_lengkap` (untuk display approver) |
| `doc.upload` | READ | `id`, `filename` (link dokumen pendukung) |

### Permissions Needed
| Actor | Permission |
|---|---|
| ROLE-RISK | `ecl_staging.read`, `instrumen.read` |
| ROLE-AUDIT | `ecl_staging.read`, `instrumen.read`, `audit_log.read` |
| Semua role lain | Tidak ada akses (forbidden) |

### Audit Events
| Action | Trigger |
|---|---|
| `ECL_STAGING.EXPORT` | Saat user export staging history (CSV/XLSX) |

### Acceptance Criteria

```gherkin
Feature: Baca staging history per instrumen dengan sort, filter, dan export

  Background:
    Given user ter-autentikasi sebagai ROLE-RISK
    And instrumen INST-0040 memiliki 7 row di ecl.stage_history (mix SICR, cure, override)

  # ─── HAPPY PATH: Tampil timeline staging ─────────────────────────────────

  Scenario: Tampilkan staging history dengan sort default terbaru di atas
    When user mengakses GET /ecl/staging/history/INST-0040
    Then response 200 mengandung array 7 row stage_history
    And diurutkan default: tanggal_migrasi DESC (terbaru di atas)
    And setiap row mengandung: tanggal_migrasi, stage_sebelum, stage_sesudah, trigger_type, detail_trigger, status_approval
    And pagination cursor-based aktif (limit default 50)
    And UI menampilkan timeline visual: badge stage per baris, ikon trigger type

  # ─── HAPPY PATH: Filter by trigger_type ──────────────────────────────────

  Scenario: Filter staging history hanya untuk trigger rating
    When user mengakses dengan filter ?filter[trigger_type]=RATING_DOWNGRADE
    Then response hanya mengandung row dengan trigger_type = 'RATING_DOWNGRADE'
    And filter chip "trigger_type: RATING_DOWNGRADE" muncul di UI
    And URL state di-update: ?filter[trigger_type]=RATING_DOWNGRADE

  # ─── HAPPY PATH: Drilldown detail trigger ────────────────────────────────

  Scenario: Drilldown ke detail row SICR — menampilkan evidence lengkap
    When user mengklik baris stage_history dengan trigger_type = 'RATING_DOWNGRADE'
    Then panel detail terbuka menampilkan:
      | Field               | Value                                         |
      | Tanggal Perubahan   | 2026-01-15                                    |
      | Stage Sebelum       | Stage 1                                       |
      | Stage Sesudah       | Stage 2                                       |
      | Trigger             | Rating Downgrade (RATING_DOWNGRADE)           |
      | Detail              | "Rating CP-BANK-01 turun dari idAA ke idBBB+" |
      | Rating Saat Migrasi | idBBB+                                        |
      | DPD                 | — (tidak relevan untuk trigger ini)           |
      | Approval            | AUTO                                          |

  # ─── HAPPY PATH: Export CSV staging history ──────────────────────────────

  Scenario: Export staging history ke CSV — respek filter aktif
    Given filter aktif: stage_sesudah = STAGE_2
    When user klik "Export" → pilih CSV
    Then file CSV diunduh langsung ke browser (< 10k row untuk instrumen tunggal = inline)
    And CSV hanya mengandung row dengan stage_sesudah = 'STAGE_2' (filter di-respect)
    And header row: "Tanggal Migrasi,Stage Sebelum,Stage Sesudah,Trigger,Detail,Rating,DPD,Status Approval"
    And audit log: ECL_STAGING.EXPORT dengan filter aktif dan row count

  # ─── EDGE CASE: Instrumen baru — belum ada staging history ───────────────

  Scenario: Instrumen baru klasifikasi AC belum pernah dievaluasi staging
    Given instrumen INST-9999 baru di-approve, belum ada row stage_history
    When user mengakses GET /ecl/staging/history/INST-9999
    Then response 200 dengan data array kosong
    And UI menampilkan empty state: "Belum ada riwayat staging untuk instrumen ini."
    And info banner: "Instrumen ini akan dievaluasi saat rating counterparty berikutnya di-import atau DPD job berikutnya berjalan."

  # ─── ERROR CASE: User tanpa permission mencoba akses ─────────────────────

  Scenario: ROLE-MAKER-TR mencoba membaca staging history — forbidden
    Given user ter-autentikasi sebagai ROLE-MAKER-TR
    When user mengakses GET /ecl/staging/history/INST-0040
    Then sistem mengembalikan HTTP 403 FORBIDDEN
    And pesan: "Anda tidak memiliki permission ecl_staging.read"

  # ─── HAPPY PATH: ROLE-AUDIT lihat staging history instrumen soft-deleted ─

  Scenario: ROLE-AUDIT mengakses history instrumen yang sudah di-soft-delete
    Given instrumen INST-OLD di-soft-delete (deleted_at IS NOT NULL)
    And ROLE-AUDIT mengakses dengan ?include_deleted=true
    When user mengakses GET /ecl/staging/history/INST-OLD?include_deleted=true
    Then response 200 dengan semua stage_history untuk INST-OLD
    And header response mengandung X-Instrument-Deleted: true
    And UI menampilkan banner: "Instrumen ini sudah di-nonaktifkan (soft-deleted)"
```

### Open Questions — Story 5
| ID | Pertanyaan | Asumsi Default |
|---|---|---|
| OQ-STG-5a | Apakah ada endpoint agregasi (staging history untuk semua instrumen dalam satu portofolio)? | Tidak di P4-M1 scope. Agregasi adalah bagian dari reporting (Phase 6). |
| OQ-STG-5b | Apakah link ke `doc.upload` menampilkan file langsung atau hanya metadata? | Metadata + signed MinIO URL (TTL 1 jam). Rendering isi dokumen = terpisah. |

---

## Cross-Story: Staging State Machine Summary

```
            RATING_DOWNGRADE (≥2 notch)
            IG_TO_NON_IG
            DPD_GTE_30
Stage 1 ─────────────────────────────────► Stage 2
   ▲                                          │
   │  CURE_3_PERIODE_BULANAN                  │ RATING_DEFAULT
   │  (Story 3, auto job)                     │ DPD_GTE_90
   │                                          ▼
   │                                        Stage 3
   │                                          │
   └──────────────────────────────────────────┘
       MANUAL_OVERRIDE + 6-eyes ALCO
       (Story 4 only — Stage 3 cure tidak otomatis)
```

**Transisi yang TIDAK valid** (engine harus reject):
- Stage 1 → Stage 1 (tidak ada perubahan)
- Stage 3 → Stage 2 via auto-job (hanya via manual override)
- Stage 3 → Stage 1 direct (harus lewat Stage 2 dulu, atau manual override dengan justifikasi)

---

## Non-Functional Requirements (Semua Story P4-M1)

| NFR | Requirement | Referensi |
|---|---|---|
| Compliance gate | `ifrs9-compliance-reviewer` BLOCKING sebelum merge ke `develop` | FSD-APP-C §3, DEC-010..012 |
| Append-only | `ecl.stage_history` tidak bisa di-UPDATE atau di-DELETE (sudah ada trigger `tg_ecl_stage_history_no_delete` di migration 000005) | DEC-018 |
| Decimal precision | Tidak ada float64 untuk DPD rate, notch numerik harus INT, EAD/delta_ecl via `shopspring/decimal` | DEC-016 |
| FVTPL/FVOCI Election | Di-skip dari staging evaluation; tetap FVTPL/FVOCI_ELECTION tidak perlu stage | OQ-G (Phase 4 plan §7) |
| Long-running job | Rating staging job (Story 1) dan cure job (Story 3) untuk seluruh portofolio harus Asynq + progress UX §3 + sys.job persist | CLAUDE.md UX rule §3 |
| Idempotency | Manual override endpoint wajib Idempotency-Key | DEC-021 |
| Audit trail | Setiap mutation ke ecl.stage_history + staging action harus tulis ke aud.audit_log di transaksi yang sama | DEC-018 |
| Cursor pagination | GET staging history gunakan cursor, bukan offset | DEC-022 |
| Export UX | List staging history: sort + filter + CSV/XLSX export. Dataset per instrumen < 10k → inline | CLAUDE.md UX rule §1 |
| SoD enforcement | Manual override: maker ≠ reviewer ≠ approver, enforced server-side | DEC-017 |
| MFA | ROLE-ALCO approver untuk manual override wajib MFA aktif | DEC-026 |

---

## Handoff Checklist

Setelah story ini di-sign-off oleh `ifrs9-compliance-reviewer`:

- [ ] `system-analyst` → OpenAPI fragment: `GET /ecl/staging/current/{instrumen_id}`, `GET /ecl/staging/history/{instrumen_id}`, `POST /ecl/staging/evaluate/{instrumen_id}` (manual trigger), `POST /ecl/staging/override` + workflow endpoints `/submit`, `/review`, `/approve`, `/reject`
- [ ] `system-analyst` → State machine diagram staging (Stage 1/2/3 + valid transitions)
- [ ] `system-analyst` → Go interface contract: `StagingService { EvaluateSICR, EvaluateCure, EvaluateDPD, GetCurrentStage, GetHistory, ProposeOverride }`
- [ ] `data-modeler` → Migration 000022: audit cols + indexes untuk `ecl.stage_history`; keputusan GAP-DPD (tabel `trx.dpd_record` jika Opsi A); override workflow state tracking
- [ ] `ecl-eir-engineer` → Implementasi domain logic di `backend/internal/ecl/staging/`
- [ ] `backend-engineer-go` → HTTP handler + Gin routing + Asynq worker plumbing
- [ ] `ifrs9-compliance-reviewer` → BLOCKING gate: verifikasi SICR trigger logic, cure 3 periode, PD=1.0 Stage 3, append-only enforcement
- [ ] Konfirmasi OQ-B (definisi 3 periode cure) ke Treasury/Finance Controller sebelum merge
- [ ] Konfirmasi GAP-DPD opsi A/B/C ke orchestrator sebelum ecl-eir-engineer mulai Story 2
