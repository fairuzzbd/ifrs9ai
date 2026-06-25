/**
 * MappingApproveDialog — workflow approval dialogs.
 * Handles both 4-eyes (ROLE-AKUN-CTL) and 6-eyes (ROLE-RISK, step-up MFA).
 * DEC-027: approve-2 requires X-Step-Up-Token.
 */

"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Shield, CheckCircle2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { MFAStepUpModal, getStepUpToken, setStepUpToken } from "@/components/blips/MFAStepUpModal";
import { approveDialogSchema, type ApproveDialogInput } from "@/lib/schemas/mapping-jurnal-p12.schema";

type ApproveVariant = "4-eyes" | "6-eyes";

interface MappingApproveDialogProps {
  eventCode: string;
  variant: ApproveVariant;
  disabled?: boolean;
  onApprove: (comment: string, stepUpToken?: string) => Promise<void>;
  children?: React.ReactNode;
}

export function MappingApproveDialog({
  eventCode,
  variant,
  disabled,
  onApprove,
  children,
}: MappingApproveDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [attested, setAttested] = React.useState(false);
  const [mfaOpen, setMfaOpen] = React.useState(false);
  const [pendingComment, setPendingComment] = React.useState("");

  const is6Eyes = variant === "6-eyes";

  const form = useForm<ApproveDialogInput>({
    resolver: zodResolver(approveDialogSchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STEP_UP" },
  });

  const handleFormSubmit = async (values: ApproveDialogInput) => {
    if (is6Eyes) {
      const existing = getStepUpToken();
      if (existing) {
        await doApprove(values.comment, existing);
      } else {
        setPendingComment(values.comment);
        setMfaOpen(true);
      }
    } else {
      await doApprove(values.comment);
    }
  };

  const doApprove = async (comment: string, stepUpToken?: string) => {
    setSubmitting(true);
    try {
      await onApprove(comment, stepUpToken);
      setOpen(false);
      form.reset();
      setAttested(false);
    } finally {
      setSubmitting(false);
    }
  };

  const handleMfaVerified = async (token: string) => {
    setStepUpToken(token);
    await doApprove(pendingComment, token);
  };

  const watchComment = form.watch("comment");

  return (
    <>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogTrigger asChild>
          {children ?? (
            <Button
              size="sm"
              disabled={disabled}
              aria-label={`Approve mapping ${eventCode} ${is6Eyes ? "(6-eyes, MFA)" : "(4-eyes)"}`}
            >
              <CheckCircle2 className="mr-1.5 h-4 w-4" aria-hidden="true" />
              {is6Eyes ? "Approve (MFA)" : "Approve"}
            </Button>
          )}
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {is6Eyes ? "Approve Mapping (ROLE-RISK)" : "Approve Mapping"}
            </DialogTitle>
            <DialogDescription>
              Approve mapping <strong>{eventCode}</strong>.
              {is6Eyes
                ? " Anda adalah approver kedua (6-eyes). Diperlukan MFA step-up (DEC-027). Setelah disetujui, mapping menjadi APPROVED_ACTIVE."
                : " Setelah disetujui, mapping menjadi APPROVED_ACTIVE dan dapat digunakan resolver."}
            </DialogDescription>
          </DialogHeader>

          {is6Eyes && (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800"
            >
              <Shield className="h-3.5 w-3.5 mt-0.5 shrink-0" aria-hidden="true" />
              <span>
                Langkah ini memerlukan <strong>MFA step-up</strong> (DEC-027).
                Verifikasi MFA akan diminta saat Approve diklik.
              </span>
            </div>
          )}

          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleFormSubmit)} className="space-y-4">
              <FormField
                control={form.control}
                name="comment"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Komentar <span className="text-destructive">*</span>
                      <span className="ml-1 text-xs text-muted-foreground">(min 10 karakter)</span>
                    </FormLabel>
                    <FormControl>
                      <Textarea
                        {...field}
                        rows={3}
                        placeholder={
                          is6Eyes
                            ? "Mapping ECL sesuai PSAK 71 §5.5 — disetujui"
                            : "Akun mapping sudah diverifikasi..."
                        }
                        disabled={submitting}
                        aria-describedby="approve-comment-counter"
                      />
                    </FormControl>
                    <div
                      id="approve-comment-counter"
                      className={`text-right text-xs ${
                        watchComment.length < 10 ? "text-destructive" : "text-muted-foreground"
                      }`}
                    >
                      {watchComment.length} / 10 karakter minimum
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="flex items-start gap-3 rounded-md border p-3">
                <Checkbox
                  id="approve-attest"
                  checked={attested}
                  onCheckedChange={(v) => setAttested(v === true)}
                  disabled={submitting}
                  aria-describedby="approve-attest-label"
                />
                <Label
                  id="approve-attest-label"
                  htmlFor="approve-attest"
                  className="cursor-pointer text-xs leading-relaxed"
                >
                  {is6Eyes
                    ? "Saya menyatakan mapping ini telah diverifikasi sesuai PSAK 71 dan COA yang berlaku, serta approve-2 valid secara compliance."
                    : "Saya menyatakan telah meverifikasi mapping ini sesuai kebijakan akuntansi yang berlaku."}
                </Label>
              </div>

              <DialogFooter>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setOpen(false)}
                  disabled={submitting}
                >
                  Batal
                </Button>
                <Button
                  type="submit"
                  disabled={submitting || !attested}
                  aria-busy={submitting}
                >
                  {submitting
                    ? "Memproses..."
                    : is6Eyes
                      ? "Approve (MFA)"
                      : "Approve"}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      <MFAStepUpModal
        open={mfaOpen}
        onOpenChange={setMfaOpen}
        title="Verifikasi MFA untuk Approve Mapping"
        description="Step-up MFA diperlukan untuk approve mapping regulated (DEC-027, scope: mapping_approve)"
        onVerified={handleMfaVerified}
      />
    </>
  );
}
