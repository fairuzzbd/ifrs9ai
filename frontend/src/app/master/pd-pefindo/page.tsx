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
import { MoreHorizontal, Plus, Upload, AlertTriangle } from "lucide-react";
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

import { pdPefindoApi } from "@/lib/api/pd-pefindo.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { PDPefindoItem } from "@/lib/schemas/pd-pefindo.schema";
import { PEFINDO_RATINGS } from "@/lib/schemas/pd-pefindo.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function usePDPefindoFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("rating:asc"),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterRating, setFilterRating] = useQueryState(
    "filter[rating]",
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
    filterRating, setFilterRating,
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
  item: PDPefindoItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus PD Pefindo — {item.rating}?</DialogTitle>
          <DialogDescription>
            Record PD untuk rating <strong>{item.rating}</strong> (periode{" "}
            {item.periodeBerlakuDari}) akan dihapus dari sistem (soft-delete).
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
// Format PD percent display
// ---------------------------------------------------------------------------

function formatPD(val: string | null): string {
  if (!val) return "—";
  const n = parseFloat(val);
  if (isNaN(n)) return val;
  return `${(n * 100).toFixed(4)}%`;
}

function formatDate(val: string | null): string {
  if (!val) return "—";
  try {
    return format(parseISO(val), "dd MMM yyyy");
  } catch {
    return val;
  }
}

// ---------------------------------------------------------------------------
// DEC-010 warning banner
// ---------------------------------------------------------------------------

function ECLParamBanner() {
  return (
    <div
      role="note"
      aria-label="Peringatan parameter ECL"
      className="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3"
    >
      <AlertTriangle
        className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
        aria-hidden
      />
      <div className="text-sm text-amber-800">
        <span className="font-semibold">Parameter ECL (DEC-010): </span>
        PD Pefindo yang sudah <strong>APPROVED</strong> digunakan langsung oleh
        ECL Engine pada calc run berikutnya. Perubahan pada record yang sudah
        disetujui memerlukan workflow 6-eyes (Risk Officer + Finance Controller +
        2x ALCO approval dengan MFA step-up).
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function PDPefindoListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = usePDPefindoFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<PDPefindoItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const queryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterRating) p["filter[rating]"] = filters.filterRating;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["pd-pefindo", queryParams],
    queryFn: () =>
      pdPefindoApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[rating]": filters.filterRating || undefined,
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
      return { id, desc: dir === "desc" };
    });
  }, [filters.sort]);

  const handleSortingChange = (newSorting: SortingState) => {
    if (newSorting.length === 0) {
      void filters.setSort("rating:asc");
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
    if (filters.filterStatus) {
      const labels: Record<string, string> = {
        DRAFT: "Draf",
        PENDING_REVIEW: "Menunggu Review",
        PENDING_APPROVAL: "Menunggu Approval",
        PENDING_APPROVAL_2: "Menunggu Approval 2",
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
    if (filters.filterRating) {
      f.push({
        key: "filter[rating]",
        label: "Rating",
        value: filters.filterRating,
        displayValue: filters.filterRating,
      });
    }
    return f;
  }, [filters.filterStatus, filters.filterRating]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[rating]") void filters.setFilterRating("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterRating("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // Delete
  const handleDelete = async (item: PDPefindoItem) => {
    try {
      await pdPefindoApi.delete(item.id, uuidv4());
      notify.success(
        `PD Pefindo rating ${item.rating} (periode ${item.periodeBerlakuDari}) berhasil dihapus.`,
      );
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  // Submit
  const handleSubmit = async (item: PDPefindoItem) => {
    try {
      await pdPefindoApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(
        `PD Pefindo ${item.rating} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/pd-pefindo/${item.id}`),
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

  // Columns
  const columns: ColumnDef<PDPefindoItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "rating",
        header: () => (
          <SortHeader
            label="Rating"
            sortKey="rating"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <code className="font-mono font-bold text-sm">
            {row.original.rating}
          </code>
        ),
      },
      {
        accessorKey: "pd12Month",
        header: () => (
          <SortHeader
            label="PD 12 Bulan"
            sortKey="pd_12month"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="font-mono tabular-nums text-right block">
            {formatPD(row.original.pd12Month)}
          </span>
        ),
      },
      {
        accessorKey: "pdLifetime3Y",
        header: "PD 3Y",
        cell: ({ row }) => (
          <span className="font-mono tabular-nums text-right block text-muted-foreground">
            {formatPD(row.original.pdLifetime3Y)}
          </span>
        ),
      },
      {
        accessorKey: "pdLifetime10Y",
        header: "PD 10Y",
        cell: ({ row }) => (
          <span className="font-mono tabular-nums text-right block text-muted-foreground">
            {formatPD(row.original.pdLifetime10Y)}
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
        cell: ({ row }) => formatDate(row.original.periodeBerlakuDari),
      },
      {
        accessorKey: "periodeBerlakuSampai",
        header: "Sampai",
        cell: ({ row }) => formatDate(row.original.periodeBerlakuSampai),
      },
      {
        accessorKey: "tanggalPublikasi",
        header: () => (
          <SortHeader
            label="Tgl Publikasi"
            sortKey="tanggal_publikasi"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => formatDate(row.original.tanggalPublikasi),
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
          <WorkflowStatusBadge
            status={
              row.original.workflowStatus as import("@/lib/schemas/mata-uang.schema").MasterWorkflowState
            }
          />
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
                  aria-label={`Aksi untuk PD Pefindo ${item.rating}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/pd-pefindo/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/pd-pefindo/${item.id}/edit`}>
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
                  <Link href={`/master/pd-pefindo/${item.id}/history`}>
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

  const exportFilename = `pd-pefindo-${format(new Date(), "yyyyMMdd")}`;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">PD Pefindo</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar PD Pefindo</h1>

      {/* ECL Parameter warning (DEC-010) */}
      <ECLParamBanner />

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
        searchPlaceholder="Cari rating, sumber..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            <Select
              value={filters.filterRating || "all"}
              onValueChange={(v) => {
                void filters.setFilterRating(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[160px]"
                aria-label="Filter rating Pefindo"
              >
                <SelectValue placeholder="Semua Rating" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Rating</SelectItem>
                {PEFINDO_RATINGS.map((r) => (
                  <SelectItem key={r} value={r}>
                    {r}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
                <SelectItem value="PENDING_APPROVAL">Menunggu Approval</SelectItem>
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
          if (fmt === "xlsx") {
            notify.info(
              "Export XLSX belum tersedia untuk PD Pefindo. Gunakan format CSV.",
            );
            return;
          }
          const url = pdPefindoApi.exportUrl({
            sort: filters.sort || undefined,
            q: filters.q || undefined,
            "filter[workflow_status]": filters.filterStatus || undefined,
            "filter[rating]": filters.filterRating || undefined,
            format: "csv",
          });
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("ecl_parameter") && !perms.isAuditRole() ? (
            <div className="flex items-center gap-2">
              <Button size="sm" variant="outline" asChild>
                <Link href="/master/pd-pefindo/upload">
                  <Upload className="mr-1.5 h-4 w-4" aria-hidden />
                  Upload XLSX Pefindo
                </Link>
              </Button>
              <Button size="sm" asChild>
                <Link href="/master/pd-pefindo/new">
                  <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                  Buat Manual
                </Link>
              </Button>
            </div>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada PD Pefindo yang cocok dengan pencarian."
            : "Belum ada data PD Pefindo. Klik 'Upload XLSX Pefindo' atau '+ Buat Manual'."
        }
        onRetry={() => void refetch()}
      />

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

export default function PDPefindoListPage() {
  return (
    <Suspense>
      <PDPefindoListContent />
    </Suspense>
  );
}
