"use client";

import * as React from "react";
import {
  Circle,
  Loader2,
  CheckCircle2,
  AlertTriangle,
  Clock,
  Lock,
  XCircle,
  ShieldX,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { CalcRunStatus } from "@/lib/schemas/calc-run.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CalcRunStatusBadgeProps {
  status: CalcRunStatus;
  size?: "sm" | "default" | "lg";
  showIcon?: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Token maps (WCAG 2.1 AA contrast-checked)
// ---------------------------------------------------------------------------

interface StatusToken {
  bg: string;
  text: string;
  icon: React.ElementType;
  label: string;
  ariaLabel: string;
  animate?: boolean;
}

const TOKEN_MAP: Record<CalcRunStatus, StatusToken> = {
  DRAFT: {
    bg: "bg-gray-100",
    text: "text-gray-700",
    icon: Circle,
    label: "DRAFT",
    ariaLabel: "Status: DRAFT",
  },
  IN_PROGRESS: {
    bg: "bg-blue-100",
    text: "text-blue-800",
    icon: Loader2,
    label: "Sedang Berjalan",
    ariaLabel: "Status: Sedang Berjalan",
    animate: true,
  },
  COMPLETED: {
    bg: "bg-green-100",
    text: "text-green-800",
    icon: CheckCircle2,
    label: "SELESAI",
    ariaLabel: "Status: SELESAI",
  },
  COMPLETED_WITH_ERRORS: {
    bg: "bg-amber-100",
    text: "text-amber-800",
    icon: AlertTriangle,
    label: "SELESAI dengan Error",
    ariaLabel: "Status: SELESAI dengan Error",
  },
  SEAL_REQUESTED: {
    bg: "bg-yellow-100",
    text: "text-yellow-800",
    icon: Clock,
    label: "Menunggu Segel",
    ariaLabel: "Status: Menunggu Segel",
  },
  SEALED: {
    bg: "bg-purple-100",
    text: "text-purple-800",
    icon: Lock,
    label: "TERSEGEL",
    ariaLabel: "Status: TERSEGEL",
  },
  SEAL_REJECTED: {
    bg: "bg-orange-100",
    text: "text-orange-800",
    icon: ShieldX,
    label: "Segel Ditolak",
    ariaLabel: "Status: Segel Ditolak",
  },
  CANCELLED: {
    bg: "bg-red-50",
    text: "text-red-700",
    icon: XCircle,
    label: "DIBATALKAN",
    ariaLabel: "Status: DIBATALKAN",
  },
};

const SIZE_CLASSES = {
  sm: { container: "px-2 py-0.5 text-xs gap-1 rounded", icon: "h-3 w-3" },
  default: { container: "px-3 py-1 text-sm gap-1.5 rounded-md", icon: "h-4 w-4" },
  lg: { container: "px-4 py-2 text-base gap-2 rounded-lg", icon: "h-5 w-5" },
} as const;

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CalcRunStatusBadge({
  status,
  size = "default",
  showIcon = true,
  className,
}: CalcRunStatusBadgeProps) {
  const tokens = TOKEN_MAP[status] ?? TOKEN_MAP["DRAFT"];
  const sizes = SIZE_CLASSES[size];
  const Icon = tokens.icon;

  return (
    <span
      aria-label={tokens.ariaLabel}
      className={cn(
        "inline-flex items-center font-medium",
        tokens.bg,
        tokens.text,
        sizes.container,
        className,
      )}
    >
      {showIcon && (
        <Icon
          className={cn(sizes.icon, "flex-shrink-0", tokens.animate && "animate-spin")}
          aria-hidden="true"
        />
      )}
      <span>{tokens.label}</span>
    </span>
  );
}
