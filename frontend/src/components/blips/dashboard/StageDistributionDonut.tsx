"use client";

/**
 * P5-M15 — StageDistributionDonut: ECL stage distribution PieChart (donut).
 * WCAG AA: color + label + screen-reader summary table.
 */

import * as React from "react";
import {
  PieChart,
  Pie,
  Cell,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import { Skeleton } from "@/components/ui/skeleton";
import { formatIDR } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface StageDatum {
  stage: 1 | 2 | 3;
  count: number;
  eclTotal: number; // raw number (IDR)
}

export interface StageDistributionDonutProps {
  data: StageDatum[];
  totalCount: number;
  loading?: boolean;
  error?: string;
  ariaLabel?: string;
  className?: string;
}

// WCAG-compliant palette (contrast verified in design §10.4)
const STAGE_COLORS = {
  1: "hsl(142, 76%, 36%)",  // green-600 AA 4.6:1
  2: "hsl(38, 92%, 50%)",   // amber-500 — text uses amber-800 for label
  3: "hsl(0, 72%, 51%)",    // red-500 AA 4.5:1
};

const STAGE_LABELS: Record<number, string> = {
  1: "Stage 1 (Performing)",
  2: "Stage 2 (SICR)",
  3: "Stage 3 (Default)",
};

function CustomLabel({
  cx,
  cy,
  totalCount,
}: {
  cx: number;
  cy: number;
  totalCount: number;
}) {
  return (
    <>
      <text
        x={cx}
        y={cy - 8}
        textAnchor="middle"
        dominantBaseline="central"
        className="text-xs fill-muted-foreground"
        aria-hidden="true"
      >
        Total
      </text>
      <text
        x={cx}
        y={cy + 12}
        textAnchor="middle"
        dominantBaseline="central"
        className="text-sm font-bold fill-foreground"
        aria-hidden="true"
      >
        {totalCount.toLocaleString("id-ID")}
      </text>
    </>
  );
}

export function StageDistributionDonut({
  data,
  totalCount,
  loading = false,
  error,
  ariaLabel = "Distribusi Stage ECL",
  className,
}: StageDistributionDonutProps) {
  const chartId = React.useId();

  if (loading) {
    return (
      <div className={cn("space-y-2", className)}>
        <Skeleton className="h-48 w-full rounded-full mx-auto" style={{ maxWidth: 200 }} />
        <div className="flex justify-center gap-4">
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-3 w-20" />
          <Skeleton className="h-3 w-20" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <p className="text-sm text-destructive py-4 text-center">{error}</p>
    );
  }

  const pieData = data.map((d) => ({
    name: STAGE_LABELS[d.stage],
    value: d.count,
    eclTotal: d.eclTotal,
    stage: d.stage,
    pct:
      totalCount > 0
        ? ((d.count / totalCount) * 100).toFixed(1)
        : "0.0",
  }));

  return (
    <div className={cn("", className)}>
      <ResponsiveContainer width="100%" height={220}>
        <PieChart
          role="img"
          aria-labelledby={`${chartId}-title`}
          aria-describedby={`${chartId}-desc`}
        >
          <title id={`${chartId}-title`}>{ariaLabel}</title>
          <desc id={`${chartId}-desc`}>
            {`Donut chart distribusi stage ECL: ${pieData
              .map((d) => `${d.name} ${d.pct}% (${d.value} instrumen)`)
              .join(", ")}`}
          </desc>
          <Pie
            data={pieData}
            cx="50%"
            cy="50%"
            innerRadius={60}
            outerRadius={100}
            paddingAngle={2}
            dataKey="value"
            aria-hidden="true"
          >
            {pieData.map((entry) => (
              <Cell
                key={`cell-${entry.stage}`}
                fill={STAGE_COLORS[entry.stage as 1 | 2 | 3]}
                aria-label={`${entry.name}: ${entry.value} instrumen (${entry.pct}%)`}
              />
            ))}
            {/* Center label */}
            {totalCount > 0 && (
              <text
                x="50%"
                y="50%"
                textAnchor="middle"
                dominantBaseline="central"
                aria-hidden="true"
              >
                <tspan
                  x="50%"
                  dy="-8"
                  className="text-xs fill-muted-foreground"
                  fontSize={12}
                  fill="currentColor"
                >
                  Total
                </tspan>
                <tspan
                  x="50%"
                  dy="20"
                  fontSize={16}
                  fontWeight="bold"
                  fill="currentColor"
                >
                  {totalCount.toLocaleString("id-ID")}
                </tspan>
              </text>
            )}
          </Pie>
          <Tooltip
            formatter={(value: number, name: string, props: { payload?: { eclTotal: number } }) => [
              `${value.toLocaleString("id-ID")} instrumen — ECL: ${formatIDR(props.payload?.eclTotal ?? 0)}`,
              name,
            ]}
          />
          <Legend
            formatter={(value: string) => (
              <span className="text-xs text-muted-foreground">{value}</span>
            )}
          />
        </PieChart>
      </ResponsiveContainer>

      {/* Screen-reader summary table */}
      <table className="sr-only" aria-label={`Data tabel: ${ariaLabel}`}>
        <caption>Data numerik distribusi stage ECL</caption>
        <thead>
          <tr>
            <th scope="col">Stage</th>
            <th scope="col">Jumlah Instrumen</th>
            <th scope="col">Persentase</th>
            <th scope="col">Total ECL</th>
          </tr>
        </thead>
        <tbody>
          {pieData.map((d) => (
            <tr key={d.stage}>
              <td>{d.name}</td>
              <td>{d.value.toLocaleString("id-ID")}</td>
              <td>{d.pct}%</td>
              <td>{formatIDR(d.eclTotal)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
