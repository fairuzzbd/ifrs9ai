"use client";

import * as React from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { AkrualJenis } from "@/lib/schemas/akrual.schema";
import { AKRUAL_JENIS_LABELS } from "@/lib/schemas/akrual.schema";

const JENIS_CONFIG: Record<AkrualJenis, { colorClass: string; tooltip: string }> = {
  BUNGA: {
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip: "Pendapatan bunga harian per EIR (stage-aware). Dihitung per instrumen oleh DAILY_ACCRUAL_JOB.",
  },
  DIVIDEN: {
    colorClass: "bg-purple-50 text-purple-700 border-purple-300",
    tooltip: "Dividen saham / distribusi reksadana. Dikurangi PPh 10% (UU PPh §17 ayat 2c). Alur: MAKER-TR → APPR-TR.",
  },
  AMORTISASI_PREMIUM: {
    colorClass: "bg-orange-50 text-orange-700 border-orange-300",
    tooltip: "Amortisasi premium obligasi via EIR schedule. Carrying turun, beban ke P&L. AMORTISASI_PD_JOB.",
  },
  AMORTISASI_DISKON: {
    colorClass: "bg-teal-50 text-teal-700 border-teal-300",
    tooltip: "Amortisasi diskon obligasi via EIR schedule. Carrying naik, pendapatan ke P&L. AMORTISASI_PD_JOB.",
  },
  DISTRIBUSI_REKSADANA: {
    colorClass: "bg-pink-50 text-pink-700 border-pink-300",
    tooltip: "Distribusi reksadana dari MI. Look-through ECL. PPh 10% dipotong saat settle.",
  },
};

const SIZE_CLASS = {
  sm: "text-xs px-1.5 py-0.5",
  default: "text-sm px-2 py-1",
};

export interface AkrualJenisBadgeProps {
  jenis: AkrualJenis;
  size?: "sm" | "default";
  className?: string;
}

export function AkrualJenisBadge({ jenis, size = "default", className }: AkrualJenisBadgeProps) {
  const config = JENIS_CONFIG[jenis];
  const label = AKRUAL_JENIS_LABELS[jenis];

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
          aria-label={`Jenis akrual: ${label}`}
        >
          {label}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{config.tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
