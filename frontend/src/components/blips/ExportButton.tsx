"use client";

import * as React from "react";
import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { notify } from "@/lib/notify";
import { API_BASE_URL } from "@/lib/api";

interface ExportButtonProps {
  /** Absolute API path, e.g. /api/v1/master/mata-uang/export */
  endpoint: string;
  filename: string;
  formats?: ("csv" | "xlsx")[];
  /** Current filter/sort query params to forward to export endpoint */
  queryParams?: Record<string, string | number | boolean | null | undefined>;
}

export function ExportButton({
  endpoint,
  filename,
  formats = ["csv", "xlsx"],
  queryParams = {},
}: ExportButtonProps) {
  const [exporting, setExporting] = React.useState(false);

  const handleExport = async (format: "csv" | "xlsx") => {
    setExporting(true);
    try {
      const params = new URLSearchParams();
      params.set("format", format);
      Object.entries(queryParams).forEach(([k, v]) => {
        if (v !== null && v !== undefined && v !== "") {
          params.set(k, String(v));
        }
      });

      const token =
        typeof window !== "undefined"
          ? localStorage.getItem("blips_token")
          : null;

      const response = await fetch(
        `${API_BASE_URL}${endpoint}?${params.toString()}`,
        {
          headers: {
            Accept:
              format === "xlsx"
                ? "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                : "text/csv",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
          },
        },
      );

      if (response.status === 202) {
        // Async job — for now just notify. JobProgressPanel would integrate here.
        const json = (await response.json()) as {
          data: { jobId: string };
        };
        notify.info(
          `Export sedang diproses (job: ${json.data.jobId}). Anda akan diberi tahu saat selesai.`,
        );
        return;
      }

      if (!response.ok) {
        throw new Error(`Export gagal: HTTP ${response.status}`);
      }

      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${filename}.${format}`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);

      const rows = response.headers.get("X-Total-Rows");
      notify.success(
        `Export ${format.toUpperCase()} berhasil.${rows ? ` ${rows} baris.` : ""}`,
      );
    } catch (err) {
      notify.error({
        code: "EXPORT_FAILED",
        message:
          err instanceof Error ? err.message : "Export gagal. Coba lagi.",
        traceId: "",
      });
    } finally {
      setExporting(false);
    }
  };

  if (formats.length === 1) {
    return (
      <Button
        variant="outline"
        size="sm"
        disabled={exporting}
        onClick={() => handleExport(formats[0])}
      >
        <Download className="mr-1.5 h-4 w-4" aria-hidden />
        {exporting ? "Mengekspor..." : `Export ${formats[0].toUpperCase()}`}
      </Button>
    );
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm" disabled={exporting}>
          <Download className="mr-1.5 h-4 w-4" aria-hidden />
          {exporting ? "Mengekspor..." : "Export"}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {formats.includes("csv") && (
          <DropdownMenuItem
            onClick={() => handleExport("csv")}
            disabled={exporting}
          >
            CSV
          </DropdownMenuItem>
        )}
        {formats.includes("xlsx") && (
          <DropdownMenuItem
            onClick={() => handleExport("xlsx")}
            disabled={exporting}
          >
            Excel (XLSX)
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
