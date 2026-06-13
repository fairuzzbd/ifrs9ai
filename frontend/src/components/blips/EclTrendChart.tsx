"use client";

import * as React from "react";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Legend,
} from "recharts";
import type { PieLabelRenderProps } from "recharts";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDRCompact(value: number): string {
  if (value >= 1_000_000_000) return `Rp ${(value / 1_000_000_000).toFixed(1)}M`;
  if (value >= 1_000_000) return `Rp ${(value / 1_000_000).toFixed(1)}Jt`;
  return `Rp ${value.toLocaleString("id-ID")}`;
}

// ---------------------------------------------------------------------------
// ECL per Stage BarChart
// ---------------------------------------------------------------------------

export interface EclStageChartData {
  stage: string;
  eclIdr: number;
  fill: string;
}

export interface EclStageBarChartProps {
  data: EclStageChartData[];
  className?: string;
}

export function EclStageBarChart({ data, className }: EclStageBarChartProps) {
  return (
    <div className={cn("w-full h-56", className)}>
      <ResponsiveContainer width="100%" height="100%">
        <BarChart data={data} margin={{ top: 8, right: 16, left: 8, bottom: 8 }}>
          <XAxis dataKey="stage" tick={{ fontSize: 12 }} />
          <YAxis
            tickFormatter={formatIDRCompact}
            tick={{ fontSize: 10 }}
            width={80}
          />
          <Tooltip
            formatter={(value) => {
              const num = typeof value === "number" ? value : Number(value);
              return new Intl.NumberFormat("id-ID", {
                style: "currency",
                currency: "IDR",
                minimumFractionDigits: 4,
              }).format(num);
            }}
            labelFormatter={(label) => String(label)}
          />
          {data.map((entry, index) => (
            <Bar key={index} dataKey="eclIdr" name={entry.stage} fill={entry.fill} />
          ))}
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Routing Distribution PieChart
// ---------------------------------------------------------------------------

export interface RoutingPieData {
  name: string;
  value: number;
  fill: string;
}

export interface RoutingPieChartProps {
  data: RoutingPieData[];
  className?: string;
}

const RADIAN = Math.PI / 180;

function CustomLabel(props: PieLabelRenderProps) {
  const { cx, cy, midAngle, innerRadius, outerRadius, percent } = props;
  if (
    typeof cx !== "number" ||
    typeof cy !== "number" ||
    typeof midAngle !== "number" ||
    typeof innerRadius !== "number" ||
    typeof outerRadius !== "number" ||
    typeof percent !== "number"
  ) {
    return null;
  }
  if (percent < 0.05) return null;

  const radius = innerRadius + (outerRadius - innerRadius) * 0.5;
  const x = cx + radius * Math.cos(-midAngle * RADIAN);
  const y = cy + radius * Math.sin(-midAngle * RADIAN);

  return (
    <text x={x} y={y} fill="white" textAnchor="middle" dominantBaseline="central" fontSize={11}>
      {`${(percent * 100).toFixed(0)}%`}
    </text>
  );
}

export function RoutingPieChart({ data, className }: RoutingPieChartProps) {
  return (
    <div className={cn("w-full h-56", className)}>
      <ResponsiveContainer width="100%" height="100%">
        <PieChart>
          <Pie
            data={data}
            cx="50%"
            cy="50%"
            outerRadius={80}
            dataKey="value"
            labelLine={false}
            label={CustomLabel}
          >
            {data.map((entry, index) => (
              <Cell key={`cell-${index}`} fill={entry.fill} />
            ))}
          </Pie>
          <Legend
            layout="vertical"
            align="right"
            verticalAlign="middle"
            iconSize={10}
            formatter={(value: string) => <span className="text-xs">{value}</span>}
          />
          <Tooltip
            formatter={(value) => {
              const num = typeof value === "number" ? value : Number(value);
              return [num.toLocaleString("id-ID"), "Instrumen"];
            }}
          />
        </PieChart>
      </ResponsiveContainer>
    </div>
  );
}
