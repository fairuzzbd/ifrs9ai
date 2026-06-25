"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ShieldAlert } from "lucide-react";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  rollbackApproveFormSchema,
  type RollbackApproveFormInput,
} from "@/lib/schemas/bulkupload.schema";

export interface BulkRollbackApproveDialogProps {
  batchId: string;
  rolledBackCount: number;
  onApprove: (data: RollbackApproveFormInput, stepUpToken: string) => Promise<void>;
  disabled?: boolean;
}

/**
 * BulkRollbackApproveDialog — CFO confirms rollback with step-up MFA token.
 * DEC-027: scope=bulk_rollback, freshness ≤ 5 menit.
 * Soft-delete only (DEC-018). Irreversible.
 */
export function BulkRollbackApproveDialog({
  batchId,
  rolledBackCount,
  onApprove,
  disabled,
}: BulkRollbackApproveDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [stepUpToken, setStepUpToken] = React.useState("");

  const form = useForm<RollbackApproveFormInput>({
    resolver: zodResolver(rollbackApproveFormSchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STEP_UP" },
  });

  async function handleSubmit(data: RollbackApproveFormInput) {
    if (!stepUpToken.trim()) {
      return;
    }
    setSubmitting(true);
    try {
      await onApprove(data, stepUpToken.trim());
      setOpen(false);
      form.reset();
      setStepUpToken("");
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
          aria-label={`Konfirmasi rollback batch ${batchId} dengan step-up MFA`}
        >
          <ShieldAlert className="h-4 w-4 mr-2" aria-hidden="true" />
          Konfirmasi Rollback (MFA)
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Konfirmasi Rollback Batch {batchId}</DialogTitle>
          <DialogDescription>
            <span className="block font-semibold text-red-700 mb-1">
              PERINGATAN: Aksi irreversible.
            </span>
            {rolledBackCount} instrumen akan di-soft-delete (DEC-018).
            Step-up MFA wajib (scope: bulk_rollback, freshness ≤ 5 menit, DEC-027).
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="step-up-token">
              Token Step-Up MFA <span aria-hidden="true">*</span>
            </Label>
            <Input
              id="step-up-token"
              type="password"
              placeholder="Token dari X-Step-Up-Token (scope=bulk_rollback)"
              value={stepUpToken}
              onChange={(e) => setStepUpToken(e.target.value)}
              required
              aria-required="true"
              aria-describedby="step-up-hint"
              disabled={submitting}
            />
            <p id="step-up-hint" className="text-xs text-muted-foreground">
              Dapatkan token via endpoint /auth/step-up?scope=bulk_rollback. Berlaku 5 menit.
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="rollback-approve-comment">
              Komentar <span aria-hidden="true">*</span>
            </Label>
            <Textarea
              id="rollback-approve-comment"
              placeholder="Contoh: Rollback disetujui — counterparty mapping telah dikonfirmasi error."
              rows={3}
              {...form.register("comment")}
              aria-describedby="rollback-approve-comment-error"
              aria-invalid={!!form.formState.errors.comment}
              disabled={submitting}
            />
            {form.formState.errors.comment && (
              <p id="rollback-approve-comment-error" role="alert" className="text-xs text-destructive">
                {form.formState.errors.comment.message}
              </p>
            )}
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={submitting}>
              Batal
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={submitting || !stepUpToken.trim()}
            >
              {submitting ? "Memproses..." : "Konfirmasi Rollback"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
