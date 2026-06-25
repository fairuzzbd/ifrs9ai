# UAT-APP-D-007 — Mapping Jurnal: Workflow 6-Eyes (DRAFT → ACTIVE)

**Modul**: APP-D — Mapping Jurnal  
**Story**: STORY-M17-03  
**AC yang diuji**: M17-03-AC1, M17-03-AC2, M17-03-AC3, M17-03-AC4  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Tersedia 5 user dengan peran berbeda (SoD) | Keycloak → Users (lihat tabel P1 di bawah) |
| P2 | Tidak ada mapping `DEPOSITO_INT` dalam status `ACTIVE` atau `DRAFT` | `SELECT COUNT(*) FROM jrnl.mapping_jurnal WHERE kode_jurnal='DEPOSITO_INT' AND deleted_at IS NULL` → 0 |
| P3 | Akses ke `/master/mapping-jurnal` dari browser Chrome terbaru | Buka URL, pastikan tidak 404 |
| P4 | ROLE-KOMITE (`usr-komite-01`) memiliki MFA terdaftar | Keycloak → usr-komite-01 → Credentials |

**Tabel user SoD (P1)**:

| User ID | Peran | Langkah Workflow |
|---|---|---|
| `usr-akun-01` | ROLE-AKUN | Maker: Create + Submit |
| `usr-akunctl-01` | ROLE-AKUN-CTL | Reviewer: Review + Setuju |
| `usr-risk-01` | ROLE-RISK | Approver-1: Approve pertama |
| `usr-komite-01` | ROLE-KOMITE | Approver-2: Approve kedua (final) |
| `usr-audit-01` | ROLE-AUDIT | Audit observer: baca saja |

---

## 2. Data Uji

```
Kode Jurnal  : DEPOSITO_INT
Nama         : Pendapatan Bunga Deposito
Akun Debit   : 113.101.001 (Pendapatan Bunga Deposito)
Akun Kredit  : 311.001.001 (Akrual Bunga Deposito)
Tipe         : ACCRUAL
```

---

## 3. Skenario Uji

### TC-007-01 — Buat Mapping Baru (DRAFT) (M17-03-AC2)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka `/master/mapping-jurnal`.
3. Klik **Buat Baru**.
4. Isi form: Kode = `DEPOSITO_INT`, Nama = "Pendapatan Bunga Deposito", Akun Debit = `113.101.001`, Akun Kredit = `311.001.001`, Tipe = `ACCRUAL`.
5. Klik **Simpan sebagai Draft**.
6. Amati hasil.

**Hasil yang diharapkan**:
- [ ] Toast hijau: "Mapping DEPOSITO_INT berhasil disimpan sebagai Draft. Menunggu disubmit."
- [ ] Record muncul di list dengan status chip "DRAFT".
- [ ] Tombol "Submit" tampil di detail.
- [ ] POST `/api/v1/master/mapping-jurnal` membawa `Idempotency-Key` (UUID v4).

**Verifikasi SQL**:
```sql
SELECT id, kode_jurnal, status_workflow, maker_id
FROM jrnl.mapping_jurnal
WHERE kode_jurnal = 'DEPOSITO_INT' AND deleted_at IS NULL;
-- Simpan ID sebagai :mj_id untuk langkah berikutnya
```

---

### TC-007-02 — Submit Mapping (DRAFT → SUBMITTED) (M17-03-AC2)

**Aktor**: ROLE-AKUN (`usr-akun-01`)  
**Pre-kondisi**: Mapping `DEPOSITO_INT` dalam status `DRAFT`.  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka detail mapping `DEPOSITO_INT` (klik dari list).
3. Klik tombol **Submit**.
4. Dialog konfirmasi muncul: "Submit mapping DEPOSITO_INT untuk review?"
5. Klik **Konfirmasi**.

**Hasil yang diharapkan**:
- [ ] Toast hijau: "Mapping DEPOSITO_INT berhasil di-submit. Menunggu review oleh Finance Controller."
- [ ] Status berubah menjadi `SUBMITTED`.
- [ ] Panel 6-eyes: node "Submit" aktif dengan timestamp dan nama `usr-akun-01`.
- [ ] Tombol "Submit" hilang dari tampilan.

---

### TC-007-03 — Review (SUBMITTED → REVIEWED) (M17-03-AC2)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/master/mapping-jurnal` → filter status = SUBMITTED.
3. Klik detail mapping `DEPOSITO_INT`.
4. Verifikasi tombol "Setuju" tersedia.
5. Klik **Setuju**.
6. Isi `comment`: "Mapping akun sesuai chart of account."
7. Klik **Konfirmasi Review**.

**Hasil yang diharapkan**:
- [ ] Toast hijau: "Review mapping DEPOSITO_INT berhasil. Menunggu persetujuan Risk Officer."
- [ ] Status → `REVIEWED`.
- [ ] Panel 6-eyes: node "Review" aktif, nama `usr-akunctl-01`, comment tersimpan.

**SoD check**: Pastikan jika `usr-akun-01` (maker) mencoba menekan Setuju → tombol tidak ada di DOM.

---

### TC-007-04 — Approve-1 (REVIEWED → APPROVED_1) (M17-03-AC2)

**Aktor**: ROLE-RISK (`usr-risk-01`)  
**Langkah**:

1. Login sebagai `usr-risk-01`.
2. Buka detail mapping `DEPOSITO_INT` (status `REVIEWED`).
3. Klik **Approve** (approve ke-1).
4. Isi `comment`: "Mapping sesuai kebijakan akuntansi PSAK 71."
5. Klik **Konfirmasi Approve**.

**Hasil yang diharapkan**:
- [ ] Toast hijau: "Approval pertama mapping DEPOSITO_INT berhasil. Menunggu approval Komite Investasi."
- [ ] Status → `APPROVED_1`.
- [ ] Panel 6-eyes: node "Approve-1" aktif.
- [ ] `signature_hash` dan `signed_at` tersimpan di database.

**Verifikasi SQL**:
```sql
SELECT status_workflow, approver1_id, approver1_signed_at, approver1_signature_hash
FROM jrnl.mapping_jurnal
WHERE kode_jurnal = 'DEPOSITO_INT';
-- Ekspektasi: APPROVED_1, approver1_id = usr-risk-01, hash IS NOT NULL
```

---

### TC-007-05 — Approve-2 (APPROVED_1 → APPROVED_2 → ACTIVE) (M17-03-AC2 + M17-03-AC4)

**Aktor**: ROLE-KOMITE (`usr-komite-01`)  
**Langkah**:

1. Login sebagai `usr-komite-01` (memiliki `mfa_verified = true` aktif).
2. Buka detail mapping `DEPOSITO_INT` (status `APPROVED_1`).
3. Klik **Approve** (approve ke-2/final).
4. Isi `comment`: "Disetujui oleh Komite Investasi."
5. Klik **Konfirmasi Approve**.
6. **Verifikasi tidak ada MFAStepUpModal** (mfa_verified di JWT sudah cukup untuk approve-2 per DEC-027 scope).
7. Amati status berubah menjadi `ACTIVE`.

**Hasil yang diharapkan**:
- [ ] **Tidak ada** `MFAStepUpModal` yang muncul setelah klik Approve.
- [ ] Toast hijau: "Mapping DEPOSITO_INT disetujui dan sekarang ACTIVE. Jurnal engine akan menggunakan mapping ini."
- [ ] Status → `ACTIVE` (atau `APPROVED_2` jika ada grace period).
- [ ] Panel 6-eyes: semua 4 node terisi.
- [ ] `approver2_signature_hash` tersimpan.

**Verifikasi SQL**:
```sql
SELECT status_workflow, approver2_id, approver2_signed_at
FROM jrnl.mapping_jurnal
WHERE kode_jurnal = 'DEPOSITO_INT';
-- Ekspektasi: ACTIVE, approver2_id = usr-komite-01
```

---

### TC-007-06 — SoD: Maker Tidak Bisa Review via API Langsung (M17-03-AC3)

**Aktor**: ROLE-AKUN (`usr-akun-01`) — user yang juga sebagai maker.  
**Pre-kondisi**: Mapping dalam status `SUBMITTED`, dibuat oleh `usr-akun-01`.  
**Langkah**:

1. Login sebagai `usr-akun-01`.
2. Buka detail mapping `DEPOSITO_INT`.
3. Verifikasi tombol "Setuju" **tidak ada di DOM**.
4. Kirim POST langsung via browser DevTools:
   ```
   POST /api/v1/master/mapping-jurnal/{mj_id}/review
   Authorization: Bearer <token usr-akun-01>
   Content-Type: application/json
   Idempotency-Key: <uuid>
   { "comment": "bypass SoD" }
   ```

**Hasil yang diharapkan**:
- [ ] Tombol "Setuju" tidak ada di DOM untuk `usr-akun-01`.
- [ ] Respon API: `403 FORBIDDEN`, body: `{ "error": { "code": "SOD_VIOLATION", "message": "maker cannot be reviewer" } }`.
- [ ] Status mapping tetap `SUBMITTED`.

---

### TC-007-07 — History Riwayat Persetujuan Tampil Lengkap (M17-03-AC4)

**Aktor**: ROLE-AUDIT (`usr-audit-01`)  
**Pre-kondisi**: Mapping `DEPOSITO_INT` dalam status `ACTIVE`.  
**Langkah**:

1. Login sebagai `usr-audit-01`.
2. Buka detail mapping `DEPOSITO_INT`.
3. Klik tab "Riwayat" (jika ada) atau amati panel audit history.

**Hasil yang diharapkan**:
- [ ] Semua 4 langkah workflow tampil: Submit, Review, Approve-1, Approve-2.
- [ ] Setiap langkah menampilkan: nama aktor, timestamp, komentar, dan signature hash (partial display).
- [ ] Tidak ada tombol mutasi untuk AUDIT.

---

## 4. Verifikasi Audit Trail

```sql
SELECT action, actor_user_id, after_jsonb->>'status_workflow' as status, event_time
FROM aud.audit_log
WHERE entity_type = 'jrnl.mapping_jurnal'
  AND entity_id = :'mj_id'
ORDER BY event_time;
```

**Urutan yang diharapkan**:
1. `MAPPING_JURNAL.CREATE` — usr-akun-01
2. `MAPPING_JURNAL.SUBMIT` — usr-akun-01
3. `MAPPING_JURNAL.REVIEW` — usr-akunctl-01
4. `MAPPING_JURNAL.APPROVE` — usr-risk-01
5. `MAPPING_JURNAL.APPROVE_FINAL` — usr-komite-01

---

## 5. Rollback / Cleanup

```sql
-- Soft delete mapping jurnal test
UPDATE jrnl.mapping_jurnal
SET deleted_at = now(), deleted_by = 'uat-cleanup', row_version = row_version + 1
WHERE kode_jurnal = 'DEPOSITO_INT' AND created_by = 'usr-akun-01';
```

---

## 6. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-AKUN (Maker UAT) | | | PASS / FAIL | |
| ROLE-AKUN-CTL (Reviewer UAT) | | | PASS / FAIL | |
| ROLE-RISK (Approver-1 UAT) | | | PASS / FAIL | |
| ROLE-KOMITE (Approver-2 UAT) | | | PASS / FAIL | |
| Product Owner | | | APPROVED / REJECT | |
