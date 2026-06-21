"use client";

import * as React from "react";
import { Suspense } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useQueryState, parseAsInteger, parseAsString } from "nuqs";
import { ArrowLeft, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { AkrualMTDYTDCard } from "@/components/blips/akrual/AkrualMTDYTDCard";
import { akrualDashboardApi, akrualQueryKeys } from "@/lib/api/akrual.api";

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

function AkrualDashboardContent() {
  const now = new Date();
  const [year, setYear] = useQueryState("year", parseAsInteger.withDefault(now.getFullYear()));
  const [month, setMonth] = useQueryState("month", parseAsInteger.withDefault(now.getMonth() + 1));
  const [instrumenId] = useQueryState("instrumen_id", parseAsString.withDefault(""));
  const [portofolioId] = useQueryState("portofolio_id", parseAsString.withDefault(""));

  const params = {
    year,
    month,
    instrumen_id: instrumenId || undefined,
    portofolio_id: portofolioId || undefined,
  };

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: akrualQueryKeys.dashboard(params),
    queryFn: () => akrualDashboardApi.get(params),
    staleTime: 60_000,
  });

  const BULAN_NAMES = [
    "Januari", "Februari", "Maret", "April", "Mei", "Juni",
    "Juli", "Agustus", "September", "Oktober", "November", "Desember",
  ];

  return (
    <div className="container mx-auto py-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/transaksi/akrual" aria-label="Kembali ke daftar akrual">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="h-4 w-4 mr-1" aria-hidden="true" />
              Kembali
            </Button>
          </Link>
          <div>
            <h1 className="text-2xl font-bold">Dashboard Akrual MTD/YTD</h1>
            <p className="text-sm text-muted-foreground">
              Ringkasan pendapatan akrual bulan berjalan dan tahun berjalan — APP-B P5-M9
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void refetch()}
          aria-label="Refresh data dashboard"
        >
          <RefreshCw className="h-4 w-4 mr-1" aria-hidden="true" />
          Refresh
        </Button>
      </div>

      {/* Period selector */}
      <div className="flex items-center gap-3 p-3 rounded-md border bg-muted/30">
        <span className="text-sm font-medium">Periode:</span>
        <select
          className="text-sm border rounded px-2 py-1 bg-background"
          value={month}
          onChange={(e) => void setMonth(Number(e.target.value))}
          aria-label="Pilih bulan"
        >
          {BULAN_NAMES.map((name, i) => (
            <option key={i + 1} value={i + 1}>{name}</option>
          ))}
        </select>
        <select
          className="text-sm border rounded px-2 py-1 bg-background"
          value={year}
          onChange={(e) => void setYear(Number(e.target.value))}
          aria-label="Pilih tahun"
        >
          {[2024, 2025, 2026, 2027].map((y) => (
            <option key={y} value={y}>{y}</option>
          ))}
        </select>
      </div>

      {/* Content */}
      {isLoading && (
        <div className="rounded-lg border p-8 text-center text-muted-foreground" aria-live="polite">
          Memuat data dashboard...
        </div>
      )}

      {isError && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-8 text-center" role="alert">
          <p className="text-red-700 mb-2">Gagal memuat data dashboard.</p>
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            Coba Lagi
          </Button>
        </div>
      )}

      {data?.data && (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          <AkrualMTDYTDCard data={data.data} className="md:col-span-2 xl:col-span-1" />

          {/* Breakdown table */}
          {data.data.breakdown.length > 0 && (
            <div className="rounded-lg border bg-card p-4 space-y-3 md:col-span-2">
              <h2 className="text-sm font-semibold">Rincian per Jenis Akrual</h2>
              <div className="overflow-auto">
                <table className="w-full text-sm" aria-label="Rincian akrual per jenis">
                  <thead>
                    <tr className="border-b text-left">
                      <th className="pb-2 font-medium text-muted-foreground">Jenis</th>
                      <th className="pb-2 font-medium text-muted-foreground text-right">
                        MTD ({BULAN_NAMES[month - 1]})
                      </th>
                      <th className="pb-2 font-medium text-muted-foreground text-right">
                        YTD ({year})
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {data.data.breakdown.map((row) => {
                      const IDR_FMT = new Intl.NumberFormat("id-ID", {
                        style: "currency",
                        currency: "IDR",
                        minimumFractionDigits: 4,
                      });
                      return (
                        <tr key={row.jenis} className="border-b last:border-0">
                          <td className="py-2 text-muted-foreground">{row.jenis}</td>
                          <td className="py-2 text-right font-mono tabular-nums">
                            {IDR_FMT.format(parseFloat(row.mtdIdr))}
                          </td>
                          <td className="py-2 text-right font-mono tabular-nums">
                            {IDR_FMT.format(parseFloat(row.ytdIdr))}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export default function AkrualDashboardPage() {
  return (
    <Suspense>
      <AkrualDashboardContent />
    </Suspense>
  );
}
