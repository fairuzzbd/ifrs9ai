# UAT-APP-D-009 — Jurnal DLQ: Retry GL Delivery dengan MFA Step-Up

**Modul**: APP-D — Jurnal DLQ  
**Story**: STORY-M17-04  
**AC yang diuji**: M17-04-AC3  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Terdapat minimal 1 entri DLQ dalam status `FAILED` | `SELECT COUNT(*) FROM jrnl.dlq WHERE status='FAILED'` → ≥ 1 |
| P2 | Tersedia 2 user aktif | Tabel user di bawah |
| P3 | User `usr-itadmin-01` memiliki MFA terdaftar (TOTP) | Keycloak → usr-itadmin-01 → Credentials |
| P4 | GL Host stub dapat menerima retry (atau stub configured untuk sukses pada retry) | `GET /api/v1/gl/health` → 200 |

**Seed DLQ entry jika belum ada**:
```sql
-- UAT seed only
INSERT INTO jrnl.dlq (
  id, jurnal_header_id, error_code, error_message, retry_count,
  status, first_failed_at, created_at, created_by, tenant_id
) VALUES (
  'dlq-uat-001',
  (SELECT id FROM jrnl.jurnal_header ORDER BY created_at DESC LIMIT 1),
  'GL_TIMEOUT',
  'Connection to GL Host timed out after 30s',
  2,
  'FAILED',
  now() - interval '1 hour',
  now(),
  'uat-seed',
  'TUGURE'
);
```

**Tabel user**:

| User ID | Peran | Akses |
|---|---|---|
| `usr-akunctl-01` | ROLE-AKUN-CTL | Baca DLQ, tidak bisa Replay |
| `usr-itadmin-01` | ROLE-IT-ADMIN | Baca + Replay DLQ |

---

## 2. Skenario Uji

### TC-009-01 — DLQ Tab Hanya Tampil untuk AKUN-CTL dan IT-ADMIN (M17-04-AC3)

**Aktor 1**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Aktor 2**: ROLE-AKUN (`usr-akun-01`) — tidak punya akses DLQ  
**Langkah**:

1. Login sebagai `usr-akun-01` → buka `/jurnal/header`.
2. Amati tab yang tersedia.
3. Logout → login sebagai `usr-akunctl-01` → buka `/jurnal/dlq`.

**Hasil yang diharapkan**:
- [ ] `usr-akun-01`: hanya tab "Header Jurnal" tampil, tab DLQ tidak ada di DOM.
- [ ] `usr-akunctl-01`: tab "Header Jurnal" dan tab "DLQ" tampil.
- [ ] `usr-akunctl-01` bisa membuka `/jurnal/dlq` dan melihat daftar.

---

### TC-009-02 — Daftar DLQ: Read-Only untuk ROLE-AKUN-CTL (M17-04-AC3)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/jurnal/dlq`.
3. Amati daftar entri DLQ.

**Hasil yang diharapkan**:
- [ ] Tabel DLQ tampil: ID, Nomor Jurnal, Error Code, Retry Count, Status, Tanggal Gagal.
- [ ] Tombol "Replay" **tidak ada di DOM** untuk `usr-akunctl-01`.
- [ ] Filter, sort berfungsi (DataTable standard).
- [ ] Badge DLQ di tab menampilkan jumlah entri FAILED (mis. "DLQ (3)").

---

### TC-009-03 — Replay DLQ: IT-ADMIN dengan MFA Step-Up (M17-04-AC3)

**Aktor**: ROLE-IT-ADMIN (`usr-itadmin-01`)  
**Pre-kondisi**: Entri DLQ `dlq-uat-001` dalam status `FAILED`.  
**Langkah**:

1. Login sebagai `usr-itadmin-01`.
2. Buka `/jurnal/dlq`.
3. Verifikasi tombol **Replay** tampil pada baris `dlq-uat-001`.
4. Klik **Replay**.
5. Dialog konfirmasi muncul: "Replay pengiriman jurnal ke GL? Entri DLQ: dlq-uat-001."
6. Klik **Konfirmasi**.
7. `MFAStepUpModal` muncul.
8. Masukkan kode TOTP yang **valid** dari `usr-itadmin-01`.
9. Klik **Verifikasi**.
10. `JobProgressPanel` muncul.
11. Tunggu selesai.

**Hasil yang diharapkan**:
- [ ] Tombol "Replay" tampil hanya untuk ROLE-IT-ADMIN.
- [ ] Dialog konfirmasi muncul sebelum MFA.
- [ ] `MFAStepUpModal` muncul setelah konfirmasi.
- [ ] Setelah MFA valid: response `202 Accepted`, `JobProgressPanel` tampil dengan step "Mengirim ulang jurnal ke GL...".
- [ ] `POST /api/v1/jurnal/dlq/{id}/replay` membawa header `X-Step-Up-Token`.
- [ ] Setelah job selesai: toast hijau "Replay DLQ dlq-uat-001 berhasil. Status jurnal: POSTED_TO_GL."
- [ ] Entri DLQ berpindah status dari `FAILED` ke `RESOLVED`.
- [ ] Badge DLQ berkurang.

**Verifikasi SQL**:
```sql
SELECT status, resolved_at, resolved_by, retry_count
FROM jrnl.dlq
WHERE id = 'dlq-uat-001';
-- Ekspektasi: RESOLVED, resolved_at IS NOT NULL, resolved_by = usr-itadmin-01

SELECT status_workflow, gl_post_at
FROM jrnl.jurnal_header
WHERE id = (SELECT jurnal_header_id FROM jrnl.dlq WHERE id = 'dlq-uat-001');
-- Ekspektasi: POSTED_TO_GL
```

**Audit check**:
```sql
SELECT action, actor_user_id, after_jsonb->>'signature_method'
FROM aud.audit_log
WHERE entity_type = 'jrnl.dlq' AND entity_id = 'dlq-uat-001'
  AND action = 'DLQ.REPLAY';
-- Ekspektasi: 1 row, signature_method = 'JWT_STEP_UP'
```

---

### TC-009-04 — MFA Salah pada Replay (M17-04-AC3)

**Aktor**: ROLE-IT-ADMIN (`usr-itadmin-01`)  
**Pre-kondisi**: `MFAStepUpModal` terbuka pada step replay.  
**Langkah**:

1. Ikuti TC-009-03 sampai `MFAStepUpModal` muncul.
2. Masukkan kode TOTP yang **salah** (mis. `999999`).
3. Klik **Verifikasi**.

**Hasil yang diharapkan**:
- [ ] Toast merah (persistent): "Kode verifikasi salah. Coba lagi."
- [ ] Modal tetap terbuka.
- [ ] Tidak ada request `POST /api/v1/jurnal/dlq/{id}/replay` yang terkirim.
- [ ] Status DLQ tetap `FAILED`.

---

### TC-009-05 — Replay Tanpa X-Step-Up-Token Ditolak API (M17-04-AC3)

**Aktor**: ROLE-IT-ADMIN (`usr-itadmin-01`)  
**Langkah** (simulasi bypass via DevTools):

1. Login sebagai `usr-itadmin-01`.
2. Kirim POST langsung:
   ```
   POST /api/v1/jurnal/dlq/dlq-uat-001/replay
   Authorization: Bearer <token valid>
   Idempotency-Key: <uuid>
   Content-Type: application/json
   -- TANPA header X-Step-Up-Token
   {}
   ```

**Hasil yang diharapkan**:
- [ ] Response: `403 FORBIDDEN`, body: `{ "error": { "code": "MFA_STEPUP_REQUIRED" } }`.
- [ ] Status DLQ tetap `FAILED`.

---

### TC-009-06 — Daftar DLQ: Filter + Export (M17-04-AC3)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/jurnal/dlq`.
3. Filter Status = `FAILED`.
4. Klik **Export CSV**.

**Hasil yang diharapkan**:
- [ ] Filter status berfungsi.
- [ ] File CSV terunduh: `dlq-failed-20260625.csv`.
- [ ] Kolom: DLQ ID, Nomor Jurnal, Error Code, Error Message, Retry Count, Status, Tanggal Gagal.
- [ ] Audit log: `DLQ.EXPORT` terekam.

---

## 3. Rollback / Cleanup

```sql
-- Reset DLQ entry UAT seed
UPDATE jrnl.dlq
SET status = 'FAILED',
    resolved_at = NULL,
    resolved_by = NULL,
    updated_at = now(),
    updated_by = 'uat-cleanup'
WHERE id = 'dlq-uat-001';
-- Hapus seed jika perlu
DELETE FROM jrnl.dlq WHERE id = 'dlq-uat-001' AND created_by = 'uat-seed';
```

---

## 4. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-IT-ADMIN (UAT Actor) | | | PASS / FAIL | |
| ROLE-AKUN-CTL (Observer UAT) | | | PASS / FAIL | |
| Product Owner | | | APPROVED / REJECT | |
