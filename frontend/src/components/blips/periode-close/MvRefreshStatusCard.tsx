"use client";

import * as React from "react";
import { RefreshCw, CheckCircle2, XCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MvRefreshStatusCardProps {
  /** Asynq job ID returned from hard-close approve response */
  jobId?: string | null;
  /** Called when MV refresh job completes successfully */
  onRefreshComplete?: () => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component — wraps JobProgressPanel for the post-hard-close MV refresh job
// ---------------------------------------------------------------------------

export function MvRefreshStatusCard({
  jobId,
  onRefreshComplete,
  className,
}: MvRefreshStatusCardProps) {
  const [completed, setCompleted] = React.useState(false);
  const [failed, setFailed] = React.useState(false);
  const [failError, setFailError] = React.useState<string | null>(null);

  const handleComplete = React.useCallback(() => {
    setCompleted(true);
    onRefreshComplete?.();
  }, [onRefreshComplete]);

  const handleFail = React.useCallback((error: unknown) => {
    setFailed(true);
    if (error && typeof error === "object" && "message" in error) {
      setFailError(String((error as { message: unknown }).message));
    } else {
      setFailError("MV refresh gagal. Coba trigger ulang secara manual via admin.");
    }
  }, []);

  // No job yet — show idle state
  if (!jobId) {
    return (
      <Card className={cn("border-dashed", className)}>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2 text-muted-foreground">
            <RefreshCw className="h-4 w-4" aria-hidden="true" />
            Refresh Materialized Views
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-xs text-muted-foreground">
            MV refresh akan berjalan otomatis setelah hard-close disetujui.
          </p>
        </CardContent>
      </Card>
    );
  }

  // Completed state
  if (completed) {
    return (
      <Card className={cn("border-green-300 bg-green-50", className)}>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2 text-green-800">
            <CheckCircle2 className="h-4 w-4 text-green-600" aria-hidden="true" />
            Refresh Materialized Views — Selesai
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-xs text-green-700">
            Semua MV laporan (rpt.mv_*) telah di-refresh. Data reporting sudah final.
          </p>
        </CardContent>
      </Card>
    );
  }

  // Failed state
  if (failed) {
    return (
      <Card className={cn("border-red-300 bg-red-50", className)}>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium flex items-center gap-2 text-red-800">
            <XCircle className="h-4 w-4 text-red-600" aria-hidden="true" />
            Refresh Materialized Views — Gagal
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {failError && <p className="text-xs text-red-700">{failError}</p>}
          <p className="text-xs text-red-600">
            Hard-close tetap berhasil. MV refresh bisa di-trigger ulang dari halaman{" "}
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 text-xs text-red-700 underline"
              onClick={() => window.location.assign("/jobs")}
              type="button"
            >
              Job History &rarr;
            </Button>
          </p>
        </CardContent>
      </Card>
    );
  }

  // Running — delegate to JobProgressPanel
  return (
    <Card className={cn(className)}>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium flex items-center gap-2">
          <RefreshCw className="h-4 w-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />
          Refresh Materialized Views — Berjalan
        </CardTitle>
      </CardHeader>
      <CardContent>
        <JobProgressPanel
          jobId={jobId}
          title="Refresh rpt.mv_* untuk laporan periode"
          onComplete={handleComplete}
          onFail={handleFail}
          showCancel={false}
          showBackground={false}
          variant="inline"
        />
      </CardContent>
    </Card>
  );
}
