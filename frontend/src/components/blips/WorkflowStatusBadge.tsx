import * as React from "react";
import { Circle, CheckCircle2, CornerDownLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import type { MasterWorkflowState } from "@/lib/schemas/mata-uang.schema";

/**
 * Extended status type: covers both mata-uang (RETURNED) and impact FL modules (REJECTED).
 * WorkflowStatusBadge accepts either variant.
 */
type ExtendedWorkflowState = MasterWorkflowState | "REJECTED";

interface StatusConfig {
  label: string;
  bgColor: string;
  textColor: string;
  borderColor: string;
  icon: React.ReactNode;
}

const STATUS_CONFIG: Record<ExtendedWorkflowState, StatusConfig> = {
  DRAFT: {
    label: "Draf",
    bgColor: "bg-[var(--blips-status-draft-bg)]",
    textColor: "text-[var(--blips-status-draft-text)]",
    borderColor: "border-[var(--blips-status-draft-border)]",
    icon: <Circle className="h-3 w-3" aria-hidden />,
  },
  PENDING_REVIEW: {
    label: "Menunggu Review",
    bgColor: "bg-[var(--blips-status-pending-review-bg)]",
    textColor: "text-[var(--blips-status-pending-review-text)]",
    borderColor: "border-[var(--blips-status-pending-review-border)]",
    icon: (
      <svg
        className="h-3 w-3"
        viewBox="0 0 16 16"
        fill="currentColor"
        aria-hidden
      >
        <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 12A5 5 0 0 1 8 3v10z" />
      </svg>
    ),
  },
  PENDING_APPROVAL: {
    label: "Menunggu Approval",
    bgColor: "bg-[var(--blips-status-pending-approval-bg)]",
    textColor: "text-[var(--blips-status-pending-approval-text)]",
    borderColor: "border-[var(--blips-status-pending-approval-border)]",
    icon: (
      <svg
        className="h-3 w-3"
        viewBox="0 0 16 16"
        fill="currentColor"
        aria-hidden
      >
        <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 12A5 5 0 0 1 8 3v10z" />
      </svg>
    ),
  },
  PENDING_APPROVAL_2: {
    label: "Menunggu Approval 2",
    bgColor: "bg-[var(--blips-status-pending-approval-2-bg)]",
    textColor: "text-[var(--blips-status-pending-approval-2-text)]",
    borderColor: "border-[var(--blips-status-pending-approval-2-border)]",
    icon: (
      <svg
        className="h-3 w-3"
        viewBox="0 0 16 16"
        fill="currentColor"
        aria-hidden
      >
        <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 12A5 5 0 0 1 8 3v10z" />
      </svg>
    ),
  },
  APPROVED: {
    label: "Disetujui",
    bgColor: "bg-[var(--blips-status-approved-bg)]",
    textColor: "text-[var(--blips-status-approved-text)]",
    borderColor: "border-[var(--blips-status-approved-border)]",
    icon: <CheckCircle2 className="h-3 w-3" aria-hidden />,
  },
  RETURNED: {
    label: "Dikembalikan",
    bgColor: "bg-[var(--blips-status-returned-bg)]",
    textColor: "text-[var(--blips-status-returned-text)]",
    borderColor: "border-[var(--blips-status-returned-border)]",
    icon: <CornerDownLeft className="h-3 w-3" aria-hidden />,
  },
  REJECTED: {
    label: "Ditolak",
    bgColor: "bg-[var(--blips-status-returned-bg)]",
    textColor: "text-[var(--blips-status-returned-text)]",
    borderColor: "border-[var(--blips-status-returned-border)]",
    icon: <CornerDownLeft className="h-3 w-3" aria-hidden />,
  },
};

interface WorkflowStatusBadgeProps {
  /** Workflow status string. Accepts any string; unknown values render nothing. */
  status: string;
  size?: "sm" | "default";
  className?: string;
}

export function WorkflowStatusBadge({
  status,
  size = "default",
  className,
}: WorkflowStatusBadgeProps) {
  const config = STATUS_CONFIG[status as ExtendedWorkflowState];
  if (!config) return null;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border font-medium",
        config.bgColor,
        config.textColor,
        config.borderColor,
        size === "sm" ? "px-2 py-0.5 text-xs" : "px-2.5 py-0.5 text-xs",
        className,
      )}
      aria-label={`Status: ${config.label}`}
    >
      {config.icon}
      {config.label}
    </span>
  );
}
