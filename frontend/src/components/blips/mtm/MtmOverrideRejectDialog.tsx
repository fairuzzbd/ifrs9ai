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
import { mtmOverrideApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import {
  mtmOverrideRejectSchema,
  type MtmOverrideRejectInput,
  type MtmOverrideRejectResponse,
} from "@/lib/schemas/mtm.schema";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmOverrideRejectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mtmId: string;
  instrumenKode: string;
  tanggalMtm: string;
  onSuccess?: (response: MtmOverrideRejectResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S4-AC2, S4-AC4 — comment ≥ 30 char WAJIB; destructive variant)
// ---------------------------------------------------------------------------

export function MtmOverrideRejectDialog({
  open,
  onOpenChange,
  mtmId,
  instrumenKode,
  tanggalMtm,
  onSuccess,
}: MtmOverrideRejectDialogProps) {
  const queryClient = useQueryClient();
  // Idempotency key fresh per dialog open (DEC-021)
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<MtmOverrideRejectInput>({
    resolver: zodResolver(mtmOverrideRejectSchema),
    defaultValues: {
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  const comment = form.watch("comment");
  const isSubmitting = form.formState.isSubmitting;

  // Button enabled only when comment ≥ 30 char (S4-AC4)
  const canSubmit = (comment?.length ?? 0) >= 30;

  const onSubmit = async (data: MtmOverrideRejectInput) => {
    try {
      const result = await mtmOverrideApi.reject(
        mtmId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        idempotencyKey.current,
      );

      const resp = result.data;
      notify.destructive(
        `MTM ${instrumenKode} ${tanggalMtm} ditolak. ROLE-AKUN telah dinotifikasi untuk re-upload.`,
      );

      // Invalidate MTM list + detail queries
      await queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
      await queryClient.invalidateQueries({ queryKey: mtmQueryKeys.detail(mtmId) });

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
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
      }
      // Close dialog on SoD or locked errors
      if (isApiError(err) && ["MTM_OVERRIDE_SOD_VIOLATION", "MTM_PERIODE_LOCKED", "WORKFLOW_INVALID_TRANSITION"].includes(err.code)) {
        onOpenChange(false);
      }
    }
  };

  return (
    <Dialog open={open} onOpenChange={(v) => {
      if (!isSubmitting) {
        form.reset();
        onOpenChange(v);
      }
    }}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-destructive">
            <XCircle className="h-5 w-5" aria-hidden="true" />
            Tolak MTM
          </DialogTitle>
          <DialogDescription>
            <strong>{instrumenKode}</strong> — {tanggalMtm}
          </DialogDescription>
        </DialogHeader>

        {/* Warning notice */}
        <div className="rounded-md border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
          Jurnal <strong>tidak akan diposting</strong>. ROLE-AKUN akan dinotifikasi untuk re-upload dengan harga yang benar.
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {/* Comment (WAJIB ≥ 30 char per S4-AC4) */}
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
                      placeholder="Mis: Harga 90.00 tidak sesuai IBPA hari ini. Re-upload dengan referensi Bloomberg atau IBPA."
                      rows={3}
                      maxLength={2000}
                      aria-describedby="reject-comment-hint"
                    />
                  </FormControl>
                  <p id="reject-comment-hint" className="text-xs text-muted-foreground">
                    Wajib minimal 30 karakter. Jelaskan alasan penolakan agar ROLE-AKUN dapat melakukan re-upload dengan benar.{" "}
                    <span className={comment?.length >= 30 ? "text-green-700" : "text-destructive"}>
                      {comment?.length ?? 0}/30
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
                aria-label="Tolak MTM — jurnal tidak akan diposting"
              >
                {isSubmitting ? "Memproses..." : "Tolak MTM"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
