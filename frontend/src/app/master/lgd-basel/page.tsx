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
import { MoreHorizontal, Plus, AlertTriangle } from "lucide-react";
import { useRouter } from "next/navigation";
import { format, parseISO } from "date-fns";
import { v4 as uuidv4 } from "uuid";

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

import { lgdBaselApi } from "@/lib/api/lgd-basel.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  TIPE_EKSPOSUR_LABELS,
  lgdDecimalToPercent,
  type LGDBaselItem,
  type TipeEksposur,
} from "@/lib/schemas/lgd-basel.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useLGDBaselFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("created_at:desc"),
  );
  const [filterTipeEksposur, setFilterTipeEksposur] = useQueryState(
    "filter[tipe_eksposur]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterSumber, setFilterSumber] = useQueryState(
    "filter[sumber]",
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
    filterTipeEksposur, setFilterTipeEksposur,
    filterStatus, setFilterStatus,
    filterSumber, setFilterSumber,
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
  item: LGDBaselItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  const label = TIPE_EKSPOSUR_LABELS[item.tipeEksposur] ?? item.tipeEksposur;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus LGD Pool {label}?</DialogTitle>
          <DialogDescription>
            LGD pool untuk <strong>{label}</strong> (LGD:{" "}
            {lgdDecimalToPercent(item.lgd)}%) akan dihapus (soft-delete).
            Jika ada referensi dari kalkulasi ECL, penghapusan akan ditolak.
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
// Helper: format date safely
// ---------------------------------------------------------------------------

function fmtDate(val: string | null | undefined): string {
  if (!val) return "Sekarang";
  try {
    return format(parseISO(val), "dd MMM yyyy");
  } catch {
    return val;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

function LGDBaselListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useLGDBaselFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<LGDBaselItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterTipeEksposur) p["filter[tipe_eksposur]"] = filters.filterTipeEksposur;
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterSumber) p["filter[sumber]"] = filters.filterSumber;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["lgd-basel", queryParams],
    queryFn: () =>
      lgdBaselApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[tipe_eksposur]": filters.filterTipeEksposur || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[sumber]": filters.filterSumber || undefined,
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
      return { id: id ?? "", desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (newSorting: SortingState) => {
    if (newSorting.length === 0) {
      void filters.setSort("created_at:desc");
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
    if (filters.filterTipeEksposur) {
      f.push({
        key: "filter[tipe_eksposur]",
        label: "Tipe Eksposur",
        value: filters.filterTipeEksposur,
        displayValue:
          TIPE_EKSPOSUR_LABELS[filters.filterTipeEksposur as TipeEksposur] ??
          filters.filterTipeEksposur,
      });
    }
    if (filters.filterStatus) {
      const statusLabels: Record<string, string> = {
        DRAFT: "Draf",
        PENDING_REVIEW: "Menunggu Review",
        PENDING_APPROVAL: "Menunggu Approval 1",
        PENDING_APPROVAL_2: "Menunggu Approval 2",
        APPROVED: "Disetujui",
        RETURNED: "Dikembalikan",
      };
      f.push({
        key: "filter[workflow_status]",
        label: "Status",
        value: filters.filterStatus,
        displayValue: statusLabels[filters.filterStatus] ?? filters.filterStatus,
      });
    }
    if (filters.filterSumber) {
      f.push({
        key: "filter[sumber]",
        label: "Sumber",
        value: filters.filterSumber,
        displayValue: filters.filterSumber,
      });
    }
    return f;
  }, [filters.filterTipeEksposur, filters.filterStatus, filters.filterSumber]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[tipe_eksposur]") void filters.setFilterTipeEksposur("");
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[sumber]") void filters.setFilterSumber("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterTipeEksposur("");
    void filters.setFilterStatus("");
    void filters.setFilterSumber("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: LGDBaselItem) => {
    try {
      await lgdBaselApi.softDelete(item.id, uuidv4());
      notify.success(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} berhasil dihapus.`,
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

  const handleSubmit = async (item: LGDBaselItem) => {
    try {
      await lgdBaselApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(
        `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur]} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/lgd-basel/${item.id}`),
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

  const columns: ColumnDef<LGDBaselItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "tipeEksposur",
        header: () => (
          <SortHeader
            label="Tipe Eksposur"
            sortKey="tipe_eksposur"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="font-medium">
            {TIPE_EKSPOSUR_LABELS[row.original.tipeEksposur] ??
              row.original.tipeEksposur}
          </span>
        ),
      },
      {
        accessorKey: "lgd",
        header: () => (
          <SortHeader
            label="LGD (%)"
            sortKey="lgd"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="font-mono text-right block">
            {lgdDecimalToPercent(row.original.lgd)}%
          </span>
        ),
      },
      {
        accessorKey: "periodeBerlakuDari",
        header: () => (
          <SortHeader
            label="Berlaku Dari"
            sortKey="periode_berlaku_dari"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => fmtDate(row.original.periodeBerlakuDari),
      },
      {
        accessorKey: "periodeBerlakuSampai",
        header: "Berlaku Sampai",
        cell: ({ row }) =>
          row.original.periodeBerlakuSampai
            ? fmtDate(row.original.periodeBerlakuSampai)
            : (
              <span className="text-green-700 font-medium text-xs">
                Sekarang
              </span>
            ),
      },
      {
        accessorKey: "sumber",
        header: "Sumber",
        cell: ({ row }) => (
          <span className="rounded border px-1.5 py-0.5 text-xs font-mono">
            {row.original.sumber}
          </span>
        ),
      },
      {
        accessorKey: "workflowStatus",
        header: () => (
          <SortHeader
            label="Status Workflow"
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
        accessorKey: "createdAt",
        header: () => (
          <SortHeader
            label="Dibuat"
            sortKey="created_at"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => {
          try {
            return format(parseISO(row.original.createdAt), "dd MMM yyyy");
          } catch {
            return row.original.createdAt;
          }
        },
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          const isDraft =
            item.workflowStatus === "DRAFT" ||
            item.workflowStatus === "RETURNED";
          const canEdit = perms.canUpdate("ecl_parameter") && isDraft;
          const canDelete = perms.canDelete("ecl_parameter") && isDraft;
          const canSubmit = perms.canSubmit("ecl_parameter") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk LGD pool ${item.tipeEksposur}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/lgd-basel/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/lgd-basel/${item.id}/edit`}>
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
                  <Link href={`/master/lgd-basel/${item.id}/history`}>
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
    if (filters.filterTipeEksposur) p["filter[tipe_eksposur]"] = filters.filterTipeEksposur;
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterSumber) p["filter[sumber]"] = filters.filterSumber;
    return p;
  }, [filters]);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Header */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span>Parameter ECL</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">LGD Basel Pool</span>
      </nav>

      {/* ECL param warning banner */}
      <div
        role="note"
        className="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <div className="text-sm text-amber-800">
          <strong>Parameter ECL</strong> — perubahan LGD akan mempengaruhi semua kalkulasi
          ECL Stage 1/2/3. Workflow 6-eyes: memerlukan persetujuan ALCO dan CFO/Komite.
          Proses approve memerlukan MFA step-up (DEC-027).
        </div>
      </div>

      <h1 className="text-2xl font-semibold">LGD Basel Pool</h1>

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
        searchPlaceholder="Cari tipe eksposur, sumber..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Filter tipe eksposur */}
            <Select
              value={filters.filterTipeEksposur || "all"}
              onValueChange={(v) => {
                void filters.setFilterTipeEksposur(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[180px]"
                aria-label="Filter tipe eksposur"
              >
                <SelectValue placeholder="Semua Tipe" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tipe</SelectItem>
                {(Object.entries(TIPE_EKSPOSUR_LABELS) as [TipeEksposur, string][]).map(
                  ([key, label]) => (
                    <SelectItem key={key} value={key}>
                      {label}
                    </SelectItem>
                  ),
                )}
              </SelectContent>
            </Select>

            {/* Filter status */}
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
                className="h-9 w-[200px]"
                aria-label="Filter status workflow"
              >
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="DRAFT">Draf</SelectItem>
                <SelectItem value="PENDING_REVIEW">Menunggu Review</SelectItem>
                <SelectItem value="PENDING_APPROVAL">
                  Menunggu Approval 1
                </SelectItem>
                <SelectItem value="PENDING_APPROVAL_2">
                  Menunggu Approval 2
                </SelectItem>
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
          const url = lgdBaselApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof lgdBaselApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canSubmit("ecl_parameter") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/lgd-basel/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Buat LGD Pool
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada LGD pool yang cocok dengan pencarian."
            : "Belum ada LGD pool. Klik '+ Buat LGD Pool' untuk mulai."
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

export default function LGDBaselListPage() {
  return (
    <Suspense>
      <LGDBaselListContent />
    </Suspense>
  );
}
