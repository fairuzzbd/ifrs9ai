"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { JatuhTempoStatusBadge } from "@/components/blips/akrual/JatuhTempoStatusBadge";
import { AkrualCronTriggerButton } from "@/components/blips/akrual/AkrualCronTriggerButton";
import { jatuhTempoListApi, jatuhTempoQueryKeys, type JatuhTempoListParams } from "@/lib/api/akrual.api";
import {
  JATUH_TEMPO_STATUS_LABELS,
  JATUH_TEMPO_JENIS_LABELS,
  type JatuhTempoListItem,
  type JatuhTempoStatus,
  type JatuhTempoJenis,
} from "@/lib/schemas/akrual.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

// ---------------------------------------------------------------------------
// IDR formatter
// ---------------------------------------------------------------------------

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

// ---------------------------------------------------------------------------
// URL filter state
// ---------------------------------------------------------------------------

function useJatuhTempoFilters() {
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_jatuh_tempo:desc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status]", parseAsString.withDefault(""));
  const [filterJenis, setFilterJenis] = useQueryState("filter[jenis]", parseAsString.withDefault(""));
  const [filterTanggal, setFilterTanggal] = useQueryState("filter[tanggal_jatuh_tempo]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return { sort, setSort, filterStatus, setFilterStatus, filterJenis, setFilterJenis, filterTanggal, setFilterTanggal, cursor, setCursor, limit };
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function JatuhTempoListContent() {
  const filters = useJatuhTempoFilters();
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const listParams: JatuhTempoListParams = {
    limit: filters.limit,
    cursor: cursorHistory[pageIndex] || undefined,
    sort: filters.sort || undefined,
    "filter[status]": filters.filterStatus || undefined,
    "filter[jenis]": filters.filterJenis || undefined,
    "filter[tanggal_jatuh_tempo]": filters.filterTanggal || undefined,
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: jatuhTempoQueryKeys.list(listParams),
    queryFn: () => jatuhTempoListApi.list(listParams),
    staleTime: 30_000,
  });

  const items = data?.data ?? [];
  const pagination = data?.pagination;
  const totalEstimate = pagination?.totalEstimate ?? 0;

  const [sorting, setSorting] = React.useState<SortingState>([{ id: "tanggal_jatuh_tempo", desc: true }]);

  React.useEffect(() => {
    const [s] = sorting;
    if (s) void filters.setSort(`${s.id}:${s.desc ? "desc" : "asc"}`);
  }, [sorting]);

  const activeFilters: ActiveFilter[] = [];
  if (filters.filterStatus)
    activeFilters.push({
      label: "Status",
      value: JATUH_TEMPO_STATUS_LABELS[filters.filterStatus as JatuhTempoStatus] ?? filters.filterStatus,
      onRemove: () => void filters.setFilterStatus(""),
    });
  if (filters.filterJenis)
    activeFilters.push({
      label: "Jenis",
      value: JATUH_TEMPO_JENIS_LABELS[filters.filterJenis as JatuhTempoJenis] ?? filters.filterJenis,
      onRemove: () => void filters.setFilterJenis(""),
    });

  const columns: ColumnDef<JatuhTempoListItem>[] = [
    {
      id: "instrumenKode",
      header: ({ column }) => <SortHeader column={column} label="Kode Instrumen" />,
      accessorKey: "instrumenKode",
      cell: ({ row }) => (
        <span className="font-mono text-sm">{row.original.instrumenKode}</span>
      ),
    },
    {
      id: "tanggalJatuhTempo",
      header: ({ column }) => <SortHeader column={column} label="Tgl Jatuh Tempo" />,
      accessorKey: "tanggalJatuhTempo",
    },
    {
      id: "jenis",
      header: "Jenis",
      accessorKey: "jenis",
      cell: ({ row }) => (
        <span className="text-xs font-medium">
          {JATUH_TEMPO_JENIS_LABELS[row.original.jenis]}
        </span>
      ),
    },
    {
      id: "pokokIdr",
      header: ({ column }) => <SortHeader column={column} label="Pokok (IDR)" />,
      accessorKey: "pokokIdr",
      cell: ({ row }) => (
        <span className="font-mono text-right block">
          {IDR.format(parseFloat(row.original.pokokIdr))}
        </span>
      ),
    },
    {
      id: "bungaLastIdr",
      header: "Bunga Last",
      accessorKey: "bungaLastIdr",
      cell: ({ row }) => (
        <span className="font-mono text-right block text-xs">
          {IDR.format(parseFloat(row.original.bungaLastIdr))}
        </span>
      ),
    },
    {
      id: "pphIdr",
      header: "PPh Dipotong",
      accessorKey: "pphIdr",
      cell: ({ row }) => (
        <span className="font-mono text-right block text-xs text-orange-600">
          {IDR.format(parseFloat(row.original.pphIdr))}
        </span>
      ),
    },
    {
      id: "netKasIdr",
      header: ({ column }) => <SortHeader column={column} label="Net Kas (IDR)" />,
      accessorKey: "netKasIdr",
      cell: ({ row }) => (
        <span className="font-mono text-right block font-semibold">
          {IDR.format(parseFloat(row.original.netKasIdr))}
        </span>
      ),
    },
    {
      id: "status",
      header: "Status",
      accessorKey: "status",
      cell: ({ row }) => <JatuhTempoStatusBadge status={row.original.status} size="sm" />,
    },
    {
      id: "jurnalHeaderId",
      header: "Jurnal",
      accessorKey: "jurnalHeaderId",
      cell: ({ row }) =>
        row.original.jurnalHeaderId ? (
          <Link
            href={`/transaksi/jurnal/${row.original.jurnalHeaderId}`}
            className="text-xs text-blue-600 hover:underline"
          >
            Lihat
          </Link>
        ) : null,
    },
  ];

  return (
    <div className="container mx-auto py-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Jatuh Tempo — Maturity Events</h1>
          <p className="text-sm text-muted-foreground">
            Settlement deposito + obligasi + reksadana jatuh tempo — APP-B P5-M9
          </p>
        </div>
        <div className="flex gap-2">
          <AkrualCronTriggerButton
            jobTypes={["DAILY_ACCRUAL_JOB"]}
            label="Trigger Maturity Cron"
            size="sm"
          />
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 items-center">
        <Select
          value={filters.filterStatus}
          onValueChange={(v) => void filters.setFilterStatus(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-44" aria-label="Filter status jatuh tempo">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Status</SelectItem>
            {Object.entries(JATUH_TEMPO_STATUS_LABELS).map(([v, l]) => (
              <SelectItem key={v} value={v}>{l}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.filterJenis}
          onValueChange={(v) => void filters.setFilterJenis(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-40" aria-label="Filter jenis instrumen">
            <SelectValue placeholder="Semua Jenis" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Jenis</SelectItem>
            {Object.entries(JATUH_TEMPO_JENIS_LABELS).map(([v, l]) => (
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
          void filters.setFilterStatus("");
          void filters.setFilterJenis("");
        }}
        exportFormats={["csv", "xlsx"]}
        onExport={(format) => {
          const base = "/api/v1/transaksi/jatuh-tempo/export";
          const params = new URLSearchParams();
          if (filters.filterStatus) params.set("filter[status]", filters.filterStatus);
          if (filters.filterJenis) params.set("filter[jenis]", filters.filterJenis);
          params.set("format", format);
          window.open(`${base}?${params.toString()}`, "_blank");
        }}
        pagination={{
          pageIndex,
          hasMore: pagination?.hasMore ?? false,
          totalEstimate,
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
        emptyMessage="Tidak ada maturity event yang cocok dengan filter."
      />
    </div>
  );
}

export default function JatuhTempoPage() {
  return (
    <Suspense>
      <JatuhTempoListContent />
    </Suspense>
  );
}
