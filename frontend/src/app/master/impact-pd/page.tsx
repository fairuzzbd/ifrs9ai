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
import { MoreHorizontal, Plus, Info } from "lucide-react";
import { useRouter } from "next/navigation";
import { format } from "date-fns";
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

import { impactPdApi } from "@/lib/api/impact-pd.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { ImpactPdItem } from "@/lib/schemas/impact-pd.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useImpactPdFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("created_at:desc"));
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));

  return {
    q, setQ,
    sort, setSort,
    filterStatus, setFilterStatus,
    cursor, setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// Delete dialog
// ---------------------------------------------------------------------------

function DeleteDialog({
  id,
  open,
  onOpenChange,
  onConfirm,
}: {
  id: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Impact PD?</DialogTitle>
          <DialogDescription>
            Record <code className="font-mono">{id.slice(0, 8)}&hellip;</code> akan dihapus (soft-delete).
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={deleting}>
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
// Multiplier badge (color-coded by value)
// ---------------------------------------------------------------------------

function MultiplierBadge({ value }: { value: string }) {
  const n = Number(value);
  return (
    <span
      className={cn(
        "font-mono tabular-nums font-semibold",
        n < 1 ? "text-green-700" : n > 1 ? "text-red-700" : "text-muted-foreground",
      )}
      title={n < 1 ? "PD berkurang" : n > 1 ? "PD meningkat" : "Netral"}
    >
      {value}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function ImpactPdListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useImpactPdFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<ImpactPdItem | null>(null);
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
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["impact-pd", queryParams],
    queryFn: () =>
      impactPdApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
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
      void filters.setSort("created_at:desc");
    } else {
      void filters.setSort(newSorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","));
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
        REJECTED: "Ditolak",
      };
      f.push({
        key: "filter[workflow_status]",
        label: "Status",
        value: filters.filterStatus,
        displayValue: labels[filters.filterStatus] ?? filters.filterStatus,
      });
    }
    return f;
  }, [filters.filterStatus]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // Delete
  const handleDelete = async (item: ImpactPdItem) => {
    try {
      await impactPdApi.delete(item.id, uuidv4());
      notify.destructive(`Impact PD berhasil dihapus.`);
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  // Submit
  const handleSubmit = async (item: ImpactPdItem) => {
    try {
      await impactPdApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(
        `Impact PD berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/impact-pd/${item.id}`),
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
  const columns: ColumnDef<ImpactPdItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "periodeId",
        header: "Periode ID",
        cell: ({ row }) => (
          <code className="font-mono text-xs">{row.original.periodeId.slice(0, 8)}&hellip;</code>
        ),
      },
      {
        accessorKey: "impactMultiplier",
        header: () => (
          <SortHeader
            label="Multiplier"
            sortKey="impact_multiplier"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => <MultiplierBadge value={row.original.impactMultiplier} />,
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
        cell: ({ row }) => <WorkflowStatusBadge status={row.original.workflowStatus as string} />,
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
            return (
              <span className="text-xs text-muted-foreground">
                {format(new Date(row.original.createdAt), "dd MMM yyyy")}
              </span>
            );
          } catch {
            return (
              <span className="text-xs text-muted-foreground">{row.original.createdAt}</span>
            );
          }
        },
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          const isEditable =
            item.workflowStatus === "DRAFT" || item.workflowStatus === "REJECTED";
          const canEdit = perms.canUpdate("ecl_parameter") && isEditable;
          const canDelete = perms.canDelete("ecl_parameter") && isEditable;
          const canSubmit = perms.canSubmit("ecl_parameter") && isEditable;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk Impact PD periode ${item.periodeId.slice(0, 8)}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/impact-pd/${item.id}`}>Lihat Detail</Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/impact-pd/${item.id}/edit`}>Edit</Link>
                  </DropdownMenuItem>
                )}
                {canSubmit && (
                  <DropdownMenuItem onClick={() => void handleSubmit(item)}>
                    {item.workflowStatus === "REJECTED"
                      ? "Kirim Ulang untuk Review"
                      : "Kirim untuk Review"}
                  </DropdownMenuItem>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem asChild>
                  <Link href={`/master/impact-pd/${item.id}/history`}>Riwayat Audit</Link>
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

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Impact PD</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Impact PD</h1>

      {/* DEC-010 Banner */}
      <div
        className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-900"
        role="note"
        aria-label="Informasi kebijakan DEC-010"
      >
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-blue-600" aria-hidden />
        <div>
          <span className="font-semibold">DEC-010 — ECL Forward-Looking PD Multiplier:</span>{" "}
          Tabel ini menyimpan 1 multiplier PD global per periode buku (UNIQUE). Range DB: 0.5–2.0.
          Default 1.0 = tidak ada penyesuaian. Nilai &lt; 1.0 mengurangi PD (kondisi membaik),
          &gt; 1.0 meningkatkan PD (kondisi memburuk). Perubahan memerlukan 6-eyes approval
          dengan step-up MFA dari ROLE-ALCO.
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
        searchPlaceholder="Cari catatan..."
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
              <SelectTrigger className="h-9 w-[200px]" aria-label="Filter status workflow">
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="DRAFT">Draf</SelectItem>
                <SelectItem value="PENDING_REVIEW">Menunggu Review</SelectItem>
                <SelectItem value="PENDING_APPROVAL">Menunggu Approval</SelectItem>
                <SelectItem value="PENDING_APPROVAL_2">Menunggu Approval 2</SelectItem>
                <SelectItem value="APPROVED">Disetujui</SelectItem>
                <SelectItem value="REJECTED">Ditolak</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = impactPdApi.exportUrl({
            sort: filters.sort || undefined,
            q: filters.q || undefined,
            "filter[workflow_status]": filters.filterStatus || undefined,
            format: fmt,
          });
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("ecl_parameter") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/impact-pd/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Impact PD
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada data yang cocok dengan filter."
            : "Belum ada Impact PD. Klik '+ Tambah' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete confirmation dialog */}
      {deleteTarget && (
        <DeleteDialog
          id={deleteTarget.id}
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

export default function ImpactPdListPage() {
  return (
    <Suspense>
      <ImpactPdListContent />
    </Suspense>
  );
}
