# UAT-APP-D-011 — 308 Permanent Redirects: 22 Aturan M17

**Modul**: APP-D — Navigation & Redirect Rules  
**Story**: STORY-M17-01-AC1, STORY-M17-03-AC1, STORY-M17-04-AC4 (cross-cutting)  
**AC yang diuji**: M17-01-AC1, M17-03-AC1, M17-04-AC4  
**Tanggal dokumen**: 2026-06-25  
**Dibuat oleh**: qa-engineer  
**Status**: DRAFT

---

## 1. Pre-kondisi

| # | Kondisi | Cara memverifikasi |
|---|---|---|
| P1 | Aplikasi frontend berjalan (Next.js) | `curl -I https://blips.tugu-re.com/health` → 200 |
| P2 | User ROLE-IT-ADMIN login (`usr-itadmin-01`) — akses semua rute | Keycloak → Users |
| P3 | `next.config.js` telah deploy dengan 32 redirect rules (10 M16 + 22 M17) | CI deploy log |

---

## 2. Skenario Uji

### TC-011-01 — Periode Buku Redirects (5 aturan) (M17-01-AC1)

**Aktor**: ROLE-IT-ADMIN (`usr-itadmin-01`)

| # | URL Lama (Legacy) | URL Baru (Canonical) | HTTP |
|---|---|---|---|
| 1 | `/master/periode-buku` | `/periode-buku` | 308 |
| 2 | `/master/periode-buku/new` | `/periode-buku/new` | 308 |
| 3 | `/master/periode-buku/prd-2026-06` | `/periode-buku/prd-2026-06` | 308 |
| 4 | `/master/periode-buku/prd-2026-06/edit` | `/periode-buku/prd-2026-06/edit` | 308 |
| 5 | `/master/periode-buku/prd-2026-06/history` | `/periode-buku/prd-2026-06/history` | 308 |

**Langkah per aturan**:

1. Login sebagai `usr-itadmin-01`.
2. Buka browser, ketik URL Lama di address bar → tekan Enter.
3. Amati URL setelah navigasi selesai.
4. Verifikasi halaman tujuan tidak 404.

**Hasil yang diharapkan**:
- [ ] URL browser berubah ke URL Baru.
- [ ] Tidak ada 404 pada halaman tujuan.
- [ ] Tidak ada infinite redirect loop.

---

### TC-011-02 — Mapping Jurnal Namespace 1 (3 aturan) (M17-03-AC1)

**Aktor**: ROLE-IT-ADMIN (`usr-itadmin-01`)

| # | URL Lama | URL Baru | HTTP |
|---|---|---|---|
| 6 | `/mapping-jurnal` | `/master/mapping-jurnal` | 308 |
| 7 | `/mapping-jurnal/import` | `/master/mapping-jurnal/new` | 308 |
| 8 | `/mapping-jurnal/DEPOSITO_INT` | `/master/mapping-jurnal/DEPOSITO_INT` | 308 |

**Langkah**: sama seperti TC-011-01.

**Hasil yang diharapkan**:
- [ ] Aturan 6, 7, 8 masing-masing redirect ke URL Baru tanpa error.
- [ ] Aturan 7: `/mapping-jurnal/import` → `/master/mapping-jurnal/new` (nama berubah).

---

### TC-011-03 — Mapping Jurnal Namespace 2 (4 aturan) (M17-03-AC1)

| # | URL Lama | URL Baru | HTTP |
|---|---|---|---|
| 9 | `/jrnl/mapping` | `/master/mapping-jurnal` | 308 |
| 10 | `/jrnl/mapping/new` | `/master/mapping-jurnal/new` | 308 |
| 11 | `/jrnl/mapping/mj-001` | `/master/mapping-jurnal/mj-001` | 308 |
| 12 | `/jrnl/mapping/mj-001/edit` | `/master/mapping-jurnal/mj-001/edit` | 308 |

**Hasil yang diharapkan**:
- [ ] Aturan 9–12 masing-masing redirect ke URL Baru tanpa error.

---

### TC-011-04 — Jurnal Namespace (10 aturan) (M17-04-AC4)

| # | URL Lama | URL Baru | HTTP |
|---|---|---|---|
| 13 | `/jrnl/journal-entries` | `/jurnal/header` | 308 |
| 14 | `/jrnl/journal-entries/jrn-2026-0042` | `/jurnal/header/jrn-2026-0042` | 308 |
| 15 | `/jrnl/gl-delivery-dlq` | `/jurnal/dlq` | 308 |
| 16 | `/jrnl/gl-delivery-dlq/dlq-001` | `/jurnal/dlq/dlq-001` | 308 |
| 17 | `/jrnl/dlq` | `/jurnal/dlq` | 308 |
| 18 | `/jrnl/dlq/dlq-001` | `/jurnal/dlq/dlq-001` | 308 |
| 19 | `/jrnl/resolve` | `/jurnal/resolve` | 308 |
| 20 | `/jrnl/post` | `/jurnal/header` | 308 |
| 21 | `/jrnl/rekonsiliasi` | `/reconciliation/daily` | 308 |
| 22 | `/jrnl/rekonsiliasi/riwayat` | `/reconciliation/daily` | 308 |

**Hasil yang diharapkan**:
- [ ] Aturan 13–22 masing-masing redirect ke URL Baru.
- [ ] Aturan 15 dan 17 keduanya mengarah ke `/jurnal/dlq` (dua alias yang sama).
- [ ] Aturan 21 dan 22 keduanya mengarah ke `/reconciliation/daily`.

---

### TC-011-05 — Query String Dipertahankan (M17-04-AC4)

**Langkah**:

1. Login sebagai `usr-itadmin-01`.
2. Buka `/jrnl/journal-entries?filter[status_workflow]=DRAFT&sort=tanggal_jurnal:desc`.
3. Amati URL setelah redirect.

**Hasil yang diharapkan**:
- [ ] URL akhir: `/jurnal/header?filter[status_workflow]=DRAFT&sort=tanggal_jurnal:desc`.
- [ ] Query string dipertahankan (tidak hilang setelah redirect).
- [ ] Halaman `/jurnal/header` menerapkan filter dan sort dari URL.

---

### TC-011-06 — Tidak Ada Redirect Loop (semua 22 aturan)

**Langkah**:

1. Buka setiap URL Baru (canonical) langsung di browser:
   - `/periode-buku`
   - `/master/mapping-jurnal`
   - `/jurnal/header`
   - `/jurnal/dlq`
   - `/jurnal/resolve`
   - `/reconciliation/daily`

**Hasil yang diharapkan**:
- [ ] Tidak ada redirect dari URL canonical ke URL lain.
- [ ] Browser tidak menampilkan "Too many redirects" error.
- [ ] Halaman tujuan load normal.

---

### TC-011-07 — Regression: M16 Redirects Masih Berfungsi

| # | URL Lama M16 | URL Baru |
|---|---|---|
| R1 | `/trx/penempatan` | `/transaksi/penempatan` |
| R2 | `/trx/penempatan/new` | `/transaksi/penempatan/new` |
| R3 | `/trx/penempatan/pnp-001` | `/transaksi/penempatan/pnp-001` |
| R4 | `/mtm` | `/transaksi/mtm` |
| R5 | `/mtm/upload` | `/transaksi/mtm/upload` |

**Langkah**: Buka setiap URL lama M16 → verifikasi redirect ke URL baru M16.

**Hasil yang diharapkan**:
- [ ] R1–R5 masing-masing redirect ke URL baru M16 tanpa error.
- [ ] Penambahan 22 aturan M17 tidak mempengaruhi 10 aturan M16 sebelumnya.

---

## 3. Cara Verifikasi HTTP Response

Jika ingin memverifikasi HTTP status code 308 secara eksplisit (bukan browser yang sudah follow redirect):

```bash
# Verifikasi 308 tanpa follow redirect
curl -I --max-redirs 0 https://blips.tugu-re.com/master/periode-buku
# Ekspektasi: HTTP/2 308
# Location: /periode-buku

curl -I --max-redirs 0 https://blips.tugu-re.com/jrnl/journal-entries
# Ekspektasi: HTTP/2 308
# Location: /jurnal/header

curl -I --max-redirs 0 https://blips.tugu-re.com/jrnl/rekonsiliasi
# Ekspektasi: HTTP/2 308
# Location: /reconciliation/daily
```

---

## 4. Rollback / Cleanup

Redirect rules ada di `next.config.js`. Tidak ada perubahan database diperlukan.

Jika ada rule yang perlu dihapus: update `next.config.js` dan redeploy.

---

## 5. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Tester | | | PASS / FAIL | |
| ROLE-IT-ADMIN (UAT Actor) | | | PASS / FAIL | |
| Frontend Engineer | | | KONFIRMASI DEPLOY | |
| Product Owner | | | APPROVED / REJECT | |
