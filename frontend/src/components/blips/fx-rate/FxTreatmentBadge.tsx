"use client";

import * as React from "react";
import { TrendingUp, BarChart2, Minus } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { FxTreatmentRouting } from "@/lib/schemas/fx-rate.schema";
import {
  FX_TREATMENT_LABELS,
  FX_TREATMENT_PSAK_REF,
} from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Badge config (WCAG 2.1 AA)
// ---------------------------------------------------------------------------

interface TreatmentConfig {
  colorClass: string;
  Icon: React.ElementType;
}

const TREATMENT_CONFIG: Record<FxTreatmentRouting, TreatmentConfig> = {
  "P&L_FOREIGN_EXCHANGE": {
    Icon: TrendingUp,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
  },
  OCI_FOREIGN_EXCHANGE_RESERVE: {
    Icon: BarChart2,
    colorClass: "bg-purple-50 text-purple-700 border-purple-300",
  },
  OCI_FOREIGN_EXCHANGE_RESERVE_NO_RECYCLING: {
    Icon: BarChart2,
    colorClass: "bg-violet-50 text-violet-700 border-violet-300",
  },
  NO_FX_TREATMENT: {
    Icon: Minus,
    colorClass: "bg-slate-100 text-slate-500 border-slate-300",
  },
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface FxTreatmentBadgeProps {
  routing: FxTreatmentRouting;
  size?: "sm" | "md";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S5-AC1..4)
// ---------------------------------------------------------------------------

export function FxTreatmentBadge({
  routing,
  size = "md",
  className,
}: FxTreatmentBadgeProps) {
  const config = TREATMENT_CONFIG[routing];
  const { Icon } = config;
  const label = FX_TREATMENT_LABELS[routing];
  const psak = FX_TREATMENT_PSAK_REF[routing];

  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconSize = size === "sm" ? "h-3 w-3" : "h-4 w-4";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            sizeClass,
            config.colorClass,
            className,
          )}
          role="status"
          aria-label={`FX treatment: ${label}`}
        >
          <Icon className={cn(iconSize)} aria-hidden="true" />
          <span>{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{psak}</p>
      </TooltipContent>
    </Tooltip>
  );
}
