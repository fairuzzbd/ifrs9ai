/**
 * MappingSubmitDialog — DRAFT → PENDING_REVIEW transition.
 * Actor: ROLE-AKUN (maker). Comment optional.
 */

"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Send } from "lucide-react";
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
import { submitDialogSchema, type SubmitDialogInput } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingSubmitDialogProps {
  eventCode: string;
  versionId: string;
  disabled?: boolean;
  onSubmit: (comment: string) => Promise<void>;
  children?: React.ReactNode;
}

export function MappingSubmitDialog({
  eventCode,
  versionId: _versionId,
  disabled,
  onSubmit,
  children,
}: MappingSubmitDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const form = useForm<SubmitDialogInput>({
    resolver: zodResolver(submitDialogSchema),
    defaultValues: { comment: "" },
  });

  const handleSubmit = async (values: SubmitDialogInput) => {
    setSubmitting(true);
    try {
      await onSubmit(values.comment);
      setOpen(false);
      form.reset();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {children ?? (
          <Button size="sm" disabled={disabled} aria-label={`Submit mapping ${eventCode} untuk review`}>
            <Send className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Submit untuk Review
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Submit Mapping untuk Review</DialogTitle>
          <DialogDescription>
            Mapping <strong>{eventCode}</strong> akan disubmit ke ROLE-AKUN-CTL untuk review.
            Pastikan semua baris akun sudah terisi.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Komentar (opsional)</FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={3}
                      placeholder="Catatan untuk reviewer..."
                      disabled={submitting}
                      aria-describedby="submit-comment-desc"
                    />
                  </FormControl>
                  <p id="submit-comment-desc" className="text-xs text-muted-foreground">
                    Komentar akan terlihat oleh reviewer.
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setOpen(false)}
                disabled={submitting}
              >
                Batal
              </Button>
              <Button type="submit" disabled={submitting} aria-busy={submitting}>
                {submitting ? "Memproses..." : "Submit"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
