"use client";

import * as React from "react";
import { AlertTriangle, ShieldAlert } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { BMViolationFlag } from "@/lib/schemas/penjualan.schema";

export interface PenjualanBMRiskBadgeProps {
  flag: BMViolationFlag;
  pct?: string;
  warnThreshold?: string;
  blockThreshold?: string;
  size?: "sm" | "default";
  className?: string;
}

export function PenjualanBMRiskBadge({
  flag,
  pct,
  warnThreshold,
  blockThreshold,
  size = "default",
  className,
}: PenjualanBMRiskBadgeProps) {
  const isBlock = flag === "BM_VIOLATION_BLOCK";
  const Icon = isBlock ? ShieldAlert : AlertTriangle;
  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconSize = size === "sm" ? "h-3 w-3" : "h-4 w-4";

  const label = isBlock ? "BM Pelanggaran (Blokir)" : "BM Risiko";
  const colorClass = isBlock
    ? "bg-red-100 text-red-700 border-red-400"
    : "bg-orange-50 text-orange-700 border-orange-400";

  const tooltip = isBlock
    ? `Disposal kumulatif 12-bulan${pct ? ` ${pct}%` : ""} melewati hard limit${blockThreshold ? ` ${blockThreshold}%` : ""}. Approval ROLE-RISK diperlukan (PSAK 71 §4.1.2b).`
    : `Disposal kumulatif 12-bulan${pct ? ` ${pct}%` : ""} melewati threshold peringatan${warnThreshold ? ` ${warnThreshold}%` : ""}. ROLE-RISK telah dinotifikasi.`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            sizeClass,
            colorClass,
            className,
          )}
          aria-label={label}
        >
          <Icon className={cn(iconSize)} aria-hidden="true" />
          <span>{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
