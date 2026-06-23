/**
 * MappingDetailTable — read-only table of akun_debit/kredit rows per event.
 * Shows: urutan, akun_debit, akun_kredit, D/K indicator, jumlah_calc, balance check.
 * Balance check: total D rows = total K rows (count-based, not multiplier).
 */

import * as React from "react";
import { CheckCircle2, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import type { MappingP12DetailRow } from "@/lib/schemas/mapping-jurnal-p12.schema";

interface MappingDetailTableProps {
  rows: MappingP12DetailRow[];
  className?: string;
}

export function MappingDetailTable({ rows, className }: MappingDetailTableProps) {
  const sorted = [...rows].sort((a, b) => a.urutan - b.urutan);

  const debitCount = sorted.filter((r) => r.debitKredit === "D").length;
  const kreditCount = sorted.filter((r) => r.debitKredit === "K").length;
  const isBalanced = debitCount > 0 && kreditCount > 0 && debitCount === kreditCount;
  const hasNullAkun = sorted.some((r) => !r.akunDebit || !r.akunKredit);

  if (rows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground italic">
        Tidak ada detail jurnal. Tambahkan baris akun debit/kredit.
      </p>
    );
  }

  return (
    <div className={cn("space-y-3", className)}>
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm" aria-label="Detail baris mapping jurnal">
          <thead className="border-b bg-muted/50">
            <tr>
              <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-muted-foreground w-12">No.</th>
              <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-muted-foreground">Akun Debit</th>
              <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-muted-foreground">Akun Kredit</th>
              <th scope="col" className="px-3 py-2 text-center text-xs font-medium text-muted-foreground w-16">D/K</th>
              <th scope="col" className="px-3 py-2 text-left text-xs font-medium text-muted-foreground">Formula</th>
            </tr>
          </thead>
          <tbody>
            {sorted.map((row) => (
              <tr key={row.id} className="border-b last:border-0 hover:bg-muted/30">
                <td className="px-3 py-2 text-muted-foreground text-xs">{row.urutan}</td>
                <td className="px-3 py-2">
                  {row.akunDebit ? (
                    <code className="font-mono text-xs">{row.akunDebit}</code>
                  ) : (
                    <span className="text-xs text-destructive italic">kosong</span>
                  )}
                </td>
                <td className="px-3 py-2">
                  {row.akunKredit ? (
                    <code className="font-mono text-xs">{row.akunKredit}</code>
                  ) : (
                    <span className="text-xs text-destructive italic">kosong</span>
                  )}
                </td>
                <td className="px-3 py-2 text-center">
                  <Badge
                    variant="outline"
                    className={cn(
                      "text-xs font-mono",
                      row.debitKredit === "D"
                        ? "border-blue-300 bg-blue-50 text-blue-700"
                        : "border-orange-300 bg-orange-50 text-orange-700",
                    )}
                  >
                    {row.debitKredit}
                  </Badge>
                </td>
                <td className="px-3 py-2 text-xs text-muted-foreground">
                  {row.jumlahCalc ?? <span className="italic">—</span>}
                </td>
              </tr>
            ))}
          </tbody>
          <tfoot className="border-t bg-muted/30">
            <tr>
              <td colSpan={3} className="px-3 py-2 text-xs text-muted-foreground text-right font-medium">
                Total:
              </td>
              <td className="px-3 py-2 text-center">
                <div className="text-xs space-y-0.5">
                  <div className="text-blue-700">D: {debitCount}</div>
                  <div className="text-orange-700">K: {kreditCount}</div>
                </div>
              </td>
              <td className="px-3 py-2">
                <div className="flex items-center gap-1">
                  {isBalanced ? (
                    <>
                      <CheckCircle2 className="h-3.5 w-3.5 text-green-600" aria-hidden="true" />
                      <span className="text-xs font-medium text-green-700">Seimbang</span>
                    </>
                  ) : (
                    <>
                      <XCircle className="h-3.5 w-3.5 text-destructive" aria-hidden="true" />
                      <span className="text-xs font-medium text-destructive">Tidak Seimbang</span>
                    </>
                  )}
                  {hasNullAkun && (
                    <span className="ml-2 text-xs text-amber-600">(ada akun kosong)</span>
                  )}
                </div>
              </td>
            </tr>
          </tfoot>
        </table>
      </div>
    </div>
  );
}
