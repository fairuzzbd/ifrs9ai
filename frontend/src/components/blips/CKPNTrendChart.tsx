"use client";

import * as React from "react";
import {
  LineChart,
  Line,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { CKPNTrendPoint } from "@/lib/schemas/roll-forward.schema";

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------

function formatIDRCompact(value: number): string {
  if (value >= 1_000_000_000_000)
    return `Rp ${(value / 1_000_000_000_000).toFixed(1)}T`;
  if (value >= 1_000_000_000)
    return `Rp ${(value / 1_000_000_000).toFixed(1)}M`;
  if (value >= 1_000_000)
    return `Rp ${(value / 1_000_000).toFixed(1)}Jt`;
  return `Rp ${value.toLocaleString("id-ID")}`;
}

function formatIDRFull(value: number): string {
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(value);
}

function parseStr(v: string | null | undefined): number {
  if (!v) return 0;
  const n = parseFloat(v);
  return isNaN(n) ? 0 : n;
}

// ---------------------------------------------------------------------------
// Chart data shape
// ---------------------------------------------------------------------------

interface ChartPoint {
  periodeId: string;
  calcRunId: string;
  priorCalcRunId: string | null;
  total: number;
  stage1: number;
  stage2: number;
  stage3: number;
  delta: number | null;
}

function toChartData(points: CKPNTrendPoint[]): ChartPoint[] {
  return points.map((p) => ({
    periodeId: p.periodeId,
    calcRunId: p.calcRunId,
    priorCalcRunId: p.priorCalcRunId ?? null,
    total: parseStr(p.eclTotalIdr),
    stage1: parseStr(p.eclByStage.stage1),
    stage2: parseStr(p.eclByStage.stage2),
    stage3: parseStr(p.eclByStage.stage3),
    delta: p.deltaVsPriorIdr ? parseStr(p.deltaVsPriorIdr) : null,
  }));
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface CKPNTrendChartProps {
  points: CKPNTrendPoint[];
  onClickPoint?: (point: CKPNTrendPoint) => void;
  className?: string;
}

type ChartMode = "line" | "stacked-bar";

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function CKPNTrendChart({
  points,
  onClickPoint,
  className,
}: CKPNTrendChartProps) {
  const [mode, setMode] = React.useState<ChartMode>("line");

  const data = React.useMemo(() => toChartData(points), [points]);

  const handleClick = (payload: Record<string, unknown>) => {
    if (!onClickPoint || !payload?.activePayload) return;
    const ap = payload.activePayload as Array<{ payload: ChartPoint }> | undefined;
    if (!ap?.[0]) return;
    const cp = ap[0].payload;
    const original = points.find((p) => p.calcRunId === cp.calcRunId);
    if (original) onClickPoint(original);
  };

  const tooltipFormatter = (value: unknown, name: unknown) => {
    const num = typeof value === "number" ? value : Number(value);
    const labels: Record<string, string> = {
      total: "Total ECL",
      stage1: "Stage 1",
      stage2: "Stage 2",
      stage3: "Stage 3",
    };
    return [formatIDRFull(num), labels[String(name)] ?? String(name)];
  };

  return (
    <div className={cn("space-y-3", className)}>
      {/* Toggle */}
      <div className="flex gap-1">
        <Button
          variant={mode === "line" ? "default" : "outline"}
          size="sm"
          onClick={() => setMode("line")}
          aria-pressed={mode === "line"}
        >
          Tren Total
        </Button>
        <Button
          variant={mode === "stacked-bar" ? "default" : "outline"}
          size="sm"
          onClick={() => setMode("stacked-bar")}
          aria-pressed={mode === "stacked-bar"}
        >
          Per Stage
        </Button>
      </div>

      {/* Charts */}
      {mode === "line" ? (
        <div className="h-64 w-full">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart
              data={data}
              margin={{ top: 8, right: 16, left: 8, bottom: 24 }}
              onClick={onClickPoint ? (p: Record<string, unknown>) => handleClick(p) : undefined}
              style={onClickPoint ? { cursor: "pointer" } : undefined}
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis
                dataKey="periodeId"
                tick={{ fontSize: 11 }}
                angle={-30}
                textAnchor="end"
                height={48}
              />
              <YAxis
                tickFormatter={formatIDRCompact}
                tick={{ fontSize: 10 }}
                width={90}
              />
              <Tooltip
                formatter={tooltipFormatter as Parameters<typeof Tooltip>[0]["formatter"]}
                labelFormatter={(label) => `Periode: ${String(label)}`}
              />
              <Legend wrapperStyle={{ fontSize: 12 }} />
              <Line
                type="monotone"
                dataKey="total"
                name="Total ECL"
                stroke="#2563eb"
                strokeWidth={2}
                dot={{ r: 4, fill: "#2563eb" }}
                activeDot={{ r: 6 }}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      ) : (
        <div className="h-64 w-full">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart
              data={data}
              margin={{ top: 8, right: 16, left: 8, bottom: 24 }}
              onClick={onClickPoint ? (p: Record<string, unknown>) => handleClick(p) : undefined}
              style={onClickPoint ? { cursor: "pointer" } : undefined}
            >
              <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
              <XAxis
                dataKey="periodeId"
                tick={{ fontSize: 11 }}
                angle={-30}
                textAnchor="end"
                height={48}
              />
              <YAxis
                tickFormatter={formatIDRCompact}
                tick={{ fontSize: 10 }}
                width={90}
              />
              <Tooltip
                formatter={tooltipFormatter as Parameters<typeof Tooltip>[0]["formatter"]}
                labelFormatter={(label) => `Periode: ${String(label)}`}
              />
              <Legend wrapperStyle={{ fontSize: 12 }} />
              <Bar dataKey="stage1" name="Stage 1" stackId="a" fill="#22c55e" />
              <Bar dataKey="stage2" name="Stage 2" stackId="a" fill="#f59e0b" />
              <Bar dataKey="stage3" name="Stage 3" stackId="a" fill="#ef4444" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}

      {onClickPoint && (
        <p className="text-xs text-muted-foreground">
          Klik pada titik / batang untuk membuka laporan roll-forward periode tersebut.
        </p>
      )}
    </div>
  );
}
