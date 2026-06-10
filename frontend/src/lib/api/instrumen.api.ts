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
  InstrumenItem,
  InstrumenDetail,
  InstrumenCreateInput,
  InstrumenUpdateInput,
  WorkflowStatus,
  CounterpartyOption,
  PortofolioOption,
  MataUangOption,
} from "@/lib/schemas/instrumen.schema";

// ---------------------------------------------------------------------------
// List query params
// ---------------------------------------------------------------------------

export interface InstrumenListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[tipe_instrumen]"?: string;
  "filter[status]"?: string;
  "filter[workflow_status]"?: string;
  "filter[mata_uang]"?: string;
  "filter[portofolio_id]"?: string;
  "filter[counterparty_id]"?: string;
  "filter[klasifikasi_psak71]"?: string;
  "filter[sppi_result]"?: string;
  "filter[bm_category]"?: string;
  include_deleted?: boolean;
}

export type InstrumenListResponse = ListResponse<InstrumenItem>;
export type InstrumenDetailResponse = SingleResponse<InstrumenDetail>;

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

export type InstrumenHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/instrumen";

export const instrumenApi = {
  list(params: InstrumenListParams = {}): Promise<InstrumenListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<InstrumenListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<InstrumenDetailResponse> {
    return apiGet<InstrumenDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: InstrumenCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<InstrumenItem>> {
    return apiPost<SingleResponse<InstrumenItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: InstrumenUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<InstrumenItem>> {
    return apiPut<SingleResponse<InstrumenItem>>(
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
  ): Promise<InstrumenHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<InstrumenHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(
    params: InstrumenListParams & { format: "csv" | "xlsx" },
  ): string {
    const qs = buildQueryString(
      params as unknown as Record<
        string,
        string | number | boolean | null | undefined
      >,
    );
    return `${BASE}/export${qs}`;
  },

  // ---------------------------------------------------------------------------
  // Reference data endpoints (for FK dropdowns)
  // ---------------------------------------------------------------------------

  /** List APPROVED counterparties for autocomplete */
  listCounterparties(
    q?: string,
  ): Promise<ListResponse<CounterpartyOption>> {
    const qs = buildQueryString({
      "filter[workflow_status]": "APPROVED",
      q: q || undefined,
      limit: 50,
    });
    return apiGet<ListResponse<CounterpartyOption>>(
      `/api/v1/master/counterparty${qs}`,
    );
  },

  /** List APPROVED portofolios for autocomplete */
  listPortofolios(q?: string): Promise<ListResponse<PortofolioOption>> {
    const qs = buildQueryString({
      "filter[workflow_status]": "APPROVED",
      q: q || undefined,
      limit: 50,
    });
    return apiGet<ListResponse<PortofolioOption>>(
      `/api/v1/master/portofolio${qs}`,
    );
  },

  /** List APPROVED mata uang for dropdown */
  listMataUang(): Promise<ListResponse<MataUangOption>> {
    const qs = buildQueryString({
      "filter[workflow_status]": "APPROVED",
      "filter[aktif_flag]": true,
      limit: 200,
      sort: "kode_mata_uang:asc",
    });
    return apiGet<ListResponse<MataUangOption>>(
      `/api/v1/master/mata-uang${qs}`,
    );
  },
};
