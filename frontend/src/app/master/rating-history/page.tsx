"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { AlertTriangle, Plus } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ratingHistoryApi } from "@/lib/api/rating-history.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { RatingHistoryItem } from "@/lib/schemas/rating-history.schema";
import { ACTION_TYPE_LABELS, RATING_OUTLOOK_LABELS } from "@/lib/schemas/rating-history.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_berlaku:desc"));
  const [filterActionType, setFilterActionType] = useQueryState("filter[action_type]", parseAsString.withDefault(""));
  const [filterSicr, setFilterSicr] = useQueryState("filter[sicr_triggered]", parseAsString.withDefault(""));
  const [filterDefault, setFilterDefault] = useQueryState("filter[default_triggered]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));
  return { q, setQ, sort, setSort, filterActionType, setFilterActionType, filterSicr, setFilterSicr, filterDefault, setFilterDefault, cursor, setCursor, limit };
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
// Page
// ---------------------------------------------------------------------------

function RatingHistoryListContent() {
  const perms = usePermissions();
  const filters = useFilters();
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams = React.useMemo(() => {
    const p: Parameters<typeof ratingHistoryApi.list>[0] = { limit: filters.limit };
    if (filters.sort) p.sort = filters.sort;
    if (filters.q) p.q = filters.q;
    if (filters.filterActionType) p["filter[action_type]"] = filters.filterActionType;
    if (filters.filterSicr !== "") p["filter[sicr_triggered]"] = filters.filterSicr === "true";
    if (filters.filterDefault !== "") p["filter[default_triggered]"] = filters.filterDefault === "true";
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["rating-history", queryParams],
    queryFn: () => ratingHistoryApi.list(queryParams),
    staleTime: 30_000,
  });

  const sortingState: SortingState = React.useMemo(() => {
    if (!filters.sort) return [];
    return filters.sort.split(",").map((part) => {
      const [id, dir] = part.split(":");
      return { id, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const toggleSort = (colId: string) => {
    const current = sortingState.find((s) => s.id === colId);
    const next: SortingState = !current
      ? [{ id: colId, desc: false }]
      : !current.desc ? [{ id: colId, desc: true }] : [];
    void filters.setSort(next.length === 0 ? "tanggal_berlaku:desc" : next.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","));
    void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const handleNextPage = () => {
    const nextCursor = data?.pagination.nextCursor;
    if (nextCursor) {
      const newHistory = [...cursorHistory, nextCursor];
      setCursorHistory(newHistory); setPageIndex(newHistory.length - 1);
      void filters.setCursor(nextCursor);
    }
  };

  const handlePrevPage = () => {
    if (pageIndex > 0) {
      const newIndex = pageIndex - 1; setPageIndex(newIndex);
      void filters.setCursor(cursorHistory[newIndex] ?? "");
    }
  };

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterActionType) f.push({ key: "filter[action_type]", label: "Action", value: filters.filterActionType, displayValue: ACTION_TYPE_LABELS[filters.filterActionType as keyof typeof ACTION_TYPE_LABELS] ?? filters.filterActionType });
    if (filters.filterSicr !== "") f.push({ key: "filter[sicr_triggered]", label: "SICR", value: filters.filterSicr, displayValue: filters.filterSicr === "true" ? "Ya" : "Tidak" });
    if (filters.filterDefault !== "") f.push({ key: "filter[default_triggered]", label: "Default", value: filters.filterDefault, displayValue: filters.filterDefault === "true" ? "Ya" : "Tidak" });
    return f;
  }, [filters.filterActionType, filters.filterSicr, filters.filterDefault]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[action_type]") void filters.setFilterActionType("");
    if (key === "filter[sicr_triggered]") void filters.setFilterSicr("");
    if (key === "filter[default_triggered]") void filters.setFilterDefault("");
    void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterActionType(""); void filters.setFilterSicr(""); void filters.setFilterDefault("");
    void filters.setQ(""); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const columns: ColumnDef<RatingHistoryItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "counterpartyNama",
        header: () => <SortHeader label="Counterparty" sortKey="counterparty_nama" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <div>
            <Link href={`/master/counterparty/${row.original.counterpartyId}`} className="font-medium hover:underline">
              {row.original.counterpartyNama}
            </Link>
            <div className="font-mono text-xs text-muted-foreground">{row.original.counterpartyKode}</div>
          </div>
        ),
      },
      {
        accessorKey: "tanggalBerlaku",
        header: () => <SortHeader label="Tgl Berlaku" sortKey="tanggal_berlaku" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => <span className="text-sm">{row.original.tanggalBerlaku}</span>,
      },
      {
        accessorKey: "ratingPefindo",
        header: () => <SortHeader label="Rating" sortKey="rating_pefindo" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => <span className="font-mono font-bold text-sm">{row.original.ratingPefindo}</span>,
      },
      {
        accessorKey: "ratingOutlook",
        header: "Outlook",
        cell: ({ row }) => <span className="text-sm">{RATING_OUTLOOK_LABELS[row.original.ratingOutlook]}</span>,
      },
      {
        accessorKey: "actionType",
        header: "Action",
        cell: ({ row }) => {
          const color: Record<string, string> = { UPGRADE: "text-green-700", DOWNGRADE: "text-red-700", WITHDRAW: "text-muted-foreground" };
          return (
            <span className={cn("text-sm font-medium", color[row.original.actionType])}>
              {ACTION_TYPE_LABELS[row.original.actionType]}
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
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => (
          <Link href={`/master/rating-history/${row.original.id}`} className="text-sm text-primary hover:underline">
            Detail
          </Link>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState],
  );

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Rating History</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Rating History</h1>

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
        searchValue={filters.q}
        onSearchChange={(v) => { void filters.setQ(v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}
        searchPlaceholder="Cari counterparty, rating..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            <Select value={filters.filterActionType || "all"} onValueChange={(v) => { void filters.setFilterActionType(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}>
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter action type">
                <SelectValue placeholder="Semua Action" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Action</SelectItem>
                {Object.entries(ACTION_TYPE_LABELS).map(([val, label]) => (
                  <SelectItem key={val} value={val}>{label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select value={filters.filterSicr || "all"} onValueChange={(v) => { void filters.setFilterSicr(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}>
              <SelectTrigger className="h-9 w-[140px]" aria-label="Filter SICR triggered">
                <SelectValue placeholder="SICR" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua SICR</SelectItem>
                <SelectItem value="true">SICR: Ya</SelectItem>
                <SelectItem value="false">SICR: Tidak</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = ratingHistoryApi.exportUrl({
            ...queryParams,
            format: fmt,
          } as Parameters<typeof ratingHistoryApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={() => { void refetch(); setLastUpdated(new Date()); }}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("rating_history") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/rating-history/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Rating
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada rating history yang cocok dengan pencarian."
            : "Belum ada rating history."
        }
        onRetry={() => void refetch()}
      />
    </div>
  );
}

export default function RatingHistoryListPage() {
  return (
    <Suspense>
      <RatingHistoryListContent />
    </Suspense>
  );
}
