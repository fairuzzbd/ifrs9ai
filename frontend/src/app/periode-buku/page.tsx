"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, Download, Calendar } from "lucide-react";
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
import {
  periodeStatusApi,
  type PeriodeBukuListParams,
} from "@/lib/api/periode-close.api";
import {
  STATUS_PERIODE_LABELS,
  TIPE_PERIODE_LABELS,
  BULAN_LABELS,
} from "@/lib/schemas/periode-close.schema";
import type { StatusPeriode, TipePeriode } from "@/lib/schemas/periode-close.schema";
import { PeriodeStatusBadge } from "@/components/blips/periode-close/PeriodeStatusBadge";
import { usePermissions } from "@/lib/stores/auth.store";

// ---------------------------------------------------------------------------
// Date helper
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
// Page (S5-AC1 — list + sort/filter/export per UX §1)
// ---------------------------------------------------------------------------

export default function PeriodeBukuListPage() {
  const router = useRouter();
  const { can } = usePermissions();

  const [search, setSearch] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState<string>("");
  const [tahunFilter, setTahunFilter] = React.useState<string>("");
  const [tipePeriodeFilter, setTipePeriodeFilter] = React.useState<string>("");
  const [sortField, setSortField] = React.useState<string>("tahun_buku:desc,bulan:desc");
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([]);

  const params: PeriodeBukuListParams = {
    cursor: cursor ?? undefined,
    limit: 50,
    sort: sortField,
    q: search || undefined,
    "filter[status_periode]": statusFilter || undefined,
    "filter[tahun_buku]": tahunFilter ? Number(tahunFilter) : undefined,
    "filter[tipe_periode]": tipePeriodeFilter || undefined,
  };

  const { data, isFetching, refetch } = useQuery({
    queryKey: ["periode-buku", "list", params],
    queryFn: () => periodeStatusApi.list(params),
    staleTime: 15_000,
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

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("");
    setTahunFilter("");
    setTipePeriodeFilter("");
    setCursor(null);
    setCursorHistory([]);
  };

  const hasActiveFilter = !!(search || statusFilter || tahunFilter || tipePeriodeFilter);

  const handleExport = () => {
    const exportParams = { ...params, export: "xlsx" as const };
    const qs = new URLSearchParams();
    if (exportParams.q) qs.set("q", exportParams.q);
    if (exportParams["filter[status_periode]"]) qs.set("filter[status_periode]", exportParams["filter[status_periode]"]!);
    if (exportParams["filter[tahun_buku]"]) qs.set("filter[tahun_buku]", String(exportParams["filter[tahun_buku]"]));
    if (exportParams.sort) qs.set("sort", exportParams.sort);
    qs.set("export", "xlsx");
    window.open(`/api/v1/periode-buku/export?${qs.toString()}`, "_blank");
  };

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-6 py-4">
        <div className="flex items-center gap-3">
          <Calendar className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
          <div>
            <h1 className="text-xl font-semibold">Periode Buku</h1>
            <p className="text-sm text-muted-foreground">
              Timeline pembukuan dan status close workflow
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleExport}
            aria-label="Export daftar periode buku ke Excel"
          >
            <Download className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Export XLSX
          </Button>
          <Button
            variant="outline"
            size="icon"
            onClick={() => refetch()}
            disabled={isFetching}
            aria-label="Refresh daftar periode buku"
          >
            <RefreshCw className={`h-4 w-4 ${isFetching ? "animate-spin" : ""}`} aria-hidden="true" />
          </Button>
        </div>
      </div>

      {/* Filter bar */}
      <div className="border-b px-6 py-3">
        <form onSubmit={handleSearch} className="flex flex-wrap gap-3">
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
            value={tipePeriodeFilter}
            onValueChange={(v) => {
              setTipePeriodeFilter(v === "_all" ? "" : v);
              setCursor(null);
              setCursorHistory([]);
            }}
          >
            <SelectTrigger className="w-36" aria-label="Filter tipe periode">
              <SelectValue placeholder="Semua tipe" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="_all">Semua tipe</SelectItem>
              {(Object.entries(TIPE_PERIODE_LABELS) as [TipePeriode, string][]).map(
                ([val, label]) => (
                  <SelectItem key={val} value={val}>
                    {label}
                  </SelectItem>
                ),
              )}
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
        </form>
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
                <TableHead>Tipe</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Tanggal Mulai</TableHead>
                <TableHead>Tanggal Akhir</TableHead>
                <TableHead>Soft-Close</TableHead>
                <TableHead>Hard-Close</TableHead>
                {can("periode.read") && <TableHead className="text-right">Aksi</TableHead>}
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
                  <TableCell>{TIPE_PERIODE_LABELS[row.tipePeriode]}</TableCell>
                  <TableCell>
                    <PeriodeStatusBadge
                      status={row.statusPeriode}
                      size="sm"
                      graceExpiresAt={row.hardCloseGraceExpiresAt ?? undefined}
                    />
                  </TableCell>
                  <TableCell>{fmtDate(row.tanggalMulai)}</TableCell>
                  <TableCell>{fmtDate(row.tanggalAkhir)}</TableCell>
                  <TableCell>
                    {row.tanggalSoftClose ? (
                      <span className="text-xs">{fmtDate(row.tanggalSoftClose)}</span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  <TableCell>
                    {row.tanggalHardClose ? (
                      <span className="text-xs">{fmtDate(row.tanggalHardClose)}</span>
                    ) : (
                      <span className="text-xs text-muted-foreground">—</span>
                    )}
                  </TableCell>
                  {can("periode.read") && (
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
                  )}
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
