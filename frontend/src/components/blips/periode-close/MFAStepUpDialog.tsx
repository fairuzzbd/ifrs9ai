"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ShieldAlert, Loader2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { cn } from "@/lib/utils";
import { mfaStepUpRequestSchema } from "@/lib/schemas/periode-close.schema";
import type { MfaStepUpScope, MfaStepUpRequest } from "@/lib/schemas/periode-close.schema";
import { mfaStepUpApi } from "@/lib/api/periode-close.api";
import { isApiError } from "@/lib/api";

// ---------------------------------------------------------------------------
// Props (matches design spec §3)
// ---------------------------------------------------------------------------

export interface MFAStepUpDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scope: MfaStepUpScope;
  actionDescription: string;
  periodeKode: string;
  onTokenReceived: (token: string) => void;
}

// ---------------------------------------------------------------------------
// Scope descriptions
// ---------------------------------------------------------------------------

const SCOPE_DESCRIPTIONS: Record<MfaStepUpScope, string> = {
  hard_close:
    "Aksi ini bersifat permanen setelah grace window 48 jam berakhir. Tidak bisa di-reverse via API.",
  reopen_closed:
    "Reopen periode CLOSED ke SOFT_CLOSED. Tindakan exceptional — akan dicatat permanen di audit trail.",
};

// ---------------------------------------------------------------------------
// Component (S3-AC2, S3-AC3, S4-AC2)
// ---------------------------------------------------------------------------

export function MFAStepUpDialog({
  open,
  onOpenChange,
  scope,
  actionDescription,
  periodeKode,
  onTokenReceived,
}: MFAStepUpDialogProps) {
  const [mfaError, setMfaError] = React.useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = React.useState(false);

  const form = useForm<MfaStepUpRequest>({
    resolver: zodResolver(mfaStepUpRequestSchema),
    defaultValues: { totpCode: "", scope },
  });

  // Focus on first input when dialog opens
  const inputRef = React.useRef<HTMLInputElement>(null);
  React.useEffect(() => {
    if (open) {
      setMfaError(null);
      form.reset({ totpCode: "", scope });
      // Small delay for dialog animation
      setTimeout(() => inputRef.current?.focus(), 100);
    }
  }, [open, form, scope]);

  const onSubmit = async (data: MfaStepUpRequest) => {
    setIsSubmitting(true);
    setMfaError(null);
    try {
      const res = await mfaStepUpApi.challenge(data);
      onOpenChange(false);
      onTokenReceived(res.data.stepUpToken);
    } catch (err) {
      if (isApiError(err)) {
        if (err.code === "MFA_STEP_UP_EXPIRED") {
          setMfaError("Token MFA sudah expired (> 5 menit). Harap ulangi verifikasi dari awal.");
        } else {
          setMfaError("Kode salah atau sudah dipakai. Tunggu kode baru atau gunakan kode cadangan.");
        }
      } else {
        setMfaError("Gagal menghubungi server. Coba lagi.");
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  const totpValue = form.watch("totpCode");
  const isValid = totpValue.length === 6 && /^\d{6}$/.test(totpValue);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-md"
        aria-labelledby="mfa-dialog-title"
        onInteractOutside={(e) => e.preventDefault()} // prevent accidental close
      >
        <DialogHeader>
          <DialogTitle id="mfa-dialog-title" className="flex items-center gap-2">
            <ShieldAlert className="h-5 w-5 text-orange-600" aria-hidden="true" />
            Verifikasi MFA Tambahan
          </DialogTitle>
        </DialogHeader>

        {/* Action context */}
        <div className="rounded-md bg-orange-50 border border-orange-200 px-4 py-3 space-y-1">
          <p className="text-sm font-medium text-orange-800">
            {actionDescription}
          </p>
          <p className="text-xs text-orange-700">
            {SCOPE_DESCRIPTIONS[scope]}
          </p>
          <p className="text-xs text-orange-600 font-mono">{periodeKode}</p>
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="totpCode"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Masukkan kode TOTP dari aplikasi autentikator Anda:
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      ref={(el) => {
                        field.ref(el);
                        (inputRef as React.MutableRefObject<HTMLInputElement | null>).current = el;
                      }}
                      type="text"
                      inputMode="numeric"
                      pattern="[0-9]*"
                      maxLength={6}
                      placeholder="_ _ _ _ _ _"
                      className={cn(
                        "text-center text-2xl font-mono tracking-widest letter-spacing-8",
                        "h-14 text-center",
                        mfaError && "border-destructive focus-visible:ring-destructive",
                      )}
                      autoComplete="one-time-code"
                      aria-describedby={mfaError ? "mfa-error-msg" : "mfa-hint"}
                      aria-invalid={!!mfaError}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* MFA error state */}
            {mfaError && (
              <div
                id="mfa-error-msg"
                role="alert"
                className="rounded-md bg-red-50 border border-red-200 px-3 py-2 flex items-start gap-2"
              >
                <ShieldAlert className="h-4 w-4 text-red-600 mt-0.5 shrink-0" aria-hidden="true" />
                <div className="text-sm text-red-700">
                  {mfaError}
                  {mfaError.includes("expired") && (
                    <Button
                      variant="link"
                      size="sm"
                      type="button"
                      className="text-red-700 underline p-0 h-auto ml-1"
                      onClick={() => {
                        form.reset({ totpCode: "", scope });
                        setMfaError(null);
                      }}
                    >
                      [Ulangi]
                    </Button>
                  )}
                </div>
              </div>
            )}

            {/* Hint */}
            {!mfaError && (
              <p id="mfa-hint" className="text-xs text-muted-foreground">
                Token berlaku 5 menit setelah verifikasi.
              </p>
            )}

            {/* Actions */}
            <div className="flex justify-end gap-3 pt-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={isSubmitting}
              >
                Batal
              </Button>
              <Button
                type="submit"
                disabled={!isValid || isSubmitting}
                aria-label="Verifikasi kode MFA dan lanjutkan"
              >
                {isSubmitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Memverifikasi...
                  </>
                ) : (
                  "Verifikasi & Lanjutkan"
                )}
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
