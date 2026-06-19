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
import { glDeliveryRetryApi } from "@/lib/api/gl-delivery.api";
import {
  retryGlDeliveryRequestSchema,
  type RetryGlDeliveryRequest,
  type GlFailureCategory,
} from "@/lib/schemas/gl-delivery.schema";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { cn } from "@/lib/utils";
import { GlFailureCategoryBadge as FailureBadge } from "./GlFailureCategoryBadge";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RetryGlDeliveryDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  jurnalHeaderId: string;
  jurnalNumber: string;
  lastError: string | null;
  failureCategory: GlFailureCategory | null;
  currentAttemptCount: number;
  maxAttempts?: number;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S3-AC1, S3-AC2, S3-AC3, S3-AC4)
// ---------------------------------------------------------------------------

const MIN_CHARS = 30;

export function RetryGlDeliveryDialog({
  open,
  onOpenChange,
  jurnalHeaderId,
  jurnalNumber,
  lastError,
  failureCategory,
  currentAttemptCount,
  maxAttempts = 5,
  onSuccess,
}: RetryGlDeliveryDialogProps) {
  const [succeeded, setSucceeded] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const {
    register,
    handleSubmit,
    watch,
    reset,
    setError,
    formState: { errors },
  } = useForm<RetryGlDeliveryRequest>({
    resolver: zodResolver(retryGlDeliveryRequestSchema),
    defaultValues: { reason: "" },
  });

  const reason = watch("reason");
  const charCount = reason.length;
  const maxAttemptsReached = currentAttemptCount >= maxAttempts;

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      reset();
      setSucceeded(false);
    }
    onOpenChange(nextOpen);
  };

  const onSubmit = async (data: RetryGlDeliveryRequest) => {
    if (maxAttemptsReached) return;
    setSubmitting(true);
    try {
      await glDeliveryRetryApi.retry(jurnalHeaderId, data);
      setSucceeded(true);
      notify.success(
        `Retry delivery ${jurnalNumber} berhasil dijadwalkan. Pantau status di panel jurnal ini.`,
      );
      onSuccess();
      // Keep dialog open to show success state — user closes manually
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "GL_DELIVERY_MAX_ATTEMPTS_EXCEEDED") {
          // Show constraint inline
          setError("root", { message: err.message });
        } else {
          notify.error(err);
        }
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
        aria-labelledby="retry-dialog-title"
      >
        <DialogHeader>
          <DialogTitle id="retry-dialog-title">
            Retry Pengiriman ke GL Host
          </DialogTitle>
        </DialogHeader>

        {/* Success state */}
        {succeeded ? (
          <div className="space-y-4 py-2">
            <div className="flex items-start gap-3 rounded-md border border-green-200 bg-green-50 p-4 text-green-800">
              <CheckCircle2 className="h-5 w-5 shrink-0 mt-0.5" aria-hidden="true" />
              <div>
                <p className="font-medium">Retry berhasil dijadwalkan</p>
                <p className="text-sm mt-1">
                  {jurnalNumber} sedang antri untuk dikirim ulang ke GL Host.
                  Status akan diperbarui otomatis di panel jurnal ini.
                </p>
              </div>
            </div>
            <div className="flex justify-end">
              <Button onClick={() => handleOpenChange(false)}>Tutup</Button>
            </div>
          </div>
        ) : (
          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4">
            {/* Summary */}
            <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-sm">
              <dt className="text-muted-foreground">Jurnal</dt>
              <dd className="font-mono font-medium">{jurnalNumber}</dd>
              {lastError && (
                <>
                  <dt className="text-muted-foreground">Error Terakhir</dt>
                  <dd className="text-red-700 text-xs">{lastError}</dd>
                </>
              )}
              {failureCategory && (
                <>
                  <dt className="text-muted-foreground">Kategori</dt>
                  <dd>
                    <FailureBadge category={failureCategory} />
                  </dd>
                </>
              )}
              <dt className="text-muted-foreground">Percobaan</dt>
              <dd>
                {currentAttemptCount} dari {maxAttempts} (maks.)
              </dd>
            </dl>

            {/* Warning banner */}
            <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
              <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
              <div>
                <p className="font-medium">
                  Pastikan kondisi penyebab kegagalan sudah diperbaiki:
                </p>
                <ul className="mt-1 list-disc list-inside text-xs space-y-0.5">
                  <li>Domain error: akun GL Host sudah diperbaiki?</li>
                  <li>Infra error: GL Host sudah kembali online?</li>
                </ul>
              </div>
            </div>

            {/* Max attempts constraint */}
            {(maxAttemptsReached || errors.root) && (
              <div
                className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-800"
                role="alert"
              >
                <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5" aria-hidden="true" />
                <div>
                  {maxAttemptsReached ? (
                    <>
                      <p className="font-medium">
                        Batas maksimum percobaan tercapai ({maxAttempts}/{maxAttempts}).
                      </p>
                      <p className="text-xs mt-0.5">
                        Retry tidak bisa dilakukan lagi. Jika jurnal ini masih perlu
                        dikirim, hubungi ROLE-IT-ADMIN untuk mendiscard entry DLQ dan
                        membuat jurnal koreksi.
                      </p>
                    </>
                  ) : (
                    <p>{errors.root?.message}</p>
                  )}
                </div>
              </div>
            )}

            {/* Reason textarea */}
            {!maxAttemptsReached && (
              <div className="space-y-1">
                <Label htmlFor="retry-reason">
                  Alasan Retry <span className="text-destructive">*</span>
                </Label>
                <Textarea
                  id="retry-reason"
                  rows={3}
                  placeholder="Contoh: Kode akun 1110-DEP sudah diperbaiki di GL Host Chart of Accounts. Retry delivery."
                  aria-required="true"
                  aria-describedby="retry-reason-hint retry-reason-error"
                  disabled={submitting}
                  {...register("reason")}
                />
                <div className="flex justify-between items-center">
                  <p
                    id="retry-reason-error"
                    className={cn(
                      "text-xs",
                      errors.reason ? "text-destructive" : "hidden",
                    )}
                    aria-live="polite"
                  >
                    {errors.reason?.message}
                  </p>
                  <p
                    id="retry-reason-hint"
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
            )}

            {/* Info note */}
            {!maxAttemptsReached && (
              <p className="text-xs text-muted-foreground">
                Sistem akan menjadwalkan ulang pengiriman. Pantau hasilnya di panel
                status jurnal ini.
              </p>
            )}

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
              {!maxAttemptsReached && (
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
                      Jadwalkan Retry
                    </>
                  )}
                </Button>
              )}
            </div>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

