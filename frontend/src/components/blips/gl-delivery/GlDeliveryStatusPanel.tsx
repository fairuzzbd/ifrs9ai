"use client";

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { XCircle, RefreshCw, Loader2 } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { glDeliveryStatusApi } from "@/lib/api/gl-delivery.api";
import type { GlDeliveryStatus } from "@/lib/schemas/gl-delivery.schema";
import { usePermissions } from "@/lib/stores/auth.store";
import { GlStatusBadge } from "./GlStatusBadge";
import { GlFailureCategoryBadge } from "./GlFailureCategoryBadge";
import { GlDeliveryHistoryTimeline } from "./GlDeliveryHistoryTimeline";
import { RetryGlDeliveryDialog } from "./RetryGlDeliveryDialog";
import { JSONBTreeView } from "@/components/blips/JSONBTreeView";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const TERMINAL_STATUSES = new Set(["DELIVERED", "DEAD_LETTER"]);

function fmt(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("id-ID", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    timeZone: "Asia/Jakarta",
  });
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface GlDeliveryStatusPanelProps {
  jurnalHeaderId: string;
  jurnalNumber: string;
  /** Optional initial data from parent GET /jurnal/header/{id} */
  initialData?: GlDeliveryStatus | null;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC2, S2-AC3 — all 5 states)
// ---------------------------------------------------------------------------

const POLLING_INTERVAL = 10_000; // 10 seconds per §5.1 design spec

export function GlDeliveryStatusPanel({
  jurnalHeaderId,
  jurnalNumber,
  initialData,
  className,
}: GlDeliveryStatusPanelProps) {
  const qc = useQueryClient();
  const { can, hasRole } = usePermissions();
  const [retryOpen, setRetryOpen] = React.useState(false);

  const canRetryPermission = can("jurnal.gl_delivery.retry");
  const canViewRaw = hasRole("ROLE-IT-ADMIN");

  const { data, isLoading, error } = useQuery({
    queryKey: ["gl-delivery", "status", jurnalHeaderId],
    queryFn: () => glDeliveryStatusApi.getStatus(jurnalHeaderId),
    initialData: initialData
      ? { data: initialData, meta: { traceId: "" } }
      : undefined,
    refetchInterval: (query) => {
      const status = query.state.data?.data.glHostStatus;
      if (!status || TERMINAL_STATUSES.has(status)) return false;
      return POLLING_INTERVAL;
    },
    // Pause polling when tab is hidden
    refetchIntervalInBackground: false,
    staleTime: 5_000,
  });

  const status = data?.data;

  const handleRetrySuccess = () => {
    void qc.invalidateQueries({ queryKey: ["gl-delivery", "status", jurnalHeaderId] });
  };

  // ---- Loading state ----
  if (isLoading && !status) {
    return (
      <Card className={cn("mt-6", className)}>
        <CardContent className="pt-4 pb-3 space-y-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            Status Pengiriman ke GL Host
          </p>
          <div className="space-y-2 animate-pulse">
            <div className="h-4 w-48 rounded bg-muted" />
            <div className="h-3 w-80 rounded bg-muted" />
            <div className="h-3 w-64 rounded bg-muted" />
          </div>
        </CardContent>
      </Card>
    );
  }

  // ---- Error / no gl_status row ----
  if (error || !status) {
    return (
      <Card className={cn("mt-6 border-red-200", className)}>
        <CardContent className="pt-4 pb-3">
          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground mb-2">
            Status Pengiriman ke GL Host
          </p>
          {!status && !error ? (
            <p className="text-sm text-muted-foreground">
              Status pengiriman belum tersedia. Hubungi IT Admin.
            </p>
          ) : (
            <div className="flex items-center justify-between">
              <p className="text-sm text-red-700">
                Gagal memuat status pengiriman GL.
              </p>
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  void qc.invalidateQueries({
                    queryKey: ["gl-delivery", "status", jurnalHeaderId],
                  })
                }
              >
                Coba lagi
              </Button>
            </div>
          )}
        </CardContent>
      </Card>
    );
  }

  return (
    <div
      className={cn("mt-6 space-y-3", className)}
      aria-label="Status pengiriman ke GL Host"
    >
      <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        Status Pengiriman ke GL Host
      </p>

      {/* Main status card */}
      <Card>
        <CardContent className="pt-4 pb-4 space-y-3">
          {/* Header row: badge + failure category */}
          <div className="flex items-center gap-2 flex-wrap">
            <GlStatusBadge status={status.glHostStatus} />
            {status.failureCategory && (
              <GlFailureCategoryBadge category={status.failureCategory} />
            )}
          </div>

          {/* STATE A — DELIVERED */}
          {status.glHostStatus === "DELIVERED" && (
            <dl className="grid grid-cols-2 gap-x-8 gap-y-1 text-sm">
              <dt className="text-muted-foreground">GL Journal ID</dt>
              <dd className="font-mono">{status.glHostJournalId ?? "—"}</dd>
              <dt className="text-muted-foreground">Waktu Terkirim</dt>
              <dd>{fmt(status.deliveredAt)}</dd>
              <dt className="text-muted-foreground">Mode Pengiriman</dt>
              <dd>{status.deliveryMode}</dd>
              <dt className="text-muted-foreground">Jumlah Retry</dt>
              <dd>{status.retryCount}</dd>
            </dl>
          )}

          {/* STATE B — FAILED */}
          {status.glHostStatus === "FAILED" && (
            <>
              <dl className="grid grid-cols-2 gap-x-8 gap-y-1 text-sm">
                <dt className="text-muted-foreground">Penyebab</dt>
                <dd className="text-red-700 text-xs break-all">{status.lastError ?? "—"}</dd>
                <dt className="text-muted-foreground">Jumlah Retry</dt>
                <dd>{status.retryCount}</dd>
              </dl>

              <div className="flex items-start gap-2 rounded border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
                <XCircle className="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
                <p>
                  Jurnal ini gagal dikirim ke GL Host. Perbaiki penyebab kegagalan
                  sebelum melakukan retry.
                </p>
              </div>

              {/* Retry button — only rendered (not just hidden) when permitted */}
              {canRetryPermission && status.canRetry && (
                <div className="flex justify-end">
                  <Button
                    size="sm"
                    onClick={() => setRetryOpen(true)}
                    aria-label={`Retry pengiriman ke GL Host untuk jurnal ${jurnalNumber}`}
                  >
                    <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
                    Retry Pengiriman
                  </Button>
                </div>
              )}
            </>
          )}

          {/* STATE C — PENDING_DELIVERY */}
          {status.glHostStatus === "PENDING_DELIVERY" && (
            <p className="text-sm text-muted-foreground">
              Jurnal ini sudah diposting dan sedang antri untuk dikirim ke GL Host.
              Proses pengiriman otomatis berjalan dalam beberapa momen.
            </p>
          )}

          {/* STATE D — DELIVERY_IN_FLIGHT */}
          {status.glHostStatus === "DELIVERY_IN_FLIGHT" && (
            <div className="flex items-center gap-2 text-sm text-blue-700">
              <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
              <p>Worker sedang mengirim jurnal ke GL Host...</p>
            </div>
          )}

          {/* STATE D2 — RETRYING */}
          {status.glHostStatus === "RETRYING" && (
            <dl className="grid grid-cols-2 gap-x-8 gap-y-1 text-sm">
              <dt className="text-muted-foreground">Error Terakhir</dt>
              <dd className="text-amber-700 text-xs break-all">{status.lastError ?? "—"}</dd>
              <dt className="text-muted-foreground">Percobaan ke-</dt>
              <dd>{status.retryCount} dari 3 (maks. otomatis)</dd>
              <dt className="text-muted-foreground">Retry Terakhir</dt>
              <dd>{fmt(status.lastRetryAt)}</dd>
            </dl>
          )}

          {/* STATE E — DEAD_LETTER */}
          {status.glHostStatus === "DEAD_LETTER" && (
            <div className="space-y-2">
              <p className="text-sm">
                Entry ini sudah dihentikan secara permanen (DEAD_LETTER). Pengiriman
                otomatis tidak bisa dilakukan lagi.
              </p>
              <p className="text-xs text-muted-foreground">
                Jika jurnal ini masih perlu dikirim ke GL Host, buat jurnal koreksi
                baru via Posting Manual (CORRECTION_PERIODE_CLOSED).
              </p>
            </div>
          )}

          {/* Raw GL response — ROLE-IT-ADMIN only */}
          {canViewRaw && status.glResponsePayloadJsonb && (
            <details className="mt-2">
              <summary className="text-xs text-muted-foreground cursor-pointer">
                GL Host Raw Response (ROLE-IT-ADMIN)
              </summary>
              <div className="mt-2 p-2 border rounded bg-muted/20">
                <JSONBTreeView
                  data={status.glResponsePayloadJsonb}
                  maxDepth={3}
                  initiallyExpanded={false}
                />
              </div>
            </details>
          )}
        </CardContent>
      </Card>

      {/* Delivery history timeline — collapsible */}
      <GlDeliveryHistoryTimeline
        jurnalHeaderId={jurnalHeaderId}
        canViewRaw={canViewRaw}
      />

      {/* Retry dialog */}
      <RetryGlDeliveryDialog
        open={retryOpen}
        onOpenChange={setRetryOpen}
        jurnalHeaderId={jurnalHeaderId}
        jurnalNumber={jurnalNumber}
        lastError={status.lastError ?? null}
        failureCategory={status.failureCategory ?? null}
        currentAttemptCount={status.retryCount}
        maxAttempts={5}
        onSuccess={handleRetrySuccess}
      />
    </div>
  );
}
