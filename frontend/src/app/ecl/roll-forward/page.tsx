"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { v4 as uuidv4 } from "uuid";

import { computeRollForwardFormSchema, type ComputeRollForwardForm } from "@/lib/schemas/roll-forward.schema";
import { rollForwardApi } from "@/lib/api/roll-forward.api";
import { calcRunApi } from "@/lib/api/calc-run.api";
import { notify } from "@/lib/notify";
import { isApiError } from "@/lib/api";
import { usePermissions } from "@/lib/stores/auth.store";
import { JobProgressPanel } from "@/components/blips/JobProgressPanel";
import { RollForwardDetectionMethodBadge } from "@/components/blips/RollForwardDetectionMethodBadge";

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Info, Loader2 } from "lucide-react";

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function GenerateRollForwardPage() {
  const router = useRouter();
  const { can } = usePermissions();

  const [submitting, setSubmitting] = React.useState(false);
  const [asyncJobId, setAsyncJobId] = React.useState<string | null>(null);
  const [idempotencyKey] = React.useState(() => uuidv4());

  const form = useForm<ComputeRollForwardForm>({
    resolver: zodResolver(computeRollForwardFormSchema),
    defaultValues: {
      currentCalcRunId: "",
      priorCalcRunId: null,
      detectionMethod: "BASIC_STATUS_DIFF",
    },
  });

  // Load eligible current runs (COMPLETED or SEALED)
  const { data: runsData, isLoading: runsLoading } = useQuery({
    queryKey: ["calc-runs-eligible-current"],
    queryFn: () =>
      calcRunApi.list({ limit: 100, sort: "created_at:desc" }),
  });

  const allRuns = runsData?.data ?? [];
  const currentEligible = allRuns.filter(
    (r) => r.status === "COMPLETED" || r.status === "COMPLETED_WITH_ERRORS" || r.status === "SEALED",
  );
  const priorEligible = allRuns.filter((r) => r.status === "SEALED");

  const selectedCurrentId = form.watch("currentCalcRunId");
  const selectedCurrentRun = currentEligible.find((r) => r.id === selectedCurrentId);

  const onSubmit = async (data: ComputeRollForwardForm) => {
    setSubmitting(true);
    try {
      const result = await rollForwardApi.compute(
        {
          currentCalcRunId: data.currentCalcRunId,
          priorCalcRunId: data.priorCalcRunId ?? null,
          options: { detectionMethod: "BASIC_STATUS_DIFF" },
        },
        idempotencyKey,
      );

      if (result.status === 202) {
        // Async — show progress panel
        const job = result.data.data;
        setAsyncJobId(job.jobId);
        notify.info(
          "Komputasi roll-forward dimulai. Laporan besar ini diproses secara async.",
        );
      } else {
        // Sync — redirect to report
        const report = result.data.data;
        notify.success(
          `Roll-forward ${report.currentPeriodeId} berhasil dihitung. Rekonsiliasi: ${report.reconcileStatus}.`,
        );
        router.push(
          `/ecl/roll-forward/${encodeURIComponent(report.reportId)}`,
        );
      }
    } catch (err) {
      if (isApiError(err)) {
        notify.error(err);
      } else {
        notify.error({
          code: "INTERNAL",
          message: "Gagal menghitung roll-forward. Coba lagi.",
          traceId: "",
        });
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleJobComplete = (result: unknown) => {
    const r = result as { reportId?: string; currentPeriodeId?: string; reconcileStatus?: string };
    if (r?.reportId) {
      notify.success(
        `Roll-forward ${r.currentPeriodeId ?? ""} selesai. Rekonsiliasi: ${r.reconcileStatus ?? "—"}.`,
        {
          action: {
            label: "Lihat laporan",
            onClick: () =>
              router.push(`/ecl/roll-forward/${encodeURIComponent(r.reportId!)}`),
          },
        },
      );
      router.push(`/ecl/roll-forward/${encodeURIComponent(r.reportId)}`);
    }
  };

  const handleJobFail = () => {
    notify.error({
      code: "INTERNAL",
      message: "Komputasi roll-forward gagal. Periksa log atau coba lagi.",
      traceId: "",
    });
    setAsyncJobId(null);
  };

  // Guard: permission (after hooks)
  if (!can("ecl.roll_forward.compute") && !can("ecl.roll_forward.read")) {
    return (
      <div className="p-6">
        <Alert variant="destructive">
          <AlertTitle>Akses Ditolak</AlertTitle>
          <AlertDescription>
            Anda tidak memiliki izin <code>ecl.roll_forward.compute</code>.
          </AlertDescription>
        </Alert>
      </div>
    );
  }

  return (
    <div className="p-6 space-y-6 max-w-2xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <ol className="flex items-center gap-1 flex-wrap">
          <li>
            <button
              className="hover:underline"
              onClick={() => router.push("/ecl/calc-runs")}
            >
              ECL Calc Runs
            </button>
          </li>
          <li aria-hidden>&rsaquo;</li>
          <li className="text-foreground">Generate Roll-Forward CKPN</li>
        </ol>
      </nav>

      {/* Header */}
      <div>
        <h1 className="text-xl font-semibold">Generate Roll-Forward CKPN</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Hitung laporan roll-forward ECL: opening → transfer stage → originasi →
          penghapusbukuan → pengukuran ulang → closing.
        </p>
      </div>

      {/* Detection method notice */}
      <Alert className="border-amber-200 bg-amber-50">
        <Info className="h-4 w-4 text-amber-600" aria-hidden="true" />
        <AlertTitle className="text-amber-800">Metode Deteksi Phase 4</AlertTitle>
        <AlertDescription className="text-amber-700 text-sm">
          Laporan ini menggunakan{" "}
          <RollForwardDetectionMethodBadge method="BASIC_STATUS_DIFF" />.
          Deteksi origination/derecognition berbasis perubahan status instrumen
          dan kehadiran di result calc run. Deteksi lifecycle penuh (penempatan,
          penjualan, jatuh tempo) akan tersedia di Phase 5.
        </AlertDescription>
      </Alert>

      {/* Async progress panel */}
      {asyncJobId && (
        <JobProgressPanel
          jobId={asyncJobId}
          title="Menghitung Roll-Forward CKPN..."
          onComplete={handleJobComplete}
          onFail={handleJobFail}
          showCancel={false}
          showBackground
        />
      )}

      {/* Form */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Parameter Laporan</CardTitle>
          <CardDescription>
            Pilih calc run saat ini dan (opsional) prior SEALED run untuk perbandingan.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(onSubmit)}
              className="space-y-5"
              aria-label="Form generate roll-forward CKPN"
            >
              {/* Current Calc Run */}
              <FormField
                control={form.control}
                name="currentCalcRunId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      Calc Run Saat Ini <span className="text-destructive">*</span>
                    </FormLabel>
                    <FormControl>
                      <Select
                        value={field.value ?? ""}
                        onValueChange={field.onChange}
                        disabled={runsLoading || submitting || !!asyncJobId}
                      >
                        <SelectTrigger aria-label="Pilih calc run saat ini">
                          <SelectValue placeholder="Pilih calc run (COMPLETED atau SEALED)..." />
                        </SelectTrigger>
                        <SelectContent>
                          {currentEligible.length === 0 && (
                            <SelectItem value="_empty" disabled>
                              Tidak ada calc run COMPLETED / SEALED
                            </SelectItem>
                          )}
                          {currentEligible.map((r) => (
                            <SelectItem key={r.id} value={r.id}>
                              {r.periodeLabel ?? r.periodeId} — {r.status} (
                              {r.id.slice(0, 8)}…)
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </FormControl>
                    <FormDescription>
                      Hanya calc run dengan status COMPLETED atau SEALED yang dapat
                      dipilih.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Prior Calc Run */}
              <FormField
                control={form.control}
                name="priorCalcRunId"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Prior Calc Run (Opsional)</FormLabel>
                    <FormControl>
                      <Select
                        value={field.value ?? "_none"}
                        onValueChange={(v) =>
                          field.onChange(v === "_none" ? null : v)
                        }
                        disabled={runsLoading || submitting || !!asyncJobId}
                      >
                        <SelectTrigger aria-label="Pilih prior calc run">
                          <SelectValue placeholder="Pilih prior SEALED run (kosong = periode pertama)..." />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="_none">
                            — Periode pertama (opening = 0)
                          </SelectItem>
                          {priorEligible
                            .filter((r) => r.id !== selectedCurrentId)
                            .map((r) => (
                              <SelectItem key={r.id} value={r.id}>
                                {r.periodeLabel ?? r.periodeId} — SEALED (
                                {r.id.slice(0, 8)}…)
                              </SelectItem>
                            ))}
                          {priorEligible.filter(
                            (r) => r.id !== selectedCurrentId,
                          ).length === 0 && (
                            <SelectItem value="_no_sealed" disabled>
                              Tidak ada calc run SEALED lain tersedia
                            </SelectItem>
                          )}
                        </SelectContent>
                      </Select>
                    </FormControl>
                    <FormDescription>
                      Kosongkan jika ini adalah periode pertama (saldo awal = 0,
                      semua instrumen = Originasi Baru).
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {/* Detection method (locked) */}
              <FormItem>
                <FormLabel>Metode Deteksi</FormLabel>
                <div className="flex items-center gap-2 mt-1">
                  <RollForwardDetectionMethodBadge method="BASIC_STATUS_DIFF" />
                  <span className="text-xs text-muted-foreground">
                    (dikunci di Phase 4)
                  </span>
                </div>
                <FormDescription>
                  Metode FULL_LIFECYCLE_PHASE_5 akan tersedia setelah integrasi
                  APP-B selesai.
                </FormDescription>
              </FormItem>

              {/* Info for first period */}
              {!form.watch("priorCalcRunId") && selectedCurrentRun && (
                <Alert className="py-2">
                  <AlertDescription className="text-sm">
                    Mode periode pertama: Saldo Awal = 0, semua instrumen dicatat
                    sebagai Originasi Baru.
                  </AlertDescription>
                </Alert>
              )}

              {/* Submit */}
              <div className="flex gap-3 pt-1">
                <Button
                  type="submit"
                  disabled={submitting || !!asyncJobId}
                  aria-label="Hitung roll-forward CKPN"
                >
                  {submitting ? (
                    <>
                      <Loader2 className="h-4 w-4 mr-2 animate-spin" aria-hidden="true" />
                      Menghitung...
                    </>
                  ) : (
                    "Hitung Roll-Forward"
                  )}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => router.back()}
                  disabled={submitting || !!asyncJobId}
                >
                  Batal
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  );
}
