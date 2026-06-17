"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, AlertOctagon, Download } from "lucide-react";
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
import { glDeliveryDlqApi, type DlqListParams } from "@/lib/api/gl-delivery.api";
import { GL_HOST_STATUS_LABELS } from "@/lib/schemas/gl-delivery.schema";
import type { GlHostStatus } from "@/lib/schemas/gl-delivery.schema";
import { GlStatusBadge } from "@/components/blips/gl-delivery/GlStatusBadge";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// Page (S1-AC1 — DLQ list with sort/filter/export, S1-AC2 — status badge)
// ---------------------------------------------------------------------------

export default function GlDeliveryDlqListPage() {
  const router = useRouter();
  const [search, setSearch] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState<string>("FAILED");
  const [failureCatFilter, setFailureCatFilter] = React.useState<string>("");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: DlqListParams = {
    cursor: cursor ?? undefined,
    limit: 50,
    sort: "created_at:desc",
    q: search || undefined,
    "filter[gl_host_status]": statusFilter || undefined,
    "filter[failure_category]": failureCatFilter || undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["gl-dlq-list", params],
    queryFn: () => glDeliveryDlqApi.list(params),
    staleTime: 15_000,
  });

  const rows = data?.data ?? [];
  const pagination = data?.pagination;
  const deadLetterCount = rows.filter((r) => r.glHostStatus === "DEAD_LETTER").length;

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
    window.open(glDeliveryDlqApi.exportUrl({ ...params, format: "xlsx" }), "_blank");
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-6 py-4">
        <div className="flex items-center gap-3">
          <div>
            <h1 className="text-xl font-semibold">GL Delivery — Dead Letter Queue</h1>
            <p className="text-sm text-muted-foreground">
              Jurnal yang gagal dikirim ke GL Host dan masuk DLQ
            </p>
          </div>
          {deadLetterCount > 0 && (
            <Badge
              variant="destructive"
              className="flex items-center gap-1"
              aria-label={`${deadLetterCount} entry DEAD_LETTER`}
            >
              <AlertOctagon className="h-3 w-3" aria-hidden="true" />
              {deadLetterCount} DEAD_LETTER
            </Badge>
          )}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleExport}
            aria-label="Export DLQ list ke Excel"
          >
            <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Export XLSX
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh daftar DLQ"
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
        aria-label="Filter DLQ GL"
      >
        <Input
          placeholder="Cari no. jurnal atau error code..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 w-64"
          aria-label="Cari DLQ GL"
        />
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="h-8 w-44" aria-label="Filter status DLQ">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">Semua Status</SelectItem>
            <SelectItem value="FAILED">{GL_HOST_STATUS_LABELS.FAILED}</SelectItem>
            <SelectItem value="DEAD_LETTER">{GL_HOST_STATUS_LABELS.DEAD_LETTER}</SelectItem>
            <SelectItem value="ALL">Semua (FAILED + DEAD_LETTER)</SelectItem>
          </SelectContent>
        </Select>
        <Select value={failureCatFilter} onValueChange={setFailureCatFilter}>
          <SelectTrigger className="h-8 w-44" aria-label="Filter kategori kegagalan">
            <SelectValue placeholder="Semua Kategori" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">Semua Kategori</SelectItem>
            <SelectItem value="DOMAIN">DOMAIN (4xx)</SelectItem>
            <SelectItem value="INFRA">INFRA (5xx/timeout)</SelectItem>
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
              <TableHead className="w-40">No. Jurnal</TableHead>
              <TableHead className="w-44">Status GL</TableHead>
              <TableHead>Error Code</TableHead>
              <TableHead className="w-48">Pesan Error</TableHead>
              <TableHead className="w-16 text-right">Retry</TableHead>
              <TableHead className="w-32">Masuk DLQ</TableHead>
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
                  {statusFilter || search
                    ? "Tidak ada entri DLQ yang cocok dengan filter."
                    : "Tidak ada entri DLQ. Semua pengiriman GL berjalan lancar."}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.dlqEntryId}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => router.push(`/jrnl/gl-delivery-dlq/${row.dlqEntryId}`)}
                  role="link"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      router.push(`/jrnl/gl-delivery-dlq/${row.dlqEntryId}`);
                    }
                  }}
                  aria-label={`DLQ entry untuk jurnal ${row.noJurnal}`}
                >
                  <TableCell className="font-mono text-xs font-medium">
                    {row.noJurnal}
                  </TableCell>
                  <TableCell>
                    <GlStatusBadge status={row.glHostStatus} size="sm" />
                  </TableCell>
                  <TableCell className="font-mono text-xs text-red-700">
                    {row.errorCode}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground truncate max-w-[12rem]">
                    {row.errorMessage}
                  </TableCell>
                  <TableCell className="text-right text-xs font-mono">
                    {row.retryCount}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {new Date(row.createdAt).toLocaleDateString("id-ID")}
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
            ? `Estimasi total: ${pagination.totalEstimate.toLocaleString("id-ID")} entri`
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
