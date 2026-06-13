"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";

import { calcRunApi } from "@/lib/api/calc-run.api";
import type { CalcRun } from "@/lib/schemas/calc-run.schema";
import { CalcRunStatusBadge } from "@/components/blips/CalcRunStatusBadge";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { CreateCalcRunModal } from "./_components/CreateCalcRunModal";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// Columns
// ---------------------------------------------------------------------------

function buildColumns(
  router: ReturnType<typeof useRouter>,
): ColumnDef<CalcRun>[] {
  return [
    {
      id: "id",
      header: "ID",
      enableSorting: true,
      cell: ({ row }) => (
        <button
          className="text-sm font-mono text-primary underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ring rounded"
          onClick={() => router.push(`/ecl/calc-runs/${row.original.id}`)}
          aria-label={`Lihat detail calc run ${row.original.id}`}
        >
          {row.original.id}
        </button>
      ),
    },
    {
      id: "periodeId",
      header: "Periode",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-sm">{row.original.periodeLabel ?? row.original.periodeId}</span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: ({ row }) => (
        <CalcRunStatusBadge status={row.original.status} size="sm" />
      ),
    },
    {
      id: "totalInstrumen",
      header: "Total Instr",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-sm tabular-nums">
          {row.original.totalInstrumen ?? "—"}
        </span>
      ),
    },
    {
      id: "processedCount",
      header: "Diproses",
      cell: ({ row }) => (
        <span className="text-sm tabular-nums">
          {row.original.processedCount != null
            ? `${row.original.processedCount} / ${row.original.totalInstrumen ?? "?"}`
            : "—"}
        </span>
      ),
    },
    {
      id: "errorCount",
      header: "Error",
      cell: ({ row }) => (
        <span
          className={
            row.original.errorCount > 0
              ? "text-sm tabular-nums text-destructive font-medium"
              : "text-sm tabular-nums text-muted-foreground"
          }
        >
          {row.original.errorCount}
        </span>
      ),
    },
    {
      id: "createdBy",
      header: "Dibuat Oleh",
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">
          {row.original.createdByUsername ?? row.original.createdBy.slice(0, 8)}
        </span>
      ),
    },
    {
      id: "createdAt",
      header: "Dibuat",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs">
          {new Date(row.original.createdAt).toLocaleDateString("id-ID", {
            day: "2-digit",
            month: "short",
            year: "numeric",
          })}
        </span>
      ),
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <Button
          size="sm"
          variant="ghost"
          onClick={() => router.push(`/ecl/calc-runs/${row.original.id}`)}
          aria-label={`Lihat detail calc run ${row.original.id}`}
        >
          Detail
        </Button>
      ),
    },
  ];
}

// ---------------------------------------------------------------------------
// Status filter options
// ---------------------------------------------------------------------------

const STATUS_OPTIONS = [
  { value: "DRAFT", label: "DRAFT" },
  { value: "IN_PROGRESS", label: "Sedang Berjalan" },
  { value: "COMPLETED", label: "SELESAI" },
  { value: "COMPLETED_WITH_ERRORS", label: "SELESAI dengan Error" },
  { value: "SEAL_REQUESTED", label: "Menunggu Segel" },
  { value: "SEALED", label: "TERSEGEL" },
  { value: "SEAL_REJECTED", label: "Segel Ditolak" },
  { value: "CANCELLED", label: "DIBATALKAN" },
];

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function CalcRunListPage() {
  const router = useRouter();
  const { can } = usePermissions();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);
  const [periodeFilter, setPeriodeFilter] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState("");
  const [searchValue, setSearchValue] = React.useState("");
  const [createModalOpen, setCreateModalOpen] = React.useState(false);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const params = {
    limit: 50,
    sort: "created_at:desc",
    ...(periodeFilter && { "filter[periode_id]": periodeFilter }),
    ...(statusFilter && { "filter[status]": statusFilter }),
    ...(searchValue && { q: searchValue }),
    ...(cursor && { cursor }),
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["calc-runs", params],
    queryFn: () => calcRunApi.list(params),
  });

  const handleRefresh = () => {
    void refetch();
    setLastUpdated(new Date());
  };

  const activeFilters: ActiveFilter[] = [];
  if (periodeFilter) {
    activeFilters.push({
      key: "periode_id",
      label: "Periode",
      value: periodeFilter,
      displayValue: periodeFilter,
    });
  }
  if (statusFilter) {
    activeFilters.push({
      key: "status",
      label: "Status",
      value: statusFilter,
      displayValue:
        STATUS_OPTIONS.find((s) => s.value === statusFilter)?.label ?? statusFilter,
    });
  }

  const handleExport = (format: "csv" | "xlsx") => {
    calcRunApi.exportList({ ...params, export: format });
  };

  const handleNextPage = () => {
    const next = data?.pagination?.nextCursor ?? null;
    if (next) {
      setPrevCursors((p) => [...p, cursor ?? ""]);
      setCursor(next);
      setPageNumber((n) => n + 1);
    }
  };

  const handlePrevPage = () => {
    const prev = prevCursors[prevCursors.length - 1] ?? null;
    setPrevCursors((p) => p.slice(0, -1));
    setCursor(prev);
    setPageNumber((n) => Math.max(1, n - 1));
  };

  const handleRemoveFilter = (key: string) => {
    if (key === "periode_id") setPeriodeFilter("");
    if (key === "status") setStatusFilter("");
  };

  const handleClearFilters = () => {
    setPeriodeFilter("");
    setStatusFilter("");
    setSearchValue("");
  };

  // Filter panel
  const filterPanel = (
    <div className="flex flex-wrap gap-2 p-3 border rounded-md bg-muted/10">
      <div className="flex flex-col gap-1 min-w-40">
        <label className="text-xs text-muted-foreground font-medium">Periode</label>
        <input
          className="border rounded px-2 py-1 text-sm bg-background"
          placeholder="JUNI-2026"
          value={periodeFilter}
          onChange={(e) => setPeriodeFilter(e.target.value)}
          aria-label="Filter berdasarkan periode"
        />
      </div>
      <div className="flex flex-col gap-1 min-w-44">
        <label className="text-xs text-muted-foreground font-medium">Status</label>
        <select
          className="border rounded px-2 py-1 text-sm bg-background"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          aria-label="Filter berdasarkan status"
        >
          <option value="">Semua status</option>
          {STATUS_OPTIONS.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </div>
    </div>
  );

  const columns = buildColumns(router);

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Perhitungan ECL — Calc Runs</h1>
          <p className="text-sm text-muted-foreground">
            Daftar semua run perhitungan Expected Credit Loss
          </p>
        </div>
        {can("calc_run.create") && (
          <Button onClick={() => setCreateModalOpen(true)}>
            <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
            Buat Calc Run Baru
          </Button>
        )}
      </div>

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        searchValue={searchValue}
        onSearchChange={setSearchValue}
        searchPlaceholder="Cari ID / periode..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={filterPanel}
        onExport={handleExport}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageNumber > 1}
        pageNumber={pageNumber}
        emptyMessage="Belum ada calc run. Buat yang pertama untuk memulai."
        onRetry={handleRefresh}
      />

      <CreateCalcRunModal
        open={createModalOpen}
        onOpenChange={setCreateModalOpen}
      />
    </div>
  );
}
