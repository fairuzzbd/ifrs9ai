"use client";

/**
 * P5-M15 — StageMovementBar: Line chart of stage movement per periode.
 */

import * as React from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ResponsiveContainer,
} from "recharts";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export interface StageMovementDatum {
  periode: string;
  s1Count: number;
  s2Count: number;
  s3Count: number;
}

export interface StageMovementBarProps {
  data: StageMovementDatum[];
  loading?: boolean;
  error?: string;
  ariaLabel?: string;
  className?: string;
}

const STAGE_COLORS = {
  s1: "hsl(142, 76%, 36%)",
  s2: "hsl(38, 92%, 50%)",
  s3: "hsl(0, 72%, 51%)",
};

export function StageMovementBar({
  data,
  loading = false,
  error,
  ariaLabel = "Tren Perpindahan Stage ECL",
  className,
}: StageMovementBarProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-52 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive py-4 text-center">{error}</p>;
  }

  return (
    <div className={cn("", className)}>
      <ResponsiveContainer width="100%" height={220}>
        <LineChart
          data={data}
          role="img"
          aria-labelledby={`${chartId}-title`}
          aria-describedby={`${chartId}-desc`}
        >
          <title id={`${chartId}-title`}>{ariaLabel}</title>
          <desc id={`${chartId}-desc`}>
            {`Grafik garis tren perpindahan stage ECL per periode untuk ${data.length} periode terakhir`}
          </desc>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis
            dataKey="periode"
            tick={{ fontSize: 11 }}
            aria-hidden="true"
          />
          <YAxis tick={{ fontSize: 11 }} aria-hidden="true" />
          <Tooltip
            formatter={(value: number, name: string) => [
              value.toLocaleString("id-ID"),
              name,
            ]}
          />
          <Legend
            formatter={(value: string) => (
              <span className="text-xs">{value}</span>
            )}
          />
          <Line
            type="monotone"
            dataKey="s1Count"
            name="Stage 1 (Performing)"
            stroke={STAGE_COLORS.s1}
            strokeWidth={2}
            dot={{ r: 3 }}
            aria-label="Stage 1 (Performing)"
          />
          <Line
            type="monotone"
            dataKey="s2Count"
            name="Stage 2 (SICR)"
            stroke={STAGE_COLORS.s2}
            strokeWidth={2}
            dot={{ r: 3 }}
            aria-label="Stage 2 (SICR)"
          />
          <Line
            type="monotone"
            dataKey="s3Count"
            name="Stage 3 (Default)"
            stroke={STAGE_COLORS.s3}
            strokeWidth={2}
            dot={{ r: 3 }}
            aria-label="Stage 3 (Default)"
          />
        </LineChart>
      </ResponsiveContainer>

      {/* Screen-reader summary */}
      <table className="sr-only" aria-label={`Data tabel: ${ariaLabel}`}>
        <caption>Data numerik tren perpindahan stage ECL</caption>
        <thead>
          <tr>
            <th scope="col">Periode</th>
            <th scope="col">Stage 1</th>
            <th scope="col">Stage 2</th>
            <th scope="col">Stage 3</th>
          </tr>
        </thead>
        <tbody>
          {data.map((d, i) => (
            <tr
              key={i}
              aria-label={`${d.periode}: Stage 1 = ${d.s1Count}, Stage 2 = ${d.s2Count}, Stage 3 = ${d.s3Count} instrumen`}
            >
              <td>{d.periode}</td>
              <td>{d.s1Count.toLocaleString("id-ID")}</td>
              <td>{d.s2Count.toLocaleString("id-ID")}</td>
              <td>{d.s3Count.toLocaleString("id-ID")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
