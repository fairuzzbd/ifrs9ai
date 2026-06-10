"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { z } from "zod";
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
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Sprout,
} from "lucide-react";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { bobotSkenarioApi } from "@/lib/api/bobot-skenario.api";
import { notify } from "@/lib/notify";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  SKENARIO_ECL_LABELS,
  bobotDecimalToPercent,
  groupIntoTrios,
  type BobotSkenarioItem,
  type SkenarioEcl,
} from "@/lib/schemas/bobot-skenario.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";

// ---------------------------------------------------------------------------
// URL state
// ---------------------------------------------------------------------------

function useBobotSkenarioFilters() {
  const [q, setQ] = useQueryState("q", parseAsString.withDefault(""));
  const [sort, setSort] = useQueryState(
    "sort",
    parseAsString.withDefault("periode_berlaku_dari:desc"),
  );
  const [filterSkenario, setFilterSkenario] = useQueryState(
    "filter[skenario]",
    parseAsString.withDefault(""),
  );
  const [filterStatus, setFilterStatus] = useQueryState(
    "filter[workflow_status]",
    parseAsString.withDefault(""),
  );
  const [filterPeriode, setFilterPeriode] = useQueryState(
    "filter[periode_berlaku_dari]",
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
    filterSkenario, setFilterSkenario,
    filterStatus, setFilterStatus,
    filterPeriode, setFilterPeriode,
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
  item: BobotSkenarioItem;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: () => Promise<void>;
}) {
  const [deleting, setDeleting] = React.useState(false);
  const label = SKENARIO_ECL_LABELS[item.skenario] ?? item.skenario;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Hapus Bobot Skenario {label}?</DialogTitle>
          <DialogDescription>
            Bobot skenario <strong>{label}</strong> (Bobot:{" "}
            {bobotDecimalToPercent(item.bobot)}%) untuk periode{" "}
            <strong>{item.periodeBerlakuDari}</strong> akan dihapus (soft-delete).
            Trio skenario periode tersebut akan menjadi tidak lengkap.
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
// Seed Default dialog
// ---------------------------------------------------------------------------

// Default periode = awal tahun berjalan (e.g. "2026-01-01")
function defaultPeriodDate(): string {
  return `${new Date().getFullYear()}-01-01`;
}

const seedPeriodeSchema = z
  .string()
  .regex(/^\d{4}-\d{2}-\d{2}$/, "Format tanggal tidak valid (YYYY-MM-DD)")
  .refine((v) => v >= "1900-01-01", "Tanggal minimal 1900-01-01")
  .refine((v) => v <= "2099-12-31", "Tanggal maksimal 2099-12-31");

function SeedDefaultDialog({
  open,
  onOpenChange,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onConfirm: (periodeBerlakuDari: string) => Promise<void>;
}) {
  const [seeding, setSeeding] = React.useState(false);
  const [periodeBerlakuDari, setPeriodeBerlakuDari] = React.useState(
    defaultPeriodDate,
  );
  const [dateError, setDateError] = React.useState<string | null>(null);

  // Reset state when dialog opens
  React.useEffect(() => {
    if (open) {
      setPeriodeBerlakuDari(defaultPeriodDate());
      setDateError(null);
    }
  }, [open]);

  const handleDateChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = e.target.value;
    setPeriodeBerlakuDari(val);
    const result = seedPeriodeSchema.safeParse(val);
    setDateError(result.success ? null : result.error.issues[0]?.message ?? "Tanggal tidak valid");
  };

  const handleConfirm = async () => {
    const result = seedPeriodeSchema.safeParse(periodeBerlakuDari);
    if (!result.success) {
      setDateError(result.error.issues[0]?.message ?? "Tanggal tidak valid");
      return;
    }
    setSeeding(true);
    try {
      await onConfirm(periodeBerlakuDari);
      onOpenChange(false);
    } finally {
      setSeeding(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Seed Default Bobot Skenario?</DialogTitle>
          <DialogDescription>
            Membuat 3 baris DRAFT dengan bobot default DEC-010:{" "}
            <strong>Good=25%, Normal=50%, Bad=25%</strong> untuk periode berlaku
            dari <strong>tanggal yang dipilih di bawah</strong>. Jika trio untuk
            periode tersebut sudah ada, operasi ini akan dilewati (idempotent
            &mdash; tidak membuat duplikat).
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <Label htmlFor="seed-periode-dari">
            Periode Berlaku Dari <span aria-hidden>*</span>
          </Label>
          <Input
            id="seed-periode-dari"
            type="date"
            value={periodeBerlakuDari}
            onChange={handleDateChange}
            min="1900-01-01"
            max="2099-12-31"
            aria-describedby={dateError ? "seed-periode-error" : undefined}
            aria-invalid={!!dateError}
            disabled={seeding}
          />
          {dateError && (
            <p
              id="seed-periode-error"
              role="alert"
              className="text-sm text-destructive"
            >
              {dateError}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={seeding}
          >
            Batal
          </Button>
          <Button
            disabled={seeding || !!dateError || !periodeBerlakuDari}
            onClick={() => void handleConfirm()}
          >
            {seeding ? "Membuat..." : "Buat Default"}
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
// Trio summary banner
// ---------------------------------------------------------------------------

function TrioSummaryBanner({ items }: { items: BobotSkenarioItem[] }) {
  const trios = React.useMemo(() => groupIntoTrios(items), [items]);
  const latestTrio = trios[0];

  if (!latestTrio) {
    return (
      <div
        role="status"
        aria-live="polite"
        className="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <div className="text-sm text-amber-800">
          <strong>Trio skenario belum ada.</strong> Buat 3 baris (Good, Normal, Bad)
          untuk periode yang sama, atau gunakan tombol{" "}
          <strong>Seed Default</strong> untuk membuat trio dengan bobot default
          DEC-010 (0.25 / 0.50 / 0.25).
        </div>
      </div>
    );
  }

  const goodPct = latestTrio.good
    ? bobotDecimalToPercent(latestTrio.good.bobot)
    : null;
  const normalPct = latestTrio.normal
    ? bobotDecimalToPercent(latestTrio.normal.bobot)
    : null;
  const badPct = latestTrio.bad
    ? bobotDecimalToPercent(latestTrio.bad.bobot)
    : null;

  const sumPct = latestTrio.sum
    ? (parseFloat(latestTrio.sum) * 100).toFixed(2)
    : null;

  if (!latestTrio.isComplete) {
    return (
      <div
        role="status"
        aria-live="polite"
        className="flex items-start gap-3 rounded-md border border-amber-300 bg-amber-50 px-4 py-3"
      >
        <AlertTriangle
          className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
          aria-hidden
        />
        <div className="text-sm text-amber-800">
          <strong>Trio aktif periode {latestTrio.periodeBerlakuDari}</strong> belum
          lengkap:{" "}
          {!latestTrio.good && <span className="font-medium">Good </span>}
          {!latestTrio.normal && <span className="font-medium">Normal </span>}
          {!latestTrio.bad && <span className="font-medium">Bad </span>}
          belum tersedia. Total bobot harus = 100% (1.0) — DEC-010.
        </div>
      </div>
    );
  }

  if (latestTrio.isValid) {
    return (
      <div
        role="status"
        aria-live="polite"
        className="flex items-start gap-3 rounded-md border border-green-300 bg-green-50 px-4 py-3"
      >
        <CheckCircle2
          className="mt-0.5 h-4 w-4 shrink-0 text-green-600"
          aria-hidden
        />
        <div className="text-sm text-green-800 space-y-1">
          <div className="font-semibold">
            Trio aktif periode {latestTrio.periodeBerlakuDari} — Sum{" "}
            <span className="font-mono">{sumPct}%</span>{" "}
            <span className="text-green-700">✓ Valid (DEC-010)</span>
          </div>
          <div className="flex gap-4 font-mono text-xs">
            <span>
              G:{" "}
              <strong>
                {goodPct}%
              </strong>
            </span>
            <span>
              N:{" "}
              <strong>
                {normalPct}%
              </strong>
            </span>
            <span>
              B:{" "}
              <strong>
                {badPct}%
              </strong>
            </span>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-md border border-red-300 bg-red-50 px-4 py-3"
    >
      <XCircle
        className="mt-0.5 h-4 w-4 shrink-0 text-red-600"
        aria-hidden
      />
      <div className="text-sm text-red-800 space-y-1">
        <div className="font-semibold">
          Trio aktif periode {latestTrio.periodeBerlakuDari} — Sum{" "}
          <span className="font-mono">{sumPct}%</span>{" "}
          <span className="text-red-700">✗ INVALID — DEC-010 violation</span>
        </div>
        <div className="flex gap-4 font-mono text-xs">
          <span>G: <strong>{goodPct ?? "—"}%</strong></span>
          <span>N: <strong>{normalPct ?? "—"}%</strong></span>
          <span>B: <strong>{badPct ?? "—"}%</strong></span>
        </div>
        <div className="text-xs">
          Total bobot harus tepat 1.0 (100%). Perbaiki nilai bobot salah satu skenario.
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

function BobotSkenarioListContent() {
  const router = useRouter();
  const perms = usePermissions();
  const filters = useBobotSkenarioFilters();

  const [deleteTarget, setDeleteTarget] = React.useState<BobotSkenarioItem | null>(null);
  const [seedDialogOpen, setSeedDialogOpen] = React.useState(false);
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["bobot-skenario", {
      limit: filters.limit,
      sort: filters.sort,
      q: filters.q,
      skenario: filters.filterSkenario,
      status: filters.filterStatus,
      periode: filters.filterPeriode,
      cursor: filters.cursor,
    }],
    queryFn: () =>
      bobotSkenarioApi.list({
        limit: filters.limit,
        sort: filters.sort || undefined,
        q: filters.q || undefined,
        "filter[skenario]": filters.filterSkenario || undefined,
        "filter[workflow_status]": filters.filterStatus || undefined,
        "filter[periode_berlaku_dari]": filters.filterPeriode || undefined,
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
      void filters.setSort("periode_berlaku_dari:desc");
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
    if (filters.filterSkenario) {
      f.push({
        key: "filter[skenario]",
        label: "Skenario",
        value: filters.filterSkenario,
        displayValue:
          SKENARIO_ECL_LABELS[filters.filterSkenario as SkenarioEcl] ??
          filters.filterSkenario,
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
    if (filters.filterPeriode) {
      f.push({
        key: "filter[periode_berlaku_dari]",
        label: "Periode Dari",
        value: filters.filterPeriode,
        displayValue: filters.filterPeriode,
      });
    }
    return f;
  }, [filters.filterSkenario, filters.filterStatus, filters.filterPeriode]);

  const handleRemoveFilter = (key: string) => {
    if (key === "filter[skenario]") void filters.setFilterSkenario("");
    if (key === "filter[workflow_status]") void filters.setFilterStatus("");
    if (key === "filter[periode_berlaku_dari]") void filters.setFilterPeriode("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleClearFilters = () => {
    void filters.setFilterSkenario("");
    void filters.setFilterStatus("");
    void filters.setFilterPeriode("");
    void filters.setQ("");
    void filters.setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  // ---------------------------------------------------------------------------
  // Delete
  // ---------------------------------------------------------------------------

  const handleDelete = async (item: BobotSkenarioItem) => {
    try {
      await bobotSkenarioApi.softDelete(item.id, uuidv4());
      notify.success(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} periode ${item.periodeBerlakuDari} berhasil dihapus.`,
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

  const handleSubmit = async (item: BobotSkenarioItem) => {
    try {
      await bobotSkenarioApi.submit(item.id, { rowVersion: item.rowVersion });
      notify.success(
        `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario]} berhasil disubmit untuk review.`,
        {
          action: {
            label: "Lihat detail",
            onClick: () => router.push(`/master/bobot-skenario/${item.id}`),
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
  // Seed Default
  // ---------------------------------------------------------------------------

  const handleSeedDefault = async (periodeBerlakuDari: string) => {
    try {
      const res = await bobotSkenarioApi.seedDefault(
        { periodeBerlakuDari },
        uuidv4(),
      );
      if (res.data.skipped) {
        notify.info(
          `Trio untuk periode ${periodeBerlakuDari} sudah ada — operasi dilewati.`,
        );
      } else {
        notify.success(
          `Trio default berhasil dibuat untuk ${periodeBerlakuDari} (G=0.25, N=0.50, B=0.25).`,
        );
      }
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

  const columns: ColumnDef<BobotSkenarioItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "skenario",
        header: () => (
          <SortHeader
            label="Skenario"
            sortKey="skenario"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="font-medium">
            {SKENARIO_ECL_LABELS[row.original.skenario] ?? row.original.skenario}
          </span>
        ),
      },
      {
        accessorKey: "bobot",
        header: () => (
          <SortHeader
            label="Bobot (%)"
            sortKey="bobot"
            sorting={sortingState}
            onToggle={toggleSort}
          />
        ),
        cell: ({ row }) => (
          <span className="font-mono text-right block">
            {bobotDecimalToPercent(row.original.bobot)}%
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
          row.original.periodeBerlakuSampai ? (
            fmtDate(row.original.periodeBerlakuSampai)
          ) : (
            <span className="text-green-700 font-medium text-xs">Sekarang</span>
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
                  aria-label={`Aksi untuk bobot skenario ${item.skenario} periode ${item.periodeBerlakuDari}`}
                >
                  <MoreHorizontal className="h-4 w-4" aria-hidden />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link href={`/master/bobot-skenario/${item.id}`}>
                    Lihat Detail
                  </Link>
                </DropdownMenuItem>
                {canEdit && (
                  <DropdownMenuItem asChild>
                    <Link href={`/master/bobot-skenario/${item.id}/edit`}>
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
                  <Link href={`/master/bobot-skenario/${item.id}/history`}>
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
    if (filters.filterSkenario) p["filter[skenario]"] = filters.filterSkenario;
    if (filters.filterStatus) p["filter[workflow_status]"] = filters.filterStatus;
    if (filters.filterPeriode) p["filter[periode_berlaku_dari]"] = filters.filterPeriode;
    return p;
  }, [filters]);

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <span>Master Data</span>
        <span className="mx-1.5">/</span>
        <span>Parameter ECL</span>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Bobot Skenario</span>
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
          <strong>Parameter ECL</strong> — perubahan bobot skenario akan mempengaruhi
          semua kalkulasi ECL (formula weighted sum DEC-010). Workflow 6-eyes: memerlukan
          persetujuan ALCO dan CFO/Komite. Proses approve memerlukan MFA step-up (DEC-027).
          Total bobot Good + Normal + Bad <strong>harus = 1.0 (100%)</strong>.
        </div>
      </div>

      <h1 className="text-2xl font-semibold">Bobot Skenario ECL</h1>

      {/* Trio sum indicator banner — live from current page data */}
      {data?.data && data.data.length > 0 && (
        <TrioSummaryBanner items={data.data} />
      )}

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
        searchPlaceholder="Cari skenario, catatan..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        filterPanel={
          <div className="flex flex-wrap gap-2">
            {/* Filter skenario */}
            <Select
              value={filters.filterSkenario || "all"}
              onValueChange={(v) => {
                void filters.setFilterSkenario(v === "all" ? "" : v);
                void filters.setCursor("");
                setCursorHistory([""]);
                setPageIndex(0);
              }}
            >
              <SelectTrigger
                className="h-9 w-[200px]"
                aria-label="Filter skenario"
              >
                <SelectValue placeholder="Semua Skenario" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Semua Skenario</SelectItem>
                <SelectItem value="GOOD">{SKENARIO_ECL_LABELS.GOOD}</SelectItem>
                <SelectItem value="NORMAL">{SKENARIO_ECL_LABELS.NORMAL}</SelectItem>
                <SelectItem value="BAD">{SKENARIO_ECL_LABELS.BAD}</SelectItem>
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
                <SelectItem value="PENDING_APPROVAL">Menunggu Approval 1</SelectItem>
                <SelectItem value="PENDING_APPROVAL_2">Menunggu Approval 2</SelectItem>
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
          const url = bobotSkenarioApi.exportUrl({
            ...exportQueryParams,
            format: fmt,
          } as Parameters<typeof bobotSkenarioApi.exportUrl>[0]);
          window.open(url, "_blank");
        }}
        onRefresh={handleRefresh}
        lastUpdated={lastUpdated}
        createButton={
          perms.canSubmit("ecl_parameter") && !perms.isAuditRole() ? (
            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => setSeedDialogOpen(true)}
              >
                <Sprout className="mr-1.5 h-4 w-4" aria-hidden />
                Seed Default
              </Button>
              <Button size="sm" asChild>
                <Link href="/master/bobot-skenario/new">
                  <Plus className="mr-1.5 h-4 w-4" aria-hidden />
                  Buat Bobot
                </Link>
              </Button>
            </div>
          ) : null
        }
        emptyMessage={
          activeFilters.length > 0 || filters.q
            ? "Tidak ada bobot skenario yang cocok dengan pencarian."
            : "Belum ada bobot skenario. Klik '+ Buat Bobot' atau 'Seed Default' untuk mulai."
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

      {/* Seed Default dialog */}
      <SeedDefaultDialog
        open={seedDialogOpen}
        onOpenChange={setSeedDialogOpen}
        onConfirm={handleSeedDefault}
      />
    </div>
  );
}

export default function BobotSkenarioListPage() {
  return (
    <Suspense>
      <BobotSkenarioListContent />
    </Suspense>
  );
}
