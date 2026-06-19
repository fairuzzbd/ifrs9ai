"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2, XCircle } from "lucide-react";
import { v4 as uuidv4 } from "uuid";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogClose,
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
import { rejectBodySchema } from "@/lib/schemas/periode-close.schema";
import type { RejectBody } from "@/lib/schemas/periode-close.schema";
import { periodeHardCloseApi } from "@/lib/api/periode-close.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface HardCloseRejectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S3-AC4 — CFO reject, reason ≥30 chars, no MFA)
// ---------------------------------------------------------------------------

export function HardCloseRejectDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  onSuccess,
}: HardCloseRejectDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<RejectBody>({
    resolver: zodResolver(rejectBodySchema),
    defaultValues: { reason: "" },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({ reason: "" });
    }
  }, [open, form]);

  const onSubmit = async (data: RejectBody) => {
    setIsSubmitting(true);
    try {
      // data.reason maps to API body { alasan_penolakan: ... } in the API client
      await periodeHardCloseApi.reject(periodeId, data, idempotencyKey.current);
      notify.success(
        `Hard-close request ${periodeKode} ditolak. Status kembali ke SOFT_CLOSED. Finance Controller akan menerima notifikasi.`,
      );
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" aria-labelledby="hc-reject-title">
        <DialogHeader>
          <DialogTitle id="hc-reject-title" className="flex items-center gap-2">
            <XCircle className="h-5 w-5 text-destructive" aria-hidden="true" />
            Tolak Hard-Close — {periodeKode}
          </DialogTitle>
        </DialogHeader>

        <p className="text-sm text-muted-foreground">
          Status periode akan kembali ke <strong>SOFT_CLOSED</strong>. Finance Controller harus
          memperbaiki masalah sebelum mengajukan ulang.
        </p>

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
                      placeholder="Mis: Ditemukan perbedaan saldo GL vs jurnal pada akun 2030 sebesar IDR 15.000.000. Harap rekonsiliasi terlebih dahulu sebelum hard-close."
                      rows={4}
                      maxLength={2000}
                      aria-required="true"
                      aria-describedby="reject-reason-hint"
                    />
                  </FormControl>
                  <p id="reject-reason-hint" className="text-xs text-muted-foreground">
                    Minimal 30 karakter. Saat ini: {(field.value as string | undefined)?.length ?? 0} karakter.
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex justify-end gap-3">
              <DialogClose asChild>
                <Button type="button" variant="outline" disabled={isSubmitting}>
                  Batal
                </Button>
              </DialogClose>
              <Button type="submit" variant="destructive" disabled={isSubmitting}>
                {isSubmitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Menolak...
                  </>
                ) : (
                  "Tolak Hard-Close"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
