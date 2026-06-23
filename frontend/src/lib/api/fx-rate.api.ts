/**
 * API client — P5-M5 FX Rate Management
 * Mirrors backend routing from api/openapi/app-d-fx-rate.yaml
 *
 * All mutating calls auto-generate Idempotency-Key (via apiPost).
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
} from "@/lib/api";
import type {
  KursListItemP5,
  KursDetailP5,
  KursUploadResponse,
  KursApproveBody,
  KursBatchApproveResponse,
  KursRejectBody,
  KursBatchRejectResponse,
  JisdorSyncTriggerBody,
  JisdorSyncJobResponse,
  KursTreatmentResponse,
} from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Base
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/kurs";

// ---------------------------------------------------------------------------
// List params (P5-M5 shape)
// ---------------------------------------------------------------------------

export interface KursListP5Params {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[kode_mata_uang]"?: string;
  "filter[tanggal_berlaku]"?: string;
  "filter[workflow_status]"?: string;
  "filter[sumber_kurs]"?: string;
  "filter[locked_flag]"?: boolean;
  "filter[deviation_flag]"?: boolean;
  "filter[upload_batch_id]"?: string;
  include_deleted?: boolean;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

export type KursListP5ApiResponse = ListResponse<KursListItemP5>;
export type KursDetailP5ApiResponse = SingleResponse<KursDetailP5>;
export type KursUploadApiResponse = SingleResponse<KursUploadResponse>;
export type KursBatchApproveApiResponse = SingleResponse<KursBatchApproveResponse>;
export type KursBatchRejectApiResponse = SingleResponse<KursBatchRejectResponse>;
export type JisdorSyncApiResponse = SingleResponse<JisdorSyncJobResponse>;
export type KursTreatmentApiResponse = SingleResponse<KursTreatmentResponse>;

// ---------------------------------------------------------------------------
// kursListApi — GET /master/kurs (S1, §1 DataTable)
// ---------------------------------------------------------------------------

export const kursListApi = {
  /**
   * GET /master/kurs — cursor-paginated list with sort + filter + export
   * Default sort: tanggal_berlaku DESC
   * Permission: kurs.read
   */
  list(params: KursListP5Params = {}): Promise<KursListP5ApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<KursListP5ApiResponse>(`${BASE}${qs}`);
  },

  /**
   * GET /master/kurs/{id} — full detail including jisdorFetchMetadata
   * Permission: kurs.read
   */
  get(id: string): Promise<KursDetailP5ApiResponse> {
    return apiGet<KursDetailP5ApiResponse>(`${BASE}/${id}`);
  },

  /**
   * Export URL helper — inline ≤10k rows, async >10k rows (returns 202)
   * Audit: KURS.EXPORT in-transaction
   */
  exportUrl(params: KursListP5Params & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}${qs}`;
  },
};

// ---------------------------------------------------------------------------
// kursUploadApi — POST /master/kurs/upload (S2, multipart)
// ---------------------------------------------------------------------------

/**
 * POST /master/kurs/upload
 * Multipart form-data: file + optional metadata fields.
 * Returns 202 on success (all rows valid) or 422/409/423 on validation fail.
 * Permission: kurs.create (ROLE-AKUN)
 * Idempotency-Key: auto-generated
 */
async function uploadManual(
  file: File,
  opts: {
    catatanUpload?: string;
    tanggalBerlakuOverride?: string;
    idempotencyKey?: string;
  } = {},
): Promise<KursUploadApiResponse> {
  const key = opts.idempotencyKey ?? uuidv4();
  const formData = new FormData();
  formData.append("file", file);
  if (opts.catatanUpload) formData.append("catatanUpload", opts.catatanUpload);
  if (opts.tanggalBerlakuOverride)
    formData.append("tanggalBerlakuOverride", opts.tanggalBerlakuOverride);

  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

  const headers: Record<string, string> = {
    "Idempotency-Key": key,
    Accept: "application/json",
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  // Note: Do NOT set Content-Type for FormData — browser sets multipart boundary

  const response = await fetch(`${API_BASE_URL}${BASE}/upload`, {
    method: "POST",
    headers,
    body: formData,
  });

  if (!response.ok) {
    const { ApiError } = await import("@/lib/api");
    type ErrEnv = { error?: { code: string; message: string; details: { field: string; rule: string; message: string }[]; traceId: string } };
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

  return (await response.json()) as KursUploadApiResponse;
}

export const kursUploadApi = {
  upload: uploadManual,
};

// ---------------------------------------------------------------------------
// kursBatchApi — approve + reject batch (S3, 4-eyes SoD)
// ---------------------------------------------------------------------------

export const kursBatchApi = {
  /**
   * POST /master/kurs/upload/{batch_id}/approve
   * Permission: kurs.approve (ROLE-AKUN-CTL)
   * SoD: approver_id ≠ batch.maker_id (DEC-017)
   * Idempotency-Key: auto-generated
   */
  approve(
    batchId: string,
    body: KursApproveBody,
    idempotencyKey = uuidv4(),
  ): Promise<KursBatchApproveApiResponse> {
    return apiPost<KursBatchApproveApiResponse>(
      `${BASE}/upload/${batchId}/approve`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /master/kurs/upload/{batch_id}/reject
   * Permission: kurs.reject (ROLE-AKUN-CTL)
   * SoD: approver_id ≠ batch.maker_id (DEC-017)
   * body.rejectReason min 20 chars (S3-AC4)
   * Idempotency-Key: auto-generated
   */
  reject(
    batchId: string,
    body: KursRejectBody,
    idempotencyKey = uuidv4(),
  ): Promise<KursBatchRejectApiResponse> {
    return apiPost<KursBatchRejectApiResponse>(
      `${BASE}/upload/${batchId}/reject`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// kursJisdorApi — POST /master/kurs/jisdor-sync (S1, ROLE-IT-ADMIN)
// ---------------------------------------------------------------------------

export const kursJisdorApi = {
  /**
   * POST /master/kurs/jisdor-sync
   * Enqueues Asynq job "fx:jisdor-fetch". Returns 202 + jobId for JobProgressPanel.
   * Permission: kurs.sync (ROLE-IT-ADMIN)
   * MFA: wajib (DEC-026)
   * Rate limit: 10 req/jam
   * Idempotency-Key: auto-generated
   */
  triggerSync(
    body: JisdorSyncTriggerBody = {},
    idempotencyKey = uuidv4(),
  ): Promise<JisdorSyncApiResponse> {
    return apiPost<JisdorSyncApiResponse>(
      `${BASE}/jisdor-sync`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// kursTreatmentApi — GET /master/kurs/treatment/{instrumen_id} (S5)
// ---------------------------------------------------------------------------

export const kursTreatmentApi = {
  /**
   * GET /master/kurs/treatment/{instrumen_id}
   * Returns FX gain/loss treatment routing based on klasifikasi_psak71.
   * compliance-critical: any change to routing logic requires ifrs9-compliance-reviewer BLOCKING.
   * Permission: kurs.read + instrumen.read
   */
  getByInstrumen(instrumenId: string): Promise<KursTreatmentApiResponse> {
    return apiGet<KursTreatmentApiResponse>(`${BASE}/treatment/${instrumenId}`);
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const fxRateQueryKeys = {
  all: ["fx-rate"] as const,
  lists: () => [...fxRateQueryKeys.all, "list"] as const,
  list: (params: KursListP5Params) =>
    [...fxRateQueryKeys.lists(), params] as const,
  details: () => [...fxRateQueryKeys.all, "detail"] as const,
  detail: (id: string) => [...fxRateQueryKeys.details(), id] as const,
  treatments: () => [...fxRateQueryKeys.all, "treatment"] as const,
  treatment: (instrumenId: string) =>
    [...fxRateQueryKeys.treatments(), instrumenId] as const,
  jisdorJobs: () => [...fxRateQueryKeys.all, "jisdor-job"] as const,
  jisdorJob: (jobId: string) =>
    [...fxRateQueryKeys.jisdorJobs(), jobId] as const,
} as const;
