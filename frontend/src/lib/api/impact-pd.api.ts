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
  ImpactPdItem,
  ImpactPdDetail,
  ImpactPdCreateInput,
  ImpactPdUpdateInput,
  ImpactPdWorkflowInfo,
} from "@/lib/schemas/impact-pd.schema";

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface ImpactPdListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[periode_id]"?: string;
  "filter[workflow_status]"?: string;
  include_deleted?: boolean;
}

export type ImpactPdListResponse = ListResponse<ImpactPdItem>;
export type ImpactPdDetailResponse = SingleResponse<ImpactPdDetail>;

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
  data: ImpactPdWorkflowInfo;
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

export type ImpactPdHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/impact-pd";

export const impactPdApi = {
  list(params: ImpactPdListParams = {}): Promise<ImpactPdListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ImpactPdListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<ImpactPdDetailResponse> {
    return apiGet<ImpactPdDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: ImpactPdCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<ImpactPdItem>> {
    return apiPost<SingleResponse<ImpactPdItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: ImpactPdUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<ImpactPdItem>> {
    return apiPut<SingleResponse<ImpactPdItem>>(`${BASE}/${id}`, data, idempotencyKey);
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
  ): Promise<ImpactPdHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ImpactPdHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: ImpactPdListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },
};
