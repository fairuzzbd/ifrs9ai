"use client";

import * as React from "react";
import { AlertTriangle } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { formatDistanceToNow, parseISO } from "date-fns";
import { id as localeId } from "date-fns/locale";

export interface StaleStagingBadgeProps {
  /** ISO date-time of the last sealed ECL run */
  lastSealedAt?: string | null;
  /** Number of days since sealed (pre-computed from server for convenience) */
  daysSinceSealed?: number | null;
  size?: "sm" | "default";
  className?: string;
}

export function StaleStagingBadge({
  lastSealedAt,
  daysSinceSealed,
  size = "default",
  className,
}: StaleStagingBadgeProps) {
  const daysLabel = daysSinceSealed != null ? `${daysSinceSealed} hari` : null;

  const relativeLabel = React.useMemo(() => {
    if (!lastSealedAt) return null;
    try {
      return formatDistanceToNow(parseISO(lastSealedAt), {
        addSuffix: true,
        locale: localeId,
      });
    } catch {
      return null;
    }
  }, [lastSealedAt]);

  const tooltipText = [
    "Staging stale: ECL sealed run terakhir",
    relativeLabel ?? daysLabel ?? "lebih dari 30 hari",
    "lalu.",
    "Akrual Stage 3 mungkin tidak akurat.",
    "ROLE-AKUN-CTL harus override atau ECL rerun diperlukan.",
  ]
    .filter(Boolean)
    .join(" ");

  const sizeClass = size === "sm" ? "text-xs px-1.5 py-0.5 gap-1" : "text-sm px-2 py-1 gap-1.5";
  const iconSize = size === "sm" ? "h-3 w-3" : "h-4 w-4";

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            "bg-amber-50 text-amber-700 border-amber-400",
            sizeClass,
            className,
          )}
          role="alert"
          aria-label={`Peringatan staging stale: ${daysLabel ?? "lebih dari 30 hari"} sejak ECL di-sealed`}
        >
          <AlertTriangle className={cn(iconSize, "flex-shrink-0")} aria-hidden="true" />
          <span>STAGING STALE</span>
          {daysLabel && (
            <span className="ml-1 text-amber-600">({daysLabel})</span>
          )}
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{tooltipText}</p>
      </TooltipContent>
    </Tooltip>
  );
}
