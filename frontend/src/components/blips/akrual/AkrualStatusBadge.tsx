"use client";

import * as React from "react";
import {
  Clock,
  CheckCircle2,
  ShieldAlert,
  CheckCircle,
  SkipForward,
} from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { AkrualStatus } from "@/lib/schemas/akrual.schema";
import { AKRUAL_STATUS_LABELS } from "@/lib/schemas/akrual.schema";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<AkrualStatus, BadgeConfig> = {
  PENDING_STALE_REVIEW: {
    Icon: Clock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip:
      "ECL sealed run lebih lama dari batas staleness. ROLE-AKUN-CTL harus konfirmasi staging sebelum akrual dapat diposting.",
  },
  AUTO_POSTED: {
    Icon: CheckCircle,
    colorClass: "bg-green-50 text-green-700 border-green-300",
    tooltip:
      "Akrual harian dihitung dan jurnal diposting secara otomatis oleh cron DAILY_ACCRUAL_JOB atau AMORTISASI_PD_JOB.",
  },
  OVERRIDE_APPROVED: {
    Icon: ShieldAlert,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip:
      "ROLE-AKUN-CTL telah mengkonfirmasi staging masih valid. Akrual akan di-recompute dan diposting.",
  },
  POSTED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-800 border-green-400",
    tooltip: "Akrual berhasil diposting. Jurnal AKRUAL_BUNGA / AMORTISASI_PD terkirim ke GL.",
  },
  SKIPPED: {
    Icon: SkipForward,
    colorClass: "bg-slate-50 text-slate-600 border-slate-300",
    tooltip:
      "Akrual dilewati: instrumen FVTPL, hari libur, atau duplikat terdeteksi (idempotency guard).",
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

export interface AkrualStatusBadgeProps {
  status: AkrualStatus;
  size?: "sm" | "default";
  className?: string;
}

export function AkrualStatusBadge({
  status,
  size = "default",
  className,
}: AkrualStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = AKRUAL_STATUS_LABELS[status];

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
          aria-label={`Status akrual: ${label}`}
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
