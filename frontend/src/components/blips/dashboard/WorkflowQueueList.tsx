"use client";

/**
 * P5-M15 — WorkflowQueueList: lightweight DataTable for pending workflow items.
 */

import * as React from "react";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { WidgetEmpty } from "./WidgetCard";
import { formatDateTime } from "@/lib/format";
import type { Rpt26Row } from "@/lib/schemas/dashboard.schema";

export interface WorkflowQueueListProps {
  data: Rpt26Row[];
  maxRows?: number;
  /** Render "Setujui" button — only if caller has jurnal.approve */
  showApproveButton?: boolean;
  loading?: boolean;
  error?: string;
  emptyMessage?: string;
  linkToFull?: string;
  dashboardLabel?: string;
}

export function WorkflowQueueList({
  data,
  maxRows = 20,
  showApproveButton = false,
  loading = false,
  error,
  emptyMessage = "Tidak ada dokumen yang menunggu approval saat ini.",
  linkToFull,
  dashboardLabel = "Dashboard",
}: WorkflowQueueListProps) {
  const visible = data.slice(0, maxRows);

  if (loading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    );
  }

  if (error) {
    return <p className="text-sm text-destructive">{error}</p>;
  }

  if (visible.length === 0) {
    return (
      <WidgetEmpty
        message={emptyMessage}
        ctaLabel={linkToFull ? "Lihat semua laporan →" : undefined}
        ctaHref={linkToFull}
      />
    );
  }

  return (
    <div className="space-y-1">
      <div
        className="overflow-auto"
        role="region"
        aria-label={`Antrian Workflow Pending — BLIPS ${dashboardLabel}`}
      >
        <table className="w-full text-xs">
          <thead>
            <tr className="border-b">
              <th scope="col" className="py-1 pr-2 text-left font-medium text-muted-foreground">
                ID Entitas
              </th>
              <th scope="col" className="py-1 pr-2 text-left font-medium text-muted-foreground">
                Tipe
              </th>
              <th scope="col" className="py-1 pr-2 text-left font-medium text-muted-foreground">
                Diajukan
              </th>
              <th scope="col" className="py-1 text-left font-medium text-muted-foreground">
                Aksi
              </th>
            </tr>
          </thead>
          <tbody>
            {visible.map((item) => {
              const href = `/master/instrumen/${item.entity_id}`;
              const approveHref = `/mapping-jurnal/approve/${item.jurnal_id ?? item.entity_id}`;
              return (
                <tr key={item.workflow_id} className="border-b last:border-0">
                  <td className="py-1 pr-2 font-mono">
                    {item.kode_entitas ?? item.entity_id.slice(0, 8)}
                  </td>
                  <td className="py-1 pr-2">
                    <Badge variant="outline" className="text-xs">
                      {item.entity_type}
                    </Badge>
                  </td>
                  <td className="py-1 pr-2 text-muted-foreground">
                    {formatDateTime(item.submitted_at)}
                  </td>
                  <td className="py-1 flex items-center gap-1">
                    <Link
                      href={href}
                      className="text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      aria-label={`Lihat detail workflow ${item.kode_entitas ?? item.entity_id}`}
                    >
                      Lihat →
                    </Link>
                    {showApproveButton && item.jurnal_id && (
                      <Link href={approveHref}>
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-5 px-2 text-xs ml-1"
                          aria-label={`Setujui jurnal ${item.jurnal_id}`}
                        >
                          Setujui
                        </Button>
                      </Link>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {linkToFull && (
        <div className="pt-1 text-right">
          <Link
            href={linkToFull}
            className="text-xs text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Lihat semua →
          </Link>
        </div>
      )}
    </div>
  );
}
