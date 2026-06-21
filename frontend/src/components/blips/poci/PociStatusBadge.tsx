"use client";

import * as React from "react";
import { Calculator, CheckCircle2, SkipForward } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { PociStatus } from "@/lib/schemas/poci.schema";
import { POCI_STATUS_LABELS } from "@/lib/schemas/poci.schema";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<PociStatus, BadgeConfig> = {
  COMPUTED: {
    Icon: Calculator,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip:
      "Delta ECL dihitung; jurnal belum diposting (status transient — jarang terjadi di produksi).",
  },
  POSTED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-700 border-green-300",
    tooltip:
      "Jurnal POCI delta berhasil diposting in-transaction. Audit POCI.DELTA_POSTED ter-catat.",
  },
  SKIPPED_ZERO: {
    Icon: SkipForward,
    colorClass: "bg-slate-50 text-slate-600 border-slate-300",
    tooltip:
      "Delta ECL = 0 (ZERO direction) atau periode CLOSED — tidak ada jurnal kosong diposting.",
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

export interface PociStatusBadgeProps {
  status: PociStatus;
  size?: "sm" | "default";
  className?: string;
}

export function PociStatusBadge({
  status,
  size = "default",
  className,
}: PociStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = POCI_STATUS_LABELS[status];

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
          aria-label={`Status delta POCI: ${label}`}
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
