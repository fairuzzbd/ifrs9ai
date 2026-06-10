"use client";

import * as React from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import Link from "next/link";
import { v4 as uuidv4 } from "uuid";
import { FileSpreadsheet, CheckCircle2, XCircle, AlertTriangle } from "lucide-react";

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
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

import { coaApi } from "@/lib/api/coa.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import { coaImportSchema, type CoAImportInput, type CoAImportJob } from "@/lib/schemas/coa.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Progress bar
// ---------------------------------------------------------------------------

function ProgressBar({
  value,
  className,
}: {
  value: number;
  className?: string;
}) {
  return (
    <div
      className={cn("h-3 w-full overflow-hidden rounded-full bg-muted", className)}
      role="progressbar"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-label={`Progress ${value}%`}
    >
      <div
        className="h-full bg-primary transition-all duration-500 ease-out"
        style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Job result summary
// ---------------------------------------------------------------------------

function JobResultSummary({ job }: { job: CoAImportJob }) {
  if (job.status === "completed" && job.result) {
    const { rowsCreated, rowsSkipped, errors } = job.result;
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-green-700">
          <CheckCircle2 className="h-5 w-5 shrink-0" aria-hidden />
          <span className="font-medium">Import selesai</span>
        </div>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <div className="rounded-lg border bg-green-50 p-3 text-center">
            <div className="text-2xl font-bold text-green-700">{rowsCreated}</div>
            <div className="text-xs text-green-600 mt-0.5">Akun dibuat</div>
          </div>
          <div className="rounded-lg border bg-amber-50 p-3 text-center">
            <div className="text-2xl font-bold text-amber-700">{rowsSkipped}</div>
            <div className="text-xs text-amber-600 mt-0.5">Baris dilewati</div>
          </div>
          <div className="rounded-lg border bg-red-50 p-3 text-center">
            <div className="text-2xl font-bold text-red-700">{errors.length}</div>
            <div className="text-xs text-red-600 mt-0.5">Error</div>
          </div>
        </div>

        {errors.length > 0 && (
          <div className="space-y-2">
            <p className="text-sm font-medium text-destructive flex items-center gap-1.5">
              <AlertTriangle className="h-4 w-4" aria-hidden />
              Detail error
            </p>
            <div className="max-h-48 overflow-y-auto rounded-md border bg-muted/30">
              <table className="w-full text-xs">
                <thead className="sticky top-0 bg-muted">
                  <tr>
                    <th className="px-3 py-2 text-left font-medium">Baris</th>
                    <th className="px-3 py-2 text-left font-medium">Kode Akun</th>
                    <th className="px-3 py-2 text-left font-medium">Alasan</th>
                  </tr>
                </thead>
                <tbody>
                  {errors.map((e, i) => (
                    <tr key={i} className="border-t">
                      <td className="px-3 py-1.5 font-mono">{e.row}</td>
                      <td className="px-3 py-1.5 font-mono">{e.kodeAkun || "—"}</td>
                      <td className="px-3 py-1.5 text-destructive">{e.reason}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}

        <div className="pt-2">
          <Button asChild>
            <Link href="/master/coa">Lihat Daftar Chart of Accounts</Link>
          </Button>
        </div>
      </div>
    );
  }

  if (job.status === "failed" && job.error) {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-destructive">
          <XCircle className="h-5 w-5 shrink-0" aria-hidden />
          <span className="font-medium">Import gagal</span>
        </div>
        <p className="text-sm text-muted-foreground">{job.error.message}</p>
        <p className="text-xs text-muted-foreground font-mono">
          Kode: {job.error.code}
        </p>
      </div>
    );
  }

  return null;
}

// ---------------------------------------------------------------------------
// Import page
// ---------------------------------------------------------------------------

type ImportPhase = "idle" | "uploading" | "polling" | "done";

export default function CoAImportPage() {
  const [phase, setPhase] = React.useState<ImportPhase>("idle");
  const [jobId, setJobId] = React.useState<string | null>(null);
  const [jobState, setJobState] = React.useState<CoAImportJob | null>(null);
  const pollingRef = React.useRef<ReturnType<typeof setInterval> | null>(null);

  const form = useForm<CoAImportInput>({
    resolver: zodResolver(coaImportSchema),
    defaultValues: {
      sumberCoa: "",
    },
  });

  // ---------------------------------------------------------------------------
  // Polling
  // ---------------------------------------------------------------------------

  const startPolling = React.useCallback((id: string) => {
    if (pollingRef.current) clearInterval(pollingRef.current);

    pollingRef.current = setInterval(async () => {
      try {
        const res = await coaApi.getImportJobStatus(id);
        const job = res.data;
        setJobState(job);

        if (job.status === "completed" || job.status === "failed" || job.status === "cancelled") {
          if (pollingRef.current) clearInterval(pollingRef.current);
          setPhase("done");

          if (job.status === "completed" && job.result) {
            notify.success(
              `Import selesai. ${job.result.rowsCreated} akun dibuat, ${job.result.errors.length} error.`,
              job.result.errors.length === 0
                ? {
                    action: {
                      label: "Lihat CoA",
                      onClick: () => window.location.assign("/master/coa"),
                    },
                  }
                : undefined,
            );
          } else if (job.status === "failed") {
            notify.error({
              code: job.error?.code ?? "IMPORT_FAILED",
              message: job.error?.message ?? "Import gagal.",
              traceId: "",
            });
          }
        }
      } catch {
        // Network hiccup — keep polling
      }
    }, 2000);
  }, []);

  // Cleanup on unmount
  React.useEffect(() => {
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current);
    };
  }, []);

  // ---------------------------------------------------------------------------
  // Submit
  // ---------------------------------------------------------------------------

  const onSubmit = async (values: CoAImportInput) => {
    setPhase("uploading");
    const idempotencyKey = uuidv4();

    try {
      const res = await coaApi.importXlsx(
        values.file as File,
        values.sumberCoa,
        idempotencyKey,
      );

      const newJobId = res.data.jobId;
      setJobId(newJobId);
      setPhase("polling");
      notify.info("File berhasil diunggah. Memproses import...");

      // Fetch initial status
      try {
        const statusRes = await coaApi.getImportJobStatus(newJobId);
        setJobState(statusRes.data);
      } catch {
        // Ignore — polling will pick it up
      }

      startPolling(newJobId);
    } catch (err) {
      setPhase("idle");
      if (isApiError(err)) {
        notify.error(err);
      } else {
        notify.error({
          code: "INTERNAL",
          message: "Gagal mengunggah file. Coba lagi.",
          traceId: "",
        });
      }
    }
  };

  const isUploading = phase === "uploading";
  const isPolling = phase === "polling";
  const isDone = phase === "done";
  const isActive = isUploading || isPolling;

  const progressValue = isPolling && jobState ? jobState.progress : isUploading ? 5 : 0;
  const currentStep =
    isUploading
      ? "Mengunggah file..."
      : isPolling && jobState?.currentStep
        ? jobState.currentStep
        : isPolling
          ? "Memproses..."
          : null;

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/coa" className="hover:underline">
          Chart of Accounts
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Import XLSX</span>
      </nav>
      <h1 className="text-2xl font-semibold">Import Chart of Accounts dari XLSX</h1>

      {/* Info */}
      <Card className="border-blue-200 bg-blue-50">
        <CardContent className="pt-4 pb-4">
          <div className="flex gap-2 text-sm text-blue-800">
            <FileSpreadsheet className="h-4 w-4 shrink-0 mt-0.5" aria-hidden />
            <div className="space-y-1">
              <p className="font-medium">Format file XLSX yang diharapkan:</p>
              <ul className="list-disc list-inside space-y-0.5 text-blue-700">
                <li>Kolom: kode_akun, nama_akun, tipe_akun, sub_tipe_akun, kategori_investasi, mata_uang_native, posisi_normal, aktif, parent_kode_akun, tanggal_mulai_aktif</li>
                <li>tipe_akun: ASET / LIABILITAS / EKUITAS / PENDAPATAN / BEBAN / KONTINJEN</li>
                <li>posisi_normal: DEBIT / KREDIT</li>
                <li>Format kode_akun: angka bertingkat dengan titik, mis: 1.1.01.001</li>
                <li>Ukuran maksimal: 10MB</li>
              </ul>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Form */}
      {phase === "idle" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Upload File</CardTitle>
          </CardHeader>
          <CardContent>
            <Form {...form}>
              <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="space-y-4">
                {/* File */}
                <FormField
                  control={form.control}
                  name="file"
                  render={({ field: { onChange, value: _value, ...field } }) => (
                    <FormItem>
                      <FormLabel>
                        File XLSX{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          type="file"
                          accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                          aria-required="true"
                          {...field}
                          onChange={(e) => {
                            const file = e.target.files?.[0];
                            if (file) onChange(file);
                          }}
                        />
                      </FormControl>
                      <FormDescription>
                        Format: .xlsx, maks 10MB
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {/* Sumber CoA */}
                <FormField
                  control={form.control}
                  name="sumberCoa"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>
                        Sumber CoA{" "}
                        <span className="text-destructive" aria-hidden>*</span>
                      </FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          placeholder="Mis: PSAK 71 v1.0, Import Sistem Lama"
                          aria-required="true"
                        />
                      </FormControl>
                      <FormDescription>
                        Keterangan asal data yang diimpor untuk audit trail
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className="flex justify-end gap-3 pt-2">
                  <Button type="button" variant="outline" asChild>
                    <Link href="/master/coa">Batal</Link>
                  </Button>
                  <Button type="submit">Upload &amp; Import</Button>
                </div>
              </form>
            </Form>
          </CardContent>
        </Card>
      )}

      {/* Progress panel */}
      {(isActive || isDone) && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              {isDone ? (
                jobState?.status === "completed" ? (
                  <CheckCircle2 className="h-5 w-5 text-green-600" aria-hidden />
                ) : (
                  <XCircle className="h-5 w-5 text-destructive" aria-hidden />
                )
              ) : (
                <span
                  className="h-4 w-4 rounded-full border-2 border-primary border-t-transparent animate-spin"
                  aria-hidden
                />
              )}
              {isDone
                ? jobState?.status === "completed"
                  ? "Import Selesai"
                  : "Import Gagal"
                : "Memproses Import..."}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Progress bar */}
            {!isDone && (
              <div className="space-y-2">
                <ProgressBar value={progressValue} />
                <div className="flex justify-between text-xs text-muted-foreground">
                  <span>{currentStep}</span>
                  <span>{progressValue}%</span>
                </div>
              </div>
            )}

            {/* Job ID */}
            {jobId && (
              <p className="text-xs text-muted-foreground font-mono">
                Job ID: {jobId}
              </p>
            )}

            {/* Result */}
            {isDone && jobState && <JobResultSummary job={jobState} />}

            {/* Actions while running */}
            {isActive && (
              <div className="flex items-center justify-between pt-2 border-t">
                <p className="text-xs text-muted-foreground">
                  Proses berjalan di background. Anda bisa menutup halaman ini.
                </p>
                <Button
                  variant="outline"
                  size="sm"
                  asChild
                >
                  <Link href="/master/coa">Kembali ke Daftar</Link>
                </Button>
              </div>
            )}

            {/* Retry on fail */}
            {isDone && jobState?.status === "failed" && (
              <Button
                variant="outline"
                onClick={() => {
                  setPhase("idle");
                  setJobId(null);
                  setJobState(null);
                  form.reset();
                }}
              >
                Coba Lagi
              </Button>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
