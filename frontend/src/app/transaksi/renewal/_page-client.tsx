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
import { RenewalStatusBadge } from "@/components/blips/renewal/RenewalStatusBadge";
import { RenewalSkemaBadge } from "@/components/blips/renewal/RenewalSkemaBadge";
import { RenewalApproveDialog } from "@/components/blips/renewal/RenewalApproveDialog";
import { RenewalRejectDialog } from "@/components/blips/renewal/RenewalRejectDialog";

import { renewalListApi, renewalQueryKeys } from "@/lib/api/renewal.api";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  RENEWAL_STATUS_LABELS,
  RENEWAL_SKEMA_LABELS,
  type RenewalListItem,
  type RenewalStatus,
  type RenewalSkema,
} from "@/lib/schemas/renewal.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

// ---------------------------------------------------------------------------
// IDR formatter (grouped thousands, no decimal for list view)
// ---------------------------------------------------------------------------

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

// ---------------------------------------------------------------------------
// URL filter state
// ---------------------------------------------------------------------------

function useRenewalFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("created_at:desc"));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status]", parseAsString.withDefault(""));
  const [filterSkema, setFilterSkema] = useQueryState("filter[skema]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return { q, setQ, sort, setSort, filterStatus, setFilterStatus, filterSkema, setFilterSkema, cursor, setCursor, limit };
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function RenewalListContent() {
  const perms = usePermissions();
  const queryClient = useQueryClient();
  const filters = useRenewalFilters();

  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const [approveTarget, setApproveTarget] = React.useState<RenewalListItem | null>(null);
  const [rejectTarget, setRejectTarget] = React.useState<RenewalListItem | null>(null);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: renewalQueryKeys.list({
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
      "filter[status]": filters.filterStatus || undefined,
      "filter[skema]": filters.filterSkema || undefined,
      cursor: filters.cursor || undefined,
    }),
    queryFn: () =>
      renewalListApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[status]": filters.filterStatus || undefined,
        "filter[skema]": filters.filterSkema || undefined,
        cursor: filters.cursor || undefined,
      }),
    staleTime: 30_000,
  });

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
        ? "created_at:desc"
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

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterStatus) {
      f.push({
        key: "filter[status]",
        label: "Status",
        value: filters.filterStatus,
        displayValue: RENEWAL_STATUS_LABELS[filters.filterStatus as RenewalStatus] ?? filters.filterStatus,
      });
    }
    if (filters.filterSkema) {
      f.push({
        key: "filter[skema]",
        label: "Skema",
        value: filters.filterSkema,
        displayValue: RENEWAL_SKEMA_LABELS[filters.filterSkema as RenewalSkema] ?? filters.filterSkema,
      });
    }
    return f;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.filterStatus, filters.filterSkema]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[status]") void filters.setFilterStatus("");
    if (key === "filter[skema]") void filters.setFilterSkema("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterSkema("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // Persona gating: approve/reject buttons only for ROLE-APPR-TR + status=PENDING_APPROVAL
  const canApprove = perms.canApprove("transaksi");
  const currentUserId = perms.userId;

  const columns: ColumnDef<RenewalListItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "instrumenLamaKode",
        header: () => (
          <SortHeader label="Instrumen" sortKey="instrumen_kode" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <Link
            href={`/transaksi/renewal/${row.original.id}`}
            className="font-mono text-sm text-primary hover:underline"
            aria-label={`Lihat detail renewal ${row.original.instrumenLamaKode}`}
          >
            {row.original.instrumenLamaKode}
          </Link>
        ),
      },
      {
        accessorKey: "skema",
        header: "Skema",
        cell: ({ row }) => <RenewalSkemaBadge skema={row.original.skema} size="sm" />,
      },
      {
        accessorKey: "tenorBaruBulan",
        header: () => (
          <SortHeader label="Tenor (bln)" sortKey="tenor_baru_bulan" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="text-sm font-mono">{row.original.tenorBaruBulan}</span>
        ),
      },
      {
        accessorKey: "rateBaruPersen",
        header: () => (
          <SortHeader label="Rate % p.a." sortKey="rate_baru_persen" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="text-sm font-mono text-right block">
            {parseFloat(row.original.rateBaruPersen).toFixed(2)}%
          </span>
        ),
      },
      {
        accessorKey: "pokokBaru",
        header: () => (
          <SortHeader label="Pokok Baru" sortKey="pokok_baru" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="text-sm font-mono text-right block">
            {IDR.format(parseFloat(row.original.pokokBaru))}
          </span>
        ),
      },
      {
        accessorKey: "tanggalEfektifBaru",
        header: () => (
          <SortHeader label="Tgl Efektif" sortKey="tanggal_efektif_baru" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="text-sm whitespace-nowrap">{row.original.tanggalEfektifBaru}</span>
        ),
      },
      {
        accessorKey: "status",
        header: () => (
          <SortHeader label="Status" sortKey="status" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => <RenewalStatusBadge status={row.original.status} size="sm" />,
      },
      {
        id: "jurnal",
        header: "Jurnal",
        cell: ({ row }) =>
          row.original.jurnalEntryId ? (
            <Link
              href={`/jurnal/${row.original.jurnalEntryId}`}
              className="inline-flex items-center text-xs text-primary hover:underline"
              aria-label={`Lihat jurnal untuk renewal ${row.original.instrumenLamaKode}`}
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
          const isPending = item.status === "PENDING_APPROVAL";
          // SoD: approver must differ from maker (client-side gating absent-from-DOM)
          const isMaker = item.makerId === currentUserId;

          if (!canApprove || !isPending || isMaker) return null;

          return (
            <div className="flex gap-1">
              <Button
                size="sm"
                variant="outline"
                className="h-7 text-xs"
                onClick={() => setApproveTarget(item)}
                aria-label={`Setujui renewal ${item.instrumenLamaKode}`}
              >
                Setuju
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="h-7 text-xs text-destructive hover:text-destructive"
                onClick={() => setRejectTarget(item)}
                aria-label={`Tolak renewal ${item.instrumenLamaKode}`}
              >
                Tolak
              </Button>
            </div>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState, canApprove, currentUserId],
  );

  const hasFilters = activeFilters.length > 0 || !!filters.q;

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Transaksi</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Renewal Deposito</span>
      </nav>

      <div className="flex flex-wrap items-start justify-between gap-3">
        <h1 className="text-2xl font-semibold">Renewal Deposito</h1>
        {perms.canCreate("transaksi") && (
          <Button size="sm" asChild>
            <Link href="/transaksi/renewal/new">
              <Plus className="mr-1.5 h-4 w-4" aria-hidden />
              Buat Renewal
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
        searchPlaceholder="Cari kode instrumen..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            <Select
              value={filters.filterStatus || "all"}
              onValueChange={(v) => {
                void filters.setFilterStatus(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[190px]" aria-label="Filter status renewal">
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="PENDING_APPROVAL">Menunggu Approval</SelectItem>
                <SelectItem value="POSTED">Diposting</SelectItem>
                <SelectItem value="REJECTED">Ditolak</SelectItem>
              </SelectContent>
            </Select>

            <Select
              value={filters.filterSkema || "all"}
              onValueChange={(v) => {
                void filters.setFilterSkema(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter skema renewal">
                <SelectValue placeholder="Semua Skema" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Skema</SelectItem>
                <SelectItem value="POKOK_SAJA">Pokok Saja</SelectItem>
                <SelectItem value="POKOK_PLUS_BUNGA">Pokok + Bunga</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = renewalListApi.exportUrl({
            sort: filters.sort || undefined,
            q: filters.q || undefined,
            "filter[status]": filters.filterStatus || undefined,
            "filter[skema]": filters.filterSkema || undefined,
            format: fmt,
          });
          window.open(url, "_blank");
        }}
        onRefresh={() => {
          void refetch();
          setLastUpdated(new Date());
        }}
        lastUpdated={lastUpdated}
        emptyMessage={
          hasFilters
            ? "Tidak ada renewal yang cocok dengan filter ini."
            : "Belum ada data renewal deposito."
        }
        onRetry={() => void refetch()}
      />

      {/* Approve dialog */}
      {approveTarget && (
        <RenewalApproveDialog
          open={!!approveTarget}
          onOpenChange={(v) => { if (!v) setApproveTarget(null); }}
          renewalId={approveTarget.id}
          instrumenKode={approveTarget.instrumenLamaKode}
          makerId={approveTarget.makerId}
          onSuccess={() => {
            setApproveTarget(null);
            void queryClient.invalidateQueries({ queryKey: renewalQueryKeys.lists() });
          }}
        />
      )}

      {/* Reject dialog */}
      {rejectTarget && (
        <RenewalRejectDialog
          open={!!rejectTarget}
          onOpenChange={(v) => { if (!v) setRejectTarget(null); }}
          renewalId={rejectTarget.id}
          instrumenKode={rejectTarget.instrumenLamaKode}
          onSuccess={() => {
            setRejectTarget(null);
            void queryClient.invalidateQueries({ queryKey: renewalQueryKeys.lists() });
          }}
        />
      )}
    </div>
  );
}

export default function RenewalListPage() {
  return (
    <Suspense>
      <RenewalListContent />
    </Suspense>
  );
}
