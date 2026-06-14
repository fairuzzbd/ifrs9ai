"use client";

import * as React from "react";
import { AlertTriangle, Info, ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { cn } from "@/lib/utils";
import { format } from "date-fns";
import type { EirPreviewResult } from "@/lib/schemas/penempatan.schema";
import type { PenempatanWorkflowStatus, KlasifikasiPsak71 } from "@/lib/schemas/penempatan.schema";
import { isFvtpl } from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatIdr(value: number): string {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(value);
}

function formatRate(value: number): string {
  return `${(value * 100).toFixed(8)}%`;
}

function formatDate(s: string): string {
  try {
    return format(new Date(s), "d MMM yyyy");
  } catch {
    return s;
  }
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface EIRPreviewSidePanelProps {
  workflowStatus: PenempatanWorkflowStatus;
  klasifikasiPsak71?: KlasifikasiPsak71 | null;
  eirAwal?: number | null;
  eirPreviewResult?: EirPreviewResult | null;
  eirPreviewLoading?: boolean;
  eirComputeJobId?: string | null;
  onRequestPreview?: () => void;
  onEirJobComplete?: (result: unknown) => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Amortization table
// ---------------------------------------------------------------------------

function AmortizationTable({ items }: { items: EirPreviewResult["amortizationSchedule"] }) {
  if (!items || items.length === 0) return null;

  return (
    <div className="overflow-x-auto mt-3">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b text-gray-500">
            <th className="py-1 text-left">Per.</th>
            <th className="py-1 text-left">Tgl Angsuran</th>
            <th className="py-1 text-right">Ang. Bunga</th>
            <th className="py-1 text-right">Ang. Pokok</th>
            <th className="py-1 text-right">Carrying Amt</th>
          </tr>
        </thead>
        <tbody>
          {items.map((row) => (
            <tr key={row.periode} className="border-b last:border-0">
              <td className="py-1">{row.periode}</td>
              <td className="py-1">{formatDate(row.tanggalAngsuran)}</td>
              <td className="py-1 text-right font-mono">
                {formatIdr(row.angsuranBunga)}
              </td>
              <td className="py-1 text-right font-mono">
                {formatIdr(row.angsuranPokok)}
              </td>
              <td className="py-1 text-right font-mono">
                {formatIdr(row.carryingAmount)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function EIRPreviewSidePanel({
  workflowStatus,
  klasifikasiPsak71,
  eirAwal,
  eirPreviewResult,
  eirPreviewLoading,
  eirComputeJobId,
  onRequestPreview,
  onEirJobComplete,
  className,
}: EIRPreviewSidePanelProps) {
  const [scheduleExpanded, setScheduleExpanded] = React.useState(false);

  // FVTPL: informational only
  if (isFvtpl(klasifikasiPsak71)) {
    return (
      <Card className={cn("border-blue-200 bg-blue-50", className)}>
        <CardContent className="pt-4 pb-3">
          <div className="flex items-start gap-2 text-blue-700">
            <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p className="text-sm">
              EIR tidak dihitung untuk instrumen FVTPL/FVOCI Election. Fair value akan
              diproses oleh MTM engine (P5-M6).
            </p>
          </div>
        </CardContent>
      </Card>
    );
  }

  const isApproved =
    workflowStatus === "APPROVED_ACTIVE" ||
    workflowStatus === "TERMINATED" ||
    workflowStatus === "MATURED";

  return (
    <Card className={className}>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">EIR Preview</CardTitle>
      </CardHeader>
      <CardContent className="pb-4 space-y-3">

        {/* Final EIR (post-approve) */}
        {isApproved && eirAwal != null && (
          <div>
            <p className="text-xs text-gray-500">EIR Awal (Final)</p>
            <p className="text-lg font-semibold text-green-700">
              {formatRate(eirAwal)}
            </p>
            <span className="inline-block mt-1 rounded-full bg-green-100 px-2 py-0.5 text-xs text-green-700">
              Final
            </span>
          </div>
        )}

        {/* EIR compute running */}
        {isApproved && eirAwal == null && eirComputeJobId && (
          <div className="space-y-2">
            <p className="text-xs text-gray-500">EIR sedang dihitung...</p>
            <JobProgressPanel
              jobId={eirComputeJobId}
              title="Menghitung EIR"
              onComplete={onEirJobComplete}
              variant="inline"
            />
          </div>
        )}

        {/* Draft/pending: show preview or button to compute */}
        {!isApproved && (
          <div className="space-y-2">
            {eirPreviewResult?.eirAwalApprox != null && (
              <div>
                <p className="text-xs text-gray-500">EIR (Estimasi)</p>
                <p className="text-lg font-semibold text-amber-700">
                  {formatRate(eirPreviewResult.eirAwalApprox)}
                </p>
                <div className="flex items-center gap-1 mt-1">
                  <span className="inline-block rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700">
                    Estimasi
                  </span>
                  <AlertTriangle className="h-3 w-3 text-amber-500" aria-hidden="true" />
                </div>
                <p className="text-xs text-amber-600 mt-1">
                  Ini estimasi. EIR final dihitung setelah approve.
                </p>
              </div>
            )}

            {eirPreviewResult?.info && (
              <p className="text-xs text-gray-500">{eirPreviewResult.info}</p>
            )}

            {onRequestPreview && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="w-full"
                onClick={onRequestPreview}
                disabled={eirPreviewLoading}
                aria-label="Hitung EIR Preview"
              >
                {eirPreviewLoading ? "Menghitung..." : "Hitung EIR Preview"}
              </Button>
            )}
          </div>
        )}

        {/* Amortization schedule accordion */}
        {eirPreviewResult?.amortizationSchedule &&
          eirPreviewResult.amortizationSchedule.length > 0 && (
            <div>
              <button
                type="button"
                className="flex w-full items-center justify-between text-xs text-blue-600 hover:underline"
                onClick={() => setScheduleExpanded((v) => !v)}
                aria-expanded={scheduleExpanded}
                aria-controls="amortization-table"
              >
                <span>Jadwal Amortisasi — {eirPreviewResult.periodePreview} Periode Pertama</span>
                {scheduleExpanded ? (
                  <ChevronDown className="h-4 w-4" aria-hidden="true" />
                ) : (
                  <ChevronRight className="h-4 w-4" aria-hidden="true" />
                )}
              </button>
              {scheduleExpanded && (
                <div id="amortization-table">
                  <AmortizationTable items={eirPreviewResult.amortizationSchedule} />
                </div>
              )}
            </div>
          )}
      </CardContent>
    </Card>
  );
}
