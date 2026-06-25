"use client";

/**
 * Route: /master/instrumen/bulk-upload/[batch_id]/dry-run
 * Story: P5-M11-S2 — Preview panel (stage_summary + errors_per_row)
 */

import * as React from "react";
import { Suspense } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { ArrowLeft, Play } from "lucide-react";

import { BulkDryRunResultPanel } from "@/components/blips/bulkupload/BulkDryRunResultPanel";
import { BulkBatchStatusBadge } from "@/components/blips/bulkupload/BulkBatchStatusBadge";
import { Button } from "@/components/ui/button";
import { bulkBatchApi, bulkDryRunApi, bulkUploadQueryKeys } from "@/lib/api/bulkupload.api";
import { notify } from "@/lib/notify";
import { ApiError } from "@/lib/api";

function DryRunContent() {
  const { batch_id: batchId } = useParams<{ batch_id: string }>();
  const router = useRouter();
  const [running, setRunning] = React.useState(false);

  const { data, isLoading, refetch } = useQuery({
    queryKey: bulkUploadQueryKeys.batchDetail(batchId),
    queryFn: () => bulkBatchApi.get(batchId),
    enabled: !!batchId,
    staleTime: 15_000,
  });

  const batch = data?.data;
  const dryRunResult = batch
    ? {
        status: batch.status === "DRY_RUN_PASSED" ? ("DRY_RUN_PASSED" as const) : ("DRY_RUN_FAILED" as const),
        totalRows: batch.totalRows,
        validRows: (batch.totalRows ?? 0) - (batch.failedRows ?? 0) - (batch.flaggedRows ?? 0),
        invalidRows: batch.failedRows ?? 0,
        flaggedRows: batch.flaggedRows ?? 0,
        errorsPerRow: [],
        dryRunExpiresAt: batch.dryRunExpiresAt ?? null,
      }
    : null;

  async function handleReRunDryRun() {
    setRunning(true);
    try {
      const result = await bulkDryRunApi.run(batchId, uuidv4());
      notify.success(`DRY_RUN selesai — status: ${result.data.status}`);
      void refetch();
    } catch (err) {
      if (err instanceof ApiError) notify.error(err);
    } finally {
      setRunning(false);
    }
  }

  if (isLoading) {
    return <div className="container mx-auto py-6 text-sm text-muted-foreground">Memuat...</div>;
  }

  return (
    <div className="container mx-auto py-6 space-y-6">
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push(`/master/instrumen/bulk-upload/${batchId}`)}
          aria-label="Kembali ke detail batch"
        >
          <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
          Kembali
        </Button>
      </div>

      <div className="flex items-start justify-between flex-wrap gap-4">
        <div>
          <h1 className="text-xl font-bold">Preview DRY_RUN</h1>
          <p className="text-sm text-muted-foreground font-mono">{batchId}</p>
          {batch && <div className="mt-1"><BulkBatchStatusBadge status={batch.status} size="sm" /></div>}
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleReRunDryRun}
            disabled={running}
            aria-busy={running}
          >
            <Play className="h-4 w-4 mr-1" aria-hidden="true" />
            {running ? "Menjalankan..." : "Ulangi DRY_RUN"}
          </Button>
          {batch?.status === "DRY_RUN_PASSED" && (
            <Button
              size="sm"
              onClick={() => router.push(`/master/instrumen/bulk-upload/${batchId}/commit`)}
            >
              Lanjut ke Commit
            </Button>
          )}
        </div>
      </div>

      {dryRunResult ? (
        <BulkDryRunResultPanel result={dryRunResult} />
      ) : (
        <p className="text-sm text-muted-foreground">
          DRY_RUN belum pernah dijalankan untuk batch ini. Klik &ldquo;Ulangi DRY_RUN&rdquo; untuk memulai.
        </p>
      )}
    </div>
  );
}

export default function DryRunPage() {
  return (
    <Suspense>
      <DryRunContent />
    </Suspense>
  );
}
