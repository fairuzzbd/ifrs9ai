"use client";

import * as React from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type SicrTriggerType =
  | "RATING_DOWNGRADE"
  | "IG_TO_NON_IG"
  | "DPD_THRESHOLD"
  | "CURE_FULL"
  | "CURE_PARTIAL"
  | "MANUAL_OVERRIDE"
  | "ORIGINATION"
  | "DPD_CORRECTION";

export interface SicrEvidenceCardProps {
  triggerType: SicrTriggerType;
  evidence?: {
    notchChange?: number | null;
    ratingLama?: string | null;
    ratingBaru?: string | null;
    dpd?: number | null;
    curiePeriode?: number | null;
  } | null;
  date?: string;
  compact?: boolean;
}

// ---------------------------------------------------------------------------
// Label map (Bahasa Indonesia)
// ---------------------------------------------------------------------------

const TRIGGER_LABEL_MAP: Record<SicrTriggerType, string> = {
  RATING_DOWNGRADE: "Penurunan Peringkat",
  IG_TO_NON_IG: "Perubahan ke Non-Investment Grade",
  DPD_THRESHOLD: "Melebihi Ambang Tunggakan",
  CURE_FULL: "Pemulihan Penuh",
  CURE_PARTIAL: "Pemulihan Sebagian",
  MANUAL_OVERRIDE: "Override Manual",
  ORIGINATION: "Originasi Instrumen",
  DPD_CORRECTION: "Koreksi Data DPD",
};

const TRIGGER_COLOR_MAP: Record<SicrTriggerType, string> = {
  RATING_DOWNGRADE: "bg-amber-100 text-amber-800",
  IG_TO_NON_IG: "bg-orange-100 text-orange-800",
  DPD_THRESHOLD: "bg-red-100 text-red-800",
  CURE_FULL: "bg-green-100 text-green-800",
  CURE_PARTIAL: "bg-teal-100 text-teal-800",
  MANUAL_OVERRIDE: "bg-blue-100 text-blue-800",
  ORIGINATION: "bg-gray-100 text-gray-700",
  DPD_CORRECTION: "bg-purple-100 text-purple-800",
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SicrEvidenceCard({
  triggerType,
  evidence,
  date,
  compact = false,
}: SicrEvidenceCardProps) {
  const label = TRIGGER_LABEL_MAP[triggerType] ?? triggerType;
  const color = TRIGGER_COLOR_MAP[triggerType] ?? "bg-gray-100 text-gray-700";

  const evidenceLines: string[] = [];

  if (evidence) {
    if (
      (triggerType === "RATING_DOWNGRADE" || triggerType === "IG_TO_NON_IG") &&
      evidence.ratingLama &&
      evidence.ratingBaru
    ) {
      evidenceLines.push(
        `Peringkat: ${evidence.ratingLama} → ${evidence.ratingBaru}`,
      );
      if (evidence.notchChange != null) {
        evidenceLines.push(
          `Delta notch: ${evidence.notchChange < 0 ? evidence.notchChange : `+${evidence.notchChange}`}`,
        );
      }
    }

    if (
      (triggerType === "DPD_THRESHOLD" || triggerType === "DPD_CORRECTION") &&
      evidence.dpd != null
    ) {
      evidenceLines.push(`Hari Tunggakan: ${evidence.dpd} hari`);
    }

    if (
      (triggerType === "CURE_FULL" || triggerType === "CURE_PARTIAL") &&
      evidence.curiePeriode != null
    ) {
      evidenceLines.push(
        `Pemulihan: ${evidence.curiePeriode} periode berturut-turut`,
      );
    }
  }

  if (compact) {
    return (
      <div className="flex flex-wrap gap-1 items-center">
        <span
          className={`inline-flex items-center px-2 py-0.5 text-xs font-medium rounded ${color}`}
        >
          {label}
        </span>
        {evidenceLines.length > 0 && (
          <span className="text-xs text-muted-foreground">
            {evidenceLines.join(" · ")}
          </span>
        )}
      </div>
    );
  }

  return (
    <Card className="border-l-4 border-l-amber-400">
      <CardContent className="pt-4 pb-3">
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">
          Pemicu SICR Terakhir
        </p>
        <div className="flex flex-wrap gap-2 items-start">
          <span
            className={`inline-flex items-center px-2.5 py-1 text-xs font-medium rounded-full ${color}`}
          >
            {label}
          </span>
          <div className="flex flex-col gap-1">
            {evidenceLines.map((line, i) => (
              <span key={i} className="text-sm">
                {line}
              </span>
            ))}
            {date && (
              <span className="text-xs text-muted-foreground">
                Tanggal: {date}
              </span>
            )}
            {evidenceLines.length === 0 && (
              <span className="text-xs text-muted-foreground italic">
                Detail evidence tidak tersedia.
              </span>
            )}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
