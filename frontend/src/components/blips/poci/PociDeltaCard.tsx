"use client";

import * as React from "react";
import { PociDirectionBadge } from "./PociDirectionBadge";
import { PociBaselineImmutableBadge } from "./PociBaselineImmutableBadge";
import { cn } from "@/lib/utils";
import type { PociDeltaLogItem } from "@/lib/schemas/poci.schema";

const IDR_FULL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

const IDR_SHORT = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function formatIDR(val: string | null | undefined, full = false): string {
  if (!val) return "—";
  const n = parseFloat(val);
  if (isNaN(n)) return "—";
  return full ? IDR_FULL.format(n) : IDR_SHORT.format(n);
}

export interface PociDeltaCardProps {
  data: PociDeltaLogItem;
  compact?: boolean;
  className?: string;
}

/**
 * Shows current ECL vs baseline vs delta cumulative per instrumen per calc run.
 * Large delta flag renders a red warning per S5-AC3.
 */
export function PociDeltaCard({ data, compact = false, className }: PociDeltaCardProps) {
  const deltaPositive = parseFloat(data.deltaEcl) > 0;
  const deltaNegative = parseFloat(data.deltaEcl) < 0;

  return (
    <div
      className={cn(
        "rounded-lg border bg-card p-4 space-y-3",
        data.largeDeltaFlag && "border-red-400",
        className,
      )}
      aria-label={`POCI delta card — instrumen ${data.instrumenKode}`}
    >
      {/* Header */}
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono text-sm font-semibold">{data.instrumenKode}</span>
        <div className="flex items-center gap-2">
          <PociDirectionBadge direction={data.direction} size="sm" />
          <PociBaselineImmutableBadge size="sm" />
        </div>
      </div>

      {/* Large delta warning */}
      {data.largeDeltaFlag && (
        <div
          className="flex items-center gap-2 rounded-md bg-red-50 border border-red-200 px-2 py-1"
          role="alert"
        >
          <span className="text-xs font-semibold text-red-700">
            LARGE DELTA — {formatIDR(data.deltaEcl)} melebihi threshold
          </span>
        </div>
      )}

      {/* Values grid */}
      <div className={cn("grid gap-3", compact ? "grid-cols-3" : "grid-cols-1 sm:grid-cols-3")}>
        <div className="space-y-0.5">
          <p className="text-xs text-muted-foreground">Baseline (Origination)</p>
          <p
            className="text-sm font-bold tabular-nums"
            title={formatIDR(data.baselineEcl, true)}
            aria-label={`ECL baseline: ${formatIDR(data.baselineEcl, true)}`}
          >
            {formatIDR(data.baselineEcl)}
          </p>
        </div>
        <div className="space-y-0.5">
          <p className="text-xs text-muted-foreground">ECL Saat Ini</p>
          <p
            className="text-sm font-bold tabular-nums"
            title={formatIDR(data.currentEcl, true)}
            aria-label={`ECL saat ini: ${formatIDR(data.currentEcl, true)}`}
          >
            {formatIDR(data.currentEcl)}
          </p>
        </div>
        <div className="space-y-0.5">
          <p className="text-xs text-muted-foreground">Delta (Run Ini)</p>
          <p
            className={cn(
              "text-sm font-bold tabular-nums",
              deltaPositive && "text-red-600",
              deltaNegative && "text-green-600",
            )}
            title={formatIDR(data.deltaEcl, true)}
            aria-label={`Delta ECL: ${formatIDR(data.deltaEcl, true)}`}
          >
            {deltaPositive ? "+" : ""}{formatIDR(data.deltaEcl)}
          </p>
        </div>
      </div>

      {/* Cumulative */}
      {data.priorDeltaCumulative != null && (
        <div className="border-t pt-2">
          <p className="text-xs text-muted-foreground">
            Kumulatif Delta (sebelum run ini):{" "}
            <span className="font-mono font-medium">
              {formatIDR(data.priorDeltaCumulative)}
            </span>
          </p>
        </div>
      )}

      {/* Compute date */}
      <p className="text-xs text-muted-foreground">
        Tanggal hitung:{" "}
        <span className="font-medium">
          {new Date(data.tanggalCompute).toLocaleDateString("id-ID", {
            timeZone: "Asia/Jakarta",
            day: "2-digit",
            month: "short",
            year: "numeric",
          })}
        </span>
      </p>
    </div>
  );
}
