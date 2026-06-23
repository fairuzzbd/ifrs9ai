"use client";

import * as React from "react";
import { Clock, CheckCircle2, XCircle, RotateCcw, Flag } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { BulkRowStatus } from "@/lib/schemas/bulkupload.schema";
import { BULK_ROW_STATUS_LABELS } from "@/lib/schemas/bulkupload.schema";

interface RowBadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const ROW_BADGE_CONFIG: Record<BulkRowStatus, RowBadgeConfig> = {
  PENDING: {
    Icon: Clock,
    colorClass: "bg-slate-50 text-slate-600 border-slate-300",
    tooltip: "Baris menunggu diproses oleh commit worker.",
  },
  COMMITTED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-700 border-green-300",
    tooltip: "INSERT ke mst.instrumen berhasil. Instrumen berstatus PENDING_APPROVAL_BULK.",
  },
  FAILED: {
    Icon: XCircle,
    colorClass: "bg-red-50 text-red-700 border-red-300",
    tooltip: "Baris gagal di-INSERT (mis. duplikat kode). Baris lain tetap diproses (partial commit).",
  },
  ROLLED_BACK: {
    Icon: RotateCcw,
    colorClass: "bg-slate-50 text-slate-500 border-slate-300",
    tooltip: "Instrumen dari baris ini telah soft-deleted oleh rollback CFO (DEC-018).",
  },
  FLAGGED_MANUAL_REVIEW: {
    Icon: Flag,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "SPPI+BM auto-eval ambiguous. Instrumen di-INSERT dengan status PENDING_CLASSIFICATION — menunggu review ROLE-RISK.",
  },
};

export interface BulkRowStatusBadgeProps {
  status: BulkRowStatus;
  size?: "sm" | "default";
  className?: string;
}

export function BulkRowStatusBadge({ status, size = "sm", className }: BulkRowStatusBadgeProps) {
  const config = ROW_BADGE_CONFIG[status];
  const { Icon } = config;
  const label = BULK_ROW_STATUS_LABELS[status];
  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconClass = size === "sm" ? "h-3 w-3" : "h-4 w-4";

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
          aria-label={`Status baris: ${label}`}
        >
          <Icon className={iconClass} aria-hidden="true" />
          <span>{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{config.tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
