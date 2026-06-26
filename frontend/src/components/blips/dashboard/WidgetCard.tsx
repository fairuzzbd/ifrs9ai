"use client";

/**
 * P5-M15 — WidgetCard: common wrapper for all dashboard widgets.
 * Provides: loading skeleton, error state, empty state, title + tooltip slot.
 */

import * as React from "react";
import { AlertCircle, RefreshCw } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  TooltipProvider,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";

export interface WidgetCardProps {
  title: string;
  tooltip?: string;
  children?: React.ReactNode;
  isLoading?: boolean;
  isError?: boolean;
  errorMessage?: string;
  traceId?: string;
  onRetry?: () => void;
  headerAction?: React.ReactNode;
  className?: string;
  /** aria-label for screen readers */
  ariaLabel?: string;
  /** Dashboard context appended to aria-label */
  dashboardLabel?: string;
}

export function WidgetCard({
  title,
  tooltip,
  children,
  isLoading = false,
  isError = false,
  errorMessage,
  traceId,
  onRetry,
  headerAction,
  className,
  ariaLabel,
  dashboardLabel,
}: WidgetCardProps) {
  const computedAriaLabel =
    ariaLabel ?? (dashboardLabel ? `${title} — BLIPS ${dashboardLabel}` : title);

  return (
    <Card
      className={cn("flex flex-col h-full", className)}
      aria-label={computedAriaLabel}
    >
      <CardHeader className="flex flex-row items-center justify-between pb-2 space-y-0">
        <div className="flex items-center gap-2">
          <CardTitle className="text-sm font-medium">{title}</CardTitle>
          {tooltip && (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className="text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    aria-label={`Info: ${title}`}
                  >
                    <svg
                      xmlns="http://www.w3.org/2000/svg"
                      width="14"
                      height="14"
                      viewBox="0 0 24 24"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      aria-hidden="true"
                    >
                      <circle cx="12" cy="12" r="10" />
                      <line x1="12" y1="16" x2="12" y2="12" />
                      <line x1="12" y1="8" x2="12.01" y2="8" />
                    </svg>
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top" className="max-w-xs text-xs">
                  {tooltip}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}
        </div>
        {headerAction && <div className="flex items-center gap-2">{headerAction}</div>}
      </CardHeader>

      <CardContent className="flex-1">
        {isLoading && <WidgetSkeleton />}

        {!isLoading && isError && (
          <div
            role="alert"
            className="flex flex-col items-center justify-center gap-3 py-8 text-center"
          >
            <AlertCircle className="h-8 w-8 text-destructive" aria-hidden="true" />
            <p className="text-sm text-muted-foreground">
              {errorMessage ?? "Gagal memuat data."}
            </p>
            {traceId && (
              <p className="text-xs text-muted-foreground font-mono">
                trace: {traceId.slice(0, 12)}
              </p>
            )}
            {onRetry && (
              <Button
                variant="outline"
                size="sm"
                onClick={onRetry}
                className="mt-1"
              >
                <RefreshCw className="mr-2 h-3 w-3" aria-hidden="true" />
                Coba lagi
              </Button>
            )}
          </div>
        )}

        {!isLoading && !isError && children}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Loading skeleton — mimics a generic chart/data widget shape
// ---------------------------------------------------------------------------

function WidgetSkeleton() {
  return (
    <div className="space-y-3 pt-2" aria-hidden="true">
      <Skeleton className="h-4 w-3/4" />
      <Skeleton className="h-32 w-full" />
      <div className="flex gap-2">
        <Skeleton className="h-3 w-1/4" />
        <Skeleton className="h-3 w-1/4" />
        <Skeleton className="h-3 w-1/4" />
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Empty state helper
// ---------------------------------------------------------------------------

export function WidgetEmpty({
  message,
  ctaLabel,
  ctaHref,
}: {
  message: string;
  ctaLabel?: string;
  ctaHref?: string;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-8 text-center">
      <div
        className="flex h-12 w-12 items-center justify-center rounded-full bg-muted"
        aria-hidden="true"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          width="24"
          height="24"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          className="text-muted-foreground"
        >
          <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
        </svg>
      </div>
      <p className="text-sm text-muted-foreground">{message}</p>
      {ctaLabel && ctaHref && (
        <a
          href={ctaHref}
          className="text-xs text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {ctaLabel}
        </a>
      )}
    </div>
  );
}
