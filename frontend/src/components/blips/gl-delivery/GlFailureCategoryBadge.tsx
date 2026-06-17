"use client";

import * as React from "react";
import { cn } from "@/lib/utils";
import type { GlFailureCategory } from "@/lib/schemas/gl-delivery.schema";
import { FAILURE_CATEGORY_LABELS } from "@/lib/schemas/gl-delivery.schema";

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const CATEGORY_CLASS: Record<GlFailureCategory, string> = {
  DOMAIN:
    "border-red-600 text-red-700 bg-transparent hover:bg-red-50",
  INFRA:
    "border-amber-600 text-amber-700 bg-transparent hover:bg-amber-50",
};

const CATEGORY_TOOLTIP: Record<GlFailureCategory, string> = {
  DOMAIN:
    "Domain Error (4xx): GL Host menolak payload. Penyebab perlu diperbaiki sebelum retry.",
  INFRA:
    "Infra Error (5xx/timeout): GL Host tidak tersedia. Biasanya sembuh sendiri setelah GL Host pulih.",
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface GlFailureCategoryBadgeProps {
  category: GlFailureCategory;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function GlFailureCategoryBadge({
  category,
  className,
}: GlFailureCategoryBadgeProps) {
  return (
    <span
      title={CATEGORY_TOOLTIP[category]}
      aria-label={`Kategori error: ${FAILURE_CATEGORY_LABELS[category]}`}
      className={cn(
        "inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-medium cursor-default",
        CATEGORY_CLASS[category],
        className,
      )}
    >
      {FAILURE_CATEGORY_LABELS[category]}
    </span>
  );
}
