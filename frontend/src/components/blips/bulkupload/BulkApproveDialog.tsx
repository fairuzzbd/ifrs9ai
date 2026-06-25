"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ShieldCheck } from "lucide-react";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle, DialogTrigger,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { approveFormSchema, type ApproveFormInput } from "@/lib/schemas/bulkupload.schema";

export interface BulkApproveDialogProps {
  batchId: string;
  committedRows: number;
  makerUsername: string;
  onApprove: (data: ApproveFormInput) => Promise<void>;
  disabled?: boolean;
}

export function BulkApproveDialog({
  batchId,
  committedRows,
  makerUsername,
  onApprove,
  disabled,
}: BulkApproveDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const form = useForm<ApproveFormInput>({
    resolver: zodResolver(approveFormSchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STEP_UP" },
  });

  async function handleSubmit(data: ApproveFormInput) {
    setSubmitting(true);
    try {
      await onApprove(data);
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
          variant="default"
          disabled={disabled}
          aria-label={`Approve batch ${batchId}`}
        >
          <ShieldCheck className="h-4 w-4 mr-2" aria-hidden="true" />
          Approve Batch
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Approve Batch {batchId}</DialogTitle>
          <DialogDescription>
            {committedRows} instrumen akan menjadi ACTIVE setelah disetujui.
            <br />
            <span className="font-medium text-amber-700">
              SoD: Anda tidak dapat menyetujui batch yang Anda buat sendiri (maker: {makerUsername}).
            </span>
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="approve-comment">
              Komentar <span aria-hidden="true">*</span>
            </Label>
            <Textarea
              id="approve-comment"
              placeholder="Contoh: Checked — instrumen sesuai daftar penempatan Juni 2026"
              rows={3}
              {...form.register("comment")}
              aria-describedby="approve-comment-error"
              aria-invalid={!!form.formState.errors.comment}
              disabled={submitting}
            />
            {form.formState.errors.comment && (
              <p id="approve-comment-error" role="alert" className="text-xs text-destructive">
                {form.formState.errors.comment.message}
              </p>
            )}
          </div>

          <p className="text-xs text-muted-foreground">
            Dengan klik Approve, Anda menandatangani secara digital menggunakan metode JWT_STEP_UP
            sesuai DEC-027. Aksi ini dicatat di audit log (BULK.APPROVED) dan tidak dapat dibatalkan.
          </p>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={submitting}>
              Batal
            </Button>
            <Button type="submit" disabled={submitting}>
              {submitting ? "Memproses..." : "Approve"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
