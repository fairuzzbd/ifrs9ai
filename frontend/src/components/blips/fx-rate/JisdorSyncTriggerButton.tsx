"use client";

import * as React from "react";
import { RefreshCw, Loader2 } from "lucide-react";
import { v4 as uuidv4 } from "uuid";
import { Button } from "@/components/ui/button";
import { usePermissions } from "@/lib/stores/auth.store";
import { kursJisdorApi } from "@/lib/api/fx-rate.api";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import type { JisdorSyncJobResponse } from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface JisdorSyncTriggerButtonProps {
  /** Defaults to today (server TZ Asia/Jakarta) */
  tanggalTarget?: string;
  /** If true: force re-fetch even if APPROVED row exists (not locked) */
  forceRefetch?: boolean;
  onJobTriggered?: (job: JisdorSyncJobResponse) => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S1-AC1, persona-gated: ABSENT from DOM when unauthorized)
// ---------------------------------------------------------------------------

export function JisdorSyncTriggerButton({
  tanggalTarget,
  forceRefetch = false,
  onJobTriggered,
  className,
}: JisdorSyncTriggerButtonProps) {
  const perms = usePermissions();
  const [isTriggering, setIsTriggering] = React.useState(false);

  // ABSENT from DOM (not just disabled) when not ROLE-IT-ADMIN
  if (!perms.can("kurs.sync")) {
    return null;
  }

  const handleSync = async () => {
    setIsTriggering(true);
    try {
      const idempotencyKey = uuidv4();
      const result = await kursJisdorApi.triggerSync(
        {
          tanggalTarget: tanggalTarget || undefined,
          forceRefetch,
        },
        idempotencyKey,
      );

      const job = result.data;
      notify.success(
        `JISDOR sync dijadwalkan untuk ${job.tanggalTarget}. Estimasi ${job.estimatedCurrencies} mata uang. Monitor progres di panel di bawah.`,
        {
          action: {
            label: "Lihat status",
            onClick: () => window.open(job.statusUrl, "_blank"),
          },
        },
      );

      onJobTriggered?.(job);
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
      variant="outline"
      size="sm"
      onClick={() => void handleSync()}
      disabled={isTriggering}
      className={className}
      aria-label="Trigger JISDOR sync manual"
    >
      {isTriggering ? (
        <>
          <Loader2 className="mr-1.5 h-4 w-4 animate-spin" aria-hidden="true" />
          Memulai...
        </>
      ) : (
        <>
          <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
          Sync JISDOR
        </>
      )}
    </Button>
  );
}
