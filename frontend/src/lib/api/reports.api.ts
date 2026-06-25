/**
 * P5-M15 — API client for dashboard report endpoints.
 * Wraps GET /api/v1/reports/{slug} with typed responses per slug.
 * Also exposes jobsListApi for GET /api/v1/jobs (list — new in M15).
 */

import {
  apiGet,
  buildQueryString,
  type ListResponse,
} from "@/lib/api";
import {
  REPORT_SLUG_SCHEMA,
  type DashboardReportSlug,
  type ReportRowType,
} from "@/lib/schemas/dashboard.schema";
import {
  jobListItemSchema,
  type JobListItem,
  type JobListParams,
} from "@/lib/schemas/jobs.schema";
import { z } from "zod";

const REPORTS = "/api/v1/reports";
const JOBS = "/api/v1/jobs";

// ---------------------------------------------------------------------------
// Report list params (shared across widgets)
// ---------------------------------------------------------------------------

export interface ReportQueryParams {
  cursor?: string | null;
  limit?: number;
  sort?: string;
  q?: string;
  [key: string]: string | number | boolean | null | undefined;
}

// ---------------------------------------------------------------------------
// dashboardReportApi — typed per slug
// ---------------------------------------------------------------------------

export const dashboardReportApi = {
  /**
   * Fetch a dashboard report slice.
   * Parses rows with the Zod schema for the slug, safe-parsing to avoid
   * throwing on unexpected fields from the backend.
   */
  async list<S extends DashboardReportSlug>(
    slug: S,
    params: ReportQueryParams = {},
  ): Promise<ListResponse<ReportRowType<S>>> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    const raw = await apiGet<ListResponse<unknown>>(`${REPORTS}/${slug}${qs}`);
    const schema = REPORT_SLUG_SCHEMA[slug] as z.ZodType<ReportRowType<S>>;
    const parsed = raw.data.map((row) => {
      const result = schema.safeParse(row);
      if (result.success) return result.data;
      // Soft-fail: return raw row cast to expected type (field may be extra/missing in dev)
      return row as ReportRowType<S>;
    });
    return { ...raw, data: parsed };
  },
};

// ---------------------------------------------------------------------------
// jobsListApi — GET /api/v1/jobs (list — new M15 endpoint)
// ---------------------------------------------------------------------------

export const jobsListApi = {
  async list(params: JobListParams = {}): Promise<ListResponse<JobListItem>> {
    const qs = buildQueryString(
      params as Record<string, string | number | boolean | null | undefined>,
    );
    const raw = await apiGet<ListResponse<unknown>>(`${JOBS}${qs}`);
    const parsed = raw.data.map((row) => {
      const result = jobListItemSchema.safeParse(row);
      return result.success ? result.data : (row as JobListItem);
    });
    return { ...raw, data: parsed };
  },
};

// ---------------------------------------------------------------------------
// TanStack Query key factories
// ---------------------------------------------------------------------------

export const dashboardQueryKeys = {
  all: ["dashboard"] as const,
  report: (slug: DashboardReportSlug, params: ReportQueryParams) =>
    [...dashboardQueryKeys.all, "report", slug, params] as const,
  jobsList: (params: JobListParams) =>
    [...dashboardQueryKeys.all, "jobs-list", params] as const,
} as const;
