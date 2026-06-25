"use client";

/**
 * P5-M15 — MaturityBucketBar: horizontal BarChart of exposure per maturity bucket.
 */

import * as React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Cell,
} from "recharts";
import { Skeleton } from "@/components/ui/skeleton";
import { formatIDRAbbrev, formatIDR } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface MaturityBucketDatum {
  bucket: "≤30d" | "31-90d" | "91-180d" | ">180d";
  nominalIdr: number;
  count?: number;
}

export interface MaturityBucketBarProps {
  data: MaturityBucketDatum[];
  loading?: boolean;
  error?: string;
  className?: string;
}

const BUCKET_COLORS = [
  "hsl(0, 72%, 51%)",   // ≤30d red — most urgent
  "hsl(38, 92%, 50%)",  // 31-90d amber
  "hsl(204, 86%, 53%)", // 91-180d blue
  "hsl(142, 76%, 36%)", // >180d green
];

export function MaturityBucketBar({
  data,
  loading = false,
  error,
  className,
}: MaturityBucketBarProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-40 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive py-4 text-center">{error}</p>;
  }

  return (
    <div className={cn("", className)}>
      <ResponsiveContainer width="100%" height={180}>
        <BarChart
          data={data}
          layout="vertical"
          role="img"
          aria-labelledby={`${chartId}-title`}
        >
          <title id={`${chartId}-title`}>Jatuh Tempo Mendatang by Bucket</title>
          <CartesianGrid strokeDasharray="3 3" horizontal={false} className="stroke-border" />
          <XAxis
            type="number"
            tickFormatter={(v: number) => formatIDRAbbrev(v)}
            tick={{ fontSize: 10 }}
            aria-hidden="true"
          />
          <YAxis
            type="category"
            dataKey="bucket"
            width={60}
            tick={{ fontSize: 11 }}
            aria-hidden="true"
          />
          <Tooltip
            formatter={(value: number, _name: string, props: { payload?: MaturityBucketDatum }) => {
              const count = props.payload?.count;
              return [
                `${formatIDR(value)}${count != null ? ` — ${count} instrumen` : ""}`,
                "Nominal IDR",
              ];
            }}
          />
          <Bar dataKey="nominalIdr" name="Nominal IDR" radius={[0, 4, 4, 0]}>
            {data.map((_, idx) => (
              <Cell
                key={idx}
                fill={BUCKET_COLORS[idx] ?? BUCKET_COLORS[BUCKET_COLORS.length - 1]}
                aria-label={`${data[idx].bucket}: ${formatIDRAbbrev(data[idx].nominalIdr)}`}
              />
            ))}
          </Bar>
        </BarChart>
      </ResponsiveContainer>

      <table className="sr-only" aria-label="Data tabel jatuh tempo mendatang">
        <caption>Data nominal instrumen per bucket jatuh tempo</caption>
        <thead>
          <tr>
            <th scope="col">Bucket</th>
            <th scope="col">Nominal IDR</th>
          </tr>
        </thead>
        <tbody>
          {data.map((d, i) => (
            <tr key={i}>
              <td>{d.bucket}</td>
              <td>{formatIDR(d.nominalIdr)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
