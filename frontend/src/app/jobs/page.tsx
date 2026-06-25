"use client";

/**
 * P5-M15 — /jobs — Job History page.
 * Lists all background jobs for the current user (admin sees all).
 * Uses shared <DataTable> with sort/filter/pagination.
 */

import * as React from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { History, RefreshCw, ExternalLink } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import Link from "next/link";
import { DataTable, type ActiveFilter } from "@/components/blips/DataTable";
import { Badge } from "@/components/ui/badge";
import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { jobsListApi, dashboardQueryKeys } from "@/lib/api/reports.api";
import {
  JOB_TYPE_LABELS,
  JOB_STATUS_LABELS,
  type JobListItem,
  type JobListParams,
} from "@/lib/schemas/jobs.schema";
import { formatDateTime, formatDuration } from "@/lib/format";
import { notify } from "@/lib/notify";
import { jobApi } from "@/lib/api/reporting.api";

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  completed: "default",
  running: "secondary",
  queued: "outline",
  failed: "destructive",
  cancelled: "outline",
};

const STATUS_COLOR: Record<string, string> = {
  completed: "text-green-700",
  running: "text-blue-700",
  queued: "text-muted-foreground",
  failed: "text-red-700",
  cancelled: "text-muted-foreground",
};

export default function JobsPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();

  // URL-driven state
  const [cursor, setCursor] = React.useState<string | null>(null);
  const [prevCursors, setPrevCursors] = React.useState<string[]>([]);
  const [pageNumber, setPageNumber] = React.useState(1);
  const [search, setSearch] = React.useState(searchParams.get("q") ?? "");
  const [statusFilter, setStatusFilter] = React.useState(searchParams.get("status") ?? "");
  const [typeFilter, setTypeFilter] = React.useState(searchParams.get("type") ?? "");
  const [sortField, setSortField] = React.useState("created_at");
  const [sortDir, setSortDir] = React.useState<"asc" | "desc">("desc");

  const params: JobListParams = {
    cursor: cursor ?? undefined,
    limit: 50,
    sort: `${sortField}:${sortDir}`,
    ...(search ? { q: search } : {}),
    ...(statusFilter ? { "filter[status]": statusFilter } : {}),
    ...(typeFilter ? { "filter[type]": typeFilter } : {}),
  };

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: dashboardQueryKeys.jobsList(params),
    queryFn: () => jobsListApi.list(params),
    staleTime: 10_000,
    refetchInterval: 10_000, // 10s polling for active jobs
  });

  const rows: JobListItem[] = data?.data ?? [];
  const hasMore = data?.pagination?.hasMore ?? false;

  // Cancel job handler
  const handleCancel = async (jobId: string) => {
    try {
      await jobApi.cancel(jobId);
      notify.success("Job berhasil dibatalkan.");
      void queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.all });
    } catch (err) {
      notify.error(err as never);
    }
  };

  const columns: ColumnDef<JobListItem>[] = [
    {
      accessorKey: "jobId",
      header: "Job ID",
      cell: ({ getValue, row }) => (
        <Link
          href={`/jobs/${String(getValue())}`}
          className="font-mono text-xs text-primary hover:underline inline-flex items-center gap-1"
          aria-label={`Lihat detail job ${String(getValue())}`}
        >
          {String(getValue()).slice(0, 12)}…
          <ExternalLink className="h-3 w-3" aria-hidden="true" />
        </Link>
      ),
    },
    {
      accessorKey: "type",
      header: "Tipe",
      cell: ({ getValue }) => (
        <span className="text-sm">
          {JOB_TYPE_LABELS[String(getValue())] ?? String(getValue())}
        </span>
      ),
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => {
        const status = String(getValue());
        return (
          <Badge
            variant={STATUS_VARIANT[status] ?? "outline"}
            className={STATUS_COLOR[status] ?? ""}
          >
            {JOB_STATUS_LABELS[status as keyof typeof JOB_STATUS_LABELS] ?? status}
          </Badge>
        );
      },
    },
    {
      accessorKey: "progress",
      header: "Progress",
      cell: ({ getValue, row }) => {
        const pct = Number(getValue());
        const status = row.original.status;
        if (status === "completed") return <span className="text-green-700 text-xs">100%</span>;
        if (status === "failed" || status === "cancelled") return <span className="text-muted-foreground text-xs">—</span>;
        return (
          <div className="flex items-center gap-2 min-w-[80px]">
            <Progress
              value={pct}
              className="h-1.5 flex-1"
              aria-label={`Progress ${pct}%`}
            />
            <span className="text-xs tabular-nums text-muted-foreground">{pct}%</span>
          </div>
        );
      },
    },
    {
      accessorKey: "currentStep",
      header: "Step Saat Ini",
      cell: ({ getValue }) => (
        <span className="text-xs text-muted-foreground line-clamp-1 max-w-[200px]">
          {String(getValue() ?? "—")}
        </span>
      ),
    },
    {
      accessorKey: "createdByUsername",
      header: "Dibuat oleh",
      cell: ({ getValue }) => <span className="text-sm">{String(getValue() ?? "—")}</span>,
    },
    {
      accessorKey: "createdAt",
      header: "Dibuat",
      cell: ({ getValue }) => (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {formatDateTime(String(getValue() ?? ""))}
        </span>
      ),
    },
    {
      accessorKey: "durationSeconds",
      header: "Durasi",
      cell: ({ getValue }) => {
        const v = getValue();
        return (
          <span className="text-xs tabular-nums text-muted-foreground">
            {v != null ? formatDuration(Number(v)) : "—"}
          </span>
        );
      },
    },
    {
      id: "actions",
      header: "Aksi",
      cell: ({ row }) => {
        const job = row.original;
        const canCancel = job.canCancel && (job.status === "running" || job.status === "queued");
        const hasResult = job.resultUrl || job.status === "completed";
        return (
          <div className="flex items-center gap-1">
            <Button asChild variant="ghost" size="sm" className="h-7 text-xs px-2">
              <Link href={`/jobs/${job.jobId}`}>Detail</Link>
            </Button>
            {hasResult && job.resultUrl && (
              <Button asChild variant="ghost" size="sm" className="h-7 text-xs px-2">
                <a href={job.resultUrl} target="_blank" rel="noopener noreferrer" aria-label="Unduh hasil job">
                  Unduh
                </a>
              </Button>
            )}
            {canCancel && (
              <Button
                variant="ghost"
                size="sm"
                className="h-7 text-xs px-2 text-red-600 hover:text-red-700 hover:bg-red-50"
                onClick={() => void handleCancel(job.jobId)}
                aria-label={`Batalkan job ${job.jobId}`}
              >
                Batalkan
              </Button>
            )}
          </div>
        );
      },
    },
  ];

  // Active filter chips
  const activeFilters: ActiveFilter[] = [
    ...(statusFilter ? [{ key: "status", label: "Status", value: statusFilter, displayValue: JOB_STATUS_LABELS[statusFilter as keyof typeof JOB_STATUS_LABELS] ?? statusFilter }] : []),
    ...(typeFilter ? [{ key: "type", label: "Tipe", value: typeFilter, displayValue: JOB_TYPE_LABELS[typeFilter] ?? typeFilter }] : []),
  ];

  const handleRemoveFilter = (key: string) => {
    if (key === "status") setStatusFilter("");
    if (key === "type") setTypeFilter("");
    setCursor(null);
    setPrevCursors([]);
    setPageNumber(1);
  };

  const handleClearFilters = () => {
    setStatusFilter("");
    setTypeFilter("");
    setSearch("");
    setCursor(null);
    setPrevCursors([]);
    setPageNumber(1);
  };

  const handleNextPage = () => {
    if (data?.pagination?.nextCursor) {
      setPrevCursors((prev) => [...prev, cursor ?? ""]);
      setCursor(data.pagination!.nextCursor ?? null);
      setPageNumber((p) => p + 1);
    }
  };

  const handlePrevPage = () => {
    const prev = [...prevCursors];
    const prevCursor = prev.pop() ?? null;
    setPrevCursors(prev);
    setCursor(prevCursor);
    setPageNumber((p) => Math.max(1, p - 1));
  };

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center gap-3">
        <History className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Riwayat Job</h1>
          <p className="text-sm text-muted-foreground">
            Background jobs: ECL run, import data, export, rekonsiliasi GL
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          onClick={() => void refetch()}
          aria-label="Refresh daftar job"
        >
          <RefreshCw className="h-4 w-4 mr-1.5" aria-hidden="true" />
          Refresh
        </Button>
      </div>

      <DataTable
        columns={columns}
        data={rows}
        pagination={data?.pagination}
        isLoading={isLoading}
        isError={isError}
        error={error as Error | null}
        // Sort (client-side for this table; server sort via params)
        searchValue={search}
        onSearchChange={(v) => {
          setSearch(v);
          setCursor(null);
          setPrevCursors([]);
          setPageNumber(1);
        }}
        searchPlaceholder="Cari job ID atau tipe..."
        activeFilters={activeFilters}
        onRemoveFilter={handleRemoveFilter}
        onClearFilters={handleClearFilters}
        onNextPage={handleNextPage}
        onPrevPage={handlePrevPage}
        canPrevPage={prevCursors.length > 0}
        pageNumber={pageNumber}
        onRefresh={() => void refetch()}
        emptyMessage="Tidak ada job yang cocok dengan filter saat ini."
        onRetry={() => void refetch()}
      />
    </div>
  );
}
