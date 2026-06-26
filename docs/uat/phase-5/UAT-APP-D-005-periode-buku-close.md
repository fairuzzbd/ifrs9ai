# UAT-APP-D-005 — Periode Buku: Soft Close & Hard Close Workflow

**Modul**: APP-D — Periode Buku & FX  
**Story**: STORY-M17-01  
**AC yang diuji**: M17-01-AC1, M17-01-AC2, M17-01-AC3, M17-01-AC4  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Periode buku Juni 2026 (`prd-2026-06`) dalam status `OPEN` | `SELECT status_close FROM sys.periode WHERE bulan = '2026-06-01'` → `OPEN` |
| P2 | Semua jurnal Juni 2026 berstatus `POSTED_TO_GL` (tidak ada DRAFT) | `SELECT COUNT(*) FROM jrnl.jurnal_header WHERE periode_id = 'prd-2026-06' AND status_workflow != 'POSTED_TO_GL'` → `0` |
| P3 | Tersedia 4 user aktif: AKUN-CTL (`usr-akunctl-01`), CFO (`usr-cfo-01`), RISK (`usr-risk-01`), AUDIT (`usr-audit-01`) | Keycloak → Users |
| P4 | User AKUN-CTL memiliki MFA terdaftar (TOTP) | Keycloak → usr-akunctl-01 → Credentials |
| P5 | User CFO memiliki MFA terdaftar (TOTP) | Keycloak → usr-cfo-01 → Credentials |
| P6 | Layar `/periode-buku` dapat diakses dari browser (Chrome terbaru) | Buka `https://blips.tugu-re.com/periode-buku` |

---

## 2. Data uji

```
Periode ID  : prd-2026-06
Bulan       : Juni 2026 (2026-06-01)
Nama        : Periode Buku Juni 2026
Status awal : OPEN
```

---

## 3. Skenario Uji

### TC-005-01 — Timeline Sidebar Tampil Benar (M17-01-AC1)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `https://blips.tugu-re.com/periode-buku`.
3. Klik periode "Juni 2026" pada daftar.
4. Amati sidebar/panel kanan yang menampilkan riwayat status.

**Hasil yang diharapkan**:
- [ ] Halaman `/periode-buku/prd-2026-06` terbuka tanpa error.
- [ ] Sidebar menampilkan timeline dengan node: `OPEN` → `SOFT_CLOSED` → `HARD_CLOSED`.
- [ ] Node `OPEN` ditandai aktif (warna berbeda / badge).
- [ ] Timestamp "Dibuat" tercantum pada node OPEN.
- [ ] Tombol "Soft Close" tampil untuk AKUN-CTL.
- [ ] Tombol "Hard Close" **tidak ada** di DOM (bukan hanya disembunyikan).

**Verifikasi SQL**: `SELECT status_close FROM sys.periode WHERE id = 'prd-2026-06'` → `OPEN`

---

### TC-005-02 — Soft Close oleh ROLE-AKUN-CTL (M17-01-AC2)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Pre-kondisi**: Periode dalam status `OPEN`.  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/periode-buku/prd-2026-06`.
3. Klik tombol **Soft Close**.
4. Dialog konfirmasi muncul: "Soft-close periode Juni 2026? Periode masih dapat di-reopen untuk koreksi."
5. Masukkan kode MFA (TOTP `usr-akunctl-01`).
6. Klik **Konfirmasi**.
7. Amati toast dan status berubah.

**Hasil yang diharapkan**:
- [ ] Dialog konfirmasi tampil sebelum aksi.
- [ ] Input MFA muncul (step-up per DEC-026 untuk ROLE-AKUN-CTL).
- [ ] Setelah konfirmasi: toast hijau "Periode Juni 2026 berhasil di-soft-close. Status: SOFT_CLOSED."
- [ ] Timeline sidebar: node `SOFT_CLOSED` sekarang aktif, timestamp tercantum.
- [ ] Tombol "Soft Close" hilang; tombol "Reopen" muncul untuk AKUN-CTL.
- [ ] Tombol "Hard Close" tetap **tidak ada** di DOM untuk AKUN-CTL.

**Verifikasi SQL**:
```sql
SELECT status_close, soft_closed_at, soft_closed_by
FROM sys.periode WHERE id = 'prd-2026-06';
-- Ekspektasi: status_close = 'SOFT_CLOSED', soft_closed_at IS NOT NULL, soft_closed_by = usr-akunctl-01
```

**Audit check**:
```sql
SELECT action, actor_user_id, after_jsonb
FROM aud.audit_log
WHERE entity_type = 'sys.periode' AND entity_id = 'prd-2026-06'
  AND action = 'PERIODE.SOFT_CLOSE'
ORDER BY event_time DESC LIMIT 1;
-- Ekspektasi: 1 row, actor_user_id = usr-akunctl-01
```

---

### TC-005-03 — Hard Close oleh ROLE-CFO + MFA Step-Up (M17-01-AC3)

**Aktor**: ROLE-CFO (`usr-cfo-01`)  
**Pre-kondisi**: Periode dalam status `SOFT_CLOSED`.  
**Langkah**:

1. Login sebagai `usr-cfo-01`.
2. Buka `/periode-buku/prd-2026-06`.
3. Verifikasi tombol "Hard Close" tampil (hanya untuk CFO).
4. Klik tombol **Hard Close**.
5. Dialog konfirmasi muncul: "Setelah hard-close, periode tidak dapat di-reopen. Lanjut?"
6. Klik **Konfirmasi**.
7. `MFAStepUpModal` muncul — masukkan kode TOTP yang valid (`usr-cfo-01`).
8. Klik **Verifikasi**.
9. Amati toast dan status berubah.

**Hasil yang diharapkan**:
- [ ] Tombol "Hard Close" hanya tampil untuk ROLE-CFO, tidak tampil untuk ROLE-AKUN-CTL.
- [ ] `MFAStepUpModal` muncul setelah dialog konfirmasi.
- [ ] Setelah MFA benar: toast hijau "Periode Juni 2026 berhasil hard-closed. Tidak dapat di-reopen."
- [ ] Status berubah menjadi `HARD_CLOSED`.
- [ ] Timeline sidebar: node `HARD_CLOSED` aktif, timestamp CFO + nama user tercantum.
- [ ] Tombol "Hard Close", "Soft Close", dan "Reopen" semua **tidak ada** di DOM setelah HARD_CLOSED.
- [ ] Request POST `/api/v1/periode/prd-2026-06/hardclose` membawa header `X-Step-Up-Token`.

**Verifikasi SQL**:
```sql
SELECT status_close, hard_closed_at, hard_closed_by
FROM sys.periode WHERE id = 'prd-2026-06';
-- Ekspektasi: HARD_CLOSED, hard_closed_at IS NOT NULL, hard_closed_by = usr-cfo-01
```

**Audit check**:
```sql
SELECT action, actor_user_id, after_jsonb->>'signature_method' as sig
FROM aud.audit_log
WHERE entity_type = 'sys.periode' AND entity_id = 'prd-2026-06'
  AND action = 'PERIODE.HARD_CLOSE'
ORDER BY event_time DESC LIMIT 1;
-- Ekspektasi: 1 row, sig = 'JWT_STEP_UP'
```

---

### TC-005-04 — MFA Code Salah pada Hard Close (M17-01-AC3)

**Aktor**: ROLE-CFO (`usr-cfo-01`)  
**Pre-kondisi**: Periode `SOFT_CLOSED`, CFO login, MFAStepUpModal terbuka.  
**Langkah**:

1. Ikuti langkah TC-005-03 sampai `MFAStepUpModal` muncul.
2. Masukkan kode MFA yang **salah** (mis. `000000`).
3. Klik **Verifikasi**.
4. Amati pesan error.

**Hasil yang diharapkan**:
- [ ] Toast merah (persistent): "Kode verifikasi salah. Silakan coba lagi."
- [ ] Modal tetap terbuka (tidak ditutup).
- [ ] Status periode tetap `SOFT_CLOSED` (tidak berubah).
- [ ] Tidak ada row baru di `aud.audit_log` untuk `PERIODE.HARD_CLOSE`.

---

### TC-005-05 — Gating: ROLE-AKUN-CTL tidak bisa Hard Close (M17-01-AC4)

**Aktor**: ROLE-AKUN-CTL (`usr-akunctl-01`)  
**Pre-kondisi**: Periode `SOFT_CLOSED`.  
**Langkah**:

1. Login sebagai `usr-akunctl-01`.
2. Buka `/periode-buku/prd-2026-06`.
3. Inspeksi DOM halaman.

**Hasil yang diharapkan**:
- [ ] Tombol "Hard Close" **tidak ditemukan di DOM sama sekali** (bukan disembunyikan via CSS).
- [ ] Browser DevTools → Elements: tidak ada elemen `button` atau `[data-action="hard-close"]`.
- [ ] Coba kirim POST manual ke `/api/v1/periode/prd-2026-06/hardclose` tanpa step-up token → response `403 FORBIDDEN`.

---

### TC-005-06 — ROLE-AUDIT: Baca Semua, Tidak Ada Tombol Aksi (M17-01-AC4)

**Aktor**: ROLE-AUDIT (`usr-audit-01`)  
**Pre-kondisi**: Periode dalam status apapun.  
**Langkah**:

1. Login sebagai `usr-audit-01`.
2. Buka `/periode-buku`.
3. Klik periode "Juni 2026".
4. Amati halaman detail.

**Hasil yang diharapkan**:
- [ ] Halaman terbuka, data periode tampil.
- [ ] **Tidak ada** tombol "Soft Close", "Hard Close", "Reopen" di DOM.
- [ ] Export riwayat tersedia (jika ada tombol export pada audit log viewer).

---

## 4. Verifikasi Audit Trail

Setelah TC-005-02 dan TC-005-03 dijalankan:

```sql
SELECT action, actor_user_id, event_time
FROM aud.audit_log
WHERE entity_type = 'sys.periode' AND entity_id = 'prd-2026-06'
ORDER BY event_time;
```

**Ekspektasi urutan**:
1. `PERIODE.SOFT_CLOSE` — actor: `usr-akunctl-01`
2. `PERIODE.HARD_CLOSE` — actor: `usr-cfo-01`

Hash chain check:
```sql
SELECT current_hash, previous_hash, event_id
FROM aud.audit_log
WHERE entity_type = 'sys.periode'
ORDER BY event_time;
-- Run: go run cmd/audit-verify --entity sys.periode --id prd-2026-06
```

---

## 5. Rollback / Cleanup

> **PERHATIAN**: Hard close **tidak dapat di-reverse** secara normal. Cleanup hanya untuk lingkungan UAT.

```sql
-- UAT ONLY — restore periode ke OPEN
UPDATE sys.periode
SET status_close = 'OPEN',
    soft_closed_at = NULL,
    soft_closed_by = NULL,
    hard_closed_at = NULL,
    hard_closed_by = NULL,
    updated_at = now(),
    updated_by = 'uat-cleanup',
    row_version = row_version + 1
WHERE id = 'prd-2026-06';
-- Hapus audit log entries test (UAT ONLY)
DELETE FROM aud.audit_log
WHERE entity_id = 'prd-2026-06' AND trace_id LIKE 'uat-%';
```

---

## 6. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-AKUN-CTL (UAT Actor) | | | PASS / FAIL | |
| ROLE-CFO (UAT Actor) | | | PASS / FAIL | |
| Product Owner | | | APPROVED / REJECT | |
