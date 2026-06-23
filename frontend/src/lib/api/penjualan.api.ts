/**
 * API client — P5-M8 Penjualan/Pencairan Instrumen
 * Mirrors api/openapi/app-b-penjualan.yaml
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
  PenjualanListItem,
  PenjualanDetail,
  CreatePenjualanInput,
  CreatePenjualanResponse,
  ApprovePenjualanResponse,
  RejectPenjualanResponse,
  PenjualanPreview,
  BMAlertItem,
} from "@/lib/schemas/penjualan.schema";

// ---------------------------------------------------------------------------
// Base path
// ---------------------------------------------------------------------------

const BASE = "/api/v1/trx/penjualan";

// ---------------------------------------------------------------------------
// List params
// ---------------------------------------------------------------------------

export interface PenjualanListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[instrumen_id]"?: string;
  "filter[status]"?: string;
  "filter[jenis_disposal]"?: string;
  "filter[tanggal_eksekusi]"?: string;
  "filter[maker_id]"?: string;
  "filter[klasifikasi]"?: string;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Response type aliases
// ---------------------------------------------------------------------------

export type PenjualanListApiResponse = ListResponse<PenjualanListItem>;
export type PenjualanDetailApiResponse = SingleResponse<PenjualanDetail>;
export type PenjualanCreateApiResponse = SingleResponse<CreatePenjualanResponse>;
export type PenjualanApproveApiResponse = SingleResponse<ApprovePenjualanResponse>;
export type PenjualanRejectApiResponse = SingleResponse<RejectPenjualanResponse>;
export type PenjualanPreviewApiResponse = SingleResponse<PenjualanPreview>;
export type BMAlertListApiResponse = SingleResponse<BMAlertItem[]>;

// ---------------------------------------------------------------------------
// penjualanListApi — GET /trx/penjualan (list + export)
// ---------------------------------------------------------------------------

export const penjualanListApi = {
  /**
   * GET /trx/penjualan — cursor-paginated list with sort + filter
   * Default sort: created_at DESC
   * Permission: penjualan.read
   */
  list(params: PenjualanListParams = {}): Promise<PenjualanListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PenjualanListApiResponse>(`${BASE}${qs}`);
  },

  /**
   * Export URL helper — inline ≤10k rows, async >10k rows (returns 202)
   * Audit: PENJUALAN.EXPORT in-transaction
   */
  exportUrl(params: PenjualanListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}${qs}`;
  },
};

// ---------------------------------------------------------------------------
// penjualanDetailApi — GET /trx/penjualan/{id} + preview
// ---------------------------------------------------------------------------

export const penjualanDetailApi = {
  /**
   * GET /trx/penjualan/{id} — full detail including preview kalkulasi
   * Permission: penjualan.read
   */
  get(id: string): Promise<PenjualanDetailApiResponse> {
    return apiGet<PenjualanDetailApiResponse>(`${BASE}/${id}`);
  },

  /**
   * GET /trx/penjualan/{id}/preview — server recompute, read-only (S1 kalkulasi)
   * Permission: penjualan.read
   */
  preview(id: string): Promise<PenjualanPreviewApiResponse> {
    return apiGet<PenjualanPreviewApiResponse>(`${BASE}/${id}/preview`);
  },
};

// ---------------------------------------------------------------------------
// penjualanCreateApi — POST /trx/penjualan (S1)
// ---------------------------------------------------------------------------

export const penjualanCreateApi = {
  /**
   * POST /trx/penjualan
   * Creates penjualan request → status PENDING_APPROVAL immediately.
   * Server computes preview: proceed, cost_basis, realized_gl, oci_recycled, bm_freq_impact_pct.
   * Permission: penjualan.create (ROLE-MAKER-TR)
   * Idempotency-Key: auto-generated (DEC-021)
   */
  create(
    body: CreatePenjualanInput,
    idempotencyKey = uuidv4(),
  ): Promise<PenjualanCreateApiResponse> {
    return apiPost<PenjualanCreateApiResponse>(BASE, body, idempotencyKey);
  },
};

// ---------------------------------------------------------------------------
// penjualanWorkflowApi — approve + reject (S2, SoD enforced)
// ---------------------------------------------------------------------------

export const penjualanWorkflowApi = {
  /**
   * POST /trx/penjualan/{id}/approve
   * PENDING_APPROVAL → POSTED (all side-effects in single DB tx: OCI recycling, BM check, jurnal, derecognition).
   * Permission: penjualan.approve (ROLE-APPR-TR)
   * SoD: approver_id ≠ maker_id (DEC-017)
   * Idempotency-Key: auto-generated (DEC-021)
   * Rate limit: 10 req/min (sensitif per api-conventions.md)
   */
  approve(
    penjualanId: string,
    body: { comment: string; signatureMethod: "JWT_STEP_UP" },
    idempotencyKey = uuidv4(),
  ): Promise<PenjualanApproveApiResponse> {
    return apiPost<PenjualanApproveApiResponse>(
      `${BASE}/${penjualanId}/approve`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /trx/penjualan/{id}/reject
   * PENDING_APPROVAL → REJECTED; reason ≥ 30 char WAJIB (S2).
   * Permission: penjualan.reject (ROLE-APPR-TR)
   * SoD: approver_id ≠ maker_id (DEC-017)
   * Idempotency-Key: auto-generated (DEC-021)
   * Rate limit: 10 req/min (sensitif)
   */
  reject(
    penjualanId: string,
    body: { reason: string; signatureMethod: "JWT_STEP_UP" },
    idempotencyKey = uuidv4(),
  ): Promise<PenjualanRejectApiResponse> {
    return apiPost<PenjualanRejectApiResponse>(
      `${BASE}/${penjualanId}/reject`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// penjualanBMApi — BM frequency alerts (S4)
// ---------------------------------------------------------------------------

export const penjualanBMApi = {
  /**
   * GET /trx/penjualan/bm-frequency-alerts
   * Returns instrumen HTC with cumulative 12-month disposal exceeding warn threshold.
   * Permission: penjualan.read (ROLE-MAKER-TR, ROLE-APPR-TR, ROLE-RISK)
   */
  bmAlerts(): Promise<BMAlertListApiResponse> {
    return apiGet<BMAlertListApiResponse>(`${BASE}/bm-frequency-alerts`);
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const penjualanQueryKeys = {
  all: ["penjualan"] as const,

  // List
  lists: () => [...penjualanQueryKeys.all, "list"] as const,
  list: (params: PenjualanListParams) =>
    [...penjualanQueryKeys.lists(), params] as const,

  // Detail
  details: () => [...penjualanQueryKeys.all, "detail"] as const,
  detail: (id: string) => [...penjualanQueryKeys.details(), id] as const,

  // Preview
  previews: () => [...penjualanQueryKeys.all, "preview"] as const,
  preview: (id: string) => [...penjualanQueryKeys.previews(), id] as const,

  // BM alerts
  bmAlerts: () => [...penjualanQueryKeys.all, "bm-alerts"] as const,
} as const;
