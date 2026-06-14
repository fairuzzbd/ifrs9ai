"use client";

import * as React from "react";
import { Info } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { RollForwardComponent } from "@/lib/schemas/ecl-core.schema";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  const formatted = new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(Math.abs(num));
  return formatted;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RollForwardWaterfallProps {
  components: RollForwardComponent[];
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RollForwardWaterfall({ components, className }: RollForwardWaterfallProps) {
  return (
    <div className={cn("overflow-x-auto", className)}>
      <table className="w-full text-sm border-collapse">
        <thead>
          <tr className="border-b">
            <th className="py-2 px-4 text-left font-medium text-muted-foreground">
              Komponen
            </th>
            <th className="py-2 px-4 text-right font-medium text-muted-foreground">
              Jumlah (IDR)
            </th>
            <th className="py-2 px-4 text-left font-medium text-muted-foreground w-16">
              Ket
            </th>
          </tr>
        </thead>
        <tbody>
          {components.map((comp, i) => (
            <tr
              key={i}
              className={cn(
                "border-b",
                comp.isClosing
                  ? "bg-muted/40 font-bold"
                  : "hover:bg-muted/20",
              )}
            >
              <td className="py-2.5 px-4">
                <span
                  className={cn(
                    comp.sign === "+" && "text-red-700",
                    comp.sign === "-" && "text-green-700",
                    comp.sign === "±" && "text-muted-foreground",
                    comp.isClosing && "font-bold",
                  )}
                >
                  {comp.sign !== "=" && !comp.isClosing && (
                    <span className="mr-1 font-mono">{comp.sign}</span>
                  )}
                  {comp.komponen}
                </span>
              </td>
              <td className="py-2.5 px-4 text-right font-mono text-xs">
                {comp.isPhase5Deferred || comp.jumlahIdr === null ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-muted-foreground cursor-help">—</span>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p className="text-xs">Data belum tersedia (Phase 5)</p>
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  <span className={cn(comp.isClosing && "text-base font-bold")}>
                    {comp.sign === "-"
                      ? `−${formatIDR(comp.jumlahIdr)}`
                      : comp.sign === "+"
                        ? `+${formatIDR(comp.jumlahIdr)}`
                        : formatIDR(comp.jumlahIdr)}
                  </span>
                )}
              </td>
              <td className="py-2.5 px-4">
                {comp.isPhase5Deferred && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Info
                        className="h-3.5 w-3.5 text-amber-500 cursor-help"
                        aria-label="Info Phase 5 defer"
                      />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p className="text-xs max-w-xs">
                        Transfer antar stage dan remeasurements akan tersedia
                        setelah Phase 5 (GL/jurnal engine) selesai.
                      </p>
                    </TooltipContent>
                  </Tooltip>
                )}
                {comp.isClosing && (
                  <span className="text-xs text-muted-foreground">(Closing)</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
