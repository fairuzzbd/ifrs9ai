"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { glDeliveryStatusApi } from "@/lib/api/gl-delivery.api";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface HistoryEntry {
  eventTime: string;
  action: string;
  actorName: string;
  detail: string;
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface GlDeliveryHistoryTimelineProps {
  jurnalHeaderId: string;
  /** If true, raw jsonb expansion is allowed (ROLE-IT-ADMIN) */
  canViewRaw?: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S2-AC2 — collapsible delivery history)
// ---------------------------------------------------------------------------

export function GlDeliveryHistoryTimeline({
  jurnalHeaderId,
  canViewRaw = false,
  className,
}: GlDeliveryHistoryTimelineProps) {
  const [expanded, setExpanded] = React.useState(false);

  const { data, isLoading } = useQuery({
    queryKey: ["gl-delivery", "history", jurnalHeaderId],
    queryFn: () => glDeliveryStatusApi.getHistory(jurnalHeaderId),
    enabled: expanded,
    staleTime: 30_000,
  });

  const entries: HistoryEntry[] = data?.data ?? [];

  return (
    <div className={cn("border rounded-md", className)}>
      {/* Collapsible trigger */}
      <Button
        variant="ghost"
        size="sm"
        className="w-full flex items-center justify-start gap-2 px-3 py-2 h-auto text-sm font-medium"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
        aria-controls="gl-delivery-history-content"
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4" aria-hidden="true" />
        ) : (
          <ChevronRight className="h-4 w-4" aria-hidden="true" />
        )}
        {expanded
          ? "Sembunyikan Riwayat Delivery"
          : `Lihat Riwayat Delivery${entries.length > 0 ? ` (${entries.length} entri)` : ""}`}
      </Button>

      {/* Content */}
      {expanded && (
        <div
          id="gl-delivery-history-content"
          className="border-t px-3 py-3"
          role="log"
          aria-label="Riwayat pengiriman GL"
          aria-live="polite"
        >
          {isLoading ? (
            <div className="space-y-2">
              {Array.from({ length: 3 }).map((_, i) => (
                <div key={i} className="h-4 w-full animate-pulse rounded bg-muted" />
              ))}
            </div>
          ) : entries.length === 0 ? (
            <p className="text-xs text-muted-foreground italic">
              Belum ada riwayat delivery.
            </p>
          ) : (
            <ol className="space-y-2">
              {entries.map((entry, idx) => (
                <li key={idx} className="flex gap-3 text-xs">
                  <time
                    dateTime={entry.eventTime}
                    className="shrink-0 text-muted-foreground w-32"
                  >
                    {new Date(entry.eventTime).toLocaleString("id-ID", {
                      day: "2-digit",
                      month: "short",
                      year: "numeric",
                      hour: "2-digit",
                      minute: "2-digit",
                    })}
                  </time>
                  <div className="min-w-0">
                    <span className="font-mono font-medium">{entry.action}</span>
                    <span className="ml-2 text-muted-foreground">
                      {entry.actorName}
                    </span>
                    {entry.detail && (
                      <p className={cn("text-muted-foreground truncate", canViewRaw && "max-w-none")}>
                        {entry.detail}
                      </p>
                    )}
                  </div>
                </li>
              ))}
            </ol>
          )}
        </div>
      )}
    </div>
  );
}
