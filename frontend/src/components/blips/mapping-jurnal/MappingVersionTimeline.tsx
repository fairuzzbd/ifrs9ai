/**
 * MappingVersionTimeline — version chain display with parent_id links + effective_from/to.
 * Shows immutable version history per event_code (DEC-018).
 */

import * as React from "react";
import Link from "next/link";
import { format } from "date-fns";
import { ArrowRight, Clock } from "lucide-react";
import { cn } from "@/lib/utils";
import { MappingStatusBadge } from "./MappingStatusBadge";
import type { MappingP12HeaderSummary } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingVersionTimelineProps {
  versions: MappingP12HeaderSummary[];
  currentVersionId: string;
  eventCode: string;
  className?: string;
}

function fmtDt(s: string | null | undefined): string {
  if (!s) return "—";
  try {
    return format(new Date(s), "dd MMM yyyy HH:mm");
  } catch {
    return s;
  }
}

export function MappingVersionTimeline({
  versions,
  currentVersionId,
  eventCode,
  className,
}: MappingVersionTimelineProps) {
  if (versions.length === 0) {
    return (
      <p className="text-xs text-muted-foreground italic">Tidak ada riwayat versi.</p>
    );
  }

  // Sort by effectiveFrom desc (newest first)
  const sorted = [...versions].sort((a, b) => {
    const aTime = a.effectiveFrom ? new Date(a.effectiveFrom).getTime() : 0;
    const bTime = b.effectiveFrom ? new Date(b.effectiveFrom).getTime() : 0;
    return bTime - aTime;
  });

  return (
    <div className={cn("space-y-0", className)} aria-label="Riwayat versi mapping">
      {sorted.map((v, idx) => {
        const isCurrent = v.id === currentVersionId;
        return (
          <div key={v.id} className="flex gap-3">
            <div className="flex flex-col items-center">
              <div
                className={cn(
                  "flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold",
                  isCurrent
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground",
                )}
                aria-hidden="true"
              >
                {sorted.length - idx}
              </div>
              {idx < sorted.length - 1 && (
                <div className="mt-1 flex-1 w-px bg-gray-200 min-h-[16px]" />
              )}
            </div>
            <div className="pb-4 min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2 mb-0.5">
                <MappingStatusBadge status={v.workflowStatus} size="sm" />
                {isCurrent && (
                  <span className="text-xs text-primary font-medium">(ini)</span>
                )}
                {!isCurrent && (
                  <Link
                    href={`/mapping-jurnal/${eventCode}?version=${v.id}`}
                    className="text-xs text-primary hover:underline"
                    aria-label={`Lihat versi ${sorted.length - idx}`}
                  >
                    Lihat
                  </Link>
                )}
              </div>
              <div className="flex items-center gap-1 text-xs text-muted-foreground">
                <Clock className="h-3 w-3 shrink-0" aria-hidden="true" />
                <span>{fmtDt(v.effectiveFrom)}</span>
                {v.effectiveTo && (
                  <>
                    <ArrowRight className="h-3 w-3" aria-hidden="true" />
                    <span>{fmtDt(v.effectiveTo)}</span>
                  </>
                )}
                {!v.effectiveTo && v.workflowStatus === "APPROVED_ACTIVE" && (
                  <>
                    <ArrowRight className="h-3 w-3" aria-hidden="true" />
                    <span className="text-green-600 font-medium">sekarang</span>
                  </>
                )}
              </div>
              {v.parentId && (
                <p className="text-xs text-muted-foreground mt-0.5">
                  Turunan dari:{" "}
                  <code className="font-mono text-xs">{v.parentId.slice(0, 8)}...</code>
                </p>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
