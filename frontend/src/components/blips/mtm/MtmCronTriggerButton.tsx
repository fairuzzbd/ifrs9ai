"use client";

import * as React from "react";
import { Play, Loader2 } from "lucide-react";
import { v4 as uuidv4 } from "uuid";
import { Button } from "@/components/ui/button";
import { usePermissions } from "@/lib/stores/auth.store";
import { mtmCronApi } from "@/lib/api/mtm.api";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import type { MtmCronJobResponse } from "@/lib/schemas/mtm.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmCronTriggerButtonProps {
  /** Tanggal target for the MTM run (YYYY-MM-DD). Default: today (server TZ Jakarta). */
  tanggalTarget?: string;
  /** If true: force re-run instruments that are already AUTO_POSTED or APPROVED. */
  forceRerun?: boolean;
  /** Callback when Asynq job is successfully enqueued — caller renders <JobProgressPanel> */
  onJobStarted?: (job: MtmCronJobResponse) => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component — ABSENT from DOM when caller does not have mtm.trigger permission
// (S1-AC1, S1-AC4: only ROLE-IT-ADMIN can manually trigger)
// ---------------------------------------------------------------------------

export function MtmCronTriggerButton({
  tanggalTarget,
  forceRerun = false,
  onJobStarted,
  className,
}: MtmCronTriggerButtonProps) {
  const perms = usePermissions();
  const [isTriggering, setIsTriggering] = React.useState(false);

  // ABSENT from DOM (not just disabled) when not ROLE-IT-ADMIN / mtm.trigger
  if (!perms.can("mtm.trigger")) {
    return null;
  }

  const handleTrigger = async () => {
    setIsTriggering(true);
    try {
      const idempotencyKey = uuidv4();
      const result = await mtmCronApi.trigger(
        {
          tanggalTarget: tanggalTarget || undefined,
          forceRerun,
        },
        idempotencyKey,
      );

      const job = result.data;
      notify.info(
        `MTM cron dijadwalkan untuk ${job.tanggalTarget}. Estimasi ${job.estimatedInstrumen} instrumen. Pantau progres di panel di bawah.`,
      );

      onJobStarted?.(job);
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      } else {
        notify.error({
          code: "NETWORK_ERROR",
          message: "Gagal menghubungi server.",
          traceId: "",
        });
      }
    } finally {
      setIsTriggering(false);
    }
  };

  return (
    <Button
      variant="default"
      size="sm"
      onClick={() => void handleTrigger()}
      disabled={isTriggering}
      className={className}
      aria-label="Jalankan MTM cron manual (ROLE-IT-ADMIN)"
    >
      {isTriggering ? (
        <>
          <Loader2 className="mr-1.5 h-4 w-4 animate-spin" aria-hidden="true" />
          Memulai...
        </>
      ) : (
        <>
          <Play className="mr-1.5 h-4 w-4" aria-hidden="true" />
          Jalankan MTM Cron
        </>
      )}
    </Button>
  );
}
