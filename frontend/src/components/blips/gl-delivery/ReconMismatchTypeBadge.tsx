"use client";

import * as React from "react";
import {
  ArrowRightFromLine,
  ArrowLeftFromLine,
  TriangleAlert,
  type LucideIcon,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { MismatchType } from "@/lib/schemas/gl-delivery.schema";
import { MISMATCH_TYPE_LABELS } from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// Config (§2 badge design)
// ---------------------------------------------------------------------------

interface MismatchConfig {
  icon: LucideIcon;
  colorClass: string;
  tooltip: string;
}

const MISMATCH_CONFIG: Record<MismatchType, MismatchConfig> = {
  BLIPS_ONLY: {
    icon: ArrowRightFromLine,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip:
      "Hanya di BLIPS: akun ini ada di BLIPS tapi tidak ditemukan di GL Host.",
  },
  GL_ONLY: {
    icon: ArrowLeftFromLine,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip:
      "Hanya di GL Host: akun ini ada di GL Host tapi tidak ada di BLIPS.",
  },
  AMOUNT_DIFF: {
    icon: TriangleAlert,
    colorClass: "bg-red-50 text-red-600 border-red-300",
    tooltip:
      "Selisih Jumlah: akun ada di kedua sistem namun jumlahnya berbeda melebihi toleransi.",
  },
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ReconMismatchTypeBadgeProps {
  type: MismatchType;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReconMismatchTypeBadge({
  type,
  className,
}: ReconMismatchTypeBadgeProps) {
  const config = MISMATCH_CONFIG[type];
  const Icon = config.icon;
  const label = MISMATCH_TYPE_LABELS[type];

  return (
    <span
      title={config.tooltip}
      aria-label={`Tipe mismatch: ${label} — ${config.tooltip}`}
      className={cn(
        "inline-flex items-center gap-1 rounded border px-1.5 py-0.5 text-xs font-medium",
        config.colorClass,
        className,
      )}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      <span>{label}</span>
    </span>
  );
}
