"use client";

import * as React from "react";
import { Coins, PlusCircle } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { RenewalSkema } from "@/lib/schemas/renewal.schema";
import { RENEWAL_SKEMA_LABELS } from "@/lib/schemas/renewal.schema";

// ---------------------------------------------------------------------------
// Badge config — 2 skema (WCAG AA: distinct color + distinct icon)
// ---------------------------------------------------------------------------

interface SkemaBadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const SKEMA_BADGE_CONFIG: Record<RenewalSkema, SkemaBadgeConfig> = {
  POKOK_SAJA: {
    Icon: Coins,
    colorClass: "bg-slate-100 text-slate-700 border-slate-300",
    tooltip: "Pokok Saja: pokok baru = pokok lama. Bunga bersih (after PPh 20%) dikembalikan ke kas.",
  },
  POKOK_PLUS_BUNGA: {
    Icon: PlusCircle,
    colorClass: "bg-indigo-50 text-indigo-700 border-indigo-300",
    tooltip: "Pokok + Bunga: pokok baru = pokok lama + bunga bersih (after PPh 20%). Minimum bunga bersih IDR 100.000.",
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

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RenewalSkemaBadgeProps {
  skema: RenewalSkema;
  size?: "sm" | "default";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RenewalSkemaBadge({
  skema,
  size = "default",
  className,
}: RenewalSkemaBadgeProps) {
  const config = SKEMA_BADGE_CONFIG[skema];
  const { Icon } = config;
  const label = RENEWAL_SKEMA_LABELS[skema];

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
          aria-label={`Skema renewal: ${label}`}
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
