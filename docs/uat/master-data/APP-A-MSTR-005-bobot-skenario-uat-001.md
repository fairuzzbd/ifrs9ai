# UAT Script — APP-A-MSTR-005: Bobot Skenario ECL Parameter
**ID UAT**: UAT-APP-A-MSTR-005-001
**Story**: APP-A-MSTR-005 Master Bobot Skenario (DEC-010 Scenario Weights, 6-Eyes ECL Parameter)
**Modul**: APP-A Master Data Management — ECL Parameter
**Tanggal**: 2026-06-03
**Versi**: 1.0
**Author**: qa-engineer
**Status**: READY FOR UAT

---

## Konteks Domain

Bobot Skenario mengatur tiga faktor pembobotan (GOOD / NORMAL / BAD) yang digunakan dalam formula ECL:

```
ECL_weighted = ECL_FL_Good × W_Good + ECL_FL_Normal × W_Normal + ECL_FL_Bad × W_Bad
```

Per DEC-010, nilai default adalah W_Good=0.25, W_Normal=0.50, W_Bad=0.25. Seluruh perhitungan menggunakan `shopspring/decimal` — bukan `float64`. Workflow: **6-eyes** (DRAFT → PENDING_REVIEW → PENDING_APPROVAL → PENDING_APPROVAL_2 → APPROVED). Kedua langkah Approve membutuhkan step-up MFA.

**Invariant kritis (DEC-010):** W_Good + W_Normal + W_Bad HARUS = 1.0 (toleransi 0.00000001). Invariant ini diperiksa saat approve final (approve2), bukan saat create.

---

## Pre-conditions

### Infrastruktur
- BLIPS dev/UAT stack berjalan: `docker compose -f deploy/docker/docker-compose.dev.yml up -d`
- Database dimigrasikan: `go run ./cmd/migrator up` (harus sudah ada migration 0011)
- Backend API berjalan di `http://localhost:8080`
- Frontend berjalan di `http://localhost:3001`

### Persona Test (4 user, di-setup di Keycloak)

| Username | Role | Deskripsi | MFA Wajib |
|---|---|---|---|
| `risk.officer.1` | ROLE-RISK | RISK Officer — Maker & Reviewer untuk ECL param | Tidak |
| `risk.officer.2` | ROLE-RISK | RISK Officer kedua untuk review (SoD) | Tidak |
| `alco.member.1` | ROLE-ALCO | ALCO — Approver1 (step-up MFA) | YA |
| `alco.member.2` | ROLE-ALCO | ALCO — Approver2 (step-up MFA, beda dari alco.1) | YA |

Permissions `ecl_parameter.*` dimiliki oleh semua persona di atas sesuai level.

### Seed Data SQL (jalankan sebelum UAT jika belum ada)

```sql
-- Pastikan tidak ada data test yang konflik dari periode yang dipakai UAT (2026-Q3).
-- Periode test: 2026-07-01 (TC-001 s.d. TC-009)
DELETE FROM sys.workflow_instance
WHERE entity_id IN (
    SELECT id FROM mst.bobot_skenario
    WHERE periode_berlaku_dari = '2026-07-01'
      AND tenant_id = 'TUGURE'
);
DELETE FROM mst.bobot_skenario
WHERE periode_berlaku_dari = '2026-07-01'
  AND tenant_id = 'TUGURE';
```

---

## SKENARIO UAT

---

### TC-001: Seed Default 2026-Q3 — Generate Trio G/N/B = 0.25/0.50/0.25 DRAFT

**Aktor**: risk.officer.1 (ROLE-RISK)
**Pre-condition**: Tidak ada baris bobot_skenario untuk periode 2026-07-01
**Tujuan**: Verifikasi bahwa seed-default menghasilkan trio DEC-010 lengkap dalam sekali klik.

**Langkah-langkah**:

1. Login sebagai `risk.officer.1` di `http://localhost:3001`
2. Navigasi ke **Master Data > ECL Parameter > Bobot Skenario** (`/master/bobot-skenario`)
3. Klik tombol **"Seed Default Periode"**
4. Di dialog seed, isi:
   - **Periode Berlaku Dari**: `2026-07-01`
5. Klik **"Generate"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 201 dari API, response `created: 3, skipped: false` | |
| 2 | 3 baris DRAFT muncul di list: GOOD=0.25000000, NORMAL=0.50000000, BAD=0.25000000 | |
| 3 | Banner summary trio tampil di list page: **"Sum 1.00 ✓"** (warna hijau) | |
| 4 | Toast sukses: *"3 bobot skenario periode 2026-07-01 berhasil dibuat (GOOD=0.25, NORMAL=0.50, BAD=0.25). Menunggu approval."* | |
| 5 | Semua 3 baris memiliki `workflow_status = DRAFT` | |
| 6 | `aud.audit_log` mengandung 3 event `BOBOT_SKENARIO.CREATE` untuk periode ini | |

**Verifikasi Audit SQL**:
```sql
SELECT skenario, bobot, workflow_status
FROM mst.bobot_skenario
WHERE periode_berlaku_dari = '2026-07-01'
  AND tenant_id = 'TUGURE'
  AND deleted_at IS NULL
ORDER BY skenario;
-- Expected: 3 rows: BAD=0.25000000 DRAFT, GOOD=0.25000000 DRAFT, NORMAL=0.50000000 DRAFT

SELECT COUNT(*) FROM aud.audit_log
WHERE action = 'BOBOT_SKENARIO.CREATE'
  AND event_time > now() - interval '5 minutes';
-- Expected: >= 3
```

**Catatan (Idempotency)**: Klik "Generate" kedua kali untuk periode yang sama → response `skipped: true`, tidak ada baris baru, toast: *"Bobot skenario periode 2026-07-01 sudah ada (3 baris), tidak ada perubahan."*

---

### TC-002: ALCO Override — Edit Bobot, Submit-Review-Approve Trio

**Aktor**: risk.officer.1 (Maker), risk.officer.2 (Reviewer), alco.member.1 (Approver1), alco.member.2 (Approver2)
**Pre-condition**: Trio DRAFT dari TC-001 sudah ada (GOOD=0.25, NORMAL=0.50, BAD=0.25)
**Tujuan**: ALCO mengubah bobot (sum tetap 1.0), kemudian mengikuti full 6-eyes workflow.

**Langkah-langkah Maker (risk.officer.1)**:

1. Buka list Bobot Skenario, cari baris **GOOD** periode 2026-07-01
2. Klik **"Edit"** pada baris GOOD
3. Ubah **Bobot** dari `0.25000000` menjadi `0.20000000`
4. Klik **"Simpan"**

5. Lakukan hal yang sama untuk baris **NORMAL**: ubah menjadi `0.60000000`
6. Baris **BAD** tetap `0.20000000`

   *Catatan: sum = 0.20 + 0.60 + 0.20 = 1.00 — invariant terpenuhi*

7. Klik **"Kirim untuk Review"** pada baris GOOD (atau submit trio sekaligus jika ada fitur batch submit)

**Hasil yang Diharapkan — Maker**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Edit GOOD berhasil: `bobot = 0.20000000` tersimpan | |
| 2 | Edit NORMAL berhasil: `bobot = 0.60000000` tersimpan | |
| 3 | BAD tetap `0.20000000` | |
| 4 | Banner sum di list terupdate: **"Sum 1.00 ✓"** | |
| 5 | Setelah submit, status GOOD berubah ke `PENDING_REVIEW` | |

**Langkah-langkah Reviewer (risk.officer.2)**:

1. Login sebagai `risk.officer.2`
2. Navigasi ke queue review atau buka detail GOOD
3. Verifikasi bobot GOOD = `0.20000000`
4. Isi komentar: `"Override ALCO sesuai rapat 2026-06-03, sum = 1.0 terkonfirmasi"`
5. Klik **"Setujui Review"**

**Langkah-langkah Approver1 (alco.member.1) — Step-up MFA**:

1. Login sebagai `alco.member.1`
2. Buka detail GOOD (status: PENDING_APPROVAL)
3. Klik **"Approve"**
4. Sistem meminta konfirmasi MFA step-up
5. Masukkan kode TOTP/Push
6. Konfirmasi

**Langkah-langkah Approver2 (alco.member.2) — Step-up MFA**:

1. Login sebagai `alco.member.2` (beda dari alco.member.1)
2. Buka detail GOOD (status: PENDING_APPROVAL_2)
3. Klik **"Final Approve"**
4. Sistem meminta konfirmasi MFA step-up
5. Masukkan kode TOTP/Push
6. Konfirmasi

**Hasil yang Diharapkan — Full Cycle**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | GOOD berubah ke `APPROVED` | |
| 2 | `mst.bobot_skenario.workflow_status = 'APPROVED'` di DB | |
| 3 | Toast Approver2: *"Bobot Skenario GOOD periode 2026-07-01 berhasil disetujui."* | |
| 4 | 4 signature records di `sys.workflow_signature` (submit + review + approve + approve2) | |
| 5 | Audit log memiliki `BOBOT_SKENARIO.APPROVE` event dengan `actor_user_id = alco.member.2.id` | |

**Verifikasi SQL**:
```sql
-- Cek bobot tersimpan dengan presisi NUMERIC(10,8).
SELECT skenario, bobot, workflow_status FROM mst.bobot_skenario
WHERE periode_berlaku_dari = '2026-07-01' AND tenant_id = 'TUGURE' AND deleted_at IS NULL
ORDER BY skenario;
-- Expected: BAD=0.20000000 DRAFT, GOOD=0.20000000 APPROVED, NORMAL=0.60000000 DRAFT

-- Cek 4 signatures untuk GOOD entity.
SELECT ws.step, ws.signed_at, ws.signer_user_id
FROM sys.workflow_signature ws
JOIN sys.workflow_instance wi ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = (
    SELECT id FROM mst.bobot_skenario
    WHERE skenario = 'GOOD' AND periode_berlaku_dari = '2026-07-01'
    LIMIT 1
)
ORDER BY ws.signed_at;
-- Expected: 4 rows (submit, review, approve, approve2)
```

---

### TC-003: Sum Invariant Violation — Approve Ditolak Karena Sum ≠ 1.0

**Aktor**: risk.officer.1 (Maker), risk.officer.2 (Reviewer), alco.member.1 (Approver1), alco.member.2 (Approver2)
**Pre-condition**: Trio DRAFT untuk periode baru (mis. 2026-08-01)
**Tujuan**: Verifikasi bahwa approve2 menolak jika G+N+B ≠ 1.0.

**Setup (jalankan SQL)**:
```sql
-- Buat trio dengan sum=1.15 (sengaja salah).
INSERT INTO mst.bobot_skenario (id, skenario, bobot, periode_berlaku_dari, maker_id, workflow_status, row_version, tenant_id, created_at, created_by, updated_at, updated_by)
VALUES
    (gen_random_uuid(), 'GOOD',   0.40, '2026-08-01', '00000000-0000-0000-0000-000000000001', 'DRAFT', 1, 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001'),
    (gen_random_uuid(), 'NORMAL', 0.50, '2026-08-01', '00000000-0000-0000-0000-000000000001', 'DRAFT', 1, 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001'),
    (gen_random_uuid(), 'BAD',    0.25, '2026-08-01', '00000000-0000-0000-0000-000000000001', 'DRAFT', 1, 'TUGURE', now(), '00000000-0000-0000-0000-000000000001', now(), '00000000-0000-0000-0000-000000000001');
-- sum = 0.40 + 0.50 + 0.25 = 1.15
```

**Langkah-langkah**:

1. Login sebagai `risk.officer.1`
2. Submit baris GOOD periode 2026-08-01
3. Login sebagai `risk.officer.2`, review GOOD → setujui
4. Login sebagai `alco.member.1`, approve (step-up MFA) → berhasil masuk PENDING_APPROVAL_2
5. Login sebagai `alco.member.2`, klik **"Final Approve"** (step-up MFA)

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Langkah 1–4 berhasil tanpa error | |
| 2 | Langkah 5 (approve2): **HTTP 422** diterima | |
| 3 | Toast error merah **persistent**: *"Total bobot G+N+B periode 2026-08-01 harus = 1.0 (Lebih dari 1.0: saat ini 1.15000000). Periksa dan koreksi bobot."* | |
| 4 | Toast mengandung traceId | |
| 5 | `mst.bobot_skenario.workflow_status` untuk GOOD tetap `PENDING_APPROVAL_2` (tidak berubah menjadi APPROVED) | |
| 6 | `sys.workflow_instance.current_state` tetap `PENDING_APPROVAL_2` | |
| 7 | Tidak ada audit event `BOBOT_SKENARIO.APPROVE` baru untuk entity ini | |

**Verifikasi SQL**:
```sql
SELECT current_state FROM sys.workflow_instance wi
JOIN mst.bobot_skenario bs ON wi.entity_id = bs.id
WHERE bs.skenario = 'GOOD' AND bs.periode_berlaku_dari = '2026-08-01';
-- Expected: PENDING_APPROVAL_2 (bukan APPROVED)
```

**Cleanup**:
```sql
DELETE FROM sys.workflow_instance WHERE entity_id IN (
    SELECT id FROM mst.bobot_skenario WHERE periode_berlaku_dari = '2026-08-01'
);
DELETE FROM mst.bobot_skenario WHERE periode_berlaku_dari = '2026-08-01';
```

---

### TC-004: 6-Eyes Happy Path — Full Cycle dengan 2 Step-Up MFA

**Aktor**: risk.officer.1, risk.officer.2, alco.member.1, alco.member.2
**Pre-condition**: Trio DRAFT default untuk periode 2026-09-01 (seed via TC-001 pattern)
**Tujuan**: Verifikasi seluruh transisi state dan MFA prompts.

**Langkah-langkah**:

1. Seed trio via Seed Default untuk periode `2026-09-01`
2. **SUBMIT** (risk.officer.1): submit baris GOOD periode 2026-09-01
   - Verifikasi: status = `PENDING_REVIEW`, timestamp `submitted_at` tersimpan
3. **REVIEW** (risk.officer.2): review dan setujui
   - Isi komentar: *"Review parameter sesuai kebijakan ALCO Q3 2026"*
   - Verifikasi: status = `PENDING_APPROVAL`
4. **APPROVE** (alco.member.1):
   - Sistem menampilkan prompt **"MFA Step-Up diperlukan"**
   - Masukkan OTP/Push
   - Klik Approve
   - Verifikasi: status = `PENDING_APPROVAL_2`
5. **APPROVE2** (alco.member.2, beda dari alco.member.1):
   - Sistem menampilkan prompt **"MFA Step-Up diperlukan"**
   - Masukkan OTP/Push
   - Klik Final Approve
   - Verifikasi: status = `APPROVED`

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | 5 state transitions berurutan: DRAFT → PR → PA → PA_2 → APPROVED | |
| 2 | 2 prompt MFA muncul (step 4 dan step 5) — tidak muncul di step 2 dan 3 | |
| 3 | Setelah APPROVED: panel workflow menampilkan semua 4 step dengan tanda centang hijau | |
| 4 | 4 records di `sys.workflow_signature` | |
| 5 | Tombol Edit tidak tersedia (APPROVED = read-only) | |
| 6 | `mst.bobot_skenario.workflow_status = 'APPROVED'` di DB | |

**Verifikasi SQL**:
```sql
SELECT wi.current_state, COUNT(ws.id) AS signature_count
FROM sys.workflow_instance wi
LEFT JOIN sys.workflow_signature ws ON ws.workflow_instance_id = wi.id
WHERE wi.entity_id = (
    SELECT id FROM mst.bobot_skenario
    WHERE skenario = 'GOOD' AND periode_berlaku_dari = '2026-09-01'
    LIMIT 1
)
GROUP BY wi.current_state;
-- Expected: current_state=APPROVED, signature_count=4
```

---

### TC-005: SoD — Approver2 Tidak Boleh Sama dengan Maker, Reviewer, atau Approver1

**Aktor**: risk.officer.1 (Maker + coba Approver2), alco.member.1 (juga Approver1)
**Pre-condition**: Trio DRAFT periode 2026-10-01, dibuat oleh risk.officer.1

**Skenario 5A — Maker Tidak Bisa Approve2**:

1. risk.officer.1 submit GOOD, risk.officer.2 review, alco.member.1 approve
2. risk.officer.1 mencoba approve2 via API langsung:

```bash
curl -X POST http://localhost:8080/api/v1/master/bobot-skenario/{GOOD_ID}/approve2 \
  -H "Authorization: Bearer <token_risk_officer_1>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"rowVersion":4,"signatureMethod":"JWT_STANDARD"}'
```

**Hasil yang Diharapkan 5A**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 SOD_VIOLATION | |
| 2 | `"error.code": "SOD_VIOLATION"` | |
| 3 | Status tetap PENDING_APPROVAL_2 | |

**Skenario 5B — Approver1 Tidak Bisa Approve2**:

```bash
curl -X POST http://localhost:8080/api/v1/master/bobot-skenario/{GOOD_ID}/approve2 \
  -H "Authorization: Bearer <token_alco_member_1>" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"rowVersion":4,"signatureMethod":"JWT_STANDARD"}'
```

**Hasil yang Diharapkan 5B**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 403 SOD_VIOLATION | |
| 2 | Status tetap PENDING_APPROVAL_2 | |

---

### TC-006: Duplicate Skenario Period — Buat 2 Baris GOOD Periode Sama

**Aktor**: risk.officer.1
**Pre-condition**: Sudah ada baris GOOD untuk periode 2026-07-01

**Langkah-langkah**:

1. Navigasi ke **+ Tambah Bobot Skenario**
2. Isi form:
   - **Skenario**: `GOOD`
   - **Bobot**: `0.20000000`
   - **Periode Berlaku Dari**: `2026-07-01`
3. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 diterima | |
| 2 | `"error.code": "BOBOT_DUPLICATE_SKENARIO_PERIOD"` | |
| 3 | Toast error: *"Bobot skenario GOOD untuk periode 2026-07-01 sudah ada. Gunakan Edit jika perlu mengubah nilai."* | |
| 4 | Tidak ada baris duplikat dibuat di DB | |

**Verifikasi SQL**:
```sql
SELECT COUNT(*) FROM mst.bobot_skenario
WHERE skenario = 'GOOD' AND periode_berlaku_dari = '2026-07-01'
  AND deleted_at IS NULL AND tenant_id = 'TUGURE';
-- Expected: 1 (bukan 2)
```

---

### TC-007: Period Overlap Same Skenario

**Aktor**: risk.officer.1
**Pre-condition**: Sudah ada baris GOOD dengan periode `2026-07-01 → 2026-12-31`

**Langkah-langkah**:

1. Tambah baris baru:
   - **Skenario**: `GOOD`
   - **Bobot**: `0.30000000`
   - **Periode Berlaku Dari**: `2026-07-01`
   - **Periode Berlaku Sampai**: `2026-09-30`
2. Klik **"Simpan"**

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | HTTP 422 diterima | |
| 2 | `"error.code"` adalah `BOBOT_PERIOD_OVERLAP` atau `BOBOT_DUPLICATE_SKENARIO_PERIOD` | |
| 3 | Toast error: *"Periode yang dimasukkan tumpang tindih dengan data GOOD yang sudah ada. Periksa periode berlaku."* | |
| 4 | Tidak ada baris baru dibuat | |

---

### TC-008: Reject Flow → RETURNED → Edit → Resubmit

**Aktor**: risk.officer.1 (Maker), risk.officer.2 (Reviewer yang menolak)
**Pre-condition**: Trio DRAFT periode 2026-11-01, submit sudah dilakukan oleh risk.officer.1

**Langkah-langkah**:

1. risk.officer.1 submit GOOD periode 2026-11-01 → status PENDING_REVIEW
2. Login sebagai `risk.officer.2` (bukan maker)
3. Buka GOOD, klik **"Tolak"**
4. Dialog konfirmasi muncul — isi alasan wajib: `"Bobot tidak sesuai dengan hasil backtesting Q2 2026. Harap konsultasi ALCO terlebih dahulu."`
5. Klik **"Lanjut Tolak"**

**Hasil yang Diharapkan — Setelah Reject**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Status GOOD berubah ke `REJECTED` (ditampilkan sebagai **RETURNED** di UI) | |
| 2 | Banner RETURNED tampil di halaman detail dengan alasan penolakan | |
| 3 | Tombol **"Edit"** tersedia kembali untuk risk.officer.1 | |
| 4 | Audit log: `BOBOT_SKENARIO.REJECT` dengan komentar | |

**Langkah-langkah Resubmit**:

6. Login sebagai `risk.officer.1`
7. Edit GOOD: ubah bobot menjadi `0.25000000`
8. Klik **"Simpan"**, lalu **"Kirim untuk Review"**

**Hasil yang Diharapkan — Setelah Resubmit**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 5 | Status berubah ke `PENDING_REVIEW` | |
| 6 | Komentar penolakan sebelumnya masih tersimpan di history | |
| 7 | Audit log: `BOBOT_SKENARIO.SUBMIT` (kedua) | |

**Verifikasi SQL**:
```sql
SELECT action, event_time FROM aud.audit_log
WHERE entity_id = (
    SELECT id FROM mst.bobot_skenario
    WHERE skenario = 'GOOD' AND periode_berlaku_dari = '2026-11-01'
    LIMIT 1
)
ORDER BY event_time;
-- Expected: BOBOT_SKENARIO.CREATE, BOBOT_SKENARIO.SUBMIT, BOBOT_SKENARIO.REJECT, BOBOT_SKENARIO.UPDATE, BOBOT_SKENARIO.SUBMIT
```

---

### TC-009: Trio Summary Banner — Verifikasi Sum di List Page

**Aktor**: risk.officer.1 (read-only view cukup)
**Pre-condition**: Ada beberapa grup trio dengan status berbeda di periode berbeda

**Setup**:
- Periode 2026-07-01: trio APPROVED sum=1.00 (dari TC-002)
- Periode 2026-08-01: trio DRAFT sum=1.15 (dari TC-003 — belum dihapus)

**Langkah-langkah**:

1. Navigasi ke list **Bobot Skenario** (`/master/bobot-skenario`)
2. Perhatikan kolom/banner summary per trio

**Hasil yang Diharapkan**:

| # | Verifikasi | Pass/Fail |
|---|---|---|
| 1 | Baris/grup periode 2026-07-01: banner atau badge **"Sum 1.00 ✓"** warna hijau | |
| 2 | Baris/grup periode 2026-08-01: banner atau badge **"Sum 1.15 ✗"** warna merah/kuning | |
| 3 | Tooltip atau detail menampilkan nilai masing-masing bobot (G/N/B) beserta total | |
| 4 | Filter `?filter[workflow_status]=APPROVED` menampilkan hanya trio APPROVED | |
| 5 | Export CSV mencakup kolom `skenario`, `bobot`, `periode_berlaku_dari`, `workflow_status` | |
| 6 | Audit event `BOBOT_SKENARIO.EXPORT` muncul setelah download | |

**Langkah Export**:

1. Aktifkan filter: `Status = APPROVED`
2. Klik **"Export" → "CSV"**
3. Download file

**Verifikasi file CSV**:
- Header: `ID,Skenario,Bobot,Periode Berlaku Dari,Periode Berlaku Sampai,Workflow Status,...`
- Encoding: UTF-8 with BOM
- Baris APPROVED muncul, baris DRAFT tidak muncul (filter dihormati)

**Verifikasi Audit SQL**:
```sql
SELECT after_value FROM aud.audit_log
WHERE action = 'BOBOT_SKENARIO.EXPORT'
ORDER BY event_time DESC LIMIT 1;
-- Expected: mengandung format='csv', filters={workflow_status:'APPROVED'}, row_count>=3
```

---

## Cleanup / Rollback

Setelah UAT selesai, jalankan:

```sql
-- Hapus semua data test dari periode yang dipakai UAT.
DO $$
DECLARE
    _id uuid;
BEGIN
    FOR _id IN
        SELECT id FROM mst.bobot_skenario
        WHERE periode_berlaku_dari IN ('2026-07-01','2026-08-01','2026-09-01','2026-10-01','2026-11-01','2026-12-01')
          AND tenant_id = 'TUGURE'
    LOOP
        DELETE FROM sys.workflow_instance WHERE entity_id = _id;
        DELETE FROM mst.bobot_skenario WHERE id = _id;
    END LOOP;
END $$;

-- Catatan: audit log TIDAK dihapus — per DEC-018 retensi 10+10 tahun.
-- Di prod: jangan jalankan DELETE di aud.audit_log.
```

---

## Checklist Ringkas QA Gate

| Gate | Status |
|---|---|
| TC-001 Seed default trio DEC-010 | |
| TC-001 Idempotency seed-default | |
| TC-002 ALCO override sum tetap 1.0 | |
| TC-002 4 signatures setelah full 6-eyes | |
| TC-003 Sum > 1 ditolak di approve2 | |
| TC-003 Pesan "Lebih dari" dalam toast error | |
| TC-003 Workflow tidak advance ke APPROVED | |
| TC-004 6-eyes full cycle 4 state transitions | |
| TC-004 MFA prompt muncul hanya di approve/approve2 | |
| TC-005 SoD: maker tidak bisa approve2 (403) | |
| TC-005 SoD: approver1 tidak bisa approve2 (403) | |
| TC-006 Duplicate skenario+period 422 | |
| TC-007 Period overlap 422 | |
| TC-008 Reject → RETURNED + resubmit berhasil | |
| TC-008 Komentar reject tersimpan di history | |
| TC-009 Trio summary banner sum ✓/✗ di list | |
| TC-009 Export CSV respects filter | |

**SEMUA TC-001 s.d. TC-009 harus PASS sebelum fitur Bobot Skenario dinyatakan siap Phase 4.**

---

## Catatan Strategi Rollback pada Sum Invariant

Saat approve2 ditolak karena sum invariant violated (TC-003), entitas bobot_skenario **tetap berada di status PENDING_APPROVAL_2** — tidak di-revert ke status sebelumnya. Ini adalah keputusan desain yang disengaja:

- Transaksi DB di-rollback sebelum state APPROVED di-commit → entitas tidak pernah masuk ke APPROVED.
- State PENDING_APPROVAL_2 dipertahankan sebagai "pending correction" sehingga Approver2 dapat memberitahu Maker untuk memperbaiki bobot row yang lain.
- Maker (atau siapapun yang bisa edit row DRAFT/RETURNED) perlu mengoreksi bobot pada row NORMAL atau BAD (yang mungkin masih DRAFT), kemudian trigger ulang Approve2.
- Jika Approver2 ingin reject sepenuhnya, gunakan endpoint `/reject` — ini mengubah state ke REJECTED dan memulai ulang siklus dari Maker.

Implikasi test: karena approve2 gagal SEBELUM write APPROVED ke DB (guard di service layer sebelum `COMMIT`), tidak ada "partial APPROVED" state yang perlu di-rollback secara eksplisit. Audit log tidak dituliskan jika approve2 gagal pada sum check.
