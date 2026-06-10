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
  LGDBaselItem,
  LGDBaselDetail,
} from "@/lib/schemas/lgd-basel.schema";
import type { WorkflowStatus } from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface LGDBaselListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[tipe_eksposur]"?: string;
  "filter[sumber]"?: string;
  "filter[workflow_status]"?: string;
  "filter[aktif]"?: boolean | string;
  include_deleted?: boolean;
}

export type LGDBaselListResponse = ListResponse<LGDBaselItem>;
export type LGDBaselDetailResponse = SingleResponse<LGDBaselDetail>;

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

export type LGDBaselHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API payload types
// ---------------------------------------------------------------------------

export interface LGDBaselCreatePayload {
  tipeEksposur: string;
  lgd: string; // decimal string 0-1, e.g. "0.4550"
  karakteristik?: string;
  periodeBerlakuDari: string;
  periodeBerlakuSampai?: string | null;
  sumber: string;
  dokumenPendukungId?: string | null;
}

export interface LGDBaselUpdatePayload extends LGDBaselCreatePayload {
  rowVersion: number;
}

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/lgd-basel";

export const lgdBaselApi = {
  list(params: LGDBaselListParams = {}): Promise<LGDBaselListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<LGDBaselListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<LGDBaselDetailResponse> {
    return apiGet<LGDBaselDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: LGDBaselCreatePayload,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<LGDBaselItem>> {
    return apiPost<SingleResponse<LGDBaselItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: LGDBaselUpdatePayload,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<LGDBaselItem>> {
    return apiPut<SingleResponse<LGDBaselItem>>(
      `${BASE}/${id}`,
      data,
      idempotencyKey,
    );
  },

  softDelete(
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

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<LGDBaselHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<LGDBaselHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: LGDBaselListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<
        string,
        string | number | boolean | null | undefined
      >,
    );
    return `${BASE}/export${qs}`;
  },
};
