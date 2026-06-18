import { v4 as uuidv4 } from "uuid";
import {
  apiGet,
  apiPost,
  buildQueryString,
  type ListResponse,
  type SingleResponse,
} from "@/lib/api";
import type {
  GlDeliveryStatus,
  RetryGlDeliveryRequest,
  RetryGlDeliveryResponse,
  RunReconciliationRequest,
  RunReconciliationResponse,
  GlReconciliationReport,
  GlReconciliationSummaryItem,
  GlDeliveryDlqListItem,
  GlDeliveryDlqDetail,
  DlqReplayResponse,
  DlqDiscardResponse,
  DlqActionRequest,
} from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// Shared response types
// ---------------------------------------------------------------------------

export type GlDeliveryStatusResponse = SingleResponse<GlDeliveryStatus>;
export type RetryGlDeliveryApiResponse = SingleResponse<RetryGlDeliveryResponse>;
export type ReconciliationReportResponse = SingleResponse<GlReconciliationReport>;
export type RunReconciliationApiResponse = SingleResponse<RunReconciliationResponse>;
export type ReconciliationHistoryResponse = ListResponse<GlReconciliationSummaryItem>;
export type DlqListResponse = ListResponse<GlDeliveryDlqListItem>;
export type DlqDetailResponse = SingleResponse<GlDeliveryDlqDetail>;
export type DlqReplayApiResponse = SingleResponse<DlqReplayResponse>;
export type DlqDiscardApiResponse = SingleResponse<DlqDiscardResponse>;

// ---------------------------------------------------------------------------
// Query params
// ---------------------------------------------------------------------------

export interface ReconciliationHistoryParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[status]"?: string;
  "filter[tanggal_from]"?: string;
  "filter[tanggal_to]"?: string;
  export?: "csv" | "xlsx";
}

export interface DlqListParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  "filter[gl_host_status]"?: string;
  "filter[failure_category]"?: string;
  "filter[error_code]"?: string;
  export?: "csv" | "xlsx";
}

// ---------------------------------------------------------------------------
// Base API paths — mirrors backend routing from app-d-gl-delivery.yaml
// ---------------------------------------------------------------------------

const JURNAL_BASE = "/api/v1/jurnal";
const DLQ_GL_BASE = "/api/v1/jurnal/gl-delivery-dlq";
const RECON_BASE = "/api/v1/jurnal/reconciliation";

// ---------------------------------------------------------------------------
// S2 — GL Delivery Status
// ---------------------------------------------------------------------------

export const glDeliveryStatusApi = {
  /** GET /jurnal/header/{id}/gl-delivery-status */
  getStatus(jurnalHeaderId: string): Promise<GlDeliveryStatusResponse> {
    return apiGet<GlDeliveryStatusResponse>(
      `${JURNAL_BASE}/header/${jurnalHeaderId}/gl-delivery-status`,
    );
  },

  /** GET /jurnal/header/{id}/gl-delivery-history (audit trail) */
  getHistory(jurnalHeaderId: string): Promise<{ data: Array<{ eventTime: string; action: string; actorName: string; detail: string }>; meta: { traceId: string } }> {
    return apiGet(`${JURNAL_BASE}/header/${jurnalHeaderId}/gl-delivery-history`);
  },
};

// ---------------------------------------------------------------------------
// S3 — Manual Retry GL Delivery
// ---------------------------------------------------------------------------

export const glDeliveryRetryApi = {
  /**
   * POST /jurnal/header/{id}/retry-gl-delivery
   * Permission: jurnal.gl_delivery.retry
   * Idempotency-Key: required (auto-generated)
   */
  retry(
    jurnalHeaderId: string,
    body: RetryGlDeliveryRequest,
    idempotencyKey = uuidv4(),
  ): Promise<RetryGlDeliveryApiResponse> {
    return apiPost<RetryGlDeliveryApiResponse>(
      `${JURNAL_BASE}/header/${jurnalHeaderId}/retry-gl-delivery`,
      body,
      idempotencyKey,
    );
  },
};

// ---------------------------------------------------------------------------
// S4 — Reconciliation
// ---------------------------------------------------------------------------

export const glReconciliationApi = {
  /**
   * GET /jurnal/reconciliation/daily?date=YYYY-MM-DD
   * Permission: jurnal.reconciliation.read
   */
  getDaily(date: string): Promise<ReconciliationReportResponse> {
    return apiGet<ReconciliationReportResponse>(
      `${RECON_BASE}/daily?date=${encodeURIComponent(date)}`,
    );
  },

  /**
   * POST /jurnal/reconciliation/run
   * Permission: jurnal.reconciliation.run (ROLE-AKUN-CTL only)
   * Returns 202 + jobId for long-running progress (§3 UX pattern)
   */
  run(
    body: RunReconciliationRequest,
    idempotencyKey = uuidv4(),
  ): Promise<RunReconciliationApiResponse> {
    return apiPost<RunReconciliationApiResponse>(
      `${RECON_BASE}/run`,
      body,
      idempotencyKey,
    );
  },

  /**
   * GET /jurnal/reconciliation/history
   * Cursor-paginated list — sort + filter + export per §1 UX pattern
   */
  listHistory(params: ReconciliationHistoryParams = {}): Promise<ReconciliationHistoryResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<ReconciliationHistoryResponse>(`${RECON_BASE}/history${qs}`);
  },

  /** Export URL helper — respects active filters */
  exportUrl(params: ReconciliationHistoryParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${RECON_BASE}/history${qs}`;
  },
};

// ---------------------------------------------------------------------------
// S5 — GL Delivery DLQ Console
// ---------------------------------------------------------------------------

export const glDeliveryDlqApi = {
  /**
   * GET /jurnal/gl-delivery-dlq
   * Default filter: gl_host_status=FAILED
   * Permission: jurnal.gl_delivery.read
   */
  list(params: DlqListParams = {}): Promise<DlqListResponse> {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return apiGet<DlqListResponse>(`${DLQ_GL_BASE}${qs}`);
  },

  /**
   * GET /jurnal/gl-delivery-dlq/{id}
   * Permission: jurnal.gl_delivery.read
   * gl_response_payload_jsonb only for ROLE-IT-ADMIN (server-filtered)
   */
  get(id: string): Promise<DlqDetailResponse> {
    return apiGet<DlqDetailResponse>(`${DLQ_GL_BASE}/${id}`);
  },

  /**
   * POST /jurnal/gl-delivery-dlq/{id}/replay
   * Permission: jurnal.gl_delivery.replay
   * FAILED → PENDING_DELIVERY. Idempotency-Key required.
   */
  replay(
    id: string,
    body: DlqActionRequest,
    idempotencyKey = uuidv4(),
  ): Promise<DlqReplayApiResponse> {
    return apiPost<DlqReplayApiResponse>(
      `${DLQ_GL_BASE}/${id}/replay`,
      body,
      idempotencyKey,
    );
  },

  /**
   * POST /jurnal/gl-delivery-dlq/{id}/discard
   * Permission: jurnal.gl_delivery.discard (ROLE-IT-ADMIN ONLY)
   * FAILED → DEAD_LETTER (terminal). Idempotency-Key required.
   */
  discard(
    id: string,
    body: DlqActionRequest,
    idempotencyKey = uuidv4(),
  ): Promise<DlqDiscardApiResponse> {
    return apiPost<DlqDiscardApiResponse>(
      `${DLQ_GL_BASE}/${id}/discard`,
      body,
      idempotencyKey,
    );
  },

  /** Export URL helper */
  exportUrl(params: DlqListParams & { format: "csv" | "xlsx" }): string {
    const qs = buildQueryString(
      params as unknown as Record<string, string | number | boolean | null | undefined>,
    );
    return `${DLQ_GL_BASE}/export${qs}`;
  },
};
