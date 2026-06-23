/**
 * API client — P5-M7 Renewal Deposito
 * Mirrors api/openapi/app-b-renewal-deposito.yaml
 *
 * All mutating calls auto-generate Idempotency-Key (DEC-021).
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
  RenewalListItem,
  RenewalDetail,
  CreateRenewalInput,
  CreateRenewalResponse,
  ApproveRenewalResponse,
  RejectRenewalResponse,
  RenewalPreview,
} from "@/lib/schemas/renewal.schema";

// ---------------------------------------------------------------------------
// Base path
// ---------------------------------------------------------------------------

const BASE = "/api/v1/trx/renewal";

// ---------------------------------------------------------------------------
// List params
// ---------------------------------------------------------------------------

export interface RenewalListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[instrumen_id]"?: string;
  "filter[status]"?: string;
  "filter[skema]"?: string;
  "filter[tanggal_efektif]"?: string;
  "filter[maker_id]"?: string;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Response type aliases
// ---------------------------------------------------------------------------

export type RenewalListApiResponse = ListResponse<RenewalListItem>;
export type RenewalDetailApiResponse = SingleResponse<RenewalDetail>;
export type RenewalCreateApiResponse = SingleResponse<CreateRenewalResponse>;
export type RenewalApproveApiResponse = SingleResponse<ApproveRenewalResponse>;
export type RenewalRejectApiResponse = SingleResponse<RejectRenewalResponse>;
export type RenewalPreviewApiResponse = SingleResponse<RenewalPreview>;

// ---------------------------------------------------------------------------
// renewalListApi — GET /trx/renewal (list + export)
// ---------------------------------------------------------------------------

export const renewalListApi = {
  /**
   * GET /trx/renewal — cursor-paginated list with sort + filter
   * Default sort: created_at DESC
   * Permission: transaksi.read
   */
  list(params: RenewalListParams = {}): Promise<RenewalListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<RenewalListApiResponse>(`${BASE}${qs}`);
  },

  /**
   * Export URL helper — inline ≤10k rows, async >10k rows (returns 202)
   * Audit: RENEWAL.EXPORT in-transaction
   */
  exportUrl(params: RenewalListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}${qs}`;
  },
};

// ---------------------------------------------------------------------------
// renewalDetailApi — GET /trx/renewal/{id}
// ---------------------------------------------------------------------------

export const renewalDetailApi = {
  /**
   * GET /trx/renewal/{id} — full detail including preview kalkulasi
   * Permission: transaksi.read
   */
  get(id: string): Promise<RenewalDetailApiResponse> {
    return apiGet<RenewalDetailApiResponse>(`${BASE}/${id}`);
  },

  /**
   * GET /trx/renewal/{id}/preview — server recompute, read-only (S4)
   * Permission: transaksi.read
   */
  preview(id: string): Promise<RenewalPreviewApiResponse> {
    return apiGet<RenewalPreviewApiResponse>(`${BASE}/${id}/preview`);
  },
};

// ---------------------------------------------------------------------------
// renewalCreateApi — POST /trx/renewal (S1)
// ---------------------------------------------------------------------------

export const renewalCreateApi = {
  /**
   * POST /trx/renewal
   * Creates renewal request → status PENDING_APPROVAL immediately.
   * Server computes preview: bunga_kotor, PPh_20pct, bunga_bersih, pokok_baru, EIR_baru.
   * Permission: transaksi.create (ROLE-MAKER-TR)
   * Idempotency-Key: auto-generated (DEC-021)
   */
  create(
    body: CreateRenewalInput,
    idempotencyKey = uuidv4(),
  ): Promise<RenewalCreateApiResponse> {
    return apiPost<RenewalCreateApiResponse>(BASE, body, idempotencyKey);
  },
};

// ---------------------------------------------------------------------------
// renewalWorkflowApi — approve + reject (S2, SoD enforced)
// ---------------------------------------------------------------------------

export const renewalWorkflowApi = {
  /**
   * POST /trx/renewal/{id}/approve
   * PENDING_APPROVAL → POSTED (all side-effects in single DB tx).
   * Permission: transaksi.approve (ROLE-APPR-TR)
   * SoD: approver_id ≠ maker_id (DEC-017)
   * Idempotency-Key: auto-generated (DEC-021)
   * Rate limit: 10 req/min (sensitif per api-conventions.md)
   */
  approve(
    renewalId: string,
    body: { comment: string; signatureMethod: "JWT_STEP_UP" },
    idempotencyKey = uuidv4(),
  ): Promise<RenewalApproveApiResponse> {
    return apiPost<RenewalApproveApiResponse>(
      `${BASE}/${renewalId}/approve`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /trx/renewal/{id}/reject
   * PENDING_APPROVAL → REJECTED; comment ≥ 30 char WAJIB (S2-AC4).
   * Permission: transaksi.approve (ROLE-APPR-TR)
   * SoD: approver_id ≠ maker_id (DEC-017)
   * Idempotency-Key: auto-generated (DEC-021)
   * Rate limit: 10 req/min (sensitif)
   */
  reject(
    renewalId: string,
    body: { comment: string; signatureMethod: "JWT_STEP_UP" },
    idempotencyKey = uuidv4(),
  ): Promise<RenewalRejectApiResponse> {
    return apiPost<RenewalRejectApiResponse>(
      `${BASE}/${renewalId}/reject`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const renewalQueryKeys = {
  all: ["renewal"] as const,

  // List
  lists: () => [...renewalQueryKeys.all, "list"] as const,
  list: (params: RenewalListParams) =>
    [...renewalQueryKeys.lists(), params] as const,

  // Detail
  details: () => [...renewalQueryKeys.all, "detail"] as const,
  detail: (id: string) => [...renewalQueryKeys.details(), id] as const,

  // Preview
  previews: () => [...renewalQueryKeys.all, "preview"] as const,
  preview: (id: string) => [...renewalQueryKeys.previews(), id] as const,
} as const;
