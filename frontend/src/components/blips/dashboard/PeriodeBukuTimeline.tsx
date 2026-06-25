"use client";

/**
 * P5-M15 — PeriodeBukuTimeline: horizontal BarChart for 12 periode status.
 */

import * as React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Cell,
  Tooltip,
  ResponsiveContainer,
  LabelList,
} from "recharts";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export interface PeriodeBukuDatum {
  kode: string;
  status: "OPEN" | "SOFT_CLOSED" | "HARD_CLOSED";
  tanggalClose?: string;
}

export interface PeriodeBukuTimelineProps {
  data: PeriodeBukuDatum[];
  currentPeriodeKode?: string;
  loading?: boolean;
  error?: string;
  className?: string;
}

const STATUS_COLORS: Record<string, string> = {
  OPEN: "hsl(142, 76%, 36%)",
  SOFT_CLOSED: "hsl(38, 92%, 50%)",
  HARD_CLOSED: "hsl(220, 9%, 60%)",
};

const STATUS_LABELS: Record<string, string> = {
  OPEN: "OPEN",
  SOFT_CLOSED: "SOFT CLOSED",
  HARD_CLOSED: "HARD CLOSED",
};

export function PeriodeBukuTimeline({
  data,
  currentPeriodeKode,
  loading = false,
  error,
  className,
}: PeriodeBukuTimelineProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-52 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive py-4 text-center">{error}</p>;
  }

  // All bars same value (1) just for visual; color encodes status
  const chartData = data.map((d) => ({
    ...d,
    value: 1,
    label: `${d.kode} — ${STATUS_LABELS[d.status]}`,
  }));

  return (
    <div className={cn("", className)}>
      <ResponsiveContainer width="100%" height={Math.max(200, data.length * 28)}>
        <BarChart
          data={chartData}
          layout="vertical"
          role="img"
          aria-labelledby={`${chartId}-title`}
          margin={{ left: 80, right: 20 }}
        >
          <title id={`${chartId}-title`}>Timeline Periode Buku</title>
          <XAxis type="number" hide />
          <YAxis
            type="category"
            dataKey="kode"
            width={75}
            tick={({ x, y, payload }: { x: number; y: number; payload: { value: string } }) => (
              <g transform={`translate(${x},${y})`}>
                <text
                  x={-2}
                  y={0}
                  dy={4}
                  textAnchor="end"
                  fontSize={10}
                  fill={payload.value === currentPeriodeKode ? "hsl(204, 86%, 53%)" : "currentColor"}
                  fontWeight={payload.value === currentPeriodeKode ? "bold" : "normal"}
                >
                  {payload.value}
                </text>
              </g>
            )}
            aria-hidden="true"
          />
          <Tooltip
            formatter={(_v: number, _n: string, props: { payload?: { status: string; tanggalClose?: string } }) => {
              const p = props.payload;
              if (!p) return ["", ""];
              return [
                `${STATUS_LABELS[p.status]}${p.tanggalClose ? ` — Ditutup: ${p.tanggalClose}` : ""}`,
                "Status",
              ];
            }}
          />
          <Bar dataKey="value" radius={[0, 4, 4, 0]}>
            {chartData.map((entry, idx) => (
              <Cell
                key={idx}
                fill={STATUS_COLORS[entry.status]}
                aria-label={`${entry.kode} — ${STATUS_LABELS[entry.status]}`}
              />
            ))}
            <LabelList
              dataKey="status"
              position="insideRight"
              formatter={(v: string) => STATUS_LABELS[v] ?? v}
              style={{ fontSize: 9, fill: "white" }}
            />
          </Bar>
        </BarChart>
      </ResponsiveContainer>

      {/* CURRENT badge */}
      {currentPeriodeKode && (
        <div className="mt-1 flex items-center gap-2">
          <Badge variant="default" className="text-xs bg-blue-600">
            CURRENT: {currentPeriodeKode}
          </Badge>
        </div>
      )}

      <table className="sr-only" aria-label="Data tabel timeline periode buku">
        <caption>Status 12 periode buku terakhir</caption>
        <thead>
          <tr>
            <th scope="col">Periode</th>
            <th scope="col">Status</th>
            <th scope="col">Tanggal Tutup</th>
          </tr>
        </thead>
        <tbody>
          {data.map((d) => (
            <tr key={d.kode}>
              <td>{d.kode}</td>
              <td>{STATUS_LABELS[d.status]}</td>
              <td>{d.tanggalClose ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
