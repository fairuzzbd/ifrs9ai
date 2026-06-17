"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, XCircle, Loader2, AlertTriangle, ExternalLink } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogClose,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { periodeChecklistApi } from "@/lib/api/periode-close.api";
import {
  CHECKLIST_TRANSITION_LABELS,
  CHECKLIST_ITEM_LABELS,
} from "@/lib/schemas/periode-close.schema";
import Link from "next/link";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ChecklistSnapshotDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  snapshotId: string;
  /** Optional: displayed in dialog title for context */
  periodeKode?: string;
}

// ---------------------------------------------------------------------------
// Component (S5-AC1, S5-AC4)
// ---------------------------------------------------------------------------

function fmt(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("id-ID", {
    day: "2-digit",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "Asia/Jakarta",
  });
}

export function ChecklistSnapshotDetailDialog({
  open,
  onOpenChange,
  periodeId,
  snapshotId,
  periodeKode: _periodeKode,
}: ChecklistSnapshotDetailDialogProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: ["periode-buku", "snapshot", periodeId, snapshotId],
    queryFn: () => periodeChecklistApi.getSnapshot(periodeId, snapshotId),
    enabled: open && !!snapshotId,
    staleTime: 5 * 60 * 1000, // 5 min (snapshots are immutable)
  });

  const snapshot = data?.data;

  const transitionLabel = snapshot
    ? (CHECKLIST_TRANSITION_LABELS[snapshot.transition] ?? snapshot.transition)
    : "Snapshot";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-2xl max-h-[90vh] overflow-y-auto"
        aria-labelledby="snapshot-dialog-title"
      >
        <DialogHeader>
          <DialogTitle id="snapshot-dialog-title">
            Snapshot Checklist — {transitionLabel}
          </DialogTitle>
        </DialogHeader>

        {/* Loading */}
        {isLoading && (
          <div className="flex items-center justify-center py-12 gap-2 text-muted-foreground">
            <Loader2 className="h-5 w-5 animate-spin" aria-hidden="true" />
            <span>Memuat snapshot...</span>
          </div>
        )}

        {/* Error */}
        {isError && (
          <div className="flex flex-col items-center py-8 gap-3 text-center">
            <AlertTriangle className="h-6 w-6 text-destructive" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">
              Gagal memuat snapshot. Coba tutup dan buka kembali.
            </p>
          </div>
        )}

        {/* Content */}
        {snapshot && (
          <div className="space-y-4">
            {/* Header info */}
            <dl className="grid grid-cols-[160px_1fr] gap-x-4 gap-y-2 text-sm">
              <dt className="text-muted-foreground">Snapshot ID</dt>
              <dd className="font-mono text-xs">{snapshot.id}</dd>

              <dt className="text-muted-foreground">Transisi</dt>
              <dd>
                <Badge variant="outline" className="text-xs">
                  {transitionLabel}
                </Badge>
              </dd>

              <dt className="text-muted-foreground">Status</dt>
              <dd>
                <Badge
                  variant="outline"
                  className={cn(
                    "text-xs",
                    snapshot.transitionStatus === "APPROVED"
                      ? "bg-green-50 text-green-700 border-green-300"
                      : "bg-red-50 text-red-700 border-red-300",
                  )}
                >
                  {snapshot.transitionStatus === "APPROVED" ? "Disetujui" : "Ditolak"}
                </Badge>
              </dd>

              <dt className="text-muted-foreground">Evaluator</dt>
              <dd>
                {snapshot.actorName ?? snapshot.actorUserId} ({snapshot.actorRole})
              </dd>

              <dt className="text-muted-foreground">Dievaluasi</dt>
              <dd>{fmt(snapshot.checklist.evaluatedAt)}</dd>
            </dl>

            {/* Divider */}
            <hr />

            {/* Checklist items */}
            <div>
              <p className="text-sm font-medium mb-2">
                Hasil Checklist (snapshot saat transisi)
              </p>
              <div role="list" aria-label="Item checklist snapshot">
                {snapshot.checklist.items.map((item) => {
                  const label = CHECKLIST_ITEM_LABELS[item.key] ?? item.label;
                  return (
                    <div
                      key={item.key}
                      className="flex items-start gap-3 py-2.5 border-b border-border last:border-0"
                      role="listitem"
                    >
                      <div className="mt-0.5 shrink-0">
                        {item.passed ? (
                          <CheckCircle2
                            className="h-4 w-4 text-green-600"
                            aria-label="Lolos"
                          />
                        ) : (
                          <XCircle
                            className="h-4 w-4 text-red-600"
                            aria-label="Gagal"
                          />
                        )}
                      </div>
                      <div className="flex-1 min-w-0">
                        <p className="text-sm">{label}</p>
                        {item.detail && (
                          <p className="text-xs text-muted-foreground mt-0.5">
                            {item.detail}
                          </p>
                        )}
                        {!item.passed && item.actionUrl && (
                          <Link
                            href={item.actionUrl}
                            onClick={() => onOpenChange(false)}
                            className="inline-flex items-center gap-1 text-xs text-blue-600 hover:text-blue-800 mt-1 focus:outline-none focus-visible:ring-2 focus-visible:ring-blue-500 rounded"
                          >
                            <ExternalLink className="h-3 w-3" aria-hidden="true" />
                            Lihat DLQ GL Delivery
                          </Link>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>

            {/* Stale re-run indicator */}
            {snapshot.isStaleRerun && (
              <>
                <hr />
                <div className="rounded-md bg-amber-50 border border-amber-200 px-3 py-2 flex items-start gap-2">
                  <AlertTriangle className="h-4 w-4 text-amber-600 mt-0.5 shrink-0" aria-hidden="true" />
                  <p className="text-xs text-amber-700">
                    Snapshot ini adalah re-evaluasi karena checklist sebelumnya sudah lebih dari 24 jam
                    sejak request diajukan.
                  </p>
                </div>
              </>
            )}

            {/* Footer */}
            <div className="flex justify-end pt-2">
              <DialogClose asChild>
                <Button variant="outline">Tutup</Button>
              </DialogClose>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
