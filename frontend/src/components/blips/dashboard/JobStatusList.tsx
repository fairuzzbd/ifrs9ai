"use client";

/**
 * P5-M15 — JobStatusList: mini list of active/queued jobs with SSE subscription.
 * Polls GET /api/v1/jobs?status=running,queued&limit=5 every 10 seconds.
 * Falls back to page-level SSE if a specific jobId is passed.
 */

import * as React from "react";
import Link from "next/link";
import { Loader2 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { jobsListApi } from "@/lib/api/reports.api";
import { JOB_TYPE_LABELS, JOB_STATUS_LABELS } from "@/lib/schemas/jobs.schema";
import type { JobListItem } from "@/lib/schemas/jobs.schema";
import { cn } from "@/lib/utils";

const POLL_INTERVAL_MS = 10_000; // 10 seconds per design

export interface JobStatusListProps {
  onViewAll?: () => void;
  loading?: boolean;
  error?: string;
  className?: string;
}

export function JobStatusList({
  onViewAll,
  className,
}: JobStatusListProps) {
  const { data, isLoading } = useQuery({
    queryKey: ["dashboard", "active-jobs"],
    queryFn: () =>
      jobsListApi.list({
        "filter[status]": "running,queued",
        limit: 5,
        sort: "created_at:desc",
      }),
    refetchInterval: POLL_INTERVAL_MS,
    staleTime: POLL_INTERVAL_MS,
  });

  const jobs: JobListItem[] = data?.data ?? [];

  if (isLoading) {
    return (
      <div className={cn("space-y-1", className)}>
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    );
  }

  if (jobs.length === 0) {
    return null; // collapsed when no active jobs (per design §6.2 row 5)
  }

  return (
    <div className={cn("space-y-1", className)}>
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs font-medium text-muted-foreground">Job Aktif</p>
        {onViewAll && (
          <button
            type="button"
            onClick={onViewAll}
            className="text-xs text-primary underline-offset-2 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Lihat semua →
          </button>
        )}
      </div>

      {jobs.map((job) => (
        <JobStatusItem key={job.jobId} job={job} />
      ))}
    </div>
  );
}

function JobStatusItem({ job }: { job: JobListItem }) {
  const typeLabel = JOB_TYPE_LABELS[job.type] ?? job.type;
  const statusLabel = JOB_STATUS_LABELS[job.status] ?? job.status;

  return (
    <div className="flex items-center gap-2 rounded-md border border-border p-2 text-xs">
      {job.status === "running" && (
        <Loader2 className="h-3 w-3 animate-spin text-primary flex-shrink-0" aria-hidden="true" />
      )}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1">
          <span className="font-medium truncate">{typeLabel}</span>
          <Badge variant="outline" className="text-xs px-1 py-0 h-4">
            {statusLabel}
          </Badge>
        </div>
        {/* Mini progress bar */}
        {job.status === "running" && (
          <div className="mt-1">
            <div className="h-1 w-full rounded-full bg-muted overflow-hidden">
              <div
                className="h-full bg-primary transition-all"
                style={{ width: `${job.progress}%` }}
                role="progressbar"
                aria-valuenow={job.progress}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={`Kemajuan ${typeLabel}: ${job.progress}%`}
              />
            </div>
            {job.estimatedCompletionAt && (
              <p className="text-muted-foreground mt-0.5">
                ETA: {job.estimatedCompletionAt}
              </p>
            )}
          </div>
        )}
      </div>
      <Link
        href={`/jobs/${job.jobId}`}
        className="text-xs text-primary underline-offset-2 hover:underline flex-shrink-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-label={`Lihat detail job ${job.jobId}`}
      >
        Lihat →
      </Link>
    </div>
  );
}
