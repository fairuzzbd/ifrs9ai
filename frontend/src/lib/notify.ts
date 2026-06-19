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
