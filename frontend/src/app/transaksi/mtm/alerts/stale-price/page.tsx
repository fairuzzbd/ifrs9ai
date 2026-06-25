"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useQueryState, parseAsString, parseAsInteger } from "nuqs";
import { type ColumnDef, type SortingState } from "@tanstack/react-table";
import { AlertTriangle } from "lucide-react";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { MtmStaleBadge } from "@/components/blips/mtm/MtmStaleBadge";
import { MtmSourceBadge } from "@/components/blips/mtm/MtmSourceBadge";
import { MtmRoutingBadge } from "@/components/blips/mtm/MtmRoutingBadge";
import { MtmStatusBadge } from "@/components/blips/mtm/MtmStatusBadge";
import { MtmOverrideApproveDialog } from "@/components/blips/mtm/MtmOverrideApproveDialog";

import { mtmAlertsApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { MtmStaleAlertItem, StalePriceReason } from "@/lib/schemas/mtm.schema";
import type { ActiveFilter } from "@/components/blips/DataTable";
import { Button } from "@/components/ui/button";

// ---------------------------------------------------------------------------
// Page — S3: Stale Price Alerts
// Persona: ROLE-AKUN-CTL, ROLE-RISK, ROLE-AUDIT, ROLE-IT-ADMIN (mtm.read)
// ---------------------------------------------------------------------------

function StaleAlertContent() {
  const perms = usePermissions();
  const queryClient = useQueryClient();

  const [cursor, setCursor] = useQueryState("cursor", parseAsString.withDefault(""));
  const [limit] = useQueryState("limit", parseAsInteger.withDefault(50));
  const [sort, setSort] = useQueryState("sort", parseAsString.withDefault("harga_age_days:desc"));
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());
  const [approveTarget, setApproveTarget] = React.useState<MtmStaleAlertItem | null>(null);

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: mtmQueryKeys.staleAlert({ cursor: cursor || undefined, limit, "filter[tanggal_mtm]": undefined }),
    queryFn: () => mtmAlertsApi.stalePrice({ cursor: cursor || undefined, limit }),
    staleTime: 30_000,
  });

  const sortingState: SortingState = React.useMemo(() => {
    if (!sort) return [];
    return sort.split(",").map((part) => {
      const [id, dir] = part.split(":");
      return { id: id ?? "", desc: dir === "desc" };
    });
  }, [sort]);

  const toggleSort = (colId: string) => {
    const current = sortingState.find((s) => s.id === colId);
    const newSorting = !current
      ? [{ id: colId, desc: false }]
      : !current.desc
      ? [{ id: colId, desc: true }]
      : [];
    void setSort(
      newSorting.length === 0
        ? "harga_age_days:desc"
        : newSorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","),
    );
    void setCursor("");
    setCursorHistory([""]);
    setPageIndex(0);
  };

  const handleNextPage = () => {
    const nextCursor = data?.pagination.nextCursor;
    if (nextCursor) {
      const newHistory = [...cursorHistory, nextCursor];
      setCursorHistory(newHistory);
      setPageIndex(newHistory.length - 1);
      void setCursor(nextCursor);
    }
  };

  const handlePrevPage = () => {
    if (pageIndex > 0) {
      const newIndex = pageIndex - 1;
      setPageIndex(newIndex);
      void setCursor(cursorHistory[newIndex] ?? "");
    }
  };

  const hasEscalated = data?.data?.some((item) => item.esklasiasiFlag) ?? false;
  const canOverride = perms.can("mtm.override");

  const columns: ColumnDef<MtmStaleAlertItem>[] = React.useMemo(
    () => [
      {
        accessorKey: "instrumenKode",
        header: () => (
          <SortHeader label="Instrumen" sortKey="instrumen_kode" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <div>
            <Link
              href={`/mtm/${row.original.id}`}
              className="text-sm font-mono text-primary hover:underline"
            >
              {row.original.instrumenKode}
            </Link>
            <p className="text-xs text-muted-foreground truncate max-w-[180px]">{row.original.instrumenNama}</p>
          </div>
        ),
      },
      {
        accessorKey: "klasifikasiSnapshot",
        header: "Klasifikasi",
        cell: ({ row }) => (
          <MtmRoutingBadge eventCodes={[]} klasifikasi={row.original.klasifikasiSnapshot} />
        ),
      },
      {
        accessorKey: "tanggalMtm",
        header: () => (
          <SortHeader label="Tanggal MTM" sortKey="tanggal_mtm" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => <span className="text-sm">{row.original.tanggalMtm}</span>,
      },
      {
        accessorKey: "hargaTanggal",
        header: "Tanggal Harga",
        cell: ({ row }) => <span className="text-sm text-muted-foreground">{row.original.hargaTanggal}</span>,
      },
      {
        accessorKey: "hargaAgeDays",
        header: () => (
          <SortHeader label="Umur Harga" sortKey="harga_age_days" sorting={sortingState} onToggle={toggleSort} />
        ),
        cell: ({ row }) => (
          <MtmStaleBadge
            hargaAgeDays={row.original.hargaAgeDays}
            stalePriceReason={row.original.stalePriceReason as StalePriceReason}
            escalated={row.original.esklasiasiFlag}
          />
        ),
      },
      {
        accessorKey: "status",
        header: "Status",
        cell: ({ row }) => <MtmStatusBadge status={row.original.status} size="sm" />,
      },
      {
        id: "actions",
        header: "Aksi",
        cell: ({ row }) => {
          const item = row.original;
          if (!canOverride || item.lockedFlag) return null;

          return (
            <div className="flex gap-1">
              <Button
                size="sm"
                variant="outline"
                className="h-7 text-xs"
                onClick={() => setApproveTarget(item)}
                aria-label={`Setujui override untuk ${item.instrumenKode}`}
              >
                Setuju Override
              </Button>
            </div>
          );
        },
      },
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [sortingState, canOverride],
  );

  const activeFilters: ActiveFilter[] = [];

  return (
    <div className="container mx-auto py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/mtm" className="hover:underline">MTM Harian</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Peringatan Harga Kedaluwarsa</span>
      </nav>

      <h1 className="text-2xl font-semibold">Peringatan Harga Kedaluwarsa (Stale Price)</h1>

      {/* Escalation banner */}
      {hasEscalated && (
        <div
          className="flex items-start gap-3 rounded-md border border-red-300 bg-red-50 px-4 py-3"
          role="alert"
          aria-live="polite"
        >
          <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-red-600" aria-hidden />
          <div>
            <p className="font-semibold text-red-800">Eskalasi Aktif</p>
            <p className="text-sm text-red-700">
              Satu atau lebih instrumen memiliki harga yang kedaluwarsa melebihi batas eskalasi (7+ hari).
              ROLE-RISK telah dinotifikasi secara otomatis. Segera koordinasi upload harga terbaru.
            </p>
          </div>
        </div>
      )}

      <DataTable
        columns={columns}
        data={data?.data ?? []}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        error={error instanceof Error ? error : null}
        sorting={sortingState}
        onSortingChange={(newSorting) => {
          void setSort(
            newSorting.length === 0
              ? "harga_age_days:desc"
              : newSorting.map((s) => `${s.id}:${s.desc ? "desc" : "asc"}`).join(","),
          );
          void setCursor("");
          setCursorHistory([""]);
          setPageIndex(0);
        }}
        activeFilters={activeFilters}
        onRemoveFilter={() => {}}
        onClearFilters={() => {}}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={pageIndex > 0}
        pageNumber={pageIndex + 1}
        onRefresh={() => {
          void refetch();
          setLastUpdated(new Date());
        }}
        lastUpdated={lastUpdated}
        emptyMessage="Tidak ada harga kedaluwarsa saat ini."
        onRetry={() => void refetch()}
      />

      {/* Override approve dialog */}
      {approveTarget && (
        <MtmOverrideApproveDialog
          open={!!approveTarget}
          onOpenChange={(v) => { if (!v) setApproveTarget(null); }}
          mtmId={approveTarget.id}
          instrumenKode={approveTarget.instrumenKode}
          tanggalMtm={approveTarget.tanggalMtm}
          deviationFlag={false}
          onSuccess={() => {
            setApproveTarget(null);
            void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.staleAlerts() });
            void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
          }}
        />
      )}
    </div>
  );
}

export default function MtmStaleAlertPage() {
  return (
    <Suspense>
      <StaleAlertContent />
    </Suspense>
  );
}
