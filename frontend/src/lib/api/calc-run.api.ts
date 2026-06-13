/**
 * API client for APP-C ECL Calc Run (P4-M10).
 *
 * Endpoints from OpenAPI app-c-calc-run.yaml.
 */

import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type { CalcRun } from "@/lib/schemas/calc-run.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CalcRunListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[periode_id]"?: string;
  "filter[status]"?: string;
  "filter[created_by]"?: string;
  export?: "csv" | "xlsx";
}

export interface CreateCalcRunRequest {
  periodeId: string;
  evaluationDate: string;
}

export interface StartCalcRunResponse {
  jobId: string;
  statusUrl: string;
  streamUrl: string;
}

export interface CancelCalcRunRequest {
  cancelReason: string;
}

export interface SealCalcRunRequest {
  action: "REQUEST" | "APPROVE" | "REJECT";
  comment?: string;
  rejectReason?: string;
  stepUpToken?: string;
}

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

export const calcRunApi = {
  list: (params: CalcRunListParams = {}): Promise<ListResponse<CalcRun>> => {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<ListResponse<CalcRun>>(`/api/v1/ecl/calc-runs${qs}`);
  },

  get: (id: string): Promise<SingleResponse<CalcRun>> =>
    apiGet<SingleResponse<CalcRun>>(`/api/v1/ecl/calc-runs/${encodeURIComponent(id)}`),

  create: (
    body: CreateCalcRunRequest,
    idempotencyKey?: string,
  ): Promise<SingleResponse<CalcRun>> =>
    apiPost<SingleResponse<CalcRun>>(
      "/api/v1/ecl/calc-runs",
      body,
      idempotencyKey ?? uuidv4(),
    ),

  start: (
    id: string,
    idempotencyKey?: string,
  ): Promise<SingleResponse<StartCalcRunResponse>> =>
    apiPost<SingleResponse<StartCalcRunResponse>>(
      `/api/v1/ecl/calc-runs/${encodeURIComponent(id)}/start`,
      {},
      idempotencyKey ?? uuidv4(),
    ),

  cancel: (
    id: string,
    body: CancelCalcRunRequest,
    idempotencyKey?: string,
  ): Promise<SingleResponse<CalcRun>> =>
    apiPost<SingleResponse<CalcRun>>(
      `/api/v1/ecl/calc-runs/${encodeURIComponent(id)}/cancel`,
      body,
      idempotencyKey ?? uuidv4(),
    ),

  seal: (
    id: string,
    body: SealCalcRunRequest,
    idempotencyKey?: string,
    stepUpToken?: string,
  ): Promise<SingleResponse<CalcRun>> => {
    const headers: Record<string, string> = {};
    if (stepUpToken) headers["X-Step-Up-Token"] = stepUpToken;
    const key = idempotencyKey ?? uuidv4();
    return apiPost<SingleResponse<CalcRun>>(
      `/api/v1/ecl/calc-runs/${encodeURIComponent(id)}/seal`,
      body,
      key,
    );
  },

  exportList: (params: CalcRunListParams = {}): void => {
    const qs = buildQueryString({ ...params, export: params.export ?? "csv" } as Record<string, string | number | boolean | null | undefined>);
    window.open(`/api/v1/ecl/calc-runs${qs}`, "_blank");
  },
};
