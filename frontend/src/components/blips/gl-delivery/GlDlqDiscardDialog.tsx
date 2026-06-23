"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Trash2, AlertTriangle } from "lucide-react";
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
// Props — only mounted/rendered for ROLE-IT-ADMIN (caller responsibility)
// ---------------------------------------------------------------------------

export interface GlDlqDiscardDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  dlqId: string;
  jurnalNumber: string;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S5-AC3, S5-AC4 — ROLE-IT-ADMIN only, destructive)
// ---------------------------------------------------------------------------

const MIN_CHARS = 30;

export function GlDlqDiscardDialog({
  open,
  onOpenChange,
  dlqId,
  jurnalNumber,
  onSuccess,
}: GlDlqDiscardDialogProps) {
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
    if (!next) reset();
    onOpenChange(next);
  };

  const onSubmit = async (data: DlqActionRequest) => {
    setSubmitting(true);
    try {
      await glDeliveryDlqApi.discard(dlqId, data);
      notify.success(
        `DLQ entry untuk jurnal ${jurnalNumber} berhasil di-discard. Status: DISCARDED.`,
      );
      handleOpenChange(false);
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
        aria-labelledby="dlq-discard-title"
      >
        <DialogHeader>
          <DialogTitle id="dlq-discard-title" className="text-destructive">
            Discard DLQ Entry
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
          {/* Destructive warning (always visible) */}
          <div
            className="flex items-start gap-3 rounded-md border border-red-300 bg-red-50 p-4 text-red-800"
            role="alert"
          >
            <AlertTriangle className="h-5 w-5 shrink-0 mt-0.5" aria-hidden="true" />
            <div className="text-sm">
              <p className="font-semibold">Aksi ini bersifat permanen dan tidak dapat dibatalkan.</p>
              <ul className="mt-2 list-disc list-inside text-xs space-y-1">
                <li>
                  Entry DLQ <span className="font-mono">{dlqId}</span> (jurnal{" "}
                  <span className="font-mono">{jurnalNumber}</span>) akan ditandai{" "}
                  <strong>DISCARDED</strong>.
                </li>
                <li>Jurnal ini tidak akan pernah dikirim ke GL Host secara otomatis lagi.</li>
                <li>Jika jurnal ini masih diperlukan, buat jurnal koreksi baru secara manual.</li>
                <li>Tindakan ini dicatat di audit log dengan nama Anda.</li>
              </ul>
            </div>
          </div>

          {/* Reason */}
          <div className="space-y-1">
            <Label htmlFor="dlq-discard-reason">
              Alasan Discard <span className="text-destructive">*</span>
            </Label>
            <Textarea
              id="dlq-discard-reason"
              rows={4}
              placeholder="Contoh: Jurnal JRNL-2026-0042 terduplikat karena error sistem. Versi yang benar sudah diposting manual dengan no JRNL-2026-0043. DLQ ini tidak perlu di-replay."
              aria-required="true"
              aria-describedby="dlq-discard-hint dlq-discard-error"
              disabled={submitting}
              className="border-red-200 focus-visible:ring-red-400"
              {...register("reason")}
            />
            <div className="flex justify-between items-center">
              <p
                id="dlq-discard-error"
                className={cn(
                  "text-xs",
                  errors.reason ? "text-destructive" : "hidden",
                )}
                aria-live="polite"
              >
                {errors.reason?.message}
              </p>
              <p
                id="dlq-discard-hint"
                className={cn(
                  "text-xs ml-auto",
                  charCount < MIN_CHARS ? "text-destructive" : "text-muted-foreground",
                )}
              >
                {charCount < MIN_CHARS
                  ? `Sisa: ${MIN_CHARS - charCount} karakter lagi`
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
              variant="destructive"
              disabled={submitting || charCount < MIN_CHARS}
            >
              {submitting ? (
                "Memproses..."
              ) : (
                <>
                  <Trash2 className="mr-2 h-4 w-4" aria-hidden="true" />
                  Ya, Discard Sekarang
                </>
              )}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
