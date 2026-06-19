"use client";

import * as React from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { JURNAL_EVENT_CODE_LABELS } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmRoutingBadgeProps {
  /** jurnal event codes — e.g. ['MTM_FVOCI', 'MTM_FX_OCI_RESERVE'] */
  eventCodes: string[];
  /** klasifikasi_psak71 — for aria context */
  klasifikasi: string;
  className?: string;
}

// ---------------------------------------------------------------------------
// Per-pill color by event code prefix
// ---------------------------------------------------------------------------

function getPillColor(code: string): string {
  if (code.includes("FVOCI_ELECTION")) return "bg-purple-50 text-purple-700 border-purple-300";
  if (code.includes("FVOCI")) return "bg-blue-50 text-blue-700 border-blue-300";
  if (code.includes("FVTPL_POCI")) return "bg-orange-50 text-orange-700 border-orange-300";
  if (code.includes("FVTPL")) return "bg-rose-50 text-rose-700 border-rose-300";
  if (code.includes("FX_OCI")) return "bg-indigo-50 text-indigo-700 border-indigo-300";
  return "bg-slate-100 text-slate-600 border-slate-300";
}

// ---------------------------------------------------------------------------
// Component — pills stacked vertically when >1 event code (§B5.7.2A dual jurnal)
// ---------------------------------------------------------------------------

export function MtmRoutingBadge({
  eventCodes,
  klasifikasi,
  className,
}: MtmRoutingBadgeProps) {
  if (!eventCodes || eventCodes.length === 0) {
    return (
      <span className="text-xs text-muted-foreground" aria-label="Routing jurnal: menunggu">
        —
      </span>
    );
  }

  return (
    <span
      className={cn("inline-flex flex-col gap-0.5", className)}
      aria-label={`Routing jurnal: ${eventCodes.join(", ")}`}
    >
      {eventCodes.map((code) => {
        const label = JURNAL_EVENT_CODE_LABELS[code] ?? code;
        return (
          <Tooltip key={code}>
            <TooltipTrigger asChild>
              <span
                className={cn(
                  "inline-flex items-center rounded border px-1 py-0.5 text-xs font-medium cursor-default",
                  getPillColor(code),
                )}
              >
                {label}
              </span>
            </TooltipTrigger>
            <TooltipContent>
              <p className="max-w-xs text-xs">
                {code} — {klasifikasi}
                {code === "MTM_FX_OCI_RESERVE" && " (§B5.7.2A FVOCI Debt FCY — dua jurnal terpisah)"}
              </p>
            </TooltipContent>
          </Tooltip>
        );
      })}
    </span>
  );
}
