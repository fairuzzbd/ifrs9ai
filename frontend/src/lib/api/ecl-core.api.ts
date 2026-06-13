/**
 * API client for APP-C ECL Core results (P4-M10).
 *
 * Endpoints from OpenAPI app-c-ecl-core.yaml.
 */

import {
  apiGet,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type { EclResultLine, PortfolioSummary, RollForwardReport } from "@/lib/schemas/ecl-core.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface EclResultListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[stage]"?: string;
  "filter[routing_path]"?: string;
  "filter[portofolio_id]"?: string;
  "filter[has_warning]"?: string;
  tab?: "all" | "stage1" | "stage2" | "stage3" | "errors" | "skipped";
  export?: "csv" | "xlsx";
}

export interface PortfolioSummaryParams {
  priorCalcRunId?: string | null;
}

export interface RollForwardParams {
  priorCalcRunId?: string | null;
}

export interface PriorRunListParams {
  periodeIdBefore?: string;
  statuses?: string;
}

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

export const eclCoreApi = {
  listResults: (
    calcRunId: string,
    params: EclResultListParams = {},
  ): Promise<ListResponse<EclResultLine>> => {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<ListResponse<EclResultLine>>(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}${qs}`,
    );
  },

  getInstrumentDrillDown: (
    calcRunId: string,
    instrumenId: string,
  ): Promise<SingleResponse<EclResultLine>> =>
    apiGet<SingleResponse<EclResultLine>>(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}/instrumen/${encodeURIComponent(instrumenId)}`,
    ),

  getPortfolioSummary: (
    calcRunId: string,
    portofolioId: string,
    params: PortfolioSummaryParams = {},
  ): Promise<SingleResponse<PortfolioSummary>> => {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<SingleResponse<PortfolioSummary>>(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}/portofolio/${encodeURIComponent(portofolioId)}/summary${qs}`,
    );
  },

  getRollForward: (
    calcRunId: string,
    params: RollForwardParams = {},
  ): Promise<SingleResponse<RollForwardReport>> => {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet<SingleResponse<RollForwardReport>>(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}/roll-forward${qs}`,
    );
  },

  listPriorRuns: (
    params: PriorRunListParams = {},
  ): Promise<ListResponse<{ id: string; periodeId: string; periodeLabel?: string; status: string }>> => {
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return apiGet(`/api/v1/ecl/calc-runs${qs}`);
  },

  exportResults: (calcRunId: string, params: EclResultListParams = {}): void => {
    const qs = buildQueryString({ ...params, export: params.export ?? "csv" } as Record<string, string | number | boolean | null | undefined>);
    window.open(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}${qs}`,
      "_blank",
    );
  },

  exportPortfolioSummary: (
    calcRunId: string,
    portofolioId: string,
    format: "csv" | "xlsx" = "xlsx",
  ): void => {
    window.open(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}/portofolio/${encodeURIComponent(portofolioId)}/summary?export=${format}`,
      "_blank",
    );
  },

  exportRollForward: (
    calcRunId: string,
    priorCalcRunId: string | null,
    format: "csv" | "xlsx" = "xlsx",
  ): void => {
    const qs = buildQueryString({
      export: format,
      ...(priorCalcRunId ? { prior_calc_run_id: priorCalcRunId } : {}),
    });
    window.open(
      `/api/v1/ecl/results/${encodeURIComponent(calcRunId)}/roll-forward${qs}`,
      "_blank",
    );
  },
};
