"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { v4 as uuidv4 } from "uuid";
import {
  FileSpreadsheet,
  Upload,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Loader2,
} from "lucide-react";

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";

import { pdPefindoApi } from "@/lib/api/pd-pefindo.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import {
  uploadXlsxSchema,
  type UploadXlsxInput,
  type JobStatus,
  type UploadJobResult,
} from "@/lib/schemas/pd-pefindo.schema";

// ---------------------------------------------------------------------------
// Progress panel
// ---------------------------------------------------------------------------

interface JobState {
  jobId: string;
  status: JobStatus;
  progress: number;
  currentStep: string | null;
  startedAt: string | null;
  estimatedCompletionAt: string | null;
  result: UploadJobResult | null;
  error: { code: string; message: string } | null;
  canCancel: boolean;
}

function useJobProgress(
  jobId: string | null,
  onComplete: (result: UploadJobResult) => void,
  onFail: (err: { code: string; message: string }) => void,
) {
  const [state, setState] = React.useState<JobState | null>(null);
  const intervalRef = React.useRef<ReturnType<typeof setInterval> | null>(null);

  const stopPolling = React.useCallback(() => {
    if (intervalRef.current) {
      clearInterval(intervalRef.current);
      intervalRef.current = null;
    }
  }, []);

  React.useEffect(() => {
    if (!jobId) return;

    const poll = async () => {
      try {
        const res = await pdPefindoApi.getJobStatus(jobId);
        const job = res.data;
        setState({
          jobId: job.jobId,
          status: job.status,
          progress: job.progress,
          currentStep: job.currentStep,
          startedAt: job.startedAt,
          estimatedCompletionAt: job.estimatedCompletionAt,
          result: job.result,
          error: job.error,
          canCancel: job.canCancel,
        });

        if (job.status === "completed") {
          stopPolling();
          if (job.result) onComplete(job.result);
        } else if (job.status === "failed" || job.status === "cancelled") {
          stopPolling();
          onFail(
            job.error ?? { code: "INTERNAL", message: "Job gagal tanpa detail error." },
          );
        }
      } catch {
        // Keep polling — transient network errors
      }
    };

    // Initial poll immediately
    void poll();

    // Poll every 2 seconds per UX rule §3.3
    intervalRef.current = setInterval(() => void poll(), 2000);

    return () => stopPolling();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jobId]);

  return { state, stopPolling };
}

// ---------------------------------------------------------------------------
// Progress panel display
// ---------------------------------------------------------------------------

function formatTime(iso: string | null): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleTimeString("id-ID", {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  } catch {
    return iso;
  }
}

function StatusIcon({ status }: { status: JobStatus }) {
  if (status === "completed") {
    return (
      <CheckCircle2
        className="h-5 w-5 text-green-600"
        aria-hidden
      />
    );
  }
  if (status === "failed" || status === "cancelled") {
    return <XCircle className="h-5 w-5 text-destructive" aria-hidden />;
  }
  return (
    <Loader2
      className="h-5 w-5 animate-spin text-primary"
      aria-hidden
    />
  );
}

function JobProgressPanel({
  state,
}: {
  state: JobState;
}) {
  const statusLabels: Record<JobStatus, string> = {
    queued: "Antrian",
    running: "Sedang Diproses",
    completed: "Selesai",
    failed: "Gagal",
    cancelled: "Dibatalkan",
  };

  return (
    <div
      role="status"
      aria-live="polite"
      aria-label="Status upload XLSX Pefindo"
      className="rounded-lg border p-6 space-y-4"
    >
      <div className="flex items-center gap-3">
        <StatusIcon status={state.status} />
        <div>
          <p className="font-semibold">
            Upload XLSX Pefindo — {statusLabels[state.status]}
          </p>
          <p className="text-xs text-muted-foreground">
            Job ID: <code className="font-mono">{state.jobId}</code>
          </p>
        </div>
      </div>

      {/* Native progress bar — Radix Progress not installed */}
      <div
        role="progressbar"
        aria-valuenow={state.progress}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`Progress: ${state.progress}%`}
        className="h-3 w-full overflow-hidden rounded-full bg-muted"
      >
        <div
          className="h-full bg-primary transition-all duration-500"
          style={{ width: `${state.progress}%` }}
        />
      </div>
      <p className="text-sm text-right text-muted-foreground tabular-nums">
        {state.progress}%
      </p>

      {state.currentStep && (
        <p className="text-sm text-muted-foreground">{state.currentStep}</p>
      )}

      <div className="flex gap-6 text-xs text-muted-foreground">
        <span>
          Mulai:{" "}
          <span className="tabular-nums">{formatTime(state.startedAt)}</span>
        </span>
        {state.estimatedCompletionAt && (
          <span>
            ETA:{" "}
            <span className="tabular-nums">
              {formatTime(state.estimatedCompletionAt)}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Upload result summary
// ---------------------------------------------------------------------------

function UploadResultSummary({ result }: { result: UploadJobResult }) {
  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2 rounded-md bg-green-50 border border-green-200 px-4 py-3">
        <CheckCircle2 className="h-5 w-5 text-green-600 shrink-0" aria-hidden />
        <div className="text-sm text-green-800">
          <p className="font-semibold">Upload berhasil diproses</p>
          <p>
            {result.rowsCreated} record dibuat
            {result.rowsSkipped > 0 ? `, ${result.rowsSkipped} dilewati` : ""}.
          </p>
        </div>
      </div>

      {result.errors.length > 0 && (
        <div className="rounded-md border border-amber-300 bg-amber-50 p-4 space-y-2">
          <div className="flex items-center gap-2">
            <AlertTriangle className="h-4 w-4 text-amber-600" aria-hidden />
            <p className="text-sm font-semibold text-amber-800">
              {result.errors.length} baris mengalami error:
            </p>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b">
                  <th className="text-left px-2 py-1 font-medium text-muted-foreground">
                    Baris
                  </th>
                  <th className="text-left px-2 py-1 font-medium text-muted-foreground">
                    Rating
                  </th>
                  <th className="text-left px-2 py-1 font-medium text-muted-foreground">
                    Pesan Error
                  </th>
                </tr>
              </thead>
              <tbody>
                {result.errors.map((e, i) => (
                  <tr key={i} className="border-b last:border-0">
                    <td className="px-2 py-1 tabular-nums">{e.row}</td>
                    <td className="px-2 py-1 font-mono">{e.rating || "—"}</td>
                    <td className="px-2 py-1 text-destructive">{e.message}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

type PagePhase = "form" | "uploading" | "polling" | "done" | "failed";

export default function UploadXlsxPage() {
  const router = useRouter();
  const [phase, setPhase] = React.useState<PagePhase>("form");
  const [activeJobId, setActiveJobId] = React.useState<string | null>(null);
  const [uploadResult, setUploadResult] = React.useState<UploadJobResult | null>(null);
  const [failError, setFailError] = React.useState<{
    code: string;
    message: string;
  } | null>(null);
  const [selectedFile, setSelectedFile] = React.useState<File | null>(null);
  const fileInputRef = React.useRef<HTMLInputElement>(null);

  const form = useForm<UploadXlsxInput>({
    resolver: zodResolver(uploadXlsxSchema),
    defaultValues: {
      tanggalPublikasi: "",
      periodeBerlakuDari: "",
      periodeBerlakuSampai: "",
    },
  });

  const { state: jobState } = useJobProgress(
    activeJobId,
    (result) => {
      setUploadResult(result);
      setPhase("done");
      notify.success(
        `Upload selesai: ${result.rowsCreated} record PD Pefindo berhasil dibuat.`,
        {
          action: {
            label: "Lihat daftar",
            onClick: () => router.push("/master/pd-pefindo"),
          },
        },
      );
    },
    (err) => {
      setFailError(err);
      setPhase("failed");
      notify.error({
        code: err.code,
        message: err.message,
        traceId: "",
      });
    },
  );

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] ?? null;
    setSelectedFile(file);
    if (file && !file.name.toLowerCase().endsWith(".xlsx")) {
      form.setError("root", {
        message: "File harus berformat XLSX (.xlsx)",
      });
    } else {
      form.clearErrors("root");
    }
  };

  const onSubmit = async (values: UploadXlsxInput) => {
    if (!selectedFile) {
      form.setError("root", { message: "Pilih file XLSX terlebih dahulu." });
      return;
    }

    if (selectedFile.size > 10 * 1024 * 1024) {
      form.setError("root", {
        message: "File terlalu besar. Maksimal 10 MB.",
      });
      return;
    }

    setPhase("uploading");
    const idempotencyKey = uuidv4();

    try {
      const res = await pdPefindoApi.uploadXlsx(
        selectedFile,
        values.periodeBerlakuDari,
        values.tanggalPublikasi,
        values.periodeBerlakuSampai || undefined,
        idempotencyKey,
      );

      setActiveJobId(res.data.jobId);
      setPhase("polling");
      notify.info(
        `File diterima. Memproses upload (Job ${res.data.jobId.slice(0, 8)}...)`,
      );
    } catch (err) {
      setPhase("form");
      if (isApiError(err)) {
        notify.error(err);
        err.details.forEach((d) => {
          form.setError("root", { message: `${d.field}: ${d.message}` });
        });
      } else {
        notify.error({
          code: "INTERNAL",
          message: "Gagal mengirim file. Coba lagi.",
          traceId: "",
        });
      }
    }
  };

  const handleReset = () => {
    setPhase("form");
    setActiveJobId(null);
    setUploadResult(null);
    setFailError(null);
    setSelectedFile(null);
    form.reset();
    if (fileInputRef.current) fileInputRef.current.value = "";
  };

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-6">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/pd-pefindo" className="hover:underline">
          PD Pefindo
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Upload XLSX Pefindo</span>
      </nav>

      <div className="flex items-center gap-3">
        <FileSpreadsheet className="h-6 w-6 text-primary" aria-hidden />
        <h1 className="text-2xl font-semibold">Upload XLSX Pefindo</h1>
      </div>

      {/* Instruction */}
      <div className="rounded-md border bg-muted/30 px-4 py-3 text-sm text-muted-foreground space-y-1">
        <p className="font-medium text-foreground">Format XLSX yang diharapkan:</p>
        <ul className="list-disc list-inside space-y-0.5">
          <li>
            Kolom: <code>rating, pd_12month, pd_lifetime_3y, pd_lifetime_5y, pd_lifetime_7y, pd_lifetime_10y</code>
          </li>
          <li>Rating harus salah satu dari 20 nilai Pefindo (idAAA s/d idD)</li>
          <li>Nilai PD desimal 0–1 (8 desimal), pd_12m ≤ 3y ≤ 5y ≤ 7y ≤ 10y</li>
          <li>Maksimal 10 MB per file</li>
        </ul>
      </div>

      <Separator />

      {/* Phase: Form */}
      {phase === "form" && (
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="space-y-6">
            <div className="rounded-lg border p-6 space-y-4">
              {/* File picker */}
              <div className="space-y-2">
                <label
                  htmlFor="xlsx-file"
                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                >
                  File XLSX{" "}
                  <span className="text-destructive" aria-hidden>
                    *
                  </span>
                </label>
                <input
                  id="xlsx-file"
                  ref={fileInputRef}
                  type="file"
                  accept=".xlsx,.xls"
                  aria-required="true"
                  aria-describedby="xlsx-file-desc"
                  className="block w-full text-sm text-muted-foreground file:mr-3 file:rounded-md file:border file:border-input file:bg-background file:px-3 file:py-1.5 file:text-sm file:font-medium file:cursor-pointer hover:file:bg-accent"
                  onChange={handleFileChange}
                />
                <p
                  id="xlsx-file-desc"
                  className="text-xs text-muted-foreground"
                >
                  {selectedFile
                    ? `Dipilih: ${selectedFile.name} (${(selectedFile.size / 1024).toFixed(1)} KB)`
                    : "Hanya .xlsx / .xls · Maksimal 10 MB"}
                </p>
                {form.formState.errors.root && (
                  <p
                    role="alert"
                    className="text-sm font-medium text-destructive"
                  >
                    {form.formState.errors.root.message}
                  </p>
                )}
              </div>

              {/* Tanggal Publikasi */}
              <FormField
                control={form.control}
                name="tanggalPublikasi"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Tanggal Publikasi{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Input type="date" {...field} aria-required="true" />
                    </FormControl>
                    <FormDescription>
                      Tanggal terbit laporan Pefindo Annual Default Study
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Periode Berlaku Dari */}
              <FormField
                control={form.control}
                name="periodeBerlakuDari"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Periode Berlaku Dari{" "}
                      <span className="text-destructive" aria-hidden>
                        *
                      </span>
                    </FormLabel>
                    <FormControl>
                      <Input type="date" {...field} aria-required="true" />
                    </FormControl>
                    <FormDescription>
                      Awal periode efektivitas PD untuk kalkulasi ECL
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Periode Berlaku Sampai */}
              <FormField
                control={form.control}
                name="periodeBerlakuSampai"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Periode Berlaku Sampai</FormLabel>
                    <FormControl>
                      <Input
                        type="date"
                        {...field}
                        value={field.value ?? ""}
                      />
                    </FormControl>
                    <FormDescription>
                      Opsional — kosongkan jika belum ada batas akhir
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>

            <div className="flex justify-end gap-3">
              <Button
                type="button"
                variant="outline"
                onClick={() => router.push("/master/pd-pefindo")}
              >
                Batal
              </Button>
              <Button
                type="submit"
                disabled={!selectedFile || form.formState.isSubmitting}
              >
                <Upload className="mr-1.5 h-4 w-4" aria-hidden />
                Upload dan Proses
              </Button>
            </div>
          </form>
        </Form>
      )}

      {/* Phase: Uploading (HTTP request in progress) */}
      {phase === "uploading" && (
        <div className="flex flex-col items-center gap-4 py-10">
          <Loader2 className="h-8 w-8 animate-spin text-primary" aria-hidden />
          <p className="text-sm text-muted-foreground">
            Mengirim file ke server...
          </p>
        </div>
      )}

      {/* Phase: Polling (Asynq job running) */}
      {phase === "polling" && jobState && (
        <div className="space-y-4">
          <JobProgressPanel state={jobState} />
          <p className="text-xs text-center text-muted-foreground">
            Anda dapat menutup halaman ini. Job akan tetap berjalan di
            background — notifikasi akan muncul saat selesai.
          </p>
        </div>
      )}

      {/* Phase: Done */}
      {phase === "done" && uploadResult && (
        <div className="space-y-6">
          <UploadResultSummary result={uploadResult} />
          <div className="flex justify-end gap-3">
            <Button variant="outline" onClick={handleReset}>
              Upload File Lain
            </Button>
            <Button asChild>
              <Link href="/master/pd-pefindo">Lihat di Daftar</Link>
            </Button>
          </div>
        </div>
      )}

      {/* Phase: Failed */}
      {phase === "failed" && failError && (
        <div className="space-y-4">
          <div className="flex items-start gap-3 rounded-md border border-destructive bg-destructive/5 px-4 py-3">
            <XCircle
              className="mt-0.5 h-5 w-5 shrink-0 text-destructive"
              aria-hidden
            />
            <div className="text-sm">
              <p className="font-semibold text-destructive">Upload gagal</p>
              <p className="text-muted-foreground">
                {failError.message} ({failError.code})
              </p>
            </div>
          </div>
          <div className="flex justify-end gap-3">
            <Button variant="outline" onClick={handleReset}>
              Coba Lagi
            </Button>
            <Button variant="outline" asChild>
              <Link href="/master/pd-pefindo">Kembali ke Daftar</Link>
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
