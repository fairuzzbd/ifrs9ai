"use client";

import * as React from "react";
import {
  CheckCircle2,
  Clock,
  CheckCircle,
  XCircle,
  AlertTriangle,
} from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { MtmStatus } from "@/lib/schemas/mtm.schema";
import { MTM_STATUS_LABELS } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Badge config — 5 states (WCAG 2.1 AA: color + icon + text)
// ---------------------------------------------------------------------------

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<MtmStatus, BadgeConfig> = {
  AUTO_POSTED: {
    Icon: CheckCircle2,
    colorClass: "bg-slate-100 text-slate-700 border-slate-300",
    tooltip: "MTM dihitung otomatis oleh cron job 18:00 WIB dan jurnal sudah diposting.",
  },
  PENDING_REVIEW: {
    Icon: Clock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "Menunggu review Finance Controller (ROLE-AKUN-CTL). Deviasi harga melebihi threshold atau upload manual.",
  },
  APPROVED: {
    Icon: CheckCircle,
    colorClass: "bg-green-50 text-green-800 border-green-400",
    tooltip: "Override disetujui oleh Finance Controller. Jurnal sudah diposting.",
  },
  REJECTED: {
    Icon: XCircle,
    colorClass: "bg-red-100 text-red-700 border-red-400",
    tooltip: "MTM ditolak. Jurnal tidak diposting. ROLE-AKUN dinotifikasi untuk re-upload.",
  },
  STALE_PRICE: {
    Icon: AlertTriangle,
    // Outline destructive (border red, white bg) — distinguishable from REJECTED (filled red)
    colorClass: "bg-white text-red-700 border-red-400",
    tooltip: "Harga pasar belum diperbarui melebihi batas waktu. MTM tidak dapat diposting otomatis.",
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

export interface MtmStatusBadgeProps {
  /** status from trx.mtm — 5 possible values */
  status: MtmStatus;
  size?: "sm" | "default";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (WCAG AA: color + icon + text; aria-label)
// ---------------------------------------------------------------------------

export function MtmStatusBadge({
  status,
  size = "default",
  className,
}: MtmStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = MTM_STATUS_LABELS[status];

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
          aria-label={`Status MTM: ${label}`}
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
