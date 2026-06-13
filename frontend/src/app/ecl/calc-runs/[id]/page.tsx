"use client";

import * as React from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Lock,
  AlertTriangle,
  ExternalLink,
  Loader2,
  Play,
  X,
} from "lucide-react";

import type { CalcRun } from "@/lib/schemas/calc-run.schema";
import { cancelCalcRunSchema, type CancelCalcRunForm } from "@/lib/schemas/calc-run.schema";
import type { EclResultLine } from "@/lib/schemas/ecl-core.schema";
import { calcRunApi } from "@/lib/api/calc-run.api";
import { eclCoreApi } from "@/lib/api/ecl-core.api";
import { notify } from "@/lib/notify";
import { usePermissions, useAuthStore } from "@/lib/stores/auth.store";
import { useCalcRunStore } from "@/lib/stores/calc-run.store";

import { CalcRunStatusBadge } from "@/components/blips/CalcRunStatusBadge";
import { StageBadge } from "@/components/blips/StageBadge";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { JSONBTreeView } from "@/components/blips/JSONBTreeView";
import { SealWorkflowPanel } from "@/components/blips/SealWorkflowPanel";
import { SignatureHashBadge } from "@/components/blips/SignatureHashBadge";
import { DataTable } from "@/components/blips/DataTable";
import type { ActiveFilter } from "@/components/blips/DataTable";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Textarea } from "@/components/ui/textarea";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";

// ---------------------------------------------------------------------------
// IDR formatter
// ---------------------------------------------------------------------------

function formatIDR(value: string | null | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 4,
    maximumFractionDigits: 4,
  }).format(num);
}

function formatDateTime(value: string | null | undefined): string {
  if (!value) return "—";
  return new Date(value).toLocaleString("id-ID", {
    timeZone: "Asia/Jakarta",
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }) + " WIB";
}

function formatDuration(start: string | null | undefined, end: string | null | undefined): string {
  if (!start || !end) return "";
  const ms = new Date(end).getTime() - new Date(start).getTime();
  const mins = Math.floor(ms / 60000);
  const secs = Math.floor((ms % 60000) / 1000);
  if (mins > 0) return ` (durasi: ${mins} menit${secs > 0 ? ` ${secs} detik` : ""})`;
  return ` (durasi: ${secs} detik)`;
}

// ---------------------------------------------------------------------------
// Result table columns
// ---------------------------------------------------------------------------

function buildResultColumns(
  calcRunId: string,
  router: ReturnType<typeof useRouter>,
): ColumnDef<EclResultLine>[] {
  return [
    {
      id: "kodeInstrumen",
      header: "Kode Instrumen",
      enableSorting: true,
      cell: ({ row }) => (
        <button
          className="text-sm font-mono text-primary underline-offset-4 hover:underline"
          onClick={() =>
            router.push(
              `/ecl/calc-runs/${calcRunId}/instrumen/${row.original.instrumenId}`,
            )
          }
        >
          {row.original.kodeInstrumen ?? row.original.instrumenId.slice(0, 12)}
        </button>
      ),
    },
    {
      id: "namaInstrumen",
      header: "Nama",
      cell: ({ row }) => (
        <span className="text-sm line-clamp-1 max-w-xs">
          {row.original.namaInstrumen ?? "—"}
        </span>
      ),
    },
    {
      id: "portofolioNama",
      header: "Portofolio",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs">{row.original.portofolioNama ?? "—"}</span>
      ),
    },
    {
      id: "stage",
      header: "Stage",
      enableSorting: true,
      cell: ({ row }) => {
        const s = row.original.stage;
        return s ? <StageBadge stage={s} size="sm" /> : <span className="text-muted-foreground text-xs">—</span>;
      },
    },
    {
      id: "eadIdr",
      header: "EAD (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs tabular-nums text-right">{formatIDR(row.original.eadIdr)}</span>
      ),
    },
    {
      id: "eclFlIdr",
      header: "ECL FL (IDR)",
      enableSorting: true,
      cell: ({ row }) => (
        <span className="text-xs tabular-nums text-right font-medium">{formatIDR(row.original.eclFlIdr)}</span>
      ),
    },
    {
      id: "routingPath",
      header: "Jalur",
      cell: ({ row }) => (
        <Badge variant="outline" className="text-xs">
          {row.original.routingPath}
        </Badge>
      ),
    },
    {
      id: "warnings",
      header: "",
      cell: ({ row }) => {
        if (!row.original.warnings?.length) return null;
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <AlertTriangle className="h-4 w-4 text-amber-500" aria-label="Ada warning" />
            </TooltipTrigger>
            <TooltipContent>
              <p className="text-xs max-w-xs">{row.original.warnings[0]}</p>
            </TooltipContent>
          </Tooltip>
        );
      },
    },
  ];
}

// ---------------------------------------------------------------------------
// Start + Cancel dialogs
// ---------------------------------------------------------------------------

function StartCalcRunDialog({
  calcRunId,
  periodeLabel,
  open,
  onOpenChange,
  onStarted,
}: {
  calcRunId: string;
  periodeLabel: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStarted: (jobId: string, streamUrl: string) => void;
}) {
  const mutation = useMutation({
    mutationFn: () => calcRunApi.start(calcRunId, uuidv4()),
    onSuccess: (res) => {
      onOpenChange(false);
      onStarted(res.data.jobId, res.data.streamUrl);
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Mulai Bulk ECL Compute — {periodeLabel}?</AlertDialogTitle>
          <AlertDialogDescription>
            Proses ini akan menghitung ECL untuk semua instrumen aktif di periode{" "}
            {periodeLabel}. Pastikan semua parameter ECL sudah diapprove ALCO.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Batal</AlertDialogCancel>
          <AlertDialogAction
            onClick={(e) => {
              e.preventDefault();
              mutation.mutate();
            }}
            disabled={mutation.isPending}
          >
            {mutation.isPending && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
            )}
            Mulai
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function CancelCalcRunDialog({
  calcRunId,
  processedCount,
  open,
  onOpenChange,
}: {
  calcRunId: string;
  processedCount: number | null | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const form = useForm<CancelCalcRunForm>({
    resolver: zodResolver(cancelCalcRunSchema),
    defaultValues: { cancelReason: "" },
  });

  const mutation = useMutation({
    mutationFn: (data: CancelCalcRunForm) =>
      calcRunApi.cancel(calcRunId, { cancelReason: data.cancelReason }, uuidv4()),
    onSuccess: () => {
      const processed = processedCount ?? 0;
      notify.success(
        `Calc run ${calcRunId} berhasil dibatalkan. ${processed} instrumen partial tetap tersimpan.`,
      );
      onOpenChange(false);
      void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
    },
    onError: (err) => notify.error(err as Parameters<typeof notify.error>[0]),
  });

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Batalkan Calc Run?</AlertDialogTitle>
          <AlertDialogDescription>
            Instrumen yang sudah selesai dihitung akan tetap tersimpan sebagai
            partial result. Partial result tidak dapat digunakan untuk pelaporan
            resmi.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit((d) => mutation.mutate(d))}
            className="space-y-4 py-2"
          >
            <FormField
              control={form.control}
              name="cancelReason"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Alasan pembatalan (wajib, minimal 30 karakter)</FormLabel>
                  <FormControl>
                    <Textarea
                      rows={3}
                      placeholder="Jelaskan alasan pembatalan..."
                      aria-describedby="cancel-reason-error"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage id="cancel-reason-error" />
                </FormItem>
              )}
            />
            <AlertDialogFooter>
              <AlertDialogCancel onClick={() => onOpenChange(false)}>
                Kembali
              </AlertDialogCancel>
              <AlertDialogAction
                type="submit"
                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                disabled={mutation.isPending}
                onClick={(e) => {
                  e.preventDefault();
                  void form.handleSubmit((d) => mutation.mutate(d))();
                }}
              >
                {mutation.isPending && (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                )}
                Batalkan Proses
              </AlertDialogAction>
            </AlertDialogFooter>
          </form>
        </Form>
      </AlertDialogContent>
    </AlertDialog>
  );
}

// ---------------------------------------------------------------------------
// Main Page
// ---------------------------------------------------------------------------

export default function CalcRunDetailPage() {
  const params = useParams<{ id: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const { can, userId } = usePermissions();
  const { setSealModalState } = useCalcRunStore();

  const calcRunId = params.id;
  const defaultTab = searchParams.get("tab") ?? "all";

  const [activeJobId, setActiveJobId] = React.useState<string | null>(null);
  const [streamUrl, setStreamUrl] = React.useState<string | null>(null);
  const [startDialogOpen, setStartDialogOpen] = React.useState(false);
  const [cancelDialogOpen, setCancelDialogOpen] = React.useState(false);
  const [activeTab, setActiveTab] = React.useState(defaultTab);

  // Results table state
  const [resultCursor, setResultCursor] = React.useState<string | null>(null);
  const [resultPage, setResultPage] = React.useState(1);
  const [resultPrevCursors, setResultPrevCursors] = React.useState<string[]>([]);
  const [resultSearch, setResultSearch] = React.useState("");
  const [resultRouteFilter, setResultRouteFilter] = React.useState("");

  const { data: runData, isLoading: runLoading } = useQuery({
    queryKey: ["calc-run", calcRunId],
    queryFn: () => calcRunApi.get(calcRunId),
    refetchInterval: (query) => {
      const status = query.state.data?.data?.status;
      return status === "IN_PROGRESS" ? 5000 : false;
    },
  });

  const run = runData?.data;

  // Derive tab filter for stage
  const tabToFilter = (tab: string): Record<string, string> => {
    if (tab === "stage1") return { "filter[stage]": "1" };
    if (tab === "stage2") return { "filter[stage]": "2" };
    if (tab === "stage3") return { "filter[stage]": "3" };
    if (tab === "errors") return { "filter[has_error]": "true" };
    if (tab === "skipped") return { "filter[routing_path]": "FVTPL_SKIPPED" };
    return {};
  };

  const resultParams = {
    limit: 50,
    sort: "ecl_fl_idr:desc",
    ...(resultCursor && { cursor: resultCursor }),
    ...(resultSearch && { q: resultSearch }),
    ...(resultRouteFilter && { "filter[routing_path]": resultRouteFilter }),
    ...tabToFilter(activeTab),
  };

  const { data: resultsData, isLoading: resultsLoading } = useQuery({
    queryKey: ["calc-run-results", calcRunId, resultParams],
    queryFn: () => eclCoreApi.listResults(calcRunId, resultParams),
    enabled: !!run && run.status !== "DRAFT" && run.status !== "IN_PROGRESS",
  });

  const handleExportResults = (format: "csv" | "xlsx") => {
    eclCoreApi.exportResults(calcRunId, { ...resultParams, export: format });
  };

  const handleResultNextPage = () => {
    const next = resultsData?.pagination?.nextCursor ?? null;
    if (next) {
      setResultPrevCursors((p) => [...p, resultCursor ?? ""]);
      setResultCursor(next);
      setResultPage((n) => n + 1);
    }
  };

  const handleResultPrevPage = () => {
    const prev = resultPrevCursors[resultPrevCursors.length - 1] ?? null;
    setResultPrevCursors((p) => p.slice(0, -1));
    setResultCursor(prev);
    setResultPage((n) => Math.max(1, n - 1));
  };

  const handleJobComplete = React.useCallback(
    (result: unknown) => {
      const r = result as {
        totalInstrumen?: number;
        totalECLWeighted?: string;
        skippedFvtpl?: number;
      };
      setActiveJobId(null);
      notify.success(
        `ECL Calc Run ${run?.periodeLabel ?? ""} selesai. ${r.totalInstrumen ?? 0} instrumen dihitung. Siap untuk di-segel.`,
        {
          action: {
            label: "Lihat hasil",
            onClick: () => router.push(`/ecl/calc-runs/${calcRunId}`),
          },
        },
      );
      void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
    },
    [run, calcRunId, router, queryClient],
  );

  const handleJobFail = React.useCallback(
    (err: unknown) => {
      setActiveJobId(null);
      const errMsg = err instanceof Error ? err.message : String(err);
      if (errMsg.includes("COMPLETED_WITH_ERRORS") || run?.errorCount && run.errorCount > 0) {
        notify.warning(
          `Calc run selesai dengan ${run?.errorCount ?? "beberapa"} error. Perbaiki data instrumen sebelum segel.`,
        );
      }
      void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
    },
    [run, calcRunId, queryClient],
  );

  if (runLoading) {
    return (
      <div className="p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 bg-muted rounded w-1/3" />
          <div className="h-40 bg-muted rounded" />
        </div>
      </div>
    );
  }

  if (!run) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Calc run tidak ditemukan.</p>
      </div>
    );
  }

  const isSealed = run.status === "SEALED";
  const isSealRequested = run.status === "SEAL_REQUESTED";
  const isSealRejected = run.status === "SEAL_REJECTED";
  const isCompleted = run.status === "COMPLETED";
  const isCompletedWithErrors = run.status === "COMPLETED_WITH_ERRORS";
  const isDraft = run.status === "DRAFT";
  const isInProgress = run.status === "IN_PROGRESS" || !!activeJobId;
  const isCancelled = run.status === "CANCELLED";

  // SoD check for approve seal
  const canApproveSeeal =
    can("calc_run.seal_approve") &&
    userId !== run.createdBy &&
    userId !== run.sealInfo?.sealRequestedBy;

  const resultColumns = buildResultColumns(calcRunId, router);

  const resultActiveFilters: ActiveFilter[] = [];
  if (resultRouteFilter) {
    resultActiveFilters.push({
      key: "routing_path",
      label: "Jalur",
      value: resultRouteFilter,
      displayValue: resultRouteFilter,
    });
  }

  return (
    <div className="p-6 space-y-6 max-w-7xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/ecl/calc-runs")}
            >
              Calc Runs
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground font-mono">{calcRunId}</li>
        </ol>
      </nav>

      {/* Header Card (sticky) */}
      <Card className="sticky top-0 z-10 shadow-sm">
        <CardContent className="pt-4 pb-3">
          <div className="flex items-start justify-between gap-4 flex-wrap">
            <div className="space-y-1">
              <div className="flex items-center gap-2 flex-wrap">
                <h1 className="text-lg font-bold font-mono">{run.id}</h1>
                <CalcRunStatusBadge status={run.status} />
              </div>
              <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
                <span>Periode: <strong className="text-foreground">{run.periodeLabel ?? run.periodeId}</strong></span>
                <span>Eval Date: <strong className="text-foreground">{run.evaluationDate}</strong></span>
                <span>Dibuat: <strong className="text-foreground">{run.createdByUsername ?? run.createdBy.slice(0, 8)}</strong></span>
              </div>
              {run.startedAt && (
                <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
                  <span>Mulai: <strong className="text-foreground">{formatDateTime(run.startedAt)}</strong></span>
                  {run.completedAt && (
                    <span>
                      Selesai: <strong className="text-foreground">
                        {formatDateTime(run.completedAt)}
                        {formatDuration(run.startedAt, run.completedAt)}
                      </strong>
                    </span>
                  )}
                </div>
              )}
              {(run.processedCount != null || run.errorCount != null) && (
                <div className="flex gap-4 text-sm">
                  <span>
                    Diproses:{" "}
                    <strong>
                      {run.processedCount ?? 0} / {run.totalInstrumen ?? "?"}
                    </strong>
                  </span>
                  <span>
                    Error:{" "}
                    <strong
                      className={run.errorCount > 0 ? "text-destructive" : ""}
                    >
                      {run.errorCount}
                    </strong>
                  </span>
                </div>
              )}
            </div>

            {/* Action Row */}
            {!isSealed && (
              <div className="flex items-center gap-2 flex-wrap">
                {isDraft && can("calc_run.start") && (
                  <Button
                    onClick={() => setStartDialogOpen(true)}
                    className="gap-1"
                  >
                    <Play className="h-4 w-4" aria-hidden="true" />
                    Start Bulk Compute
                  </Button>
                )}
                {isDraft && can("calc_run.start") && (
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => setCancelDialogOpen(true)}
                  >
                    Batalkan Draft
                  </Button>
                )}
                {isCompleted && can("calc_run.seal_request") && (
                  <Button onClick={() => setSealModalState("request")}>
                    Request Seal
                  </Button>
                )}
                {isSealRejected && can("calc_run.seal_request") && (
                  <Button onClick={() => setSealModalState("request")}>
                    Request Seal Ulang
                  </Button>
                )}
                {isSealRequested && canApproveSeeal && (
                  <>
                    <Button onClick={() => setSealModalState("approve-confirm")}>
                      Approve Seal
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() => setSealModalState("reject")}
                    >
                      Tolak Seal
                    </Button>
                  </>
                )}
                {isSealRequested && !canApproveSeeal && can("calc_run.seal_approve") && (
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Button disabled>Approve Seal</Button>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p className="text-xs max-w-xs">
                        Anda adalah pembuat calc run ini. SoD tidak
                        memperbolehkan self-approval (DEC-017).
                      </p>
                    </TooltipContent>
                  </Tooltip>
                )}
                {isCancelled && can("calc_run.create") && (
                  <Button
                    variant="outline"
                    onClick={() => router.push("/ecl/calc-runs?create=1")}
                  >
                    Buat Calc Run Baru
                  </Button>
                )}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() =>
                    router.push(
                      `/audit?entity_type=ecl.calc_run&entity_id=${calcRunId}`,
                    )
                  }
                  aria-label="Lihat audit trail"
                >
                  <ExternalLink className="h-4 w-4 mr-1" aria-hidden="true" />
                  Lihat Audit Trail
                </Button>
              </div>
            )}
          </div>

          {/* IN_PROGRESS: JobProgressPanel inline */}
          {isInProgress && activeJobId && (
            <div className="mt-3">
              <JobProgressPanel
                jobId={activeJobId}
                title={`ECL Bulk Compute — ${run.periodeLabel ?? run.periodeId}`}
                onComplete={handleJobComplete}
                onFail={handleJobFail}
                showCancel={can("calc_run.cancel")}
                showBackground
                variant="inline"
              />
            </div>
          )}
          {isInProgress && !activeJobId && run.jobId && (
            <div className="mt-3">
              <JobProgressPanel
                jobId={run.jobId}
                title={`ECL Bulk Compute — ${run.periodeLabel ?? run.periodeId}`}
                onComplete={handleJobComplete}
                onFail={handleJobFail}
                showCancel={can("calc_run.cancel")}
                showBackground
                variant="inline"
              />
            </div>
          )}
        </CardContent>
      </Card>

      {/* SEAL_REQUESTED info bar */}
      {isSealRequested && (
        <Alert className="border-yellow-200 bg-yellow-50">
          <AlertTitle className="text-yellow-800">
            Menunggu persetujuan seal
          </AlertTitle>
          <AlertDescription className="text-yellow-700">
            Diminta oleh {run.sealInfo?.sealRequestedBy ?? "—"} pada{" "}
            {formatDateTime(run.sealInfo?.sealRequestedAt)}.
            {run.sealInfo?.sealRequestedComment && (
              <span> Catatan: &ldquo;{run.sealInfo.sealRequestedComment}&rdquo;</span>
            )}
          </AlertDescription>
        </Alert>
      )}

      {/* SEAL_REJECTED rejection notice */}
      {isSealRejected && run.sealInfo?.sealRejectedReason && (
        <Alert variant="destructive">
          <AlertTriangle className="h-4 w-4" aria-hidden="true" />
          <AlertTitle>Seal ditolak oleh {run.sealInfo.sealRejectedBy ?? "ALCO"}</AlertTitle>
          <AlertDescription>
            &ldquo;{run.sealInfo.sealRejectedReason}&rdquo;
            {run.sealInfo.sealRejectedAt && (
              <span> — Ditolak pada: {formatDateTime(run.sealInfo.sealRejectedAt)}.</span>
            )}
            {" "}Request seal ulang jika sudah diperbaiki.
          </AlertDescription>
        </Alert>
      )}

      {/* SEALED info bar */}
      {isSealed && (
        <Alert className="border-purple-200 bg-purple-50">
          <Lock className="h-4 w-4 text-purple-700" aria-hidden="true" />
          <AlertTitle className="text-purple-800">
            Tersegel — {formatDateTime(run.sealInfo?.sealedAt)}
            {run.sealInfo?.sealApprovedBy && ` oleh ${run.sealInfo.sealApprovedBy}`}
          </AlertTitle>
          <AlertDescription className="text-purple-700 space-y-1">
            {run.sealInfo?.sealSignature1 && (
              <SignatureHashBadge hash={run.sealInfo.sealSignature1} />
            )}
            <p className="text-xs">
              Calc run ini immutable. Tidak dapat dimodifikasi (DEC-018).
            </p>
          </AlertDescription>
        </Alert>
      )}

      {/* COMPLETED_WITH_ERRORS notice */}
      {isCompletedWithErrors && (
        <Alert className="border-amber-200 bg-amber-50">
          <AlertTriangle className="h-4 w-4 text-amber-700" aria-hidden="true" />
          <AlertTitle className="text-amber-800">
            Selesai dengan {run.errorCount} error
          </AlertTitle>
          <AlertDescription className="text-amber-700">
            Perbaiki data instrumen yang error sebelum mengajukan seal.
            <Button
              variant="link"
              size="sm"
              className="h-auto p-0 ml-1 text-amber-700 underline"
              onClick={() => setActiveTab("errors")}
            >
              Lihat error detail
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Parameter Snapshot */}
      <Collapsible>
        <Card>
          <CardHeader className="pb-2">
            <CollapsibleTrigger className="flex items-center gap-2 w-full text-left hover:opacity-70 transition-opacity">
              <CardTitle className="text-base">Parameter Snapshot</CardTitle>
              {run.parameterSnapshotJsonb && (
                <Badge variant="outline" className="text-xs font-normal">
                  <Lock className="h-3 w-3 mr-1" aria-hidden="true" />
                  Read-only — Frozen{run.startedAt ? ` pada ${formatDateTime(run.startedAt)}` : ""}
                </Badge>
              )}
            </CollapsibleTrigger>
          </CardHeader>
          <CollapsibleContent>
            <CardContent className="pt-0">
              {run.parameterSnapshotJsonb ? (
                <JSONBTreeView
                  data={run.parameterSnapshotJsonb}
                  initiallyExpanded
                  className="max-w-full"
                />
              ) : (
                <p className="text-sm text-muted-foreground italic">
                  Parameter snapshot belum tersedia (run belum dimulai).
                </p>
              )}
            </CardContent>
          </CollapsibleContent>
        </Card>
      </Collapsible>

      {/* Results section */}
      {run.status !== "DRAFT" && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Hasil ECL</CardTitle>
          </CardHeader>
          <CardContent className="p-0">
            <Tabs value={activeTab} onValueChange={setActiveTab}>
              <div className="px-4">
                <TabsList className="flex-wrap h-auto gap-1">
                  <TabsTrigger value="all">
                    Semua ({run.processedCount ?? 0})
                  </TabsTrigger>
                  <TabsTrigger value="stage1">Stage 1</TabsTrigger>
                  <TabsTrigger value="stage2">Stage 2</TabsTrigger>
                  <TabsTrigger value="stage3">Stage 3</TabsTrigger>
                  {(run.errorCount ?? 0) > 0 && (
                    <TabsTrigger value="errors">
                      <span className="flex items-center gap-1">
                        Error
                        <Badge variant="destructive" className="text-xs h-4 px-1">
                          {run.errorCount}
                        </Badge>
                      </span>
                    </TabsTrigger>
                  )}
                  <TabsTrigger value="skipped">
                    Di-skip FVTPL ({run.skippedFvtplCount ?? 0})
                  </TabsTrigger>
                </TabsList>
              </div>

              {["all", "stage1", "stage2", "stage3", "errors", "skipped"].map(
                (tabVal) => (
                  <TabsContent key={tabVal} value={tabVal} className="mt-0">
                    <DataTable
                      columns={resultColumns}
                      data={resultsData?.data ?? []}
                      pagination={resultsData?.pagination}
                      isLoading={resultsLoading}
                      searchValue={resultSearch}
                      onSearchChange={setResultSearch}
                      searchPlaceholder="Cari kode instrumen..."
                      activeFilters={resultActiveFilters}
                      onRemoveFilter={(key) => {
                        if (key === "routing_path") setResultRouteFilter("");
                      }}
                      onClearFilters={() => setResultRouteFilter("")}
                      onExport={handleExportResults}
                      onNextPage={handleResultNextPage}
                      onPrevPage={handleResultPrevPage}
                      canPrevPage={resultPage > 1}
                      pageNumber={resultPage}
                      emptyMessage="Tidak ada instrumen di tab ini."
                    />
                  </TabsContent>
                ),
              )}
            </Tabs>
          </CardContent>
        </Card>
      )}

      {/* Roll-Forward link */}
      {(isCompleted || isCompletedWithErrors || isSealed) && (
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => router.push(`/ecl/calc-runs/${calcRunId}/roll-forward`)}
          >
            Lihat Roll-Forward
          </Button>
        </div>
      )}

      {/* Dialogs */}
      <StartCalcRunDialog
        calcRunId={calcRunId}
        periodeLabel={run.periodeLabel ?? run.periodeId}
        open={startDialogOpen}
        onOpenChange={setStartDialogOpen}
        onStarted={(jobId, url) => {
          setActiveJobId(jobId);
          setStreamUrl(url);
          void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] });
        }}
      />

      <CancelCalcRunDialog
        calcRunId={calcRunId}
        processedCount={run.processedCount}
        open={cancelDialogOpen}
        onOpenChange={setCancelDialogOpen}
      />

      {/* Seal Workflow Panel (modals) */}
      <SealWorkflowPanel
        calcRunId={calcRunId}
        calcRunLabel={run.periodeLabel ?? calcRunId}
        onSealed={() =>
          void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] })
        }
        onSealRequested={() =>
          void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] })
        }
        onSealRejected={() =>
          void queryClient.invalidateQueries({ queryKey: ["calc-run", calcRunId] })
        }
      />
    </div>
  );
}
