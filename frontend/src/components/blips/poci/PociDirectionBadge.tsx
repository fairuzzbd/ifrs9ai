"use client";

import * as React from "react";
import { TrendingUp, TrendingDown, Minus } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { PociDirection } from "@/lib/schemas/poci.schema";
import { POCI_DIRECTION_LABELS } from "@/lib/schemas/poci.schema";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<PociDirection, BadgeConfig> = {
  INCREASE: {
    Icon: TrendingUp,
    colorClass: "bg-red-50 text-red-700 border-red-300",
    tooltip:
      "Delta ECL positif — kualitas kredit memburuk sejak origination. Jurnal: D Beban Penurunan Nilai / K Cadangan ECL POCI (PSAK 71 §5.5.14).",
  },
  DECREASE: {
    Icon: TrendingDown,
    colorClass: "bg-green-50 text-green-700 border-green-300",
    tooltip:
      "Delta ECL negatif — kualitas kredit membaik sejak origination. Jurnal: D Cadangan ECL POCI / K Pendapatan Pemulihan ECL POCI (PSAK 71 §5.5.14).",
  },
  ZERO: {
    Icon: Minus,
    colorClass: "bg-slate-50 text-slate-600 border-slate-300",
    tooltip:
      "Delta ECL = 0 — tidak ada perubahan kualitas kredit. Tidak ada jurnal diposting (delta = current − baseline persis sama).",
  },
};

const SIZE_CLASS = {
  sm: "text-xs px-1.5 py-0.5 gap-1",
  default: "text-sm px-2 py-1 gap-1.5",
};

const ICON_SIZE = {
  sm: "h-3 w-3",
  default: "h-4 w-4",
};

export interface PociDirectionBadgeProps {
  direction: PociDirection;
  size?: "sm" | "default";
  className?: string;
}

export function PociDirectionBadge({
  direction,
  size = "default",
  className,
}: PociDirectionBadgeProps) {
  const config = BADGE_CONFIG[direction];
  const { Icon } = config;
  const label = POCI_DIRECTION_LABELS[direction];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            SIZE_CLASS[size],
            config.colorClass,
            className,
          )}
          role="status"
          aria-label={`Arah delta ECL POCI: ${label}`}
        >
          <Icon className={cn(ICON_SIZE[size])} aria-hidden="true" />
          <span>{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{config.tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
