"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger, parseAsBoolean } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { LayoutDashboard } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { AkrualStatusBadge } from "@/components/blips/akrual/AkrualStatusBadge";
import { AkrualStageBadge } from "@/components/blips/akrual/AkrualStageBadge";
import { AkrualJenisBadge } from "@/components/blips/akrual/AkrualJenisBadge";
import { StaleStagingBadge } from "@/components/blips/akrual/StaleStagingBadge";
import { AkrualCronTriggerButton } from "@/components/blips/akrual/AkrualCronTriggerButton";
import { AkrualOverrideStaleDialog } from "@/components/blips/akrual/AkrualOverrideStaleDialog";

import {
  akrualListApi,
  akrualQueryKeys,
  type AkrualListParams,
} from "@/lib/api/akrual.api";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  AKRUAL_STATUS_LABELS,
  AKRUAL_JENIS_LABELS,
  type AkrualListItem,
  type AkrualStatus,
  type AkrualJenis,
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

function useAkrualFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_akrual:desc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status]", parseAsString.withDefault(""));
  const [filterJenis, setFilterJenis] = useQueryState("filter[jenis]", parseAsString.withDefault(""));
  const [filterStage, setFilterStage] = useQueryState("filter[stage]", parseAsString.withDefault(""));
  const [filterTanggal, setFilterTanggal] = useQueryState("filter[tanggal_akrual]", parseAsString.withDefault(""));
  const [hasStaleFlag, setHasStaleFlag] = useQueryState("has_stale_flag", parseAsBoolean.withDefault(false));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ, sort, setSort,
    filterStatus, setFilterStatus,
    filterJenis, setFilterJenis,
    filterStage, setFilterStage,
    filterTanggal, setFilterTanggal,
    hasStaleFlag, setHasStaleFlag,
    cursor, setCursor, limit,
  };
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function AkrualListContent() {
  const perms = usePermissions();
  const queryClient = useQueryClient();
  const filters = useAkrualFilters();
  const { setSort } = filters;

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [overrideTarget, setOverrideTarget] = React.useState<AkrualListItem | null>(null);

  const listParams: AkrualListParams = {
    limit: filters.limit,
    cursor: cursorHistory[pageIndex] || undefined,
    sort: filters.sort || undefined,
    q: filters.q || undefined,
    "filter[status]": filters.filterStatus || undefined,
    "filter[jenis]": filters.filterJenis || undefined,
    "filter[stage]": filters.filterStage ? Number(filters.filterStage) : undefined,
    "filter[tanggal_akrual]": filters.filterTanggal || undefined,
    has_stale_flag: filters.hasStaleFlag || undefined,
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: akrualQueryKeys.list(listParams),
    queryFn: () => akrualListApi.list(listParams),
    staleTime: 30_000,
  });

  const items = data?.data ?? [];
  const pagination = data?.pagination;
  const staleCount = data?.staleCount ?? 0;
  const totalEstimate = pagination?.totalEstimate ?? 0;

  const [sorting, setSorting] = React.useState<SortingState>([{ id: "tanggal_akrual", desc: true }]);

  React.useEffect(() => {
    const [s] = sorting;
    if (s) {
      void setSort(`${s.id}:${s.desc ? "desc" : "asc"}`);
    }
  }, [sorting, setSort]);

  const activeFilters: ActiveFilter[] = [];
  if (filters.filterStatus)
    activeFilters.push({
      label: "Status",
      value: AKRUAL_STATUS_LABELS[filters.filterStatus as AkrualStatus] ?? filters.filterStatus,
      onRemove: () => void filters.setFilterStatus(""),
    });
  if (filters.filterJenis)
    activeFilters.push({
      label: "Jenis",
      value: AKRUAL_JENIS_LABELS[filters.filterJenis as AkrualJenis] ?? filters.filterJenis,
      onRemove: () => void filters.setFilterJenis(""),
    });
  if (filters.filterStage)
    activeFilters.push({
      label: "Stage",
      value: `Stage ${filters.filterStage}`,
      onRemove: () => void filters.setFilterStage(""),
    });
  if (filters.hasStaleFlag)
    activeFilters.push({
      label: "Stale",
      value: "Hanya Staging Stale",
      onRemove: () => void filters.setHasStaleFlag(false),
    });

  const columns: ColumnDef<AkrualListItem>[] = [
    {
      id: "instrumenKode",
      header: ({ column }) => <SortHeader column={column} label="Kode Instrumen" />,
      accessorKey: "instrumenKode",
      cell: ({ row }) => (
        <Link
          href={`/transaksi/akrual/${row.original.id}`}
          className="font-mono text-blue-600 hover:underline focus:outline-none focus:ring-2 focus:ring-blue-400 rounded"
        >
          {row.original.instrumenKode}
        </Link>
      ),
    },
    {
      id: "tanggalAkrual",
      header: ({ column }) => <SortHeader column={column} label="Tgl Akrual" />,
      accessorKey: "tanggalAkrual",
    },
    {
      id: "jenis",
      header: "Jenis",
      accessorKey: "jenis",
      cell: ({ row }) => <AkrualJenisBadge jenis={row.original.jenis} size="sm" />,
    },
    {
      id: "stage",
      header: "Stage",
      accessorKey: "stage",
      cell: ({ row }) => (
        <AkrualStageBadge
          stage={row.original.stage as 1 | 2 | 3 | null}
          netCarryingIdr={
            row.original.carryingBasis === "NET_CARRYING" ? row.original.carryingIdr : null
          }
          size="sm"
        />
      ),
    },
    {
      id: "carryingBasis",
      header: "Basis",
      accessorKey: "carryingBasis",
      cell: ({ row }) => (
        <span className="font-mono text-xs">
          {row.original.carryingBasis === "NET_CARRYING" ? (
            <span className="text-red-600 font-semibold">NET</span>
          ) : (
            "GROSS"
          )}
        </span>
      ),
    },
    {
      id: "bungaBersih",
      header: ({ column }) => <SortHeader column={column} label="Akrual (IDR)" />,
      accessorKey: "bungaBersih",
      cell: ({ row }) => (
        <span className="font-mono text-right block">
          {IDR.format(parseFloat(row.original.bungaBersih))}
        </span>
      ),
    },
    {
      id: "staleStagingFlag",
      header: "Stale",
      accessorKey: "staleStagingFlag",
      cell: ({ row }) =>
        row.original.staleStagingFlag ? <StaleStagingBadge size="sm" /> : null,
    },
    {
      id: "status",
      header: "Status",
      accessorKey: "status",
      cell: ({ row }) => <AkrualStatusBadge status={row.original.status} size="sm" />,
    },
    {
      id: "actions",
      header: "Aksi",
      cell: ({ row }) => {
        const item = row.original;
        const canOverride =
          perms.can("akrual.override_stale") && item.status === "PENDING_STALE_REVIEW";
        if (!canOverride) return null;
        return (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setOverrideTarget(item)}
            aria-label={`Override staging stale ${item.instrumenKode}`}
          >
            Override
          </Button>
        );
      },
    },
  ];

  const handleExport = (format: "csv" | "xlsx") => {
    const url = akrualListApi.exportUrl({ ...listParams, format });
    window.open(url, "_blank");
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Pendapatan Akrual Harian</h1>
          <p className="text-sm text-muted-foreground">
            Akrual EIR, amortisasi premium/diskon, dividen — APP-B P5-M9
          </p>
        </div>
        <div className="flex gap-2">
          <Link href="/transaksi/akrual/dashboard">
            <Button variant="outline" size="sm" aria-label="Lihat dashboard MTD/YTD akrual">
              <LayoutDashboard className="h-4 w-4 mr-1" aria-hidden="true" />
              Dashboard
            </Button>
          </Link>
          <AkrualCronTriggerButton size="sm" />
        </div>
      </div>

      {/* Stale warning banner (S5-AC3) */}
      {staleCount > 0 && (
        <div
          className="rounded-md border border-amber-300 bg-amber-50 px-4 py-2 text-sm text-amber-800"
          role="alert"
        >
          <strong>Peringatan:</strong> {staleCount} instrumen memiliki staging stale (&gt; batas
          konfigurasi). Review diperlukan oleh ROLE-RISK atau ROLE-AKUN-CTL.{" "}
          <button
            className="underline"
            onClick={() => void filters.setHasStaleFlag(true)}
            aria-label="Filter hanya instrumen staging stale"
          >
            Tampilkan saja stale
          </button>
        </div>
      )}

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 items-center">
        <Select
          value={filters.filterStatus}
          onValueChange={(v) => void filters.setFilterStatus(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-52" aria-label="Filter status akrual">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Status</SelectItem>
            {Object.entries(AKRUAL_STATUS_LABELS).map(([v, l]) => (
              <SelectItem key={v} value={v}>{l}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.filterJenis}
          onValueChange={(v) => void filters.setFilterJenis(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-44" aria-label="Filter jenis akrual">
            <SelectValue placeholder="Semua Jenis" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Jenis</SelectItem>
            {Object.entries(AKRUAL_JENIS_LABELS).map(([v, l]) => (
              <SelectItem key={v} value={v}>{l}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.filterStage}
          onValueChange={(v) => void filters.setFilterStage(v === "ALL" ? "" : v)}
        >
          <SelectTrigger className="w-32" aria-label="Filter ECL stage">
            <SelectValue placeholder="Semua Stage" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Stage</SelectItem>
            <SelectItem value="1">Stage 1</SelectItem>
            <SelectItem value="2">Stage 2</SelectItem>
            <SelectItem value="3">Stage 3</SelectItem>
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
          void filters.setFilterStage("");
          void filters.setHasStaleFlag(false);
          void filters.setQ("");
        }}
        searchValue={filters.q}
        onSearchChange={(v) => void filters.setQ(v)}
        onExport={handleExport}
        exportFormats={["csv", "xlsx"]}
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
        emptyMessage="Tidak ada akrual yang cocok dengan filter."
      />

      {overrideTarget && (
        <AkrualOverrideStaleDialog
          open={!!overrideTarget}
          onOpenChange={(v) => {
            if (!v) setOverrideTarget(null);
          }}
          akrualId={overrideTarget.id}
          instrumenKode={overrideTarget.instrumenKode}
          stage={overrideTarget.stage as 1 | 2 | 3 | null}
          onSuccess={() => {
            setOverrideTarget(null);
            void queryClient.invalidateQueries({ queryKey: akrualQueryKeys.lists() });
          }}
        />
      )}
    </div>
  );
}

export default function AkrualPage() {
  return (
    <Suspense>
      <AkrualListContent />
    </Suspense>
  );
}
