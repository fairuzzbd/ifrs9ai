"use client";

import * as React from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  ReferenceLine,
} from "recharts";
import type { PociDeltaHistoryItem } from "@/lib/schemas/poci.schema";

const IDR_SHORT = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
  notation: "compact",
});

interface ChartDataPoint {
  date: string;
  deltaEcl: number;
  cumulative: number;
}

export interface PociDeltaHistoryChartProps {
  data: PociDeltaHistoryItem[];
  className?: string;
}

/**
 * Recharts line chart showing cumulative delta ECL over time per instrumen.
 * Two lines: per-run delta and cumulative delta since origination (S5-AC2).
 */
export function PociDeltaHistoryChart({ data, className }: PociDeltaHistoryChartProps) {
  // Build cumulative from most-recent-first data (reverse for chart)
  const chartData: ChartDataPoint[] = React.useMemo(() => {
    const sorted = [...data].sort(
      (a, b) => new Date(a.tanggalCompute).getTime() - new Date(b.tanggalCompute).getTime(),
    );
    let cumulative = 0;
    return sorted.map((row) => {
      const delta = parseFloat(row.deltaEcl) || 0;
      cumulative += delta;
      return {
        date: row.tanggalCompute,
        deltaEcl: delta,
        cumulative,
      };
    });
  }, [data]);

  if (chartData.length === 0) {
    return (
      <div className={className}>
        <p className="text-sm text-muted-foreground text-center py-8">
          Belum ada data delta untuk instrumen ini.
        </p>
      </div>
    );
  }

  return (
    <div className={className}>
      <p className="text-xs text-muted-foreground mb-2">
        Delta ECL POCI per run dan kumulatif sejak origination
      </p>
      <ResponsiveContainer width="100%" height={240}>
        <LineChart
          data={chartData}
          margin={{ top: 4, right: 16, left: 16, bottom: 4 }}
        >
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
          <XAxis
            dataKey="date"
            tick={{ fontSize: 11 }}
            tickFormatter={(v: string) =>
              new Date(v).toLocaleDateString("id-ID", { month: "short", year: "2-digit" })
            }
          />
          <YAxis
            tick={{ fontSize: 11 }}
            tickFormatter={(v: number) => IDR_SHORT.format(v)}
          />
          <ReferenceLine y={0} stroke="#94a3b8" strokeDasharray="4 2" />
          <Tooltip
            formatter={(value: number, name: string) => [
              IDR_SHORT.format(value),
              name === "deltaEcl" ? "Delta Run Ini" : "Kumulatif",
            ]}
            labelFormatter={(label: string) =>
              new Date(label).toLocaleDateString("id-ID", {
                timeZone: "Asia/Jakarta",
                day: "2-digit",
                month: "long",
                year: "numeric",
              })
            }
          />
          <Line
            type="monotone"
            dataKey="deltaEcl"
            stroke="#ef4444"
            strokeWidth={2}
            dot={{ r: 3 }}
            name="deltaEcl"
          />
          <Line
            type="monotone"
            dataKey="cumulative"
            stroke="#3b82f6"
            strokeWidth={2}
            dot={false}
            strokeDasharray="5 3"
            name="cumulative"
          />
        </LineChart>
      </ResponsiveContainer>
      <div className="flex gap-4 justify-center mt-1">
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <span className="inline-block w-4 h-0.5 bg-red-500" aria-hidden="true" />
          Delta Run Ini
        </span>
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <span className="inline-block w-4 h-0.5 bg-blue-500 opacity-70" aria-hidden="true" style={{ borderTop: "2px dashed" }} />
          Kumulatif
        </span>
      </div>
    </div>
  );
}
