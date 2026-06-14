"use client";

import * as React from "react";
import { format, parseISO } from "date-fns";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface AuditEntry {
  eventId: string;
  eventTime: string;
  actorUsername: string;
  actorRole: string;
  action: string;
  beforeJsonb: Record<string, unknown> | null;
  afterJsonb: Record<string, unknown> | null;
  ip: string | null;
}

interface AuditHistoryTableProps {
  entries: AuditEntry[];
  isLoading?: boolean;
  isAuditRole?: boolean;
  hasMore?: boolean;
  onLoadMore?: () => void;
}

const ACTION_LABELS: Record<string, string> = {
  "MATA_UANG.CREATE": "Dibuat",
  "MATA_UANG.UPDATE": "Diperbarui",
  "MATA_UANG.DELETE": "Dihapus",
  "MATA_UANG.SUBMIT": "Disubmit",
  "MATA_UANG.RESUBMIT": "Disubmit Ulang",
  "MATA_UANG.REVIEW": "Disetujui (Review)",
  "MATA_UANG.APPROVE": "Disetujui (Final)",
  "MATA_UANG.REJECT": "Ditolak",
  "MATA_UANG.EXPORT": "Diekspor",
};

function JsonViewer({ data }: { data: Record<string, unknown> }) {
  return (
    <pre className="max-h-48 overflow-auto rounded bg-muted p-2 text-xs">
      {JSON.stringify(data, null, 2)}
    </pre>
  );
}

function AuditRow({
  entry,
  isAuditRole,
}: {
  entry: AuditEntry;
  isAuditRole: boolean;
}) {
  const [expanded, setExpanded] = React.useState(false);
  const hasChanges =
    isAuditRole && (entry.beforeJsonb || entry.afterJsonb);

  return (
    <>
      <tr className="border-b hover:bg-muted/40">
        <td className="px-4 py-3 text-sm text-muted-foreground">
          {format(parseISO(entry.eventTime), "dd MMM yyyy, HH:mm:ss")}
        </td>
        <td className="px-4 py-3">
          <span className="rounded-full bg-secondary px-2 py-0.5 text-xs font-medium">
            {ACTION_LABELS[entry.action] ?? entry.action}
          </span>
        </td>
        <td className="px-4 py-3 text-sm">
          <span className="font-medium">{entry.actorUsername}</span>
          <span className="ml-1 text-xs text-muted-foreground">
            ({entry.actorRole})
          </span>
        </td>
        <td className="px-4 py-3 text-sm text-muted-foreground">
          {hasChanges ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-auto px-2 py-0.5 text-xs"
              onClick={() => setExpanded((p) => !p)}
              aria-expanded={expanded}
            >
              {expanded ? (
                <ChevronDown className="mr-1 h-3 w-3" />
              ) : (
                <ChevronRight className="mr-1 h-3 w-3" />
              )}
              Lihat perubahan
            </Button>
          ) : (
            "—"
          )}
        </td>
      </tr>
      {expanded && hasChanges && (
        <tr className="border-b bg-muted/20">
          <td colSpan={4} className="px-4 py-3">
            <div className="grid gap-4 sm:grid-cols-2">
              {entry.beforeJsonb && (
                <div>
                  <p className="mb-1 text-xs font-medium text-muted-foreground">
                    Sebelum
                  </p>
                  <JsonViewer data={entry.beforeJsonb} />
                </div>
              )}
              {entry.afterJsonb && (
                <div>
                  <p className="mb-1 text-xs font-medium text-muted-foreground">
                    Setelah
                  </p>
                  <JsonViewer data={entry.afterJsonb} />
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  );
}

export function AuditHistoryTable({
  entries,
  isLoading = false,
  isAuditRole = false,
  hasMore = false,
  onLoadMore,
}: AuditHistoryTableProps) {
  return (
    <div className="space-y-4">
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm" aria-busy={isLoading}>
          <thead className="border-b bg-muted/50">
            <tr>
              <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                Waktu
              </th>
              <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                Aksi
              </th>
              <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                Dilakukan Oleh
              </th>
              <th className="px-4 py-3 text-left font-medium text-muted-foreground">
                Perubahan
              </th>
            </tr>
          </thead>
          <tbody>
            {isLoading
              ? Array.from({ length: 4 }).map((_, i) => (
                  <tr key={i} className="border-b">
                    {Array.from({ length: 4 }).map((__, j) => (
                      <td key={j} className="px-4 py-3">
                        <Skeleton className="h-4 w-full" />
                      </td>
                    ))}
                  </tr>
                ))
              : entries.length === 0
                ? (
                  <tr>
                    <td
                      colSpan={4}
                      className="px-4 py-8 text-center text-sm text-muted-foreground"
                    >
                      Tidak ada riwayat audit.
                    </td>
                  </tr>
                )
                : entries.map((entry) => (
                    <AuditRow
                      key={entry.eventId}
                      entry={entry}
                      isAuditRole={isAuditRole}
                    />
                  ))}
          </tbody>
        </table>
      </div>

      {hasMore && onLoadMore && (
        <div className="flex justify-center">
          <Button variant="outline" size="sm" onClick={onLoadMore}>
            Muat lebih banyak
          </Button>
        </div>
      )}
    </div>
  );
}
