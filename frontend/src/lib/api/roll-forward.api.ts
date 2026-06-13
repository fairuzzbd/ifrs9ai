/**
 * API client for APP-C Roll-Forward CKPN (P4-M11).
 *
 * Endpoints from OpenAPI app-c-roll-forward.yaml.
 * Money: string-based — no parseFloat.
 */

import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type SingleResponse,
  type ListResponse,
} from "@/lib/api";
import type {
  RollForwardM11Report,
  RollForwardJobAccepted,
  PortfolioRollForward,
  CKPNTrendResponse,
  RollForwardInstrumentLine,
} from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ComputeRollForwardBody {
  currentCalcRunId: string;
  priorCalcRunId?: string | null;
  options?: {
    detectionMethod?: "BASIC_STATUS_DIFF";
  };
}

/** 200 = sync full report; 202 = async job dispatched */
export type ComputeRollForwardResponse =
  | { status: 200; data: SingleResponse<RollForwardM11Report> }
  | { status: 202; data: SingleResponse<RollForwardJobAccepted> };

export interface GetRollForwardParams {
  currentCalcRunId: string;
  priorCalcRunId?: string | null;
}

export interface GetPortfolioRollForwardParams {
  currentCalcRunId: string;
  priorCalcRunId?: string | null;
}

export interface ListInstrumentsParams {
  currentCalcRunId: string;
  priorCalcRunId?: string | null;
  bucket?: string;
  "filter[override_flag]"?: boolean;
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  export?: "csv" | "xlsx";
}

export interface GetCKPNTrendParams {
  periods?: number;
  "filter[portofolio_id]"?: string;
  sort?: string;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Raw fetch for mixed 200/202 response
// ---------------------------------------------------------------------------

async function fetchComputeRollForward(
  body: ComputeRollForwardBody,
  idempotencyKey: string,
): Promise<{ httpStatus: number; payload: unknown }> {
  const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081";
  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

  const res = await fetch(`${API_BASE_URL}/api/v1/ecl/roll-forward/compute`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    const errBody = await res.json().catch(() => ({
      error: { code: "INTERNAL", message: `HTTP ${res.status}`, details: [], traceId: "" },
    })) as { error: { code: string; message: string; details: unknown[]; traceId: string } };
    const err = new Error(errBody.error?.message ?? `HTTP ${res.status}`);
    (err as Error & { code: string; status: number; details: unknown[]; traceId: string }).code =
      errBody.error?.code ?? "INTERNAL";
    (err as Error & { status: number }).status = res.status;
    (err as Error & { details: unknown[] }).details = errBody.error?.details ?? [];
    (err as Error & { traceId: string }).traceId = errBody.error?.traceId ?? "";
    throw err;
  }

  const payload = await res.json();
  return { httpStatus: res.status, payload };
}

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------

export const rollForwardApi = {
  /**
   * POST /ecl/roll-forward/compute
   * 200 = synchronous result, 202 = async job
   */
  compute: async (
    body: ComputeRollForwardBody,
    idempotencyKey?: string,
  ): Promise<ComputeRollForwardResponse> => {
    const key = idempotencyKey ?? uuidv4();
    const { httpStatus, payload } = await fetchComputeRollForward(body, key);

    if (httpStatus === 202) {
      return {
        status: 202,
        data: payload as SingleResponse<RollForwardJobAccepted>,
      };
    }
    return {
      status: 200,
      data: payload as SingleResponse<RollForwardM11Report>,
    };
  },

  /**
   * GET /ecl/roll-forward
   * Returns sync (200) or async-dispatched (202) — same union type.
   */
  get: (
    params: GetRollForwardParams,
  ): Promise<SingleResponse<RollForwardM11Report>> => {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<SingleResponse<RollForwardM11Report>>(
      `/api/v1/ecl/roll-forward${qs}`,
    );
  },

  /**
   * GET /ecl/roll-forward/{reportId}/export
   * Opens download in new tab.
   * force_mismatch=true for MISMATCH overrides.
   */
  export: (
    reportId: string,
    format: "xlsx" | "csv" = "xlsx",
    forceMismatch = false,
  ): void => {
    const qs = buildQueryString({
      format,
      ...(forceMismatch ? { force_mismatch: true } : {}),
    } as Record<string, string | number | boolean | null | undefined>);
    const base = process.env.NEXT_PUBLIC_API_URL ?? "";
    window.open(
      `${base}/api/v1/ecl/roll-forward/${encodeURIComponent(reportId)}/export${qs}`,
      "_blank",
    );
  },

  /**
   * GET /ecl/roll-forward/portfolios/{portofolioId}
   */
  getPortfolio: (
    portofolioId: string,
    params: GetPortfolioRollForwardParams,
  ): Promise<SingleResponse<PortfolioRollForward>> => {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<SingleResponse<PortfolioRollForward>>(
      `/api/v1/ecl/roll-forward/portfolios/${encodeURIComponent(portofolioId)}${qs}`,
    );
  },

  /**
   * GET /ecl/roll-forward/portfolios/{portofolioId}/instruments (DataTable, UX §1)
   */
  listPortfolioInstruments: (
    portofolioId: string,
    params: ListInstrumentsParams,
  ): Promise<ListResponse<RollForwardInstrumentLine>> => {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ListResponse<RollForwardInstrumentLine>>(
      `/api/v1/ecl/roll-forward/portfolios/${encodeURIComponent(portofolioId)}/instruments${qs}`,
    );
  },

  /**
   * Export instrument drill-down table
   */
  exportPortfolioInstruments: (
    portofolioId: string,
    params: ListInstrumentsParams & { format: "csv" | "xlsx" },
  ): void => {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    const base = process.env.NEXT_PUBLIC_API_URL ?? "";
    window.open(
      `${base}/api/v1/ecl/roll-forward/portfolios/${encodeURIComponent(portofolioId)}/instruments${qs}`,
      "_blank",
    );
  },

  /**
   * GET /dashboard/ckpn-trend
   */
  getCKPNTrend: (
    params: GetCKPNTrendParams = {},
  ): Promise<SingleResponse<CKPNTrendResponse>> => {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<SingleResponse<CKPNTrendResponse>>(
      `/api/v1/dashboard/ckpn-trend${qs}`,
    );
  },

  /**
   * POST compute via apiPost helper (thin wrapper, no mixed-status handling)
   * Use rollForwardApi.compute() for proper 200/202 handling.
   */
  computeSimple: (
    body: ComputeRollForwardBody,
    idempotencyKey?: string,
  ): Promise<SingleResponse<RollForwardM11Report>> =>
    apiPost<SingleResponse<RollForwardM11Report>>(
      "/api/v1/ecl/roll-forward/compute",
      body,
      idempotencyKey ?? uuidv4(),
    ),
};
