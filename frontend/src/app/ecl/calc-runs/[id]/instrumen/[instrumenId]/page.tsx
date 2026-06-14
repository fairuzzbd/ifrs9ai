"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Info } from "lucide-react";

import { eclCoreApi } from "@/lib/api/ecl-core.api";
import { StageBadge } from "@/components/blips/StageBadge";
import { EclResultDrillDownTable } from "@/components/blips/EclResultDrillDownTable";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Separator } from "@/components/ui/separator";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
}

function formatRate(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return `${(num * 100).toFixed(8)}%`;
}

// ---------------------------------------------------------------------------
// Routing path badge inline
// ---------------------------------------------------------------------------

const ROUTING_COLORS: Record<string, { bg: string; text: string }> = {
  STANDARD: { bg: "bg-gray-100", text: "text-gray-700" },
  LPS: { bg: "bg-blue-100", text: "text-blue-800" },
  LOOKTHROUGH: { bg: "bg-purple-100", text: "text-purple-800" },
  POCI_DEFERRED: { bg: "bg-amber-100", text: "text-amber-800" },
  FVTPL_SKIPPED: { bg: "bg-gray-100", text: "text-gray-500" },
};

function RoutingBadge({ path }: { path: string }) {
  const colors = ROUTING_COLORS[path] ?? ROUTING_COLORS["STANDARD"];
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-full ${colors.bg} ${colors.text}`}
      aria-label={`Jalur perhitungan: ${path}`}
    >
      {path}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function EclDrillDownPage() {
  const params = useParams<{ id: string; instrumenId: string }>();
  const router = useRouter();
  const calcRunId = params.id;
  const instrumenId = params.instrumenId;

  const { data, isLoading } = useQuery({
    queryKey: ["ecl-drill-down", calcRunId, instrumenId],
    queryFn: () => eclCoreApi.getInstrumentDrillDown(calcRunId, instrumenId),
  });

  const result = data?.data;

  if (isLoading) {
    return (
      <div className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-6 bg-muted rounded w-1/2" />
          <div className="h-48 bg-muted rounded" />
        </div>
      </div>
    );
  }

  if (!result) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Data instrumen tidak ditemukan.</p>
      </div>
    );
  }

  const isFvtplSkipped = result.routingPath === "FVTPL_SKIPPED";
  const isStage3 = result.stage === 3;
  const hasWarnings = (result.warnings?.length ?? 0) > 0;
  const hasLookthrough = result.routingPath === "LOOKTHROUGH" && (result.lookthroughUnderlying?.length ?? 0) > 0;
  const hasLps = result.routingPath === "LPS" && result.lpsAggregation;

  return (
    <div className="p-6 space-y-6 max-w-5xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1 flex-wrap">
          <li>
            <button className="hover:underline" onClick={() => router.push("/ecl/calc-runs")}>
              Calc Runs
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push(`/ecl/calc-runs/${calcRunId}`)}
            >
              {calcRunId}
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground font-mono">
            {result.kodeInstrumen ?? instrumenId}
          </li>
        </ol>
      </nav>

      {/* Info Card */}
      <Card>
        <CardContent className="pt-4">
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <div className="space-y-1">
              <div className="flex items-center gap-2 flex-wrap">
                <h1 className="text-lg font-bold font-mono">
                  {result.kodeInstrumen ?? instrumenId}
                </h1>
                <RoutingBadge path={result.routingPath} />
                {result.stage && <StageBadge stage={result.stage} size="sm" />}
              </div>
              {result.namaInstrumen && (
                <p className="text-sm text-muted-foreground">{result.namaInstrumen}</p>
              )}
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm mt-2">
                {result.klasifikasiPsak71 && (
                  <span>Klasifikasi: <strong>{result.klasifikasiPsak71}</strong></span>
                )}
                {result.portofolioNama && (
                  <span>Portofolio: <strong>{result.portofolioNama}</strong></span>
                )}
                {result.lgdUsed && (
                  <span>LGD: <strong className="font-mono">{formatRate(result.lgdUsed)}</strong></span>
                )}
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm mt-1">
                {!isFvtplSkipped && (
                  <>
                    <span>
                      EAD: <strong className="font-mono">{formatIDR(result.eadIdr)}</strong>
                    </span>
                    <span>
                      ECL Weighted:{" "}
                      <strong className="font-mono">{formatIDR(result.eclWeightedIdr)}</strong>
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* FVTPL info banner */}
      {isFvtplSkipped && (
        <Alert>
          <Info className="h-4 w-4" aria-hidden="true" />
          <AlertTitle>Instrumen FVTPL — ECL Tidak Dihitung</AlertTitle>
          <AlertDescription>
            Instrumen ini berstatus FVTPL. ECL tidak dihitung sesuai PSAK 71 / IFRS 9.
            Tidak ada breakdown skenario.
          </AlertDescription>
        </Alert>
      )}

      {/* Stage 3 info */}
      {isStage3 && result.netCarryingIdr && (
        <Alert className="border-blue-200 bg-blue-50">
          <Info className="h-4 w-4 text-blue-700" aria-hidden="true" />
          <AlertTitle className="text-blue-800">Bunga dihitung dari Net Carrying Amount</AlertTitle>
          <AlertDescription className="text-blue-700">
            Net Carrying: <strong className="font-mono">{formatIDR(result.netCarryingIdr)}</strong>
            {result.grossCarryingIdr && (
              <> (Gross {formatIDR(result.grossCarryingIdr)} − ECL Allowance {formatIDR(result.eclAllowancePriorIdr)})</>
            )}
            — PSAK 71 §5.4.1(b)
          </AlertDescription>
        </Alert>
      )}

      {/* Warnings */}
      {hasWarnings && (
        <Alert variant="default" className="border-amber-200 bg-amber-50">
          <AlertTriangle className="h-4 w-4 text-amber-700" aria-hidden="true" />
          <AlertTitle className="text-amber-800">
            {result.warnings.length} Warning
          </AlertTitle>
          <AlertDescription className="text-amber-700">
            {result.warnings.map((w, i) => (
              <p key={i} className="text-sm">{w}</p>
            ))}
          </AlertDescription>
        </Alert>
      )}

      {/* Scenario Breakdown Table */}
      {!isFvtplSkipped && result.scenarioBreakdown && result.scenarioBreakdown.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Breakdown Skenario</CardTitle>
          </CardHeader>
          <CardContent className="p-0 pb-4">
            <EclResultDrillDownTable
              scenarioBreakdown={result.scenarioBreakdown}
              eclWeightedIdr={result.eclWeightedIdr}
              stage={result.stage ?? null}
            />
          </CardContent>
        </Card>
      )}

      {/* Look-through section */}
      {hasLookthrough && result.lookthroughUnderlying && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Look-through Underlying</CardTitle>
          </CardHeader>
          <CardContent className="p-0 pb-4">
            <div className="overflow-x-auto">
              <table className="w-full text-sm border-collapse">
                <thead>
                  <tr className="border-b">
                    <th className="py-2 px-4 text-left font-medium text-muted-foreground">Asset Class</th>
                    <th className="py-2 px-4 text-right font-medium text-muted-foreground">% NAB</th>
                    <th className="py-2 px-4 text-right font-medium text-muted-foreground">EAD (IDR)</th>
                    <th className="py-2 px-4 text-right font-medium text-muted-foreground">ECL per Class</th>
                  </tr>
                </thead>
                <tbody>
                  {result.lookthroughUnderlying.map((u, i) => (
                    <tr key={i} className="border-b hover:bg-muted/20">
                      <td className="py-2 px-4">{u.assetClass}</td>
                      <td className="py-2 px-4 text-right font-mono text-xs">{u.percentNab}%</td>
                      <td className="py-2 px-4 text-right font-mono text-xs">{formatIDR(u.eadIdr)}</td>
                      <td className="py-2 px-4 text-right font-mono text-xs">{formatIDR(u.eclIdr)}</td>
                    </tr>
                  ))}
                  <tr className="font-semibold bg-muted/20">
                    <td className="py-2 px-4" colSpan={3}>
                      Total ECL (sama dengan ECL Weighted di atas)
                    </td>
                    <td className="py-2 px-4 text-right font-mono text-xs">
                      {formatIDR(result.eclWeightedIdr)}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </CardContent>
        </Card>
      )}

      {/* LPS section */}
      {hasLps && result.lpsAggregation && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">LPS Aggregasi</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between py-1 border-b">
                <span className="text-muted-foreground">Total Eksposur (nasabah + bank)</span>
                <span className="font-mono">{formatIDR(result.lpsAggregation.totalEksposurIdr)}</span>
              </div>
              <div className="flex justify-between py-1 border-b">
                <span className="text-muted-foreground">Dijamin LPS (cap IDR 2 miliar)</span>
                <span className="font-mono">{formatIDR(result.lpsAggregation.coveredIdr)}</span>
              </div>
              <div className="flex justify-between py-1 font-semibold">
                <span>Excess (ECL basis)</span>
                <span className="font-mono">{formatIDR(result.lpsAggregation.excessIdr)}</span>
              </div>
            </div>
            <p className="text-xs text-muted-foreground mt-2">
              ECL hanya dihitung untuk excess di atas cap LPS IDR 2 miliar (DEC-014).
            </p>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
