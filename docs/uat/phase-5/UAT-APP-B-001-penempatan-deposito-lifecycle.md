# UAT-APP-B-001 — Penempatan Deposito: Lifecycle & Workflow

**Modul**: APP-B — Transaction Lifecycle (P5-M1)
**Version**: 1.0
**Tanggal**: 2026-06-14
**Author**: qa-engineer
**Status**: DRAFT — menunggu sign-off Treasury Manager + ROLE-RISK

**Linked Stories**: P5-M1-S1, S2, S3, S4, S5, S6
**Linked Decisions**: DEC-P5-M1-001 (FVTPL guard), DEC-P5-M1-004 (settlement informational), DEC-P5-M1-005 (terminate 4-eyes), DEC-017 (SoD), DEC-021 (Idempotency), DEC-013 (EIR Newton-Raphson)
**Linked E2E Test**: `backend/tests/e2e/p5_m1_penempatan_test.go` (Scenario P5-A..K)

---

## Pra-kondisi Global (Semua TC)

Langkah-langkah berikut wajib dilakukan sebelum menjalankan UAT. Satu kali per sesi UAT.

### Data Seed

```sql
-- 1. Instrumen AC (Obligasi BCA, klasifikasi APPROVED)
INSERT INTO mst.instrumen (id, kode_instrumen, nama, tipe_instrumen, klasifikasi_psak71, workflow_status, status, counterparty_id, portofolio_id, tenor_bulan, kupon_persen, mata_uang_id, created_by, updated_by, tenant_id)
VALUES (
  'aaaaaaaa-0001-0000-0000-000000000001',
  'INST-UAT-AC-001',
  'Deposito BCA 12 Bulan',
  'DEPOSITO',
  'AC',
  'APPROVED',
  'AKTIF',
  'bbbbbbbb-0001-0000-0000-000000000001', -- counterparty BCA
  'cccccccc-0001-0000-0000-000000000001', -- portofolio FIXED_INCOME
  12, 0.05250000,
  'dddddddd-0001-0000-0000-000000000001', -- IDR
  'eeeeeeee-0001-0000-0000-000000000001', -- admin
  'eeeeeeee-0001-0000-0000-000000000001',
  'TUGURE'
);

-- 2. Instrumen FVTPL (Saham)
INSERT INTO mst.instrumen (id, kode_instrumen, nama, tipe_instrumen, klasifikasi_psak71, workflow_status, status, created_by, updated_by, tenant_id)
VALUES (
  'aaaaaaaa-0002-0000-0000-000000000002',
  'INST-UAT-FVTPL-001',
  'Saham BUMI - FVTPL',
  'SAHAM',
  'FVTPL',
  'APPROVED',
  'AKTIF',
  'eeeeeeee-0001-0000-0000-000000000001',
  'eeeeeeee-0001-0000-0000-000000000001',
  'TUGURE'
);

-- 3. Periode Buku Juni 2026 = OPEN
INSERT INTO mst.periode_buku (id, kode_periode, bulan, tahun, status_periode, created_by, updated_by, tenant_id)
VALUES (
  'ffffffff-0001-0000-0000-000000000001',
  'PBUKU-2026-06',
  6, 2026, 'OPEN',
  'eeeeeeee-0001-0000-0000-000000000001',
  'eeeeeeee-0001-0000-0000-000000000001',
  'TUGURE'
);

-- 4. Settlement account balance (untuk TC-007)
INSERT INTO sys.settlement_account_balance (settlement_account_id, last_known_balance_idr, as_of_date, updated_by, tenant_id)
VALUES ('1234567890', 3000000000.0000, '2026-06-13', 'eeeeeeee-0001-0000-0000-000000000001', 'TUGURE')
ON CONFLICT (settlement_account_id) DO UPDATE
  SET last_known_balance_idr = EXCLUDED.last_known_balance_idr,
      as_of_date = EXCLUDED.as_of_date;
```

### Role Assignment (User Berbeda untuk SoD)

| User ID | Nama | Role | Keterangan |
|---|---|---|---|
| USR-UAT-001 | Ahmad Fauzi | ROLE-MAKER-TR | Maker utama |
| USR-UAT-002 | Budi Santoso | ROLE-APPR-TR | Reviewer |
| USR-UAT-003 | Citra Dewi | ROLE-APPR-TR | Approver (Treasury Manager, MFA aktif) |
| USR-UAT-004 | Dian Pratama | ROLE-MAKER-TR | Terminate proposer |
| USR-UAT-005 | Eko Wijaya | ROLE-APPR-TR | Terminate reviewer |
| USR-UAT-006 | Fika Sari | ROLE-APPR-TR | Terminate approver (TM, MFA aktif) |
| USR-UAT-007 | Gita Rahayu | ROLE-AUDIT | Auditor |

### Rollback / Cleanup

```sql
-- Jalankan setelah UAT selesai untuk restore ke kondisi bersih.
DELETE FROM trx.penempatan_deposito WHERE kode_transaksi LIKE 'PNP-2026%' AND tenant_id = 'TUGURE';
DELETE FROM ecl.stage_history WHERE trigger_type = 'INITIAL_PLACEMENT' AND created_at >= '2026-06-14';
DELETE FROM aud.audit_log WHERE action LIKE 'PENEMPATAN.%' AND event_time >= '2026-06-14';
```

---

## TC-001: Treasury Maker Membuat Penempatan AC Obligasi (Happy Path)

**Actor**: USR-UAT-001 (ROLE-MAKER-TR)
**Goal**: Membuat record penempatan deposito IDR dalam status DRAFT
**Mapped Scenario E2E**: P5-A (step 1)
**Mapped AC**: P5-M1-S1 Happy Path 1

### Pra-kondisi TC-001

1. Login sebagai USR-UAT-001 (ROLE-MAKER-TR).
2. Instrumen `INST-UAT-AC-001` tersedia, `workflow_status = APPROVED`, `klasifikasi_psak71 = AC`.
3. Counterparty BCA (`CP-BCA-001`) aktif.
4. Periode buku Juni-2026 `status_periode = OPEN`.

### Langkah TC-001

1. Navigasi ke `/trx/penempatan/new`.
2. Pilih instrumen: `INST-UAT-AC-001 — Deposito BCA 12 Bulan`.
3. Pilih counterparty bank: `BCA (CP-BCA-001)`.
4. Isi tanggal penempatan: `2026-06-14`.
5. Isi nominal IDR: `5.000.000.000` (IDR 5 Miliar).
6. Isi tenor bulan: `12`.
7. Isi kupon %: `5.25` (akan disimpan sebagai `0.05250000`).
8. Isi nomor referensi bank: `BCA/DEP/2026/001`.
9. Isi rekening settlement: `1234567890`.
10. Lampirkan dokumen kontrak: `kontrak_deposito_BCA_Jun2026.pdf`.
11. Klik **Simpan**.

### Hasil yang Diharapkan TC-001

| Verifikasi | Expected |
|---|---|
| HTTP response | 201 Created |
| `kode_transaksi` format | `PNP-202606-000001` (format PNP-YYYYMM-######) |
| `workflow_status` | `DRAFT` |
| `tanggal_jatuh_tempo` | `2027-06-14` (= 2026-06-14 + 12 bulan) |
| `eir_awal` | `NULL` (belum dihitung, akan async post-approve) |
| `maker_id` | `USR-UAT-001` |
| `reviewer_id` | `NULL` |
| `approver_id` | `NULL` |
| Toast sukses | "Penempatan PNP-202606-000001 berhasil dibuat. Status: DRAFT. Lampirkan dokumen jika belum, lalu submit untuk review." |
| Settlement balance hint | Amber warning: "Nominal (IDR 5.000.000.000) melebihi saldo terakhir yang diketahui (IDR 3.000.000.000 per 2026-06-13)" |

### Assersi DB TC-001

```sql
-- Verifikasi record DRAFT ter-insert.
SELECT kode_transaksi, workflow_status, nominal_idr, tenor_bulan, maker_id, eir_awal
FROM trx.penempatan_deposito
WHERE kode_transaksi = 'PNP-202606-000001' AND tenant_id = 'TUGURE';
-- Expected: 1 row, workflow_status='DRAFT', nominal_idr=5000000000.0000, eir_awal=NULL

-- Verifikasi audit log PENEMPATAN.CREATED.
SELECT action, actor_user_id, entity_type
FROM aud.audit_log
WHERE action = 'PENEMPATAN.CREATED'
  AND entity_id = (SELECT id FROM trx.penempatan_deposito WHERE kode_transaksi = 'PNP-202606-000001');
-- Expected: 1 row dengan actor_user_id = USR-UAT-001
```

### Sign-off TC-001

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| Treasury Maker | | | VERIFIED | |

---

## TC-002: 4-Eyes Workflow — 3 Aktor Berbeda

**Actor**: USR-UAT-001 (Maker), USR-UAT-002 (Reviewer), USR-UAT-003 (Approver/TM)
**Goal**: Penempatan melalui 4-eyes lengkap sampai APPROVED_ACTIVE, EIR Asynq job di-enqueue
**Mapped Scenario E2E**: P5-A (full flow)
**Mapped AC**: P5-M1-S2 Happy Path Full 4-Eyes

**Pra-kondisi**: TC-001 berhasil, `PNP-202606-000001` dalam status DRAFT.

### Langkah TC-002 — Submit (USR-UAT-001)

1. Login sebagai USR-UAT-001.
2. Navigasi ke `/trx/penempatan/PNP-202606-000001`.
3. Klik tombol **Submit untuk Review**.
4. Isi komentar: "Penempatan deposito BCA sesuai limit portofolio, dokumen terlampir."
5. Klik **Konfirmasi Submit**.

**Expected setelah submit**:
- `workflow_status = PENDING_REVIEW`
- Audit `PENEMPATAN.SUBMITTED` tercatat
- Notifikasi badge muncul untuk USR-UAT-002 (ROLE-APPR-TR)

### Langkah TC-002 — Review (USR-UAT-002)

6. Login sebagai USR-UAT-002.
7. Navigasi ke `/trx/penempatan` → filter PENDING_REVIEW.
8. Buka `PNP-202606-000001`.
9. Periksa detail: nominal, tenor, kupon, dokumen terlampir.
10. Klik tombol **Review & Setujui**.
11. Isi komentar: "Dokumen lengkap, nominal dan tenor sesuai limit portofolio. Referensi bank valid."
12. Klik **Konfirmasi Review**.

**Expected setelah review**:
- `workflow_status = PENDING_APPROVAL`
- `reviewer_id = USR-UAT-002`
- `reviewer_signed_at` terisi timestamp
- `reviewer_signature_hash` terisi (SHA-256 dari payload)
- Audit `PENEMPATAN.REVIEWED` tercatat
- Notifikasi ke Treasury Manager (USR-UAT-003)

### Langkah TC-002 — Approve (USR-UAT-003, MFA Required)

13. Login sebagai USR-UAT-003 (Treasury Manager).
14. Sistem mendeteksi MFA step-up dibutuhkan → tampil dialog MFA.
15. Selesaikan MFA step-up (TOTP).
16. Navigasi ke `/trx/penempatan` → filter PENDING_APPROVAL.
17. Buka `PNP-202606-000001`.
18. Klik tombol **Setujui Penempatan**.
19. Isi komentar: "Disetujui sesuai RKAP 2026. Nominal dalam batas investasi portofolio."
20. Klik **Konfirmasi Persetujuan**.

### Hasil yang Diharapkan TC-002

| Verifikasi | Expected |
|---|---|
| `workflow_status` | `APPROVED_ACTIVE` |
| `approver_id` | `USR-UAT-003` |
| `approver_signed_at` | Timestamp sekarang |
| `approver_signature_hash` | Non-empty SHA-256 |
| Audit `PENEMPATAN.APPROVED` | Ada, actor = USR-UAT-003 |
| Audit `PENEMPATAN.STAGING_INITIAL` | Ada (AC instrument → Stage 1) |
| `ecl.stage_history` row | INSERT: `stage_sesudah='STAGE_1'`, `trigger_type='INITIAL_PLACEMENT'` |
| Asynq task `EIR_COMPUTE` | Ter-enqueue (visible di `/jobs`) |
| Toast sukses | "Penempatan PNP-202606-000001 disetujui. EIR sedang dihitung (lihat progress di /jobs)." |
| `PenempatanApprovedEvent` | Ter-emit ke event bus (dikonsumsi P5-M2) |

### Assersi DB TC-002

```sql
-- Verifikasi APPROVED_ACTIVE + approver fields.
SELECT workflow_status, approver_id, approver_signed_at, approver_signature_hash
FROM trx.penempatan_deposito
WHERE kode_transaksi = 'PNP-202606-000001';
-- Expected: workflow_status='APPROVED_ACTIVE', approver_signature_hash NOT NULL

-- Verifikasi ecl.stage_history Stage 1 initial.
SELECT instrumen_id, stage_sesudah, trigger_type, status_approval
FROM ecl.stage_history
WHERE instrumen_id = 'aaaaaaaa-0001-0000-0000-000000000001'
  AND trigger_type = 'INITIAL_PLACEMENT';
-- Expected: 1 row, stage_sesudah='STAGE_1'

-- Verifikasi audit chain (3 approval steps harus ada).
SELECT action, actor_user_id
FROM aud.audit_log
WHERE entity_id = (SELECT id FROM trx.penempatan_deposito WHERE kode_transaksi = 'PNP-202606-000001')
ORDER BY event_time ASC;
-- Expected: PENEMPATAN.CREATED, PENEMPATAN.SUBMITTED, PENEMPATAN.REVIEWED, PENEMPATAN.APPROVED, PENEMPATAN.STAGING_INITIAL
```

### Sign-off TC-002

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| Treasury Manager | | | VERIFIED | |
| Risk Officer | | | VERIFIED | |

---

## TC-003: FVTPL Skip — Tidak Ada ECL/EIR yang Di-trigger

**Actor**: USR-UAT-001 (Maker), USR-UAT-002 (Reviewer), USR-UAT-003 (Approver)
**Goal**: Instrumen FVTPL setelah approve TIDAK men-trigger ECL staging atau EIR compute
**Mapped Scenario E2E**: P5-B
**Mapped AC**: P5-M1-S2 + DEC-P5-M1-001

**Pra-kondisi**: Instrumen `INST-UAT-FVTPL-001` tersedia, `klasifikasi_psak71 = FVTPL`, `workflow_status = APPROVED`.

### Langkah TC-003

1. USR-UAT-001: Buat penempatan baru dengan instrumen `INST-UAT-FVTPL-001`.
2. Submit, review (USR-UAT-002), approve (USR-UAT-003 + MFA).
3. Setelah approve, periksa response body dari server.
4. Cek `/jobs` untuk EIR compute task.
5. Cek database untuk ecl.stage_history.

### Hasil yang Diharapkan TC-003

| Verifikasi | Expected |
|---|---|
| `workflow_status` | `APPROVED_ACTIVE` |
| Response field `stagingAction` | `SKIPPED_FVTPL` |
| Response field `eirComputeJobId` | `null` |
| `/jobs` — EIR task | Tidak ada task untuk penempatan ini |
| Audit `PENEMPATAN.STAGING_SKIPPED_FVTPL` | Ada (DEC-P5-M1-001 compliance marker) |
| Audit `PENEMPATAN.STAGING_INITIAL` | Tidak ada (harus ABSEN untuk FVTPL) |
| `eir_awal` | `NULL` (FVTPL tidak butuh EIR) |

### Assersi DB TC-003

```sql
-- Audit STAGING_SKIPPED_FVTPL harus ada.
SELECT action FROM aud.audit_log
WHERE entity_id = (SELECT id FROM trx.penempatan_deposito
                   WHERE instrumen_id = 'aaaaaaaa-0002-0000-0000-000000000002'
                   AND tenant_id = 'TUGURE'
                   ORDER BY created_at DESC LIMIT 1)
  AND action = 'PENEMPATAN.STAGING_SKIPPED_FVTPL';
-- Expected: 1 row

-- stage_history TIDAK boleh ada untuk FVTPL instrumen (DEC-P5-M1-001).
SELECT COUNT(*) FROM ecl.stage_history
WHERE instrumen_id = 'aaaaaaaa-0002-0000-0000-000000000002'
  AND trigger_type = 'INITIAL_PLACEMENT';
-- Expected: 0 rows
```

### Sign-off TC-003

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| IFRS9 Compliance Reviewer | | | VERIFIED | |

---

## TC-004: SoD Violation — Negative Tests (3 Sub-kasus)

**Actor**: USR-UAT-001 (Maker), USR-UAT-002 (Reviewer), USR-UAT-003 (Approver)
**Goal**: Verifikasi enforcement SoD server-side — bukan hanya UI
**Mapped Scenario E2E**: P5-D, P5-E
**Mapped AC**: P5-M1-S2 Error Cases SoD

**Pra-kondisi**: Buat penempatan baru `PNP-202606-SoD-001` oleh USR-UAT-001, lalu submit.

### TC-004a: Maker Mencoba Me-review Transaksi Sendiri

1. Login sebagai USR-UAT-001 (yang membuat penempatan).
2. Kirim request langsung via Postman/curl:
   ```
   POST /api/v1/transaksi/penempatan/{id}/review
   Authorization: Bearer {USR-UAT-001 token}
   Idempotency-Key: {uuid}
   Body: {"comment": "Mencoba review transaksi sendiri", "signature_method": "JWT_STEP_UP"}
   ```

**Expected**: HTTP 403 dengan body:
```json
{
  "error": {
    "code": "PENEMPATAN_SOD_VIOLATION",
    "message": "Anda tidak bisa menjadi reviewer untuk penempatan yang Anda buat sendiri (DEC-017)."
  }
}
```

**Assersi DB**: `aud.audit_log` mengandung `PENEMPATAN.SOD_VIOLATION_ATTEMPT` untuk penempatan ini.

### TC-004b: Reviewer Mencoba Menjadi Approver

1. USR-UAT-002 berhasil review → status PENDING_APPROVAL.
2. Login sebagai USR-UAT-002 (reviewer).
3. Kirim request:
   ```
   POST /api/v1/transaksi/penempatan/{id}/approve
   Authorization: Bearer {USR-UAT-002 token}
   ```

**Expected**: HTTP 403 `PENEMPATAN_SOD_VIOLATION` — "Reviewer tidak bisa menjadi approver pada transaksi yang sama (DEC-017)."

### TC-004c: Maker Mencoba Menjadi Approver (Langsung)

1. Login sebagai USR-UAT-001 (maker asli).
2. Kirim request approve langsung via API (bypass UI):
   ```
   POST /api/v1/transaksi/penempatan/{id}/approve
   Authorization: Bearer {USR-UAT-001 token}
   ```

**Expected**: HTTP 403 `PENEMPATAN_SOD_VIOLATION` — "Approver tidak bisa menjadi maker".

### Sign-off TC-004

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| Security Engineer | | | VERIFIED | |
| Internal Audit (USR-UAT-007) | | | VERIFIED | |

---

## TC-005: Step-Up MFA Enforcement pada Approve

**Actor**: USR-UAT-003 (Approver/Treasury Manager)
**Goal**: Approve tanpa MFA step-up (atau step-up kadaluarsa > 5 menit) harus ditolak
**Mapped Scenario E2E**: P5-F
**Mapped AC**: P5-M1-S2 + DEC-027

**Pra-kondisi**: Penempatan dalam status PENDING_APPROVAL.

### Langkah TC-005

#### Sub-kasus A: Approve Tanpa MFA (mfa_verified=false)

1. Siapkan JWT token USR-UAT-003 dengan `mfa_verified: false` (via test identity provider).
2. Kirim request approve:
   ```
   POST /api/v1/transaksi/penempatan/{id}/approve
   Authorization: Bearer {token-no-mfa}
   X-Step-Up-Token: (absent)
   ```
3. **Expected**: HTTP 403 `PENEMPATAN_STEP_UP_REQUIRED` — "Persetujuan penempatan memerlukan MFA step-up."

#### Sub-kasus B: Approve dengan Step-Up Kadaluarsa (> 5 menit lalu)

1. Set `stepup_verified_at` = 10 menit lalu di JWT claims.
2. Kirim request approve.
3. **Expected**: HTTP 403 `PENEMPATAN_STEP_UP_REQUIRED`.

#### Sub-kasus C: Approve dengan Step-Up Segar (< 5 menit)

4. Lakukan MFA step-up: `POST /auth/step-up` → `step_up_token`.
5. Set `stepup_verified_at` = now - 2 menit.
6. Kirim request approve dengan step-up segar.
7. **Expected**: HTTP 200, `workflow_status = APPROVED_ACTIVE`.

### Assersi DB TC-005

```sql
-- Verifikasi hanya 1 PENEMPATAN.APPROVED (dari sub-kasus C).
SELECT COUNT(*) FROM aud.audit_log
WHERE entity_id = {penempatan_id}
  AND action = 'PENEMPATAN.APPROVED';
-- Expected: 1 (bukan 3 — sub-kasus A dan B ditolak sebelum audit ditulis)
```

### Sign-off TC-005

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| Security Engineer | | | VERIFIED | |
| Treasury Manager | | | VERIFIED | |

---

## TC-006: Terminate Workflow 4-Eyes

**Actor**: USR-UAT-004 (Terminate Proposer), USR-UAT-005 (Terminate Reviewer), USR-UAT-006 (Terminate Approver)
**Goal**: Terminasi manual penempatan APPROVED_ACTIVE melalui 4-eyes penuh (DEC-P5-M1-005)
**Mapped Scenario E2E**: P5-G, P5-H
**Mapped AC**: P5-M1-S5 Happy Path 2 (Manual Terminate)

**Pra-kondisi**: Penempatan `PNP-202606-000001` dalam status `APPROVED_ACTIVE`.

### Langkah TC-006 — Propose Terminate (USR-UAT-004)

1. Login sebagai USR-UAT-004 (ROLE-MAKER-TR).
2. Navigasi ke `/trx/penempatan/PNP-202606-000001`.
3. Klik tombol **Ajukan Terminasi** (visible karena status APPROVED_ACTIVE).
4. Isi alasan terminasi (minimal 30 karakter):
   "Bank counterparty meminta pengembalian dana lebih awal karena restrukturisasi internal. Surat tertanggal 2026-11-30 terlampir."
5. Lampirkan dokumen terminasi: `surat_terminasi_BCA_Nov2026.pdf`.
6. Klik **Kirim Proposal Terminasi**.

**Expected setelah propose**:
- `workflow_status = TERMINATION_PENDING_REVIEW`
- Audit `PENEMPATAN.TERMINATE_PROPOSED` tercatat
- Notifikasi ke ROLE-APPR-TR (USR-UAT-005)

### Langkah TC-006 — Terminate Review (USR-UAT-005)

7. Login sebagai USR-UAT-005.
8. Buka `PNP-202606-000001` → Tab "Terminasi Pending Review".
9. Baca detail proposal dan dokumen terminasi.
10. Klik **Review Terminasi**.
11. Isi komentar: "Dokumen surat dari bank terlampir dan valid. Alasan terminasi sesuai prosedur internal."
12. Klik **Konfirmasi Review Terminasi**.

**Expected setelah terminate review**:
- `workflow_status = TERMINATION_PENDING_APPROVAL`
- `terminate_reviewer_id = USR-UAT-005`
- `terminate_reviewer_signature_hash` terisi
- Audit `PENEMPATAN.TERMINATE_REVIEWED` tercatat

### Langkah TC-006 — Terminate Approve (USR-UAT-006, MFA)

13. Login sebagai USR-UAT-006 (Treasury Manager, MFA aktif).
14. Lakukan MFA step-up jika diperlukan.
15. Buka `PNP-202606-000001`.
16. Klik **Setujui Terminasi**.
17. Isi komentar: "Disetujui sesuai memo Direktur Keuangan No. 123/2026."
18. Klik **Konfirmasi Persetujuan Terminasi**.

### Hasil yang Diharapkan TC-006

| Verifikasi | Expected |
|---|---|
| `workflow_status` | `TERMINATED` |
| `terminated_at` | Tanggal hari ini (2026-12-01) |
| `terminate_approver_id` | `USR-UAT-006` |
| `terminate_approver_signature_hash` | Non-empty SHA-256 |
| Audit `PENEMPATAN.TERMINATE_APPROVED` | Ada |
| Audit `PENEMPATAN.DERECOGNITION_QUEUED` | Ada (event ke P5-M9) |
| `PenempatanTerminatedEvent` | Ter-emit (dikonsumsi P5-M9) |
| SoD verify | USR-UAT-006 ≠ USR-UAT-004 AND USR-UAT-006 ≠ USR-UAT-005 |

### Negative Test TC-006 — Terminate SoD

19. Coba USR-UAT-004 (proposer) melakukan terminate-review → HTTP 403 SOD_VIOLATION.
20. Coba USR-UAT-005 (terminate reviewer) melakukan terminate-approve → HTTP 403 SOD_VIOLATION.
21. Coba USR-UAT-001 (original create maker) melakukan terminate-review → HTTP 403 SOD_VIOLATION.

**Expected**: Semua 3 attempt menghasilkan 403 dan audit `PENEMPATAN.SOD_VIOLATION_ATTEMPT`.

### Assersi DB TC-006

```sql
-- Verifikasi TERMINATED.
SELECT workflow_status, terminated_at, terminate_approver_id
FROM trx.penempatan_deposito WHERE kode_transaksi = 'PNP-202606-000001';
-- Expected: TERMINATED, terminated_at SET

-- Verifikasi audit chain terminasi.
SELECT action, actor_user_id FROM aud.audit_log
WHERE entity_id = (SELECT id FROM trx.penempatan_deposito WHERE kode_transaksi = 'PNP-202606-000001')
  AND action IN ('PENEMPATAN.TERMINATE_PROPOSED','PENEMPATAN.TERMINATE_REVIEWED',
                 'PENEMPATAN.TERMINATE_APPROVED','PENEMPATAN.DERECOGNITION_QUEUED',
                 'PENEMPATAN.SOD_VIOLATION_ATTEMPT')
ORDER BY event_time ASC;
-- Expected: 7 rows (PROPOSED, REVIEWED, 3 × SOD_ATTEMPT, APPROVED, DERECOGNITION_QUEUED)
```

### Sign-off TC-006

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| Treasury Manager | | | VERIFIED | |
| Risk Officer | | | VERIFIED | |

---

## TC-007: Settlement Balance Informational (DEC-P5-M1-004)

**Actor**: USR-UAT-001 (Maker)
**Goal**: Settlement balance hint bersifat informatif — tidak memblok create (DEC-P5-M1-004)
**Mapped Scenario E2E**: P5-I
**Mapped AC**: P5-M1-S1 Informational Balance Hint

### Langkah TC-007

#### Sub-kasus A: Nominal > Saldo Terakhir → Warning Amber, Tidak Diblok

1. Pastikan `sys.settlement_account_balance` memiliki record: `settlement_account='1234567890'`, `last_known_balance_idr=3000000000`, `as_of_date='2026-06-13'`.
2. Buat penempatan dengan `nominal_idr=5000000000` dan `settlement_account='1234567890'`.
3. Klik **Simpan**.

**Expected**:
- HTTP 201 Created (tidak 422)
- Amber warning di UI: "Perhatian: Nominal (IDR 5.000.000.000) melebihi saldo terakhir yang diketahui (IDR 3.000.000.000 per 2026-06-13). Pastikan saldo tersedia sebelum submit."
- Record ter-insert dengan `workflow_status = DRAFT`
- Response body mengandung `settlement_balance_hint.last_known_idr = 3000000000.0000`
- `settlement_balance_hint.is_sufficient = null` (tidak dinilai)

#### Sub-kasus B: Rekening Tidak Ditemukan → Hint Null, Tidak Error

4. Buat penempatan dengan `settlement_account='9999999999'` (tidak ada di sys.settlement_account_balance).
5. Klik **Simpan**.

**Expected**:
- HTTP 201 Created
- `settlement_balance_hint = null` di response
- UI menampilkan label abu-abu: "Saldo tidak tersedia — pastikan saldo mencukupi sebelum submit"
- Record ter-insert dengan `workflow_status = DRAFT`

#### Sub-kasus C: Saldo Kadaluarsa (> 24 Jam) → isStale = true

6. Update `sys.settlement_account_balance` → `as_of_date = CURRENT_DATE - 2` (2 hari lalu).
7. Buat penempatan.

**Expected**:
- HTTP 201 Created
- `settlement_balance_hint.is_stale = true` di response
- UI menampilkan flag "Data saldo mungkin sudah tidak akurat (lebih dari 24 jam)"

### Assersi DB TC-007

```sql
-- Verifikasi tidak ada 422 block — record harus ada.
SELECT workflow_status FROM trx.penempatan_deposito
WHERE settlement_account = '1234567890'
  AND nominal_idr = 5000000000.0000
  AND tenant_id = 'TUGURE';
-- Expected: row EXISTS dengan workflow_status = 'DRAFT'
```

### Sign-off TC-007

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| Finance Controller | | | VERIFIED (validasi keputusan DEC-P5-M1-004) | |

---

## TC-008: Asynq Maturity Cron — Auto-Mature

**Actor**: System (Asynq worker, tidak ada human actor)
**Goal**: Asynq daily cron 09:00 WIB memindahkan penempatan ke MATURED saat tanggal_jatuh_tempo tiba
**Mapped Scenario E2E**: P5-A (maturity step)
**Mapped AC**: P5-M1-S5 Happy Path 1

**Pra-kondisi**: Minimal 1 penempatan dalam status `APPROVED_ACTIVE` dengan `tanggal_jatuh_tempo = CURRENT_DATE`.

### Setup TC-008

```sql
-- Simulasi penempatan dengan jatuh tempo hari ini.
UPDATE trx.penempatan_deposito
SET tanggal_jatuh_tempo = CURRENT_DATE
WHERE kode_transaksi = 'PNP-202606-000001'
  AND tenant_id = 'TUGURE';
```

### Langkah TC-008

1. Trigger manual maturity-checker job:
   ```bash
   # Via Asynq task injection (dev/UAT only)
   asynq task enqueue --type="penempatan:maturity_check" --payload='{"date":"'$(date +%Y-%m-%d)'"}'
   ```
   Atau tunggu cron job 09:00 WIB berjalan.

2. Navigasi ke `/jobs` → filter type = `PENEMPATAN_MATURITY_CHECK`.
3. Periksa job status.

### Hasil yang Diharapkan TC-008

| Verifikasi | Expected |
|---|---|
| `workflow_status` | `MATURED` |
| `matured_at` | `CURRENT_DATE` |
| `/jobs` progress | "1 of 1 penempatan diproses" (atau lebih jika batch) |
| `sys.job.status` | `COMPLETED` |
| Audit `PENEMPATAN.MATURED` | Ada, actor = `system` |
| Audit `PENEMPATAN.DERECOGNITION_QUEUED` | Ada |
| `PenempatanMaturedEvent` | Dikonsumsi oleh P5-M9 (verifikasi queue P5-M9 terpisah) |
| Notifikasi | USR-UAT-001 (ROLE-MAKER-TR) + ROLE-RISK mendapat notifikasi |

### Assersi DB TC-008

```sql
-- Verifikasi MATURED.
SELECT workflow_status, matured_at FROM trx.penempatan_deposito
WHERE kode_transaksi = 'PNP-202606-000001' AND tenant_id = 'TUGURE';
-- Expected: MATURED, matured_at = CURRENT_DATE

-- Verifikasi audit trail maturity.
SELECT action FROM aud.audit_log
WHERE entity_id = (SELECT id FROM trx.penempatan_deposito
                   WHERE kode_transaksi = 'PNP-202606-000001')
  AND action IN ('PENEMPATAN.MATURED', 'PENEMPATAN.DERECOGNITION_QUEUED');
-- Expected: 2 rows
```

### Sign-off TC-008

| Peran | Nama | Tanggal | Hasil | Paraf |
|---|---|---|---|---|
| Tester | | | PASS / FAIL | |
| IT Admin | | | VERIFIED (Asynq job ran correctly) | |
| Treasury Manager | | | VERIFIED | |

---

## Ringkasan Sign-off UAT

| TC | Judul | Status | Tester | Tanggal |
|---|---|---|---|---|
| TC-001 | Treasury Maker Membuat Penempatan AC | | | |
| TC-002 | 4-Eyes Workflow 3 Aktor Berbeda | | | |
| TC-003 | FVTPL Skip — No ECL/EIR | | | |
| TC-004 | SoD Violation Negative Tests | | | |
| TC-005 | Step-Up MFA Enforcement | | | |
| TC-006 | Terminate Workflow 4-Eyes | | | |
| TC-007 | Settlement Balance Informational | | | |
| TC-008 | Asynq Maturity Cron Auto-Mature | | | |

**UAT Approved by**:

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Treasury Manager | | | |
| Risk Officer | | | |
| Finance Controller | | | |
| IFRS9 Compliance Reviewer | | | |
| Security Engineer (SoD/MFA) | | | |
| QA Engineer | | | |
