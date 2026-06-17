"use client";

import * as React from "react";
import { Circle, Lock, ClockAlert, LockKeyhole, Ban } from "lucide-react";
import { cn } from "@/lib/utils";
import type { StatusPeriode } from "@/lib/schemas/periode-close.schema";
import { STATUS_PERIODE_LABELS } from "@/lib/schemas/periode-close.schema";

// ---------------------------------------------------------------------------
// Badge config per status (WCAG 2.1 AA: color + icon + text)
// ---------------------------------------------------------------------------

interface BadgeConfig {
  colorClass: string;
  dotPulse?: boolean;
  Icon: React.ElementType;
}

const BADGE_CONFIG: Record<StatusPeriode, BadgeConfig> = {
  OPEN: {
    Icon: Circle,
    colorClass: "bg-slate-100 text-slate-700 border-slate-300",
  },
  SOFT_CLOSED: {
    Icon: Lock,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
  },
  HARD_CLOSE_PENDING: {
    Icon: ClockAlert,
    colorClass: "bg-orange-50 text-orange-700 border-orange-300",
    dotPulse: true,
  },
  CLOSED: {
    Icon: LockKeyhole,
    colorClass: "bg-green-50 text-green-800 border-green-400",
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
// Grace countdown hook
// ---------------------------------------------------------------------------

function useGraceCountdown(graceExpiresAt?: string): string | null {
  const [countdown, setCountdown] = React.useState<string | null>(null);

  React.useEffect(() => {
    if (!graceExpiresAt) return;

    const update = () => {
      const diff = new Date(graceExpiresAt).getTime() - Date.now();
      if (diff <= 0) {
        setCountdown(null);
        return;
      }
      const hours = Math.floor(diff / (1000 * 60 * 60));
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
      setCountdown(`${hours}j ${minutes}m`);
    };

    update();
    const id = setInterval(update, 60_000);
    return () => clearInterval(id);
  }, [graceExpiresAt]);

  return countdown;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PeriodeStatusBadgeProps {
  status: StatusPeriode;
  graceExpiresAt?: string;
  size?: "sm" | "md";
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S5-AC2)
// ---------------------------------------------------------------------------

export function PeriodeStatusBadge({
  status,
  graceExpiresAt,
  size = "md",
  className,
}: PeriodeStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = STATUS_PERIODE_LABELS[status];
  const graceCountdown = useGraceCountdown(graceExpiresAt);

  // Determine if grace is expired (CLOSED but no countdown)
  const isGraceExpired =
    status === "CLOSED" &&
    graceExpiresAt != null &&
    new Date(graceExpiresAt).getTime() < Date.now();

  return (
    <span
      className={cn("inline-flex items-center gap-1.5 flex-wrap", className)}
      role="status"
      aria-label={`Status periode: ${label}`}
    >
      {/* Main badge */}
      <span
        className={cn(
          "inline-flex items-center rounded-md border font-medium",
          SIZE_CLASS[size],
          config.colorClass,
        )}
      >
        {config.dotPulse ? (
          <span className="relative flex h-2 w-2" aria-hidden="true">
            <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-400 opacity-75 motion-reduce:animate-none" />
            <span className="relative inline-flex rounded-full h-2 w-2 bg-orange-500" />
          </span>
        ) : (
          <Icon className={cn(ICON_SIZE[size])} aria-hidden="true" />
        )}
        <span>{label}</span>
      </span>

      {/* Grace countdown sub-badge (CLOSED within grace window) */}
      {status === "CLOSED" && graceCountdown && (
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium",
            "text-xs px-1.5 py-0.5 gap-1",
            "bg-blue-50 text-blue-700 border-blue-300",
          )}
          aria-label={`Reopen tersedia dalam ${graceCountdown}`}
        >
          <LockKeyhole className="h-3 w-3" aria-hidden="true" />
          Reopen tersedia: {graceCountdown}
        </span>
      )}

      {/* Grace expired indicator */}
      {status === "CLOSED" && isGraceExpired && (
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium",
            "text-xs px-1.5 py-0.5 gap-1",
            "bg-slate-100 text-slate-500 border-slate-300",
          )}
          aria-label="Grace window telah berakhir, tidak bisa di-reopen"
        >
          <Ban className="h-3 w-3" aria-hidden="true" />
          Tidak bisa di-reopen
        </span>
      )}
    </span>
  );
}
