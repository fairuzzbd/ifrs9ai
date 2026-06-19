"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2 } from "lucide-react";
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
import { softCloseRequestBodySchema } from "@/lib/schemas/periode-close.schema";
import type { SoftCloseRequestBody } from "@/lib/schemas/periode-close.schema";
import { periodeSoftCloseApi } from "@/lib/api/periode-close.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface SoftCloseRequestDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  rowVersion: number;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S1-AC1..S1-AC4)
// ---------------------------------------------------------------------------

export function SoftCloseRequestDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  rowVersion,
  onSuccess,
}: SoftCloseRequestDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<SoftCloseRequestBody>({
    resolver: zodResolver(softCloseRequestBodySchema),
    defaultValues: { catatan: "", rowVersion },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({ catatan: "", rowVersion });
    }
  }, [open, rowVersion, form]);

  const onSubmit = async (data: SoftCloseRequestBody) => {
    setIsSubmitting(true);
    try {
      await periodeSoftCloseApi.request(periodeId, data, idempotencyKey.current);
      notify.success(
        `Soft-close request ${periodeKode} berhasil diajukan. Menunggu approval dari Finance Controller lain.`,
      );
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "CLOSING_CHECKLIST_FAILED") {
          const failedItems = err.details.map((d) => d.field).join(", ");
          notify.error(err, {
            action: {
              label: "Lihat checklist",
              onClick: () => onOpenChange(false),
            },
          });
          // Set field errors for each failed item
          err.details.forEach((d) => {
            form.setError("catatan", {
              type: "manual",
              message: `Checklist gagal: ${d.message}`,
            });
          });
          void failedItems; // used indirectly via err.details
        } else {
          notify.error(err);
        }
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" aria-labelledby="soft-close-req-title">
        <DialogHeader>
          <DialogTitle id="soft-close-req-title">
            Ajukan Soft-Close — {periodeKode}
          </DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          Sistem akan mengevaluasi 4-item closing checklist secara real-time. Request akan diteruskan ke
          Finance Controller lain untuk approval (4-eyes, SoD).
        </p>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="catatan"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Catatan (opsional)</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Soft close request periode Juni 2026. Semua transaksi telah diverifikasi."
                      rows={3}
                      maxLength={1000}
                      aria-describedby="catatan-hint"
                    />
                  </FormControl>
                  <p id="catatan-hint" className="text-xs text-muted-foreground">
                    Maksimal 1000 karakter
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
              <Button type="submit" disabled={isSubmitting}>
                {isSubmitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Mengajukan...
                  </>
                ) : (
                  "Ajukan Soft-Close"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
