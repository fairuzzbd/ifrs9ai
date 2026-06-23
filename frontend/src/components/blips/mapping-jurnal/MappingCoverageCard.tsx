/**
 * MappingCoverageCard — RPT-19 GAP_COVERAGE per event group.
 * Shows: total/active/missing counts, per-event badge, DLQ link.
 */

"use client";

import * as React from "react";
import Link from "next/link";
import { CheckCircle2, XCircle, AlertTriangle, ExternalLink } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { Rpt19Coverage, Rpt19CoverageEvent, GapCoverage } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingCoverageCardProps {
  coverage: Rpt19Coverage;
  className?: string;
}

const GAP_STYLES: Record<GapCoverage, { bg: string; text: string; icon: React.ReactNode; label: string }> = {
  OK: {
    bg: "bg-green-50 border-green-200",
    text: "text-green-700",
    icon: <CheckCircle2 className="h-4 w-4 text-green-600" aria-hidden="true" />,
    label: "Lengkap",
  },
  MISSING: {
    bg: "bg-red-50 border-red-200",
    text: "text-red-700",
    icon: <XCircle className="h-4 w-4 text-red-600" aria-hidden="true" />,
    label: "Belum ada mapping aktif",
  },
  INCOMPLETE: {
    bg: "bg-amber-50 border-amber-200",
    text: "text-amber-700",
    icon: <AlertTriangle className="h-4 w-4 text-amber-500" aria-hidden="true" />,
    label: "Incomplete — ada akun kosong",
  },
};

function GapBadge({ gap }: { gap: GapCoverage }) {
  const style = GAP_STYLES[gap];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
        style.bg,
        style.text,
      )}
      role="status"
      aria-label={style.label}
    >
      {style.icon}
      <span>{style.label}</span>
    </span>
  );
}

function CoverageEventRow({ event }: { event: Rpt19CoverageEvent }) {
  return (
    <div className="flex items-start justify-between gap-3 py-3 border-b last:border-0">
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="flex items-center gap-2 flex-wrap">
          <code className="font-mono text-sm font-bold">{event.eventCode}</code>
          <span className="text-xs text-muted-foreground">{event.namaEvent}</span>
        </div>
        {event.lastDlqError && (
          <div className="flex items-center gap-1 text-xs text-red-600">
            <XCircle className="h-3 w-3 shrink-0" aria-hidden="true" />
            <span>DLQ error: {new Date(event.lastDlqError).toLocaleDateString("id-ID")}</span>
            <Link
              href={`/jurnal/dlq?filter[event_code]=${event.eventCode}&filter[error_code]=JURNAL_EVENT_NOT_MAPPED`}
              className="underline hover:no-underline ml-1 inline-flex items-center gap-0.5"
              aria-label={`Lihat DLQ untuk ${event.eventCode}`}
            >
              Lihat DLQ
              <ExternalLink className="h-3 w-3" aria-hidden="true" />
            </Link>
          </div>
        )}
        {event.missingAkunCount > 0 && (
          <p className="text-xs text-amber-600">
            {event.missingAkunCount} baris dengan akun kosong
          </p>
        )}
      </div>
      <div className="flex items-center gap-2 shrink-0">
        <Link
          href={`/mapping-jurnal?filter[event_code]=${event.eventCode}`}
          className="text-xs text-primary hover:underline"
          aria-label={`Lihat mapping ${event.eventCode}`}
        >
          Detail
        </Link>
        <GapBadge gap={event.gapCoverage} />
      </div>
    </div>
  );
}

export function MappingCoverageCard({ coverage, className }: MappingCoverageCardProps) {
  const coveragePct =
    coverage.totalEvents > 0
      ? Math.round((coverage.activeEvents / coverage.totalEvents) * 100)
      : 0;

  return (
    <div className={cn("space-y-4", className)}>
      {/* Summary KPI */}
      <div className="grid grid-cols-3 gap-3">
        <div className="rounded-lg border bg-muted/30 p-3 text-center">
          <p className="text-2xl font-bold">{coverage.totalEvents}</p>
          <p className="text-xs text-muted-foreground mt-0.5">Total Event</p>
        </div>
        <div className="rounded-lg border bg-green-50 border-green-200 p-3 text-center">
          <p className="text-2xl font-bold text-green-700">{coverage.activeEvents}</p>
          <p className="text-xs text-green-600 mt-0.5">Aktif ({coveragePct}%)</p>
        </div>
        <div className="rounded-lg border bg-red-50 border-red-200 p-3 text-center">
          <p className="text-2xl font-bold text-red-700">{coverage.missingEvents}</p>
          <p className="text-xs text-red-600 mt-0.5">Belum Aktif</p>
        </div>
      </div>

      {/* Gap events list */}
      {coverage.gapEvents.length === 0 ? (
        <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-3">
          <CheckCircle2 className="h-5 w-5 text-green-600 shrink-0" aria-hidden="true" />
          <span className="text-sm text-green-700 font-medium">
            Semua event sudah memiliki mapping aktif dan lengkap.
          </span>
        </div>
      ) : (
        <div className="rounded-lg border">
          <div className="px-4 py-2 border-b bg-muted/30">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Event dengan GAP ({coverage.gapEvents.length})
            </h3>
          </div>
          <div className="px-4 divide-y divide-transparent">
            {coverage.gapEvents.map((event) => (
              <CoverageEventRow key={event.eventCode} event={event} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
