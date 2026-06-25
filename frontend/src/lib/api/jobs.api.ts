/**
 * P5-M15 — Job API client (list + single + cancel + stream).
 * Re-exports jobApi from reporting.api (M13) and adds jobsListApi.
 * Import this file for all job-related API calls in dashboard modules.
 */

export { jobApi } from "@/lib/api/reporting.api";
export { jobsListApi, dashboardQueryKeys } from "@/lib/api/reports.api";
export type { JobListParams } from "@/lib/schemas/jobs.schema";
