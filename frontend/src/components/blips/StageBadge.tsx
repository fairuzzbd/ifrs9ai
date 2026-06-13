"use client";

import * as React from "react";
import { ShieldCheck, AlertTriangle, ShieldX, Circle } from "lucide-react";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface StageBadgeProps {
  stage: 1 | 2 | 3 | null;
  size?: "sm" | "default" | "lg";
  showIcon?: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Token maps (WCAG 2.1 AA contrast-checked per design spec)
// ---------------------------------------------------------------------------

interface StageToken {
  bg: string;
  text: string;
  icon: React.ElementType;
  label: string;
  ariaLabel: string;
}

const FALLBACK_TOKEN: StageToken = {
  bg: "bg-gray-100",
  text: "text-gray-600",
  icon: Circle,
  label: "Belum Dievaluasi",
  ariaLabel: "Stage: Belum Dievaluasi",
};

const TOKEN_MAP: Record<1 | 2 | 3, StageToken> = {
  1: {
    bg: "bg-green-100",
    text: "text-green-800",
    icon: ShieldCheck,
    label: "Stage 1 — Berkinerja Baik",
    ariaLabel: "Stage: Stage 1 — Berkinerja Baik",
  },
  2: {
    bg: "bg-amber-100",
    text: "text-amber-800",
    icon: AlertTriangle,
    label: "Stage 2 — Risiko Meningkat",
    ariaLabel: "Stage: Stage 2 — Risiko Meningkat",
  },
  3: {
    bg: "bg-red-100",
    text: "text-red-800",
    icon: ShieldX,
    label: "Stage 3 — Default",
    ariaLabel: "Stage: Stage 3 — Default",
  },
};

// ---------------------------------------------------------------------------
// Size classes
// ---------------------------------------------------------------------------

const SIZE_CLASSES = {
  sm: {
    container: "px-2 py-0.5 text-xs gap-1 rounded",
    icon: "h-3 w-3",
  },
  default: {
    container: "px-3 py-1 text-sm gap-1.5 rounded-md",
    icon: "h-4 w-4",
  },
  lg: {
    container: "px-4 py-3 text-base gap-2 rounded-lg w-full justify-center",
    icon: "h-5 w-5",
  },
} as const;

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function StageBadge({
  stage,
  size = "default",
  showIcon = true,
  className,
}: StageBadgeProps) {
  const tokens =
    stage !== null && stage !== undefined
      ? (TOKEN_MAP[stage] ?? FALLBACK_TOKEN)
      : FALLBACK_TOKEN;
  const sizes = SIZE_CLASSES[size];
  const Icon = tokens.icon;

  return (
    <span
      role={size === "lg" ? "status" : undefined}
      aria-label={tokens.ariaLabel}
      className={cn(
        "inline-flex items-center font-medium",
        tokens.bg,
        tokens.text,
        sizes.container,
        className,
      )}
    >
      {showIcon && <Icon className={cn(sizes.icon, "flex-shrink-0")} aria-hidden="true" />}
      <span>{tokens.label}</span>
    </span>
  );
}

// ---------------------------------------------------------------------------
// Short display (just "Stage N" for tables)
// ---------------------------------------------------------------------------

export function StageText({ stage }: { stage: 1 | 2 | 3 | null }) {
  if (stage === null) return <span className="text-gray-400">—</span>;
  return (
    <StageBadge stage={stage} size="sm" />
  );
}
