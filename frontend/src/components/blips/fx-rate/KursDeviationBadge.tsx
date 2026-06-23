"use client";

import * as React from "react";
import { AlertTriangle } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KursDeviationBadgeProps {
  /** deviation_flag from mst.kurs */
  deviationFlag: boolean;
  /** rate_deviation_pct from mst.kurs — shown as ±X.XX% */
  rateDeviationPct?: number | null;
  /** Deviation threshold used — default 20% from sys.config */
  threshold?: number;
  size?: "sm" | "md";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S1-AC2, S2-AC2 — warning badge when deviation_flag=true)
// ---------------------------------------------------------------------------

export function KursDeviationBadge({
  deviationFlag,
  rateDeviationPct,
  threshold = 20,
  size = "md",
  className,
}: KursDeviationBadgeProps) {
  if (!deviationFlag) return null;

  const pctText =
    rateDeviationPct != null
      ? `${rateDeviationPct >= 0 ? "+" : ""}${rateDeviationPct.toFixed(2)}%`
      : null;

  const tooltipMsg = pctText
    ? `Deviasi ${pctText} melebihi threshold ${threshold}%. Harap verifikasi data kurs sebelum digunakan dalam perhitungan MTM dan akrual.`
    : `Deviasi melebihi threshold ${threshold}%. Harap verifikasi data kurs.`;

  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconSize = size === "sm" ? "h-3 w-3" : "h-4 w-4";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium",
            "bg-amber-50 text-amber-700 border-amber-300",
            sizeClass,
            className,
          )}
          role="alert"
          aria-label={`Peringatan deviasi kurs${pctText ? ` ${pctText}` : ""}`}
        >
          <AlertTriangle className={cn(iconSize, "shrink-0")} aria-hidden="true" />
          <span>
            Deviasi{pctText ? ` ${pctText}` : ""}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{tooltipMsg}</p>
      </TooltipContent>
    </Tooltip>
  );
}
