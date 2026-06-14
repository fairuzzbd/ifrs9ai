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
import { Loader2 } from "lucide-react";

const schema = z.object({
  comment: z.string().min(1, "Komentar wajib diisi").max(1000),
  attestChecked: z.boolean().refine((v) => v, { message: "Anda harus mencentang pernyataan ini" }),
});
type FormValues = z.infer<typeof schema>;

interface ReviewDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kodeTransaksi: string;
  sodBlocked: boolean;
  onConfirm: (comment: string) => Promise<void>;
}

export function ReviewDialog({ open, onOpenChange, kodeTransaksi, sodBlocked, onConfirm }: ReviewDialogProps) {
  const [submitting, setSubmitting] = React.useState(false);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { comment: "", attestChecked: false },
  });

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
      <DialogContent className="max-w-md" aria-describedby="review-desc">
        <DialogHeader>
          <DialogTitle>Review Penempatan {kodeTransaksi}</DialogTitle>
          <DialogDescription id="review-desc">
            Anda akan memberikan persetujuan review untuk penempatan ini.
          </DialogDescription>
        </DialogHeader>

        {sodBlocked && (
          <SodBlockBanner message="Anda tidak bisa menjadi reviewer untuk penempatan yang Anda buat sendiri (SoD — DEC-017)." />
        )}

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="comment"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Komentar Review <span aria-hidden="true">*</span></FormLabel>
                  <FormControl>
                    <Textarea {...field} placeholder="Berikan catatan review..." rows={3} aria-required="true" disabled={sodBlocked} />
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
                    <Checkbox checked={field.value} onCheckedChange={field.onChange} id="review-attest" disabled={sodBlocked} />
                  </FormControl>
                  <Label htmlFor="review-attest" className="text-sm leading-snug cursor-pointer">
                    Saya menyatakan telah memeriksa kelengkapan dan kebenaran data penempatan ini.
                  </Label>
                </FormItem>
              )}
            />
            <div className="flex justify-end gap-2 pt-2">
              <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>Batal</Button>
              <Button type="submit" disabled={submitting || sodBlocked}>
                {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
                Setujui Review
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
