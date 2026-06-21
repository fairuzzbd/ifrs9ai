"use client";

import * as React from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { RenewalPreview } from "@/lib/schemas/renewal.schema";

// ---------------------------------------------------------------------------
// IDR formatter — grouped thousands, 4 decimal places (tables)
// ---------------------------------------------------------------------------

const IDR_TABLE = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
});

function fmt(value: string | undefined | null): string {
  if (!value) return "—";
  const n = parseFloat(value);
  if (isNaN(n)) return "—";
  return IDR_TABLE.format(n);
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

type ScheduleRow = NonNullable<RenewalPreview["scheduleBaruPreview"]>[number];

export interface RenewalAmortizationTableProps {
  schedule: ScheduleRow[];
  className?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function RenewalAmortizationTable({
  schedule,
  className,
}: RenewalAmortizationTableProps) {
  if (!schedule || schedule.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-2">
        Schedule amortisasi belum tersedia.
      </p>
    );
  }

  return (
    <div className={className}>
      <div className="overflow-x-auto rounded-md border">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/50">
              <TableHead className="text-right w-16">Bulan</TableHead>
              <TableHead className="text-right">Bunga Kotor</TableHead>
              <TableHead className="text-right">PPh 20%</TableHead>
              <TableHead className="text-right">Bunga Bersih</TableHead>
              <TableHead className="text-right">Saldo Akhir</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {schedule.map((row) => (
              <TableRow key={row.bulan}>
                <TableCell className="text-right font-mono text-sm">
                  {row.bulan}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {fmt(row.bungaKotorBulan)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm text-muted-foreground">
                  {fmt(row.pphBulan)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {fmt(row.bungaBersihBulan)}
                </TableCell>
                <TableCell className="text-right font-mono text-sm">
                  {fmt(row.saldoPokokAkhir)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
      <p className="text-xs text-muted-foreground mt-1">
        Nilai dalam IDR. PPh 20% per PP No. 131/2000. DEC-016: NUMERIC(20,4).
      </p>
    </div>
  );
}
