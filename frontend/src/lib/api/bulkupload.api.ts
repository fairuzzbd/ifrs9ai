/**
 * API client — P5-M11 Bulk Upload Master Instrumen
 * Mirrors api/openapi/app-b-bulk-upload.yaml
 *
 * 7 endpoints; all mutating calls auto-inject Idempotency-Key (DEC-021).
 * Upload uses multipart/form-data — NOT JSON.
 * 5 namespaces: upload, batchDetail, dryRun, commit, approve, rollback.
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
  NetworkError,
  type ApiErrorBody,
} from "@/lib/api";
import type {
  BulkUploadBatchSummary,
  BulkUploadBatchDetail,
  BulkUploadRowItem,
  DryRunResult,
  CommitJobResponse,
  ApproveResult,
  ApproveFormInput,
  RollbackRequestFormInput,
  RollbackApproveFormInput,
  RollbackResult,
} from "@/lib/schemas/bulkupload.schema";

// ---------------------------------------------------------------------------
// Base path
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/instrumen/bulk-upload";

// ---------------------------------------------------------------------------
// Param types
// ---------------------------------------------------------------------------

export interface BulkBatchListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  row_status?: string;
  sheet_name?: string;
}

// ---------------------------------------------------------------------------
// Response aliases
// ---------------------------------------------------------------------------

export type BulkUploadApiResponse = SingleResponse<BulkUploadBatchSummary>;
export type BulkBatchDetailApiResponse = {
  data: BulkUploadBatchDetail;
  rows: BulkUploadRowItem[];
  pagination: { nextCursor: string | null; hasMore: boolean; totalEstimate: number | null; limit: number };
  meta: { traceId: string };
};
export type DryRunApiResponse = SingleResponse<DryRunResult>;
export type CommitApiResponse = SingleResponse<CommitJobResponse>;
export type ApproveApiResponse = SingleResponse<ApproveResult>;
export type RollbackResultApiResponse = SingleResponse<RollbackResult>;

// ---------------------------------------------------------------------------
// bulkUploadApi — POST /master/instrumen/bulk-upload  (S1)
// ---------------------------------------------------------------------------

export const bulkUploadApi = {
  /**
   * Upload XLSX 5-sheet. multipart/form-data.
   * Server-side size + MIME check (magic bytes PK\x03\x04).
   * Returns 202 with BulkUploadBatchSummary.
   */
  async upload(
    file: File,
    portofolioId?: string,
    idempotencyKey: string = uuidv4(),
  ): Promise<BulkUploadApiResponse> {
    const formData = new FormData();
    formData.append("file", file);
    if (portofolioId) formData.append("portofolio_id", portofolioId);

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
    };

    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    let response: Response;
    try {
      response = await fetch(`${API_BASE_URL}${BASE}`, {
        method: "POST",
        headers,
        body: formData,
      });
    } catch (cause) {
      throw new NetworkError(cause);
    }

    if (!response.ok) {
      let body: { error?: ApiErrorBody };
      try { body = await response.json(); } catch {
        body = { error: { code: "INTERNAL", message: `HTTP ${response.status}`, details: [], traceId: "" } };
      }
      throw new ApiError(
        response.status,
        body.error ?? { code: "INTERNAL", message: "Terjadi kesalahan server", details: [], traceId: "" },
      );
    }

    return (await response.json()) as BulkUploadApiResponse;
  },
};

// ---------------------------------------------------------------------------
// bulkBatchApi — GET /master/instrumen/bulk-upload/{batch_id}  (detail + rows)
// ---------------------------------------------------------------------------

export const bulkBatchApi = {
  get(batchId: string, params: BulkBatchListParams = {}): Promise<BulkBatchDetailApiResponse> {
    const qs = buildQueryString(params as unknown as Record<string, string | number | boolean | null | undefined>);
    return apiGet<BulkBatchDetailApiResponse>(`${BASE}/${batchId}${qs}`);
  },
};

// ---------------------------------------------------------------------------
// bulkDryRunApi — POST /master/instrumen/bulk-upload/{batch_id}/dry-run  (S2)
// ---------------------------------------------------------------------------

export const bulkDryRunApi = {
  run(
    batchId: string,
    idempotencyKey: string = uuidv4(),
  ): Promise<DryRunApiResponse> {
    return apiPost<DryRunApiResponse>(
      `${BASE}/${batchId}/dry-run`,
      {},
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// bulkCommitApi — POST /master/instrumen/bulk-upload/{batch_id}/commit  (S3)
// ---------------------------------------------------------------------------

export const bulkCommitApi = {
  commit(
    batchId: string,
    idempotencyKey: string = uuidv4(),
  ): Promise<CommitApiResponse> {
    return apiPost<CommitApiResponse>(
      `${BASE}/${batchId}/commit`,
      {},
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// bulkApproveApi — POST /master/instrumen/bulk-upload/{batch_id}/approve  (S4)
// ---------------------------------------------------------------------------

export const bulkApproveApi = {
  approve(
    batchId: string,
    body: ApproveFormInput,
    idempotencyKey: string = uuidv4(),
  ): Promise<ApproveApiResponse> {
    return apiPost<ApproveApiResponse>(
      `${BASE}/${batchId}/approve`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// bulkRollbackApi — POST rollback-request + rollback-approve  (S5)
// ---------------------------------------------------------------------------

export const bulkRollbackApi = {
  request(
    batchId: string,
    body: RollbackRequestFormInput,
    idempotencyKey: string = uuidv4(),
  ): Promise<SingleResponse<{ batchId: string; status: string }>> {
    return apiPost(
      `${BASE}/${batchId}/rollback-request`,
      body,
      idempotencyKey,
    );
  },

  approve(
    batchId: string,
    body: RollbackApproveFormInput,
    stepUpToken: string,
    idempotencyKey: string = uuidv4(),
  ): Promise<RollbackResultApiResponse> {
    // Step-up token passed as extra header — must bypass apiPost helper
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
      "Content-Type": "application/json",
      "X-Step-Up-Token": stepUpToken,
    };
    if (token) headers["Authorization"] = `Bearer ${token}`;

    return fetch(`${API_BASE_URL}${BASE}/${batchId}/rollback-approve`, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    }).then(async (response) => {
      if (!response.ok) {
        let errBody: { error?: ApiErrorBody };
        try { errBody = await response.json(); } catch {
          errBody = { error: { code: "INTERNAL", message: `HTTP ${response.status}`, details: [], traceId: "" } };
        }
        throw new ApiError(
          response.status,
          errBody.error ?? { code: "INTERNAL", message: "Terjadi kesalahan server", details: [], traceId: "" },
        );
      }
      return response.json() as Promise<RollbackResultApiResponse>;
    });
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const bulkUploadQueryKeys = {
  all: ["bulk-upload"] as const,

  batches: () => [...bulkUploadQueryKeys.all, "batch"] as const,
  batchDetail: (batchId: string, params?: BulkBatchListParams) =>
    [...bulkUploadQueryKeys.batches(), batchId, params ?? {}] as const,
} as const;
