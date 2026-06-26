"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Download, ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { glReconciliationApi, type ReconciliationHistoryParams as ReconHistoryParams } from "@/lib/api/gl-delivery.api";
import {
  RECON_STATUS_LABELS,
  type ReconReportStatus,
} from "@/lib/schemas/gl-delivery.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const STATUS_VARIANT: Record<ReconReportStatus, "default" | "secondary" | "destructive" | "outline"> = {
  COMPLETED: "default",
  COMPLETED_WITH_MISMATCH: "destructive",
  RUNNING: "secondary",
  PENDING: "outline",
  FAILED: "destructive",
};

// ---------------------------------------------------------------------------
// Page (S4-AC4 — history list with sort/filter/export)
// ---------------------------------------------------------------------------

export default function RekonsiliasiRiwayatPage() {
  const router = useRouter();
  const [search, setSearch] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState<string>("");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: ReconHistoryParams = {
    cursor: cursor ?? undefined,
    limit: 50,
    sort: "recon_date:desc",
    q: search || undefined,
    "filter[status]": statusFilter || undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["gl-recon", "history", params],
    queryFn: () => glReconciliationApi.listHistory(params),
    staleTime: 30_000,
  });

  const rows = data?.data ?? [];
  const pagination = data?.pagination;

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setCursor(null);
    setCursorHistory([]);
  };

  const handleNext = () => {
    if (!pagination?.nextCursor) return;
    if (cursor) setCursorHistory((h) => [...h, cursor]);
    setCursor(pagination.nextCursor ?? null);
  };

  const handlePrev = () => {
    const history = [...cursorHistory];
    const prev = history.pop() ?? null;
    setCursorHistory(history);
    setCursor(prev);
  };

  const handleExport = () => {
    window.open(
      glReconciliationApi.exportUrl({ ...params, format: "xlsx" }),
      "_blank",
    );
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-6 py-4">
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => router.push("/jrnl/rekonsiliasi")}
            aria-label="Kembali ke rekonsiliasi hari ini"
          >
            <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
          </Button>
          <div>
            <h1 className="text-xl font-semibold">Riwayat Rekonsiliasi GL</h1>
            <p className="text-sm text-muted-foreground">
              Semua laporan rekonsiliasi BLIPS ↔ GL Host
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleExport}
            aria-label="Export riwayat rekonsiliasi ke Excel"
          >
            <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Export XLSX
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh riwayat rekonsiliasi"
          >
            <RefreshCw
              className={cn("h-4 w-4", isFetching && "animate-spin")}
              aria-hidden="true"
            />
          </Button>
        </div>
      </div>

      {/* Filter bar */}
      <form
        onSubmit={handleSearch}
        className="flex flex-wrap gap-3 px-6 py-3 border-b bg-muted/30"
        role="search"
        aria-label="Filter riwayat rekonsiliasi"
      >
        <Input
          placeholder="Cari tanggal atau periode..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 w-64"
          aria-label="Cari rekonsiliasi"
        />
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="h-8 w-44" aria-label="Filter status rekonsiliasi">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">Semua Status</SelectItem>
            {(Object.keys(RECON_STATUS_LABELS) as ReconReportStatus[]).map((s) => (
              <SelectItem key={s} value={s}>
                {RECON_STATUS_LABELS[s]}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button type="submit" size="sm" variant="outline" className="h-8">
          Cari
        </Button>
      </form>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader className="sticky top-0 bg-background z-10">
            <TableRow>
              <TableHead className="w-32">Tanggal Rekon</TableHead>
              <TableHead className="w-28">Status</TableHead>
              <TableHead className="w-24 text-right">Akun Dicek</TableHead>
              <TableHead className="w-20 text-right">Mismatch</TableHead>
              <TableHead className="w-40 text-right">Selisih (IDR)</TableHead>
              <TableHead className="w-36">Dijalankan</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isFetching && rows.length === 0 ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 6 }).map((_, j) => (
                    <TableCell key={j}>
                      <div className="h-4 w-full animate-pulse rounded bg-muted" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className="py-12 text-center text-sm text-muted-foreground"
                >
                  Tidak ada laporan rekonsiliasi yang cocok.
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.reportId}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() =>
                    router.push(
                      `/jrnl/rekonsiliasi?date=${row.tanggalRekonsiliasi}`,
                    )
                  }
                  role="link"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      router.push(`/jrnl/rekonsiliasi?date=${row.tanggalRekonsiliasi}`);
                    }
                  }}
                  aria-label={`Laporan rekonsiliasi ${row.tanggalRekonsiliasi}`}
                >
                  <TableCell className="font-mono text-sm">{row.tanggalRekonsiliasi}</TableCell>
                  <TableCell>
                    <Badge variant={STATUS_VARIANT[row.status]} className="text-xs">
                      {RECON_STATUS_LABELS[row.status]}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right font-mono text-sm">
                    {row.totalAkunChecked ?? "—"}
                  </TableCell>
                  <TableCell
                    className={cn(
                      "text-right font-mono text-sm",
                      row.totalMismatchCount > 0 && "text-amber-700 font-medium",
                    )}
                  >
                    {row.totalMismatchCount}
                  </TableCell>
                  <TableCell
                    className={cn(
                      "text-right font-mono text-xs",
                      (row.deltaIdr ?? 0) !== 0 && "text-red-600 font-medium",
                    )}
                  >
                    {row.deltaIdr != null
                      ? new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 4 }).format(row.deltaIdr)
                      : "—"}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {row.generatedAt
                      ? new Date(row.generatedAt).toLocaleString("id-ID", {
                          day: "2-digit",
                          month: "short",
                          hour: "2-digit",
                          minute: "2-digit",
                          timeZone: "Asia/Jakarta",
                        })
                      : "—"}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between border-t px-6 py-3">
        <p className="text-xs text-muted-foreground">
          {pagination?.totalEstimate != null
            ? `Estimasi total: ${pagination.totalEstimate.toLocaleString("id-ID")} laporan`
            : ""}
        </p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={cursorHistory.length === 0}
            onClick={handlePrev}
          >
            Sebelumnya
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!pagination?.hasMore}
            onClick={handleNext}
          >
            Berikutnya
          </Button>
        </div>
      </div>
    </div>
  );
}
