"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  useQueryState,
  parseAsString,
  parseAsInteger,
  parseAsBoolean,
} from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { MoreHorizontal, Plus, Shield } from "lucide-react";
import { useRouter } from "next/navigation";
import { format, parseISO } from "date-fns";

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

import { mataUangApi } from "@/lib/api/mata-uang.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { MataUangItem } from "@/lib/schemas/mata-uang.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { v4 as uuidv4 } from "uuid";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useMataUangFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("kode_mata_uang:asc"),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterAktif, setFilterAktif] = useQueryState(
    "filter[aktif_flag]",
    parseAsString.withDefault(""),
  );
  const [filterSumber, setFilterSumber] = useQueryState(
    "filter[sumber_kurs_default]",
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
    filterStatus, setFilterStatus,
    filterAktif, setFilterAktif,
    filterSumber, setFilterSumber,
    cursor, setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// Delete dialog
// ---------------------------------------------------------------------------

function DeleteDialog({
  kode,
  nama,
  open,
  onOpenChange,
  onConfirm,
}: {
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
          <DialogTitle>Hapus Mata Uang {kode}?</DialogTitle>
          <DialogDescription>
            Mata uang <strong>{nama}</strong> ({kode}) akan dihapus dari sistem (soft-delete).
            Tindakan ini dapat dibatalkan oleh administrator.
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
// Page
// ---------------------------------------------------------------------------

function MataUangListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useMataUangFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<MataUangItem | null>(null);
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
    if (filters.filterAktif !== "")
      p["filter[aktif_flag]"] = filters.filterAktif === "true";
    if (filters.filterSumber) p["filter[sumber_kurs_default]"] = filters.filterSumber;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["mata-uang", queryParams],
    queryFn: () =>
      mataUangApi.list({
        ...queryParams,
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[aktif_flag]":
          filters.filterAktif !== ""
            ? filters.filterAktif === "true"
            : undefined,
        "filter[sumber_kurs_default]": filters.filterSumber || undefined,
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
      void filters.setSort("kode_mata_uang:asc");
    } else {
      void filters.setSort(
        newSorting
          .map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`)
          .join(","),
      );
    }
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // Toggle sort for a column
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
    if (filters.filterAktif !== "") {
      f.push({
        key: "filter[aktif_flag]",
        label: "Aktif",
        value: filters.filterAktif,
        displayValue: filters.filterAktif === "true" ? "Ya" : "Tidak",
      });
    }
    if (filters.filterSumber) {
      f.push({
        key: "filter[sumber_kurs_default]",
        label: "Sumber Kurs",
        value: filters.filterSumber,
        displayValue: filters.filterSumber.replace(/_/g, " "),
      });
    }
    return f;
  }, [filters.filterStatus, filters.filterAktif, filters.filterSumber]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[aktif_flag]") void filters.setFilterAktif("");
    if (key === "filter[sumber_kurs_default]") void filters.setFilterSumber("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterAktif("");
    void filters.setFilterSumber("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: MataUangItem) => {
    try {
      await mataUangApi.delete(item.kodeMataUang, uuidv4());
      notify.success(
        `Mata uang ${item.kodeMataUang} — ${item.namaMataUang} berhasil dihapus.`,
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

  const handleSubmit = async (item: MataUangItem) => {
    try {
      await mataUangApi.submit(item.kodeMataUang, {
        rowVersion: item.rowVersion,
      });
      notify.success(
        `Mata uang ${item.kodeMataUang} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () =>
              router.push(`/master/mata-uang/${item.kodeMataUang}`),
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

  const columns: ColumnDef<MataUangItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "kodeMataUang",
        header: () => (
          <SortHeader
            label="Kode"
            sortKey="kode_mata_uang"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <div className="flex items-center gap-1.5">
            <code className="font-mono font-bold text-sm">
              {row.original.kodeMataUang}
            </code>
            {row.original.isSystemCurrency && (
              <span title="Mata uang fungsional sistem">
                <Shield
                  className="h-3.5 w-3.5 text-muted-foreground"
                  aria-label="Mata uang fungsional sistem"
                />
              </span>
            )}
          </div>
        ),
      },
      {
        accessorKey: "namaMataUang",
        header: () => (
          <SortHeader
            label="Nama"
            sortKey="nama_mata_uang"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span
            className="block max-w-[200px] truncate"
            title={row.original.namaMataUang}
          >
            {row.original.namaMataUang}
          </span>
        ),
      },
      {
        accessorKey: "simbol",
        header: "Simbol",
        cell: ({ row }) => (
          <span className="font-bold text-center block">
            {row.original.simbol}
          </span>
        ),
      },
      {
        accessorKey: "decimalPlaces",
        header: "Des.",
        cell: ({ row }) => (
          <span className="text-center block">{row.original.decimalPlaces}</span>
        ),
      },
      {
        accessorKey: "sumberKursDefault",
        header: "Sumber Kurs",
        cell: ({ row }) => (
          <span className="rounded-md border px-1.5 py-0.5 text-xs">
            {row.original.sumberKursDefault.replace(/_/g, " ")}
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
          const canEdit = perms.canUpdate("mata_uang") && isDraft;
          const canDelete =
            perms.canDelete("mata_uang") &&
            isDraft &&
            !item.isSystemCurrency;
          const canSubmit =
            perms.canSubmit("mata_uang") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk mata uang ${item.kodeMataUang}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/mata-uang/${item.kodeMataUang}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link
                      href={`/master/mata-uang/${item.kodeMataUang}/edit`}
                    >
                      Edit
                    </Link>
                  </DropdownMenuItem>
                )}
                {canSubmit && (
                  <DropdownMenuItem
                    onClick={() => void handleSubmit(item)}
                  >
                    {item.workflowStatus === "RETURNED"
                      ? "Kirim Ulang untuk Review"
                      : "Kirim untuk Review"}
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link
                    href={`/master/mata-uang/${item.kodeMataUang}/history`}
                  >
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

  // Export params (current filters + sort)
  const exportQueryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterAktif !== "")
      p["filter[aktif_flag]"] = filters.filterAktif === "true";
    if (filters.filterSumber) p["filter[sumber_kurs_default]"] = filters.filterSumber;
    return p;
  }, [filters]);

  const exportFilename = `mata-uang-${format(new Date(), "yyyyMMdd")}`;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Header */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Mata Uang</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Mata Uang</h1>

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
        searchPlaceholder="Cari kode, nama, simbol..."
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
              <SelectTrigger className="h-9 w-[180px]" aria-label="Filter status workflow">
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
              <SelectTrigger className="h-9 w-[120px]" aria-label="Filter status aktif">
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
          // Inline handler that triggers export
          const url = mataUangApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof mataUangApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("mata_uang") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/mata-uang/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Mata Uang
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? `Tidak ada mata uang yang cocok dengan pencarian.`
            : "Belum ada mata uang. Klik '+ Tambah Mata Uang' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete confirmation dialog */}
      {deleteTarget && (
        <DeleteDialog
          kode={deleteTarget.kodeMataUang}
          nama={deleteTarget.namaMataUang}
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

export default function MataUangListPage() {
  return (
    <Suspense>
      <MataUangListContent />
    </Suspense>
  );
}
