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
import { penjualanWorkflowApi, penjualanQueryKeys } from "@/lib/api/penjualan.api";
import {
  approvePenjualanSchema,
  type ApprovePenjualanInput,
  type ApprovePenjualanResponse,
} from "@/lib/schemas/penjualan.schema";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PenjualanApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  penjualanId: string;
  instrumenKode: string;
  /** makerId for SoD display note */
  makerId?: string;
  onSuccess?: (response: ApprovePenjualanResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC2 SoD, S2-AC4 idempotency)
// ---------------------------------------------------------------------------

export function PenjualanApproveDialog({
  open,
  onOpenChange,
  penjualanId,
  instrumenKode,
  onSuccess,
}: PenjualanApproveDialogProps) {
  const queryClient = useQueryClient();
  // Fresh idempotency key per dialog open (DEC-021)
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<ApprovePenjualanInput>({
    resolver: zodResolver(approvePenjualanSchema),
    defaultValues: {
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  const comment = form.watch("comment");
  const isSubmitting = form.formState.isSubmitting;

  const onSubmit = async (data: ApprovePenjualanInput) => {
    try {
      const result = await penjualanWorkflowApi.approve(
        penjualanId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        idempotencyKey.current,
      );

      const resp = result.data;
      const statusMsg = resp.status === "PENDING_BM_REVIEW"
        ? "Penjualan memerlukan review BM (disposal kumulatif HTC melampaui threshold). ROLE-RISK telah dinotifikasi."
        : `Penjualan ${instrumenKode} disetujui dan diposting. Instrumen ${resp.instrumenStatusAfter === "DISPOSED" ? "berstatus DISPOSED" : "qty diperbarui"}.`;

      notify.success(statusMsg, {
        action: resp.jurnalEntryId
          ? {
              label: "Lihat Jurnal",
              onClick: () => {
                window.location.href = `/transaksi/jurnal/${resp.jurnalEntryId}`;
              },
            }
          : undefined,
      });

      // Warn if BM risk flagged
      if (resp.bmViolationRisk) {
        notify.warning(
          "Peringatan BM HTC: Disposal kumulatif 12-bulan melewati threshold peringatan. ROLE-RISK telah dinotifikasi.",
        );
      }

      await queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.lists() });
      await queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.detail(penjualanId) });

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
      if (
        isApiError(err) &&
        ["SOD_VIOLATION", "PENJUALAN_PERIODE_LOCKED", "WORKFLOW_INVALID_TRANSITION"].includes(err.code)
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
            Setujui Penjualan Instrumen
          </DialogTitle>
          <DialogDescription>
            Instrumen: <strong>{instrumenKode}</strong>
          </DialogDescription>
        </DialogHeader>

        {/* SoD informational note */}
        <div className="rounded-md border border-blue-100 bg-blue-50 px-3 py-2 text-xs text-blue-800">
          <strong>SoD:</strong> Anda bertindak sebagai Treasury Approver.
          Anda tidak boleh menyetujui penjualan yang Anda buat sendiri (DEC-017).
          Setelah disetujui, OCI recycling, BM check, jurnal multi-leg, dan derecognition
          instrumen akan diproses dalam satu transaksi.
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
                      placeholder="Mis: Preview diverifikasi. Harga sesuai IBPA/BEI closing. OCI recycling dikonfirmasi. Disetujui."
                      rows={3}
                      maxLength={2000}
                      aria-describedby="approve-comment-hint"
                    />
                  </FormControl>
                  <p id="approve-comment-hint" className="text-xs text-muted-foreground">
                    Jelaskan dasar verifikasi harga, OCI, dan preview kalkulasi.{" "}
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
                aria-label="Setujui penjualan dan posting jurnal multi-leg"
              >
                {isSubmitting ? "Memproses..." : "Setuju & Posting"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
