"use client";

import * as React from "react";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import type { CommitJobResponse } from "@/lib/schemas/bulkupload.schema";

export interface BulkCommitProgressPanelProps {
  commitJob: CommitJobResponse;
  batchId: string;
  onComplete?: (result: unknown) => void;
  onFail?: (error: unknown) => void;
  className?: string;
}

/**
 * BulkCommitProgressPanel — wraps JobProgressPanel for bulk upload commit job.
 * Subscribes via SSE (/api/v1/jobs/{jobId}/stream) with polling fallback (2s).
 * S3-AC1: shows progress 0→100%, current step, ETA.
 */
export function BulkCommitProgressPanel({
  commitJob,
  batchId,
  onComplete,
  onFail,
  className,
}: BulkCommitProgressPanelProps) {
  return (
    <JobProgressPanel
      jobId={commitJob.jobId}
      title={`Commit Batch ${batchId}`}
      onComplete={onComplete}
      onFail={onFail}
      showCancel={false}
      showBackground={true}
      variant="panel"
      className={className}
    />
  );
}
