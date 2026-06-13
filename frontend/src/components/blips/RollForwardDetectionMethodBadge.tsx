"use client";

import * as React from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { DetectionMethod } from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RollForwardDetectionMethodBadgeProps {
  method: DetectionMethod;
  className?: string;
}

// ---------------------------------------------------------------------------
// Token map
// ---------------------------------------------------------------------------

const METHOD_TOKENS: Record<
  DetectionMethod,
  { label: string; bg: string; text: string; tooltip: string }
> = {
  BASIC_STATUS_DIFF: {
    label: "BASIC_STATUS_DIFF",
    bg: "bg-amber-100",
    text: "text-amber-800",
    tooltip:
      "Metode Phase 4: Deteksi origination/derecognition dari perubahan status instrumen dan kehadiran di calc_run result. Deteksi berbasis lifecycle event (penempatan, penjualan, jatuh tempo) akan tersedia di Phase 5.",
  },
  FULL_LIFECYCLE_PHASE_5: {
    label: "FULL_LIFECYCLE",
    bg: "bg-green-100",
    text: "text-green-800",
    tooltip:
      "Metode Phase 5: Deteksi penuh berbasis lifecycle event dari APP-B (penempatan, penjualan, jatuh tempo). Presisi lebih tinggi.",
  },
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RollForwardDetectionMethodBadge({
  method,
  className,
}: RollForwardDetectionMethodBadgeProps) {
  const tokens = METHOD_TOKENS[method] ?? METHOD_TOKENS.BASIC_STATUS_DIFF;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={`Metode deteksi: ${tokens.label}`}
          className={cn(
            "inline-flex items-center px-2.5 py-0.5 text-xs font-medium rounded-md cursor-help",
            tokens.bg,
            tokens.text,
            className,
          )}
        >
          {tokens.label}
        </span>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        <p className="max-w-sm text-xs">{tokens.tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
