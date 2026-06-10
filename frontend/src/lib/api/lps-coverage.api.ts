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
  LPSCoverageItem,
  LPSCoverageDetail,
  LPSCoverageCreateInput,
  LPSCoverageUpdateInput,
  WorkflowStatus,
} from "@/lib/schemas/lps-coverage.schema";

// ---------------------------------------------------------------------------
// Query params
// ---------------------------------------------------------------------------

export interface LPSCoverageListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  /** filter[status]: DRAFT | PENDING_REVIEW | PENDING_APPROVAL | APPROVED | RETURNED */
  "filter[workflow_status]"?: string;
  /** filter[active]: true = periode_berlaku_sampai IS NULL */
  "filter[active]"?: boolean;
  /** filter[year]: berlaku_dari year, e.g. "2026" */
  "filter[year]"?: string;
  /** filter[mata_uang]: always IDR but kept for symmetry */
  "filter[mata_uang]"?: string;
  include_deleted?: boolean;
}

export type LPSCoverageListResponse = ListResponse<LPSCoverageItem>;
export type LPSCoverageDetailResponse = SingleResponse<LPSCoverageDetail>;

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

export type LPSCoverageHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/lps-coverage";

export const lpsCoverageApi = {
  list(params: LPSCoverageListParams = {}): Promise<LPSCoverageListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<LPSCoverageListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<LPSCoverageDetailResponse> {
    return apiGet<LPSCoverageDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: LPSCoverageCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<LPSCoverageItem>> {
    return apiPost<SingleResponse<LPSCoverageItem>>(
      BASE,
      data,
      idempotencyKey,
    );
  },

  update(
    id: string,
    data: LPSCoverageUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<LPSCoverageItem>> {
    return apiPut<SingleResponse<LPSCoverageItem>>(
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

  getWorkflow(id: string): Promise<WorkflowStatusResponse> {
    return apiGet<WorkflowStatusResponse>(`${BASE}/${id}/workflow`);
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<LPSCoverageHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<LPSCoverageHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(
    params: LPSCoverageListParams & { format: "csv" | "xlsx" },
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
