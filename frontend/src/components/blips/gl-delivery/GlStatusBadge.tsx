"use client";

import * as React from "react";
import {
  Clock,
  Loader2,
  RefreshCw,
  XCircle,
  CheckCircle2,
  Skull,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { GlHostStatus } from "@/lib/schemas/gl-delivery.schema";
import { GL_HOST_STATUS_LABELS } from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// Badge config per status (WCAG 2.1 AA: color + icon + text)
// ---------------------------------------------------------------------------

interface BadgeConfig {
  icon: LucideIcon;
  colorClass: string;
  spin?: boolean;
}

const BADGE_CONFIG: Record<GlHostStatus, BadgeConfig> = {
  PENDING_DELIVERY: {
    icon: Clock,
    colorClass: "bg-slate-100 text-slate-700 border-slate-300",
  },
  DELIVERY_IN_FLIGHT: {
    icon: Loader2,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    spin: true,
  },
  RETRYING: {
    icon: RefreshCw,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    spin: true,
  },
  FAILED: {
    icon: XCircle,
    colorClass: "bg-red-50 text-red-700 border-red-300",
  },
  DELIVERED: {
    icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-700 border-green-300",
  },
  DEAD_LETTER: {
    icon: Skull,
    colorClass: "bg-red-950 text-red-200 border-red-800",
  },
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

export interface GlStatusBadgeProps {
  status: GlHostStatus;
  size?: "sm" | "md";
  showIcon?: boolean;
  showLabel?: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC2, S2-AC3)
// ---------------------------------------------------------------------------

export function GlStatusBadge({
  status,
  size = "md",
  showIcon = true,
  showLabel = true,
  className,
}: GlStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const Icon = config.icon;
  const label = GL_HOST_STATUS_LABELS[status];

  return (
    <span
      role="status"
      aria-label={`Status pengiriman GL: ${label}`}
      className={cn(
        "inline-flex items-center rounded-md border font-medium",
        SIZE_CLASS[size],
        config.colorClass,
        className,
      )}
    >
      {showIcon && (
        <Icon
          className={cn(
            ICON_SIZE[size],
            config.spin && "animate-spin motion-reduce:animate-none",
          )}
          aria-hidden="true"
        />
      )}
      {showLabel && <span>{label}</span>}
    </span>
  );
}
