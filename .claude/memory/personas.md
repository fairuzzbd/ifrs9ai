# Personas / RBAC Roles — BLIPS

## 10 Base roles

### ROLE-MAKER-TR — Treasury Maker
- **Tugas**: Input transaksi penempatan, renewal, jatuh tempo, jual deposito/obligasi/saham. Upload dokumen pendukung.
- **MFA**: tidak wajib (kecuali Treasury Manager).
- **Cannot**: review atau approve transaksi sendiri (SoD).
- **Typical screens**: Form Penempatan, Form MTM Adjustment, Upload Dokumen.

### ROLE-APPR-TR — Treasury Approver / Reviewer
- **Tugas**: Review transaksi yang di-submit Maker. Approve atau reject dengan komentar.
- **MFA**: wajib jika senior (Treasury Manager).
- **Cannot**: review transaksi yang dia create sendiri.
- **Typical screens**: Queue Pending Review, Detail Transaksi (read + sign panel).

### ROLE-RISK — Risk Officer
- **Tugas**: Review klasifikasi PSAK 71 (SPPI + BM), parameter ECL preview. Sign-off pertama untuk klasifikasi 6-eyes (sebelum ROLE-KOMITE).
- **MFA**: tidak wajib (tapi best practice).
- **Typical screens**: SPPI Test Review, BM Assessment Review, Calc Run Review.

### ROLE-AKUN — Akuntansi
- **Tugas**: Input mapping jurnal, validate jurnal posting result, upload FX rate manual jika feed gagal, upload NAB Reksadana / closing price.
- **MFA**: tidak wajib.
- **Typical screens**: Mapping Jurnal, FX Manual Entry, Upload Feeds.

### ROLE-AKUN-CTL — Finance Controller
- **Tugas**: Approve jurnal posting ke GL, approve mapping jurnal baru, kontrol periode buku (soft close).
- **MFA**: WAJIB.
- **Typical screens**: Jurnal Queue Approver, Periode Buku Soft Close.

### ROLE-CFO
- **Tugas**: Hard close periode buku (final). Tanda tangan terakhir laporan ECL summary.
- **MFA**: WAJIB + step-up untuk hard close.
- **Typical screens**: Periode Buku Hard Close, Executive Dashboard.

### ROLE-AUDIT — Internal Auditor
- **Tugas**: Read-only access ke seluruh schema. Verifikasi hash-chain audit log. Export untuk auditor eksternal.
- **MFA**: tidak wajib (read-only).
- **Cannot**: ANY mutation. ANY screen yang non-readonly disable.
- **Typical screens**: Audit Log Browser, Hash Chain Verifier, Report Export.

### ROLE-IT-ADMIN
- **Tugas**: User management (di Keycloak), role assignment, integrasi config, DLQ inspection, runbook execution.
- **MFA**: WAJIB.
- **Cannot**: create transaksi atau approve domain workflow (SoD — IT bukan domain user).
- **Typical screens**: User Admin, Integration Status, DLQ Browser.

### ROLE-KOMITE — Komite Investasi
- **Tugas**: Approve klasifikasi PSAK 71 (FVOCI Election, override). 6-eyes step terakhir.
- **MFA**: WAJIB.
- **Typical screens**: Klasifikasi Pending Approval, FVOCI Election Decision.

### ROLE-ALCO — Asset-Liability Committee
- **Tugas**: Approve parameter ECL (PD curve, LGD pool, scenario weights, FL multiplier). Setiap parameter aktivasi butuh ALCO.
- **MFA**: WAJIB.
- **Typical screens**: ECL Parameter Approve, Scenario Weight Override.

## Sample permission matrix

| Action | Permission | Role | MFA | Notes |
|---|---|---|---|---|
| Create instrumen | `instrumen.create` | MAKER-TR | no | |
| Review instrumen | `instrumen.review` | APPR-TR | no | maker ≠ reviewer |
| Approve klasifikasi | `klasifikasi.approve` | RISK + KOMITE | yes | 6-eyes |
| Run ECL calc | `ecl_run.create` | RISK | no | uses ALCO-approved params |
| Seal ECL run | `ecl_run.seal` | RISK + ALCO | yes (ALCO) | immutable thereafter |
| Approve ECL param | `ecl_parameter.approve` | ALCO | yes (step-up) | |
| Soft close periode | `periode.softclose` | AKUN-CTL | yes | |
| Hard close periode | `periode.hardclose` | CFO | yes (step-up) | irreversible |
| Post jurnal ke GL | `jurnal.post` | AKUN-CTL | yes | |
| Read audit_log | `audit_log.read` | AUDIT | no | |

## Persona writing tips (for business-analyst)
- Selalu sebut role spesifik di user story Actor field.
- Jika story menyentuh multiple personas (mis. flow Maker → Reviewer → Approver), tulis 3 sub-story atau gunakan 1 story dengan multiple AC blocks per persona.
- Test data seed harus include semua 10 persona dengan user terpisah agar SoD bisa di-test.
