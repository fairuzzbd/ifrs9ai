"use client";

import * as React from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useQueryState } from "nuqs";
import { format } from "date-fns";
import { id as localeId } from "date-fns/locale";
import {
  CalendarIcon,
  RefreshCw,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Clock,
  Download,
  Play,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { glReconciliationApi } from "@/lib/api/gl-delivery.api";
import type { GlReconMismatchLine } from "@/lib/schemas/gl-delivery.schema";
import { usePermissions } from "@/lib/stores/auth.store";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { v4 as uuidv4 } from "uuid";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  maximumFractionDigits: 0,
});

function formatIdr(n: number | null | undefined): string {
  if (n == null) return "—";
  return IDR.format(n);
}

const MISMATCH_LABELS: Record<string, string> = {
  MISSING_IN_GL: "Tidak Ada di GL",
  AMOUNT_DIFF: "Selisih Nominal",
  EXTRA_IN_GL: "Extra di GL",
};

const MISMATCH_ROW_CLASS: Record<string, string> = {
  MISSING_IN_GL: "bg-yellow-50 dark:bg-yellow-950/20",
  AMOUNT_DIFF: "bg-orange-50 dark:bg-orange-950/20",
  EXTRA_IN_GL: "bg-red-50 dark:bg-red-950/20",
};

const MISMATCH_BADGE_VARIANT: Record<string, string> = {
  MISSING_IN_GL: "warning",
  AMOUNT_DIFF: "secondary",
  EXTRA_IN_GL: "destructive",
};

const STATUS_LABELS: Record<string, string> = {
  MATCHED: "Cocok",
  MISMATCH: "Tidak Cocok",
  PENDING: "Menunggu",
  ERROR: "Error",
};

const STATUS_COLORS: Record<string, string> = {
  MATCHED: "text-green-700",
  MISMATCH: "text-red-700",
  PENDING: "text-amber-600",
  ERROR: "text-destructive",
};

// ---------------------------------------------------------------------------
// KPI Card
// ---------------------------------------------------------------------------

interface KpiCardProps {
  title: string;
  value: string | number;
  subtitle?: string;
  icon: React.ReactNode;
  alert?: boolean;
  className?: string;
}

function KpiCard({ title, value, subtitle, icon, alert, className }: KpiCardProps) {
  return (
    <Card className={cn(alert && "border-destructive/50 bg-destructive/5", className)}>
      <CardContent className="p-4 flex items-start gap-3">
        <div
          className={cn(
            "mt-0.5 flex-shrink-0",
            alert ? "text-destructive" : "text-muted-foreground",
          )}
          aria-hidden="true"
        >
          {icon}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-xs text-muted-foreground truncate">{title}</p>
          <p
            className={cn(
              "text-xl font-semibold tabular-nums",
              alert && "text-destructive",
            )}
          >
            {value}
          </p>
          {subtitle && (
            <p className="text-xs text-muted-foreground mt-0.5">{subtitle}</p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Mismatch table
// ---------------------------------------------------------------------------

function MismatchTable({ lines }: { lines: GlReconMismatchLine[] }) {
  if (lines.length === 0) return null;

  return (
    <div className="rounded-md border overflow-hidden">
      <Table>
        <TableHeader className="sticky top-0 bg-background z-10">
          <TableRow>
            <TableHead className="w-40">Kode Akun</TableHead>
            <TableHead>Nama Akun</TableHead>
            <TableHead className="text-right w-44">Nominal BLIPS (IDR)</TableHead>
            <TableHead className="text-right w-44">Nominal GL (IDR)</TableHead>
            <TableHead className="text-right w-40">Selisih (IDR)</TableHead>
            <TableHead className="w-40">Jenis Mismatch</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {lines.map((line, idx) => (
            <TableRow
              key={`${line.kodeAkun}-${idx}`}
              className={cn(
                MISMATCH_ROW_CLASS[line.mismatchType] ?? "",
              )}
            >
              <TableCell className="font-mono text-xs">{line.kodeAkun}</TableCell>
              <TableCell className="text-sm">
                {line.namaAkun ?? (
                  <span className="text-muted-foreground">—</span>
                )}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatIdr(line.blipsAmountIdr)}
              </TableCell>
              <TableCell className="text-right font-mono text-sm">
                {formatIdr(line.glHostAmountIdr)}
              </TableCell>
              <TableCell
                className={cn(
                  "text-right font-mono text-sm font-medium",
                  line.deltaIdr !== 0 && "text-destructive",
                )}
              >
                {formatIdr(line.deltaIdr)}
              </TableCell>
              <TableCell>
                <Badge
                  variant={
                    (MISMATCH_BADGE_VARIANT[line.mismatchType] as
                      | "default"
                      | "secondary"
                      | "destructive"
                      | "outline") ?? "outline"
                  }
                  className="text-xs"
                >
                  {MISMATCH_LABELS[line.mismatchType] ?? line.mismatchType}
                </Badge>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main page client
// ---------------------------------------------------------------------------

export function ReconciliationDailyPageClient() {
  const { can } = usePermissions();

  const today = format(new Date(), "yyyy-MM-dd");
  const [tanggal, setTanggal] = useQueryState("tanggal", { defaultValue: today });

  const [runJobId, setRunJobId] = React.useState<string | null>(null);
  const [calendarOpen, setCalendarOpen] = React.useState(false);

  const selectedDate = new Date(tanggal + "T00:00:00");

  const {
    data,
    isFetching,
    isError,
    refetch,
  } = useQuery({
    queryKey: ["reconciliation-daily", tanggal],
    queryFn: () => glReconciliationApi.getDaily(tanggal),
    enabled: !!tanggal,
    retry: false,
  });

  const report = data?.data;

  const runMutation = useMutation({
    mutationFn: () =>
      glReconciliationApi.run(
        { date: tanggal },
        uuidv4(),
      ),
    onSuccess: (res) => {
      const jobId = res.data?.jobId;
      if (jobId) {
        setRunJobId(jobId);
      } else {
        notify.success(`Rekonsiliasi ${tanggal} berhasil dijalankan.`);
        void refetch();
      }
    },
    onError: (err) => {
      if (isApiError(err)) notify.error(err);
    },
  });

  const canRun = can("jurnal.reconciliation.run");

  // Export handler — CSV download
  const handleExport = (format: "csv" | "xlsx") => {
    const url = glReconciliationApi.exportUrl({
      format,
      "filter[tanggal_from]": tanggal,
      "filter[tanggal_to]": tanggal,
    });
    window.open(url, "_blank", "noopener,noreferrer");
  };

  return (
    <TooltipProvider>
      <div className="flex flex-col gap-6 px-6 py-6">
        {/* Page header */}
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">Rekonsiliasi Harian</h1>
            <p className="text-sm text-muted-foreground">
              Perbandingan jurnal BLIPS vs. GL Host per tanggal posting
            </p>
          </div>

          {/* Controls */}
          <div className="flex flex-wrap items-center gap-2">
            {/* Date picker */}
            <Popover open={calendarOpen} onOpenChange={setCalendarOpen}>
              <PopoverTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 gap-2 min-w-[150px]"
                  aria-label="Pilih tanggal rekonsiliasi"
                >
                  <CalendarIcon className="h-4 w-4" aria-hidden="true" />
                  {format(selectedDate, "d MMMM yyyy", { locale: localeId })}
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-auto p-0" align="end">
                <Calendar
                  mode="single"
                  selected={selectedDate}
                  onSelect={(d) => {
                    if (d) {
                      void setTanggal(format(d, "yyyy-MM-dd"));
                      setCalendarOpen(false);
                    }
                  }}
                  disabled={(d) => d > new Date()}
                  initialFocus
                />
              </PopoverContent>
            </Popover>

            {/* Refresh */}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 w-8 p-0"
                  onClick={() => void refetch()}
                  disabled={isFetching}
                  aria-label="Muat ulang data rekonsiliasi"
                >
                  <RefreshCw
                    className={cn("h-4 w-4", isFetching && "animate-spin")}
                    aria-hidden="true"
                  />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Muat ulang</TooltipContent>
            </Tooltip>

            {/* Export */}
            <Popover>
              <PopoverTrigger asChild>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 gap-1"
                  aria-label="Ekspor laporan rekonsiliasi"
                >
                  <Download className="h-4 w-4" aria-hidden="true" />
                  Ekspor
                </Button>
              </PopoverTrigger>
              <PopoverContent className="w-36 p-1" align="end">
                <div className="flex flex-col gap-0.5">
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-full justify-start text-xs"
                    onClick={() => handleExport("csv")}
                  >
                    CSV
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="h-8 w-full justify-start text-xs"
                    onClick={() => handleExport("xlsx")}
                  >
                    Excel (XLSX)
                  </Button>
                </div>
              </PopoverContent>
            </Popover>

            {/* Run reconciliation — AKUN-CTL only */}
            {canRun && (
              <Button
                size="sm"
                className="h-8 gap-1"
                onClick={() => runMutation.mutate()}
                disabled={runMutation.isPending || !!runJobId}
                aria-label="Jalankan rekonsiliasi untuk tanggal ini"
              >
                <Play className="h-4 w-4" aria-hidden="true" />
                Jalankan
              </Button>
            )}
          </div>
        </div>

        {/* Job progress (run in-flight) */}
        {runJobId && (
          <JobProgressPanel
            jobId={runJobId}
            title={`Rekonsiliasi ${tanggal}`}
            showCancel={false}
            showBackground={false}
            onComplete={() => {
              notify.success(
                `Rekonsiliasi ${tanggal} selesai.`,
                {
                  action: {
                    label: "Muat ulang",
                    onClick: () => void refetch(),
                  },
                },
              );
              setRunJobId(null);
              void refetch();
            }}
            onFail={(err) => {
              if (isApiError(err)) notify.error(err);
              setRunJobId(null);
            }}
          />
        )}

        {/* Loading skeleton */}
        {isFetching && !report && (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Card key={i}>
                <CardContent className="p-4">
                  <div className="h-3 w-24 animate-pulse rounded bg-muted mb-2" />
                  <div className="h-7 w-16 animate-pulse rounded bg-muted" />
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {/* Error state */}
        {isError && !isFetching && (
          <div className="flex flex-col items-center py-16 gap-3">
            <XCircle className="h-10 w-10 text-muted-foreground" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">
              Laporan rekonsiliasi untuk tanggal ini belum tersedia atau terjadi kesalahan.
            </p>
            <Button variant="outline" size="sm" onClick={() => void refetch()}>
              Coba Lagi
            </Button>
          </div>
        )}

        {/* Report content */}
        {report && (
          <>
            {/* Status banner */}
            <div
              className={cn(
                "flex items-center gap-2 rounded-md border px-4 py-2 text-sm",
                report.status === "MATCHED"
                  ? "border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/30 dark:text-green-300"
                  : report.status === "MISMATCH"
                  ? "border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-300"
                  : "border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-300",
              )}
              role="status"
              aria-live="polite"
            >
              {report.status === "MATCHED" ? (
                <CheckCircle2 className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
              ) : report.status === "MISMATCH" ? (
                <XCircle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
              ) : (
                <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
              )}
              <span>
                Status rekonsiliasi{" "}
                <strong>{format(selectedDate, "d MMMM yyyy", { locale: localeId })}</strong>:{" "}
                <strong className={STATUS_COLORS[report.status] ?? ""}>
                  {STATUS_LABELS[report.status] ?? report.status}
                </strong>
              </span>
              <span className="ml-auto flex items-center gap-1 text-xs opacity-70">
                <Clock className="h-3 w-3" aria-hidden="true" />
                Dibuat:{" "}
                {new Date(report.generatedAt).toLocaleString("id-ID", {
                  timeZone: "Asia/Jakarta",
                  dateStyle: "short",
                  timeStyle: "short",
                })}
              </span>
            </div>

            {/* KPI cards */}
            <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <KpiCard
                title="Total Akun Diperiksa"
                value={report.totalAkunChecked.toLocaleString("id-ID")}
                icon={<CheckCircle2 className="h-5 w-5" />}
              />
              <KpiCard
                title="Total Mismatch"
                value={report.totalMismatchCount.toLocaleString("id-ID")}
                icon={<AlertTriangle className="h-5 w-5" />}
                alert={report.totalMismatchCount > 0}
              />
              <KpiCard
                title="Total BLIPS (IDR)"
                value={formatIdr(report.blipsTotalIdr)}
                icon={<CheckCircle2 className="h-5 w-5" />}
              />
              <KpiCard
                title="Selisih BLIPS vs GL (IDR)"
                value={formatIdr(report.deltaIdr)}
                icon={<XCircle className="h-5 w-5" />}
                alert={report.deltaIdr !== 0}
                subtitle={
                  report.toleranceIdr != null
                    ? `Toleransi: ${formatIdr(report.toleranceIdr)}`
                    : undefined
                }
              />
            </div>

            {/* Mismatch detail table */}
            {(report.mismatchLines?.length ?? 0) > 0 ? (
              <section aria-labelledby="mismatch-heading">
                <div className="mb-3 flex items-center justify-between">
                  <h2 id="mismatch-heading" className="text-base font-semibold">
                    Detail Mismatch
                    <Badge
                      variant="destructive"
                      className="ml-2 text-xs"
                      aria-label={`${report.mismatchLines!.length} baris mismatch`}
                    >
                      {report.mismatchLines!.length}
                    </Badge>
                  </h2>
                  <div className="flex gap-1.5 text-xs text-muted-foreground">
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-sm bg-yellow-200 dark:bg-yellow-800" aria-hidden="true" />
                      Tidak Ada di GL
                    </span>
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-sm bg-orange-200 dark:bg-orange-800" aria-hidden="true" />
                      Selisih Nominal
                    </span>
                    <span className="flex items-center gap-1">
                      <span className="inline-block h-2.5 w-2.5 rounded-sm bg-red-200 dark:bg-red-800" aria-hidden="true" />
                      Extra di GL
                    </span>
                  </div>
                </div>
                <MismatchTable lines={report.mismatchLines!} />
              </section>
            ) : (
              report.status === "MATCHED" && (
                <div className="flex flex-col items-center py-12 gap-2 text-sm text-muted-foreground">
                  <CheckCircle2 className="h-10 w-10 text-green-500" aria-hidden="true" />
                  <p>Semua jurnal cocok. Tidak ada mismatch untuk tanggal ini.</p>
                </div>
              )
            )}

            {/* Meta footer */}
            <div className="text-xs text-muted-foreground flex flex-wrap gap-4 pt-2 border-t">
              <span>
                Report ID: <span className="font-mono">{report.reportId}</span>
              </span>
              {report.jobId && (
                <span>
                  Job ID: <span className="font-mono">{report.jobId}</span>
                </span>
              )}
              <span>
                Total GL Host (IDR): <span className="font-mono">{formatIdr(report.glHostTotalIdr)}</span>
              </span>
            </div>
          </>
        )}
      </div>
    </TooltipProvider>
  );
}
