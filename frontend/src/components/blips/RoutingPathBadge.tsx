"use client";

import * as React from "react";
import { User, RefreshCw, Layers, Upload } from "lucide-react";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type TriggerSource =
  | "MANUAL"
  | "CRON_DRIFT"
  | "AD_HOC_BULK"
  | "DOCUMENT_UPLOAD";

export interface RoutingPathBadgeProps {
  triggerSource: TriggerSource;
  className?: string;
}

// ---------------------------------------------------------------------------
// Token map
// ---------------------------------------------------------------------------

const TOKEN_MAP: Record<
  TriggerSource,
  { label: string; bg: string; text: string; Icon: React.ElementType }
> = {
  MANUAL: {
    label: "Manual",
    bg: "bg-gray-100",
    text: "text-gray-700",
    Icon: User,
  },
  CRON_DRIFT: {
    label: "AUTO (Drift)",
    bg: "bg-amber-100",
    text: "text-amber-800",
    Icon: RefreshCw,
  },
  AD_HOC_BULK: {
    label: "Bulk Ad-Hoc",
    bg: "bg-blue-100",
    text: "text-blue-800",
    Icon: Layers,
  },
  DOCUMENT_UPLOAD: {
    label: "Upload Dokumen",
    bg: "bg-purple-100",
    text: "text-purple-800",
    Icon: Upload,
  },
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RoutingPathBadge({ triggerSource, className }: RoutingPathBadgeProps) {
  const tokens = TOKEN_MAP[triggerSource] ?? TOKEN_MAP["MANUAL"];
  const { label, bg, text, Icon } = tokens;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 px-2 py-0.5 text-xs font-medium rounded-full",
        bg,
        text,
        className,
      )}
      aria-label={`Sumber trigger: ${label}`}
    >
      <Icon className="h-3 w-3" aria-hidden="true" />
      {label}
    </span>
  );
}
