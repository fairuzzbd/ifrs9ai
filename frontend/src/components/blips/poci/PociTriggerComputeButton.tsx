"use client";

import * as React from "react";
import { useState } from "react";
import { Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { notify } from "@/lib/notify";
import { pociComputeApi } from "@/lib/api/poci.api";
import { usePermissions } from "@/lib/stores/auth.store";

export interface PociTriggerComputeButtonProps {
  calcRunId: string;
  size?: "sm" | "default";
  onJobStarted?: (jobId: string, streamUrl: string) => void;
}

/**
 * Trigger POCI delta batch computation.
 * Absent-from-DOM for roles without poci.delta.compute permission
 * (ROLE-IT-ADMIN or ROLE-RISK only — S2 permission gate).
 * Returns 202 + jobId → parent can wire into JobProgressPanel (UX §3).
 */
export function PociTriggerComputeButton({
  calcRunId,
  size = "default",
  onJobStarted,
}: PociTriggerComputeButtonProps) {
  const perms = usePermissions();
  const [submitting, setSubmitting] = useState(false);

  // Absent-from-DOM persona gate — S2-AC1 permission enforcement
  if (!perms.can("poci.delta.compute")) {
    return null;
  }

  const handleTrigger = async () => {
    setSubmitting(true);
    try {
      const result = await pociComputeApi.triggerBatch(calcRunId);
      const job = result.data;
      notify.info(
        `POCI delta batch dimulai untuk calc run ${calcRunId.slice(0, 8)}. Pantau progres di Job Panel.`,
      );
      onJobStarted?.(job.jobId, job.streamUrl);
    } catch (err: unknown) {
      notify.error(err as { code: string; message: string; traceId: string });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Button
      size={size}
      variant="outline"
      disabled={submitting}
      onClick={() => void handleTrigger()}
      aria-label="Trigger komputasi delta POCI batch"
    >
      {submitting ? (
        <span className="flex items-center gap-1.5">
          <span className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" aria-hidden="true" />
          Memproses...
        </span>
      ) : (
        <span className="flex items-center gap-1.5">
          <Play className="h-4 w-4" aria-hidden="true" />
          Hitung Delta POCI
        </span>
      )}
    </Button>
  );
}
