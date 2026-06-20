"use client";

import * as React from "react";
import { PlayCircle } from "lucide-react";
import { v4 as uuidv4 } from "uuid";
import { Button } from "@/components/ui/button";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import { akrualCronApi } from "@/lib/api/akrual.api";
import { usePermissions } from "@/lib/stores/auth.store";

export interface AkrualCronTriggerButtonProps {
  tanggal?: string;
  jobTypes?: ("DAILY_ACCRUAL_JOB" | "AMORTISASI_PD_JOB")[];
  /** Callback when job is queued — caller can open JobProgressPanel */
  onJobQueued?: (jobId: string, statusUrl: string, streamUrl: string) => void;
  variant?: "outline" | "default" | "ghost";
  size?: "sm" | "default";
  label?: string;
}

/**
 * AkrualCronTriggerButton — absent-from-DOM unless ROLE-IT-ADMIN.
 * Manual trigger for DAILY_ACCRUAL_JOB / AMORTISASI_PD_JOB.
 * Returns 202 + jobId → caller handles JobProgressPanel (UX rule §3).
 */
export function AkrualCronTriggerButton({
  tanggal,
  jobTypes,
  onJobQueued,
  variant = "outline",
  size = "sm",
  label = "Trigger Cron Akrual",
}: AkrualCronTriggerButtonProps) {
  const perms = usePermissions();
  const [triggering, setTriggering] = React.useState(false);

  // Absent from DOM for non-ROLE-IT-ADMIN (permission gate)
  if (!perms.can("sys.cron.trigger")) {
    return null;
  }

  const handleTrigger = async () => {
    setTriggering(true);
    const idempotencyKey = uuidv4();
    try {
      const result = await akrualCronApi.trigger(
        { tanggal, jobTypes },
        idempotencyKey,
      );
      const { jobId, statusUrl, streamUrl } = result.data;
      notify.info(
        `Akrual cron job diantrekan (ID: ${jobId.slice(0, 12)}). Proses berjalan di background.`,
      );
      onJobQueued?.(jobId, statusUrl, streamUrl);
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal trigger cron job.", traceId: "" });
      }
    } finally {
      setTriggering(false);
    }
  };

  return (
    <Button
      variant={variant}
      size={size}
      disabled={triggering}
      onClick={() => void handleTrigger()}
      aria-label="Trigger manual akrual cron job (ROLE-IT-ADMIN)"
    >
      <PlayCircle className="h-4 w-4 mr-1" aria-hidden="true" />
      {triggering ? "Mengantre..." : label}
    </Button>
  );
}
