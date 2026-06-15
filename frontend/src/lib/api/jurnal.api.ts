import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  MappingHeaderSummary,
  MappingHeaderDetail,
  MappingHeaderCreateInput,
  MappingHeaderEditInput,
  WorkflowTransitionResponse,
  ResolverRequest,
  ResolverResponse,
  ManualPostInput,
  JurnalHeaderSummary,
  JurnalHeaderDetail,
  DlqEntrySummary,
  DlqEntryDetail,
} from "@/lib/schemas/jurnal.schema";

// ---------------------------------------------------------------------------
// Shared response types
// ---------------------------------------------------------------------------

export type MappingListResponse = ListResponse<MappingHeaderSummary>;
export type MappingDetailResponse = SingleResponse<MappingHeaderDetail>;
export type JurnalListResponse = ListResponse<JurnalHeaderSummary>;
export type JurnalDetailResponse = SingleResponse<JurnalHeaderDetail>;
export type DlqListResponse = ListResponse<DlqEntrySummary>;
export type DlqDetailResponse = SingleResponse<DlqEntryDetail>;

// ---------------------------------------------------------------------------
// List query params
// ---------------------------------------------------------------------------

export interface MappingListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[event_code]"?: string;
  "filter[kategori_event]"?: string;
  "filter[workflow_status]"?: string;
  "filter[aktif_flag]"?: boolean;
}

export interface JurnalListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[periode_id]"?: string;
  "filter[event_code]"?: string;
  "filter[instrumen_id]"?: string;
  "filter[status_internal]"?: string;
  "filter[tanggal_posting]"?: string;
  "filter[total_debit]"?: string;
}

export interface DlqListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[status]"?: string;
  "filter[event_code]"?: string;
  "filter[source_event_type]"?: string;
  "filter[error_code]"?: string;
}

// ---------------------------------------------------------------------------
// API base paths
// ---------------------------------------------------------------------------

const MAPPING_BASE = "/api/v1/jurnal/mapping-headers";
const JURNAL_BASE = "/api/v1/jurnal";
const DLQ_BASE = "/api/v1/jurnal/dlq";

// ---------------------------------------------------------------------------
// Mapping Jurnal Header API
// ---------------------------------------------------------------------------

export const mappingApi = {
  list(params: MappingListParams = {}): Promise<MappingListResponse> {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<MappingListResponse>(`${MAPPING_BASE}${qs}`);
  },

  get(id: string): Promise<MappingDetailResponse> {
    return apiGet<MappingDetailResponse>(`${MAPPING_BASE}/${id}`);
  },

  create(data: MappingHeaderCreateInput, idempotencyKey = uuidv4()): Promise<MappingDetailResponse> {
    return apiPost<MappingDetailResponse>(MAPPING_BASE, data, idempotencyKey);
  },

  edit(id: string, data: MappingHeaderEditInput, idempotencyKey = uuidv4()): Promise<MappingDetailResponse> {
    return apiPost<MappingDetailResponse>(`${MAPPING_BASE}/${id}`, { _method: "PATCH", ...data }, idempotencyKey);
  },

  submit(id: string, body: { comment?: string; signatureMethod?: string }, idempotencyKey = uuidv4()): Promise<WorkflowTransitionResponse> {
    return apiPost<WorkflowTransitionResponse>(`${MAPPING_BASE}/${id}/submit`, body, idempotencyKey);
  },

  review(id: string, body: { comment?: string; signatureMethod: string }, idempotencyKey = uuidv4()): Promise<WorkflowTransitionResponse> {
    return apiPost<WorkflowTransitionResponse>(`${MAPPING_BASE}/${id}/review`, body, idempotencyKey);
  },

  approve(id: string, body: { comment?: string; signatureMethod: string }, stepUpToken?: string, idempotencyKey = uuidv4()): Promise<WorkflowTransitionResponse> {
    const headers: Record<string, string> = {};
    if (stepUpToken) headers["X-Step-Up-Token"] = stepUpToken;
    return apiPost<WorkflowTransitionResponse>(`${MAPPING_BASE}/${id}/approve`, body, idempotencyKey);
  },

  approve2(id: string, body: { comment?: string; signatureMethod: string }, stepUpToken: string, idempotencyKey = uuidv4()): Promise<WorkflowTransitionResponse> {
    return apiPost<WorkflowTransitionResponse>(`${MAPPING_BASE}/${id}/approve-2`, { ...body, stepUpToken }, idempotencyKey);
  },

  reject(id: string, body: { rejectReason: string; signatureMethod: string }, idempotencyKey = uuidv4()): Promise<WorkflowTransitionResponse> {
    return apiPost<WorkflowTransitionResponse>(`${MAPPING_BASE}/${id}/reject`, body, idempotencyKey);
  },

  withdraw(id: string, body?: { reason?: string }, idempotencyKey = uuidv4()): Promise<WorkflowTransitionResponse> {
    return apiPost<WorkflowTransitionResponse>(`${MAPPING_BASE}/${id}/withdraw`, body ?? {}, idempotencyKey);
  },

  deactivate(id: string, body?: { reason?: string }, idempotencyKey = uuidv4()): Promise<SingleResponse<{ id: string; aktifFlag: boolean; workflowStatus: string }>> {
    return apiPost<SingleResponse<{ id: string; aktifFlag: boolean; workflowStatus: string }>>(`${MAPPING_BASE}/${id}/deactivate`, body ?? {}, idempotencyKey);
  },

  exportUrl(params: MappingListParams & { format?: "csv" | "xlsx" }): string {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return `${MAPPING_BASE}/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// Resolver API
// ---------------------------------------------------------------------------

export const resolverApi = {
  resolve(data: ResolverRequest): Promise<SingleResponse<ResolverResponse>> {
    return apiPost<SingleResponse<ResolverResponse>>(`${JURNAL_BASE}/resolve`, data);
  },
};

// ---------------------------------------------------------------------------
// Manual Posting API
// ---------------------------------------------------------------------------

export const manualPostApi = {
  create(data: ManualPostInput, idempotencyKey = uuidv4()): Promise<JurnalDetailResponse> {
    return apiPost<JurnalDetailResponse>(`${JURNAL_BASE}/post`, data, idempotencyKey);
  },

  submit(jurnalHeaderId: string, body: { comment?: string }, idempotencyKey = uuidv4()): Promise<SingleResponse<{ id: string; statusInternal: string }>> {
    return apiPost<SingleResponse<{ id: string; statusInternal: string }>>(`${JURNAL_BASE}/${jurnalHeaderId}/submit`, body, idempotencyKey);
  },

  approve(jurnalHeaderId: string, body: { comment?: string; signatureMethod: string }, idempotencyKey = uuidv4()): Promise<JurnalDetailResponse> {
    return apiPost<JurnalDetailResponse>(`${JURNAL_BASE}/${jurnalHeaderId}/approve`, body, idempotencyKey);
  },

  reject(jurnalHeaderId: string, body: { rejectReason: string; signatureMethod: string }, idempotencyKey = uuidv4()): Promise<SingleResponse<{ id: string; statusInternal: string }>> {
    return apiPost<SingleResponse<{ id: string; statusInternal: string }>>(`${JURNAL_BASE}/${jurnalHeaderId}/reject`, body, idempotencyKey);
  },
};

// ---------------------------------------------------------------------------
// Journal Entries API
// ---------------------------------------------------------------------------

export const jurnalQueryApi = {
  list(params: JurnalListParams = {}): Promise<JurnalListResponse> {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<JurnalListResponse>(`${JURNAL_BASE}${qs}`);
  },

  get(id: string): Promise<JurnalDetailResponse> {
    return apiGet<JurnalDetailResponse>(`${JURNAL_BASE}/${id}`);
  },

  exportUrl(params: JurnalListParams & { format?: "csv" | "xlsx" }): string {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return `${JURNAL_BASE}/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// DLQ API
// ---------------------------------------------------------------------------

export const dlqApi = {
  list(params: DlqListParams = {}): Promise<DlqListResponse> {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<DlqListResponse>(`${DLQ_BASE}${qs}`);
  },

  get(id: string): Promise<DlqDetailResponse> {
    return apiGet<DlqDetailResponse>(`${DLQ_BASE}/${id}`);
  },

  replay(id: string, body?: { reason?: string }, idempotencyKey = uuidv4()): Promise<SingleResponse<{ dlqId: string; jobId: string; statusUrl: string }>> {
    return apiPost<SingleResponse<{ dlqId: string; jobId: string; statusUrl: string }>>(`${DLQ_BASE}/${id}/replay`, body ?? {}, idempotencyKey);
  },

  discard(id: string, body: { discardReason: string }, idempotencyKey = uuidv4()): Promise<SingleResponse<{ id: string; status: string }>> {
    return apiPost<SingleResponse<{ id: string; status: string }>>(`${DLQ_BASE}/${id}/discard`, body, idempotencyKey);
  },
};
