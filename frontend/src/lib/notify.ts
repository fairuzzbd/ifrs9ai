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
