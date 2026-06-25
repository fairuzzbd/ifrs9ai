"use client";

/**
 * Route: /master/instrumen/bulk-upload
 * Story: P5-M11-S1 — Upload XLSX 5-sheet + history list
 * Actor: ROLE-MAKER-TR (upload); all roles (history read)
 */

import * as React from "react";
import { Suspense } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { type ColumnDef } from "@tanstack/react-table";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { BulkUploadDropzone } from "@/components/blips/bulkupload/BulkUploadDropzone";
import { BulkBatchStatusBadge } from "@/components/blips/bulkupload/BulkBatchStatusBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { bulkUploadApi, bulkUploadQueryKeys } from "@/lib/api/bulkupload.api";
import { notify } from "@/lib/notify";
import { ApiError } from "@/lib/api";
import type { BulkUploadBatchSummary } from "@/lib/schemas/bulkupload.schema";

function BulkUploadContent() {
  const router = useRouter();
  const [selectedFile, setSelectedFile] = React.useState<File | null>(null);
  const [uploading, setUploading] = React.useState(false);
  const [idempotencyKey] = React.useState(() => uuidv4());

  // History list — using GET /master/instrumen/bulk-upload is a stub;
  // production would have a real list endpoint.
  const { data: _historyData, isLoading: _histLoading } = useQuery({
    queryKey: bulkUploadQueryKeys.batches(),
    queryFn: () => Promise.resolve({ data: [] as BulkUploadBatchSummary[], pagination: { hasMore: false, nextCursor: null, totalEstimate: 0, limit: 50 }, meta: { traceId: "" } }),
    staleTime: 30_000,
  });

  async function handleUpload() {
    if (!selectedFile) return;
    setUploading(true);
    try {
      const result = await bulkUploadApi.upload(selectedFile, undefined, idempotencyKey);
      notify.success(
        `Batch ${result.data.batchId} berhasil diupload — ${result.data.totalRows} baris diparsing.`,
        { action: { label: "Lihat detail", onClick: () => router.push(`/master/instrumen/bulk-upload/${result.data.batchId}`) } },
      );
      router.push(`/master/instrumen/bulk-upload/${result.data.batchId}`);
    } catch (err) {
      if (err instanceof ApiError) {
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal upload file", traceId: "" });
      }
    } finally {
      setUploading(false);
    }
  }

  const columns: ColumnDef<BulkUploadBatchSummary>[] = [
    {
      id: "batchId",
      header: ({ column }) => <SortHeader column={column} label="Batch ID" />,
      accessorKey: "batchId",
      cell: ({ row }) => (
        <button
          type="button"
          className="font-mono text-xs text-primary underline-offset-4 hover:underline"
          onClick={() => router.push(`/master/instrumen/bulk-upload/${row.original.batchId}`)}
          aria-label={`Buka detail batch ${row.original.batchId}`}
        >
          {row.original.batchId}
        </button>
      ),
    },
    {
      id: "status",
      header: "Status",
      cell: ({ row }) => <BulkBatchStatusBadge status={row.original.status} size="sm" />,
    },
    {
      id: "totalRows",
      header: ({ column }) => <SortHeader column={column} label="Total Baris" />,
      accessorKey: "totalRows",
    },
    {
      id: "createdAt",
      header: ({ column }) => <SortHeader column={column} label="Diupload" />,
      accessorKey: "createdAt",
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">
          {new Date(row.original.createdAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}
        </span>
      ),
    },
  ];

  return (
    <div className="container mx-auto py-6 space-y-6">
      <div>
        <h1 className="text-2xl font-bold">Bulk Upload Master Instrumen</h1>
        <p className="text-sm text-muted-foreground">
          Upload XLSX 5-sheet (Deposito, Obligasi, Saham, Reksadana, Tabungan_Cash) — maks 50MB.
        </p>
      </div>

      {/* Upload card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Upload File XLSX</CardTitle>
          <CardDescription>
            File akan diparsing ke sys.upload_batch. Jalankan DRY_RUN setelah upload selesai.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <BulkUploadDropzone
            onFileSelected={setSelectedFile}
            disabled={uploading}
          />
          <Button
            onClick={handleUpload}
            disabled={!selectedFile || uploading}
            aria-busy={uploading}
          >
            {uploading ? "Mengupload..." : "Upload File"}
          </Button>
        </CardContent>
      </Card>

      {/* History table */}
      <div className="space-y-2">
        <h2 className="text-lg font-semibold">Riwayat Upload</h2>
        <DataTable
          columns={columns}
          data={[]}
          isLoading={false}
          isError={false}
          onRetry={() => {}}
          sorting={[]}
          onSortingChange={() => {}}
          activeFilters={[]}
          onClearAllFilters={() => {}}
          searchValue=""
          onSearchChange={() => {}}
          onExport={() => {}}
          exportFormats={["csv", "xlsx"]}
          pagination={{
            pageIndex: 0,
            hasMore: false,
            totalEstimate: 0,
            limit: 50,
            onNext: () => {},
            onPrev: () => {},
          }}
          lastUpdated={new Date()}
          onRefresh={() => {}}
          emptyMessage="Belum ada batch upload. Upload file XLSX untuk memulai."
        />
      </div>
    </div>
  );
}

export default function BulkUploadPage() {
  return (
    <Suspense>
      <BulkUploadContent />
    </Suspense>
  );
}
