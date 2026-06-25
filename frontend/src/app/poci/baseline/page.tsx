"use client";

import * as React from "react";
import { Suspense } from "react";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { PociBaselineImmutableBadge } from "@/components/blips/poci/PociBaselineImmutableBadge";
import {
  pociBaselineApi,
  pociQueryKeys,
  type PociBaselineListParams,
} from "@/lib/api/poci.api";
import type { PociBaselineListItem } from "@/lib/schemas/poci.schema";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function usePociBaselineFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_baseline:desc"));
  const [filterInstrumenId, setFilterInstrumenId] = useQueryState(
    "filter[instrumen_id]",
    parseAsString.withDefault(""),
  );
  const [filterTanggal, setFilterTanggal] = useQueryState(
    "filter[tanggal_baseline]",
    parseAsString.withDefault(""),
  );
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ, sort, setSort,
    filterInstrumenId, setFilterInstrumenId,
    filterTanggal, setFilterTanggal,
    cursor, setCursor, limit,
  };
}

function PociBaselineContent() {
  const filters = usePociBaselineFilters();
  const { setSort } = filters;

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [sorting, setSorting] = React.useState<SortingState>([
    { id: "tanggal_baseline", desc: true },
  ]);

  const listParams: PociBaselineListParams = {
    limit: filters.limit,
    cursor: cursorHistory[pageIndex] || undefined,
    sort: filters.sort || undefined,
    q: filters.q || undefined,
    "filter[instrumen_id]": filters.filterInstrumenId || undefined,
    "filter[tanggal_baseline]": filters.filterTanggal || undefined,
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: pociQueryKeys.baselineList(listParams),
    queryFn: () => pociBaselineApi.list(listParams),
    staleTime: 30_000,
  });

  const items = data?.data ?? [];
  const pagination = data?.pagination;

  React.useEffect(() => {
    const [s] = sorting;
    if (s) void setSort(`${s.id}:${s.desc ? "desc" : "asc"}`);
  }, [sorting, setSort]);

  const columns: ColumnDef<PociBaselineListItem>[] = [
    {
      id: "instrumenKode",
      header: ({ column }) => <SortHeader column={column} label="Kode Instrumen" />,
      accessorKey: "instrumenKode",
      cell: ({ row }) => (
        <span className="font-mono text-sm">{row.original.instrumenKode}</span>
      ),
    },
    {
      id: "tanggalBaseline",
      header: ({ column }) => <SortHeader column={column} label="Tgl Baseline" />,
      accessorKey: "tanggalBaseline",
    },
    {
      id: "lifetimeEclAtOrigination",
      header: ({ column }) => <SortHeader column={column} label="ECL Baseline (IDR)" />,
      accessorKey: "lifetimeEclAtOrigination",
      cell: ({ row }) => (
        <span className="font-mono text-right block">
          {IDR.format(parseFloat(row.original.lifetimeEclAtOrigination))}
        </span>
      ),
    },
    {
      id: "creditAdjustedEir",
      header: "Credit-Adj EIR",
      accessorKey: "creditAdjustedEir",
      cell: ({ row }) => (
        <span className="font-mono text-sm">
          {(parseFloat(row.original.creditAdjustedEir) * 100).toFixed(4)}%
        </span>
      ),
    },
    {
      id: "immutable",
      header: "Status",
      cell: () => <PociBaselineImmutableBadge size="sm" />,
    },
  ];

  const handleExport = (format: "csv" | "xlsx") => {
    const url = pociBaselineApi.exportUrl({ ...listParams, format });
    window.open(url, "_blank");
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Baseline POCI ECL</h1>
          <p className="text-sm text-muted-foreground">
            Baseline lifetime ECL per instrumen POCI — immutable sejak origination (DEC-018)
          </p>
        </div>
      </div>

      <DataTable
        columns={columns}
        data={items}
        isLoading={isLoading}
        isError={isError}
        onRetry={() => void refetch()}
        sorting={sorting}
        onSortingChange={setSorting}
        activeFilters={[]}
        onClearAllFilters={() => {
          void filters.setFilterInstrumenId("");
          void filters.setFilterTanggal("");
          void filters.setQ("");
        }}
        searchValue={filters.q}
        onSearchChange={(v) => void filters.setQ(v)}
        onExport={handleExport}
        exportFormats={["csv", "xlsx"]}
        pagination={{
          pageIndex,
          hasMore: pagination?.hasMore ?? false,
          totalEstimate: pagination?.totalEstimate ?? 0,
          limit: filters.limit,
          onNext: () => {
            if (pagination?.nextCursor) {
              setCursorHistory((prev) => [...prev, pagination.nextCursor!]);
              setPageIndex((i) => i + 1);
            }
          },
          onPrev: () => {
            if (pageIndex > 0) {
              setCursorHistory((prev) => prev.slice(0, -1));
              setPageIndex((i) => i - 1);
            }
          },
        }}
        lastUpdated={lastUpdated}
        onRefresh={() => {
          void refetch();
          setLastUpdated(new Date());
        }}
        emptyMessage="Tidak ada baseline POCI yang cocok dengan filter."
      />
    </div>
  );
}

export default function PociBaselinePage() {
  return (
    <Suspense>
      <PociBaselineContent />
    </Suspense>
  );
}
