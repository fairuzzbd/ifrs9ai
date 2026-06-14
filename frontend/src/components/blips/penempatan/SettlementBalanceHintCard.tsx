"use client";

import * as React from "react";
import { AlertTriangle, Info } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { format, differenceInHours } from "date-fns";
import type { SettlementBalanceHint } from "@/lib/schemas/penempatan.schema";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatIdr(value: number): string {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(value);
}

function parseDecimalString(s: string | undefined): number | null {
  if (!s) return null;
  const n = parseFloat(s.replace(/\./g, "").replace(",", "."));
  return isNaN(n) ? null : n;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface SettlementBalanceHintCardProps {
  hint: SettlementBalanceHint | null;
  /** Raw string from form state (may contain thousands separators) */
  nominalIdrRaw?: string;
  settlementAccount?: string;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SettlementBalanceHintCard({
  hint,
  nominalIdrRaw,
  settlementAccount,
}: SettlementBalanceHintCardProps) {
  if (!settlementAccount) return null;

  if (!hint || hint.lastKnownIdr === null) {
    return (
      <Card className="border-gray-200">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm font-medium text-gray-600">
            Rekening Settlement
          </CardTitle>
        </CardHeader>
        <CardContent className="pb-3">
          <p className="font-mono text-sm text-gray-700">{settlementAccount}</p>
          <div className="mt-2 flex items-start gap-2 text-gray-500">
            <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p className="text-xs">
              Saldo tidak tersedia — pastikan saldo mencukupi sebelum submit.
            </p>
          </div>
        </CardContent>
      </Card>
    );
  }

  const nominalIdr = parseDecimalString(nominalIdrRaw);
  const lastKnown = hint.lastKnownIdr;

  // Check staleness (> 24h)
  const isStale =
    hint.isStale ||
    (hint.asOfDate
      ? differenceInHours(new Date(), new Date(hint.asOfDate)) > 24
      : false);

  const isInsufficient = nominalIdr !== null && nominalIdr > lastKnown;
  const isAmber = isInsufficient || isStale;

  const formattedDate = hint.asOfDate
    ? format(new Date(hint.asOfDate), "d MMM yyyy")
    : "-";

  return (
    <Card
      className={cn(
        "border",
        isAmber ? "border-amber-300 bg-amber-50" : "border-gray-200",
      )}
    >
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-gray-700">
          Rekening Settlement
        </CardTitle>
      </CardHeader>
      <CardContent className="pb-3 space-y-2">
        <p className="font-mono text-sm text-gray-800">{settlementAccount}</p>

        <div>
          <p className="text-xs text-gray-500">Saldo terakhir diketahui:</p>
          <p className="text-sm font-semibold text-gray-800">
            {formatIdr(lastKnown)}
          </p>
          <p className="text-xs text-gray-500">per {formattedDate}</p>
        </div>

        {isStale && (
          <div
            className="flex items-start gap-1.5 text-amber-700"
            role="alert"
            aria-live="polite"
          >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p className="text-xs">
              Perhatian: Data saldo mungkin tidak terkini (terakhir diperbarui{" "}
              {formattedDate}). Konfirmasi saldo sebelum submit.
            </p>
          </div>
        )}

        {isInsufficient && !isStale && (
          <div
            className="flex items-start gap-1.5 text-amber-700"
            role="alert"
            aria-live="polite"
          >
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p className="text-xs">
              Perhatian: Nominal ({nominalIdrRaw ? formatIdr(nominalIdr!) : "-"}) melebihi
              saldo terakhir yang diketahui ({formatIdr(lastKnown)} per {formattedDate}).
              Pastikan saldo tersedia sebelum submit.
            </p>
          </div>
        )}

        {!isAmber && (
          <div className="flex items-start gap-1.5 text-green-700">
            <Info className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <p className="text-xs">Saldo terakhir mencukupi nominal penempatan.</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
