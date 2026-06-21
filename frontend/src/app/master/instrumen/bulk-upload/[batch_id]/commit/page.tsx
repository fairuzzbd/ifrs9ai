"use client";

/**
 * Route: /master/instrumen/bulk-upload/[batch_id]/commit
 * Story: P5-M11-S3 — Commit progress via JobProgressPanel SSE
 */

import * as React from "react";
import { Suspense } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { ArrowLeft, Send } from "lucide-react";

import { BulkCommitProgressPanel } from "@/components/blips/bulkupload/BulkCommitProgressPanel";
import { BulkBatchStatusBadge } from "@/components/blips/bulkupload/BulkBatchStatusBadge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  bulkBatchApi,
  bulkCommitApi,
  bulkUploadQueryKeys,
} from "@/lib/api/bulkupload.api";
import { notify } from "@/lib/notify";
import { ApiError } from "@/lib/api";
import type { CommitJobResponse } from "@/lib/schemas/bulkupload.schema";

function CommitContent() {
  const { batch_id: batchId } = useParams<{ batch_id: string }>();
  const router = useRouter();
  const qc = useQueryClient();
  const [committing, setCommitting] = React.useState(false);
  const [commitJob, setCommitJob] = React.useState<CommitJobResponse | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: bulkUploadQueryKeys.batchDetail(batchId),
    queryFn: () => bulkBatchApi.get(batchId),
    enabled: !!batchId,
    staleTime: 15_000,
  });

  const batch = data?.data;

  async function handleCommit() {
    setCommitting(true);
    try {
      const result = await bulkCommitApi.commit(batchId, uuidv4());
      setCommitJob(result.data);
      notify.info(`Commit job enqueued — ID: ${result.data.jobId}. Pantau progress di bawah.`);
    } catch (err) {
      if (err instanceof ApiError) notify.error(err);
      setCommitting(false);
    }
  }

  function handleJobComplete(result: unknown) {
    const r = result as { committedRows?: number; failedRows?: number };
    notify.success(
      `Commit selesai: ${r.committedRows ?? "—"} berhasil, ${r.failedRows ?? 0} gagal.`,
      { action: { label: "Lihat detail", onClick: () => router.push(`/master/instrumen/bulk-upload/${batchId}`) } },
    );
    void qc.invalidateQueries({ queryKey: bulkUploadQueryKeys.batches() });
    setCommitting(false);
  }

  function handleJobFail(error: unknown) {
    const e = error as { code?: string; message?: string; traceId?: string };
    notify.error({
      code: e.code ?? "INTERNAL",
      message: e.message ?? "Commit job gagal",
      traceId: e.traceId ?? "",
    });
    setCommitting(false);
  }

  if (isLoading) {
    return <div className="container mx-auto py-6 text-sm text-muted-foreground">Memuat...</div>;
  }

  const canCommit = batch?.status === "DRY_RUN_PASSED";

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
          <h1 className="text-xl font-bold">Commit Batch</h1>
          <p className="text-sm text-muted-foreground font-mono">{batchId}</p>
          {batch && <div className="mt-1"><BulkBatchStatusBadge status={batch.status} size="sm" /></div>}
        </div>
      </div>

      {!commitJob ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Enqueue Commit Job</CardTitle>
            <CardDescription>
              Asynq worker akan INSERT instrumen ke mst.instrumen secara per-baris.
              Baris gagal dilog, baris lain tetap diproses (partial commit OK).
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {batch && (
              <div className="grid grid-cols-2 gap-3 text-sm">
                <div>
                  <p className="text-xs text-muted-foreground">Total Baris</p>
                  <p className="font-semibold">{batch.totalRows}</p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Status</p>
                  <BulkBatchStatusBadge status={batch.status} size="sm" />
                </div>
                {batch.dryRunExpiresAt && (
                  <div className="col-span-2">
                    <p className="text-xs text-muted-foreground">DRY_RUN berlaku hingga</p>
                    <p className="text-xs font-mono text-amber-700">
                      {new Date(batch.dryRunExpiresAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}
                    </p>
                  </div>
                )}
              </div>
            )}

            {!canCommit && batch && (
              <div role="alert" className="text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3">
                Commit tidak tersedia — status batch harus DRY_RUN_PASSED. Status saat ini:{" "}
                <strong>{batch.status}</strong>
              </div>
            )}

            <Button
              onClick={handleCommit}
              disabled={!canCommit || committing}
              aria-busy={committing}
            >
              <Send className="h-4 w-4 mr-2" aria-hidden="true" />
              {committing ? "Mengirim..." : "Enqueue Commit Job"}
            </Button>
          </CardContent>
        </Card>
      ) : (
        <BulkCommitProgressPanel
          commitJob={commitJob}
          batchId={batchId}
          onComplete={handleJobComplete}
          onFail={handleJobFail}
        />
      )}
    </div>
  );
}

export default function CommitPage() {
  return (
    <Suspense>
      <CommitContent />
    </Suspense>
  );
}
