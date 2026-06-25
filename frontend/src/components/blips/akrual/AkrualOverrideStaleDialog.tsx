"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { ShieldAlert } from "lucide-react";
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
import { akrualOverrideApi, akrualQueryKeys } from "@/lib/api/akrual.api";
import {
  overrideStaleSchema,
  type OverrideStaleInput,
  type OverrideStaleResponse,
} from "@/lib/schemas/akrual.schema";
import { useQueryClient } from "@tanstack/react-query";

export interface AkrualOverrideStaleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  akrualId: string;
  instrumenKode: string;
  /** Stage saat ini untuk ditampilkan di dialog */
  stage?: 1 | 2 | 3 | null;
  onSuccess?: (response: OverrideStaleResponse) => void;
}

/**
 * Dialog untuk ROLE-AKUN-CTL mengkonfirmasi staging stale masih valid.
 * Implements S5-AC4: reason ≥ 30 char, signatureMethod = JWT_STEP_UP.
 * SoD: hanya ROLE-AKUN-CTL (absent-from-DOM enforcement di parent).
 */
export function AkrualOverrideStaleDialog({
  open,
  onOpenChange,
  akrualId,
  instrumenKode,
  stage,
  onSuccess,
}: AkrualOverrideStaleDialogProps) {
  const queryClient = useQueryClient();
  // Fresh idempotency key per dialog open (DEC-021)
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<OverrideStaleInput>({
    resolver: zodResolver(overrideStaleSchema),
    defaultValues: {
      reason: "",
      signatureMethod: "JWT_STEP_UP",
    },
  });

  const reason = form.watch("reason");
  const isSubmitting = form.formState.isSubmitting;

  const onSubmit = async (data: OverrideStaleInput) => {
    try {
      const result = await akrualOverrideApi.overrideStale(
        akrualId,
        data,
        idempotencyKey.current,
      );
      const resp = result.data;

      notify.success(
        `Staging Stage ${stage ?? "?"} instrumen ${instrumenKode} dikonfirmasi valid. Akrual IDR ${new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR", minimumFractionDigits: 4 }).format(parseFloat(resp.akrualIdr))} diposting.`,
        {
          action: {
            label: "Lihat Jurnal",
            onClick: () => {
              window.location.href = `/transaksi/jurnal/${resp.jurnalEntryId}`;
            },
          },
        },
      );

      await queryClient.invalidateQueries({ queryKey: akrualQueryKeys.lists() });
      await queryClient.invalidateQueries({ queryKey: akrualQueryKeys.detail(akrualId) });

      form.reset();
      onOpenChange(false);
      onSuccess?.(resp);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "VALIDATION_FAILED" && err.details.length > 0) {
          err.details.forEach((d) => {
            if (d.field.includes("reason")) {
              form.setError("reason", { message: d.message });
            }
          });
        }
        notify.error(err);
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
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
            <ShieldAlert className="h-5 w-5 text-amber-600" aria-hidden="true" />
            Konfirmasi Staging Stale — Override
          </DialogTitle>
          <DialogDescription>
            Instrumen: <strong>{instrumenKode}</strong>
            {stage && (
              <>
                {" · "}Stage saat ini: <strong>Stage {stage}</strong>
              </>
            )}
          </DialogDescription>
        </DialogHeader>

        <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 space-y-1">
          <p>
            <strong>Perhatian:</strong> ECL sealed run terakhir lebih dari batas staleness. Akrual
            mungkin tidak mencerminkan staging terkini.
          </p>
          <p>
            Dengan mengkonfirmasi, Anda sebagai <strong>ROLE-AKUN-CTL</strong> menyatakan staging
            saat ini masih valid dan akrual boleh diposting menggunakan ECL yang ada.
          </p>
          <p>
            Audit <strong>STAGING_STALE_ALERT override</strong> akan dicatat in-transaction.
          </p>
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="reason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Konfirmasi <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Tidak ada perubahan material sejak ECL run terakhir. Staging Stage 2 dikonfirmasi valid per judgement CFO 2026-06-20."
                      rows={4}
                      maxLength={2000}
                      aria-describedby="override-reason-hint"
                    />
                  </FormControl>
                  <p id="override-reason-hint" className="text-xs text-muted-foreground">
                    Minimal 30 karakter.{" "}
                    <span
                      className={reason.length >= 30 ? "text-green-700 font-medium" : "text-amber-600"}
                    >
                      {reason.length} / 30 karakter minimum
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
                variant="default"
                disabled={isSubmitting || reason.trim().length < 30}
                aria-label="Konfirmasi staging valid dan posting akrual"
              >
                {isSubmitting ? "Memproses..." : "Konfirmasi & Posting"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
