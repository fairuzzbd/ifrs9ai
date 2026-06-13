"use client";

import * as React from "react";
import { ExternalLink } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface CatchUpAdjustmentCardProps {
  adjustment: {
    deltaAmount: string; // IDR NUMERIC(20,4) as string
    formulaVersion: string;
    approvalRecordUrl?: string;
  } | null;
}

// ---------------------------------------------------------------------------
// IDR formatter (string-based — no parseFloat)
// ---------------------------------------------------------------------------

function formatIDRString(value: string): string {
  try {
    // Parse as integer+decimal parts
    const [intPart, decPart = "0000"] = value.split(".");
    const int = parseInt(intPart, 10);
    if (isNaN(int)) return value;
    const formatted = new Intl.NumberFormat("id-ID").format(int);
    return `Rp ${formatted},${decPart.padEnd(4, "0").slice(0, 4)}`;
  } catch {
    return value;
  }
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CatchUpAdjustmentCard({ adjustment }: CatchUpAdjustmentCardProps) {
  if (!adjustment) return null;

  const { deltaAmount, formulaVersion, approvalRecordUrl } = adjustment;

  return (
    <Card className="border border-blue-200 bg-blue-50/50">
      <CardContent className="pt-4 pb-3">
        <p className="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">
          Penyesuaian Catch-Up
        </p>
        <div className="space-y-1">
          <div className="flex justify-between items-baseline">
            <span className="text-sm text-muted-foreground">Delta</span>
            <span className="font-mono text-sm font-medium">
              {formatIDRString(deltaAmount)}
            </span>
          </div>
          <div className="flex justify-between items-baseline">
            <span className="text-sm text-muted-foreground">Formula</span>
            <span className="text-sm">{formulaVersion}</span>
          </div>
          {approvalRecordUrl && (
            <div className="pt-1">
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0 text-xs"
                asChild
              >
                <a href={approvalRecordUrl} target="_blank" rel="noreferrer">
                  Lihat Approval Record
                  <ExternalLink className="ml-1 h-3 w-3" aria-hidden="true" />
                </a>
              </Button>
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
