"use client";

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";

import { eclCoreApi } from "@/lib/api/ecl-core.api";
import { calcRunApi } from "@/lib/api/calc-run.api";
import type { EclResultLine } from "@/lib/schemas/ecl-core.schema";
import { PortfolioSummaryKPI } from "@/components/blips/PortfolioSummaryKPI";
import { EclStageBarChart, RoutingPieChart } from "@/components/blips/EclTrendChart";
import { StageBadge } from "@/components/blips/StageBadge";
import { DataTable } from "@/components/blips/DataTable";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { usePermissions } from "@/lib/stores/auth.store";

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

function deltaColor(current: string | null | undefined, prior: string | null | undefined): string {
  if (!current || !prior) return "";
  const delta = parseFloat(current) - parseFloat(prior);
  if (delta > 0) return "text-red-700 font-medium"; // ECL naik = risiko naik
  if (delta < 0) return "text-green-700 font-medium"; // ECL turun = risiko turun
  return "text-muted-foreground";
}

function formatDelta(current: string | null | undefined, prior: string | null | undefined): string {
  if (!current || !prior) return "—";
  const c = parseFloat(current);
  const p = parseFloat(prior);
  const delta = c - p;
  const pct = p !== 0 ? ((delta / p) * 100).toFixed(2) : "0.00";
  const sign = delta >= 0 ? "+" : "";
  return `${sign}${new Intl.NumberFormat("id-ID").format(Math.round(delta))} (${sign}${pct}%)`;
}

// ---------------------------------------------------------------------------
// Instrument columns
// ---------------------------------------------------------------------------

function buildInstrColumns(
  calcRunId: string,
  router: ReturnType<typeof useRouter>,
): ColumnDef<EclResultLine>[] {
  return [
    {
      id: "kodeInstrumen",
      header: "Kode Instrumen",
      enableSorting: true,
      cell: ({ row }) => (
        <button
          className="text-sm font-mono text-primary underline-offset-4 hover:underline"
          onClick={() =>
            router.push(`/ecl/calc-runs/${calcRunId}/instrumen/${row.original.instrumenId}`)
          }
        >
          {row.original.kodeInstrumen ?? row.original.instrumenId.slice(0, 12)}
        </button>
      ),
    },
    {
      id: "namaInstrumen",
      header: "Nama",
      cell: ({ row }) => (
        <span className="text-sm line-clamp-1 max-w-xs">{row.original.namaInstrumen ?? "—"}</span>
      ),
    },
    {
      id: "stage",
      header: "Stage",
      enableSorting: true,
      cell: ({ row }) =>
        row.original.stage ? <StageBadge stage={row.original.stage} size="sm" /> : null,
    },
    {
      id: "routingPath",
      header: "Jalur",
      cell: ({ row }) => (
        <Badge variant="outline" className="text-xs">{row.original.routingPath}</Badge>
      ),
    },
    {
      id: "eadIdr",
      header: "EAD (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs tabular-nums">{formatIDR(row.original.eadIdr)}</span>
      ),
    },
    {
      id: "eclWeightedIdr",
      header: "ECL Weighted (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs tabular-nums font-medium">{formatIDR(row.original.eclWeightedIdr)}</span>
      ),
    },
  ];
}

// ---------------------------------------------------------------------------
// Chart color palettes
// ---------------------------------------------------------------------------

const STAGE_CHART_DATA = (s1Idr: string, s2Idr: string, s3Idr: string) => [
  { stage: "Stage 1", eclIdr: parseFloat(s1Idr) || 0, fill: "#22c55e" },
  { stage: "Stage 2", eclIdr: parseFloat(s2Idr) || 0, fill: "#f59e0b" },
  { stage: "Stage 3", eclIdr: parseFloat(s3Idr) || 0, fill: "#ef4444" },
];

const ROUTING_COLORS: Record<string, string> = {
  STANDARD: "#6b7280",
  LPS: "#3b82f6",
  LOOKTHROUGH: "#8b5cf6",
  POCI_DEFERRED: "#f59e0b",
  FVTPL_SKIPPED: "#d1d5db",
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function PortfolioSummaryPage() {
  const params = useParams<{ id: string; portofolioId: string }>();
  const router = useRouter();
  const { can } = usePermissions();

  const calcRunId = params.id;
  const portofolioId = params.portofolioId;

  const [priorRunId, setPriorRunId] = React.useState<string | null>(null);
  const [instrCursor, setInstrCursor] = React.useState<string | null>(null);
  const [instrPage, setInstrPage] = React.useState(1);
  const [instrPrevCursors, setInstrPrevCursors] = React.useState<string[]>([]);
  const [instrSearch, setInstrSearch] = React.useState("");
  const [showInstrumen, setShowInstrumen] = React.useState(false);

  const { data: summaryData, isLoading } = useQuery({
    queryKey: ["portfolio-summary", calcRunId, portofolioId, priorRunId],
    queryFn: () =>
      eclCoreApi.getPortfolioSummary(calcRunId, portofolioId, {
        priorCalcRunId: priorRunId ?? undefined,
      }),
  });

  const summary = summaryData?.data;

  // Prior runs list
  const { data: priorRunsData } = useQuery({
    queryKey: ["prior-runs-list"],
    queryFn: () =>
      calcRunApi.list({
        "filter[status]": "SEALED",
        limit: 50,
        sort: "created_at:desc",
      }),
  });
  const priorRuns = (priorRunsData?.data ?? []).filter((r) => r.id !== calcRunId);

  const { data: instrData, isLoading: instrLoading } = useQuery({
    queryKey: ["portfolio-instruments", calcRunId, portofolioId, instrCursor, instrSearch],
    queryFn: () =>
      eclCoreApi.listResults(calcRunId, {
        "filter[portofolio_id]": portofolioId,
        limit: 50,
        sort: "ecl_weighted_idr:desc",
        ...(instrCursor && { cursor: instrCursor }),
        ...(instrSearch && { q: instrSearch }),
      }),
    enabled: showInstrumen,
  });

  if (isLoading || !summary) {
    return (
      <div className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-6 bg-muted rounded w-1/2" />
          <div className="h-32 bg-muted rounded" />
        </div>
      </div>
    );
  }

  const stageChartData = STAGE_CHART_DATA(
    summary.stage1EclIdr,
    summary.stage2EclIdr,
    summary.stage3EclIdr,
  );

  const routingPieData = (summary.routingDistribution ?? []).map((rd) => ({
    name: rd.routingPath,
    value: rd.count,
    fill: ROUTING_COLORS[rd.routingPath] ?? "#9ca3af",
  }));

  const instrColumns = buildInstrColumns(calcRunId, router);

  const handleExport = (format: "csv" | "xlsx") => {
    eclCoreApi.exportPortfolioSummary(calcRunId, portofolioId, format);
  };

  return (
    <div className="p-6 space-y-6 max-w-7xl">
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
          <li className="text-foreground">{summary.portofolioNama ?? portofolioId}</li>
        </ol>
      </nav>

      {/* Page header */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div>
          <h1 className="text-xl font-semibold">
            Ringkasan Portofolio: {summary.portofolioNama ?? portofolioId}
          </h1>
          <p className="text-sm text-muted-foreground">
            Calc Run: {calcRunId}
          </p>
        </div>
        {can("ecl.result.export") && (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => handleExport("csv")}>
              Export CSV
            </Button>
            <Button variant="outline" size="sm" onClick={() => handleExport("xlsx")}>
              Export XLSX
            </Button>
          </div>
        )}
      </div>

      {/* KPI cards */}
      <PortfolioSummaryKPI
        totalEclWeightedIdr={summary.totalEclWeightedIdr}
        totalInstrumen={summary.totalInstrumen}
        stage1Count={summary.stage1Count}
        stage2Count={summary.stage2Count}
        stage3Count={summary.stage3Count}
        errorCount={summary.errorCount}
      />

      {/* Charts */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">ECL per Stage</CardTitle>
          </CardHeader>
          <CardContent>
            <EclStageBarChart data={stageChartData} />
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Distribusi Jalur Perhitungan</CardTitle>
          </CardHeader>
          <CardContent>
            {routingPieData.length > 0 ? (
              <RoutingPieChart data={routingPieData} />
            ) : (
              <p className="text-sm text-muted-foreground py-4 text-center">
                Data distribusi tidak tersedia.
              </p>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Comparison section */}
      <Card>
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between gap-2 flex-wrap">
            <CardTitle className="text-sm">Perbandingan dengan Prior Run</CardTitle>
            <Select
              value={priorRunId ?? "_none"}
              onValueChange={(v) => setPriorRunId(v === "_none" ? null : v)}
            >
              <SelectTrigger className="w-64" aria-label="Pilih prior calc run untuk perbandingan">
                <SelectValue placeholder="Pilih prior run..." />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="_none">Pilih prior run...</SelectItem>
                {priorRuns.map((r) => (
                  <SelectItem key={r.id} value={r.id}>
                    {r.id} ({r.periodeLabel ?? r.periodeId}, {r.status})
                  </SelectItem>
                ))}
                {priorRuns.length === 0 && (
                  <SelectItem value="_empty" disabled>
                    Tidak ada prior run SEALED
                  </SelectItem>
                )}
              </SelectContent>
            </Select>
          </div>
        </CardHeader>
        <CardContent>
          {!priorRunId ? (
            <p className="text-sm text-muted-foreground">
              Pilih prior run dari dropdown untuk melihat perbandingan.
            </p>
          ) : !summary.priorTotalEclWeightedIdr ? (
            <p className="text-sm text-muted-foreground">
              Tidak ada data prior run untuk perbandingan.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm border-collapse">
                <thead>
                  <tr className="border-b">
                    <th className="py-2 px-3 text-left font-medium text-muted-foreground">Metrik</th>
                    <th className="py-2 px-3 text-right font-medium text-muted-foreground">Prior</th>
                    <th className="py-2 px-3 text-right font-medium text-muted-foreground">Current</th>
                    <th className="py-2 px-3 text-right font-medium text-muted-foreground">Delta</th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    {
                      label: "Total ECL Weighted",
                      current: summary.totalEclWeightedIdr,
                      prior: summary.priorTotalEclWeightedIdr,
                    },
                    {
                      label: "Stage 1 ECL",
                      current: summary.stage1EclIdr,
                      prior: summary.priorStage1EclIdr,
                    },
                    {
                      label: "Stage 2 ECL",
                      current: summary.stage2EclIdr,
                      prior: summary.priorStage2EclIdr,
                    },
                    {
                      label: "Stage 3 ECL",
                      current: summary.stage3EclIdr,
                      prior: summary.priorStage3EclIdr,
                    },
                  ].map((row) => (
                    <tr key={row.label} className="border-b hover:bg-muted/20">
                      <td className="py-2 px-3">{row.label}</td>
                      <td className="py-2 px-3 text-right font-mono text-xs">{formatIDR(row.prior)}</td>
                      <td className="py-2 px-3 text-right font-mono text-xs">{formatIDR(row.current)}</td>
                      <td className={`py-2 px-3 text-right font-mono text-xs ${deltaColor(row.current, row.prior)}`}>
                        {formatDelta(row.current, row.prior)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <p className="text-xs text-muted-foreground mt-2">
                Catatan: Delta positif (ECL naik) = merah; delta negatif (ECL turun) = hijau. ECL naik berarti risiko kredit meningkat.
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Instrument drill-down */}
      <Card>
        <CardHeader className="pb-2">
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm">Drill-Down Instrumen</CardTitle>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setShowInstrumen((prev) => !prev)}
            >
              {showInstrumen ? "Sembunyikan" : "Lihat Instrumen"}
            </Button>
          </div>
        </CardHeader>
        {showInstrumen && (
          <CardContent className="p-0">
            <DataTable
              columns={instrColumns}
              data={instrData?.data ?? []}
              pagination={instrData?.pagination}
              isLoading={instrLoading}
              searchValue={instrSearch}
              onSearchChange={setInstrSearch}
              searchPlaceholder="Cari kode instrumen..."
              onExport={handleExport}
              onNextPage={() => {
                const next = instrData?.pagination?.nextCursor ?? null;
                if (next) {
                  setInstrPrevCursors((p) => [...p, instrCursor ?? ""]);
                  setInstrCursor(next);
                  setInstrPage((n) => n + 1);
                }
              }}
              onPrevPage={() => {
                const prev = instrPrevCursors[instrPrevCursors.length - 1] ?? null;
                setInstrPrevCursors((p) => p.slice(0, -1));
                setInstrCursor(prev);
                setInstrPage((n) => Math.max(1, n - 1));
              }}
              canPrevPage={instrPage > 1}
              pageNumber={instrPage}
              emptyMessage="Tidak ada instrumen di portofolio ini."
            />
          </CardContent>
        )}
      </Card>
    </div>
  );
}
