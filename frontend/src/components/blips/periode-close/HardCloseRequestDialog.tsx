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
import { hardCloseRequestBodySchema } from "@/lib/schemas/periode-close.schema";
import type { HardCloseRequestBody } from "@/lib/schemas/periode-close.schema";
import { periodeHardCloseApi } from "@/lib/api/periode-close.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface HardCloseRequestDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  rowVersion: number;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S3-AC1 step 1)
// ---------------------------------------------------------------------------

export function HardCloseRequestDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  rowVersion,
  onSuccess,
}: HardCloseRequestDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<HardCloseRequestBody>({
    resolver: zodResolver(hardCloseRequestBodySchema),
    defaultValues: { catatan: "", rowVersion },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({ catatan: "", rowVersion });
    }
  }, [open, rowVersion, form]);

  const onSubmit = async (data: HardCloseRequestBody) => {
    setIsSubmitting(true);
    try {
      await periodeHardCloseApi.request(periodeId, data, idempotencyKey.current);
      notify.success(
        `Hard-close request ${periodeKode} berhasil diajukan. Menunggu approval CFO — step-up MFA diperlukan.`,
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
      <DialogContent className="max-w-md" aria-labelledby="hard-close-req-title">
        <DialogHeader>
          <DialogTitle id="hard-close-req-title">
            Ajukan Hard-Close — {periodeKode}
          </DialogTitle>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          Sistem akan mengevaluasi ulang 4-item closing checklist. Jika lolos, request diteruskan ke CFO
          untuk approval dengan step-up MFA (DEC-027).
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
                      placeholder="Mis: Hard close request PRD-2026-06. Semua koreksi selesai. Periode siap untuk finalisasi."
                      rows={3}
                      maxLength={1000}
                    />
                  </FormControl>
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
                  "Ajukan Hard-Close"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
