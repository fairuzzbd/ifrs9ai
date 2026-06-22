/**
 * MappingReviewDialog — PENDING_REVIEW → PENDING_APPROVAL / PENDING_APPROVAL_2.
 * Actor: ROLE-AKUN-CTL. SoD: reviewer ≠ maker. Comment ≥ 30 chars.
 */

"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { ClipboardCheck } from "lucide-react";
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
import { reviewDialogSchema, type ReviewDialogInput } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingReviewDialogProps {
  eventCode: string;
  isRegulated: boolean;
  disabled?: boolean;
  onReview: (comment: string) => Promise<void>;
  children?: React.ReactNode;
}

export function MappingReviewDialog({
  eventCode,
  isRegulated,
  disabled,
  onReview,
  children,
}: MappingReviewDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [attested, setAttested] = React.useState(false);

  const form = useForm<ReviewDialogInput>({
    resolver: zodResolver(reviewDialogSchema),
    defaultValues: { comment: "", signatureMethod: "JWT_STEP_UP" },
  });

  const watchComment = form.watch("comment");

  const handleSubmit = async (values: ReviewDialogInput) => {
    setSubmitting(true);
    try {
      await onReview(values.comment);
      setOpen(false);
      form.reset();
      setAttested(false);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {children ?? (
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            aria-label={`Review mapping ${eventCode}`}
          >
            <ClipboardCheck className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Review
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Review Mapping Jurnal</DialogTitle>
          <DialogDescription>
            Review mapping <strong>{eventCode}</strong>.
            {isRegulated
              ? " Event ini regulated — setelah review akan dilanjutkan ke ROLE-RISK (6-eyes)."
              : " Setelah review akan dilanjutkan ke approver (4-eyes)."}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Komentar Review <span className="text-destructive">*</span>
                    <span className="ml-1 text-xs text-muted-foreground">(min 30 karakter)</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={4}
                      placeholder="Verifikasi akun debit/kredit sudah sesuai COA..."
                      disabled={submitting}
                      aria-describedby="review-comment-counter"
                    />
                  </FormControl>
                  <div
                    id="review-comment-counter"
                    className={`text-right text-xs ${
                      watchComment.length < 30 ? "text-destructive" : "text-muted-foreground"
                    }`}
                  >
                    {watchComment.length} / 30 karakter minimum
                  </div>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex items-start gap-3 rounded-md border p-3">
              <Checkbox
                id="review-attest"
                checked={attested}
                onCheckedChange={(v) => setAttested(v === true)}
                disabled={submitting}
                aria-describedby="review-attest-label"
              />
              <Label
                id="review-attest-label"
                htmlFor="review-attest"
                className="cursor-pointer text-xs leading-relaxed"
              >
                Saya menyatakan telah memeriksa akun debit/kredit dan formula mapping sesuai COA yang berlaku.
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
                {submitting ? "Memproses..." : "Review & Lanjutkan"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
