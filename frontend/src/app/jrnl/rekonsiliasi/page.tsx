"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";
import { CalendarDays, PlayCircle, History, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { ReconSummaryCard } from "@/components/blips/gl-delivery/ReconSummaryCard";
import { glReconciliationApi } from "@/lib/api/gl-delivery.api";
import {
  runReconciliationRequestSchema,
  type RunReconciliationRequest,
} from "@/lib/schemas/gl-delivery.schema";
import { usePermissions } from "@/lib/stores/auth.store";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Page (S4-AC1, S4-AC2, S4-AC3, S4-AC4)
// ---------------------------------------------------------------------------

export default function RekonsiliasiPage() {
  const router = useRouter();
  const qc = useQueryClient();
  const { can } = usePermissions();

  const canRunRecon = can("jurnal.gl_reconciliation.run");

  // Default to today in Asia/Jakarta
  const todayJakarta = new Date().toLocaleDateString("en-CA", {
    timeZone: "Asia/Jakarta",
  });

  const [selectedDate, setSelectedDate] = React.useState(todayJakarta);
  const [submitting, setSubmitting] = React.useState(false);
  const [asyncJobId, setAsyncJobId] = React.useState<string | null>(null);

  const form = useForm<RunReconciliationRequest>({
    resolver: zodResolver(runReconciliationRequestSchema),
    defaultValues: {
      date: todayJakarta,
    },
  });

  // Load daily report for selectedDate
  const {
    data: reportData,
    isLoading: reportLoading,
    refetch: refetchReport,
  } = useQuery({
    queryKey: ["gl-recon", "daily", selectedDate],
    queryFn: () => glReconciliationApi.getDaily(selectedDate),
    // If no report yet, we get a 404 or null — handle gracefully
    retry: false,
    staleTime: 30_000,
  });

  const report = reportData?.data ?? null;

  const handleRunRecon = async (data: RunReconciliationRequest) => {
    if (!canRunRecon) return;
    setSubmitting(true);
    setAsyncJobId(null);
    try {
      const res = await glReconciliationApi.run(data, uuidv4());
      const jobId = res.data.jobId;
      setAsyncJobId(jobId);
      notify.success(
        `Rekonsiliasi untuk ${data.date} berhasil dimulai. Pantau progress di bawah.`,
      );
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleJobComplete = () => {
    // Refresh daily report when job finishes
    void qc.invalidateQueries({ queryKey: ["gl-recon", "daily", selectedDate] });
    void refetchReport();
  };

  return (
    <div className="mx-auto max-w-5xl px-6 py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Rekonsiliasi GL Host</h1>
          <p className="text-sm text-muted-foreground">
            Bandingkan jurnal BLIPS dengan GL Host dan temukan selisih
          </p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => router.push("/jrnl/rekonsiliasi/riwayat")}
          aria-label="Lihat riwayat rekonsiliasi"
        >
          <History className="mr-1.5 h-4 w-4" aria-hidden="true" />
          Riwayat Rekon
        </Button>
      </div>

      {/* Date selector + run button */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Pilih Tanggal Rekonsiliasi</CardTitle>
          <CardDescription className="text-xs">
            Pilih tanggal dan jalankan rekonsiliasi antara BLIPS dan GL Host.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form
            onSubmit={form.handleSubmit(handleRunRecon)}
            className="flex items-end gap-4"
          >
            <div className="space-y-1">
              <Label htmlFor="recon-date" className="text-xs">
                Tanggal Rekon
              </Label>
              <div className="flex items-center gap-2">
                <CalendarDays className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
                <Input
                  id="recon-date"
                  type="date"
                  className="h-8 w-40"
                  aria-label="Tanggal rekonsiliasi"
                  {...form.register("date", {
                    onChange: (e: React.ChangeEvent<HTMLInputElement>) =>
                      setSelectedDate(e.target.value),
                  })}
                />
              </div>
              {form.formState.errors.date && (
                <p className="text-xs text-destructive" aria-live="polite">
                  {form.formState.errors.date.message}
                </p>
              )}
            </div>

            {/* Refresh report */}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => refetchReport()}
              disabled={reportLoading}
              aria-label="Refresh laporan rekonsiliasi"
            >
              <RefreshCw
                className={cn("h-4 w-4", reportLoading && "animate-spin")}
                aria-hidden="true"
              />
            </Button>

            {/* Run recon — only for authorized users */}
            {canRunRecon && (
              <Button
                type="submit"
                size="sm"
                disabled={submitting}
                aria-label={`Jalankan rekonsiliasi untuk tanggal ${selectedDate}`}
              >
                {submitting ? (
                  <>
                    <RefreshCw className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
                    Memulai...
                  </>
                ) : (
                  <>
                    <PlayCircle className="mr-2 h-4 w-4" aria-hidden="true" />
                    Jalankan Rekonsiliasi
                  </>
                )}
              </Button>
            )}
          </form>
        </CardContent>
      </Card>

      {/* Job progress (§3 pattern) — shown after triggering */}
      {asyncJobId && (
        <JobProgressPanel
          jobId={asyncJobId}
          title={`Rekonsiliasi GL — ${selectedDate}`}
          onComplete={handleJobComplete}
          onFail={(err) => {
            const e = err as { message?: string; traceId?: string } | null;
            notify.error({
              code: "RECON_JOB_FAILED",
              message: `Rekonsiliasi ${selectedDate} gagal: ${e?.message ?? "Lihat log job untuk detail."}`,
              traceId: e?.traceId ?? "",
            });
          }}
          showCancel={false}
        />
      )}

      {/* Daily report card */}
      {reportLoading ? (
        <div className="space-y-2 animate-pulse">
          <div className="h-6 w-48 rounded bg-muted" />
          <div className="h-32 rounded bg-muted" />
        </div>
      ) : report ? (
        <ReconSummaryCard report={report} />
      ) : !asyncJobId ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center justify-center py-10 text-muted-foreground gap-2">
            <CalendarDays className="h-8 w-8 opacity-40" aria-hidden="true" />
            <p className="text-sm">
              Tidak ada laporan rekonsiliasi untuk{" "}
              <span className="font-medium">{selectedDate}</span>.
            </p>
            {canRunRecon && (
              <p className="text-xs">Klik &ldquo;Jalankan Rekonsiliasi&rdquo; untuk memulai.</p>
            )}
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
