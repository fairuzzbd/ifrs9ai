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
  CounterpartyListItem,
  CounterpartyDetail,
  CounterpartyPII,
  CounterpartyCreateInput,
  CounterpartyUpdateInput,
} from "@/lib/schemas/counterparty.schema";
import type {
  WorkflowActionResponse,
  WorkflowStatusResponse,
  MataUangHistoryResponse,
} from "@/lib/api/mata-uang.api";

// Re-export workflow types for convenience
export type { WorkflowActionResponse, WorkflowStatusResponse };

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface CounterpartyListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[tipe]"?: string;
  "filter[tipe_eksposur_basel]"?: string;
  "filter[status]"?: string;
  "filter[eligible_lps_flag]"?: boolean;
  "filter[workflow_status]"?: string;
  "filter[rating_pefindo_current]"?: string;
  include_deleted?: boolean;
}

export type CounterpartyListResponse = ListResponse<CounterpartyListItem>;
export type CounterpartyDetailResponse = SingleResponse<CounterpartyDetail>;
export type CounterpartyPIIResponse = SingleResponse<CounterpartyPII>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/counterparty";

export const counterpartyApi = {
  list(params: CounterpartyListParams = {}): Promise<CounterpartyListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<CounterpartyListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<CounterpartyDetailResponse> {
    return apiGet<CounterpartyDetailResponse>(`${BASE}/${id}`);
  },

  /**
   * Fetch decrypted PII. Only available to users with `counterparty.view_pii`.
   * Every call fires an audit event on the backend.
   */
  getPII(id: string): Promise<CounterpartyPIIResponse> {
    return apiGet<CounterpartyPIIResponse>(`${BASE}/${id}/pii`);
  },

  create(
    data: CounterpartyCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<CounterpartyListItem>> {
    return apiPost<SingleResponse<CounterpartyListItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: CounterpartyUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<CounterpartyListItem>> {
    return apiPut<SingleResponse<CounterpartyListItem>>(
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

  getWorkflow(id: string): Promise<WorkflowStatusResponse> {
    return apiGet<WorkflowStatusResponse>(`${BASE}/${id}/workflow`);
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<MataUangHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MataUangHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: CounterpartyListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },
};
