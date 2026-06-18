"use client";

import * as React from "react";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { notify } from "@/lib/notify";
import type { JisdorSyncJobResponse } from "@/lib/schemas/fx-rate.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface JisdorJobProgressPanelProps {
  /** Job returned from JisdorSyncTriggerButton.onJobTriggered */
  job: JisdorSyncJobResponse | null;
  onComplete?: () => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component — JobProgressPanel wrapper for fx:jisdor-fetch Asynq task (§3)
// ---------------------------------------------------------------------------

export function JisdorJobProgressPanel({
  job,
  onComplete,
  className,
}: JisdorJobProgressPanelProps) {
  if (!job) return null;

  const handleComplete = (result: unknown) => {
    // Typing: result from worker has { currenciesInserted, currenciesSkipped }
    const r = result as { currenciesInserted?: number; currenciesSkipped?: number } | null;
    const inserted = r?.currenciesInserted ?? "?";
    const skipped = r?.currenciesSkipped ?? 0;

    notify.success(
      `JISDOR sync selesai untuk ${job.tanggalTarget}. ${String(inserted)} mata uang berhasil di-fetch${Number(skipped) > 0 ? `, ${String(skipped)} di-skip (sudah ada)` : ""}.`,
      {
        action: {
          label: "Lihat kurs",
          onClick: () => {
            window.location.href = `/master/kurs?filter[tanggal_berlaku]=eq:${job.tanggalTarget}&filter[sumber_kurs]=BI_JISDOR`;
          },
        },
      },
    );

    onComplete?.();
  };

  const handleFail = (error: unknown) => {
    const e = error as { message?: string; code?: string } | null;
    notify.error({
      code: e?.code ?? "JISDOR_WORKER_FAILED",
      message:
        e?.message ??
        `JISDOR sync gagal untuk ${job.tanggalTarget}. Periksa DLQ atau coba lagi.`,
      traceId: "",
    });
  };

  return (
    <div className={cn("rounded-md border bg-card", className)}>
      <JobProgressPanel
        jobId={job.jobId}
        title={`JISDOR Sync — ${job.tanggalTarget} (${job.estimatedCurrencies} mata uang)`}
        onComplete={handleComplete}
        onFail={handleFail}
        showCancel={false}
        showBackground={true}
        variant="panel"
      />
    </div>
  );
}
