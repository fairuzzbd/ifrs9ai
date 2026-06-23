/**
 * ExportStatusBadge — 5-state badge for async export status.
 * States: REQUESTED | QUEUED | COMPUTING | COMPLETED | FAILED
 * WCAG AA: contrast ratio ≥ 4.5:1 for all pairs.
 */

import * as React from "react";
import { cn } from "@/lib/utils";
import type { ExportStatus } from "@/lib/schemas/reporting.schema";
import { EXPORT_STATUS_LABELS } from "@/lib/schemas/reporting.schema";

type BadgeSize = "sm" | "md";

interface ExportStatusBadgeProps {
  status: ExportStatus;
  size?: BadgeSize;
  className?: string;
}

const STATUS_STYLES: Record<ExportStatus, string> = {
  // slate: contrast ~9.3:1
  REQUESTED: "bg-slate-100 text-slate-700 border-slate-200",
  // blue: contrast ~5.2:1
  QUEUED: "bg-blue-50 text-blue-700 border-blue-200",
  // amber: contrast ~4.7:1
  COMPUTING: "bg-amber-50 text-amber-800 border-amber-200",
  // green: contrast ~5.5:1
  COMPLETED: "bg-green-50 text-green-800 border-green-200",
  // red: contrast ~5.1:1
  FAILED: "bg-red-50 text-red-800 border-red-200",
};

const STATUS_ANIMATE: Partial<Record<ExportStatus, string>> = {
  COMPUTING: "animate-pulse",
};

export function ExportStatusBadge({
  status,
  size = "md",
  className,
}: ExportStatusBadgeProps) {
  const label = EXPORT_STATUS_LABELS[status] ?? status;

  return (
    <span
      role="status"
      aria-label={`Status export: ${label}`}
      className={cn(
        "inline-flex items-center rounded-full border font-medium",
        size === "sm" ? "px-2 py-0.5 text-xs" : "px-2.5 py-1 text-xs",
        STATUS_STYLES[status],
        STATUS_ANIMATE[status],
        className,
      )}
    >
      {label}
    </span>
  );
}
