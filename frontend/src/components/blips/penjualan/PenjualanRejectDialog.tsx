"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { XCircle } from "lucide-react";
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
  rejectPenjualanSchema,
  type RejectPenjualanInput,
  type RejectPenjualanResponse,
} from "@/lib/schemas/penjualan.schema";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PenjualanRejectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  penjualanId: string;
  instrumenKode: string;
  onSuccess?: (response: RejectPenjualanResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S2 — reject with reason ≥ 30 char)
// ---------------------------------------------------------------------------

export function PenjualanRejectDialog({
  open,
  onOpenChange,
  penjualanId,
  instrumenKode,
  onSuccess,
}: PenjualanRejectDialogProps) {
  const queryClient = useQueryClient();
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<RejectPenjualanInput>({
    resolver: zodResolver(rejectPenjualanSchema),
    defaultValues: {
      reason: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  const reason = form.watch("reason");
  const isSubmitting = form.formState.isSubmitting;
  const MIN_REASON = 30;

  const onSubmit = async (data: RejectPenjualanInput) => {
    try {
      const result = await penjualanWorkflowApi.reject(
        penjualanId,
        { reason: data.reason, signatureMethod: data.signatureMethod },
        idempotencyKey.current,
      );

      const resp = result.data;
      notify.destructive(
        `Penjualan ${instrumenKode} ditolak. Maker akan menerima notifikasi alasan penolakan.`,
      );

      await queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.lists() });
      await queryClient.invalidateQueries({ queryKey: penjualanQueryKeys.detail(penjualanId) });

      form.reset();
      onOpenChange(false);
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            if (d.field.includes("reason")) {
              form.setError("reason", { message: d.message });
            }
          });
        }
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
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
            <XCircle className="h-5 w-5 text-red-600" aria-hidden="true" />
            Tolak Penjualan Instrumen
          </DialogTitle>
          <DialogDescription>
            Instrumen: <strong>{instrumenKode}</strong>
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="reason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Penolakan <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Harga jual 1.050.000 melebihi IBPA fair value 1.035.000 lebih dari 2%. Harap klarifikasi atau revisi harga."
                      rows={4}
                      maxLength={2000}
                      aria-describedby="reject-reason-hint"
                    />
                  </FormControl>
                  <p id="reject-reason-hint" className="text-xs text-muted-foreground">
                    Minimal {MIN_REASON} karakter.{" "}
                    <span
                      className={
                        reason.length >= MIN_REASON ? "text-green-700" : "text-red-500"
                      }
                    >
                      {reason.length} / {MIN_REASON}
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
                variant="destructive"
                disabled={isSubmitting || reason.length < MIN_REASON}
                aria-label="Tolak penjualan dan kirim notifikasi ke maker"
              >
                {isSubmitting ? "Memproses..." : "Tolak Penjualan"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
