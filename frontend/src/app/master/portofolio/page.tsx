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
import { MoreHorizontal, Plus } from "lucide-react";
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

import { portofolioApi } from "@/lib/api/portofolio.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  BM_CATEGORY_PSAK71,
  type PortofolioItem,
} from "@/lib/schemas/portofolio.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { v4 as uuidv4 } from "uuid";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function usePortofolioFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("kode_portofolio:asc"),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterBm, setFilterBm] = useQueryState(
    "filter[bm_category_default]",
    parseAsString.withDefault(""),
  );
  const [filterAktif, setFilterAktif] = useQueryState(
    "filter[aktif_flag]",
    parseAsString.withDefault(""),
  );
  const [cursor, setCursor] = useQueryState(
    "cursor",
    parseAsString.withDefault(""),
  );
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q,
    setQ,
    sort,
    setSort,
    filterStatus,
    setFilterStatus,
    filterBm,
    setFilterBm,
    filterAktif,
    setFilterAktif,
    cursor,
    setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// Delete dialog
// ---------------------------------------------------------------------------

function DeleteDialog({
  id,
  kode,
  nama,
  open,
  onOpenChange,
  onConfirm,
}: {
  id: string;
  kode: string;
  nama: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Portofolio {kode}?</DialogTitle>
          <DialogDescription>
            Portofolio <strong>{nama}</strong> ({kode}) akan dihapus dari
            sistem (soft-delete). Tindakan ini dapat dibatalkan oleh
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
// BM category badge
// ---------------------------------------------------------------------------

const BM_COLORS: Record<string, string> = {
  HTC: "bg-green-50 text-green-700 border-green-200",
  HTCS: "bg-blue-50 text-blue-700 border-blue-200",
  OTHER: "bg-orange-50 text-orange-700 border-orange-200",
};

function BmBadge({ value }: { value: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-medium",
        BM_COLORS[value] ?? "bg-muted text-muted-foreground",
      )}
      title={BM_CATEGORY_PSAK71[value as keyof typeof BM_CATEGORY_PSAK71]}
    >
      {value}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

function PortofolioListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = usePortofolioFilters();

  const [deleteTarget, setDeleteTarget] =
    React.useState<PortofolioItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  // Build query params
  const queryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterBm) p["filter[bm_category_default]"] = filters.filterBm;
    if (filters.filterAktif !== "")
      p["filter[aktif_flag]"] = filters.filterAktif === "true";
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["portofolio", queryParams],
    queryFn: () =>
      portofolioApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[bm_category_default]": filters.filterBm || undefined,
        "filter[aktif_flag]":
          filters.filterAktif !== ""
            ? filters.filterAktif === "true"
            : undefined,
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
      void filters.setSort("kode_portofolio:asc");
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
    if (filters.filterBm) {
      f.push({
        key: "filter[bm_category_default]",
        label: "BM",
        value: filters.filterBm,
        displayValue: filters.filterBm,
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
    return f;
  }, [filters.filterStatus, filters.filterBm, filters.filterAktif]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[bm_category_default]") void filters.setFilterBm("");
    if (key === "filter[aktif_flag]") void filters.setFilterAktif("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterBm("");
    void filters.setFilterAktif("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: PortofolioItem) => {
    try {
      await portofolioApi.delete(item.id, uuidv4());
      notify.destructive(
        `Portofolio ${item.kodePortofolio} — ${item.nama} berhasil dihapus.`,
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

  const handleSubmit = async (item: PortofolioItem) => {
    try {
      await portofolioApi.submit(item.id, {
        rowVersion: item.rowVersion,
      });
      notify.success(
        `Portofolio ${item.kodePortofolio} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/portofolio/${item.id}`),
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

  const columns: ColumnDef<PortofolioItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "kodePortofolio",
        header: () => (
          <SortHeader
            label="Kode"
            sortKey="kode_portofolio"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <code className="font-mono font-bold text-sm">
            {row.original.kodePortofolio}
          </code>
        ),
      },
      {
        accessorKey: "nama",
        header: () => (
          <SortHeader
            label="Nama"
            sortKey="nama"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span
            className="block max-w-[240px] truncate"
            title={row.original.nama}
          >
            {row.original.nama}
          </span>
        ),
      },
      {
        accessorKey: "bmCategoryDefault",
        header: () => (
          <SortHeader
            label="BM Default"
            sortKey="bm_category_default"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <BmBadge value={row.original.bmCategoryDefault} />
        ),
      },
      {
        accessorKey: "periodeReviewTerakhir",
        header: "Review Terakhir",
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground">
            {row.original.periodeReviewTerakhir ?? "—"}
          </span>
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
          const canEdit = perms.canUpdate("portofolio") && isDraft;
          const canDelete = perms.canDelete("portofolio") && isDraft;
          const canSubmit = perms.canSubmit("portofolio") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk portofolio ${item.kodePortofolio}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/portofolio/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/portofolio/${item.id}/edit`}>
                      Edit
                    </Link>
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
                  <Link href={`/master/portofolio/${item.id}/history`}>
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

  // Export params
  const exportQueryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterBm) p["filter[bm_category_default]"] = filters.filterBm;
    if (filters.filterAktif !== "")
      p["filter[aktif_flag]"] = filters.filterAktif === "true";
    return p;
  }, [filters]);

  const exportFilename = `portofolio-${format(new Date(), "yyyyMMdd")}`;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Header */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Portofolio</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Portofolio</h1>

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
        searchPlaceholder="Cari kode, nama portofolio..."
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
              <SelectTrigger
                className="h-9 w-[180px]"
                aria-label="Filter status workflow"
              >
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="DRAFT">Draf</SelectItem>
                <SelectItem value="PENDING_REVIEW">Menunggu Review</SelectItem>
                <SelectItem value="PENDING_APPROVAL">
                  Menunggu Approval
                </SelectItem>
                <SelectItem value="APPROVED">Disetujui</SelectItem>
                <SelectItem value="RETURNED">Dikembalikan</SelectItem>
              </SelectContent>
            </Select>

            {/* BM category filter */}
            <Select
              value={filters.filterBm || "all"}
              onValueChange={(v) => {
                void filters.setFilterBm(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[150px]"
                aria-label="Filter Business Model"
              >
                <SelectValue placeholder="Semua BM" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua BM</SelectItem>
                <SelectItem value="HTC">HTC</SelectItem>
                <SelectItem value="HTCS">HTCS</SelectItem>
                <SelectItem value="OTHER">OTHER</SelectItem>
              </SelectContent>
            </Select>

            {/* Aktif filter */}
            <Select
              value={
                filters.filterAktif === ""
                  ? "all"
                  : filters.filterAktif === "true"
                    ? "true"
                    : "false"
              }
              onValueChange={(v) => {
                void filters.setFilterAktif(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[120px]"
                aria-label="Filter status aktif"
              >
                <SelectValue placeholder="Semua" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua</SelectItem>
                <SelectItem value="true">Aktif</SelectItem>
                <SelectItem value="false">Tidak Aktif</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = portofolioApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof portofolioApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("portofolio") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/portofolio/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Portofolio
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada portofolio yang cocok dengan pencarian."
            : "Belum ada portofolio. Klik '+ Tambah Portofolio' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete confirmation dialog */}
      {deleteTarget && (
        <DeleteDialog
          id={deleteTarget.id}
          kode={deleteTarget.kodePortofolio}
          nama={deleteTarget.nama}
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

export default function PortofolioListPage() {
  return (
    <Suspense>
      <PortofolioListContent />
    </Suspense>
  );
}
