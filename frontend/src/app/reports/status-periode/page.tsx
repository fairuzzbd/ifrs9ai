"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Download, BarChart3, CheckCircle2, XCircle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import { Badge } from "@/components/ui/badge";
import {
  periodeReportApi,
  type StatusPeriodeReportParams,
} from "@/lib/api/periode-close.api";
import {
  STATUS_PERIODE_LABELS,
  BULAN_LABELS,
  CHECKLIST_TRANSITION_LABELS,
} from "@/lib/schemas/periode-close.schema";
import type {
  StatusPeriode,
  StatusPeriodeListItem,
  ChecklistTransition,
} from "@/lib/schemas/periode-close.schema";
import { PeriodeStatusBadge } from "@/components/blips/periode-close/PeriodeStatusBadge";

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

function fmtDate(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleDateString("id-ID", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    timeZone: "Asia/Jakarta",
  });
}

// ---------------------------------------------------------------------------
// Page (S5-AC3 — status-periode report DataTable)
// ---------------------------------------------------------------------------

export default function StatusPeriodeReportPage() {
  const router = useRouter();

  const [search, setSearch] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState<string>("");
  const [tahunFilter, setTahunFilter] = React.useState<string>("");
  const [sortField, setSortField] = React.useState<string>("tahun_buku:desc,bulan:desc");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: StatusPeriodeReportParams = {
    cursor: cursor ?? undefined,
    limit: 50,
    sort: sortField,
    q: search || undefined,
    "filter[status_periode]": statusFilter || undefined,
    "filter[tahun_buku]": tahunFilter ? Number(tahunFilter) : undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["reports", "status-periode", params],
    queryFn: () => periodeReportApi.listStatus(params),
    staleTime: 30_000,
  });

  const rows: StatusPeriodeListItem[] = data?.data ?? [];
  const pagination = data?.pagination;

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

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("");
    setTahunFilter("");
    setCursor(null);
    setCursorHistory([]);
  };

  const hasActiveFilter = !!(search || statusFilter || tahunFilter);

  const handleExportCsv = () => {
    const qs = new URLSearchParams();
    if (params.q) qs.set("q", params.q);
    if (params["filter[status_periode]"]) qs.set("filter[status_periode]", params["filter[status_periode]"]!);
    if (params["filter[tahun_buku]"]) qs.set("filter[tahun_buku]", String(params["filter[tahun_buku]"]));
    if (params.sort) qs.set("sort", params.sort);
    qs.set("export", "csv");
    window.open(`/api/v1/reports/status-periode/export?${qs.toString()}`, "_blank");
  };

  const handleExportXlsx = () => {
    const qs = new URLSearchParams();
    if (params.q) qs.set("q", params.q);
    if (params["filter[status_periode]"]) qs.set("filter[status_periode]", params["filter[status_periode]"]!);
    if (params["filter[tahun_buku]"]) qs.set("filter[tahun_buku]", String(params["filter[tahun_buku]"]));
    if (params.sort) qs.set("sort", params.sort);
    qs.set("export", "xlsx");
    window.open(`/api/v1/reports/status-periode/export?${qs.toString()}`, "_blank");
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-6 py-4">
        <div className="flex items-center gap-3">
          <BarChart3 className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
          <div>
            <h1 className="text-xl font-semibold">Status Periode</h1>
            <p className="text-sm text-muted-foreground">
              Laporan ringkasan status close per periode buku
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex">
            <Button
              variant="outline"
              size="sm"
              className="rounded-r-none border-r-0"
              onClick={handleExportCsv}
              aria-label="Export CSV"
            >
              <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
              CSV
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="rounded-l-none"
              onClick={handleExportXlsx}
              aria-label="Export XLSX"
            >
              XLSX
            </Button>
          </div>
          <Button
            variant="outline"
            size="icon"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh laporan status periode"
          >
            <RefreshCw
              className={`h-4 w-4 ${isFetching ? "animate-spin" : ""}`}
              aria-hidden="true"
            />
          </Button>
        </div>
      </div>

      {/* Filter bar */}
      <div className="border-b px-6 py-3">
        <div className="flex flex-wrap gap-3">
          <Input
            placeholder="Cari kode periode..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setCursor(null);
              setCursorHistory([]);
            }}
            className="w-52"
            aria-label="Cari kode periode"
          />

          <Select
            value={statusFilter}
            onValueChange={(v) => {
              setStatusFilter(v === "_all" ? "" : v);
              setCursor(null);
              setCursorHistory([]);
            }}
          >
            <SelectTrigger className="w-44" aria-label="Filter status periode">
              <SelectValue placeholder="Semua status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">Semua status</SelectItem>
              {(Object.entries(STATUS_PERIODE_LABELS) as [StatusPeriode, string][]).map(
                ([val, label]) => (
                  <SelectItem key={val} value={val}>
                    {label}
                  </SelectItem>
                ),
              )}
            </SelectContent>
          </Select>

          <Select
            value={tahunFilter}
            onValueChange={(v) => {
              setTahunFilter(v === "_all" ? "" : v);
              setCursor(null);
              setCursorHistory([]);
            }}
          >
            <SelectTrigger className="w-32" aria-label="Filter tahun buku">
              <SelectValue placeholder="Semua tahun" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">Semua tahun</SelectItem>
              {[2024, 2025, 2026, 2027].map((y) => (
                <SelectItem key={y} value={String(y)}>
                  {y}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={sortField}
            onValueChange={(v) => {
              setSortField(v);
              setCursor(null);
              setCursorHistory([]);
            }}
          >
            <SelectTrigger className="w-44" aria-label="Urutan">
              <SelectValue placeholder="Urutkan" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="tahun_buku:desc,bulan:desc">Terbaru dulu</SelectItem>
              <SelectItem value="tahun_buku:asc,bulan:asc">Terlama dulu</SelectItem>
              <SelectItem value="status_periode:asc">Status (A-Z)</SelectItem>
            </SelectContent>
          </Select>

          {hasActiveFilter && (
            <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
              Hapus filter
            </Button>
          )}
        </div>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {isFetching && rows.length === 0 ? (
          <div className="p-8 text-center text-muted-foreground" aria-live="polite">
            <RefreshCw className="h-6 w-6 animate-spin mx-auto mb-2" aria-hidden="true" />
            Memuat data...
          </div>
        ) : rows.length === 0 ? (
          <div className="p-8 text-center text-muted-foreground" aria-live="polite">
            <p className="text-base font-medium">Tidak ada data yang cocok.</p>
            {hasActiveFilter && (
              <Button variant="link" className="mt-2" onClick={clearFilters}>
                Hapus filter
              </Button>
            )}
          </div>
        ) : (
          <Table>
            <TableHeader className="sticky top-0 bg-background z-10">
              <TableRow>
                <TableHead className="w-36">Kode Periode</TableHead>
                <TableHead>Tahun</TableHead>
                <TableHead>Bulan</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Soft-Close</TableHead>
                <TableHead>Oleh</TableHead>
                <TableHead>Hard-Close</TableHead>
                <TableHead>MV Refresh</TableHead>
                <TableHead>Checklist Terakhir</TableHead>
                <TableHead>Pernah Reopen</TableHead>
                <TableHead className="text-right">Aksi</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {rows.map((row) => (
                <TableRow
                  key={row.id}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => router.push(`/periode-buku/${row.id}`)}
                  tabIndex={0}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      router.push(`/periode-buku/${row.id}`);
                    }
                  }}
                  aria-label={`Lihat detail periode ${row.periodeKode}`}
                >
                  <TableCell className="font-mono text-sm font-semibold">
                    {row.periodeKode}
                  </TableCell>
                  <TableCell>{row.tahunBuku}</TableCell>
                  <TableCell>{BULAN_LABELS[row.bulan] ?? row.bulan}</TableCell>
                  <TableCell>
                    <PeriodeStatusBadge
                      status={row.statusPeriode}
                      size="sm"
                      graceExpiresAt={row.hardCloseGraceExpiresAt ?? undefined}
                    />
                  </TableCell>
                  <TableCell>
                    {row.tanggalSoftClose ? (
                      <span className="text-xs">{fmtDate(row.tanggalSoftClose)}</span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <span className="text-xs text-muted-foreground font-mono">
                      {row.softCloseBy ?? "—"}
                    </span>
                  </TableCell>
                  <TableCell>
                    {row.tanggalHardClose ? (
                      <span className="text-xs">{fmtDate(row.tanggalHardClose)}</span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    {row.mvRefreshStatus ? (
                      <Badge
                        variant="outline"
                        className={
                          row.mvRefreshStatus === "completed"
                            ? "text-green-700 border-green-300"
                            : row.mvRefreshStatus === "failed"
                              ? "text-red-700 border-red-300"
                              : row.mvRefreshStatus === "running"
                                ? "text-blue-700 border-blue-300"
                                : "text-muted-foreground"
                        }
                      >
                        {row.mvRefreshStatus}
                      </Badge>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    {row.checklistLastSnapshot ? (
                      <span className="flex items-center gap-1.5 text-xs">
                        {row.checklistLastSnapshot.allPassed ? (
                          <CheckCircle2
                            className="h-3.5 w-3.5 text-green-600 shrink-0"
                            aria-hidden="true"
                          />
                        ) : (
                          <XCircle
                            className="h-3.5 w-3.5 text-red-500 shrink-0"
                            aria-hidden="true"
                          />
                        )}
                        {CHECKLIST_TRANSITION_LABELS[row.checklistLastSnapshot.transition as ChecklistTransition]}
                      </span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    {row.reopenedFlag ? (
                      <Badge variant="outline" className="text-orange-700 border-orange-300 text-xs">
                        Ya
                      </Badge>
                    ) : (
                      <span className="text-xs text-muted-foreground">Tidak</span>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        router.push(`/periode-buku/${row.id}`);
                      }}
                      aria-label={`Buka detail ${row.periodeKode}`}
                    >
                      Detail
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </div>

      {/* Pagination */}
      <div className="border-t px-6 py-3 flex items-center justify-between">
        <span className="text-sm text-muted-foreground">
          {rows.length > 0
            ? `Menampilkan ${rows.length} data${
                pagination?.totalEstimate
                  ? ` dari ~${pagination.totalEstimate.toLocaleString("id-ID")}`
                  : ""
              }`
            : "Tidak ada data"}
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handlePrev}
            disabled={cursorHistory.length === 0}
            aria-label="Halaman sebelumnya"
          >
            Sebelumnya
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={handleNext}
            disabled={!pagination?.hasMore}
            aria-label="Halaman berikutnya"
          >
            Berikutnya
          </Button>
        </div>
      </div>
    </div>
  );
}
