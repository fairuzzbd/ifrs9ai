"use client";

import * as React from "react";
import { Clock, CheckCircle, CheckCircle2, XCircle } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { RenewalStatus } from "@/lib/schemas/renewal.schema";
import { RENEWAL_STATUS_LABELS } from "@/lib/schemas/renewal.schema";

// ---------------------------------------------------------------------------
// Badge config — 4 states (WCAG 2.1 AA: color + icon + text)
// ---------------------------------------------------------------------------

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<RenewalStatus, BadgeConfig> = {
  PENDING_APPROVAL: {
    Icon: Clock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "Menunggu persetujuan ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker.",
  },
  APPROVED: {
    Icon: CheckCircle,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip: "Disetujui oleh Treasury Approver. Proses posting sedang berjalan.",
  },
  POSTED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-800 border-green-400",
    tooltip: "Renewal berhasil diposting. Instrumen baru telah dibuat dan jurnal RENEWAL_DEPOSITO telah dicatat.",
  },
  REJECTED: {
    Icon: XCircle,
    colorClass: "bg-red-100 text-red-700 border-red-400",
    tooltip: "Renewal ditolak oleh Treasury Approver. Maker dapat melihat alasan penolakan dan mengajukan ulang.",
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

export interface RenewalStatusBadgeProps {
  status: RenewalStatus;
  size?: "sm" | "default";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RenewalStatusBadge({
  status,
  size = "default",
  className,
}: RenewalStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = RENEWAL_STATUS_LABELS[status];

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
          aria-label={`Status renewal: ${label}`}
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
