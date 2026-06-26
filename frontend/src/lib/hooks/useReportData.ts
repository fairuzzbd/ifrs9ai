"use client";

/**
 * P5-M15 — TanStack Query hook for dashboard report widget data.
 * Default 5-minute polling; pauses when tab is hidden.
 */

import { useQuery, useQueryClient } from "@tanstack/react-query";
import * as React from "react";
import { dashboardReportApi, dashboardQueryKeys, type ReportQueryParams } from "@/lib/api/reports.api";
import type { DashboardReportSlug, ReportRowType } from "@/lib/schemas/dashboard.schema";
import type { ListResponse } from "@/lib/api";

const POLL_INTERVAL_MS = 300_000; // 5 minutes

export interface UseReportDataResult<S extends DashboardReportSlug> {
  data: ListResponse<ReportRowType<S>> | undefined;
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  lastUpdated: Date | undefined;
  refetch: () => void;
}

/**
 * Fetch a dashboard report slice with 5-minute polling.
 * Polling auto-pauses when document is hidden and resumes on visibility change.
 *
 * @param slug  - Report slug (rpt-01, rpt-13, etc.)
 * @param params - Query params forwarded to the API
 * @param intervalMs - Override polling interval (default: 300_000 = 5min)
 */
export function useReportData<S extends DashboardReportSlug>(
  slug: S,
  params: ReportQueryParams = {},
  intervalMs: number = POLL_INTERVAL_MS,
): UseReportDataResult<S> {
  const queryClient = useQueryClient();
  const [lastUpdated, setLastUpdated] = React.useState<Date | undefined>(
    undefined,
  );

  // Pause polling when tab is hidden
  const [isVisible, setIsVisible] = React.useState(true);

  React.useEffect(() => {
    const handler = () => {
      setIsVisible(document.visibilityState === "visible");
    };
    document.addEventListener("visibilitychange", handler);
    return () => document.removeEventListener("visibilitychange", handler);
  }, []);

  const queryKey = dashboardQueryKeys.report(slug, params);

  const query = useQuery({
    queryKey,
    queryFn: () => dashboardReportApi.list(slug, params),
    refetchInterval: isVisible ? intervalMs : false,
    staleTime: intervalMs,
    retry: 2,
  });

  // Track last successful update time
  React.useEffect(() => {
    if (query.isSuccess && query.data) {
      setLastUpdated(new Date());
    }
  }, [query.isSuccess, query.data]);

  const refetch = React.useCallback(() => {
    void queryClient.invalidateQueries({ queryKey });
  }, [queryClient, queryKey]);

  return {
    data: query.data as ListResponse<ReportRowType<S>> | undefined,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error as Error | null,
    lastUpdated,
    refetch,
  };
}

/**
 * Invalidate all dashboard report queries — used by the manual "Refresh" button.
 */
export function useInvalidateAllDashboardData() {
  const queryClient = useQueryClient();
  return React.useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.all });
  }, [queryClient]);
}
