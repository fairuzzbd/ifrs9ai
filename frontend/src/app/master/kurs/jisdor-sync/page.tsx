"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Radio, Calendar, Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePermissions } from "@/lib/stores/auth.store";
import {
  JisdorSyncTriggerButton,
  JisdorJobProgressPanel,
} from "@/components/blips/fx-rate";
import { kursListApi, fxRateQueryKeys } from "@/lib/api/fx-rate.api";
import type { JisdorSyncJobResponse } from "@/lib/schemas/fx-rate.schema";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// JISDOR sync history table (last 10 fetches)
// ---------------------------------------------------------------------------

function JisdorSyncHistoryCard() {
  const { data, isLoading, isError } = useQuery({
    queryKey: fxRateQueryKeys.list({
      "filter[sumber_kurs]": "BI_JISDOR",
      sort: "tanggal_berlaku:desc",
      limit: 10,
    }),
    queryFn: () =>
      kursListApi.list({
        "filter[sumber_kurs]": "BI_JISDOR",
        sort: "tanggal_berlaku:desc",
        limit: 10,
      }),
    staleTime: 60_000,
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">Riwayat Fetch JISDOR (10 terbaru)</CardTitle>
        <CardDescription>
          Menampilkan 10 baris kurs terakhir yang berasal dari BI JISDOR.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="space-y-2">
            {Array.from({ length: 4 }).map((_, i) => (
              <div
                key={i}
                className="h-8 rounded bg-muted animate-pulse"
                aria-hidden="true"
              />
            ))}
          </div>
        )}
        {isError && (
          <p className="text-sm text-destructive">
            Gagal memuat riwayat JISDOR. Coba muat ulang halaman.
          </p>
        )}
        {!isLoading && !isError && (
          <div className="overflow-x-auto">
            <table className="w-full text-sm" role="grid" aria-label="Riwayat JISDOR">
              <thead>
                <tr className="border-b text-left text-muted-foreground text-xs">
                  <th className="pb-2 pr-3 font-medium">Tanggal</th>
                  <th className="pb-2 pr-3 font-medium">Mata Uang</th>
                  <th className="pb-2 pr-3 font-medium text-right">Kurs Tengah</th>
                  <th className="pb-2 pr-3 font-medium">Status</th>
                  <th className="pb-2 font-medium">Terkunci</th>
                </tr>
              </thead>
              <tbody>
                {(data?.data ?? []).length === 0 ? (
                  <tr>
                    <td colSpan={5} className="py-4 text-center text-muted-foreground">
                      Belum ada data JISDOR.
                    </td>
                  </tr>
                ) : (
                  (data?.data ?? []).map((row) => (
                    <tr key={row.id} className="border-b last:border-0 hover:bg-muted/30">
                      <td className="py-1.5 pr-3 font-mono text-xs">{row.tanggalBerlaku}</td>
                      <td className="py-1.5 pr-3">
                        <code className="font-mono font-bold">{row.kodeMataUang}</code>
                      </td>
                      <td className="py-1.5 pr-3 text-right font-mono text-xs">
                        {new Intl.NumberFormat("id-ID", {
                          minimumFractionDigits: 4,
                          maximumFractionDigits: 4,
                        }).format(row.kursTengah)}
                      </td>
                      <td className="py-1.5 pr-3">
                        <span
                          className={cn(
                            "inline-flex items-center rounded-md border px-1.5 py-0.5 text-xs font-medium",
                            row.workflowStatus === "APPROVED"
                              ? "bg-green-50 text-green-700 border-green-300"
                              : row.workflowStatus === "PENDING_APPROVAL"
                              ? "bg-amber-50 text-amber-700 border-amber-300"
                              : "bg-red-50 text-red-700 border-red-300",
                          )}
                        >
                          {row.workflowStatus === "APPROVED"
                            ? "Aktif"
                            : row.workflowStatus === "PENDING_APPROVAL"
                            ? "Menunggu"
                            : "Ditolak"}
                        </span>
                      </td>
                      <td className="py-1.5 text-xs text-muted-foreground">
                        {row.lockedFlag ? "Ya" : "Tidak"}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        )}

        {data && data.pagination.hasMore && (
          <p className="mt-2 text-xs text-muted-foreground">
            Hanya menampilkan 10 terakhir.{" "}
            <Link
              href="/master/kurs?filter[sumber_kurs]=BI_JISDOR"
              className="underline hover:no-underline"
            >
              Lihat semua
            </Link>
          </p>
        )}
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Page content (S1-AC1..4, persona-gated)
// ---------------------------------------------------------------------------

function JisdorSyncContent() {
  const perms = usePermissions();
  const [activeJob, setActiveJob] = React.useState<JisdorSyncJobResponse | null>(null);
  const [tanggalTarget, setTanggalTarget] = React.useState("");

  // Guard: entire page absent for non-ROLE-IT-ADMIN
  if (!perms.can("kurs.sync")) {
    return (
      <div className="container mx-auto py-6">
        <div className="rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          Halaman ini hanya dapat diakses oleh ROLE-IT-ADMIN. Anda tidak memiliki
          permission <code>kurs.sync</code>.
        </div>
      </div>
    );
  }

  return (
    <div className="container mx-auto py-6 space-y-5 max-w-3xl">
      {/* Breadcrumb */}
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/kurs" className="hover:text-foreground transition-colors">
          Master Data / Kurs
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">JISDOR Sync</span>
      </nav>

      <div className="flex items-center gap-3">
        <Button variant="outline" size="sm" asChild>
          <Link href="/master/kurs" aria-label="Kembali ke daftar kurs">
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            Kembali
          </Link>
        </Button>
        <h1 className="text-2xl font-semibold flex items-center gap-2">
          <Radio className="h-6 w-6 text-primary" aria-hidden="true" />
          BI JISDOR Sync Admin Panel
        </h1>
      </div>

      {/* Info banner */}
      <div className="flex items-start gap-2 rounded-md border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800">
        <Info className="mt-0.5 h-4 w-4 shrink-0 text-blue-600" aria-hidden="true" />
        <div className="space-y-1">
          <p>
            <strong>Cron otomatis</strong>: JISDOR di-fetch setiap hari kerja (Senin–Jumat)
            pukul 10:30 WIB via Asynq scheduler. Trigger manual ini digunakan jika cron
            terlewat atau perlu backfill tanggal tertentu.
          </p>
          <p className="text-xs text-blue-700">
            Rate limit: 10 req/jam. Hanya tersedia untuk ROLE-IT-ADMIN (permission:{" "}
            <code>kurs.sync</code>).
          </p>
        </div>
      </div>

      {/* Trigger card */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Trigger JISDOR Sync Manual</CardTitle>
          <CardDescription>
            Memulai fetch kurs dari BI JISDOR untuk tanggal target. Proses berjalan async
            di background — progres tersedia di panel di bawah.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Date picker */}
          <div className="space-y-1.5">
            <Label htmlFor="jisdor-tanggal-target" className="flex items-center gap-1.5">
              <Calendar className="h-4 w-4 text-muted-foreground" aria-hidden="true" />
              Tanggal Target
            </Label>
            <div className="flex gap-2 items-center">
              <Input
                id="jisdor-tanggal-target"
                type="date"
                value={tanggalTarget}
                max={new Date().toISOString().split("T")[0] as string}
                onChange={(e) => setTanggalTarget(e.target.value)}
                className="w-48"
                aria-label="Tanggal target JISDOR sync"
              />
              <span className="text-sm text-muted-foreground">
                Kosongkan untuk hari ini
              </span>
            </div>
          </div>

          <JisdorSyncTriggerButton
            tanggalTarget={tanggalTarget || undefined}
            forceRefetch={false}
            onJobTriggered={(job) => setActiveJob(job)}
          />
        </CardContent>
      </Card>

      {/* Job progress panel (SSE + polling fallback) */}
      {activeJob && (
        <JisdorJobProgressPanel
          job={activeJob}
          onComplete={() => setActiveJob(null)}
        />
      )}

      {/* History */}
      <JisdorSyncHistoryCard />
    </div>
  );
}

export default function JisdorSyncPage() {
  return (
    <Suspense>
      <JisdorSyncContent />
    </Suspense>
  );
}
