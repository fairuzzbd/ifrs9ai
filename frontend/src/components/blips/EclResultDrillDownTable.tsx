"use client";

import * as React from "react";
import { Info } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { ScenarioLine } from "@/lib/schemas/ecl-core.schema";

// ---------------------------------------------------------------------------
// IDR formatter (titik ribuan, koma desimal, 4 desimal)
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value || value === "0") return "Rp 0,0000";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
}

function formatRate(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return `${(num * 100).toFixed(8)}%`;
}

function formatMultiplier(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return num.toFixed(8);
}

function formatWeight(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return `${(num * 100).toFixed(2)}%`;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface EclResultDrillDownTableProps {
  scenarioBreakdown: ScenarioLine[];
  eclWeightedIdr: string;
  stage: 1 | 2 | 3 | null;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function EclResultDrillDownTable({
  scenarioBreakdown,
  eclWeightedIdr,
  stage,
  className,
}: EclResultDrillDownTableProps) {
  const good = scenarioBreakdown.find((s) => s.scenario === "GOOD");
  const normal = scenarioBreakdown.find((s) => s.scenario === "NORMAL");
  const bad = scenarioBreakdown.find((s) => s.scenario === "BAD");

  const goodWeight = formatWeight(good?.weight);
  const normalWeight = formatWeight(normal?.weight);
  const badWeight = formatWeight(bad?.weight);

  const isStage3 = stage === 3;

  const formulaTooltip = `ECL_weighted = ECL_FL_Good × ${good?.weight ?? "0.25"} + ECL_FL_Normal × ${normal?.weight ?? "0.50"} + ECL_FL_Bad × ${bad?.weight ?? "0.25"} = ${formatIDR(eclWeightedIdr)}`;

  return (
    <div className={cn("overflow-x-auto", className)}>
      <table className="w-full text-sm border-collapse">
        <thead>
          <tr className="border-b">
            <th className="py-2 px-3 text-left font-medium text-muted-foreground w-36">
              Metrik
            </th>
            <th className="py-2 px-3 text-right font-medium text-green-800 bg-green-50">
              Good ({goodWeight})
            </th>
            <th className="py-2 px-3 text-right font-medium text-blue-800 bg-blue-50">
              Normal ({normalWeight})
            </th>
            <th className="py-2 px-3 text-right font-medium text-red-800 bg-red-50">
              Bad ({badWeight})
            </th>
          </tr>
        </thead>
        <tbody>
          {/* PD */}
          <tr className="border-b hover:bg-muted/30">
            <td className="py-2 px-3 font-medium text-muted-foreground">PD</td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatRate(good?.pdUsed)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatRate(normal?.pdUsed)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatRate(bad?.pdUsed)}
            </td>
          </tr>

          {/* LGD */}
          <tr className="border-b hover:bg-muted/30">
            <td className="py-2 px-3 font-medium text-muted-foreground">LGD</td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatRate(good?.lgdUsed)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatRate(normal?.lgdUsed)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatRate(bad?.lgdUsed)}
            </td>
          </tr>

          {/* EAD */}
          <tr className="border-b hover:bg-muted/30">
            <td className="py-2 px-3 font-medium text-muted-foreground">EAD (IDR)</td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(good?.eadIdr)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(normal?.eadIdr)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(bad?.eadIdr)}
            </td>
          </tr>

          {/* ECL Skenario */}
          <tr className="border-b hover:bg-muted/30">
            <td className="py-2 px-3 font-medium text-muted-foreground">ECL Skenario</td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(good?.eclScenarioIdr)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(normal?.eclScenarioIdr)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(bad?.eclScenarioIdr)}
            </td>
          </tr>

          {/* FL Multiplier */}
          <tr className="border-b hover:bg-muted/30">
            <td className="py-2 px-3 font-medium text-muted-foreground">FL Multiplier</td>
            {[good, normal, bad].map((sc, i) => (
              <td key={i} className="py-2 px-3 text-right font-mono text-xs">
                {isStage3 ? (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span className="text-muted-foreground cursor-help">N/A</span>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p className="max-w-xs text-xs">
                        FL multiplier tidak diaplikasikan untuk Stage 3 (PD sudah fixed = 1.0)
                      </p>
                    </TooltipContent>
                  </Tooltip>
                ) : (
                  formatMultiplier(sc?.flMultiplier)
                )}
              </td>
            ))}
          </tr>

          {/* ECL FL */}
          <tr className="border-b hover:bg-muted/30">
            <td className="py-2 px-3 font-medium text-muted-foreground">ECL FL</td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(good?.eclFlIdr)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(normal?.eclFlIdr)}
            </td>
            <td className="py-2 px-3 text-right font-mono text-xs">
              {formatIDR(bad?.eclFlIdr)}
            </td>
          </tr>

          {/* ECL Weighted total row */}
          <tr className="bg-muted/30 font-semibold">
            <td className="py-3 px-3 flex items-center gap-1.5">
              ECL Weighted (IDR)
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info
                    className="h-3.5 w-3.5 text-muted-foreground cursor-help"
                    aria-label="Lihat formula ECL weighted"
                  />
                </TooltipTrigger>
                <TooltipContent className="max-w-xs">
                  <p className="text-xs font-mono">{formulaTooltip}</p>
                </TooltipContent>
              </Tooltip>
            </td>
            <td
              colSpan={3}
              className="py-3 px-3 text-right font-mono text-base"
            >
              {formatIDR(eclWeightedIdr)}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}
