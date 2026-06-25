/**
 * MappingHistoryTable — RPT-21 audit log filter per event_code.
 * Shows: action, actor_role, event_time, before/after jsonb summary.
 * Read-only — no mutation buttons for ROLE-AUDIT.
 */

"use client";

import * as React from "react";
import { format } from "date-fns";
import { type ColumnDef } from "@tanstack/react-table";
import { JSONBTreeView } from "@/components/blips/JSONBTreeView";
import { DataTable, SortHeader } from "@/components/blips/DataTable";
import type { AuditLogEntry } from "@/lib/schemas/mapping-jurnal-p12.schema";
import type { SortingState } from "@tanstack/react-table";

interface MappingHistoryTableProps {
  data: AuditLogEntry[];
  isLoading: boolean;
  isError: boolean;
  sorting: SortingState;
  onSortingChange: (s: SortingState) => void;
  pagination: {
    pageIndex: number;
    hasMore: boolean;
    totalEstimate: number;
    limit: number;
    onNext: () => void;
    onPrev: () => void;
  };
  onExport?: (format: "csv" | "xlsx") => void;
  onRefresh?: () => void;
  lastUpdated?: Date;
}

function fmtDt(s: string): string {
  try {
    return format(new Date(s), "dd MMM yyyy HH:mm");
  } catch {
    return s;
  }
}

const ACTION_COLOR: Record<string, string> = {
  "MAPPING.SUBMITTED": "text-blue-700 bg-blue-50",
  "MAPPING.REVIEWED": "text-amber-700 bg-amber-50",
  "MAPPING.APPROVED_ACTIVE": "text-green-700 bg-green-50",
  "MAPPING.REJECTED": "text-red-700 bg-red-50",
  "MAPPING.VERSION_CREATED": "text-purple-700 bg-purple-50",
  "MAPPING.DETAIL_CREATED": "text-slate-700 bg-slate-50",
  "MAPPING.BULK_IMPORTED": "text-indigo-700 bg-indigo-50",
  "MAPPING.SOD_VIOLATION_ATTEMPT": "text-destructive bg-red-50",
};

export function MappingHistoryTable({
  data,
  isLoading,
  isError,
  sorting,
  onSortingChange,
  pagination,
  onExport,
  onRefresh,
  lastUpdated,
}: MappingHistoryTableProps) {
  const columns: ColumnDef<AuditLogEntry>[] = React.useMemo(
    () => [
      {
        id: "eventTime",
        header: ({ column }) => (
          <SortHeader column={column} label="Waktu" />
        ),
        accessorKey: "eventTime",
        cell: ({ row }) => (
          <span className="text-xs whitespace-nowrap">{fmtDt(row.original.eventTime)}</span>
        ),
      },
      {
        id: "action",
        header: "Aksi",
        accessorKey: "action",
        cell: ({ row }) => {
          const style = ACTION_COLOR[row.original.action] ?? "text-muted-foreground bg-muted/30";
          return (
            <span
              className={`inline-block rounded px-2 py-0.5 text-xs font-mono font-medium ${style}`}
              aria-label={`Aksi: ${row.original.action}`}
            >
              {row.original.action}
            </span>
          );
        },
      },
      {
        id: "actorRole",
        header: "Role",
        accessorKey: "actorRole",
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">{row.original.actorRole}</span>
        ),
      },
      {
        id: "entityId",
        header: "Entity ID",
        accessorKey: "entityId",
        cell: ({ row }) => (
          <code className="text-xs font-mono text-muted-foreground">
            {row.original.entityId.slice(0, 8)}...
          </code>
        ),
      },
      {
        id: "afterJsonb",
        header: "Setelah",
        cell: ({ row }) =>
          row.original.afterJsonb ? (
            <JSONBTreeView data={row.original.afterJsonb} maxDepth={2} />
          ) : (
            <span className="text-xs text-muted-foreground italic">—</span>
          ),
      },
    ],
    [],
  );

  return (
    <DataTable
      columns={columns}
      data={data}
      isLoading={isLoading}
      isError={isError}
      onRetry={() => onRefresh?.()}
      sorting={sorting}
      onSortingChange={onSortingChange}
      activeFilters={[]}
      onClearAllFilters={() => {}}
      searchValue=""
      onSearchChange={() => {}}
      onExport={onExport}
      exportFormats={["csv", "xlsx"]}
      pagination={{
        pageIndex: pagination.pageIndex,
        hasMore: pagination.hasMore,
        totalEstimate: pagination.totalEstimate,
        limit: pagination.limit,
        onNext: pagination.onNext,
        onPrev: pagination.onPrev,
      }}
      lastUpdated={lastUpdated}
      onRefresh={onRefresh}
      emptyMessage="Tidak ada riwayat audit untuk event ini."
    />
  );
}
