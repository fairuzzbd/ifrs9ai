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
  PortofolioItem,
  PortofolioDetail,
  PortofolioCreateInput,
  PortofolioUpdateInput,
  WorkflowStatus,
} from "@/lib/schemas/portofolio.schema";

// ---------------------------------------------------------------------------
// Query params
// ---------------------------------------------------------------------------

export interface PortofolioListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[bm_category_default]"?: string;
  "filter[workflow_status]"?: string;
  "filter[aktif_flag]"?: boolean;
  include_deleted?: boolean;
}

export type PortofolioListResponse = ListResponse<PortofolioItem>;
export type PortofolioDetailResponse = SingleResponse<PortofolioDetail>;

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

export type PortofolioHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/portofolio";

export const portofolioApi = {
  list(params: PortofolioListParams = {}): Promise<PortofolioListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PortofolioListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<PortofolioDetailResponse> {
    return apiGet<PortofolioDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: PortofolioCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<PortofolioItem>> {
    return apiPost<SingleResponse<PortofolioItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: PortofolioUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<PortofolioItem>> {
    return apiPut<SingleResponse<PortofolioItem>>(
      `${BASE}/${id}`,
      data,
      idempotencyKey,
    );
  },

  delete(
    id: string,
    idempotencyKey = uuidv4(),
  ): Promise<
    SingleResponse<{ deleted: boolean; deletedAt: string; entityId: string }>
  > {
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

  getWorkflow(id: string): Promise<WorkflowStatusResponse> {
    return apiGet<WorkflowStatusResponse>(`${BASE}/${id}/workflow`);
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<PortofolioHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PortofolioHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(
    params: PortofolioListParams & { format: "csv" | "xlsx" },
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
