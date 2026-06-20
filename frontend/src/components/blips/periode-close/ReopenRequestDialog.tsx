"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Loader2, RotateCcw } from "lucide-react";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { reopenRequestBodySchema } from "@/lib/schemas/periode-close.schema";
import type { ReopenRequestBody, StatusPeriode } from "@/lib/schemas/periode-close.schema";
import { periodeReopenApi } from "@/lib/api/periode-close.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface ReopenRequestDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  periodeId: string;
  periodeKode: string;
  currentStatus: StatusPeriode;
  rowVersion: number;
  onSuccess: () => void;
}

// ---------------------------------------------------------------------------
// Target status options by current status
// ---------------------------------------------------------------------------

type ReopenTargetStatus = "OPEN" | "SOFT_CLOSED";

interface TargetOption {
  value: ReopenTargetStatus;
  label: string;
  warning?: string;
}

function getTargetOptions(currentStatus: StatusPeriode): TargetOption[] {
  if (currentStatus === "CLOSED") {
    return [
      {
        value: "SOFT_CLOSED" as ReopenTargetStatus,
        label: "SOFT_CLOSED — kembalikan ke soft-close",
        warning:
          "Reopen CLOSED→SOFT_CLOSED memerlukan step-up MFA CFO dan hanya dalam grace window 48 jam.",
      },
    ];
  }
  if (currentStatus === "SOFT_CLOSED") {
    return [
      {
        value: "OPEN" as ReopenTargetStatus,
        label: "OPEN — buka kembali untuk koreksi",
      },
    ];
  }
  return [];
}

// ---------------------------------------------------------------------------
// Component (S4-AC1)
// ---------------------------------------------------------------------------

export function ReopenRequestDialog({
  open,
  onOpenChange,
  periodeId,
  periodeKode,
  currentStatus,
  rowVersion,
  onSuccess,
}: ReopenRequestDialogProps) {
  const [isSubmitting, setIsSubmitting] = React.useState(false);
  const idempotencyKey = React.useRef(uuidv4());
  const targetOptions = getTargetOptions(currentStatus);

  const form = useForm<ReopenRequestBody>({
    resolver: zodResolver(reopenRequestBodySchema),
    defaultValues: {
      reason: "",
      targetStatus: targetOptions[0]?.value ?? "SOFT_CLOSED",
      rowVersion,
    },
  });

  React.useEffect(() => {
    if (open) {
      idempotencyKey.current = uuidv4();
      form.reset({
        reason: "",
        targetStatus: targetOptions[0]?.value ?? "SOFT_CLOSED",
        rowVersion,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, rowVersion]);

  const onSubmit = async (data: ReopenRequestBody) => {
    setIsSubmitting(true);
    try {
      await periodeReopenApi.request(periodeId, data, idempotencyKey.current);
      const toStatus =
        data.targetStatus === "OPEN" ? "OPEN" : "SOFT_CLOSED";
      const extra =
        toStatus === "SOFT_CLOSED"
          ? " Menunggu konfirmasi dengan step-up MFA."
          : " Periode dapat dikoreksi kembali.";
      notify.success(
        `Reopen request ${periodeKode} ke ${toStatus} berhasil diajukan.${extra}`,
      );
      onOpenChange(false);
      onSuccess();
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "PERIODE_GRACE_EXPIRED") {
          notify.error(err, {
            action: {
              label: "Lihat periode",
              onClick: () => onOpenChange(false),
            },
          });
        } else {
          notify.error(err);
        }
      } else {
        notify.error({ code: "NETWORK_ERROR", message: "Gagal menghubungi server.", traceId: "" });
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const selectedTarget = form.watch("targetStatus");
  const selectedOption = targetOptions.find((o) => o.value === selectedTarget);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" aria-labelledby="reopen-req-title">
        <DialogHeader>
          <DialogTitle id="reopen-req-title" className="flex items-center gap-2">
            <RotateCcw className="h-5 w-5 text-orange-600" aria-hidden="true" />
            Reopen Periode — {periodeKode}
          </DialogTitle>
        </DialogHeader>

        <p className="text-sm text-muted-foreground">
          Status saat ini:{" "}
          <span className="font-mono font-medium">{currentStatus}</span>.
        </p>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {/* Target status — only show if there are multiple options */}
            {targetOptions.length > 1 && (
              <FormField
                control={form.control}
                name="targetStatus"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Target Status</FormLabel>
                    <Select onValueChange={field.onChange} defaultValue={field.value}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="Pilih target status" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {targetOptions.map((opt) => (
                          <SelectItem key={opt.value} value={opt.value}>
                            {opt.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {/* Warning for CLOSED→SOFT_CLOSED */}
            {selectedOption?.warning && (
              <div className="rounded-md bg-orange-50 border border-orange-200 px-3 py-2">
                <p className="text-xs text-orange-700">{selectedOption.warning}</p>
              </div>
            )}

            {/* Reason */}
            <FormField
              control={form.control}
              name="reason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Reopen <span aria-hidden="true">*</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Mis: Koreksi jurnal akun 2030 diperlukan akibat temuan audit internal. Perbedaan IDR 15.000.000 perlu disesuaikan."
                      rows={4}
                      maxLength={2000}
                      aria-required="true"
                      aria-describedby="reopen-reason-hint"
                    />
                  </FormControl>
                  <p id="reopen-reason-hint" className="text-xs text-muted-foreground">
                    Minimal 30 karakter. Saat ini: {(field.value as string | undefined)?.length ?? 0} karakter.
                  </p>
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
                  "Ajukan Reopen"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
