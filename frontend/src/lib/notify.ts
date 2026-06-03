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
