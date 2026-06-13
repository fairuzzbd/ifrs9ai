"use client";

import * as React from "react";
import { AlertTriangle } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import type { ScheduleVersion, EIRScheduleRow } from "@/lib/schemas/eir.schema";
import type { Pagination } from "@/lib/api";
import { DataTable } from "@/components/blips/DataTable";
import type { ColumnDef } from "@tanstack/react-table";

// ---------------------------------------------------------------------------
// Formatters (string-based per DEC-016)
// ---------------------------------------------------------------------------

function formatIDR4(value: string | undefined | null): string {
  if (!value) return "—";
  try {
    const [int, dec = "0000"] = value.split(".");
    const n = parseInt(int.replace(/[^0-9-]/g, ""), 10);
    if (isNaN(n)) return value;
    return new Intl.NumberFormat("id-ID").format(n) + "," + dec.padEnd(4, "0").slice(0, 4);
  } catch {
    return value;
  }
}

function formatEIR8(value: string | undefined | null): string {
  if (!value) return "—";
  // Show 8 decimal places
  try {
    const n = parseFloat(value);
    if (isNaN(n)) return value;
    return (n * 100).toFixed(8) + "%";
  } catch {
    return value;
  }
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AmortizationScheduleTableProps {
  instrumenId: string;
  versions: ScheduleVersion[];
  data: EIRScheduleRow[];
  pagination?: Pagination;
  isLoading?: boolean;
  isError?: boolean;
  selectedVersion: number | null;
  onVersionChange: (version: number | null) => void;
  onNextPage?: () => void;
  onPrevPage?: () => void;
  canPrevPage?: boolean;
  pageNumber?: number;
  onExport?: (format: "csv" | "xlsx") => void;
  onRefresh?: () => void;
}

// ---------------------------------------------------------------------------
// Columns
// ---------------------------------------------------------------------------

const columns: ColumnDef<EIRScheduleRow>[] = [
  {
    id: "periodeSeq",
    accessorKey: "periodeSeq",
    header: "Seq",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs">{row.original.periodeSeq}</span>
    ),
  },
  {
    id: "tanggalPosting",
    accessorKey: "tanggalPosting",
    header: "Tgl Posting",
    cell: ({ row }) => (
      <span className="text-xs">{row.original.tanggalPosting}</span>
    ),
  },
  {
    id: "openingCarrying",
    accessorKey: "openingCarryingIdr",
    header: "Opening Carrying",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs text-right block">
        {formatIDR4(row.original.openingCarryingIdr)}
      </span>
    ),
  },
  {
    id: "pendapatanBunga",
    accessorKey: "pendapatanBungaEirIdr",
    header: "Pend. Bunga EIR",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs text-right block">
        {formatIDR4(row.original.pendapatanBungaEirIdr)}
      </span>
    ),
  },
  {
    id: "amortisasi",
    accessorKey: "amortisasiPdIdr",
    header: "Amortisasi P/D",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs text-right block">
        {formatIDR4(row.original.amortisasiPdIdr)}
      </span>
    ),
  },
  {
    id: "pelunasanPokok",
    accessorKey: "pelunasanPokokIdr",
    header: "Pelunasan Pokok",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs text-right block">
        {formatIDR4(row.original.pelunasanPokokIdr)}
      </span>
    ),
  },
  {
    id: "closingCarrying",
    accessorKey: "closingCarryingIdr",
    header: "Closing Carrying",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs text-right block">
        {formatIDR4(row.original.closingCarryingIdr)}
      </span>
    ),
  },
  {
    id: "eirPeriode",
    accessorKey: "eirPeriode",
    header: "EIR Periode",
    meta: { align: "right" },
    cell: ({ row }) => (
      <span className="font-mono text-xs text-right block">
        {formatEIR8(row.original.eirPeriode)}
      </span>
    ),
  },
  {
    id: "statusPosting",
    accessorKey: "statusPosting",
    header: "Status",
    cell: ({ row }) => {
      const s = row.original.statusPosting;
      return (
        <span
          className={
            s === "POSTED"
              ? "text-green-700 text-xs font-medium"
              : "text-gray-500 text-xs"
          }
        >
          {s === "POSTED" ? "Diposting" : s === "PROYEKSI" ? "Proyeksi" : s}
        </span>
      );
    },
  },
];

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function AmortizationScheduleTable({
  versions,
  data,
  pagination,
  isLoading,
  isError,
  selectedVersion,
  onVersionChange,
  onNextPage,
  onPrevPage,
  canPrevPage,
  pageNumber,
  onExport,
  onRefresh,
}: AmortizationScheduleTableProps) {
  const activeVersion = versions.find((v) => v.isActive);
  const isViewingSuperseded =
    selectedVersion !== null &&
    selectedVersion !== (activeVersion?.scheduleVersion ?? null);

  const displayValue =
    selectedVersion !== null
      ? String(selectedVersion)
      : activeVersion
        ? String(activeVersion.scheduleVersion)
        : "";

  return (
    <div className="space-y-3">
      {/* Version selector */}
      {versions.length > 0 && (
        <div className="flex items-center gap-3">
          <label className="text-sm font-medium text-muted-foreground">
            Versi Schedule
          </label>
          <Select
            value={displayValue}
            onValueChange={(v) => onVersionChange(v ? parseInt(v, 10) : null)}
            aria-label="Pilih versi schedule amortisasi"
          >
            <SelectTrigger className="w-64">
              <SelectValue placeholder="Pilih versi..." />
            </SelectTrigger>
            <SelectContent>
              {versions.map((v) => (
                <SelectItem key={v.scheduleVersion} value={String(v.scheduleVersion)}>
                  {v.versionLabel}
                  {!v.isActive && (
                    <Badge variant="secondary" className="ml-2 text-xs">
                      Superseded
                    </Badge>
                  )}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {/* Superseded warning */}
      {isViewingSuperseded && (
        <Alert variant="default" className="border-amber-300 bg-amber-50">
          <AlertTriangle className="h-4 w-4 text-amber-600" aria-hidden="true" />
          <AlertDescription className="text-amber-800">
            Anda melihat versi schedule yang sudah digantikan (superseded). Versi
            aktif tersedia di dropdown di atas.
          </AlertDescription>
        </Alert>
      )}

      <DataTable
        columns={columns}
        data={data}
        pagination={pagination}
        isLoading={isLoading}
        isError={isError}
        onNextPage={onNextPage}
        onPrevPage={onPrevPage}
        canPrevPage={canPrevPage}
        pageNumber={pageNumber}
        onExport={onExport}
        onRefresh={onRefresh}
        emptyMessage="Belum ada jadwal amortisasi. Hitung EIR terlebih dahulu."
      />
    </div>
  );
}
