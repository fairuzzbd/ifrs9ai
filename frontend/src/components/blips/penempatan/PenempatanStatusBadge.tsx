"use client";

import * as React from "react";
import {
  CircleDashed,
  Clock,
  ClockCheck,
  CheckCircle,
  XCircle,
  Ban,
  CalendarCheck,
  XOctagon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { PenempatanWorkflowStatus } from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Status config
// ---------------------------------------------------------------------------

interface StatusConfig {
  label: string;
  color: string;
  bgColor: string;
  icon: React.ElementType;
}

const STATUS_MAP: Record<PenempatanWorkflowStatus, StatusConfig> = {
  DRAFT: {
    label: "Konsep",
    color: "text-gray-600",
    bgColor: "bg-gray-100",
    icon: CircleDashed,
  },
  PENDING_REVIEW: {
    label: "Menunggu Review",
    color: "text-amber-700",
    bgColor: "bg-amber-100",
    icon: Clock,
  },
  PENDING_APPROVAL: {
    label: "Menunggu Approval",
    color: "text-amber-700",
    bgColor: "bg-amber-100",
    icon: ClockCheck,
  },
  APPROVED_ACTIVE: {
    label: "Aktif",
    color: "text-green-700",
    bgColor: "bg-green-100",
    icon: CheckCircle,
  },
  TERMINATION_PENDING_REVIEW: {
    label: "Menunggu Review Terminasi",
    color: "text-purple-700",
    bgColor: "bg-purple-100",
    icon: Clock,
  },
  TERMINATION_PENDING_APPROVAL: {
    label: "Menunggu Approval Terminasi",
    color: "text-purple-700",
    bgColor: "bg-purple-100",
    icon: ClockCheck,
  },
  TERMINATED: {
    label: "Diterminasi",
    color: "text-purple-800",
    bgColor: "bg-purple-100",
    icon: XOctagon,
  },
  MATURED: {
    label: "Jatuh Tempo",
    color: "text-blue-700",
    bgColor: "bg-blue-100",
    icon: CalendarCheck,
  },
  CANCELLED: {
    label: "Dibatalkan",
    color: "text-gray-500",
    bgColor: "bg-gray-100",
    icon: Ban,
  },
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface PenempatanStatusBadgeProps {
  status: PenempatanWorkflowStatus;
  size?: "sm" | "md";
  className?: string;
}

export function PenempatanStatusBadge({
  status,
  size = "md",
  className,
}: PenempatanStatusBadgeProps) {
  const config = STATUS_MAP[status] ?? {
    label: status,
    color: "text-gray-500",
    bgColor: "bg-gray-100",
    icon: CircleDashed,
  };

  const Icon = config.icon;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full font-medium",
        config.bgColor,
        config.color,
        size === "sm" ? "px-2 py-0.5 text-xs" : "px-3 py-1 text-sm",
        className,
      )}
      role="status"
      aria-label={`Status: ${config.label}`}
    >
      <Icon
        className={size === "sm" ? "h-3 w-3" : "h-4 w-4"}
        aria-hidden="true"
      />
      {config.label}
    </span>
  );
}

export { STATUS_MAP };
