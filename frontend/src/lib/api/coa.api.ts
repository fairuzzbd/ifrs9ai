import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  apiPut,
  apiDelete,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  CoAItem,
  CoADetail,
  CoACreateInput,
  CoAUpdateInput,
  CoAImportJob,
} from "@/lib/schemas/coa.schema";

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface CoAListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[tipe_akun]"?: string;
  "filter[posisi_normal]"?: string;
  "filter[aktif_flag]"?: boolean;
  "filter[workflow_status]"?: string;
  "filter[sumber_coa]"?: string;
  "filter[mata_uang_native]"?: string;
  include_deleted?: boolean;
}

export type CoAListResponse = ListResponse<CoAItem>;
export type CoADetailResponse = SingleResponse<CoADetail>;

export interface WorkflowActionResponse {
  data: {
    entityId: string;
    entityType: string;
    previousState: string;
    currentState: string;
    action: string;
    performedBy: string;
    performedAt: string;
    signature: {
      signatureHash: string;
      signatureMethod: string;
    };
    nextActions: string[];
    workflowEyes: number;
  };
  meta: { traceId: string };
}

export interface CoAImportJobResponse {
  data: {
    jobId: string;
    type: string;
    statusUrl: string;
    streamUrl: string;
  };
  meta: { traceId: string };
}

export interface CoAImportJobStatusResponse {
  data: CoAImportJob;
  meta: { traceId: string };
}

export interface AuditHistoryEntry {
  eventId: string;
  eventTime: string;
  actorUserId: string;
  actorUsername: string;
  actorRole: string;
  action: string;
  beforeJsonb: Record<string, unknown> | null;
  afterJsonb: Record<string, unknown> | null;
  ip: string | null;
  traceId: string | null;
}

export type CoAHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/coa";

export const coaApi = {
  list(params: CoAListParams = {}): Promise<CoAListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<CoAListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<CoADetailResponse> {
    return apiGet<CoADetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: CoACreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<CoAItem>> {
    return apiPost<SingleResponse<CoAItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: CoAUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<CoAItem>> {
    return apiPut<SingleResponse<CoAItem>>(`${BASE}/${id}`, data, idempotencyKey);
  },

  delete(
    id: string,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<{ deleted: boolean; deletedAt: string; entityId: string }>> {
    return apiDelete(`${BASE}/${id}`, idempotencyKey);
  },

  submit(
    id: string,
    body: { comment?: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${id}/submit`,
      body,
      idempotencyKey,
    );
  },

  review(
    id: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${id}/review`,
      body,
      idempotencyKey,
    );
  },

  approve(
    id: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${id}/approve`,
      body,
      idempotencyKey,
    );
  },

  reject(
    id: string,
    body: { comment: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${id}/reject`,
      body,
      idempotencyKey,
    );
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<CoAHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<CoAHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: CoAListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },

  // ---------------------------------------------------------------------------
  // XLSX Import — returns 202 with jobId
  // ---------------------------------------------------------------------------

  async importXlsx(
    file: File,
    sumberCoa: string,
    idempotencyKey = uuidv4(),
  ): Promise<CoAImportJobResponse> {
    const formData = new FormData();
    formData.append("file", file);
    formData.append("sumber_coa", sumberCoa);

    // Use raw fetch for multipart — baseFetch only supports JSON body
    const apiBase =
      process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081";
    const token =
      typeof window !== "undefined"
        ? localStorage.getItem("blips_token")
        : null;

    const response = await fetch(`${apiBase}${BASE}/import-xlsx`, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Idempotency-Key": idempotencyKey,
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: formData,
    });

    if (!response.ok) {
      let body: { error?: { code: string; message: string; details: []; traceId: string } };
      try {
        body = await response.json();
      } catch {
        body = {
          error: {
            code: "INTERNAL",
            message: `HTTP ${response.status} ${response.statusText}`,
            details: [],
            traceId: "",
          },
        };
      }
      const { ApiError } = await import("@/lib/api");
      throw new ApiError(
        response.status,
        body.error ?? {
          code: "INTERNAL",
          message: "Gagal mengupload file",
          details: [],
          traceId: "",
        },
      );
    }

    return (await response.json()) as CoAImportJobResponse;
  },

  getImportJobStatus(jobId: string): Promise<CoAImportJobStatusResponse> {
    return apiGet<CoAImportJobStatusResponse>(
      `/api/v1/master/coa/import-jobs/${jobId}`,
    );
  },

  // Autocomplete — returns only APPROVED accounts for parent selection
  listApproved(params: { q?: string; limit?: number } = {}): Promise<CoAListResponse> {
    const qs = buildQueryString({
      ...params,
      "filter[workflow_status]": "APPROVED",
      limit: params.limit ?? 20,
    } as Record<string, string | number | boolean | null | undefined>);
    return apiGet<CoAListResponse>(`${BASE}${qs}`);
  },
};
