/**
 * periode-buku.api.ts
 *
 * Minimal client for the periode_buku endpoint, used primarily to populate
 * the periode dropdown in Impact MEV-PD and Impact PD forms.
 *
 * Full periode management screens live under APP-D and will extend this file.
 */

import { apiGet, buildQueryString, type ListResponse } from "@/lib/api";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Minimal shape returned by GET /api/v1/master/periode-buku (list). */
export interface PeriodeBukuItem {
  id: string;
  /** Format: YYYY-MM — accounting period month */
  periodeBulan: string;
  /** Display label, e.g. "Juni 2026" */
  label: string;
  status: "OPEN" | "SOFT_CLOSED" | "HARD_CLOSED";
  createdAt: string;
}

export type PeriodeBukuListResponse = ListResponse<PeriodeBukuItem>;

export interface PeriodeBukuListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  /** Filter by status, e.g. "OPEN" */
  "filter[status]"?: string;
}

// ---------------------------------------------------------------------------
// API
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/periode-buku";

export const periodeBukuApi = {
  /**
   * Fetches all approved/open periods for use in dropdowns.
   * Default: returns all non-hard-closed periods sorted by periode_bulan desc.
   */
  list(params: PeriodeBukuListParams = {}): Promise<PeriodeBukuListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<PeriodeBukuListResponse>(`${BASE}${qs}`);
  },
};
