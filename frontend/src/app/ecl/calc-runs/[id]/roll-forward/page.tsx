"use client";

import * as React from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";

import { eclCoreApi } from "@/lib/api/ecl-core.api";
import { calcRunApi } from "@/lib/api/calc-run.api";
import { RollForwardWaterfall } from "@/components/blips/RollForwardWaterfall";
import { ReconcileBadge } from "@/components/blips/ReconcileBadge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { usePermissions } from "@/lib/stores/auth.store";
import { AlertTriangle } from "lucide-react";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function RollForwardPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { can } = usePermissions();

  const calcRunId = params.id;
  const defaultPrior = searchParams.get("priorCalcRunId") ?? null;

  const [priorRunId, setPriorRunId] = React.useState<string | null>(defaultPrior);
  const [exportConfirmOpen, setExportConfirmOpen] = React.useState<"csv" | "xlsx" | null>(null);

  const { data: rollData, isLoading } = useQuery({
    queryKey: ["roll-forward", calcRunId, priorRunId],
    queryFn: () => eclCoreApi.getRollForward(calcRunId, { priorCalcRunId: priorRunId ?? undefined }),
    enabled: !!priorRunId,
  });

  const report = rollData?.data;

  // Prior runs (COMPLETED or SEALED from earlier periods)
  const { data: priorRunsData } = useQuery({
    queryKey: ["prior-runs-completed-sealed"],
    queryFn: () =>
      calcRunApi.list({
        limit: 100,
        sort: "created_at:desc",
      }),
  });
  const availablePriorRuns = (priorRunsData?.data ?? []).filter(
    (r) =>
      r.id !== calcRunId &&
      (r.status === "COMPLETED" || r.status === "SEALED"),
  );

  const handleExport = (format: "csv" | "xlsx") => {
    if (report?.reconcileStatus === "MISMATCH") {
      setExportConfirmOpen(format);
    } else {
      eclCoreApi.exportRollForward(calcRunId, priorRunId, format);
    }
  };

  const reconcileTooltip = report?.reconcileStatus === "RECONCILED"
    ? `Closing matches ECL total calc run. Selisih: ${formatIDR(report.selisihIdr ?? "0")}.`
    : report?.reconcileStatus === "MISMATCH"
      ? `Closing roll-forward (${formatIDR(report.closingIdr)}) ≠ ECL total calc run (${formatIDR(report.eclTotalCalcRunIdr)}). Selisih: ${formatIDR(report.selisihIdr)}.`
      : "Laporan ini bersifat partial karena beberapa komponen belum tersedia (Phase 5).";

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1 flex-wrap">
          <li>
            <button className="hover:underline" onClick={() => router.push("/ecl/calc-runs")}>
              Calc Runs
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push(`/ecl/calc-runs/${calcRunId}`)}
            >
              {calcRunId}
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">Roll Forward</li>
        </ol>
      </nav>

      {/* Page header */}
      <div className="flex items-start justify-between gap-4 flex-wrap">
        <div>
          <h1 className="text-xl font-semibold">
            Roll Forward CKPN
            {report?.priorPeriodeLabel && `: ${report.priorPeriodeLabel} → ${report.periodeLabel ?? calcRunId}`}
          </h1>
          <div className="flex items-center gap-3 mt-2 flex-wrap">
            <label className="text-sm text-muted-foreground">Prior Run:</label>
            <Select
              value={priorRunId ?? "_none"}
              onValueChange={(v) => setPriorRunId(v === "_none" ? null : v)}
            >
              <SelectTrigger className="w-72" aria-label="Pilih prior calc run">
                <SelectValue placeholder="Pilih prior run..." />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_none">Pilih prior run...</SelectItem>
                {availablePriorRuns.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.id} ({r.periodeLabel ?? r.periodeId}, {r.status})
                  </SelectItem>
                ))}
                {availablePriorRuns.length === 0 && (
                  <SelectItem value="_empty" disabled>
                    Tidak ada run COMPLETED/SEALED
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
            {report && (
              <ReconcileBadge
                status={report.reconcileStatus}
                tooltip={reconcileTooltip}
              />
            )}
          </div>
        </div>
        {can("ecl.result.export") && report && (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => handleExport("csv")}>
              Export CSV
            </Button>
            <Button variant="outline" size="sm" onClick={() => handleExport("xlsx")}>
              Export XLSX
            </Button>
          </div>
        )}
      </div>

      {/* No prior run state */}
      {!priorRunId && (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            Roll-forward tidak tersedia: pilih prior calc run dari dropdown di atas.
          </CardContent>
        </Card>
      )}

      {/* No data after selection */}
      {priorRunId && !isLoading && !report && (
        <Card>
          <CardContent className="py-8 text-center text-muted-foreground">
            Roll-forward tidak tersedia: tidak ada calc run dari periode sebelumnya yang
            COMPLETED atau SEALED.
          </CardContent>
        </Card>
      )}

      {/* MISMATCH alert */}
      {report?.reconcileStatus === "MISMATCH" && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" aria-hidden="true" />
          <AlertTitle>MISMATCH Rekonsiliasi</AlertTitle>
          <AlertDescription>
            Closing roll-forward ({formatIDR(report.closingIdr)}) tidak sama dengan ECL
            total calc run ({formatIDR(report.eclTotalCalcRunIdr)}). Selisih:{" "}
            {formatIDR(report.selisihIdr)}. Investigasi diperlukan sebelum seal.
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 ml-1 text-destructive underline"
              onClick={() =>
                router.push(`/audit?entity_type=ecl.roll_forward&entity_id=${calcRunId}`)
              }
            >
              Lihat Audit Trail
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* PARTIAL info */}
      {report?.reconcileStatus === "PARTIAL_PHASE_5_DEFER" && (
        <Alert className="border-amber-200 bg-amber-50">
          <AlertTitle className="text-amber-800">Partial — Fase 5 Defer</AlertTitle>
          <AlertDescription className="text-amber-700">
            Transfer antar stage dan remeasurements akan tersedia setelah Phase 5
            (GL/jurnal engine) selesai. Laporan ini bersifat partial.
          </AlertDescription>
        </Alert>
      )}

      {/* Waterfall table */}
      {report && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Waterfall CKPN</CardTitle>
          </CardHeader>
          <CardContent className="p-0 pb-4">
            <RollForwardWaterfall components={report.components} />
          </CardContent>
          <CardContent className="pt-0 pb-3">
            <ReconcileBadge
              status={report.reconcileStatus}
              tooltip={reconcileTooltip}
              className="text-sm"
            />
          </CardContent>
        </Card>
      )}

      {/* Per-portfolio breakdown */}
      {report?.portofolioBreakdown && report.portofolioBreakdown.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Breakdown per Portofolio</CardTitle>
          </CardHeader>
          <CardContent className="p-0 pb-4">
            <div className="divide-y">
              {report.portofolioBreakdown.map((pb) => (
                <Collapsible key={pb.portofolioId}>
                  <CollapsibleTrigger className="flex items-center justify-between w-full px-4 py-3 text-left hover:bg-muted/20 transition-colors">
                    <span className="font-medium">
                      {pb.portofolioNama ?? pb.portofolioId}
                    </span>
                    <span className="text-sm text-muted-foreground">
                      {formatIDR(pb.openingIdr)} → {formatIDR(pb.closingIdr)}
                    </span>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="px-4 pb-3">
                    <div className="flex gap-4 text-sm">
                      <span>Opening: <strong>{formatIDR(pb.openingIdr)}</strong></span>
                      <span>Closing: <strong>{formatIDR(pb.closingIdr)}</strong></span>
                    </div>
                    <Button
                      variant="link"
                      size="sm"
                      className="h-auto p-0 mt-1"
                      onClick={() =>
                        router.push(
                          `/ecl/calc-runs/${calcRunId}/portofolio/${pb.portofolioId}/summary`,
                        )
                      }
                    >
                      Lihat Detail
                    </Button>
                  </CollapsibleContent>
                </Collapsible>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Export MISMATCH confirm dialog */}
      <AlertDialog
        open={!!exportConfirmOpen}
        onOpenChange={(open) => { if (!open) setExportConfirmOpen(null); }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Ekspor dengan Status MISMATCH?</AlertDialogTitle>
            <AlertDialogDescription>
              Laporan ini memiliki mismatch rekonsiliasi. Ekspor tetap tersedia untuk
              investigasi. Lanjutkan?
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setExportConfirmOpen(null)}>
              Batal
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (exportConfirmOpen) {
                  eclCoreApi.exportRollForward(calcRunId, priorRunId, exportConfirmOpen);
                }
                setExportConfirmOpen(null);
              }}
            >
              Ekspor Tetap
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
