"use client";

import * as React from "react";
import { Pencil } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { HargaSumber } from "@/lib/schemas/mtm.schema";
import { HARGA_SUMBER_LABELS } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Source → color config
// ---------------------------------------------------------------------------

const SOURCE_COLOR: Record<HargaSumber, string> = {
  IBPA: "bg-blue-50 text-blue-700 border-blue-300",
  IBPA_MANUAL: "bg-blue-50 text-blue-700 border-blue-300",
  BEI: "bg-green-50 text-green-800 border-green-300",
  BEI_MANUAL: "bg-green-50 text-green-800 border-green-300",
  KSEI: "bg-purple-50 text-purple-700 border-purple-300",
  MANUAL: "bg-slate-100 text-slate-600 border-slate-300",
};

const IS_MANUAL: Record<HargaSumber, boolean> = {
  IBPA: false,
  IBPA_MANUAL: true,
  BEI: false,
  BEI_MANUAL: true,
  KSEI: false,
  MANUAL: true,
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmSourceBadgeProps {
  source: HargaSumber;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (color + icon for MANUAL variants; aria-label)
// ---------------------------------------------------------------------------

export function MtmSourceBadge({ source, className }: MtmSourceBadgeProps) {
  const label = HARGA_SUMBER_LABELS[source];
  const isManual = IS_MANUAL[source];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-xs font-medium cursor-default",
            SOURCE_COLOR[source],
            className,
          )}
          aria-label={`Sumber harga: ${label}`}
        >
          {label}
          {isManual && (
            <Pencil className="h-2.5 w-2.5" aria-hidden="true" />
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">
          {isManual ? "Harga di-input manual (override dari sumber resmi)" : `Feed otomatis dari ${label}`}
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
