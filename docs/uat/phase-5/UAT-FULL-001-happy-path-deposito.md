# UAT-FULL-001 — Happy Path Deposito Full Cycle

**Modul**: Cross-Modul (APP-B + APP-C + APP-D)
**Story**: Full lifecycle penempatan deposito yang performing hingga periode hard-close
**Tanggal dokumen**: 2026-06-25
**Dibuat oleh**: qa-engineer
**Status**: DRAFT

---

## 1. Metadata

| Field | Nilai |
|---|---|
| ID UAT | UAT-FULL-001 |
| Skenario | Happy path deposito AC Stage 1 → periodik akrual → ECL calc run → GL delivery → soft-close → hard-close |
| Referensi test | `TestP5M18_HappyPath_Deposito_Full_Cycle` |
| Referensi FSD | FSD-APP-B §1 (penempatan), FSD-APP-C §3.1 (Stage 1), FSD-APP-D §2 (periode close) |
| PSAK 71 | §5.5.1 (Stage 1), §5.4.1(a) (interest Gross Carrying) |
| DEC terkait | DEC-010, DEC-013, DEC-017, DEC-018, DEC-021, DEC-027 |

---

## 2. Persona yang Terlibat

| Persona | Role | Aksi |
|---|---|---|
| Treasury Maker | ROLE-MAKER-TR | Buat penempatan deposito |
| Treasury Reviewer | ROLE-APPR-TR | Review penempatan |
| Treasury Approver | ROLE-APPR-TR | Approve penempatan (MFA step-up) |
| Risk Officer | ROLE-RISK | Review dan jalankan ECL calc run |
| Akuntansi | ROLE-AKUN | Verifikasi jurnal akrual dan ECL |
| Finance Controller | ROLE-AKUN-CTL | Soft-close periode |
| CFO | ROLE-CFO | Hard-close periode (MFA step-up) |

---

## 3. Pre-kondisi

| # | Kondisi | Cara verifikasi |
|---|---|---|
| P1 | Master instrumen deposito BCA sudah ada di status APPROVED dengan klasifikasi AC | `GET /api/v1/master/instrumen?filter[klasifikasi_psak71]=AC&filter[jenis]=DEPOSITO` |
| P2 | Periode buku Juni 2026 dalam status OPEN | `GET /api/v1/periode-buku/PBUKU-2026-06` → `status: "OPEN"` |
| P3 | Mapping jurnal event code PENEMPATAN, AKRUAL_BUNGA, ECL_PEMBENTUKAN sudah APPROVED_ACTIVE | `GET /api/v1/master/mapping-jurnal?filter[status]=APPROVED_ACTIVE` |
| P4 | GL Host tersambung dan status HEALTHY | `GET /api/v1/reconciliation/daily/status` → `gl_host: "HEALTHY"` |
| P5 | User maker, reviewer, approver adalah 3 akun berbeda (SoD seed) | Keycloak admin → 3 user aktif |
| P6 | EIR calculator sudah konfigurasi dengan tolerance 1e-10 (DEC-013) | `GET /api/v1/master/parameter?key=eir_tolerance` → `1e-10` |

---

## 4. Data Test

| Field | Nilai |
|---|---|
| Instrumen | INST-DEP-BCA-001 (deposito BCA 12 bulan, AC) |
| Nominal | IDR 10.000.000.000 (10 miliar) |
| Tenor | 12 bulan |
| Kupon gross | 5,25% p.a. |
| PPh | 20% final |
| Tanggal penempatan | 2026-06-14 |
| Tanggal jatuh tempo | 2027-06-14 |
| Settlement account | ACC-1234567890 |
| EIR yang diharapkan (setelah PPh) | ≈ 4,20% p.a. (Newton-Raphson) |
| PD 12M (Stage 1) | 0,50% |
| LGD | 45% |
| FL Normal | 1,10 |
| ECL weighted (estimasi) | IDR 12.375.000 (nominal × 0.5% × 45% × FL-weighted) |

---

## 5. Langkah-Langkah

### Fase 1: Penempatan → Approval

**Step 1.1 — Buat penempatan (Treasury Maker)**

1. Login sebagai `usr-maker-tr-01`.
2. Buka `/transaksi/penempatan/new`.
3. Isi form: pilih instrumen INST-DEP-BCA-001, nominal IDR 10.000.000.000, tenor 12 bulan, kupon 5,25%.
4. Klik **Simpan sebagai Draft**.

Hasil yang diharapkan:
- [ ] Status menjadi `DRAFT`.
- [ ] `kode_transaksi` tergenerate format `PNP-202606-XXXXXX`.
- [ ] Toast: "Penempatan PNP-202606-XXXXXX berhasil dibuat. Menunggu review."
- [ ] Audit log: `PENEMPATAN.CREATED` tercatat.

**Step 1.2 — Submit untuk review**

1. Dari halaman detail penempatan, klik **Submit untuk Review**.
2. Isi komentar.

Hasil yang diharapkan:
- [ ] Status menjadi `PENDING_REVIEW`.
- [ ] Audit log: `PENEMPATAN.SUBMITTED`.

**Step 1.3 — Review (Treasury Reviewer)**

1. Login sebagai `usr-appr-tr-01` (bukan usr-maker-tr-01).
2. Buka notifikasi atau `/transaksi/penempatan?filter[status]=PENDING_REVIEW`.
3. Klik **Review** → masukkan komentar.

Hasil yang diharapkan:
- [ ] Status menjadi `PENDING_APPROVAL`.
- [ ] `reviewer_signature_hash` terisi.
- [ ] Audit log: `PENEMPATAN.REVIEWED`.

**Step 1.4 — Approve (Treasury Approver + MFA step-up)**

1. Login sebagai `usr-appr-tr-02` (bukan maker atau reviewer).
2. Buka halaman penempatan → klik **Approve**.
3. MFA step-up prompt muncul → masukkan OTP.

Hasil yang diharapkan:
- [ ] Status menjadi `APPROVED_ACTIVE`.
- [ ] `staging_action: "STAGE_1_ASSIGNED"` (AC → Stage 1).
- [ ] Asynq job EIR_COMPUTE terkuantum. `/api/v1/jobs/{jobId}` → status running.
- [ ] Jurnal PENEMPATAN ter-post: D Investasi Deposito / K Kas. Jurnal balanced.
- [ ] GL status jurnal: `DELIVERED` (setelah ≤ 30s).
- [ ] Audit log: `PENEMPATAN.APPROVED`, `PENEMPATAN.STAGING_INITIAL`.

### Fase 2: Akrual Harian (EIR method)

**Step 2.1 — Akrual bunga harian (otomatis oleh cron)**

1. Tunggu Asynq cron akrual harian (default 23:59 WIB) atau trigger manual via `/api/v1/transaksi/akrual/run`.
2. Verifikasi akrual harian.

Hasil yang diharapkan:
- [ ] Akrual bunga = `gross_carrying × EIR / 365` (EIR method, bukan simple).
- [ ] Stage 1 → basis Gross Carrying Amount.
- [ ] Jurnal AKRUAL_BUNGA: D Piutang Bunga / K Pendapatan Bunga. Balanced.
- [ ] `GL status: DELIVERED`.
- [ ] Audit log: `ECL.AKRUAL_BUNGA` dengan `basis: "GROSS_CARRYING"`.

### Fase 3: ECL Calc Run (Stage 1)

**Step 3.1 — Jalankan ECL calc run (Risk Officer)**

1. Login sebagai `usr-risk-01`.
2. Buka `/ecl/calc-run/new`, pilih periode PBUKU-2026-06, klik **Jalankan**.
3. `<JobProgressPanel>` muncul, tampilkan progress.

Hasil yang diharapkan:
- [ ] Job submit → `202 { jobId, statusUrl }`.
- [ ] Progress naik 0% → 100%.
- [ ] Stage instrumen INST-DEP-BCA-001 tetap `STAGE_1` (tidak ada SICR trigger).
- [ ] ECL_weighted untuk instrumen ini = **IDR ≤ 100 juta** (low PD deposito).
- [ ] ECL weighted formula: `EAD × PD_Normal × LGD × FL_Normal × W_Normal + ...`.
- [ ] Jurnal ECL_PEMBENTUKAN ter-post: D Beban CKPN / K Cadangan CKPN. Balanced.
- [ ] Audit: `ECL.CALC_RUN`.

### Fase 4: GL Delivery Verification

**Step 4.1 — Verifikasi rekonsiliasi harian**

1. Buka `/reconciliation/daily`.
2. Filter tanggal hari ini.

Hasil yang diharapkan:
- [ ] Semua jurnal header status `DELIVERED`.
- [ ] Tidak ada item di DLQ (`/jurnal/dlq` kosong untuk tanggal ini).
- [ ] Jumlah jurnal PENEMPATAN + AKRUAL_BUNGA + ECL_PEMBENTUKAN = 3 header.

### Fase 5: Periode Close

**Step 5.1 — Soft-close (Finance Controller)**

1. Login sebagai `usr-fincon-01` (ROLE-AKUN-CTL, MFA aktif).
2. Buka `/periode-buku/PBUKU-2026-06` → klik **Request Soft-Close**.
3. Reviewer lain approve soft-close.

Hasil yang diharapkan:
- [ ] Pre-condition checklist terpenuhi: 0 jurnal PENDING, GL recon pass.
- [ ] Status menjadi `SOFT_CLOSED`.
- [ ] Audit log: `PERIODE.SOFT_CLOSED`.

**Step 5.2 — Hard-close (CFO + MFA step-up)**

1. Login sebagai `usr-cfo-01` (ROLE-CFO, MFA wajib).
2. Buka `/periode-buku/PBUKU-2026-06` → klik **Hard-Close**.
3. Dialog konfirmasi → "Hard-close (CFO MFA required)".
4. MFA step-up prompt → masukkan OTP (max 5 menit sebelum aksi).

Hasil yang diharapkan:
- [ ] Status menjadi `HARD_CLOSED`.
- [ ] Timestamp `hard_closed_at` terisi.
- [ ] `CFO_signature_hash` terisi.
- [ ] Audit log: `PERIODE.HARD_CLOSED` dengan `step_up_mfa: true`.
- [ ] Toast: "Periode Juni 2026 berhasil hard-closed."

**Step 5.3 — Verifikasi mutasi diblokir setelah hard-close**

1. Coba POST penempatan baru untuk periode PBUKU-2026-06.

Hasil yang diharapkan:
- [ ] Response: `423 { code: "PERIODE_CLOSED", message: "..." }`.
- [ ] Tidak ada data baru masuk ke DB.

---

## 6. Audit Checks

| Aksi | Audit Action yang Diharapkan |
|---|---|
| Buat penempatan | `PENEMPATAN.CREATED` |
| Submit | `PENEMPATAN.SUBMITTED` |
| Review | `PENEMPATAN.REVIEWED` |
| Approve | `PENEMPATAN.APPROVED` + `PENEMPATAN.STAGING_INITIAL` |
| Akrual harian | `ECL.AKRUAL_BUNGA` |
| ECL calc run | `ECL.CALC_RUN` |
| Jurnal post | `JURNAL.POSTED` (3×) |
| GL delivered | `GL.DELIVERED` (3×) |
| Soft-close | `PERIODE.SOFT_CLOSED` |
| Hard-close | `PERIODE.HARD_CLOSED` |

Hash chain: `GET /api/v1/audit/verify?from=2026-06-14&to=2026-06-30` → `{ valid: true }`.

---

## 7. Rollback / Cleanup

1. Hapus penempatan dengan soft-delete (`deleted_at`): `DELETE /api/v1/transaksi/penempatan/{id}`.
2. Reopen periode (jika perlu): hanya mungkin dari SOFT_CLOSED, bukan HARD_CLOSED.
3. Jurnal yang sudah di-deliver ke GL Host tidak dapat di-rollback — koordinasikan dengan GL Host operator.
4. ECL calc run result: soft-delete di `ecl.ecl_calc_run` (bukan hard delete).

---

## 8. Pass / Fail

- [ ] PASS — Semua expected results terpenuhi, audit hash chain valid.
- [ ] FAIL — Satu atau lebih expected results tidak terpenuhi.

---

## 9. Sign-Off

| Peran | Nama | Tanggal | Hasil | Tanda tangan |
|---|---|---|---|---|
| QA Engineer | | | PASS / FAIL | |
| Treasury Maker (UAT Actor) | | | PASS / FAIL | |
| Finance Controller (UAT Actor) | | | PASS / FAIL | |
| CFO | | | PASS / FAIL | |
| Compliance Officer | | | APPROVED / REJECT | |
| Internal Auditor | | | VERIFIED | |
