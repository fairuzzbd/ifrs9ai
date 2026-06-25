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
  ReferenceDot,
} from "recharts";
import { useQuery } from "@tanstack/react-query";
import { format, subDays } from "date-fns";
import { mtmListApi, mtmQueryKeys } from "@/lib/api/mtm.api";
import { Skeleton } from "@/components/ui/skeleton";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatYAxisIDR(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(1)} M`;
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)} Jt`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(0)} rb`;
  return value.toFixed(0);
}

function formatXAxisDate(dateStr: string): string {
  return format(new Date(dateStr), "dd MMM");
}

// ---------------------------------------------------------------------------
// Custom tooltip
// ---------------------------------------------------------------------------

interface TooltipPayloadEntry {
  payload?: {
    tanggalMtm: string;
    hargaPasarIdr: number;
    hargaSumber: string;
    deltaPct: number;
  };
}

function CustomTooltip({ active, payload }: { active?: boolean; payload?: TooltipPayloadEntry[] }) {
  if (!active || !payload?.length) return null;
  const d = payload[0]?.payload;
  if (!d) return null;

  const idr = new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(d.hargaPasarIdr);

  const deltaFormatted = `${d.deltaPct >= 0 ? "+" : ""}${d.deltaPct.toFixed(2)}%`;

  return (
    <div className="rounded-md border bg-background px-3 py-2 text-xs shadow-md">
      <p className="font-semibold">{d.tanggalMtm}</p>
      <p>{idr}</p>
      <p className={d.deltaPct >= 0 ? "text-green-700" : "text-red-700"}>{deltaFormatted}</p>
      <p className="text-muted-foreground">{d.hargaSumber}</p>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface MtmPriceHistoryChartProps {
  instrumenId: string;
  instrumenKode: string;
  /** The current MTM date — shown with a special red diamond marker */
  tanggalMtm: string;
}

// ---------------------------------------------------------------------------
// Component — Recharts LineChart, last 30 days (design spec §9.10)
// ---------------------------------------------------------------------------

export function MtmPriceHistoryChart({
  instrumenId,
  instrumenKode,
  tanggalMtm,
}: MtmPriceHistoryChartProps) {
  const today = new Date(tanggalMtm);
  const thirtyDaysAgo = subDays(today, 30);
  const dateFrom = format(thirtyDaysAgo, "yyyy-MM-dd");

  const { data, isLoading, isError } = useQuery({
    queryKey: mtmQueryKeys.priceHistory(instrumenId),
    queryFn: () =>
      mtmListApi.list({
        "filter[instrumen_id]": instrumenId,
        "filter[tanggal_mtm]": `between:${dateFrom},${tanggalMtm}`,
        sort: "tanggal_mtm:asc",
        limit: 30,
      }),
    staleTime: 60_000,
  });

  if (isLoading) {
    return <Skeleton className="h-[200px] w-full rounded-md" />;
  }

  if (isError) {
    // Fail silently per design spec §11.5
    return null;
  }

  const chartData = data?.data ?? [];

  if (chartData.length === 0) {
    return (
      <p className="text-sm text-muted-foreground italic">
        Tidak ada riwayat harga untuk 30 hari terakhir.
      </p>
    );
  }

  // Current day data point for special marker
  const todayPoint = chartData.find((d) => d.tanggalMtm === tanggalMtm);

  // Visually-hidden summary table for accessibility
  const hiddenTableId = `price-history-table-${instrumenId}`;

  return (
    <div>
      <div
        role="img"
        aria-label={`Grafik riwayat harga ${instrumenKode} 30 hari terakhir`}
        aria-describedby={hiddenTableId}
        className="h-[200px] w-full"
      >
        <ResponsiveContainer width="100%" height="100%">
          <LineChart
            data={chartData}
            margin={{ top: 4, right: 8, left: 8, bottom: 4 }}
          >
            <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
            <XAxis
              dataKey="tanggalMtm"
              tickFormatter={formatXAxisDate}
              tick={{ fontSize: 10 }}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              tickFormatter={formatYAxisIDR}
              tick={{ fontSize: 10 }}
              tickLine={false}
              axisLine={false}
              width={56}
            />
            <Tooltip content={<CustomTooltip />} />
            <Line
              type="monotone"
              dataKey="hargaPasarIdr"
              stroke="hsl(var(--primary))"
              strokeWidth={1.5}
              dot={false}
              activeDot={{ r: 4 }}
            />
            {/* Special marker for today's MTM point */}
            {todayPoint && (
              <ReferenceDot
                x={tanggalMtm}
                y={todayPoint.hargaPasarIdr}
                r={6}
                fill="hsl(var(--destructive))"
                stroke="white"
                strokeWidth={2}
                label={undefined}
              />
            )}
          </LineChart>
        </ResponsiveContainer>
      </div>

      {/* Visually hidden summary table for screen readers */}
      <table id={hiddenTableId} className="sr-only">
        <caption>Riwayat harga MTM {instrumenKode} — 30 hari terakhir</caption>
        <thead>
          <tr>
            <th>Tanggal</th>
            <th>Harga Pasar (IDR)</th>
            <th>Delta %</th>
            <th>Sumber</th>
          </tr>
        </thead>
        <tbody>
          {chartData.map((d) => (
            <tr key={d.id}>
              <td>{d.tanggalMtm}</td>
              <td>
                {new Intl.NumberFormat("id-ID", { style: "currency", currency: "IDR" }).format(d.hargaPasarIdr)}
              </td>
              <td>{`${d.deltaPct >= 0 ? "+" : ""}${d.deltaPct.toFixed(2)}%`}</td>
              <td>{d.hargaSumber}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
