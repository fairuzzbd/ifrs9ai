# UAT Script — APP-D Jurnal Engine (P5-M2)

**ID**: UAT-APP-D-001
**Modul**: APP-D — Periode Buku, FX, Mapping Jurnal & GL Interface
**Story Set**: P5-M2 (S1–S6)
**Tanggal Dibuat**: 2026-06-15
**Author**: qa-engineer
**Linked Stories**: `docs/stories/phase-5/P5-M2-jurnal-engine.md`
**Linked Decisions**: DEC-P5-M1-002 (27 codes), DEC-P5-M1-003 (6-eyes/4-eyes), DEC-017 (SoD), DEC-018 (audit), DEC-021 (idempotency), DEC-027 (MFA step-up)

---

## Aktor Penguji

| Alias | Nama | Role | MFA |
|---|---|---|---|
| USR-010 | Dewi Rahayu | ROLE-AKUN | Tidak wajib |
| USR-011 | Eko Susanto | ROLE-AKUN-CTL | Wajib (step-up untuk regulated) |
| USR-012 | Fajar Hidayat | ROLE-RISK | Wajib (step-up wajib untuk approve_2) |
| USR-013 | Admin IT | ROLE-IT-ADMIN | Wajib |
| USR-014 | Ahmad Aulia | ROLE-AUDIT | Tidak wajib (read-only) |

---

## Pre-Kondisi Global

Sebelum menjalankan seluruh TC di bawah, pastikan:

1. **Migration 000035** telah dijalankan: `go run ./cmd/migrator up`
2. **27 event codes** tersedia di `mst.mapping_jurnal_header` dengan `workflow_status = 'APPROVED'`
   ```sql
   SELECT event_code, workflow_status, aktif_flag
   FROM mst.mapping_jurnal_header
   WHERE aktif_flag = true
   ORDER BY event_code;
   -- Harus mengembalikan 27 baris APPROVED + aktif_flag=true
   ```
3. **Periode buku Juni-2026** (`PBUKU-2026-06`) tersedia dengan `status_periode = 'OPEN'`
4. **Chart of Accounts** memiliki minimal kode akun `1110-DEP` dan `1001-KAS` dengan `aktif_flag = true`
5. **Instrumen INST-DEP-001** terdaftar dengan `klasifikasi_psak71 = 'AC'` dan `workflow_status = 'APPROVED'`
6. Semua user aktor (USR-010 s/d USR-014) telah ter-provisioning di Keycloak
7. **Browser**: Chrome/Firefox terbaru, device authenticator untuk MFA terpasang di USR-011, USR-012, USR-013
8. **URL** aplikasi: `https://blips.tugu-re.com` (UAT environment)

---

## TC-001 — ROLE-AKUN Membuat Mapping PENEMPATAN (4-eyes Operational)

**Story**: P5-M2-S1 (AC: Maker berhasil membuat mapping PENEMPATAN baru, operational, 4-eyes)
**Aktor**: USR-010 (Maker), USR-011 (Reviewer + Approver)
**Waktu Estimasi**: 15 menit

### Pre-Kondisi TC-001

- USR-010 login dengan role ROLE-AKUN
- Event code `PENEMPATAN_UAT_001` belum ada di `mst.mapping_jurnal_header`
- Chart of Accounts tersedia: `1110-DEP` (Deposito) dan `1001-KAS` (Kas)

### Langkah-langkah TC-001

**Langkah 1 — USR-010 membuat mapping baru (DRAFT)**

1. Login sebagai USR-010
2. Buka menu **Pengaturan → Mapping Jurnal → Buat Baru**
3. Isi form:
   - Event Code: `PENEMPATAN_UAT_001`
   - Nama Event: `Penempatan Instrumen UAT 001`
   - Kategori Event: `PENEMPATAN` (pilih dari dropdown)
   - Trigger Source: `USER_INPUT`
   - Klasifikasi Berlaku: centang `AC`, `FVOCI`, `FVTPL`
   - Deskripsi: `Template penempatan untuk UAT testing`
4. Tambahkan baris detail:
   - Baris 1: Urutan=1, Kode Akun=`1110-DEP`, DK=DEBIT, Sumber Amount=`nominal_idr`
   - Baris 2: Urutan=2, Kode Akun=`1001-KAS`, DK=KREDIT, Sumber Amount=`nominal_idr`
5. Klik **Simpan**

**Hasil yang diharapkan Langkah 1:**
- Toast hijau muncul: `"Mapping PENEMPATAN_UAT_001 berhasil dibuat. Status: DRAFT. Menunggu submit ke review."`
- Status mapping = `DRAFT`
- Tombol **Submit untuk Review** aktif
- **Audit DB**: `SELECT * FROM aud.audit_log WHERE action = 'JURNAL_MAPPING.CREATE' AND entity_id = {id}` — harus ada 1 baris

**Langkah 2 — USR-010 submit mapping**

6. Klik tombol **Submit untuk Review**
7. Konfirmasi dialog: klik **Ya, Submit**

**Hasil yang diharapkan Langkah 2:**
- Toast hijau: `"Mapping PENEMPATAN_UAT_001 berhasil di-submit. Menunggu review ROLE-AKUN-CTL."`
- Status mapping = `PENDING_REVIEW`
- **Audit DB**: `action = 'JURNAL_MAPPING.SUBMIT'` tersedia

**Langkah 3 — USR-011 review mapping**

8. Login sebagai USR-011
9. Buka **Notifikasi → Queue Review** — tampil `PENEMPATAN_UAT_001`
10. Klik **Review** → lihat detail mapping (2 baris debit/kredit seimbang)
11. Isi komentar: `"Template sudah sesuai chart of accounts. Kode akun valid."`
12. Klik **Setujui Review** → masukkan signature method `JWT_STEP_UP`

**Hasil yang diharapkan Langkah 3:**
- Toast hijau: `"Review berhasil. Mapping PENEMPATAN_UAT_001 menunggu persetujuan Anda."`
- Status = `PENDING_APPROVAL`
- `reviewer_signature_hash` terisi (non-empty)
- **Audit DB**: `action = 'JURNAL_MAPPING.REVIEW'`, `actor_user_id = USR-011`

**Langkah 4 — USR-011 approve mapping (4-eyes operational, NO step-up MFA)**

13. Dari halaman yang sama, klik tombol **Setujui** (tombol Approve)
14. Isi komentar: `"Disetujui. Template siap digunakan resolver."`
15. Klik **Konfirmasi Persetujuan**

> **Catatan**: Untuk mapping operational (PENEMPATAN), step-up MFA TIDAK diperlukan. Jika sistem meminta MFA step-up untuk mapping operational, ini adalah BUG — eskalasi ke tim teknis.

**Hasil yang diharapkan Langkah 4:**
- Toast hijau: `"Mapping PENEMPATAN_UAT_001 berhasil disetujui dan aktif. Resolver dapat menggunakan template ini."`
- Status = `APPROVED_ACTIVE` (bukan PENDING_APPROVAL_2)
- `aktif_flag = true`
- `approver_signature_hash` non-empty
- **Audit DB**: `action = 'JURNAL_MAPPING.APPROVE'`, `actor_user_id = USR-011`

### Verifikasi DB TC-001

```sql
SELECT
    event_code,
    workflow_status,
    aktif_flag,
    maker_id,
    reviewer_id,
    approver_id,
    approver_2_id,
    reviewer_signature_hash,
    approver_signature_hash
FROM mst.mapping_jurnal_header
WHERE event_code = 'PENEMPATAN_UAT_001';
-- Expected: workflow_status='APPROVED_ACTIVE', aktif_flag=true, approver_2_id=NULL
```

```sql
SELECT action, actor_user_id, event_time
FROM aud.audit_log
WHERE entity_type = 'mst.mapping_jurnal_header'
  AND entity_id = (SELECT id FROM mst.mapping_jurnal_header WHERE event_code='PENEMPATAN_UAT_001')
ORDER BY event_time;
-- Expected: CREATE → SUBMIT → REVIEW → APPROVE (4 baris, berurutan)
```

### Sign-off TC-001

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-002 — ROLE-AKUN Membuat Mapping ECL_PEMBENTUKAN (6-eyes Regulated, MFA Step-up)

**Story**: P5-M2-S1 (AC: Maker membuat mapping ECL_PEMBENTUKAN, regulated, 6-eyes)
**Aktor**: USR-010 (Maker), USR-011 (Reviewer + Approver_1), USR-012 (Approver_2 ROLE-RISK)
**Waktu Estimasi**: 20 menit

### Pre-Kondisi TC-002

- USR-010 login dengan role ROLE-AKUN
- Event code `ECL_PEMBENTUKAN_UAT_002` belum ada (buat versi baru, bukan edit yang sudah di-seed)
- USR-011 memiliki device authenticator (MFA) aktif untuk step-up
- USR-012 memiliki device authenticator (MFA) aktif untuk approve_2

### Langkah-langkah TC-002

**Langkah 1–2**: Sama dengan TC-001 Langkah 1–2, dengan perbedaan:
- Event Code: `ECL_PEMBENTUKAN_UAT_002`
- Kategori Event: `ECL` (sistem akan otomatis mendeteksi sebagai REGULATED — 6-eyes)
- Sumber Amount baris 1: `ecl_amount`
- Sumber Amount baris 2: `ecl_amount`

**Verifikasi setelah Create**: Badge `REGULATED (6-eyes)` tampil di detail mapping.

**Langkah 3 — USR-011 review** (sama dengan TC-001 Langkah 3)

**Langkah 4 — USR-011 approve_1 (dengan MFA step-up)**

1. USR-011 klik **Setujui** di halaman mapping `ECL_PEMBENTUKAN_UAT_002`
2. Sistem meminta **MFA Step-up** (karena mapping regulated per DEC-027):
   - Tampil dialog: `"Mapping ECL_PEMBENTUKAN_UAT_002 adalah regulated (6-eyes). Verifikasi MFA step-up diperlukan."`
   - USR-011 masukkan OTP dari authenticator
3. Klik **Konfirmasi**

**Hasil yang diharapkan Langkah 4:**
- Status = `PENDING_APPROVAL_2` (**bukan** APPROVED_ACTIVE!)
- Notifikasi dikirim ke USR-012: `"Mapping ECL_PEMBENTUKAN_UAT_002 menunggu persetujuan kedua Anda (6-eyes)."`
- Toast USR-011: `"Approve_1 berhasil. Menunggu second approval dari ROLE-RISK."`

**Langkah 5 — USR-012 approve_2 (ROLE-RISK, MFA step-up WAJIB)**

1. Login sebagai USR-012 (ROLE-RISK)
2. Buka **Notifikasi → Queue Approve-2**
3. Klik mapping `ECL_PEMBENTUKAN_UAT_002` → review detail
4. Klik **Setujui (Second Approval)**
5. Sistem meminta **MFA Step-up** (WAJIB per OQ-M2-1c dan DEC-027):
   - USR-012 masukkan OTP dari authenticator
6. Isi komentar: `"Template ECL sudah sesuai PSAK 71 §5.5.8. Disetujui."`
7. Klik **Konfirmasi**

**Hasil yang diharapkan Langkah 5:**
- Status = `APPROVED_ACTIVE`
- `aktif_flag = true`
- `approver_2_id = USR-012`
- `approver_2_signed_at` terisi timestamp saat ini
- `approver_2_signature_hash` non-empty (SHA-256)
- Toast USR-012: `"Mapping ECL_PEMBENTUKAN_UAT_002 berhasil disetujui (6-eyes selesai). Template aktif."`

### Verifikasi DB TC-002

```sql
SELECT
    workflow_status,
    aktif_flag,
    approver_id,
    approver_2_id,
    approver_2_signed_at,
    LENGTH(approver_2_signature_hash) AS sig_hash_len
FROM mst.mapping_jurnal_header
WHERE event_code = 'ECL_PEMBENTUKAN_UAT_002';
-- Expected: APPROVED_ACTIVE, true, non-null approver_2_id, sig_hash_len = 64 (SHA-256 hex)
```

```sql
SELECT action, actor_user_id
FROM aud.audit_log
WHERE entity_id = (SELECT id FROM mst.mapping_jurnal_header WHERE event_code='ECL_PEMBENTUKAN_UAT_002')
ORDER BY event_time;
-- Expected sequence: CREATE, SUBMIT, REVIEW, APPROVE, APPROVE_2 (5 baris)
```

### Sign-off TC-002

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-003 — SoD Violation Negative (4-eyes dan 6-eyes)

**Story**: P5-M2-S1 (AC: SoD violation — maker coba jadi approver)
**Aktor**: USR-010 (mencoba jadi reviewer/approver sendiri)
**Waktu Estimasi**: 10 menit

### Pre-Kondisi TC-003

- Mapping `JATUH_TEMPO_SOD_TEST` dalam status `PENDING_APPROVAL`, maker_id = USR-010

### Langkah-langkah TC-003

**Path A — 4-eyes: Maker mencoba approve mapping sendiri**

1. Login sebagai USR-010 (yang merupakan maker dari mapping `JATUH_TEMPO_SOD_TEST`)
2. Buka detail mapping `JATUH_TEMPO_SOD_TEST`
3. Coba klik tombol **Setujui** (meski status PENDING_APPROVAL)
4. Atau panggil API langsung: `POST /api/v1/mapping-jurnal/{id}/approve` dengan JWT USR-010

**Hasil yang diharapkan:**
- HTTP 403: `{"error": {"code": "JURNAL_SOD_VIOLATION", "message": "Anda tidak bisa menjadi approver untuk mapping yang Anda buat sendiri (DEC-017)."}}`
- Status mapping tetap `PENDING_APPROVAL`
- Tombol Approve di UI ter-disable untuk USR-010 (jika UI mengimplementasikan cek sisi klien)

**Path B — 6-eyes: User yang sama mencoba jadi approver_2 setelah approve_1**

1. Buat mapping regulated baru dengan USR-010 sebagai maker
2. USR-011 review + approve_1 → status = PENDING_APPROVAL_2
3. USR-011 coba klik **Setujui (Second Approval)** (ROLE-RISK context)
4. Atau panggil: `POST /api/v1/mapping-jurnal/{id}/approve-2` dengan JWT USR-011

**Hasil yang diharapkan Path B:**
- HTTP 403: `{"error": {"code": "JURNAL_SOD_VIOLATION", ...}}`
- Status tetap `PENDING_APPROVAL_2`

### Verifikasi DB TC-003

```sql
-- Audit harus mencatat SoD violation attempt
SELECT action, actor_user_id, after_jsonb->'sod_violation' AS sod_detail
FROM aud.audit_log
WHERE action = 'JURNAL_MAPPING.SOD_VIOLATION_ATTEMPT'
  AND entity_id = (SELECT id FROM mst.mapping_jurnal_header WHERE event_code = 'JATUH_TEMPO_SOD_TEST');
-- Expected: 1 baris dengan sod_violation detail
```

### Sign-off TC-003

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-004 — Step-up MFA Negative (Stale Token)

**Story**: P5-M2-S1 (AC: MFA step-up stale → 403)
**Aktor**: USR-012 (ROLE-RISK dengan step-up token > 5 menit)
**Waktu Estimasi**: 10 menit

### Pre-Kondisi TC-004

- Mapping regulated dalam status `PENDING_APPROVAL_2`
- USR-012 telah login, TETAPI tidak melakukan step-up MFA (atau step-up dilakukan > 5 menit yang lalu)

### Langkah-langkah TC-004

1. Login sebagai USR-012 tanpa melakukan step-up MFA
2. Tunggu > 5 menit setelah login (atau gunakan token yang sudah stale)
3. Buka mapping regulated yang sudah di-PENDING_APPROVAL_2
4. Klik **Setujui (Second Approval)**
5. Masukkan komentar approval, klik Konfirmasi **tanpa** OTP step-up

**Hasil yang diharapkan:**
- Sistem memblok: dialog MFA step-up muncul
- Jika dipanggil lewat API tanpa `X-Step-Up-Token` valid:
  ```json
  HTTP 403
  {"error": {"code": "JURNAL_STEP_UP_REQUIRED",
   "message": "approve_2 memerlukan MFA step-up yang masih valid (≤ 5 menit, DEC-027)"}}
  ```
- Status mapping tetap `PENDING_APPROVAL_2`

**Verifikasi positif** (step-up fresh):
1. USR-012 lakukan step-up MFA sekarang (OTP valid)
2. Langsung klik approve_2 dalam 5 menit
3. Harus berhasil → APPROVED_ACTIVE

### Sign-off TC-004

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-005 — Resolver Klasifikasi Compatibility Matrix (Semua 27 Codes Summary)

**Story**: P5-M2-S2 (AC: Resolver menyelesaikan event + AC: klasifikasi mismatch)
**Aktor**: Sistem (via API test — ROLE-AKUN-CTL atau ROLE-RISK dengan permission jurnal.preview)
**Waktu Estimasi**: 30 menit

### Pre-Kondisi TC-005

- 27 mapping APPROVED dari migration 000035 tersedia
- Instrumen dengan klasifikasi AC, FVOCI, FVTPL, FVOCI_ELECTION, POCI masing-masing tersedia

### Langkah-langkah TC-005

**A — Resolver preview happy path (beberapa event codes)**

Kirim `POST /api/v1/jurnal/resolve` untuk setiap kombinasi:

| Event Code | Klasifikasi | Amount IDR | Ekspektasi |
|---|---|---|---|
| `PENEMPATAN` | `AC` | 5.000.000.000 | 2 lines, debit=kredit=5Bn |
| `PENEMPATAN` | `FVOCI` | 5.000.000.000 | 2 lines, balanced |
| `AKRUAL_BUNGA` | `AC` | 25.000.000 | 2 lines, balanced |
| `ECL_PEMBENTUKAN` | `AC` | 1.500.000 | 2 lines, balanced |
| `ECL_PEMBENTUKAN` | `FVOCI` | 1.500.000 | 2 lines, balanced |
| `MTM_FVOCI` | `FVOCI` | 50.000.000 | 2 lines, balanced |
| `JATUH_TEMPO` | `AC` | 5.025.000.000 | 2 lines, balanced |

**Verifikasi setiap response:**
```json
{
  "data": {
    "lines": [
      {"urutan": 1, "posisi": "DEBIT", "akun_id": "...", "amount_idr": 5000000000.0000},
      {"urutan": 2, "posisi": "KREDIT", "akun_id": "...", "amount_idr": 5000000000.0000}
    ],
    "is_balanced": true,
    "total_debit": 5000000000.0000,
    "total_kredit": 5000000000.0000
  }
}
```

**B — Resolver klasifikasi tidak eligible**

```
POST /api/v1/jurnal/resolve
{
  "event_code": "MTM_FVOCI",
  "klasifikasi_psak71": "FVTPL",
  "amount_idr": 1000000
}
```

**Hasil yang diharapkan B:**
```json
HTTP 422
{"error": {"code": "JURNAL_KLASIFIKASI_NOT_ELIGIBLE",
  "message": "Event code 'MTM_FVOCI' tidak berlaku untuk klasifikasi PSAK 71 'FVTPL'..."}}
```

**C — Resolver event not mapped**

```
POST /api/v1/jurnal/resolve
{"event_code": "UNKNOWN_EVENT_XYZ", "klasifikasi_psak71": "AC", "amount_idr": 100000}
```

**Hasil yang diharapkan C:**
```json
HTTP 422
{"error": {"code": "JURNAL_EVENT_NOT_MAPPED",
  "message": "Tidak ada mapping jurnal APPROVED untuk event code 'UNKNOWN_EVENT_XYZ'..."}}
```

### Verifikasi Coverage TC-005

```sql
-- Pastikan semua 27 event codes memiliki mapping APPROVED + aktif
SELECT COUNT(*) FROM mst.mapping_jurnal_header
WHERE workflow_status = 'APPROVED_ACTIVE' AND aktif_flag = true;
-- Expected: >= 27
```

### Sign-off TC-005

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-006 — End-to-End penempatan:approved → Jurnal Terposting

**Story**: P5-M2-S3 (AC: Worker menerima event penempatan:approved dan memposting jurnal)
**Aktor**: Sistem (Asynq worker), USR-014 (ROLE-AUDIT untuk verifikasi)
**Waktu Estimasi**: 15 menit

### Pre-Kondisi TC-006

- Mapping `PENEMPATAN` status APPROVED_ACTIVE, aktif_flag=true
- Penempatan deposito `PNP-202606-000001` dalam status APPROVED_ACTIVE
- Periode PBUKU-2026-06 status OPEN
- Asynq worker berjalan (atau trigger manual via admin UI)

### Langkah-langkah TC-006

**Langkah 1 — Trigger event penempatan:approved**

Cara A (via workflow UAT TC-006 terhubung dengan penempatan yang sudah di-approve):
1. Jika penempatan `PNP-202606-000001` sudah APPROVED_ACTIVE, event seharusnya sudah di-enqueue
2. Pantau Asynq dashboard di `/admin/jobs`
3. Tunggu event `penempatan:approved` diproses (biasanya < 5 detik)

Cara B (trigger manual via test endpoint — UAT only):
```
POST /api/v1/admin/test/trigger-event (UAT environment only)
{"event_type": "penempatan:approved", "penempatan_id": "{PNP-202606-000001-uuid}"}
```

**Langkah 2 — Verifikasi jurnal terposting**

1. Login sebagai USR-014 (ROLE-AUDIT)
2. Buka **Jurnal → List Jurnal**
3. Filter: Periode=PBUKU-2026-06, Event Code=PENEMPATAN
4. Temukan jurnal dengan `reference_event_type = 'penempatan_deposito'` dan `reference_event_id = PNP-202606-000001-uuid`

**Hasil yang diharapkan:**
- `jrnl.header` berhasil ter-INSERT
- `no_jurnal` = `JRN-2026-000001` (atau sequential berikutnya)
- `total_debit = total_kredit = 5.000.000.000,00` (IDR)
- `status_internal = 'POSTED'`
- `jrnl.detail`: 2 baris (DEBIT Deposito + KREDIT Kas)
- `CHECK CONSTRAINT ck_jrnl_balance PASS` (total_debit = total_kredit di DB)

### Verifikasi DB TC-006

```sql
SELECT
    h.no_jurnal,
    h.event_code,
    h.total_debit,
    h.total_kredit,
    h.status_internal,
    h.reference_event_type,
    (SELECT COUNT(*) FROM jrnl.detail d WHERE d.header_id = h.id) AS detail_count
FROM jrnl.header h
WHERE h.reference_event_id = '{PNP-202606-000001-uuid}'
  AND h.event_code = 'PENEMPATAN';
-- Expected: 1 row, total_debit=5000000000, total_kredit=5000000000, detail_count=2
```

```sql
-- Verifikasi audit JURNAL.POST in-transaction
SELECT action, event_time, trace_id
FROM aud.audit_log
WHERE entity_type = 'jrnl.header'
  AND entity_id = (
    SELECT id FROM jrnl.header
    WHERE reference_event_id = '{PNP-202606-000001-uuid}'
  )
  AND action = 'JURNAL.POST';
-- Expected: 1 baris
```

```sql
-- Verifikasi idempotency key stored
SELECT idempotency_key FROM jrnl.header
WHERE reference_event_id = '{PNP-202606-000001-uuid}';
-- Expected: SHA256 hex string (64 chars)
```

### Sign-off TC-006

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-007 — Periode HARD_CLOSED Guard → DLQ

**Story**: P5-M2-S3 (AC: Worker mencoba posting ke periode HARD_CLOSED → masuk DLQ)
**Aktor**: Sistem (Asynq worker), USR-013 (ROLE-IT-ADMIN untuk DLQ monitoring)
**Waktu Estimasi**: 15 menit

### Pre-Kondisi TC-007

- Periode PBUKU-2026-05 (Mei 2026) status = `HARD_CLOSED`
- Ada penempatan yang masuk dengan `periode_id = PBUKU-2026-05`
- Mapping `PENEMPATAN` masih APPROVED (tapi periode target closed)

### Langkah-langkah TC-007

**Langkah 1 — Trigger event ke periode HARD_CLOSED**

Via test endpoint:
```
POST /api/v1/admin/test/trigger-event (UAT only)
{
  "event_type": "penempatan:approved",
  "event_code": "PENEMPATAN",
  "periode_id": "{PBUKU-2026-05-uuid}",
  "amount_idr": 1000000000
}
```

**Langkah 2 — Verifikasi DLQ entry terbuat**

1. Login sebagai USR-013 (ROLE-IT-ADMIN)
2. Buka **DLQ Jurnal → List DLQ** (`/jurnal/dlq`)
3. Filter: Status=FAILED, Event Code=PENEMPATAN

**Hasil yang diharapkan:**
- Tampil DLQ entry baru dengan:
  - `error_code`: `JURNAL_PERIODE_HARD_CLOSED`
  - `error_category`: `DOMAIN`
  - `status`: `FAILED`
  - `attempt_count`: 1
- **TIDAK ADA** `jrnl.header` baru ter-INSERT

**Langkah 3 — Verifikasi tidak ada retry**

- Di Asynq dashboard, task tidak masuk ke queue retry
- `attempt_count` di DLQ tetap 1 (bukan bertambah)

### Verifikasi DB TC-007

```sql
SELECT
    d.error_code,
    d.error_message,
    d.error_category,
    d.status,
    d.attempt_count
FROM sys.dlq_jurnal_post d
WHERE d.event_code = 'PENEMPATAN'
  AND d.status = 'FAILED'
ORDER BY d.created_at DESC
LIMIT 1;
-- Expected: error_code='JURNAL_PERIODE_HARD_CLOSED', error_category='DOMAIN', attempt_count=1
```

```sql
-- Tidak ada jrnl.header untuk periode HARD_CLOSED
SELECT COUNT(*) FROM jrnl.header
WHERE periode_id = '{PBUKU-2026-05-uuid}';
-- Expected: 0 (atau hanya yang sudah ada sebelum test)
```

```sql
-- Audit JURNAL.POST_FAILED harus ada
SELECT action FROM aud.audit_log
WHERE entity_id = (
    SELECT id FROM sys.dlq_jurnal_post
    WHERE error_code = 'JURNAL_PERIODE_HARD_CLOSED'
    ORDER BY created_at DESC LIMIT 1
)
AND action = 'JURNAL.POST_FAILED';
-- Expected: 1 baris
```

### Sign-off TC-007

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-008 — DLQ Replay Flow (ROLE-AKUN-CTL)

**Story**: P5-M2-S6 (AC: DLQ entry di-replay setelah mapping diperbaiki)
**Aktor**: USR-011 (ROLE-AKUN-CTL), Sistem (Asynq worker)
**Waktu Estimasi**: 20 menit

### Pre-Kondisi TC-008

- Ada DLQ entry dengan `error_code = 'JURNAL_EVENT_NOT_MAPPED'` (event code belum ter-mapping)
- Mapping untuk event code tersebut sudah di-approve setelah DLQ masuk
- Periode target status OPEN

### Langkah-langkah TC-008

**Langkah 1 — Verifikasi DLQ entry**

1. Login sebagai USR-011 (ROLE-AKUN-CTL)
2. Buka **DLQ Jurnal** (`/jurnal/dlq`)
3. Temukan entry DLQ-001 dengan `status=FAILED`, `error_code=JURNAL_EVENT_NOT_MAPPED`

**Langkah 2 — Trigger replay**

1. Klik tombol **Replay** pada DLQ-001
2. Konfirmasi dialog: `"Replay DLQ-001? Worker akan mencoba posting ulang."`
3. Klik **Ya, Replay**

**Hasil yang diharapkan setelah klik replay:**
- Toast: `"Replay DLQ-001 sedang diproses. Jurnal akan diposting dalam beberapa detik."`
- Status DLQ-001 berubah ke `REPLAYING`
- JobProgressPanel tampil dengan progress bar

**Langkah 3 — Verifikasi replay berhasil**

Setelah worker selesai (biasanya < 5 detik):
- Toast sukses: `"Replay DLQ-001 berhasil. Jurnal JRN-2026-{NNNNNN} diposting."`
- Status DLQ-001 = `REPLAYED_OK`
- `replayed_jurnal_id` terisi (UUID jrnl.header baru)
- `replayed_by` = USR-011
- `replayed_at` = timestamp saat ini

**Langkah 4 — Verifikasi tidak ada duplikat**

Jika replay dipicu dua kali dengan DLQ-001 yang sama:
- Kedua replay menghasilkan `jrnl.header` yang sama (idempotency via unique index `uq_jrnl_idempotency`)
- Tidak ada `jrnl.header` duplikat

### Verifikasi DB TC-008

```sql
SELECT
    d.status,
    d.replayed_jurnal_id,
    d.replayed_by,
    d.replayed_at,
    d.attempt_count
FROM sys.dlq_jurnal_post d
WHERE d.id = '{DLQ-001-uuid}';
-- Expected: status='REPLAYED_OK', replayed_jurnal_id non-null, replayed_by=USR-011
```

```sql
-- Audit JURNAL.DLQ_REPLAYED harus ada (SEBELUM Asynq task di-enqueue)
SELECT action, actor_user_id, event_time
FROM aud.audit_log
WHERE entity_id = '{DLQ-001-uuid}'
  AND action = 'JURNAL.DLQ_REPLAYED';
-- Expected: 1 baris, actor_user_id = USR-011
```

```sql
-- Jurnal terposting (dari replay)
SELECT h.no_jurnal, h.total_debit, h.total_kredit, h.status_internal
FROM jrnl.header h
WHERE h.id = (
    SELECT replayed_jurnal_id FROM sys.dlq_jurnal_post WHERE id = '{DLQ-001-uuid}'
);
-- Expected: status_internal='POSTED', total_debit=total_kredit
```

### Sign-off TC-008

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## TC-009 — DLQ Discard Flow dengan Validasi Reason

**Story**: P5-M2-S6 (AC: ROLE-IT-ADMIN mengabaikan DLQ entry)
**Aktor**: USR-013 (ROLE-IT-ADMIN)
**Waktu Estimasi**: 10 menit

### Pre-Kondisi TC-009

- Ada DLQ entry DLQ-003 dengan status `FAILED` (mis. JURNAL_BALANCE_FAILED dari template yang sudah diganti)
- USR-013 memiliki permission `jurnal.dlq.abandon`

### Langkah-langkah TC-009

**Path A — Reason terlalu pendek (negative)**

1. Login sebagai USR-013 (ROLE-IT-ADMIN)
2. Buka DLQ list, klik **Abandon** pada DLQ-003
3. Dialog: isi reason `"Terlalu pendek"` (< 30 karakter)
4. Klik **Konfirmasi**

**Hasil yang diharapkan Path A:**
- Error inline: `"Alasan abandon minimal 30 karakter. Saat ini: 13 karakter."`
- Status DLQ tetap `FAILED`

**Path B — Discard valid (positive)**

1. Isi reason: `"Entry duplikat dengan DLQ-003-REV. Manual review menunjukkan balance sudah benar di template versi baru yang sudah di-approve."`
2. Klik **Konfirmasi Abandon**

**Hasil yang diharapkan Path B:**
- Toast: `"DLQ-003 berhasil di-abandon. Entry tidak akan diproses ulang."`
- Status DLQ-003 = `ABANDONED`
- Entry tidak muncul di list default (filter FAILED/REPLAYING)
- Akses via `?filter[status]=ABANDONED` masih bisa untuk audit trail

### Verifikasi DB TC-009

```sql
SELECT status, discarded_reason FROM sys.dlq_jurnal_post
WHERE id = '{DLQ-003-uuid}';
-- Expected: status='ABANDONED', LENGTH(discarded_reason) >= 30
```

```sql
-- Audit JURNAL.DLQ_DISCARD harus ada dengan reason
SELECT action, after_jsonb->>'reason' AS reason, actor_user_id
FROM aud.audit_log
WHERE entity_id = '{DLQ-003-uuid}'
  AND action = 'JURNAL.DLQ_DISCARD';
-- Expected: 1 baris, reason non-empty, actor=USR-013
```

### Sign-off TC-009

| Penguji | Hasil | Catatan |
|---|---|---|
| | PASS / FAIL | |

---

## Rollback & Cleanup

Setelah seluruh UAT selesai:

```sql
-- Hapus mapping UAT (soft-delete)
UPDATE mst.mapping_jurnal_header
SET deleted_at = now(), deleted_by = (SELECT id FROM sec.user WHERE username = 'admin-uat')
WHERE event_code IN ('PENEMPATAN_UAT_001', 'ECL_PEMBENTUKAN_UAT_002');

-- Hapus DLQ entries test
DELETE FROM sys.dlq_jurnal_post WHERE created_at > '2026-06-15 00:00:00+07';

-- Catatan: jrnl.header TIDAK boleh dihapus (append-only — aud/jrnl hard rules DEC-018)
-- Jika jurnal test perlu dibersihkan, gunakan REVERSE entry via event CORRECTION_PERIODE_CLOSED
```

---

## Matriks Coverage AC vs TC

| Story | Acceptance Criteria | TC | Layer |
|---|---|---|---|
| P5-M2-S1 | 4-eyes operational PENEMPATAN happy | TC-001 | E2E + UAT |
| P5-M2-S1 | 6-eyes regulated ECL_PEMBENTUKAN happy | TC-002 | E2E + UAT |
| P5-M2-S1 | SoD violation maker coba approve | TC-003 | E2E + UAT |
| P5-M2-S1 | Regulated mapping di-approve tanpa ROLE-RISK | TC-003 (Path B) | E2E |
| P5-M2-S1 | Balance detail rows tidak simetris | P5-M2-L (E2E) | E2E |
| P5-M2-S1 | Non-aktifkan mapping APPROVED | (in TC-001 teardown) | UAT |
| P5-M2-S2 | Resolver PENEMPATAN AC balanced | TC-005-A | E2E (P5-M2-E) + UAT |
| P5-M2-S2 | Resolver MTM_FVOCI FVOCI klasifikasi filter | TC-005-A | E2E (P5-M2-E) |
| P5-M2-S2 | Resolver event_code tidak ada | TC-005-C | E2E (P5-M2-G) + UAT |
| P5-M2-S2 | Resolver klasifikasi mismatch | TC-005-B | E2E (P5-M2-F) + UAT |
| P5-M2-S2 | Resolver imbalance template | P5-M2-L (E2E) | E2E |
| P5-M2-S3 | penempatan:approved → jurnal POSTED | TC-006 | E2E (P5-M2-H) + UAT |
| P5-M2-S3 | Idempotency replay no double-post | TC-006 (step 4) | E2E (P5-M2-M) + UAT |
| P5-M2-S3 | Periode HARD_CLOSED → DLQ | TC-007 | E2E (P5-M2-I) + UAT |
| P5-M2-S3 | event_code not mapped → DLQ | P5-M2-N (E2E) | E2E |
| P5-M2-S4 | Step-up MFA stale on approve_2 | TC-004 | E2E (P5-M2-D) + UAT |
| P5-M2-S6 | DLQ list + filter | TC-007, TC-008 | UAT |
| P5-M2-S6 | DLQ replay sukses | TC-008 | E2E (P5-M2-J) + UAT |
| P5-M2-S6 | DLQ discard + reason validation | TC-009 | E2E (P5-M2-K) + UAT |
| P5-M2-S6 | Domain err → DLQ immediate no retry | P5-M2-N (E2E) | E2E |
| P5-M2-S6 | Infra err → 3x retry then DLQ | P5-M2-O (E2E) | E2E |

---

## Sign-off Final UAT

| No | TC | Status | Penguji | Tanggal |
|---|---|---|---|---|
| 1 | TC-001 | | | |
| 2 | TC-002 | | | |
| 3 | TC-003 | | | |
| 4 | TC-004 | | | |
| 5 | TC-005 | | | |
| 6 | TC-006 | | | |
| 7 | TC-007 | | | |
| 8 | TC-008 | | | |
| 9 | TC-009 | | | |

**UAT Disetujui oleh**: _________________________ (Kepala Akuntansi / ROLE-AKUN-CTL Senior)

**Tanggal**: _________________________
