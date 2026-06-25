"use client";

/**
 * P5-M15 — ScenarioSensitivityBar: 3-scenario ECL bar chart.
 */

import * as React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Cell,
  LabelList,
  ResponsiveContainer,
} from "recharts";
import { Skeleton } from "@/components/ui/skeleton";
import { formatIDRAbbrev, formatIDR } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface ScenarioDatum {
  scenario: "Good" | "Normal" | "Bad";
  eclTotal: number;
  weight: number; // e.g. 0.25
}

export interface ScenarioSensitivityBarProps {
  data: ScenarioDatum[];
  weightedEcl: number;
  loading?: boolean;
  error?: string;
  className?: string;
}

const SCENARIO_COLORS: Record<string, string> = {
  Good: "hsl(142, 76%, 36%)",
  Normal: "hsl(204, 86%, 53%)",
  Bad: "hsl(0, 72%, 51%)",
};

const SCENARIO_LABELS: Record<string, string> = {
  Good: "Optimis (Good)",
  Normal: "Base (Normal)",
  Bad: "Pesimis (Bad)",
};

export function ScenarioSensitivityBar({
  data,
  weightedEcl,
  loading = false,
  error,
  className,
}: ScenarioSensitivityBarProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-52 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive py-4 text-center">{error}</p>;
  }

  const chartData = data.map((d) => ({
    ...d,
    label: SCENARIO_LABELS[d.scenario],
    weightPct: `${(d.weight * 100).toFixed(0)}%`,
  }));

  const subLabel = data
    .map((d) => `${SCENARIO_LABELS[d.scenario].split(" ")[0]} ${(d.weight * 100).toFixed(0)}%`)
    .join(" / ");

  return (
    <div className={cn("space-y-2", className)}>
      <ResponsiveContainer width="100%" height={200}>
        <BarChart
          data={chartData}
          role="img"
          aria-labelledby={`${chartId}-title`}
          aria-describedby={`${chartId}-desc`}
        >
          <title id={`${chartId}-title`}>Sensitivitas Skenario ECL</title>
          <desc id={`${chartId}-desc`}>
            {`Bar chart 3 skenario ECL: ${chartData
              .map((d) => `${d.label} = ${formatIDRAbbrev(d.eclTotal)}`)
              .join(", ")}`}
          </desc>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis dataKey="label" tick={{ fontSize: 10 }} aria-hidden="true" />
          <YAxis
            tickFormatter={(v: number) => formatIDRAbbrev(v)}
            tick={{ fontSize: 10 }}
            aria-hidden="true"
          />
          <Tooltip
            formatter={(value: number, _name: string) => {
              const delta = value - weightedEcl;
              const sign = delta >= 0 ? "+" : "";
              return [
                `${formatIDR(value)} (${sign}${formatIDRAbbrev(delta)} vs weighted)`,
                "Total ECL",
              ];
            }}
          />
          <Bar dataKey="eclTotal" name="Total ECL" radius={[4, 4, 0, 0]}>
            {chartData.map((entry) => (
              <Cell
                key={entry.scenario}
                fill={SCENARIO_COLORS[entry.scenario]}
                aria-label={`${entry.label}: ${formatIDRAbbrev(entry.eclTotal)}`}
              />
            ))}
            <LabelList
              dataKey="eclTotal"
              position="top"
              formatter={(v: number) => formatIDRAbbrev(v)}
              style={{ fontSize: 10 }}
            />
          </Bar>
        </BarChart>
      </ResponsiveContainer>

      <p className="text-xs text-muted-foreground text-center">
        Bobot skenario aktif: {subLabel} (ALCO-approved)
      </p>

      <table className="sr-only" aria-label="Data tabel sensitivitas skenario ECL">
        <caption>Data total ECL per skenario</caption>
        <thead>
          <tr>
            <th scope="col">Skenario</th>
            <th scope="col">Total ECL</th>
            <th scope="col">Bobot</th>
          </tr>
        </thead>
        <tbody>
          {data.map((d) => (
            <tr key={d.scenario}>
              <td>{SCENARIO_LABELS[d.scenario]}</td>
              <td>{formatIDR(d.eclTotal)}</td>
              <td>{(d.weight * 100).toFixed(0)}%</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
