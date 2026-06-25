"use client";

/**
 * P5-M15 — AuditLogVolumeArea: AreaChart of audit event count per day (30 days).
 */

import * as React from "react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

export interface AuditLogDatum {
  tanggal: string;
  eventCount: number;
}

export interface AuditLogVolumeAreaProps {
  data: AuditLogDatum[];
  totalEvents: number;
  loading?: boolean;
  error?: string;
  className?: string;
}

export function AuditLogVolumeArea({
  data,
  totalEvents,
  loading = false,
  error,
  className,
}: AuditLogVolumeAreaProps) {
  const chartId = React.useId();

  if (loading) {
    return <Skeleton className={cn("h-52 w-full", className)} />;
  }

  if (error) {
    return <p className="text-sm text-destructive py-4 text-center">{error}</p>;
  }

  return (
    <div className={cn("space-y-2", className)}>
      <p className="text-xs text-muted-foreground">
        Total 30 hari:{" "}
        <span className="font-semibold text-foreground">
          {totalEvents.toLocaleString("id-ID")} events
        </span>
      </p>

      <ResponsiveContainer width="100%" height={200}>
        <AreaChart
          data={data}
          role="img"
          aria-labelledby={`${chartId}-title`}
          aria-describedby={`${chartId}-desc`}
        >
          <title id={`${chartId}-title`}>Volume Audit Log 30 Hari</title>
          <desc id={`${chartId}-desc`}>
            {`Area chart jumlah event audit log per hari selama 30 hari terakhir. Total: ${totalEvents.toLocaleString("id-ID")} events.`}
          </desc>
          <defs>
            <linearGradient id={`${chartId}-fill`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor="hsl(204, 86%, 53%)" stopOpacity={0.25} />
              <stop offset="95%" stopColor="hsl(204, 86%, 53%)" stopOpacity={0} />
            </linearGradient>
          </defs>
          <CartesianGrid strokeDasharray="3 3" className="stroke-border" />
          <XAxis dataKey="tanggal" tick={{ fontSize: 9 }} aria-hidden="true" />
          <YAxis tick={{ fontSize: 10 }} aria-hidden="true" />
          <Tooltip
            formatter={(value: number) => [
              value.toLocaleString("id-ID"),
              "Events",
            ]}
            labelFormatter={(label: string) => `Tanggal: ${label}`}
          />
          <Area
            type="monotone"
            dataKey="eventCount"
            name="Events"
            stroke="hsl(204, 86%, 53%)"
            fill={`url(#${chartId}-fill)`}
            strokeWidth={2}
          />
        </AreaChart>
      </ResponsiveContainer>

      <table className="sr-only" aria-label="Data tabel volume audit log">
        <caption>Jumlah event audit log per hari</caption>
        <thead>
          <tr>
            <th scope="col">Tanggal</th>
            <th scope="col">Jumlah Event</th>
          </tr>
        </thead>
        <tbody>
          {data.map((d, i) => (
            <tr key={i} aria-label={`Tanggal ${d.tanggal}: ${d.eventCount} events`}>
              <td>{d.tanggal}</td>
              <td>{d.eventCount.toLocaleString("id-ID")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
