"use client";

/**
 * P5-M15 — DashboardShell: common layout wrapper for all dashboard pages.
 * Provides: top bar with "Terakhir diperbarui" + refresh-all button, grid container.
 */

import * as React from "react";
import { RefreshCw, type LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { useInvalidateAllDashboardData } from "@/lib/hooks/useReportData";
import { formatLastUpdated } from "@/lib/format";
import { cn } from "@/lib/utils";

export interface DashboardShellProps {
  title: string;
  subtitle?: string;
  icon?: LucideIcon;
  badge?: React.ReactNode;
  periodeLabel?: string;
  dashboardLabel?: string;
  children: React.ReactNode;
  className?: string;
}

export function DashboardShell({
  title,
  subtitle,
  icon: Icon,
  badge,
  periodeLabel,
  dashboardLabel = "Dashboard",
  children,
  className,
}: DashboardShellProps) {
  const invalidateAll = useInvalidateAllDashboardData();
  const [isRefreshing, setIsRefreshing] = React.useState(false);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  const handleRefresh = async () => {
    setIsRefreshing(true);
    invalidateAll();
    // Give TanStack Query time to kick off
    await new Promise<void>((resolve) => setTimeout(resolve, 500));
    setLastUpdated(new Date());
    setIsRefreshing(false);
  };

  return (
    <div className={cn("flex flex-col gap-6 p-6", className)}>
      {/* Page header */}
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            {Icon && <Icon className="h-5 w-5 text-muted-foreground" aria-hidden="true" />}
            <h1 className="text-xl font-semibold">{title}</h1>
            {badge}
          </div>
          {subtitle && (
            <p className="text-sm text-muted-foreground">{subtitle}</p>
          )}
          {periodeLabel && (
            <p className="text-xs text-muted-foreground">{periodeLabel}</p>
          )}
        </div>

        <div className="flex items-center gap-2 flex-shrink-0">
          <span className="text-xs text-muted-foreground hidden sm:inline">
            Terakhir diperbarui: {formatLastUpdated(lastUpdated)}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void handleRefresh()}
            disabled={isRefreshing}
            aria-label={`Perbarui semua data ${dashboardLabel}`}
          >
            <RefreshCw
              className={cn("h-4 w-4 mr-1", isRefreshing && "animate-spin")}
              aria-hidden="true"
            />
            Perbarui
          </Button>
        </div>
      </div>

      {/* Grid content */}
      <div className="grid grid-cols-12 gap-4">
        {children}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Helper component for dashboard grid columns
// ---------------------------------------------------------------------------

export interface GridColProps {
  span?: 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | 10 | 11 | 12;
  children: React.ReactNode;
  className?: string;
}

const COL_SPAN_MAP: Record<number, string> = {
  1: "col-span-1",
  2: "col-span-2",
  3: "col-span-3",
  4: "col-span-4",
  5: "col-span-5",
  6: "col-span-6",
  7: "col-span-7",
  8: "col-span-8",
  9: "col-span-9",
  10: "col-span-10",
  11: "col-span-11",
  12: "col-span-12",
};

export function GridCol({ span = 12, children, className }: GridColProps) {
  return (
    <div className={cn(COL_SPAN_MAP[span], className)}>
      {children}
    </div>
  );
}
