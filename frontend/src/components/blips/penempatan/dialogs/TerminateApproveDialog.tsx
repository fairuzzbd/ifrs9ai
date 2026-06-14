"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { SodBlockBanner } from "@/components/blips/SodBlockBanner";
import {
  MFAStepUpModal,
  getStepUpToken,
  setStepUpToken,
  isMFAFresh,
} from "@/components/blips/MFAStepUpModal";
import { Loader2 } from "lucide-react";

const schema = z.object({
  comment: z.string().min(1, "Komentar wajib diisi").max(1000),
  attestChecked: z.boolean().refine((v) => v, { message: "Anda harus mencentang pernyataan ini" }),
});
type FormValues = z.infer<typeof schema>;

interface TerminateApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kodeTransaksi: string;
  sodBlocked: boolean;
  onConfirm: (comment: string, stepUpToken: string) => Promise<void>;
}

export function TerminateApproveDialog({
  open,
  onOpenChange,
  kodeTransaksi,
  sodBlocked,
  onConfirm,
}: TerminateApproveDialogProps) {
  const [submitting, setSubmitting] = React.useState(false);
  const [showMFA, setShowMFA] = React.useState(false);
  const [pendingComment, setPendingComment] = React.useState<string | null>(null);

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { comment: "", attestChecked: false },
  });

  React.useEffect(() => { if (!open) { form.reset(); setPendingComment(null); } }, [open, form]);

  const handleFormSubmit = (values: FormValues) => {
    if (sodBlocked) return;
    if (isMFAFresh()) {
      void doApprove(values.comment, getStepUpToken()!);
    } else {
      setPendingComment(values.comment);
      setShowMFA(true);
    }
  };

  const doApprove = async (comment: string, token: string) => {
    setSubmitting(true);
    try {
      await onConfirm(comment, token);
      onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  };

  const handleMFAVerified = (token: string) => {
    setStepUpToken(token);
    if (pendingComment != null) void doApprove(pendingComment, token);
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-md" aria-describedby="term-approve-desc">
          <DialogHeader>
            <DialogTitle>Approve Terminasi {kodeTransaksi}</DialogTitle>
            <DialogDescription id="term-approve-desc">
              Persetujuan terminasi final memerlukan verifikasi MFA (DEC-027).
            </DialogDescription>
          </DialogHeader>

          {sodBlocked && (
            <SodBlockBanner message="Anda tidak bisa menjadi approver untuk proposal terminasi yang Anda buat atau review sendiri." />
          )}

          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleFormSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="comment"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Komentar Approve Terminasi <span aria-hidden="true">*</span></FormLabel>
                    <FormControl>
                      <Textarea {...field} rows={3} disabled={sodBlocked} aria-required="true" />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="attestChecked"
                render={({ field }) => (
                  <FormItem className="flex items-start gap-3 space-y-0">
                    <FormControl>
                      <Checkbox checked={field.value} onCheckedChange={field.onChange} id="term-approve-attest" disabled={sodBlocked} />
                    </FormControl>
                    <Label htmlFor="term-approve-attest" className="text-sm leading-snug cursor-pointer">
                      Saya menyetujui terminasi dini instrumen ini. Saya memahami bahwa EIR catch-up,
                      ECL reversal, dan realized gain/loss akan diproses otomatis (P5-M9).
                    </Label>
                  </FormItem>
                )}
              />

              <div className="flex justify-end gap-2 pt-2">
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>Batal</Button>
                <Button type="submit" disabled={submitting || sodBlocked}>
                  {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
                  Lanjut ke Verifikasi MFA
                </Button>
              </div>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      <MFAStepUpModal
        open={showMFA}
        onOpenChange={setShowMFA}
        title="Verifikasi MFA diperlukan untuk approve terminasi"
        onVerified={handleMFAVerified}
        onCancel={() => setPendingComment(null)}
      />
    </>
  );
}
