"use client";

import * as React from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, Info } from "lucide-react";

import { rollForwardApi } from "@/lib/api/roll-forward.api";
import { usePermissions } from "@/lib/stores/auth.store";
import { ReconcileBadge } from "@/components/blips/ReconcileBadge";
import { RollForwardDetectionMethodBadge } from "@/components/blips/RollForwardDetectionMethodBadge";
import { TransferBucketRow } from "@/components/blips/TransferBucketRow";
import { RollForwardExportButton } from "@/components/blips/RollForwardExportButton";
import { DataTable } from "@/components/blips/DataTable";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import type { ReconcileStatus } from "@/lib/schemas/ecl-core.schema";

// ---------------------------------------------------------------------------
// Formatter helpers
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

function formatDateTime(iso: string | undefined): string {
  if (!iso) return "—";
  return new Intl.DateTimeFormat("id-ID", {
    timeZone: "Asia/Jakarta",
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(iso));
}

// ---------------------------------------------------------------------------
// Per-portfolio breakdown columns
// ---------------------------------------------------------------------------

interface PortofolioRow {
  portofolioId: string;
  portofolioNama?: string;
  openingEclIdr: string | null;
  closingEclIdr: string | null;
}

function buildPortofolioColumns(
  router: ReturnType<typeof useRouter>,
  reportId: string,
  currentCalcRunId: string,
  priorCalcRunId: string | null,
): ColumnDef<PortofolioRow>[] {
  return [
    {
      id: "portofolioNama",
      header: "Portofolio",
      enableSorting: true,
      cell: ({ row }) => (
        <button
          className="text-sm text-primary underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ring rounded"
          onClick={() =>
            router.push(
              `/ecl/roll-forward/${encodeURIComponent(reportId)}/portofolio/${encodeURIComponent(row.original.portofolioId)}?currentCalcRunId=${currentCalcRunId}${priorCalcRunId ? `&priorCalcRunId=${priorCalcRunId}` : ""}`,
            )
          }
          aria-label={`Lihat detail portofolio ${row.original.portofolioNama ?? row.original.portofolioId}`}
        >
          {row.original.portofolioNama ?? row.original.portofolioId}
        </button>
      ),
    },
    {
      id: "openingEclIdr",
      header: "Saldo Awal (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="font-mono text-xs">{formatIDR(row.original.openingEclIdr)}</span>
      ),
    },
    {
      id: "closingEclIdr",
      header: "Saldo Akhir (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="font-mono text-xs font-semibold">
          {formatIDR(row.original.closingEclIdr)}
        </span>
      ),
    },
  ];
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function RollForwardReportPage() {
  const params = useParams<{ reportId: string }>();
  const searchParams = useSearchParams();
  const router = useRouter();
  const { can } = usePermissions();

  const reportId = params.reportId;
  const currentCalcRunId = searchParams.get("currentCalcRunId") ?? reportId;
  const priorCalcRunId = searchParams.get("priorCalcRunId") ?? null;
  const hasPermission = can("ecl.roll_forward.read");

  // Fetch report using the compute endpoint with GET semantic
  const {
    data: reportData,
    isLoading,
    isError,
    refetch,
  } = useQuery({
    queryKey: ["roll-forward-report", currentCalcRunId, priorCalcRunId],
    queryFn: () =>
      rollForwardApi.get({
        currentCalcRunId,
        priorCalcRunId: priorCalcRunId ?? undefined,
      }),
    enabled: !!currentCalcRunId,
    retry: 1,
  });

  const report = reportData?.data;

  // Reconcile badge — M11 only has RECONCILED or MISMATCH (no PARTIAL_PHASE_5_DEFER)
  // Map to compatible type for ReconcileBadge which still accepts PARTIAL_PHASE_5_DEFER
  const reconcileStatusForBadge: ReconcileStatus =
    report?.reconcileStatus === "MISMATCH"
      ? "MISMATCH"
      : "RECONCILED";

  const reconcileTooltip = report
    ? report.reconcileStatus === "RECONCILED"
      ? `Saldo Akhir sesuai total ECL calc run. Delta: ${formatIDR(report.reconcileDeltaIdr)}.`
      : `Saldo Akhir (${formatIDR(report.closingEclIdr)}) tidak sesuai total ECL calc run. Delta: ${formatIDR(report.reconcileDeltaIdr)}. Toleransi: ${formatIDR(report.reconcileTolerance)}.`
    : "";

  // Portfolio breakdown columns
  const portofolioColumns = React.useMemo(
    () =>
      buildPortofolioColumns(
        router,
        reportId,
        currentCalcRunId,
        priorCalcRunId,
      ),
    [router, reportId, currentCalcRunId, priorCalcRunId],
  );

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

  // Loading skeleton
  if (isLoading) {
    return (
      <div className="p-6 space-y-4 max-w-5xl">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-6 w-96" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  // Error state
  if (isError || !report) {
    return (
      <div className="p-6 space-y-4 max-w-3xl">
        <Alert variant="destructive">
          <AlertTitle>Laporan Tidak Ditemukan</AlertTitle>
          <AlertDescription>
            Tidak dapat memuat laporan roll-forward. Pastikan reportId valid dan
            calc run tersedia.
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
        <Button
          variant="outline"
          onClick={() => router.push("/ecl/roll-forward")}
        >
          Generate Laporan Baru
        </Button>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1 flex-wrap">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/ecl/roll-forward")}
            >
              Roll-Forward CKPN
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">
            {report.priorPeriodeId
              ? `${report.priorPeriodeId} → ${report.currentPeriodeId}`
              : report.currentPeriodeId}
          </li>
        </ol>
      </nav>

      {/* Sticky header */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div className="space-y-1">
          <h1 className="text-xl font-semibold">
            Roll-Forward CKPN
            {report.priorPeriodeId
              ? ` — ${report.priorPeriodeId} → ${report.currentPeriodeId}`
              : ` — ${report.currentPeriodeId} (Periode Pertama)`}
          </h1>
          <div className="flex items-center gap-3 flex-wrap text-sm text-muted-foreground">
            <span>Dihitung: {formatDateTime(report.computedAt)}</span>
            <span aria-hidden>·</span>
            <RollForwardDetectionMethodBadge method={report.detectionMethod} />
            <span aria-hidden>·</span>
            <ReconcileBadge
              status={reconcileStatusForBadge}
              tooltip={reconcileTooltip}
            />
          </div>
        </div>

        {can("ecl.roll_forward.export") && (
          <RollForwardExportButton
            reportId={report.reportId}
            reconcileStatus={report.reconcileStatus}
          />
        )}
      </div>

      {/* Alerts */}
      {report.reconcileStatus === "MISMATCH" && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" aria-hidden="true" />
          <AlertTitle>MISMATCH Status Rekonsiliasi</AlertTitle>
          <AlertDescription>
            Saldo Akhir roll-forward ({formatIDR(report.closingEclIdr)}) tidak
            sesuai total ECL calc run. Delta:{" "}
            <strong>{formatIDR(report.reconcileDeltaIdr)}</strong>. Toleransi:
            {formatIDR(report.reconcileTolerance)}. Investigasi diperlukan
            sebelum laporan digunakan secara formal.
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 ml-1 text-destructive underline"
              onClick={() =>
                router.push(
                  `/audit?entity_type=ecl.roll_forward&q=${report.currentCalcRunId}`,
                )
              }
            >
              Lihat Audit Trail
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {report.warnings.includes("ROLL_FORWARD_FIRST_PERIOD_OPENING_ZERO") && (
        <Alert className="border-blue-200 bg-blue-50">
          <Info className="h-4 w-4 text-blue-600" aria-hidden="true" />
          <AlertTitle className="text-blue-800">Periode Pertama</AlertTitle>
          <AlertDescription className="text-blue-700">
            Saldo Awal = 0. Semua instrumen dicatat sebagai Originasi Baru.
          </AlertDescription>
        </Alert>
      )}

      {/* Phase 5 limitation notice */}
      <Alert className="border-amber-200 bg-amber-50">
        <Info className="h-4 w-4 text-amber-600" aria-hidden="true" />
        <AlertTitle className="text-amber-800">Keterbatasan Phase 4</AlertTitle>
        <AlertDescription className="text-amber-700 text-sm">
          {report.phase5LimitationNote}
        </AlertDescription>
      </Alert>

      {/* Waterfall table — M11 full version with 6 buckets */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Waterfall CKPN</CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          <div className="overflow-x-auto">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b">
                  <th className="py-2 px-4 text-left font-medium text-muted-foreground">
                    Komponen
                  </th>
                  <th className="py-2 px-4 text-right font-medium text-muted-foreground">
                    Jumlah (IDR)
                  </th>
                  <th className="py-2 px-4 text-left font-medium text-muted-foreground w-32">
                    Ket
                  </th>
                </tr>
              </thead>
              <tbody>
                {/* Opening ECL */}
                <tr className="border-b bg-muted/20 font-semibold">
                  <td className="py-2.5 px-4 text-sm">Saldo Awal CKPN</td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs">
                    {formatIDR(report.openingEclIdr)}
                  </td>
                  <td className="py-2.5 px-4 text-xs text-muted-foreground">
                    (Opening)
                  </td>
                </tr>

                {/* Transfer buckets */}
                <TransferBucketRow
                  label="Penurunan/SICR (Stage 1 → 2)"
                  bucket={report.transfers.stage1To2}
                  sign="+"
                />
                <TransferBucketRow
                  label="Pemulihan/Cure (Stage 2 → 1)"
                  bucket={report.transfers.stage2To1}
                  sign="-"
                />
                <TransferBucketRow
                  label="Default (Stage 2 → 3)"
                  bucket={report.transfers.stage2To3}
                  sign="+"
                />
                <TransferBucketRow
                  label="Default Langsung (Stage 1 → 3)"
                  bucket={report.transfers.stage1To3}
                  sign="+"
                />
                <TransferBucketRow
                  label="Pemulihan Parsial (Stage 3 → 2)"
                  bucket={report.transfers.stage3To2}
                  sign="-"
                />
                <TransferBucketRow
                  label="Pemulihan Penuh (Stage 3 → 1)"
                  bucket={report.transfers.stage3To1}
                  sign="-"
                />

                {/* New Originations */}
                <tr className="border-b hover:bg-muted/20">
                  <td className="py-2.5 px-4 pl-8 text-sm text-red-700">
                    <span className="mr-1 font-mono text-xs">+</span>
                    Originasi Baru
                  </td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs text-red-700">
                    +{formatIDR(report.newOriginations.eclIdr)}
                  </td>
                  <td className="py-2.5 px-4">
                    <span className="text-xs text-muted-foreground">
                      {report.newOriginations.count} instr.
                    </span>
                  </td>
                </tr>

                {/* Derecognitions */}
                <tr className="border-b hover:bg-muted/20">
                  <td className="py-2.5 px-4 pl-8 text-sm text-green-700">
                    <span className="mr-1 font-mono text-xs">−</span>
                    Penghapusbukuan
                  </td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs text-green-700">
                    −{formatIDR(report.derecognitions.priorEclIdr)}
                  </td>
                  <td className="py-2.5 px-4">
                    <span className="text-xs text-muted-foreground">
                      {report.derecognitions.count} instr.
                    </span>
                  </td>
                </tr>

                {/* Remeasurements */}
                <tr className="border-b hover:bg-muted/20">
                  <td className="py-2.5 px-4 pl-8 text-sm text-muted-foreground">
                    <span className="mr-1 font-mono text-xs">±</span>
                    Pengukuran Ulang (Residual)
                  </td>
                  <td className="py-2.5 px-4 text-right font-mono text-xs">
                    {formatIDR(report.remeasurementsIdr)}
                  </td>
                  <td className="py-2.5 px-4" />
                </tr>

                {/* Closing ECL */}
                <tr className="border-t-2 bg-muted/40 font-bold">
                  <td className="py-3 px-4">
                    <span className="text-sm">= Saldo Akhir CKPN</span>
                  </td>
                  <td className="py-3 px-4 text-right font-mono">
                    {formatIDR(report.closingEclIdr)}
                  </td>
                  <td className="py-3 px-4">
                    <ReconcileBadge
                      status={reconcileStatusForBadge}
                      tooltip={reconcileTooltip}
                      className="text-xs"
                    />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </CardContent>
      </Card>

      {/* Reconcile delta detail */}
      <div className="flex items-center gap-3 text-sm">
        <span className="text-muted-foreground">Status Rekonsiliasi:</span>
        <ReconcileBadge
          status={reconcileStatusForBadge}
          tooltip={reconcileTooltip}
        />
        {report.reconcileDeltaIdr && report.reconcileDeltaIdr !== "0.0000" && (
          <Badge variant="outline" className="font-mono text-xs">
            Delta: {formatIDR(report.reconcileDeltaIdr)}
          </Badge>
        )}
      </div>

      {/* Data quality warnings */}
      {report.warnings.includes("ROLL_FORWARD_HAS_DATA_QUALITY_WARNINGS") &&
        report.dataQualityWarnings &&
        report.dataQualityWarnings.length > 0 && (
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base flex items-center gap-2">
                <AlertTriangle className="h-4 w-4 text-amber-500" aria-hidden="true" />
                Peringatan Kualitas Data ({report.dataQualityWarnings.length})
              </CardTitle>
            </CardHeader>
            <CardContent>
              <ul className="space-y-1.5">
                {report.dataQualityWarnings.map((w, i) => (
                  <li key={i} className="text-sm text-amber-800 bg-amber-50 rounded px-3 py-2">
                    <span className="font-mono text-xs mr-2 text-amber-600">
                      {w.warningCode}
                    </span>
                    {w.instrumenKode && (
                      <span className="font-medium mr-1">[{w.instrumenKode}]</span>
                    )}
                    {w.message}
                  </li>
                ))}
              </ul>
            </CardContent>
          </Card>
        )}

      {/* Per-portfolio breakdown DataTable */}
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Breakdown per Portofolio</CardTitle>
        </CardHeader>
        <CardContent className="p-0 pb-4">
          <DataTable
            columns={portofolioColumns}
            data={[]}
            isLoading={false}
            emptyMessage="Tidak ada data portofolio tersedia"
          />
        </CardContent>
      </Card>
    </div>
  );
}
