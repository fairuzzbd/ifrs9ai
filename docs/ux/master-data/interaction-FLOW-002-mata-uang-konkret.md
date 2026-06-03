# Interaction FLOW-002 — Mata Uang Konkret: Flow Lengkap + Edge Cases

**Flow ID**: FLOW-002  
**Berlaku untuk**: `mst.mata_uang` (pilot pola generik)  
**Story**: APP-A-MSTR-002  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

---

## Happy Path — Lifecycle Lengkap GBP

### Step 1: AKUN Maker membuat GBP

**URL**: `/master/mata-uang/new`  
**Pre-condition**: User login sebagai `akun.maker` (ROLE-AKUN)

1. User mengakses halaman list `/master/mata-uang`
2. Klik "+ Tambah Mata Uang" → navigate ke `/master/mata-uang/new`
3. Form kosong. Idempotency-Key baru dibuat saat halaman mount.
4. User mengisi:
   - Kode Mata Uang: `GBP` (uppercase otomatis, max 3 char)
   - Nama Mata Uang: `Pound Sterling`
   - Simbol: `£`
   - Decimal Places: `2` (spinner)
   - Sumber Kurs Default: `BI Kurs Tengah` (dropdown)
   - Frekuensi Update: `Harian` (dropdown)
   - Tanggal Mulai Aktif: `2026-06-03` (date picker)
   - Status Aktif: toggle ON (default)
5. User klik "Simpan"
   - Tombol disabled + spinner
   - `POST /api/v1/master/mata-uang` dengan Idempotency-Key
   - Response 201
6. Toast hijau: "Mata uang GBP — Pound Sterling berhasil dibuat. Menunggu review Finance Controller. [Lihat detail →]"
7. Redirect ke `/master/mata-uang/GBP`

### Step 2: AKUN Maker submit GBP untuk review

**URL**: `/master/mata-uang/GBP`  
**State**: DRAFT

1. Halaman detail GBP tampil
2. Badge: `○ Draf`
3. Di panel workflow: Step 1 (Maker) = aktif, tombol "Kirim untuk Review" di bagian bawah workflow panel
4. User klik "Kirim untuk Review"
   - Confirm dialog tidak perlu (bukan aksi destructive)
   - `POST .../submit` dengan rowVersion=1
   - Response: `{currentState: PENDING_REVIEW}`
5. Badge berubah ke `◑ Menunggu Review`
6. Toast: "GBP berhasil dikirim untuk review Finance Controller."
7. In-app notifikasi dikirim ke semua user ROLE-AKUN-CTL

### Step 3: AKUN-CTL review GBP

**URL**: `/master/mata-uang/GBP` (diakses oleh `akun.ctl.1`)  
**State**: PENDING_REVIEW

1. `akun.ctl.1` menerima notifikasi, klik → navigate ke GBP
2. Panel workflow: Step 2 (Reviewer) = aktif
3. `akun.ctl.1` BUKAN maker GBP → SoD OK → action area muncul
4. Isi komentar (opsional): "Review OK — kode ISO valid, decimal places sesuai standar."
5. Centang attest checkbox
6. Klik "Setujui & Lanjutkan"
   - `POST .../review` dengan signatureMethod=JWT_STANDARD, rowVersion=1
   - Server check: mfa_verified=true ✓, reviewer≠maker ✓
   - Response: `{currentState: PENDING_APPROVAL}`
7. Toast: "Review GBP berhasil. Menunggu approval final Finance Controller."
8. Badge: `◑ Menunggu Approval`

### Step 4: AKUN-CTL ke-2 approve GBP

**URL**: `/master/mata-uang/GBP` (diakses oleh `akun.ctl.2`)  
**State**: PENDING_APPROVAL

1. `akun.ctl.2` (berbeda dari `akun.ctl.1`) menerima notifikasi
2. Panel: Step 3 (Approver) = aktif
3. SoD: `akun.ctl.2` ≠ maker (`akun.maker`) ✓ dan ≠ reviewer (`akun.ctl.1`) ✓
4. Isi komentar: "Disetujui. Mata uang GBP aktif per 2026-06-03."
5. Centang attest
6. Klik "Setujui & Lanjutkan"
   - `POST .../approve` dengan signatureMethod=JWT_STANDARD, rowVersion=1
   - Response: `{currentState: APPROVED}`
7. Toast: "Mata uang GBP berhasil disetujui."
8. Badge: `● Disetujui`
9. In-app notifikasi ke `akun.maker`: "Mata uang GBP yang Anda buat telah disetujui."
10. Panel workflow menampilkan semua 3 step ✓ dengan signature hash

---

## Edge Case: Kode Mata Uang Duplikat

**Skenario**: User input GBP padahal GBP sudah ada di sistem.

1. User mengisi form dengan kode `GBP` (sudah ada)
2. Klik "Simpan"
3. Client-side: tidak ada duplicate check (butuh API call)
4. `POST /api/v1/master/mata-uang` → 409 CONFLICT
5. Field "Kode Mata Uang" mendapat error inline:
   - Border merah
   - `⚠ Mata uang GBP sudah terdaftar di sistem.`
6. Toast merah: "1 field bermasalah — lihat form di bawah. Trace: {id}"
7. User mengubah kode atau membatalkan

---

## Edge Case: Decimal Places = 0 (IDR)

**Skenario**: User membuat IDR dengan decimal_places=0.

Setelah APPROVED, di dropdown mata uang di form instrumen:
- IDR tampil dengan format amount tanpa desimal (Rp 1.000.000)
- Komponen `CurrencyAmountInput` membaca `decimalPlaces` dari mata_uang data dan menyesuaikan format

---

## Edge Case: Nonaktifkan mata uang GBP (aktif_flag = false)

**Skenario**: GBP tidak lagi digunakan, ingin dinonaktifkan.

1. GBP status APPROVED, `aktif_flag = true`
2. `akun.maker` akses edit GBP
   - Halaman detail: karena APPROVED, form edit tidak langsung ada
   - User klik ••• → "Ajukan Perubahan"
   - Ini trigger "amendment workflow": PUT ke APPROVED record → backend reset ke DRAFT + workflow baru
3. Form edit muncul. `aktif_flag` toggle di-set ke OFF
4. User simpan → DRAFT
5. Submit → Review → Approve (siklus normal)
6. Setelah APPROVE dengan `aktif_flag = false`:
   - GBP tidak muncul di dropdown mata uang form instrumen baru
   - GBP tetap di list dengan badge `● Disetujui` tapi ada tanda visual: label "(Tidak Aktif)"
   - Data historis instrumen yang pakai GBP tidak terpengaruh

**Catatan**: Jika GBP masih punya referensi aktif dan user coba set `aktif_flag = false`:
- Server return 422 `ACTIVE_FLAG_REF_CONFLICT`
- Toast merah: "Mata uang GBP tidak bisa dinonaktifkan karena masih digunakan oleh 3 instrumen aktif. Selesaikan instrumen tersebut terlebih dahulu."

---

## Edge Case: Soft-Delete CHF (tanpa referensi)

**Skenario**: CHF status DRAFT, belum punya referensi.

1. User akses list, temukan CHF dengan status DRAFT
2. Klik ••• → "Hapus"
3. Confirm dialog:
   ```
   Hapus Mata Uang CHF?
   ────────────────────────────────────────
   Mata uang "CHF — Franc Swiss" akan dihapus.
   Data yang terhubung tidak akan terpengaruh.
   Aksi ini tidak dapat dibatalkan.

   [Batal]                     [Hapus]
   ```
4. User klik "Hapus"
5. `DELETE /api/v1/master/mata-uang/CHF` dengan Idempotency-Key
6. Response 200 `{deleted: true}`
7. Toast: "Mata uang CHF berhasil dihapus dari sistem."
8. CHF menghilang dari list (soft-deleted, `deleted_at` ter-set)

---

## Edge Case: Soft-Delete USD (punya referensi aktif)

**Skenario**: USD direferensikan 12 instrumen.

1. User klik ••• → "Hapus" di baris USD
2. Confirm dialog muncul (user belum tahu ada referensi)
3. User klik "Hapus" di dialog
4. `DELETE .../USD` → 409 ENTITY_IN_USE
5. Toast merah: "Mata uang USD tidak bisa dihapus karena masih digunakan oleh 12 instrumen. Nonaktifkan mata uang ini dengan mengubah aktif_flag menjadi false."
6. Dialog auto-close
7. User mempertimbangkan alternatif (nonaktifkan, bukan hapus)

---

## Edge Case: Soft-Delete IDR (system currency)

**Skenario**: User mencoba hapus IDR.

1. User klik ••• pada IDR
2. **Tombol "Hapus" tidak ada di menu** untuk IDR (karena `isSystemCurrency = true` ditampilkan di frontend → menu item dihapus)
3. Jika user bypass UI dan panggil API langsung: 403 `SYSTEM_CURRENCY_PROTECTED`

---

## Edge Case: Idempotency Replay

**Skenario**: User klik "Simpan" dua kali (misal koneksi lambat, klik ganda).

1. Klik pertama: `POST` dengan Idempotency-Key `uuid-A` → 201
2. Klik kedua (dari Idempotency-Key yang sama karena dibuat saat mount): 200 dengan `IDEMPOTENCY_REPLAY`
3. Frontend treat kedua response sebagai sukses
4. Toast dan redirect hanya satu kali (tidak duplikat — karena klik kedua sebenarnya tidak bisa terjadi karena tombol sudah disabled)

---

## Copy Text Matrix (ID / EN)

### Halaman List

| ID | EN |
|---|---|
| Daftar Mata Uang | Currency List |
| + Tambah Mata Uang | + Add Currency |
| Cari kode, nama, simbol... | Search code, name, symbol... |
| Hapus semua | Clear all |
| Terakhir: {waktu} WIB | Last updated: {time} |
| {N} dari ~{M} | {N} of ~{M} |

### Form

| ID | EN |
|---|---|
| Kode Mata Uang | Currency Code |
| Nama Mata Uang | Currency Name |
| Simbol | Symbol |
| Decimal Places | Decimal Places |
| Sumber Kurs Default | Default Exchange Rate Source |
| Frekuensi Update | Update Frequency |
| Tanggal Mulai Aktif | Effective Start Date |
| Status Aktif | Active Status |
| Mata uang aktif | Currency is active |
| Simpan | Save |
| Batal | Cancel |

### Workflow Panel

| ID | EN |
|---|---|
| Proses Persetujuan | Approval Process |
| Pembuat (Maker) | Creator (Maker) |
| Pemeriksa (Reviewer) | Reviewer |
| Pemberi Persetujuan (Approver) | Approver |
| Kirim untuk Review | Submit for Review |
| Kirim Ulang untuk Review | Resubmit for Review |
| Setujui & Lanjutkan | Approve & Continue |
| Tolak & Kembalikan | Reject & Return |
| Alasan Penolakan | Rejection Reason |
| Saya telah memeriksa... | I have reviewed... |
| Menunggu review dari Finance Controller | Awaiting Finance Controller review |

### Status Badge

| ID | EN |
|---|---|
| Draf | Draft |
| Menunggu Review | Pending Review |
| Menunggu Approval | Pending Approval |
| Menunggu Approval 2 | Pending Second Approval |
| Disetujui | Approved |
| Dikembalikan | Returned |

### Toast Messages

| Skenario | Pesan ID |
|---|---|
| Create sukses | "Mata uang {kode} — {nama} berhasil dibuat. Menunggu review Finance Controller." |
| Update sukses | "Mata uang {kode} — {nama} berhasil diperbarui." |
| Submit sukses | "{kode} berhasil dikirim untuk review Finance Controller." |
| Review sukses | "Review {kode} berhasil. Menunggu approval final Finance Controller." |
| Approve sukses | "Mata uang {kode} berhasil disetujui." |
| Reject sukses | "{kode} dikembalikan ke {maker_name}." |
| Delete sukses | "Mata uang {kode} berhasil dihapus dari sistem." |
| SoD violation | "Anda tidak bisa menjadi reviewer/approver untuk mata uang yang Anda buat sendiri." |
| MFA diperlukan | "Multi-Factor Authentication wajib untuk Finance Controller. Silakan login ulang dengan MFA." |
| Conflict (lock) | "Mata uang {kode} telah diubah oleh {user}. Muat ulang halaman untuk melihat data terbaru." |
| Entity in use | "Mata uang {kode} tidak bisa dihapus karena masih digunakan oleh {N} {entity}. Nonaktifkan mata uang ini sebagai alternatif." |
| Export sukses | "Export {format} selesai. [Unduh file (berlaku 24 jam) →]" |
| Validation error | "{N} field bermasalah — lihat form di bawah. Trace: {traceId}" |
