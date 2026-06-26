"use client";

import * as React from "react";
import { Clock, CheckCircle, CheckCircle2, XCircle, AlertTriangle } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { PenjualanStatus } from "@/lib/schemas/penjualan.schema";
import { PENJUALAN_STATUS_LABELS } from "@/lib/schemas/penjualan.schema";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<PenjualanStatus, BadgeConfig> = {
  PENDING_APPROVAL: {
    Icon: Clock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "Menunggu persetujuan ROLE-APPR-TR. SoD: approver tidak boleh sama dengan maker.",
  },
  APPROVED: {
    Icon: CheckCircle,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip: "Disetujui oleh Treasury Approver. Side-effects sedang diproses dalam satu transaksi.",
  },
  POSTED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-800 border-green-400",
    tooltip: "Penjualan berhasil diposting. OCI recycling, BM check, dan jurnal selesai dalam satu transaksi.",
  },
  REJECTED: {
    Icon: XCircle,
    colorClass: "bg-red-100 text-red-700 border-red-400",
    tooltip: "Penjualan ditolak oleh Treasury Approver. Lihat alasan penolakan di detail.",
  },
  PENDING_BM_REVIEW: {
    Icon: AlertTriangle,
    colorClass: "bg-orange-50 text-orange-700 border-orange-400",
    tooltip: "Disposal melebihi hard threshold BM (PSAK 71 §4.1.2b). Menunggu review ROLE-RISK sebelum bisa diposting.",
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

export interface PenjualanStatusBadgeProps {
  status: PenjualanStatus;
  size?: "sm" | "default";
  className?: string;
}

export function PenjualanStatusBadge({
  status,
  size = "default",
  className,
}: PenjualanStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = PENJUALAN_STATUS_LABELS[status];

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
          aria-label={`Status penjualan: ${label}`}
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
