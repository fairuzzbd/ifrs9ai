/**
 * API client for APP-C EIR Newton-Raphson + Amendment (P4-M9).
 *
 * Endpoints from OpenAPI app-c-eir.yaml.
 */

import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  EIRComputeResponse,
  EIRScheduleRow,
  EIRAmendmentProposal,
  DriftReport,
} from "@/lib/schemas/eir.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface EIRComputeRequest {
  instrumenId: string;
  cashflowProjection?: Array<{ date: string; amountIdr: number }>;
  initialPrincipalIdr?: number;
  couponRate?: number;
  persistResult?: boolean;
  forceRecompute?: boolean;
  pociMode?: boolean;
}

export interface EIRScheduleListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  "filter[status_posting]"?: string;
  "filter[periode_seq]"?: string;
  "filter[tanggal_posting]"?: string;
  "filter[recomputed_from_seq]"?: string;
  export?: "csv" | "xlsx";
}

export interface AmendmentListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[workflow_status]"?: string;
  "filter[instrumen_id]"?: string;
  "filter[tanggal_re_estimation]"?: string;
  "filter[trigger_source]"?: string;
  export?: "csv" | "xlsx";
}

export interface DriftReportListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  "filter[trigger_source]"?: string;
  "filter[scan_started_at]"?: string;
}

export interface AmendmentProposeRequest {
  instrumenId: string;
  amendmentDate: string;
  revisedCashflows: Array<{ date: string; amountIdr: number }>;
  alasan: string;
  dokumenPendukungId: string;
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

export interface JobAcceptedResponse {
  jobId: string;
  type: string;
  statusUrl: string;
  streamUrl: string;
  estimatedDurationSeconds?: number | null;
}

export type EIRComputeApiResponse = SingleResponse<EIRComputeResponse>;
export type EIRScheduleListResponse = ListResponse<EIRScheduleRow> & {
  warning?: string | null;
};
export type AmendmentListResponse = ListResponse<EIRAmendmentProposal>;
export type AmendmentSingleResponse = SingleResponse<EIRAmendmentProposal>;
export type DriftReportListResponse = ListResponse<DriftReport>;
export type DriftReportSingleResponse = SingleResponse<DriftReport>;

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

export const eirApi = {
  // ---- Compute ----

  compute(
    body: EIRComputeRequest,
    idempotencyKey = uuidv4(),
  ): Promise<EIRComputeApiResponse> {
    return apiPost<EIRComputeApiResponse>(
      `/api/v1/ecl/eir/compute`,
      body,
      idempotencyKey,
    );
  },

  // ---- Schedule ----

  listSchedule(
    instrumenId: string,
    params: EIRScheduleListParams = {},
  ): Promise<EIRScheduleListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<EIRScheduleListResponse>(
      `/api/v1/ecl/eir/schedule/${instrumenId}/full${qs}`,
    );
  },

  getScheduleHistory(
    instrumenId: string,
    params: EIRScheduleListParams = {},
  ): Promise<EIRScheduleListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<EIRScheduleListResponse>(
      `/api/v1/ecl/eir/schedule/${instrumenId}/history${qs}`,
    );
  },

  scheduleExportUrl(instrumenId: string, params: EIRScheduleListParams): string {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return `/api/v1/ecl/eir/schedule/${instrumenId}/full${qs}`;
  },

  // ---- Amendments ----

  proposeAmendment(
    body: AmendmentProposeRequest,
    idempotencyKey = uuidv4(),
  ): Promise<AmendmentSingleResponse> {
    return apiPost<AmendmentSingleResponse>(
      `/api/v1/ecl/eir/amendments`,
      body,
      idempotencyKey,
    );
  },

  listAmendments(params: AmendmentListParams = {}): Promise<AmendmentListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<AmendmentListResponse>(`/api/v1/ecl/eir/amendments${qs}`);
  },

  getAmendment(id: string): Promise<AmendmentSingleResponse> {
    return apiGet<AmendmentSingleResponse>(`/api/v1/ecl/eir/amendments/${id}`);
  },

  reviewAmendment(
    id: string,
    body: WorkflowActionRequest,
    idempotencyKey = uuidv4(),
  ): Promise<AmendmentSingleResponse> {
    return apiPost<AmendmentSingleResponse>(
      `/api/v1/ecl/eir/amendments/${id}/review`,
      body,
      idempotencyKey,
    );
  },

  approveAmendment(
    id: string,
    body: WorkflowActionRequest,
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<AmendmentSingleResponse> {
    return apiPost<AmendmentSingleResponse>(
      `/api/v1/ecl/eir/amendments/${id}/approve`,
      body,
      idempotencyKey,
    );
  },

  rejectAmendment(
    id: string,
    body: WorkflowRejectRequest,
    idempotencyKey = uuidv4(),
  ): Promise<AmendmentSingleResponse> {
    return apiPost<AmendmentSingleResponse>(
      `/api/v1/ecl/eir/amendments/${id}/reject`,
      body,
      idempotencyKey,
    );
  },

  cancelAmendment(
    id: string,
    body: WorkflowRejectRequest,
    idempotencyKey = uuidv4(),
  ): Promise<AmendmentSingleResponse> {
    return apiPost<AmendmentSingleResponse>(
      `/api/v1/ecl/eir/amendments/${id}/cancel`,
      body,
      idempotencyKey,
    );
  },

  // ---- Bulk recompute / Drift ----

  triggerBulkRecompute(
    body: { scope?: string; periodeId?: string | null; reason?: string },
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<JobAcceptedResponse>> {
    return apiPost<SingleResponse<JobAcceptedResponse>>(
      `/api/v1/ecl/eir/bulk-recompute`,
      body,
      idempotencyKey,
    );
  },

  listDriftReports(
    params: DriftReportListParams = {},
  ): Promise<DriftReportListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<DriftReportListResponse>(`/api/v1/ecl/eir/drift-reports${qs}`);
  },

  getDriftReport(id: string): Promise<DriftReportSingleResponse> {
    return apiGet<DriftReportSingleResponse>(`/api/v1/ecl/eir/drift-reports/${id}`);
  },
};
