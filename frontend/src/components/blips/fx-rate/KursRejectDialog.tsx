"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { XCircle, Loader2 } from "lucide-react";
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
import { isApiError } from "@/lib/api";
import { notify } from "@/lib/notify";
import { kursBatchApi } from "@/lib/api/fx-rate.api";
import {
  kursRejectBodySchema,
  type KursRejectBody,
  type KursBatchRejectResponse,
} from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KursRejectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  batchId: string;
  rowCount: number;
  tanggalBerlaku: string;
  onSuccess?: (response: KursBatchRejectResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S3-AC2, S3-AC4 reject_reason min 20 chars)
// ---------------------------------------------------------------------------

export function KursRejectDialog({
  open,
  onOpenChange,
  batchId,
  rowCount,
  tanggalBerlaku,
  onSuccess,
}: KursRejectDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<KursRejectBody>({
    resolver: zodResolver(kursRejectBodySchema),
    defaultValues: {
      rejectReason: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({ rejectReason: "", signatureMethod: "JWT_STEP_UP" });
    }
  }, [open, form]);

  const watchedReason = form.watch("rejectReason");
  const charCount = watchedReason.length;
  const isUnderMin = charCount < 20;

  const onSubmit = async (data: KursRejectBody) => {
    setIsSubmitting(true);
    try {
      const result = await kursBatchApi.reject(batchId, data, idempotencyKey.current);
      const resp = result.data;

      notify.destructive(
        `${resp.rowsRejected} kurs untuk ${tanggalBerlaku} ditolak. ROLE-AKUN dinotifikasi untuk re-upload.`,
      );

      onOpenChange(false);
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            if (d.field.includes("rejectReason")) {
              form.setError("rejectReason", { message: d.message });
            }
          });
        }
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
      <DialogContent
        className="max-w-lg"
        aria-labelledby="kurs-reject-title"
        aria-describedby="kurs-reject-desc"
      >
        <DialogHeader>
          <DialogTitle
            id="kurs-reject-title"
            className="flex items-center gap-2 text-red-700"
          >
            <XCircle className="h-5 w-5 shrink-0" aria-hidden="true" />
            Tolak Kurs Manual — {tanggalBerlaku}
          </DialogTitle>
        </DialogHeader>

        <p
          id="kurs-reject-desc"
          className="text-sm text-muted-foreground"
        >
          <strong>{rowCount}</strong> kurs akan ditolak. ROLE-AKUN akan
          dinotifikasi untuk melakukan re-upload kurs yang benar.
        </p>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-4"
          >
            <FormField
              control={form.control}
              name="rejectReason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Penolakan <span aria-hidden="true">*</span>
                    <span className="ml-1 text-xs font-normal text-muted-foreground">
                      (minimal 20 karakter)
                    </span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Kurs tengah USD 16250 tidak sesuai data BI JISDOR hari ini. Seharusnya 16100. Mohon re-upload dengan sumber yang tepat."
                      rows={4}
                      maxLength={2000}
                      aria-required="true"
                      aria-describedby="reject-char-count"
                    />
                  </FormControl>
                  <p
                    id="reject-char-count"
                    className={`text-xs ${isUnderMin ? "text-destructive" : "text-muted-foreground"}`}
                    aria-live="polite"
                  >
                    {charCount}/2000 karakter{isUnderMin ? ` (minimal 20 diperlukan)` : ""}
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex justify-end gap-3">
              <DialogClose asChild>
                <Button
                  type="button"
                  variant="outline"
                  disabled={isSubmitting}
                >
                  Batal
                </Button>
              </DialogClose>
              <Button
                type="submit"
                variant="destructive"
                disabled={isSubmitting}
                aria-label={`Tolak ${rowCount} kurs untuk ${tanggalBerlaku}`}
              >
                {isSubmitting ? (
                  <>
                    <Loader2
                      className="mr-2 h-4 w-4 animate-spin"
                      aria-hidden="true"
                    />
                    Menolak...
                  </>
                ) : (
                  `Tolak ${rowCount} Kurs`
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
