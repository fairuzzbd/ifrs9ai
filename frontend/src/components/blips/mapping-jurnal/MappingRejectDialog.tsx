/**
 * MappingRejectDialog — reject at any PENDING_* step → back to DRAFT.
 * Reason ≥ 30 chars required.
 */

"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { XCircle } from "lucide-react";
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
import { rejectDialogSchema, type RejectDialogInput } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingRejectDialogProps {
  eventCode: string;
  disabled?: boolean;
  onReject: (reason: string) => Promise<void>;
  children?: React.ReactNode;
}

export function MappingRejectDialog({
  eventCode,
  disabled,
  onReject,
  children,
}: MappingRejectDialogProps) {
  const [open, setOpen] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);

  const form = useForm<RejectDialogInput>({
    resolver: zodResolver(rejectDialogSchema),
    defaultValues: { reason: "" },
  });

  const watchReason = form.watch("reason");

  const handleSubmit = async (values: RejectDialogInput) => {
    setSubmitting(true);
    try {
      await onReject(values.reason);
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
          <Button
            size="sm"
            variant="outline"
            disabled={disabled}
            className="border-destructive text-destructive hover:bg-destructive/5"
            aria-label={`Tolak mapping ${eventCode}`}
          >
            <XCircle className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Tolak
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="text-destructive">Tolak Mapping Jurnal</DialogTitle>
          <DialogDescription>
            Mapping <strong>{eventCode}</strong> akan dikembalikan ke status DRAFT.
            Maker harus memperbaiki dan submit ulang.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="reason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Penolakan <span className="text-destructive">*</span>
                    <span className="ml-1 text-xs text-muted-foreground">(min 30 karakter)</span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      rows={4}
                      placeholder="Jelaskan alasan penolakan secara detail..."
                      disabled={submitting}
                      aria-describedby="reject-reason-counter"
                    />
                  </FormControl>
                  <div
                    id="reject-reason-counter"
                    className={`text-right text-xs ${
                      watchReason.length < 30 ? "text-destructive" : "text-muted-foreground"
                    }`}
                    aria-live="polite"
                  >
                    {watchReason.length} / 30 karakter minimum
                  </div>
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
              <Button
                type="submit"
                variant="destructive"
                disabled={submitting}
                aria-busy={submitting}
              >
                {submitting ? "Memproses..." : "Tolak & Kembalikan"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
