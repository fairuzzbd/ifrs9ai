/**
 * ExportFormatBadge — badge for export format (CSV / XLSX / PDF).
 * WCAG AA compliant color pairs.
 */

import * as React from "react";
import { cn } from "@/lib/utils";
import type { ExportFormat } from "@/lib/schemas/reporting.schema";
import { EXPORT_FORMAT_LABELS } from "@/lib/schemas/reporting.schema";

interface ExportFormatBadgeProps {
  format: ExportFormat;
  className?: string;
}

const FORMAT_STYLES: Record<ExportFormat, string> = {
  // teal: contrast ~5.3:1
  csv: "bg-teal-50 text-teal-800 border-teal-200",
  // blue: contrast ~5.2:1
  xlsx: "bg-blue-50 text-blue-800 border-blue-200",
  // violet: contrast ~5.0:1
  pdf: "bg-violet-50 text-violet-800 border-violet-200",
};

export function ExportFormatBadge({ format, className }: ExportFormatBadgeProps) {
  const label = EXPORT_FORMAT_LABELS[format] ?? format.toUpperCase();

  return (
    <span
      aria-label={`Format: ${label}`}
      className={cn(
        "inline-flex items-center rounded border px-2 py-0.5 text-xs font-mono font-semibold",
        FORMAT_STYLES[format],
        className,
      )}
    >
      {label}
    </span>
  );
}
