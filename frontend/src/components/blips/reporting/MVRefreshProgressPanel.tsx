/**
 * MVRefreshProgressPanel — wraps JobProgressPanel for MV refresh jobs.
 * SSE subscriber → shows progress bar, ETA, cancel.
 * Used on /admin/mv-refresh after manual trigger (S2).
 */

"use client";

import * as React from "react";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { notify } from "@/lib/notify";
import { useRouter } from "next/navigation";

interface MVRefreshProgressPanelProps {
  jobId: string;
  mvName?: string | null;
  onDone?: () => void;
}

export function MVRefreshProgressPanel({
  jobId,
  mvName,
  onDone,
}: MVRefreshProgressPanelProps) {
  const router = useRouter();
  const label = mvName
    ? mvName.replace("rpt.mv_", "").replace(/_/g, " ")
    : "Semua MV";

  const handleComplete = React.useCallback(
    (result: unknown) => {
      notify.success(
        `Refresh Materialized View ${label} selesai.`,
        {
          action: {
            label: "Lihat status",
            onClick: () => router.push("/admin/mv-refresh"),
          },
        },
      );
      onDone?.();
    },
    [label, onDone, router],
  );

  const handleFail = React.useCallback(
    (error: unknown) => {
      notify.error({
        code: "MV_REFRESH_FAILED",
        message: `Refresh MV ${label} gagal.`,
        traceId: (error as { traceId?: string })?.traceId ?? "",
      });
      onDone?.();
    },
    [label, onDone],
  );

  return (
    <JobProgressPanel
      jobId={jobId}
      title={`Refresh MV — ${label}`}
      onComplete={handleComplete}
      onFail={handleFail}
      showCancel={false}
      showBackground={true}
      variant="panel"
    />
  );
}
