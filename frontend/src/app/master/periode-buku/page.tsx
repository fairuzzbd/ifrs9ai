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
import {
  MoreHorizontal,
  Plus,
  Sparkles,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { format, parseISO } from "date-fns";
import { v4 as uuidv4 } from "uuid";

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
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { periodeBukuApi } from "@/lib/api/periode-buku.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import type { PeriodeBukuItem, TipePeriode, StatusPeriode } from "@/lib/schemas/periode-buku.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Status period badge
// ---------------------------------------------------------------------------

function StatusPeriodeBadge({ status }: { status: StatusPeriode }) {
  const config: Record<StatusPeriode, { label: string; className: string }> = {
    OPEN: {
      label: "Buka",
      className: "bg-green-50 text-green-700 border-green-200",
    },
    SOFT_CLOSED: {
      label: "Soft-Close",
      className: "bg-amber-50 text-amber-700 border-amber-200",
    },
    CLOSED: {
      label: "Ditutup",
      className: "bg-red-50 text-red-700 border-red-200",
    },
  };
  const c = config[status] ?? {
    label: status,
    className: "bg-muted text-muted-foreground",
  };
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium",
        c.className,
      )}
      aria-label={`Status periode: ${c.label}`}
    >
      {c.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Tipe period badge
// ---------------------------------------------------------------------------

function TipePeriodeBadge({ tipe }: { tipe: TipePeriode }) {
  const config: Record<TipePeriode, { label: string; className: string }> = {
    BULANAN: {
      label: "Bulanan",
      className: "bg-blue-50 text-blue-700 border-blue-200",
    },
    TRIWULANAN: {
      label: "Triwulanan",
      className: "bg-purple-50 text-purple-700 border-purple-200",
    },
    TAHUNAN: {
      label: "Tahunan",
      className: "bg-slate-50 text-slate-700 border-slate-200",
    },
  };
  const c = config[tipe] ?? { label: tipe, className: "" };
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium",
        c.className,
      )}
    >
      {c.label}
    </span>
  );
}

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function usePeriodeBukuFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("created_at:desc"),
  );
  const [filterTipe, setFilterTipe] = useQueryState(
    "filter[tipe_periode]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[status_periode]",
    parseAsString.withDefault(""),
  );
  const [filterTahun, setFilterTahun] = useQueryState(
    "filter[tahun_buku]",
    parseAsString.withDefault(""),
  );
  const [filterWorkflow, setFilterWorkflow] = useQueryState(
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
    filterStatus, setFilterStatus,
    filterTahun, setFilterTahun,
    filterWorkflow, setFilterWorkflow,
    cursor, setCursor,
    limit,
  };
}

// ---------------------------------------------------------------------------
// Generate dialog
// ---------------------------------------------------------------------------

interface GenerateDialogProps {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onSuccess: () => void;
}

function GenerateDialog({
  open,
  onOpenChange,
  onSuccess,
}: GenerateDialogProps) {
  const [tahun, setTahun] = React.useState<number>(new Date().getFullYear());
  const [selectedTipe, setSelectedTipe] = React.useState<TipePeriode[]>([
    "BULANAN",
    "TRIWULANAN",
    "TAHUNAN",
  ]);
  const [generating, setGenerating] = React.useState(false);

  const toggleTipe = (tipe: TipePeriode) => {
    setSelectedTipe((prev) =>
      prev.includes(tipe) ? prev.filter((t) => t !== tipe) : [...prev, tipe],
    );
  };

  const handleGenerate = async () => {
    if (selectedTipe.length === 0) {
      notify.error({
        code: "VALIDATION_FAILED",
        message: "Pilih minimal satu tipe periode.",
        traceId: "",
      });
      return;
    }
    if (tahun < 2000 || tahun > 2099) {
      notify.error({
        code: "VALIDATION_FAILED",
        message: "Tahun buku harus antara 2000–2099.",
        traceId: "",
      });
      return;
    }
    setGenerating(true);
    try {
      const res = await periodeBukuApi.generate(
        { tahunBuku: tahun, tipe: selectedTipe },
        uuidv4(),
      );
      const { generated, skipped } = res.data;
      notify.success(
        `Generate periode ${tahun} selesai. Dibuat: ${generated} periode, Dilewati (sudah ada): ${skipped}.`,
      );
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(
          err as { code: string; message: string; traceId: string },
        );
      } else {
        notify.error({
          code: "INTERNAL",
          message: "Generate gagal. Coba lagi.",
          traceId: "",
        });
      }
    } finally {
      setGenerating(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Generate Periode Buku</DialogTitle>
          <DialogDescription>
            Generate otomatis semua periode untuk satu tahun buku. Periode yang
            sudah ada akan dilewati (idempotent).
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {/* Tahun Buku */}
          <div className="space-y-2">
            <Label htmlFor="gen-tahun">
              Tahun Buku{" "}
              <span className="text-destructive" aria-hidden>
                *
              </span>
            </Label>
            <Input
              id="gen-tahun"
              type="number"
              min={2000}
              max={2099}
              value={tahun}
              onChange={(e) => setTahun(parseInt(e.target.value, 10))}
              aria-required="true"
            />
          </div>

          {/* Tipe checkboxes */}
          <div className="space-y-2">
            <Label>
              Tipe Periode{" "}
              <span className="text-destructive" aria-hidden>
                *
              </span>
            </Label>
            <div className="space-y-2">
              {(["BULANAN", "TRIWULANAN", "TAHUNAN"] as TipePeriode[]).map(
                (tipe) => {
                  const labels: Record<TipePeriode, string> = {
                    BULANAN: "Bulanan (12 periode)",
                    TRIWULANAN: "Triwulanan (4 periode)",
                    TAHUNAN: "Tahunan (1 periode)",
                  };
                  return (
                    <div key={tipe} className="flex items-center gap-3">
                      <Checkbox
                        id={`gen-tipe-${tipe}`}
                        checked={selectedTipe.includes(tipe)}
                        onCheckedChange={() => toggleTipe(tipe)}
                        aria-describedby={`gen-tipe-${tipe}-label`}
                      />
                      <Label
                        id={`gen-tipe-${tipe}-label`}
                        htmlFor={`gen-tipe-${tipe}`}
                        className="cursor-pointer font-normal"
                      >
                        {labels[tipe]}
                      </Label>
                    </div>
                  );
                },
              )}
            </div>
          </div>

          {/* Preview count */}
          <div className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">
            Estimasi periode yang akan dibuat:{" "}
            <strong>
              {selectedTipe.reduce((acc, t) => {
                if (t === "BULANAN") return acc + 12;
                if (t === "TRIWULANAN") return acc + 4;
                if (t === "TAHUNAN") return acc + 1;
                return acc;
              }, 0)}
            </strong>
          </div>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={generating}
          >
            Batal
          </Button>
          <Button
            onClick={() => void handleGenerate()}
            disabled={generating || selectedTipe.length === 0}
          >
            {generating ? "Memproses..." : "Generate"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
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
  item: PeriodeBukuItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Periode {item.periodeIdKode}?</DialogTitle>
          <DialogDescription>
            Periode <strong>{item.periodeIdKode}</strong> akan dihapus dari
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
// Page
// ---------------------------------------------------------------------------

function PeriodeBukuListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = usePeriodeBukuFilters();

  const [deleteTarget, setDeleteTarget] =
    React.useState<PeriodeBukuItem | null>(null);
  const [generateOpen, setGenerateOpen] = React.useState(false);
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
    if (filters.filterTipe) p["filter[tipe_periode]"] = filters.filterTipe;
    if (filters.filterStatus) p["filter[status_periode]"] = filters.filterStatus;
    if (filters.filterTahun) p["filter[tahun_buku]"] = filters.filterTahun;
    if (filters.filterWorkflow) p["filter[workflow_status]"] = filters.filterWorkflow;
    if (filters.cursor) p.cursor = filters.cursor;
    return p;
  }, [filters]);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["periode-buku", queryParams],
    queryFn: () =>
      periodeBukuApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[tipe_periode]": filters.filterTipe || undefined,
        "filter[status_periode]": filters.filterStatus || undefined,
        "filter[tahun_buku]": filters.filterTahun || undefined,
        "filter[workflow_status]": filters.filterWorkflow || undefined,
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

  const TIPE_LABELS: Record<string, string> = {
    BULANAN: "Bulanan",
    TRIWULANAN: "Triwulanan",
    TAHUNAN: "Tahunan",
  };

  const STATUS_PERIODE_LABELS: Record<string, string> = {
    OPEN: "Buka",
    SOFT_CLOSED: "Soft-Close",
    CLOSED: "Ditutup",
  };

  const WORKFLOW_LABELS: Record<string, string> = {
    DRAFT: "Draf",
    PENDING_REVIEW: "Menunggu Review",
    PENDING_APPROVAL: "Menunggu Approval",
    APPROVED: "Disetujui",
    RETURNED: "Dikembalikan",
  };

  const activeFilters: ActiveFilter[] = React.useMemo(() => {
    const f: ActiveFilter[] = [];
    if (filters.filterTipe) {
      f.push({
        key: "filter[tipe_periode]",
        label: "Tipe",
        value: filters.filterTipe,
        displayValue: TIPE_LABELS[filters.filterTipe] ?? filters.filterTipe,
      });
    }
    if (filters.filterStatus) {
      f.push({
        key: "filter[status_periode]",
        label: "Status Periode",
        value: filters.filterStatus,
        displayValue:
          STATUS_PERIODE_LABELS[filters.filterStatus] ?? filters.filterStatus,
      });
    }
    if (filters.filterTahun) {
      f.push({
        key: "filter[tahun_buku]",
        label: "Tahun",
        value: filters.filterTahun,
        displayValue: filters.filterTahun,
      });
    }
    if (filters.filterWorkflow) {
      f.push({
        key: "filter[workflow_status]",
        label: "Status Workflow",
        value: filters.filterWorkflow,
        displayValue:
          WORKFLOW_LABELS[filters.filterWorkflow] ?? filters.filterWorkflow,
      });
    }
    return f;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    filters.filterTipe,
    filters.filterStatus,
    filters.filterTahun,
    filters.filterWorkflow,
  ]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[tipe_periode]") void filters.setFilterTipe("");
    if (key === "filter[status_periode]") void filters.setFilterStatus("");
    if (key === "filter[tahun_buku]") void filters.setFilterTahun("");
    if (key === "filter[workflow_status]") void filters.setFilterWorkflow("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterTipe("");
    void filters.setFilterStatus("");
    void filters.setFilterTahun("");
    void filters.setFilterWorkflow("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: PeriodeBukuItem) => {
    try {
      await periodeBukuApi.delete(item.id, uuidv4());
      notify.success(
        `Periode ${item.periodeIdKode} berhasil dihapus.`,
      );
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(
          err as { code: string; message: string; traceId: string },
        );
      }
    }
  };

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const handleSubmit = async (item: PeriodeBukuItem) => {
    try {
      await periodeBukuApi.submit(
        item.id,
        { rowVersion: item.rowVersion },
        uuidv4(),
      );
      notify.success(
        `Periode ${item.periodeIdKode} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/periode-buku/${item.id}`),
          },
        },
      );
      void refetch();
    } catch (err) {
      if (err && typeof err === "object" && "code" in err) {
        notify.error(
          err as { code: string; message: string; traceId: string },
        );
      }
    }
  };

  // ---------------------------------------------------------------------------
  // Format date helper
  // ---------------------------------------------------------------------------

  const fmtDate = (iso: string | null | undefined) => {
    if (!iso) return "—";
    try {
      return format(parseISO(iso), "dd MMM yyyy");
    } catch {
      return iso;
    }
  };

  // ---------------------------------------------------------------------------
  // Columns
  // ---------------------------------------------------------------------------

  const columns: ColumnDef<PeriodeBukuItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "periodeIdKode",
        header: () => (
          <SortHeader
            label="Kode Periode"
            sortKey="periode_id_kode"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <code className="font-mono font-bold text-sm">
            {row.original.periodeIdKode}
          </code>
        ),
      },
      {
        accessorKey: "tipePeriode",
        header: () => (
          <SortHeader
            label="Tipe"
            sortKey="tipe_periode"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <TipePeriodeBadge tipe={row.original.tipePeriode} />
        ),
      },
      {
        accessorKey: "tahunBuku",
        header: () => (
          <SortHeader
            label="Tahun"
            sortKey="tahun_buku"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="tabular-nums">{row.original.tahunBuku}</span>
        ),
      },
      {
        id: "bulanTriwulan",
        header: "Bulan / Triwulan",
        cell: ({ row }) => {
          const item = row.original;
          if (item.tipePeriode === "BULANAN" && item.bulan !== null) {
            return (
              <span className="tabular-nums text-sm">
                Bulan {String(item.bulan).padStart(2, "0")}
              </span>
            );
          }
          if (item.tipePeriode === "TRIWULANAN" && item.triwulan !== null) {
            return <span className="text-sm">Q{item.triwulan}</span>;
          }
          return <span className="text-muted-foreground text-sm">—</span>;
        },
      },
      {
        accessorKey: "tanggalMulai",
        header: "Tanggal Mulai",
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {fmtDate(row.original.tanggalMulai)}
          </span>
        ),
      },
      {
        accessorKey: "tanggalAkhir",
        header: "Tanggal Akhir",
        cell: ({ row }) => (
          <span className="text-sm tabular-nums">
            {fmtDate(row.original.tanggalAkhir)}
          </span>
        ),
      },
      {
        accessorKey: "statusPeriode",
        header: () => (
          <SortHeader
            label="Status Periode"
            sortKey="status_periode"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <StatusPeriodeBadge status={row.original.statusPeriode} />
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
          <WorkflowStatusBadge
            status={row.original.workflowStatus as Parameters<typeof WorkflowStatusBadge>[0]["status"]}
          />
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
        cell: ({ row }) => (
          <span className="text-sm text-muted-foreground tabular-nums">
            {fmtDate(row.original.createdAt)}
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
          const canEdit = perms.canUpdate("periode") && isDraft;
          const canDelete = perms.canDelete("periode") && isDraft;
          const canSubmit = perms.canSubmit("periode") && isDraft;

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  aria-label={`Aksi untuk periode ${item.periodeIdKode}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/periode-buku/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/periode-buku/${item.id}/edit`}>
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
                  <Link href={`/master/periode-buku/${item.id}/history`}>
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

  // ---------------------------------------------------------------------------
  // Export params
  // ---------------------------------------------------------------------------

  const exportQueryParams = React.useMemo(() => {
    const p: Record<string, string | number | boolean | null | undefined> = {
      sort: filters.sort || undefined,
      q: filters.q || undefined,
    };
    if (filters.filterTipe) p["filter[tipe_periode]"] = filters.filterTipe;
    if (filters.filterStatus) p["filter[status_periode]"] = filters.filterStatus;
    if (filters.filterTahun) p["filter[tahun_buku]"] = filters.filterTahun;
    if (filters.filterWorkflow) p["filter[workflow_status]"] = filters.filterWorkflow;
    return p;
  }, [filters]);

  const exportFilename = `periode-buku-${format(new Date(), "yyyyMMdd")}`;

  const canCreate = perms.canCreate("periode") && !perms.isAuditRole();

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Header */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Periode Buku</span>
      </nav>
      <h1 className="text-2xl font-semibold">Daftar Periode Buku</h1>

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
        searchPlaceholder="Cari kode periode (mis. 2026-M06)..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Filter Tipe Periode */}
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
                aria-label="Filter tipe periode"
              >
                <SelectValue placeholder="Semua Tipe" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Tipe</SelectItem>
                <SelectItem value="BULANAN">Bulanan</SelectItem>
                <SelectItem value="TRIWULANAN">Triwulanan</SelectItem>
                <SelectItem value="TAHUNAN">Tahunan</SelectItem>
              </SelectContent>
            </Select>

            {/* Filter Status Periode */}
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
                className="h-9 w-[160px]"
                aria-label="Filter status periode"
              >
                <SelectValue placeholder="Semua Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status</SelectItem>
                <SelectItem value="OPEN">Buka</SelectItem>
                <SelectItem value="SOFT_CLOSED">Soft-Close</SelectItem>
                <SelectItem value="CLOSED">Ditutup</SelectItem>
              </SelectContent>
            </Select>

            {/* Filter Workflow */}
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
                className="h-9 w-[200px]"
                aria-label="Filter status workflow"
              >
                <SelectValue placeholder="Semua Status Workflow" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Status Workflow</SelectItem>
                <SelectItem value="DRAFT">Draf</SelectItem>
                <SelectItem value="PENDING_REVIEW">Menunggu Review</SelectItem>
                <SelectItem value="PENDING_APPROVAL">
                  Menunggu Approval
                </SelectItem>
                <SelectItem value="APPROVED">Disetujui</SelectItem>
                <SelectItem value="RETURNED">Dikembalikan</SelectItem>
              </SelectContent>
            </Select>

            {/* Filter Tahun Buku */}
            <Input
              type="number"
              min={2000}
              max={2099}
              placeholder="Tahun"
              className="h-9 w-[100px]"
              value={filters.filterTahun}
              onChange={(e) => {
                void filters.setFilterTahun(e.target.value);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
              aria-label="Filter tahun buku"
            />
          </div>
        }
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onExport={(fmt) => {
          const url = periodeBukuApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof periodeBukuApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          canCreate ? (
            <div className="flex items-center gap-2">
              {/* Generate button — only for users with periode.create */}
              <Button
                size="sm"
                variant="outline"
                onClick={() => setGenerateOpen(true)}
              >
                <Sparkles className="mr-1.5 h-4 w-4" aria-hidden />
                Generate Tahun...
              </Button>
              <Button size="sm" asChild>
                <Link href="/master/periode-buku/new">
                  <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                  Buat Periode
                </Link>
              </Button>
            </div>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada periode buku yang cocok dengan pencarian."
            : "Belum ada periode buku. Klik '+ Buat Periode' atau 'Generate Tahun...' untuk mulai."
        }
        onRetry={() => void refetch()}
      />

      {/* Generate dialog */}
      <GenerateDialog
        open={generateOpen}
        onOpenChange={setGenerateOpen}
        onSuccess={() => void refetch()}
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

export default function PeriodeBukuListPage() {
  return (
    <Suspense>
      <PeriodeBukuListContent />
    </Suspense>
  );
}
