/**
 * API client for APP-B P5-M1 Penempatan Deposito.
 * All 15 endpoints per OpenAPI app-b-penempatan-deposito.yaml.
 */

import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  apiDelete,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  PenempatanDeposito,
  PenempatanListItem,
  PenempatanCreateInput,
  PenempatanUpdateInput,
  PenempatanListFilters,
  EirPreviewResult,
  AuditTimelineEvent,
  ApproveResponseData,
} from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Base path
// ---------------------------------------------------------------------------

const BASE = "/api/v1/trx/penempatan-deposito";

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

export type PenempatanListResponse = ListResponse<PenempatanListItem>;
export type PenempatanDetailResponse = SingleResponse<PenempatanDeposito>;
export type PenempatanApproveResponse = {
  data: ApproveResponseData;
  meta: { traceId: string };
};
export type PenempatanWithdrawResponse = {
  data: { id: string; kodeTransaksi: string; workflowStatus: "CANCELLED" };
  meta: { traceId: string };
};
export type EirPreviewResponse = SingleResponse<EirPreviewResult>;
export type AuditTimelineResponse = ListResponse<AuditTimelineEvent> & {
  hashChainValid: boolean | null;
};

// ---------------------------------------------------------------------------
// PATCH helper (not in lib/api — using POST with method override not needed)
// ---------------------------------------------------------------------------

async function apiPatch<T>(path: string, body: unknown, idempotencyKey?: string): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (idempotencyKey) {
    headers["Idempotency-Key"] = idempotencyKey;
  }
  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const url = `${process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081"}${path}`;
  const res = await fetch(url, {
    method: "PATCH",
    headers,
    body: JSON.stringify(body),
  });

  if (!res.ok) {
    let errBody: { error?: { code: string; message: string; details: unknown[]; traceId: string } };
    try {
      errBody = await res.json();
    } catch {
      errBody = { error: { code: "INTERNAL", message: `HTTP ${res.status}`, details: [], traceId: "" } };
    }
    const { ApiError } = await import("@/lib/api");
    throw new ApiError(res.status, {
      code: errBody.error?.code ?? "INTERNAL",
      message: errBody.error?.message ?? "Unknown error",
      details: (errBody.error?.details ?? []) as { field: string; rule: string; message: string }[],
      traceId: errBody.error?.traceId ?? "",
    });
  }

  return (await res.json()) as T;
}

// ---------------------------------------------------------------------------
// API object
// ---------------------------------------------------------------------------

export const penempatanApi = {
  // ── List ──────────────────────────────────────────────────────────────────

  list(params: PenempatanListFilters = {}): Promise<PenempatanListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PenempatanListResponse>(`${BASE}${qs}`);
  },

  // ── Create ────────────────────────────────────────────────────────────────

  create(
    data: Omit<PenempatanCreateInput, "attestChecked">,
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(BASE, data, idempotencyKey);
  },

  // ── Get detail ────────────────────────────────────────────────────────────

  get(id: string): Promise<PenempatanDetailResponse> {
    return apiGet<PenempatanDetailResponse>(`${BASE}/${id}`);
  },

  // ── Update (PATCH, DRAFT only) ────────────────────────────────────────────

  update(
    id: string,
    data: PenempatanUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPatch<PenempatanDetailResponse>(`${BASE}/${id}`, data, idempotencyKey);
  },

  // ── Withdraw (DELETE / soft-delete) ──────────────────────────────────────

  withdraw(
    id: string,
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanWithdrawResponse> {
    return apiDelete<PenempatanWithdrawResponse>(`${BASE}/${id}`, idempotencyKey);
  },

  // ── Submit ────────────────────────────────────────────────────────────────

  submit(
    id: string,
    body: { comment: string; signatureMethod: string },
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(
      `${BASE}/${id}/submit`,
      body,
      idempotencyKey,
    );
  },

  // ── Review ────────────────────────────────────────────────────────────────

  review(
    id: string,
    body: { comment: string; signatureMethod: string },
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(
      `${BASE}/${id}/review`,
      body,
      idempotencyKey,
    );
  },

  // ── Approve (MFA step-up required) ───────────────────────────────────────

  approve(
    id: string,
    body: { comment: string; signatureMethod: string },
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanApproveResponse> {
    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
      "X-Step-Up-Token": stepUpToken,
    };
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const url = `${process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081"}${BASE}/${id}/approve`;
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...headers },
      body: JSON.stringify(body),
    }).then(async (res) => {
      if (!res.ok) {
        const errBody = await res.json().catch(() => ({
          error: { code: "INTERNAL", message: `HTTP ${res.status}`, details: [], traceId: "" },
        })) as { error: { code: string; message: string; details: { field: string; rule: string; message: string }[]; traceId: string } };
        const { ApiError } = await import("@/lib/api");
        throw new ApiError(res.status, errBody.error);
      }
      return res.json() as Promise<PenempatanApproveResponse>;
    });
  },

  // ── Reject ────────────────────────────────────────────────────────────────

  reject(
    id: string,
    body: { comment: string; signatureMethod: string },
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(
      `${BASE}/${id}/reject`,
      body,
      idempotencyKey,
    );
  },

  // ── Terminate propose ─────────────────────────────────────────────────────

  terminate(
    id: string,
    body: { terminateReason: string; dokumenTerminasiId?: string; signatureMethod: string },
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(
      `${BASE}/${id}/terminate`,
      body,
      idempotencyKey,
    );
  },

  // ── Terminate review ──────────────────────────────────────────────────────

  terminateReview(
    id: string,
    body: { comment: string; signatureMethod: string },
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(
      `${BASE}/${id}/terminate-review`,
      body,
      idempotencyKey,
    );
  },

  // ── Terminate approve (MFA step-up required) ──────────────────────────────

  terminateApprove(
    id: string,
    body: { comment: string; signatureMethod: string },
    stepUpToken: string,
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    const headers: Record<string, string> = {
      "Idempotency-Key": idempotencyKey,
      "X-Step-Up-Token": stepUpToken,
    };
    const token =
      typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const url = `${process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081"}${BASE}/${id}/terminate-approve`;
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json", ...headers },
      body: JSON.stringify(body),
    }).then(async (res) => {
      if (!res.ok) {
        const errBody = await res.json().catch(() => ({
          error: { code: "INTERNAL", message: `HTTP ${res.status}`, details: [], traceId: "" },
        })) as { error: { code: string; message: string; details: { field: string; rule: string; message: string }[]; traceId: string } };
        const { ApiError } = await import("@/lib/api");
        throw new ApiError(res.status, errBody.error);
      }
      return res.json() as Promise<PenempatanDetailResponse>;
    });
  },

  // ── Terminate reject ──────────────────────────────────────────────────────

  terminateReject(
    id: string,
    body: { comment: string; signatureMethod: string },
    idempotencyKey = uuidv4(),
  ): Promise<PenempatanDetailResponse> {
    return apiPost<PenempatanDetailResponse>(
      `${BASE}/${id}/terminate-reject`,
      body,
      idempotencyKey,
    );
  },

  // ── EIR Preview ───────────────────────────────────────────────────────────

  eirPreview(id: string): Promise<EirPreviewResponse> {
    return apiGet<EirPreviewResponse>(`${BASE}/${id}/eir-preview`);
  },

  // ── Audit timeline ────────────────────────────────────────────────────────

  auditTimeline(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<AuditTimelineResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<AuditTimelineResponse>(`${BASE}/${id}/audit-timeline${qs}`);
  },
};
