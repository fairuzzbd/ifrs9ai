"use client";

import * as React from "react";
import { Info, TrendingUp, TrendingDown, Minus } from "lucide-react";
import { cn } from "@/lib/utils";
import type { PenjualanPreview } from "@/lib/schemas/penjualan.schema";
import { PenjualanRoutingBadge } from "./PenjualanRoutingBadge";
import type { KlasifikasiPsak71, JenisDisposal } from "@/lib/schemas/penjualan.schema";

// ---------------------------------------------------------------------------
// IDR formatter (full precision for detail views)
// ---------------------------------------------------------------------------

const IDR_DETAIL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

function formatIDR(value: string): string {
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return IDR_DETAIL.format(num);
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PenjualanPreviewPanelProps {
  preview: PenjualanPreview;
  jenisDisposal?: JenisDisposal;
  className?: string;
}

// ---------------------------------------------------------------------------
// Realized G/L icon
// ---------------------------------------------------------------------------

function GLIcon({ value }: { value: string }) {
  const num = parseFloat(value);
  if (isNaN(num) || num === 0) return <Minus className="h-4 w-4 text-gray-400" aria-hidden="true" />;
  if (num > 0) return <TrendingUp className="h-4 w-4 text-green-600" aria-hidden="true" />;
  return <TrendingDown className="h-4 w-4 text-red-500" aria-hidden="true" />;
}

// ---------------------------------------------------------------------------
// Component (S1 — preview computed by server)
// ---------------------------------------------------------------------------

export function PenjualanPreviewPanel({
  preview,
  jenisDisposal,
  className,
}: PenjualanPreviewPanelProps) {
  const isFVOCIElection = preview.klasifikasiPsak71 === "FVOCI_ELECTION";
  const hasBMWarning = !!preview.bmFreqWarning;

  return (
    <div
      className={cn("rounded-md border bg-slate-50 p-4 space-y-3", className)}
      aria-label="Kalkulasi preview penjualan"
    >
      <h3 className="text-sm font-semibold text-slate-700">Preview Kalkulasi Penjualan</h3>

      {/* Klasifikasi + routing */}
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-xs text-muted-foreground">Klasifikasi:</span>
        <span className="text-xs font-mono font-medium">{preview.klasifikasiPsak71}</span>
        {jenisDisposal && (
          <PenjualanRoutingBadge
            klasifikasi={preview.klasifikasiPsak71 as KlasifikasiPsak71}
            jenisDisposal={jenisDisposal}
          />
        )}
      </div>

      {/* Financial summary */}
      <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
        <dt className="text-muted-foreground">Proceeds IDR</dt>
        <dd className="font-mono font-medium text-right">{formatIDR(preview.proceedIdr)}</dd>

        <dt className="text-muted-foreground">Cost Basis</dt>
        <dd className="font-mono text-right">{formatIDR(preview.costBasis)}</dd>

        <dt className="flex items-center gap-1 text-muted-foreground">
          <GLIcon value={preview.realizedGl} />
          Realized G/L
          {isFVOCIElection && (
            <span className="text-xs text-orange-600 ml-1">(informasional, tetap di OCI)</span>
          )}
        </dt>
        <dd
          className={cn(
            "font-mono font-semibold text-right",
            parseFloat(preview.realizedGl) > 0 ? "text-green-700" : "text-red-600",
          )}
        >
          {formatIDR(preview.realizedGl)}
        </dd>
      </dl>

      {/* OCI recycling info */}
      {preview.ociRecycled && (
        <div className="rounded border border-teal-200 bg-teal-50 px-3 py-2 text-xs text-teal-800">
          <strong>OCI Recycle ke P&L:</strong> {formatIDR(preview.ociRecycled)} via jurnal REKLAS_OCI_PL
        </div>
      )}

      {/* FVOCI Election no-recycling note */}
      {preview.noRecyclingNote && (
        <div className="rounded border border-slate-200 bg-slate-100 px-3 py-2 text-xs text-slate-700 flex gap-2">
          <Info className="h-4 w-4 shrink-0 text-slate-500 mt-0.5" aria-hidden="true" />
          <span>{preview.noRecyclingNote}</span>
        </div>
      )}

      {/* BM frequency impact */}
      {preview.bmFreqImpactPct && (
        <div
          className={cn(
            "rounded border px-3 py-2 text-xs flex gap-2",
            hasBMWarning
              ? "border-orange-300 bg-orange-50 text-orange-800"
              : "border-gray-200 bg-gray-50 text-gray-700",
          )}
        >
          <Info className="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
          <div>
            <div>
              <strong>BM HTC Frequency Impact:</strong> {preview.bmFreqImpactPct}% kumulatif 12-bulan
            </div>
            {preview.bmFreqWarning && (
              <div className="mt-0.5 text-orange-700">{preview.bmFreqWarning}</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
