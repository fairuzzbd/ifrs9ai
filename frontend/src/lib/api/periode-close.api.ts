/**
 * API client — P5-M4 Periode Buku Close Workflow
 * Mirrors backend routing from api/openapi/app-d-periode-close.yaml
 *
 * All mutating calls auto-generate Idempotency-Key (via apiPost).
 * Step-up MFA endpoints include X-Step-Up-Token header when provided.
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
  SoftCloseRequestBody,
  SoftCloseRequestResponse,
  WorkflowApproveBody,
  SoftCloseApproveResponse,
  HardCloseRequestBody,
  HardCloseRequestResponse,
  HardCloseApproveResponse,
  RejectBody,
  PeriodeStateTransitionResponse,
  ReopenRequestBody,
  ReopenRequestResponse,
  ReopenApproveResponse,
  ClosingChecklistResponse,
  PeriodeBukuListItem,
  StatusPeriodeListItem,
  MfaStepUpRequest,
  MfaStepUpResponse,
  ChecklistSnapshotDetail,
  PeriodeBukuDetail,
} from "@/lib/schemas/periode-close.schema";

// ---------------------------------------------------------------------------
// Shared response types
// ---------------------------------------------------------------------------

export type SoftCloseRequestApiResponse = SingleResponse<SoftCloseRequestResponse>;
export type SoftCloseApproveApiResponse = SingleResponse<SoftCloseApproveResponse>;
export type HardCloseRequestApiResponse = SingleResponse<HardCloseRequestResponse>;
export type HardCloseApproveApiResponse = SingleResponse<HardCloseApproveResponse>;
export type HardCloseRejectApiResponse = SingleResponse<PeriodeStateTransitionResponse>;
export type ReopenRequestApiResponse = SingleResponse<ReopenRequestResponse>;
export type ReopenApproveApiResponse = SingleResponse<ReopenApproveResponse>;
export type ClosingChecklistApiResponse = SingleResponse<ClosingChecklistResponse>;
export type PeriodeBukuListApiResponse = ListResponse<PeriodeBukuListItem>;
export type StatusPeriodeListApiResponse = ListResponse<StatusPeriodeListItem>;
export type PeriodeBukuDetailApiResponse = SingleResponse<PeriodeBukuDetail>;
export type ChecklistSnapshotApiResponse = SingleResponse<ChecklistSnapshotDetail>;
export type MfaStepUpApiResponse = SingleResponse<MfaStepUpResponse>;

// ---------------------------------------------------------------------------
// Query params
// ---------------------------------------------------------------------------

export interface PeriodeBukuListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[status_periode]"?: string;
  "filter[tahun_buku]"?: number;
  "filter[bulan]"?: number;
  "filter[tipe_periode]"?: string;
  export?: "csv" | "xlsx";
}

export interface StatusPeriodeReportParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[status_periode]"?: string;
  "filter[tahun_buku]"?: number;
  "filter[bulan]"?: number;
  "filter[tipe_periode]"?: string;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Base path
// ---------------------------------------------------------------------------

const PERIODE_BASE = "/api/v1/periode-buku";
const AUTH_BASE = "/api/v1/auth";

// ---------------------------------------------------------------------------
// Helper — POST with optional X-Step-Up-Token header
// ---------------------------------------------------------------------------

async function apiPostWithStepUp<T>(
  path: string,
  body: unknown,
  idempotencyKey: string,
  stepUpToken?: string,
): Promise<T> {
  const headers: Record<string, string> = {
    "Idempotency-Key": idempotencyKey,
    "Content-Type": "application/json",
  };
  if (stepUpToken) {
    headers["X-Step-Up-Token"] = stepUpToken;
  }

  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081";
  const response = await fetch(`${API_BASE_URL}${path}`, {
    method: "POST",
    headers,
    body: JSON.stringify(body),
  });

  if (!response.ok) {
    const apiLib = await import("@/lib/api");
    type ErrBody = { code: string; message: string; details: { field: string; rule: string; message: string }[]; traceId: string };
    const empty: ErrBody = {
      code: "INTERNAL",
      message: `HTTP ${response.status} ${response.statusText}`,
      details: [],
      traceId: "",
    };
    let errBody: ErrBody;
    try {
      const parsed = await response.json() as { error?: ErrBody };
      errBody = parsed.error ?? empty;
    } catch {
      errBody = empty;
    }
    throw new apiLib.ApiError(response.status, errBody);
  }

  return (await response.json()) as T;
}

// ---------------------------------------------------------------------------
// S1 — Soft-close request
// ---------------------------------------------------------------------------

export const periodeStatusApi = {
  /**
   * GET /periode-buku — cursor-paginated list
   * Permission: periode.read
   */
  list(params: PeriodeBukuListParams = {}): Promise<PeriodeBukuListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PeriodeBukuListApiResponse>(`${PERIODE_BASE}${qs}`);
  },

  /**
   * GET /periode-buku/{id} — full detail
   * Permission: periode.read
   */
  get(id: string): Promise<PeriodeBukuDetailApiResponse> {
    return apiGet<PeriodeBukuDetailApiResponse>(`${PERIODE_BASE}/${id}`);
  },

  /** Export URL helper */
  exportUrl(params: PeriodeBukuListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${PERIODE_BASE}/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// S1 — Soft-close request
// ---------------------------------------------------------------------------

export const periodeSoftCloseApi = {
  /**
   * POST /periode-buku/{id}/soft-close-request
   * Permission: periode.softclose.request (ROLE-AKUN-CTL)
   * S1-AC1..S1-AC4
   */
  request(
    periodeId: string,
    body: SoftCloseRequestBody,
    idempotencyKey = uuidv4(),
  ): Promise<SoftCloseRequestApiResponse> {
    return apiPost<SoftCloseRequestApiResponse>(
      `${PERIODE_BASE}/${periodeId}/soft-close-request`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /periode-buku/{id}/soft-close-approve
   * Permission: periode.softclose.approve (ROLE-AKUN-CTL — SoD: ≠ requester)
   * S2-AC1..S2-AC4
   */
  approve(
    periodeId: string,
    body: WorkflowApproveBody,
    idempotencyKey = uuidv4(),
  ): Promise<SoftCloseApproveApiResponse> {
    return apiPost<SoftCloseApproveApiResponse>(
      `${PERIODE_BASE}/${periodeId}/soft-close-approve`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// S3 — Hard-close request + approve + reject
// ---------------------------------------------------------------------------

export const periodeHardCloseApi = {
  /**
   * POST /periode-buku/{id}/hard-close-request
   * Permission: periode.hardclose.request (ROLE-AKUN-CTL)
   * S3-AC1 (step 1)
   */
  request(
    periodeId: string,
    body: HardCloseRequestBody,
    idempotencyKey = uuidv4(),
  ): Promise<HardCloseRequestApiResponse> {
    return apiPost<HardCloseRequestApiResponse>(
      `${PERIODE_BASE}/${periodeId}/hard-close-request`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /periode-buku/{id}/hard-close-approve
   * Permission: periode.hardclose.approve (ROLE-CFO)
   * X-Step-Up-Token WAJIB (DEC-027)
   * S3-AC1 (step 2), S3-AC2, S3-AC3
   */
  approve(
    periodeId: string,
    body: WorkflowApproveBody,
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<HardCloseApproveApiResponse> {
    return apiPostWithStepUp<HardCloseApproveApiResponse>(
      `${PERIODE_BASE}/${periodeId}/hard-close-approve`,
      body,
      idempotencyKey,
      stepUpToken,
    );
  },

  /**
   * POST /periode-buku/{id}/hard-close-reject
   * Permission: periode.hardclose.approve (ROLE-CFO)
   * No step-up MFA required (OQ-M4-3a)
   */
  reject(
    periodeId: string,
    body: RejectBody,
    idempotencyKey = uuidv4(),
  ): Promise<HardCloseRejectApiResponse> {
    return apiPost<HardCloseRejectApiResponse>(
      `${PERIODE_BASE}/${periodeId}/hard-close-reject`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// S4 — Reopen request + approve
// ---------------------------------------------------------------------------

export const periodeReopenApi = {
  /**
   * POST /periode-buku/{id}/reopen-request
   * Permission: periode.reopen.request (ROLE-CFO)
   * S4-AC1..S4-AC4
   */
  request(
    periodeId: string,
    body: ReopenRequestBody,
    idempotencyKey = uuidv4(),
  ): Promise<ReopenRequestApiResponse> {
    return apiPost<ReopenRequestApiResponse>(
      `${PERIODE_BASE}/${periodeId}/reopen-request`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /periode-buku/{id}/reopen-approve
   * Permission: periode.reopen.approve (ROLE-CFO)
   * X-Step-Up-Token WAJIB jika CLOSED→SOFT_CLOSED (DEC-027)
   * S4-AC1 (no MFA), S4-AC2 (with MFA)
   */
  approve(
    periodeId: string,
    body: WorkflowApproveBody,
    stepUpToken: string | undefined,
    idempotencyKey = uuidv4(),
  ): Promise<ReopenApproveApiResponse> {
    return apiPostWithStepUp<ReopenApproveApiResponse>(
      `${PERIODE_BASE}/${periodeId}/reopen-approve`,
      body,
      idempotencyKey,
      stepUpToken,
    );
  },
};

// ---------------------------------------------------------------------------
// S5 — Closing checklist + snapshot detail
// ---------------------------------------------------------------------------

export const periodeChecklistApi = {
  /**
   * GET /periode-buku/{id}/closing-checklist
   * Permission: periode.read
   * Auto-polls every 30s (managed by caller via TanStack Query refetchInterval)
   * S5-AC1, S5-AC4
   */
  get(periodeId: string): Promise<ClosingChecklistApiResponse> {
    return apiGet<ClosingChecklistApiResponse>(
      `${PERIODE_BASE}/${periodeId}/closing-checklist`,
    );
  },

  /**
   * GET /periode-buku/{periodeId}/closing-checklist-snapshot/{snapshotId}
   * Permission: periode.read
   * S5-AC1, S5-AC4
   */
  getSnapshot(
    periodeId: string,
    snapshotId: string,
  ): Promise<ChecklistSnapshotApiResponse> {
    return apiGet<ChecklistSnapshotApiResponse>(
      `${PERIODE_BASE}/${periodeId}/closing-checklist-snapshot/${snapshotId}`,
    );
  },
};

// ---------------------------------------------------------------------------
// S5 — Status periode report
// ---------------------------------------------------------------------------

export const periodeReportApi = {
  /**
   * GET /reports/status-periode
   * Permission: periode.read
   * S5-AC2, S5-AC3
   */
  listStatus(params: StatusPeriodeReportParams = {}): Promise<StatusPeriodeListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<StatusPeriodeListApiResponse>(`/api/v1/reports/status-periode${qs}`);
  },

  /** Export URL helper */
  exportUrl(params: StatusPeriodeReportParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `/api/v1/reports/status-periode${qs}`;
  },
};

// ---------------------------------------------------------------------------
// MFA Step-up (used by MFAStepUpDialog)
// ---------------------------------------------------------------------------

export const mfaStepUpApi = {
  /**
   * POST /auth/step-up
   * Returns stepUpToken (TTL 5 min)
   * Used before hard-close-approve and reopen-approve (CLOSED→SOFT_CLOSED)
   */
  challenge(
    body: MfaStepUpRequest,
    idempotencyKey = uuidv4(),
  ): Promise<MfaStepUpApiResponse> {
    return apiPost<MfaStepUpApiResponse>(
      `${AUTH_BASE}/step-up`,
      body,
      idempotencyKey,
    );
  },
};
