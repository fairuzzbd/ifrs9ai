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
import { AlertTriangle, Loader2 } from "lucide-react";

const MIN_CHARS = 30;

const schema = z.object({
  comment: z
    .string()
    .min(MIN_CHARS, `Alasan penolakan minimal ${MIN_CHARS} karakter`)
    .max(1000),
  attestChecked: z.boolean().refine((v) => v, { message: "Anda harus mencentang pernyataan ini" }),
});
type FormValues = z.infer<typeof schema>;

interface RejectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kodeTransaksi: string;
  onConfirm: (comment: string) => Promise<void>;
}

export function RejectDialog({ open, onOpenChange, kodeTransaksi, onConfirm }: RejectDialogProps) {
  const [submitting, setSubmitting] = React.useState(false);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { comment: "", attestChecked: false },
  });

  const commentValue = form.watch("comment");

  React.useEffect(() => { if (!open) form.reset(); }, [open, form]);

  const handleSubmit = async (values: FormValues) => {
    setSubmitting(true);
    try {
      await onConfirm(values.comment);
      onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" aria-describedby="reject-desc" role="alertdialog">
        <DialogHeader>
          <DialogTitle>Tolak Penempatan {kodeTransaksi}</DialogTitle>
          <DialogDescription id="reject-desc">
            Penempatan akan dikembalikan ke status Konsep (DRAFT). Maker dapat memperbaiki dan
            submit ulang.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" aria-hidden="true" />
          <p className="text-sm text-amber-700">
            Penempatan akan dikembalikan ke status Konsep. Maker dapat memperbaiki dan submit ulang.
          </p>
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Penolakan <span aria-hidden="true">*</span>{" "}
                    <span className="text-muted-foreground font-normal">
                      (minimal {MIN_CHARS} karakter)
                    </span>
                  </FormLabel>
                  <FormControl>
                    <Textarea {...field} rows={4} aria-required="true" />
                  </FormControl>
                  <p className="text-xs text-muted-foreground text-right">
                    {commentValue.length}/{MIN_CHARS}
                  </p>
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
                    <Checkbox checked={field.value} onCheckedChange={field.onChange} id="reject-attest" />
                  </FormControl>
                  <Label htmlFor="reject-attest" className="text-sm leading-snug cursor-pointer">
                    Saya menyatakan penolakan ini berdasarkan alasan yang tertulis di atas.
                  </Label>
                </FormItem>
              )}
            />
            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>Batal</Button>
              <Button type="submit" variant="destructive" disabled={submitting}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
                Tolak Penempatan
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
