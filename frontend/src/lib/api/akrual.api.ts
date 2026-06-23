/**
 * API client — P5-M9 Jatuh Tempo + Pendapatan Akrual Harian
 * Mirrors api/openapi/app-b-jatuh-tempo-akrual.yaml
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
  AkrualListItem,
  AkrualDetail,
  AkrualDashboard,
  OverrideStaleInput,
  OverrideStaleResponse,
  JatuhTempoListItem,
} from "@/lib/schemas/akrual.schema";

// ---------------------------------------------------------------------------
// Base paths
// ---------------------------------------------------------------------------

const BASE_AKRUAL = "/api/v1/transaksi/akrual";
const BASE_JT = "/api/v1/transaksi/jatuh-tempo";

// ---------------------------------------------------------------------------
// Param types
// ---------------------------------------------------------------------------

export interface AkrualListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[instrumen_id]"?: string;
  "filter[tanggal_akrual]"?: string;
  "filter[stage]"?: number | string;
  "filter[status]"?: string;
  "filter[jenis]"?: string;
  has_stale_flag?: boolean;
  periode_bulanan_id?: string;
}

export interface AkrualDashboardParams {
  instrumen_id?: string;
  portofolio_id?: string;
  year: number;
  month: number;
}

export interface JatuhTempoListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  "filter[instrumen_id]"?: string;
  "filter[status]"?: string;
  "filter[jenis]"?: string;
  "filter[tanggal_jatuh_tempo]"?: string;
}

// ---------------------------------------------------------------------------
// Response aliases
// ---------------------------------------------------------------------------

export type AkrualListApiResponse = ListResponse<AkrualListItem> & { staleCount?: number };
export type AkrualDetailApiResponse = SingleResponse<AkrualDetail>;
export type AkrualDashboardApiResponse = SingleResponse<AkrualDashboard>;
export type OverrideStaleApiResponse = SingleResponse<OverrideStaleResponse>;
export type JatuhTempoListApiResponse = ListResponse<JatuhTempoListItem>;

export interface CronTriggerResponse {
  jobId: string;
  type: string;
  statusUrl: string;
  streamUrl: string;
  jobIds?: string[];
}

// ---------------------------------------------------------------------------
// akrualListApi — GET /transaksi/akrual (S5-AC1)
// ---------------------------------------------------------------------------

export const akrualListApi = {
  list(params: AkrualListParams = {}): Promise<AkrualListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<AkrualListApiResponse>(`${BASE_AKRUAL}${qs}`);
  },

  exportUrl(params: AkrualListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${BASE_AKRUAL}/export${qs}`;
  },
};

// ---------------------------------------------------------------------------
// akrualDetailApi — GET /transaksi/akrual/{id}
// ---------------------------------------------------------------------------

export const akrualDetailApi = {
  get(id: string): Promise<AkrualDetailApiResponse> {
    return apiGet<AkrualDetailApiResponse>(`${BASE_AKRUAL}/${id}`);
  },
};

// ---------------------------------------------------------------------------
// akrualDashboardApi — GET /transaksi/akrual/dashboard (S5-AC2)
// ---------------------------------------------------------------------------

export const akrualDashboardApi = {
  get(params: AkrualDashboardParams): Promise<AkrualDashboardApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<AkrualDashboardApiResponse>(`${BASE_AKRUAL}/dashboard${qs}`);
  },
};

// ---------------------------------------------------------------------------
// akrualOverrideApi — POST /transaksi/akrual/{id}/override-stale (S5-AC4)
// Permission: akrual.override_stale (ROLE-AKUN-CTL)
// ---------------------------------------------------------------------------

export const akrualOverrideApi = {
  overrideStale(
    id: string,
    body: OverrideStaleInput,
    idempotencyKey = uuidv4(),
  ): Promise<OverrideStaleApiResponse> {
    return apiPost<OverrideStaleApiResponse>(
      `${BASE_AKRUAL}/${id}/override-stale`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// akrualCronApi — POST /transaksi/akrual/cron-trigger (ROLE-IT-ADMIN)
// ---------------------------------------------------------------------------

export const akrualCronApi = {
  trigger(
    body: { tanggal?: string; jobTypes?: ("DAILY_ACCRUAL_JOB" | "AMORTISASI_PD_JOB")[] },
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<CronTriggerResponse>> {
    return apiPost<SingleResponse<CronTriggerResponse>>(
      `${BASE_AKRUAL}/cron-trigger`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// jatuhTempoListApi — GET /transaksi/jatuh-tempo (S1)
// ---------------------------------------------------------------------------

export const jatuhTempoListApi = {
  list(params: JatuhTempoListParams = {}): Promise<JatuhTempoListApiResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<JatuhTempoListApiResponse>(`${BASE_JT}${qs}`);
  },
};

// ---------------------------------------------------------------------------
// jatuhTempoCronApi — POST /transaksi/jatuh-tempo/cron-trigger (ROLE-IT-ADMIN)
// ---------------------------------------------------------------------------

export const jatuhTempoCronApi = {
  trigger(
    body: { tanggal?: string },
    idempotencyKey = uuidv4(),
  ): Promise<SingleResponse<CronTriggerResponse>> {
    return apiPost<SingleResponse<CronTriggerResponse>>(
      `${BASE_JT}/cron-trigger`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const akrualQueryKeys = {
  all: ["akrual"] as const,

  lists: () => [...akrualQueryKeys.all, "list"] as const,
  list: (params: AkrualListParams) => [...akrualQueryKeys.lists(), params] as const,

  details: () => [...akrualQueryKeys.all, "detail"] as const,
  detail: (id: string) => [...akrualQueryKeys.details(), id] as const,

  dashboards: () => [...akrualQueryKeys.all, "dashboard"] as const,
  dashboard: (params: AkrualDashboardParams) => [...akrualQueryKeys.dashboards(), params] as const,
} as const;

export const jatuhTempoQueryKeys = {
  all: ["jatuh-tempo"] as const,
  lists: () => [...jatuhTempoQueryKeys.all, "list"] as const,
  list: (params: JatuhTempoListParams) => [...jatuhTempoQueryKeys.lists(), params] as const,
} as const;
