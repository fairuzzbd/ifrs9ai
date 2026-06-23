"use client";

import * as React from "react";
import { Clock, CheckCircle2, XCircle, SkipForward } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { JatuhTempoStatus } from "@/lib/schemas/akrual.schema";
import { JATUH_TEMPO_STATUS_LABELS } from "@/lib/schemas/akrual.schema";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<JatuhTempoStatus, BadgeConfig> = {
  PENDING: {
    Icon: Clock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "Proses jatuh tempo sedang berjalan. Menunggu settlement maturity.",
  },
  SETTLED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-800 border-green-400",
    tooltip:
      "Settlement berhasil. Pokok + bunga last diterima, PPh dipotong. Instrumen berstatus MATURED. Jurnal MATURITY_SETTLEMENT terposting.",
  },
  FAILED: {
    Icon: XCircle,
    colorClass: "bg-red-100 text-red-700 border-red-400",
    tooltip:
      "Proses jatuh tempo gagal. Instrumen tidak di-MATURED. Lihat sys.dlq untuk detail error dan retry.",
  },
  SKIPPED: {
    Icon: SkipForward,
    colorClass: "bg-slate-50 text-slate-600 border-slate-300",
    tooltip: "Dilewati: hari libur nasional (MATURITY.HOLIDAY_SKIP).",
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

export interface JatuhTempoStatusBadgeProps {
  status: JatuhTempoStatus;
  size?: "sm" | "default";
  className?: string;
}

export function JatuhTempoStatusBadge({
  status,
  size = "default",
  className,
}: JatuhTempoStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = JATUH_TEMPO_STATUS_LABELS[status];

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
          aria-label={`Status jatuh tempo: ${label}`}
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
