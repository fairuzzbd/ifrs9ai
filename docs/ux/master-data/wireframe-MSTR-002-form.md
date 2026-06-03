# Wireframe MSTR-002-FORM — Form Create / Edit Master Data (Pola Generik)

**Screen ID**: MSTR-002-FORM  
**Berlaku untuk**: Semua 16 modul `mst.*` — konkret `mata_uang`  
**Story**: APP-A-MSTR-001 §Feature:Create, §Feature:Update; APP-A-MSTR-002  
**Author**: uiux-designer  
**Tanggal**: 2026-06-03

---

## Layout — Form Create Mata Uang

```
┌────────────────────────────────────────────────────────────────────────────────┐
│  BLIPS IFRS9   [nav]                                    [user badge] [notif]   │
├────────────────────────────────────────────────────────────────────────────────┤
│                                                                                │
│  ▸ Master Data  /  Mata Uang  /  Tambah Mata Uang        [breadcrumb]         │
│                                                                                │
│  Tambah Mata Uang                                                              │
│  ─────────────────────────────────────────────────────────                     │
│                                                                                │
│  ┌─────────────────────────────────────────────────────────────────────────┐  │
│  │  INFORMASI DASAR                                              [seksi]   │  │
│  │  ─────────────────────────────────────────────────────────────────────  │  │
│  │                                                                         │  │
│  │  Kode Mata Uang *                     Nama Mata Uang *                  │  │
│  │  ┌──────────────────────┐             ┌────────────────────────────┐   │  │
│  │  │ IDR                  │             │ Rupiah Indonesia           │   │  │
│  │  └──────────────────────┘             └────────────────────────────┘   │  │
│  │  3 huruf kapital, ISO 4217            Min 3 karakter                   │  │
│  │                                                                         │  │
│  │  Simbol *                             Decimal Places *                  │  │
│  │  ┌──────────────────────┐             ┌────────────────────────────┐   │  │
│  │  │ Rp                   │             │ 0                     [↕]  │   │  │
│  │  └──────────────────────┘             └────────────────────────────┘   │  │
│  │  Mis: Rp, $, €, £, S$                0–4 desimal (IDR=0, USD=2)        │  │
│  │                                                                         │  │
│  │  Sumber Kurs Default *                Frekuensi Update *                │  │
│  │  ┌──────────────────────────────┐    ┌────────────────────────────┐   │  │
│  │  │ BI JISDOR             ▾      │    │ Harian                ▾    │   │  │
│  │  └──────────────────────────────┘    └────────────────────────────┘   │  │
│  │                                                                         │  │
│  │  Tanggal Mulai Aktif *                                                  │  │
│  │  ┌──────────────────────────────┐                                       │  │
│  │  │ 2026-06-03        [kalender] │                                       │  │
│  │  └──────────────────────────────┘                                       │  │
│  │  Tidak boleh di masa depan                                              │  │
│  │                                                                         │  │
│  │  Status Aktif                                                           │  │
│  │  ┌──────────────────────────────┐                                       │  │
│  │  │ [Toggle ON]  Mata uang aktif │                                       │  │
│  │  └──────────────────────────────┘                                       │  │
│  │  Jika tidak aktif, mata uang tidak bisa dipilih di form instrumen baru │  │
│  └─────────────────────────────────────────────────────────────────────────┘  │
│                                                                                │
│  ─────────────────────────────────────────────────────────────                 │
│                        [Batal]    [Simpan]                                     │
│                                                                                │
└────────────────────────────────────────────────────────────────────────────────┘
```

---

## Layout — Form Edit (status DRAFT atau RETURNED)

Sama dengan Create, dengan perbedaan:

1. Header: "Edit Mata Uang — GBP"
2. **Field "Kode Mata Uang" = read-only** (disabled, warna abu) dengan tooltip:
   > "Kode mata uang tidak bisa diubah setelah dibuat. Untuk mengganti kode, nonaktifkan mata uang ini dan buat baru."
3. Untuk status RETURNED: banner kuning di atas form (lihat bawah)
4. Field `rowVersion` tersimpan sebagai hidden input (dikirim otomatis saat PUT)

---

## Banner Status RETURNED

Muncul di atas form ketika `workflowStatus = RETURNED`:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  ↩  Dikembalikan oleh: akun.ctl.1 — Finance Controller                     │
│     2026-06-03, 14:22 WIB                                                   │
│                                                                              │
│     "Kode GBP sudah ada di sistem dengan nama berbeda. Harap verifikasi    │
│      nama resmi sesuai ISO 4217."                                            │
└─────────────────────────────────────────────────────────────────────────────┘
```

Warna: amber-50 border amber-300. Ikon ↩. Teks komentar penolakan dalam tanda kutip. Seluruh riwayat penolakan tersedia di tab "Riwayat" di halaman detail.

---

## Form Detail: Field Specification (mata_uang konkret)

| Field | Label (ID) | Tipe Input | Validasi | Catatan |
|---|---|---|---|---|
| kode_mata_uang | Kode Mata Uang | text input (max 3 char, uppercase) | Required, pattern `^[A-Z]{3}$`, unique check | Immutable saat edit — disabled |
| nama_mata_uang | Nama Mata Uang | text input | Required, min 3, max 60 | - |
| simbol | Simbol | text input (max 5 char) | Required, min 1, max 5 | - |
| decimal_places | Decimal Places | number input, spinner 0-4 | Required, integer, 0-4 | Helper: "IDR=0, USD=2, JPY=0, KWD=3" |
| sumber_kurs_default | Sumber Kurs Default | Select | Required, enum | Options: BI JISDOR / BI Kurs Tengah / Internal |
| frekuensi_update | Frekuensi Update | Select | Required, enum | Options: Harian / Intra Day / Bulanan |
| tanggal_mulai_aktif | Tanggal Mulai Aktif | Date picker | Required, max = hari ini | Error: "Tanggal tidak boleh di masa depan" |
| aktif_flag | Status Aktif | Toggle switch | Opsional, default ON | - |

---

## Proteksi `is_system_currency` (IDR)

Untuk mata uang dengan `isSystemCurrency = true`:

1. Tombol "Hapus" **tidak tampil** di action menu list (bukan disabled — benar-benar tersembunyi)
2. Pada halaman detail: badge "Mata uang fungsional sistem" muncul di samping nama
3. Jika ROLE-AKUN mencoba mengakses form edit IDR:
   - Kode mata uang: disabled (seperti biasa)
   - Tidak ada perbedaan visual khusus di form — proteksi utama ada di server
   - Server yang return `SYSTEM_CURRENCY_PROTECTED` jika ada upaya delete

---

## Inline Validation (WCAG 2.1 AA)

**Kapan validasi client-side muncul**:
- Setelah user meninggalkan field (onBlur), bukan saat mengetik
- Saat submit: semua field divalidasi sekaligus

**Layout error inline**:
```
Kode Mata Uang *
┌──────────────────────────────────────────┐
│ RUPIAH                                   │  ← border merah
└──────────────────────────────────────────┘
⚠ Kode mata uang harus 3 huruf kapital sesuai ISO 4217 (contoh: IDR, USD, EUR)
   [ikon segitiga peringatan] [pesan dalam text-sm text-red-600]
```

`aria-describedby` linking: `<input id="kode" aria-describedby="kode-error">`, `<p id="kode-error" role="alert">...pesan...</p>`

**Validation error dari server (HTTP 400)**:
- Field-level: diset via `form.setError(fieldName, {message})`
- Error toast merah persistent: "N field bermasalah — lihat form di bawah"
- Scroll otomatis ke field pertama yang error setelah validation

**Unique check server-side**: ketika submit CREATE dan kode sudah ada → `CONFLICT 409` → error di field kode + toast error.

---

## Submit Flow

```
User klik "Simpan"
    │
    ▼ Validasi client-side (Zod schema)
    │  Gagal → highlight field + toast error → STOP
    │  Lulus → lanjut
    │
    ▼ Button "Simpan" → disable + spinner inline (tidak bisa double-submit)
    │
    ▼ POST /api/v1/master/mata-uang
      Header: Idempotency-Key: {uuidv4 baru setiap submit}
    │
    ├─ 201 Created
    │   → Toast hijau 4 detik: "Mata uang GBP — Pound Sterling berhasil dibuat.
    │     Menunggu review Finance Controller. [Lihat detail →]"
    │   → Redirect ke /master/mata-uang/GBP
    │   → Button kembali aktif (tapi user sudah redirect)
    │
    ├─ 200 (IDEMPOTENCY_REPLAY)
    │   → Treated seperti 201 — tampil toast sukses, redirect ke detail
    │
    ├─ 400 VALIDATION_FAILED
    │   → Highlight field + inline error per field
    │   → Toast merah: "{N} field bermasalah — lihat form di bawah. Trace: {id}"
    │   → Button kembali aktif
    │
    ├─ 409 CONFLICT (kode sudah ada)
    │   → Error di field Kode: "Mata uang GBP sudah terdaftar di sistem."
    │   → Toast merah
    │   → Button kembali aktif
    │
    ├─ 409 CONFLICT (row_version mismatch — saat edit)
    │   → Toast merah: "Mata uang GBP telah diubah oleh user lain. Muat ulang halaman."
    │   → Tawarkan tombol [Muat Ulang] di toast
    │   → Form di-lock (semua field disabled) sampai user muat ulang
    │
    └─ 422 IDEMPOTENCY_MISMATCH
        → Toast merah: "Request sebelumnya sudah berhasil tapi dengan data berbeda. Coba lagi atau hubungi IT."
```

---

## Tombol Footer

```
[Batal]   variant="outline" → kembali ke halaman list (confirm dialog jika form sudah diisi)
[Simpan]  variant="default" → submit form
```

**Confirm dialog saat Batal** (jika ada input yang belum disimpan):
```
Yakin ingin meninggalkan halaman?
Data yang sudah diisi akan hilang.

[Tetap di Sini]    [Keluar Tanpa Menyimpan]
```

---

## Form Edit — Perbedaan Create vs Edit

| Aspek | Create | Edit |
|---|---|---|
| Header | "Tambah Mata Uang" | "Edit Mata Uang — {kode}" |
| Method API | POST /master/mata-uang | PUT /master/mata-uang/{kode} |
| Field kode | Input aktif | Disabled (read-only) |
| rowVersion | Tidak ada | Hidden field, dikirim ke server |
| Idempotency-Key | UUID baru saat submit | UUID baru saat submit |
| Toast sukses | "...berhasil dibuat. Menunggu review..." | "...berhasil diperbarui." |
| Redirect | Ke detail page | Ke detail page (stay) |

---

## Read-Only Mode (APPROVED record atau ROLE-AUDIT)

Saat record APPROVED atau user adalah ROLE-AUDIT:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  INFORMASI DASAR                                                            │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  Kode Mata Uang              Nama Mata Uang                                 │
│  IDR                         Rupiah Indonesia                               │
│                                                                             │
│  Simbol                      Decimal Places                                 │
│  Rp                          0                                              │
│  ...                                                                        │
└─────────────────────────────────────────────────────────────────────────────┘

  [⚠ Informasi ini sudah disetujui. Untuk mengubah atribut, ajukan request
   ke Finance Controller untuk diproses melalui workflow amandemen.]

  (tidak ada tombol Simpan — hanya tombol Kembali)
```

Field tampil sebagai teks statis (tidak ada border input). Label di atas, nilai di bawah dengan font weight normal. Lebih compact dari form edit.

---

## Aksesibilitas

- Semua label terhubung ke input via `htmlFor` / `id`
- Error message via `aria-describedby` + `role="alert"`
- Semua field reachable via Tab key, order: kode → nama → simbol → decimal → sumber kurs → frekuensi → tanggal → aktif → batal → simpan
- Toggle switch: `role="switch"` + `aria-checked`
- Date picker: keyboard navigable, format tooltip instruksi
- Required field: asterisk (*) di label + `required` attribute di input + `aria-required="true"`
- Error state: border merah + ikon peringatan + teks (tidak hanya warna)
