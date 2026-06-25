"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  useQueryState,
  parseAsString,
  parseAsInteger,
} from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { Suspense } from "react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { ExportButton } from "@/components/blips/ExportButton";
import { Button } from "@/components/ui/button";

import { jurnalQueryApi, type JurnalListParams } from "@/lib/api/jurnal.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { JurnalHeaderSummary } from "@/lib/schemas/jurnal.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function useJurnalFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_posting:desc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status_internal]", parseAsString.withDefault(""));
  const [filterPeriode, setFilterPeriode] = useQueryState("filter[periode_id]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));
  return { q, setQ, sort, setSort, filterStatus, setFilterStatus, filterPeriode, setFilterPeriode, cursor, setCursor, limit };
}

function JurnalHeaderListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useJurnalFilters();
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams: JurnalListParams = {
    limit: filters.limit,
    sort: filters.sort || undefined,
    q: filters.q || undefined,
    "filter[status_internal]": filters.filterStatus || undefined,
    "filter[periode_id]": filters.filterPeriode || undefined,
    cursor: filters.cursor || undefined,
  };

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["jurnal-header-list", queryParams],
    queryFn: () => jurnalQueryApi.list(queryParams),
    staleTime: 30_000,
  });

  const sortingState: SortingState = React.useMemo(() => {
    if (!filters.sort) return [];
    return filters.sort.split(",").map((part) => {
      const [id, dir] = part.split(":");
      return { id, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (newSorting: SortingState) => {
    void filters.setSort(
      newSorting.length === 0
        ? "tanggal_posting:desc"
        : newSorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","),
    );
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const toggleSort = (colId: string) => {
    const current = sortingState.find((s) => s.id === colId);
    if (!current) handleSortingChange([{ id: colId, desc: false }]);
    else if (!current.desc) handleSortingChange([{ id: colId, desc: true }]);
    else handleSortingChange([]);
  };

  const handleNextPage = () => {
    const nextCursor = data?.pagination?.nextCursor;
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

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterStatus) f.push({ key: "filter[status_internal]", label: "Status", value: filters.filterStatus, displayValue: filters.filterStatus });
    if (filters.filterPeriode) f.push({ key: "filter[periode_id]", label: "Periode", value: filters.filterPeriode, displayValue: filters.filterPeriode });
    return f;
  }, [filters.filterStatus, filters.filterPeriode]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[status_internal]") void filters.setFilterStatus("");
    if (key === "filter[periode_id]") void filters.setFilterPeriode("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterPeriode("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const columns: ColumnDef<JurnalHeaderSummary>[] = React.useMemo(
    () => [
      {
        accessorKey: "noJurnal",
        header: () => <SortHeader label="Nomor Jurnal" sortKey="no_jurnal" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <Link href={`/jurnal/header/${row.original.id}`} className="font-mono text-xs font-medium hover:underline">
            {row.original.noJurnal}
          </Link>
        ),
      },
      {
        accessorKey: "tanggalPosting",
        header: () => <SortHeader label="Tgl Jurnal" sortKey="tanggal_posting" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className="text-xs">
            {new Date(row.original.tanggalPosting).toLocaleDateString("id-ID")}
          </span>
        ),
      },
      {
        accessorKey: "eventCode",
        header: "Keterangan",
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground max-w-[180px] block truncate">
            {row.original.eventCode}
          </span>
        ),
      },
      {
        accessorKey: "totalDebit",
        header: () => <SortHeader label="Total Debit" sortKey="total_debit" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className="text-right block font-mono text-xs">
            {IDR.format(parseFloat(row.original.totalDebit || "0"))}
          </span>
        ),
      },
      {
        accessorKey: "totalKredit",
        header: () => <SortHeader label="Total Kredit" sortKey="total_kredit" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className="text-right block font-mono text-xs">
            {IDR.format(parseFloat(row.original.totalKredit || "0"))}
          </span>
        ),
      },
      {
        accessorKey: "statusInternal",
        header: () => <SortHeader label="Status" sortKey="status_internal" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => <WorkflowStatusBadge status={row.original.statusInternal} />,
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.push(`/jurnal/header/${row.original.id}`)}
            aria-label={`Lihat detail jurnal ${row.original.noJurnal}`}
          >
            Lihat
          </Button>
        ),
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState, perms],
  );

  const handleRefresh = () => {
    void refetch();
    setLastUpdated(new Date());
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        error={error instanceof Error ? error : null}
        sorting={sortingState}
        onSortingChange={handleSortingChange}
        searchValue={filters.q}
        onSearchChange={(v) => {
          void filters.setQ(v);
          void filters.setCursor("");
          setCursorHistory([""]);
          setPageIndex(0);
        }}
        searchPlaceholder="Cari nomor jurnal atau keterangan..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = jurnalQueryApi.exportUrl({ ...queryParams, format: fmt } as Parameters<typeof jurnalQueryApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada jurnal yang cocok dengan filter."
            : "Belum ada data jurnal."
        }
        onRetry={() => void refetch()}
      />
    </div>
  );
}

export function JurnalHeaderListPageClient() {
  return (
    <Suspense>
      <JurnalHeaderListContent />
    </Suspense>
  );
}
