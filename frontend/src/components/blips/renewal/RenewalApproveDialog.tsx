"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { CheckCircle } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { renewalWorkflowApi, renewalQueryKeys } from "@/lib/api/renewal.api";
import {
  approveRenewalSchema,
  type ApproveRenewalInput,
  type ApproveRenewalResponse,
} from "@/lib/schemas/renewal.schema";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RenewalApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  renewalId: string;
  instrumenKode: string;
  /** makerId for SoD display note */
  makerId?: string;
  onSuccess?: (response: ApproveRenewalResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC3 SoD, S2-AC4 idempotency)
// ---------------------------------------------------------------------------

export function RenewalApproveDialog({
  open,
  onOpenChange,
  renewalId,
  instrumenKode,
  onSuccess,
}: RenewalApproveDialogProps) {
  const queryClient = useQueryClient();
  // Fresh idempotency key per dialog open (DEC-021)
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<ApproveRenewalInput>({
    resolver: zodResolver(approveRenewalSchema),
    defaultValues: {
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  const comment = form.watch("comment");
  const isSubmitting = form.formState.isSubmitting;

  const onSubmit = async (data: ApproveRenewalInput) => {
    try {
      const result = await renewalWorkflowApi.approve(
        renewalId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        idempotencyKey.current,
      );

      const resp = result.data;
      notify.success(
        `Renewal ${instrumenKode} disetujui dan diposting. Instrumen baru ${resp.instrumenBaruId ? "telah dibuat" : "sedang diproses"}.`,
        {
          action: resp.instrumenBaruId
            ? {
                label: "Lihat Instrumen Baru",
                onClick: () => {
                  window.location.href = `/master/instrumen/${resp.instrumenBaruId}`;
                },
              }
            : undefined,
        },
      );

      await queryClient.invalidateQueries({ queryKey: renewalQueryKeys.lists() });
      await queryClient.invalidateQueries({ queryKey: renewalQueryKeys.detail(renewalId) });

      form.reset();
      onOpenChange(false);
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            if (d.field.includes("comment")) {
              form.setError("comment", { message: d.message });
            }
          });
        }
        notify.error(err);
      } else {
        notify.error({
          code: "NETWORK_ERROR",
          message: "Gagal menghubungi server.",
          traceId: "",
        });
      }
      // Close dialog on non-retryable errors
      if (
        isApiError(err) &&
        ["SOD_VIOLATION", "PERIODE_CLOSED", "WORKFLOW_INVALID_TRANSITION"].includes(err.code)
      ) {
        onOpenChange(false);
      }
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => {
        if (!isSubmitting) {
          form.reset();
          onOpenChange(v);
        }
      }}
    >
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CheckCircle className="h-5 w-5 text-green-600" aria-hidden="true" />
            Setujui Renewal Deposito
          </DialogTitle>
          <DialogDescription>
            Instrumen: <strong>{instrumenKode}</strong>
          </DialogDescription>
        </DialogHeader>

        {/* SoD informational note */}
        <div className="rounded-md border border-blue-100 bg-blue-50 px-3 py-2 text-xs text-blue-800">
          <strong>SoD:</strong> Anda bertindak sebagai Treasury Approver.
          Anda tidak boleh menyetujui renewal yang Anda buat sendiri (DEC-017, PSAK 71).
          Setelah disetujui, instrumen baru akan dibuat dan jurnal RENEWAL_DEPOSITO
          diposting secara otomatis dalam satu transaksi.
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Komentar Persetujuan <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Preview diverifikasi. Rate 5.75% sesuai BI Rate + spread 1.75%. Disetujui."
                      rows={3}
                      maxLength={2000}
                      aria-describedby="approve-comment-hint"
                    />
                  </FormControl>
                  <p id="approve-comment-hint" className="text-xs text-muted-foreground">
                    Jelaskan dasar verifikasi rate dan preview kalkulasi.{" "}
                    <span className={comment.length >= 1 ? "text-green-700" : "text-muted-foreground"}>
                      {comment.length} karakter
                    </span>
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={isSubmitting}
                onClick={() => {
                  form.reset();
                  onOpenChange(false);
                }}
              >
                Batal
              </Button>
              <Button
                type="submit"
                disabled={isSubmitting || !comment.trim()}
                aria-label="Setujui renewal dan posting jurnal"
              >
                {isSubmitting ? "Memproses..." : "Setuju & Posting Jurnal"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
