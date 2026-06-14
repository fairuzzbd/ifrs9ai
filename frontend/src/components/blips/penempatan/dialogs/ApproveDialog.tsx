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

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const schema = z.object({
  comment: z.string().min(1, "Komentar wajib diisi").max(1000),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini",
  }),
});

type FormValues = z.infer<typeof schema>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ApproveDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kodeTransaksi: string;
  sodBlocked: boolean;
  /** Called with (comment, stepUpToken) after MFA verified */
  onConfirm: (comment: string, stepUpToken: string) => Promise<void>;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ApproveDialog({
  open,
  onOpenChange,
  kodeTransaksi,
  sodBlocked,
  onConfirm,
}: ApproveDialogProps) {
  const [submitting, setSubmitting] = React.useState(false);
  const [showMFA, setShowMFA] = React.useState(false);
  const [pendingComment, setPendingComment] = React.useState<string | null>(null);

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { comment: "", attestChecked: false },
  });

  React.useEffect(() => {
    if (!open) {
      form.reset();
      setPendingComment(null);
    }
  }, [open, form]);

  const handleFormSubmit = (values: FormValues) => {
    if (sodBlocked) return;
    // Check MFA freshness
    if (isMFAFresh()) {
      const token = getStepUpToken()!;
      void doApprove(values.comment, token);
    } else {
      setPendingComment(values.comment);
      setShowMFA(true);
    }
  };

  const doApprove = async (comment: string, stepUpToken: string) => {
    setSubmitting(true);
    try {
      await onConfirm(comment, stepUpToken);
      onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  };

  const handleMFAVerified = (token: string) => {
    setStepUpToken(token);
    if (pendingComment != null) {
      void doApprove(pendingComment, token);
    }
  };

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-w-md" aria-describedby="approve-desc">
          <DialogHeader>
            <DialogTitle>Approve Penempatan {kodeTransaksi}</DialogTitle>
            <DialogDescription id="approve-desc">
              Persetujuan final untuk penempatan ini memerlukan verifikasi MFA (DEC-027).
            </DialogDescription>
          </DialogHeader>

          {sodBlocked && (
            <SodBlockBanner message="Anda tidak bisa menjadi approver untuk penempatan yang Anda buat atau review sendiri (SoD — DEC-017)." />
          )}

          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleFormSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="comment"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Komentar Approve <span aria-hidden="true">*</span>
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        placeholder="Berikan catatan persetujuan..."
                        rows={3}
                        aria-required="true"
                        disabled={sodBlocked}
                      />
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
                      <Checkbox
                        checked={field.value}
                        onCheckedChange={field.onChange}
                        id="approve-attest"
                        disabled={sodBlocked}
                      />
                    </FormControl>
                    <Label htmlFor="approve-attest" className="text-sm leading-snug cursor-pointer">
                      Saya menyatakan penempatan ini sesuai dengan batas investasi dan RKAP yang
                      berlaku, dan menyetujui pemrosesan EIR serta staging ECL otomatis.
                    </Label>
                  </FormItem>
                )}
              />

              <div className="flex justify-end gap-2 pt-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => onOpenChange(false)}
                  disabled={submitting}
                >
                  Batal
                </Button>
                <Button type="submit" disabled={submitting || sodBlocked}>
                  {submitting && (
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                  )}
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
        title="Verifikasi MFA diperlukan untuk menyetujui penempatan"
        description="Masukkan kode TOTP 6 digit dari aplikasi authenticator Anda."
        onVerified={handleMFAVerified}
        onCancel={() => setPendingComment(null)}
      />
    </>
  );
}
