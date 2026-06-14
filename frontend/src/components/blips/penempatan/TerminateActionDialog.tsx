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
  terminateReason: z
    .string()
    .min(MIN_CHARS, `Alasan terminasi minimal ${MIN_CHARS} karakter`)
    .max(2000),
  attestChecked: z.boolean().refine((v) => v, {
    message: "Anda harus mencentang pernyataan ini",
  }),
});

type FormValues = z.infer<typeof schema>;

interface TerminateActionDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kodeTransaksi: string;
  onConfirm: (reason: string) => Promise<void>;
}

export function TerminateActionDialog({
  open,
  onOpenChange,
  kodeTransaksi,
  onConfirm,
}: TerminateActionDialogProps) {
  const [submitting, setSubmitting] = React.useState(false);
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { terminateReason: "", attestChecked: false },
  });

  const reasonValue = form.watch("terminateReason");

  React.useEffect(() => { if (!open) form.reset(); }, [open, form]);

  const handleSubmit = async (values: FormValues) => {
    setSubmitting(true);
    try {
      await onConfirm(values.terminateReason);
      onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg" role="alertdialog" aria-describedby="terminate-desc">
        <DialogHeader>
          <DialogTitle>Ajukan Terminasi {kodeTransaksi}</DialogTitle>
          <DialogDescription id="terminate-desc">
            Terminasi lebih awal dari jatuh tempo memiliki dampak finansial material.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-600" aria-hidden="true" />
          <p className="text-sm text-red-700">
            Terminasi lebih awal dari jatuh tempo memiliki dampak finansial material: EIR
            catch-up, ECL derecognition, dan realized gain/loss (per DEC-P5-M1-005).
          </p>
        </div>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="space-y-4">
            <FormField
              control={form.control}
              name="terminateReason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    Alasan Terminasi <span aria-hidden="true">*</span>{" "}
                    <span className="text-muted-foreground font-normal">
                      (minimal {MIN_CHARS} karakter)
                    </span>
                  </FormLabel>
                  <FormControl>
                    <Textarea
                      {...field}
                      placeholder="Jelaskan alasan terminasi secara detail..."
                      rows={4}
                      aria-required="true"
                    />
                  </FormControl>
                  <p className="text-xs text-muted-foreground text-right">
                    {reasonValue.length}/{MIN_CHARS}
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
                    <Checkbox
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      id="terminate-attest"
                      aria-required="true"
                    />
                  </FormControl>
                  <Label htmlFor="terminate-attest" className="text-sm leading-snug cursor-pointer">
                    Saya menyatakan terminasi ini berdasarkan alasan yang valid dan dokumen
                    pendukung terlampir.
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
              <Button
                type="submit"
                variant="destructive"
                disabled={submitting}
              >
                {submitting && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                )}
                Ajukan Terminasi
              </Button>
            </div>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
