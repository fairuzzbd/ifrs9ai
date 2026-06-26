# UAT-FULL-008 — Periode Buku Full Cycle: OPEN → Soft-Close → Reopen → Hard-Close

**Modul**: APP-D (Periode Buku)
**Story**: Full lifecycle periode buku termasuk satu siklus reopen, hard-close dengan CFO MFA step-up, dan verifikasi blokir mutasi setelah hard-close
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-008 |
| Referensi test | `TestP5M18_Periode_Close_Then_Reopen_Hard_Close` |
| Referensi FSD | FSD-APP-D §1 (Periode Buku) |
| DEC terkait | DEC-027 (step-up MFA hard-close) |
| State machine | OPEN → SOFT_CLOSED → OPEN → SOFT_CLOSED → HARD_CLOSED (terminal) |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Finance Controller | ROLE-AKUN-CTL | Soft-close request + approve |
| CEO | ROLE-CEO | Reopen periode |
| CFO | ROLE-CFO | Hard-close + MFA step-up |
| Treasury Maker | ROLE-MAKER-TR | Coba post transaksi setelah hard-close (gagal) |
| Internal Auditor | ROLE-AUDIT | Verifikasi immutability |

---

## 3. Pre-kondisi

| # | Kondisi |
|---|---|
| P1 | Periode PBUKU-2026-05 (Mei 2026) status OPEN |
| P2 | Semua jurnal bulan Mei sudah ter-post dan GL status DELIVERED |
| P3 | ECL calc run bulan Mei sudah sealed |
| P4 | Pre-condition checklist: 0 transaksi PENDING_APPROVAL, 0 DLQ item aktif, GL recon pass |
| P5 | CFO (`usr-cfo-01`) memiliki MFA aktif dan step-up konfigurasi |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Periode | PBUKU-2026-05 (Mei 2026) |
| Alasan reopen | "Koreksi akrual PPh bulan Mei — temuan audit internal" |
| Alasan hard-close | "Semua koreksi selesai, periode final" |

---

## 5. Langkah-Langkah

### Fase 1: OPEN → SOFT_CLOSED (Finance Controller)

**Step 1.1 — Request soft-close**

1. Login sebagai `usr-fincon-01` (ROLE-AKUN-CTL, MFA aktif).
2. Buka `/periode-buku/PBUKU-2026-05`.
3. Klik **Request Soft-Close**.
4. Sistem menjalankan pre-condition checklist.

Hasil yang diharapkan:
- [ ] Checklist: 0 transaksi PENDING_APPROVAL ✓
- [ ] Checklist: semua jurnal GL DELIVERED ✓
- [ ] Checklist: ECL run sealed ✓
- [ ] Checklist: DLQ kosong ✓
- [ ] Status menjadi `PENDING_SOFT_CLOSE`.
- [ ] Notifikasi ke reviewer soft-close.

**Step 1.2 — Approve soft-close**

1. Reviewer lain (ROLE-AKUN-CTL) approve soft-close.

Hasil yang diharapkan:
- [ ] Status menjadi `SOFT_CLOSED`.
- [ ] Audit: `PERIODE.SOFT_CLOSED`.
- [ ] Toast: "Periode Mei 2026 berhasil soft-closed."
- [ ] Materialized view refresh terpicu (Asynq job).

**Step 1.3 — Verifikasi write terbatas di SOFT_CLOSED**

1. Coba buat penempatan baru di periode Mei 2026.

Hasil yang diharapkan:
- [ ] Response: `423 { code: "PERIODE_CLOSED", message: "..." }`.
- [ ] ECL result: masih bisa dibaca (read-only OK).

### Fase 2: SOFT_CLOSED → OPEN (Reopen — CEO)

**Step 2.1 — Request reopen**

1. Login sebagai `usr-ceo-01` (ROLE-CEO).
2. Klik **Reopen Periode** → isi alasan (min 30 karakter).

Hasil yang diharapkan:
- [ ] Dialog konfirmasi muncul: "Periode akan dibuka kembali untuk koreksi. Lanjut?"
- [ ] Status kembali menjadi `OPEN`.
- [ ] Audit: `PERIODE.REOPENED` dengan alasan tercatat.
- [ ] `soft_closed_at` di-reset ke NULL.

**Step 2.2 — Buat jurnal koreksi**

1. Login sebagai `usr-akun-01` (ROLE-AKUN).
2. Buat jurnal koreksi PPh.

Hasil yang diharapkan:
- [ ] Jurnal koreksi berhasil ter-post (periode sudah OPEN kembali).
- [ ] GL deliver OK.

### Fase 3: OPEN → SOFT_CLOSED (Kedua Kali)

**Step 3.1 — Soft-close lagi**

Ulangi Step 1.1 — 1.2.

Hasil yang diharapkan:
- [ ] Alur sama, status kembali `SOFT_CLOSED`.
- [ ] Audit total: 2× `PERIODE.SOFT_CLOSED`, 1× `PERIODE.REOPENED`.

### Fase 4: SOFT_CLOSED → HARD_CLOSED (CFO + MFA Step-Up)

**Step 4.1 — Stale MFA → ditolak**

1. Login sebagai `usr-cfo-01`, token step-up sudah > 5 menit lalu.
2. Klik **Hard-Close**.

Hasil yang diharapkan:
- [ ] Response: `403 { code: "STEP_UP_MFA_REQUIRED" }`.
- [ ] Dialog MFA step-up muncul.

**Step 4.2 — MFA step-up (< 5 menit) → diterima**

1. `usr-cfo-01` lakukan OTP → step-up token fresh (< 5 menit).
2. Klik **Hard-Close** lagi.
3. Dialog konfirmasi final: "Setelah hard-close, periode tidak bisa di-reopen. Lanjut?"
4. Konfirmasi.

Hasil yang diharapkan:
- [ ] Status menjadi `HARD_CLOSED`.
- [ ] `hard_closed_at` terisi.
- [ ] `cfo_signature_hash` terisi.
- [ ] Audit: `PERIODE.HARD_CLOSED` dengan `step_up_mfa: true`.
- [ ] Toast: "Periode Mei 2026 berhasil hard-closed. Tidak dapat dibuka kembali."
- [ ] Materialized view final refresh terpicu.

### Fase 5: Verifikasi Immutability HARD_CLOSED

**Step 5.1 — Coba post jurnal setelah hard-close**

1. Coba `POST /api/v1/jurnal/header` dengan `periode_id = PBUKU-2026-05`.

Hasil yang diharapkan:
- [ ] `423 { code: "PERIODE_CLOSED" }`.

**Step 5.2 — Coba reopen setelah hard-close**

1. Klik **Reopen** (jika tombol tersedia, harus disabled).

Hasil yang diharapkan:
- [ ] Tombol Reopen disabled / tidak tampil.
- [ ] API `POST /api/v1/periode-buku/PBUKU-2026-05/reopen` → `422 { code: "PERIODE_REOPEN_FORBIDDEN" }`.

**Step 5.3 — FX rate masih bisa dibaca**

1. `GET /api/v1/master/kurs?periode=PBUKU-2026-05`.

Hasil yang diharapkan:
- [ ] Data FX rate terbaca (read-only OK).
- [ ] `locked_flag = true` pada semua kurs periode Mei 2026.

---

## 6. Audit Checks

| Aksi | Audit Action |
|---|---|
| Soft-close #1 | `PERIODE.SOFT_CLOSED` |
| Reopen | `PERIODE.REOPENED` |
| Soft-close #2 | `PERIODE.SOFT_CLOSED` |
| Hard-close | `PERIODE.HARD_CLOSED` (step_up_mfa=true) |

Hash chain harus valid untuk keseluruhan audit trail periode ini.

---

## 7. Rollback / Cleanup

HARD_CLOSED tidak dapat di-rollback. Ini adalah desain yang disengaja.
Jika ada kesalahan setelah hard-close: buat adjustment di periode berikutnya.

---

## 8. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Finance Controller (UAT Actor) | | | PASS / FAIL | |
| CFO | | | PASS / FAIL | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
