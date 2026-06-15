"use client";

import * as React from "react";
import { Plus, X, GripVertical } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { MappingDetailRow, KlasifikasiPsak71 } from "@/lib/schemas/jurnal.schema";

const SUMBER_AMOUNT_OPTIONS = [
  { value: "nominal_idr", label: "Nominal IDR" },
  { value: "ecl_amount", label: "Jumlah ECL" },
  { value: "mtm_change", label: "Perubahan MTM" },
  { value: "accrued_interest", label: "Akrual Bunga" },
  { value: "net_carrying_idr", label: "Net Carrying IDR" },
  { value: "fx_gain_loss", label: "Gain/Loss FX" },
  { value: "premium_discount_amortization", label: "Amortisasi Premi/Diskonto" },
];

const KLASIFIKASI_OPTIONS: { value: KlasifikasiPsak71; label: string }[] = [
  { value: "AC", label: "AC" },
  { value: "FVOCI", label: "FVOCI" },
  { value: "FVTPL", label: "FVTPL" },
  { value: "FVOCI_ELECTION", label: "FVOCI Election" },
  { value: "POCI", label: "POCI" },
];

export interface CoaOption {
  id: string;
  kodeAkun: string;
  namaAkun: string;
}

export interface MappingDetailRowsBuilderProps {
  value: MappingDetailRow[];
  onChange: (rows: MappingDetailRow[]) => void;
  disabled?: boolean;
  klasifikasiBerlaku?: KlasifikasiPsak71[];
  onSearchCoa?: (q: string) => Promise<CoaOption[]>;
  errors?: Record<string, string>;
}

function newRow(urutan: number): MappingDetailRow {
  return {
    _clientKey: crypto.randomUUID(),
    urutan,
    dkIndicator: "DEBIT",
    kodeAkunId: "",
    sumberAmount: "nominal_idr",
    multiplier: "1.0000",
    klasifikasiFilter: null,
    catatan: "",
  };
}

export function MappingDetailRowsBuilder({
  value,
  onChange,
  disabled = false,
  klasifikasiBerlaku = [],
  onSearchCoa,
  errors = {},
}: MappingDetailRowsBuilderProps) {
  const [coaSearches, setCoaSearches] = React.useState<Record<string, string>>({});
  const [coaOptions, setCoaOptions] = React.useState<Record<string, CoaOption[]>>({});
  const [coaOpen, setCoaOpen] = React.useState<Record<string, boolean>>({});
  const dragIdx = React.useRef<number | null>(null);

  const addRow = () => {
    onChange([...value, newRow(value.length + 1)]);
  };

  const removeRow = (idx: number) => {
    const next = value.filter((_, i) => i !== idx).map((r, i) => ({ ...r, urutan: i + 1 }));
    onChange(next);
  };

  const updateRow = (idx: number, patch: Partial<MappingDetailRow>) => {
    onChange(value.map((r, i) => (i === idx ? { ...r, ...patch } : r)));
  };

  const handleCoaSearch = async (key: string, q: string) => {
    setCoaSearches((s) => ({ ...s, [key]: q }));
    if (!onSearchCoa || q.length < 1) return;
    const opts = await onSearchCoa(q);
    setCoaOptions((s) => ({ ...s, [key]: opts }));
  };

  const handleDragStart = (idx: number) => {
    dragIdx.current = idx;
  };

  const handleDrop = (idx: number) => {
    if (dragIdx.current === null || dragIdx.current === idx) return;
    const reordered = [...value];
    const [moved] = reordered.splice(dragIdx.current, 1);
    reordered.splice(idx, 0, moved);
    onChange(reordered.map((r, i) => ({ ...r, urutan: i + 1 })));
    dragIdx.current = null;
  };

  if (value.length === 0) {
    return (
      <div className="space-y-3">
        <div className="rounded-md border border-dashed border-gray-300 p-6 text-center">
          <p className="text-sm text-muted-foreground">
            Belum ada baris. Tambahkan minimal 1 baris DEBIT dan 1 baris KREDIT.
          </p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={addRow} disabled={disabled}>
          <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
          Tambah Baris
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {/* Table header */}
      <div
        className="grid grid-cols-[24px_60px_1fr_130px_80px_100px_36px] gap-2 text-xs font-medium text-muted-foreground px-2"
        aria-hidden="true"
      >
        <span />
        <span>No</span>
        <span>Kode Akun</span>
        <span>Sumber Amount</span>
        <span>Multiplier</span>
        <span>Klasifikasi</span>
        <span />
      </div>

      {/* Rows */}
      {value.map((row, idx) => {
        const key = row._clientKey ?? String(idx);
        const coaOpts = coaOptions[key] ?? [];
        const coaQ = coaSearches[key] ?? row.kodeAkunDisplay ?? "";
        const isCoaOpen = coaOpen[key] ?? false;

        return (
          <div
            key={key}
            className="grid grid-cols-[24px_60px_1fr_130px_80px_100px_36px] gap-2 items-center rounded border border-transparent hover:border-gray-200 px-2 py-1.5"
            draggable={!disabled}
            onDragStart={() => handleDragStart(idx)}
            onDragOver={(e) => e.preventDefault()}
            onDrop={() => handleDrop(idx)}
            aria-label={`Baris ${idx + 1}`}
          >
            {/* Drag handle */}
            <button
              type="button"
              aria-label="Geser untuk reorder"
              tabIndex={-1}
              className="cursor-grab text-gray-300 hover:text-gray-500 focus:outline-none"
              disabled={disabled}
            >
              <GripVertical className="h-4 w-4" aria-hidden="true" />
            </button>

            {/* D/K indicator */}
            <Select
              value={row.dkIndicator}
              onValueChange={(v) => updateRow(idx, { dkIndicator: v as "DEBIT" | "KREDIT" })}
              disabled={disabled}
            >
              <SelectTrigger
                className={cn(
                  "h-8 text-xs font-bold",
                  row.dkIndicator === "DEBIT"
                    ? "border-blue-300 text-blue-700 bg-blue-50"
                    : "border-green-300 text-green-700 bg-green-50",
                )}
                aria-label={`Posisi baris ${idx + 1}`}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="DEBIT">D — Debit</SelectItem>
                <SelectItem value="KREDIT">K — Kredit</SelectItem>
              </SelectContent>
            </Select>

            {/* Kode Akun picker */}
            <div className="relative">
              <Input
                value={coaQ}
                onChange={(e) => {
                  updateRow(idx, { kodeAkunId: "", kodeAkunDisplay: e.target.value });
                  void handleCoaSearch(key, e.target.value);
                  setCoaOpen((s) => ({ ...s, [key]: true }));
                }}
                onFocus={() => setCoaOpen((s) => ({ ...s, [key]: true }))}
                onBlur={() => setTimeout(() => setCoaOpen((s) => ({ ...s, [key]: false })), 200)}
                placeholder="Cari akun..."
                className="h-8 text-xs"
                disabled={disabled}
                aria-label={`Kode akun baris ${idx + 1}`}
                aria-autocomplete="list"
                aria-expanded={isCoaOpen && coaOpts.length > 0}
              />
              {isCoaOpen && coaOpts.length > 0 && (
                <div className="absolute z-50 top-full left-0 right-0 mt-0.5 rounded-md border bg-white shadow-lg max-h-40 overflow-y-auto">
                  {coaOpts.map((opt) => (
                    <button
                      key={opt.id}
                      type="button"
                      className="w-full px-3 py-1.5 text-left text-xs hover:bg-accent"
                      onMouseDown={() => {
                        updateRow(idx, {
                          kodeAkunId: opt.id,
                          kodeAkunDisplay: `${opt.kodeAkun} — ${opt.namaAkun}`,
                          namaAkun: opt.namaAkun,
                        });
                        setCoaOpen((s) => ({ ...s, [key]: false }));
                      }}
                    >
                      <span className="font-mono font-medium">{opt.kodeAkun}</span>
                      <span className="ml-2 text-muted-foreground">{opt.namaAkun}</span>
                    </button>
                  ))}
                </div>
              )}
              {errors[`detailRows.${idx}.kodeAkunId`] && (
                <p className="text-xs text-destructive mt-0.5">
                  {errors[`detailRows.${idx}.kodeAkunId`]}
                </p>
              )}
            </div>

            {/* Sumber Amount */}
            <Select
              value={row.sumberAmount}
              onValueChange={(v) => updateRow(idx, { sumberAmount: v as MappingDetailRow["sumberAmount"] })}
              disabled={disabled}
            >
              <SelectTrigger className="h-8 text-xs" aria-label={`Sumber amount baris ${idx + 1}`}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SUMBER_AMOUNT_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value} className="text-xs">
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Multiplier */}
            <Input
              type="text"
              inputMode="decimal"
              value={row.multiplier}
              onChange={(e) => updateRow(idx, { multiplier: e.target.value })}
              className="h-8 text-xs font-mono"
              placeholder="1.0000"
              disabled={disabled}
              aria-label={`Multiplier baris ${idx + 1}`}
            />

            {/* Klasifikasi override */}
            <Select
              value={row.klasifikasiFilter ?? "_all"}
              onValueChange={(v) =>
                updateRow(idx, {
                  klasifikasiFilter: v === "_all" ? null : (v as KlasifikasiPsak71),
                })
              }
              disabled={disabled}
            >
              <SelectTrigger className="h-8 text-xs" aria-label={`Override klasifikasi baris ${idx + 1}`}>
                <SelectValue placeholder="Semua" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_all" className="text-xs text-muted-foreground">
                  Semua
                </SelectItem>
                {(klasifikasiBerlaku.length > 0 ? klasifikasiBerlaku : KLASIFIKASI_OPTIONS.map((o) => o.value)).map(
                  (k) => (
                    <SelectItem key={k} value={k} className="text-xs">
                      {k}
                    </SelectItem>
                  ),
                )}
              </SelectContent>
            </Select>

            {/* Remove */}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8 text-muted-foreground hover:text-destructive"
              onClick={() => removeRow(idx)}
              disabled={disabled || value.length <= 2}
              aria-label={`Hapus baris ${idx + 1}`}
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        );
      })}

      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={addRow}
        disabled={disabled}
        className="mt-2"
      >
        <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
        Tambah Baris
      </Button>

      {errors["detailRows"] && (
        <p role="alert" className="text-sm text-destructive mt-1">
          {errors["detailRows"]}
        </p>
      )}
    </div>
  );
}
