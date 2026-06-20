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
import { renewalWorkflowApi, renewalQueryKeys } from "@/lib/api/renewal.api";
import {
  rejectRenewalSchema,
  type RejectRenewalInput,
  type RejectRenewalResponse,
} from "@/lib/schemas/renewal.schema";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RenewalRejectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  renewalId: string;
  instrumenKode: string;
  onSuccess?: (response: RejectRenewalResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S2 — reject; comment ≥ 30 char WAJIB)
// ---------------------------------------------------------------------------

export function RenewalRejectDialog({
  open,
  onOpenChange,
  renewalId,
  instrumenKode,
  onSuccess,
}: RenewalRejectDialogProps) {
  const queryClient = useQueryClient();
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<RejectRenewalInput>({
    resolver: zodResolver(rejectRenewalSchema),
    defaultValues: {
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  const comment = form.watch("comment");
  const isSubmitting = form.formState.isSubmitting;
  const commentLen = comment.length;
  const canSubmit = commentLen >= 30;

  const onSubmit = async (data: RejectRenewalInput) => {
    try {
      const result = await renewalWorkflowApi.reject(
        renewalId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        idempotencyKey.current,
      );

      const resp = result.data;
      notify.destructive(
        `Renewal ${instrumenKode} ditolak. Maker akan dinotifikasi.`,
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
      if (
        isApiError(err) &&
        ["SOD_VIOLATION", "WORKFLOW_INVALID_TRANSITION"].includes(err.code)
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
            <XCircle className="h-5 w-5 text-destructive" aria-hidden="true" />
            Tolak Renewal Deposito
          </DialogTitle>
          <DialogDescription>
            Instrumen: <strong>{instrumenKode}</strong>. Maker akan dinotifikasi.
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Penolakan <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Rate 5.75% melebihi benchmark internal 5.50%. Harap revisi rate atau lampirkan persetujuan ALCO."
                      rows={4}
                      maxLength={2000}
                      aria-describedby="reject-comment-hint"
                    />
                  </FormControl>
                  <p id="reject-comment-hint" className="text-xs text-muted-foreground">
                    Alasan penolakan wajib minimal 30 karakter.{" "}
                    <span
                      className={
                        commentLen >= 30 ? "text-green-700 font-medium" : "text-amber-600"
                      }
                    >
                      {commentLen}/30
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
                disabled={isSubmitting || !canSubmit}
                aria-label="Tolak renewal deposito"
              >
                {isSubmitting ? "Memproses..." : "Tolak Renewal"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
