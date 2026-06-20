"use client";

import * as React from "react";
import { ArrowLeftRight, Lock } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { KlasifikasiPsak71 } from "@/lib/schemas/penjualan.schema";

export type OCIRecycleMode = "RECYCLE" | "NO_RECYCLE" | "NOT_APPLICABLE";

/** Determine OCI recycle mode from klasifikasi */
export function resolveOCIRecycleMode(klasifikasi: KlasifikasiPsak71): OCIRecycleMode {
  if (klasifikasi === "FVOCI") return "RECYCLE";
  if (klasifikasi === "FVOCI_ELECTION") return "NO_RECYCLE";
  return "NOT_APPLICABLE";
}

export interface PenjualanOCIRecycleBadgeProps {
  klasifikasi: KlasifikasiPsak71;
  ociRecycled?: string | null;
  size?: "sm" | "default";
  className?: string;
}

export function PenjualanOCIRecycleBadge({
  klasifikasi,
  ociRecycled,
  size = "default",
  className,
}: PenjualanOCIRecycleBadgeProps) {
  const mode = resolveOCIRecycleMode(klasifikasi);

  if (mode === "NOT_APPLICABLE") return null;

  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconSize = size === "sm" ? "h-3 w-3" : "h-4 w-4";

  if (mode === "RECYCLE") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            className={cn(
              "inline-flex items-center rounded-md border font-medium cursor-default",
              "bg-teal-50 text-teal-700 border-teal-300",
              sizeClass,
              className,
            )}
            aria-label="OCI Recycle: OCI cumulative akan di-recycle ke P&amp;L"
          >
            <ArrowLeftRight className={cn(iconSize)} aria-hidden="true" />
            <span>Recycle ke P{"&"}L</span>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          <p className="max-w-xs text-xs">
            {"FVOCI debt — OCI kumulatif di-recycle ke P&L via jurnal REKLAS_OCI_PL. "}
            {ociRecycled ? `OCI: IDR ${ociRecycled}` : ""}
          </p>
        </TooltipContent>
      </Tooltip>
    );
  }

  // NO_RECYCLE — FVOCI Election §B5.7.1
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            "bg-slate-50 text-slate-600 border-slate-300",
            sizeClass,
            className,
          )}
          aria-label="OCI No Recycle: gain/loss tetap di ekuitas per PSAK 71 §B5.7.1"
        >
          <Lock className={cn(iconSize)} aria-hidden="true" />
          <span>Tetap di OCI</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">
          {"FVOCI Election (ekuitas) — gain/loss TIDAK di-recycle ke P&L per PSAK 71 §B5.7.1. Tetap di OCI atau dipindah ke retained earnings."}
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
