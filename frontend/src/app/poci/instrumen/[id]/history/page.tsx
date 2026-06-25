"use client";

import * as React from "react";
import { Suspense } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { ArrowLeft } from "lucide-react";

import { Button } from "@/components/ui/button";
import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { PociDirectionBadge } from "@/components/blips/poci/PociDirectionBadge";
import { PociStatusBadge } from "@/components/blips/poci/PociStatusBadge";
import { PociDeltaHistoryChart } from "@/components/blips/poci/PociDeltaHistoryChart";
import {
  pociBaselineApi,
  pociDeltaHistoryApi,
  pociQueryKeys,
  type PociDeltaHistoryParams,
} from "@/lib/api/poci.api";
import type { PociDeltaHistoryItem } from "@/lib/schemas/poci.schema";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function PociInstrumenHistoryContent() {
  const { id } = useParams<{ id: string }>();

  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_compute:desc"));
  const [filterDirection, setFilterDirection] = useQueryState(
    "filter[direction]",
    parseAsString.withDefault(""),
  );
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [sorting, setSorting] = React.useState<SortingState>([
    { id: "tanggal_compute", desc: true },
  ]);

  const historyParams: PociDeltaHistoryParams = {
    instrumen_id: id,
    limit,
    cursor: cursorHistory[pageIndex] || undefined,
    sort: sort || undefined,
    "filter[direction]": filterDirection || undefined,
  };

  const { data: historyData, isLoading, isError, refetch } = useQuery({
    queryKey: pociQueryKeys.history(historyParams),
    queryFn: () => pociDeltaHistoryApi.list(historyParams),
    staleTime: 30_000,
    enabled: !!id,
  });

  const { data: baselineData } = useQuery({
    queryKey: pociQueryKeys.baselineDetail(id),
    queryFn: () => pociBaselineApi.get(id),
    staleTime: 300_000,
    enabled: !!id,
  });

  React.useEffect(() => {
    const [s] = sorting;
    if (s) void setSort(`${s.id}:${s.desc ? "desc" : "asc"}`);
  }, [sorting, setSort]);

  const items = historyData?.data ?? [];
  const pagination = historyData?.pagination;
  const baseline = baselineData?.data;

  const columns: ColumnDef<PociDeltaHistoryItem>[] = [
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
        <span className="font-mono text-xs text-right block">
          {IDR.format(parseFloat(row.original.baselineEcl))}
        </span>
      ),
    },
    {
      id: "currentEcl",
      header: "ECL Saat Ini (IDR)",
      accessorKey: "currentEcl",
      cell: ({ row }) => (
        <span className="font-mono text-xs text-right block">
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
            className={`font-mono text-xs text-right block font-semibold ${
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
      id: "priorDeltaCumulative",
      header: "Kumulatif Sebelum",
      accessorKey: "priorDeltaCumulative",
      cell: ({ row }) =>
        row.original.priorDeltaCumulative != null ? (
          <span className="font-mono text-xs">
            {IDR.format(parseFloat(row.original.priorDeltaCumulative))}
          </span>
        ) : (
          <span className="text-muted-foreground text-xs">—</span>
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
  ];

  const handleExport = (format: "csv" | "xlsx") => {
    const url = pociDeltaHistoryApi.exportUrl({ ...historyParams, format });
    window.open(url, "_blank");
  };

  return (
    <div className="container mx-auto py-6 space-y-6">
      <div className="flex items-center gap-3">
        <Link href="/poci/delta-log" aria-label="Kembali ke delta log">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
            Kembali
          </Button>
        </Link>
        <div>
          <h1 className="text-2xl font-bold">
            Riwayat Delta POCI
            {baseline && (
              <span className="ml-2 font-mono text-lg text-muted-foreground">
                — {baseline.instrumenKode}
              </span>
            )}
          </h1>
          <p className="text-sm text-muted-foreground">
            Kumulatif delta ECL sejak origination — APP-C P5-M10
          </p>
        </div>
      </div>

      {/* Baseline summary */}
      {baseline && (
        <div className="rounded-lg border bg-muted/20 p-4 grid grid-cols-3 gap-4 text-sm">
          <div>
            <p className="text-xs text-muted-foreground">Baseline ECL (Origination)</p>
            <p className="font-bold tabular-nums">
              {IDR.format(parseFloat(baseline.lifetimeEclAtOrigination))}
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Credit-Adjusted EIR</p>
            <p className="font-bold">
              {(parseFloat(baseline.creditAdjustedEir) * 100).toFixed(4)}%
            </p>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">Tgl Baseline</p>
            <p className="font-medium">{baseline.tanggalBaseline}</p>
          </div>
        </div>
      )}

      {/* Chart */}
      {items.length > 0 && (
        <div className="rounded-lg border p-4">
          <PociDeltaHistoryChart data={items} />
        </div>
      )}

      {/* Table */}
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
          void setFilterDirection("");
        }}
        searchValue=""
        onSearchChange={() => undefined}
        onExport={handleExport}
        exportFormats={["csv", "xlsx"]}
        pagination={{
          pageIndex,
          hasMore: pagination?.hasMore ?? false,
          totalEstimate: pagination?.totalEstimate ?? 0,
          limit,
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
        emptyMessage="Belum ada riwayat delta untuk instrumen ini."
      />
    </div>
  );
}

export default function PociInstrumenHistoryPage() {
  return (
    <Suspense>
      <PociInstrumenHistoryContent />
    </Suspense>
  );
}
