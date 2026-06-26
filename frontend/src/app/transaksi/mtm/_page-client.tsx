"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { Plus, Link2 } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { MtmStatusBadge } from "@/components/blips/mtm/MtmStatusBadge";
import { MtmDeviationBadge } from "@/components/blips/mtm/MtmDeviationBadge";
import { MtmStaleBadge } from "@/components/blips/mtm/MtmStaleBadge";
import { MtmSourceBadge } from "@/components/blips/mtm/MtmSourceBadge";
import { MtmRoutingBadge } from "@/components/blips/mtm/MtmRoutingBadge";
import { MtmOverrideApproveDialog } from "@/components/blips/mtm/MtmOverrideApproveDialog";
import { MtmOverrideRejectDialog } from "@/components/blips/mtm/MtmOverrideRejectDialog";

import { mtmListApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  MTM_STATUS_LABELS,
  HARGA_SUMBER_LABELS,
  MTM_KLASIFIKASI_LABELS,
  type MtmListItem,
  type HargaSumber,
  type MtmKlasifikasi,
  type StalePriceReason,
} from "@/lib/schemas/mtm.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

// ---------------------------------------------------------------------------
// URL state hook
// ---------------------------------------------------------------------------

function useMtmFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("tanggal_mtm:desc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status]", parseAsString.withDefault(""));
  const [filterKlasifikasi, setFilterKlasifikasi] = useQueryState("filter[klasifikasi_psak71]", parseAsString.withDefault(""));
  const [filterSumber, setFilterSumber] = useQueryState("filter[harga_sumber]", parseAsString.withDefault(""));
  const [filterDeviation, setFilterDeviation] = useQueryState("filter[deviation_flag]", parseAsString.withDefault(""));
  const [filterStale, setFilterStale] = useQueryState("filter[stale_price_flag]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ,
    sort, setSort,
    filterStatus, setFilterStatus,
    filterKlasifikasi, setFilterKlasifikasi,
    filterSumber, setFilterSumber,
    filterDeviation, setFilterDeviation,
    filterStale, setFilterStale,
    cursor, setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// IDR formatter
// ---------------------------------------------------------------------------

const IDR = new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 0, maximumFractionDigits: 0 });

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function MtmListContent() {
  const perms = usePermissions();
  const queryClient = useQueryClient();
  const filters = useMtmFilters();

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  // Override dialog state
  const [approveTarget, setApproveTarget] = React.useState<MtmListItem | null>(null);
  const [rejectTarget, setRejectTarget] = React.useState<MtmListItem | null>(null);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: mtmQueryKeys.list({
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
      "filter[status]": filters.filterStatus || undefined,
      "filter[klasifikasi_psak71]": filters.filterKlasifikasi || undefined,
      "filter[harga_sumber]": filters.filterSumber || undefined,
      "filter[deviation_flag]": filters.filterDeviation === "true" ? true : undefined,
      "filter[stale_price_flag]": filters.filterStale === "true" ? true : undefined,
      cursor: filters.cursor || undefined,
    }),
    queryFn: () =>
      mtmListApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[status]": filters.filterStatus || undefined,
        "filter[klasifikasi_psak71]": filters.filterKlasifikasi as MtmKlasifikasi | undefined,
        "filter[harga_sumber]": filters.filterSumber as HargaSumber | undefined,
        "filter[deviation_flag]": filters.filterDeviation === "true" ? true : undefined,
        "filter[stale_price_flag]": filters.filterStale === "true" ? true : undefined,
        cursor: filters.cursor || undefined,
      }),
    staleTime: 30_000,
  });

  const handleRefresh = () => {
    void refetch();
    setLastUpdated(new Date());
  };

  // Sort
  const sortingState: SortingState = React.useMemo(() => {
    if (!filters.sort) return [];
    return filters.sort.split(",").map((part) => {
      const [id, dir] = part.split(":");
      return { id: id ?? "", desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (newSorting: SortingState) => {
    void filters.setSort(
      newSorting.length === 0
        ? "tanggal_mtm:desc"
        : newSorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","),
    );
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const toggleSort = (colId: string) => {
    const current = sortingState.find((s) => s.id === colId);
    if (!current) handleSortingChange([{ id: colId, desc: false }]);
    else if (!current.desc) handleSortingChange([{ id: colId, desc: true }]);
    else handleSortingChange([]);
  };

  // Pagination
  const handleNextPage = () => {
    const nextCursor = data?.pagination.nextCursor;
    if (nextCursor) {
      const newHistory = [...cursorHistory, nextCursor];
      setCursorHistory(newHistory);
      setPageIndex(newHistory.length - 1);
      void filters.setCursor(nextCursor);
    }
  };

  const handlePrevPage = () => {
    if (pageIndex > 0) {
      const newIndex = pageIndex - 1;
      setPageIndex(newIndex);
      void filters.setCursor(cursorHistory[newIndex] ?? "");
    }
  };

  // Active filter chips
  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterStatus) f.push({ key: "filter[status]", label: "Status", value: filters.filterStatus, displayValue: MTM_STATUS_LABELS[filters.filterStatus as keyof typeof MTM_STATUS_LABELS] ?? filters.filterStatus });
    if (filters.filterKlasifikasi) f.push({ key: "filter[klasifikasi_psak71]", label: "Klasifikasi", value: filters.filterKlasifikasi, displayValue: MTM_KLASIFIKASI_LABELS[filters.filterKlasifikasi as MtmKlasifikasi] ?? filters.filterKlasifikasi });
    if (filters.filterSumber) f.push({ key: "filter[harga_sumber]", label: "Sumber", value: filters.filterSumber, displayValue: HARGA_SUMBER_LABELS[filters.filterSumber as HargaSumber] ?? filters.filterSumber });
    if (filters.filterDeviation === "true") f.push({ key: "filter[deviation_flag]", label: "Deviasi", value: "true", displayValue: "Ada Deviasi" });
    if (filters.filterStale === "true") f.push({ key: "filter[stale_price_flag]", label: "Stale", value: "true", displayValue: "Harga Kedaluwarsa" });
    return f;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.filterStatus, filters.filterKlasifikasi, filters.filterSumber, filters.filterDeviation, filters.filterStale]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[status]") void filters.setFilterStatus("");
    if (key === "filter[klasifikasi_psak71]") void filters.setFilterKlasifikasi("");
    if (key === "filter[harga_sumber]") void filters.setFilterSumber("");
    if (key === "filter[deviation_flag]") void filters.setFilterDeviation("");
    if (key === "filter[stale_price_flag]") void filters.setFilterStale("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterKlasifikasi("");
    void filters.setFilterSumber("");
    void filters.setFilterDeviation("");
    void filters.setFilterStale("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const canOverride = perms.can("mtm.override");

  // Columns
  const columns: ColumnDef<MtmListItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "tanggalMtm",
        header: () => (
          <SortHeader label="Tanggal MTM" sortKey="tanggal_mtm" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="text-sm whitespace-nowrap">{row.original.tanggalMtm}</span>
        ),
      },
      {
        accessorKey: "instrumenKode",
        header: () => (
          <SortHeader label="Instrumen" sortKey="instrumen_kode" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <div>
            <p className="text-sm font-medium font-mono">{row.original.instrumenKode}</p>
            <p className="text-xs text-muted-foreground truncate max-w-[180px]">{row.original.instrumenNama}</p>
          </div>
        ),
      },
      {
        accessorKey: "klasifikasiSnapshot",
        header: "Klasifikasi",
        cell: ({ row }) => (
          <MtmRoutingBadge
            eventCodes={row.original.jurnalEventCode ? [row.original.jurnalEventCode] : []}
            klasifikasi={row.original.klasifikasiSnapshot}
          />
        ),
      },
      {
        accessorKey: "hargaSumber",
        header: "Sumber",
        cell: ({ row }) => (
          <MtmSourceBadge source={row.original.hargaSumber} />
        ),
      },
      {
        accessorKey: "hargaPasarIdr",
        header: () => (
          <SortHeader label="Harga Pasar (IDR)" sortKey="harga_pasar_idr" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="font-mono text-right block text-sm">
            {IDR.format(row.original.hargaPasarIdr)}
          </span>
        ),
      },
      {
        accessorKey: "deltaPct",
        header: () => (
          <SortHeader label="Delta" sortKey="delta_pct" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => {
          const { deltaPct, deviationFlag } = row.original;
          const formatted = `${deltaPct >= 0 ? "+" : ""}${deltaPct.toFixed(2)}%`;
          return (
            <div className="space-y-0.5">
              <span className={`text-sm font-mono ${deltaPct >= 0 ? "text-green-700" : "text-red-700"}`}>
                {formatted}
              </span>
              {deviationFlag && (
                <MtmDeviationBadge deltaPct={deltaPct} thresholdPct={5} />
              )}
            </div>
          );
        },
      },
      {
        accessorKey: "hargaAgeDays",
        header: () => (
          <SortHeader label="Umur Harga" sortKey="harga_age_days" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => {
          const { hargaAgeDays, stalePriceFlag } = row.original;
          if (!stalePriceFlag) {
            return <span className="text-sm text-muted-foreground">{hargaAgeDays} hari</span>;
          }
          return (
            <MtmStaleBadge
              hargaAgeDays={hargaAgeDays}
              stalePriceReason={"HARGA_TIDAK_TERSEDIA" as StalePriceReason}
              escalated={hargaAgeDays > 7}
            />
          );
        },
      },
      {
        accessorKey: "status",
        header: () => (
          <SortHeader label="Status" sortKey="status" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <MtmStatusBadge status={row.original.status} size="sm" />
        ),
      },
      {
        id: "jurnal",
        header: "Jurnal",
        cell: ({ row }) =>
          row.original.jurnalEntryId ? (
            <Link
              href={`/jurnal/${row.original.jurnalEntryId}`}
              className="inline-flex items-center text-xs text-primary hover:underline"
              aria-label={`Lihat jurnal untuk MTM ${row.original.instrumenKode}`}
            >
              <Link2 className="h-3.5 w-3.5" aria-hidden="true" />
            </Link>
          ) : (
            <span className="text-muted-foreground text-xs">—</span>
          ),
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          const allowsOverride = ["PENDING_REVIEW", "STALE_PRICE"].includes(item.status);

          if (!canOverride || !allowsOverride || item.lockedFlag) {
            return null;
          }

          const isStaleWithoutReupload = item.status === "STALE_PRICE";

          return (
            <div className="flex flex-wrap gap-1">
              <Button
                size="sm"
                variant="outline"
                className="h-7 text-xs"
                disabled={isStaleWithoutReupload}
                title={isStaleWithoutReupload ? "Upload harga terbaru terlebih dahulu sebelum menyetujui baris ini." : undefined}
                onClick={() => setApproveTarget(item)}
                aria-label={`Override setujui MTM ${item.instrumenKode}`}
              >
                Setuju
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="h-7 text-xs text-destructive hover:text-destructive"
                onClick={() => setRejectTarget(item)}
                aria-label={`Tolak MTM ${item.instrumenKode}`}
              >
                Tolak
              </Button>
            </div>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState, perms, canOverride],
  );

  const hasFilters = activeFilters.length > 0 || !!filters.q;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Mark-to-Market</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">MTM Harian</span>
      </nav>

      <div className="flex flex-wrap items-start justify-between gap-3">
        <h1 className="text-2xl font-semibold">Mark-to-Market Harian</h1>
        {perms.can("mtm.create") && (
          <Button size="sm" asChild>
            <Link href="/mtm/upload">
              <Plus className="mr-1.5 h-4 w-4" aria-hidden />
              Upload Manual
            </Link>
          </Button>
        )}
      </div>

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        error={error instanceof Error ? error : null}
        sorting={sortingState}
        onSortingChange={handleSortingChange}
        searchValue={filters.q}
        onSearchChange={(v) => {
          void filters.setQ(v);
          void filters.setCursor("");
          setCursorHistory([""]);
          setPageIndex(0);
        }}
        searchPlaceholder="Cari instrumen kode atau nama..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Status filter */}
            <Select
              value={filters.filterStatus || "all"}
              onValueChange={(v) => {
                void filters.setFilterStatus(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[190px]" aria-label="Filter status MTM">
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="AUTO_POSTED">Auto Diposting</SelectItem>
                <SelectItem value="PENDING_REVIEW">Menunggu Review</SelectItem>
                <SelectItem value="APPROVED">Disetujui</SelectItem>
                <SelectItem value="REJECTED">Ditolak</SelectItem>
                <SelectItem value="STALE_PRICE">Harga Kedaluwarsa</SelectItem>
              </SelectContent>
            </Select>

            {/* Klasifikasi filter */}
            <Select
              value={filters.filterKlasifikasi || "all"}
              onValueChange={(v) => {
                void filters.setFilterKlasifikasi(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter klasifikasi">
                <SelectValue placeholder="Semua Klasifikasi" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Klasifikasi</SelectItem>
                <SelectItem value="FVOCI_DEBT">FVOCI Utang</SelectItem>
                <SelectItem value="FVTPL">FVTPL</SelectItem>
                <SelectItem value="FVOCI_ELECTION">FVOCI Ekuitas</SelectItem>
                <SelectItem value="POCI">POCI</SelectItem>
              </SelectContent>
            </Select>

            {/* Sumber filter */}
            <Select
              value={filters.filterSumber || "all"}
              onValueChange={(v) => {
                void filters.setFilterSumber(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter sumber harga">
                <SelectValue placeholder="Semua Sumber" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Sumber</SelectItem>
                <SelectItem value="IBPA">IBPA</SelectItem>
                <SelectItem value="BEI">BEI</SelectItem>
                <SelectItem value="KSEI">KSEI</SelectItem>
                <SelectItem value="MANUAL">Manual</SelectItem>
                <SelectItem value="IBPA_MANUAL">IBPA Manual</SelectItem>
                <SelectItem value="BEI_MANUAL">BEI Manual</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = mtmListApi.exportUrl({
            sort: filters.sort || undefined,
            q: filters.q || undefined,
            "filter[status]": filters.filterStatus || undefined,
            "filter[klasifikasi_psak71]": filters.filterKlasifikasi as MtmKlasifikasi | undefined,
            "filter[harga_sumber]": filters.filterSumber as HargaSumber | undefined,
            "filter[deviation_flag]": filters.filterDeviation === "true" ? true : undefined,
            "filter[stale_price_flag]": filters.filterStale === "true" ? true : undefined,
            format: fmt,
          });
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        emptyMessage={
          hasFilters
            ? "Tidak ada MTM yang cocok dengan filter ini."
            : "Belum ada data MTM hari ini. Cron berjalan pukul 18:00 WIB atau upload manual."
        }
        onRetry={() => void refetch()}
      />

      {/* Override approve dialog */}
      {approveTarget && (
        <MtmOverrideApproveDialog
          open={!!approveTarget}
          onOpenChange={(v) => { if (!v) setApproveTarget(null); }}
          mtmId={approveTarget.id}
          instrumenKode={approveTarget.instrumenKode}
          tanggalMtm={approveTarget.tanggalMtm}
          deviationFlag={approveTarget.deviationFlag}
          deltaPct={approveTarget.deltaPct}
          thresholdPct={5}
          onSuccess={() => {
            setApproveTarget(null);
            void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
          }}
        />
      )}

      {/* Override reject dialog */}
      {rejectTarget && (
        <MtmOverrideRejectDialog
          open={!!rejectTarget}
          onOpenChange={(v) => { if (!v) setRejectTarget(null); }}
          mtmId={rejectTarget.id}
          instrumenKode={rejectTarget.instrumenKode}
          tanggalMtm={rejectTarget.tanggalMtm}
          onSuccess={() => {
            setRejectTarget(null);
            void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
          }}
        />
      )}
    </div>
  );
}

export default function MtmListPage() {
  return (
    <Suspense>
      <MtmListContent />
    </Suspense>
  );
}
