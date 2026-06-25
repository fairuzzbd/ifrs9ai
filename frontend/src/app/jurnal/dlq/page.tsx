"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, AlertOctagon } from "lucide-react";
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
import { dlqApi, type DlqListParams } from "@/lib/api/jurnal.api";
import type { DlqStatus } from "@/lib/schemas/jurnal.schema";
import { useJurnalStore } from "@/lib/stores/jurnal.store";
import { cn } from "@/lib/utils";

const STATUS_LABELS: Record<DlqStatus, string> = {
  FAILED: "Gagal",
  REPLAYING: "Sedang Diulang",
  REPLAYED_OK: "Berhasil Diulang",
  ABANDONED: "Diabaikan",
};

const STATUS_VARIANT: Record<DlqStatus, string> = {
  FAILED: "destructive",
  REPLAYING: "secondary",
  REPLAYED_OK: "default",
  ABANDONED: "outline",
};

export default function DLQConsolePage() {
  const router = useRouter();
  const { dlqFilters, setDlqFilters } = useJurnalStore();
  const [search, setSearch] = React.useState(dlqFilters.q ?? "");
  const [statusFilter, setStatusFilter] = React.useState(dlqFilters.status ?? "FAILED");
  const [errorCodeFilter, setErrorCodeFilter] = React.useState(dlqFilters.errorCode ?? "");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: DlqListParams = {
    cursor,
    limit: 50,
    sort: "last_attempt_at:desc",
    q: search || undefined,
    "filter[status]": statusFilter || undefined,
    "filter[error_code]": errorCodeFilter || undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["dlq-list", params],
    queryFn: () => dlqApi.list(params),
  });

  const rows = data?.data ?? [];
  const pagination = data?.pagination;
  const failedCount = rows.filter((r) => r.status === "FAILED").length;

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setDlqFilters({ q: search, status: statusFilter, errorCode: errorCodeFilter });
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

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-6 py-4">
        <div className="flex items-center gap-3">
          <div>
            <h1 className="text-xl font-semibold">DLQ — Dead Letter Queue</h1>
            <p className="text-sm text-muted-foreground">
              Entri jurnal yang gagal diproses dan perlu ditangani
            </p>
          </div>
          {failedCount > 0 && (
            <Badge variant="destructive" className="flex items-center gap-1">
              <AlertOctagon className="h-3 w-3" aria-hidden="true" />
              {failedCount} gagal
            </Badge>
          )}
        </div>
      </div>

      {/* Filter bar */}
      <form
        onSubmit={handleSearch}
        className="flex flex-wrap gap-3 px-6 py-3 border-b bg-muted/30"
      >
        <Input
          placeholder="Cari sumber event atau kode error..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 w-64"
          aria-label="Cari entri DLQ"
        />
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="h-8 w-44">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">Semua Status</SelectItem>
            {(Object.keys(STATUS_LABELS) as DlqStatus[]).map((s) => (
              <SelectItem key={s} value={s}>{STATUS_LABELS[s]}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Input
          placeholder="Filter kode error..."
          value={errorCodeFilter}
          onChange={(e) => setErrorCodeFilter(e.target.value)}
          className="h-8 w-48"
          aria-label="Filter kode error"
        />
        <Button type="submit" size="sm" variant="outline" className="h-8">
          Cari
        </Button>
        <div className="ml-auto">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-8"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh DLQ"
          >
            <RefreshCw
              className={cn("h-4 w-4", isFetching && "animate-spin")}
              aria-hidden="true"
            />
          </Button>
        </div>
      </form>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader className="sticky top-0 bg-background z-10">
            <TableRow>
              <TableHead className="w-36">Source Event Type</TableHead>
              <TableHead className="w-32">Kode Event</TableHead>
              <TableHead>Kode Error</TableHead>
              <TableHead className="w-48">Pesan Error</TableHead>
              <TableHead className="w-16 text-right">Attempt</TableHead>
              <TableHead className="w-32">Terakhir Dicoba</TableHead>
              <TableHead className="w-28">Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isFetching && rows.length === 0 ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 7 }).map((_, j) => (
                    <TableCell key={j}>
                      <div className="h-4 w-full animate-pulse rounded bg-muted" />
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={7}
                  className="py-12 text-center text-sm text-muted-foreground"
                >
                  Tidak ada entri DLQ yang cocok
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.id}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => router.push(`/jurnal/dlq/${row.id}`)}
                  role="link"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      router.push(`/jurnal/dlq/${row.id}`);
                    }
                  }}
                >
                  <TableCell className="font-mono text-xs">{row.sourceEventType}</TableCell>
                  <TableCell className="font-mono text-xs">{row.eventCode}</TableCell>
                  <TableCell className="font-mono text-xs text-red-700">{row.errorCode}</TableCell>
                  <TableCell className="text-xs text-muted-foreground truncate max-w-[12rem]">
                    {row.errorMessage}
                  </TableCell>
                  <TableCell className="text-right text-xs font-mono">{row.attemptCount}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {new Date(row.lastAttemptAt).toLocaleDateString("id-ID")}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        (STATUS_VARIANT[row.status] as "default" | "secondary" | "destructive" | "outline") ?? "outline"
                      }
                      className="text-xs"
                    >
                      {STATUS_LABELS[row.status]}
                    </Badge>
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
