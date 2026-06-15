"use client";

import * as React from "react";
import { CheckCircle, AlertTriangle } from "lucide-react";
import { cn } from "@/lib/utils";
import type { JurnalLine } from "@/lib/schemas/jurnal.schema";

const IDR = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

export interface JurnalLinesTableProps {
  lines: JurnalLine[];
  showSubtotal?: boolean;
  showBalanceBadge?: boolean;
  className?: string;
}

export function JurnalLinesTable({
  lines,
  showSubtotal = true,
  showBalanceBadge = true,
  className,
}: JurnalLinesTableProps) {
  const totalDebit = lines
    .filter((l) => l.posisi === "DEBIT")
    .reduce((acc, l) => acc + parseFloat(l.amountIdr || "0"), 0);

  const totalKredit = lines
    .filter((l) => l.posisi === "KREDIT")
    .reduce((acc, l) => acc + parseFloat(l.amountIdr || "0"), 0);

  const isBalanced = Math.abs(totalDebit - totalKredit) < 0.01;

  if (lines.length === 0) {
    return (
      <div className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
        Belum ada baris jurnal
      </div>
    );
  }

  return (
    <div className={cn("overflow-hidden rounded-md border", className)}>
      <table className="w-full text-sm" role="table" aria-label="Baris jurnal debit/kredit">
        <thead className="bg-muted/50">
          <tr>
            <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground w-8">No</th>
            <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground w-20">Posisi</th>
            <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground w-28">Kode Akun</th>
            <th className="px-3 py-2 text-left text-xs font-medium text-muted-foreground">Nama Akun</th>
            <th className="px-3 py-2 text-right text-xs font-medium text-muted-foreground">Debit (IDR)</th>
            <th className="px-3 py-2 text-right text-xs font-medium text-muted-foreground">Kredit (IDR)</th>
          </tr>
        </thead>
        <tbody className="divide-y">
          {lines.map((line, idx) => (
            <tr key={idx} className="hover:bg-muted/20">
              <td className="px-3 py-2 text-xs text-muted-foreground">{line.urutan}</td>
              <td className="px-3 py-2">
                <span
                  className={cn(
                    "inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium",
                    line.posisi === "DEBIT"
                      ? "bg-blue-100 text-blue-700"
                      : "bg-green-100 text-green-700",
                  )}
                >
                  {line.posisi}
                </span>
              </td>
              <td className="px-3 py-2 font-mono text-xs">{line.akunKode}</td>
              <td className="px-3 py-2 text-xs">{line.akunNama}</td>
              <td className="px-3 py-2 text-right font-mono text-xs">
                {line.posisi === "DEBIT" ? IDR.format(parseFloat(line.amountIdr || "0")) : ""}
              </td>
              <td className="px-3 py-2 text-right font-mono text-xs">
                {line.posisi === "KREDIT" ? IDR.format(parseFloat(line.amountIdr || "0")) : ""}
              </td>
            </tr>
          ))}
        </tbody>

        {showSubtotal && (
          <tfoot className="border-t bg-muted/30">
            <tr>
              <td colSpan={4} className="px-3 py-2 text-xs font-semibold text-right">
                SUBTOTAL
              </td>
              <td className="px-3 py-2 text-right font-mono text-xs font-semibold">
                {IDR.format(totalDebit)}
              </td>
              <td className="px-3 py-2 text-right font-mono text-xs font-semibold">
                {IDR.format(totalKredit)}
              </td>
            </tr>
          </tfoot>
        )}
      </table>

      {showBalanceBadge && (
        <div
          className={cn(
            "flex items-center gap-2 px-3 py-2 text-xs font-medium border-t",
            isBalanced
              ? "text-green-700 bg-green-50"
              : "text-red-700 bg-red-50",
          )}
          role="status"
          aria-live="polite"
        >
          {isBalanced ? (
            <>
              <CheckCircle className="h-3.5 w-3.5" aria-hidden="true" />
              <span>SUDAH SEIMBANG</span>
            </>
          ) : (
            <>
              <AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />
              <span>
                TIDAK SEIMBANG — DEBIT {IDR.format(totalDebit)} ≠ KREDIT {IDR.format(totalKredit)}
              </span>
            </>
          )}
        </div>
      )}
    </div>
  );
}
