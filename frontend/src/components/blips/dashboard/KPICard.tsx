"use client";

/**
 * P5-M15 — KPICard: single metric with delta, sub-label, status color.
 */

import * as React from "react";
import { ArrowUp, ArrowDown, Minus, type LucideIcon } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export interface KPICardDelta {
  value: string;
  direction: "up" | "down" | "neutral";
  label: string;
}

export interface KPICardProps {
  title: string;
  value: string;
  /** Full value for screen readers, e.g. "Lima ratus miliar rupiah" */
  valueAriaLabel?: string;
  delta?: KPICardDelta;
  subLabel?: string;
  status?: "default" | "success" | "warning" | "danger";
  icon?: LucideIcon;
  loading?: boolean;
  error?: string;
  ariaLive?: "polite" | "off";
  className?: string;
}

const STATUS_COLORS: Record<string, string> = {
  default: "text-foreground",
  success: "text-green-700",
  warning: "text-amber-700",
  danger: "text-red-700",
};

const STATUS_BORDER: Record<string, string> = {
  default: "border-border",
  success: "border-green-200",
  warning: "border-amber-200",
  danger: "border-red-200",
};

const DELTA_COLORS: Record<string, string> = {
  up: "text-red-600",   // up ECL ratio = bad
  down: "text-green-600",
  neutral: "text-muted-foreground",
};

export function KPICard({
  title,
  value,
  valueAriaLabel,
  delta,
  subLabel,
  status = "default",
  icon: Icon,
  loading = false,
  error,
  ariaLive = "polite",
  className,
}: KPICardProps) {
  if (loading) {
    return (
      <div
        className={cn(
          "rounded-xl border p-4 space-y-2 bg-card",
          STATUS_BORDER[status],
          className,
        )}
        aria-busy="true"
      >
        <Skeleton className="h-4 w-2/3" />
        <Skeleton className="h-8 w-1/2" />
        <Skeleton className="h-3 w-3/4" />
      </div>
    );
  }

  if (error) {
    return (
      <div
        className={cn(
          "rounded-xl border border-destructive p-4",
          className,
        )}
        role="alert"
      >
        <p className="text-xs text-muted-foreground">{title}</p>
        <p className="text-sm text-destructive mt-1">{error}</p>
      </div>
    );
  }

  return (
    <div
      className={cn(
        "rounded-xl border p-4 bg-card",
        STATUS_BORDER[status],
        className,
      )}
      role="status"
      aria-live={ariaLive}
      aria-label={valueAriaLabel ? `${title}: ${valueAriaLabel}` : undefined}
    >
      <div className="flex items-center justify-between mb-1">
        <p className="text-xs font-medium text-muted-foreground truncate">
          {title}
        </p>
        {Icon && (
          <Icon
            className="h-4 w-4 text-muted-foreground flex-shrink-0"
            aria-hidden="true"
          />
        )}
      </div>

      <p
        className={cn("text-2xl font-bold tracking-tight", STATUS_COLORS[status])}
        aria-label={valueAriaLabel}
      >
        {value}
      </p>

      {delta && (
        <div className="flex items-center gap-1 mt-1">
          {delta.direction === "up" && (
            <ArrowUp className={cn("h-3 w-3", DELTA_COLORS.up)} aria-hidden="true" />
          )}
          {delta.direction === "down" && (
            <ArrowDown className={cn("h-3 w-3", DELTA_COLORS.down)} aria-hidden="true" />
          )}
          {delta.direction === "neutral" && (
            <Minus className={cn("h-3 w-3", DELTA_COLORS.neutral)} aria-hidden="true" />
          )}
          <span className={cn("text-xs", DELTA_COLORS[delta.direction])}>
            {delta.value}
          </span>
          <span className="text-xs text-muted-foreground">{delta.label}</span>
        </div>
      )}

      {subLabel && (
        <p className="text-xs text-muted-foreground mt-1 truncate">{subLabel}</p>
      )}
    </div>
  );
}
