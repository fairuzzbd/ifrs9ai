"use client";

/**
 * P5-M15 — /jobs/[jobId] — Job detail page.
 * Shows JobProgressPanel for active jobs, result/error details for finished ones.
 */

import * as React from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Download, RotateCcw } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { jobApi } from "@/lib/api/reporting.api";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { notify } from "@/lib/notify";
import { dashboardQueryKeys } from "@/lib/api/reports.api";
import {
  JOB_TYPE_LABELS,
  JOB_STATUS_LABELS,
} from "@/lib/schemas/jobs.schema";
import { formatDateTime, formatDuration } from "@/lib/format";

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  completed: "default",
  running: "secondary",
  queued: "outline",
  failed: "destructive",
  cancelled: "outline",
};

export default function JobDetailPage() {
  const params = useParams();
  const router = useRouter();
  const queryClient = useQueryClient();
  const jobId = typeof params.jobId === "string" ? params.jobId : "";

  const { data, isLoading, isError } = useQuery({
    queryKey: ["job-detail", jobId],
    queryFn: () => jobApi.getStatus(jobId),
    enabled: Boolean(jobId),
    staleTime: 5_000,
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status;
      if (status === "running" || status === "queued") return 5_000;
      return false;
    },
  });

  const job = data?.data;

  const isActive = job?.status === "running" || job?.status === "queued";

  const handleComplete = (result: unknown) => {
    void queryClient.invalidateQueries({ queryKey: dashboardQueryKeys.all });
    const r = result as Record<string, unknown> | null;
    notify.success(
      `Job ${JOB_TYPE_LABELS[job?.type ?? ""] ?? job?.type ?? ""} selesai.${r?.totalECL ? ` Total ECL: ${String(r.totalECL)}` : ""}`,
    );
  };

  const handleFail = (error: unknown) => {
    const err = error as { message?: string; code?: string; traceId?: string } | null;
    notify.error({
      code: err?.code ?? "JOB_FAILED",
      message: err?.message ?? "Job gagal — lihat detail di bawah.",
      traceId: err?.traceId ?? "",
      details: [],
    } as never);
  };

  if (isLoading) {
    return (
      <div className="p-6 space-y-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-32 w-full" />
      </div>
    );
  }

  if (isError || !job) {
    return (
      <div className="p-6 space-y-4">
        <p className="text-red-600">Gagal memuat detail job. Periksa Job ID atau coba lagi.</p>
        <Button asChild variant="outline">
          <Link href="/jobs">
            <ArrowLeft className="h-4 w-4 mr-1.5" aria-hidden="true" />
            Kembali ke Daftar Job
          </Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-3xl mx-auto">
      {/* Header */}
      <div className="flex items-start gap-4">
        <Button asChild variant="ghost" size="sm" className="-ml-2">
          <Link href="/jobs" aria-label="Kembali ke daftar job">
            <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
            Daftar Job
          </Link>
        </Button>
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <h1 className="text-xl font-semibold">
          {JOB_TYPE_LABELS[job.type] ?? job.type}
        </h1>
        <Badge variant={STATUS_VARIANT[job.status] ?? "outline"}>
          {JOB_STATUS_LABELS[job.status as keyof typeof JOB_STATUS_LABELS] ?? job.status}
        </Badge>
      </div>

      {/* Metadata */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm text-muted-foreground">Informasi Job</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Job ID</dt>
            <dd className="font-mono">{job.jobId}</dd>
            <dt className="text-muted-foreground">Tipe</dt>
            <dd>{JOB_TYPE_LABELS[job.type] ?? job.type}</dd>
            <dt className="text-muted-foreground">Status</dt>
            <dd>
              <Badge variant={STATUS_VARIANT[job.status] ?? "outline"} className="text-xs">
                {JOB_STATUS_LABELS[job.status as keyof typeof JOB_STATUS_LABELS] ?? job.status}
              </Badge>
            </dd>
            <dt className="text-muted-foreground">Dibuat oleh</dt>
            <dd>{job.createdByUsername ?? job.createdBy ?? "—"}</dd>
            {job.startedAt && (
              <>
                <dt className="text-muted-foreground">Dimulai</dt>
                <dd>{formatDateTime(job.startedAt)}</dd>
              </>
            )}
            {job.completedAt && (
              <>
                <dt className="text-muted-foreground">Selesai</dt>
                <dd>{formatDateTime(job.completedAt)}</dd>
              </>
            )}
            {job.durationSeconds != null && (
              <>
                <dt className="text-muted-foreground">Durasi</dt>
                <dd>{formatDuration(job.durationSeconds)}</dd>
              </>
            )}
          </dl>
        </CardContent>
      </Card>

      {/* Progress panel (active jobs) */}
      {isActive && (
        <JobProgressPanel
          jobId={jobId}
          title={JOB_TYPE_LABELS[job.type] ?? job.type}
          onComplete={handleComplete}
          onFail={handleFail}
          showCancel={job.canCancel}
          showBackground={false}
          variant="panel"
        />
      )}

      {/* Result */}
      {job.status === "completed" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Hasil</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {job.resultUrl ? (
              <Button asChild variant="outline" size="sm">
                <a href={job.resultUrl} target="_blank" rel="noopener noreferrer" aria-label="Unduh hasil job">
                  <Download className="h-4 w-4 mr-1.5" aria-hidden="true" />
                  Unduh Hasil
                </a>
              </Button>
            ) : null}
            {job.result != null && (
              <pre className="text-xs bg-muted rounded p-3 overflow-auto max-h-48" aria-label="Data hasil job">
                {JSON.stringify(job.result, null, 2)}
              </pre>
            )}
          </CardContent>
        </Card>
      )}

      {/* Error */}
      {job.status === "failed" && (
        <Card className="border-red-200">
          <CardHeader>
            <CardTitle className="text-sm text-red-700">Error</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <p className="text-sm text-red-600">
              {typeof job.error === "object" && job.error !== null && "message" in job.error
                ? String((job.error as Record<string, unknown>).message)
                : "Job gagal tanpa detail error."}
            </p>
            {job.error != null && (
              <pre className="text-xs bg-red-50 rounded p-3 overflow-auto max-h-32" aria-label="Detail error job">
                {JSON.stringify(job.error, null, 2)}
              </pre>
            )}
            <Button
              variant="outline"
              size="sm"
              className="text-red-600 border-red-300 hover:bg-red-50"
              onClick={() => void router.push("/ecl/calc-run")}
              aria-label="Coba ulangi dengan membuat job baru"
            >
              <RotateCcw className="h-4 w-4 mr-1.5" aria-hidden="true" />
              Buat Ulang Job
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
