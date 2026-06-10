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
import { MoreHorizontal, Plus, Info, AlertTriangle } from "lucide-react";
import { useRouter } from "next/navigation";
import { format, parseISO } from "date-fns";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { WorkflowStatusBadge } from "@/components/blips/WorkflowStatusBadge";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
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

import { lpsCoverageApi } from "@/lib/api/lps-coverage.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  formatIDR,
  type LPSCoverageItem,
} from "@/lib/schemas/lps-coverage.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { v4 as uuidv4 } from "uuid";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useLPSCoverageFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("periode_berlaku_dari:desc"),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterActive, setFilterActive] = useQueryState(
    "filter[active]",
    parseAsString.withDefault(""),
  );
  const [filterYear, setFilterYear] = useQueryState(
    "filter[year]",
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
    filterActive, setFilterActive,
    filterYear, setFilterYear,
    cursor, setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// Delete dialog
// ---------------------------------------------------------------------------

function DeleteDialog({
  id,
  coverageAmount,
  open,
  onOpenChange,
  onConfirm,
}: {
  id: string;
  coverageAmount: string;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus LPS Coverage Cap?</DialogTitle>
          <DialogDescription>
            Record coverage <strong>{formatIDR(coverageAmount)}</strong> (ID:{" "}
            {id}) akan dihapus dari sistem (soft-delete). Tindakan ini dapat
            dibatalkan oleh administrator.
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
// Active badge for currently-active coverage (periodeBerlakuSampai = null)
// ---------------------------------------------------------------------------

function ActiveCoverageBadge() {
  return (
    <Badge
      variant="default"
      className="bg-green-100 text-green-800 border-green-300 hover:bg-green-100 text-xs font-semibold"
    >
      AKTIF
    </Badge>
  );
}

// ---------------------------------------------------------------------------
// Year options helper
// ---------------------------------------------------------------------------

function buildYearOptions(): string[] {
  const currentYear = new Date().getFullYear();
  const years: string[] = [];
  for (let y = currentYear + 1; y >= currentYear - 5; y--) {
    years.push(String(y));
  }
  return years;
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function LPSCoverageListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useLPSCoverageFilters();

  const [deleteTarget, setDeleteTarget] =
    React.useState<LPSCoverageItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const yearOptions = React.useMemo(() => buildYearOptions(), []);

  // Build query params
  const queryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterActive !== "")
      p["filter[active]"] = filters.filterActive === "true";
    if (filters.filterYear) p["filter[year]"] = filters.filterYear;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["lps-coverage", queryParams],
    queryFn: () =>
      lpsCoverageApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[active]":
          filters.filterActive !== ""
            ? filters.filterActive === "true"
            : undefined,
        "filter[year]": filters.filterYear || undefined,
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
      void filters.setSort("periode_berlaku_dari:desc");
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
    if (filters.filterActive !== "") {
      f.push({
        key: "filter[active]",
        label: "Periode",
        value: filters.filterActive,
        displayValue: filters.filterActive === "true" ? "Aktif" : "Tidak Aktif",
      });
    }
    if (filters.filterYear) {
      f.push({
        key: "filter[year]",
        label: "Tahun",
        value: filters.filterYear,
        displayValue: filters.filterYear,
      });
    }
    return f;
  }, [filters.filterStatus, filters.filterActive, filters.filterYear]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[active]") void filters.setFilterActive("");
    if (key === "filter[year]") void filters.setFilterYear("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterStatus("");
    void filters.setFilterActive("");
    void filters.setFilterYear("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: LPSCoverageItem) => {
    try {
      await lpsCoverageApi.delete(item.id, uuidv4());
      notify.destructive(
        `LPS Coverage cap ${formatIDR(item.coverageAmount)} berhasil dihapus.`,
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

  const handleSubmit = async (item: LPSCoverageItem) => {
    try {
      await lpsCoverageApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(
        `LPS Coverage ${item.id} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/lps-coverage/${item.id}`),
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
  // Format date
  // ---------------------------------------------------------------------------

  function fmtDate(d: string | null | undefined): string {
    if (!d) return "—";
    try {
      return format(parseISO(d), "dd MMM yyyy");
    } catch {
      return d;
    }
  }

  // ---------------------------------------------------------------------------
  // Columns
  // ---------------------------------------------------------------------------

  const columns: ColumnDef<LPSCoverageItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "coverageAmount",
        header: () => (
          <SortHeader
            label="Coverage Cap"
            sortKey="coverage_amount"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <span className="font-mono font-medium tabular-nums">
              {formatIDR(row.original.coverageAmount)}
            </span>
            {row.original.periodeBerlakuSampai === null &&
              row.original.workflowStatus === "APPROVED" && (
                <ActiveCoverageBadge />
              )}
          </div>
        ),
      },
      {
        accessorKey: "mataUang",
        header: "Mata Uang",
        cell: () => (
          <Badge variant="outline" className="font-mono text-xs">
            IDR
          </Badge>
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
        cell: ({ row }) => (
          <span className="tabular-nums">
            {fmtDate(row.original.periodeBerlakuDari)}
          </span>
        ),
      },
      {
        accessorKey: "periodeBerlakuSampai",
        header: () => (
          <SortHeader
            label="Berlaku Sampai"
            sortKey="periode_berlaku_sampai"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="tabular-nums">
            {row.original.periodeBerlakuSampai
              ? fmtDate(row.original.periodeBerlakuSampai)
              : "—"}
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
                  aria-label={`Aksi untuk LPS coverage ${item.id}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/lps-coverage/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/lps-coverage/${item.id}/edit`}>
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
                  <Link href={`/master/lps-coverage/${item.id}/history`}>
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
    if (filters.filterActive !== "")
      p["filter[active]"] = filters.filterActive === "true";
    if (filters.filterYear) p["filter[year]"] = filters.filterYear;
    return p;
  }, [filters]);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">LPS Coverage</span>
      </nav>

      <h1 className="text-2xl font-semibold">LPS Coverage Cap</h1>

      {/* DEC-014 ECL impact banner */}
      <div
        role="note"
        aria-label="Peringatan parameter ECL"
        className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <div className="text-sm text-amber-800">
          <strong>Parameter ECL — LPS Coverage Cap</strong> mempengaruhi LPS
          Aggregator di ECL Stage 1/2/3 (DEC-014). Coverage cap default IDR 2
          miliar per nasabah per bank sesuai regulasi LPS. Eksposur di atas cap
          dikenakan ECL; di bawah cap ECL = 0 (dijamin LPS).{" "}
          <Link
            href="https://www.lps.go.id"
            target="_blank"
            rel="noopener noreferrer"
            className="underline hover:no-underline"
          >
            Referensi LPS
          </Link>
          .
          <span className="ml-1 inline-flex items-center gap-1">
            <Info className="h-3 w-3" aria-hidden />
            Hanya satu record dengan periode terbuka (Berlaku Sampai kosong)
            yang dapat aktif pada satu waktu.
          </span>
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
        searchPlaceholder="Cari coverage amount, referensi regulasi..."
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

            {/* Active / inactive period filter */}
            <Select
              value={
                filters.filterActive === ""
                  ? "all"
                  : filters.filterActive === "true"
                    ? "true"
                    : "false"
              }
              onValueChange={(v) => {
                void filters.setFilterActive(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[150px]"
                aria-label="Filter periode aktif"
              >
                <SelectValue placeholder="Semua Periode" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Periode</SelectItem>
                <SelectItem value="true">Aktif</SelectItem>
                <SelectItem value="false">Sudah Berakhir</SelectItem>
              </SelectContent>
            </Select>

            {/* Year filter */}
            <Select
              value={filters.filterYear || "all"}
              onValueChange={(v) => {
                void filters.setFilterYear(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[120px]"
                aria-label="Filter tahun berlaku"
              >
                <SelectValue placeholder="Semua Tahun" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tahun</SelectItem>
                {yearOptions.map((y) => (
                  <SelectItem key={y} value={y}>
                    {y}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = lpsCoverageApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof lpsCoverageApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("ecl_parameter") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/lps-coverage/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Coverage Cap
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada LPS coverage yang cocok dengan filter."
            : "Belum ada LPS coverage cap. Klik '+ Tambah Coverage Cap' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete confirmation dialog */}
      {deleteTarget && (
        <DeleteDialog
          id={deleteTarget.id}
          coverageAmount={deleteTarget.coverageAmount}
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

export default function LPSCoverageListPage() {
  return (
    <Suspense>
      <LPSCoverageListContent />
    </Suspense>
  );
}
