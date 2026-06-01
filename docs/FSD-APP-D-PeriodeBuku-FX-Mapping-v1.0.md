*\[ LOGO TUGURE \]*

**FUNCTIONAL SPECIFICATION DOCUMENT**

**BLIPS IFRS 9 — APPENDIX D**

*Periode Buku • FX Rate Management • Mapping Jurnal & GL Interface*

**PT TUGU REASURANSI INDONESIA**

(TUGURE)

Versi 1.0 • 02 Mei 2026

*Status: DRAFT FOR REVIEW*

# Atribut Dokumen

| **Atribut**        | **Keterangan**                                                                                 |
| ------------------ | ---------------------------------------------------------------------------------------------- |
| Kode Dokumen       | FSD-APP-D-2026-001                                                                             |
| Versi              | 1.0                                                                                            |
| Status             | DRAFT FOR REVIEW                                                                               |
| Tanggal Terbit     | 02 Mei 2026                                                                                    |
| Reference Upstream | FSD Master v1.0; BRD v1.0; SoW v1.1 §5.9, §5.1.8, §5.10, §5.1.10, §5.11                        |
| Modul Tercakup     | Periode Buku (3-status state machine), FX Rate Management, Master Mapping Jurnal, GL Interface |
| BR-IDs             | BR-PRD-001 to 014; BR-FX-001 to 013; BR-JNL-001 to 011                                         |

# 1\. Modul Periode Buku (Financial Period Management)

## 1.1 Module Overview

Periode Buku mengelola siklus akuntansi (bulanan/triwulanan/tahunan) dengan 3-status state machine: OPEN, SOFT\_CLOSED, CLOSED. Modul ini menjadi enforcement utama untuk integrity transaksi & posting jurnal.

## 1.2 State Machine

> States:  
> OPEN → Boleh input/edit/approve transaksi  
> SOFT\_CLOSED → Hanya Akuntansi adjustment journal (PERIODE\_ADJUSTMENT)  
> CLOSED → Hard-locked; koreksi via prior-period adjustment di periode terbuka berikutnya  
>   
> Transitions:  
> OPEN → SOFT\_CLOSED (request Akuntansi Maker → approve Finance Controller, H+5)  
> SOFT\_CLOSED → CLOSED (request Akuntansi Maker → approve CFO, H+15 max)  
> CLOSED → SOFT\_CLOSED (REOPEN — request CFO + approve CEO/Komite Audit)  
> SOFT\_CLOSED → OPEN (REOPEN — request CFO + approve CEO/Komite Audit)  
>   
> Auto Transitions:  
> \- Triwulanan auto soft-close: ketika 3 periode bulanan SOFT\_CLOSED  
> \- Tahunan auto soft-close: ketika 12 bulanan + 4 triwulanan CLOSED

## 1.3 Field Specs

Reference: FSD Appendix A §1.5 untuk full field specs. Highlight key fields untuk state machine:

| **Field**                    | **Type**    | **Note**                                  |
| ---------------------------- | ----------- | ----------------------------------------- |
| status\_periode              | VARCHAR(20) | OPEN/SOFT\_CLOSED/CLOSED                  |
| tanggal\_soft\_close         | TIMESTAMPTZ | Auto-set saat transition                  |
| tanggal\_hard\_close         | TIMESTAMPTZ | Auto-set saat CLOSED                      |
| user\_closer\_id FK          | UUID        | Akuntansi yang request                    |
| user\_approver\_close\_id FK | UUID        | Finance Controller (soft) atau CFO (hard) |
| catatan\_closing             | TEXT        | Optional                                  |
| reopened\_flag               | BOOLEAN     | Y bila pernah di-reopen                   |
| reopened\_reason             | TEXT        | Wajib jika reopened\_flag=Y               |
| reopened\_at                 | TIMESTAMPTZ |                                           |
| reopened\_by FK              | UUID        | CFO yang request                          |
| reopened\_approved\_by FK    | UUID        | CEO atau Komite Audit chair               |

## 1.4 Transition Rules

**OPEN → SOFT\_CLOSED Workflow**

1.  H+1 sampai H+5 hari kerja: Treasury complete entry transaksi.

2.  Otomatisasi job: akrual harian, MTM, ECL akhir bulan dijalankan.

3.  Akuntansi rekonsiliasi GL: cek saldo per akun, mismatch report.

4.  H+5 (recommended): Akuntansi Maker request soft-close via UI atau API POST /periode/{id}/soft-close.

5.  Sistem validate: tidak ada transaksi PENDING\_APPROVAL; semua jurnal balanced; rekonsiliasi GL pass.

6.  Finance Controller approve → status berubah SOFT\_CLOSED.

7.  Notification ke Treasury (no more new transactions for this period).

**SOFT\_CLOSED → CLOSED Workflow**

8.  H+15 (max): Akuntansi Maker request hard-close.

9.  Pre-condition: tidak ada adjustment PENDING\_APPROVAL; rekonsiliasi final pass.

10. CFO approve → status CLOSED.

11. Sistem auto-set Locked Flag untuk semua kurs pada periode ini.

12. Sistem disable input/edit untuk periode tersebut secara absolut.

**CLOSED → REOPEN (Special Procedure)**

13. Hanya CFO yang dapat memicu reopen request via API POST /periode/{id}/reopen-request.

14. Memo justifikasi + dokumen pendukung wajib.

15. CEO atau Komite Audit (untuk perusahaan publik) approve.

16. Reopened Flag set Y; audit trail simpan history reopen.

17. Setelah koreksi selesai, Akuntansi run workflow soft-close + hard-close ulang.

## 1.5 Validation Rules

| **Trigger**                      | **Validation**                                                                                                      |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Input transaksi baru             | Cek status periode dari Tanggal Transaksi. Jika SOFT\_CLOSED atau CLOSED → block dengan ERR-BIZ-PRD-001.            |
| Approve transaksi                | Cek lagi status periode (mungkin berubah sejak input). Jika tidak OPEN → block.                                     |
| Edit transaksi PENDING\_APPROVAL | Sama dengan input baru.                                                                                             |
| Backdated entry ke SOFT\_CLOSED  | Hanya Akuntansi (Finance Controller role); wajib via PERIODE\_ADJUSTMENT event dengan adjustment reason.            |
| Forward-dated entry              | Block; tidak boleh tanggal di masa depan.                                                                           |
| Job sistem (akrual, MTM, ECL)    | Hanya berjalan untuk periode OPEN; bila SOFT\_CLOSED hanya jika di-trigger manual oleh Risk untuk parameter update. |
| Soft-close request               | Pre-check: 0 PENDING\_APPROVAL; semua jurnal balanced; rekonsiliasi GL pass.                                        |
| Hard-close request               | Pre-check: 0 PENDING\_APPROVAL adjustment; rekonsiliasi final pass.                                                 |
| Hard-close periode tahunan       | Pre-check: semua 12 bulanan + 4 triwulanan CLOSED.                                                                  |

## 1.6 API Endpoints

| **Method** | **Endpoint**                                 | **Permission**                                 |
| ---------- | -------------------------------------------- | ---------------------------------------------- |
| GET        | /api/v1/periode-buku                         | periode.read                                   |
| GET        | /api/v1/periode-buku/{id}                    | periode.read                                   |
| GET        | /api/v1/periode-buku/current                 | periode.read (current month)                   |
| POST       | /api/v1/periode-buku/{id}/soft-close-request | periode.softclose.request (Akuntansi)          |
| POST       | /api/v1/periode-buku/{id}/soft-close-approve | periode.softclose.approve (Finance Controller) |
| POST       | /api/v1/periode-buku/{id}/hard-close-request | periode.hardclose.request (Akuntansi)          |
| POST       | /api/v1/periode-buku/{id}/hard-close-approve | periode.hardclose.approve (CFO)                |
| POST       | /api/v1/periode-buku/{id}/reopen-request     | periode.reopen.request (CFO)                   |
| POST       | /api/v1/periode-buku/{id}/reopen-approve     | periode.reopen.approve (CEO)                   |
| GET        | /api/v1/periode-buku/{id}/closing-checklist  | periode.read                                   |
| GET        | /api/v1/periode-buku/{id}/audit-trail        | periode.read                                   |

## 1.7 Schema Preview

> CREATE TABLE mst.periode\_buku (  
> id UUID PRIMARY KEY,  
> periode\_id\_kode VARCHAR(20) NOT NULL UNIQUE,  
> tipe\_periode VARCHAR(20) NOT NULL, -- BULANAN/TRIWULANAN/TAHUNAN  
> tahun\_buku INT NOT NULL,  
> bulan INT,  
> triwulan INT,  
> tanggal\_mulai DATE NOT NULL,  
> tanggal\_akhir DATE NOT NULL,  
> status\_periode VARCHAR(20) NOT NULL DEFAULT 'OPEN',  
> tanggal\_soft\_close TIMESTAMPTZ,  
> tanggal\_hard\_close TIMESTAMPTZ,  
> user\_closer\_id UUID REFERENCES sec.user(id),  
> user\_approver\_close\_id UUID REFERENCES sec.user(id),  
> catatan\_closing TEXT,  
> reopened\_flag BOOLEAN NOT NULL DEFAULT FALSE,  
> reopened\_reason TEXT,  
> reopened\_at TIMESTAMPTZ,  
> reopened\_by UUID REFERENCES sec.user(id),  
> reopened\_approved\_by UUID REFERENCES sec.user(id),  
> created\_at TIMESTAMPTZ NOT NULL DEFAULT now(),  
> updated\_at TIMESTAMPTZ,  
> CONSTRAINT ck\_status CHECK (status\_periode IN ('OPEN','SOFT\_CLOSED','CLOSED')),  
> CONSTRAINT ck\_tipe CHECK (tipe\_periode IN ('BULANAN','TRIWULANAN','TAHUNAN')),  
> CONSTRAINT uq\_bulan EXCLUDE USING GIST (  
> daterange(tanggal\_mulai, tanggal\_akhir, '\[\]') WITH &&  
> ) WHERE (tipe\_periode = 'BULANAN')  
> );  
>   
> CREATE INDEX ix\_periode\_status ON mst.periode\_buku(status\_periode);  
> CREATE INDEX ix\_periode\_tahun\_bulan ON mst.periode\_buku(tahun\_buku, bulan)  
> WHERE tipe\_periode='BULANAN';

# 2\. Modul FX Rate Management

## 2.1 Module Overview

FX Rate Management menyediakan kurs harian dengan kurs Tengah BI / JISDOR sebagai sumber resmi. Semua perhitungan internal (EAD, ECL, MTM) dilakukan dalam IDR equivalent.

## 2.2 Hierarchy of Rates per Event

| **Event Akuntansi**                 | **Kurs yang Dipakai**                            |
| ----------------------------------- | ------------------------------------------------ |
| Penempatan / Pembelian              | Kurs Tengah pada Tanggal Penempatan (trade date) |
| Akrual Bunga / Kupon Harian         | Kurs Tengah pada tanggal akrual                  |
| MTM Harian                          | Kurs Tengah pada tanggal closing                 |
| Hitung ECL Akhir Bulan              | Kurs Tengah pada period-end                      |
| Pembayaran Kupon (Realized)         | Kurs Tengah pada tanggal pembayaran              |
| Penjualan / Pencairan / Jatuh Tempo | Kurs Tengah pada tanggal closure                 |
| Closing Balance Periode (Reporting) | Kurs Tengah pada period-end                      |

## 2.3 BI JISDOR Scheduled Job

> job: bi\_jisdor\_update\_job  
> schedule: 30 10 \* \* MON-FRI // 10:30 hari kerja WIB  
>   
> steps:  
> 1\. HTTP GET ke BI JISDOR endpoint atau scrape resmi:  
> https://www.bi.go.id/id/statistik/informasi-kurs/jisdor-sgd  
>   
> 2\. Parse response (HTML / JSON / file):  
> \- USD/IDR Tengah, Bid, Ask  
> \- Tanggal publikasi  
>   
> 3\. Validate:  
> \- Tanggal = hari kerja sekarang  
> \- Kurs reasonability: deviasi ≤ 3% dari hari sebelumnya  
> \- Bila \> 3%: alert untuk Akuntansi review (bukan auto-reject)  
>   
> 4\. Compare dengan kurs hari ini di mst.kurs:  
> \- Jika kosong: insert baru dengan source=BI\_JISDOR  
> \- Jika sudah ada (manual upload): tidak overwrite; flag inconsistency  
>   
> 5\. Update Master Kurs:  
> \- kode\_mata\_uang: USD  
> \- tanggal\_berlaku: today  
> \- kurs\_tengah, kurs\_beli, kurs\_jual  
> \- sumber\_kurs: BI\_JISDOR  
> \- locked\_flag: FALSE (akan auto-set Y saat periode CLOSED)  
>   
> 6\. Trigger downstream:  
> \- Notify daily MTM job (now ready to start)  
> \- Notify FX revaluation job  
>   
> 7\. Log job\_run\_history dengan status SUCCESS/FAILED, rate value, anomalies  
>   
> failure handling:  
> \- Network error: retry 3x dengan 30 menit interval  
> \- Bila gagal jam 11:30: alert Akuntansi untuk manual upload  
> \- Manual upload: workflow Akuntansi Maker → Finance Controller Approver  
> \- Auto-flag REPEAT\_RATE jika tidak ada kurs untuk hari tertentu (use kurs hari kerja sebelumnya)

## 2.4 Field Specs (mst.kurs)

| **Field**               | **Type**                             | **Note**                                                |
| ----------------------- | ------------------------------------ | ------------------------------------------------------- |
| id                      | UUID PK                              |                                                         |
| fx\_rate\_id\_kode      | VARCHAR(20)                          | FX-{ccy}-{YYYYMMDD}                                     |
| kode\_mata\_uang FK     | CHAR(3)                              | Reference mst.mata\_uang                                |
| tanggal\_berlaku        | DATE                                 |                                                         |
| kurs\_beli              | NUMERIC(15,4)                        | Bid                                                     |
| kurs\_jual              | NUMERIC(15,4)                        | Ask                                                     |
| kurs\_tengah            | NUMERIC(15,4)                        | Mid — DIPAKAI UNTUK PEMBUKUAN                           |
| sumber\_kurs            | VARCHAR(30)                          | BI\_JISDOR/BI\_KURS\_TENGAH/UPLOAD\_MANUAL/REPEAT\_RATE |
| periode\_bulanan\_id FK | UUID                                 | Auto                                                    |
| locked\_flag            | BOOLEAN                              | Auto Y saat periode bulanan CLOSED                      |
| maker\_id FK            | UUID                                 | Wajib untuk UPLOAD\_MANUAL                              |
| approver\_id FK         | UUID                                 |                                                         |
| dokumen\_bukti\_url     | VARCHAR(500)                         | Wajib untuk UPLOAD\_MANUAL                              |
| created\_at             | TIMESTAMPTZ                          |                                                         |
| UNIQUE                  | (kode\_mata\_uang, tanggal\_berlaku) | Unique constraint                                       |

## 2.5 FX Gain/Loss Treatment

| **Tipe Gain/Loss**                   | **Pengakuan**                                                                  | **Akun**                              |
| ------------------------------------ | ------------------------------------------------------------------------------ | ------------------------------------- |
| Unrealized FX (FVTPL/AC)             | Harian → P\&L                                                                  | 4.1.4.002 Unrealized FX Gain/Loss     |
| Unrealized FX (FVOCI utang)          | Harian → P\&L (kunci PSAK 71: monetary items FX revaluation ke P\&L bukan OCI) | 4.1.4.002                             |
| Unrealized FX (FVOCI Election Saham) | Harian → OCI (no recycling)                                                    | 3.2.1.002 OCI Selisih MTM FVOCI Saham |
| Realized FX (Closure event)          | Saat penjualan/JT/pencairan → P\&L                                             | 4.1.4.001 Realized FX Gain/Loss       |

## 2.6 API Endpoints

| **Method** | **Endpoint**                                        | **Permission**                           |
| ---------- | --------------------------------------------------- | ---------------------------------------- |
| GET        | /api/v1/master/kurs?mata\_uang=\&tanggal=           | kurs.read                                |
| GET        | /api/v1/master/kurs/{id}                            | kurs.read                                |
| GET        | /api/v1/master/kurs/today                           | kurs.read (current rates)                |
| POST       | /api/v1/master/kurs/upload                          | kurs.upload (Akuntansi manual)           |
| POST       | /api/v1/master/kurs/{id}/approve                    | kurs.approve (Finance Controller)        |
| GET        | /api/v1/master/kurs/history?mata\_uang=\&from=\&to= | kurs.read                                |
| POST       | /api/v1/master/kurs/jisdor-sync                     | kurs.sync (manual trigger BI JISDOR job) |

# 3\. Modul Mapping Jurnal & GL Interface

## 3.1 Module Overview

Master Mapping Jurnal mendefinisikan template jurnal akuntansi untuk setiap event bisnis, dengan struktur Header-Detail. Sistem menggunakan event mapping generic — resolusi runtime berdasarkan klasifikasi/tipe/underlying.

## 3.2 Header & Detail Schema

**Master Event Jurnal Header (mst.mapping\_jurnal\_header):**

| **Field**                | **Type**           | **Note**                                                                                                   |
| ------------------------ | ------------------ | ---------------------------------------------------------------------------------------------------------- |
| id                       | UUID PK            |                                                                                                            |
| event\_id\_kode          | VARCHAR(40) UNIQUE | EVT-PENEMPATAN, EVT-AKRUAL\_BUNGA, EVT-AMORTISASI\_PREMI\_DISKONTO, dll                                    |
| event\_code              | VARCHAR(40)        | PENEMPATAN, AKRUAL\_BUNGA, dll (ENUM)                                                                      |
| nama\_event              | VARCHAR(120)       | Human-readable                                                                                             |
| kategori\_event          | VARCHAR(30)        | PENEMPATAN/AKRUAL/MUTASI\_MTM/PENDAPATAN/ECL/CLOSURE/REKLASIFIKASI/FX/STAGE\_MIGRATION/PERIODE\_ADJUSTMENT |
| trigger\_source          | VARCHAR(20)        | USER\_INPUT/SYSTEM\_JOB/UPLOAD                                                                             |
| tipe\_instrumen\_berlaku | VARCHAR\[\]        | Filter array; NULL = semua                                                                                 |
| klasifikasi\_berlaku     | VARCHAR\[\]        | Filter array; NULL = semua                                                                                 |
| aktif\_flag              | BOOLEAN            |                                                                                                            |
| catatan                  | TEXT               | Reference PSAK                                                                                             |

**Detail Mapping Jurnal (mst.mapping\_jurnal\_detail):**

| **Field**                | **Type**       | **Note**                                                                                                                                                                                                                                                                                                        |
| ------------------------ | -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| id                       | UUID PK        |                                                                                                                                                                                                                                                                                                                 |
| event\_header\_id FK     | UUID           | Reference ke header                                                                                                                                                                                                                                                                                             |
| urutan                   | INT            | 1, 2, 3, ...                                                                                                                                                                                                                                                                                                    |
| kode\_akun FK            | UUID → mst.coa | Akun specific                                                                                                                                                                                                                                                                                                   |
| dk\_indicator            | VARCHAR(10)    | DEBIT/KREDIT                                                                                                                                                                                                                                                                                                    |
| sumber\_amount           | VARCHAR(50)    | EAD\_IDR / NOMINAL\_IDR / BUNGA\_AKRUAL\_IDR / KUPON\_AKRUAL\_IDR / MTM\_DELTA\_IDR / ECL\_AMOUNT\_IDR / DIVIDEN\_GROSS\_IDR / DIVIDEN\_NETTO\_IDR / PPH\_AMOUNT\_IDR / REALIZED\_GAINLOSS\_IDR / UNREALIZED\_GAINLOSS\_IDR / FX\_UNREALIZED\_IDR / FX\_REALIZED\_IDR / OCI\_BALANCE\_IDR / AMORTISASI\_PD\_IDR |
| klasifikasi\_filter      | VARCHAR(20)    | AC/FVOCI/FVTPL/NULL                                                                                                                                                                                                                                                                                             |
| tipe\_instrumen\_filter  | VARCHAR\[\]    | Array filter                                                                                                                                                                                                                                                                                                    |
| underlying\_type\_filter | VARCHAR(20)    | NON\_EQUITY/EQUITY/NULL (untuk reksadana look-through)                                                                                                                                                                                                                                                          |
| multiplier               | NUMERIC(8,4)   | Default 1.0; misal -0.1 untuk PPh kupon 10%                                                                                                                                                                                                                                                                     |
| mata\_uang\_posting      | CHAR(3)        | Default IDR                                                                                                                                                                                                                                                                                                     |
| catatan                  | TEXT           |                                                                                                                                                                                                                                                                                                                 |

## 3.3 Resolusi Runtime Algorithm

> function resolveJurnal(event\_code, instrumen, amount\_data):  
> \# 1. Get header  
> header = getMappingHeader(event\_code)  
> if not header.aktif\_flag:  
> raise EventInactiveException()  
>   
> \# 2. Get all active detail lines  
> detail\_lines = getMappingDetails(header.id, where=aktif\_flag=TRUE)  
>   
> journal\_entries = \[\]  
> for line in detail\_lines:  
> \# 3. Filter check  
> if line.klasifikasi\_filter and line.klasifikasi\_filter \!= instrumen.klasifikasi:  
> continue  
> if line.tipe\_instrumen\_filter and instrumen.tipe not in line.tipe\_instrumen\_filter:  
> continue  
> if line.underlying\_type\_filter:  
> if not matchUnderlyingType(line.underlying\_type\_filter, instrumen):  
> continue  
>   
> \# 4. Get amount  
> amount = amount\_data\[line.sumber\_amount.lower()\] \# mis. amount\_data\['ead\_idr'\]  
> amount = amount \* line.multiplier  
>   
> \# 5. Build journal entry line  
> journal\_entries.append({  
> 'urutan': line.urutan,  
> 'kode\_akun': line.kode\_akun,  
> 'debit': amount if line.dk\_indicator == 'DEBIT' else 0,  
> 'kredit': amount if line.dk\_indicator == 'KREDIT' else 0,  
> 'mata\_uang': line.mata\_uang\_posting,  
> 'description': f"{header.nama\_event} - {instrumen.kode\_instrumen}"  
> })  
>   
> \# 6. Validate balance  
> total\_debit = sum(j.debit for j in journal\_entries)  
> total\_kredit = sum(j.kredit for j in journal\_entries)  
> if abs(total\_debit - total\_kredit) \> 0.01:  
> raise UnbalancedJurnalException(f"Debit {total\_debit} \!= Kredit {total\_kredit}")  
>   
> \# 7. Persist jurnal  
> header\_record = {  
> 'no\_jurnal': generateJurnalNumber(),  
> 'tanggal\_posting': today(),  
> 'periode\_id': getCurrentPeriode(),  
> 'event\_code': event\_code,  
> 'instrumen\_id': instrumen.id,  
> 'reference\_event\_id': amount\_data\['reference\_event\_id'\],  
> 'total\_debit': total\_debit,  
> 'total\_kredit': total\_kredit  
> }  
> persistJurnal(header\_record, journal\_entries)  
>   
> \# 8. Async deliver to GL Host  
> queueGLDelivery(header\_record)  
>   
> return header\_record

## 3.4 Sample Mapping (Event PENEMPATAN)

| **Urut** | **Kode Akun**                 | **D/K** | **Sumber Amount** | **Klasifikasi Filter** | **Multiplier** |
| -------- | ----------------------------- | ------- | ----------------- | ---------------------- | -------------- |
| 1        | 1.1.2.001 (SB AC Obligasi)    | DEBIT   | EAD\_IDR          | AC                     | 1,0000         |
| 1        | 1.1.3.001 (SB FVOCI Obligasi) | DEBIT   | EAD\_IDR          | FVOCI                  | 1,0000         |
| 1        | 1.1.4.001 (SB FVTPL Saham)    | DEBIT   | EAD\_IDR          | FVTPL                  | 1,0000         |
| 2        | 1.1.1.001 (Kas Mandiri)       | KREDIT  | EAD\_IDR          | (any)                  | 1,0000         |

*Resolusi runtime: untuk instrumen FVOCI, hanya line urut 1 dengan filter FVOCI yang aktif (lainnya skip). Line urut 2 berlaku untuk semua klasifikasi.*

## 3.5 Sample Mapping (Event PEMBAYARAN\_KUPON dengan PPh 10%)

| **Urut** | **Kode Akun**                     | **D/K** | **Sumber Amount**  | **Multiplier**     |
| -------- | --------------------------------- | ------- | ------------------ | ------------------ |
| 1        | 1.1.1.001 (Kas)                   | DEBIT   | KUPON\_AKRUAL\_IDR | 0,9000 (90% netto) |
| 2        | 5.1.2.001 (Beban PPh)             | DEBIT   | KUPON\_AKRUAL\_IDR | 0,1000 (10% PPh)   |
| 3        | 1.1.9.004 (Akrual Kupon Obligasi) | KREDIT  | KUPON\_AKRUAL\_IDR | 1,0000             |

*Total Debit = (0.9 + 0.1) × kupon = 1.0 × kupon. Total Kredit = 1.0 × kupon. BALANCE.*

## 3.6 GL Host Integration

Reference: FSD Master §5.2. Setiap jurnal yang ter-resolve dikirim ke GL Host via API REST atau file batch.

**Jurnal Header Schema:**

| **Field**             | **Type**           | **Note**                                                |
| --------------------- | ------------------ | ------------------------------------------------------- |
| id                    | UUID PK            |                                                         |
| no\_jurnal            | VARCHAR(20) UNIQUE | JNL-YYYY-MM-\#\#\#\#\#                                  |
| tanggal\_posting      | DATE               |                                                         |
| periode\_id FK        | UUID               |                                                         |
| event\_code           | VARCHAR(40)        | PENEMPATAN/AKRUAL\_BUNGA/dll                            |
| instrumen\_id FK      | UUID               | Optional (untuk transaksi-related)                      |
| reference\_event\_id  | UUID               | FK ke trx.\* atau ecl.calc\_header (untuk traceability) |
| currency              | CHAR(3)            | IDR                                                     |
| total\_debit          | NUMERIC(20,2)      |                                                         |
| total\_kredit         | NUMERIC(20,2)      |                                                         |
| narrative             | VARCHAR(500)       | Description                                             |
| status\_internal      | VARCHAR(20)        | PENDING/POSTED/REVERSED                                 |
| gl\_host\_status      | VARCHAR(20)        | PENDING\_DELIVERY/DELIVERED/FAILED/RETRYING             |
| gl\_host\_journal\_id | VARCHAR(50)        | ID dari GL Host setelah ter-post                        |
| gl\_delivered\_at     | TIMESTAMPTZ        |                                                         |
| gl\_retry\_count      | INT                | Default 0                                               |
| gl\_last\_error       | TEXT               |                                                         |
| created\_at           | TIMESTAMPTZ        |                                                         |

## 3.7 Daily Reconciliation

Setiap end-of-day, sistem reconcile BLIPS jurnal vs GL Host:

18. Query semua BLIPS jurnal dengan status\_internal=POSTED untuk periode sekarang.

19. Untuk setiap jurnal, cek gl\_host\_status = DELIVERED dan gl\_host\_journal\_id terisi.

20. Bila ada PENDING\_DELIVERY \> 1 jam: alert IT.

21. Bila FAILED \> 3 retry: dead letter queue + escalate ke Akuntansi.

22. Bandingkan total debit/kredit per akun di BLIPS vs di GL Host (via GL API).

23. Mismatch \> tolerance → alert P1.

24. Generate Daily Reconciliation Report.

## 3.8 API Endpoints

| **Method** | **Endpoint**                                 | **Permission**                 |
| ---------- | -------------------------------------------- | ------------------------------ |
| GET        | /api/v1/mapping-jurnal/header                | mapping\_jurnal.read           |
| GET        | /api/v1/mapping-jurnal/header/{id}           | mapping\_jurnal.read           |
| POST       | /api/v1/mapping-jurnal/header                | mapping\_jurnal.create         |
| PUT        | /api/v1/mapping-jurnal/header/{id}           | mapping\_jurnal.update         |
| GET        | /api/v1/mapping-jurnal/header/{id}/detail    | mapping\_jurnal.read           |
| POST       | /api/v1/mapping-jurnal/header/{id}/detail    | mapping\_jurnal.update         |
| PUT        | /api/v1/mapping-jurnal/detail/{id}           | mapping\_jurnal.update         |
| DELETE     | /api/v1/mapping-jurnal/detail/{id}           | mapping\_jurnal.update         |
| POST       | /api/v1/mapping-jurnal/import-excel          | mapping\_jurnal.import         |
| GET        | /api/v1/mapping-jurnal/export-excel          | mapping\_jurnal.export         |
| GET        | /api/v1/jurnal/header?event=\&periode=       | jurnal.read                    |
| GET        | /api/v1/jurnal/header/{id}                   | jurnal.read                    |
| GET        | /api/v1/jurnal/header/{id}/detail            | jurnal.read                    |
| POST       | /api/v1/jurnal/header/{id}/retry-gl-delivery | jurnal.retry (manual trigger)  |
| POST       | /api/v1/jurnal/header/{id}/reverse           | jurnal.reverse (untuk koreksi) |
| GET        | /api/v1/reconciliation/daily?periode=        | reconciliation.read            |

# Sign-Off Page

Appendix D mencakup tata kelola periode, FX, dan jurnal — fondasi compliance accounting.

**Disusun oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Solution Designer                                                    | Lead Akuntansi BA                                                    |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

**Disetujui oleh:**

| **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** | **\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_** |
| -------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Direktur Keuangan (CFO)                                              | Direktur Teknologi Informasi                                         |
| Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        | Tanggal: \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_                        |

*--- AKHIR APPENDIX D ---*
