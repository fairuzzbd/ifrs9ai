/**
 * API client — P5-M13 Reporting MV Foundation
 * Mirrors api/openapi/app-e-reporting-mv.yaml
 *
 * 4 namespaces:
 *   mvApi           — listMVStatus, triggerMVRefresh
 *   exportApi       — exportReport (inline/async), listExportLog, downloadExport
 *   scheduledEmailApi — list, create, delete, optOut
 *   jobApi          — getJob (reused across modules)
 *
 * All mutating calls auto-inject Idempotency-Key (DEC-021).
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
  MVStatusItem,
  AsyncJobRef,
  ExportLogItem,
  ScheduledEmailItem,
  MVRefreshRequest,
  ExportFormat,
  ReportSlug,
} from "@/lib/schemas/reporting.schema";

// ---------------------------------------------------------------------------
// Base paths
// ---------------------------------------------------------------------------

const ADMIN = "/api/v1/admin";
const REPORTS = "/api/v1/reports";
const JOBS = "/api/v1/jobs";

// ---------------------------------------------------------------------------
// Param types
// ---------------------------------------------------------------------------

export interface MVListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
}

export interface ExportLogListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[format]"?: ExportFormat;
  "filter[status]"?: string;
}

export interface ScheduledEmailListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
}

export type MVListResponse = ListResponse<MVStatusItem>;
export type ExportLogListResponse = ListResponse<ExportLogItem>;
export type ScheduledEmailListResponse = ListResponse<ScheduledEmailItem>;

// ---------------------------------------------------------------------------
// 1. mvApi — MV status + refresh
// ---------------------------------------------------------------------------

export const mvApi = {
  list(params: MVListParams = {}): Promise<MVListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MVListResponse>(`${ADMIN}/mv-status${qs}`);
  },

  /** Returns AsyncJobRef (202). ROLE-IT-ADMIN only. */
  triggerRefresh(
    body: MVRefreshRequest,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<AsyncJobRef>> {
    return apiPost<SingleResponse<AsyncJobRef>>(
      `${ADMIN}/mv-refresh`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// 2. exportApi — report export (inline ≤10k, async >10k) + export log
// ---------------------------------------------------------------------------

export const exportApi = {
  /**
   * Export report. Returns either:
   * - `{ inline: true, url: string }` for sync 200 → trigger browser download via URL
   * - `{ inline: false, job: AsyncJobRef }` for async 202
   */
  exportUrl(slug: ReportSlug, format: ExportFormat, periodeId?: string): string {
    const params: Record<string, string> = { format };
    if (periodeId) params["periode_id"] = periodeId;
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    return `${REPORTS}/${slug}/export${qs}`;
  },

  /**
   * Trigger async export — POST-like via GET with Idempotency-Key.
   * If server returns 200 (inline), resolves with `{ inline: true }`.
   * If server returns 202, resolves with `{ inline: false, job: AsyncJobRef }`.
   */
  async requestExport(
    slug: ReportSlug,
    format: ExportFormat,
    periodeId?: string,
    idempotencyKey = uuidv4(),
  ): Promise<
    | { inline: true; blobUrl: string; filename: string }
    | { inline: false; job: AsyncJobRef }
  > {
    const params: Record<string, string> = { format };
    if (periodeId) params["periode_id"] = periodeId;
    const qs = buildQueryString(params as Record<string, string | number | boolean | null | undefined>);
    const url = `${API_BASE_URL}${REPORTS}/${slug}/export${qs}`;

    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
    };
    if (token) headers["Authorization"] = `Bearer ${token}`;

    let response: Response;
    try {
      response = await fetch(url, { headers });
    } catch (cause) {
      throw new NetworkError(cause);
    }

    if (response.status === 200) {
      const blob = await response.blob();
      const blobUrl = URL.createObjectURL(blob);
      const cd = response.headers.get("content-disposition") ?? "";
      const match = cd.match(/filename="?([^"]+)"?/);
      const filename = match?.[1] ?? `${slug}.${format}`;
      return { inline: true, blobUrl, filename };
    }

    if (response.status === 202) {
      const json = (await response.json()) as SingleResponse<AsyncJobRef>;
      return { inline: false, job: json.data };
    }

    let errBody: { error?: ApiErrorBody };
    try {
      errBody = await response.json();
    } catch {
      errBody = {
        error: { code: "INTERNAL", message: `HTTP ${response.status}`, details: [], traceId: "" },
      };
    }
    throw new ApiError(
      response.status,
      errBody.error ?? { code: "INTERNAL", message: "Terjadi kesalahan server", details: [], traceId: "" },
    );
  },

  listExportLog(params: ExportLogListParams = {}): Promise<ExportLogListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ExportLogListResponse>(`${REPORTS}/export-log${qs}`);
  },

  downloadUrl(exportId: string): string {
    return `${API_BASE_URL}${REPORTS}/export/${exportId}/download`;
  },
};

// ---------------------------------------------------------------------------
// 3. scheduledEmailApi — CRUD + opt-out
// ---------------------------------------------------------------------------

export interface ScheduledEmailCreateRequest {
  reportSlug: string;
  format: ExportFormat;
  frequency: "daily" | "weekly" | "monthly";
  sendTime: string;
  recipients: string[];
  active: boolean;
  subjectTemplate?: string;
  bodyTemplate?: string;
}

export const scheduledEmailApi = {
  list(params: ScheduledEmailListParams = {}): Promise<ScheduledEmailListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ScheduledEmailListResponse>(`${REPORTS}/scheduled-emails${qs}`);
  },

  create(
    body: ScheduledEmailCreateRequest,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<ScheduledEmailItem>> {
    return apiPost<SingleResponse<ScheduledEmailItem>>(
      `${REPORTS}/scheduled-emails`,
      body,
      idempotencyKey,
    );
  },

  async delete(id: string, idempotencyKey = uuidv4()): Promise<void> {
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;

    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
      "Content-Type": "application/json",
    };
    if (token) headers["Authorization"] = `Bearer ${token}`;

    let response: Response;
    try {
      response = await fetch(`${API_BASE_URL}${REPORTS}/scheduled-emails/${id}`, {
        method: "DELETE",
        headers,
      });
    } catch (cause) {
      throw new NetworkError(cause);
    }

    if (!response.ok) {
      let errBody: { error?: ApiErrorBody };
      try {
        errBody = await response.json();
      } catch {
        errBody = { error: { code: "INTERNAL", message: `HTTP ${response.status}`, details: [], traceId: "" } };
      }
      throw new ApiError(
        response.status,
        errBody.error ?? { code: "INTERNAL", message: "Terjadi kesalahan server", details: [], traceId: "" },
      );
    }
  },

  /** Opt-out via signed token — no auth required (public endpoint). */
  async optOut(id: string, token: string, email: string): Promise<void> {
    const url = `${API_BASE_URL}${REPORTS}/scheduled-emails/${id}/opt-out?token=${encodeURIComponent(token)}&email=${encodeURIComponent(email)}`;
    let response: Response;
    try {
      response = await fetch(url, { method: "POST" });
    } catch (cause) {
      throw new NetworkError(cause);
    }
    if (!response.ok) {
      let errBody: { error?: ApiErrorBody };
      try {
        errBody = await response.json();
      } catch {
        errBody = { error: { code: "INTERNAL", message: `HTTP ${response.status}`, details: [], traceId: "" } };
      }
      throw new ApiError(
        response.status,
        errBody.error ?? { code: "INTERNAL", message: "Terjadi kesalahan tidak diketahui", details: [], traceId: "" },
      );
    }
  },
};

// ---------------------------------------------------------------------------
// 4. jobApi — generic job status (reused by JobProgressPanel)
// ---------------------------------------------------------------------------

export interface JobStatusResponse {
  jobId: string;
  type: string;
  status: "queued" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  currentStep?: string;
  startedAt?: string;
  estimatedCompletionAt?: string;
  result?: unknown;
  error?: unknown;
  canCancel?: boolean;
  createdBy?: string;
}

export const jobApi = {
  getStatus(jobId: string): Promise<SingleResponse<JobStatusResponse>> {
    return apiGet<SingleResponse<JobStatusResponse>>(`${JOBS}/${jobId}`);
  },

  streamUrl(jobId: string): string {
    return `${API_BASE_URL}${JOBS}/${jobId}/stream`;
  },

  cancel(jobId: string, idempotencyKey = uuidv4()): Promise<SingleResponse<{ status: string }>> {
    return apiPost<SingleResponse<{ status: string }>>(
      `${JOBS}/${jobId}/cancel`,
      {},
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const reportingQueryKeys = {
  all: ["reporting"] as const,
  mvList: (params?: MVListParams) => [...reportingQueryKeys.all, "mv-list", params ?? {}] as const,
  exportLog: (params?: ExportLogListParams) =>
    [...reportingQueryKeys.all, "export-log", params ?? {}] as const,
  schedEmailList: (params?: ScheduledEmailListParams) =>
    [...reportingQueryKeys.all, "sched-email-list", params ?? {}] as const,
  job: (jobId: string) => [...reportingQueryKeys.all, "job", jobId] as const,
} as const;
