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
import { workflowApproveBodySchema } from "@/lib/schemas/periode-close.schema";
import type { WorkflowApproveBody } from "@/lib/schemas/periode-close.schema";
import { periodeSoftCloseApi } from "@/lib/api/periode-close.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface SoftCloseApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  requesterName?: string;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1..S2-AC4)
// ---------------------------------------------------------------------------

export function SoftCloseApproveDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  requesterName,
  onSuccess,
}: SoftCloseApproveDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<WorkflowApproveBody>({
    resolver: zodResolver(workflowApproveBodySchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STEP_UP" },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({ comment: "", signatureMethod: "JWT_STEP_UP" });
    }
  }, [open, form]);

  const onSubmit = async (data: WorkflowApproveBody) => {
    setIsSubmitting(true);
    try {
      await periodeSoftCloseApi.approve(periodeId, data, idempotencyKey.current);
      notify.success(
        `Periode ${periodeKode} berhasil di-soft-close. Mutasi transaksi/jurnal diblokir. Siap untuk hard-close oleh CFO.`,
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
      <DialogContent className="max-w-md" aria-labelledby="soft-close-apr-title">
        <DialogHeader>
          <DialogTitle id="soft-close-apr-title">
            Approve Soft-Close — {periodeKode}
          </DialogTitle>
        </DialogHeader>
        {requesterName && (
          <p className="text-sm text-muted-foreground">
            Request diajukan oleh{" "}
            <span className="font-medium">{requesterName}</span>.
            Sistem akan mengevaluasi ulang checklist jika sudah lebih dari 24 jam sejak request.
          </p>
        )}
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Komentar Approval <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Approved. Semua posisi telah diverifikasi oleh Finance Controller."
                      rows={3}
                      maxLength={2000}
                      aria-required="true"
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
                    Menyetujui...
                  </>
                ) : (
                  "Approve Soft-Close"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
