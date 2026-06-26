"use client";

import * as React from "react";
import { Layers, Square } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { JenisDisposal } from "@/lib/schemas/penjualan.schema";
import { JENIS_DISPOSAL_LABELS } from "@/lib/schemas/penjualan.schema";

export interface PenjualanJenisBadgeProps {
  jenis: JenisDisposal;
  size?: "sm" | "default";
  className?: string;
}

export function PenjualanJenisBadge({
  jenis,
  size = "default",
  className,
}: PenjualanJenisBadgeProps) {
  const isFull = jenis === "FULL";
  const Icon = isFull ? Square : Layers;
  const label = JENIS_DISPOSAL_LABELS[jenis];
  const colorClass = isFull
    ? "bg-purple-50 text-purple-700 border-purple-300"
    : "bg-indigo-50 text-indigo-700 border-indigo-300";
  const tooltip = isFull
    ? "Penjualan penuh — seluruh qty holding dijual. Instrumen akan berstatus DISPOSED."
    : "Penjualan sebagian — qty holding dikurangi. Instrumen tetap ACTIVE.";

  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconSize = size === "sm" ? "h-3 w-3" : "h-4 w-4";

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
          aria-label={`Jenis disposal: ${label}`}
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
