"use client";

import * as React from "react";
import { CheckCircle2, Clock, XCircle } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { ReconcileStatus } from "@/lib/schemas/ecl-core.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ReconcileBadgeProps {
  status: ReconcileStatus;
  tooltip?: string;
  className?: string;
}

// ---------------------------------------------------------------------------
// Token map (warna + ikon + teks — bukan warna saja, color-blind safe)
// ---------------------------------------------------------------------------

const TOKEN_MAP: Record<
  ReconcileStatus,
  { label: string; bg: string; text: string; icon: React.ElementType }
> = {
  RECONCILED: {
    label: "REKONSILIASI OK",
    bg: "bg-green-100",
    text: "text-green-800",
    icon: CheckCircle2,
  },
  PARTIAL_PHASE_5_DEFER: {
    label: "PARTIAL — Fase 5 Defer",
    bg: "bg-amber-100",
    text: "text-amber-800",
    icon: Clock,
  },
  MISMATCH: {
    label: "MISMATCH",
    bg: "bg-red-100",
    text: "text-red-800",
    icon: XCircle,
  },
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReconcileBadge({ status, tooltip, className }: ReconcileBadgeProps) {
  const tokens = TOKEN_MAP[status];
  const Icon = tokens.icon;

  const badge = (
    <span
      aria-label={`Status rekonsiliasi: ${tokens.label}`}
      className={cn(
        "inline-flex items-center gap-1.5 px-3 py-1 text-sm font-medium rounded-md",
        tokens.bg,
        tokens.text,
        className,
      )}
    >
      <Icon className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
      {tokens.label}
    </span>
  );

  if (tooltip) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{badge}</TooltipTrigger>
        <TooltipContent>
          <p className="max-w-sm text-xs">{tooltip}</p>
        </TooltipContent>
      </Tooltip>
    );
  }

  return badge;
}
