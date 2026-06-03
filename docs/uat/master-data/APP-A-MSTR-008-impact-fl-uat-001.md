# UAT Script — APP-A-MSTR-008: Impact FL Multipliers
## Dokumen: APP-A-MSTR-008-impact-fl-uat-001
## Tanggal: 2026-06-04 | Versi: 1.0 | Author: qa-engineer

---

## 1. Konteks Bisnis

Tabel `mst.impact_mev_pd` dan `mst.impact_pd` menyimpan **dual forward-looking (FL) multiplier**
yang digunakan engine ECL (APP-C) untuk menyesuaikan PD berdasarkan kondisi makroekonomi.

- `impact_mev_pd`: multiplier per (periode, skenario) — skenario = GOOD atau BAD.
  NORMAL tidak disimpan; nilainya selalu 1.0 (DEC-010).
- `impact_pd`: satu multiplier per periode (agregat semua skenario).

Kedua entitas menggunakan **workflow 6-eyes** (DEC-017). Langkah APPROVE dan APPROVE2
**wajib step-up MFA** (DEC-027). Hanya ROLE-ALCO yang bisa menjadi approver.

---

## 2. Prasyarat

### 2.1 Data Seed

| ID Pengguna | Username | Role | Keterangan |
|---|---|---|---|
| USR-MAKER | risk_maker_01 | ROLE-RISK | Maker — input parameter |
| USR-REVIEWER | risk_reviewer_01 | ROLE-RISK | Reviewer — validasi teknis |
| USR-ALCO-1 | alco_approver_01 | ROLE-ALCO | Approver pertama (ALCO) |
| USR-ALCO-2 | alco_approver_02 | ROLE-ALCO | Approver kedua (ALCO) |

Pastikan semua user sudah di-assign ke tenant TUGURE dan role masing-masing sudah aktif di Keycloak.

### 2.2 Periode Buku

Pastikan periode buku 2026-Q1 sudah ada di `mst.periode` dan statusnya AKTIF (bukan hard-closed).

- Catat `periode_id` = `<UUID-PERIODE-2026-Q1>` dari tabel `mst.periode`.

### 2.3 State Awal

- Tabel `mst.impact_mev_pd` tidak memiliki row dengan `periode_id = <UUID-PERIODE-2026-Q1>`.
- Tabel `mst.impact_pd` tidak memiliki row dengan `periode_id = <UUID-PERIODE-2026-Q1>`.

### 2.4 Rollback/Cleanup

Setelah test selesai (atau jika test gagal):
```sql
-- Hapus data test (sesuaikan entity_id dengan UUID yang dibuat selama UAT)
DELETE FROM sys.workflow_instance WHERE entity_id IN ('<UUID-MEV-GOOD>', '<UUID-MEV-BAD>', '<UUID-PD-1>');
DELETE FROM mst.impact_mev_pd WHERE id IN ('<UUID-MEV-GOOD>', '<UUID-MEV-BAD>');
DELETE FROM mst.impact_pd WHERE id IN ('<UUID-PD-1>');
```

---

## 3. Skenario Test

### TC-001: Buat impact_mev_pd GOOD multiplier=0.85 periode 2026-Q1

**Aktor**: risk_maker_01 (ROLE-RISK)

**Tujuan**: Verifikasi pembuatan row impact_mev_pd untuk skenario GOOD berhasil dan
record masuk ke DB dalam status DRAFT.

**Langkah-langkah**:

1. Login sebagai `risk_maker_01`.
2. Navigasi ke menu **Master Data > ECL Parameter > Impact MEV PD**.
3. Klik tombol **+ Tambah Baru**.
4. Isi form:
   - Periode: 2026-Q1 (pilih dari dropdown)
   - Skenario: GOOD
   - Impact Multiplier: `0.85`
   - MEV Components JSON: `{"gdp_growth": 0.4, "inflation_rate": 0.3, "exchange_rate": 0.3, "weights": {"gdp_growth": 0.4, "inflation_rate": 0.3, "exchange_rate": 0.3}}`
   - Catatan: "Skenario optimis Q1 2026 — sesuai proyeksi BI"
5. Klik **Simpan**.

**Expected Result**:

- Toast hijau muncul: "Impact MEV PD berhasil dibuat. Menunggu review." + link "Lihat detail →".
- Form di-reset ke kosong.
- Di tabel list, row baru muncul dengan status `DRAFT`.
- Verifikasi DB:
  ```sql
  SELECT id, periode_id, skenario, impact_multiplier, workflow_status
  FROM mst.impact_mev_pd
  WHERE skenario = 'GOOD'
  ORDER BY created_at DESC LIMIT 1;
  -- Expected: impact_multiplier = 0.85000000, workflow_status = 'DRAFT'
  ```
- Verifikasi audit:
  ```sql
  SELECT action, actor_user_id FROM aud.audit_log
  WHERE action = 'IMPACT_MEV_PD.CREATE'
  ORDER BY event_time DESC LIMIT 1;
  -- Expected: 1 row, actor_user_id = USR-MAKER UUID
  ```

**Catat**: UUID entity yang dibuat → `<UUID-MEV-GOOD>`.

---

### TC-002: Buat impact_mev_pd BAD multiplier=1.30 periode 2026-Q1

**Aktor**: risk_maker_01 (ROLE-RISK)

**Tujuan**: Verifikasi pembuatan row impact_mev_pd untuk skenario BAD pada periode yang sama.

**Langkah-langkah**:

1. (Masih login sebagai `risk_maker_01`)
2. Di halaman Impact MEV PD, klik **+ Tambah Baru**.
3. Isi form:
   - Periode: 2026-Q1
   - Skenario: BAD
   - Impact Multiplier: `1.30`
   - Catatan: "Skenario pesimis Q1 2026 — rezim krisis ringan"
4. Klik **Simpan**.

**Expected Result**:

- Toast hijau: "Impact MEV PD berhasil dibuat. Menunggu review."
- Tabel sekarang punya 2 row untuk 2026-Q1: GOOD (0.85) dan BAD (1.30), keduanya DRAFT.
- Verifikasi DB:
  ```sql
  SELECT skenario, impact_multiplier, workflow_status
  FROM mst.impact_mev_pd
  WHERE periode_id = '<UUID-PERIODE-2026-Q1>'
    AND deleted_at IS NULL
  ORDER BY skenario;
  -- Expected: 2 rows — BAD 1.30000000 DRAFT, GOOD 0.85000000 DRAFT
  ```

**Catat**: UUID entity BAD → `<UUID-MEV-BAD>`.

---

### TC-003: Duplicate (periode, skenario) GOOD → toast error

**Aktor**: risk_maker_01 (ROLE-RISK)

**Tujuan**: Verifikasi sistem menolak duplikasi (periode_id, skenario) dengan error yang jelas.

**Langkah-langkah**:

1. (Masih login sebagai `risk_maker_01`)
2. Klik **+ Tambah Baru**.
3. Isi:
   - Periode: 2026-Q1
   - Skenario: GOOD (sudah ada dari TC-001)
   - Impact Multiplier: `0.90`
4. Klik **Simpan**.

**Expected Result**:

- Toast merah persistent: "Impact MEV PD untuk periode 2026-Q1 skenario GOOD sudah ada."
  (error code `IMPACT_DUPLICATE_PERIODE_SKENARIO`)
- Tidak ada row baru di tabel.
- Verifikasi DB:
  ```sql
  SELECT COUNT(*) FROM mst.impact_mev_pd
  WHERE periode_id = '<UUID-PERIODE-2026-Q1>' AND skenario = 'GOOD' AND deleted_at IS NULL;
  -- Expected: 1 (bukan 2)
  ```

**Catatan Auditor**: Error code `IMPACT_DUPLICATE_PERIODE_SKENARIO` harus muncul di response
body (dapat dicek via DevTools Network tab).

---

### TC-004: 6-Eyes Happy Path impact_mev_pd dengan 2 step-up MFA

**Aktor**: risk_maker_01, risk_reviewer_01, alco_approver_01, alco_approver_02

**Tujuan**: Verifikasi alur lengkap DRAFT→PENDING_REVIEW→PENDING_APPROVAL→
PENDING_APPROVAL_2→APPROVED untuk impact_mev_pd GOOD (dari TC-001).

**Langkah-langkah**:

#### Step 1: SUBMIT (risk_maker_01)
1. Login sebagai `risk_maker_01`.
2. Buka detail `<UUID-MEV-GOOD>`.
3. Klik **Submit untuk Review**.
4. Dialog konfirmasi muncul — isi komentar: "Impact MEV PD Q1 2026 siap untuk review."
5. Klik **Konfirmasi Submit**.

**Expected**: Status berubah ke `PENDING_REVIEW`. Toast: "Berhasil disubmit ke reviewer."
Audit: `IMPACT_MEV_PD.SUBMIT` muncul di history.

#### Step 2: REVIEW (risk_reviewer_01)
1. Login sebagai `risk_reviewer_01`.
2. Buka antrian **Pending Review** — `<UUID-MEV-GOOD>` muncul di daftar.
3. Buka detail, verifikasi impact_multiplier = 0.85 dan skenario = GOOD.
4. Klik **Review & Setuju**.
5. Komentar: "Parameter MEV wajar sesuai proyeksi makro BI Q1 2026."
6. Klik **Konfirmasi Review**.

**Expected**: Status berubah ke `PENDING_APPROVAL`. Reviewer tidak bisa approve (tombol Approve
tidak aktif untuk user ini karena SoD).

#### Step 3: APPROVE (alco_approver_01 — step-up MFA wajib)
1. Login sebagai `alco_approver_01`.
2. Buka antrian **Pending Approval** — entity muncul.
3. Klik **Setuju (Approve)**.
4. Sistem meminta **Step-Up MFA**: masukkan OTP/TOTP dari authenticator.
5. Komentar: "Disetujui ALCO — Impact MEV PD sesuai kebijakan ALCO Q1 2026."
6. Klik **Konfirmasi Approve**.

**Expected**: Status berubah ke `PENDING_APPROVAL_2`. Step-up MFA token diverifikasi.
Signature method di audit: `JWT_STEP_UP`.

#### Step 4: APPROVE2 (alco_approver_02 — step-up MFA wajib, user berbeda)
1. Login sebagai `alco_approver_02` (BERBEDA dari alco_approver_01).
2. Buka antrian — entity masih di `PENDING_APPROVAL_2`.
3. Klik **Setuju (Approve 2)**.
4. Sistem meminta **Step-Up MFA** — masukkan OTP.
5. Komentar: "Disetujui ALCO kedua — dual FL multiplier MEV untuk ECL Q1 2026."
6. Klik **Konfirmasi Approve 2**.

**Expected**: Status berubah ke `APPROVED`. Toast: "Impact MEV PD berhasil disetujui oleh dua ALCO."

**Verifikasi akhir**:
```sql
-- State di workflow_instance
SELECT current_state, approver1_id, approver2_id
FROM sys.workflow_instance
WHERE entity_id = '<UUID-MEV-GOOD>';
-- Expected: APPROVED, approver1_id = alco_approver_01 UUID, approver2_id = alco_approver_02 UUID

-- Sync ke tabel domain
SELECT workflow_status FROM mst.impact_mev_pd WHERE id = '<UUID-MEV-GOOD>';
-- Expected: APPROVED

-- 4 signature records
SELECT action, user_id, signature_method FROM sys.workflow_signature
WHERE workflow_id = (SELECT id FROM sys.workflow_instance WHERE entity_id = '<UUID-MEV-GOOD>');
-- Expected: SUBMIT (JWT_STANDARD), REVIEW (JWT_STANDARD), APPROVE (JWT_STEP_UP), APPROVE2 (JWT_STEP_UP)

-- Audit trail
SELECT action FROM aud.audit_log
WHERE entity_id = '<UUID-MEV-GOOD>'
ORDER BY event_time;
-- Expected: IMPACT_MEV_PD.CREATE, IMPACT_MEV_PD.SUBMIT, IMPACT_MEV_PD.REVIEW,
--           IMPACT_MEV_PD.APPROVE, IMPACT_MEV_PD.APPROVE2
```

---

### TC-005: Buat impact_pd multiplier=1.0 periode 2026-Q1

**Aktor**: risk_maker_01 (ROLE-RISK)

**Tujuan**: Verifikasi pembuatan row impact_pd (agregat FL per periode) berhasil.

**Langkah-langkah**:

1. Login sebagai `risk_maker_01`.
2. Navigasi ke **Master Data > ECL Parameter > Impact PD**.
3. Klik **+ Tambah Baru**.
4. Isi form:
   - Periode: 2026-Q1
   - Impact Multiplier: `1.0` (nilai default — tidak ada adjustment)
   - Catatan: "FL PD periode Q1 2026 — baseline"
5. Klik **Simpan**.

**Expected Result**:

- Toast hijau: "Impact PD berhasil dibuat. Menunggu review."
- Row muncul di tabel dengan status `DRAFT`.
- Verifikasi DB:
  ```sql
  SELECT id, periode_id, impact_multiplier, workflow_status
  FROM mst.impact_pd
  WHERE periode_id = '<UUID-PERIODE-2026-Q1>';
  -- Expected: impact_multiplier = 1.00000000, workflow_status = 'DRAFT'
  ```

**Catat**: UUID entity → `<UUID-PD-1>`.

---

### TC-006: impact_pd out-of-range (2.5) → inline error

**Aktor**: risk_maker_01 (ROLE-RISK)

**Tujuan**: Verifikasi sistem menolak multiplier di luar range [0.5, 2.0] dengan inline error pada form.

**Langkah-langkah**:

1. Login sebagai `risk_maker_01`.
2. Klik **+ Tambah Baru** di halaman Impact PD.
3. Isi:
   - Periode: pilih periode baru (bukan 2026-Q1 yang sudah ada)
   - Impact Multiplier: `2.5` (di atas batas maksimum 2.0)
4. Klik **Simpan**.

**Expected Result**:

- Toast merah persistent: "Impact multiplier harus berada di antara 0.5 dan 2.0 (diterima: 2.5)."
  (error code: `IMPACT_PD_OUT_OF_RANGE`)
- Field "Impact Multiplier" disorot merah dengan pesan inline:
  "Impact multiplier harus antara 0.5 dan 2.0"
- Tidak ada row baru di DB.

**Ulangi untuk batas bawah**: masukkan `0.4` → same error.

**Verifikasi DB (negative test)**:
```sql
SELECT COUNT(*) FROM mst.impact_pd WHERE impact_multiplier > 2.0;
-- Expected: 0 (tidak ada row yang lolos constraint)
```

---

### TC-007: 6-Eyes Happy Path impact_pd

**Aktor**: risk_maker_01, risk_reviewer_01, alco_approver_01, alco_approver_02

**Tujuan**: Verifikasi alur 6-eyes untuk impact_pd (dari TC-005, `<UUID-PD-1>`).

**Langkah-langkah**: Sama dengan TC-004 (Impact MEV PD), namun untuk tabel `impact_pd`.

Gunakan endpoint `/api/v1/master/impact-pd/{id}/{submit|review|approve|approve2}`.

**Expected Result per step**:

| Step | Aktor | Status Sebelum | Status Sesudah | MFA |
|---|---|---|---|---|
| SUBMIT | risk_maker_01 | DRAFT | PENDING_REVIEW | Tidak |
| REVIEW | risk_reviewer_01 | PENDING_REVIEW | PENDING_APPROVAL | Tidak |
| APPROVE | alco_approver_01 | PENDING_APPROVAL | PENDING_APPROVAL_2 | **Wajib step-up** |
| APPROVE2 | alco_approver_02 | PENDING_APPROVAL_2 | APPROVED | **Wajib step-up** |

**Verifikasi akhir**:
```sql
-- Workflow state
SELECT current_state FROM sys.workflow_instance WHERE entity_id = '<UUID-PD-1>';
-- Expected: APPROVED

-- Domain sync
SELECT workflow_status FROM mst.impact_pd WHERE id = '<UUID-PD-1>';
-- Expected: APPROVED

-- Signatures
SELECT COUNT(*), COUNT(CASE WHEN signature_method = 'JWT_STEP_UP' THEN 1 END) step_up_count
FROM sys.workflow_signature
WHERE workflow_id = (SELECT id FROM sys.workflow_instance WHERE entity_id = '<UUID-PD-1>');
-- Expected: 4 total, 2 step_up_count (approve + approve2)
```

---

### TC-008: SoD — approver2 harus berbeda dari semua pengguna sebelumnya

**Aktor**: risk_maker_01, risk_reviewer_01, alco_approver_01, (percobaan bypass oleh alco_approver_01)

**Tujuan**: Verifikasi SoD enforcement — approver2 TIDAK bisa menjadi maker, reviewer, atau approver1.

**Pre-condition**: Buat entity baru (impact_pd atau impact_mev_pd) dalam DRAFT, lalu advance ke
PENDING_APPROVAL_2 menggunakan maker, reviewer, dan approver1 yang berbeda (mis. gunakan
`<UUID-MEV-BAD>` dari TC-002 yang belum di-approve).

**Langkah-langkah** (percobaan bypass oleh alco_approver_01 sebagai approver2):

1. Advance `<UUID-MEV-BAD>` ke PENDING_APPROVAL_2:
   - risk_maker_01 → SUBMIT
   - risk_reviewer_01 → REVIEW
   - alco_approver_01 → APPROVE (step-up MFA)

2. Login sebagai `alco_approver_01` (sudah menjadi approver1).
3. Buka entity `<UUID-MEV-BAD>` di PENDING_APPROVAL_2.
4. Klik **Setuju (Approve 2)** — tombol mungkin tampak aktif karena user punya permission.
5. Masukkan OTP step-up MFA.
6. Klik **Konfirmasi**.

**Expected Result**:

- API mengembalikan 403 dengan toast merah persistent:
  "Anda tidak dapat menjadi approver kedua karena Anda sudah berpartisipasi sebagai approver pertama."
  (error code: `SOD_VIOLATION`)
- Status workflow tetap `PENDING_APPROVAL_2`.
- Tidak ada signature baru ditambahkan.

**Verifikasi SoD enforcement di API level** (bukan hanya UI):

Gunakan curl / Postman dengan JWT alco_approver_01 yang valid:
```bash
curl -X POST /api/v1/master/impact-mev-pd/<UUID-MEV-BAD>/approve2 \
  -H "Authorization: Bearer <JWT-ALCO-01>" \
  -H "Idempotency-Key: <UUID>" \
  -d '{"rowVersion":4,"signatureMethod":"JWT_STEP_UP"}'
# Expected: HTTP 403 {"error":{"code":"SOD_VIOLATION",...}}
```

Ulangi percobaan dengan `risk_maker_01` sebagai approver2 → same 403 SOD_VIOLATION.
Ulangi dengan `risk_reviewer_01` sebagai approver2 → same 403 SOD_VIOLATION.

---

## 4. Cek Audit Trail

Setelah semua TC selesai, verifikasi integritas audit trail:

```sql
-- Semua action untuk kedua entitas
SELECT entity_id, action, event_time, actor_user_id
FROM aud.audit_log
WHERE entity_id IN ('<UUID-MEV-GOOD>', '<UUID-MEV-BAD>', '<UUID-PD-1>')
ORDER BY event_time;

-- Hash chain integrity (harus tidak ada gap)
-- Jalankan via cmd/audit-verify:
-- go run ./backend/cmd/audit-verify --range "2026-06-01:2026-06-30"
```

Tidak boleh ada row dengan `current_hash IS NULL`.

---

## 5. Acceptance Criteria Checklist

| # | Kriteria | TC | Status |
|---|---|---|---|
| AC-1 | impact_mev_pd GOOD skenario berhasil dibuat dalam DRAFT | TC-001 | [ ] |
| AC-2 | impact_mev_pd BAD skenario berhasil dibuat dalam DRAFT | TC-002 | [ ] |
| AC-3 | Duplikat (periode, skenario) ditolak dengan IMPACT_DUPLICATE_PERIODE_SKENARIO | TC-003 | [ ] |
| AC-4 | 6-eyes workflow impact_mev_pd selesai dengan 2 step-up MFA ALCO | TC-004 | [ ] |
| AC-5 | impact_pd dibuat dalam DRAFT dengan multiplier 1.0 | TC-005 | [ ] |
| AC-6 | impact_pd multiplier 2.5 (>2.0) ditolak inline dengan IMPACT_PD_OUT_OF_RANGE | TC-006 | [ ] |
| AC-7 | 6-eyes workflow impact_pd selesai dengan 2 step-up MFA ALCO | TC-007 | [ ] |
| AC-8 | SoD: approver2 tidak bisa merangkap maker/reviewer/approver1 | TC-008 | [ ] |
| AC-9 | audit_log berisi semua event CREATE, SUBMIT, REVIEW, APPROVE, APPROVE2 | TC-004 + TC-007 | [ ] |
| AC-10 | workflow_status di tabel domain ter-sync ke APPROVED setelah APPROVE2 | TC-004 + TC-007 | [ ] |
| AC-11 | 4 signature records per entity (submit+review+approve+approve2) | TC-004 + TC-007 | [ ] |
| AC-12 | Approve dan Approve2 menggunakan signature_method = JWT_STEP_UP | TC-004 + TC-007 | [ ] |

---

## 6. Referensi

- DEC-010: ECL formula — 3-skenario × dual FL multiplier
- DEC-017: 6-eyes workflow, SoD
- DEC-027: Step-up MFA pada approve/approve2 ECL parameter
- DEC-016: Decimal precision NUMERIC(10,8)
- FSD-APP-A-MSTR-008 §3 (Impact FL design)
- SoW v1.4 §4 (ECL formula FL component)
- Integration tests: `backend/internal/test/integration/impactmevpd_test.go`
- Integration tests: `backend/internal/test/integration/impactpd_test.go`
