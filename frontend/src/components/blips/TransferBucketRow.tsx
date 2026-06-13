"use client";

import * as React from "react";
import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { TransferBucket } from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const isNeg = value.startsWith("-");
  const abs = isNeg ? value.slice(1) : value;
  const num = parseFloat(abs);
  if (isNaN(num)) return value;
  const formatted = new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
  return isNeg ? `−${formatted}` : formatted;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface TransferBucketRowProps {
  label: string;
  bucket: TransferBucket;
  /** Sign: "+" = increase in allowance (loss); "-" = decrease (cure/release) */
  sign: "+" | "-";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function TransferBucketRow({
  label,
  bucket,
  sign,
  className,
}: TransferBucketRowProps) {
  const isPositive = sign === "+";
  const amountText = isPositive
    ? `+${formatIDR(bucket.eclMovementIdr)}`
    : formatIDR(bucket.eclMovementIdr);

  return (
    <tr
      className={cn(
        "border-b hover:bg-muted/20 transition-colors",
        className,
      )}
    >
      {/* Komponen label — indented sub-row */}
      <td className="py-2 px-4 pl-8">
        <span
          className={cn(
            "text-sm",
            isPositive ? "text-red-700" : "text-green-700",
          )}
        >
          <span className="mr-1 font-mono text-xs">{sign}</span>
          {label}
        </span>
      </td>

      {/* ECL movement */}
      <td className="py-2 px-4 text-right font-mono text-xs">
        <span className={cn(isPositive ? "text-red-700" : "text-green-700")}>
          {amountText}
        </span>
      </td>

      {/* Count + override badge */}
      <td className="py-2 px-4">
        <div className="flex items-center gap-1.5">
          <span className="text-xs text-muted-foreground">
            {bucket.count} instr.
          </span>
          {bucket.countOverride > 0 && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge
                  variant="outline"
                  className="text-xs px-1.5 py-0 border-amber-400 text-amber-700 bg-amber-50 cursor-help"
                >
                  {bucket.countOverride} override
                </Badge>
              </TooltipTrigger>
              <TooltipContent>
                <p className="text-xs max-w-xs">
                  {bucket.countOverride} instrumen dipindahkan melalui Management Override
                  (trigger_type = MANAGEMENT_OVERRIDE di ecl.stage_history).
                </p>
              </TooltipContent>
            </Tooltip>
          )}
        </div>
      </td>
    </tr>
  );
}
