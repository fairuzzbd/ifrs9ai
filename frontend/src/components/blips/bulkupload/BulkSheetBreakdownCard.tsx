"use client";

import * as React from "react";
import { FileSpreadsheet } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

const SHEET_ORDER = ["Deposito", "Obligasi", "Saham", "Reksadana", "Tabungan_Cash"];

const SHEET_LABELS: Record<string, string> = {
  Deposito: "Deposito",
  Obligasi: "Obligasi",
  Saham: "Saham / Ekuitas",
  Reksadana: "Reksa Dana",
  Tabungan_Cash: "Tabungan / Cash",
};

export interface BulkSheetBreakdownCardProps {
  sheets: Record<string, number>;
  totalRows: number;
  className?: string;
}

export function BulkSheetBreakdownCard({ sheets, totalRows, className }: BulkSheetBreakdownCardProps) {
  const orderedSheets = SHEET_ORDER.filter((s) => sheets[s] !== undefined);
  const otherSheets = Object.keys(sheets).filter((s) => !SHEET_ORDER.includes(s));
  const allSheets = [...orderedSheets, ...otherSheets];

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm">
          <FileSpreadsheet className="h-4 w-4" aria-hidden="true" />
          Ringkasan per Sheet
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          {allSheets.map((sheet) => {
            const count = sheets[sheet] ?? 0;
            const pct = totalRows > 0 ? Math.round((count / totalRows) * 100) : 0;
            return (
              <div key={sheet} className="space-y-1">
                <div className="flex items-center justify-between text-xs">
                  <span className="font-medium">{SHEET_LABELS[sheet] ?? sheet}</span>
                  <span className="text-muted-foreground">
                    {count.toLocaleString("id-ID")} baris ({pct}%)
                  </span>
                </div>
                <div
                  role="progressbar"
                  aria-valuenow={pct}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-label={`${sheet}: ${pct}%`}
                  className="h-1.5 bg-muted rounded-full overflow-hidden"
                >
                  <div
                    className="h-full bg-primary rounded-full transition-all"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            );
          })}
          <div className="pt-2 border-t flex items-center justify-between text-xs font-medium">
            <span>Total</span>
            <span>{totalRows.toLocaleString("id-ID")} baris</span>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
