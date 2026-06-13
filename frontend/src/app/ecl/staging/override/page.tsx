"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";

import { stagingApi } from "@/lib/api/staging.api";
import type { StagingOverrideProposal } from "@/lib/schemas/staging.schema";
import { StageBadge } from "@/components/blips/StageBadge";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { usePermissions } from "@/lib/stores/auth.store";

const STATUS_LABELS: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
  DRAFT: { label: "Draft", variant: "secondary" },
  PENDING_REVIEW: { label: "Menunggu Review", variant: "default" },
  PENDING_APPROVAL_1: { label: "Menunggu ALCO", variant: "default" },
  PENDING_APPROVAL_2: { label: "Menunggu KOMITE", variant: "default" },
  APPROVED: { label: "Disetujui", variant: "default" },
  REJECTED: { label: "Ditolak", variant: "destructive" },
  CANCELLED: { label: "Dibatalkan", variant: "outline" },
};

function buildColumns(router: ReturnType<typeof useRouter>): ColumnDef<StagingOverrideProposal>[] {
  return [
    {
      id: "instrumenKode",
      header: "Instrumen",
      enableSorting: true,
      cell: ({ row }) => (
        <button
          className="text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ring rounded"
          onClick={() => router.push(`/ecl/staging/override/${row.original.id}`)}
        >
          {row.original.kodeInstrumen ?? row.original.instrumenId}
        </button>
      ),
    },
    {
      id: "stageDiusulkan",
      header: "Stage Diusulkan",
      cell: ({ row }) => {
        const s = row.original.stageTo;
        const num = parseInt(s.replace("STAGE_", ""), 10) as 1 | 2 | 3;
        return <StageBadge stage={num} size="sm" />;
      },
    },
    {
      id: "status",
      header: "Status",
      cell: ({ row }) => {
        const cfg = STATUS_LABELS[row.original.status] ?? { label: row.original.status, variant: "secondary" as const };
        return <Badge variant={cfg.variant}>{cfg.label}</Badge>;
      },
    },
    {
      id: "alasan",
      header: "Alasan",
      cell: ({ row }) => (
        <span className="text-sm line-clamp-2 max-w-xs">{row.original.alasan}</span>
      ),
    },
    {
      id: "makerId",
      header: "Diajukan Oleh",
      cell: ({ row }) => <span className="text-xs font-mono text-muted-foreground">{row.original.makerId.slice(0, 8)}</span>,
    },
    {
      id: "createdAt",
      header: "Dibuat",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs">
          {new Date(row.original.createdAt).toLocaleDateString("id-ID")}
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
          onClick={() => router.push(`/ecl/staging/override/${row.original.id}`)}
          aria-label={`Lihat detail override ${row.original.kodeInstrumen ?? row.original.instrumenId}`}
        >
          Detail
        </Button>
      ),
    },
  ];
}

export default function StagingOverrideQueuePage() {
  const router = useRouter();
  const { can } = usePermissions();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);
  const [statusFilter, setStatusFilter] = React.useState("");
  const [searchValue, setSearchValue] = React.useState("");

  const params = {
    limit: 50,
    sort: "created_at:desc",
    ...(statusFilter && { "filter[status]": statusFilter }),
    ...(searchValue && { q: searchValue }),
    ...(cursor && { cursor }),
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["staging-overrides", params],
    queryFn: () => stagingApi.listOverrides(params),
  });

  const activeFilters: ActiveFilter[] = [];
  if (statusFilter) {
    activeFilters.push({
      key: "status",
      label: "Status",
      value: statusFilter,
      displayValue: STATUS_LABELS[statusFilter]?.label ?? statusFilter,
    });
  }

  const handleExport = (format: "csv" | "xlsx") => {
    const url = `/api/v1/ecl/staging/overrides/export?format=${format}&${Object.entries(params)
      .map(([k, v]) => `${k}=${v}`)
      .join("&")}`;
    window.open(url, "_blank");
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

  const columns = buildColumns(router);

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Override Staging</h1>
          <p className="text-sm text-muted-foreground">
            Antrian proposal override stage instrumen
          </p>
        </div>
        {can("ecl_staging.override.propose") && (
          <Button onClick={() => router.push("/ecl/staging/override/new")}>
            <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
            Ajukan Override
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
        searchPlaceholder="Cari instrumen..."
        activeFilters={activeFilters}
        onRemoveFilter={(key) => { if (key === "status") setStatusFilter(""); }}
        onClearFilters={() => setStatusFilter("")}
        onExport={handleExport}
        onRefresh={() => void refetch()}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageNumber > 1}
        pageNumber={pageNumber}
        emptyMessage="Belum ada proposal override staging."
        onRetry={() => void refetch()}
      />
    </div>
  );
}
