"use client";

import * as React from "react";
import { Lock } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

const SIZE_CLASS = {
  sm: "text-xs px-1.5 py-0.5 gap-1",
  default: "text-sm px-2 py-1 gap-1.5",
};

const ICON_SIZE = {
  sm: "h-3 w-3",
  default: "h-4 w-4",
};

export interface PociBaselineImmutableBadgeProps {
  size?: "sm" | "default";
  className?: string;
}

/**
 * Badge indicating ecl.poci_baseline WORM status.
 * DEC-018: append-only, no UPDATE or DELETE trigger at DB level.
 * S1-AC2: POCI_BASELINE_IMMUTABLE_VIOLATION returned on any overwrite attempt.
 */
export function PociBaselineImmutableBadge({
  size = "default",
  className,
}: PociBaselineImmutableBadgeProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            "bg-slate-50 text-slate-700 border-slate-300",
            SIZE_CLASS[size],
            className,
          )}
          role="img"
          aria-label="Baseline immutable — WORM"
        >
          <Lock className={cn(ICON_SIZE[size])} aria-hidden="true" />
          <span>WORM</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">
          Baseline POCI bersifat WORM (Write Once Read Many) — append-only per DEC-018.
          Tidak dapat diubah atau dihapus sejak origination. DB trigger mencegah UPDATE/DELETE
          di lapisan database sebagai defence-in-depth (S1-AC2).
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
