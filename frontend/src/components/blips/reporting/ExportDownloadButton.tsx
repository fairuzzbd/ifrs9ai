/**
 * ExportDownloadButton — triggers download for a completed export.
 * Uses signed MinIO URL for async exports, direct stream for inline.
 * Audit EXPORT.DOWNLOADED is written server-side on redirect/stream.
 */

"use client";

import * as React from "react";
import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { exportApi } from "@/lib/api/reporting.api";
import type { ExportLogItem } from "@/lib/schemas/reporting.schema";

interface ExportDownloadButtonProps {
  exportItem: ExportLogItem;
  className?: string;
}

export function ExportDownloadButton({ exportItem, className }: ExportDownloadButtonProps) {
  const isAvailable =
    exportItem.status === "COMPLETED" &&
    (!exportItem.expiresAt || new Date(exportItem.expiresAt) > new Date());

  if (!isAvailable) return null;

  const handleDownload = () => {
    const url = exportApi.downloadUrl(exportItem.id);
    // Backend returns 302 to MinIO signed URL or 200 stream
    const a = document.createElement("a");
    a.href = url;
    a.download = `${exportItem.reportSlug}.${exportItem.format}`;
    a.rel = "noopener noreferrer";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
  };

  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={handleDownload}
      aria-label={`Unduh export ${exportItem.reportSlug} format ${exportItem.format.toUpperCase()}`}
      className={className}
    >
      <Download className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
      Unduh
    </Button>
  );
}
