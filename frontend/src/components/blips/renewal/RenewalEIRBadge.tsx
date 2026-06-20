"use client";

import * as React from "react";
import { TrendingUp } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RenewalEIRBadgeProps {
  /** EIR as decimal string e.g. "0.04600000" (8 decimal places, DEC-013) */
  eirBaru: string;
  className?: string;
}

// ---------------------------------------------------------------------------
// Formatter — convert decimal to percentage display (4 decimal places for UI)
// ---------------------------------------------------------------------------

function formatEIR(eirDecimal: string): string {
  const val = parseFloat(eirDecimal);
  if (isNaN(val)) return "—";
  return `${(val * 100).toFixed(4)}%`;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RenewalEIRBadge({ eirBaru, className }: RenewalEIRBadgeProps) {
  const display = formatEIR(eirBaru);

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center gap-1 rounded-md border px-2 py-1 text-sm font-mono font-medium",
            "bg-teal-50 text-teal-700 border-teal-300 cursor-default",
            className,
          )}
          aria-label={`EIR baru: ${display} per tahun`}
        >
          <TrendingUp className="h-4 w-4 shrink-0" aria-hidden="true" />
          <span>{display}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">
          EIR (Effective Interest Rate) dihitung menggunakan metode Newton-Raphson IRR
          (DEC-013: tolerance 1e-10, max 100 iterasi). Mencerminkan yield after-tax
          (bunga bersih setelah PPh 20% — PP No. 131/2000). Presisi 8 desimal.
        </p>
        <p className="mt-1 text-xs font-mono text-muted-foreground">Raw: {eirBaru}</p>
      </TooltipContent>
    </Tooltip>
  );
}
