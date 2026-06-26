"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { Plus, AlertTriangle } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PenjualanStatusBadge } from "@/components/blips/penjualan/PenjualanStatusBadge";
import { PenjualanJenisBadge } from "@/components/blips/penjualan/PenjualanJenisBadge";
import { PenjualanApproveDialog } from "@/components/blips/penjualan/PenjualanApproveDialog";
import { PenjualanRejectDialog } from "@/components/blips/penjualan/PenjualanRejectDialog";

import { penjualanListApi, penjualanQueryKeys } from "@/lib/api/penjualan.api";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  PENJUALAN_STATUS_LABELS,
  JENIS_DISPOSAL_LABELS,
  type PenjualanListItem,
  type PenjualanStatus,
  type JenisDisposal,
} from "@/lib/schemas/penjualan.schema";
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

function usePenjualanFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("created_at:desc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status]", parseAsString.withDefault(""));
  const [filterJenis, setFilterJenis] = useQueryState("filter[jenis_disposal]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return { q, setQ, sort, setSort, filterStatus, setFilterStatus, filterJenis, setFilterJenis, cursor, setCursor, limit };
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function PenjualanListContent() {
  const perms = usePermissions();
  const queryClient = useQueryClient();
  const filters = usePenjualanFilters();
  const { setSort } = filters;

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const [approveTarget, setApproveTarget] = React.useState<PenjualanListItem | null>(null);
  const [rejectTarget, setRejectTarget] = React.useState<PenjualanListItem | null>(null);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: penjualanQueryKeys.list({
      limit: filters.limit,
      cursor: cursorHistory[pageIndex] || undefined,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
      "filter[status]": filters.filterStatus || undefined,
      "filter[jenis_disposal]": filters.filterJenis || undefined,
    }),
    queryFn: () =>
      penjualanListApi.list({
        limit: filters.limit,
        cursor: cursorHistory[pageIndex] || undefined,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[status]": filters.filterStatus || undefined,
        "filter[jenis_disposal]": filters.filterJenis || undefined,
      }),
    staleTime: 30_000,
  });

  const items = data?.data ?? [];
  const pagination = data?.pagination;
  const totalEstimate = pagination?.totalEstimate ?? 0;

  const [sorting, setSorting] = React.useState<SortingState>([{ id: "created_at", desc: true }]);

  React.useEffect(() => {
    const [s] = sorting;
    if (s) {
      void setSort(`${s.id}:${s.desc ? "desc" : "asc"}`);
    }
  }, [sorting, setSort]);

  const activeFilters: ActiveFilter[] = [];
  if (filters.filterStatus) activeFilters.push({ label: "Status", value: PENJUALAN_STATUS_LABELS[filters.filterStatus as PenjualanStatus] ?? filters.filterStatus, onRemove: () => void filters.setFilterStatus("") });
  if (filters.filterJenis) activeFilters.push({ label: "Jenis", value: JENIS_DISPOSAL_LABELS[filters.filterJenis as JenisDisposal] ?? filters.filterJenis, onRemove: () => void filters.setFilterJenis("") });

  const columns: ColumnDef<PenjualanListItem>[] = [
    {
      id: "instrumenKode",
      header: ({ column }) => <SortHeader column={column} label="Kode Instrumen" />,
      accessorKey: "instrumenKode",
      cell: ({ row }) => (
        <Link href={`/transaksi/penjualan/${row.original.id}`} className="font-mono text-blue-600 hover:underline focus:outline-none focus:ring-2 focus:ring-blue-400 rounded">
          {row.original.instrumenKode}
        </Link>
      ),
    },
    {
      id: "jenisDisposal",
      header: "Jenis",
      accessorKey: "jenisDisposal",
      cell: ({ row }) => <PenjualanJenisBadge jenis={row.original.jenisDisposal} size="sm" />,
    },
    {
      id: "klasifikasiSnapshot",
      header: "Klasifikasi",
      accessorKey: "klasifikasiSnapshot",
      cell: ({ row }) => (
        <span className="font-mono text-xs">{row.original.klasifikasiSnapshot}</span>
      ),
    },
    {
      id: "qtyTerjual",
      header: ({ column }) => <SortHeader column={column} label="Qty Terjual" />,
      accessorKey: "qtyTerjual",
      cell: ({ row }) => <span className="font-mono text-right block">{row.original.qtyTerjual}</span>,
    },
    {
      id: "proceedIdr",
      header: ({ column }) => <SortHeader column={column} label="Proceeds (IDR)" />,
      accessorKey: "proceedIdr",
      cell: ({ row }) => (
        <span className="font-mono text-right block">{IDR.format(parseFloat(row.original.proceedIdr))}</span>
      ),
    },
    {
      id: "tanggalEksekusi",
      header: ({ column }) => <SortHeader column={column} label="Tgl Eksekusi" />,
      accessorKey: "tanggalEksekusi",
    },
    {
      id: "status",
      header: "Status",
      accessorKey: "status",
      cell: ({ row }) => <PenjualanStatusBadge status={row.original.status} size="sm" />,
    },
    {
      id: "actions",
      header: "Aksi",
      cell: ({ row }) => {
        const item = row.original;
        const canApprove = perms.canApprove?.("transaksi") && item.status === "PENDING_APPROVAL";
        if (!canApprove) return null;
        return (
          <div className="flex gap-1">
            <Button size="sm" variant="outline" onClick={() => setApproveTarget(item)} aria-label={`Setujui penjualan ${item.instrumenKode}`}>
              Setujui
            </Button>
            <Button size="sm" variant="ghost" className="text-red-600" onClick={() => setRejectTarget(item)} aria-label={`Tolak penjualan ${item.instrumenKode}`}>
              Tolak
            </Button>
          </div>
        );
      },
    },
  ];

  const handleExport = (format: "csv" | "xlsx") => {
    const url = penjualanListApi.exportUrl({
      format,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
      "filter[status]": filters.filterStatus || undefined,
    });
    window.open(url, "_blank");
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Penjualan / Pencairan Instrumen</h1>
          <p className="text-sm text-muted-foreground">
            Daftar penjualan instrumen keuangan — APP-B P5-M8
          </p>
        </div>
        <div className="flex gap-2">
          <Link href="/transaksi/penjualan/bm-alerts">
            <Button variant="outline" size="sm" aria-label="Lihat BM frequency alerts">
              <AlertTriangle className="h-4 w-4 mr-1" aria-hidden="true" />
              BM Alerts
            </Button>
          </Link>
          {perms.canCreate?.("transaksi") && (
            <Link href="/transaksi/penjualan/new">
              <Button size="sm" aria-label="Buat penjualan baru">
                <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
                Buat Penjualan
              </Button>
            </Link>
          )}
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 items-center">
        <Select value={filters.filterStatus} onValueChange={(v) => void filters.setFilterStatus(v === "ALL" ? "" : v)}>
          <SelectTrigger className="w-48" aria-label="Filter status penjualan">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Status</SelectItem>
            {Object.entries(PENJUALAN_STATUS_LABELS).map(([v, l]) => (
              <SelectItem key={v} value={v}>{l}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={filters.filterJenis} onValueChange={(v) => void filters.setFilterJenis(v === "ALL" ? "" : v)}>
          <SelectTrigger className="w-40" aria-label="Filter jenis disposal">
            <SelectValue placeholder="Semua Jenis" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">Semua Jenis</SelectItem>
            {Object.entries(JENIS_DISPOSAL_LABELS).map(([v, l]) => (
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
        emptyMessage="Tidak ada penjualan yang cocok dengan filter."
      />

      {approveTarget && (
        <PenjualanApproveDialog
          open={!!approveTarget}
          onOpenChange={(v) => { if (!v) setApproveTarget(null); }}
          penjualanId={approveTarget.id}
          instrumenKode={approveTarget.instrumenKode}
          makerId={approveTarget.makerId}
          onSuccess={() => {
            setApproveTarget(null);
            void queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.lists() });
          }}
        />
      )}

      {rejectTarget && (
        <PenjualanRejectDialog
          open={!!rejectTarget}
          onOpenChange={(v) => { if (!v) setRejectTarget(null); }}
          penjualanId={rejectTarget.id}
          instrumenKode={rejectTarget.instrumenKode}
          onSuccess={() => {
            setRejectTarget(null);
            void queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.lists() });
          }}
        />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PenjualanPage() {
  return (
    <Suspense>
      <PenjualanListContent />
    </Suspense>
  );
}
