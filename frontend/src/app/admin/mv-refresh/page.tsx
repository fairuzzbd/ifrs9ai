/**
 * /admin/mv-refresh — Materialized View refresh dashboard + history.
 * ROLE-IT-ADMIN: trigger refresh, view status, view last refresh time.
 * ROLE-AUDIT: read-only view.
 * Others: 403 redirect (enforced server-side; client persona gates absent-from-DOM buttons).
 */

"use client";

import * as React from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { RefreshCw, Database } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { MVStatusBadge } from "@/components/blips/reporting/MVStatusBadge";
import { MVRefreshButton } from "@/components/blips/reporting/MVRefreshButton";
import { MVRefreshProgressPanel } from "@/components/blips/reporting/MVRefreshProgressPanel";
import { notify } from "@/lib/notify";
import { mvApi, reportingQueryKeys } from "@/lib/api/reporting.api";
import type { AsyncJobRef } from "@/lib/schemas/reporting.schema";
import { formatInTimeZone } from "date-fns-tz";

// ---------------------------------------------------------------------------
// Persona gate helper (client-side only — server also enforces)
// ---------------------------------------------------------------------------

function useCurrentUserRoles(): string[] {
  // In production this comes from the session/JWT. Stub from localStorage for now.
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem("blips_roles");
    return raw ? (JSON.parse(raw) as string[]) : [];
  } catch {
    return [];
  }
}

function formatRefreshAt(iso: string | null | undefined): string {
  if (!iso) return "—";
  return formatInTimeZone(new Date(iso), "Asia/Jakarta", "dd MMM yyyy HH:mm:ss");
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function MVRefreshPage() {
  const queryClient = useQueryClient();
  const roles = useCurrentUserRoles();
  const isITAdmin = roles.includes("ROLE-IT-ADMIN");

  const [activeJob, setActiveJob] = React.useState<(AsyncJobRef & { mvName?: string | null }) | null>(
    null,
  );

  const { data, isLoading, error } = useQuery({
    queryKey: reportingQueryKeys.mvList(),
    queryFn: () => mvApi.list({ limit: 20, sort: "mvName:asc" }),
    refetchInterval: activeJob ? 5000 : false,
  });

  const refreshMutation = useMutation({
    mutationFn: ({ mvName, ik }: { mvName?: string | null; ik: string }) =>
      mvApi.triggerRefresh({ mvName }, ik),
    onSuccess: (res, vars) => {
      setActiveJob({ ...res.data, mvName: vars.mvName });
    },
    onError: (err) => {
      notify.error(err as Parameters<typeof notify.error>[0]);
    },
  });

  const handleRefresh = (mvName?: string | null) => {
    refreshMutation.mutate({ mvName, ik: uuidv4() });
  };

  const handleJobDone = React.useCallback(() => {
    setActiveJob(null);
    void queryClient.invalidateQueries({ queryKey: reportingQueryKeys.mvList() });
  }, [queryClient]);

  const mvItems = data?.data ?? [];

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Database className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
          <h1 className="text-xl font-semibold">Materialized View — Dashboard Refresh</h1>
        </div>
        <MVRefreshButton
          mvName={null}
          isITAdmin={isITAdmin}
          loading={refreshMutation.isPending}
          onClick={handleRefresh}
        />
      </div>

      <Separator />

      {/* Active job progress */}
      {activeJob && (
        <MVRefreshProgressPanel
          jobId={activeJob.jobId}
          mvName={activeJob.mvName}
          onDone={handleJobDone}
        />
      )}

      {/* MV status grid */}
      {isLoading && (
        <div className="text-sm text-muted-foreground p-4">Memuat status MV...</div>
      )}
      {error && (
        <div role="alert" className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-800">
          Gagal memuat status MV. Coba muat ulang halaman.
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {mvItems.map((mv) => (
          <Card key={mv.mvName} className="relative">
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium truncate" title={mv.mvName}>
                {mv.mvName.replace("rpt.mv_", "").replace(/_/g, " ")}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              <MVStatusBadge status={mv.status} />

              <dl className="space-y-1 text-xs text-muted-foreground">
                <div className="flex justify-between">
                  <dt>Baris</dt>
                  <dd className="font-mono">
                    {mv.rowCount != null ? mv.rowCount.toLocaleString("id-ID") : "—"}
                  </dd>
                </div>
                <div className="flex justify-between">
                  <dt>Refresh terakhir</dt>
                  <dd>{formatRefreshAt(mv.lastRefreshAt)}</dd>
                </div>
                {mv.triggeredBy && (
                  <div className="flex justify-between">
                    <dt>Oleh</dt>
                    <dd>{mv.triggeredBy}</dd>
                  </div>
                )}
              </dl>

              {mv.lastError && (
                <p className="text-xs text-red-600 break-all" role="alert">
                  {mv.lastError}
                </p>
              )}

              <MVRefreshButton
                mvName={mv.mvName}
                isITAdmin={isITAdmin}
                loading={refreshMutation.isPending}
                onClick={handleRefresh}
                className="w-full mt-1"
              />
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
