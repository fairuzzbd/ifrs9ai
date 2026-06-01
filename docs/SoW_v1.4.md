**SCOPE OF WORK & FLOW**

**SISTEM BLIPS IFRS 9 INSTRUMEN INVESTASI**

*Modul Penempatan, Mark-to-Market, Renewal, Penjualan, Jatuh Tempo, Pendapatan Investasi, Media Upload, dan Perhitungan Expected Credit Loss (ECL)*

Klasifikasi Akuntansi: PSAK 71 — Instrumen Keuangan

Sumber Parameter Risiko: Pefindo (PD) • Basel (LGD) • Forward-Looking via Media Upload (Impact PD)

Presisi Perhitungan: 4 Angka Desimal

| **Atribut**             | **Keterangan**                                                 |
| ----------------------- | -------------------------------------------------------------- |
| Judul Dokumen           | SoW & Flow Sistem Instrumen Investasi Sederhana                |
| Versi                   | 1.4                                                            |
| Bahasa                  | Bahasa Indonesia                                               |
| Standar Akuntansi Acuan | PSAK 71 (PSAK 50/55 selaras), PSAK 65 (look-through reksadana) |
| Standar Risiko Acuan    | Basel III IRB Foundation (LGD), Pefindo Default Study (PD)     |
| Lingkungan Pengguna     | Treasury / Investment / Accounting / Risk Management           |

# 1\. Pendahuluan

## 1.1 Latar Belakang

Dokumen ini menguraikan Scope of Work (SoW) dan alur (flow) untuk pembangunan sistem pengelolaan instrumen investasi sederhana. Cakupan instrumen meliputi Cash di Bank, Deposito Berjangka, Obligasi (pemerintah maupun korporasi), dan Reksadana. Sistem dirancang untuk mendukung siklus hidup penuh instrumen — mulai dari penempatan, mark-to-market (MTM), renewal, penjualan, jatuh tempo, hingga pengakuan pendapatan investasi — disertai fasilitas media upload bukti dokumen, serta perhitungan Expected Credit Loss (ECL) sesuai PSAK 71 dengan tiga skenario (optimistic, base, pessimistic) dan penyesuaian forward-looking.

## 1.2 Tujuan

1.  Menyediakan satu sistem terintegrasi untuk pencatatan dan pelaporan instrumen investasi sederhana.

2.  Menjamin pemenuhan tata kelola dokumen melalui fasilitas media upload yang melekat pada setiap event instrumen.

3.  Menyediakan perhitungan ECL berbasis tiga skenario probabilitas default (PD) dengan penyesuaian forward-looking (Impact PD) yang bersumber dari upload.

4.  Menghasilkan jurnal akuntansi otomatis untuk setiap event sehingga rekonsiliasi antara modul investasi dan general ledger dapat dilakukan otomatis.

5.  Memenuhi presisi perhitungan 4 angka desimal pada seluruh parameter rasio (PD, LGD, Impact PD, bobot skenario).

## 1.3 Definisi & Singkatan

| **Istilah**                   | **Definisi**                                                                                                                                                                                                                                                                       |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| ECL                           | Expected Credit Loss — cadangan kerugian penurunan nilai (CKPN) berbasis ekspektasi sesuai PSAK 71.                                                                                                                                                                                |
| EAD                           | Exposure at Default — nilai eksposur yang berisiko default pada saat default terjadi.                                                                                                                                                                                              |
| PD                            | Probability of Default — probabilitas counterparty mengalami default dalam horizon waktu tertentu.                                                                                                                                                                                 |
| LGD                           | Loss Given Default — proporsi kerugian terhadap eksposur jika default terjadi.                                                                                                                                                                                                     |
| Impact PD                     | Multiplier forward-looking yang diterapkan di tingkat OUTPUT (ECL Weighted) untuk menghasilkan ECL FL. Bekerja sebagai layer overlay final.                                                                                                                                        |
| Impact MEV to PD              | Multiplier forward-looking yang diterapkan di tingkat INPUT (PD Normal) untuk men-derivasi PD Good (Optimistic) dan PD Bad (Pessimistic). Diturunkan dari proyeksi variabel makroekonomi (GDP, inflasi, BI Rate, dll).                                                             |
| PD Normal                     | Probabilitas default 12-bulan kondisi netral, bersumber dari Pefindo Default Study. Dipakai sebagai PD Base.                                                                                                                                                                       |
| PD Good                       | PD skenario optimistic = PD Normal × Impact MEV (Good).                                                                                                                                                                                                                            |
| PD Bad                        | PD skenario pessimistic = PD Normal × Impact MEV (Bad).                                                                                                                                                                                                                            |
| MEV                           | Macroeconomic Variables — variabel makroekonomi (GDP growth, inflasi, BI Rate, USD/IDR, harga minyak, IHSG growth, dll) yang menjadi basis perhitungan Impact MEV to PD.                                                                                                           |
| Periode Buku                  | Periode akuntansi yang dipakai sistem untuk stamp transaksi dan kontrol posting. Memiliki 3 status: OPEN (boleh input/edit), SOFT\_CLOSED (hanya Akuntansi yang bisa adjustment), CLOSED (hard-locked).                                                                            |
| Soft Close                    | Status periode di mana transaksi normal tidak bisa diinput, tetapi adjustment journal entry oleh Akuntansi masih diperbolehkan. Umumnya berlangsung H+5 sampai H+15 setelah akhir bulan.                                                                                           |
| Hard Close                    | Status periode yang sepenuhnya terkunci. Tidak ada perubahan yang dapat dilakukan; koreksi error harus via prior-period adjustment pada periode terbuka berikutnya (PSAK 25).                                                                                                      |
| Backdated Entry               | Input transaksi dengan Tanggal Transaksi pada periode lampau yang masih SOFT\_CLOSED. Hanya dapat dilakukan oleh Akuntansi dengan adjustment reason.                                                                                                                               |
| Prior-Period Adjustment       | Koreksi error material dari periode yang sudah CLOSED, dilakukan via journal entry pada periode terbuka berikutnya, mengikuti prinsip PSAK 25 Kebijakan Akuntansi, Perubahan Estimasi Akuntansi, dan Kesalahan.                                                                    |
| IDR Equivalent                | Nilai dalam Rupiah dari instrumen yang berdenominasi mata uang asing, dihitung dengan: nilai mata uang asli × Kurs Tengah BI pada tanggal evaluasi. Semua perhitungan EAD, ECL, MTM, dan reporting menggunakan IDR equivalent.                                                     |
| Kurs Tengah BI                | Kurs konversi mata uang asing yang dipublikasikan Bank Indonesia setiap hari kerja (JISDOR untuk USD, kurs tengah BI untuk mata uang lainnya). Sumber resmi untuk pembukuan.                                                                                                       |
| Unrealized FX Gain/Loss       | Selisih kurs yang belum direalisasi pada instrumen valas yang masih aktif, dihitung dari perbedaan IDR equivalent saat ini dengan IDR equivalent posisi sebelumnya. Diakui di P\&L untuk monetary items mengikuti PSAK 71.                                                         |
| Realized FX Gain/Loss         | Selisih kurs yang direalisasi saat closure event (penjualan, pencairan, jatuh tempo), dihitung dari perbedaan IDR equivalent settlement actual dengan IDR equivalent posisi terakhir.                                                                                              |
| JISDOR                        | Jakarta Interbank Spot Dollar Rate — kurs USD/IDR yang dipublikasikan Bank Indonesia setiap hari kerja jam 10:00 WIB sebagai referensi resmi.                                                                                                                                      |
| Chart of Accounts (CoA)       | Struktur kode akun general ledger yang dipakai sistem untuk posting jurnal. Disimpan di Master CoA (Bab 5.1.9), di-integrate dari sistem ERP/GL existing atau via import Excel.                                                                                                    |
| Master Mapping Jurnal         | Template event-jurnal dengan struktur header-detail. Header menyimpan event-level info (mis. PENEMPATAN); detail menyimpan baris D/K dengan filter klasifikasi/tipe instrumen yang resolusi-nya runtime saat event terpicu (Bab 5.1.10).                                           |
| Sumber Amount                 | Field instrumen yang dipakai untuk amount jurnal (mis. EAD\_IDR, BUNGA\_AKRUAL\_IDR, ECL\_AMOUNT\_IDR). Semua dalam IDR equivalent karena perhitungan EAD/ECL dilakukan dalam IDR (Bab 5.1.8 dan 7.1).                                                                             |
| Resolusi Runtime              | Algoritma sistem untuk memilih akun spesifik dan amount aktual saat event terpicu, berdasarkan atribut instrumen (klasifikasi PSAK 71, tipe, underlying type) yang dievaluasi terhadap filter di Master Mapping Jurnal.                                                            |
| LPS                           | Lembaga Penjamin Simpanan — penjamin simpanan nasabah perbankan di Indonesia.                                                                                                                                                                                                      |
| MTM                           | Mark-to-Market — penyesuaian nilai instrumen ke harga pasar terkini.                                                                                                                                                                                                               |
| NAB                           | Nilai Aktiva Bersih — harga per unit reksadana.                                                                                                                                                                                                                                    |
| FVOCI                         | Fair Value through Other Comprehensive Income — klasifikasi pengukuran PSAK 71.                                                                                                                                                                                                    |
| FVTPL                         | Fair Value through Profit or Loss — klasifikasi pengukuran PSAK 71.                                                                                                                                                                                                                |
| AC                            | Amortized Cost — klasifikasi pengukuran biaya perolehan diamortisasi.                                                                                                                                                                                                              |
| IBPA                          | Indonesia Bond Pricing Agency — penyedia harga referensi obligasi.                                                                                                                                                                                                                 |
| Pefindo                       | PT Pemeringkat Efek Indonesia — sumber rating dan PD historis.                                                                                                                                                                                                                     |
| CKPN                          | Cadangan Kerugian Penurunan Nilai — pos akuntansi untuk ECL.                                                                                                                                                                                                                       |
| SPPI                          | Solely Payments of Principal and Interest — uji karakteristik arus kas kontraktual aset keuangan sesuai PSAK 71. Aset lulus SPPI bila arus kasnya hanya berupa pembayaran pokok dan bunga atas saldo pokok yang masih terutang.                                                    |
| BM Test                       | Business Model Test — uji model bisnis pengelolaan aset keuangan sesuai PSAK 71. Tiga kategori: Held to Collect (HTC), Held to Collect & Sell (HTC\&S), dan Other (Trading/Manage on Fair Value Basis).                                                                            |
| HTC                           | Held to Collect — model bisnis menahan aset untuk memungut arus kas kontraktual hingga jatuh tempo.                                                                                                                                                                                |
| HTC\&S                        | Held to Collect and Sell — model bisnis menahan aset untuk memungut arus kas kontraktual dan menjual aset.                                                                                                                                                                         |
| EIR (Effective Interest Rate) | Suku Bunga Efektif — tingkat diskonto yang menyamakan nilai kini seluruh estimasi arus kas masa depan kontraktual sepanjang umur instrumen dengan carrying amount awal (harga beli + biaya transaksi kapitalisasi ± premium/diskonto). Sesuai PSAK 71 paragraf 5.4 dan Lampiran A. |
| Effective Interest Method     | Metode pengakuan pendapatan bunga sesuai PSAK 71. Pendapatan bunga = Carrying Amount × EIR. Untuk Stage 1 & 2 berbasis Gross Carrying; untuk Stage 3 (credit-impaired) berbasis Net Carrying (post-CKPN).                                                                          |
| Premium                       | Selisih harga beli \> nilai par/face value. Diamortisasi sebagai pengurang pendapatan bunga sepanjang umur instrumen via EIR.                                                                                                                                                      |
| Diskonto                      | Selisih harga beli \< nilai par/face value. Diamortisasi sebagai penambah pendapatan bunga sepanjang umur instrumen via EIR.                                                                                                                                                       |
| Carrying Amount (Bruto)       | Nilai tercatat aset keuangan = Pengakuan Awal − Pelunasan Pokok ± Amortisasi Kumulatif Premium/Diskonto. Untuk AC: setelah dikurangi CKPN. Untuk FVOCI utang: nilai wajar (gross), CKPN dimemo terpisah di OCI.                                                                    |
| Net Carrying Amount           | Carrying Bruto − Saldo CKPN. Dipakai sebagai basis perhitungan pendapatan bunga untuk instrumen Stage 3 sesuai PSAK 71 paragraf 5.4.1(b).                                                                                                                                          |
| Amortization Schedule         | Tabel proyeksi periodik (per kupon atau per akrual) yang menyimpan: Opening Carrying, Cash Inflow (kupon kontraktual), Pendapatan Bunga (Carrying × EIR), Amortisasi Premium/Diskonto, Closing Carrying. Diregenerate saat re-estimation EIR.                                      |
| Biaya Transaksi Kapitalisasi  | Biaya inkremental yang dapat diatribusikan langsung ke akuisisi aset keuangan (broker fee, subscription fee, due diligence biaya). Dikapitalisasi ke carrying awal untuk AC dan FVOCI; dibebankan langsung ke P\&L untuk FVTPL.                                                    |
| Re-estimation EIR             | Perhitungan ulang EIR akibat modifikasi material kontrak (perubahan kupon, tenor, prepayment) atau revisi estimasi cash flow. Sesuai PSAK 71 paragraf B5.4.5–B5.4.6: re-estimation tanpa derecognition memakai EIR original; modifikasi material → derecognition + EIR baru.       |

# 2\. Ruang Lingkup

## 2.1 In-Scope

1.  Master data: instrumen, penerbit/issuer, bank, manajer investasi, emiten saham, rating Pefindo, mapping LGD Basel, master portofolio.

2.  Modul SPPI Test & Business Model Test untuk klasifikasi PSAK 71 (AC/FVOCI/FVTPL), termasuk reklasifikasi prospektif.

3.  Tipe instrumen: Cash, Deposito, Obligasi, Saham, dan Reksadana (Pasar Uang, Pendapatan Tetap, Saham, Campuran).

4.  Modul transaksi: Penempatan, Mutasi/MTM, Renewal, Penjualan/Pencairan, Jatuh Tempo, Pendapatan Bunga/Hasil Investasi.

5.  Modul Media Upload pada setiap event instrumen dan untuk parameter Impact PD forward-looking.

6.  Modul Perhitungan ECL untuk Cash, Deposito, Obligasi, dan Reksadana (look-through).

7.  Modul jurnal otomatis terintegrasi ke General Ledger (GL) host.

8.  Pelaporan: posisi portofolio, MTM gain/loss, accrual bunga, mutasi instrumen, ECL summary & detail.

## 2.2 Out-of-Scope

  - Instrumen derivatif (swap, forward, option), repo/reverse repo, structured products.

  - Manajemen kas dan cash-flow forecasting (treasury planning).

  - Manajemen counterparty limit dan credit approval workflow.

  - Pelaporan regulasi spesifik OJK/BI (LBU/LBBU) yang memerlukan format dedicated.

## 2.3 Asumsi

  - Sistem akuntansi/General Ledger (GL) host telah tersedia dan menyediakan API untuk posting jurnal.

  - Data master Pefindo Rating dan PD tersedia melalui mekanisme upload manual berkala (bulanan/triwulanan).

  - Data harga referensi (IBPA untuk obligasi, NAB untuk reksadana) tersedia melalui upload harian.

  - Nilai pertanggungan LPS mengikuti regulasi terkini; saat ini diasumsikan sebesar Rp 2.000.000.000 per nasabah per bank.

  - Suku bunga Deposito mengikuti tingkat penjaminan LPS; deposito yang melebihi tingkat penjaminan tidak akan diterima sebagai eligible cash equivalent.

# 3\. Arsitektur Fungsional

## 3.1 Tipe Instrumen yang Didukung

| **No** | **Tipe Instrumen** | **Klasifikasi PSAK 71**                                         | **Sumber Harga**                                    | **Frekuensi MTM**               |
| ------ | ------------------ | --------------------------------------------------------------- | --------------------------------------------------- | ------------------------------- |
| 1      | Cash di Bank       | Amortized Cost                                                  | Rekening koran / statement                          | Harian (saldo aktual)           |
| 2      | Deposito           | Amortized Cost                                                  | Bilyet deposito (nominal)                           | Tidak ada (akrual bunga harian) |
| 3      | Obligasi           | FVOCI atau Amortized Cost                                       | Harga IBPA harian                                   | Harian                          |
| 4      | Saham              | FVTPL (default) atau FVOCI Election (irrevocable, no recycling) | Harga penutupan BEI harian                          | Harian                          |
| 5      | Reksadana          | FVTPL (default) atau FVOCI (kebijakan)                          | NAB harian (KSEI / MI); untuk ETF: harga BEI harian | Harian                          |

**Catatan klasifikasi:**

  - Cash dan Deposito → Amortized Cost (held to collect) → wajib ECL PSAK 71.

  - Obligasi → default FVOCI (held to collect & sell) → wajib ECL PSAK 71. Bila held-to-maturity → AC, ECL juga wajib.

  - Saham → SPPI FAIL otomatis → default FVTPL (P\&L). Entitas dapat memilih FVOCI Election (irrevocable, no recycling, dividen tetap di P\&L) untuk strategic equity holdings; pilihan ini wajib di-approve Komite Investasi.

  - Reksadana → default FVTPL. Entitas DAPAT mengklasifikasikan sebagai FVOCI atas keputusan kebijakan akuntansi (umumnya untuk portofolio jangka panjang dengan model bisnis HTC\&S, terutama RDN Pasar Uang dan RDN Pendapatan Tetap berbasis underlying SPPI). Klasifikasi FVOCI untuk reksadana wajib disetujui oleh Komite Investasi/CFO dan didokumentasikan dalam kebijakan akuntansi entitas.

  - Konsekuensi RDN FVOCI: MTM ke OCI (with recycling saat redemption); ECL look-through diakui di P\&L (kontra di OCI), bukan lagi sebatas risk-management view.

  - Konsekuensi RDN FVTPL: MTM ke P\&L; ECL look-through hanya risk-management view, tidak masuk laporan keuangan.

  - Look-through ECL hanya bermakna untuk underlying ber-PD: obligasi, deposito, cash. Underlying ekuitas (saham) tidak menghasilkan ECL — risiko ekuitas dimonitor melalui VaR/sensitivitas terpisah.

  - Saham individu (bukan reksadana) → tidak ada pengakuan ECL. Risiko dimonitor melalui parameter risiko pasar (VaR, beta, sensitivitas indeks) — di luar lingkup dokumen ini.

## 3.2 Modul Sistem (High-Level)

| **No** | **Modul**             | **Fungsi Utama**                                                                                                                 |
| ------ | --------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| 1      | Master Data           | CRUD instrumen, issuer, bank, MI, mapping rating-PD, mapping LGD Basel, parameter LPS, master portofolio.                        |
| 2      | SPPI & BM Test Engine | Pre-trade clearance: SPPI checklist Q1–Q10, BM Test, auto-derive klasifikasi PSAK 71 (AC/FVOCI/FVTPL), reklasifikasi prospektif. |
| 3      | Penempatan            | Pencatatan transaksi pembelian/penempatan instrumen baru, dengan upload bukti.                                                   |
| 4      | Mutasi / MTM          | Update harga harian, perhitungan unrealized gain/loss, jurnal MTM otomatis.                                                      |
| 5      | Renewal               | Perpanjangan deposito jatuh tempo (auto-rollover atau manual), rekalkulasi bunga.                                                |
| 6      | Penjualan / Pencairan | Disposal sebelum jatuh tempo dengan perhitungan realized gain/loss.                                                              |
| 7      | Jatuh Tempo (Closure) | Settlement saat instrumen jatuh tempo, rekonsiliasi bunga & pokok.                                                               |
| 8      | Pendapatan Investasi  | Akrual harian bunga deposito/obligasi, pengakuan distribusi reksadana, posting kupon.                                            |
| 9      | Media Upload          | Repositori dokumen terenkripsi yang menempel pada setiap event instrumen.                                                        |
| 10     | ECL Engine            | Perhitungan ECL dan ECL FL berbasis EAD × PD (3 skenario) × LGD × Impact PD.                                                     |
| 11     | Reporting             | Posisi portofolio, P\&L investasi, akrual bunga, mutasi, ECL summary & detail.                                                   |
| 12     | Jurnal & GL Interface | Generate jurnal otomatis dan kirim ke GL host via API/file batch.                                                                |

# 4\. Klasifikasi PSAK 71: SPPI Test & Business Model Test

PSAK 71 mensyaratkan setiap aset keuangan diklasifikasikan berdasarkan dua uji: (1) SPPI Test — uji karakteristik arus kas kontraktual; dan (2) Business Model Test — uji model bisnis pengelolaan aset. Hasil kedua uji menentukan kategori pengukuran: Amortized Cost (AC), Fair Value through OCI (FVOCI), atau Fair Value through Profit or Loss (FVTPL). Sistem menyediakan modul tersendiri untuk mendokumentasikan, menjalankan, mereview, dan menyimpan jejak audit kedua uji ini, lengkap dengan media upload bukti pendukung.

## 4.1 SPPI Test (Solely Payments of Principal and Interest)

### 4.1.1 Definisi & Konsep

Suatu aset keuangan lulus SPPI Test bila persyaratan kontraktualnya menghasilkan arus kas yang semata-mata merupakan pembayaran pokok dan bunga atas saldo pokok terutang. Yang dimaksud bunga di sini adalah imbalan atas time value of money, risiko kredit atas saldo pokok terutang dalam periode waktu tertentu, dan biaya/keuntungan dasar pemberian pinjaman lainnya (mis. profit margin).

### 4.1.2 Checklist SPPI per Tipe Instrumen

| **No** | **Pertanyaan SPPI**                                          | **Penjelasan**                                                                     | **Indikator FAIL bila…**                                                                         |
| ------ | ------------------------------------------------------------ | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| 1      | Apakah pokok dan bunga didefinisikan jelas?                  | Pokok = nilai wajar saat pengakuan awal; bunga = TVM + credit risk + biaya dasar.  | Pokok/bunga tidak jelas atau berbasis kinerja non-kredit.                                        |
| 2      | Apakah ada leverage / multiplier?                            | Faktor pengali yang memperbesar volatilitas arus kas (mis. ×3 LIBOR).              | Ada faktor leverage \> 1.                                                                        |
| 3      | Apakah ada link ke variabel non-kredit?                      | Mis. harga komoditas, indeks saham, kinerja entitas tertentu.                      | Ada linkage ke ekuitas/komoditas/forex non-functional.                                           |
| 4      | Apakah ada fitur konversi ekuitas?                           | Convertible bond yang dapat dikonversi ke saham penerbit.                          | Ada opsi konversi ke ekuitas.                                                                    |
| 5      | Apakah ada fitur subordination yang non-standar?             | Junior tranche dalam securitization yang menyerap kerugian secara disproporsional. | Subordinasi melebihi exposure-nya sendiri ("non-recourse" plus eksposur kredit pihak lain).      |
| 6      | Apakah ada fitur prepayment / extension?                     | Opsi pelunasan dini atau perpanjangan otomatis.                                    | Hanya FAIL bila kompensasi prepayment \> nilai wajar selisih bunga (de minimis test).            |
| 7      | Apakah suku bunga reset menggunakan tenor yang konsisten?    | Mis. floating rate berbasis JIBOR 3-bulan dengan reset 3-bulan.                    | Mismatch tenor (3-month rate reset 6-monthly tanpa adjustment) dan benefit kuantitatif material. |
| 8      | Apakah modifikasi suku bunga "de minimis"?                   | Penyesuaian kecil yang tidak material vs benchmark.                                | Modifikasi material yang menghasilkan arus kas berbeda signifikan dari benchmark.                |
| 9      | Apakah aset dijamin oleh aset tertentu (non-recourse)?       | Untuk non-recourse, lakukan look-through ke aset jaminan.                          | Underlying jaminan tidak lulus SPPI.                                                             |
| 10     | Apakah ada fitur kontingen yang dapat memodifikasi arus kas? | Mis. step-up rate, contingent payment.                                             | Kontinjensi tidak bersifat genuine atau memengaruhi SPPI material.                               |

### 4.1.3 Hasil SPPI per Tipe Instrumen (Default)

| **Tipe Instrumen**                 | **Default SPPI**  | **Justifikasi**                                                                                                                            |
| ---------------------------------- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Cash di Bank                       | **PASS**          | Saldo dapat dicairkan kapan saja pada nilai pari + bunga sederhana.                                                                        |
| Deposito                           | **PASS**          | Pokok terjaga, bunga tetap atau floating sederhana, tanpa fitur ekuitas/komoditas.                                                         |
| Obligasi Pemerintah (SUN/SBN)      | **PASS**          | Pokok + kupon tetap/floating standar; lulus SPPI.                                                                                          |
| Obligasi Korporasi Plain Vanilla   | **PASS**          | Pokok + kupon tetap/floating standar; perlu cek prepayment & step-up clause.                                                               |
| Sukuk Ijarah / Mudharabah          | **PASS (review)** | Lulus SPPI bila imbal hasil mencerminkan TVM + credit risk; perlu review skema bagi hasil.                                                 |
| Convertible Bond                   | **FAIL**          | Fitur konversi ekuitas → otomatis FVTPL.                                                                                                   |
| Saham (Ekuitas)                    | **FAIL**          | Instrumen ekuitas; arus kas tidak fixed (dividen diskresioner). Default FVTPL; tersedia opsi FVOCI Election irrevocable (tanpa recycling). |
| Reksadana (kecuali sub-tipe Saham) | FAIL              | Unit penyertaan = puttable equity instrument → FVTPL. Berlaku untuk sub-tipe PENDAPATAN\_TETAP, CAMPURAN, PASAR\_UANG, ETF.                |
| Reksadana sub-tipe Saham           | FAIL              | Unit penyertaan + underlying ekuitas → FVTPL.                                                                                              |

### 4.1.4 Field Modul SPPI Test

| **Field**                   | **Tipe Data** | **Wajib** | **Keterangan**                                                                                  |
| --------------------------- | ------------- | --------- | ----------------------------------------------------------------------------------------------- |
| SPPI Test ID                | VARCHAR(20)   | Ya        | Auto-generate (mis. SPPI-2026-00001).                                                           |
| Kode Instrumen              | FK            | Ya        | Reference master instrumen.                                                                     |
| Tanggal Test                | DATE          | Ya        | Tanggal pelaksanaan uji.                                                                        |
| Tipe Test                   | ENUM          | Ya        | INITIAL (saat penempatan) | PERIODIC (review tahunan) | TRIGGERED (modifikasi kontrak).         |
| Jawaban Checklist (Q1–Q10)  | JSON          | Ya        | Y/N + catatan per pertanyaan.                                                                   |
| Hasil SPPI                  | ENUM          | Ya        | PASS | FAIL — auto-calc dari checklist.                                                         |
| Catatan Penilaian           | TEXT          | Ya        | Justifikasi naratif terutama untuk fitur khusus (prepayment, step-up, dll).                     |
| Dokumen Bukti (Upload)      | FILE          | Ya        | Term sheet, prospektus, perjanjian, opini legal/akuntansi.                                      |
| Maker / Reviewer / Approver | FK            | Ya        | Three-eyes principle: Treasury Maker → Risk/Akuntansi Reviewer → Komite Investasi/CFO Approver. |
| Status                      | ENUM          | Ya        | DRAFT | PENDING REVIEW | PENDING APPROVAL | APPROVED | REJECTED.                                |

## 4.2 Business Model Test

### 4.2.1 Definisi & Konsep

Business Model Test menentukan tujuan entitas dalam mengelola portofolio aset keuangan. Penilaian dilakukan pada level portofolio (bukan per instrumen) berdasarkan bagaimana entitas secara aktual mengelola aset untuk menghasilkan arus kas. Tiga kategori model bisnis adalah:

1.  Held to Collect (HTC) — tujuan utama menahan aset untuk memungut arus kas kontraktual hingga jatuh tempo.

2.  Held to Collect and Sell (HTC\&S) — tujuan menahan untuk memungut arus kas DAN menjual aset; aktivitas menjual integral terhadap pencapaian tujuan.

3.  Other — model bisnis lain, terutama trading atau pengelolaan berbasis nilai wajar (mis. portofolio yang dievaluasi atas dasar fair value performance).

### 4.2.2 Indikator Penilaian Business Model

| **Indikator**                        | **HTC**                                    | **HTC\&S**                         | **Other**                        |
| ------------------------------------ | ------------------------------------------ | ---------------------------------- | -------------------------------- |
| Frekuensi Penjualan Historis         | Sangat jarang                              | Reguler & signifikan               | Sangat aktif                     |
| Volume Penjualan vs Total Portofolio | Insignifikan (\< 5%)                       | Signifikan (5%–50%)                | Dominan (\> 50%)                 |
| Alasan Penjualan                     | Hanya saat credit deterioration / dekat JT | Manajemen likuiditas / rebalancing | Profit-taking / spekulasi        |
| Dasar Evaluasi Kinerja Manager       | Imbal hasil bunga                          | Bunga + realisasi gain/loss        | Total return / fair value change |
| Skema Kompensasi Manager             | Berbasis bunga & holding                   | Berbasis total return moderat      | Berbasis fair value performance  |

### 4.2.3 Field Modul Business Model Test

| **Field**                          | **Tipe Data** | **Wajib**   | **Keterangan**                                                                             |
| ---------------------------------- | ------------- | ----------- | ------------------------------------------------------------------------------------------ |
| BM Test ID                         | VARCHAR(20)   | Ya          | Auto-generate (mis. BMT-2026-00001).                                                       |
| Portofolio ID                      | FK            | Ya          | Reference ke master portofolio (Treasury Liquidity, Investment, Trading, dll).             |
| Tanggal Penilaian                  | DATE          | Ya          | Tanggal penetapan/review BM.                                                               |
| Tujuan Pengelolaan                 | TEXT          | Ya          | Naratif: tujuan strategis pengelolaan portofolio.                                          |
| Indikator Penilaian                | JSON          | Ya          | Skor/jawaban per indikator (frekuensi jual, volume, alasan, evaluasi kinerja).             |
| Frekuensi Penjualan Historis (12M) | NUMERIC(8,4)  | Ya          | % volume penjualan vs total portofolio dalam 12 bulan terakhir.                            |
| Hasil BM Test                      | ENUM          | Ya          | HTC | HTC\&S | OTHER — auto-suggest dari indikator + override manual dengan justifikasi.   |
| Justifikasi Override               | TEXT          | Kondisional | Wajib bila hasil sistem di-override.                                                       |
| Dokumen Bukti (Upload)             | FILE          | Ya          | Investment policy, treasury policy, riwayat transaksi, KPI manager, memo Komite Investasi. |
| Approver                           | FK            | Ya          | Komite Investasi / ALCO / CFO.                                                             |
| Periode Berlaku                    | DATE RANGE    | Ya          | Dari–sampai; review minimum tahunan.                                                       |
| Status                             | ENUM          | Ya          | DRAFT | PENDING APPROVAL | APPROVED | EXPIRED.                                             |

### 4.2.4 Frekuensi Reassessment BM

1.  Reassessment WAJIB minimal sekali setahun (annual review).

2.  Reassessment juga TRIGGERED bila terjadi: (a) perubahan strategi treasury/investasi, (b) perubahan struktur organisasi yang signifikan, (c) volume penjualan aktual menyimpang material dari ekspektasi.

3.  Perubahan BM yang sah → reklasifikasi prospektif (bukan retrospektif) sesuai PSAK 71.

## 4.3 Matriks Klasifikasi PSAK 71 (SPPI × Business Model)

Sistem otomatis menentukan klasifikasi pengukuran berdasarkan kombinasi hasil SPPI dan BM Test:

| **Hasil SPPI** | **Held to Collect (HTC)** | **Held to Collect & Sell (HTC\&S)** | **Other (Trading/FV-managed)** |
| -------------- | ------------------------- | ----------------------------------- | ------------------------------ |
| **PASS**       | **Amortized Cost (AC)**   | **FVOCI (with recycling)**          | **FVTPL**                      |
| **FAIL**       | **FVTPL**                 | **FVTPL**                           | **FVTPL**                      |

**Konsekuensi klasifikasi:**

| **Klasifikasi** | **Pengukuran Setelah Pengakuan**                  | **MTM Pengakuan**                | **Pengakuan ECL**                            |
| --------------- | ------------------------------------------------- | -------------------------------- | -------------------------------------------- |
| **AC**          | Biaya perolehan diamortisasi (effective interest) | Tidak ada MTM jurnal             | **WAJIB ECL — kontra-aset**                  |
| **FVOCI**       | Nilai wajar                                       | OCI (Other Comprehensive Income) | **WAJIB ECL — kontra di OCI, beban di P\&L** |
| **FVTPL**       | Nilai wajar                                       | Laba/Rugi (P\&L)                 | **TIDAK ADA pengakuan ECL**                  |

### 4.3.1 Mapping Default Sistem (Plain Vanilla)

| **Tipe Instrumen**                                          | **SPPI** | **BM (Default)** | **Klasifikasi Otomatis**                       |
| ----------------------------------------------------------- | -------- | ---------------- | ---------------------------------------------- |
| Cash di Bank                                                | **PASS** | HTC              | **Amortized Cost**                             |
| Deposito                                                    | **PASS** | HTC              | **Amortized Cost**                             |
| Obligasi (held to maturity)                                 | **PASS** | HTC              | **Amortized Cost**                             |
| Obligasi (likuiditas)                                       | **PASS** | HTC\&S           | **FVOCI (default)**                            |
| Obligasi (trading)                                          | **PASS** | Other            | **FVTPL**                                      |
| Convertible Bond                                            | **FAIL** | Any              | **FVTPL (forced)**                             |
| Saham — Trading/Investment                                  | **FAIL** | Any              | **FVTPL (default)**                            |
| Saham — Strategic (FVOCI Election)                          | **FAIL** | N/A              | **FVOCI Election (irrevocable, no recycling)** |
| Reksadana PASAR\_UANG / PENDAPATAN\_TETAP / ETF (trading)   | FAIL     | Other            | FVTPL (default)                                |
| Reksadana PASAR\_UANG / PENDAPATAN\_TETAP / ETF (long-term) | FAIL     | HTC\&S           | FVOCI (kebijakan)                              |
| Reksadana SAHAM (trading)                                   | **FAIL** | Other            | **FVTPL (default)**                            |
| Reksadana SAHAM (long-term)                                 | **FAIL** | HTC\&S           | **FVOCI (kebijakan)**                          |
| Reksadana CAMPURAN (trading)                                | **FAIL** | Other            | **FVTPL (default)**                            |
| Reksadana CAMPURAN (long-term)                              | **FAIL** | HTC\&S           | **FVOCI (kebijakan)**                          |

**FVOCI Election untuk Saham (PSAK 71 paragraf 5.7.5):**

  - Bersifat irrevocable pada pengakuan awal saja.

  - Tidak ada recycling — gain/loss MTM masuk OCI dan tidak pernah direklasifikasi ke P\&L, bahkan saat dijual.

  - Dividen tetap diakui di P\&L.

  - Hanya untuk equity instruments yang TIDAK held-for-trading. Wajib disetujui Komite Investasi.

**Klasifikasi FVOCI untuk Reksadana (Kebijakan Akuntansi):**

  - Bukan FVOCI Election seperti saham — melainkan klasifikasi FVOCI penuh dengan recycling (sama seperti FVOCI debt).

  - Konsekuensi: MTM ke OCI; ECL look-through diakui di P\&L (kontra di OCI); saat redemption, akumulasi OCI direklasifikasi ke P\&L.

  - Persyaratan: model bisnis HTC\&S (long-term holding) + dokumentasi kebijakan akuntansi yang konsisten.

  - Untuk RDN Saham/Campuran sebagai FVOCI: ECL dihitung look-through HANYA pada komponen non-equity; komponen equity tidak menghasilkan ECL tetapi MTM-nya tetap masuk OCI.

  - Wajib disetujui Komite Investasi/CFO; reklasifikasi prospektif jika model bisnis berubah.

## 4.4 Workflow SPPI & BM Test

### 4.4.1 Flow Pre-Trade (Initial Classification)

Alur pre-trade clearance terdiri dari satu rangkaian SPPI Test → BM Test yang dijalankan oleh Treasury Maker secara berurutan, kemudian sistem auto-derive klasifikasi PSAK 71, baru setelah itu masuk ke jalur review dan approval.

| **Step** | **Aktor**                   | **Aksi**                                                                           | **Output / Trigger**                                                                |
| -------- | --------------------------- | ---------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| 1        | Treasury (Maker)            | Input rencana penempatan instrumen baru                                            | Permintaan pre-trade clearance dibuat                                               |
| 2        | Treasury (Maker)            | Upload term sheet / prospektus                                                     | Dokumen tersimpan, hash terverifikasi                                               |
| 3        | Treasury (Maker)            | Jalankan SPPI Test (jawab checklist Q1–Q10)                                        | Sistem auto-evaluasi SPPI: PASS atau FAIL                                           |
| 4        | Sistem                      | Decision: SPPI hasil PASS atau FAIL?                                               | Jika PASS → lanjut Step 5; jika FAIL → klasifikasi otomatis FVTPL, lompat ke Step 7 |
| 5        | Treasury (Maker)            | Jalankan BM Test (jawab pertanyaan Business Model + indikator pendukung)           | BM hasil: HTC | HTC\&S | Other                                                      |
| 6        | Sistem                      | Auto-derive klasifikasi PSAK 71 dari kombinasi SPPI × BM (matriks 4.3)             | Draft klasifikasi: AC | FVOCI | FVTPL                                               |
| 7        | Risk / Akuntansi (Reviewer) | Review hasil SPPI + BM Test + klasifikasi draft (validasi jawaban + naratif)       | Status: PENDING APPROVAL                                                            |
| 8        | Komite Investasi (Approver) | Approve klasifikasi PSAK 71 + tetapkan portofolio                                  | Klasifikasi ter-lock di Master Instrumen                                            |
| 9        | Treasury (Maker → Approver) | Input transaksi penempatan (Bab 5.2) → Approve → posting jurnal sesuai klasifikasi | Jurnal ter-posting ke GL                                                            |

**Activity Diagram — Pre-Trade SPPI & BM Test (Swimlane):**

![](media/image1.png)

*Gambar 4.4.1 — Activity diagram flow pre-trade dengan empat swimlane (Treasury Maker, Sistem, Risk/Akuntansi, Komite Investasi). SPPI Test dan BM Test dijalankan oleh Treasury Maker secara berurutan dalam satu rangkaian (step 3 → 5), kemudian sistem auto-derive klasifikasi (step 6) sebelum masuk jalur review (step 7) dan approval Komite (step 8). Decision node 'SPPI result?' memisahkan instrumen SPPI Pass (lanjut BM Test) dan SPPI Fail (otomatis FVTPL, skip BM Test, lompat ke auto-derive). Jalur merah putus-putus menggambarkan loop reject yang mengembalikan transaksi ke maker.*

### 4.4.1.a Urutan Input: Master Instrumen vs Transaksi Penempatan

Pertanyaan umum dalam implementasi: apakah harus input instrumen (master) dulu atau transaksi penempatan dulu? Jawabannya: MASTER INSTRUMEN dulu, baru transaksi penempatan. Klasifikasi PSAK 71 (AC/FVOCI/FVTPL) adalah atribut instrumen yang ditentukan saat initial recognition dan menentukan akun mana yang didebit di jurnal penempatan — tanpa klasifikasi yang sudah ter-lock, sistem tidak dapat membentuk jurnal yang benar.

**Empat alasan utama mengapa urutan ini wajib:**

1.  Klasifikasi PSAK 71 melekat pada instrumen, bukan transaksi. Setiap penempatan yang dilakukan terhadap instrumen yang sama akan mengikuti klasifikasi yang sudah ditetapkan.

2.  Master Instrumen reusable untuk multiple transaksi (top-up, partial sell). Bila urutan dibalik, setiap top-up akan butuh re-input data master yang sama — duplikasi data dan risiko inkonsistensi.

3.  Pre-trade clearance memberi keputusan apakah instrumen layak/tidak dari sisi klasifikasi (mis. obligasi convertible akan FAIL SPPI dan otomatis FVTPL — manajemen mungkin memutuskan tidak membeli).

4.  Compliance PSAK 71: standar mengatur bahwa klasifikasi ditentukan at initial recognition; eksekusi penempatan tanpa klasifikasi yang jelas merupakan financial reporting risk dan dapat menjadi temuan audit.

**Decision tree alur: instrumen baru vs top-up existing:**

![](media/image2.png)

*Gambar 4.4.1.a — Decision tree urutan input. Branch kiri (TIDAK / instrumen baru) menjalankan alur pre-trade lengkap A1–A6 dari Bab 4.4.1. Branch kanan (YA / top-up existing) langsung pakai klasifikasi yang sudah ter-lock; bila terjadi modifikasi material atau trigger reassessment (Bab 4.4.3), sistem mengarahkan kembali ke alur SPPI/BM Test (jalur merah putus-putus). Kedua branch bertemu di merge node sebelum input transaksi penempatan ke modul Bab 5.2.*

**Skenario top-up / repeat purchase pada instrumen existing:**

1.  Treasury Maker mencari instrumen di Master (by ISIN, kode, atau nama) — sistem menampilkan klasifikasi yang sudah ter-lock.

2.  Sistem cek apakah ada modifikasi material sejak terakhir SPPI/BM Test (mis. perubahan term obligasi, perubahan kebijakan investasi, ambang volume penjualan terlampaui).

3.  Bila TIDAK ada modifikasi material → langsung ke input transaksi penempatan, klasifikasi mengikuti yang sudah ter-lock.

4.  Bila ADA modifikasi material → trigger Reassessment (Bab 4.4.3) yang mengarahkan kembali ke SPPI/BM Test; bila hasilnya berbeda → trigger Reklasifikasi prospektif (Bab 4.5).

5.  Periodic Review tahunan (Bab 4.4.2) tetap berlaku untuk seluruh instrumen di Master, terlepas dari frekuensi top-up.

### 4.4.2 Flow Periodic Review

1.  Sistem mengirim notifikasi 30 hari sebelum SPPI/BM expired (review tahunan).

2.  Risk Officer membuka periodic review case di sistem.

3.  Sistem menarik data aktual: penjualan 12-bulan terakhir, modifikasi kontrak, perubahan kebijakan.

4.  Reviewer menetapkan: SPPI tetap | SPPI berubah | BM tetap | BM berubah.

5.  Bila berubah → trigger reklasifikasi (lihat 4.5).

### 4.4.3 Flow Triggered Reassessment

1.  Trigger otomatis bila: (a) modifikasi kontrak material di-input, (b) kebijakan investasi berubah dan di-upload, (c) volume penjualan portofolio melampaui threshold (mis. \> 5% untuk HTC, \> 50% untuk HTC\&S).

2.  Sistem membuat case Triggered Reassessment dan menahan transaksi baru pada portofolio terkait sampai review selesai.

## 4.5 Reklasifikasi & Konsekuensi Akuntansi

Reklasifikasi diterapkan secara prospektif sejak tanggal reklasifikasi (reclassification date) tanpa restate periode sebelumnya. Jurnal transisi dibentuk otomatis oleh sistem berdasarkan kombinasi from–to klasifikasi:

| **Dari** | **Ke** | **Treatment Nilai Tercatat**                                                                                | **Saldo OCI / P\&L**                     |
| -------- | ------ | ----------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| AC       | FVOCI  | Reukur ke Fair Value                                                                                        | Selisih → OCI                            |
| AC       | FVTPL  | Reukur ke Fair Value                                                                                        | Selisih → P\&L                           |
| FVOCI    | AC     | Fair Value menjadi carrying amount baru; akumulasi OCI di-reklasifikasi sebagai penyesuaian carrying amount | OCI cumulative dieliminasi terhadap aset |
| FVOCI    | FVTPL  | Tidak ada reukur (sudah FV); akumulasi OCI di-reklasifikasi ke P\&L                                         | OCI cumulative → P\&L                    |
| FVTPL    | AC     | Fair Value menjadi carrying amount baru; mulai amortisasi                                                   | Tidak ada saldo OCI                      |
| FVTPL    | FVOCI  | Tidak ada reukur; mulai akumulasi OCI sejak tanggal reklasifikasi                                           | Saldo OCI awal = 0                       |

### 4.5.1 Contoh Jurnal Reklasifikasi

**Contoh A: Reklasifikasi Obligasi dari FVOCI ke AC. Carrying (FV) Rp 5.050.000.000, akumulasi OCI gain Rp 50.000.000:**

| **Tanggal** | **Akun**                                                | **Debit**  | **Kredit** |
| ----------- | ------------------------------------------------------- | ---------- | ---------- |
| 01/01/2027  | OCI — Selisih Penilaian Wajar Obligasi                  | 50.000.000 |            |
| 01/01/2027  | Investasi Obligasi — AC (penyesuaian terhadap carrying) |            | 50.000.000 |

*Catatan: Carrying FVOCI Rp 5.050M dipindahkan ke akun AC dengan nilai sama; akumulasi OCI dieliminasi terhadap nilai tercatat sehingga nilai amortisasi awal = nilai wajar pada tanggal reklasifikasi.*

**Contoh B: Reklasifikasi Obligasi dari FVOCI ke FVTPL. Akumulasi OCI loss Rp 25.000.000:**

| **Tanggal** | **Akun**                               | **Debit**  | **Kredit** |
| ----------- | -------------------------------------- | ---------- | ---------- |
| 01/01/2027  | Kerugian Reklasifikasi dari OCI (P\&L) | 25.000.000 |            |
| 01/01/2027  | OCI — Selisih Penilaian Wajar Obligasi |            | 25.000.000 |

**Contoh C: Reklasifikasi Obligasi dari AC ke FVTPL. Carrying AC Rp 5.000.000.000, FV pada tgl reklasifikasi Rp 5.080.000.000:**

| **Tanggal** | **Akun**                        | **Debit**     | **Kredit**    |
| ----------- | ------------------------------- | ------------- | ------------- |
| 01/01/2027  | Investasi Obligasi — FVTPL      | 5.080.000.000 |               |
| 01/01/2027  | Investasi Obligasi — AC         |               | 5.000.000.000 |
| 01/01/2027  | Keuntungan Reklasifikasi (P\&L) |               | 80.000.000    |

## 4.6 Kontrol, Audit Trail & Media Upload

1.  SPPI Test dan BM Test mempunyai audit trail terpisah (ID, versi, history jawaban, timestamp, IP user).

2.  Setiap perubahan hasil SPPI atau BM otomatis membuat versi baru dan menyimpan versi lama (immutable history).

3.  Dokumen wajib upload (term sheet, prospektus, investment policy, opini legal/akuntansi) tersimpan dalam repositori terenkripsi.

4.  Auditor/regulator dapat menelusuri rasionalisasi klasifikasi setiap aset keuangan via single dashboard: Master Instrumen, SPPI History, BM History, Reklasifikasi History.

5.  Three-eyes principle: Maker (Treasury), Reviewer (Risk/Akuntansi), Approver (Komite Investasi/CFO) — ketiganya harus orang berbeda.

### 4.6.1 Daftar Wajib Upload Modul SPPI/BM

| **Event**              | **Wajib Upload**                                                                                                          |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| Initial SPPI Test      | Term sheet, prospektus/info memo, draft perjanjian, opini legal (jika fitur kompleks), opini akuntansi (jika diperlukan). |
| Initial BM Test        | Investment Policy / Treasury Policy yang berlaku, memo Komite Investasi penetapan portofolio.                             |
| Periodic Review SPPI   | Dokumen amandemen kontrak (jika ada), rekap modifikasi material.                                                          |
| Periodic Review BM     | Riwayat penjualan 12-bulan (export sistem), KPI manager portofolio, notulen Komite Investasi.                             |
| Triggered Reassessment | Memo trigger event (perubahan strategi/struktur), analisis dampak.                                                        |
| Reklasifikasi          | Memo persetujuan Komite, bukti tanggal efektif perubahan BM/SPPI.                                                         |

# 5\. SoW Detail per Modul

## 5.1 Master Data

### 5.1.1 Master Instrumen Investasi

| **Field**                    | **Tipe Data** | **Wajib**   | **Keterangan**                                                                                                                                                                                                                                                                                                                                                                                                                           |
| ---------------------------- | ------------- | ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kode Instrumen               | VARCHAR(20)   | Ya          | Unique key, format auto-generate (mis. CSH-0001, DEP-0001, OBL-0001, SHM-0001, RDN-0001). Sub-tipe disimpan terpisah di field Sub-Tipe Instrumen.                                                                                                                                                                                                                                                                                        |
| Tipe Instrumen               | ENUM          | Ya          | CASH | DEPOSITO | OBLIGASI | SAHAM | REKSADANA.                                                                                                                                                                                                                                                                                                                                                                                          |
| Sub-Tipe Instrumen           | ENUM          | Ya          | Bergantung Tipe — lihat tabel 5.1.1.a. Contoh: GIRO, TABUNGAN, BERJANGKA, ON\_CALL, NEGARA, KORPORASI, SUKUK\_NEGARA, SUKUK\_KORPORASI, LQ45, IDX30, NON\_LQ45, PAPAN\_PENGEMBANGAN, PENDAPATAN\_TETAP, CAMPURAN, PASAR\_UANG, ETF.                                                                                                                                                                                                      |
| Nama Instrumen               | VARCHAR(120)  | Ya          | Nama deskriptif (mis. "Deposito Bank Mandiri 6M", "Obligasi PT XYZ Seri A 2024", "Sukuk Negara SR-018", "Saham BBCA", "RDN Schroder Dana Prestasi Plus", "ETF XIIT").                                                                                                                                                                                                                                                                    |
| ISIN / Kode Efek             | VARCHAR(20)   | Tidak       | Untuk obligasi, saham, dan reksadana.                                                                                                                                                                                                                                                                                                                                                                                                    |
| Counterparty/Issuer ID       | FK            | Ya          | Reference ke master counterparty. Untuk Cash/Deposito = bank; untuk Obligasi = issuer/penerbit; untuk Saham = emiten; untuk Reksadana = bank kustodian.                                                                                                                                                                                                                                                                                  |
| Manajer Investasi (MI) ID    | FK            | Kondisional | Wajib untuk seluruh tipe Reksadana (REKSADANA sub-tipe PASAR\_UANG, REKSADANA sub-tipe PENDAPATAN\_TETAP, REKSADANA sub-tipe SAHAM, REKSADANA sub-tipe CAMPURAN); N/A untuk Cash/Deposito/Obligasi/Saham. Reference ke master Manajer Investasi (lihat Bab 5.1.2 — counterparty dengan tipe MANAJER\_INVESTASI). Mis. PT Schroder Investment Management Indonesia, PT Bahana TCW Investment Management, PT BNP Paribas Asset Management. |
| Bank Kustodian ID            | FK            | Kondisional | Wajib untuk seluruh tipe Reksadana; reference ke master counterparty (bank yang berfungsi sebagai kustodian dana — mis. Standard Chartered, Citibank, BNI Custodian).                                                                                                                                                                                                                                                                    |
| FVOCI Election (Equity)      | BOOLEAN       | Kondisional | Hanya untuk saham. Bila Y → klasifikasi FVOCI Election (irrevocable, no recycling). Default N → FVTPL. Wajib approval Komite Investasi.                                                                                                                                                                                                                                                                                                  |
| SPPI Test Result             | ENUM          | Ya          | PASS | FAIL — hasil uji SPPI lihat Bab 4.1; FAIL otomatis FVTPL (kecuali FVOCI Election untuk saham).                                                                                                                                                                                                                                                                                                                                    |
| BM Test Category             | ENUM          | Ya          | HTC | HTC\&S | OTHER — hasil uji Business Model lihat Bab 4.2 (N/A untuk saham FVOCI Election).                                                                                                                                                                                                                                                                                                                                          |
| SPPI/BM Last Review Date     | DATE          | Ya          | Tanggal review terakhir; wajib diperbarui minimal tahunan atau saat ada modifikasi material.                                                                                                                                                                                                                                                                                                                                             |
| Klasifikasi PSAK 71          | ENUM          | Ya          | AC | FVOCI | FVOCI\_ELECTION | FVTPL — auto-derived dari kombinasi SPPI + BM + FVOCI Election (lihat 4.3).                                                                                                                                                                                                                                                                                                                               |
| Mata Uang                    | CHAR(3)       | Ya          | ISO 4217 (mis. IDR, USD, SGD, EUR, JPY, AUD, CNY, MYR). Default IDR. Setiap instrumen non-IDR akan dikonversi ke IDR equivalent menggunakan kurs dari Master Mata Uang & Kurs (Bab 5.1.8) untuk semua perhitungan EAD, ECL, dan reporting.                                                                                                                                                                                               |
| Nominal/Face Value           | NUMERIC(20,2) | Ya          | Untuk deposito = pokok; obligasi = nominal; saham = jumlah lot × harga awal × 100 lembar; reksadana = jumlah unit × NAB awal.                                                                                                                                                                                                                                                                                                            |
| Jumlah Lot/Lembar            | NUMERIC(18,0) | Kondisional | Khusus saham (1 lot = 100 lembar).                                                                                                                                                                                                                                                                                                                                                                                                       |
| Tanggal Penempatan           | DATE          | Ya          | Trade date.                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Tanggal Jatuh Tempo          | DATE          | Kondisional | Wajib untuk deposito & obligasi; N/A untuk saham/reksadana.                                                                                                                                                                                                                                                                                                                                                                              |
| Suku Bunga / Kupon           | NUMERIC(8,4)  | Kondisional | % per annum, 4 desimal.                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Frekuensi Bunga              | ENUM          | Kondisional | BULANAN | TRIWULANAN | SEMESTERAN | TAHUNAN | DI MUKA | JATUH TEMPO.                                                                                                                                                                                                                                                                                                                                                                     |
| Auto Renewal Flag            | BOOLEAN       | Kondisional | Untuk deposito; jika Y → auto-rollover saat jatuh tempo.                                                                                                                                                                                                                                                                                                                                                                                 |
| Status                       | ENUM          | Ya          | AKTIF | DICAIRKAN | JATUH TEMPO | DIJUAL.                                                                                                                                                                                                                                                                                                                                                                                                |
| EIR Awal                     | NUMERIC(12,8) | Kondisional | Wajib untuk klasifikasi AC dan FVOCI utang. Suku Bunga Efektif yang dihitung saat pengakuan awal via IRR solver (lihat Bab 5.12). 8 desimal internal, 4 desimal di laporan. N/A untuk FVTPL/Saham/Reksadana.                                                                                                                                                                                                                             |
| Tanggal EIR Computed         | DATE          | Kondisional | Tanggal sistem menghitung EIR awal — umumnya = Tanggal Penempatan. Diperbarui saat re-estimation EIR (modifikasi material).                                                                                                                                                                                                                                                                                                              |
| Premium / Diskonto Awal      | NUMERIC(20,2) | Kondisional | Selisih Harga Beli − Nilai Par. Positif = premium (harga \> par); negatif = diskonto (harga \< par). Untuk obligasi AC/FVOCI saat penempatan.                                                                                                                                                                                                                                                                                            |
| Biaya Transaksi Kapitalisasi | NUMERIC(20,2) | Kondisional | Biaya transaksi yang dikapitalisasi ke Carrying Amount awal untuk AC/FVOCI. Untuk FVTPL biaya transaksi langsung ke P\&L (field tetap diisi untuk audit trail tetapi tidak dikapitalisasi).                                                                                                                                                                                                                                              |
| EIR Method Flag              | BOOLEAN       | Kondisional | Default Y untuk AC dan FVOCI utang. Bila Y → akrual bunga & amortisasi memakai EIR (Bab 5.12); bila N → simple interest method (hanya untuk Cash giro/tabungan dengan rate variabel harian).                                                                                                                                                                                                                                             |
| Amortization Frequency       | ENUM          | Kondisional | HARIAN | BULANAN | TRIWULANAN | SEMESTERAN | TAHUNAN. Frekuensi posting amortisasi premium/diskonto. Default mengikuti Frekuensi Bunga; HARIAN dapat dipilih untuk granularitas penuh.                                                                                                                                                                                                                                                   |

### 5.1.1.a Mapping Sub-Tipe Instrumen

Sub-Tipe digunakan untuk granularitas analisis, mapping rate pajak, mapping LGD spesifik, dan pelaporan agregat.

| **Tipe**      | **Sub-Tipe (ENUM Value)** | **Karakteristik & Contoh**                                                                                                                |
| ------------- | ------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| **CASH**      | GIRO                      | Saldo giro bank (dapat ditarik kapan saja). Sumber bunga: floating rate per kebijakan bank.                                               |
| **CASH**      | TABUNGAN                  | Saldo tabungan bank. Sumber bunga: floating rate per kebijakan bank.                                                                      |
| **DEPOSITO**  | BERJANGKA                 | Tenor tetap (1, 3, 6, 12 bulan); penarikan sebelum jatuh tempo dikenakan penalti. PPh 4(2) Final 20%.                                     |
| **DEPOSITO**  | ON\_CALL                  | Tenor harian-mingguan; bunga lebih rendah; dapat ditarik dengan notice (mis. 1×24 jam atau 7 hari). PPh 4(2) Final 20%.                   |
| **OBLIGASI**  | NEGARA                    | SUN, SBN, ORI, SR, INDON. LGD Sovereign 0,4500; PPh kupon 10% (atau sesuai tarif).                                                        |
| **OBLIGASI**  | KORPORASI                 | Obligasi korporasi konvensional (tercatat/tidak tercatat). LGD Senior Unsecured Korporasi 0,4500; PPh kupon 10%.                          |
| **OBLIGASI**  | SUKUK\_NEGARA             | Sukuk Negara (SR, ST, PBS, IFR). Imbal hasil syariah; LGD Sovereign 0,4500; PPh sesuai tarif.                                             |
| **OBLIGASI**  | SUKUK\_KORPORASI          | Sukuk Korporasi (Ijarah, Mudharabah, Wakalah). Perlu review SPPI untuk skema bagi hasil; LGD Senior Unsecured Korporasi 0,4500.           |
| **SAHAM**     | LQ45                      | Saham anggota indeks LQ45 (likuiditas tinggi).                                                                                            |
| **SAHAM**     | IDX30                     | Saham anggota indeks IDX30.                                                                                                               |
| **SAHAM**     | NON\_LQ45                 | Saham di luar indeks LQ45/IDX30.                                                                                                          |
| **SAHAM**     | PAPAN\_PENGEMBANGAN       | Saham di papan pengembangan/akselerasi BEI.                                                                                               |
| **REKSADANA** | PENDAPATAN\_TETAP         | Underlying mayoritas obligasi/sukuk (≥ 80%). Look-through ECL ke underlying.                                                              |
| **REKSADANA** | CAMPURAN                  | Bauran obligasi/saham/cash, tiap kelas aset ≤ 79%. Look-through ECL HANYA pada komponen non-equity.                                       |
| **REKSADANA** | SAHAM                     | Underlying mayoritas saham (≥ 80%). TIDAK ada ECL look-through (underlying ekuitas).                                                      |
| **REKSADANA** | PASAR\_UANG               | Underlying: deposito bank, sertifikat deposito, SBI, instrumen pasar uang ≤ 1 tahun. Look-through ECL penuh ke underlying.                |
| **REKSADANA** | ETF                       | Exchange-Traded Fund — diperdagangkan di BEI seperti saham. Harga acuan: closing price BEI; ECL look-through sesuai komposisi underlying. |

*Catatan: Sub-Tipe dapat diperluas oleh administrator melalui modul master tanpa pengembangan ulang sistem.*

### 5.1.2 Master Counterparty (Bank / Issuer / Manajer Investasi)

| **Field**             | **Tipe Data** | **Wajib**   | **Keterangan**                                                                                          |
| --------------------- | ------------- | ----------- | ------------------------------------------------------------------------------------------------------- |
| Counterparty ID       | VARCHAR(20)   | Ya          | Unique key.                                                                                             |
| Nama                  | VARCHAR(120)  | Ya          | Nama legal entitas.                                                                                     |
| Tipe                  | ENUM          | Ya          | BANK | BANK\_KUSTODIAN | KORPORASI | PEMERINTAH | MANAJER\_INVESTASI | EMITEN\_SAHAM.                   |
| Rating Pefindo        | VARCHAR(8)    | Kondisional | idAAA, idAA+, idAA, …, idD. Wajib untuk Bank, Korporasi, MI.                                            |
| Tanggal Rating        | DATE          | Kondisional | Tanggal terbit rating terbaru.                                                                          |
| Tipe Eksposur Basel   | ENUM          | Ya          | Senior Secured | Senior Unsecured | Subordinated | Sovereign — untuk lookup LGD.                        |
| Eligible LPS Flag     | BOOLEAN       | Ya          | Untuk Bank → otomatis Y; lainnya N.                                                                     |
| NPWP / Registrasi     | VARCHAR(30)   | Tidak       | Identitas pajak.                                                                                        |
| Nomor Izin OJK (MI)   | VARCHAR(40)   | Kondisional | Wajib untuk Tipe = MANAJER\_INVESTASI; nomor SIUP-MI dari OJK (mis. KEP-XX/PM/2024).                    |
| Tanggal Izin OJK (MI) | DATE          | Kondisional | Wajib untuk MI; tanggal terbit izin.                                                                    |
| AUM Terakhir (MI)     | NUMERIC(20,2) | Kondisional | Asset Under Management terakhir untuk MI (Rp); update minimal triwulanan dari publikasi MI.             |
| Tanggal AUM Terakhir  | DATE          | Kondisional | Tanggal snapshot AUM.                                                                                   |
| Kategori MI           | ENUM          | Kondisional | BUMN | SWASTA\_NASIONAL | SWASTA\_ASING | JOINT\_VENTURE — untuk MI, granularitas analisis konsentrasi. |

### 5.1.2.a Counterparty Rating History

Setiap perubahan rating Pefindo pada counterparty tercatat dalam tabel history terpisah. History ini menjadi sumber audit trail dan input untuk: (a) deteksi Significant Increase in Credit Risk (SICR) → trigger Stage 2, (b) deteksi default → trigger Stage 3, (c) analisis tren risiko kredit, (d) re-perhitungan ECL retroaktif jika diperlukan.

**Field Rating History:**

| **Field**                | **Tipe Data** | **Wajib** | **Keterangan**                                                                                                      |
| ------------------------ | ------------- | --------- | ------------------------------------------------------------------------------------------------------------------- |
| Rating History ID        | VARCHAR(20)   | Ya        | Auto-generate (mis. RTH-2026-00001).                                                                                |
| Counterparty ID          | FK            | Ya        | Reference ke master counterparty.                                                                                   |
| Tanggal Berlaku          | DATE          | Ya        | Effective date rating.                                                                                              |
| Tanggal Berakhir         | DATE          | Tidak     | Diisi otomatis saat rating berikut diinput.                                                                         |
| Rating Pefindo           | VARCHAR(8)    | Ya        | idAAA … idD.                                                                                                        |
| Rating Outlook           | ENUM          | Tidak     | POSITIVE | STABLE | NEGATIVE | DEVELOPING.                                                                          |
| Sumber Rating            | ENUM          | Ya        | PEFINDO\_REGULAR | PEFINDO\_REVIEW | LEMBAGA\_LAIN (mis. Fitch, Moody's, S\&P) — sumber dari upload.                |
| Tanggal Publikasi Rating | DATE          | Ya        | Tanggal terbit rating dari lembaga pemeringkat.                                                                     |
| Action Type              | ENUM          | Ya        | INITIAL | UPGRADE | DOWNGRADE | AFFIRMED | WITHDRAWN.                                                               |
| Notch Change             | INT           | Ya        | Jumlah notch perubahan vs rating sebelumnya (positif = upgrade, negatif = downgrade, 0 = affirmed).                 |
| SICR Triggered           | BOOLEAN       | Ya        | Auto-calc: TRUE jika downgrade ≥ 2 notch dari saat origination, atau berpindah ke non-investment grade (lihat 8.5). |
| Default Triggered        | BOOLEAN       | Ya        | Auto-calc: TRUE jika rating = idD atau ada peristiwa default lain (gagal bayar \> 90 hari).                         |
| Dokumen Bukti (Upload)   | FILE          | Ya        | Press release Pefindo, rating action report.                                                                        |
| Maker / Approver         | FK            | Ya        | Risk Officer (Maker), Risk Manager (Approver).                                                                      |

**Aturan integritas:**

1.  Setiap counterparty harus memiliki minimal satu Rating History (saat onboarding = INITIAL).

2.  Hanya satu Rating History yang aktif (Tanggal Berakhir = NULL) per counterparty pada satu waktu.

3.  Saat rating baru diinput, sistem otomatis mengisi Tanggal Berakhir untuk record yang masih aktif sebelumnya = (Tanggal Berlaku rating baru − 1 hari).

4.  Jika SICR Triggered = TRUE atau Default Triggered = TRUE, sistem otomatis mengubah staging instrumen terkait (Stage 1 → 2 atau Stage 2 → 3) — lihat Bab 8.5.

5.  Rating History tidak dapat dihapus; hanya bisa di-correct via record baru (audit trail tetap utuh).

### 5.1.3 Master Mapping PD Normal (Sumber: Pefindo)

Tabel berikut adalah PD Normal (PD Base) per rating yang bersumber dari Pefindo Default Study, diperbarui minimal triwulanan melalui media upload (CSV/XLSX). PD Normal merepresentasikan probabilitas default 12-bulan dalam kondisi makroekonomi netral/normal.

**Penting: Sistem TIDAK menyimpan PD Optimistic dan PD Pessimistic langsung dari Pefindo. PD Good (Optimistic) dan PD Bad (Pessimistic) DIDERIVASI dari PD Normal menggunakan Impact MEV to PD (lihat Bab 5.8.3 dan 7.4).**

| **Rating Pefindo** | **PD Normal (12-Month)** | **Keterangan**                                       |
| ------------------ | ------------------------ | ---------------------------------------------------- |
| idAAA              | 0,0000                   | Highest grade — no default 2007-2025                 |
| idAA               | 0,0000                   | No default Y1; default at Y5 (0,20%)                 |
| idA                | 0,0031                   | Investment grade — 0,31% historical Y1               |
| idBBB              | 0,0567                   | Lower investment grade — 5,67% Y1                    |
| idBB               | 0,5008                   | Speculative — 50,08% Y1 (high historical)            |
| idB                | 0,0000                   | Limited Pefindo monitoring — override internal model |
| idCCC              | 0,0939                   | Substantial risk — 9,39% Y1                          |
| idD                | 1,0000                   | Default actual                                       |

*Catatan: Angka di atas di-seed dari Pefindo Annual Default Study 2007-2025 (Appendix 2, Survival Pool Cumulative Average Default Rate based on Debt Instrument). Update berkala dilakukan saat Pefindo publish version terbaru. Tabel ini merepresentasikan PD 12-bulan (1-year PD) Normal yang digunakan untuk Stage 1.*

### 5.1.3.a Master Mapping Lifetime PD Normal (Stage 2 & 3)

Lifetime PD digunakan untuk perhitungan ECL pada instrumen di Stage 2 dan Stage 3 (lihat Bab 8.5). Lifetime PD merupakan akumulasi probabilitas default sepanjang sisa umur kontraktual instrumen, dengan formula umum: Lifetime PD = 1 − ∏(1 − PD\_marginal\_t) untuk t = 1 sampai T (tenor sisa).

Pendekatan praktis: gunakan tabel cumulative PD per rating dari Pefindo Default Study (multi-year), atau hitung dari PD 12-bulan dengan asumsi konstan:

| **Rating** | **PD 1-Yr (Normal)** | **PD 3-Yr** | **PD 5-Yr** | **PD 7-Yr** | **PD 10-Yr** |
| ---------- | -------------------- | ----------- | ----------- | ----------- | ------------ |
| idAAA      | 0,0000               | 0,0000      | 0,0000      | 0,0000      | 0,0000       |
| idAA       | 0,0000               | 0,0000      | 0,0020      | 0,0020      | 0,0020       |
| idA        | 0,0031               | 0,0290      | 0,0549      | 0,0549      | 0,0549       |
| idBBB      | 0,0567               | 0,1734      | 0,1866      | 0,1934      | 0,1934       |
| idBB       | 0,5008               | 0,5683      | 0,5683      | 0,5683      | 0,5683       |
| idB        | 0,0000               | 0,0000      | 0,0000      | 0,0000      | 0,0000       |
| idCCC      | 0,0939               | 0,6633      | 0,6633      | 0,6633      | 0,6633       |
| idD        | 1,0000               | 1,0000      | 1,0000      | 1,0000      | 1,0000       |

*Sumber: Hasil derivation dari PD Normal 1-Year dengan asumsi konstan (Lifetime PD\_T = 1 − (1 − PD\_1yr)^T). Dalam implementasi riil, gunakan tabel cumulative PD multi-year dari Pefindo Default Study atau internal model. Tabel ini juga adalah PD Normal — derivasi PD Good/Bad mengikuti mekanisme yang sama (dari Impact MEV to PD).*

*Sistem akan melakukan interpolasi linear untuk tenor non-standar (misal 4 tahun → interpolasi antara 3-Year dan 5-Year).*

### 5.1.4 Master Mapping LGD (Sumber: Basel)

| **Tipe Eksposur**            | **Karakteristik**                               | **LGD** |
| ---------------------------- | ----------------------------------------------- | ------- |
| Sovereign / Pemerintah       | SUN, SBN, Obligasi Pemerintah                   | 0,4500  |
| Senior Secured               | Obligasi dengan jaminan aktiva spesifik         | 0,2500  |
| Senior Unsecured (Bank)      | Cash di bank, deposito (di luar penjaminan LPS) | 0,4500  |
| Senior Unsecured (Korporasi) | Obligasi korporasi tanpa jaminan                | 0,4500  |
| Subordinated                 | Obligasi subordinasi, sukuk subordinasi         | 0,7500  |

*Sumber: Basel III Foundation IRB Approach. Diperbarui melalui upload bila terjadi perubahan kebijakan.*

### 5.1.5 Master Bobot Skenario PD

| **Skenario**      | **Bobot Default** | **Catatan**                                                                    |
| ----------------- | ----------------- | ------------------------------------------------------------------------------ |
| Good (Optimistic) | 0,2500            | PD Good = PD Normal × Impact MEV (Good); bobot disesuaikan oleh Komite Risiko. |
| Normal (Base)     | 0,5000            | PD Normal langsung dari Pefindo; skenario paling representatif.                |
| Bad (Pessimistic) | 0,2500            | PD Bad = PD Normal × Impact MEV (Bad); bobot disesuaikan oleh Komite Risiko.   |
| **TOTAL**         | **1,0000**        |                                                                                |

### 5.1.7 Master Periode Buku

Master Periode Buku mendefinisikan periode akuntansi yang dipakai sistem untuk: (a) stamp periode\_id pada setiap transaksi (penempatan, MTM, akrual, ECL, jurnal), (b) kontrol posting (transaksi tidak bisa di-input/edit pada periode yang sudah CLOSED), (c) cut-off untuk laporan komparatif (period-over-period), dan (d) basis untuk closing entries (akhir bulan/triwulan/tahun).

*Hierarki periode: Tahun Buku → Triwulan → Bulan → Hari kerja akuntansi.*

**Field Master Periode Buku:**

| **Field**           | **Tipe Data** | **Wajib**   | **Keterangan**                                                                                                                        |
| ------------------- | ------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Periode ID          | VARCHAR(20)   | Ya          | Auto-generate (mis. PRD-2026-01, PRD-2026-Q1, PRD-2026).                                                                              |
| Tipe Periode        | ENUM          | Ya          | BULANAN | TRIWULANAN | TAHUNAN.                                                                                                       |
| Tahun Buku          | INT(4)        | Ya          | Tahun fiskal (mis. 2026).                                                                                                             |
| Bulan               | INT(2)        | Kondisional | Wajib untuk tipe BULANAN (1–12).                                                                                                      |
| Triwulan            | INT(1)        | Kondisional | Wajib untuk tipe TRIWULANAN (1–4).                                                                                                    |
| Tanggal Mulai       | DATE          | Ya          | Tanggal awal periode (inklusif).                                                                                                      |
| Tanggal Akhir       | DATE          | Ya          | Tanggal akhir periode (inklusif).                                                                                                     |
| Status Periode      | ENUM          | Ya          | OPEN | SOFT\_CLOSED | CLOSED. OPEN = boleh input/edit; SOFT\_CLOSED = boleh adjustment terbatas oleh Akuntansi; CLOSED = hard-locked. |
| Tanggal Soft Close  | DATETIME      | Kondisional | Diisi otomatis saat status berubah ke SOFT\_CLOSED.                                                                                   |
| Tanggal Hard Close  | DATETIME      | Kondisional | Diisi otomatis saat status berubah ke CLOSED; setelah ini transaksi tidak bisa diubah dengan cara apapun.                             |
| User Closer         | FK            | Kondisional | Wajib saat closing; user yang menutup periode (Akuntansi/Finance Controller).                                                         |
| User Approver Close | FK            | Kondisional | Wajib untuk hard close; user yang approve closing (CFO atau Finance Manager).                                                         |
| Catatan Closing     | TEXT          | Tidak       | Catatan dari closer (mis. "Periode Jan 2026 ditutup tanggal 5 Feb setelah rekonsiliasi GL").                                          |
| Reopened Flag       | BOOLEAN       | Ya          | Default N. Y bila periode pernah di-reopen setelah CLOSED (audit trail).                                                              |
| Reopened Reason     | TEXT          | Kondisional | Wajib bila Reopened Flag = Y; alasan reopen + dokumen pendukung.                                                                      |

**Aturan integritas periode:**

1.  Setiap tanggal kalender harus mapped ke tepat 1 periode bulanan, 1 periode triwulanan, dan 1 periode tahunan (overlap diperbolehkan antara hierarki, tetapi tidak antar periode pada level yang sama).

2.  Sistem otomatis generate 12 periode bulanan, 4 periode triwulanan, dan 1 periode tahunan setiap awal Tahun Buku baru (wizard inisialisasi).

3.  Tahun Buku default mengikuti tahun kalender (1 Jan – 31 Des). Bila perusahaan menggunakan tahun fiskal non-kalender, dapat dikonfigurasi via parameter sistem.

4.  Status periode memiliki transisi: OPEN → SOFT\_CLOSED → CLOSED. Transisi mundur (CLOSED → OPEN, SOFT\_CLOSED → OPEN) hanya dapat dilakukan via fitur Reopen yang dijaga akses CFO + audit trail.

5.  Hard close periode tahunan (Tahun Buku) hanya bisa dilakukan setelah seluruh 12 periode bulanan dan 4 periode triwulanan dalam tahun tersebut sudah CLOSED.

**Status periode dan dampaknya pada modul transaksi:**

| **Status**       | **Dampak pada Modul Transaksi**                                                                                                                                                                                                                                                                                                           |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **OPEN**         | Semua user dapat input transaksi baru, edit transaksi yang masih PENDING, dan approve. Job sistem (akrual, MTM, ECL) berjalan normal.                                                                                                                                                                                                     |
| **SOFT\_CLOSED** | Treasury TIDAK BISA input transaksi baru atau edit. Hanya Akuntansi (Finance Controller) yang dapat melakukan adjustment journal entry untuk koreksi (mis. reklasifikasi, koreksi ECL) dengan trail Maker-Approver. Job ECL akhir bulan masih bisa di-rerun bila ada parameter update.                                                    |
| **CLOSED**       | Periode hard-locked. Tidak ada user yang dapat input atau edit apapun pada periode ini. Bila terdeteksi error setelah closing, koreksi harus dilakukan via journal entry pada periode terbuka berikutnya (mengikuti prinsip prior-period adjustment PSAK 25). Reopen periode hanya bisa dilakukan via fitur khusus yang dijaga akses CFO. |

### 5.1.8 Master Mata Uang & Kurs

Sistem menyimpan kurs konversi (FX rate) per mata uang per tanggal, dengan referensi periode buku. Semua perhitungan internal — termasuk EAD, ECL, MTM, jurnal akuntansi, dan reporting — dilakukan dalam IDR equivalent. Mata uang asli (foreign currency) tetap disimpan untuk audit trail dan disclosure, namun semua agregasi laporan menggunakan IDR equivalent.

**Prinsip konversi:**

1.  Semua field EAD, MTM, dan ECL disimpan dalam DUA bentuk: nilai mata uang asli dan IDR equivalent.

2.  Konversi ke IDR menggunakan kurs yang sesuai dengan event akuntansi: trade date untuk penempatan, daily closing rate untuk MTM harian, period-end rate untuk ECL akhir bulan.

3.  Tipe kurs yang dipakai: kurs tengah Bank Indonesia (JISDOR untuk USD, kurs tengah BI untuk mata uang lainnya). Kurs alternatif (mis. kurs broker) hanya untuk dokumentasi, bukan untuk pembukuan.

4.  Periode buku CLOSED → kurs yang disimpan ter-lock dan tidak dapat diubah meskipun ada koreksi kurs publikasi BI di kemudian hari.

**Field Master Mata Uang:**

| **Field**           | **Tipe Data** | **Wajib** | **Keterangan**                                                               |
| ------------------- | ------------- | --------- | ---------------------------------------------------------------------------- |
| Kode Mata Uang      | CHAR(3)       | Ya        | ISO 4217 (USD, SGD, EUR, JPY, AUD, CNY, MYR, GBP, dll).                      |
| Nama Mata Uang      | VARCHAR(60)   | Ya        | Mis. United States Dollar, Singapore Dollar.                                 |
| Simbol              | VARCHAR(5)    | Tidak     | Mis. $, S$, €, ¥.                                                            |
| Sumber Kurs Default | ENUM          | Ya        | BI\_JISDOR | BI\_KURS\_TENGAH | INTERNAL — referensi sumber publikasi resmi. |
| Frekuensi Update    | ENUM          | Ya        | HARIAN | INTRA\_DAY (untuk USD) | BULANAN (untuk mata uang non-aktif).       |
| Aktif Flag          | BOOLEAN       | Ya        | Y bila digunakan untuk transaksi aktif; N bila hanya untuk legacy.           |
| Tanggal Mulai Aktif | DATE          | Ya        | Tanggal mata uang ditambahkan ke sistem.                                     |

**Field Tabel Kurs Harian (FX Rate History):**

| **Field**              | **Tipe Data** | **Wajib**   | **Keterangan**                                                                             |
| ---------------------- | ------------- | ----------- | ------------------------------------------------------------------------------------------ |
| FX Rate ID             | VARCHAR(20)   | Ya          | Auto-generate (mis. FX-USD-20260131).                                                      |
| Kode Mata Uang         | FK            | Ya          | Reference ke Master Mata Uang.                                                             |
| Tanggal Berlaku        | DATE          | Ya          | Tanggal kurs berlaku.                                                                      |
| Kurs Beli (Bid)        | NUMERIC(15,4) | Tidak       | Untuk dokumentasi; rate untuk konversi FX → IDR (jual valas).                              |
| Kurs Jual (Ask)        | NUMERIC(15,4) | Tidak       | Untuk dokumentasi; rate untuk konversi IDR → FX (beli valas).                              |
| Kurs Tengah            | NUMERIC(15,4) | Ya          | Kurs yang dipakai untuk pembukuan (mid rate). 4 desimal.                                   |
| Sumber Kurs            | ENUM          | Ya          | BI\_JISDOR | BI\_KURS\_TENGAH | UPLOAD\_MANUAL.                                            |
| Periode Bulanan ID     | FK            | Ya          | Auto-derive dari Tanggal Berlaku.                                                          |
| Locked Flag            | BOOLEAN       | Ya          | Auto-set Y ketika periode bulanan terkait HARD CLOSED. Setelah Y, kurs tidak dapat diubah. |
| Maker / Approver       | FK            | Kondisional | Wajib untuk upload manual; Akuntansi (Maker) → Finance Controller (Approver).              |
| Dokumen Bukti (Upload) | FILE          | Kondisional | Wajib untuk upload manual; screenshot/PDF publikasi BI.                                    |

**Hierarki kurs untuk pembukuan (per event):**

| **Event Akuntansi**                     | **Kurs yang Dipakai**                                                                                                            |
| --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| **Penempatan / Pembelian**              | Kurs Tengah pada Tanggal Penempatan (trade date). EAD awal langsung ter-stamp dalam IDR equivalent.                              |
| **Akrual Bunga / Kupon Harian**         | Kurs Tengah pada tanggal akrual (harian). Bunga akrual dalam mata uang asli, langsung di-konversi ke IDR pada tanggal yang sama. |
| **MTM Harian**                          | Kurs Tengah pada tanggal closing. MTM dalam mata uang asli × kurs tanggal tersebut = MTM IDR.                                    |
| **Hitung ECL Akhir Bulan**              | Kurs Tengah pada tanggal akhir bulan (period-end rate). EAD dalam IDR equivalent dipakai untuk perhitungan ECL × PD × LGD.       |
| **Pembayaran Kupon (Realized)**         | Kurs Tengah pada tanggal pembayaran. Selisih dengan kurs akrual = realized FX gain/loss.                                         |
| **Penjualan / Pencairan / Jatuh Tempo** | Kurs Tengah pada tanggal closure. Selisih dengan kurs penempatan = realized FX gain/loss.                                        |
| **Closing Balance Periode (Reporting)** | Kurs Tengah pada tanggal akhir periode (bulanan/triwulanan/tahunan).                                                             |

**FX Gain/Loss Treatment:**

1.  Unrealized FX Gain/Loss: dihitung harian untuk setiap instrumen valas yang masih aktif. Selisih antara IDR equivalent saat ini (kurs hari ini) dengan IDR equivalent pada akrual terakhir → posting ke akun Selisih Kurs Belum Direalisasi (P\&L untuk FVTPL/AC, OCI untuk FVOCI utang).

2.  Realized FX Gain/Loss: dihitung saat closure event (penjualan, jatuh tempo, pencairan). Selisih antara IDR equivalent settlement actual dengan IDR equivalent posisi terakhir → posting ke akun Laba/Rugi Selisih Kurs (P\&L).

3.  Untuk instrumen FVOCI utang (mis. obligasi USD): unrealized FX gain/loss diakui di P\&L (BUKAN OCI), karena PSAK 71 mengharuskan FX revaluation pada monetary items diakui di P\&L, terpisah dari MTM (yang ke OCI).

**Ilustrasi numerik konversi:**

| **Skenario**                                                                   | **Nilai Asli** | **IDR Equivalent**                          |
| ------------------------------------------------------------------------------ | -------------- | ------------------------------------------- |
| Penempatan Deposito USD 100.000 pada 1 Jan 2026 (Kurs Tengah USD/IDR = 16.000) | USD 100.000,00 | Rp 1.600.000.000,00                         |
| Akrual bunga 1 bulan @ 5% p.a. pada 31 Jan 2026 (Kurs Tengah = 16.150)         | USD 416,67     | Rp 6.729,21                                 |
| MTM Position 31 Jan 2026 (Kurs Tengah = 16.150)                                | USD 100.416,67 | Rp 1.621.729,21 (×1.000)                    |
| Unrealized FX Gain (kurs naik 16.000→16.150)                                   | —              | Rp 15.000.000,00 (= USD 100.000 × Δ150)     |
| EAD untuk ECL Stage 1 akhir Jan 2026                                           | USD 100.416,67 | Rp 1.621.729.205,50 (basis perhitungan ECL) |

*Catatan: Seluruh perhitungan ECL (EAD × PD × LGD × Impact PD) dilakukan dalam IDR equivalent. PD dan LGD bersifat unitless (rasio), Impact PD juga unitless. Output ECL otomatis dalam IDR.*

**Aturan integritas master kurs:**

1.  Setiap kombinasi (Kode Mata Uang × Tanggal Berlaku) harus unique.

2.  Kurs dengan Sumber = BI\_JISDOR di-update otomatis melalui scheduled job pada hari kerja jam 10:30 WIB (post publikasi BI).

3.  Bila pada suatu hari kurs BI tidak tersedia (mis. hari libur/cuti bersama), sistem menggunakan kurs hari kerja terakhir; ini ter-flag REPEAT\_RATE untuk audit trail.

4.  Periode bulanan HARD CLOSED → seluruh kurs di periode tersebut auto-set Locked Flag = Y; tidak bisa diubah.

5.  Bila ada kebutuhan koreksi kurs di periode CLOSED (mis. ditemukan salah upload), koreksi harus dilakukan via prior-period FX adjustment journal entry pada periode terbuka berikutnya, mengikuti PSAK 25.

### 5.1.9 Master Chart of Accounts (CoA)

Master Chart of Accounts (CoA) menyimpan struktur kode akun general ledger yang dipakai sistem untuk posting jurnal. CoA bersifat referensi master — semua field Kode Akun di Master Mapping Jurnal (Bab 5.1.10) dan transaksi posting harus ter-reference ke entry valid di CoA. Sistem mendukung integrasi dengan CoA existing organisasi (mis. dari sistem ERP/GL) via API atau import Excel.

**Field Master Chart of Accounts:**

| **Field**           | **Tipe Data** | **Wajib** | **Keterangan**                                                                                                                                                                                                                                                   |
| ------------------- | ------------- | --------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kode Akun           | VARCHAR(20)   | Ya        | Kode unik akun GL (mis. 1.1.2.001, 4.1.1.005). Format mengikuti struktur CoA organisasi.                                                                                                                                                                         |
| Nama Akun           | VARCHAR(100)  | Ya        | Mis. "Surat Berharga AC — Obligasi", "Pendapatan Bunga — Deposito".                                                                                                                                                                                              |
| Tipe Akun           | ENUM          | Ya        | ASET | LIABILITAS | EKUITAS | PENDAPATAN | BEBAN | KONTINJEN.                                                                                                                                                                                                    |
| Sub-Tipe Akun       | ENUM          | Ya        | Untuk ASET: LANCAR | TIDAK\_LANCAR | KONTRA. Untuk LIABILITAS: JANGKA\_PENDEK | JANGKA\_PANJANG. Untuk EKUITAS: MODAL | LABA\_DITAHAN | OCI | KONTRA. Untuk PENDAPATAN: OPERASIONAL | NON\_OPERASIONAL | OCI. Untuk BEBAN: OPERASIONAL | NON\_OPERASIONAL | OCI. |
| Kategori Investasi  | ENUM          | Tidak     | Filter spesifik untuk akun yang terkait instrumen investasi: AC | FVOCI | FVTPL | OCI\_FVOCI | CKPN | NULL (untuk akun umum).                                                                                                                                    |
| Mata Uang Native    | CHAR(3)       | Ya        | Default IDR. Untuk akun multicurrency, mata uang native akun (umumnya IDR untuk semua akun investasi mengingat semua perhitungan dalam IDR equivalent).                                                                                                          |
| Posisi Normal       | ENUM          | Ya        | DEBIT | KREDIT — saldo normal akun (Aset/Beban = DEBIT; Liabilitas/Ekuitas/Pendapatan = KREDIT).                                                                                                                                                                 |
| Aktif Flag          | BOOLEAN       | Ya        | Y bila akun aktif untuk posting; N bila legacy/dihentikan.                                                                                                                                                                                                       |
| Parent Akun         | FK (self)     | Tidak     | Reference ke akun parent untuk struktur hierarki (mis. 1.1.2 adalah parent dari 1.1.2.001).                                                                                                                                                                      |
| Sumber CoA          | ENUM          | Ya        | INTERNAL | IMPORT\_ERP | IMPORT\_EXCEL — sumber pengisian record.                                                                                                                                                                                                |
| Tanggal Mulai Aktif | DATE          | Ya        | Tanggal akun mulai aktif untuk posting.                                                                                                                                                                                                                          |

**Contoh struktur CoA untuk modul investasi (ilustratif):**

| **Kode Akun** | **Nama Akun**                           | **Tipe / Sub-Tipe**           | **Kategori** |
| ------------- | --------------------------------------- | ----------------------------- | ------------ |
| 1.1.1.001     | Kas — Bank Mandiri (IDR)                | ASET / LANCAR                 | —            |
| 1.1.1.002     | Kas — Bank BCA (IDR)                    | ASET / LANCAR                 | —            |
| 1.1.2.001     | Surat Berharga AC — Obligasi            | ASET / TIDAK\_LANCAR          | AC           |
| 1.1.2.002     | Surat Berharga AC — Deposito            | ASET / LANCAR                 | AC           |
| 1.1.3.001     | Surat Berharga FVOCI — Obligasi         | ASET / TIDAK\_LANCAR          | FVOCI        |
| 1.1.3.002     | Surat Berharga FVOCI — Saham (Election) | ASET / TIDAK\_LANCAR          | FVOCI        |
| 1.1.4.001     | Surat Berharga FVTPL — Saham            | ASET / LANCAR                 | FVTPL        |
| 1.1.4.002     | Surat Berharga FVTPL — Reksadana        | ASET / LANCAR                 | FVTPL        |
| 1.1.9.001     | CKPN — Surat Berharga AC                | ASET / KONTRA                 | CKPN         |
| 1.1.9.002     | CKPN — Surat Berharga FVOCI (Memo)      | ASET / KONTRA                 | CKPN         |
| 1.1.9.003     | Akrual Bunga — Deposito                 | ASET / LANCAR                 | —            |
| 1.1.9.004     | Akrual Kupon — Obligasi                 | ASET / LANCAR                 | —            |
| 3.2.1.001     | OCI — Selisih MTM FVOCI Obligasi        | EKUITAS / OCI                 | OCI\_FVOCI   |
| 3.2.1.002     | OCI — Selisih MTM FVOCI Saham           | EKUITAS / OCI                 | OCI\_FVOCI   |
| 3.2.1.003     | OCI — CKPN FVOCI                        | EKUITAS / OCI                 | OCI\_FVOCI   |
| 4.1.1.001     | Pendapatan Bunga — Deposito             | PENDAPATAN / OPERASIONAL      | —            |
| 4.1.1.002     | Pendapatan Kupon — Obligasi             | PENDAPATAN / OPERASIONAL      | —            |
| 4.1.2.001     | Pendapatan Dividen                      | PENDAPATAN / OPERASIONAL      | —            |
| 4.1.2.002     | Pendapatan Distribusi Reksadana         | PENDAPATAN / OPERASIONAL      | —            |
| 4.1.3.001     | Realized Gain/Loss — Penjualan SB       | PENDAPATAN / NON\_OPERASIONAL | —            |
| 4.1.3.002     | Unrealized Gain/Loss — MTM FVTPL        | PENDAPATAN / NON\_OPERASIONAL | —            |
| 4.1.4.001     | Realized FX Gain/Loss                   | PENDAPATAN / NON\_OPERASIONAL | —            |
| 4.1.4.002     | Unrealized FX Gain/Loss                 | PENDAPATAN / NON\_OPERASIONAL | —            |
| 5.1.1.001     | Beban CKPN — Surat Berharga             | BEBAN / OPERASIONAL           | CKPN         |
| 5.1.2.001     | Beban PPh Final — Bunga                 | BEBAN / OPERASIONAL           | —            |

**Aturan integritas CoA:**

1.  Kode Akun harus unique.

2.  Akun yang dipakai sebagai referensi di Master Mapping Jurnal harus memiliki Aktif Flag = Y.

3.  Akun KONTRA (mis. CKPN) memiliki Posisi Normal = KREDIT meskipun klasifikasi induknya ASET.

4.  Sumber pengisian: input manual oleh Akuntansi, atau import via Excel/API dari sistem ERP/GL existing.

5.  Update CoA (mis. tambah akun baru, ubah nama) tidak boleh memengaruhi mapping yang sudah aktif — Akuntansi wajib review Master Mapping Jurnal setiap update CoA.

### 5.1.10 Master Mapping Jurnal (Header-Detail)

Master Mapping Jurnal mendefinisikan template jurnal akuntansi untuk setiap event bisnis. Struktur tabel mengikuti pola Header-Detail: header menyimpan event-level info, detail menyimpan satu atau lebih baris D/K (debit/kredit) yang dibentuk saat event terpicu. Setiap detail mereferensikan kode akun dari Master CoA (Bab 5.1.9). Sistem menggunakan event mapping generic — resolusi akun spesifik dilakukan saat runtime berdasarkan atribut instrumen (klasifikasi PSAK 71, sub-tipe, mata uang).

**Skema Header — Master Event Jurnal:**

| **Field**              | **Tipe Data** | **Wajib** | **Keterangan**                                                                                                                                                                                                                                         |
| ---------------------- | ------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Event ID               | VARCHAR(40)   | Ya        | Auto-generate atau ditetapkan (mis. EVT-PENEMPATAN, EVT-AKRUAL-BUNGA, EVT-MTM-FVOCI). Unique.                                                                                                                                                          |
| Event Code             | VARCHAR(40)   | Ya        | Kode internal yang dipakai di trigger sistem (mis. PENEMPATAN, AKRUAL\_BUNGA, MTM\_FVOCI, MTM\_FVTPL, ECL\_PEMBENTUKAN, ECL\_REVERSAL, JATUH\_TEMPO, PENJUALAN, REKLAS\_OCI\_PL, FX\_UNREALIZED, FX\_REALIZED, STAGE\_MIGRATION, PERIODE\_ADJUSTMENT). |
| Nama Event             | VARCHAR(120)  | Ya        | Deskripsi human-readable (mis. "Penempatan Instrumen Investasi", "Akrual Bunga Deposito Harian").                                                                                                                                                      |
| Kategori Event         | ENUM          | Ya        | PENEMPATAN | AKRUAL | MUTASI\_MTM | PENDAPATAN | ECL | CLOSURE | REKLASIFIKASI | FX | STAGE\_MIGRATION | PERIODE\_ADJUSTMENT.                                                                                                                          |
| Trigger Source         | ENUM          | Ya        | USER\_INPUT (transaksi manual) | SYSTEM\_JOB (job otomatis: akrual harian, MTM harian, ECL akhir bulan) | UPLOAD (mis. upload kurs).                                                                                                                   |
| Tipe Instrumen Berlaku | ARRAY\[ENUM\] | Tidak     | Filter tipe instrumen di mana event berlaku (mis. \[DEPOSITO, OBLIGASI\] untuk akrual). NULL = semua tipe.                                                                                                                                             |
| Klasifikasi Berlaku    | ARRAY\[ENUM\] | Tidak     | Filter klasifikasi: \[AC, FVOCI, FVTPL\]. NULL = semua klasifikasi.                                                                                                                                                                                    |
| Aktif Flag             | BOOLEAN       | Ya        | Y bila event aktif untuk posting jurnal otomatis.                                                                                                                                                                                                      |
| Catatan                | TEXT          | Tidak     | Penjelasan tambahan, referensi PSAK terkait.                                                                                                                                                                                                           |

**Skema Detail — Detail Mapping Jurnal:**

| **Field**              | **Tipe Data** | **Wajib** | **Keterangan**                                                                                                                                                                                                                                                                                                                                          |
| ---------------------- | ------------- | --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Detail ID              | VARCHAR(40)   | Ya        | Auto-generate. Unique within an event.                                                                                                                                                                                                                                                                                                                  |
| Event ID               | FK            | Ya        | Reference ke header Master Event.                                                                                                                                                                                                                                                                                                                       |
| Urutan                 | INT           | Ya        | Order line dalam jurnal (1, 2, 3, ...). Umumnya: 1 = debit pertama, 2 = kredit pertama, dst.                                                                                                                                                                                                                                                            |
| Kode Akun              | FK            | Ya        | Reference ke Master CoA (Bab 5.1.9). Sistem validasi Aktif Flag = Y saat runtime.                                                                                                                                                                                                                                                                       |
| D/K Indicator          | ENUM          | Ya        | DEBIT | KREDIT.                                                                                                                                                                                                                                                                                                                                         |
| Sumber Amount          | ENUM          | Ya        | EAD\_IDR | NOMINAL\_IDR | BUNGA\_AKRUAL\_IDR | KUPON\_AKRUAL\_IDR | MTM\_DELTA\_IDR | ECL\_AMOUNT\_IDR | DIVIDEN\_GROSS\_IDR | DIVIDEN\_NETTO\_IDR | PPH\_AMOUNT\_IDR | REALIZED\_GAINLOSS\_IDR | UNREALIZED\_GAINLOSS\_IDR | FX\_UNREALIZED\_IDR | FX\_REALIZED\_IDR | OCI\_BALANCE\_IDR. Field instrumen yang dipakai untuk amount (semua dalam IDR). |
| Klasifikasi Filter     | ENUM          | Tidak     | Filter klasifikasi: AC | FVOCI | FVTPL. Bila diisi, line ini hanya aktif jika klasifikasi instrumen sesuai. NULL = berlaku untuk semua.                                                                                                                                                                                                                 |
| Tipe Instrumen Filter  | ARRAY\[ENUM\] | Tidak     | Filter tipe instrumen tambahan. NULL = berlaku untuk semua.                                                                                                                                                                                                                                                                                             |
| Underlying Type Filter | ENUM          | Tidak     | Untuk reksadana look-through: NON\_EQUITY | EQUITY. NULL = berlaku untuk semua.                                                                                                                                                                                                                                                                         |
| Multiplier             | NUMERIC(8,4)  | Ya        | Default 1,0000. Untuk tax-adjusted line (mis. -0,1000 untuk PPh kupon 10%).                                                                                                                                                                                                                                                                             |
| Mata Uang Posting      | CHAR(3)       | Ya        | Default IDR (semua posting dalam IDR equivalent — lihat Bab 5.1.8).                                                                                                                                                                                                                                                                                     |
| Catatan                | TEXT          | Tidak     | Penjelasan rule khusus untuk line ini.                                                                                                                                                                                                                                                                                                                  |

**Resolusi runtime — bagaimana sistem memilih akun spesifik:**

Mapping bersifat generic (Opsi A). Saat event terpicu, sistem mengikuti algoritma resolusi:

1.  Sistem identifikasi event code dari trigger (mis. PENEMPATAN dari Modul 5.2).

2.  Sistem ambil semua detail line di event tersebut yang Aktif Flag = Y.

3.  Untuk tiap line, sistem cek filter (Klasifikasi, Tipe Instrumen, Underlying Type). Hanya line yang lulus semua filter yang akan di-post.

4.  Sistem ambil amount dari instrumen sesuai field Sumber Amount, dikalikan Multiplier. Amount sudah dalam IDR equivalent (lihat Bab 5.1.8).

5.  Sistem post journal entry ke GL: untuk tiap line, debit/kredit ke Kode Akun yang ditentukan.

6.  Validasi balance: total debit = total kredit per event posting. Bila tidak balance → block posting + alert ke Akuntansi.

**Contoh mapping event (illustratif untuk 4 event utama):**

*Event: PENEMPATAN — header:*

| **Field**              | **Nilai**                              |
| ---------------------- | -------------------------------------- |
| Event Code             | PENEMPATAN                             |
| Nama Event             | Penempatan Instrumen Investasi         |
| Kategori               | PENEMPATAN                             |
| Trigger Source         | USER\_INPUT                            |
| Tipe Instrumen Berlaku | \[DEPOSITO, OBLIGASI, SAHAM, RDN\_\*\] |
| Klasifikasi Berlaku    | \[AC, FVOCI, FVTPL\]                   |

*Event: PENEMPATAN — detail:*

| **Urut** | **Kode Akun** | **D/K**    | **Sumber Amount** | **Klasifikasi** | **Multiplier** |
| -------- | ------------- | ---------- | ----------------- | --------------- | -------------- |
| 1        | 1.1.2.001     | **DEBIT**  | EAD\_IDR          | AC              | 1,0000         |
| 1        | 1.1.3.001     | **DEBIT**  | EAD\_IDR          | FVOCI           | 1,0000         |
| 1        | 1.1.4.001     | **DEBIT**  | EAD\_IDR          | FVTPL           | 1,0000         |
| 2        | 1.1.1.001     | **KREDIT** | EAD\_IDR          | —               | 1,0000         |

*Catatan: 3 line debit dengan filter klasifikasi yang berbeda — sistem otomatis pilih line yang sesuai dengan klasifikasi instrumen yang sedang dipost. Line kredit tunggal ke Kas berlaku untuk semua klasifikasi.*

*Event: AKRUAL\_BUNGA — detail (untuk deposito AC):*

| **Urut** | **Kode Akun** | **D/K**    | **Sumber Amount**  | **Tipe Instrumen** | **Multiplier** |
| -------- | ------------- | ---------- | ------------------ | ------------------ | -------------- |
| 1        | 1.1.9.003     | **DEBIT**  | BUNGA\_AKRUAL\_IDR | DEPOSITO           | 1,0000         |
| 1        | 1.1.9.004     | **DEBIT**  | KUPON\_AKRUAL\_IDR | OBLIGASI           | 1,0000         |
| 2        | 4.1.1.001     | **KREDIT** | BUNGA\_AKRUAL\_IDR | DEPOSITO           | 1,0000         |
| 2        | 4.1.1.002     | **KREDIT** | KUPON\_AKRUAL\_IDR | OBLIGASI           | 1,0000         |

*Event: ECL\_PEMBENTUKAN — detail (3 skenario klasifikasi):*

| **Urut** | **Kode Akun** | **D/K**    | **Sumber Amount** | **Klasifikasi** | **Multiplier** |
| -------- | ------------- | ---------- | ----------------- | --------------- | -------------- |
| 1        | 5.1.1.001     | **DEBIT**  | ECL\_AMOUNT\_IDR  | AC              | 1,0000         |
| 1        | 5.1.1.001     | **DEBIT**  | ECL\_AMOUNT\_IDR  | FVOCI           | 1,0000         |
| 2        | 1.1.9.001     | **KREDIT** | ECL\_AMOUNT\_IDR  | AC              | 1,0000         |
| 2        | 3.2.1.003     | **KREDIT** | ECL\_AMOUNT\_IDR  | FVOCI           | 1,0000         |

*Catatan: AC menempatkan kredit CKPN ke akun kontra-aset (1.1.9.001); FVOCI menempatkan kredit ke OCI (3.2.1.003) karena CKPN FVOCI hanya memo entry di OCI, tidak mengurangi nilai tercatat aset.*

*Event: PEMBAYARAN\_KUPON — detail (gross-up dengan PPh):*

| **Urut** | **Kode Akun** | **D/K**    | **Sumber Amount**  | **Tipe** | **Multiplier** |
| -------- | ------------- | ---------- | ------------------ | -------- | -------------- |
| 1        | 1.1.1.001     | **DEBIT**  | KUPON\_AKRUAL\_IDR | —        | 0,9000         |
| 2        | 5.1.2.001     | **DEBIT**  | KUPON\_AKRUAL\_IDR | —        | 0,1000         |
| 3        | 1.1.9.004     | **KREDIT** | KUPON\_AKRUAL\_IDR | OBLIGASI | 1,0000         |

*Catatan: Kas yang diterima = 90% (netto setelah PPh kupon 10%); Beban PPh diakui terpisah sebesar 10%; akrual kupon di-clear sepenuhnya. Total Debit (0,90 + 0,10) = Kredit (1,00) — balance.*

**Event mapping yang ter-cover di sistem:**

| **Event Code**                  | **Deskripsi & Detail Mapping (Ringkas)**                                                                                                                                                                                        |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **PENEMPATAN**                  | D: SB AC/FVOCI/FVTPL (sesuai klasifikasi); K: Kas. EAD awal langsung dalam IDR equivalent.                                                                                                                                      |
| **AKRUAL\_BUNGA**               | D: Akrual Bunga/Kupon; K: Pendapatan Bunga/Kupon. Harian, IDR.                                                                                                                                                                  |
| **MTM\_FVOCI**                  | D atau K: SB FVOCI; K atau D: OCI Selisih MTM FVOCI. Jurnal balik bila MTM negatif.                                                                                                                                             |
| **MTM\_FVTPL**                  | D atau K: SB FVTPL; K atau D: Unrealized Gain/Loss MTM (P\&L).                                                                                                                                                                  |
| **PEMBAYARAN\_BUNGA / KUPON**   | D: Kas (90% / netto); D: Beban PPh (10%); K: Akrual Bunga/Kupon.                                                                                                                                                                |
| **PENERIMAAN\_DIVIDEN**         | D: Kas (90%); D: Beban PPh Dividen (10%); K: Pendapatan Dividen.                                                                                                                                                                |
| **DISTRIBUSI\_REKSADANA**       | D: Kas; K: Pendapatan Distribusi Reksadana (tidak kena PPh — bukan objek pajak).                                                                                                                                                |
| **ECL\_PEMBENTUKAN**            | D: Beban CKPN; K: CKPN (untuk AC) atau OCI CKPN (untuk FVOCI).                                                                                                                                                                  |
| **ECL\_REVERSAL**               | Jurnal balik dari ECL\_PEMBENTUKAN bila ECL turun.                                                                                                                                                                              |
| **PENJUALAN\_PENCAIRAN**        | D: Kas; D atau K: Realized Gain/Loss; K: SB; (untuk FVOCI utang: tambahan REKLAS\_OCI\_PL).                                                                                                                                     |
| **JATUH\_TEMPO**                | D: Kas; K: SB (di nilai par); selisih ke Realized Gain/Loss.                                                                                                                                                                    |
| **REKLAS\_OCI\_PL**             | D atau K: OCI Selisih MTM FVOCI; K atau D: Realized Gain/Loss. Saat closure FVOCI utang.                                                                                                                                        |
| **FX\_UNREALIZED**              | D atau K: SB (instrumen valas); K atau D: Unrealized FX Gain/Loss (P\&L).                                                                                                                                                       |
| **FX\_REALIZED**                | D atau K: Kas / SB; K atau D: Realized FX Gain/Loss (P\&L). Saat closure valas.                                                                                                                                                 |
| **STAGE\_MIGRATION**            | Δ ECL = ECL\_baru − ECL\_lama. Bila Δ \> 0: trigger ECL\_PEMBENTUKAN incremental; bila Δ \< 0: trigger ECL\_REVERSAL incremental.                                                                                               |
| **PERIODE\_ADJUSTMENT**         | Free-form journal entry oleh Akuntansi pada periode SOFT\_CLOSED, dengan referensi event-asli + adjustment reason. Mapping flexible (manual).                                                                                   |
| **CORRECTION\_PERIODE\_CLOSED** | Prior-period adjustment journal entry pada periode terbuka berikutnya. Mengikuti PSAK 25.                                                                                                                                       |
| AMORTISASI\_PREMI\_DISKONTO     | D atau K: SB AC/FVOCI; K atau D: Pendapatan Bunga/Kupon. Amortisasi premium (mengurangi pendapatan) atau diskonto (menambah pendapatan) per periode posting. Dipicu otomatis bersama AKRUAL\_BUNGA untuk EIR Method Flag = Y.   |
| EIR\_REESTIMATION               | Tidak ada jurnal langsung; sistem menghitung ulang EIR & regenerate amortization schedule. Bila modifikasi material → trigger MODIFIKASI\_MATERIAL (derecognition + recognition baru). Dokumentasikan delta EIR di audit trail. |
| MODIFIKASI\_MATERIAL            | D: SB Baru; K: SB Lama; selisih ke Realized Gain/Loss (P\&L) untuk AC, atau ke OCI untuk FVOCI. Mengikuti PSAK 71 paragraf 3.3.2: derecognition aset asli + recognition aset baru pada nilai wajar.                             |

## 5.2 Modul Penempatan Instrumen

Modul penempatan mencatat transaksi pembelian/penempatan instrumen baru. Setiap penempatan wajib disertai bukti dokumen melalui media upload.

### 5.2.1 Field Transaksi Penempatan

| **Field**               | **Tipe Data** | **Wajib**   | **Keterangan**                                                             |
| ----------------------- | ------------- | ----------- | -------------------------------------------------------------------------- |
| Nomor Transaksi         | VARCHAR(20)   | Ya          | Auto-generate (mis. PNP-2026-00001).                                       |
| Tanggal Transaksi       | DATE          | Ya          | Trade date.                                                                |
| Tanggal Settlement      | DATE          | Ya          | Value date / settlement date.                                              |
| Kode Instrumen          | FK            | Ya          | Reference master instrumen.                                                |
| Nominal/Face Value      | NUMERIC(20,2) | Ya          | Untuk obligasi: face value; deposito: pokok; reksadana: jumlah pembayaran. |
| Harga Beli (% atau NAB) | NUMERIC(15,4) | Kondisional | Obligasi: % dari nominal (4 desimal); reksadana: NAB per unit.             |
| Jumlah Unit             | NUMERIC(18,4) | Kondisional | Khusus reksadana, 4 desimal.                                               |
| Accrued Interest Dibeli | NUMERIC(20,2) | Kondisional | Khusus obligasi, jika beli di antara tanggal kupon.                        |
| Total Pembayaran        | NUMERIC(20,2) | Ya          | Auto-calc: harga beli + accrued interest + biaya.                          |
| Biaya Transaksi         | NUMERIC(20,2) | Tidak       | Biaya broker / komisi (untuk reksadana = subscription fee).                |
| Akun Sumber Dana        | FK            | Ya          | Rekening kas/bank yang dipakai untuk pembayaran.                           |
| Dokumen Bukti (Upload)  | FILE          | Ya          | Bilyet, NoA, konfirmasi pembelian — lihat 5.8.                             |
| User Maker / Approver   | FK            | Ya          | Pemisahan tugas (4-eyes principle).                                        |

### 5.2.2 Validasi Bisnis pada Saat Penempatan

1.  Tanggal jatuh tempo \> tanggal penempatan untuk deposito dan obligasi.

2.  Jumlah unit reksadana = total pembayaran ÷ NAB per unit (toleransi pembulatan 4 desimal).

3.  Sumber dana rekening harus mempunyai saldo ≥ total pembayaran.

4.  Counterparty harus aktif dan memiliki Rating Pefindo yang valid (kecuali sovereign).

5.  Wajib unggah minimal 1 (satu) dokumen bukti.

## 5.3 Modul Mutasi / Mark-to-Market (MTM)

### 5.3.1 Cakupan MTM per Tipe Instrumen

| **Instrumen**          | **Frekuensi**             | **Sumber Harga**           | **Pengakuan Selisih**                               |
| ---------------------- | ------------------------- | -------------------------- | --------------------------------------------------- |
| Cash di Bank           | Harian (saldo aktual)     | Rekening koran             | Tidak ada MTM (Amortized Cost)                      |
| Deposito               | Tidak ada                 | Nominal pokok              | Tidak ada MTM (Amortized Cost; akrual bunga harian) |
| Obligasi (FVOCI)       | Harian                    | Harga IBPA (upload harian) | OCI (Other Comprehensive Income)                    |
| Obligasi (AC)          | Harian (untuk monitoring) | Harga IBPA (upload harian) | Tidak dijurnal; hanya monitoring impairment         |
| Saham (FVTPL)          | Harian                    | Harga penutupan BEI harian | P\&L (laba rugi periode)                            |
| Saham (FVOCI Election) | Harian                    | Harga penutupan BEI harian | OCI (no recycling — tidak pernah ke P\&L)           |
| Reksadana (FVTPL)      | Harian                    | NAB harian dari MI/KSEI    | P\&L (laba rugi periode)                            |
| Reksadana (FVOCI)      | Harian                    | NAB harian dari MI/KSEI    | OCI (with recycling saat redemption)                |

### 5.3.2 Field Transaksi MTM

| **Field**                  | **Tipe Data** | **Wajib** | **Keterangan**                                           |
| -------------------------- | ------------- | --------- | -------------------------------------------------------- |
| Tanggal Valuasi            | DATE          | Ya        | Tanggal posisi valuasi.                                  |
| Kode Instrumen             | FK            | Ya        | Reference master instrumen.                              |
| Carrying Amount Sebelumnya | NUMERIC(20,2) | Ya        | Nilai tercatat sebelum MTM.                              |
| Harga / NAB Baru           | NUMERIC(15,4) | Ya        | 4 desimal.                                               |
| Fair Value Baru            | NUMERIC(20,2) | Ya        | Untuk obligasi: % × nominal; reksadana: unit × NAB.      |
| Selisih MTM                | NUMERIC(20,2) | Ya        | Auto-calc: Fair Value Baru − Carrying Amount Sebelumnya. |
| Akun Pengakuan             | ENUM          | Ya        | OCI | LABA-RUGI.                                         |
| Dokumen Sumber (Upload)    | FILE          | Ya        | File IBPA harian / NAB report harian.                    |

## 5.4 Modul Renewal (Khusus Deposito)

Renewal berlaku untuk deposito yang jatuh tempo, baik secara otomatis (auto-rollover) maupun manual. Sistem mendukung dua skema:

1.  Renewal Pokok Saja — bunga ditarik ke rekening bank, pokok diperpanjang dengan tenor & rate baru.

2.  Renewal Pokok + Bunga Net — bunga (setelah PPh 4(2) Final 20%) digabung ke pokok, deposito baru dengan nominal yang lebih besar.

### 5.4.1 Field Transaksi Renewal

| **Field**                | **Tipe Data** | **Wajib** | **Keterangan**                         |
| ------------------------ | ------------- | --------- | -------------------------------------- |
| Kode Instrumen Lama      | FK            | Ya        | Deposito yang di-renewal.              |
| Tanggal Jatuh Tempo Lama | DATE          | Ya        | Auto.                                  |
| Skema Renewal            | ENUM          | Ya        | POKOK\_SAJA | POKOK\_PLUS\_BUNGA.      |
| Tenor Baru               | INT           | Ya        | Dalam hari/bulan.                      |
| Suku Bunga Baru          | NUMERIC(8,4)  | Ya        | % per annum, 4 desimal.                |
| Pokok Baru               | NUMERIC(20,2) | Ya        | Auto-calc berdasarkan skema.           |
| Kode Instrumen Baru      | VARCHAR(20)   | Ya        | Auto-generate.                         |
| Dokumen Bukti (Upload)   | FILE          | Ya        | Bilyet baru, surat instruksi rollover. |

## 5.5 Modul Penjualan / Pencairan

Penjualan/pencairan berlaku saat instrumen di-disposal sebelum jatuh tempo (untuk deposito = break deposito; obligasi = penjualan secondary market; reksadana = redemption).

### 5.5.1 Field Transaksi Penjualan

| **Field**                 | **Tipe Data** | **Wajib**   | **Keterangan**                                 |
| ------------------------- | ------------- | ----------- | ---------------------------------------------- |
| Kode Instrumen            | FK            | Ya          | Reference master instrumen.                    |
| Tanggal Penjualan         | DATE          | Ya          | Trade date.                                    |
| Tanggal Settlement        | DATE          | Ya          | Value date.                                    |
| Nominal/Unit yang Dijual  | NUMERIC(20,4) | Ya          | Untuk parsial penjualan.                       |
| Harga Jual (% atau NAB)   | NUMERIC(15,4) | Ya          | 4 desimal.                                     |
| Accrued Interest Dijual   | NUMERIC(20,2) | Kondisional | Obligasi.                                      |
| Total Penerimaan          | NUMERIC(20,2) | Ya          | Auto-calc.                                     |
| Biaya Transaksi           | NUMERIC(20,2) | Tidak       | Komisi / redemption fee.                       |
| Carrying Amount Saat Jual | NUMERIC(20,2) | Ya          | Auto-calc.                                     |
| Realized Gain/Loss        | NUMERIC(20,2) | Ya          | Auto-calc: Penerimaan − Carrying − biaya.      |
| Dokumen Bukti (Upload)    | FILE          | Ya          | Konfirmasi penjualan, redemption confirmation. |

## 5.6 Modul Jatuh Tempo (Closure)

Setiap H-1 jatuh tempo, sistem menampilkan list instrumen yang akan jatuh tempo besok. Pada tanggal jatuh tempo, sistem membentuk transaksi closure otomatis (atau menunggu konfirmasi user untuk deposito non-auto-rollover).

### 5.6.1 Logika Jatuh Tempo

1.  Deposito dengan Auto Renewal = Y → otomatis renewal sesuai skema rollover (lihat 5.4).

2.  Deposito dengan Auto Renewal = N → settlement: pokok + bunga akrual sisa masuk ke rekening bank.

3.  Obligasi → settlement: nominal + kupon terakhir masuk ke rekening bank; saldo OCI direklasifikasi ke laba/rugi.

4.  Reksadana → tidak memiliki jatuh tempo (perpetual); hanya bisa di-redeem (Modul 5.5).

## 5.7 Modul Pendapatan Investasi

| **Tipe Pendapatan**        | **Mekanisme**                                                                                                                                                                                                                                                                                                                                           |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bunga Cash di Bank         | Untuk AC dengan rate variabel harian: Saldo × Rate harian (simple interest); EIR Method Flag = N. Pencatatan otomatis dari rekening koran.                                                                                                                                                                                                              |
| Bunga Deposito             | Berbasis EIR (PSAK 71 paragraf 5.4): Akrual harian = Carrying × EIR ÷ 365. Untuk deposito plain vanilla tanpa premium/diskonto, EIR ≈ kupon nominal sehingga praktis identik dengan formula sederhana Pokok × Rate ÷ 365. Jurnal akrual harian; PPh 4(2) Final 20% diakui saat realisasi.                                                               |
| Kupon Obligasi             | Berbasis EIR (PSAK 71 paragraf 5.4): Pendapatan bunga harian = Gross Carrying × EIR ÷ 365 (Stage 1 & 2) atau Net Carrying × EIR ÷ 365 (Stage 3). Selisih dengan kupon kontraktual nominal = Amortisasi Premium/Diskonto. Lihat Bab 5.12 untuk algoritma & jurnal. PPh Final (10% korporasi non-public, 0%/10% pemerintah) diakui saat penerimaan kupon. |
| Dividen Saham              | Diakui pada cum-date / ex-date sesuai kebijakan akuntansi; PPh Final 10% (untuk WP OP) atau exemption sesuai PP 9/2021 untuk WP Badan yang reinvestasi. Untuk saham FVOCI Election, dividen tetap diakui di P\&L (bukan OCI).                                                                                                                           |
| Distribusi/Hasil Reksadana | Diakui saat dibagikan oleh MI (cash distribution); jurnal pengakuan langsung ke laba/rugi.                                                                                                                                                                                                                                                              |
| MTM Gain/Loss              | Lihat Modul 5.3.                                                                                                                                                                                                                                                                                                                                        |

## 5.8 Modul Media Upload

Semua dokumen bukti melekat pada event-nya, dengan trail audit lengkap. Sistem menyimpan file dalam object storage terenkripsi (mis. S3 + KMS).

### 5.8.1 Spesifikasi Teknis Upload

| **Aspek**                  | **Spesifikasi**                                                                       |
| -------------------------- | ------------------------------------------------------------------------------------- |
| Format File yang Diizinkan | PDF, JPG, JPEG, PNG, XLSX, XLS, CSV                                                   |
| Ukuran Maksimum per File   | 10 MB                                                                                 |
| Jumlah File per Event      | Maksimum 10 file                                                                      |
| Penamaan File              | Auto-rename: {KODE\_INSTRUMEN}\_{EVENT}\_{TIMESTAMP}\_{ORIGINAL\_NAME}                |
| Anti-Virus Scan            | Wajib scan saat upload (mis. ClamAV); blok jika positif                               |
| Enkripsi Penyimpanan       | Server-side encryption (AES-256)                                                      |
| Audit Trail                | Mencatat: user, timestamp, IP address, hash SHA-256 file, action (upload/view/delete) |
| Retensi                    | Minimal 10 tahun sesuai ketentuan dokumen pendukung pajak/audit                       |
| Akses                      | RBAC — hanya user dengan role tertentu (Maker, Approver, Auditor) yang bisa view      |

### 5.8.2 Daftar Wajib Upload per Event

| **Event**              | **Wajib Upload**    | **Contoh Dokumen**                                                         |
| ---------------------- | ------------------- | -------------------------------------------------------------------------- |
| Penempatan Deposito    | Ya (min. 1)         | Bilyet/Sertifikat Deposito, Surat Konfirmasi                               |
| Penempatan Obligasi    | Ya (min. 1)         | Notice of Allocation (NoA), Trade Confirmation, Settlement Confirmation    |
| Penempatan Reksadana   | Ya (min. 1)         | Surat Konfirmasi Pembelian, Bukti Transfer                                 |
| MTM Harian             | Ya (per batch)      | File IBPA harian, NAB report harian dari KSEI/MI                           |
| Renewal                | Ya (min. 1)         | Bilyet Deposito Baru, Surat Instruksi Rollover                             |
| Penjualan/Pencairan    | Ya (min. 1)         | Konfirmasi Penjualan, Redemption Confirmation, Bukti Penerimaan            |
| Jatuh Tempo            | Ya (min. 1)         | Surat Pencairan Deposito, Bukti Settlement Pokok+Kupon Obligasi            |
| Pendapatan Bunga/Kupon | Ya (per pembayaran) | Bukti Setor Bunga, Bukti Potong Pajak (BPP) PPh Final                      |
| Pengakuan ECL          | Ya (per periode)    | File parameter PD Pefindo, file Impact PD forward-looking, perhitungan ECL |

### 5.8.3 Upload Khusus: Total Impact MEV to PD (Derivasi PD Good & PD Bad)

Total Impact MEV to PD adalah multiplier forward-looking yang merepresentasikan dampak proyeksi variabel makroekonomi (Macroeconomic Variables — MEV) terhadap PD. Sistem TIDAK mengupload PD Good/Bad langsung dari Pefindo; PD Good (skenario Optimistic) dan PD Bad (skenario Pessimistic) DIDERIVASI dari PD Normal (sumber Pefindo) menggunakan Impact MEV.

**Formula derivasi PD per skenario (per rating, per tenor):**

| **PD Skenario**          | **Formula**                                   |
| ------------------------ | --------------------------------------------- |
| **PD Good (Optimistic)** | PD Good = PD Normal × Impact MEV (Optimistic) |
| **PD Normal (Base)**     | PD Normal = data Pefindo (sumber utama)       |
| **PD Bad (Pessimistic)** | PD Bad = PD Normal × Impact MEV (Pessimistic) |

*Catatan: Impact MEV (Optimistic) umumnya \< 1,0000 (mengurangi PD karena kondisi membaik); Impact MEV (Pessimistic) umumnya \> 1,0000 (memperbesar PD karena kondisi memburuk); Impact MEV (Base) tidak diperlukan karena PD Normal langsung dipakai sebagai PD Base.*

**Spesifikasi file upload Total Impact MEV to PD:**

| **Field**                     | **Spesifikasi**                                                                                        |
| ----------------------------- | ------------------------------------------------------------------------------------------------------ |
| Format File                   | XLSX (template baku disediakan sistem)                                                                 |
| Periode Berlaku               | Tanggal mulai – tanggal akhir berlaku                                                                  |
| Skenario                      | GOOD (Optimistic) | BAD (Pessimistic) — TIDAK perlu Base/Normal karena PD Normal langsung dari Pefindo |
| Tipe Counterparty             | BANK | KORPORASI | PEMERINTAH | MI | EMITEN\_SAHAM                                                     |
| Tipe Instrumen                | CASH | DEPOSITO | OBLIGASI | REKSADANA                                                                 |
| Sub-Tipe Instrumen (opsional) | Granularitas tambahan jika diperlukan                                                                  |
| Rating Bucket (opsional)      | idAAA, idAA, idA, dst.                                                                                 |
| Komponen MEV (sheet detail)   | Variabel makro & koefisien sensitivitas (GDP, CPI, BI Rate, USD/IDR, dll)                              |
| Total Impact MEV Multiplier   | NUMERIC, 4 desimal — hasil agregasi komponen MEV                                                       |
| Dokumen Pendukung             | Memo Komite Risiko, analisis makro, model dokumentasi, wajib di-attach                                 |
| Otorisasi                     | Disetujui minimal 2 level (Maker Risk Officer → Approver Komite Risiko/ALCO)                           |
| Audit Trail                   | Versi dan history nilai per periode; rekam variabel MEV input dan koefisien                            |
| Frekuensi Update              | Minimal triwulanan; ad-hoc bila terjadi shock makroekonomi material                                    |

**Template upload (sheet 1 — Summary Multiplier):**

| **Skenario** | **Tipe Instrumen** | **Rating Bucket** | **Impact MEV** | **Catatan**                          |
| ------------ | ------------------ | ----------------- | -------------- | ------------------------------------ |
| GOOD         | OBLIGASI           | idA               | 0,5000         | GDP growth +5,5% → PD turun 50%      |
| BAD          | OBLIGASI           | idA               | 2,5000         | GDP growth +3,5% → PD naik 150%      |
| GOOD         | DEPOSITO           | idAAA             | 0,5000         | —                                    |
| BAD          | DEPOSITO           | idAAA             | 2,5000         | —                                    |
| …            | …                  | …                 | …              | Diisi seluruh kombinasi yang relevan |

**Template upload (sheet 2 — Detail Komponen MEV):**

| **Variabel MEV**    | **Bobot** | **GOOD Scenario** | **Normal** | **BAD Scenario** |
| ------------------- | --------- | ----------------- | ---------- | ---------------- |
| GDP Growth (%)      | 30%       | 5,50%             | 5,00%      | 3,50%            |
| CPI / Inflasi (%)   | 20%       | 2,50%             | 3,00%      | 5,00%            |
| BI Rate (%)         | 20%       | 5,00%             | 5,75%      | 7,00%            |
| USD/IDR (Rp)        | 15%       | 15.500            | 16.000     | 17.500           |
| Oil Price (USD/bbl) | 10%       | 70                | 85         | 110              |
| IHSG Growth (%)     | 5%        | 10,00%            | 5,00%      | −5,00%           |

*Total Impact MEV (Summary Multiplier sheet 1) merupakan hasil agregasi tertimbang dari komponen MEV pada sheet 2 melalui model regresi/expert judgment yang didokumentasikan terpisah.*

### 5.8.3.a Upload Khusus: Impact PD (Multiplier ECL ke ECL FL)

Impact PD adalah multiplier akhir yang dikenakan pada nilai ECL Weighted untuk menghasilkan ECL Forward-Looking (ECL FL). Berbeda dari Impact MEV to PD (yang men-derivasi PD Good/Bad dari PD Normal di tingkat parameter), Impact PD bekerja di tingkat output ECL — sebagai layer adjustment final yang merefleksikan risiko forward-looking yang belum tertangkap oleh PD scenario semata.

**Formula penggunaan Impact PD:**

| **Komponen**       | **Formula**                           |
| ------------------ | ------------------------------------- |
| **ECL FL (final)** | **ECL FL = ECL Weighted × Impact PD** |

**Hubungan dua mekanisme forward-looking:**

1.  Impact MEV to PD bekerja di TINGKAT INPUT (parameter PD per skenario): mengubah PD Normal menjadi PD Good dan PD Bad untuk perhitungan tiga skenario ECL.

2.  Impact PD bekerja di TINGKAT OUTPUT (hasil ECL Weighted): menerapkan layer multiplier final menjadi ECL FL yang dijurnal.

3.  Kedua mekanisme dapat hidup berdampingan: Impact MEV menyesuaikan probabilitas default secara skenario, sementara Impact PD menyesuaikan nilai ECL agregat untuk faktor-faktor yang tidak tertangkap di tingkat parameter (mis. management overlay, expert judgment, sektor-spesifik).

**Spesifikasi file upload Impact PD:**

| **Field**                | **Spesifikasi**                                                                     |
| ------------------------ | ----------------------------------------------------------------------------------- |
| Format File              | XLSX (template baku disediakan sistem)                                              |
| Periode Berlaku          | Tanggal mulai – tanggal akhir berlaku                                               |
| Tipe Counterparty        | BANK | KORPORASI | PEMERINTAH | MI | EMITEN\_SAHAM (opsional, default = ALL)        |
| Tipe Instrumen           | CASH | DEPOSITO | OBLIGASI | REKSADANA (opsional, default = ALL)                    |
| Rating Bucket (opsional) | idAAA, idAA, idA, dst.                                                              |
| Stage (opsional)         | STAGE\_1 | STAGE\_2 | STAGE\_3                                                      |
| Impact PD Multiplier     | NUMERIC, 4 desimal (mis. 1,1500 berarti +15% adjustment final ke ECL)               |
| Justifikasi              | TEXT — alasan management overlay, sektor-spesifik, atau expert judgment             |
| Dokumen Pendukung        | Memo Komite Risiko, model overlay, analisis sektor, wajib di-attach                 |
| Otorisasi                | Disetujui minimal 2 level (Maker Risk Officer → Approver Komite Risiko/CFO)         |
| Audit Trail              | Versi dan history nilai per periode                                                 |
| Frekuensi Update         | Minimal triwulanan, atau ad-hoc bila ada peristiwa material yang memerlukan overlay |

**Template upload Impact PD:**

| **Tipe Instrumen** | **Rating Bucket** | **Stage** | **Impact PD** | **Justifikasi**                         |
| ------------------ | ----------------- | --------- | ------------- | --------------------------------------- |
| OBLIGASI           | idA               | STAGE\_1  | 1,1500        | Management overlay sektor properti +15% |
| OBLIGASI           | idBBB             | STAGE\_1  | 1,2500        | Management overlay sektor properti +25% |
| DEPOSITO           | idAAA             | STAGE\_1  | 1,0500        | Standard FL multiplier                  |
| …                  | …                 | …         | …             | Diisi seluruh kombinasi yang relevan    |

### 5.8.4 Upload Khusus: Underlying Detail Reksadana

Setiap reksadana yang diinvestasikan WAJIB di-upload detail underlying-nya secara periodik (minimal bulanan) untuk: (a) perhitungan ECL look-through, (b) monitoring konsentrasi risiko, (c) verifikasi konsistensi sub-tipe reksadana (mis. RDN Pendapatan Tetap harus tetap memiliki underlying obligasi ≥ 80%), (d) audit trail komposisi portofolio.

**Spesifikasi file upload Underlying Reksadana:**

| **Field**            | **Spesifikasi**                                                                              |
| -------------------- | -------------------------------------------------------------------------------------------- |
| Format File          | XLSX (template baku disediakan sistem)                                                       |
| Sumber Data          | Fund Fact Sheet (FFS), Laporan Bulanan MI, atau export Bank Kustodian                        |
| Frekuensi Upload     | Minimal bulanan (akhir bulan); ad-hoc bila ada perubahan strategi MI                         |
| Tanggal Snapshot NAB | Tanggal posisi underlying (umumnya akhir bulan)                                              |
| Validasi Sistem      | Total bobot underlying = 100,0000% (toleransi ±0,01%); konsistensi dengan sub-tipe reksadana |
| Otorisasi            | Maker (Treasury) → Approver (Risk Officer)                                                   |
| Audit Trail          | Versi snapshot bulanan; tampilan trend komposisi underlying                                  |

**Template upload (per reksadana):**

| **No**    | **Nama Underlying**                           | **Kategori**       | **Bobot %**   | **ISIN/Kode** | **Issuer / Counterparty**               | Rating Pefindo             |
| --------- | --------------------------------------------- | ------------------ | ------------- | ------------- | --------------------------------------- | -------------------------- |
| 1         | ORI023                                        | Obligasi Negara    | 25,0000%      | IDG000016605  | Pemerintah RI                           | Sovereign (idAAA implicit) |
| 2         | Obligasi Berkelanjutan I PT XYZ Tahap II 2024 | Obligasi Korporasi | 20,0000%      | IDA000XXXXXX  | PT XYZ Tbk (idA)                        | idA                        |
| 3         | Sukuk Mudharabah PT ABC                       | Sukuk Korporasi    | 15,0000%      | IDB000YYYYYY  | PT ABC Tbk (idAA)                       | idAA                       |
| 4         | Saham BBCA                                    | Saham              | 10,0000%      | BBCA          | PT Bank Central Asia Tbk                | —                          |
| 5         | Deposito Bank Mandiri 3M                      | Deposito           | 20,0000%      | —             | PT Bank Mandiri (Persero) Tbk (idAAA)   | idAAA                      |
| 6         | Cash & Equivalents                            | Cash               | 10,0000%      | —             | Multi-bank (lihat detail tab kustodian) | —                          |
| **TOTAL** |                                               |                    | **100,0000%** |               |                                         |                            |

**Validasi otomatis sistem saat upload:**

1.  Total bobot underlying = 100,0000% (toleransi ±0,01%).

2.  Issuer/Counterparty pada underlying harus terdaftar di Master Counterparty (auto-suggest atau redirect ke modul tambah counterparty).

3.  Untuk underlying obligasi/sukuk: rating Pefindo wajib ada di Counterparty Rating History (untuk lookup PD). Kolom Rating wajib diisi di template upload (lihat template di atas).

4.  Manajer Investasi (MI) yang mengelola reksadana harus terdaftar di Master Counterparty dengan Tipe = MANAJER\_INVESTASI dan memiliki Nomor Izin OJK yang valid.

5.  Bank Kustodian harus terdaftar di Master Counterparty dengan Tipe = BANK\_KUSTODIAN.

6.  Konsistensi dengan sub-tipe reksadana:
    
      - RDN Pendapatan Tetap → underlying obligasi+sukuk ≥ 80%
    
      - RDN Saham → underlying saham ≥ 80%
    
      - RDN Pasar Uang → underlying instrumen pasar uang (deposito, SBI, SPN, sertifikat deposito) 100%, tenor maks 1 tahun
    
      - RDN Campuran → tiap kelas aset ≤ 79%

<!-- end list -->

1.  Bila validasi gagal, sistem block upload dan tampilkan error spesifik.

### 5.8.5 Upload MTM Harian (Mark-to-Market Pricing)

Modul Upload MTM Harian menyediakan mekanisme bulk upload harga referensi instrumen untuk perhitungan Mark-to-Market harian. Modul ini mendukung dua mode: (a) Automated Feed — scheduled job pickup file dari IBPA/KSEI/BEI secara otomatis pada hari kerja; (b) Manual Upload — upload UI oleh Akuntansi bila feed otomatis tidak tersedia, terlambat, atau perlu koreksi. Modul ini ditambahkan pada versi 1.3 sebagai pelengkap modul MTM (Bab 5.3) untuk mengatasi gap operasional saat feed eksternal tidak available.

**Spesifikasi file upload MTM Harian:**

| **Aspek**                 | **Spesifikasi**                                                                                                                                     |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Format File               | XLSX (template baku disediakan sistem); fallback CSV untuk integrasi otomatis                                                                       |
| Sumber Data               | (a) IBPA — file feed harian via SFTP/HTTPS download; (b) KSEI / MI — NAB report harian; (c) BEI — closing price harian; (d) Manual upload Akuntansi |
| Frekuensi Upload          | Hari kerja, end-of-day setelah market close (jam 18:00 WIB)                                                                                         |
| Tanggal Valuasi           | \= tanggal hari kerja saat upload; tidak boleh masa depan; backdate hanya untuk koreksi via PERIODE\_ADJUSTMENT                                     |
| Cakupan Instrumen         | OBLIGASI (sumber IBPA), SAHAM (sumber BEI), REKSADANA semua sub-tipe termasuk ETF (sumber NAB MI/KSEI atau BEI untuk ETF)                           |
| Otorisasi (Manual Upload) | Maker (Akuntansi) → Approver (Finance Controller)                                                                                                   |
| Validasi Sistem           | ISIN/Kode terdaftar di Master Instrumen; price reasonability ≤ 5% deviasi vs hari kerja sebelumnya; mata uang konsisten dengan Master Instrumen     |
| Audit Trail               | Versi snapshot harian per instrumen; sumber data; uploader/approver; SHA-256 hash file; status (POSTED/STALE\_PRICE/REJECTED)                       |
| Retensi                   | 10 tahun (sama dengan transaksi terkait)                                                                                                            |

**Template upload MTM Harian (sample 5 baris):**

| **No** | **Tanggal Valuasi** | **Tipe Instrumen** | **ISIN / Kode** | **Sumber Harga** | **Mata Uang** | **Harga (% atau NAB)** | **Yield (%)** | **Volume**    | **Catatan**              |
| ------ | ------------------- | ------------------ | --------------- | ---------------- | ------------- | ---------------------- | ------------- | ------------- | ------------------------ |
| 1      | 2026-05-29          | OBLIGASI           | IDG000016605    | IBPA             | IDR           | 101,5000%              | 5,7500        | —             | ORI023                   |
| 2      | 2026-05-29          | OBLIGASI           | IDA000XXXXXX    | IBPA             | IDR           | 98,7500%               | 6,1250        | —             | PT XYZ Tbk Seri A        |
| 3      | 2026-05-29          | SAHAM              | BBCA            | BEI              | IDR           | 9.525,00               | —             | Closing 9.525 | —                        |
| 4      | 2026-05-29          | REKSADANA          | RDN-0001        | NAB\_MI          | IDR           | 1.262,5000             | —             | MI: Schroder  | RDN PT                   |
| 5      | 2026-05-29          | REKSADANA          | XIIT            | BEI              | IDR           | 543,00                 | —             | Closing ETF   | ETF XIIT (NAB acuan BEI) |

**Validasi otomatis sistem saat upload:**

  - ISIN/Kode Instrumen wajib terdaftar di Master Instrumen dengan status AKTIF. Bila tidak: row di-flag REJECTED dengan error 'INSTRUMEN\_TIDAK\_DITEMUKAN'.

  - Tanggal Valuasi harus hari kerja (cek terhadap kalender libur BI); tidak boleh masa depan; tidak boleh di periode buku CLOSED.

  - Price reasonability check: deviasi harga vs hari kerja sebelumnya ≤ 5%. Bila \> 5%: row di-flag PRICE\_DEVIATION\_HIGH untuk review Akuntansi (tidak auto-reject — bisa override dengan justifikasi).

  - Mata Uang harus sama dengan Master Instrumen.mata\_uang; bila berbeda → REJECTED.

  - Sumber Harga harus sesuai dengan Tipe Instrumen: OBLIGASI → IBPA atau MANUAL; SAHAM → BEI atau MANUAL; REKSADANA → NAB\_MI/NAB\_KSEI/BEI (untuk ETF) atau MANUAL.

  - Untuk instrumen valas: kurs tengah BI pada tanggal valuasi harus tersedia di Master Kurs; bila tidak → REJECTED dengan error 'KURS\_TIDAK\_TERSEDIA'.

  - Duplicate detection: (ISIN, Tanggal Valuasi) UNIQUE — bila sudah ada record POSTED → row di-flag DUPLICATE untuk override approval.

**Workflow Upload MTM (Manual Mode):**

Akuntansi (Maker) login → Modul Upload MTM → klik 'Upload XLSX'.

Pilih file → sistem virus scan + parse → tampilkan preview staging dengan status per row (VALID / WARNING / REJECTED).

Maker review staging; dapat: (a) Submit All (batch) untuk approval; (b) Override warning per row dengan justifikasi; (c) Re-upload setelah perbaikan.

Sistem hitung Δ harga per row (vs harga sebelumnya) sebagai info preview.

Finance Controller (Approver) review batch → Approve All atau Approve dengan exclusions.

Setelah APPROVED: sistem post sebagai trx.mtm records; trigger jurnal MTM\_FVOCI / MTM\_FVTPL per klasifikasi instrumen.

Untuk row REJECTED: tetap di-log di tabel staging dengan reason; tidak menghasilkan trx.mtm record.

Notification ke Akuntansi & Treasury bila ada PRICE\_DEVIATION\_HIGH yang di-approve dengan override.

**Fallback dan Failure Handling:**

  - Job otomatis IBPA/NAB/BEI gagal selama 3 retry (interval 30 menit) → alert Akuntansi via email + dashboard; sistem switch ke mode 'WAITING\_MANUAL\_UPLOAD'.

  - Bila manual upload juga tidak masuk hingga end-of-day (jam 22:00 WIB): sistem mark instrumen tersebut dengan flag STALE\_PRICE dan pakai harga hari kerja terakhir untuk MTM hari ini. Notification ke Akuntansi + CFO.

  - STALE\_PRICE \> 3 hari kerja berturut-turut: escalate ke Treasury Manager + Risk Officer untuk review (possible delisting/suspended trading).

  - Bila file upload corrupt atau format salah: sistem tampilkan error spesifik kolom mana yang invalid; tidak ada partial commit.

### 5.8.6 Upload Master Instrumen — Bulk Import (NEW v1.3)

Modul Upload Master Instrumen Bulk Import menyediakan mekanisme onboarding multiple instrumen sekaligus via Excel template. Digunakan untuk: (a) migrasi data awal dari sistem legacy ke BLIPS saat go-live, (b) bulk onboarding instrumen baru dari hasil tender obligasi primer atau IPO saham, (c) periodic refresh master data dari sumber eksternal. Modul ini menyediakan staging mode dengan preview per-row sebelum commit batch ke production.

**Spesifikasi file upload Master Instrumen Bulk:**

| **Aspek**               | **Spesifikasi**                                                                                                                                                                                           |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Format File             | XLSX (template baku disediakan sistem); satu sheet per Tipe Instrumen (5 sheets: CASH, DEPOSITO, OBLIGASI, SAHAM, REKSADANA)                                                                              |
| Sumber Data             | (a) Spreadsheet legacy Treasury/Risk; (b) Export dari sistem ERP/GL existing; (c) Term sheet/prospektus dari issuer (untuk obligasi primer)                                                               |
| Frekuensi Upload        | Ad-hoc; umumnya saat: inisialisasi sistem (go-live), batch onboarding (mis. hasil tender), refresh master (triwulanan untuk RDN)                                                                          |
| Maximum Rows per Upload | 1.000 instrumen per batch (untuk performance & manageable review)                                                                                                                                         |
| Otorisasi               | Maker (Treasury) → Reviewer (Risk + Akuntansi) → Approver (Treasury Manager untuk instrumen dengan klasifikasi locked existing; Komite Investasi untuk instrumen baru dengan klasifikasi PSAK 71 inisial) |
| Pre-condition           | Counterparty existing untuk semua issuer (atau auto-create dengan workflow approval terpisah); Portofolio target sudah ditentukan; Pefindo PD master ter-update                                           |
| Validasi Sistem         | Field required completeness; FK constraints (counterparty/portofolio/MI/kustodian); business rules (JT \> penempatan, kupon ≥ 0, klasifikasi enum valid)                                                  |
| Staging Mode            | Wajib — semua upload masuk staging area dulu; preview per-row dengan status VALID/WARNING/REJECTED; commit hanya setelah review & approve                                                                 |
| Audit Trail             | Per batch: batch\_id, filename, hash, uploader, timestamp, total\_rows, success\_rows, rejected\_rows, approval workflow log                                                                              |
| Retensi Batch History   | 10 tahun (sama dengan instrumen)                                                                                                                                                                          |

**Template upload Master Instrumen Bulk (sheet OBLIGASI sample — kolom utama):**

| **Kolom**                | **Wajib**   | **Contoh Value**                                      | **Validasi**                                                                             |
| ------------------------ | ----------- | ----------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| row\_number              | Auto        | 1, 2, 3, ...                                          | Auto-increment dalam batch                                                               |
| tipe\_instrumen          | Ya          | OBLIGASI                                              | ENUM 5 nilai (per sheet konsisten)                                                       |
| sub\_tipe                | Ya          | NEGARA / KORPORASI / SUKUK\_NEGARA / SUKUK\_KORPORASI | ENUM per tipe (lihat 5.1.1.a)                                                            |
| nama                     | Ya          | Obligasi PT XYZ Tbk Seri A 2026                       | Min 5 karakter                                                                           |
| isin                     | Conditional | IDA000ABCDEF                                          | 12 alphanumeric; wajib untuk obligasi/saham/RDN                                          |
| counterparty\_kode       | Ya          | CP-0042 (atau Nama Issuer)                            | FK ke Master Counterparty; bila Nama → fuzzy match + manual confirm                      |
| manajer\_investasi\_kode | Conditional | CP-MI-0010                                            | FK MI; wajib untuk RDN                                                                   |
| bank\_kustodian\_kode    | Conditional | CP-BK-0005                                            | FK Bank Kustodian; wajib untuk RDN                                                       |
| portofolio\_kode         | Ya          | PORT-INV-LT                                           | FK ke Master Portofolio                                                                  |
| mata\_uang               | Ya          | IDR                                                   | ISO 4217 dari Master Mata Uang                                                           |
| nominal                  | Ya          | 5000000000.00                                         | \> 0; format 2 desimal                                                                   |
| tanggal\_penempatan      | Ya          | 2026-01-15                                            | Format ISO; ≤ hari ini                                                                   |
| tanggal\_jatuh\_tempo    | Conditional | 2030-12-31                                            | Wajib DEPOSITO/OBLIGASI; \> tanggal\_penempatan                                          |
| kupon                    | Conditional | 5.0000                                                | % pa; ≥ 0; 4 desimal; wajib DEPOSITO/OBLIGASI                                            |
| frekuensi\_bunga         | Conditional | SEMESTERAN                                            | ENUM; wajib DEPOSITO/OBLIGASI                                                            |
| harga\_beli\_persen      | Conditional | 101.5000                                              | % dari par; wajib OBLIGASI                                                               |
| biaya\_transaksi         | Tidak       | 5000000.00                                            | ≥ 0; default 0                                                                           |
| fvoci\_election          | Conditional | FALSE                                                 | BOOLEAN; hanya SAHAM                                                                     |
| sppi\_test\_kode         | Conditional | SPPI-2026-00123                                       | FK ke SPPI Test existing; opsional jika instrumen sudah ada di legacy dengan klasifikasi |
| bm\_test\_kode           | Conditional | BMT-2026-00045                                        | FK ke BM Test existing                                                                   |
| klasifikasi\_psak71      | Conditional | FVOCI                                                 | AC/FVOCI/FVOCI\_ELECTION/FVTPL; wajib bila migration dari legacy                         |
| auto\_renewal\_flag      | Conditional | TRUE                                                  | BOOLEAN; hanya DEPOSITO                                                                  |
| catatan\_legacy          | Tidak       | Migrated from system XYZ                              | Free text untuk audit trail                                                              |

**Validasi staging per row:**

  - REQUIRED fields completeness: semua field 'Ya' harus ada nilai; row dengan missing required → REJECTED.

  - FK validation: counterparty\_kode, portofolio\_kode, mata\_uang, manajer\_investasi\_kode, bank\_kustodian\_kode harus exist di master masing-masing. Bila ada FK yang tidak match → WARNING (suggest auto-create counterparty) atau REJECTED (untuk FK yang strict).

  - Business rule: tanggal\_jatuh\_tempo \> tanggal\_penempatan; kupon ≥ 0; nominal \> 0; harga\_beli\_persen \> 0.

  - Sub-tipe consistency: sub\_tipe harus valid untuk tipe\_instrumen (mis. tipe=OBLIGASI hanya boleh sub\_tipe NEGARA/KORPORASI/SUKUK\_NEGARA/SUKUK\_KORPORASI).

  - Klasifikasi PSAK 71 consistency: bila klasifikasi=AC → BM harus HTC; bila FVOCI → BM HTCS; bila FVTPL → BM Other atau SPPI FAIL. Sistem cross-check terhadap SPPI/BM Test referenced.

  - Duplicate detection: (tipe, sub\_tipe, isin, counterparty) UNIQUE → bila duplicate → WARNING untuk konfirmasi (mungkin top-up, bukan instrumen baru).

  - ISIN format: bila diisi, harus 12 karakter alphanumeric ISO 6166 format.

  - Mata Uang valuta asing: kurs Tengah BI pada tanggal\_penempatan harus tersedia.

**Workflow Bulk Upload:**

Treasury Maker login → Modul Upload Master Instrumen → klik 'Upload XLSX Template'.

Pilih file → sistem virus scan + parse semua 5 sheets → tampilkan summary: total rows, breakdown per Tipe Instrumen.

Sistem run staging validation per row → tampilkan dashboard dengan filter status (VALID / WARNING / REJECTED) per sheet.

Maker review per-row staging; dapat: (a) Edit inline untuk fix WARNING; (b) Re-upload file koreksi; (c) Submit batch untuk approval.

Risk + Akuntansi Reviewer review klasifikasi PSAK 71 (jika ada klasifikasi inisial baru) dan validasi parameter risiko.

Approver — bergantung scope: (a) Treasury Manager untuk top-up instrumen existing dengan klasifikasi locked; (b) Komite Investasi untuk batch dengan klasifikasi PSAK 71 baru atau perubahan material.

Setelah APPROVED: sistem commit batch — INSERT records ke mst.instrumen, lock klasifikasi, generate EIR awal (untuk AC/FVOCI utang), generate amortization schedule.

Untuk rows REJECTED: tetap di-log di staging dengan reason; tidak masuk ke mst.instrumen.

Notification ke seluruh stakeholder + batch summary report (rows success/rejected, total nominal IDR equivalent, breakdown per Tipe).

**Special Modes:**

  - Migration Mode (saat go-live) — Treasury dapat upload dengan klasifikasi\_psak71 yang sudah filled (dari legacy); sistem skip pre-trade clearance flow tetapi tetap require dokumen pendukung migrasi (legacy export + audit trail).

  - Top-Up Mode — bila instrumen sudah exist (match by ISIN+counterparty), sistem treat sebagai penempatan baru pada instrumen existing — buat trx.penempatan record, bukan mst.instrumen baru.

  - Dry Run Mode — Maker dapat trigger staging validation tanpa submit; output: report rows status untuk preview internal sebelum proper upload.

  - Rollback — bila terjadi error sistem material setelah commit batch (mis. FK constraint runtime), CFO dapat trigger rollback batch dengan justifikasi; semua records ditandai REVERSED dengan audit trail link ke batch\_id.

**Audit Trail Batch Upload:**

| **Field Audit**                              | **Description**                               |
| -------------------------------------------- | --------------------------------------------- |
| batch\_id                                    | UUID unique per batch                         |
| batch\_code                                  | Auto-generate (BATCH-INS-{YYYY}-{\#\#\#\#\#}) |
| filename\_original                           | Nama file yang di-upload                      |
| file\_sha256                                 | Hash integrity                                |
| uploaded\_by                                 | FK ke sec.user                                |
| uploaded\_at                                 | Timestamp                                     |
| total\_rows                                  | Total baris di file                           |
| valid\_rows / warning\_rows / rejected\_rows | Status breakdown                              |
| committed\_rows                              | Final yang masuk ke mst.instrumen             |
| mode                                         | MIGRATION / TOPUP / DRY\_RUN / STANDARD       |
| reviewer\_id, approver\_id                   | Workflow actors                               |
| committed\_at                                | Timestamp commit                              |
| rollback\_at, rollback\_by, rollback\_reason | Jika di-rollback                              |

## 5.9 Modul Periode Buku (Financial Period Management)

Modul Periode Buku mengelola siklus hidup setiap periode akuntansi: opening, closing (soft & hard), dan reopening. Modul ini bersifat cross-cutting — setiap modul transaksi (penempatan, MTM, akrual, ECL, jurnal closure) wajib memvalidasi status periode buku dari tanggal transaksi sebelum proses dijalankan.

### 5.9.1 Field Transaksi Stamp Periode

Setiap rekord transaksi (di seluruh modul: penempatan, mutasi, MTM, akrual, ECL, closure, jurnal) wajib menyimpan field stamping berikut:

| **Field**                 | **Tipe Data** | **Wajib** | **Keterangan**                                                                                                               |
| ------------------------- | ------------- | --------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Tanggal Transaksi         | DATE          | Ya        | Tanggal yang relevan secara akuntansi (trade date / value date / akrual date).                                               |
| Periode Bulanan ID        | FK            | Ya        | Auto-derive dari Tanggal Transaksi → lookup Master Periode Buku tipe BULANAN.                                                |
| Periode Triwulanan ID     | FK            | Ya        | Auto-derive.                                                                                                                 |
| Periode Tahunan ID        | FK            | Ya        | Auto-derive.                                                                                                                 |
| Status Periode (snapshot) | ENUM          | Ya        | OPEN | SOFT\_CLOSED | CLOSED — snapshot saat transaksi dibuat. Dipakai untuk validasi.                                       |
| Posting Date              | DATETIME      | Ya        | Tanggal-waktu sistem saat transaksi di-create (boleh berbeda dengan Tanggal Transaksi, mis. backdated entry oleh Akuntansi). |

### 5.9.2 Aturan Validasi Posting Period

1.  Saat input transaksi baru: sistem cek Status Periode dari Tanggal Transaksi. Jika OPEN → lolos. Jika SOFT\_CLOSED dan user bukan Akuntansi → block. Jika CLOSED → block untuk semua user.

2.  Saat edit transaksi yang masih PENDING\_APPROVAL: sistem cek status periode saat ini (bukan saat transaksi dibuat). Jika periode sudah berubah ke SOFT\_CLOSED/CLOSED → block edit.

3.  Saat approve transaksi: sistem cek status periode dari Tanggal Transaksi. Jika sudah CLOSED → block approve, transaksi auto-cancel dan harus diinput ulang pada periode terbuka.

4.  Backdated entry: hanya Akuntansi (Finance Controller) yang dapat input transaksi dengan Tanggal Transaksi pada periode SOFT\_CLOSED, dan wajib dengan Adjustment Reason + dokumen pendukung. Tidak ada user yang bisa backdated entry pada periode CLOSED.

5.  Forward-dated entry: input transaksi dengan Tanggal Transaksi di masa depan diperbolehkan untuk planning (mis. komitmen penempatan future), tetapi posting jurnal ditunda sampai tanggal tersebut tiba.

6.  Job sistem (akrual harian, MTM harian, ECL akhir bulan) hanya berjalan pada periode OPEN. Bila job berjalan ketika periode bulanan sudah SOFT\_CLOSED, output dialokasikan ke periode bulanan berikutnya.

### 5.9.3 Workflow Soft Close Bulanan

1.  H+1 sampai H+5 hari kerja setelah akhir bulan: Treasury menyelesaikan input transaksi tertunda; Akuntansi melakukan rekonsiliasi GL.

2.  H+5: sistem mengirim notifikasi ke Akuntansi untuk soft-close (default; konfigurabel per organisasi).

3.  Akuntansi menjalankan checklist pre-close: rekonsiliasi GL, verifikasi ECL akhir bulan ter-jurnal, semua transaksi PENDING\_APPROVAL sudah disposisi.

4.  Akuntansi (Maker) request soft-close → Finance Controller (Approver) approve → status periode bulanan berubah ke SOFT\_CLOSED.

5.  Pada status SOFT\_CLOSED, Akuntansi masih dapat melakukan adjustment journal entry untuk koreksi material yang teridentifikasi (mis. reklasifikasi salah, koreksi parameter ECL) dengan trail audit.

### 5.9.4 Workflow Hard Close Bulanan

1.  Maksimal H+15 hari kerja setelah akhir bulan: Akuntansi melakukan hard-close.

2.  Pre-condition: tidak ada adjustment yang sedang PENDING\_APPROVAL; laporan keuangan bulanan sudah ter-finalisasi; tidak ada selisih rekonsiliasi yang material.

3.  Akuntansi (Maker) request hard-close → CFO (Approver) approve → status periode bulanan berubah ke CLOSED.

4.  Setelah CLOSED, periode bulanan tidak dapat diubah dengan cara apapun. Koreksi error yang teridentifikasi setelah closing harus dilakukan via prior-period adjustment journal entry pada periode terbuka berikutnya (mengikuti PSAK 25).

### 5.9.5 Workflow Closing Triwulanan & Tahunan

1.  Periode TRIWULANAN soft-close otomatis ketika 3 periode bulanan dalam triwulan tersebut sudah CLOSED.

2.  Periode TRIWULANAN hard-close memerlukan tambahan: laporan triwulanan ter-finalisasi (untuk perusahaan publik: laporan ke OJK).

3.  Periode TAHUNAN soft-close otomatis ketika 12 periode bulanan dan 4 periode triwulanan sudah CLOSED.

4.  Periode TAHUNAN hard-close memerlukan: audit eksternal selesai, laporan keuangan tahunan ter-tanda tangani, RUPS persetujuan.

### 5.9.6 Workflow Reopen Periode (Exceptional)

Reopen periode CLOSED hanya dapat dilakukan dalam kondisi exceptional, mis. ditemukan error material yang tidak dapat dikoreksi via prior-period adjustment, atau ada kewajiban regulasi untuk restate. Reopen mengganggu integritas laporan keuangan yang sudah disampaikan, sehingga harus dijaga ketat.

1.  Hanya CFO yang dapat memicu reopen request, dengan disertai memo tertulis + persetujuan auditor (bila tahunan).

2.  Reopen request → CEO (atau Komite Audit untuk perusahaan publik) approval → sistem mengubah status CLOSED → OPEN.

3.  Reopened Flag pada Master Periode Buku otomatis ter-set Y, dan seluruh adjustment yang dilakukan pada periode reopened ditandai khusus (Flag: REOPENED\_ADJUSTMENT) untuk audit trail dan disclosure.

4.  Setelah koreksi selesai, Akuntansi wajib menjalankan workflow soft-close → hard-close ulang pada periode tersebut.

5.  Bila periode yang di-reopen adalah periode tahunan yang sudah laporan keuangan-nya disampaikan ke regulator, organisasi wajib mengajukan restated financial statement.

### 5.9.7 Reporting & Dashboard Periode Buku

1.  Dashboard "Status Periode" menampilkan timeline 12 bulan terakhir + 12 bulan ke depan, dengan kode warna: hijau (OPEN), kuning (SOFT\_CLOSED), abu-abu (CLOSED), merah (REOPENED).

2.  Notifikasi otomatis: H-3 sebelum tenggat soft-close (default H+5), H-3 sebelum tenggat hard-close (default H+15), dan pada saat status berubah.

3.  Laporan Closing Audit Trail: per periode, daftar semua transaksi adjustment yang dibuat pada status SOFT\_CLOSED + reopened adjustment, dengan maker/approver/reason.

4.  Filter periode tersedia di seluruh laporan (Bab 10.3): Saldo CKPN, Stage Distribution, Roll-Forward, Mutasi Instrumen, P\&L Investasi, dst — semua dapat di-cut per periode bulanan/triwulanan/tahunan.

## 5.10 Modul FX Rate Management

Modul FX Rate Management mengelola siklus hidup kurs harian: ingestion (otomatis dari BI atau upload manual), validasi, lock saat closing, dan distribusi ke seluruh modul yang membutuhkan konversi mata uang. Modul ini bekerja erat dengan Modul Periode Buku (Bab 5.9) untuk memastikan kurs di periode CLOSED tidak dapat diubah.

### 5.10.1 Sumber Kurs & Mekanisme Update

| **Sumber Kurs**              | **Mekanisme**                                                                                                                                                                                    |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **BI JISDOR (USD)**          | Auto-fetch dari publikasi BI setiap hari kerja jam 10:30 WIB. Sistem connect ke API/halaman BI, parsing kurs, dan tersimpan dengan Sumber = BI\_JISDOR. Locked Flag = N (sampai periode CLOSED). |
| **BI Kurs Tengah (Non-USD)** | Auto-fetch untuk SGD, EUR, JPY, AUD, CNY, MYR, GBP, dll. dari publikasi BI setiap hari kerja jam 10:30 WIB.                                                                                      |
| **Upload Manual**            | Untuk kasus khusus: backdated rate, mata uang non-aktif, atau koreksi. Wajib Maker (Akuntansi) → Approver (Finance Controller) + dokumen bukti screenshot/PDF publikasi BI.                      |
| **Repeat Rate (Hari Libur)** | Sistem otomatis menggunakan kurs hari kerja terakhir bila pada hari libur/cuti bersama tidak ada publikasi kurs baru. Ter-flag REPEAT\_RATE untuk audit trail.                                   |

### 5.10.2 Workflow Upload Manual Kurs

1.  Akuntansi (Maker) input kurs baru: pilih mata uang, tanggal berlaku, kurs tengah, sumber referensi.

2.  Sistem validasi: tanggal tidak di periode CLOSED, kombinasi (mata uang × tanggal) belum ada (atau ada tetapi belum locked).

3.  Maker upload dokumen bukti (PDF/screenshot publikasi BI atau memo internal untuk kurs khusus).

4.  Approver (Finance Controller) review → Approve → kurs aktif.

5.  Bila kurs di-upload untuk tanggal yang sudah punya kurs (overwrite): sistem auto-create record baru dengan flag REVISED, kurs lama ditandai SUPERSEDED untuk audit trail.

### 5.10.3 Validasi Periode pada Upload/Edit Kurs

1.  Tanggal Berlaku pada periode OPEN: maker dapat input/edit tanpa restriction.

2.  Tanggal Berlaku pada periode SOFT\_CLOSED: hanya Akuntansi yang bisa upload, dengan trail Maker-Approver dan adjustment reason.

3.  Tanggal Berlaku pada periode CLOSED: TIDAK BISA. Koreksi harus via prior-period FX adjustment journal entry pada periode terbuka berikutnya.

4.  Saat periode bulanan transition ke HARD CLOSED: sistem otomatis set Locked Flag = Y untuk seluruh kurs di periode tersebut.

### 5.10.4 Job Sistem yang Terdampak Kurs

Setiap job sistem yang berinteraksi dengan instrumen valas wajib lookup kurs dari Master FX Rate History sesuai tanggal eventnya:

1.  Job Akrual Bunga Harian: lookup kurs hari ini, hitung akrual valas → konversi ke IDR pada kurs hari ini → posting jurnal IDR + record bunga valas asli.

2.  Job MTM Harian: lookup kurs hari ini, MTM mata uang asli × kurs = MTM IDR → posting selisih ke OCI/P\&L sesuai klasifikasi.

3.  Job Hitung ECL Akhir Bulan: lookup kurs tanggal akhir bulan, EAD valas × kurs = EAD IDR → ECL = EAD IDR × PD × LGD × Impact PD (semua dalam IDR).

4.  Job Closing Period: snapshot kurs akhir periode dipakai untuk laporan komparatif period-over-period.

### 5.10.5 Reporting & Dashboard FX

1.  Dashboard "Posisi Valas" menampilkan total exposure per mata uang dalam mata uang asli dan IDR equivalent, dengan trend kurs harian.

2.  Laporan FX Gain/Loss: per periode, breakdown unrealized vs realized, per mata uang, per instrumen, dengan jurnal posting reference.

3.  Laporan FX Rate History: tabel kurs harian per mata uang dengan source, locked status, dan revision history.

4.  Notifikasi otomatis bila ada hari kerja tanpa kurs ter-upload (alert untuk Akuntansi pada jam 11:00 WIB jika kurs hari itu belum masuk).

## 5.11 Modul Mapping Jurnal \[Phase 1: Standalone mode — jurnal di-export ke file CSV/XLSX untuk upload manual ke GL Host. Integrasi API ke GL Host di-defer ke Phase 2. Lihat Decision Log DEC-005.\]

Modul Mapping Jurnal mengelola siklus hidup mapping event-jurnal: setup awal, integrasi Chart of Accounts, edit mapping, export/import bulk via Excel, dan resolusi runtime saat event terpicu. Modul ini bekerja erat dengan Master CoA (Bab 5.1.9) dan Master Mapping Jurnal Header-Detail (Bab 5.1.10).

### 5.11.1 Setup Awal & Integrasi Chart of Accounts

Saat implementasi pertama, sistem menyediakan default mapping untuk seluruh 17 event mapping yang ter-cover. Akuntansi menyesuaikan kode akun ke struktur CoA spesifik organisasi. Integrasi CoA dapat dilakukan via dua jalur:

1.  Import Excel CoA: Akuntansi download template, isi struktur CoA organisasi, upload kembali. Sistem validasi format dan import ke Master CoA. Sumber CoA = IMPORT\_EXCEL.

2.  Integrasi API ke sistem ERP/GL: bila organisasi memiliki sistem ERP eksternal (SAP, Oracle, dll), CoA dapat di-sync via API/scheduled job. Sumber CoA = IMPORT\_ERP.

3.  Setelah CoA tersedia, Akuntansi mapping default (provided by system) ke kode akun spesifik di CoA organisasi. Tools: UI mapping editor + Excel export/import.

### 5.11.2 Edit Mapping (UI Editor)

Akuntansi dapat edit mapping via UI dengan flow:

1.  Pilih event dari daftar (mis. PENEMPATAN). Sistem tampilkan header + tabel detail.

2.  Tambah/edit/hapus line detail. Untuk setiap line, pilih Kode Akun (auto-suggest dari CoA aktif), tetapkan D/K, Sumber Amount, filter Klasifikasi/Tipe Instrumen, dan Multiplier.

3.  Sistem validasi real-time:
    
      - Kode Akun valid di CoA dan Aktif Flag = Y
    
      - Kombinasi (Klasifikasi × Tipe Instrumen × Underlying Type) untuk setiap urutan tidak duplikat
    
      - Sumber Amount sesuai dengan kategori event (mis. EVENT AKRUAL hanya boleh BUNGA\_AKRUAL\_IDR atau KUPON\_AKRUAL\_IDR)
    
      - Untuk setiap kombinasi filter yang valid, total Multiplier × D harus sama dengan total Multiplier × K (untuk balance)

<!-- end list -->

1.  Save mapping → langsung aktif (no approval workflow).

2.  Audit trail: setiap perubahan mapping tercatat dengan user, timestamp, before-after value.

### 5.11.3 Export / Import Excel (Bulk Update)

Untuk update massal atau setup awal yang lebih cepat, mapping dapat di-export dan di-import via Excel.

**Format file Export Mapping (XLSX, 2 sheet):**

| **Sheet**            | **Konten**                                                                                                                                                                                                                                 |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Sheet 1: Events**  | Header semua event: Event ID, Event Code, Nama Event, Kategori, Trigger Source, Tipe Instrumen Berlaku, Klasifikasi Berlaku, Aktif Flag, Catatan.                                                                                          |
| **Sheet 2: Details** | Seluruh line detail: Detail ID, Event ID (FK), Urutan, Kode Akun, Nama Akun (auto-fill dari CoA), D/K Indicator, Sumber Amount, Klasifikasi Filter, Tipe Instrumen Filter, Underlying Type Filter, Multiplier, Mata Uang Posting, Catatan. |

**Workflow Import:**

1.  Akuntansi download template export current state.

2.  Edit di Excel (tambah/edit/hapus row di Sheet 1 dan/atau Sheet 2).

3.  Upload kembali. Sistem validasi:
    
      - Format header sheet sesuai template
    
      - Setiap Detail.Event ID memiliki entry di Sheet 1 Events
    
      - Setiap Kode Akun ada di Master CoA dengan Aktif Flag = Y
    
      - Setiap Sumber Amount valid (enum check)
    
      - Filter Klasifikasi/Tipe Instrumen valid
    
      - Untuk tiap event, balance check (total Debit = total Kredit per filter combination)

<!-- end list -->

1.  Bila validasi pass: sistem replace mapping aktif dengan content dari upload (full replace, bukan delta merge — untuk safety).

2.  Bila validasi fail: sistem block import, tampilkan error report dengan baris bermasalah.

3.  Audit trail: import event tercatat dengan user, timestamp, jumlah event/detail yang ter-update, dan file backup.

**Use case bulk update yang umum:**

1.  Migrasi awal dari sistem lama: export template kosong → fill all events dari kebijakan akuntansi → import.

2.  Restructuring CoA: setelah CoA di-restructure (mis. konsolidasi sub-akun), update mapping seluruh event yang terdampak via export-edit-import.

3.  Update kebijakan tax: bila tarif PPh kupon/dividen berubah, edit Multiplier di line terkait via Excel.

### 5.11.4 Resolusi Runtime saat Event Terpicu

Setiap kali event terpicu (oleh user input atau system job), sistem mengikuti algoritma resolusi 6-langkah untuk membentuk journal entry actual:

1.  Sistem identifikasi Event Code dari trigger (mis. PENEMPATAN, AKRUAL\_BUNGA, MTM\_FVOCI).

2.  Sistem ambil header event dari Master Mapping Jurnal — pastikan Aktif Flag = Y.

3.  Sistem ambil semua line detail di event tersebut yang Aktif Flag = Y, sorted by Urutan.

4.  Untuk tiap line, sistem evaluasi filter terhadap atribut instrumen yang sedang diproses:
    
      - Klasifikasi Filter — match dengan klasifikasi PSAK 71 instrumen
    
      - Tipe Instrumen Filter — match dengan tipe (DEPOSITO/OBLIGASI/dst)
    
      - Underlying Type Filter — untuk reksadana look-through

<!-- end list -->

1.  Hanya line yang lulus semua filter yang dipakai. Untuk line yang lulus: sistem ambil Sumber Amount dari instrumen × Multiplier = Amount IDR final.

2.  Sistem assemble journal entry final dengan posting tanggal sesuai event timestamp + periode buku ID (Bab 5.9). Validasi balance (total Debit = total Kredit). Bila balance: post ke GL. Bila tidak balance: block + alert ke Akuntansi (incident reporting).

### 5.11.5 Reporting & Dashboard Mapping Jurnal

1.  Dashboard "Mapping Coverage" — daftar event dengan status Aktif/Tidak Aktif dan jumlah line detail per event; warning bila ada event dengan 0 line detail (mapping incomplete).

2.  Laporan Mapping Validation — daftar event yang gagal balance check pada saat resolusi runtime di periode tertentu, dengan trace amount dan instrumen terkait.

3.  Laporan Mapping Change History — audit trail seluruh perubahan mapping (UI edit + Excel import) dengan user, timestamp, before-after value, dan tipe operasi (CREATE | UPDATE | DELETE | IMPORT).

4.  Cross-reference Report — untuk setiap Kode Akun di CoA, daftar event mapping yang menggunakannya. Berguna saat CoA dilakukan restructuring.

## 5.12 Modul EIR & Amortisasi

Modul EIR (Effective Interest Rate / Suku Bunga Efektif) dan Amortisasi merupakan engine subsequent measurement untuk aset keuangan yang diukur pada Amortized Cost (AC) dan Fair Value through OCI (FVOCI) untuk instrumen utang. Modul ini melengkapi modul ECL dengan perhitungan pendapatan bunga berbasis EIR sesuai PSAK 71 paragraf 5.4 dan Lampiran A, serta menghasilkan Amortization Schedule yang menjadi sumber jurnal akrual harian dan amortisasi premium/diskonto. Modul ini ditambahkan pada versi 1.1 untuk memenuhi gap subsequent measurement di v1.0.

### 5.12.1 Definisi & Konsep

EIR (Effective Interest Rate) adalah tingkat diskonto yang menyamakan nilai kini (present value) seluruh estimasi arus kas masa depan kontraktual sepanjang umur instrumen dengan carrying amount awal pada saat pengakuan. Carrying amount awal mencakup harga beli, biaya transaksi yang dikapitalisasi, serta premium atau diskonto bawaan. EIR ditetapkan sekali pada pengakuan awal dan tetap konstan sepanjang umur instrumen, kecuali terjadi modifikasi material atau revisi estimasi cash flow yang memicu re-estimation.

Effective Interest Method adalah metode pengakuan pendapatan bunga di mana pendapatan = Carrying Amount × EIR. Untuk Stage 1 dan Stage 2, basis perhitungan adalah Gross Carrying Amount (sebelum CKPN). Untuk Stage 3 (credit-impaired), basis berubah menjadi Net Carrying Amount (post-CKPN) sesuai PSAK 71 paragraf 5.4.1(b). Selisih antara pendapatan bunga EIR-based dengan kupon kontraktual nominal merupakan amortisasi premium (bila harga beli \> par) atau amortisasi diskonto (bila harga beli \< par) yang dibukukan untuk menyesuaikan carrying amount sehingga tepat sama dengan nilai par pada saat jatuh tempo.

### 5.12.2 Cakupan Instrumen

Modul EIR & Amortisasi WAJIB diterapkan untuk klasifikasi:

  - Cash di Bank (AC) — bila rate kontraktual variabel harian, EIR Method Flag = N (simple interest); bila rate tetap, EIR Method Flag = Y.

  - Deposito Berjangka (AC) — EIR umumnya ≈ kupon nominal untuk plain vanilla; berbeda bila terdapat biaya transaksi atau premium/diskonto.

  - Obligasi (AC) — Held to Maturity. EIR mencakup amortisasi premium/diskonto dan biaya transaksi.

  - Obligasi (FVOCI) — Held to Collect & Sell. Pendapatan bunga tetap berbasis EIR meskipun MTM ke OCI.

Modul TIDAK berlaku untuk:

  - FVTPL (saham, reksadana, obligasi trading) — pengukuran penuh fair value, tidak ada amortisasi.

  - Saham FVOCI Election — instrumen ekuitas tanpa bunga kontraktual.

  - Reksadana (semua klasifikasi) — distribusi sesuai cash distribution dari MI, bukan bunga kontraktual.

### 5.12.3 Formula EIR

**EIR didefinisikan sebagai r yang memenuhi persamaan present value:**

*P0 = Σ \[ CFt / (1 + r)^t \] untuk t = 1, 2, ..., T*

Di mana:

  - P0 = Carrying Amount Awal = Harga Beli + Biaya Transaksi Kapitalisasi (untuk AC/FVOCI utang).

  - CFt = Estimasi arus kas kontraktual pada periode t (kupon, pelunasan pokok, dll).

  - T = Tenor sisa kontrak dalam jumlah periode kupon.

  - r = EIR per periode kupon. Annualized EIR = (1 + r)^f − 1, di mana f = frekuensi kupon per tahun.

Untuk obligasi dengan kupon tetap c (per annum), frekuensi f kali per tahun, nominal N, harga beli P (kotor termasuk biaya transaksi kapitalisasi):

  - CFt = (c × N) / f untuk t = 1 sampai T-1 (kupon periodik)

  - CFT = (c × N) / f + N untuk t = T (kupon final + pelunasan pokok)

  - Solver mencari r (per periode) yang memenuhi P = Σ CFt/(1+r)^t.

  - EIR Annualized = (1 + r)^f − 1, dilaporkan dalam 4 desimal.

### 5.12.4 Algoritma IRR Solver (Newton-Raphson)

Sistem menggunakan metode Newton-Raphson untuk menyelesaikan IRR karena cepat konvergen untuk pola cash flow normal obligasi. Algoritma:

  - Initial guess: r0 = kupon kontraktual / f (asumsi awal).

  - Iterasi: rk+1 = rk − f(rk) / f'(rk).

  - f(r) = P0 − Σ CFt / (1+r)^t.

  - f'(r) = Σ t × CFt / (1+r)^(t+1).

  - Konvergensi: |f(rk+1)| \< 0,00000001 (toleransi 8 desimal internal).

  - Maksimum 50 iterasi. Bila tidak konvergen, sistem raise exception 'EIR\_NOT\_CONVERGED' untuk review manual oleh Akuntansi/Risk.

  - Fallback: bila Newton-Raphson gagal (mis. cash flow non-konvensional), sistem switch ke metode bisection dengan range r ∈ \[-0,99 ; 1,00\].

Sistem menyimpan EIR internal dengan presisi 8 angka desimal (NUMERIC(12,8)) dan menampilkan 4 desimal di laporan. Akurasi 4 desimal pada rasio sesuai standar presisi sistem (Bab 7.3).

### 5.12.5 Treatment Biaya Transaksi & Premium/Diskonto

| **Klasifikasi**        | **Biaya Transaksi**                                           | **Premium / Diskonto**                                      | **Pendapatan Bunga**                        |
| ---------------------- | ------------------------------------------------------------- | ----------------------------------------------------------- | ------------------------------------------- |
| AC                     | Dikapitalisasi ke Carrying Amount Awal; diamortisasi via EIR. | Diamortisasi via EIR sepanjang umur instrumen.              | Carrying × EIR (Gross/Net per Stage).       |
| FVOCI Utang            | Dikapitalisasi ke Carrying Amount Awal; diamortisasi via EIR. | Diamortisasi via EIR; selisih dengan FV masuk OCI.          | Carrying × EIR; MTM selisih ke OCI.         |
| FVTPL                  | Langsung dibebankan ke P\&L pada saat penempatan.             | Tidak relevan — pengukuran penuh fair value setiap periode. | Tidak ada akrual EIR; perubahan FV ke P\&L. |
| FVOCI Election (Saham) | Dikapitalisasi ke Carrying.                                   | Tidak relevan — instrumen ekuitas.                          | Dividen ke P\&L (bukan EIR-based).          |

*Catatan implementasi: untuk obligasi FVOCI utang, dua mekanisme bekerja paralel — (a) carrying amount untuk pendapatan bunga di-track via amortization schedule berbasis EIR; (b) carrying amount untuk MTM ke OCI = nilai wajar IBPA. Selisih antara amortized carrying dan fair value merupakan komponen OCI cumulative yang akan di-recycle ke P\&L saat penjualan/jatuh tempo.*

### 5.12.6 Field Tabel Amortization Schedule

Sistem mengelola tabel Amortization Schedule per instrumen sebagai sumber tunggal kebenaran untuk akrual bunga harian dan amortisasi premium/diskonto. Setiap baris merepresentasikan satu periode amortisasi (umumnya per kupon, atau harian untuk granularitas penuh).

| **Field**                         | **Tipe Data** | **Wajib** | **Keterangan**                                                                                                                                        |
| --------------------------------- | ------------- | --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Schedule ID                       | VARCHAR(20)   | Ya        | Auto-generate (mis. AMSCH-OBL-0001-2026-001).                                                                                                         |
| Kode Instrumen                    | FK            | Ya        | Reference ke Master Instrumen.                                                                                                                        |
| Periode                           | INT           | Ya        | Urutan periode (1, 2, 3, ...).                                                                                                                        |
| Tanggal Posting                   | DATE          | Ya        | Tanggal amortisasi periode tersebut.                                                                                                                  |
| Opening Carrying (IDR)            | NUMERIC(20,2) | Ya        | Carrying amount awal periode = Closing Carrying periode sebelumnya.                                                                                   |
| Cash Inflow (Kupon Kontraktual)   | NUMERIC(20,2) | Ya        | Kupon nominal yang diterima/akan diterima dalam mata uang asli; dikonversi ke IDR via kurs Tengah BI.                                                 |
| Pendapatan Bunga EIR (IDR)        | NUMERIC(20,2) | Ya        | Auto-calc: Opening Carrying × (EIR ÷ frekuensi) × (jumlah hari dalam periode ÷ jumlah hari basis tahunan). Untuk Stage 3 berbasis Net Carrying.       |
| Amortisasi Premium/Diskonto (IDR) | NUMERIC(20,2) | Ya        | Auto-calc: Pendapatan Bunga EIR − Cash Inflow. Positif = amortisasi diskonto (menambah carrying); negatif = amortisasi premium (mengurangi carrying). |
| Pelunasan Pokok (IDR)             | NUMERIC(20,2) | Tidak     | Untuk obligasi amortizing atau saat jatuh tempo. Default 0.                                                                                           |
| Closing Carrying (IDR)            | NUMERIC(20,2) | Ya        | Auto-calc: Opening + Amortisasi P/D − Pelunasan Pokok.                                                                                                |
| EIR Periode Ini                   | NUMERIC(12,8) | Ya        | EIR yang berlaku pada periode ini (dapat berubah karena re-estimation).                                                                               |
| Stage Saat Posting                | ENUM          | Ya        | STAGE\_1 | STAGE\_2 | STAGE\_3. Untuk Stage 3 basis perhitungan beralih ke Net Carrying.                                                              |
| Status Posting                    | ENUM          | Ya        | PROYEKSI | POSTED | REVERSED | RECOMPUTED.                                                                                                            |
| Jurnal Reference                  | FK            | Tidak     | Reference ke jurnal AKRUAL\_BUNGA + AMORTISASI\_PREMI\_DISKONTO yang sudah ter-posting.                                                               |

**Aturan integritas Amortization Schedule:**

  - Schedule digenerate otomatis saat penempatan instrumen baru (event PENEMPATAN), berdasarkan term sheet & EIR yang dihitung modul.

  - Closing Carrying pada baris terakhir (jatuh tempo) HARUS sama dengan nilai par + kupon final, dengan toleransi pembulatan ±0,01 IDR (round-half-to-even).

  - Status PROYEKSI berlaku untuk baris di masa depan; saat tanggal posting tercapai dan job amortisasi berjalan, status berubah menjadi POSTED.

  - Bila terjadi re-estimation EIR (Bab 5.12.8), seluruh baris berstatus PROYEKSI di-recompute (status RECOMPUTED) tanpa membatalkan baris POSTED. Catch-up adjustment dibukukan pada periode aktif.

  - Schedule tidak dapat dihapus; hanya bisa di-deactivate saat instrumen berstatus DICAIRKAN/JATUH\_TEMPO/DIJUAL — audit trail tetap utuh.

### 5.12.7 Akrual Bunga Harian Berbasis EIR

Untuk granularitas akuntansi harian, sistem menjalankan job 'Akrual Bunga EIR' setiap end-of-day yang membentuk jurnal harian. Formula:

*Akrual Harian (IDR) = Carrying Bruto × EIR × (1 ÷ Hari Basis Tahunan)*

Hari Basis Tahunan default = 365 (actual/365). Sub-bab dapat di-override per instrumen via field Day Count Convention di Master Instrumen (default ACT/365; alternatif 30/360 untuk instrumen tertentu).

| **Komponen**                | **Treatment Akrual Harian**                                                                                             |
| --------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Pendapatan Bunga (P\&L)     | Carrying Bruto × EIR ÷ 365 (Stage 1, 2). Net Carrying × EIR ÷ 365 (Stage 3).                                            |
| Akrual Bunga (kontra-aset)  | Sama dengan Pendapatan Bunga. Akumulasi sampai cash inflow kupon, lalu di-clear.                                        |
| Amortisasi Premium/Diskonto | (Pendapatan Bunga EIR Harian) − (Kupon Kontraktual Harian = c × N ÷ 365). Selisih dibukukan ke kontra-akun investasi.   |
| FX Akrual (untuk valas)     | Akrual mata uang asli × Kurs Tengah BI hari ini (lihat Bab 5.1.8). Selisih kurs ke akun Selisih Kurs Belum Direalisasi. |

### 5.12.8 Re-estimation EIR

EIR pada prinsipnya konstan, namun PSAK 71 mengatur dua kondisi di mana EIR perlu direvisi:

**A. Modifikasi Material (PSAK 71 paragraf 3.3.2):**

  - Modifikasi kontrak yang substansial — perubahan kupon ≥ 10%, perpanjangan tenor signifikan, perubahan currency, atau perubahan counterparty — diperlakukan sebagai derecognition aset asli + recognition aset baru pada nilai wajar.

  - EIR aset baru dihitung ulang dari awal berdasarkan term yang baru. Selisih antara nilai tercatat aset asli dan nilai wajar aset baru diakui sebagai realized gain/loss di P\&L (untuk AC) atau di OCI (untuk FVOCI).

  - Event jurnal: MODIFIKASI\_MATERIAL (Bab 5.1.10).

**B. Revisi Estimasi Cash Flow (PSAK 71 paragraf B5.4.6):**

  - Bila estimasi cash flow di masa depan berubah (mis. revisi prepayment assumption, revisi step-up rate trigger), tetapi modifikasi tidak material, EIR original tetap dipakai.

  - Recompute carrying amount = Σ (revised CFt / (1 + EIR\_original)^t). Selisih dengan carrying lama diakui sebagai catch-up adjustment di P\&L (event EIR\_REESTIMATION).

  - Amortization Schedule diregenerate dari periode aktif ke depan; baris POSTED tidak diubah.

**Workflow Re-estimation:**

  - Trigger: input modifikasi kontrak oleh Treasury Maker, atau revisi assumption oleh Risk Officer dengan dokumen pendukung.

  - Sistem auto-calc delta EIR dan delta Carrying. Bila modifikasi material → block input transaksi → escalate ke Komite Investasi untuk approve derecognition + recognition baru.

  - Bila non-material (revisi cash flow saja) → sistem post EIR\_REESTIMATION journal + regenerate schedule. Approval Akuntansi (Maker) → Finance Controller (Approver).

  - Audit trail wajib: dokumen amandemen, memo justifikasi, before/after EIR, before/after carrying, jurnal reference.

### 5.12.9 EIR untuk Stage 3 (Net Carrying Method)

Saat instrumen bermigrasi ke Stage 3 (credit-impaired) sesuai Bab 8.5.2-B, basis perhitungan pendapatan bunga berubah dari Gross Carrying (sebelum CKPN) menjadi Net Carrying (post-CKPN). Ini adalah requirement spesifik PSAK 71 paragraf 5.4.1(b) untuk menghindari overstating pendapatan bunga pada aset yang sudah credit-impaired.

**Algoritma Stage 3:**

  - Pada saat migrasi Stage 2 → Stage 3, sistem menyimpan snapshot Gross Carrying dan Net Carrying.

  - EIR yang dipakai untuk akrual periode berjalan tetap EIR original (tidak di-recompute karena Stage migration bukan modifikasi material).

  - Akrual harian berikutnya: Net Carrying × EIR ÷ 365.

  - Selisih antara hasil EIR-Gross (yang seharusnya bila tetap Stage 1/2) dan EIR-Net dibukukan sebagai bagian dari beban CKPN incremental — TIDAK diakui sebagai pengurang pendapatan bunga.

  - Bila terjadi curing Stage 3 → Stage 2 (Bab 8.5.2-C), basis kembali ke Gross Carrying secara prospektif.

### 5.12.10 EIR pada Reklasifikasi PSAK 71

Saat terjadi reklasifikasi prospektif (Bab 4.5), penanganan EIR mengikuti tabel berikut:

| **Dari → Ke** | **Treatment EIR & Schedule**                                                                                                                                       |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| AC → FVOCI    | EIR original tetap dipakai untuk akrual bunga; selisih FV vs amortized carrying ke OCI sejak tanggal reklasifikasi. Schedule tetap aktif.                          |
| AC → FVTPL    | EIR & Schedule di-deactivate. Pendapatan bunga selanjutnya tidak EIR-based; perubahan FV ke P\&L.                                                                  |
| FVOCI → AC    | EIR baru dihitung ulang dari Fair Value pada tanggal reklasifikasi sebagai carrying baru, sisa cash flow kontraktual; akumulasi OCI dieliminasi terhadap carrying. |
| FVOCI → FVTPL | EIR & Schedule di-deactivate; akumulasi OCI direklas ke P\&L.                                                                                                      |
| FVTPL → AC    | EIR baru dihitung dari Fair Value pada tanggal reklasifikasi sebagai carrying baru, sisa cash flow kontraktual. Schedule baru digenerate.                          |
| FVTPL → FVOCI | Sama seperti FVTPL → AC: EIR baru dihitung dari FV; schedule baru digenerate; mulai akumulasi OCI = 0.                                                             |

### 5.12.11 Contoh Numerik: Obligasi AC dengan Premium

**Data input:**

| **Parameter**                | **Nilai**                           |
| ---------------------------- | ----------------------------------- |
| Obligasi                     | PT XYZ Tbk (rating idA-)            |
| Nilai Nominal (Par)          | Rp 5.000.000.000                    |
| Harga Beli (% dari par)      | 101,5000% = Rp 5.075.000.000        |
| Biaya Transaksi (broker fee) | Rp 5.000.000                        |
| Carrying Amount Awal (P0)    | Rp 5.080.000.000                    |
| Kupon Kontraktual            | 5,00% per annum, dibayar semesteran |
| Tenor                        | 5 tahun (10 periode kupon)          |
| Klasifikasi PSAK 71          | AC (Held to Maturity)               |
| Tanggal Penempatan           | 01/01/2026                          |

**Perhitungan EIR (Newton-Raphson):**

Cash Flow per periode (semester): CFt = (5,00% × 5.000.000.000) ÷ 2 = Rp 125.000.000 untuk t=1..9; CF10 = 125.000.000 + 5.000.000.000 = Rp 5.125.000.000.

Solver mencari r (per semester) yang memenuhi: 5.080.000.000 = Σ \[125.000.000/(1+r)^t\] (t=1..9) + 5.125.000.000/(1+r)^10.

Hasil iterasi (konvergen di iterasi ke-4): r\_semester = 0,02384921, EIR Annualized = (1 + 0,02384921)² − 1 = 0,04826688 ≈ 4,8267%.

**Sample Amortization Schedule (3 periode pertama):**

| **Periode** | **Tanggal** | **Opening Carrying** | **Kupon Kontraktual (Cash)** | **Pendapatan Bunga EIR** | **Amortisasi Premium** | **Closing Carrying** |
| ----------- | ----------- | -------------------- | ---------------------------- | ------------------------ | ---------------------- | -------------------- |
| 1           | 30/06/2026  | 5.080.000.000        | 125.000.000                  | 121.154.001              | (3.845.999)            | 5.076.154.001        |
| 2           | 31/12/2026  | 5.076.154.001        | 125.000.000                  | 121.062.286              | (3.937.714)            | 5.072.216.287        |
| 3           | 30/06/2027  | 5.072.216.287        | 125.000.000                  | 120.968.385              | (4.031.615)            | 5.068.184.672        |
| ...         | ...         | ...                  | ...                          | ...                      | ...                    | ...                  |
| 10 (JT)     | 31/12/2030  | 5.004.762.137        | 5.125.000.000                | 120.237.863              | (4.762.137)            | 0 (par dilunasi)     |

*Catatan: Amortisasi premium negatif (= mengurangi carrying) karena instrumen dibeli di atas par. Pada saat jatuh tempo (periode 10), Closing Carrying ≈ 0 setelah dikurangi pelunasan pokok par Rp 5.000.000.000, dengan toleransi pembulatan 4 desimal pada rasio.*

**Jurnal periode 1 (30/06/2026):**

| **Tanggal** | **Akun**                                     | **Debit**   | **Kredit**  |
| ----------- | -------------------------------------------- | ----------- | ----------- |
| 30/06/2026  | Kas / Bank (90% netto setelah PPh kupon 10%) | 112.500.000 |             |
| 30/06/2026  | Beban PPh Kupon (10% × 125.000.000)          | 12.500.000  |             |
| 30/06/2026  | Pendapatan Bunga Obligasi (EIR)              |             | 121.154.001 |
| 30/06/2026  | Investasi Obligasi AC (amortisasi premium)   |             | 3.845.999   |

*Catatan jurnal: Total Debit Rp 125.000.000 = Total Kredit (121.154.001 + 3.845.999) — balance. Pendapatan bunga yang diakui di P\&L Rp 121.154.001 (lebih rendah dari kupon nominal 125 juta) karena premium diamortisasi sebagai pengurang pendapatan. Carrying Amount turun Rp 3.845.999.*

### 5.12.12 Contoh Numerik: Obligasi FVOCI dengan Diskonto

**Data input:**

| **Parameter**             | **Nilai**                              |
| ------------------------- | -------------------------------------- |
| Obligasi                  | Sukuk Korporasi Bank ABC (rating idAA) |
| Nilai Nominal (Par)       | Rp 3.000.000.000                       |
| Harga Beli (% dari par)   | 98,5000% = Rp 2.955.000.000            |
| Biaya Transaksi           | Rp 0 (waived)                          |
| Carrying Amount Awal (P0) | Rp 2.955.000.000                       |
| Kupon Kontraktual         | 6,00% per annum, dibayar semesteran    |
| Tenor                     | 3 tahun (6 periode kupon)              |
| Klasifikasi PSAK 71       | FVOCI (HTC\&S)                         |

EIR Annualized = 6,3445% (di atas kupon nominal 6,00% karena dibeli di bawah par). Diskonto Rp 45.000.000 diamortisasi sepanjang 3 tahun sebagai TAMBAHAN pendapatan bunga.

**Sample Schedule (semester 1):**

| **Periode** | **Opening Carrying** | **Kupon Kontraktual** | **Pendapatan Bunga EIR** | **Amortisasi Diskonto** | **Closing Carrying** |
| ----------- | -------------------- | --------------------- | ------------------------ | ----------------------- | -------------------- |
| 1 (30/06)   | 2.955.000.000        | 90.000.000            | 93.762.700               | \+3.762.700             | 2.958.762.700        |
| 2 (31/12)   | 2.958.762.700        | 90.000.000            | 93.882.121               | \+3.882.121             | 2.962.644.821        |

*Note FVOCI: pendapatan bunga Rp 93.762.700 diakui di P\&L. Selisih antara amortized carrying (Rp 2.958.762.700) dengan fair value IBPA pada 30/06 (mis. Rp 2.965.000.000) sebesar Rp 6.237.300 dibukukan di OCI sebagai unrealized MTM. Akumulasi OCI akan di-recycle ke P\&L saat obligasi dijual sebelum jatuh tempo.*

### 5.12.13 Validasi & Kontrol

  - Validasi pre-calc: harga beli \> 0; tenor sisa ≥ 1 periode kupon; kupon kontraktual ≥ 0%.

  - Validasi konvergensi: bila Newton-Raphson tidak konvergen dalam 50 iterasi → escalate ke Risk/Akuntansi dengan log iterasi & cash flow.

  - Validasi closing: Closing Carrying pada periode terakhir HARUS sama dengan nilai par (toleransi ±0,01 IDR). Bila tidak → block save schedule + alert.

  - Validasi balance jurnal: setiap periode posting, Total Debit = Total Kredit (sudah include amortisasi premium/diskonto).

  - Validasi reklasifikasi: saat instrumen direklasifikasi ke FVTPL, sistem auto-deactivate schedule dan tidak boleh ada akrual bunga EIR setelah tanggal efektif reklasifikasi.

  - Audit trail wajib: setiap re-estimation EIR menyimpan before/after EIR, before/after carrying, trigger event, dokumen pendukung, user maker & approver, timestamp.

  - Reporting wajib: Amortization Schedule per Instrumen, EIR Summary, Roll-Forward Carrying Amount, EIR Re-estimation Log (lihat Bab 10.3).

  - Three-eyes principle untuk re-estimation: Treasury/Risk Maker → Akuntansi Reviewer → Finance Controller/CFO Approver.

# 6\. Flow Proses Bisnis

## 6.1 Flow End-to-End Instrumen

Flow berikut menggambarkan siklus hidup penuh instrumen, dari pemilihan hingga penghapusan, dan keterkaitan tiap modul terhadap media upload dan jurnal.

| **Step** | **Aktor**                        | **Aksi**                                                                       | **Output / Trigger**                                  |
| -------- | -------------------------------- | ------------------------------------------------------------------------------ | ----------------------------------------------------- |
| 1        | Treasury (Maker)                 | Input data master instrumen baru (jika belum ada)                              | Master Instrumen tersimpan; rating Pefindo wajib      |
| 2        | Treasury, Risk, Komite Investasi | Pre-trade SPPI Test + BM Test (lihat Bab 4.4) + upload bukti term sheet/policy | Klasifikasi PSAK 71 ditetapkan & ter-lock             |
| 3        | Treasury (Maker)                 | Input transaksi Penempatan + upload bukti                                      | Status: PENDING APPROVAL                              |
| 4        | Treasury (Approver)              | Approve transaksi                                                              | Posting jurnal Penempatan otomatis ke GL              |
| 5        | Sistem (Job Harian)              | Akrual bunga deposito/obligasi (per hari)                                      | Jurnal akrual harian; update piutang bunga            |
| 6        | Sistem (Job Harian)              | Update MTM dari file upload IBPA/NAB                                           | Jurnal MTM ke OCI atau P\&L                           |
| 7        | Sistem (Job Harian)              | Update saldo cash dari rekening koran upload                                   | Update saldo cash di sistem                           |
| 8        | Sistem (Akhir Bulan)             | Hitung ECL: EAD × PD (3 skenario) × LGD; lalu × Impact PD                      | Jurnal CKPN — lihat 8.x                               |
| 9        | Treasury (Maker)                 | Penjualan / Renewal / Jatuh Tempo + upload bukti                               | Status: PENDING APPROVAL                              |
| 10       | Treasury (Approver)              | Approve transaksi closure                                                      | Posting jurnal closure & realized gain/loss           |
| 11       | Sistem                           | Saat jatuh tempo: kupon/bunga terakhir + pokok diterima                        | Jurnal jatuh tempo otomatis                           |
| 12       | Risk Officer                     | Periodic / Triggered Reassessment SPPI & BM (Bab 4.4.2/4.4.3)                  | Reklasifikasi prospektif jika hasil berubah (Bab 4.5) |
| 13       | Akuntansi                        | Rekonsiliasi ke GL (akhir hari/bulan)                                          | Reconciliation report                                 |
| 14       | Risk Mgmt                        | Upload Impact PD baru (triwulanan)                                             | Re-run ECL Engine                                     |
| 15       | Auditor / Internal Control       | Akses dokumen via Media Upload Repository                                      | Audit trail report                                    |

**Activity Diagram — Flow End-to-End Instrumen Investasi (Swimlane):**

![](media/image3.png)

*Gambar 6.1 — Activity diagram flow end-to-end instrumen investasi dengan lima swimlane (Treasury, SPPI/BM Committee, Sistem, Risk Officer, Akuntansi/Auditor) dan empat fase vertikal (Setup & Penempatan, Operasional Harian/Bulanan, Lifecycle Events, Review & Governance). Garis putus-putus menunjukkan feedback loop async: reklasifikasi prospektif (ungu) memicu BM Test ulang; re-run ECL Engine (amber) dipicu upload parameter forward-looking baru.*

## 6.2 Flow Mini per Tipe Instrumen

### 6.2.1 Cash di Bank

\[Setup Rekening\] → \[Upload Statement Harian\] → \[Sistem rekonsiliasi saldo\] → \[Akrual bunga jasa giro/tabungan\] → \[Hitung ECL bulanan dengan agregasi LPS\] → \[Jurnal CKPN\]

![](media/image4.png)

*Gambar 6.2.1 — Activity diagram Cash di Bank dengan swimlane User vs Sistem (klasifikasi Amortized Cost — wajib ECL).*

### 6.2.2 Deposito Berjangka

\[Penempatan + Upload Bilyet\] → \[Akrual bunga harian\] → \[Hitung ECL bulanan dengan agregasi LPS\] → \[Jatuh Tempo / Renewal / Pencairan + Upload Bukti\] → \[Settlement & jurnal closure\]

![](media/image5.png)

*Gambar 6.2.2 — Activity diagram Deposito Berjangka dengan swimlane User vs Sistem (klasifikasi Amortized Cost — wajib ECL).*

### 6.2.3 Obligasi

\[Penempatan + Upload NoA\] → \[Akrual kupon harian\] → \[MTM harian dari IBPA + Upload File\] → \[Jurnal OCI\] → \[Hitung ECL bulanan dari rating Pefindo\] → \[Jurnal CKPN ke OCI\] → \[Pembayaran kupon periodik + Upload Bukti\] → \[Penjualan / Jatuh Tempo + Upload Bukti\] → \[Reklasifikasi OCI ke P\&L\]

![](media/image6.png)

*Gambar 6.2.3 — Activity diagram Obligasi dengan swimlane User vs Sistem (klasifikasi FVOCI default atau AC — wajib ECL).*

### 6.2.4 Saham

Klasifikasi FVTPL (default): \[Pembelian (lot) + Upload Konfirmasi Broker\] → \[MTM harian dari harga penutupan BEI + Upload File\] → \[Jurnal P\&L\] → \[Penerimaan Dividen (cum/ex date) + Upload BPP PPh\] → \[Penjualan + Upload Konfirmasi Broker\] → \[Realized gain/loss ke P\&L\]

Klasifikasi FVOCI Election: \[Pembelian + Upload\] → \[MTM harian ke OCI tanpa recycling\] → \[Dividen tetap di P\&L\] → \[Penjualan: saldo OCI ke Saldo Laba Ditahan, BUKAN ke P\&L\]

![](media/image7.png)

*Gambar 6.2.4 — Activity diagram Saham dengan swimlane User vs Sistem dan decision branching FVTPL vs FVOCI Election (TIDAK ada ECL — saham adalah instrumen ekuitas).*

### 6.2.5 Reksadana Pasar Uang & Pendapatan Tetap

Klasifikasi FVTPL (default): \[Pembelian + Upload Konfirmasi\] → \[MTM harian dari NAB ke P\&L\] → \[Hitung ECL look-through (risk-mgmt view, tidak masuk LK)\] → \[Distribusi/Hasil ke P\&L\] → \[Redemption (tanpa selisih realized terpisah)\]

Klasifikasi FVOCI (kebijakan): \[Pembelian + Upload Konfirmasi\] → \[MTM harian dari NAB ke OCI\] → \[Hitung ECL look-through; jurnal beban CKPN ke P\&L, kontra ke OCI\] → \[Distribusi ke P\&L\] → \[Redemption: reklasifikasi akumulasi OCI ke P\&L (recycling)\]

![](media/image8.png)

*Gambar 6.2.5 — Activity diagram RDN Pasar Uang & Pendapatan Tetap dengan swimlane User vs Sistem; ECL look-through diakui di LK hanya bila klasifikasi FVOCI.*

### 6.2.6 Reksadana Saham

Klasifikasi FVTPL (default): \[Pembelian + Upload\] → \[MTM harian dari NAB ke P\&L\] → \[Tidak ada ECL — underlying ekuitas; gunakan VaR/beta untuk monitoring\] → \[Distribusi ke P\&L\] → \[Redemption tanpa selisih realized terpisah\]

Klasifikasi FVOCI (kebijakan): \[Pembelian + Upload\] → \[MTM harian ke OCI\] → \[Tidak ada ECL — underlying ekuitas\] → \[Distribusi ke P\&L\] → \[Redemption: reklasifikasi OCI ke P\&L (recycling)\]

![](media/image9.png)

*Gambar 6.2.6 — Activity diagram RDN Saham dengan swimlane User vs Sistem; flow berakhir pada Monitoring VaR/beta di lane Sistem (TIDAK ada CKPN — risiko ekuitas).*

### 6.2.7 Reksadana Campuran

Klasifikasi FVTPL (default): \[Pembelian + Upload\] → \[MTM ke P\&L\] → \[ECL look-through HANYA komponen non-equity (risk-mgmt view); equity dimonitor via VaR\] → \[Distribusi ke P\&L\] → \[Redemption\]

Klasifikasi FVOCI (kebijakan): \[Pembelian + Upload\] → \[MTM ke OCI\] → \[ECL look-through komponen non-equity → P\&L (kontra OCI); equity tidak hasilkan ECL tetapi MTM-nya tetap di OCI\] → \[Distribusi ke P\&L\] → \[Redemption: recycling OCI ke P\&L\]

![](media/image10.png)

*Gambar 6.2.7 — Activity diagram RDN Campuran dengan swimlane User vs Sistem dan decision underlying type. Komponen non-equity dihitung ECL look-through; komponen ekuitas excluded dari ECL.*

# 7\. Framework Perhitungan ECL

## 7.1 Formula Inti

**Sistem menggunakan dua mekanisme forward-looking yang bekerja pada level berbeda:**

1.  Impact MEV to PD bekerja di level INPUT — men-derivasi PD Good (Optimistic) dan PD Bad (Pessimistic) dari PD Normal (Base) yang berasal dari Pefindo.

2.  Impact PD bekerja di level OUTPUT — sebagai multiplier final dari ECL Weighted ke ECL FL.

**Konversi mata uang (mandatory):**

**SELURUH perhitungan EAD, ECL, dan ECL FL dilakukan dalam IDR equivalent. Untuk instrumen valas, EAD mata uang asli dikonversi terlebih dulu menggunakan kurs tengah BI pada tanggal evaluasi (lihat Bab 5.1.8 dan 5.10):**

**EAD\_IDR = EAD\_FX × Kurs\_Tengah\_BI(tanggal\_evaluasi)**

*Untuk instrumen IDR, kurs implisit = 1,0000, sehingga EAD\_IDR = EAD\_FX. PD, LGD, Impact MEV, dan Impact PD adalah rasio (unitless), sehingga output ECL otomatis dalam IDR.*

**Tahapan perhitungan ECL FL (semua dalam IDR equivalent):**

| **Step** | **Komponen**                     | **Formula**                                                                                              |
| -------- | -------------------------------- | -------------------------------------------------------------------------------------------------------- |
| 0        | **Konversi EAD ke IDR**          | EAD\_IDR = EAD\_mata\_uang\_asli × Kurs Tengah BI (tanggal evaluasi)                                     |
| 1        | **Derivasi PD per Skenario**     | PD Good = PD Normal × Impact MEV (Good); PD Normal = data Pefindo; PD Bad = PD Normal × Impact MEV (Bad) |
| 2        | **ECL Stage 1 per Skenario**     | ECL\_S1\_skenario (IDR) = EAD\_IDR × PD 12-Month\_skenario × LGD                                         |
| 3        | **ECL Stage 2 / 3 per Skenario** | ECL\_S2/S3\_skenario (IDR) = EAD\_IDR × Lifetime PD\_skenario × LGD                                      |
| 4        | **ECL Weighted**                 | ECL (IDR) = (w\_good × ECL\_good) + (w\_normal × ECL\_normal) + (w\_bad × ECL\_bad)                      |
| 5        | **ECL Forward-Looking (FL)**     | **ECL FL (IDR) = ECL Weighted × Impact PD**                                                              |

**Parameter pendukung:**

| **Parameter**                 | **Nilai Default**                                                                    |
| ----------------------------- | ------------------------------------------------------------------------------------ |
| **Bobot Skenario**            | w\_good = 0,2500; w\_normal = 0,5000; w\_bad = 0,2500 (total = 1,0000)               |
| **Sumber PD per Stage**       | Stage 1: PD 12-Month Normal (Pefindo); Stage 2/3: Lifetime PD Normal (lihat Bab 8.5) |
| **Default Impact MEV (Good)** | Umumnya \< 1,0000 (mengurangi PD karena kondisi membaik)                             |
| **Default Impact MEV (Bad)**  | Umumnya \> 1,0000 (memperbesar PD karena kondisi memburuk)                           |
| **Default Impact PD**         | 1,0000 (no overlay) sampai 1,1500 (standard FL adjustment), tergantung kebijakan     |

## 7.2 Sumber Data Wajib

| **Parameter**                               | **Sumber**                                              | **Mekanisme Update**                                               |
| ------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------ |
| **PD Normal 12-Month (Stage 1)**            | Pefindo Default Study (1-year)                          | Upload CSV/XLSX per rating, triwulanan (Bab 5.1.3)                 |
| **PD Normal Lifetime (Stage 2/3)**          | Pefindo Cumulative Default Study atau derivasi internal | Upload tabel cumulative PD multi-tenor (Bab 5.1.3.a)               |
| **LGD**                                     | Basel III Foundation IRB                                | Upload tabel LGD per tipe eksposur                                 |
| **Impact MEV to PD (derivasi PD Good/Bad)** | Komite Risiko / ALCO                                    | Upload XLSX, per periode (Bab 5.8.3)                               |
| **Impact PD (multiplier ECL FL)**           | Komite Risiko / ALCO / CFO                              | Upload XLSX, per periode (Bab 5.8.3.a)                             |
| **EAD**                                     | Saldo internal sistem                                   | Otomatis dari modul transaksi                                      |
| **Stage Allocation**                        | Counterparty Rating History + DPD + qualitative trigger | Otomatis re-evaluasi setiap akhir bulan dan saat input rating baru |
| **Underlying Reksadana**                    | Fund Fact Sheet / Laporan Bulanan MI                    | Upload bulanan (Bab 5.8.4)                                         |

## 7.3 Aturan Presisi Desimal

1.  Rasio (PD Normal/Good/Bad, LGD, Impact MEV, Impact PD, bobot skenario): 4 angka di belakang koma.

2.  Nilai mata uang IDR: 2 angka di belakang koma untuk perhitungan internal; pembulatan ke rupiah penuh hanya saat presentasi laporan.

3.  Pembulatan menggunakan metode banker's rounding (round-half-to-even) untuk mengurangi bias.

4.  Hasil ECL FL ditampilkan dengan 2 desimal IDR pada laporan.

## 7.4 Contoh Lengkap Derivasi PD dan Perhitungan ECL FL

**Contoh untuk Obligasi PT XYZ (rating idA), Stage 1, EAD Rp 5.075.000.000:**

**Step 1 — Derivasi PD per skenario dari PD Normal:**

| **Skenario**      | **PD Normal (Pefindo)** | **Impact MEV**           | **PD Skenario**            |
| ----------------- | ----------------------- | ------------------------ | -------------------------- |
| GOOD (Optimistic) | 0,0020                  | 0,5000                   | 0,0010 (= 0,0020 × 0,5000) |
| NORMAL (Base)     | 0,0020                  | 1,0000 (tidak digunakan) | 0,0020 (langsung)          |
| BAD (Pessimistic) | 0,0020                  | 2,5000                   | 0,0050 (= 0,0020 × 2,5000) |

**Step 2 — Hitung ECL per skenario (LGD = 0,4500):**

| **Skenario** | **EAD**       | **PD Skenario** | **LGD** | **ECL**       |
| ------------ | ------------- | --------------- | ------- | ------------- |
| GOOD         | 5.075.000.000 | 0,0010          | 0,4500  | 2.283.750,00  |
| NORMAL       | 5.075.000.000 | 0,0020          | 0,4500  | 4.567.500,00  |
| BAD          | 5.075.000.000 | 0,0050          | 0,4500  | 11.418.750,00 |

**Step 3 — Hitung ECL Weighted (bobot 0,2500 / 0,5000 / 0,2500):**

ECL Weighted = (0,2500 × 2.283.750) + (0,5000 × 4.567.500) + (0,2500 × 11.418.750) = 570.937,50 + 2.283.750,00 + 2.854.687,50 = Rp 5.709.375,00

**Step 4 — Terapkan Impact PD (multiplier final, asumsi 1,1500):**

**ECL FL = ECL Weighted × Impact PD = 5.709.375,00 × 1,1500 = Rp 6.565.781,25**

*Catatan: Hasil akhir ECL FL Rp 6.565.781,25 sama dengan contoh di Bab 8.2.2 — namun dengan transparansi penuh tahapan derivasi (PD Good/Bad bukan ditarik langsung dari Pefindo, melainkan diderivasi via Impact MEV).*

# 8\. Perhitungan ECL per Tipe Instrumen

**Catatan penting untuk instrumen ekuitas dan reksadana:**

  - Saham (klasifikasi FVTPL maupun FVOCI Election) → TIDAK ADA pengakuan ECL. Risiko dimonitor melalui VaR / sensitivitas pasar / beta — di luar lingkup dokumen ini.

  - Reksadana FVTPL → ECL look-through hanya risk-management view (tidak masuk laporan keuangan).

  - Reksadana FVOCI → ECL look-through DIAKUI di laporan keuangan: beban CKPN ke P\&L, kontra ke OCI (sama mekanisme dengan obligasi FVOCI).

  - Reksadana Saham (FVTPL atau FVOCI) → TIDAK ADA pengakuan ECL pada laporan keuangan karena underlying ekuitas (PD ekuitas tidak terdefinisi). Untuk klasifikasi FVOCI, hanya MTM yang masuk OCI; tidak ada beban CKPN.

  - Reksadana Campuran (FVTPL atau FVOCI) → ECL look-through HANYA untuk komponen non-equity (obligasi, deposito, cash). Komponen ekuitas tidak masuk perhitungan ECL meskipun MTM-nya tetap dijurnal sesuai klasifikasi.

  - Reksadana Pasar Uang & Pendapatan Tetap → ECL look-through penuh ke seluruh underlying.

## 8.1 ECL Cash & Deposito (Logika LPS Agregat)

### 8.1.1 Konsep Kunci

LPS menjamin total simpanan per nasabah per bank — saldo Tabungan/Giro (= Cash) DAN Deposito di bank yang sama digabung dan dibandingkan terhadap batas penjaminan LPS (Rp 2.000.000.000).

Eksposur yang dihitung untuk ECL hanya bagian yang TIDAK terjamin LPS. Setelah eksposur tak terjamin diketahui, dialokasikan secara proporsional ke Cash dan Deposito untuk menjaga konsistensi pencatatan akuntansi yang terpisah.

### 8.1.2 Algoritma

1.  Untuk setiap Bank: Total\_Bank = Σ Saldo Cash + Σ Saldo Deposito (per CIF/nasabah per bank).

2.  Eksposur Tak Terjamin: EAD\_Bank = MAX(0; Total\_Bank − LPS\_Coverage).

3.  Alokasi proporsional:
    
      - EAD\_Cash = (Σ Saldo Cash ÷ Total\_Bank) × EAD\_Bank
    
      - EAD\_Deposito = (Σ Saldo Deposito ÷ Total\_Bank) × EAD\_Bank

<!-- end list -->

1.  Lookup PD (3 skenario) dari rating Pefindo bank.

2.  Lookup LGD dari Basel — Senior Unsecured (Bank) = 0,4500.

3.  Hitung ECL per skenario: ECL\_s = EAD × PD\_s × LGD.

4.  ECL Weighted = Σ (w\_s × ECL\_s).

5.  ECL FL = ECL × Impact PD\_bank.

### 8.1.3 Contoh Perhitungan: Bank Mandiri (Rating idAAA)

**Data input:**

| **Parameter**                                 | **Nilai**                |
| --------------------------------------------- | ------------------------ |
| Saldo Cash (Giro+Tabungan)                    | Rp 1.500.000.000         |
| Saldo Deposito                                | Rp 3.000.000.000         |
| **Total Eksposur ke Bank Mandiri**            | **Rp 4.500.000.000**     |
| LPS Coverage                                  | Rp 2.000.000.000         |
| **Eksposur Tak Terjamin (EAD\_Bank)**         | **Rp 2.500.000.000**     |
| Rating Pefindo                                | idAAA                    |
| PD Normal (Pefindo, idAAA)                    | 0,0002                   |
| Impact MEV (Good / Bad)                       | 0,5000 / 2,5000          |
| PD Good (= PD Normal × Impact MEV Good)       | 0,0001                   |
| PD Bad (= PD Normal × Impact MEV Bad)         | 0,0005                   |
| LGD (Basel — Senior Unsecured Bank)           | 0,4500                   |
| Bobot Skenario (w\_good / w\_normal / w\_bad) | 0,2500 / 0,5000 / 0,2500 |
| Impact PD (multiplier akhir ke ECL FL)        | 1,1500                   |

**Alokasi EAD per komponen:**

| **Komponen**               | **Proporsi**                           | **EAD (IDR)**        |
| -------------------------- | -------------------------------------- | -------------------- |
| Cash                       | 1.500.000.000 ÷ 4.500.000.000 = 0,3333 | 833.333.333,33       |
| Deposito                   | 3.000.000.000 ÷ 4.500.000.000 = 0,6667 | 1.666.666.666,67     |
| **Total EAD Tak Terjamin** | 1,0000                                 | **2.500.000.000,00** |

**Perhitungan ECL Cash:**

| **Skenario** | **EAD**        | **PD** | **LGD** | **ECL**    |
| ------------ | -------------- | ------ | ------- | ---------- |
| Optimistic   | 833.333.333,33 | 0,0001 | 0,4500  | 37.500,00  |
| Base         | 833.333.333,33 | 0,0002 | 0,4500  | 75.000,00  |
| Pessimistic  | 833.333.333,33 | 0,0005 | 0,4500  | 187.500,00 |

ECL Weighted Cash = (0,2500 × 37.500) + (0,5000 × 75.000) + (0,2500 × 187.500) = 9.375 + 37.500 + 46.875 = Rp 93.750,00

**ECL FL Cash = 93.750,00 × 1,1500 = Rp 107.812,50**

**Perhitungan ECL Deposito:**

| **Skenario** | **EAD**          | **PD** | **LGD** | **ECL**    |
| ------------ | ---------------- | ------ | ------- | ---------- |
| Optimistic   | 1.666.666.666,67 | 0,0001 | 0,4500  | 75.000,00  |
| Base         | 1.666.666.666,67 | 0,0002 | 0,4500  | 150.000,00 |
| Pessimistic  | 1.666.666.666,67 | 0,0005 | 0,4500  | 375.000,00 |

ECL Weighted Deposito = (0,2500 × 75.000) + (0,5000 × 150.000) + (0,2500 × 375.000) = 18.750 + 75.000 + 93.750 = Rp 187.500,00

**ECL FL Deposito = 187.500,00 × 1,1500 = Rp 215.625,00**

*Catatan: Apabila Total Eksposur ke bank ≤ LPS Coverage (Rp 2.000.000.000), maka EAD = 0 dan ECL = 0.*

## 8.2 ECL Obligasi

### 8.2.1 Algoritma

1.  EAD = Carrying Amount + Accrued Interest. Untuk FVOCI: Carrying = Fair Value; untuk AC: Carrying = Amortized Cost.

2.  PD diambil dari rating Pefindo issuer.

3.  LGD diambil dari klasifikasi Basel issuer (sovereign / senior secured / unsecured / subordinated).

4.  ECL per skenario = EAD × PD\_s × LGD.

5.  ECL Weighted = Σ (w\_s × ECL\_s).

6.  ECL FL = ECL × Impact PD untuk tipe eksposur tersebut.

### 8.2.2 Contoh: Obligasi PT XYZ Tbk (Rating idA-)

**Data input:**

| **Parameter**                           | **Nilai**                |        |         |               |
| --------------------------------------- | ------------------------ | ------ | ------- | ------------- |
| Nilai Nominal (Face Value)              | Rp 5.000.000.000         |        |         |               |
| Carrying Amount (Fair Value, FVOCI)     | Rp 5.050.000.000         |        |         |               |
| Accrued Interest                        | Rp 25.000.000            |        |         |               |
| **EAD**                                 | **Rp 5.075.000.000**     |        |         |               |
| Rating Pefindo (idA)                    | PD Normal: 0,0020        |        |         |               |
| Impact MEV (Good / Bad)                 | 0,5000 / 2,5000          |        |         |               |
| PD Good / Normal / Bad (hasil derivasi) | 0,0010 / 0,0020 / 0,0050 |        |         |               |
| LGD (Senior Unsecured Korporasi)        | 0,4500                   |        |         |               |
| Bobot Skenario                          | 0,2500 / 0,5000 / 0,2500 |        |         |               |
| Impact PD (multiplier akhir)            | 1,1500                   |        |         |               |
| **Skenario**                            | **EAD**                  | **PD** | **LGD** | **ECL**       |
| Optimistic                              | 5.075.000.000            | 0,0010 | 0,4500  | 2.283.750,00  |
| Base                                    | 5.075.000.000            | 0,0020 | 0,4500  | 4.567.500,00  |
| Pessimistic                             | 5.075.000.000            | 0,0050 | 0,4500  | 11.418.750,00 |

ECL Weighted = (0,2500 × 2.283.750) + (0,5000 × 4.567.500) + (0,2500 × 11.418.750) = 570.937,50 + 2.283.750,00 + 2.854.687,50 = Rp 5.709.375,00

**ECL FL Obligasi = 5.709.375,00 × 1,1500 = Rp 6.565.781,25**

## 8.3 ECL Reksadana (Look-Through)

### 8.3.1 Algoritma

Reksadana di-look-through ke aset underlying. Setiap underlying diperlakukan sebagai eksposur tersendiri:

1.  Identifikasi komposisi underlying reksadana dari Fund Fact Sheet (FFS) atau laporan bulanan MI (upload).

2.  Hitung NAB total reksadana yang dimiliki = Jumlah Unit × NAB per Unit.

3.  Untuk setiap underlying i: EAD\_i = NAB\_total × Bobot\_i.

4.  Lookup PD\_i (3 skenario) dan LGD\_i berdasarkan jenis underlying:
    
      - Underlying obligasi pemerintah → PD sovereign, LGD 0,4500
    
      - Underlying obligasi korporasi → PD per rating Pefindo issuer, LGD per Basel
    
      - Underlying cash di bank (rekening MI) → PD bank, LGD 0,4500. Catatan: LPS tidak berlaku karena ini eksposur MI ke bank, bukan eksposur investor.
    
      - Underlying instrumen pasar uang lain → ekuivalen ke kategori terdekat

<!-- end list -->

1.  ECL\_i = EAD\_i × PD\_i × LGD\_i (per skenario).

2.  ECL Weighted\_i = Σ (w\_s × ECL\_i\_s).

3.  ECL Reksadana = Σ ECL Weighted\_i.

4.  ECL FL Reksadana = ECL Reksadana × Impact PD (rata-rata tertimbang underlying atau Impact PD spesifik tipe instrumen — sesuai parameter upload).

**Catatan PSAK 71:**

*Reksadana umumnya diklasifikasi FVTPL → tidak ada pengakuan ECL pada laporan keuangan. Perhitungan ECL look-through ini diakui sebagai risk-management view (untuk monitoring konsentrasi risiko, stress testing, dan internal capital).*

### 8.3.2 Contoh: Reksadana Pendapatan Tetap ABC, NAB Rp 10.000.000.000

**Komposisi underlying (dari Fund Fact Sheet, dilampirkan sebagai upload):**

| **Underlying**            | **Bobot**   | **EAD**            | **Rating / Tipe**          |
| ------------------------- | ----------- | ------------------ | -------------------------- |
| Obligasi Pemerintah (SUN) | 60,00%      | 6.000.000.000      | Sovereign                  |
| Obligasi Korporasi (idAA) | 30,00%      | 3.000.000.000      | Senior Unsecured Korporasi |
| Cash di Bank (idAAA)      | 10,00%      | 1.000.000.000      | Senior Unsecured Bank      |
| **TOTAL NAB**             | **100,00%** | **10.000.000.000** |                            |

**Underlying 1: Obligasi Pemerintah (SUN)**

| **Skenario** | **EAD**       | **PD** | **LGD** | **ECL**      |
| ------------ | ------------- | ------ | ------- | ------------ |
| Optimistic   | 6.000.000.000 | 0,0001 | 0,4500  | 270.000,00   |
| Base         | 6.000.000.000 | 0,0002 | 0,4500  | 540.000,00   |
| Pessimistic  | 6.000.000.000 | 0,0005 | 0,4500  | 1.350.000,00 |

ECL Weighted SUN = (0,2500 × 270.000) + (0,5000 × 540.000) + (0,2500 × 1.350.000) = Rp 675.000,00

**Underlying 2: Obligasi Korporasi (idAA)**

| **Skenario** | **EAD**       | **PD** | **LGD** | **ECL**      |
| ------------ | ------------- | ------ | ------- | ------------ |
| Optimistic   | 3.000.000.000 | 0,0003 | 0,4500  | 405.000,00   |
| Base         | 3.000.000.000 | 0,0005 | 0,4500  | 675.000,00   |
| Pessimistic  | 3.000.000.000 | 0,0010 | 0,4500  | 1.350.000,00 |

ECL Weighted Korporasi = (0,2500 × 405.000) + (0,5000 × 675.000) + (0,2500 × 1.350.000) = 101.250 + 337.500 + 337.500 = Rp 776.250,00

**Underlying 3: Cash di Bank (idAAA)**

| **Skenario** | **EAD**       | **PD** | **LGD** | **ECL**    |
| ------------ | ------------- | ------ | ------- | ---------- |
| Optimistic   | 1.000.000.000 | 0,0001 | 0,4500  | 45.000,00  |
| Base         | 1.000.000.000 | 0,0002 | 0,4500  | 90.000,00  |
| Pessimistic  | 1.000.000.000 | 0,0005 | 0,4500  | 225.000,00 |

ECL Weighted Cash dalam RDN = (0,2500 × 45.000) + (0,5000 × 90.000) + (0,2500 × 225.000) = Rp 112.500,00

**Rekapitulasi ECL Reksadana:**

| **Underlying**            | **Bobot**   | **ECL Weighted (IDR)** |
| ------------------------- | ----------- | ---------------------- |
| Obligasi Pemerintah (SUN) | 60,00%      | 675.000,00             |
| Obligasi Korporasi (idAA) | 30,00%      | 776.250,00             |
| Cash di Bank (idAAA)      | 10,00%      | 112.500,00             |
| **TOTAL ECL Reksadana**   | **100,00%** | **1.563.750,00**       |

**ECL FL Reksadana = 1.563.750,00 × 1,1500 = Rp 1.798.312,50**

### 8.3.3 Contoh: Reksadana Campuran XYZ, NAB Rp 5.000.000.000

*Reksadana Campuran memiliki underlying gabungan: obligasi, saham, dan cash. Komponen ekuitas TIDAK masuk perhitungan ECL.*

**Komposisi underlying (dari Fund Fact Sheet):**

| **Underlying**             | **Bobot**   | **EAD**           | **Treatment ECL**        |
| -------------------------- | ----------- | ----------------- | ------------------------ |
| Obligasi Korporasi (idA)   | 40,00%      | 2.000.000.000     | Hitung ECL               |
| Saham (Equity)             | 40,00%      | 2.000.000.000     | EXCLUDED — tidak ada ECL |
| Cash di Bank (idAAA)       | 20,00%      | 1.000.000.000     | Hitung ECL               |
| **TOTAL NAB**              | **100,00%** | **5.000.000.000** |                          |
| **EAD Eligible untuk ECL** | **60,00%**  | **3.000.000.000** | Hanya non-equity         |

**Underlying 1: Obligasi Korporasi (idA)**

| **Skenario** | **EAD**       | **PD** | **LGD** | **ECL**      |
| ------------ | ------------- | ------ | ------- | ------------ |
| Optimistic   | 2.000.000.000 | 0,0010 | 0,4500  | 900.000,00   |
| Base         | 2.000.000.000 | 0,0020 | 0,4500  | 1.800.000,00 |
| Pessimistic  | 2.000.000.000 | 0,0050 | 0,4500  | 4.500.000,00 |

ECL Weighted Obligasi = (0,2500 × 900.000) + (0,5000 × 1.800.000) + (0,2500 × 4.500.000) = 225.000 + 900.000 + 1.125.000 = Rp 2.250.000,00

**Underlying 2: Saham (Equity)**

*EXCLUDED dari perhitungan ECL. Risiko ekuitas dimonitor terpisah melalui VaR / beta / sensitivitas indeks pasar — tidak menghasilkan jurnal CKPN.*

**Underlying 3: Cash di Bank (idAAA)**

| **Skenario** | **EAD**       | **PD** | **LGD** | **ECL**    |
| ------------ | ------------- | ------ | ------- | ---------- |
| Optimistic   | 1.000.000.000 | 0,0001 | 0,4500  | 45.000,00  |
| Base         | 1.000.000.000 | 0,0002 | 0,4500  | 90.000,00  |
| Pessimistic  | 1.000.000.000 | 0,0005 | 0,4500  | 225.000,00 |

ECL Weighted Cash dalam RDN Campuran = (0,2500 × 45.000) + (0,5000 × 90.000) + (0,2500 × 225.000) = Rp 112.500,00

**Rekapitulasi ECL Reksadana Campuran:**

| **Underlying**                   | **Bobot**           | **ECL Weighted (IDR)** |
| -------------------------------- | ------------------- | ---------------------- |
| Obligasi Korporasi (idA)         | 40,00%              | 2.250.000,00           |
| Saham (Equity) — EXCLUDED        | 40,00%              | —                      |
| Cash di Bank (idAAA)             | 20,00%              | 112.500,00             |
| **TOTAL ECL Reksadana Campuran** | **60,00% eligible** | **2.362.500,00**       |

**ECL FL Reksadana Campuran (risk-mgmt view) = 2.362.500,00 × 1,1500 = Rp 2.716.875,00**

*Catatan: ECL ini bersifat risk-management view (FVTPL → tidak dijurnal di laporan keuangan).*

### 8.3.4 Reksadana Saham — Tidak Ada Perhitungan ECL

Reksadana Saham memiliki underlying mayoritas/seluruhnya berupa saham. Karena instrumen ekuitas tidak menghasilkan PD dalam framework ECL PSAK 71, maka:

  - Tidak ada perhitungan ECL look-through untuk reksadana saham.

  - Tidak ada jurnal CKPN.

  - Risiko dimonitor melalui metrik pasar terpisah (VaR, beta, tracking error vs IHSG) — di luar lingkup dokumen ini.

## 8.4 Rangkuman Total ECL Portofolio (Contoh Konsolidasi)

| **Instrumen**                     | **Klasifikasi** | **ECL Weighted** | **ECL FL**   | **Pengakuan ECL**                            |
| --------------------------------- | --------------- | ---------------- | ------------ | -------------------------------------------- |
| Cash (Bank Mandiri)               | AC              | 93.750,00        | 107.812,50   | P\&L (CKPN)                                  |
| Deposito (Bank Mandiri)           | AC              | 187.500,00       | 215.625,00   | P\&L (CKPN)                                  |
| Obligasi PT XYZ                   | FVOCI           | 5.709.375,00     | 6.565.781,25 | P\&L (kontra OCI)                            |
| Saham (semua klasifikasi)         | FVTPL/FVOCI El. | —                | —            | Tidak ada ECL                                |
| RDN Pendapatan Tetap ABC          | FVTPL           | 1.563.750,00     | 1.798.312,50 | Risk-Mgmt View                               |
| RDN Pendapatan Tetap (jika FVOCI) | FVOCI           | 1.563.750,00     | 1.798.312,50 | P\&L (kontra OCI)                            |
| RDN Campuran XYZ                  | FVTPL           | 2.362.500,00     | 2.716.875,00 | Risk-Mgmt View                               |
| RDN Campuran (jika FVOCI)         | FVOCI           | 2.362.500,00     | 2.716.875,00 | P\&L (kontra OCI; hanya komponen non-equity) |
| RDN Saham (semua klasifikasi)     | FVTPL/FVOCI     | —                | —            | Tidak ada ECL                                |

**Catatan tabel:**

  - Tabel di atas menampilkan ECL portofolio fiktif di mana RDN Pendapatan Tetap dan RDN Campuran masing-masing ditampilkan dua baris untuk membandingkan pengakuan ECL pada klasifikasi FVTPL vs FVOCI. Dalam pencatatan riil, satu instrumen hanya memiliki satu klasifikasi.

  - Bila SELURUH reksadana di portofolio diklasifikasi FVTPL: Total ECL diakui di LK = Rp 5.990.625,00 (Cash + Deposito + Obligasi); ECL FL = Rp 6.889.218,75. Sisanya risk-management view.

  - Bila SELURUH reksadana di portofolio diklasifikasi FVOCI: Total ECL diakui di LK = Rp 9.916.875,00 (ditambah RDN Pendapatan Tetap & RDN Campuran); ECL FL = Rp 11.404.406,25.

## 8.5 Staging ECL (PSAK 71 — 3 Stage Model)

PSAK 71 mengharuskan klasifikasi setiap aset keuangan ke salah satu dari tiga Stage berdasarkan kondisi risiko kreditnya, yang menentukan horizon perhitungan ECL: 12-Month PD untuk Stage 1, atau Lifetime PD untuk Stage 2 dan Stage 3.

### 8.5.1 Definisi 3 Stage

| **Stage**   | **Karakteristik**                                                                          | **Sumber PD**                                      | **Perhitungan ECL**                                             |
| ----------- | ------------------------------------------------------------------------------------------ | -------------------------------------------------- | --------------------------------------------------------------- |
| **Stage 1** | Performing — risiko kredit pada pengakuan awal, atau tidak ada SICR                        | PD 12-Month dari Pefindo (lihat 5.1.3)             | ECL 12-Month = EAD × PD 12M × LGD                               |
| **Stage 2** | Underperforming — terjadi Significant Increase in Credit Risk (SICR), tetapi belum default | Lifetime PD (lihat 5.1.3.a)                        | ECL Lifetime = EAD × Lifetime PD × LGD                          |
| **Stage 3** | Non-Performing / Credit-Impaired — sudah default atau credit deterioration material        | Lifetime PD (umumnya = 1,0000 saat default actual) | ECL Lifetime — bunga diakui pada nilai net carrying (post CKPN) |

### 8.5.2 Kriteria Migrasi Antar Stage

**A. Trigger Migrasi Stage 1 → Stage 2 (SICR Indicator):**

1.  Penurunan rating Pefindo ≥ 2 notch dari rating saat origination (mis. idAA → idA-).

2.  Rating berpindah dari investment grade (idBBB ke atas) ke non-investment grade (idBB ke bawah).

3.  Tunggakan pembayaran 30–90 hari (presumption SICR per PSAK 71).

4.  Outlook rating menjadi NEGATIVE setidaknya 2 review berturut-turut.

5.  Indikator kuantitatif: PD 12-Month meningkat ≥ 100% dari level origination.

6.  Indikator kualitatif: kondisi keuangan issuer memburuk material (laporan keuangan menunjukkan covenant breach, qualified opinion auditor, dll).

**B. Trigger Migrasi ke Stage 3 (Default / Credit-Impaired):**

1.  Rating Pefindo = idD.

2.  Tunggakan pembayaran \> 90 hari (rebuttable presumption per PSAK 71).

3.  Counterparty mengajukan PKPU/Pailit.

4.  Restrukturisasi paksa (forced restructuring) yang menyebabkan kerugian material bagi pemegang.

5.  Indikator kualitatif: gagal bayar kupon/pokok, debitur dalam proses likuidasi.

**C. Curing — Migrasi Mundur (Stage 2 → 1, Stage 3 → 2):**

1.  Bila kondisi yang memicu SICR/default tidak lagi terpenuhi selama probationary period (umumnya 3–6 bulan), instrumen dapat di-migrasi mundur.

2.  Migrasi mundur dari Stage 3 → Stage 2 → Stage 1 dilakukan bertahap; tidak diperbolehkan loncat dari Stage 3 langsung ke Stage 1.

3.  Curing wajib di-approve Komite Risiko dan didokumentasikan.

### 8.5.3 Mekanisme Otomatis Sistem

1.  Setiap input rating baru ke Counterparty Rating History → sistem otomatis mengevaluasi SICR dan Default trigger.

2.  Setiap akhir bulan (job batch) → sistem mengevaluasi seluruh instrumen aktif terhadap kriteria SICR dan default.

3.  Bila migrasi terjadi → sistem membuat record di tabel Stage History (instrument\_id, tanggal migrasi, dari Stage X ke Stage Y, alasan, user\_approver).

4.  Migrasi otomatis untuk Stage 3 (default) langsung diproses sistem; curing (migrasi mundur) memerlukan approval manual.

5.  Notifikasi otomatis ke Risk Officer dan Akuntansi untuk setiap migrasi.

### 8.5.4 Field Tabel Stage History per Instrumen

| **Field**                  | **Tipe Data** | **Wajib**   | **Keterangan**                                                                                                                   |
| -------------------------- | ------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------- |
| Stage History ID           | VARCHAR(20)   | Ya          | Auto-generate (mis. STH-2026-00001).                                                                                             |
| Kode Instrumen             | FK            | Ya          | Reference master instrumen.                                                                                                      |
| Tanggal Migrasi            | DATE          | Ya          | Tanggal efektif migrasi.                                                                                                         |
| Stage Sebelum              | ENUM          | Ya          | STAGE\_1 | STAGE\_2 | STAGE\_3.                                                                                                  |
| Stage Sesudah              | ENUM          | Ya          | STAGE\_1 | STAGE\_2 | STAGE\_3.                                                                                                  |
| Trigger Type               | ENUM          | Ya          | RATING\_DOWNGRADE | DPD\_30\_90 | DPD\_GT\_90 | DEFAULT\_RATING\_D | PKPU\_PAILIT | RESTRUKTURISASI | CURING | MANUAL\_OVERRIDE. |
| Detail Trigger             | TEXT          | Ya          | Penjelasan teknis (mis. "Rating Pefindo turun dari idAA menjadi idBBB; \> 2 notch dari origination idAAA").                      |
| Rating Saat Migrasi        | VARCHAR(8)    | Ya          | Rating Pefindo pada tanggal migrasi.                                                                                             |
| Hari Tunggakan (DPD)       | INT           | Kondisional | Untuk trigger DPD.                                                                                                               |
| User Approver              | FK            | Kondisional | Wajib untuk curing dan manual override.                                                                                          |
| Status Approval            | ENUM          | Ya          | AUTO | PENDING\_APPROVAL | APPROVED | REJECTED.                                                                                  |
| Dokumen Pendukung (Upload) | FILE          | Kondisional | Untuk curing dan manual override; memo Komite Risiko.                                                                            |

### 8.5.5 Contoh Perhitungan ECL per Stage

**Contoh A: Obligasi PT XYZ, EAD Rp 5.075.000.000, sisa tenor 5 tahun.**

Origination rating: idA. Pada tanggal evaluasi, rating tetap idA (tidak ada SICR) → Stage 1.

| **Skenario** | **EAD**       | **PD 12-Month** | **LGD** | **ECL (Stage 1)** |
| ------------ | ------------- | --------------- | ------- | ----------------- |
| Optimistic   | 5.075.000.000 | 0,0010          | 0,4500  | 2.283.750,00      |
| Base         | 5.075.000.000 | 0,0020          | 0,4500  | 4.567.500,00      |
| Pessimistic  | 5.075.000.000 | 0,0050          | 0,4500  | 11.418.750,00     |

ECL Weighted Stage 1 = (0,2500 × 2.283.750) + (0,5000 × 4.567.500) + (0,2500 × 11.418.750) = Rp 5.709.375,00

**ECL FL Stage 1 = 5.709.375,00 × 1,1500 = Rp 6.565.781,25**

**Contoh B: Obligasi yang sama, namun rating turun ke idBB (downgrade 3 notch dari idA) → Stage 2.**

Lookup Lifetime PD untuk idBB pada tenor 5 tahun (dari Bab 5.1.3.a). Skenario tetap 3 (optimistic/base/pessimistic), tetapi PD-nya berbasis Lifetime.

| **Skenario** | **EAD**       | **Lifetime PD (5-Yr, idBB)** | **LGD** | **ECL (Stage 2)** |
| ------------ | ------------- | ---------------------------- | ------- | ----------------- |
| Optimistic   | 5.075.000.000 | 0,0707                       | 0,4500  | 161.460.563,00    |
| Base         | 5.075.000.000 | 0,1413                       | 0,4500  | 322.694.438,00    |
| Pessimistic  | 5.075.000.000 | 0,3170                       | 0,4500  | 723.948.750,00    |

*Catatan: PD Good & PD Bad untuk Stage 2 diderivasi dari PD Normal Lifetime (5-Yr idBB = 0,1413) menggunakan Impact MEV yang sama dengan Stage 1: PD Good = 0,1413 × 0,5000 = 0,0707; PD Bad = 0,1413 × 2,5000 = 0,3170 (capped di 1,0000).*

ECL Weighted Stage 2 = (0,2500 × 161.460.563) + (0,5000 × 322.694.438) + (0,2500 × 723.948.750) = Rp 382.699.547,25

**ECL FL Stage 2 = 382.699.547,25 × 1,1500 = Rp 440.104.479,34**

*Δ ECL FL akibat migrasi Stage 1 → Stage 2 = Rp 440.104.479,34 − Rp 6.565.781,25 = Rp 433.538.698,09 (tambahan beban CKPN).*

**Contoh C: Obligasi yang sama, rating turun ke idD (default) → Stage 3.**

PD = 1,0000 (default actual). LGD tetap 0,4500 (asumsi recovery rate 55%). EAD = nilai tercatat dikurangi pembayaran yang sudah diterima.

ECL Stage 3 = EAD × 1,0000 × 0,4500 = 5.075.000.000 × 0,4500 = Rp 2.283.750.000,00

Bunga selanjutnya tidak diakui pada gross EAD, melainkan pada net carrying amount (post CKPN) — mengikuti PSAK 71 untuk credit-impaired assets.

# 9\. Skenario Transaksi & Jurnal Akuntansi

Bab ini menguraikan jurnal otomatis yang dihasilkan sistem untuk setiap event. Asumsi pajak: PPh 4(2) Final 20% atas bunga deposito dan jasa giro; PPh Final 10% atas kupon obligasi korporasi; PPh Final 10% atas kupon SUN (mengikuti tarif yang berlaku); PPh Final 0,1% atas transaksi penjualan saham di bursa; PPh Final 10% atas dividen WP OP (atau exempt untuk WP Badan sesuai PP 9/2021 jika reinvestasi). Format jurnal: tanggal, akun, debit, kredit.

## 9.1 Skenario Penempatan

### 9.1.1 Penempatan Deposito Rp 3.000.000.000 di Bank Mandiri

Pokok Rp 3.000.000.000, tenor 6 bulan, bunga 5,0000% p.a., dipotong dari rekening operasional.

| **Tanggal** | **Akun**                          | **Debit**     | **Kredit**    |
| ----------- | --------------------------------- | ------------- | ------------- |
| 01/02/2026  | Deposito Berjangka — Bank Mandiri | 3.000.000.000 |               |
| 01/02/2026  | Bank Operasional — Bank Mandiri   |               | 3.000.000.000 |

### 9.1.2 Penempatan Obligasi PT XYZ (FVOCI)

Nominal Rp 5.000.000.000, harga beli 100,5000% (= Rp 5.025.000.000), accrued interest dibeli Rp 25.000.000.

| **Tanggal** | **Akun**                            | **Debit**     | **Kredit**    |
| ----------- | ----------------------------------- | ------------- | ------------- |
| 15/02/2026  | Investasi Obligasi — FVOCI (PT XYZ) | 5.025.000.000 |               |
| 15/02/2026  | Piutang Bunga Obligasi              | 25.000.000    |               |
| 15/02/2026  | Bank Operasional                    |               | 5.050.000.000 |

### 9.1.3 Penempatan Reksadana (FVTPL — Default)

Pembelian 1.000.000 unit Reksadana ABC @ NAB Rp 1.500/unit = Rp 1.500.000.000. Untuk FVTPL, biaya transaksi (jika ada) langsung diakui di P\&L.

| **Tanggal** | **Akun**                              | **Debit**     | **Kredit**    |
| ----------- | ------------------------------------- | ------------- | ------------- |
| 20/02/2026  | Investasi Reksadana — FVTPL (RDN ABC) | 1.500.000.000 |               |
| 20/02/2026  | Bank Operasional                      |               | 1.500.000.000 |

### 9.1.4 Penempatan Reksadana (FVOCI — Kebijakan)

Pembelian 2.000.000 unit Reksadana DEF (RDN Pendapatan Tetap, klasifikasi FVOCI per kebijakan) @ NAB Rp 1.250/unit = Rp 2.500.000.000. Subscription fee 0,5% = Rp 12.500.000. Untuk FVOCI, biaya transaksi DIKAPITALISASI ke nilai tercatat awal.

| **Tanggal** | **Akun**                              | **Debit**     | **Kredit**    |
| ----------- | ------------------------------------- | ------------- | ------------- |
| 20/02/2026  | Investasi Reksadana — FVOCI (RDN DEF) | 2.512.500.000 |               |
| 20/02/2026  | Bank Operasional                      |               | 2.512.500.000 |

### 9.1.5 Penempatan Saham (FVTPL — Default)

Pembelian 100 lot saham BBCA @ Rp 9.500/lembar (1 lot = 100 lembar) = Rp 95.000.000. Komisi broker 0,15% = Rp 142.500. Sesuai PSAK 71, untuk aset FVTPL biaya transaksi langsung diakui di P\&L.

| **Tanggal** | **Akun**                       | **Debit**  | **Kredit** |
| ----------- | ------------------------------ | ---------- | ---------- |
| 10/03/2026  | Investasi Saham — FVTPL (BBCA) | 95.000.000 |            |
| 10/03/2026  | Beban Komisi Broker            | 142.500    |            |
| 10/03/2026  | Bank Operasional               |            | 95.142.500 |

### 9.1.6 Penempatan Saham (FVOCI Election — Strategic Holding)

Pembelian 50.000 lembar saham strategis @ Rp 5.000/lembar = Rp 250.000.000. Komisi broker 0,15% = Rp 375.000. Untuk FVOCI Election, biaya transaksi DIKAPITALISASI ke nilai tercatat (treatment sama dengan FVOCI debt).

| **Tanggal** | **Akun**                                     | **Debit**   | **Kredit**  |
| ----------- | -------------------------------------------- | ----------- | ----------- |
| 10/03/2026  | Investasi Saham — FVOCI Election (Strategic) | 250.375.000 |             |
| 10/03/2026  | Bank Operasional                             |             | 250.375.000 |

## 9.2 Skenario Mutasi / Mark-to-Market

### 9.2.1 MTM Obligasi (FVOCI) — Naik Rp 25.000.000

Carrying sebelumnya Rp 5.025.000.000, fair value baru Rp 5.050.000.000.

| **Tanggal** | **Akun**                               | **Debit**  | **Kredit** |
| ----------- | -------------------------------------- | ---------- | ---------- |
| 28/02/2026  | Investasi Obligasi — FVOCI (PT XYZ)    | 25.000.000 |            |
| 28/02/2026  | OCI — Selisih Penilaian Wajar Obligasi |            | 25.000.000 |

### 9.2.2 MTM Obligasi (FVOCI) — Turun Rp 50.000.000

Carrying Rp 5.050.000.000, fair value baru Rp 5.000.000.000.

| **Tanggal** | **Akun**                               | **Debit**  | **Kredit** |
| ----------- | -------------------------------------- | ---------- | ---------- |
| 31/03/2026  | OCI — Selisih Penilaian Wajar Obligasi | 50.000.000 |            |
| 31/03/2026  | Investasi Obligasi — FVOCI (PT XYZ)    |            | 50.000.000 |

### 9.2.3 MTM Reksadana (FVTPL) — Naik Rp 20.000.000

NAB naik dari Rp 1.500/unit menjadi Rp 1.520/unit; total fair value Rp 1.520.000.000.

| **Tanggal** | **Akun**                                 | **Debit**  | **Kredit** |
| ----------- | ---------------------------------------- | ---------- | ---------- |
| 28/02/2026  | Investasi Reksadana — FVTPL (RDN ABC)    | 20.000.000 |            |
| 28/02/2026  | Keuntungan Belum Direalisasi — Reksadana |            | 20.000.000 |

### 9.2.4 MTM Reksadana (FVTPL) — Turun Rp 30.000.000

NAB turun ke Rp 1.490/unit; total fair value Rp 1.490.000.000 (dari sebelumnya Rp 1.520.000.000).

| **Tanggal** | **Akun**                               | **Debit**  | **Kredit** |
| ----------- | -------------------------------------- | ---------- | ---------- |
| 31/03/2026  | Kerugian Belum Direalisasi — Reksadana | 30.000.000 |            |
| 31/03/2026  | Investasi Reksadana — FVTPL (RDN ABC)  |            | 30.000.000 |

### 9.2.5 MTM Reksadana (FVOCI) — Naik Rp 25.000.000

NAB RDN DEF naik dari Rp 1.250 ke Rp 1.262,5/unit; total fair value Rp 2.525.000.000. Carrying sebelumnya Rp 2.512.500.000 → naik Rp 12.500.000 (selisih NAB) + Rp 12.500.000 (amortisasi biaya transaksi yang dikapitalisasi sebelumnya, untuk ilustrasi, total Rp 25.000.000).

Untuk RDN FVOCI, MTM masuk OCI dengan recycling — saldo OCI akan direklasifikasi ke P\&L saat redemption.

| **Tanggal** | **Akun**                                | **Debit**  | **Kredit** |
| ----------- | --------------------------------------- | ---------- | ---------- |
| 31/03/2026  | Investasi Reksadana — FVOCI (RDN DEF)   | 25.000.000 |            |
| 31/03/2026  | OCI — Selisih Penilaian Wajar Reksadana |            | 25.000.000 |

### 9.2.6 MTM Reksadana (FVOCI) — Turun Rp 40.000.000

| **Tanggal** | **Akun**                                | **Debit**  | **Kredit** |
| ----------- | --------------------------------------- | ---------- | ---------- |
| 30/04/2026  | OCI — Selisih Penilaian Wajar Reksadana | 40.000.000 |            |
| 30/04/2026  | Investasi Reksadana — FVOCI (RDN DEF)   |            | 40.000.000 |

### 9.2.7 MTM Saham (FVTPL) — Naik Rp 5.000.000

Saham BBCA naik dari Rp 9.500 ke Rp 10.000/lembar; carrying naik Rp 95.000.000 → Rp 100.000.000.

| **Tanggal** | **Akun**                             | **Debit** | **Kredit** |
| ----------- | ------------------------------------ | --------- | ---------- |
| 31/03/2026  | Investasi Saham — FVTPL (BBCA)       | 5.000.000 |            |
| 31/03/2026  | Keuntungan Belum Direalisasi — Saham |           | 5.000.000  |

### 9.2.8 MTM Saham (FVTPL) — Turun Rp 8.000.000

| **Tanggal** | **Akun**                           | **Debit** | **Kredit** |
| ----------- | ---------------------------------- | --------- | ---------- |
| 30/04/2026  | Kerugian Belum Direalisasi — Saham | 8.000.000 |            |
| 30/04/2026  | Investasi Saham — FVTPL (BBCA)     |           | 8.000.000  |

### 9.2.9 MTM Saham (FVOCI Election) — Naik Rp 12.500.000

Carrying sebelumnya Rp 250.375.000, fair value baru Rp 262.875.000. Untuk FVOCI Election, selisih MTM masuk OCI dan tidak akan pernah direklasifikasi ke P\&L.

| **Tanggal** | **Akun**                                           | **Debit**  | **Kredit** |
| ----------- | -------------------------------------------------- | ---------- | ---------- |
| 31/03/2026  | Investasi Saham — FVOCI Election                   | 12.500.000 |            |
| 31/03/2026  | OCI — Selisih Penilaian Wajar Saham (No Recycling) |            | 12.500.000 |

### 9.2.10 MTM Saham (FVOCI Election) — Turun Rp 7.000.000

| **Tanggal** | **Akun**                                           | **Debit** | **Kredit** |
| ----------- | -------------------------------------------------- | --------- | ---------- |
| 30/04/2026  | OCI — Selisih Penilaian Wajar Saham (No Recycling) | 7.000.000 |            |
| 30/04/2026  | Investasi Saham — FVOCI Election                   |           | 7.000.000  |

## 9.3 Skenario Pendapatan Bunga / Hasil Investasi

### 9.3.1 Akrual Bunga Deposito (Harian / Bulanan)

Pokok Rp 3.000.000.000 × 5,0000% ÷ 365 = Rp 410.958,90/hari. Akrual untuk 28 hari (Februari) = Rp 11.506.849,32 (dibulatkan ke Rp 11.506.849).

| **Tanggal** | **Akun**                  | **Debit**  | **Kredit** |
| ----------- | ------------------------- | ---------- | ---------- |
| 28/02/2026  | Piutang Bunga Deposito    | 11.506.849 |            |
| 28/02/2026  | Pendapatan Bunga Deposito |            | 11.506.849 |

### 9.3.2 Pembayaran Bunga Deposito Bulanan (jika dibayar bulanan)

Bunga bruto Februari Rp 11.506.849, PPh 4(2) Final 20% = Rp 2.301.370, bunga net Rp 9.205.479.

| **Tanggal** | **Akun**                   | **Debit** | **Kredit** |
| ----------- | -------------------------- | --------- | ---------- |
| 01/03/2026  | Bank Operasional           | 9.205.479 |            |
| 01/03/2026  | Beban Pajak PPh 4(2) Final | 2.301.370 |            |
| 01/03/2026  | Piutang Bunga Deposito     |           | 11.506.849 |

### 9.3.3 Akrual Kupon Obligasi (Bulanan)

Nominal Rp 5.000.000.000 × 7,0000% ÷ 12 = Rp 29.166.666,67/bulan.

| **Tanggal** | **Akun**                  | **Debit**  | **Kredit** |
| ----------- | ------------------------- | ---------- | ---------- |
| 31/03/2026  | Piutang Bunga Obligasi    | 29.166.667 |            |
| 31/03/2026  | Pendapatan Bunga Obligasi |            | 29.166.667 |

### 9.3.4 Penerimaan Kupon Obligasi (Semesteran)

Kupon bruto 6 bulan Rp 175.000.000, PPh Final 10% = Rp 17.500.000, kupon net Rp 157.500.000.

| **Tanggal** | **Akun**                       | **Debit**   | **Kredit**  |
| ----------- | ------------------------------ | ----------- | ----------- |
| 15/08/2026  | Bank Operasional               | 157.500.000 |             |
| 15/08/2026  | Beban Pajak PPh Final Obligasi | 17.500.000  |             |
| 15/08/2026  | Piutang Bunga Obligasi         |             | 175.000.000 |

### 9.3.5 Pendapatan Distribusi Reksadana

Reksadana melakukan pembagian hasil Rp 10.000.000 (jika reksadana model open-ended dengan distribusi tunai).

| **Tanggal** | **Akun**                         | **Debit**  | **Kredit** |
| ----------- | -------------------------------- | ---------- | ---------- |
| 30/06/2026  | Bank Operasional                 | 10.000.000 |            |
| 30/06/2026  | Pendapatan Investasi — Reksadana |            | 10.000.000 |

### 9.3.6 Pendapatan Dividen Saham (FVTPL)

Saham BBCA mengumumkan dividen tunai Rp 250/lembar. Holding 10.000 lembar (100 lot). Dividen bruto Rp 2.500.000. Asumsi PPh Final 10% (untuk WP OP): Rp 250.000. Dividen net Rp 2.250.000.

Step 1 — Pengakuan piutang dividen pada cum-date:

| **Tanggal** | **Akun**           | **Debit** | **Kredit** |
| ----------- | ------------------ | --------- | ---------- |
| 15/04/2026  | Piutang Dividen    | 2.500.000 |            |
| 15/04/2026  | Pendapatan Dividen |           | 2.500.000  |

Step 2 — Penerimaan dividen pada payment date:

| **Tanggal** | **Akun**                           | **Debit** | **Kredit** |
| ----------- | ---------------------------------- | --------- | ---------- |
| 30/04/2026  | Bank Operasional                   | 2.250.000 |            |
| 30/04/2026  | Beban Pajak PPh Final atas Dividen | 250.000   |            |
| 30/04/2026  | Piutang Dividen                    |           | 2.500.000  |

*Catatan WP Badan: Berdasarkan PP 9/2021, dividen dari saham dapat dikecualikan dari objek pajak bila diinvestasikan kembali di Indonesia minimal 30% dari laba setelah pajak. Pencatatan disesuaikan: bila exempt, jurnal langsung Bank/Dividen tanpa baris PPh.*

### 9.3.7 Pendapatan Dividen Saham (FVOCI Election)

Penting: Untuk saham FVOCI Election, dividen TETAP diakui di P\&L (bukan OCI). Hanya MTM yang masuk OCI tanpa recycling. Holding strategis 50.000 lembar @ dividen Rp 100/lembar = Rp 5.000.000 bruto, PPh Final 10% = Rp 500.000, net Rp 4.500.000.

| **Tanggal** | **Akun**                           | **Debit** | **Kredit** |
| ----------- | ---------------------------------- | --------- | ---------- |
| 30/04/2026  | Bank Operasional                   | 4.500.000 |            |
| 30/04/2026  | Beban Pajak PPh Final atas Dividen | 500.000   |            |
| 30/04/2026  | Pendapatan Dividen                 |           | 5.000.000  |

## 9.4 Skenario Renewal Deposito

### 9.4.1 Renewal Skema Pokok Saja (bunga ke rekening)

Deposito Rp 3.000.000.000 jatuh tempo 01/08/2026; bunga 6 bulan bruto Rp 75.000.000, PPh 20% Rp 15.000.000, net Rp 60.000.000. Bunga net masuk ke rekening, pokok di-roll dengan rate baru 5,2500%.

Step 1 — Settle bunga akhir & pencairan akrual:

| **Tanggal** | **Akun**                   | **Debit**  | **Kredit** |
| ----------- | -------------------------- | ---------- | ---------- |
| 01/08/2026  | Bank Operasional           | 60.000.000 |            |
| 01/08/2026  | Beban Pajak PPh 4(2) Final | 15.000.000 |            |
| 01/08/2026  | Piutang Bunga Deposito     |            | 75.000.000 |

Step 2 — Roll-over pokok (kode instrumen lama → baru):

| **Tanggal** | **Akun**                                         | **Debit**     | **Kredit**    |
| ----------- | ------------------------------------------------ | ------------- | ------------- |
| 01/08/2026  | Deposito Berjangka — Bank Mandiri (DEP-002 Baru) | 3.000.000.000 |               |
| 01/08/2026  | Deposito Berjangka — Bank Mandiri (DEP-001 Lama) |               | 3.000.000.000 |

### 9.4.2 Renewal Skema Pokok + Bunga Net (auto-rollover penuh)

Bunga net Rp 60.000.000 digabung ke pokok → pokok baru Rp 3.060.000.000.

| **Tanggal** | **Akun**                                         | **Debit**     | **Kredit**    |
| ----------- | ------------------------------------------------ | ------------- | ------------- |
| 01/08/2026  | Deposito Berjangka — Bank Mandiri (DEP-002 Baru) | 3.060.000.000 |               |
| 01/08/2026  | Beban Pajak PPh 4(2) Final                       | 15.000.000    |               |
| 01/08/2026  | Deposito Berjangka — Bank Mandiri (DEP-001 Lama) |               | 3.000.000.000 |
| 01/08/2026  | Piutang Bunga Deposito                           |               | 75.000.000    |

## 9.5 Skenario Penjualan / Pencairan

### 9.5.1 Pencairan Dini (Break) Deposito

Deposito Rp 3.000.000.000 dicairkan sebelum jatuh tempo. Bank memberlakukan penalti Rp 5.000.000 dan bunga akrual sampai tanggal break Rp 8.000.000 bruto (PPh 20% = Rp 1.600.000).

| **Tanggal** | **Akun**                          | **Debit**     | **Kredit**    |
| ----------- | --------------------------------- | ------------- | ------------- |
| 15/05/2026  | Bank Operasional                  | 3.001.400.000 |               |
| 15/05/2026  | Beban Pajak PPh 4(2) Final        | 1.600.000     |               |
| 15/05/2026  | Beban Penalti Pencairan Dini      | 5.000.000     |               |
| 15/05/2026  | Deposito Berjangka — Bank Mandiri |               | 3.000.000.000 |
| 15/05/2026  | Piutang Bunga Deposito            |               | 8.000.000     |

*Catatan: 3.001.400.000 = 3.000.000.000 + (8.000.000 − 1.600.000) − 5.000.000.*

### 9.5.2 Penjualan Obligasi (FVOCI) di Pasar Sekunder

Carrying Rp 5.000.000.000, akrual sampai tgl jual Rp 30.000.000, harga jual 102,0000% = Rp 5.100.000.000, accrued interest dijual Rp 30.000.000. Saldo OCI cumulative −Rp 25.000.000 (rugi MTM yang belum direalisasi).

Step 1 — Pencatatan penjualan & realisasi:

| **Tanggal** | **Akun**                            | **Debit**     | **Kredit**    |
| ----------- | ----------------------------------- | ------------- | ------------- |
| 15/06/2026  | Bank Operasional                    | 5.130.000.000 |               |
| 15/06/2026  | Investasi Obligasi — FVOCI (PT XYZ) |               | 5.000.000.000 |
| 15/06/2026  | Piutang Bunga Obligasi              |               | 30.000.000    |
| 15/06/2026  | Keuntungan Penjualan Obligasi       |               | 100.000.000   |

Step 2 — Reklasifikasi saldo OCI ke laba/rugi (PSAK 71 untuk debt FVOCI):

| **Tanggal** | **Akun**                               | **Debit**  | **Kredit** |
| ----------- | -------------------------------------- | ---------- | ---------- |
| 15/06/2026  | Kerugian Reklasifikasi dari OCI        | 25.000.000 |            |
| 15/06/2026  | OCI — Selisih Penilaian Wajar Obligasi |            | 25.000.000 |

*Total realized P\&L = Rp 100.000.000 − Rp 25.000.000 = Rp 75.000.000 keuntungan bersih.*

### 9.5.3 Redemption Reksadana (FVTPL)

Redemption 500.000 unit RDN ABC @ NAB Rp 1.490/unit = Rp 745.000.000. Carrying amount per unit di sistem (FVTPL = fair value terakhir) Rp 1.490/unit, jadi tidak ada selisih realized terpisah.

| **Tanggal** | **Akun**                              | **Debit**   | **Kredit**  |
| ----------- | ------------------------------------- | ----------- | ----------- |
| 20/06/2026  | Bank Operasional                      | 745.000.000 |             |
| 20/06/2026  | Investasi Reksadana — FVTPL (RDN ABC) |             | 745.000.000 |

*Catatan: Bila redemption fee 0,5% diberlakukan MI, jurnal tambahan: Beban Redemption Fee Dr 3.725.000 / Investasi Reksadana Cr 3.725.000 sebelum cash diterima.*

### 9.5.4 Redemption Reksadana (FVOCI) — DENGAN Recycling

Redemption 1.000.000 unit RDN DEF @ NAB Rp 1.245/unit = Rp 1.245.000.000. Carrying terakhir di sistem (FV setelah MTM) Rp 1.248,75/unit × 1.000.000 unit = Rp 1.248.750.000 (untuk separuh holding). Akumulasi OCI cumulative atas porsi yang di-redeem: −Rp 7.500.000 (50% dari saldo OCI gain Rp −15M, contoh ilustratif loss).

**Step 1 — Pencatatan redemption pada nilai NAB (proceed cash):**

| **Tanggal** | **Akun**                              | **Debit**     | **Kredit**    |
| ----------- | ------------------------------------- | ------------- | ------------- |
| 20/06/2026  | Bank Operasional                      | 1.245.000.000 |               |
| 20/06/2026  | Kerugian Realized Reksadana (P\&L)    | 3.750.000     |               |
| 20/06/2026  | Investasi Reksadana — FVOCI (RDN DEF) |               | 1.248.750.000 |

**Step 2 — Reklasifikasi (recycling) saldo OCI cumulative ke P\&L:**

| **Tanggal** | **Akun**                                | **Debit** | **Kredit** |
| ----------- | --------------------------------------- | --------- | ---------- |
| 20/06/2026  | Kerugian Reklasifikasi dari OCI (P\&L)  | 7.500.000 |            |
| 20/06/2026  | OCI — Selisih Penilaian Wajar Reksadana |           | 7.500.000  |

*PENTING: Berbeda dengan FVOCI Election Saham (no recycling, OCI ke Saldo Laba Ditahan), FVOCI untuk Reksadana mengikuti pola yang sama dengan FVOCI Debt: akumulasi OCI cumulative DI-REKLASIFIKASI ke P\&L pada saat redemption (recycling).*

### 9.5.5 Penjualan Saham (FVTPL)

Penjualan 50 lot saham BBCA (5.000 lembar) @ harga jual Rp 10.500/lembar = Rp 52.500.000. Carrying terakhir Rp 10.000/lembar (post MTM) = Rp 50.000.000. PPh Final 0,1% atas transaksi penjualan saham di bursa = Rp 52.500. Komisi broker 0,25% = Rp 131.250.

Realized gain dihitung dari selisih harga jual dengan carrying terakhir; komisi & PPh dipotong dari penerimaan kas.

| **Tanggal** | **Akun**                       | **Debit**  | **Kredit** |
| ----------- | ------------------------------ | ---------- | ---------- |
| 20/05/2026  | Bank Operasional               | 52.316.250 |            |
| 20/05/2026  | Beban Komisi Broker            | 131.250    |            |
| 20/05/2026  | Beban Pajak PPh Final 0,1%     | 52.500     |            |
| 20/05/2026  | Investasi Saham — FVTPL (BBCA) |            | 50.000.000 |
| 20/05/2026  | Keuntungan Penjualan Saham     |            | 2.500.000  |

*Catatan: 52.316.250 = 52.500.000 − 131.250 − 52.500.*

### 9.5.6 Penjualan Saham (FVOCI Election) — TANPA Recycling

Penjualan seluruh holding strategis 50.000 lembar @ Rp 5.500/lembar = Rp 275.000.000. Carrying terakhir Rp 5.512/lembar = Rp 275.625.000 (carrying termasuk akumulasi MTM). Saldo akumulasi OCI Rp 5.250.000 (gain MTM cumulative).

Step 1 — Pencatatan penjualan:

| **Tanggal** | **Akun**                         | **Debit**   | **Kredit**  |
| ----------- | -------------------------------- | ----------- | ----------- |
| 20/05/2026  | Bank Operasional                 | 274.725.000 |             |
| 20/05/2026  | Beban Komisi Broker              | 625.000     |             |
| 20/05/2026  | Beban Pajak PPh Final 0,1%       | 275.000     |             |
| 20/05/2026  | Investasi Saham — FVOCI Election |             | 275.625.000 |

Step 2 — Pemindahan saldo OCI ke saldo laba ditahan (TANPA melalui P\&L — no recycling):

| **Tanggal** | **Akun**                                           | **Debit** | **Kredit** |
| ----------- | -------------------------------------------------- | --------- | ---------- |
| 20/05/2026  | OCI — Selisih Penilaian Wajar Saham (No Recycling) | 5.250.000 |            |
| 20/05/2026  | Saldo Laba Ditahan                                 |           | 5.250.000  |

*PENTING: Berbeda dengan FVOCI debt (Bab 9.5.2) yang me-reklasifikasi OCI ke P\&L saat dijual, FVOCI Election untuk saham TIDAK pernah merealisasikan OCI ke P\&L. Saldo OCI dipindahkan ke Saldo Laba Ditahan langsung di ekuitas.*

## 9.6 Skenario Jatuh Tempo (Closure)

### 9.6.1 Jatuh Tempo Deposito (tanpa renewal)

Deposito Rp 3.000.000.000 jatuh tempo 01/08/2026. Bunga akrual sisa Rp 75.000.000 bruto, PPh 20% Rp 15.000.000, net Rp 60.000.000.

| **Tanggal** | **Akun**                          | **Debit**     | **Kredit**    |
| ----------- | --------------------------------- | ------------- | ------------- |
| 01/08/2026  | Bank Operasional                  | 3.060.000.000 |               |
| 01/08/2026  | Beban Pajak PPh 4(2) Final        | 15.000.000    |               |
| 01/08/2026  | Deposito Berjangka — Bank Mandiri |               | 3.000.000.000 |
| 01/08/2026  | Piutang Bunga Deposito            |               | 75.000.000    |

### 9.6.2 Jatuh Tempo Obligasi

Obligasi PT XYZ jatuh tempo 15/02/2029. Pokok Rp 5.000.000.000 + kupon final Rp 175.000.000 (PPh 10% = Rp 17.500.000, net Rp 157.500.000). Saldo OCI cumulative pada saat jatuh tempo 0 (sudah ter-amortisasi).

| **Tanggal** | **Akun**                            | **Debit**     | **Kredit**    |
| ----------- | ----------------------------------- | ------------- | ------------- |
| 15/02/2029  | Bank Operasional                    | 5.157.500.000 |               |
| 15/02/2029  | Beban Pajak PPh Final Obligasi      | 17.500.000    |               |
| 15/02/2029  | Investasi Obligasi — FVOCI (PT XYZ) |               | 5.000.000.000 |
| 15/02/2029  | Piutang Bunga Obligasi              |               | 175.000.000   |

*Reklasifikasi OCI residual (jika masih ada saldo) ke P\&L pada tanggal jatuh tempo.*

## 9.7 Jurnal Pengakuan ECL (Forward-Looking)

Mengacu Rangkuman 8.4. ECL FL untuk Cash, Deposito, dan Obligasi diakui sebagai beban CKPN. Untuk Reksadana FVTPL, tidak ada jurnal akuntansi (risk-management view saja).

### 9.7.1 Cash (Amortized Cost) — ECL FL Rp 107.812,50

Sebelum ada saldo CKPN sebelumnya. Kontra-aset CKPN langsung mengurangi nilai tercatat aset Cash.

| **Tanggal** | **Akun**                          | **Debit** | **Kredit** |
| ----------- | --------------------------------- | --------- | ---------- |
| 28/02/2026  | Beban CKPN — Cash                 | 107.813   |            |
| 28/02/2026  | CKPN — Cash di Bank (Kontra-Aset) |           | 107.813    |

### 9.7.2 Deposito (Amortized Cost) — ECL FL Rp 215.625,00

| **Tanggal** | **Akun**                                | **Debit** | **Kredit** |
| ----------- | --------------------------------------- | --------- | ---------- |
| 28/02/2026  | Beban CKPN — Deposito                   | 215.625   |            |
| 28/02/2026  | CKPN — Deposito Berjangka (Kontra-Aset) |           | 215.625    |

### 9.7.3 Obligasi (FVOCI) — ECL FL Rp 6.565.781,25

Untuk debt FVOCI: Beban CKPN diakui di laba/rugi, sisi kontra-nya di OCI (bukan kontra aset, karena aset sudah disajikan pada nilai wajar).

| **Tanggal** | **Akun**                      | **Debit** | **Kredit** |
| ----------- | ----------------------------- | --------- | ---------- |
| 28/02/2026  | Beban CKPN — Obligasi         | 6.565.781 |            |
| 28/02/2026  | OCI — Akumulasi CKPN Obligasi |           | 6.565.781  |

### 9.7.4 Reksadana (FVOCI) — ECL FL Rp 1.798.312,50

Untuk Reksadana FVOCI dengan klasifikasi RDN Pendapatan Tetap, ECL look-through diakui dengan mekanisme yang SAMA dengan FVOCI Obligasi: beban CKPN ke P\&L, kontra ke OCI. Tidak mengurangi nilai tercatat aset (yang tetap di nilai wajar).

| **Tanggal** | **Akun**                       | **Debit** | **Kredit** |
| ----------- | ------------------------------ | --------- | ---------- |
| 28/02/2026  | Beban CKPN — Reksadana         | 1.798.313 |            |
| 28/02/2026  | OCI — Akumulasi CKPN Reksadana |           | 1.798.313  |

*Catatan: Untuk Reksadana FVTPL → tidak ada jurnal CKPN (risk-management view saja). Untuk Reksadana Saham FVOCI → tidak ada jurnal CKPN (underlying ekuitas tidak menghasilkan ECL).*

### 9.7.5 Penyesuaian ECL Periode Berikutnya (Δ ECL)

Pada periode berikutnya, sistem menghitung ulang ECL FL. Hanya selisih (delta) yang dijurnal:

  - Jika ECL baru \> ECL lama → tambahan beban CKPN (pengakuan tambahan).

  - Jika ECL baru \< ECL lama → pemulihan CKPN (reversal beban).

Contoh: ECL FL Cash periode lalu Rp 107.812,50; periode kini Rp 95.000,00; selisih Rp 12.812,50 → reversal:

| **Tanggal** | **Akun**                    | **Debit** | **Kredit** |
| ----------- | --------------------------- | --------- | ---------- |
| 31/03/2026  | CKPN — Cash di Bank         | 12.813    |            |
| 31/03/2026  | Pemulihan Beban CKPN — Cash |           | 12.813     |

# 10\. Tata Kelola, Kontrol & Audit Trail

## 10.1 Pemisahan Tugas (Segregation of Duties)

| **Role**                    | **Hak Akses Utama**                                                                    |
| --------------------------- | -------------------------------------------------------------------------------------- |
| Maker (Treasury)            | Input transaksi, upload dokumen — tanpa hak posting jurnal.                            |
| Approver (Treasury Manager) | Approve/reject transaksi maker; tidak boleh sebagai maker pada transaksi yang sama.    |
| Risk Officer                | Upload Impact PD, parameter PD/LGD; review hasil ECL; tidak boleh transaksi instrumen. |
| Akuntansi                   | View jurnal otomatis, posting manual adjustment (limited), rekonsiliasi GL.            |
| Auditor                     | Read-only akses penuh, termasuk audit trail dan media upload.                          |
| IT Admin                    | Manajemen user, role, log; tidak ada akses transaksi atau dokumen finansial.           |

## 10.2 Audit Trail

1.  Setiap transaksi mencatat: Created By, Created At, Approved By, Approved At, Last Modified By, Last Modified At.

2.  Setiap upload dokumen mencatat: Uploader, Timestamp, IP, hash SHA-256, file size, file type.

3.  Akses media upload (view/download) dicatat dalam access log dengan timestamp dan user.

4.  Modifikasi parameter master (PD, LGD, Impact PD, LPS) wajib melalui workflow approval dengan log perubahan (before/after value).

## 10.3 Reporting & Dashboard

| **Laporan**                                    | **Konten Utama**                                                                                                                                                                                                                                               |
| ---------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Daftar Posisi Portofolio                       | Per tanggal: tipe instrumen, sub-tipe, counterparty, EAD, MTM, accrued interest, Stage, ECL FL, status.                                                                                                                                                        |
| Mutasi Instrumen                               | Daftar event per periode (penempatan, MTM, jual, renewal, jatuh tempo).                                                                                                                                                                                        |
| P\&L Investasi                                 | Bunga deposito, kupon obligasi, dividen, distribusi reksadana, MTM gain/loss, realized gain/loss, beban PPh.                                                                                                                                                   |
| Akrual Bunga                                   | Per instrumen, harian/bulanan, dengan reconciliation ke GL.                                                                                                                                                                                                    |
| ECL Summary                                    | Per instrumen, per counterparty, per stage, per periode; ECL Weighted, ECL FL, Δ vs periode lalu.                                                                                                                                                              |
| ECL Detail                                     | Tampilan rumusan: EAD × PD (12-Month/Lifetime, 3 skenario) × LGD × Total Impact MEV per instrumen.                                                                                                                                                             |
| **Saldo CKPN (NEW)**                           | Saldo CKPN per instrumen, per tipe, per counterparty, per stage; opening balance, addition, reversal, ending balance; mutasi periode dengan breakdown sumber (origination, SICR migration, default, curing, parameter update); roll-forward bulanan + tahunan. |
| **Stage Distribution Report (NEW)**            | Distribusi exposure per stage (1/2/3) per tipe instrumen, per counterparty; tampilan persentase EAD dan jumlah ECL per stage; trend bulanan; matrix transition stage period-on-period.                                                                         |
| **Counterparty Rating History (NEW)**          | Riwayat rating per counterparty: tanggal berlaku, rating, outlook, action (upgrade/downgrade/affirmed/withdrawn), notch change, SICR/Default trigger flag, dokumen pendukung.                                                                                  |
| **Underlying Reksadana Snapshot (NEW)**        | Per reksadana, per tanggal snapshot: komposisi underlying (kategori, bobot, ISIN, issuer, rating); trend komposisi bulanan; alert ketidaksesuaian dengan sub-tipe (mis. RDN PT yang underlying obligasinya turun di bawah 80%).                                |
| Konsentrasi Risiko                             | Per counterparty, per rating, per tipe instrumen, per sub-tipe instrumen.                                                                                                                                                                                      |
| Dokumen Per Event                              | List dokumen upload per transaksi (link & metadata).                                                                                                                                                                                                           |
| LPS Coverage Report                            | Per bank: total eksposur, LPS coverage, eksposur tak terjamin.                                                                                                                                                                                                 |
| MEV Sensitivity Report                         | Sensitivitas ECL terhadap perubahan komponen MEV (GDP, CPI, BI Rate, USD/IDR); stress test result.                                                                                                                                                             |
| **Status Periode Dashboard (NEW)**             | Timeline 12 bulan terakhir + 12 bulan ke depan dengan kode warna status (OPEN/SOFT\_CLOSED/CLOSED/REOPENED); tenggat soft-close & hard-close per periode; SLA closing.                                                                                         |
| **Closing Audit Trail Report (NEW)**           | Per periode: daftar semua adjustment journal entry yang dibuat pada status SOFT\_CLOSED dan REOPENED\_ADJUSTMENT, dengan tanggal, maker, approver, alasan, dan dokumen pendukung.                                                                              |
| **Posisi Valas Dashboard (NEW)**               | Total exposure per mata uang dalam mata uang asli dan IDR equivalent; trend kurs harian; breakdown per tipe instrumen valas.                                                                                                                                   |
| **FX Gain/Loss Report (NEW)**                  | Per periode: breakdown unrealized vs realized FX gain/loss, per mata uang, per instrumen, dengan jurnal posting reference; dampak P\&L dari fluktuasi kurs.                                                                                                    |
| **FX Rate History Report (NEW)**               | Tabel kurs harian per mata uang dengan source (BI\_JISDOR/BI\_KURS\_TENGAH/UPLOAD\_MANUAL), locked status, revision history, repeat-rate flag.                                                                                                                 |
| **Mapping Coverage Dashboard (NEW)**           | Daftar event mapping dengan status Aktif/Tidak Aktif, jumlah line detail per event; warning untuk event dengan 0 line detail (mapping incomplete).                                                                                                             |
| **Mapping Validation Report (NEW)**            | Daftar event yang gagal balance check pada resolusi runtime per periode, dengan trace amount dan instrumen terkait — incident report untuk Akuntansi.                                                                                                          |
| **Mapping Change History (NEW)**               | Audit trail seluruh perubahan mapping (UI edit + Excel import) dengan user, timestamp, before-after value, dan tipe operasi (CREATE/UPDATE/DELETE/IMPORT).                                                                                                     |
| **CoA Cross-Reference Report (NEW)**           | Per Kode Akun di CoA: daftar event mapping yang menggunakannya. Berguna saat CoA restructuring untuk impact analysis.                                                                                                                                          |
| Amortization Schedule per Instrumen (NEW v1.1) | Per instrumen AC/FVOCI utang: schedule amortisasi periodik dengan kolom Opening Carrying, Cash Inflow, Interest Income (EIR), Amortisasi Premium/Diskonto, Closing Carrying. Drill-down per tanggal posting; export Excel.                                     |
| EIR Summary Report (NEW v1.1)                  | Per instrumen: EIR Awal, EIR Current (post re-estimation), kupon kontraktual, premium/diskonto kapitalisasi, biaya transaksi kapitalisasi, sisa amortisasi belum dibukukan; agregasi per tipe instrumen, per counterparty, per portofolio.                     |
| Roll-Forward Carrying Amount (NEW v1.1)        | Per periode (bulanan/tahunan), per tipe instrumen: Saldo Awal Carrying + Penempatan Baru − Pelunasan Pokok ± Amortisasi Periode − ECL = Saldo Akhir Carrying. Memenuhi disclosure PSAK 71 paragraf 35H.                                                        |
| EIR Re-estimation Log (NEW v1.1)               | Audit trail seluruh re-estimation EIR: tanggal, instrumen, EIR sebelum, EIR sesudah, trigger event (modifikasi kontrak / revisi cash flow estimasi), user approver, dokumen pendukung.                                                                         |

### 10.3.1 Detail Laporan Saldo CKPN

Laporan Saldo CKPN merupakan reporting kunci untuk pemenuhan disclosure PSAK 71. Berikut struktur tabel yang dihasilkan:

**A. Roll-Forward CKPN per Periode (per tipe instrumen):**

| **Komponen Mutasi**                 | **Stage 1** | **Stage 2** | **Stage 3** | **Total** |
| ----------------------------------- | ----------- | ----------- | ----------- | --------- |
| Saldo Awal                          | X           | X           | X           | X         |
| Penambahan: Origination Baru        | \+X         | —           | —           | \+X       |
| Migrasi: Stage 1 → Stage 2          | −X          | \+X         | —           | 0         |
| Migrasi: Stage 2 → Stage 3          | —           | −X          | \+X         | 0         |
| Curing: Stage 2 → Stage 1           | \+X         | −X          | —           | 0         |
| Curing: Stage 3 → Stage 2           | —           | \+X         | −X          | 0         |
| Δ Parameter (PD, LGD, MEV)          | ±X          | ±X          | ±X          | ±X        |
| Δ EAD (mutasi exposure)             | ±X          | ±X          | ±X          | ±X        |
| Penghapusan (Write-off)             | —           | —           | −X          | −X        |
| Pencairan / Jatuh Tempo / Penjualan | −X          | −X          | −X          | −X        |
| **Saldo Akhir**                     | **X**       | **X**       | **X**       | **X**     |

**B. Saldo CKPN per Instrumen (snapshot)**

| **Kode Instrumen** | **Nama / Counterparty** | **Stage** | **EAD**       | **ECL FL**  | **Carrying Net** |
| ------------------ | ----------------------- | --------- | ------------- | ----------- | ---------------- |
| DEP-0001           | Deposito Bank Mandiri   | 1         | 1.666.666.667 | 215.625     | 1.666.451.042    |
| OBL-0001           | Obligasi PT XYZ         | 1         | 5.075.000.000 | 6.565.781   | 5.068.434.219    |
| OBL-0002           | Obligasi PT ABC         | 2         | 3.000.000.000 | 187.500.000 | 2.812.500.000    |
| OBL-0003           | Obligasi PT DEF         | **3**     | 1.000.000.000 | 450.000.000 | 550.000.000      |
| …                  | …                       | …         | …             | …           | …                |

**C. Filter & Drill-Down yang tersedia:**

  - Per Tipe Instrumen / Sub-Tipe

  - Per Counterparty / Per Rating Bucket

  - Per Stage (1, 2, 3)

  - Per Klasifikasi PSAK 71 (AC, FVOCI, FVTPL)

  - Per Periode (bulanan, triwulanan, tahunan)

  - Drill-down dari saldo agregat → instrumen individu → detail event/jurnal

**D. Format Output:**

  - Tampilan dashboard interaktif (web)

  - Export ke Excel/PDF dengan watermark dan timestamp

  - Scheduled email report (bulanan, triwulanan)

  - API endpoint untuk integrasi ke sistem manajemen risiko / regulator (LBU/LBBU)

# 11\. Deliverables & Milestone Implementasi

## 11.1 Deliverables

1.  BRD (Business Requirements Document) — final approved.

2.  FSD (Functional Specification Document) per modul.

3.  Rancangan database (ERD) dan kamus data.

4.  Kode aplikasi (frontend, backend, batch jobs) dengan dokumentasi.

5.  Test cases & UAT scripts (positive + negative + edge cases LPS).

6.  User manual & training materials (Maker, Approver, Risk Officer).

7.  API documentation untuk integrasi GL.

8.  Production deployment package dan runbook operasional.

## 11.2 Milestone Indikatif

| **Fase** | **Aktivitas**           | **Durasi** | **Output Utama**                                                                    |
| -------- | ----------------------- | ---------- | ----------------------------------------------------------------------------------- |
| 1        | Discovery & Requirement | 3 minggu   | BRD final, sign-off                                                                 |
| 2        | Desain Sistem           | 3 minggu   | FSD, ERD, mock-up UI                                                                |
| 3        | Development phase 1     | 4 minggu   | Master Data + Master Portofolio + SPPI & BM Test Engine + Modul Penempatan + Upload |
| 4        | Development phase 2     | 4 minggu   | Modul MTM + Pendapatan + Renewal                                                    |
| 5        | Development phase 3     | 4 minggu   | Modul Jual + Jatuh Tempo + Jurnal & GL Interface                                    |
| 6        | Development phase 4     | 4 minggu   | EIR Engine + ECL Engine (CSH, DEP, OBL, RDN look-through)                           |
| 7        | Development phase 5     | 4 minggu   | Reporting                                                                           |
| 8        | SIT                     | 3 minggu   | System Integration Testing report                                                   |
| 9        | UAT                     | 3 minggu   | User Acceptance Testing sign-off                                                    |
| 10       | Production Deployment   | 1 minggu   | Go-live                                                                             |
| 11       | Hypercare               | 4 minggu   | Stabilization, fine-tuning, knowledge transfer                                      |

*Total estimasi: ± 33 minggu (≈ 8 bulan).*

## 11.3 Acceptance Criteria

1.  Semua field master data dapat di-CRUD dengan validasi schema.

2.  SPPI Test menjalankan checklist Q1–Q10 dengan auto-derive PASS/FAIL; instrumen ber-SPPI FAIL otomatis terkunci sebagai FVTPL.

3.  BM Test menampilkan riwayat penjualan 12-bulan dan auto-suggest kategori HTC/HTC\&S/Other; override hanya bisa dengan justifikasi tertulis.

4.  Matriks klasifikasi 4.3 (SPPI × BM → AC/FVOCI/FVTPL) terimplementasi dan ter-lock pada master instrumen sebelum transaksi penempatan dapat diinput.

5.  Reklasifikasi prospektif menghasilkan jurnal transisi otomatis sesuai matriks 4.5 (6 kombinasi from–to).

6.  Setiap event instrumen memiliki minimal 1 dokumen upload yang ter-link dengan benar.

7.  Hasil perhitungan ECL FL match (manual rekalkulasi) dengan toleransi pembulatan 4 desimal pada rasio.

8.  Jurnal otomatis tervalidasi balance (Σ Debit = Σ Kredit) untuk setiap event.

9.  Logika LPS aggregator menggabungkan Cash + Deposito per Bank dan menghitung EAD tak terjamin secara benar.

10. Look-through Reksadana mampu memecah underlying minimal 3 kategori (sovereign, korporasi, cash) dan menghitung ECL per underlying.

11. Audit trail menampilkan jejak lengkap dari maker, approver, hingga modifikasi parameter.

12. Reporting menampilkan ECL Weighted dan ECL FL dengan presisi 4 desimal pada rasio dan 2 desimal pada IDR.

# 12\. Penutup

Dokumen SoW ini menjadi dasar pembangunan sistem investasi sederhana yang mencakup empat tipe instrumen (Cash, Deposito, Obligasi, Reksadana) dengan siklus hidup penuh, fasilitas media upload, perhitungan ECL berbasis tiga skenario PD dengan penyesuaian forward-looking (Impact PD), dan jurnal otomatis. Penyesuaian terhadap parameter LPS, tarif pajak, mapping rating-PD Pefindo, dan mapping LGD Basel mengikuti regulasi terkini dan ditinjau ulang oleh Komite Risiko/ALCO setiap perubahan kebijakan.

Hal-hal yang belum diatur dalam dokumen ini akan didetailkan pada FSD dan tahap UAT, dengan tetap menjaga konsistensi terhadap PSAK 71 dan kerangka kerja Basel III.

| **Disusun oleh**                                                            | **Disetujui oleh**                                                             |
| --------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ Treasury / Risk Management | \_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_\_ Director / Steering Committee |
