*\[ LOGO TUGURE \]*

**FUNCTIONAL SPECIFICATION DOCUMENT**

**BLIPS IFRS 9 — APPENDIX B**

*Transaction Lifecycle: Penempatan • MTM • Renewal • Penjualan • Jatuh Tempo • Pendapatan • Media Upload*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**        | **Keterangan**                                                                                                                                                                      |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Judul Dokumen      | FSD Appendix B — Transaction Lifecycle (7 Modul)                                                                                                                                    |
| Kode Dokumen       | FSD-APP-B-2026-001                                                                                                                                                                  |
| Versi              | 1.1                                                                                                                                                                                 |
| Status             | DRAFT FOR REVIEW                                                                                                                                                                    |
| Tanggal Terbit     | 02 Mei 2026                                                                                                                                                                         |
| Reference Upstream | FSD Master v1.0; FSD Appendix A v1.1; BRD v1.1; SoW v1.3                                                                                                                            |
| Modul Tercakup     | Penempatan, MTM, Renewal, Penjualan, Jatuh Tempo, Pendapatan Investasi, Media Upload + Upload MTM Harian + Bulk Upload Master Instrumen (NEW v1.1)                                  |
| BR-IDs Tercakup    | BR-PNP-001 to 012, BR-MTM-001 to 009, BR-RNW-001 to 006, BR-SLE-001 to 008, BR-MAT-001 to 006, BR-PND-001 to 008, BR-UPL-001 to 009; plus Upload MTM & Bulk Master Instrumen (v1.1) |

# Outline Appendix B

| **Bab** | **Modul**                       | **SoW Reference** |
| ------- | ------------------------------- | ----------------- |
| 1       | Modul Penempatan Instrumen      | SoW §5.2          |
| 2       | Modul Mark-to-Market (MTM)      | SoW §5.3          |
| 3       | Modul Renewal (Khusus Deposito) | SoW §5.4          |
| 4       | Modul Penjualan / Pencairan     | SoW §5.5          |
| 5       | Modul Jatuh Tempo (Closure)     | SoW §5.6          |
| 6       | Modul Pendapatan Investasi      | SoW §5.7          |
| 7       | Modul Media Upload              | SoW §5.8          |

# 1\. Modul Penempatan Instrumen

## 1.1 Module Overview

Modul Penempatan mencatat transaksi pembelian/penempatan instrumen baru. Modul ini bekerja DOWNSTREAM dari pre-trade clearance (SPPI/BM Test approved); klasifikasi PSAK 71 sudah ter-lock di Master Instrumen.

**Pre-condition (BR-PNP-006):**

  - Master Instrumen tercatat dengan klasifikasi PSAK 71 ter-lock.

  - Counterparty aktif dengan rating Pefindo valid (kecuali sovereign).

  - Periode Buku tanggal transaksi dalam status OPEN.

  - Saldo rekening sumber ≥ total pembayaran.

  - Minimal 1 dokumen bukti ter-upload.

## 1.2 Screen Flow & UI Mockup

**Form Input — /transaksi/penempatan/new**

> ┌─────────────────────────────────────────────────────────────────────┐  
> │ ← Master Instrumen New Transaksi Penempatan │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ Kode Instrumen: \[Search/Pick Master Instrumen ▼\] │  
> │ └→ Auto-load: Tipe, Sub-Tipe, Klasifikasi (locked), Counterparty │  
> │ │  
> │ Periode Buku: \[Auto: PRD-2026-01 (OPEN)\] ✓ │  
> │ │  
> │ ▼ Detail Transaksi │  
> │ Tanggal Transaksi (Trade Date) \*: \[📅 15/01/2026\] │  
> │ Tanggal Settlement \*: \[📅 17/01/2026\] │  
> │ Nominal/Face Value \*: \[Rp 5.000.000.000,00\] │  
> │ Harga Beli (% atau NAB) \*: \[101,5000 %\] │  
> │ Accrued Interest Dibeli: \[Rp 0\] │  
> │ Total Pembayaran (auto): \[Rp 5.075.000.000,00\] │  
> │ Biaya Transaksi: \[Rp 5.000.000\] │  
> │ Akun Sumber Dana \*: \[Bank Mandiri Operating ▼\] │  
> │ └→ Saldo: Rp 12.500.000.000 ✓ (mencukupi) │  
> │ │  
> │ ▼ EIR Preview (auto-compute) │  
> │ EIR Awal: 0,04826688 (4,8267%) │  
> │ Premium: Rp 75.000.000 • Biaya Trans: Rp 5.000.000 │  
> │ Carrying Awal: Rp 5.080.000.000 │  
> │ \[📊 View Amortization Schedule (10 periode)\] │  
> │ │  
> │ ▼ Dokumen Pendukung \* │  
> │ \[+ Upload Dokumen\] (Bilyet, NoA, Konfirmasi) │  
> │ 📎 NoA\_OBL\_XYZ\_15Jan2026.pdf (uploaded) │  
> │ │  
> │ \[ Cancel \] \[ Save Draft \] \[ Submit \] │  
> └─────────────────────────────────────────────────────────────────────┘

## 1.3 Field Specifications

| **Field**               | **DB Column**             | **Type**           | **Wajib**       | **Validation**                                                    |
| ----------------------- | ------------------------- | ------------------ | --------------- | ----------------------------------------------------------------- |
| Nomor Transaksi         | no\_transaksi             | VARCHAR(20) UNIQUE | Auto            | Format: PNP-{YYYY}-{\#\#\#\#\#}                                   |
| Tanggal Transaksi       | tanggal\_transaksi        | DATE               | Ya              | ≤ hari ini; periode harus OPEN                                    |
| Tanggal Settlement      | tanggal\_settlement       | DATE               | Ya              | ≥ tanggal\_transaksi                                              |
| Kode Instrumen FK       | instrumen\_id             | UUID               | Ya              | Master Instrumen aktif, klasifikasi locked                        |
| Periode Buku ID FK      | periode\_id               | UUID               | Ya (auto)       | Auto dari tanggal\_transaksi; status OPEN                         |
| Nominal/Face Value      | nominal                   | NUMERIC(20,2)      | Ya              | \> 0; sesuai denominasi instrumen                                 |
| Harga Beli (% atau NAB) | harga\_beli               | NUMERIC(15,4)      | Ya jika OBL/RDN | \> 0; obligasi: % nominal; reksadana: NAB/unit                    |
| Jumlah Unit             | jumlah\_unit              | NUMERIC(18,4)      | Conditional     | Khusus reksadana; = total\_pembayaran ÷ NAB (toleransi 4 desimal) |
| Accrued Interest Dibeli | accrued\_interest\_dibeli | NUMERIC(20,2)      | Conditional     | Khusus obligasi inter-coupon                                      |
| Total Pembayaran        | total\_pembayaran         | NUMERIC(20,2)      | Auto            | \= harga × nominal + accrued + biaya                              |
| Biaya Transaksi         | biaya\_transaksi          | NUMERIC(20,2)      | Tidak           | ≥ 0; default 0                                                    |
| Akun Sumber Dana FK     | akun\_sumber\_dana\_id    | UUID               | Ya              | Mst rekening; saldo ≥ total\_pembayaran                           |
| Mata Uang               | mata\_uang                | CHAR(3)            | Auto            | Inherit dari Master Instrumen                                     |
| Kurs Tengah BI          | kurs\_tengah\_bi          | NUMERIC(15,4)      | Conditional     | Auto dari Master Kurs untuk tanggal\_transaksi (untuk valas)      |
| Total Pembayaran IDR    | total\_pembayaran\_idr    | NUMERIC(20,2)      | Auto            | \= total\_pembayaran × kurs\_tengah\_bi                           |
| Status Workflow         | workflow\_status          | VARCHAR(30)        | Auto            | DRAFT/PENDING\_APPROVAL/APPROVED/REJECTED                         |
| EIR Awal Computed       | eir\_awal                 | NUMERIC(12,8)      | Conditional     | Auto-compute untuk AC/FVOCI utang                                 |
| Carrying Amount Awal    | carrying\_amount\_awal    | NUMERIC(20,2)      | Auto            | \= harga\_beli + biaya\_transaksi (untuk AC/FVOCI)                |
| Maker ID FK             | maker\_id                 | UUID               | Auto            | Treasury Maker                                                    |
| Approver ID FK          | approver\_id              | UUID               | Conditional     | Treasury Manager (filled saat approve)                            |

## 1.4 Business Rules

1.  Cek Master Instrumen aktif dengan klasifikasi locked. Bila tidak: ERR-VAL-2001 'Klasifikasi PSAK 71 belum di-lock'.

2.  Cek Periode Buku status OPEN untuk tanggal\_transaksi. Bila SOFT\_CLOSED: ERR-BIZ-0010; bila CLOSED: hard block.

3.  Cek saldo rekening sumber ≥ total\_pembayaran (cross-reference ke saldo book Cash di Master). Bila kurang: ERR-VAL-2002.

4.  Cek counterparty aktif dengan rating valid (kecuali pemerintah). Bila rating expired (\> 1 tahun): WARN ke Risk untuk update.

5.  Untuk obligasi/deposito: tanggal\_jatuh\_tempo \> tanggal\_transaksi. Bila tidak: ERR-VAL-1001.

6.  Untuk reksadana: |jumlah\_unit × NAB - total\_pembayaran| ≤ 0,01 (toleransi pembulatan).

7.  Auto-trigger EIR computation untuk klasifikasi AC atau FVOCI utang setelah Approver approve.

8.  Auto-trigger jurnal posting event PENEMPATAN setelah approve. Jurnal dirinci di Master Mapping Jurnal (SoW §5.1.10).

## 1.5 API Endpoints

| **Method** | **Endpoint**                                  | **Permission**                            |
| ---------- | --------------------------------------------- | ----------------------------------------- |
| GET        | /api/v1/transaksi/penempatan                  | transaksi\_penempatan.read                |
| GET        | /api/v1/transaksi/penempatan/{id}             | transaksi\_penempatan.read                |
| POST       | /api/v1/transaksi/penempatan                  | transaksi\_penempatan.create              |
| PUT        | /api/v1/transaksi/penempatan/{id}             | transaksi\_penempatan.update (only DRAFT) |
| POST       | /api/v1/transaksi/penempatan/{id}/submit      | transaksi\_penempatan.submit              |
| POST       | /api/v1/transaksi/penempatan/{id}/approve     | transaksi\_penempatan.approve             |
| POST       | /api/v1/transaksi/penempatan/{id}/reject      | transaksi\_penempatan.reject              |
| GET        | /api/v1/transaksi/penempatan/{id}/eir-preview | eir.preview                               |
| GET        | /api/v1/transaksi/penempatan/{id}/jurnal      | jurnal.read                               |

## 1.6 Database Schema Preview

> CREATE TABLE trx.penempatan (  
> id UUID PRIMARY KEY DEFAULT uuidv7(),  
> no\_transaksi VARCHAR(20) NOT NULL UNIQUE,  
> tanggal\_transaksi DATE NOT NULL,  
> tanggal\_settlement DATE NOT NULL,  
> instrumen\_id UUID NOT NULL REFERENCES mst.instrumen(id),  
> periode\_id UUID NOT NULL REFERENCES mst.periode\_buku(id),  
> nominal NUMERIC(20,2) NOT NULL,  
> harga\_beli NUMERIC(15,4),  
> jumlah\_unit NUMERIC(18,4),  
> accrued\_interest\_dibeli NUMERIC(20,2) DEFAULT 0,  
> total\_pembayaran NUMERIC(20,2) NOT NULL,  
> biaya\_transaksi NUMERIC(20,2) DEFAULT 0,  
> akun\_sumber\_dana\_id UUID NOT NULL,  
> mata\_uang CHAR(3) NOT NULL DEFAULT 'IDR',  
> kurs\_tengah\_bi NUMERIC(15,4),  
> total\_pembayaran\_idr NUMERIC(20,2) NOT NULL,  
> eir\_awal NUMERIC(12,8),  
> carrying\_amount\_awal NUMERIC(20,2),  
> maker\_id UUID NOT NULL REFERENCES sec.user(id),  
> approver\_id UUID REFERENCES sec.user(id),  
> workflow\_status VARCHAR(30) NOT NULL DEFAULT 'DRAFT',  
> created\_at TIMESTAMPTZ NOT NULL DEFAULT now(),  
> updated\_at TIMESTAMPTZ,  
> approved\_at TIMESTAMPTZ,  
> jurnal\_header\_id UUID REFERENCES jrnl.header(id),  
> is\_deleted BOOLEAN DEFAULT FALSE,  
> CONSTRAINT ck\_settlement\_after\_trade CHECK (tanggal\_settlement \>= tanggal\_transaksi),  
> CONSTRAINT ck\_total\_positive CHECK (total\_pembayaran \> 0)  
> );  
>   
> CREATE INDEX ix\_penempatan\_instrumen ON trx.penempatan(instrumen\_id);  
> CREATE INDEX ix\_penempatan\_periode ON trx.penempatan(periode\_id);  
> CREATE INDEX ix\_penempatan\_tanggal ON trx.penempatan(tanggal\_transaksi);  
> CREATE INDEX ix\_penempatan\_status ON trx.penempatan(workflow\_status) WHERE is\_deleted=FALSE;

## 1.7 Error Codes Specific

| **Code**      | **Message**                                                     | **HTTP** |
| ------------- | --------------------------------------------------------------- | -------- |
| ERR-VAL-2001  | Klasifikasi PSAK 71 belum di-lock di Master Instrumen           | 409      |
| ERR-VAL-2002  | Saldo rekening sumber tidak mencukupi                           | 400      |
| ERR-VAL-2003  | Tanggal transaksi di luar periode buku OPEN                     | 409      |
| ERR-VAL-2004  | Kurs tengah BI tidak tersedia untuk tanggal transaksi (valas)   | 400      |
| ERR-VAL-2005  | Jumlah unit reksadana tidak match dengan total pembayaran ÷ NAB | 400      |
| ERR-CALC-2010 | EIR computation failed                                          | 500      |
| ERR-INT-2020  | Jurnal posting ke GL failed                                     | 503      |

# 2\. Modul Mark-to-Market (MTM)

## 2.1 Module Overview

MTM melakukan revaluation harian instrumen ke fair value berdasarkan harga referensi. Sistem mencatat selisih MTM dan posting jurnal sesuai klasifikasi.

## 2.2 MTM Coverage Matrix

| **Tipe Instrumen** | **Klasifikasi** | **MTM Frekuensi**        | **Sumber Harga** | **Pengakuan Selisih**                   |
| ------------------ | --------------- | ------------------------ | ---------------- | --------------------------------------- |
| Cash di Bank       | AC              | Harian saldo aktual      | Rekening koran   | Tidak ada MTM (AC)                      |
| Deposito           | AC              | Tidak ada                | Nominal pokok    | Tidak ada MTM (AC); akrual bunga harian |
| Obligasi           | FVOCI           | Harian                   | IBPA             | OCI                                     |
| Obligasi           | AC              | Harian (monitoring only) | IBPA             | Tidak dijurnal; hanya monitoring        |
| Saham              | FVTPL           | Harian                   | BEI close        | P\&L                                    |
| Saham              | FVOCI Election  | Harian                   | BEI close        | OCI no-recycling                        |
| Reksadana          | FVTPL           | Harian                   | NAB MI/KSEI      | P\&L                                    |
| Reksadana          | FVOCI           | Harian                   | NAB MI/KSEI      | OCI with recycling                      |

## 2.3 Daily MTM Job

Job batch end-of-day, scheduled jam 18:00 WIB pada hari kerja:

> job: daily\_mtm\_job  
> schedule: 0 18 \* \* MON-FRI  
>   
> steps:  
> 1\. Wait for price feeds (IBPA, NAB, BEI close).  
> 2\. Validate completeness — alert if missing for \> 5 instrumen.  
> 3\. For each active instrument with klasifikasi OBL/SAHAM/RDN\_\*:  
> a. Lookup last price from feed.  
> b. Compute new fair value = price × nominal/unit.  
> c. Compute MTM delta = new FV - carrying amount sebelumnya.  
> d. For valas: convert MTM delta IDR via kurs hari ini.  
> e. Insert trx.mtm record.  
> f. Trigger jurnal posting event MTM\_FVOCI / MTM\_FVTPL / MTM\_FVOCI\_ELECTION.  
> 4\. Generate MTM Summary Report.  
> 5\. Reconciliation check: total MTM impact vs P\&L + OCI movement.  
> 6\. Alert if any failure.  
>   
> Performance target: ≤ 30 menit untuk 1.500 instrumen.

## 2.4 Field Specifications

| **Field**                    | **Type**      | **Note**                             |
| ---------------------------- | ------------- | ------------------------------------ |
| id                           | UUID PK       |                                      |
| instrumen\_id FK             | UUID          |                                      |
| tanggal\_valuasi             | DATE          | Hari kerja                           |
| periode\_id FK               | UUID          | Auto                                 |
| carrying\_amount\_sebelumnya | NUMERIC(20,2) | Last carrying                        |
| harga\_referensi\_baru       | NUMERIC(15,4) | Dari IBPA/NAB/BEI                    |
| fair\_value\_baru            | NUMERIC(20,2) | Computed                             |
| selisih\_mtm\_native         | NUMERIC(20,2) | Mata uang asli                       |
| selisih\_mtm\_idr            | NUMERIC(20,2) | IDR equivalent                       |
| kurs\_tengah\_bi             | NUMERIC(15,4) | Untuk valas                          |
| akun\_pengakuan              | VARCHAR(20)   | OCI/LABA\_RUGI/NONE                  |
| sumber\_harga                | VARCHAR(30)   | IBPA/NAB\_MI/BEI/MANUAL              |
| dokumen\_sumber\_url         | VARCHAR(500)  | File upload IBPA daily               |
| jurnal\_header\_id FK        | UUID          | Reference                            |
| status\_flag                 | VARCHAR(20)   | POSTED/STALE\_PRICE/MANUAL\_OVERRIDE |

## 2.5 STALE\_PRICE Handling

Bila harga referensi tidak tersedia pada hari kerja:

  - Sistem retry pickup feed 3x dengan interval 30 menit.

  - Bila masih tidak tersedia jam 19:00 WIB: gunakan harga hari kerja sebelumnya, flag STALE\_PRICE.

  - Alert ke Akuntansi.

  - Konsekuensi: MTM delta dihitung tetap, tetapi flagged untuk review oleh Akuntansi.

  - Bila harga arrived later (misalnya hari berikutnya): sistem tidak retroactive update; pakai harga baru di MTM hari berikutnya. Akuntansi dapat manual upload bila perlu correct retroactively (PERIODE\_ADJUSTMENT untuk SOFT\_CLOSED).

# 3\. Modul Renewal (Khusus Deposito)

## 3.1 Module Overview

Renewal berlaku untuk deposito yang jatuh tempo. Mendukung dua skema: POKOK\_SAJA (bunga ditarik) dan POKOK\_PLUS\_BUNGA (bunga net digabung ke pokok).

## 3.2 Field Specifications

| **Field**               | **Type**           | **Note**                         |
| ----------------------- | ------------------ | -------------------------------- |
| id                      | UUID PK            |                                  |
| no\_renewal             | VARCHAR(20) UNIQUE | RNW-YYYY-\#\#\#\#\#              |
| instrumen\_lama\_id FK  | UUID               | Deposito yang JT                 |
| instrumen\_baru\_id FK  | UUID               | Auto-create master baru          |
| tanggal\_jt\_lama       | DATE               | Auto-fill                        |
| skema\_renewal          | VARCHAR(30)        | POKOK\_SAJA / POKOK\_PLUS\_BUNGA |
| tenor\_baru\_hari       | INT                | Untuk on-call                    |
| tenor\_baru\_bulan      | INT                | Untuk berjangka                  |
| suku\_bunga\_baru       | NUMERIC(8,4)       | % pa                             |
| pokok\_lama             | NUMERIC(20,2)      | Auto-fill                        |
| bunga\_akrual\_terakhir | NUMERIC(20,2)      | Auto-fill                        |
| pph\_bunga              | NUMERIC(20,2)      | \= bunga × 20%                   |
| bunga\_net              | NUMERIC(20,2)      | \= bunga - pph                   |
| pokok\_baru             | NUMERIC(20,2)      | Auto-calc per skema              |
| dokumen\_bukti\_url     | VARCHAR(500)       | Bilyet baru, surat instruksi     |
| maker\_id FK            | UUID               |                                  |
| approver\_id FK         | UUID               |                                  |
| workflow\_status        | VARCHAR(30)        |                                  |

## 3.3 Business Logic

> function processRenewal(deposito\_lama, skema, tenor\_baru, rate\_baru):  
> \# 1. Compute bunga akrual sampai tanggal JT  
> bunga\_akrual = computeBungaAkrual(deposito\_lama) \# via EIR jika applicable  
> pph = bunga\_akrual \* 0.20  
> bunga\_net = bunga\_akrual - pph  
>   
> \# 2. Compute pokok baru per skema  
> if skema == 'POKOK\_SAJA':  
> pokok\_baru = deposito\_lama.pokok  
> \# bunga net masuk ke akun bank / kas  
> kas\_in = bunga\_net  
> elif skema == 'POKOK\_PLUS\_BUNGA':  
> pokok\_baru = deposito\_lama.pokok + bunga\_net  
> kas\_in = 0  
>   
> \# 3. Create master instrumen baru (DEP-{YYYY}-\#\#\#\#\#)  
> instrumen\_baru = createMasterInstrumen({  
> kode: generateKode('DEP'),  
> nominal: pokok\_baru,  
> kupon: rate\_baru,  
> tanggal\_penempatan: deposito\_lama.tanggal\_jt,  
> tanggal\_jatuh\_tempo: deposito\_lama.tanggal\_jt + tenor\_baru,  
> klasifikasi: 'AC', \# inherit  
> ...  
> })  
>   
> \# 4. Create transaksi penempatan untuk instrumen baru  
> createTransaksiPenempatan(instrumen\_baru, ...)  
>   
> \# 5. Update deposito\_lama: status = JATUH\_TEMPO  
> deposito\_lama.status = 'JATUH\_TEMPO'  
>   
> \# 6. Compute EIR baru untuk instrumen\_baru  
> instrumen\_baru.eir\_awal = computeEIR(instrumen\_baru)  
> generateAmortizationSchedule(instrumen\_baru)  
>   
> \# 7. Post jurnal:  
> \# - Pembayaran kupon lama: D Kas (80%) + D PPh (20%) = K Akrual Bunga  
> \# - Penempatan baru: D Investasi Deposito Baru = K Kas (POKOK\_SAJA) atau K Investasi Deposito Lama (POKOK\_PLUS\_BUNGA)  
> postJurnal('PENEMPATAN', ...)

# 4\. Modul Penjualan / Pencairan

## 4.1 Module Overview

Penjualan/pencairan saat instrumen di-disposal sebelum jatuh tempo: break deposito, penjualan obligasi secondary market, redemption reksadana.

## 4.2 Field Specifications

| **Field**                    | **Type**      | **Note**                                            |
| ---------------------------- | ------------- | --------------------------------------------------- |
| id                           | UUID PK       |                                                     |
| no\_penjualan                | VARCHAR(20)   | JUL-YYYY-\#\#\#\#\#                                 |
| instrumen\_id FK             | UUID          |                                                     |
| tanggal\_penjualan           | DATE          | Trade date                                          |
| tanggal\_settlement          | DATE          |                                                     |
| nominal\_unit\_dijual        | NUMERIC(20,4) | Untuk parsial                                       |
| harga\_jual                  | NUMERIC(15,4) | % atau NAB                                          |
| accrued\_interest\_dijual    | NUMERIC(20,2) | Untuk obligasi                                      |
| total\_penerimaan            | NUMERIC(20,2) | Auto-calc                                           |
| biaya\_transaksi             | NUMERIC(20,2) | Komisi/redemption fee                               |
| carrying\_amount\_saat\_jual | NUMERIC(20,2) | Auto                                                |
| realized\_gain\_loss         | NUMERIC(20,2) | Auto: penerimaan - carrying - biaya                 |
| realized\_oci\_recycled      | NUMERIC(20,2) | Untuk FVOCI utang: akumulasi OCI di-recycle ke P\&L |
| dokumen\_bukti\_url          | VARCHAR(500)  |                                                     |
| maker\_id FK                 | UUID          |                                                     |
| approver\_id FK              | UUID          |                                                     |
| workflow\_status             | VARCHAR(30)   |                                                     |

## 4.3 OCI Recycling Logic

Khusus FVOCI utang dan FVOCI Reksadana — saat penjualan, akumulasi OCI direklas ke P\&L (recycling):

> function processPenjualan(instrumen, tanggal, harga\_jual, ...):  
> \# 1. Compute realized gain/loss  
> carrying = getCurrentCarrying(instrumen, tanggal)  
> realized\_gl = total\_penerimaan - carrying - biaya\_transaksi  
>   
> \# 2. Untuk FVOCI utang: recycle accumulated OCI  
> if instrumen.klasifikasi == 'FVOCI':  
> accumulated\_oci = getAccumulatedOCI(instrumen)  
> \# Posting:  
> \# D OCI Selisih MTM FVOCI Obligasi = accumulated\_oci (kontra)  
> \# K Realized Gain Reklasifikasi OCI = accumulated\_oci (P\&L)  
> postJurnal('REKLAS\_OCI\_PL', accumulated\_oci)  
>   
> \# 3. Untuk FVOCI Election Saham: TIDAK ada recycling  
> elif instrumen.klasifikasi == 'FVOCI\_ELECTION':  
> \# Accumulated OCI tetap di OCI permanent  
> \# Hanya catat realized cash flow  
> pass  
>   
> \# 4. Posting penjualan:  
> \# D Kas = total\_penerimaan  
> \# K Investasi (FVOCI/AC/FVTPL) = carrying  
> \# D/K Realized Gain/Loss (P\&L) = realized\_gl  
> postJurnal('PENJUALAN\_PENCAIRAN', ...)  
>   
> \# 5. Update Master Instrumen  
> if dijual\_penuh:  
> instrumen.status = 'DIJUAL'  
> deactivateAmortizationSchedule(instrumen)  
> else:  
> \# Parsial — kurangi nominal/unit  
> instrumen.nominal -= nominal\_dijual

## 4.4 Business Rules

9.  Untuk obligasi AC: jika dijual sebelum JT, trigger evaluasi BM Test (frekuensi penjualan vs threshold HTC). Jika threshold \> 5% terlampaui dalam 12-bulan: notifikasi reassessment ke Risk.

10. Break deposito: penalty (jika ada per term sheet) dikurangi dari total penerimaan.

11. Untuk reksadana: jumlah unit dijual ≤ jumlah unit aktual.

12. Wajib dokumen pendukung: konfirmasi penjualan / redemption confirmation.

13. Untuk FVOCI utang: trigger event REKLAS\_OCI\_PL bersamaan dengan PENJUALAN.

# 5\. Modul Jatuh Tempo (Closure)

## 5.1 Module Overview

Setiap H-1 jatuh tempo, sistem menampilkan list instrumen yang akan JT besok (dashboard reminder). Pada tanggal JT, sistem membentuk transaksi closure otomatis.

## 5.2 Logic per Tipe Instrumen

| **Tipe**                | **Behavior**                                                                                                   |
| ----------------------- | -------------------------------------------------------------------------------------------------------------- |
| Deposito Auto-Renewal=Y | Otomatis renewal sesuai skema rollover (lihat Modul 3)                                                         |
| Deposito Auto-Renewal=N | Settlement: pokok + bunga akrual sisa (post-PPh) ke rekening bank                                              |
| Obligasi                | Settlement: nominal par + kupon final (post-PPh) ke rekening bank; FVOCI utang: akumulasi OCI direklas ke P\&L |
| Reksadana               | Tidak ada JT (perpetual); harus via Modul Penjualan/Redemption                                                 |

## 5.3 Jatuh Tempo Job

> job: maturity\_processing\_job  
> schedule: 0 09 \* \* MON-FRI // 09:00 hari kerja  
>   
> steps:  
> 1\. Query: SELECT \* FROM mst.instrumen  
> WHERE tanggal\_jatuh\_tempo = CURRENT\_DATE  
> AND status = 'AKTIF'  
>   
> 2\. For each maturing instrument:  
> a. If DEPOSITO with auto\_renewal\_flag=TRUE:  
> → trigger renewal flow (Modul 3)  
> b. Else:  
> → create trx.jatuh\_tempo record  
> → compute pokok + bunga/kupon final  
> → post jurnal JATUH\_TEMPO  
> → for FVOCI utang: post REKLAS\_OCI\_PL  
> → update instrumen.status = 'JATUH\_TEMPO'  
> → deactivate amortization schedule  
>   
> 3\. For Amortization Schedule:  
> → verify Closing Carrying (last row) ≈ par (toleransi ±0,01 IDR)  
> → if mismatch: ERR-CALC-5001 + alert  
>   
> 4\. Generate Maturity Daily Report ke Akuntansi & Treasury.

# 6\. Modul Pendapatan Investasi

## 6.1 Module Overview

Modul Pendapatan menghitung akrual harian bunga/kupon, pengakuan dividen, dan distribusi reksadana, plus PPh terkait.

## 6.2 Akrual Harian Logic

**Untuk Cash di Bank (AC, EIR Method=N):**

> Akrual\_Harian = Saldo × Rate\_Harian (simple interest)

**Untuk Deposito (AC, EIR Method=Y):**

> Akrual\_Harian = Carrying × EIR ÷ 365 (untuk plain vanilla, ≈ Pokok × Rate ÷ 365)

**Untuk Obligasi AC dan FVOCI utang (EIR Method=Y):**

> Akrual\_Harian\_EIR = Gross\_Carrying × EIR ÷ 365 (Stage 1, 2)  
> Akrual\_Harian\_EIR = Net\_Carrying × EIR ÷ 365 (Stage 3)  
>   
> Kupon\_Kontraktual\_Harian = Nominal × Kupon ÷ 365  
>   
> Amortisasi\_P/D\_Harian = Akrual\_EIR - Kupon\_Kontraktual  
> (negatif untuk premium, positif untuk diskonto)

## 6.3 Dividen & Distribusi

| **Tipe**                     | **Treatment**                                        | **PPh**                                                            |
| ---------------------------- | ---------------------------------------------------- | ------------------------------------------------------------------ |
| Dividen Saham FVTPL          | Pendapatan Dividen ke P\&L pada cum-date             | PPh 10% Final (WP OP) atau exempt (WP Badan reinvestasi PP 9/2021) |
| Dividen Saham FVOCI Election | Pendapatan Dividen ke P\&L (BUKAN OCI)               | Same                                                               |
| Distribusi Reksadana         | Pendapatan Distribusi ke P\&L saat dibagikan oleh MI | Tidak kena PPh (bukan objek pajak)                                 |
| Kupon Obligasi Pemerintah    | Pendapatan Kupon ke P\&L pada cash-receive           | PPh Final 0% (untuk SUN, sesuai PP)                                |
| Kupon Obligasi Korporasi     | Pendapatan Kupon ke P\&L                             | PPh Final 10%                                                      |

## 6.4 Field Specifications (Akrual)

| **Field**                      | **Type**      | **Note**                   |
| ------------------------------ | ------------- | -------------------------- |
| id                             | UUID PK       |                            |
| instrumen\_id FK               | UUID          |                            |
| tanggal\_akrual                | DATE          |                            |
| periode\_id FK                 | UUID          |                            |
| carrying\_amount               | NUMERIC(20,2) | Gross atau Net (per Stage) |
| eir                            | NUMERIC(12,8) | EIR yang dipakai           |
| kupon\_kontraktual\_harian     | NUMERIC(20,2) | Native                     |
| pendapatan\_bunga\_eir\_harian | NUMERIC(20,2) | Native                     |
| amortisasi\_p\_d\_harian       | NUMERIC(20,2) | Native (= EIR - kupon)     |
| kurs\_tengah\_bi               | NUMERIC(15,4) |                            |
| pendapatan\_bunga\_idr         | NUMERIC(20,2) | Posted ke P\&L             |
| amortisasi\_p\_d\_idr          | NUMERIC(20,2) | Posted to carrying         |
| fx\_unrealized\_idr            | NUMERIC(20,2) | Selisih kurs harian        |
| stage\_saat\_akrual            | VARCHAR(20)   | STAGE\_1/2/3               |
| jurnal\_header\_id FK          | UUID          |                            |
| status                         | VARCHAR(20)   | POSTED/REVERSED            |

# 7\. Modul Media Upload

## 7.1 Module Overview

Repositori dokumen terenkripsi (S3 + KMS) untuk seluruh bukti dokumen transaksi. Setiap upload menghasilkan SHA-256 hash dan metadata.

## 7.2 Field Specifications (doc\_upload)

| **Field**           | **Type**     | **Note**                                                                               |
| ------------------- | ------------ | -------------------------------------------------------------------------------------- |
| id                  | UUID PK      |                                                                                        |
| filename            | VARCHAR(255) | Original filename                                                                      |
| filename\_storage   | VARCHAR(500) | Path di S3: documents/{year}/{month}/{entity}/{uuid}.{ext}                             |
| mime\_type          | VARCHAR(100) | application/pdf, image/png, dll                                                        |
| file\_size\_bytes   | BIGINT       | Max 50 MB                                                                              |
| sha256\_hash        | CHAR(64)     | Untuk integrity check                                                                  |
| entity\_type        | VARCHAR(50)  | INSTRUMEN/SPPI\_TEST/BM\_TEST/PENEMPATAN/MTM/RENEWAL/PENJUALAN/JATUH\_TEMPO/AKRUAL/dll |
| entity\_id          | UUID         | Reference ke entity                                                                    |
| uploader\_id FK     | UUID         |                                                                                        |
| uploaded\_at        | TIMESTAMPTZ  |                                                                                        |
| uploader\_ip        | INET         |                                                                                        |
| virus\_scan\_result | VARCHAR(20)  | CLEAN/INFECTED/PENDING                                                                 |
| virus\_scan\_at     | TIMESTAMPTZ  |                                                                                        |
| status              | VARCHAR(20)  | ACTIVE/INACTIVE                                                                        |
| inactive\_by FK     | UUID         | Hanya CFO                                                                              |
| inactive\_at        | TIMESTAMPTZ  |                                                                                        |
| inactive\_reason    | TEXT         |                                                                                        |

## 7.3 Upload Workflow

14. User pilih file di UI (atau drag-and-drop).

15. Frontend validate: extension allowed, size ≤ 50 MB.

16. Frontend POST /api/v1/documents/upload (multipart/form-data) dengan entity\_type & entity\_id.

17. Backend: stream file ke S3 staging area.

18. Backend: trigger virus scan (sync atau async).

19. Bila CLEAN: move file ke production location; compute SHA-256; store doc\_upload record.

20. Bila INFECTED: delete file; return error ERR-SEC-7001.

21. Backend response 201 dengan doc\_id; frontend update entity dengan doc\_id reference.

## 7.4 API Endpoints

| **Method** | **Endpoint**                                       | **Permission**                             |
| ---------- | -------------------------------------------------- | ------------------------------------------ |
| POST       | /api/v1/documents/upload                           | document.upload                            |
| GET        | /api/v1/documents/{id}                             | document.read                              |
| GET        | /api/v1/documents/{id}/download                    | document.download (returns pre-signed URL) |
| GET        | /api/v1/documents?entity\_type=...\&entity\_id=... | document.read                              |
| POST       | /api/v1/documents/{id}/inactivate                  | document.inactivate (CFO only)             |
| GET        | /api/v1/documents/{id}/access-log                  | document.audit (Auditor)                   |

## 7.5 Daftar Wajib Upload per Event (SoW §4.6.1)

| **Event**                 | **Dokumen Wajib**                                                           |
| ------------------------- | --------------------------------------------------------------------------- |
| Initial SPPI Test         | Term sheet, prospektus, draft perjanjian, opini legal (jika fitur kompleks) |
| Initial BM Test           | Investment Policy / Treasury Policy, memo Komite Investasi                  |
| Periodic Review SPPI      | Dokumen amandemen kontrak (jika ada)                                        |
| Periodic Review BM        | Riwayat penjualan 12-bulan, KPI manager, notulen Komite                     |
| Triggered Reassessment    | Memo trigger event, analisis dampak                                         |
| Reklasifikasi             | Memo persetujuan Komite, bukti tanggal efektif                              |
| Penempatan Obligasi/Saham | Bilyet, NoA, konfirmasi pembelian                                           |
| Penempatan Deposito       | Bilyet deposito, instruksi penempatan                                       |
| Penempatan Reksadana      | Bukti subscription, fund fact sheet                                         |
| Renewal Deposito          | Bilyet baru, surat instruksi rollover                                       |
| Penjualan                 | Konfirmasi penjualan, redemption confirmation                               |
| Jatuh Tempo Obligasi      | Settlement statement, bukti pembayaran                                      |
| Upload Pefindo Rating     | Press release Pefindo, rating action report                                 |
| Upload Kurs Manual        | Screenshot/PDF publikasi BI                                                 |
| Upload IBPA Daily         | File CSV IBPA                                                               |
| Upload NAB Reksadana      | Fund Fact Sheet PDF + XLSX                                                  |
| Stage Migration / Curing  | Memo Komite Risiko, dokumen pendukung                                       |
| EIR Re-estimation         | Dokumen amandemen kontrak, memo justifikasi                                 |
| Periode Buku Closing      | Checklist closing, rekonsiliasi GL                                          |

Appendix B merinci 7 modul transaksi lifecycle. Sign-off:

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Solution Designer                                                    | Lead BA Transaction                                                  |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

*--- AKHIR APPENDIX B ---*

# 8\. Modul Upload MTM Harian (NEW v1.1)

## 8.1 Module Overview

Modul Upload MTM Harian melengkapi Modul MTM (Bab 2) dengan mekanisme bulk upload harga referensi via Excel template, mendukung mode Automated Feed (scheduled job) dan Manual Upload (UI Akuntansi). Reference: SoW v1.3 §5.8.5.

## 8.2 Screen Flow & UI Mockup

**Upload Page — /transaksi/mtm/upload**

> ┌─────────────────────────────────────────────────────────────────────┐  
> │ Upload MTM Harian \[Help: Template XLSX ↓\] │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ Tanggal Valuasi: \[📅 2026-05-29\] (auto: hari kerja) │  
> │ Sumber Feed (auto): IBPA ✓ | NAB MI ✓ | BEI ✓ | Manual │  
> │ │  
> │ ▼ Drop XLSX file di sini atau \[Browse...\] │  
> │ 📎 mtm\_29May2026.xlsx (1.2 MB) ✓ scanned │  
> │ │  
> │ \[Parse & Stage\] │  
> │ │  
> │ ▼ Staging Preview (1.245 rows) │  
> │ 🟢 VALID: 1.180 rows │  
> │ 🟡 WARNING: 42 rows (PRICE\_DEVIATION\_HIGH) │  
> │ 🔴 REJECTED: 23 rows (INSTRUMEN\_TIDAK\_DITEMUKAN) │  
> │ │  
> │ \[Filter ▼\] \[Status: All ▼\] \[Tipe: All ▼\] │  
> │ ┌──────┬──────────────┬──────┬──────────┬──────────┬─────────────┐ │  
> │ │Row\# │ ISIN/Kode │ Tipe │ Harga │ Δ vs H-1 │ Status │ │  
> │ ├──────┼──────────────┼──────┼──────────┼──────────┼─────────────┤ │  
> │ │ 1 │ IDG000016605 │ OBL │ 101,5000 │ +0,2% │ 🟢 VALID │ │  
> │ │ 2 │ IDA000XXXXXX │ OBL │ 98,7500 │ -7,3% │ 🟡 WARNING │ │  
> │ │ 3 │ BBCA │ SHM │ 9.525,00 │ +1,1% │ 🟢 VALID │ │  
> │ │ ... │ ... │ ... │ ... │ ... │ ... │ │  
> │ └──────┴──────────────┴──────┴──────────┴──────────┴─────────────┘ │  
> │ │  
> │ \[Override Warning per Row\] \[Submit Batch for Approval\] │  
> └─────────────────────────────────────────────────────────────────────┘

## 8.3 Field Specifications (mtm\_upload\_batch + mtm\_upload\_row)

**Tabel staging — sys.upload\_batch (header):**

| **Field**                                    | **Type**           | **Note**                                                                        |
| -------------------------------------------- | ------------------ | ------------------------------------------------------------------------------- |
| id                                           | UUID PK            | Auto                                                                            |
| batch\_code                                  | VARCHAR(30) UNIQUE | BATCH-MTM-YYYY-MMDD-\#\#\#\#\#                                                  |
| batch\_type                                  | VARCHAR(20)        | MTM\_UPLOAD                                                                     |
| filename\_original                           | VARCHAR(500)       | Original filename                                                               |
| file\_sha256                                 | CHAR(64)           | Integrity check                                                                 |
| file\_storage\_url                           | VARCHAR(500)       | S3 reference                                                                    |
| uploaded\_by                                 | UUID FK            | Akuntansi user                                                                  |
| uploaded\_at                                 | TIMESTAMPTZ        |                                                                                 |
| tanggal\_valuasi                             | DATE               | Hari kerja                                                                      |
| total\_rows                                  | INT                |                                                                                 |
| valid\_rows / warning\_rows / rejected\_rows | INT each           |                                                                                 |
| committed\_rows                              | INT                |                                                                                 |
| mode                                         | VARCHAR(20)        | AUTOMATED / MANUAL / FALLBACK                                                   |
| status                                       | VARCHAR(20)        | PARSING / STAGED / PENDING\_APPROVAL / APPROVED / REJECTED / COMMITTED / FAILED |
| approver\_id                                 | UUID FK            | Finance Controller                                                              |
| approved\_at                                 | TIMESTAMPTZ        |                                                                                 |
| committed\_at                                | TIMESTAMPTZ        |                                                                                 |
| error\_summary\_json                         | JSONB              | Top-level errors                                                                |

**Tabel detail — sys.upload\_batch\_row (per row):**

| **Field**                   | **Type**      | **Note**                                                                                                                |
| --------------------------- | ------------- | ----------------------------------------------------------------------------------------------------------------------- |
| id                          | UUID PK       |                                                                                                                         |
| batch\_id FK                | UUID          |                                                                                                                         |
| row\_number                 | INT           | Urutan di file                                                                                                          |
| row\_data\_json             | JSONB         | Snapshot data row dari Excel                                                                                            |
| instrumen\_id FK            | UUID          | Resolved dari ISIN/Kode (null jika REJECTED)                                                                            |
| sumber\_harga               | VARCHAR(30)   | IBPA/NAB\_MI/NAB\_KSEI/BEI/MANUAL                                                                                       |
| harga\_native               | NUMERIC(15,4) | Harga from row                                                                                                          |
| harga\_sebelumnya           | NUMERIC(15,4) | From last MTM record                                                                                                    |
| deviation\_pct              | NUMERIC(8,4)  | (harga - sebelumnya) / sebelumnya                                                                                       |
| status\_validation          | VARCHAR(30)   | VALID / WARNING\_PRICE\_DEVIATION / REJECTED\_INSTRUMEN\_TIDAK\_DITEMUKAN / REJECTED\_KURS\_TIDAK\_TERSEDIA / DUPLICATE |
| validation\_errors\_json    | JSONB         |                                                                                                                         |
| override\_flag              | BOOLEAN       | TRUE jika maker override warning                                                                                        |
| override\_reason            | TEXT          | Wajib jika override\_flag=TRUE                                                                                          |
| override\_by                | UUID FK       |                                                                                                                         |
| committed\_to\_trx\_mtm\_id | UUID FK       | Reference setelah commit                                                                                                |
| status\_commit              | VARCHAR(30)   | PENDING / COMMITTED / SKIPPED                                                                                           |

## 8.4 API Endpoints

| **Method** | **Endpoint**                                                 | **Permission**      |
| ---------- | ------------------------------------------------------------ | ------------------- |
| GET        | /api/v1/mtm/upload/template?format=xlsx                      | mtm.upload.template |
| POST       | /api/v1/mtm/upload/batch                                     | mtm.upload.create   |
| GET        | /api/v1/mtm/upload/batch/{batch\_id}                         | mtm.upload.read     |
| GET        | /api/v1/mtm/upload/batch/{batch\_id}/rows                    | mtm.upload.read     |
| GET        | /api/v1/mtm/upload/batch/{batch\_id}/rows?status=WARNING     | mtm.upload.read     |
| POST       | /api/v1/mtm/upload/batch/{batch\_id}/rows/{row\_id}/override | mtm.upload.override |
| POST       | /api/v1/mtm/upload/batch/{batch\_id}/submit                  | mtm.upload.submit   |
| POST       | /api/v1/mtm/upload/batch/{batch\_id}/approve                 | mtm.upload.approve  |
| POST       | /api/v1/mtm/upload/batch/{batch\_id}/reject                  | mtm.upload.reject   |
| GET        | /api/v1/mtm/upload/history?from=\&to=                        | mtm.upload.read     |

## 8.5 Error Codes

| **Code**        | **Message**                                         | **HTTP**                 |
| --------------- | --------------------------------------------------- | ------------------------ |
| ERR-UPL-MTM-001 | Tanggal Valuasi bukan hari kerja                    | 400                      |
| ERR-UPL-MTM-002 | Periode buku untuk Tanggal Valuasi sudah CLOSED     | 409                      |
| ERR-UPL-MTM-010 | Instrumen tidak ditemukan untuk ISIN/Kode           | Per row WARNING/REJECTED |
| ERR-UPL-MTM-011 | Mata uang tidak sesuai dengan Master Instrumen      | Per row REJECTED         |
| ERR-UPL-MTM-020 | Price deviation \> 5% — requires override           | Per row WARNING          |
| ERR-UPL-MTM-030 | Duplicate (ISIN, Tanggal) — already POSTED          | Per row WARNING          |
| ERR-UPL-MTM-040 | Kurs Tengah BI tidak tersedia untuk instrumen valas | Per row REJECTED         |
| ERR-UPL-MTM-050 | File corrupt atau format invalid                    | 400                      |

# 9\. Modul Bulk Upload Master Instrumen (NEW v1.1)

## 9.1 Module Overview

Modul Bulk Upload Master Instrumen menyediakan mekanisme onboarding multiple instrumen sekaligus via Excel template dengan 5 sheets per Tipe Instrumen. Mendukung modes: MIGRATION (initial go-live), TOPUP (existing instrumen), DRY\_RUN (preview only), STANDARD (regular batch). Reference: SoW v1.3 §5.8.6.

## 9.2 Screen Flow & UI Mockup

**Bulk Upload Page — /master/instrumen/bulk-upload**

> ┌─────────────────────────────────────────────────────────────────────┐  
> │ Bulk Upload Master Instrumen │  
> ├─────────────────────────────────────────────────────────────────────┤  
> │ Mode: ( ) Standard ( ) Migration ( ) Top-Up ( ) Dry Run │  
> │ Portofolio Target: \[PORT-INV-LT ▼\] │  
> │ │  
> │ ▼ Template (XLSX dengan 5 sheets: CASH, DEPOSITO, OBLIGASI, │  
> │ SAHAM, REKSADANA) │  
> │ \[Download Template ↓\] │  
> │ │  
> │ ▼ Drop XLSX file: │  
> │ 📎 instrumen\_batch\_29May2026.xlsx (3.5 MB) ✓ │  
> │ │  
> │ \[Parse & Stage\] │  
> │ │  
> │ ▼ Staging Preview (450 rows total) │  
> │ Per Tipe Instrumen: │  
> │ CASH: 12 rows (12 VALID, 0 WARNING, 0 REJECTED) │  
> │ DEPOSITO: 48 rows (45 VALID, 2 WARNING, 1 REJECTED) │  
> │ OBLIGASI: 230 rows (215 VALID, 10 WARNING, 5 REJECTED) │  
> │ SAHAM: 85 rows (80 VALID, 3 WARNING, 2 REJECTED) │  
> │ REKSADANA: 75 rows (70 VALID, 4 WARNING, 1 REJECTED) │  
> │ │  
> │ \[View Details by Tipe ▼\] \[Download Error Report\] \[Edit Inline\] │  
> │ │  
> │ \[Submit for Approval (442 rows)\] \[Cancel\] \[Re-upload\] │  
> └─────────────────────────────────────────────────────────────────────┘

## 9.3 Field Specifications (extends sys.upload\_batch + sys.upload\_batch\_row)

Same table schema as Upload MTM (Bab 8.3) dengan batch\_type='INSTRUMEN\_BULK'. Additional field:

| **Field**                 | **Type**    | **Note**                                                 |
| ------------------------- | ----------- | -------------------------------------------------------- |
| batch\_mode               | VARCHAR(20) | STANDARD / MIGRATION / TOPUP / DRY\_RUN                  |
| portofolio\_target\_id FK | UUID        | Default portofolio jika tidak specified per row          |
| sheet\_breakdown\_json    | JSONB       | Stats per sheet (CASH/DEPOSITO/OBLIGASI/SAHAM/REKSADANA) |
| committed\_instrumen\_ids | UUID\[\]    | Array semua mst.instrumen.id yang berhasil di-commit     |
| rollback\_status          | VARCHAR(20) | NULL / PENDING\_ROLLBACK / ROLLED\_BACK                  |
| rollback\_by FK           | UUID        | CFO                                                      |
| rollback\_at              | TIMESTAMPTZ |                                                          |
| rollback\_reason          | TEXT        |                                                          |

## 9.4 Staging Validation Pipeline

Per-row validation runs in 4 stages, in order:

22. Schema validation: tipe column matches sheet name; required fields filled; data types valid (NUMERIC parses; DATE format ISO).

23. FK validation: counterparty\_kode resolves to mst.counterparty; portofolio\_kode resolves; manajer\_investasi\_kode (untuk RDN); mata\_uang exists.

24. Business rules: ck\_jt\_after\_penempatan; ck\_kupon\_nonneg; ck\_nominal\_positive; sub\_tipe valid untuk tipe\_instrumen.

25. Cross-reference: bila sppi\_test\_kode atau bm\_test\_kode disertakan, harus exists; klasifikasi consistency check.

**Staging output per row:**

> {  
> "row\_number": 42,  
> "status\_validation": "VALID" | "WARNING" | "REJECTED",  
> "validation\_errors": \[  
> { "stage": "FK", "field": "counterparty\_kode", "value": "CP-XXXX",  
> "message": "Counterparty not found", "severity": "ERROR" },  
> { "stage": "BUSINESS", "field": "tanggal\_jatuh\_tempo", "value": "2025-12-31",  
> "message": "JT must be \> tanggal\_penempatan (2026-01-15)", "severity": "ERROR" }  
> \],  
> "warnings": \[  
> { "field": "isin", "value": null,  
> "message": "ISIN missing — recommended for OBLIGASI", "severity": "WARNING" }  
> \],  
> "preview\_master\_instrumen": {  
> "kode\_instrumen": "OBL-2026-XXXXX (auto-generated)",  
> "klasifikasi\_psak71": "FVOCI (derived from SPPI PASS + BM HTCS)",  
> "eir\_awal\_preview": "0.04826688 (computed via Newton-Raphson)"  
> }  
> }

## 9.5 API Endpoints

| **Method** | **Endpoint**                                                               | **Permission**                     |
| ---------- | -------------------------------------------------------------------------- | ---------------------------------- |
| GET        | /api/v1/master/instrumen/bulk-upload/template?format=xlsx\&tipe=...        | instrumen.bulk.template            |
| POST       | /api/v1/master/instrumen/bulk-upload/batch                                 | instrumen.bulk.create              |
| GET        | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}                     | instrumen.bulk.read                |
| GET        | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/rows?status=\&tipe= | instrumen.bulk.read                |
| POST       | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/dry-run             | instrumen.bulk.dryrun              |
| PUT        | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/rows/{row\_id}      | instrumen.bulk.edit (inline fix)   |
| POST       | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/submit              | instrumen.bulk.submit              |
| POST       | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/approve             | instrumen.bulk.approve             |
| POST       | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/commit              | instrumen.bulk.commit (system)     |
| POST       | /api/v1/master/instrumen/bulk-upload/batch/{batch\_id}/rollback            | instrumen.bulk.rollback (CFO only) |
| GET        | /api/v1/master/instrumen/bulk-upload/history?from=\&to=\&mode=             | instrumen.bulk.read                |

## 9.6 Commit Pipeline (Post-Approval)

Setelah Approver approve batch, sistem run commit job (async):

26. Lock batch (status=COMMITTING) untuk prevent concurrent modification.

27. For each VALID row + override-approved WARNING row: BEGIN TRANSACTION.

28. INSERT INTO mst.instrumen ... — auto-generate kode\_instrumen via sequence.

29. IF sppi\_test\_kode dan bm\_test\_kode disertakan: INSERT INTO sppi.klasifikasi\_history untuk lock klasifikasi.

30. IF mode=MIGRATION: skip SPPI/BM workflow, use klasifikasi\_psak71 dari row direct lock.

31. IF tipe IN (AC, FVOCI utang): compute EIR via Newton-Raphson; INSERT INTO ecl.eir\_amortization\_schedule.

32. Audit log: aud.audit\_log insert dengan batch\_id reference.

33. Link committed instrumen\_id ke sys.upload\_batch\_row.committed\_to\_instrumen\_id.

34. Bila satu row fail: log error, SKIP row, continue ke row berikutnya (partial commit OK).

35. Update batch: status=COMMITTED, committed\_at=now(), committed\_rows=count(success).

36. Send notification ke Maker + Approver dengan batch summary report.

## 9.7 Rollback Specification

Rollback hanya boleh dilakukan oleh CFO dengan justifikasi memo:

  - Pre-condition: batch status=COMMITTED; tidak ada transaksi (penempatan, MTM, akrual) yang sudah ter-link ke instrumen yang di-commit dari batch ini.

  - Bila ada transaksi terkait: rollback BLOCKED dengan error 'TRANSACTIONS\_EXIST'; CFO harus reverse transaksi tersebut dulu (via prior-period adjustment) sebelum rollback batch.

  - Rollback execution: soft-delete semua mst.instrumen yang ter-link ke batch (is\_deleted=TRUE, delete\_reason='Rollback batch {batch\_code}'); update batch.rollback\_status='ROLLED\_BACK'.

  - Audit trail: aud.audit\_log per record yang di-soft-delete dengan reference ke rollback action.

# Sign-Off Page
