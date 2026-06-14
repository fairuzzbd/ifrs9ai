"use client";

import * as React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { AlertTriangle, Loader2 } from "lucide-react";

interface WithdrawDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kodeTransaksi: string;
  onConfirm: () => Promise<void>;
}

export function WithdrawDialog({ open, onOpenChange, kodeTransaksi, onConfirm }: WithdrawDialogProps) {
  const [submitting, setSubmitting] = React.useState(false);

  const handleConfirm = async () => {
    setSubmitting(true);
    try {
      await onConfirm();
      onOpenChange(false);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm" role="alertdialog" aria-describedby="withdraw-desc">
        <DialogHeader>
          <DialogTitle>Batalkan Penempatan {kodeTransaksi}</DialogTitle>
          <DialogDescription id="withdraw-desc">
            Tindakan ini tidak dapat dibalik.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 rounded-md border border-red-200 bg-red-50 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-600" aria-hidden="true" />
          <p className="text-sm text-red-700">
            Penempatan ini akan dibatalkan dan tidak dapat disubmit kembali.
          </p>
        </div>

        <p className="text-sm text-gray-600">
          Apakah Anda yakin ingin membatalkan penempatan ini?
        </p>

        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            Kembali
          </Button>
          <Button variant="destructive" onClick={() => void handleConfirm()} disabled={submitting}>
            {submitting && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            Ya, Batalkan Sekarang
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
