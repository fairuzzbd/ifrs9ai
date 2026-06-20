"use client";

import * as React from "react";
import { Tag } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { KlasifikasiPsak71, JenisDisposal } from "@/lib/schemas/penjualan.schema";

/** Resolve event codes per routing matrix (state-machine §Klasifikasi Routing Matrix) */
export function resolveJurnalEventCodes(
  klasifikasi: KlasifikasiPsak71,
  _jenisDisposal: JenisDisposal,
): string[] {
  switch (klasifikasi) {
    case "AC":
      return ["PENJUALAN_AC"];
    case "FVOCI":
      return ["PENJUALAN_FVOCI_DEBT", "REKLAS_OCI_PL"];
    case "FVOCI_ELECTION":
      return ["PENJUALAN_FVOCI_ELECTION"];
    case "FVTPL":
      return ["PENJUALAN_FVTPL"];
    case "POCI":
      return ["PENJUALAN_POCI"];
    default:
      return [];
  }
}

export interface PenjualanRoutingBadgeProps {
  klasifikasi: KlasifikasiPsak71;
  jenisDisposal: JenisDisposal;
  className?: string;
}

export function PenjualanRoutingBadge({
  klasifikasi,
  jenisDisposal,
  className,
}: PenjualanRoutingBadgeProps) {
  const codes = resolveJurnalEventCodes(klasifikasi, jenisDisposal);

  return (
    <div className={cn("flex flex-wrap gap-1", className)}>
      {codes.map((code) => (
        <Tooltip key={code}>
          <TooltipTrigger asChild>
            <span
              className="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-0.5 text-xs font-mono text-gray-700 border border-gray-200"
              aria-label={`Jurnal event code: ${code}`}
            >
              <Tag className="h-3 w-3" aria-hidden="true" />
              {code}
            </span>
          </TooltipTrigger>
          <TooltipContent>
            <p className="text-xs">Jurnal event code dari P5-M2 engine</p>
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}
