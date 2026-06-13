"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";

import { eirApi } from "@/lib/api/eir.api";
import type { EIRAmendmentProposal } from "@/lib/schemas/eir.schema";
import { RoutingPathBadge } from "@/components/blips/RoutingPathBadge";
import type { TriggerSource } from "@/components/blips/RoutingPathBadge";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { usePermissions } from "@/lib/stores/auth.store";

const STATUS_LABELS: Record<string, { label: string; variant: "default" | "secondary" | "destructive" | "outline" }> = {
  DRAFT: { label: "Draft", variant: "secondary" },
  PENDING_REVIEW: { label: "Menunggu Review", variant: "default" },
  PENDING_APPROVAL: { label: "Menunggu Approval ALCO", variant: "default" },
  APPROVED: { label: "Disetujui", variant: "default" },
  REJECTED: { label: "Ditolak", variant: "destructive" },
};

// Format EIR string (NUMERIC(10,8)) for display — parseFloat only for display
function fmt(s: string | null | undefined): string {
  if (!s) return "—";
  const n = parseFloat(s);
  if (isNaN(n)) return s;
  return `${(n * 100).toFixed(4)}%`;
}

function buildColumns(router: ReturnType<typeof useRouter>): ColumnDef<EIRAmendmentProposal>[] {
  return [
    {
      id: "instrumen",
      header: "Instrumen",
      enableSorting: true,
      cell: ({ row }) => (
        <button
          className="text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ring rounded"
          onClick={() => router.push(`/ecl/eir/amendments/${row.original.id}`)}
        >
          {row.original.kodeInstrumen ?? row.original.instrumenId.slice(0, 8)}
        </button>
      ),
    },
    {
      id: "eirSebelum",
      header: "EIR Sebelum",
      cell: ({ row }) => (
        <span className="text-sm font-mono">{fmt(row.original.eirSebelum)}</span>
      ),
    },
    {
      id: "eirSesudah",
      header: "EIR Sesudah",
      cell: ({ row }) => (
        <span className="text-sm font-mono">{fmt(row.original.eirSesudah)}</span>
      ),
    },
    {
      id: "delta",
      header: "Delta",
      cell: ({ row }) => {
        const d = row.original.driftDelta;
        if (!d) return <span className="text-muted-foreground">—</span>;
        const num = parseFloat(d);
        const bps = isNaN(num) ? "?" : Math.round(num * 10000).toString();
        return <span className={`text-sm font-mono ${num > 0 ? "text-green-600" : "text-red-600"}`}>{bps} bp</span>;
      },
    },
    {
      id: "triggerSource",
      header: "Sumber",
      cell: ({ row }) => (
        <RoutingPathBadge triggerSource={row.original.triggerSource as TriggerSource} />
      ),
    },
    {
      id: "workflowStatus",
      header: "Status",
      cell: ({ row }) => {
        const cfg = STATUS_LABELS[row.original.workflowStatus] ?? { label: row.original.workflowStatus, variant: "secondary" as const };
        return <Badge variant={cfg.variant}>{cfg.label}</Badge>;
      },
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
          onClick={() => router.push(`/ecl/eir/amendments/${row.original.id}`)}
          aria-label={`Lihat detail amandemen ${row.original.kodeInstrumen ?? row.original.instrumenId}`}
        >
          Detail
        </Button>
      ),
    },
  ];
}

export default function EIRAmendmentQueuePage() {
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
    ...(statusFilter && { "filter[workflow_status]": statusFilter }),
    ...(searchValue && { q: searchValue }),
    ...(cursor && { cursor }),
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["eir-amendments", params],
    queryFn: () => eirApi.listAmendments(params),
  });

  const activeFilters: ActiveFilter[] = [];
  if (statusFilter) {
    activeFilters.push({
      key: "workflow_status",
      label: "Status",
      value: statusFilter,
      displayValue: STATUS_LABELS[statusFilter]?.label ?? statusFilter,
    });
  }

  const handleExport = (format: "csv" | "xlsx") => {
    const url = `/api/v1/ecl/eir/amendments/export?format=${format}`;
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
          <h1 className="text-xl font-semibold">Amandemen EIR</h1>
          <p className="text-sm text-muted-foreground">
            Antrian proposal re-estimasi EIR (4-eyes: AKUN → RISK → ALCO)
          </p>
        </div>
        {can("ecl_eir.amendment.propose") && (
          <Button onClick={() => router.push("/ecl/eir/amendments/new")}>
            <Plus className="h-4 w-4 mr-1" aria-hidden="true" />
            Ajukan Amandemen
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
        onRemoveFilter={(key) => { if (key === "workflow_status") setStatusFilter(""); }}
        onClearFilters={() => setStatusFilter("")}
        onExport={handleExport}
        onRefresh={() => void refetch()}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageNumber > 1}
        pageNumber={pageNumber}
        emptyMessage="Belum ada proposal amandemen EIR."
        onRetry={() => void refetch()}
      />
    </div>
  );
}
