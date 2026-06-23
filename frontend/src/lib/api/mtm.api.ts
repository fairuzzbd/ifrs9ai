/**
 * API client — P5-M6 MTM Daily Job + Manual Upload
 * Mirrors backend routing from api/openapi/app-b-mtm.yaml
 *
 * All mutating calls auto-generate Idempotency-Key (DEC-021).
 * Multipart upload uses native fetch (not apiPost) to preserve FormData.
 */

import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
  API_BASE_URL,
  ApiError,
} from "@/lib/api";
import type {
  MtmListItem,
  MtmDetail,
  MtmUploadBatchResponse,
  MtmUploadBatchDetail,
  MtmOverrideApproveResponse,
  MtmOverrideRejectResponse,
  MtmCronJobResponse,
  MtmStaleAlertItem,
  MtmAsyncExportJobResponse,
} from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Base
// ---------------------------------------------------------------------------

const BASE = "/api/v1/trx/mtm";

// ---------------------------------------------------------------------------
// List params
// ---------------------------------------------------------------------------

export interface MtmListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[instrumen_id]"?: string;
  "filter[instrumen_kode]"?: string;
  "filter[tanggal_mtm]"?: string;
  "filter[status]"?: string;
  "filter[klasifikasi_psak71]"?: string;
  "filter[deviation_flag]"?: boolean;
  "filter[stale_price_flag]"?: boolean;
  "filter[harga_sumber]"?: string;
  "filter[upload_batch_id]"?: string;
  "filter[periode_bulanan_id]"?: string;
  include_deleted?: boolean;
  export?: "csv" | "xlsx";
}

export interface MtmStaleAlertParams {
  cursor?: string | null;
  limit?: number;
  "filter[tanggal_mtm]"?: string;
  "filter[klasifikasi_psak71]"?: string;
  "filter[harga_sumber]"?: string;
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

export type MtmListApiResponse = ListResponse<MtmListItem>;
export type MtmDetailApiResponse = SingleResponse<MtmDetail>;
export type MtmUploadBatchApiResponse = SingleResponse<MtmUploadBatchResponse>;
export type MtmUploadBatchDetailApiResponse = SingleResponse<MtmUploadBatchDetail>;
export type MtmOverrideApproveApiResponse = SingleResponse<MtmOverrideApproveResponse>;
export type MtmOverrideRejectApiResponse = SingleResponse<MtmOverrideRejectResponse>;
export type MtmCronJobApiResponse = SingleResponse<MtmCronJobResponse>;
export type MtmStaleAlertApiResponse = ListResponse<MtmStaleAlertItem>;
export type MtmExportApiResponse = SingleResponse<MtmAsyncExportJobResponse>;

// ---------------------------------------------------------------------------
// mtmListApi — GET /trx/mtm (S1, S2, S3 — DataTable §1)
// ---------------------------------------------------------------------------

export const mtmListApi = {
  /**
   * GET /trx/mtm — cursor-paginated list with sort + filter + export
   * Default sort: tanggal_mtm DESC, created_at DESC
   * Permission: mtm.read
   */
  list(params: MtmListParams = {}): Promise<MtmListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MtmListApiResponse>(`${BASE}${qs}`);
  },

  /**
   * GET /trx/mtm/{id} — full detail including FCY fields + override history
   * Permission: mtm.read
   */
  get(id: string): Promise<MtmDetailApiResponse> {
    return apiGet<MtmDetailApiResponse>(`${BASE}/${id}`);
  },

  /**
   * Export URL helper — inline ≤10k rows, async >10k rows (returns 202)
   * Audit: MTM.EXPORT in-transaction
   */
  exportUrl(params: MtmListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${API_BASE_URL}${BASE}${qs}`;
  },
};

// ---------------------------------------------------------------------------
// mtmUploadApi — POST /trx/mtm/upload/batch (S2, multipart)
// ---------------------------------------------------------------------------

/**
 * POST /trx/mtm/upload/batch
 * Multipart form-data: file + optional metadata fields.
 * Returns 202 on success (all rows valid) or 422/409/423 on validation fail.
 * Permission: mtm.create (ROLE-AKUN)
 * Idempotency-Key: auto-generated (DEC-021)
 */
async function uploadMtmBatch(
  file: File,
  opts: {
    catatanUpload?: string;
    tanggalMtmOverride?: string;
    idempotencyKey?: string;
  } = {},
): Promise<MtmUploadBatchApiResponse> {
  const key = opts.idempotencyKey ?? uuidv4();
  const formData = new FormData();
  formData.append("file", file);
  if (opts.catatanUpload) formData.append("catatanUpload", opts.catatanUpload);
  if (opts.tanggalMtmOverride)
    formData.append("tanggalMtmOverride", opts.tanggalMtmOverride);

  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

  const headers: Record<string, string> = {
    "Idempotency-Key": key,
    Accept: "application/json",
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  // Note: Do NOT set Content-Type for FormData — browser sets multipart boundary

  const response = await fetch(`${API_BASE_URL}${BASE}/upload/batch`, {
    method: "POST",
    headers,
    body: formData,
  });

  if (!response.ok) {
    type ErrEnv = {
      error?: { code: string; message: string; details: { field: string; rule: string; message: string }[]; traceId: string };
    };
    let errBody: ErrEnv = {};
    try {
      errBody = (await response.json()) as ErrEnv;
    } catch {
      // ignore parse error
    }
    throw new ApiError(response.status, errBody.error ?? {
      code: "INTERNAL",
      message: `HTTP ${response.status}`,
      details: [],
      traceId: "",
    });
  }

  return (await response.json()) as MtmUploadBatchApiResponse;
}

export const mtmUploadApi = {
  uploadBatch: uploadMtmBatch,

  /**
   * GET /trx/mtm/upload/batch/{batch_id} — batch detail with row breakdown
   * Permission: mtm.read (ROLE-AKUN own batches only; ROLE-AKUN-CTL, ROLE-AUDIT all)
   */
  getBatch(batchId: string): Promise<MtmUploadBatchDetailApiResponse> {
    return apiGet<MtmUploadBatchDetailApiResponse>(`${BASE}/upload/batch/${batchId}`);
  },
};

// ---------------------------------------------------------------------------
// mtmOverrideApi — override-approve + override-reject (S4, SoD enforced)
// ---------------------------------------------------------------------------

export const mtmOverrideApi = {
  /**
   * POST /trx/mtm/{id}/override-approve
   * PENDING_REVIEW → APPROVED; call P5-M2 jurnal engine in-transaction.
   * Permission: mtm.override (ROLE-AKUN-CTL)
   * SoD: override_approver_id ≠ uploader_id (DEC-017) — S4-AC3
   * Idempotency-Key: auto-generated (DEC-021)
   */
  approve(
    mtmId: string,
    body: { comment: string; signatureMethod: "JWT_STEP_UP" },
    idempotencyKey = uuidv4(),
  ): Promise<MtmOverrideApproveApiResponse> {
    return apiPost<MtmOverrideApproveApiResponse>(
      `${BASE}/${mtmId}/override-approve`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /trx/mtm/{id}/override-reject
   * PENDING_REVIEW → REJECTED; comment ≥ 30 char WAJIB (S4-AC4); no jurnal posting.
   * Permission: mtm.override (ROLE-AKUN-CTL)
   * SoD: override_approver_id ≠ uploader_id (DEC-017)
   * Idempotency-Key: auto-generated (DEC-021)
   */
  reject(
    mtmId: string,
    body: { comment: string; signatureMethod: "JWT_STEP_UP" },
    idempotencyKey = uuidv4(),
  ): Promise<MtmOverrideRejectApiResponse> {
    return apiPost<MtmOverrideRejectApiResponse>(
      `${BASE}/${mtmId}/override-reject`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// mtmCronApi — POST /trx/mtm/cron/trigger (S1, ROLE-IT-ADMIN)
// ---------------------------------------------------------------------------

export const mtmCronApi = {
  /**
   * POST /trx/mtm/cron/trigger
   * Enqueues Asynq job "trx:mtm_daily_run". Returns 202 + jobId for JobProgressPanel (§3 UX).
   * Permission: mtm.trigger (ROLE-IT-ADMIN)
   * MFA: wajib (DEC-026)
   * Rate limit: 10 req/jam per user
   * Idempotency-Key: auto-generated (DEC-021)
   */
  trigger(
    body: { tanggalTarget?: string; forceRerun?: boolean } = {},
    idempotencyKey = uuidv4(),
  ): Promise<MtmCronJobApiResponse> {
    return apiPost<MtmCronJobApiResponse>(
      `${BASE}/cron/trigger`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// mtmAlertsApi — GET /trx/mtm/alerts/stale-price (S3)
// ---------------------------------------------------------------------------

export const mtmAlertsApi = {
  /**
   * GET /trx/mtm/alerts/stale-price — list STALE_PRICE alerts
   * Default sort: harga_age_days DESC (paling lama pertama)
   * Permission: mtm.read (ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT, ROLE-IT-ADMIN)
   */
  stalePrice(params: MtmStaleAlertParams = {}): Promise<MtmStaleAlertApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MtmStaleAlertApiResponse>(`${BASE}/alerts/stale-price${qs}`);
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const mtmQueryKeys = {
  all: ["mtm"] as const,

  // List
  lists: () => [...mtmQueryKeys.all, "list"] as const,
  list: (params: MtmListParams) => [...mtmQueryKeys.lists(), params] as const,

  // Detail
  details: () => [...mtmQueryKeys.all, "detail"] as const,
  detail: (id: string) => [...mtmQueryKeys.details(), id] as const,

  // Upload batch
  batches: () => [...mtmQueryKeys.all, "batch"] as const,
  batch: (batchId: string) => [...mtmQueryKeys.batches(), batchId] as const,

  // Stale alerts
  staleAlerts: () => [...mtmQueryKeys.all, "stale-alerts"] as const,
  staleAlert: (params: MtmStaleAlertParams) =>
    [...mtmQueryKeys.staleAlerts(), params] as const,

  // Price history (reuse list with instrumen filter)
  priceHistory: (instrumenId: string) =>
    [...mtmQueryKeys.all, "price-history", instrumenId] as const,

  // Job status (for cron trigger progress)
  jobs: () => [...mtmQueryKeys.all, "job"] as const,
  job: (jobId: string) => [...mtmQueryKeys.jobs(), jobId] as const,
} as const;
