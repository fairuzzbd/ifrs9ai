"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  useQueryState,
  parseAsString,
  parseAsInteger,
} from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { MoreHorizontal, Plus, Upload } from "lucide-react";
import { useRouter } from "next/navigation";
import { format } from "date-fns";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { ExportButton } from "@/components/blips/ExportButton";
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

import { coaApi } from "@/lib/api/coa.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { CoAItem, TipeAkun, PosisiNormal } from "@/lib/schemas/coa.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { v4 as uuidv4 } from "uuid";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useCoAFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("kode_akun:asc"),
  );
  const [filterTipe, setFilterTipe] = useQueryState(
    "filter[tipe_akun]",
    parseAsString.withDefault(""),
  );
  const [filterPosisi, setFilterPosisi] = useQueryState(
    "filter[posisi_normal]",
    parseAsString.withDefault(""),
  );
  const [filterAktif, setFilterAktif] = useQueryState(
    "filter[aktif_flag]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [cursor, setCursor] = useQueryState(
    "cursor",
    parseAsString.withDefault(""),
  );
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ,
    sort, setSort,
    filterTipe, setFilterTipe,
    filterPosisi, setFilterPosisi,
    filterAktif, setFilterAktif,
    filterStatus, setFilterStatus,
    cursor, setCursor,
    limit,
  };
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
  item: CoAItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Akun {item.kodeAkun}?</DialogTitle>
          <DialogDescription>
            Akun <strong>{item.namaAkun}</strong> ({item.kodeAkun}) akan dihapus
            dari sistem (soft-delete). Tindakan ini dapat dibatalkan oleh
            administrator.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={deleting}
          >
            Batal
          </Button>
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
// Tipe akun badge
// ---------------------------------------------------------------------------

const TIPE_COLORS: Record<TipeAkun, string> = {
  ASET: "bg-blue-50 text-blue-700 border-blue-200",
  LIABILITAS: "bg-red-50 text-red-700 border-red-200",
  EKUITAS: "bg-purple-50 text-purple-700 border-purple-200",
  PENDAPATAN: "bg-green-50 text-green-700 border-green-200",
  BEBAN: "bg-amber-50 text-amber-700 border-amber-200",
  KONTINJEN: "bg-gray-50 text-gray-700 border-gray-200",
};

function TipeAkunBadge({ tipe }: { tipe: TipeAkun }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-medium",
        TIPE_COLORS[tipe],
      )}
    >
      {tipe}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

function CoAListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useCoAFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<CoAItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterTipe) p["filter[tipe_akun]"] = filters.filterTipe;
    if (filters.filterPosisi) p["filter[posisi_normal]"] = filters.filterPosisi;
    if (filters.filterAktif !== "")
      p["filter[aktif_flag]"] = filters.filterAktif === "true";
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["coa", queryParams],
    queryFn: () =>
      coaApi.list({
        ...queryParams,
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[tipe_akun]": (filters.filterTipe as TipeAkun) || undefined,
        "filter[posisi_normal]": (filters.filterPosisi as PosisiNormal) || undefined,
        "filter[aktif_flag]":
          filters.filterAktif !== ""
            ? filters.filterAktif === "true"
            : undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        cursor: filters.cursor || undefined,
      }),
    staleTime: 30_000,
  });

  const handleRefresh = () => {
    void refetch();
    setLastUpdated(new Date());
  };

  // ---------------------------------------------------------------------------
  // Sort
  // ---------------------------------------------------------------------------

  const sortingState: SortingState = React.useMemo(() => {
    if (!filters.sort) return [];
    return filters.sort.split(",").map((part) => {
      const [id, dir] = part.split(":");
      return { id, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (newSorting: SortingState) => {
    if (newSorting.length === 0) {
      void filters.setSort("kode_akun:asc");
    } else {
      void filters.setSort(
        newSorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","),
      );
    }
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const toggleSort = (colId: string) => {
    const current = sortingState.find((s) => s.id === colId);
    if (!current) {
      handleSortingChange([{ id: colId, desc: false }]);
    } else if (!current.desc) {
      handleSortingChange([{ id: colId, desc: true }]);
    } else {
      handleSortingChange([]);
    }
  };

  // ---------------------------------------------------------------------------
  // Pagination
  // ---------------------------------------------------------------------------

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

  // ---------------------------------------------------------------------------
  // Active filters
  // ---------------------------------------------------------------------------

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterTipe) {
      f.push({
        key: "filter[tipe_akun]",
        label: "Tipe",
        value: filters.filterTipe,
        displayValue: filters.filterTipe,
      });
    }
    if (filters.filterPosisi) {
      f.push({
        key: "filter[posisi_normal]",
        label: "Posisi",
        value: filters.filterPosisi,
        displayValue: filters.filterPosisi,
      });
    }
    if (filters.filterAktif !== "") {
      f.push({
        key: "filter[aktif_flag]",
        label: "Aktif",
        value: filters.filterAktif,
        displayValue: filters.filterAktif === "true" ? "Ya" : "Tidak",
      });
    }
    if (filters.filterStatus) {
      const labels: Record<string, string> = {
        DRAFT: "Draf",
        PENDING_REVIEW: "Menunggu Review",
        PENDING_APPROVAL: "Menunggu Approval",
        APPROVED: "Disetujui",
        RETURNED: "Dikembalikan",
      };
      f.push({
        key: "filter[workflow_status]",
        label: "Status",
        value: filters.filterStatus,
        displayValue: labels[filters.filterStatus] ?? filters.filterStatus,
      });
    }
    return f;
  }, [filters.filterTipe, filters.filterPosisi, filters.filterAktif, filters.filterStatus]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[tipe_akun]") void filters.setFilterTipe("");
    if (key === "filter[posisi_normal]") void filters.setFilterPosisi("");
    if (key === "filter[aktif_flag]") void filters.setFilterAktif("");
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterTipe("");
    void filters.setFilterPosisi("");
    void filters.setFilterAktif("");
    void filters.setFilterStatus("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: CoAItem) => {
    try {
      await coaApi.delete(item.id, uuidv4());
      notify.success(
        `Akun ${item.kodeAkun} — ${item.namaAkun} berhasil dihapus.`,
      );
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const handleSubmit = async (item: CoAItem) => {
    try {
      await coaApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(
        `Akun ${item.kodeAkun} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/coa/${item.id}`),
          },
        },
      );
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  // ---------------------------------------------------------------------------
  // Columns
  // ---------------------------------------------------------------------------

  const columns: ColumnDef<CoAItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "kodeAkun",
        header: () => (
          <SortHeader
            label="Kode Akun"
            sortKey="kode_akun"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <code className="font-mono font-bold text-sm">
            {row.original.kodeAkun}
          </code>
        ),
      },
      {
        accessorKey: "namaAkun",
        header: () => (
          <SortHeader
            label="Nama Akun"
            sortKey="nama_akun"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span
            className="block max-w-[240px] truncate"
            title={row.original.namaAkun}
          >
            {row.original.namaAkun}
          </span>
        ),
      },
      {
        accessorKey: "tipeAkun",
        header: "Tipe",
        cell: ({ row }) => <TipeAkunBadge tipe={row.original.tipeAkun} />,
      },
      {
        accessorKey: "posisiNormal",
        header: "Posisi",
        cell: ({ row }) => (
          <span className="text-xs font-mono">{row.original.posisiNormal}</span>
        ),
      },
      {
        accessorKey: "matauangNative",
        header: "Mata Uang",
        cell: ({ row }) => (
          <span className="font-mono text-xs">{row.original.matauangNative}</span>
        ),
      },
      {
        accessorKey: "workflowStatus",
        header: () => (
          <SortHeader
            label="Status"
            sortKey="workflow_status"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <WorkflowStatusBadge status={row.original.workflowStatus} />
        ),
      },
      {
        accessorKey: "aktifFlag",
        header: "Aktif",
        cell: ({ row }) => (
          <span
            className={cn(
              "text-xs",
              row.original.aktifFlag
                ? "text-green-700"
                : "text-muted-foreground",
            )}
          >
            {row.original.aktifFlag ? "Ya" : "Tidak"}
          </span>
        ),
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          const isDraft =
            item.workflowStatus === "DRAFT" ||
            item.workflowStatus === "RETURNED";
          const canEdit = perms.canUpdate("coa") && isDraft;
          const canDelete = perms.canDelete("coa") && isDraft;
          const canSubmit = perms.canSubmit("coa") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk akun ${item.kodeAkun}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/coa/${item.id}`}>Lihat Detail</Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/coa/${item.id}/edit`}>Edit</Link>
                  </DropdownMenuItem>
                )}
                {canSubmit && (
                  <DropdownMenuItem onClick={() => void handleSubmit(item)}>
                    {item.workflowStatus === "RETURNED"
                      ? "Kirim Ulang untuk Review"
                      : "Kirim untuk Review"}
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href={`/master/coa/${item.id}/history`}>
                    Riwayat Audit
                  </Link>
                </DropdownMenuItem>
                {canDelete && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                      className="text-destructive"
                      onClick={() => setDeleteTarget(item)}
                    >
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
    if (filters.filterTipe) p["filter[tipe_akun]"] = filters.filterTipe;
    if (filters.filterPosisi) p["filter[posisi_normal]"] = filters.filterPosisi;
    if (filters.filterAktif !== "")
      p["filter[aktif_flag]"] = filters.filterAktif === "true";
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    return p;
  }, [filters]);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Chart of Accounts</span>
      </nav>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold">Chart of Accounts</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Struktur Chart of Accounts mengacu pada standar akuntansi PSAK/IFRS.
          </p>
        </div>
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
        searchPlaceholder="Cari kode akun, nama akun..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Tipe Akun */}
            <Select
              value={filters.filterTipe || "all"}
              onValueChange={(v) => {
                void filters.setFilterTipe(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter tipe akun">
                <SelectValue placeholder="Semua Tipe" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tipe</SelectItem>
                <SelectItem value="ASET">Aset</SelectItem>
                <SelectItem value="LIABILITAS">Liabilitas</SelectItem>
                <SelectItem value="EKUITAS">Ekuitas</SelectItem>
                <SelectItem value="PENDAPATAN">Pendapatan</SelectItem>
                <SelectItem value="BEBAN">Beban</SelectItem>
                <SelectItem value="KONTINJEN">Kontinjen</SelectItem>
              </SelectContent>
            </Select>

            {/* Posisi Normal */}
            <Select
              value={filters.filterPosisi || "all"}
              onValueChange={(v) => {
                void filters.setFilterPosisi(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[140px]" aria-label="Filter posisi normal">
                <SelectValue placeholder="Semua Posisi" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Posisi</SelectItem>
                <SelectItem value="DEBIT">Debit</SelectItem>
                <SelectItem value="KREDIT">Kredit</SelectItem>
              </SelectContent>
            </Select>

            {/* Aktif */}
            <Select
              value={filters.filterAktif === "" ? "all" : filters.filterAktif}
              onValueChange={(v) => {
                void filters.setFilterAktif(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[120px]" aria-label="Filter status aktif">
                <SelectValue placeholder="Semua" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua</SelectItem>
                <SelectItem value="true">Aktif</SelectItem>
                <SelectItem value="false">Tidak Aktif</SelectItem>
              </SelectContent>
            </Select>

            {/* Status Workflow */}
            <Select
              value={filters.filterStatus || "all"}
              onValueChange={(v) => {
                void filters.setFilterStatus(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[190px]" aria-label="Filter status workflow">
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
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
          const url = coaApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof coaApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("coa") && !perms.isAuditRole() ? (
            <div className="flex items-center gap-2">
              <Button size="sm" variant="outline" asChild>
                <Link href="/master/coa/import">
                  <Upload className="mr-1.5 h-4 w-4" aria-hidden />
                  Import XLSX
                </Link>
              </Button>
              <Button size="sm" asChild>
                <Link href="/master/coa/new">
                  <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                  Buat Akun
                </Link>
              </Button>
            </div>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada akun yang cocok dengan pencarian."
            : "Belum ada Chart of Accounts. Klik '+ Buat Akun' atau 'Import XLSX' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete confirmation dialog */}
      {deleteTarget && (
        <DeleteDialog
          item={deleteTarget}
          open={!!deleteTarget}
          onOpenChange={(v) => {
            if (!v) setDeleteTarget(null);
          }}
          onConfirm={() => handleDelete(deleteTarget)}
        />
      )}
    </div>
  );
}

export default function CoAListPage() {
  return (
    <Suspense>
      <CoAListContent />
    </Suspense>
  );
}
