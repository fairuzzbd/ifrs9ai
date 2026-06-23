"use client";

import * as React from "react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { StalePriceReason } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Reason labels
// ---------------------------------------------------------------------------

const REASON_LABELS: Record<StalePriceReason, string> = {
  HARGA_TIDAK_TERSEDIA: "Harga tidak tersedia dari feed",
  KURS_FCY_TIDAK_TERSEDIA: "Kurs mata uang asing belum tersedia (APPROVED)",
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmStaleBadgeProps {
  /** harga_age_days from trx.mtm */
  hargaAgeDays: number;
  /** reason for stale: harga not found OR kurs FCY not available */
  stalePriceReason: StalePriceReason;
  /** TRUE if harga_age_days > MTM_STALE_ESCALATION_DAYS (default 7) */
  escalated: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (amber normal; red escalation — WCAG AA: color + text)
// ---------------------------------------------------------------------------

export function MtmStaleBadge({
  hargaAgeDays,
  stalePriceReason,
  escalated,
  className,
}: MtmStaleBadgeProps) {
  const label = escalated
    ? `ESKALASI ${hargaAgeDays} hari`
    : `STALE ${hargaAgeDays} hari`;

  const colorClass = escalated
    ? "bg-red-100 text-red-700 border-red-300"
    : "bg-amber-100 text-amber-700 border-amber-300";

  const reasonText = REASON_LABELS[stalePriceReason];
  const escalationNote = escalated
    ? " Eskalasi ke Risk Officer sudah dikirim."
    : "";

  const ariaLabel = escalated
    ? `Harga kedaluwarsa: ${hargaAgeDays} hari, dieskalasi`
    : `Harga kedaluwarsa: ${hargaAgeDays} hari`;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border px-1.5 py-0.5 text-xs font-semibold cursor-default",
            colorClass,
            className,
          )}
          aria-label={ariaLabel}
        >
          {label}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">
          {reasonText}.{escalationNote}
        </p>
      </TooltipContent>
    </Tooltip>
  );
}
