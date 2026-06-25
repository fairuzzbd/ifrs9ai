/**
 * /admin/exports — User's export history + download.
 * Shows sys.export_log entries for the current user.
 * ROLE-AUDIT sees all users' exports.
 * Sort/filter/cursor-paging per UX §1.
 */

"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, FileDown } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ExportFormatBadge } from "@/components/blips/reporting/ExportFormatBadge";
import { ExportStatusBadge } from "@/components/blips/reporting/ExportStatusBadge";
import { ExportDownloadButton } from "@/components/blips/reporting/ExportDownloadButton";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { exportApi, reportingQueryKeys } from "@/lib/api/reporting.api";
import type { ExportFormat, ExportStatus } from "@/lib/schemas/reporting.schema";
import { REPORT_SLUG_LABELS } from "@/lib/schemas/reporting.schema";
import { notify } from "@/lib/notify";
import { v4 as uuidv4 } from "uuid";
import { formatInTimeZone } from "date-fns-tz";

function formatDate(iso: string | null): string {
  if (!iso) return "—";
  return formatInTimeZone(new Date(iso), "Asia/Jakarta", "dd MMM yyyy HH:mm");
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ExportsPage() {
  const [formatFilter, setFormatFilter] = React.useState<ExportFormat | "">("");
  const [statusFilter, setStatusFilter] = React.useState<ExportStatus | "">("");
  const [search, setSearch] = React.useState("");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [activeJobId, setActiveJobId] = React.useState<string | null>(null);

  const params = {
    limit: 50,
    sort: "requestedAt:desc",
    ...(search ? { q: search } : {}),
    ...(formatFilter ? { "filter[format]": formatFilter as ExportFormat } : {}),
    ...(statusFilter ? { "filter[status]": statusFilter } : {}),
    ...(cursor ? { cursor } : {}),
  };

  const { data, isLoading, refetch } = useQuery({
    queryKey: reportingQueryKeys.exportLog(params),
    queryFn: () => exportApi.listExportLog(params),
  });

  const items = data?.data ?? [];
  const pagination = data?.pagination;

  const handleExportNow = async (slug: string, format: ExportFormat) => {
    try {
      const result = await exportApi.requestExport(
        slug as Parameters<typeof exportApi.requestExport>[0],
        format,
        undefined,
        uuidv4(),
      );
      if (result.inline) {
        const a = document.createElement("a");
        a.href = result.blobUrl;
        a.download = result.filename;
        a.click();
        notify.success(`Export ${slug} (${format.toUpperCase()}) selesai.`);
        void refetch();
      } else {
        setActiveJobId(result.job.jobId);
      }
    } catch (err) {
      notify.error(err as Parameters<typeof notify.error>[0]);
    }
  };

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center gap-2">
        <FileDown className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
        <h1 className="text-xl font-semibold">Riwayat Export</h1>
      </div>

      <Separator />

      {/* Active async job panel */}
      {activeJobId && (
        <JobProgressPanel
          jobId={activeJobId}
          title="Export Laporan"
          onComplete={(result) => {
            const r = result as { signedUrl?: string; rowCount?: number };
            notify.success(
              `Export selesai. ${r.rowCount ? r.rowCount.toLocaleString("id-ID") : ""} baris siap diunduh (TTL 24 jam).`,
              r.signedUrl
                ? { action: { label: "Unduh sekarang", onClick: () => window.open(r.signedUrl, "_blank") } }
                : undefined,
            );
            setActiveJobId(null);
            void refetch();
          }}
          onFail={(err) => {
            notify.error({ code: "MV_REFRESH_FAILED", message: "Export gagal.", traceId: "" });
            setActiveJobId(null);
          }}
          showBackground
        />
      )}

      {/* Filter bar */}
      <div className="flex flex-wrap gap-3 items-center">
        <Input
          placeholder="Cari laporan..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-56"
          aria-label="Cari riwayat export"
        />

        <Select
          value={formatFilter || "all"}
          onValueChange={(v) => setFormatFilter(v === "all" ? "" : (v as ExportFormat))}
        >
          <SelectTrigger className="w-32" aria-label="Filter format">
            <SelectValue placeholder="Format" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua Format</SelectItem>
            <SelectItem value="csv">CSV</SelectItem>
            <SelectItem value="xlsx">XLSX</SelectItem>
            <SelectItem value="pdf">PDF</SelectItem>
          </SelectContent>
        </Select>

        <Select
          value={statusFilter || "all"}
          onValueChange={(v) => setStatusFilter(v === "all" ? "" : (v as ExportStatus))}
        >
          <SelectTrigger className="w-40" aria-label="Filter status">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Semua Status</SelectItem>
            <SelectItem value="REQUESTED">Diminta</SelectItem>
            <SelectItem value="QUEUED">Dalam Antrean</SelectItem>
            <SelectItem value="COMPUTING">Sedang Diproses</SelectItem>
            <SelectItem value="COMPLETED">Selesai</SelectItem>
            <SelectItem value="FAILED">Gagal</SelectItem>
          </SelectContent>
        </Select>

        {(formatFilter || statusFilter || search) && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setFormatFilter("");
              setStatusFilter("");
              setSearch("");
              setCursor(null);
            }}
          >
            Reset Filter
          </Button>
        )}
      </div>

      {/* Table */}
      {isLoading ? (
        <p className="text-sm text-muted-foreground p-4">Memuat riwayat export...</p>
      ) : items.length === 0 ? (
        <div className="rounded-lg border p-8 text-center">
          <Download className="mx-auto mb-2 h-8 w-8 text-muted-foreground/40" aria-hidden="true" />
          <p className="text-sm text-muted-foreground">Belum ada riwayat export.</p>
        </div>
      ) : (
        <div className="rounded-lg border overflow-x-auto">
          <Table aria-label="Riwayat export laporan">
            <TableHeader className="bg-muted/40">
              <TableRow>
                <TableHead className="w-48">Laporan</TableHead>
                <TableHead className="w-20">Format</TableHead>
                <TableHead className="w-32">Status</TableHead>
                <TableHead className="w-24 text-right">Baris</TableHead>
                <TableHead className="w-36">Diminta Pada</TableHead>
                <TableHead className="w-36">Selesai Pada</TableHead>
                <TableHead className="w-28 text-center">Aksi</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <TableRow key={item.id}>
                  <TableCell className="text-sm font-medium">
                    {REPORT_SLUG_LABELS[item.reportSlug as keyof typeof REPORT_SLUG_LABELS] ??
                      item.reportSlug}
                  </TableCell>
                  <TableCell>
                    <ExportFormatBadge format={item.format} />
                  </TableCell>
                  <TableCell>
                    <ExportStatusBadge status={item.status} />
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {item.rowCount != null ? item.rowCount.toLocaleString("id-ID") : "—"}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDate(item.requestedAt)}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {formatDate(item.completedAt)}
                  </TableCell>
                  <TableCell className="text-center">
                    <ExportDownloadButton exportItem={item} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Pagination */}
      {(pagination?.hasMore || cursor) && (
        <div className="flex items-center justify-between text-sm text-muted-foreground">
          <span>
            ~{pagination?.totalEstimate?.toLocaleString("id-ID") ?? 0} total entri
          </span>
          <div className="flex gap-2">
            {cursor && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setCursor(null)}
              >
                Kembali ke Awal
              </Button>
            )}
            {pagination?.hasMore && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setCursor(pagination.nextCursor ?? null)}
              >
                Selanjutnya
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
