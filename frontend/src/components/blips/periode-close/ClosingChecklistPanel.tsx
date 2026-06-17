"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, CheckCircle2, XCircle, Loader2, AlertTriangle, ExternalLink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { periodeChecklistApi } from "@/lib/api/periode-close.api";
import type { ChecklistItem } from "@/lib/schemas/periode-close.schema";
import { CHECKLIST_ITEM_LABELS } from "@/lib/schemas/periode-close.schema";
import Link from "next/link";

// ---------------------------------------------------------------------------
// ChecklistItem sub-component
// ---------------------------------------------------------------------------

function ChecklistItemRow({ item }: { item: ChecklistItem }) {
  const label = CHECKLIST_ITEM_LABELS[item.key] ?? item.label;

  return (
    <div
      className={cn(
        "flex items-start gap-3 py-3 border-b border-border last:border-0",
      )}
    >
      <div className="mt-0.5 shrink-0">
        {item.passed ? (
          <CheckCircle2
            className="h-5 w-5 text-green-600"
            aria-label="Lolos"
          />
        ) : (
          <XCircle
            className="h-5 w-5 text-red-600"
            aria-label="Gagal"
          />
        )}
      </div>
      <div className="flex-1 min-w-0">
        <p
          className={cn(
            "text-sm font-medium",
            item.passed ? "text-green-700" : "text-red-700",
          )}
        >
          {item.passed ? "Lolos" : "Gagal"}
        </p>
        <p className="text-sm text-foreground mt-0.5">{label}</p>
        {item.detail && (
          <p className="text-xs text-muted-foreground mt-0.5">{item.detail}</p>
        )}
        {!item.passed && item.actionUrl && (
          <Link
            href={item.actionUrl}
            className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-800 mt-1 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 rounded"
          >
            <ExternalLink className="h-3 w-3" aria-hidden="true" />
            Tindak Lanjut
          </Link>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ClosingChecklistPanelProps {
  periodeId: string;
  statusPeriode: string;
  onAllPassed?: (allPassed: boolean) => void;
  onSnapshotClick?: (snapshotId: string) => void;
}

// ---------------------------------------------------------------------------
// Component (S5-AC1, S5-AC4)
// ---------------------------------------------------------------------------

export function ClosingChecklistPanel({
  periodeId,
  statusPeriode,
  onAllPassed,
  onSnapshotClick: _onSnapshotClick,
}: ClosingChecklistPanelProps) {
  const isClosed = statusPeriode === "CLOSED";
  const shouldPoll = !isClosed;

  const { data, isFetching, isError, refetch, dataUpdatedAt } = useQuery({
    queryKey: ["periode-buku", "checklist", periodeId],
    queryFn: () => periodeChecklistApi.get(periodeId),
    staleTime: shouldPoll ? 25_000 : Infinity,
    refetchInterval: shouldPoll ? 30_000 : false,
  });

  const checklist = data?.data;

  // Notify parent when allPassed changes
  React.useEffect(() => {
    if (checklist != null && onAllPassed) {
      onAllPassed(checklist.allPassed);
    }
  }, [checklist?.allPassed, onAllPassed, checklist]);

  const handleRefresh = () => {
    void refetch();
  };

  const lastUpdated = dataUpdatedAt
    ? new Date(dataUpdatedAt).toLocaleString("id-ID", {
        day: "2-digit",
        month: "short",
        hour: "2-digit",
        minute: "2-digit",
        timeZone: "Asia/Jakarta",
      })
    : null;

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-semibold">Closing Checklist</CardTitle>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleRefresh}
            disabled={isFetching}
            aria-label="Evaluasi ulang closing checklist"
            className="h-7 px-2 text-xs"
          >
            <RefreshCw
              className={cn("h-3.5 w-3.5 mr-1", isFetching && "animate-spin motion-reduce:animate-none")}
              aria-hidden="true"
            />
            {isFetching ? "Memuat..." : "Evaluasi Ulang"}
          </Button>
        </div>
        {lastUpdated && (
          <p className="text-xs text-muted-foreground">
            Terakhir dievaluasi: {lastUpdated}
          </p>
        )}
      </CardHeader>
      <CardContent className="pt-0">
        {/* Closed: historical data notice */}
        {isClosed && (
          <div className="mb-3 rounded-md bg-slate-50 border border-slate-200 px-3 py-2">
            <p className="text-xs text-muted-foreground">
              Data historis dari snapshot terakhir. Evaluasi real-time tidak tersedia untuk periode CLOSED.
            </p>
          </div>
        )}

        {/* Loading skeletons */}
        {isFetching && !checklist && (
          <div className="space-y-3" aria-label="Memuat checklist..." aria-busy="true">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="flex items-start gap-3 py-3 border-b last:border-0">
                <div className="mt-0.5 h-5 w-5 rounded-full animate-pulse bg-muted shrink-0" />
                <div className="flex-1 space-y-1">
                  <div className="h-3 w-16 rounded animate-pulse bg-muted" />
                  <div className="h-3 w-full rounded animate-pulse bg-muted" />
                  <div className="h-3 w-2/3 rounded animate-pulse bg-muted" />
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Error state */}
        {isError && !checklist && (
          <div className="flex flex-col items-center py-6 gap-3 text-center">
            <AlertTriangle className="h-6 w-6 text-destructive" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">
              Gagal memuat checklist.
            </p>
            <Button variant="outline" size="sm" onClick={handleRefresh}>
              Coba lagi
            </Button>
          </div>
        )}

        {/* Checklist items */}
        {checklist && (
          <>
            <div role="list" aria-label="Item closing checklist">
              {checklist.items.map((item) => (
                <ChecklistItemRow key={item.key} item={item} />
              ))}
            </div>

            {/* Summary */}
            <div
              className={cn(
                "mt-3 rounded-md px-3 py-2 text-xs font-medium flex items-center gap-2",
                checklist.allPassed
                  ? "bg-green-50 text-green-700 border border-green-200"
                  : "bg-red-50 text-red-700 border border-red-200",
              )}
              aria-live="polite"
            >
              {checklist.allPassed ? (
                <CheckCircle2 className="h-4 w-4" aria-hidden="true" />
              ) : (
                <XCircle className="h-4 w-4" aria-hidden="true" />
              )}
              {checklist.allPassed
                ? "Semua checklist lolos"
                : `${checklist.items.filter((i) => !i.passed).length} item checklist belum lolos`}
            </div>

            {/* Not real-time indicator */}
            {!checklist.isRealTimeEval && (
              <p className="text-xs text-muted-foreground mt-2">
                Data historis dari snapshot {checklist.lastSnapshot?.transition ?? "terakhir"}.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
