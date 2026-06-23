"use client";

/**
 * Route: /master/instrumen/bulk-upload/[batch_id]
 * Story: P5-M11-S1-AC1, S2, S3, S4, S5
 * Batch detail + 4-stage validation status + row breakdown DataTable
 * Actor: ROLE-MAKER-TR (actions), ROLE-APPR-TR (approve), ROLE-CFO (rollback), ROLE-AUDIT (read)
 */

import * as React from "react";
import { Suspense } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { type ColumnDef } from "@tanstack/react-table";

import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { BulkBatchStatusBadge } from "@/components/blips/bulkupload/BulkBatchStatusBadge";
import { BulkRowStatusBadge } from "@/components/blips/bulkupload/BulkRowStatusBadge";
import { BulkSheetBreakdownCard } from "@/components/blips/bulkupload/BulkSheetBreakdownCard";
import { BulkApproveDialog } from "@/components/blips/bulkupload/BulkApproveDialog";
import { BulkRollbackRequestDialog } from "@/components/blips/bulkupload/BulkRollbackRequestDialog";
import { BulkRollbackApproveDialog } from "@/components/blips/bulkupload/BulkRollbackApproveDialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  bulkBatchApi,
  bulkDryRunApi,
  bulkApproveApi,
  bulkRollbackApi,
  bulkUploadQueryKeys,
  type BulkBatchListParams,
} from "@/lib/api/bulkupload.api";
import { notify } from "@/lib/notify";
import { ApiError } from "@/lib/api";
import type {
  BulkUploadRowItem,
  ApproveFormInput,
  RollbackRequestFormInput,
  RollbackApproveFormInput,
} from "@/lib/schemas/bulkupload.schema";

function BatchDetailContent() {
  const { batch_id: batchId } = useParams<{ batch_id: string }>();
  const router = useRouter();
  const qc = useQueryClient();

  const [rowParams, setRowParams] = React.useState<BulkBatchListParams>({
    limit: 50,
    sort: "row_number:asc",
  });
  const [cursorHistory, setCursorHistory] = React.useState<string[]>([""]);
  const [pageIndex, setPageIndex] = React.useState(0);

  const queryKey = bulkUploadQueryKeys.batchDetail(batchId, rowParams);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey,
    queryFn: () => bulkBatchApi.get(batchId, { ...rowParams, cursor: cursorHistory[pageIndex] || undefined }),
    staleTime: 15_000,
    enabled: !!batchId,
  });

  const batch = data?.data;
  const rows = data?.rows ?? [];
  const pagination = data?.pagination;

  // ─── Actions ──────────────────────────────────────────────────────────────

  async function handleDryRun() {
    try {
      const result = await bulkDryRunApi.run(batchId, uuidv4());
      await qc.invalidateQueries({ queryKey: bulkUploadQueryKeys.batches() });
      if (result.data.status === "DRY_RUN_PASSED") {
        notify.success(
          `DRY_RUN BATCH-${batchId} lulus — ${result.data.validRows ?? 0} baris valid, ${result.data.flaggedRows ?? 0} perlu review manual.`,
          { action: { label: "Lihat preview", onClick: () => router.push(`/master/instrumen/bulk-upload/${batchId}/dry-run`) } },
        );
      } else {
        notify.warning(`DRY_RUN gagal — ${result.data.invalidRows ?? 0} baris bermasalah. Lihat detail di preview.`);
      }
      router.push(`/master/instrumen/bulk-upload/${batchId}/dry-run`);
    } catch (err) {
      if (err instanceof ApiError) notify.error(err);
    }
  }

  async function handleApprove(body: ApproveFormInput) {
    try {
      const result = await bulkApproveApi.approve(batchId, body, uuidv4());
      await qc.invalidateQueries({ queryKey: bulkUploadQueryKeys.batches() });
      notify.success(
        `Batch ${batchId} disetujui. ${result.data.activatedCount} instrumen sekarang ACTIVE.`,
      );
      void refetch();
    } catch (err) {
      if (err instanceof ApiError) notify.error(err);
    }
  }

  async function handleRollbackRequest(body: RollbackRequestFormInput) {
    try {
      await bulkRollbackApi.request(batchId, body, uuidv4());
      await qc.invalidateQueries({ queryKey: bulkUploadQueryKeys.batches() });
      notify.info(`Rollback batch ${batchId} diajukan. Konfirmasi dengan step-up MFA untuk menyelesaikan.`);
      void refetch();
    } catch (err) {
      if (err instanceof ApiError) notify.error(err);
    }
  }

  async function handleRollbackApprove(body: RollbackApproveFormInput, stepUpToken: string) {
    try {
      const result = await bulkRollbackApi.approve(batchId, body, stepUpToken, uuidv4());
      await qc.invalidateQueries({ queryKey: bulkUploadQueryKeys.batches() });
      notify.destructive(
        `Batch ${batchId} di-rollback. ${result.data.rolledBackCount} instrumen soft-deleted. Audit log dicatat.`,
      );
      void refetch();
    } catch (err) {
      if (err instanceof ApiError) notify.error(err);
    }
  }

  // ─── Columns ──────────────────────────────────────────────────────────────

  const columns: ColumnDef<BulkUploadRowItem>[] = [
    {
      id: "rowNumber",
      header: ({ column }) => <SortHeader column={column} label="#" />,
      accessorKey: "rowNumber",
      cell: ({ row }) => <span className="font-mono text-xs">{row.original.rowNumber}</span>,
    },
    {
      id: "sheetName",
      header: ({ column }) => <SortHeader column={column} label="Sheet" />,
      accessorKey: "sheetName",
      cell: ({ row }) => <span className="text-xs font-mono">{row.original.sheetName}</span>,
    },
    {
      id: "rowStatus",
      header: "Status",
      cell: ({ row }) => <BulkRowStatusBadge status={row.original.rowStatus} size="sm" />,
    },
    {
      id: "instrumenId",
      header: "ID Instrumen",
      cell: ({ row }) => (
        <span className="font-mono text-xs text-muted-foreground">
          {row.original.instrumenId ? row.original.instrumenId.slice(0, 8) + "…" : "—"}
        </span>
      ),
    },
    {
      id: "error",
      header: "Error",
      cell: ({ row }) => {
        const err = row.original.rowErrorJsonb;
        if (!err) return null;
        const msg = typeof err === "object" && err !== null && "error" in err
          ? String((err as { error: string }).error)
          : JSON.stringify(err);
        return <span className="text-xs text-red-600 max-w-xs truncate block">{msg}</span>;
      },
    },
  ];

  if (!batchId) return null;

  return (
    <div className="container mx-auto py-6 space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between flex-wrap gap-4">
        <div>
          <p className="text-xs text-muted-foreground">Batch ID</p>
          <h1 className="text-xl font-bold font-mono">{batchId}</h1>
          {batch && (
            <div className="mt-1">
              <BulkBatchStatusBadge status={batch.status} />
            </div>
          )}
        </div>
        {/* Action buttons — persona-gated absent-from-DOM */}
        <div className="flex items-center gap-2 flex-wrap">
          {batch?.status === "PARSED" && (
            <Button onClick={handleDryRun} variant="secondary" aria-label="Jalankan DRY_RUN validasi">
              Jalankan DRY_RUN
            </Button>
          )}
          {batch?.status === "DRY_RUN_PASSED" && (
            <Button
              onClick={() => router.push(`/master/instrumen/bulk-upload/${batchId}/commit`)}
              aria-label="Lanjut ke halaman commit"
            >
              Lanjut ke Commit
            </Button>
          )}
          {batch?.status === "DRY_RUN_FAILED" && (
            <Button
              variant="outline"
              onClick={() => router.push(`/master/instrumen/bulk-upload/${batchId}/dry-run`)}
            >
              Lihat Detail Error
            </Button>
          )}
          {(batch?.status === "COMMITTED" || batch?.status === "PARTIAL_COMMIT") && (
            <BulkApproveDialog
              batchId={batchId}
              committedRows={batch?.committedRows ?? 0}
              makerUsername="(maker)"
              onApprove={handleApprove}
            />
          )}
          {batch?.status === "APPROVED" && (
            <BulkRollbackRequestDialog
              batchId={batchId}
              graceExpiresAt={batch.rollbackGraceExpiresAt}
              onRequest={handleRollbackRequest}
            />
          )}
          {batch?.status === "ROLLBACK_PENDING" && (
            <BulkRollbackApproveDialog
              batchId={batchId}
              rolledBackCount={batch?.committedRows ?? 0}
              onApprove={handleRollbackApprove}
            />
          )}
        </div>
      </div>

      {/* Sheet breakdown + batch stats */}
      {batch && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <BulkSheetBreakdownCard
            sheets={batch.sheets ?? {}}
            totalRows={batch.totalRows}
          />
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-sm">Statistik Batch</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-3 text-sm">
              {[
                { label: "Total Baris", value: batch.totalRows },
                { label: "Committed", value: batch.committedRows ?? "—" },
                { label: "Gagal", value: batch.failedRows ?? "—" },
                { label: "Perlu Review", value: batch.flaggedRows ?? "—" },
              ].map(({ label, value }) => (
                <div key={label}>
                  <p className="text-xs text-muted-foreground">{label}</p>
                  <p className="font-semibold">{String(value)}</p>
                </div>
              ))}
              {batch.dryRunExpiresAt && (
                <div className="col-span-2">
                  <p className="text-xs text-muted-foreground">DRY_RUN berlaku hingga</p>
                  <p className="text-xs font-mono">
                    {new Date(batch.dryRunExpiresAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      )}

      {/* Row breakdown DataTable */}
      <div className="space-y-2">
        <h2 className="text-base font-semibold">Breakdown Baris</h2>
        <DataTable
          columns={columns}
          data={rows}
          isLoading={isLoading}
          isError={isError}
          onRetry={() => void refetch()}
          sorting={[]}
          onSortingChange={() => {}}
          activeFilters={[]}
          onClearAllFilters={() => {
            setRowParams((p) => ({ ...p, row_status: undefined, sheet_name: undefined }));
          }}
          searchValue=""
          onSearchChange={() => {}}
          onExport={() => {}}
          exportFormats={["csv", "xlsx"]}
          pagination={{
            pageIndex,
            hasMore: pagination?.hasMore ?? false,
            totalEstimate: pagination?.totalEstimate ?? 0,
            limit: rowParams.limit ?? 50,
            onNext: () => {
              if (pagination?.nextCursor) {
                setCursorHistory((prev) => [...prev, pagination.nextCursor!]);
                setPageIndex((i) => i + 1);
              }
            },
            onPrev: () => {
              if (pageIndex > 0) {
                setCursorHistory((prev) => prev.slice(0, -1));
                setPageIndex((i) => i - 1);
              }
            },
          }}
          lastUpdated={new Date()}
          onRefresh={() => void refetch()}
          emptyMessage="Tidak ada baris yang cocok dengan filter."
        />
      </div>
    </div>
  );
}

export default function BatchDetailPage() {
  return (
    <Suspense>
      <BatchDetailContent />
    </Suspense>
  );
}
