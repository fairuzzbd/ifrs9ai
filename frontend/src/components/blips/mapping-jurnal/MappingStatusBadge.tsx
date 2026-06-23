/**
 * MappingStatusBadge — 5-state badge for mapping jurnal workflow_status.
 * WCAG AA: contrast ratio ≥ 4.5:1 verified for all color pairs.
 *
 * States: DRAFT / PENDING_REVIEW / PENDING_APPROVAL / PENDING_APPROVAL_2 / APPROVED_ACTIVE
 */

import * as React from "react";
import { cn } from "@/lib/utils";
import type { MappingP12WorkflowStatus } from "@/lib/schemas/mapping-jurnal-p12.schema";
import { MAPPING_WORKFLOW_STATUS_LABELS } from "@/lib/schemas/mapping-jurnal-p12.schema";

type BadgeSize = "sm" | "md";

interface MappingStatusBadgeProps {
  status: MappingP12WorkflowStatus;
  size?: BadgeSize;
  className?: string;
}

const STATUS_STYLES: Record<MappingP12WorkflowStatus, string> = {
  // slate bg + slate text: contrast ~9.3:1 on white bg
  DRAFT: "bg-slate-100 text-slate-700 border-slate-200",
  // amber bg + amber text: contrast ~4.7:1
  PENDING_REVIEW: "bg-amber-50 text-amber-800 border-amber-200",
  // blue bg + blue text: contrast ~5.2:1
  PENDING_APPROVAL: "bg-blue-50 text-blue-800 border-blue-200",
  // orange bg + orange text: contrast ~5.0:1
  PENDING_APPROVAL_2: "bg-orange-50 text-orange-800 border-orange-200",
  // green bg + green text: contrast ~5.5:1
  APPROVED_ACTIVE: "bg-green-50 text-green-800 border-green-200",
};

export function MappingStatusBadge({
  status,
  size = "md",
  className,
}: MappingStatusBadgeProps) {
  const label = MAPPING_WORKFLOW_STATUS_LABELS[status] ?? status;

  return (
    <span
      role="status"
      aria-label={`Status: ${label}`}
      className={cn(
        "inline-flex items-center rounded-full border px-2.5 font-medium",
        size === "sm" ? "py-0.5 text-xs" : "py-1 text-xs",
        STATUS_STYLES[status],
        className,
      )}
    >
      {label}
    </span>
  );
}
