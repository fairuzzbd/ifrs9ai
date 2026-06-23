import { toast } from "sonner";
import type { ApiError } from "@/lib/api";

interface NotifyAction {
  label: string;
  onClick: () => void;
}

interface NotifyOpts {
  action?: NotifyAction;
}

// ---------------------------------------------------------------------------
// Error code → pesan Bahasa Indonesia
// ---------------------------------------------------------------------------

const ERROR_MESSAGE_MAP: Record<string, string> = {
  SOD_VIOLATION:
    "Anda tidak bisa menjadi reviewer/approver untuk data yang Anda buat sendiri (Segregation of Duties).",
  IDEMPOTENCY_REPLAY: "Request ini sudah pernah berhasil sebelumnya.",
  IDEMPOTENCY_MISMATCH:
    "Idempotency-Key sudah dipakai dengan payload berbeda. Coba lagi atau hubungi IT.",
  PERIODE_CLOSED:
    "Periode buku sudah hard-closed, tidak bisa di-mutate.",
  ECL_PARAM_FROZEN:
    "Calc run sudah di-seal, parameter tidak bisa diubah.",
  SYSTEM_CURRENCY_PROTECTED:
    "Mata uang fungsional sistem tidak bisa dihapus atau diubah kodenya.",
  ENTITY_IN_USE:
    "Record tidak bisa dihapus karena masih digunakan oleh entitas lain.",
  MASTER_APPROVED_NO_EDIT:
    "Record yang sudah disetujui tidak bisa diubah langsung. Ajukan permintaan perubahan.",
  WORKFLOW_INVALID_TRANSITION:
    "Transisi workflow tidak valid dari status saat ini.",
  UNAUTHORIZED: "Sesi Anda telah berakhir. Silakan login kembali.",
  FORBIDDEN: "Anda tidak memiliki izin untuk melakukan aksi ini.",
  NOT_FOUND: "Data yang diminta tidak ditemukan.",
  CONFLICT:
    "Data telah diubah oleh pengguna lain. Muat ulang halaman untuk melihat data terbaru.",
  RATE_LIMITED: "Terlalu banyak request. Coba lagi dalam beberapa saat.",
  MFA_REQUIRED:
    "Verifikasi Multi-Factor Authentication diperlukan. Silakan login ulang dengan MFA.",
  NETWORK_ERROR: "Tidak dapat terhubung ke server. Periksa koneksi internet Anda.",
  CALC_RUN_PERIODE_ALREADY_SEALED:
    "Periode ini sudah memiliki calc run yang di-segel. Override memerlukan persetujuan ALCO — fitur belum tersedia.",
  ECL_PARAM_NOT_FOUND:
    "Parameter ECL untuk periode ini belum disetujui ALCO. Hubungi ROLE-ALCO.",
  CALC_RUN_NOT_FOUND: "Calc run tidak ditemukan.",
  CALC_RUN_INVALID_STATUS:
    "Aksi tidak valid untuk status calc run saat ini.",

  // --- Jurnal Engine (P5-M2) ---
  JURNAL_EVENT_NOT_MAPPED:
    "Tidak ada mapping jurnal aktif untuk kode event ini. Hubungi Akuntansi untuk membuat mapping.",
  JURNAL_KLASIFIKASI_NOT_ELIGIBLE:
    "Klasifikasi instrumen tidak eligible untuk event ini. Periksa KLASIFIKASI_COMPATIBILITY.",
  JURNAL_BALANCE_INVARIANT:
    "Jurnal tidak seimbang — total DEBIT ≠ total KREDIT. Periksa template mapping.",
  JURNAL_PERIODE_HARD_CLOSED:
    "Periode buku sudah hard-closed, jurnal tidak bisa diposting ke periode ini.",
  JURNAL_DUPLICATE_POST:
    "Jurnal duplikat — posting dengan idempotency key yang sama sudah ada. Tidak ada aksi diperlukan.",
  JURNAL_INVALID_TRANSITION:
    "Transisi status jurnal tidak valid dari status saat ini.",
  JURNAL_SOD_VIOLATION:
    "Anda tidak bisa mereview/approve mapping jurnal yang Anda buat sendiri (SoD).",
  JURNAL_STEP_UP_REQUIRED:
    "Langkah ini memerlukan verifikasi MFA step-up. Silakan verifikasi MFA terlebih dahulu.",
  JURNAL_AMOUNT_INVALID:
    "Nominal jurnal tidak valid. Pastikan nominal lebih dari 0 dan format benar.",
  JURNAL_INSTRUMEN_NOT_FOUND:
    "Instrumen tidak ditemukan. Pastikan ID instrumen benar.",
  JURNAL_HEADER_NOT_FOUND:
    "Header jurnal tidak ditemukan.",
  JURNAL_DLQ_NOT_FOUND:
    "Entri DLQ tidak ditemukan.",
  JURNAL_DLQ_ALREADY_REPLAYED:
    "Entri DLQ ini sudah berhasil di-replay sebelumnya.",
  JURNAL_DLQ_DISCARD_REASON_TOO_SHORT:
    "Alasan discard terlalu pendek — minimal 30 karakter diperlukan.",
  JURNAL_DLQ_REPLAY_PERIODE_HARD_CLOSED:
    "Tidak bisa replay jurnal ke periode yang sudah hard-closed.",
  JURNAL_MAPPING_WORKFLOW_GATE:
    "Mapping jurnal belum disetujui atau tidak aktif. Tidak bisa digunakan untuk posting.",

  // --- Periode Close (P5-M4) ---
  CLOSING_CHECKLIST_FAILED:
    "Closing checklist belum semua lulus. Selesaikan item yang gagal sebelum melanjutkan.",
  CLOSING_CHECKLIST_STALE:
    "Data checklist sudah lebih dari 24 jam. Sistem akan mengevaluasi ulang secara otomatis.",
  PERIODE_SOFT_CLOSED:
    "Periode buku sudah soft-closed. Mutasi transaksi dan jurnal tidak diizinkan. GL delivery retry masih diperbolehkan.",
  MFA_STEP_UP_REQUIRED:
    "Aksi ini memerlukan verifikasi step-up MFA. Silakan verifikasi TOTP terlebih dahulu.",
  MFA_STEP_UP_EXPIRED:
    "Token step-up MFA sudah expired (> 5 menit). Harap ulangi verifikasi dari awal.",
  PERIODE_GRACE_EXPIRED:
    "Grace window 48 jam sudah berakhir. Periode yang sudah CLOSED tidak dapat di-reopen lagi.",
  SOFT_CLOSE_PENDING_EXISTS:
    "Sudah ada request soft-close yang menunggu approval. Tidak bisa mengajukan request baru.",

  // --- FX Rate (P5-M5) ---
  FX_RATE_LOCKED:
    "Kurs sudah dikunci karena periode hard-closed. Tidak bisa ditambah atau diubah. Hubungi CFO untuk reopen dalam grace window.",
  KURS_DUPLICATE_DATE:
    "Kurs untuk tanggal dan mata uang ini sudah ada (APPROVED atau PENDING_APPROVAL). Tidak bisa di-override via manual upload. Jika perlu koreksi, hubungi Finance Controller.",
  KURS_UPLOAD_VALIDATION_FAILED:
    "File upload kurs gagal validasi. Periksa detail per baris — perbaiki file lalu upload ulang.",
  KLASIFIKASI_NOT_LOCKED:
    "Instrumen belum memiliki klasifikasi PSAK 71 yang final (locked). FX treatment tidak dapat ditentukan. Selesaikan SPPI Test + BM Assessment + Klasifikasi Approval terlebih dahulu.",
  KURS_PERIODE_MISMATCH:
    "Tanggal berlaku kurs tidak berada dalam periode buku manapun yang OPEN. Pastikan tanggal benar atau hubungi Finance Controller untuk membuka periode.",

  // --- MTM Daily (P5-M6) ---
  MTM_PRICE_STALE:
    "Kurs mata uang asing untuk tanggal tersebut belum tersedia (APPROVED). Upload kurs manual via halaman Kurs terlebih dahulu.",
  MTM_PRICE_DEVIATION_REJECTED:
    "MTM ditolak karena deviasi harga melebihi threshold. ROLE-AKUN telah dinotifikasi untuk re-upload dengan harga yang benar.",
  MTM_BATCH_NOT_FOUND:
    "Batch upload MTM tidak ditemukan. Pastikan batch_id benar dan Anda memiliki akses ke batch tersebut.",
  MTM_OVERRIDE_SOD_VIOLATION:
    "Anda tidak dapat menyetujui MTM yang Anda upload sendiri. SoD: override-approver ≠ uploader (DEC-017).",
  MTM_INSTRUMEN_AC_SKIP:
    "Instrumen berklasifikasi AC — tidak ada MTM untuk AC per PSAK 71. Hapus baris instrumen AC dari file upload.",
  MTM_PERIODE_LOCKED:
    "Periode buku sudah hard-closed. Tidak bisa menambah atau mengubah MTM untuk periode ini.",

  // --- Renewal Deposito (P5-M7) ---
  RENEWAL_INSTRUMEN_NOT_ELIGIBLE:
    "Instrumen tidak eligible untuk renewal. Pastikan instrumen adalah deposito ACTIVE dengan klasifikasi final.",
  RENEWAL_SKEMA_INVALID:
    "Skema renewal tidak valid. Pilih 'Pokok Saja' atau 'Pokok + Bunga'.",
  RENEWAL_TENOR_OUT_OF_RANGE:
    "Tenor di luar range 1–60 bulan. Masukkan nilai antara 1 dan 60.",
  RENEWAL_RATE_OUT_OF_RANGE:
    "Rate di luar range 0%–30%. Masukkan nilai antara 0 dan 30.",
  RENEWAL_BUNGA_BERSIH_TOO_SMALL:
    "Bunga bersih kurang dari minimum IDR 100.000 untuk skema Pokok + Bunga. Gunakan skema Pokok Saja atau pilih instrumen dengan nominal lebih besar.",
  RENEWAL_PPH_CALC_MISMATCH:
    "Nilai PPh 20% tidak sesuai kalkulasi server. Muat ulang preview dan gunakan nilai yang ditampilkan sistem.",

  // --- Penjualan/Pencairan (P5-M8) ---
  PENJUALAN_INSTRUMEN_NOT_ACTIVE:
    "Instrumen tidak eligible untuk penjualan: harus berstatus ACTIVE dengan klasifikasi PSAK 71 terkunci.",
  PENJUALAN_QTY_EXCEEDS_HOLDING:
    "Qty terjual melebihi qty holding saat ini. Periksa saldo instrumen dan kurangi qty terjual.",
  PENJUALAN_KLASIFIKASI_NOT_LOCKED:
    "Klasifikasi PSAK 71 instrumen belum final (locked). Selesaikan SPPI Test + BM Assessment + Klasifikasi Approval terlebih dahulu.",
  PENJUALAN_HARGA_INVALID:
    "Harga jual per unit tidak valid. Pastikan harga lebih dari 0 dan format angka benar.",
  PENJUALAN_PERIODE_LOCKED:
    "Periode buku sudah hard-closed. Penjualan tidak bisa diposting ke periode ini. Hubungi Finance Controller.",
  PENJUALAN_BM_VIOLATION_BLOCK:
    "Penjualan menyebabkan disposal kumulatif 12-bulan melampaui hard limit. Approval ROLE-RISK diperlukan sebelum penjualan ini bisa diposting.",
  PENJUALAN_FVOCI_ELECTION_NO_RECYCLING_WARN:
    "Penjualan FVOCI Election berhasil. Gain/loss tetap di OCI per PSAK 71 §B5.7.1 — tidak direkognisi di P&L.",

  // --- Jatuh Tempo + Akrual (P5-M9) ---
  MATURITY_INSTRUMEN_NOT_ACTIVE:
    "Instrumen tidak eligible untuk proses jatuh tempo: harus berstatus ACTIVE. Instrumen ini dikirim ke DLQ — proses instrumen lain dilanjutkan.",
  AKRUAL_STAGING_STALE:
    "ECL sealed run terakhir lebih dari batas staleness (AKRUAL_STAGING_STALE_DAYS hari). Akrual Stage 3 mungkin tidak akurat. Hubungi ROLE-AKUN-CTL untuk konfirmasi atau trigger ECL rerun.",
  AKRUAL_FX_RATE_MISSING:
    "Kurs (FX rate) status APPROVED tidak tersedia untuk mata uang dan tanggal akrual ini. Upload kurs manual via halaman Kurs terlebih dahulu.",
  AKRUAL_PERIODE_LOCKED:
    "Periode buku sudah hard-closed. Akrual tidak bisa diposting ke periode ini.",
  AKRUAL_DUPLICATE:
    "Duplikat akrual terdeteksi untuk instrumen + tanggal + jenis yang sama (idempotency guard). Tidak ada aksi diperlukan — instrumen lain dilanjutkan.",
  AKRUAL_EIR_NOT_FOUND:
    "Tidak ada amortisasi schedule aktif (ecl.amortisasi_schedule) untuk instrumen ini. Hubungi tim APP-C untuk setup EIR schedule terlebih dahulu.",
  DIVIDEN_VALIDATION_FAILED:
    "Input dividen tidak valid. Pastikan gross_dividen_IDR lebih dari 0 dan semua field wajib diisi.",

  // --- POCI Delta ECL (P5-M10) ---
  POCI_BASELINE_MISSING:
    "Baseline POCI tidak ditemukan untuk instrumen ini. Pastikan penempatan POCI sudah di-approve dan baseline ter-capture (S1). Instrumen ini dilewati — run dilanjutkan.",
  POCI_BASELINE_IMMUTABLE_VIOLATION:
    "Baseline POCI untuk instrumen ini sudah ada dan tidak dapat di-overwrite (WORM per DEC-018). Baseline bersifat immutable sejak origination.",
  POCI_DELTA_DUPLICATE:
    "Delta POCI duplikat: kombinasi (calc_run, instrumen) sudah ada (idempotency guard). Tidak ada aksi diperlukan — instrumen lain dilanjutkan.",
  POCI_INSTRUMEN_NOT_POCI:
    "Instrumen ini bukan POCI (is_poci = FALSE). Endpoint POCI khusus hanya untuk instrumen dengan flag POCI aktif.",
  POCI_PERIODE_LOCKED:
    "Periode buku sudah CLOSED. Delta POCI tidak dapat diposting ke periode ini. Hubungi Finance Controller.",
  POCI_JURNAL_DIRECTION_MISMATCH:
    "Inkonsistensi data: sign delta_ecl tidak sesuai direction enum. Posting dibatalkan — alert dikirim ke ROLE-IT-ADMIN dan ROLE-RISK untuk investigasi.",

  // --- Mapping Jurnal P5-M12 ---
  MAPPING_EVENT_NOT_FOUND:
    "Kode event mapping tidak ditemukan di sistem. Pastikan event_code benar.",
  MAPPING_AKUN_INVALID:
    "Akun debit atau kredit tidak ditemukan di Chart of Accounts. Pastikan kode akun sudah terdaftar dan aktif.",
  MAPPING_UNBALANCED:
    "Mapping tidak seimbang — total baris debit ≠ total baris kredit. Jurnal harus balanced per PSAK 71.",
  MAPPING_REGULATED_REQUIRES_RISK:
    "Event ini adalah event regulated dan memerlukan jalur 6-eyes (ROLE-RISK sebagai approver-2). Hubungi ROLE-RISK untuk approval.",
  MAPPING_DUPLICATE_VERSION:
    "Sudah ada versi DRAFT atau PENDING untuk event ini. Selesaikan atau tolak versi yang ada sebelum membuat versi baru.",
  MAPPING_SOD_VIOLATION:
    "SoD: Anda tidak dapat menjadi reviewer/approver untuk mapping yang Anda buat sendiri (DEC-017).",
  MAPPING_PERIODE_LOCKED:
    "Periode buku sudah HARD_CLOSED. Perubahan mapping tidak dapat diaktifkan di periode ini. Hubungi Finance Controller.",

  // --- Bulk Upload (P5-M11) ---
  BULK_FILE_TOO_LARGE:
    "Ukuran file melebihi batas 50MB. Kompres atau pisah file lalu upload ulang.",
  BULK_MIME_INVALID:
    "Tipe file tidak valid. Hanya XLSX (format Office Open XML) yang diterima. Pastikan file bukan CSV atau XLS lama.",
  BULK_DRY_RUN_EXPIRED:
    "Sesi DRY_RUN sudah expired (1 jam). Jalankan ulang DRY_RUN sebelum commit.",
  BULK_DRY_RUN_FAILED:
    "COMMIT tidak dapat diproses karena DRY_RUN masih FAILED. Perbaiki baris yang bermasalah dan upload ulang file.",
  BULK_PERIODE_LOCKED:
    "Periode buku sudah CLOSED. Bulk upload / commit tidak dapat diproses. Hubungi Finance Controller.",
  BULK_ROLLBACK_GRACE_EXPIRED:
    "Grace window rollback (default 7 hari) sudah berakhir. Rollback tidak dapat dilakukan. Eskalasi ke ROLE-IT-ADMIN.",
  BULK_APPROVE_SOD_VIOLATION:
    "SoD: Maker tidak dapat menjadi approver untuk batch yang sama (DEC-017). Gunakan akun ROLE-APPR-TR yang berbeda.",

  // --- Reporting MV + Export + Scheduled Email (P5-M13) ---
  EXPORT_TOO_LARGE:
    "Dataset melebihi batas 100.000 baris per export. Gunakan filter untuk mempersempit data sebelum export.",
  EXPORT_PERMISSION_DENIED:
    "Anda tidak punya permission untuk export laporan ini. Hubungi ROLE-IT-ADMIN untuk request akses.",
  EXPORT_FORMAT_UNSUPPORTED:
    "Format export tidak didukung. Format tersedia: CSV, XLSX, PDF.",
  MV_REFRESH_LOCKED:
    "Refresh Materialized View sedang berjalan. Coba lagi setelah proses refresh selesai.",
  MV_REFRESH_FAILED:
    "Refresh Materialized View gagal. Periksa DLQ dan log sistem. Hubungi ROLE-IT-ADMIN.",
  SCHEDULED_EMAIL_SMTP_FAILED:
    "Pengiriman email terjadwal gagal setelah 3x percobaan. Job dipindah ke DLQ. Periksa konfigurasi SMTP.",

  // --- GL Delivery (P5-M3) ---
  GL_DELIVERY_JURNAL_NOT_FOUND:
    "Jurnal header tidak ditemukan.",
  GL_DELIVERY_REASON_TOO_SHORT:
    "Alasan retry / discard wajib minimal 30 karakter.",
  GL_DELIVERY_INVALID_TRANSITION:
    "Jurnal memiliki status DEAD_LETTER dan tidak dapat di-retry. Hubungi ROLE-IT-ADMIN.",
  GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED:
    "Batas maksimum percobaan delivery tercapai (5/5). Hubungi ROLE-IT-ADMIN untuk tindak lanjut.",
  GL_DELIVERY_PERMISSION_DENIED:
    "Anda tidak memiliki izin untuk tindakan ini. Hanya ROLE-IT-ADMIN yang dapat mendiscard entry GL Delivery DLQ.",
  GL_DELIVERY_HOST_4XX:
    "GL Host menolak payload (domain error). Perbaiki penyebab kegagalan sebelum retry.",
  GL_DELIVERY_HOST_UNREACHABLE:
    "GL Host tidak dapat dijangkau (infra error). Coba retry setelah GL Host pulih.",
  GL_DLQ_REPLAY_INVALID_STATE:
    "DLQ entry tidak bisa di-replay dari status saat ini. Pastikan entry berstatus FAILED.",
  GL_RECONCILIATION_REPORT_NOT_FOUND:
    "Belum ada laporan rekonsiliasi untuk tanggal ini. Jalankan rekonsiliasi manual.",
  GL_RECONCILIATION_DATE_INVALID:
    "Tanggal tidak valid atau merupakan hari libur. Rekonsiliasi hanya untuk hari kerja.",
  GL_RECONCILIATION_IN_PROGRESS:
    "Rekonsiliasi untuk tanggal ini sedang berjalan. Tunggu proses selesai sebelum menjalankan ulang.",
  GL_RECONCILIATION_HOST_FETCH_FAILED:
    "Gagal mengambil data dari GL Host saat rekonsiliasi. Periksa koneksi GL Host.",
};

function formatError(err: ApiError | { code: string; message: string; traceId: string }): string {
  return ERROR_MESSAGE_MAP[err.code] ?? err.message ?? "Terjadi kesalahan tidak diketahui.";
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export const notify = {
  /** Sukses — toast hijau, auto-dismiss 4 detik */
  success(msg: string, opts?: NotifyOpts) {
    toast.success(msg, {
      duration: 4000,
      action: opts?.action
        ? { label: opts.action.label, onClick: opts.action.onClick }
        : undefined,
    });
  },

  /** Error — toast merah persistent (manual close) */
  error(
    err: ApiError | { code: string; message: string; traceId: string },
    opts?: NotifyOpts,
  ) {
    const msg = formatError(err);
    toast.error(msg, {
      duration: Infinity,
      action: opts?.action
        ? { label: opts.action.label, onClick: opts.action.onClick }
        : {
            label: "Salin ID",
            onClick: () => {
              if (err.traceId) {
                void navigator.clipboard.writeText(err.traceId);
              }
            },
          },
      description: err.traceId
        ? `${err.code} · trace: ${err.traceId.slice(0, 12)}`
        : err.code,
    });
  },

  /** Warning — amber, auto-dismiss 8 detik */
  warning(msg: string) {
    toast.warning(msg, { duration: 8000 });
  },

  /** Info — blue, auto-dismiss 4 detik */
  info(msg: string) {
    toast.info(msg, { duration: 4000 });
  },

  /** Destructive action sukses (delete, reject) — toast merah, 4 detik */
  destructive(msg: string) {
    toast.error(msg, { duration: 4000 });
  },
};

export { formatError };
