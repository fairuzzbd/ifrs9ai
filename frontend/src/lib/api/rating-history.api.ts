import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  apiPut,
  apiDelete,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  RatingHistoryItem,
  RatingHistoryCreateInput,
  RatingHistoryUpdateInput,
} from "@/lib/schemas/rating-history.schema";
import type { MataUangHistoryResponse } from "@/lib/api/mata-uang.api";

// ---------------------------------------------------------------------------
// Query params
// ---------------------------------------------------------------------------

export interface RatingHistoryListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[counterparty_id]"?: string;
  "filter[rating_pefindo]"?: string;
  "filter[action_type]"?: string;
  "filter[sicr_triggered]"?: boolean;
  "filter[default_triggered]"?: boolean;
  "filter[tanggal_berlaku]"?: string;
  include_deleted?: boolean;
}

export type RatingHistoryListResponse = ListResponse<RatingHistoryItem>;
export type RatingHistoryDetailResponse = SingleResponse<RatingHistoryItem>;

// ---------------------------------------------------------------------------
// API functions — global list  (/api/v1/master/rating-history)
// ---------------------------------------------------------------------------

const BASE = "/api/v1/master/rating-history";
const NESTED_BASE = (cpId: string) =>
  `/api/v1/master/counterparty/${cpId}/rating-history`;

export const ratingHistoryApi = {
  /** Global list across all counterparties */
  list(params: RatingHistoryListParams = {}): Promise<RatingHistoryListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<RatingHistoryListResponse>(`${BASE}${qs}`);
  },

  /** Nested list filtered by counterparty */
  listByCounterparty(
    cpId: string,
    params: Omit<RatingHistoryListParams, "filter[counterparty_id]"> = {},
  ): Promise<RatingHistoryListResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<RatingHistoryListResponse>(`${NESTED_BASE(cpId)}${qs}`);
  },

  get(id: string): Promise<RatingHistoryDetailResponse> {
    return apiGet<RatingHistoryDetailResponse>(`${BASE}/${id}`);
  },

  create(
    data: RatingHistoryCreateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<RatingHistoryItem>> {
    return apiPost<SingleResponse<RatingHistoryItem>>(BASE, data, idempotencyKey);
  },

  update(
    id: string,
    data: RatingHistoryUpdateInput,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<RatingHistoryItem>> {
    return apiPut<SingleResponse<RatingHistoryItem>>(
      `${BASE}/${id}`,
      data,
      idempotencyKey,
    );
  },

  delete(
    id: string,
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<{ deleted: boolean; deletedAt: string; entityId: string }>> {
    return apiDelete(`${BASE}/${id}`, idempotencyKey);
  },

  getHistory(
    id: string,
    params: { cursor?: string; limit?: number } = {},
  ): Promise<MataUangHistoryResponse> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<MataUangHistoryResponse>(`${BASE}/${id}/history${qs}`);
  },

  exportUrl(params: RatingHistoryListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE}/export${qs}`;
  },
};
