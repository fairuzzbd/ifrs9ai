"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ShieldCheck, Loader2, Info } from "lucide-react";
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
  kursApproveBodySchema,
  type KursApproveBody,
  type KursBatchApproveResponse,
} from "@/lib/schemas/fx-rate.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface KursApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  batchId: string;
  rowCount: number;
  tanggalBerlaku: string;
  /** Maker user_id — shown in SoD note (approver ≠ maker) */
  makerId?: string | null;
  onSuccess?: (response: KursBatchApproveResponse) => void;
}

// ---------------------------------------------------------------------------
// Component (S3-AC1, S3-AC3 SoD contract)
// ---------------------------------------------------------------------------

export function KursApproveDialog({
  open,
  onOpenChange,
  batchId,
  rowCount,
  tanggalBerlaku,
  makerId,
  onSuccess,
}: KursApproveDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());

  const form = useForm<KursApproveBody>({
    resolver: zodResolver(kursApproveBodySchema),
    defaultValues: {
      comment: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({ comment: "", signatureMethod: "JWT_STEP_UP" });
    }
  }, [open, form]);

  const onSubmit = async (data: KursApproveBody) => {
    setIsSubmitting(true);
    try {
      const result = await kursBatchApi.approve(batchId, data, idempotencyKey.current);
      const resp = result.data;

      notify.success(
        `${resp.rowsApproved} kurs untuk ${tanggalBerlaku} berhasil disetujui dan aktif untuk digunakan sistem.`,
        {
          action: {
            label: "Lihat kurs",
            onClick: () => {
              window.location.href = `/master/kurs?filter[upload_batch_id]=${batchId}`;
            },
          },
        },
      );

      onOpenChange(false);
      onSuccess?.(resp);
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
        aria-labelledby="kurs-approve-title"
        aria-describedby="kurs-approve-desc"
      >
        <DialogHeader>
          <DialogTitle
            id="kurs-approve-title"
            className="flex items-center gap-2 text-green-800"
          >
            <ShieldCheck className="h-5 w-5 shrink-0" aria-hidden="true" />
            Approve Kurs Manual — {tanggalBerlaku}
          </DialogTitle>
        </DialogHeader>

        <div id="kurs-approve-desc" className="space-y-3">
          {/* Batch summary */}
          <div className="rounded-md border bg-muted/30 px-4 py-3 text-sm">
            <p>
              <strong>{rowCount}</strong> kurs untuk tanggal{" "}
              <strong>{tanggalBerlaku}</strong> akan disetujui dan langsung aktif
              digunakan dalam perhitungan MTM, akrual, dan ECL.
            </p>
          </div>

          {/* SoD notice */}
          {makerId && (
            <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
              <Info
                className="mt-0.5 h-4 w-4 shrink-0 text-blue-600"
                aria-hidden="true"
              />
              <p>
                <strong>SoD (Segregation of Duties)</strong>: Anda hanya bisa
                menyetujui kurs yang{" "}
                <strong>bukan Anda yang upload</strong>. Pastikan Anda adalah
                ROLE-AKUN-CTL yang berbeda dari pembuat batch ini (DEC-017).
              </p>
            </div>
          )}
        </div>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-4"
          >
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
                      placeholder="Mis: Kurs 2026-06-18 telah diverifikasi dengan sumber Bloomberg jam 11:25 WIB. Disetujui."
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
                disabled={isSubmitting}
                className="bg-green-700 hover:bg-green-800 text-white"
                aria-label={`Approve ${rowCount} kurs untuk ${tanggalBerlaku}`}
              >
                {isSubmitting ? (
                  <>
                    <Loader2
                      className="mr-2 h-4 w-4 animate-spin"
                      aria-hidden="true"
                    />
                    Memproses...
                  </>
                ) : (
                  `Approve ${rowCount} Kurs`
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
