"use client";

import * as React from "react";
import { CheckCircle, AlertTriangle, HelpCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import type { MappingDetailRow, JurnalLine } from "@/lib/schemas/jurnal.schema";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

export interface BalancePreviewCardProps {
  rows: MappingDetailRow[];
  amountIdr?: string | null; // if provided, compute actual amounts
  resolverLines?: JurnalLine[]; // from resolver response
  className?: string;
}

export function BalancePreviewCard({
  rows,
  amountIdr,
  resolverLines,
  className,
}: BalancePreviewCardProps) {
  const debitRows = rows.filter((r) => r.dkIndicator === "DEBIT");
  const kreditRows = rows.filter((r) => r.dkIndicator === "KREDIT");
  const hasDebit = debitRows.length > 0;
  const hasKredit = kreditRows.length > 0;

  // If resolver returned lines, use actual computed amounts
  if (resolverLines && resolverLines.length > 0) {
    const totalDebit = resolverLines
      .filter((l) => l.posisi === "DEBIT")
      .reduce((acc, l) => acc + parseFloat(l.amountIdr || "0"), 0);
    const totalKredit = resolverLines
      .filter((l) => l.posisi === "KREDIT")
      .reduce((acc, l) => acc + parseFloat(l.amountIdr || "0"), 0);
    const isBalanced = Math.abs(totalDebit - totalKredit) < 0.01;

    return (
      <div className={cn("rounded-md border p-4 space-y-2", isBalanced ? "border-green-200 bg-green-50" : "border-red-200 bg-red-50", className)}>
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Total DEBIT</span>
          <span className="font-mono font-semibold">{IDR.format(totalDebit)}</span>
        </div>
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Total KREDIT</span>
          <span className="font-mono font-semibold">{IDR.format(totalKredit)}</span>
        </div>
        <div className="border-t pt-2">
          {isBalanced ? (
            <div className="flex items-center gap-2 text-green-700 text-sm font-medium">
              <CheckCircle className="h-4 w-4" aria-hidden="true" />
              <span>SUDAH SEIMBANG</span>
            </div>
          ) : (
            <div className="flex items-center gap-2 text-red-700 text-sm font-medium">
              <AlertTriangle className="h-4 w-4" aria-hidden="true" />
              <span>BELUM SEIMBANG — Selisih {IDR.format(Math.abs(totalDebit - totalKredit))}</span>
            </div>
          )}
        </div>
      </div>
    );
  }

  // Template-level check (when no runtime amount known)
  if (!amountIdr) {
    const templateBalanced = hasDebit && hasKredit;

    return (
      <div
        className={cn(
          "rounded-md border p-4 space-y-2",
          templateBalanced ? "border-green-200 bg-green-50" : "border-amber-200 bg-amber-50",
          className,
        )}
        role="status"
        aria-live="polite"
        aria-label="Status balance template"
      >
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Baris DEBIT</span>
          <span className={cn("font-semibold", hasDebit ? "text-green-700" : "text-red-600")}>
            {debitRows.length} baris
          </span>
        </div>
        <div className="flex items-center justify-between text-sm">
          <span className="text-muted-foreground">Baris KREDIT</span>
          <span className={cn("font-semibold", hasKredit ? "text-green-700" : "text-red-600")}>
            {kreditRows.length} baris
          </span>
        </div>
        <div className="border-t pt-2">
          {templateBalanced ? (
            <div className="flex items-center gap-2 text-green-700 text-sm font-medium">
              <CheckCircle className="h-4 w-4" aria-hidden="true" />
              <span>SUDAH SEIMBANG — Nilai akan divalidasi saat posting</span>
            </div>
          ) : (
            <div className="flex items-center gap-2 text-amber-700 text-sm font-medium">
              <AlertTriangle className="h-4 w-4" aria-hidden="true" />
              <span>
                BELUM SEIMBANG — Jumlah baris DEBIT ({debitRows.length}) ≠ KREDIT (
                {kreditRows.length}). Pastikan setiap sumber amount DEBIT memiliki pasangan KREDIT.
              </span>
            </div>
          )}
        </div>
        <div className="flex items-center gap-2 text-xs text-muted-foreground pt-1">
          <HelpCircle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          <span>
            Nilai aktual (IDR) akan dihitung saat resolver dijalankan dengan nominal posting.
          </span>
        </div>
      </div>
    );
  }

  // Amount known — compute from multipliers
  const amount = parseFloat(amountIdr || "0");
  const totalDebit = debitRows.reduce(
    (acc, r) => acc + amount * parseFloat(r.multiplier || "1"),
    0,
  );
  const totalKredit = kreditRows.reduce(
    (acc, r) => acc + amount * parseFloat(r.multiplier || "1"),
    0,
  );
  const isBalanced = Math.abs(totalDebit - totalKredit) < 0.01;

  return (
    <div
      className={cn(
        "rounded-md border p-4 space-y-2",
        isBalanced ? "border-green-200 bg-green-50" : "border-amber-200 bg-amber-50",
        className,
      )}
      role="status"
      aria-live="polite"
    >
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">Total DEBIT</span>
        <span className="font-mono font-semibold">{IDR.format(totalDebit)}</span>
      </div>
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">Total KREDIT</span>
        <span className="font-mono font-semibold">{IDR.format(totalKredit)}</span>
      </div>
      <div className="border-t pt-2">
        {isBalanced ? (
          <div className="flex items-center gap-2 text-green-700 text-sm font-medium">
            <CheckCircle className="h-4 w-4" aria-hidden="true" />
            <span>SUDAH SEIMBANG</span>
          </div>
        ) : (
          <div className="flex items-center gap-2 text-amber-700 text-sm font-medium">
            <AlertTriangle className="h-4 w-4" aria-hidden="true" />
            <span>BELUM SEIMBANG — Jumlah DEBIT ≠ KREDIT</span>
          </div>
        )}
      </div>
    </div>
  );
}
