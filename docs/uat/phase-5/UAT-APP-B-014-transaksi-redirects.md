# UAT-APP-B-014 — Transaksi Redirects: 10 Aturan 308 Permanent Redirect

**UAT ID**: UAT-APP-B-014
**Modul**: APP-B — Transaction Lifecycle (Frontend Consolidation)
**Story Set**: P5-M16 / Story P5-M16-01 (penempatan) + Story P5-M16-02 (MTM)
**AC yang dicakup**: M16-01-AC1 (4 penempatan redirects), M16-02-AC1 (6 MTM redirects)
**Tanggal UAT**: _(diisi saat pelaksanaan)_
**Penyusun**: qa-engineer
**Gate**: tidak ada BLOCKING gate — redirect adalah konfigurasi Next.js; security-engineer review untuk informasi

---

## Pre-Kondisi

1. Environment UAT berjalan
2. P5-M16 deployed — `next.config.js` mengandung 10 redirect rules di fungsi `redirects()`
3. Domain UAT: `https://uat.blips.tugu-re.com`
4. Data seed minimal:
   - 1 penempatan valid dengan id `PNP-TEST-001`
   - 1 batch upload MTM valid dengan id `BATCH-MTM-001`
   - 1 MTM record valid dengan id `MTM-REC-001`
5. Tools tersedia: browser (Chrome/Firefox), `curl` (di terminal), Network DevTools

---

## Konteks Redirect

P5-M16 memindahkan URL namespace dari dua lokasi lama:
- `/trx/penempatan/*` → `/transaksi/penempatan/*` (4 rules)
- `/mtm/*` → `/transaksi/mtm/*` (6 rules)

**Urutan penting**: Path spesifik didaftarkan sebelum wildcard di `redirects()` array:
- `/mtm/alerts/stale-price` harus SEBELUM `/mtm/:id` (agar tidak ditangkap wildcard)
- `/mtm/cron` harus SEBELUM `/mtm/:id`
- `/mtm/upload` harus SEBELUM `/mtm/:id`

Kode HTTP yang diharapkan: **308 Permanent Redirect** (Next.js default untuk `permanent: true`)

---

## Skenario UAT

### TC-001 — M16-01-AC1: 4 redirect penempatan — semua path lama redirect dengan benar

**Actor**: USR-MAKER-001 (ROLE-MAKER-TR)

**Tabel 4 redirect penempatan**:

| Nomor | Path Lama | Path Baru | Status |
|---|---|---|---|
| R01 | `/trx/penempatan` | `/transaksi/penempatan` | 308 |
| R02 | `/trx/penempatan/new` | `/transaksi/penempatan/new` | 308 |
| R03 | `/trx/penempatan/PNP-TEST-001` | `/transaksi/penempatan/PNP-TEST-001` | 308 |
| R04 | `/trx/penempatan/PNP-TEST-001/edit` | `/transaksi/penempatan/PNP-TEST-001/edit` | 308 |

**Langkah**:
1. Login sebagai USR-MAKER-001
2. Buka tab baru, akses path lama R01: `/trx/penempatan`
3. Catat URL di address bar setelah halaman load
4. Ulangi untuk R02, R03, R04
5. Buka Network DevTools (tab Network, filter: All)
6. Akses `/trx/penempatan` dengan Network tab terbuka
7. Cek network entry pertama (sebelum halaman load) → cek Status code dan Location header
8. Verifikasi query string preserved: akses `/trx/penempatan?filter[stage]=2` → cek URL baru

**Hasil yang Diharapkan**:
- Langkah 2-4: Setiap path lama redirect ke path baru; browser address bar menampilkan URL `/transaksi/penempatan/...`; tidak ada 404; halaman konten render normal
- Langkah 6-7: Network entry pertama menampilkan status 308 (bukan 301/302); `Location` header: `/transaksi/penempatan`
- Langkah 8: Query string dipertahankan: URL final = `/transaksi/penempatan?filter[stage]=2`

**Verifikasi via curl**:
```bash
curl -I https://uat.blips.tugu-re.com/trx/penempatan
# Expected: HTTP/2 308
# Expected: location: /transaksi/penempatan

curl -I "https://uat.blips.tugu-re.com/trx/penempatan?filter[stage]=2"
# Expected: HTTP/2 308
# Expected: location: /transaksi/penempatan?filter%5Bstage%5D=2
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-002 — M16-02-AC1: 6 redirect MTM — semua path lama redirect dengan benar

**Actor**: USR-AKUN-001 (ROLE-AKUN)

**Tabel 6 redirect MTM**:

| Nomor | Path Lama | Path Baru | Urutan Penting |
|---|---|---|---|
| R05 | `/mtm` | `/transaksi/mtm` | — |
| R06 | `/mtm/upload` | `/transaksi/mtm/upload` | Sebelum /mtm/:id |
| R07 | `/mtm/upload/batch/BATCH-MTM-001` | `/transaksi/mtm/upload/batch/BATCH-MTM-001` | Sebelum /mtm/:id |
| R08 | `/mtm/cron` | `/transaksi/mtm/cron` | Sebelum /mtm/:id |
| R09 | `/mtm/alerts/stale-price` | `/transaksi/mtm/alerts/stale-price` | Sebelum /mtm/:id |
| R10 | `/mtm/MTM-REC-001` | `/transaksi/mtm/MTM-REC-001` | Wildcard (last) |

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Akses path lama R05 sampai R10 satu per satu
3. Catat URL di address bar setelah setiap redirect
4. Perhatikan R09 secara khusus: akses `/mtm/alerts/stale-price`
5. Verifikasi URL final R09 adalah `/transaksi/mtm/alerts/stale-price` (bukan `/transaksi/mtm/alerts`)
6. Network DevTools: cek status code dan Location untuk R09

**Hasil yang Diharapkan**:
- R05: `/transaksi/mtm` (list halaman load normal)
- R06: `/transaksi/mtm/upload` (upload page)
- R07: `/transaksi/mtm/upload/batch/BATCH-MTM-001` (batch detail)
- R08: `/transaksi/mtm/cron` (cron page)
- R09: `/transaksi/mtm/alerts/stale-price` — BUKAN `/transaksi/mtm/alerts` (wildcard tidak menangkap path ini karena R09 didaftarkan sebelum wildcard R10)
- R10: `/transaksi/mtm/MTM-REC-001` (MTM detail)
- Tidak ada 404 dari keenam path
- Semua status code: 308

**Verifikasi via curl** (khusus urutan wildcard):
```bash
# R09 — path spesifik sebelum wildcard
curl -I https://uat.blips.tugu-re.com/mtm/alerts/stale-price
# Expected: HTTP/2 308
# Expected: location: /transaksi/mtm/alerts/stale-price

# R10 — wildcard
curl -I https://uat.blips.tugu-re.com/mtm/MTM-REC-001
# Expected: HTTP/2 308
# Expected: location: /transaksi/mtm/MTM-REC-001

# Negative test — URL yang tidak ada di namespace lama tidak redirect
curl -I https://uat.blips.tugu-re.com/transaksi/mtm
# Expected: HTTP/2 200 (sudah di URL baru, tidak ada redirect loop)
```

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

### TC-003 — Negative: URL baru tidak redirect lagi (tidak ada redirect loop)

**Actor**: USR-AKUN-001 (ROLE-AKUN)

**Langkah**:
1. Login sebagai USR-AKUN-001
2. Akses langsung: `/transaksi/penempatan`
3. Akses langsung: `/transaksi/mtm`
4. Akses langsung: `/transaksi/mtm/alerts/stale-price`
5. Network DevTools: cek apakah ada redirect chain dari URL baru

**Hasil yang Diharapkan**:
- Langkah 2-4: Setiap URL baru langsung render halaman (200); TIDAK ada redirect (308/301/302) dari URL baru
- Langkah 5: Network entry untuk setiap URL: status 200 pada request pertama; tidak ada chain ke URL lain
- Tidak ada redirect loop (infinite redirect) jika pengguna mengakses URL baru yang juga cocok dengan pattern redirect

**Pass/Fail**: ☐ Pass ☐ Fail
**Catatan Tester**:

---

## Sign-Off

| Peran | Nama | Tanggal | Tanda Tangan |
|---|---|---|---|
| Tester (QA) | | | |
| Reviewer (Tech Lead) | | | |
| Security Reviewer | | | |
| Approver (Business) | | | |
