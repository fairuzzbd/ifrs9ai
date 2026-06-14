"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { MoreHorizontal, Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { format } from "date-fns";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import { counterpartyApi } from "@/lib/api/counterparty.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { CounterpartyListItem } from "@/lib/schemas/counterparty.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { v4 as uuidv4 } from "uuid";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useCounterpartyFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("kode_counterparty:asc"));
  const [filterTipe, setFilterTipe] = useQueryState("filter[tipe]", parseAsString.withDefault(""));
  const [filterStatus, setFilterStatus] = useQueryState("filter[status]", parseAsString.withDefault(""));
  const [filterWorkflow, setFilterWorkflow] = useQueryState("filter[workflow_status]", parseAsString.withDefault(""));
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return { q, setQ, sort, setSort, filterTipe, setFilterTipe, filterStatus, setFilterStatus, filterWorkflow, setFilterWorkflow, cursor, setCursor, limit };
}

// ---------------------------------------------------------------------------
// Delete dialog
// ---------------------------------------------------------------------------

function DeleteDialog({
  item,
  open,
  onOpenChange,
  onConfirm,
}: {
  item: CounterpartyListItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Counterparty?</DialogTitle>
          <DialogDescription>
            <strong>{item.nama}</strong> ({item.kodeCounterparty}) akan dihapus (soft-delete).
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={deleting}>Batal</Button>
          <Button
            variant="destructive"
            disabled={deleting}
            onClick={async () => {
              setDeleting(true);
              try {
                await onConfirm();
                onOpenChange(false);
              } finally {
                setDeleting(false);
              }
            }}
          >
            {deleting ? "Menghapus..." : "Hapus"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

function CounterpartyListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useCounterpartyFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<CounterpartyListItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams = React.useMemo(() => {
    const p: CounterpartyListParams = { limit: filters.limit };
    if (filters.sort) p.sort = filters.sort;
    if (filters.q) p.q = filters.q;
    if (filters.filterTipe) p["filter[tipe]"] = filters.filterTipe;
    if (filters.filterStatus) p["filter[status]"] = filters.filterStatus;
    if (filters.filterWorkflow) p["filter[workflow_status]"] = filters.filterWorkflow;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["counterparty", queryParams],
    queryFn: () => counterpartyApi.list(queryParams),
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
      return { id, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (newSorting: SortingState) => {
    void filters.setSort(
      newSorting.length === 0
        ? "kode_counterparty:asc"
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

  // Active filters
  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    const tipeLabels: Record<string, string> = {
      BANK: "Bank", PERUSAHAAN_ASURANSI: "Asuransi", PERUSAHAAN_SEKURITAS: "Sekuritas",
      MANAJER_INVESTASI: "Manajer Investasi", PEMERINTAH: "Pemerintah",
      KORPORASI: "Korporasi", LAINNYA: "Lainnya",
    };
    const statusLabels: Record<string, string> = {
      AKTIF: "Aktif", TIDAK_AKTIF: "Tidak Aktif", DIBLOKIR: "Diblokir",
    };
    const wfLabels: Record<string, string> = {
      DRAFT: "Draf", PENDING_REVIEW: "Menunggu Review",
      PENDING_APPROVAL: "Menunggu Approval", APPROVED: "Disetujui", RETURNED: "Dikembalikan",
    };
    if (filters.filterTipe) f.push({ key: "filter[tipe]", label: "Tipe", value: filters.filterTipe, displayValue: tipeLabels[filters.filterTipe] ?? filters.filterTipe });
    if (filters.filterStatus) f.push({ key: "filter[status]", label: "Status", value: filters.filterStatus, displayValue: statusLabels[filters.filterStatus] ?? filters.filterStatus });
    if (filters.filterWorkflow) f.push({ key: "filter[workflow_status]", label: "Workflow", value: filters.filterWorkflow, displayValue: wfLabels[filters.filterWorkflow] ?? filters.filterWorkflow });
    return f;
  }, [filters.filterTipe, filters.filterStatus, filters.filterWorkflow]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[tipe]") void filters.setFilterTipe("");
    if (key === "filter[status]") void filters.setFilterStatus("");
    if (key === "filter[workflow_status]") void filters.setFilterWorkflow("");
    void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterTipe(""); void filters.setFilterStatus(""); void filters.setFilterWorkflow("");
    void filters.setQ(""); void filters.setCursor("");
    setCursorHistory([""]); setPageIndex(0);
  };

  // Delete
  const handleDelete = async (item: CounterpartyListItem) => {
    try {
      await counterpartyApi.delete(item.id, uuidv4());
      notify.success(`Counterparty ${item.kodeCounterparty} — ${item.nama} berhasil dihapus.`);
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  // Submit
  const handleSubmit = async (item: CounterpartyListItem) => {
    try {
      await counterpartyApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(`Counterparty ${item.kodeCounterparty} berhasil disubmit untuk review.`, {
        action: { label: "Lihat detail", onClick: () => router.push(`/master/counterparty/${item.id}`) },
      });
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  // Columns — PII columns intentionally absent
  const columns: ColumnDef<CounterpartyListItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "kodeCounterparty",
        header: () => <SortHeader label="Kode" sortKey="kode_counterparty" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => <code className="font-mono font-bold text-sm">{row.original.kodeCounterparty}</code>,
      },
      {
        accessorKey: "nama",
        header: () => <SortHeader label="Nama" sortKey="nama" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => (
          <span className="block max-w-[200px] truncate" title={row.original.nama}>
            {row.original.nama}
          </span>
        ),
      },
      {
        accessorKey: "tipe",
        header: "Tipe",
        cell: ({ row }) => (
          <span className="rounded-md border px-1.5 py-0.5 text-xs">
            {row.original.tipe.replace(/_/g, " ")}
          </span>
        ),
      },
      {
        accessorKey: "ratingPefindoCurrent",
        header: "Rating Pefindo",
        cell: ({ row }) => (
          <span className={cn("font-mono text-sm font-medium", !row.original.ratingPefindoCurrent && "text-muted-foreground")}>
            {row.original.ratingPefindoCurrent ?? "—"}
          </span>
        ),
      },
      {
        accessorKey: "status",
        header: "Status",
        cell: ({ row }) => {
          const statusColor: Record<string, string> = {
            AKTIF: "text-green-700",
            TIDAK_AKTIF: "text-muted-foreground",
            DIBLOKIR: "text-destructive",
          };
          return (
            <span className={cn("text-xs font-medium", statusColor[row.original.status])}>
              {row.original.status.replace(/_/g, " ")}
            </span>
          );
        },
      },
      {
        accessorKey: "workflowStatus",
        header: () => <SortHeader label="Workflow" sortKey="workflow_status" sorting={sortingState} onToggle={toggleSort} />,
        cell: ({ row }) => <WorkflowStatusBadge status={row.original.workflowStatus} />,
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          const isDraft = item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
          const canEdit = perms.canUpdate("counterparty") && isDraft;
          const canDelete = perms.canDelete("counterparty") && isDraft;
          const canSubmit = perms.canSubmit("counterparty") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8" aria-label={`Aksi untuk ${item.kodeCounterparty}`}>
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/counterparty/${item.id}`}>Lihat Detail</Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/counterparty/${item.id}/edit`}>Edit</Link>
                  </DropdownMenuItem>
                )}
                {canSubmit && (
                  <DropdownMenuItem onClick={() => void handleSubmit(item)}>
                    {item.workflowStatus === "RETURNED" ? "Kirim Ulang untuk Review" : "Kirim untuk Review"}
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href={`/master/counterparty/${item.id}/history`}>Riwayat Audit</Link>
                </DropdownMenuItem>
                <DropdownMenuItem asChild>
                  <Link href={`/master/counterparty/${item.id}/rating-history`}>Rating History</Link>
                </DropdownMenuItem>
                {canDelete && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem className="text-destructive" onClick={() => setDeleteTarget(item)}>
                      Hapus
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState, perms],
  );

  const exportQueryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterTipe) p["filter[tipe]"] = filters.filterTipe;
    if (filters.filterStatus) p["filter[status]"] = filters.filterStatus;
    if (filters.filterWorkflow) p["filter[workflow_status]"] = filters.filterWorkflow;
    return p;
  }, [filters]);

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Counterparty</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Counterparty</h1>

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
          void filters.setQ(v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0);
        }}
        searchPlaceholder="Cari kode, nama..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            <Select value={filters.filterTipe || "all"} onValueChange={(v) => { void filters.setFilterTipe(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}>
              <SelectTrigger className="h-9 w-[200px]" aria-label="Filter tipe counterparty">
                <SelectValue placeholder="Semua Tipe" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tipe</SelectItem>
                <SelectItem value="BANK">Bank</SelectItem>
                <SelectItem value="PERUSAHAAN_ASURANSI">Asuransi</SelectItem>
                <SelectItem value="PERUSAHAAN_SEKURITAS">Sekuritas</SelectItem>
                <SelectItem value="MANAJER_INVESTASI">Manajer Investasi</SelectItem>
                <SelectItem value="PEMERINTAH">Pemerintah</SelectItem>
                <SelectItem value="KORPORASI">Korporasi</SelectItem>
                <SelectItem value="LAINNYA">Lainnya</SelectItem>
              </SelectContent>
            </Select>
            <Select value={filters.filterStatus || "all"} onValueChange={(v) => { void filters.setFilterStatus(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}>
              <SelectTrigger className="h-9 w-[140px]" aria-label="Filter status counterparty">
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="AKTIF">Aktif</SelectItem>
                <SelectItem value="TIDAK_AKTIF">Tidak Aktif</SelectItem>
                <SelectItem value="DIBLOKIR">Diblokir</SelectItem>
              </SelectContent>
            </Select>
            <Select value={filters.filterWorkflow || "all"} onValueChange={(v) => { void filters.setFilterWorkflow(v === "all" ? "" : v); void filters.setCursor(""); setCursorHistory([""]); setPageIndex(0); }}>
              <SelectTrigger className="h-9 w-[200px]" aria-label="Filter status workflow">
                <SelectValue placeholder="Semua Workflow" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Workflow</SelectItem>
                <SelectItem value="DRAFT">Draf</SelectItem>
                <SelectItem value="PENDING_REVIEW">Menunggu Review</SelectItem>
                <SelectItem value="PENDING_APPROVAL">Menunggu Approval</SelectItem>
                <SelectItem value="APPROVED">Disetujui</SelectItem>
                <SelectItem value="RETURNED">Dikembalikan</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = counterpartyApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof counterpartyApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("counterparty") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/counterparty/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Counterparty
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada counterparty yang cocok dengan pencarian."
            : "Belum ada counterparty. Klik '+ Tambah Counterparty' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {deleteTarget && (
        <DeleteDialog
          item={deleteTarget}
          open={!!deleteTarget}
          onOpenChange={(v) => { if (!v) setDeleteTarget(null); }}
          onConfirm={() => handleDelete(deleteTarget)}
        />
      )}
    </div>
  );
}

// Type needed inline since import is circular if we add to api file
type CounterpartyListParams = Parameters<typeof counterpartyApi.list>[0];

export default function CounterpartyListPage() {
  return (
    <Suspense>
      <CounterpartyListContent />
    </Suspense>
  );
}
