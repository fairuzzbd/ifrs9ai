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
  ImpactMevPdItem,
  ImpactMevPdDetail,
  ImpactMevPdCreateInput,
  ImpactMevPdUpdateInput,
  ImpactMevPdWorkflowInfo,
} from "@/lib/schemas/impact-mev-pd.schema";

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface ImpactMevPdListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[periode_id]"?: string;
  "filter[skenario]"?: string;
  "filter[workflow_status]"?: string;
  include_deleted?: boolean;
}

export type ImpactMevPdListResponse = ListResponse<ImpactMevPdItem>;
export type ImpactMevPdDetailResponse = SingleResponse<ImpactMevPdDetail>;

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
  data: ImpactMevPdWorkflowInfo;
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

export type ImpactMevPdHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/impact-mev-pd";

export const impactMevPdApi = {
  list(params: ImpactMevPdListParams = {}): Promise<ImpactMevPdListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ImpactMevPdListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<ImpactMevPdDetailResponse> {
    return apiGet<ImpactMevPdDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: ImpactMevPdCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<ImpactMevPdItem>> {
    return apiPost<SingleResponse<ImpactMevPdItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: ImpactMevPdUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<ImpactMevPdItem>> {
    return apiPut<SingleResponse<ImpactMevPdItem>>(`${BASE}/${id}`, data, idempotencyKey);
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
    return apiPost<WorkflowActionResponse>(`${BASE}/${id}/submit`, body, idempotencyKey);
  },

  review(
    id: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(`${BASE}/${id}/review`, body, idempotencyKey);
  },

  approve(
    id: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(`${BASE}/${id}/approve`, body, idempotencyKey);
  },

  approve2(
    id: string,
    body: { comment?: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(`${BASE}/${id}/approve2`, body, idempotencyKey);
  },

  reject(
    id: string,
    body: { comment: string; signatureMethod: string; rowVersion: number },
    idempotencyKey = uuidv4(),
  ): Promise<WorkflowActionResponse> {
    return apiPost<WorkflowActionResponse>(`${BASE}/${id}/reject`, body, idempotencyKey);
  },

  getWorkflow(id: string): Promise<WorkflowStatusResponse> {
    return apiGet<WorkflowStatusResponse>(`${BASE}/${id}/workflow`);
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<ImpactMevPdHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ImpactMevPdHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: ImpactMevPdListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },
};
