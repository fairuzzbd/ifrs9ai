"use client";

import * as React from "react";
import { Suspense } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import Link from "next/link";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { AlertTriangle, Plus } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import { ratingHistoryApi } from "@/lib/api/rating-history.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { RatingHistoryItem } from "@/lib/schemas/rating-history.schema";
import { ACTION_TYPE_LABELS, RATING_OUTLOOK_LABELS } from "@/lib/schemas/rating-history.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useFilters() {
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_berlaku:desc"));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));
  return { sort, setSort, cursor, setCursor, limit };
}

// ---------------------------------------------------------------------------
// SICR / DEFAULT badge
// ---------------------------------------------------------------------------

function TriggerBadges({ item }: { item: RatingHistoryItem }) {
  return (
    <div className="flex flex-wrap gap-1">
      {item.sicrTriggered && (
        <span className="inline-flex items-center gap-1 rounded-full bg-red-100 px-2 py-0.5 text-xs font-medium text-red-700 border border-red-300">
          <AlertTriangle className="h-3 w-3" aria-hidden />
          SICR
        </span>
      )}
      {item.defaultTriggered && (
        <span className="inline-flex items-center gap-1 rounded-full bg-orange-100 px-2 py-0.5 text-xs font-medium text-orange-700 border border-orange-300">
          <AlertTriangle className="h-3 w-3" aria-hidden />
          DEFAULT
        </span>
      )}
      {!item.sicrTriggered && !item.defaultTriggered && (
        <span className="text-xs text-muted-foreground">—</span>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function RatingHistoryByCounterpartyContent() {
  const { id } = useParams<{ id: string }>();
  const perms = usePermissions();
  const filters = useFilters();

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams = React.useMemo(() => ({
    sort: filters.sort || undefined,
    cursor: filters.cursor || undefined,
    limit: filters.limit,
  }), [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["rating-history", "cp", id, queryParams],
    queryFn: () => ratingHistoryApi.listByCounterparty(id, queryParams),
    staleTime: 30_000,
    enabled: !!id,
  });

  const sortingState: SortingState = React.useMemo(() => {
    if (!filters.sort) return [];
    return filters.sort.split(",").map((part) => {
      const [sid, dir] = part.split(":");
      return { id: sid, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const toggleSort = (colId: string) => {
    const current = sortingState.find((s) => s.id === colId);
    const next: SortingState = !current
      ? [{ id: colId, desc: false }]
      : !current.desc
        ? [{ id: colId, desc: true }]
        : [];
    void filters.setSort(next.length === 0 ? "tanggal_berlaku:desc" : next.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","));
    void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const handleNextPage = () => {
    const nextCursor = data?.pagination.nextCursor;
    if (nextCursor) {
      const newHistory = [...cursorHistory, nextCursor];
      setCursorHistory(newHistory);
      setPageIndex(newHistory.length - 1);
      void filters.setCursor(nextCursor);
    }
  };

  const handlePrevPage = () => {
    if (pageIndex > 0) {
      const newIndex = pageIndex - 1;
      setPageIndex(newIndex);
      void filters.setCursor(cursorHistory[newIndex] ?? "");
    }
  };

  // Check if any row has SICR triggered — for banner
  const hasSicr = (data?.data ?? []).some((r) => r.sicrTriggered);
  const latestSicr = (data?.data ?? []).find((r) => r.sicrTriggered);

  const columns: ColumnDef<RatingHistoryItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "tanggalBerlaku",
        header: () => <SortHeader label="Tgl Berlaku" sortKey="tanggal_berlaku" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className="text-sm">{row.original.tanggalBerlaku}</span>
        ),
      },
      {
        accessorKey: "ratingPefindo",
        header: () => <SortHeader label="Rating" sortKey="rating_pefindo" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className="font-mono font-bold text-sm">{row.original.ratingPefindo}</span>
        ),
      },
      {
        accessorKey: "ratingOutlook",
        header: "Outlook",
        cell: ({ row }) => (
          <span className="text-sm">{RATING_OUTLOOK_LABELS[row.original.ratingOutlook] ?? row.original.ratingOutlook}</span>
        ),
      },
      {
        accessorKey: "actionType",
        header: "Action",
        cell: ({ row }) => {
          const color: Record<string, string> = {
            UPGRADE: "text-green-700",
            DOWNGRADE: "text-red-700",
            WITHDRAW: "text-muted-foreground",
          };
          return (
            <span className={cn("text-sm font-medium", color[row.original.actionType])}>
              {ACTION_TYPE_LABELS[row.original.actionType] ?? row.original.actionType}
            </span>
          );
        },
      },
      {
        accessorKey: "notchChange",
        header: "Notch",
        cell: ({ row }) => {
          const n = row.original.notchChange;
          return (
            <span className={cn("font-mono text-sm", n > 0 ? "text-green-700" : n < 0 ? "text-red-700" : "text-muted-foreground")}>
              {n > 0 ? `+${n}` : n}
            </span>
          );
        },
      },
      {
        id: "triggers",
        header: "Trigger",
        cell: ({ row }) => <TriggerBadges item={row.original} />,
      },
      {
        accessorKey: "tanggalBerakhir",
        header: "Tgl Berakhir",
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {row.original.tanggalBerakhir ?? "—"}
          </span>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState],
  );

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/counterparty" className="hover:underline">Counterparty</Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/counterparty/${id}`} className="hover:underline">{id}</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Rating History</span>
      </nav>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Rating History</h1>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/master/counterparty/${id}`}>&larr; Kembali ke Detail</Link>
        </Button>
      </div>

      {/* SICR banner */}
      {hasSicr && latestSicr && (
        <div className="flex items-start gap-3 rounded-lg border border-amber-400 bg-amber-50 px-4 py-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-700" aria-hidden />
          <p className="text-sm text-amber-800">
            <strong>SICR Terdeteksi</strong> — Counterparty ini mengalami Significant Increase in Credit Risk
            pada <strong>{latestSicr.tanggalBerlaku}</strong>. Counterparty dipindah ke Stage 2 ECL.
          </p>
        </div>
      )}

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        error={error instanceof Error ? error : null}
        sorting={sortingState}
        onSortingChange={(s) => {
          void filters.setSort(s.length === 0 ? "tanggal_berlaku:desc" : s.map((x) => `${x.id}:${x.desc ? "desc" : "asc"}`).join(","));
          void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
        }}
        searchPlaceholder="Cari rating..."
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onRefresh={() => { void refetch(); setLastUpdated(new Date()); }}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("rating_history") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href={`/master/rating-history/new?counterpartyId=${id}`}>
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Rating
              </Link>
            </Button>
          ) : null
        }
        emptyMessage="Belum ada riwayat rating untuk counterparty ini."
        onRetry={() => void refetch()}
      />
    </div>
  );
}

export default function RatingHistoryByCounterpartyPage() {
  return (
    <Suspense>
      <RatingHistoryByCounterpartyContent />
    </Suspense>
  );
}
