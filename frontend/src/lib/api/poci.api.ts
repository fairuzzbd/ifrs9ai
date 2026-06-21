/**
 * API client — P5-M10 POCI Delta ECL
 * Mirrors api/openapi/app-c-poci-delta.yaml
 *
 * All mutating calls auto-generate Idempotency-Key (DEC-021).
 * 4 namespaces: baseline, delta-log, history, dashboard, compute-trigger.
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
  PociBaselineListItem,
  PociBaselineDetail,
  PociDeltaLogItem,
  PociDeltaHistoryItem,
  PociDeltaSummary,
} from "@/lib/schemas/poci.schema";

// ---------------------------------------------------------------------------
// Base paths
// ---------------------------------------------------------------------------

const BASE_POCI = "/api/v1/poci";

// ---------------------------------------------------------------------------
// Param types
// ---------------------------------------------------------------------------

export interface PociBaselineListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[instrumen_id]"?: string;
  "filter[tanggal_baseline]"?: string;
}

export interface PociDeltaLogParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[calc_run_id]"?: string;
  "filter[instrumen_id]"?: string;
  "filter[periode]"?: string;
  "filter[direction]"?: string;
  "filter[status]"?: string;
}

export interface PociDeltaHistoryParams {
  instrumen_id: string;
  cursor?: string | null;
  limit?: number;
  sort?: string;
  "filter[direction]"?: string;
}

export interface PociDeltaSummaryParams {
  portofolio_id?: string;
  year: number;
  month: number;
}

// ---------------------------------------------------------------------------
// Response aliases
// ---------------------------------------------------------------------------

export type PociBaselineListApiResponse = ListResponse<PociBaselineListItem>;
export type PociBaselineDetailApiResponse = SingleResponse<PociBaselineDetail>;
export type PociDeltaLogApiResponse = ListResponse<PociDeltaLogItem>;
export type PociDeltaHistoryApiResponse = ListResponse<PociDeltaHistoryItem>;
export type PociDeltaSummaryApiResponse = SingleResponse<PociDeltaSummary>;

export interface ComputeDeltaBatchResponse {
  jobId: string;
  type: string;
  statusUrl: string;
  streamUrl: string;
}

// ---------------------------------------------------------------------------
// pociBaselineApi — GET /poci/baseline (S1-AC4)
// ---------------------------------------------------------------------------

export const pociBaselineApi = {
  list(params: PociBaselineListParams = {}): Promise<PociBaselineListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PociBaselineListApiResponse>(`${BASE_POCI}/baseline${qs}`);
  },

  get(instrumenId: string): Promise<PociBaselineDetailApiResponse> {
    return apiGet<PociBaselineDetailApiResponse>(`${BASE_POCI}/baseline/${instrumenId}`);
  },

  exportUrl(params: PociBaselineListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE_POCI}/baseline/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// pociDeltaLogApi — GET /poci/delta-log (S2, S3)
// ---------------------------------------------------------------------------

export const pociDeltaLogApi = {
  list(params: PociDeltaLogParams = {}): Promise<PociDeltaLogApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PociDeltaLogApiResponse>(`${BASE_POCI}/delta-log${qs}`);
  },

  exportUrl(params: PociDeltaLogParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE_POCI}/delta-log/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// pociDeltaHistoryApi — GET /poci/delta-history (S5-AC1)
// ---------------------------------------------------------------------------

export const pociDeltaHistoryApi = {
  list(params: PociDeltaHistoryParams): Promise<PociDeltaHistoryApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PociDeltaHistoryApiResponse>(`${BASE_POCI}/delta-history${qs}`);
  },

  exportUrl(params: PociDeltaHistoryParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE_POCI}/delta-history/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// pociDashboardApi — GET /poci/delta-history/summary (S5-AC2)
// ---------------------------------------------------------------------------

export const pociDashboardApi = {
  summary(params: PociDeltaSummaryParams): Promise<PociDeltaSummaryApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PociDeltaSummaryApiResponse>(`${BASE_POCI}/delta-history/summary${qs}`);
  },
};

// ---------------------------------------------------------------------------
// pociComputeApi — POST /poci/compute-delta-batch (ROLE-IT-ADMIN / ROLE-RISK)
// Permission: poci.delta.compute. Rate limit: 10 req/min (DEC-027).
// Returns 202 + jobId. Monitor via GET /api/v1/jobs/{jobId}.
// ---------------------------------------------------------------------------

export const pociComputeApi = {
  triggerBatch(
    calcRunId: string,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<ComputeDeltaBatchResponse>> {
    return apiPost<SingleResponse<ComputeDeltaBatchResponse>>(
      `${BASE_POCI}/compute-delta-batch`,
      { calcRunId },
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const pociQueryKeys = {
  all: ["poci"] as const,

  baselines: () => [...pociQueryKeys.all, "baseline"] as const,
  baselineList: (params: PociBaselineListParams) =>
    [...pociQueryKeys.baselines(), "list", params] as const,
  baselineDetail: (instrumenId: string) =>
    [...pociQueryKeys.baselines(), "detail", instrumenId] as const,

  deltaLogs: () => [...pociQueryKeys.all, "delta-log"] as const,
  deltaLogList: (params: PociDeltaLogParams) =>
    [...pociQueryKeys.deltaLogs(), params] as const,

  histories: () => [...pociQueryKeys.all, "delta-history"] as const,
  history: (params: PociDeltaHistoryParams) =>
    [...pociQueryKeys.histories(), params] as const,

  dashboards: () => [...pociQueryKeys.all, "dashboard"] as const,
  dashboard: (params: PociDeltaSummaryParams) =>
    [...pociQueryKeys.dashboards(), params] as const,
} as const;
