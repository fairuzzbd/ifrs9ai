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
  MappingJurnalItem,
  MappingJurnalDetail,
  MappingJurnalFormInput,
  MappingJurnalUpdateInput,
} from "@/lib/schemas/mapping-jurnal.schema";
import type {
  WorkflowActionResponse,
  AuditHistoryEntry,
} from "@/lib/api/mata-uang.api";

// ---------------------------------------------------------------------------
// Query params for list endpoint
// ---------------------------------------------------------------------------

export interface MappingJurnalListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[event_code]"?: string;
  "filter[kategori_event]"?: string;
  "filter[aktif_flag]"?: boolean;
  "filter[workflow_status]"?: string;
  "filter[trigger_source]"?: string;
  include_deleted?: boolean;
}

export type MappingJurnalListResponse = ListResponse<MappingJurnalItem>;
export type MappingJurnalDetailResponse = SingleResponse<MappingJurnalDetail>;
export type MappingJurnalHistoryResponse = ListResponse<AuditHistoryEntry>;

// ---------------------------------------------------------------------------
// CoA autocomplete types (APPROVED accounts only)
// ---------------------------------------------------------------------------

export interface CoaOption {
  id: string;
  kodeAkun: string;
  namaAkun: string;
  tipeAkun: string;
}

export type CoaSearchResponse = ListResponse<CoaOption>;

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/mapping-jurnal";
const COA_BASE = "/api/v1/master/coa";

export const mappingJurnalApi = {
  list(
    params: MappingJurnalListParams = {},
  ): Promise<MappingJurnalListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MappingJurnalListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<MappingJurnalDetailResponse> {
    return apiGet<MappingJurnalDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: MappingJurnalFormInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<MappingJurnalDetail>> {
    return apiPost<SingleResponse<MappingJurnalDetail>>(
      BASE,
      data,
      idempotencyKey,
    );
  },

  update(
    id: string,
    data: MappingJurnalUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<MappingJurnalDetail>> {
    return apiPut<SingleResponse<MappingJurnalDetail>>(
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

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<MappingJurnalHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MappingJurnalHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(
    params: MappingJurnalListParams & { format: "csv" | "xlsx" },
  ): string {
    const qs = buildQueryString(
      params as unknown as Record<
        string,
        string | number | boolean | null | undefined
      >,
    );
    return `${BASE}/export${qs}`;
  },

  // CoA autocomplete — only return APPROVED accounts
  searchCoa(q: string, limit = 20): Promise<CoaSearchResponse> {
    const qs = buildQueryString({
      q,
      limit,
      "filter[workflow_status]": "APPROVED",
    });
    return apiGet<CoaSearchResponse>(`${COA_BASE}${qs}`);
  },
};
