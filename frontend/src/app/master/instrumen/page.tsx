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

import { instrumenApi } from "@/lib/api/instrumen.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { InstrumenItem } from "@/lib/schemas/instrumen.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { v4 as uuidv4 } from "uuid";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// IDR formatter
// ---------------------------------------------------------------------------

const idr = (s: string) => {
  const n = parseFloat(s);
  if (isNaN(n)) return s;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    maximumFractionDigits: 0,
  }).format(n);
};

// ---------------------------------------------------------------------------
// Tipe badge colors
// ---------------------------------------------------------------------------

const TIPE_COLORS: Record<string, string> = {
  DEPOSITO: "bg-blue-50 text-blue-700 border-blue-200",
  OBLIGASI: "bg-violet-50 text-violet-700 border-violet-200",
  SAHAM: "bg-green-50 text-green-700 border-green-200",
  REKSADANA: "bg-teal-50 text-teal-700 border-teal-200",
  SBN: "bg-orange-50 text-orange-700 border-orange-200",
  SPN: "bg-amber-50 text-amber-700 border-amber-200",
  SUKUK: "bg-rose-50 text-rose-700 border-rose-200",
};

function TipeBadge({ tipe }: { tipe: string }) {
  return (
    <span
      className={cn(
        "inline-flex rounded-full border px-2 py-0.5 text-xs font-medium",
        TIPE_COLORS[tipe] ?? "bg-muted text-muted-foreground border-border",
      )}
    >
      {tipe}
    </span>
  );
}

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useInstrumenFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("created_at:desc"),
  );
  const [filterTipe, setFilterTipe] = useQueryState(
    "filter[tipe_instrumen]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[status]",
    parseAsString.withDefault(""),
  );
  const [filterWorkflow, setFilterWorkflow] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterKlasifikasi, setFilterKlasifikasi] = useQueryState(
    "filter[klasifikasi_psak71]",
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
    filterStatus, setFilterStatus,
    filterWorkflow, setFilterWorkflow,
    filterKlasifikasi, setFilterKlasifikasi,
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
  item: InstrumenItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Instrumen {item.kodeInstrumen}?</DialogTitle>
          <DialogDescription>
            Instrumen <strong>{item.nama}</strong> ({item.kodeInstrumen}) akan
            dihapus dari sistem (soft-delete). Tindakan ini dapat dibatalkan
            oleh administrator.
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
// Page content
// ---------------------------------------------------------------------------

function InstrumenListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useInstrumenFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<InstrumenItem | null>(
    null,
  );
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
    if (filters.filterTipe) p["filter[tipe_instrumen]"] = filters.filterTipe;
    if (filters.filterStatus) p["filter[status]"] = filters.filterStatus;
    if (filters.filterWorkflow)
      p["filter[workflow_status]"] = filters.filterWorkflow;
    if (filters.filterKlasifikasi)
      p["filter[klasifikasi_psak71]"] = filters.filterKlasifikasi;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["instrumen", queryParams],
    queryFn: () =>
      instrumenApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[tipe_instrumen]": filters.filterTipe || undefined,
        "filter[status]": filters.filterStatus || undefined,
        "filter[workflow_status]": filters.filterWorkflow || undefined,
        "filter[klasifikasi_psak71]": filters.filterKlasifikasi || undefined,
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
    if (!current) handleSortingChange([{ id: colId, desc: false }]);
    else if (!current.desc) handleSortingChange([{ id: colId, desc: true }]);
    else handleSortingChange([]);
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
        key: "filter[tipe_instrumen]",
        label: "Tipe",
        value: filters.filterTipe,
        displayValue: filters.filterTipe,
      });
    }
    if (filters.filterWorkflow) {
      const labels: Record<string, string> = {
        DRAFT: "Draf",
        PENDING_REVIEW: "Menunggu Review",
        PENDING_APPROVAL: "Menunggu Approval",
        APPROVED: "Disetujui",
        RETURNED: "Dikembalikan",
      };
      f.push({
        key: "filter[workflow_status]",
        label: "Workflow",
        value: filters.filterWorkflow,
        displayValue: labels[filters.filterWorkflow] ?? filters.filterWorkflow,
      });
    }
    if (filters.filterStatus) {
      f.push({
        key: "filter[status]",
        label: "Status",
        value: filters.filterStatus,
        displayValue: filters.filterStatus.replace(/_/g, " "),
      });
    }
    if (filters.filterKlasifikasi) {
      f.push({
        key: "filter[klasifikasi_psak71]",
        label: "Klasifikasi",
        value: filters.filterKlasifikasi,
        displayValue: filters.filterKlasifikasi,
      });
    }
    return f;
  }, [
    filters.filterTipe,
    filters.filterWorkflow,
    filters.filterStatus,
    filters.filterKlasifikasi,
  ]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[tipe_instrumen]") void filters.setFilterTipe("");
    if (key === "filter[workflow_status]") void filters.setFilterWorkflow("");
    if (key === "filter[status]") void filters.setFilterStatus("");
    if (key === "filter[klasifikasi_psak71]")
      void filters.setFilterKlasifikasi("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterTipe("");
    void filters.setFilterWorkflow("");
    void filters.setFilterStatus("");
    void filters.setFilterKlasifikasi("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: InstrumenItem) => {
    try {
      await instrumenApi.delete(item.id, uuidv4());
      notify.success(
        `Instrumen ${item.kodeInstrumen} — ${item.nama} berhasil dihapus.`,
      );
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  const handleSubmit = async (item: InstrumenItem) => {
    try {
      await instrumenApi.submit(
        item.id,
        { rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Instrumen ${item.kodeInstrumen} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/instrumen/${item.id}`),
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

  const columns: ColumnDef<InstrumenItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "kodeInstrumen",
        header: () => (
          <SortHeader
            label="Kode"
            sortKey="kode_instrumen"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <code className="font-mono font-bold text-sm">
            {row.original.kodeInstrumen}
          </code>
        ),
      },
      {
        accessorKey: "tipeInstrumen",
        header: () => (
          <SortHeader
            label="Tipe"
            sortKey="tipe_instrumen"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => <TipeBadge tipe={row.original.tipeInstrumen} />,
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
        accessorKey: "mataUang",
        header: "Mata Uang",
        cell: ({ row }) => (
          <span className="font-mono text-sm">{row.original.mataUang}</span>
        ),
      },
      {
        accessorKey: "nominal",
        header: () => (
          <SortHeader
            label="Nominal"
            sortKey="nominal"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="block text-right font-mono text-sm tabular-nums">
            {idr(row.original.nominal)}
          </span>
        ),
      },
      {
        accessorKey: "klasifikasiPsak71",
        header: "Klasifikasi",
        cell: ({ row }) => {
          const k = row.original.klasifikasiPsak71;
          if (!k) {
            return (
              <span className="text-xs text-muted-foreground italic">
                Belum ditetapkan
              </span>
            );
          }
          const colorMap: Record<string, string> = {
            AC: "bg-sky-50 text-sky-700 border-sky-200",
            FVOCI: "bg-indigo-50 text-indigo-700 border-indigo-200",
            FVOCI_ELECTION: "bg-purple-50 text-purple-700 border-purple-200",
            FVTPL: "bg-rose-50 text-rose-700 border-rose-200",
          };
          return (
            <span
              className={cn(
                "inline-flex rounded-full border px-2 py-0.5 text-xs font-medium",
                colorMap[k] ?? "bg-muted border-border text-muted-foreground",
              )}
            >
              {k}
            </span>
          );
        },
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
        accessorKey: "tanggalPenempatan",
        header: () => (
          <SortHeader
            label="Tgl Penempatan"
            sortKey="tanggal_penempatan"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {row.original.tanggalPenempatan}
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
          const canEdit = perms.canUpdate("instrumen") && isDraft;
          const canDelete = perms.canDelete("instrumen") && isDraft;
          const canSubmit = perms.canSubmit("instrumen") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk instrumen ${item.kodeInstrumen}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/instrumen/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/instrumen/${item.id}/edit`}>
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
                  <Link href={`/master/instrumen/${item.id}/history`}>
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
    if (filters.filterTipe) p["filter[tipe_instrumen]"] = filters.filterTipe;
    if (filters.filterWorkflow)
      p["filter[workflow_status]"] = filters.filterWorkflow;
    if (filters.filterStatus) p["filter[status]"] = filters.filterStatus;
    if (filters.filterKlasifikasi)
      p["filter[klasifikasi_psak71]"] = filters.filterKlasifikasi;
    return p;
  }, [filters]);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Header */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Instrumen</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Instrumen</h1>

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
        searchPlaceholder="Cari kode, nama, ISIN..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Filter Tipe */}
            <Select
              value={filters.filterTipe || "all"}
              onValueChange={(v) => {
                void filters.setFilterTipe(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[160px]"
                aria-label="Filter tipe instrumen"
              >
                <SelectValue placeholder="Semua Tipe" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tipe</SelectItem>
                <SelectItem value="DEPOSITO">Deposito</SelectItem>
                <SelectItem value="OBLIGASI">Obligasi</SelectItem>
                <SelectItem value="SAHAM">Saham</SelectItem>
                <SelectItem value="REKSADANA">Reksa Dana</SelectItem>
                <SelectItem value="SBN">SBN</SelectItem>
                <SelectItem value="SPN">SPN</SelectItem>
                <SelectItem value="SUKUK">Sukuk</SelectItem>
              </SelectContent>
            </Select>

            {/* Filter Workflow Status */}
            <Select
              value={filters.filterWorkflow || "all"}
              onValueChange={(v) => {
                void filters.setFilterWorkflow(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[190px]"
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

            {/* Filter Klasifikasi */}
            <Select
              value={filters.filterKlasifikasi || "all"}
              onValueChange={(v) => {
                void filters.setFilterKlasifikasi(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[170px]"
                aria-label="Filter klasifikasi PSAK 71"
              >
                <SelectValue placeholder="Semua Klasifikasi" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Klasifikasi</SelectItem>
                <SelectItem value="AC">AC</SelectItem>
                <SelectItem value="FVOCI">FVOCI Debt</SelectItem>
                <SelectItem value="FVOCI_ELECTION">FVOCI Election</SelectItem>
                <SelectItem value="FVTPL">FVTPL</SelectItem>
              </SelectContent>
            </Select>
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = instrumenApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof instrumenApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("instrumen") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/instrumen/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Instrumen
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada instrumen yang cocok dengan pencarian."
            : "Belum ada instrumen. Klik '+ Tambah Instrumen' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete dialog */}
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

export default function InstrumenListPage() {
  return (
    <Suspense>
      <InstrumenListContent />
    </Suspense>
  );
}
