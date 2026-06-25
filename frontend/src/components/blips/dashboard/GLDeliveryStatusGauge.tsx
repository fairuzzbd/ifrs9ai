"use client";

/**
 * P5-M15 — GLDeliveryStatusGauge: donut + KPI for GL delivery success rate.
 */

import * as React from "react";
import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from "recharts";
import { AlertTriangle } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { formatPct } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface GLDeliveryStatusGaugeProps {
  delivered: number;
  failed: number;
  pending: number;
  loading?: boolean;
  error?: string;
  alertThresholdPct?: number;
  className?: string;
}

const GL_COLORS = {
  DELIVERED: "hsl(142, 76%, 36%)",
  FAILED: "hsl(0, 72%, 51%)",
  PENDING: "hsl(38, 92%, 50%)",
};

export function GLDeliveryStatusGauge({
  delivered,
  failed,
  pending,
  loading = false,
  error,
  alertThresholdPct = 5,
  className,
}: GLDeliveryStatusGaugeProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-40 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive">{error}</p>;
  }

  const total = delivered + failed + pending;
  const successRate = total > 0 ? (delivered / total) * 100 : 0;
  const failedRate = total > 0 ? (failed / total) * 100 : 0;
  const showWarning = failedRate > alertThresholdPct;

  const pieData = [
    { name: "Terkirim", value: delivered, key: "DELIVERED" },
    { name: "Gagal", value: failed, key: "FAILED" },
    { name: "Menunggu", value: pending, key: "PENDING" },
  ];

  return (
    <div className={cn("space-y-2", className)}>
      {showWarning && (
        <div
          role="alert"
          className="flex items-center gap-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-xs text-amber-800"
        >
          <AlertTriangle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
          <span>
            Peringatan: Tingkat kegagalan GL delivery melebihi {alertThresholdPct}%
          </span>
        </div>
      )}

      <div className="flex items-center gap-4">
        <ResponsiveContainer width={120} height={120}>
          <PieChart
            role="img"
            aria-labelledby={`${chartId}-title`}
          >
            <title id={`${chartId}-title`}>Status GL Delivery</title>
            <Pie
              data={pieData}
              cx="50%"
              cy="50%"
              innerRadius={35}
              outerRadius={55}
              paddingAngle={1}
              dataKey="value"
              aria-hidden="true"
            >
              {pieData.map((entry) => (
                <Cell
                  key={entry.key}
                  fill={GL_COLORS[entry.key as keyof typeof GL_COLORS]}
                  aria-label={`${entry.name}: ${entry.value}`}
                />
              ))}
            </Pie>
            <Tooltip
              formatter={(value: number, name: string) => [
                `${value.toLocaleString("id-ID")}`,
                name,
              ]}
            />
          </PieChart>
        </ResponsiveContainer>

        <div className="flex-1 space-y-1">
          <p
            className="text-2xl font-bold text-green-700"
            role="status"
            aria-live="polite"
            aria-label={`Success Rate ${formatPct(successRate)} — ${delivered.toLocaleString("id-ID")} dari ${total.toLocaleString("id-ID")} jurnal berhasil dikirim ke GL`}
          >
            {formatPct(successRate)}
          </p>
          <p className="text-xs text-muted-foreground">
            {delivered.toLocaleString("id-ID")} dari {total.toLocaleString("id-ID")} berhasil
          </p>

          <div className="space-y-0.5 text-xs">
            {pieData.map((d) => (
              <div key={d.key} className="flex items-center gap-1">
                <span
                  className="inline-block h-2 w-2 rounded-full"
                  style={{ backgroundColor: GL_COLORS[d.key as keyof typeof GL_COLORS] }}
                  aria-hidden="true"
                />
                <span className="text-muted-foreground">{d.name}:</span>
                <span>{d.value.toLocaleString("id-ID")}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <table className="sr-only" aria-label="Data tabel status GL delivery">
        <caption>Data numerik GL delivery status</caption>
        <thead>
          <tr>
            <th scope="col">Status</th>
            <th scope="col">Jumlah</th>
            <th scope="col">Persentase</th>
          </tr>
        </thead>
        <tbody>
          {pieData.map((d) => (
            <tr key={d.key}>
              <td>{d.name}</td>
              <td>{d.value.toLocaleString("id-ID")}</td>
              <td>{total > 0 ? formatPct((d.value / total) * 100) : "0%"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
