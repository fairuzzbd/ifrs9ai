"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ColumnDef } from "@tanstack/react-table";
import { format } from "date-fns";
import { CalendarDays } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { DataTable, SortHeader } from "@/components/blips/DataTable";
import { MtmCronTriggerButton } from "@/components/blips/mtm/MtmCronTriggerButton";

import { mtmQueryKeys, mtmCronApi } from "@/lib/api/mtm.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { MtmCronJobResponse } from "@/lib/schemas/mtm.schema";
import { apiGet, type ListResponse } from "@/lib/api";

// ---------------------------------------------------------------------------
// Job history list — GET /api/v1/jobs?type=MTM_DAILY_RUN
// ---------------------------------------------------------------------------

interface JobRecord {
  id: string;
  type: string;
  status: "queued" | "running" | "completed" | "failed" | "cancelled";
  progress: number;
  currentStep: string | null;
  tanggalTarget: string | null;
  startedAt: string | null;
  completedAt: string | null;
  createdBy: string;
  canCancel: boolean;
  error: { message: string } | null;
}

function JobStatusBadge({ status }: { status: JobRecord["status"] }) {
  const MAP = {
    queued: "bg-slate-100 text-slate-700",
    running: "bg-blue-100 text-blue-700",
    completed: "bg-green-100 text-green-700",
    failed: "bg-red-100 text-red-700",
    cancelled: "bg-amber-100 text-amber-700",
  } as const;
  const LABELS = {
    queued: "Antri",
    running: "Berjalan",
    completed: "Selesai",
    failed: "Gagal",
    cancelled: "Dibatalkan",
  } as const;
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${MAP[status]}`}>
      {LABELS[status]}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Inline JobProgressPanel equivalent using SSE / polling for single job
// ---------------------------------------------------------------------------

function ActiveJobProgress({
  jobId,
  onDone,
}: {
  jobId: string;
  onDone: () => void;
}) {
  const { data } = useQuery({
    queryKey: ["job", jobId],
    queryFn: () => apiGet<{ data: { status: string; progress: number; currentStep: string; estimatedCompletionAt: string | null } }>(`/api/v1/jobs/${jobId}`),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status;
      if (status === "completed" || status === "failed" || status === "cancelled") {
        return false;
      }
      return 2000;
    },
    staleTime: 0,
  });

  const job = data?.data;

  React.useEffect(() => {
    if (job?.status === "completed" || job?.status === "failed") {
      onDone();
    }
  }, [job?.status, onDone]);

  if (!job) return <div className="text-sm text-muted-foreground">Memuat status job...</div>;

  const pct = job.progress ?? 0;
  const isDone = job.status === "completed" || job.status === "failed";

  return (
    <div className="rounded-lg border p-4 space-y-3 bg-blue-50/40">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">MTM Cron — sedang berjalan</span>
        <JobStatusBadge status={job.status as JobRecord["status"]} />
      </div>
      <div>
        <div
          className="h-2 w-full rounded-full bg-muted overflow-hidden"
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={`Progress MTM cron: ${pct}%`}
        >
          <div
            className="h-full bg-primary transition-all duration-300"
            style={{ width: `${pct}%` }}
          />
        </div>
        <p className="text-xs text-muted-foreground mt-1">
          {pct}% — {job.currentStep ?? "Memproses..."}
        </p>
      </div>
      {job.estimatedCompletionAt && !isDone && (
        <p className="text-xs text-muted-foreground">
          ETA: {job.estimatedCompletionAt}
        </p>
      )}
      {isDone && (
        <p className="text-xs text-green-700 font-medium">
          {job.status === "completed" ? "Job selesai." : "Job gagal. Periksa error log."}
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// History columns
// ---------------------------------------------------------------------------

const HISTORY_COLUMNS: ColumnDef<JobRecord>[] = [
  {
    accessorKey: "id",
    header: "Job ID",
    cell: ({ row }) => (
      <span className="font-mono text-xs text-muted-foreground">
        {row.original.id.slice(0, 12)}...
      </span>
    ),
  },
  {
    accessorKey: "tanggalTarget",
    header: "Tanggal Target",
    cell: ({ row }) => (
      <span className="text-sm">{row.original.tanggalTarget ?? "—"}</span>
    ),
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <JobStatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "progress",
    header: "Progress",
    cell: ({ row }) => (
      <div
        className="h-1.5 w-24 rounded-full bg-muted overflow-hidden"
        role="progressbar"
        aria-valuenow={row.original.progress}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`${row.original.progress}%`}
        title={`${row.original.progress}%`}
      >
        <div className="h-full bg-primary" style={{ width: `${row.original.progress}%` }} />
      </div>
    ),
  },
  {
    accessorKey: "startedAt",
    header: "Mulai",
    cell: ({ row }) => (
      <span className="text-xs text-muted-foreground">{row.original.startedAt ?? "—"}</span>
    ),
  },
  {
    accessorKey: "completedAt",
    header: "Selesai",
    cell: ({ row }) => (
      <span className="text-xs text-muted-foreground">{row.original.completedAt ?? "—"}</span>
    ),
  },
  {
    accessorKey: "error",
    header: "Error",
    cell: ({ row }) =>
      row.original.error ? (
        <span className="text-xs text-destructive">{row.original.error.message}</span>
      ) : (
        <span className="text-xs text-muted-foreground">—</span>
      ),
  },
];

// ---------------------------------------------------------------------------
// Page content
// ---------------------------------------------------------------------------

function CronPageContent() {
  const perms = usePermissions();
  const queryClient = useQueryClient();

  const [tanggalTarget, setTanggalTarget] = React.useState(format(new Date(), "yyyy-MM-dd"));
  const [forceRerun, setForceRerun] = React.useState(false);
  const [activeJobId, setActiveJobId] = React.useState<string | null>(null);
  const [lastUpdated, setLastUpdated] = React.useState<Date>(new Date());

  // History — reuse jobs list endpoint
  const { data: historyData, isLoading: historyLoading, refetch: refetchHistory } = useQuery({
    queryKey: [...mtmQueryKeys.jobs(), { type: "MTM_DAILY_RUN" }],
    queryFn: () =>
      apiGet<ListResponse<JobRecord>>(
        `/api/v1/jobs?filter[type]=MTM_DAILY_RUN&sort=created_at:desc&limit=20`,
      ),
    staleTime: 30_000,
    enabled: perms.can("mtm.trigger"),
  });

  const handleJobStarted = (job: MtmCronJobResponse) => {
    setActiveJobId(job.jobId);
  };

  const handleJobDone = () => {
    setActiveJobId(null);
    void refetchHistory();
    void queryClient.invalidateQueries({ queryKey: mtmQueryKeys.lists() });
  };

  // Guard: if user has no mtm.trigger, render nothing (middleware should catch first)
  if (!perms.can("mtm.trigger")) {
    return (
      <div className="container mx-auto py-6 text-center space-y-3">
        <p className="text-muted-foreground">Halaman ini hanya untuk ROLE-IT-ADMIN.</p>
        <Button variant="outline" asChild>
          <Link href="/mtm">Kembali ke MTM</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto py-6 space-y-6">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/mtm" className="hover:underline">MTM Harian</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Trigger Cron Manual</span>
      </nav>

      <div>
        <h1 className="text-2xl font-semibold">Trigger MTM Cron Manual</h1>
        <p className="text-muted-foreground text-sm mt-1">
          Jalankan proses MTM harian secara manual untuk tanggal tertentu (ROLE-IT-ADMIN).
          Cron otomatis berjalan setiap hari kerja pukul 18:00 WIB.
        </p>
      </div>

      {/* Trigger form */}
      <section className="rounded-lg border p-5 space-y-4 max-w-lg" aria-labelledby="trigger-heading">
        <h2 id="trigger-heading" className="text-base font-semibold">Parameter Run</h2>

        <div className="space-y-1.5">
          <Label htmlFor="tanggalTarget" className="flex items-center gap-1.5">
            <CalendarDays className="h-4 w-4 text-muted-foreground" aria-hidden />
            Tanggal Target
          </Label>
          <Input
            id="tanggalTarget"
            type="date"
            value={tanggalTarget}
            max={format(new Date(), "yyyy-MM-dd")}
            onChange={(e) => setTanggalTarget(e.target.value)}
            className="w-[200px]"
            aria-describedby="tanggalTarget-hint"
          />
          <p id="tanggalTarget-hint" className="text-xs text-muted-foreground">
            Tidak boleh tanggal yang akan datang (DEC-context: MTM proses harga aktual).
          </p>
        </div>

        <div className="flex items-center gap-3">
          <Switch
            id="forceRerun"
            checked={forceRerun}
            onCheckedChange={setForceRerun}
            aria-describedby="forceRerun-hint"
          />
          <div>
            <Label htmlFor="forceRerun" className="text-sm">Force Re-run</Label>
            <p id="forceRerun-hint" className="text-xs text-muted-foreground">
              Jika aktif, instrumen yang sudah AUTO_POSTED atau APPROVED akan diproses ulang.
            </p>
          </div>
        </div>

        <MtmCronTriggerButton
          tanggalTarget={tanggalTarget}
          forceRerun={forceRerun}
          onJobStarted={handleJobStarted}
        />
      </section>

      {/* Active job progress */}
      {activeJobId && (
        <section className="max-w-lg" aria-labelledby="progress-heading">
          <h2 id="progress-heading" className="sr-only">Progress Job Aktif</h2>
          <ActiveJobProgress jobId={activeJobId} onDone={handleJobDone} />
        </section>
      )}

      {/* Job history */}
      <section aria-labelledby="history-heading">
        <h2 id="history-heading" className="text-base font-semibold mb-3">Riwayat MTM Cron (20 Terbaru)</h2>
        <DataTable
          columns={HISTORY_COLUMNS}
          data={historyData?.data ?? []}
          isLoading={historyLoading}
          isError={false}
          error={null}
          sorting={[]}
          onSortingChange={() => {}}
          activeFilters={[]}
          onRemoveFilter={() => {}}
          onClearFilters={() => {}}
          onNextPage={() => {}}
          onPrevPage={() => {}}
          canPrevPage={false}
          pageNumber={1}
          onRefresh={() => {
            void refetchHistory();
            setLastUpdated(new Date());
          }}
          lastUpdated={lastUpdated}
          emptyMessage="Belum ada riwayat MTM cron."
        />
      </section>
    </div>
  );
}

export default function MtmCronPage() {
  return (
    <Suspense>
      <CronPageContent />
    </Suspense>
  );
}
