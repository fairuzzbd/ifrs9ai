"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import { AlertTriangle, CheckCircle } from "lucide-react";
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
import { mtmOverrideApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import {
  mtmOverrideApproveSchema,
  type MtmOverrideApproveInput,
  type MtmOverrideApproveResponse,
} from "@/lib/schemas/mtm.schema";
import { useQueryClient } from "@tanstack/react-query";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmOverrideApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mtmId: string;
  instrumenKode: string;
  tanggalMtm: string;
  deviationFlag: boolean;
  deltaPct?: number;
  thresholdPct?: number;
  onSuccess?: (response: MtmOverrideApproveResponse) => void;
}

// ---------------------------------------------------------------------------
// DeviationWarningBanner — shown when deviationFlag=TRUE
// ---------------------------------------------------------------------------

function DeviationWarningBanner({
  deltaPct,
  thresholdPct,
}: {
  deltaPct?: number;
  thresholdPct?: number;
}) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
      <AlertTriangle
        className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
        aria-hidden="true"
      />
      <div>
        <p className="font-semibold">Deviasi Harga Signifikan</p>
        <p>
          Delta {deltaPct !== undefined ? `${deltaPct >= 0 ? "+" : ""}${deltaPct.toFixed(2)}%` : "—"}{" "}
          melebihi threshold {thresholdPct !== undefined ? `${thresholdPct.toFixed(2)}%` : "—"}.{" "}
          Pastikan harga telah diverifikasi dari sumber primer (IBPA / Bloomberg) sebelum menyetujui.
        </p>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Component (S4-AC1, S4-AC3 SoD, attest checkbox, comment ≥ 30 char)
// ---------------------------------------------------------------------------

export function MtmOverrideApproveDialog({
  open,
  onOpenChange,
  mtmId,
  instrumenKode,
  tanggalMtm,
  deviationFlag,
  deltaPct,
  thresholdPct,
  onSuccess,
}: MtmOverrideApproveDialogProps) {
  const queryClient = useQueryClient();
  // Idempotency key fresh per dialog open (DEC-021)
  const idempotencyKey = React.useRef(uuidv4());

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
    }
  }, [open]);

  const form = useForm<MtmOverrideApproveInput>({
    resolver: zodResolver(mtmOverrideApproveSchema),
    defaultValues: {
      comment: "",
      signatureMethod: "JWT_STEP_UP",
      attest: undefined,
    },
  });

  const comment = form.watch("comment");
  const attest = form.watch("attest");
  const isSubmitting = form.formState.isSubmitting;

  // Button enabled: comment ≥ 30 char + attest checked
  const canSubmit = (comment?.length ?? 0) >= 30 && attest === true;

  const onSubmit = async (data: MtmOverrideApproveInput) => {
    try {
      const result = await mtmOverrideApi.approve(
        mtmId,
        { comment: data.comment, signatureMethod: data.signatureMethod },
        idempotencyKey.current,
      );

      const resp = result.data;
      notify.success(
        `Override MTM ${instrumenKode} ${tanggalMtm} disetujui. Jurnal ${resp.jurnalEventCodes?.join(", ") ?? ""} berhasil diposting.`,
        {
          action: resp.jurnalEntryId
            ? {
                label: "Lihat Jurnal",
                onClick: () => {
                  window.location.href = `/jurnal/${resp.jurnalEntryId}`;
                },
              }
            : undefined,
        },
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
      // Close dialog on SoD or locked errors (user cannot retry)
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
          <DialogTitle className="flex items-center gap-2">
            <CheckCircle className="h-5 w-5 text-green-600" aria-hidden="true" />
            Override Approve MTM
          </DialogTitle>
          <DialogDescription>
            <strong>{instrumenKode}</strong> — {tanggalMtm}
          </DialogDescription>
        </DialogHeader>

        {/* Deviation warning banner */}
        {deviationFlag && (
          <DeviationWarningBanner deltaPct={deltaPct} thresholdPct={thresholdPct} />
        )}

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {/* Comment */}
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Komentar Persetujuan <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Harga terverifikasi via Bloomberg. Delta wajar karena rilis FOMC. Disetujui."
                      rows={3}
                      maxLength={2000}
                      aria-describedby="approve-comment-hint"
                    />
                  </FormControl>
                  <p id="approve-comment-hint" className="text-xs text-muted-foreground">
                    Minimal 30 karakter. Jelaskan dasar verifikasi harga (Bloomberg, telepon counterparty, dll).{" "}
                    <span className={comment?.length >= 30 ? "text-green-700" : "text-muted-foreground"}>
                      {comment?.length ?? 0}/30
                    </span>
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Attest checkbox */}
            <FormField
              control={form.control}
              name="attest"
              render={({ field }) => (
                <FormItem className="flex flex-row items-start space-x-3 space-y-0 rounded-md border p-3">
                  <FormControl>
                    <Checkbox
                      checked={field.value === true}
                      onCheckedChange={(checked) => field.onChange(checked ? true : undefined)}
                      aria-describedby="attest-hint"
                    />
                  </FormControl>
                  <div className="space-y-1 leading-none">
                    <FormLabel className="text-sm font-normal">
                      Saya menyatakan bahwa harga ini telah diverifikasi dari sumber primer.
                    </FormLabel>
                    <FormMessage />
                  </div>
                </FormItem>
              )}
            />

            {/* SoD note (informational) */}
            <p id="attest-hint" className="text-xs text-muted-foreground">
              Anda bertindak sebagai Finance Controller (override-approver). SoD: Anda tidak boleh menyetujui MTM yang Anda upload sendiri (DEC-017).
            </p>

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
                disabled={isSubmitting || !canSubmit}
                aria-label="Setujui dan posting jurnal MTM"
              >
                {isSubmitting ? "Memproses..." : "Setuju & Posting Jurnal"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
