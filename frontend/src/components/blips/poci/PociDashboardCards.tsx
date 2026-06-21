"use client";

import * as React from "react";
import { TrendingUp, TrendingDown, Minus, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import type { PociDeltaSummary } from "@/lib/schemas/poci.schema";

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

const BULAN_NAMES = [
  "Januari", "Februari", "Maret", "April", "Mei", "Juni",
  "Juli", "Agustus", "September", "Oktober", "November", "Desember",
];

export interface PociDashboardCardsProps {
  data: PociDeltaSummary;
  className?: string;
}

/**
 * MTD/YTD aggregate cards for POCI delta dashboard (S5-AC2).
 * Large delta count renders amber warning if > 0 (S5-AC3 guard).
 */
export function PociDashboardCards({ data, className }: PociDashboardCardsProps) {
  const bulanLabel = BULAN_NAMES[data.month - 1] ?? String(data.month);
  const hasLargeDelta = data.largeDeltaCount > 0;
  const mtdParsed = parseFloat(data.deltaEclMtdIdr);

  return (
    <div className={cn("space-y-4", className)} aria-label="Dashboard POCI delta MTD/YTD">
      {/* Large delta alert */}
      {hasLargeDelta && (
        <div
          className="flex items-center gap-2 rounded-md border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-800"
          role="alert"
        >
          <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
          <span>
            <strong>{data.largeDeltaCount} instrumen</strong> memiliki large delta ECL melebihi
            threshold. ROLE-CFO telah dinotifikasi — lihat detail di tabel delta log.
          </span>
        </div>
      )}

      {/* MTD/YTD/Cumulative row */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div
          className={cn(
            "rounded-lg border bg-card p-4 space-y-1",
            mtdParsed > 0 && "border-red-200",
            mtdParsed < 0 && "border-green-200",
          )}
        >
          <p className="text-xs text-muted-foreground">MTD — {bulanLabel} {data.year}</p>
          <p
            className={cn(
              "text-xl font-bold tabular-nums",
              mtdParsed > 0 && "text-red-600",
              mtdParsed < 0 && "text-green-600",
            )}
            title={formatIDR(data.deltaEclMtdIdr, true)}
          >
            {mtdParsed > 0 ? "+" : ""}{formatIDR(data.deltaEclMtdIdr)}
          </p>
          <p className="text-xs text-muted-foreground">Delta ECL POCI bulan ini</p>
        </div>

        <div className="rounded-lg border bg-card p-4 space-y-1">
          <p className="text-xs text-muted-foreground">YTD — Tahun {data.year}</p>
          <p
            className="text-xl font-bold tabular-nums"
            title={formatIDR(data.deltaEclYtdIdr, true)}
          >
            {formatIDR(data.deltaEclYtdIdr)}
          </p>
          <p className="text-xs text-muted-foreground">Sigma delta Jan–{bulanLabel} {data.year}</p>
        </div>

        <div className="rounded-lg border bg-card p-4 space-y-1">
          <p className="text-xs text-muted-foreground">Kumulatif Sejak Origination</p>
          <p
            className="text-xl font-bold tabular-nums"
            title={formatIDR(data.netCumulativeDeltaIdr, true)}
          >
            {formatIDR(data.netCumulativeDeltaIdr)}
          </p>
          <p className="text-xs text-muted-foreground">
            {data.instrumenCount} instrumen POCI aktif
          </p>
        </div>
      </div>

      {/* Direction breakdown */}
      <div className="rounded-lg border bg-card p-4">
        <p className="text-sm font-medium mb-3">Rincian per Arah Delta</p>
        <div className="grid grid-cols-3 gap-3">
          <div className="flex items-center gap-2">
            <TrendingUp className="h-4 w-4 text-red-500 flex-shrink-0" aria-hidden="true" />
            <div>
              <p className="text-xs text-muted-foreground">INCREASE</p>
              <p className="text-sm font-semibold">
                {data.directionBreakdown.increase.count} instrumen
              </p>
              <p className="text-xs font-mono text-red-600">
                {formatIDR(data.directionBreakdown.increase.amountIdr)}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <TrendingDown className="h-4 w-4 text-green-500 flex-shrink-0" aria-hidden="true" />
            <div>
              <p className="text-xs text-muted-foreground">DECREASE</p>
              <p className="text-sm font-semibold">
                {data.directionBreakdown.decrease.count} instrumen
              </p>
              <p className="text-xs font-mono text-green-600">
                {formatIDR(data.directionBreakdown.decrease.amountIdr)}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Minus className="h-4 w-4 text-slate-400 flex-shrink-0" aria-hidden="true" />
            <div>
              <p className="text-xs text-muted-foreground">ZERO</p>
              <p className="text-sm font-semibold">
                {data.directionBreakdown.zero.count} instrumen
              </p>
              <p className="text-xs text-muted-foreground">Tidak ada perubahan</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
