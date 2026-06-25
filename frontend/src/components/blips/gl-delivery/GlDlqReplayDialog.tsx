"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { RefreshCw, AlertTriangle, CheckCircle2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { glDeliveryDlqApi } from "@/lib/api/gl-delivery.api";
import {
  dlqActionRequestSchema,
  type DlqActionRequest,
} from "@/lib/schemas/gl-delivery.schema";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface GlDlqReplayDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dlqId: string;
  jurnalNumber: string;
  currentAttemptCount: number;
  maxAttempts?: number;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S5-AC2)
// ---------------------------------------------------------------------------

const MIN_CHARS = 30;

export function GlDlqReplayDialog({
  open,
  onOpenChange,
  dlqId,
  jurnalNumber,
  currentAttemptCount,
  maxAttempts = 5,
  onSuccess,
}: GlDlqReplayDialogProps) {
  const [succeeded, setSucceeded] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const {
    register,
    handleSubmit,
    watch,
    reset,
    formState: { errors },
  } = useForm<DlqActionRequest>({
    resolver: zodResolver(dlqActionRequestSchema),
    defaultValues: { reason: "" },
  });

  const reason = watch("reason");
  const charCount = reason.length;

  const handleOpenChange = (next: boolean) => {
    if (!next) {
      reset();
      setSucceeded(false);
    }
    onOpenChange(next);
  };

  const onSubmit = async (data: DlqActionRequest) => {
    setSubmitting(true);
    try {
      await glDeliveryDlqApi.replay(dlqId, data);
      setSucceeded(true);
      notify.success(
        `Replay DLQ berhasil dijadwalkan untuk jurnal ${jurnalNumber}. Pantau status di tabel DLQ.`,
      );
      onSuccess();
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="max-w-lg"
        aria-modal="true"
        aria-labelledby="dlq-replay-title"
      >
        <DialogHeader>
          <DialogTitle id="dlq-replay-title">
            Replay DLQ Entry ke GL Host
          </DialogTitle>
        </DialogHeader>

        {/* Success state */}
        {succeeded ? (
          <div className="space-y-4 py-2">
            <div className="flex items-start gap-3 rounded-md border border-green-200 bg-green-50 p-4 text-green-800">
              <CheckCircle2 className="h-5 w-5 shrink-0 mt-0.5" aria-hidden="true" />
              <div>
                <p className="font-medium">Replay berhasil dijadwalkan</p>
                <p className="text-sm mt-1">
                  {jurnalNumber} sedang antri untuk dikirim ulang ke GL Host via DLQ.
                  Status akan diperbarui di tabel DLQ.
                </p>
              </div>
            </div>
            <div className="flex justify-end">
              <Button onClick={() => handleOpenChange(false)}>Tutup</Button>
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            {/* Info */}
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
              <dt className="text-muted-foreground">Jurnal</dt>
              <dd className="font-mono font-medium">{jurnalNumber}</dd>
              <dt className="text-muted-foreground">DLQ ID</dt>
              <dd className="font-mono text-xs">{dlqId}</dd>
              <dt className="text-muted-foreground">Percobaan saat ini</dt>
              <dd>{currentAttemptCount} dari {maxAttempts}</dd>
            </dl>

            {/* Warning */}
            <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
              <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
              <div>
                <p className="font-medium">Sebelum replay:</p>
                <ul className="mt-1 list-disc list-inside text-xs space-y-0.5">
                  <li>Pastikan GL Host sudah dapat menerima payload ini.</li>
                  <li>Replay menggunakan idempotency key asli jurnal.</li>
                  <li>Jika jurnal sudah ter-deliver, GL Host akan mengembalikan hasil awal.</li>
                </ul>
              </div>
            </div>

            {/* Reason */}
            <div className="space-y-1">
              <Label htmlFor="dlq-replay-reason">
                Alasan Replay <span className="text-destructive">*</span>
              </Label>
              <Textarea
                id="dlq-replay-reason"
                rows={3}
                placeholder="Contoh: GL Host sudah kembali online setelah maintenance. Replay untuk pengiriman yang gagal tadi malam."
                aria-required="true"
                aria-describedby="dlq-replay-hint dlq-replay-error"
                disabled={submitting}
                {...register("reason")}
              />
              <div className="flex justify-between items-center">
                <p
                  id="dlq-replay-error"
                  className={cn(
                    "text-xs",
                    errors.reason ? "text-destructive" : "hidden",
                  )}
                  aria-live="polite"
                >
                  {errors.reason?.message}
                </p>
                <p
                  id="dlq-replay-hint"
                  className={cn(
                    "text-xs ml-auto",
                    charCount < MIN_CHARS ? "text-destructive" : "text-muted-foreground",
                  )}
                >
                  {charCount < MIN_CHARS
                    ? `Sisa: ${MIN_CHARS - charCount} karakter`
                    : `${charCount} karakter`}
                </p>
              </div>
            </div>

            {/* Actions */}
            <div className="flex justify-end gap-2 pt-1">
              <Button
                type="button"
                variant="ghost"
                onClick={() => handleOpenChange(false)}
                disabled={submitting}
              >
                Batal
              </Button>
              <Button
                type="submit"
                disabled={submitting || charCount < MIN_CHARS}
              >
                {submitting ? (
                  <>
                    <RefreshCw className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Menjadwalkan...
                  </>
                ) : (
                  <>
                    <RefreshCw className="mr-2 h-4 w-4" aria-hidden="true" />
                    Replay ke GL Host
                  </>
                )}
              </Button>
            </div>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
