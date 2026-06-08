import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  apiDelete,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
  type ApiErrorDetail,
} from "@/lib/api";
import type {
  PeriodeBukuItem,
  PeriodeBukuDetail,
  PeriodeBukuCreateInput,
  PeriodeBukuUpdateInput,
  PeriodeBukuGenerateInput,
  GenerateResult,
} from "@/lib/schemas/periode-buku.schema";

// ---------------------------------------------------------------------------
// Query param interface
// ---------------------------------------------------------------------------

export interface PeriodeBukuListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[tipe_periode]"?: string;
  "filter[status_periode]"?: string;
  "filter[tahun_buku]"?: string | number;
  "filter[workflow_status]"?: string;
  include_deleted?: boolean;
}

export type PeriodeBukuListResponse = ListResponse<PeriodeBukuItem>;
export type PeriodeBukuDetailResponse = SingleResponse<PeriodeBukuDetail>;

// Workflow action response mirrors mata-uang pattern
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

export type PeriodeBukuHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/periode-buku";

// PATCH is not available in the lib/api helpers; use apiPost with method override
// The backend uses PATCH /:id — we call it via baseFetch directly.
// Since api.ts only exports apiPut (PUT), we route PATCH through a raw fetch.
async function apiPatch<T>(
  path: string,
  body: unknown,
  idempotencyKey?: string,
): Promise<T> {
  const { API_BASE_URL } = await import("@/lib/api");
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    Accept: "application/json",
  };
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;
  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: "PATCH",
    headers,
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    let errBody: { error?: { code: string; message: string; details: ApiErrorDetail[]; traceId: string } };
    try {
      errBody = await response.json();
    } catch {
      errBody = {
        error: {
          code: "INTERNAL",
          message: `HTTP ${response.status}`,
          details: [] as ApiErrorDetail[],
          traceId: "",
        },
      };
    }
    const { ApiError } = await import("@/lib/api");
    throw new ApiError(response.status, errBody.error ?? {
      code: "INTERNAL",
      message: "Terjadi kesalahan server",
      details: [] as ApiErrorDetail[],
      traceId: "",
    });
  }

  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export const periodeBukuApi = {
  list(params: PeriodeBukuListParams = {}): Promise<PeriodeBukuListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PeriodeBukuListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<PeriodeBukuDetailResponse> {
    return apiGet<PeriodeBukuDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: PeriodeBukuCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<PeriodeBukuItem>> {
    return apiPost<SingleResponse<PeriodeBukuItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: PeriodeBukuUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<PeriodeBukuItem>> {
    return apiPatch<SingleResponse<PeriodeBukuItem>>(
      `${BASE}/${id}`,
      data,
      idempotencyKey,
    );
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
  ): Promise<PeriodeBukuHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PeriodeBukuHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  generate(
    data: PeriodeBukuGenerateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<GenerateResult>> {
    return apiPost<SingleResponse<GenerateResult>>(
      `${BASE}/generate`,
      data,
      idempotencyKey,
    );
  },

  exportUrl(
    params: PeriodeBukuListParams & { format: "csv" | "xlsx" },
  ): string {
    const qs = buildQueryString(
      params as unknown as Record<
        string,
        string | number | boolean | null | undefined
      >,
    );
    return `${BASE}/export${qs}`;
  },
};
