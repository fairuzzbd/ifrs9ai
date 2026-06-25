"use client";

/**
 * P5-M15 — ECLRollForwardLine: AreaChart cumulative ECL movement MTD/YTD.
 */

import * as React from "react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  Legend,
  ReferenceLine,
  ResponsiveContainer,
} from "recharts";
import { Skeleton } from "@/components/ui/skeleton";
import { formatIDRAbbrev, formatIDR } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface RollForwardDatum {
  tanggal: string;
  mtdCumulative: number;
  ytdCumulative: number;
}

export interface ECLRollForwardLineProps {
  data: RollForwardDatum[];
  mode?: "mtd" | "ytd" | "both";
  loading?: boolean;
  error?: string;
  className?: string;
}

export function ECLRollForwardLine({
  data,
  mode = "both",
  loading = false,
  error,
  className,
}: ECLRollForwardLineProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-52 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive py-4 text-center">{error}</p>;
  }

  const showMtd = mode === "mtd" || mode === "both";
  const showYtd = mode === "ytd" || mode === "both";

  return (
    <div className={cn("", className)}>
      <ResponsiveContainer width="100%" height={220}>
        <AreaChart
          data={data}
          role="img"
          aria-labelledby={`${chartId}-title`}
          aria-describedby={`${chartId}-desc`}
        >
          <title id={`${chartId}-title`}>Dampak P&L ECL — MTD & YTD</title>
          <desc id={`${chartId}-desc`}>
            Area chart pergerakan kumulatif ECL terhadap P&L. Nilai positif = penambahan impairment, negatif = reversal.
          </desc>
          <defs>
            <linearGradient id={`${chartId}-mtd`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="hsl(204, 86%, 53%)" stopOpacity={0.2} />
              <stop offset="95%" stopColor="hsl(204, 86%, 53%)" stopOpacity={0} />
            </linearGradient>
            <linearGradient id={`${chartId}-ytd`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="hsl(142, 76%, 36%)" stopOpacity={0.2} />
              <stop offset="95%" stopColor="hsl(142, 76%, 36%)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis dataKey="tanggal" tick={{ fontSize: 10 }} aria-hidden="true" />
          <YAxis
            tickFormatter={(v: number) => formatIDRAbbrev(v)}
            tick={{ fontSize: 10 }}
            aria-hidden="true"
          />
          <ReferenceLine y={0} stroke="hsl(0, 0%, 50%)" strokeDasharray="4 2" />
          <Tooltip
            formatter={(value: number) => [formatIDR(value), ""]}
            labelFormatter={(label: string) => `Tanggal: ${label}`}
          />
          <Legend formatter={(v: string) => <span className="text-xs">{v}</span>} />

          {showMtd && (
            <Area
              type="monotone"
              dataKey="mtdCumulative"
              name="MTD kumulatif"
              stroke="hsl(204, 86%, 53%)"
              fill={`url(#${chartId}-mtd)`}
              strokeWidth={2}
            />
          )}
          {showYtd && (
            <Area
              type="monotone"
              dataKey="ytdCumulative"
              name="YTD kumulatif"
              stroke="hsl(142, 76%, 36%)"
              fill={`url(#${chartId}-ytd)`}
              strokeWidth={2}
            />
          )}
        </AreaChart>
      </ResponsiveContainer>

      <table className="sr-only" aria-label="Data tabel pergerakan ECL MTD/YTD">
        <caption>Data kumulatif pergerakan ECL</caption>
        <thead>
          <tr>
            <th scope="col">Tanggal</th>
            {showMtd && <th scope="col">MTD Kumulatif</th>}
            {showYtd && <th scope="col">YTD Kumulatif</th>}
          </tr>
        </thead>
        <tbody>
          {data.map((d, i) => (
            <tr key={i}>
              <td>{d.tanggal}</td>
              {showMtd && <td>{formatIDR(d.mtdCumulative)}</td>}
              {showYtd && <td>{formatIDR(d.ytdCumulative)}</td>}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
