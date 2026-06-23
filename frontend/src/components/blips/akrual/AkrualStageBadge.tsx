"use client";

import * as React from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

interface AkrualStageBadgeProps {
  stage: 1 | 2 | 3 | null | undefined;
  /** Net carrying IDR string (NUMERIC(20,4)) — shown in tooltip for Stage 3 */
  netCarryingIdr?: string | null;
  grossCarryingIdr?: string | null;
  eclIdr?: string | null;
  size?: "sm" | "default";
  className?: string;
}

const IDR_FULL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

function formatIDR(val: string | null | undefined): string {
  if (!val) return "—";
  const n = parseFloat(val);
  if (isNaN(n)) return "—";
  return IDR_FULL.format(n);
}

const STAGE_CONFIG = {
  1: {
    colorClass: "bg-green-50 text-green-700 border-green-300",
    label: "Stage 1",
    description: "Performing — PD 12-bulan. Bunga dari Gross Carrying Amount.",
  },
  2: {
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    label: "Stage 2",
    description:
      "SICR — Significant Increase in Credit Risk. PD Lifetime. Bunga dari Gross Carrying Amount.",
  },
  3: {
    colorClass: "bg-red-100 text-red-700 border-red-400",
    label: "Stage 3",
    description:
      "Credit-impaired. PD = 1.0. Bunga dihitung dari Net Carrying Amount (Gross − ECL) per PSAK 71 §5.4.1(b).",
  },
};

export function AkrualStageBadge({
  stage,
  netCarryingIdr,
  grossCarryingIdr,
  eclIdr,
  size = "default",
  className,
}: AkrualStageBadgeProps) {
  if (!stage) {
    return (
      <span className="text-xs text-muted-foreground" aria-label="Stage tidak tersedia">
        —
      </span>
    );
  }

  const config = STAGE_CONFIG[stage as 1 | 2 | 3];
  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5" : "text-sm px-2 py-1";

  const tooltipContent =
    stage === 3 ? (
      <div className="space-y-1 text-xs max-w-xs">
        <p className="font-semibold">{config.description}</p>
        {grossCarryingIdr && (
          <p>Gross: {formatIDR(grossCarryingIdr)}</p>
        )}
        {eclIdr && <p>ECL (sealed): {formatIDR(eclIdr)}</p>}
        {netCarryingIdr && (
          <p className="font-medium text-red-700">
            Net Carrying (basis akrual): {formatIDR(netCarryingIdr)}
          </p>
        )}
      </div>
    ) : (
      <p className="max-w-xs text-xs">{config.description}</p>
    );

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-semibold cursor-default",
            sizeClass,
            config.colorClass,
            className,
          )}
          role="status"
          aria-label={`ECL stage: ${config.label}`}
        >
          {config.label}
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltipContent}</TooltipContent>
    </Tooltip>
  );
}
