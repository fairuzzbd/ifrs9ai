"use client";

import * as React from "react";
import { TrendingUp } from "lucide-react";
import { cn } from "@/lib/utils";
import type { AkrualDashboard } from "@/lib/schemas/akrual.schema";
import { AKRUAL_JENIS_LABELS } from "@/lib/schemas/akrual.schema";
import { AkrualStageBadge } from "./AkrualStageBadge";
import { StaleStagingBadge } from "./StaleStagingBadge";

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

export interface AkrualMTDYTDCardProps {
  data: AkrualDashboard;
  /** Compact mode for dashboard grid */
  compact?: boolean;
  className?: string;
}

const BULAN_NAMES = [
  "Januari", "Februari", "Maret", "April", "Mei", "Juni",
  "Juli", "Agustus", "September", "Oktober", "November", "Desember",
];

export function AkrualMTDYTDCard({ data, compact = false, className }: AkrualMTDYTDCardProps) {
  const bulanLabel = BULAN_NAMES[data.month - 1] ?? String(data.month);
  const isStale = data.staleCount > 0;

  return (
    <div
      className={cn(
        "rounded-lg border bg-card p-4 space-y-3",
        isStale && "border-amber-300",
        className,
      )}
      aria-label="Ringkasan akrual MTD/YTD"
    >
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <TrendingUp className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
          <span className="text-sm font-semibold">
            Akrual {bulanLabel} {data.year}
          </span>
        </div>
        {data.stageSaatIni != null && (
          <AkrualStageBadge stage={data.stageSaatIni as 1 | 2 | 3} size="sm" />
        )}
      </div>

      {/* Stale warning */}
      {isStale && (
        <div className="flex items-center gap-2 rounded-md bg-amber-50 border border-amber-200 px-2 py-1">
          <StaleStagingBadge size="sm" />
          <span className="text-xs text-amber-700">
            {data.staleCount} instrumen staging stale
          </span>
        </div>
      )}

      {/* MTD / YTD summary */}
      <div className={cn("grid gap-3", compact ? "grid-cols-2" : "grid-cols-1 sm:grid-cols-2")}>
        <div className="space-y-0.5">
          <p className="text-xs text-muted-foreground">MTD (Bulan ini)</p>
          <p
            className="text-lg font-bold tabular-nums"
            title={formatIDR(data.akrualMtdIdr, true)}
            aria-label={`Akrual MTD: ${formatIDR(data.akrualMtdIdr, true)}`}
          >
            {formatIDR(data.akrualMtdIdr)}
          </p>
        </div>
        <div className="space-y-0.5">
          <p className="text-xs text-muted-foreground">YTD (Tahun {data.year})</p>
          <p
            className="text-lg font-bold tabular-nums"
            title={formatIDR(data.akrualYtdIdr, true)}
            aria-label={`Akrual YTD: ${formatIDR(data.akrualYtdIdr, true)}`}
          >
            {formatIDR(data.akrualYtdIdr)}
          </p>
        </div>
      </div>

      {/* Breakdown per jenis */}
      {!compact && data.breakdown.length > 0 && (
        <div className="border-t pt-2 space-y-1">
          <p className="text-xs font-medium text-muted-foreground mb-1">Rincian per Jenis</p>
          {data.breakdown.map((row) => (
            <div key={row.jenis} className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground">
                {AKRUAL_JENIS_LABELS[row.jenis as keyof typeof AKRUAL_JENIS_LABELS] ?? row.jenis}
              </span>
              <span className="font-mono tabular-nums">{formatIDR(row.mtdIdr)}</span>
            </div>
          ))}
        </div>
      )}

      {/* ECL run info */}
      {data.eclRunSealedAt && (
        <p className="text-xs text-muted-foreground">
          ECL run terakhir sealed:{" "}
          <span className="font-medium">
            {new Date(data.eclRunSealedAt).toLocaleDateString("id-ID", {
              timeZone: "Asia/Jakarta",
              day: "2-digit",
              month: "short",
              year: "numeric",
            })}
          </span>
        </p>
      )}
    </div>
  );
}
