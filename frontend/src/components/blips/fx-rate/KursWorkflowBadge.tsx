"use client";

import * as React from "react";
import { Clock, CheckCircle2, XCircle, Lock } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { KursWorkflowStatusP5 } from "@/lib/schemas/fx-rate.schema";
import { WORKFLOW_STATUS_P5_LABELS } from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Badge config — 4 visual states (WCAG 2.1 AA: color + icon + text)
// ---------------------------------------------------------------------------

// Extended state includes LOCKED (APPROVED + locked_flag=TRUE)
export type KursDisplayState = KursWorkflowStatusP5 | "LOCKED";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
}

const BADGE_CONFIG: Record<KursDisplayState, BadgeConfig> = {
  PENDING_APPROVAL: {
    Icon: Clock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "Menunggu approval Finance Controller (ROLE-AKUN-CTL).",
  },
  APPROVED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-800 border-green-400",
    tooltip: "Kurs disetujui dan aktif digunakan dalam sistem.",
  },
  REJECTED: {
    Icon: XCircle,
    colorClass: "bg-red-50 text-red-700 border-red-300",
    tooltip: "Kurs ditolak oleh Finance Controller. Harap re-upload.",
  },
  LOCKED: {
    Icon: Lock,
    colorClass: "bg-slate-100 text-slate-600 border-slate-300",
    tooltip: "Kurs dikunci karena periode hard-closed. Tidak bisa diubah.",
  },
};

const DISPLAY_LABELS: Record<KursDisplayState, string> = {
  ...WORKFLOW_STATUS_P5_LABELS,
  LOCKED: "Terkunci",
};

const SIZE_CLASS = {
  sm: "text-xs px-1.5 py-0.5 gap-1",
  md: "text-sm px-2 py-1 gap-1.5",
};

const ICON_SIZE = {
  sm: "h-3 w-3",
  md: "h-4 w-4",
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KursWorkflowBadgeProps {
  /** workflow_status from mst.kurs — PENDING_APPROVAL | APPROVED | REJECTED */
  workflowStatus: KursWorkflowStatusP5;
  /** If true, displays the LOCKED visual state (approved + locked_flag=TRUE) */
  lockedFlag?: boolean;
  size?: "sm" | "md";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S1-AC1 list display, S4-AC1 locked state, WCAG AA)
// ---------------------------------------------------------------------------

export function KursWorkflowBadge({
  workflowStatus,
  lockedFlag = false,
  size = "md",
  className,
}: KursWorkflowBadgeProps) {
  // LOCKED takes precedence over APPROVED if locked_flag=TRUE
  const displayState: KursDisplayState =
    lockedFlag && workflowStatus === "APPROVED" ? "LOCKED" : workflowStatus;

  const config = BADGE_CONFIG[displayState];
  const { Icon } = config;
  const label = DISPLAY_LABELS[displayState];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            SIZE_CLASS[size],
            config.colorClass,
            className,
          )}
          role="status"
          aria-label={`Status kurs: ${label}`}
        >
          <Icon className={cn(ICON_SIZE[size])} aria-hidden="true" />
          <span>{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{config.tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
