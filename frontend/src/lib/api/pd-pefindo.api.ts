import { v4 as uuidv4 } from "uuid";
import {
  API_BASE_URL,
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  PDPefindoItem,
  PDPefindoDetail,
  PDPefindoCreateInput,
  PDPefindoUpdateInput,
  JobStatusResponse,
  UploadJobResponse,
} from "@/lib/schemas/pd-pefindo.schema";

// ---------------------------------------------------------------------------
// List query params
// ---------------------------------------------------------------------------

export interface PDPefindoListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[rating]"?: string;
  "filter[workflow_status]"?: string;
  "filter[sumber]"?: string;
  "filter[periode_berlaku_dari]"?: string;
  include_deleted?: boolean;
}

export type PDPefindoListResponse = ListResponse<PDPefindoItem>;
export type PDPefindoDetailResponse = SingleResponse<PDPefindoDetail>;

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

export type PDPefindoHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API base path
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/pd-pefindo";

// ---------------------------------------------------------------------------
// API object
// ---------------------------------------------------------------------------

export const pdPefindoApi = {
  list(params: PDPefindoListParams = {}): Promise<PDPefindoListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PDPefindoListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<PDPefindoDetailResponse> {
    return apiGet<PDPefindoDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: PDPefindoCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<PDPefindoItem>> {
    // Normalise: empty strings → undefined (backend ignores absent optional fields)
    const body = {
      ...data,
      tanggalPublikasi: data.tanggalPublikasi || undefined,
      periodeBerlakuSampai: data.periodeBerlakuSampai || undefined,
      pdLifetime3Y: data.pdLifetime3Y || undefined,
      pdLifetime5Y: data.pdLifetime5Y || undefined,
      pdLifetime7Y: data.pdLifetime7Y || undefined,
      pdLifetime10Y: data.pdLifetime10Y || undefined,
    };
    return apiPost<SingleResponse<PDPefindoItem>>(BASE, body, idempotencyKey);
  },

  update(
    id: string,
    data: PDPefindoUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<PDPefindoItem>> {
    const body = {
      ...data,
      tanggalPublikasi: data.tanggalPublikasi || undefined,
      periodeBerlakuSampai: data.periodeBerlakuSampai || undefined,
      pdLifetime3Y: data.pdLifetime3Y || undefined,
      pdLifetime5Y: data.pdLifetime5Y || undefined,
      pdLifetime7Y: data.pdLifetime7Y || undefined,
      pdLifetime10Y: data.pdLifetime10Y || undefined,
    };
    // Backend uses PATCH
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
    };
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;
    return fetch(`${API_BASE_URL}${BASE}/${id}`, {
      method: "PATCH",
      headers,
      body: JSON.stringify(body),
    }).then(async (res) => {
      const json = await res.json();
      if (!res.ok) throw json.error;
      return json as SingleResponse<PDPefindoItem>;
    });
  },

  delete(
    id: string,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<{ deleted: boolean; deletedAt: string; entityId: string }>> {
    // Use apiPost with DELETE verb via baseFetch directly
    const headers: Record<string, string> = {};
    if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;
    // We must do DELETE — use raw fetch so we don't carry a body.
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    return fetch(`${API_BASE_URL}${BASE}/${id}`, {
      method: "DELETE",
      headers,
    }).then(async (res) => {
      const body = await res.json();
      if (!res.ok) throw body.error;
      return body as SingleResponse<{ deleted: boolean; deletedAt: string; entityId: string }>;
    });
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

  approve2(
    id: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${id}/approve2`,
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

  getWorkflow(id: string): Promise<SingleResponse<unknown>> {
    return apiGet<SingleResponse<unknown>>(`${BASE}/${id}/workflow`);
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<PDPefindoHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PDPefindoHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: PDPefindoListParams & { format: "csv" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },

  /**
   * Upload XLSX file asynchronously.
   * Returns 202 with { jobId, statusUrl, streamUrl }.
   */
  uploadXlsx(
    file: File,
    periodeBerlakuDari: string,
    tanggalPublikasi: string,
    periodeBerlakuSampai?: string,
    idempotencyKey = uuidv4(),
  ): Promise<{ data: UploadJobResponse; meta: { traceId: string } }> {
    const form = new FormData();
    form.append("file", file);
    form.append("tanggal_publikasi", tanggalPublikasi);
    form.append("periode_berlaku_dari", periodeBerlakuDari);
    if (periodeBerlakuSampai) {
      form.append("periode_berlaku_sampai", periodeBerlakuSampai);
    }

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
    };
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    return fetch(`${API_BASE_URL}${BASE}/upload-xlsx`, {
      method: "POST",
      headers,
      body: form,
    }).then(async (res) => {
      const body = await res.json();
      if (!res.ok) throw body.error;
      return body as { data: UploadJobResponse; meta: { traceId: string } };
    });
  },

  /**
   * Poll job status for an XLSX upload job.
   */
  getJobStatus(jobId: string): Promise<{ data: JobStatusResponse; meta: { traceId: string } }> {
    return apiGet<{ data: JobStatusResponse; meta: { traceId: string } }>(
      `${BASE}/upload-jobs/${jobId}`,
    );
  },
};
