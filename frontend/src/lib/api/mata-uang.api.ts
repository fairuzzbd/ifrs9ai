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
  MataUangItem,
  MataUangDetail,
  MataUangCreateInput,
  MataUangUpdateInput,
  WorkflowStatus,
} from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface MataUangListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[aktif_flag]"?: boolean;
  "filter[workflow_status]"?: string;
  "filter[sumber_kurs_default]"?: string;
  "filter[frekuensi_update]"?: string;
  "filter[kode_mata_uang]"?: string;
  include_deleted?: boolean;
}

export type MataUangListResponse = ListResponse<MataUangItem>;
export type MataUangDetailResponse = SingleResponse<MataUangDetail>;

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

export interface WorkflowStatusResponse {
  data: WorkflowStatus;
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

export type MataUangHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/mata-uang";

export const mataUangApi = {
  list(params: MataUangListParams = {}): Promise<MataUangListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MataUangListResponse>(`${BASE}${qs}`);
  },

  get(kode: string): Promise<MataUangDetailResponse> {
    return apiGet<MataUangDetailResponse>(`${BASE}/${kode}`);
  },

  create(
    data: MataUangCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<MataUangItem>> {
    return apiPost<SingleResponse<MataUangItem>>(
      BASE,
      data,
      idempotencyKey,
    );
  },

  update(
    kode: string,
    data: MataUangUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<MataUangItem>> {
    return apiPut<SingleResponse<MataUangItem>>(
      `${BASE}/${kode}`,
      data,
      idempotencyKey,
    );
  },

  delete(
    kode: string,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<{ deleted: boolean; deletedAt: string; entityId: string }>> {
    return apiDelete(`${BASE}/${kode}`, idempotencyKey);
  },

  submit(
    kode: string,
    body: { comment?: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${kode}/submit`,
      body,
      idempotencyKey,
    );
  },

  review(
    kode: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${kode}/review`,
      body,
      idempotencyKey,
    );
  },

  approve(
    kode: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${kode}/approve`,
      body,
      idempotencyKey,
    );
  },

  reject(
    kode: string,
    body: { comment: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(
      `${BASE}/${kode}/reject`,
      body,
      idempotencyKey,
    );
  },

  getWorkflow(kode: string): Promise<WorkflowStatusResponse> {
    return apiGet<WorkflowStatusResponse>(`${BASE}/${kode}/workflow`);
  },

  getHistory(
    kode: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<MataUangHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MataUangHistoryResponse>(`${BASE}/${kode}/history${qs}`);
  },

  exportUrl(params: MataUangListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },
};
