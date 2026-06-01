*\[ LOGO TUGURE \]*

**FUNCTIONAL SPECIFICATION DOCUMENT**

**BLIPS IFRS 9 — APPENDIX A**

*Master Data Management • SPPI Test • Business Model Test*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**        | **Keterangan**                                                                                             |
| ------------------ | ---------------------------------------------------------------------------------------------------------- |
| Judul Dokumen      | FSD Appendix A — Master Data + SPPI Test + BM Test                                                         |
| Kode Dokumen       | FSD-APP-A-2026-001                                                                                         |
| Versi              | 1.1                                                                                                        |
| Status             | DRAFT FOR REVIEW                                                                                           |
| Tanggal Terbit     | 02 Mei 2026                                                                                                |
| Bahasa             | Bahasa Indonesia                                                                                           |
| Reference Upstream | FSD Master v1.0; BRD v1.1; SoW v1.2                                                                        |
| Modul Tercakup     | Master Data (10 sub-modul) + SPPI Test + Business Model Test + Klasifikasi Engine + Reklasifikasi Workflow |
| BR-IDs Tercakup    | BR-MAS-001 to 015 (Master Data); BR-SPP-001 to 012 (SPPI/BM)                                               |
| Penyusun           | Solution Designer + Lead BA                                                                                |
| Approver           | Direktur IT + Direktur Investasi                                                                           |

# Outline Appendix A

Appendix A mencakup foundational data layer dan klasifikasi engine — kedua hal yang menjadi precondition untuk seluruh transaksi & perhitungan compliance.

| **Bab** | **Topik**                    | **Modul Tercakup**                                                                                 |
| ------- | ---------------------------- | -------------------------------------------------------------------------------------------------- |
| 1       | Master Data Management       | Instrumen, Counterparty, Rating PD, LGD Basel, Periode, Mata Uang, CoA, Mapping Jurnal, Portofolio |
| 2       | SPPI Test Engine             | 10-question checklist + audit trail                                                                |
| 3       | Business Model Test Engine   | Indicator-based assessment + auto-suggest                                                          |
| 4       | Klasifikasi PSAK 71 Engine   | Matriks SPPI × BM → AC/FVOCI/FVTPL/FVOCI Election                                                  |
| 5       | Reklasifikasi Prospektif     | 6 kombinasi from-to + jurnal transisi otomatis                                                     |
| 6       | Pre-Trade Clearance Workflow | End-to-end flow Treasury Maker → Komite Approver                                                   |

# 1\. Master Data Management

## 1.1 Master Instrumen Investasi

### 1.1.1 Module Overview

Master Instrumen menyimpan registry lengkap aset keuangan yang dimiliki Tugure. Setiap instrumen ter-link dengan klasifikasi PSAK 71 (ter-lock), counterparty, manajer investasi (untuk reksadana), portofolio, dan parameter EIR (untuk AC/FVOCI utang).

### 1.1.2 Screen Flow & UI Mockup (Textual)

**List Page — /master/instrumen**

> ┌─────────────────────────────────────────────────────────────────────┐  
> │ \[Sidebar: Master Data\] \[TopBar: Tugure | Search | 🔔 | Profile\] │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ Master Instrumen Investasi \[+ New\] \[↓\] │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ Filter: \[Tipe ▼\] \[Klasifikasi ▼\] \[Status ▼\] \[Counterparty ▼\] \[🔍\] │  
> ├─────┬───────────────┬───────────┬─────────┬─────────┬────────────────┤  
> │ ☐ │ Kode │ Tipe │ Klas │ Status │ Aksi │  
> ├─────┼───────────────┼───────────┼─────────┼─────────┼────────────────┤  
> │ ☐ │ OBL-2026-001 │ OBLIGASI │ FVOCI │ AKTIF │ 👁 ✏ 🔒 │  
> │ ☐ │ DEP-2026-001 │ DEPOSITO │ AC │ AKTIF │ 👁 ✏ 🔒 │  
> │ ☐ │ RDN-2026-1 │ REKSADANA │ FVTPL │ AKTIF │ 👁 ✏ 🔒 │  
> └─────┴───────────────┴───────────┴─────────┴─────────┴────────────────┘  
> \[Pagination: 1 2 3 ... \> \] \[Bulk: Export ▼\]

**Detail Page — /master/instrumen/{id}**

> ┌─────────────────────────────────────────────────────────────────────┐  
> │ ← Back Detail: OBL-2026-00001 — Obligasi PT XYZ Tbk Seri A │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ Status: \[AKTIF\] Klasifikasi: \[FVOCI 🔒\] Versi: 1 │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ \[Tab: Detail | SPPI/BM | Amortization | History | Documents\] │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ ▼ Identitas │  
> │ Kode Instrumen: OBL-2026-00001 (auto) │  
> │ Tipe: OBLIGASI / Sub-Tipe: KORPORASI │  
> │ Nama: Obligasi PT XYZ Tbk Seri A 2026 │  
> │ ISIN: ID1000123456 │  
> │ Counterparty: PT XYZ Tbk (Rating: idA-) │  
> │ ▼ Term & Pricing │  
> │ Nominal: Rp 5.000.000.000 • Mata Uang: IDR │  
> │ Kupon: 5,0000% pa • Frekuensi: SEMESTERAN │  
> │ Tgl Penempatan: 15/01/2026 • Tgl JT: 31/12/2030 │  
> │ ▼ EIR & Amortisasi (FVOCI Utang) │  
> │ EIR Awal: 0,04826688 (4,8267%) • EIR Method: Y │  
> │ Premium/Diskonto Awal: Rp 80.000.000 (premium) │  
> │ Biaya Transaksi: Rp 5.000.000 • Day Count: ACT/365 │  
> │ ▼ Audit Trail │  
> │ Created by treasury.maker @ 15/01/2026 08:23 │  
> │ Approved by treasury.manager @ 15/01/2026 09:45 │  
> │ Klasifikasi locked by komite.investasi @ 15/01/2026 14:30 │  
> │ │  
> │ \[✏ Edit (versi baru)\] \[🔒 Locked Section\] \[📎 Documents (3)\] │  
> └─────────────────────────────────────────────────────────────────────┘

### 1.1.3 Detail Field Specifications

| **Field**                   | **DB Column**                 | **Type**      | **Nullable** | **Default**                 | **Validation Rule**                                       |
| --------------------------- | ----------------------------- | ------------- | ------------ | --------------------------- | --------------------------------------------------------- |
| id                          | id                            | UUID          | NO           | uuidv7()                    | Auto-generated                                            |
| Kode Instrumen              | kode\_instrumen               | VARCHAR(20)   | NO           | Auto-generated via sequence | Format: {prefix}-{year}-{\#\#\#\#}; unique                |
| Tipe Instrumen              | tipe\_instrumen               | VARCHAR(30)   | NO           | —                           | Enum: lookup TIPE\_INSTRUMEN                              |
| Sub-Tipe                    | sub\_tipe                     | VARCHAR(50)   | NO           | —                           | Conditional based on tipe (lihat tabel sub-tipe)          |
| Nama                        | nama                          | VARCHAR(200)  | NO           | —                           | Min 5 karakter                                            |
| ISIN/Kode Efek              | isin                          | VARCHAR(20)   | YES          | NULL                        | Format: 12 alphanumeric (untuk obligasi/saham/RDN)        |
| Counterparty ID             | counterparty\_id              | UUID FK       | NO           | —                           | Must exist in mst\_counterparty                           |
| Manajer Investasi ID        | manajer\_investasi\_id        | UUID FK       | Conditional  | NULL                        | Required jika tipe=REKSADANA                              |
| Bank Kustodian ID           | bank\_kustodian\_id           | UUID FK       | Conditional  | NULL                        | Required jika tipe=REKSADANA                              |
| Mata Uang                   | mata\_uang                    | CHAR(3)       | NO           | IDR                         | ISO 4217; lookup MATA\_UANG                               |
| Nominal/Face Value          | nominal                       | NUMERIC(20,2) | NO           | —                           | Must \> 0                                                 |
| Jumlah Lot/Lembar           | jumlah\_lot                   | NUMERIC(18,0) | Conditional  | NULL                        | Required jika tipe=SAHAM (1 lot=100 lembar)               |
| Tanggal Penempatan          | tanggal\_penempatan           | DATE          | NO           | —                           | ≤ Tanggal hari ini                                        |
| Tanggal Jatuh Tempo         | tanggal\_jatuh\_tempo         | DATE          | Conditional  | NULL                        | Required untuk DEPOSITO/OBLIGASI; \> tanggal\_penempatan  |
| Suku Bunga/Kupon            | kupon                         | NUMERIC(8,4)  | Conditional  | NULL                        | Required untuk DEPOSITO/OBLIGASI; ≥ 0                     |
| Frekuensi Bunga             | frekuensi\_bunga              | VARCHAR(20)   | Conditional  | NULL                        | Required untuk DEPOSITO/OBLIGASI; lookup FREKUENSI\_BUNGA |
| Auto Renewal Flag           | auto\_renewal\_flag           | BOOLEAN       | Conditional  | FALSE                       | Hanya untuk DEPOSITO                                      |
| FVOCI Election (Equity)     | fvoci\_election               | BOOLEAN       | Conditional  | FALSE                       | Hanya untuk SAHAM; locked saat approved                   |
| Klasifikasi PSAK 71         | klasifikasi\_psak71           | VARCHAR(20)   | NO           | —                           | Auto-derived dari SPPI×BM; locked                         |
| SPPI Test Result            | sppi\_result                  | VARCHAR(10)   | NO           | —                           | PASS/FAIL                                                 |
| BM Test Category            | bm\_category                  | VARCHAR(10)   | Conditional  | NULL                        | HTC/HTCS/OTHER; null jika SPPI=FAIL & equity              |
| SPPI/BM Last Review         | sppi\_bm\_last\_review\_date  | DATE          | NO           | tanggal\_penempatan         | Wajib refresh ≤ 1 tahun                                   |
| EIR Awal                    | eir\_awal                     | NUMERIC(12,8) | Conditional  | NULL                        | Required untuk AC & FVOCI utang; 0 ≤ x \< 1               |
| Tanggal EIR Computed        | tanggal\_eir\_computed        | DATE          | Conditional  | tanggal\_penempatan         | Required jika eir\_awal present                           |
| Premium/Diskonto Awal       | premium\_diskonto\_awal       | NUMERIC(20,2) | Conditional  | 0                           | Computed: harga\_beli - nominal                           |
| Biaya Transaksi Capitalized | biaya\_transaksi\_capitalized | NUMERIC(20,2) | Conditional  | 0                           | ≥ 0; capitalized untuk AC/FVOCI                           |
| EIR Method Flag             | eir\_method\_flag             | BOOLEAN       | Conditional  | TRUE                        | Y untuk AC/FVOCI utang; N untuk Cash giro variabel rate   |
| Day Count Convention        | day\_count\_convention        | VARCHAR(10)   | Conditional  | ACT/365                     | ACT/365, 30/360                                           |
| Amortization Frequency      | amortization\_frequency       | VARCHAR(20)   | Conditional  | Same as frekuensi\_bunga    | Lookup FREKUENSI\_BUNGA                                   |
| Status Instrumen            | status                        | VARCHAR(30)   | NO           | AKTIF                       | Lookup INSTRUMEN\_STATUS                                  |
| Portofolio ID               | portofolio\_id                | UUID FK       | NO           | —                           | Must exist in mst\_portofolio                             |

### 1.1.4 Business Rules

1.  Kode Instrumen di-auto-generate berdasarkan tipe + tahun + sequence (mis. OBL-2026-00001).

2.  Klasifikasi PSAK 71 di-derive otomatis dari kombinasi SPPI Result × BM Category × FVOCI Election (lihat Bab 4 — Klasifikasi Engine).

3.  Setelah klasifikasi locked, perubahan hanya melalui Reklasifikasi (Bab 5).

4.  EIR computation triggered saat klasifikasi locked + transaksi penempatan submitted (untuk AC & FVOCI utang).

5.  Sub-Tipe validation: dependent on Tipe Instrumen — sistem block invalid combination.

6.  Untuk reksadana: Manajer Investasi & Bank Kustodian wajib terdaftar sebagai counterparty dengan tipe MANAJER\_INVESTASI / BANK\_KUSTODIAN.

7.  Soft delete only; instrumen aktif tidak dapat di-delete; status berubah lewat workflow (DICAIRKAN, JATUH\_TEMPO, DIJUAL, REKLASIFIKASI).

### 1.1.5 API Endpoints

| **Method** | **Endpoint**                                        | **Permission**      | **Deskripsi**                                   |
| ---------- | --------------------------------------------------- | ------------------- | ----------------------------------------------- |
| GET        | /api/v1/master/instrumen                            | instrumen.read      | List dengan pagination, filter, sort            |
| GET        | /api/v1/master/instrumen/{id}                       | instrumen.read      | Get detail single instrumen                     |
| POST       | /api/v1/master/instrumen                            | instrumen.create    | Create new (status DRAFT, klasifikasi NULL)     |
| PUT        | /api/v1/master/instrumen/{id}                       | instrumen.update    | Update — only allowed before klasifikasi locked |
| POST       | /api/v1/master/instrumen/{id}/lock-klasifikasi      | klasifikasi.approve | Lock klasifikasi setelah Komite approve         |
| DELETE     | /api/v1/master/instrumen/{id}                       | instrumen.delete    | Soft delete (only if no related transactions)   |
| GET        | /api/v1/master/instrumen/{id}/sppi-test             | sppi.read           | Get latest SPPI test result                     |
| GET        | /api/v1/master/instrumen/{id}/bm-test               | bm.read             | Get latest BM test result                       |
| GET        | /api/v1/master/instrumen/{id}/amortization-schedule | amortization.read   | Get schedule (untuk AC/FVOCI utang)             |
| GET        | /api/v1/master/instrumen/{id}/history               | instrumen.read      | Audit trail untuk instrumen ini                 |

### 1.1.6 Database Schema Preview

> CREATE TABLE mst.instrumen (  
> id UUID PRIMARY KEY DEFAULT uuidv7(),  
> kode\_instrumen VARCHAR(20) NOT NULL UNIQUE,  
> tipe\_instrumen VARCHAR(30) NOT NULL,  
> sub\_tipe VARCHAR(50) NOT NULL,  
> nama VARCHAR(200) NOT NULL,  
> isin VARCHAR(20),  
> counterparty\_id UUID NOT NULL REFERENCES mst.counterparty(id),  
> manajer\_investasi\_id UUID REFERENCES mst.counterparty(id),  
> bank\_kustodian\_id UUID REFERENCES mst.counterparty(id),  
> mata\_uang CHAR(3) NOT NULL DEFAULT 'IDR',  
> nominal NUMERIC(20,2) NOT NULL CHECK (nominal \> 0),  
> jumlah\_lot NUMERIC(18,0),  
> tanggal\_penempatan DATE NOT NULL,  
> tanggal\_jatuh\_tempo DATE,  
> kupon NUMERIC(8,4),  
> frekuensi\_bunga VARCHAR(20),  
> auto\_renewal\_flag BOOLEAN DEFAULT FALSE,  
> fvoci\_election BOOLEAN DEFAULT FALSE,  
> klasifikasi\_psak71 VARCHAR(20),  
> klasifikasi\_locked\_at TIMESTAMPTZ,  
> klasifikasi\_locked\_by UUID REFERENCES sec.user(id),  
> sppi\_result VARCHAR(10),  
> bm\_category VARCHAR(10),  
> sppi\_bm\_last\_review\_date DATE NOT NULL,  
> eir\_awal NUMERIC(12,8),  
> tanggal\_eir\_computed DATE,  
> premium\_diskonto\_awal NUMERIC(20,2) DEFAULT 0,  
> biaya\_transaksi\_capitalized NUMERIC(20,2) DEFAULT 0,  
> eir\_method\_flag BOOLEAN DEFAULT TRUE,  
> day\_count\_convention VARCHAR(10) DEFAULT 'ACT/365',  
> amortization\_frequency VARCHAR(20),  
> status VARCHAR(30) NOT NULL DEFAULT 'AKTIF',  
> portofolio\_id UUID NOT NULL REFERENCES mst.portofolio(id),  
> \-- Audit fields  
> created\_by UUID NOT NULL REFERENCES sec.user(id),  
> created\_at TIMESTAMPTZ NOT NULL DEFAULT now(),  
> updated\_by UUID REFERENCES sec.user(id),  
> updated\_at TIMESTAMPTZ,  
> approved\_by UUID REFERENCES sec.user(id),  
> approved\_at TIMESTAMPTZ,  
> workflow\_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT',  
> version INT NOT NULL DEFAULT 1,  
> is\_deleted BOOLEAN NOT NULL DEFAULT FALSE,  
> \-- Constraints  
> CONSTRAINT ck\_jt\_after\_penempatan CHECK (  
> tanggal\_jatuh\_tempo IS NULL OR tanggal\_jatuh\_tempo \> tanggal\_penempatan  
> ),  
> CONSTRAINT ck\_kupon\_nonneg CHECK (kupon IS NULL OR kupon \>= 0),  
> CONSTRAINT ck\_eir\_range CHECK (eir\_awal IS NULL OR (eir\_awal \>= 0 AND eir\_awal \< 1))  
> );  
>   
> CREATE INDEX ix\_instrumen\_tipe ON mst.instrumen(tipe\_instrumen);  
> CREATE INDEX ix\_instrumen\_klasifikasi ON mst.instrumen(klasifikasi\_psak71);  
> CREATE INDEX ix\_instrumen\_counterparty ON mst.instrumen(counterparty\_id);  
> CREATE INDEX ix\_instrumen\_status ON mst.instrumen(status) WHERE is\_deleted = FALSE;  
> CREATE INDEX ix\_instrumen\_isin ON mst.instrumen(isin) WHERE isin IS NOT NULL;

### 1.1.7 Error Codes Specific

| **Code**     | **Message**                                                    | **HTTP** | **Resolution**                  |
| ------------ | -------------------------------------------------------------- | -------- | ------------------------------- |
| ERR-VAL-1001 | Tanggal jatuh tempo harus setelah tanggal penempatan           | 400      | Update tanggal                  |
| ERR-VAL-1002 | Sub-Tipe tidak valid untuk Tipe Instrumen yang dipilih         | 400      | Pilih sub-tipe yang sesuai      |
| ERR-VAL-1003 | Manajer Investasi wajib diisi untuk Reksadana                  | 400      | Pilih MI                        |
| ERR-VAL-1004 | ISIN format tidak valid (12 alphanumeric)                      | 400      | Verifikasi ISIN                 |
| ERR-BIZ-1010 | Klasifikasi sudah locked, hanya dapat diubah via Reklasifikasi | 409      | Submit Reklasifikasi case       |
| ERR-BIZ-1020 | Counterparty tidak aktif atau rating expired                   | 409      | Update Counterparty Rating dulu |
| ERR-BIZ-1030 | Cannot soft-delete instrumen dengan transaksi aktif            | 409      | Resolve transactions first      |

## 1.2 Master Counterparty

Counterparty mencakup Bank, Bank Kustodian, Issuer Korporasi, Pemerintah, Manajer Investasi, dan Emiten Saham.

### 1.2.1 Field Specifications

| **Field**                | **Type**           | **Wajib**   | **Note**                                                                        |
| ------------------------ | ------------------ | ----------- | ------------------------------------------------------------------------------- |
| id (PK)                  | UUID               | Ya          | Auto                                                                            |
| kode\_counterparty       | VARCHAR(20) UNIQUE | Ya          | Auto-generate: CP-\#\#\#\#\#                                                    |
| nama                     | VARCHAR(200)       | Ya          | Nama legal                                                                      |
| tipe                     | VARCHAR(30)        | Ya          | BANK, BANK\_KUSTODIAN, KORPORASI, PEMERINTAH, MANAJER\_INVESTASI, EMITEN\_SAHAM |
| rating\_pefindo\_current | VARCHAR(8)         | Conditional | Wajib untuk BANK, KORPORASI, MI; null untuk PEMERINTAH (sovereign)              |
| tipe\_eksposur\_basel    | VARCHAR(30)        | Ya          | SOVEREIGN, SENIOR\_SECURED, SENIOR\_UNSECURED, SUBORDINATED                     |
| eligible\_lps\_flag      | BOOLEAN            | Ya          | Auto Y untuk BANK; N lainnya                                                    |
| npwp                     | VARCHAR(30)        | Tidak       | Encrypted at column level                                                       |
| nomor\_izin\_ojk         | VARCHAR(40)        | Conditional | Wajib untuk MANAJER\_INVESTASI                                                  |
| tanggal\_izin\_ojk       | DATE               | Conditional | Wajib untuk MI                                                                  |
| aum\_terakhir            | NUMERIC(20,2)      | Conditional | Untuk MI; update triwulanan                                                     |
| tanggal\_aum\_terakhir   | DATE               | Conditional | Untuk MI                                                                        |
| kategori\_mi             | VARCHAR(30)        | Conditional | BUMN, SWASTA\_NASIONAL, SWASTA\_ASING, JOINT\_VENTURE                           |
| status                   | VARCHAR(20)        | Ya          | AKTIF / TIDAK\_AKTIF                                                            |

### 1.2.2 API Endpoints

| **Method** | **Endpoint**                                    | **Permission**      |
| ---------- | ----------------------------------------------- | ------------------- |
| GET        | /api/v1/master/counterparty                     | counterparty.read   |
| GET        | /api/v1/master/counterparty/{id}                | counterparty.read   |
| POST       | /api/v1/master/counterparty                     | counterparty.create |
| PUT        | /api/v1/master/counterparty/{id}                | counterparty.update |
| GET        | /api/v1/master/counterparty/{id}/rating-history | rating.read         |
| POST       | /api/v1/master/counterparty/{id}/rating-history | rating.create       |
| GET        | /api/v1/master/counterparty/{id}/exposure       | counterparty.read   |

### 1.2.3 Sub-Module: Counterparty Rating History

Rating History per counterparty dengan auto-trigger SICR/Default. Field detail (mengikuti SoW §5.1.2.a):

| **Field**             | **Type**     | **Note**                                       |
| --------------------- | ------------ | ---------------------------------------------- |
| id (PK)               | UUID         | Auto                                           |
| counterparty\_id (FK) | UUID         | Ke mst.counterparty                            |
| tanggal\_berlaku      | DATE         | Effective date                                 |
| tanggal\_berakhir     | DATE         | Auto-set saat record baru diinput              |
| rating\_pefindo       | VARCHAR(8)   | idAAA - idD                                    |
| rating\_outlook       | VARCHAR(20)  | POSITIVE/STABLE/NEGATIVE/DEVELOPING            |
| sumber\_rating        | VARCHAR(30)  | PEFINDO\_REGULAR/PEFINDO\_REVIEW/LEMBAGA\_LAIN |
| tanggal\_publikasi    | DATE         | Tanggal terbit rating                          |
| action\_type          | VARCHAR(20)  | INITIAL/UPGRADE/DOWNGRADE/AFFIRMED/WITHDRAWN   |
| notch\_change         | INT          | Auto-calc vs rating sebelumnya                 |
| sicr\_triggered       | BOOLEAN      | Auto-calc berdasarkan threshold                |
| default\_triggered    | BOOLEAN      | Auto: TRUE jika rating=idD                     |
| dokumen\_bukti\_url   | VARCHAR(500) | Link ke dokumen Pefindo press release          |

**Trigger rule untuk SICR (BR-MAS-004 di BRD):**

  - Rating downgrade ≥ 2 notch dari rating origination → SICR triggered.

  - Rating berpindah dari investment grade (idBBB ke atas) ke non-investment grade (idBB ke bawah) → SICR.

  - Rating outlook NEGATIVE 2 review berturut-turut → SICR (qualitative).

  - Rating = idD atau gagal bayar \> 90 hari → DEFAULT (Stage 3).

## 1.3 Master PD Pefindo (12-Month + Lifetime)

PD Normal di-load dari Pefindo Default Study via upload triwulanan.

| **Field**                | **Type**     | **Note**                                  |
| ------------------------ | ------------ | ----------------------------------------- |
| id                       | UUID         | Auto                                      |
| rating                   | VARCHAR(8)   | idAAA - idD                               |
| pd\_12month              | NUMERIC(8,4) | Stage 1 PD (per annum)                    |
| pd\_lifetime\_3y         | NUMERIC(8,4) | Cumulative 3-year                         |
| pd\_lifetime\_5y         | NUMERIC(8,4) | Cumulative 5-year                         |
| pd\_lifetime\_7y         | NUMERIC(8,4) | Cumulative 7-year                         |
| pd\_lifetime\_10y        | NUMERIC(8,4) | Cumulative 10-year                        |
| sumber                   | VARCHAR(30)  | PEFINDO\_DS\_2025\_Q4 atau internal model |
| tanggal\_publikasi       | DATE         |                                           |
| periode\_berlaku\_dari   | DATE         |                                           |
| periode\_berlaku\_sampai | DATE         | Null jika current                         |
| dokumen\_pendukung\_url  | VARCHAR(500) | Pefindo PDF                               |

### 1.3.1 Upload Pefindo Workflow

8.  Risk Officer login → Master Data → PD Pefindo → Upload.

9.  Pilih file XLSX (sample format provided).

10. Sistem validate schema: kolom Rating, PD 1Y, PD 3Y, PD 5Y, PD 7Y, PD 10Y, Tanggal Berlaku.

11. Sistem stage data → review screen menampilkan diff vs PD existing.

12. Risk Officer review & approve.

13. Sistem commit; auto-calc SICR check pada seluruh instrumen aktif (re-evaluate berdasarkan PD baru — jika ada perubahan signifikan).

14. Audit trail: filename, hash, uploader, rows processed, row diff (before/after).

### 1.3.2 Linear Interpolation untuk Tenor Non-Standard

Untuk tenor yang tidak tersedia langsung di tabel (mis. 4 tahun, 6 tahun), sistem melakukan linear interpolation:

> function getLifetimePD(rating, tenor\_years):  
> if tenor\_years \<= 1:  
> return pd\_12month\[rating\]  
>   
> \# Find bracket  
> brackets = \[(3, pd\_3y), (5, pd\_5y), (7, pd\_7y), (10, pd\_10y)\]  
>   
> if tenor\_years \>= 10:  
> return pd\_10y\[rating\]  
>   
> for i in range(len(brackets) - 1):  
> t\_low, pd\_low = brackets\[i\]  
> t\_high, pd\_high = brackets\[i+1\]  
> if t\_low \<= tenor\_years \<= t\_high:  
> \# Linear interpolation  
> ratio = (tenor\_years - t\_low) / (t\_high - t\_low)  
> return pd\_low + ratio \* (pd\_high - pd\_low)

## 1.4 Master LGD Basel

LGD per tipe eksposur sesuai Basel III IRB Foundation Approach.

| **Tipe Eksposur**            | **LGD Default** | **Karakteristik**                             |
| ---------------------------- | --------------- | --------------------------------------------- |
| Sovereign / Pemerintah       | 0,4500          | SUN, SBN, Obligasi Pemerintah                 |
| Senior Secured               | 0,2500          | Obligasi dengan jaminan aktiva spesifik       |
| Senior Unsecured (Bank)      | 0,4500          | Cash di bank, deposito di luar penjaminan LPS |
| Senior Unsecured (Korporasi) | 0,4500          | Obligasi korporasi tanpa jaminan              |
| Subordinated                 | 0,7500          | Obligasi subordinasi, sukuk subordinasi       |

*Update mechanism: melalui workflow approval Risk Officer (Maker) → Risk Manager (Approver) bila Basel update kebijakan. Audit trail wajib dengan dokumen pendukung.*

## 1.5 Master Periode Buku

Detail spec di FSD Appendix D Bab 1 (Periode Buku Module). Section ini hanya menampilkan field master untuk reference.

| **Field**                 | **Type**           | **Note**                                |
| ------------------------- | ------------------ | --------------------------------------- |
| id                        | UUID               |                                         |
| periode\_id\_kode         | VARCHAR(20) UNIQUE | Mis. PRD-2026-01, PRD-2026-Q1, PRD-2026 |
| tipe\_periode             | VARCHAR(20)        | BULANAN/TRIWULANAN/TAHUNAN              |
| tahun\_buku               | INT                | Mis. 2026                               |
| bulan                     | INT                | 1-12; null kecuali BULANAN              |
| triwulan                  | INT                | 1-4; null kecuali TRIWULANAN            |
| tanggal\_mulai            | DATE               |                                         |
| tanggal\_akhir            | DATE               |                                         |
| status\_periode           | VARCHAR(20)        | OPEN/SOFT\_CLOSED/CLOSED                |
| tanggal\_soft\_close      | TIMESTAMPTZ        |                                         |
| tanggal\_hard\_close      | TIMESTAMPTZ        |                                         |
| user\_closer\_id          | UUID FK            |                                         |
| user\_approver\_close\_id | UUID FK            |                                         |
| reopened\_flag            | BOOLEAN            |                                         |
| reopened\_reason          | TEXT               |                                         |

## 1.6 Master Chart of Accounts & Mapping Jurnal

Detail spec di FSD Appendix D Bab 3. Section ini menampilkan reference field saja.

**Master CoA — key fields:**

| **Field**           | **Type**           | **Note**                                           |
| ------------------- | ------------------ | -------------------------------------------------- |
| kode\_akun          | VARCHAR(20) UNIQUE | Format struktur ERP (1.1.2.001, dst)               |
| nama\_akun          | VARCHAR(200)       |                                                    |
| tipe\_akun          | VARCHAR(20)        | ASET/LIABILITAS/EKUITAS/PENDAPATAN/BEBAN/KONTINJEN |
| sub\_tipe\_akun     | VARCHAR(30)        |                                                    |
| kategori\_investasi | VARCHAR(20)        | AC/FVOCI/FVTPL/OCI\_FVOCI/CKPN/null                |
| mata\_uang\_native  | CHAR(3)            | Default IDR                                        |
| posisi\_normal      | VARCHAR(10)        | DEBIT/KREDIT                                       |
| aktif\_flag         | BOOLEAN            |                                                    |
| parent\_akun\_id    | UUID FK self       |                                                    |
| sumber\_coa         | VARCHAR(20)        | INTERNAL/IMPORT\_ERP/IMPORT\_EXCEL                 |

*Master Mapping Jurnal — header + detail (lihat detail di Appendix D Bab 3).*

## 1.7 Master Mata Uang & Kurs

Detail spec di FSD Appendix D Bab 2. Reference field master:

| **Field (Master Mata Uang)** | **Type**    | **Note**                             |
| ---------------------------- | ----------- | ------------------------------------ |
| kode\_mata\_uang             | CHAR(3) PK  | ISO 4217                             |
| nama\_mata\_uang             | VARCHAR(60) |                                      |
| simbol                       | VARCHAR(5)  |                                      |
| sumber\_kurs\_default        | VARCHAR(30) | BI\_JISDOR/BI\_KURS\_TENGAH/INTERNAL |
| frekuensi\_update            | VARCHAR(20) | HARIAN/INTRA\_DAY/BULANAN            |
| aktif\_flag                  | BOOLEAN     |                                      |
| tanggal\_mulai\_aktif        | DATE        |                                      |

| **Field (FX Rate History)** | **Type**      | **Note**                        |
| --------------------------- | ------------- | ------------------------------- |
| id                          | UUID PK       |                                 |
| kode\_mata\_uang FK         | CHAR(3)       |                                 |
| tanggal\_berlaku            | DATE          |                                 |
| kurs\_beli                  | NUMERIC(15,4) | Optional                        |
| kurs\_jual                  | NUMERIC(15,4) | Optional                        |
| kurs\_tengah                | NUMERIC(15,4) | Wajib (untuk pembukuan)         |
| sumber\_kurs                | VARCHAR(30)   |                                 |
| periode\_bulanan\_id FK     | UUID          |                                 |
| locked\_flag                | BOOLEAN       | Auto Y saat periode HARD CLOSED |
| maker\_id                   | UUID FK       |                                 |
| approver\_id                | UUID FK       |                                 |
| dokumen\_bukti\_url         | VARCHAR(500)  |                                 |

*Unique constraint: (kode\_mata\_uang, tanggal\_berlaku).*

## 1.8 Master Portofolio

Master Portofolio menjadi unit Business Model Test. Setiap instrumen di-link ke 1 portofolio.

| **Field**                  | **Type**           | **Note**                                                   |
| -------------------------- | ------------------ | ---------------------------------------------------------- |
| id                         | UUID PK            |                                                            |
| kode\_portofolio           | VARCHAR(20) UNIQUE | Mis. PORT-TR-LIQ, PORT-INV-LT, PORT-TRADING                |
| nama                       | VARCHAR(200)       | Treasury Liquidity, Investment Long-Term, Trading, dll     |
| tujuan\_pengelolaan        | TEXT               | Naratif: tujuan strategis                                  |
| bm\_category\_default      | VARCHAR(10)        | HTC/HTCS/OTHER (default untuk instrumen di portofolio ini) |
| benchmark                  | VARCHAR(100)       | Mis. INDOBeX, IBPA Govt Bond Index                         |
| kompensasi\_manager\_basis | VARCHAR(50)        | Berbasis bunga/total return/fair value                     |
| periode\_review\_terakhir  | DATE               |                                                            |
| aktif\_flag                | BOOLEAN            |                                                            |

# 2\. SPPI Test Engine

## 2.1 Module Overview

SPPI (Solely Payments of Principal and Interest) Test menentukan apakah arus kas kontraktual aset keuangan memenuhi kriteria pure principal + interest sesuai PSAK 71. Hasil: PASS atau FAIL.

FAIL otomatis menghasilkan klasifikasi FVTPL (kecuali equity dengan FVOCI Election).

## 2.2 SPPI Checklist (10 Questions)

Sesuai SoW v1.1 §4.1.2 — checklist Q1 sampai Q10:

| **Q\#** | **Pertanyaan**                                               | **FAIL Indicator**                                           |
| ------- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| Q1      | Apakah pokok dan bunga didefinisikan jelas?                  | Pokok/bunga tidak jelas atau berbasis kinerja non-kredit     |
| Q2      | Apakah ada leverage/multiplier?                              | Faktor pengali \> 1 (mis. 3× LIBOR)                          |
| Q3      | Apakah ada link ke variabel non-kredit?                      | Linkage ke ekuitas/komoditas/forex non-functional            |
| Q4      | Apakah ada fitur konversi ekuitas?                           | Convertible bond → opsi konversi ke ekuitas                  |
| Q5      | Apakah ada fitur subordination yang non-standar?             | Subordinasi melebihi exposure-nya sendiri                    |
| Q6      | Apakah ada fitur prepayment / extension?                     | FAIL bila kompensasi prepayment \> nilai wajar selisih bunga |
| Q7      | Apakah suku bunga reset menggunakan tenor yang konsisten?    | Mismatch tenor material (mis. 3M rate reset 6M)              |
| Q8      | Apakah modifikasi suku bunga "de minimis"?                   | Modifikasi material vs benchmark                             |
| Q9      | Apakah aset dijamin oleh aset tertentu (non-recourse)?       | Underlying jaminan tidak lulus SPPI                          |
| Q10     | Apakah ada fitur kontingen yang dapat memodifikasi arus kas? | Kontinjensi tidak genuine atau memengaruhi SPPI material     |

## 2.3 Auto-Evaluation Logic

> function evaluateSPPI(answers):  
> \# answers: dict {Q1: 'YES'/'NO', Q2: ..., ..., Q10: ...}  
>   
> \# Q1: pokok/bunga jelas — Y diharapkan  
> if answers.Q1 == 'NO':  
> return ('FAIL', 'Q1: Pokok dan bunga tidak terdefinisi jelas')  
>   
> \# Q2: leverage — N diharapkan  
> if answers.Q2 == 'YES':  
> return ('FAIL', 'Q2: Terdapat leverage/multiplier')  
>   
> \# Q3: link non-kredit — N diharapkan  
> if answers.Q3 == 'YES':  
> return ('FAIL', 'Q3: Ada linkage ke variabel non-kredit')  
>   
> \# Q4: konversi ekuitas — N diharapkan (FAIL absolute)  
> if answers.Q4 == 'YES':  
> return ('FAIL', 'Q4: Ada fitur konversi ekuitas')  
>   
> \# Q5: subordination non-standar — N diharapkan  
> if answers.Q5 == 'YES':  
> return ('FAIL', 'Q5: Subordination non-standar')  
>   
> \# Q6: prepayment — perlu de minimis check  
> if answers.Q6 == 'YES' and answers.Q6\_de\_minimis\_pass \!= 'YES':  
> return ('FAIL', 'Q6: Prepayment compensation tidak de minimis')  
>   
> \# Q7: reset tenor — Y diharapkan (konsisten)  
> if answers.Q7 == 'NO':  
> return ('FAIL', 'Q7: Tenor reset tidak konsisten dan material')  
>   
> \# Q8: modifikasi de minimis — Y diharapkan  
> if answers.Q8 == 'NO':  
> return ('FAIL', 'Q8: Modifikasi suku bunga material')  
>   
> \# Q9: non-recourse — bila Y, perlu look-through ke jaminan  
> if answers.Q9 == 'YES' and answers.Q9\_underlying\_pass \!= 'YES':  
> return ('FAIL', 'Q9: Underlying jaminan tidak lulus SPPI')  
>   
> \# Q10: kontingen — N diharapkan (atau Y dengan justification not material)  
> if answers.Q10 == 'YES' and answers.Q10\_genuine \!= 'YES':  
> return ('FAIL', 'Q10: Fitur kontingen mempengaruhi SPPI material')  
>   
> return ('PASS', 'All criteria met')

## 2.4 SPPI Test Field Specifications

| **Field**            | **Type**           | **Note**                                                  |
| -------------------- | ------------------ | --------------------------------------------------------- |
| id                   | UUID PK            |                                                           |
| sppi\_test\_id\_kode | VARCHAR(20) UNIQUE | SPPI-2026-\#\#\#\#\#                                      |
| instrumen\_id FK     | UUID               |                                                           |
| tanggal\_test        | DATE               |                                                           |
| tipe\_test           | VARCHAR(20)        | INITIAL/PERIODIC/TRIGGERED                                |
| jawaban\_checklist   | JSONB              | {Q1: 'YES', Q1\_note: '...', ..., Q10: 'NO', ...}         |
| hasil\_sppi          | VARCHAR(10)        | PASS/FAIL — auto-derived                                  |
| catatan\_penilaian   | TEXT               | Justifikasi naratif                                       |
| dokumen\_bukti\_url  | VARCHAR(500)       | Term sheet, prospektus, opini legal                       |
| maker\_id FK         | UUID               | Treasury                                                  |
| reviewer\_id FK      | UUID               | Risk/Akuntansi                                            |
| approver\_id FK      | UUID               | Komite Investasi                                          |
| status               | VARCHAR(30)        | DRAFT/PENDING\_REVIEW/PENDING\_APPROVAL/APPROVED/REJECTED |

## 2.5 API Endpoints

| **Method** | **Endpoint**                       | **Permission**                              |
| ---------- | ---------------------------------- | ------------------------------------------- |
| GET        | /api/v1/sppi-test                  | sppi.read                                   |
| GET        | /api/v1/sppi-test/{id}             | sppi.read                                   |
| POST       | /api/v1/sppi-test                  | sppi.create                                 |
| PUT        | /api/v1/sppi-test/{id}             | sppi.update (only DRAFT status)             |
| POST       | /api/v1/sppi-test/{id}/submit      | sppi.submit (transition to PENDING\_REVIEW) |
| POST       | /api/v1/sppi-test/{id}/review      | sppi.review (approve/reject by reviewer)    |
| POST       | /api/v1/sppi-test/{id}/approve     | sppi.approve (by Komite)                    |
| GET        | /api/v1/sppi-test/{id}/audit-trail | sppi.read                                   |

# 3\. Business Model Test Engine

## 3.1 Module Overview

BM Test menentukan tujuan pengelolaan portofolio: Held to Collect (HTC), Held to Collect & Sell (HTC\&S), atau Other. Penilaian di level portofolio (bukan per instrumen).

## 3.2 BM Test Indicators

| **Indikator**                        | **HTC**                               | **HTC\&S**                         | **Other**                       |
| ------------------------------------ | ------------------------------------- | ---------------------------------- | ------------------------------- |
| Frekuensi Penjualan Historis (12M)   | Sangat jarang                         | Reguler & signifikan               | Sangat aktif                    |
| Volume Penjualan vs Total Portofolio | \< 5%                                 | 5-50%                              | \> 50%                          |
| Alasan Penjualan                     | Hanya credit deterioration / dekat JT | Manajemen likuiditas / rebalancing | Profit-taking / spekulasi       |
| Dasar Evaluasi Kinerja Manager       | Imbal hasil bunga                     | Bunga + realized gain/loss         | Total return / fair value       |
| Skema Kompensasi Manager             | Berbasis bunga & holding              | Berbasis total return moderat      | Berbasis fair value performance |

## 3.3 Auto-Suggest Logic

> function suggestBMCategory(portofolio\_id, period\_12m):  
> metrics = computePortofolioMetrics(portofolio\_id, period\_12m)  
> \# metrics: {volume\_jual\_pct, frekuensi\_jual, dasar\_evaluasi, ...}  
>   
> score\_htc = 0  
> score\_htcs = 0  
> score\_other = 0  
>   
> if metrics.volume\_jual\_pct \< 0.05:  
> score\_htc += 3  
> elif metrics.volume\_jual\_pct \<= 0.50:  
> score\_htcs += 3  
> else:  
> score\_other += 3  
>   
> if metrics.dasar\_evaluasi == 'IMBAL\_HASIL\_BUNGA':  
> score\_htc += 2  
> elif metrics.dasar\_evaluasi == 'BUNGA\_PLUS\_REALIZED':  
> score\_htcs += 2  
> elif metrics.dasar\_evaluasi == 'TOTAL\_RETURN':  
> score\_other += 2  
>   
> \# Other indicators...  
>   
> suggestion = max(\['HTC', 'HTCS', 'OTHER'\], key=lambda x: score\_x)  
> confidence = (max\_score / total\_indicators) \* 100  
>   
> return {  
> suggestion: suggestion,  
> confidence: confidence,  
> breakdown: {HTC: score\_htc, HTCS: score\_htcs, OTHER: score\_other}  
> }

## 3.4 BM Test Field Specifications

| **Field**                  | **Type**     | **Note**                                 |
| -------------------------- | ------------ | ---------------------------------------- |
| id                         | UUID PK      |                                          |
| bm\_test\_id\_kode         | VARCHAR(20)  | BMT-2026-\#\#\#\#\#                      |
| portofolio\_id FK          | UUID         |                                          |
| tanggal\_penilaian         | DATE         |                                          |
| tujuan\_pengelolaan        | TEXT         |                                          |
| indikator\_penilaian       | JSONB        | {volume\_jual\_pct: 0.045, ...}          |
| frekuensi\_penjualan\_12m  | NUMERIC(8,4) | % volume vs total                        |
| hasil\_bm\_test\_suggested | VARCHAR(10)  | HTC/HTCS/OTHER (auto)                    |
| hasil\_bm\_test\_final     | VARCHAR(10)  | HTC/HTCS/OTHER (after override jika ada) |
| override\_flag             | BOOLEAN      |                                          |
| justifikasi\_override      | TEXT         | Required jika override\_flag=TRUE        |
| dokumen\_bukti\_url        | VARCHAR(500) | Investment Policy, KPI manager, memo     |
| approver\_id FK            | UUID         | Komite Investasi                         |
| periode\_berlaku\_dari     | DATE         |                                          |
| periode\_berlaku\_sampai   | DATE         | Auto +1 year, refresh annual             |
| status                     | VARCHAR(30)  | DRAFT/PENDING\_APPROVAL/APPROVED/EXPIRED |

# 4\. Klasifikasi PSAK 71 Engine

## 4.1 Matriks Klasifikasi (SPPI × BM)

| **Hasil SPPI** | **HTC**             | **HTC\&S**             | **Other** |
| -------------- | ------------------- | ---------------------- | --------- |
| PASS           | Amortized Cost (AC) | FVOCI (with recycling) | FVTPL     |
| FAIL           | FVTPL               | FVTPL                  | FVTPL     |

**Pengecualian:**

  - Saham (otomatis FAIL SPPI) dapat di-elect FVOCI Election (irrevocable, no recycling) untuk strategic holdings — wajib approval Komite Investasi.

  - Reksadana (FAIL SPPI default FVTPL) dapat diklasifikasi FVOCI dengan kebijakan akuntansi (HTC\&S) — wajib approval Komite + dokumentasi kebijakan.

## 4.2 Auto-Derivation Logic

> function deriveKlasifikasiPSAK71(sppi\_result, bm\_category, fvoci\_election=False, tipe\_instrumen):  
> \# Saham — pengecualian khusus  
> if tipe\_instrumen == 'SAHAM':  
> if fvoci\_election:  
> return 'FVOCI\_ELECTION'  
> else:  
> return 'FVTPL'  
>   
> \# Reksadana — selalu FAIL SPPI  
> if tipe\_instrumen == 'REKSADANA':  
> if bm\_category == 'HTCS':  
> \# Dapat dipilih FVOCI dengan kebijakan akuntansi  
> return 'FVOCI' \# Bila approved  
> else:  
> return 'FVTPL'  
>   
> \# Standard matrix untuk Cash, Deposito, Obligasi  
> if sppi\_result == 'PASS':  
> if bm\_category == 'HTC':  
> return 'AC'  
> elif bm\_category == 'HTCS':  
> return 'FVOCI'  
> else: \# OTHER  
> return 'FVTPL'  
> else:  
> \# SPPI FAIL — semua FVTPL  
> return 'FVTPL'

## 4.3 Klasifikasi Lock Mechanism

Setelah klasifikasi di-approve oleh Komite Investasi:

  - Field klasifikasi\_psak71 di Master Instrumen ter-set.

  - Field klasifikasi\_locked\_at, klasifikasi\_locked\_by ter-set.

  - Klasifikasi tidak dapat diubah via PUT /master/instrumen/{id} (akan ERR-BIZ-1010).

  - Perubahan hanya melalui Reklasifikasi (Bab 5).

  - Trigger downstream: EIR computation initiated untuk AC/FVOCI utang.

# 5\. Reklasifikasi Prospektif (PSAK 71)

## 5.1 Matriks Reklasifikasi (6 Kombinasi)

| **Dari** | **Ke** | **Treatment Nilai Tercatat**                   | **Saldo OCI/P\&L**         | **EIR Treatment**         |
| -------- | ------ | ---------------------------------------------- | -------------------------- | ------------------------- |
| AC       | FVOCI  | Reukur ke FV                                   | Selisih → OCI              | EIR original tetap        |
| AC       | FVTPL  | Reukur ke FV                                   | Selisih → P\&L             | EIR di-deactivate         |
| FVOCI    | AC     | FV menjadi carrying baru; OCI direklas ke aset | OCI cumulative dieliminasi | EIR baru dihitung dari FV |
| FVOCI    | FVTPL  | Tidak ada reukur (sudah FV)                    | OCI cumulative → P\&L      | EIR di-deactivate         |
| FVTPL    | AC     | FV menjadi carrying baru; mulai amortisasi     | Tidak ada saldo OCI        | EIR baru dihitung dari FV |
| FVTPL    | FVOCI  | Tidak ada reukur; mulai akumulasi OCI          | Saldo OCI awal = 0         | EIR baru dihitung dari FV |

## 5.2 Reklasifikasi Workflow

15. Risk Officer / Treasury identify trigger reklasifikasi (Periodic Review SPPI/BM, Triggered Reassessment, atau model bisnis berubah).

16. Maker input case Reklasifikasi: instrumen, dari/ke klasifikasi, justifikasi, tanggal efektif, dokumen pendukung.

17. Sistem validate: tanggal efektif \>= tanggal hari ini; klasifikasi target valid sesuai matriks SPPI×BM.

18. Komite Investasi review case → approve/reject.

19. Sistem auto-generate jurnal transisi sesuai matriks (Bab 5.1) — auto-posting via Master Mapping Jurnal event REKLAS\_OCI\_PL atau related events.

20. Sistem update Master Instrumen: klasifikasi baru, tanggal efektif reklas, audit trail.

21. Untuk EIR-affected reklas (ke/dari AC): sistem recompute EIR dari FV pada tanggal efektif, generate Amortization Schedule baru.

22. Notifikasi ke seluruh stakeholder (Treasury, Risk, Akuntansi).

## 5.3 Reklasifikasi Field Specifications

| **Field**                     | **Type**      | **Note**                                 |
| ----------------------------- | ------------- | ---------------------------------------- |
| id                            | UUID PK       |                                          |
| reklas\_id\_kode              | VARCHAR(20)   | REKLAS-2026-\#\#\#\#\#                   |
| instrumen\_id FK              | UUID          |                                          |
| klasifikasi\_dari             | VARCHAR(20)   | AC/FVOCI/FVTPL                           |
| klasifikasi\_ke               | VARCHAR(20)   | AC/FVOCI/FVTPL                           |
| tanggal\_efektif              | DATE          |                                          |
| fair\_value\_tanggal\_efektif | NUMERIC(20,2) | Untuk computation                        |
| carrying\_amount\_dari        | NUMERIC(20,2) | Carrying sebelum reklas                  |
| accumulated\_oci\_dari        | NUMERIC(20,2) | OCI cumulative sebelum (jika dari FVOCI) |
| eir\_dari                     | NUMERIC(12,8) |                                          |
| eir\_ke                       | NUMERIC(12,8) | Auto-compute jika applicable             |
| justifikasi                   | TEXT          | Wajib                                    |
| dokumen\_bukti\_url           | VARCHAR(500)  |                                          |
| maker\_id FK                  | UUID          |                                          |
| approver\_id FK               | UUID          | Komite Investasi                         |
| jurnal\_transisi\_id FK       | UUID          | Reference ke jurnal yang ter-post        |
| status                        | VARCHAR(30)   |                                          |

# 6\. Pre-Trade Clearance Workflow

## 6.1 End-to-End Flow

Workflow lengkap dari penempatan instrumen baru sampai jurnal ter-posting:

23. Treasury Maker create Master Instrumen baru (status DRAFT, klasifikasi NULL).

24. Treasury Maker upload dokumen: term sheet, prospektus.

25. Treasury Maker run SPPI Test (jawab 10 questions). Sistem auto-evaluate.

26. Bila SPPI PASS: Treasury Maker run BM Test → sistem auto-suggest HTC/HTCS/OTHER.

27. Bila SPPI FAIL: skip BM Test → klasifikasi auto FVTPL (kecuali Saham FVOCI Election).

28. Sistem auto-derive klasifikasi PSAK 71 dari kombinasi SPPI × BM × FVOCI Election.

29. Risk/Akuntansi Reviewer review SPPI + BM + klasifikasi suggested.

30. Komite Investasi Approver approve klasifikasi → Master Instrumen klasifikasi locked.

31. Untuk AC/FVOCI utang: sistem trigger EIR computation; generate Amortization Schedule.

32. Treasury Maker input Transaksi Penempatan (Modul 5.2) → Treasury Manager Approve → Jurnal posting otomatis.

## 6.2 State Machine

> States: DRAFT → SPPI\_PENDING → BM\_PENDING → KLASIFIKASI\_PENDING\_REVIEW  
> → KLASIFIKASI\_PENDING\_APPROVAL → KLASIFIKASI\_LOCKED → AKTIF  
>   
> Transitions:  
> DRAFT → SPPI\_PENDING (Maker submit SPPI Test)  
> SPPI\_PENDING → BM\_PENDING (SPPI PASS) atau KLASIFIKASI\_PENDING\_REVIEW (SPPI FAIL skip BM)  
> BM\_PENDING → KLASIFIKASI\_PENDING\_REVIEW (BM Test submitted)  
> KLASIFIKASI\_PENDING\_REVIEW → KLASIFIKASI\_PENDING\_APPROVAL (Reviewer approve)  
> → DRAFT (Reviewer reject, back to Maker)  
> KLASIFIKASI\_PENDING\_APPROVAL → KLASIFIKASI\_LOCKED (Komite approve)  
> → DRAFT (Komite reject)  
> KLASIFIKASI\_LOCKED → AKTIF (after Transaksi Penempatan first time approved)  
>   
> Reverse: tidak ada transition mundur kecuali via Reklasifikasi.

## 6.3 Workflow SLAs

| **Step**                            | **Aktor**         | **SLA**                                     | **Eskalasi**                                                                                 |
| ----------------------------------- | ----------------- | ------------------------------------------- | -------------------------------------------------------------------------------------------- |
| SPPI Test submit                    | Treasury Maker    | —                                           | —                                                                                            |
| SPPI/BM Reviewer review             | Risk/Akuntansi    | 2 hari kerja                                | Reminder D-1, escalate ke supervisor jika lewat                                              |
| Komite Investasi approve            | Komite            | 5 hari kerja (atau hingga rapat berikutnya) | Notifikasi CFO bila \> 7 hari kerja                                                          |
| Treasury input Transaksi Penempatan | Treasury Maker    | Setelah klasifikasi locked, max 30 hari     | Otomatis cancel jika tidak ada follow-up dalam 30 hari (klasifikasi tetap valid untuk reuse) |
| Treasury Manager approve transaksi  | Treasury Approver | 1 hari kerja                                | Reminder D-0                                                                                 |

# Sign-Off Page

Appendix A ini merinci spec Master Data dan SPPI/BM Test sesuai BRD §8.1, 8.2 dan SoW §4, 5.1.

**Disusun oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Solution Designer                                                    | Lead Business Analyst                                                |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Disetujui oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Direktur IT                                                          | Direktur Investasi & Treasury                                        |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

*--- AKHIR APPENDIX A ---*
