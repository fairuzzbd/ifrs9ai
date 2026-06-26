"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { RotateCcw } from "lucide-react";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  rollbackRequestFormSchema,
  type RollbackRequestFormInput,
} from "@/lib/schemas/bulkupload.schema";

export interface BulkRollbackRequestDialogProps {
  batchId: string;
  graceExpiresAt: string | null | undefined;
  onRequest: (data: RollbackRequestFormInput) => Promise<void>;
  disabled?: boolean;
}

export function BulkRollbackRequestDialog({
  batchId,
  graceExpiresAt,
  onRequest,
  disabled,
}: BulkRollbackRequestDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const form = useForm<RollbackRequestFormInput>({
    resolver: zodResolver(rollbackRequestFormSchema),
    defaultValues: { reason: "" },
  });

  const reasonValue = form.watch("reason");
  const charCount = reasonValue.length;

  async function handleSubmit(data: RollbackRequestFormInput) {
    setSubmitting(true);
    try {
      await onRequest(data);
      setOpen(false);
      form.reset();
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button
          variant="destructive"
          disabled={disabled}
          aria-label={`Ajukan rollback batch ${batchId}`}
        >
          <RotateCcw className="h-4 w-4 mr-2" aria-hidden="true" />
          Ajukan Rollback
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Ajukan Rollback Batch {batchId}</DialogTitle>
          <DialogDescription className="space-y-1">
            <span>
              Rollback akan soft-delete semua instrumen dari batch ini (DEC-018).
              Aksi ini memerlukan konfirmasi step-up MFA di langkah berikutnya.
            </span>
            {graceExpiresAt && (
              <span className="block text-amber-700 text-xs">
                Grace window berakhir:{" "}
                {new Date(graceExpiresAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}
              </span>
            )}
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="rollback-reason">
              Alasan Rollback <span aria-hidden="true">*</span>
            </Label>
            <Textarea
              id="rollback-reason"
              placeholder="Minimal 50 karakter. Contoh: Error counterparty mapping ditemukan post-commit. Diperlukan rollback untuk koreksi."
              rows={4}
              {...form.register("reason")}
              aria-describedby="rollback-reason-error rollback-reason-hint"
              aria-invalid={!!form.formState.errors.reason}
              disabled={submitting}
            />
            <div className="flex justify-between items-center">
              <p
                id="rollback-reason-hint"
                className={charCount < 50 ? "text-xs text-amber-600" : "text-xs text-muted-foreground"}
              >
                {charCount}/50 karakter minimum
              </p>
              {form.formState.errors.reason && (
                <p id="rollback-reason-error" role="alert" className="text-xs text-destructive">
                  {form.formState.errors.reason.message}
                </p>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={submitting}>
              Batal
            </Button>
            <Button type="submit" variant="destructive" disabled={submitting}>
              {submitting ? "Memproses..." : "Ajukan Rollback"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
