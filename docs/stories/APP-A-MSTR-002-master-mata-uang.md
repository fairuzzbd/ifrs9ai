# APP-A-MSTR-002 — Master Mata Uang (Pilot Pola Generik)

**Story ID**: APP-A-MSTR-002
**Modul**: APP-A — Master Data Management
**Sub-modul**: `mst.mata_uang` (Currency Master)
**Tipe**: Concrete Story — Pilot implementasi pola generik (APP-A-MSTR-001)
**Status**: DRAFT — menunggu review system-analyst
**Author**: business-analyst
**Tanggal**: 2026-06-03
**Linked FSD**: FSD-APP-A-MasterData-SPPI-BM-v1.1.md §1.7 (Master Mata Uang & Kurs)
**Linked FSD (parent master)**: FSD-APP-D-PeriodeBuku-FX-Mapping-v1.0.md §2
**Linked BRD**: BR-MAS-009 (Multi-currency support); BRD §8.1
**Linked Decision Log**: DEC-016 (decimal), DEC-017 (4-eyes), DEC-021 (idempotency), DEC-022 (cursor pagination)
**Parent Story**: APP-A-MSTR-001 (pola generik — semua AC di sana berlaku di sini kecuali di-override)

---

## Ringkasan Modul

Master Mata Uang menyimpan daftar kode mata uang ISO 4217 yang diakui sistem BLIPS. Setiap transaksi, instrumen, dan FX rate mengacu ke tabel ini. Ini adalah modul **paling sederhana** di antara 16 modul mst.* — dipilih sebagai **pilot pola generik** untuk memvalidasi template sebelum direplikasi ke modul kompleks.

**Kenapa sederhana**: Tidak ada PII, tidak ada ECL parameter, tidak ada upload/import, tidak ada FK kompleks. Pure reference data dengan lifecycle: AKTIF / TIDAK_AKTIF.

---

## Entitas: `mst.mata_uang`

| Field DB | Tipe | Wajib | Validasi | Keterangan |
|---|---|---|---|---|
| `kode_mata_uang` | CHAR(3) PK | Ya | Format ISO 4217 (3 huruf kapital) | Primary key & business key; contoh: IDR, USD, EUR, SGD |
| `nama_mata_uang` | VARCHAR(60) | Ya | Min 3 karakter | Nama lengkap; contoh: Rupiah Indonesia |
| `simbol` | VARCHAR(5) | Ya | Min 1 karakter | Contoh: Rp, $, €, £, S$ |
| `decimal_places` | SMALLINT | Ya | 0 ≤ nilai ≤ 4; default 2 | Jumlah digit desimal untuk tampilan (IDR = 0, USD = 2, KWD = 3) |
| `sumber_kurs_default` | VARCHAR(30) | Ya | Enum: BI_JISDOR, BI_KURS_TENGAH, INTERNAL | Default source untuk FX rate |
| `frekuensi_update` | VARCHAR(20) | Ya | Enum: HARIAN, INTRA_DAY, BULANAN | Frekuensi update kurs |
| `aktif_flag` | BOOLEAN | Ya | Default TRUE | Aktif/non-aktif; non-aktif = tidak bisa dipakai di transaksi baru |
| `tanggal_mulai_aktif` | DATE | Ya | ≤ tanggal hari ini | Tanggal mata uang mulai aktif di BLIPS |
| Audit fields | — | Ya | — | `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `row_version`, `tenant_id` (DEC-018, DEC-023) |

**Catatan**: `mst.mata_uang` tidak memiliki kolom `workflow_status` yang terpisah karena kode mata uang adalah reference data yang relatif stabil. Workflow tetap diterapkan menggunakan tabel `sys.workflow_instance` yang di-link via FK (generic workflow engine dari Phase 2).

---

## Actors

| Role | Permission | Aksi |
|---|---|---|
| ROLE-AKUN | `mata_uang.create`, `mata_uang.update`, `mata_uang.read` | Maker — input & edit mata uang baru |
| ROLE-AKUN-CTL | `mata_uang.review`, `mata_uang.approve`, `mata_uang.read` | Reviewer & Approver (MFA mandatory per DEC-026) |
| ROLE-AUDIT | `mata_uang.read`, `mata_uang.export` | Read-only; bisa include_deleted |
| ROLE-IT-ADMIN | `mata_uang.read` | Monitoring saja; tidak bisa domain workflow |

**Alasan AKUN sebagai Maker, AKUN-CTL sebagai Approver**: Mata uang adalah master akuntansi (bukan treasury). Sesuai RACI BRD §4 — Akuntansi adalah Process Owner untuk reference data FX/mata uang.

---

## Trigger

1. ROLE-AKUN menambahkan mata uang baru yang belum ada (mis. GBP untuk instrumen obligasi sterling baru).
2. ROLE-AKUN memperbarui atribut mata uang (mis. mengubah `decimal_places` atau menonaktifkan mata uang yang sudah tidak digunakan).
3. Proses migrasi awal: seed data 30+ mata uang standar (IDR, USD, EUR, SGD, JPY, GBP, AUD, dll) via SQL seeder tanpa workflow (initial setup).

---

## Pre-conditions

1. User ter-autentikasi via Keycloak, JWT valid.
2. ROLE-AKUN-CTL: JWT harus mengandung `mfa_verified: true`.
3. Setiap mutation request wajib menyertakan `Idempotency-Key: <uuid-v4>` di header.
4. Untuk create: `kode_mata_uang` belum ada di tabel (unique PK).

---

## Acceptance Criteria — Story Konkret `mst.mata_uang`

### Feature: Create mata uang baru

```gherkin
Feature: ROLE-AKUN membuat master mata uang baru

  Background:
    Given ROLE-AKUN ter-autentikasi dan memiliki permission "mata_uang.create"
    And request header: "Idempotency-Key: 550e8400-e29b-41d4-a716-446655440000"
    And halaman Create Mata Uang terbuka di /master/mata-uang/new

  Scenario: Happy path — create mata uang GBP
    When user mengisi form:
      | Field              | Nilai              |
      | Kode Mata Uang     | GBP                |
      | Nama               | Pound Sterling     |
      | Simbol             | £                  |
      | Decimal Places     | 2                  |
      | Sumber Kurs        | BI_KURS_TENGAH     |
      | Frekuensi Update   | HARIAN             |
      | Tgl Mulai Aktif    | 2026-06-03         |
    And user klik "Simpan"
    Then HTTP 201 diterima dengan body:
      """
      {
        "data": {
          "kode_mata_uang": "GBP",
          "nama_mata_uang": "Pound Sterling",
          "simbol": "£",
          "decimal_places": 2,
          "sumber_kurs_default": "BI_KURS_TENGAH",
          "frekuensi_update": "HARIAN",
          "aktif_flag": true,
          "tanggal_mulai_aktif": "2026-06-03",
          "workflow_status": "DRAFT"
        },
        "meta": { "traceId": "..." }
      }
      """
    And toast sukses hijau 4 detik: "Mata uang GBP — Pound Sterling berhasil dibuat. Menunggu review Finance Controller."
    And toast mengandung link "Lihat detail →" menuju /master/mata-uang/GBP
    And audit log row dibuat: action="MATA_UANG.CREATE", entity_id="GBP"
    And URL redirect ke detail page /master/mata-uang/GBP

  Scenario: Validation error — kode tidak ISO 4217 format
    When user mengisi "Kode Mata Uang" dengan "RUPIAH" (lebih dari 3 huruf)
    And user klik "Simpan"
    Then HTTP 400 VALIDATION_FAILED diterima
    And field "Kode Mata Uang" di-highlight merah dengan pesan:
      "Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)"
    And toast error merah persistent: "1 field bermasalah — lihat form di bawah. Trace: {traceId}"
    And tidak ada record dibuat

  Scenario: Validation error — kode sudah ada
    Given "USD" sudah ada di database
    When user mengisi "Kode Mata Uang" dengan "USD"
    And user klik "Simpan"
    Then HTTP 400 VALIDATION_FAILED diterima
    And pesan: "Mata uang USD sudah terdaftar di sistem."
    And field "Kode Mata Uang" di-highlight

  Scenario: Validation error — decimal_places di luar range
    When user mengisi "Decimal Places" dengan 5
    And user klik "Simpan"
    Then HTTP 400 VALIDATION_FAILED diterima
    And pesan: "Decimal places harus antara 0 dan 4."

  Scenario: Validation error — tanggal mulai aktif di masa depan
    When user mengisi "Tgl Mulai Aktif" dengan tanggal besok
    And user klik "Simpan"
    Then HTTP 400 VALIDATION_FAILED diterima
    And pesan: "Tanggal mulai aktif tidak boleh di masa depan."

  Scenario: Submit button disabled selama proses
    When user klik "Simpan"
    Then tombol "Simpan" langsung di-disable dan menampilkan spinner inline
    And tidak bisa di-klik dua kali (block double-submit)
    And tombol kembali aktif setelah response diterima

  Scenario: Idempotency replay — request duplikat
    Given request create GBP sudah berhasil (HTTP 201)
    When user kirim ulang request dengan Idempotency-Key yang sama dan payload identik
    Then HTTP 200 diterima dengan response identik ke create pertama
    And "error.code" dalam response: "IDEMPOTENCY_REPLAY"
    And tidak ada row audit log tambahan
    And tidak ada duplicate record di database

  Scenario: Idempotency mismatch — key sama payload beda
    Given request create GBP sudah berhasil
    When user kirim request dengan Idempotency-Key yang sama tapi nama berbeda ("British Pound")
    Then HTTP 422 IDEMPOTENCY_MISMATCH diterima
    And pesan menjelaskan konflik idempotency
```

---

### Feature: List mata uang (sort + paging + filter + export)

```gherkin
Feature: Daftar mata uang dengan DataTable lengkap

  Background:
    Given user ter-autentikasi dengan permission "mata_uang.read"
    And halaman /master/mata-uang terbuka

  Scenario: Tampilan default list
    Then tabel menampilkan kolom: Kode, Nama, Simbol, Decimal Places, Sumber Kurs, Status, Dibuat Oleh, Dibuat Pada, Aksi
    And data di-sort default "kode_mata_uang ASC"
    And paginator menampilkan limit 50, tombol Prev (disabled di halaman 1) dan Next
    And URL: /master/mata-uang?sort=kode_mata_uang:asc
    And ikon sort ↑ muncul di header kolom "Kode"
    And tombol "+ Tambah Mata Uang" tampil (hanya untuk ROLE-AKUN)

  Scenario: Sort berdasarkan Nama secara descending
    When user klik header "Nama" dua kali
    Then data di-sort "nama_mata_uang DESC"
    And ikon ↓ muncul di header "Nama"
    And URL: /master/mata-uang?sort=nama_mata_uang:desc

  Scenario: Filter status aktif saja
    When user pilih filter "Status = Aktif"
    Then tabel hanya menampilkan record dengan "aktif_flag = true"
    And filter chip "Status: Aktif" muncul di filter bar
    And URL: /master/mata-uang?filter[aktif_flag]=true

  Scenario: Text search global
    When user mengetik "Dollar" di search box
    Then tabel menampilkan record yang mengandung "Dollar" di nama atau kode
    And URL: /master/mata-uang?q=Dollar

  Scenario: Kombinasi sort + filter + search
    Given filter "Status = Aktif" aktif
    And search "USD" aktif
    When user sort berdasarkan "Kode ASC"
    Then query backend: "?sort=kode_mata_uang:asc&filter[aktif_flag]=true&q=USD"
    And semua kondisi teraplikasi sekaligus

  Scenario: Export CSV — dataset kecil
    Given 45 record mata uang ada dengan filter aktif
    When user klik "Export" → pilih "CSV"
    Then browser mendownload file "mata-uang-20260603.csv"
    And file mengandung 45 baris data (sesuai filter aktif)
    And header row: "Kode,Nama,Simbol,Decimal Places,Sumber Kurs,Status,Tgl Mulai Aktif"
    And encoding UTF-8 with BOM
    And audit log: action="MATA_UANG.EXPORT", row_count=45, format="csv"

  Scenario: Export dengan filter tertentu — hanya mata uang aktif
    Given filter "Status = Aktif" aktif (50 record)
    When user klik Export CSV
    Then file hanya berisi 50 record aktif (bukan semua mata uang termasuk non-aktif)

  Scenario: Paging — navigasi ke halaman berikutnya
    Given ada 120 record mata uang
    And limit default 50
    When user klik "Next"
    Then halaman 2 dimuat dengan cursor baru
    And footer: "Page 2 of ~3"
    And URL: /master/mata-uang?cursor=eyJ...&sort=kode_mata_uang:asc

  Scenario: Empty state — tidak ada mata uang dengan filter aktif
    Given search "XXXXX" tidak menghasilkan data
    Then tabel menampilkan: ilustrasi kosong + "Tidak ada mata uang yang cocok dengan pencarian 'XXXXX'"
    And tombol "Hapus pencarian" tampil

  Scenario: ROLE-AUDIT tidak melihat tombol aksi mutasi
    Given ROLE-AUDIT mengakses halaman list
    Then kolom "Aksi" tidak menampilkan tombol Edit atau Delete
    And tidak ada tombol "+ Tambah Mata Uang"
    And tombol Export tetap tersedia
```

---

### Feature: Detail & Update mata uang

```gherkin
Feature: Lihat dan edit detail mata uang

  Background:
    Given user ROLE-AKUN dengan permission "mata_uang.update"
    And mata uang "GBP" ada dengan status "DRAFT"

  Scenario: Happy path — update nama dan simbol
    Given user mengakses /master/mata-uang/GBP
    When user klik "Edit"
    And mengubah "Nama" menjadi "British Pound Sterling"
    And mengubah "Simbol" menjadi "£ GB"
    And klik "Simpan" dengan Idempotency-Key baru
    Then HTTP 200 diterima
    And data GBP terupdate di database
    And row_version increment dari 1 ke 2
    And audit log: action="MATA_UANG.UPDATE", before_jsonb mengandung nama lama, after_jsonb mengandung nama baru
    And toast sukses: "Mata uang GBP — British Pound Sterling berhasil diperbarui."

  Scenario: Update kode mata uang — tidak diizinkan
    Given GBP sudah ada
    When user mencoba mengubah field "Kode Mata Uang" di form
    Then field "Kode Mata Uang" bersifat read-only (disabled)
    And tooltip: "Kode mata uang tidak bisa diubah setelah dibuat. Nonaktifkan mata uang ini dan buat baru jika perlu."

  Scenario: Optimistic lock — dua user edit bersamaan
    Given user A dan user B keduanya membuka form edit GBP (row_version=1)
    When user A simpan (berhasil, row_version jadi 2)
    And user B kemudian mencoba simpan (masih pakai row_version=1)
    Then HTTP 409 CONFLICT diterima user B
    And pesan: "Mata uang GBP telah diubah oleh user lain (treasury.maker). Muat ulang halaman untuk melihat data terbaru."
    And data user B tidak tersimpan

  Scenario: Tidak bisa edit mata uang yang sudah APPROVED
    Given mata uang "IDR" dalam status "APPROVED"
    When ROLE-AKUN mengakses form edit IDR
    Then tombol "Edit" tidak tampil (read-only mode)
    And pesan info: "Mata uang ini sudah aktif. Untuk mengubah atribut, ajukan request ke Finance Controller."
```

---

### Feature: Workflow Approval 4-Eyes (Maker=AKUN, Reviewer+Approver=AKUN-CTL)

```gherkin
Feature: Workflow approval mata uang — AKUN → AKUN-CTL

  Background:
    Given mata uang "GBP" ada dengan status "DRAFT"
    And ROLE-AKUN adalah pembuatnya (maker_id)

  Scenario: AKUN submit untuk review
    Given user ROLE-AKUN yang merupakan maker GBP
    When user klik "Kirim untuk Review"
    Then status GBP berubah menjadi "PENDING_REVIEW"
    And timestamp submitted_at tersimpan
    And audit log: "MATA_UANG.SUBMIT"
    And notifikasi in-app dikirim ke semua ROLE-AKUN-CTL: "Mata uang GBP menunggu review Anda"
    And toast sukses: "GBP berhasil dikirim untuk review Finance Controller."

  Scenario: AKUN-CTL review dan sign-off (approve ke approval)
    Given status GBP: "PENDING_REVIEW"
    And user ROLE-AKUN-CTL (bukan maker GBP; SoD OK)
    And user memiliki mfa_verified: true
    When user mengakses queue review dan buka detail GBP
    And mengisi komentar "Review OK — kode ISO valid, decimal places sesuai standar"
    And klik "Setujui"
    Then karena 4-eyes: AKUN-CTL berperan sekaligus sebagai reviewer DAN approver (satu langkah approve final)
    And status GBP berubah menjadi "APPROVED"
    And aktif_flag tetap TRUE
    And audit log: "MATA_UANG.APPROVE" dengan komentar
    And toast sukses AKUN-CTL: "Mata uang GBP berhasil disetujui."
    And notifikasi ke maker: "Mata uang GBP yang Anda buat telah disetujui oleh {reviewer_name}."

  Scenario: SoD violation — AKUN-CTL yang sama adalah maker GBP
    Given mata uang "GBP" dibuat oleh user X yang punya role AKUN-CTL juga
    And status: "PENDING_REVIEW"
    When user X (yang adalah maker) mencoba approve GBP
    Then HTTP 403 SOD_VIOLATION diterima
    And pesan: "Anda adalah pembuat mata uang ini. Tidak bisa menjadi approver sesuai aturan Segregation of Duties. Minta Finance Controller lain untuk mereview."
    And tidak ada perubahan status

  Scenario: AKUN-CTL tanpa MFA — ditolak
    Given user ROLE-AKUN-CTL dengan mfa_verified: false di JWT
    When user mencoba approve GBP
    Then HTTP 403 FORBIDDEN diterima
    And pesan: "Multi-Factor Authentication wajib untuk Finance Controller. Silakan login ulang dengan MFA."

  Scenario: AKUN-CTL reject — kembalikan ke AKUN
    Given status GBP: "PENDING_REVIEW"
    When ROLE-AKUN-CTL (bukan maker) klik "Tolak"
    Then dialog konfirmasi muncul dengan input "Alasan Penolakan" (required)
    When user mengisi "Kode mata uang GBP sudah ada di sistem dengan kode yang berbeda. Harap verifikasi."
    And klik "Lanjut Tolak"
    Then status GBP berubah menjadi "RETURNED"
    And komentar penolakan tersimpan
    And audit log: "MATA_UANG.REJECT" dengan komentar
    And notifikasi ke maker: "Mata uang GBP dikembalikan oleh {reviewer_name}: 'Kode mata uang GBP sudah ada...'"
    And ROLE-AKUN bisa edit dan re-submit GBP

  Scenario: Maker re-submit setelah returned
    Given status GBP: "RETURNED"
    And ROLE-AKUN (maker) melihat komentar penolakan
    When user mengedit GBP (field yang relevan)
    And klik "Kirim Ulang untuk Review"
    Then status kembali ke "PENDING_REVIEW"
    And audit log: "MATA_UANG.RESUBMIT"
```

---

### Feature: Soft-Delete mata uang

```gherkin
Feature: Soft-delete mata uang yang tidak terpakai

  Background:
    Given ROLE-AKUN dengan permission "mata_uang.delete"

  Scenario: Happy path — soft-delete mata uang DRAFT tanpa referensi
    Given mata uang "XYZ" (status DRAFT) tidak direferensikan oleh instrumen manapun
    When user klik "Hapus" pada XYZ
    Then dialog konfirmasi muncul: "Yakin ingin menghapus mata uang XYZ — Xyzcoin? Aksi ini tidak dapat dibatalkan."
    When user klik "Hapus" di dialog
    Then sistem men-set deleted_at = now() dan deleted_by = user.id
    And XYZ tidak muncul di list default
    And audit log: "MATA_UANG.DELETE" dengan before_jsonb lengkap
    And toast sukses: "Mata uang XYZ berhasil dihapus dari sistem."

  Scenario: Tidak bisa hapus mata uang yang direferensikan instrumen
    Given mata uang "USD" direferensikan oleh 12 instrumen aktif
    When user mencoba menghapus USD
    Then HTTP 409 CONFLICT diterima
    And pesan: "Mata uang USD tidak bisa dihapus karena masih digunakan oleh 12 instrumen. Nonaktifkan mata uang ini dengan mengubah aktif_flag menjadi false."
    And tidak ada perubahan data

  Scenario: Tidak bisa hapus mata uang "IDR" (currency fungsional Tugure)
    Given "IDR" adalah currency fungsional sistem
    When user mencoba menghapus IDR
    Then HTTP 403 FORBIDDEN diterima
    And pesan: "Mata uang IDR adalah currency fungsional Tugure dan tidak bisa dihapus."

  Scenario: ROLE-AUDIT melihat mata uang yang di-soft-delete
    Given mata uang "XYZ" sudah di-soft-delete
    And user ROLE-AUDIT mengakses /master/mata-uang?include_deleted=true
    Then XYZ muncul di list dengan indikator visual "Dihapus" (badge merah)
    And kolom "Dihapus Oleh" dan "Tanggal Hapus" tampil
    And tidak ada tombol aksi (Akun AUDIT read-only)
```

---

### Feature: Nonaktifkan/Aktifkan mata uang

```gherkin
Feature: Toggle aktif/non-aktif mata uang

  Background:
    Given ROLE-AKUN-CTL dengan permission "mata_uang.update"
    And mfa_verified: true

  Scenario: Nonaktifkan mata uang yang sudah APPROVED
    Given mata uang "CHF" status APPROVED dan aktif_flag = true
    And "CHF" tidak digunakan dalam instrumen aktif manapun
    When ROLE-AKUN (maker) mengajukan perubahan aktif_flag = false
    And ROLE-AKUN-CTL menyetujui (workflow mini 4-eyes)
    Then aktif_flag CHF berubah menjadi false
    And CHF tidak bisa dipilih di form instrumen baru
    And CHF tetap tampil di data historis (instrumen lama tidak terpengaruh)
    And audit log: "MATA_UANG.UPDATE" dengan before aktif_flag=true, after aktif_flag=false

  Scenario: Mata uang non-aktif tidak bisa dipilih di form instrumen baru
    Given "CHF" aktif_flag = false
    When user ROLE-MAKER-TR membuka form create instrumen baru
    And mencari "CHF" di dropdown mata uang
    Then "CHF" tidak muncul di dropdown (di-filter aktif saja)
    And tooltip di field mata uang: "Hanya menampilkan mata uang aktif."
```

---

## Data Test Seed (untuk QA)

```
Minimal seed untuk UAT mata uang:
- IDR | Rupiah Indonesia | Rp | 0 | BI_JISDOR | HARIAN | true | 2020-01-01
- USD | Dolar Amerika | $ | 2 | BI_JISDOR | HARIAN | true | 2020-01-01
- EUR | Euro | € | 2 | BI_KURS_TENGAH | HARIAN | true | 2020-01-01
- SGD | Dolar Singapura | S$ | 2 | BI_KURS_TENGAH | HARIAN | true | 2020-01-01
- JPY | Yen Jepang | ¥ | 0 | BI_KURS_TENGAH | HARIAN | true | 2020-01-01
- GBP | Pound Sterling | £ | 2 | BI_KURS_TENGAH | HARIAN | false | 2020-01-01  ← contoh non-aktif
- CHF | Franc Swiss | Fr | 2 | INTERNAL | BULANAN | false | 2020-01-01  ← untuk test delete
- XYZ | Test Currency | X | 2 | INTERNAL | BULANAN | true | 2026-06-03  ← untuk test delete (no ref)
```

**Test persona seed** (sesuai SoD requirement):
- `akun.maker` — ROLE-AKUN (tidak punya AKUN-CTL) → bisa create, tidak bisa approve
- `akun.ctl.1` — ROLE-AKUN-CTL → bisa approve; tidak membuat GBP
- `akun.ctl.2` — ROLE-AKUN-CTL → bisa approve; untuk test jika ctl.1 adalah maker
- `audit.viewer` — ROLE-AUDIT → read-only semua

---

## API Endpoints (mata uang konkret)

| Method | Endpoint | Permission | Keterangan |
|---|---|---|---|
| GET | /api/v1/master/mata-uang | mata_uang.read | List dengan sort+paging+filter |
| GET | /api/v1/master/mata-uang/{kode} | mata_uang.read | Detail single (kode = PK, mis. GBP) |
| POST | /api/v1/master/mata-uang | mata_uang.create | Create baru (status DRAFT) |
| PUT | /api/v1/master/mata-uang/{kode} | mata_uang.update | Update (hanya jika DRAFT atau RETURNED) |
| DELETE | /api/v1/master/mata-uang/{kode} | mata_uang.delete | Soft-delete |
| POST | /api/v1/master/mata-uang/{kode}/submit | mata_uang.submit | Maker submit untuk review |
| POST | /api/v1/master/mata-uang/{kode}/approve | mata_uang.approve | AKUN-CTL approve |
| POST | /api/v1/master/mata-uang/{kode}/reject | mata_uang.reject | AKUN-CTL reject dengan komentar |
| GET | /api/v1/master/mata-uang/export | mata_uang.read | Export CSV/XLSX (respects filter) |
| GET | /api/v1/master/mata-uang/{kode}/history | mata_uang.read | Audit trail entitas ini |

**Note untuk system-analyst**: Path menggunakan `{kode}` (CHAR(3)) sebagai identifier URL karena ini PK dan human-readable. Tidak perlu UUID di URL untuk entitas ini.

---

## Decision Log Check (Konfirmasi Tidak Ada Konflik)

| Decision | Teraplikasi di Story ini | Status |
|---|---|---|
| DEC-016: shopspring/decimal, NUMERIC(20,4) IDR | Tidak ada amount money di mata_uang; decimal_places adalah SMALLINT (jumlah digit) — aman | OK |
| DEC-017: 4-eyes untuk master umum | Diterapkan: AKUN (maker) → AKUN-CTL (approver) | OK |
| DEC-021: Idempotency-Key wajib | Semua mutation endpoint mewajibkan header ini | OK |
| DEC-022: Cursor pagination only | List endpoint menggunakan cursor pagination | OK |
| DEC-018: Audit trail 10+10 tahun | aud.audit_log ditulis setiap mutasi | OK |
| DEC-026: MFA wajib AKUN-CTL | Cek mfa_verified di JWT sebelum approve | OK |
| DEC-028: PII column encrypt | Tidak ada PII di mata_uang — not applicable | OK, tidak perlu |

**Tidak ada konflik dengan Decision Log.**

---

## Open Questions Spesifik untuk Modul Mata Uang

1. **Workflow reviewer vs approver**: Pada pola generik didefinisikan 3 pihak (maker, reviewer, approver). Untuk `mst.mata_uang`, apakah AKUN-CTL berperan sebagai sekaligus reviewer + approver (2-step dalam 1 role), atau perlu 2 AKUN-CTL berbeda (satu reviewer, satu approver)? Story ini mengasumsikan AKUN-CTL = single role yang approve langsung (1 step, bukan 2). Konfirmasi diperlukan.

2. **IDR proteksi hard-coded atau via config**: Apakah proteksi "IDR tidak bisa dihapus" di-hard-code di service layer, atau via flag `system_currency: true` di tabel yang bisa di-config IT Admin? Usulan: flag `is_system_currency BOOLEAN DEFAULT FALSE` agar maintainable.

3. **decimal_places usage di UI**: Apakah `decimal_places` di `mst.mata_uang` langsung dipakai untuk formatting angka di frontend (mis. USD selalu tampil dengan 2 desimal)? Jika ya, sistem perlu config lookup saat render amount. Konfirmasi agar frontend engineer implement dengan benar.

4. **Mata uang untuk FX Rate seed awal**: Berapa mata uang yang perlu di-seed untuk go-live? Apakah cukup yang ada di portofolio Tugure aktual (mungkin hanya IDR + USD + SGD), atau seed semua ISO 4217 (150+ kode)?

---

## Rekomendasi Next Step

**Kepada tech-lead-orchestrator** untuk di-dispatch:

1. **system-analyst**: Buat OpenAPI contract untuk 10 endpoint di atas. Khusus: definisikan state machine `mata_uang.workflow_status` (DRAFT → PENDING_REVIEW → APPROVED / RETURNED) dan validasi SoD. Ini menjadi template state machine untuk semua modul mst.*.

2. **data-modeler**: Verifikasi `mst.mata_uang` schema di init SQL sudah sesuai field spec di story ini (tambah `decimal_places` jika belum ada, verifikasi audit cols). Buat seed migration untuk mata uang standar Tugure.

3. **backend-engineer-go**: Implementasi setelah OpenAPI contract dari system-analyst siap dan PR #10 (Phase 2 foundation) merge ke develop.

4. **qa-engineer**: Buat UAT script menggunakan data test seed di atas, covering: happy path, validation error, idempotency, SoD violation, soft-delete, dan export.

5. **TIDAK perlu** dispatch `ifrs9-compliance-reviewer` atau `security-engineer` untuk modul ini (bukan ECL param, tidak ada PII).

6. Setelah `mst.mata_uang` selesai dan passing UAT, validasi pola generik APP-A-MSTR-001 sudah correct, lalu mulai replikasi ke modul berikutnya (`mst.periode_buku`).
