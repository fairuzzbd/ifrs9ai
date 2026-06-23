/**
 * API client — P5-M12 Mapping Jurnal (6-eyes workflow, version chain, RPT-19/20/21)
 * Mirrors api/openapi/app-d-mapping-jurnal.yaml
 *
 * 5 namespaces:
 *   mappingP12Api       — list, detail, export (GET)
 *   mappingVersionApi   — new-version (POST)
 *   mappingWorkflowApi  — submit, review, approve, approve-2, reject
 *   mappingImportApi    — bulk-import (multipart)
 *   mappingReportsApi   — RPT-19, RPT-20, RPT-21 (GET + export)
 *
 * All mutating calls auto-inject Idempotency-Key (DEC-021).
 * approve-2 sends X-Step-Up-Token header (DEC-027).
 */

import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
  API_BASE_URL,
  ApiError,
  NetworkError,
  type ApiErrorBody,
} from "@/lib/api";
import type {
  MappingP12HeaderSummary,
  MappingP12HeaderDetail,
  WorkflowActionResponse,
  NewVersionRequest,
  ReviewRequest,
  ApproveRequest,
  RejectRequest,
  BulkImportResult,
  Rpt19Coverage,
  Rpt20Validation,
  AuditLogEntry,
} from "@/lib/schemas/mapping-jurnal-p12.schema";

// ---------------------------------------------------------------------------
// Base paths
// ---------------------------------------------------------------------------

const BASE = "/api/v1/mapping-jurnal";
const REPORTS_BASE = "/api/v1/reports";

// ---------------------------------------------------------------------------
// Param types
// ---------------------------------------------------------------------------

export interface MappingP12ListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[workflow_status]"?: string;
  "filter[workflow_path]"?: string;
  "filter[regulated_flag]"?: boolean;
  "filter[event_code]"?: string;
}

export interface MappingP12ExportParams {
  format: "csv" | "xlsx";
  "filter[workflow_status]"?: string;
  "filter[regulated_flag]"?: boolean;
}

export type MappingP12ListResponse = ListResponse<MappingP12HeaderSummary>;
export type MappingP12DetailResponse = SingleResponse<MappingP12HeaderDetail>;
export type MappingP12WorkflowResponse = WorkflowActionResponse;
export type Rpt21ListResponse = ListResponse<AuditLogEntry>;

export interface Rpt21ListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  "filter[event_code]"?: string;
  "filter[actor_role]"?: string;
  "filter[action]"?: string;
}

// ---------------------------------------------------------------------------
// 1. mappingP12Api — list + detail + export
// ---------------------------------------------------------------------------

export const mappingP12Api = {
  list(params: MappingP12ListParams = {}): Promise<MappingP12ListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MappingP12ListResponse>(`${BASE}${qs}`);
  },

  get(eventCode: string): Promise<MappingP12DetailResponse> {
    return apiGet<MappingP12DetailResponse>(`${BASE}/${eventCode}`);
  },

  exportUrl(params: MappingP12ExportParams): string {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// 2. mappingVersionApi — create new version
// ---------------------------------------------------------------------------

export const mappingVersionApi = {
  createVersion(
    eventCode: string,
    body: NewVersionRequest,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<MappingP12HeaderSummary>> {
    return apiPost<SingleResponse<MappingP12HeaderSummary>>(
      `${BASE}/${eventCode}/new-version`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// 3. mappingWorkflowApi — submit, review, approve, approve-2, reject
// ---------------------------------------------------------------------------

export const mappingWorkflowApi = {
  submit(
    eventCode: string,
    versionId: string,
    body: { comment: string },
    idempotencyKey = uuidv4(),
  ): Promise<MappingP12WorkflowResponse> {
    return apiPost<MappingP12WorkflowResponse>(
      `${BASE}/${eventCode}/version/${versionId}/submit`,
      body,
      idempotencyKey,
    );
  },

  review(
    eventCode: string,
    versionId: string,
    body: ReviewRequest,
    idempotencyKey = uuidv4(),
  ): Promise<MappingP12WorkflowResponse> {
    return apiPost<MappingP12WorkflowResponse>(
      `${BASE}/${eventCode}/version/${versionId}/review`,
      body,
      idempotencyKey,
    );
  },

  approve(
    eventCode: string,
    versionId: string,
    body: ApproveRequest,
    idempotencyKey = uuidv4(),
  ): Promise<MappingP12WorkflowResponse> {
    return apiPost<MappingP12WorkflowResponse>(
      `${BASE}/${eventCode}/version/${versionId}/approve`,
      body,
      idempotencyKey,
    );
  },

  /**
   * approve-2 — 6-eyes ROLE-RISK path.
   * Sends X-Step-Up-Token header (DEC-027).
   */
  async approve2(
    eventCode: string,
    versionId: string,
    body: ApproveRequest,
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<MappingP12WorkflowResponse> {
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
      "Content-Type": "application/json",
      "X-Step-Up-Token": stepUpToken,
    };
    if (token) headers["Authorization"] = `Bearer ${token}`;

    let response: Response;
    try {
      response = await fetch(
        `${API_BASE_URL}${BASE}/${eventCode}/version/${versionId}/approve-2`,
        { method: "POST", headers, body: JSON.stringify(body) },
      );
    } catch (cause) {
      throw new NetworkError(cause);
    }

    if (!response.ok) {
      let errBody: { error?: ApiErrorBody };
      try {
        errBody = await response.json();
      } catch {
        errBody = {
          error: {
            code: "INTERNAL",
            message: `HTTP ${response.status}`,
            details: [],
            traceId: "",
          },
        };
      }
      throw new ApiError(
        response.status,
        errBody.error ?? {
          code: "INTERNAL",
          message: "Terjadi kesalahan server",
          details: [],
          traceId: "",
        },
      );
    }
    return (await response.json()) as MappingP12WorkflowResponse;
  },

  reject(
    eventCode: string,
    versionId: string,
    body: RejectRequest,
    idempotencyKey = uuidv4(),
  ): Promise<MappingP12WorkflowResponse> {
    return apiPost<MappingP12WorkflowResponse>(
      `${BASE}/${eventCode}/version/${versionId}/reject`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// 4. mappingImportApi — bulk-import multipart XLSX
// ---------------------------------------------------------------------------

export const mappingImportApi = {
  async import(
    file: File,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<BulkImportResult>> {
    const formData = new FormData();
    formData.append("file", file);

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
    };
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    let response: Response;
    try {
      response = await fetch(`${API_BASE_URL}${BASE}/bulk-import`, {
        method: "POST",
        headers,
        body: formData,
      });
    } catch (cause) {
      throw new NetworkError(cause);
    }

    if (!response.ok) {
      let errBody: { error?: ApiErrorBody };
      try {
        errBody = await response.json();
      } catch {
        errBody = {
          error: {
            code: "INTERNAL",
            message: `HTTP ${response.status}`,
            details: [],
            traceId: "",
          },
        };
      }
      throw new ApiError(
        response.status,
        errBody.error ?? {
          code: "INTERNAL",
          message: "Terjadi kesalahan server",
          details: [],
          traceId: "",
        },
      );
    }
    return (await response.json()) as SingleResponse<BulkImportResult>;
  },
};

// ---------------------------------------------------------------------------
// 5. mappingReportsApi — RPT-19, RPT-20, RPT-21
// ---------------------------------------------------------------------------

export const mappingReportsApi = {
  getRpt19(): Promise<SingleResponse<Rpt19Coverage>> {
    return apiGet<SingleResponse<Rpt19Coverage>>(
      `${REPORTS_BASE}/rpt-19-mapping-coverage`,
    );
  },

  exportRpt19Url(format: "csv" | "xlsx"): string {
    return `${REPORTS_BASE}/rpt-19-mapping-coverage/export?format=${format}`;
  },

  getRpt20(): Promise<SingleResponse<Rpt20Validation>> {
    return apiGet<SingleResponse<Rpt20Validation>>(
      `${REPORTS_BASE}/rpt-20-mapping-validation`,
    );
  },

  exportRpt20Url(format: "csv" | "xlsx"): string {
    return `${REPORTS_BASE}/rpt-20-mapping-validation/export?format=${format}`;
  },

  getRpt21(params: Rpt21ListParams = {}): Promise<Rpt21ListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<Rpt21ListResponse>(`${REPORTS_BASE}/rpt-21-mapping-history${qs}`);
  },

  exportRpt21Url(params: { format: "csv" | "xlsx"; "filter[event_code]"?: string }): string {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return `${REPORTS_BASE}/rpt-21-mapping-history/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const mappingP12QueryKeys = {
  all: ["mapping-jurnal-p12"] as const,
  list: (params?: MappingP12ListParams) =>
    [...mappingP12QueryKeys.all, "list", params ?? {}] as const,
  detail: (eventCode: string) =>
    [...mappingP12QueryKeys.all, "detail", eventCode] as const,
  rpt19: () => [...mappingP12QueryKeys.all, "rpt19"] as const,
  rpt20: () => [...mappingP12QueryKeys.all, "rpt20"] as const,
  rpt21: (params?: Rpt21ListParams) =>
    [...mappingP12QueryKeys.all, "rpt21", params ?? {}] as const,
} as const;
