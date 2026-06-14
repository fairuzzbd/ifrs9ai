"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { SodBlockBanner } from "@/components/blips/SodBlockBanner";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------

const approveSchema = z.object({
  comment: z.string().max(1000).optional(),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini sebelum melanjutkan",
  }),
});

const rejectSchema = z.object({
  comment: z
    .string()
    .min(10, "Alasan penolakan minimal 10 karakter")
    .max(1000, "Maksimal 1000 karakter"),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini sebelum melanjutkan",
  }),
});

type ApproveFormValues = z.infer<typeof approveSchema>;
type RejectFormValues = z.infer<typeof rejectSchema>;

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ApprovalWithSignatureProps {
  actionType: "review" | "approve";
  sodBlocked: boolean;
  sodMessage?: string;
  submitting?: boolean;
  /**
   * When true, signals that this approval step requires MFA step-up (DEC-027).
   * A warning banner is shown to the approver before they act.
   * The calling code is responsible for passing signatureMethod="JWT_STEP_UP" to the API.
   */
  requireStepUpMfa?: boolean;
  onApprove: (comment: string | undefined) => void;
  onReject: (comment: string) => void;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ApprovalWithSignature({
  actionType,
  sodBlocked,
  sodMessage,
  submitting = false,
  requireStepUpMfa = false,
  onApprove,
  onReject,
  className,
}: ApprovalWithSignatureProps) {
  const [showRejectPanel, setShowRejectPanel] = React.useState(false);

  const approveForm = useForm<ApproveFormValues>({
    resolver: zodResolver(approveSchema),
    defaultValues: { comment: "", attestChecked: false },
  });

  const rejectForm = useForm<RejectFormValues>({
    resolver: zodResolver(rejectSchema),
    defaultValues: { comment: "", attestChecked: false },
  });

  const approveComment = approveForm.watch("comment") ?? "";
  const approveAttested = approveForm.watch("attestChecked");
  const rejectComment = rejectForm.watch("comment") ?? "";
  const rejectCommentLen = rejectComment.length;
  const approveCommentLen = approveComment.length;
  const rejectAttested = rejectForm.watch("attestChecked");

  if (sodBlocked) {
    return (
      <div className={className}>
        <SodBlockBanner message={sodMessage} />
      </div>
    );
  }

  if (showRejectPanel) {
    return (
      <div className={cn("space-y-4", className)}>
        <h4 className="text-sm font-semibold text-destructive">
          Tolak &amp; Kembalikan ke Maker
        </h4>
        <Separator />

        <div className="space-y-2">
          <Label htmlFor="reject-comment" className="text-sm font-medium">
            Alasan Penolakan{" "}
            <span className="text-destructive" aria-hidden>
              *
            </span>
          </Label>
          <Textarea
            id="reject-comment"
            placeholder="Jelaskan alasan penolakan..."
            rows={4}
            aria-required="true"
            aria-describedby="reject-comment-desc"
            {...rejectForm.register("comment")}
          />
          <div
            id="reject-comment-desc"
            className="flex justify-between text-xs text-muted-foreground"
          >
            <span>Minimal 10 karakter. Wajib diisi.</span>
            <span
              className={cn(
                rejectCommentLen < 10 ? "text-destructive" : "",
              )}
            >
              Sisa: {1000 - rejectCommentLen} karakter
            </span>
          </div>
          {rejectForm.formState.errors.comment && (
            <p role="alert" className="text-sm text-destructive">
              {rejectForm.formState.errors.comment.message}
            </p>
          )}
        </div>

        <div className="flex items-start gap-3 rounded-md border p-3">
          <Checkbox
            id="reject-attest"
            checked={rejectForm.watch("attestChecked")}
            onCheckedChange={(v) =>
              rejectForm.setValue("attestChecked", v === true)
            }
            aria-describedby="reject-attest-label"
          />
          <Label
            id="reject-attest-label"
            htmlFor="reject-attest"
            className="cursor-pointer text-sm leading-relaxed"
          >
            Saya menyatakan bahwa penolakan ini berdasarkan pertimbangan yang
            valid.
          </Label>
        </div>
        {rejectForm.formState.errors.attestChecked && (
          <p role="alert" className="text-sm text-destructive">
            {rejectForm.formState.errors.attestChecked.message}
          </p>
        )}

        <div className="flex gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setShowRejectPanel(false)}
            disabled={submitting}
          >
            Batal
          </Button>
          <Button
            variant="destructive"
            size="sm"
            disabled={
              submitting || rejectCommentLen < 10 || !rejectAttested
            }
            onClick={rejectForm.handleSubmit((v) => onReject(v.comment))}
          >
            {submitting ? "Memproses..." : "Tolak & Kembalikan"}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className={cn("space-y-4", className)}>
      <h4 className="text-sm font-semibold">Tindakan</h4>
      <Separator />

      {/* MFA step-up warning banner */}
      {requireStepUpMfa && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-2"
        >
          <svg
            className="mt-0.5 h-4 w-4 shrink-0 text-amber-600"
            viewBox="0 0 16 16"
            fill="currentColor"
            aria-hidden
          >
            <path d="M8 1a7 7 0 1 0 0 14A7 7 0 0 0 8 1zm0 11a1 1 0 1 1 0-2 1 1 0 0 1 0 2zm.75-4.25a.75.75 0 0 1-1.5 0V5a.75.75 0 0 1 1.5 0v2.75z" />
          </svg>
          <p className="text-xs text-amber-800">
            Langkah ini memerlukan <strong>MFA step-up</strong> (DEC-027).
            Pastikan Anda sudah melakukan verifikasi MFA terkini sebelum memberikan persetujuan.
          </p>
        </div>
      )}

      {/* Comment textarea */}
      <div className="space-y-2">
        <Label htmlFor="approve-comment" className="text-sm font-medium">
          Komentar{" "}
          <span className="text-xs text-muted-foreground">(opsional)</span>
        </Label>
        <Textarea
          id="approve-comment"
          placeholder="Tambahkan komentar..."
          rows={3}
          aria-describedby="approve-comment-desc"
          {...approveForm.register("comment")}
        />
        <p
          id="approve-comment-desc"
          className="flex justify-end text-xs text-muted-foreground"
        >
          Sisa: {1000 - approveCommentLen} karakter
        </p>
      </div>

      {/* Attestation checkbox */}
      <div className="flex items-start gap-3 rounded-md border p-3">
        <Checkbox
          id="approve-attest"
          checked={approveAttested}
          onCheckedChange={(v) =>
            approveForm.setValue("attestChecked", v === true)
          }
          aria-describedby="approve-attest-label"
        />
        <Label
          id="approve-attest-label"
          htmlFor="approve-attest"
          className="cursor-pointer text-sm leading-relaxed"
        >
          Saya menyatakan bahwa data ini telah saya periksa dan sesuai dengan
          standar yang berlaku.
        </Label>
      </div>
      {approveForm.formState.errors.attestChecked && (
        <p role="alert" className="text-sm text-destructive">
          {approveForm.formState.errors.attestChecked.message}
        </p>
      )}

      {/* SoD note */}
      <p className="text-xs text-muted-foreground">
        Catatan: Anda tidak bisa{" "}
        {actionType === "review" ? "mereview" : "menyetujui"} data yang Anda
        buat sendiri (Segregation of Duties).
      </p>

      {/* Action buttons */}
      <div className="flex gap-3">
        <Button
          variant="outline"
          size="sm"
          disabled={submitting}
          onClick={() => setShowRejectPanel(true)}
        >
          Tolak
        </Button>
        <Button
          size="sm"
          disabled={submitting || !approveAttested}
          onClick={approveForm.handleSubmit((v) => onApprove(v.comment || undefined))}
        >
          {submitting
            ? "Memproses..."
            : actionType === "review"
              ? "Setujui & Lanjutkan"
              : "Berikan Persetujuan"}
        </Button>
      </div>
    </div>
  );
}
