"use client";

import * as React from "react";
import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { rollForwardApi } from "@/lib/api/roll-forward.api";
import { notify } from "@/lib/notify";
import type { RollForwardReconcileStatus } from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RollForwardExportButtonProps {
  reportId: string;
  reconcileStatus: RollForwardReconcileStatus;
  disabled?: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RollForwardExportButton({
  reportId,
  reconcileStatus,
  disabled,
  className,
}: RollForwardExportButtonProps) {
  const [pendingFormat, setPendingFormat] = React.useState<"xlsx" | "csv" | null>(null);

  const handleExport = (format: "xlsx" | "csv") => {
    if (reconcileStatus === "MISMATCH") {
      setPendingFormat(format);
    } else {
      rollForwardApi.export(reportId, format, false);
      notify.info(
        `Export ${format.toUpperCase()} dimulai. Tindakan ini tercatat di audit log.`,
      );
    }
  };

  const handleConfirmMismatch = () => {
    if (pendingFormat) {
      rollForwardApi.export(reportId, pendingFormat, true);
      notify.warning(
        `Export ${pendingFormat.toUpperCase()} dengan MISMATCH. Dokumen ini bertanda "TIDAK UNTUK PUBLIKASI". Tindakan tercatat di audit log.`,
      );
      setPendingFormat(null);
    }
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            disabled={disabled}
            className={className}
            aria-label="Ekspor laporan roll-forward"
          >
            <Download className="h-4 w-4 mr-1.5" aria-hidden="true" />
            Ekspor
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => handleExport("xlsx")}>
            Ekspor XLSX (Disclosure PSAK 71)
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleExport("csv")}>
            Ekspor CSV
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {/* MISMATCH confirm dialog */}
      <AlertDialog
        open={!!pendingFormat}
        onOpenChange={(open) => {
          if (!open) setPendingFormat(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Ekspor dengan Status MISMATCH?</AlertDialogTitle>
            <AlertDialogDescription>
              Laporan roll-forward ini memiliki status rekonsiliasi MISMATCH. Ekspor
              formal disclosure ({pendingFormat?.toUpperCase()}) diblokir untuk
              laporan yang tidak reconcile.
              <br />
              <br />
              Jika Anda lanjutkan, dokumen akan bertanda{" "}
              <strong>&ldquo;TIDAK UNTUK PUBLIKASI&rdquo;</strong> di lembar Sign-Off. Hanya
              untuk keperluan analisis internal. Tindakan ini dicatat di audit log.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setPendingFormat(null)}>
              Batal
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={handleConfirmMismatch}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              Ekspor (Analisis Internal)
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
