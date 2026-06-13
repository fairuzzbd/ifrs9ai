/**
 * API client for APP-C Staging Engine (P4-M9).
 *
 * Endpoints from OpenAPI app-c-staging.yaml.
 */

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
  StagingCurrent,
  StageHistoryRow,
  StagingOverrideProposal,
  DpdRecord,
} from "@/lib/schemas/staging.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface StagingHistoryParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[trigger_type]"?: string;
  "filter[stage_sesudah]"?: string;
  "filter[stage_sebelum]"?: string;
  "filter[status_approval]"?: string;
  "filter[tanggal_migrasi]"?: string;
  export?: "csv" | "xlsx";
}

export interface OverrideListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[status]"?: string;
  "filter[stage_from]"?: string;
  "filter[stage_to]"?: string;
  "filter[instrumen_id]"?: string;
  "filter[periode_id]"?: string;
  "filter[created_at]"?: string;
  export?: "csv" | "xlsx";
}

export interface DpdListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[periode]"?: string;
  "filter[source]"?: string;
  "filter[dpd_value]"?: string;
  export?: "csv" | "xlsx";
}

export interface StagingEvaluateRequest {
  instrumenIds?: string[];
  triggerType: "RATING" | "DPD" | "ALL";
  periodeId?: string | null;
  reason?: string;
}

export interface JobAcceptedResponse {
  jobId: string;
  type: string;
  statusUrl: string;
  streamUrl: string;
  estimatedDurationSeconds?: number | null;
}

export interface OverrideSubmitRequest {
  instrumenId: string;
  stageTarget: "STAGE_1" | "STAGE_2" | "STAGE_3";
  alasan: string;
  dokumenPendukungId?: string;
  periodeId: string;
}

export interface WorkflowActionRequest {
  action: "APPROVE" | "REJECT";
  comment: string;
  signatureMethod: string;
}

export interface WorkflowRejectRequest {
  comment: string;
  signatureMethod: string;
}

export interface DpdRecordCreateRequest {
  instrumenId: string;
  periode: string;
  dpdValue: number;
  source: "MANUAL";
  catatan?: string;
}

export type StagingCurrentResponse = SingleResponse<StagingCurrent>;
export type StageHistoryListResponse = ListResponse<StageHistoryRow>;
export type OverrideListResponse = ListResponse<StagingOverrideProposal>;
export type OverrideSingleResponse = SingleResponse<StagingOverrideProposal>;
export type DpdRecordListResponse = ListResponse<DpdRecord>;
export type DpdRecordSingleResponse = SingleResponse<DpdRecord>;

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

export const stagingApi = {
  // ---- Current staging ----

  getCurrent(instrumenId: string): Promise<StagingCurrentResponse> {
    return apiGet<StagingCurrentResponse>(
      `/api/v1/ecl/staging/instrumen/${instrumenId}`,
    );
  },

  // ---- Stage history ----

  listHistory(
    instrumenId: string,
    params: StagingHistoryParams = {},
  ): Promise<StageHistoryListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<StageHistoryListResponse>(
      `/api/v1/ecl/staging/instrumen/${instrumenId}/history${qs}`,
    );
  },

  historyExportUrl(instrumenId: string, params: StagingHistoryParams): string {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return `/api/v1/ecl/staging/instrumen/${instrumenId}/history${qs}`;
  },

  // ---- Evaluate ----

  evaluate(
    body: StagingEvaluateRequest,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<JobAcceptedResponse>> {
    return apiPost<SingleResponse<JobAcceptedResponse>>(
      `/api/v1/ecl/staging/evaluate`,
      body,
      idempotencyKey,
    );
  },

  // ---- Override ----

  submitOverride(
    body: OverrideSubmitRequest,
    idempotencyKey = uuidv4(),
  ): Promise<OverrideSingleResponse> {
    return apiPost<OverrideSingleResponse>(
      `/api/v1/ecl/staging/override/submit`,
      body,
      idempotencyKey,
    );
  },

  listOverrides(params: OverrideListParams = {}): Promise<OverrideListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<OverrideListResponse>(`/api/v1/ecl/staging/overrides${qs}`);
  },

  getOverride(id: string): Promise<OverrideSingleResponse> {
    return apiGet<OverrideSingleResponse>(`/api/v1/ecl/staging/overrides/${id}`);
  },

  reviewOverride(
    id: string,
    body: WorkflowActionRequest,
    idempotencyKey = uuidv4(),
  ): Promise<OverrideSingleResponse> {
    return apiPost<OverrideSingleResponse>(
      `/api/v1/ecl/staging/override/${id}/review`,
      body,
      idempotencyKey,
    );
  },

  approveOverride(
    id: string,
    body: WorkflowActionRequest,
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<OverrideSingleResponse> {
    return apiPost<OverrideSingleResponse>(
      `/api/v1/ecl/staging/override/${id}/approve`,
      body,
      idempotencyKey,
    );
  },

  approveOverride2(
    id: string,
    body: WorkflowActionRequest,
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<OverrideSingleResponse> {
    return apiPost<OverrideSingleResponse>(
      `/api/v1/ecl/staging/override/${id}/approve2`,
      body,
      idempotencyKey,
    );
  },

  rejectOverride(
    id: string,
    body: WorkflowRejectRequest,
    idempotencyKey = uuidv4(),
  ): Promise<OverrideSingleResponse> {
    return apiPost<OverrideSingleResponse>(
      `/api/v1/ecl/staging/override/${id}/reject`,
      body,
      idempotencyKey,
    );
  },

  // ---- DPD Records ----

  createDpdRecord(
    body: DpdRecordCreateRequest,
    idempotencyKey = uuidv4(),
  ): Promise<DpdRecordSingleResponse> {
    return apiPost<DpdRecordSingleResponse>(
      `/api/v1/ecl/dpd/record`,
      body,
      idempotencyKey,
    );
  },

  updateDpdRecord(
    id: string,
    body: Partial<DpdRecordCreateRequest>,
    idempotencyKey = uuidv4(),
  ): Promise<DpdRecordSingleResponse> {
    return apiPut<DpdRecordSingleResponse>(
      `/api/v1/ecl/dpd/record/${id}`,
      body,
      idempotencyKey,
    );
  },

  deleteDpdRecord(
    id: string,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<{ deleted: boolean }>> {
    return apiDelete(`/api/v1/ecl/dpd/record/${id}`, idempotencyKey);
  },

  listDpdRecords(params: DpdListParams = {}): Promise<DpdRecordListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<DpdRecordListResponse>(`/api/v1/ecl/dpd/records${qs}`);
  },
};
