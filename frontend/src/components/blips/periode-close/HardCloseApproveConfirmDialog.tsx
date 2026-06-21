"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { AlertTriangle, Loader2 } from "lucide-react";
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
import { periodeHardCloseApi } from "@/lib/api/periode-close.api";

// ---------------------------------------------------------------------------
// Local schema (attestation checkbox + comment)
// ---------------------------------------------------------------------------

const hardCloseApproveFormSchema = z.object({
  comment: z.string().min(10, "Komentar minimal 10 karakter"),
  attested: z
    .boolean()
    .refine((v) => v === true, { message: "Centang pernyataan di atas untuk melanjutkan" }),
  signatureMethod: z.literal("JWT_STEP_UP"),
});

type HardCloseApproveForm = z.infer<typeof hardCloseApproveFormSchema>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface HardCloseApproveConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  rowVersion: number;
  stepUpToken: string;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Component (S3-AC2, S3-AC3 — "destructive confirm before MFA flow")
// ---------------------------------------------------------------------------

export function HardCloseApproveConfirmDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  rowVersion: _rowVersion,
  stepUpToken,
  onSuccess,
}: HardCloseApproveConfirmDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<HardCloseApproveForm>({
    resolver: zodResolver(hardCloseApproveFormSchema),
    defaultValues: {
      comment: "",
      attested: false,
      signatureMethod: "JWT_STEP_UP",
    },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({
        comment: "",
        attested: false,
        signatureMethod: "JWT_STEP_UP",
      });
    }
  }, [open, form]);

  const onSubmit = async (data: HardCloseApproveForm) => {
    if (!stepUpToken) return;
    setIsSubmitting(true);
    try {
      await periodeHardCloseApi.approve(
        periodeId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        stepUpToken,
        idempotencyKey.current,
      );
      notify.success(
        `Periode ${periodeKode} berhasil di-hard-close. Entri jurnal final, laporan ECL dapat dipublikasikan.`,
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
      <DialogContent
        className="max-w-lg"
        aria-labelledby="hc-approve-title"
        aria-describedby="hc-approve-desc"
      >
        <DialogHeader>
          <DialogTitle id="hc-approve-title" className="flex items-center gap-2 text-red-700">
            <AlertTriangle className="h-5 w-5 shrink-0" aria-hidden="true" />
            Konfirmasi Hard-Close — {periodeKode}
          </DialogTitle>
        </DialogHeader>

        <div
          id="hc-approve-desc"
          className="rounded-md bg-red-50 border border-red-200 px-4 py-3 space-y-1"
        >
          <p className="text-sm font-semibold text-red-800">
            Tindakan ini bersifat permanen setelah grace window 48 jam berakhir.
          </p>
          <ul className="text-xs text-red-700 list-disc list-inside space-y-0.5">
            <li>Semua mutasi pada periode ini akan diblokir secara permanen.</li>
            <li>Jurnal posting menjadi final dan tidak bisa diubah.</li>
            <li>MV reporting akan di-refresh secara otomatis setelah proses selesai.</li>
            <li>
              Reopen ke SOFT_CLOSED hanya bisa dilakukan dalam grace window 48 jam dengan step-up
              MFA.
            </li>
          </ul>
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
            {/* Attestation checkbox */}
            <FormField
              control={form.control}
              name="attested"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start gap-3 rounded-md border border-red-300 bg-red-50/50 p-3">
                  <FormControl>
                    <Checkbox
                      id="attest-hc"
                      checked={field.value === true}
                      onCheckedChange={(checked) => {
                        field.onChange(checked === true);
                      }}
                      aria-describedby="attest-hc-label"
                      className="mt-0.5 border-red-400 data-[state=checked]:bg-red-700 data-[state=checked]:border-red-700"
                    />
                  </FormControl>
                  <FormLabel
                    htmlFor="attest-hc"
                    id="attest-hc-label"
                    className="text-sm font-medium text-red-800 leading-snug cursor-pointer"
                  >
                    Saya, sebagai CFO Tugu Reasuransi, menyatakan bahwa periode buku {periodeKode}{" "}
                    telah diverifikasi dan siap untuk ditutup secara final sesuai PSAK 71 / IFRS 9.
                  </FormLabel>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Comment field */}
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Komentar Approval CFO <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Hard-close disetujui. Semua posisi telah diverifikasi bersama Finance Controller. Periode siap final."
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
            {stepUpToken && (
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
                variant="destructive"
                disabled={isSubmitting || form.watch("attested") !== true}
                aria-label={`Konfirmasi hard-close periode ${periodeKode}`}
              >
                {isSubmitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Memproses Hard-Close...
                  </>
                ) : (
                  "Hard-Close Sekarang"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
