"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef, SortingState } from "@tanstack/react-table";
import { TrendingUp, Info } from "lucide-react";

import { rollForwardApi } from "@/lib/api/roll-forward.api";
import { usePermissions } from "@/lib/stores/auth.store";
import { useRollForwardStore } from "@/lib/stores/roll-forward.store";
import { CKPNTrendChart } from "@/components/blips/CKPNTrendChart";
import { DataTable } from "@/components/blips/DataTable";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { CKPNTrendPoint } from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const isNeg = value.startsWith("-");
  const abs = isNeg ? value.slice(1) : value;
  const num = parseFloat(abs);
  if (isNaN(num)) return value;
  const f = new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
  return isNeg ? `−${f}` : f;
}

// ---------------------------------------------------------------------------
// DataTable columns
// ---------------------------------------------------------------------------

function buildTrendColumns(
  router: ReturnType<typeof useRouter>,
): ColumnDef<CKPNTrendPoint>[] {
  return [
    {
      id: "periodeId",
      header: "Periode",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-sm font-medium">{row.original.periodeId}</span>
      ),
    },
    {
      id: "eclTotalIdr",
      header: "Total ECL (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="font-mono text-xs">
          {formatIDR(row.original.eclTotalIdr)}
        </span>
      ),
    },
    {
      id: "stage1",
      header: "Stage 1 (IDR)",
      cell: ({ row }) => (
        <span className="font-mono text-xs text-green-700">
          {formatIDR(row.original.eclByStage.stage1)}
        </span>
      ),
    },
    {
      id: "stage2",
      header: "Stage 2 (IDR)",
      cell: ({ row }) => (
        <span className="font-mono text-xs text-amber-700">
          {formatIDR(row.original.eclByStage.stage2)}
        </span>
      ),
    },
    {
      id: "stage3",
      header: "Stage 3 (IDR)",
      cell: ({ row }) => (
        <span className="font-mono text-xs text-red-700">
          {formatIDR(row.original.eclByStage.stage3)}
        </span>
      ),
    },
    {
      id: "deltaVsPriorIdr",
      header: "Delta vs Sebelumnya",
      enableSorting: true,
      cell: ({ row }) => {
        const v = row.original.deltaVsPriorIdr;
        const pct = row.original.deltaPct;
        if (!v) return <span className="text-xs text-muted-foreground">—</span>;
        const isNeg = v.startsWith("-");
        return (
          <span className={`font-mono text-xs ${isNeg ? "text-green-700" : "text-red-700"}`}>
            {formatIDR(v)}
            {pct && (
              <span className="ml-1 text-muted-foreground">
                ({isNeg ? "" : "+"}
                {pct}%)
              </span>
            )}
          </span>
        );
      },
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => (
        <Button
          variant="link"
          size="sm"
          className="h-auto p-0 text-xs"
          onClick={() => {
            const p = row.original;
            const url = `/ecl/roll-forward?currentCalcRunId=${p.calcRunId}${p.priorCalcRunId ? `&priorCalcRunId=${p.priorCalcRunId}` : ""}`;
            router.push(url);
          }}
          aria-label={`Lihat roll-forward ${row.original.periodeId}`}
        >
          Roll-Forward →
        </Button>
      ),
    },
  ];
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function CKPNTrendPage() {
  const router = useRouter();
  const { can } = usePermissions();

  const trendPeriods = useRollForwardStore((s) => s.trendPeriods);
  const setTrendPeriods = useRollForwardStore((s) => s.setTrendPeriods);

  const [sorting, setSorting] = React.useState<SortingState>([]);
  const hasPermission = can("ecl.roll_forward.read");

  const {
    data: trendData,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: ["ckpn-trend", trendPeriods],
    queryFn: () => rollForwardApi.getCKPNTrend({ periods: trendPeriods }),
    retry: 1,
  });

  const trend = trendData?.data;
  const points = trend?.periods ?? [];

  // Insufficient data (422) — check error code
  const isInsufficientData =
    isError &&
    (error as { code?: string }).code === "ROLL_FORWARD_TREND_INSUFFICIENT_DATA";

  const handleClickPoint = (point: CKPNTrendPoint) => {
    const url = `/ecl/roll-forward?currentCalcRunId=${point.calcRunId}${point.priorCalcRunId ? `&priorCalcRunId=${point.priorCalcRunId}` : ""}`;
    router.push(url);
  };

  const sortStr = sorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(",");

  const handleExport = (format: "csv" | "xlsx") => {
    rollForwardApi.getCKPNTrend({
      periods: trendPeriods,
      export: format,
      sort: sortStr || undefined,
    });
  };

  const columns = React.useMemo(() => buildTrendColumns(router), [router]);

  // Permission guard (after all hooks)
  if (!hasPermission) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertTitle>Akses Ditolak</AlertTitle>
          <AlertDescription>
            Anda tidak memiliki izin <code>ecl.roll_forward.read</code>.
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-6xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/dashboard")}
            >
              Dashboard
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">Tren CKPN</li>
        </ol>
      </nav>

      {/* Header */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-xl font-semibold flex items-center gap-2">
            <TrendingUp className="h-5 w-5 text-primary" aria-hidden="true" />
            Tren ECL CKPN Multi-Periode
          </h1>
          <p className="text-sm text-muted-foreground mt-1">
            Data dari calc run yang sudah di-SEAL. Klik titik/batang untuk
            membuka laporan roll-forward periode tersebut.
          </p>
        </div>

        {/* Period selector */}
        <div className="flex items-center gap-2">
          <label
            htmlFor="period-select"
            className="text-sm font-medium text-muted-foreground"
          >
            Periode terakhir:
          </label>
          <Select
            value={String(trendPeriods)}
            onValueChange={(v) => setTrendPeriods(parseInt(v, 10))}
          >
            <SelectTrigger
              id="period-select"
              className="w-24"
              aria-label="Pilih jumlah periode"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {[2, 3, 6, 12, 18, 24].map((n) => (
                <SelectItem key={n} value={String(n)}>
                  {n}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      {/* Insufficient data empty state */}
      {isInsufficientData && (
        <Card className="border-dashed">
          <CardContent className="py-12 text-center">
            <Info className="h-10 w-10 text-muted-foreground mx-auto mb-3" aria-hidden="true" />
            <p className="text-lg font-medium text-muted-foreground">
              Data Tren Belum Tersedia
            </p>
            <p className="text-sm text-muted-foreground mt-1 max-w-sm mx-auto">
              Minimal 2 periode SEALED diperlukan untuk menampilkan tren ECL.
              Seal setidaknya 2 calc run terlebih dahulu.
            </p>
            <Button
              variant="outline"
              className="mt-4"
              onClick={() => router.push("/ecl/calc-runs")}
            >
              Kelola Calc Runs
            </Button>
          </CardContent>
        </Card>
      )}

      {/* Generic error */}
      {isError && !isInsufficientData && (
        <Alert variant="destructive">
          <AlertTitle>Gagal Memuat Tren</AlertTitle>
          <AlertDescription>
            Tidak dapat memuat data tren CKPN.
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 ml-1"
              onClick={() => void refetch()}
            >
              Coba lagi
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Loading skeleton */}
      {isLoading && (
        <div className="space-y-4">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-64 w-full" />
          <Skeleton className="h-48 w-full" />
        </div>
      )}

      {/* Chart */}
      {!isLoading && !isError && points.length >= 2 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">
              Grafik ECL CKPN — {points.length} Periode
              {trend?.note && (
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  ({trend.note})
                </span>
              )}
            </CardTitle>
            <CardDescription>
              Data dari calc run SEALED. Total ECL dan distribusi per stage.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <CKPNTrendChart
              points={points}
              onClickPoint={handleClickPoint}
            />
          </CardContent>
        </Card>
      )}

      {/* Data table */}
      {!isLoading && !isError && points.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">
              Data Tren CKPN — Detail
            </CardTitle>
          </CardHeader>
          <CardContent className="p-0 pb-4">
            <DataTable
              columns={columns}
              data={points}
              isLoading={isLoading}
              isError={isError}
              sorting={sorting}
              onSortingChange={setSorting}
              onExport={handleExport}
              onRefresh={() => void refetch()}
              emptyMessage="Tidak ada data tren CKPN tersedia"
            />
          </CardContent>
        </Card>
      )}

      {/* Quick link */}
      {!isLoading && (
        <div className="flex gap-3">
          <Button
            variant="outline"
            onClick={() => router.push("/ecl/roll-forward")}
          >
            Generate Roll-Forward Baru
          </Button>
          <Button
            variant="outline"
            onClick={() => router.push("/ecl/calc-runs")}
          >
            Kelola Calc Runs
          </Button>
        </div>
      )}
    </div>
  );
}
