"use client";

import * as React from "react";
import { Suspense } from "react";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import Link from "next/link";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { PociDirectionBadge } from "@/components/blips/poci/PociDirectionBadge";
import { PociStatusBadge } from "@/components/blips/poci/PociStatusBadge";
import {
  pociDeltaLogApi,
  pociQueryKeys,
  type PociDeltaLogParams,
} from "@/lib/api/poci.api";
import {
  POCI_DIRECTION_LABELS,
  POCI_STATUS_LABELS,
  type PociDeltaLogItem,
  type PociDirection,
  type PociStatus,
} from "@/lib/schemas/poci.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function usePociDeltaLogFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_compute:desc"));
  const [filterCalcRunId, setFilterCalcRunId] = useQueryState(
    "filter[calc_run_id]",
    parseAsString.withDefault(""),
  );
  const [filterInstrumenId, setFilterInstrumenId] = useQueryState(
    "filter[instrumen_id]",
    parseAsString.withDefault(""),
  );
  const [filterPeriode, setFilterPeriode] = useQueryState(
    "filter[periode]",
    parseAsString.withDefault(""),
  );
  const [filterDirection, setFilterDirection] = useQueryState(
    "filter[direction]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[status]",
    parseAsString.withDefault(""),
  );
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ, sort, setSort,
    filterCalcRunId, setFilterCalcRunId,
    filterInstrumenId, setFilterInstrumenId,
    filterPeriode, setFilterPeriode,
    filterDirection, setFilterDirection,
    filterStatus, setFilterStatus,
    cursor, setCursor, limit,
  };
}

function PociDeltaLogContent() {
  const filters = usePociDeltaLogFilters();

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [sorting, setSorting] = React.useState<SortingState>([
    { id: "tanggal_compute", desc: true },
  ]);

  const listParams: PociDeltaLogParams = {
    limit: filters.limit,
    cursor: cursorHistory[pageIndex] || undefined,
    sort: filters.sort || undefined,
    q: filters.q || undefined,
    "filter[calc_run_id]": filters.filterCalcRunId || undefined,
    "filter[instrumen_id]": filters.filterInstrumenId || undefined,
    "filter[periode]": filters.filterPeriode || undefined,
    "filter[direction]": filters.filterDirection || undefined,
    "filter[status]": filters.filterStatus || undefined,
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: pociQueryKeys.deltaLogList(listParams),
    queryFn: () => pociDeltaLogApi.list(listParams),
    staleTime: 30_000,
  });

  const items = data?.data ?? [];
  const pagination = data?.pagination;

  React.useEffect(() => {
    const [s] = sorting;
    if (s) void filters.setSort(`${s.id}:${s.desc ? "desc" : "asc"}`);
  }, [sorting]);

  const activeFilters: ActiveFilter[] = [];
  if (filters.filterDirection)
    activeFilters.push({
      label: "Arah",
      value: POCI_DIRECTION_LABELS[filters.filterDirection as PociDirection] ?? filters.filterDirection,
      onRemove: () => void filters.setFilterDirection(""),
    });
  if (filters.filterStatus)
    activeFilters.push({
      label: "Status",
      value: POCI_STATUS_LABELS[filters.filterStatus as PociStatus] ?? filters.filterStatus,
      onRemove: () => void filters.setFilterStatus(""),
    });
  if (filters.filterPeriode)
    activeFilters.push({
      label: "Periode",
      value: filters.filterPeriode,
      onRemove: () => void filters.setFilterPeriode(""),
    });

  const columns: ColumnDef<PociDeltaLogItem>[] = [
    {
      id: "instrumenKode",
      header: ({ column }) => <SortHeader column={column} label="Kode Instrumen" />,
      accessorKey: "instrumenKode",
      cell: ({ row }) => (
        <Link
          href={`/poci/instrumen/${row.original.instrumenId}/history`}
          className="font-mono text-blue-600 hover:underline focus:outline-none focus:ring-2 focus:ring-blue-400 rounded"
        >
          {row.original.instrumenKode}
        </Link>
      ),
    },
    {
      id: "tanggalCompute",
      header: ({ column }) => <SortHeader column={column} label="Tgl Hitung" />,
      accessorKey: "tanggalCompute",
    },
    {
      id: "baselineEcl",
      header: "Baseline (IDR)",
      accessorKey: "baselineEcl",
      cell: ({ row }) => (
        <span className="font-mono text-right block text-xs">
          {IDR.format(parseFloat(row.original.baselineEcl))}
        </span>
      ),
    },
    {
      id: "currentEcl",
      header: "ECL Saat Ini (IDR)",
      accessorKey: "currentEcl",
      cell: ({ row }) => (
        <span className="font-mono text-right block text-xs">
          {IDR.format(parseFloat(row.original.currentEcl))}
        </span>
      ),
    },
    {
      id: "deltaEcl",
      header: ({ column }) => <SortHeader column={column} label="Delta (IDR)" />,
      accessorKey: "deltaEcl",
      cell: ({ row }) => {
        const v = parseFloat(row.original.deltaEcl);
        return (
          <span
            className={`font-mono text-right block text-xs font-semibold ${
              v > 0 ? "text-red-600" : v < 0 ? "text-green-600" : "text-slate-500"
            }`}
          >
            {v > 0 ? "+" : ""}{IDR.format(v)}
          </span>
        );
      },
    },
    {
      id: "direction",
      header: "Arah",
      accessorKey: "direction",
      cell: ({ row }) => (
        <PociDirectionBadge direction={row.original.direction} size="sm" />
      ),
    },
    {
      id: "status",
      header: "Status",
      accessorKey: "status",
      cell: ({ row }) => (
        <PociStatusBadge status={row.original.status} size="sm" />
      ),
    },
    {
      id: "largeDeltaFlag",
      header: "Large Delta",
      accessorKey: "largeDeltaFlag",
      cell: ({ row }) =>
        row.original.largeDeltaFlag ? (
          <span className="inline-flex items-center rounded-md bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 border border-red-300">
            LARGE
          </span>
        ) : null,
    },
  ];

  const handleExport = (format: "csv" | "xlsx") => {
    const url = pociDeltaLogApi.exportUrl({ ...listParams, format });
    window.open(url, "_blank");
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Delta Log POCI ECL</h1>
          <p className="text-sm text-muted-foreground">
            Riwayat komputasi delta ECL per instrumen POCI per calc run — APP-C P5-M10
          </p>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 items-center">
        <Select
          value={filters.filterDirection}
          onValueChange={(v) => void filters.setFilterDirection(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-44" aria-label="Filter arah delta">
            <SelectValue placeholder="Semua Arah" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Arah</SelectItem>
            {Object.entries(POCI_DIRECTION_LABELS).map(([v, l]) => (
              <SelectItem key={v} value={v}>{l}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.filterStatus}
          onValueChange={(v) => void filters.setFilterStatus(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-48" aria-label="Filter status delta">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Status</SelectItem>
            {Object.entries(POCI_STATUS_LABELS).map(([v, l]) => (
              <SelectItem key={v} value={v}>{l}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <DataTable
        columns={columns}
        data={items}
        isLoading={isLoading}
        isError={isError}
        onRetry={() => void refetch()}
        sorting={sorting}
        onSortingChange={setSorting}
        activeFilters={activeFilters}
        onClearAllFilters={() => {
          void filters.setFilterDirection("");
          void filters.setFilterStatus("");
          void filters.setFilterPeriode("");
          void filters.setFilterCalcRunId("");
          void filters.setFilterInstrumenId("");
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
        emptyMessage="Tidak ada delta log POCI yang cocok dengan filter."
      />
    </div>
  );
}

export default function PociDeltaLogPage() {
  return (
    <Suspense>
      <PociDeltaLogContent />
    </Suspense>
  );
}
