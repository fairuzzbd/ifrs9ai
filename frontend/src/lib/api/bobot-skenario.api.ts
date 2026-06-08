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
  BobotSkenarioItem,
  BobotSkenarioDetail,
} from "@/lib/schemas/bobot-skenario.schema";
import type { WorkflowStatus } from "@/lib/schemas/mata-uang.schema";

// ---------------------------------------------------------------------------
// Query params
// ---------------------------------------------------------------------------

export interface BobotSkenarioListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[skenario]"?: string;
  "filter[workflow_status]"?: string;
  "filter[periode_berlaku_dari]"?: string;
  include_deleted?: boolean;
}

export type BobotSkenarioListResponse = ListResponse<BobotSkenarioItem>;
export type BobotSkenarioDetailResponse = SingleResponse<BobotSkenarioDetail>;

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

export type BobotSkenarioHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API payload types
// ---------------------------------------------------------------------------

export interface BobotSkenarioCreatePayload {
  skenario: string;
  bobot: string; // decimal string 0-1, e.g. "0.25000000"
  periodeBerlakuDari: string;
  periodeBerlakuSampai?: string | null;
  catatan?: string | null;
}

export interface BobotSkenarioUpdatePayload extends BobotSkenarioCreatePayload {
  rowVersion: number;
}

export interface SeedDefaultPayload {
  periodeBerlakuDari: string; // YYYY-MM-DD
}

export interface SeedDefaultResponse {
  data: {
    created: number; // number of rows created (0 if idempotent skip)
    ids: string[];
    skipped: boolean; // true if 3 rows already existed for the period
  };
  meta: { traceId: string };
}

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/bobot-skenario";

export const bobotSkenarioApi = {
  list(params: BobotSkenarioListParams = {}): Promise<BobotSkenarioListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<BobotSkenarioListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<BobotSkenarioDetailResponse> {
    return apiGet<BobotSkenarioDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: BobotSkenarioCreatePayload,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<BobotSkenarioItem>> {
    return apiPost<SingleResponse<BobotSkenarioItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: BobotSkenarioUpdatePayload,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<BobotSkenarioItem>> {
    return apiPut<SingleResponse<BobotSkenarioItem>>(
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

  /**
   * POST /api/v1/master/bobot-skenario/seed-default
   * Creates 3 default DRAFT rows (GOOD=0.25, NORMAL=0.50, BAD=0.25) for the given period.
   */
  seedDefault(
    payload: SeedDefaultPayload,
    idempotencyKey = uuidv4(),
  ): Promise<SeedDefaultResponse> {
    return apiPost<SeedDefaultResponse>(`${BASE}/seed-default`, payload, idempotencyKey);
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
  ): Promise<BobotSkenarioHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<BobotSkenarioHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: BobotSkenarioListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<
        string,
        string | number | boolean | null | undefined
      >,
    );
    return `${BASE}/export${qs}`;
  },
};
