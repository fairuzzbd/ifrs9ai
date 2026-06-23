"use client";

import * as React from "react";
import { CheckCircle2, XCircle, Flag, AlertCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { DryRunResult, RowValidationError, StageSummary } from "@/lib/schemas/bulkupload.schema";

// ---------------------------------------------------------------------------
// Stage summary card
// ---------------------------------------------------------------------------

function StageSummaryItem({
  label,
  status,
  errorCount,
}: {
  label: string;
  status: "PASS" | "FAIL" | "PARTIAL" | "UNAVAILABLE" | undefined;
  errorCount?: number;
}) {
  const isPassing = status === "PASS" || status === "PARTIAL";
  const isUnavailable = status === "UNAVAILABLE";
  return (
    <div className="flex items-center justify-between p-3 rounded-md border">
      <div className="flex items-center gap-2">
        {isPassing ? (
          <CheckCircle2 className="h-4 w-4 text-green-600" aria-hidden="true" />
        ) : isUnavailable ? (
          <AlertCircle className="h-4 w-4 text-amber-500" aria-hidden="true" />
        ) : (
          <XCircle className="h-4 w-4 text-red-500" aria-hidden="true" />
        )}
        <span className="text-sm font-medium">{label}</span>
      </div>
      <div className="flex items-center gap-2">
        {errorCount !== undefined && errorCount > 0 && (
          <span className="text-xs text-red-600">{errorCount} error</span>
        )}
        <Badge
          variant={isPassing ? "secondary" : isUnavailable ? "outline" : "destructive"}
          className={cn(isPassing && "bg-green-100 text-green-700", isUnavailable && "text-amber-600")}
        >
          {status ?? "—"}
        </Badge>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export interface BulkDryRunResultPanelProps {
  result: DryRunResult;
  className?: string;
}

export function BulkDryRunResultPanel({ result, className }: BulkDryRunResultPanelProps) {
  const [showErrors, setShowErrors] = React.useState(false);
  const summary: StageSummary = result.stageSummary ?? {};
  const errors: RowValidationError[] = result.errorsPerRow ?? [];

  const passed = result.status === "DRY_RUN_PASSED";

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          {passed ? (
            <CheckCircle2 className="h-5 w-5 text-green-600" aria-hidden="true" />
          ) : (
            <XCircle className="h-5 w-5 text-red-500" aria-hidden="true" />
          )}
          Hasil DRY_RUN: {passed ? "Lulus" : "Gagal"}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Row counts */}
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          {[
            { label: "Total Baris", value: result.totalRows ?? 0, color: "text-foreground" },
            { label: "Baris Valid", value: result.validRows ?? 0, color: "text-green-700" },
            { label: "Baris Invalid", value: result.invalidRows ?? 0, color: "text-red-600" },
            {
              label: "Perlu Review Manual",
              value: result.flaggedRows ?? 0,
              color: "text-amber-600",
            },
          ].map(({ label, value, color }) => (
            <div key={label} className="rounded-md border p-3 text-center">
              <p className={cn("text-xl font-bold", color)}>{value}</p>
              <p className="text-xs text-muted-foreground mt-0.5">{label}</p>
            </div>
          ))}
        </div>

        {/* Stage summary */}
        <div className="space-y-2">
          <p className="text-sm font-medium text-muted-foreground">Tahapan Validasi</p>
          <div className="space-y-1.5">
            <StageSummaryItem
              label="Tahap 1: Format & Tipe Sel"
              status={summary.stage1?.status}
              errorCount={summary.stage1?.errorCount}
            />
            <StageSummaryItem
              label="Tahap 2: Aturan Bisnis"
              status={summary.stage2?.status}
              errorCount={summary.stage2?.errorCount}
            />
            <StageSummaryItem
              label="Tahap 3: Referensi Silang"
              status={summary.stage3?.status}
              errorCount={summary.stage3?.errorCount}
            />
            <StageSummaryItem
              label={`Tahap 4: SPPI+BM Auto-Eval${summary.stage4?.sppiServiceUnavailable ? " (service unavailable)" : ""}`}
              status={summary.stage4?.status}
              errorCount={summary.stage4?.flagged}
            />
          </div>
        </div>

        {/* Flagged rows notice */}
        {(result.flaggedRows ?? 0) > 0 && (
          <div className="flex items-start gap-2 p-3 rounded-md bg-amber-50 border border-amber-200">
            <Flag className="h-4 w-4 text-amber-600 mt-0.5 shrink-0" aria-hidden="true" />
            <p className="text-xs text-amber-800">
              {result.flaggedRows} baris memerlukan review klasifikasi PSAK 71 manual oleh ROLE-RISK.
              COMMIT tetap diizinkan — instrumen akan di-INSERT dengan status PENDING_CLASSIFICATION.
            </p>
          </div>
        )}

        {/* Dry run expiry */}
        {result.dryRunExpiresAt && (
          <p className="text-xs text-muted-foreground">
            Hasil DRY_RUN berlaku hingga:{" "}
            <span className="font-mono">{new Date(result.dryRunExpiresAt).toLocaleString("id-ID", { timeZone: "Asia/Jakarta" })}</span>
          </p>
        )}

        {/* Error details */}
        {errors.length > 0 && (
          <div>
            <button
              type="button"
              onClick={() => setShowErrors((v) => !v)}
              className="text-sm font-medium text-primary underline-offset-4 hover:underline"
              aria-expanded={showErrors}
            >
              {showErrors ? "Sembunyikan" : "Tampilkan"} detail error ({errors.length} baris)
            </button>
            {showErrors && (
              <div className="mt-2 max-h-64 overflow-y-auto rounded-md border">
                <table className="w-full text-xs">
                  <thead className="sticky top-0 bg-muted">
                    <tr>
                      <th className="text-left px-3 py-2">Sheet</th>
                      <th className="text-left px-3 py-2">Baris</th>
                      <th className="text-left px-3 py-2">Tahap</th>
                      <th className="text-left px-3 py-2">Kolom</th>
                      <th className="text-left px-3 py-2">Error</th>
                    </tr>
                  </thead>
                  <tbody>
                    {errors.map((err, i) => (
                      <tr key={i} className="border-t">
                        <td className="px-3 py-2 font-mono">{err.sheet}</td>
                        <td className="px-3 py-2">{err.row}</td>
                        <td className="px-3 py-2">{err.stage}</td>
                        <td className="px-3 py-2 font-mono">{err.col ?? "—"}</td>
                        <td className="px-3 py-2 text-red-700">{err.error}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
