"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { AlertTriangle, CheckCircle2, Loader2 } from "lucide-react";
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
import { Checkbox } from "@/components/ui/checkbox";
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
import { periodeReopenApi } from "@/lib/api/periode-close.api";
import type { StatusPeriode } from "@/lib/schemas/periode-close.schema";

// ---------------------------------------------------------------------------
// Local form schema
// ---------------------------------------------------------------------------

const reopenApproveFormSchema = z.object({
  comment: z.string().min(10, "Komentar minimal 10 karakter"),
  attested: z
    .boolean()
    .refine((v) => v === true, { message: "Centang pernyataan untuk melanjutkan" })
    .optional(),
  signatureMethod: z.literal("JWT_STEP_UP"),
});

type ReopenApproveForm = z.infer<typeof reopenApproveFormSchema>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ReopenApproveConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  targetStatus: StatusPeriode;
  /** stepUpToken required for CLOSED→SOFT_CLOSED; undefined for SOFT_CLOSED→OPEN */
  stepUpToken?: string;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S4-AC2, S4-AC3)
// ---------------------------------------------------------------------------

export function ReopenApproveConfirmDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  targetStatus,
  stepUpToken,
  onSuccess,
}: ReopenApproveConfirmDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const isDestructive = targetStatus === "SOFT_CLOSED"; // CLOSED→SOFT_CLOSED path
  const requiresAttestation = isDestructive;
  const requiresMFA = isDestructive;

  const form = useForm<ReopenApproveForm>({
    resolver: zodResolver(reopenApproveFormSchema),
    defaultValues: {
      comment: "",
      attested: requiresAttestation ? false : undefined,
      signatureMethod: "JWT_STEP_UP",
    },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({
        comment: "",
        attested: requiresAttestation ? false : undefined,
        signatureMethod: "JWT_STEP_UP",
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = async (data: ReopenApproveForm) => {
    setIsSubmitting(true);
    try {
      await periodeReopenApi.approve(
        periodeId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        stepUpToken,
        idempotencyKey.current,
      );
      notify.success(
        `Periode ${periodeKode} berhasil di-reopen ke ${targetStatus}. Koreksi dapat dilakukan.`,
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

  const attestedValue = form.watch("attested");
  const submitDisabled = isSubmitting || (requiresAttestation && !attestedValue);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-lg"
        aria-labelledby="reopen-apr-title"
        aria-describedby="reopen-apr-desc"
      >
        <DialogHeader>
          <DialogTitle
            id="reopen-apr-title"
            className={`flex items-center gap-2 ${isDestructive ? "text-orange-700" : "text-green-700"}`}
          >
            {isDestructive ? (
              <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
            ) : (
              <CheckCircle2 className="h-5 w-5 shrink-0" aria-hidden="true" />
            )}
            Konfirmasi Reopen ke {targetStatus} — {periodeKode}
          </DialogTitle>
        </DialogHeader>

        <div
          id="reopen-apr-desc"
          className={`rounded-md border px-4 py-3 space-y-1 ${
            isDestructive
              ? "bg-orange-50 border-orange-200"
              : "bg-green-50 border-green-200"
          }`}
        >
          {isDestructive ? (
            <>
              <p className={`text-sm font-semibold text-orange-800`}>
                Tindakan exceptional — dicatat di audit trail.
              </p>
              <ul className="text-xs text-orange-700 list-disc list-inside space-y-0.5">
                <li>
                  Grace window 48 jam berlaku. Setelah expired, reopen tidak bisa dilakukan.
                </li>
                <li>Alasan koreksi akan terlihat di audit log permanen.</li>
                <li>Step-up MFA CFO diperlukan.</li>
              </ul>
            </>
          ) : (
            <p className="text-sm text-green-800">
              Periode {periodeKode} akan dikembalikan ke OPEN. Transaksi dan jurnal dapat
              dikoreksi kembali.
            </p>
          )}
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
            {/* Attestation — only for destructive CLOSED→SOFT_CLOSED */}
            {requiresAttestation && (
              <FormField
                control={form.control}
                name="attested"
                render={({ field }) => (
                  <FormItem className="flex flex-row items-start gap-3 rounded-md border border-orange-300 bg-orange-50/50 p-3">
                    <FormControl>
                      <Checkbox
                        id="attest-reopen"
                        checked={field.value === true}
                        onCheckedChange={(checked) => field.onChange(checked === true)}
                        aria-describedby="attest-reopen-label"
                        className="mt-0.5 border-orange-400 data-[state=checked]:bg-orange-700 data-[state=checked]:border-orange-700"
                      />
                    </FormControl>
                    <FormLabel
                      htmlFor="attest-reopen"
                      id="attest-reopen-label"
                      className="text-sm font-medium text-orange-800 leading-snug cursor-pointer"
                    >
                      Saya memahami bahwa reopen periode CLOSED ke SOFT_CLOSED adalah tindakan
                      exceptional dan akan tercatat permanen di audit trail BLIPS.
                    </FormLabel>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {/* Comment */}
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Komentar <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder={
                        isDestructive
                          ? "Mis: Reopen approved untuk koreksi akun 2030. Perbedaan IDR 15.000.000 teridentifikasi saat audit."
                          : "Mis: Approved. Periode diopen kembali untuk koreksi jurnal sesuai permintaan Finance Controller."
                      }
                      rows={3}
                      maxLength={2000}
                      aria-required="true"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Step-up token indicator */}
            {requiresMFA && stepUpToken && (
              <p className="text-xs text-green-700 bg-green-50 border border-green-200 rounded px-3 py-1.5">
                Step-up MFA terverifikasi. Token valid 5 menit.
              </p>
            )}

            <div className="flex justify-end gap-3">
              <DialogClose asChild>
                <Button type="button" variant="outline" disabled={isSubmitting}>
                  Batal
                </Button>
              </DialogClose>
              <Button
                type="submit"
                variant={isDestructive ? "destructive" : "default"}
                disabled={submitDisabled}
              >
                {isSubmitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Memproses Reopen...
                  </>
                ) : (
                  `Approve Reopen ke ${targetStatus}`
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
