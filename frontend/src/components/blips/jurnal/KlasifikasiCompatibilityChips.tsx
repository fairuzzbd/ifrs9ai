"use client";

import * as React from "react";
import { Lock, AlertTriangle } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger, TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import {
  KLASIFIKASI_COMPATIBILITY,
  type KlasifikasiPsak71,
} from "@/lib/schemas/jurnal.schema";

const ALL_KLASIFIKASI: KlasifikasiPsak71[] = [
  "AC",
  "FVOCI",
  "FVTPL",
  "FVOCI_ELECTION",
  "POCI",
];

const KLASIFIKASI_LABELS: Record<KlasifikasiPsak71, string> = {
  AC: "AC",
  FVOCI: "FVOCI",
  FVTPL: "FVTPL",
  FVOCI_ELECTION: "FVOCI Election",
  POCI: "POCI",
};

const DISABLED_REASON: Partial<Record<string, string>> = {
  FVTPL: "FVTPL tidak memiliki ECL (PSAK 71 §5.5.15)",
  FVOCI_ELECTION: "Saham FVOCI Election tidak dikenakan ECL",
  AC: "Klasifikasi ini tidak berlaku untuk event ini",
  FVOCI: "Klasifikasi ini tidak berlaku untuk event ini",
  POCI: "Klasifikasi ini tidak berlaku untuk event ini",
};

export interface KlasifikasiCompatibilityChipsProps {
  selectedEventCode: string | null;
  value: KlasifikasiPsak71[];
  onChange: (value: KlasifikasiPsak71[]) => void;
  allowNull?: boolean;
  disabled?: boolean;
}

export function KlasifikasiCompatibilityChips({
  selectedEventCode,
  value,
  onChange,
  allowNull = true,
  disabled = false,
}: KlasifikasiCompatibilityChipsProps) {
  const [applyToAll, setApplyToAll] = React.useState(value.length === 0 && allowNull);

  const allowedByEvent: KlasifikasiPsak71[] | null = selectedEventCode
    ? (KLASIFIKASI_COMPATIBILITY[selectedEventCode] ?? null)
    : null;

  const isDisabled = (k: KlasifikasiPsak71): boolean => {
    if (!allowedByEvent) return false;
    return !allowedByEvent.includes(k);
  };

  const toggle = (k: KlasifikasiPsak71) => {
    if (disabled || isDisabled(k)) return;
    if (applyToAll) {
      setApplyToAll(false);
      onChange([k]);
      return;
    }
    if (value.includes(k)) {
      onChange(value.filter((v) => v !== k));
    } else {
      onChange([...value, k]);
    }
  };

  const handleToggleAll = (checked: boolean) => {
    setApplyToAll(checked);
    if (checked) onChange([]);
  };

  return (
    <TooltipProvider>
      <div className="space-y-3">
        {allowNull && (
          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={applyToAll}
              onChange={(e) => handleToggleAll(e.target.checked)}
              disabled={disabled}
              className="h-4 w-4 rounded border-gray-300"
              aria-label="Berlaku untuk semua klasifikasi"
            />
            <span className="text-sm text-muted-foreground">
              Berlaku untuk SEMUA klasifikasi (NULL)
            </span>
          </label>
        )}

        <div className="flex flex-wrap gap-2" role="group" aria-label="Pilih klasifikasi PSAK 71">
          {ALL_KLASIFIKASI.map((k) => {
            const dis = isDisabled(k) || disabled || applyToAll;
            const selected = !applyToAll && value.includes(k);
            const reason = isDisabled(k)
              ? DISABLED_REASON[k] ?? "Tidak kompatibel dengan event ini"
              : undefined;

            const chip = (
              <button
                key={k}
                type="button"
                role="checkbox"
                aria-checked={selected}
                aria-disabled={dis}
                disabled={dis}
                onClick={() => toggle(k)}
                className={cn(
                  "inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-medium transition-all",
                  "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
                  selected && !applyToAll
                    ? "border-blue-600 bg-blue-600 text-white"
                    : "border-gray-300 bg-white text-gray-700",
                  dis && "opacity-40 cursor-not-allowed",
                )}
              >
                {isDisabled(k) && (
                  <Lock className="h-3 w-3" aria-hidden="true" />
                )}
                {KLASIFIKASI_LABELS[k]}
              </button>
            );

            if (reason) {
              return (
                <Tooltip key={k}>
                  <TooltipTrigger asChild>{chip}</TooltipTrigger>
                  <TooltipContent>
                    <p className="text-xs max-w-xs">{reason}</p>
                  </TooltipContent>
                </Tooltip>
              );
            }
            return chip;
          })}
        </div>

        {/* Inline warning chips for incompatible selections */}
        {value.some((v) => allowedByEvent && !allowedByEvent.includes(v)) && (
          <div className="flex items-start gap-2 text-xs text-amber-700 rounded border border-amber-200 bg-amber-50 px-3 py-2">
            <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" aria-hidden="true" />
            <span>Beberapa klasifikasi yang dipilih tidak kompatibel dengan event ini.</span>
          </div>
        )}
      </div>
    </TooltipProvider>
  );
}
