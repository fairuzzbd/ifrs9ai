"use client";

import * as React from "react";
import { Info } from "lucide-react";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { RenewalEIRBadge } from "./RenewalEIRBadge";
import { RenewalAmortizationTable } from "./RenewalAmortizationTable";
import type { RenewalPreview } from "@/lib/schemas/renewal.schema";

// ---------------------------------------------------------------------------
// IDR formatter — full precision for preview panel (DEC-016: NUMERIC(20,4))
// ---------------------------------------------------------------------------

const IDR_FULL = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 4,
  maximumFractionDigits: 4,
});

function formatIDR(value: string | undefined | null): string {
  if (!value) return "—";
  const n = parseFloat(value);
  if (isNaN(n)) return "—";
  return IDR_FULL.format(n);
}

// ---------------------------------------------------------------------------
// Row helper
// ---------------------------------------------------------------------------

interface PreviewRowProps {
  label: string;
  value: React.ReactNode;
  hint?: string;
  emphasis?: boolean;
  className?: string;
}

function PreviewRow({ label, value, hint, emphasis, className }: PreviewRowProps) {
  return (
    <div
      className={cn(
        "flex items-center justify-between gap-4 py-2 border-b last:border-b-0",
        emphasis && "font-semibold bg-muted/30 px-2 -mx-2 rounded",
        className,
      )}
    >
      <dt className="flex items-center gap-1 text-sm text-muted-foreground">
        {label}
        {hint && (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="cursor-help" aria-label={`Info: ${hint}`}>
                <Info className="h-3.5 w-3.5 text-muted-foreground/60" aria-hidden="true" />
              </span>
            </TooltipTrigger>
            <TooltipContent>
              <p className="max-w-xs text-xs">{hint}</p>
            </TooltipContent>
          </Tooltip>
        )}
      </dt>
      <dd className="text-sm font-mono text-right">{value}</dd>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface RenewalPreviewPanelProps {
  preview: RenewalPreview;
  tanggalJatuhTempoBaru?: string;
  className?: string;
  /** If true, shows the amortization schedule table */
  showSchedule?: boolean;
}

// ---------------------------------------------------------------------------
// Component — read-only preview of server-computed kalkulasi (S1, S2, S4)
// ---------------------------------------------------------------------------

export function RenewalPreviewPanel({
  preview,
  tanggalJatuhTempoBaru,
  className,
  showSchedule = true,
}: RenewalPreviewPanelProps) {
  const jatuhTempo = tanggalJatuhTempoBaru ?? preview.tanggalJatuhTempoBaru;

  return (
    <div className={cn("rounded-lg border bg-card p-4 space-y-1", className)}>
      <h3 className="text-sm font-semibold mb-3">Preview Kalkulasi</h3>
      <dl className="space-y-0">
        <PreviewRow
          label="Pokok Lama"
          value={formatIDR(preview.pokokLama)}
          hint="Nominal pokok instrumen deposito lama."
        />
        <PreviewRow
          label="Bunga Kotor"
          value={formatIDR(preview.bungaKotor)}
          hint="pokok_lama × (rate_lama / 100) × (hari_berjalan / 365)"
        />
        <PreviewRow
          label="PPh 20% (PP No. 131/2000)"
          value={formatIDR(preview.pph20pct)}
          hint="bunga_kotor × 0.20 — pajak penghasilan final atas bunga deposito bank."
        />
        <PreviewRow
          label="Bunga Bersih"
          value={formatIDR(preview.bungaBersih)}
          hint="bunga_kotor − PPh_20pct"
          emphasis
        />
        <PreviewRow
          label="Pokok Baru"
          value={formatIDR(preview.pokokBaru)}
          hint="POKOK_SAJA: = pokok_lama. POKOK_PLUS_BUNGA: = pokok_lama + bunga_bersih."
          emphasis
        />
        <PreviewRow
          label="Tanggal Jatuh Tempo Baru"
          value={jatuhTempo ?? "—"}
          hint="tanggal_efektif_baru + tenor_baru_bulan"
        />
        <div className="flex items-center justify-between gap-4 py-2">
          <dt className="text-sm text-muted-foreground flex items-center gap-1">
            EIR Baru
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="cursor-help">
                  <Info className="h-3.5 w-3.5 text-muted-foreground/60" aria-hidden="true" />
                </span>
              </TooltipTrigger>
              <TooltipContent>
                <p className="max-w-xs text-xs">
                  Newton-Raphson IRR dari cashflow instrumen baru after-tax.
                  DEC-013: tolerance 1e-10, max 100 iterasi, presisi 8 desimal.
                </p>
              </TooltipContent>
            </Tooltip>
          </dt>
          <dd>
            <RenewalEIRBadge eirBaru={preview.eirBaru} />
          </dd>
        </div>
      </dl>

      {showSchedule && preview.scheduleBaruPreview && preview.scheduleBaruPreview.length > 0 && (
        <div className="mt-4 pt-4 border-t">
          <h4 className="text-sm font-medium mb-2">Schedule Amortisasi Preview</h4>
          <RenewalAmortizationTable schedule={preview.scheduleBaruPreview} />
        </div>
      )}
    </div>
  );
}
