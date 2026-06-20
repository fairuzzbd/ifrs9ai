"use client";

import * as React from "react";
import { RefreshCw, Trash2, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { GlHostStatus } from "@/lib/schemas/gl-delivery.schema";
import { GL_HOST_STATUS_LABELS } from "@/lib/schemas/gl-delivery.schema";
import { GlDlqReplayDialog } from "./GlDlqReplayDialog";
import { GlDlqDiscardDialog } from "./GlDlqDiscardDialog";

// ---------------------------------------------------------------------------
// Status config
// ---------------------------------------------------------------------------

// DLQ entries only have FAILED or DEAD_LETTER as glHostStatus values
const STATUS_CONFIG: Partial<Record<
  GlHostStatus,
  {
    badgeVariant: "default" | "secondary" | "destructive" | "outline";
    colorClass: string;
  }
>> = {
  FAILED: {
    badgeVariant: "destructive",
    colorClass: "border-red-200 bg-red-50",
  },
  DEAD_LETTER: {
    badgeVariant: "destructive",
    colorClass: "border-gray-200 bg-gray-50",
  },
  RETRYING: {
    badgeVariant: "secondary",
    colorClass: "border-amber-200 bg-amber-50",
  },
  DELIVERED: {
    badgeVariant: "default",
    colorClass: "border-green-200 bg-green-50",
  },
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface GlDlqActionPanelProps {
  dlqId: string;
  jurnalNumber: string;
  status: GlHostStatus;
  retryCount: number;
  /** Max total manual retries from DLQ */
  maxRetry?: number;
  /** Only rendered for ROLE-AKUN-CTL, ROLE-CFO etc (check upstream) */
  canReplay: boolean;
  /** Only rendered when role = ROLE-IT-ADMIN — caller must NOT pass true for others */
  canDiscard: boolean;
  onReplay: () => void;
  onDiscard: () => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S5-AC1, S5-AC2, S5-AC3, S5-AC4)
// ---------------------------------------------------------------------------

export function GlDlqActionPanel({
  dlqId,
  jurnalNumber,
  status,
  retryCount,
  maxRetry = 5,
  canReplay,
  canDiscard,
  onReplay,
  onDiscard,
  className,
}: GlDlqActionPanelProps) {
  const [replayOpen, setReplayOpen] = React.useState(false);
  const [discardOpen, setDiscardOpen] = React.useState(false);

  const config = STATUS_CONFIG[status] ?? { badgeVariant: "outline" as const, colorClass: "border-gray-200 bg-gray-50" };
  const label = GL_HOST_STATUS_LABELS[status] ?? status;
  const isTerminal = status === "DELIVERED" || status === "DEAD_LETTER";

  return (
    <div className={cn("space-y-4", className)}>
      {/* Status */}
      <div
        className={cn(
          "flex items-center gap-2 rounded-md border px-3 py-2",
          config.colorClass,
        )}
        role="status"
        aria-live="polite"
        aria-label={`Status DLQ: ${label}`}
      >
        <Badge variant={config.badgeVariant}>{label}</Badge>
        <span className="ml-auto text-xs text-muted-foreground">
          {retryCount} / {maxRetry} retry
        </span>
      </div>

      {/* Action buttons */}
      {!isTerminal && (
        <div className="flex flex-wrap gap-2">
          {/* Replay — ROLE-AKUN-CTL / ROLE-CFO; only show when permitted */}
          {canReplay && status === "FAILED" && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setReplayOpen(true)}
              aria-label={`Replay DLQ entry untuk jurnal ${jurnalNumber}`}
            >
              <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
              Replay ke GL Host
            </Button>
          )}

          {/* Discard — ROLE-IT-ADMIN ONLY. If canDiscard is false, the button is NOT rendered. */}
          {canDiscard && status === "FAILED" && (
            <Button
              variant="outline"
              size="sm"
              className="border-destructive/40 text-destructive hover:bg-destructive/5"
              onClick={() => setDiscardOpen(true)}
              aria-label={`Discard DLQ entry untuk jurnal ${jurnalNumber}`}
            >
              <Trash2 className="mr-1.5 h-4 w-4" aria-hidden="true" />
              Discard
            </Button>
          )}

          {/* No permission message */}
          {!canReplay && !canDiscard && status === "FAILED" && (
            <div className="flex items-start gap-2 text-xs text-muted-foreground">
              <AlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" aria-hidden="true" />
              <p>
                Anda tidak memiliki izin untuk replay atau discard entri ini.
                Hubungi Finance Controller atau IT Admin.
              </p>
            </div>
          )}
        </div>
      )}

      {/* Terminal state message */}
      {isTerminal && (
        <p className="text-xs text-muted-foreground italic">
          {status === "DELIVERED"
            ? "Entry berhasil di-replay ke GL Host. Tidak ada aksi lebih lanjut."
            : "Entry sudah di-discard (DEAD_LETTER). Tidak ada aksi lebih lanjut."}
        </p>
      )}

      {/* Replay dialog */}
      <GlDlqReplayDialog
        open={replayOpen}
        onOpenChange={setReplayOpen}
        dlqId={dlqId}
        jurnalNumber={jurnalNumber}
        currentAttemptCount={retryCount}
        maxAttempts={maxRetry}
        onSuccess={() => {
          setReplayOpen(false);
          onReplay();
        }}
      />

      {/* Discard dialog — only mounted when canDiscard (ROLE-IT-ADMIN) */}
      {canDiscard && (
        <GlDlqDiscardDialog
          open={discardOpen}
          onOpenChange={setDiscardOpen}
          dlqId={dlqId}
          jurnalNumber={jurnalNumber}
          onSuccess={() => {
            setDiscardOpen(false);
            onDiscard();
          }}
        />
      )}
    </div>
  );
}
