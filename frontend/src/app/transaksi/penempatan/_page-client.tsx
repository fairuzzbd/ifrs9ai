"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Plus, MoreVertical } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { PenempatanStatusBadge } from "@/components/blips/penempatan/PenempatanStatusBadge";
import { WithdrawDialog } from "@/components/blips/penempatan/dialogs/WithdrawDialog";
import { notify } from "@/lib/notify";
import { penempatanApi } from "@/lib/api/penempatan.api";
import type { PenempatanListItem, PenempatanWorkflowStatus } from "@/lib/schemas/penempatan.schema";
import { isApiError } from "@/lib/api";
import type { SortingState } from "@tanstack/react-table";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatIdr(value: number): string {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

function formatDate(s: string): string {
  try {
    return new Intl.DateTimeFormat("id-ID", {
      day: "numeric",
      month: "short",
      year: "numeric",
      timeZone: "Asia/Jakarta",
    }).format(new Date(s));
  } catch {
    return s;
  }
}

// ---------------------------------------------------------------------------
// List page
// ---------------------------------------------------------------------------

export default function PenempatanListPage() {
  const router = useRouter();

  const [data, setData] = React.useState<PenempatanListItem[]>([]);
  const [isLoading, setIsLoading] = React.useState(false);
  const [isError, setIsError] = React.useState(false);
  const [error, setError] = React.useState<Error | null>(null);
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [prevCursors, setPrevCursors] = React.useState<(string | null)[]>([null]);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [hasMore, setHasMore] = React.useState(false);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const [sorting, setSorting] = React.useState<SortingState>([
    { id: "created_at", desc: true },
  ]);
  const [searchValue, setSearchValue] = React.useState("");
  const [statusFilter, setStatusFilter] = React.useState("");

  // Withdraw dialog
  const [withdrawTarget, setWithdrawTarget] = React.useState<PenempatanListItem | null>(null);

  // Current user id (simplified — from localStorage in a real app)
  const [currentUserId] = React.useState<string | null>(() => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("blips_user_id");
    }
    return null;
  });

  const sortParam = React.useMemo(() => {
    return sorting
      .map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`)
      .join(",");
  }, [sorting]);

  const loadData = React.useCallback(
    async (cursorOverride?: string | null) => {
      setIsLoading(true);
      setIsError(false);
      try {
        const params = {
          limit: 50,
          sort: sortParam || "created_at:desc",
          q: searchValue || undefined,
          cursor: cursorOverride !== undefined ? cursorOverride : cursor,
          "filter[workflow_status]": statusFilter || undefined,
        };
        const res = await penempatanApi.list(params);
        setData(res.data);
        setHasMore(res.pagination.hasMore);
        if (res.pagination.nextCursor) {
          setCursor(res.pagination.nextCursor);
        }
        setLastUpdated(new Date());
      } catch (err) {
        setIsError(true);
        setError(err instanceof Error ? err : new Error("Gagal memuat data"));
      } finally {
        setIsLoading(false);
      }
    },
    [sortParam, searchValue, statusFilter, cursor],
  );

  // Initial load + when filters change
  React.useEffect(() => {
    setPrevCursors([null]);
    setPageNumber(1);
    void loadData(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sortParam, searchValue, statusFilter]);

  const handleNextPage = () => {
    setPrevCursors((prev) => [...prev, cursor]);
    setPageNumber((p) => p + 1);
    void loadData(cursor);
  };

  const handlePrevPage = () => {
    const prev = [...prevCursors];
    prev.pop();
    const prevCursor = prev[prev.length - 1] ?? null;
    setPrevCursors(prev);
    setPageNumber((p) => p - 1);
    void loadData(prevCursor);
  };

  const handleExport = async (format: "csv" | "xlsx") => {
    notify.info(`Export ${format.toUpperCase()} sedang diproses...`);
    try {
      const params = {
        sort: sortParam || "created_at:desc",
        q: searchValue || undefined,
        "filter[workflow_status]": statusFilter || undefined,
        export: format,
      };
      const url =
        `/api/v1/transaksi/penempatan-deposito?export=${format}` +
        (searchValue ? `&q=${encodeURIComponent(searchValue)}` : "") +
        (statusFilter ? `&filter[workflow_status]=${statusFilter}` : "");
      // Trigger download
      const a = document.createElement("a");
      a.href = url;
      a.download = `penempatan-${new Date().toISOString().split("T")[0]}.${format}`;
      a.click();
      void params; // suppress lint warning
    } catch {
      notify.info("Export gagal. Coba lagi.");
    }
  };

  // ── Columns ─────────────────────────────────────────────────────────────────

  const columns = React.useMemo<ColumnDef<PenempatanListItem>[]>(
    () => [
      {
        accessorKey: "kodeTransaksi",
        header: "Kode",
        cell: ({ row }) => (
          <Link
            href={`/transaksi/penempatan/${row.original.id}`}
            className="font-mono text-sm text-blue-600 hover:underline"
          >
            {row.original.kodeTransaksi}
          </Link>
        ),
        enableSorting: true,
      },
      {
        id: "counterpartyBankNama",
        accessorKey: "counterpartyBankNama",
        header: "Bank Counterparty",
        enableSorting: true,
      },
      {
        accessorKey: "workflowStatus",
        header: "Status",
        cell: ({ row }) => (
          <PenempatanStatusBadge status={row.original.workflowStatus} size="sm" />
        ),
        enableSorting: true,
      },
      {
        accessorKey: "nominalIdr",
        header: "Nominal (IDR)",
        cell: ({ row }) => (
          <span className="text-right font-mono">{formatIdr(row.original.nominalIdr)}</span>
        ),
        enableSorting: true,
        meta: { align: "right" },
      },
      {
        accessorKey: "tanggalPenempatan",
        header: "Tgl Penempatan",
        cell: ({ row }) => formatDate(row.original.tanggalPenempatan),
        enableSorting: true,
      },
      {
        accessorKey: "tanggalJatuhTempo",
        header: "Jatuh Tempo",
        cell: ({ row }) => formatDate(row.original.tanggalJatuhTempo),
        enableSorting: true,
      },
      {
        accessorKey: "klasifikasiPsak71",
        header: "Klasifikasi",
        cell: ({ row }) =>
          row.original.klasifikasiPsak71 ? (
            <span className="rounded bg-gray-100 px-2 py-0.5 text-xs font-medium">
              {row.original.klasifikasiPsak71}
            </span>
          ) : null,
      },
      {
        id: "aksi",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          const isMaker = item.makerId === currentUserId;
          const ws = item.workflowStatus;

          const menuItems: { label: string; href?: string; onClick?: () => void; destructive?: boolean }[] = [];

          menuItems.push({ label: "Lihat Detail", href: `/transaksi/penempatan/${item.id}` });

          if (ws === "DRAFT" && isMaker) {
            menuItems.push({ label: "Edit", href: `/transaksi/penempatan/${item.id}/edit` });
            menuItems.push({ label: "Submit", href: `/transaksi/penempatan/${item.id}` });
            menuItems.push({
              label: "Batalkan",
              onClick: () => setWithdrawTarget(item),
              destructive: true,
            });
          }

          if (ws === "APPROVED_ACTIVE" && isMaker) {
            menuItems.push({ label: "Ajukan Terminasi", href: `/transaksi/penempatan/${item.id}` });
          }

          return (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="sm" className="h-8 w-8 p-0" aria-label="Menu aksi">
                  <MoreVertical className="h-4 w-4" aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {menuItems.map((item) =>
                  item.href ? (
                    <DropdownMenuItem key={item.label} asChild>
                      <Link href={item.href}>{item.label}</Link>
                    </DropdownMenuItem>
                  ) : (
                    <DropdownMenuItem
                      key={item.label}
                      onClick={item.onClick}
                      className={item.destructive ? "text-destructive" : undefined}
                    >
                      {item.label}
                    </DropdownMenuItem>
                  ),
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          );
        },
      },
    ],
    [currentUserId],
  );

  // ── Active filters ──────────────────────────────────────────────────────────

  const activeFilters: ActiveFilter[] = [];
  if (statusFilter) {
    activeFilters.push({
      key: "workflowStatus",
      label: "Status",
      value: statusFilter,
      displayValue: statusFilter,
    });
  }

  const statusOptions: { value: PenempatanWorkflowStatus | ""; label: string }[] = [
    { value: "", label: "Semua Status" },
    { value: "DRAFT", label: "Konsep" },
    { value: "PENDING_REVIEW", label: "Menunggu Review" },
    { value: "PENDING_APPROVAL", label: "Menunggu Approval" },
    { value: "APPROVED_ACTIVE", label: "Aktif" },
    { value: "TERMINATION_PENDING_REVIEW", label: "Menunggu Review Terminasi" },
    { value: "TERMINATION_PENDING_APPROVAL", label: "Menunggu Approval Terminasi" },
    { value: "TERMINATED", label: "Diterminasi" },
    { value: "MATURED", label: "Jatuh Tempo" },
    { value: "CANCELLED", label: "Dibatalkan" },
  ];

  const filterPanel = (
    <div className="flex items-center gap-2">
      <select
        value={statusFilter}
        onChange={(e) => setStatusFilter(e.target.value)}
        className="h-9 rounded-md border px-3 text-sm"
        aria-label="Filter status"
      >
        {statusOptions.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  );

  const handleWithdrawConfirm = async () => {
    if (!withdrawTarget) return;
    try {
      await penempatanApi.withdraw(withdrawTarget.id);
      notify.success(`Penempatan ${withdrawTarget.kodeTransaksi} berhasil dibatalkan.`);
      setWithdrawTarget(null);
      void loadData(null);
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      } else {
        notify.error({ code: "INTERNAL", message: "Gagal membatalkan penempatan", traceId: "" });
      }
    }
  };

  return (
    <>
      <div className="space-y-4 p-6">
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-semibold text-gray-900">Penempatan Deposito</h1>
          <Button asChild>
            <Link href="/transaksi/penempatan/new">
              <Plus className="mr-2 h-4 w-4" aria-hidden="true" />
              Buat Penempatan
            </Link>
          </Button>
        </div>

        <DataTable
          columns={columns}
          data={data}
          isLoading={isLoading}
          isError={isError}
          error={error}
          sorting={sorting}
          onSortingChange={setSorting}
          searchValue={searchValue}
          onSearchChange={setSearchValue}
          searchPlaceholder="Cari kode / ref bank..."
          activeFilters={activeFilters}
          onRemoveFilter={(key) => {
            if (key === "workflowStatus") setStatusFilter("");
          }}
          onClearFilters={() => {
            setStatusFilter("");
            setSearchValue("");
          }}
          filterPanel={filterPanel}
          onNextPage={handleNextPage}
          onPrevPage={handlePrevPage}
          canPrevPage={pageNumber > 1}
          pageNumber={pageNumber}
          onExport={handleExport}
          onRefresh={() => void loadData(null)}
          lastUpdated={lastUpdated}
          onRetry={() => void loadData(null)}
          createButton={
            <Button asChild size="sm">
              <Link href="/transaksi/penempatan/new">
                <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
                Buat Penempatan
              </Link>
            </Button>
          }
          emptyMessage="Tidak ada penempatan yang cocok dengan filter aktif"
        />
      </div>

      {withdrawTarget && (
        <WithdrawDialog
          open={!!withdrawTarget}
          onOpenChange={(open) => { if (!open) setWithdrawTarget(null); }}
          kodeTransaksi={withdrawTarget.kodeTransaksi}
          onConfirm={handleWithdrawConfirm}
        />
      )}
    </>
  );
}
