"use client";

import * as React from "react";
import { Suspense } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { ArrowLeft } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { MtmStatusBadge } from "@/components/blips/mtm/MtmStatusBadge";
import { MtmDeviationBadge } from "@/components/blips/mtm/MtmDeviationBadge";
import { MtmStaleBadge } from "@/components/blips/mtm/MtmStaleBadge";

import { mtmUploadApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import type { MtmBatchRow } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Batch row table — S2-AC4: row-level breakdown
// ---------------------------------------------------------------------------

const IDR_SHORT = new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 0 });

const BATCH_COLUMNS: ColumnDef<MtmBatchRow>[] = [
  {
    accessorKey: "lineNumber",
    header: () => <SortHeader label="#" sortKey="lineNumber" sorting={[]} onToggle={() => {}} />,
    cell: ({ row }) => <span className="text-xs text-muted-foreground">{row.original.lineNumber}</span>,
    size: 48,
  },
  {
    accessorKey: "instrumenKode",
    header: "Instrumen",
    cell: ({ row }) => (
      <Link
        href={`/mtm/${row.original.mtmId}`}
        className="font-mono text-sm text-primary hover:underline"
        aria-label={`Lihat detail MTM ${row.original.instrumenKode}`}
      >
        {row.original.instrumenKode}
      </Link>
    ),
  },
  {
    accessorKey: "tanggalMtm",
    header: "Tanggal MTM",
    cell: ({ row }) => <span className="text-sm">{row.original.tanggalMtm}</span>,
  },
  {
    accessorKey: "hargaPasarIdr",
    header: "Harga Pasar (IDR)",
    cell: ({ row }) => (
      <span className="font-mono text-sm text-right block">
        {IDR_SHORT.format(row.original.hargaPasarIdr)}
      </span>
    ),
  },
  {
    accessorKey: "deltaPct",
    header: "Delta %",
    cell: ({ row }) => {
      const { deltaPct, deviationFlag } = row.original;
      return (
        <div className="space-y-0.5">
          <span className={`text-sm font-mono ${deltaPct >= 0 ? "text-green-700" : "text-red-700"}`}>
            {deltaPct >= 0 ? "+" : ""}{deltaPct.toFixed(2)}%
          </span>
          {deviationFlag && (
            <MtmDeviationBadge deltaPct={deltaPct} thresholdPct={5} />
          )}
        </div>
      );
    },
  },
  {
    accessorKey: "stalePriceFlag",
    header: "Stale",
    cell: ({ row }) =>
      row.original.stalePriceFlag ? (
        <MtmStaleBadge
          hargaAgeDays={0}
          stalePriceReason="HARGA_TIDAK_TERSEDIA"
          escalated={false}
        />
      ) : (
        <span className="text-xs text-muted-foreground">—</span>
      ),
  },
  {
    accessorKey: "rowStatus",
    header: "Status",
    cell: ({ row }) => <MtmStatusBadge status={row.original.rowStatus} size="sm" />,
  },
  {
    accessorKey: "rowErrorMsg",
    header: "Error",
    cell: ({ row }) =>
      row.original.rowErrorMsg ? (
        <span className="text-xs text-destructive">{row.original.rowErrorMsg}</span>
      ) : (
        <span className="text-xs text-muted-foreground">—</span>
      ),
  },
];

// ---------------------------------------------------------------------------
// Content
// ---------------------------------------------------------------------------

function BatchDetailContent() {
  const params = useParams<{ batch_id: string }>();
  const router = useRouter();

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: mtmQueryKeys.batch(params.batch_id),
    queryFn: () => mtmUploadApi.getBatch(params.batch_id),
    staleTime: 60_000,
  });

  const batch = data?.data;

  if (isLoading) {
    return (
      <div className="container mx-auto py-6 space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-48 w-full" />
      </div>
    );
  }

  if (isError || !batch) {
    return (
      <div className="container mx-auto py-6 text-center space-y-3">
        <p className="text-muted-foreground">Batch tidak ditemukan atau terjadi kesalahan.</p>
        <Button variant="outline" onClick={() => router.back()}>Kembali</Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto py-6 space-y-5">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="flex items-center gap-2 text-sm text-muted-foreground">
        <Button variant="ghost" size="sm" className="h-7 gap-1 px-2" asChild>
          <Link href="/mtm/upload" aria-label="Kembali ke upload MTM">
            <ArrowLeft className="h-3.5 w-3.5" aria-hidden />
            Kembali
          </Link>
        </Button>
        <span>/</span>
        <Link href="/mtm" className="hover:underline">MTM Harian</Link>
        <span>/</span>
        <Link href="/mtm/upload" className="hover:underline">Upload Manual</Link>
        <span>/</span>
        <span className="text-foreground font-medium truncate max-w-[160px]">
          Batch {batch.uploadBatchId.slice(0, 8)}...
        </span>
      </nav>

      {/* Header */}
      <div>
        <h1 className="text-2xl font-semibold">Detail Batch Upload</h1>
        <p className="text-muted-foreground text-sm mt-0.5">
          Diupload oleh {batch.uploaderName} pada {batch.createdAt}
        </p>
      </div>

      {/* Summary */}
      <section className="grid grid-cols-3 gap-4" aria-labelledby="batch-summary">
        <h2 id="batch-summary" className="sr-only">Ringkasan Batch</h2>
        <div className="rounded-lg border p-4 text-center">
          <p className="text-3xl font-bold">{batch.rowsParsed}</p>
          <p className="text-xs text-muted-foreground mt-1">Baris Dibaca</p>
        </div>
        <div className="rounded-lg border p-4 text-center">
          <p className="text-3xl font-bold text-green-700">{batch.rowsValid}</p>
          <p className="text-xs text-muted-foreground mt-1">Baris Valid</p>
        </div>
        <div className="rounded-lg border p-4 text-center">
          <p className={`text-3xl font-bold ${batch.rowsInvalid > 0 ? "text-destructive" : "text-muted-foreground"}`}>
            {batch.rowsInvalid}
          </p>
          <p className="text-xs text-muted-foreground mt-1">Baris Invalid</p>
        </div>
      </section>

      {/* Catatan upload */}
      {batch.catatanUpload && (
        <div className="rounded-md border px-4 py-3 text-sm">
          <span className="font-medium text-muted-foreground">Catatan: </span>
          {batch.catatanUpload}
        </div>
      )}

      {/* Row breakdown DataTable (no server-side pagination needed — all in response) */}
      <section aria-labelledby="rows-heading">
        <h2 id="rows-heading" className="text-base font-semibold mb-3">
          Baris ({batch.rows.length})
        </h2>
        <DataTable
          columns={BATCH_COLUMNS}
          data={batch.rows}
          isLoading={false}
          isError={false}
          error={null}
          sorting={[]}
          onSortingChange={() => {}}
          activeFilters={[]}
          onRemoveFilter={() => {}}
          onClearFilters={() => {}}
          onNextPage={() => {}}
          onPrevPage={() => {}}
          canPrevPage={false}
          pageNumber={1}
          onRefresh={() => void refetch()}
          emptyMessage="Tidak ada baris dalam batch ini."
        />
      </section>
    </div>
  );
}

export default function MtmBatchDetailPage() {
  return (
    <Suspense>
      <BatchDetailContent />
    </Suspense>
  );
}
