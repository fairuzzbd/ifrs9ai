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
import { MoreHorizontal, Plus, RefreshCw, Info, Lock } from "lucide-react";
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
import { Input } from "@/components/ui/input";

import { kursApi } from "@/lib/api/kurs.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  formatKursTable,
  SUMBER_KURS_LABELS,
  type KursItem,
} from "@/lib/schemas/kurs.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// URL state hook
// ---------------------------------------------------------------------------

function useKursFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("tanggal_berlaku:desc"),
  );
  const [filterKode, setFilterKode] = useQueryState(
    "filter[kode_mata_uang]",
    parseAsString.withDefault(""),
  );
  const [filterSumber, setFilterSumber] = useQueryState(
    "filter[sumber_kurs]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterYear, setFilterYear] = useQueryState(
    "filter[year]",
    parseAsString.withDefault(""),
  );
  const [filterMonth, setFilterMonth] = useQueryState(
    "filter[month]",
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
    filterKode, setFilterKode,
    filterSumber, setFilterSumber,
    filterStatus, setFilterStatus,
    filterYear, setFilterYear,
    filterMonth, setFilterMonth,
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
  item: KursItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Kurs {item.fxRateIdKode}?</DialogTitle>
          <DialogDescription>
            Kurs <strong>{item.kodeMataUang}</strong> tanggal{" "}
            <strong>{item.tanggalBerlaku}</strong> akan dihapus dari sistem
            (soft-delete).
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
// JISDOR sync dialog
// ---------------------------------------------------------------------------

function JisdorSyncDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const today = new Date().toISOString().split("T")[0] as string;
  const [tanggal, setTanggal] = React.useState(today);
  const [syncing, setSyncing] = React.useState(false);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Sync BI JISDOR</DialogTitle>
          <DialogDescription>
            Memulai proses sinkronisasi kurs dari BI JISDOR untuk tanggal
            yang dipilih. Proses berjalan di background (async).
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 py-2">
          <div className="rounded-md border border-amber-200 bg-amber-50 px-4 py-3">
            <p className="text-xs text-amber-800">
              <strong>Info:</strong> BI JISDOR feed adalah fitur Phase 4. Saat ini
              Anda dapat memulai sync manual. Feed otomatis harian akan tersedia
              pada Phase 4.
            </p>
          </div>
          <div className="space-y-1">
            <label className="text-sm font-medium" htmlFor="jisdor-tanggal">
              Tanggal Berlaku
            </label>
            <Input
              id="jisdor-tanggal"
              type="date"
              value={tanggal}
              max={today}
              onChange={(e) => setTanggal(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={syncing}
          >
            Batal
          </Button>
          <Button
            disabled={syncing || !tanggal}
            onClick={async () => {
              setSyncing(true);
              try {
                const res = await kursApi.jisdorSync(tanggal, uuidv4());
                notify.success(
                  `Sync JISDOR dimulai. Job ID: ${res.data.jobId.slice(0, 8)}... — ${res.data.message}`,
                  {
                    action: {
                      label: "Lihat status",
                      onClick: () =>
                        window.open(res.data.statusUrl, "_blank"),
                    },
                  },
                );
                onOpenChange(false);
              } catch (err) {
                if (err && typeof err === "object" && "code" in err) {
                  notify.error(
                    err as { code: string; message: string; traceId: string },
                  );
                }
              } finally {
                setSyncing(false);
              }
            }}
          >
            {syncing ? "Memproses..." : "Mulai Sync"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function KursListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useKursFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<KursItem | null>(null);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [jisdorOpen, setJisdorOpen] = React.useState(false);

  // Build query params
  const queryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      limit: filters.limit,
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterKode) p["filter[kode_mata_uang]"] = filters.filterKode;
    if (filters.filterSumber) p["filter[sumber_kurs]"] = filters.filterSumber;
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterYear) p["filter[year]"] = filters.filterYear;
    if (filters.filterMonth) p["filter[month]"] = filters.filterMonth;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["kurs", queryParams],
    queryFn: () =>
      kursApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[kode_mata_uang]": filters.filterKode || undefined,
        "filter[sumber_kurs]": filters.filterSumber || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[year]": filters.filterYear || undefined,
        "filter[month]": filters.filterMonth || undefined,
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
      void filters.setSort("tanggal_berlaku:desc");
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
  // Active filter chips
  // ---------------------------------------------------------------------------

  const WORKFLOW_LABELS: Record<string, string> = {
    DRAFT: "Draf",
    PENDING_REVIEW: "Menunggu Review",
    PENDING_APPROVAL: "Menunggu Approval",
    APPROVED: "Disetujui",
    RETURNED: "Dikembalikan",
  };

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterKode)
      f.push({ key: "filter[kode_mata_uang]", label: "Mata Uang", value: filters.filterKode, displayValue: filters.filterKode });
    if (filters.filterSumber)
      f.push({ key: "filter[sumber_kurs]", label: "Sumber Kurs", value: filters.filterSumber, displayValue: SUMBER_KURS_LABELS[filters.filterSumber] ?? filters.filterSumber });
    if (filters.filterStatus)
      f.push({ key: "filter[workflow_status]", label: "Status", value: filters.filterStatus, displayValue: WORKFLOW_LABELS[filters.filterStatus] ?? filters.filterStatus });
    if (filters.filterYear)
      f.push({ key: "filter[year]", label: "Tahun", value: filters.filterYear, displayValue: filters.filterYear });
    if (filters.filterMonth)
      f.push({ key: "filter[month]", label: "Bulan", value: filters.filterMonth, displayValue: filters.filterMonth });
    return f;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.filterKode, filters.filterSumber, filters.filterStatus, filters.filterYear, filters.filterMonth]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[kode_mata_uang]") void filters.setFilterKode("");
    if (key === "filter[sumber_kurs]") void filters.setFilterSumber("");
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[year]") void filters.setFilterYear("");
    if (key === "filter[month]") void filters.setFilterMonth("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterKode("");
    void filters.setFilterSumber("");
    void filters.setFilterStatus("");
    void filters.setFilterYear("");
    void filters.setFilterMonth("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete / submit
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: KursItem) => {
    try {
      await kursApi.delete(item.id, uuidv4());
      notify.success(`Kurs ${item.fxRateIdKode} berhasil dihapus.`);
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(err as { code: string; message: string; traceId: string });
      }
    }
  };

  const handleSubmit = async (item: KursItem) => {
    try {
      await kursApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(`Kurs ${item.fxRateIdKode} berhasil disubmit untuk review.`, {
        action: {
          label: "Lihat detail",
          onClick: () => router.push(`/master/kurs/${item.id}`),
        },
      });
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

  const columns: ColumnDef<KursItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "fxRateIdKode",
        header: () => (
          <SortHeader label="ID Kurs" sortKey="fx_rate_id_kode" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <code className="font-mono text-xs">{row.original.fxRateIdKode}</code>
        ),
      },
      {
        accessorKey: "kodeMataUang",
        header: () => (
          <SortHeader label="Mata Uang" sortKey="kode_mata_uang" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <code className="font-mono font-bold">{row.original.kodeMataUang}</code>
        ),
      },
      {
        accessorKey: "tanggalBerlaku",
        header: () => (
          <SortHeader label="Tgl Berlaku" sortKey="tanggal_berlaku" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => row.original.tanggalBerlaku,
      },
      {
        accessorKey: "kursTengah",
        header: () => (
          <SortHeader label="Kurs Tengah" sortKey="kurs_tengah" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="font-mono text-right block">
            {formatKursTable(row.original.kursTengah)}
          </span>
        ),
      },
      {
        accessorKey: "kursBeli",
        header: "Kurs Beli",
        cell: ({ row }) => (
          <span className="font-mono text-right block text-muted-foreground">
            {formatKursTable(row.original.kursBeli) }
          </span>
        ),
      },
      {
        accessorKey: "kursJual",
        header: "Kurs Jual",
        cell: ({ row }) => (
          <span className="font-mono text-right block text-muted-foreground">
            {formatKursTable(row.original.kursJual)}
          </span>
        ),
      },
      {
        accessorKey: "sumberKurs",
        header: () => (
          <SortHeader label="Sumber" sortKey="sumber_kurs" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <span className="rounded-md border px-1.5 py-0.5 text-xs">
            {SUMBER_KURS_LABELS[row.original.sumberKurs] ?? row.original.sumberKurs}
          </span>
        ),
      },
      {
        accessorKey: "lockedFlag",
        header: "Terkunci",
        cell: ({ row }) =>
          row.original.lockedFlag ? (
            <span className="flex items-center gap-1 text-xs text-slate-600">
              <Lock className="h-3 w-3" aria-hidden />
              Ya
            </span>
          ) : (
            <span className="text-xs text-muted-foreground">Tidak</span>
          ),
      },
      {
        accessorKey: "workflowStatus",
        header: () => (
          <SortHeader label="Status" sortKey="workflow_status" sorting={sortingState} onToggle={toggleSort} />
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
          const isDraft = item.workflowStatus === "DRAFT" || item.workflowStatus === "RETURNED";
          const canEdit = perms.canUpdate("kurs") && isDraft && !item.lockedFlag;
          const canDelete = perms.canDelete("kurs") && isDraft && !item.lockedFlag;
          const canSubmit = perms.canSubmit("kurs") && isDraft && !item.lockedFlag;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk kurs ${item.fxRateIdKode}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/kurs/${item.id}`}>Lihat Detail</Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/kurs/${item.id}/edit`}>Edit</Link>
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
                  <Link href={`/master/kurs/${item.id}/history`}>Riwayat Audit</Link>
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

  // Year options (current year ± 2)
  const currentYear = new Date().getFullYear();
  const yearOptions = Array.from({ length: 5 }, (_, i) => (currentYear - 2 + i).toString());

  const exportFilename = `kurs-${format(new Date(), "yyyyMMdd")}`;

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Kurs</span>
      </nav>

      <div className="flex flex-wrap items-start justify-between gap-3">
        <h1 className="text-2xl font-semibold">Daftar Kurs</h1>
        {/* JISDOR sync button — only for users with jisdor_sync permission */}
        {perms.can("kurs.jisdor_sync") && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setJisdorOpen(true)}
          >
            <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden />
            Sync JISDOR
          </Button>
        )}
      </div>

      {/* Phase banner */}
      <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-blue-600" aria-hidden />
        <p>
          <strong>BI JISDOR feed Phase 4</strong> — Feed otomatis harian belum
          tersedia. Gunakan &ldquo;Sync JISDOR&rdquo; untuk tarik data manual, atau
          tambah kurs secara manual melalui tombol &ldquo;Tambah Kurs&rdquo;.
        </p>
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
        searchPlaceholder="Cari kode mata uang, ID kurs..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Kode mata uang filter */}
            <Input
              className="h-9 w-[100px]"
              placeholder="Mata uang"
              value={filters.filterKode}
              onChange={(e) => {
                void filters.setFilterKode(e.target.value.toUpperCase());
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
              aria-label="Filter kode mata uang"
              maxLength={3}
            />
            {/* Sumber kurs */}
            <Select
              value={filters.filterSumber || "all"}
              onValueChange={(v) => {
                void filters.setFilterSumber(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[160px]" aria-label="Filter sumber kurs">
                <SelectValue placeholder="Semua Sumber" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Sumber</SelectItem>
                <SelectItem value="BI_JISDOR">BI JISDOR</SelectItem>
                <SelectItem value="BI_KURS_TENGAH">BI Kurs Tengah</SelectItem>
                <SelectItem value="INTERNAL">Internal</SelectItem>
                <SelectItem value="MANUAL">Manual</SelectItem>
              </SelectContent>
            </Select>
            {/* Workflow status */}
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
            {/* Year */}
            <Select
              value={filters.filterYear || "all"}
              onValueChange={(v) => {
                void filters.setFilterYear(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[110px]" aria-label="Filter tahun">
                <SelectValue placeholder="Tahun" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tahun</SelectItem>
                {yearOptions.map((y) => (
                  <SelectItem key={y} value={y}>{y}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {/* Month */}
            <Select
              value={filters.filterMonth || "all"}
              onValueChange={(v) => {
                void filters.setFilterMonth(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger className="h-9 w-[120px]" aria-label="Filter bulan">
                <SelectValue placeholder="Bulan" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Bulan</SelectItem>
                {["01","02","03","04","05","06","07","08","09","10","11","12"].map((m) => (
                  <SelectItem key={m} value={m}>{m}</SelectItem>
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
          const url = kursApi.exportUrl({
            sort: filters.sort || undefined,
            q: filters.q || undefined,
            "filter[kode_mata_uang]": filters.filterKode || undefined,
            "filter[sumber_kurs]": filters.filterSumber || undefined,
            "filter[workflow_status]": filters.filterStatus || undefined,
            "filter[year]": filters.filterYear || undefined,
            "filter[month]": filters.filterMonth || undefined,
            format: fmt,
          });
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canCreate("kurs") && !perms.isAuditRole() ? (
            <Button size="sm" asChild>
              <Link href="/master/kurs/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                Tambah Kurs
              </Link>
            </Button>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada kurs yang cocok dengan pencarian."
            : "Belum ada kurs. Klik '+ Tambah Kurs' atau gunakan 'Sync JISDOR'."
        }
        onRetry={() => void refetch()}
      />

      {/* Delete dialog */}
      {deleteTarget && (
        <DeleteDialog
          item={deleteTarget}
          open={!!deleteTarget}
          onOpenChange={(v) => { if (!v) setDeleteTarget(null); }}
          onConfirm={() => handleDelete(deleteTarget)}
        />
      )}

      {/* JISDOR sync dialog */}
      <JisdorSyncDialog open={jisdorOpen} onOpenChange={setJisdorOpen} />
    </div>
  );
}

export default function KursListPage() {
  return (
    <Suspense>
      <KursListContent />
    </Suspense>
  );
}
