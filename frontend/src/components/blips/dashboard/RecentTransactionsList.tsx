"use client";

/**
 * P5-M15 — RecentTransactionsList: lightweight read-only table widget.
 * No paging — for dashboard quick-view only.
 */

import * as React from "react";
import Link from "next/link";
import { type ColumnDef, flexRender, getCoreRowModel, useReactTable } from "@tanstack/react-table";
import { Skeleton } from "@/components/ui/skeleton";
import { WidgetEmpty } from "./WidgetCard";
import { cn } from "@/lib/utils";

export interface RecentTransactionsListProps<TData> {
  data: TData[];
  columns: ColumnDef<TData>[];
  maxRows?: number;
  linkToFull?: string;
  loading?: boolean;
  error?: string;
  emptyMessage?: string;
  ariaLabel?: string;
  className?: string;
}

export function RecentTransactionsList<TData>({
  data,
  columns,
  maxRows = 20,
  linkToFull,
  loading = false,
  error,
  emptyMessage = "Belum ada data dalam periode ini.",
  ariaLabel = "Daftar terbaru",
  className,
}: RecentTransactionsListProps<TData>) {
  const visible = data.slice(0, maxRows);

  const table = useReactTable({
    data: visible,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  if (loading) {
    return (
      <div className={cn("space-y-2", className)}>
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-7 w-full" />
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
        ctaLabel={linkToFull ? "Lihat semua →" : undefined}
        ctaHref={linkToFull}
      />
    );
  }

  return (
    <div className={cn("space-y-1", className)}>
      <div className="overflow-auto" role="region" aria-label={ariaLabel}>
        <table className="w-full text-xs">
          <thead>
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id} className="border-b">
                {hg.headers.map((header) => (
                  <th
                    key={header.id}
                    scope="col"
                    className="py-1 pr-3 text-left font-medium text-muted-foreground"
                  >
                    {flexRender(
                      header.column.columnDef.header,
                      header.getContext(),
                    )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr key={row.id} className="border-b last:border-0 hover:bg-muted/30">
                {row.getVisibleCells().map((cell) => (
                  <td
                    key={cell.id}
                    className="py-1 pr-3"
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
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
