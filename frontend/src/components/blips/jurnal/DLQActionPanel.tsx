"use client";

import * as React from "react";
import { RefreshCw, Archive, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { DlqStatus } from "@/lib/schemas/jurnal.schema";

export interface DLQActionPanelProps {
  dlqId: string;
  status: DlqStatus;
  retryCount: number;
  maxRetry?: number;
  canReplay: boolean;
  canDiscard: boolean;
  onReplay: () => Promise<void>;
  onDiscard: (reason: string) => Promise<void>;
  className?: string;
}

const STATUS_LABEL: Record<DlqStatus, string> = {
  FAILED: "GAGAL",
  REPLAYING: "SEDANG DIULANG",
  REPLAYED_OK: "BERHASIL DIULANG",
  ABANDONED: "DIABAIKAN",
};

const STATUS_COLOR: Record<DlqStatus, string> = {
  FAILED: "border-red-200 bg-red-50 text-red-700",
  REPLAYING: "border-amber-200 bg-amber-50 text-amber-700",
  REPLAYED_OK: "border-green-200 bg-green-50 text-green-700",
  ABANDONED: "border-gray-200 bg-gray-50 text-gray-500",
};

export function DLQActionPanel({
  dlqId: _dlqId,
  status,
  retryCount,
  maxRetry = 3,
  canReplay,
  canDiscard,
  onReplay,
  onDiscard,
  className,
}: DLQActionPanelProps) {
  const [mode, setMode] = React.useState<"idle" | "confirm-replay" | "discard">("idle");
  const [discardReason, setDiscardReason] = React.useState("");
  const [submitting, setSubmitting] = React.useState(false);
  const [replayConfirmed, setReplayConfirmed] = React.useState(false);

  const MIN_DISCARD_CHARS = 30;

  const wrap = async (fn: () => Promise<void>) => {
    setSubmitting(true);
    try {
      await fn();
      setMode("idle");
      setDiscardReason("");
      setReplayConfirmed(false);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className={cn("space-y-4", className)}>
      {/* Status badge */}
      <div
        className={cn(
          "flex items-center gap-2 rounded-md border px-3 py-2 text-sm font-medium",
          STATUS_COLOR[status],
        )}
        role="status"
        aria-live="polite"
      >
        <span>Status: {STATUS_LABEL[status]}</span>
        <span className="ml-auto text-xs font-normal opacity-75">
          Retry {retryCount} / {maxRetry}
        </span>
      </div>

      {/* Idle state — show action buttons */}
      {mode === "idle" && (
        <div className="flex flex-wrap gap-2">
          {canReplay && status === "FAILED" && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setMode("confirm-replay")}
              disabled={submitting}
              aria-label="Replay — ulangi posting jurnal"
            >
              <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
              Replay
            </Button>
          )}
          {canDiscard && (status === "FAILED" || status === "REPLAYING") && (
            <Button
              variant="outline"
              size="sm"
              className="border-destructive/40 text-destructive hover:bg-destructive/5"
              onClick={() => setMode("discard")}
              disabled={submitting}
              aria-label="Discard — abaikan dan tandai ABANDONED"
            >
              <Archive className="mr-1.5 h-4 w-4" aria-hidden="true" />
              Discard
            </Button>
          )}
          {!canReplay && !canDiscard && status === "FAILED" && (
            <p className="text-xs text-muted-foreground">
              Anda tidak memiliki izin untuk replay atau discard entri ini.
            </p>
          )}
          {(status === "REPLAYED_OK" || status === "ABANDONED") && (
            <p className="text-xs text-muted-foreground italic">
              {status === "REPLAYED_OK"
                ? "Jurnal berhasil di-posting ulang. Tidak ada aksi lebih lanjut."
                : "Entri ini sudah diabaikan (ABANDONED). Tidak ada aksi lebih lanjut."}
            </p>
          )}
        </div>
      )}

      {/* Confirm replay */}
      {mode === "confirm-replay" && (
        <div className="rounded-md border border-amber-200 bg-amber-50 p-4 space-y-3">
          <div className="flex items-start gap-2 text-sm text-amber-800">
            <AlertTriangle className="h-4 w-4 mt-0.5 shrink-0" aria-hidden="true" />
            <div>
              <p className="font-medium">Konfirmasi Replay</p>
              <p className="mt-1 text-xs">
                Replay akan mencoba posting jurnal ulang dengan idempotency key yang sama. Jika jurnal
                sudah ter-posting (duplicate), sistem akan mengembalikan hasil awal (idempotency replay).
              </p>
            </div>
          </div>

          <label className="flex items-start gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={replayConfirmed}
              onChange={(e) => setReplayConfirmed(e.target.checked)}
              disabled={submitting}
              className="mt-0.5 h-4 w-4"
              aria-label="Saya memahami efek replay"
            />
            <span className="text-xs text-amber-800">
              Saya memahami bahwa replay akan mengeksekusi ulang handler jurnal ini.
            </span>
          </label>

          <div className="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => { setMode("idle"); setReplayConfirmed(false); }}
              disabled={submitting}
            >
              Batal
            </Button>
            <Button
              size="sm"
              disabled={submitting || !replayConfirmed}
              onClick={() => wrap(onReplay)}
            >
              {submitting ? (
                <>
                  <RefreshCw className="mr-1.5 h-4 w-4 animate-spin" aria-hidden="true" />
                  Memutar ulang...
                </>
              ) : (
                <>
                  <RefreshCw className="mr-1.5 h-4 w-4" aria-hidden="true" />
                  Ya, Replay Sekarang
                </>
              )}
            </Button>
          </div>
        </div>
      )}

      {/* Discard form */}
      {mode === "discard" && (
        <div className="rounded-md border border-red-200 bg-red-50 p-4 space-y-3">
          <div className="flex items-start gap-2 text-sm text-red-800">
            <Archive className="h-4 w-4 mt-0.5 shrink-0" aria-hidden="true" />
            <div>
              <p className="font-medium">Discard — Tandai ABANDONED</p>
              <p className="mt-1 text-xs">
                Aksi ini bersifat final. Entri DLQ akan ditandai ABANDONED dan tidak bisa di-replay lagi.
                Hanya ROLE-IT-ADMIN yang dapat melakukan discard.
              </p>
            </div>
          </div>

          <div className="space-y-1">
            <Label
              htmlFor="discard-reason"
              className="text-xs text-red-800"
            >
              Alasan Discard <span className="text-destructive">*</span>
              <span className="ml-1 text-red-600 opacity-80">
                (min {MIN_DISCARD_CHARS} karakter)
              </span>
            </Label>
            <Textarea
              id="discard-reason"
              rows={3}
              value={discardReason}
              onChange={(e) => setDiscardReason(e.target.value)}
              placeholder="Jelaskan mengapa entri ini diabaikan dan tidak perlu di-replay..."
              disabled={submitting}
              className="border-red-200 focus:border-red-400 bg-white"
              aria-required="true"
              aria-describedby="discard-char-count"
            />
            <p
              id="discard-char-count"
              className={cn(
                "text-right text-xs",
                discardReason.length < MIN_DISCARD_CHARS ? "text-destructive" : "text-muted-foreground",
              )}
            >
              {discardReason.length} / {MIN_DISCARD_CHARS} minimum
            </p>
          </div>

          <div className="flex gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => { setMode("idle"); setDiscardReason(""); }}
              disabled={submitting}
            >
              Batal
            </Button>
            <Button
              variant="destructive"
              size="sm"
              disabled={submitting || discardReason.length < MIN_DISCARD_CHARS}
              onClick={() => wrap(() => onDiscard(discardReason))}
            >
              {submitting ? (
                "Memproses..."
              ) : (
                <>
                  <Archive className="mr-1.5 h-4 w-4" aria-hidden="true" />
                  Ya, Abandoned
                </>
              )}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
