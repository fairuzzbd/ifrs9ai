"use client";

import * as React from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmDeviationBadgeProps {
  /** delta_pct from trx.mtm — percentage value (e.g. -9.09) */
  deltaPct: number;
  /** configured threshold (from sys.config PRICE_DEVIATION_THRESHOLD_PCT) */
  thresholdPct: number;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component — rendered only when deviation_flag=TRUE (caller's responsibility)
// ---------------------------------------------------------------------------

export function MtmDeviationBadge({
  deltaPct,
  thresholdPct,
  className,
}: MtmDeviationBadgeProps) {
  const formatted = `${deltaPct >= 0 ? "+" : ""}${deltaPct.toFixed(2)}%`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border border-amber-300 bg-amber-100 px-1.5 py-0.5 text-xs font-semibold text-amber-700 cursor-default",
            className,
          )}
          aria-label={`Peringatan deviasi harga: ${formatted}`}
        >
          DEVIATION {formatted}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">
          Melebihi threshold {thresholdPct.toFixed(2)}%. Verifikasi diperlukan sebelum
          Finance Controller menyetujui jurnal posting.
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
