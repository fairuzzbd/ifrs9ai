"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import type { ColumnDef } from "@tanstack/react-table";
import { RefreshCw } from "lucide-react";

import { eirApi } from "@/lib/api/eir.api";
import type { DriftReport } from "@/lib/schemas/eir.schema";
import { RoutingPathBadge } from "@/components/blips/RoutingPathBadge";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { DataTable } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import { useEIRStore } from "@/lib/stores/eir.store";

// Trigger source mapping for DriftReport
const TRIGGER_LABELS: Record<string, string> = {
  CRON_DAILY: "Cron Harian",
  AD_HOC: "Ad-Hoc",
  PRE_ECL_CALC: "Pre-ECL Calc",
};

function buildColumns(router: ReturnType<typeof useRouter>): ColumnDef<DriftReport>[] {
  return [
    {
      id: "id",
      header: "Report ID",
      cell: ({ row }) => (
        <button
          className="text-sm font-mono text-primary hover:underline focus-visible:ring-2 focus-visible:ring-ring rounded"
          onClick={() => router.push(`/ecl/eir/drift-reports/${row.original.id}`)}
        >
          {row.original.id.slice(0, 12)}…
        </button>
      ),
    },
    {
      id: "triggerSource",
      header: "Sumber",
      cell: ({ row }) => (
        <span className="text-sm">
          {TRIGGER_LABELS[row.original.triggerSource] ?? row.original.triggerSource}
        </span>
      ),
    },
    {
      id: "scanStartedAt",
      header: "Mulai Scan",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-sm">
          {new Date(row.original.scanStartedAt).toLocaleString("id-ID")}
        </span>
      ),
    },
    {
      id: "totalScanned",
      header: "Total Instrumen",
      cell: ({ row }) => (
        <span className="text-sm text-right font-mono">
          {row.original.totalScanned.toLocaleString("id-ID")}
        </span>
      ),
    },
    {
      id: "driftCount",
      header: "Drift",
      cell: ({ row }) => (
        <span className={`text-sm font-mono font-medium ${row.original.driftCount > 0 ? "text-amber-600" : "text-green-600"}`}>
          {row.original.driftCount}
        </span>
      ),
    },
    {
      id: "proposalsAutoCreated",
      header: "Auto-Proposal",
      cell: ({ row }) => (
        <span className="text-sm font-mono">{row.original.proposalsAutoCreated}</span>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: ({ row }) => {
        const s = row.original.status ?? "COMPLETED";
        return (
          <Badge variant={s === "COMPLETED" ? "default" : "secondary"}>
            {s}
          </Badge>
        );
      },
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <Button
          size="sm"
          variant="ghost"
          onClick={() => router.push(`/ecl/eir/drift-reports/${row.original.id}`)}
          aria-label={`Lihat detail drift report ${row.original.id}`}
        >
          Detail
        </Button>
      ),
    },
  ];
}

export default function DriftReportsPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { can } = usePermissions();
  const { activeDriftJobId, setActiveDriftJobId } = useEIRStore();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);

  const params = {
    limit: 50,
    sort: "scan_started_at:desc",
    ...(cursor && { cursor }),
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ["eir-drift-reports", params],
    queryFn: () => eirApi.listDriftReports(params),
  });

  const bulkRecomputeMutation = useMutation({
    mutationFn: () =>
      eirApi.triggerBulkRecompute(
        { scope: "ALL", reason: "Ad-hoc bulk recompute triggered by user" },
        uuidv4(),
      ),
    onSuccess: (res) => {
      setActiveDriftJobId(res.data.jobId);
      notify.info("Bulk re-estimasi EIR dimulai...");
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

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
          <h1 className="text-xl font-semibold">EIR Drift Reports</h1>
          <p className="text-sm text-muted-foreground">
            Laporan pemindaian drift EIR — mendeteksi penyimpangan antara EIR
            tersimpan dan hasil re-komputasi Newton-Raphson.
          </p>
        </div>
        {can("ecl_eir.bulk_recompute") && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => bulkRecomputeMutation.mutate()}
            disabled={bulkRecomputeMutation.isPending || !!activeDriftJobId}
          >
            <RefreshCw className="h-4 w-4 mr-1" aria-hidden="true" />
            Jalankan Scan Ad-Hoc
          </Button>
        )}
      </div>

      {/* Progress panel */}
      {activeDriftJobId && (
        <JobProgressPanel
          jobId={activeDriftJobId}
          title="Bulk re-estimasi EIR berjalan..."
          variant="inline"
          showBackground
          onComplete={() => {
            setActiveDriftJobId(null);
            void queryClient.invalidateQueries({ queryKey: ["eir-drift-reports"] });
            notify.success("Bulk re-estimasi EIR selesai. Laporan drift terbaru tersedia.");
          }}
          onFail={() => {
            setActiveDriftJobId(null);
          }}
        />
      )}

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        onRefresh={() => void refetch()}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageNumber > 1}
        pageNumber={pageNumber}
        emptyMessage="Belum ada drift report. Jalankan scan ad-hoc atau tunggu cron harian."
        onRetry={() => void refetch()}
      />
    </div>
  );
}
