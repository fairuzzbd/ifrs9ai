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
  KursItem,
  KursDetail,
  KursCreateInput,
  KursUpdateInput,
} from "@/lib/schemas/kurs.schema";

// ---------------------------------------------------------------------------
// List params
// ---------------------------------------------------------------------------

export interface KursListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[kode_mata_uang]"?: string;
  "filter[sumber_kurs]"?: string;
  "filter[workflow_status]"?: string;
  "filter[locked_flag]"?: boolean;
  /** Year filter — YYYY */
  "filter[year]"?: string;
  /** Month filter — MM (01-12) */
  "filter[month]"?: string;
  include_deleted?: boolean;
}

export type KursListResponse = ListResponse<KursItem>;
export type KursDetailResponse = SingleResponse<KursDetail>;

// ---------------------------------------------------------------------------
// Workflow types
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Audit history
// ---------------------------------------------------------------------------

export interface KursAuditHistoryEntry {
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

export type KursHistoryResponse = ListResponse<KursAuditHistoryEntry>;

// ---------------------------------------------------------------------------
// JISDOR sync
// ---------------------------------------------------------------------------

export interface JisdorSyncResponse {
  data: {
    jobId: string;
    statusUrl: string;
    message: string;
  };
  meta: { traceId: string };
}

// ---------------------------------------------------------------------------
// API functions
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/kurs";

/**
 * Convert KursCreateInput (with string decimal fields) to the wire format.
 * kursBeli/kursJual are optional; send null when empty to clear.
 */
function toCreateWire(data: KursCreateInput) {
  return {
    kodeMataUang: data.kodeMataUang,
    tanggalBerlaku: data.tanggalBerlaku,
    kursBeli: data.kursBeli?.trim() || null,
    kursJual: data.kursJual?.trim() || null,
    kursTengah: data.kursTengah,
    sumberKurs: data.sumberKurs,
  };
}

function toUpdateWire(data: KursUpdateInput) {
  return {
    kursBeli: data.kursBeli?.trim() || null,
    kursJual: data.kursJual?.trim() || null,
    kursTengah: data.kursTengah,
    sumberKurs: data.sumberKurs,
    rowVersion: data.rowVersion,
  };
}

export const kursApi = {
  list(params: KursListParams = {}): Promise<KursListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<KursListResponse>(`${BASE}${qs}`);
  },

  get(id: string): Promise<KursDetailResponse> {
    return apiGet<KursDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: KursCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<KursItem>> {
    return apiPost<SingleResponse<KursItem>>(BASE, toCreateWire(data), idempotencyKey);
  },

  update(
    id: string,
    data: KursUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<KursItem>> {
    return apiPut<SingleResponse<KursItem>>(
      `${BASE}/${id}`,
      toUpdateWire(data),
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

  getWorkflow(id: string) {
    return apiGet<{ data: import("@/lib/schemas/kurs.schema").KursWorkflowStatus; meta: { traceId: string } }>(
      `${BASE}/${id}/workflow`,
    );
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<KursHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<KursHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(
    params: KursListParams & { format: "csv" | "xlsx" },
  ): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },

  /**
   * POST /master/kurs/jisdor-sync — triggers async JISDOR sync job.
   * Returns 202 with jobId + statusUrl.
   */
  jisdorSync(
    tanggalBerlaku: string,
    idempotencyKey = uuidv4(),
  ): Promise<JisdorSyncResponse> {
    return apiPost<JisdorSyncResponse>(
      `${BASE}/jisdor-sync`,
      { tanggalBerlaku },
      idempotencyKey,
    );
  },
};
