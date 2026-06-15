"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Download } from "lucide-react";
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
import { jurnalQueryApi, type JurnalListParams } from "@/lib/api/jurnal.api";
import { useJurnalStore } from "@/lib/stores/jurnal.store";
import { cn } from "@/lib/utils";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

const STATUS_VARIANT: Record<string, string> = {
  POSTED: "default",
  REVERSED: "destructive",
  PENDING_APPROVAL: "secondary",
};

const STATUS_LABELS: Record<string, string> = {
  POSTED: "Terposting",
  REVERSED: "Direverse",
  PENDING_APPROVAL: "Menunggu Persetujuan",
};

export default function JurnalEntriesPage() {
  const router = useRouter();
  const { jurnalFilters, setJurnalFilters } = useJurnalStore();
  const [search, setSearch] = React.useState(jurnalFilters.q ?? "");
  const [statusFilter, setStatusFilter] = React.useState(jurnalFilters.statusInternal ?? "");
  const [eventFilter, setEventFilter] = React.useState(jurnalFilters.eventCode ?? "");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: JurnalListParams = {
    cursor,
    limit: 50,
    sort: "tanggal_posting:desc",
    q: search || undefined,
    "filter[status_internal]": statusFilter || undefined,
    "filter[event_code]": eventFilter || undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["jurnal-entries-list", params],
    queryFn: () => jurnalQueryApi.list(params),
  });

  const rows = data?.data ?? [];
  const pagination = data?.pagination;

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault();
    setJurnalFilters({ q: search, statusInternal: statusFilter, eventCode: eventFilter });
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
        <div>
          <h1 className="text-xl font-semibold">Entri Jurnal</h1>
          <p className="text-sm text-muted-foreground">
            Riwayat semua posting jurnal (append-only)
          </p>
        </div>
        <Button onClick={() => router.push("/jrnl/post")} variant="outline">
          Posting Manual
        </Button>
      </div>

      {/* Filter bar */}
      <form
        onSubmit={handleSearch}
        className="flex flex-wrap gap-3 px-6 py-3 border-b bg-muted/30"
      >
        <Input
          placeholder="Cari no. jurnal atau event..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 w-64"
          aria-label="Cari entri jurnal"
        />
        <Select value={statusFilter} onValueChange={setStatusFilter}>
          <SelectTrigger className="h-8 w-44">
            <SelectValue placeholder="Semua Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">Semua Status</SelectItem>
            <SelectItem value="POSTED">Terposting</SelectItem>
            <SelectItem value="REVERSED">Direverse</SelectItem>
            <SelectItem value="PENDING_APPROVAL">Menunggu Persetujuan</SelectItem>
          </SelectContent>
        </Select>
        <Input
          placeholder="Filter event code..."
          value={eventFilter}
          onChange={(e) => setEventFilter(e.target.value)}
          className="h-8 w-44"
          aria-label="Filter event code"
        />
        <Button type="submit" size="sm" variant="outline" className="h-8">
          Cari
        </Button>
        <div className="ml-auto flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-8"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh data"
          >
            <RefreshCw
              className={cn("h-4 w-4", isFetching && "animate-spin")}
              aria-hidden="true"
            />
          </Button>
          <Button type="button" size="sm" variant="ghost" className="h-8" asChild>
            <a
              href={jurnalQueryApi.exportUrl({ ...params, format: "xlsx" })}
              download
              aria-label="Export ke Excel"
            >
              <Download className="h-4 w-4 mr-1.5" aria-hidden="true" />
              Export
            </a>
          </Button>
        </div>
      </form>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table>
          <TableHeader className="sticky top-0 bg-background z-10">
            <TableRow>
              <TableHead className="w-40">No. Jurnal</TableHead>
              <TableHead className="w-28">Tgl Posting</TableHead>
              <TableHead className="w-36">Kode Event</TableHead>
              <TableHead>Instrumen</TableHead>
              <TableHead className="w-32 text-right">Total Debit</TableHead>
              <TableHead className="w-32 text-right">Total Kredit</TableHead>
              <TableHead className="w-32">Status</TableHead>
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
                  Tidak ada entri jurnal yang cocok
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.id}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => router.push(`/jrnl/journal-entries/${row.id}`)}
                  role="link"
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      router.push(`/jrnl/journal-entries/${row.id}`);
                    }
                  }}
                >
                  <TableCell className="font-mono text-xs font-medium">{row.noJurnal}</TableCell>
                  <TableCell className="text-xs">
                    {new Date(row.tanggalPosting).toLocaleDateString("id-ID")}
                  </TableCell>
                  <TableCell className="font-mono text-xs">{row.eventCode}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {row.instrumenNama ?? "-"}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {IDR.format(parseFloat(row.totalDebit || "0"))}
                  </TableCell>
                  <TableCell className="text-right font-mono text-xs">
                    {IDR.format(parseFloat(row.totalKredit || "0"))}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        (STATUS_VARIANT[row.statusInternal] as "default" | "secondary" | "destructive" | "outline") ?? "outline"
                      }
                      className="text-xs"
                    >
                      {STATUS_LABELS[row.statusInternal] ?? row.statusInternal}
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
